# Review: 2026-05-24-dlq

## Findings

```text
P0:
- 无。

P1:
- 无。

P2:
1. close 与 grant_order / quota 的联动被明确排除在本期之外，但这会影响后续对账和补偿设计。
   analysis.md:220-221、303、367、373 已把 close 定义为只关闭 DLQ outbox，不处理 grant_order 终态和 quota reservation。该口径可进入研发，但后续必须由对账、callback 或 quota 超时释放需求承接，否则运营 close 后可能留下发奖单/配额悬挂状态。

2. Admin 鉴权和 operator 来源仍是最小口径。
   analysis.md:274、302、368、374 明确本期不做完整生产 Admin 鉴权和 RBAC，operator 由请求体传入。作为内部 Admin API 最小闭环可接受，但上线前需要避免公网暴露无鉴权 replay / close 能力。
```

## Review Notes

- 原任务要求的“查看失败原因、手动重放、关闭、重新进入 outbox、审计、防重复扣预算、防重复生成 grant_order”均已覆盖：analysis.md:60-65、88-92、151-164、205-222、243-250、315-358。
- replay 语义清晰：只重投原 outbox payload，不重新处理事件，不新建 participation / grant_order / outbox，不重复扣 quota：analysis.md:207-213、243-250、327-334。
- 状态边界清晰：仅 `DLQ` 可 replay / close，`DLQ_CLOSED` 是机器可判定终态，并发最多一个成功：analysis.md:178-195、258-263、345-352。
- 租户隔离和审计要求具备可验收性：analysis.md:109-127、224-241、252-256、347-357。

## Open Questions

- 非阻塞：后续是否需要 close 联动 grant_order 终态或 quota reservation 释放，本期已明确不纳入。
- 非阻塞：生产环境 operator 是否改为来自登录态 / RBAC，本期已明确按请求体传入。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              92/100
```

扣分原因：close 后业务补偿链路和生产 Admin 鉴权属于后续生产化风险，但 analysis 已将其明确为边界和风险，不阻塞本期 DLQ 管理需求进入研发。

## Decision

```text
PASS
```

需求：2026-05-24-dlq
