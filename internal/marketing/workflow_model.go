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

const (
	WorkflowVersionActive   WorkflowVersionStatus = "ACTIVE"
	WorkflowVersionArchived WorkflowVersionStatus = "ARCHIVED"

	WorkflowExecutionRunning WorkflowExecutionStatus = "RUNNING"
	WorkflowExecutionSuccess WorkflowExecutionStatus = "SUCCESS"
	WorkflowExecutionFailed  WorkflowExecutionStatus = "FAILED"

	WorkflowNodeRunning WorkflowNodeExecutionStatus = "RUNNING"
	WorkflowNodeSuccess WorkflowNodeExecutionStatus = "SUCCESS"
	WorkflowNodeFailed  WorkflowNodeExecutionStatus = "FAILED"
	WorkflowNodeSkipped WorkflowNodeExecutionStatus = "SKIPPED"
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

type WorkflowPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceHandle string `json:"sourceHandle,omitempty"`
	TargetHandle string `json:"targetHandle,omitempty"`
	Label        string `json:"label,omitempty"`
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
	ID          string                `json:"id"`
	TenantID    string                `json:"tenantId"`
	ActivityID  string                `json:"activityId"`
	Version     int                   `json:"version"`
	Status      WorkflowVersionStatus `json:"status"`
	Definition  json.RawMessage       `json:"definition"`
	PublishedBy string                `json:"publishedBy,omitempty"`
	PublishedAt time.Time             `json:"publishedAt"`
	CreatedAt   time.Time             `json:"createdAt"`
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

type SaveWorkflowDraftCommand struct {
	Operator   string          `json:"operator,omitempty"`
	Definition json.RawMessage `json:"definition"`
}

type PublishWorkflowCommand struct {
	Operator string `json:"operator,omitempty"`
}

type WorkflowExecutionQuery struct {
	TenantID    string `json:"tenantId,omitempty"`
	ActivityID  string `json:"activityId,omitempty"`
	EventID     string `json:"eventId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
	Limit       int    `json:"limit,omitempty"`
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
