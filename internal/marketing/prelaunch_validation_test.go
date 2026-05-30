package marketing

import (
	"encoding/json"
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

func TestCreateActivityCommandValidatesProbabilityTotalWeight(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards[0].CalculateType = CalculateProbability
	cmd.Rewards[0].Config = []byte(`{
		"currency":"USD",
		"targetRef":"https://wallet.example.com/reward",
		"items":[
			{"itemCode":"A","weight":"80","amount":"10"},
			{"itemCode":"EMPTY","weight":"0","amount":"0"},
			{"itemCode":"B","weight":"20","amount":"20"}
		]
	}`)
	if err := cmd.Validate(now); err != nil {
		t.Fatalf("expected probability weight total 100 with zero item to validate: %v", err)
	}

	cmd.Rewards[0].Config = []byte(`{
		"currency":"USD",
		"targetRef":"https://wallet.example.com/reward",
		"items":[
			{"itemCode":"A","weight":"80","amount":"10"},
			{"itemCode":"B","weight":"19.9999","amount":"20"}
		]
	}`)
	if err := cmd.Validate(now); err == nil || !strings.Contains(err.Error(), "total weight must equal 100.0000") {
		t.Fatalf("expected probability total weight validation error, got %v", err)
	}
}

func TestCreateActivityCommandAllowsMQRewardTargets(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	for name, targetRef := range map[string]string{
		"kafka":           "kafka://reward.commands",
		"rabbit exchange": "rabbitmq://reward-exchange/reward.commands",
		"rabbit default":  "rabbitmq:///reward.commands",
		"pulsar":          "pulsar://tenant-a/rewards/commands",
	} {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rewards[0].GrantChannel = GrantMQ
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"amount":"10.00","currency":"USD","targetRef":"` + targetRef + `"}`)

			if err := cmd.Validate(now); err != nil {
				t.Fatalf("expected MQ targetRef to validate: %v", err)
			}
		})
	}
}

func TestCreateActivityCommandRejectsInvalidMQRewardTargets(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	for name, targetRef := range map[string]string{
		"http":        "https://wallet.example.com/reward",
		"credentials": "kafka://user:pass@reward.commands",
		"missing":     "kafka://",
		"rabbit":      "rabbitmq://reward-exchange",
		"pulsar":      "pulsar://tenant-a/rewards",
	} {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rewards[0].GrantChannel = GrantMQ
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"amount":"10.00","currency":"USD","targetRef":"` + targetRef + `"}`)

			if err := cmd.Validate(now); err == nil || !strings.Contains(err.Error(), "targetRef") {
				t.Fatalf("expected MQ targetRef validation error, got %v", err)
			}
		})
	}
}

