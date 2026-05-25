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

- 已提交当前需求相关代码、迁移、API 文档、README、backlog 和 `tasks/2026-05-24-113400-10` 任务文件。
- 未暂存、未提交 `tasks/*/runs/` 执行日志。
- 未提交其他任务目录改动：`tasks/2026-05-24-113400-9/*`、`tasks/2026-05-24-113400-11/*`。

## Pre-commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

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
hash: 1ca24efa774f6bd32ca0b7c892c62b1a7fd41972
message: feat: add marketing audit reconciliation
remote: not pushed
~~~

需求：2026-05-24-113400-10
