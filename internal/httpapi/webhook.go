package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const webhookReplayWindow = 300 * time.Second

type WebhookReceiverConfig struct {
	MaxBodyBytes int64
	Receivers    map[string]WebhookReceiver
}

type WebhookReceiver struct {
	ReceiverCode         string
	Enabled              bool
	Secret               string
	AllowedTenantIDs     []string
	AllowedSourceSystems []string
	AllowedIPs           []string
	Mapping              WebhookMapping
}

type WebhookMapping struct {
	EventID      WebhookMappingRule
	EventType    WebhookMappingRule
	SourceSystem WebhookMappingRule
	TenantID     WebhookMappingRule
	SubjectType  WebhookMappingRule
	SubjectID    WebhookMappingRule
	BizType      WebhookMappingRule
	BizID        WebhookMappingRule
	OccurredAt   WebhookMappingRule
	Attributes   map[string]WebhookMappingRule
}

type WebhookMappingRule struct {
	Path     string
	Const    any
	HasConst bool
}

func LoadWebhookReceiverConfigFromEnv() (WebhookReceiverConfig, error) {
	cfg := WebhookReceiverConfig{MaxBodyBytes: defaultEventMaxBodyBytes, Receivers: map[string]WebhookReceiver{}}
	raw := strings.TrimSpace(os.Getenv("EX_MARKETING_WEBHOOK_RECEIVERS_JSON"))
	if raw == "" {
		return cfg, nil
	}
	var receivers []webhookReceiverJSON
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receivers); err != nil {
		return WebhookReceiverConfig{}, fmt.Errorf("EX_MARKETING_WEBHOOK_RECEIVERS_JSON invalid: %w", err)
	}
	for _, item := range receivers {
		receiver := WebhookReceiver{
			ReceiverCode:         strings.TrimSpace(item.ReceiverCode),
			Enabled:              item.Enabled,
			Secret:               item.Secret,
			AllowedTenantIDs:     trimStringSlice(item.AllowedTenantIDs),
			AllowedSourceSystems: trimStringSlice(item.AllowedSourceSystems),
			AllowedIPs:           trimStringSlice(item.AllowedIPs),
			Mapping:              item.Mapping,
		}
		if err := receiver.validate(); err != nil {
			return WebhookReceiverConfig{}, err
		}
		if _, exists := cfg.Receivers[receiver.ReceiverCode]; exists {
			return WebhookReceiverConfig{}, fmt.Errorf("webhook receiverCode %q is duplicated", receiver.ReceiverCode)
		}
		cfg.Receivers[receiver.ReceiverCode] = receiver
	}
	return cfg, nil
}

type webhookReceiverJSON struct {
	ReceiverCode         string         `json:"receiverCode"`
	Enabled              bool           `json:"enabled"`
	Secret               string         `json:"secret"`
	AllowedTenantIDs     []string       `json:"allowedTenantIds"`
	AllowedSourceSystems []string       `json:"allowedSourceSystems"`
	AllowedIPs           []string       `json:"allowedIPs"`
	Mapping              WebhookMapping `json:"mapping"`
}

func (m *WebhookMapping) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	required := map[string]*WebhookMappingRule{
		"eventId":      &m.EventID,
		"eventType":    &m.EventType,
		"sourceSystem": &m.SourceSystem,
		"tenantId":     &m.TenantID,
		"subject.type": &m.SubjectType,
		"subject.id":   &m.SubjectID,
		"biz.bizType":  &m.BizType,
		"biz.bizId":    &m.BizID,
		"occurredAt":   &m.OccurredAt,
	}
	for key, target := range required {
		value, ok := raw[key]
		if !ok {
			return fmt.Errorf("webhook mapping %s is required", key)
		}
		if err := json.Unmarshal(value, target); err != nil {
			return fmt.Errorf("webhook mapping %s invalid: %w", key, err)
		}
		delete(raw, key)
	}
	m.Attributes = map[string]WebhookMappingRule{}
	if value, ok := raw["attributes"]; ok {
		if err := json.Unmarshal(value, &m.Attributes); err != nil {
			return fmt.Errorf("webhook mapping attributes invalid: %w", err)
		}
		delete(raw, "attributes")
	}
	if len(raw) > 0 {
		for key := range raw {
			return fmt.Errorf("webhook mapping %s is unknown", key)
		}
	}
	return nil
}

