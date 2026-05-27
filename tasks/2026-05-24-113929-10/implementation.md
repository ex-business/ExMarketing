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

- 完成对账查询最小修复：`GET /admin/v1/reward/reconciliation` 支持 `participationId`、`outboxId` 定位，并统一参数错误码为 `INVALID_RECONCILIATION_QUERY`。
- 对账结果不再只依赖 grant_order 主表：memory repository 可从 participation、grant_order、outbox 和 event inbox 拼装可空链路，支持规则拒绝、无下游发奖单、孤立 outbox 等不完整链路。
- 增加每条链路的 `participation`、`grantOrder` 和 `diagnostics` 字段，同时保留既有扁平字段，兼容已有测试和页面展示。
- 补充 diagnostics：`EVENT_RECEIVED_NO_PARTICIPATION`、`PARTICIPATION_REJECTED`、`ZERO_REWARD_SKIPPED`、`PARTICIPATION_REWARDABLE_NO_GRANT_ORDER`、`GRANT_ORDER_NO_OUTBOX`、`OUTBOX_FAILED_OR_DLQ`、`OUTBOX_SENT_GRANT_PENDING`、`GRANT_SUCCESS_PARTICIPATION_NOT_GRANTED`、`GRANT_FAILED_QUOTA_STILL_FROZEN`。

## Changed Files

- `internal/marketing/model.go`
  - 扩展 `RewardReconciliationQuery`：新增 `ParticipationID`、`OutboxID`。
  - 扩展 `RewardReconciliationItem`：新增可选 `participation`、`grantOrder`、`diagnostics`，并将缺失下游时可能为空的字段改为 `omitempty`。
- `internal/httpapi/handler.go`
  - 解析 `participationId`、`outboxId`。
  - 对账查询错误码统一为 `INVALID_RECONCILIATION_QUERY`。
  - 响应增加顶层空 `diagnostics`，无数据时也保持可断言空诊断结构。
- `internal/engine/service.go`
  - 增加 tenant、定位条件、subject 成对参数、limit 上限校验。
- `internal/marketing/memory_repository.go`
  - 重写对账查询组装逻辑，覆盖 participation 主路径、grant order 主路径、outbox 主路径和 event-only 主路径。
  - 增加 outbox payload 中 `commandId`、`participationId`、`idempotencyKey` 的回溯关联。
- `internal/postgres/repository.go`
  - PostgreSQL 查询兼容 `participationId`、`outboxId` 过滤。
- `internal/httpapi/handler_test.go`
  - 新增 `TestRewardReconciliationValidatesQueryAndSupportsPartialLinks`，覆盖无定位条件、rejected participation、outboxId 查询和 diagnostics。

## Behavior Changes

- 只传 `tenantId` 且无任何定位条件时返回 `400 INVALID_RECONCILIATION_QUERY`，避免全表扫描。
- `subjectType` 和 `subjectId` 必须成对传入。
- `limit` 不能为负数，最大为 500。
- 按 `participationId` 查询时，即使无 grant_order/outbox，也返回 participation/event inbox 和对应 diagnostics。
- 按 `outboxId` 查询时，优先从 outbox payload 回溯发奖链路；无法关联上游时仍返回 outbox 本身并标记 `OUTBOX_UNLINKED`。

## Notes

- 本轮按 review 建议选择“机器可读 diagnostics 本轮落地”，并通过可空片段/数组表达缺失下游链路。
- `cursor`、`from/to` 仍未实现为 API 合同；test-plan 中保留为后续增强测试点，本轮没有扩大到分页游标和时间范围。
- 已执行并通过：

~~~bash
go test ./...
go build ./...
~~~

需求：2026-05-24-113929-10
