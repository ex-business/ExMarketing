# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。

提交前必须检查：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

## Scope

~~~text
提交现有 Dev/Test/Review 产物与代码变更：
- marketing grant idempotency / quota / atomic grant 相关实现与测试
- PostgreSQL 幂等唯一约束与事务回滚 smoke 测试
- data-model 与任务记录更新

未暂存 tasks/*/runs/ 执行日志。
~~~

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Pre-commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 0c135a9
message: Add marketing grant idempotency safeguards
remote: not pushed
~~~

需求：2026-05-24-113400-5
