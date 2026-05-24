package reward

import (
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

func TestConfigCalculatorLadderMatchesClosedInterval(t *testing.T) {
	ctx := baseContext()
	ctx.Event.Attributes["continueDays"] = "7"
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
	if result.Amount != "58.0000" || result.Payload["ladderIndex"] != 0 {
		t.Fatalf("unexpected ladder result: %+v payload=%#v", result, result.Payload)
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

func TestConfigCalculatorProbabilityZeroItemSkipsReward(t *testing.T) {
	cfg := rewardConfig(marketing.CalculateProbability, `{
		"items":[
			{"itemCode":"NO_PRIZE","weight":"1","rewardType":"BONUS","amount":"0","label":"No prize"}
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
