package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	activityplugin "github.com/ex-stack/ExMarketing/internal/plugin"
)

func TestPostgresAtomicGrantRollsBackOnStockExceeded(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_atomic_rollback"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	for _, quota := range []marketing.Quota{
		{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_user", ActivityID: activityID, Type: marketing.QuotaUserLimit, Scope: marketing.QuotaScopeUser, ScopeKey: "user_1", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock_a", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock_b", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_B", TotalValue: "0", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repo.CreateQuota(ctx, quota); err != nil {
			t.Fatalf("create quota %s: %v", quota.ID, err)
		}
	}

	_, err = repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation: marketing.Participation{
			ID:             prefix + "_part",
			TenantID:       "site_001",
			ActivityID:     activityID,
			EventInboxID:   prefix + "_inbox",
			Subject:        marketing.EventSubject{Type: "USER", ID: "user_1"},
			Biz:            marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
			Status:         marketing.ParticipationRewardable,
			Snapshot:       []byte(`{}`),
			IdempotencyKey: prefix + "_part_idem",
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		BudgetAmount:     "20",
		ReserveUserLimit: true,
		UserScopeKey:     "user_1",
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_B", Amount: "1"},
		},
		GrantOrders: []marketing.GrantOrder{
			{ID: prefix + "_grant_1", TenantID: "site_001", ActivityID: activityID, ParticipationID: prefix + "_part", Subject: marketing.EventSubject{Type: "USER", ID: "user_1"}, RewardType: marketing.RewardBonus, RewardAmount: "10", RewardPayload: []byte(`{}`), BillNo: prefix + "_cmd_1", GrantStatus: marketing.GrantPending, GrantChannel: marketing.GrantHTTP, IdempotencyKey: prefix + "_grant_1", CreatedAt: now, UpdatedAt: now},
			{ID: prefix + "_grant_2", TenantID: "site_001", ActivityID: activityID, ParticipationID: prefix + "_part", Subject: marketing.EventSubject{Type: "USER", ID: "user_1"}, RewardType: marketing.RewardBonus, RewardAmount: "10", RewardPayload: []byte(`{}`), BillNo: prefix + "_cmd_2", GrantStatus: marketing.GrantPending, GrantChannel: marketing.GrantHTTP, IdempotencyKey: prefix + "_grant_2", CreatedAt: now, UpdatedAt: now},
		},
		OutboxMessages: []marketing.OutboxMessage{
			{ID: prefix + "_outbox_1", TenantID: "site_001", MessageType: "REWARD_COMMAND", TargetChannel: marketing.GrantHTTP, TargetRef: "https://reward.example.com", Payload: []byte(`{}`), SendStatus: marketing.OutboxPending, CreatedAt: now, UpdatedAt: now},
			{ID: prefix + "_outbox_2", TenantID: "site_001", MessageType: "REWARD_COMMAND", TargetChannel: marketing.GrantHTTP, TargetRef: "https://reward.example.com", Payload: []byte(`{}`), SendStatus: marketing.OutboxPending, CreatedAt: now, UpdatedAt: now},
		},
	})
	if !errors.Is(err, marketing.ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}

	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value <> 0`, activityID, 0)
}

func TestPostgresCreateInboxDetectsDuplicateAndConflict(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_inbox_idem"
	cleanupPostgresInboxTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresInboxTest(t, repo, prefix) })

	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	event := marketing.ActivityEvent{
		EventID:      prefix + "_evt",
		EventType:    "DEPOSIT_SUCCESS",
		SourceSystem: "payment",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "10001"},
		Biz:          marketing.EventBiz{BizType: "DEPOSIT", BizID: prefix + "_pay"},
		OccurredAt:   now,
		Attributes:   map[string]any{"amount": jsonNumber("12.34"), "count": jsonNumber("2")},
	}
	inbox := marketing.EventInbox{
		ID:             prefix + "_inbox_1",
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  marketing.InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if _, duplicate, err := repo.CreateInbox(ctx, inbox); err != nil || duplicate {
		t.Fatalf("create inbox duplicate=%v err=%v", duplicate, err)
	}

	same := inbox
	same.ID = prefix + "_inbox_2"
	same.Event.Attributes = map[string]any{"count": 2.0, "amount": 12.34}
	existing, duplicate, err := repo.CreateInbox(ctx, same)
	if err != nil || !duplicate {
		t.Fatalf("expected duplicate inbox, duplicate=%v err=%v", duplicate, err)
	}
	if existing.ID != inbox.ID {
		t.Fatalf("expected existing inbox %s, got %s", inbox.ID, existing.ID)
	}

	conflict := inbox
	conflict.ID = prefix + "_inbox_3"
	conflict.Event.EventID = prefix + "_evt_conflict"
	if _, duplicate, err := repo.CreateInbox(ctx, conflict); !errors.Is(err, marketing.ErrIdempotencyConflict) || duplicate {
		t.Fatalf("expected idempotency conflict, duplicate=%v err=%v", duplicate, err)
	}

	eventIDConflict := inbox
	eventIDConflict.ID = prefix + "_inbox_4"
	eventIDConflict.Event.Biz.BizID = prefix + "_pay_other"
	eventIDConflict.IdempotencyKey = eventIDConflict.Event.IdempotencyKey()
	existing, duplicate, err = repo.CreateInbox(ctx, eventIDConflict)
	if err != nil || !duplicate {
		t.Fatalf("expected same event id duplicate, duplicate=%v err=%v", duplicate, err)
	}
	if existing.ID != inbox.ID {
		t.Fatalf("expected original inbox %s, got %s", inbox.ID, existing.ID)
	}
}

func TestPostgresWorkspaceCompositeConstraintsRejectCrossWorkspaceReferences(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_workspace_fk"
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })

	now := time.Now().UTC()
	first := marketing.Workspace{
		ID:          prefix + "_ws_1",
		TenantID:    "site_001",
		Name:        "First",
		Slug:        prefix + "-first",
		Visibility:  marketing.WorkspaceTeam,
		DefaultRole: marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	second := first
	second.ID = prefix + "_ws_2"
	second.Name = "Second"
	second.Slug = prefix + "-second"
	if _, err := repo.CreateWorkspace(ctx, first); err != nil {
		t.Fatalf("create first workspace: %v", err)
	}
	if _, err := repo.CreateWorkspace(ctx, second); err != nil {
		t.Fatalf("create second workspace: %v", err)
	}

	parent := marketing.WorkspaceFolder{
		ID:          prefix + "_folder_parent",
		WorkspaceID: first.ID,
		Name:        "Parent",
		Visibility:  marketing.WorkspaceFolderTeam,
		Status:      marketing.WorkspaceFolderActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateWorkspaceFolder(ctx, parent); err != nil {
		t.Fatalf("create parent folder: %v", err)
	}

	_, err := repo.CreateWorkspaceFolder(ctx, marketing.WorkspaceFolder{
		ID:          prefix + "_folder_child",
		WorkspaceID: second.ID,
		ParentID:    parent.ID,
		Name:        "Invalid child",
		Visibility:  marketing.WorkspaceFolderTeam,
		Status:      marketing.WorkspaceFolderActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected not found for cross-workspace parent, got %v", err)
	}

	_, err = repo.CreateWorkspaceShareLink(ctx, marketing.WorkspaceShareLink{
		ID:          prefix + "_share",
		WorkspaceID: second.ID,
		FolderID:    parent.ID,
		Token:       prefix + "_token",
		Scope:       marketing.WorkspaceShareFolder,
		Permission:  marketing.WorkspaceShareView,
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected not found for cross-workspace share folder, got %v", err)
	}

	_, err = repo.CreateActivity(ctx, marketing.Activity{
		ID:             prefix + "_activity",
		TenantID:       "site_001",
		WorkspaceID:    second.ID,
		FolderID:       parent.ID,
		Code:           prefix + "_activity",
		Name:           "Invalid placement",
		Type:           "STUDIO_WORKFLOW",
		Status:         marketing.ActivityDraft,
		TriggerEvents:  []string{"STUDIO_EVENT"},
		StartTime:      now.Add(-time.Minute),
		EndTime:        now.Add(time.Hour),
		Timezone:       "UTC",
		Priority:       1,
		ExclusiveGroup: "",
		Stackable:      true,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected not found for cross-workspace activity folder, got %v", err)
	}
}

func TestPostgresResourceRepositoryStoresBindingsAndAudit(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_resource_%d", time.Now().UnixNano())
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	resource := marketing.Resource{
		ID:           prefix + "_res",
		TenantID:     "site_001",
		WorkspaceID:  prefix + "_ws",
		Code:         prefix + "_user_profile",
		Name:         "用户资料 HTTP 资源",
		Type:         marketing.ResourceHTTPProvider,
		Driver:       marketing.ResourceDriverHTTP,
		Scope:        marketing.ResourceScopeWorkspace,
		Status:       marketing.ResourceActive,
		Config:       json.RawMessage(`{"endpoint":"https://profile.example.com/query","method":"POST"}`),
		Capabilities: []marketing.ResourceOperation{marketing.ResourceRead, marketing.ResourceCall, marketing.ResourceTestConnection},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	saved, err := repo.SaveResource(ctx, resource)
	if err != nil {
		t.Fatalf("save resource: %v", err)
	}
	if saved.ID != resource.ID || saved.ConfigMap()["endpoint"] != "https://profile.example.com/query" {
		t.Fatalf("unexpected saved resource: %+v", saved)
	}
	resource.Name = "用户资料 HTTP 资源 V2"
	resource.Capabilities = []marketing.ResourceOperation{marketing.ResourceRead}
	updated, err := repo.SaveResource(ctx, resource)
	if err != nil {
		t.Fatalf("update resource: %v", err)
	}
	if updated.ID != resource.ID || updated.Name != "用户资料 HTTP 资源 V2" || !updated.Supports(marketing.ResourceRead) || updated.Supports(marketing.ResourceCall) {
		t.Fatalf("unexpected updated resource: %+v", updated)
	}
	got, err := repo.GetResource(ctx, "site_001", resource.Code)
	if err != nil {
		t.Fatalf("get resource: %v", err)
	}
	if got.ID != resource.ID {
		t.Fatalf("expected resource id %s, got %+v", resource.ID, got)
	}
	listed, err := repo.ListResources(ctx, marketing.ResourceQuery{TenantID: "site_001", WorkspaceID: prefix + "_ws", Type: marketing.ResourceHTTPProvider, Status: marketing.ResourceActive})
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(listed) != 1 || listed[0].Code != resource.Code {
		t.Fatalf("expected resource in list, got %+v", listed)
	}

	binding, err := repo.SaveResourceBinding(ctx, marketing.ResourceBinding{
		ID:                prefix + "_bind",
		ResourceID:        resource.ID,
		TenantID:          "site_001",
		WorkspaceID:       prefix + "_ws",
		NodeType:          "fact-query-http",
		AllowedOperations: []marketing.ResourceOperation{marketing.ResourceRead},
		AllowedNamespaces: []string{"profile"},
		Status:            marketing.ResourceBindingActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil {
		t.Fatalf("save binding: %v", err)
	}
	bindings, err := repo.ListResourceBindings(ctx, marketing.ResourceBindingQuery{ResourceID: resource.ID, TenantID: "site_001", WorkspaceID: prefix + "_ws", NodeType: "fact-query-http", Status: marketing.ResourceBindingActive})
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != binding.ID || !bindings[0].Allows(marketing.ResourceAccessRequest{TenantID: "site_001", WorkspaceID: prefix + "_ws", NodeType: "fact-query-http", Operation: marketing.ResourceRead, Namespace: "profile"}) {
		t.Fatalf("expected matching binding, got %+v", bindings)
	}

	if err := repo.CreateResourceAuditLog(ctx, marketing.ResourceAuditLog{
		ID:           prefix + "_audit",
		TenantID:     "site_001",
		WorkspaceID:  prefix + "_ws",
		ResourceID:   resource.ID,
		ResourceCode: resource.Code,
		Operation:    marketing.ResourceRead,
		Status:       marketing.ResourceAuditSuccess,
		NodeID:       prefix + "_node",
		NodeType:     "fact-query-http",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	logs, err := repo.ListResourceAuditLogs(ctx, marketing.ResourceAuditQuery{ResourceID: resource.ID, TenantID: "site_001", WorkspaceID: prefix + "_ws", Status: marketing.ResourceAuditSuccess})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].ResourceCode != resource.Code || logs[0].NodeID != prefix+"_node" {
		t.Fatalf("expected audit log, got %+v", logs)
	}
}

func TestPostgresWorkspaceSearchLargeDatasetPerformance(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_search_perf_%d", time.Now().UnixNano())
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })

	now := time.Now().UTC()
	actorID := prefix + "_actor"
	workspaceID := prefix + "_workspace"
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace (
			id, tenant_id, name, slug, description, icon, color, visibility, default_role, owner_id, status, created_by, updated_by, created_at, updated_at
		) VALUES ($1, 'site_001', 'Search Performance Workspace', $2, 'large search fixture', '', '', 'TEAM', 'VIEWER', $3, 'ACTIVE', $3, $3, $4, $4)
	`, workspaceID, prefix+"-workspace", actorID, now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_member (
			id, workspace_id, user_id, email, display_name, role, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'Search Actor', 'ADMIN', 'ACTIVE', $5, $5)
	`, prefix+"_member", workspaceID, actorID, actorID+"@example.com", now); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_workspace_folder (
			id, workspace_id, parent_id, name, description, icon, color, visibility, sort_order, status, created_by, updated_by, created_at, updated_at
		)
		SELECT $1 || '_folder_' || gs::text, $2, NULL,
		       CASE WHEN gs % 97 = 0 THEN 'needle-campaign folder ' || gs::text ELSE 'bulk folder ' || gs::text END,
		       'folder performance fixture ' || gs::text, '', '', 'TEAM', gs, 'ACTIVE', $3, $3, $4, $4
		FROM generate_series(1, 1500) AS gs
	`, prefix, workspaceID, actorID, now); err != nil {
		t.Fatalf("insert folders: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_activity (
			id, tenant_id, workspace_id, folder_id, activity_code, activity_name, activity_type, status, visibility,
			trigger_events, start_time, end_time, timezone, priority, exclusive_group, stackable, plugin_code,
			version, config_json, created_at, updated_at
		)
		SELECT $1 || '_activity_' || gs::text, 'site_001', $2, $1 || '_folder_' || ((gs % 1500) + 1)::text,
		       $1 || '_ACT_' || gs::text,
		       CASE WHEN gs % 101 = 0 THEN 'needle-campaign workflow ' || gs::text ELSE 'bulk workflow ' || gs::text END,
		       'MARKETING', 'ACTIVE', 'TEAM', '[]'::jsonb, $3::timestamptz, $3::timestamptz + interval '30 days', 'UTC', 0, '', false, '',
		       1, '{}'::jsonb, $3::timestamptz, $3::timestamptz
		FROM generate_series(1, 3500) AS gs
	`, prefix, workspaceID, now); err != nil {
		t.Fatalf("insert activities: %v", err)
	}

	start := time.Now()
	result, err := repo.SearchWorkspaceHome(ctx, marketing.WorkspaceHomeSearchQuery{
		Keyword: "needle-campaign",
		ActorID: actorID,
		Limit:   20,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("search large dataset: %v", err)
	}
	if len(result.Items) == 0 || result.Total == 0 {
		t.Fatalf("expected search hits, got %+v", result)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("large dataset search exceeded budget: %s total=%d", elapsed, result.Total)
	}
	foundHighlight := false
	foundPermissionSource := false
	for _, item := range result.Items {
		if len(item.PermissionSources) >= 2 {
			foundPermissionSource = true
		}
		if fmt.Sprint(item.Highlights["title"]) != "" || fmt.Sprint(item.Highlights["subtitle"]) != "" {
			foundHighlight = true
		}
	}
	if !foundHighlight || !foundPermissionSource {
		t.Fatalf("expected highlights and detailed permission sources, got %+v", result.Items)
	}
	t.Logf("large workspace search total=%d returned=%d elapsed=%s", result.Total, len(result.Items), elapsed)
}

func TestPostgresWorkspaceSearchTeamWorkspaceRequiresMembership(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_search_visibility_%d", time.Now().UnixNano())
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })

	now := time.Now().UTC()
	teamWorkspace := marketing.Workspace{
		ID:          prefix + "_team",
		TenantID:    "site_001",
		Name:        "Visibility Probe Team Workspace",
		Slug:        prefix + "-team",
		Visibility:  marketing.WorkspaceTeam,
		DefaultRole: marketing.WorkspaceRoleViewer,
		OwnerID:     prefix + "_owner",
		Status:      marketing.WorkspaceActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	publicWorkspace := teamWorkspace
	publicWorkspace.ID = prefix + "_public"
	publicWorkspace.Name = "Visibility Probe Public Workspace"
	publicWorkspace.Slug = prefix + "-public"
	publicWorkspace.Visibility = marketing.WorkspacePublic
	if _, err := repo.CreateWorkspace(ctx, teamWorkspace); err != nil {
		t.Fatalf("create team workspace: %v", err)
	}
	if _, err := repo.CreateWorkspace(ctx, publicWorkspace); err != nil {
		t.Fatalf("create public workspace: %v", err)
	}

	strangerResult, err := repo.SearchWorkspaceHome(ctx, marketing.WorkspaceHomeSearchQuery{
		Keyword: "Visibility Probe",
		Type:    "workspace",
		ActorID: prefix + "_stranger",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search as stranger: %v", err)
	}
	if containsWorkspaceSearchResult(strangerResult.Items, teamWorkspace.ID) {
		t.Fatalf("stranger should not see TEAM workspace, got %+v", strangerResult.Items)
	}
	if !containsWorkspaceSearchResult(strangerResult.Items, publicWorkspace.ID) {
		t.Fatalf("stranger should see PUBLIC workspace, got %+v", strangerResult.Items)
	}

	memberID := prefix + "_member"
	if _, err := repo.CreateWorkspaceMember(ctx, marketing.WorkspaceMember{
		ID:          memberID,
		WorkspaceID: teamWorkspace.ID,
		UserID:      prefix + "_member_user",
		Email:       prefix + "_member@example.com",
		Role:        marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceMemberActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create workspace member: %v", err)
	}
	memberResult, err := repo.SearchWorkspaceHome(ctx, marketing.WorkspaceHomeSearchQuery{
		Keyword: "Visibility Probe",
		Type:    "workspace",
		ActorID: prefix + "_member_user",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search as member: %v", err)
	}
	if !containsWorkspaceSearchResult(memberResult.Items, teamWorkspace.ID) {
		t.Fatalf("member should see TEAM workspace, got %+v", memberResult.Items)
	}
}

func TestPostgresWorkspaceBatchRollsBackWhenOneWorkflowUnauthorized(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_batch_acl_%d", time.Now().UnixNano())
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })
	fixture := createPostgresBatchFixture(t, ctx, repo, prefix)
	actorID := prefix + "_editor"
	if _, err := repo.CreateWorkspaceMember(ctx, marketing.WorkspaceMember{
		ID:          prefix + "_member_editor",
		WorkspaceID: fixture.workspace.ID,
		UserID:      actorID,
		Role:        marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceMemberActive,
		CreatedAt:   fixture.now,
		UpdatedAt:   fixture.now,
	}); err != nil {
		t.Fatalf("create editor member: %v", err)
	}
	if _, err := repo.CreateWorkspaceActivityGrant(ctx, marketing.WorkspaceActivityGrant{
		ID:            prefix + "_grant_allowed",
		WorkspaceID:   fixture.workspace.ID,
		ActivityID:    fixture.activeWorkflow.ID,
		PrincipalType: marketing.WorkspacePrincipalUser,
		PrincipalID:   actorID,
		Role:          marketing.WorkspaceRoleEditor,
		CreatedAt:     fixture.now,
		UpdatedAt:     fixture.now,
	}); err != nil {
		t.Fatalf("create workflow grant: %v", err)
	}

	result, err := repo.ApplyWorkspaceBatch(ctx, marketing.WorkspaceBatchCommand{
		WorkspaceID: fixture.workspace.ID,
		Action:      marketing.WorkspaceBatchPermission,
		Visibility:  marketing.WorkspacePublic,
		ActorID:     actorID,
		Items: []marketing.WorkspaceBatchItem{
			{ID: fixture.activeWorkflow.ID, Type: marketing.WorkspaceBatchWorkflow},
			{ID: fixture.secondWorkflow.ID, Type: marketing.WorkspaceBatchWorkflow},
		},
	})
	if !errors.Is(err, marketing.ErrInvalidWorkspace) || result.Committed {
		t.Fatalf("expected invalid uncommitted batch, result=%+v err=%v", result, err)
	}
	assertPostgresBatchFailure(t, result, fixture.secondWorkflow.ID, "actor has no permission for workflow")
	assertPostgresActivityVisibility(t, repo, fixture.activeWorkflow.ID, marketing.WorkspaceTeam)
	assertPostgresActivityVisibility(t, repo, fixture.secondWorkflow.ID, marketing.WorkspaceTeam)
}

func TestPostgresWorkspaceBatchRollsBackIllegalFolderHierarchy(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_batch_hierarchy_%d", time.Now().UnixNano())
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })
	fixture := createPostgresBatchFixture(t, ctx, repo, prefix)

	result, err := repo.ApplyWorkspaceBatch(ctx, marketing.WorkspaceBatchCommand{
		WorkspaceID: fixture.workspace.ID,
		Action:      marketing.WorkspaceBatchMove,
		FolderID:    fixture.childFolder.ID,
		ActorID:     fixture.workspace.OwnerID,
		Items: []marketing.WorkspaceBatchItem{
			{ID: fixture.parentFolder.ID, Type: marketing.WorkspaceBatchFolder},
			{ID: fixture.activeWorkflow.ID, Type: marketing.WorkspaceBatchWorkflow},
		},
	})
	if !errors.Is(err, marketing.ErrInvalidWorkspace) || result.Committed {
		t.Fatalf("expected invalid uncommitted batch, result=%+v err=%v", result, err)
	}
	assertPostgresBatchFailure(t, result, fixture.parentFolder.ID, "cannot move folder into itself or its descendant")
	assertPostgresFolderParent(t, repo, fixture.parentFolder.ID, "")
	assertPostgresActivityFolder(t, repo, fixture.activeWorkflow.ID, fixture.parentFolder.ID)
}

func TestPostgresWorkspaceBatchRollsBackWhenWorkflowArchived(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_batch_archived_%d", time.Now().UnixNano())
	cleanupPostgresWorkspaceTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresWorkspaceTest(t, repo, prefix) })
	fixture := createPostgresBatchFixture(t, ctx, repo, prefix)

	result, err := repo.ApplyWorkspaceBatch(ctx, marketing.WorkspaceBatchCommand{
		WorkspaceID: fixture.workspace.ID,
		Action:      marketing.WorkspaceBatchMove,
		FolderID:    fixture.targetFolder.ID,
		ActorID:     fixture.workspace.OwnerID,
		Items: []marketing.WorkspaceBatchItem{
			{ID: fixture.activeWorkflow.ID, Type: marketing.WorkspaceBatchWorkflow},
			{ID: fixture.archivedWorkflow.ID, Type: marketing.WorkspaceBatchWorkflow},
		},
	})
	if !errors.Is(err, marketing.ErrInvalidWorkspace) || result.Committed {
		t.Fatalf("expected invalid uncommitted batch, result=%+v err=%v", result, err)
	}
	assertPostgresBatchFailure(t, result, fixture.archivedWorkflow.ID, "workflow is archived")
	assertPostgresActivityFolder(t, repo, fixture.activeWorkflow.ID, fixture.parentFolder.ID)
	assertPostgresActivityFolder(t, repo, fixture.archivedWorkflow.ID, fixture.parentFolder.ID)
}

func TestPostgresPluginScheduleEventInboxSmoke(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_schedule_inbox"
	cleanupPostgresInboxTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresInboxTest(t, repo, prefix) })

	scheduledAt := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	job := activityplugin.PluginScheduleJob{
		JobID:              prefix + "_job",
		TenantID:           "site_001",
		ActivityID:         prefix + "_act",
		ActivityType:       "LOTTERY",
		ActivityVersion:    7,
		ActivityConfigHash: "locked-config",
		PluginCode:         "LOTTERY_PLUGIN",
		JobType:            activityplugin.PluginScheduleJobDraw,
		ScheduledAt:        scheduledAt,
	}
	event, err := activityplugin.StandardEventFromPluginJob(job, activityplugin.PluginGeneratedEvent{
		Attributes: map[string]any{"roundId": prefix + "_round"},
	}, 0)
	if err != nil {
		t.Fatalf("map scheduler event: %v", err)
	}
	if !event.OccurredAt.Equal(scheduledAt) {
		t.Fatalf("occurredAt=%s, want scheduledAt %s", event.OccurredAt, scheduledAt)
	}
	inbox := marketing.EventInbox{
		ID:             prefix + "_inbox_1",
		Event:          event,
		IdempotencyKey: event.IdempotencyKey(),
		ProcessStatus:  marketing.InboxReceived,
		CreatedAt:      scheduledAt,
		UpdatedAt:      scheduledAt,
	}
	if _, duplicate, err := repo.CreateInbox(ctx, inbox); err != nil || duplicate {
		t.Fatalf("create scheduler inbox duplicate=%v err=%v", duplicate, err)
	}

	same := inbox
	same.ID = prefix + "_inbox_2"
	existing, duplicate, err := repo.CreateInbox(ctx, same)
	if err != nil || !duplicate || existing.ID != inbox.ID {
		t.Fatalf("expected scheduler duplicate, existing=%+v duplicate=%v err=%v", existing, duplicate, err)
	}

	conflict := inbox
	conflict.ID = prefix + "_inbox_3"
	conflict.Event.EventID = prefix + "_evt_conflict"
	if _, duplicate, err := repo.CreateInbox(ctx, conflict); !errors.Is(err, marketing.ErrIdempotencyConflict) || duplicate {
		t.Fatalf("expected scheduler idempotency conflict, duplicate=%v err=%v", duplicate, err)
	}
}

func TestPostgresActivityUniqueDetailAndUpdate(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_activity_main"
	cleanupPostgresActivityTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresActivityTest(t, repo, prefix) })

	now := time.Now().UTC()
	activity := testPostgresActivity(prefix+"_act_1", "tenant_a", prefix+"_code", now)
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	duplicate := testPostgresActivity(prefix+"_act_2", "tenant_a", prefix+"_code", now)
	if _, err := repo.CreateActivity(ctx, duplicate); !errors.Is(err, marketing.ErrActivityConflict) {
		t.Fatalf("expected activity conflict, got %v", err)
	}
	otherTenant := testPostgresActivity(prefix+"_act_3", "tenant_b", prefix+"_code", now)
	if _, err := repo.CreateActivity(ctx, otherTenant); err != nil {
		t.Fatalf("expected same code in different tenant to succeed: %v", err)
	}

	got, err := repo.GetActivity(ctx, activity.ID)
	if err != nil {
		t.Fatalf("get activity: %v", err)
	}
	got.Name = "Updated"
	got.Status = marketing.ActivityPaused
	got.Timezone = "Asia/Shanghai"
	got.Priority = 7
	got.Stackable = true
	got.Version++
	got.Config = []byte(`{"metadata":{"owner":"qa"}}`)
	got.UpdatedAt = now.Add(time.Second)
	updated, err := repo.UpdateActivity(ctx, got)
	if err != nil {
		t.Fatalf("update activity: %v", err)
	}
	if updated.Version != 2 || updated.Name != "Updated" || updated.Timezone != "Asia/Shanghai" || !strings.Contains(string(updated.Config), `"qa"`) {
		t.Fatalf("unexpected updated activity: %+v config=%s", updated, string(updated.Config))
	}
}

func TestPostgresParticipationAndGrantIdempotencyUniqueConstraints(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_part_grant_idem"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	participation := testPostgresParticipation(prefix+"_part_1", prefix+"_act", prefix+"_part_idem", now)
	if _, err := repo.CreateParticipation(ctx, participation); err != nil {
		t.Fatalf("create participation: %v", err)
	}
	participation.ID = prefix + "_part_2"
	if _, err := repo.CreateParticipation(ctx, participation); err == nil {
		t.Fatal("expected duplicate participation idempotency key to fail")
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE idempotency_key = $1`, prefix+"_part_idem", 1)

	order := testPostgresGrant(prefix+"_grant_1", prefix+"_act", prefix+"_part_1", prefix+"_grant_idem", now)
	if _, err := repo.CreateGrantOrder(ctx, order); err != nil {
		t.Fatalf("create grant order: %v", err)
	}
	order.ID = prefix + "_grant_2"
	order.BillNo = prefix + "_cmd_2"
	if _, err := repo.CreateGrantOrder(ctx, order); err == nil {
		t.Fatal("expected duplicate grant order idempotency key to fail")
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE idempotency_key = $1`, prefix+"_grant_idem", 1)
}

func TestPostgresActivityStateReadUpsertAndCAS(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_activity_state"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activity := testPostgresActivity(prefix+"_act", "site_001", strings.ToUpper(prefix), now)
	if _, err := repo.CreateActivity(ctx, activity); err != nil {
		t.Fatalf("create activity: %v", err)
	}
	key := marketing.ActivityStateKey{
		TenantID:   "site_001",
		ActivityID: activity.ID,
		Namespace:  "checkin",
		StateKey:   activity.ID + ":user_1",
	}
	if _, err := repo.GetActivityState(ctx, key); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected missing state, got %v", err)
	}

	created, err := repo.UpsertActivityState(ctx, marketing.UpsertActivityStateCommand{
		State: marketing.ActivityState{
			ID:          prefix + "_state",
			TenantID:    key.TenantID,
			ActivityID:  key.ActivityID,
			Namespace:   key.Namespace,
			StateKey:    key.StateKey,
			Subject:     marketing.EventSubject{Type: "USER", ID: "user_1"},
			Data:        json.RawMessage(`{"lastCheckinDate":"2026-06-14","streakDays":1}`),
			LastEventID: prefix + "_evt_1",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		Merge: true,
	})
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %+v", created)
	}

	updated, err := repo.UpsertActivityState(ctx, marketing.UpsertActivityStateCommand{
		State: marketing.ActivityState{
			TenantID:    key.TenantID,
			ActivityID:  key.ActivityID,
			Namespace:   key.Namespace,
			StateKey:    key.StateKey,
			Subject:     marketing.EventSubject{Type: "USER", ID: "user_1"},
			Data:        json.RawMessage(`{"streakDays":2}`),
			LastEventID: prefix + "_evt_2",
			UpdatedAt:   now.Add(time.Minute),
		},
		ExpectedVersion: 1,
		RequireVersion:  true,
		Merge:           true,
	})
	if err != nil {
		t.Fatalf("cas update state: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %+v", updated)
	}
	var data map[string]any
	if err := json.Unmarshal(updated.Data, &data); err != nil {
		t.Fatalf("decode state data: %v", err)
	}
	if data["lastCheckinDate"] != "2026-06-14" || data["streakDays"] != float64(2) {
		t.Fatalf("expected merged state data, got %+v", data)
	}

	_, err = repo.UpsertActivityState(ctx, marketing.UpsertActivityStateCommand{
		State: marketing.ActivityState{
			TenantID:   key.TenantID,
			ActivityID: key.ActivityID,
			Namespace:  key.Namespace,
			StateKey:   key.StateKey,
			Data:       json.RawMessage(`{"streakDays":3}`),
			UpdatedAt:  now.Add(2 * time.Minute),
		},
		ExpectedVersion: 1,
		RequireVersion:  true,
		Merge:           true,
	})
	if !errors.Is(err, marketing.ErrActivityStateConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestPostgresStorageStateReadUpsertAndCAS(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_storage_state"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	key := marketing.StorageStateKey{
		TenantID:   "site_001",
		Namespace:  "checkin",
		StorageKey: prefix + "_user_1",
	}
	if _, err := repo.GetStorageState(ctx, key); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected missing storage state, got %v", err)
	}

	created, err := repo.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
		State: marketing.StorageState{
			ID:          prefix + "_state",
			TenantID:    key.TenantID,
			Namespace:   key.Namespace,
			StorageKey:  key.StorageKey,
			ValueType:   "TABLE",
			Data:        json.RawMessage(`[{"day":"2026-06-14","reward":10}]`),
			LastEventID: prefix + "_evt_1",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	})
	if err != nil {
		t.Fatalf("create storage state: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("expected version 1, got %+v", created)
	}

	updated, err := repo.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
		State: marketing.StorageState{
			TenantID:    key.TenantID,
			Namespace:   key.Namespace,
			StorageKey:  key.StorageKey,
			ValueType:   "TABLE",
			Data:        json.RawMessage(`[{"day":"2026-06-15","reward":20}]`),
			LastEventID: prefix + "_evt_2",
			UpdatedAt:   now.Add(time.Minute),
		},
		ExpectedVersion: 1,
		RequireVersion:  true,
	})
	if err != nil {
		t.Fatalf("cas update storage state: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %+v", updated)
	}

	_, err = repo.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
		State: marketing.StorageState{
			TenantID:   key.TenantID,
			Namespace:  key.Namespace,
			StorageKey: key.StorageKey,
			ValueType:  "TABLE",
			Data:       json.RawMessage(`[{"day":"2026-06-16","reward":30}]`),
			UpdatedAt:  now.Add(2 * time.Minute),
		},
		ExpectedVersion: 1,
		RequireVersion:  true,
	})
	if !errors.Is(err, marketing.ErrStorageStateConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}

	if err := repo.DeleteStorageState(ctx, key); err != nil {
		t.Fatalf("delete storage state: %v", err)
	}
	if _, err := repo.GetStorageState(ctx, key); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected deleted storage state, got %v", err)
	}
}

func TestPostgresStorageStateConcurrentFirstAppend(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_storage_append"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	key := marketing.StorageStateKey{
		TenantID:   "site_001",
		Namespace:  "checkin",
		StorageKey: prefix + "_events",
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, value := range []string{"first", "second"} {
		value := value
		go func() {
			<-start
			_, err := repo.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
				State: marketing.StorageState{
					ID:          prefix + "_" + value,
					TenantID:    key.TenantID,
					Namespace:   key.Namespace,
					StorageKey:  key.StorageKey,
					ValueType:   "ARRAY",
					Data:        json.RawMessage(`"` + value + `"`),
					LastEventID: prefix + "_evt_" + value,
					CreatedAt:   time.Now().UTC(),
					UpdatedAt:   time.Now().UTC(),
				},
				Append: true,
			})
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("append %d failed: %v", i, err)
		}
	}
	state, err := repo.GetStorageState(ctx, key)
	if err != nil {
		t.Fatalf("get appended storage state: %v", err)
	}
	var items []string
	if err := json.Unmarshal(state.Data, &items); err != nil {
		t.Fatalf("decode appended state: %v", err)
	}
	if len(items) != 2 || !slices.Contains(items, "first") || !slices.Contains(items, "second") {
		t.Fatalf("expected both concurrent append values, got %+v", items)
	}
}

func TestPostgresReconciliationIncludesParticipationWithoutGrantOrder(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_part_only_reconcile"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	inbox := marketing.EventInbox{
		ID:             prefix + "_part_inbox",
		IdempotencyKey: prefix + "_inbox_idem",
		ProcessStatus:  marketing.InboxProcessed,
		Event: marketing.ActivityEvent{
			TenantID:     "site_001",
			EventID:      prefix + "_event",
			EventType:    "CHECKIN_SUCCESS",
			SourceSystem: "app",
			Subject:      marketing.EventSubject{Type: "USER", ID: "user_1"},
			Biz:          marketing.EventBiz{BizType: "CHECKIN", BizID: prefix + "_day"},
			OccurredAt:   now,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, _, err := repo.CreateInbox(ctx, inbox); err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	participation := testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now)
	participation.EventInboxID = inbox.ID
	participation.Subject = inbox.Event.Subject
	participation.Biz = inbox.Event.Biz
	if _, err := repo.CreateParticipation(ctx, participation); err != nil {
		t.Fatalf("create participation: %v", err)
	}
	items, err := repo.ListRewardReconciliation(ctx, marketing.RewardReconciliationQuery{
		TenantID:   "site_001",
		ActivityID: activityID,
		EventID:    inbox.Event.EventID,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("list reconciliation: %v", err)
	}
	if len(items) != 1 || items[0].ParticipationID != participation.ID || items[0].Participation == nil || items[0].GrantOrderID != "" {
		t.Fatalf("expected participation-only reconciliation item, got %+v", items)
	}
}

func TestPostgresCloseOutboxDLQOnlyClosesOutbox(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_dlq_close"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	createPostgresAtomicQuotas(t, repo, activityID, prefix, now)
	grant := testPostgresGrant(prefix+"_grant", activityID, prefix+"_part", prefix+"_grant_idem", now)
	grant.RewardAmount = "10"
	outbox := testPostgresOutbox(prefix+"_outbox", now)
	outbox.Payload = []byte(`{"commandId":"` + grant.BillNo + `"}`)
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now),
		BudgetAmount:   "10",
		GrantOrders:    []marketing.GrantOrder{grant},
		OutboxMessages: []marketing.OutboxMessage{outbox},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(ctx, "site_001", grant.BillNo, marketing.GrantSending, "sending"); err != nil {
		t.Fatalf("mark sending: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(ctx, "site_001", grant.BillNo, marketing.GrantRetry, "target returned status 400"); err != nil {
		t.Fatalf("mark retry: %v", err)
	}
	if err := repo.MarkOutboxDLQ(ctx, outbox.ID, 5, "target returned status 400"); err != nil {
		t.Fatalf("mark dlq: %v", err)
	}
	closed, err := repo.CloseOutboxDLQ(ctx, "site_001", outbox.ID, "ops", "business rejected", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("close dlq: %v", err)
	}
	if closed.SendStatus != marketing.OutboxDLQClosed {
		t.Fatalf("expected DLQ_CLOSED, got %s", closed.SendStatus)
	}
	assertPostgresGrantStatus(t, repo, "site_001", grant.BillNo, marketing.GrantRetry)
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "10.0000")
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1 AND reservation_status = 'FROZEN'`, activityID, 1)
	assertPostgresParticipationStatus(t, repo, prefix+"_part", marketing.ParticipationRewardable)
}

