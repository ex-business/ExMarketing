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

- Added Prometheus-style engine metrics instrumentation and `/metrics` rendering support.
- Covered event intake, quota reserve outcomes, outbox backlog/DLQ, callback failures, worker claim latency, and reward command latency.
- Updated operations documentation, task artifacts, and backlog status for `2026-05-27-engine-metrics-alerts`.
- Excluded unrelated task directory changes and did not stage any `tasks/*/runs/` execution logs.

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
hash: 3a06f6e
message: Add engine metrics alerts
remote: not pushed
~~~

需求：2026-05-27-engine-metrics-alerts
