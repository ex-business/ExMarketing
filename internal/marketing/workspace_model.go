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
type WorkspacePrincipalType string
type WorkspaceFileObjectKind string
type WorkspaceBatchAction string
type WorkspaceBatchItemType string

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

	WorkspacePrincipalUser   WorkspacePrincipalType = "USER"
	WorkspacePrincipalMember WorkspacePrincipalType = "MEMBER"
	WorkspacePrincipalRole   WorkspacePrincipalType = "ROLE"

	WorkspaceFileImportWorkflow      WorkspaceFileObjectKind = "IMPORT_WORKFLOW"
	WorkspaceFileWorkflowVersion     WorkspaceFileObjectKind = "WORKFLOW_VERSION"
	WorkspaceFileExecutionTrace      WorkspaceFileObjectKind = "EXECUTION_TRACE"
	WorkspaceFileExecutionAttachment WorkspaceFileObjectKind = "EXECUTION_ATTACHMENT"

	WorkspaceBatchMove       WorkspaceBatchAction = "MOVE"
	WorkspaceBatchPermission WorkspaceBatchAction = "PERMISSION"
	WorkspaceBatchArchive    WorkspaceBatchAction = "ARCHIVE"
	WorkspaceBatchRestore    WorkspaceBatchAction = "RESTORE"

	WorkspaceBatchFolder   WorkspaceBatchItemType = "FOLDER"
	WorkspaceBatchWorkflow WorkspaceBatchItemType = "WORKFLOW"
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
	ID               string                `json:"id"`
	WorkspaceID      string                `json:"workspaceId"`
	UserID           string                `json:"userId,omitempty"`
	Email            string                `json:"email,omitempty"`
	DisplayName      string                `json:"displayName,omitempty"`
	Role             WorkspaceRole         `json:"role"`
	Status           WorkspaceMemberStatus `json:"status"`
	InviteToken      string                `json:"inviteToken,omitempty"`
	InvitedBy        string                `json:"invitedBy,omitempty"`
	InvitedAt        *time.Time            `json:"invitedAt,omitempty"`
	InviteExpiresAt  *time.Time            `json:"inviteExpiresAt,omitempty"`
	InviteAcceptedAt *time.Time            `json:"inviteAcceptedAt,omitempty"`
	JoinedAt         *time.Time            `json:"joinedAt,omitempty"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
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
	ID             string                   `json:"id"`
	WorkspaceID    string                   `json:"workspaceId"`
	FolderID       string                   `json:"folderId,omitempty"`
	ActivityID     string                   `json:"activityId,omitempty"`
	Token          string                   `json:"token"`
	Scope          WorkspaceShareScope      `json:"scope"`
	Permission     WorkspaceSharePermission `json:"permission"`
	ExpiresAt      *time.Time               `json:"expiresAt,omitempty"`
	PasswordHash   string                   `json:"passwordHash,omitempty"`
	MaxAccessCount int                      `json:"maxAccessCount,omitempty"`
	AccessCount    int                      `json:"accessCount"`
	LastAccessedAt *time.Time               `json:"lastAccessedAt,omitempty"`
	DisabledReason string                   `json:"disabledReason,omitempty"`
	Enabled        bool                     `json:"enabled"`
	CreatedBy      string                   `json:"createdBy,omitempty"`
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
}

type WorkspaceShareLinkPatch struct {
	FolderID       *string                   `json:"folderId,omitempty"`
	ActivityID     *string                   `json:"activityId,omitempty"`
	Token          *string                   `json:"token,omitempty"`
	Scope          *WorkspaceShareScope      `json:"scope,omitempty"`
	Permission     *WorkspaceSharePermission `json:"permission,omitempty"`
	ExpiresAt      *time.Time                `json:"expiresAt,omitempty"`
	PasswordHash   *string                   `json:"passwordHash,omitempty"`
	MaxAccessCount *int                      `json:"maxAccessCount,omitempty"`
	DisabledReason *string                   `json:"disabledReason,omitempty"`
	Enabled        *bool                     `json:"enabled,omitempty"`
	UpdatedBy      *string                   `json:"updatedBy,omitempty"`
}

type WorkspaceActivityGrant struct {
	ID            string                 `json:"id"`
	WorkspaceID   string                 `json:"workspaceId"`
	ActivityID    string                 `json:"activityId"`
	PrincipalType WorkspacePrincipalType `json:"principalType"`
	PrincipalID   string                 `json:"principalId"`
	Role          WorkspaceRole          `json:"role"`
	CreatedBy     string                 `json:"createdBy,omitempty"`
	UpdatedBy     string                 `json:"updatedBy,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

type WorkspaceActivityGrantQuery struct {
	WorkspaceID   string
	ActivityID    string
	PrincipalType WorkspacePrincipalType
	PrincipalID   string
	Limit         int
}

type WorkspaceHomeSearchQuery struct {
	Keyword    string
	Type       string
	Status     string
	Visibility string
	Sort       string
	Order      string
	ActorID    string
	Offset     int
	Limit      int
}

