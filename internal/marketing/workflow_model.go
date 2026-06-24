package marketing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type WorkflowVersionStatus string
type WorkflowExecutionStatus string
type WorkflowNodeExecutionStatus string
type WorkflowEdgeRouteType string
type WorkflowDraftTemplateStatus string
type WorkflowDraftTemplateSource string

const (
	WorkflowVersionDraft      WorkflowVersionStatus = "DRAFT"
	WorkflowVersionActive     WorkflowVersionStatus = "ACTIVE"
	WorkflowVersionSuperseded WorkflowVersionStatus = "SUPERSEDED"
	WorkflowVersionArchived   WorkflowVersionStatus = "ARCHIVED"

	WorkflowExecutionRunning  WorkflowExecutionStatus = "RUNNING"
	WorkflowExecutionSuccess  WorkflowExecutionStatus = "SUCCESS"
	WorkflowExecutionFailed   WorkflowExecutionStatus = "FAILED"
	WorkflowExecutionCanceled WorkflowExecutionStatus = "CANCELED"

	WorkflowNodeRunning WorkflowNodeExecutionStatus = "RUNNING"
	WorkflowNodeSuccess WorkflowNodeExecutionStatus = "SUCCESS"
	WorkflowNodeFailed  WorkflowNodeExecutionStatus = "FAILED"
	WorkflowNodeSkipped WorkflowNodeExecutionStatus = "SKIPPED"

	WorkflowEdgeRouteDefault WorkflowEdgeRouteType = "DEFAULT"
	WorkflowEdgeRouteOutput  WorkflowEdgeRouteType = "OUTPUT"
	WorkflowEdgeRouteIF      WorkflowEdgeRouteType = "IF"
	WorkflowEdgeRouteELIF    WorkflowEdgeRouteType = "ELIF"
	WorkflowEdgeRouteELSE    WorkflowEdgeRouteType = "ELSE"
	WorkflowEdgeRoutePASS    WorkflowEdgeRouteType = "PASS"
	WorkflowEdgeRouteBLOCK   WorkflowEdgeRouteType = "BLOCK"
	WorkflowEdgeRouteRISK    WorkflowEdgeRouteType = "RISK"
	WorkflowEdgeRouteSKIP    WorkflowEdgeRouteType = "SKIP"

	WorkflowDraftTemplateActive   WorkflowDraftTemplateStatus = "ACTIVE"
	WorkflowDraftTemplateArchived WorkflowDraftTemplateStatus = "ARCHIVED"

	WorkflowDraftTemplateBuiltin WorkflowDraftTemplateSource = "BUILTIN"
	WorkflowDraftTemplateSaved   WorkflowDraftTemplateSource = "SAVED"
)

type WorkflowDefinition struct {
	Nodes    []WorkflowNode          `json:"nodes"`
	Edges    []WorkflowEdge          `json:"edges"`
	Viewport map[string]float64      `json:"viewport,omitempty"`
	Meta     map[string]any          `json:"meta,omitempty"`
	Raw      json.RawMessage         `json:"-"`
	Index    map[string]WorkflowNode `json:"-"`
}

type WorkflowNode struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Kind     string           `json:"kind,omitempty"`
	Code     string           `json:"code,omitempty"`
	Title    string           `json:"title,omitempty"`
	Position WorkflowPosition `json:"position,omitempty"`
	Inputs   []string         `json:"inputs,omitempty"`
	Outputs  []string         `json:"outputs,omitempty"`
	Config   json.RawMessage  `json:"config,omitempty"`
}

type WorkflowNodeInputMappingRef struct {
	SourceNodeID string `json:"sourceNodeId"`
	OutputKey    string `json:"outputKey"`
	Path         string `json:"path,omitempty"`
}

type ProviderChainTargetType string

const (
	ProviderChainTargetServiceInterface ProviderChainTargetType = "SERVICE_INTERFACE"
	ProviderChainTargetResource         ProviderChainTargetType = "RESOURCE"
	ProviderChainTargetWorkflowContext  ProviderChainTargetType = "WORKFLOW_CONTEXT"
	ProviderChainTargetLocalCache       ProviderChainTargetType = "LOCAL_CACHE"
	ProviderChainTargetProvider         ProviderChainTargetType = "PROVIDER"
	ProviderChainTargetFactService      ProviderChainTargetType = "FACT_SERVICE"
)

