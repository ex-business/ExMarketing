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

已提交本任务范围内的 MQ 生产运行时配置改造：

- MQ targetRef 解析与安全校验。
- 运行时 MQ sender 配置加载、凭据隔离、provider allowlist、timeout 和 transport ack 语义。
- outbox worker MQ 发送路径接入 retry / DLQ / SENT 流转。
- MQ consumer 日期测试回归修正。
- README、architecture、operations、backlog 和本任务记录更新。

未暂存/未提交：

- `tasks/*/runs/` 执行日志。
- `tasks/2026-05-27-manual-grant-export/*` 既有未提交改动。

提交前检查已执行：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

检查结果：全部通过。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Commit

~~~text
hash: 9b295cd8e9cddee360e106fff9336edd2360769e
message: Add production MQ runtime configuration
remote: not pushed; local main is ahead of origin/main by 2 commits
~~~

需求：2026-05-27-mq-production-config-mq
