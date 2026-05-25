package marketing

import (
	"strings"
	"testing"
	"time"
)

func TestValidatePrelaunchBlocksInvalidProbabilityReward(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards = []Reward{{
		Code:          "DRAW",
		Type:          RewardBonus,
		CalculateType: CalculateProbability,
		Config:        []byte(`{"currency":"USD","targetRef":"https://wallet.example.com/reward","items":[{"itemCode":"A","weight":"-1","amount":"10"}]}`),
		GrantMode:     GrantAuto,
		GrantChannel:  GrantHTTP,
		Enabled:       true,
		Version:       1,
	}}

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchBlock || result.AllowGoLive {
		t.Fatalf("conclusion=%s allow=%v, want BLOCK false", result.Conclusion, result.AllowGoLive)
	}
	if !hasPrelaunchCode(result.Items, "CONFIG_SCHEMA") {
		t.Fatalf("expected CONFIG_SCHEMA item, got %+v", result.Items)
	}
}

func TestValidatePrelaunchRequiresApprovalForAutoFundReward(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, Actor: "operator", TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchApprovalRequired || result.AllowGoLive {
		t.Fatalf("conclusion=%s allow=%v, want APPROVAL_REQUIRED false", result.Conclusion, result.AllowGoLive)
	}
	if !result.RequireApproval || len(result.HighRiskItems) == 0 {
		t.Fatalf("expected approval and high-risk items, got %+v", result)
	}
	if !hasPrelaunchCode(result.Items, "BUDGET_MISSING_AUTO_FUND") {
		t.Fatalf("expected budget high-risk item, got %+v", result.Items)
	}
}

func TestValidatePrelaunchAllowsApprovedHighRiskVersion(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	req := PrelaunchValidationRequest{
		Activity: cmd,
		Quotas: []Quota{{
			ActivityID: "draft",
			Type:       QuotaBudget,
			Scope:      QuotaScopeActivity,
			ScopeKey:   "TOTAL",
			TotalValue: "1000",
		}},
		Approval: ApprovalSnapshot{
			Status:          ApprovalApproved,
			Submitter:       "operator",
			Approver:        "manager",
			ActivityVersion: 1,
		},
		TargetAction: "GO_LIVE",
	}

	result := ValidatePrelaunch(req, now)

	if !result.AllowGoLive {
		t.Fatalf("expected approved high-risk request to allow go-live, got %+v", result)
	}
	if result.Conclusion != PrelaunchWarn {
		t.Fatalf("conclusion=%s, want WARN because optional stock remains a warning", result.Conclusion)
	}
	if !hasPrelaunchCode(result.Items, "APPROVAL_VALID") {
		t.Fatalf("expected approval validation item, got %+v", result.Items)
	}
}

func TestValidatePrelaunchBlocksExclusiveGroupSamePriorityConflict(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	existing := Activity{
		TenantID:       cmd.TenantID,
		Code:           "EXISTING",
		Status:         ActivityActive,
		TriggerEvents:  []string{"DEPOSIT_SUCCESS"},
		StartTime:      cmd.StartTime.Add(time.Hour),
		EndTime:        cmd.EndTime.Add(-time.Hour),
		ExclusiveGroup: cmd.ExclusiveGroup,
		Stackable:      false,
		Priority:       cmd.Priority,
	}

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, ExistingActivities: []Activity{existing}}, now)

	if result.Conclusion != PrelaunchBlock || !hasPrelaunchCode(result.Items, "EXCLUSIVE_GROUP_CONFLICT") {
		t.Fatalf("expected exclusive-group BLOCK, got %+v", result)
	}
}

func TestValidatePrelaunchBlocksUnknownPluginCode(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Type = "LOTTERY"
	cmd.PluginCode = "MISSING_PLUGIN"
	cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"state.plugin.MISSING_PLUGIN.drawChance","op":">","value":"0"}]}`)
	cmd.Rewards[0].Config = []byte(`{"baseField":"state.plugin.MISSING_PLUGIN.amount","rate":"0.20","maxAmount":"500.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchBlock || result.AllowGoLive {
		t.Fatalf("conclusion=%s allow=%v, want BLOCK false", result.Conclusion, result.AllowGoLive)
	}
	if !hasPrelaunchCode(result.Items, "PLUGIN_NOT_REGISTERED") {
		t.Fatalf("expected PLUGIN_NOT_REGISTERED item, got %+v", result.Items)
	}
}

