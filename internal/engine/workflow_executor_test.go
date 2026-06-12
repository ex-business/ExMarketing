package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/fact"
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

func TestWorkflowQuotaNodeRecordsCommittedReservations(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	reviewedAt := now
	activity := marketing.Activity{
		ID:            "act_workflow_quota_history",
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          "WORKFLOW_QUOTA_HISTORY",
		Name:          "Workflow Quota History",
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
	for _, quota := range []marketing.Quota{
		{ID: "quota_budget", ActivityID: activity.ID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_user", ActivityID: activity.ID, Type: marketing.QuotaUserLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "u_quota", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_daily", ActivityID: activity.ID, Type: marketing.QuotaDailyLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "u_quota:2026-06-02", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activity.ID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "BONUS", TotalValue: "10", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repo.CreateQuota(ctx, quota); err != nil {
			t.Fatalf("create quota %s: %v", quota.ID, err)
		}
	}
	definitionBytes, _ := json.Marshal(workflowTemplateDefinition([]string{"event-in", "inbox", "normalize", "match", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}))
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definitionBytes}); err != nil {
		t.Fatalf("save workflow draft: %v", err)
	}
	if _, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"}); err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	if _, err := service.ProcessEvent(ctx, marketing.ActivityEvent{
		EventID:      "evt_workflow_quota_history",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u_quota"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_quota"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "200.00"},
	}); err != nil {
		t.Fatalf("process event: %v", err)
	}
	executions, err := service.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activity.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected one execution, got %+v", executions)
	}
	nodes, err := service.ListWorkflowNodeExecutions(ctx, executions[0].ID)
	if err != nil {
		t.Fatalf("list node executions: %v", err)
	}
	var quotaOutput map[string]any
	for _, node := range nodes {
		if node.NodeID == "quota" {
			if err := json.Unmarshal(node.Output, &quotaOutput); err != nil {
				t.Fatalf("unmarshal quota output: %v", err)
			}
		}
	}
	quotaHold, ok := quotaOutput["quota_hold"].(map[string]any)
	if !ok {
		t.Fatalf("expected quota_hold output, got %+v", quotaOutput)
	}
	committed, ok := quotaHold["committedReservations"].([]any)
	if !ok || len(committed) != 4 {
		t.Fatalf("expected four committed reservations, got %+v", quotaHold)
	}
	first := committed[0].(map[string]any)
	if first["id"] == "" || first["reservationStatus"] != string(marketing.QuotaReservationFrozen) {
		t.Fatalf("expected committed reservation id and status, got %+v", first)
	}
}

func TestWorkflowExecutorFollowsRouteRules(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 8, 30, 0, 0, time.UTC)
	reviewedAt := now
	activity := marketing.Activity{
		ID:            "act_workflow_route_test",
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          "WORKFLOW_ROUTE_TEST",
		Name:          "Workflow Route Test",
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
		"meta": {"activityId": "act_workflow_route_test", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口"},
			{"id":"qualify","type":"qualify","kind":"logic","title":"资格校验"},
			{"id":"reward","type":"reward","kind":"reward","title":"奖励计算"},
			{"id":"limit","type":"limit","kind":"quota","title":"次数限制"},
			{"id":"audit","type":"audit","kind":"audit","title":"审计记录"}
		],
		"edges": [
			{"id":"event-in-qualify","source":"event-in","target":"qualify"},
			{"id":"qualify-pass","source":"qualify","target":"reward","route":{"type":"PASS","key":"PASS","label":"PASS","expression":"decision == \"PASS\""}},
			{"id":"qualify-block","source":"qualify","target":"limit","route":{"type":"BLOCK","key":"BLOCK","label":"BLOCK","expression":"decision == \"BLOCK\""}},
			{"id":"qualify-risk","source":"qualify","target":"audit","route":{"type":"RISK","key":"RISK","label":"RISK","expression":"decision == \"RISK\"","default":true}}
		]
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "tester", Definition: definition}); err != nil {
		t.Fatalf("save workflow draft: %v", err)
	}
	if _, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"}); err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	if _, err := service.ProcessEvent(ctx, marketing.ActivityEvent{
		EventID:      "evt_workflow_route_test",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u_route"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_route"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "200.00"},
	}); err != nil {
		t.Fatalf("process event: %v", err)
	}
	executions, err := service.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activity.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list workflow executions: %v", err)
	}
	executionOutput := map[string]any{}
	if err := json.Unmarshal(executions[0].Output, &executionOutput); err != nil {
		t.Fatalf("unmarshal execution output: %v", err)
	}
	routeTrace, ok := executionOutput["routeTrace"].([]any)
	if !ok || len(routeTrace) == 0 {
		t.Fatalf("expected route trace in execution output, got %+v", executionOutput)
	}
	nodeExecutions, err := service.ListWorkflowNodeExecutions(ctx, executions[0].ID)
	if err != nil {
		t.Fatalf("list node executions: %v", err)
	}
	statusByNode := map[string]marketing.WorkflowNodeExecutionStatus{}
	for _, nodeExecution := range nodeExecutions {
		statusByNode[nodeExecution.NodeID] = nodeExecution.Status
	}
	for _, nodeID := range []string{"event-in", "qualify", "reward"} {
		if statusByNode[nodeID] != marketing.WorkflowNodeSuccess {
			t.Fatalf("expected node %s to succeed, got %+v", nodeID, statusByNode)
		}
	}
	for _, nodeID := range []string{"limit", "audit"} {
		if statusByNode[nodeID] != marketing.WorkflowNodeSkipped {
			t.Fatalf("expected node %s to be persisted as skipped by route rules, got %+v", nodeID, statusByNode)
		}
	}
}