func TestPostgresOutboxRetryEligibilityAndTerminalStatusNotClaimed(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_outbox_retry_terminal"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC().Add(-time.Hour)
	futureRetryAt := time.Now().UTC().Add(time.Hour)
	eligibleRetryAt := time.Now().UTC().Add(-time.Minute)
	future := testPostgresOutbox(prefix+"_future_retry", now)
	future.SendStatus = marketing.OutboxFailed
	future.RetryCount = 1
	future.NextRetryAt = &futureRetryAt
	eligible := testPostgresOutbox(prefix+"_eligible_retry", now.Add(time.Millisecond))
	eligible.SendStatus = marketing.OutboxFailed
	eligible.RetryCount = 1
	eligible.NextRetryAt = &eligibleRetryAt
	terminalSent := testPostgresOutbox(prefix+"_sent", now.Add(2*time.Millisecond))
	terminalSent.SendStatus = marketing.OutboxSent
	terminalDLQ := testPostgresOutbox(prefix+"_dlq", now.Add(3*time.Millisecond))
	terminalDLQ.SendStatus = marketing.OutboxDLQ
	terminalClosed := testPostgresOutbox(prefix+"_dlq_closed", now.Add(4*time.Millisecond))
	terminalClosed.SendStatus = marketing.OutboxDLQClosed

	for _, message := range []marketing.OutboxMessage{future, eligible, terminalSent, terminalDLQ, terminalClosed} {
		if _, err := repo.CreateOutbox(ctx, message); err != nil {
			t.Fatalf("create outbox %s: %v", message.ID, err)
		}
	}
	before := postgresOutboxUpdatedAt(t, repo, prefix+"_sent")

	claimed, err := repo.ListPendingOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("list pending outbox: %v", err)
	}
	if got := outboxIDsWithPrefix(claimed, prefix); len(got) != 1 || got[0] != prefix+"_eligible_retry" {
		t.Fatalf("expected only eligible retry to be claimed, got %v", got)
	}
	if got := postgresOutboxStatus(t, repo, prefix+"_future_retry"); got != marketing.OutboxFailed {
		t.Fatalf("expected future retry to remain FAILED, got %s", got)
	}
	expectedTerminal := map[string]marketing.OutboxStatus{
		prefix + "_sent":       marketing.OutboxSent,
		prefix + "_dlq":        marketing.OutboxDLQ,
		prefix + "_dlq_closed": marketing.OutboxDLQClosed,
	}
	for id, want := range expectedTerminal {
		if got := postgresOutboxStatus(t, repo, id); got != want {
			t.Fatalf("expected terminal status %s for %s, got %s", want, id, got)
		}
	}
	if after := postgresOutboxUpdatedAt(t, repo, prefix+"_sent"); !after.Equal(before) {
		t.Fatalf("expected terminal outbox updated_at unchanged, before=%s after=%s", before, after)
	}
}

