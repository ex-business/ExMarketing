package engine

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/reward"
	"github.com/ex-stack/ExMarketing/internal/rule"
)

type Service struct {
	repository       marketing.Repository
	ruleRegistry     *rule.Registry
	rewardCalculator reward.Calculator
	now              func() time.Time
}

func NewService(repository marketing.Repository) *Service {
	return &Service{
		repository:       repository,
		ruleRegistry:     rule.NewRegistry(),
		rewardCalculator: reward.ConfigCalculator{},
		now:              func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) CreateActivity(ctx context.Context, cmd marketing.CreateActivityCommand) (marketing.Activity, error) {
	now := s.now()
	if err := cmd.Validate(now); err != nil {
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
		Version:        1,
		Config:         cmd.Config,
		Rules:          normalizeRules(cmd.Rules, now),
		Rewards:        normalizeRewards(cmd.Rewards, now),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	created, err := s.repository.CreateActivity(ctx, activity)
	if err != nil {
		return marketing.Activity{}, err
	}
	_ = s.audit(ctx, created.TenantID, "system", "CREATE_ACTIVITY", "activity", created.ID, created)
	return created, nil
}

func (s *Service) ListActivities(ctx context.Context) ([]marketing.Activity, error) {
	return s.repository.ListActivities(ctx)
}

func (s *Service) ValidatePrelaunch(ctx context.Context, req marketing.PrelaunchValidationRequest) (marketing.PrelaunchValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return marketing.PrelaunchValidationResult{}, err
	}
	result := marketing.ValidatePrelaunch(req, s.now())
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "system"
	}
	_ = s.audit(ctx, req.Activity.TenantID, actor, "PRELAUNCH_VALIDATE", "activity", req.Activity.Code, result.AuditSummary)
	return result, nil
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
	_ = s.audit(ctx, "", "system", "CREATE_QUOTA", "quota", created.ID, created)
	return created, nil
}

func (s *Service) ListAudit(ctx context.Context, limit int) ([]marketing.AuditRecord, error) {
	return s.repository.ListAudit(ctx, limit)
}

func (s *Service) ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]marketing.OutboxMessage, error) {
	return s.repository.ListOutboxDLQ(ctx, tenantID, includeClosed, limit)
}

