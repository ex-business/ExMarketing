package marketing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type ServiceType string
type ServiceAssetScope string
type ServiceAssetStatus string
type ServiceInterfaceOperation string
type ServiceProviderProtocol string
type ServiceSchemaType string
type ServicePrincipalType string
type ServiceProviderHealthStatus string
type ServiceProviderCircuitStatus string
type ServiceCallAuditStatus string
type ServiceRateLimitBackend string
type WorkerLeaseStatus string

const (
	ServiceTypeInternal ServiceType = "INTERNAL"
	ServiceTypeExternal ServiceType = "EXTERNAL"
	ServiceTypeSystem   ServiceType = "SYSTEM"

	ServiceAssetScopeSystem    ServiceAssetScope = "SYSTEM"
	ServiceAssetScopeTenant    ServiceAssetScope = "TENANT"
	ServiceAssetScopeWorkspace ServiceAssetScope = "WORKSPACE"
	ServiceAssetScopeFolder    ServiceAssetScope = "FOLDER"
	ServiceAssetScopeActivity  ServiceAssetScope = "ACTIVITY"
	ServiceAssetScopeNode      ServiceAssetScope = "NODE"

	ServiceAssetActive   ServiceAssetStatus = "ACTIVE"
	ServiceAssetDraft    ServiceAssetStatus = "DRAFT"
	ServiceAssetDisabled ServiceAssetStatus = "DISABLED"
	ServiceAssetDeleted  ServiceAssetStatus = "DELETED"

	ServiceOperationCall             ServiceInterfaceOperation = "CALL"
	ServiceOperationPublish          ServiceInterfaceOperation = "PUBLISH"
	ServiceOperationRequestReply     ServiceInterfaceOperation = "REQUEST_REPLY"
	ServiceOperationSubscribeTrigger ServiceInterfaceOperation = "SUBSCRIBE_TRIGGER"

	ServiceProviderHTTP       ServiceProviderProtocol = "HTTP"
	ServiceProviderRPC        ServiceProviderProtocol = "RPC"
	ServiceProviderMQ         ServiceProviderProtocol = "MQ"
	ServiceProviderPlugin     ServiceProviderProtocol = "PLUGIN"
	ServiceProviderTCP        ServiceProviderProtocol = "TCP"
	ServiceProviderLuaAdapter ServiceProviderProtocol = "LUA_ADAPTER"
	ServiceProviderResource   ServiceProviderProtocol = "RESOURCE"

	ServiceSchemaInput  ServiceSchemaType = "INPUT"
	ServiceSchemaOutput ServiceSchemaType = "OUTPUT"
	ServiceSchemaError  ServiceSchemaType = "ERROR"

	ServicePrincipalRole      ServicePrincipalType = "ROLE"
	ServicePrincipalUser      ServicePrincipalType = "USER"
	ServicePrincipalGroup     ServicePrincipalType = "GROUP"
	ServicePrincipalAccount   ServicePrincipalType = "SERVICE_ACCOUNT"
	ServicePrincipalWorkspace ServicePrincipalType = "WORKSPACE"
	ServicePrincipalFolder    ServicePrincipalType = "FOLDER"
	ServicePrincipalActivity  ServicePrincipalType = "ACTIVITY"
	ServicePrincipalNode      ServicePrincipalType = "NODE"

	ServiceProviderHealthUnknown   ServiceProviderHealthStatus = "UNKNOWN"
	ServiceProviderHealthHealthy   ServiceProviderHealthStatus = "HEALTHY"
	ServiceProviderHealthDegraded  ServiceProviderHealthStatus = "DEGRADED"
	ServiceProviderHealthUnhealthy ServiceProviderHealthStatus = "UNHEALTHY"

	ServiceProviderCircuitClosed   ServiceProviderCircuitStatus = "CLOSED"
	ServiceProviderCircuitOpen     ServiceProviderCircuitStatus = "OPEN"
	ServiceProviderCircuitHalfOpen ServiceProviderCircuitStatus = "HALF_OPEN"

	ServiceCallAuditSuccess ServiceCallAuditStatus = "SUCCESS"
	ServiceCallAuditFailed  ServiceCallAuditStatus = "FAILED"
	ServiceCallAuditDenied  ServiceCallAuditStatus = "DENIED"

	ServiceRateLimitBackendPostgres ServiceRateLimitBackend = "POSTGRES"
	ServiceRateLimitBackendRedis    ServiceRateLimitBackend = "REDIS"

	WorkerLeaseAcquired WorkerLeaseStatus = "ACQUIRED"
	WorkerLeaseHeld     WorkerLeaseStatus = "HELD"
)