func TestPostgresOutboxConcurrentClaimSingleMessage(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_outbox_claim_single"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	if _, err := repo.CreateOutbox(ctx, testPostgresOutbox(prefix+"_outbox", time.Now().UTC())); err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	claimedIDs := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			messages, err := repo.ListPendingOutbox(ctx, 1)
			if err != nil {
				errs <- err
				return
			}
			for _, message := range messages {
				claimedIDs <- message.ID
			}
		}()
	}
	close(start)
	wg.Wait()
	close(claimedIDs)
	close(errs)
	for err := range errs {
		t.Fatalf("claim outbox: %v", err)
	}
	var ids []string
	for id := range claimedIDs {
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != prefix+"_outbox" {
		t.Fatalf("expected one worker to claim the outbox once, got %v", ids)
	}
	if got := postgresOutboxStatus(t, repo, prefix+"_outbox"); got != marketing.OutboxSending {
		t.Fatalf("expected claimed outbox SENDING, got %s", got)
	}
}

func TestPostgresOutboxConcurrentClaimMultipleMessages(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_outbox_claim_many"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC().Add(-time.Minute)
	const total = 12
	for i := 0; i < total; i++ {
		message := testPostgresOutbox(fmt.Sprintf("%s_outbox_%02d", prefix, i), now.Add(time.Duration(i)*time.Millisecond))
		if i%2 == 0 {
			message.TenantID = "tenant_a"
		} else {
			message.TenantID = "tenant_b"
			message.TargetChannel = marketing.GrantMQ
			message.TargetRef = "kafka://reward.commands"
		}
		if _, err := repo.CreateOutbox(ctx, message); err != nil {
			t.Fatalf("create outbox %s: %v", message.ID, err)
		}
	}

	const workers = 4
	start := make(chan struct{})
	claimedIDs := make(chan string, total)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			messages, err := repo.ListPendingOutbox(ctx, 3)
			if err != nil {
				errs <- err
				return
			}
			for _, message := range messages {
				claimedIDs <- message.ID
			}
		}()
	}
	close(start)
	wg.Wait()
	close(claimedIDs)
	close(errs)
	for err := range errs {
		t.Fatalf("claim outbox: %v", err)
	}

	seen := map[string]int{}
	for id := range claimedIDs {
		seen[id]++
	}
	if len(seen) != total {
		t.Fatalf("expected %d claimed outboxes, got %d: %v", total, len(seen), seen)
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("expected outbox %s claimed once, got %d", id, count)
		}
		if got := postgresOutboxStatus(t, repo, id); got != marketing.OutboxSending {
			t.Fatalf("expected outbox %s SENDING, got %s", id, got)
		}
	}
}

