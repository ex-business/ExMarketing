package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
	"github.com/ex-stack/ExMarketing/internal/workflow"
)

const defaultEventMaxBodyBytes int64 = 1 << 20

type OpenAPIEventConfig struct {
	RequireAuth  bool
	Credentials  map[string]OpenAPIEventCredential
	MaxBodyBytes int64
}

type OpenAPIEventCredential struct {
	TenantIDs     map[string]struct{}
	SourceSystems map[string]struct{}
}

type Handler struct {
	engine            *engine.Service
	openAPIEventCfg   OpenAPIEventConfig
	webhookCfg        WebhookReceiverConfig
	rewardCallbackCfg RewardCallbackConfig
	inviteEmailCfg    InviteEmailConfig
	inviteEmailSender inviteEmailSender
}

type RewardCallbackConfig struct {
	Secrets      map[string]string
	MaxSkew      time.Duration
	MaxBodyBytes int64
}

func NewHandler(engine *engine.Service) *Handler {
	return NewHandlerWithOpenAPIEventConfig(engine, OpenAPIEventConfig{MaxBodyBytes: defaultEventMaxBodyBytes})
}

func NewHandlerWithOpenAPIEventConfig(engine *engine.Service, cfg OpenAPIEventConfig) *Handler {
	return NewHandlerWithConfigs(engine, cfg, WebhookReceiverConfig{})
}

func NewHandlerWithConfigs(engine *engine.Service, cfg OpenAPIEventConfig, webhookCfg WebhookReceiverConfig) *Handler {
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultEventMaxBodyBytes
	}
	if webhookCfg.MaxBodyBytes <= 0 {
		webhookCfg.MaxBodyBytes = defaultEventMaxBodyBytes
	}
	rewardCallbackCfg := RewardCallbackConfig{MaxSkew: 5 * time.Minute, MaxBodyBytes: defaultEventMaxBodyBytes}
	inviteEmailCfg := LoadInviteEmailConfigFromEnv()
	return &Handler{
		engine:            engine,
		openAPIEventCfg:   cfg,
		webhookCfg:        webhookCfg,
		rewardCallbackCfg: rewardCallbackCfg,
		inviteEmailCfg:    inviteEmailCfg,
		inviteEmailSender: newInviteEmailSender(inviteEmailCfg),
	}
}

func NewHandlerWithRewardCallbackConfig(engine *engine.Service, cfg OpenAPIEventConfig, webhookCfg WebhookReceiverConfig, rewardCallbackCfg RewardCallbackConfig) *Handler {
	handler := NewHandlerWithConfigs(engine, cfg, webhookCfg)
	if rewardCallbackCfg.MaxSkew <= 0 {
		rewardCallbackCfg.MaxSkew = 5 * time.Minute
	}
	if rewardCallbackCfg.MaxBodyBytes <= 0 {
		rewardCallbackCfg.MaxBodyBytes = defaultEventMaxBodyBytes
	}
	handler.rewardCallbackCfg = rewardCallbackCfg
	return handler
}

func LoadOpenAPIEventConfigFromEnv(logger *slog.Logger) (OpenAPIEventConfig, error) {
	cfg := OpenAPIEventConfig{
		RequireAuth:  strings.EqualFold(strings.TrimSpace(os.Getenv("EX_MARKETING_REQUIRE_OPENAPI_AUTH")), "true"),
		MaxBodyBytes: defaultEventMaxBodyBytes,
		Credentials:  map[string]OpenAPIEventCredential{},
	}
	if rawMax := strings.TrimSpace(os.Getenv("EX_MARKETING_EVENT_MAX_BODY_BYTES")); rawMax != "" {
		max, err := strconv.ParseInt(rawMax, 10, 64)
		if err != nil || max <= 0 {
			return OpenAPIEventConfig{}, fmt.Errorf("EX_MARKETING_EVENT_MAX_BODY_BYTES must be a positive integer")
		}
		cfg.MaxBodyBytes = max
	}
	credentials, err := parseOpenAPIEventTokens(os.Getenv("EX_MARKETING_OPENAPI_TOKENS"))
	if err != nil {
		return OpenAPIEventConfig{}, err
	}
	cfg.Credentials = credentials
	if cfg.RequireAuth && len(cfg.Credentials) == 0 {
		return OpenAPIEventConfig{}, fmt.Errorf("EX_MARKETING_REQUIRE_OPENAPI_AUTH=true requires at least one valid EX_MARKETING_OPENAPI_TOKENS entry")
	}
	if !cfg.RequireAuth && logger != nil {
		logger.Warn("openapi event auth disabled for development mode")
	}
	return cfg, nil
}

func parseOpenAPIEventTokens(raw string) (map[string]OpenAPIEventCredential, error) {
	credentials := map[string]OpenAPIEventCredential{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return credentials, nil
	}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid EX_MARKETING_OPENAPI_TOKENS entry")
		}
		token := strings.TrimSpace(parts[0])
		if token == "" {
			return nil, fmt.Errorf("EX_MARKETING_OPENAPI_TOKENS token cannot be empty")
		}
		tenants, err := parseNonEmptyCSVSet(parts[1], "tenantId")
		if err != nil {
			return nil, err
		}
		sources, err := parseNonEmptyCSVSet(parts[2], "sourceSystem")
		if err != nil {
			return nil, err
		}
		credentials[token] = OpenAPIEventCredential{TenantIDs: tenants, SourceSystems: sources}
	}
	return credentials, nil
}