func TestCompareWorkflowExecutionsReturnsCompactDiff(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC)
	currentFinished := now.Add(300 * time.Millisecond)
	baselineFinished := now.Add(220 * time.Millisecond)
	current, err := repo.CreateWorkflowExecution(ctx, marketing.WorkflowExecution{
		ID:                "exec_current",
		TenantID:          "site_001",
		ActivityID:        "act_compare",
		WorkflowVersionID: "version_1",
		WorkflowVersion:   1,
		EventID:           "evt_current",
		Status:            marketing.WorkflowExecutionFailed,
		StartedAt:         now,
		FinishedAt:        &currentFinished,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("create current execution: %v", err)
	}
	baseline, err := repo.CreateWorkflowExecution(ctx, marketing.WorkflowExecution{
		ID:                "exec_baseline",
		TenantID:          "site_001",
		ActivityID:        "act_compare",
		WorkflowVersionID: "version_1",
		WorkflowVersion:   1,
		EventID:           "evt_baseline",
		Status:            marketing.WorkflowExecutionSuccess,
		StartedAt:         now,
		FinishedAt:        &baselineFinished,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("create baseline execution: %v", err)
	}
	currentNodeFinished := now.Add(120 * time.Millisecond)
	baselineNodeFinished := now.Add(90 * time.Millisecond)
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:          "node_current_inbox",
		TenantID:    "site_001",
		ExecutionID: current.ID,
		NodeID:      "inbox",
		NodeTitle:   "幂等收件箱",
		Status:      marketing.WorkflowNodeFailed,
		Input:       json.RawMessage(`{"raw_event":{"userId":"user_1"}}`),
		Output:      json.RawMessage(`{"errorCode":"INBOX_WRITE_FAILED"}`),
		StartedAt:   now,
		FinishedAt:  &currentNodeFinished,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create current node: %v", err)
	}
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:          "node_baseline_inbox",
		TenantID:    "site_001",
		ExecutionID: baseline.ID,
		NodeID:      "inbox",
		NodeTitle:   "幂等收件箱",
		Status:      marketing.WorkflowNodeSuccess,
		Input:       json.RawMessage(`{"raw_event":{"userId":"user_0"}}`),
		Output:      json.RawMessage(`{"event_context":{"accepted":true}}`),
		StartedAt:   now,
		FinishedAt:  &baselineNodeFinished,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create baseline node: %v", err)
	}
	compare, err := service.CompareWorkflowExecutions(ctx, current.ID, baseline.ID)
	if err != nil {
		t.Fatalf("compare executions: %v", err)
	}
	if compare.Current.ID != current.ID || compare.Baseline.ID != baseline.ID {
		t.Fatalf("unexpected compare executions: %+v", compare)
	}
	if len(compare.CurrentFailures) != 1 || compare.CurrentFailures[0] != "幂等收件箱" {
		t.Fatalf("expected current failure label, got %+v", compare.CurrentFailures)
	}
	if compare.CurrentDurationMs != 120 || compare.BaselineDurationMs != 90 || compare.DurationDeltaMs != 30 {
		t.Fatalf("unexpected durations: %+v", compare)
	}
	if len(compare.PayloadChanges) != 1 || len(compare.PayloadChanges[0].Input) == 0 || compare.PayloadChanges[0].Input[0].Path != "$.raw_event.userId" {
		t.Fatalf("expected payload diff, got %+v", compare.PayloadChanges)
	}
	limited, err := service.CompareWorkflowExecutionsWithOptions(ctx, current.ID, baseline.ID, marketing.WorkflowExecutionCompareOptions{
		MaxPayloadChangeNodes: 1,
		MaxJSONDiffRows:       1,
	})
	if err != nil {
		t.Fatalf("compare executions with options: %v", err)
	}
	if limited.PayloadChangeLimit != 1 || limited.JSONDiffLimit != 1 {
		t.Fatalf("expected compare limits to be returned, got %+v", limited)
	}
	if len(limited.PayloadChanges) != 1 || len(limited.PayloadChanges[0].Input) > 1 || len(limited.PayloadChanges[0].Output) > 1 {
		t.Fatalf("expected compact payload diff, got %+v", limited.PayloadChanges)
	}
}

func TestWorkflowNodeExecutionPolicyRetriesAndRecordsAttempt(t *testing.T) {
	ctx := context.Background()
	node := marketing.WorkflowNode{
		ID:     "retry-node",
		Type:   "code",
		Config: json.RawMessage(`{"timeoutSeconds":1,"maxRetries":1}`),
	}
	attempts := 0
	result := executeWorkflowNodeWithPolicy(ctx, node, workflowRuntime{}, func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		attempts++
		if attempts == 1 {
			return nodeFailed("temporary failure", map[string]any{"temporary": true})
		}
		return nodeSuccess(map[string]any{"ok": true})
	})
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected retry to recover, got %+v", result)
	}
	if attempts != 2 || result.Output["attempt"] != 2 {
		t.Fatalf("expected two attempts to be recorded, attempts=%d output=%+v", attempts, result.Output)
	}
}

func TestWorkflowNodeExecutionPolicyTimesOut(t *testing.T) {
	ctx := context.Background()
	node := marketing.WorkflowNode{
		ID:     "timeout-node",
		Type:   "code",
		Config: json.RawMessage(`{"timeoutSeconds":0.01}`),
	}
	result := executeWorkflowNodeWithPolicy(ctx, node, workflowRuntime{}, func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		select {
		case <-ctx.Done():
			return nodeFailed(ctx.Err().Error(), nil)
		case <-time.After(100 * time.Millisecond):
			return nodeSuccess(nil)
		}
	})
	if result.Status != marketing.WorkflowNodeFailed || !strings.Contains(result.Error, "timeout") {
		t.Fatalf("expected timeout failure, got %+v", result)
	}
}

func TestCancelWorkflowExecutionMarksRunningExecutionCanceled(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC)
	execution := marketing.WorkflowExecution{
		ID:                "wfexec_cancel",
		TenantID:          "site_001",
		ActivityID:        "activity_cancel",
		WorkflowVersionID: "wfver_cancel",
		WorkflowVersion:   1,
		Status:            marketing.WorkflowExecutionRunning,
		StartedAt:         now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if _, err := repo.CreateWorkflowExecution(ctx, execution); err != nil {
		t.Fatalf("create workflow execution: %v", err)
	}
	canceled, err := service.CancelWorkflowExecution(ctx, execution.ID, marketing.CancelWorkflowExecutionCommand{
		Operator: "tester",
		Reason:   "manual stop",
	})
	if err != nil {
		t.Fatalf("cancel workflow execution: %v", err)
	}
	if canceled.Status != marketing.WorkflowExecutionCanceled || canceled.ErrorMessage != "manual stop" || canceled.FinishedAt == nil {
		t.Fatalf("unexpected canceled execution: %+v", canceled)
	}
}