func TestPostgresOutboxStaleRecoveryConcurrentClaim(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := fmt.Sprintf("pg_outbox_stale_concurrent_%d", time.Now().UnixNano())
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	staleAt := time.Unix(1, 0).UTC()
	stale := testPostgresOutbox(prefix+"_stale", staleAt)
	stale.SendStatus = marketing.OutboxSending
	stale.UpdatedAt = staleAt
	if _, err := repo.CreateOutbox(ctx, stale); err != nil {
		t.Fatalf("create stale outbox: %v", err)
	}

	const attempts = 2
	start := make(chan struct{})
	recoveredIDs := make(chan string, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			messages, err := repo.RecoverStaleOutbox(ctx, time.Now().UTC().Add(-5*time.Minute), 1)
			if err != nil {
				errs <- err
				return
			}
			for _, message := range messages {
				recoveredIDs <- message.ID
			}
		}()
	}
	close(start)
	wg.Wait()
	close(recoveredIDs)
	close(errs)
	for err := range errs {
		t.Fatalf("recover stale outbox: %v", err)
	}
	var ids []string
	for id := range recoveredIDs {
		if strings.HasPrefix(id, prefix) {
			ids = append(ids, id)
		}
	}
	if len(ids) != 1 || ids[0] != prefix+"_stale" {
		t.Fatalf("expected one stale recovery, got %v", ids)
	}
	claimed, err := repo.ListPendingOutbox(ctx, 1)
	if err != nil {
		t.Fatalf("claim recovered stale outbox: %v", err)
	}
	if got := outboxIDsWithPrefix(claimed, prefix); len(got) != 1 || got[0] != prefix+"_stale" {
		t.Fatalf("expected recovered outbox to be claimable once, got %v", got)
	}
}

