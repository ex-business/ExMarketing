package fact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
)

const (
	CodeInvalidFactQuery      = "INVALID_FACT_QUERY"
	CodeProviderNotConfigured = "FACT_PROVIDER_NOT_CONFIGURED"
	CodeProviderDisabled      = "FACT_PROVIDER_DISABLED"
	CodeProviderHTTPFailed    = "FACT_PROVIDER_HTTP_FAILED"
	CodeProviderTimeout       = "FACT_PROVIDER_TIMEOUT"
	CodeProviderHostBlocked   = "FACT_PROVIDER_HOST_NOT_ALLOWLISTED"
	CodeProviderPrivateIP     = "FACT_PROVIDER_PRIVATE_IP_BLOCKED"
	CodeMappingFailed         = "FACT_MAPPING_FAILED"
	CodeCacheUnavailable      = "FACT_CACHE_UNAVAILABLE"
)

const (
	ReasonMappingRequiredMissing = "MAPPING_REQUIRED_MISSING"
	ReasonMappingTypeMismatch    = "MAPPING_TYPE_MISMATCH"
	ReasonMappingKeyInvalid      = "MAPPING_KEY_INVALID"
	ReasonMappingSourceInvalid   = "MAPPING_SOURCE_INVALID"
	ReasonMappingConfigInvalid   = "MAPPING_CONFIG_INVALID"
)

type Client struct {
	httpClient *http.Client
	mu         sync.Mutex
	cache      map[string]cacheEntry
}

type cacheEntry struct {
	expiresAt time.Time
	response  marketing.FactQueryResponse
}

type mappingError struct {
	Reason  string
	Message string
}

func (e mappingError) Error() string {
	if e.Reason == "" {
		return e.Message
	}
	return e.Reason + ": " + e.Message
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
			Transport:     secureTransport(),
		},
		cache: map[string]cacheEntry{},
	}
}

func ValidateRequest(request marketing.FactQueryRequest) error {
	if strings.TrimSpace(request.TenantID) == "" {
		return errors.New("tenantId is required")
	}
	if strings.TrimSpace(request.FactCode) == "" {
		return errors.New("factCode is required")
	}
	if strings.TrimSpace(request.Subject.Type) == "" {
		return errors.New("subject.type is required")
	}
	if strings.TrimSpace(request.Subject.ID) == "" {
		return errors.New("subject.id is required")
	}
	if strings.TrimSpace(request.Biz.BizType) == "" {
		return errors.New("biz.bizType is required")
	}
	if strings.TrimSpace(request.Biz.BizID) == "" {
		return errors.New("biz.bizId is required")
	}
	if request.Attributes == nil {
		return errors.New("attributes is required")
	}
	return nil
}

func providerValidationFailure(request marketing.FactQueryRequest, err error) marketing.FactQueryResponse {
	message := err.Error()
	if strings.Contains(message, "mapping path") {
		return failure(request, CodeMappingFailed, message, ReasonMappingKeyInvalid, "")
	}
	if strings.Contains(message, "source is invalid") {
		return failure(request, CodeMappingFailed, message, ReasonMappingSourceInvalid, "")
	}
	if strings.Contains(message, "mapping is required") || strings.Contains(message, "type is invalid") {
		return failure(request, CodeMappingFailed, message, ReasonMappingConfigInvalid, "")
	}
	return failure(request, CodeProviderNotConfigured, message, "", "")
}

func endpointFailure(request marketing.FactQueryRequest, err error) marketing.FactQueryResponse {
	message := err.Error()
	if strings.Contains(message, "not allowlisted") {
		return failure(request, CodeProviderHostBlocked, message, "", "")
	}
	if strings.Contains(message, "private") || strings.Contains(message, "local") {
		return failure(request, CodeProviderPrivateIP, message, "", "")
	}
	return failure(request, CodeProviderHTTPFailed, message, "", "")
}

func NewClientWithHTTPClient(httpClient *http.Client) *Client {
	client := NewClient()
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client
}

