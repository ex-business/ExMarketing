package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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
		ID:           "node_current_inbox",
		TenantID:     "site_001",
		ExecutionID:  current.ID,
		NodeID:       "inbox",
		NodeTitle:    "幂等收件箱",
		Status:       marketing.WorkflowNodeFailed,
		Input:        json.RawMessage(`{"raw_event":{"userId":"user_1"}}`),
		Output:       json.RawMessage(`{"errorCode":"INBOX_WRITE_FAILED","attempt":2}`),
		ErrorMessage: "inbox write failed",
		StartedAt:    now,
		FinishedAt:   &currentNodeFinished,
		CreatedAt:    now,
		UpdatedAt:    now,
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
		Output:      json.RawMessage(`{"event_context":{"accepted":true},"attempt":1}`),
		StartedAt:   now,
		FinishedAt:  &baselineNodeFinished,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create baseline node: %v", err)
	}
	currentInbox := marketing.EventInbox{
		ID:             "inbox_current",
		IdempotencyKey: "idem_current",
		ProcessStatus:  marketing.InboxProcessed,
		Event: marketing.ActivityEvent{
			TenantID:   "site_001",
			EventID:    current.EventID,
			EventType:  "CHECKIN_SUCCESS",
			Subject:    marketing.EventSubject{Type: "USER", ID: "user_1"},
			Biz:        marketing.EventBiz{BizType: "CHECKIN", BizID: "day_1"},
			OccurredAt: now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := repo.CreateInbox(ctx, currentInbox); err != nil {
		t.Fatalf("create current inbox: %v", err)
	}
	baselineInbox := currentInbox
	baselineInbox.ID = "inbox_baseline"
	baselineInbox.IdempotencyKey = "idem_baseline"
	baselineInbox.Event.EventID = baseline.EventID
	baselineInbox.Event.Subject.ID = "user_0"
	if _, _, err := repo.CreateInbox(ctx, baselineInbox); err != nil {
		t.Fatalf("create baseline inbox: %v", err)
	}
	currentParticipation := marketing.Participation{
		ID:             "part_current",
		TenantID:       "site_001",
		ActivityID:     "act_compare",
		EventInboxID:   currentInbox.ID,
		Subject:        currentInbox.Event.Subject,
		Biz:            currentInbox.Event.Biz,
		Status:         marketing.ParticipationRewardable,
		IdempotencyKey: "part_idem_current",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	baselineParticipation := currentParticipation
	baselineParticipation.ID = "part_baseline"
	baselineParticipation.EventInboxID = baselineInbox.ID
	baselineParticipation.Subject = baselineInbox.Event.Subject
	baselineParticipation.IdempotencyKey = "part_idem_baseline"
	if _, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation: currentParticipation,
		GrantOrders: []marketing.GrantOrder{{
			ID:              "grant_current",
			TenantID:        "site_001",
			ActivityID:      "act_compare",
			ParticipationID: currentParticipation.ID,
			Subject:         currentParticipation.Subject,
			RewardType:      marketing.RewardBonus,
			RewardAmount:    "2.0000",
			BillNo:          "cmd_current",
			GrantStatus:     marketing.GrantPending,
			GrantChannel:    marketing.GrantHTTP,
			IdempotencyKey:  "grant_idem_current",
			CreatedAt:       now,
			UpdatedAt:       now,
		}},
		OutboxMessages: []marketing.OutboxMessage{{
			ID:            "outbox_current",
			TenantID:      "site_001",
			MessageType:   "REWARD_GRANT",
			TargetChannel: marketing.GrantHTTP,
			TargetRef:     "cmd_current",
			SendStatus:    marketing.OutboxPending,
			CreatedAt:     now,
			UpdatedAt:     now,
		}},
	}); err != nil {
		t.Fatalf("create current side effects: %v", err)
	}
	if _, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation: baselineParticipation,
		GrantOrders: []marketing.GrantOrder{{
			ID:              "grant_baseline",
			TenantID:        "site_001",
			ActivityID:      "act_compare",
			ParticipationID: baselineParticipation.ID,
			Subject:         baselineParticipation.Subject,
			RewardType:      marketing.RewardBonus,
			RewardAmount:    "1.0000",
			BillNo:          "cmd_baseline",
			GrantStatus:     marketing.GrantSuccess,
			GrantChannel:    marketing.GrantHTTP,
			IdempotencyKey:  "grant_idem_baseline",
			CreatedAt:       now,
			UpdatedAt:       now,
		}},
		OutboxMessages: []marketing.OutboxMessage{{
			ID:            "outbox_baseline",
			TenantID:      "site_001",
			MessageType:   "REWARD_GRANT",
			TargetChannel: marketing.GrantHTTP,
			TargetRef:     "cmd_baseline",
			SendStatus:    marketing.OutboxSent,
			CreatedAt:     now,
			UpdatedAt:     now,
		}},
	}); err != nil {
		t.Fatalf("create baseline side effects: %v", err)
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
	if len(compare.NodeStatusChanges) != 1 || compare.NodeStatusChanges[0].CurrentStatus != marketing.WorkflowNodeFailed || compare.NodeStatusChanges[0].BaselineStatus != marketing.WorkflowNodeSuccess || compare.NodeStatusChanges[0].CurrentAttempt != 2 {
		t.Fatalf("expected node status diff with retry attempt, got %+v", compare.NodeStatusChanges)
	}
	if len(compare.SideEffectChanges) == 0 {
		t.Fatalf("expected side effect changes, got %+v", compare.SideEffectChanges)
	}
	var grantChanged bool
	for _, change := range compare.SideEffectChanges {
		if change.Kind == "grantOrders" {
			grantChanged = true
			if len(change.Changes) == 0 {
				t.Fatalf("expected grant side effect JSON changes: %+v", change)
			}
		}
	}
	if !grantChanged {
		t.Fatalf("expected grant side effect diff, got %+v", compare.SideEffectChanges)
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

func TestCompareWorkflowExecutionsMarksSharedSideEffectScope(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	for _, execution := range []marketing.WorkflowExecution{
		{
			ID:                "exec_same_event_current",
			TenantID:          "site_001",
			ActivityID:        "act_same_event_compare",
			WorkflowVersionID: "version_1",
			WorkflowVersion:   1,
			EventID:           "evt_same_event",
			Status:            marketing.WorkflowExecutionSuccess,
			Input:             json.RawMessage(`{"idempotencyKey":"idem_same_event"}`),
			StartedAt:         now,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "exec_same_event_baseline",
			TenantID:          "site_001",
			ActivityID:        "act_same_event_compare",
			WorkflowVersionID: "version_1",
			WorkflowVersion:   1,
			EventID:           "evt_same_event",
			Status:            marketing.WorkflowExecutionSuccess,
			Input:             json.RawMessage(`{"idempotencyKey":"idem_same_event"}`),
			StartedAt:         now,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	} {
		if _, err := repo.CreateWorkflowExecution(ctx, execution); err != nil {
			t.Fatalf("create execution %s: %v", execution.ID, err)
		}
	}
	compare, err := service.CompareWorkflowExecutions(ctx, "exec_same_event_current", "exec_same_event_baseline")
	if err != nil {
		t.Fatalf("compare executions: %v", err)
	}
	if len(compare.SideEffectChanges) == 0 || compare.SideEffectChanges[0].Kind != "scope" {
		t.Fatalf("expected shared side effect scope warning, got %+v", compare.SideEffectChanges)
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

func TestWorkflowExecutorPreservesCancelAfterLastNode(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	repo := &cancelAfterNodeUpdateRepository{Repository: baseRepo}
	service := NewService(repo)
	now := time.Date(2026, 6, 2, 11, 30, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	execution, err := service.executeWorkflowDefinition(ctx, workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             "inbox_cancel_race",
			IdempotencyKey: "idem_cancel_race",
			Event: marketing.ActivityEvent{
				EventID:      "evt_cancel_race",
				EventType:    "CHECKIN_SUCCESS",
				SourceSystem: "app",
				TenantID:     "site_001",
				Subject:      marketing.EventSubject{Type: "USER", ID: "u_cancel_race"},
				OccurredAt:   now,
			},
		},
		Activity: marketing.Activity{ID: "act_cancel_race", TenantID: "site_001"},
	}, workflowExecutionOptions{
		WorkflowVersionID: "wfver_cancel_race",
		WorkflowVersion:   1,
		Definition: json.RawMessage(`{
			"nodes": [{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口","config":{"eventTypes":["CHECKIN_SUCCESS"],"sourceSystems":["app"]}}],
			"edges": []
		}`),
		Mode:            "PRODUCTION",
		IdempotencyKey:  "idem_cancel_race",
		SkipIdempotency: true,
	})
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}
	if execution.Status != marketing.WorkflowExecutionCanceled || execution.ErrorMessage != "manual stop after last node" {
		t.Fatalf("expected final execution to preserve canceled state, got %+v", execution)
	}
}

func TestWorkflowExecutorMarksBusinessRejectedRuntime(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	execution, err := service.executeWorkflowDefinition(ctx, workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             "inbox_rejected_runtime",
			IdempotencyKey: "idem_rejected_runtime",
			Event: marketing.ActivityEvent{
				EventID:      "evt_rejected_runtime",
				EventType:    "CHECKIN_SUCCESS",
				SourceSystem: "app",
				TenantID:     "site_001",
				Subject:      marketing.EventSubject{Type: "USER", ID: "u_missing"},
				Biz:          marketing.EventBiz{BizType: "CHECKIN", BizID: "daily_u_missing"},
				OccurredAt:   now,
				Attributes:   map[string]any{"checkin": true},
			},
		},
		Activity: marketing.Activity{ID: "act_rejected_runtime", TenantID: "site_001"},
		Participation: marketing.Participation{
			ID:           "part_rejected_runtime",
			TenantID:     "site_001",
			ActivityID:   "act_rejected_runtime",
			Subject:      marketing.EventSubject{Type: "USER", ID: "u_missing"},
			Biz:          marketing.EventBiz{BizType: "CHECKIN", BizID: "daily_u_missing"},
			Status:       marketing.ParticipationRejected,
			RejectReason: "CONDITION_NOT_MET: facts.USER_PROFILE.exists != true",
		},
		RuleResult: marketing.ActivityRuleResult{
			ActivityID: "act_rejected_runtime",
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "CONDITION_NOT_MET",
			Reason:     "facts.USER_PROFILE.exists != true",
		},
	}, workflowExecutionOptions{
		WorkflowVersionID: "wfver_rejected_runtime",
		WorkflowVersion:   1,
		Definition: json.RawMessage(`{
			"nodes": [{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口","config":{"eventTypes":["CHECKIN_SUCCESS"],"sourceSystems":["app"]}}],
			"edges": []
		}`),
		Mode:            "PRODUCTION",
		IdempotencyKey:  "idem_rejected_runtime",
		SkipIdempotency: true,
	})
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}
	if execution.Status != marketing.WorkflowExecutionFailed {
		t.Fatalf("status=%s, want %s", execution.Status, marketing.WorkflowExecutionFailed)
	}
	if !strings.Contains(execution.ErrorMessage, "facts.USER_PROFILE.exists != true") {
		t.Fatalf("expected business rejection reason in error message, got %q", execution.ErrorMessage)
	}
}

func TestWorkflowExecutorWaitsForAllFactBranchesBeforeFactsCompose(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	execution, err := service.executeWorkflowDefinition(ctx, workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             "inbox_facts_join",
			IdempotencyKey: "idem_facts_join",
			Event: marketing.ActivityEvent{
				EventID:      "evt_facts_join",
				EventType:    "CHECKIN_SUCCESS",
				SourceSystem: "app",
				TenantID:     "site_001",
				Subject:      marketing.EventSubject{Type: "USER", ID: "u_facts_join"},
				OccurredAt:   now,
				Attributes: map[string]any{
					"USER_PROFILE": map[string]any{"vipLevel": "GOLD"},
					"KYC_PROFILE":  map[string]any{"status": "VERIFIED"},
				},
			},
		},
		Activity: marketing.Activity{ID: "act_facts_join", TenantID: "site_001"},
	}, workflowExecutionOptions{
		WorkflowVersionID: "wfver_facts_join",
		WorkflowVersion:   1,
		Definition: json.RawMessage(`{
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口","config":{"eventTypes":["CHECKIN_SUCCESS"],"sourceSystems":["app"]}},
				{"id":"fact-user","type":"context-query","title":"用户事实","config":{"factKeys":["USER_PROFILE"],"sourcePath":"$.attributes.USER_PROFILE","failureAction":"BLOCK"}},
				{"id":"fact-kyc","type":"context-query","title":"KYC事实","config":{"factKeys":["KYC_PROFILE"],"sourcePath":"$.attributes.KYC_PROFILE","failureAction":"BLOCK"}},
				{"id":"facts-compose","type":"facts-compose","title":"事实合并","config":{"mergeUpstreamFacts":true,"conflictPolicy":"BLOCK"}}
			],
			"edges": [
				{"id":"event-user","source":"event-in","sourceHandle":"USER_PROFILE","target":"fact-user"},
				{"id":"event-kyc","source":"event-in","sourceHandle":"KYC_PROFILE","target":"fact-kyc"},
				{"id":"user-compose","source":"fact-user","target":"facts-compose"},
				{"id":"kyc-compose","source":"fact-kyc","target":"facts-compose"}
			]
		}`),
		Mode:            "PRODUCTION",
		IdempotencyKey:  "idem_facts_join",
		SkipIdempotency: true,
	})
	if err != nil {
		t.Fatalf("execute workflow: %v", err)
	}
	if execution.Status != marketing.WorkflowExecutionSuccess {
		t.Fatalf("expected success, got %+v", execution)
	}
	trace, err := service.ExportWorkflowExecutionTrace(ctx, execution.ID)
	if err != nil {
		t.Fatalf("export trace: %v", err)
	}
	order := map[string]int{}
	var composed map[string]any
	for index, item := range trace.NodeExecutions {
		order[item.NodeID] = index
		if item.NodeID == "facts-compose" {
			if err := json.Unmarshal(item.Output, &composed); err != nil {
				t.Fatalf("unmarshal facts-compose output: %v", err)
			}
		}
	}
	if order["facts-compose"] <= order["fact-user"] || order["facts-compose"] <= order["fact-kyc"] {
		t.Fatalf("facts-compose executed before all fact branches: %+v", order)
	}
	facts := composed["facts"].(map[string]any)
	if _, ok := facts["USER_PROFILE"]; !ok {
		t.Fatalf("expected USER_PROFILE in composed facts, got %+v", facts)
	}
	if _, ok := facts["KYC_PROFILE"]; !ok {
		t.Fatalf("expected KYC_PROFILE in composed facts, got %+v", facts)
	}
}

type cancelAfterNodeUpdateRepository struct {
	marketing.Repository
	executionID string
	canceled    atomic.Bool
}

func (r *cancelAfterNodeUpdateRepository) CreateWorkflowExecution(ctx context.Context, execution marketing.WorkflowExecution) (marketing.WorkflowExecution, error) {
	created, err := r.Repository.CreateWorkflowExecution(ctx, execution)
	if err == nil {
		r.executionID = created.ID
	}
	return created, err
}

func (r *cancelAfterNodeUpdateRepository) UpdateWorkflowNodeExecution(ctx context.Context, execution marketing.WorkflowNodeExecution) error {
	if err := r.Repository.UpdateWorkflowNodeExecution(ctx, execution); err != nil {
		return err
	}
	if r.executionID == "" || r.canceled.Swap(true) {
		return nil
	}
	executions, err := r.Repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: r.executionID, Limit: 1})
	if err != nil || len(executions) == 0 {
		return err
	}
	now := execution.UpdatedAt
	current := executions[0]
	current.Status = marketing.WorkflowExecutionCanceled
	current.ErrorMessage = "manual stop after last node"
	current.FinishedAt = &now
	current.UpdatedAt = now
	return r.Repository.UpdateWorkflowExecution(ctx, current)
}