func TestValidatePrelaunchRequiresApprovalForAutoFundRewardWithoutApprovalFlow(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, Actor: "operator", TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchApprovalRequired || result.AllowGoLive {
		t.Fatalf("conclusion=%s allow=%v, want APPROVAL_REQUIRED false", result.Conclusion, result.AllowGoLive)
	}
	if !result.RequireApproval || len(result.HighRiskItems) == 0 {
		t.Fatalf("expected approval and high-risk items, got %+v", result)
	}
	if !hasPrelaunchCode(result.Items, "HIGH_RISK_MISSING_BUDGET") {
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

	if !result.AllowGoLive || result.Conclusion != PrelaunchWarn {
		t.Fatalf("expected approved high-risk request with stock warning to allow go-live, got %+v", result)
	}
	if !hasPrelaunchCode(result.Items, "HIGH_RISK_AUTO_FUND_REWARD") {
		t.Fatalf("expected high-risk auto fund item, got %+v", result.Items)
	}
}

func TestValidatePrelaunchUsesAvailableBudgetAfterUsedAndFrozen(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards[0].CalculateType = CalculateFixed
	cmd.Rewards[0].Config = []byte(`{"amount":"50.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)

	result := ValidatePrelaunch(PrelaunchValidationRequest{
		Activity: cmd,
		Quotas: []Quota{{
			Type:        QuotaBudget,
			Scope:       QuotaScopeActivity,
			ScopeKey:    "TOTAL",
			TotalValue:  "100.00",
			UsedValue:   "30.00",
			FrozenValue: "30.00",
		}},
		TargetAction: "GO_LIVE",
	}, now)

	if result.Conclusion != PrelaunchBlock || !hasPrelaunchCode(result.Items, "BUDGET_EXCEEDED") {
		t.Fatalf("expected budget exceeded against available budget, got %+v", result)
	}
}

func TestValidatePrelaunchRequiresStockForInventoryRewardType(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards[0] = Reward{
		Code:          "WELCOME_COUPON",
		Type:          RewardCoupon,
		CalculateType: CalculateFixed,
		Config:        []byte(`{"couponTemplateId":"WELCOME_10","quantity":"1","targetRef":"https://coupon.example.com/reward"}`),
		GrantMode:     GrantAuto,
		GrantChannel:  GrantHTTP,
		Enabled:       true,
		Version:       1,
	}

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchBlock || !hasPrelaunchCode(result.Items, "STOCK_MISSING") {
		t.Fatalf("expected coupon reward to require stock quota, got %+v", result)
	}
}

func TestValidatePrelaunchWarnOnlyStillAllowsGoLive(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards[0] = Reward{
		Code:          "WELCOME_POINTS",
		Type:          RewardPoint,
		CalculateType: CalculateFixed,
		Config:        []byte(`{"points":"10","unit":"POINT","targetRef":"https://points.example.com/reward"}`),
		GrantMode:     GrantAuto,
		GrantChannel:  GrantHTTP,
		Enabled:       true,
		Version:       1,
	}

	result := ValidatePrelaunch(PrelaunchValidationRequest{Activity: cmd, TargetAction: "GO_LIVE"}, now)

	if result.Conclusion != PrelaunchWarn || !result.AllowGoLive {
		t.Fatalf("conclusion=%s allow=%v, want WARN true; items=%+v", result.Conclusion, result.AllowGoLive, result.Items)
	}
	if result.AuditSummary.Conclusion != PrelaunchWarn || !result.AuditSummary.AllowGoLive {
		t.Fatalf("audit conclusion=%s allow=%v, want WARN true", result.AuditSummary.Conclusion, result.AuditSummary.AllowGoLive)
	}
	if !hasPrelaunchCode(result.Items, "BUDGET_OPTIONAL") || !hasPrelaunchCode(result.Items, "STOCK_OPTIONAL") {
		t.Fatalf("expected warning items to remain itemized, got %+v", result.Items)
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
	if !hasPrelaunchCode(result.Items, "PLUGIN_NOT_FOUND") {
		t.Fatalf("expected PLUGIN_NOT_FOUND item, got %+v", result.Items)
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

	if result.Conclusion != PrelaunchBlock || !hasPrelaunchCode(result.Items, "PLUGIN_ACTIVITY_TYPE_UNSUPPORTED") {
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

	if hasPrelaunchCode(result.Items, "PLUGIN_NOT_FOUND") || hasPrelaunchCode(result.Items, "PLUGIN_ACTIVITY_TYPE_UNSUPPORTED") {
		t.Fatalf("expected plugin validation to pass, got %+v", result.Items)
	}
	if !hasPrelaunchCode(result.Items, "PLUGIN_REGISTERED") {
		t.Fatalf("expected PLUGIN_REGISTERED item, got %+v", result.Items)
	}
}

func TestCreateActivityCommandValidatesRuleSchema(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"=","value":"DEPOSIT_SUCCESS"},{"field":"subject.id","op":"=","value":"10001"},{"field":"biz.bizId","op":"=","value":"pay_1"},{"field":"attributes.amount","op":">=","value":"100"}]}`)
	if err := cmd.Validate(now); err != nil {
		t.Fatalf("expected standard event and attributes fields to validate: %v", err)
	}

	for name, mutate := range map[string]func(*CreateActivityCommand){
		"budget phase": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Phase = RulePhase("BUDGET")
		},
		"risk limit budget phase": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Phase = RulePhase("RISK_LIMIT_BUDGET")
		},
		"missing logic": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"conditions":[{"field":"eventType","op":"=","value":"DEPOSIT_SUCCESS"}]}`)
		},
		"empty conditions": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[]}`)
		},
		"unknown config field": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"=","value":"DEPOSIT_SUCCESS"}],"extra":true}`)
		},
		"business short path": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"user.vipLevel","op":"=","value":3}]}`)
		},
		"non standard top-level": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"orderAmount","op":"=","value":"100"}]}`)
		},
		"bad op": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"LIKE","value":"DEPOSIT"}]}`)
		},
		"missing value": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"="}]}`)
		},
		"boolean comparison": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"attributes.isFirstDeposit","op":">","value":true}]}`)
		},
		"in scalar": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"IN","value":"DEPOSIT_SUCCESS"}]}`)
		},
		"in empty": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"IN","value":[]}]}`)
		},
		"disabled invalid schema": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Enabled = false
			cmd.Rules[0].Config = []byte(`{"logic":"AND","conditions":[{"field":"eventType","op":"LIKE","value":"DEPOSIT"}]}`)
		},
		"duplicate rule code": func(cmd *CreateActivityCommand) {
			cmd.Rules = append(cmd.Rules, cmd.Rules[0])
		},
		"negative sort": func(cmd *CreateActivityCommand) {
			cmd.Rules[0].Sort = -1
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := validPrelaunchActivity()
			mutate(&candidate)
			if err := candidate.Validate(now); err == nil {
				t.Fatal("expected rule schema validation error")
			}
		})
	}
}

