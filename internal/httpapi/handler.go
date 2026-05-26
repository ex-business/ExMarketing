package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
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
	engine          *engine.Service
	openAPIEventCfg OpenAPIEventConfig
	webhookCfg      WebhookReceiverConfig
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
	return &Handler{engine: engine, openAPIEventCfg: cfg, webhookCfg: webhookCfg}
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
	mux.HandleFunc("GET /admin/v1/activities", h.listActivities)
	mux.HandleFunc("POST /admin/v1/activities", h.createActivity)
	mux.HandleFunc("GET /admin/v1/activities/{id}", h.getActivity)
	mux.HandleFunc("PUT /admin/v1/activities/{id}", h.updateActivity)
	mux.HandleFunc("POST /admin/v1/activities/prelaunch-validation", h.validatePrelaunch)
	mux.HandleFunc("POST /admin/v1/quotas", h.createQuota)
	mux.HandleFunc("GET /admin/v1/audit", h.listAudit)
	mux.HandleFunc("GET /admin/v1/outbox/dlq", h.listOutboxDLQ)
	mux.HandleFunc("POST /admin/v1/outbox/{id}/replay", h.replayOutboxDLQ)
	mux.HandleFunc("POST /admin/v1/outbox/{id}/close", h.closeOutboxDLQ)
	mux.HandleFunc("GET /admin/v1/reward/reconciliation", h.listRewardReconciliation)
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
	mux.HandleFunc("POST /openapi/v1/webhooks/{receiverCode}", h.createWebhookEvent)
	mux.HandleFunc("POST /openapi/v1/webhooks/", h.missingWebhookReceiver)
	mux.HandleFunc("POST /openapi/v1/facts/query", h.queryFacts)
	mux.HandleFunc("POST /openapi/v1/reward/callback", h.rewardCallback)
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", http.FileServer(http.Dir("web/admin"))))
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) listActivities(w http.ResponseWriter, r *http.Request) {
	activities, err := h.engine.ListActivities(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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

	activity, err := h.engine.CreateActivity(r.Context(), cmd)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidActivity) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrActivityConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, activity)
}

func (h *Handler) getActivity(w http.ResponseWriter, r *http.Request) {
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
	records, err := h.engine.ListAudit(r.Context(), marketing.AuditQuery{
		TenantID:   r.URL.Query().Get("tenantId"),
		Actor:      r.URL.Query().Get("actor"),
		Action:     r.URL.Query().Get("action"),
		Resource:   r.URL.Query().Get("resource"),
		ResourceID: r.URL.Query().Get("resourceId"),
		ActivityID: r.URL.Query().Get("activityId"),
		EventID:    r.URL.Query().Get("eventId"),
		CommandID:  r.URL.Query().Get("commandId"),
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records})
}

func (h *Handler) listOutboxDLQ(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	includeClosed := r.URL.Query().Get("includeClosed") == "true"
	messages, err := h.engine.ListOutboxDLQ(r.Context(), r.URL.Query().Get("tenantId"), includeClosed, limit)
	if err != nil {
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
		TenantID:       r.URL.Query().Get("tenantId"),
		CommandID:      r.URL.Query().Get("commandId"),
		GrantOrderID:   r.URL.Query().Get("grantOrderId"),
		ActivityID:     r.URL.Query().Get("activityId"),
		SubjectType:    r.URL.Query().Get("subjectType"),
		SubjectID:      r.URL.Query().Get("subjectId"),
		EventID:        r.URL.Query().Get("eventId"),
		IdempotencyKey: r.URL.Query().Get("idempotencyKey"),
		Limit:          limit,
	})
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_RECONCILIATION_REQUEST", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeEventError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "unsupported media type")
		return
	}

	token := ""
	if h.openAPIEventCfg.RequireAuth {
		var ok bool
		token, ok = bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		if _, ok := h.openAPIEventCfg.Credentials[token]; !ok {
			writeEventError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
	}

	var event marketing.ActivityEvent
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, h.openAPIEventCfg.MaxBodyBytes))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		if isRequestTooLarge(err) {
			writeEventError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
			return
		}
		if isJSONSyntaxError(err) {
			writeEventError(w, http.StatusBadRequest, "INVALID_JSON", "invalid json")
			return
		}
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	if err := event.Validate(); err != nil {
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	if h.openAPIEventCfg.RequireAuth {
		credential := h.openAPIEventCfg.Credentials[token]
		if _, ok := credential.TenantIDs[event.TenantID]; !ok {
			writeEventError(w, http.StatusForbidden, "TENANT_FORBIDDEN", "tenant forbidden")
			return
		}
		if _, ok := credential.SourceSystems[event.SourceSystem]; !ok {
			writeEventError(w, http.StatusForbidden, "SOURCE_FORBIDDEN", "source forbidden")
			return
		}
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		if isRequestTooLarge(err) {
			writeEventError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "request body too large")
			return
		}
		if isJSONSyntaxError(err) {
			writeEventError(w, http.StatusBadRequest, "INVALID_JSON", "invalid json")
			return
		}
		writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	if key != event.IdempotencyKey() {
		writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key does not match event payload")
		return
	}

	result, err := h.engine.ProcessEvent(r.Context(), event)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidEvent) {
			writeEventError(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrIdempotencyConflict) {
			writeEventError(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaExceeded) {
			writeQuotaError(w, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Duplicate {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) rewardCallback(w http.ResponseWriter, r *http.Request) {
	var callback marketing.RewardCallback
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&callback); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := h.engine.ApplyRewardCallback(r.Context(), callback)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeErrorCode(w, http.StatusNotFound, "GRANT_ORDER_NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrInvalidCallback) {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_CALLBACK_REQUEST", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrCallbackConflict) {
			writeErrorCode(w, http.StatusConflict, "CALLBACK_STATUS_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrGrantOrderClosed) {
			writeErrorCode(w, http.StatusConflict, "GRANT_ORDER_CLOSED", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaReservationMissing) {
			writeErrorCode(w, http.StatusConflict, "QUOTA_RESERVATION_MISSING", err.Error())
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

func (h *Handler) queryFacts(w http.ResponseWriter, r *http.Request) {
	var request marketing.FactQueryRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
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
	writeJSON(w, status, map[string]string{"error": message})
}

func writeErrorCode(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
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
