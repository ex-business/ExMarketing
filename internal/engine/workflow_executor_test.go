package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
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
		"meta": {"activityId": "act_workflow_test", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry","code":"Event In","title":"事件入口","position":{"x":80,"y":120},"outputs":["raw_event"]},
			{"id":"match","type":"match","kind":"logic","code":"MATCH","title":"活动匹配","position":{"x":410,"y":120},"inputs":["raw_event"],"outputs":["matched_event"],"config":{"inputMapping":{"event_context":{"sourceNodeId":"event-in","outputKey":"raw_event","path":"$.subject.id"}}}},
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
	matchInput := map[string]any{}
	for _, nodeExecution := range nodeExecutions {
		if nodeExecution.NodeID == "match" {
			if err := json.Unmarshal(nodeExecution.Input, &matchInput); err != nil {
				t.Fatalf("unmarshal match input: %v", err)
			}
		}
	}
	resolvedInputs, ok := matchInput["resolvedInputs"].(map[string]any)
	if !ok {
		t.Fatalf("expected resolvedInputs in match node execution input, got %+v", matchInput)
	}
	if resolvedInputs["event_context"] != "u_001" {
		t.Fatalf("expected inputMapping path to resolve user id, got %+v", resolvedInputs)
	}
}

func TestPublishWorkflowVersionRequiresValidationPass(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:          "act_workflow_publish_validation",
		TenantID:    "site_001",
		WorkspaceID: "workspace-1",
		FolderID:    "folder-1",
		Code:        "WORKFLOW_PUBLISH_VALIDATION",
		Name:        "Workflow Publish Validation",
		Type:        "FIRST_DEPOSIT",
		Status:      marketing.ActivityActive,
		StartTime:   now.Add(-time.Hour),
		EndTime:     now.Add(time.Hour),
		Timezone:    "UTC",
		Priority:    10,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	definitionMissingMetadata := json.RawMessage(`{
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry"},
			{"id":"match","type":"match"}
		],
		"edges": [
			{"id":"event-in-match","source":"event-in","target":"match"}
		]
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definitionMissingMetadata}); err != nil {
		t.Fatalf("save workflow draft: %v", err)
	}

	_, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil {
		t.Fatalf("expected publish to fail")
	}
	if !errors.Is(err, marketing.ErrInvalidWorkflow) || !strings.Contains(err.Error(), "WORKFLOW_METADATA_ACTIVITY_ID_REQUIRED") {
		t.Fatalf("expected publish validation failure, got %v", err)
	}
}

func TestSaveWorkflowDraftAllowsValidationInvalidDefinition(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:          "act_invalid_draft_allowed",
		TenantID:    "site_001",
		WorkspaceID: "workspace-1",
		FolderID:    "folder-1",
		Code:        "INVALID_DRAFT_ALLOWED",
		Name:        "Invalid Draft Allowed",
		Type:        "STUDIO_WORKFLOW",
		Status:      marketing.ActivityDraft,
		StartTime:   now.Add(-time.Hour),
		EndTime:     now.Add(time.Hour),
		Timezone:    "UTC",
		Priority:    10,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}

	definitionMissingEntry := json.RawMessage(`{
		"meta": {"activityId": "act_invalid_draft_allowed", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id":"match","type":"match"}
		],
		"edges": []
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definitionMissingEntry}); err != nil {
		t.Fatalf("expected invalid but parseable draft to save: %v", err)
	}
	validation, err := service.ValidateWorkflowDraft(ctx, activity.ID)
	if err != nil {
		t.Fatalf("validate workflow draft: %v", err)
	}
	if validation.Valid {
		t.Fatalf("expected saved invalid draft to fail validation")
	}
	_, err = service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"})
	if err == nil {
		t.Fatalf("expected invalid draft publish to fail")
	}
	if !errors.Is(err, marketing.ErrInvalidWorkflow) || !strings.Contains(err.Error(), "WORKFLOW_ENTRY_MISSING") {
		t.Fatalf("expected invalid draft publish validation failure, got %v", err)
	}
}
