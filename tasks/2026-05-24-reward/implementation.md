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

- 补强 `CONDITION` evaluator 的运行期校验：空 `conditions` 不再默认通过，而是返回 `INVALID_RULE_CONFIG`。
- 为条件规则结果增加 `Detail.conditions`，逐条记录 `field`、`op`、`expected`、`actual`、`passed` 和失败原因，满足 REWARD 阶段条件级解释要求。
- 保留既有 REWARD 执行边界：REWARD 只决定是否进入奖励计算，不直接生成外部发奖动作；grant_order/outbox 仍由后续奖励和 quota 链路处理。
- 按 Review P2 增加回归覆盖：非金额 COUPON reward 的 `reward_amount=0.0000` 不会被当成零奖励跳过，权益数量有效时仍生成 grant_order/outbox。

## Changed Files

- `internal/rule/evaluator.go`
- `internal/rule/evaluator_test.go`
- `internal/engine/service_test.go`
- `tasks/2026-05-24-reward/test-plan.md`
- `tasks/2026-05-24-reward/implementation.md`

## Behavior Changes

- `ruleType=CONDITION` 且 `conditions=[]` 的规则在运行期返回配置错误，避免绕过保存校验后静默通过。
- `AND` / `OR` 条件规则会完整评估条件列表并输出条件级解释；规则最终通过/失败语义不变。
- `IN` / `NOT_IN` 支持事件实际值为数组时按任一元素匹配，覆盖标签、风险标签等集合字段。
- 模拟运行中 REWARD 条件失败时，活动进入 rejected/unmatched 结果，不展示可执行 grant/outbox 预览。

## Verification

~~~bash
go test ./internal/rule ./internal/engine
go test ./...
go build ./...
~~~

结果：PASS。

## Notes

- 未新增业务专用字段别名、外部业务库查询、直接发奖逻辑或 reward calculator 计算规则。
- `test.md` 和 `commit.md` 当前仍为模板占位，未包含真实 FAIL/BLOCK 原因；本轮按返工反馈中可执行的最小修复处理。

需求：2026-05-24-reward
