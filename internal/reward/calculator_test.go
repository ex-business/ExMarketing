package reward

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestConfigCalculatorFixedReward(t *testing.T) {
	result, err := ConfigCalculator{}.Calculate(baseContext(), rewardConfig(marketing.CalculateFixed, `{"amount":"18.88888","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`))
	if err != nil {
		t.Fatalf("calculate fixed: %v", err)
	}
	if result.Amount != "18.8889" || result.ReasonCode != ReasonOK {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Payload["source"] != string(marketing.CalculateFixed) || result.Payload["currency"] != "CNY" {
		t.Fatalf("unexpected payload: %#v", result.Payload)
	}
}

func TestConfigCalculatorFixedNonAmountRewardCreatesPayablePayload(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateFixed, `{
		"couponTemplateId":"WELCOME_10",
		"quantity":"2",
		"couponType":"CASH_OFF",
		"targetRef":"https://coupon.example.com/grant"
	}`)
	cfg.Type = marketing.RewardCoupon

	result, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate fixed coupon: %v", err)
	}
	if result.Skipped || result.Amount != "0.0000" || result.Type != marketing.RewardCoupon {
		t.Fatalf("expected payable non-amount reward with zero accounting amount, got %+v", result)
	}
	if result.Payload["quantity"] != 2 || result.Payload["couponTemplateId"] != "WELCOME_10" {
		t.Fatalf("unexpected non-amount payload: %#v", result.Payload)
	}
}

func TestConfigCalculatorRatioUsesAttributesBaseFieldAndCap(t *testing.T) {
	ctx := baseContext()
	ctx.Event.Attributes["amount"] = "1000.00"
	result, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.amount","rate":"0.20","maxAmount":"150.00","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`))
	if err != nil {
		t.Fatalf("calculate ratio: %v", err)
	}
	if result.Amount != "150.0000" {
		t.Fatalf("expected capped amount 150.0000, got %s", result.Amount)
	}
	if result.Payload["capped"] != true {
		t.Fatalf("expected capped payload, got %#v", result.Payload)
	}
}

func TestConfigCalculatorRatioAvoidsFloatDriftAndAcceptsJSONNumbers(t *testing.T) {
	var event marketing.ActivityEvent
	if err := json.Unmarshal([]byte(`{
		"eventId":"evt_decimal",
		"eventType":"DEPOSIT_SUCCESS",
		"sourceSystem":"payment",
		"tenantId":"tenant_1",
		"subject":{"type":"USER","id":"user_1"},
		"biz":{"bizType":"DEPOSIT","bizId":"pay_1"},
		"occurredAt":"2026-05-24T12:00:00Z",
		"attributes":{"amount":299.00}
	}`), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	ctx := baseContext()
	ctx.Event = event
	result, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.amount","rate":0.20,"currency":"CNY","targetRef":"https://wallet.example.com/reward"}`))
	if err != nil {
		t.Fatalf("calculate ratio: %v", err)
	}
	if result.Amount != "59.8000" || result.Payload["rate"] != "0.2000" || result.Payload["baseAmount"] != "299.0000" {
		t.Fatalf("unexpected decimal ratio result: %+v payload=%#v", result, result.Payload)
	}
}

func TestConfigCalculatorRebateRatioIsStable(t *testing.T) {
	ctx := baseContext()
	ctx.Event.Attributes["rebateBaseAmount"] = "333.33"
	first, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.rebateBaseAmount","rate":"0.015","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`))
	if err != nil {
		t.Fatalf("calculate rebate first: %v", err)
	}
	second, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.rebateBaseAmount","rate":"0.015","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`))
	if err != nil {
		t.Fatalf("calculate rebate second: %v", err)
	}
	if first.Amount != "5.0000" || second.Amount != first.Amount {
		t.Fatalf("expected stable rebate amount 5.0000, first=%+v second=%+v", first, second)
	}
}

func TestConfigCalculatorRejectsNonIntegerQuantityWithoutRounding(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateFixed, `{"couponTemplateId":"WELCOME_10","quantity":"1.00001","targetRef":"https://coupon.example.com/grant"}`)
	cfg.Type = marketing.RewardCoupon

	_, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	var rewardErr Error
	if !errors.As(err, &rewardErr) || rewardErr.ReasonCode != ReasonInvalidRewardConfig {
		t.Fatalf("expected invalid integer quantity error, got %T %[1]v", err)
	}
}

