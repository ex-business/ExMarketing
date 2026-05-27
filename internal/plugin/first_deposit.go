package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

const (
	FirstDepositPluginCode = "FIRST_DEPOSIT"

	ReasonFirstDepositEventTypeInvalid = "FIRST_DEPOSIT_EVENT_TYPE_INVALID"
	ReasonFirstDepositBizTypeInvalid   = "FIRST_DEPOSIT_BIZ_TYPE_INVALID"
	ReasonFirstDepositAmountMissing    = "FIRST_DEPOSIT_AMOUNT_MISSING"
	ReasonFirstDepositAmountInvalid    = "FIRST_DEPOSIT_AMOUNT_INVALID"
	ReasonFirstDepositCurrencyMissing  = "FIRST_DEPOSIT_CURRENCY_MISSING"
	ReasonFirstDepositFlagMissing      = "FIRST_DEPOSIT_FLAG_MISSING"
	ReasonFirstDepositFlagInvalid      = "FIRST_DEPOSIT_FLAG_INVALID"
	ReasonFirstDepositNotFirst         = "FIRST_DEPOSIT_NOT_FIRST"
	ReasonFirstDepositBizIDConflict    = "FIRST_DEPOSIT_BIZ_ID_CONFLICT"
)

type FirstDepositPlugin struct{}

func (FirstDepositPlugin) Code() string {
	return FirstDepositPluginCode
}

func (FirstDepositPlugin) CompatibleActivityTypes() []string {
	return []string{"FIRST_DEPOSIT"}
}

func (FirstDepositPlugin) ValidateConfig(json.RawMessage) error {
	return nil
}

func (FirstDepositPlugin) BuildContext(_ context.Context, req BuildContextRequest) (ContextResult, error) {
	event := req.Event
	pluginContext := map[string]any{
		"depositBizId": event.Biz.BizID,
	}
	explain := map[string]any{
		"source":       "event.attributes",
		"eventType":    event.EventType,
		"bizType":      event.Biz.BizType,
		"depositBizId": event.Biz.BizID,
	}

	if event.EventType != "DEPOSIT_SUCCESS" {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositEventTypeInvalid, "eventType must be DEPOSIT_SUCCESS"), nil
	}
	if event.Biz.BizType != "DEPOSIT" {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositBizTypeInvalid, "biz.bizType must be DEPOSIT"), nil
	}

	if depositNo := stringAttribute(event.Attributes, "depositNo"); depositNo != "" && event.Biz.BizID != "" && depositNo != event.Biz.BizID {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositBizIDConflict, "attributes.depositNo conflicts with biz.bizId"), nil
	}

	amountText, ok := decimalAttributeText(event.Attributes, "amount")
	if !ok {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositAmountMissing, "attributes.amount is required"), nil
	}
	amount, err := decimal.Parse(amountText)
	if err != nil || !amount.IsPositive() {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositAmountInvalid, "attributes.amount must be positive decimal"), nil
	}
	pluginContext["amount"] = amountText

	currency := stringAttribute(event.Attributes, "currency")
	if currency == "" {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositCurrencyMissing, "attributes.currency is required"), nil
	}
	pluginContext["currency"] = currency

	isFirstDeposit, ok, valid := boolAttribute(event.Attributes, "isFirstDeposit")
	if !ok {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositFlagMissing, "attributes.isFirstDeposit is required"), nil
	}
	if !valid {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositFlagInvalid, "attributes.isFirstDeposit must be boolean"), nil
	}
	pluginContext["isFirstDeposit"] = isFirstDeposit
	if !isFirstDeposit {
		return firstDepositRejected(pluginContext, explain, ReasonFirstDepositNotFirst, "attributes.isFirstDeposit is false"), nil
	}

	if paymentMethod := stringAttribute(event.Attributes, "paymentMethod"); paymentMethod != "" {
		pluginContext["paymentMethod"] = paymentMethod
	}

	return ContextResult{
		Context: pluginContext,
		Precheck: PrecheckResult{
			Allowed:    true,
			ReasonCode: "OK",
			Reason:     "first deposit precheck passed",
		},
		Explain: explain,
	}, nil
}

func (FirstDepositPlugin) AfterQualified(context.Context, AfterQualifiedRequest) ([]StateChangeIntent, error) {
	return nil, nil
}

func firstDepositRejected(context map[string]any, explain map[string]any, reasonCode string, reason string) ContextResult {
	return ContextResult{
		Context: context,
		Precheck: PrecheckResult{
			Allowed:    false,
			ReasonCode: reasonCode,
			Reason:     reason,
		},
		Explain: explain,
	}
}

func stringAttribute(attributes map[string]any, key string) string {
	value, ok := attributes[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func boolAttribute(attributes map[string]any, key string) (bool, bool, bool) {
	value, ok := attributes[key]
	if !ok {
		return false, false, false
	}
	typed, valid := value.(bool)
	return typed, true, valid
}

func decimalAttributeText(attributes map[string]any, key string) (string, bool) {
	value, ok := attributes[key]
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		return text, text != ""
	case json.Number:
		return typed.String(), typed.String() != ""
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	default:
		return fmt.Sprint(value), true
	}
}

var _ ActivityPlugin = FirstDepositPlugin{}