func TestCreateActivityCommandAcceptsRegisteredFormulaRule(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rules = []Rule{{
		Code:    "FORMULA_RULE",
		Phase:   RulePhaseQualify,
		Type:    RuleTypeFormula,
		Config:  []byte(`{"formula":"attributes.amount * 0.2","resultType":"DECIMAL","compare":{"op":">=","value":"100.00"},"scale":2}`),
		Enabled: true,
		Version: 1,
	}}

	if err := cmd.Validate(now); err != nil {
		t.Fatalf("expected registered FORMULA rule to validate: %v", err)
	}
}

func TestCreateActivityCommandRejectsUnsafeFormulaRules(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	for name, config := range map[string]string{
		"unsupported field":      `{"formula":"orderAmount * 0.2","resultType":"DECIMAL","compare":{"op":">=","value":"100.00"}}`,
		"function call":          `{"formula":"now()","resultType":"DECIMAL","compare":{"op":">=","value":"100.00"}}`,
		"missing compare":        `{"formula":"attributes.amount * 0.2","resultType":"DECIMAL"}`,
		"unsupported resultType": `{"formula":"attributes.amount","resultType":"STRING","compare":{"op":"=","value":"100.00"}}`,
		"bad boolean op":         `{"formula":"attributes.isFirstDeposit","resultType":"BOOLEAN","compare":{"op":">","value":true}}`,
		"bad scale":              `{"formula":"attributes.amount","resultType":"DECIMAL","compare":{"op":">","value":"0"},"scale":9}`,
	} {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rules = []Rule{{
				Code:    "FORMULA_RULE",
				Phase:   RulePhaseQualify,
				Type:    RuleTypeFormula,
				Config:  []byte(config),
				Enabled: true,
				Version: 1,
			}}
			if err := cmd.Validate(now); err == nil {
				t.Fatal("expected unsafe formula validation error")
			}
		})
	}
}

func TestCreateActivityCommandRejectsUnregisteredRuleEvaluatorsAfterSchemaValidation(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	for name, rule := range map[string]Rule{
		"limit": {
			Code:    "LIMIT_RULE",
			Phase:   RulePhaseLimit,
			Type:    RuleTypeLimit,
			Config:  []byte(`{"limitType":"USER_COUNT","scope":"USER","window":"ACTIVITY","max":1,"field":"subject.id"}`),
			Enabled: true,
			Version: 1,
		},
		"script": {
			Code:    "SCRIPT_RULE",
			Phase:   RulePhaseRisk,
			Type:    RuleTypeScript,
			Config:  []byte(`{"scriptRef":"risk_rule_v1","language":"CEL","inputs":["eventType","attributes.amount"],"timeoutMs":1000}`),
			Enabled: true,
			Version: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rules = []Rule{rule}
			if err := cmd.Validate(now); err == nil || !strings.Contains(err.Error(), "evaluator is not registered") {
				t.Fatalf("expected unregistered evaluator rejection, got %v", err)
			}
		})
	}
}

