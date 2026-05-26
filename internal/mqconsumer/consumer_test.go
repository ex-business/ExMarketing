package mqconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestConsumerProcessesEventAndAcksDuplicateWithoutNewGrants(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	service := engine.NewService(repo)
	activity := createTestActivity(t, service)
	store := NewMemoryFailureStore()
	consumer := New(service, Config{
		FailureStore: store,
		Allowlist:    NewSourceAllowlist(map[string][]string{"site_001": {"payment"}}),
	})

	body := eventJSON("evt_mq_1", "pay_mq_1", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	key := "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_mq_1"
	result, err := consumer.Process(context.Background(), Message{
		Broker:  BrokerFake,
		Topic:   "marketing.events",
		Body:    []byte(body),
		Headers: map[string]string{"Idempotency-Key": key, "Authorization": "Bearer secret"},
		Metadata: MessageMetadata{
			MessageID: "msg-1",
			TraceID:   "trace-1",
		},
	})
	if err != nil {
		t.Fatalf("process event: %v", err)
	}
	if result.Action != ActionAck || result.DerivedIdempotencyKey {
		t.Fatalf("expected ack with explicit key, got action=%s derived=%v", result.Action, result.DerivedIdempotencyKey)
	}
	if len(result.EventResult.Participations) != 1 || len(result.EventResult.GrantOrders) != 1 || len(result.EventResult.OutboxMessages) != 1 {
		t.Fatalf("expected participation/grant/outbox, got p=%d g=%d o=%d", len(result.EventResult.Participations), len(result.EventResult.GrantOrders), len(result.EventResult.OutboxMessages))
	}
	expectedParticipationKey := "site_001:" + activity.ID + ":" + key
	if result.EventResult.Participations[0].IdempotencyKey != expectedParticipationKey {
		t.Fatalf("unexpected participation idempotency key: %s", result.EventResult.Participations[0].IdempotencyKey)
	}

	duplicate, err := consumer.Process(context.Background(), Message{
		Broker: BrokerFake,
		Topic:  "marketing.events",
		Body: []byte(`{
			"tenantId":"site_001",
			"sourceSystem":"payment",
			"eventType":"DEPOSIT_SUCCESS",
			"eventId":"evt_mq_1",
			"subject":{"id":"10001","type":"USER"},
			"biz":{"bizId":"pay_mq_1","bizType":"DEPOSIT"},
			"occurredAt":"2026-05-24T12:00:00Z",
			"attributes":{"isFirstDeposit":true,"vipLevel":3,"amount":"299.00"}
		}`),
		Headers: map[string]string{"idempotency-key": key},
	})
	if err != nil {
		t.Fatalf("process duplicate: %v", err)
	}
	if duplicate.Action != ActionAck || !duplicate.EventResult.Duplicate {
		t.Fatalf("expected duplicate ack, got action=%s duplicate=%v", duplicate.Action, duplicate.EventResult.Duplicate)
	}
	if len(duplicate.EventResult.GrantOrders) != 0 || len(duplicate.EventResult.OutboxMessages) != 0 {
		t.Fatalf("duplicate should not create new grants/outbox: %+v", duplicate.EventResult)
	}
	if failures := store.ListFailures(); len(failures) != 0 {
		t.Fatalf("unexpected failures: %+v", failures)
	}
}

func TestConsumerAllowsDerivedIdempotencyKeyAndRecordsSourceAudit(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	service := engine.NewService(repo)
	createTestActivity(t, service)
	consumer := New(service, Config{})

	result, err := consumer.Process(context.Background(), Message{
		Broker: BrokerFake,
		Topic:  "marketing.events",
		Body:   []byte(eventJSON("evt_mq_derived", "pay_mq_derived", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)),
	})
	if err != nil {
		t.Fatalf("process derived: %v", err)
	}
	if result.Action != ActionAck || !result.DerivedIdempotencyKey {
		t.Fatalf("expected derived key ack, got action=%s derived=%v", result.Action, result.DerivedIdempotencyKey)
	}

	audit, err := repo.ListAudit(context.Background(), marketing.AuditQuery{TenantID: "site_001", Action: "EVENT_IN"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audit) == 0 || audit[0].Actor != "mq" {
		t.Fatalf("expected mq audit actor, got %+v", audit)
	}
}

func TestConsumerRejectsInvalidMessagesToDLQ(t *testing.T) {
	service := engine.NewService(marketing.NewMemoryRepository())
	store := NewMemoryFailureStore()
	consumer := New(service, Config{
		FailureStore: store,
		Allowlist:    NewSourceAllowlist(map[string][]string{"site_001": {"payment"}}),
	})
	validBody := eventJSON("evt_mq_invalid", "pay_mq_invalid", `"amount":"299.00"`)

	tests := []struct {
		name       string
		body       string
		headers    map[string]string
		reasonCode string
	}{
		{name: "bad json", body: `{`, reasonCode: "INVALID_JSON"},
		{name: "missing field", body: strings.Replace(validBody, `"eventId": "evt_mq_invalid",`, "", 1), reasonCode: "INVALID_EVENT"},
		{name: "unknown field", body: strings.Replace(validBody, `"attributes": {`, `"unexpected": true, "attributes": {`, 1), reasonCode: "INVALID_EVENT"},
		{name: "mismatched key", body: validBody, headers: map[string]string{"Idempotency-Key": "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:other"}, reasonCode: "INVALID_IDEMPOTENCY_KEY"},
		{name: "forbidden source", body: strings.Replace(validBody, `"sourceSystem": "payment"`, `"sourceSystem": "order"`, 1), reasonCode: "SOURCE_FORBIDDEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := consumer.Process(context.Background(), Message{
				Broker:  BrokerKafka,
				Topic:   "marketing.events",
				Body:    []byte(tt.body),
				Headers: tt.headers,
				Metadata: MessageMetadata{
					MessageID: "msg-" + tt.name,
				},
			})
			if err != nil {
				t.Fatalf("unexpected process error: %v", err)
			}
			if result.Action != ActionDLQ || result.Failure == nil {
				t.Fatalf("expected dlq failure, got action=%s failure=%+v", result.Action, result.Failure)
			}
			if result.Failure.Retryable || result.Failure.ReasonCode != tt.reasonCode {
				t.Fatalf("unexpected failure: %+v", result.Failure)
			}
		})
	}
	failures := store.ListFailures()
	if len(failures) != len(tests) {
		t.Fatalf("expected %d failures, got %d", len(tests), len(failures))
	}
	for _, failure := range failures {
		if _, ok := failure.Headers["Authorization"]; ok {
			t.Fatalf("failure leaked authorization header: %+v", failure)
		}
	}
}

