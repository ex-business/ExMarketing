package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const workflowServiceInterfaceMigrationConfirmation = "APPLY_WORKFLOW_SERVICE_INTERFACE_MIGRATION"

type workflowServiceInterfaceMigrationRequest struct {
	DryRun       bool     `json:"dryRun"`
	Confirm      bool     `json:"confirm"`
	Confirmation string   `json:"confirmation"`
	Paths        []string `json:"paths"`
	Reason       string   `json:"reason"`
}

type workflowServiceInterfaceMigrationAnalysis struct {
	Status   string                                     `json:"status"`
	Summary  workflowServiceInterfaceMigrationSummary   `json:"summary"`
	Items    []workflowServiceInterfaceMigrationItem    `json:"items"`
	Blockers []workflowServiceInterfaceMigrationBlocker `json:"blockers"`
}

type workflowServiceInterfaceMigrationSummary struct {
	LegacySteps       int `json:"legacySteps"`
	AutoMigratable    int `json:"autoMigratable"`
	ManualReview      int `json:"manualReview"`
	Blockers          int `json:"blockers"`
	ServiceInterfaces int `json:"serviceInterfaces"`
	Resources         int `json:"resources"`
}

type workflowServiceInterfaceMigrationItem struct {
	NodeID            string         `json:"nodeId"`
	NodeType          string         `json:"nodeType"`
	Path              string         `json:"path"`
	LegacyType        string         `json:"legacyType"`
	LegacyFields      []string       `json:"legacyFields,omitempty"`
	RecommendedTarget string         `json:"recommendedTarget"`
	ServiceRef        string         `json:"serviceRef,omitempty"`
	InterfaceRef      string         `json:"interfaceRef,omitempty"`
	ProviderPolicy    string         `json:"providerPolicy,omitempty"`
	ProviderRef       string         `json:"providerRef,omitempty"`
	ResourceRef       string         `json:"resourceRef,omitempty"`
	Operation         string         `json:"operation,omitempty"`
	Confidence        string         `json:"confidence"`
	Risk              string         `json:"risk"`
	BlockingIssues    []string       `json:"blockingIssues,omitempty"`
	Before            map[string]any `json:"before,omitempty"`
	After             map[string]any `json:"after,omitempty"`
}

