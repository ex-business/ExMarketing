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

~~~text
提交当前 Outbox Worker 重试与 DLQ 任务的代码、测试、文档和任务产物：
- HTTP / WEBHOOK outbox worker retry、Retry-After、最大重试与 DLQ 状态流转
- DLQ 查询、replay、close API 与 memory/PostgreSQL repository 支持
- outbox worker 与 HTTP DLQ API 单元测试
- README、API、operations 文档和本任务流水线产物

未暂存 tasks/*/runs/ 执行日志。
未提交工作区中无当前 review/test 闸门的 workflow 管理界面相关改动。
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
hash: 33998ea
message: Implement outbox worker retry and DLQ
remote: not pushed
~~~

需求：2026-05-24-outbox-worker-dlq
