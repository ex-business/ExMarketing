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

已提交目标任务相关变更：

- 管理台模拟运行、上线前校验、审批记录、goLive、rollback 页面能力。
- 上线前校验 `WARN` / `APPROVAL_REQUIRED` 结论语义与高危审批门禁。
- goLive / rollback 状态变更 reason 记录及审计 action。
- 目标任务目录记录文件，不包含 `tasks/*/runs/` 执行日志。

未纳入提交：

- 其他任务目录的 agent 状态/报告变更。
- `requirements/backlog.json` 中混合的其他任务状态变更。
- 所有 `tasks/*/runs/` 日志。

提交前检查：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

结果：全部通过。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Commit

~~~text
hash: 04f11ddfd2b9483b759e01bea144c55192008fcc
message: Add admin simulation approval pages
remote: not pushed (local main is ahead of origin/main)
~~~

需求：2026-05-29-admin-simulation-approval-pages
