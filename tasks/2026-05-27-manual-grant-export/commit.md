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

提交 manual grant export PostgreSQL 持久化实现与集成测试，并同步任务状态记录。

## Review

~~~text
PASS
~~~

review.md 决议：PASS。无 P0/P1；仅剩 P2 口径问题，不阻塞提交。

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
hash: c7f2e65
message: Implement manual grant export persistence
remote: not pushed
~~~

需求：2026-05-27-manual-grant-export
