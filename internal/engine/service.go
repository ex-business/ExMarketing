package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/fact"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
	activityplugin "github.com/ex-stack/ExMarketing/internal/plugin"
	"github.com/ex-stack/ExMarketing/internal/reward"
	"github.com/ex-stack/ExMarketing/internal/rule"
)

type Service struct {
	repository       marketing.Repository
	ruleRegistry     *rule.Registry
	pluginRegistry   *activityplugin.Registry
	rewardCalculator reward.Calculator
	factClient       *fact.Client
	metrics          *metrics.Registry
	now              func() time.Time
}

func NewService(repository marketing.Repository) *Service {
	service := &Service{
		repository:       repository,
		ruleRegistry:     rule.NewRegistry(),
		pluginRegistry:   activityplugin.NewRegistry(),
		rewardCalculator: reward.ConfigCalculator{},
		factClient:       fact.NewClient(),
		metrics:          metrics.Default,
		now:              func() time.Time { return time.Now().UTC() },
	}
	_ = service.RegisterActivityPlugin(activityplugin.FirstDepositPlugin{})
	_ = service.RegisterActivityPlugin(activityplugin.RedEnvelopePlugin{})
	_ = service.RegisterActivityPlugin(activityplugin.LotteryPlugin{})
	return service
}

type outboxBacklogRepository interface {
	CountOutboxBacklog(ctx context.Context, now time.Time) ([]marketing.OutboxBacklogMetric, error)
	CountOutboxDLQ(ctx context.Context) ([]marketing.OutboxDLQCount, error)
}

func (s *Service) MetricsRegistry() *metrics.Registry {
	return s.metrics
}

func (s *Service) RefreshMetrics(ctx context.Context) error {
	repository, ok := s.repository.(outboxBacklogRepository)
	if !ok || s.metrics == nil {
		return nil
	}
	now := s.now()
	s.metrics.DeleteMetric("exmarketing_outbox_backlog")
	s.metrics.DeleteMetric("exmarketing_outbox_oldest_eligible_age_seconds")
	s.metrics.DeleteMetric("exmarketing_outbox_dlq_count")
	backlog, err := repository.CountOutboxBacklog(ctx, now)
	if err != nil {
		return err
	}
	for _, item := range backlog {
		labels := []metrics.Label{
			metrics.L("tenant_id", item.TenantID),
			metrics.L("target_channel", string(item.TargetChannel)),
			metrics.L("send_status", string(item.SendStatus)),
		}
		s.metrics.SetGauge("exmarketing_outbox_backlog", float64(item.Count), labels...)
		if item.SendStatus != marketing.OutboxDLQClosed {
			s.metrics.SetGauge("exmarketing_outbox_oldest_eligible_age_seconds", item.OldestEligibleAgeSeconds,
				metrics.L("tenant_id", item.TenantID),
				metrics.L("target_channel", string(item.TargetChannel)),
			)
		}
	}
	dlqCounts, err := repository.CountOutboxDLQ(ctx)
	if err != nil {
		return err
	}
	for _, count := range dlqCounts {
		s.metrics.SetGauge("exmarketing_outbox_dlq_count", float64(count.Count),
			metrics.L("tenant_id", count.TenantID),
			metrics.L("target_channel", string(count.TargetChannel)),
		)
	}
	return nil
}

func (s *Service) SaveFactProvider(ctx context.Context, provider marketing.FactProvider) (marketing.FactProvider, error) {
	return s.repository.SaveFactProvider(ctx, provider)
}

func (s *Service) QueryFacts(ctx context.Context, request marketing.FactQueryRequest) (marketing.FactQueryResponse, error) {
	if err := fact.ValidateRequest(request); err != nil {
		return marketing.FactQueryResponse{
			TenantID:  request.TenantID,
			FactCode:  request.FactCode,
			Success:   false,
			ErrorCode: fact.CodeInvalidFactQuery,
			Error:     err.Error(),
		}, nil
	}
	provider, err := s.repository.FindEnabledFactProvider(ctx, request.TenantID, request.FactCode)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return marketing.FactQueryResponse{
				TenantID:  request.TenantID,
				FactCode:  request.FactCode,
				Success:   false,
				ErrorCode: fact.CodeProviderNotConfigured,
				Error:     "fact provider is not configured",
			}, nil
		}
		if errors.Is(err, marketing.ErrFactProviderDisabled) {
			return marketing.FactQueryResponse{
				TenantID:  request.TenantID,
				FactCode:  request.FactCode,
				Success:   false,
				ErrorCode: fact.CodeProviderDisabled,
				Error:     "fact provider is disabled",
			}, nil
		}
		return marketing.FactQueryResponse{}, err
	}
	return s.factClient.Query(ctx, provider, request), nil
}

func (s *Service) RegisterActivityPlugin(plugin activityplugin.ActivityPlugin) error {
	return s.pluginRegistry.Register(plugin)
}

func (s *Service) CreateActivity(ctx context.Context, cmd marketing.CreateActivityCommand) (marketing.Activity, error) {
	now := s.now()
	if err := cmd.Validate(now); err != nil {
		return marketing.Activity{}, err
	}
	if err := s.validateActivityPluginConfig(cmd); err != nil {
		return marketing.Activity{}, err
	}
	if err := s.validateActivityFactReferences(ctx, cmd.TenantID, cmd.Rules); err != nil {
		return marketing.Activity{}, err
	}

	activity := marketing.Activity{
		ID:             marketingID("act"),
		TenantID:       cmd.TenantID,
		Code:           cmd.Code,
		Name:           cmd.Name,
		Type:           cmd.Type,
		Status:         normalizeActivityStatus(cmd.Status, now, cmd.StartTime, cmd.EndTime),
		TriggerEvents:  cmd.TriggerEvents,
		StartTime:      cmd.StartTime,
		EndTime:        cmd.EndTime,
		Timezone:       cmd.Timezone,
		Priority:       cmd.Priority,
		ExclusiveGroup: cmd.ExclusiveGroup,
		Stackable:      cmd.Stackable,
		PluginCode:     strings.TrimSpace(cmd.PluginCode),
		Version:        1,
		Config:         cmd.ConfigPayload(),
		Rules:          normalizeRules(cmd.Rules, now),
		Rewards:        normalizeRewards(cmd.Rewards, now),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if activity.Status == marketing.ActivityScheduled || activity.Status == marketing.ActivityActive {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_REQUIRED: activity must be approved before SCHEDULED or ACTIVE"))
	}

	created, err := s.repository.CreateActivity(ctx, activity)
	if err != nil {
		return marketing.Activity{}, err
	}
	_ = s.audit(ctx, created.TenantID, actorOrSystem(cmd.Operator), "CREATE_ACTIVITY", "ACTIVITY", created.ID, auditSnapshot(nil, created, nil))
	return created, nil
}

func (s *Service) validateActivityFactReferences(ctx context.Context, tenantID string, rules []marketing.Rule) error {
	refs, err := marketing.ExtractFactReferencesValidated(rules)
	if err != nil {
		return errors.Join(marketing.ErrInvalidActivity, err)
	}
	for factCode, fields := range refs {
		provider, err := s.repository.FindEnabledFactProvider(ctx, tenantID, factCode)
		if err != nil {
			if errors.Is(err, marketing.ErrNotFound) {
				return errors.Join(marketing.ErrInvalidActivity, fmt.Errorf("FACT_PROVIDER_NOT_CONFIGURED: fact provider %s is missing", factCode))
			}
			if errors.Is(err, marketing.ErrFactProviderDisabled) {
				return errors.Join(marketing.ErrInvalidActivity, fmt.Errorf("FACT_PROVIDER_DISABLED: fact provider %s is disabled", factCode))
			}
			return err
		}
		if err := provider.Validate(); err != nil {
			return errors.Join(marketing.ErrInvalidActivity, err)
		}
		for field := range fields {
			if _, ok := provider.Mapping[field]; !ok {
				return errors.Join(marketing.ErrInvalidActivity, fmt.Errorf("FACT_FIELD_MISSING: fact provider %s mapping missing %s", factCode, field))
			}
		}
	}
	return nil
}

func (s *Service) ListActivities(ctx context.Context) ([]marketing.Activity, error) {
	activities, err := s.repository.ListActivities(ctx)
	if err != nil {
		return nil, err
	}
	for i := range activities {
		activities[i] = s.hydrateApproval(ctx, activities[i])
	}
	return activities, nil
}

func (s *Service) GetActivity(ctx context.Context, id string) (marketing.Activity, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return marketing.Activity{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, id)
	if err != nil {
		return marketing.Activity{}, err
	}
	return s.hydrateApproval(ctx, activity), nil
}

func (s *Service) UpdateActivity(ctx context.Context, id string, cmd marketing.UpdateActivityCommand) (marketing.Activity, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return marketing.Activity{}, marketing.ErrNotFound
	}
	before, err := s.repository.GetActivity(ctx, id)
	if err != nil {
		return marketing.Activity{}, err
	}
	before = s.hydrateApproval(ctx, before)
	updated, err := cmd.ApplyTo(before, s.now())
	if err != nil {
		return marketing.Activity{}, err
	}
	if cmd.ConfigVersionChanging() && updated.Approval.Status != "" {
		updated.Approval = marketing.ApprovalSnapshot{Status: marketing.ApprovalNone}
	}
	if (updated.Status == marketing.ActivityScheduled || updated.Status == marketing.ActivityActive) && !activityApprovalValid(updated) {
		reason := strings.TrimSpace(cmd.Reason)
		if reason == "" {
			reason = "APPROVAL_REQUIRED"
		}
		_ = s.audit(ctx, before.TenantID, actorOrSystem(cmd.Operator), "ACTIVITY_GO_LIVE_BLOCKED", "ACTIVITY", before.ID, approvalAuditPayload(before, before.Approval, before.Approval, reason, nil))
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_REQUIRED: activity version is not approved"))
	}
	saved, err := s.repository.UpdateActivity(ctx, updated)
	if err != nil {
		return marketing.Activity{}, err
	}
	saved.Approval = updated.Approval
	action := "UPDATE_ACTIVITY"
	if before.Status != updated.Status && updated.Status == marketing.ActivityActive {
		action = "ACTIVITY_GO_LIVE"
	} else if before.Status != updated.Status && updated.Status == marketing.ActivityPaused {
		action = "ACTIVITY_ROLLBACK"
	}
	details := map[string]any{}
	if reason := strings.TrimSpace(cmd.Reason); reason != "" {
		details["reason"] = reason
	}
	if before.Status != updated.Status {
		details["beforeStatus"] = before.Status
		details["afterStatus"] = updated.Status
	}
	if len(details) == 0 {
		details = nil
	}
	_ = s.audit(ctx, saved.TenantID, actorOrSystem(cmd.Operator), action, "ACTIVITY", saved.ID, auditSnapshot(before, saved, details))
	return saved, nil
}

func (s *Service) SubmitActivityApproval(ctx context.Context, id string, cmd marketing.SubmitApprovalCommand) (marketing.Activity, error) {
	operator := strings.TrimSpace(cmd.Operator)
	if operator == "" || cmd.ExpectedVersion <= 0 {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("operator and expectedVersion are required"))
	}
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(id))
	if err != nil {
		return marketing.Activity{}, err
	}
	activity = s.hydrateApproval(ctx, activity)
	if activity.Status == marketing.ActivityEnded {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("ENDED activity cannot be submitted for approval"))
	}
	if activity.Version != cmd.ExpectedVersion {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_VERSION_MISMATCH: activity version changed"))
	}
	if activity.Approval.Status == marketing.ApprovalPendingL1 || activity.Approval.Status == marketing.ApprovalPendingL2 {
		return activity, nil
	}
	if activity.Approval.Status == marketing.ApprovalApproved && activity.Approval.ActivityVersion == activity.Version {
		return activity, nil
	}
	now := s.now()
	before := activity.Approval
	activity.Approval = marketing.ApprovalSnapshot{
		Status:          marketing.ApprovalPendingL1,
		Submitter:       operator,
		Opinion:         strings.TrimSpace(cmd.Reason),
		HighRisk:        activityRequiresSecondApproval(activity),
		ActivityVersion: activity.Version,
		SubmittedAt:     &now,
	}
	saved, err := s.repository.UpdateActivity(ctx, activity)
	if err != nil {
		return marketing.Activity{}, err
	}
	saved.Approval = activity.Approval
	_ = s.audit(ctx, saved.TenantID, operator, "APPROVAL_SUBMIT", "APPROVAL", saved.ID, approvalAuditPayload(saved, before, saved.Approval, strings.TrimSpace(cmd.Reason), nil))
	return saved, nil
}

