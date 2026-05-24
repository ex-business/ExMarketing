package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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
	if !strings.Contains(prompt, "只负责需求分析和需求细化") || !strings.Contains(prompt, "不写代码") {
		t.Fatalf("analysis prompt should forbid coding, got: %s", prompt)
	}
}

func TestTailForPromptKeepsValidUTF8(t *testing.T) {
	content := strings.Repeat("说明", 7000)
	got := tailForPrompt(content, 12001)
	if !utf8.ValidString(got) {
		t.Fatalf("tailForPrompt returned invalid UTF-8")
	}
	if len(got) > 12001 {
		t.Fatalf("tail length = %d, want <= 12001", len(got))
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

func TestEvaluateAgentResultUsesTotalScoreInMarkdownFence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte(`# Analysis

## Score

`+"```text"+`
需求清晰度        19/20
需求边界清楚      18/20
风险可控          17/20
验收标准明确      19/20
约束一致性        19/20
总分              92/100
`+"```"+`

## Decision

`+"```text"+`
PASS
`+"```"+`
`), 0o644); err != nil {
		t.Fatalf("write analysis: %v", err)
	}

	status, score, err := EvaluateAgentResult(dir, AgentAnalysis)
	if err != nil {
		t.Fatalf("evaluate analysis: %v", err)
	}
	if status != StatusReviewReady || score != 92 {
		t.Fatalf("status=%s score=%d, want %s 92", status, score, StatusReviewReady)
	}
}

func TestEvaluateReviewResultUsesFirstDecisionFence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(`# Review

## Decision

~~~text
BLOCK
~~~

规则：

~~~text
无 P0/P1 且 Score >= 80 -> PASS
~~~
`), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}

	status, _, err := EvaluateAgentResult(dir, AgentReview)
	if err != nil {
		t.Fatalf("evaluate review: %v", err)
	}
	if status != StatusReviewBlock {
		t.Fatalf("status=%s, want %s", status, StatusReviewBlock)
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

func TestResolveAgentRoutesAnalysisFailureBackToAnalysis(t *testing.T) {
	for _, status := range []string{StatusAnalysisFail, StatusReviewBlock} {
		agent, err := ResolveAgent(status, AgentAuto)
		if err != nil {
			t.Fatalf("resolve agent for %s: %v", status, err)
		}
		if agent != AgentAnalysis {
			t.Fatalf("status %s resolved to %s, want %s", status, agent, AgentAnalysis)
		}
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

func TestDevPromptIncludesAnalysisFailureFeedback(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "analysis.md"), []byte(`# Analysis

## Score

~~~text
72
~~~

## Decision

~~~text
BLOCK
~~~

缺少边界条件。
`), 0o644); err != nil {
		t.Fatalf("write analysis: %v", err)
	}
	prompt, err := PromptForAgent(dir, AgentAnalysis)
	if err != nil {
		t.Fatalf("prompt for analysis: %v", err)
	}
	if !strings.Contains(prompt, "分析返工反馈") || !strings.Contains(prompt, "缺少边界条件") {
		t.Fatalf("expected analysis prompt to include failure feedback, got: %s", prompt)
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

func TestResetDevAndTestAfterCommitFailureKeepsCommitContext(t *testing.T) {
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := MarkAgentDone(req, AgentRun{Agent: AgentDev, OutputPath: filepath.Join(dir, "runs", "dev.log"), NextStatus: StatusReviewPass}); err != nil {
		t.Fatalf("mark dev done: %v", err)
	}
	if err := MarkAgentDone(req, AgentRun{Agent: AgentTest, OutputPath: filepath.Join(dir, "runs", "test.log"), NextStatus: StatusCommitReady}); err != nil {
		t.Fatalf("mark test done: %v", err)
	}
	commitLog := filepath.Join(dir, "runs", "commit.log")
	if err := os.MkdirAll(filepath.Dir(commitLog), 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.WriteFile(commitLog, []byte("git diff --check failed\n"), 0o644); err != nil {
		t.Fatalf("write commit log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "commit.md"), []byte(`# Commit

## Review

~~~text
BLOCK
~~~

git diff --check failed.
`), 0o644); err != nil {
		t.Fatalf("write commit artifact: %v", err)
	}
	if err := MarkAgentFailed(req, AgentRun{Agent: AgentCommit, OutputPath: commitLog, ExitCode: 1}, os.ErrInvalid); err != nil {
		t.Fatalf("mark commit failed: %v", err)
	}

	if err := ResetDevAndTestAfterCommitFailure(req); err != nil {
		t.Fatalf("reset dev/test: %v", err)
	}

	status, err := LoadAgentStatus(dir)
	if err != nil {
		t.Fatalf("load status: %v", err)
	}
	if got := status.Agents[string(AgentDev)].Status; got != AgentStatusWaiting {
		t.Fatalf("dev status = %s, want %s", got, AgentStatusWaiting)
	}
	if got := status.Agents[string(AgentTest)].Status; got != AgentStatusWaiting {
		t.Fatalf("test status = %s, want %s", got, AgentStatusWaiting)
	}
	if got := status.Agents[string(AgentCommit)].Status; got != AgentStatusFailed {
		t.Fatalf("commit status = %s, want %s", got, AgentStatusFailed)
	}
	prompt, err := PromptForAgent(dir, AgentDev)
	if err != nil {
		t.Fatalf("prompt for dev: %v", err)
	}
	if !strings.Contains(prompt, "Commit 失败") || !strings.Contains(prompt, "git diff --check failed") {
		t.Fatalf("expected dev prompt to keep commit failure context, got: %s", prompt)
	}
}

func TestReadOnlyWriteGuardIgnoresWorkflowRuntimeFiles(t *testing.T) {
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
		t.Fatalf("run analysis should ignore agent-status runtime writes: %v", err)
	}
	if run.NextStatus != StatusReviewReady {
		t.Fatalf("next status = %s, want %s", run.NextStatus, StatusReviewReady)
	}
}

func TestEnsureTaskFilesRefreshesCanonicalAgentPrompts(t *testing.T) {
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
	if !strings.Contains(got, "## Test Plan Agent") {
		t.Fatalf("expected missing test plan prompt to be appended, got: %s", got)
	}
	if strings.Contains(got, "custom prompt edits") {
		t.Fatalf("expected stale prompt edits to be replaced, got: %s", got)
	}
	if !strings.Contains(got, "只负责需求分析和需求细化") || !strings.Contains(got, "不写代码") {
		t.Fatalf("expected canonical analysis prompt, got: %s", got)
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
	if err := MarkAgentRunning(req, AgentDev, logPath, 0); err != nil {
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

func TestEnsureAgentStatusMarksMissingProcessInterrupted(t *testing.T) {
	t.Setenv("EX_MARKETING_STALE_AGENT_SECONDS", "300")
	dir := t.TempDir()
	req := Requirement{
		ID:      "req-1",
		Title:   "Title",
		TaskDir: dir,
	}
	if err := CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	logPath := filepath.Join(dir, "runs", "missing-process.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir runs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("startedAt=now\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := MarkAgentRunning(req, AgentAnalysis, logPath, 999999); err != nil {
		t.Fatalf("mark running: %v", err)
	}

	status, err := EnsureAgentStatus(req)
	if err != nil {
		t.Fatalf("ensure status: %v", err)
	}
	current := status.Agents[string(AgentAnalysis)]
	if current.Status != AgentStatusStale {
		t.Fatalf("agent status = %s, want %s", current.Status, AgentStatusStale)
	}
	if !strings.Contains(current.Error, "process 999999 is not running") {
		t.Fatalf("unexpected error: %s", current.Error)
	}
}
