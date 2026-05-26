package rule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	ReasonRulePassed        = "RULE_PASSED"
	ReasonRuleDisabled      = "RULE_DISABLED"
	ReasonInvalidConfig     = "INVALID_RULE_CONFIG"
	ReasonFieldMissing      = "FIELD_MISSING"
	ReasonFactFieldMissing  = "FACT_FIELD_MISSING"
	ReasonTypeMismatch      = "TYPE_MISMATCH"
	ReasonUnsupportedOp     = "UNSUPPORTED_OPERATOR"
	ReasonEvaluatorNotFound = "EVALUATOR_NOT_FOUND"
	ReasonInvalidPhase      = "INVALID_RULE_PHASE"
	ReasonConditionFailed   = "CONDITION_NOT_MET"
)

type Context struct {
	Event marketing.ActivityEvent
	State map[string]any
	Facts map[string]any
}

type Result struct {
	Passed     bool           `json:"passed"`
	Reason     string         `json:"reason,omitempty"`
	ReasonCode string         `json:"reasonCode,omitempty"`
	Field      string         `json:"field,omitempty"`
	Expected   any            `json:"expected,omitempty"`
	Actual     any            `json:"actual,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
}

type Evaluator interface {
	Evaluate(ctx Context, rule marketing.Rule) Result
}

type Registry struct {
	evaluators map[marketing.RuleType]Evaluator
}

func NewRegistry() *Registry {
	registry := &Registry{evaluators: map[marketing.RuleType]Evaluator{}}
	registry.Register(marketing.RuleTypeCondition, ConditionEvaluator{})
	return registry
}

func (r *Registry) Register(ruleType marketing.RuleType, evaluator Evaluator) {
	if r.evaluators == nil {
		r.evaluators = map[marketing.RuleType]Evaluator{}
	}
	r.evaluators[ruleType] = evaluator
}

func (r *Registry) Evaluate(ctx Context, rule marketing.Rule) marketing.RuleExecutionResult {
	if !validPhase(rule.Phase) {
		return executionResult(rule, marketing.RuleDecisionError, false, Result{
			ReasonCode: ReasonInvalidPhase,
			Reason:     "rule phase is invalid",
		})
	}
	if !rule.Enabled {
		return executionResult(rule, marketing.RuleDecisionSkipped, false, Result{
			ReasonCode: ReasonRuleDisabled,
			Reason:     "rule is disabled",
		})
	}
	evaluator, ok := r.evaluators[rule.Type]
	if !ok {
		return executionResult(rule, marketing.RuleDecisionError, false, Result{
			ReasonCode: ReasonEvaluatorNotFound,
			Reason:     "rule evaluator is not registered",
		})
	}
	result := evaluator.Evaluate(ctx, rule)
	if result.Passed {
		if result.ReasonCode == "" {
			result.ReasonCode = ReasonRulePassed
		}
		if result.Reason == "" {
			result.Reason = "rule passed"
		}
		return executionResult(rule, marketing.RuleDecisionPassed, true, result)
	}
	status := marketing.RuleDecisionRejected
	if result.ReasonCode == ReasonInvalidConfig || result.ReasonCode == ReasonUnsupportedOp || result.ReasonCode == ReasonEvaluatorNotFound {
		status = marketing.RuleDecisionError
	}
	return executionResult(rule, status, false, result)
}

func Execute(ctx Context, rules []marketing.Rule, registry *Registry) marketing.ActivityRuleResult {
	if registry == nil {
		registry = NewRegistry()
	}
	ordered := stableRules(rules)
	out := marketing.ActivityRuleResult{
		Decision:        marketing.RuleDecisionPassed,
		CanReward:       true,
		CanReserveQuota: true,
	}
	for _, stage := range stagePlan() {
		stageResult := marketing.RuleStageResult{
			Stage:  stage.stage,
			Status: marketing.RuleDecisionPassed,
		}
		for _, activityRule := range ordered {
			if !slices.Contains(stage.phases, activityRule.Phase) {
				continue
			}
			ruleResult := registry.Evaluate(ctx, activityRule)
			stageResult.RuleResults = append(stageResult.RuleResults, ruleResult)
			switch ruleResult.Status {
			case marketing.RuleDecisionSkipped:
				stageResult.SkippedCount++
			case marketing.RuleDecisionPassed:
				stageResult.ExecutedCount++
			default:
				stageResult.ExecutedCount++
				stageResult.FailedCount++
				if stageResult.Status == marketing.RuleDecisionPassed {
					stageResult.Status = ruleResult.Status
					stageResult.ReasonCode = ruleResult.ReasonCode
					stageResult.Reason = ruleResult.Reason
				}
			}
		}
		out.StageResults = append(out.StageResults, stageResult)
		if stageResult.Status != marketing.RuleDecisionPassed {
			out.Decision = marketing.RuleDecisionRejected
			if stageResult.Status == marketing.RuleDecisionError {
				out.Decision = marketing.RuleDecisionError
			}
			out.FailedStage = stage.stage
			out.ReasonCode = stageResult.ReasonCode
			out.Reason = stageResult.Reason
			out.CanReward = false
			out.CanReserveQuota = false
			return out
		}
	}
	return out
}

type stageSpec struct {
	stage  marketing.RuleStage
	phases []marketing.RulePhase
}

func stagePlan() []stageSpec {
	return []stageSpec{
		{stage: marketing.RuleStageMatch, phases: []marketing.RulePhase{marketing.RulePhaseMatch}},
		{stage: marketing.RuleStageQualify, phases: []marketing.RulePhase{marketing.RulePhaseQualify}},
		{stage: marketing.RuleStageReward, phases: []marketing.RulePhase{marketing.RulePhaseReward}},
		{stage: marketing.RuleStageRiskLimitBudget, phases: []marketing.RulePhase{marketing.RulePhaseRisk, marketing.RulePhaseLimit}},
	}
}

func stableRules(rules []marketing.Rule) []marketing.Rule {
	out := slices.Clone(rules)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func validPhase(phase marketing.RulePhase) bool {
	switch phase {
	case marketing.RulePhaseMatch, marketing.RulePhaseQualify, marketing.RulePhaseReward, marketing.RulePhaseRisk, marketing.RulePhaseLimit:
		return true
	default:
		return false
	}
}

func executionResult(rule marketing.Rule, status marketing.RuleDecision, passed bool, result Result) marketing.RuleExecutionResult {
	if result.ReasonCode == "" {
		result.ReasonCode = ReasonConditionFailed
	}
	if result.Reason == "" {
		result.Reason = "rule failed"
	}
	return marketing.RuleExecutionResult{
		RuleCode:   rule.Code,
		RulePhase:  rule.Phase,
		RuleType:   rule.Type,
		Status:     status,
		Passed:     passed,
		ReasonCode: result.ReasonCode,
		Reason:     result.Reason,
		Field:      result.Field,
		Expected:   result.Expected,
		Actual:     result.Actual,
		Sort:       rule.Sort,
		Detail:     result.Detail,
	}
}

type ConditionEvaluator struct{}

func (e ConditionEvaluator) Evaluate(ctx Context, rule marketing.Rule) Result {
	var config marketing.ConditionRuleConfig
	decoder := json.NewDecoder(bytes.NewReader(rule.Config))
	decoder.UseNumber()
	if err := decoder.Decode(&config); err != nil {
		return Result{Passed: false, ReasonCode: ReasonInvalidConfig, Reason: "invalid rule config"}
	}

	logic := strings.ToUpper(strings.TrimSpace(config.Logic))
	if logic == "" {
		logic = "AND"
	}
	if logic != "AND" && logic != "OR" {
		return Result{Passed: false, ReasonCode: ReasonInvalidConfig, Reason: "invalid rule logic"}
	}
	if len(config.Conditions) == 0 {
		return Result{Passed: false, ReasonCode: ReasonInvalidConfig, Reason: "rule conditions are required"}
	}

	anyPassed := false
	conditionDetails := make([]map[string]any, 0, len(config.Conditions))
	var firstFailure Result
	var lastFailure Result
	for _, condition := range config.Conditions {
		result := evaluateCondition(ctx, condition)
		conditionDetails = append(conditionDetails, conditionDetail(condition, result))
		if logic == "AND" && !result.Passed {
			if firstFailure.Reason == "" {
				firstFailure = result
			}
		}
		if !result.Passed {
			lastFailure = result
		}
		anyPassed = anyPassed || result.Passed
	}

	detail := map[string]any{
		"logic":      logic,
		"conditions": conditionDetails,
	}
	if logic == "AND" && firstFailure.Reason != "" {
		firstFailure.Detail = detail
		return firstFailure
	}
	if logic == "OR" {
		if anyPassed {
			return Result{Passed: true, Detail: detail}
		}
		if lastFailure.Reason == "" {
			lastFailure = Result{Passed: false, ReasonCode: ReasonConditionFailed, Reason: "no OR condition passed"}
		}
		lastFailure.Detail = detail
		return lastFailure
	}
	return Result{Passed: true, Detail: detail}
}

func evaluateCondition(ctx Context, condition marketing.Condition) Result {
	actual, ok := fieldValue(ctx, condition.Field)
	if !ok {
		reasonCode := ReasonFieldMissing
		if strings.HasPrefix(condition.Field, "facts.") {
			reasonCode = ReasonFactFieldMissing
		}
		return Result{Passed: false, ReasonCode: reasonCode, Reason: fmt.Sprintf("field %s not found", condition.Field), Field: condition.Field, Expected: condition.Value}
	}

	op := strings.ToUpper(strings.TrimSpace(condition.Op))
	switch op {
	case "=", "==":
		return conditionResult(valuesEqual(actual, condition.Value), condition, actual, ReasonConditionFailed, fmt.Sprintf("%s != %v", condition.Field, condition.Value))
	case "!=", "<>":
		return conditionResult(!valuesEqual(actual, condition.Value), condition, actual, ReasonConditionFailed, fmt.Sprintf("%s == %v", condition.Field, condition.Value))
	case ">=", ">", "<=", "<":
		return compareDecimal(condition, actual, condition.Value, op)
	case "IN":
		return conditionResult(matchesMembership(condition.Value, actual), condition, actual, ReasonConditionFailed, fmt.Sprintf("%s not in %v", condition.Field, condition.Value))
	case "NOT_IN":
		return conditionResult(!matchesMembership(condition.Value, actual), condition, actual, ReasonConditionFailed, fmt.Sprintf("%s in %v", condition.Field, condition.Value))
	default:
		return Result{Passed: false, ReasonCode: ReasonUnsupportedOp, Reason: fmt.Sprintf("unsupported op %s", condition.Op), Field: condition.Field, Expected: condition.Value, Actual: actual}
	}
}

func conditionResult(passed bool, condition marketing.Condition, actual any, reasonCode string, reason string) Result {
	if passed {
		return Result{Passed: true, Field: condition.Field, Expected: condition.Value, Actual: actual}
	}
	return Result{Passed: false, ReasonCode: reasonCode, Reason: reason, Field: condition.Field, Expected: condition.Value, Actual: actual}
}

func conditionDetail(condition marketing.Condition, result Result) map[string]any {
	detail := map[string]any{
		"field":    condition.Field,
		"op":       condition.Op,
		"expected": condition.Value,
		"actual":   result.Actual,
		"passed":   result.Passed,
	}
	if result.ReasonCode != "" {
		detail["reasonCode"] = result.ReasonCode
	}
	if result.Reason != "" {
		detail["reason"] = result.Reason
	}
	return detail
}

func fieldValue(ctx Context, path string) (any, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, false
	}

	var current any
	switch parts[0] {
	case "eventId":
		current = ctx.Event.EventID
	case "eventType":
		current = ctx.Event.EventType
	case "sourceSystem":
		current = ctx.Event.SourceSystem
	case "tenantId":
		current = ctx.Event.TenantID
	case "occurredAt":
		current = ctx.Event.OccurredAt.Format("2006-01-02T15:04:05Z07:00")
	case "subject":
		current = map[string]any{"type": ctx.Event.Subject.Type, "id": ctx.Event.Subject.ID}
	case "biz":
		current = map[string]any{"bizType": ctx.Event.Biz.BizType, "bizId": ctx.Event.Biz.BizID}
	case "attributes":
		current = ctx.Event.Attributes
	case "state":
		current = ctx.State
	case "facts":
		current = ctx.Facts
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

func compareDecimal(condition marketing.Condition, actual any, expected any, op string) Result {
	ad, ok := toDecimal(actual)
	if !ok {
		return Result{Passed: false, ReasonCode: ReasonTypeMismatch, Reason: "actual value is not decimal", Field: condition.Field, Expected: expected, Actual: actual}
	}
	bd, ok := toDecimal(expected)
	if !ok {
		return Result{Passed: false, ReasonCode: ReasonTypeMismatch, Reason: "expected value is not decimal", Field: condition.Field, Expected: expected, Actual: actual}
	}
	cmp := ad.Cmp(bd)
	var passed bool
	var reason string
	switch op {
	case ">=":
		passed = cmp >= 0
		reason = fmt.Sprintf("%s < %s", ad.String(), bd.String())
	case ">":
		passed = cmp > 0
		reason = fmt.Sprintf("%s <= %s", ad.String(), bd.String())
	case "<=":
		passed = cmp <= 0
		reason = fmt.Sprintf("%s > %s", ad.String(), bd.String())
	case "<":
		passed = cmp < 0
		reason = fmt.Sprintf("%s >= %s", ad.String(), bd.String())
	default:
		return Result{Passed: false, ReasonCode: ReasonUnsupportedOp, Reason: "unsupported decimal op", Field: condition.Field, Expected: expected, Actual: actual}
	}
	return conditionResult(passed, condition, actual, ReasonConditionFailed, reason)
}

func matchesMembership(expected any, actual any) bool {
	expectedValues := sliceValues(expected)
	actualValues := sliceValues(actual)
	if len(actualValues) == 0 {
		actualValues = []any{actual}
	}
	for _, actualValue := range actualValues {
		if slices.ContainsFunc(expectedValues, func(expectedValue any) bool {
			return valuesEqual(expectedValue, actualValue)
		}) {
			return true
		}
	}
	return false
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
	default:
		return decimal.Decimal{}, false
	}
}
