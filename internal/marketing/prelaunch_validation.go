package marketing

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

type PrelaunchConclusion string
type PrelaunchSeverity string
type PrelaunchItemResult string
type ApprovalStatus string

const (
	PrelaunchPass             PrelaunchConclusion = "PASS"
	PrelaunchWarn             PrelaunchConclusion = "WARN"
	PrelaunchBlock            PrelaunchConclusion = "BLOCK"
	PrelaunchApprovalRequired PrelaunchConclusion = "APPROVAL_REQUIRED"

	PrelaunchInfo     PrelaunchSeverity = "INFO"
	PrelaunchWarning  PrelaunchSeverity = "WARN"
	PrelaunchError    PrelaunchSeverity = "ERROR"
	PrelaunchHighRisk PrelaunchSeverity = "HIGH_RISK"

	PrelaunchItemPassed PrelaunchItemResult = "PASSED"
	PrelaunchItemWarn   PrelaunchItemResult = "WARN"
	PrelaunchItemFailed PrelaunchItemResult = "FAILED"

	ApprovalNone      ApprovalStatus = "NONE"
	ApprovalPending   ApprovalStatus = "PENDING"
	ApprovalApproved  ApprovalStatus = "APPROVED"
	ApprovalRejected  ApprovalStatus = "REJECTED"
	ApprovalWithdrawn ApprovalStatus = "WITHDRAWN"
)

type ApprovalSnapshot struct {
	Status          ApprovalStatus `json:"status"`
	Submitter       string         `json:"submitter,omitempty"`
	Approver        string         `json:"approver,omitempty"`
	Opinion         string         `json:"opinion,omitempty"`
	ActivityVersion int            `json:"activityVersion,omitempty"`
	ReviewedAt      *time.Time     `json:"reviewedAt,omitempty"`
}

type PrelaunchValidationRequest struct {
	Activity           CreateActivityCommand `json:"activity"`
	Quotas             []Quota               `json:"quotas,omitempty"`
	ExistingActivities []Activity            `json:"existingActivities,omitempty"`
	RegisteredPlugins  []PluginRegistration  `json:"registeredPlugins,omitempty"`
	Approval           ApprovalSnapshot      `json:"approval,omitempty"`
	Actor              string                `json:"actor,omitempty"`
	Reason             string                `json:"reason,omitempty"`
	TargetAction       string                `json:"targetAction,omitempty"`
}

type PluginRegistration struct {
	PluginCode              string   `json:"pluginCode"`
	CompatibleActivityTypes []string `json:"compatibleActivityTypes,omitempty"`
}

type PrelaunchValidationResult struct {
	Conclusion      PrelaunchConclusion `json:"conclusion"`
	AllowGoLive     bool                `json:"allowGoLive"`
	RequireApproval bool                `json:"requireApproval"`
	Items           []PrelaunchItem     `json:"items"`
	HighRiskItems   []HighRiskItem      `json:"highRiskItems,omitempty"`
	Approval        ApprovalSnapshot    `json:"approval"`
	AuditSummary    PrelaunchAudit      `json:"auditSummary"`
}

type PrelaunchItem struct {
	Code         string              `json:"code"`
	Name         string              `json:"name"`
	Severity     PrelaunchSeverity   `json:"severity"`
	Result       PrelaunchItemResult `json:"result"`
	Reason       string              `json:"reason"`
	Fields       []string            `json:"fields,omitempty"`
	CurrentValue any                 `json:"currentValue,omitempty"`
	Expected     any                 `json:"expected,omitempty"`
	Suggestion   string              `json:"suggestion,omitempty"`
}

type HighRiskItem struct {
	Code             string   `json:"code"`
	Reason           string   `json:"reason"`
	Fields           []string `json:"fields,omitempty"`
	ApprovalRequired bool     `json:"approvalRequired"`
}

type PrelaunchAudit struct {
	Actor           string              `json:"actor,omitempty"`
	ActivityCode    string              `json:"activityCode"`
	ActivityVersion int                 `json:"activityVersion"`
	TargetAction    string              `json:"targetAction,omitempty"`
	Conclusion      PrelaunchConclusion `json:"conclusion"`
	AllowGoLive     bool                `json:"allowGoLive"`
	RequireApproval bool                `json:"requireApproval"`
	ItemCount       int                 `json:"itemCount"`
	HighRiskCount   int                 `json:"highRiskCount"`
	ApprovalStatus  ApprovalStatus      `json:"approvalStatus"`
}