func TestValidatePrelaunchBlocksIncompatiblePluginCode(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Type = "FIRST_DEPOSIT"
	cmd.PluginCode = "LOTTERY_TEST"
	cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"state.plugin.LOTTERY_TEST.drawChance","op":">","value":"0"}]}`)
	cmd.Rewards[0].Config = []byte(`{"baseField":"state.plugin.LOTTERY_TEST.amount","rate":"0.20","maxAmount":"500.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)

	result := ValidatePrelaunch(PrelaunchValidationRequest{
		Activity: cmd,
		RegisteredPlugins: []PluginRegistration{{
			PluginCode:              "LOTTERY_TEST",
			CompatibleActivityTypes: []string{"LOTTERY"},
		}},
		TargetAction: "GO_LIVE",
	}, now)

	if result.Conclusion != PrelaunchBlock || !hasPrelaunchCode(result.Items, "PLUGIN_INCOMPATIBLE_ACTIVITY_TYPE") {
		t.Fatalf("expected incompatible plugin BLOCK, got %+v", result)
	}
}

func TestValidatePrelaunchAllowsRegisteredCompatiblePlugin(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Type = "LOTTERY"
	cmd.PluginCode = "LOTTERY_TEST"
	cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"state.plugin.LOTTERY_TEST.drawChance","op":">","value":"0"}]}`)
	cmd.Rewards[0].Config = []byte(`{"baseField":"state.plugin.LOTTERY_TEST.amount","rate":"0.20","maxAmount":"500.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)

	result := ValidatePrelaunch(PrelaunchValidationRequest{
		Activity: cmd,
		RegisteredPlugins: []PluginRegistration{{
			PluginCode:              "LOTTERY_TEST",
			CompatibleActivityTypes: []string{"LOTTERY"},
		}},
		TargetAction: "GO_LIVE",
	}, now)

	if hasPrelaunchCode(result.Items, "PLUGIN_NOT_REGISTERED") || hasPrelaunchCode(result.Items, "PLUGIN_INCOMPATIBLE_ACTIVITY_TYPE") {
		t.Fatalf("expected plugin validation to pass, got %+v", result.Items)
	}
	if !hasPrelaunchCode(result.Items, "PLUGIN_REGISTERED") {
		t.Fatalf("expected PLUGIN_REGISTERED item, got %+v", result.Items)
	}
}

func validPrelaunchActivity() CreateActivityCommand {
	return CreateActivityCommand{
		TenantID:       "site_001",
		Code:           "FIRST_DEPOSIT_BONUS",
		Name:           "First Deposit Bonus",
		Type:           "FIRST_DEPOSIT",
		Status:         ActivityActive,
		TriggerEvents:  []string{"DEPOSIT_SUCCESS"},
		StartTime:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndTime:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		Timezone:       "UTC",
		Priority:       100,
		ExclusiveGroup: "deposit_bonus",
		Stackable:      false,
		Version:        1,
		Config:         []byte(`{"metadata":{"owner":"growth"}}`),
		Rules: []Rule{{
			Code:    "FIRST_DEPOSIT_AND_AMOUNT",
			Phase:   RulePhaseQualify,
			Type:    RuleTypeCondition,
			Config:  []byte(`{"logic":"AND","conditions":[{"field":"attributes.amount","op":">=","value":"100"}]}`),
			Sort:    1,
			Enabled: true,
			Version: 1,
		}},
		Rewards: []Reward{{
			Code:          "FIRST_DEPOSIT_RATIO",
			Type:          RewardBonus,
			CalculateType: CalculateRatio,
			Config:        []byte(`{"baseField":"attributes.amount","rate":"0.20","maxAmount":"500.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`),
			GrantMode:     GrantAuto,
			GrantChannel:  GrantHTTP,
			Enabled:       true,
			Version:       1,
		}},
	}
}

func hasPrelaunchCode(items []PrelaunchItem, code string) bool {
	for _, item := range items {
		if item.Code == code || strings.Contains(item.Code, code) {
			return true
		}
	}
	return false
}
