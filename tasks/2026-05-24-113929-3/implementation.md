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

- 按 `analysis.md` 和 `review.md` 的最小口径确认现有 `FIXED`、`RATIO`、`LADDER` 奖励计算实现。
- 补强连续签到阶梯奖励单元测试，覆盖 `continueDays=1/3/7` 三个阶梯命中结果。
- 补齐 `test-plan.md`，明确测试策略、用例和 PASS 决策。

## Changed Files

- `internal/reward/calculator_test.go`
- `tasks/2026-05-24-113929-3/test-plan.md`
- `tasks/2026-05-24-113929-3/implementation.md`

## Behavior Changes

- 无生产代码行为变更。
- LADDER 单测现在验证连续签到阶梯 `[1,2]`、`[3,6]`、`[7,999]` 分别输出 `5.0000`、`18.0000`、`58.0000`。
- `LADDER` 未命中继续沿用 `ReasonLadderNotMatched` 明确失败语义，不改为 0 奖励跳过。

## Notes

- 已执行 `go test ./...`：PASS。
- 已执行 `go build ./...`：PASS。
- `test.md` 和 `commit.md` 当前只是模板占位，没有实际 `FAIL` / `BLOCK` 失败原因；本轮未替 Test Agent 或 Commit Agent 写入结果。

需求：2026-05-24-113929-3