func TestCoreWorkflowNodeHandlersProduceStableOutputs(t *testing.T) {
	ctx := context.Background()
	runtime := testWorkflowHandlerRuntime(true)
	service := NewService(marketing.NewMemoryRepository())
	decimalCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{ResolvedInput: map[string]any{"amount_expr": "12.345"}})
	factsComposeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"fact-user": {"facts": map[string]any{"USER_PROFILE": map[string]any{"vipLevel": "GOLD"}}},
		},
	})
	cases := []struct {
		name      string
		node      marketing.WorkflowNode
		handler   workflowNodeHandler
		outputKey string
		ctx       context.Context
	}{
		{name: "event in", node: marketing.WorkflowNode{ID: "node_event", Type: "event-in"}, handler: entryWorkflowNodeHandler, outputKey: "raw_event"},
		{name: "mq event", node: marketing.WorkflowNode{ID: "node_mq_event", Type: "mq-event", Config: json.RawMessage(`{"broker":"KAFKA","topic":"marketing.events","consumerGroup":"exmarketing","ackPolicy":"ACK_AFTER_INBOX_COMMIT","retryPolicy":"RETRY_THEN_DLQ"}`)}, handler: mqEventWorkflowNodeHandler, outputKey: "consumerPlan"},
		{name: "schedule event", node: marketing.WorkflowNode{ID: "node_schedule_event", Type: "schedule-event", Config: json.RawMessage(`{"scheduleType":"CRON","cron":"0 0 * * *","timezone":"UTC","eventType":"SCHEDULE_TICK"}`)}, handler: scheduleEventWorkflowNodeHandler, outputKey: "schedulePlan"},
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
		{name: "facts compose", node: marketing.WorkflowNode{ID: "node_facts_compose", Type: "facts-compose"}, handler: factsComposeWorkflowNodeHandler, outputKey: "facts", ctx: factsComposeCtx},
		{name: "switch", node: marketing.WorkflowNode{ID: "node_switch", Type: "switch"}, handler: decisionWorkflowNodeHandler, outputKey: "route"},
		{name: "code", node: marketing.WorkflowNode{ID: "node_code", Type: "code", Config: json.RawMessage(`{"language":"EXPRESSION","source":"$.subject.id","outputPath":"subject_id"}`)}, handler: codeWorkflowNodeHandler, outputKey: "code_result"},
		{name: "combine", node: marketing.WorkflowNode{ID: "node_combine", Type: "combine", Config: json.RawMessage(`{"language":"LUA","source":"emit({combined = {eventId = ctx.event.eventId}})"}`)}, handler: transformScriptWorkflowNodeHandler("combine", "combined_result"), outputKey: "combined_result"},
		{name: "split", node: marketing.WorkflowNode{ID: "node_split", Type: "split", Config: json.RawMessage(`{"language":"LUA","source":"emit({items = {'a', 'b'}})"}`)}, handler: transformScriptWorkflowNodeHandler("split", "split_items"), outputKey: "split_items"},
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

func TestQualifyNodeEligibilityRuleConfigExecutesConditions(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"facts": map[string]any{
				"USER_PROFILE": map[string]any{
					"exists":   true,
					"status":   "ACTIVE",
					"vipLevel": 2,
				},
			},
		},
	})
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rules = nil
	runtime.Inbox.Event.EventType = "DEPOSIT_SUCCESS"
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "user_001"}
	node := marketing.WorkflowNode{
		ID:   "qualify",
		Type: "qualify",
		Config: json.RawMessage(`{
			"ruleCodes":["ELIGIBILITY","VIP_LEVEL"],
			"qualifyRules":[
				{
					"ruleCode":"ELIGIBILITY",
					"enabled":true,
					"logic":"AND",
					"conditions":[
						{"field":"eventType","op":"=","value":"DEPOSIT_SUCCESS"},
						{"field":"subject.id","op":"!=","value":""},
						{"field":"facts.USER_PROFILE.exists","op":"=","value":true},
						{"field":"facts.USER_PROFILE.status","op":"=","value":"ACTIVE"}
					]
				},
				{
					"ruleCode":"VIP_LEVEL",
					"enabled":true,
					"logic":"AND",
					"conditions":[
						{"field":"facts.USER_PROFILE.vipLevel","op":">=","value":1}
					]
				}
			]
		}`),
	}

	result := decisionWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["decision"] != "PASS" || result.Output["PASS"] != true {
		t.Fatalf("expected eligibility rule to pass, got %+v", result)
	}
	ruleResult, _ := result.Output["ruleResult"].(map[string]any)
	stageResults, _ := ruleResult["stageResults"].([]any)
	qualifyStage, _ := stageResults[1].(map[string]any)
	ruleResults, _ := qualifyStage["ruleResults"].([]any)
	firstRule, _ := ruleResults[0].(map[string]any)
	secondRule, _ := ruleResults[1].(map[string]any)
	if len(ruleResults) != 2 || firstRule["ruleCode"] != "ELIGIBILITY" || secondRule["ruleCode"] != "VIP_LEVEL" {
		t.Fatalf("expected qualify rules to keep configured order, got %+v", ruleResults)
	}

	blockedCtx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"facts": map[string]any{
				"USER_PROFILE": map[string]any{
					"exists":   true,
					"status":   "SUSPENDED",
					"vipLevel": 2,
				},
			},
		},
	})
	blocked := decisionWorkflowNodeHandler(blockedCtx, node, runtime)
	if blocked.Status != marketing.WorkflowNodeSuccess || blocked.Output["decision"] != "BLOCK" || blocked.Output["BLOCK"] != true {
		t.Fatalf("expected eligibility rule to block, got %+v", blocked)
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

func TestWorkflowExecutionPlanRejectsDuplicateEdgeIDs(t *testing.T) {
	_, err := workflowExecutionPlan(marketing.WorkflowDefinition{
		Nodes: []marketing.WorkflowNode{
			{ID: "event-in", Type: "event-in", Kind: "entry"},
			{ID: "normalize", Type: "normalize"},
			{ID: "reward", Type: "reward"},
		},
		Edges: []marketing.WorkflowEdge{
			{ID: "event-normalize", Source: "event-in", Target: "normalize"},
			{ID: " event-normalize ", Source: "normalize", Target: "reward"},
		},
	})
	if err == nil {
		t.Fatal("expected workflow plan compile error")
	}
	var compileErr workflowPlanCompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("expected workflowPlanCompileError, got %T %v", err, err)
	}
	for _, issue := range compileErr.Issues {
		if issue.Code == "WORKFLOW_DAG_EDGE_ID_DUPLICATED" && issue.EdgeID == "event-normalize" {
			return
		}
	}
	t.Fatalf("expected duplicate edge issue, got %+v", compileErr.Issues)
}

func TestWorkflowExecutionPlanRejectsUnreachableNodes(t *testing.T) {
	_, err := workflowExecutionPlan(marketing.WorkflowDefinition{
		Nodes: []marketing.WorkflowNode{
			{ID: "event-in", Type: "event-in", Kind: "entry"},
			{ID: "normalize", Type: "normalize"},
			{ID: "orphan", Type: "audit"},
		},
		Edges: []marketing.WorkflowEdge{
			{ID: "event-normalize", Source: "event-in", Target: "normalize"},
		},
	})
	if err == nil {
		t.Fatal("expected workflow plan compile error")
	}
	var compileErr workflowPlanCompileError
	if !errors.As(err, &compileErr) {
		t.Fatalf("expected workflowPlanCompileError, got %T %v", err, err)
	}
	for _, issue := range compileErr.Issues {
		if issue.Code == "WORKFLOW_DAG_NODE_UNREACHABLE" && issue.NodeID == "orphan" {
			return
		}
	}
	t.Fatalf("expected unreachable node issue, got %+v", compileErr.Issues)
}

