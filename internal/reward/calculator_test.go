package reward

import (
	"strings"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestConfigCalculatorRatioUsesAttributesBaseField(t *testing.T) {
	result, err := ConfigCalculator{}.Calculate(Context{
		Event: marketing.ActivityEvent{Attributes: map[string]any{"amount": "100.00"}},
	}, marketing.Reward{
		Type:          marketing.RewardBonus,
		CalculateType: marketing.CalculateRatio,
		Config:        []byte(`{"baseField":"attributes.amount","rate":"0.20"}`),
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("calculate ratio: %v", err)
	}
	if result.Amount != "20.0000" {
		t.Fatalf("expected amount 20.0000, got %s", result.Amount)
	}
}

func TestConfigCalculatorRejectsNonAttributesBaseField(t *testing.T) {
	_, err := ConfigCalculator{}.Calculate(Context{
		Event: marketing.ActivityEvent{Attributes: map[string]any{"amount": "100.00"}},
	}, marketing.Reward{
		Type:          marketing.RewardBonus,
		CalculateType: marketing.CalculateRatio,
		Config:        []byte(`{"baseField":"biz.amount","rate":"0.20"}`),
		Enabled:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "baseField not found") {
		t.Fatalf("expected baseField error, got %v", err)
	}
}
