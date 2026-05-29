package dbbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/dbmigration"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, env []string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil || !isExecutableNotFound(err) || !isPostgresClientTool(name) {
		return out, err
	}
	fallbackOut, fallbackErr := DockerComposePostgresRunner{}.Run(ctx, name, args, env)
	if fallbackErr != nil {
		return fallbackOut, fmt.Errorf("%w; docker compose postgres fallback failed: %w", err, fallbackErr)
	}
	return fallbackOut, nil
}

type DockerComposePostgresRunner struct {
	Service string
}

func (r DockerComposePostgresRunner) Run(ctx context.Context, name string, args []string, env []string) ([]byte, error) {
	if !isPostgresClientTool(name) {
		return nil, fmt.Errorf("unsupported docker compose fallback command %q", name)
	}
	service := strings.TrimSpace(r.Service)
	if service == "" {
		service = "postgres"
	}
	args, outputFile, stdinFile := dockerPostgresToolArgs(name, args)
	args = rewriteLocalDSNsForContainer(args)

	dockerArgs := []string{"compose", "exec", "-T"}
	for _, item := range env {
		if strings.TrimSpace(item) != "" {
			dockerArgs = append(dockerArgs, "-e", item)
		}
	}
	dockerArgs = append(dockerArgs, service, name)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdinFile != "" {
		file, err := os.Open(stdinFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		cmd.Stdin = file
	}
	err := cmd.Run()
	combined := append(stdout.Bytes(), stderr.Bytes()...)
	if err != nil {
		return combined, err
	}
	if outputFile != "" {
		if err := os.WriteFile(outputFile, stdout.Bytes(), 0o600); err != nil {
			return combined, err
		}
	}
	return combined, nil
}

type BackupResult struct {
	Path       string
	SizeBytes  int64
	SHA256     string
	CreatedUTC time.Time
}

type TableCount struct {
	Table string `json:"table"`
	Rows  int64  `json:"rows"`
}

func Backup(ctx context.Context, runner CommandRunner, databaseURL, outputDir string, now time.Time) (BackupResult, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return BackupResult{}, errors.New("BACKUP: EX_MARKETING_DATABASE_URL is required")
	}
	if outputDir == "" {
		outputDir = "backups/db-restore-drills"
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return BackupResult{}, fmt.Errorf("BACKUP: create output directory: %w", err)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dbName := databaseName(databaseURL)
	if dbName == "" {
		dbName = "exmarketing"
	}
	path := filepath.Join(outputDir, fmt.Sprintf("%s_%s.dump", sanitizeFilePart(dbName), now.UTC().Format("20060102T150405Z")))
	if _, err := os.Stat(path); err == nil {
		return BackupResult{}, fmt.Errorf("BACKUP: refusing to overwrite existing backup %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupResult{}, fmt.Errorf("BACKUP: stat output file: %w", err)
	}

	commandDSN, env := commandDSN(databaseURL)
	args := []string{"--format=custom", "--no-owner", "--no-acl", "--file", path, commandDSN}
	out, err := runner.Run(ctx, "pg_dump", args, env)
	if err != nil {
		_ = os.Remove(path)
		return BackupResult{}, fmt.Errorf("BACKUP: pg_dump failed for %s: %w: %s", RedactDSN(databaseURL), err, sanitizeOutput(string(out)))
	}
	result, err := fileResult(path, now.UTC())
	if err != nil {
		return BackupResult{}, fmt.Errorf("BACKUP: inspect backup file: %w", err)
	}
	if result.SizeBytes == 0 {
		return BackupResult{}, fmt.Errorf("BACKUP: backup file is empty: %s", path)
	}
	return result, nil
}

func Restore(ctx context.Context, runner CommandRunner, db *sql.DB, sourceURL, targetURL, backupPath string, force bool) error {
	targetURL = strings.TrimSpace(targetURL)
	backupPath = strings.TrimSpace(backupPath)
	if targetURL == "" {
		return errors.New("RESTORE: target database URL is required")
	}
	if backupPath == "" {
		return errors.New("RESTORE: backup file is required")
	}
	if sameDSN(sourceURL, targetURL) && !force {
		return fmt.Errorf("RESTORE: source and restore database are the same (%s); use --force only for a reviewed local drill", RedactDSN(targetURL))
	}
	if info, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("RESTORE: backup file is not readable: %w", err)
	} else if info.Size() == 0 {
		return fmt.Errorf("RESTORE: backup file is empty: %s", backupPath)
	}
	if db != nil && !force {
		nonEmpty, err := HasPublicTables(ctx, db)
		if err != nil {
			return fmt.Errorf("RESTORE: inspect target database: %w", err)
		}
		if nonEmpty {
			return fmt.Errorf("RESTORE: target database is not empty (%s); pass --force after confirming this isolated restore target can be overwritten", RedactDSN(targetURL))
		}
	}
	commandTarget, env := commandDSN(targetURL)
	args := []string{"--clean", "--if-exists", "--no-owner", "--no-acl", "--dbname", commandTarget, backupPath}
	out, err := runner.Run(ctx, "pg_restore", args, env)
	if err != nil {
		return fmt.Errorf("RESTORE: pg_restore failed for %s: %w: %s", RedactDSN(targetURL), err, sanitizeOutput(string(out)))
	}
	return nil
}

func Validate(ctx context.Context, db *sql.DB, migrationDir string) ([]TableCount, error) {
	manager := dbmigration.Manager{DB: db, MigrationDir: migrationDir}
	if err := manager.Drift(ctx); err != nil {
		return nil, fmt.Errorf("SCHEMA_CHECK: %w", err)
	}
	counts := make([]TableCount, 0, len(dbmigration.RequiredTables()))
	for _, table := range dbmigration.RequiredTables() {
		var count int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			return nil, fmt.Errorf("DATA_CHECK: count %s: %w", table, err)
		}
		counts = append(counts, TableCount{Table: table, Rows: count})
	}
	return counts, nil
}

