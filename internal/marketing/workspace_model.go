package marketing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type WorkspaceVisibility string
type WorkspaceStatus string
type WorkspaceRole string
type WorkspaceMemberStatus string
type WorkspaceFolderVisibility string
type WorkspaceFolderStatus string
type WorkspaceShareScope string
type WorkspaceSharePermission string

const (
	WorkspacePrivate WorkspaceVisibility = "PRIVATE"
	WorkspaceTeam    WorkspaceVisibility = "TEAM"
	WorkspacePublic  WorkspaceVisibility = "PUBLIC"

	WorkspaceActive   WorkspaceStatus = "ACTIVE"
	WorkspaceArchived WorkspaceStatus = "ARCHIVED"
	WorkspaceDeleted  WorkspaceStatus = "DELETED"

	WorkspaceRoleOwner  WorkspaceRole = "OWNER"
	WorkspaceRoleAdmin  WorkspaceRole = "ADMIN"
	WorkspaceRoleEditor WorkspaceRole = "EDITOR"
	WorkspaceRoleViewer WorkspaceRole = "VIEWER"
	WorkspaceRoleGuest  WorkspaceRole = "GUEST"

	WorkspaceMemberActive    WorkspaceMemberStatus = "ACTIVE"
	WorkspaceMemberInvited   WorkspaceMemberStatus = "INVITED"
	WorkspaceMemberSuspended WorkspaceMemberStatus = "SUSPENDED"
	WorkspaceMemberRemoved   WorkspaceMemberStatus = "REMOVED"

	WorkspaceFolderInherit WorkspaceFolderVisibility = "INHERIT"
	WorkspaceFolderPrivate WorkspaceFolderVisibility = "PRIVATE"
	WorkspaceFolderTeam    WorkspaceFolderVisibility = "TEAM"
	WorkspaceFolderPublic  WorkspaceFolderVisibility = "PUBLIC"

	WorkspaceFolderActive   WorkspaceFolderStatus = "ACTIVE"
	WorkspaceFolderArchived WorkspaceFolderStatus = "ARCHIVED"
	WorkspaceFolderDeleted  WorkspaceFolderStatus = "DELETED"

	WorkspaceShareWorkspace WorkspaceShareScope = "WORKSPACE"
	WorkspaceShareFolder    WorkspaceShareScope = "FOLDER"
	WorkspaceShareActivity  WorkspaceShareScope = "ACTIVITY"

	WorkspaceShareView WorkspaceSharePermission = "VIEW"
	WorkspaceShareEdit WorkspaceSharePermission = "EDIT"
)

