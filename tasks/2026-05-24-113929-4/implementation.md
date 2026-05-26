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

已按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 做最小修复，收敛 `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 的奖励计算和配置校验边界。

本轮重点修复：

- `PROBABILITY` 权重语义改为百分比总和必须等于 `100.0000`，允许单个 item `weight=0`，但总和非法会在配置校验和运行计算两层阻断。
- `RANDOM` 的 `step` 改为必填；当 `step` 大于区间长度时只使用 `minAmount` 一个候选，避免生成超过 `maxAmount` 的随机金额。
- `POOL_SPLIT COUNT` 支持 analysis/test-plan 中定义的 `countField`，并兼容既有 `participantCountField`。
- 补充 reward 和 marketing 单元测试，覆盖概率总和、概率空奖、随机大步长、COUNT 奖池拆分别名。

## Changed Files

- `internal/reward/calculator.go`
- `internal/reward/calculator_test.go`
- `internal/marketing/model.go`
- `internal/marketing/prelaunch_validation_test.go`
- `tasks/2026-05-24-113929-4/implementation.md`

## Behavior Changes

- `CalculateProbability` 现在要求 `items[].weight` 总和严格等于 `100.0000`，否则返回 `PROBABILITY_TOTAL_INVALID`。
- `CreateActivityCommand.Validate` 对 `PROBABILITY` 配置执行相同总权重校验，非法概率配置不能进入可执行活动配置。
- `PROBABILITY` item 的 `weight=0` 合法，但不形成命中区间；全部为 0 会因总和不等于 100 被拒绝。
- `CalculateRandom` 缺少 `step` 时返回 `RANDOM_STEP_INVALID`；`step > maxAmount-minAmount` 时候选数为 1，结果为 `minAmount`。
- `POOL_SPLIT COUNT` 可使用 `countField` 读取事件或上下文字段，旧配置 `participantCountField` 仍兼容。

## Notes

验证命令已执行：

```bash
go test ./...
go build ./...
```

结果：均通过。

工作区在本轮开始前已有大量未提交改动；本轮仅围绕本需求相关文件做增量修复，没有回退无关改动。

需求：2026-05-24-113929-4