func TestWorkflowExecutionPlanNormalizesEdgeEndpoints(t *testing.T) {
	plan, err := workflowExecutionPlan(marketing.WorkflowDefinition{
		Nodes: []marketing.WorkflowNode{
			{ID: "event-in", Type: "event-in", Kind: "entry"},
			{ID: "normalize", Type: "normalize"},
		},
		Edges: []marketing.WorkflowEdge{
			{ID: " event-normalize ", Source: " event-in ", Target: " normalize "},
		},
	})
	if err != nil {
		t.Fatalf("expected plan to compile with trimmed endpoints: %v", err)
	}
	edges := plan.EdgesBySource["event-in"]
	if len(edges) != 1 || edges[0].ID != "event-normalize" || edges[0].Source != "event-in" || edges[0].Target != "normalize" {
		t.Fatalf("expected normalized edge in plan, got %+v", edges)
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

func TestFactsComposeWorkflowNodeHandlerMergesMultipleFactOutputs(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"fact-kyc": {
				"facts": map[string]any{
					"KYC_PROFILE": map[string]any{"status": "VERIFIED"},
				},
			},
			"fact-user": {
				"facts": map[string]any{
					"USER_PROFILE": map[string]any{"vipLevel": "GOLD"},
				},
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "facts-compose",
		Type: "facts-compose",
		Config: json.RawMessage(`{
			"mergeUpstreamFacts": true,
			"conflictPolicy": "BLOCK"
		}`),
	}

	result := factsComposeWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	facts, ok := result.Output["facts"].(map[string]any)
	if !ok {
		t.Fatalf("expected facts map, got %+v", result.Output["facts"])
	}
	if _, ok := facts["USER_PROFILE"]; !ok {
		t.Fatalf("expected USER_PROFILE in composed facts, got %+v", facts)
	}
	if _, ok := facts["KYC_PROFILE"]; !ok {
		t.Fatalf("expected KYC_PROFILE in composed facts, got %+v", facts)
	}
	if result.Output["factCount"] != 2 {
		t.Fatalf("expected two composed facts, got %+v", result.Output)
	}
}

func TestFactsComposeWorkflowNodeHandlerSupportsExplicitMappings(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"risk-query": {
				"provider_response": map[string]any{
					"profile": map[string]any{"score": 88, "tier": "MEDIUM"},
				},
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "facts-compose",
		Type: "facts-compose",
		Config: json.RawMessage(`{
			"mergeUpstreamFacts": false,
			"factMappings": [
				{"factCode":"RISK_PROFILE","sourceNodeId":"risk-query","outputKey":"provider_response","path":"$.profile","required":true}
			]
		}`),
	}

	result := factsComposeWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	facts := result.Output["facts"].(map[string]any)
	risk := facts["RISK_PROFILE"].(map[string]any)
	if risk["score"] != float64(88) && risk["score"] != 88 {
		t.Fatalf("expected mapped risk profile, got %+v", facts)
	}
}

func TestFactsNormalizeWorkflowNodeHandlerBuildsStandardContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"facts": map[string]any{
				"USER_PROFILE":   map[string]any{"exists": true, "vipLevel": 3},
				"KYC_PROFILE":    map[string]any{"status": "VERIFIED"},
				"RISK_PROFILE":   map[string]any{"riskLevel": "LOW", "score": 12},
				"CHECKIN_STREAK": map[string]any{"continuousDays": 7},
				"UNRELATED_FACT": map[string]any{"kept": true},
			},
		},
	})
	node := marketing.WorkflowNode{ID: "facts-normalize", Type: "facts-normalize"}
	result := factsNormalizeWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	standard := result.Output["standard_context"].(map[string]any)
	user := standard["user"].(map[string]any)
	if user["level"] != float64(3) || user["exists"] != true {
		t.Fatalf("expected standardized user, got %+v", user)
	}
	kyc := standard["kyc"].(map[string]any)
	if kyc["status"] != "VERIFIED" {
		t.Fatalf("expected standardized kyc, got %+v", kyc)
	}
	risk := standard["risk"].(map[string]any)
	if risk["level"] != "LOW" || risk["score"] != float64(12) {
		t.Fatalf("expected standardized risk, got %+v", risk)
	}
	checkin := standard["checkin"].(map[string]any)
	if checkin["streak"] != float64(7) {
		t.Fatalf("expected standardized checkin streak, got %+v", checkin)
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

func TestCandidateRewardWorkflowNodeHandlerUsesProviderChainRewardConfig(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rewards = []marketing.Reward{{
		ID:            "reward_checkin",
		Code:          "DAILY_CHECKIN_BONUS",
		Type:          marketing.RewardBonus,
		CalculateType: marketing.CalculateFixed,
		Config:        json.RawMessage(`{"amount":"1","currency":"USD"}`),
		GrantMode:     marketing.GrantAuto,
		GrantChannel:  marketing.GrantHTTP,
		Enabled:       true,
	}}
	node := marketing.WorkflowNode{
		ID:   "candidate-reward",
		Type: "candidate-reward",
		Config: json.RawMessage(`{
			"providerChain":[
				{"source":"RESOURCE","resourceRef":"default_activity_reward_config","namespace":"reward","operation":"READ","keyTemplate":"${activityId}","valuePath":"$.reward_items","outputAs":"rewardConfig","onMiss":"BLOCK"}
			],
			"notQualifiedAction":"SKIP",
			"emptyAction":"BLOCK"
		}`),
	}
	result := NewService(marketing.NewMemoryRepository()).candidateRewardWorkflowNodeHandler(context.Background(), node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected candidate reward success, got %+v", result)
	}
	items := result.Output["candidate_rewards"].([]any)
	item := items[0].(map[string]any)
	if item["rewardCode"] != "DAILY_CHECKIN_BONUS" || item["amount"] != "1.0000" {
		t.Fatalf("expected provider-chain candidate reward, got %+v", item)
	}
	resourceResult := result.Output["resource_result"].(map[string]any)
	if resourceResult["resourceRef"] != "default_activity_reward_config" || resourceResult["adapterMode"] != "ACTIVITY_REWARD_CONFIG" {
		t.Fatalf("expected activity reward config resource result, got %+v", resourceResult)
	}
}

func TestCandidateRewardWorkflowNodeHandlerUsesCheckinRewardRedisRefDefault(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rewards = nil
	node := marketing.WorkflowNode{
		ID:   "candidate-reward",
		Type: "candidate-reward",
		Config: json.RawMessage(`{
			"providerChain":[
				{"source":"RESOURCE","resourceRef":"checkin_reward_config_ref","namespace":"reward","operation":"READ","keyTemplate":"${activityId}","valuePath":"$.reward_items","outputAs":"rewardConfig","onMiss":"BLOCK"}
			],
			"notQualifiedAction":"SKIP",
			"emptyAction":"BLOCK"
		}`),
	}
	result := NewService(marketing.NewMemoryRepository()).candidateRewardWorkflowNodeHandler(context.Background(), node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected candidate reward success, got %+v", result)
	}
	items := result.Output["candidate_rewards"].([]any)
	item := items[0].(map[string]any)
	if item["rewardCode"] != "DAILY_CHECKIN_1_USDT" || item["amount"] != "1.0000" || item["currency"] != "USDT" {
		t.Fatalf("expected checkin redis default reward, got %+v", item)
	}
	resourceResult := result.Output["resource_result"].(map[string]any)
	if resourceResult["resourceRef"] != "checkin_reward_config_ref" || resourceResult["adapterMode"] != "CHECKIN_REWARD_CONFIG_DEFAULT" {
		t.Fatalf("expected checkin reward config default result, got %+v", resourceResult)
	}
}

func TestCandidateRewardWorkflowNodeHandlerAppliesCandidatePolicy(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Rewards = []marketing.Reward{
		{ID: "reward_small", Code: "SMALL", Type: marketing.RewardCash, CalculateType: marketing.CalculateFixed, Config: json.RawMessage(`{"amount":"1","currency":"USDT"}`), GrantMode: marketing.GrantAuto, GrantChannel: marketing.GrantHTTP, Enabled: true},
		{ID: "reward_large", Code: "LARGE", Type: marketing.RewardCash, CalculateType: marketing.CalculateFixed, Config: json.RawMessage(`{"amount":"3","currency":"USDT"}`), GrantMode: marketing.GrantAuto, GrantChannel: marketing.GrantHTTP, Enabled: true},
	}
	cases := []struct {
		policy string
		code   string
	}{
		{policy: "FIRST_MATCHED", code: "SMALL"},
		{policy: "HIGHEST_VALUE", code: "LARGE"},
	}
	for _, tt := range cases {
		t.Run(tt.policy, func(t *testing.T) {
			node := marketing.WorkflowNode{
				ID:     "candidate-reward",
				Type:   "candidate-reward",
				Config: json.RawMessage(fmt.Sprintf(`{"source":"ACTIVITY_REWARD_CONFIG","candidatePolicy":%q,"notQualifiedAction":"SKIP","emptyAction":"BLOCK"}`, tt.policy)),
			}
			result := NewService(marketing.NewMemoryRepository()).candidateRewardWorkflowNodeHandler(context.Background(), node, runtime)
			if result.Status != marketing.WorkflowNodeSuccess {
				t.Fatalf("expected candidate reward success, got %+v", result)
			}
			items := result.Output["candidate_rewards"].([]any)
			if len(items) != 1 {
				t.Fatalf("expected one selected candidate, got %+v", items)
			}
			item := items[0].(map[string]any)
			if item["rewardCode"] != tt.code || result.Output["candidatePolicy"] != tt.policy {
				t.Fatalf("expected policy %s to select %s, got item=%+v output=%+v", tt.policy, tt.code, item, result.Output)
			}
		})
	}
}

func TestCandidateRewardEvaluationSupportsPrizeModuleRewardTypes(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.IdempotencyKey = "idem_prize_module"
	items := []any{
		map[string]any{"rewardCode": "RANDOM_USDT", "rewardType": "CASH", "calculateType": "RANDOM", "minAmount": "1", "maxAmount": "3", "step": "1", "currency": "USDT"},
		map[string]any{"rewardCode": "POINTS", "rewardType": "POINT", "calculateType": "FIXED", "points": "100", "unit": "LOYALTY_POINT"},
		map[string]any{"rewardCode": "COUPON", "rewardType": "COUPON", "calculateType": "FIXED", "quantity": "2", "couponTemplateId": "tpl_coupon", "faceValue": "5", "currency": "USDT"},
		map[string]any{"rewardCode": "DRAW", "rewardType": "DRAW_CHANCE", "calculateType": "FIXED", "chanceCount": "1", "drawPoolCode": "daily_draw"},
	}
	evaluated, err := workflowEvaluateCandidateRewards(context.Background(), runtime, items)
	if err != nil {
		t.Fatalf("expected prize module rewards to evaluate, got %v", err)
	}
	randomCash := evaluated[0].(map[string]any)
	if randomCash["amount"] == "" || randomCash["currency"] != "USDT" {
		t.Fatalf("expected random cash amount, got %+v", randomCash)
	}
	points := evaluated[1].(map[string]any)
	if workflowOutputString(points["points"]) != "100" {
		t.Fatalf("expected point payload copied to candidate, got %+v", points)
	}
	coupon := evaluated[2].(map[string]any)
	if workflowOutputString(coupon["quantity"]) != "2" {
		t.Fatalf("expected coupon quantity copied to candidate, got %+v", coupon)
	}
	draw := evaluated[3].(map[string]any)
	if workflowOutputString(draw["chanceCount"]) != "1" {
		t.Fatalf("expected draw chance copied to candidate, got %+v", draw)
	}
}

func TestCandidateRewardFiltersDisabledAndSupportsRandomOnePolicy(t *testing.T) {
	items := []any{
		map[string]any{"rewardCode": "A", "amount": "1"},
		map[string]any{"rewardCode": "B", "amount": "2", "enabled": false},
		map[string]any{"rewardCode": "C", "amount": "3", "disabled": true},
		map[string]any{"rewardCode": "D", "amount": "4"},
	}
	filtered := workflowFilterEnabledCandidateRewards(items)
	if len(filtered) != 2 {
		t.Fatalf("expected disabled rewards filtered, got %+v", filtered)
	}
	selected := workflowSelectCandidateRewards(filtered, "RANDOM_ONE", workflowRuntime{Activity: marketing.Activity{ID: "activity_random"}, Inbox: marketing.EventInbox{IdempotencyKey: "idem_random"}})
	if len(selected) != 1 {
		t.Fatalf("expected RANDOM_ONE to select one reward, got %+v", selected)
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

func TestContextFactQueryWorkflowNodeHandlerReadsEventAttributes(t *testing.T) {
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Attributes["USER_PROFILE"] = map[string]any{"vipLevel": 1, "channel": "APP"}
	node := marketing.WorkflowNode{
		ID:   "context-query",
		Type: "context-query",
		Config: json.RawMessage(`{
			"factKeys": ["USER_PROFILE"],
			"sourcePath": "$.attributes.USER_PROFILE",
			"failureAction": "BLOCK"
		}`),
	}

	result := contextFactQueryWorkflowNodeHandler(context.Background(), node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	facts := result.Output["facts"].(map[string]any)
	profile := facts["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"].(float64) != 1 || profile["channel"] != "APP" {
		t.Fatalf("expected mapped profile facts, got %+v", result.Output)
	}
}

func TestLuaFactQueryWorkflowNodeHandlerEmitsFacts(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:   "lua-query",
		Type: "lua-query",
		Config: json.RawMessage(`{
			"source": "emit({facts = {USER_PROFILE = {vipLevel = 1, channel = 'APP'}}})",
			"failureAction": "BLOCK"
		}`),
	}

	result := luaFactQueryWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected success, got %+v", result)
	}
	facts := result.Output["facts"].(map[string]any)
	profile := facts["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"].(float64) != 1 || profile["channel"] != "APP" {
		t.Fatalf("expected lua profile facts, got %+v", result.Output)
	}
}

func TestFactAdapterRequiredWorkflowNodeHandlerIsExplicitlySkipped(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:     "redis-query",
		Type:   "redis-query",
		Config: json.RawMessage(`{"factKeys":["USER_PROFILE"],"failureAction":"SKIP"}`),
	}

	result := factAdapterRequiredWorkflowNodeHandler("REDIS", "RedisFactCache", false)(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSkipped || result.Output["errorCode"] != "FACT_PROVIDER_ADAPTER_REQUIRED" {
		t.Fatalf("expected explicit adapter skipped output, got %+v", result)
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

func TestServiceHTTPFactQueryWorkflowNodeHandlerUsesInlineEndpointConfig(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	service.factClient = fact.NewClientWithHTTPClient(&http.Client{Transport: engineRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.com/profile" {
			t.Fatalf("unexpected provider endpoint: %s", req.URL.String())
		}
		if req.Method != http.MethodPost {
			t.Fatalf("unexpected provider method: %s", req.Method)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		if !strings.Contains(string(body), "USER_PROFILE") || !strings.Contains(string(body), "user_001") {
			t.Fatalf("provider request missing fact code or subject: %s", string(body))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"profile":{"vipLevel":"VIP1","country":"PH"}}}`)),
			Header:     http.Header{},
		}, nil
	})})
	node := marketing.WorkflowNode{
		ID:   "http-query",
		Type: "http-query",
		Config: json.RawMessage(`{
			"factKeys": ["USER_PROFILE"],
			"subjectPath": "subject.id",
			"endpoint": "https://example.com/profile",
			"method": "POST",
			"mapping": {
				"vipLevel": {"source": "$.data.profile.vipLevel", "type": "string", "required": true},
				"country": {"source": "$.data.profile.country", "type": "string", "required": true}
			},
			"failureAction": "BLOCK",
			"timeoutSeconds": 1
		}`),
	}

	result := service.httpFactQueryWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected inline provider query success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != "VIP1" || profile["country"] != "PH" {
		t.Fatalf("expected inline mapped profile facts, got %+v", result.Output)
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

func TestBudgetReserveWorkflowNodeHandlerUsesReserveRefConfig(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	runtime := workflowRuntime{
		Activity:      marketing.Activity{ID: "activity_handler", TenantID: "site_001"},
		Participation: marketing.Participation{ID: "participation_handler"},
		Qualified:     true,
		RuleResult:    marketing.ActivityRuleResult{Decision: marketing.RuleDecisionPassed},
	}
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"candidate-reward": {"candidate_rewards": []any{map[string]any{"rewardCode": "COUPON_A", "quantity": "2"}}},
		},
	})
	result := service.budgetReserveWorkflowNodeHandler(ctx, marketing.WorkflowNode{
		ID:     "budget-reserve",
		Type:   "budget-reserve",
		Config: json.RawMessage(`{"reserveProviderRef":"checkin_prize_stock_ref"}`),
	}, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected budget reserve success, got %+v", result)
	}
	hold := result.Output["quota_hold"].(map[string]any)
	if hold["quotaCode"] != "CHECKIN_PRIZE_STOCK" || hold["quotaType"] != string(marketing.QuotaStock) {
		t.Fatalf("expected stock quota from reserve ref, got %+v", hold)
	}
	if hold["scopeKey"] != "activity_handler:COUPON_A" || hold["plannedAmount"] != "2" {
		t.Fatalf("unexpected stock reservation hold: %+v", hold)
	}
	if result.Output["resourceRef"] != defaultCheckinPrizeStock {
		t.Fatalf("expected reserve resource output, got %+v", result.Output)
	}
	if result.Output["reserveProviderRef"] != defaultCheckinPrizeStock {
		t.Fatalf("expected reserve provider output, got %+v", result.Output)
	}
}

func TestBudgetReserveWorkflowNodeHandlerRejectsLimitQuotaTypes(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	result := service.budgetReserveWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{
		ID:     "budget-reserve",
		Type:   "budget-reserve",
		Config: json.RawMessage(`{"quotaType":"DAILY_LIMIT","scope":"USER","scopeKeyTemplate":"${subject.id}:${activityId}"}`),
	}, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "BUDGET_RESERVE_QUOTA_TYPE_UNSUPPORTED" {
		t.Fatalf("expected daily limit to be rejected by budget-reserve, got %+v", result)
	}
}

func TestBudgetReserveWorkflowNodeHandlerCallsExternalHTTPReserveRef(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := workflowRuntime{
		Activity:      marketing.Activity{ID: "activity_handler", TenantID: "site_001"},
		Inbox:         marketing.EventInbox{IdempotencyKey: "idem_external", Event: marketing.ActivityEvent{EventID: "event_external", Subject: marketing.EventSubject{Type: "USER", ID: "user_001"}}},
		Participation: marketing.Participation{ID: "participation_handler"},
		Qualified:     true,
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode provider body: %v", err)
		}
		value := body["value"].(map[string]any)
		if value["quotaType"] != "STOCK" || value["scopeKey"] != "activity_handler:COUPON_A" || value["amount"] != "2" {
			t.Fatalf("unexpected reserve payload: %+v", value)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":true,"reservationId":"resv_http_1","status":"RESERVED","quotaType":"STOCK","scope":"ACTIVITY","scopeKey":"activity_handler:COUPON_A","amount":"2"}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(context.Background(), marketing.Resource{
		TenantID: "site_001",
		Code:     "external_stock_http_ref",
		Name:     "External Stock Reserve HTTP Ref",
		Type:     marketing.ResourceHTTPProvider,
		Driver:   marketing.ResourceDriverHTTP,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config: json.RawMessage(fmt.Sprintf(`{
			"contract":"QUOTA_RESERVE_V1",
			"reserveMode":"EXTERNAL_PROVIDER",
			"endpoint":%q,
			"httpMethod":"POST",
			"quotaCode":"EXT_STOCK",
			"quotaType":"STOCK",
			"scope":"ACTIVITY",
			"scopeKeyTemplate":"${activityId}:${candidate_rewards[0].rewardCode}",
			"amountField":"candidate_rewards[0].quantity"
		}`, provider.URL)),
		Capabilities: []marketing.ResourceOperation{marketing.ResourceCall, marketing.ResourceTestConnection},
	}); err != nil {
		t.Fatalf("save external http reserve resource: %v", err)
	}
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		NodeOutputs: map[string]map[string]any{
			"candidate-reward": {"candidate_rewards": []any{map[string]any{"rewardCode": "COUPON_A", "quantity": "2"}}},
		},
	})
	result := service.budgetReserveWorkflowNodeHandler(ctx, marketing.WorkflowNode{
		ID:     "budget-reserve",
		Type:   "budget-reserve",
		Config: json.RawMessage(`{"reserveProviderRef":"external_stock_http_ref","failAction":"BLOCK","releaseOnFailure":true}`),
	}, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected external http reserve success, got %+v", result)
	}
	hold := result.Output["quota_hold"].(map[string]any)
	if hold["reservationId"] != "resv_http_1" || hold["external"] != true || hold["commitAuthority"] != "ExternalReserveProvider" {
		t.Fatalf("unexpected external quota hold: %+v", hold)
	}
	if result.Output["reserveMode"] != "EXTERNAL_PROVIDER" || result.Output["targetResourceRef"] != "external_stock_http_ref" || result.Output["reserveProviderRef"] != "external_stock_http_ref" {
		t.Fatalf("unexpected external reserve metadata: %+v", result.Output)
	}
}

func TestBudgetReserveWorkflowNodeHandlerRequiresGRPCReserveAdapter(t *testing.T) {
	service := NewService(marketing.NewMemoryRepository())
	if _, err := service.SaveResource(context.Background(), marketing.Resource{
		TenantID:     "site_001",
		Code:         "external_budget_grpc_ref",
		Name:         "External Budget Reserve gRPC Ref",
		Type:         marketing.ResourceGRPCProvider,
		Driver:       marketing.ResourceDriverInternal,
		Scope:        marketing.ResourceScopeWorkspace,
		Status:       marketing.ResourceActive,
		Config:       json.RawMessage(`{"contract":"QUOTA_RESERVE_V1","reserveMode":"EXTERNAL_PROVIDER","grpcTarget":"dns:///budget-reserve:9090","grpcMethod":"Reserve","quotaCode":"EXT_BUDGET","quotaType":"BUDGET","scope":"ACTIVITY","scopeKeyTemplate":"${activityId}","amountField":"candidate_rewards[0].amount"}`),
		Capabilities: []marketing.ResourceOperation{marketing.ResourceCall, marketing.ResourceTestConnection},
	}); err != nil {
		t.Fatalf("save external grpc reserve resource: %v", err)
	}
	result := service.budgetReserveWorkflowNodeHandler(context.Background(), marketing.WorkflowNode{
		ID:     "budget-reserve",
		Type:   "budget-reserve",
		Config: json.RawMessage(`{"reserveProviderRef":"external_budget_grpc_ref"}`),
	}, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "BUDGET_RESERVE_GRPC_ADAPTER_REQUIRED" {
		t.Fatalf("expected gRPC adapter boundary failure, got %+v", result)
	}
}

func TestCodeWorkflowNodeHandlerExecutesLuaWithWorkflowContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"amount": "200.00"},
		NodeOutputs: map[string]map[string]any{
			"qualify": {
				"decision": "PASS",
				"id":       "output-id",
				"score":    88,
				"stage_result": map[string]any{
					"decision": "PASS",
				},
			},
		},
		NodeDefinitions: map[string]marketing.WorkflowNode{
			"qualify": {
				ID:     "qualify",
				Type:   "qualify",
				Title:  "资格校验",
				Config: json.RawMessage(`{"evaluationMode":"ALL_PASS"}`),
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:   "lua_code",
		Type: "code",
		Config: json.RawMessage(`{
			"language": "LUA",
			"outputPath": "lua_result",
			"source": "local user_id = ctx.event.subject.id\nlocal qualify = node('qualify')\nlocal decision = ctx.nodes.qualify.decision\nlocal nested_decision = node_output_path('qualify', '$.stage_result.decision')\nlocal mode = node_config('qualify', 'evaluationMode')\nemit({userId = user_id, nodeTitle = qualify.title, outputId = ctx.nodes.qualify.id, definitionId = qualify.id, decision = decision, nestedDecision = nested_decision, mode = mode, amount = ctx.node.input.amount, passed = decision == 'PASS'})"
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
	if luaResult["userId"] != "user_001" || luaResult["nodeTitle"] != "资格校验" || luaResult["outputId"] != "output-id" || luaResult["definitionId"] != "qualify" || luaResult["decision"] != "PASS" || luaResult["nestedDecision"] != "PASS" || luaResult["mode"] != "ALL_PASS" || luaResult["amount"] != "200.00" || luaResult["passed"] != true {
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

func TestCombineAndSplitWorkflowNodeHandlersSupportLua(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"items": []any{
				map[string]any{"id": "a", "amount": "10.00"},
				map[string]any{"id": "b", "amount": "20.00"},
			},
			"context": map[string]any{"source": "unit-test"},
		},
	})
	runtime := testWorkflowHandlerRuntime(true)
	combineNode := marketing.WorkflowNode{
		ID:   "combine",
		Type: "combine",
		Config: json.RawMessage(`{
			"language": "LUA",
			"outputPath": "combined_result",
			"source": "local items = input('items')\nemit({combined = {count = #items, firstId = items[1].id, eventId = ctx.event.eventId}})"
		}`),
	}

	combined := transformScriptWorkflowNodeHandler("combine", "combined_result")(ctx, combineNode, runtime)
	if combined.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected combine success, got %+v", combined)
	}
	combinedResult, ok := combined.Output["combined_result"].(map[string]any)
	if !ok {
		t.Fatalf("expected combined_result map, got %+v", combined.Output["combined_result"])
	}
	if combinedResult["count"] != float64(2) || combinedResult["firstId"] != "a" || combinedResult["eventId"] != "evt_handler" {
		t.Fatalf("unexpected combined result: %+v", combinedResult)
	}

	splitCtx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"payload": combinedResult,
			"items":   []any{"gold", "silver"},
		},
	})
	splitNode := marketing.WorkflowNode{
		ID:   "split",
		Type: "split",
		Config: json.RawMessage(`{
			"language": "LUA",
			"outputPath": "split_items",
			"source": "emit({items = {input('items')[1], input('items')[2], input('payload').firstId}})"
		}`),
	}
	split := transformScriptWorkflowNodeHandler("split", "split_items")(splitCtx, splitNode, runtime)
	if split.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected split success, got %+v", split)
	}
	items, ok := split.Output["split_items"].([]any)
	if !ok || len(items) != 3 || items[0] != "gold" || items[2] != "a" {
		t.Fatalf("unexpected split items: %+v", split.Output["split_items"])
	}
	if _, ok := split.Output["split_result"]; !ok {
		t.Fatalf("expected split_result output, got %+v", split.Output)
	}
}

func TestScheduleEventWorkflowNodeHandlerUsesGeneratedBizIDInEventID(t *testing.T) {
	node := marketing.WorkflowNode{
		ID:   "schedule_event",
		Type: "schedule-event",
		Config: json.RawMessage(`{
			"scheduleType":"CRON",
			"cron":"0 0 * * *",
			"timezone":"UTC",
			"eventType":"SCHEDULE_TICK",
			"bizType":"SCHEDULE",
			"bizIdTemplate":"planned:${activityId}:${nodeId}",
			"eventIdTemplate":"evt:${biz.bizId}"
		}`),
	}

	result := scheduleEventWorkflowNodeHandler(context.Background(), node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected schedule event success, got %+v", result)
	}
	schedulePlan, ok := result.Output["schedulePlan"].(map[string]any)
	if !ok {
		t.Fatalf("expected schedulePlan map, got %+v", result.Output["schedulePlan"])
	}
	eventTemplate, ok := schedulePlan["eventTemplate"].(map[string]any)
	if !ok {
		t.Fatalf("expected eventTemplate map, got %+v", schedulePlan["eventTemplate"])
	}
	biz, ok := eventTemplate["biz"].(map[string]any)
	if !ok {
		t.Fatalf("expected biz map, got %+v", eventTemplate["biz"])
	}
	expectedBizID := "planned:activity_handler:schedule_event"
	if biz["bizId"] != expectedBizID {
		t.Fatalf("expected generated bizId %q, got %+v", expectedBizID, biz["bizId"])
	}
	if eventTemplate["eventId"] != "evt:"+expectedBizID {
		t.Fatalf("expected eventId rendered from generated bizId, got %+v", eventTemplate["eventId"])
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

func TestStateEventWorkflowNodeHandlerRequiresResourceRefForExternalSources(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	node := marketing.WorkflowNode{
		ID:     "state_event",
		Type:   "state-event",
		Config: json.RawMessage(`{"source":"REDIS","namespace":"state-watch","keyTemplate":"activity:state:${activityId}","condition":"value.enabled == true","intervalSeconds":10}`),
	}

	result := service.stateEventWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_EVENT_RESOURCE_REF_REQUIRED" {
		t.Fatalf("expected missing resourceRef failure, got %+v", result)
	}
}

func TestStateEventWorkflowNodeHandlerReadsResourceAndMatchesCondition(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := repo.SaveResource(ctx, marketing.Resource{
		ID:           "res_state_watch",
		TenantID:     "site_001",
		Code:         "state_watch_resource",
		Name:         "State Watch Resource",
		Type:         marketing.ResourceDatabaseKV,
		Driver:       marketing.ResourceDriverInternal,
		Scope:        marketing.ResourceScopeTenant,
		Status:       marketing.ResourceActive,
		Capabilities: []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceUpsert},
	}); err != nil {
		t.Fatalf("save resource: %v", err)
	}
	seed := marketing.WorkflowNode{ID: "seed", Type: "state-store", Config: json.RawMessage(`{"resourceRef":"state_watch_resource","operation":"UPSERT","namespace":"state-watch","keyTemplate":"activity:state:${activityId}","valueType":"OBJECT","defaultValue":{"enabled":true,"count":3}}`)}
	if result := service.stateStoreWorkflowNodeHandler(ctx, seed, runtime); result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("seed state: %+v", result)
	}
	node := marketing.WorkflowNode{ID: "state_event", Type: "state-event", Config: json.RawMessage(`{"source":"RESOURCE","resourceRef":"state_watch_resource","namespace":"state-watch","keyTemplate":"activity:state:${activityId}","condition":"value.enabled == true","intervalSeconds":10}`)}
	result := service.stateEventWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["triggered"] != true {
		t.Fatalf("expected triggered state event, got %+v", result)
	}
	if result.Output["executionMode"] != "TRIGGER_ADAPTER" || result.Output["executionAuthority"] != "StateWatcher/ResourceAPI" {
		t.Fatalf("unexpected execution metadata: %+v", result.Output)
	}

	node.Config = json.RawMessage(`{"source":"RESOURCE","resourceRef":"state_watch_resource","namespace":"state-watch","keyTemplate":"activity:state:${activityId}","condition":"value.count > 10","intervalSeconds":10}`)
	skipped := service.stateEventWorkflowNodeHandler(ctx, node, runtime)
	if skipped.Status != marketing.WorkflowNodeSkipped || skipped.Output["triggered"] != false {
		t.Fatalf("expected skipped state event, got %+v", skipped)
	}
}