func TestPostgresOutboxDLQCountByTenantAndChannel(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	prefix := "pg_outbox_dlq_count"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	messages := []marketing.OutboxMessage{
		testPostgresOutbox(prefix+"_a_http_1", now),
		testPostgresOutbox(prefix+"_a_http_2", now.Add(time.Millisecond)),
		testPostgresOutbox(prefix+"_a_mq", now.Add(2*time.Millisecond)),
		testPostgresOutbox(prefix+"_b_http", now.Add(3*time.Millisecond)),
		testPostgresOutbox(prefix+"_closed", now.Add(4*time.Millisecond)),
	}
	for i := range messages {
		messages[i].SendStatus = marketing.OutboxDLQ
	}
	messages[0].TenantID = "tenant_a"
	messages[1].TenantID = "tenant_a"
	messages[2].TenantID = "tenant_a"
	messages[2].TargetChannel = marketing.GrantMQ
	messages[2].TargetRef = "kafka://reward.commands"
	messages[3].TenantID = "tenant_b"
	messages[4].TenantID = "tenant_a"
	messages[4].SendStatus = marketing.OutboxDLQClosed
	for _, message := range messages {
		if _, err := repo.CreateOutbox(ctx, message); err != nil {
			t.Fatalf("create outbox %s: %v", message.ID, err)
		}
	}

	counts, err := repo.CountOutboxDLQ(ctx)
	if err != nil {
		t.Fatalf("count outbox dlq: %v", err)
	}
	got := map[string]int{}
	for _, count := range counts {
		if strings.HasPrefix(count.TenantID, "tenant_") {
			got[count.TenantID+"|"+string(count.TargetChannel)] = count.Count
		}
	}
	if got["tenant_a|HTTP"] != 2 || got["tenant_a|MQ"] != 1 || got["tenant_b|HTTP"] != 1 {
		t.Fatalf("unexpected DLQ counts: %+v", got)
	}
}

func TestPostgresAtomicGrantRollsBackWhenGrantOrderCreateFails(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_grant_insert_rollback"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	createPostgresAtomicQuotas(t, repo, activityID, prefix, now)

	_, err = repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now),
		BudgetAmount:   "2",
		GrantOrders:    []marketing.GrantOrder{testPostgresGrant(prefix+"_grant_1", activityID, prefix+"_part", prefix+"_grant_idem", now), testPostgresGrant(prefix+"_grant_2", activityID, prefix+"_part", prefix+"_grant_idem", now)},
		OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(prefix+"_outbox_1", now)},
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
	})
	if err == nil {
		t.Fatal("expected duplicate grant order idempotency key to fail atomic grant")
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value <> 0`, activityID, 0)
}

func TestPostgresAtomicGrantRollsBackWhenOutboxCreateFails(t *testing.T) {
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	prefix := "pg_outbox_insert_rollback"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	createPostgresAtomicQuotas(t, repo, activityID, prefix, now)
	badOutbox := testPostgresOutbox(prefix+"_outbox_1", now)
	badOutbox.Payload = []byte(`{`)

	_, err = repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now),
		BudgetAmount:   "2",
		GrantOrders:    []marketing.GrantOrder{testPostgresGrant(prefix+"_grant_1", activityID, prefix+"_part", prefix+"_grant_idem", now)},
		OutboxMessages: []marketing.OutboxMessage{badOutbox},
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid outbox payload to fail atomic grant")
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value <> 0`, activityID, 0)
}

func TestPostgresQuotaInsufficientBudgetHasNoDirtyWrites(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	prefix := "pg_quota_budget_dirty"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	mustCreatePostgresQuota(t, repo, marketing.Quota{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "10", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})

	_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now),
		BudgetAmount:   "20",
		GrantOrders:    []marketing.GrantOrder{testPostgresGrant(prefix+"_grant", activityID, prefix+"_part", prefix+"_grant_idem", now)},
		OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(prefix+"_outbox", now)},
	})
	if !errors.Is(err, marketing.ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}

	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_participation WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1`, activityID, 0)
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "0.0000")
}

func TestPostgresCallbackConfirmReleaseIdempotencyAndConflict(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	now := time.Now().UTC()
	successPrefix := "pg_callback_success"
	cleanupPostgresAtomicTest(t, repo, successPrefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, successPrefix) })
	successActivityID := successPrefix + "_act"
	createPostgresCallbackQuotas(t, repo, successActivityID, successPrefix, "user_1", "user_1:2026-05-24", now)

	successGrant := testPostgresGrant(successPrefix+"_grant", successActivityID, successPrefix+"_part", successPrefix+"_grant_idem", now)
	successGrant.RewardAmount = "10"
	successGrant.RewardPayload = []byte(`{"rewardCode":"REWARD_A"}`)
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:      testPostgresParticipation(successPrefix+"_part", successActivityID, successPrefix+"_part_idem", now),
		BudgetAmount:       "10",
		ReserveUserLimit:   true,
		UserScopeKey:       "user_1",
		ReserveDailyLimit:  true,
		DailyLimitScopeKey: "user_1:2026-05-24",
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
		GrantOrders:    []marketing.GrantOrder{successGrant},
		OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(successPrefix+"_outbox", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant success path: %v", err)
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1 AND reservation_status = 'FROZEN'`, successActivityID, 4)
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "10.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "REWARD_A", "0.0000", "1.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "user_1", "0.0000", "1.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, "user_1:2026-05-24", "0.0000", "1.0000")

	if err := repo.UpdateGrantOrderByCommandID(ctx, "site_001", successGrant.BillNo, marketing.GrantSending, "sending"); err != nil {
		t.Fatalf("mark grant sending: %v", err)
	}
	result, err := repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:        "site_001",
		CommandID:       successGrant.BillNo,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: successPrefix + "_external",
		FinishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("success callback: %v", err)
	}
	if result.Duplicate || result.ParticipationStatus != marketing.ParticipationGranted {
		t.Fatalf("unexpected success callback result: %+v", result)
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1 AND reservation_status = 'CONFIRMED'`, successActivityID, 4)
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "REWARD_A", "1.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "user_1", "1.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, successActivityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, "user_1:2026-05-24", "1.0000", "0.0000")
	successVersion := postgresQuotaVersion(t, repo, successActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL")

	result, err = repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:        "site_001",
		CommandID:       successGrant.BillNo,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: successPrefix + "_external",
		FinishedAt:      now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("duplicate success callback: %v", err)
	}
	if !result.Duplicate {
		t.Fatalf("expected duplicate success callback result: %+v", result)
	}
	if got := postgresQuotaVersion(t, repo, successActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL"); got != successVersion {
		t.Fatalf("duplicate success callback changed budget version: before=%d after=%d", successVersion, got)
	}
	if _, err := repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:   "site_001",
		CommandID:  successGrant.BillNo,
		Status:     marketing.GrantFailed,
		Message:    "late failure",
		FinishedAt: now.Add(3 * time.Minute),
	}); !errors.Is(err, marketing.ErrCallbackConflict) {
		t.Fatalf("expected success-to-failed callback conflict, got %v", err)
	}

	releasePrefix := "pg_callback_release"
	cleanupPostgresAtomicTest(t, repo, releasePrefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, releasePrefix) })
	releaseActivityID := releasePrefix + "_act"
	mustCreatePostgresQuota(t, repo, marketing.Quota{ID: releasePrefix + "_budget", ActivityID: releaseActivityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "20", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})
	releaseGrant := testPostgresGrant(releasePrefix+"_grant", releaseActivityID, releasePrefix+"_part", releasePrefix+"_grant_idem", now)
	releaseGrant.RewardAmount = "10"
	_, err = repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:  testPostgresParticipation(releasePrefix+"_part", releaseActivityID, releasePrefix+"_part_idem", now),
		BudgetAmount:   "10",
		GrantOrders:    []marketing.GrantOrder{releaseGrant},
		OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(releasePrefix+"_outbox", now)},
	})
	if err != nil {
		t.Fatalf("atomic grant release path: %v", err)
	}
	if err := repo.UpdateGrantOrderByCommandID(ctx, "site_001", releaseGrant.BillNo, marketing.GrantSending, "sending"); err != nil {
		t.Fatalf("mark release grant sending: %v", err)
	}
	result, err = repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:   "site_001",
		CommandID:  releaseGrant.BillNo,
		Status:     marketing.GrantFailed,
		Message:    "failed",
		FinishedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("failed callback: %v", err)
	}
	if result.ParticipationStatus != marketing.ParticipationFailed {
		t.Fatalf("expected failed participation, got %+v", result)
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1 AND reservation_status = 'RELEASED'`, releaseActivityID, 1)
	assertPostgresQuotaValues(t, repo, releaseActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "0.0000")
	releaseVersion := postgresQuotaVersion(t, repo, releaseActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL")

	result, err = repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:   "site_001",
		CommandID:  releaseGrant.BillNo,
		Status:     marketing.GrantFailed,
		Message:    "failed retry",
		FinishedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("duplicate failed callback: %v", err)
	}
	if !result.Duplicate {
		t.Fatalf("expected duplicate failed callback result: %+v", result)
	}
	if got := postgresQuotaVersion(t, repo, releaseActivityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL"); got != releaseVersion {
		t.Fatalf("duplicate failed callback changed budget version: before=%d after=%d", releaseVersion, got)
	}
}

