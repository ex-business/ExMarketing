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

~~~text
BLOCKED before commit.

Changed files were present across Go source/tests, requirements, and task documents.
No files were staged or committed because a required pre-commit check failed.
~~~

## Review

~~~text
PASS

Evidence:
- tasks/2026-05-24-reward/review.md Decision: PASS
- P0: None
- P1: None
- Score: 86
~~~

## Test

~~~text
PASS

Executed pre-commit commands:
- git status -sb: completed
- git diff --check: FAIL
- go test ./...: PASS
- go build ./...: PASS

go test ./... summary:
- PASS for all packages with tests.
- No test files in cmd/exmarketing, cmd/workflowctl, internal/decimal, internal/plugin.

go build ./... summary:
- PASS, no build errors.
~~~

## Commit

~~~text
BLOCK

Reason:
git diff --check failed:
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.

Per Commit Agent constraints, this must be fixed by Dev Agent. Commit Agent did not modify business code, test code, documentation code, configuration, or any file other than this commit.md.

hash: N/A
message: N/A
remote: N/A
~~~

需求：2026-05-24-reward
