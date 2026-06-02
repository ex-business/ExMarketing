package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestWorkflowDraftPublishAndExecutionLogs(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)

	now := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	reviewedAt := now
	activity := marketing.Activity{
		ID:            "act_workflow_test",
		TenantID:      "site_001",
		Code:          "WORKFLOW_TEST",
		Name:          "Workflow Test",
		Type:          "FIRST_DEPOSIT",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(24 * time.Hour),
		Timezone:      "UTC",
		Priority:      100,
		Version:       1,
		Approval: marketing.ApprovalSnapshot{
			Status:          marketing.ApprovalApproved,
			ActivityVersion: 1,
			Approver:        "director",
			ReviewedAt:      &reviewedAt,
		},
		Rules: []marketing.Rule{{
			ID:      "rule_amount",
			Code:    "AMOUNT",
			Phase:   marketing.RulePhaseQualify,
			Type:    marketing.RuleTypeCondition,
			Config:  []byte(`{"logic":"AND","conditions":[{"field":"attributes.amount","op":">=","value":"100.00"}]}`),
			Sort:    1,
			Enabled: true,
			Version: 1,
		}},
		Rewards: []marketing.Reward{{
			ID:            "reward_bonus",
			Code:          "BONUS",
			Type:          marketing.RewardBonus,
			CalculateType: marketing.CalculateFixed,
			Config:        []byte(`{"amount":"10.00","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			GrantMode:     marketing.GrantAuto,
			GrantChannel:  marketing.GrantHTTP,
			Enabled:       true,
			Version:       1,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	definition := json.RawMessage(`{
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry","code":"Event In","title":"事件入口","position":{"x":80,"y":120},"outputs":["raw_event"]},
			{"id":"match","type":"match","kind":"logic","code":"MATCH","title":"活动匹配","position":{"x":410,"y":120},"inputs":["raw_event"],"outputs":["matched_event"]},
			{"id":"reward","type":"reward","kind":"reward","code":"REWARD","title":"奖励计算","position":{"x":740,"y":120},"inputs":["matched_event"],"outputs":["reward"]},
			{"id":"outbox","type":"outbox","kind":"dispatch","code":"OUTBOX","title":"发奖队列","position":{"x":1070,"y":120},"inputs":["reward"],"outputs":["HTTP"]}
		],
		"edges": [
			{"id":"event-in-match","source":"event-in","target":"match"},
			{"id":"match-reward","source":"match","target":"reward"},
			{"id":"reward-outbox","source":"reward","target":"outbox"}
		]
	}`)
	draft, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definition})
	if err != nil {
		t.Fatalf("save workflow draft: %v", err)
	}
	if draft.ActivityID != activity.ID || len(draft.Definition) == 0 {
		t.Fatalf("unexpected draft: %+v", draft)
	}
	version, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	if version.Version != 1 || version.Status != marketing.WorkflowVersionActive {
		t.Fatalf("unexpected version: %+v", version)
	}

	result, err := service.ProcessEvent(ctx, marketing.ActivityEvent{
		EventID:      "evt_workflow_test",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u_001"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_001"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "200.00"},
	})
	if err != nil {
		t.Fatalf("process event: %v", err)
	}
	if len(result.Participations) != 1 || len(result.GrantOrders) != 1 {
		t.Fatalf("expected participation and grant order, got %+v", result)
	}

	executions, err := service.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activity.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list workflow executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != marketing.WorkflowExecutionSuccess {
		t.Fatalf("unexpected executions: %+v", executions)
	}
	nodeExecutions, err := service.ListWorkflowNodeExecutions(ctx, executions[0].ID)
	if err != nil {
		t.Fatalf("list node executions: %v", err)
	}
	if len(nodeExecutions) != 4 {
		t.Fatalf("expected four node executions, got %+v", nodeExecutions)
	}
	for _, nodeExecution := range nodeExecutions {
		if nodeExecution.Status == marketing.WorkflowNodeFailed {
			t.Fatalf("unexpected failed node execution: %+v", nodeExecution)
		}
	}
}
