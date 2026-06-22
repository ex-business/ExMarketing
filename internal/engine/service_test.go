package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/fact"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type engineRoundTripFunc func(*http.Request) (*http.Response, error)

func (f engineRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSimulateEventMatchFiltersAndReasons(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activities := []marketing.Activity{
		matchTestActivity("active", "ACTIVE", marketing.ActivityActive, base, 10),
		matchTestActivity("inactive", "INACTIVE", marketing.ActivityPaused, base, 9),
		matchTestActivity("not_started", "NOT_STARTED", marketing.ActivityActive, base.Add(2*time.Hour), 8),
		matchTestActivity("ended", "ENDED", marketing.ActivityActive, base.Add(-3*time.Hour), 7),
		matchTestActivity("other_tenant", "OTHER_TENANT", marketing.ActivityActive, base, 6),
		matchTestActivity("other_event", "OTHER_EVENT", marketing.ActivityActive, base, 5),
	}
	activities[3].EndTime = base
	activities[4].TenantID = "site_002"
	activities[5].TriggerEvents = []string{"WITHDRAW_SUCCESS"}
	for _, activity := range activities {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("seed activity %s: %v", activity.Code, err)
		}
	}

	result, err := NewService(repo).SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if result.Summary.CandidateCount != 4 || result.Summary.MatchedCount != 1 || result.Summary.UnmatchedCount != 3 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if got := result.MatchedActivities[0].ActivityCode; got != "ACTIVE" {
		t.Fatalf("matched activity=%s, want ACTIVE", got)
	}
	reasons := unmatchedReasons(result.UnmatchedActivities)
	for _, want := range []string{"ACTIVITY_NOT_ACTIVE", "ACTIVITY_NOT_STARTED", "ACTIVITY_ENDED"} {
		if !reasons[want] {
			t.Fatalf("missing unmatched reason %s in %+v", want, result.UnmatchedActivities)
		}
	}
	for _, activity := range append(result.MatchedActivities, result.UnmatchedActivities...) {
		if activity.ActivityCode == "OTHER_TENANT" || activity.ActivityCode == "OTHER_EVENT" {
			t.Fatalf("activity outside match boundary leaked into result: %+v", activity)
		}
	}
}

func TestProcessEventTargetsActivityCodeWhenProvided(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()

	legacy := matchTestActivity("legacy_daily", "DAILY_CHECKIN_REWARD_DEV_FINAL", marketing.ActivityActive, base, 100)
	legacy.TriggerEvents = []string{"CHECKIN_SUCCESS"}
	legacy.Type = "CHECKIN"
	legacy.Rules = []marketing.Rule{{
		ID:      "rule_legacy_event_type",
		Code:    "LEGACY_EVENT_TYPE",
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(`{"logic":"AND","conditions":[{"field":"attributes.eventType","op":"=","value":"DAILY_CHECKIN"}]}`),
		Enabled: true,
		Version: 1,
	}}
	target := matchTestActivity("streak_daily", "DAILY_CHECKIN_STREAK_REWARD", marketing.ActivityActive, base, 90)
	target.TriggerEvents = []string{"CHECKIN_SUCCESS"}
	target.Type = "CHECKIN"
	target.Rewards = []marketing.Reward{fixedBonusReward("DAY1_USDT", `{"amount":"1.00","currency":"USDT","targetRef":"https://wallet.example.com/reward"}`)}

	for _, activity := range []marketing.Activity{legacy, target} {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("seed activity %s: %v", activity.Code, err)
		}
	}

	event := marketing.ActivityEvent{
		EventID:      "evt_streak_target",
		EventType:    "CHECKIN_SUCCESS",
		SourceSystem: "app",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u1"},
		Biz:          marketing.EventBiz{BizType: "CHECKIN", BizID: "b1"},
		OccurredAt:   base,
		Attributes: map[string]any{
			"activityCode":  "DAILY_CHECKIN_STREAK_REWARD",
			"activityScene": "seven_day_checkin",
			"checkin":       true,
		},
	}

	result, err := NewService(repo).ProcessEvent(ctx, event)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(result.Participations) != 1 {
		t.Fatalf("expected only targeted participation, got %+v", result.Participations)
	}
	if got := result.Participations[0].ActivityID; got != target.ID {
		t.Fatalf("participation activity=%s, want %s", got, target.ID)
	}
	if len(result.RuleResults) != 1 || result.RuleResults[0].ActivityCode != target.Code {
		t.Fatalf("expected only target rule result, got %+v", result.RuleResults)
	}
	if len(result.GrantOrders) != 1 {
		t.Fatalf("expected target grant order, got %+v", result.GrantOrders)
	}
}

