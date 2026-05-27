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

- 提交 `FIRST_DEPOSIT` 活动插件实现，支持从标准入金成功事件构建插件上下文并执行预检查。
- 提交默认插件注册、活动创建兼容性校验，以及规则/奖励读取插件上下文的引擎测试覆盖。
- 提交本需求相关 backlog、分析、实现、评审和测试记录。

未暂存 `tasks/*/runs/` 执行日志；未暂存既有 `tasks/2026-05-24-activityplugin/*` 脏改动。

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
hash: f17d4fec590aac3f6d3228c737a1388684c2fadc
message: Add first deposit plugin
remote: not pushed
~~~

需求：2026-05-24-113929-11