type ServiceDefinition struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenantId"`
	WorkspaceID    string             `json:"workspaceId,omitempty"`
	FolderID       string             `json:"folderId,omitempty"`
	Code           string             `json:"code"`
	Name           string             `json:"name"`
	Description    string             `json:"description,omitempty"`
	ServiceType    ServiceType        `json:"serviceType"`
	Scope          ServiceAssetScope  `json:"scope"`
	Status         ServiceAssetStatus `json:"status"`
	Owner          string             `json:"owner,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	Builtin        bool               `json:"builtin,omitempty"`
	CatalogVersion string             `json:"catalogVersion,omitempty"`
	SourceHash     string             `json:"sourceHash,omitempty"`
	DeletedAt      *time.Time         `json:"deletedAt,omitempty"`
	Metadata       json.RawMessage    `json:"metadata,omitempty"`
	CreatedBy      string             `json:"createdBy,omitempty"`
	UpdatedBy      string             `json:"updatedBy,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type ServiceInterface struct {
	ID                     string                    `json:"id"`
	TenantID               string                    `json:"tenantId"`
	ServiceRef             string                    `json:"serviceRef"`
	Code                   string                    `json:"code"`
	Name                   string                    `json:"name"`
	Description            string                    `json:"description,omitempty"`
	Operation              ServiceInterfaceOperation `json:"operation"`
	InputSchemaRef         string                    `json:"inputSchemaRef,omitempty"`
	OutputSchemaRef        string                    `json:"outputSchemaRef,omitempty"`
	ErrorSchemaRef         string                    `json:"errorSchemaRef,omitempty"`
	IdempotencyKeyTemplate string                    `json:"idempotencyKeyTemplate,omitempty"`
	TimeoutMS              int                       `json:"timeoutMs"`
	RetryPolicy            string                    `json:"retryPolicy,omitempty"`
	LuaCallable            bool                      `json:"luaCallable"`
	Status                 ServiceAssetStatus        `json:"status"`
	Builtin                bool                      `json:"builtin,omitempty"`
	CatalogVersion         string                    `json:"catalogVersion,omitempty"`
	SourceHash             string                    `json:"sourceHash,omitempty"`
	DeletedAt              *time.Time                `json:"deletedAt,omitempty"`
	Metadata               json.RawMessage           `json:"metadata,omitempty"`
	CreatedBy              string                    `json:"createdBy,omitempty"`
	UpdatedBy              string                    `json:"updatedBy,omitempty"`
	CreatedAt              time.Time                 `json:"createdAt"`
	UpdatedAt              time.Time                 `json:"updatedAt"`
}

