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

待填写。

## Review

~~~text
PASS / BLOCK
~~~

## Test

~~~text
PASS / FAIL
~~~

## Commit

~~~text
hash:
message:
remote:
~~~

需求：2026-05-24-outbox-worker-dlq