func ValidatePrelaunch(req PrelaunchValidationRequest, now time.Time) PrelaunchValidationResult {
	builder := prelaunchBuilder{request: req, now: now}
	builder.checkSchema()
	builder.checkPlugin()
	builder.checkBudget()
	builder.checkStock()
	builder.checkConflicts()
	builder.checkHighRisk()
	builder.checkApproval()
	return builder.result()
}

func (b *prelaunchBuilder) checkPlugin() {
	pluginCode := strings.TrimSpace(b.request.Activity.PluginCode)
	if pluginCode == "" {
		b.addPass("PLUGIN_OPTIONAL", "Activity plugin validation", []string{"pluginCode"})
		return
	}
	for _, registered := range b.request.RegisteredPlugins {
		if registered.PluginCode != pluginCode {
			continue
		}
		if len(registered.CompatibleActivityTypes) > 0 && !slices.Contains(registered.CompatibleActivityTypes, b.request.Activity.Type) {
			b.addError("PLUGIN_INCOMPATIBLE_ACTIVITY_TYPE", "Activity plugin validation", "pluginCode is not compatible with activityType", []string{"pluginCode", "activityType"}, pluginCode, "choose a compatible plugin or activityType")
			return
		}
		b.addPass("PLUGIN_REGISTERED", "Activity plugin validation", []string{"pluginCode"})
		return
	}
	b.addError("PLUGIN_NOT_REGISTERED", "Activity plugin validation", "pluginCode is not registered", []string{"pluginCode"}, pluginCode, "register the plugin before create, update, or go-live validation")
}

type prelaunchBuilder struct {
	request       PrelaunchValidationRequest
	now           time.Time
	items         []PrelaunchItem
	highRiskItems []HighRiskItem
}

func (b *prelaunchBuilder) checkSchema() {
	cmd := b.request.Activity
	if err := cmd.Validate(b.now); err != nil {
		b.addError("CONFIG_SCHEMA", "JSON Schema and field validation", err.Error(), []string{"activity"}, nil, "fix invalid activity, rule, reward, URL, Decimal, or enum fields")
		return
	}
	b.addPass("CONFIG_SCHEMA", "JSON Schema and field validation", []string{"activity"})
}

func (b *prelaunchBuilder) checkBudget() {
	exposure, deterministic, fields := maxRewardExposure(b.request.Activity.Rewards)
	budget, hasBudget, budgetOK := b.activityBudget()
	if hasBudget && !budgetOK {
		b.addError("BUDGET_SCHEMA", "Budget quota validation", "activity budget quota totalValue must be a non-negative Decimal", []string{"quotas"}, nil, "fix BUDGET/ACTIVITY/TOTAL quota")
		return
	}
	if hasBudget {
		if !deterministic {
			b.addHighRisk("BUDGET_EXPOSURE_UNKNOWN", "reward maximum exposure cannot be fully determined and needs approval", fields)
			b.addItem("BUDGET_EXPOSURE_UNKNOWN", "Budget exposure validation", PrelaunchHighRisk, PrelaunchItemFailed, "reward maximum exposure cannot be fully determined", fields, nil, "configure upper bounds or submit for second-level approval")
			return
		}
		if exposure.Cmp(budget) > 0 {
			b.addError("BUDGET_EXCEEDED", "Budget exposure validation", "maximum reward exposure exceeds activity budget", fields, map[string]string{"exposure": exposure.String(), "budget": budget.String()}, "lower reward exposure or raise approved budget")
			return
		}
		b.addPass("BUDGET_EXPOSURE", "Budget exposure validation", fields)
		return
	}
	if hasAutoFundReward(b.request.Activity.Rewards) {
		b.addHighRisk("BUDGET_MISSING_AUTO_FUND", "automatic fund reward has no activity budget quota", []string{"quotas", "rewards"})
		b.addItem("BUDGET_MISSING_AUTO_FUND", "Budget exposure validation", PrelaunchHighRisk, PrelaunchItemFailed, "automatic fund reward has no BUDGET/ACTIVITY/TOTAL quota", []string{"quotas", "rewards"}, nil, "configure total budget or submit for second-level approval")
		return
	}
	b.addItem("BUDGET_OPTIONAL", "Budget exposure validation", PrelaunchWarning, PrelaunchItemWarn, "activity has no total budget quota", []string{"quotas"}, nil, "configure budget quota when budget control is required")
}

