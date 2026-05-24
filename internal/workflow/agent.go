package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Agent string

const (
	AgentAuto     Agent = "auto"
	AgentAnalysis Agent = "analysis"
	AgentReview   Agent = "review"
	AgentTestPlan Agent = "test-plan"
	AgentDev      Agent = "dev"
	AgentTest     Agent = "test"
	AgentCommit   Agent = "commit"
)

type AgentRun struct {
	ID         string
	Agent      Agent
	Command    string
	Prompt     string
	OutputPath string
	NextStatus string
	Score      int
	ExitCode   int
}

func ResolveAgent(status string, requested Agent) (Agent, error) {
	if requested != "" && requested != AgentAuto {
		return requested, nil
	}
	switch status {
	case StatusAnalysisReady:
		return AgentAnalysis, nil
	case StatusReviewReady:
		return AgentReview, nil
	case StatusTestPlanReady:
		return AgentTestPlan, nil
	case StatusTestPlanDone:
		return AgentDev, nil
	case StatusDevReady:
		return AgentDev, nil
	case StatusTestExecReady, StatusTestReady:
		return AgentTest, nil
	case StatusTestFail, StatusCommitFail:
		return AgentDev, nil
	case StatusCommitReady:
		return AgentCommit, nil
	default:
		return "", fmt.Errorf("status %s has no runnable agent", status)
	}
}

func PromptForAgent(taskDir string, agent Agent) (string, error) {
	bytes, err := os.ReadFile(filepath.Join(taskDir, "agent-prompts.md"))
	if err != nil {
		return "", err
	}
	heading := map[Agent]string{
		AgentAnalysis: "Analysis Agent",
		AgentReview:   "Review Agent",
		AgentTestPlan: "Test Plan Agent",
		AgentDev:      "Dev Agent",
		AgentTest:     "Test Agent",
		AgentCommit:   "Commit Agent",
	}[agent]
	if heading == "" {
		return "", fmt.Errorf("unknown agent %s", agent)
	}
	pattern := regexp.MustCompile(`(?ms)^## ` + regexp.QuoteMeta(heading) + `\s+~~~text\s+(.*?)\s+~~~`)
	match := pattern.FindSubmatch(bytes)
	if len(match) != 2 {
		return "", fmt.Errorf("prompt for %s not found in %s", heading, filepath.Join(taskDir, "agent-prompts.md"))
	}
	prompt := strings.TrimSpace(string(match[1])) + "\n"
	if agent == AgentDev {
		prompt += renderDevFeedbackContext(taskDir)
	}
	return prompt, nil
}

