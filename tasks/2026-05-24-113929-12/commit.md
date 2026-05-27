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

提交管理后台通用配置页面及相关工作流状态文件。

未暂存或提交 `tasks/*/runs/` 执行日志。

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
PASS
hash: 6b07a23f5173bf258abb2114e524afbaa0f08504
message: Add generic admin activity configuration page
remote: not pushed
~~~

## Pre-Commit Checks

~~~text
git status -sb: PASS (executed)
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

需求：2026-05-24-113929-12
