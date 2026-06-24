package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
)

type workflowExecutionIDContextKey struct{}

type serviceCallAuditInput struct {
	Runtime         workflowRuntime
	Node            marketing.WorkflowNode
	ServiceRef      string
	InterfaceRef    string
	ProviderRef     string
	ResourceRef     string
	StepID          string
	Attempt         int
	Status          marketing.ServiceCallAuditStatus
	ErrorCode       string
	ErrorMessage    string
	StartedAt       time.Time
	Request         any
	Response        any
	ProviderTrace   any
	SchemaTrace     any
	PermissionTrace any
}

func (s *Service) ListServiceCallAuditLogs(ctx context.Context, query marketing.ServiceCallAuditQuery) ([]marketing.ServiceCallAuditLog, error) {
	return s.repository.ListServiceCallAuditLogs(ctx, query)
}

func (s *Service) PurgeServiceCallAuditLogs(ctx context.Context, tenantID string, workspaceID string, before time.Time, limit int) (int, error) {
	deleted, err := s.repository.PurgeServiceCallAuditLogs(ctx, tenantID, workspaceID, before, limit)
	result := "success"
	if err != nil {
		result = "failed"
	}
	s.metrics.IncCounter("exmarketing_service_call_audit_purge_total",
		metrics.L("tenant_id", tenantID),
		metrics.L("workspace_id", workspaceID),
		metrics.L("result", result),
	)
	return deleted, err
}

func (s *Service) recordServiceCallAudit(ctx context.Context, input serviceCallAuditInput) {
	if strings.TrimSpace(input.ServiceRef) == "" || strings.TrimSpace(input.InterfaceRef) == "" {
		return
	}
	status := input.Status
	if status == "" {
		status = marketing.ServiceCallAuditSuccess
	}
	now := s.now()
	latency := int64(0)
	if !input.StartedAt.IsZero() {
		latency = now.Sub(input.StartedAt).Milliseconds()
		if latency < 0 {
			latency = 0
		}
	}
	actor := ""
	if source := marketing.AuditSourceFromContext(ctx); len(source) > 0 {
		actor = firstWorkflowString(workflowConfigString(source, "actor"), firstWorkflowString(workflowConfigString(source, "actorId"), workflowConfigString(source, "operator")))
	}
	workflowID, _ := ctx.Value(workflowExecutionIDContextKey{}).(string)
	log := marketing.ServiceCallAuditLog{
		ID:              marketing.NewID("svccallaudit"),
		TenantID:        input.Runtime.Activity.TenantID,
		WorkspaceID:     input.Runtime.Activity.WorkspaceID,
		FolderID:        input.Runtime.Activity.FolderID,
		ActivityID:      input.Runtime.Activity.ID,
		WorkflowID:      workflowID,
		NodeID:          input.Node.ID,
		NodeType:        nodeHandlerKey(input.Node),
		ServiceRef:      strings.TrimSpace(input.ServiceRef),
		InterfaceRef:    strings.TrimSpace(input.InterfaceRef),
		ProviderRef:     strings.TrimSpace(input.ProviderRef),
		ResourceRef:     strings.TrimSpace(input.ResourceRef),
		StepID:          strings.TrimSpace(input.StepID),
		Attempt:         input.Attempt,
		Status:          status,
		ErrorCode:       strings.TrimSpace(input.ErrorCode),
		ErrorMessage:    marketing.RedactSensitiveString(input.ErrorMessage),
		LatencyMS:       latency,
		RequestHash:     marketing.SensitivePayloadHash(input.Request),
		ResponseHash:    marketing.SensitivePayloadHash(input.Response),
		ProviderTrace:   serviceCallAuditTraceJSON(input.ProviderTrace),
		SchemaTrace:     serviceCallAuditTraceJSON(input.SchemaTrace),
		PermissionTrace: serviceCallAuditTraceJSON(input.PermissionTrace),
		Actor:           actor,
		CreatedAt:       now,
	}
	if err := s.repository.CreateServiceCallAuditLog(ctx, log); err != nil {
		s.metrics.IncCounter("exmarketing_service_call_audit_write_failed_total",
			metrics.L("tenant_id", log.TenantID),
			metrics.L("service_ref", log.ServiceRef),
			metrics.L("interface_ref", log.InterfaceRef),
			metrics.L("status", string(log.Status)),
		)
	}
}

func serviceCallAuditTraceJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	redacted := marketing.RedactSensitive(value)
	payload, err := json.Marshal(redacted)
	if err != nil {
		return nil
	}
	return payload
}
