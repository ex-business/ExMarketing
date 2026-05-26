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

BLOCK，未提交。

原因：提交前硬性检查 `git diff --check` 失败，发现非本任务允许写入范围内的既有文件存在 whitespace 问题：

~~~text
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

根据 Commit Agent 约束，本轮只允许写入 `tasks/2026-05-24-113929-3/commit.md`，禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件；因此不能自行修复该文件，流程需打回 Dev Agent 或由对应任务修复后重试提交。

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
git status -sb: PASS，已执行；工作区存在大量未提交变更。
git diff --check: BLOCK，tasks/2026-05-24-qualify/analysis.md:396 存在 new blank line at EOF.
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: BLOCK，未提交
message: BLOCK，未提交
remote: 未推送
~~~

需求：2026-05-24-113929-3
