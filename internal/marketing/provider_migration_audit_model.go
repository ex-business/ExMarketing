package marketing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const MaxAuditDetailBytes = 4096

type ProviderDecision string

const (
	ProviderDecisionSelected     ProviderDecision = "selected"
	ProviderDecisionSkipped      ProviderDecision = "skipped"
	ProviderDecisionFallback     ProviderDecision = "fallback"
	ProviderDecisionRateLimited  ProviderDecision = "rate_limited"
	ProviderDecisionUnhealthy    ProviderDecision = "unhealthy"
	ProviderDecisionCircuitOpen  ProviderDecision = "circuit_open"
	ProviderDecisionSchemaFailed ProviderDecision = "schema_failed"
)

type ProviderDecisionAuditLog struct {
	ID                   string           `json:"id"`
	TenantID             string           `json:"tenantId"`
	WorkspaceID          string           `json:"workspaceId,omitempty"`
	FolderID             string           `json:"folderId,omitempty"`
	ActivityID           string           `json:"activityId,omitempty"`
	NodeID               string           `json:"nodeId,omitempty"`
	NodeType             string           `json:"nodeType,omitempty"`
	ServiceRef           string           `json:"serviceRef"`
	InterfaceRef         string           `json:"interfaceRef"`
	ProviderPolicy       string           `json:"providerPolicy,omitempty"`
	CandidateProviderRef string           `json:"candidateProviderRef,omitempty"`
	Decision             ProviderDecision `json:"decision"`
	Reason               string           `json:"reason,omitempty"`
	LatencyMS            int64            `json:"latencyMs,omitempty"`
	Weight               int              `json:"weight,omitempty"`
	Environment          string           `json:"environment,omitempty"`
	Mock                 bool             `json:"mock,omitempty"`
	TraceID              string           `json:"traceId,omitempty"`
	RunID                string           `json:"runId,omitempty"`
	Details              json.RawMessage  `json:"details,omitempty"`
	CreatedAt            time.Time        `json:"createdAt"`
}

type ProviderDecisionAuditQuery struct {
	TenantID     string
	WorkspaceID  string
	FolderID     string
	ActivityID   string
	NodeID       string
	ServiceRef   string
	InterfaceRef string
	ProviderRef  string
	Decision     ProviderDecision
	Cursor       string
	Limit        int
}

type MigrationAuditResult string

const (
	MigrationAuditSuccess MigrationAuditResult = "SUCCESS"
	MigrationAuditFailed  MigrationAuditResult = "FAILED"
	MigrationAuditDenied  MigrationAuditResult = "DENIED"
)

type MigrationAuditLog struct {
	ID               string               `json:"id"`
	TenantID         string               `json:"tenantId"`
	WorkspaceID      string               `json:"workspaceId,omitempty"`
	FolderID         string               `json:"folderId,omitempty"`
	ActivityID       string               `json:"activityId,omitempty"`
	MigrationType    string               `json:"migrationType"`
	Operation        string               `json:"operation"`
	DryRun           bool                 `json:"dryRun,omitempty"`
	Actor            string               `json:"actor,omitempty"`
	Scope            string               `json:"scope,omitempty"`
	BreakGlassTicket string               `json:"breakGlassTicket,omitempty"`
	Summary          json.RawMessage      `json:"summary,omitempty"`
	ItemsCount       int                  `json:"itemsCount,omitempty"`
	BlockersCount    int                  `json:"blockersCount,omitempty"`
	Result           MigrationAuditResult `json:"result"`
	Error            string               `json:"error,omitempty"`
	CreatedAt        time.Time            `json:"createdAt"`
}

type MigrationAuditQuery struct {
	TenantID      string
	WorkspaceID   string
	FolderID      string
	ActivityID    string
	MigrationType string
	Operation     string
	Actor         string
	Result        MigrationAuditResult
	Cursor        string
	Limit         int
}

func (a ProviderDecisionAuditLog) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(a.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(a.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if !validProviderDecision(a.Decision) {
		return errors.Join(ErrInvalidResource, errors.New("decision is invalid"))
	}
	if a.LatencyMS < 0 {
		return errors.Join(ErrInvalidResource, errors.New("latencyMs must be non-negative"))
	}
	if len(a.Details) > 0 && !json.Valid(a.Details) {
		return errors.Join(ErrInvalidResource, errors.New("details must be valid json"))
	}
	return nil
}

func (a MigrationAuditLog) Validate() error {
	if strings.TrimSpace(a.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(a.MigrationType) == "" {
		return errors.Join(ErrInvalidResource, errors.New("migrationType is required"))
	}
	if strings.TrimSpace(a.Operation) == "" {
		return errors.Join(ErrInvalidResource, errors.New("operation is required"))
	}
	if !validMigrationAuditResult(a.Result) {
		return errors.Join(ErrInvalidResource, errors.New("result is invalid"))
	}
	if a.ItemsCount < 0 || a.BlockersCount < 0 {
		return errors.Join(ErrInvalidResource, errors.New("counts must be non-negative"))
	}
	if len(a.Summary) > 0 && !json.Valid(a.Summary) {
		return errors.Join(ErrInvalidResource, errors.New("summary must be valid json"))
	}
	return nil
}

func validProviderDecision(value ProviderDecision) bool {
	switch value {
	case ProviderDecisionSelected, ProviderDecisionSkipped, ProviderDecisionFallback, ProviderDecisionRateLimited, ProviderDecisionUnhealthy, ProviderDecisionCircuitOpen, ProviderDecisionSchemaFailed:
		return true
	default:
		return false
	}
}

func validMigrationAuditResult(value MigrationAuditResult) bool {
	switch value {
	case MigrationAuditSuccess, MigrationAuditFailed, MigrationAuditDenied:
		return true
	default:
		return false
	}
}
