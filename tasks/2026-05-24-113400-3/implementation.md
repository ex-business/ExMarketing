# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已实现规则引擎四阶段执行：

- 固定阶段顺序：`MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
- `RISK_LIMIT_BUDGET` 合并执行 `RISK` / `LIMIT` 规则阶段；未新增 `BUDGET` 作为 rule_phase。
- 规则执行输出活动级、阶段级、规则级解释，包含 `ruleCode`、`rulePhase`、`ruleType`、`status`、`passed`、`reasonCode`、`reason`、`field`、`expected`、`actual`、`sort`。
- `enabled=false` 规则输出 `SKIPPED / RULE_DISABLED`，不影响裁决。
- `CONDITION` 支持读取 `attributes.*`、`state.*`、`facts.*` 字段路径。
- 新增 `rule.Registry`，主流程只调用四阶段 executor；新增 evaluator 通过注册扩展，未在 engine 主流程新增具体规则类型分支。

## Changed Files

- `internal/rule/evaluator.go`
- `internal/rule/evaluator_test.go`
- `internal/marketing/model.go`
- `internal/engine/service.go`
- `internal/httpapi/handler_test.go`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-113400-3/test-plan.md`

## Behavior Changes

- 正式事件处理会在返回体附带 `ruleResults`；规则拒绝时写 `REJECTED` participation，不生成 grant order/outbox，也不进入 quota 扣减。
- 模拟运行会返回同样的规则解释明细；规则拒绝后不生成模拟 grant/outbox。
- 未注册 evaluator、非法 phase、非法配置、非法 op、字段缺失、fact 字段缺失、Decimal 类型不匹配均不会默认通过。
- quota 最终扣减逻辑保持原有原子事务语义；预算、库存、用户次数不足仍返回 `QUOTA_EXCEEDED` 并写审计。

## Notes

验证命令已执行：

~~~bash
go test ./...
go build ./...
~~~

结果均 PASS。

需求：2026-05-24-113400-3
