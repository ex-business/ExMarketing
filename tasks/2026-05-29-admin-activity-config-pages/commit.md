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

- 提交后台活动配置页面实现与 backlog 状态更新。
- 提交本任务交付记录：task、analysis、review、test-plan、implementation、test、agent report/status/prompts。
- 未提交 `tasks/2026-05-29-admin-activity-config-pages/runs/` 执行日志。
- 未提交 `commit.md` 本身；本文件按 Commit Agent 要求在提交后记录结果。

## Pre-Commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

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
hash: 1b285b0a0c70dc4f01c112d1b97fcc450030d647
message: Add admin activity config pages
remote: not pushed
~~~

需求：2026-05-29-admin-activity-config-pages