func TestCreateActivityCommandValidatesRewardSchemaBoundaries(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	validConfigs := map[RewardType]string{
		RewardBonus:      `{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`,
		RewardCash:       `{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`,
		RewardPoint:      `{"points":"10","unit":"POINT","targetRef":"https://points.example.com/reward"}`,
		RewardCoupon:     `{"couponTemplateId":"WELCOME_10","quantity":"1","targetRef":"https://coupon.example.com/reward"}`,
		RewardGift:       `{"giftSkuId":"SKU_1","quantity":"1","targetRef":"https://gift.example.com/reward"}`,
		RewardDrawChance: `{"drawPoolCode":"POOL_1","chanceCount":"1","targetRef":"https://draw.example.com/reward"}`,
		RewardMembership: `{"membershipPlanId":"VIP_30","durationDays":"30","targetRef":"https://member.example.com/reward"}`,
	}
	for rewardType, config := range validConfigs {
		t.Run(string(rewardType), func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rewards[0] = Reward{
				Code:          string(rewardType) + "_FIXED",
				Type:          rewardType,
				CalculateType: CalculateFixed,
				Config:        []byte(config),
				GrantMode:     GrantAuto,
				GrantChannel:  GrantHTTP,
				Enabled:       true,
				Version:       1,
			}
			if err := cmd.Validate(now); err != nil {
				t.Fatalf("expected %s reward schema to validate: %v", rewardType, err)
			}
		})
	}

	invalidNonAmountCombos := map[string]struct {
		rewardType    RewardType
		calculateType CalculateType
		config        string
	}{
		"point ratio": {
			rewardType:    RewardPoint,
			calculateType: CalculateRatio,
			config:        `{"points":"10","unit":"POINT","baseField":"attributes.amount","rate":"1","targetRef":"https://points.example.com/reward"}`,
		},
		"coupon ladder": {
			rewardType:    RewardCoupon,
			calculateType: CalculateLadder,
			config:        `{"couponTemplateId":"WELCOME_10","quantity":"1","field":"attributes.amount","ladders":[{"min":"0","max":"100","amount":"1"}],"targetRef":"https://coupon.example.com/reward"}`,
		},
		"gift random": {
			rewardType:    RewardGift,
			calculateType: CalculateRandom,
			config:        `{"giftSkuId":"SKU_1","quantity":"1","minAmount":"1","maxAmount":"2","step":"1","targetRef":"https://gift.example.com/reward"}`,
		},
		"draw chance probability": {
			rewardType:    RewardDrawChance,
			calculateType: CalculateProbability,
			config:        `{"drawPoolCode":"POOL_1","chanceCount":"1","items":[{"itemCode":"A","weight":"100","amount":"0"}],"targetRef":"https://draw.example.com/reward"}`,
		},
		"membership pool split": {
			rewardType:    RewardMembership,
			calculateType: CalculatePoolSplit,
			config:        `{"membershipPlanId":"VIP_30","durationDays":"30","poolAmount":"100","splitBy":"COUNT","countField":"attributes.amount","targetRef":"https://member.example.com/reward"}`,
		},
	}
	for name, tc := range invalidNonAmountCombos {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			cmd.Rewards[0] = Reward{
				Code:          "NON_AMOUNT_INVALID",
				Type:          tc.rewardType,
				CalculateType: tc.calculateType,
				Config:        []byte(tc.config),
				GrantMode:     GrantAuto,
				GrantChannel:  GrantHTTP,
				Enabled:       true,
				Version:       1,
			}
			err := cmd.Validate(now)
			if err == nil || !strings.Contains(err.Error(), "calculateType is unsupported for rewardType") {
				t.Fatalf("expected calculateType matrix rejection, got %v", err)
			}
		})
	}

	for name, mutate := range map[string]func(*CreateActivityCommand){
		"cash flow multiple": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardCash
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)
			cmd.Rewards[0].FlowMultiple = "1"
		},
		"point currency": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardPoint
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"points":"10","unit":"POINT","currency":"USD","targetRef":"https://points.example.com/reward"}`)
		},
		"coupon missing template": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardCoupon
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"quantity":"1","targetRef":"https://coupon.example.com/reward"}`)
		},
		"gift fractional quantity": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardGift
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"giftSkuId":"SKU_1","quantity":"1.5","targetRef":"https://gift.example.com/reward"}`)
		},
		"membership missing duration": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardMembership
			cmd.Rewards[0].CalculateType = CalculateFixed
			cmd.Rewards[0].Config = []byte(`{"membershipPlanId":"VIP","targetRef":"https://member.example.com/reward"}`)
		},
		"non amount unsupported calculate type": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardCoupon
			cmd.Rewards[0].CalculateType = CalculateRatio
			cmd.Rewards[0].Config = []byte(`{"couponTemplateId":"WELCOME_10","quantity":"1","baseField":"attributes.amount","rate":"1","targetRef":"https://coupon.example.com/reward"}`)
		},
		"probability item mixed reward type": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].Type = RewardBonus
			cmd.Rewards[0].CalculateType = CalculateProbability
			cmd.Rewards[0].Config = []byte(`{"currency":"USD","targetRef":"https://wallet.example.com/reward","items":[{"itemCode":"A","weight":"1","rewardType":"POINT","amount":"10"}]}`)
		},
		"pool split alias conflict": func(cmd *CreateActivityCommand) {
			cmd.Rewards[0].CalculateType = CalculatePoolSplit
			cmd.Rewards[0].Config = []byte(`{"poolAmount":"100","splitBy":"COUNT","splitMode":"WEIGHT","participantCountField":"attributes.amount","currency":"USD","targetRef":"https://wallet.example.com/reward"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := validPrelaunchActivity()
			mutate(&cmd)
			if err := cmd.Validate(now); err == nil {
				t.Fatal("expected reward schema validation error")
			}
		})
	}
}