type ServiceProvider struct {
	ID             string                  `json:"id"`
	TenantID       string                  `json:"tenantId"`
	ServiceRef     string                  `json:"serviceRef"`
	InterfaceRef   string                  `json:"interfaceRef"`
	Code           string                  `json:"code"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description,omitempty"`
	Protocol       ServiceProviderProtocol `json:"protocol"`
	Endpoint       string                  `json:"endpoint,omitempty"`
	Topic          string                  `json:"topic,omitempty"`
	Method         string                  `json:"method,omitempty"`
	Path           string                  `json:"path,omitempty"`
	AuthRef        string                  `json:"authRef,omitempty"`
	ResourceRefs   []string                `json:"resourceRefs,omitempty"`
	Config         json.RawMessage         `json:"config,omitempty"`
	Weight         int                     `json:"weight,omitempty"`
	Priority       int                     `json:"priority,omitempty"`
	Environment    string                  `json:"environment,omitempty"`
	Status         ServiceAssetStatus      `json:"status"`
	Builtin        bool                    `json:"builtin,omitempty"`
	CatalogVersion string                  `json:"catalogVersion,omitempty"`
	SourceHash     string                  `json:"sourceHash,omitempty"`
	DeletedAt      *time.Time              `json:"deletedAt,omitempty"`
	CreatedBy      string                  `json:"createdBy,omitempty"`
	UpdatedBy      string                  `json:"updatedBy,omitempty"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
}

type ServiceSchema struct {
	ID             string             `json:"id"`
	TenantID       string             `json:"tenantId"`
	WorkspaceID    string             `json:"workspaceId,omitempty"`
	FolderID       string             `json:"folderId,omitempty"`
	SchemaRef      string             `json:"schemaRef"`
	SchemaType     ServiceSchemaType  `json:"schemaType"`
	Scope          ServiceAssetScope  `json:"scope"`
	Version        int                `json:"version"`
	Status         ServiceAssetStatus `json:"status"`
	SchemaJSON     json.RawMessage    `json:"schemaJson"`
	ExamplesJSON   json.RawMessage    `json:"examplesJson,omitempty"`
	Description    string             `json:"description,omitempty"`
	Builtin        bool               `json:"builtin,omitempty"`
	CatalogVersion string             `json:"catalogVersion,omitempty"`
	SourceHash     string             `json:"sourceHash,omitempty"`
	DeletedAt      *time.Time         `json:"deletedAt,omitempty"`
	CreatedBy      string             `json:"createdBy,omitempty"`
	UpdatedBy      string             `json:"updatedBy,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type ServiceInterfaceBinding struct {
	ID                       string                      `json:"id"`
	TenantID                 string                      `json:"tenantId"`
	ServiceRef               string                      `json:"serviceRef"`
	InterfaceRef             string                      `json:"interfaceRef"`
	Scope                    ServiceAssetScope           `json:"scope"`
	WorkspaceID              string                      `json:"workspaceId,omitempty"`
	FolderID                 string                      `json:"folderId,omitempty"`
	ActivityID               string                      `json:"activityId,omitempty"`
	NodeID                   string                      `json:"nodeId,omitempty"`
	PrincipalType            ServicePrincipalType        `json:"principalType"`
	PrincipalID              string                      `json:"principalId"`
	AllowedNodeTypes         []string                    `json:"allowedNodeTypes,omitempty"`
	AllowedOperations        []ServiceInterfaceOperation `json:"allowedOperations,omitempty"`
	AllowedProviderPolicies  []string                    `json:"allowedProviderPolicies,omitempty"`
	AllowExplicitProviderRef bool                        `json:"allowExplicitProviderRef,omitempty"`
	AllowedProviderRefs      []string                    `json:"allowedProviderRefs,omitempty"`
	IncludeSubfolders        bool                        `json:"includeSubfolders,omitempty"`
	MinRole                  WorkspaceRole               `json:"minRole,omitempty"`
	Environment              string                      `json:"environment,omitempty"`
	RateLimitRef             string                      `json:"rateLimitRef,omitempty"`
	LuaCallable              bool                        `json:"luaCallable,omitempty"`
	RequireApproval          bool                        `json:"requireApproval,omitempty"`
	Deny                     bool                        `json:"deny,omitempty"`
	Status                   ServiceAssetStatus          `json:"status"`
	Builtin                  bool                        `json:"builtin,omitempty"`
	CatalogVersion           string                      `json:"catalogVersion,omitempty"`
	SourceHash               string                      `json:"sourceHash,omitempty"`
	DeletedAt                *time.Time                  `json:"deletedAt,omitempty"`
	Metadata                 json.RawMessage             `json:"metadata,omitempty"`
	CreatedBy                string                      `json:"createdBy,omitempty"`
	UpdatedBy                string                      `json:"updatedBy,omitempty"`
	CreatedAt                time.Time                   `json:"createdAt"`
	UpdatedAt                time.Time                   `json:"updatedAt"`
}

