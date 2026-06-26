package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const dbMigrationApplyConfirmation = "APPLY_DB_MIGRATIONS"
const dbMigrationBreakGlassScope = "db-migration:apply"

type dbMigrationApplyRequest struct {
	DryRun              bool   `json:"dryRun"`
	Confirm             bool   `json:"confirm"`
	Confirmation        string `json:"confirmation"`
	BreakGlass          bool   `json:"breakGlass"`
	BreakGlassScope     string `json:"breakGlassScope"`
	BreakGlassActor     string `json:"breakGlassActor"`
	BreakGlassExpiresAt string `json:"breakGlassExpiresAt"`
	BreakGlassTicket    string `json:"breakGlassTicket"`
	Reason              string `json:"reason"`
}

func (h *Handler) getDBMigrationStatus(w http.ResponseWriter, r *http.Request) {
	h.analyzeDBMigrations(w, r)
}

func (h *Handler) analyzeDBMigrations(w http.ResponseWriter, r *http.Request) {
	if !h.requireMigrationAdmin(w, r) {
		h.recordDeniedMigrationAudit(r, "DB", "ANALYZE", "", "db-migration:admin", "platform admin or tenant workspace admin is required")
		return
	}
	audit := marketing.MigrationAuditLog{
		TenantID:      tenantFromRequest(r),
		MigrationType: "DB",
		Operation:     "ANALYZE",
		Actor:         actorFromRequest(r),
		Result:        marketing.MigrationAuditFailed,
	}
	defer func() { h.engine.RecordMigrationAudit(r.Context(), audit) }()
	if h.migrationManager == nil {
		audit.Error = "db migration manager is not configured"
		writeError(w, http.StatusServiceUnavailable, "db migration manager is not configured")
		return
	}
	analysis, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		audit.Error = err.Error()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audit.Result = marketing.MigrationAuditSuccess
	audit.Summary = migrationAuditSummaryJSON(dbMigrationAuditSummary(analysis))
	audit.ItemsCount = len(analysis.Pending)
	audit.BlockersCount = dbMigrationBlockersCount(analysis)
	writeJSON(w, http.StatusOK, map[string]any{
		"actor":    actorFromRequest(r),
		"analysis": analysis,
	})
}