func TestSimulateEventMatchEntryAudienceAndConfigReasons(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		config   []byte
		expected string
		decision marketing.RuleDecision
	}{
		{name: "entry disabled", config: []byte(`{"entry":{"enabled":false}}`), expected: "ENTRY_DISABLED", decision: marketing.RuleDecisionRejected},
		{name: "channel", config: []byte(`{"entry":{"channels":["WEB"]}}`), expected: "ENTRY_CHANNEL_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "source system", config: []byte(`{"entry":{"sourceSystems":["wallet"]}}`), expected: "ENTRY_SOURCE_SYSTEM_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "tenant", config: []byte(`{"entry":{"tenantIds":["site_999"]}}`), expected: "ENTRY_TENANT_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "subject type", config: []byte(`{"entry":{"audience":{"subjectTypes":["ACCOUNT"]}}}`), expected: "AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "segment", config: []byte(`{"entry":{"audience":{"segments":["vip"]}}}`), expected: "AUDIENCE_SEGMENT_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "risk level", config: []byte(`{"entry":{"audience":{"riskLevels":["HIGH"]}}}`), expected: "AUDIENCE_RISK_LEVEL_NOT_ALLOWED", decision: marketing.RuleDecisionRejected},
		{name: "invalid config", config: []byte(`{"entry":"bad"}`), expected: "INVALID_MATCH_CONFIG", decision: marketing.RuleDecisionError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := marketing.NewMemoryRepository()
			activity := matchTestActivity("act_"+tt.name, "ACT_"+tt.name, marketing.ActivityActive, base, 1)
			activity.Config = tt.config
			if _, err := repo.CreateActivity(ctx, activity); err != nil {
				t.Fatalf("seed activity: %v", err)
			}
			result, err := NewService(repo).SimulateEvent(ctx, matchTestEvent(base))
			if err != nil {
				t.Fatalf("simulate: %v", err)
			}
			if len(result.UnmatchedActivities) != 1 {
				t.Fatalf("expected one unmatched activity, got %+v", result)
			}
			got := result.UnmatchedActivities[0]
			if got.Decision != tt.decision || got.DecisionReasonCode != tt.expected {
				t.Fatalf("decision=%s reason=%s, want %s/%s", got.Decision, got.DecisionReasonCode, tt.decision, tt.expected)
			}
		})
	}
}

func TestSimulateEventMatchRuleFailureUsesMatchReason(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activity := matchTestActivity("match_rule", "MATCH_RULE", marketing.ActivityActive, base, 1)
	activity.Rules = []marketing.Rule{{
		ID:      "rule_match",
		Code:    "MATCH_CHANNEL",
		Phase:   marketing.RulePhaseMatch,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(`{"logic":"AND","conditions":[{"field":"attributes.channel","op":"=","value":"WEB"}]}`),
		Enabled: true,
		Version: 1,
	}}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("seed activity: %v", err)
	}

	result, err := NewService(repo).SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if len(result.UnmatchedActivities) != 1 {
		t.Fatalf("expected one unmatched activity, got %+v", result)
	}
	got := result.UnmatchedActivities[0]
	if got.Decision != marketing.RuleDecisionRejected || got.DecisionReasonCode != "MATCH_RULE_REJECTED" {
		t.Fatalf("decision=%s reason=%s, want REJECTED/MATCH_RULE_REJECTED", got.Decision, got.DecisionReasonCode)
	}
	if got.RuleResult.FailedStage != marketing.RuleStageMatch || got.RuleResult.ReasonCode == "" {
		t.Fatalf("expected MATCH rule explain, got %+v", got.RuleResult)
	}
}

