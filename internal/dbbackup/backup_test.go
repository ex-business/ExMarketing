package dbbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	name string
	args []string
	env  []string
	err  error
	out  string
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, env []string) ([]byte, error) {
	f.name = name
	f.args = append([]string(nil), args...)
	f.env = append([]string(nil), env...)
	for i, arg := range args {
		if arg == "--file" && i+1 < len(args) {
			if err := os.WriteFile(args[i+1], []byte("backup"), 0o600); err != nil {
				return nil, err
			}
		}
	}
	return []byte(f.out), f.err
}

func TestBackupCreatesUniqueCustomDumpPath(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	result, err := Backup(context.Background(), runner, "postgres://user:pass@localhost:5432/exmarketing?sslmode=disable", dir, time.Date(2026, 5, 27, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if runner.name != "pg_dump" {
		t.Fatalf("expected pg_dump, got %s", runner.name)
	}
	if strings.Contains(strings.Join(runner.args, " "), "pass") {
		t.Fatalf("pg_dump args leaked password: %v", runner.args)
	}
	if got := strings.Join(runner.env, " "); !strings.Contains(got, "PGPASSWORD=pass") {
		t.Fatalf("expected password in env, got %v", runner.env)
	}
	if !strings.HasSuffix(result.Path, filepath.Join(dir, "exmarketing_20260527T010203Z.dump")) {
		t.Fatalf("unexpected path %s", result.Path)
	}
	if result.SizeBytes != int64(len("backup")) || result.SHA256 == "" {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestBackupRemovesFailedDumpAndRedactsDSN(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{err: errors.New("exit 1"), out: "password=secret token=abc"}
	_, err := Backup(context.Background(), runner, "postgres://user:secret@localhost:5432/exmarketing?sslmode=disable", dir, time.Date(2026, 5, 27, 1, 2, 3, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token=abc") {
		t.Fatalf("error leaked secret: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "exmarketing_20260527T010203Z.dump")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed dump should be removed, stat=%v", statErr)
	}
}

func TestRestoreRejectsSameTargetWithoutForce(t *testing.T) {
	file := filepath.Join(t.TempDir(), "backup.dump")
	if err := os.WriteFile(file, []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Restore(context.Background(), &fakeRunner{}, nil, "postgres://u:p@localhost/db", "postgres://u:p@localhost/db", file, false)
	if err == nil || !strings.Contains(err.Error(), "source and restore database are the same") {
		t.Fatalf("expected same DSN error, got %v", err)
	}
}

func TestRedactDSN(t *testing.T) {
	redacted := RedactDSN("postgres://user:secret@localhost:5432/exmarketing?sslmode=disable&token=abc")
	if strings.Contains(redacted, "secret") || strings.Contains(redacted, "abc") {
		t.Fatalf("dsn was not redacted: %s", redacted)
	}
	if !strings.Contains(redacted, "user") {
		t.Fatalf("expected username to remain visible: %s", redacted)
	}
}

func TestDockerPostgresToolArgsDumpStreamsToHostFile(t *testing.T) {
	args, outputFile, stdinFile := dockerPostgresToolArgs("pg_dump", []string{
		"--format=custom",
		"--file", "/tmp/exmarketing.dump",
		"postgres://user@localhost:54329/exmarketing?sslmode=disable",
	})
	if outputFile != "/tmp/exmarketing.dump" {
		t.Fatalf("expected output file to be extracted, got %q", outputFile)
	}
	if stdinFile != "" {
		t.Fatalf("pg_dump should not use stdin file, got %q", stdinFile)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--file") || strings.Contains(joined, "/tmp/exmarketing.dump") {
		t.Fatalf("pg_dump fallback should stream stdout instead of writing in container: %v", args)
	}
}

func TestDockerPostgresToolArgsRestoreReadsHostFileFromStdin(t *testing.T) {
	args, outputFile, stdinFile := dockerPostgresToolArgs("pg_restore", []string{
		"--clean",
		"--dbname", "postgres://user@localhost:54329/exmarketing_restore?sslmode=disable",
		"/tmp/exmarketing.dump",
	})
	if stdinFile != "/tmp/exmarketing.dump" {
		t.Fatalf("expected backup file to be extracted for stdin, got %q", stdinFile)
	}
	if outputFile != "" {
		t.Fatalf("pg_restore should not use output file, got %q", outputFile)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "/tmp/exmarketing.dump") {
		t.Fatalf("pg_restore fallback should read host file through stdin: %v", args)
	}
}

func TestRewriteLocalDSNForContainer(t *testing.T) {
	got := rewriteLocalDSNForContainer("postgres://user@localhost:54329/exmarketing?sslmode=disable")
	if !strings.Contains(got, "localhost:5432") {
		t.Fatalf("expected local host port to be rewritten for container, got %s", got)
	}
	remote := "postgres://user@db.example.com:54329/exmarketing?sslmode=disable"
	if got := rewriteLocalDSNForContainer(remote); got != remote {
		t.Fatalf("remote DSN should not be rewritten, got %s", got)
	}
}