func (r *WebhookMappingRule) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	path, hasPath := raw["path"]
	constValue, hasConst := raw["const"]
	if hasPath == hasConst {
		return errors.New("exactly one of path or const is required")
	}
	if len(raw) != 1 {
		return errors.New("only path or const is allowed")
	}
	if hasPath {
		if err := json.Unmarshal(path, &r.Path); err != nil || strings.TrimSpace(r.Path) == "" {
			return errors.New("path must be non-empty string")
		}
		if !validWebhookPath(r.Path) {
			return fmt.Errorf("path %q is unsupported", r.Path)
		}
		return nil
	}
	decoder = json.NewDecoder(bytes.NewReader(constValue))
	decoder.UseNumber()
	if err := decoder.Decode(&r.Const); err != nil {
		return err
	}
	r.HasConst = true
	return nil
}

func (h *Handler) createWebhookEvent(w http.ResponseWriter, r *http.Request) {
	receiverCode := strings.TrimSpace(r.PathValue("receiverCode"))
	if receiverCode == "" {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_RECEIVER_NOT_FOUND", "webhook receiver not found", receiverCode, nil, nil, "")
		return
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeEventError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "unsupported media type")
		return
	}
	receiver, ok := h.webhookCfg.Receivers[receiverCode]
	if !ok {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_RECEIVER_NOT_FOUND", "webhook receiver not found", receiverCode, nil, nil, "")
		return
	}
	if !receiver.Enabled {
		h.writeWebhookFailure(w, r.Context(), http.StatusForbidden, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_RECEIVER_DISABLED", "webhook receiver disabled", receiverCode, nil, nil, "")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.webhookCfg.MaxBodyBytes))
	if err != nil {
		if isRequestTooLarge(err) {
			writeEventError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
			return
		}
		writeEventError(w, http.StatusBadRequest, "INVALID_JSON", "invalid json")
		return
	}
	if !receiver.allowsIP(clientIP(r)) {
		h.writeWebhookFailure(w, r.Context(), http.StatusForbidden, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_SOURCE_FORBIDDEN", "webhook source forbidden", receiverCode, body, r.Header, "")
		return
	}

	timestamp := strings.TrimSpace(r.Header.Get("X-ExMarketing-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-ExMarketing-Signature"))
	if timestamp == "" || signature == "" {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_SIGNATURE_FAILED", "WEBHOOK_SIGNATURE_MISSING", "webhook signature missing", receiverCode, body, r.Header, "")
		return
	}
	if err := validateWebhookTimestamp(timestamp, time.Now().UTC()); err != nil {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_SIGNATURE_FAILED", "WEBHOOK_TIMESTAMP_INVALID", "webhook timestamp invalid", receiverCode, body, r.Header, "")
		return
	}
	if !validWebhookSignature(receiver.Secret, timestamp, body, signature) {
		h.writeWebhookFailure(w, r.Context(), http.StatusUnauthorized, "WEBHOOK_SIGNATURE_FAILED", "WEBHOOK_SIGNATURE_INVALID", "webhook signature invalid", receiverCode, body, r.Header, "")
		return
	}

	payload := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_JSON", "invalid json", receiverCode, body, r.Header, "")
		return
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_JSON", "invalid json", receiverCode, body, r.Header, "")
		return
	}

	event, err := receiver.Mapping.mapEvent(payload)
	if err != nil {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_MAPPING_FAILED", "WEBHOOK_MAPPING_FAILED", "webhook mapping failed", receiverCode, body, r.Header, "")
		return
	}
	if !slices.Contains(receiver.AllowedTenantIDs, event.TenantID) || !slices.Contains(receiver.AllowedSourceSystems, event.SourceSystem) {
		h.writeWebhookFailure(w, r.Context(), http.StatusForbidden, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_SOURCE_FORBIDDEN", "webhook source forbidden", receiverCode, body, r.Header, event.TenantID)
		return
	}
	if err := event.Validate(); err != nil {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_EVENT", err.Error(), receiverCode, body, r.Header, event.TenantID)
		return
	}
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key != "" && key != event.IdempotencyKey() {
		h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key does not match event payload", receiverCode, body, r.Header, event.TenantID)
		return
	}

	result, err := h.engine.ProcessEventFrom(r.Context(), event, "webhook:"+receiverCode)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidEvent) {
			h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_EVENT", err.Error(), receiverCode, body, r.Header, event.TenantID)
			return
		}
		if errors.Is(err, marketing.ErrIdempotencyConflict) {
			h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_EVENT_INVALID", "INVALID_IDEMPOTENCY_KEY", err.Error(), receiverCode, body, r.Header, event.TenantID)
			return
		}
		if errors.Is(err, marketing.ErrQuotaExceeded) {
			if !h.recordWebhookAudit(r.Context(), event.TenantID, "WEBHOOK_QUOTA_EXCEEDED", receiverCode, body, r.Header, event, "QUOTA_EXCEEDED", err.Error()) {
				writeEventError(w, http.StatusInternalServerError, "WEBHOOK_AUDIT_FAILED", "webhook audit failed")
				return
			}
			writeQuotaError(w, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := "WEBHOOK_RECEIVED"
	status := http.StatusAccepted
	if result.Duplicate {
		action = "WEBHOOK_DUPLICATE"
		status = http.StatusOK
	}
	if !h.recordWebhookAudit(r.Context(), event.TenantID, action, receiverCode, body, r.Header, event, "", "") {
		writeEventError(w, http.StatusInternalServerError, "WEBHOOK_AUDIT_FAILED", "webhook audit failed")
		return
	}
	writeJSON(w, status, result)
}

func (h *Handler) missingWebhookReceiver(w http.ResponseWriter, r *http.Request) {
	h.writeWebhookFailure(w, r.Context(), http.StatusBadRequest, "WEBHOOK_RECEIVER_REJECTED", "WEBHOOK_RECEIVER_NOT_FOUND", "webhook receiver not found", "", nil, nil, "")
}

func (h *Handler) writeWebhookFailure(w http.ResponseWriter, ctx context.Context, status int, action string, code string, message string, receiverCode string, body []byte, header http.Header, tenantID string) {
	if !h.recordWebhookAudit(ctx, tenantID, action, receiverCode, body, header, marketing.ActivityEvent{}, code, message) {
		writeEventError(w, http.StatusInternalServerError, "WEBHOOK_AUDIT_FAILED", "webhook audit failed")
		return
	}
	writeEventError(w, status, code, message)
}

func (h *Handler) recordWebhookAudit(ctx context.Context, tenantID string, action string, receiverCode string, body []byte, header http.Header, event marketing.ActivityEvent, errorCode string, message string) bool {
	signature := ""
	if header != nil {
		signature = strings.TrimSpace(header.Get("X-ExMarketing-Signature"))
	}
	payload := map[string]any{
		"receiverCode":      receiverCode,
		"rawPayloadSha256":  sha256Hex(body),
		"signatureSha256":   sha256Hex([]byte(signature)),
		"signatureProvided": signature != "",
	}
	if event.EventID != "" {
		payload["eventId"] = event.EventID
		payload["idempotencyKey"] = event.IdempotencyKey()
	}
	if providerEventID, ok := event.Attributes["providerEventId"]; ok {
		payload["providerEventId"] = providerEventID
	}
	if errorCode != "" {
		payload["errorCode"] = errorCode
		payload["message"] = message
	}
	return h.engine.RecordAudit(ctx, tenantID, "webhook:"+receiverCode, action, "WEBHOOK_RECEIVER", receiverCode, payload) == nil
}

func (m WebhookMapping) mapEvent(payload map[string]any) (marketing.ActivityEvent, error) {
	attributes := map[string]any{}
	for key, rule := range m.Attributes {
		value, err := rule.value(payload)
		if err != nil {
			return marketing.ActivityEvent{}, err
		}
		attributes[key] = value
	}
	event := marketing.ActivityEvent{
		EventID:      "",
		EventType:    "",
		SourceSystem: "",
		TenantID:     "",
		Subject:      marketing.EventSubject{},
		Biz:          marketing.EventBiz{},
		Attributes:   attributes,
	}
	var err error
	if event.EventID, err = m.EventID.stringValue(payload, "eventId"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.EventType, err = m.EventType.stringValue(payload, "eventType"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.SourceSystem, err = m.SourceSystem.stringValue(payload, "sourceSystem"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.TenantID, err = m.TenantID.stringValue(payload, "tenantId"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.Subject.Type, err = m.SubjectType.stringValue(payload, "subject.type"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.Subject.ID, err = m.SubjectID.stringValue(payload, "subject.id"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.Biz.BizType, err = m.BizType.stringValue(payload, "biz.bizType"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	if event.Biz.BizID, err = m.BizID.stringValue(payload, "biz.bizId"); err != nil {
		return marketing.ActivityEvent{}, err
	}
	occurredAt, err := m.OccurredAt.stringValue(payload, "occurredAt")
	if err != nil {
		return marketing.ActivityEvent{}, err
	}
	event.OccurredAt, err = time.Parse(time.RFC3339, occurredAt)
	if err != nil {
		return marketing.ActivityEvent{}, fmt.Errorf("occurredAt must be RFC3339 time with timezone")
	}
	return event, nil
}

func (r WebhookMappingRule) stringValue(payload map[string]any, field string) (string, error) {
	value, err := r.value(payload)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	return text, nil
}

func (r WebhookMappingRule) value(payload map[string]any) (any, error) {
	if r.HasConst {
		return r.Const, nil
	}
	current := any(payload)
	parts := strings.Split(strings.TrimPrefix(r.Path, "$."), ".")
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %s is not object at %s", r.Path, part)
		}
		value, ok := object[part]
		if !ok || value == nil {
			return nil, fmt.Errorf("path %s is missing", r.Path)
		}
		current = value
	}
	return current, nil
}

func (r WebhookReceiver) validate() error {
	if r.ReceiverCode == "" {
		return errors.New("webhook receiverCode is required")
	}
	if strings.Contains(r.ReceiverCode, "/") {
		return fmt.Errorf("webhook receiverCode %q is invalid", r.ReceiverCode)
	}
	if strings.TrimSpace(r.Secret) == "" {
		return fmt.Errorf("webhook receiver %s secret is required", r.ReceiverCode)
	}
	if len(r.AllowedTenantIDs) == 0 {
		return fmt.Errorf("webhook receiver %s allowedTenantIds is required", r.ReceiverCode)
	}
	if len(r.AllowedSourceSystems) == 0 {
		return fmt.Errorf("webhook receiver %s allowedSourceSystems is required", r.ReceiverCode)
	}
	for _, allowedIP := range r.AllowedIPs {
		if net.ParseIP(allowedIP) == nil {
			if _, _, err := net.ParseCIDR(allowedIP); err != nil {
				return fmt.Errorf("webhook receiver %s allowedIP %q is invalid", r.ReceiverCode, allowedIP)
			}
		}
	}
	return nil
}

func (r WebhookReceiver) allowsIP(ipText string) bool {
	if len(r.AllowedIPs) == 0 {
		return true
	}
	ip := net.ParseIP(ipText)
	if ip == nil {
		return false
	}
	for _, allowed := range r.AllowedIPs {
		if allowedIP := net.ParseIP(allowed); allowedIP != nil && allowedIP.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(allowed); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func validateWebhookTimestamp(value string, now time.Time) error {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	requestTime := time.Unix(seconds, 0).UTC()
	delta := now.Sub(requestTime)
	if delta < 0 {
		delta = -delta
	}
	if delta > webhookReplayWindow {
		return fmt.Errorf("timestamp outside replay window")
	}
	return nil
}

func validWebhookSignature(secret string, timestamp string, body []byte, signature string) bool {
	if signature != strings.ToLower(signature) {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)
	return subtle.ConstantTimeCompare(provided, expected) == 1
}

func validWebhookPath(path string) bool {
	if !strings.HasPrefix(path, "$.") || len(path) <= 2 {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		if part == "" || strings.ContainsAny(part, "[]*?() ") {
			return false
		}
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func trimStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
