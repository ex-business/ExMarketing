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

func AutoSupplementRequirement(req Requirement) (bool, error) {
	if req.TaskDir == "" {
		return false, fmt.Errorf("taskDir is required")
	}
	path := filepath.Join(req.TaskDir, "task.md")
	bytes, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(bytes)
	if hasOwnerDecisionSection(content) {
		return false, nil
	}
	supplement, ok := defaultOwnerDecisionSupplement(req, content)
	if !ok {
		return false, nil
	}
	next := strings.TrimRight(content, "\n") + "\n\n" + supplement + "\n"
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func hasOwnerDecisionSection(content string) bool {
	return strings.Contains(content, "## Owner 补充决策") ||
		strings.Contains(content, "## Owner 决策") ||
		strings.Contains(content, "## 业主补充决策")
}

func defaultOwnerDecisionSupplement(req Requirement, taskContent string) (string, bool) {
	haystack := strings.ToLower(req.ID + "\n" + req.Title + "\n" + req.Description + "\n" + taskContent)
	if strings.Contains(haystack, "outbox") && strings.Contains(haystack, "dlq") &&
		(strings.Contains(haystack, "http/mq/webhook") || strings.Contains(haystack, "mq/webhook")) {
		return outboxDLQOwnerDecisionSupplement(), true
	}
	if strings.Contains(haystack, "fact query") || strings.Contains(haystack, "fact provider") || strings.Contains(haystack, "facts.*") {
		return factQueryOwnerDecisionSupplement(), true
	}
	if strings.Contains(haystack, "管理后台") || strings.Contains(haystack, "admin") {
		if strings.Contains(haystack, "审批") || strings.Contains(haystack, "上线") || strings.Contains(haystack, "活动配置") {
			return adminConsoleOwnerDecisionSupplement(), true
		}
	}
	return "", false
}

func outboxDLQOwnerDecisionSupplement() string {
	return `## Owner 补充决策

### 本期范围

本期只实现 HTTP / Webhook Outbox Worker。

MQ Command Out 不在本期实现范围内，拆分为后续独立任务。本期如果遇到 ` + "`target_channel = MQ`" + `，不进行投递，不把 MQ sender / MQ retry / MQ DLQ / MQ replay / MQ close 作为本任务验收项。

### 重试策略

Outbox Worker 使用指数退避重试。

默认参数：

- 最大重试失败次数：5 次。
- 初始延迟：30 秒。
- 退避倍率：2。
- 最大延迟：30 分钟。
- jitter：允许正负 20% 抖动，避免集中重试。
- 时间基准：以本次发送失败写入时间作为下一次 ` + "`next_retry_at`" + ` 的计算基准。
- 验收容差：` + "`next_retry_at`" + ` 允许正负 2 秒的测试容差，jitter 验收只校验落在允许区间内。

每次失败必须更新：

- ` + "`retry_count`" + `
- ` + "`next_retry_at`" + `
- ` + "`error_message`" + `
- ` + "`updated_at`" + `

可重试失败：

- 网络连接失败。
- DNS / TLS 错误。
- 请求超时。
- HTTP 408。
- HTTP 429。
- HTTP 5xx。

不可重试失败：

- HTTP 400 / 401 / 403 / 404 / 409 / 422：视为业务或配置错误，直接进入 DLQ。
- payload 格式错误：直接进入 DLQ。
- ` + "`target_ref`" + ` 非法：直接进入 DLQ。

HTTP 2xx 视为发送成功。本期不校验响应体业务字段。

HTTP 429 如果存在 ` + "`Retry-After`" + `，优先使用 ` + "`Retry-After`" + `；如果不存在或不可解析，则使用指数退避。

达到最大重试失败次数后进入 DLQ，自动投递停止。

### DLQ 状态语义

Outbox 增加机器可判定的关闭语义，使用明确状态 ` + "`DLQ_CLOSED`" + ` 表示人工关闭后的最终状态。

状态流转：

` + "```text" + `
PENDING -> SENDING -> SENT
PENDING -> SENDING -> FAILED -> PENDING
FAILED -> DLQ
DLQ -> PENDING      # replay
DLQ -> DLQ_CLOSED   # close
` + "```" + `

规则：

- ` + "`DLQ`" + ` 可以 replay。
- ` + "`DLQ_CLOSED`" + ` 不允许 replay。
- close 只允许对 ` + "`DLQ`" + ` 执行。
- replay 只允许对 ` + "`DLQ`" + ` 执行。
- close 必须记录 operator、reason、closed_at。
- replay 必须记录 operator、reason、replayed_at。
- replay 不重新执行活动规则、奖励计算、quota 扣减、participation 创建、grant_order 创建。
- replay 只重新发送原 outbox payload。
- retry / replay 必须复用原 ` + "`commandId`" + ` / ` + "`bill_no`" + `，依赖外部发奖系统幂等。
- replay 后如果再次失败，继续按同一重试和 DLQ 规则处理。
- close 与自动重试并发时，以状态条件更新为准；只有当前仍为 ` + "`DLQ`" + ` 才允许 close 成功。
- replay 与自动重试并发时，以状态条件更新为准；只有当前仍为 ` + "`DLQ`" + ` 才允许 replay 成功。`
}

func factQueryOwnerDecisionSupplement() string {
	return `## Owner 补充决策

### Provider Registry 契约

本期采用持久化 provider registry，不允许活动规则或 ` + "`/openapi/v1/facts/query`" + ` 直接传 inline provider 配置绕过 registry。

Fact Provider 以 ` + "`tenantId + factCode`" + ` 唯一定位。

Provider 字段：

- ` + "`tenantId`" + `
- ` + "`factCode`" + `
- ` + "`providerType`" + `，本期只支持 ` + "`HTTP`" + `
- ` + "`endpoint`" + `
- ` + "`method`" + `，本期支持 ` + "`GET`" + ` / ` + "`POST`" + `
- ` + "`authRef`" + `
- ` + "`mapping`" + `
- ` + "`cacheTtlSeconds`" + `
- ` + "`enabled`" + `
- ` + "`timeoutMs`" + `
- ` + "`createdAt`" + `
- ` + "`updatedAt`" + `

只有 ` + "`enabled = true`" + ` 且 schema 校验通过的 provider 才能被规则引用和 Fact Query 使用。

### 规则配置校验

后台配置规则时，如果规则引用 ` + "`facts.<factCode>.*`" + `，必须校验当前 tenant 下存在已启用的 ` + "`factCode`" + ` provider。

未配置 provider 时：

- 规则保存或上线校验必须失败。
- 错误信息必须指出缺失的 ` + "`factCode`" + `。
- 不能在运行时临时接受 inline provider 配置。

事件 ` + "`attributes`" + ` 足够完成判断时，不强制配置 Fact Provider。

### Fact Query API 输入契约

` + "`POST /openapi/v1/facts/query`" + ` 本期只接受：

- ` + "`tenantId`" + `
- ` + "`factCode`" + `
- ` + "`subject`" + `
- ` + "`biz`" + `
- ` + "`attributes`" + `
- ` + "`traceId`" + ` 或 request id

不接受 inline endpoint、inline auth、inline mapping 或 inline provider。

服务端按 ` + "`tenantId + factCode`" + ` 查找 enabled provider，完成 HTTP 查询、mapping、缓存和错误降级。

### 租户隔离

provider registry、规则校验、Fact Query 查询都必须按 ` + "`tenantId`" + ` 隔离。

跨 tenant 不能读取、引用或执行 provider。

### Mapping 与缓存

` + "`mapping`" + ` 负责把外部 HTTP 响应映射为标准 ` + "`facts.<factCode>.*`" + ` 字段。

` + "`cacheTtlSeconds`" + ` 为 0 表示不缓存，大于 0 表示按 tenant、factCode、subject、biz 和必要 attributes 组成缓存 key。

### 失败降级

Fact Query 是可选能力。provider 不存在、未启用、HTTP 失败、超时、mapping 失败或缓存失败时，必须返回可解释错误原因。

规则执行时是否阻断由规则配置决定，但失败原因必须进入 rule result / simulation explain。`
}

func adminConsoleOwnerDecisionSupplement() string {
	return `## Owner 补充决策

### 本期范围

本期只交付“当前后端 API 已支持或已在本工程明确存在的本地管理页面”。

审批流、上线发布、活动编辑发布、复制、删除、暂停、恢复、结束、版本回滚属于后续独立任务。本期页面可以展示入口或占位状态，但必须禁用这些操作，不允许伪造成功，不允许用前端状态假装后端已经支持。

本期页面定位为本地开发 / 内网管理台，不作为公网生产 Admin。

### 本期可执行页面

- 活动列表。
- 新增活动。
- 活动详情只读展示。
- Quota 创建和展示。
- 模拟运行。
- 审计记录查询。
- 当前 git diff / Agent 工作台相关页面。
- 已有 API 可返回的事件、Fact Query、Callback、对账或 DLQ 相关只读/操作能力；如果后端接口不存在，页面必须禁用并展示“待后端接入”。

### 审批与上线

审批流和上线发布不在本期实现范围内。

本期规则：

- 不提供可点击成功的提交审批、审批通过、审批拒绝、撤回、上线、下线、版本回滚。
- 如果展示审批/上线区域，只能展示禁用状态和待接入说明。
- 不能在前端本地改状态模拟审批成功。
- 不能把静态 mock 当作真实后端结果。

### 活动编辑发布

活动编辑、复制、删除、暂停、恢复、结束、版本回滚不在本期实现范围内。

本期只允许：

- 通过现有 ` + "`POST /admin/v1/activities`" + ` 创建活动。
- 通过现有列表/详情能力查看活动。
- 展示后端返回的真实状态。

### 错误处理

所有页面操作必须展示后端真实错误原因。接口不存在、接口返回非 2xx、校验失败、状态冲突时，页面必须显示失败，不得显示成功。`
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
如果 %s/review.md 存在 BLOCK、P0/P1/P2、Open Questions 或打回原因，必须先读取并逐条回应 Review Agent 的意见，再读取 %s/task.md、PRD/设计文档、业务规则文档、已有 API/数据字典说明。
不需要读取代码，不允许用代码实现细节替代需求澄清。
只写 %s/analysis.md，不写代码，不改代码，不提交，不做实现，不设计具体代码改法。
输出细化后的需求说明、背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题。
如果是 Review 打回后的返工，analysis.md 必须单独包含“Review 反馈处理”小节，说明每条打回意见如何处理或为何仍需 BLOCK。
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
`, req.ID, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, renderTestPlanAgentPrompt(req), req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir, req.TaskDir)
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
