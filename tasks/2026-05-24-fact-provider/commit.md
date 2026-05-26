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

Fact Provider 配置变更提交闸门。

候选变更以当前工作区为准；本 Commit Agent 未修改业务代码、测试代码、文档代码、配置文件或其他任务文件。

## Review

~~~text
PASS
~~~

Evidence:

- `tasks/2026-05-24-fact-provider/review.md` Decision 为 `PASS`。

## Test

~~~text
PASS
~~~

Evidence:

- `tasks/2026-05-24-fact-provider/test.md` Result 为 `PASS`。

## Pre-Commit Checks

~~~bash
git status -sb
~~~

Result: PASS，命令已执行。

~~~bash
git diff --check
~~~

Result: BLOCK。

~~~text
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

~~~bash
go test ./...
~~~

Result: PASS，退出码 0。

~~~bash
go build ./...
~~~

Result: PASS，退出码 0。

## Commit

~~~text
BLOCK
hash:
message:
remote:
~~~

## Block Reason

~~~text
提交前检查 `git diff --check` 失败。失败文件为 tasks/2026-05-24-qualify/analysis.md，
不属于本任务允许写入范围；按 Commit Agent 约束不能修复，也不能在检查失败时提交。
请打回对应 Dev Agent 修复 EOF 空行问题后重新执行提交流程。
~~~

需求：2026-05-24-fact-provider