func parseNonEmptyCSVSet(raw string, name string) (map[string]struct{}, error) {
	values := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("EX_MARKETING_OPENAPI_TOKENS %s cannot be empty", name)
		}
		values[value] = struct{}{}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("EX_MARKETING_OPENAPI_TOKENS %s cannot be empty", name)
	}
	return values, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /metrics", h.metrics)
	mux.HandleFunc("GET /invite/{token}", h.workspaceInvitePage)
	mux.HandleFunc("POST /invite/{token}", h.acceptWorkspaceInviteByToken)
	mux.HandleFunc("GET /admin/v1/activities", h.listActivities)
	mux.HandleFunc("GET /admin/v1/workspace-home/search", h.searchWorkspaceHome)
	mux.HandleFunc("GET /admin/v1/workspace-home/storage", h.getWorkspaceHomeStorage)
	mux.HandleFunc("GET /admin/v1/workspace-files", h.listWorkspaceFileObjects)
	mux.HandleFunc("POST /admin/v1/workspace-files", h.uploadWorkspaceFileObject)
	mux.HandleFunc("DELETE /admin/v1/workspace-files/{fileId}", h.deleteWorkspaceFileObject)
	mux.HandleFunc("GET /admin/v1/resources", h.listResources)
	mux.HandleFunc("POST /admin/v1/resources", h.saveResource)
	mux.HandleFunc("GET /admin/v1/resources/available", h.listAvailableResources)
	mux.HandleFunc("GET /admin/v1/resources/{code}", h.getResource)
	mux.HandleFunc("PUT /admin/v1/resources/{code}", h.saveResource)
	mux.HandleFunc("POST /admin/v1/resources/{code}/test", h.testResource)
	mux.HandleFunc("GET /admin/v1/resources/{code}/bindings", h.listResourceBindings)
	mux.HandleFunc("POST /admin/v1/resources/{code}/bindings", h.saveResourceBinding)
	mux.HandleFunc("GET /admin/v1/resources/{code}/audit-logs", h.listResourceAuditLogs)
	mux.HandleFunc("GET /admin/v1/workspaces", h.listWorkspaces)
	mux.HandleFunc("POST /admin/v1/workspaces", h.createWorkspace)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}", h.getWorkspace)
	mux.HandleFunc("PUT /admin/v1/workspaces/{workspaceId}", h.updateWorkspace)
	mux.HandleFunc("DELETE /admin/v1/workspaces/{workspaceId}", h.deleteWorkspace)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/members", h.listWorkspaceMembers)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/members", h.createWorkspaceMember)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/members/{memberId}", h.getWorkspaceMember)
	mux.HandleFunc("PUT /admin/v1/workspaces/{workspaceId}/members/{memberId}", h.updateWorkspaceMember)
	mux.HandleFunc("DELETE /admin/v1/workspaces/{workspaceId}/members/{memberId}", h.deleteWorkspaceMember)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/members/{memberId}/resend-invite", h.resendWorkspaceMemberInvite)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/members/{memberId}/accept-invite", h.acceptWorkspaceMemberInvite)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/folders", h.listWorkspaceFolders)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/folders", h.createWorkspaceFolder)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/folders/{folderId}", h.getWorkspaceFolder)
	mux.HandleFunc("PUT /admin/v1/workspaces/{workspaceId}/folders/{folderId}", h.updateWorkspaceFolder)
	mux.HandleFunc("DELETE /admin/v1/workspaces/{workspaceId}/folders/{folderId}", h.deleteWorkspaceFolder)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/folders/{folderId}/restore", h.restoreWorkspaceFolder)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/batch", h.batchWorkspaceHome)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/share-links", h.listWorkspaceShareLinks)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/share-links", h.createWorkspaceShareLink)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/share-links/{linkId}", h.getWorkspaceShareLink)
	mux.HandleFunc("PUT /admin/v1/workspaces/{workspaceId}/share-links/{linkId}", h.updateWorkspaceShareLink)
	mux.HandleFunc("DELETE /admin/v1/workspaces/{workspaceId}/share-links/{linkId}", h.deleteWorkspaceShareLink)
	mux.HandleFunc("POST /admin/v1/workspaces/{workspaceId}/share-links/{linkId}/regenerate", h.regenerateWorkspaceShareLink)
	mux.HandleFunc("GET /admin/v1/workspaces/{workspaceId}/audit-logs", h.listWorkspaceAuditLogs)
	mux.HandleFunc("GET /share/{token}", h.shareLinkPage)
	mux.HandleFunc("POST /share/{token}", h.resolveShareLink)
	mux.HandleFunc("POST /admin/v1/activities", h.createActivity)
	mux.HandleFunc("GET /admin/v1/activities/{id}", h.getActivity)
	mux.HandleFunc("PUT /admin/v1/activities/{id}", h.updateActivity)
	mux.HandleFunc("GET /admin/v1/activities/{id}/acl", h.listActivityACL)
	mux.HandleFunc("POST /admin/v1/activities/{id}/acl", h.createActivityACL)
	mux.HandleFunc("DELETE /admin/v1/activities/{id}/acl/{grantId}", h.deleteActivityACL)
	mux.HandleFunc("GET /admin/v1/activities/{id}/workflow/draft", h.getActivityWorkflowDraft)
	mux.HandleFunc("PUT /admin/v1/activities/{id}/workflow/draft", h.saveActivityWorkflowDraft)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/validate", h.validateActivityWorkflow)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/draft/validate", h.validateActivityWorkflowDraft)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/debug-run", h.runActivityWorkflowDebug)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/publish", h.publishActivityWorkflow)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/draft-template", h.saveActivityWorkflowDraftTemplate)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/draft-templates/{templateId}/apply", h.applyActivityWorkflowDraftTemplate)
	mux.HandleFunc("GET /admin/v1/activities/{id}/workflow/versions", h.listActivityWorkflowVersions)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/versions/{versionId}/rollback", h.rollbackActivityWorkflowVersion)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/versions/{versionId}/archive", h.archiveActivityWorkflowVersion)
	mux.HandleFunc("GET /admin/v1/activities/{id}/workflow/executions", h.listActivityWorkflowExecutions)
	mux.HandleFunc("GET /admin/v1/workflow/executions/{id}/nodes", h.listWorkflowNodeExecutions)
	mux.HandleFunc("GET /admin/v1/workflow/executions/{id}/nodes/{nodeExecutionId}", h.getWorkflowNodeExecution)
	mux.HandleFunc("GET /admin/v1/workflow/executions/{id}/compare", h.compareWorkflowExecutions)
	mux.HandleFunc("GET /admin/v1/workflow/executions/{id}/trace", h.exportWorkflowExecutionTrace)
	mux.HandleFunc("POST /admin/v1/activities/{id}/workflow/executions/{executionId}/rerun", h.rerunWorkflowExecution)
	mux.HandleFunc("POST /admin/v1/workflow/executions/{id}/cancel", h.cancelWorkflowExecution)
	mux.HandleFunc("POST /admin/v1/workflow/executions/{id}/resume", h.resumeWorkflowExecution)
	mux.HandleFunc("GET /admin/v1/workflow/node-config-schemas", h.listWorkflowNodeConfigSchemas)
	mux.HandleFunc("GET /admin/v1/workflow/node-presets", h.listWorkflowNodePresets)
	mux.HandleFunc("GET /admin/v1/workflow/draft-templates", h.listWorkflowDraftTemplates)
	mux.HandleFunc("GET /admin/v1/workflow/draft-templates/{templateId}", h.getWorkflowDraftTemplate)
	mux.HandleFunc("PUT /admin/v1/workflow/draft-templates/{templateId}", h.updateWorkflowDraftTemplate)
	mux.HandleFunc("POST /admin/v1/workflow/draft-templates/{templateId}/copy", h.copyWorkflowDraftTemplate)
	mux.HandleFunc("GET /admin/v1/workflow/draft-templates/{templateId}/compare", h.compareWorkflowDraftTemplateVersions)
	mux.HandleFunc("GET /admin/v1/workflow/draft-templates/{templateId}/versions", h.listWorkflowDraftTemplateVersions)
	mux.HandleFunc("POST /admin/v1/activities/{id}/approval/submit", h.submitActivityApproval)
	mux.HandleFunc("POST /admin/v1/activities/{id}/approval/approve", h.approveActivity)
	mux.HandleFunc("POST /admin/v1/activities/{id}/approval/reject", h.rejectActivityApproval)
	mux.HandleFunc("GET /admin/v1/activities/{id}/approval-records", h.listActivityApprovalRecords)
	mux.HandleFunc("POST /admin/v1/activities/prelaunch-validation", h.validatePrelaunch)
	mux.HandleFunc("POST /admin/v1/quotas", h.createQuota)
	mux.HandleFunc("POST /admin/v1/quota-reservations/timeout-release", h.releaseExpiredQuotaReservations)
	mux.HandleFunc("GET /admin/v1/audit", h.listAudit)
	mux.HandleFunc("GET /admin/v1/outbox/dlq", h.listOutboxDLQ)
	mux.HandleFunc("POST /admin/v1/outbox/{id}/replay", h.replayOutboxDLQ)
	mux.HandleFunc("POST /admin/v1/outbox/{id}/close", h.closeOutboxDLQ)
	mux.HandleFunc("GET /admin/v1/reward/reconciliation", h.listRewardReconciliation)
	mux.HandleFunc("POST /admin/v1/manual-grants/decision", h.applyManualGrantDecision)
	mux.HandleFunc("POST /admin/v1/manual-grant-exports", h.createManualGrantExport)
	mux.HandleFunc("POST /admin/v1/simulate", h.simulate)
	mux.HandleFunc("GET /admin/v1/workflow/requirements", h.listWorkflowRequirements)
	mux.HandleFunc("GET /admin/v1/workflow/config", h.getWorkflowConfig)
	mux.HandleFunc("GET /admin/v1/workflow/templates", h.listWorkflowTemplates)
	mux.HandleFunc("POST /admin/v1/workflow/templates/{id}/apply", h.applyWorkflowTemplate)
	mux.HandleFunc("POST /admin/v1/workflow/requirements", h.createWorkflowRequirement)
	mux.HandleFunc("DELETE /admin/v1/workflow/requirements", h.deleteWorkflowRequirements)
	mux.HandleFunc("POST /admin/v1/workflow/intake", h.intakeWorkflowRequirements)
	mux.HandleFunc("GET /admin/v1/workflow/requirements/{id}", h.getWorkflowRequirement)
	mux.HandleFunc("DELETE /admin/v1/workflow/requirements/{id}", h.deleteWorkflowRequirement)
	mux.HandleFunc("POST /admin/v1/workflow/requirements/{id}/run-agent", h.runWorkflowAgent)
	mux.HandleFunc("POST /admin/v1/workflow/requirements/{id}/run-pipeline", h.runWorkflowPipeline)
	mux.HandleFunc("POST /admin/v1/workflow/requirements/{id}/stop-agent", h.stopWorkflowAgent)
	mux.HandleFunc("GET /admin/v1/workflow/requirements/{id}/logs/{agent}", h.getWorkflowAgentLog)
	mux.HandleFunc("GET /admin/v1/workflow/requirements/{id}/runs/{runID}", h.getWorkflowRunLog)
	mux.HandleFunc("GET /admin/v1/git/diff", h.getGitDiff)
	mux.HandleFunc("POST /admin/v1/git/commit", h.createGitCommit)
	mux.HandleFunc("GET /v1/campaigns", h.listActivities)
	mux.HandleFunc("POST /v1/campaigns", h.createActivity)
	mux.HandleFunc("POST /openapi/v1/events", h.createEvent)
	mux.HandleFunc("GET /openapi/v1/checkin/status", h.getCheckinStatus)
	mux.HandleFunc("POST /openapi/v1/webhooks/{receiverCode}", h.createWebhookEvent)
	mux.HandleFunc("POST /openapi/v1/webhooks/", h.missingWebhookReceiver)
	mux.HandleFunc("POST /openapi/v1/facts/query", h.queryFacts)
	mux.HandleFunc("POST /internal/facts/user-profile", h.userProfileFactProvider)
	mux.HandleFunc("POST /openapi/v1/reward/callback", h.rewardCallback)
	mux.HandleFunc("GET /admin-flow", redirectToTrailingSlash)
	mux.Handle("GET /admin-flow/", http.StripPrefix("/admin-flow/", http.FileServer(http.Dir("web/admin-flow"))))
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir("web/admin"))))
	return withAuditSource(mux)
}