func TestCoreWorkflowNodeHandlersProduceStableOutputs(t *testing.T) {
	ctx := context.Background()
	runtime := testWorkflowHandlerRuntime(true)
	service := NewService(marketing.NewMemoryRepository())
	decimalCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{ResolvedInput: map[string]any{"amount_expr": "12.345"}})
	cases := []struct {
		name      string
		node      marketing.WorkflowNode
		handler   workflowNodeHandler
		outputKey string
		ctx       context.Context
	}{
		{name: "event in", node: marketing.WorkflowNode{ID: "node_event", Type: "event-in"}, handler: entryWorkflowNodeHandler, outputKey: "raw_event"},
		{name: "inbox", node: marketing.WorkflowNode{ID: "node_inbox", Type: "inbox"}, handler: inboxWorkflowNodeHandler, outputKey: "event_context"},
		{name: "normalize", node: marketing.WorkflowNode{ID: "node_normalize", Type: "normalize", Config: json.RawMessage(`{"defaults":{"channel":"web"}}`)}, handler: normalizeWorkflowNodeHandler, outputKey: "normalized_event"},
		{name: "match", node: marketing.WorkflowNode{ID: "node_match", Type: "match", Config: json.RawMessage(`{"requireActiveActivity":true}`)}, handler: matchWorkflowNodeHandler, outputKey: "matched_activity"},
		{name: "condition", node: marketing.WorkflowNode{ID: "node_condition", Type: "condition"}, handler: decisionWorkflowNodeHandler, outputKey: "stage_result"},
		{name: "qualify", node: marketing.WorkflowNode{ID: "node_qualify", Type: "qualify"}, handler: decisionWorkflowNodeHandler, outputKey: "PASS"},
		{name: "reward", node: marketing.WorkflowNode{ID: "node_reward", Type: "reward"}, handler: rewardWorkflowNodeHandler, outputKey: "reward_items"},
		{name: "limit", node: marketing.WorkflowNode{ID: "node_limit", Type: "limit", Config: json.RawMessage(`{"scope":"USER","maxCount":2,"period":"DAY"}`)}, handler: limitWorkflowNodeHandler, outputKey: "limit_hold"},
		{name: "quota", node: marketing.WorkflowNode{ID: "node_quota", Type: "quota", Config: json.RawMessage(`{"quotaCode":"BUDGET","consumeMode":"ATOMIC"}`)}, handler: quotaWorkflowNodeHandler, outputKey: "quota_hold"},
		{name: "participation", node: marketing.WorkflowNode{ID: "node_participation", Type: "participation"}, handler: participationWorkflowNodeHandler, outputKey: "participation_id"},
		{name: "grant order", node: marketing.WorkflowNode{ID: "node_grant", Type: "grant-order"}, handler: grantOrderWorkflowNodeHandler, outputKey: "grant_order"},
		{name: "outbox", node: marketing.WorkflowNode{ID: "node_outbox", Type: "outbox"}, handler: outboxWorkflowNodeHandler, outputKey: "HTTP"},
		{name: "fact query", node: marketing.WorkflowNode{ID: "node_fact", Type: "fact-query", Config: json.RawMessage(`{"factKeys":["profile.level"],"failureAction":"USE_EMPTY"}`)}, handler: service.factQueryWorkflowNodeHandler, outputKey: "facts"},
		{name: "switch", node: marketing.WorkflowNode{ID: "node_switch", Type: "switch"}, handler: decisionWorkflowNodeHandler, outputKey: "route"},
		{name: "code", node: marketing.WorkflowNode{ID: "node_code", Type: "code", Config: json.RawMessage(`{"language":"EXPRESSION","source":"$.subject.id","outputPath":"subject_id"}`)}, handler: codeWorkflowNodeHandler, outputKey: "code_result"},
		{name: "decimal", node: marketing.WorkflowNode{ID: "node_decimal", Type: "decimal"}, handler: decimalWorkflowNodeHandler, outputKey: "decimal_amount", ctx: decimalCtx},
		{name: "callback", node: marketing.WorkflowNode{ID: "node_callback", Type: "callback", Config: json.RawMessage(`{"provider":"wallet","requireSignature":true}`)}, handler: callbackWorkflowNodeHandler, outputKey: "callback_result"},
		{name: "audit", node: marketing.WorkflowNode{ID: "node_audit", Type: "audit", Config: json.RawMessage(`{"action":"WORKFLOW_AUDIT","includePayload":true}`)}, handler: auditWorkflowNodeHandler, outputKey: "audit_log"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			runCtx := ctx
			if tt.ctx != nil {
				runCtx = tt.ctx
			}
			result := tt.handler(runCtx, tt.node, runtime)
			if result.Status != marketing.WorkflowNodeSuccess {
				t.Fatalf("expected success, got %+v", result)
			}
			if _, ok := result.Output[tt.outputKey]; !ok {
				t.Fatalf("expected output key %q, got %+v", tt.outputKey, result.Output)
			}
			if _, ok := result.Output["audit"]; !ok {
				t.Fatalf("expected audit fields, got %+v", result.Output)
			}
		})
	}
}

func TestStandaloneFactQueryWorkflowNodeHandlerFailsInsteadOfPlaceholder(t *testing.T) {
	result := factQueryWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{
		ID:     "fact-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"factKeys":["USER_PROFILE"]}`),
	}, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "FACT_QUERY_SERVICE_REQUIRED" {
		t.Fatalf("expected service-required failure, got %+v", result)
	}
}

func TestAdvancedAndOpsWorkflowNodeHandlersDeclareExecutionAuthority(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	cases := []struct {
		name    string
		node    marketing.WorkflowNode
		handler workflowNodeHandler
		want    string
	}{
		{name: "retry", node: marketing.WorkflowNode{ID: "retry", Type: "retry"}, handler: retryWorkflowNodeHandler, want: "OutboxWorker"},
		{name: "dlq", node: marketing.WorkflowNode{ID: "dlq", Type: "dlq"}, handler: dlqWorkflowNodeHandler, want: "OutboxWorker/Admin DLQ API"},
		{name: "http-call", node: marketing.WorkflowNode{ID: "http-call", Type: "http-call"}, handler: dispatchWorkflowNodeHandler("HTTP"), want: "OutboxWorker"},
		{name: "reconcile", node: marketing.WorkflowNode{ID: "reconcile", Type: "reconcile"}, handler: reconcileWorkflowNodeHandler, want: "Reconciliation/Admin API"},
		{name: "alert", node: marketing.WorkflowNode{ID: "alert", Type: "alert"}, handler: alertWorkflowNodeHandler, want: "Metrics/Alertmanager"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.handler(context.Background(), tt.node, runtime)
			if result.Output["executionAuthority"] != tt.want {
				t.Fatalf("expected executionAuthority=%s, got %+v", tt.want, result.Output)
			}
			if result.Output["executionMode"] != "EXPLAIN_ONLY" {
				t.Fatalf("expected EXPLAIN_ONLY execution mode, got %+v", result.Output)
			}
		})
	}

	callback := callbackWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{ID: "callback", Type: "callback", Config: json.RawMessage(`{"provider":"wallet","requireSignature":true}`)}, runtime)
	if callback.Output["executionMode"] != "EXTERNAL_ENDPOINT" || callback.Output["terminalAuthority"] != "RewardCallback/Reconciliation" {
		t.Fatalf("expected callback external endpoint authority, got %+v", callback.Output)
	}
}