type ServiceProviderRuntimeState struct {
	ID                       string                       `json:"id"`
	TenantID                 string                       `json:"tenantId"`
	ServiceRef               string                       `json:"serviceRef"`
	InterfaceRef             string                       `json:"interfaceRef"`
	ProviderRef              string                       `json:"providerRef"`
	HealthStatus             ServiceProviderHealthStatus  `json:"healthStatus"`
	HealthMessage            string                       `json:"healthMessage,omitempty"`
	CircuitStatus            ServiceProviderCircuitStatus `json:"circuitStatus"`
	FailureCount             int                          `json:"failureCount"`
	SuccessCount             int                          `json:"successCount"`
	ConsecutiveFailures      int                          `json:"consecutiveFailures"`
	RateLimitWindow          time.Time                    `json:"rateLimitWindow"`
	RateLimitCount           int                          `json:"rateLimitCount"`
	HalfOpenProbeCount       int                          `json:"halfOpenProbeCount,omitempty"`
	HalfOpenProbeLimit       int                          `json:"halfOpenProbeLimit,omitempty"`
	HalfOpenSuccessCount     int                          `json:"halfOpenSuccessCount,omitempty"`
	HalfOpenSuccessThreshold int                          `json:"halfOpenSuccessThreshold,omitempty"`
	LastFailureAt            *time.Time                   `json:"lastFailureAt,omitempty"`
	LastSuccessAt            *time.Time                   `json:"lastSuccessAt,omitempty"`
	CircuitOpenedAt          *time.Time                   `json:"circuitOpenedAt,omitempty"`
	CircuitResetAt           *time.Time                   `json:"circuitResetAt,omitempty"`
	Metadata                 json.RawMessage              `json:"metadata,omitempty"`
	UpdatedAt                time.Time                    `json:"updatedAt"`
	CreatedAt                time.Time                    `json:"createdAt"`
}

type ServiceProviderHealthSummary struct {
	TenantID         string                       `json:"tenantId"`
	ServiceRef       string                       `json:"serviceRef"`
	InterfaceRef     string                       `json:"interfaceRef"`
	ProviderRef      string                       `json:"providerRef"`
	ProviderName     string                       `json:"providerName,omitempty"`
	ProviderStatus   ServiceAssetStatus           `json:"providerStatus"`
	Protocol         ServiceProviderProtocol      `json:"protocol"`
	HealthStatus     ServiceProviderHealthStatus  `json:"healthStatus"`
	CircuitStatus    ServiceProviderCircuitStatus `json:"circuitStatus"`
	Status           string                       `json:"status"`
	HealthMessage    string                       `json:"healthMessage,omitempty"`
	RateLimitCount   int                          `json:"rateLimitCount,omitempty"`
	RateLimitWindow  *time.Time                   `json:"rateLimitWindow,omitempty"`
	LastCheckedAt    *time.Time                   `json:"lastCheckedAt,omitempty"`
	LastCheckedBy    string                       `json:"lastCheckedBy,omitempty"`
	LastSuccessAt    *time.Time                   `json:"lastSuccessAt,omitempty"`
	LastFailureAt    *time.Time                   `json:"lastFailureAt,omitempty"`
	CircuitOpenedAt  *time.Time                   `json:"circuitOpenedAt,omitempty"`
	CircuitResetAt   *time.Time                   `json:"circuitResetAt,omitempty"`
	RuntimeUpdatedAt time.Time                    `json:"runtimeUpdatedAt,omitempty"`
}