type workflowServiceInterfaceMigrationBlocker struct {
	NodeID string `json:"nodeId"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func (h *Handler) analyzeWorkflowServiceInterfaceMigration(w http.ResponseWriter, r *http.Request) {
	activityID := workflowMigrationActivityID(r)
	if !h.requireActivityWorkspacePermission(w, r, activityID, workspacePermissionAdmin) {
		h.recordDeniedMigrationAudit(r, "WORKFLOW_SERVICE_INTERFACE", "ANALYZE", activityID, "workflow-service-interface-migration:"+activityID, "workspace admin permission is required")
		return
	}
	audit := marketing.MigrationAuditLog{
		TenantID:      tenantFromRequest(r),
		ActivityID:    activityID,
		MigrationType: "WORKFLOW_SERVICE_INTERFACE",
		Operation:     "ANALYZE",
		Actor:         actorFromRequest(r),
		Result:        marketing.MigrationAuditFailed,
	}
	defer func() { h.engine.RecordMigrationAudit(r.Context(), audit) }()
	analysis, _, _, err := h.workflowServiceInterfaceMigrationAnalysis(r, activityID)
	if err != nil {
		audit.Error = err.Error()
		workflowMigrationError(w, err)
		return
	}
	if draft, err := h.engine.GetWorkflowDraft(r.Context(), activityID); err == nil {
		audit.TenantID = draft.TenantID
	}
	audit.Result = marketing.MigrationAuditSuccess
	audit.Summary = migrationAuditSummaryJSON(workflowMigrationAuditSummary(analysis))
	audit.ItemsCount = len(analysis.Items)
	audit.BlockersCount = len(analysis.Blockers)
	writeJSON(w, http.StatusOK, map[string]any{
		"activityId": activityID,
		"data":       analysis,
	})
}

func (h *Handler) applyWorkflowServiceInterfaceMigration(w http.ResponseWriter, r *http.Request) {
	activityID := workflowMigrationActivityID(r)
	if !h.requireActivityWorkspacePermission(w, r, activityID, workspacePermissionAdmin) {
		h.recordDeniedMigrationAudit(r, "WORKFLOW_SERVICE_INTERFACE", "APPLY", activityID, "workflow-service-interface-migration:"+activityID, "workspace admin permission is required")
		return
	}
	audit := marketing.MigrationAuditLog{
		TenantID:      tenantFromRequest(r),
		ActivityID:    activityID,
		MigrationType: "WORKFLOW_SERVICE_INTERFACE",
		Operation:     "APPLY",
		Actor:         actorFromRequest(r),
		Result:        marketing.MigrationAuditFailed,
	}
	defer func() { h.engine.RecordMigrationAudit(r.Context(), audit) }()
	var cmd workflowServiceInterfaceMigrationRequest
	if r.Body != nil {
		if err := decodeStrictJSON(r.Body, &cmd); err != nil {
			audit.Error = "invalid json body"
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	audit.DryRun = cmd.DryRun
	if cmd.DryRun {
		audit.Operation = "DRY_RUN"
	}
	analysis, rawDefinition, draft, err := h.workflowServiceInterfaceMigrationAnalysis(r, activityID)
	if err != nil {
		audit.Error = err.Error()
		workflowMigrationError(w, err)
		return
	}
	audit.TenantID = draft.TenantID
	if activity, err := h.engine.GetActivity(r.Context(), activityID); err == nil {
		audit.WorkspaceID = activity.WorkspaceID
		audit.FolderID = activity.FolderID
	}
	audit.Summary = migrationAuditSummaryJSON(workflowMigrationAuditSummary(analysis))
	audit.ItemsCount = len(analysis.Items)
	audit.BlockersCount = len(analysis.Blockers)
	selected := workflowMigrationSelectedPaths(cmd.Paths)
	migratedDefinition, applied, err := applyWorkflowServiceInterfaceMigration(rawDefinition, analysis.Items, selected)
	if err != nil {
		audit.Error = err.Error()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cmd.DryRun {
		audit.Result = marketing.MigrationAuditSuccess
		audit.ItemsCount = len(applied)
		writeJSON(w, http.StatusOK, map[string]any{
			"activityId":   activityID,
			"dryRun":       true,
			"applied":      false,
			"appliedItems": applied,
			"data":         analysis,
			"definition":   migratedDefinition,
		})
		return
	}
	if !cmd.Confirm || strings.TrimSpace(cmd.Confirmation) != workflowServiceInterfaceMigrationConfirmation {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "confirm=true and confirmation=APPLY_WORKFLOW_SERVICE_INTERFACE_MIGRATION are required"
		writeError(w, http.StatusBadRequest, "confirm=true and confirmation=APPLY_WORKFLOW_SERVICE_INTERFACE_MIGRATION are required")
		return
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "reason is required"
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if len(applied) == 0 {
		audit.Result = marketing.MigrationAuditSuccess
		audit.ItemsCount = 0
		writeJSON(w, http.StatusOK, map[string]any{
			"activityId":   activityID,
			"dryRun":       false,
			"applied":      false,
			"appliedItems": []workflowServiceInterfaceMigrationItem{},
			"data":         analysis,
		})
		return
	}
	saved, err := h.engine.SaveWorkflowDraft(r.Context(), activityID, marketing.SaveWorkflowDraftCommand{
		Operator:   actorFromRequest(r),
		Definition: migratedDefinition,
	})
	if err != nil {
		audit.Error = err.Error()
		workflowMigrationError(w, err)
		return
	}
	_ = h.engine.RecordAudit(r.Context(), draft.TenantID, actorFromRequest(r), "WORKFLOW_SERVICE_INTERFACE_MIGRATION_APPLY", "WORKFLOW_DRAFT", activityID, map[string]any{
		"reason":       strings.TrimSpace(cmd.Reason),
		"appliedPaths": workflowMigrationItemPaths(applied),
		"blockers":     analysis.Blockers,
	})
	afterAnalysis, _, _, err := h.workflowServiceInterfaceMigrationAnalysis(r, activityID)
	if err != nil {
		audit.Error = err.Error()
		workflowMigrationError(w, err)
		return
	}
	audit.Result = marketing.MigrationAuditSuccess
	audit.ItemsCount = len(applied)
	audit.BlockersCount = len(afterAnalysis.Blockers)
	audit.Summary = migrationAuditSummaryJSON(map[string]any{
		"reason":       strings.TrimSpace(cmd.Reason),
		"appliedPaths": workflowMigrationItemPaths(applied),
		"before":       workflowMigrationAuditSummary(analysis),
		"after":        workflowMigrationAuditSummary(afterAnalysis),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"activityId":   activityID,
		"dryRun":       false,
		"applied":      true,
		"appliedItems": applied,
		"draft":        saved,
		"before":       analysis,
		"after":        afterAnalysis,
	})
}

func (h *Handler) workflowServiceInterfaceMigrationAnalysis(r *http.Request, activityID string) (workflowServiceInterfaceMigrationAnalysis, map[string]any, marketing.WorkflowDraft, error) {
	draft, err := h.engine.GetWorkflowDraft(r.Context(), activityID)
	if err != nil {
		return workflowServiceInterfaceMigrationAnalysis{}, nil, marketing.WorkflowDraft{}, err
	}
	var raw map[string]any
	if err := json.Unmarshal(draft.Definition, &raw); err != nil {
		return workflowServiceInterfaceMigrationAnalysis{}, nil, marketing.WorkflowDraft{}, errors.Join(marketing.ErrInvalidWorkflow, err)
	}
	analysis := analyzeWorkflowServiceInterfaceMigrationDefinition(raw)
	return analysis, raw, draft, nil
}

func analyzeWorkflowServiceInterfaceMigrationDefinition(definition map[string]any) workflowServiceInterfaceMigrationAnalysis {
	analysis := workflowServiceInterfaceMigrationAnalysis{Status: "NOOP"}
	nodes, _ := definition["nodes"].([]any)
	for nodeIndex, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		nodeID := workflowMigrationString(node, "id")
		nodeType := workflowMigrationString(node, "type")
		config, _ := node["config"].(map[string]any)
		if len(config) == 0 {
			continue
		}
		if chain, ok := config["providerChain"].([]any); ok {
			for stepIndex, rawStep := range chain {
				step, ok := rawStep.(map[string]any)
				if !ok {
					continue
				}
				path := fmt.Sprintf("nodes[%d].config.providerChain[%d]", nodeIndex, stepIndex)
				if item, ok := analyzeWorkflowMigrationConfig(nodeID, nodeType, path, step, false); ok {
					addWorkflowMigrationAnalysisItem(&analysis, item)
				}
			}
			continue
		}
		path := fmt.Sprintf("nodes[%d].config", nodeIndex)
		if item, ok := analyzeWorkflowMigrationConfig(nodeID, nodeType, path, config, true); ok {
			addWorkflowMigrationAnalysisItem(&analysis, item)
		}
	}
	if analysis.Summary.LegacySteps > 0 {
		analysis.Status = "READY"
	}
	if analysis.Summary.ManualReview > 0 {
		analysis.Status = "NEEDS_REVIEW"
	}
	if analysis.Summary.AutoMigratable == 0 && analysis.Summary.Blockers > 0 {
		analysis.Status = "BLOCKED"
	}
	return analysis
}

func analyzeWorkflowMigrationConfig(nodeID string, nodeType string, path string, config map[string]any, rootConfig bool) (workflowServiceInterfaceMigrationItem, bool) {
	legacyType := workflowMigrationLegacyType(config)
	legacyFields := workflowMigrationLegacyFields(config)
	if legacyType == "" && len(legacyFields) == 0 {
		return workflowServiceInterfaceMigrationItem{}, false
	}
	before := workflowMigrationCopyMap(config)
	item := workflowServiceInterfaceMigrationItem{
		NodeID:       nodeID,
		NodeType:     nodeType,
		Path:         path,
		LegacyType:   firstNonEmpty(legacyType, "LEGACY_FIELDS"),
		LegacyFields: legacyFields,
		Before:       before,
		Confidence:   "MEDIUM",
		Risk:         "MEDIUM",
	}
	after, target, blockers := workflowMigrationRecommendedConfig(config)
	item.After = after
	item.BlockingIssues = blockers
	item.RecommendedTarget = target
	item.ServiceRef = workflowMigrationString(after, "serviceRef")
	item.InterfaceRef = workflowMigrationString(after, "interfaceRef")
	item.ProviderPolicy = workflowMigrationString(after, "providerPolicy")
	item.ProviderRef = workflowMigrationString(after, "providerRef")
	item.ResourceRef = workflowMigrationString(after, "resourceRef")
	item.Operation = workflowMigrationString(after, "operation")
	if len(blockers) == 0 {
		if target == string(marketing.ProviderChainTargetServiceInterface) {
			item.Confidence = "HIGH"
			item.Risk = "LOW"
		}
		if target == string(marketing.ProviderChainTargetResource) {
			item.Confidence = "MEDIUM"
			item.Risk = "MEDIUM"
		}
		if rootConfig {
			item.After = workflowMigrationRootConfigAfter(config, after)
		}
		return item, true
	}
	item.Confidence = "LOW"
	item.Risk = "HIGH"
	return item, true
}

func addWorkflowMigrationAnalysisItem(analysis *workflowServiceInterfaceMigrationAnalysis, item workflowServiceInterfaceMigrationItem) {
	analysis.Items = append(analysis.Items, item)
	analysis.Summary.LegacySteps++
	if len(item.BlockingIssues) > 0 {
		analysis.Summary.ManualReview++
		analysis.Summary.Blockers += len(item.BlockingIssues)
		for _, issue := range item.BlockingIssues {
			analysis.Blockers = append(analysis.Blockers, workflowServiceInterfaceMigrationBlocker{NodeID: item.NodeID, Path: item.Path, Reason: issue})
		}
		return
	}
	analysis.Summary.AutoMigratable++
	switch item.RecommendedTarget {
	case string(marketing.ProviderChainTargetServiceInterface):
		analysis.Summary.ServiceInterfaces++
	case string(marketing.ProviderChainTargetResource):
		analysis.Summary.Resources++
	}
}

func workflowMigrationRecommendedConfig(config map[string]any) (map[string]any, string, []string) {
	source := workflowMigrationLegacyType(config)
	serviceRef := workflowMigrationString(config, "serviceRef")
	interfaceRef := workflowMigrationString(config, "interfaceRef")
	resourceRef := firstNonEmpty(workflowMigrationString(config, "resourceRef"), workflowMigrationString(config, "redisRef"))
	if serviceRef != "" || interfaceRef != "" {
		if serviceRef == "" || interfaceRef == "" {
			return nil, string(marketing.ProviderChainTargetServiceInterface), []string{"serviceRef and interfaceRef are both required for SERVICE_INTERFACE migration"}
		}
		after := workflowMigrationBaseAfter(config)
		after["targetType"] = string(marketing.ProviderChainTargetServiceInterface)
		after["source"] = string(marketing.ProviderChainTargetServiceInterface)
		after["serviceRef"] = serviceRef
		after["interfaceRef"] = interfaceRef
		if policy := workflowMigrationString(config, "providerPolicy"); policy != "" {
			after["providerPolicy"] = policy
		} else {
			after["providerPolicy"] = "PRIMARY"
		}
		if providerRef := firstNonEmpty(workflowMigrationString(config, "providerRef"), workflowMigrationString(config, "providerCode")); providerRef != "" {
			after["providerRef"] = providerRef
		}
		return after, string(marketing.ProviderChainTargetServiceInterface), nil
	}
	if resourceRef != "" {
		after := workflowMigrationBaseAfter(config)
		after["targetType"] = string(marketing.ProviderChainTargetResource)
		after["source"] = string(marketing.ProviderChainTargetResource)
		after["resourceRef"] = resourceRef
		if operation := workflowMigrationOperation(config, source); operation != "" {
			after["operation"] = operation
		}
		return after, string(marketing.ProviderChainTargetResource), nil
	}
	blockers := []string{"no serviceRef/interfaceRef or governed resourceRef is available"}
	for _, field := range []string{"endpoint", "providerEndpoint", "providerCode", "grpcTarget", "pluginCode"} {
		if workflowMigrationString(config, field) != "" {
			blockers = append(blockers, field+" requires manual mapping to a Service Interface provider or Resource")
		}
	}
	return nil, "", blockers
}

func workflowMigrationRootConfigAfter(config map[string]any, step map[string]any) map[string]any {
	after := workflowMigrationCopyMap(config)
	workflowMigrationDeleteLegacyFields(after)
	after["providerChain"] = []any{step}
	return after
}

func workflowMigrationBaseAfter(config map[string]any) map[string]any {
	after := workflowMigrationCopyMap(config)
	workflowMigrationDeleteLegacyFields(after)
	return after
}

func workflowMigrationDeleteLegacyFields(config map[string]any) {
	for _, field := range []string{"targetType", "source", "provider", "providerRef", "resourceRef", "endpoint", "providerEndpoint", "providerCode", "redisRef", "grpcTarget", "pluginCode"} {
		delete(config, field)
	}
}

func workflowMigrationLegacyType(config map[string]any) string {
	value := strings.ToUpper(firstNonEmpty(
		workflowMigrationString(config, "targetType"),
		workflowMigrationString(config, "source"),
		workflowMigrationString(config, "provider"),
	))
	if value == "" {
		if workflowMigrationString(config, "endpoint") != "" || workflowMigrationString(config, "providerEndpoint") != "" || workflowMigrationString(config, "providerCode") != "" {
			return "FACT_SERVICE"
		}
		if workflowMigrationString(config, "redisRef") != "" {
			return "REDIS"
		}
		if workflowMigrationString(config, "grpcTarget") != "" {
			return "GRPC"
		}
		if workflowMigrationString(config, "pluginCode") != "" {
			return "PLUGIN"
		}
		return ""
	}
	switch value {
	case "SERVICE_INTERFACE", "RESOURCE", "WORKFLOW_CONTEXT", "LOCAL_CACHE", "CACHE":
		if len(workflowMigrationLegacyFields(config)) == 0 {
			return ""
		}
		return "LEGACY_FIELDS"
	case "FACT_SERVICE", "HTTP_PROVIDER", "HTTP", "PROVIDER", "REDIS", "GRPC", "PLUGIN", "CUSTOM", "LUA_QUERY":
		return value
	default:
		return ""
	}
}

func workflowMigrationLegacyFields(config map[string]any) []string {
	fields := []string{}
	fieldNames := []string{"provider", "providerRef", "resourceRef", "endpoint", "providerEndpoint", "providerCode", "redisRef", "grpcTarget", "pluginCode"}
	switch strings.ToUpper(firstNonEmpty(workflowMigrationString(config, "targetType"), workflowMigrationString(config, "source"))) {
	case "SERVICE_INTERFACE":
		fieldNames = []string{"provider", "resourceRef", "endpoint", "providerEndpoint", "providerCode", "redisRef", "grpcTarget", "pluginCode"}
	case "RESOURCE":
		fieldNames = []string{"provider", "providerRef", "endpoint", "providerEndpoint", "providerCode", "redisRef", "grpcTarget", "pluginCode"}
	case "WORKFLOW_CONTEXT", "LOCAL_CACHE", "CACHE":
		fieldNames = []string{"provider", "providerRef", "resourceRef", "endpoint", "providerEndpoint", "providerCode", "redisRef", "grpcTarget", "pluginCode"}
	}
	for _, field := range fieldNames {
		if workflowMigrationString(config, field) != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func workflowMigrationOperation(config map[string]any, source string) string {
	if operation := workflowMigrationString(config, "operation"); operation != "" {
		return operation
	}
	switch strings.ToUpper(source) {
	case "REDIS":
		return firstNonEmpty(workflowMigrationString(config, "mode"), "READ")
	default:
		return ""
	}
}

func applyWorkflowServiceInterfaceMigration(definition map[string]any, items []workflowServiceInterfaceMigrationItem, selected map[string]struct{}) (json.RawMessage, []workflowServiceInterfaceMigrationItem, error) {
	appliedByPath := map[string]workflowServiceInterfaceMigrationItem{}
	for _, item := range items {
		if len(item.BlockingIssues) > 0 {
			continue
		}
		if len(selected) > 0 {
			if _, ok := selected[item.Path]; !ok {
				continue
			}
		}
		appliedByPath[item.Path] = item
	}
	nodes, _ := definition["nodes"].([]any)
	for nodeIndex, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		config, _ := node["config"].(map[string]any)
		if len(config) == 0 {
			continue
		}
		rootPath := fmt.Sprintf("nodes[%d].config", nodeIndex)
		if item, ok := appliedByPath[rootPath]; ok {
			node["config"] = item.After
			continue
		}
		chain, _ := config["providerChain"].([]any)
		for stepIndex := range chain {
			path := fmt.Sprintf("nodes[%d].config.providerChain[%d]", nodeIndex, stepIndex)
			if item, ok := appliedByPath[path]; ok {
				chain[stepIndex] = item.After
			}
		}
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		return nil, nil, err
	}
	applied := make([]workflowServiceInterfaceMigrationItem, 0, len(appliedByPath))
	for _, item := range items {
		if _, ok := appliedByPath[item.Path]; ok {
			applied = append(applied, item)
		}
	}
	return payload, applied, nil
}

func workflowMigrationSelectedPaths(paths []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			out[path] = struct{}{}
		}
	}
	return out
}

func workflowMigrationItemPaths(items []workflowServiceInterfaceMigrationItem) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func workflowMigrationActivityID(r *http.Request) string {
	return strings.TrimSpace(firstNonEmpty(r.PathValue("activityId"), r.PathValue("id")))
}

func workflowMigrationString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	switch value := config[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func workflowMigrationCopyMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func workflowMigrationError(w http.ResponseWriter, err error) {
	if errors.Is(err, marketing.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, marketing.ErrInvalidWorkflow) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, marketing.ErrForbidden) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
