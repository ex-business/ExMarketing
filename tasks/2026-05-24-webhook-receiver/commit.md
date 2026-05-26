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

- Commit Agent checked `review.md` and `test.md`: both PASS.
- Submitted Webhook Receiver implementation and task artifacts.
- Excluded `tasks/*/runs/` execution logs from the commit.
- Left unrelated pre-existing worktree changes unstaged.

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
hash: 3b48d456e46a7b43c7b150058698f0790a76c9ca
message: Implement webhook receiver
remote: not pushed
~~~

需求：2026-05-24-webhook-receiver
