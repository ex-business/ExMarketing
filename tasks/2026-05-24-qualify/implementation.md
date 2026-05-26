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

本轮按 `analysis.md` 的最小可验收口径实现 QUALIFY 修复：继续使用通用 `CONDITION` evaluator，不新增 VIP、注册天数、渠道、标签专用 evaluator。

主要修复 `IN` / `NOT_IN` 在实际字段为数组时的判断语义。此前只支持“期望数组包含实际标量”，当 `attributes.tags`、`attributes.riskTags` 等实际值为数组时无法按交集判断。现在：

- 实际值为标量：保持原语义，判断标量是否在期望数组中。
- 实际值为数组：判断实际数组与期望数组是否存在交集。
- `NOT_IN` 使用同一交集判断取反，适合风险标签、黑名单标签排除。

## Changed Files

- `internal/rule/evaluator.go`
  - 将 membership 判断抽成 `matchesMembership(expected, actual)`。
  - 支持 actual 为数组或标量的 `IN` / `NOT_IN`。
- `internal/rule/evaluator_test.go`
  - 新增 QUALIFY 测试覆盖 VIP、注册天数、渠道、标签数组准入、风险标签排除。
  - 新增 QUALIFY 缺失字段解释测试，验证 `FIELD_MISSING`、`field` 和拒绝后 `CanReward=false`、`CanReserveQuota=false`。
- `tasks/2026-05-24-qualify/test-plan.md`
  - 从模板态补齐为 PASS，明确 Test Agent 后续验收用例。
- `tasks/2026-05-24-qualify/implementation.md`
  - 记录本轮实现、行为变化和验证结果。

## Behavior Changes

- `attributes.tags IN ["new_user","high_value"]` 在 `attributes.tags=["sports","high_value"]` 时通过。
- `attributes.riskTags NOT_IN ["black","bonus_abuse"]` 在 `attributes.riskTags=["normal"]` 时通过，在存在交集时拒绝。
- 缺失 QUALIFY 字段仍按既有规则拒绝，并返回可解释 `FIELD_MISSING`。
- 不改变字段路径白名单、facts provider 引用校验、未注册 evaluator 拒绝策略。
- 不扩展完整 condition trace；按 Review P2，本任务最低验收仍为规则级解释稳定。

## Validation

~~~bash
go test ./internal/rule -run 'Qualify|Condition|Fact|Missing'
# ok github.com/ex-stack/ExMarketing/internal/rule 0.628s

go test ./internal/engine ./internal/httpapi -run 'Rule|Qualify|Simulate|Fact'
# ok github.com/ex-stack/ExMarketing/internal/engine 1.066s
# ok github.com/ex-stack/ExMarketing/internal/httpapi 0.908s

go test ./...
# ok; all packages passed

go build ./...
# ok
~~~

## Notes

当前工作区存在大量本任务外的既有未提交修改，本轮未回退或改动这些无关文件。

需求：2026-05-24-qualify