func TestWorkflowNodeSummariesExplainTraceNodes(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	finishedAt := now.Add(15 * time.Millisecond)
	summaries := workflowNodeSummaries([]marketing.WorkflowNodeExecution{
		{
			ID:           "node_1",
			NodeID:       "qualify",
			NodeType:     "qualify",
			Status:       marketing.WorkflowNodeSuccess,
			Input:        json.RawMessage(`{"event_context":{"subject":{"id":"u1"}},"matched_activity":{"activityId":"act_1"}}`),
			Output:       json.RawMessage(`{"decision":"PASS","PASS":true,"ruleResult":{"decision":"PASS"}}`),
			StartedAt:    now,
			FinishedAt:   &finishedAt,
			CreatedAt:    now,
			UpdatedAt:    finishedAt,
			ErrorMessage: "",
		},
		{
			ID:           "node_2",
			NodeID:       "quota",
			NodeType:     "quota",
			Status:       marketing.WorkflowNodeFailed,
			Output:       json.RawMessage(`{"errorCode":"QUOTA_INSUFFICIENT"}`),
			ErrorMessage: "quota insufficient",
			StartedAt:    now,
			FinishedAt:   &finishedAt,
			CreatedAt:    now,
			UpdatedAt:    finishedAt,
		},
	})
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %+v", summaries)
	}
	if summaries[0].Decision != "PASS" || summaries[0].DurationMs != 15 || !strings.Contains(summaries[0].Explanation, "decision PASS") {
		t.Fatalf("unexpected qualify summary: %+v", summaries[0])
	}
	if summaries[1].ErrorCode != "QUOTA_INSUFFICIENT" || !strings.Contains(summaries[1].Explanation, "QUOTA_INSUFFICIENT") {
		t.Fatalf("unexpected quota summary: %+v", summaries[1])
	}
}

func TestCoreWorkflowNodeHandlersValidateInputsAndConfig(t *testing.T) {
	ctx := context.Background()
	runtime := testWorkflowHandlerRuntime(true)
	cases := []struct {
		name    string
		node    marketing.WorkflowNode
		handler workflowNodeHandler
		want    marketing.WorkflowNodeExecutionStatus
		code    string
	}{
		{
			name:    "event in rejects unlisted event type",
			node:    marketing.WorkflowNode{ID: "event", Type: "event-in", Config: json.RawMessage(`{"eventTypes":["WITHDRAW"]}`)},
			handler: entryWorkflowNodeHandler,
			want:    marketing.WorkflowNodeFailed,
			code:    "EVENT_IN_TYPE_NOT_ALLOWED",
		},
		{
			name:    "code requires source",
			node:    marketing.WorkflowNode{ID: "code", Type: "code", Config: json.RawMessage(`{"language":"EXPRESSION"}`)},
			handler: codeWorkflowNodeHandler,
			want:    marketing.WorkflowNodeFailed,
			code:    "CODE_CONFIG_SOURCE_REQUIRED",
		},
		{
			name:    "reward skips when not qualified",
			node:    marketing.WorkflowNode{ID: "reward", Type: "reward"},
			handler: rewardWorkflowNodeHandler,
			want:    marketing.WorkflowNodeSkipped,
			code:    "REWARD_NOT_QUALIFIED",
		},
		{
			name:    "callback skips without grant order",
			node:    marketing.WorkflowNode{ID: "callback", Type: "callback"},
			handler: callbackWorkflowNodeHandler,
			want:    marketing.WorkflowNodeSkipped,
			code:    "CALLBACK_NO_GRANT_ORDER",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			testRuntime := runtime
			if tt.node.Type == "reward" {
				testRuntime = testWorkflowHandlerRuntime(false)
			}
			if tt.node.Type == "callback" {
				testRuntime.GrantOrders = nil
			}
			result := tt.handler(ctx, tt.node, testRuntime)
			if result.Status != tt.want {
				t.Fatalf("status=%s, want %s: %+v", result.Status, tt.want, result)
			}
			if got := workflowOutputString(result.Output["errorCode"]); got != tt.code {
				t.Fatalf("errorCode=%q, want %q: %+v", got, tt.code, result.Output)
			}
		})
	}
}

func TestNormalizeWorkflowNodeHandlerAppliesInputMappingConfig(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"raw_event": map[string]any{
				"eventId": "evt_custom",
				"attributes": map[string]any{
					"amount":  "300.50",
					"channel": "app",
				},
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "normalize",
		Type: "normalize",
		Config: json.RawMessage(`{
			"mapping": {
				"order.amount": "$.attributes.amount",
				"channel": "$.attributes.channel"
			},
			"defaults": {
				"source": "workflow"
			}
		}`),
	}

	result := normalizeWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	normalized := result.Output["normalized_event"].(map[string]any)
	order := normalized["order"].(map[string]any)
	if order["amount"] != "300.50" || normalized["channel"] != "app" || normalized["source"] != "workflow" {
		t.Fatalf("unexpected normalized output: %+v", normalized)
	}
}

func TestQualifyWorkflowNodeHandlerEvaluatesActivityRules(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rules = []marketing.Rule{{
		Code:    "MIN_AMOUNT",
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeCondition,
		Enabled: true,
		Config:  json.RawMessage(`{"logic":"AND","conditions":[{"field":"event.attributes.amount","op":">=","value":"250"}]}`),
	}}
	node := marketing.WorkflowNode{ID: "qualify", Type: "qualify"}

	result := decisionWorkflowNodeHandler(context.Background(), node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.Output["decision"] != "BLOCK" || result.Output["PASS"] != false || result.Output["BLOCK"] != true {
		t.Fatalf("expected rule rejection route, got %+v", result.Output)
	}
	ruleResult, ok := result.Output["ruleResult"].(map[string]any)
	if !ok || ruleResult["failedStage"] != string(marketing.RuleStageQualify) {
		t.Fatalf("expected structured rule result, got %+v", result.Output["ruleResult"])
	}
}

func TestConditionWorkflowNodeHandlerEvaluatesJSONLogic(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"fact": map[string]any{"amount": "120.00", "vip": true},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "condition",
		Type: "condition",
		Config: json.RawMessage(`{
			"expressionLanguage": "JSON_LOGIC",
			"expression": "{\"and\":[{\">=\":[{\"var\":\"fact.amount\"},100]},{\"==\":[{\"var\":\"fact.vip\"},true]}]}",
			"errorAction": "BLOCK"
		}`),
	}
	result := decisionWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(false))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected condition success, got %+v", result)
	}
	if result.Output["IF"] != true || result.Output["ELSE"] != false || result.Output["decision"] != "PASS" {
		t.Fatalf("expected JSON logic IF route, got %+v", result.Output)
	}
}

