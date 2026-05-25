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

营销活动插件支持：

- 新增活动插件注册、上下文、运行时校验和预览链路。
- 新增上线前插件兼容性校验。
- 新增 `marketing_activity.plugin_code` 持久化字段与相关仓储映射。
- 补充插件、规则、奖励、PostgreSQL 和工作流相关测试与任务记录。

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
hash: af0fab3
message: feat: add marketing activity plugin support
remote: not pushed
~~~

需求：2026-05-24-113400-9