func (b *prelaunchBuilder) checkStock() {
	for i, reward := range b.request.Activity.Rewards {
		if !reward.Enabled {
			continue
		}
		stock, exists, ok := b.rewardStock(reward.Code)
		field := fmt.Sprintf("rewards[%d]", i)
		if !ok {
			b.addError("STOCK_SCHEMA", "Stock quota validation", "stock quota totalValue must be a non-negative Decimal", []string{"quotas", field}, nil, "fix STOCK/ACTIVITY/rewardCode quota")
			return
		}
		if requiresStock(reward.CalculateType) {
			if !exists {
				b.addError("STOCK_MISSING", "Stock quota validation", "stock-sensitive reward is missing activity stock quota", []string{"quotas", field}, reward.Code, "add STOCK/ACTIVITY quota for this reward")
				return
			}
			if !stock.IsPositive() {
				b.addError("STOCK_EMPTY", "Stock quota validation", "stock-sensitive reward has zero stock", []string{"quotas", field}, reward.Code, "increase stock before go-live")
				return
			}
			continue
		}
		if !exists {
			b.addItem("STOCK_OPTIONAL", "Stock quota validation", PrelaunchWarning, PrelaunchItemWarn, "reward has no stock quota and will be treated as unlimited", []string{"quotas", field}, reward.Code, "configure stock quota if this reward is inventory-controlled")
			return
		}
	}
	b.addPass("STOCK_AVAILABLE", "Stock quota validation", []string{"quotas", "rewards"})
}

func (b *prelaunchBuilder) checkConflicts() {
	cmd := b.request.Activity
	for _, existing := range b.request.ExistingActivities {
		if existing.TenantID != cmd.TenantID || existing.Code == cmd.Code {
			continue
		}
		if existing.Status != ActivityScheduled && existing.Status != ActivityActive {
			continue
		}
		if !timeWindowsOverlap(cmd.StartTime, cmd.EndTime, existing.StartTime, existing.EndTime) {
			continue
		}
		if !stringSlicesOverlap(cmd.TriggerEvents, existing.TriggerEvents) {
			continue
		}
		if cmd.ExclusiveGroup != "" && cmd.ExclusiveGroup == existing.ExclusiveGroup && (!cmd.Stackable || !existing.Stackable) {
			if cmd.Priority == existing.Priority {
				b.addError("EXCLUSIVE_GROUP_CONFLICT", "Exclusive group conflict validation", "same exclusive group, overlapping time and trigger events, and equal priority cannot decide one winner", []string{"exclusiveGroup", "triggerEvents", "startTime", "endTime", "priority"}, existing.Code, "change exclusive group, time window, trigger events, stackable flag, or priority")
				return
			}
			b.addItem("EXCLUSIVE_GROUP_PRIORITY", "Exclusive group conflict validation", PrelaunchWarning, PrelaunchItemWarn, "same exclusive group overlaps but priority can decide ordering", []string{"exclusiveGroup", "priority"}, existing.Code, "confirm priority is intended")
			return
		}
		if !cmd.Stackable && !existing.Stackable {
			b.addError("TIME_WINDOW_CONFLICT", "Activity time conflict validation", "non-stackable activities overlap for the same tenant and trigger event", []string{"triggerEvents", "startTime", "endTime", "stackable"}, existing.Code, "adjust time window, trigger events, or stackable policy")
			return
		}
	}
	b.addPass("ACTIVITY_CONFLICT", "Activity time and exclusive-group validation", []string{"tenantId", "triggerEvents", "startTime", "endTime", "exclusiveGroup"})
}

func (b *prelaunchBuilder) checkHighRisk() {
	for i, reward := range b.request.Activity.Rewards {
		if !reward.Enabled {
			continue
		}
		field := fmt.Sprintf("rewards[%d]", i)
		if reward.GrantMode == GrantAuto && (reward.Type == RewardBonus || reward.Type == RewardCash) {
			b.addHighRisk("AUTO_FUND_REWARD", "automatic fund reward requires second-level approval", []string{field, "grantMode", "rewardType"})
		}
		switch reward.CalculateType {
		case CalculateRandom, CalculateProbability, CalculatePoolSplit:
			b.addHighRisk("VARIABLE_REWARD", "random, probability, or pool-split reward requires second-level approval", []string{field, "calculateType"})
		}
	}
	if len(b.highRiskItems) == 0 {
		b.addPass("HIGH_RISK", "High-risk configuration validation", []string{"rewards", "quotas"})
		return
	}
	for _, highRisk := range b.highRiskItems {
		if !b.hasItem(highRisk.Code) {
			b.addItem(highRisk.Code, "High-risk configuration validation", PrelaunchHighRisk, PrelaunchItemFailed, highRisk.Reason, highRisk.Fields, nil, "submit second-level approval before go-live")
		}
	}
}

