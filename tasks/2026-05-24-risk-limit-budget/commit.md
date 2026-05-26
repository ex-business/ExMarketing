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

Risk/LIMIT/BUDGET implementation commit was blocked by pre-commit checks.

## Review

~~~text
PASS

Evidence:
- tasks/2026-05-24-risk-limit-budget/review.md Decision: PASS
~~~

## Test

~~~text
PASS

Evidence:
- tasks/2026-05-24-risk-limit-budget/test.md Result: PASS
- go test ./...: PASS
- go build ./...: PASS
~~~

## Pre-Commit Checks

~~~text
git status -sb: executed
git diff --check: FAIL
go test ./...: PASS
go build ./...: PASS
~~~

## Decision

~~~text
BLOCK
~~~

Reason:

~~~text
git diff --check failed:
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.

Commit Agent is only allowed to write tasks/2026-05-24-risk-limit-budget/commit.md.
The failing file is outside the allowed write scope, so this must be fixed by Dev Agent before commit can proceed.
No commit was created.
~~~

## Commit

~~~text
hash: N/A
message: N/A
remote: N/A
~~~

需求：2026-05-24-risk-limit-budget
