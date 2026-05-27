package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestFirstDepositPluginBuildsContextFromStandardEvent(t *testing.T) {
	event := firstDepositPluginEvent(map[string]any{
		"amount":         json.Number("299.00"),
		"currency":       "CNY",
		"isFirstDeposit": true,
		"paymentMethod":  "BANK_CARD",
		"depositNo":      "pay_001",
	})

	result, err := FirstDepositPlugin{}.BuildContext(context.Background(), BuildContextRequest{Event: event})
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !result.Precheck.Allowed || result.Precheck.ReasonCode != "OK" {
		t.Fatalf("unexpected precheck: %+v", result.Precheck)
	}
	if result.Context["amount"] != "299.00" ||
		result.Context["currency"] != "CNY" ||
		result.Context["isFirstDeposit"] != true ||
		result.Context["paymentMethod"] != "BANK_CARD" ||
		result.Context["depositBizId"] != "pay_001" {
		t.Fatalf("unexpected context: %+v", result.Context)
	}

	intents, err := FirstDepositPlugin{}.AfterQualified(context.Background(), AfterQualifiedRequest{})
	if err != nil {
		t.Fatalf("after qualified: %v", err)
	}
	if len(intents) != 0 {
		t.Fatalf("first deposit plugin must not emit state intents: %+v", intents)
	}
}

func TestFirstDepositPluginPrecheckRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		event      marketing.ActivityEvent
		reasonCode string
	}{
		{
			name:       "invalid event type",
			event:      firstDepositPluginEventWithType("ORDER_PAID", "DEPOSIT", map[string]any{"amount": "299.00", "currency": "CNY", "isFirstDeposit": true}),
			reasonCode: ReasonFirstDepositEventTypeInvalid,
		},
		{
			name:       "invalid biz type",
			event:      firstDepositPluginEventWithType("DEPOSIT_SUCCESS", "ORDER", map[string]any{"amount": "299.00", "currency": "CNY", "isFirstDeposit": true}),
			reasonCode: ReasonFirstDepositBizTypeInvalid,
		},
		{
			name:       "missing amount",
			event:      firstDepositPluginEvent(map[string]any{"currency": "CNY", "isFirstDeposit": true}),
			reasonCode: ReasonFirstDepositAmountMissing,
		},
		{
			name:       "non-positive amount",
			event:      firstDepositPluginEvent(map[string]any{"amount": "0", "currency": "CNY", "isFirstDeposit": true}),
			reasonCode: ReasonFirstDepositAmountInvalid,
		},
		{
			name:       "missing currency",
			event:      firstDepositPluginEvent(map[string]any{"amount": "299.00", "isFirstDeposit": true}),
			reasonCode: ReasonFirstDepositCurrencyMissing,
		},
		{
			name:       "missing first deposit flag",
			event:      firstDepositPluginEvent(map[string]any{"amount": "299.00", "currency": "CNY"}),
			reasonCode: ReasonFirstDepositFlagMissing,
		},
		{
			name:       "invalid first deposit flag",
			event:      firstDepositPluginEvent(map[string]any{"amount": "299.00", "currency": "CNY", "isFirstDeposit": "true"}),
			reasonCode: ReasonFirstDepositFlagInvalid,
		},
		{
			name:       "not first deposit",
			event:      firstDepositPluginEvent(map[string]any{"amount": "299.00", "currency": "CNY", "isFirstDeposit": false}),
			reasonCode: ReasonFirstDepositNotFirst,
		},
		{
			name:       "deposit id conflict",
			event:      firstDepositPluginEvent(map[string]any{"amount": "299.00", "currency": "CNY", "isFirstDeposit": true, "depositNo": "pay_other"}),
			reasonCode: ReasonFirstDepositBizIDConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FirstDepositPlugin{}.BuildContext(context.Background(), BuildContextRequest{Event: tt.event})
			if err != nil {
				t.Fatalf("build context: %v", err)
			}
			if result.Precheck.Allowed || result.Precheck.ReasonCode != tt.reasonCode {
				t.Fatalf("precheck=%+v, want rejected %s", result.Precheck, tt.reasonCode)
			}
		})
	}
}

func firstDepositPluginEvent(attributes map[string]any) marketing.ActivityEvent {
	return firstDepositPluginEventWithType("DEPOSIT_SUCCESS", "DEPOSIT", attributes)
}

func firstDepositPluginEventWithType(eventType string, bizType string, attributes map[string]any) marketing.ActivityEvent {
	return marketing.ActivityEvent{
		EventID:      "evt_001",
		EventType:    eventType,
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "user_001"},
		Biz:          marketing.EventBiz{BizType: bizType, BizID: "pay_001"},
		OccurredAt:   time.Now().UTC(),
		Attributes:   attributes,
	}
}