func (b *prelaunchBuilder) checkApproval() {
	if len(b.highRiskItems) == 0 {
		return
	}
	approval := b.request.Approval
	switch approval.Status {
	case ApprovalApproved:
		if approval.ActivityVersion != b.activityVersion() {
			b.addError("APPROVAL_VERSION_MISMATCH", "Second-level approval validation", "approval is not valid for current activity version", []string{"approval.activityVersion", "activity.version"}, approval.ActivityVersion, "submit approval again for this version")
			return
		}
		if strings.TrimSpace(approval.Submitter) != "" && approval.Submitter == approval.Approver {
			b.addError("APPROVAL_SELF_APPROVED", "Second-level approval validation", "approver cannot be the same as submitter", []string{"approval.submitter", "approval.approver"}, approval.Approver, "use a different second-level approver")
			return
		}
		b.addPass("APPROVAL_VALID", "Second-level approval validation", []string{"approval"})
	case ApprovalRejected:
		b.addError("APPROVAL_REJECTED", "Second-level approval validation", "approval was rejected", []string{"approval.status"}, approval.Status, "modify configuration and resubmit approval")
	default:
		b.addItem("APPROVAL_REQUIRED", "Second-level approval validation", PrelaunchHighRisk, PrelaunchItemFailed, "high-risk configuration has not been approved", []string{"approval.status"}, approval.Status, "submit and pass second-level approval")
	}
}

func (b *prelaunchBuilder) result() PrelaunchValidationResult {
	hasError := false
	hasHighRisk := false
	hasWarn := false
	for _, item := range b.items {
		switch item.Severity {
		case PrelaunchError:
			hasError = true
		case PrelaunchHighRisk:
			hasHighRisk = true
		case PrelaunchWarning:
			hasWarn = true
		}
	}
	approved := b.request.Approval.Status == ApprovalApproved && b.request.Approval.ActivityVersion == b.activityVersion()
	conclusion := PrelaunchPass
	allow := true
	requireApproval := len(b.highRiskItems) > 0
	switch {
	case hasError:
		conclusion = PrelaunchBlock
		allow = false
	case hasHighRisk && !approved:
		conclusion = PrelaunchApprovalRequired
		allow = false
	case hasWarn:
		conclusion = PrelaunchWarn
	default:
		conclusion = PrelaunchPass
	}
	audit := PrelaunchAudit{
		Actor:           b.request.Actor,
		ActivityCode:    b.request.Activity.Code,
		ActivityVersion: b.activityVersion(),
		TargetAction:    b.request.TargetAction,
		Conclusion:      conclusion,
		AllowGoLive:     allow,
		RequireApproval: requireApproval,
		ItemCount:       len(b.items),
		HighRiskCount:   len(b.highRiskItems),
		ApprovalStatus:  b.request.Approval.Status,
	}
	return PrelaunchValidationResult{
		Conclusion:      conclusion,
		AllowGoLive:     allow,
		RequireApproval: requireApproval,
		Items:           b.items,
		HighRiskItems:   b.highRiskItems,
		Approval:        b.request.Approval,
		AuditSummary:    audit,
	}
}

func (b *prelaunchBuilder) activityVersion() int {
	if b.request.Activity.Version > 0 {
		return b.request.Activity.Version
	}
	return 1
}

func (b *prelaunchBuilder) activityBudget() (decimal.Decimal, bool, bool) {
	for _, quota := range b.request.Quotas {
		if quota.Type == QuotaBudget && quota.Scope == QuotaScopeActivity && quota.ScopeKey == "TOTAL" {
			value, err := decimal.Parse(quota.TotalValue)
			if err != nil || value.IsNegative() {
				return decimal.Decimal{}, true, false
			}
			return value, true, true
		}
	}
	return decimal.Zero(), false, true
}

func (b *prelaunchBuilder) rewardStock(rewardCode string) (decimal.Decimal, bool, bool) {
	for _, quota := range b.request.Quotas {
		if quota.Type == QuotaStock && quota.Scope == QuotaScopeActivity && quota.ScopeKey == rewardCode {
			value, err := decimal.Parse(quota.TotalValue)
			if err != nil || value.IsNegative() {
				return decimal.Decimal{}, true, false
			}
			return value, true, true
		}
	}
	return decimal.Zero(), false, true
}

