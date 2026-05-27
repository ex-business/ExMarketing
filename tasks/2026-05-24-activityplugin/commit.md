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

- 提交 ActivityPlugin 注册表重复 pluginCode 防护。
- 提交对应单元测试，覆盖重复注册拒绝和失败后原插件仍可查找。
- 提交本任务相关分析、评审、测试记录和 backlog 状态更新。

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
hash: 98f8d34
message: Add activity plugin duplicate guard
remote: not pushed
~~~

需求：2026-05-24-activityplugin