func (s *Service) ApproveActivity(ctx context.Context, id string, cmd marketing.ApprovalActionCommand) (marketing.Activity, error) {
	operator := strings.TrimSpace(cmd.Operator)
	if operator == "" {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("operator is required"))
	}
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(id))
	if err != nil {
		return marketing.Activity{}, err
	}
	activity = s.hydrateApproval(ctx, activity)
	before := activity.Approval
	if activity.Version != before.ActivityVersion {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_VERSION_MISMATCH: activity version changed"))
	}
	if operator == before.Submitter || operator == before.L1Approver {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_SAME_OPERATOR: approval operators must be different"))
	}
	now := s.now()
	switch before.Status {
	case marketing.ApprovalPendingL1:
		activity.Approval.L1Approver = operator
		activity.Approval.Approver = operator
		if before.HighRisk {
			activity.Approval.Status = marketing.ApprovalPendingL2
		} else {
			activity.Approval.Status = marketing.ApprovalApproved
			activity.Approval.ReviewedAt = &now
		}
	case marketing.ApprovalPendingL2:
		activity.Approval.L2Approver = operator
		activity.Approval.Approver = operator
		activity.Approval.Status = marketing.ApprovalApproved
		activity.Approval.ReviewedAt = &now
	case marketing.ApprovalApproved:
		return activity, nil
	default:
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_STATE_INVALID: approval is not pending"))
	}
	activity.Approval.Opinion = strings.TrimSpace(cmd.Reason)
	saved, err := s.repository.UpdateActivity(ctx, activity)
	if err != nil {
		return marketing.Activity{}, err
	}
	saved.Approval = activity.Approval
	_ = s.audit(ctx, saved.TenantID, operator, "APPROVAL_APPROVE", "APPROVAL", saved.ID, approvalAuditPayload(saved, before, saved.Approval, strings.TrimSpace(cmd.Reason), nil))
	return saved, nil
}

func (s *Service) RejectActivityApproval(ctx context.Context, id string, cmd marketing.ApprovalActionCommand) (marketing.Activity, error) {
	operator := strings.TrimSpace(cmd.Operator)
	reason := strings.TrimSpace(cmd.Reason)
	if operator == "" || reason == "" {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("operator and reason are required"))
	}
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(id))
	if err != nil {
		return marketing.Activity{}, err
	}
	activity = s.hydrateApproval(ctx, activity)
	before := activity.Approval
	if before.Status != marketing.ApprovalPendingL1 && before.Status != marketing.ApprovalPendingL2 {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_STATE_INVALID: approval is not pending"))
	}
	if operator == before.Submitter || operator == before.L1Approver {
		return marketing.Activity{}, errors.Join(marketing.ErrInvalidOperation, errors.New("APPROVAL_SAME_OPERATOR: approval operators must be different"))
	}
	now := s.now()
	activity.Approval.Status = marketing.ApprovalRejected
	activity.Approval.Approver = operator
	activity.Approval.RejectReason = reason
	activity.Approval.Opinion = reason
	activity.Approval.ReviewedAt = &now
	saved, err := s.repository.UpdateActivity(ctx, activity)
	if err != nil {
		return marketing.Activity{}, err
	}
	saved.Approval = activity.Approval
	_ = s.audit(ctx, saved.TenantID, operator, "APPROVAL_REJECT", "APPROVAL", saved.ID, approvalAuditPayload(saved, before, saved.Approval, reason, nil))
	return saved, nil
}

func (s *Service) ListApprovalRecords(ctx context.Context, id string) ([]marketing.ApprovalRecord, error) {
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	audits, err := s.repository.ListAudit(ctx, marketing.AuditQuery{TenantID: activity.TenantID, ResourceID: activity.ID, Limit: 200})
	if err != nil {
		return nil, err
	}
	records := make([]marketing.ApprovalRecord, 0, len(audits))
	for _, audit := range audits {
		if !strings.HasPrefix(audit.Action, "APPROVAL_") && audit.Action != "ACTIVITY_GO_LIVE_BLOCKED" && audit.Action != "ACTIVITY_GO_LIVE_ALLOWED" {
			continue
		}
		record := marketing.ApprovalRecord{
			ID:           audit.ID,
			TenantID:     audit.TenantID,
			ActivityID:   activity.ID,
			ActivityCode: activity.Code,
			Action:       audit.Action,
			Operator:     audit.Actor,
			CreatedAt:    audit.CreatedAt,
		}
		var payload struct {
			BeforeStatus    marketing.ApprovalStatus `json:"beforeStatus"`
			AfterStatus     marketing.ApprovalStatus `json:"afterStatus"`
			ActivityVersion int                      `json:"activityVersion"`
			HighRisk        bool                     `json:"highRisk"`
			Reason          string                   `json:"reason"`
		}
		_ = json.Unmarshal(audit.Payload, &payload)
		record.BeforeStatus = payload.BeforeStatus
		record.AfterStatus = payload.AfterStatus
		record.ActivityVersion = payload.ActivityVersion
		record.HighRisk = payload.HighRisk
		record.Reason = payload.Reason
		records = append(records, record)
	}
	return records, nil
}

func (s *Service) ValidatePrelaunch(ctx context.Context, req marketing.PrelaunchValidationRequest) (marketing.PrelaunchValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return marketing.PrelaunchValidationResult{}, err
	}
	if len(req.RegisteredPlugins) == 0 {
		req.RegisteredPlugins = s.pluginRegistry.Registrations()
	}
	result := marketing.ValidatePrelaunch(req, s.now())
	if err := s.validateActivityFactReferences(ctx, req.Activity.TenantID, req.Activity.Rules); err != nil {
		result.Items = append(result.Items, marketing.PrelaunchItem{
			Code:       "FACT_PROVIDER_REFERENCE",
			Name:       "Fact provider reference validation",
			Severity:   marketing.PrelaunchError,
			Result:     marketing.PrelaunchItemFailed,
			Reason:     err.Error(),
			Fields:     []string{"activity.rules"},
			Suggestion: "configure an enabled HTTP fact provider with mapping for every facts.<factCode> field before saving or go-live validation",
		})
		result.Conclusion = marketing.PrelaunchBlock
		result.AllowGoLive = false
		result.AuditSummary.Conclusion = result.Conclusion
		result.AuditSummary.AllowGoLive = false
		result.AuditSummary.ItemCount = len(result.Items)
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "system"
	}
	_ = s.audit(ctx, req.Activity.TenantID, actor, "PRELAUNCH_VALIDATE", "ACTIVITY", req.Activity.Code, auditSnapshot(nil, result.AuditSummary, nil))
	return result, nil
}

func (s *Service) validateActivityPluginConfig(cmd marketing.CreateActivityCommand) error {
	pluginCode := strings.TrimSpace(cmd.PluginCode)
	if pluginCode == "" {
		return nil
	}
	plugin, ok := s.pluginRegistry.Get(pluginCode)
	if !ok {
		return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginNotRegistered+": pluginCode is not registered"))
	}
	registration, _ := s.pluginRegistry.GetRegistration(pluginCode)
	if registration.Metadata.Disabled {
		return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginDisabled+": pluginCode is disabled"))
	}
	if !s.pluginRegistry.IsCompatible(pluginCode, cmd.Type) {
		return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginIncompatible+": pluginCode is not compatible with activityType"))
	}
	if missing := activityplugin.MissingCapabilities(registration.Metadata.Capabilities, []string{activityplugin.CapabilityConfigValidate, activityplugin.CapabilityContextBuild}); len(missing) > 0 {
		return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginCapabilityMissing+": "+strings.Join(missing, ",")))
	}
	if err := plugin.ValidateConfig(cmd.ConfigPayload()); err != nil {
		return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginConfigInvalid+": "+err.Error()))
	}
	if validator, ok := plugin.(activityplugin.ActivityConfigValidator); ok {
		if err := validator.ValidateActivityConfig(cmd); err != nil {
			return errors.Join(marketing.ErrInvalidActivity, errors.New(activityplugin.ReasonPluginConfigInvalid+": "+err.Error()))
		}
	}
	return nil
}

func (s *Service) CreateQuota(ctx context.Context, cmd marketing.CreateQuotaCommand) (marketing.Quota, error) {
	if err := cmd.Validate(); err != nil {
		return marketing.Quota{}, err
	}
	now := s.now()
	quota := marketing.Quota{
		ID:          marketingID("quota"),
		ActivityID:  cmd.ActivityID,
		Type:        cmd.Type,
		Scope:       cmd.Scope,
		ScopeKey:    cmd.ScopeKey,
		TotalValue:  cmd.TotalValue,
		UsedValue:   "0",
		FrozenValue: "0",
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created, err := s.repository.CreateQuota(ctx, quota)
	if err != nil {
		return marketing.Quota{}, err
	}
	details := map[string]any{"activityId": created.ActivityID}
	_ = s.audit(ctx, strings.TrimSpace(cmd.TenantID), actorOrSystem(cmd.Operator), "CREATE_QUOTA", "QUOTA", created.ID, auditSnapshot(nil, created, details))
	return created, nil
}

func (s *Service) ListAudit(ctx context.Context, query marketing.AuditQuery) ([]marketing.AuditRecord, error) {
	if strings.TrimSpace(query.TenantID) == "" {
		return nil, errors.Join(marketing.ErrInvalidOperation, errors.New("tenantId is required"))
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	return s.repository.ListAudit(ctx, query)
}

func (s *Service) RecordAudit(ctx context.Context, tenantID, actor, action, resource, resourceID string, payload any) error {
	return s.audit(ctx, tenantID, actor, action, resource, resourceID, payload)
}

func (s *Service) ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]marketing.OutboxMessage, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.Join(marketing.ErrInvalidOperation, errors.New("tenantId is required"))
	}
	return s.repository.ListOutboxDLQ(ctx, tenantID, includeClosed, limit)
}

func (s *Service) ListRewardReconciliation(ctx context.Context, query marketing.RewardReconciliationQuery) ([]marketing.RewardReconciliationItem, error) {
	if strings.TrimSpace(query.TenantID) == "" {
		return nil, errors.New("tenantId is required")
	}
	if query.Limit < 0 {
		return nil, errors.New("limit must be non-negative")
	}
	if query.Limit > 500 {
		return nil, errors.New("limit must not exceed 500")
	}
	if strings.TrimSpace(query.SubjectID) != "" && strings.TrimSpace(query.SubjectType) == "" {
		return nil, errors.New("subjectType is required when subjectId is provided")
	}
	if strings.TrimSpace(query.SubjectType) != "" && strings.TrimSpace(query.SubjectID) == "" {
		return nil, errors.New("subjectId is required when subjectType is provided")
	}
	if strings.TrimSpace(query.CommandID) == "" &&
		strings.TrimSpace(query.GrantOrderID) == "" &&
		strings.TrimSpace(query.ActivityID) == "" &&
		strings.TrimSpace(query.SubjectType) == "" &&
		strings.TrimSpace(query.SubjectID) == "" &&
		strings.TrimSpace(query.EventID) == "" &&
		strings.TrimSpace(query.IdempotencyKey) == "" &&
		strings.TrimSpace(query.ParticipationID) == "" &&
		strings.TrimSpace(query.OutboxID) == "" {
		return nil, errors.New("at least one reconciliation locator is required")
	}
	return s.repository.ListRewardReconciliation(ctx, query)
}

