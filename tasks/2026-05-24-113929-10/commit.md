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

- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/marketing/memory_repository.go`
- `internal/marketing/model.go`
- `internal/postgres/repository.go`
- `requirements/backlog.json`
- `tasks/2026-05-24-113929-10/agent-prompts.md`
- `tasks/2026-05-24-113929-10/agent-report.md`
- `tasks/2026-05-24-113929-10/agent-status.json`
- `tasks/2026-05-24-113929-10/analysis.md`
- `tasks/2026-05-24-113929-10/implementation.md`
- `tasks/2026-05-24-113929-10/review.md`
- `tasks/2026-05-24-113929-10/test-plan.md`
- `tasks/2026-05-24-113929-10/test.md`

未暂存 / 未提交 `tasks/*/runs/` 执行日志。
未纳入当前工作区中上一需求 `2026-05-24-113929-9` 的收尾改动。

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
hash: 0eeb8fc
message: feat: add reward reconciliation diagnostics
remote: not pushed; local main is ahead of origin/main by 4 commits
~~~

需求：2026-05-24-113929-10
