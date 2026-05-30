# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。
只允许写 commit.md；禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
如果提交前检查发现代码需要修复，必须在 commit.md 写明 BLOCK 和原因，让流程打回 Dev Agent，不能自己修。

提交前必须检查：

~~~bash
git status -sb
go test ./...
go build ./...
~~~

## Scope

- Committed `docs/security-threat-model.md`.
- Committed task artifacts under `tasks/2026-05-29-security-threat-model/`, excluding `runs/` logs and this post-commit `commit.md`.
- Did not stage unrelated modified or untracked files.

## Review

~~~text
PASS

review.md Decision = PASS.
No P0/P1 findings.
~~~

## Test

~~~text
PASS

test.md Result = PASS.

Pre-commit checks:
- git status -sb: executed before commit.
- git diff --check: PASS.
- go test ./...: PASS.
- go build ./...: PASS.
- git diff --cached --check: PASS.
~~~

## Commit

~~~text
hash: 31bb3a241b5da51ede0119081435540dfdf60c09
message: docs: add security threat model
remote: not pushed
~~~

## Post-Commit Status

~~~text
Branch: main...origin/main [ahead 12]
Remaining worktree changes are unrelated pre-existing modified/untracked files.
tasks/*/runs/ logs were not staged or committed.
~~~

需求：2026-05-29-security-threat-model
