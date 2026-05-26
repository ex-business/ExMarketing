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

type writeGuardSnapshot struct {
	Enabled bool
	Files   map[string]bool
}

func ResolveAgent(status string, requested Agent) (Agent, error) {
	if requested != "" && requested != AgentAuto {
		return requested, nil
	}
	switch status {
	case StatusAnalysisReady, StatusAnalysisFail, StatusReviewBlock:
		return AgentAnalysis, nil
	case StatusReviewReady:
		return AgentReview, nil
	case StatusReviewPass:
		return AgentTestPlan, nil
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
	if agent == AgentAnalysis {
		prompt += renderAnalysisFeedbackContext(taskDir)
	} else if agent == AgentDev {
		prompt += renderDevFeedbackContext(taskDir)
	}
	return strings.ToValidUTF8(prompt, ""), nil
}

func renderAnalysisFeedbackContext(taskDir string) string {
	analysisContent, _ := os.ReadFile(filepath.Join(taskDir, "analysis.md"))
	reviewContent, _ := os.ReadFile(filepath.Join(taskDir, "review.md"))
	status, _ := LoadAgentStatus(taskDir)
	analysisStatus := status.Agents[string(AgentAnalysis)]
	reviewStatus := status.Agents[string(AgentReview)]
	score := parseScore(string(analysisContent))
	hasAnalysisFailure := score > 0 && score < AnalysisPassScore() || parseDecision(string(analysisContent)) == "BLOCK" || analysisStatus.Status == AgentStatusFailed || analysisStatus.Error != ""
	hasReviewFailure := parseDecision(string(reviewContent)) == "BLOCK" || reviewStatus.Status == AgentStatusFailed || reviewStatus.Error != ""
	if !hasAnalysisFailure && !hasReviewFailure {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## 分析返工反馈\n\n")
	b.WriteString("上一轮分析或审核未通过。必须先读取并逐条处理 Review Agent 在 review.md 中的打回意见；如果 analysis.md 自己上一轮 Decision BLOCK、Score < 80 或记录了失败，也必须读取并逐条处理 Analysis 自己写出的 BLOCK 原因、扣分原因、待确认问题和错误假设；然后必须重新读取 task.md。\n\n")
	b.WriteString("返工时必须基于拒绝原因补充 analysis.md 的需求细节、边界条件、输入输出、验收标准和非目标范围。不能等待 Owner 决策，不能只列出“需补充决策”后停止；如果现有材料没有唯一答案，必须给出默认最小可验收口径，写清推导依据和默认约束，并用该口径关闭旧 BLOCK。\n\n")
	b.WriteString("返工输出必须能回应 Review 的 P0/P1/P2、Open Questions 和 BLOCK 原因，也必须回应 Analysis 自己上一轮的 BLOCK 原因和扣分项；不能只重写需求而忽略审核意见或自我分析失败原因。只有 Score >= 80 且 Decision PASS 才能继续交给 Review Agent。\n\n")
	b.WriteString("注意：Analysis Agent 只完善需求细节，不读代码、不写代码、不改代码、不提交；如果历史 analysis.md 中出现实现指令，以本次 prompt 为准。\n\n")
	appendAgentFailureFeedback(&b, "Review", "review.md", string(reviewContent), reviewStatus)
	appendAgentFailureFeedback(&b, "Analysis", "analysis.md", string(analysisContent), analysisStatus)
	return b.String()
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
		!(parseScore(artifactContent) > 0 && parseScore(artifactContent) < AnalysisPassScore()) &&
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
	content = strings.ToValidUTF8(content, "")
	if len(content) <= maxBytes {
		return content
	}
	runes := []rune(content)
	totalBytes := 0
	start := len(runes)
	for start > 0 {
		nextBytes := len(string(runes[start-1]))
		if totalBytes+nextBytes > maxBytes {
			break
		}
		totalBytes += nextBytes
		start--
	}
	return string(runes[start:])
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
	writeGuard, err := newWriteGuardSnapshot(agent)
	if err != nil {
		return AgentRun{}, err
	}

	runDir := filepath.Join(req.TaskDir, "runs")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return AgentRun{}, err
	}
	outputPath := filepath.Join(runDir, time.Now().UTC().Format("20060102T150405Z")+"-"+string(agent)+".log")
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
	err = cmd.Start()
	if err != nil {
		run := AgentRun{
			ID:         req.ID,
			Agent:      agent,
			Command:    command,
			Prompt:     prompt,
			OutputPath: outputPath,
			ExitCode:   1,
		}
		_, _ = fmt.Fprintf(logFile, "\nfinishedAt=%s exitCode=%d error=%v\n", time.Now().UTC().Format(time.RFC3339), 1, err)
		if statusErr := MarkAgentFailed(req, run, err); statusErr != nil {
			return run, statusErr
		}
		return run, err
	}
	if err := MarkAgentRunning(req, agent, outputPath, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return AgentRun{}, err
	}
	err = cmd.Wait()

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

	if err := validateReadOnlyAgentWrites(req, agent, writeGuard, logFile); err != nil {
		run := AgentRun{
			ID:         req.ID,
			Agent:      agent,
			Command:    command,
			Prompt:     prompt,
			OutputPath: outputPath,
			ExitCode:   exitCode,
		}
		_, _ = fmt.Fprintf(logFile, "\nwriteGuard=%v\n", err)
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

func newWriteGuardSnapshot(agent Agent) (writeGuardSnapshot, error) {
	if !readOnlyAgent(agent) {
		return writeGuardSnapshot{}, nil
	}
	files, err := gitChangedFiles()
	if err != nil {
		return writeGuardSnapshot{}, err
	}
	return writeGuardSnapshot{Enabled: true, Files: files}, nil
}

func validateReadOnlyAgentWrites(req Requirement, agent Agent, snapshot writeGuardSnapshot, logFile *os.File) error {
	if !snapshot.Enabled {
		return nil
	}
	if agent == AgentTestPlan && hasMutatingAgentRunning(req) {
		_, _ = fmt.Fprintln(logFile, "\nwriteGuard=skipped because Dev/Test/Commit is running in parallel")
		return nil
	}
	after, err := gitChangedFiles()
	if err != nil {
		return err
	}
	allowed := filepath.ToSlash(filepath.Clean(filepath.Join(req.TaskDir, ArtifactForAgent(agent))))
	var violations []string
	for file := range after {
		if isOtherTaskDirFile(req, file) {
			continue
		}
		if snapshot.Files[file] || file == allowed || isWorkflowRuntimeFile(req, file) {
			continue
		}
		violations = append(violations, file)
	}
	if len(violations) == 0 {
		return nil
	}
	return fmt.Errorf("%s agent is read-only and may only write %s; unexpected changed files: %s", agent, allowed, strings.Join(violations, ", "))
}

func isOtherTaskDirFile(req Requirement, file string) bool {
	file = filepath.ToSlash(filepath.Clean(file))
	taskDir := filepath.ToSlash(filepath.Clean(req.TaskDir))
	return strings.HasPrefix(file, "tasks/") && !strings.HasPrefix(file, taskDir+"/")
}

func isWorkflowRuntimeFile(req Requirement, file string) bool {
	taskDir := filepath.ToSlash(filepath.Clean(req.TaskDir))
	switch file {
	case filepath.ToSlash(filepath.Clean(filepath.Join(req.TaskDir, AgentStatusFileName))),
		filepath.ToSlash(filepath.Clean(filepath.Join(req.TaskDir, AgentReportFileName))):
		return true
	}
	return strings.HasPrefix(file, taskDir+"/runs/")
}

func readOnlyAgent(agent Agent) bool {
	return agent == AgentAnalysis || agent == AgentReview || agent == AgentTestPlan || agent == AgentTest || agent == AgentCommit
}

func hasMutatingAgentRunning(req Requirement) bool {
	status, err := LoadAgentStatus(req.TaskDir)
	if err != nil {
		return false
	}
	for _, agent := range []Agent{AgentDev, AgentTest, AgentCommit} {
		if status.Agents[string(agent)].Status == AgentStatusRunning {
			return true
		}
	}
	return false
}

func gitChangedFiles() (map[string]bool, error) {
	files := map[string]bool{}
	if _, err := exec.Command("git", "rev-parse", "--is-inside-work-tree").Output(); err != nil {
		return files, nil
	}
	commands := [][]string{
		{"git", "diff", "--name-only", "--"},
		{"git", "diff", "--cached", "--name-only", "--"},
		{"git", "ls-files", "--others", "--exclude-standard"},
	}
	for _, args := range commands {
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			return nil, fmt.Errorf("read git changed files: %w", err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			files[filepath.ToSlash(filepath.Clean(line))] = true
		}
	}
	return files, nil
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
		return StatusAnalysisFail, score, nil
	case AgentReview:
		content, err := readTaskFile(taskDir, "review.md")
		if err != nil {
			return "", 0, err
		}
		if parseDecision(content) == "PASS" {
			return StatusReviewPass, -1, nil
		}
		return StatusReviewBlock, -1, nil
	case AgentTestPlan:
		content, err := readTaskFile(taskDir, "test-plan.md")
		if err != nil {
			return "", 0, err
		}
		if parseDecision(content) == "PASS" {
			return StatusReviewPass, -1, nil
		}
		return StatusTestFail, -1, nil
	case AgentDev:
		return StatusReviewPass, -1, nil
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
	totalRe := regexp.MustCompile(`\b([0-9]{1,3})\s*/\s*100\b`)
	totalMatches := totalRe.FindAllStringSubmatch(section, -1)
	if len(totalMatches) > 0 {
		score, err := strconv.Atoi(totalMatches[len(totalMatches)-1][1])
		if err == nil {
			return clampScore(score)
		}
	}
	re := regexp.MustCompile(`\b([0-9]{1,3})\b`)
	match := re.FindStringSubmatch(section)
	if len(match) != 2 {
		return 0
	}
	score, err := strconv.Atoi(match[1])
	if err != nil || score < 0 {
		return 0
	}
	return clampScore(score)
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func parseTokenAfterHeading(content, heading string, allowed []string) string {
	section := sectionAfterHeading(content, heading)
	if fenced := firstFenceContent(section); fenced != "" {
		if token := firstAllowedToken(fenced, allowed); token != "" {
			return token
		}
	}
	return firstAllowedToken(section, allowed)
}

func firstAllowedToken(content string, allowed []string) string {
	section := strings.ToUpper(content)
	for _, token := range allowed {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(token) + `\b`).MatchString(section) {
			return token
		}
	}
	return ""
}

func firstFenceContent(content string) string {
	re := regexp.MustCompile("(?ms)(?:~~~|```)[a-zA-Z0-9_-]*\\s*\\n(.*?)\\n(?:~~~|```)")
	match := re.FindStringSubmatch(content)
	if len(match) != 2 {
		return ""
	}
	return match[1]
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