func (s *Service) ReplayOutboxDLQ(ctx context.Context, id string, cmd marketing.DLQOperationCommand) (marketing.OutboxMessage, error) {
	if strings.TrimSpace(cmd.TenantID) == "" || strings.TrimSpace(cmd.Operator) == "" || strings.TrimSpace(cmd.Reason) == "" {
		return marketing.OutboxMessage{}, errors.Join(marketing.ErrInvalidOperation, errors.New("tenantId, operator and reason are required"))
	}
	return s.repository.ReplayOutboxDLQ(ctx, cmd.TenantID, id, cmd.Operator, cmd.Reason, s.now())
}

func (s *Service) CloseOutboxDLQ(ctx context.Context, id string, cmd marketing.DLQOperationCommand) (marketing.OutboxMessage, error) {
	if strings.TrimSpace(cmd.TenantID) == "" || strings.TrimSpace(cmd.Operator) == "" || strings.TrimSpace(cmd.Reason) == "" {
		return marketing.OutboxMessage{}, errors.Join(marketing.ErrInvalidOperation, errors.New("tenantId, operator and reason are required"))
	}
	return s.repository.CloseOutboxDLQ(ctx, cmd.TenantID, id, cmd.Operator, cmd.Reason, s.now())
}

func (s *Service) ProcessEvent(ctx context.Context, event marketing.ActivityEvent) (marketing.ProcessEventResult, error) {
	return s.ProcessEventFrom(ctx, event, "openapi")
}

func (s *Service) ProcessEventFrom(ctx context.Context, event marketing.ActivityEvent, actor string) (marketing.ProcessEventResult, error) {
	if err := event.Validate(); err != nil {
		return marketing.ProcessEventResult{}, err
	}
	event = event.Normalized()
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "system"
	}

	now := s.now()
	inbox := marketing.EventInbox{
		ID:             marketingID("inbox"),
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  marketing.InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	inbox, duplicate, err := s.repository.CreateInbox(ctx, inbox)
	if err != nil {
		return marketing.ProcessEventResult{}, err
	}
	if duplicate {
		return marketing.ProcessEventResult{Inbox: inbox, Duplicate: true}, nil
	}
	_ = s.audit(ctx, event.TenantID, actor, "EVENT_IN", "EVENT", inbox.ID, auditSnapshot(nil, event, map[string]any{
		"eventId":        event.EventID,
		"idempotencyKey": event.IdempotencyKey(),
	}))

	activities, err := s.repository.FindMatchingActivities(ctx, event.TenantID, event.EventType)
	if err != nil {
		return marketing.ProcessEventResult{}, err
	}
	for i := range activities {
		activities[i] = s.hydrateApproval(ctx, activities[i])
	}
	sortActivitiesForMatch(activities)
	exclusiveSingleSelect := exclusiveSingleSelectGroups(activities)

	result := marketing.ProcessEventResult{Inbox: inbox}
	matchedExclusive := map[string]string{}
	for _, activity := range activities {
		if decision := matchWindowDecision(activity, event.OccurredAt); decision.Decision != marketing.RuleDecisionPassed {
			continue
		}
		if entryAllowed, entryRuleResult := activityEntryAllowed(activity, event); !entryAllowed {
			participation, err := s.createRejectedParticipation(ctx, inbox, activity, entryRuleResult)
			if err != nil {
				inbox.ProcessStatus = marketing.InboxFailed
				inbox.ErrorMessage = err.Error()
				inbox.UpdatedAt = now
				_ = s.repository.UpdateInbox(ctx, inbox)
				return result, err
			}
			result.Participations = append(result.Participations, participation)
			result.RuleResults = append(result.RuleResults, entryRuleResult)
			continue
		}
		if activity.ExclusiveGroup != "" && matchedExclusive[activity.ExclusiveGroup] != "" && exclusiveSingleSelect[activity.ExclusiveGroup] {
			participation, ruleResult, err := s.createExclusiveRejectedParticipation(ctx, inbox, activity, matchedExclusive[activity.ExclusiveGroup])
			if err != nil {
				inbox.ProcessStatus = marketing.InboxFailed
				inbox.ErrorMessage = err.Error()
				inbox.UpdatedAt = now
				_ = s.repository.UpdateInbox(ctx, inbox)
				return result, err
			}
			result.Participations = append(result.Participations, participation)
			result.RuleResults = append(result.RuleResults, ruleResult)
			continue
		}

		participation, rewardOrders, outboxMessages, ruleResult, qualified, err := s.evaluateActivity(ctx, inbox, activity)
		if err != nil {
			inbox.ProcessStatus = marketing.InboxFailed
			inbox.ErrorMessage = err.Error()
			inbox.UpdatedAt = now
			_ = s.repository.UpdateInbox(ctx, inbox)
			return result, err
		}

		result.Participations = append(result.Participations, participation)
		result.GrantOrders = append(result.GrantOrders, rewardOrders...)
		result.OutboxMessages = append(result.OutboxMessages, outboxMessages...)
		result.RuleResults = append(result.RuleResults, ruleResult)
		if qualified && activity.ExclusiveGroup != "" {
			matchedExclusive[activity.ExclusiveGroup] = activity.Code
		}
	}

	inbox.ProcessStatus = marketing.InboxProcessed
	inbox.UpdatedAt = now
	if err := s.repository.UpdateInbox(ctx, inbox); err != nil {
		return result, err
	}
	result.Inbox = inbox
	return result, nil
}

func (s *Service) SimulateEvent(ctx context.Context, event marketing.ActivityEvent) (marketing.SimulationResult, error) {
	if err := event.Validate(); err != nil {
		return marketing.SimulationResult{}, err
	}
	event = event.Normalized()
	activities, err := s.repository.FindMatchingActivities(ctx, event.TenantID, event.EventType)
	if err != nil {
		return marketing.SimulationResult{}, err
	}
	for i := range activities {
		activities[i] = s.hydrateApproval(ctx, activities[i])
	}
	sortActivitiesForMatch(activities)

	now := s.now()
	result := marketing.SimulationResult{Event: event, SimulatedAt: now}
	var matchPassed []marketing.SimulationActivityResult
	for _, activity := range activities {
		result.Summary.CandidateCount++
		if decision := matchWindowDecision(activity, event.OccurredAt); decision.Decision != marketing.RuleDecisionPassed {
			activityResult := simulationActivityBase(activity, nil)
			activityResult.Decision = decision.Decision
			activityResult.DecisionReasonCode = decision.ReasonCode
			activityResult.DecisionReason = decision.Reason
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
			continue
		}
		if entryAllowed, entryRuleResult := activityEntryAllowed(activity, event); !entryAllowed {
			activityResult := simulationActivityBase(activity, &entryRuleResult)
			activityResult.Decision = entryRuleResult.Decision
			activityResult.DecisionReasonCode = entryRuleResult.ReasonCode
			activityResult.DecisionReason = entryRuleResult.Reason
			result.RuleResults = append(result.RuleResults, entryRuleResult)
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
			continue
		}

		participation, grants, outbox, ruleResult, pluginSnapshot, factQueryResults, rewardPreviews, stockReservations, totalRewardAmount := s.simulateActivity(ctx, event, activity)
		activityResult := simulationActivityBase(activity, &ruleResult)
		activityResult.Decision = ruleResult.Decision
		activityResult.DecisionReasonCode = ruleResult.ReasonCode
		activityResult.DecisionReason = ruleResult.Reason
		normalizeMatchRuleDecision(&activityResult)
		activityResult.Plugin = pluginSnapshot
		activityResult.FactQueryResults = factQueryResults
		activityResult.RewardPreviews = rewardPreviews
		activityResult.ParticipationPreview = &participation
		activityResult.GrantOrderPreviews = grants
		activityResult.OutboxCommandPreviews = outbox
		activityResult.WouldCreateGrantOrders = len(grants) > 0

		if ruleResult.Decision == marketing.RuleDecisionPassed && len(grants) > 0 {
			quotaPreviews, hasQuotaRisk, err := s.quotaImpactPreviews(ctx, activity, event, totalRewardAmount, stockReservations)
			if err != nil {
				return marketing.SimulationResult{}, err
			}
			activityResult.QuotaImpactPreviews = quotaPreviews
			if hasQuotaRisk {
				result.Summary.HasQuotaRisk = true
				activityResult.Decision = marketing.RuleDecisionRejected
				activityResult.DecisionReasonCode = "QUOTA_EXCEEDED"
				activityResult.DecisionReason = "one or more quota impacts would exceed available quota"
				participation.Status = marketing.ParticipationRejected
				participation.RejectReason = "QUOTA_EXCEEDED: one or more quota impacts would exceed available quota"
				activityResult.ParticipationPreview = &participation
				activityResult.GrantOrderPreviews = nil
				activityResult.OutboxCommandPreviews = nil
				activityResult.WouldCreateGrantOrders = false
				grants = nil
				outbox = nil
			}
		}

		result.Participations = append(result.Participations, participation)
		result.GrantOrders = append(result.GrantOrders, grants...)
		result.OutboxMessages = append(result.OutboxMessages, outbox...)
		result.RuleResults = append(result.RuleResults, ruleResult)
		if activityResult.Decision == marketing.RuleDecisionPassed {
			matchPassed = append(matchPassed, activityResult)
			result.Summary.HasRewardableActivity = result.Summary.HasRewardableActivity || len(grants) > 0
		} else {
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
		}
	}
	matched, exclusiveRejected := applyExclusiveGroups(matchPassed)
	result.MatchedActivities = matched
	result.UnmatchedActivities = append(result.UnmatchedActivities, exclusiveRejected...)
	result.Summary.MatchedCount = len(result.MatchedActivities)
	result.Summary.UnmatchedCount = len(result.UnmatchedActivities)
	_ = s.audit(ctx, event.TenantID, "system", "SIMULATION_RUN", "SIMULATION", event.IdempotencyKey(), auditSnapshot(nil, result.Summary, map[string]any{
		"eventId":        event.EventID,
		"idempotencyKey": event.IdempotencyKey(),
	}))
	return result, nil
}