func TestConditionWorkflowNodeHandlerRejectsUnsupportedExpressionLanguage(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:   "condition",
		Type: "condition",
		Config: json.RawMessage(`{
			"expressionLanguage": "CEL",
			"expression": "$.fact.amount >= 100",
			"errorAction": "BLOCK"
		}`),
	}
	result := decisionWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "CONDITION_LANGUAGE_UNSUPPORTED" || !strings.Contains(result.Error, "CEL") {
		t.Fatalf("expected unsupported CEL failure, got %+v", result)
	}
}

func TestRewardWorkflowNodeHandlerCalculatesConfiguredReward(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rewards = []marketing.Reward{{
		ID:            "reward_calc",
		Code:          "BONUS",
		Type:          marketing.RewardBonus,
		CalculateType: marketing.CalculateFixed,
		Config:        json.RawMessage(`{"amount":"12.50","currency":"USD"}`),
		GrantMode:     marketing.GrantAuto,
		GrantChannel:  marketing.GrantHTTP,
		Enabled:       true,
	}}

	result := rewardWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{ID: "reward", Type: "reward"}, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	items := result.Output["reward_items"].([]any)
	item := items[0].(map[string]any)
	if item["amount"] != "12.5000" || item["currency"] != "USD" || item["reasonCode"] != "OK" {
		t.Fatalf("expected calculated reward item, got %+v", item)
	}
}

func TestServiceFactQueryWorkflowNodeHandlerHonorsUseEmptyFailureAction(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys": ["USER_PROFILE"],
			"source": "FACT_SERVICE",
			"failureAction": "USE_EMPTY"
		}`),
	}

	result := service.factQueryWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected USE_EMPTY to continue, got %+v", result)
	}
	if result.Output["failureAction"] != "USE_EMPTY" || result.Output["errorCode"] == "" {
		t.Fatalf("expected explicit fact query degradation output, got %+v", result.Output)
	}
	facts := result.Output["facts"].(map[string]any)
	if len(facts) != 0 {
		t.Fatalf("expected empty facts fallback, got %+v", facts)
	}
}

func TestServiceFactQueryWorkflowNodeHandlerAppliesSourceAndSubjectPath(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Attributes["accountId"] = "account_001"
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys": ["USER_PROFILE"],
			"source": "FACT_SERVICE",
			"subjectPath": "attributes.accountId",
			"failureAction": "USE_EMPTY",
			"timeoutSeconds": 1,
			"cacheTtlSeconds": 30
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(context.Background(), node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected fallback success, got %+v", result)
	}
	subject := result.Output["subject"].(map[string]any)
	if subject["id"] != "account_001" || result.Output["source"] != "FACT_SERVICE" || result.Output["cacheTtlSeconds"] != float64(30) {
		t.Fatalf("expected subject/source/cache config in output, got %+v", result.Output)
	}

	unsupported := node
	unsupported.Config = json.RawMessage(`{"factKeys":["USER_PROFILE"],"source":"WAREHOUSE","failureAction":"USE_EMPTY"}`)
	result = service.factQueryWorkflowNodeHandler(context.Background(), unsupported, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["errorCode"] != "FACT_QUERY_SOURCE_UNSUPPORTED" {
		t.Fatalf("expected unsupported source to degrade with errorCode, got %+v", result)
	}
}

func TestServiceFactQueryWorkflowNodeHandlerQueriesProviderAndReportsCache(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	base := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	if _, err := repo.SaveFactProvider(ctx, marketing.FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		CacheTTLSeconds: 300,
		Enabled:         true,
		TimeoutMS:       5000,
		UpdatedAt:       base,
	}); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	var calls int32
	service := NewService(repo)
	service.factClient = fact.NewClientWithHTTPClient(&http.Client{Transport: engineRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		if strings.Contains(string(body), "traceId") || !strings.Contains(string(body), "DEPOSIT_SUCCESS") {
			t.Fatalf("provider request should use standard context without traceId, got %s", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys": ["USER_PROFILE"],
			"source": "FACT_SERVICE",
			"failureAction": "FAIL",
			"timeoutSeconds": 1,
			"cacheTtlSeconds": 300
		}`),
	}
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.EventType = "DEPOSIT_SUCCESS"
	runtime.Inbox.Event.OccurredAt = base

	first := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected provider query success, got %+v", first)
	}
	facts := first.Output["facts"].(map[string]any)
	profile := facts["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != "VIP3" {
		t.Fatalf("expected mapped provider facts, got %+v", first.Output)
	}
	firstResponses := first.Output["responses"].([]any)
	if firstResponses[0].(map[string]any)["cached"] != false {
		t.Fatalf("expected first provider response to miss cache, got %+v", firstResponses)
	}

	second := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if second.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected cached provider query success, got %+v", second)
	}
	secondResponses := second.Output["responses"].([]any)
	if secondResponses[0].(map[string]any)["cached"] != true {
		t.Fatalf("expected second provider response to hit cache, got %+v", secondResponses)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("provider calls=%d, want 1", calls)
	}
}

func TestLimitAndQuotaWorkflowNodeHandlersEmitReservationPlans(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	limit := limitWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{
		ID:     "limit",
		Type:   "limit",
		Config: json.RawMessage(`{"scope":"USER","scopeKeyTemplate":"${subject.id}:${activityId}","period":"ACTIVITY","maxCount":1}`),
	}, runtime)
	if limit.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected limit success, got %+v", limit)
	}
	limitHold := limit.Output["limit_hold"].(map[string]any)
	if limitHold["scopeKey"] != "user_001:activity_handler" {
		t.Fatalf("unexpected limit scope key: %+v", limitHold)
	}
	limitReservations := limitHold["reservations"].([]any)
	limitReservation := limitReservations[0].(map[string]any)
	if limitReservation["quotaType"] != string(marketing.QuotaUserLimit) || limitReservation["amount"] != "1" {
		t.Fatalf("unexpected limit reservation: %+v", limitReservation)
	}

	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"reward": {"reward_items": []any{map[string]any{"amount": "12.50"}}},
		},
	})
	quota := quotaWorkflowNodeHandler(ctx, marketing.WorkflowNode{
		ID:     "quota",
		Type:   "quota",
		Config: json.RawMessage(`{"quotaType":"BUDGET","scope":"ACTIVITY","scopeKeyTemplate":"${activityId}","amountField":"reward_items[0].amount"}`),
	}, runtime)
	if quota.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected quota success, got %+v", quota)
	}
	quotaHold := quota.Output["quota_hold"].(map[string]any)
	if quotaHold["plannedAmount"] != "12.50" || quotaHold["commitAuthority"] != "CreateParticipationGrantOutboxesWithQuota" {
		t.Fatalf("unexpected quota hold: %+v", quotaHold)
	}
	quotaReservations := quotaHold["reservations"].([]any)
	quotaReservation := quotaReservations[0].(map[string]any)
	if quotaReservation["quotaType"] != string(marketing.QuotaBudget) || quotaReservation["scopeKey"] != "activity_handler" || quotaReservation["amount"] != "12.50" {
		t.Fatalf("unexpected quota reservation: %+v", quotaReservation)
	}
}

