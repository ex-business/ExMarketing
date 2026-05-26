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

func TestConditionEvaluatorKeepsJSONNumberDecimalPrecision(t *testing.T) {
	event := marketing.ActivityEvent{
		EventType:  "DEPOSIT_SUCCESS",
		Attributes: map[string]any{"amount": "0.3"},
	}
	rule := marketing.Rule{
		Type: marketing.RuleTypeCondition,
		Config: []byte(`{
			"logic": "AND",
			"conditions": [
				{"field": "attributes.amount", "op": "=", "value": 0.30},
				{"field": "attributes.amount", "op": ">=", "value": 0.1}
			]
		}`),
	}

	result := ConditionEvaluator{}.Evaluate(Context{Event: event}, rule)
	if !result.Passed {
		t.Fatalf("expected JSON number decimal comparisons to pass, got %+v", result)
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

func TestExecuteRunsFourStagesWithStableRuleDetails(t *testing.T) {
	rules := []marketing.Rule{
		conditionRule("limit", marketing.RulePhaseLimit, 20, `{"conditions":[{"field":"state.quota.USER_LIMIT.remaining","op":">","value":"0"}]}`),
		conditionRule("match", marketing.RulePhaseMatch, 10, `{"conditions":[{"field":"eventType","op":"=","value":"DEPOSIT_SUCCESS"}]}`),
		conditionRule("reward", marketing.RulePhaseReward, 10, `{"conditions":[{"field":"attributes.amount","op":">=","value":"100"}]}`),
		conditionRule("risk", marketing.RulePhaseRisk, 10, `{"conditions":[{"field":"facts.RISK.score","op":"<","value":"80"}]}`),
		conditionRule("qualify", marketing.RulePhaseQualify, 10, `{"conditions":[{"field":"state.activity.status","op":"=","value":"ACTIVE"}]}`),
	}
	event := marketing.ActivityEvent{
		EventType:  "DEPOSIT_SUCCESS",
		Attributes: map[string]any{"amount": "100.00"},
	}
	result := Execute(Context{
		Event: event,
		State: map[string]any{
			"activity": map[string]any{"status": "ACTIVE"},
			"quota":    map[string]any{"USER_LIMIT": map[string]any{"remaining": "1"}},
		},
		Facts: map[string]any{"RISK": map[string]any{"score": "20"}},
	}, rules, NewRegistry())

	if result.Decision != marketing.RuleDecisionPassed {
		t.Fatalf("expected pass, got %+v", result)
	}
	wantStages := []marketing.RuleStage{
		marketing.RuleStageMatch,
		marketing.RuleStageQualify,
		marketing.RuleStageReward,
		marketing.RuleStageRiskLimitBudget,
	}
	for i, want := range wantStages {
		if result.StageResults[i].Stage != want {
			t.Fatalf("stage[%d] expected %s, got %s", i, want, result.StageResults[i].Stage)
		}
	}
	riskLimit := result.StageResults[3].RuleResults
	if len(riskLimit) != 2 || riskLimit[0].RuleCode != "risk" || riskLimit[1].RuleCode != "limit" {
		t.Fatalf("expected RISK then LIMIT in grouped stage, got %+v", riskLimit)
	}
}

func TestExecuteReportsDisabledAndMissingFact(t *testing.T) {
	disabled := conditionRule("disabled", marketing.RulePhaseMatch, 1, `{"conditions":[{"field":"eventType","op":"=","value":"OTHER"}]}`)
	disabled.Enabled = false
	missingFact := conditionRule("fact", marketing.RulePhaseQualify, 1, `{"conditions":[{"field":"facts.USER_PROFILE.vipLevel","op":">=","value":"3"}]}`)

	result := Execute(Context{Event: marketing.ActivityEvent{EventType: "DEPOSIT_SUCCESS"}}, []marketing.Rule{disabled, missingFact}, NewRegistry())

	if result.Decision != marketing.RuleDecisionRejected || result.FailedStage != marketing.RuleStageQualify {
		t.Fatalf("expected QUALIFY rejection, got %+v", result)
	}
	if got := result.StageResults[0].RuleResults[0]; got.Status != marketing.RuleDecisionSkipped || got.ReasonCode != ReasonRuleDisabled {
		t.Fatalf("expected disabled skip, got %+v", got)
	}
	if got := result.StageResults[1].RuleResults[0]; got.ReasonCode != ReasonFactFieldMissing || got.Field != "facts.USER_PROFILE.vipLevel" {
		t.Fatalf("expected fact missing detail, got %+v", got)
	}
}

func TestRewardConditionReturnsConditionDetailsAndRejectsEmptyConditions(t *testing.T) {
	empty := conditionRule("reward_empty", marketing.RulePhaseReward, 1, `{"logic":"AND","conditions":[]}`)
	emptyResult := NewRegistry().Evaluate(Context{}, empty)
	if emptyResult.Status != marketing.RuleDecisionError || emptyResult.ReasonCode != ReasonInvalidConfig {
		t.Fatalf("expected empty REWARD conditions to be invalid, got %+v", emptyResult)
	}

	rule := conditionRule("reward_amount", marketing.RulePhaseReward, 10, `{
		"logic": "AND",
		"conditions": [
			{"field":"attributes.amount","op":">=","value":"100.00"},
			{"field":"attributes.inviteCount","op":">=","value":"3"}
		]
	}`)
	event := marketing.ActivityEvent{
		Attributes: map[string]any{"amount": "99.99", "inviteCount": "2"},
	}

	result := Execute(Context{Event: event}, []marketing.Rule{rule}, NewRegistry())

	if result.Decision != marketing.RuleDecisionRejected || result.FailedStage != marketing.RuleStageReward {
		t.Fatalf("expected REWARD rejection, got %+v", result)
	}
	got := result.StageResults[2].RuleResults[0]
	if got.Field != "attributes.amount" || got.Actual != "99.99" || got.Expected != "100.00" {
		t.Fatalf("expected first failing condition explain, got %+v", got)
	}
	conditions, ok := got.Detail["conditions"].([]map[string]any)
	if !ok || len(conditions) != 2 {
		t.Fatalf("expected condition details, got %#v", got.Detail)
	}
	if conditions[0]["field"] != "attributes.amount" || conditions[0]["passed"] != false {
		t.Fatalf("unexpected first condition detail: %#v", conditions[0])
	}
	if conditions[1]["field"] != "attributes.inviteCount" || conditions[1]["actual"] != "2" {
		t.Fatalf("unexpected second condition detail: %#v", conditions[1])
	}
}

func TestQualifyConditionSupportsVIPRegisterChannelAndTags(t *testing.T) {
	rule := conditionRule("qualify_profile", marketing.RulePhaseQualify, 10, `{
		"logic": "AND",
		"conditions": [
			{"field":"attributes.vipLevel","op":">=","value":"3"},
			{"field":"attributes.registerDays","op":">=","value":"7"},
			{"field":"attributes.channel","op":"IN","value":["APP","WEB"]},
			{"field":"attributes.tags","op":"IN","value":["new_user","high_value"]},
			{"field":"attributes.riskTags","op":"NOT_IN","value":["black","bonus_abuse"]}
		]
	}`)
	event := marketing.ActivityEvent{
		Attributes: map[string]any{
			"vipLevel":     "3",
			"registerDays": 8,
			"channel":      "APP",
			"tags":         []any{"sports", "high_value"},
			"riskTags":     []any{"normal"},
		},
	}

	result := Execute(Context{Event: event}, []marketing.Rule{rule}, NewRegistry())

	if result.Decision != marketing.RuleDecisionPassed {
		t.Fatalf("expected QUALIFY array and decimal conditions to pass, got %+v", result)
	}
}

func TestQualifyConditionExplainsMissingAttributeWithoutRewardOrQuota(t *testing.T) {
	rule := conditionRule("qualify_missing", marketing.RulePhaseQualify, 10, `{
		"logic": "AND",
		"conditions": [
			{"field":"attributes.vipLevel","op":">=","value":"3"},
			{"field":"attributes.registerDays","op":">=","value":"7"}
		]
	}`)
	event := marketing.ActivityEvent{
		Attributes: map[string]any{"vipLevel": "3"},
	}

	result := Execute(Context{Event: event}, []marketing.Rule{rule}, NewRegistry())

	if result.Decision != marketing.RuleDecisionRejected || result.FailedStage != marketing.RuleStageQualify {
		t.Fatalf("expected QUALIFY rejection, got %+v", result)
	}
	if result.CanReward || result.CanReserveQuota {
		t.Fatalf("expected rejected QUALIFY to block reward and quota, got %+v", result)
	}
	got := result.StageResults[1].RuleResults[0]
	if got.ReasonCode != ReasonFieldMissing || got.Field != "attributes.registerDays" {
		t.Fatalf("expected missing field explain, got %+v", got)
	}
}

func TestRegistryRejectsUnregisteredEvaluator(t *testing.T) {
	rule := marketing.Rule{
		Code:    "formula",
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeFormula,
		Enabled: true,
	}

	result := NewRegistry().Evaluate(Context{}, rule)

	if result.Status != marketing.RuleDecisionError || result.ReasonCode != ReasonEvaluatorNotFound {
		t.Fatalf("expected unregistered evaluator error, got %+v", result)
	}
}

func conditionRule(code string, phase marketing.RulePhase, sort int, config string) marketing.Rule {
	return marketing.Rule{
		Code:    code,
		Phase:   phase,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(config),
		Sort:    sort,
		Enabled: true,
	}
}
