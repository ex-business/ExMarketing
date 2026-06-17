package marketing

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

type ResourceType string
type ResourceDriver string
type ResourceScope string
type ResourceStatus string
type ResourceOperation string
type ResourceBindingStatus string
type ResourceAuditStatus string

const (
	ResourceMemory          ResourceType = "MEMORY"
	ResourceWorkflowContext ResourceType = "WORKFLOW_CONTEXT"
	ResourceDatabase        ResourceType = "DATABASE"
	ResourceDatabaseKV      ResourceType = "DATABASE_KV"
	ResourceDatabaseTable   ResourceType = "DATABASE_TABLE"
	ResourceRedisKV         ResourceType = "REDIS_KV"
	ResourceRedisHash       ResourceType = "REDIS_HASH"
	ResourceRedisCounter    ResourceType = "REDIS_COUNTER"
	ResourceRedisLock       ResourceType = "REDIS_LOCK"
	ResourceHTTPProvider    ResourceType = "HTTP_PROVIDER"
	ResourceGRPCProvider    ResourceType = "GRPC_PROVIDER"
	ResourceMQTopic         ResourceType = "MQ_TOPIC"
	ResourceObjectStorage   ResourceType = "OBJECT_STORAGE"
	ResourcePluginProvider  ResourceType = "PLUGIN_PROVIDER"
	ResourceCustomProvider  ResourceType = "CUSTOM_PROVIDER"

	ResourceDriverInternal ResourceDriver = "INTERNAL"
	ResourceDriverPostgres ResourceDriver = "POSTGRES"
	ResourceDriverHTTP     ResourceDriver = "HTTP"
	ResourceDriverREST     ResourceDriver = "REST"

	ResourceScopeSystem    ResourceScope = "SYSTEM"
	ResourceScopeTenant    ResourceScope = "TENANT"
	ResourceScopeWorkspace ResourceScope = "WORKSPACE"
	ResourceScopeFolder    ResourceScope = "FOLDER"
	ResourceScopeActivity  ResourceScope = "ACTIVITY"
	ResourceScopeNode      ResourceScope = "NODE"

	ResourceActive   ResourceStatus = "ACTIVE"
	ResourceDraft    ResourceStatus = "DRAFT"
	ResourceDisabled ResourceStatus = "DISABLED"
	ResourceDeleted  ResourceStatus = "DELETED"

	ResourceBindingActive   ResourceBindingStatus = "ACTIVE"
	ResourceBindingDisabled ResourceBindingStatus = "DISABLED"

	ResourceAuditSuccess ResourceAuditStatus = "SUCCESS"
	ResourceAuditFailed  ResourceAuditStatus = "FAILED"

	ResourceRead              ResourceOperation = "READ"
	ResourceWrite             ResourceOperation = "WRITE"
	ResourceUpsert            ResourceOperation = "UPSERT"
	ResourceAppend            ResourceOperation = "APPEND"
	ResourceDelete            ResourceOperation = "DELETE"
	ResourceCAS               ResourceOperation = "CAS"
	ResourceCall              ResourceOperation = "CALL"
	ResourcePublish           ResourceOperation = "PUBLISH"
	ResourceTestConnection    ResourceOperation = "TEST_CONNECTION"
	ResourceCheckAndIncrement ResourceOperation = "CHECK_AND_INCREMENT"
)

type Resource struct {
	ID            string              `json:"id"`
	TenantID      string              `json:"tenantId"`
	WorkspaceID   string              `json:"workspaceId,omitempty"`
	FolderID      string              `json:"folderId,omitempty"`
	ActivityID    string              `json:"activityId,omitempty"`
	Code          string              `json:"code"`
	Name          string              `json:"name"`
	Description   string              `json:"description,omitempty"`
	Type          ResourceType        `json:"type"`
	Driver        ResourceDriver      `json:"driver"`
	Scope         ResourceScope       `json:"scope"`
	Status        ResourceStatus      `json:"status"`
	Environment   string              `json:"environment,omitempty"`
	Config        json.RawMessage     `json:"config,omitempty"`
	Capabilities  []ResourceOperation `json:"capabilities"`
	SchemaVersion int                 `json:"schemaVersion"`
	CreatedBy     string              `json:"createdBy,omitempty"`
	UpdatedBy     string              `json:"updatedBy,omitempty"`
	CreatedAt     time.Time           `json:"createdAt"`
	UpdatedAt     time.Time           `json:"updatedAt"`
}

type ResourceQuery struct {
	TenantID    string
	WorkspaceID string
	FolderID    string
	ActivityID  string
	Type        ResourceType
	Status      ResourceStatus
	Code        string
	Limit       int
}