type ServiceProviderHealthSummaryQuery struct {
	TenantID      string
	ServiceRef    string
	InterfaceRef  string
	ProviderRef   string
	HealthStatus  ServiceProviderHealthStatus
	CircuitStatus ServiceProviderCircuitStatus
	Status        string
	Limit         int
	Offset        int
}

type WorkerLease struct {
	Name       string            `json:"name"`
	Owner      string            `json:"owner"`
	Status     WorkerLeaseStatus `json:"status"`
	LeaseTTL   time.Duration     `json:"-"`
	LeaseUntil time.Time         `json:"leaseUntil"`
	AcquiredAt time.Time         `json:"acquiredAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type WorkerLeaseAcquireCommand struct {
	Name  string
	Owner string
	TTL   time.Duration
	Now   time.Time
}

type ServiceRateLimitPolicy struct {
	ID             string                  `json:"id"`
	TenantID       string                  `json:"tenantId"`
	Code           string                  `json:"code"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description,omitempty"`
	ServiceRef     string                  `json:"serviceRef,omitempty"`
	InterfaceRef   string                  `json:"interfaceRef,omitempty"`
	ProviderRef    string                  `json:"providerRef,omitempty"`
	Limit          int                     `json:"limit"`
	WindowSeconds  int                     `json:"windowSeconds"`
	Backend        ServiceRateLimitBackend `json:"backend"`
	Status         ServiceAssetStatus      `json:"status"`
	Builtin        bool                    `json:"builtin,omitempty"`
	CatalogVersion string                  `json:"catalogVersion,omitempty"`
	SourceHash     string                  `json:"sourceHash,omitempty"`
	DeletedAt      *time.Time              `json:"deletedAt,omitempty"`
	Metadata       json.RawMessage         `json:"metadata,omitempty"`
	CreatedBy      string                  `json:"createdBy,omitempty"`
	UpdatedBy      string                  `json:"updatedBy,omitempty"`
	CreatedAt      time.Time               `json:"createdAt"`
	UpdatedAt      time.Time               `json:"updatedAt"`
}