func (s *Service) ApplyRewardCallback(ctx context.Context, callback marketing.RewardCallback) (marketing.RewardCallbackResult, error) {
	callback = callback.Normalized()
	if callback.FinishedAt.IsZero() {
		callback.FinishedAt = s.now()
	}
	if err := callback.Validate(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	callback.FinishedAt = callback.FinishedAt.UTC()
	result, err := s.repository.ApplyRewardCallback(ctx, callback)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	if result.GrantStatus == marketing.GrantFailed && !result.Duplicate {
		_ = s.audit(ctx, result.TenantID, "reward_callback", "GRANT_FAILED", "GRANT_ORDER", result.GrantOrderID, auditSnapshot(nil, nil, map[string]any{
			"grantOrderId":   result.GrantOrderID,
			"commandId":      result.CommandID,
			"idempotencyKey": result.IdempotencyKey,
			"status":         result.GrantStatus,
			"message":        callback.Message,
			"finishedAt":     callback.FinishedAt,
		}))
	}
	return result, nil
}

func (s *Service) ApplyManualGrantDecision(ctx context.Context, cmd marketing.ManualGrantDecisionCommand) (marketing.RewardCallbackResult, error) {
	cmd = cmd.Normalized(s.now())
	if err := cmd.Validate(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	result, err := s.repository.ApplyManualGrantDecision(ctx, cmd)
	if err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	action := "MANUAL_GRANT_CONFIRM_FAILED"
	if cmd.Status == marketing.GrantSuccess {
		action = "MANUAL_GRANT_CONFIRM_SUCCESS"
	}
	if cmd.Status == marketing.GrantClosed {
		action = "MANUAL_GRANT_CLOSE"
	}
	_ = s.audit(ctx, cmd.TenantID, cmd.Operator, action, "GRANT_ORDER", result.GrantOrderID, auditSnapshot(nil, result, map[string]any{
		"grantOrderId":        result.GrantOrderID,
		"billNo":              result.BillNo,
		"idempotencyKey":      result.IdempotencyKey,
		"operator":            cmd.Operator,
		"reason":              cmd.Reason,
		"quotaReservations":   result.Reservations,
		"participationStatus": result.ParticipationStatus,
	}))
	return result, nil
}

func (s *Service) CreateManualGrantExport(ctx context.Context, cmd marketing.ManualGrantExportCreateCommand) (marketing.ManualGrantExportCreateResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	result, err := s.repository.CreateManualGrantExport(ctx, cmd)
	if err != nil {
		return marketing.ManualGrantExportCreateResult{}, err
	}
	_ = s.audit(ctx, cmd.TenantID, cmd.Operator, "MANUAL_GRANT_EXPORT_READY", "MANUAL_GRANT_EXPORT", result.ExportFile.ID, auditSnapshot(nil, result.ExportFile, map[string]any{
		"exportId": result.ExportFile.ID,
		"operator": cmd.Operator,
		"reason":   cmd.Reason,
		"rowCount": result.ExportFile.RowCount,
		"details":  result.Details,
	}))
	return result, nil
}

func (s *Service) ReleaseExpiredQuotaReservations(ctx context.Context, cmd marketing.QuotaTimeoutReleaseCommand) (marketing.QuotaTimeoutReleaseResult, error) {
	if cmd.OlderThan.IsZero() {
		cmd.OlderThan = s.now().Add(-2 * time.Hour)
	}
	cmd.OlderThan = cmd.OlderThan.UTC()
	if cmd.Limit <= 0 {
		cmd.Limit = 100
	}
	if err := cmd.Validate(); err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	result, err := s.repository.ReleaseExpiredQuotaReservations(ctx, cmd)
	if err != nil {
		return marketing.QuotaTimeoutReleaseResult{}, err
	}
	action := "QUOTA_TIMEOUT_RELEASE"
	if cmd.DryRun {
		action = "QUOTA_TIMEOUT_RELEASE_DRY_RUN"
	}
	_ = s.audit(ctx, cmd.TenantID, actorOrSystem(cmd.Operator), action, "QUOTA", cmd.TenantID, auditSnapshot(nil, result.Items, map[string]any{
		"olderThan": cmd.OlderThan,
		"limit":     cmd.Limit,
		"dryRun":    cmd.DryRun,
		"reason":    cmd.Reason,
		"count":     len(result.Items),
	}))
	return result, nil
}

type activityEntryConfig struct {
	Entry *struct {
		Enabled       *bool    `json:"enabled,omitempty"`
		Channels      []string `json:"channels,omitempty"`
		SourceSystems []string `json:"sourceSystems,omitempty"`
		TenantIDs     []string `json:"tenantIds,omitempty"`
		Audience      *struct {
			SubjectTypes []string `json:"subjectTypes,omitempty"`
			Segments     []string `json:"segments,omitempty"`
			RiskLevels   []string `json:"riskLevels,omitempty"`
		} `json:"audience,omitempty"`
	} `json:"entry,omitempty"`
}

func activityEntryAllowed(activity marketing.Activity, event marketing.ActivityEvent) (bool, marketing.ActivityRuleResult) {
	if len(bytes.TrimSpace(activity.Config)) == 0 || bytes.Equal(bytes.TrimSpace(activity.Config), []byte("null")) {
		return true, marketing.ActivityRuleResult{}
	}
	var cfg activityEntryConfig
	decoder := json.NewDecoder(bytes.NewReader(activity.Config))
	if err := decoder.Decode(&cfg); err != nil {
		return false, entryError(activity, "INVALID_MATCH_CONFIG", "activity entry config is invalid")
	}
	if cfg.Entry == nil {
		return true, marketing.ActivityRuleResult{}
	}
	entry := cfg.Entry
	if entry.Enabled != nil && !*entry.Enabled {
		return false, entryRejected(activity, "ENTRY_DISABLED", "activity entry is disabled")
	}
	if len(entry.TenantIDs) > 0 && !containsString(entry.TenantIDs, event.TenantID) {
		return false, entryRejected(activity, "ENTRY_TENANT_NOT_ALLOWED", "event tenantId is not allowed by activity entry")
	}
	if len(entry.SourceSystems) > 0 && !containsString(entry.SourceSystems, event.SourceSystem) {
		return false, entryRejected(activity, "ENTRY_SOURCE_SYSTEM_NOT_ALLOWED", "event sourceSystem is not allowed by activity entry")
	}
	if len(entry.Channels) > 0 && !containsString(entry.Channels, attributeString(event.Attributes, "channel")) {
		return false, entryRejected(activity, "ENTRY_CHANNEL_NOT_ALLOWED", "event channel is not allowed by activity entry")
	}
	if entry.Audience != nil {
		if len(entry.Audience.SubjectTypes) > 0 && !containsString(entry.Audience.SubjectTypes, event.Subject.Type) {
			return false, entryRejected(activity, "AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED", "event subject.type is not allowed by activity entry audience")
		}
		if len(entry.Audience.Segments) > 0 && !intersectsString(entry.Audience.Segments, attributeStrings(event.Attributes, "segments", "segment")) {
			return false, entryRejected(activity, "AUDIENCE_SEGMENT_NOT_ALLOWED", "event segment is not allowed by activity entry audience")
		}
		if len(entry.Audience.RiskLevels) > 0 && !containsString(entry.Audience.RiskLevels, attributeString(event.Attributes, "riskLevel")) {
			return false, entryRejected(activity, "AUDIENCE_RISK_LEVEL_NOT_ALLOWED", "event riskLevel is not allowed by activity entry audience")
		}
	}
	return true, marketing.ActivityRuleResult{}
}

func entryRejected(activity marketing.Activity, code string, reason string) marketing.ActivityRuleResult {
	return marketing.ActivityRuleResult{
		ActivityID:      activity.ID,
		ActivityCode:    activity.Code,
		ActivityVersion: activity.Version,
		Decision:        marketing.RuleDecisionRejected,
		FailedStage:     marketing.RuleStageMatch,
		ReasonCode:      code,
		Reason:          reason,
		CanReward:       false,
		CanReserveQuota: false,
		StageResults: []marketing.RuleStageResult{{
			Stage:      marketing.RuleStageMatch,
			Status:     marketing.RuleDecisionRejected,
			ReasonCode: code,
			Reason:     reason,
		}},
	}
}

func entryError(activity marketing.Activity, code string, reason string) marketing.ActivityRuleResult {
	result := entryRejected(activity, code, reason)
	result.Decision = marketing.RuleDecisionError
	if len(result.StageResults) > 0 {
		result.StageResults[0].Status = marketing.RuleDecisionError
	}
	return result
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func intersectsString(allowed []string, actual []string) bool {
	for _, value := range actual {
		if containsString(allowed, value) {
			return true
		}
	}
	return false
}

func attributeString(attributes map[string]any, key string) string {
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

func attributeStrings(attributes map[string]any, keys ...string) []string {
	var values []string
	for _, key := range keys {
		value, ok := attributes[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				values = append(values, typed)
			}
		case []any:
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					values = append(values, text)
				}
			}
		case []string:
			values = append(values, typed...)
		}
	}
	return values
}

type matchDecision struct {
	Decision   marketing.RuleDecision
	ReasonCode string
	Reason     string
}

func matchWindowDecision(activity marketing.Activity, occurredAt time.Time) matchDecision {
	if activity.Status != marketing.ActivityActive {
		return matchDecision{
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "ACTIVITY_NOT_ACTIVE",
			Reason:     "activity status is not ACTIVE",
		}
	}
	if !activityApprovalValid(activity) {
		return matchDecision{
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "APPROVAL_REQUIRED",
			Reason:     "activity version is not approved",
		}
	}
	if activity.StartTime.IsZero() || activity.EndTime.IsZero() || !activity.StartTime.Before(activity.EndTime) {
		return matchDecision{
			Decision:   marketing.RuleDecisionError,
			ReasonCode: "INVALID_MATCH_CONFIG",
			Reason:     "activity time window is invalid",
		}
	}
	if occurredAt.Before(activity.StartTime) {
		return matchDecision{
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "ACTIVITY_NOT_STARTED",
			Reason:     "event occurred before activity startTime",
		}
	}
	if !occurredAt.Before(activity.EndTime) {
		return matchDecision{
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "ACTIVITY_ENDED",
			Reason:     "event occurred at or after activity endTime",
		}
	}
	return matchDecision{Decision: marketing.RuleDecisionPassed}
}

func activityApprovalValid(activity marketing.Activity) bool {
	return activity.Approval.Status == marketing.ApprovalApproved && activity.Approval.ActivityVersion == activity.Version
}

func activityRequiresSecondApproval(activity marketing.Activity) bool {
	for _, reward := range activity.Rewards {
		if !reward.Enabled {
			continue
		}
		if reward.Type == marketing.RewardBonus || reward.Type == marketing.RewardCash {
			return true
		}
		if reward.GrantMode == marketing.GrantAuto && (reward.GrantChannel == marketing.GrantHTTP || reward.GrantChannel == marketing.GrantWebhook || reward.GrantChannel == marketing.GrantMQ) {
			return true
		}
		switch reward.CalculateType {
		case marketing.CalculateRandom, marketing.CalculateProbability, marketing.CalculatePoolSplit:
			return true
		}
	}
	for _, rule := range activity.Rules {
		if rule.Phase == marketing.RulePhaseRisk || rule.Phase == marketing.RulePhaseLimit {
			return true
		}
		if bytes.Contains(rule.Config, []byte(`facts.`)) || bytes.Contains(rule.Config, []byte(`state.quota.`)) || bytes.Contains(rule.Config, []byte(`state.plugin.`)) {
			return true
		}
	}
	return false
}

func approvalAuditPayload(activity marketing.Activity, before marketing.ApprovalSnapshot, after marketing.ApprovalSnapshot, reason string, details map[string]any) map[string]any {
	payload := map[string]any{
		"activityId":      activity.ID,
		"activityCode":    activity.Code,
		"activityVersion": activity.Version,
		"highRisk":        after.HighRisk || before.HighRisk,
		"beforeStatus":    before.Status,
		"afterStatus":     after.Status,
		"reason":          strings.TrimSpace(reason),
		"beforeSnapshot":  before,
		"afterSnapshot":   after,
	}
	for key, value := range details {
		payload[key] = value
	}
	return payload
}

func (s *Service) hydrateApproval(ctx context.Context, activity marketing.Activity) marketing.Activity {
	if activity.Approval.Status != "" && activity.Approval.Status != marketing.ApprovalNone {
		return activity
	}
	audits, err := s.repository.ListAudit(ctx, marketing.AuditQuery{TenantID: activity.TenantID, ResourceID: activity.ID, Limit: 200})
	if err != nil {
		return activity
	}
	var latest time.Time
	for _, audit := range audits {
		if !strings.HasPrefix(audit.Action, "APPROVAL_") {
			continue
		}
		if !latest.IsZero() && audit.CreatedAt.Before(latest) {
			continue
		}
		var payload struct {
			AfterSnapshot marketing.ApprovalSnapshot `json:"afterSnapshot"`
		}
		if err := json.Unmarshal(audit.Payload, &payload); err != nil {
			continue
		}
		if payload.AfterSnapshot.Status != "" {
			activity.Approval = payload.AfterSnapshot
			latest = audit.CreatedAt
		}
	}
	return activity
}

func sortActivitiesForMatch(activities []marketing.Activity) {
	slices.SortFunc(activities, func(a, b marketing.Activity) int {
		if a.Priority != b.Priority {
			return b.Priority - a.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func exclusiveSingleSelectGroups(activities []marketing.Activity) map[string]bool {
	groups := map[string]bool{}
	for _, activity := range activities {
		if activity.ExclusiveGroup != "" && !activity.Stackable {
			groups[activity.ExclusiveGroup] = true
		}
	}
	return groups
}

func applyExclusiveGroups(candidates []marketing.SimulationActivityResult) ([]marketing.SimulationActivityResult, []marketing.SimulationActivityResult) {
	singleSelect := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.ExclusiveGroup != "" && !candidate.Stackable {
			singleSelect[candidate.ExclusiveGroup] = true
		}
	}
	groupMatched := map[string]string{}
	matched := make([]marketing.SimulationActivityResult, 0, len(candidates))
	var rejected []marketing.SimulationActivityResult
	for _, candidate := range candidates {
		group := candidate.ExclusiveGroup
		if group == "" || !singleSelect[group] {
			matched = append(matched, candidate)
			continue
		}
		if winnerCode, ok := groupMatched[group]; ok {
			candidate.Decision = marketing.RuleDecisionRejected
			candidate.DecisionReasonCode = "EXCLUSIVE_GROUP_LOST"
			candidate.DecisionReason = "activity lost exclusive group selection to " + winnerCode
			rejected = append(rejected, candidate)
			continue
		}
		groupMatched[group] = candidate.ActivityCode
		matched = append(matched, candidate)
	}
	return matched, rejected
}

func normalizeMatchRuleDecision(activityResult *marketing.SimulationActivityResult) {
	if activityResult.RuleResult.FailedStage != marketing.RuleStageMatch || activityResult.Decision == marketing.RuleDecisionPassed {
		return
	}
	if activityResult.Decision == marketing.RuleDecisionError {
		activityResult.DecisionReasonCode = "INVALID_MATCH_CONFIG"
		if activityResult.DecisionReason == "" {
			activityResult.DecisionReason = "MATCH rule config is invalid"
		}
		return
	}
	activityResult.DecisionReasonCode = "MATCH_RULE_REJECTED"
	if activityResult.DecisionReason == "" {
		activityResult.DecisionReason = "MATCH rule rejected activity"
	}
}

func (s *Service) createExclusiveRejectedParticipation(ctx context.Context, inbox marketing.EventInbox, activity marketing.Activity, winnerCode string) (marketing.Participation, marketing.ActivityRuleResult, error) {
	reason := "activity lost exclusive group selection to " + winnerCode
	ruleResult := marketing.ActivityRuleResult{
		ActivityID:      activity.ID,
		ActivityCode:    activity.Code,
		ActivityVersion: activity.Version,
		Decision:        marketing.RuleDecisionRejected,
		FailedStage:     marketing.RuleStageRiskLimitBudget,
		ReasonCode:      "EXCLUSIVE_GROUP_LOST",
		Reason:          reason,
		CanReward:       false,
		CanReserveQuota: false,
		StageResults: []marketing.RuleStageResult{{
			Stage:      marketing.RuleStageRiskLimitBudget,
			Status:     marketing.RuleDecisionRejected,
			ReasonCode: "EXCLUSIVE_GROUP_LOST",
			Reason:     reason,
		}},
	}
	participation, err := s.createRejectedParticipation(ctx, inbox, activity, ruleResult)
	return participation, ruleResult, err
}

func (s *Service) createRejectedParticipation(ctx context.Context, inbox marketing.EventInbox, activity marketing.Activity, ruleResult marketing.ActivityRuleResult) (marketing.Participation, error) {
	now := s.now()
	rejectReason := ruleResult.ReasonCode
	if ruleResult.Reason != "" {
		rejectReason += ": " + ruleResult.Reason
	}
	participation := marketing.Participation{
		ID:             marketingID("part"),
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   inbox.ID,
		Subject:        inbox.Event.Subject,
		Biz:            inbox.Event.Biz,
		Status:         marketing.ParticipationRejected,
		RejectReason:   rejectReason,
		IdempotencyKey: participationIdempotencyKey(activity, inbox.Event),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, nil, nil)
	created, err := s.repository.CreateParticipation(ctx, participation)
	return created, err
}

func (s *Service) evaluateActivity(ctx context.Context, inbox marketing.EventInbox, activity marketing.Activity) (marketing.Participation, []marketing.GrantOrder, []marketing.OutboxMessage, marketing.ActivityRuleResult, bool, error) {
	now := s.now()
	participation := marketing.Participation{
		ID:             marketingID("part"),
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   inbox.ID,
		Subject:        inbox.Event.Subject,
		Biz:            inbox.Event.Biz,
		Status:         marketing.ParticipationInit,
		IdempotencyKey: participationIdempotencyKey(activity, inbox.Event),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	pluginSnapshot, state, pluginRejected, pluginRuleResult, err := s.preparePlugin(ctx, activity, inbox.Event)
	if err != nil {
		return participation, nil, nil, marketing.ActivityRuleResult{}, false, err
	}
	if pluginRejected {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = pluginRuleResult.ReasonCode + ": " + pluginRuleResult.Reason
		participation.Snapshot = participationSnapshot(activity, inbox, pluginRuleResult, nil, pluginSnapshot)
		participation.UpdatedAt = now
		created, err := s.repository.CreateParticipation(ctx, participation)
		return created, nil, nil, pluginRuleResult, false, err
	}

	facts, factFailure, _ := s.queryActivityFacts(ctx, inbox.Event, activity)
	ruleResult := s.executeRulesWithState(inbox.Event, activity, state, facts)
	if factFailure != nil {
		ruleResult.Decision = marketing.RuleDecisionRejected
		ruleResult.ReasonCode = factFailure.ErrorCode
		ruleResult.Reason = factFailure.Error
	}
	if ruleResult.Decision != marketing.RuleDecisionPassed {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = ruleResult.ReasonCode + ": " + ruleResult.Reason
		participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, nil, pluginSnapshot)
		participation.UpdatedAt = now
		created, err := s.repository.CreateParticipation(ctx, participation)
		return created, nil, nil, ruleResult, false, err
	}

	var orders []marketing.GrantOrder
	var messages []marketing.OutboxMessage
	var stockReservations []marketing.QuotaReservation
	var rewardSummaries []map[string]any
	totalRewardAmount := decimal.Zero()
	participation.Status = marketing.ParticipationRewardable
	participation.UpdatedAt = now
	for _, configuredReward := range activity.Rewards {
		if !configuredReward.Enabled {
			continue
		}
		rewardResult, err := s.rewardCalculator.Calculate(reward.Context{
			Event:          inbox.Event,
			TenantID:       activity.TenantID,
			ActivityID:     activity.ID,
			IdempotencyKey: inbox.IdempotencyKey,
			State:          state,
		}, configuredReward)
		if err != nil {
			return participation, orders, messages, ruleResult, true, err
		}
		if rewardResult.Skipped {
			rewardSummaries = append(rewardSummaries, rewardSnapshot(configuredReward, rewardResult, true))
			continue
		}

		commandID := marketingID("cmd")
		rewardPayload := grantRewardPayload(configuredReward, rewardResult)
		payload, _ := json.Marshal(rewardPayload)
		order := marketing.GrantOrder{
			ID:              marketingID("grant"),
			TenantID:        activity.TenantID,
			ActivityID:      activity.ID,
			ParticipationID: participation.ID,
			Subject:         inbox.Event.Subject,
			RewardType:      rewardResult.Type,
			RewardAmount:    rewardResult.Amount,
			RewardPayload:   payload,
			BillNo:          commandID,
			GrantStatus:     marketing.GrantPending,
			GrantChannel:    configuredReward.GrantChannel,
			IdempotencyKey:  participation.IdempotencyKey + ":" + configuredReward.Code,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		command := marketing.RewardCommand{
			CommandID:       commandID,
			TenantID:        activity.TenantID,
			ActivityID:      activity.ID,
			ParticipationID: participation.ID,
			Subject:         inbox.Event.Subject,
			Reward:          rewardPayload,
			Reason: map[string]any{
				"eventId":   inbox.Event.EventID,
				"eventType": inbox.Event.EventType,
				"bizId":     inbox.Event.Biz.BizID,
			},
			IdempotencyKey: order.IdempotencyKey,
		}
		var message *marketing.OutboxMessage
		if configuredReward.GrantMode == marketing.GrantAuto {
			messagePayload, _ := json.Marshal(command)
			message = &marketing.OutboxMessage{
				ID:            marketingID("outbox"),
				TenantID:      activity.TenantID,
				MessageType:   "REWARD_COMMAND",
				TargetChannel: configuredReward.GrantChannel,
				TargetRef:     targetRefFromReward(configuredReward),
				Payload:       messagePayload,
				SendStatus:    marketing.OutboxPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
		}
		amount, err := decimal.Parse(rewardResult.Amount)
		if err != nil {
			return participation, orders, messages, ruleResult, true, err
		}
		totalRewardAmount = totalRewardAmount.Add(amount)
		orders = append(orders, order)
		if message != nil {
			messages = append(messages, *message)
		}
		rewardSummaries = append(rewardSummaries, rewardSnapshot(configuredReward, rewardResult, false))
		stockReservations = append(stockReservations, marketing.QuotaReservation{
			Type:     marketing.QuotaStock,
			Scope:    marketing.QuotaScopeActivity,
			ScopeKey: configuredReward.Code,
			Amount:   "1",
		})
	}

	if len(orders) == 0 {
		participation.Status = marketing.ParticipationQualified
		participation.RejectReason = reward.ReasonZeroRewardSkipped + ": no payable reward"
		if err := s.appendPluginIntents(ctx, activity, inbox.Event, &participation, pluginSnapshot); err != nil {
			return participation, orders, messages, ruleResult, true, err
		}
		participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, rewardSummaries, pluginSnapshot)
		createdParticipation, err := s.repository.CreateParticipation(ctx, participation)
		return createdParticipation, nil, nil, ruleResult, true, err
	}
	if err := s.appendPluginIntents(ctx, activity, inbox.Event, &participation, pluginSnapshot); err != nil {
		return participation, orders, messages, ruleResult, true, err
	}
	participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, rewardSummaries, pluginSnapshot)
	quotaAttempts := quotaReserveAttempts(activity, inbox.Event, totalRewardAmount.String(), stockReservations)
	atomicResult, err := s.repository.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:      participation,
		BudgetAmount:       totalRewardAmount.String(),
		ReserveUserLimit:   true,
		UserScopeKey:       inbox.Event.Subject.ID,
		ReserveDailyLimit:  true,
		DailyLimitScopeKey: dailyLimitScopeKey(activity, inbox.Event),
		StockReservations:  stockReservations,
		GrantOrders:        orders,
		OutboxMessages:     messages,
	})
	if err != nil {
		if errors.Is(err, marketing.ErrQuotaExceeded) {
			s.recordQuotaAttempts(activity, quotaAttempts, false, err)
			payload := map[string]any{
				"activityId":   activity.ID,
				"eventInboxId": inbox.ID,
				"subject":      inbox.Event.Subject,
				"biz":          inbox.Event.Biz,
				"error":        err.Error(),
				"reason":       "QUOTA_EXCEEDED",
			}
			var quotaErr marketing.QuotaExceededError
			if errors.As(err, &quotaErr) {
				payload["quotaType"] = quotaErr.Type
				payload["scope"] = quotaErr.Scope
				payload["scopeKey"] = quotaErr.ScopeKey
				payload["amount"] = quotaErr.Amount
			}
			_ = s.audit(ctx, activity.TenantID, "engine", "QUOTA_EXCEEDED", "QUOTA", inbox.ID, auditSnapshot(nil, nil, payload))
		}
		return participation, orders, messages, ruleResult, true, err
	}
	s.recordQuotaAttempts(activity, quotaAttempts, true, nil)
	return atomicResult.Participation, atomicResult.GrantOrders, atomicResult.OutboxMessages, ruleResult, true, nil
}

type quotaAttempt struct {
	quotaType marketing.QuotaType
	scope     marketing.QuotaScope
	scopeKey  string
}

func quotaReserveAttempts(activity marketing.Activity, event marketing.ActivityEvent, budgetAmount string, stockReservations []marketing.QuotaReservation) []quotaAttempt {
	attempts := []quotaAttempt{
		{quotaType: marketing.QuotaBudget, scope: marketing.QuotaScopeActivity, scopeKey: "TOTAL"},
		{quotaType: marketing.QuotaUserLimit, scope: marketing.QuotaScopeUser, scopeKey: event.Subject.ID},
		{quotaType: marketing.QuotaDailyLimit, scope: marketing.QuotaScopeUser, scopeKey: dailyLimitScopeKey(activity, event)},
	}
	if decimal.MustParse(budgetAmount).IsZero() {
		attempts = attempts[1:]
	}
	for _, reservation := range stockReservations {
		attempts = append(attempts, quotaAttempt{quotaType: reservation.Type, scope: reservation.Scope, scopeKey: reservation.ScopeKey})
	}
	return attempts
}

func (s *Service) recordQuotaAttempts(activity marketing.Activity, attempts []quotaAttempt, success bool, err error) {
	if s.metrics == nil {
		return
	}
	failedType := marketing.QuotaType("")
	failedScope := marketing.QuotaScope("")
	reason := "ok"
	if err != nil {
		reason = "internal_error"
		var quotaErr marketing.QuotaExceededError
		if errors.As(err, &quotaErr) {
			failedType = quotaErr.Type
			failedScope = quotaErr.Scope
			reason = "quota_exceeded"
		}
	}
	for _, attempt := range attempts {
		result := "success"
		attemptReason := "ok"
		if !success {
			if failedType == "" || (attempt.quotaType == failedType && attempt.scope == failedScope) {
				result = "failed"
				attemptReason = reason
			} else {
				result = "skipped"
				attemptReason = "blocked_by_other_quota"
			}
		}
		labels := []metrics.Label{
			metrics.L("tenant_id", activity.TenantID),
			metrics.L("activity_id", activity.ID),
			metrics.L("quota_type", string(attempt.quotaType)),
			metrics.L("scope", string(attempt.scope)),
			metrics.L("action", "reserve"),
			metrics.L("result", result),
			metrics.L("reason", attemptReason),
		}
		s.metrics.IncCounter("exmarketing_quota_attempt_total", labels...)
		if result == "failed" {
			s.metrics.IncCounter("exmarketing_quota_fail_total",
				metrics.L("tenant_id", activity.TenantID),
				metrics.L("activity_id", activity.ID),
				metrics.L("quota_type", string(attempt.quotaType)),
				metrics.L("scope", string(attempt.scope)),
				metrics.L("action", "reserve"),
				metrics.L("reason", attemptReason),
			)
		}
	}
}

func (s *Service) simulateActivity(ctx context.Context, event marketing.ActivityEvent, activity marketing.Activity) (marketing.Participation, []marketing.GrantOrder, []marketing.OutboxMessage, marketing.ActivityRuleResult, *marketing.ActivityPluginSnapshot, []marketing.FactQuerySimulation, []marketing.RewardPreview, []marketing.QuotaReservation, decimal.Decimal) {
	now := s.now()
	participation := marketing.Participation{
		ID:             marketingID("sim_part"),
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   "simulation",
		Subject:        event.Subject,
		Biz:            event.Biz,
		Status:         marketing.ParticipationRewardable,
		IdempotencyKey: participationIdempotencyKey(activity, event),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	pluginSnapshot, state, pluginRejected, pluginRuleResult, err := s.preparePlugin(context.Background(), activity, event)
	if err != nil {
		pluginRuleResult = pluginErrorRuleResult(activity, activityplugin.ReasonPluginContextError, err.Error())
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = pluginRuleResult.ReasonCode + ": " + pluginRuleResult.Reason
		return participation, nil, nil, pluginRuleResult, pluginSnapshot, nil, nil, nil, decimal.Zero()
	}
	if pluginRejected {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = pluginRuleResult.ReasonCode + ": " + pluginRuleResult.Reason
		return participation, nil, nil, pluginRuleResult, pluginSnapshot, nil, nil, nil, decimal.Zero()
	}
	facts, factFailure, factQueryResults := s.queryActivityFacts(ctx, event, activity)
	ruleResult := s.executeRulesWithState(event, activity, state, facts)
	if factFailure != nil {
		ruleResult.Decision = marketing.RuleDecisionRejected
		ruleResult.ReasonCode = simulationFactReasonCode(factFailure)
		ruleResult.Reason = factFailure.Error
	}
	if ruleResult.Decision != marketing.RuleDecisionPassed {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = ruleResult.ReasonCode + ": " + ruleResult.Reason
		return participation, nil, nil, ruleResult, pluginSnapshot, factQueryResults, nil, nil, decimal.Zero()
	}

	var orders []marketing.GrantOrder
	var outbox []marketing.OutboxMessage
	var rewardPreviews []marketing.RewardPreview
	var stockReservations []marketing.QuotaReservation
	totalRewardAmount := decimal.Zero()
	for _, configuredReward := range activity.Rewards {
		rewardResult, err := s.rewardCalculator.Calculate(reward.Context{
			Event:          event,
			TenantID:       activity.TenantID,
			ActivityID:     activity.ID,
			IdempotencyKey: event.IdempotencyKey(),
			State:          state,
		}, configuredReward)
		if err != nil {
			participation.Status = marketing.ParticipationFailed
			participation.RejectReason = err.Error()
			rewardPreviews = append(rewardPreviews, rewardErrorPreview(configuredReward, err))
			return participation, orders, outbox, ruleResult, pluginSnapshot, factQueryResults, rewardPreviews, stockReservations, totalRewardAmount
		}
		rewardPreviews = append(rewardPreviews, rewardPreview(configuredReward, rewardResult))
		if rewardResult.Skipped {
			continue
		}
		amount, err := decimal.Parse(rewardResult.Amount)
		if err != nil {
			participation.Status = marketing.ParticipationFailed
			participation.RejectReason = err.Error()
			return participation, orders, outbox, ruleResult, pluginSnapshot, factQueryResults, rewardPreviews, stockReservations, totalRewardAmount
		}
		totalRewardAmount = totalRewardAmount.Add(amount)
		commandID := marketingID("sim_cmd")
		rewardPayload := grantRewardPayload(configuredReward, rewardResult)
		payload, _ := json.Marshal(rewardPayload)
		order := marketing.GrantOrder{
			ID:              marketingID("sim_grant"),
			TenantID:        activity.TenantID,
			ActivityID:      activity.ID,
			ParticipationID: participation.ID,
			Subject:         event.Subject,
			RewardType:      rewardResult.Type,
			RewardAmount:    rewardResult.Amount,
			RewardPayload:   payload,
			BillNo:          commandID,
			GrantStatus:     marketing.GrantPending,
			GrantChannel:    configuredReward.GrantChannel,
			IdempotencyKey:  participation.IdempotencyKey + ":" + configuredReward.Code,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		orders = append(orders, order)
		command := marketing.RewardCommand{
			CommandID:       commandID,
			TenantID:        activity.TenantID,
			ActivityID:      activity.ID,
			ParticipationID: participation.ID,
			Subject:         event.Subject,
			Reward:          rewardPayload,
			Reason: map[string]any{
				"eventId":   event.EventID,
				"eventType": event.EventType,
				"bizId":     event.Biz.BizID,
			},
			IdempotencyKey: order.IdempotencyKey,
		}
		if configuredReward.GrantMode == marketing.GrantAuto {
			messagePayload, _ := json.Marshal(command)
			outbox = append(outbox, marketing.OutboxMessage{
				ID:            marketingID("sim_outbox"),
				TenantID:      activity.TenantID,
				MessageType:   "REWARD_COMMAND",
				TargetChannel: configuredReward.GrantChannel,
				TargetRef:     targetRefFromReward(configuredReward),
				Payload:       messagePayload,
				SendStatus:    marketing.OutboxPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		}
		stockReservations = append(stockReservations, marketing.QuotaReservation{
			Type:     marketing.QuotaStock,
			Scope:    marketing.QuotaScopeActivity,
			ScopeKey: configuredReward.Code,
			Amount:   "1",
		})
	}
	if len(orders) == 0 {
		participation.Status = marketing.ParticipationQualified
		participation.RejectReason = reward.ReasonZeroRewardSkipped + ": no payable reward"
	}
	if err := s.appendPluginIntents(context.Background(), activity, event, &participation, pluginSnapshot); err != nil {
		participation.Status = marketing.ParticipationFailed
		participation.RejectReason = err.Error()
	}
	return participation, orders, outbox, ruleResult, pluginSnapshot, factQueryResults, rewardPreviews, stockReservations, totalRewardAmount
}

func simulationActivityBase(activity marketing.Activity, ruleResult *marketing.ActivityRuleResult) marketing.SimulationActivityResult {
	result := marketing.SimulationActivityResult{
		ActivityID:     activity.ID,
		ActivityCode:   activity.Code,
		ActivityName:   activity.Name,
		ActivityType:   activity.Type,
		Status:         activity.Status,
		Version:        activity.Version,
		Priority:       activity.Priority,
		ExclusiveGroup: activity.ExclusiveGroup,
		Stackable:      activity.Stackable,
	}
	if ruleResult != nil {
		result.RuleResult = *ruleResult
	}
	return result
}

func rewardPreview(configuredReward marketing.Reward, result reward.Result) marketing.RewardPreview {
	return marketing.RewardPreview{
		RewardCode:    configuredReward.Code,
		RewardType:    result.Type,
		CalculateType: configuredReward.CalculateType,
		GrantMode:     configuredReward.GrantMode,
		GrantChannel:  configuredReward.GrantChannel,
		Amount:        result.Amount,
		Currency:      result.Currency,
		Skipped:       result.Skipped,
		ReasonCode:    result.ReasonCode,
		Reason:        result.Reason,
		Detail:        result.Payload,
	}
}

func rewardErrorPreview(configuredReward marketing.Reward, err error) marketing.RewardPreview {
	reasonCode := "REWARD_CALCULATION_ERROR"
	reason := err.Error()
	var rewardErr reward.Error
	if errors.As(err, &rewardErr) {
		reasonCode = rewardErr.ReasonCode
		reason = rewardErr.Reason
	}
	return marketing.RewardPreview{
		RewardCode:    configuredReward.Code,
		RewardType:    configuredReward.Type,
		CalculateType: configuredReward.CalculateType,
		GrantMode:     configuredReward.GrantMode,
		GrantChannel:  configuredReward.GrantChannel,
		Skipped:       true,
		ReasonCode:    reasonCode,
		Reason:        reason,
	}
}

func (s *Service) quotaImpactPreviews(ctx context.Context, activity marketing.Activity, event marketing.ActivityEvent, totalRewardAmount decimal.Decimal, stockReservations []marketing.QuotaReservation) ([]marketing.QuotaImpactPreview, bool, error) {
	quotas, err := s.repository.ListActivityQuotas(ctx, activity.ID)
	if err != nil {
		return nil, false, err
	}
	var impacts []marketing.QuotaImpactPreview
	if totalRewardAmount.IsPositive() {
		impacts = append(impacts, quotaImpactPreview(quotas, activity.ID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", totalRewardAmount.String()))
	}
	impacts = append(impacts, quotaImpactPreview(quotas, activity.ID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, event.Subject.ID, "1"))
	impacts = append(impacts, quotaImpactPreview(quotas, activity.ID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, dailyLimitScopeKey(activity, event), "1"))
	for _, reservation := range stockReservations {
		impacts = append(impacts, quotaImpactPreview(quotas, activity.ID, reservation.Type, reservation.Scope, reservation.ScopeKey, reservation.Amount))
	}
	hasRisk := false
	for _, impact := range impacts {
		if impact.WouldExceed {
			hasRisk = true
			break
		}
	}
	return impacts, hasRisk, nil
}

func quotaImpactPreview(quotas []marketing.Quota, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, delta string) marketing.QuotaImpactPreview {
	preview := marketing.QuotaImpactPreview{
		QuotaType:  quotaType,
		Scope:      scope,
		ScopeKey:   scopeKey,
		Delta:      decimal.MustParse(delta).String(),
		ReasonCode: "OK",
		Reason:     "quota is available",
	}
	for _, quota := range quotas {
		if quota.ActivityID != activityID || quota.Type != quotaType || quota.Scope != scope || quota.ScopeKey != scopeKey {
			continue
		}
		total, err := decimal.Parse(quota.TotalValue)
		if err != nil {
			return quotaUnavailablePreview(preview, "quota total value is invalid")
		}
		used, err := decimal.Parse(quota.UsedValue)
		if err != nil {
			return quotaUnavailablePreview(preview, "quota used value is invalid")
		}
		frozen, err := decimal.Parse(quota.FrozenValue)
		if err != nil {
			return quotaUnavailablePreview(preview, "quota frozen value is invalid")
		}
		increment := decimal.MustParse(delta)
		available := total.Sub(used).Sub(frozen)
		preview.CurrentTotal = total.String()
		preview.CurrentUsed = used.String()
		preview.CurrentFrozen = frozen.String()
		preview.AvailableBefore = available.String()
		if increment.Cmp(available) > 0 {
			preview.WouldExceed = true
			preview.ReasonCode = "QUOTA_EXCEEDED"
			preview.Reason = "delta exceeds available quota"
		}
		return preview
	}
	preview.Unlimited = true
	preview.ReasonCode = "QUOTA_UNLIMITED"
	preview.Reason = "quota row is missing; treated as unlimited"
	return preview
}

func quotaUnavailablePreview(preview marketing.QuotaImpactPreview, reason string) marketing.QuotaImpactPreview {
	preview.WouldExceed = true
	preview.ReasonCode = "QUOTA_UNAVAILABLE"
	preview.Reason = reason
	return preview
}

func (s *Service) executeRules(event marketing.ActivityEvent, activity marketing.Activity) marketing.ActivityRuleResult {
	return s.executeRulesWithState(event, activity, activityState(activity, event.OccurredAt), nil)
}

func (s *Service) executeRulesWithState(event marketing.ActivityEvent, activity marketing.Activity, state map[string]any, facts map[string]any) marketing.ActivityRuleResult {
	result := rule.Execute(rule.Context{
		Event: event,
		State: state,
		Facts: facts,
	}, activity.Rules, s.ruleRegistry)
	result.ActivityID = activity.ID
	result.ActivityCode = activity.Code
	result.ActivityVersion = activity.Version
	return result
}

func (s *Service) queryActivityFacts(ctx context.Context, event marketing.ActivityEvent, activity marketing.Activity) (map[string]any, *marketing.FactQueryResponse, []marketing.FactQuerySimulation) {
	refs := marketing.ExtractFactReferences(activity.Rules)
	if len(refs) == 0 {
		return nil, nil, nil
	}
	facts := map[string]any{}
	requiredBy := factRequiredBy(activity.Rules)
	factCodes := make([]string, 0, len(refs))
	for factCode := range refs {
		factCodes = append(factCodes, factCode)
	}
	slices.Sort(factCodes)
	var simulations []marketing.FactQuerySimulation
	for _, factCode := range factCodes {
		response, err := s.QueryFacts(ctx, marketing.FactQueryRequest{
			EventID:      event.EventID,
			EventType:    event.EventType,
			SourceSystem: event.SourceSystem,
			TenantID:     activity.TenantID,
			FactCode:     factCode,
			Subject:      event.Subject,
			Biz:          event.Biz,
			OccurredAt:   event.OccurredAt,
			Attributes:   event.Attributes,
			TraceID:      event.EventID,
		})
		if err != nil {
			failure := marketing.FactQueryResponse{TenantID: activity.TenantID, FactCode: factCode, Success: false, ErrorCode: fact.CodeProviderHTTPFailed, Error: err.Error()}
			simulations = append(simulations, factQuerySimulation(failure, refs[factCode], requiredBy[factCode], facts))
			return facts, &failure, simulations
		}
		simulations = append(simulations, factQuerySimulation(response, refs[factCode], requiredBy[factCode], facts))
		if !response.Success {
			return facts, &response, simulations
		}
		if value, ok := response.Facts[factCode]; ok {
			facts[factCode] = value
			simulations[len(simulations)-1] = factQuerySimulation(response, refs[factCode], requiredBy[factCode], facts)
		}
	}
	return facts, nil, simulations
}

func factRequiredBy(rules []marketing.Rule) map[string][]string {
	out := map[string][]string{}
	for _, activityRule := range rules {
		if activityRule.Type != marketing.RuleTypeCondition || len(activityRule.Config) == 0 {
			continue
		}
		var cfg marketing.ConditionRuleConfig
		if err := json.Unmarshal(activityRule.Config, &cfg); err != nil {
			continue
		}
		seen := map[string]struct{}{}
		for _, condition := range cfg.Conditions {
			factCode, _, ok := splitFactField(condition.Field)
			if !ok {
				continue
			}
			if _, exists := seen[factCode]; exists {
				continue
			}
			seen[factCode] = struct{}{}
			out[factCode] = append(out[factCode], activityRule.Code)
		}
	}
	for factCode := range out {
		slices.Sort(out[factCode])
	}
	return out
}

func factQuerySimulation(response marketing.FactQueryResponse, fields map[string]struct{}, requiredBy []string, facts map[string]any) marketing.FactQuerySimulation {
	status := marketing.RuleDecisionPassed
	reasonCode := "FACT_QUERY_PASSED"
	reason := "fact query passed"
	if !response.Success {
		status = marketing.RuleDecisionRejected
		reasonCode = simulationFactReasonCode(&response)
		reason = response.Error
		if response.Reason != "" {
			reason = response.Reason + ": " + response.Error
		}
	}
	return marketing.FactQuerySimulation{
		FactCode:   response.FactCode,
		RequiredBy: requiredBy,
		Queried:    true,
		Cached:     response.Cached,
		Status:     status,
		ReasonCode: reasonCode,
		Reason:     reason,
		Fields:     factQueryFieldResults(response.FactCode, fields, facts),
	}
}

func factQueryFieldResults(factCode string, fields map[string]struct{}, facts map[string]any) []marketing.FactQueryFieldResult {
	paths := make([]string, 0, len(fields))
	for field := range fields {
		paths = append(paths, field)
	}
	slices.Sort(paths)
	out := make([]marketing.FactQueryFieldResult, 0, len(paths))
	for _, path := range paths {
		_, available := nestedValue(facts, append([]string{factCode}, strings.Split(path, ".")...))
		out = append(out, marketing.FactQueryFieldResult{
			Path:      path,
			FullPath:  "facts." + factCode + "." + path,
			Available: available,
		})
	}
	return out
}

func nestedValue(root map[string]any, parts []string) (any, bool) {
	var current any = root
	for _, part := range parts {
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

func simulationFactReasonCode(response *marketing.FactQueryResponse) string {
	if response == nil {
		return ""
	}
	if response.ErrorCode == fact.CodeMappingFailed {
		return fact.CodeMappingFailed
	}
	return "FACT_QUERY_FAILED"
}

func splitFactField(field string) (string, string, bool) {
	if !strings.HasPrefix(field, "facts.") {
		return "", "", false
	}
	rest := strings.TrimPrefix(field, "facts.")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Service) preparePlugin(ctx context.Context, activity marketing.Activity, event marketing.ActivityEvent) (*marketing.ActivityPluginSnapshot, map[string]any, bool, marketing.ActivityRuleResult, error) {
	state := activityState(activity, event.OccurredAt)
	pluginCode := strings.TrimSpace(activity.PluginCode)
	if pluginCode == "" {
		return nil, state, false, marketing.ActivityRuleResult{}, nil
	}
	registration, ok := s.pluginRegistry.GetRegistration(pluginCode)
	if !ok {
		result := pluginErrorRuleResult(activity, activityplugin.ReasonPluginNotRegistered, "pluginCode is not registered")
		return &marketing.ActivityPluginSnapshot{
			PluginCode: pluginCode,
			Status:     string(marketing.RuleDecisionRejected),
			ReasonCode: result.ReasonCode,
			Reason:     result.Reason,
			PrecheckResult: marketing.PluginPrecheckResult{
				Allowed:    false,
				ReasonCode: result.ReasonCode,
				Reason:     result.Reason,
			},
		}, state, true, result, nil
	}
	activityPlugin := registration.Plugin
	pluginVersion := registration.Metadata.Version
	if registration.Metadata.Disabled {
		result := pluginErrorRuleResult(activity, activityplugin.ReasonPluginDisabled, "pluginCode is disabled")
		_ = s.audit(ctx, activity.TenantID, "system", "PLUGIN_EXECUTION_FAILED", "ACTIVITY_PLUGIN", activity.ID, auditSnapshot(nil, map[string]any{
			"tenantId":        activity.TenantID,
			"activityId":      activity.ID,
			"activityCode":    activity.Code,
			"activityVersion": activity.Version,
			"pluginCode":      pluginCode,
			"pluginVersion":   pluginVersion,
			"lifecycleStep":   "BeforeMatch",
			"status":          "ERROR",
			"errorCode":       result.ReasonCode,
			"reason":          result.Reason,
			"eventId":         event.EventID,
		}, nil))
		return &marketing.ActivityPluginSnapshot{
			PluginCode:    pluginCode,
			PluginVersion: pluginVersion,
			LifecycleStep: "BeforeMatch",
			Status:        string(marketing.RuleDecisionError),
			ReasonCode:    result.ReasonCode,
			Reason:        result.Reason,
			PrecheckResult: marketing.PluginPrecheckResult{
				Allowed:    false,
				ReasonCode: result.ReasonCode,
				Reason:     result.Reason,
			},
		}, state, true, result, nil
	}
	if !s.pluginRegistry.IsCompatible(pluginCode, activity.Type) {
		result := pluginErrorRuleResult(activity, activityplugin.ReasonPluginIncompatible, "pluginCode is not compatible with activityType")
		return &marketing.ActivityPluginSnapshot{
			PluginCode:    pluginCode,
			PluginVersion: pluginVersion,
			LifecycleStep: "BeforeMatch",
			Status:        string(marketing.RuleDecisionRejected),
			ReasonCode:    result.ReasonCode,
			Reason:        result.Reason,
			PrecheckResult: marketing.PluginPrecheckResult{
				Allowed:    false,
				ReasonCode: result.ReasonCode,
				Reason:     result.Reason,
			},
		}, state, true, result, nil
	}
	pluginResult, err := activityPlugin.BuildContext(ctx, activityplugin.BuildContextRequest{
		Activity: activity,
		Event:    event,
	})
	if err != nil {
		result := pluginErrorRuleResult(activity, activityplugin.ReasonPluginContextError, err.Error())
		_ = s.audit(ctx, activity.TenantID, "system", "PLUGIN_EXECUTION_FAILED", "ACTIVITY_PLUGIN", activity.ID, auditSnapshot(nil, map[string]any{
			"tenantId":        activity.TenantID,
			"activityId":      activity.ID,
			"activityCode":    activity.Code,
			"activityVersion": activity.Version,
			"pluginCode":      pluginCode,
			"pluginVersion":   pluginVersion,
			"lifecycleStep":   "BeforeMatch",
			"status":          "ERROR",
			"errorCode":       result.ReasonCode,
			"reason":          result.Reason,
			"eventId":         event.EventID,
		}, nil))
		return &marketing.ActivityPluginSnapshot{
			PluginCode:    pluginCode,
			PluginVersion: pluginVersion,
			LifecycleStep: "BeforeMatch",
			Status:        string(marketing.RuleDecisionError),
			ReasonCode:    result.ReasonCode,
			Reason:        result.Reason,
			PrecheckResult: marketing.PluginPrecheckResult{
				Allowed:    false,
				ReasonCode: result.ReasonCode,
				Reason:     result.Reason,
			},
		}, state, true, result, nil
	}
	precheck := marketing.PluginPrecheckResult{
		Allowed:    pluginResult.Precheck.Allowed,
		ReasonCode: pluginResult.Precheck.ReasonCode,
		Reason:     pluginResult.Precheck.Reason,
	}
	if precheck.ReasonCode == "" {
		precheck.ReasonCode = "OK"
	}
	if precheck.Reason == "" {
		precheck.Reason = "plugin precheck passed"
	}
	snapshot := &marketing.ActivityPluginSnapshot{
		PluginCode:       pluginCode,
		PluginVersion:    pluginVersion,
		LifecycleStep:    "BeforeMatch",
		Status:           "PASS",
		PluginContext:    pluginResult.Context,
		PrecheckResult:   precheck,
		Explain:          pluginResult.Explain,
		CandidateResults: pluginResult.CandidateResults,
		RollbackToken:    pluginResult.RollbackToken,
	}
	if !precheck.Allowed {
		if precheck.ReasonCode == "OK" {
			precheck.ReasonCode = activityplugin.ReasonPluginPrecheck
			snapshot.PrecheckResult = precheck
		}
		snapshot.Status = string(marketing.RuleDecisionRejected)
		snapshot.ReasonCode = precheck.ReasonCode
		snapshot.Reason = precheck.Reason
		return snapshot, state, true, pluginErrorRuleResult(activity, precheck.ReasonCode, precheck.Reason), nil
	}
	state["plugin"] = map[string]any{pluginCode: pluginResult.Context}
	return snapshot, state, false, marketing.ActivityRuleResult{}, nil
}

func (s *Service) appendPluginIntents(ctx context.Context, activity marketing.Activity, event marketing.ActivityEvent, participation *marketing.Participation, snapshot *marketing.ActivityPluginSnapshot) error {
	if snapshot == nil || snapshot.PluginCode == "" {
		return nil
	}
	registration, ok := s.pluginRegistry.GetRegistration(snapshot.PluginCode)
	if !ok {
		return errors.New(activityplugin.ReasonPluginNotRegistered + ": pluginCode is not registered")
	}
	if registration.Metadata.Disabled {
		return errors.New(activityplugin.ReasonPluginDisabled + ": pluginCode is disabled")
	}
	activityPlugin := registration.Plugin
	snapshot.LifecycleStep = "AfterRewardCalculated"
	intents, err := activityPlugin.AfterQualified(ctx, activityplugin.AfterQualifiedRequest{
		Activity:                    activity,
		Event:                       event,
		ParticipationID:             participation.ID,
		ParticipationIdempotencyKey: participation.IdempotencyKey,
		PluginContext:               snapshot.PluginContext,
	})
	if err != nil {
		snapshot.Status = string(marketing.RuleDecisionError)
		snapshot.ReasonCode = activityplugin.ReasonPluginExecutionFailed
		snapshot.Reason = err.Error()
		_ = s.audit(ctx, activity.TenantID, "system", "PLUGIN_EXECUTION_FAILED", "ACTIVITY_PLUGIN", activity.ID, auditSnapshot(nil, map[string]any{
			"tenantId":        activity.TenantID,
			"activityId":      activity.ID,
			"activityCode":    activity.Code,
			"activityVersion": activity.Version,
			"pluginCode":      snapshot.PluginCode,
			"pluginVersion":   snapshot.PluginVersion,
			"lifecycleStep":   "AfterRewardCalculated",
			"status":          "ERROR",
			"errorCode":       activityplugin.ReasonPluginExecutionFailed,
			"reason":          err.Error(),
			"traceId":         participation.IdempotencyKey,
			"eventId":         event.EventID,
		}, nil))
		return errors.New(activityplugin.ReasonPluginIntentError + ": " + err.Error())
	}
	snapshot.Status = "PASS"
	snapshot.StateChangeIntents = make([]marketing.PluginStateChangeIntent, 0, len(intents))
	for _, intent := range intents {
		pluginCode := firstNonEmpty(intent.PluginCode, snapshot.PluginCode)
		idempotencyKey := intent.IdempotencyKey
		if idempotencyKey == "" {
			idempotencyKey = participation.IdempotencyKey + ":plugin:" + pluginCode + ":" + intent.IntentType
		}
		snapshot.StateChangeIntents = append(snapshot.StateChangeIntents, marketing.PluginStateChangeIntent{
			PluginCode:                  pluginCode,
			IntentType:                  intent.IntentType,
			TenantID:                    firstNonEmpty(intent.TenantID, activity.TenantID),
			ActivityID:                  firstNonEmpty(intent.ActivityID, activity.ID),
			ActivityCode:                firstNonEmpty(intent.ActivityCode, activity.Code),
			Subject:                     event.Subject,
			Biz:                         event.Biz,
			EventID:                     firstNonEmpty(intent.EventID, event.EventID),
			ParticipationID:             firstNonEmpty(intent.ParticipationID, participation.ID),
			ParticipationIdempotencyKey: firstNonEmpty(intent.ParticipationIdempotencyKey, participation.IdempotencyKey),
			IdempotencyKey:              idempotencyKey,
			Payload:                     intent.Payload,
		})
	}
	return nil
}

func pluginErrorRuleResult(activity marketing.Activity, reasonCode string, reason string) marketing.ActivityRuleResult {
	return marketing.ActivityRuleResult{
		ActivityID:      activity.ID,
		ActivityCode:    activity.Code,
		ActivityVersion: activity.Version,
		Decision:        marketing.RuleDecisionRejected,
		FailedStage:     marketing.RuleStageQualify,
		ReasonCode:      reasonCode,
		Reason:          reason,
		CanReward:       false,
		CanReserveQuota: false,
		StageResults: []marketing.RuleStageResult{{
			Stage:       marketing.RuleStageQualify,
			Status:      marketing.RuleDecisionRejected,
			FailedCount: 1,
			ReasonCode:  reasonCode,
			Reason:      reason,
		}},
	}
}

func activityState(activity marketing.Activity, now time.Time) map[string]any {
	return map[string]any{
		"activity": map[string]any{
			"id":        activity.ID,
			"code":      activity.Code,
			"status":    activity.Status,
			"startTime": activity.StartTime.Format(time.RFC3339),
			"endTime":   activity.EndTime.Format(time.RFC3339),
			"timezone":  activity.Timezone,
			"active":    activityRunnable(activity, now),
			"priority":  activity.Priority,
		},
		"exclusive": map[string]any{
			"group":     activity.ExclusiveGroup,
			"stackable": activity.Stackable,
		},
	}
}

func dailyLimitScopeKey(activity marketing.Activity, event marketing.ActivityEvent) string {
	location, err := time.LoadLocation(strings.TrimSpace(activity.Timezone))
	if err != nil {
		location = time.UTC
	}
	return event.Subject.ID + ":" + event.OccurredAt.In(location).Format("2006-01-02")
}

func (s *Service) audit(ctx context.Context, tenantID, actor, action, resource, resourceID string, payload any) error {
	now := s.now()
	payload = auditPayloadWithSource(ctx, payload)
	bytes, _ := json.Marshal(payload)
	return s.repository.CreateAudit(ctx, marketing.AuditRecord{
		ID:         marketingID("audit"),
		TenantID:   tenantID,
		Actor:      actor,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Payload:    bytes,
		CreatedAt:  now,
	})
}

func actorOrSystem(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "system"
	}
	return actor
}

func auditSnapshot(before any, after any, details map[string]any) map[string]any {
	payload := map[string]any{
		"beforeSnapshot": before,
		"afterSnapshot":  after,
	}
	for key, value := range details {
		payload[key] = value
	}
	return payload
}

func auditPayloadWithSource(ctx context.Context, payload any) any {
	source := marketing.AuditSourceFromContext(ctx)
	if len(source) == 0 {
		source = map[string]any{"component": "internal"}
	}
	if typed, ok := payload.(map[string]any); ok {
		out := make(map[string]any, len(typed)+1)
		for key, value := range typed {
			out[key] = value
		}
		if _, exists := out["source"]; !exists {
			out["source"] = source
		}
		return out
	}
	return map[string]any{
		"source":        source,
		"afterSnapshot": payload,
	}
}

func normalizeActivityStatus(status marketing.ActivityStatus, now, start, end time.Time) marketing.ActivityStatus {
	if status == "" {
		return statusForWindow(now, start, end)
	}
	return status
}

func statusForWindow(now, start, end time.Time) marketing.ActivityStatus {
	if now.Before(start) {
		return marketing.ActivityScheduled
	}
	if now.Before(end) {
		return marketing.ActivityActive
	}
	return marketing.ActivityEnded
}

func activityRunnable(activity marketing.Activity, now time.Time) bool {
	return activity.Status == marketing.ActivityActive && !now.Before(activity.StartTime) && now.Before(activity.EndTime)
}

func normalizeRules(rules []marketing.Rule, now time.Time) []marketing.Rule {
	for i := range rules {
		if rules[i].ID == "" {
			rules[i].ID = marketingID("rule")
		}
		if rules[i].Type == "" {
			rules[i].Type = marketing.RuleTypeCondition
		}
		if rules[i].Phase == "" {
			rules[i].Phase = marketing.RulePhaseQualify
		}
		if rules[i].Version == 0 {
			rules[i].Version = 1
		}
		rules[i].CreatedAt = now
		rules[i].UpdatedAt = now
	}
	slices.SortFunc(rules, func(a, b marketing.Rule) int {
		return a.Sort - b.Sort
	})
	return rules
}

func normalizeRewards(rewards []marketing.Reward, now time.Time) []marketing.Reward {
	for i := range rewards {
		if rewards[i].ID == "" {
			rewards[i].ID = marketingID("reward")
		}
		if rewards[i].GrantMode == "" {
			rewards[i].GrantMode = marketing.GrantAuto
		}
		if rewards[i].GrantChannel == "" {
			if rewards[i].GrantMode == marketing.GrantManual || rewards[i].GrantMode == marketing.GrantManualExport {
				rewards[i].GrantChannel = marketing.GrantManualFile
			} else {
				rewards[i].GrantChannel = marketing.GrantHTTP
			}
		}
		if rewards[i].Version == 0 {
			rewards[i].Version = 1
		}
		rewards[i].CreatedAt = now
		rewards[i].UpdatedAt = now
	}
	return rewards
}

func marketingID(prefix string) string {
	return marketing.NewID(prefix)
}

func targetRefFromReward(configuredReward marketing.Reward) string {
	var config marketing.RewardConfig
	if err := json.Unmarshal(configuredReward.Config, &config); err == nil && config.TargetRef != "" {
		return config.TargetRef
	}
	return string(configuredReward.GrantChannel)
}

func participationIdempotencyKey(activity marketing.Activity, event marketing.ActivityEvent) string {
	return activity.TenantID + ":" + activity.ID + ":" + event.IdempotencyKey()
}

func participationSnapshot(activity marketing.Activity, inbox marketing.EventInbox, ruleResult marketing.ActivityRuleResult, rewardSummaries []map[string]any, pluginSnapshot *marketing.ActivityPluginSnapshot) json.RawMessage {
	snapshot := map[string]any{
		"activity": map[string]any{
			"id":             activity.ID,
			"activityCode":   activity.Code,
			"activityType":   activity.Type,
			"version":        activity.Version,
			"priority":       activity.Priority,
			"exclusiveGroup": activity.ExclusiveGroup,
			"stackable":      activity.Stackable,
			"pluginCode":     activity.PluginCode,
			"config":         rawJSONValue(activity.Config),
			"rules":          activity.Rules,
			"rewards":        activity.Rewards,
		},
		"event": map[string]any{
			"inboxId":        inbox.ID,
			"eventId":        inbox.Event.EventID,
			"eventType":      inbox.Event.EventType,
			"sourceSystem":   inbox.Event.SourceSystem,
			"tenantId":       inbox.Event.TenantID,
			"subject":        inbox.Event.Subject,
			"biz":            inbox.Event.Biz,
			"occurredAt":     inbox.Event.OccurredAt,
			"attributes":     inbox.Event.Attributes,
			"idempotencyKey": inbox.IdempotencyKey,
		},
		"ruleResult": ruleResult,
		"rewards":    rewardSummaries,
	}
	if pluginSnapshot != nil {
		snapshot["plugin"] = pluginSnapshot
	}
	bytes, _ := json.Marshal(snapshot)
	return bytes
}

func rewardSnapshot(configuredReward marketing.Reward, result reward.Result, skipped bool) map[string]any {
	return map[string]any{
		"rewardCode":    configuredReward.Code,
		"rewardType":    result.Type,
		"calculateType": configuredReward.CalculateType,
		"grantMode":     configuredReward.GrantMode,
		"grantChannel":  configuredReward.GrantChannel,
		"amount":        result.Amount,
		"currency":      result.Currency,
		"skipped":       skipped,
		"reasonCode":    result.ReasonCode,
		"reason":        result.Reason,
	}
}

func grantRewardPayload(configuredReward marketing.Reward, result reward.Result) map[string]any {
	payload := map[string]any{}
	for key, value := range result.Payload {
		payload[key] = value
	}
	payload["rewardCode"] = configuredReward.Code
	payload["rewardType"] = result.Type
	payload["calculateType"] = configuredReward.CalculateType
	payload["grantMode"] = configuredReward.GrantMode
	payload["grantChannel"] = configuredReward.GrantChannel
	payload["targetRef"] = targetRefFromReward(configuredReward)
	return payload
}

func rawJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
