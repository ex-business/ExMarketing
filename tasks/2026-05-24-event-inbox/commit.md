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

- 提交了 Event Inbox 任务材料、workflow 返工自动重试相关实现和测试、以及 `requirements/backlog.json` 中 `2026-05-24-event-inbox` 的状态 hunk。
- 未暂存或提交 `tasks/*/runs/` 执行日志。
- 未纳入其他任务目录的未提交改动。

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
hash: 0749484be5a900318c8bc9e3a25316cba15767c4
message: Handle event inbox workflow retry
remote: not pushed
~~~

## Remaining Uncommitted Files

~~~text
requirements/backlog.json
tasks/2026-05-24-event-inbox/agent-status.json
tasks/2026-05-24-fact-query/agent-report.md
tasks/2026-05-24-fact-query/agent-status.json
tasks/2026-05-24-fact-query/commit.md
tasks/2026-05-24-webhook-receiver/agent-report.md
tasks/2026-05-24-webhook-receiver/agent-status.json
tasks/2026-05-24-webhook-receiver/analysis.md
~~~

需求：2026-05-24-event-inbox
