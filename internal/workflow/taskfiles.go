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
	promptsPath := filepath.Join(req.TaskDir, "agent-prompts.md")
	bytes, err := os.ReadFile(promptsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(promptsPath, []byte(renderAgentPrompts(req)), 0o644)
		}
		return err
	}
	if !strings.Contains(string(bytes), "## Test Plan Agent") {
		content := strings.TrimRight(string(bytes), "\n") + "\n\n" + renderTestPlanAgentPrompt(req)
		return os.WriteFile(promptsPath, []byte(content), 0o644)
	}
	return nil
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

你是 Analysis Agent。请读取本目录 task.md、项目 README.md、CLAUDE.md、docs/architecture.md 和相关代码。

只做分析，不改代码，不提交。

## 评分规则

满分 100，低于 80 不允许进入研发：

~~~text
需求清晰度        20
范围可控          20
风险可控          20
验收标准明确      20
架构一致性        20
~~~

## 现状

待填写。

## 目标理解

待填写。

## 方案

待填写。

## 改动范围

待填写。

## 风险点

待填写。

## 验收方式

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

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

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
你是 Analysis Agent。请处理 %s。
读取 %s/task.md、README.md、CLAUDE.md、docs/architecture.md 和相关代码。
只写 %s/analysis.md，不改代码。
按评分规则打分，Score >= 80 且方案可控才 Decision PASS。
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
完成后必须执行 go test ./... 和 go build ./...。
完成后写 %s/implementation.md。
~~~

## Test Agent

~~~text
你是 Test Agent。请根据 %s/task.md、analysis.md、review.md、test-plan.md、implementation.md 执行测试。
执行必要测试并写 %s/test.md。
~~~

## Commit Agent

~~~text
你是 Commit Agent。只有 review.md 和 test.md 都 PASS 才能提交。
提交前必须执行 git status -sb、git diff --check、go test ./...、go build ./...。
提交时不要暂存 tasks/*/runs/ 执行日志；这些日志只用于页面查看，不属于代码提交内容。
提交后写 %s/commit.md。
~~~
`, req.ID, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, renderTestPlanAgentPrompt(req), req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir)
}

func renderTestPlanAgentPrompt(req Requirement) string {
	return fmt.Sprintf(`## Test Plan Agent

~~~text
你是 Test Plan Agent。请在 %s/review.md 为 PASS 后，先设计测试用例。
只写 %s/test-plan.md，不改业务代码。
测试计划必须覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
Decision 必须是 PASS / BLOCK。
~~~`, req.TaskDir, req.TaskDir)
}