func (h *Handler) applyDBMigrations(w http.ResponseWriter, r *http.Request) {
	if !h.requireMigrationAdmin(w, r) {
		h.recordDeniedMigrationAudit(r, "DB", "APPLY", "", dbMigrationBreakGlassScope, "platform admin or tenant workspace admin is required")
		return
	}
	audit := marketing.MigrationAuditLog{
		TenantID:      tenantFromRequest(r),
		MigrationType: "DB",
		Operation:     "APPLY",
		Actor:         actorFromRequest(r),
		Result:        marketing.MigrationAuditFailed,
	}
	defer func() { h.engine.RecordMigrationAudit(r.Context(), audit) }()
	if h.migrationManager == nil {
		audit.Error = "db migration manager is not configured"
		writeError(w, http.StatusServiceUnavailable, "db migration manager is not configured")
		return
	}
	var cmd dbMigrationApplyRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			if errors.Is(err, io.EOF) {
				cmd.DryRun = true
			} else {
				audit.Error = "invalid json body"
				writeError(w, http.StatusBadRequest, "invalid json body")
				return
			}
		}
	}
	audit.DryRun = cmd.DryRun
	if cmd.DryRun {
		audit.Operation = "DRY_RUN"
	}
	before, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		audit.Error = err.Error()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audit.Summary = migrationAuditSummaryJSON(map[string]any{"before": dbMigrationAuditSummary(before)})
	audit.ItemsCount = len(before.Pending)
	audit.BlockersCount = dbMigrationBlockersCount(before)
	if cmd.DryRun {
		audit.Result = marketing.MigrationAuditSuccess
		writeJSON(w, http.StatusOK, map[string]any{
			"actor":    actorFromRequest(r),
			"dryRun":   true,
			"analysis": before,
		})
		return
	}
	if !cmd.Confirm || strings.TrimSpace(cmd.Confirmation) != dbMigrationApplyConfirmation {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "confirm=true and confirmation=APPLY_DB_MIGRATIONS are required"
		writeError(w, http.StatusBadRequest, "confirm=true and confirmation=APPLY_DB_MIGRATIONS are required")
		return
	}
	breakGlass, ok := parseDBMigrationBreakGlass(r, cmd)
	if !ok {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "valid break-glass scope, actor, ticket, and unexpired ttl are required"
		writeError(w, http.StatusForbidden, "valid break-glass scope, actor, ticket, and unexpired ttl are required")
		return
	}
	audit.Scope = breakGlass.Scope
	audit.BreakGlassTicket = breakGlass.Ticket
	if strings.TrimSpace(cmd.Reason) == "" {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "reason is required"
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if before.Drift {
		audit.Result = marketing.MigrationAuditDenied
		audit.Error = "migration drift must be resolved before apply"
		writeError(w, http.StatusConflict, "migration drift must be resolved before apply")
		return
	}
	if _, err := h.migrationManager.Apply(r.Context()); err != nil {
		audit.Error = err.Error()
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	after, err := h.migrationManager.Analyze(r.Context())
	if err != nil {
		audit.Error = err.Error()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audit.Result = marketing.MigrationAuditSuccess
	audit.Summary = migrationAuditSummaryJSON(map[string]any{
		"reason": strings.TrimSpace(cmd.Reason),
		"before": dbMigrationAuditSummary(before),
		"after":  dbMigrationAuditSummary(after),
	})
	_ = h.engine.RecordAudit(r.Context(), tenantFromRequest(r), actorFromRequest(r), "DB_MIGRATION_APPLY", "DB_MIGRATION", "schema_migrations", map[string]any{
		"reason":     strings.TrimSpace(cmd.Reason),
		"breakGlass": breakGlass,
		"before":     before,
		"after":      after,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"actor":            actorFromRequest(r),
		"dryRun":           false,
		"breakGlass":       true,
		"breakGlassScope":  breakGlass.Scope,
		"breakGlassTicket": breakGlass.Ticket,
		"reason":           strings.TrimSpace(cmd.Reason),
		"before":           before,
		"after":            after,
		"confirmation":     dbMigrationApplyConfirmation,
	})
}

type migrationBreakGlassGrant struct {
	Scope     string    `json:"scope"`
	Actor     string    `json:"actor"`
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (h *Handler) requireMigrationAdmin(w http.ResponseWriter, r *http.Request) bool {
	actor := strings.TrimSpace(actorFromRequest(r))
	if actor == "" {
		writeError(w, http.StatusUnauthorized, "X-Actor-Id is required")
		return false
	}
	allowed, err := h.isMigrationAdmin(r, actor)
	if err != nil {
		workspaceError(w, err)
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "platform admin or tenant workspace admin is required")
		return false
	}
	return true
}

func (h *Handler) isMigrationAdmin(r *http.Request, actor string) (bool, error) {
	if isPlatformAdminRequest(r) {
		return true, nil
	}
	tenantID := tenantFromRequest(r)
	workspaces, err := h.engine.ListWorkspaces(r.Context(), marketing.WorkspaceQuery{
		TenantID: tenantID,
		Status:   marketing.WorkspaceActive,
		Limit:    200,
	})
	if err != nil {
		return false, err
	}
	for _, workspace := range workspaces {
		role, err := h.workspaceActorRole(r, workspace.ID, actor)
		if err != nil {
			return false, err
		}
		if workspaceRoleAllows(role, workspacePermissionAdmin) {
			return true, nil
		}
	}
	return false, nil
}

func isPlatformAdminRequest(r *http.Request) bool {
	role := strings.ToUpper(strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Platform-Role"), r.Header.Get("X-Admin-Role"))))
	if role != "ADMIN" && role != "OWNER" && role != "PLATFORM_ADMIN" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Platform-Admin"), r.Header.Get("X-Admin"))), "true")
}

func parseDBMigrationBreakGlass(r *http.Request, cmd dbMigrationApplyRequest) (migrationBreakGlassGrant, bool) {
	if !cmd.BreakGlass && !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Break-Glass")), "true") {
		return migrationBreakGlassGrant{}, false
	}
	grant := migrationBreakGlassGrant{
		Scope:  strings.TrimSpace(firstNonEmpty(cmd.BreakGlassScope, r.Header.Get("X-Break-Glass-Scope"))),
		Actor:  strings.TrimSpace(firstNonEmpty(cmd.BreakGlassActor, r.Header.Get("X-Break-Glass-Actor"))),
		Ticket: strings.TrimSpace(firstNonEmpty(cmd.BreakGlassTicket, r.Header.Get("X-Break-Glass-Ticket"))),
	}
	rawExpiresAt := strings.TrimSpace(firstNonEmpty(cmd.BreakGlassExpiresAt, r.Header.Get("X-Break-Glass-Expires-At")))
	expiresAt, err := time.Parse(time.RFC3339, rawExpiresAt)
	if err != nil {
		return migrationBreakGlassGrant{}, false
	}
	grant.ExpiresAt = expiresAt.UTC()
	if grant.Scope != dbMigrationBreakGlassScope {
		return migrationBreakGlassGrant{}, false
	}
	if grant.Actor == "" || grant.Actor != actorFromRequest(r) {
		return migrationBreakGlassGrant{}, false
	}
	if grant.Ticket == "" {
		return migrationBreakGlassGrant{}, false
	}
	now := time.Now().UTC()
	if !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(2*time.Hour)) {
		return migrationBreakGlassGrant{}, false
	}
	return grant, true
}