func renderDevFeedbackContext(taskDir string) string {
	testContent, _ := os.ReadFile(filepath.Join(taskDir, "test.md"))
	commitContent, _ := os.ReadFile(filepath.Join(taskDir, "commit.md"))
	status, _ := LoadAgentStatus(taskDir)
	testStatus := status.Agents[string(AgentTest)]
	commitStatus := status.Agents[string(AgentCommit)]
	hasTestFailure := strings.Contains(strings.ToUpper(string(testContent)), "FAIL") || testStatus.Status == AgentStatusFailed || testStatus.Error != ""
	hasCommitFailure := strings.Contains(strings.ToUpper(string(commitContent)), "BLOCK") || commitStatus.Status == AgentStatusFailed || commitStatus.Error != ""
	if !hasTestFailure && !hasCommitFailure {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## 返工反馈\n\n")
	b.WriteString("请根据下面失败反馈做最小修复，然后必须重新运行 go test ./... 和 go build ./...。修复完成后继续交给 Test Agent 验证。\n\n")
	appendAgentFailureFeedback(&b, "Test", "test.md", string(testContent), testStatus)
	appendAgentFailureFeedback(&b, "Commit", "commit.md", string(commitContent), commitStatus)
	return b.String()
}

func appendAgentFailureFeedback(b *strings.Builder, label, artifactName, artifactContent string, status AgentStatus) {
	if !strings.Contains(strings.ToUpper(artifactContent), "FAIL") &&
		!strings.Contains(strings.ToUpper(artifactContent), "BLOCK") &&
		status.Status != AgentStatusFailed &&
		status.Error == "" {
		return
	}
	b.WriteString("### ")
	b.WriteString(label)
	b.WriteString(" 失败\n\n")
	if status.Log != "" {
		b.WriteString("最近日志：")
		b.WriteString(status.Log)
		b.WriteString("\n")
	}
	if status.Error != "" {
		b.WriteString("错误：")
		b.WriteString(status.Error)
		b.WriteString("\n")
	}
	if artifactContent != "" {
		b.WriteString("\n#### ")
		b.WriteString(artifactName)
		b.WriteString(" 摘要\n\n~~~markdown\n")
		b.WriteString(tailForPrompt(artifactContent, 12000))
		b.WriteString("\n~~~\n")
	}
	if status.Log != "" {
		if bytes, err := os.ReadFile(status.Log); err == nil && len(bytes) > 0 {
			b.WriteString("\n#### 日志摘要\n\n~~~text\n")
			b.WriteString(tailForPrompt(string(bytes), 12000))
			b.WriteString("\n~~~\n")
		}
	}
	b.WriteString("\n")
}

func tailForPrompt(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	return content[len(content)-maxBytes:]
}

func RunAgent(req Requirement, agent Agent, command string) (AgentRun, error) {
	if req.ID == "" || req.TaskDir == "" {
		return AgentRun{}, fmt.Errorf("requirement id and taskDir are required")
	}
	if strings.TrimSpace(command) == "" {
		return AgentRun{}, fmt.Errorf("agent command is required; set EX_MARKETING_AGENT_CMD or pass --cmd")
	}
	if err := ensureAgentCanRun(req, agent); err != nil {
		return AgentRun{}, err
	}
	prompt, err := PromptForAgent(req.TaskDir, agent)
	if err != nil {
		return AgentRun{}, err
	}

	runDir := filepath.Join(req.TaskDir, "runs")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return AgentRun{}, err
	}
	outputPath := filepath.Join(runDir, time.Now().UTC().Format("20060102T150405Z")+"-"+string(agent)+".log")
	if err := MarkAgentRunning(req, agent, outputPath); err != nil {
		return AgentRun{}, err
	}
	logFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return AgentRun{}, err
	}
	defer logFile.Close()
	_, _ = fmt.Fprintf(logFile, "startedAt=%s agent=%s requirement=%s\n\n", time.Now().UTC().Format(time.RFC3339), agent, req.ID)

	ctx, cancel := context.WithTimeout(context.Background(), agentTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Env = append(os.Environ(),
		"EX_MARKETING_AGENT="+string(agent),
		"EX_MARKETING_REQUIREMENT_ID="+req.ID,
		"EX_MARKETING_TASK_DIR="+req.TaskDir,
		"EX_MARKETING_AGENT_PROMPT="+prompt,
	)
	cmd.Stdout = io.MultiWriter(logFile)
	cmd.Stderr = io.MultiWriter(logFile)
	err = cmd.Run()

	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
		exitCode = 124
	}
	_, _ = fmt.Fprintf(logFile, "\nfinishedAt=%s exitCode=%d error=%v\n", time.Now().UTC().Format(time.RFC3339), exitCode, err)
	if err != nil {
		run := AgentRun{
			ID:         req.ID,
			Agent:      agent,
			Command:    command,
			Prompt:     prompt,
			OutputPath: outputPath,
			ExitCode:   exitCode,
		}
		if statusErr := MarkAgentFailed(req, run, err); statusErr != nil {
			return run, statusErr
		}
		return run, err
	}

	nextStatus, score, err := EvaluateAgentResult(req.TaskDir, agent)
	if err != nil {
		run := AgentRun{
			ID:         req.ID,
			Agent:      agent,
			Command:    command,
			Prompt:     prompt,
			OutputPath: outputPath,
			ExitCode:   exitCode,
		}
		if statusErr := MarkAgentFailed(req, run, err); statusErr != nil {
			return run, statusErr
		}
		return run, err
	}
	run := AgentRun{
		ID:         req.ID,
		Agent:      agent,
		Command:    command,
		Prompt:     prompt,
		OutputPath: outputPath,
		NextStatus: nextStatus,
		Score:      score,
		ExitCode:   exitCode,
	}
	if err := MarkAgentDone(req, run); err != nil {
		return AgentRun{}, err
	}
	return run, nil
}

