package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestBuiltinWorkflowDraftTemplatesFollowCoreContract(t *testing.T) {
	validator := marketing.NewWorkflowValidationService()
	for _, template := range builtinWorkflowDraftTemplates(time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)) {
		t.Run(template.ID, func(t *testing.T) {
			result := validator.ValidateDefinition(template.Definition)
			if !result.Valid {
				t.Fatalf("builtin template must validate: %+v", result.Items)
			}
			definition, err := marketing.ParseWorkflowDefinition(template.Definition)
			if err != nil {
				t.Fatalf("parse definition: %v", err)
			}
			nodeTypes := map[string]bool{}
			for _, node := range definition.Nodes {
				nodeTypes[node.Type] = true
			}
			if nodeTypes["callback"] {
				t.Fatalf("callback must not be part of default production templates")
			}
			if nodeTypes["qualify"] {
				assertTemplateRoute(t, definition.Edges, "qualify", "PASS")
				assertTemplateRoute(t, definition.Edges, "qualify", "BLOCK")
				assertTemplateRoute(t, definition.Edges, "qualify", "RISK")
			}
			if nodeTypes["reward"] {
				for _, required := range []string{"participation", "grant-order", "outbox", "audit"} {
					if !nodeTypes[required] {
						t.Fatalf("reward templates must include %s", required)
					}
				}
			}
		})
	}
}

func assertTemplateRoute(t *testing.T, edges []marketing.WorkflowEdge, source string, routeKey string) {
	t.Helper()
	for _, edge := range edges {
		if edge.Source != source || edge.Route == nil {
			continue
		}
		if edge.Route.OutputKey() == routeKey {
			return
		}
	}
	t.Fatalf("expected %s route from %s", routeKey, source)
}

func TestCompareWorkflowDraftTemplateVersionsReturnsNodeAndEdgeConfigDiff(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)
	leftDefinition := json.RawMessage(`{
		"nodes":[
			{"id":"event","type":"event-in","title":"Event","config":{"eventTypes":["CHECKIN_SUCCESS"]}},
			{"id":"reward","type":"reward","title":"Reward","config":{"amount":1,"currency":"USD"}}
		],
		"edges":[
			{"id":"edge_event_reward","source":"event","target":"reward","route":{"type":"PASS","key":"PASS"}}
		]
	}`)
	rightDefinition := json.RawMessage(`{
		"nodes":[
			{"id":"event","type":"event-in","title":"Event","config":{"eventTypes":["CHECKIN_SUCCESS"]}},
			{"id":"reward","type":"reward","title":"Reward","config":{"amount":2,"currency":"USD","rewardCode":"DAILY_CHECKIN"}}
		],
		"edges":[
			{"id":"edge_event_reward","source":"event","target":"reward","route":{"type":"IF","key":"QUALIFIED","expression":"runtime.qualified == true"}}
		]
	}`)
	if _, err := repo.CreateWorkflowDraftTemplate(ctx, marketing.WorkflowDraftTemplate{
		ID:         "tpl_compare",
		VersionID:  "tpl_compare_v1",
		Version:    1,
		Name:       "Compare Template",
		Source:     marketing.WorkflowDraftTemplateSaved,
		Status:     marketing.WorkflowDraftTemplateActive,
		Definition: leftDefinition,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("create left template: %v", err)
	}
	if _, err := repo.CreateWorkflowDraftTemplate(ctx, marketing.WorkflowDraftTemplate{
		ID:         "tpl_compare",
		VersionID:  "tpl_compare_v2",
		Version:    2,
		Name:       "Compare Template",
		Source:     marketing.WorkflowDraftTemplateSaved,
		Status:     marketing.WorkflowDraftTemplateActive,
		Definition: rightDefinition,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatalf("create right template: %v", err)
	}
	compare, err := service.CompareWorkflowDraftTemplateVersions(ctx, "tpl_compare", "tpl_compare_v1", "tpl_compare_v2")
	if err != nil {
		t.Fatalf("compare template versions: %v", err)
	}
	if len(compare.NodeChanges) != 1 || compare.NodeChanges[0].NodeID != "reward" {
		t.Fatalf("expected reward node config diff, got %+v", compare.NodeChanges)
	}
	if !workflowDiffContainsPath(compare.NodeChanges[0].Changes, "$.config.amount") || !workflowDiffContainsPath(compare.NodeChanges[0].Changes, "$.config.rewardCode") {
		t.Fatalf("expected amount and rewardCode config diff, got %+v", compare.NodeChanges[0].Changes)
	}
	if len(compare.EdgeChanges) != 1 || compare.EdgeChanges[0].EdgeID != "edge_event_reward" {
		t.Fatalf("expected edge route diff, got %+v", compare.EdgeChanges)
	}
	if !workflowDiffContainsPath(compare.EdgeChanges[0].Changes, "$.route.Type") && !workflowDiffContainsPath(compare.EdgeChanges[0].Changes, "$.route.type") {
		t.Fatalf("expected route type diff, got %+v", compare.EdgeChanges[0].Changes)
	}
	defaultCompare, err := service.CompareWorkflowDraftTemplateVersions(ctx, "tpl_compare", "", "")
	if err != nil {
		t.Fatalf("compare default template versions: %v", err)
	}
	if defaultCompare.LeftVersionID != "tpl_compare_v1" || defaultCompare.RightVersionID != "tpl_compare_v2" {
		t.Fatalf("expected default compare to use oldest -> latest, got %s -> %s", defaultCompare.LeftVersionID, defaultCompare.RightVersionID)
	}
	if defaultCompare.ChangedMeta != nil {
		t.Fatalf("expected unchanged template metadata to be omitted, got %+v", defaultCompare.ChangedMeta)
	}
	if _, err := service.CompareWorkflowDraftTemplateVersions(ctx, "tpl_compare", "missing-left-version", "tpl_compare_v2"); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected missing left version to return ErrNotFound, got %v", err)
	}
	if _, err := service.CompareWorkflowDraftTemplateVersions(ctx, "tpl_compare", "tpl_compare_v1", "missing-right-version"); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected missing right version to return ErrNotFound, got %v", err)
	}
}