func TestConfigCalculatorLadderMatchesClosedInterval(t *testing.T) {
	tests := []struct {
		continueDays string
		wantAmount   string
		wantIndex    int
	}{
		{continueDays: "1", wantAmount: "5.0000", wantIndex: 1},
		{continueDays: "3", wantAmount: "18.0000", wantIndex: 2},
		{continueDays: "7", wantAmount: "58.0000", wantIndex: 0},
	}
	for _, tt := range tests {
		t.Run(tt.continueDays, func(t *testing.T) {
			ctx := baseContext()
			ctx.Event.Attributes["continueDays"] = tt.continueDays
			result, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculateLadder, `{
				"field":"attributes.continueDays",
				"ladders":[
					{"min":"7","max":"999","amount":"58.00"},
					{"min":"1","max":"2","amount":"5.00"},
					{"min":"3","max":"6","amount":"18.00"}
				],
				"currency":"CNY",
				"targetRef":"https://wallet.example.com/reward"
			}`))
			if err != nil {
				t.Fatalf("calculate ladder: %v", err)
			}
			if result.Amount != tt.wantAmount || result.Payload["ladderIndex"] != tt.wantIndex {
				t.Fatalf("unexpected ladder result: %+v payload=%#v", result, result.Payload)
			}
		})
	}
}

func TestConfigCalculatorRandomIsStableForSameSeed(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateRandom, `{"minAmount":"1.00","maxAmount":"2.00","step":"0.01","currency":"CNY","seedSalt":"salt-a","targetRef":"https://wallet.example.com/reward"}`)
	first, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate random first: %v", err)
	}
	second, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate random second: %v", err)
	}
	if first.Amount != second.Amount || first.Payload["selectedIndex"] != second.Payload["selectedIndex"] {
		t.Fatalf("expected stable random result, first=%+v second=%+v", first, second)
	}
}

func TestConfigCalculatorRandomStepLargerThanRangeUsesMinAmount(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateRandom, `{"minAmount":"1.00","maxAmount":"2.00","step":"5.00","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`)
	result, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate random: %v", err)
	}
	if result.Amount != "1.0000" || result.Payload["candidateCount"] != int64(1) {
		t.Fatalf("expected only minAmount candidate, got %+v payload=%#v", result, result.Payload)
	}
}

func TestConfigCalculatorProbabilityZeroItemSkipsReward(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateProbability, `{
		"items":[
			{"itemCode":"NO_PRIZE","weight":"100","rewardType":"BONUS","amount":"0","label":"No prize"}
		],
		"currency":"CNY",
		"targetRef":"https://wallet.example.com/reward"
	}`)
	result, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate probability: %v", err)
	}
	if !result.Skipped || result.ReasonCode != ReasonZeroRewardSkipped || result.Amount != "0.0000" {
		t.Fatalf("expected zero reward skip, got %+v", result)
	}
	if result.Payload["itemCode"] != "NO_PRIZE" {
		t.Fatalf("expected selected item payload, got %#v", result.Payload)
	}
}

func TestConfigCalculatorProbabilitySupportsMixedPrizeItems(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateProbability, `{
		"items":[
			{"itemCode":"COUPON_5","weight":"100","rewardType":"COUPON","quantity":"1","couponTemplateId":"tpl_5","faceValue":"5","currency":"USDT","label":"5 USDT coupon"},
			{"itemCode":"POINT_100","weight":"0","rewardType":"POINT","points":"100","unit":"LOYALTY_POINT","label":"100 points"}
		],
		"targetRef":"https://reward.example.com/grant"
	}`)
	result, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	if err != nil {
		t.Fatalf("calculate mixed probability: %v", err)
	}
	if result.Type != marketing.RewardCoupon || result.Amount != "0.0000" || result.Payload["couponTemplateId"] != "tpl_5" {
		t.Fatalf("expected coupon prize selected, got %+v payload=%#v", result, result.Payload)
	}
	if result.Payload["quantity"] != 1 {
		t.Fatalf("expected coupon quantity payload, got %#v", result.Payload)
	}
}

func TestConfigCalculatorProbabilityRejectsTotalWeightNot100(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateProbability, `{
		"items":[
			{"itemCode":"A","weight":"50","amount":"1.00"},
			{"itemCode":"B","weight":"49.9999","amount":"2.00"}
		],
		"currency":"CNY",
		"targetRef":"https://wallet.example.com/reward"
	}`)
	_, err := ConfigCalculator{}.Calculate(baseContext(), cfg)
	var rewardErr Error
	if !errors.As(err, &rewardErr) || rewardErr.ReasonCode != ReasonProbabilityTotalInvalid {
		t.Fatalf("expected probability total error, got %T %[1]v", err)
	}
}

