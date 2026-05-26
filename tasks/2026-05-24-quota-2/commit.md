# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 和 test.md 都 PASS 才能提交。

提交前必须检查：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

## Scope

本轮提交前检查读取了当前工作区状态，并确认 `tasks/2026-05-24-quota-2/review.md` 与 `tasks/2026-05-24-quota-2/test.md` 均为 PASS。

只允许写入本文件；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

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
git diff --check: BLOCK
go test ./...: PASS
go build ./...: PASS
~~~

`git diff --check` 失败：

~~~text
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

按 Commit Agent 约束，当前流程不能修改 `tasks/2026-05-24-qualify/analysis.md` 或任何非本文件内容，因此不能自行修复该问题，必须打回 Dev Agent 处理。

## Decision

~~~text
BLOCK
~~~

## Commit

~~~text
未提交。
~~~

需求：2026-05-24-quota-2