type ResourceBinding struct {
	ID                string                 `json:"id"`
	ResourceID        string                 `json:"resourceId"`
	TenantID          string                 `json:"tenantId"`
	WorkspaceID       string                 `json:"workspaceId,omitempty"`
	FolderID          string                 `json:"folderId,omitempty"`
	ActivityID        string                 `json:"activityId,omitempty"`
	NodeType          string                 `json:"nodeType,omitempty"`
	NodeID            string                 `json:"nodeId,omitempty"`
	PrincipalType     WorkspacePrincipalType `json:"principalType,omitempty"`
	PrincipalID       string                 `json:"principalId,omitempty"`
	MinRole           WorkspaceRole          `json:"minRole,omitempty"`
	AllowedOperations []ResourceOperation    `json:"allowedOperations,omitempty"`
	AllowedNamespaces []string               `json:"allowedNamespaces,omitempty"`
	KeyPolicy         json.RawMessage        `json:"keyPolicy,omitempty"`
	IncludeSubfolders bool                   `json:"includeSubfolders"`
	RequireApproval   bool                   `json:"requireApproval"`
	Status            ResourceBindingStatus  `json:"status"`
	CreatedBy         string                 `json:"createdBy,omitempty"`
	UpdatedBy         string                 `json:"updatedBy,omitempty"`
	CreatedAt         time.Time              `json:"createdAt"`
	UpdatedAt         time.Time              `json:"updatedAt"`
}

type ResourceBindingQuery struct {
	ResourceID  string
	TenantID    string
	WorkspaceID string
	FolderID    string
	ActivityID  string
	NodeType    string
	Status      ResourceBindingStatus
	Limit       int
}

type ResourceAuditLog struct {
	ID                  string              `json:"id"`
	TenantID            string              `json:"tenantId"`
	WorkspaceID         string              `json:"workspaceId,omitempty"`
	FolderID            string              `json:"folderId,omitempty"`
	ActivityID          string              `json:"activityId,omitempty"`
	WorkflowExecutionID string              `json:"workflowExecutionId,omitempty"`
	NodeID              string              `json:"nodeId,omitempty"`
	NodeType            string              `json:"nodeType,omitempty"`
	ResourceID          string              `json:"resourceId"`
	ResourceCode        string              `json:"resourceCode"`
	Operation           ResourceOperation   `json:"operation"`
	Status              ResourceAuditStatus `json:"status"`
	DurationMS          int                 `json:"durationMs"`
	ErrorCode           string              `json:"errorCode,omitempty"`
	ErrorMessage        string              `json:"errorMessage,omitempty"`
	ActorID             string              `json:"actorId,omitempty"`
	CreatedAt           time.Time           `json:"createdAt"`
}

type ResourceAuditQuery struct {
	ResourceID  string
	TenantID    string
	WorkspaceID string
	ActivityID  string
	Status      ResourceAuditStatus
	Limit       int
}

type ResourceAccessRequest struct {
	TenantID    string            `json:"tenantId"`
	WorkspaceID string            `json:"workspaceId,omitempty"`
	FolderID    string            `json:"folderId,omitempty"`
	ActivityID  string            `json:"activityId,omitempty"`
	NodeID      string            `json:"nodeId,omitempty"`
	NodeType    string            `json:"nodeType,omitempty"`
	Operation   ResourceOperation `json:"operation"`
	Namespace   string            `json:"namespace,omitempty"`
	ResourceKey string            `json:"resourceKey,omitempty"`
	ActorID     string            `json:"actorId,omitempty"`
	ActorRole   WorkspaceRole     `json:"actorRole,omitempty"`
}