func TestSimulateEventRewardConditionFailureDoesNotPreviewGrantOrOutbox(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activity := matchTestActivity("reward_fail", "REWARD_FAIL", marketing.ActivityActive, base, 1)
	activity.Rules = []marketing.Rule{rewardAmountRule("REWARD_AMOUNT", "100.00")}
	activity.Rewards = []marketing.Reward{fixedBonusReward("REWARD_A", `{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	event := matchTestEvent(base)
	event.Attributes["amount"] = "99.99"

	result, err := NewService(repo).SimulateEvent(ctx, event)
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if len(result.UnmatchedActivities) != 1 {
		t.Fatalf("expected one unmatched activity, got %+v", result)
	}
	got := result.UnmatchedActivities[0]
	if got.ParticipationPreview == nil || got.ParticipationPreview.Status != marketing.ParticipationRejected || got.RuleResult.FailedStage != marketing.RuleStageReward {
		t.Fatalf("expected REWARD rejected participation, got %+v", got)
	}
	if got.WouldCreateGrantOrders || len(got.GrantOrderPreviews) != 0 || len(got.OutboxCommandPreviews) != 0 {
		t.Fatalf("REWARD failure must not preview grants/outbox, got %+v", got)
	}
	ruleResult := got.RuleResult.StageResults[2].RuleResults[0]
	if ruleResult.Field != "attributes.amount" || ruleResult.Actual != "99.99" {
		t.Fatalf("expected REWARD amount explain, got %+v", ruleResult)
	}
}

func TestProcessEventRewardConditionAllowsNonAmountRewardWithZeroAmount(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activity := matchTestActivity("reward_coupon", "REWARD_COUPON", marketing.ActivityActive, base, 1)
	activity.Rules = []marketing.Rule{rewardAmountRule("REWARD_AMOUNT", "100.00")}
	activity.Rewards = []marketing.Reward{{
		ID:            "reward_coupon",
		Code:          "COUPON_A",
		Type:          marketing.RewardCoupon,
		CalculateType: marketing.CalculateFixed,
		Config:        []byte(`{"couponTemplateId":"WELCOME_10","quantity":"1","targetRef":"https://coupon.example.com/grant"}`),
		GrantMode:     marketing.GrantAuto,
		GrantChannel:  marketing.GrantHTTP,
		Enabled:       true,
		Version:       1,
	}}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	event := matchTestEvent(base)
	event.Attributes["amount"] = "299.00"

	result, err := NewService(repo).ProcessEvent(ctx, event)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(result.Participations) != 1 || result.Participations[0].Status != marketing.ParticipationRewardable {
		t.Fatalf("expected rewardable participation, got %+v", result.Participations)
	}
	if len(result.GrantOrders) != 1 || len(result.OutboxMessages) != 1 {
		t.Fatalf("expected coupon grant and outbox, got grants=%+v outbox=%+v", result.GrantOrders, result.OutboxMessages)
	}
	if result.GrantOrders[0].RewardType != marketing.RewardCoupon || result.GrantOrders[0].RewardAmount != "0.0000" {
		t.Fatalf("expected non-amount coupon grant with zero accounting amount, got %+v", result.GrantOrders[0])
	}
	if result.RuleResults[0].FailedStage != "" || !result.RuleResults[0].CanReward {
		t.Fatalf("expected REWARD rule pass before reward calculation, got %+v", result.RuleResults[0])
	}
}

func TestSimulateEventExclusiveGroupSelection(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activities := []marketing.Activity{
		matchTestActivity("high", "HIGH", marketing.ActivityActive, base, 100),
		matchTestActivity("stackable_loser", "STACKABLE_LOSER", marketing.ActivityActive, base, 90),
		matchTestActivity("low_nonstackable", "LOW_NONSTACKABLE", marketing.ActivityActive, base, 80),
		matchTestActivity("free", "FREE", marketing.ActivityActive, base, 70),
		matchTestActivity("tie_old", "TIE_OLD", marketing.ActivityActive, base, 60),
		matchTestActivity("tie_new", "TIE_NEW", marketing.ActivityActive, base, 60),
	}
	for i := range activities {
		switch activities[i].Code {
		case "HIGH", "STACKABLE_LOSER", "LOW_NONSTACKABLE":
			activities[i].ExclusiveGroup = "g1"
		case "TIE_OLD", "TIE_NEW":
			activities[i].ExclusiveGroup = "g2"
		}
		activities[i].Stackable = true
	}
	activities[2].Stackable = false
	activities[4].CreatedAt = base.Add(-time.Hour)
	activities[5].CreatedAt = base
	for _, activity := range activities {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("seed activity %s: %v", activity.Code, err)
		}
	}

	result, err := NewService(repo).SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	matched := matchedCodes(result.MatchedActivities)
	if !matched["HIGH"] || !matched["FREE"] || !matched["TIE_OLD"] || !matched["TIE_NEW"] {
		t.Fatalf("unexpected matched activities: %+v", result.MatchedActivities)
	}
	if matched["STACKABLE_LOSER"] || matched["LOW_NONSTACKABLE"] {
		t.Fatalf("exclusive losers should not be matched: %+v", result.MatchedActivities)
	}
	reasons := unmatchedReasons(result.UnmatchedActivities)
	if !reasons["EXCLUSIVE_GROUP_LOST"] {
		t.Fatalf("expected exclusive rejection, got %+v", result.UnmatchedActivities)
	}
}

func TestSimulateEventQueriesFactsAndReportsCache(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	if _, err := repo.SaveFactProvider(ctx, marketing.FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		CacheTTLSeconds: 300,
		Enabled:         true,
		TimeoutMS:       5000,
		UpdatedAt:       base,
	}); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	activity := matchTestActivity("fact_match", "FACT_MATCH", marketing.ActivityActive, base, 1)
	activity.Rules = []marketing.Rule{{
		ID:      "rule_fact",
		Code:    "FACT_VIP",
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(`{"logic":"AND","conditions":[{"field":"facts.USER_PROFILE.vipLevel","op":"=","value":"VIP3"}]}`),
		Enabled: true,
		Version: 1,
	}}
	activity.Rewards = []marketing.Reward{fixedBonusReward("REWARD_A", `{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("seed activity: %v", err)
	}

	var calls int32
	service := NewService(repo)
	service.factClient = fact.NewClientWithHTTPClient(&http.Client{Transport: engineRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read fact request: %v", err)
		}
		if strings.Contains(string(body), "traceId") || !strings.Contains(string(body), "DEPOSIT_SUCCESS") {
			t.Fatalf("fact query should use standard event context without traceId, got %s", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})

	first, err := service.SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("first simulate: %v", err)
	}
	if len(first.MatchedActivities) != 1 {
		t.Fatalf("expected fact-backed activity to match, got %+v", first)
	}
	firstFact := first.MatchedActivities[0].FactQueryResults
	if len(firstFact) != 1 || firstFact[0].FactCode != "USER_PROFILE" || firstFact[0].Status != marketing.RuleDecisionPassed || firstFact[0].Cached {
		t.Fatalf("unexpected first fact query result: %+v", firstFact)
	}
	if len(firstFact[0].Fields) != 1 || !firstFact[0].Fields[0].Available || firstFact[0].Fields[0].FullPath != "facts.USER_PROFILE.vipLevel" {
		t.Fatalf("unexpected fact fields: %+v", firstFact[0].Fields)
	}

	second, err := service.SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("second simulate: %v", err)
	}
	secondFact := second.MatchedActivities[0].FactQueryResults
	if len(secondFact) != 1 || !secondFact[0].Cached {
		t.Fatalf("expected second fact query to hit cache, got %+v", secondFact)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("provider calls=%d, want 1", calls)
	}
}

func TestSimulateEventFactQueryFailureIsExplainedAndIsolated(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	factActivity := matchTestActivity("fact_missing", "FACT_MISSING", marketing.ActivityActive, base, 10)
	factActivity.Rules = []marketing.Rule{{
		ID:      "rule_fact_missing",
		Code:    "FACT_VIP",
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(`{"logic":"AND","conditions":[{"field":"facts.USER_PROFILE.vipLevel","op":"=","value":"VIP3"}]}`),
		Enabled: true,
		Version: 1,
	}}
	plainActivity := matchTestActivity("plain", "PLAIN", marketing.ActivityActive, base, 1)
	for _, activity := range []marketing.Activity{factActivity, plainActivity} {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("seed activity %s: %v", activity.Code, err)
		}
	}

	result, err := NewService(repo).SimulateEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if len(result.MatchedActivities) != 1 || result.MatchedActivities[0].ActivityCode != "PLAIN" {
		t.Fatalf("non-fact activity should still match, got %+v", result.MatchedActivities)
	}
	if len(result.UnmatchedActivities) != 1 {
		t.Fatalf("expected fact activity to be unmatched, got %+v", result.UnmatchedActivities)
	}
	got := result.UnmatchedActivities[0]
	if got.DecisionReasonCode != "FACT_QUERY_FAILED" {
		t.Fatalf("expected FACT_QUERY_FAILED, got %+v", got)
	}
	if len(got.FactQueryResults) != 1 || got.FactQueryResults[0].ReasonCode != "FACT_QUERY_FAILED" || got.FactQueryResults[0].Status != marketing.RuleDecisionRejected {
		t.Fatalf("expected explained fact query failure, got %+v", got.FactQueryResults)
	}
	if len(got.FactQueryResults[0].Fields) != 1 || got.FactQueryResults[0].Fields[0].Available {
		t.Fatalf("expected unavailable fact field, got %+v", got.FactQueryResults[0].Fields)
	}
}

