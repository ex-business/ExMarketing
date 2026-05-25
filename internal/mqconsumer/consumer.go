package mqconsumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const idempotencyKeyHeader = "Idempotency-Key"

type BrokerKind string

const (
	BrokerKafka    BrokerKind = "kafka"
	BrokerRabbitMQ BrokerKind = "rabbitmq"
	BrokerPulsar   BrokerKind = "pulsar"
	BrokerFake     BrokerKind = "fake"
)

type Message struct {
	Broker   BrokerKind
	Topic    string
	Body     []byte
	Headers  map[string]string
	Metadata MessageMetadata
}

type MessageMetadata struct {
	MessageID     string
	Partition     string
	Offset        string
	DeliveryTag   string
	TraceID       string
	CorrelationID string
	Attempt       int
	PublishedAt   time.Time
}

type Action string

const (
	ActionAck       Action = "ACK"
	ActionNackRetry Action = "NACK_RETRY"
	ActionDLQ       Action = "DLQ"
)

type Result struct {
	Action                Action
	EventResult           marketing.ProcessEventResult
	Failure               *FailureRecord
	IdempotencyKey        string
	DerivedIdempotencyKey bool
}

type FailureRecord struct {
	ID             string            `json:"id"`
	Retryable      bool              `json:"retryable"`
	ReasonCode     string            `json:"reasonCode"`
	Message        string            `json:"message"`
	Broker         BrokerKind        `json:"broker"`
	Topic          string            `json:"topic"`
	MessageID      string            `json:"messageId,omitempty"`
	TraceID        string            `json:"traceId,omitempty"`
	CorrelationID  string            `json:"correlationId,omitempty"`
	TenantID       string            `json:"tenantId,omitempty"`
	SourceSystem   string            `json:"sourceSystem,omitempty"`
	EventType      string            `json:"eventType,omitempty"`
	BizType        string            `json:"bizType,omitempty"`
	BizID          string            `json:"bizId,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
}

type FailureStore interface {
	SaveFailure(ctx context.Context, record FailureRecord) error
}

type MemoryFailureStore struct {
	mu      sync.RWMutex
	records []FailureRecord
}

func NewMemoryFailureStore() *MemoryFailureStore {
	return &MemoryFailureStore{}
}

func (s *MemoryFailureStore) SaveFailure(ctx context.Context, record FailureRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

func (s *MemoryFailureStore) ListFailures() []FailureRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]FailureRecord, len(s.records))
	copy(out, s.records)
	return out
}

type SourceAllowlist map[string]map[string]struct{}

func NewSourceAllowlist(entries map[string][]string) SourceAllowlist {
	out := SourceAllowlist{}
	for tenantID, sources := range entries {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			continue
		}
		if out[tenantID] == nil {
			out[tenantID] = map[string]struct{}{}
		}
		for _, source := range sources {
			source = strings.TrimSpace(source)
			if source != "" {
				out[tenantID][source] = struct{}{}
			}
		}
	}
	return out
}

func (a SourceAllowlist) Allows(tenantID string, sourceSystem string) bool {
	if len(a) == 0 {
		return true
	}
	sources, ok := a[tenantID]
	if !ok {
		return false
	}
	_, ok = sources[sourceSystem]
	return ok
}

type Consumer struct {
	engine       *engine.Service
	failures     FailureStore
	allowlist    SourceAllowlist
	now          func() time.Time
	maxBodyBytes int
}

type Config struct {
	FailureStore FailureStore
	Allowlist    SourceAllowlist
	Now          func() time.Time
	MaxBodyBytes int
}

func New(service *engine.Service, cfg Config) *Consumer {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Consumer{
		engine:       service,
		failures:     cfg.FailureStore,
		allowlist:    cfg.Allowlist,
		now:          cfg.Now,
		maxBodyBytes: cfg.MaxBodyBytes,
	}
}

func (c *Consumer) Process(ctx context.Context, message Message) (Result, error) {
	if c.engine == nil {
		return Result{}, errors.New("mq consumer requires engine service")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if len(bytes.TrimSpace(message.Body)) == 0 {
		return c.fail(ctx, message, marketing.ActivityEvent{}, "", false, "INVALID_JSON", "message body is empty")
	}
	if c.maxBodyBytes > 0 && len(message.Body) > c.maxBodyBytes {
		return c.fail(ctx, message, marketing.ActivityEvent{}, "", false, "REQUEST_TOO_LARGE", "message body is too large")
	}

	var event marketing.ActivityEvent
	decoder := json.NewDecoder(bytes.NewReader(message.Body))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return c.fail(ctx, message, event, "", false, invalidDecodeReason(err), err.Error())
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return c.fail(ctx, message, event, "", false, invalidDecodeReason(err), err.Error())
	}
	if err := event.Validate(); err != nil {
		return c.fail(ctx, message, event, "", false, "INVALID_EVENT", err.Error())
	}
	event = event.Normalized()

	if !c.allowlist.Allows(event.TenantID, event.SourceSystem) {
		return c.fail(ctx, message, event, event.IdempotencyKey(), false, "SOURCE_FORBIDDEN", "tenantId/sourceSystem is not allowed")
	}

	headerKey := headerValue(message.Headers, idempotencyKeyHeader)
	derivedKey := event.IdempotencyKey()
	derived := strings.TrimSpace(headerKey) == ""
	if !derived && headerKey != derivedKey {
		return c.fail(ctx, message, event, headerKey, false, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key does not match event payload")
	}

	result, err := c.engine.ProcessEventFrom(ctx, event, "mq")
	if err != nil {
		switch {
		case errors.Is(err, marketing.ErrInvalidEvent):
			return c.fail(ctx, message, event, derivedKey, false, "INVALID_EVENT", err.Error())
		case errors.Is(err, marketing.ErrIdempotencyConflict):
			return c.fail(ctx, message, event, derivedKey, false, "INVALID_IDEMPOTENCY_KEY", err.Error())
		case errors.Is(err, marketing.ErrQuotaExceeded):
			return c.fail(ctx, message, event, derivedKey, false, "QUOTA_EXCEEDED", err.Error())
		default:
			failureResult, saveErr := c.fail(ctx, message, event, derivedKey, true, "RETRYABLE_PROCESSING_ERROR", err.Error())
			if saveErr != nil {
				return failureResult, saveErr
			}
			return failureResult, err
		}
	}

	return Result{
		Action:                ActionAck,
		EventResult:           result,
		IdempotencyKey:        derivedKey,
		DerivedIdempotencyKey: derived,
	}, nil
}

func (c *Consumer) fail(ctx context.Context, message Message, event marketing.ActivityEvent, idempotencyKey string, retryable bool, reasonCode string, detail string) (Result, error) {
	action := ActionDLQ
	if retryable {
		action = ActionNackRetry
	}
	record := FailureRecord{
		ID:             fmt.Sprintf("mqfail_%d", c.now().UnixNano()),
		Retryable:      retryable,
		ReasonCode:     reasonCode,
		Message:        detail,
		Broker:         message.Broker,
		Topic:          message.Topic,
		MessageID:      message.Metadata.MessageID,
		TraceID:        message.Metadata.TraceID,
		CorrelationID:  message.Metadata.CorrelationID,
		TenantID:       event.TenantID,
		SourceSystem:   event.SourceSystem,
		EventType:      event.EventType,
		BizType:        event.Biz.BizType,
		BizID:          event.Biz.BizID,
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		Headers:        nonSensitiveHeaders(message.Headers),
		CreatedAt:      c.now(),
	}
	if record.IdempotencyKey == "" && event.TenantID != "" && event.SourceSystem != "" && event.EventType != "" && event.Biz.BizType != "" && event.Biz.BizID != "" {
		record.IdempotencyKey = event.IdempotencyKey()
	}
	if c.failures != nil {
		if err := c.failures.SaveFailure(ctx, record); err != nil {
			return Result{Action: action, Failure: &record}, err
		}
	}
	return Result{Action: action, Failure: &record, IdempotencyKey: record.IdempotencyKey}, nil
}

func invalidDecodeReason(err error) string {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "INVALID_JSON"
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return "INVALID_JSON"
	}
	return "INVALID_EVENT"
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("request body must contain a single json value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func headerValue(headers map[string]string, key string) string {
	for name, value := range headers {
		if strings.EqualFold(name, key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonSensitiveHeaders(headers map[string]string) map[string]string {
	out := map[string]string{}
	for name, value := range headers {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
