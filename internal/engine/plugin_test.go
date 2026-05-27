package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	activityplugin "github.com/ex-stack/ExMarketing/internal/plugin"
)

type testActivityPlugin struct {
	code            string
	activityTypes   []string
	context         map[string]any
	precheckAllowed bool
}

func (p testActivityPlugin) Code() string {
	return p.code
}

func (p testActivityPlugin) CompatibleActivityTypes() []string {
	return p.activityTypes
}

func (p testActivityPlugin) ValidateConfig(json.RawMessage) error {
	return nil
}

func (p testActivityPlugin) BuildContext(context.Context, activityplugin.BuildContextRequest) (activityplugin.ContextResult, error) {
	return activityplugin.ContextResult{
		Context: p.context,
		Precheck: activityplugin.PrecheckResult{
			Allowed:    p.precheckAllowed,
			ReasonCode: "OK",
			Reason:     "plugin precheck passed",
		},
		Explain: map[string]any{"source": "test"},
	}, nil
}

func (p testActivityPlugin) AfterQualified(context.Context, activityplugin.AfterQualifiedRequest) ([]activityplugin.StateChangeIntent, error) {
	return []activityplugin.StateChangeIntent{{
		PluginCode: p.code,
		IntentType: "consumeDrawChance",
		Payload:    map[string]any{"amount": "1"},
	}}, nil
}

func TestActivityPluginContextFeedsRulesRewardsAndSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	if err := service.RegisterActivityPlugin(testActivityPlugin{
		code:            "LOTTERY_TEST",
		activityTypes:   []string{"LOTTERY"},
		precheckAllowed: true,
		context: map[string]any{
			"drawChance": "1",
			"amount":     "20.00",
		},
	}); err != nil {
		t.Fatalf("register plugin: %v", err)
	}

	activity, err := createApprovedPluginActivity(ctx, service, pluginActivityCommand("LOTTERY_TEST"))
	if err != nil {
		t.Fatalf("create activity: %v", err)
	}
	result, err := service.ProcessEvent(ctx, pluginEvent("evt_plugin_ok", "biz_plugin_ok"))
	if err != nil {
		t.Fatalf("process event: %v", err)
	}

	if len(result.Participations) != 1 || len(result.GrantOrders) != 1 || len(result.OutboxMessages) != 1 {
		t.Fatalf("expected one participation/grant/outbox, got %+v", result)
	}
	if result.GrantOrders[0].RewardAmount != "10.0000" {
		t.Fatalf("reward amount=%s, want 10.0000", result.GrantOrders[0].RewardAmount)
	}

	var snapshot map[string]any
	if err := json.Unmarshal(result.Participations[0].Snapshot, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	pluginSnapshot, ok := snapshot["plugin"].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin snapshot, got %s", string(result.Participations[0].Snapshot))
	}
	if pluginSnapshot["pluginCode"] != "LOTTERY_TEST" {
		t.Fatalf("pluginCode=%v", pluginSnapshot["pluginCode"])
	}
	contextSnapshot, ok := pluginSnapshot["pluginContext"].(map[string]any)
	if !ok || contextSnapshot["drawChance"] != "1" || contextSnapshot["amount"] != "20.00" {
		t.Fatalf("unexpected plugin context: %+v", pluginSnapshot["pluginContext"])
	}
	intents, ok := pluginSnapshot["stateChangeIntents"].([]any)
	if !ok || len(intents) != 1 {
		t.Fatalf("expected one stateChangeIntent, got %+v", pluginSnapshot["stateChangeIntents"])
	}
	intent := intents[0].(map[string]any)
	if intent["participationIdempotencyKey"] != result.Participations[0].IdempotencyKey {
		t.Fatalf("intent not tied to participation idempotency key: %+v", intent)
	}
	if activity.PluginCode != "LOTTERY_TEST" {
		t.Fatalf("activity pluginCode=%s", activity.PluginCode)
	}
}

func TestActivityPluginUnknownCodeRejectedAtCreateAndRuntime(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	_, err := service.CreateActivity(ctx, pluginActivityCommand("MISSING_PLUGIN"))
	if err == nil || !errors.Is(err, marketing.ErrInvalidActivity) {
		t.Fatalf("expected invalid activity for unknown plugin, got %v", err)
	}

	repo := marketing.NewMemoryRepository()
	dirty := activityFromCommand(pluginActivityCommand("MISSING_PLUGIN"))
	dirty.Status = marketing.ActivityActive
	if _, err := repo.CreateActivity(ctx, dirty); err != nil {
		t.Fatalf("seed dirty activity: %v", err)
	}
	runtimeService := NewService(repo)
	result, err := runtimeService.ProcessEvent(ctx, pluginEvent("evt_plugin_missing", "biz_plugin_missing"))
	if err != nil {
		t.Fatalf("runtime unknown plugin should reject activity without failing event: %v", err)
	}
	if len(result.GrantOrders) != 0 || len(result.OutboxMessages) != 0 {
		t.Fatalf("expected no grants/outbox for unknown runtime plugin, got %+v", result)
	}
	if len(result.Participations) != 1 || result.Participations[0].Status != marketing.ParticipationRejected {
		t.Fatalf("expected rejected participation, got %+v", result.Participations)
	}
	if result.RuleResults[0].ReasonCode != activityplugin.ReasonPluginNotRegistered {
		t.Fatalf("reason=%s, want %s", result.RuleResults[0].ReasonCode, activityplugin.ReasonPluginNotRegistered)
	}
}