func (s *Service) ListRewardReconciliation(ctx context.Context, query marketing.RewardReconciliationQuery) ([]marketing.RewardReconciliationItem, error) {
	if strings.TrimSpace(query.TenantID) == "" {
		return nil, errors.New("tenantId is required")
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
	if err := event.Validate(); err != nil {
		return marketing.ProcessEventResult{}, err
	}
	event = event.Normalized()

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
	_ = s.audit(ctx, event.TenantID, "openapi", "EVENT_IN", "event_inbox", inbox.ID, event)

	activities, err := s.repository.FindMatchingActivities(ctx, event.TenantID, event.EventType)
	if err != nil {
		return marketing.ProcessEventResult{}, err
	}

	result := marketing.ProcessEventResult{Inbox: inbox}
	matchedExclusive := map[string]bool{}
	for _, activity := range activities {
		if !activityRunnable(activity, now) {
			continue
		}
		if activity.ExclusiveGroup != "" && matchedExclusive[activity.ExclusiveGroup] && !activity.Stackable {
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
			matchedExclusive[activity.ExclusiveGroup] = true
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

	now := s.now()
	result := marketing.SimulationResult{Event: event, SimulatedAt: now}
	matchedExclusive := map[string]bool{}
	for _, activity := range activities {
		result.Summary.CandidateCount++
		if !activityRunnable(activity, now) {
			activityResult := simulationActivityBase(activity, nil)
			activityResult.Decision = marketing.RuleDecisionRejected
			activityResult.DecisionReasonCode = "ACTIVITY_NOT_ACTIVE"
			activityResult.DecisionReason = "activity is not active in current time window"
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
			continue
		}
		if activity.ExclusiveGroup != "" && matchedExclusive[activity.ExclusiveGroup] && !activity.Stackable {
			activityResult := simulationActivityBase(activity, nil)
			activityResult.Decision = marketing.RuleDecisionRejected
			activityResult.DecisionReasonCode = "EXCLUSIVE_GROUP_REJECTED"
			activityResult.DecisionReason = "another activity in the exclusive group already matched"
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
			continue
		}

		participation, grants, outbox, ruleResult, rewardPreviews, stockReservations, totalRewardAmount := s.simulateActivity(event, activity)
		activityResult := simulationActivityBase(activity, &ruleResult)
		activityResult.Decision = ruleResult.Decision
		activityResult.DecisionReasonCode = ruleResult.ReasonCode
		activityResult.DecisionReason = ruleResult.Reason
		activityResult.RewardPreviews = rewardPreviews
		activityResult.ParticipationPreview = &participation
		activityResult.GrantOrderPreviews = grants
		activityResult.OutboxCommandPreviews = outbox
		activityResult.WouldCreateGrantOrders = len(grants) > 0

		if ruleResult.Decision == marketing.RuleDecisionPassed && len(grants) > 0 {
			quotaPreviews, hasQuotaRisk, err := s.quotaImpactPreviews(ctx, activity.ID, event.Subject.ID, totalRewardAmount, stockReservations)
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
			result.MatchedActivities = append(result.MatchedActivities, activityResult)
			result.Summary.HasRewardableActivity = result.Summary.HasRewardableActivity || len(grants) > 0
			if activity.ExclusiveGroup != "" {
				matchedExclusive[activity.ExclusiveGroup] = true
			}
		} else {
			result.UnmatchedActivities = append(result.UnmatchedActivities, activityResult)
		}
	}
	result.Summary.MatchedCount = len(result.MatchedActivities)
	result.Summary.UnmatchedCount = len(result.UnmatchedActivities)
	return result, nil
}

func (s *Service) ApplyRewardCallback(ctx context.Context, callback marketing.RewardCallback) (marketing.RewardCallbackResult, error) {
	if err := callback.Validate(); err != nil {
		return marketing.RewardCallbackResult{}, err
	}
	callback.FinishedAt = callback.FinishedAt.UTC()
	return s.repository.ApplyRewardCallback(ctx, callback)
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

	ruleResult := s.executeRules(inbox.Event, activity)
	if ruleResult.Decision != marketing.RuleDecisionPassed {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = ruleResult.ReasonCode + ": " + ruleResult.Reason
		participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, nil)
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
			State:          activityState(activity, now),
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
		messagePayload, _ := json.Marshal(command)
		message := marketing.OutboxMessage{
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
		amount, err := decimal.Parse(rewardResult.Amount)
		if err != nil {
			return participation, orders, messages, ruleResult, true, err
		}
		totalRewardAmount = totalRewardAmount.Add(amount)
		orders = append(orders, order)
		messages = append(messages, message)
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
		participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, rewardSummaries)
		createdParticipation, err := s.repository.CreateParticipation(ctx, participation)
		return createdParticipation, nil, nil, ruleResult, true, err
	}
	participation.Snapshot = participationSnapshot(activity, inbox, ruleResult, rewardSummaries)
	atomicResult, err := s.repository.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:     participation,
		BudgetAmount:      totalRewardAmount.String(),
		ReserveUserLimit:  true,
		UserScopeKey:      inbox.Event.Subject.ID,
		StockReservations: stockReservations,
		GrantOrders:       orders,
		OutboxMessages:    messages,
	})
	if err != nil {
		if errors.Is(err, marketing.ErrQuotaExceeded) {
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
			_ = s.audit(ctx, activity.TenantID, "engine", "QUOTA_EXCEEDED", "event_inbox", inbox.ID, payload)
		}
		return participation, orders, messages, ruleResult, true, err
	}
	return atomicResult.Participation, atomicResult.GrantOrders, atomicResult.OutboxMessages, ruleResult, true, nil
}