func redirectToTrailingSlash(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Path + "/"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.RefreshMetrics(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(h.engine.MetricsRegistry().RenderPrometheus())
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listActivities(w http.ResponseWriter, r *http.Request) {
	query := marketing.ActivityQuery{
		TenantID:    strings.TrimSpace(r.URL.Query().Get("tenantId")),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		Status:      strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		query.Limit = limit
	}
	filteredQuery := query.TenantID != "" || query.WorkspaceID != "" || query.FolderID != "" || query.Status != "" || query.Limit > 0
	var (
		activities []marketing.Activity
		err        error
	)
	if filteredQuery {
		activities, err = h.engine.ListActivitiesByQuery(r.Context(), query)
	} else {
		activities, err = h.engine.ListActivities(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if actorFromRequest(r) != "" {
		activities = h.filterVisibleActivities(r, activities)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": activities})
}

func (h *Handler) createActivity(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.CreateActivityCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if strings.TrimSpace(cmd.WorkspaceID) != "" {
		if !h.requireWorkspacePermission(w, r, cmd.WorkspaceID, workspacePermissionEdit) {
			return
		}
		if strings.TrimSpace(cmd.FolderID) != "" {
			folder, err := h.engine.GetWorkspaceFolder(r.Context(), cmd.WorkspaceID, cmd.FolderID)
			if err != nil {
				workspaceError(w, err)
				return
			}
			if actorFromRequest(r) != "" && !h.canViewFolder(r, cmd.WorkspaceID, folder) {
				workspaceError(w, marketing.ErrForbidden)
				return
			}
		}
	}

	activity, err := h.engine.CreateActivity(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidActivity) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrActivityConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, activity)
}

func (h *Handler) getActivity(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	activity, err := h.engine.GetActivity(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) updateActivity(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if err := rejectActivityIdentityFields(body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cmd marketing.UpdateActivityCommand
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	if cmd.WorkspaceID != nil && strings.TrimSpace(*cmd.WorkspaceID) != "" {
		if !h.requireWorkspacePermission(w, r, *cmd.WorkspaceID, workspacePermissionEdit) {
			return
		}
	}
	if cmd.WorkspaceID != nil && cmd.FolderID != nil && strings.TrimSpace(*cmd.FolderID) != "" {
		folder, err := h.engine.GetWorkspaceFolder(r.Context(), strings.TrimSpace(*cmd.WorkspaceID), strings.TrimSpace(*cmd.FolderID))
		if err != nil {
			workspaceError(w, err)
			return
		}
		if actorFromRequest(r) != "" && !h.canViewFolder(r, strings.TrimSpace(*cmd.WorkspaceID), folder) {
			workspaceError(w, marketing.ErrForbidden)
			return
		}
	}
	if cmd.WorkspaceID == nil && cmd.FolderID != nil && strings.TrimSpace(*cmd.FolderID) != "" {
		current, err := h.engine.GetActivity(r.Context(), r.PathValue("id"))
		if err != nil {
			workspaceError(w, err)
			return
		}
		folder, err := h.engine.GetWorkspaceFolder(r.Context(), current.WorkspaceID, strings.TrimSpace(*cmd.FolderID))
		if err != nil {
			workspaceError(w, err)
			return
		}
		if actorFromRequest(r) != "" && !h.canViewFolder(r, current.WorkspaceID, folder) {
			workspaceError(w, marketing.ErrForbidden)
			return
		}
	}

	activity, err := h.engine.UpdateActivity(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidActivity) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrActivityConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) getActivityWorkflowDraft(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	draft, err := h.engine.GetWorkflowDraft(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *Handler) saveActivityWorkflowDraft(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.SaveWorkflowDraftCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	draft, err := h.engine.SaveWorkflowDraft(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *Handler) validateActivityWorkflow(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	var cmd marketing.ValidateWorkflowCommand
	if r.Body != nil {
		if err := decodeStrictJSON(r.Body, &cmd); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	result, err := h.engine.ValidateWorkflowDefinition(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) validateActivityWorkflowDraft(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	result, err := h.engine.ValidateWorkflowDraft(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) runActivityWorkflowDebug(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.WorkflowDebugRunCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	trace, err := h.engine.RunWorkflowDebug(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, trace)
}

func (h *Handler) publishActivityWorkflow(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.PublishWorkflowCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionAdmin) {
		return
	}
	version, err := h.engine.PublishWorkflowVersion(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activity, activityErr := h.engine.GetActivity(r.Context(), version.ActivityID)
	if activityErr == nil && strings.TrimSpace(activity.WorkspaceID) != "" {
		_, _ = h.storeWorkspaceJSONFile(r, activity.WorkspaceID, "workflow-version-"+version.ID+".json", version.Definition, workspaceFileMeta{
			FolderID:          activity.FolderID,
			ActivityID:        activity.ID,
			WorkflowVersionID: version.ID,
			Kind:              marketing.WorkspaceFileWorkflowVersion,
			CreatedBy:         actorFromRequest(r),
		})
	}
	writeJSON(w, http.StatusCreated, version)
}

func (h *Handler) saveActivityWorkflowDraftTemplate(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.SaveWorkflowDraftTemplateCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	template, err := h.engine.SaveCurrentWorkflowAsTemplate(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, template)
}

func (h *Handler) applyActivityWorkflowDraftTemplate(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ApplyWorkflowDraftTemplateCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(cmd.TemplateID) == "" {
		cmd.TemplateID = r.PathValue("templateId")
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	draft, err := h.engine.ApplyWorkflowDraftTemplate(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (h *Handler) listActivityWorkflowVersions(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	versions, err := h.engine.ListWorkflowVersions(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": versions})
}

func (h *Handler) rollbackActivityWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.WorkflowVersionActionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionAdmin) {
		return
	}
	version, err := h.engine.RollbackWorkflowVersion(r.Context(), r.PathValue("id"), r.PathValue("versionId"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

func (h *Handler) archiveActivityWorkflowVersion(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.WorkflowVersionActionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionAdmin) {
		return
	}
	version, err := h.engine.ArchiveWorkflowVersion(r.Context(), r.PathValue("id"), r.PathValue("versionId"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, version)
}

func (h *Handler) listActivityWorkflowExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView) {
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}
	pageLimit := limit
	if pageLimit <= 0 || pageLimit > 100 {
		pageLimit = 20
	}
	query := marketing.WorkflowExecutionQuery{
		ActivityID:        r.PathValue("id"),
		TenantID:          strings.TrimSpace(r.URL.Query().Get("tenantId")),
		WorkflowVersionID: strings.TrimSpace(r.URL.Query().Get("workflowVersionId")),
		EventID:           strings.TrimSpace(r.URL.Query().Get("eventId")),
		Limit:             pageLimit + 1,
		Offset:            offset,
	}
	executions, err := h.engine.ListWorkflowExecutions(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasMore := len(executions) > pageLimit
	if hasMore {
		executions = executions[:pageLimit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      executions,
		"limit":      pageLimit,
		"offset":     offset,
		"nextOffset": offset + len(executions),
		"hasMore":    hasMore,
	})
}

type workflowResourceAccess struct {
	ActivityID  string
	WorkspaceID string
	FolderID    string
}

func (h *Handler) workflowExecutionResource(r *http.Request, executionID string) (marketing.WorkflowExecution, workflowResourceAccess, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return marketing.WorkflowExecution{}, workflowResourceAccess{}, marketing.ErrNotFound
	}
	executions, err := h.engine.ListWorkflowExecutions(r.Context(), marketing.WorkflowExecutionQuery{ExecutionID: executionID, Limit: 1})
	if err != nil {
		return marketing.WorkflowExecution{}, workflowResourceAccess{}, err
	}
	if len(executions) == 0 {
		return marketing.WorkflowExecution{}, workflowResourceAccess{}, marketing.ErrNotFound
	}
	execution := executions[0]
	return execution, workflowResourceAccess{ActivityID: execution.ActivityID}, nil
}

func workflowDraftTemplateResource(template marketing.WorkflowDraftTemplate) workflowResourceAccess {
	resource := workflowResourceAccess{
		WorkspaceID: strings.TrimSpace(template.WorkspaceID),
		FolderID:    strings.TrimSpace(template.FolderID),
	}
	definition, err := marketing.ParseWorkflowDefinition(template.Definition)
	if err != nil {
		return resource
	}
	if resource.WorkspaceID == "" {
		resource.WorkspaceID = workflowTemplateRealMeta(definition.Meta, "workspaceId")
	}
	if resource.FolderID == "" {
		resource.FolderID = workflowTemplateRealMeta(definition.Meta, "folderId")
	}
	resource.ActivityID = workflowTemplateRealMeta(definition.Meta, "activityId")
	return resource
}

func workflowTemplateRealMeta(meta map[string]any, key string) string {
	value := workflowMetaString(meta, key)
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "template":
		return ""
	default:
		return value
	}
}

func workflowDraftTemplateVisibility(template marketing.WorkflowDraftTemplate) marketing.WorkspaceVisibility {
	if template.Visibility != "" {
		return template.Visibility
	}
	definition, err := marketing.ParseWorkflowDefinition(template.Definition)
	if err != nil {
		return marketing.WorkspaceTeam
	}
	return marketing.WorkspaceVisibility(workflowMetaString(definition.Meta, "visibility"))
}

func workflowMetaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	switch value := meta[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func (h *Handler) hasWorkflowResourcePermission(r *http.Request, resource workflowResourceAccess, permission workspacePermission) (bool, error) {
	if strings.TrimSpace(resource.ActivityID) != "" {
		return h.hasActivityWorkspacePermission(r, resource.ActivityID, permission)
	}
	workspaceID := strings.TrimSpace(resource.WorkspaceID)
	if workspaceID == "" {
		return false, nil
	}
	allowed, err := h.hasWorkspacePermission(r, workspaceID, permission)
	if err != nil || !allowed {
		return allowed, err
	}
	folderID := strings.TrimSpace(resource.FolderID)
	if folderID == "" || actorFromRequest(r) == "" {
		return true, nil
	}
	folder, err := h.engine.GetWorkspaceFolder(r.Context(), workspaceID, folderID)
	if err != nil {
		return false, err
	}
	if !h.canViewFolder(r, workspaceID, folder) {
		return false, nil
	}
	return true, nil
}

func (h *Handler) requireWorkflowResourcePermission(w http.ResponseWriter, r *http.Request, resource workflowResourceAccess, permission workspacePermission) bool {
	allowed, err := h.hasWorkflowResourcePermission(r, resource, permission)
	if err != nil {
		workspaceError(w, err)
		return false
	}
	if !allowed {
		workspaceError(w, marketing.ErrForbidden)
		return false
	}
	return true
}

func (h *Handler) requireWorkflowExecutionWorkspacePermission(w http.ResponseWriter, r *http.Request, executionID string, permission workspacePermission) (marketing.WorkflowExecution, bool) {
	execution, resource, err := h.workflowExecutionResource(r, executionID)
	if err != nil {
		workspaceError(w, err)
		return marketing.WorkflowExecution{}, false
	}
	if !h.requireWorkflowResourcePermission(w, r, resource, permission) {
		return marketing.WorkflowExecution{}, false
	}
	return execution, true
}

func (h *Handler) listWorkflowNodeExecutions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView); !ok {
		return
	}
	var executions []marketing.WorkflowNodeExecution
	var err error
	limit := intQuery(r, "limit")
	offset := intQuery(r, "offset")
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	query := marketing.WorkflowNodeExecutionQuery{ExecutionID: r.PathValue("id"), Limit: limit + 1, Offset: offset}
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("payload")), "summary") {
		executions, err = h.engine.ListWorkflowNodeExecutionsPage(r.Context(), query)
		for index := range executions {
			executions[index].Input = nil
			executions[index].Output = nil
		}
	} else {
		executions, err = h.engine.ListWorkflowNodeExecutionsPage(r.Context(), query)
	}
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasMore := len(executions) > limit
	if hasMore {
		executions = executions[:limit]
	}
	nextOffset := offset + len(executions)
	writeJSON(w, http.StatusOK, map[string]any{"items": executions, "limit": limit, "offset": offset, "nextOffset": nextOffset, "hasMore": hasMore})
}

func (h *Handler) getWorkflowNodeExecution(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView); !ok {
		return
	}
	execution, err := h.engine.GetWorkflowNodeExecution(r.Context(), r.PathValue("id"), r.PathValue("nodeExecutionId"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, execution)
}

func (h *Handler) compareWorkflowExecutions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView); !ok {
		return
	}
	baselineExecutionID := strings.TrimSpace(r.URL.Query().Get("baselineExecutionId"))
	if baselineExecutionID != "" {
		if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, baselineExecutionID, workspacePermissionView); !ok {
			return
		}
	}
	compare, err := h.engine.CompareWorkflowExecutionsWithOptions(r.Context(), r.PathValue("id"), baselineExecutionID, marketing.WorkflowExecutionCompareOptions{
		MaxPayloadChangeNodes: intQuery(r, "maxPayloadChangeNodes"),
		MaxJSONDiffRows:       intQuery(r, "maxJsonDiffRows"),
	})
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, compare)
}

func (h *Handler) exportWorkflowExecutionTrace(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionView); !ok {
		return
	}
	trace, err := h.engine.ExportWorkflowExecutionTrace(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activity, activityErr := h.engine.GetActivity(r.Context(), trace.Execution.ActivityID)
	if activityErr == nil && strings.TrimSpace(activity.WorkspaceID) != "" {
		_, _ = h.storeWorkspaceJSONFile(r, activity.WorkspaceID, "execution-trace-"+trace.Execution.ID+".json", trace, workspaceFileMeta{
			FolderID:          activity.FolderID,
			ActivityID:        activity.ID,
			WorkflowVersionID: trace.Execution.WorkflowVersionID,
			ExecutionID:       trace.Execution.ID,
			Kind:              marketing.WorkspaceFileExecutionTrace,
			CreatedBy:         actorFromRequest(r),
		})
	}
	writeJSON(w, http.StatusOK, trace)
}

func (h *Handler) rerunWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.WorkflowDebugRunCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if !h.requireActivityWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit) {
		return
	}
	trace, err := h.engine.RerunWorkflowExecution(r.Context(), r.PathValue("id"), r.PathValue("executionId"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, trace)
}

func (h *Handler) cancelWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.CancelWorkflowExecutionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit); !ok {
		return
	}
	execution, err := h.engine.CancelWorkflowExecution(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, execution)
}

func (h *Handler) resumeWorkflowExecution(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ResumeWorkflowExecutionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if _, ok := h.requireWorkflowExecutionWorkspacePermission(w, r, r.PathValue("id"), workspacePermissionEdit); !ok {
		return
	}
	trace, err := h.engine.ResumeWorkflowExecution(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidWorkflow) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, trace)
}

func (h *Handler) listWorkflowNodeConfigSchemas(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": h.engine.ListWorkflowNodeConfigSchemas()})
}

func (h *Handler) listWorkflowNodePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": h.engine.ListWorkflowNodePresets()})
}

func (h *Handler) listWorkflowDraftTemplates(w http.ResponseWriter, r *http.Request) {
	latestOnly := true
	if raw := strings.TrimSpace(r.URL.Query().Get("latestOnly")); raw != "" {
		latestOnly, _ = strconv.ParseBool(raw)
	}
	items, err := h.engine.ListWorkflowDraftTemplates(r.Context(), marketing.WorkflowDraftTemplateQuery{
		TemplateID:  strings.TrimSpace(r.URL.Query().Get("templateId")),
		Source:      marketing.WorkflowDraftTemplateSource(strings.TrimSpace(r.URL.Query().Get("source"))),
		Status:      marketing.WorkflowDraftTemplateStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId")),
		FolderID:    strings.TrimSpace(r.URL.Query().Get("folderId")),
		Visibility:  marketing.WorkspaceVisibility(strings.TrimSpace(r.URL.Query().Get("visibility"))),
		LatestOnly:  latestOnly,
		Limit:       intQuery(r, "limit"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = h.filterVisibleWorkflowDraftTemplates(r, items)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getWorkflowDraftTemplate(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("version")))
	item, err := h.engine.GetWorkflowDraftTemplate(r.Context(), r.PathValue("templateId"), strings.TrimSpace(r.URL.Query().Get("versionId")), version)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.requireWorkflowDraftTemplatePermission(w, r, item, workspacePermissionView) {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) updateWorkflowDraftTemplate(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("version")))
	existing, err := h.engine.GetWorkflowDraftTemplate(r.Context(), r.PathValue("templateId"), strings.TrimSpace(r.URL.Query().Get("versionId")), version)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.requireWorkflowDraftTemplatePermission(w, r, existing, workspacePermissionAdmin) {
		return
	}
	var cmd marketing.UpdateWorkflowDraftTemplateCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(cmd.VersionID) == "" {
		cmd.VersionID = strings.TrimSpace(r.URL.Query().Get("versionId"))
	}
	if cmd.Version <= 0 {
		cmd.Version = version
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	updated, err := h.engine.UpdateWorkflowDraftTemplate(r.Context(), r.PathValue("templateId"), cmd)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) copyWorkflowDraftTemplate(w http.ResponseWriter, r *http.Request) {
	version, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("version")))
	source, err := h.engine.GetWorkflowDraftTemplate(r.Context(), r.PathValue("templateId"), strings.TrimSpace(r.URL.Query().Get("versionId")), version)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.requireWorkflowDraftTemplatePermission(w, r, source, workspacePermissionView) {
		return
	}
	var cmd marketing.CopyWorkflowDraftTemplateCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(cmd.VersionID) == "" {
		cmd.VersionID = strings.TrimSpace(r.URL.Query().Get("versionId"))
	}
	if cmd.Version <= 0 {
		cmd.Version = version
	}
	if actorID := actorFromRequest(r); actorID != "" && strings.TrimSpace(cmd.Operator) == "" {
		cmd.Operator = actorID
	}
	if strings.TrimSpace(cmd.WorkspaceID) != "" && !h.requireWorkspacePermission(w, r, cmd.WorkspaceID, workspacePermissionEdit) {
		return
	}
	copied, err := h.engine.CopyWorkflowDraftTemplate(r.Context(), r.PathValue("templateId"), cmd)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, copied)
}

