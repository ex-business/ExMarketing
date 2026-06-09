package marketing

import (
	"context"
	"time"
)

type Repository interface {
	CreateWorkspace(ctx context.Context, workspace Workspace) (Workspace, error)
	GetWorkspace(ctx context.Context, id string) (Workspace, error)
	ListWorkspaces(ctx context.Context, query WorkspaceQuery) ([]Workspace, error)
	UpdateWorkspace(ctx context.Context, workspace Workspace) (Workspace, error)
	CreateWorkspaceMember(ctx context.Context, member WorkspaceMember) (WorkspaceMember, error)
	ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]WorkspaceMember, error)
	FindWorkspaceMemberByInviteToken(ctx context.Context, token string) (WorkspaceMember, error)
	UpdateWorkspaceMember(ctx context.Context, member WorkspaceMember) (WorkspaceMember, error)
	CreateWorkspaceFolder(ctx context.Context, folder WorkspaceFolder) (WorkspaceFolder, error)
	GetWorkspaceFolder(ctx context.Context, id string) (WorkspaceFolder, error)
	ListWorkspaceFolders(ctx context.Context, query WorkspaceFolderQuery) ([]WorkspaceFolder, error)
	UpdateWorkspaceFolder(ctx context.Context, folder WorkspaceFolder) (WorkspaceFolder, error)
	CreateWorkspaceShareLink(ctx context.Context, link WorkspaceShareLink) (WorkspaceShareLink, error)
	ListWorkspaceShareLinks(ctx context.Context, query WorkspaceShareLinkQuery) ([]WorkspaceShareLink, error)
	UpdateWorkspaceShareLink(ctx context.Context, link WorkspaceShareLink) (WorkspaceShareLink, error)
	CreateWorkspaceActivityGrant(ctx context.Context, grant WorkspaceActivityGrant) (WorkspaceActivityGrant, error)
	ListWorkspaceActivityGrants(ctx context.Context, query WorkspaceActivityGrantQuery) ([]WorkspaceActivityGrant, error)
	UpdateWorkspaceActivityGrant(ctx context.Context, grant WorkspaceActivityGrant) (WorkspaceActivityGrant, error)
	DeleteWorkspaceActivityGrant(ctx context.Context, id string) error
	CreateWorkspaceFileObject(ctx context.Context, file WorkspaceFileObject) (WorkspaceFileObject, error)
	ListWorkspaceFileObjects(ctx context.Context, query WorkspaceFileObjectQuery) ([]WorkspaceFileObject, error)
	DeleteWorkspaceFileObject(ctx context.Context, workspaceID string, fileID string) (WorkspaceFileObject, error)
	ApplyWorkspaceBatch(ctx context.Context, cmd WorkspaceBatchCommand) (WorkspaceBatchResult, error)
	CreateWorkspaceAuditLog(ctx context.Context, log WorkspaceAuditLog) error
	ListWorkspaceAuditLogs(ctx context.Context, query WorkspaceAuditLogQuery) ([]WorkspaceAuditLog, error)

	CreateActivity(ctx context.Context, activity Activity) (Activity, error)
	GetActivity(ctx context.Context, id string) (Activity, error)
	ListActivities(ctx context.Context) ([]Activity, error)
	UpdateActivity(ctx context.Context, activity Activity) (Activity, error)
	FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]Activity, error)
	SaveWorkflowDraft(ctx context.Context, draft WorkflowDraft) (WorkflowDraft, error)
	GetWorkflowDraft(ctx context.Context, activityID string) (WorkflowDraft, error)
	CreateWorkflowDraftTemplate(ctx context.Context, template WorkflowDraftTemplate) (WorkflowDraftTemplate, error)
	ListWorkflowDraftTemplates(ctx context.Context, query WorkflowDraftTemplateQuery) ([]WorkflowDraftTemplate, error)
	GetWorkflowDraftTemplate(ctx context.Context, templateID string, versionID string, version int) (WorkflowDraftTemplate, error)
	CreateWorkflowVersion(ctx context.Context, version WorkflowVersion) (WorkflowVersion, error)
	GetWorkflowVersion(ctx context.Context, activityID string, versionID string) (WorkflowVersion, error)
	GetActiveWorkflowVersion(ctx context.Context, activityID string) (WorkflowVersion, error)
	ListWorkflowVersions(ctx context.Context, activityID string) ([]WorkflowVersion, error)
	UpdateWorkflowVersionStatus(ctx context.Context, activityID string, versionID string, status WorkflowVersionStatus) (WorkflowVersion, error)
	CreateWorkflowExecution(ctx context.Context, execution WorkflowExecution) (WorkflowExecution, error)
	UpdateWorkflowExecution(ctx context.Context, execution WorkflowExecution) error
	CreateWorkflowNodeExecution(ctx context.Context, execution WorkflowNodeExecution) (WorkflowNodeExecution, error)
	UpdateWorkflowNodeExecution(ctx context.Context, execution WorkflowNodeExecution) error
	ListWorkflowExecutions(ctx context.Context, query WorkflowExecutionQuery) ([]WorkflowExecution, error)
	ListWorkflowNodeExecutions(ctx context.Context, query WorkflowNodeExecutionQuery) ([]WorkflowNodeExecution, error)
	SaveFactProvider(ctx context.Context, provider FactProvider) (FactProvider, error)
	FindEnabledFactProvider(ctx context.Context, tenantID string, factCode string) (FactProvider, error)

	CreateInbox(ctx context.Context, inbox EventInbox) (EventInbox, bool, error)
	UpdateInbox(ctx context.Context, inbox EventInbox) error

	CreateParticipation(ctx context.Context, participation Participation) (Participation, error)
	CreateGrantOrder(ctx context.Context, order GrantOrder) (GrantOrder, error)
	UpdateGrantOrderByCommandID(ctx context.Context, tenantID string, commandID string, status GrantStatus, message string) error
	ApplyRewardCallback(ctx context.Context, callback RewardCallback) (RewardCallbackResult, error)
	ApplyManualGrantDecision(ctx context.Context, cmd ManualGrantDecisionCommand) (RewardCallbackResult, error)
	CreateManualGrantExport(ctx context.Context, cmd ManualGrantExportCreateCommand) (ManualGrantExportCreateResult, error)
	ReleaseExpiredQuotaReservations(ctx context.Context, cmd QuotaTimeoutReleaseCommand) (QuotaTimeoutReleaseResult, error)
	ListRewardReconciliation(ctx context.Context, query RewardReconciliationQuery) ([]RewardReconciliationItem, error)
	CreateOutbox(ctx context.Context, message OutboxMessage) (OutboxMessage, error)
	CreateParticipationGrantOutboxesWithQuota(ctx context.Context, cmd AtomicGrantCommand) (AtomicGrantResult, error)

	CreateQuota(ctx context.Context, quota Quota) (Quota, error)
	ListActivityQuotas(ctx context.Context, activityID string) ([]Quota, error)
	ReserveQuota(ctx context.Context, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) (bool, error)

	ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error)
	ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]OutboxMessage, error)
	MarkOutboxSent(ctx context.Context, id string) error
	MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error
	MarkOutboxDLQ(ctx context.Context, id string, retryCount int, message string) error
	ReplayOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, replayedAt time.Time) (OutboxMessage, error)
	CloseOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, closedAt time.Time) (OutboxMessage, error)

	CreateAudit(ctx context.Context, record AuditRecord) error
	ListAudit(ctx context.Context, query AuditQuery) ([]AuditRecord, error)
}