func (c *Client) Query(ctx context.Context, provider marketing.FactProvider, request marketing.FactQueryRequest) marketing.FactQueryResponse {
	startedAt := time.Now()
	var response marketing.FactQueryResponse
	defer func() {
		recordFactQueryMetrics(request, response, startedAt)
	}()
	if err := ValidateRequest(request); err != nil {
		response = failure(request, CodeInvalidFactQuery, err.Error(), "", "")
		return response
	}
	if !provider.Enabled {
		response = failure(request, CodeProviderDisabled, "fact provider is disabled", "", "")
		return response
	}
	if err := provider.Validate(); err != nil {
		response = providerValidationFailure(request, err)
		return response
	}
	if request.TenantID != provider.TenantID || request.FactCode != provider.FactCode {
		response = failure(request, CodeProviderNotConfigured, "fact provider is not configured for tenant and factCode", "", "")
		return response
	}
	if provider.CacheTTLSeconds > 0 {
		key, err := cacheKey(provider, request)
		if err != nil {
			response = failure(request, CodeCacheUnavailable, err.Error(), "", "")
			return response
		}
		if cached, ok := c.cached(key); ok {
			cached.Cached = true
			response = cached
			return response
		}
		response = c.queryProvider(ctx, provider, request)
		if response.Success {
			c.store(key, response, time.Duration(provider.CacheTTLSeconds)*time.Second)
		}
		return response
	}
	response = c.queryProvider(ctx, provider, request)
	return response
}

func recordFactQueryMetrics(request marketing.FactQueryRequest, response marketing.FactQueryResponse, startedAt time.Time) {
	result := factQueryResult(response)
	reason := factQueryReason(response)
	metrics.Default.IncCounter("exmarketing_fact_query_total",
		metrics.L("tenant_id", request.TenantID),
		metrics.L("fact_code", request.FactCode),
		metrics.L("result", result),
		metrics.L("reason", reason),
	)
	metrics.Default.Observe("exmarketing_fact_query_latency_seconds", time.Since(startedAt).Seconds(),
		metrics.L("tenant_id", request.TenantID),
		metrics.L("fact_code", request.FactCode),
		metrics.L("result", result),
	)
}

func factQueryResult(response marketing.FactQueryResponse) string {
	if response.Success && response.Cached {
		return "cache_hit"
	}
	if response.Success {
		return "success"
	}
	switch response.ErrorCode {
	case CodeProviderTimeout:
		return "timeout"
	case CodeMappingFailed:
		return "mapping_failed"
	case CodeInvalidFactQuery, CodeProviderNotConfigured, CodeProviderDisabled, CodeProviderHostBlocked, CodeProviderPrivateIP:
		return "rejected"
	default:
		return "failed"
	}
}

func factQueryReason(response marketing.FactQueryResponse) string {
	if response.Success && response.Cached {
		return "cache_hit"
	}
	if response.Success {
		return "ok"
	}
	if response.Reason != "" {
		return strings.ToLower(response.Reason)
	}
	switch response.ErrorCode {
	case CodeProviderTimeout:
		return "provider_timeout"
	case CodeProviderHTTPFailed:
		return "provider_http_error"
	case CodeProviderNotConfigured:
		return "provider_not_configured"
	case CodeProviderDisabled:
		return "provider_disabled"
	case CodeProviderHostBlocked, CodeProviderPrivateIP:
		return "provider_blocked"
	case CodeInvalidFactQuery:
		return "invalid_request"
	case CodeCacheUnavailable:
		return "cache_unavailable"
	default:
		return "unknown"
	}
}

