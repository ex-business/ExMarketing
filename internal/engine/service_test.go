package engine

import (
	"context"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

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
		Config:        []byte(`{"display":{"visible":false}}`),
		CreatedAt:     base,
		UpdatedAt:     base,
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
