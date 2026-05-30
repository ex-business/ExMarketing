# Commit

## Scope

- 提交后台运维页本期最小闭环：审计过滤、奖励对账定位、DLQ replay/close、人工发奖确认、导出发奖创建。
- 提交后端审计查询 `startTime/endTime/traceId/requestId` 服务端过滤、manual grant 稳定审计 action、DLQ targetRef 脱敏。
- 提交 API 文档、测试用例、任务文档和 backlog 中本任务 `COMMIT_READY` 状态。
- 未暂存 `tasks/2026-05-29-admin-ops-pages-dlq/runs/` 执行日志。

## Gates

~~~text
review.md: PASS
test.md: PASS
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
hash: da7625f32acfef564d9e0b814283f07682584f9c
message: admin ops audit dlq pages
remote: not pushed
~~~

## Notes

- 提交后 `main` 相对 `origin/main` 为 ahead 8。
- 工作树仍有其它任务的未暂存文件，以及 `requirements/backlog.json` 中其它任务状态的未暂存变更；本次提交未包含这些无关改动。

需求：2026-05-29-admin-ops-pages-dlq
