package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/ex-stack/ExMarketing/internal/dbmigration"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (h *Handler) recordDeniedMigrationAudit(r *http.Request, migrationType string, operation string, activityID string, scope string, message string) {
	h.engine.RecordMigrationAudit(r.Context(), marketing.MigrationAuditLog{
		TenantID:      tenantFromRequest(r),
		ActivityID:    activityID,
		MigrationType: migrationType,
		Operation:     operation,
		Actor:         actorFromRequest(r),
		Scope:         scope,
		Result:        marketing.MigrationAuditDenied,
		Error:         message,
	})
}

func migrationAuditSummaryJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return payload
}

func dbMigrationAuditSummary(analysis dbmigration.Analysis) map[string]any {
	return map[string]any{
		"latest":                analysis.Latest,
		"safeToApply":           analysis.SafeToApply,
		"drift":                 analysis.Drift,
		"localMigrations":       len(analysis.LocalMigrations),
		"applied":               len(analysis.Applied),
		"pending":               len(analysis.Pending),
		"requiredTables":        len(analysis.RequiredTables),
		"missingRequiredTables": len(analysis.MissingRequiredTables),
		"legacyUntrackedTables": len(analysis.LegacyUntrackedTables),
		"errors":                len(analysis.Errors),
	}
}

func dbMigrationBlockersCount(analysis dbmigration.Analysis) int {
	count := len(analysis.Errors) + len(analysis.MissingRequiredTables) + len(analysis.LegacyUntrackedTables)
	if analysis.Drift {
		count++
	}
	return count
}

func workflowMigrationAuditSummary(analysis workflowServiceInterfaceMigrationAnalysis) map[string]any {
	return map[string]any{
		"status":            analysis.Status,
		"legacySteps":       analysis.Summary.LegacySteps,
		"autoMigratable":    analysis.Summary.AutoMigratable,
		"manualReview":      analysis.Summary.ManualReview,
		"blockers":          analysis.Summary.Blockers,
		"serviceInterfaces": analysis.Summary.ServiceInterfaces,
		"resources":         analysis.Summary.Resources,
	}
}
