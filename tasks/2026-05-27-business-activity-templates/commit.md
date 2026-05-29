# Commit

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

- docs/api.md
- docs/business-activity-templates.md
- internal/marketing/business_activity_templates_test.go
- requirements/backlog.json
- tasks/2026-05-27-business-activity-templates/agent-prompts.md
- tasks/2026-05-27-business-activity-templates/agent-report.md
- tasks/2026-05-27-business-activity-templates/agent-status.json
- tasks/2026-05-27-business-activity-templates/analysis.md
- tasks/2026-05-27-business-activity-templates/implementation.md
- tasks/2026-05-27-business-activity-templates/review.md
- tasks/2026-05-27-business-activity-templates/test-plan.md
- tasks/2026-05-27-business-activity-templates/test.md

未暂存或提交 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS
~~~

Evidence: `tasks/2026-05-27-business-activity-templates/review.md` decision is PASS.

## Test

~~~text
PASS
~~~

Evidence: `tasks/2026-05-27-business-activity-templates/test.md` result is PASS.

## Pre-Commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 98a9cce
message: Add business activity templates
remote: not pushed
~~~

需求：2026-05-27-business-activity-templates
