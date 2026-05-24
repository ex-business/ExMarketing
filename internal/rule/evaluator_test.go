package rule

import (
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestConditionEvaluatorStandardEventFields(t *testing.T) {
	event := marketing.ActivityEvent{
		EventID:      "evt_1",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "10001"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
		OccurredAt:   time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
		Attributes:   map[string]any{"amount": "100.00", "vipLevel": 3},
	}

	rule := marketing.Rule{
		Type: marketing.RuleTypeCondition,
		Config: []byte(`{
			"logic": "AND",
			"conditions": [
				{"field": "eventId", "op": "=", "value": "evt_1"},
				{"field": "eventType", "op": "=", "value": "DEPOSIT_SUCCESS"},
				{"field": "sourceSystem", "op": "=", "value": "payment"},
				{"field": "tenantId", "op": "=", "value": "site_001"},
				{"field": "subject.type", "op": "=", "value": "USER"},
				{"field": "subject.id", "op": "=", "value": "10001"},
				{"field": "biz.bizType", "op": "=", "value": "DEPOSIT"},
				{"field": "biz.bizId", "op": "=", "value": "pay_1"},
				{"field": "occurredAt", "op": "=", "value": "2026-05-24T12:00:00Z"},
				{"field": "attributes.amount", "op": ">=", "value": "100.00"}
			]
		}`),
	}

	result := ConditionEvaluator{}.Evaluate(Context{Event: event}, rule)
	if !result.Passed {
		t.Fatalf("expected standard fields to pass, got reason %q", result.Reason)
	}
}

func TestConditionEvaluatorRejectsNonStandardTopLevelField(t *testing.T) {
	rule := marketing.Rule{
		Type: marketing.RuleTypeCondition,
		Config: []byte(`{
			"conditions": [
				{"field": "orderAmount", "op": "=", "value": "100.00"}
			]
		}`),
	}

	result := ConditionEvaluator{}.Evaluate(Context{Event: marketing.ActivityEvent{}}, rule)
	if result.Passed {
		t.Fatal("expected non-standard top-level field to fail")
	}
	if result.Reason != "field orderAmount not found" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}
}
