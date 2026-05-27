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

- `docs/api.md`
- `docs/data-model.md`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/marketing/audit_context.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/worker/outbox_worker.go`
- `internal/worker/outbox_worker_test.go`
- `requirements/backlog.json`
- `tasks/2026-05-24-113929-9/agent-prompts.md`
- `tasks/2026-05-24-113929-9/agent-report.md`
- `tasks/2026-05-24-113929-9/agent-status.json`
- `tasks/2026-05-24-113929-9/analysis.md`
- `tasks/2026-05-24-113929-9/implementation.md`
- `tasks/2026-05-24-113929-9/review.md`
- `tasks/2026-05-24-113929-9/test-plan.md`
- `tasks/2026-05-24-113929-9/test.md`

提交同时包含上一需求 `2026-05-24-113929-8` 的流水线提交收尾文件：

- `tasks/2026-05-24-113929-8/agent-report.md`
- `tasks/2026-05-24-113929-8/agent-status.json`
- `tasks/2026-05-24-113929-8/commit.md`

未暂存 / 未提交 `tasks/*/runs/` 执行日志。

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
hash: 9de33ad
message: feat: add operation audit records
remote: not pushed; local main is ahead of origin/main by 3 commits
~~~

需求：2026-05-24-113929-9
