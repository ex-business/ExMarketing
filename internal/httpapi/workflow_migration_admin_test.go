package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestWorkflowServiceInterfaceMigrationAnalyzeApplyAndBlockers(t *testing.T) {
	ctx := context.Background()
	repo := marketing.NewMemoryRepository()
	service := engine.NewService(repo)
	handler := NewHandler(service)
	routes := handler.Routes()
	activity := createWorkflowMigrationFixture(t, ctx, repo, service)

	viewerReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workflows/"+activity.ID+"/migration/service-interface/analyze", nil)
	viewerReq.Header.Set("X-Actor-Id", "viewer")
	viewerRec := httptest.NewRecorder()
	routes.ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer migration analyze forbidden, got %d: %s", viewerRec.Code, viewerRec.Body.String())
	}
	deniedAudits, err := service.ListMigrationAuditLogs(ctx, marketing.MigrationAuditQuery{TenantID: "site_001", ActivityID: activity.ID, MigrationType: "WORKFLOW_SERVICE_INTERFACE", Operation: "ANALYZE", Result: marketing.MigrationAuditDenied})
	if err != nil {
		t.Fatalf("list denied migration audit logs: %v", err)
	}
	if len(deniedAudits) != 1 || deniedAudits[0].Actor != "viewer" || !strings.Contains(deniedAudits[0].Error, "workspace admin") {
		t.Fatalf("expected workflow migration auth denied audit, got %+v", deniedAudits)
	}

	analyzeReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workflows/"+activity.ID+"/migration/service-interface/analyze", nil)
	analyzeReq.Header.Set("X-Actor-Id", "owner")
	analyzeRec := httptest.NewRecorder()
	routes.ServeHTTP(analyzeRec, analyzeReq)
	if analyzeRec.Code != http.StatusOK {
		t.Fatalf("analyze failed: %d %s", analyzeRec.Code, analyzeRec.Body.String())
	}
	var analyzePayload struct {
		Data workflowServiceInterfaceMigrationAnalysis `json:"data"`
	}
	if err := json.NewDecoder(analyzeRec.Body).Decode(&analyzePayload); err != nil {
		t.Fatalf("decode analyze: %v", err)
	}
	if analyzePayload.Data.Summary.LegacySteps != 3 || analyzePayload.Data.Summary.AutoMigratable != 2 || analyzePayload.Data.Summary.Blockers == 0 {
		t.Fatalf("unexpected migration summary: %+v", analyzePayload.Data.Summary)
	}

	beforeDraft, err := service.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		t.Fatalf("get draft before dry-run: %v", err)
	}
	dryRunReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workflows/"+activity.ID+"/migration/service-interface/apply", strings.NewReader(`{"dryRun":true}`))
	dryRunReq.Header.Set("X-Actor-Id", "owner")
	dryRunRec := httptest.NewRecorder()
	routes.ServeHTTP(dryRunRec, dryRunReq)
	if dryRunRec.Code != http.StatusOK || !strings.Contains(dryRunRec.Body.String(), `"dryRun":true`) {
		t.Fatalf("dry-run failed: %d %s", dryRunRec.Code, dryRunRec.Body.String())
	}
	afterDryRunDraft, err := service.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		t.Fatalf("get draft after dry-run: %v", err)
	}
	if string(beforeDraft.Definition) != string(afterDryRunDraft.Definition) {
		t.Fatalf("dry-run changed draft\nbefore=%s\nafter=%s", beforeDraft.Definition, afterDryRunDraft.Definition)
	}

	confirmReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workflows/"+activity.ID+"/migration/service-interface/apply", strings.NewReader(`{
		"confirm": true,
		"confirmation": "APPLY_WORKFLOW_SERVICE_INTERFACE_MIGRATION",
		"reason": "migrate legacy provider fields"
	}`))
	confirmReq.Header.Set("X-Actor-Id", "owner")
	confirmRec := httptest.NewRecorder()
	routes.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("confirmed apply failed: %d %s", confirmRec.Code, confirmRec.Body.String())
	}

	draft, err := service.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		t.Fatalf("get draft after apply: %v", err)
	}
	var definition map[string]any
	if err := json.Unmarshal(draft.Definition, &definition); err != nil {
		t.Fatalf("decode migrated definition: %v", err)
	}
	nodes := definition["nodes"].([]any)
	factsConfig := nodes[0].(map[string]any)["config"].(map[string]any)
	factsChain := factsConfig["providerChain"].([]any)
	factsStep := factsChain[0].(map[string]any)
	if factsStep["targetType"] != "SERVICE_INTERFACE" || factsStep["serviceRef"] != "user-profile-service" || factsStep["interfaceRef"] != "query-user-profile" || factsStep["providerRef"] != "legacy-user-profile" {
		t.Fatalf("fact-query was not migrated to service interface: %+v", factsStep)
	}
	if _, ok := factsConfig["providerCode"]; ok {
		t.Fatalf("root legacy providerCode was not removed: %+v", factsConfig)
	}
	cacheStep := nodes[1].(map[string]any)["config"].(map[string]any)["providerChain"].([]any)[0].(map[string]any)
	if cacheStep["targetType"] != "RESOURCE" || cacheStep["resourceRef"] != "legacy-fact-cache" {
		t.Fatalf("redisRef was not migrated to resourceRef: %+v", cacheStep)
	}
	unsafeConfig := nodes[2].(map[string]any)["config"].(map[string]any)
	if unsafeConfig["endpoint"] != "https://legacy.example.test/facts" {
		t.Fatalf("blocked direct endpoint should not be applied: %+v", unsafeConfig)
	}

	postAnalyzeReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workflows/"+activity.ID+"/migration/service-interface/analyze", nil)
	postAnalyzeReq.Header.Set("X-Actor-Id", "owner")
	postAnalyzeRec := httptest.NewRecorder()
	routes.ServeHTTP(postAnalyzeRec, postAnalyzeReq)
	if postAnalyzeRec.Code != http.StatusOK {
		t.Fatalf("post analyze failed: %d %s", postAnalyzeRec.Code, postAnalyzeRec.Body.String())
	}
	var postAnalyzePayload struct {
		Data workflowServiceInterfaceMigrationAnalysis `json:"data"`
	}
	if err := json.NewDecoder(postAnalyzeRec.Body).Decode(&postAnalyzePayload); err != nil {
		t.Fatalf("decode post analyze: %v", err)
	}
	if postAnalyzePayload.Data.Summary.AutoMigratable != 0 || postAnalyzePayload.Data.Summary.Blockers == 0 {
		t.Fatalf("expected only blocker to remain, got %+v", postAnalyzePayload.Data.Summary)
	}
	audits, err := service.ListMigrationAuditLogs(ctx, marketing.MigrationAuditQuery{TenantID: "site_001", ActivityID: activity.ID, MigrationType: "WORKFLOW_SERVICE_INTERFACE", Limit: 10})
	if err != nil {
		t.Fatalf("list migration audit logs: %v", err)
	}
	seen := map[string]marketing.MigrationAuditLog{}
	for _, audit := range audits {
		if audit.Result != marketing.MigrationAuditSuccess {
			continue
		}
		seen[audit.Operation] = audit
	}
	for _, operation := range []string{"ANALYZE", "DRY_RUN", "APPLY"} {
		if seen[operation].ID == "" {
			t.Fatalf("missing %s migration audit in %+v", operation, audits)
		}
	}
	if seen["DRY_RUN"].DryRun != true || seen["DRY_RUN"].ItemsCount != 2 {
		t.Fatalf("expected dry-run audit with two applied items, got %+v", seen["DRY_RUN"])
	}
	if seen["APPLY"].ItemsCount != 2 || seen["APPLY"].BlockersCount == 0 {
		t.Fatalf("expected apply audit with applied items and remaining blocker, got %+v", seen["APPLY"])
	}
}

func createWorkflowMigrationFixture(t *testing.T, ctx context.Context, repo *marketing.MemoryRepository, service *engine.Service) marketing.Activity {
	t.Helper()
	workspace, err := service.CreateWorkspace(ctx, marketing.Workspace{
		TenantID:    "site_001",
		Name:        "Workflow Migration",
		Slug:        "workflow-migration-" + time.Now().Format("150405000000000"),
		Visibility:  marketing.WorkspaceTeam,
		DefaultRole: marketing.WorkspaceRoleGuest,
		OwnerID:     "owner",
		CreatedBy:   "owner",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := service.CreateWorkspaceMember(ctx, workspace.ID, marketing.WorkspaceMember{
		UserID:      "viewer",
		DisplayName: "Viewer",
		Role:        marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceMemberActive,
		InvitedBy:   "owner",
	}); err != nil {
		t.Fatalf("create viewer member: %v", err)
	}
	now := time.Now().UTC()
	activity, err := repo.CreateActivity(ctx, marketing.Activity{
		ID:            "act_legacy_migration_" + now.Format("150405000000000"),
		TenantID:      "site_001",
		WorkspaceID:   workspace.ID,
		Code:          "legacy-migration-" + now.Format("150405000000000"),
		Name:          "Legacy Migration",
		Type:          "CHECKIN",
		Status:        marketing.ActivityDraft,
		Visibility:    marketing.WorkspaceTeam,
		TriggerEvents: []string{"USER_CHECKIN"},
		StartTime:     now.Add(time.Hour),
		EndTime:       now.Add(24 * time.Hour),
		Timezone:      "UTC",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create activity: %v", err)
	}
	definition := json.RawMessage(`{
		"meta": {"name": "legacy migration fixture"},
		"nodes": [
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "source": "FACT_SERVICE", "serviceRef": "user-profile-service", "interfaceRef": "query-user-profile", "providerCode": "legacy-user-profile", "onMiss": "BLOCK"}},
			{"id": "cache", "type": "fact-query", "config": {"factKeys": ["CACHE"], "providerChain": [{"source": "REDIS", "redisRef": "legacy-fact-cache", "operation": "READ"}]}},
			{"id": "unsafe", "type": "fact-query", "config": {"factKeys": ["UNSAFE"], "source": "HTTP", "endpoint": "https://legacy.example.test/facts"}}
		],
		"edges": []
	}`)
	if _, err := service.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{Operator: "owner", Definition: definition}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	return activity
}
