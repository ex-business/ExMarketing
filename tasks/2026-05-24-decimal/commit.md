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

BLOCKED before commit. No git commit was created.

提交范围未暂存；当前工作区包含 decimal 任务相关变更，也包含其他任务目录与文档变更。按提交前检查结果，本轮禁止提交。

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
git status -sb: executed
git diff --check: FAIL
go test ./...: PASS
go build ./...: PASS
~~~

## Blocker

~~~text
BLOCK

git diff --check failed:
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.

Commit Agent is only allowed to write tasks/2026-05-24-decimal/commit.md, so this file was not fixed here. Return to Dev Agent to remove the trailing blank line or otherwise resolve the whitespace error, then rerun the commit gate.
~~~

## Commit

~~~text
hash: none
message: none
remote: not pushed
~~~

需求：2026-05-24-decimal
