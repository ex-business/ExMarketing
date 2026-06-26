package dbmigration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigrationsOrdersAndChecksums(t *testing.T) {
	dir := t.TempDir()
	writeMigration(t, dir, "002_second.sql", "SELECT 2;")
	writeMigration(t, dir, "001_first.sql", "SELECT 1;")

	migrations, err := LoadMigrations(dir)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}
	if migrations[0].Version != 1 || migrations[0].FileName != "001_first.sql" {
		t.Fatalf("unexpected first migration: %+v", migrations[0])
	}
	if migrations[1].Version != 2 || migrations[1].FileName != "002_second.sql" {
		t.Fatalf("unexpected second migration: %+v", migrations[1])
	}
	if len(migrations[0].Checksum) != 64 {
		t.Fatalf("expected sha256 hex checksum, got %q", migrations[0].Checksum)
	}
}

func TestLoadMigrationsRejectsInvalidAndDuplicateVersions(t *testing.T) {
	t.Run("invalid filename", func(t *testing.T) {
		dir := t.TempDir()
		writeMigration(t, dir, "bad.sql", "SELECT 1;")

		_, err := LoadMigrations(dir)
		if err == nil || !strings.Contains(err.Error(), "invalid migration filename") {
			t.Fatalf("expected invalid filename error, got %v", err)
		}
	})

	t.Run("duplicate version", func(t *testing.T) {
		dir := t.TempDir()
		writeMigration(t, dir, "001_first.sql", "SELECT 1;")
		writeMigration(t, dir, "001_second.sql", "SELECT 2;")

		_, err := LoadMigrations(dir)
		if err == nil || !strings.Contains(err.Error(), "duplicate migration version") {
			t.Fatalf("expected duplicate version error, got %v", err)
		}
	})
}

func TestValidateRecordsDetectsDriftAndFailures(t *testing.T) {
	migrations := []Migration{
		{Version: 1, FileName: "001_first.sql", Checksum: strings.Repeat("a", 64)},
		{Version: 2, FileName: "002_second.sql", Checksum: strings.Repeat("b", 64)},
	}

	if err := validateRecords([]Record{{Version: 1, FileName: "001_first.sql", Checksum: strings.Repeat("a", 64), Status: StatusSuccess}}, migrations); err != nil {
		t.Fatalf("expected matching record to pass: %v", err)
	}
	if err := validateRecords([]Record{{Version: 1, FileName: "001_first.sql", Checksum: strings.Repeat("z", 64), Status: StatusSuccess}}, migrations); err == nil || !strings.Contains(err.Error(), "checksum drift") {
		t.Fatalf("expected checksum drift, got %v", err)
	}
	if err := validateRecords([]Record{{Version: 2, FileName: "002_second.sql", Checksum: strings.Repeat("b", 64), Status: StatusFailed}}, migrations); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected failed migration block, got %v", err)
	}
	if err := validateRecords([]Record{{Version: 3, FileName: "003_missing.sql", Checksum: strings.Repeat("c", 64), Status: StatusSuccess}}, migrations); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing local migration block, got %v", err)
	}
}

func TestPendingMigrationsSkipsSuccessfulRecords(t *testing.T) {
	migrations := []Migration{
		{Version: 1, FileName: "001_first.sql"},
		{Version: 2, FileName: "002_second.sql"},
		{Version: 3, FileName: "003_third.sql"},
	}
	pending := pendingMigrations([]Record{{Version: 1, Status: StatusSuccess}, {Version: 3, Status: StatusSuccess}}, migrations)
	if len(pending) != 1 || pending[0].Version != 2 {
		t.Fatalf("unexpected pending migrations: %+v", pending)
	}
}

func TestServiceAssetSoftDeleteUniqueMigrationDefinesPartialIndexes(t *testing.T) {
	migrations, err := LoadMigrations("../../db/migration")
	if err != nil {
		t.Fatalf("load project migrations: %v", err)
	}
	var bindingFields Migration
	var migration Migration
	for _, candidate := range migrations {
		if candidate.FileName == "018_service_interface_binding_long_term_fields.sql" {
			bindingFields = candidate
		}
		if candidate.FileName == "021_service_asset_soft_delete_unique_indexes.sql" {
			migration = candidate
		}
	}
	if bindingFields.FileName == "" {
		t.Fatalf("expected service interface binding long-term fields migration")
	}
	if migration.FileName == "" {
		t.Fatalf("expected service asset soft-delete unique migration")
	}
	if bindingFields.Version >= migration.Version {
		t.Fatalf("binding deleted_at migration must run before partial unique indexes: %d >= %d", bindingFields.Version, migration.Version)
	}
	if !strings.Contains(bindingFields.SQL, "ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ") {
		t.Fatalf("expected binding long-term fields migration to add deleted_at")
	}
	for _, want := range []string{
		"marketing_service_active_code_uidx",
		"marketing_service_interface_active_code_uidx",
		"marketing_service_provider_active_code_uidx",
		"marketing_service_schema_active_ref_version_uidx",
		"marketing_service_interface_binding_active_scope_uidx",
		"marketing_service_rate_limit_policy_active_code_uidx",
		"marketing_service_rate_limit_policy_active_scope_uidx",
	} {
		if !strings.Contains(migration.SQL, want) {
			t.Fatalf("expected migration to define %s", want)
		}
	}
	if got := strings.Count(migration.SQL, "WHERE deleted_at IS NULL"); got < 7 {
		t.Fatalf("expected partial unique indexes on deleted_at, got %d predicates", got)
	}
}

func writeMigration(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}
}