type ProviderChainStep struct {
	TargetType     ProviderChainTargetType `json:"targetType,omitempty"`
	Source         string                  `json:"source,omitempty"`
	ServiceRef     string                  `json:"serviceRef,omitempty"`
	InterfaceRef   string                  `json:"interfaceRef,omitempty"`
	ProviderPolicy string                  `json:"providerPolicy,omitempty"`
	ProviderRef    string                  `json:"providerRef,omitempty"`
	ResourceRef    string                  `json:"resourceRef,omitempty"`
	Operation      string                  `json:"operation,omitempty"`
	Namespace      string                  `json:"namespace,omitempty"`
	KeyTemplate    string                  `json:"keyTemplate,omitempty"`
	InputMapping   map[string]any          `json:"inputMapping,omitempty"`
	ValuePath      string                  `json:"valuePath,omitempty"`
	OutputAs       string                  `json:"outputAs,omitempty"`
	TimeoutMS      int                     `json:"timeoutMs,omitempty"`
	OnMiss         string                  `json:"onMiss,omitempty"`
}

type WorkflowPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowEdge struct {
	ID           string             `json:"id"`
	Source       string             `json:"source"`
	Target       string             `json:"target"`
	SourceHandle string             `json:"sourceHandle,omitempty"`
	TargetHandle string             `json:"targetHandle,omitempty"`
	Label        string             `json:"label,omitempty"`
	Route        *WorkflowEdgeRoute `json:"route,omitempty"`
}

type WorkflowEdgeRoute struct {
	Type         WorkflowEdgeRouteType `json:"type"`
	Key          string                `json:"key,omitempty"`
	Label        string                `json:"label,omitempty"`
	Expression   string                `json:"expression,omitempty"`
	ConditionRef string                `json:"conditionRef,omitempty"`
	Default      bool                  `json:"default,omitempty"`
}

func (route WorkflowEdgeRoute) NormalizedType() WorkflowEdgeRouteType {
	return WorkflowEdgeRouteType(strings.ToUpper(strings.TrimSpace(string(route.Type))))
}

func (route WorkflowEdgeRoute) NormalizedKey() string {
	for _, value := range []string{route.Key, route.Label, string(route.NormalizedType())} {
		value = strings.TrimSpace(value)
		if value != "" {
			return strings.ToUpper(value)
		}
	}
	return ""
}

func (route WorkflowEdgeRoute) IsSupported() bool {
	switch route.NormalizedType() {
	case WorkflowEdgeRouteDefault,
		WorkflowEdgeRouteOutput,
		WorkflowEdgeRouteIF,
		WorkflowEdgeRouteELIF,
		WorkflowEdgeRouteELSE,
		WorkflowEdgeRoutePASS,
		WorkflowEdgeRouteBLOCK,
		WorkflowEdgeRouteRISK,
		WorkflowEdgeRouteSKIP:
		return true
	default:
		return false
	}
}

func (route WorkflowEdgeRoute) IsDefault() bool {
	if route.Default {
		return true
	}
	switch route.NormalizedType() {
	case WorkflowEdgeRouteDefault, WorkflowEdgeRouteELSE:
		return true
	default:
		return false
	}
}

func (route WorkflowEdgeRoute) RequiresCondition() bool {
	if strings.TrimSpace(route.Expression) != "" || strings.TrimSpace(route.ConditionRef) != "" {
		return false
	}
	switch route.NormalizedType() {
	case WorkflowEdgeRouteIF,
		WorkflowEdgeRouteELIF,
		WorkflowEdgeRoutePASS,
		WorkflowEdgeRouteBLOCK,
		WorkflowEdgeRouteRISK,
		WorkflowEdgeRouteSKIP:
		return true
	default:
		return false
	}
}

func (route WorkflowEdgeRoute) OutputKey() string {
	key := route.NormalizedKey()
	switch key {
	case "", string(WorkflowEdgeRouteOutput), string(WorkflowEdgeRouteDefault):
		return ""
	default:
		return key
	}
}

