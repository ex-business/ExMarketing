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

已提交 worker 生产化加固实现与本任务过程文件。

提交范围：

- `cmd/engine-e2e-postgres/main.go`
- `cmd/exmarketing/main.go`
- `internal/marketing/memory_repository.go`
- `internal/marketing/model.go`
- `internal/postgres/repository.go`
- `internal/postgres/repository_integration_test.go`
- `internal/worker/outbox_worker.go`
- `internal/worker/outbox_worker_test.go`
- `tasks/2026-05-27-worker-production-hardening-worker/agent-prompts.md`
- `tasks/2026-05-27-worker-production-hardening-worker/agent-report.md`
- `tasks/2026-05-27-worker-production-hardening-worker/agent-status.json`
- `tasks/2026-05-27-worker-production-hardening-worker/analysis.md`
- `tasks/2026-05-27-worker-production-hardening-worker/implementation.md`
- `tasks/2026-05-27-worker-production-hardening-worker/review.md`
- `tasks/2026-05-27-worker-production-hardening-worker/test-plan.md`
- `tasks/2026-05-27-worker-production-hardening-worker/test.md`

未暂存 / 未提交：

- `tasks/*/runs/` 执行日志未提交。
- 其它 task 目录改动未提交。
- `requirements/backlog.json` 含交叉 task 状态改动，未纳入本次提交。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

提交前检查：

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 9176a25f17294b6cf0d5e7559f632a0c00d0d49d
message: Harden outbox worker production behavior
remote: not pushed; main is ahead of origin/main by 3 commits after this commit
~~~

需求：2026-05-27-worker-production-hardening-worker
