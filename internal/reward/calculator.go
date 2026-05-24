package reward

import (
	"encoding/json"
	"errors"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type Context struct {
	Event marketing.ActivityEvent
}

type Result struct {
	Type     marketing.RewardType `json:"type"`
	Amount   string               `json:"amount"`
	Currency string               `json:"currency"`
	Payload  map[string]any       `json:"payload"`
}

type Calculator interface {
	Calculate(ctx Context, reward marketing.Reward) (Result, error)
}

type ConfigCalculator struct{}

func (c ConfigCalculator) Calculate(ctx Context, reward marketing.Reward) (Result, error) {
	var config marketing.RewardConfig
	if err := json.Unmarshal(reward.Config, &config); err != nil {
		return Result{}, err
	}

	currency := config.Currency
	if currency == "" {
		currency = "CNY"
	}

	amount, err := calculateAmount(ctx, reward.CalculateType, config)
	if err != nil {
		return Result{}, err
	}

	payload := map[string]any{
		"type":         reward.Type,
		"amount":       amount.String(),
		"currency":     currency,
		"flowMultiple": firstNonEmpty(config.FlowMultiple, reward.FlowMultiple),
	}
	if config.ExpireAt != "" {
		payload["expireAt"] = config.ExpireAt
	}
	if config.RewardPayload != nil {
		payload["extra"] = config.RewardPayload
	}

	return Result{
		Type:     reward.Type,
		Amount:   amount.String(),
		Currency: currency,
		Payload:  payload,
	}, nil
}

func calculateAmount(ctx Context, calculateType marketing.CalculateType, config marketing.RewardConfig) (decimal.Decimal, error) {
	switch calculateType {
	case marketing.CalculateFixed:
		return decimal.Parse(config.Amount)
	case marketing.CalculateRatio:
		baseValue, ok := eventField(ctx.Event, config.BaseField)
		if !ok {
			return decimal.Decimal{}, errors.New("baseField not found")
		}
		base, err := decimal.Parse(toString(baseValue))
		if err != nil {
			return decimal.Decimal{}, err
		}
		rate, err := decimal.Parse(config.Rate)
		if err != nil {
			return decimal.Decimal{}, err
		}
		amount := base.Mul(rate)
		if config.MaxAmount != "" {
			maxAmount, err := decimal.Parse(config.MaxAmount)
			if err != nil {
				return decimal.Decimal{}, err
			}
			amount = amount.Min(maxAmount)
		}
		return amount, nil
	default:
		return decimal.Decimal{}, errors.New("unsupported calculateType")
	}
}

func eventField(event marketing.ActivityEvent, path string) (any, bool) {
	if path == "attributes.amount" {
		value, ok := event.Attributes["amount"]
		return value, ok
	}
	if path == "" {
		return nil, false
	}

	const prefix = "attributes."
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return nil, false
	}
	value, ok := event.Attributes[path[len(prefix):]]
	return value, ok
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return jsonNumberString(typed)
	}
}

func jsonNumberString(value any) string {
	bytes, _ := json.Marshal(value)
	return string(bytes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "0"
}