func workflowDiffContainsPath(rows []marketing.WorkflowExecutionJSONDiff, path string) bool {
	for _, row := range rows {
		if row.Path == path {
			return true
		}
	}
	return false
}

func TestApplyBuiltinRewardTemplatePublishesAndRunsEndToEnd(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC)
	reviewedAt := now
	activity := marketing.Activity{
		ID:            "act_builtin_template_e2e",
		TenantID:      "site_001",
		WorkspaceID:   "workspace-1",
		FolderID:      "folder-1",
		Code:          "BUILTIN_TEMPLATE_E2E",
		Name:          "Builtin Template E2E",
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
			Config:        []byte(`{"amount":"10.00","currency":"USD","targetRef":"https://wallet.example.com/reward"}`),
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
		{ID: "quota_user", ActivityID: activity.ID, Type: marketing.QuotaUserLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "u_template", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_daily", ActivityID: activity.ID, Type: marketing.QuotaDailyLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "u_template:2026-06-13", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "quota_stock", ActivityID: activity.ID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "BONUS", TotalValue: "10", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repo.CreateQuota(ctx, quota); err != nil {
			t.Fatalf("create quota %s: %v", quota.ID, err)
		}
	}
	draft, err := service.ApplyWorkflowDraftTemplate(ctx, activity.ID, marketing.ApplyWorkflowDraftTemplateCommand{
		TemplateID:  "simple-reward-chain",
		Operator:    "tester",
		WorkspaceID: "workspace-1",
		FolderID:    "folder-1",
	})
	if err != nil {
		t.Fatalf("apply template: %v", err)
	}
	definition, err := marketing.ParseWorkflowDefinition(draft.Definition)
	if err != nil {
		t.Fatalf("parse draft: %v", err)
	}
	if definition.Meta["contractVersion"] != "core-node-contract.v1" {
		t.Fatalf("expected core contract version in applied template, got %+v", definition.Meta)
	}
	if _, err := service.PublishWorkflowVersion(ctx, activity.ID, marketing.PublishWorkflowCommand{Operator: "tester"}); err != nil {
		t.Fatalf("publish workflow: %v", err)
	}
	if _, err := service.ProcessEvent(ctx, marketing.ActivityEvent{
		EventID:      "evt_builtin_template_e2e",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "u_template"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_template"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": "200.00"},
	}); err != nil {
		t.Fatalf("process event: %v", err)
	}
	executions, err := service.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activity.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list workflow executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != marketing.WorkflowExecutionSuccess {
		t.Fatalf("expected one successful workflow execution, got %+v", executions)
	}
	trace, err := service.ExportWorkflowExecutionTrace(ctx, executions[0].ID)
	if err != nil {
		t.Fatalf("export trace: %v", err)
	}
	statusByNode := map[string]marketing.WorkflowNodeExecutionStatus{}
	for _, node := range trace.NodeExecutions {
		statusByNode[node.NodeID] = node.Status
	}
	for _, nodeID := range []string{"event-in", "inbox", "normalize", "match", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"} {
		if statusByNode[nodeID] != marketing.WorkflowNodeSuccess {
			t.Fatalf("expected node %s to succeed, got %+v", nodeID, statusByNode)
		}
	}
	if len(trace.NodeSummaries) != len(trace.NodeExecutions) {
		t.Fatalf("expected node summaries for every execution, got summaries=%d nodes=%d", len(trace.NodeSummaries), len(trace.NodeExecutions))
	}
	var quotaOutput map[string]any
	for _, node := range trace.NodeExecutions {
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
		t.Fatalf("expected committed quota reservations in run history, got %+v", quotaHold)
	}
}