type WorkflowDraft struct {
	ActivityID string          `json:"activityId"`
	TenantID   string          `json:"tenantId"`
	Definition json.RawMessage `json:"definition"`
	UpdatedBy  string          `json:"updatedBy,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type WorkflowVersion struct {
	ID              string                `json:"id"`
	TenantID        string                `json:"tenantId"`
	ActivityID      string                `json:"activityId"`
	Version         int                   `json:"version"`
	Status          WorkflowVersionStatus `json:"status"`
	Definition      json.RawMessage       `json:"definition"`
	PublishedBy     string                `json:"publishedBy,omitempty"`
	PublishedAt     time.Time             `json:"publishedAt"`
	ReleaseNote     string                `json:"releaseNote,omitempty"`
	SourceVersionID string                `json:"sourceVersionId,omitempty"`
	Action          string                `json:"action,omitempty"`
	CreatedAt       time.Time             `json:"createdAt"`
}

type WorkflowDraftTemplate struct {
	ID          string                      `json:"id"`
	VersionID   string                      `json:"versionId"`
	Version     int                         `json:"version"`
	Name        string                      `json:"name"`
	Description string                      `json:"description,omitempty"`
	Category    string                      `json:"category,omitempty"`
	WorkspaceID string                      `json:"workspaceId,omitempty"`
	FolderID    string                      `json:"folderId,omitempty"`
	Visibility  WorkspaceVisibility         `json:"visibility,omitempty"`
	Tags        []string                    `json:"tags,omitempty"`
	Source      WorkflowDraftTemplateSource `json:"source"`
	Status      WorkflowDraftTemplateStatus `json:"status"`
	Definition  json.RawMessage             `json:"definition"`
	CreatedBy   string                      `json:"createdBy,omitempty"`
	CreatedAt   time.Time                   `json:"createdAt"`
	UpdatedAt   time.Time                   `json:"updatedAt"`
}

type WorkflowDraftTemplateQuery struct {
	TemplateID  string
	Source      WorkflowDraftTemplateSource
	Status      WorkflowDraftTemplateStatus
	WorkspaceID string
	FolderID    string
	Visibility  WorkspaceVisibility
	LatestOnly  bool
	Limit       int
}

type SaveWorkflowDraftTemplateCommand struct {
	TemplateID    string         `json:"templateId,omitempty"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Category      string         `json:"category,omitempty"`
	Operator      string         `json:"operator,omitempty"`
	IncludeLayout bool           `json:"includeLayout,omitempty"`
	WorkspaceID   string         `json:"workspaceId,omitempty"`
	FolderID      string         `json:"folderId,omitempty"`
	Visibility    string         `json:"visibility,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Parameters    map[string]any `json:"parameters,omitempty"`
}

type ApplyWorkflowDraftTemplateCommand struct {
	TemplateID  string         `json:"templateId,omitempty"`
	VersionID   string         `json:"versionId,omitempty"`
	Version     int            `json:"version,omitempty"`
	Operator    string         `json:"operator,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	WorkspaceID string         `json:"workspaceId,omitempty"`
	FolderID    string         `json:"folderId,omitempty"`
}

type UpdateWorkflowDraftTemplateCommand struct {
	VersionID   string   `json:"versionId,omitempty"`
	Version     int      `json:"version,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Category    *string  `json:"category,omitempty"`
	WorkspaceID *string  `json:"workspaceId,omitempty"`
	FolderID    *string  `json:"folderId,omitempty"`
	Visibility  *string  `json:"visibility,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Status      string   `json:"status,omitempty"`
	Operator    string   `json:"operator,omitempty"`
}