func (c *Client) queryProvider(ctx context.Context, provider marketing.FactProvider, request marketing.FactQueryRequest) marketing.FactQueryResponse {
	if err := validateEndpoint(provider.Endpoint); err != nil {
		return endpointFailure(request, err)
	}
	if provider.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(provider.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	input := providerInput(request)
	body, err := json.Marshal(input)
	if err != nil {
		return failure(request, CodeProviderHTTPFailed, err.Error(), "", "")
	}
	method := strings.ToUpper(strings.TrimSpace(provider.Method))
	endpoint := provider.Endpoint
	var req *http.Request
	if method == http.MethodGet {
		endpoint = withQuery(endpoint, request)
		req, err = http.NewRequestWithContext(ctx, method, endpoint, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	}
	if err != nil {
		return failure(request, CodeProviderHTTPFailed, err.Error(), "", "")
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		code := CodeProviderHTTPFailed
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			code = CodeProviderTimeout
		}
		return failure(request, code, "fact provider request failed", "", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return failure(request, CodeProviderHTTPFailed, "fact provider returned non-2xx status", "", resp.Status)
	}
	var raw map[string]any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return failure(request, CodeProviderHTTPFailed, "fact provider returned invalid json", "", resp.Status)
	}
	mapped, err := mapFacts(provider, raw)
	if err != nil {
		var mappingErr mappingError
		if errors.As(err, &mappingErr) {
			return failure(request, CodeMappingFailed, mappingErr.Message, mappingErr.Reason, resp.Status)
		}
		return failure(request, CodeMappingFailed, err.Error(), ReasonMappingConfigInvalid, resp.Status)
	}
	return marketing.FactQueryResponse{
		TenantID:       request.TenantID,
		FactCode:       request.FactCode,
		Success:        true,
		Cached:         false,
		Facts:          map[string]any{provider.FactCode: mapped},
		ProviderStatus: resp.Status,
	}
}

func failure(request marketing.FactQueryRequest, code string, message string, reason string, providerStatus string) marketing.FactQueryResponse {
	return marketing.FactQueryResponse{
		TenantID:       request.TenantID,
		FactCode:       request.FactCode,
		Success:        false,
		Cached:         false,
		ErrorCode:      code,
		Error:          message,
		Reason:         reason,
		ProviderStatus: providerStatus,
	}
}

func (c *Client) cached(key string) (marketing.FactQueryResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[key]
	if !ok || time.Now().UTC().After(entry.expiresAt) {
		delete(c.cache, key)
		return marketing.FactQueryResponse{}, false
	}
	return entry.response, true
}

func (c *Client) store(key string, response marketing.FactQueryResponse, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	response.Cached = false
	c.cache[key] = cacheEntry{expiresAt: time.Now().UTC().Add(ttl), response: response}
}

func cacheKey(provider marketing.FactProvider, request marketing.FactQueryRequest) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"providerVersion": providerVersion(provider),
		"input":           providerInput(request),
	})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func providerVersion(provider marketing.FactProvider) map[string]any {
	if !provider.UpdatedAt.IsZero() {
		return map[string]any{
			"tenantId":   provider.TenantID,
			"factCode":   provider.FactCode,
			"updatedAt":  provider.UpdatedAt.UTC().Format(time.RFC3339Nano),
			"versionKey": "updatedAt",
		}
	}
	return map[string]any{
		"tenantId":        provider.TenantID,
		"factCode":        provider.FactCode,
		"providerType":    strings.ToUpper(strings.TrimSpace(provider.ProviderType)),
		"endpoint":        strings.TrimSpace(provider.Endpoint),
		"method":          strings.ToUpper(strings.TrimSpace(provider.Method)),
		"authRef":         provider.AuthRef,
		"mapping":         provider.Mapping,
		"cacheTtlSeconds": provider.CacheTTLSeconds,
		"enabled":         provider.Enabled,
		"timeoutMs":       provider.TimeoutMS,
		"versionKey":      "configFingerprint",
	}
}

func providerInput(request marketing.FactQueryRequest) map[string]any {
	input := map[string]any{
		"tenantId":   request.TenantID,
		"factCode":   request.FactCode,
		"subject":    map[string]any{"type": request.Subject.Type, "id": request.Subject.ID},
		"biz":        map[string]any{"bizType": request.Biz.BizType, "bizId": request.Biz.BizID},
		"attributes": request.Attributes,
	}
	if request.EventID != "" {
		input["eventId"] = request.EventID
	}
	if request.EventType != "" {
		input["eventType"] = request.EventType
	}
	if request.SourceSystem != "" {
		input["sourceSystem"] = request.SourceSystem
	}
	if !request.OccurredAt.IsZero() {
		input["occurredAt"] = request.OccurredAt.UTC().Format(time.RFC3339Nano)
	}
	return input
}