func (b *prelaunchBuilder) addPass(code, name string, fields []string) {
	b.addItem(code, name, PrelaunchInfo, PrelaunchItemPassed, "validation passed", fields, nil, "")
}

func (b *prelaunchBuilder) addError(code, name, reason string, fields []string, current any, suggestion string) {
	b.addItem(code, name, PrelaunchError, PrelaunchItemFailed, reason, fields, current, suggestion)
}

func (b *prelaunchBuilder) addHighRisk(code, reason string, fields []string) {
	for _, item := range b.highRiskItems {
		if item.Code == code && slices.Equal(item.Fields, fields) {
			return
		}
	}
	b.highRiskItems = append(b.highRiskItems, HighRiskItem{Code: code, Reason: reason, Fields: fields, ApprovalRequired: true})
}

func (b *prelaunchBuilder) addItem(code, name string, severity PrelaunchSeverity, result PrelaunchItemResult, reason string, fields []string, current any, suggestion string) {
	b.items = append(b.items, PrelaunchItem{
		Code:         code,
		Name:         name,
		Severity:     severity,
		Result:       result,
		Reason:       reason,
		Fields:       fields,
		CurrentValue: current,
		Expected:     expectedForSeverity(severity),
		Suggestion:   suggestion,
	})
}

func (b *prelaunchBuilder) hasItem(code string) bool {
	return slices.ContainsFunc(b.items, func(item PrelaunchItem) bool {
		return item.Code == code
	})
}

func expectedForSeverity(severity PrelaunchSeverity) any {
	switch severity {
	case PrelaunchError:
		return "no blocking validation errors"
	case PrelaunchHighRisk:
		return "second-level approval before go-live"
	case PrelaunchWarning:
		return "operator acknowledgement"
	default:
		return "valid configuration"
	}
}

func maxRewardExposure(rewards []Reward) (decimal.Decimal, bool, []string) {
	total := decimal.Zero()
	deterministic := true
	var fields []string
	for i, reward := range rewards {
		if !reward.Enabled {
			continue
		}
		fields = append(fields, fmt.Sprintf("rewards[%d]", i))
		value, ok := maxRewardExposureOne(reward)
		if !ok {
			deterministic = false
			continue
		}
		total = total.Add(value)
	}
	return total, deterministic, fields
}

func maxRewardExposureOne(reward Reward) (decimal.Decimal, bool) {
	var cfg RewardConfig
	if err := json.Unmarshal(reward.Config, &cfg); err != nil {
		return decimal.Zero(), false
	}
	switch reward.CalculateType {
	case CalculateFixed:
		value, err := decimal.Parse(cfg.Amount)
		return value, err == nil
	case CalculateRatio:
		if strings.TrimSpace(cfg.MaxAmount) == "" {
			return decimal.Zero(), false
		}
		value, err := decimal.Parse(cfg.MaxAmount)
		return value, err == nil
	case CalculateLadder:
		max := decimal.Zero()
		for _, ladder := range cfg.Ladders {
			value, err := decimal.Parse(ladder.Amount)
			if err != nil {
				return decimal.Zero(), false
			}
			if value.Cmp(max) > 0 {
				max = value
			}
		}
		return max, true
	case CalculateRandom:
		value, err := decimal.Parse(cfg.MaxAmount)
		return value, err == nil
	case CalculateProbability:
		max := decimal.Zero()
		for _, item := range cfg.Items {
			value, err := decimal.Parse(item.Amount)
			if err != nil {
				return decimal.Zero(), false
			}
			if value.Cmp(max) > 0 {
				max = value
			}
		}
		return max, true
	case CalculatePoolSplit:
		value, err := decimal.Parse(cfg.PoolAmount)
		return value, err == nil
	default:
		return decimal.Zero(), false
	}
}

func hasAutoFundReward(rewards []Reward) bool {
	return slices.ContainsFunc(rewards, func(reward Reward) bool {
		return reward.Enabled && reward.GrantMode == GrantAuto && (reward.Type == RewardBonus || reward.Type == RewardCash)
	})
}

func requiresStock(calculateType CalculateType) bool {
	return calculateType == CalculateProbability || calculateType == CalculatePoolSplit
}

func timeWindowsOverlap(leftStart, leftEnd, rightStart, rightEnd time.Time) bool {
	return leftStart.Before(rightEnd) && rightStart.Before(leftEnd)
}

func stringSlicesOverlap(left, right []string) bool {
	for _, l := range left {
		if slices.Contains(right, l) {
			return true
		}
	}
	return false
}