func TestCodeWorkflowNodeHandlerExecutesLuaWithWorkflowContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"amount": "200.00"},
		NodeOutputs: map[string]map[string]any{
			"qualify": {
				"decision": "PASS",
				"score":    88,
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "lua_code",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"outputPath": "lua_result",
			"source": "local user_id = ctx.event.subject.id\nlocal decision = ctx.nodes.qualify.decision\nemit({userId = user_id, decision = decision, amount = ctx.node.input.amount, passed = decision == 'PASS'})"
		}`),
	}

	result := codeWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected lua code node success, got %+v", result)
	}
	luaResult, ok := result.Output["lua_result"].(map[string]any)
	if !ok {
		t.Fatalf("expected lua_result map, got %+v", result.Output["lua_result"])
	}
	if luaResult["userId"] != "user_001" || luaResult["decision"] != "PASS" || luaResult["amount"] != "200.00" || luaResult["passed"] != true {
		t.Fatalf("unexpected lua output: %+v", luaResult)
	}
	codeResult, ok := result.Output["code_result"].(map[string]any)
	if !ok || codeResult["value"] == nil {
		t.Fatalf("expected code_result value, got %+v", result.Output["code_result"])
	}
}

func TestCodeWorkflowNodeHandlerRejectsUnsupportedLanguage(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:     "js_code",
		Type:   "code",
		Config: json.RawMessage(`{"language":"JAVASCRIPT","source":"return 1"}`),
	}

	result := codeWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "CODE_LANGUAGE_UNSUPPORTED" {
		t.Fatalf("expected unsupported code language failure, got %+v", result)
	}
}

func TestCodeWorkflowNodeHandlerSupportsNodeAndGlobalVariables(t *testing.T) {
	variables := map[string]any{}
	firstCtx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"amount": "200.00"},
		NodeOutputs:   map[string]map[string]any{},
		Variables:     variables,
	})
	firstNode := marketing.WorkflowNode{
		ID:   "calc",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"source": "local local_only = 'discarded'\nset_var('node', 'bonusAmount', input('amount'))\nset_var('global', 'campaignUserId', ctx.event.subject.id)\nset_var('activity', 'lastDepositAmount', input('amount'))\nset_var('user', 'lastEventId', ctx.event.eventId)\nset_node_var('riskScore', 8)\nemit({route = 'PASS', risk = get_node_var('riskScore')})"
		}`),
	}

	first := codeWorkflowNodeHandler(firstCtx, firstNode, testWorkflowHandlerRuntime(true))
	if first.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected first lua node success, got %+v", first)
	}
	firstResult := first.Output["code_result"].(map[string]any)
	if firstResult["bonusAmount"] != "200.00" || firstResult["route"] != "PASS" || firstResult["risk"] != float64(8) {
		t.Fatalf("expected node vars in first output, got %+v", firstResult)
	}
	if variables["campaignUserId"] != "user_001" {
		t.Fatalf("expected global variable to be stored in workflow variables, got %+v", variables)
	}
	activityVars, _ := variables["activity"].(map[string]any)
	userVars, _ := variables["user"].(map[string]any)
	if activityVars["lastDepositAmount"] != "200.00" || userVars["lastEventId"] != "evt_handler" {
		t.Fatalf("expected namespaced variables, got activity=%+v user=%+v all=%+v", activityVars, userVars, variables)
	}
	if _, ok := firstResult["local_only"]; ok {
		t.Fatalf("lua local variables must not leak into output: %+v", firstResult)
	}

	secondCtx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"calc": firstResult,
		},
		Variables: variables,
	})
	secondNode := marketing.WorkflowNode{
		ID:   "read_vars",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"source": "local amount = node_output('calc', 'bonusAmount')\nlocal attr_amount = event_attr('amount')\nlocal subject_id = get_path(ctx, 'event.subject.id')\nemit({userId = ctx.vars.campaignUserId, viaFunction = get_global('campaignUserId'), viaGetVar = get_var('global', 'campaignUserId'), viaNamespace = ctx.vars.execution.campaignUserId, activityAmount = get_var('activity', 'lastDepositAmount'), userEventId = ctx.vars.user.lastEventId, amount = amount, attrAmount = attr_amount, subjectId = subject_id, amountNumber = to_number(amount), fallback = coalesce('', nil, 'fallback')})"
		}`),
	}
	second := codeWorkflowNodeHandler(secondCtx, secondNode, testWorkflowHandlerRuntime(true))
	if second.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected second lua node success, got %+v", second)
	}
	secondResult := second.Output["code_result"].(map[string]any)
	value := secondResult["value"].(map[string]any)
	if value["userId"] != "user_001" || value["viaFunction"] != "user_001" || value["viaGetVar"] != "user_001" || value["viaNamespace"] != "user_001" || value["activityAmount"] != "200.00" || value["userEventId"] != "evt_handler" || value["amount"] != "200.00" || value["attrAmount"] != "200.00" || value["subjectId"] != "user_001" || value["amountNumber"] != float64(200) || value["fallback"] != "fallback" {
		t.Fatalf("unexpected second lua output: %+v", value)
	}
}

func TestCodeWorkflowNodeHandlerProvidesDecimalAndTimeFunctions(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:   "lua_decimal_time",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"source": "local ms = 1770000000000\nemit({sum = decimal_add('1.1000', '2.2050'), diff = decimal_sub('5', '1.25'), product = decimal_mul('2.5', '4'), quotient = decimal_div('5', '2'), cmp = decimal_compare('2.0000', '2'), day = format_date(ms), sameDay = same_day(ms, ms + 1000), nowType = type(now_ms())})"
		}`),
	}

	result := codeWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	value := result.Output["code_result"].(map[string]any)["value"].(map[string]any)
	if value["sum"] != "3.3050" || value["diff"] != "3.7500" || value["product"] != "10.0000" || value["quotient"] != "2.5000" || value["cmp"] != float64(0) || value["day"] != "2026-02-02" || value["sameDay"] != true || value["nowType"] != "number" {
		t.Fatalf("unexpected decimal/time output: %+v", value)
	}
}