func TestMatchWorkflowNodeHandlerEnforcesActivityMatchSemantics(t *testing.T) {
	ctx := context.Background()
	node := marketing.WorkflowNode{ID: "match", Type: "match", Config: json.RawMessage(`{"requireActiveActivity":true,"fallbackAction":"NO_MATCH_FAIL"}`)}
	ok := matchWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if ok.Status != marketing.WorkflowNodeSuccess || ok.Output["matched"] != true {
		t.Fatalf("expected successful match, got %+v", ok)
	}

	eventTypeMismatch := testWorkflowHandlerRuntime(true)
	eventTypeMismatch.Inbox.Event.EventType = "CHECKIN_SUCCESS"
	failed := matchWorkflowNodeHandler(ctx, node, eventTypeMismatch)
	if failed.Status != marketing.WorkflowNodeFailed || failed.Output["errorCode"] != "MATCH_EVENT_TYPE_NOT_ALLOWED" {
		t.Fatalf("expected event type mismatch failure, got %+v", failed)
	}

	channelBlocked := testWorkflowHandlerRuntime(true)
	channelBlocked.Activity.Config = json.RawMessage(`{"entry":{"channels":["APP"]}}`)
	channelBlocked.Inbox.Event.Attributes["channel"] = "WEB"
	failed = matchWorkflowNodeHandler(ctx, node, channelBlocked)
	if failed.Status != marketing.WorkflowNodeFailed || failed.Output["errorCode"] != "ENTRY_CHANNEL_NOT_ALLOWED" {
		t.Fatalf("expected channel mismatch failure, got %+v", failed)
	}

	skipNode := marketing.WorkflowNode{ID: "match", Type: "match", Config: json.RawMessage(`{"requireActiveActivity":true,"fallbackAction":"NO_MATCH_SKIP"}`)}
	skipped := matchWorkflowNodeHandler(ctx, skipNode, eventTypeMismatch)
	if skipped.Status != marketing.WorkflowNodeSkipped || skipped.Output["errorCode"] != "MATCH_EVENT_TYPE_NOT_ALLOWED" {
		t.Fatalf("expected skipped mismatch, got %+v", skipped)
	}
}

func TestActivityStateWorkflowNodeHandlerPersistsInternalState(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	readNode := marketing.WorkflowNode{
		ID:     "state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"INTERNAL","operation":"READ","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}","missingPolicy":"EMPTY_STATE"}`),
	}

	read := service.activityStateWorkflowNodeHandler(ctx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess || read.Output["state_version"] != int64(0) {
		t.Fatalf("expected empty state success, got %+v", read)
	}
	state := read.Output["activity_state"].(map[string]any)
	if state["exists"] != false {
		t.Fatalf("expected missing state marker, got %+v", state)
	}

	writeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"state_patch": map[string]any{"lastCheckinDate": "2026-06-14", "streakDays": 1},
		},
	})
	upsertNode := marketing.WorkflowNode{
		ID:     "state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"INTERNAL","operation":"UPSERT","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}"}`),
	}
	upsert := service.activityStateWorkflowNodeHandler(writeCtx, upsertNode, runtime)
	if upsert.Status != marketing.WorkflowNodeSuccess || upsert.Output["state_version"] != int64(1) {
		t.Fatalf("expected upsert success, got %+v", upsert)
	}

	read = service.activityStateWorkflowNodeHandler(ctx, readNode, runtime)
	state = read.Output["activity_state"].(map[string]any)
	data := state["data"].(map[string]any)
	if state["exists"] != true || data["lastCheckinDate"] != "2026-06-14" || data["streakDays"] != float64(1) {
		t.Fatalf("expected persisted state, got %+v", state)
	}

	casCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"state_version": int64(1),
			"state_patch":   map[string]any{"streakDays": 2},
		},
	})
	casNode := marketing.WorkflowNode{
		ID:     "state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"INTERNAL","operation":"CAS","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}","requireVersion":true}`),
	}
	cas := service.activityStateWorkflowNodeHandler(casCtx, casNode, runtime)
	if cas.Status != marketing.WorkflowNodeSuccess || cas.Output["state_version"] != int64(2) {
		t.Fatalf("expected cas success, got %+v", cas)
	}
	conflict := service.activityStateWorkflowNodeHandler(casCtx, casNode, runtime)
	if conflict.Status != marketing.WorkflowNodeFailed || conflict.Output["errorCode"] != "ACTIVITY_STATE_VERSION_CONFLICT" {
		t.Fatalf("expected cas conflict, got %+v", conflict)
	}
}

func TestActivityStateWorkflowNodeHandlerSupportsExternalResourceState(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	writeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"state_patch": map[string]any{"lastCheckinDate": "2026-06-19", "streakDays": 8},
		},
	})
	upsertNode := marketing.WorkflowNode{
		ID:     "external-state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"EXTERNAL","resourceRef":"default_workflow_state","operation":"UPSERT","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}:external","missingPolicy":"EMPTY_STATE"}`),
	}
	upsert := service.activityStateWorkflowNodeHandler(writeCtx, upsertNode, runtime)
	if upsert.Status != marketing.WorkflowNodeSuccess || upsert.Output["storeType"] != "EXTERNAL" || upsert.Output["resourceRef"] != "default_workflow_state" {
		t.Fatalf("expected external activity state upsert success, got %+v", upsert)
	}
	state := upsert.Output["activity_state"].(map[string]any)
	data := state["data"].(map[string]any)
	if state["external"] != true || data["lastCheckinDate"] != "2026-06-19" || data["streakDays"] != float64(8) {
		t.Fatalf("expected external activity state shape, got %+v", state)
	}

	readNode := marketing.WorkflowNode{
		ID:     "external-state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"EXTERNAL","resourceRef":"default_workflow_state","operation":"READ","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}:external","missingPolicy":"BLOCK"}`),
	}
	read := service.activityStateWorkflowNodeHandler(ctx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess || read.Output["state_version"] != int64(1) {
		t.Fatalf("expected external activity state read success, got %+v", read)
	}
}

func TestActivityStateWorkflowNodeHandlerSupportsStateOperationChain(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	seedCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"state_patch": map[string]any{"lastCheckinDate": "2026-06-18", "streakDays": 1},
		},
	})
	seedNode := marketing.WorkflowNode{
		ID:     "seed-state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"EXTERNAL","resourceRef":"default_workflow_state","operation":"UPSERT","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}:chain"}`),
	}
	if seed := service.activityStateWorkflowNodeHandler(seedCtx, seedNode, runtime); seed.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected seed success, got %+v", seed)
	}

	chainNode := marketing.WorkflowNode{
		ID:   "checkin-state-chain",
		Type: "activity-state",
		Config: json.RawMessage(`{
			"namespace":"checkin",
			"providerChain":[
				{"source":"RESOURCE","name":"current","operation":"READ","resourceRef":"default_workflow_state","keyTemplate":"${activityId}:${subject.id}:chain","outputAs":"currentState","onMiss":"BLOCK"},
				{"source":"RESOURCE","name":"claim","operation":"UPSERT","resourceRef":"default_workflow_state","keyTemplate":"${activityId}:${subject.id}:chain","valuePath":"$.input.currentState_value","outputAs":"updatedState"}
			]
		}`),
	}
	result := service.activityStateWorkflowNodeHandler(ctx, chainNode, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected state chain success, got %+v", result)
	}
	if result.Output["operation"] != "CHAIN" {
		t.Fatalf("expected chain operation output, got %+v", result.Output)
	}
	traces, ok := result.Output["stateChain"].([]any)
	if !ok || len(traces) != 2 {
		t.Fatalf("expected two state chain traces, got %+v", result.Output["stateChain"])
	}
	results := result.Output["state_results"].(map[string]any)
	updated := results["updatedState"].(map[string]any)
	value := updated["stored_value"].(map[string]any)
	if value["lastCheckinDate"] != "2026-06-18" || value["streakDays"] != float64(1) {
		t.Fatalf("expected second step to use first step value, got %+v", value)
	}
	state := result.Output["activity_state"].(map[string]any)
	if state["external"] != true || state["resourceRef"] != "default_workflow_state" {
		t.Fatalf("expected external activity state output from chain, got %+v", state)
	}
}

func TestActivityStateWorkflowNodeHandlerSupportsInternalStateOperationChainWithoutResourceRef(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	seedCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"state_patch": map[string]any{"lastCheckinDate": "2026-06-20", "streakDays": 2},
		},
	})
	seedNode := marketing.WorkflowNode{
		ID:     "seed-state",
		Type:   "activity-state",
		Config: json.RawMessage(`{"storeType":"INTERNAL","operation":"UPSERT","namespace":"checkin","stateKeyTemplate":"${activityId}:${subject.id}:internal-chain"}`),
	}
	if seed := service.activityStateWorkflowNodeHandler(seedCtx, seedNode, runtime); seed.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected seed success, got %+v", seed)
	}

	chainNode := marketing.WorkflowNode{
		ID:   "checkin-state-chain",
		Type: "activity-state",
		Config: json.RawMessage(`{
			"namespace":"checkin",
			"providerChain":[
				{"source":"INTERNAL","operation":"READ","keyTemplate":"${activityId}:${subject.id}:internal-chain","outputAs":"currentState","onMiss":"BLOCK"}
			]
		}`),
	}
	result := service.activityStateWorkflowNodeHandler(ctx, chainNode, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected internal state chain success, got %+v", result)
	}
	if result.Output["state_version"] != int64(1) {
		t.Fatalf("expected internal state version, got %+v", result.Output)
	}
	state := result.Output["activity_state"].(map[string]any)
	if state["external"] == true || state["resourceRef"] != nil {
		t.Fatalf("expected internal activity state without resourceRef, got %+v", state)
	}
	data := state["data"].(map[string]any)
	if data["lastCheckinDate"] != "2026-06-20" || data["streakDays"] != float64(2) {
		t.Fatalf("expected internal state data, got %+v", data)
	}
}

func TestStateStoreWorkflowNodeHandlerSupportsMemoryValues(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	vars := map[string]any{}
	writeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		Variables: vars,
		ResolvedInput: map[string]any{
			"value": map[string]any{"streakDays": 1},
		},
	})
	node := marketing.WorkflowNode{
		ID:     "store-user",
		Type:   "state-store",
		Config: json.RawMessage(`{"storageType":"MEMORY","operation":"DEFINE","namespace":"checkin","keyTemplate":"${activityId}:${eventId}:${nodeId}","valueType":"OBJECT"}`),
	}

	write := service.stateStoreWorkflowNodeHandler(writeCtx, node, runtime)
	if write.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected memory write success, got %+v", write)
	}
	entry := write.Output["storage_entry"].(map[string]any)
	if entry["key"] != runtime.Activity.ID+":"+runtime.Inbox.Event.EventID+":store-user" {
		t.Fatalf("expected node id in rendered key, got %+v", entry)
	}

	readNode := node
	readNode.Config = json.RawMessage(`{"storageType":"MEMORY","operation":"READ","namespace":"checkin","keyTemplate":"${activityId}:${eventId}:${nodeId}","valueType":"OBJECT"}`)
	readCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{Variables: vars})
	read := service.stateStoreWorkflowNodeHandler(readCtx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected memory read success, got %+v", read)
	}
	value := read.Output["stored_value"].(map[string]any)
	if value["streakDays"] != float64(1) {
		t.Fatalf("expected stored object, got %+v", value)
	}

	appendNode := marketing.WorkflowNode{
		ID:     "events",
		Type:   "state-store",
		Config: json.RawMessage(`{"storageType":"MEMORY","operation":"APPEND","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:events","valueType":"ARRAY"}`),
	}
	appendCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		Variables:     vars,
		ResolvedInput: map[string]any{"value": "CHECKIN_SUCCESS"},
	})
	appendResult := service.stateStoreWorkflowNodeHandler(appendCtx, appendNode, runtime)
	if appendResult.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected append success, got %+v", appendResult)
	}
	items := appendResult.Output["stored_value"].([]any)
	if len(items) != 1 || items[0] != "CHECKIN_SUCCESS" {
		t.Fatalf("expected appended item, got %+v", items)
	}
}

func TestStateStoreWorkflowNodeHandlerPersistsDatabaseValuesAndCAS(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:     "store-db",
		Type:   "state-store",
		Config: json.RawMessage(`{"storageType":"DATABASE","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}","valueType":"TABLE","defaultValue":[{"day":"2026-06-14","reward":10}]}`),
	}

	upsert := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if upsert.Status != marketing.WorkflowNodeSuccess || upsert.Output["storage_version"] != int64(1) {
		t.Fatalf("expected database upsert success, got %+v", upsert)
	}
	rows := upsert.Output["stored_value"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected stored table row, got %+v", rows)
	}

	casCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"value":           []any{map[string]any{"day": "2026-06-15", "reward": 20}},
			"state_version":   int64(1),
			"storage_version": int64(1),
		},
	})
	casNode := marketing.WorkflowNode{
		ID:     "store-db",
		Type:   "state-store",
		Config: json.RawMessage(`{"storageType":"DATABASE","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}","valueType":"TABLE","requireVersion":true}`),
	}
	cas := service.stateStoreWorkflowNodeHandler(casCtx, casNode, runtime)
	if cas.Status != marketing.WorkflowNodeSuccess || cas.Output["storage_version"] != int64(2) {
		t.Fatalf("expected database cas success, got %+v", cas)
	}
	conflict := service.stateStoreWorkflowNodeHandler(casCtx, casNode, runtime)
	if conflict.Status != marketing.WorkflowNodeFailed || conflict.Output["errorCode"] != "STATE_STORE_VERSION_CONFLICT" {
		t.Fatalf("expected database cas conflict, got %+v", conflict)
	}
}

func TestStateStoreWorkflowNodeHandlerAppendsDatabaseValuesInRepository(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:     "store-events",
		Type:   "state-store",
		Config: json.RawMessage(`{"storageType":"DATABASE","operation":"APPEND","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:events","valueType":"ARRAY"}`),
	}

	firstCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": "first"},
	})
	first := service.stateStoreWorkflowNodeHandler(firstCtx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess || first.Output["storage_version"] != int64(1) {
		t.Fatalf("expected first append success, got %+v", first)
	}

	secondCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": "second"},
	})
	second := service.stateStoreWorkflowNodeHandler(secondCtx, node, runtime)
	if second.Status != marketing.WorkflowNodeSuccess || second.Output["storage_version"] != int64(2) {
		t.Fatalf("expected second append success, got %+v", second)
	}
	items := second.Output["stored_value"].([]any)
	if len(items) != 2 || items[0] != "first" || items[1] != "second" {
		t.Fatalf("expected repository append to preserve both items, got %+v", items)
	}
}

func TestStateStoreWorkflowNodeHandlerSupportsHTTPProvider(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())
	runtime := testWorkflowHandlerRuntime(true)
	var received map[string]any
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"provider":"ok"},"version":7,"exists":true}`))
	}))
	defer provider.Close()
	node := marketing.WorkflowNode{
		ID:   "external-store",
		Type: "state-store",
		Config: json.RawMessage(`{
			"storageType":"HTTP",
			"operation":"UPSERT",
			"namespace":"checkin",
			"keyTemplate":"${activityId}:${subject.id}:external",
			"valueType":"OBJECT",
			"providerEndpoint":"` + provider.URL + `",
			"timeoutSeconds":3
		}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["storage_version"] != int64(7) {
		t.Fatalf("expected http provider success, got %+v", result)
	}
	if received["operation"] != "UPSERT" || received["namespace"] != "checkin" || received["key"] != runtime.Activity.ID+":"+runtime.Inbox.Event.Subject.ID+":external" {
		t.Fatalf("expected normalized provider request, got %+v", received)
	}
	value := result.Output["stored_value"].(map[string]any)
	if value["provider"] != "ok" {
		t.Fatalf("expected normalized provider output, got %+v", result.Output)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesDatabaseTableResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:     runtime.Activity.TenantID,
		WorkspaceID:  runtime.Activity.WorkspaceID,
		Code:         "checkin_user_state",
		Name:         "签到用户状态表",
		Type:         marketing.ResourceDatabaseTable,
		Driver:       marketing.ResourceDriverPostgres,
		Scope:        marketing.ResourceScopeWorkspace,
		Status:       marketing.ResourceActive,
		Config:       json.RawMessage(`{"physicalTable":"marketing_checkin_user_state","primaryKey":["tenantId","activityId","subjectId"]}`),
		Capabilities: []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceUpsert, marketing.ResourceDelete},
	}); err != nil {
		t.Fatalf("save table resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "store-table",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"checkin_user_state","operation":"UPSERT","namespace":"checkin","keyTemplate":"ignored","valueType":"TABLE"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": []any{map[string]any{"streakDays": 2, "lastCheckinDate": "2026-06-14"}}},
	})
	upsert := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if upsert.Status != marketing.WorkflowNodeSuccess || upsert.Output["storageType"] != "DATABASE_TABLE" || upsert.Output["rowCount"] != 1 {
		t.Fatalf("expected database table upsert, got %+v", upsert)
	}
	rows := upsert.Output["stored_value"].([]any)
	row := rows[0].(map[string]any)
	if row["tenantId"] != runtime.Activity.TenantID || row["activityId"] != runtime.Activity.ID || row["subjectId"] != runtime.Inbox.Event.Subject.ID {
		t.Fatalf("expected enriched table row, got %+v", row)
	}

	readNode := node
	readNode.Config = json.RawMessage(`{"resourceRef":"checkin_user_state","operation":"READ","namespace":"checkin","keyTemplate":"${tenantId}:${activityId}:${subject.id}","valueType":"TABLE","missingPolicy":"BLOCK"}`)
	read := service.stateStoreWorkflowNodeHandler(ctx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess || read.Output["storageType"] != "DATABASE_TABLE" {
		t.Fatalf("expected database table read, got %+v", read)
	}
	readValue := read.Output["stored_value"].(map[string]any)
	if readValue["streakDays"] != float64(2) {
		t.Fatalf("expected stored table row, got %+v", readValue)
	}
}

