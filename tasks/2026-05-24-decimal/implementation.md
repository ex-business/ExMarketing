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

- 按 `analysis.md` / `review.md` / `test-plan.md` 的 Decimal 口径完成最小修复。
- 收紧 `internal/decimal` 解析，只接受普通十进制字符串，拒绝 `1/3`、科学计数法、NaN/Infinity 等非规范金额表达。
- 保留全程 Decimal 计算和最终 4 位字符串输出，补充 `.00005` 四舍五入边界测试。
- 奖励配置、阶梯配置、概率奖项和 quota 创建命令支持历史 JSON number 输入，并转换为原始十进制文本，不经过 `float64`。
- 规则条件执行改为 `json.Decoder.UseNumber()`，移除规则 Decimal 比较中的 `float64` 转换路径。
- Quota 校验改为非负 Decimal；`STOCK` / `USER_LIMIT` / `DAILY_LIMIT` 要求非负整数语义 Decimal，允许 `0` 作为立即封顶配置。
- 修复数量类非金额 reward 的整数校验，避免 `1.00001` 被最终 4 位量化后误判为整数。

## Changed Files

- `internal/decimal/decimal.go`
- `internal/decimal/decimal_test.go`
- `internal/marketing/model.go`
- `internal/marketing/prelaunch_validation_test.go`
- `internal/reward/calculator.go`
- `internal/reward/calculator_test.go`
- `internal/rule/evaluator.go`
- `internal/rule/evaluator_test.go`
- `internal/httpapi/handler_test.go`
- `tasks/2026-05-24-decimal/test-plan.md`
- `tasks/2026-05-24-decimal/implementation.md`

## Behavior Changes

- `RewardConfig` 中的 `amount`、`rate`、`maxAmount`、`minAmount`、`step`、`poolAmount`、`flowMultiple`、估值和数量字段可用 JSON string 或 JSON number 表达；内部统一按 Decimal 字符串校验和计算。
- `RewardLadder.min/max/amount`、`ProbabilityItem.weight/amount` 可用 JSON number 输入，仍按 Decimal 字符串进入计算。
- `CreateQuotaCommand.totalValue` 可用 JSON number 输入；金额类预算允许非负 Decimal，库存和次数限制只允许非负整数 Decimal。
- 规则条件中的 JSON number 比较值保持 `json.Number`，不再落入 `float64`。
- 非 Decimal 普通文本格式如 `1e-3`、`1/3` 不再被金额解析接受。

## Notes

- `go test ./...`：PASS
- `go build ./...`：PASS
- 首次全量测试发现旧 HTTP 测试用 `0.1` 构造库存/次数超限；按本需求“库存/次数为整数语义 Decimal”调整为 `0`，仍保持立即超限语义。
- 静态扫描 `internal/decimal internal/rule internal/reward internal/marketing internal/engine` 未发现金额相关 `float64` / `ParseFloat` / `FormatFloat` 使用。

需求：2026-05-24-decimal
