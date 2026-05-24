package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const AgentReportFileName = "agent-report.md"

func WriteAgentReport(req Requirement, runs []AgentRun, finalStatus string, message string) error {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Report\n\n")
	fmt.Fprintf(&b, "需求 ID: %s\n\n", req.ID)
	fmt.Fprintf(&b, "标题: %s\n\n", req.Title)
	fmt.Fprintf(&b, "风险: %s\n\n", req.Risk)
	fmt.Fprintf(&b, "最终状态: %s\n\n", finalStatus)
	fmt.Fprintf(&b, "生成时间: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	if strings.TrimSpace(message) != "" {
		fmt.Fprintf(&b, "说明: %s\n\n", strings.TrimSpace(message))
	}

	fmt.Fprintf(&b, "## Agent 状态\n\n")
	fmt.Fprintf(&b, "| Agent | Status | Output | Log | Next |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | --- |\n")
	for _, agent := range []Agent{AgentAnalysis, AgentReview, AgentTestPlan, AgentDev, AgentTest, AgentCommit} {
		current := status.Agents[string(agent)]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			agent,
			current.Status,
			current.Output,
			current.Log,
			current.NextStatus,
		)
	}

	fmt.Fprintf(&b, "\n## 本次运行\n\n")
	if len(runs) == 0 {
		fmt.Fprintf(&b, "没有 Agent 运行。\n")
	} else {
		for _, run := range runs {
			fmt.Fprintf(&b, "### %s\n\n", run.Agent)
			fmt.Fprintf(&b, "- ExitCode: %d\n", run.ExitCode)
			fmt.Fprintf(&b, "- NextStatus: %s\n", run.NextStatus)
			fmt.Fprintf(&b, "- Score: %d\n", run.Score)
			fmt.Fprintf(&b, "- Log: %s\n", run.OutputPath)
			fmt.Fprintf(&b, "- Artifact: %s\n\n", filepath.Join(req.TaskDir, ArtifactForAgent(run.Agent)))
		}
	}

	fmt.Fprintf(&b, "\n## 产物文件\n\n")
	for _, name := range []string{"task.md", "analysis.md", "review.md", "test-plan.md", "implementation.md", "test.md", "commit.md", "agent-status.json"} {
		fmt.Fprintf(&b, "- %s\n", filepath.Join(req.TaskDir, name))
	}
	return os.WriteFile(filepath.Join(req.TaskDir, AgentReportFileName), []byte(b.String()), 0o644)
}