type WorkspaceHomeSearchResult struct {
	ID                string         `json:"id"`
	Type              string         `json:"type"`
	Title             string         `json:"title"`
	Subtitle          string         `json:"subtitle"`
	WorkspaceID       string         `json:"workspaceId,omitempty"`
	FolderID          string         `json:"folderId,omitempty"`
	Owner             string         `json:"owner,omitempty"`
	Status            string         `json:"status,omitempty"`
	Visibility        string         `json:"visibility,omitempty"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	MatchedBy         string         `json:"matchedBy"`
	Score             float64        `json:"score"`
	Highlights        map[string]any `json:"highlights,omitempty"`
	Permission        string         `json:"permission"`
	PermissionReason  string         `json:"permissionReason"`
	PermissionSources []string       `json:"permissionSources,omitempty"`
}

type WorkspaceHomeSearchResponse struct {
	Items  []WorkspaceHomeSearchResult `json:"items"`
	Total  int                         `json:"total"`
	Offset int                         `json:"offset"`
	Limit  int                         `json:"limit"`
}

type WorkspaceFileObject struct {
	ID                string                  `json:"id"`
	WorkspaceID       string                  `json:"workspaceId"`
	FolderID          string                  `json:"folderId,omitempty"`
	ActivityID        string                  `json:"activityId,omitempty"`
	WorkflowVersionID string                  `json:"workflowVersionId,omitempty"`
	ExecutionID       string                  `json:"executionId,omitempty"`
	Kind              WorkspaceFileObjectKind `json:"kind"`
	FileName          string                  `json:"fileName"`
	ContentType       string                  `json:"contentType,omitempty"`
	SizeBytes         int64                   `json:"sizeBytes"`
	StorageProvider   string                  `json:"storageProvider"`
	StorageKey        string                  `json:"storageKey"`
	ChecksumSHA256    string                  `json:"checksumSha256"`
	CreatedBy         string                  `json:"createdBy,omitempty"`
	CreatedAt         time.Time               `json:"createdAt"`
}

type WorkspaceFileObjectQuery struct {
	WorkspaceID       string
	FolderID          string
	ActivityID        string
	WorkflowVersionID string
	ExecutionID       string
	Kind              WorkspaceFileObjectKind
	Limit             int
}

type WorkspaceBatchItem struct {
	ID   string                 `json:"id"`
	Type WorkspaceBatchItemType `json:"type"`
}

type WorkspaceBatchCommand struct {
	WorkspaceID string               `json:"workspaceId,omitempty"`
	Action      WorkspaceBatchAction `json:"action"`
	FolderID    string               `json:"folderId,omitempty"`
	Visibility  WorkspaceVisibility  `json:"visibility,omitempty"`
	ActorID     string               `json:"actorId,omitempty"`
	Items       []WorkspaceBatchItem `json:"items"`
}

type WorkspaceBatchItemResult struct {
	ID     string                 `json:"id"`
	Type   WorkspaceBatchItemType `json:"type"`
	Status string                 `json:"status"`
	Error  string                 `json:"error,omitempty"`
}

type WorkspaceBatchResult struct {
	Committed bool                       `json:"committed"`
	Count     int                        `json:"count"`
	Items     []WorkspaceBatchItemResult `json:"items"`
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
	Token       string
	Scope       WorkspaceShareScope
	FolderID    string
	ActivityID  string
	EnabledOnly bool
	Limit       int
}

type WorkspaceStorageUsage struct {
	UsedBytes      int64 `json:"usedBytes"`
	TotalBytes     int64 `json:"totalBytes"`
	Workspaces     int   `json:"workspaces"`
	Folders        int   `json:"folders"`
	Workflows      int   `json:"workflows"`
	Members        int   `json:"members"`
	ShareLinks     int   `json:"shareLinks"`
	AuditLogs      int   `json:"auditLogs"`
	Drafts         int   `json:"drafts"`
	Versions       int   `json:"versions"`
	Executions     int   `json:"executions"`
	NodeExecutions int   `json:"nodeExecutions"`
	ActivityGrants int   `json:"activityGrants"`
	FileObjects    int   `json:"fileObjects"`
	FileBytes      int64 `json:"fileBytes"`
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
	if s.MaxAccessCount < 0 || s.AccessCount < 0 {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("access count is invalid"))
	}
	if s.Scope == WorkspaceShareFolder && strings.TrimSpace(s.FolderID) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("folderId is required for folder share"))
	}
	if s.Scope == WorkspaceShareActivity && strings.TrimSpace(s.ActivityID) == "" {
		return errors.Join(ErrInvalidWorkspaceShare, errors.New("activityId is required for activity share"))
	}
	return nil
}

func (g WorkspaceActivityGrant) Validate() error {
	if strings.TrimSpace(g.WorkspaceID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("workspaceId is required"))
	}
	if strings.TrimSpace(g.ActivityID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("activityId is required"))
	}
	if !validWorkspacePrincipalType(g.PrincipalType) {
		return errors.Join(ErrInvalidWorkspace, errors.New("principalType is invalid"))
	}
	if strings.TrimSpace(g.PrincipalID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("principalId is required"))
	}
	if !validWorkspaceRole(g.Role) || g.Role == WorkspaceRoleOwner {
		return errors.Join(ErrInvalidWorkspace, errors.New("role is invalid"))
	}
	return nil
}

func (f WorkspaceFileObject) Validate() error {
	if strings.TrimSpace(f.WorkspaceID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("workspaceId is required"))
	}
	if strings.TrimSpace(f.ID) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("id is required"))
	}
	if strings.TrimSpace(string(f.Kind)) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("kind is required"))
	}
	if strings.TrimSpace(f.FileName) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("fileName is required"))
	}
	if f.SizeBytes < 0 {
		return errors.Join(ErrInvalidWorkspace, errors.New("sizeBytes must be non-negative"))
	}
	if strings.TrimSpace(f.StorageProvider) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("storageProvider is required"))
	}
	if strings.TrimSpace(f.StorageKey) == "" {
		return errors.Join(ErrInvalidWorkspace, errors.New("storageKey is required"))
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

func validWorkspacePrincipalType(value WorkspacePrincipalType) bool {
	switch value {
	case WorkspacePrincipalUser, WorkspacePrincipalMember, WorkspacePrincipalRole:
		return true
	default:
		return false
	}
}
