# Review

## Findings

```text
P0:
- None.

P1:
- None.

P2:
- tasks/2026-05-27-manual-grant-export/analysis.md:172-188,373
  `MANUAL_EXPORT` 的主验收口径写成“导出文件 READY 后进入 SENDING，并可逐单确认 SUCCESS/FAILED/CLOSED”，但通用合法流转同时列出了 `PENDING -> SUCCESS/FAILED/CLOSED`。如果 Dev 按通用流转无条件实现，导出发奖单可能在未纳入任何 READY 导出文件、没有 exportId / 明细 / checksum 追踪的情况下直接确认成功，削弱“导出文件可追踪”的核心目标。建议 Dev/Test 收口为：`MANUAL_EXPORT` 的成功确认默认只允许已 READY/已交付文件内的 `SENDING` 单据；若允许未导出单据人工失败或关闭，需要在接口语义、审计和验收中明确这是取消处理而不是文件处理结果。

- tasks/2026-05-27-manual-grant-export/analysis.md:124-127
  审计 payload 被统一描述为必须包含 `exportId`，但 `MANUAL` 确认成功、确认失败、关闭天然没有导出文件。该表述容易导致实现为人工单生成无意义 exportId，或测试误判手工确认审计缺字段。建议明确：`exportId` 仅对导出相关动作和来自导出文件的逐单确认必填；纯 `MANUAL` 动作应记录为 null / absent，并以 `grantOrderId` / `billNo`、operator、reason 作为审计主键线索。
```

## Open Questions

```text
- `MANUAL_EXPORT` 未纳入 READY 导出文件前，是否允许直接人工确认 SUCCESS？
  建议默认不允许；失败或关闭可作为取消处理路径，但必须保留独立审计原因，并不得伪造导出追踪。
```

## Score

```text
88
```

扣分说明：上一轮 BLOCK 的 quota reservation 粒度、人工单 timeout-release 隔离、导出文件失败原子性已经补齐。剩余问题是导出模式确认前置条件和审计字段可选性表述不够精确，属于会影响接口/测试口径但不改变核心链路可开发性的 P2。

## Decision

```text
PASS
```

规则未命中 BLOCK：无 P0/P1，Score >= 80。

需求：2026-05-27-manual-grant-export
