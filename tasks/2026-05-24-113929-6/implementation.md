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

已按本任务 `analysis.md` / `review.md` 的保守口径完成返工修复：

- 根据 Test Agent 失败反馈修正 WARN-only 整体决策：仅有 `PrelaunchWarning` 且无 `PrelaunchError` / `PrelaunchHighRisk` 时，整体 `conclusion=PASS`、`allowGoLive=true`；WARN 只保留在 `items[].result=WARN` 和审计明细中。
- 上线前校验中，高危配置不再通过 `APPROVAL_REQUIRED` 或 `ApprovalApproved` 放行；审批流未落地前统一返回 `BLOCK`，`allowGoLive=false`。
- 预算和库存校验改为使用可用额度：`totalValue - usedValue - frozenValue`，避免已用/冻结额度被重复当作可上线额度。
- 增加默认高危阈值封装：单次最大暴露 `1000.0000`、活动总预算 `100000.0000`，命中时生成 `HIGH_RISK_*` 阻断项。
- 库存型权益 reward 扩展为 `COUPON`、`GIFT`、`DRAW_CHANCE`、`MEMBERSHIP` 以及 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 计算类型，缺少库存或库存为 0 时阻断。
- `PrelaunchItem.result` 调整为任务要求的 `PASS` / `WARN` / `BLOCK` 结果值。

## Changed Files

- `internal/marketing/prelaunch_validation.go`
  - 调整 WARN-only 整体决策、高危决策、预算/库存可用额度计算、默认阈值和库存型 reward 判断。
- `internal/marketing/prelaunch_validation_test.go`
  - 更新高危审批相关用例为高危阻断。
  - 新增预算可用额度、库存型权益缺少库存 quota 的回归用例。
  - 新增 `TestValidatePrelaunchWarnOnlyStillAllowsGoLive`，覆盖只有 WARN 时整体 `PASS` 且允许上线。
- `internal/httpapi/handler_test.go`
  - 更新预检接口断言，从 `APPROVAL_REQUIRED` 改为 `BLOCK`。
- `tasks/2026-05-24-113929-6/implementation.md`
  - 记录本轮实现和验证结果。

## Behavior Changes

- 自动资金类 reward 缺少预算 quota 返回 `HIGH_RISK_MISSING_BUDGET`，整体 `BLOCK`。
- 自动资金类、变量型资金奖励、高于默认阈值的预算/单次暴露均为高危阻断，不再允许通过 approval 字段上线。
- 预算不足以可用预算判断，不再只看 `totalValue`。
- 库存不足以可用库存判断，不再只看 `totalValue`。
- 库存型权益 reward 没有 `STOCK + ACTIVITY + rewardCode` quota 时会返回 `STOCK_MISSING`。
- 普通非资金、非库存型 reward 缺少预算和库存 quota 时只返回 `BUDGET_OPTIONAL` / `STOCK_OPTIONAL` WARN，整体仍为 `PASS`。

## Notes

验证命令：

~~~bash
go test ./internal/marketing -run 'TestValidatePrelaunchWarnOnlyStillAllowsGoLive|TestValidatePrelaunch|TestCreateActivityCommandValidatesProbabilityTotalWeight|TestCreateActivityCommandValidatesRuleSchema|TestCreateActivityCommandValidatesRewardSchemaBoundaries|TestCreateQuotaCommandValidatesDecimalAndIntegerSemantics' -count=1
awk '
  /case hasWarn:/ {
    getline
    if ($0 ~ /conclusion = PrelaunchWarn/) {
      print "FAIL: warn-only prelaunch result sets conclusion to WARN; expected PASS with allowGoLive=true when no BLOCK items exist"
      print FILENAME ":" NR ": " $0
      exit 1
    }
  }
' internal/marketing/prelaunch_validation.go
go test ./...
go build ./...
~~~

验证结果：

~~~text
targeted prelaunch tests PASS
WARN-only awk assertion PASS
go test ./...  PASS
go build ./... PASS
~~~

仓库当前存在大量既有未提交变更，本轮只按需求修改上述相关文件，未回滚或整理其他任务文件。

需求：2026-05-24-113929-6
