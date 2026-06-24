package dbmigration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_(.+)\.sql$`)

type Migration struct {
	Version  int
	Name     string
	FileName string
	Checksum string
	SQL      string
}

type Record struct {
	Version      int
	Name         string
	FileName     string
	Checksum     string
	Status       string
	StartedAt    time.Time
	FinishedAt   sql.NullTime
	ErrorMessage sql.NullString
}

type Plan struct {
	Applied []Record
	Pending []Migration
}

type Manager struct {
	DB             *sql.DB
	MigrationDir   string
	RequiredTables []string
}

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("EX_MARKETING_DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func LoadMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migration dir: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	versions := map[int]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := migrationFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename %q: expected NNN_description.sql", entry.Name())
		}
		version := 0
		for _, r := range match[1] {
			version = version*10 + int(r-'0')
		}
		if previous, ok := versions[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %03d in %s and %s", version, previous, entry.Name())
		}
		versions[version] = entry.Name()

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(content)
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     strings.TrimSuffix(match[2], ".sql"),
			FileName: entry.Name(),
			Checksum: hex.EncodeToString(sum[:]),
			SQL:      string(content),
		})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	if len(migrations) == 0 {
		return nil, fs.ErrNotExist
	}
	return migrations, nil
}

func (m Manager) Precheck(ctx context.Context) (Plan, error) {
	migrations, err := LoadMigrations(m.dir())
	if err != nil {
		return Plan{}, err
	}
	if err := m.ensureVersionTable(ctx); err != nil {
		return Plan{}, err
	}
	if err := m.blockLegacyUntrackedSchema(ctx); err != nil {
		return Plan{}, err
	}
	records, err := m.records(ctx)
	if err != nil {
		return Plan{}, err
	}
	if err := validateRecords(records, migrations); err != nil {
		return Plan{}, err
	}
	pending := pendingMigrations(records, migrations)
	return Plan{Applied: records, Pending: pending}, nil
}

func (m Manager) Apply(ctx context.Context) (Plan, error) {
	plan, err := m.Precheck(ctx)
	if err != nil {
		return Plan{}, err
	}
	for _, migration := range plan.Pending {
		if err := m.applyOne(ctx, migration); err != nil {
			return Plan{}, err
		}
	}
	return m.Precheck(ctx)
}

func (m Manager) Drift(ctx context.Context) error {
	plan, err := m.Precheck(ctx)
	if err != nil {
		return err
	}
	if len(plan.Pending) > 0 {
		return fmt.Errorf("database is not at latest migration: %d pending", len(plan.Pending))
	}
	return m.checkRequiredTables(ctx)
}

func (m Manager) dir() string {
	if strings.TrimSpace(m.MigrationDir) == "" {
		return defaultMigrationDir()
	}
	return m.MigrationDir
}

func defaultMigrationDir() string {
	const relative = "db/migration"
	if _, err := os.Stat(relative); err == nil {
		return relative
	}
	wd, err := os.Getwd()
	if err != nil {
		return relative
	}
	for {
		candidate := filepath.Join(wd, relative)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return relative
		}
		wd = parent
	}
}

func (m Manager) requiredTables() []string {
	if len(m.RequiredTables) > 0 {
		return m.RequiredTables
	}
	return RequiredTables()
}

func RequiredTables() []string {
	return []string{
		"marketing_activity",
		"marketing_rule",
		"marketing_reward",
		"marketing_quota",
		"marketing_event_inbox",
		"marketing_participation",
		"marketing_grant_order",
		"marketing_outbox",
		"marketing_audit",
		"marketing_fact_provider",
		"marketing_service",
		"marketing_service_interface",
		"marketing_service_provider",
		"marketing_service_schema",
		"marketing_service_interface_binding",
		"marketing_service_provider_runtime_state",
		"marketing_service_call_audit",
		"marketing_activity_state",
		"marketing_storage_state",
		"marketing_resource",
		"marketing_resource_schema",
		"marketing_resource_binding",
		"marketing_resource_secret",
		"marketing_resource_audit",
		"marketing_user_profile",
		"marketing_kyc_profile",
		"marketing_risk_profile",
	}
}

func (m Manager) ensureVersionTable(ctx context.Context) error {
	_, err := m.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			file_name TEXT NOT NULL UNIQUE,
			checksum CHAR(64) NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
			started_at TIMESTAMPTZ NOT NULL,
			finished_at TIMESTAMPTZ,
			error_message TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_schema_migrations_status
			ON schema_migrations (status, version);
	`)
	return err
}

