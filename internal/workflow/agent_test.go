package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromptForAgent(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}

	prompt, err := PromptForAgent(dir, AgentAnalysis)
	if err != nil {
		t.Fatalf("prompt for agent: %v", err)
	}
	if want := "你是 Analysis Agent"; len(prompt) < len(want) || prompt[:len(want)] != want {
		t.Fatalf("prompt = %q, want prefix %q", prompt, want)
	}
}

func TestEvaluateAgentResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte(`# Analysis

## Score

~~~text
85
~~~

## Decision

~~~text
PASS
~~~
`), 0o644); err != nil {
		t.Fatalf("write analysis: %v", err)
	}

	status, score, err := EvaluateAgentResult(dir, AgentAnalysis)
	if err != nil {
		t.Fatalf("evaluate analysis: %v", err)
	}
	if status != StatusReviewReady || score != 85 {
		t.Fatalf("status=%s score=%d, want %s 85", status, score, StatusReviewReady)
	}
}

func TestResolveAgentRoutesTestFailBackToDev(t *testing.T) {
	agent, err := ResolveAgent(StatusTestFail, AgentAuto)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agent != AgentDev {
		t.Fatalf("agent = %s, want %s", agent, AgentDev)
	}
}

func TestResolveAgentRoutesCommitFailBackToDev(t *testing.T) {
	agent, err := ResolveAgent(StatusCommitFail, AgentAuto)
	if err != nil {
		t.Fatalf("resolve agent: %v", err)
	}
	if agent != AgentDev {
		t.Fatalf("agent = %s, want %s", agent, AgentDev)
	}
}

func TestDevPromptIncludesTestFailureFeedback(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(`# Test

## Result

~~~text
FAIL
~~~

## Evidence

PostgreSQL smoke failed: duplicate inbox row.
`), 0o644); err != nil {
		t.Fatalf("write test: %v", err)
	}
	prompt, err := PromptForAgent(dir, AgentDev)
	if err != nil {
		t.Fatalf("prompt for dev: %v", err)
	}
	if !strings.Contains(prompt, "返工反馈") || !strings.Contains(prompt, "PostgreSQL smoke failed") {
		t.Fatalf("expected dev prompt to include test failure feedback, got: %s", prompt)
	}
}

func TestDevPromptIncludesCommitFailureFeedback(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	logPath := filepath.Join(dir, "runs", "commit.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("git diff --check failed\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	run := AgentRun{Agent: AgentCommit, OutputPath: logPath, ExitCode: 1}
	if err := MarkAgentFailed(req, run, os.ErrInvalid); err != nil {
		t.Fatalf("mark commit failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "commit.md"), []byte(`# Commit

## Review

~~~text
BLOCK
~~~

git diff --check failed.
`), 0o644); err != nil {
		t.Fatalf("write commit: %v", err)
	}
	prompt, err := PromptForAgent(dir, AgentDev)
	if err != nil {
		t.Fatalf("prompt for dev: %v", err)
	}
	if !strings.Contains(prompt, "Commit 失败") || !strings.Contains(prompt, "git diff --check failed") {
		t.Fatalf("expected dev prompt to include commit failure feedback, got: %s", prompt)
	}
}

func TestEnsureTaskFilesPreservesExistingAgentPrompts(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	original := "# Agent Prompts\n\ncustom prompt edits\n"
	if err := os.WriteFile(filepath.Join(dir, "agent-prompts.md"), []byte(original), 0o644); err != nil {
		t.Fatalf("write prompts: %v", err)
	}

	if err := EnsureTaskFiles(req); err != nil {
		t.Fatalf("ensure task files: %v", err)
	}

	bytes, err := os.ReadFile(filepath.Join(dir, "agent-prompts.md"))
	if err != nil {
		t.Fatalf("read prompts: %v", err)
	}
	got := string(bytes)
	if !strings.HasPrefix(got, strings.TrimRight(original, "\n")) {
		t.Fatalf("expected existing prompt content to be preserved, got: %s", got)
	}
	if !strings.Contains(got, "## Test Plan Agent") {
		t.Fatalf("expected missing test plan prompt to be appended, got: %s", got)
	}
}

func TestRunAgent(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	command := `cat >/dev/null && cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
90
~~~

## Decision

~~~text
PASS
~~~
EOF`
	run, err := RunAgent(req, AgentAnalysis, command)
	if err != nil {
		t.Fatalf("run agent: %v", err)
	}
	if run.NextStatus != StatusReviewReady || run.Score != 90 {
		t.Fatalf("next=%s score=%d, want %s 90", run.NextStatus, run.Score, StatusReviewReady)
	}
	if _, err := os.Stat(run.OutputPath); err != nil {
		t.Fatalf("output log missing: %v", err)
	}
	status, err := LoadAgentStatus(dir)
	if err != nil {
		t.Fatalf("load agent status: %v", err)
	}
	current := status.Agents[string(AgentAnalysis)]
	if current.Status != AgentStatusDone {
		t.Fatalf("agent status = %s, want %s", current.Status, AgentStatusDone)
	}
	if current.Log != run.OutputPath {
		t.Fatalf("agent log = %s, want %s", current.Log, run.OutputPath)
	}
}

func TestRunAgentMarksFailed(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}

	run, err := RunAgent(req, AgentAnalysis, "exit 7")
	if err == nil {
		t.Fatal("run agent succeeded, want failure")
	}
	status, loadErr := LoadAgentStatus(dir)
	if loadErr != nil {
		t.Fatalf("load agent status: %v", loadErr)
	}
	current := status.Agents[string(AgentAnalysis)]
	if current.Status != AgentStatusFailed {
		t.Fatalf("agent status = %s, want %s", current.Status, AgentStatusFailed)
	}
	if current.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", current.ExitCode)
	}
	if current.Log != run.OutputPath {
		t.Fatalf("agent log = %s, want %s", current.Log, run.OutputPath)
	}
}

func TestEnsureAgentStatusMarksStaleRunningInterrupted(t *testing.T) {
	t.Setenv("EX_MARKETING_STALE_AGENT_SECONDS", "1")
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	logPath := filepath.Join(dir, "runs", "old-dev.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("startedAt=old\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(logPath, old, old); err != nil {
		t.Fatalf("chtimes log: %v", err)
	}
	if err := MarkAgentRunning(req, AgentDev, logPath); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if err := os.Chtimes(logPath, old, old); err != nil {
		t.Fatalf("restore log mtime: %v", err)
	}

	status, err := EnsureAgentStatus(req)
	if err != nil {
		t.Fatalf("ensure status: %v", err)
	}
	current := status.Agents[string(AgentDev)]
	if current.Status != AgentStatusStale {
		t.Fatalf("agent status = %s, want %s", current.Status, AgentStatusStale)
	}
	if current.Error == "" {
		t.Fatal("expected interrupted error")
	}
}
