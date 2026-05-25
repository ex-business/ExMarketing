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

- 标准事件 Schema 校验：限制 8 个顶层字段，校验 subject、biz、attributes 和 RFC3339 带时区时间。
- HTTP API 错误码：事件解析和模拟运行统一返回 `INVALID_EVENT`，幂等键错误返回 `INVALID_IDEMPOTENCY_KEY`。
- API 文档和测试覆盖：补充字段约束、错误示例、幂等键行为和接口级回归测试。
- 提交时未暂存 `tasks/*/runs/` 执行日志。

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
hash: ca66f05
message: Implement standard event schema validation
remote: not pushed
~~~

需求：2026-05-24-schema
