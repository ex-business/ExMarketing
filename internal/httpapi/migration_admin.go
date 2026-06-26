package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const dbMigrationApplyConfirmation = "APPLY_DB_MIGRATIONS"

type dbMigrationApplyRequest struct {
	DryRun       bool   `json:"dryRun"`
	Confirm      bool   `json:"confirm"`
	Confirmation string `json:"confirmation"`
	BreakGlass   bool   `json:"breakGlass"`
	Reason       string `json:"reason"`
}

func (h *Handler) getDBMigrationStatus(w http.ResponseWriter, r *http.Request) {
	h.analyzeDBMigrations(w, r)
}

func (h *Handler) analyzeDBMigrations(w http.ResponseWriter, r *http.Request) {
	if !h.requireMigrationActor(w, r) {
		return
	}
	if h.migrationManager == nil {
		writeError(w, http.StatusServiceUnavailable, "db migration manager is not configured")
		return
	}
	analysis, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"actor":    actorFromRequest(r),
		"analysis": analysis,
	})
}

func (h *Handler) applyDBMigrations(w http.ResponseWriter, r *http.Request) {
	if !h.requireMigrationActor(w, r) {
		return
	}
	if h.migrationManager == nil {
		writeError(w, http.StatusServiceUnavailable, "db migration manager is not configured")
		return
	}
	var cmd dbMigrationApplyRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			if errors.Is(err, io.EOF) {
				cmd.DryRun = true
			} else {
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
		}
	}
	before, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cmd.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"actor":    actorFromRequest(r),
			"dryRun":   true,
			"analysis": before,
		})
		return
	}
	if !cmd.Confirm || strings.TrimSpace(cmd.Confirmation) != dbMigrationApplyConfirmation {
		writeError(w, http.StatusBadRequest, "confirm=true and confirmation=APPLY_DB_MIGRATIONS are required")
		return
	}
	if !cmd.BreakGlass && !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Break-Glass")), "true") {
		writeError(w, http.StatusForbidden, "breakGlass=true or X-Break-Glass=true is required")
		return
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if before.Drift {
		writeError(w, http.StatusConflict, "migration drift must be resolved before apply")
		return
	}
	if _, err := h.migrationManager.Apply(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	after, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"actor":        actorFromRequest(r),
		"dryRun":       false,
		"breakGlass":   true,
		"reason":       strings.TrimSpace(cmd.Reason),
		"before":       before,
		"after":        after,
		"confirmation": dbMigrationApplyConfirmation,
	})
}

func (h *Handler) requireMigrationActor(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(actorFromRequest(r)) == "" {
		writeError(w, http.StatusUnauthorized, "X-Actor-Id is required")
		return false
	}
	return true
}
