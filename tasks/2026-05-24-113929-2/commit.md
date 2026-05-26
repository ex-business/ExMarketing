# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。

提交前必须检查：

~~~bash
git status -sb
go test ./...
go build ./...
~~~

## Scope

已提交本任务实现相关文件：

~~~text
docs/api.md
internal/engine/service.go
internal/httpapi/handler_test.go
internal/marketing/model.go
requirements/backlog.json
tasks/2026-05-24-113929-2/agent-prompts.md
tasks/2026-05-24-113929-2/agent-status.json
tasks/2026-05-24-113929-2/analysis.md
tasks/2026-05-24-113929-2/implementation.md
tasks/2026-05-24-113929-2/review.md
tasks/2026-05-24-113929-2/test-plan.md
tasks/2026-05-24-113929-2/test.md
~~~

未暂存、未提交其他任务目录改动；未暂存 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Pre-Commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: a1eb5e85e53af74b56f68f9b4c8371c608c8e1e6
message: Add activity display and entry config
remote: origin https://github.com/ex-business/ExMarketing.git
~~~

需求：2026-05-24-113929-2
