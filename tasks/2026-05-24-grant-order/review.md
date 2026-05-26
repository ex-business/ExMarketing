# Review

## Findings

```text
P0:

P1:

P2:
- tasks/2026-05-24-grant-order/analysis.md:222-231,311-316
  callback 规则已经覆盖 `SENDING` / `RETRY` 首次终态、重复同终态、相反终态和 quota reservation 冲突，但没有显式写出重复 `SUCCESS` callback 携带不同 `externalGrantId` 时必须返回冲突。`docs/api.md` 已要求 `externalGrantId` 冲突返回机器可识别错误；建议 Test Plan 将该 case 纳入 callback 测试，避免同一 `commandId` 被不同外部流水静默视为 duplicate。该问题不影响本轮状态机主流程闭环，不构成阻塞。
```

## Review Notes

- 上一轮 P1 已关闭：`RETRY` 现在明确表示“本地未获得外部最终结果”，并允许合法 callback 在 reservation 仍为 `FROZEN` 时执行 `RETRY -> SUCCESS/FAILED`，覆盖发送超时、响应丢失、DLQ 后迟到 callback 等可靠投递场景。
- 上一轮 P2 已关闭：`SENDING` 悬挂恢复给出默认 `15 分钟` 阈值、`SENDING -> RETRY` 动作、`retry_count` 递增、quota 保持 `FROZEN`、并发 callback 不覆盖终态等最小可验收口径。
- `bill_no` / `commandId` 稳定复用、tenant 隔离、重复创建幂等、DLQ replay / close 联动 grant order、quota reservation 确认 / 释放规则均已形成可测试边界。
- 验收标准要求 PostgreSQL 覆盖唯一约束、事务、callback tenant 隔离、outbox claim、`SENDING` 恢复、DLQ replay / close 和 quota reservation，符合该任务的高风险等级。

## Open Questions

- 重复 `SUCCESS` callback 但 `externalGrantId` 不同的返回码和审计字段建议在 Test Plan 中固定，默认应按 `CALLBACK_STATUS_CONFLICT` 或等价机器可识别冲突处理。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          18/20
验收标准明确      18/20
架构一致性        18/20
总分              90/100
```

## Decision

```text
PASS
```

无 P0/P1，Score 为 90/100，未触发 BLOCK 规则。需求可进入 Test Plan。

需求：2026-05-24-grant-order