func TestCodeWorkflowNodeHandlerRejectsOversizedLuaOutput(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:   "lua_big_output",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"maxOutputBytes": 1024,
			"source": "emit({payload = string.rep('x', 2048)})"
		}`),
	}

	result := codeWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "CODE_LUA_EXECUTION_FAILED" || !strings.Contains(result.Error, "maxOutputBytes") {
		t.Fatalf("expected oversized output failure, got %+v", result)
	}
}

func TestWorkflowNodeHandlersRespectCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := entryWorkflowNodeHandler(ctx, marketing.WorkflowNode{ID: "event", Type: "event-in"}, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "WORKFLOW_NODE_CONTEXT_CANCELED" {
		t.Fatalf("expected canceled context failure, got %+v", result)
	}
}

func testWorkflowHandlerRuntime(qualified bool) workflowRuntime {
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	decision := marketing.RuleDecisionPassed
	if !qualified {
		decision = marketing.RuleDecisionRejected
	}
	return workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             "inbox_handler",
			IdempotencyKey: "idem_handler",
			ProcessStatus:  marketing.InboxProcessed,
			Event: marketing.ActivityEvent{
				EventID:      "evt_handler",
				EventType:    "DEPOSIT_SUCCESS",
				SourceSystem: "payment",
				TenantID:     "site_001",
				Subject:      marketing.EventSubject{Type: "USER", ID: "user_001"},
				Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_001"},
				OccurredAt:   now,
				Attributes:   map[string]any{"amount": "200.00"},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		Activity: marketing.Activity{
			ID:       "activity_handler",
			TenantID: "site_001",
			Code:     "HANDLER",
			Name:     "Handler Activity",
			Status:   marketing.ActivityActive,
			Rewards: []marketing.Reward{{
				ID:            "reward_handler",
				Code:          "BONUS",
				Type:          marketing.RewardBonus,
				CalculateType: marketing.CalculateFixed,
				GrantMode:     marketing.GrantAuto,
				GrantChannel:  marketing.GrantHTTP,
			}},
		},
		Participation: marketing.Participation{
			ID:         "part_handler",
			TenantID:   "site_001",
			ActivityID: "activity_handler",
			Subject:    marketing.EventSubject{Type: "USER", ID: "user_001"},
			Status:     marketing.ParticipationQualified,
		},
		GrantOrders: []marketing.GrantOrder{{
			ID:              "grant_handler",
			TenantID:        "site_001",
			ActivityID:      "activity_handler",
			ParticipationID: "part_handler",
			RewardType:      marketing.RewardBonus,
			RewardAmount:    "10.00",
			GrantStatus:     marketing.GrantPending,
			GrantChannel:    marketing.GrantHTTP,
		}},
		OutboxMessages: []marketing.OutboxMessage{{
			ID:            "outbox_handler",
			TenantID:      "site_001",
			MessageType:   "REWARD_COMMAND",
			TargetChannel: marketing.GrantHTTP,
			TargetRef:     "https://reward.example.test",
			Payload:       json.RawMessage(`{"billNo":"cmd_handler"}`),
			SendStatus:    marketing.OutboxPending,
		}},
		RuleResult: marketing.ActivityRuleResult{
			ActivityID:   "activity_handler",
			Decision:     decision,
			ReasonCode:   "RULE_OK",
			Reason:       "matched",
			CanReward:    qualified,
			StageResults: nil,
		},
		Qualified: qualified,
	}
}

func TestWorkflowRouteEvaluation(t *testing.T) {
	output := map[string]any{
		"decision":  marketing.RuleDecisionRejected,
		"qualified": false,
		"fact": map[string]any{
			"amount": "120.50",
			"risk": map[string]any{
				"flagged": true,
			},
		},
		"route": "custom_path",
	}
	cases := []struct {
		name       string
		route      marketing.WorkflowEdgeRoute
		wantMatch  bool
		wantDefers bool
	}{
		{
			name:      "route key matches normalized decision",
			route:     marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteBLOCK, Key: "BLOCK"},
			wantMatch: true,
		},
		{
			name:      "expression supports nested numeric comparison",
			route:     marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteRISK, Key: "RISK", Expression: "$.fact.amount >= 100"},
			wantMatch: true,
		},
		{
			name:      "expression supports bool equality",
			route:     marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteIF, Key: "IF", Expression: "qualified == false"},
			wantMatch: true,
		},
		{
			name:      "condition ref supports nested truthy value",
			route:     marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteIF, Key: "IF", ConditionRef: "fact.risk.flagged"},
			wantMatch: true,
		},
		{
			name:      "output route uses route key fallback",
			route:     marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteOutput, Key: "custom_path"},
			wantMatch: true,
		},
		{
			name:       "default route does not match during explicit evaluation",
			route:      marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteRISK, Key: "RISK", Default: true},
			wantDefers: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			edge := marketing.WorkflowEdge{ID: "edge", Source: "branch", Target: "target", Route: &tt.route}
			got := workflowRouteMatches(edge, output)
			if got != tt.wantMatch {
				t.Fatalf("workflowRouteMatches=%v, want %v", got, tt.wantMatch)
			}
			if tt.wantDefers && !workflowRouteIsExplicitDefault(edge) {
				t.Fatalf("expected route to defer as explicit default")
			}
		})
	}
}

func TestWorkflowRouteEvaluationUsesRouteModelNotCanvasLabel(t *testing.T) {
	output := map[string]any{"decision": marketing.RuleDecisionRejected}
	edge := marketing.WorkflowEdge{
		ID:     "qualify-mislabeled",
		Source: "qualify",
		Target: "limit",
		Label:  "PASS",
		Route:  &marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteBLOCK, Key: "BLOCK", Label: "BLOCK"},
	}
	if !workflowRouteMatches(edge, output) {
		t.Fatalf("expected executor to follow route model BLOCK even when canvas label is PASS")
	}
}

func TestWorkflowNextEdgesUsesExplicitDefaultWhenNoRouteMatches(t *testing.T) {
	node := marketing.WorkflowNode{ID: "qualify", Type: "qualify"}
	result := workflowNodeResult{Status: marketing.WorkflowNodeSuccess, Output: map[string]any{"decision": "REVIEW"}}
	edges := []marketing.WorkflowEdge{
		{ID: "qualify-pass", Source: "qualify", Target: "reward", Route: &marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRoutePASS, Key: "PASS", Expression: `decision == "PASS"`}},
		{ID: "qualify-block", Source: "qualify", Target: "limit", Route: &marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteBLOCK, Key: "BLOCK", Expression: `decision == "BLOCK"`}},
		{ID: "qualify-risk", Source: "qualify", Target: "audit", Route: &marketing.WorkflowEdgeRoute{Type: marketing.WorkflowEdgeRouteRISK, Key: "RISK", Default: true}},
	}
	next := workflowNextEdges(node, result, edges)
	if len(next) != 1 || next[0].ID != "qualify-risk" {
		t.Fatalf("expected explicit default route, got %+v", next)
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

func TestWorkflowExecutionPlanReturnsStructuredCompileIssues(t *testing.T) {
	_, err := workflowExecutionPlan(marketing.WorkflowDefinition{
		Nodes: []marketing.WorkflowNode{
			{ID: "event-in", Type: "event-in", Kind: "entry"},
			{ID: "match", Type: "match"},
		},
		Edges: []marketing.WorkflowEdge{
			{ID: "missing-target", Source: "event-in", Target: "missing"},
		},
	})
	var compileErr workflowPlanCompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("expected structured compile error, got %v", err)
	}
	if len(compileErr.Issues) != 1 || compileErr.Issues[0].Code != "WORKFLOW_DAG_EDGE_TARGET_MISSING" || compileErr.Issues[0].EdgeID != "missing-target" {
		t.Fatalf("unexpected compile issues: %+v", compileErr.Issues)
	}
}

func TestWorkflowNodePolicyIncludesRetryAndConcurrencyStrategy(t *testing.T) {
	attempts := 0
	node := marketing.WorkflowNode{
		ID:     "retry-node",
		Type:   "code",
		Config: json.RawMessage(`{"timeoutSeconds":1,"maxRetries":2,"retryBackoffSeconds":0.001,"concurrencyGroup":"reward-writes","concurrencyLimit":3}`),
	}
	result := executeWorkflowNodeWithPolicy(context.Background(), node, workflowRuntime{}, func(context.Context, marketing.WorkflowNode, workflowRuntime) workflowNodeResult {
		attempts++
		if attempts < 3 {
			return nodeFailed("temporary failure", map[string]any{"errorCode": "TEMPORARY"})
		}
		return nodeSuccess(map[string]any{"ok": true})
	})
	if result.Status != marketing.WorkflowNodeSuccess || attempts != 3 {
		t.Fatalf("expected success after retries, got attempts=%d result=%+v", attempts, result)
	}
	if result.Output["concurrencyGroup"] != "reward-writes" || int(result.Output["concurrencyLimit"].(int)) != 3 {
		t.Fatalf("expected concurrency policy in output, got %+v", result.Output)
	}
	if result.Output["retryBackoffMs"].(int64) <= 0 {
		t.Fatalf("expected retry backoff in output, got %+v", result.Output)
	}
}

func TestResumeWorkflowExecutionCreatesResumeTrace(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 6, 11, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:            "act_resume",
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          "RESUME_TEST",
		Name:          "Resume Test",
		Type:          "STUDIO_WORKFLOW",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"DEPOSIT_SUCCESS"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(time.Hour),
		Timezone:      "UTC",
		Priority:      1,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	definition := json.RawMessage(`{
		"meta": {"activityId": "act_resume", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry","outputs":["raw_event"]},
			{"id":"match","type":"match","outputs":["matched_activity"],"config":{"inputMapping":{"event_context":{"sourceNodeId":"event-in","outputKey":"raw_event","path":"$.subject.id"}}}},
			{"id":"audit","type":"audit"}
		],
		"edges": [
			{"id":"event-in-match","source":"event-in","target":"match"},
			{"id":"match-audit","source":"match","target":"audit"}
		]
	}`)
	version, err := repo.CreateWorkflowVersion(ctx, marketing.WorkflowVersion{
		ID:          "wfver_resume",
		TenantID:    activity.TenantID,
		ActivityID:  activity.ID,
		Version:     1,
		Status:      marketing.WorkflowVersionActive,
		Definition:  definition,
		PublishedAt: now,
		PublishedBy: "tester",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}
	finishedAt := now.Add(100 * time.Millisecond)
	source, err := repo.CreateWorkflowExecution(ctx, marketing.WorkflowExecution{
		ID:                "wfexec_resume_source",
		TenantID:          activity.TenantID,
		ActivityID:        activity.ID,
		WorkflowVersionID: version.ID,
		WorkflowVersion:   version.Version,
		EventInboxID:      "inbox_resume",
		EventID:           "evt_resume",
		Status:            marketing.WorkflowExecutionFailed,
		Input: marshalWorkflowPayload(map[string]any{
			"event": marketing.ActivityEvent{
				EventID:      "evt_resume",
				EventType:    "DEPOSIT_SUCCESS",
				SourceSystem: "test",
				TenantID:     activity.TenantID,
				Subject:      marketing.EventSubject{Type: "USER", ID: "u_resume"},
				Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_resume"},
				OccurredAt:   now,
				Attributes:   map[string]any{"amount": "200"},
			},
			"idempotencyKey": "resume-key",
		}),
		ErrorMessage: "match failed",
		StartedAt:    now,
		FinishedAt:   &finishedAt,
		CreatedAt:    now,
		UpdatedAt:    finishedAt,
	})
	if err != nil {
		t.Fatalf("create source execution: %v", err)
	}
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:          "node_resume_event",
		TenantID:    activity.TenantID,
		ExecutionID: source.ID,
		NodeID:      "event-in",
		NodeType:    "event-in",
		Status:      marketing.WorkflowNodeSuccess,
		Output:      json.RawMessage(`{"raw_event":{"subject":{"id":"u_resume"}}}`),
		StartedAt:   now,
		FinishedAt:  &finishedAt,
		CreatedAt:   now,
		UpdatedAt:   finishedAt,
	}); err != nil {
		t.Fatalf("create event node execution: %v", err)
	}
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:           "node_resume_match",
		TenantID:     activity.TenantID,
		ExecutionID:  source.ID,
		NodeID:       "match",
		NodeType:     "match",
		Status:       marketing.WorkflowNodeFailed,
		ErrorMessage: "temporary match failure",
		StartedAt:    now.Add(10 * time.Millisecond),
		FinishedAt:   &finishedAt,
		CreatedAt:    now,
		UpdatedAt:    finishedAt,
	}); err != nil {
		t.Fatalf("create failed node execution: %v", err)
	}
	trace, err := service.ResumeWorkflowExecution(ctx, source.ID, marketing.ResumeWorkflowExecutionCommand{Operator: "ops", Reason: "retry after fix"})
	if err != nil {
		t.Fatalf("resume execution: %v", err)
	}
	if trace.Execution.ID == source.ID || trace.Execution.Status != marketing.WorkflowExecutionSuccess {
		t.Fatalf("expected new successful resume execution, got %+v", trace.Execution)
	}
	if !strings.Contains(string(trace.Execution.Input), `"mode":"RESUME"`) || !strings.Contains(string(trace.Execution.Input), `"sourceExecutionId":"`+source.ID+`"`) {
		t.Fatalf("expected resume metadata in input, got %s", string(trace.Execution.Input))
	}
}