func TestConfigCalculatorPoolSplitCountFieldAlias(t *testing.T) {
	ctx := baseContext()
	ctx.Event.Attributes["poolParticipantCount"] = "4"
	result, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculatePoolSplit, `{
		"poolAmount":"1000.00",
		"splitBy":"COUNT",
		"countField":"attributes.poolParticipantCount",
		"currency":"CNY",
		"targetRef":"https://wallet.example.com/reward"
	}`))
	if err != nil {
		t.Fatalf("calculate pool split count: %v", err)
	}
	if result.Amount != "250.0000" || result.Payload["participantCount"] != "4.0000" {
		t.Fatalf("expected count split 250.0000, got %+v payload=%#v", result, result.Payload)
	}
}

func TestConfigCalculatorPoolSplitWeight(t *testing.T) {
	ctx := baseContext()
	ctx.Event.Attributes["userWeight"] = "2"
	ctx.Event.Attributes["totalWeight"] = "8"
	result, err := ConfigCalculator{}.Calculate(ctx, rewardConfig(marketing.CalculatePoolSplit, `{
		"poolAmount":"1000.00",
		"splitBy":"WEIGHT",
		"weightField":"attributes.userWeight",
		"totalWeightField":"attributes.totalWeight",
		"currency":"CNY",
		"targetRef":"https://wallet.example.com/reward"
	}`))
	if err != nil {
		t.Fatalf("calculate pool split: %v", err)
	}
	if result.Amount != "250.0000" {
		t.Fatalf("expected 250.0000, got %s", result.Amount)
	}
}

func TestConfigCalculatorReturnsExplicitErrors(t *testing.T) {
	tests := []struct {
		name       string
		reward     marketing.Reward
		ctx        Context
		reasonCode string
	}{
		{
			name:       "empty config",
			reward:     rewardConfig(marketing.CalculateFixed, `{}`),
			ctx:        baseContext(),
			reasonCode: ReasonInvalidRewardConfig,
		},
		{
			name:       "missing field",
			reward:     rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.missing","rate":"0.2","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			ctx:        baseContext(),
			reasonCode: ReasonRatioBaseMissing,
		},
		{
			name:       "type mismatch",
			reward:     rewardConfig(marketing.CalculateRatio, `{"baseField":"attributes.amount","rate":"0.2","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			ctx:        contextWithAmount(true),
			reasonCode: ReasonFieldTypeMismatch,
		},
		{
			name:       "ladder not matched",
			reward:     rewardConfig(marketing.CalculateLadder, `{"field":"attributes.amount","ladders":[{"min":"1","max":"2","amount":"5"}],"currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			ctx:        contextWithAmount("10"),
			reasonCode: ReasonLadderNotMatched,
		},
		{
			name:       "unsupported calculate type",
			reward:     rewardConfig(marketing.CalculateType("UNKNOWN"), `{"amount":"1","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			ctx:        baseContext(),
			reasonCode: ReasonUnsupportedCalculateType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConfigCalculator{}.Calculate(tt.ctx, tt.reward)
			var rewardErr Error
			if !errors.As(err, &rewardErr) {
				t.Fatalf("expected reward error, got %v", err)
			}
			if rewardErr.ReasonCode != tt.reasonCode {
				t.Fatalf("expected reason %s, got %s (%v)", tt.reasonCode, rewardErr.ReasonCode, err)
			}
		})
	}
}

func baseContext() Context {
	return contextWithAmount("100.00")
}

func contextWithAmount(amount any) Context {
	return Context{
		Event: marketing.ActivityEvent{
			EventID:  "evt_1",
			TenantID: "tenant_1",
			Subject:  marketing.EventSubject{Type: "USER", ID: "user_1"},
			Biz:      marketing.EventBiz{BizType: "ORDER", BizID: "order_1"},
			Attributes: map[string]any{
				"amount": amount,
			},
		},
		TenantID:       "tenant_1",
		ActivityID:     "act_1",
		IdempotencyKey: "idem_1",
	}
}

func rewardConfig(calculateType marketing.CalculateType, config string) marketing.Reward {
	return marketing.Reward{
		Code:          "reward_1",
		Type:          marketing.RewardBonus,
		CalculateType: calculateType,
		Config:        []byte(config),
		GrantMode:     marketing.GrantAuto,
		GrantChannel:  marketing.GrantHTTP,
		Enabled:       true,
		Version:       1,
	}
}