type Workspace struct {
	ID          string              `json:"id"`
	TenantID    string              `json:"tenantId"`
	Name        string              `json:"name"`
	Slug        string              `json:"slug"`
	Description string              `json:"description,omitempty"`
	Icon        string              `json:"icon,omitempty"`
	Color       string              `json:"color,omitempty"`
	Visibility  WorkspaceVisibility `json:"visibility"`
	DefaultRole WorkspaceRole       `json:"defaultRole"`
	OwnerID     string              `json:"ownerId"`
	Status      WorkspaceStatus     `json:"status"`
	CreatedBy   string              `json:"createdBy,omitempty"`
	UpdatedBy   string              `json:"updatedBy,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

type WorkspacePatch struct {
	Name        *string              `json:"name,omitempty"`
	Slug        *string              `json:"slug,omitempty"`
	Description *string              `json:"description,omitempty"`
	Icon        *string              `json:"icon,omitempty"`
	Color       *string              `json:"color,omitempty"`
	Visibility  *WorkspaceVisibility `json:"visibility,omitempty"`
	DefaultRole *WorkspaceRole       `json:"defaultRole,omitempty"`
	OwnerID     *string              `json:"ownerId,omitempty"`
	Status      *WorkspaceStatus     `json:"status,omitempty"`
	UpdatedBy   *string              `json:"updatedBy,omitempty"`
}

type WorkspaceMember struct {
	ID          string                `json:"id"`
	WorkspaceID string                `json:"workspaceId"`
	UserID      string                `json:"userId,omitempty"`
	Email       string                `json:"email,omitempty"`
	DisplayName string                `json:"displayName,omitempty"`
	Role        WorkspaceRole         `json:"role"`
	Status      WorkspaceMemberStatus `json:"status"`
	InvitedBy   string                `json:"invitedBy,omitempty"`
	InvitedAt   *time.Time            `json:"invitedAt,omitempty"`
	JoinedAt    *time.Time            `json:"joinedAt,omitempty"`
	CreatedAt   time.Time             `json:"createdAt"`
	UpdatedAt   time.Time             `json:"updatedAt"`
}

type WorkspaceFolder struct {
	ID          string                    `json:"id"`
	WorkspaceID string                    `json:"workspaceId"`
	ParentID    string                    `json:"parentId,omitempty"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Icon        string                    `json:"icon,omitempty"`
	Color       string                    `json:"color,omitempty"`
	Visibility  WorkspaceFolderVisibility `json:"visibility"`
	SortOrder   int                       `json:"sortOrder"`
	Status      WorkspaceFolderStatus     `json:"status"`
	CreatedBy   string                    `json:"createdBy,omitempty"`
	UpdatedBy   string                    `json:"updatedBy,omitempty"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
}

type WorkspaceFolderPatch struct {
	ParentID    *string                    `json:"parentId,omitempty"`
	Name        *string                    `json:"name,omitempty"`
	Description *string                    `json:"description,omitempty"`
	Icon        *string                    `json:"icon,omitempty"`
	Color       *string                    `json:"color,omitempty"`
	Visibility  *WorkspaceFolderVisibility `json:"visibility,omitempty"`
	SortOrder   *int                       `json:"sortOrder,omitempty"`
	Status      *WorkspaceFolderStatus     `json:"status,omitempty"`
	UpdatedBy   *string                    `json:"updatedBy,omitempty"`
}

type WorkspaceShareLink struct {
	ID           string                   `json:"id"`
	WorkspaceID  string                   `json:"workspaceId"`
	FolderID     string                   `json:"folderId,omitempty"`
	ActivityID   string                   `json:"activityId,omitempty"`
	Token        string                   `json:"token"`
	Scope        WorkspaceShareScope      `json:"scope"`
	Permission   WorkspaceSharePermission `json:"permission"`
	ExpiresAt    *time.Time               `json:"expiresAt,omitempty"`
	PasswordHash string                   `json:"passwordHash,omitempty"`
	Enabled      bool                     `json:"enabled"`
	CreatedBy    string                   `json:"createdBy,omitempty"`
	CreatedAt    time.Time                `json:"createdAt"`
	UpdatedAt    time.Time                `json:"updatedAt"`
}

type WorkspaceShareLinkPatch struct {
	FolderID     *string                   `json:"folderId,omitempty"`
	ActivityID   *string                   `json:"activityId,omitempty"`
	Token        *string                   `json:"token,omitempty"`
	Scope        *WorkspaceShareScope      `json:"scope,omitempty"`
	Permission   *WorkspaceSharePermission `json:"permission,omitempty"`
	ExpiresAt    *time.Time                `json:"expiresAt,omitempty"`
	PasswordHash *string                   `json:"passwordHash,omitempty"`
	Enabled      *bool                     `json:"enabled,omitempty"`
	UpdatedBy    *string                   `json:"updatedBy,omitempty"`
}

type WorkspaceAuditLog struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspaceId"`
	ActorID     string          `json:"actorId,omitempty"`
	Action      string          `json:"action"`
	TargetType  string          `json:"targetType"`
	TargetID    string          `json:"targetId"`
	Before      json.RawMessage `json:"beforeJson,omitempty"`
	After       json.RawMessage `json:"afterJson,omitempty"`
	IP          string          `json:"ip,omitempty"`
	UserAgent   string          `json:"userAgent,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

type WorkspaceQuery struct {
	TenantID     string
	Status       WorkspaceStatus
	Visibility   WorkspaceVisibility
	MemberUserID string
	Limit        int
}

type WorkspaceFolderQuery struct {
	WorkspaceID string
	ParentID    string
	Status      WorkspaceFolderStatus
	Visibility  WorkspaceFolderVisibility
	Limit       int
}

type WorkspaceShareLinkQuery struct {
	WorkspaceID string
	Scope       WorkspaceShareScope
	FolderID    string
	ActivityID  string
	EnabledOnly bool
	Limit       int
}

type WorkspaceAuditLogQuery struct {
	WorkspaceID string
	ActorID     string
	Action      string
	TargetType  string
	TargetID    string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
}

func (w Workspace) Validate() error {
	if strings.TrimSpace(w.TenantID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(w.Name) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("name is required"))
	}
	if strings.TrimSpace(w.Slug) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("slug is required"))
	}
	if w.Visibility != "" && !validWorkspaceVisibility(w.Visibility) {
		return errors.Join(ErrInvalidWorkspace, errors.New("visibility is invalid"))
	}
	if w.DefaultRole != "" && !validWorkspaceRole(w.DefaultRole) {
		return errors.Join(ErrInvalidWorkspace, errors.New("defaultRole is invalid"))
	}
	if w.Status != "" && !validWorkspaceStatus(w.Status) {
		return errors.Join(ErrInvalidWorkspace, errors.New("status is invalid"))
	}
	return nil
}

func (m WorkspaceMember) Validate() error {
	if strings.TrimSpace(m.WorkspaceID) == "" {
		return errors.Join(ErrInvalidWorkspaceMember, errors.New("workspaceId is required"))
	}
	if strings.TrimSpace(m.UserID) == "" && strings.TrimSpace(m.Email) == "" {
		return errors.Join(ErrInvalidWorkspaceMember, errors.New("userId or email is required"))
	}
	if !validWorkspaceRole(m.Role) {
		return errors.Join(ErrInvalidWorkspaceMember, errors.New("role is invalid"))
	}
	if m.Status != "" && !validWorkspaceMemberStatus(m.Status) {
		return errors.Join(ErrInvalidWorkspaceMember, errors.New("status is invalid"))
	}
	return nil
}

func (f WorkspaceFolder) Validate() error {
	if strings.TrimSpace(f.WorkspaceID) == "" {
		return errors.Join(ErrInvalidWorkspaceFolder, errors.New("workspaceId is required"))
	}
	if strings.TrimSpace(f.Name) == "" {
		return errors.Join(ErrInvalidWorkspaceFolder, errors.New("name is required"))
	}
	if f.Visibility != "" && !validWorkspaceFolderVisibility(f.Visibility) {
		return errors.Join(ErrInvalidWorkspaceFolder, errors.New("visibility is invalid"))
	}
	if f.Status != "" && !validWorkspaceFolderStatus(f.Status) {
		return errors.Join(ErrInvalidWorkspaceFolder, errors.New("status is invalid"))
	}
	return nil
}

func (s WorkspaceShareLink) Validate() error {
	if strings.TrimSpace(s.WorkspaceID) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("workspaceId is required"))
	}
	if strings.TrimSpace(s.Token) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("token is required"))
	}
	if !validWorkspaceShareScope(s.Scope) {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("scope is invalid"))
	}
	if !validWorkspaceSharePermission(s.Permission) {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("permission is invalid"))
	}
	if s.Scope == WorkspaceShareFolder && strings.TrimSpace(s.FolderID) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("folderId is required for folder share"))
	}
	if s.Scope == WorkspaceShareActivity && strings.TrimSpace(s.ActivityID) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("activityId is required for activity share"))
	}
	return nil
}

func validWorkspaceVisibility(value WorkspaceVisibility) bool {
	switch value {
	case WorkspacePrivate, WorkspaceTeam, WorkspacePublic:
		return true
	default:
		return false
	}
}

func validWorkspaceStatus(value WorkspaceStatus) bool {
	switch value {
	case WorkspaceActive, WorkspaceArchived, WorkspaceDeleted:
		return true
	default:
		return false
	}
}

func validWorkspaceRole(value WorkspaceRole) bool {
	switch value {
	case WorkspaceRoleOwner, WorkspaceRoleAdmin, WorkspaceRoleEditor, WorkspaceRoleViewer, WorkspaceRoleGuest:
		return true
	default:
		return false
	}
}

func validWorkspaceMemberStatus(value WorkspaceMemberStatus) bool {
	switch value {
	case WorkspaceMemberActive, WorkspaceMemberInvited, WorkspaceMemberSuspended, WorkspaceMemberRemoved:
		return true
	default:
		return false
	}
}

func validWorkspaceFolderVisibility(value WorkspaceFolderVisibility) bool {
	switch value {
	case WorkspaceFolderInherit, WorkspaceFolderPrivate, WorkspaceFolderTeam, WorkspaceFolderPublic:
		return true
	default:
		return false
	}
}

func validWorkspaceFolderStatus(value WorkspaceFolderStatus) bool {
	switch value {
	case WorkspaceFolderActive, WorkspaceFolderArchived, WorkspaceFolderDeleted:
		return true
	default:
		return false
	}
}

func validWorkspaceShareScope(value WorkspaceShareScope) bool {
	switch value {
	case WorkspaceShareWorkspace, WorkspaceShareFolder, WorkspaceShareActivity:
		return true
	default:
		return false
	}
}

func validWorkspaceSharePermission(value WorkspaceSharePermission) bool {
	switch value {
	case WorkspaceShareView, WorkspaceShareEdit:
		return true
	default:
		return false
	}
}
