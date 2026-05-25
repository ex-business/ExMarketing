# Review

## Findings

未发现 P0/P1 阻断问题。

上一轮阻断项已在本轮 `analysis.md` 中收敛：

```text
1. 预算安全闭环：
   analysis.md 明确要求 grant_order 级 quota reservation / ledger / voucher 等价凭证，
   且缺失凭证时必须返回 QUOTA_RESERVATION_MISSING，不允许用 aggregate frozen_value 猜测释放金额。

2. 多 grant_order participation 语义：
   analysis.md 明确不新增 PARTIAL / NEED_REVIEW，部分成功 + 部分失败最终 participation.FAILED，
   grant_order 明细保留已成功与已失败事实。

3. callback 安全边界：
   analysis.md 明确本期不做签名 / token，不得作为公网生产暴露能力验收；
   生产公网暴露必须前置鉴权需求。
```

## Non-Blocking Risks

```text
P2: reservation / ledger / voucher 被定义为“等价凭证”而非固定表结构。
    这给 Dev Agent 留出实现空间，但 Dev 阶段必须证明每笔 grant_order 可反查自身 quota 份额，
    且 PostgreSQL 并发下只能终结一次。

P2: 历史缺少 reservation 的 grant_order 默认不迁移，只暴露为 MISSING。
    该口径符合本任务范围，但上线前需要运维确认是否存在必须自动补偿的历史数据。

P2: SUCCESS 强制 externalGrantId 必填可能要求外部发奖系统适配。
    analysis.md 已把它列为风险，不影响本轮进入 Dev。
```

## Open Questions

以下问题不阻断 Dev；如 Owner 改变默认口径，需要重新分析：

```text
1. 后续是否需要 PARTIAL / NEED_REVIEW participation 状态？
2. 是否另起迁移任务处理缺少 reservation 的历史 grant_order？
3. DLQ_CLOSED 或人工 close 是否应释放预算？
4. 最小对账查询复用现有 admin 查询还是新增 reconciliation API？
5. callback 签名 / token 是否需要提前纳入本期？
```

## Score

```text
需求清晰度        18/20
范围可控          17/20
风险可控          17/20
验收标准明确      19/20
架构一致性        18/20
总分              89/100
```

## Decision

```text
PASS
```