func mapFacts(provider marketing.FactProvider, raw map[string]any) (map[string]any, error) {
	if err := provider.Validate(); err != nil {
		return nil, mappingReasonFromValidation(err)
	}
	out := map[string]any{}
	keys := make([]string, 0, len(provider.Mapping))
	for key := range provider.Mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, outputPath := range keys {
		mapping := provider.Mapping[outputPath]
		value, ok := jsonPathValue(raw, mapping.Source)
		if !ok {
			if !mappingRequired(mapping) {
				continue
			}
			return nil, mappingError{Reason: ReasonMappingRequiredMissing, Message: fmt.Sprintf("mapping source %s for %s not found", mapping.Source, outputPath)}
		}
		if !mappingTypeMatches(value, mapping.Type) {
			return nil, mappingError{Reason: ReasonMappingTypeMismatch, Message: fmt.Sprintf("mapping source %s for %s is not %s", mapping.Source, outputPath, mapping.Type)}
		}
		setNested(out, outputPath, value)
	}
	return out, nil
}

func mappingReasonFromValidation(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "mapping path"):
		return mappingError{Reason: ReasonMappingKeyInvalid, Message: message}
	case strings.Contains(message, "source is invalid"):
		return mappingError{Reason: ReasonMappingSourceInvalid, Message: message}
	default:
		return mappingError{Reason: ReasonMappingConfigInvalid, Message: message}
	}
}

func jsonPathValue(raw map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "$.") {
		return nil, false
	}
	var current any = raw
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		name, indexes, ok := parsePathPart(part)
		if !ok {
			return nil, false
		}
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[name]
		if !ok {
			return nil, false
		}
		for _, index := range indexes {
			array, ok := current.([]any)
			if !ok || index < 0 || index >= len(array) {
				return nil, false
			}
			current = array[index]
		}
	}
	return current, true
}

func mappingRequired(mapping marketing.FactMapping) bool {
	return mapping.Required == nil || *mapping.Required
}

func parsePathPart(part string) (string, []int, bool) {
	nameEnd := strings.Index(part, "[")
	if nameEnd == -1 {
		if part == "" {
			return "", nil, false
		}
		return part, nil, true
	}
	name := part[:nameEnd]
	if name == "" {
		return "", nil, false
	}
	var indexes []int
	rest := part[nameEnd:]
	for rest != "" {
		if !strings.HasPrefix(rest, "[") {
			return "", nil, false
		}
		close := strings.Index(rest, "]")
		if close <= 1 {
			return "", nil, false
		}
		var index int
		for _, ch := range rest[1:close] {
			if ch < '0' || ch > '9' {
				return "", nil, false
			}
			index = index*10 + int(ch-'0')
		}
		indexes = append(indexes, index)
		rest = rest[close+1:]
	}
	return name, indexes, true
}

func mappingTypeMatches(value any, typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case json.Number, float64, float32, int, int64, int32:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		return reflect.ValueOf(value).IsValid() && reflect.ValueOf(value).Kind() == reflect.Slice
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func setNested(out map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := out
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func withQuery(endpoint string, request marketing.FactQueryRequest) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	q := parsed.Query()
	for key, value := range providerInput(request) {
		switch typed := value.(type) {
		case string:
			q.Set(key, typed)
		default:
			encoded, err := json.Marshal(typed)
			if err == nil {
				q.Set(key, string(encoded))
			}
		}
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func validateEndpoint(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("fact endpoint must use http or https")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("fact endpoint host is required")
	}
	if !hostAllowed(host) {
		return errors.New("fact endpoint host is not allowlisted")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if isPrivateOrLocalIP(ip) {
			return errors.New("fact endpoint resolves to private or local address")
		}
	}
	return nil
}

func hostAllowed(host string) bool {
	allowlist := os.Getenv("EX_MARKETING_FACT_ALLOWLIST")
	if allowlist == "" {
		return false
	}
	for _, allowed := range strings.Split(allowlist, ",") {
		allowed = strings.TrimSpace(strings.ToLower(allowed))
		if allowed == "" {
			continue
		}
		host = strings.ToLower(host)
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func isPrivateOrLocalIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func secureTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isPrivateOrLocalIP(ip) {
				return nil, errors.New("fact endpoint resolves to private or local address")
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("fact endpoint did not resolve")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		return nil, nil
	}
	return transport
}
