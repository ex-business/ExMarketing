# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 和 test.md 都 PASS 才能提交。

提交前必须检查：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

## Scope

已提交 reward calculation modes 相关变更：

- 扩展 reward calculator 支持 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT。
- 补充 Decimal 精度处理、稳定随机 seed、预算/库存/用户次数处理路径。
- 更新模型、工作流、API/data model 文档、backlog 和任务记录。
- 未暂存或提交 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Pre-commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 686f0cb
message: Implement reward calculation modes
remote: not pushed
~~~

需求：2026-05-24-113400-4