func ensureAgentCanRun(req Requirement, agent Agent) error {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(agent)]
	if current.Status == AgentStatusRunning {
		return fmt.Errorf("agent %s is already running", agent)
	}
	return nil
}

func agentTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("EX_MARKETING_AGENT_TIMEOUT_SECONDS"))
	if value == "" {
		return 15 * time.Minute
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 15 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func EvaluateAgentResult(taskDir string, agent Agent) (string, int, error) {
	switch agent {
	case AgentAnalysis:
		content, err := readTaskFile(taskDir, "analysis.md")
		if err != nil {
			return "", 0, err
		}
		score := parseScore(content)
		if score >= AnalysisPassScore() && parseDecision(content) == "PASS" {
			return StatusReviewReady, score, nil
		}
		return StatusReviewBlock, score, nil
	case AgentReview:
		content, err := readTaskFile(taskDir, "review.md")
		if err != nil {
			return "", 0, err
		}
		if parseDecision(content) == "PASS" {
			return StatusTestPlanReady, -1, nil
		}
		return StatusReviewBlock, -1, nil
	case AgentTestPlan:
		content, err := readTaskFile(taskDir, "test-plan.md")
		if err != nil {
			return "", 0, err
		}
		if parseDecision(content) == "PASS" {
			return StatusDevReady, -1, nil
		}
		return StatusTestFail, -1, nil
	case AgentDev:
		return StatusTestExecReady, -1, nil
	case AgentTest:
		content, err := readTaskFile(taskDir, "test.md")
		if err != nil {
			return "", 0, err
		}
		if parseResult(content) == "PASS" {
			return StatusCommitReady, -1, nil
		}
		return StatusTestFail, -1, nil
	case AgentCommit:
		return StatusCommitted, -1, nil
	default:
		return "", 0, fmt.Errorf("unknown agent %s", agent)
	}
}

func AnalysisPassScore() int {
	value := strings.TrimSpace(os.Getenv("EX_MARKETING_ANALYSIS_PASS_SCORE"))
	if value == "" {
		return 80
	}
	score, err := strconv.Atoi(value)
	if err != nil {
		return 80
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func readTaskFile(taskDir, name string) (string, error) {
	bytes, err := os.ReadFile(filepath.Join(taskDir, name))
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func parseDecision(content string) string {
	return parseTokenAfterHeading(content, "Decision", []string{"PASS", "BLOCK"})
}

func parseResult(content string) string {
	return parseTokenAfterHeading(content, "Result", []string{"PASS", "FAIL"})
}

func parseScore(content string) int {
	section := sectionAfterHeading(content, "Score")
	re := regexp.MustCompile(`\b([0-9]{1,3})\b`)
	match := re.FindStringSubmatch(section)
	if len(match) != 2 {
		return 0
	}
	score, err := strconv.Atoi(match[1])
	if err != nil || score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func parseTokenAfterHeading(content, heading string, allowed []string) string {
	section := strings.ToUpper(sectionAfterHeading(content, heading))
	for _, token := range allowed {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(token) + `\b`).MatchString(section) {
			return token
		}
	}
	return ""
}

func sectionAfterHeading(content, heading string) string {
	re := regexp.MustCompile(`(?im)^##\s+` + regexp.QuoteMeta(heading) + `\s*$`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		return content
	}
	rest := content[loc[1]:]
	next := regexp.MustCompile(`(?m)^##\s+`).FindStringIndex(rest)
	if next == nil {
		return rest
	}
	return rest[:next[0]]
}
