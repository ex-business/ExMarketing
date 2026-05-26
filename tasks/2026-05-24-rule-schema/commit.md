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

- Rule 配置 Schema 校验：`CONDITION / FORMULA / LIMIT / SCRIPT` 结构边界、字段路径、op/value、sort/version、重复 ruleCode。
- Admin create / prelaunch validation 复用 Schema 校验，非法规则不得保存或上线。
- 文档同步更新 Rule Schema 口径。
- 未暂存 `tasks/*/runs/` 执行日志。

## Pre-Commit Checks

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

~~~text
PASS
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
hash: 668151c
message: Validate rule configuration schema
remote: not pushed
~~~

需求：2026-05-24-rule-schema