func (h *Handler) compareWorkflowDraftTemplateVersions(w http.ResponseWriter, r *http.Request) {
	leftVersionID := strings.TrimSpace(r.URL.Query().Get("leftVersionId"))
	rightVersionID := strings.TrimSpace(r.URL.Query().Get("rightVersionId"))
	left, right, err := h.workflowDraftTemplateCompareVersions(r, r.PathValue("templateId"), leftVersionID, rightVersionID)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.requireWorkflowDraftTemplatePermission(w, r, left, workspacePermissionView) {
		return
	}
	if right.VersionID != left.VersionID && !h.requireWorkflowDraftTemplatePermission(w, r, right, workspacePermissionView) {
		return
	}
	result, err := h.engine.CompareWorkflowDraftTemplateVersions(r.Context(), r.PathValue("templateId"), leftVersionID, rightVersionID)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) workflowDraftTemplateCompareVersions(r *http.Request, templateID string, leftVersionID string, rightVersionID string) (marketing.WorkflowDraftTemplate, marketing.WorkflowDraftTemplate, error) {
	versions, err := h.engine.ListWorkflowDraftTemplateVersions(r.Context(), templateID)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, marketing.WorkflowDraftTemplate{}, err
	}
	if len(versions) == 0 {
		return marketing.WorkflowDraftTemplate{}, marketing.WorkflowDraftTemplate{}, marketing.ErrNotFound
	}
	left := versions[len(versions)-1]
	right := versions[0]
	for _, item := range versions {
		if leftVersionID != "" && item.VersionID == leftVersionID {
			left = item
		}
		if rightVersionID != "" && item.VersionID == rightVersionID {
			right = item
		}
	}
	if leftVersionID != "" && left.VersionID != leftVersionID {
		return marketing.WorkflowDraftTemplate{}, marketing.WorkflowDraftTemplate{}, marketing.ErrNotFound
	}
	if rightVersionID != "" && right.VersionID != rightVersionID {
		return marketing.WorkflowDraftTemplate{}, marketing.WorkflowDraftTemplate{}, marketing.ErrNotFound
	}
	return left, right, nil
}