func TestCreateActivityCommandAcceptsRewardDecimalJSONNumbers(t *testing.T) {
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	cmd := validPrelaunchActivity()
	cmd.Rewards[0].Config = []byte(`{
		"baseField":"attributes.amount",
		"rate":0.20,
		"maxAmount":500.00,
		"currency":"USD",
		"targetRef":"https://wallet.example.com/reward"
	}`)

	if err := cmd.Validate(now); err != nil {
		t.Fatalf("expected JSON number decimal reward config to validate: %v", err)
	}
}

func TestCreateQuotaCommandValidatesDecimalAndIntegerSemantics(t *testing.T) {
	var budget CreateQuotaCommand
	if err := json.Unmarshal([]byte(`{"activityId":"act_1","quotaType":"BUDGET","scope":"ACTIVITY","scopeKey":"TOTAL","totalValue":100.25}`), &budget); err != nil {
		t.Fatalf("unmarshal budget quota: %v", err)
	}
	if budget.TotalValue != "100.25" {
		t.Fatalf("expected totalValue to preserve JSON number text, got %q", budget.TotalValue)
	}
	if err := budget.Validate(); err != nil {
		t.Fatalf("expected decimal budget quota to validate: %v", err)
	}

	stock := CreateQuotaCommand{ActivityID: "act_1", Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1.5"}
	if err := stock.Validate(); err == nil {
		t.Fatal("expected fractional stock quota to be rejected")
	}
	userLimit := CreateQuotaCommand{ActivityID: "act_1", Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: "user_1", TotalValue: "2.0000"}
	if err := userLimit.Validate(); err != nil {
		t.Fatalf("expected integer decimal user limit to validate: %v", err)
	}
	zeroStock := CreateQuotaCommand{ActivityID: "act_1", Type: QuotaStock, Scope: QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "0.0000"}
	if err := zeroStock.Validate(); err != nil {
		t.Fatalf("expected zero stock quota to validate as immediate cap: %v", err)
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