func TestStateStoreWorkflowNodeHandlerAppendsDatabaseTableResourceRows(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "checkin_history_table",
		Name:        "签到历史表",
		Type:        marketing.ResourceDatabaseTable,
		Driver:      marketing.ResourceDriverPostgres,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"physicalTable":"marketing_checkin_history","primaryKey":["tenantId","activityId","subjectId"]}`),
	}); err != nil {
		t.Fatalf("save table resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "append-table",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"checkin_history_table","operation":"APPEND","namespace":"checkin","keyTemplate":"ignored","valueType":"TABLE"}`),
	}
	for _, streakDays := range []int{1, 2} {
		nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
			ResolvedInput: map[string]any{"value": map[string]any{"streakDays": streakDays}},
		})
		result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
		if result.Status != marketing.WorkflowNodeSuccess {
			t.Fatalf("expected append success, got %+v", result)
		}
	}

	readNode := node
	readNode.Config = json.RawMessage(`{"resourceRef":"checkin_history_table","operation":"READ","namespace":"checkin","keyTemplate":"${tenantId}:${activityId}:${subject.id}","valueType":"TABLE","missingPolicy":"BLOCK"}`)
	read := service.stateStoreWorkflowNodeHandler(ctx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected read success, got %+v", read)
	}
	items := read.Output["stored_value"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected appended rows, got %+v", read.Output["stored_value"])
	}
}

func TestStateStoreWorkflowNodeHandlerAppliesDatabaseTableSchemaAndMapping(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "mapped_checkin_table",
		Name:        "映射签到表",
		Type:        marketing.ResourceDatabaseTable,
		Driver:      marketing.ResourceDriverPostgres,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config: json.RawMessage(`{
			"physicalTable":"marketing_mapped_checkin",
			"primaryKey":["tenantId","activityId","subjectId","bizId"],
			"schema":{"columns":{
				"tenantId":{"type":"STRING","required":true},
				"activityId":{"type":"STRING","required":true},
				"subjectId":{"type":"STRING","required":true},
				"bizId":{"type":"STRING","required":true},
				"streakDays":{"type":"NUMBER","required":true},
				"eventType":{"type":"STRING","required":true}
			}},
			"keyMapping":{"bizId":"$.event.biz.bizId"},
			"valueMapping":{"streakDays":"$.row.streak","eventType":"$.event.eventType"}
		}`),
	}); err != nil {
		t.Fatalf("save mapped table resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "mapped-table",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"mapped_checkin_table","operation":"UPSERT","namespace":"checkin","keyTemplate":"ignored","valueType":"TABLE"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streak": 3}},
	})
	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected mapped table success, got %+v", result)
	}
	row := result.Output["stored_value"].([]any)[0].(map[string]any)
	if row["bizId"] != runtime.Inbox.Event.Biz.BizID || row["streakDays"] != float64(3) || row["eventType"] != runtime.Inbox.Event.EventType {
		t.Fatalf("expected mapped row values, got %+v", row)
	}
	storageEntry := result.Output["storage_entry"].(map[string]any)
	if storageEntry["key"] != runtime.Activity.TenantID+":"+runtime.Activity.ID+":"+runtime.Inbox.Event.Subject.ID+":"+runtime.Inbox.Event.Biz.BizID {
		t.Fatalf("expected mapped primary key storage key, got %+v", result.Output)
	}
}

func TestStateStoreWorkflowNodeHandlerRejectsDatabaseTableSchemaViolation(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "schema_checkin_table",
		Name:        "Schema 签到表",
		Type:        marketing.ResourceDatabaseTable,
		Driver:      marketing.ResourceDriverPostgres,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"physicalTable":"marketing_schema_checkin","primaryKey":["tenantId","activityId","subjectId"],"schema":{"columns":{"tenantId":{"type":"STRING","required":true},"activityId":{"type":"STRING","required":true},"subjectId":{"type":"STRING","required":true},"streakDays":{"type":"NUMBER","required":true}}}}`),
	}); err != nil {
		t.Fatalf("save schema table resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "schema-table",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"schema_checkin_table","operation":"UPSERT","namespace":"checkin","keyTemplate":"ignored","valueType":"TABLE"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"note": "missing streak"}},
	})
	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_STORE_TABLE_SCHEMA_INVALID" {
		t.Fatalf("expected schema violation, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerRejectsInvalidDatabaseTableResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "bad_table_resource",
		Name:        "错误公共表",
		Type:        marketing.ResourceDatabaseTable,
		Driver:      marketing.ResourceDriverPostgres,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"physicalTable":"marketing_bad_table"}`),
	}); err != nil {
		t.Fatalf("save invalid table resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "bad-table",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"bad_table_resource","operation":"UPSERT","namespace":"checkin","keyTemplate":"ignored","valueType":"TABLE"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})
	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "RESOURCE_TABLE_PRIMARY_KEY_REQUIRED" {
		t.Fatalf("expected invalid resource failure, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesHTTPProviderResourceConfig(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	var receivedMethod string
	var receivedHeader string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedHeader = r.Header.Get("X-Provider")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"resourceProvider":"ok"},"version":9,"exists":true}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:     runtime.Activity.TenantID,
		WorkspaceID:  runtime.Activity.WorkspaceID,
		Code:         "external_storage_provider",
		Name:         "外部存储 Provider",
		Type:         marketing.ResourceHTTPProvider,
		Driver:       marketing.ResourceDriverREST,
		Scope:        marketing.ResourceScopeWorkspace,
		Status:       marketing.ResourceActive,
		Config:       json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"PUT","headers":{"X-Provider":"resource-config"},"timeoutSeconds":3}`),
		Capabilities: []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceUpsert, marketing.ResourceTestConnection},
	}); err != nil {
		t.Fatalf("save http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "resource-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"external_storage_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:external","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["storage_version"] != int64(9) {
		t.Fatalf("expected http resource provider success, got %+v", result)
	}
	if receivedMethod != http.MethodPut || receivedHeader != "resource-config" {
		t.Fatalf("expected provider config method/header, got method=%s header=%s", receivedMethod, receivedHeader)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesHTTPProviderSecretRefAndResponseSchema(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	t.Setenv("EXM_PROVIDER_TOKEN", "secret-token-001")
	var receivedAuth string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"resourceProvider":"ok"},"version":12,"exists":true}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "secret_http_provider",
		Name:        "Secret HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","secretRef":"env:EXM_PROVIDER_TOKEN","responseSchema":{"required":["$.value"],"fields":{"$.version":"NUMBER","$.value":"OBJECT"}}}`),
	}); err != nil {
		t.Fatalf("save secret http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "secret-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"secret_http_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:secret","valueType":"OBJECT"}`),
	}
	result := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["storage_version"] != int64(12) {
		t.Fatalf("expected secret http provider success, got %+v", result)
	}
	if receivedAuth != "Bearer secret-token-001" {
		t.Fatalf("expected Authorization from secretRef, got %q", receivedAuth)
	}
}

func TestStateStoreWorkflowNodeHandlerRejectsHTTPProviderResponseSchemaMismatch(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":"bad","version":"v12"}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "bad_response_http_provider",
		Name:        "Bad Response HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","responseSchema":{"fields":{"$.version":"NUMBER","$.value":"OBJECT"}}}`),
	}); err != nil {
		t.Fatalf("save bad response http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "bad-response-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"bad_response_http_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:bad-response","valueType":"OBJECT"}`),
	}
	result := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID" {
		t.Fatalf("expected response schema failure, got %+v", result)
	}
}

func TestResourceHTTPProviderRejectsSensitiveLiteralHeaderWithoutSecretRef(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID: "site_001",
		Code:     "literal_secret_http_provider",
		Name:     "Literal Secret HTTP Provider",
		Type:     marketing.ResourceHTTPProvider,
		Driver:   marketing.ResourceDriverREST,
		Scope:    marketing.ResourceScopeWorkspace,
		Status:   marketing.ResourceActive,
		Config:   json.RawMessage(`{"endpoint":"https://provider.example/state","method":"POST","headers":{"Authorization":"Bearer literal-token"}}`),
	}); err != nil {
		t.Fatalf("save literal secret provider: %v", err)
	}
	result, err := service.TestResource(ctx, "site_001", "literal_secret_http_provider", "tester")
	if err != nil {
		t.Fatalf("test literal secret provider: %v", err)
	}
	if result["ok"] != false || result["errorCode"] != "RESOURCE_HTTP_SECRET_REF_REQUIRED" {
		t.Fatalf("expected secret ref required, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerRejectsHTTPProviderOutsideAllowlist(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "restricted_http_provider",
		Name:        "受限 HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"https://evil.example/internal/state","method":"POST","allowedHosts":["provider.example"]}`),
	}); err != nil {
		t.Fatalf("save restricted http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "restricted-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"restricted_http_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT"}`),
	}
	result := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_STORE_HTTP_ENDPOINT_DENIED" {
		t.Fatalf("expected http allowlist rejection, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerRejectsHTTPProviderPrivateNetworkWhenConfigured(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "private_http_provider",
		Name:        "内网 HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"http://127.0.0.1/internal/state","method":"POST","blockPrivateNetwork":true}`),
	}); err != nil {
		t.Fatalf("save private http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "private-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"private_http_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT"}`),
	}
	result := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_STORE_HTTP_ENDPOINT_DENIED" {
		t.Fatalf("expected private network rejection, got %+v", result)
	}
}

func TestWorkflowValidateHTTPProviderEndpointSecurityPolicy(t *testing.T) {
	if err := workflowValidateHTTPProviderEndpoint("https://provider.example/internal/state", map[string]any{"allowedHostSuffixes": []any{"example"}}); err != nil {
		t.Fatalf("expected provider.example to satisfy suffix allowlist: %v", err)
	}
	if err := workflowValidateHTTPProviderEndpoint("https://user:pass@provider.example/internal/state", map[string]any{}); err == nil {
		t.Fatalf("expected userinfo endpoint to be rejected")
	}
	if err := workflowValidateHTTPProviderEndpoint("http://localhost/internal/state", map[string]any{"blockPrivateNetwork": true}); err == nil {
		t.Fatalf("expected localhost endpoint to be rejected")
	}
	if err := workflowValidateHTTPProviderEndpoint("http://127.0.0.1/internal/state", map[string]any{"blockPrivateNetwork": true}); err == nil {
		t.Fatalf("expected loopback endpoint to be rejected")
	}
	client := workflowHTTPProviderClient(map[string]any{"blockPrivateNetwork": true})
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.DialContext == nil {
		t.Fatalf("expected guarded http transport, got %+v", client.Transport)
	}
	conn, err := transport.DialContext(context.Background(), "tcp", "127.0.0.1:80")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatalf("expected guarded dial to reject loopback address")
	}
}

func TestStateStoreWorkflowNodeHandlerUsesHTTPProviderHTTPMethodAlias(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	var receivedMethod string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"resourceProvider":"ok"},"version":4,"exists":true}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "external_storage_provider_http_method",
		Name:        "外部存储 Provider HTTP Method",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"` + provider.URL + `","httpMethod":"PUT"}`),
	}); err != nil {
		t.Fatalf("save http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "resource-http-method-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"external_storage_provider_http_method","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:external","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || receivedMethod != http.MethodPut {
		t.Fatalf("expected httpMethod alias to use PUT, method=%s result=%+v", receivedMethod, result)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesHTTPProviderTimeoutResourceConfig(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"late":true},"version":1,"exists":true}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "slow_external_storage_provider",
		Name:        "慢外部存储 Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","timeoutSeconds":0.01}`),
	}); err != nil {
		t.Fatalf("save http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "slow-http-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"slow_external_storage_provider","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:slow","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_STORE_HTTP_FAILED" {
		t.Fatalf("expected resource timeout to override schema default, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesResourceRefAndAudits(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:     "resource-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"default_workflow_state","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:resource","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 3}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected resourceRef state-store success, got %+v", result)
	}
	if result.Output["resourceRef"] != "default_workflow_state" || result.Output["storageType"] != "DATABASE" {
		t.Fatalf("expected database resource output, got %+v", result.Output)
	}
	logs, err := repo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: "res_builtin_default_workflow_state"})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].ResourceCode != "default_workflow_state" || logs[0].NodeID != "resource-store" || logs[0].Status != marketing.ResourceAuditSuccess {
		t.Fatalf("expected resource audit log, got %+v", logs)
	}
}

func TestStateStoreWorkflowNodeHandlerCASRequiresVersion(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:     "cas-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"default_workflow_state","operation":"CAS","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:cas","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})
	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "STATE_STORE_VERSION_REQUIRED" {
		t.Fatalf("expected cas version required, got %+v", result)
	}
	logs, err := repo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: "res_builtin_default_workflow_state"})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Operation != marketing.ResourceCAS || logs[0].Status != marketing.ResourceAuditFailed {
		t.Fatalf("expected failed CAS resource audit, got %+v", logs)
	}
}

func TestStateStoreWorkflowNodeHandlerCASChecksVersion(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	upsertNode := marketing.WorkflowNode{
		ID:     "cas-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"default_workflow_state","operation":"UPSERT","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:cas","valueType":"OBJECT"}`),
	}
	upsertCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 1}},
	})
	created := service.stateStoreWorkflowNodeHandler(upsertCtx, upsertNode, runtime)
	if created.Status != marketing.WorkflowNodeSuccess || created.Output["storage_version"] != int64(1) {
		t.Fatalf("expected initial upsert, got %+v", created)
	}

	casNode := upsertNode
	casNode.Config = json.RawMessage(`{"resourceRef":"default_workflow_state","operation":"CAS","namespace":"checkin","keyTemplate":"${activityId}:${subject.id}:cas","valueType":"OBJECT","expectedVersion":"1"}`)
	casCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 2}},
	})
	updated := service.stateStoreWorkflowNodeHandler(casCtx, casNode, runtime)
	if updated.Status != marketing.WorkflowNodeSuccess || updated.Output["storage_version"] != int64(2) {
		t.Fatalf("expected cas update, got %+v", updated)
	}

	conflict := service.stateStoreWorkflowNodeHandler(casCtx, casNode, runtime)
	if conflict.Status != marketing.WorkflowNodeFailed || conflict.Output["errorCode"] != "STATE_STORE_VERSION_CONFLICT" {
		t.Fatalf("expected cas version conflict, got %+v", conflict)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesRedisKVResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "redis_user_state",
		Name:        "Redis 用户状态",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"marketing:user"}`),
	}); err != nil {
		t.Fatalf("save redis resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "redis-store",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"redis_user_state","operation":"UPSERT","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 4}},
	})
	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["storageType"] != "REDIS" || result.Output["redisKey"] != "marketing:user:"+runtime.Inbox.Event.Subject.ID {
		t.Fatalf("expected redis kv success, got %+v", result)
	}

	readNode := node
	readNode.Config = json.RawMessage(`{"resourceRef":"redis_user_state","operation":"READ","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT","missingPolicy":"BLOCK"}`)
	read := service.stateStoreWorkflowNodeHandler(ctx, readNode, runtime)
	if read.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected redis read success, got %+v", read)
	}
	value := read.Output["stored_value"].(map[string]any)
	if value["streakDays"] != float64(4) {
		t.Fatalf("expected redis value, got %+v", value)
	}
}

func TestFactQueryWorkflowNodeHandlerReadsRedisKVResourceRef(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "user_fact_cache",
		Name:        "用户事实缓存",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"fact:user"}`),
	}); err != nil {
		t.Fatalf("save fact cache resource: %v", err)
	}
	storeNode := marketing.WorkflowNode{
		ID:     "store-user-profile",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"user_fact_cache","operation":"UPSERT","namespace":"facts","keyTemplate":"${subject.id}:USER_PROFILE","valueType":"OBJECT"}`),
	}
	storeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"vipLevel": "VIP1", "country": "BR"}},
	})
	stored := service.stateStoreWorkflowNodeHandler(storeCtx, storeNode, runtime)
	if stored.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected fact cache seed success, got %+v", stored)
	}

	node := marketing.WorkflowNode{
		ID:     "fact-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"resourceRef":"user_fact_cache","factKeys":["USER_PROFILE"],"namespace":"facts","keyTemplate":"${subject.id}:${factCode}","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected resource fact query success, got %+v", result)
	}
	facts := result.Output["facts"].(map[string]any)
	profile := facts["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != "VIP1" || result.Output["resourceRef"] != "user_fact_cache" || result.Output["source"] != "RESOURCE" {
		t.Fatalf("expected profile facts from resource, got %+v", result.Output)
	}
}

type workflowUserProfileTestRepository struct {
	marketing.Repository
	profiles map[string]marketing.UserProfile
	kyc      map[string]marketing.KYCProfile
	risk     map[string]marketing.RiskProfile
}

func (r workflowUserProfileTestRepository) GetUserProfile(_ context.Context, tenantID string, subjectType string, subjectID string) (marketing.UserProfile, error) {
	profile, ok := r.profiles[tenantID+"|"+subjectType+"|"+subjectID]
	if !ok {
		return marketing.UserProfile{}, marketing.ErrNotFound
	}
	return profile, nil
}

func (r workflowUserProfileTestRepository) GetKYCProfile(_ context.Context, tenantID string, subjectType string, subjectID string) (marketing.KYCProfile, error) {
	profile, ok := r.kyc[tenantID+"|"+subjectType+"|"+subjectID]
	if !ok {
		return marketing.KYCProfile{}, marketing.ErrNotFound
	}
	return profile, nil
}

func (r workflowUserProfileTestRepository) GetRiskProfile(_ context.Context, tenantID string, subjectType string, subjectID string) (marketing.RiskProfile, error) {
	profile, ok := r.risk[tenantID+"|"+subjectType+"|"+subjectID]
	if !ok {
		return marketing.RiskProfile{}, marketing.ErrNotFound
	}
	return profile, nil
}

func TestFactQueryWorkflowNodeHandlerReadsUserProfileStoreFromDefaultMarketingDatabase(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_resource_001"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_resource_001": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_resource_001",
				UserLevel:   2,
				VIPLevel:    3,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:     "fact-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"resourceRef":"default_user_profile_store","factKeys":["USER_PROFILE"],"namespace":"facts","table":"marketing_user_profile","subjectPath":"subject.id","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected default user profile resource success, got %+v", result)
	}
	if result.Output["source"] != "RESOURCE" || result.Output["resourceRef"] != "default_user_profile_store" {
		t.Fatalf("expected resource-backed fact query, got %+v", result.Output)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["exists"] != true || profile["level"] != 2 || profile["vipLevel"] != 3 {
		t.Fatalf("expected user profile facts from resource, got %+v", profile)
	}
	logs, err := baseRepo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: "res_builtin_default_user_profile_store"})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].ResourceCode != "default_user_profile_store" || logs[0].Operation != marketing.ResourceRead || logs[0].Status != marketing.ResourceAuditSuccess {
		t.Fatalf("expected default user profile store audit log, got %+v", logs)
	}
}

func TestFactQueryWorkflowNodeHandlerReadsUserProfileViaProviderResource(t *testing.T) {
	ctx := context.Background()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_provider_001"}
	repo := workflowUserProfileTestRepository{
		Repository: marketing.NewMemoryRepository(),
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_provider_001": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_provider_001",
				UserLevel:   4,
				VIPLevel:    5,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:     "fact-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"source":"PROVIDER","resourceRef":"user-profile-provider","factKeys":["USER_PROFILE"],"namespace":"facts","subjectPath":"subject.id","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected provider resource fact query success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["exists"] != true || profile["level"] != 4 || profile["vipLevel"] != 5 {
		t.Fatalf("expected user profile facts from provider resource, got %+v", profile)
	}
	if result.Output["resourceRef"] != "default_user_profile_store" {
		t.Fatalf("expected provider to resolve target resource, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerReadsKYCProfileStoreFromDefaultMarketingDatabase(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_kyc_001"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		kyc: map[string]marketing.KYCProfile{
			runtime.Activity.TenantID + "|USER|u_kyc_001": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_kyc_001",
				Status:      "VERIFIED",
				Level:       "STANDARD",
				Country:     "US",
				Provider:    "INTERNAL_USER_PROFILE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:     "kyc-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"resourceRef":"default_kyc_profile_store","factKeys":["KYC_PROFILE"],"namespace":"facts","table":"marketing_kyc_profile","subjectPath":"subject.id","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected default kyc profile resource success, got %+v", result)
	}
	if result.Output["source"] != "RESOURCE" || result.Output["resourceRef"] != "default_kyc_profile_store" {
		t.Fatalf("expected kyc resource-backed fact query, got %+v", result.Output)
	}
	profile := result.Output["facts"].(map[string]any)["KYC_PROFILE"].(map[string]any)
	if profile["exists"] != true || profile["verified"] != true || profile["status"] != "VERIFIED" || profile["level"] != "STANDARD" {
		t.Fatalf("expected kyc profile facts from resource, got %+v", profile)
	}
	logs, err := baseRepo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: "res_builtin_default_kyc_profile_store"})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].ResourceCode != "default_kyc_profile_store" || logs[0].Operation != marketing.ResourceRead || logs[0].Status != marketing.ResourceAuditSuccess {
		t.Fatalf("expected default kyc profile store audit log, got %+v", logs)
	}
}

func TestFactQueryWorkflowNodeHandlerReadsRiskProfileStoreFromDefaultMarketingDatabase(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_risk_001"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		risk: map[string]marketing.RiskProfile{
			runtime.Activity.TenantID + "|USER|u_risk_001": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_risk_001",
				Score:       12,
				Level:       "LOW",
				Status:      "CLEAR",
				Blacklisted: false,
				Country:     "US",
				Provider:    "INTERNAL_USER_PROFILE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:     "risk-query",
		Type:   "fact-query",
		Config: json.RawMessage(`{"resourceRef":"default_risk_profile_store","factKeys":["RISK_PROFILE"],"namespace":"facts","table":"marketing_risk_profile","subjectPath":"subject.id","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected default risk profile resource success, got %+v", result)
	}
	if result.Output["source"] != "RESOURCE" || result.Output["resourceRef"] != "default_risk_profile_store" {
		t.Fatalf("expected risk resource-backed fact query, got %+v", result.Output)
	}
	profile := result.Output["facts"].(map[string]any)["RISK_PROFILE"].(map[string]any)
	if profile["exists"] != true || profile["clear"] != true || profile["score"] != 12 || profile["level"] != "LOW" || profile["blacklisted"] != false {
		t.Fatalf("expected risk profile facts from resource, got %+v", profile)
	}
	logs, err := baseRepo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: "res_builtin_default_risk_profile_store"})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].ResourceCode != "default_risk_profile_store" || logs[0].Operation != marketing.ResourceRead || logs[0].Status != marketing.ResourceAuditSuccess {
		t.Fatalf("expected default risk profile store audit log, got %+v", logs)
	}
}

