package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/dbbackup"
	"github.com/ex-stack/ExMarketing/internal/dbmigration"
)

func main() {
	var (
		mode         = flag.String("mode", "validate", "backup, restore, validate, or smoke")
		databaseURL  = flag.String("database-url", "", "PostgreSQL DSN; defaults to EX_MARKETING_DATABASE_URL")
		restoreURL   = flag.String("restore-database-url", "", "restore target DSN; defaults to EX_MARKETING_RESTORE_DATABASE_URL")
		backupFile   = flag.String("backup-file", "", "backup file for restore")
		outputDir    = flag.String("output-dir", "backups/db-restore-drills", "backup output directory")
		migrationDir = flag.String("dir", "db/migration", "migration SQL directory")
		force        = flag.Bool("force", false, "allow restore into a non-empty or same local target")
		timeout      = flag.Duration("timeout", 2*time.Minute, "operation timeout")
		smokeCommand = flag.String("smoke-command", "go run ./cmd/engine-e2e-postgres", "command used by smoke mode")
	)
	flag.Parse()

	sourceDSN := firstNonEmpty(*databaseURL, os.Getenv("EX_MARKETING_DATABASE_URL"))
	targetDSN := firstNonEmpty(*restoreURL, os.Getenv("EX_MARKETING_RESTORE_DATABASE_URL"), sourceDSN)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "backup":
		result, err := dbbackup.Backup(ctx, dbbackup.ExecRunner{}, sourceDSN, *outputDir, time.Now().UTC())
		if err != nil {
			fail(err)
		}
		printJSON("BACKUP PASS", map[string]any{
			"path":       result.Path,
			"sizeBytes":  result.SizeBytes,
			"sha256":     result.SHA256,
			"createdUTC": result.CreatedUTC.Format(time.RFC3339),
			"source":     dbbackup.RedactDSN(sourceDSN),
		})
	case "restore":
		db, err := dbmigration.Open(ctx, targetDSN)
		if err != nil {
			fail(fmt.Errorf("RESTORE: connect target %s: %w", dbbackup.RedactDSN(targetDSN), err))
		}
		defer db.Close()
		if err := dbbackup.Restore(ctx, dbbackup.ExecRunner{}, db, sourceDSN, targetDSN, *backupFile, *force); err != nil {
			fail(err)
		}
		printJSON("RESTORE PASS", map[string]any{
			"backupFile": *backupFile,
			"target":     dbbackup.RedactDSN(targetDSN),
			"force":      *force,
		})
	case "validate":
		counts, err := validate(ctx, targetDSN, *migrationDir)
		if err != nil {
			fail(err)
		}
		printJSON("VALIDATE PASS", map[string]any{
			"database": dbbackup.RedactDSN(targetDSN),
			"counts":   counts,
		})
	case "smoke":
		if err := runSmoke(ctx, targetDSN, *smokeCommand); err != nil {
			fail(err)
		}
	default:
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
}

func validate(ctx context.Context, databaseURL, migrationDir string) ([]dbbackup.TableCount, error) {
	db, err := dbmigration.Open(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("SCHEMA_CHECK: connect %s: %w", dbbackup.RedactDSN(databaseURL), err)
	}
	defer db.Close()
	return dbbackup.Validate(ctx, db, migrationDir)
}

func runSmoke(ctx context.Context, databaseURL, commandLine string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return fmt.Errorf("EVENT_IN_SMOKE: restore database URL is required")
	}
	fields := strings.Fields(commandLine)
	if len(fields) == 0 {
		return fmt.Errorf("EVENT_IN_SMOKE: smoke command is required")
	}
	cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
	cmd.Env = append(os.Environ(), "EX_MARKETING_DATABASE_URL="+databaseURL)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("EVENT_IN_SMOKE: %s failed for %s: %w: %s", commandLine, dbbackup.RedactDSN(databaseURL), err, dbbackup.RedactOutput(string(output)))
	}
	fmt.Print(string(output))
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func printJSON(title string, value any) {
	fmt.Println(title)
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Printf("%v\n", value)
		return
	}
	fmt.Println(string(encoded))
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
	os.Exit(1)
}
