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

- Committed `POST /openapi/v1/events` OpenAPI Event API auth, authorization, request boundary, and error response changes.
- Included related user-facing docs and task records for `tasks/2026-05-24-http-event-api`.
- Excluded `tasks/*/runs/` execution logs.

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
hash: 0a7b1a0aea4c131bfc4ff92e48864f1ebcade271
message: Add OpenAPI event auth controls
remote: not pushed
~~~

需求：2026-05-24-http-event-api