func TestConsumerClassifiesEngineSystemErrorAsRetryable(t *testing.T) {
	service := engine.NewService(failingRepository{Repository: marketing.NewMemoryRepository(), err: errors.New("database unavailable")})
	store := NewMemoryFailureStore()
	consumer := New(service, Config{FailureStore: store})

	result, err := consumer.Process(context.Background(), Message{
		Broker: BrokerRabbitMQ,
		Topic:  "marketing.events",
		Body:   []byte(eventJSON("evt_mq_retry", "pay_mq_retry", `"amount":"299.00"`)),
	})
	if err == nil {
		t.Fatalf("expected retryable engine error")
	}
	if result.Action != ActionNackRetry || result.Failure == nil || !result.Failure.Retryable {
		t.Fatalf("expected retryable failure, got action=%s failure=%+v", result.Action, result.Failure)
	}
	if result.Failure.ReasonCode != "RETRYABLE_PROCESSING_ERROR" {
		t.Fatalf("unexpected reason: %+v", result.Failure)
	}
}

func createTestActivity(t *testing.T, service *engine.Service) marketing.Activity {
	t.Helper()
	eventTime := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	activity, err := service.CreateActivity(context.Background(), marketing.CreateActivityCommand{
		TenantID:      "site_001",
		Code:          "FIRST_DEPOSIT_BONUS",
		Name:          "First Deposit Bonus",
		Type:          "FIRST_DEPOSIT",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     eventTime.Add(-24 * time.Hour),
		EndTime:       eventTime.Add(96 * time.Hour),
		Timezone:      "UTC",
		Priority:      100,
		Version:       1,
		Config:        json.RawMessage(`{"metadata":{"owner":"growth"}}`),
		Rules: []marketing.Rule{{
			Code:    "FIRST_DEPOSIT_AND_AMOUNT",
			Phase:   marketing.RulePhaseQualify,
			Type:    marketing.RuleTypeCondition,
			Config:  json.RawMessage(`{"logic":"AND","conditions":[{"field":"attributes.isFirstDeposit","op":"=","value":true},{"field":"attributes.amount","op":">=","value":"100.00"}]}`),
			Sort:    1,
			Enabled: true,
			Version: 1,
		}},
		Rewards: []marketing.Reward{{
			Code:          "FIRST_DEPOSIT_RATIO",
			Type:          marketing.RewardBonus,
			CalculateType: marketing.CalculateRatio,
			Config:        json.RawMessage(`{"baseField":"attributes.amount","rate":"0.20","maxAmount":"500.00","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			GrantMode:     marketing.GrantAuto,
			GrantChannel:  marketing.GrantHTTP,
			Enabled:       true,
			Version:       1,
		}},
	})
	if err != nil {
		t.Fatalf("create activity: %v", err)
	}
	return activity
}

func eventJSON(eventID string, bizID string, attributesJSON string) string {
	attributesJSON = strings.TrimSpace(attributesJSON)
	if !strings.HasPrefix(attributesJSON, "{") {
		attributesJSON = "{" + attributesJSON + "}"
	}
	return `{
		"eventId": "` + eventID + `",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "` + bizID + `"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": ` + attributesJSON + `
	}`
}

type failingRepository struct {
	marketing.Repository
	err error
}

func (r failingRepository) CreateInbox(ctx context.Context, inbox marketing.EventInbox) (marketing.EventInbox, bool, error) {
	return marketing.EventInbox{}, false, r.err
}
