package marketing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBusinessActivityTemplatesValidateAgainstCurrentSchema(t *testing.T) {
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		payload  string
		quotas   []string
		formula  string
		compare  string
		noPlugin bool
	}{
		{
			name: "T1 sign in daily ladder",
			payload: templatePayload("TPL_SIGN_IN_DAILY_202605", "SIGN_IN_BONUS", "SIGN_IN_SUCCESS", "sign_in_bonus",
				conditionRuleJSON("T1_CONTINUE_DAYS_POSITIVE", `{"field":"attributes.continueDays","op":">=","value":"1"}`),
				rewardJSON("T1_SIGN_IN_LADDER", "BONUS", "LADDER", `{"field":"attributes.continueDays","ladders":[{"min":"1","max":"6","amount":"2.00"},{"min":"7","max":"999","amount":"8.00"}],"currency":"CNY","flowMultiple":"1","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t1", "DAILY_LIMIT", "USER", "10001:2026-05-29", "1"), quotaJSON("act_t1", "BUDGET", "ACTIVITY", "TOTAL", "5000.00")},
		},
		{
			name: "T2 monthly deposit single-month window",
			payload: monthlyTemplatePayload("TPL_MONTHLY_DEPOSIT_202605", "MONTHLY_DEPOSIT_BONUS", "DEPOSIT_SUCCESS", "monthly_deposit_bonus",
				conditionRuleJSON("T2_AMOUNT_MIN", `{"field":"attributes.amount","op":">=","value":"100.00"}`),
				rewardJSON("T2_MONTHLY_DEPOSIT_RATIO", "BONUS", "RATIO", `{"baseField":"attributes.amount","rate":"0.05","maxAmount":"100.00","currency":"CNY","flowMultiple":"3","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t2", "USER_LIMIT", "USER", "10001:2026-05", "1"), quotaJSON("act_t2", "BUDGET", "ACTIVITY", "TOTAL", "10000.00")},
		},
		{
			name: "T3 redeposit bonus",
			payload: templatePayload("TPL_REDEPOSIT_202605", "REDEPOSIT_BONUS", "DEPOSIT_SUCCESS", "redeposit_bonus",
				conditionRulesJSON("T3_REDEPOSIT_AND_AMOUNT", []string{`{"field":"attributes.depositCount","op":">=","value":"2"}`, `{"field":"attributes.amount","op":">=","value":"100.00"}`}),
				rewardJSON("T3_REDEPOSIT_FIXED", "BONUS", "FIXED", `{"amount":"18.00","currency":"CNY","flowMultiple":"3","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t3", "USER_LIMIT", "USER", "10001", "1"), quotaJSON("act_t3", "BUDGET", "ACTIVITY", "TOTAL", "8000.00")},
		},
		{
			name: "T4 newbie task coupon",
			payload: templatePayload("TPL_NEWBIE_TASK_202605", "NEWBIE_TASK", "TASK_COMPLETED", "newbie_task",
				conditionRulesJSON("T4_TASK_COMPLETED", []string{`{"field":"attributes.taskCode","op":"IN","value":["BIND_PHONE","FIRST_BET"]}`, `{"field":"attributes.completed","op":"=","value":true}`}),
				rewardJSON("T4_NEWBIE_COUPON", "COUPON", "FIXED", `{"couponTemplateId":"NEWBIE_COUPON_10","quantity":"1","targetRef":"https://coupon.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t4", "USER_LIMIT", "USER", "10001:BIND_PHONE", "1"), quotaJSON("act_t4", "STOCK", "ACTIVITY", "T4_NEWBIE_COUPON", "10000")},
		},
		{
			name: "T5 rebate ratio",
			payload: templatePayload("TPL_REBATE_WEEKLY_202605", "REBATE_BONUS", "REBATE_SETTLED", "rebate_bonus",
				conditionRuleJSON("T5_VALID_BET_POSITIVE", `{"field":"attributes.validBetAmount","op":">","value":"0"}`),
				rewardJSON("T5_REBATE_RATIO", "BONUS", "RATIO", `{"baseField":"attributes.validBetAmount","rate":"0.008","maxAmount":"500.00","currency":"CNY","flowMultiple":"1","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t5", "USER_LIMIT", "USER", "10001:2026-W22", "1"), quotaJSON("act_t5", "BUDGET", "ACTIVITY", "TOTAL", "50000.00")},
		},
		{
			name: "T6 invite friend bonus",
			payload: templatePayload("TPL_INVITE_FRIEND_202605", "INVITE_FRIEND_BONUS", "INVITEE_FIRST_DEPOSIT", "invite_friend_bonus",
				conditionRulesJSON("T6_INVITEE_QUALIFIED", []string{`{"field":"attributes.inviteeId","op":"!=","value":""}`, `{"field":"attributes.depositAmount","op":">=","value":"100.00"}`}),
				rewardJSON("T6_INVITER_FIXED", "BONUS", "FIXED", `{"amount":"28.00","currency":"CNY","flowMultiple":"3","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t6", "USER_LIMIT", "USER", "10001:invitee_20001", "1"), quotaJSON("act_t6", "BUDGET", "ACTIVITY", "TOTAL", "30000.00")},
		},
		{
			name: "T7 vip weekly bonus",
			payload: templatePayload("TPL_VIP_WEEKLY_202605", "VIP_WEEKLY_BONUS", "VIP_BONUS_ELIGIBLE", "vip_weekly_bonus",
				conditionRulesJSON("T7_VIP_AND_TURNOVER", []string{`{"field":"facts.VIP.level","op":">=","value":"3"}`, `{"field":"attributes.weeklyTurnover","op":">=","value":"1000.00"}`}),
				rewardJSON("T7_VIP_WEEKLY_LADDER", "BONUS", "LADDER", `{"field":"facts.VIP.level","ladders":[{"min":"3","max":"4","amount":"38.00"},{"min":"5","max":"99","amount":"88.00"}],"currency":"CNY","flowMultiple":"1","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t7", "USER_LIMIT", "USER", "10001:2026-W22", "1"), quotaJSON("act_t7", "BUDGET", "ACTIVITY", "TOTAL", "60000.00")},
		},
		{
			name: "T8 birthday bonus",
			payload: templatePayload("TPL_BIRTHDAY_202605", "BIRTHDAY_BONUS", "BIRTHDAY_BONUS_ELIGIBLE", "birthday_bonus",
				conditionRulesJSON("T8_BIRTHDAY_MONTH_AND_ACTIVE", []string{`{"field":"facts.PROFILE.birthMonthDay","op":"=","value":"05-29"}`, `{"field":"attributes.activeDays30","op":">=","value":"1"}`}),
				rewardJSON("T8_BIRTHDAY_FIXED", "BONUS", "FIXED", `{"amount":"58.00","currency":"CNY","flowMultiple":"1","targetRef":"https://wallet.example.com/reward"}`)),
			quotas: []string{quotaJSON("act_t8", "USER_LIMIT", "USER", "10001:2026", "1"), quotaJSON("act_t8", "BUDGET", "ACTIVITY", "TOTAL", "20000.00")},
		},
		{
			name: "T9 upgrade bonus formula",
			payload: templatePayload("TPL_UPGRADE_BONUS_202605", "UPGRADE_BONUS", "VIP_LEVEL_UPGRADED", "upgrade_bonus",
				formulaRuleJSON("T9_NEW_GT_OLD", "attributes.newLevel - attributes.oldLevel", `{"op":">","value":"0"}`),
				rewardJSON("T9_UPGRADE_LADDER", "BONUS", "LADDER", `{"field":"attributes.newLevel","ladders":[{"min":"2","max":"3","amount":"68.00"},{"min":"4","max":"99","amount":"188.00"}],"currency":"CNY","flowMultiple":"1","targetRef":"https://wallet.example.com/reward"}`)),
			quotas:  []string{quotaJSON("act_t9", "USER_LIMIT", "USER", "10001:4", "1"), quotaJSON("act_t9", "BUDGET", "ACTIVITY", "TOTAL", "40000.00")},
			formula: "attributes.newLevel - attributes.oldLevel",
			compare: `{"op":">","value":"0"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cmd CreateActivityCommand
			if err := json.Unmarshal([]byte(tc.payload), &cmd); err != nil {
				t.Fatalf("unmarshal activity template: %v", err)
			}
			if err := cmd.Validate(now); err != nil {
				t.Fatalf("template activity should validate: %v\npayload=%s", err, tc.payload)
			}
			if cmd.PluginCode != "" {
				t.Fatalf("template must not require ActivityPlugin, got %q", cmd.PluginCode)
			}
			for _, rawQuota := range tc.quotas {
				var quota CreateQuotaCommand
				if err := json.Unmarshal([]byte(rawQuota), &quota); err != nil {
					t.Fatalf("unmarshal quota: %v", err)
				}
				if err := quota.Validate(); err != nil {
					t.Fatalf("quota should validate: %v\nquota=%s", err, rawQuota)
				}
			}
			if tc.formula != "" {
				var cfg formulaRuleConfig
				if err := json.Unmarshal(cmd.Rules[0].Config, &cfg); err != nil {
					t.Fatalf("unmarshal formula rule: %v", err)
				}
				if cfg.Formula != tc.formula {
					t.Fatalf("formula=%q, want %q", cfg.Formula, tc.formula)
				}
				var expected ruleCompare
				if err := json.Unmarshal([]byte(tc.compare), &expected); err != nil {
					t.Fatalf("unmarshal expected compare: %v", err)
				}
				if cfg.Compare == nil || cfg.Compare.Op != expected.Op || string(cfg.Compare.Value) != string(expected.Value) {
					t.Fatalf("compare=%+v, want %+v", cfg.Compare, expected)
				}
			}
		})
	}
}

func templatePayload(code, activityType, triggerEvent, exclusiveGroup, rule, reward string) string {
	return `{
		"tenantId":"site_001",
		"activityCode":"` + code + `",
		"activityName":"` + code + `",
		"activityType":"` + activityType + `",
		"status":"DRAFT",
		"triggerEvents":["` + triggerEvent + `"],
		"startTime":"2026-05-01T00:00:00+08:00",
		"endTime":"2026-12-31T23:59:59+08:00",
		"timezone":"Asia/Shanghai",
		"priority":100,
		"config":{"metadata":{"sourceSystem":"template","bizType":"` + activityType + `"}},
		"exclusiveGroup":"` + exclusiveGroup + `",
		"stackable":false,
		"rules":[` + rule + `],
		"rewards":[` + reward + `]
	}`
}

func monthlyTemplatePayload(code, activityType, triggerEvent, exclusiveGroup, rule, reward string) string {
	return `{
		"tenantId":"site_001",
		"activityCode":"` + code + `",
		"activityName":"` + code + `",
		"activityType":"` + activityType + `",
		"status":"DRAFT",
		"triggerEvents":["` + triggerEvent + `"],
		"startTime":"2026-05-01T00:00:00+08:00",
		"endTime":"2026-05-31T23:59:59+08:00",
		"timezone":"Asia/Shanghai",
		"priority":100,
		"config":{"metadata":{"sourceSystem":"payment","bizType":"DEPOSIT","period":"2026-05"}},
		"exclusiveGroup":"` + exclusiveGroup + `",
		"stackable":false,
		"rules":[` + rule + `],
		"rewards":[` + reward + `]
	}`
}

func conditionRuleJSON(code, condition string) string {
	return conditionRulesJSON(code, []string{condition})
}

func conditionRulesJSON(code string, conditions []string) string {
	out := `{"ruleCode":"` + code + `","rulePhase":"QUALIFY","ruleType":"CONDITION","ruleConfig":{"logic":"AND","conditions":[`
	for i, condition := range conditions {
		if i > 0 {
			out += ","
		}
		out += condition
	}
	return out + `]},"sort":1,"enabled":true,"version":1}`
}

func formulaRuleJSON(code, formula, compare string) string {
	return `{"ruleCode":"` + code + `","rulePhase":"QUALIFY","ruleType":"FORMULA","ruleConfig":{"formula":"` + formula + `","resultType":"DECIMAL","compare":` + compare + `},"sort":1,"enabled":true,"version":1}`
}

func rewardJSON(code, rewardType, calculateType, config string) string {
	return `{"rewardCode":"` + code + `","rewardType":"` + rewardType + `","calculateType":"` + calculateType + `","rewardConfig":` + config + `,"grantMode":"AUTO","grantChannel":"HTTP","enabled":true,"version":1}`
}

func quotaJSON(activityID, quotaType, scope, scopeKey, totalValue string) string {
	return `{"activityId":"` + activityID + `","quotaType":"` + quotaType + `","scope":"` + scope + `","scopeKey":"` + scopeKey + `","totalValue":"` + totalValue + `"}`
}
