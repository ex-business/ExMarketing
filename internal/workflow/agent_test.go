package workflow

import (
	"os"
	"path/filepath"
	"testing"
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
