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

BLOCKED. No commit was created because the required pre-commit check
`git diff --check` failed outside this task scope.

Failing check:

~~~text
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

Per Commit Agent constraints, this agent is not allowed to modify
`tasks/2026-05-24-qualify/analysis.md` or any file other than this
`commit.md`. The flow must be returned to Dev Agent to fix the whitespace
error, then Commit Agent can rerun the gate.

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
git status -sb: completed
git diff --check: FAIL
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
BLOCK
hash: none
message: none
remote: not pushed
~~~

需求：2026-05-24-grant-order
