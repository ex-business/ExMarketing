package engine

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/reward"
	"github.com/ex-stack/ExMarketing/internal/rule"
)

type Service struct {
	repository       marketing.Repository
	ruleEvaluator    rule.Evaluator
	rewardCalculator reward.Calculator
	now              func() time.Time
}

func NewService(repository marketing.Repository) *Service {
	return &Service{
		repository:       repository,
		ruleEvaluator:    rule.ConditionEvaluator{},
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

		participation, rewardOrders, outboxMessages, qualified, err := s.evaluateActivity(ctx, inbox, activity)
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
	activities, err := s.repository.FindMatchingActivities(ctx, event.TenantID, event.EventType)
	if err != nil {
		return marketing.SimulationResult{}, err
	}

	now := s.now()
	result := marketing.SimulationResult{Event: event}
	for _, activity := range activities {
		if !activityRunnable(activity, now) {
			continue
		}
		participation, grants, outbox := s.simulateActivity(event, activity)
		result.Participations = append(result.Participations, participation)
		result.GrantOrders = append(result.GrantOrders, grants...)
		result.OutboxMessages = append(result.OutboxMessages, outbox...)
	}
	return result, nil
}

func (s *Service) ApplyRewardCallback(ctx context.Context, callback marketing.RewardCallback) error {
	if callback.CommandID == "" || callback.TenantID == "" {
		return errors.New("commandId and tenantId are required")
	}
	return s.repository.UpdateGrantOrderByCommandID(ctx, callback.TenantID, callback.CommandID, callback.Status, callback.Message)
}

func (s *Service) evaluateActivity(ctx context.Context, inbox marketing.EventInbox, activity marketing.Activity) (marketing.Participation, []marketing.GrantOrder, []marketing.OutboxMessage, bool, error) {
	now := s.now()
	snapshot, _ := json.Marshal(activity)
	participation := marketing.Participation{
		ID:             marketingID("part"),
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   inbox.ID,
		Subject:        inbox.Event.Subject,
		Biz:            inbox.Event.Biz,
		Status:         marketing.ParticipationInit,
		Snapshot:       snapshot,
		IdempotencyKey: activity.TenantID + ":" + activity.ID + ":" + inbox.Event.Subject.Type + ":" + inbox.Event.Subject.ID + ":" + inbox.Event.Biz.BizID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	for _, activityRule := range activity.Rules {
		if !activityRule.Enabled || activityRule.Type != marketing.RuleTypeCondition {
			continue
		}
		if !slices.Contains([]marketing.RulePhase{
			marketing.RulePhaseMatch,
			marketing.RulePhaseQualify,
			marketing.RulePhaseReward,
			marketing.RulePhaseRisk,
			marketing.RulePhaseLimit,
		}, activityRule.Phase) {
			continue
		}
		ruleResult := s.ruleEvaluator.Evaluate(rule.Context{Event: inbox.Event}, activityRule)
		if !ruleResult.Passed {
			participation.Status = marketing.ParticipationRejected
			participation.RejectReason = activityRule.Code + ": " + ruleResult.Reason
			participation.UpdatedAt = now
			created, err := s.repository.CreateParticipation(ctx, participation)
			return created, nil, nil, false, err
		}
	}

	var orders []marketing.GrantOrder
	var messages []marketing.OutboxMessage
	var stockReservations []marketing.QuotaReservation
	totalRewardAmount := decimal.Zero()
	participation.Status = marketing.ParticipationRewardable
	participation.UpdatedAt = now
	for _, configuredReward := range activity.Rewards {
		if !configuredReward.Enabled {
			continue
		}
		rewardResult, err := s.rewardCalculator.Calculate(reward.Context{Event: inbox.Event}, configuredReward)
		if err != nil {
			return participation, orders, messages, true, err
		}

		commandID := marketingID("cmd")
		payload, _ := json.Marshal(rewardResult.Payload)
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
			Reward:          rewardResult.Payload,
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
			return participation, orders, messages, true, err
		}
		totalRewardAmount = totalRewardAmount.Add(amount)
		orders = append(orders, order)
		messages = append(messages, message)
		stockReservations = append(stockReservations, marketing.QuotaReservation{
			Type:     marketing.QuotaStock,
			Scope:    marketing.QuotaScopeActivity,
			ScopeKey: configuredReward.Code,
			Amount:   "1",
		})
	}

	if len(orders) == 0 {
		createdParticipation, err := s.repository.CreateParticipation(ctx, participation)
		return createdParticipation, nil, nil, true, err
	}
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
		return participation, orders, messages, true, err
	}
	return atomicResult.Participation, atomicResult.GrantOrders, atomicResult.OutboxMessages, true, nil
}

func (s *Service) simulateActivity(event marketing.ActivityEvent, activity marketing.Activity) (marketing.Participation, []marketing.GrantOrder, []marketing.OutboxMessage) {
	now := s.now()
	participation := marketing.Participation{
		ID:             marketingID("sim_part"),
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   "simulation",
		Subject:        event.Subject,
		Biz:            event.Biz,
		Status:         marketing.ParticipationRewardable,
		IdempotencyKey: activity.TenantID + ":" + activity.ID + ":" + event.Subject.Type + ":" + event.Subject.ID + ":" + event.Biz.BizID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for _, activityRule := range activity.Rules {
		if !activityRule.Enabled || activityRule.Type != marketing.RuleTypeCondition {
			continue
		}
		ruleResult := s.ruleEvaluator.Evaluate(rule.Context{Event: event}, activityRule)
		if !ruleResult.Passed {
			participation.Status = marketing.ParticipationRejected
			participation.RejectReason = activityRule.Code + ": " + ruleResult.Reason
			return participation, nil, nil
		}
	}

	var orders []marketing.GrantOrder
	var outbox []marketing.OutboxMessage
	for _, configuredReward := range activity.Rewards {
		if !configuredReward.Enabled {
			continue
		}
		rewardResult, err := s.rewardCalculator.Calculate(reward.Context{Event: event}, configuredReward)
		if err != nil {
			participation.Status = marketing.ParticipationFailed
			participation.RejectReason = err.Error()
			return participation, orders, outbox
		}
		commandID := marketingID("sim_cmd")
		payload, _ := json.Marshal(rewardResult.Payload)
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
			Reward:          rewardResult.Payload,
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
	}
	return participation, orders, outbox
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
		rules[i].Enabled = true
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
		rewards[i].Enabled = true
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