func TestPostgresCallbackTenantIsolation(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	prefix := "pg_callback_tenant"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	commandID := prefix + "_shared_cmd"
	for _, tenantID := range []string{"tenant_a", "tenant_b"} {
		suffix := strings.TrimPrefix(tenantID, "tenant_")
		activityID := prefix + "_" + suffix + "_act"
		mustCreatePostgresQuota(t, repo, marketing.Quota{ID: prefix + "_" + suffix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "20", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})
		part := testPostgresParticipation(prefix+"_"+suffix+"_part", activityID, prefix+"_"+suffix+"_part_idem", now)
		part.TenantID = tenantID
		grant := testPostgresGrant(prefix+"_"+suffix+"_grant", activityID, part.ID, prefix+"_"+suffix+"_grant_idem", now)
		grant.TenantID = tenantID
		grant.BillNo = commandID
		grant.RewardAmount = "10"
		outbox := testPostgresOutbox(prefix+"_"+suffix+"_outbox", now)
		outbox.TenantID = tenantID
		_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
			Participation:  part,
			BudgetAmount:   "10",
			GrantOrders:    []marketing.GrantOrder{grant},
			OutboxMessages: []marketing.OutboxMessage{outbox},
		})
		if err != nil {
			t.Fatalf("atomic grant tenant %s: %v", tenantID, err)
		}
		if err := repo.UpdateGrantOrderByCommandID(ctx, tenantID, commandID, marketing.GrantSending, "sending"); err != nil {
			t.Fatalf("mark tenant %s sending: %v", tenantID, err)
		}
	}

	if _, err := repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:        "tenant_a",
		CommandID:       commandID,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: prefix + "_external_a",
		FinishedAt:      now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("tenant A callback: %v", err)
	}

	assertPostgresGrantStatus(t, repo, "tenant_a", commandID, marketing.GrantSuccess)
	assertPostgresGrantStatus(t, repo, "tenant_b", commandID, marketing.GrantSending)
	assertPostgresQuotaValues(t, repo, prefix+"_a_act", marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, prefix+"_b_act", marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "10.0000")

	if _, err := repo.ApplyRewardCallback(ctx, marketing.RewardCallback{
		TenantID:        "tenant_missing",
		CommandID:       commandID,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: prefix + "_external_missing",
		FinishedAt:      now.Add(2 * time.Minute),
	}); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected tenant-missing callback not found, got %v", err)
	}
	assertPostgresGrantStatus(t, repo, "tenant_b", commandID, marketing.GrantSending)
	assertPostgresQuotaValues(t, repo, prefix+"_b_act", marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "10.0000")
}

func TestPostgresConcurrentBudgetDoesNotOverspend(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	prefix := "pg_concurrent_budget"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	mustCreatePostgresQuota(t, repo, marketing.Quota{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})

	const attempts = 50
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			suffix := fmt.Sprintf("%02d", i)
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
				Participation:  testPostgresParticipation(prefix+"_part_"+suffix, activityID, prefix+"_part_idem_"+suffix, now),
				BudgetAmount:   "10",
				GrantOrders:    []marketing.GrantOrder{testPostgresGrant(prefix+"_grant_"+suffix, activityID, prefix+"_part_"+suffix, prefix+"_grant_idem_"+suffix, now)},
				OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(prefix+"_outbox_"+suffix, now)},
			})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	success, quotaExceeded := countPostgresAtomicResults(t, results)
	if success != 10 || quotaExceeded != attempts-10 {
		t.Fatalf("expected 10 successes and %d quota failures, got success=%d quotaExceeded=%d", attempts-10, success, quotaExceeded)
	}
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "100.0000")
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE activity_id = $1`, activityID, 10)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 10)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1`, activityID, 10)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value + frozen_value > total_value`, activityID, 0)
}

func TestPostgresConcurrentStockLastItem(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	prefix := "pg_concurrent_stock"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	mustCreatePostgresQuota(t, repo, marketing.Quota{ID: prefix + "_stock", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})

	const attempts = 20
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			suffix := fmt.Sprintf("%02d", i)
			grant := testPostgresGrant(prefix+"_grant_"+suffix, activityID, prefix+"_part_"+suffix, prefix+"_grant_idem_"+suffix, now)
			grant.RewardAmount = "0"
			grant.RewardPayload = []byte(`{"rewardCode":"REWARD_A"}`)
			_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
				Participation: testPostgresParticipation(prefix+"_part_"+suffix, activityID, prefix+"_part_idem_"+suffix, now),
				StockReservations: []marketing.QuotaReservation{
					{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
				},
				GrantOrders:    []marketing.GrantOrder{grant},
				OutboxMessages: []marketing.OutboxMessage{testPostgresOutbox(prefix+"_outbox_"+suffix, now)},
			})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	success, quotaExceeded := countPostgresAtomicResults(t, results)
	if success != 1 || quotaExceeded != attempts-1 {
		t.Fatalf("expected 1 success and %d quota failures, got success=%d quotaExceeded=%d", attempts-1, success, quotaExceeded)
	}
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "REWARD_A", "0.0000", "1.0000")
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_grant_order WHERE activity_id = $1`, activityID, 1)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 1)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1`, activityID, 1)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota WHERE activity_id = $1 AND used_value + frozen_value > total_value`, activityID, 0)
}

func TestPostgresManualExportReadySuccessPersistsTraceAuditAndQuota(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)
	service := engine.NewService(repo)

	prefix := "pg_manual_export_ready"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	createPostgresCallbackQuotas(t, repo, activityID, prefix, "user_1", "user_1:2026-05-29", now)
	grant := testPostgresManualGrant(prefix+"_grant", activityID, prefix+"_part", prefix+"_grant_idem", "REWARD_A", "10", marketing.GrantManualExport, now)

	_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation:      testPostgresParticipation(prefix+"_part", activityID, prefix+"_part_idem", now),
		BudgetAmount:       "10",
		ReserveUserLimit:   true,
		UserScopeKey:       "user_1",
		ReserveDailyLimit:  true,
		DailyLimitScopeKey: "user_1:2026-05-29",
		StockReservations: []marketing.QuotaReservation{
			{Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", Amount: "1"},
		},
		GrantOrders: []marketing.GrantOrder{grant},
	})
	if err != nil {
		t.Fatalf("atomic manual export grant: %v", err)
	}
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_outbox WHERE id LIKE $1`, prefix+"_%", 0)

	_, err = service.ApplyManualGrantDecision(ctx, marketing.ManualGrantDecisionCommand{
		TenantID:        "site_001",
		CommandID:       grant.BillNo,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: prefix + "_external",
		Operator:        "ops",
		Reason:          "paid before export",
		FinishedAt:      now.Add(time.Minute),
	})
	if !errors.Is(err, marketing.ErrManualGrantExportRequired) {
		t.Fatalf("expected manual export required before READY file, got %v", err)
	}

	exportResult, err := service.CreateManualGrantExport(ctx, marketing.ManualGrantExportCreateCommand{
		TenantID:   "site_001",
		Operator:   "ops",
		Reason:     "daily offline file",
		ActivityID: activityID,
	})
	if err != nil {
		t.Fatalf("create manual export: %v", err)
	}
	if exportResult.ExportFile.Status != marketing.ManualGrantExportReady || exportResult.ExportFile.RowCount != 1 || exportResult.ExportFile.FileChecksum == "" {
		t.Fatalf("unexpected export file: %+v", exportResult.ExportFile)
	}
	if len(exportResult.Details) != 1 || exportResult.Details[0].GrantOrderID != grant.ID {
		t.Fatalf("unexpected export details: %+v", exportResult.Details)
	}
	assertPostgresGrantStatus(t, repo, "site_001", grant.BillNo, marketing.GrantSending)
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM marketing_manual_grant_export_detail d
		JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		WHERE d.grant_order_id = $1 AND d.detail_status = 'READY' AND f.status = 'READY'
	`, grant.ID, 1)

	result, err := service.ApplyManualGrantDecision(ctx, marketing.ManualGrantDecisionCommand{
		TenantID:        "site_001",
		CommandID:       grant.BillNo,
		Status:          marketing.GrantSuccess,
		ExternalGrantID: prefix + "_external",
		Operator:        "ops",
		Reason:          "offline paid",
		FinishedAt:      now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("manual export success: %v", err)
	}
	if result.GrantStatus != marketing.GrantSuccess || result.ParticipationStatus != marketing.ParticipationGranted {
		t.Fatalf("unexpected manual export success result: %+v", result)
	}
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "REWARD_A", "1.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "user_1", "1.0000", "0.0000")
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, "user_1:2026-05-29", "1.0000", "0.0000")
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_quota_reservation WHERE activity_id = $1 AND reservation_status = 'CONFIRMED'`, activityID, 4)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_audit WHERE resource_id = $1 AND action = 'MANUAL_GRANT_EXPORT_CREATE'`, exportResult.ExportFile.ID, 1)
	assertPostgresCount(t, repo, `SELECT COUNT(*) FROM marketing_audit WHERE resource_id = $1 AND action = 'MANUAL_GRANT_DECISION'`, grant.ID, 1)
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM marketing_manual_grant_export_detail d
		JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		WHERE d.grant_order_id = $1 AND f.file_checksum IS NOT NULL AND f.status = 'READY'
	`, grant.ID, 1)
}

func TestPostgresManualExportConcurrentUniqueInclusionAndFailedHistoryReexport(t *testing.T) {
	ctx, repo := openPostgresIntegrationRepository(t)

	prefix := "pg_manual_export_concurrent"
	cleanupPostgresAtomicTest(t, repo, prefix)
	t.Cleanup(func() { cleanupPostgresAtomicTest(t, repo, prefix) })

	now := time.Now().UTC()
	activityID := prefix + "_act"
	mustCreatePostgresQuota(t, repo, marketing.Quota{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now})

	var grants []marketing.GrantOrder
	for i := 0; i < 3; i++ {
		suffix := fmt.Sprintf("%02d", i)
		grant := testPostgresManualGrant(prefix+"_grant_"+suffix, activityID, prefix+"_part_"+suffix, prefix+"_grant_idem_"+suffix, "REWARD_"+suffix, "5", marketing.GrantManualExport, now.Add(time.Duration(i)*time.Millisecond))
		grants = append(grants, grant)
		_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
			Participation: testPostgresParticipation(prefix+"_part_"+suffix, activityID, prefix+"_part_idem_"+suffix, now),
			BudgetAmount:  "5",
			GrantOrders:   []marketing.GrantOrder{grant},
		})
		if err != nil {
			t.Fatalf("atomic grant %s: %v", suffix, err)
		}
	}

	const attempts = 2
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := repo.CreateManualGrantExport(ctx, marketing.ManualGrantExportCreateCommand{
				TenantID:   "site_001",
				Operator:   fmt.Sprintf("ops_%d", i),
				Reason:     "concurrent export",
				ActivityID: activityID,
			})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var success, notFound int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, marketing.ErrNotFound):
			notFound++
		default:
			t.Fatalf("unexpected concurrent export error: %v", err)
		}
	}
	if success != 1 || notFound != attempts-1 {
		t.Fatalf("expected one export success and %d not found, got success=%d notFound=%d", attempts-1, success, notFound)
	}
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM marketing_manual_grant_export_detail d
		JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		WHERE d.grant_order_id LIKE $1 AND f.status <> 'FAILED'
	`, prefix+"_grant_%", len(grants))
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM (
			SELECT d.grant_order_id
			FROM marketing_manual_grant_export_detail d
			JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
			WHERE d.grant_order_id LIKE $1 AND f.status <> 'FAILED'
			GROUP BY d.grant_order_id
			HAVING COUNT(*) > 1
		) duplicates
	`, prefix+"_grant_%", 0)

	retryGrant := testPostgresManualGrant(prefix+"_retry_grant", activityID, prefix+"_retry_part", prefix+"_retry_grant_idem", "REWARD_RETRY", "5", marketing.GrantManualExport, now.Add(time.Second))
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(ctx, marketing.AtomicGrantCommand{
		Participation: testPostgresParticipation(prefix+"_retry_part", activityID, prefix+"_retry_part_idem", now),
		BudgetAmount:  "5",
		GrantOrders:   []marketing.GrantOrder{retryGrant},
	})
	if err != nil {
		t.Fatalf("atomic retry grant: %v", err)
	}
	if err := insertPostgresFailedManualExportHistory(ctx, repo, prefix, retryGrant, now.Add(2*time.Second)); err != nil {
		t.Fatalf("insert failed export history: %v", err)
	}
	assertPostgresGrantStatus(t, repo, "site_001", retryGrant.BillNo, marketing.GrantPending)
	assertPostgresQuotaValues(t, repo, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "0.0000", "20.0000")

	reexport, err := repo.CreateManualGrantExport(ctx, marketing.ManualGrantExportCreateCommand{
		TenantID:      "site_001",
		Operator:      "ops_retry",
		Reason:        "retry failed file",
		GrantOrderIDs: []string{retryGrant.ID},
	})
	if err != nil {
		t.Fatalf("re-export after failed history: %v", err)
	}
	if reexport.ExportFile.Status != marketing.ManualGrantExportReady || len(reexport.Details) != 1 || reexport.Details[0].GrantOrderID != retryGrant.ID {
		t.Fatalf("unexpected reexport result: %+v", reexport)
	}
	assertPostgresGrantStatus(t, repo, "site_001", retryGrant.BillNo, marketing.GrantSending)
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM marketing_manual_grant_export_detail d
		JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		WHERE d.grant_order_id = $1 AND f.status = 'FAILED'
	`, retryGrant.ID, 1)
	assertPostgresCount(t, repo, `
		SELECT COUNT(*)
		FROM marketing_manual_grant_export_detail d
		JOIN marketing_manual_grant_export_file f ON f.id = d.export_id
		WHERE d.grant_order_id = $1 AND f.status <> 'FAILED'
	`, retryGrant.ID, 1)
}