func (m Manager) blockLegacyUntrackedSchema(ctx context.Context) error {
	var migrationCount int
	if err := m.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		return err
	}
	if migrationCount > 0 {
		return nil
	}
	existing, err := m.existingRequiredTables(ctx)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}
	return fmt.Errorf("legacy schema detected without schema_migrations records: %s; create a reviewed baseline before applying migrations", strings.Join(existing, ", "))
}

func (m Manager) records(ctx context.Context) ([]Record, error) {
	rows, err := m.DB.QueryContext(ctx, `
		SELECT version, name, file_name, checksum, status, started_at, finished_at, error_message
		FROM schema_migrations
		ORDER BY version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.Version, &record.Name, &record.FileName, &record.Checksum, &record.Status, &record.StartedAt, &record.FinishedAt, &record.ErrorMessage); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func validateRecords(records []Record, migrations []Migration) error {
	byVersion := map[int]Migration{}
	for _, migration := range migrations {
		byVersion[migration.Version] = migration
	}
	lastVersion := 0
	for _, record := range records {
		if record.Status == StatusFailed {
			return fmt.Errorf("migration %03d is failed: %s", record.Version, truncate(record.ErrorMessage.String, 160))
		}
		if record.Status != StatusSuccess {
			return fmt.Errorf("migration %03d has unsupported status %q", record.Version, record.Status)
		}
		if record.Version < lastVersion {
			return fmt.Errorf("schema_migrations records are out of order at version %03d", record.Version)
		}
		lastVersion = record.Version
		migration, ok := byVersion[record.Version]
		if !ok {
			return fmt.Errorf("applied migration %03d is missing from local migration files", record.Version)
		}
		if record.FileName != migration.FileName {
			return fmt.Errorf("migration %03d filename drift: database=%s local=%s", record.Version, record.FileName, migration.FileName)
		}
		if record.Checksum != migration.Checksum {
			return fmt.Errorf("migration %03d checksum drift: database=%s local=%s", record.Version, record.Checksum, migration.Checksum)
		}
	}
	return nil
}

func pendingMigrations(records []Record, migrations []Migration) []Migration {
	applied := map[int]bool{}
	for _, record := range records {
		if record.Status == StatusSuccess {
			applied[record.Version] = true
		}
	}
	var pending []Migration
	for _, migration := range migrations {
		if !applied[migration.Version] {
			pending = append(pending, migration)
		}
	}
	return pending
}

func (m Manager) applyOne(ctx context.Context, migration Migration) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(617247132)`); err != nil {
		rollback(tx)
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, file_name, checksum, status, started_at)
		VALUES ($1, $2, $3, $4, $5, now())
	`, migration.Version, migration.Name, migration.FileName, migration.Checksum, StatusFailed); err != nil {
		rollback(tx)
		return err
	}
	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		rollback(tx)
		_ = m.recordFailure(ctx, migration, err)
		return fmt.Errorf("apply migration %s: %w", migration.FileName, err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE schema_migrations
		SET status = $2, finished_at = now(), error_message = NULL
		WHERE version = $1
	`, migration.Version, StatusSuccess); err != nil {
		rollback(tx)
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (m Manager) recordFailure(ctx context.Context, migration Migration, applyErr error) error {
	_, err := m.DB.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, file_name, checksum, status, started_at, finished_at, error_message)
		VALUES ($1, $2, $3, $4, $5, now(), now(), $6)
		ON CONFLICT (version) DO UPDATE
		SET name = EXCLUDED.name,
		    file_name = EXCLUDED.file_name,
		    checksum = EXCLUDED.checksum,
		    status = EXCLUDED.status,
		    started_at = EXCLUDED.started_at,
		    finished_at = EXCLUDED.finished_at,
		    error_message = EXCLUDED.error_message
	`, migration.Version, migration.Name, migration.FileName, migration.Checksum, StatusFailed, truncate(sanitizeError(applyErr.Error()), 1000))
	return err
}

func (m Manager) checkRequiredTables(ctx context.Context) error {
	existing, err := m.existingRequiredTables(ctx)
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, table := range existing {
		found[table] = true
	}
	var missing []string
	for _, table := range m.requiredTables() {
		if !found[table] {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tables missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (m Manager) existingRequiredTables(ctx context.Context) ([]string, error) {
	required := m.requiredTables()
	args := make([]any, 0, len(required))
	placeholders := make([]string, 0, len(required))
	for i, table := range required {
		args = append(args, table)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
	}
	rows, err := m.DB.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY table_name
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func rollback(tx *sql.Tx) {
	_ = tx.Rollback()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func sanitizeError(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	for _, marker := range []string{"password=", "Password=", "token=", "Token=", "Authorization:"} {
		if idx := strings.Index(value, marker); idx >= 0 {
			return value[:idx] + marker + "[redacted]"
		}
	}
	return value
}