func (h *Handler) listWorkflowDraftTemplateVersions(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.ListWorkflowDraftTemplateVersions(r.Context(), r.PathValue("templateId"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = h.filterVisibleWorkflowDraftTemplates(r, items)
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, marketing.ErrNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) filterVisibleWorkflowDraftTemplates(r *http.Request, items []marketing.WorkflowDraftTemplate) []marketing.WorkflowDraftTemplate {
	filtered := items[:0]
	for _, item := range items {
		if h.canViewWorkflowDraftTemplate(r, item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (h *Handler) requireWorkflowDraftTemplatePermission(w http.ResponseWriter, r *http.Request, item marketing.WorkflowDraftTemplate, permission workspacePermission) bool {
	if item.Source == marketing.WorkflowDraftTemplateBuiltin {
		return true
	}
	return h.requireWorkflowResourcePermission(w, r, workflowDraftTemplateResource(item), permission)
}

func (h *Handler) canViewWorkflowDraftTemplate(r *http.Request, item marketing.WorkflowDraftTemplate) bool {
	if item.Source == marketing.WorkflowDraftTemplateBuiltin {
		return true
	}
	allowed, err := h.hasWorkflowResourcePermission(r, workflowDraftTemplateResource(item), workspacePermissionView)
	return err == nil && allowed
}

func (h *Handler) submitActivityApproval(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.SubmitApprovalCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	activity, err := h.engine.SubmitActivityApproval(r.Context(), r.PathValue("id"), cmd)
	writeActivityApprovalResponse(w, activity, err)
}

func (h *Handler) approveActivity(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ApprovalActionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	activity, err := h.engine.ApproveActivity(r.Context(), r.PathValue("id"), cmd)
	writeActivityApprovalResponse(w, activity, err)
}

func (h *Handler) rejectActivityApproval(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ApprovalActionCommand
	if err := decodeStrictJSON(r.Body, &cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	activity, err := h.engine.RejectActivityApproval(r.Context(), r.PathValue("id"), cmd)
	writeActivityApprovalResponse(w, activity, err)
}

func (h *Handler) listActivityApprovalRecords(w http.ResponseWriter, r *http.Request) {
	records, err := h.engine.ListApprovalRecords(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records})
}

func writeActivityApprovalResponse(w http.ResponseWriter, activity marketing.Activity, err error) {
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, activity)
}

func (h *Handler) validatePrelaunch(w http.ResponseWriter, r *http.Request) {
	var req marketing.PrelaunchValidationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := h.engine.ValidatePrelaunch(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createQuota(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.CreateQuotaCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	quota, err := h.engine.CreateQuota(r.Context(), cmd)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, quota)
}

func (h *Handler) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	startTime, err := parseOptionalRFC3339Query(r, "startTime")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	endTime, err := parseOptionalRFC3339Query(r, "endTime")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	records, err := h.engine.ListAudit(r.Context(), marketing.AuditQuery{
		TenantID:   r.URL.Query().Get("tenantId"),
		Actor:      r.URL.Query().Get("actor"),
		Action:     r.URL.Query().Get("action"),
		Resource:   r.URL.Query().Get("resource"),
		ResourceID: r.URL.Query().Get("resourceId"),
		ActivityID: r.URL.Query().Get("activityId"),
		EventID:    r.URL.Query().Get("eventId"),
		CommandID:  r.URL.Query().Get("commandId"),
		StartTime:  startTime,
		EndTime:    endTime,
		TraceID:    r.URL.Query().Get("traceId"),
		RequestID:  r.URL.Query().Get("requestId"),
		Limit:      limit,
	})
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records})
}

func parseOptionalRFC3339Query(r *http.Request, name string) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339", name)
	}
	return parsed.UTC(), nil
}

