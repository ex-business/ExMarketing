package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestWorkspaceServiceRejectsCrossWorkspaceFolderReferences(t *testing.T) {
	ctx := context.Background()
	service := NewService(marketing.NewMemoryRepository())

	first, err := service.CreateWorkspace(ctx, marketing.Workspace{
		ID:          "ws_first",
		TenantID:    "site_001",
		Name:        "First",
		Slug:        "first",
		Visibility:  marketing.WorkspaceTeam,
		DefaultRole: marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceActive,
	})
	if err != nil {
		t.Fatalf("create first workspace: %v", err)
	}
	second, err := service.CreateWorkspace(ctx, marketing.Workspace{
		ID:          "ws_second",
		TenantID:    "site_001",
		Name:        "Second",
		Slug:        "second",
		Visibility:  marketing.WorkspaceTeam,
		DefaultRole: marketing.WorkspaceRoleViewer,
		Status:      marketing.WorkspaceActive,
	})
	if err != nil {
		t.Fatalf("create second workspace: %v", err)
	}
	parent, err := service.CreateWorkspaceFolder(ctx, first.ID, marketing.WorkspaceFolder{
		ID:         "folder_parent",
		Name:       "Parent",
		Visibility: marketing.WorkspaceFolderTeam,
		Status:     marketing.WorkspaceFolderActive,
	})
	if err != nil {
		t.Fatalf("create parent folder: %v", err)
	}

	if _, err := service.CreateWorkspaceFolder(ctx, second.ID, marketing.WorkspaceFolder{
		ID:         "folder_child",
		ParentID:   parent.ID,
		Name:       "Cross workspace child",
		Visibility: marketing.WorkspaceFolderTeam,
		Status:     marketing.WorkspaceFolderActive,
	}); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected not found for cross-workspace parent, got %v", err)
	}

	if _, err := service.CreateWorkspaceShareLink(ctx, second.ID, marketing.WorkspaceShareLink{
		ID:         "share_cross",
		FolderID:   parent.ID,
		Token:      "token_cross",
		Scope:      marketing.WorkspaceShareFolder,
		Permission: marketing.WorkspaceShareView,
	}); !errors.Is(err, marketing.ErrNotFound) {
		t.Fatalf("expected not found for cross-workspace folder share, got %v", err)
	}
}