type ServiceCallAuditLog struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	WorkspaceID     string                 `json:"workspaceId,omitempty"`
	FolderID        string                 `json:"folderId,omitempty"`
	ActivityID      string                 `json:"activityId,omitempty"`
	WorkflowID      string                 `json:"workflowId,omitempty"`
	NodeID          string                 `json:"nodeId,omitempty"`
	NodeType        string                 `json:"nodeType,omitempty"`
	ServiceRef      string                 `json:"serviceRef"`
	InterfaceRef    string                 `json:"interfaceRef"`
	ProviderRef     string                 `json:"providerRef,omitempty"`
	ResourceRef     string                 `json:"resourceRef,omitempty"`
	StepID          string                 `json:"stepId,omitempty"`
	Attempt         int                    `json:"attempt,omitempty"`
	Status          ServiceCallAuditStatus `json:"status"`
	ErrorCode       string                 `json:"errorCode,omitempty"`
	ErrorMessage    string                 `json:"errorMessage,omitempty"`
	LatencyMS       int64                  `json:"latencyMs"`
	RequestHash     string                 `json:"requestHash,omitempty"`
	ResponseHash    string                 `json:"responseHash,omitempty"`
	ProviderTrace   json.RawMessage        `json:"providerTrace,omitempty"`
	SchemaTrace     json.RawMessage        `json:"schemaTrace,omitempty"`
	PermissionTrace json.RawMessage        `json:"permissionTrace,omitempty"`
	Actor           string                 `json:"actor,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
}

type ServiceDefinitionQuery struct {
	TenantID       string
	WorkspaceID    string
	FolderID       string
	Status         ServiceAssetStatus
	Code           string
	IncludeDeleted bool
	Limit          int
}

type ServiceInterfaceQuery struct {
	TenantID       string
	ServiceRef     string
	Status         ServiceAssetStatus
	Code           string
	IncludeDeleted bool
	Limit          int
}

type ServiceProviderQuery struct {
	TenantID       string
	ServiceRef     string
	InterfaceRef   string
	Protocol       ServiceProviderProtocol
	Status         ServiceAssetStatus
	Code           string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

type ServiceSchemaQuery struct {
	TenantID       string
	WorkspaceID    string
	FolderID       string
	SchemaRef      string
	SchemaType     ServiceSchemaType
	Status         ServiceAssetStatus
	IncludeDeleted bool
	Limit          int
}

type ServiceInterfaceBindingQuery struct {
	TenantID       string
	ServiceRef     string
	InterfaceRef   string
	Scope          ServiceAssetScope
	WorkspaceID    string
	FolderID       string
	ActivityID     string
	NodeID         string
	NodeType       string
	Environment    string
	Status         ServiceAssetStatus
	IncludeDeleted bool
	Limit          int
}

type ServiceProviderRuntimeStateQuery struct {
	TenantID      string
	ServiceRef    string
	InterfaceRef  string
	ProviderRef   string
	HealthStatus  ServiceProviderHealthStatus
	CircuitStatus ServiceProviderCircuitStatus
	Limit         int
	Offset        int
}

type ServiceRateLimitPolicyQuery struct {
	TenantID       string
	Code           string
	ServiceRef     string
	InterfaceRef   string
	ProviderRef    string
	Status         ServiceAssetStatus
	IncludeDeleted bool
	Limit          int
}

type ServiceProviderRateLimitConsume struct {
	State       ServiceProviderRuntimeState
	PolicyRef   string
	Scope       string
	SubjectType string
	SubjectID   string
	SubjectHash string
	Limit       int
	Window      time.Duration
	Now         time.Time
}

type ServiceCallAuditQuery struct {
	TenantID     string
	WorkspaceID  string
	FolderID     string
	ActivityID   string
	WorkflowID   string
	NodeID       string
	ServiceRef   string
	InterfaceRef string
	ProviderRef  string
	ResourceRef  string
	Status       ServiceCallAuditStatus
	ErrorCode    string
	Cursor       string
	Limit        int
}

func (s ServiceDefinition) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(s.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceType(s.ServiceType) {
		return errors.Join(ErrInvalidResource, errors.New("serviceType is invalid"))
	}
	if !validServiceAssetScope(s.Scope) {
		return errors.Join(ErrInvalidResource, errors.New("scope is invalid"))
	}
	if !validServiceAssetStatus(s.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(s.Metadata) > 0 && string(s.Metadata) != "{}" && !jsonObjectValid(s.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (i ServiceInterface) Validate() error {
	if strings.TrimSpace(i.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(i.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(i.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceInterfaceOperation(i.Operation) {
		return errors.Join(ErrInvalidResource, errors.New("operation is invalid"))
	}
	if i.TimeoutMS < 0 {
		return errors.Join(ErrInvalidResource, errors.New("timeoutMs must be non-negative"))
	}
	if !validServiceAssetStatus(i.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(i.Metadata) > 0 && string(i.Metadata) != "{}" && !jsonObjectValid(i.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (p ServiceProvider) Validate() error {
	if strings.TrimSpace(p.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(p.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(p.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if strings.TrimSpace(p.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceProviderProtocol(p.Protocol) {
		return errors.Join(ErrInvalidResource, errors.New("protocol is invalid"))
	}
	if !validServiceAssetStatus(p.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(p.Config) > 0 && string(p.Config) != "{}" && !jsonObjectValid(p.Config) {
		return errors.Join(ErrInvalidResource, errors.New("config must be json object"))
	}
	return nil
}

func (s ServiceSchema) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(s.SchemaRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("schemaRef is required"))
	}
	if !validServiceSchemaType(s.SchemaType) {
		return errors.Join(ErrInvalidResource, errors.New("schemaType is invalid"))
	}
	if !validServiceAssetScope(s.Scope) {
		return errors.Join(ErrInvalidResource, errors.New("scope is invalid"))
	}
	if s.Version <= 0 {
		return errors.Join(ErrInvalidResource, errors.New("version must be positive"))
	}
	if !validServiceAssetStatus(s.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(s.SchemaJSON) == 0 || !jsonObjectValid(s.SchemaJSON) {
		return errors.Join(ErrInvalidResource, errors.New("schemaJson must be json object"))
	}
	if len(s.ExamplesJSON) > 0 && !json.Valid(s.ExamplesJSON) {
		return errors.Join(ErrInvalidResource, errors.New("examplesJson must be valid json"))
	}
	return nil
}

func (b ServiceInterfaceBinding) Validate() error {
	if strings.TrimSpace(b.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(b.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(b.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if !validServiceAssetScope(b.Scope) {
		return errors.Join(ErrInvalidResource, errors.New("scope is invalid"))
	}
	if !validServicePrincipalType(b.PrincipalType) {
		return errors.Join(ErrInvalidResource, errors.New("principalType is invalid"))
	}
	if strings.TrimSpace(b.PrincipalID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("principalId is required"))
	}
	if !validServiceAssetStatus(b.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	for _, operation := range b.AllowedOperations {
		if !validServiceInterfaceOperation(operation) {
			return errors.Join(ErrInvalidResource, errors.New("allowedOperations contains invalid operation"))
		}
	}
	if b.AllowExplicitProviderRef && len(b.AllowedProviderRefs) == 0 {
		return errors.Join(ErrInvalidResource, errors.New("allowedProviderRefs is required when explicit provider ref is allowed"))
	}
	if b.MinRole != "" && !validServiceBindingMinRole(b.MinRole) {
		return errors.Join(ErrInvalidResource, errors.New("minRole is invalid"))
	}
	if len(b.Metadata) > 0 && string(b.Metadata) != "{}" && !jsonObjectValid(b.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func validServiceBindingMinRole(role WorkspaceRole) bool {
	switch role {
	case WorkspaceRoleViewer, WorkspaceRoleEditor, WorkspaceRoleAdmin, WorkspaceRoleOwner:
		return true
	default:
		return false
	}
}

func (s ServiceProviderRuntimeState) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(s.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(s.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if strings.TrimSpace(s.ProviderRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("providerRef is required"))
	}
	if !validServiceProviderHealthStatus(s.HealthStatus) {
		return errors.Join(ErrInvalidResource, errors.New("healthStatus is invalid"))
	}
	if !validServiceProviderCircuitStatus(s.CircuitStatus) {
		return errors.Join(ErrInvalidResource, errors.New("circuitStatus is invalid"))
	}
	if s.RateLimitCount < 0 || s.FailureCount < 0 || s.SuccessCount < 0 || s.ConsecutiveFailures < 0 {
		return errors.Join(ErrInvalidResource, errors.New("runtime counters must be non-negative"))
	}
	if s.HalfOpenProbeCount < 0 || s.HalfOpenProbeLimit < 0 || s.HalfOpenSuccessCount < 0 || s.HalfOpenSuccessThreshold < 0 {
		return errors.Join(ErrInvalidResource, errors.New("half-open counters must be non-negative"))
	}
	if len(s.Metadata) > 0 && string(s.Metadata) != "{}" && !jsonObjectValid(s.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (p ServiceRateLimitPolicy) Validate() error {
	if strings.TrimSpace(p.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(p.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if p.Limit < 0 {
		return errors.Join(ErrInvalidResource, errors.New("limit must be non-negative"))
	}
	if p.WindowSeconds <= 0 {
		return errors.Join(ErrInvalidResource, errors.New("windowSeconds must be positive"))
	}
	if strings.TrimSpace(p.ProviderRef) != "" && (strings.TrimSpace(p.ServiceRef) == "" || strings.TrimSpace(p.InterfaceRef) == "") {
		return errors.Join(ErrInvalidResource, errors.New("providerRef requires serviceRef and interfaceRef"))
	}
	if strings.TrimSpace(p.InterfaceRef) != "" && strings.TrimSpace(p.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef requires serviceRef"))
	}
	if !validServiceRateLimitBackend(p.Backend) {
		return errors.Join(ErrInvalidResource, errors.New("backend is invalid"))
	}
	if !validServiceAssetStatus(p.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(p.Metadata) > 0 && string(p.Metadata) != "{}" && !jsonObjectValid(p.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (a ServiceCallAuditLog) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(a.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(a.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if !validServiceCallAuditStatus(a.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if a.LatencyMS < 0 {
		return errors.Join(ErrInvalidResource, errors.New("latencyMs must be non-negative"))
	}
	if a.Attempt < 0 {
		return errors.Join(ErrInvalidResource, errors.New("attempt must be non-negative"))
	}
	return nil
}

func validServiceType(value ServiceType) bool {
	switch value {
	case ServiceTypeInternal, ServiceTypeExternal, ServiceTypeSystem:
		return true
	default:
		return false
	}
}

func validServiceAssetScope(value ServiceAssetScope) bool {
	switch value {
	case ServiceAssetScopeSystem, ServiceAssetScopeTenant, ServiceAssetScopeWorkspace, ServiceAssetScopeFolder, ServiceAssetScopeActivity, ServiceAssetScopeNode:
		return true
	default:
		return false
	}
}

func validServiceAssetStatus(value ServiceAssetStatus) bool {
	switch value {
	case ServiceAssetActive, ServiceAssetDraft, ServiceAssetDisabled, ServiceAssetDeleted:
		return true
	default:
		return false
	}
}

func validServiceInterfaceOperation(value ServiceInterfaceOperation) bool {
	switch value {
	case ServiceOperationCall, ServiceOperationPublish, ServiceOperationRequestReply, ServiceOperationSubscribeTrigger:
		return true
	default:
		return false
	}
}

func validServiceProviderProtocol(value ServiceProviderProtocol) bool {
	switch value {
	case ServiceProviderHTTP, ServiceProviderRPC, ServiceProviderMQ, ServiceProviderPlugin, ServiceProviderTCP, ServiceProviderLuaAdapter, ServiceProviderResource:
		return true
	default:
		return false
	}
}

func validServiceSchemaType(value ServiceSchemaType) bool {
	switch value {
	case ServiceSchemaInput, ServiceSchemaOutput, ServiceSchemaError:
		return true
	default:
		return false
	}
}

func validServicePrincipalType(value ServicePrincipalType) bool {
	switch value {
	case ServicePrincipalRole, ServicePrincipalUser, ServicePrincipalGroup, ServicePrincipalAccount, ServicePrincipalWorkspace, ServicePrincipalFolder, ServicePrincipalActivity, ServicePrincipalNode:
		return true
	default:
		return false
	}
}

func validServiceProviderHealthStatus(value ServiceProviderHealthStatus) bool {
	switch value {
	case ServiceProviderHealthUnknown, ServiceProviderHealthHealthy, ServiceProviderHealthDegraded, ServiceProviderHealthUnhealthy:
		return true
	default:
		return false
	}
}

func validServiceProviderCircuitStatus(value ServiceProviderCircuitStatus) bool {
	switch value {
	case ServiceProviderCircuitClosed, ServiceProviderCircuitOpen, ServiceProviderCircuitHalfOpen:
		return true
	default:
		return false
	}
}

func validServiceRateLimitBackend(value ServiceRateLimitBackend) bool {
	switch value {
	case ServiceRateLimitBackendPostgres, ServiceRateLimitBackendRedis:
		return true
	default:
		return false
	}
}

func validServiceCallAuditStatus(value ServiceCallAuditStatus) bool {
	switch value {
	case ServiceCallAuditSuccess, ServiceCallAuditFailed, ServiceCallAuditDenied:
		return true
	default:
		return false
	}
}

func jsonObjectValid(raw json.RawMessage) bool {
	var mapped map[string]any
	return json.Unmarshal(raw, &mapped) == nil && mapped != nil
}
