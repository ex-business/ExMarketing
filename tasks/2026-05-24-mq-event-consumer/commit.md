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

- `internal/engine/service.go`
  - 新增 `ProcessEventFrom(ctx, event, actor)`，保留 `ProcessEvent` 的 OpenAPI 默认 actor。
- `internal/mqconsumer/consumer.go`
  - 新增 broker-neutral MQ Event Consumer、幂等键校验/派生、allowlist、ACK/NACK_RETRY/DLQ 分类与失败记录。
- `internal/mqconsumer/consumer_test.go`
  - 覆盖成功消费、重复消费、派生幂等键、MQ 审计 actor、DLQ 与 retryable failure。
- `tasks/2026-05-24-mq-event-consumer/*`
  - 提交本任务 analysis/review/test-plan/implementation/test/report/status/prompt 交付件。

未暂存 `tasks/*/runs/` 执行日志。

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
hash: 1371fd5a1c14f7a9e6242eeee8f5e66cf27e61ba
message: feat: add mq event consumer
remote: not pushed
~~~

需求：2026-05-24-mq-event-consumer