func HasPublicTables(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
	`).Scan(&count)
	return count > 0, err
}

func RedactDSN(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return redactKeyValue(value)
	}
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			parsed.User = url.UserPassword("[redacted]", "[redacted]")
		} else {
			parsed.User = url.UserPassword(username, "[redacted]")
		}
	}
	query := parsed.Query()
	for _, key := range []string{"password", "pass", "token", "secret"} {
		if query.Has(key) {
			query.Set(key, "[redacted]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func commandDSN(value string) (string, []string) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return value, nil
	}
	var env []string
	if parsed.User != nil {
		username := parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			env = append(env, "PGPASSWORD="+password)
		}
		if username == "" {
			parsed.User = nil
		} else {
			parsed.User = url.User(username)
		}
	}
	query := parsed.Query()
	for _, key := range []string{"password", "pass"} {
		if query.Has(key) {
			if len(env) == 0 {
				env = append(env, "PGPASSWORD="+query.Get(key))
			}
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), env
}

func isExecutableNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

func isPostgresClientTool(name string) bool {
	return name == "pg_dump" || name == "pg_restore"
}

func dockerPostgresToolArgs(name string, args []string) ([]string, string, string) {
	copied := append([]string(nil), args...)
	switch name {
	case "pg_dump":
		copied, outputFile := removeOptionValue(copied, "--file")
		return copied, outputFile, ""
	case "pg_restore":
		if len(copied) == 0 {
			return copied, "", ""
		}
		last := copied[len(copied)-1]
		if strings.HasPrefix(last, "-") {
			return copied, "", ""
		}
		return copied[:len(copied)-1], "", last
	default:
		return copied, "", ""
	}
}

func removeOptionValue(args []string, option string) ([]string, string) {
	copied := make([]string, 0, len(args))
	var value string
	for i := 0; i < len(args); i++ {
		if args[i] == option && i+1 < len(args) {
			value = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], option+"=") {
			value = strings.TrimPrefix(args[i], option+"=")
			continue
		}
		copied = append(copied, args[i])
	}
	return copied, value
}

func rewriteLocalDSNsForContainer(args []string) []string {
	rewritten := append([]string(nil), args...)
	for i, arg := range rewritten {
		rewritten[i] = rewriteLocalDSNForContainer(arg)
	}
	return rewritten
}

func rewriteLocalDSNForContainer(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" {
		return value
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return value
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return value
	}
	parsed.Host = net.JoinHostPort(host, "5432")
	return parsed.String()
}

func RedactOutput(value string) string {
	return sanitizeOutput(value)
}

func fileResult(path string, created time.Time) (BackupResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return BackupResult{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return BackupResult{}, err
	}
	return BackupResult{
		Path:       path,
		SizeBytes:  size,
		SHA256:     hex.EncodeToString(hash.Sum(nil)),
		CreatedUTC: created,
	}, nil
}

func databaseName(databaseURL string) string {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(parsed.Path), "/")
}

func sanitizeFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "exmarketing"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func sameDSN(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func sanitizeOutput(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = redactKeyValue(value)
	if len(value) > 1200 {
		return value[:1200]
	}
	return value
}

func redactKeyValue(value string) string {
	replacements := []string{"password", "Password", "token", "Token", "secret", "Secret", "Authorization"}
	for _, key := range replacements {
		value = redactAfter(value, key+"=")
		value = redactAfter(value, key+":")
	}
	return value
}

func redactAfter(value, marker string) string {
	searchFrom := 0
	for {
		idx := strings.Index(value[searchFrom:], marker)
		if idx < 0 {
			return value
		}
		idx += searchFrom
		start := idx + len(marker)
		end := start
		for end < len(value) && value[end] != ' ' && value[end] != '&' && value[end] != ';' {
			end++
		}
		value = value[:start] + "[redacted]" + value[end:]
		searchFrom = start + len("[redacted]")
		if searchFrom > len(value) {
			searchFrom = len(value)
		}
	}
}

func FormatCounts(counts []TableCount) string {
	var b strings.Builder
	for _, count := range counts {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(count.Table)
		b.WriteByte('=')
		b.WriteString(strconv.FormatInt(count.Rows, 10))
	}
	return b.String()
}
