package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CreateTaskFiles(req Requirement) error {
	if req.TaskDir == "" {
		return fmt.Errorf("taskDir is required")
	}
	if err := os.MkdirAll(req.TaskDir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"task.md":           renderTask(req),
		"analysis.md":       renderAnalysis(req),
		"review.md":         renderReview(req),
		"test-plan.md":      renderTestPlan(req),
		"implementation.md": renderImplementation(req),
		"test.md":           renderTest(req),
		"commit.md":         renderCommit(req),
		"agent-prompts.md":  renderAgentPrompts(req),
		"agent-report.md":   renderInitialAgentReport(req),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(req.TaskDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return SaveAgentStatus(req.TaskDir, NewAgentStatusFile(req))
}

func EnsureTaskFiles(req Requirement) error {
	if req.TaskDir == "" {
		return fmt.Errorf("taskDir is required")
	}
	if err := os.MkdirAll(req.TaskDir, 0o755); err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(req.TaskDir, "test-plan.md"), renderTestPlan(req)); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(req.TaskDir, "agent-prompts.md"), []byte(renderAgentPrompts(req)), 0o644)
}

func writeFileIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderInitialAgentReport(req Requirement) string {
	return fmt.Sprintf(`# Agent Report

需求 ID: %s

标题: %s

最终状态: %s

说明: 等待 Agent 运行。
`, req.ID, req.Title, req.Status)
}

func TaskDirFor(id string) string {
	return filepath.Join("tasks", id)
}

func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func renderTask(req Requirement) string {
	return fmt.Sprintf(`# Task

## 标题

%s

## 需求 ID

%s

## 背景

%s

## 目标

- 待 Analysis Agent 细化。

## 范围

- 待 Analysis Agent 评估。

## 非目标

- 待 Analysis Agent 评估。

## 风险等级

%s

## 验收标准

- 待 Analysis Agent 定义。
`, req.Title, req.ID, req.Description, req.Risk)
}

func renderAnalysis(req Requirement) string {
	return fmt.Sprintf(`# Analysis

## Agent 指令

你是 Analysis Agent，只负责需求分析和需求细化。
请读取本目录 task.md、PRD/设计文档、业务规则文档、已有 API/数据字典说明。
不需要读取代码，不允许用代码实现细节替代需求澄清。

只写 analysis.md，不写代码，不改代码，不提交，不做实现，不设计具体代码改法。
你的目标是把需求细节一定一定搞清楚：背景、目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、验收标准和待确认问题。
如果关键信息不清楚，必须 Decision BLOCK，并列出需要补充的问题；不要用代码实现假设替代需求澄清。

## 评分规则

满分 100，低于 80 不允许进入研发：

~~~text
需求清晰度        20
业务规则完整      20
边界异常清楚      20
验收标准明确      20
约束一致性        20
~~~

## 背景和已知信息

待填写。

## 目标理解

待填写。

## 细化需求

待填写。

## 业务规则

待填写。

## 输入输出

待填写。

## 异常场景

待填写。

## 边界条件

待填写。

## 非目标范围

待填写。

## 风险点

待填写。

## 验收方式

待填写。

## 待确认问题

待填写。

## Score

~~~text
0
~~~

## Decision

~~~text
PASS / BLOCK
~~~

需求：%s
`, req.ID)
}

func renderReview(req Requirement) string {
	return fmt.Sprintf(`# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

不改代码，不提交。

## Findings

~~~text
P0:
P1:
P2:
~~~

## Open Questions

待填写。

## Decision

~~~text
PASS / BLOCK
~~~

规则：

~~~text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
~~~

需求：%s
`, req.ID)
}

func renderImplementation(req Requirement) string {
	return fmt.Sprintf(`# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

待填写。

## Changed Files

待填写。

## Behavior Changes

待填写。

## Notes

待填写。

需求：%s
`, req.ID)
}

func renderTestPlan(req Requirement) string {
	return fmt.Sprintf(`# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

待填写。

## Test Cases

待填写。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

待填写。

## Decision

~~~text
PASS / BLOCK
~~~

需求：%s
`, req.ID)
}

func renderTest(req Requirement) string {
	return fmt.Sprintf(`# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

待填写。

## Commands

待填写。

## Result

~~~text
PASS / FAIL
~~~

## Evidence

待填写。

需求：%s
`, req.ID)
}

