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

提交数据库备份 / 恢复工具链：

- 新增 `cmd/db-backup-restore` 命令，支持 backup / restore / validate / smoke。
- 新增 `internal/dbbackup` 备份恢复 helper 与单元测试。
- 补充 migration schema 校验能力，并让 PostgreSQL E2E runId 可重复稳定执行。
- 更新 README、operations 文档、backlog 与本任务交付记录。

未暂存或提交 `tasks/*/runs/` 执行日志。
未提交其它任务目录的状态文件改动。

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
hash: fff3f45
message: Add database backup restore tooling
remote: not pushed
~~~

## Decision

~~~text
PASS
~~~

需求：2026-05-27-db-backup-restore
