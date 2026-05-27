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

BLOCK：未提交。

本轮只允许提交 `review.md` 和 `test.md` 都为 PASS 的变更；当前 `test.md` 结论为 FAIL，PostgreSQL E2E runner 不能在同一数据库中重复运行，且失败诊断 SQL snapshot 查询存在参数/字段错误。该问题需要打回 Dev Agent 修复，Commit Agent 不修改业务代码、测试代码、文档代码或配置文件。

提交前检查已执行：

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
FAIL
~~~

## Commit

~~~text
hash: N/A
message: N/A
remote: N/A
~~~

需求：2026-05-27-engine-e2e-postgres-postgresql