func TestFactQueryWorkflowNodeHandlerUsesStructuredProviderChainResourceStep(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_chain_001"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_chain_001": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_chain_001",
				UserLevel:   3,
				VIPLevel:    1,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"subjectPath":"subject.id",
			"failureAction":"BLOCK",
			"providerChain":[
				{"source":"LOCAL_CACHE","cachePath":"$.vars.execution.facts.USER_PROFILE","onMiss":"NEXT"},
				{"source":"FACT_SERVICE","resourceRef":"default_user_profile_store","namespace":"facts","table":"marketing_user_profile","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","onMiss":"BLOCK"}
			]
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected structured provider chain success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["exists"] != true || profile["level"] != 3 || profile["vipLevel"] != 1 {
		t.Fatalf("expected user profile facts from provider chain resource step, got %+v", profile)
	}
	chain := result.Output["providerChain"].([]any)
	if len(chain) != 2 {
		t.Fatalf("expected two provider chain traces, got %+v", result.Output["providerChain"])
	}
	second := chain[1].(map[string]any)
	if second["resourceRef"] != "default_user_profile_store" || second["source"] != "FACT_SERVICE" {
		t.Fatalf("expected resource step trace, got %+v", second)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainTakesPrecedenceOverTopLevelResourceRef(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_chain_precedence"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_chain_precedence": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_chain_precedence",
				UserLevel:   4,
				VIPLevel:    2,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"resourceRef":"default_user_profile_store",
			"factKeys":["USER_PROFILE"],
			"providerChain":[
				{"source":"LOCAL_CACHE","cachePath":"$.vars.execution.facts.USER_PROFILE","onMiss":"NEXT"},
				{"source":"FACT_SERVICE","resourceRef":"default_user_profile_store","namespace":"facts","table":"marketing_user_profile","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected provider chain success, got %+v", result)
	}
	chain, ok := result.Output["providerChain"].([]any)
	if !ok || len(chain) != 2 {
		t.Fatalf("expected provider chain trace instead of single resource path, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainUsesServiceInterfaceFallbackPolicy(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_service_interface"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_service_interface": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_service_interface",
				UserLevel:   5,
				VIPLevel:    3,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY_WITH_FALLBACK"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service interface binding: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"subjectPath":"subject.id",
			"providerChain":[
				{"targetType":"SERVICE_INTERFACE","source":"HTTP","serviceRef":"user-profile-service","interfaceRef":"query-user-profile","providerPolicy":"PRIMARY_WITH_FALLBACK","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected service interface provider chain success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != 3 || profile["level"] != 5 {
		t.Fatalf("expected user profile facts from service interface, got %+v", profile)
	}
	chain := result.Output["providerChain"].([]any)
	step := chain[0].(map[string]any)
	if step["source"] != "SERVICE_INTERFACE" || step["providerPolicy"] != "PRIMARY_WITH_FALLBACK" || step["providerRef"] == "" {
		t.Fatalf("expected service interface trace with selected provider, got %+v", step)
	}
}

func TestFactQueryWorkflowNodeHandlerServiceInterfaceFallbackAttemptsNextProvider(t *testing.T) {
	ctx := context.Background()
	baseRepo := marketing.NewMemoryRepository()
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Inbox.Event.Subject = marketing.EventSubject{Type: "USER", ID: "u_service_fallback"}
	repo := workflowUserProfileTestRepository{
		Repository: baseRepo,
		profiles: map[string]marketing.UserProfile{
			runtime.Activity.TenantID + "|USER|u_service_fallback": {
				TenantID:    runtime.Activity.TenantID,
				SubjectType: "USER",
				SubjectID:   "u_service_fallback",
				UserLevel:   6,
				VIPLevel:    4,
				Status:      "ACTIVE",
			},
		},
	}
	service := NewService(repo)
	if _, err := service.SaveServiceDefinition(ctx, marketing.ServiceDefinition{
		TenantID:    runtime.Activity.TenantID,
		Code:        "custom-profile-service",
		Name:        "Custom Profile Service",
		ServiceType: marketing.ServiceTypeInternal,
		Scope:       marketing.ServiceAssetScopeTenant,
		Status:      marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if _, err := service.SaveServiceInterface(ctx, marketing.ServiceInterface{
		TenantID:   runtime.Activity.TenantID,
		ServiceRef: "custom-profile-service",
		Code:       "query-profile",
		Name:       "Query profile",
		Operation:  marketing.ServiceOperationCall,
		TimeoutMS:  1000,
		Status:     marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save interface: %v", err)
	}
	for _, provider := range []marketing.ServiceProvider{
		{
			TenantID:     runtime.Activity.TenantID,
			ServiceRef:   "custom-profile-service",
			InterfaceRef: "query-profile",
			Code:         "missing-primary",
			Name:         "Missing primary",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"missing_profile_store"},
			Priority:     0,
			Weight:       100,
			Status:       marketing.ServiceAssetActive,
		},
		{
			TenantID:     runtime.Activity.TenantID,
			ServiceRef:   "custom-profile-service",
			InterfaceRef: "query-profile",
			Code:         "resource-fallback",
			Name:         "Resource fallback",
			Protocol:     marketing.ServiceProviderResource,
			ResourceRefs: []string{"default_user_profile_store"},
			Priority:     10,
			Weight:       1,
			Status:       marketing.ServiceAssetActive,
		},
	} {
		if _, err := service.SaveServiceProvider(ctx, provider); err != nil {
			t.Fatalf("save provider %s: %v", provider.Code, err)
		}
	}
	if _, err := service.SaveServiceInterfaceBinding(ctx, marketing.ServiceInterfaceBinding{
		TenantID:                runtime.Activity.TenantID,
		ServiceRef:              "custom-profile-service",
		InterfaceRef:            "query-profile",
		Scope:                   marketing.ServiceAssetScopeActivity,
		ActivityID:              runtime.Activity.ID,
		PrincipalType:           marketing.ServicePrincipalRole,
		PrincipalID:             "*",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []marketing.ServiceInterfaceOperation{marketing.ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY_WITH_FALLBACK"},
		Status:                  marketing.ServiceAssetActive,
	}); err != nil {
		t.Fatalf("save service interface binding: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"subjectPath":"subject.id",
			"providerChain":[
				{"targetType":"SERVICE_INTERFACE","serviceRef":"custom-profile-service","interfaceRef":"query-profile","providerPolicy":"PRIMARY_WITH_FALLBACK","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected fallback provider chain success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != 4 || profile["level"] != 6 {
		t.Fatalf("expected user profile facts from fallback provider, got %+v", profile)
	}
	chain := result.Output["providerChain"].([]any)
	if len(chain) != 2 {
		t.Fatalf("expected primary failure and fallback traces, got %+v", chain)
	}
	first := chain[0].(map[string]any)
	second := chain[1].(map[string]any)
	if first["providerRef"] != "missing-primary" || first["status"] != "FAILED_NEXT" {
		t.Fatalf("expected failed primary trace, got %+v", first)
	}
	if second["providerRef"] != "resource-fallback" || second["status"] != "DONE" {
		t.Fatalf("expected successful fallback trace, got %+v", second)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainBlocksOnLocalCacheMiss(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"providerChain":[
				{"source":"LOCAL_CACHE","cachePath":"$.vars.execution.facts.USER_PROFILE","onMiss":"BLOCK"},
				{"source":"FACT_SERVICE","resourceRef":"default_user_profile_store","namespace":"facts","table":"marketing_user_profile","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected local cache miss to block provider chain, got %+v", result)
	}
	if result.Output["errorCode"] != "FACT_QUERY_PROVIDER_MISS" {
		t.Fatalf("expected FACT_QUERY_PROVIDER_MISS, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainBlocksOnPartialLocalCacheMiss(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	vars := map[string]any{}
	workflowSetVar(vars, "execution", "facts", map[string]any{"USER_PROFILE": map[string]any{"exists": true, "vipLevel": 1}})
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE","DEPOSIT_SUMMARY"],
			"providerChain":[
				{"source":"LOCAL_CACHE","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{Variables: vars})
	result := service.factQueryWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected partial local cache miss to block provider chain, got %+v", result)
	}
	if result.Output["errorCode"] != "FACT_QUERY_PROVIDER_MISS" {
		t.Fatalf("expected FACT_QUERY_PROVIDER_MISS, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainFailsWhenFactsRemainUnresolved(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	vars := map[string]any{}
	workflowSetVar(vars, "execution", "facts", map[string]any{"USER_PROFILE": map[string]any{"exists": true, "vipLevel": 1}})
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE","DEPOSIT_SUMMARY"],
			"providerChain":[
				{"source":"LOCAL_CACHE","onMiss":"NEXT"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{Variables: vars})
	result := service.factQueryWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected unresolved remaining facts to fail provider chain, got %+v", result)
	}
	if result.Output["errorCode"] != "FACT_QUERY_NO_PROVIDER_HIT" {
		t.Fatalf("expected FACT_QUERY_NO_PROVIDER_HIT, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainDoesNotSwallowResourceErrors(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"providerChain":[
				{"source":"REDIS","resourceRef":"missing_resource","namespace":"facts","keyTemplate":"${subject.id}:${factCode}","onMiss":"NEXT"},
				{"source":"FACT_SERVICE","resourceRef":"default_user_profile_store","namespace":"facts","table":"marketing_user_profile","keyTemplate":"${tenantId}:${subject.type}:${subject.id}","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed {
		t.Fatalf("expected missing resource to fail even with onMiss NEXT, got %+v", result)
	}
	if result.Output["errorCode"] != "RESOURCE_NOT_FOUND" {
		t.Fatalf("expected RESOURCE_NOT_FOUND, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerProviderChainUsesRedisResourceRefStep(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "user_fact_cache_chain",
		Name:        "用户事实缓存",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"fact:user"}`),
	}); err != nil {
		t.Fatalf("save fact cache resource: %v", err)
	}
	storeNode := marketing.WorkflowNode{
		ID:     "store-user-profile",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"user_fact_cache_chain","operation":"UPSERT","namespace":"facts","keyTemplate":"${subject.id}:USER_PROFILE","valueType":"OBJECT"}`),
	}
	storeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"vipLevel": "VIP3", "country": "BR"}},
	})
	if stored := service.stateStoreWorkflowNodeHandler(storeCtx, storeNode, runtime); stored.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected fact cache seed success, got %+v", stored)
	}

	node := marketing.WorkflowNode{
		ID:   "fact-query",
		Type: "fact-query",
		Config: json.RawMessage(`{
			"factKeys":["USER_PROFILE"],
			"providerChain":[
				{"source":"REDIS","resourceRef":"user_fact_cache_chain","namespace":"facts","keyTemplate":"${subject.id}:${factCode}","onMiss":"BLOCK"}
			],
			"failureAction":"BLOCK"
		}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected redis resource provider chain success, got %+v", result)
	}
	profile := result.Output["facts"].(map[string]any)["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != "VIP3" {
		t.Fatalf("expected redis profile fact, got %+v", result.Output)
	}
}

func TestFactQueryWorkflowNodeHandlerCallsHTTPProviderResourceRef(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	var received map[string]any
	var receivedMethod string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":{"facts":{"USER_PROFILE":{"vipLevel":"VIP2","channel":"APP"}}},"version":3,"exists":true}`))
	}))
	defer provider.Close()
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "profile_http_provider",
		Name:        "用户画像 HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"` + provider.URL + `","method":"POST","timeoutSeconds":3}`),
	}); err != nil {
		t.Fatalf("save http fact provider resource: %v", err)
	}

	node := marketing.WorkflowNode{
		ID:     "fact-query-http",
		Type:   "fact-query",
		Config: json.RawMessage(`{"resourceRef":"profile_http_provider","operation":"CALL","factKeys":["USER_PROFILE"],"namespace":"facts","keyTemplate":"${tenantId}:${subject.id}:${factCode}","failureAction":"BLOCK"}`),
	}
	result := service.factQueryWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected http resource fact query success, got %+v", result)
	}
	if receivedMethod != http.MethodPost || received["operation"] != "CALL" || received["key"] != runtime.Activity.TenantID+":"+runtime.Inbox.Event.Subject.ID+":USER_PROFILE" {
		t.Fatalf("expected normalized provider request, method=%s body=%+v", receivedMethod, received)
	}
	facts := result.Output["facts"].(map[string]any)
	profile := facts["USER_PROFILE"].(map[string]any)
	if profile["vipLevel"] != "VIP2" {
		t.Fatalf("expected http profile fact, got %+v", result.Output)
	}
}

func TestStateStoreWorkflowNodeHandlerUsesRedisCounterResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "redis_checkin_counter",
		Name:        "Redis 签到计数器",
		Type:        marketing.ResourceRedisCounter,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"marketing:counter"}`),
	}); err != nil {
		t.Fatalf("save redis counter resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "redis-counter",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"redis_checkin_counter","operation":"CHECK_AND_INCREMENT","namespace":"checkin","keyTemplate":"${subject.id}","delta":2,"max":3,"valueType":"NUMBER"}`),
	}
	first := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess || first.Output["counter"] != float64(2) {
		t.Fatalf("expected counter increment, got %+v", first)
	}
	second := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if second.Status != marketing.WorkflowNodeFailed || second.Output["errorCode"] != "REDIS_COUNTER_LIMIT_EXCEEDED" {
		t.Fatalf("expected counter limit failure, got %+v", second)
	}
}

func TestQuotaCheckWorkflowNodeHandlerUsesRedisCounterResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "quota_counter",
		Name:        "次数频控计数器",
		Type:        marketing.ResourceRedisCounter,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"quota:counter"}`),
	}); err != nil {
		t.Fatalf("save quota counter resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "quota-check",
		Type:   "quota-check",
		Config: json.RawMessage(`{"resourceRef":"quota_counter","namespace":"checkin","scope":"USER","scopeKeyTemplate":"${subject.id}:${activityId}","maxCount":1,"consumeMode":"ATOMIC","exceedAction":"BLOCK"}`),
	}
	first := service.quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected first quota check success, got %+v", first)
	}
	quotaCheck := first.Output["quota_check"].(map[string]any)
	if quotaCheck["allowed"] != true || quotaCheck["counter"] != float64(1) {
		t.Fatalf("expected quota counter allowed, got %+v", quotaCheck)
	}
	if quotaCheck["scopeKey"] != "user_001:activity_handler" || quotaCheck["logicalKey"] != "user_001:activity_handler:20260605" {
		t.Fatalf("expected day-window logical key, got %+v", quotaCheck)
	}
	windowSegment := quotaCheck["windowSegment"].(map[string]any)
	if windowSegment["segmentKey"] != "20260605" || windowSegment["source"] != "BUILTIN" {
		t.Fatalf("expected builtin day window segment, got %+v", windowSegment)
	}
	second := service.quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	if second.Status != marketing.WorkflowNodeFailed || second.Output["errorCode"] != "REDIS_COUNTER_LIMIT_EXCEEDED" {
		t.Fatalf("expected quota counter limit failure, got %+v", second)
	}
}

func TestQuotaCheckWorkflowNodeHandlerUsesProviderChainResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:   "quota-check",
		Type: "quota-check",
		Config: json.RawMessage(`{
			"providerChain":[
				{"source":"RESOURCE","resourceRef":"default_checkin_quota_counter","namespace":"checkin","operation":"READ","keyTemplate":"${subject.id}:${activityId}","valuePath":"$.counter","outputAs":"currentQuota","onMiss":"BLOCK"}
			],
			"scope":"USER",
			"window":"DAY",
			"maxCount":1,
			"consumeMode":"CHECK_ONLY",
			"exceedAction":"BLOCK"
		}`),
	}
	result := service.quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected provider-chain quota check success, got %+v", result)
	}
	quotaCheck := result.Output["quota_check"].(map[string]any)
	if quotaCheck["resourceRef"] != "default_checkin_quota_counter" || quotaCheck["logicalKey"] != "user_001:activity_handler:20260605" {
		t.Fatalf("expected provider-chain quota resource and key, got %+v", quotaCheck)
	}
	resourceResult := result.Output["resource_result"].(map[string]any)
	if resourceResult["redisNamespace"] != "redis:default_checkin_quota_counter:checkin" {
		t.Fatalf("expected checkin redis namespace, got %+v", resourceResult)
	}
}

func TestUserLevelCheckWorkflowNodeHandlerUsesUserProfileFacts(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"facts": map[string]any{
				"USER_PROFILE": map[string]any{"exists": true, "vipLevel": 2},
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:     "user-level",
		Type:   "user-level-check",
		Config: json.RawMessage(`{"requireExistingUser":true,"minLevel":1,"levelPaths":["$.facts.USER_PROFILE.vipLevel"],"existsPath":"exists","failDecision":"BLOCK"}`),
	}
	result := userLevelCheckWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["decision"] != "PASS" || result.Output["PASS"] != true {
		t.Fatalf("expected user level pass, got %+v", result)
	}
	check := result.Output["user_level_check"].(map[string]any)
	if check["level"] != float64(2) || check["levelPath"] != "$.facts.USER_PROFILE.vipLevel" {
		t.Fatalf("expected vip level from facts, got %+v", check)
	}

	node.Config = json.RawMessage(`{"requireExistingUser":true,"minLevel":3,"levelPaths":["$.facts.USER_PROFILE.vipLevel"],"existsPath":"exists","failDecision":"BLOCK"}`)
	blocked := userLevelCheckWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if blocked.Status != marketing.WorkflowNodeSuccess || blocked.Output["decision"] != "BLOCK" || blocked.Output["BLOCK"] != true || blocked.Output["reasonCode"] != "USER_LEVEL_TOO_LOW" {
		t.Fatalf("expected user level block, got %+v", blocked)
	}
}

func TestUserLevelCheckWorkflowNodeHandlerUsesStandardContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{
			"standard_context": map[string]any{
				"user": map[string]any{"exists": true, "level": 4},
			},
		},
	})
	node := marketing.WorkflowNode{
		ID:     "user-level",
		Type:   "user-level-check",
		Config: json.RawMessage(`{"requireExistingUser":true,"minLevel":2,"userPaths":["$.standard_context.user"],"levelPaths":["$.standard_context.user.level"],"existsPath":"exists","failDecision":"BLOCK"}`),
	}
	result := userLevelCheckWorkflowNodeHandler(ctx, node, testWorkflowHandlerRuntime(true))
	if result.Status != marketing.WorkflowNodeSuccess || result.Output["decision"] != "PASS" || result.Output["PASS"] != true {
		t.Fatalf("expected user level pass from standard context, got %+v", result)
	}
	check := result.Output["user_level_check"].(map[string]any)
	if check["level"] != float64(4) || check["levelPath"] != "$.standard_context.user.level" {
		t.Fatalf("expected level from standard context, got %+v", check)
	}
}

func TestQuotaCheckWorkflowNodeHandlerSupportsCustomLuaWindow(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "quota_counter",
		Name:        "次数频控计数器",
		Type:        marketing.ResourceRedisCounter,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"quota:counter"}`),
	}); err != nil {
		t.Fatalf("save quota counter resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "quota-check",
		Type: "quota-check",
		Config: json.RawMessage(`{
			"resourceRef":"quota_counter",
			"namespace":"checkin",
			"scope":"USER",
			"scopeKeyTemplate":"${subject.id}:${activityId}",
			"window":"CUSTOM_RANGE",
			"windowLuaSource":"emit({segmentKey = 'promo-june', windowStart = '2026-06-01T00:00:00Z', windowEnd = '2026-06-30T23:59:59Z'})",
			"maxCount":1,
			"consumeMode":"ATOMIC",
			"exceedAction":"BLOCK"
		}`),
	}
	first := service.quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected first quota check success, got %+v", first)
	}
	quotaCheck := first.Output["quota_check"].(map[string]any)
	if quotaCheck["logicalKey"] != "user_001:activity_handler:promo-june" || quotaCheck["window"] != "CUSTOM_RANGE" {
		t.Fatalf("expected custom-window logical key, got %+v", quotaCheck)
	}
	windowSegment := quotaCheck["windowSegment"].(map[string]any)
	if windowSegment["segmentKey"] != "promo-june" || windowSegment["source"] != "LUA" {
		t.Fatalf("expected lua custom window segment, got %+v", windowSegment)
	}
	if windowSegment["start"] != "2026-06-01T00:00:00Z" || windowSegment["end"] != "2026-06-30T23:59:59Z" {
		t.Fatalf("expected lua custom window bounds, got %+v", windowSegment)
	}
	second := service.quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	if second.Status != marketing.WorkflowNodeFailed || second.Output["errorCode"] != "REDIS_COUNTER_LIMIT_EXCEEDED" {
		t.Fatalf("expected custom-window quota limit failure, got %+v", second)
	}
}