type CopyWorkflowDraftTemplateCommand struct {
	VersionID   string   `json:"versionId,omitempty"`
	Version     int      `json:"version,omitempty"`
	Name        string   `json:"name,omitempty"`
	Category    string   `json:"category,omitempty"`
	WorkspaceID string   `json:"workspaceId,omitempty"`
	FolderID    string   `json:"folderId,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Operator    string   `json:"operator,omitempty"`
}

type WorkflowDraftTemplateCompareResult struct {
	LeftVersionID  string                     `json:"leftVersionId"`
	RightVersionID string                     `json:"rightVersionId"`
	LeftVersion    int                        `json:"leftVersion"`
	RightVersion   int                        `json:"rightVersion"`
	NodeDelta      int                        `json:"nodeDelta"`
	EdgeDelta      int                        `json:"edgeDelta"`
	AddedNodes     []string                   `json:"addedNodes"`
	RemovedNodes   []string                   `json:"removedNodes"`
	CommonNodes    int                        `json:"commonNodes"`
	NodeChanges    []WorkflowTemplateNodeDiff `json:"nodeChanges,omitempty"`
	EdgeChanges    []WorkflowTemplateEdgeDiff `json:"edgeChanges,omitempty"`
	ChangedMeta    map[string]any             `json:"changedMeta,omitempty"`
}

type WorkflowTemplateNodeDiff struct {
	NodeID  string                      `json:"nodeId"`
	Label   string                      `json:"label,omitempty"`
	Type    string                      `json:"type,omitempty"`
	Kind    string                      `json:"kind,omitempty"`
	Changes []WorkflowExecutionJSONDiff `json:"changes"`
}

type WorkflowTemplateEdgeDiff struct {
	EdgeID  string                      `json:"edgeId"`
	Source  string                      `json:"source,omitempty"`
	Target  string                      `json:"target,omitempty"`
	Label   string                      `json:"label,omitempty"`
	Changes []WorkflowExecutionJSONDiff `json:"changes"`
}

type WorkflowNodePreset struct {
	ID          string          `json:"id"`
	NodeType    string          `json:"nodeType"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}

type WorkflowExecution struct {
	ID                string                  `json:"id"`
	TenantID          string                  `json:"tenantId"`
	ActivityID        string                  `json:"activityId"`
	WorkflowVersionID string                  `json:"workflowVersionId"`
	WorkflowVersion   int                     `json:"workflowVersion"`
	EventInboxID      string                  `json:"eventInboxId,omitempty"`
	EventID           string                  `json:"eventId,omitempty"`
	Status            WorkflowExecutionStatus `json:"status"`
	Input             json.RawMessage         `json:"input,omitempty"`
	Output            json.RawMessage         `json:"output,omitempty"`
	ErrorMessage      string                  `json:"errorMessage,omitempty"`
	StartedAt         time.Time               `json:"startedAt"`
	FinishedAt        *time.Time              `json:"finishedAt,omitempty"`
	CreatedAt         time.Time               `json:"createdAt"`
	UpdatedAt         time.Time               `json:"updatedAt"`
}

type WorkflowNodeExecution struct {
	ID           string                      `json:"id"`
	TenantID     string                      `json:"tenantId"`
	ExecutionID  string                      `json:"executionId"`
	NodeID       string                      `json:"nodeId"`
	NodeType     string                      `json:"nodeType"`
	NodeTitle    string                      `json:"nodeTitle,omitempty"`
	Status       WorkflowNodeExecutionStatus `json:"status"`
	Input        json.RawMessage             `json:"input,omitempty"`
	Output       json.RawMessage             `json:"output,omitempty"`
	ErrorMessage string                      `json:"errorMessage,omitempty"`
	StartedAt    time.Time                   `json:"startedAt"`
	FinishedAt   *time.Time                  `json:"finishedAt,omitempty"`
	CreatedAt    time.Time                   `json:"createdAt"`
	UpdatedAt    time.Time                   `json:"updatedAt"`
}

type WorkflowNodeExecutionQuery struct {
	ExecutionID string `json:"executionId,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

type SaveWorkflowDraftCommand struct {
	Operator   string          `json:"operator,omitempty"`
	Definition json.RawMessage `json:"definition"`
}

type PublishWorkflowCommand struct {
	Operator    string `json:"operator,omitempty"`
	ReleaseNote string `json:"releaseNote,omitempty"`
}

type WorkflowVersionActionCommand struct {
	Operator string `json:"operator,omitempty"`
	Note     string `json:"note,omitempty"`
}

type CancelWorkflowExecutionCommand struct {
	Operator string `json:"operator,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type ResumeWorkflowExecutionCommand struct {
	Operator    string `json:"operator,omitempty"`
	Reason      string `json:"reason,omitempty"`
	StartNodeID string `json:"startNodeId,omitempty"`
}

