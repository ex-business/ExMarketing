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

- Added database migration release tooling and schema migration validation.
- Added `cmd/db-migration-release` entrypoint and `internal/dbmigration` implementation/tests.
- Updated PostgreSQL startup guard, operations/architecture docs, README, compose config, backlog, and task artifacts for this release.
- Excluded `tasks/*/runs/` execution logs from the commit.

## Review

~~~text
PASS
~~~

Evidence:

- `tasks/2026-05-27-db-migration-release-migration/review.md` decision: `PASS`
- No P0/P1 findings; score 92/100.

## Test

~~~text
PASS
~~~

Evidence:

- `tasks/2026-05-27-db-migration-release-migration/test.md` result: `PASS`
- Required pre-commit checks executed:
  - `git status -sb`
  - `git diff --check`
  - `go test ./...`
  - `go build ./...`

## Commit

~~~text
hash: 32b713c
message: Add database migration release checks
remote: not pushed
~~~

## Decision

~~~text
PASS
~~~

需求：2026-05-27-db-migration-release-migration