func renderCommit(req Requirement) string {
	return fmt.Sprintf(`# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。
只允许写 commit.md；禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
如果提交前检查发现代码需要修复，必须在 commit.md 写明 BLOCK 和原因，让流程打回 Dev Agent，不能自己修。

提交前必须检查：

~~~bash
git status -sb
go test ./...
go build ./...
~~~

## Scope

待填写。

## Review

~~~text
PASS / BLOCK
~~~

## Test

~~~text
PASS / FAIL
~~~

## Commit

~~~text
hash:
message:
remote:
~~~

需求：%s
`, req.ID)
}

func renderAgentPrompts(req Requirement) string {
	return fmt.Sprintf(`# Agent Prompts

## Analysis Agent

~~~text
你是 Analysis Agent，只负责需求分析和需求细化。请处理 %s。
如果 %s/review.md 存在 BLOCK、P0/P1/P2、Open Questions 或打回原因，必须先读取并逐条回应 Review Agent 的意见。
如果 %s/analysis.md 自己上一轮存在 Decision BLOCK、Score < 80、扣分原因或待确认问题，必须继续读取并逐条回应 Analysis 自己的 BLOCK 原因、扣分项、待确认问题和错误假设。
然后再读取 %s/task.md、PRD/设计文档、业务规则文档、已有 API/数据字典说明。
不需要读取代码，不允许用代码实现细节替代需求澄清。
只写 %s/analysis.md，不写代码，不改代码，不提交，不做实现，不设计具体代码改法。
输出细化后的需求说明、背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题。
如果是 Review 打回后的返工，analysis.md 必须单独包含“Review 反馈处理”小节，说明每条打回意见如何处理或为何仍需 BLOCK。
如果是 Analysis 自己上一轮 BLOCK 后的返工，analysis.md 必须单独包含“Analysis 自检修正”小节，说明每条 BLOCK 原因、扣分项、待确认问题如何修正。
返工时必须基于拒绝原因补充需求细节、边界条件、输入输出、验收标准和非目标范围；能从 task.md、backlog、PRD/设计文档或业务规则文档推导出明确最小口径的，直接写入 analysis.md 并说明推导依据。
如果仍无法从现有材料推导，必须继续 Decision BLOCK，并列出缺的需求信息；不要等待系统自动补充决策。
如果关键信息不清楚，必须 Decision BLOCK，并列出需要补充的问题；不要用代码实现假设替代需求澄清。
按评分规则打分，Score >= 80 且需求细节清楚、可审核、可验收才 Decision PASS。
~~~

## Review Agent

~~~text
你是 Review Agent。请审核 %s/analysis.md。
只写 %s/review.md，不改代码。
存在 P0/P1 或 Score < 80 必须 BLOCK。
~~~

%s

## Dev Agent

~~~text
你是 Dev Agent。请在 %s/review.md 和 %s/test-plan.md 为 PASS 后开发或修复。
严格按 %s/task.md、analysis.md、review.md、test-plan.md 修改代码和文档。
如果 %s/test.md 已存在且 Result 为 FAIL，必须优先根据失败原因做最小修复。
如果 %s/commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先根据提交失败上下文做最小修复。
完成后必须执行 go test ./... 和 go build ./...。
完成后写 %s/implementation.md。
~~~

## Test Agent

~~~text
你是 Test Agent。请根据 %s/task.md、analysis.md、review.md、test-plan.md、implementation.md 执行测试。
只允许写 %s/test.md；禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
如果测试失败，必须在 test.md 写清失败命令、失败日志摘要、失败原因和复现方式，让流程打回 Dev Agent，不能自己修。
执行必要测试并写 %s/test.md。
~~~

## Commit Agent

~~~text
你是 Commit Agent。只有 review.md 和 test.md 都 PASS 才能提交。
只允许写 %s/commit.md；禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
如果提交前检查发现代码需要修复，必须在 commit.md 写明 BLOCK 和原因，让流程打回 Dev Agent，不能自己修。
提交前必须执行 git status -sb、git diff --check、go test ./...、go build ./...。
提交时不要暂存 tasks/*/runs/ 执行日志；这些日志只用于页面查看，不属于代码提交内容。
提交后写 %s/commit.md。
~~~
`, req.ID, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, renderTestPlanAgentPrompt(req), req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir)
}

func renderTestPlanAgentPrompt(req Requirement) string {
	return fmt.Sprintf(`## Test Plan Agent

~~~text
你是 Test Plan Agent。请在 %s/review.md 为 PASS 后，先设计测试用例。
只允许写 %s/test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。
测试计划必须覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
Decision 必须是 PASS / BLOCK。
~~~`, req.TaskDir, req.TaskDir)
}