func cleanupPostgresAtomicTest(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM marketing_resource_audit WHERE id LIKE $1 OR resource_id LIKE $1 OR resource_code LIKE $1`,
		`DELETE FROM marketing_resource_binding WHERE id LIKE $1 OR resource_id LIKE $1`,
		`DELETE FROM marketing_resource_secret WHERE id LIKE $1 OR resource_id LIKE $1`,
		`DELETE FROM marketing_resource_schema WHERE id LIKE $1 OR resource_id LIKE $1`,
		`DELETE FROM marketing_resource WHERE id LIKE $1 OR code LIKE $1`,
		`DELETE FROM marketing_audit WHERE resource_id LIKE $1 OR payload_json::text LIKE '%' || $1 || '%'`,
		`DELETE FROM marketing_manual_grant_export_detail WHERE grant_order_id LIKE $1 OR export_id LIKE $1 OR id LIKE $1`,
		`DELETE FROM marketing_manual_grant_export_file WHERE id LIKE $1`,
		`DELETE FROM marketing_quota_reservation WHERE activity_id LIKE $1 OR grant_order_id LIKE $1 OR command_id LIKE $1`,
		`DELETE FROM marketing_outbox WHERE id LIKE $1`,
		`DELETE FROM marketing_grant_order WHERE id LIKE $1`,
		`DELETE FROM marketing_participation WHERE id LIKE $1`,
		`DELETE FROM marketing_quota WHERE id LIKE $1`,
		`DELETE FROM marketing_storage_state WHERE id LIKE $1 OR storage_key LIKE $1`,
		`DELETE FROM marketing_activity_state WHERE activity_id LIKE $1 OR id LIKE $1`,
		`DELETE FROM marketing_activity WHERE id LIKE $1`,
	} {
		if _, err := repo.db.ExecContext(ctx, stmt, prefix+"_%"); err != nil {
			t.Fatalf("cleanup %q: %v", stmt, err)
		}
	}
}

func openPostgresIntegrationRepository(t *testing.T) (context.Context, *Repository) {
	t.Helper()
	databaseURL := os.Getenv("EX_MARKETING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("EX_MARKETING_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repo, err := NewRepository(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return ctx, repo
}

func cleanupPostgresInboxTest(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(), `DELETE FROM marketing_event_inbox WHERE id LIKE $1 OR event_id LIKE $1`, prefix+"_%"); err != nil {
		t.Fatalf("cleanup inbox: %v", err)
	}
}

func cleanupPostgresActivityTest(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM marketing_reward WHERE activity_id LIKE $1`,
		`DELETE FROM marketing_rule WHERE activity_id LIKE $1`,
		`DELETE FROM marketing_activity WHERE id LIKE $1 OR activity_code LIKE $1`,
	} {
		if _, err := repo.db.ExecContext(ctx, stmt, prefix+"_%"); err != nil {
			t.Fatalf("cleanup activity %q: %v", stmt, err)
		}
	}
}

