package marketing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryRepositoryWorkspaceLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Now().UTC()

	workspace := Workspace{
		ID:          "ws_1",
		TenantID:    "site_001",
		Name:        "Growth Team",
		Slug:        "growth-team",
		Description: "Growth workspace",
		Visibility:  WorkspaceTeam,
		DefaultRole: WorkspaceRoleViewer,
		OwnerID:     "user_owner",
		Status:      WorkspaceActive,
		CreatedBy:   "user_owner",
		UpdatedBy:   "user_owner",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	createdWorkspace, err := repo.CreateWorkspace(ctx, workspace)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if createdWorkspace.ID != workspace.ID {
		t.Fatalf("unexpected workspace: %+v", createdWorkspace)
	}
	if _, err := repo.CreateWorkspace(ctx, workspace); !errors.Is(err, ErrWorkspaceConflict) {
		t.Fatalf("expected workspace conflict, got %v", err)
	}

	member := WorkspaceMember{
		ID:          "wsm_1",
		WorkspaceID: workspace.ID,
		UserID:      "user_editor",
		Email:       "editor@example.com",
		DisplayName: "Editor",
		Role:        WorkspaceRoleEditor,
		Status:      WorkspaceMemberActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateWorkspaceMember(ctx, member); err != nil {
		t.Fatalf("create member: %v", err)
	}
	workspaces, err := repo.ListWorkspaces(ctx, WorkspaceQuery{TenantID: "site_001", MemberUserID: "user_editor"})
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("expected member workspace, got %+v", workspaces)
	}

	folder := WorkspaceFolder{
		ID:          "folder_1",
		WorkspaceID: workspace.ID,
		Name:        "VIP 活动",
		Visibility:  WorkspaceFolderTeam,
		SortOrder:   10,
		Status:      WorkspaceFolderActive,
		CreatedBy:   "user_editor",
		UpdatedBy:   "user_editor",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateWorkspaceFolder(ctx, folder); err != nil {
		t.Fatalf("create folder: %v", err)
	}
	folders, err := repo.ListWorkspaceFolders(ctx, WorkspaceFolderQuery{WorkspaceID: workspace.ID})
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 1 || folders[0].ID != folder.ID {
		t.Fatalf("expected folder, got %+v", folders)
	}

	share := WorkspaceShareLink{
		ID:          "share_1",
		WorkspaceID: workspace.ID,
		FolderID:    folder.ID,
		Token:       "token_1",
		Scope:       WorkspaceShareFolder,
		Permission:  WorkspaceShareView,
		Enabled:     true,
		CreatedBy:   "user_editor",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := repo.CreateWorkspaceShareLink(ctx, share); err != nil {
		t.Fatalf("create share: %v", err)
	}
	shares, err := repo.ListWorkspaceShareLinks(ctx, WorkspaceShareLinkQuery{WorkspaceID: workspace.ID, EnabledOnly: true})
	if err != nil {
		t.Fatalf("list shares: %v", err)
	}
	if len(shares) != 1 || shares[0].Token != share.Token {
		t.Fatalf("expected share, got %+v", shares)
	}

	log := WorkspaceAuditLog{
		ID:          "wsa_1",
		WorkspaceID: workspace.ID,
		ActorID:     "user_editor",
		Action:      "CREATE_FOLDER",
		TargetType:  "FOLDER",
		TargetID:    folder.ID,
		CreatedAt:   now,
	}
	if err := repo.CreateWorkspaceAuditLog(ctx, log); err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	logs, err := repo.ListWorkspaceAuditLogs(ctx, WorkspaceAuditLogQuery{WorkspaceID: workspace.ID, TargetID: folder.ID})
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != log.Action {
		t.Fatalf("expected audit log, got %+v", logs)
	}
}

func TestMemoryRepositoryUpdateWorkspaceMemberRejectsDuplicateIdentity(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Now().UTC()

	members := []WorkspaceMember{
		{
			ID:          "wsm_1",
			WorkspaceID: "ws_1",
			UserID:      "user_1",
			Email:       "one@example.com",
			Role:        WorkspaceRoleEditor,
			Status:      WorkspaceMemberActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "wsm_2",
			WorkspaceID: "ws_1",
			UserID:      "user_2",
			Email:       "two@example.com",
			Role:        WorkspaceRoleViewer,
			Status:      WorkspaceMemberActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	for _, member := range members {
		if _, err := repo.CreateWorkspaceMember(ctx, member); err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	members[1].UserID = members[0].UserID
	if _, err := repo.UpdateWorkspaceMember(ctx, members[1]); !errors.Is(err, ErrWorkspaceConflict) {
		t.Fatalf("expected workspace conflict, got %v", err)
	}
}
