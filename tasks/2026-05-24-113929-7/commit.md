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
- `internal/engine/service.go`
- `internal/engine/service_test.go`
- `internal/marketing/model.go`
- `requirements/backlog.json`
- `tasks/2026-05-24-113929-7/agent-report.md`
- `tasks/2026-05-24-113929-7/agent-status.json`
- `tasks/2026-05-24-113929-7/analysis.md`
- `tasks/2026-05-24-113929-7/implementation.md`
- `tasks/2026-05-24-113929-7/review.md`
- `tasks/2026-05-24-113929-7/test-plan.md`
- `tasks/2026-05-24-113929-7/test.md`

未暂存 / 未提交 `tasks/2026-05-24-113929-7/runs/` 执行日志。

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
hash: 12141a0
message: feat: add marketing event simulation previews
remote: not pushed; local main is ahead of origin/main by 1 commit
~~~

需求：2026-05-24-113929-7