func TestWorkflowFieldBindingsResolveConfigFromContextAndResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "activity_config_kv",
		Name:        "活动配置 KV",
		Type:        marketing.ResourceDatabaseKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"activity-config"}`),
	}); err != nil {
		t.Fatalf("save activity config resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "quota-check",
		Type: "quota-check",
		Config: json.RawMessage(`{
			"resourceBindings":{"activityConfig":"activity_config_kv"},
			"scope":"USER",
			"scopeKeyTemplate":"${subject.id}:${activityId}",
			"window":"DAY",
			"maxCount":1,
			"fieldBindings":{
				"maxCount":{"mode":"EXPRESSION","language":"LUA","valueType":"NUMBER","source":"Resource.activityConfig.get('daily_checkin_max_count', {namespace = 'config'})"},
				"scopeKeyTemplate":{"mode":"EXPRESSION","language":"LUA","source":"Context.get('event.subject.id') .. ':' .. Context.get('activityId')"}
			}
		}`),
	}
	execCtx := workflowNodeExecutionContext{
		ResolvedInput:   map[string]any{"eventId": runtime.Inbox.Event.EventID, "activityId": runtime.Activity.ID},
		NodeOutputs:     map[string]map[string]any{},
		NodeDefinitions: map[string]marketing.WorkflowNode{node.ID: node},
		Variables:       map[string]any{},
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, execCtx)
	if _, err := service.workflowExecuteResourceAPI(nodeCtx, node, runtime, workflowResourceAPIRequest{
		Operation:   marketing.ResourceUpsert,
		ResourceRef: "activity_config_kv",
		Namespace:   "config",
		Key:         "daily_checkin_max_count",
		Value:       3,
		ValueType:   "NUMBER",
	}); err != nil {
		t.Fatalf("seed resource config: %v", err)
	}
	resolvedNode, resolvedConfig, trace, err := service.resolveWorkflowNodeConfig(nodeCtx, node, runtime)
	if err != nil {
		t.Fatalf("resolve field bindings: %v", err)
	}
	if resolvedConfig["maxCount"] != float64(3) || resolvedConfig["scopeKeyTemplate"] != "user_001:activity_handler" {
		t.Fatalf("unexpected resolved config: %+v", resolvedConfig)
	}
	if len(trace) != 2 {
		t.Fatalf("expected two field binding trace items, got %+v", trace)
	}
	if _, ok := workflowNodeConfigMap(resolvedNode)["fieldBindings"]; ok {
		t.Fatalf("resolved node config must not expose fieldBindings to handler: %+v", workflowNodeConfigMap(resolvedNode))
	}
}

func TestWorkflowConfigScriptAssignsSchemaFields(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	node := marketing.WorkflowNode{
		ID:   "quota-check",
		Type: "quota-check",
		Config: json.RawMessage(`{
			"scope":"USER",
			"scopeKeyTemplate":"${subject.id}:${activityId}",
			"window":"DAY",
			"maxCount":1,
			"configMode":"LUA",
			"configScript":"local userId = Context.get('event.subject.id')\nscope = 'USER'\nscopeKeyTemplate = userId .. ':' .. Context.get('activityId')\nwindow = 'DAY'\nmaxCount = 5\nconsumeMode = 'ATOMIC'\nexceedAction = 'BLOCK'\nlocalOnly = 'ignored'"
		}`),
	}
	execCtx := workflowNodeExecutionContext{
		ResolvedInput:   map[string]any{"eventId": runtime.Inbox.Event.EventID, "activityId": runtime.Activity.ID},
		NodeOutputs:     map[string]map[string]any{},
		NodeDefinitions: map[string]marketing.WorkflowNode{node.ID: node},
		Variables:       map[string]any{},
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, execCtx)
	resolvedNode, resolvedConfig, trace, err := service.resolveWorkflowNodeConfig(nodeCtx, node, runtime)
	if err != nil {
		t.Fatalf("resolve config script: %v", err)
	}
	if resolvedConfig["maxCount"] != float64(5) || resolvedConfig["scopeKeyTemplate"] != "user_001:activity_handler" || resolvedConfig["consumeMode"] != "ATOMIC" {
		t.Fatalf("unexpected config script result: %+v", resolvedConfig)
	}
	if _, ok := workflowNodeConfigMap(resolvedNode)["configScript"]; ok {
		t.Fatalf("resolved node config must not expose configScript to handler: %+v", workflowNodeConfigMap(resolvedNode))
	}
	if len(trace) != 1 || trace[0]["field"] != "configScript" || trace[0]["status"] != "SUCCESS" {
		t.Fatalf("expected config script trace, got %+v", trace)
	}
	assigned := trace[0]["assignedFields"].([]string)
	if !slices.Contains(assigned, "maxCount") || !slices.Contains(assigned, "scopeKeyTemplate") {
		t.Fatalf("expected assigned schema fields, got %+v", trace[0])
	}
}

func TestLockWorkflowNodeHandlerUsesRedisLockResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "user_lock",
		Name:        "用户互斥锁",
		Type:        marketing.ResourceRedisLock,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"lock:user","ttlSeconds":30}`),
	}); err != nil {
		t.Fatalf("save lock resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "lock",
		Type:   "lock",
		Config: json.RawMessage(`{"resourceRef":"user_lock","namespace":"checkin","keyTemplate":"${subject.id}","operation":"ACQUIRE","owner":"test-owner","ttlSeconds":30}`),
	}
	first := service.lockWorkflowNodeHandler(ctx, node, runtime)
	if first.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected first lock acquire success, got %+v", first)
	}
	second := service.lockWorkflowNodeHandler(ctx, node, runtime)
	if second.Status != marketing.WorkflowNodeFailed || second.Output["errorCode"] != "REDIS_LOCK_HELD" {
		t.Fatalf("expected held lock failure, got %+v", second)
	}
	releaseNode := node
	releaseNode.Config = json.RawMessage(`{"resourceRef":"user_lock","namespace":"checkin","keyTemplate":"${subject.id}","operation":"RELEASE"}`)
	released := service.lockWorkflowNodeHandler(ctx, releaseNode, runtime)
	if released.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected lock release success, got %+v", released)
	}
	reacquired := service.lockWorkflowNodeHandler(ctx, node, runtime)
	if reacquired.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected lock reacquire success after release, got %+v", reacquired)
	}
}

func TestStateStoreWorkflowNodeHandlerRequiresApprovalForHighRiskResourceOperation(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "approval_redis_state",
		Name:        "审批 Redis 状态",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"approval:state","requireApproval":true}`),
	}); err != nil {
		t.Fatalf("save redis resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "approval-delete",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"approval_redis_state","operation":"DELETE","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT"}`),
	}
	blocked := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if blocked.Status != marketing.WorkflowNodeFailed || blocked.Output["errorCode"] != "RESOURCE_APPROVAL_REQUIRED" {
		t.Fatalf("expected approval failure, got %+v", blocked)
	}

	approvedNode := node
	approvedNode.Config = json.RawMessage(`{"resourceRef":"approval_redis_state","operation":"DELETE","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT","approvalId":"approval_001"}`)
	approved := service.stateStoreWorkflowNodeHandler(ctx, approvedNode, runtime)
	if approved.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected approved delete success, got %+v", approved)
	}
}

func TestStateStoreWorkflowNodeHandlerRequiresFormalActivityApprovalForHighRiskResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	runtime.Activity.Approval = marketing.ApprovalSnapshot{Status: marketing.ApprovalNone}
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "formal_approval_state",
		Name:        "正式审批资源",
		Type:        marketing.ResourceDatabaseKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"requireApproval":true,"requireFormalApproval":true}`),
	}); err != nil {
		t.Fatalf("save resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "formal-approval-delete",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"formal_approval_state","operation":"DELETE","namespace":"checkin","keyTemplate":"${subject.id}","valueType":"OBJECT","approvalId":"approval_001"}`),
	}
	blocked := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if blocked.Status != marketing.WorkflowNodeFailed || blocked.Output["errorCode"] != "RESOURCE_APPROVAL_REQUIRED" {
		t.Fatalf("expected formal approval failure, got %+v", blocked)
	}

	runtime.Activity.Approval = marketing.ApprovalSnapshot{Status: marketing.ApprovalApproved, ActivityVersion: runtime.Activity.Version, Submitter: "ops", Approver: "lead"}
	approved := service.stateStoreWorkflowNodeHandler(ctx, node, runtime)
	if approved.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected formally approved operation success, got %+v", approved)
	}
}

func TestMQPublishWorkflowNodeHandlerUsesTopicResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "reward_commands_topic",
		Name:        "奖励命令 Topic",
		Type:        marketing.ResourceMQTopic,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"provider":"KAFKA","topic":"reward.commands"}`),
	}); err != nil {
		t.Fatalf("save mq topic resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "mq-publish",
		Type:   "mq-publish",
		Config: json.RawMessage(`{"resourceRef":"reward_commands_topic","namespace":"reward","keyTemplate":"${eventId}"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"payload": map[string]any{"commandId": "cmd_001"}},
	})
	result := service.mqPublishWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected mq publish success, got %+v", result)
	}
	mqResult := result.Output["mq_result"].(map[string]any)
	if mqResult["destination"] != "reward.commands" || mqResult["status"] != "PUBLISH_ACCEPTED" {
		t.Fatalf("expected mq publish result, got %+v", mqResult)
	}
}

