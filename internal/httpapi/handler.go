package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/fact"
	"github.com/ex-stack/ExMarketing/internal/marketing"
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
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if key := r.Header.Get("Idempotency-Key"); key != "" && key != event.IdempotencyKey() {
		writeError(w, http.StatusBadRequest, "Idempotency-Key does not match event payload")
		return
	}

	result, err := h.engine.ProcessEvent(r.Context(), event)
	if err != nil {
		if errors.Is(err, marketing.ErrInvalidEvent) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, marketing.ErrQuotaExceeded) {
			writeError(w, http.StatusConflict, err.Error())
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
