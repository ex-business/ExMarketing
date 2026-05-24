package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/fact"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/workflow"
)

type Handler struct {
	engine *engine.Service
	facts  *fact.Client
}

func NewHandler(engine *engine.Service) *Handler {
	return &Handler{engine: engine, facts: fact.NewClient()}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /admin/v1/activities", h.listActivities)
	mux.HandleFunc("POST /admin/v1/activities", h.createActivity)
	mux.HandleFunc("POST /admin/v1/quotas", h.createQuota)
	mux.HandleFunc("GET /admin/v1/audit", h.listAudit)
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
	mux.HandleFunc("GET /admin/v1/workflow/requirements/{id}/logs/{agent}", h.getWorkflowAgentLog)
	mux.HandleFunc("GET /admin/v1/workflow/requirements/{id}/runs/{runID}", h.getWorkflowRunLog)
	mux.HandleFunc("GET /admin/v1/git/diff", h.getGitDiff)
	mux.HandleFunc("POST /admin/v1/git/commit", h.createGitCommit)
	mux.HandleFunc("GET /v1/campaigns", h.listActivities)
	mux.HandleFunc("POST /v1/campaigns", h.createActivity)
	mux.HandleFunc("POST /openapi/v1/events", h.createEvent)
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, activity)
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
	records, err := h.engine.ListAudit(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": records})
}

func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	var event marketing.ActivityEvent
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	result, err := h.engine.SimulateEvent(r.Context(), event)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	var event marketing.ActivityEvent
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EVENT", "invalid json body")
		return
	}

	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	if err := event.Validate(); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
		return
	}
	if key != event.IdempotencyKey() {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key does not match event payload")
		return
	}

	result, err := h.engine.ProcessEvent(r.Context(), event)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidEvent) {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_EVENT", err.Error())
			return
		}
		if errors.Is(err, marketing.ErrIdempotencyConflict) {
			writeErrorCode(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
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

	if err := h.engine.ApplyRewardCallback(r.Context(), callback); err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	response, err := h.facts.Query(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