func TestOutboxWorkflowNodeHandlerBindsHTTPProviderResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "notification_http_provider",
		Name:        "通知 HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"https://notify.example/internal/outbox","method":"POST"}`),
	}); err != nil {
		t.Fatalf("save outbox http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "outbox-resource",
		Type:   "outbox",
		Config: json.RawMessage(`{"resourceRef":"notification_http_provider","channel":"HTTP","namespace":"outbox","keyTemplate":"${eventId}"}`),
	}
	result := service.outboxWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected resource outbox success, got %+v", result)
	}
	if result.Output["resourceRef"] != "notification_http_provider" || result.Output["resourceMessageCount"] != 1 {
		t.Fatalf("expected resource-bound outbox output, got %+v", result.Output)
	}
	plan := result.Output["resourceDispatchPlan"].(map[string]any)
	if plan["targetRef"] != "https://notify.example/internal/outbox" || plan["executionAuthority"] != "OutboxWorker" {
		t.Fatalf("expected outbox resource dispatch plan, got %+v", plan)
	}
	logs, err := repo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{})
	if err != nil {
		t.Fatalf("list resource audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].NodeType != "outbox" || logs[0].Operation != marketing.ResourceCall {
		t.Fatalf("expected outbox resource audit, got %+v", logs)
	}
}

func TestOutboxWorkflowNodeHandlerRequiresApprovalForHighRiskResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "approval_notification_http_provider",
		Name:        "审批通知 HTTP Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"https://notify.example/internal/outbox","method":"POST","requireApproval":true}`),
	}); err != nil {
		t.Fatalf("save approval outbox provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "outbox-approval-resource",
		Type:   "outbox",
		Config: json.RawMessage(`{"resourceRef":"approval_notification_http_provider","channel":"HTTP","namespace":"outbox","keyTemplate":"${eventId}"}`),
	}
	blocked := service.outboxWorkflowNodeHandler(ctx, node, runtime)
	if blocked.Status != marketing.WorkflowNodeFailed || blocked.Output["errorCode"] != "RESOURCE_APPROVAL_REQUIRED" {
		t.Fatalf("expected outbox approval failure, got %+v", blocked)
	}

	approvedNode := node
	approvedNode.Config = json.RawMessage(`{"resourceRef":"approval_notification_http_provider","channel":"HTTP","namespace":"outbox","keyTemplate":"${eventId}","approvalId":"approval_001"}`)
	approved := service.outboxWorkflowNodeHandler(ctx, approvedNode, runtime)
	if approved.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected approved outbox resource success, got %+v", approved)
	}
}

func TestCallbackWorkflowNodeHandlerBindsHTTPProviderResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "wallet_callback_provider",
		Name:        "钱包回调 Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"https://wallet.example/internal/callback","method":"POST"}`),
	}); err != nil {
		t.Fatalf("save callback http provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "callback-resource",
		Type:   "callback",
		Config: json.RawMessage(`{"provider":"wallet","resourceRef":"wallet_callback_provider","requireSignature":true,"namespace":"callback","keyTemplate":"${eventId}"}`),
	}
	result := service.callbackWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected resource callback success, got %+v", result)
	}
	if result.Output["resourceRef"] != "wallet_callback_provider" || result.Output["resourceized"] != true {
		t.Fatalf("expected resource-bound callback output, got %+v", result.Output)
	}
	plan := result.Output["callbackResourcePlan"].(map[string]any)
	if plan["targetRef"] != "https://wallet.example/internal/callback" || plan["executionAuthority"] != "RewardCallback/Reconciliation" {
		t.Fatalf("expected callback resource plan, got %+v", plan)
	}
}

func TestCallbackWorkflowNodeHandlerRequiresApprovalForHighRiskResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "approval_wallet_callback_provider",
		Name:        "审批钱包回调 Provider",
		Type:        marketing.ResourceHTTPProvider,
		Driver:      marketing.ResourceDriverREST,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"endpoint":"https://wallet.example/internal/callback","method":"POST","requireApproval":true}`),
	}); err != nil {
		t.Fatalf("save approval callback provider resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "callback-approval-resource",
		Type:   "callback",
		Config: json.RawMessage(`{"provider":"wallet","resourceRef":"approval_wallet_callback_provider","requireSignature":true,"namespace":"callback","keyTemplate":"${eventId}"}`),
	}
	blocked := service.callbackWorkflowNodeHandler(ctx, node, runtime)
	if blocked.Status != marketing.WorkflowNodeFailed || blocked.Output["errorCode"] != "RESOURCE_APPROVAL_REQUIRED" {
		t.Fatalf("expected callback approval failure, got %+v", blocked)
	}

	approvedNode := node
	approvedNode.Config = json.RawMessage(`{"provider":"wallet","resourceRef":"approval_wallet_callback_provider","requireSignature":true,"namespace":"callback","keyTemplate":"${eventId}","approvalId":"approval_001"}`)
	approved := service.callbackWorkflowNodeHandler(ctx, approvedNode, runtime)
	if approved.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected approved callback resource success, got %+v", approved)
	}
}

func TestLuaResourceAPIUpsertsAndReadsResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "lua_redis_state",
		Name:        "Lua Redis 状态",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"lua:state"}`),
	}); err != nil {
		t.Fatalf("save lua redis resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "lua-code",
		Type: "code",
		Config: json.RawMessage(`{
			"language":"LUA",
			"source":"resource_upsert('lua_redis_state', 'subject:' .. ctx.event.subject.id, {streakDays = 5}, {namespace = 'lua', valueType = 'OBJECT'}); local stored = resource_read('lua_redis_state', 'subject:' .. ctx.event.subject.id, {namespace = 'lua', valueType = 'OBJECT'}); emit({stored = stored.stored_value})"
		}`),
	}
	result := service.codeWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected lua resource api success, got %+v", result)
	}
	codeResult := result.Output["code_result"].(map[string]any)
	stored := codeResult["stored"].(map[string]any)
	if stored["streakDays"] != float64(5) {
		t.Fatalf("expected lua stored value, got %+v", stored)
	}
}

func TestWorkflowResourceAPICallsLuaResource(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "lua_transform_resource",
		Name:        "Lua Transform Resource",
		Type:        marketing.ResourceLua,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"source":"local req = input('resource'); emit({value = {resourceRef = req.ref, key = req.key, amount = req.value.amount}, facts = {LUA_RESULT = {ok = true}}})"}`),
	}); err != nil {
		t.Fatalf("save lua resource: %v", err)
	}
	node := marketing.WorkflowNode{ID: "lua-resource-call", Type: "code", Config: json.RawMessage(`{"language":"LUA","source":"emit({ok=true})"}`)}
	output, err := service.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
		Operation:   marketing.ResourceCall,
		ResourceRef: "lua_transform_resource",
		Namespace:   "lua",
		Key:         "subject:user_001",
		Value:       map[string]any{"amount": 12},
		ValueType:   "OBJECT",
	})
	if err != nil {
		t.Fatalf("expected lua resource call success: %v output=%+v", err, output)
	}
	value, ok := output["value"].(map[string]any)
	if !ok || value["resourceRef"] != "lua_transform_resource" || value["key"] != "subject:user_001" || value["amount"] != float64(12) {
		t.Fatalf("unexpected lua resource value: %+v", output)
	}
	if output["resourceAdapterMode"] != "LUA" || output["executionAuthority"] != "WorkflowExecutor/LuaResource" {
		t.Fatalf("expected lua resource metadata, got %+v", output)
	}
}

func TestLuaResourceAPIEnforcesResourceCallLimit(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResource(ctx, marketing.Resource{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		Code:        "lua_limited_state",
		Name:        "Lua 限额状态",
		Type:        marketing.ResourceRedisKV,
		Driver:      marketing.ResourceDriverInternal,
		Scope:       marketing.ResourceScopeWorkspace,
		Status:      marketing.ResourceActive,
		Config:      json.RawMessage(`{"keyPrefix":"lua:limited"}`),
	}); err != nil {
		t.Fatalf("save lua redis resource: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:   "lua-limited-code",
		Type: "code",
		Config: json.RawMessage(`{
			"language":"LUA",
			"maxResourceCalls":1,
			"source":"resource_upsert('lua_limited_state', 'subject:' .. ctx.event.subject.id, {streakDays = 5}, {namespace = 'lua', valueType = 'OBJECT'}); resource_read('lua_limited_state', 'subject:' .. ctx.event.subject.id, {namespace = 'lua', valueType = 'OBJECT'}); emit({ok = true})"
		}`),
	}
	result := service.codeWorkflowNodeHandler(ctx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || !strings.Contains(result.Error, "workflow resource api call limit exceeded") {
		t.Fatalf("expected lua resource call limit failure, got %+v", result)
	}
}

func TestStateStoreWorkflowNodeHandlerDeniesResourceKeyPolicy(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	runtime := testWorkflowHandlerRuntime(true)
	if _, err := service.SaveResourceBinding(ctx, marketing.ResourceBinding{
		ResourceID:        "res_builtin_default_workflow_state",
		TenantID:          runtime.Activity.TenantID,
		WorkspaceID:       runtime.Activity.WorkspaceID,
		NodeType:          "state-store",
		AllowedOperations: []marketing.ResourceOperation{marketing.ResourceUpsert},
		AllowedNamespaces: []string{"checkin"},
		KeyPolicy:         json.RawMessage(`{"allowedPrefixes":["allowed:"]}`),
	}); err != nil {
		t.Fatalf("save resource binding: %v", err)
	}
	node := marketing.WorkflowNode{
		ID:     "resource-store-denied",
		Type:   "state-store",
		Config: json.RawMessage(`{"resourceRef":"default_workflow_state","operation":"UPSERT","namespace":"checkin","keyTemplate":"denied:${subject.id}","valueType":"OBJECT"}`),
	}
	nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
		ResolvedInput: map[string]any{"value": map[string]any{"streakDays": 3}},
	})

	result := service.stateStoreWorkflowNodeHandler(nodeCtx, node, runtime)
	if result.Status != marketing.WorkflowNodeFailed || result.Output["errorCode"] != "RESOURCE_PERMISSION_DENIED" {
		t.Fatalf("expected resource permission denied, got %+v", result)
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
			ID:            "activity_handler",
			TenantID:      "site_001",
			Code:          "HANDLER",
			Name:          "Handler Activity",
			Status:        marketing.ActivityActive,
			TriggerEvents: []string{"DEPOSIT_SUCCESS"},
			StartTime:     now.Add(-time.Hour),
			EndTime:       now.Add(time.Hour),
			Version:       1,
			Approval: marketing.ApprovalSnapshot{
				Status:          marketing.ApprovalApproved,
				ActivityVersion: 1,
			},
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
	if result.Output["concurrencyMode"] != "SERIAL_PER_EXECUTION" || result.Output["concurrencyEnforced"] != false {
		t.Fatalf("expected explicit advisory concurrency semantics, got %+v", result.Output)
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
	if trace.Execution.ID == source.ID || trace.Execution.Status != marketing.WorkflowExecutionFailed {
		t.Fatalf("expected new failed resume execution, got %+v", trace.Execution)
	}
	if !strings.Contains(string(trace.Execution.Input), `"mode":"RESUME"`) || !strings.Contains(string(trace.Execution.Input), `"sourceExecutionId":"`+source.ID+`"`) {
		t.Fatalf("expected resume metadata in input, got %s", string(trace.Execution.Input))
	}
}

func TestResumeWorkflowExecutionHydratesCommittedRewardRuntime(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:            "act_resume_reward",
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          "RESUME_REWARD",
		Name:          "Resume Reward",
		Type:          "STUDIO_WORKFLOW",
		Status:        marketing.ActivityActive,
		TriggerEvents: []string{"CHECKIN_SUCCESS"},
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
		"meta": {"activityId": "act_resume_reward", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id":"event-in","type":"event-in","kind":"entry","outputs":["raw_event"]},
			{"id":"grant-order","type":"grant-order","outputs":["grant_order"]},
			{"id":"outbox","type":"outbox","outputs":["HTTP"]},
			{"id":"audit","type":"audit"}
		],
		"edges": [
			{"id":"event-grant","source":"event-in","target":"grant-order"},
			{"id":"grant-outbox","source":"grant-order","target":"outbox"},
			{"id":"outbox-audit","source":"outbox","target":"audit"}
		]
	}`)
	version, err := repo.CreateWorkflowVersion(ctx, marketing.WorkflowVersion{
		ID:          "wfver_resume_reward",
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
	event := marketing.ActivityEvent{
		EventID:      "evt_resume_reward",
		EventType:    "CHECKIN_SUCCESS",
		SourceSystem: "app",
		TenantID:     activity.TenantID,
		Subject:      marketing.EventSubject{Type: "USER", ID: "u_resume_reward"},
		Biz:          marketing.EventBiz{BizType: "CHECKIN", BizID: "2026-06-06:u_resume_reward"},
		OccurredAt:   now,
		Attributes:   map[string]any{"streakDays": 1},
	}
	inbox, _, err := repo.CreateInbox(ctx, marketing.EventInbox{
		ID:             "inbox_resume_reward",
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  marketing.InboxProcessed,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	participation := marketing.Participation{
		ID:             "part_resume_reward",
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		EventInboxID:   inbox.ID,
		Subject:        event.Subject,
		Biz:            event.Biz,
		Status:         marketing.ParticipationQualified,
		IdempotencyKey: inbox.IdempotencyKey + ":" + activity.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	order := marketing.GrantOrder{
		ID:              "grant_resume_reward",
		TenantID:        activity.TenantID,
		ActivityID:      activity.ID,
		ParticipationID: participation.ID,
		Subject:         event.Subject,
		RewardType:      marketing.RewardBonus,
		RewardAmount:    "1.00",
		RewardPayload:   json.RawMessage(`{"rewardCode":"DAILY_CHECKIN_BONUS","currency":"USD"}`),
		BillNo:          "cmd_resume_reward",
		GrantStatus:     marketing.GrantPending,
		GrantChannel:    marketing.GrantHTTP,
		IdempotencyKey:  participation.IdempotencyKey + ":DAILY_CHECKIN_BONUS",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	outbox := marketing.OutboxMessage{
		ID:            "outbox_resume_reward",
		TenantID:      activity.TenantID,
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       json.RawMessage(`{"commandId":"cmd_resume_reward","participationId":"part_resume_reward"}`),
		SendStatus:    marketing.OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  participation,
		BudgetAmount:   "1.00",
		GrantOrders:    []marketing.GrantOrder{order},
		OutboxMessages: []marketing.OutboxMessage{outbox},
	}); err != nil {
		t.Fatalf("create committed reward runtime: %v", err)
	}
	finishedAt := now.Add(100 * time.Millisecond)
	source, err := repo.CreateWorkflowExecution(ctx, marketing.WorkflowExecution{
		ID:                "wfexec_resume_reward_source",
		TenantID:          activity.TenantID,
		ActivityID:        activity.ID,
		WorkflowVersionID: version.ID,
		WorkflowVersion:   version.Version,
		EventInboxID:      inbox.ID,
		EventID:           event.EventID,
		Status:            marketing.WorkflowExecutionFailed,
		Input: marshalWorkflowPayload(map[string]any{
			"event":          event,
			"idempotencyKey": workflowExecutionIdempotencyKey(activity.ID, version.ID, inbox.IdempotencyKey),
		}),
		ErrorMessage: "grant-order failed",
		StartedAt:    now,
		FinishedAt:   &finishedAt,
		CreatedAt:    now,
		UpdatedAt:    finishedAt,
	})
	if err != nil {
		t.Fatalf("create source execution: %v", err)
	}
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:          "node_resume_reward_event",
		TenantID:    activity.TenantID,
		ExecutionID: source.ID,
		NodeID:      "event-in",
		NodeType:    "event-in",
		Status:      marketing.WorkflowNodeSuccess,
		Output:      json.RawMessage(`{"raw_event":{"eventId":"evt_resume_reward"}}`),
		StartedAt:   now,
		FinishedAt:  &finishedAt,
		CreatedAt:   now,
		UpdatedAt:   finishedAt,
	}); err != nil {
		t.Fatalf("create event node execution: %v", err)
	}
	if _, err := repo.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:           "node_resume_reward_grant",
		TenantID:     activity.TenantID,
		ExecutionID:  source.ID,
		NodeID:       "grant-order",
		NodeType:     "grant-order",
		Status:       marketing.WorkflowNodeFailed,
		ErrorMessage: "temporary grant-order failure",
		StartedAt:    now.Add(10 * time.Millisecond),
		FinishedAt:   &finishedAt,
		CreatedAt:    now,
		UpdatedAt:    finishedAt,
	}); err != nil {
		t.Fatalf("create failed grant node execution: %v", err)
	}

	trace, err := service.ResumeWorkflowExecution(ctx, source.ID, marketing.ResumeWorkflowExecutionCommand{Operator: "ops", Reason: "retry committed reward tail"})
	if err != nil {
		t.Fatalf("resume execution: %v", err)
	}
	if trace.Execution.Status != marketing.WorkflowExecutionSuccess {
		t.Fatalf("expected successful resume, got %+v", trace.Execution)
	}
	statusByNode := map[string]marketing.WorkflowNodeExecutionStatus{}
	outputByNode := map[string]map[string]any{}
	for _, node := range trace.NodeExecutions {
		statusByNode[node.NodeID] = node.Status
		payload := map[string]any{}
		_ = json.Unmarshal(node.Output, &payload)
		outputByNode[node.NodeID] = payload
	}
	if statusByNode["grant-order"] != marketing.WorkflowNodeSuccess || statusByNode["outbox"] != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected grant-order and outbox to resume with hydrated runtime, statuses=%+v outputs=%+v", statusByNode, outputByNode)
	}
	if outputByNode["grant-order"]["grantOrderCount"] != float64(1) || outputByNode["outbox"]["outboxCount"] != float64(1) {
		t.Fatalf("expected hydrated grant/outbox counts, outputs=%+v", outputByNode)
	}
}