type AvailableResource struct {
	Resource          Resource `json:"resource"`
	Available         bool     `json:"available"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
}

func (r Resource) ConfigMap() map[string]any {
	if len(r.Config) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(r.Config, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func (r Resource) Supports(operation ResourceOperation) bool {
	if operation == "" {
		return true
	}
	for _, capability := range r.Capabilities {
		if capability == operation {
			return true
		}
	}
	return false
}

func (r Resource) Validate() error {
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(r.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validResourceType(r.Type) {
		return errors.Join(ErrInvalidResource, errors.New("type is invalid"))
	}
	if strings.TrimSpace(string(r.Driver)) == "" {
		return errors.Join(ErrInvalidResource, errors.New("driver is required"))
	}
	if !validResourceScope(r.Scope) {
		return errors.Join(ErrInvalidResource, errors.New("scope is invalid"))
	}
	if !validResourceStatus(r.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(r.Config) == 0 {
		r.Config = json.RawMessage(`{}`)
	}
	return nil
}

func (b ResourceBinding) Allows(req ResourceAccessRequest) bool {
	if b.Status != "" && b.Status != ResourceBindingActive {
		return false
	}
	if b.TenantID != "" && req.TenantID != "" && b.TenantID != req.TenantID {
		return false
	}
	if b.WorkspaceID != "" && b.WorkspaceID != req.WorkspaceID {
		return false
	}
	if b.FolderID != "" && b.FolderID != req.FolderID && !b.IncludeSubfolders {
		return false
	}
	if b.ActivityID != "" && b.ActivityID != req.ActivityID {
		return false
	}
	if b.NodeType != "" && b.NodeType != req.NodeType {
		return false
	}
	if b.NodeID != "" && b.NodeID != req.NodeID {
		return false
	}
	if len(b.AllowedOperations) > 0 && !slices.Contains(b.AllowedOperations, req.Operation) {
		return false
	}
	if len(b.AllowedNamespaces) > 0 && req.Namespace != "" && !slices.Contains(b.AllowedNamespaces, req.Namespace) {
		return false
	}
	if b.MinRole != "" && req.ActorRole != "" && workspaceRoleRank(req.ActorRole) < workspaceRoleRank(b.MinRole) {
		return false
	}
	if !resourceKeyPolicyAllows(b.KeyPolicy, req.ResourceKey) {
		return false
	}
	return true
}

func (b ResourceBinding) Validate() error {
	if strings.TrimSpace(b.ResourceID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("resourceId is required"))
	}
	if strings.TrimSpace(b.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if b.Status != "" && b.Status != ResourceBindingActive && b.Status != ResourceBindingDisabled {
		return errors.Join(ErrInvalidResource, errors.New("binding status is invalid"))
	}
	if b.PrincipalType != "" && !validWorkspacePrincipalType(b.PrincipalType) {
		return errors.Join(ErrInvalidResource, errors.New("principalType is invalid"))
	}
	if b.MinRole != "" && !validWorkspaceRole(b.MinRole) {
		return errors.Join(ErrInvalidResource, errors.New("minRole is invalid"))
	}
	for _, operation := range b.AllowedOperations {
		if !validResourceOperation(operation) {
			return errors.Join(ErrInvalidResource, errors.New("allowedOperations contains invalid operation"))
		}
	}
	if len(b.KeyPolicy) > 0 && string(b.KeyPolicy) != "{}" {
		var policy map[string]any
		if err := json.Unmarshal(b.KeyPolicy, &policy); err != nil {
			return errors.Join(ErrInvalidResource, errors.New("keyPolicy must be json object"))
		}
		if policy == nil {
			return errors.Join(ErrInvalidResource, errors.New("keyPolicy must be json object"))
		}
	}
	return nil
}

func validResourceType(value ResourceType) bool {
	switch value {
	case ResourceMemory, ResourceWorkflowContext, ResourceDatabase, ResourceDatabaseKV, ResourceDatabaseTable, ResourceRedisKV, ResourceRedisHash, ResourceRedisCounter, ResourceRedisLock, ResourceHTTPProvider, ResourceGRPCProvider, ResourceMQTopic, ResourceObjectStorage, ResourcePluginProvider, ResourceCustomProvider:
		return true
	default:
		return false
	}
}

func validResourceScope(value ResourceScope) bool {
	switch value {
	case ResourceScopeSystem, ResourceScopeTenant, ResourceScopeWorkspace, ResourceScopeFolder, ResourceScopeActivity, ResourceScopeNode:
		return true
	default:
		return false
	}
}

func validResourceStatus(value ResourceStatus) bool {
	switch value {
	case ResourceActive, ResourceDraft, ResourceDisabled, ResourceDeleted:
		return true
	default:
		return false
	}
}

func validResourceOperation(value ResourceOperation) bool {
	switch value {
	case ResourceRead, ResourceWrite, ResourceUpsert, ResourceAppend, ResourceDelete, ResourceCAS, ResourceCall, ResourcePublish, ResourceTestConnection, ResourceCheckAndIncrement:
		return true
	default:
		return false
	}
}

func workspaceRoleRank(role WorkspaceRole) int {
	switch role {
	case WorkspaceRoleOwner:
		return 5
	case WorkspaceRoleAdmin:
		return 4
	case WorkspaceRoleEditor:
		return 3
	case WorkspaceRoleViewer:
		return 2
	case WorkspaceRoleGuest:
		return 1
	default:
		return 0
	}
}

func resourceKeyPolicyAllows(raw json.RawMessage, resourceKey string) bool {
	if len(raw) == 0 || string(raw) == "{}" || strings.TrimSpace(resourceKey) == "" {
		return true
	}
	var policy struct {
		AllowedPrefixes []string `json:"allowedPrefixes"`
		DeniedPrefixes  []string `json:"deniedPrefixes"`
		ExactKeys       []string `json:"exactKeys"`
	}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return false
	}
	for _, prefix := range policy.DeniedPrefixes {
		if prefix != "" && strings.HasPrefix(resourceKey, prefix) {
			return false
		}
	}
	if len(policy.ExactKeys) > 0 {
		return slices.Contains(policy.ExactKeys, resourceKey)
	}
	if len(policy.AllowedPrefixes) == 0 {
		return true
	}
	for _, prefix := range policy.AllowedPrefixes {
		if prefix != "" && strings.HasPrefix(resourceKey, prefix) {
			return true
		}
	}
	return false
}
