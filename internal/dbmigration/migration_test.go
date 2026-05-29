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

func writeMigration(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}
}