func TestSimulateEventIncludesPluginPreviewWithoutPersistence(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	if err := service.RegisterActivityPlugin(testActivityPlugin{
		code:            "LOTTERY_TEST",
		activityTypes:   []string{"LOTTERY"},
		precheckAllowed: true,
		context:         map[string]any{"drawChance": "1", "amount": "20.00"},
	}); err != nil {
		t.Fatalf("register plugin: %v", err)
	}
	if _, err := createApprovedPluginActivity(ctx, service, pluginActivityCommand("LOTTERY_TEST")); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	result, err := service.SimulateEvent(ctx, pluginEvent("evt_plugin_sim", "biz_plugin_sim"))
	if err != nil {
		t.Fatalf("simulate event: %v", err)
	}
	if len(result.MatchedActivities) != 1 {
		t.Fatalf("expected matched activity, got %+v", result)
	}
	pluginPreview := result.MatchedActivities[0].Plugin
	if pluginPreview == nil || pluginPreview.PluginCode != "LOTTERY_TEST" || len(pluginPreview.StateChangeIntents) != 1 {
		t.Fatalf("expected plugin preview with intent, got %+v", pluginPreview)
	}
	activities, err := repo.ListActivities(ctx)
	if err != nil || len(activities) != 1 {
		t.Fatalf("expected only activity persisted after simulate, activities=%d err=%v", len(activities), err)
	}
}

func pluginActivityCommand(pluginCode string) marketing.CreateActivityCommand {
	now := time.Now().UTC()
	ruleConfig := fmt.Sprintf(`{"logic":"AND","conditions":[{"field":"state.plugin.%s.drawChance","op":">","value":"0"}]}`, pluginCode)
	rewardConfig := fmt.Sprintf(`{"baseField":"state.plugin.%s.amount","rate":"0.5","currency":"USD","targetRef":"https://wallet.example.com/reward"}`, pluginCode)
	return marketing.CreateActivityCommand{
		TenantID:      "site_001",
		Code:          "LOTTERY_PLUGIN",
		Name:          "Lottery Plugin",
		Type:          "LOTTERY",
		Status:        marketing.ActivityDraft,
		TriggerEvents: []string{"LOTTERY_DRAW"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(time.Hour),
		Timezone:      "UTC",
		Priority:      100,
		PluginCode:    pluginCode,
		Version:       1,
		Config:        []byte(`{"metadata":{"owner":"growth"}}`),
		Rules: []marketing.Rule{{
			Code:    "HAS_DRAW_CHANCE",
			Phase:   marketing.RulePhaseQualify,
			Type:    marketing.RuleTypeCondition,
			Config:  []byte(ruleConfig),
			Sort:    1,
			Enabled: true,
			Version: 1,
		}},
		Rewards: []marketing.Reward{{
			Code:          "PLUGIN_RATIO",
			Type:          marketing.RewardBonus,
			CalculateType: marketing.CalculateRatio,
			Config:        []byte(rewardConfig),
			GrantMode:     marketing.GrantAuto,
			GrantChannel:  marketing.GrantHTTP,
			Enabled:       true,
			Version:       1,
		}},
	}
}

func createApprovedPluginActivity(ctx context.Context, service *Service, cmd marketing.CreateActivityCommand) (marketing.Activity, error) {
	created, err := service.CreateActivity(ctx, cmd)
	if err != nil {
		return marketing.Activity{}, err
	}
	if _, err := service.SubmitActivityApproval(ctx, created.ID, marketing.SubmitApprovalCommand{Operator: "ops", ExpectedVersion: created.Version}); err != nil {
		return marketing.Activity{}, err
	}
	if _, err := service.ApproveActivity(ctx, created.ID, marketing.ApprovalActionCommand{Operator: "lead"}); err != nil {
		return marketing.Activity{}, err
	}
	approved, err := service.ApproveActivity(ctx, created.ID, marketing.ApprovalActionCommand{Operator: "director"})
	if err != nil {
		return marketing.Activity{}, err
	}
	active := marketing.ActivityActive
	return service.UpdateActivity(ctx, approved.ID, marketing.UpdateActivityCommand{Operator: "ops", Status: &active})
}

func activityFromCommand(cmd marketing.CreateActivityCommand) marketing.Activity {
	now := time.Now().UTC()
	return marketing.Activity{
		ID:            "act_dirty_plugin",
		TenantID:      cmd.TenantID,
		Code:          cmd.Code,
		Name:          cmd.Name,
		Type:          cmd.Type,
		Status:        cmd.Status,
		TriggerEvents: cmd.TriggerEvents,
		StartTime:     cmd.StartTime,
		EndTime:       cmd.EndTime,
		Timezone:      cmd.Timezone,
		Priority:      cmd.Priority,
		PluginCode:    cmd.PluginCode,
		Version:       1,
		Approval: marketing.ApprovalSnapshot{
			Status:          marketing.ApprovalApproved,
			Submitter:       "ops",
			L1Approver:      "lead",
			L2Approver:      "director",
			ActivityVersion: 1,
			HighRisk:        true,
		},
		Config:    cmd.Config,
		Rules:     cmd.Rules,
		Rewards:   cmd.Rewards,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func pluginEvent(eventID string, bizID string) marketing.ActivityEvent {
	return marketing.ActivityEvent{
		EventID:      eventID,
		EventType:    "LOTTERY_DRAW",
		SourceSystem: "game",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "user_001"},
		Biz:          marketing.EventBiz{BizType: "LOTTERY", BizID: bizID},
		OccurredAt:   time.Now().UTC(),
		Attributes:   map[string]any{"amount": "999.00"},
	}
}