func (h *Handler) releaseExpiredQuotaReservations(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.QuotaTimeoutReleaseCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_TIMEOUT_RELEASE_REQUEST", "invalid json body")
		return
	}
	result, err := h.engine.ReleaseExpiredQuotaReservations(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_TIMEOUT_RELEASE_REQUEST", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaReservationConflict) {
			writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaAggregateConflict) {
			writeErrorCode(w, http.StatusConflict, "QUOTA_AGGREGATE_CONFLICT", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listOutboxDLQ(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	includeClosed := r.URL.Query().Get("includeClosed") == "true"
	messages, err := h.engine.ListOutboxDLQ(r.Context(), r.URL.Query().Get("tenantId"), includeClosed, limit)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": messages})
}

func (h *Handler) replayOutboxDLQ(w http.ResponseWriter, r *http.Request) {
	h.handleOutboxDLQOperation(w, r, h.engine.ReplayOutboxDLQ)
}

func (h *Handler) closeOutboxDLQ(w http.ResponseWriter, r *http.Request) {
	h.handleOutboxDLQOperation(w, r, h.engine.CloseOutboxDLQ)
}

func (h *Handler) listRewardReconciliation(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.engine.ListRewardReconciliation(r.Context(), marketing.RewardReconciliationQuery{
		TenantID:        r.URL.Query().Get("tenantId"),
		CommandID:       r.URL.Query().Get("commandId"),
		GrantOrderID:    r.URL.Query().Get("grantOrderId"),
		ActivityID:      r.URL.Query().Get("activityId"),
		SubjectType:     r.URL.Query().Get("subjectType"),
		SubjectID:       r.URL.Query().Get("subjectId"),
		EventID:         r.URL.Query().Get("eventId"),
		IdempotencyKey:  r.URL.Query().Get("idempotencyKey"),
		ParticipationID: r.URL.Query().Get("participationId"),
		OutboxID:        r.URL.Query().Get("outboxId"),
		Limit:           limit,
	})
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_RECONCILIATION_QUERY", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "diagnostics": []string{}})
}

func (h *Handler) applyManualGrantDecision(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ManualGrantDecisionCommand
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, defaultEventMaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_MANUAL_GRANT_REQUEST", "invalid json body")
		return
	}
	result, err := h.engine.ApplyManualGrantDecision(r.Context(), cmd)
	if err != nil {
		writeManualGrantError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createManualGrantExport(w http.ResponseWriter, r *http.Request) {
	var cmd marketing.ManualGrantExportCreateCommand
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, defaultEventMaxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_MANUAL_EXPORT_REQUEST", "invalid json body")
		return
	}
	result, err := h.engine.CreateManualGrantExport(r.Context(), cmd)
	if err != nil {
		writeManualGrantError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func writeManualGrantError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketing.ErrInvalidOperation):
		writeErrorCode(w, http.StatusBadRequest, "INVALID_MANUAL_GRANT_REQUEST", err.Error())
	case errors.Is(err, marketing.ErrNotFound):
		writeErrorCode(w, http.StatusNotFound, "GRANT_ORDER_NOT_FOUND", err.Error())
	case errors.Is(err, marketing.ErrCallbackLocatorConflict):
		writeErrorCode(w, http.StatusConflict, "GRANT_ORDER_LOCATOR_CONFLICT", err.Error())
	case errors.Is(err, marketing.ErrManualGrantExportRequired):
		writeErrorCode(w, http.StatusConflict, "MANUAL_EXPORT_REQUIRED", err.Error())
	case errors.Is(err, marketing.ErrGrantOrderClosed):
		writeErrorCode(w, http.StatusConflict, "GRANT_ORDER_CLOSED", err.Error())
	case errors.Is(err, marketing.ErrCallbackConflict):
		writeErrorCode(w, http.StatusConflict, "GRANT_ORDER_STATUS_CONFLICT", err.Error())
	case errors.Is(err, marketing.ErrQuotaReservationMissing):
		writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_MISSING", err.Error())
	case errors.Is(err, marketing.ErrQuotaReservationConflict):
		writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_CONFLICT", err.Error())
	case errors.Is(err, marketing.ErrQuotaAggregateConflict):
		writeErrorCode(w, http.StatusConflict, "QUOTA_AGGREGATE_CONFLICT", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) handleOutboxDLQOperation(w http.ResponseWriter, r *http.Request, operation func(context.Context, string, marketing.DLQOperationCommand) (marketing.OutboxMessage, error)) {
	var cmd marketing.DLQOperationCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	message, err := operation(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidOperation) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, message)
}

func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	var event marketing.ActivityEvent
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	result, err := h.engine.SimulateEvent(r.Context(), event)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	event := marketing.ActivityEvent{}
	record := func(result, reason string) {
		h.recordEventIntakeMetric(event, result, reason, startedAt)
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		record("failed", "invalid_payload")
		writeEventError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "unsupported media type")
		return
	}

	token := ""
	if h.openAPIEventCfg.RequireAuth {
		var ok bool
		token, ok = bearerToken(r.Header.Get("Authorization"))
		if !ok {
			record("auth_failed", "auth_failed")
			writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		if _, ok := h.openAPIEventCfg.Credentials[token]; !ok {
			record("auth_failed", "auth_failed")
			writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.openAPIEventCfg.MaxBodyBytes))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		if isRequestTooLarge(err) {
			record("failed", "invalid_payload")
			writeEventError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
			return
		}
		if isJSONSyntaxError(err) {
			record("failed", "invalid_payload")
			writeEventError(w, http.StatusBadRequest, "INVALID_JSON", "invalid json")
			return
		}
		record("failed", "invalid_payload")
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		record("failed", "idempotency_key_mismatch")
		writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	if err := event.Validate(); err != nil {
		record("failed", "invalid_payload")
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	if h.openAPIEventCfg.RequireAuth {
		credential := h.openAPIEventCfg.Credentials[token]
		if _, ok := credential.TenantIDs[event.TenantID]; !ok {
			record("auth_failed", "auth_failed")
			writeEventError(w, http.StatusForbidden, "TENANT_FORBIDDEN", "tenant forbidden")
			return
		}
		if _, ok := credential.SourceSystems[event.SourceSystem]; !ok {
			record("auth_failed", "auth_failed")
			writeEventError(w, http.StatusForbidden, "SOURCE_FORBIDDEN", "source forbidden")
			return
		}
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		if isRequestTooLarge(err) {
			record("failed", "invalid_payload")
			writeEventError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
			return
		}
		if isJSONSyntaxError(err) {
			record("failed", "invalid_payload")
			writeEventError(w, http.StatusBadRequest, "INVALID_JSON", "invalid json")
			return
		}
		record("failed", "invalid_payload")
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	if key != event.IdempotencyKey() {
		record("failed", "idempotency_key_mismatch")
		writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key does not match event payload")
		return
	}

	result, err := h.engine.ProcessEvent(r.Context(), event)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidEvent) {
			record("failed", "invalid_payload")
			writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrIdempotencyConflict) {
			record("failed", "idempotency_key_mismatch")
			writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaExceeded) {
			record("rejected", "quota_exceeded")
			writeQuotaError(w, err)
			return
		}
		record("failed", "internal_error")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Duplicate {
		record("duplicate", "duplicate")
		writeJSON(w, http.StatusConflict, map[string]any{
			"errorCode":      "DUPLICATE_EVENT",
			"code":           "DUPLICATE_EVENT",
			"message":        "duplicate eventId rejected",
			"error":          "duplicate eventId rejected",
			"inbox":          result.Inbox,
			"participations": result.Participations,
			"grantOrders":    result.GrantOrders,
			"outboxMessages": result.OutboxMessages,
			"ruleResults":    result.RuleResults,
			"duplicate":      true,
		})
		return
	}
	if processEventResultRejected(result) {
		record("rejected", "business_rejected")
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"errorCode":      "BUSINESS_REJECTED",
			"code":           "BUSINESS_REJECTED",
			"message":        "event processed but business rules rejected it",
			"error":          "event processed but business rules rejected it",
			"inbox":          result.Inbox,
			"participations": result.Participations,
			"grantOrders":    result.GrantOrders,
			"outboxMessages": result.OutboxMessages,
			"ruleResults":    result.RuleResults,
			"duplicate":      false,
		})
		return
	}
	record("success", "ok")
	writeJSON(w, http.StatusAccepted, result)
}

type checkinStatusResponse struct {
	TenantID            string                 `json:"tenantId"`
	ActivityCode        string                 `json:"activityCode,omitempty"`
	Subject             marketing.EventSubject `json:"subject"`
	Date                string                 `json:"date"`
	Signed              bool                   `json:"signed"`
	Rewarded            bool                   `json:"rewarded"`
	ParticipationStatus string                 `json:"participationStatus,omitempty"`
	GrantStatus         string                 `json:"grantStatus,omitempty"`
	RewardType          string                 `json:"rewardType,omitempty"`
	RewardCode          string                 `json:"rewardCode,omitempty"`
	RewardAmount        string                 `json:"rewardAmount,omitempty"`
	LastUpdatedAt       *time.Time             `json:"lastUpdatedAt,omitempty"`
	Records             []checkinStatusRecord  `json:"records,omitempty"`
}