func TestProcessEventExclusiveGroupLoserCreatesRejectedParticipation(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	winner := matchTestActivity("winner", "WINNER", marketing.ActivityActive, base, 100)
	loser := matchTestActivity("loser", "LOSER", marketing.ActivityActive, base, 90)
	winner.ExclusiveGroup = "deposit_bonus"
	loser.ExclusiveGroup = "deposit_bonus"
	winner.Stackable = false
	loser.Stackable = false
	for _, activity := range []marketing.Activity{winner, loser} {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("seed activity %s: %v", activity.Code, err)
		}
	}

	result, err := NewService(repo).ProcessEvent(ctx, matchTestEvent(base))
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(result.Participations) != 2 {
		t.Fatalf("expected winner and exclusive loser participations, got %+v", result.Participations)
	}
	var loserFound bool
	for _, participation := range result.Participations {
		if participation.ActivityID == "loser" {
			loserFound = true
			if participation.Status != marketing.ParticipationRejected || participation.RejectReason == "" {
				t.Fatalf("expected rejected exclusive loser participation, got %+v", participation)
			}
		}
	}
	if !loserFound {
		t.Fatalf("exclusive loser participation missing: %+v", result.Participations)
	}
}

func TestProcessEventDailyLimitUsesActivityTimezone(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 5, 24, 22, 30, 0, 0, time.UTC)
	repo := marketing.NewMemoryRepository()
	activity := matchTestActivity("daily_limit", "DAILY_LIMIT", marketing.ActivityActive, base, 1)
	activity.StartTime = base.Add(-time.Hour)
	activity.EndTime = base.Add(3 * time.Hour)
	activity.Timezone = "Asia/Shanghai"
	activity.Rewards = []marketing.Reward{fixedBonusReward("REWARD_A", `{"amount":"1.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	if _, err := repo.CreateQuota(ctx, marketing.Quota{
		ID:          "quota_daily",
		ActivityID:  activity.ID,
		Type:        marketing.QuotaDailyLimit,
		Scope:       marketing.QuotaScopeUser,
		ScopeKey:    "u1:2026-05-25",
		TotalValue:  "1",
		UsedValue:   "0",
		FrozenValue: "0",
		Version:     1,
		CreatedAt:   base,
		UpdatedAt:   base,
	}); err != nil {
		t.Fatalf("seed daily quota: %v", err)
	}

	service := NewService(repo)
	event := matchTestEvent(base)
	if _, err := service.ProcessEvent(ctx, event); err != nil {
		t.Fatalf("first process should pass: %v", err)
	}
	event.EventID = "evt_match_2"
	event.Biz.BizID = "b2"
	_, err := service.ProcessEvent(ctx, event)
	if err == nil {
		t.Fatal("expected daily limit quota error")
	}
	var quotaErr marketing.QuotaExceededError
	if !errors.As(err, &quotaErr) || quotaErr.Type != marketing.QuotaDailyLimit || quotaErr.ScopeKey != "u1:2026-05-25" {
		t.Fatalf("expected DAILY_LIMIT for activity local date, got %T %[1]v", err)
	}
}

func rewardAmountRule(code string, threshold string) marketing.Rule {
	return marketing.Rule{
		ID:      "rule_" + code,
		Code:    code,
		Phase:   marketing.RulePhaseReward,
		Type:    marketing.RuleTypeCondition,
		Config:  []byte(`{"logic":"AND","conditions":[{"field":"attributes.amount","op":">=","value":"` + threshold + `"}]}`),
		Enabled: true,
		Version: 1,
	}
}

func fixedBonusReward(code string, config string) marketing.Reward {
	return marketing.Reward{
		ID:            "reward_" + code,
		Code:          code,
		Type:          marketing.RewardBonus,
		CalculateType: marketing.CalculateFixed,
		Config:        []byte(config),
		GrantMode:     marketing.GrantAuto,
		GrantChannel:  marketing.GrantHTTP,
		Enabled:       true,
		Version:       1,
	}
}

func matchTestActivity(id string, code string, status marketing.ActivityStatus, base time.Time, priority int) marketing.Activity {
	return marketing.Activity{
		ID:            id,
		TenantID:      "site_001",
		Code:          code,
		Name:          code,
		Type:          "BONUS",
		Status:        status,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     base.Add(-time.Hour),
		EndTime:       base.Add(time.Hour),
		Timezone:      "UTC",
		Priority:      priority,
		Version:       1,
		Approval: marketing.ApprovalSnapshot{
			Status:          marketing.ApprovalApproved,
			Submitter:       "ops",
			L1Approver:      "lead",
			ActivityVersion: 1,
		},
		Config:    []byte(`{"display":{"visible":false}}`),
		CreatedAt: base,
		UpdatedAt: base,
	}
}

func matchTestEvent(occurredAt time.Time) marketing.ActivityEvent {
	return marketing.ActivityEvent{
		EventID:      "evt_match",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u1"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "b1"},
		OccurredAt:   occurredAt,
		Attributes:   map[string]any{"channel": "APP", "segment": "new_user", "riskLevel": "NORMAL"},
	}
}

func unmatchedReasons(activities []marketing.SimulationActivityResult) map[string]bool {
	out := map[string]bool{}
	for _, activity := range activities {
		out[activity.DecisionReasonCode] = true
	}
	return out
}

func matchedCodes(activities []marketing.SimulationActivityResult) map[string]bool {
	out := map[string]bool{}
	for _, activity := range activities {
		out[activity.ActivityCode] = true
	}
	return out
}
