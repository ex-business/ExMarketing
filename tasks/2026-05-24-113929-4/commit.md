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

未提交。提交前检查发现工作区存在 `git diff --check` 阻断项，按 Commit Agent 规则打回 Dev Agent 修复。

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

## Block Reason

~~~text
BLOCK
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.

原因：提交前必须执行并通过 git diff --check；当前存在 whitespace error。按用户约束，Commit Agent 禁止修改除 tasks/2026-05-24-113929-4/commit.md 以外的任何文件，因此不能自行修复，必须打回 Dev Agent。
~~~

## Commit

~~~text
hash: BLOCK - no commit created
message: BLOCK - pre-commit check failed
remote:
~~~

需求：2026-05-24-113929-4