func (s *Service) simulateActivity(event marketing.ActivityEvent, activity marketing.Activity) (marketing.Participation, []marketing.GrantOrder, []marketing.OutboxMessage, marketing.ActivityRuleResult, []marketing.RewardPreview, []marketing.QuotaReservation, decimal.Decimal) {
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
	ruleResult := s.executeRules(event, activity)
	if ruleResult.Decision != marketing.RuleDecisionPassed {
		participation.Status = marketing.ParticipationRejected
		participation.RejectReason = ruleResult.ReasonCode + ": " + ruleResult.Reason
		return participation, nil, nil, ruleResult, nil, nil, decimal.Zero()
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
			State:          activityState(activity, now),
		}, configuredReward)
		if err != nil {
			participation.Status = marketing.ParticipationFailed
			participation.RejectReason = err.Error()
			rewardPreviews = append(rewardPreviews, rewardErrorPreview(configuredReward, err))
			return participation, orders, outbox, ruleResult, rewardPreviews, stockReservations, totalRewardAmount
		}
		rewardPreviews = append(rewardPreviews, rewardPreview(configuredReward, rewardResult))
		if rewardResult.Skipped {
			continue
		}
		amount, err := decimal.Parse(rewardResult.Amount)
		if err != nil {
			participation.Status = marketing.ParticipationFailed
			participation.RejectReason = err.Error()
			return participation, orders, outbox, ruleResult, rewardPreviews, stockReservations, totalRewardAmount
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
	return participation, orders, outbox, ruleResult, rewardPreviews, stockReservations, totalRewardAmount
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

func (s *Service) quotaImpactPreviews(ctx context.Context, activityID string, userScopeKey string, totalRewardAmount decimal.Decimal, stockReservations []marketing.QuotaReservation) ([]marketing.QuotaImpactPreview, bool, error) {
	quotas, err := s.repository.ListActivityQuotas(ctx, activityID)
	if err != nil {
		return nil, false, err
	}
	var impacts []marketing.QuotaImpactPreview
	if totalRewardAmount.IsPositive() {
		impacts = append(impacts, quotaImpactPreview(quotas, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", totalRewardAmount.String()))
		impacts = append(impacts, quotaImpactPreview(quotas, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, userScopeKey, "1"))
	}
	for _, reservation := range stockReservations {
		impacts = append(impacts, quotaImpactPreview(quotas, activityID, reservation.Type, reservation.Scope, reservation.ScopeKey, reservation.Amount))
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
	result := rule.Execute(rule.Context{
		Event: event,
		State: activityState(activity, s.now()),
	}, activity.Rules, s.ruleRegistry)
	result.ActivityID = activity.ID
	result.ActivityCode = activity.Code
	result.ActivityVersion = activity.Version
	return result
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

func (s *Service) audit(ctx context.Context, tenantID, actor, action, resource, resourceID string, payload any) error {
	now := s.now()
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
			rewards[i].GrantChannel = marketing.GrantHTTP
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

func participationSnapshot(activity marketing.Activity, inbox marketing.EventInbox, ruleResult marketing.ActivityRuleResult, rewardSummaries []map[string]any) json.RawMessage {
	snapshot := map[string]any{
		"activity": map[string]any{
			"id":             activity.ID,
			"activityCode":   activity.Code,
			"activityType":   activity.Type,
			"version":        activity.Version,
			"priority":       activity.Priority,
			"exclusiveGroup": activity.ExclusiveGroup,
			"stackable":      activity.Stackable,
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
	bytes, _ := json.Marshal(snapshot)
	return bytes
}

func rewardSnapshot(configuredReward marketing.Reward, result reward.Result, skipped bool) map[string]any {
	return map[string]any{
		"rewardCode":    configuredReward.Code,
		"rewardType":    result.Type,
		"calculateType": configuredReward.CalculateType,
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