func cleanupPostgresWorkspaceTest(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM marketing_activity WHERE id LIKE $1 OR activity_code LIKE $1`,
		`DELETE FROM marketing_workspace_share_link WHERE id LIKE $1 OR token LIKE $1`,
		`DELETE FROM marketing_workspace_folder WHERE id LIKE $1`,
		`DELETE FROM marketing_workspace_audit_log WHERE workspace_id LIKE $1 OR id LIKE $1`,
		`DELETE FROM marketing_workspace_member WHERE workspace_id LIKE $1 OR id LIKE $1`,
		`DELETE FROM marketing_workspace WHERE id LIKE $1 OR slug LIKE $1`,
	} {
		if _, err := repo.db.ExecContext(ctx, stmt, prefix+"%"); err != nil {
			t.Fatalf("cleanup workspace %q: %v", stmt, err)
		}
	}
}

type postgresBatchFixture struct {
	now              time.Time
	workspace        marketing.Workspace
	parentFolder     marketing.WorkspaceFolder
	childFolder      marketing.WorkspaceFolder
	targetFolder     marketing.WorkspaceFolder
	activeWorkflow   marketing.Activity
	secondWorkflow   marketing.Activity
	archivedWorkflow marketing.Activity
}

func createPostgresBatchFixture(t *testing.T, ctx context.Context, repo *Repository, prefix string) postgresBatchFixture {
	t.Helper()
	now := time.Now().UTC()
	workspace := marketing.Workspace{
		ID:          prefix + "_workspace",
		TenantID:    "site_001",
		Name:        "Batch Workspace",
		Slug:        prefix + "-workspace",
		Visibility:  marketing.WorkspacePrivate,
		DefaultRole: marketing.WorkspaceRoleViewer,
		OwnerID:     prefix + "_owner",
		Status:      marketing.WorkspaceActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	parent := marketing.WorkspaceFolder{
		ID:          prefix + "_folder_parent",
		WorkspaceID: workspace.ID,
		Name:        "Parent",
		Visibility:  marketing.WorkspaceFolderTeam,
		Status:      marketing.WorkspaceFolderActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	child := parent
	child.ID = prefix + "_folder_child"
	child.ParentID = parent.ID
	child.Name = "Child"
	target := parent
	target.ID = prefix + "_folder_target"
	target.Name = "Target"
	for _, folder := range []marketing.WorkspaceFolder{parent, child, target} {
		if _, err := repo.CreateWorkspaceFolder(ctx, folder); err != nil {
			t.Fatalf("create folder %s: %v", folder.ID, err)
		}
	}
	active := testPostgresActivity(prefix+"_workflow_active", "site_001", prefix+"_active", now)
	active.WorkspaceID = workspace.ID
	active.FolderID = parent.ID
	active.Visibility = marketing.WorkspaceTeam
	second := testPostgresActivity(prefix+"_workflow_second", "site_001", prefix+"_second", now)
	second.WorkspaceID = workspace.ID
	second.FolderID = parent.ID
	second.Visibility = marketing.WorkspaceTeam
	archived := testPostgresActivity(prefix+"_workflow_archived", "site_001", prefix+"_archived", now)
	archived.WorkspaceID = workspace.ID
	archived.FolderID = parent.ID
	archived.Visibility = marketing.WorkspaceTeam
	archived.Status = marketing.ActivityArchived
	for _, activity := range []marketing.Activity{active, second, archived} {
		if _, err := repo.CreateActivity(ctx, activity); err != nil {
			t.Fatalf("create activity %s: %v", activity.ID, err)
		}
	}
	return postgresBatchFixture{
		now:              now,
		workspace:        workspace,
		parentFolder:     parent,
		childFolder:      child,
		targetFolder:     target,
		activeWorkflow:   active,
		secondWorkflow:   second,
		archivedWorkflow: archived,
	}
}

func assertPostgresBatchFailure(t *testing.T, result marketing.WorkspaceBatchResult, id string, contains string) {
	t.Helper()
	for _, item := range result.Items {
		if item.ID == id && item.Status == "FAILED" && strings.Contains(item.Error, contains) {
			return
		}
	}
	t.Fatalf("expected batch failure id=%s containing %q, got %+v", id, contains, result.Items)
}

func containsWorkspaceSearchResult(items []marketing.WorkspaceHomeSearchResult, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func assertPostgresActivityVisibility(t *testing.T, repo *Repository, activityID string, want marketing.WorkspaceVisibility) {
	t.Helper()
	var got marketing.WorkspaceVisibility
	if err := repo.db.QueryRowContext(context.Background(), `SELECT visibility FROM marketing_activity WHERE id = $1`, activityID).Scan(&got); err != nil {
		t.Fatalf("query activity visibility %s: %v", activityID, err)
	}
	if got != want {
		t.Fatalf("activity %s visibility got %s want %s", activityID, got, want)
	}
}

func assertPostgresActivityFolder(t *testing.T, repo *Repository, activityID string, want string) {
	t.Helper()
	var got string
	if err := repo.db.QueryRowContext(context.Background(), `SELECT COALESCE(folder_id, '') FROM marketing_activity WHERE id = $1`, activityID).Scan(&got); err != nil {
		t.Fatalf("query activity folder %s: %v", activityID, err)
	}
	if got != want {
		t.Fatalf("activity %s folder got %s want %s", activityID, got, want)
	}
}

func assertPostgresFolderParent(t *testing.T, repo *Repository, folderID string, want string) {
	t.Helper()
	var got string
	if err := repo.db.QueryRowContext(context.Background(), `SELECT COALESCE(parent_id, '') FROM marketing_workspace_folder WHERE id = $1`, folderID).Scan(&got); err != nil {
		t.Fatalf("query folder parent %s: %v", folderID, err)
	}
	if got != want {
		t.Fatalf("folder %s parent got %s want %s", folderID, got, want)
	}
}

func jsonNumber(value string) any {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		panic(err)
	}
	return out
}

func assertPostgresCount(t *testing.T, repo *Repository, query string, arg any, want int) {
	t.Helper()
	var got int
	if err := repo.db.QueryRowContext(context.Background(), query, arg).Scan(&got); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if got != want {
		t.Fatalf("expected count %d, got %d for %s", want, got, query)
	}
}

func assertPostgresQuotaValues(t *testing.T, repo *Repository, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, wantUsed string, wantFrozen string) {
	t.Helper()
	var used string
	var frozen string
	err := repo.db.QueryRowContext(context.Background(), `
		SELECT used_value::numeric(18,4)::text, frozen_value::numeric(18,4)::text
		FROM marketing_quota
		WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
	`, activityID, quotaType, scope, scopeKey).Scan(&used, &frozen)
	if err != nil {
		t.Fatalf("query quota values: %v", err)
	}
	if used != wantUsed || frozen != wantFrozen {
		t.Fatalf("expected quota %s/%s/%s used=%s frozen=%s, got used=%s frozen=%s", quotaType, scope, scopeKey, wantUsed, wantFrozen, used, frozen)
	}
}

func postgresQuotaVersion(t *testing.T, repo *Repository, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string) int {
	t.Helper()
	var version int
	err := repo.db.QueryRowContext(context.Background(), `
		SELECT version
		FROM marketing_quota
		WHERE activity_id = $1 AND quota_type = $2 AND scope = $3 AND scope_key = $4
	`, activityID, quotaType, scope, scopeKey).Scan(&version)
	if err != nil {
		t.Fatalf("query quota version: %v", err)
	}
	return version
}

func assertPostgresGrantStatus(t *testing.T, repo *Repository, tenantID string, commandID string, want marketing.GrantStatus) {
	t.Helper()
	var got marketing.GrantStatus
	if err := repo.db.QueryRowContext(context.Background(), `
		SELECT grant_status
		FROM marketing_grant_order
		WHERE tenant_id = $1 AND bill_no = $2
	`, tenantID, commandID).Scan(&got); err != nil {
		t.Fatalf("query grant status: %v", err)
	}
	if got != want {
		t.Fatalf("expected grant %s/%s status %s, got %s", tenantID, commandID, want, got)
	}
}

func assertPostgresParticipationStatus(t *testing.T, repo *Repository, participationID string, want marketing.ParticipationStatus) {
	t.Helper()
	var got marketing.ParticipationStatus
	if err := repo.db.QueryRowContext(context.Background(), `
		SELECT status
		FROM marketing_participation
		WHERE id = $1
	`, participationID).Scan(&got); err != nil {
		t.Fatalf("query participation status: %v", err)
	}
	if got != want {
		t.Fatalf("expected participation %s status %s, got %s", participationID, want, got)
	}
}

func postgresOutboxStatus(t *testing.T, repo *Repository, id string) marketing.OutboxStatus {
	t.Helper()
	var got marketing.OutboxStatus
	if err := repo.db.QueryRowContext(context.Background(), `
		SELECT send_status
		FROM marketing_outbox
		WHERE id = $1
	`, id).Scan(&got); err != nil {
		t.Fatalf("query outbox status %s: %v", id, err)
	}
	return got
}

func postgresOutboxUpdatedAt(t *testing.T, repo *Repository, id string) time.Time {
	t.Helper()
	var got time.Time
	if err := repo.db.QueryRowContext(context.Background(), `
		SELECT updated_at
		FROM marketing_outbox
		WHERE id = $1
	`, id).Scan(&got); err != nil {
		t.Fatalf("query outbox updated_at %s: %v", id, err)
	}
	return got
}

func outboxIDsWithPrefix(messages []marketing.OutboxMessage, prefix string) []string {
	var ids []string
	for _, message := range messages {
		if strings.HasPrefix(message.ID, prefix) {
			ids = append(ids, message.ID)
		}
	}
	return ids
}

func countPostgresAtomicResults(t *testing.T, results <-chan error) (int, int) {
	t.Helper()
	var success int
	var quotaExceeded int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, marketing.ErrQuotaExceeded):
			quotaExceeded++
		default:
			t.Fatalf("unexpected concurrent atomic grant error: %v", err)
		}
	}
	return success, quotaExceeded
}

func mustCreatePostgresQuota(t *testing.T, repo *Repository, quota marketing.Quota) {
	t.Helper()
	if _, err := repo.CreateQuota(context.Background(), quota); err != nil {
		t.Fatalf("create quota %s: %v", quota.ID, err)
	}
}

func createPostgresCallbackQuotas(t *testing.T, repo *Repository, activityID string, prefix string, userScopeKey string, dailyScopeKey string, now time.Time) {
	t.Helper()
	for _, quota := range []marketing.Quota{
		{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "10", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_user", ActivityID: activityID, Type: marketing.QuotaUserLimit, Scope: marketing.QuotaScopeUser, ScopeKey: userScopeKey, TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_daily", ActivityID: activityID, Type: marketing.QuotaDailyLimit, Scope: marketing.QuotaScopeUser, ScopeKey: dailyScopeKey, TotalValue: "1", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		mustCreatePostgresQuota(t, repo, quota)
	}
}

func createPostgresAtomicQuotas(t *testing.T, repo *Repository, activityID string, prefix string, now time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, quota := range []marketing.Quota{
		{ID: prefix + "_budget", ActivityID: activityID, Type: marketing.QuotaBudget, Scope: marketing.QuotaScopeActivity, ScopeKey: "TOTAL", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: prefix + "_stock", ActivityID: activityID, Type: marketing.QuotaStock, Scope: marketing.QuotaScopeActivity, ScopeKey: "REWARD_A", TotalValue: "100", UsedValue: "0", FrozenValue: "0", Version: 1, CreatedAt: now, UpdatedAt: now},
	} {
		if _, err := repo.CreateQuota(ctx, quota); err != nil {
			t.Fatalf("create quota %s: %v", quota.ID, err)
		}
	}
}

func testPostgresActivity(id string, tenantID string, code string, now time.Time) marketing.Activity {
	return marketing.Activity{
		ID:             id,
		TenantID:       tenantID,
		Code:           code,
		Name:           "Postgres Activity",
		Type:           "MAIN_CONFIG",
		Status:         marketing.ActivityActive,
		TriggerEvents:  []string{"DEPOSIT_SUCCESS"},
		StartTime:      now.Add(time.Hour),
		EndTime:        now.Add(2 * time.Hour),
		Timezone:       "UTC",
		Priority:       1,
		ExclusiveGroup: "pg_group",
		Stackable:      false,
		Version:        1,
		Config:         []byte(`{"metadata":{"owner":"pg"}}`),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testPostgresParticipation(id string, activityID string, idempotencyKey string, now time.Time) marketing.Participation {
	return marketing.Participation{
		ID:             id,
		TenantID:       "site_001",
		ActivityID:     activityID,
		EventInboxID:   id + "_inbox",
		Subject:        marketing.EventSubject{Type: "USER", ID: "user_1"},
		Biz:            marketing.EventBiz{BizType: "DEPOSIT", BizID: id + "_pay"},
		Status:         marketing.ParticipationRewardable,
		Snapshot:       []byte(`{}`),
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func testPostgresGrant(id string, activityID string, participationID string, idempotencyKey string, now time.Time) marketing.GrantOrder {
	return marketing.GrantOrder{
		ID:              id,
		TenantID:        "site_001",
		ActivityID:      activityID,
		ParticipationID: participationID,
		Subject:         marketing.EventSubject{Type: "USER", ID: "user_1"},
		RewardType:      marketing.RewardBonus,
		RewardAmount:    "1",
		RewardPayload:   []byte(`{}`),
		BillNo:          id + "_cmd",
		GrantStatus:     marketing.GrantPending,
		GrantChannel:    marketing.GrantHTTP,
		IdempotencyKey:  idempotencyKey,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func testPostgresManualGrant(id string, activityID string, participationID string, idempotencyKey string, rewardCode string, amount string, mode marketing.GrantMode, now time.Time) marketing.GrantOrder {
	payload, _ := json.Marshal(map[string]any{
		"rewardCode":    rewardCode,
		"rewardType":    marketing.RewardBonus,
		"calculateType": marketing.CalculateFixed,
		"grantMode":     mode,
		"grantChannel":  marketing.GrantManualFile,
	})
	order := testPostgresGrant(id, activityID, participationID, idempotencyKey, now)
	order.RewardAmount = amount
	order.RewardPayload = payload
	order.GrantChannel = marketing.GrantManualFile
	return order
}

func insertPostgresFailedManualExportHistory(ctx context.Context, repo *Repository, prefix string, grant marketing.GrantOrder, now time.Time) error {
	exportID := prefix + "_failed_export"
	if _, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_manual_grant_export_file (
			id, tenant_id, file_name, file_format, status, operator, reason, filter_snapshot_json,
			row_count, file_checksum, storage_ref, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, 'CSV', 'FAILED', 'ops', 'file write failed', $4, 1, NULL, NULL, 'disk full', $5, $5)
	`, exportID, grant.TenantID, exportID+".csv", []byte(`{"reason":"test failed history"}`), now); err != nil {
		return err
	}
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO marketing_manual_grant_export_detail (
			id, export_id, tenant_id, grant_order_id, bill_no, row_number,
			export_grant_status, detail_status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 1, $6, 'READY', $7, $7)
	`, prefix+"_failed_detail", exportID, grant.TenantID, grant.ID, grant.BillNo, grant.GrantStatus, now)
	return err
}

func testPostgresOutbox(id string, now time.Time) marketing.OutboxMessage {
	return marketing.OutboxMessage{
		ID:            id,
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://reward.example.com",
		Payload:       []byte(`{}`),
		SendStatus:    marketing.OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
