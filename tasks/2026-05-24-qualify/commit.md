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

本轮提交门禁检查针对当前工作区变更。

未提交：提交前检查未全部通过，按 Commit Agent 规则打回 Dev Agent。

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
git diff --check: BLOCK
go test ./...: PASS
go build ./...: PASS
~~~

阻断原因：

~~~text
tasks/2026-05-24-qualify/analysis.md:396: new blank line at EOF.
~~~

处理结论：

~~~text
BLOCK
~~~

说明：根据用户指令，Commit Agent 只允许写入本文件；发现需要修复的代码/文档问题时不能自行修改，必须记录 BLOCK 并打回 Dev Agent。

## Commit

~~~text
hash:
message:
remote:
status: not committed
~~~

需求：2026-05-24-qualify
