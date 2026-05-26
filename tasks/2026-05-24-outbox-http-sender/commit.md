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

Commit gate for `2026-05-24-outbox-http-sender`.

No business code, test code, documentation code, configuration, or task run logs were modified by this Commit Agent.

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
git status -sb: PASS (worktree contains many existing modified files; branch main ahead of origin/main by 8)
git diff --check: BLOCK
go test ./...: PASS
go build ./...: PASS
~~~

## Blocker

~~~text
BLOCK

git diff --check failed:
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.

Per Commit Agent rules, this requires Dev Agent cleanup before commit.
This Commit Agent did not fix the file because only tasks/2026-05-24-outbox-http-sender/commit.md may be written.
~~~

## Commit

~~~text
hash: BLOCKED - no commit created
message: BLOCKED - git diff --check failed
remote: not pushed
~~~

需求：2026-05-24-outbox-http-sender
