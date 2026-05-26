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

BLOCKED before commit. No files were staged or committed.

Pre-commit checks executed:

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

Blocking failure:

~~~text
git diff --check
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

Per Commit Agent constraints, this agent is not allowed to modify business code,
test code, documentation code, configuration files, or any file other than this
commit.md. The newline issue must be fixed by Dev Agent before commit can proceed.

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Commit

~~~text
hash: BLOCK
message: BLOCK - git diff --check failed
remote: not pushed
~~~

需求：2026-05-24-fact-query-2