type WorkflowDebugRunCommand struct {
	Operator          string          `json:"operator,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Template          string          `json:"template,omitempty"`
	StartNodeID       string          `json:"startNodeId,omitempty"`
	SourceExecutionID string          `json:"sourceExecutionId,omitempty"`
}

type WorkflowExecutionTrace struct {
	SchemaVersion  string                  `json:"schemaVersion,omitempty"`
	ExportedAt     time.Time               `json:"exportedAt,omitempty"`
	Execution      WorkflowExecution       `json:"execution"`
	NodeExecutions []WorkflowNodeExecution `json:"nodeExecutions"`
	NodeSummaries  []WorkflowNodeSummary   `json:"nodeSummaries,omitempty"`
	RouteTrace     any                     `json:"routeTrace,omitempty"`
}

type WorkflowNodeSummary struct {
	NodeID      string                      `json:"nodeId"`
	NodeType    string                      `json:"nodeType"`
	NodeTitle   string                      `json:"nodeTitle,omitempty"`
	Status      WorkflowNodeExecutionStatus `json:"status"`
	DurationMs  int64                       `json:"durationMs,omitempty"`
	InputKeys   []string                    `json:"inputKeys,omitempty"`
	OutputKeys  []string                    `json:"outputKeys,omitempty"`
	Decision    string                      `json:"decision,omitempty"`
	ErrorCode   string                      `json:"errorCode,omitempty"`
	Explanation string                      `json:"explanation,omitempty"`
}

type WorkflowExecutionCompareSummary struct {
	ID              string                  `json:"id"`
	Status          WorkflowExecutionStatus `json:"status"`
	EventID         string                  `json:"eventId,omitempty"`
	WorkflowVersion int                     `json:"workflowVersion"`
	ErrorMessage    string                  `json:"errorMessage,omitempty"`
	StartedAt       time.Time               `json:"startedAt"`
	FinishedAt      *time.Time              `json:"finishedAt,omitempty"`
}

type WorkflowExecutionCompare struct {
	Current            WorkflowExecutionCompareSummary    `json:"current"`
	Baseline           WorkflowExecutionCompareSummary    `json:"baseline"`
	CurrentPath        []string                           `json:"currentPath"`
	BaselinePath       []string                           `json:"baselinePath"`
	PathChanged        bool                               `json:"pathChanged"`
	NodeStatusChanges  []WorkflowExecutionNodeStatusDiff  `json:"nodeStatusChanges,omitempty"`
	SideEffectChanges  []WorkflowExecutionSideEffectDiff  `json:"sideEffectChanges,omitempty"`
	CurrentFailures    []string                           `json:"currentFailures"`
	BaselineFailures   []string                           `json:"baselineFailures"`
	CurrentDurationMs  int64                              `json:"currentDurationMs"`
	BaselineDurationMs int64                              `json:"baselineDurationMs"`
	DurationDeltaMs    int64                              `json:"durationDeltaMs"`
	PayloadChanges     []WorkflowExecutionPayloadNodeDiff `json:"payloadChanges"`
	PayloadChangeLimit int                                `json:"payloadChangeLimit,omitempty"`
	JSONDiffLimit      int                                `json:"jsonDiffLimit,omitempty"`
	Truncated          bool                               `json:"truncated,omitempty"`
}

type WorkflowExecutionPayloadNodeDiff struct {
	NodeID string                      `json:"nodeId"`
	Label  string                      `json:"label"`
	Input  []WorkflowExecutionJSONDiff `json:"input"`
	Output []WorkflowExecutionJSONDiff `json:"output"`
}

type WorkflowExecutionNodeStatusDiff struct {
	NodeID             string                      `json:"nodeId"`
	Label              string                      `json:"label"`
	CurrentStatus      WorkflowNodeExecutionStatus `json:"currentStatus,omitempty"`
	BaselineStatus     WorkflowNodeExecutionStatus `json:"baselineStatus,omitempty"`
	CurrentError       string                      `json:"currentError,omitempty"`
	BaselineError      string                      `json:"baselineError,omitempty"`
	CurrentErrorCode   string                      `json:"currentErrorCode,omitempty"`
	BaselineErrorCode  string                      `json:"baselineErrorCode,omitempty"`
	CurrentAttempt     int                         `json:"currentAttempt,omitempty"`
	BaselineAttempt    int                         `json:"baselineAttempt,omitempty"`
	CurrentDurationMs  int64                       `json:"currentDurationMs,omitempty"`
	BaselineDurationMs int64                       `json:"baselineDurationMs,omitempty"`
}

type WorkflowExecutionSideEffectDiff struct {
	Kind     string                      `json:"kind"`
	Changes  []WorkflowExecutionJSONDiff `json:"changes"`
	Current  any                         `json:"current,omitempty"`
	Baseline any                         `json:"baseline,omitempty"`
}

type WorkflowExecutionCompareOptions struct {
	MaxPayloadChangeNodes int
	MaxJSONDiffRows       int
}

type WorkflowExecutionJSONDiff struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Current  any    `json:"current,omitempty"`
	Baseline any    `json:"baseline,omitempty"`
}

type ValidateWorkflowCommand struct {
	Definition json.RawMessage `json:"definition,omitempty"`
}

type WorkflowExecutionQuery struct {
	TenantID          string `json:"tenantId,omitempty"`
	ActivityID        string `json:"activityId,omitempty"`
	WorkflowVersionID string `json:"workflowVersionId,omitempty"`
	EventID           string `json:"eventId,omitempty"`
	ExecutionID       string `json:"executionId,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	Offset            int    `json:"offset,omitempty"`
}

