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

已提交 MATCH 活动匹配规则实现与任务产物：

- `internal/engine/service.go`
- `internal/engine/service_test.go`
- `internal/httpapi/handler_test.go`
- `internal/mqconsumer/consumer_test.go`
- `docs/api.md`
- `tasks/2026-05-24-match/agent-prompts.md`
- `tasks/2026-05-24-match/agent-report.md`
- `tasks/2026-05-24-match/agent-status.json`
- `tasks/2026-05-24-match/analysis.md`
- `tasks/2026-05-24-match/implementation.md`
- `tasks/2026-05-24-match/review.md`
- `tasks/2026-05-24-match/test-plan.md`
- `tasks/2026-05-24-match/test.md`

未暂存或提交 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
git status -sb: executed before commit
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 92acdc4
message: Implement MATCH activity filtering
remote: not pushed
~~~

需求：2026-05-24-match
