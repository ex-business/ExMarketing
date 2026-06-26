package engine

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
)

type providerDecisionAuditInput struct {
	Runtime              workflowRuntime
	Node                 marketing.WorkflowNode
	ServiceRef           string
	InterfaceRef         string
	ProviderPolicy       string
	CandidateProviderRef string
	Decision             marketing.ProviderDecision
	Reason               string
	StartedAt            time.Time
	Weight               int
	Environment          string
	Mock                 bool
	Details              any
}

func (s *Service) ListProviderDecisionAuditLogs(ctx context.Context, query marketing.ProviderDecisionAuditQuery) ([]marketing.ProviderDecisionAuditLog, error) {
	return s.repository.ListProviderDecisionAuditLogs(ctx, query)
}

func (s *Service) ListMigrationAuditLogs(ctx context.Context, query marketing.MigrationAuditQuery) ([]marketing.MigrationAuditLog, error) {
	return s.repository.ListMigrationAuditLogs(ctx, query)
}

func (s *Service) RecordMigrationAudit(ctx context.Context, log marketing.MigrationAuditLog) {
	log.TenantID = strings.TrimSpace(log.TenantID)
	log.MigrationType = strings.ToUpper(strings.TrimSpace(log.MigrationType))
	log.Operation = strings.ToUpper(strings.TrimSpace(log.Operation))
	log.Actor = strings.TrimSpace(log.Actor)
	log.Scope = strings.TrimSpace(log.Scope)
	log.BreakGlassTicket = marketing.RedactSensitiveString(log.BreakGlassTicket)
	log.Error = marketing.RedactSensitiveString(log.Error)
	if log.Result == "" {
		log.Result = marketing.MigrationAuditSuccess
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = s.now()
	}
	log.Summary = auditDetailJSON(log.Summary)
	if err := s.repository.CreateMigrationAuditLog(ctx, log); err != nil {
		s.metrics.IncCounter("exmarketing_migration_audit_write_failed_total",
			metrics.L("tenant_id", log.TenantID),
			metrics.L("migration_type", log.MigrationType),
			metrics.L("operation", log.Operation),
			metrics.L("result", string(log.Result)),
		)
	}
}

func (s *Service) recordProviderDecisionAudit(ctx context.Context, input providerDecisionAuditInput) {
	if strings.TrimSpace(input.ServiceRef) == "" || strings.TrimSpace(input.InterfaceRef) == "" || input.Decision == "" {
		return
	}
	now := s.now()
	latency := int64(0)
	if !input.StartedAt.IsZero() {
		latency = now.Sub(input.StartedAt).Milliseconds()
		if latency < 0 {
			latency = 0
		}
	}
	traceID := ""
	if source := marketing.AuditSourceFromContext(ctx); len(source) > 0 {
		traceID = firstWorkflowString(workflowConfigString(source, "traceId"), workflowConfigString(source, "requestId"))
	}
	runID, _ := ctx.Value(workflowExecutionIDContextKey{}).(string)
	log := marketing.ProviderDecisionAuditLog{
		ID:                   marketing.NewID("provdecision"),
		TenantID:             input.Runtime.Activity.TenantID,
		WorkspaceID:          input.Runtime.Activity.WorkspaceID,
		FolderID:             input.Runtime.Activity.FolderID,
		ActivityID:           input.Runtime.Activity.ID,
		NodeID:               input.Node.ID,
		NodeType:             nodeHandlerKey(input.Node),
		ServiceRef:           strings.TrimSpace(input.ServiceRef),
		InterfaceRef:         strings.TrimSpace(input.InterfaceRef),
		ProviderPolicy:       strings.ToUpper(strings.TrimSpace(input.ProviderPolicy)),
		CandidateProviderRef: strings.TrimSpace(input.CandidateProviderRef),
		Decision:             input.Decision,
		Reason:               marketing.RedactSensitiveString(input.Reason),
		LatencyMS:            latency,
		Weight:               input.Weight,
		Environment:          strings.TrimSpace(input.Environment),
		Mock:                 input.Mock,
		TraceID:              traceID,
		RunID:                runID,
		Details:              providerDecisionDetailJSON(input.Details),
		CreatedAt:            now,
	}
	if err := s.repository.CreateProviderDecisionAuditLog(ctx, log); err != nil {
		s.metrics.IncCounter("exmarketing_provider_decision_audit_write_failed_total",
			metrics.L("tenant_id", log.TenantID),
			metrics.L("service_ref", log.ServiceRef),
			metrics.L("interface_ref", log.InterfaceRef),
			metrics.L("decision", string(log.Decision)),
		)
	}
}

func providerDecisionDetailJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	redacted := marketing.RedactSensitive(value)
	payload, err := json.Marshal(redacted)
	if err != nil {
		payload, _ = json.Marshal(map[string]any{"error": marketing.RedactSensitiveString(err.Error())})
	}
	summary := providerDecisionValueSummary(redacted)
	summary["sizeBytes"] = len(payload)
	summary["hash"] = marketing.SensitivePayloadHash(redacted)
	out, err := json.Marshal(summary)
	if err != nil {
		return json.RawMessage(`{"type":"unknown"}`)
	}
	return out
}

func providerDecisionValueSummary(value any) map[string]any {
	summary := map[string]any{"type": auditValueType(value)}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		status := map[string]any{}
		for key, child := range typed {
			keys = append(keys, key)
			if auditStatusField(key) {
				status[key] = auditScalarSummary(child)
			}
		}
		sort.Strings(keys)
		summary["topLevelKeyCount"] = len(keys)
		summary["topLevelKeys"] = auditFirstStrings(keys, 20)
		if len(status) > 0 {
			summary["statusFields"] = status
		}
		for _, key := range []string{"providerOutput", "resourceOutput", "runtimeGate", "weightedSelection", "cacheAside", "cacheAsideWriteBack"} {
			if child, ok := typed[key]; ok {
				summary[key] = providerDecisionNestedSummary(child)
			}
		}
	case []any:
		summary["length"] = len(typed)
	}
	return summary
}

func providerDecisionNestedSummary(value any) map[string]any {
	nested := providerDecisionValueSummary(value)
	delete(nested, "hash")
	delete(nested, "sizeBytes")
	return nested
}

func auditValueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		return "number"
	default:
		return "unknown"
	}
}

func auditStatusField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "status", "result", "errorcode", "code", "decision":
		return true
	default:
		return false
	}
}

func auditScalarSummary(value any) any {
	switch typed := value.(type) {
	case string:
		return marketing.RedactSensitiveString(typed)
	case bool:
		return typed
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		return typed
	default:
		return auditValueType(value)
	}
}

func auditFirstStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, "...")
	return out
}

func auditDetailJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	redacted := marketing.RedactSensitive(value)
	payload, err := json.Marshal(redacted)
	if err != nil {
		payload, _ = json.Marshal(map[string]any{"error": marketing.RedactSensitiveString(err.Error())})
	}
	if len(payload) <= marketing.MaxAuditDetailBytes {
		return payload
	}
	summary, _ := json.Marshal(map[string]any{
		"truncated": true,
		"sizeBytes": len(payload),
		"hash":      marketing.SensitivePayloadHash(redacted),
	})
	return summary
}
