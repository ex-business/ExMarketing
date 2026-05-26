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

Reward callback implementation changes were present in the worktree, but no commit was created because a required pre-commit check failed.

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
git status -sb: PASS (worktree has pending changes)
git diff --check: FAIL
  tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
BLOCK
hash: none
message: none
remote: none
reason: git diff --check failed on tasks/2026-05-24-qualify/analysis.md:396 with "new blank line at EOF."; Commit Agent is not allowed to modify that file, so this must be returned to Dev Agent for cleanup before commit.
~~~

需求：2026-05-24-reward-callback
