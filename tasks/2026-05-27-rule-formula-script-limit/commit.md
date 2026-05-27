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

- Committed safe `FORMULA` rule support with a bounded parser/evaluator.
- Included creation/prelaunch validation, runtime evaluator behavior, unit tests, and API/data-model documentation.
- Excluded `tasks/*/runs/` execution logs and unrelated task directories from the commit.

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 8c4453efdbecd8e75508a25e8c5c4eb8cb070caf
message: Add safe formula rule evaluator
remote: not pushed
~~~

需求：2026-05-27-rule-formula-script-limit