func TestBuiltinTemplateAuditRunsOnQualifyBlockRoute(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	runtime := workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             "inbox_builtin_template_block",
			IdempotencyKey: "idem_builtin_template_block",
			Event: marketing.ActivityEvent{
				EventID:      "evt_builtin_template_block",
				EventType:    "DEPOSIT_SUCCESS",
				SourceSystem: "payment",
				TenantID:     "site_001",
				Subject:      marketing.EventSubject{Type: "USER", ID: "u_block"},
				Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: "dep_block"},
				OccurredAt:   now,
				Attributes:   map[string]any{"amount": "50.00"},
			},
		},
		Activity: marketing.Activity{
			ID:          "act_builtin_template_block",
			TenantID:    "site_001",
			WorkspaceID: "workspace-1",
			FolderID:    "folder-1",
			Code:        "BUILTIN_TEMPLATE_BLOCK",
			Name:        "Builtin Template Block",
			Status:      marketing.ActivityActive,
			Version:     1,
		},
		Qualified: false,
		RuleResult: marketing.ActivityRuleResult{
			Decision:   marketing.RuleDecisionRejected,
			ReasonCode: "QUALIFY_BLOCKED",
			Reason:     "qualification failed",
		},
	}
	definitionBytes, _ := json.Marshal(workflowTemplateDefinition([]string{"event-in", "inbox", "normalize", "match", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}))
	execution, err := service.executeWorkflowDefinition(ctx, runtime, workflowExecutionOptions{
		WorkflowVersionID: "wfver_builtin_template_block",
		WorkflowVersion:   1,
		Definition:        definitionBytes,
		Mode:              "DEBUG",
		SkipIdempotency:   true,
	})
	if err != nil {
		t.Fatalf("execute workflow definition: %v", err)
	}
	if execution.Status != marketing.WorkflowExecutionFailed {
		t.Fatalf("expected blocked route to fail the overall execution, got %+v", execution)
	}
	nodes, err := service.ListWorkflowNodeExecutions(ctx, execution.ID)
	if err != nil {
		t.Fatalf("list node executions: %v", err)
	}
	statusByNode := map[string]marketing.WorkflowNodeExecutionStatus{}
	for _, node := range nodes {
		statusByNode[node.NodeID] = node.Status
		if node.NodeID == "audit" && node.ErrorMessage != "" {
			t.Fatalf("audit must not fail on BLOCK route, got %q", node.ErrorMessage)
		}
	}
	if statusByNode["qualify"] != marketing.WorkflowNodeSuccess || statusByNode["audit"] != marketing.WorkflowNodeSuccess {
		t.Fatalf("expected qualify and audit success, got %+v", statusByNode)
	}
	if statusByNode["reward"] == marketing.WorkflowNodeSuccess {
		t.Fatalf("reward should not succeed on BLOCK route, got %+v", statusByNode)
	}
}