var ErrInvalidWorkflow = errors.New("invalid workflow")

func ParseWorkflowDefinition(raw json.RawMessage) (WorkflowDefinition, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return WorkflowDefinition{}, errors.Join(ErrInvalidWorkflow, errors.New("definition is required"))
	}
	var definition WorkflowDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return WorkflowDefinition{}, errors.Join(ErrInvalidWorkflow, err)
	}
	definition.Raw = json.RawMessage(append([]byte(nil), raw...))
	if err := definition.Validate(); err != nil {
		return WorkflowDefinition{}, err
	}
	return definition, nil
}

func (definition *WorkflowDefinition) Validate() error {
	if len(definition.Nodes) == 0 {
		return errors.Join(ErrInvalidWorkflow, errors.New("nodes is required"))
	}
	seen := map[string]WorkflowNode{}
	entryCount := 0
	for i, node := range definition.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		node.Type = strings.TrimSpace(node.Type)
		node.Kind = strings.TrimSpace(node.Kind)
		node.Code = strings.TrimSpace(node.Code)
		node.Title = strings.TrimSpace(node.Title)
		if node.ID == "" {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("nodes[%d].id is required", i))
		}
		if node.Type == "" {
			node.Type = node.Code
		}
		if node.Type == "" {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("nodes[%d].type is required", i))
		}
		if _, ok := seen[node.ID]; ok {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("nodes[%d].id is duplicated", i))
		}
		if node.Type == "event-in" || node.Kind == "entry" {
			entryCount++
		}
		definition.Nodes[i] = node
		seen[node.ID] = node
	}
	if entryCount == 0 {
		return errors.Join(ErrInvalidWorkflow, errors.New("event-in entry node is required"))
	}
	for i, edge := range definition.Edges {
		edge.ID = strings.TrimSpace(edge.ID)
		edge.Source = strings.TrimSpace(edge.Source)
		edge.Target = strings.TrimSpace(edge.Target)
		if edge.Source == "" || edge.Target == "" {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("edges[%d].source and target are required", i))
		}
		if _, ok := seen[edge.Source]; !ok {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("edges[%d].source node is missing", i))
		}
		if _, ok := seen[edge.Target]; !ok {
			return errors.Join(ErrInvalidWorkflow, fmt.Errorf("edges[%d].target node is missing", i))
		}
		definition.Edges[i] = edge
	}
	definition.Index = seen
	return nil
}

func NormalizeWorkflowDefinition(raw json.RawMessage) (json.RawMessage, error) {
	definition, err := ParseWorkflowDefinition(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func NormalizeWorkflowDraftDefinition(raw json.RawMessage) (json.RawMessage, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, errors.Join(ErrInvalidWorkflow, errors.New("definition is required"))
	}
	var definition WorkflowDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return nil, errors.Join(ErrInvalidWorkflow, err)
	}
	normalized, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
