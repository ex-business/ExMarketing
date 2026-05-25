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

提交任务 `2026-05-24-113400-8` 的模拟运行接口实现：

- 扩展 `POST /admin/v1/simulate` 活动级结构化预览输出。
- 新增命中/未命中活动、规则解释、奖励预览、quota 影响预览、participation/grant/outbox command 预览。
- 增加只读 quota 查询仓储接口及 memory/postgres 实现。
- 增加模拟运行无副作用、quota 风险预览相关测试。
- 更新 API 文档与任务 8 流程文档。

未暂存、未提交 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS

review.md Decision: PASS
P0: 无
P1: 无
~~~

## Test

~~~text
PASS

提交前现场执行：
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: aa902061be1eb6d3a6eaed944e28271122cd283f
message: Implement marketing simulation preview
remote: not pushed
~~~

需求：2026-05-24-113400-8
