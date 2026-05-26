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
)

const (
	CodeProviderNotConfigured = "FACT_PROVIDER_NOT_CONFIGURED"
	CodeProviderDisabled      = "FACT_PROVIDER_DISABLED"
	CodeProviderHTTPFailed    = "FACT_PROVIDER_HTTP_FAILED"
	CodeProviderTimeout       = "FACT_PROVIDER_TIMEOUT"
	CodeProviderBlocked       = "FACT_PROVIDER_BLOCKED"
	CodeMappingFailed         = "FACT_MAPPING_FAILED"
	CodeCacheFailed           = "FACT_CACHE_FAILED"
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

func NewClientWithHTTPClient(httpClient *http.Client) *Client {
	client := NewClient()
	if httpClient != nil {
		client.httpClient = httpClient
	}
	return client
}

func (c *Client) Query(ctx context.Context, provider marketing.FactProvider, request marketing.FactQueryRequest) marketing.FactQueryResponse {
	if !provider.Enabled {
		return failure(request, CodeProviderDisabled, "fact provider is disabled", "")
	}
	if err := provider.Validate(); err != nil {
		return failure(request, CodeProviderNotConfigured, err.Error(), "")
	}
	if request.TenantID != provider.TenantID || request.FactCode != provider.FactCode {
		return failure(request, CodeProviderNotConfigured, "fact provider is not configured for tenant and factCode", "")
	}
	if provider.CacheTTLSeconds > 0 {
		key, err := cacheKey(provider, request)
		if err != nil {
			return failure(request, CodeCacheFailed, err.Error(), "")
		}
		if cached, ok := c.cached(key); ok {
			cached.Cached = true
			return cached
		}
		response := c.queryProvider(ctx, provider, request)
		if response.Success {
			c.store(key, response, time.Duration(provider.CacheTTLSeconds)*time.Second)
		}
		return response
	}
	return c.queryProvider(ctx, provider, request)
}

func (c *Client) queryProvider(ctx context.Context, provider marketing.FactProvider, request marketing.FactQueryRequest) marketing.FactQueryResponse {
	if err := validateEndpoint(provider.Endpoint); err != nil {
		return failure(request, CodeProviderBlocked, err.Error(), "")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return failure(request, CodeProviderHTTPFailed, err.Error(), "")
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
		return failure(request, CodeProviderHTTPFailed, err.Error(), "")
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		code := CodeProviderHTTPFailed
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			code = CodeProviderTimeout
		}
		return failure(request, code, "fact provider request failed", err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return failure(request, CodeProviderHTTPFailed, "fact provider returned non-2xx status", resp.Status)
	}
	var raw map[string]any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return failure(request, CodeMappingFailed, "fact provider returned invalid json", resp.Status)
	}
	mapped, err := mapFacts(provider, raw)
	if err != nil {
		return failure(request, CodeMappingFailed, err.Error(), resp.Status)
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

func failure(request marketing.FactQueryRequest, code string, message string, providerStatus string) marketing.FactQueryResponse {
	return marketing.FactQueryResponse{
		TenantID:       request.TenantID,
		FactCode:       request.FactCode,
		Success:        false,
		Cached:         false,
		ErrorCode:      code,
		Error:          message,
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
	attributes, err := json.Marshal(request.Attributes)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		provider.TenantID,
		provider.FactCode,
		provider.UpdatedAt.UTC().Format(time.RFC3339Nano),
		request.Subject.Type,
		request.Subject.ID,
		request.Biz.BizType,
		request.Biz.BizID,
		string(attributes),
	}, "\x00"), nil
}

func mapFacts(provider marketing.FactProvider, raw map[string]any) (map[string]any, error) {
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
			return nil, fmt.Errorf("FACT_FIELD_MISSING: mapping source %s for %s not found", mapping.Source, outputPath)
		}
		if !mappingTypeMatches(value, mapping.Type) {
			return nil, fmt.Errorf("FACT_TYPE_MISMATCH: mapping source %s for %s is not %s", mapping.Source, outputPath, mapping.Type)
		}
		setNested(out, outputPath, value)
	}
	return out, nil
}

func jsonPathValue(raw map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "$.") {
		return nil, false
	}
	var current any = raw
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
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
	q.Set("tenantId", request.TenantID)
	q.Set("factCode", request.FactCode)
	q.Set("subjectType", request.Subject.Type)
	q.Set("subjectId", request.Subject.ID)
	q.Set("bizType", request.Biz.BizType)
	q.Set("bizId", request.Biz.BizID)
	if request.TraceID != "" {
		q.Set("traceId", request.TraceID)
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