type checkinStatusRecord struct {
	ParticipationStatus string     `json:"participationStatus,omitempty"`
	GrantStatus         string     `json:"grantStatus,omitempty"`
	RewardType          string     `json:"rewardType,omitempty"`
	RewardCode          string     `json:"rewardCode,omitempty"`
	RewardAmount        string     `json:"rewardAmount,omitempty"`
	OccurredAt          *time.Time `json:"occurredAt,omitempty"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

func (h *Handler) getCheckinStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.URL.Query().Get("tenantId"))
	activityID := strings.TrimSpace(r.URL.Query().Get("activityId"))
	activityCode := strings.TrimSpace(r.URL.Query().Get("activityCode"))
	subjectType := strings.TrimSpace(r.URL.Query().Get("subjectType"))
	subjectID := strings.TrimSpace(r.URL.Query().Get("subjectId"))
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if tenantID == "" || (activityID == "" && activityCode == "") || subjectType == "" || subjectID == "" {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CHECKIN_STATUS_QUERY", "tenantId, activityId or activityCode, subjectType and subjectId are required")
		return
	}
	if !h.authorizeOpenAPIStatusRequest(w, r, tenantID) {
		return
	}
	if date == "" {
		date = time.Now().UTC().Format(time.DateOnly)
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CHECKIN_STATUS_QUERY", "date must be YYYY-MM-DD")
		return
	}
	if activityID == "" {
		activity, err := h.findActivityByCode(r, tenantID, activityCode)
		if err != nil {
			if errors.Is(err, marketing.ErrNotFound) {
				writeErrorCode(w, http.StatusNotFound, "CHECKIN_ACTIVITY_NOT_FOUND", "checkin activity not found")
				return
			}
			writeErrorCode(w, http.StatusBadRequest, "INVALID_CHECKIN_STATUS_QUERY", err.Error())
			return
		}
		activityID = activity.ID
	}
	items, err := h.engine.ListRewardReconciliation(r.Context(), marketing.RewardReconciliationQuery{
		TenantID:    tenantID,
		ActivityID:  activityID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Limit:       100,
	})
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CHECKIN_STATUS_QUERY", err.Error())
		return
	}
	records := make([]checkinStatusRecord, 0, len(items))
	for _, item := range items {
		record := checkinStatusRecordFromReconciliation(item)
		if record.OccurredAt != nil && record.OccurredAt.UTC().Format(time.DateOnly) != date {
			continue
		}
		if record.OccurredAt == nil && record.UpdatedAt.UTC().Format(time.DateOnly) != date {
			continue
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	response := checkinStatusResponse{
		TenantID:     tenantID,
		ActivityCode: activityCode,
		Subject:      marketing.EventSubject{Type: subjectType, ID: subjectID},
		Date:         date,
		Records:      records,
	}
	for _, record := range records {
		if record.ParticipationStatus == string(marketing.ParticipationRewardable) ||
			record.ParticipationStatus == string(marketing.ParticipationGranted) ||
			record.GrantStatus != "" {
			response.Signed = true
			response.ParticipationStatus = record.ParticipationStatus
			response.GrantStatus = record.GrantStatus
			response.RewardType = record.RewardType
			response.RewardCode = record.RewardCode
			response.RewardAmount = record.RewardAmount
			response.LastUpdatedAt = &record.UpdatedAt
			response.Rewarded = record.GrantStatus != "" && record.GrantStatus != string(marketing.GrantFailed)
			break
		}
	}
	if !response.Signed && len(records) > 0 {
		record := records[0]
		response.ParticipationStatus = record.ParticipationStatus
		response.GrantStatus = record.GrantStatus
		response.RewardType = record.RewardType
		response.RewardCode = record.RewardCode
		response.RewardAmount = record.RewardAmount
		response.LastUpdatedAt = &record.UpdatedAt
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) authorizeOpenAPIStatusRequest(w http.ResponseWriter, r *http.Request, tenantID string) bool {
	if !h.openAPIEventCfg.RequireAuth {
		return true
	}
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return false
	}
	credential, ok := h.openAPIEventCfg.Credentials[token]
	if !ok {
		writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return false
	}
	if _, ok := credential.TenantIDs[tenantID]; !ok {
		writeEventError(w, http.StatusForbidden, "TENANT_FORBIDDEN", "tenant forbidden")
		return false
	}
	return true
}

func (h *Handler) findActivityByCode(r *http.Request, tenantID string, activityCode string) (marketing.Activity, error) {
	activities, err := h.engine.ListActivitiesByQuery(r.Context(), marketing.ActivityQuery{TenantID: tenantID, Limit: 500})
	if err != nil {
		return marketing.Activity{}, err
	}
	for _, activity := range activities {
		if activity.Code == activityCode {
			return activity, nil
		}
	}
	return marketing.Activity{}, marketing.ErrNotFound
}

func checkinStatusRecordFromReconciliation(item marketing.RewardReconciliationItem) checkinStatusRecord {
	record := checkinStatusRecord{
		ParticipationStatus: string(item.ParticipationStatus),
		GrantStatus:         string(item.GrantStatus),
		RewardType:          string(item.RewardType),
		RewardAmount:        item.RewardAmount,
		UpdatedAt:           item.ServerUpdatedAt,
	}
	if item.EventInbox != nil {
		occurredAt := item.EventInbox.Event.OccurredAt
		record.OccurredAt = &occurredAt
	}
	if item.Participation != nil {
		record.ParticipationStatus = firstCheckinStatusNonEmpty(record.ParticipationStatus, string(item.Participation.Status))
	}
	if item.GrantOrder != nil {
		record.GrantStatus = firstCheckinStatusNonEmpty(record.GrantStatus, string(item.GrantOrder.GrantStatus))
		record.RewardType = firstCheckinStatusNonEmpty(record.RewardType, string(item.GrantOrder.RewardType))
		record.RewardAmount = firstCheckinStatusNonEmpty(record.RewardAmount, item.GrantOrder.RewardAmount)
		record.RewardCode = rewardCodeFromPayload(item.GrantOrder.RewardPayload)
	}
	return record
}

func rewardCodeFromPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	if code, ok := body["rewardCode"].(string); ok {
		return code
	}
	return ""
}

func firstCheckinStatusNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func processEventResultRejected(result marketing.ProcessEventResult) bool {
	for _, participation := range result.Participations {
		if participation.Status == marketing.ParticipationRejected {
			return true
		}
	}
	for _, ruleResult := range result.RuleResults {
		if ruleResult.Decision == marketing.RuleDecisionRejected {
			return true
		}
	}
	return false
}

func (h *Handler) recordEventIntakeMetric(event marketing.ActivityEvent, result, reason string, startedAt time.Time) {
	registry := h.engine.MetricsRegistry()
	labels := []metrics.Label{
		metrics.L("tenant_id", event.TenantID),
		metrics.L("source_system", event.SourceSystem),
		metrics.L("event_type", event.EventType),
		metrics.L("result", result),
		metrics.L("reason", reason),
	}
	registry.IncCounter("exmarketing_event_intake_total", labels...)
	registry.Observe("exmarketing_event_intake_latency_seconds", time.Since(startedAt).Seconds(),
		metrics.L("tenant_id", event.TenantID),
		metrics.L("source_system", event.SourceSystem),
		metrics.L("event_type", event.EventType),
		metrics.L("result", result),
	)
}

func (h *Handler) rewardCallback(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	callback := marketing.RewardCallback{}
	record := func(result, reason string) {
		h.recordRewardCallbackMetric(callback, result, reason, startedAt)
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		record("rejected", "invalid_payload")
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CALLBACK_REQUEST", "unsupported media type")
		return
	}
	maxBodyBytes := h.rewardCallbackCfg.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultEventMaxBodyBytes
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		record("rejected", "invalid_payload")
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CALLBACK_REQUEST", "invalid request body")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&callback); err != nil {
		record("rejected", "invalid_payload")
		writeErrorCode(w, http.StatusBadRequest, "INVALID_CALLBACK_REQUEST", "invalid json body")
		return
	}
	callback = callback.Normalized()
	if err := h.verifyRewardCallbackSignature(r, callback.TenantID, body); err != nil {
		status := http.StatusUnauthorized
		code := "CALLBACK_SIGNATURE_INVALID"
		if errors.Is(err, errCallbackSignatureMissing) {
			code = "CALLBACK_SIGNATURE_MISSING"
		}
		if errors.Is(err, errCallbackTimestampInvalid) {
			code = "CALLBACK_TIMESTAMP_INVALID"
		}
		record("rejected", "signature_invalid")
		writeErrorCode(w, status, code, err.Error())
		return
	}

	result, err := h.engine.ApplyRewardCallback(r.Context(), callback)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			record("rejected", "grant_order_not_found")
			writeErrorCode(w, http.StatusNotFound, "GRANT_ORDER_NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidCallback) {
			record("rejected", "invalid_payload")
			writeErrorCode(w, http.StatusBadRequest, "INVALID_CALLBACK_REQUEST", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrCallbackConflict) {
			record("conflict", "status_conflict")
			writeErrorCode(w, http.StatusConflict, "CALLBACK_STATUS_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrCallbackLocatorConflict) {
			record("conflict", "status_conflict")
			writeErrorCode(w, http.StatusConflict, "CALLBACK_LOCATOR_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrGrantOrderClosed) {
			record("conflict", "status_conflict")
			writeErrorCode(w, http.StatusConflict, "GRANT_ORDER_CLOSED", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaReservationMissing) {
			record("failed", "quota_reservation_missing")
			writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_MISSING", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaReservationConflict) {
			record("failed", "quota_aggregate_conflict")
			writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaAggregateConflict) {
			record("failed", "quota_aggregate_conflict")
			writeErrorCode(w, http.StatusConflict, "QUOTA_AGGREGATE_CONFLICT", err.Error())
			return
		}
		record("failed", "internal_error")
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resultLabel := "accepted"
	reason := "ok"
	if result.Duplicate {
		resultLabel = "duplicate"
		reason = "duplicate"
	}
	record(resultLabel, reason)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) recordRewardCallbackMetric(callback marketing.RewardCallback, result, reason string, startedAt time.Time) {
	registry := h.engine.MetricsRegistry()
	registry.IncCounter("exmarketing_reward_callback_total",
		metrics.L("tenant_id", callback.TenantID),
		metrics.L("grant_channel", "unknown"),
		metrics.L("callback_status", string(callback.Status)),
		metrics.L("result", result),
		metrics.L("reason", reason),
	)
	registry.Observe("exmarketing_reward_callback_latency_seconds", time.Since(startedAt).Seconds(),
		metrics.L("tenant_id", callback.TenantID),
		metrics.L("grant_channel", "unknown"),
		metrics.L("result", result),
	)
}

var (
	errCallbackSignatureMissing = errors.New("callback signature missing")
	errCallbackSignatureInvalid = errors.New("callback signature invalid")
	errCallbackTimestampInvalid = errors.New("callback timestamp invalid")
)

func (h *Handler) verifyRewardCallbackSignature(r *http.Request, tenantID string, body []byte) error {
	secret := ""
	if h.rewardCallbackCfg.Secrets != nil {
		secret = strings.TrimSpace(h.rewardCallbackCfg.Secrets[tenantID])
	}
	if secret == "" {
		return nil
	}
	timestamp := strings.TrimSpace(r.Header.Get("X-ExMarketing-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-ExMarketing-Signature"))
	if timestamp == "" || signature == "" {
		return errCallbackSignatureMissing
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errCallbackTimestampInvalid
	}
	signedAt := time.Unix(unixSeconds, 0).UTC()
	maxSkew := h.rewardCallbackCfg.MaxSkew
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	if time.Since(signedAt) > maxSkew || signedAt.Sub(time.Now().UTC()) > maxSkew {
		return errCallbackTimestampInvalid
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return errCallbackSignatureInvalid
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return errCallbackSignatureInvalid
	}
	return nil
}

func (h *Handler) queryFacts(w http.ResponseWriter, r *http.Request) {
	var request marketing.FactQueryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, marketing.FactQueryResponse{Success: false, ErrorCode: "INVALID_FACT_QUERY", Error: "invalid json body"})
		return
	}
	response, err := h.engine.QueryFacts(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !response.Success {
		writeJSON(w, http.StatusBadRequest, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) userProfileFactProvider(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if !isLoopbackRequest(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"success": false, "error": "internal fact provider is local-only"})
		return
	}
	var request marketing.FactQueryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid json body"})
		return
	}
	if request.TenantID == "" || request.Subject.Type == "" || request.Subject.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "tenantId and subject are required"})
		return
	}
	profile, err := h.engine.GetUserProfileFact(r.Context(), request.TenantID, request.Subject)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": profile})
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

func (h *Handler) listWorkflowRequirements(w http.ResponseWriter, r *http.Request) {
	backlog, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(backlog.Items))
	for _, req := range backlog.Items {
		reconcileCommitFailure(&req)
		status, _ := workflow.EnsureAgentStatus(req)
		items = append(items, map[string]any{
			"id":          req.ID,
			"title":       req.Title,
			"description": req.Description,
			"status":      req.Status,
			"risk":        req.Risk,
			"score":       req.Score,
			"taskDir":     req.TaskDir,
			"createdAt":   req.CreatedAt,
			"updatedAt":   req.UpdatedAt,
			"agents":      status.Agents,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getWorkflowRequirement(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	backlog, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, req := range backlog.Items {
		if req.ID != id {
			continue
		}
		reconcileCommitFailure(&req)
		if err := workflow.EnsureTaskFiles(req); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		status, err := workflow.EnsureAgentStatus(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		artifacts, err := readWorkflowArtifacts(req.TaskDir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		runs, err := workflow.ListRunHistory(req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"requirement": req,
			"agents":      status.Agents,
			"artifacts":   artifacts,
			"runs":        runs,
		})
		return
	}
	writeError(w, http.StatusNotFound, "requirement not found")
}

func readWorkflowArtifacts(taskDir string) (map[string]string, error) {
	cleanTaskDir := filepath.Clean(taskDir)
	if cleanTaskDir == "." || strings.HasPrefix(cleanTaskDir, "..") {
		return nil, errors.New("invalid taskDir")
	}
	files := []string{"task.md", "analysis.md", "review.md", "test-plan.md", "implementation.md", "test.md", "commit.md", "agent-report.md", "agent-prompts.md", "agent-status.json"}
	artifacts := make(map[string]string, len(files))
	for _, name := range files {
		path := filepath.Join(cleanTaskDir, name)
		bytes, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		artifacts[name] = string(bytes)
	}
	return artifacts, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	message = marketing.RedactSensitiveString(message)
	writeJSON(w, status, map[string]string{"error": message})
}

func writeErrorCode(w http.ResponseWriter, status int, code string, message string) {
	message = marketing.RedactSensitiveString(message)
	writeJSON(w, status, map[string]string{"code": code, "errorCode": code, "error": message, "message": message})
}

func withAuditSource(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
		}
		if requestID := strings.TrimSpace(r.Header.Get("X-Request-Id")); requestID != "" {
			source["requestId"] = requestID
		}
		if traceID := strings.TrimSpace(r.Header.Get("X-Trace-Id")); traceID != "" {
			source["traceId"] = traceID
		}
		if ip := maskedRemoteIP(r); ip != "" {
			source["ip"] = ip
		}
		if ua := summarizeUserAgent(r.UserAgent()); ua != "" {
			source["userAgent"] = ua
		}
		next.ServeHTTP(w, r.WithContext(marketing.WithAuditSource(r.Context(), source)))
	})
}

func maskedRemoteIP(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if raw != "" {
		raw = strings.TrimSpace(strings.Split(raw, ",")[0])
	} else {
		raw = strings.TrimSpace(r.RemoteAddr)
	}
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	if strings.Contains(raw, ".") {
		parts := strings.Split(raw, ".")
		if len(parts) == 4 {
			parts[3] = "0"
			return strings.Join(parts, ".")
		}
	}
	if strings.Contains(raw, ":") {
		parts := strings.Split(raw, ":")
		if len(parts) > 4 {
			return strings.Join(parts[:4], ":") + "::"
		}
	}
	return raw
}

func summarizeUserAgent(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		return value[:120]
	}
	return value
}

func decodeStrictJSON(r io.Reader, out any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func rejectActivityIdentityFields(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return errors.New("invalid json body")
	}
	for _, name := range []string{"id", "tenantId", "activityCode", "activityType"} {
		if _, ok := fields[name]; ok {
			return fmt.Errorf("%s cannot be updated", name)
		}
	}
	return nil
}

func writeEventError(w http.ResponseWriter, status int, code string, message string) {
	message = marketing.RedactSensitiveString(message)
	writeJSON(w, status, map[string]string{
		"errorCode": code,
		"message":   message,
		"code":      code,
		"error":     message,
	})
}

func isJSONContentType(contentType string) bool {
	if strings.TrimSpace(contentType) == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("request body must contain a single json object")
	}
	return err
}

func isRequestTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func isJSONSyntaxError(err error) bool {
	var syntaxErr *json.SyntaxError
	return errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF)
}

func writeQuotaError(w http.ResponseWriter, err error) {
	body := map[string]any{
		"error": err.Error(),
		"code":  "QUOTA_EXCEEDED",
	}
	var quotaErr marketing.QuotaExceededError
	if errors.As(err, &quotaErr) {
		body["quotaType"] = quotaErr.Type
		body["scope"] = quotaErr.Scope
		body["scopeKey"] = quotaErr.ScopeKey
	}
	writeJSON(w, http.StatusConflict, body)
}
