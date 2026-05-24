package rule

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type Context struct {
	Event marketing.ActivityEvent
}

type Result struct {
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

type Evaluator interface {
	Evaluate(ctx Context, rule marketing.Rule) Result
}

type ConditionEvaluator struct{}

func (e ConditionEvaluator) Evaluate(ctx Context, rule marketing.Rule) Result {
	var config marketing.ConditionRuleConfig
	if err := json.Unmarshal(rule.Config, &config); err != nil {
		return Result{Passed: false, Reason: "invalid rule config"}
	}

	logic := strings.ToUpper(strings.TrimSpace(config.Logic))
	if logic == "" {
		logic = "AND"
	}
	if len(config.Conditions) == 0 {
		return Result{Passed: true}
	}

	anyPassed := false
	for _, condition := range config.Conditions {
		ok, reason := evaluateCondition(ctx, condition)
		if logic == "AND" && !ok {
			return Result{Passed: false, Reason: reason}
		}
		if logic == "OR" && ok {
			return Result{Passed: true}
		}
		anyPassed = anyPassed || ok
	}

	if logic == "OR" {
		return Result{Passed: anyPassed, Reason: "no OR condition passed"}
	}
	return Result{Passed: true}
}

func evaluateCondition(ctx Context, condition marketing.Condition) (bool, string) {
	actual, ok := fieldValue(ctx.Event, condition.Field)
	if !ok {
		return false, fmt.Sprintf("field %s not found", condition.Field)
	}

	op := strings.ToUpper(strings.TrimSpace(condition.Op))
	switch op {
	case "=", "==":
		return valuesEqual(actual, condition.Value), fmt.Sprintf("%s != %v", condition.Field, condition.Value)
	case "!=", "<>":
		return !valuesEqual(actual, condition.Value), fmt.Sprintf("%s == %v", condition.Field, condition.Value)
	case ">=", ">", "<=", "<":
		return compareDecimal(actual, condition.Value, op)
	case "IN":
		return contains(condition.Value, actual), fmt.Sprintf("%s not in %v", condition.Field, condition.Value)
	case "NOT_IN":
		return !contains(condition.Value, actual), fmt.Sprintf("%s in %v", condition.Field, condition.Value)
	default:
		return false, fmt.Sprintf("unsupported op %s", condition.Op)
	}
}

func fieldValue(event marketing.ActivityEvent, path string) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	var current any
	switch parts[0] {
	case "eventId":
		current = event.EventID
	case "eventType":
		current = event.EventType
	case "sourceSystem":
		current = event.SourceSystem
	case "tenantId":
		current = event.TenantID
	case "occurredAt":
		current = event.OccurredAt.Format("2006-01-02T15:04:05Z07:00")
	case "subject":
		current = map[string]any{"type": event.Subject.Type, "id": event.Subject.ID}
	case "biz":
		current = map[string]any{"bizType": event.Biz.BizType, "bizId": event.Biz.BizID}
	case "attributes":
		current = event.Attributes
	default:
		return nil, false
	}

	for _, part := range parts[1:] {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func valuesEqual(a, b any) bool {
	if ad, aok := toDecimal(a); aok {
		if bd, bok := toDecimal(b); bok {
			return ad.Cmp(bd) == 0
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b) || reflect.DeepEqual(a, b)
}

func compareDecimal(a, b any, op string) (bool, string) {
	ad, ok := toDecimal(a)
	if !ok {
		return false, "actual value is not decimal"
	}
	bd, ok := toDecimal(b)
	if !ok {
		return false, "expected value is not decimal"
	}
	cmp := ad.Cmp(bd)
	switch op {
	case ">=":
		return cmp >= 0, fmt.Sprintf("%s < %s", ad.String(), bd.String())
	case ">":
		return cmp > 0, fmt.Sprintf("%s <= %s", ad.String(), bd.String())
	case "<=":
		return cmp <= 0, fmt.Sprintf("%s > %s", ad.String(), bd.String())
	case "<":
		return cmp < 0, fmt.Sprintf("%s >= %s", ad.String(), bd.String())
	default:
		return false, "unsupported decimal op"
	}
}

func contains(haystack any, needle any) bool {
	values := sliceValues(haystack)
	return slices.ContainsFunc(values, func(value any) bool {
		return valuesEqual(value, needle)
	})
}

func sliceValues(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = v
		}
		return out
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out
	}
}

func toDecimal(value any) (decimal.Decimal, bool) {
	switch typed := value.(type) {
	case string:
		d, err := decimal.Parse(typed)
		return d, err == nil
	case json.Number:
		d, err := decimal.Parse(typed.String())
		return d, err == nil
	case int:
		d, err := decimal.Parse(strconv.Itoa(typed))
		return d, err == nil
	case int64:
		d, err := decimal.Parse(strconv.FormatInt(typed, 10))
		return d, err == nil
	case float64:
		d, err := decimal.Parse(strconv.FormatFloat(typed, 'f', -1, 64))
		return d, err == nil
	default:
		return decimal.Decimal{}, false
	}
}
