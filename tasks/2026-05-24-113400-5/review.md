# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

不改代码，不提交。

## Findings

~~~text
P0: 无
P1: 无
P2:
1. analysis.md 将 participation.idempotency_key / grant_order.idempotency_key 的最终拼接格式留到开发阶段固定。该口径不阻塞开发，但 Dev 必须同步 docs/data-model.md 或 docs/api.md，避免只实现唯一约束而缺少可审计的稳定格式说明。
2. 规则通过但全部奖励为 0 时，analysis.md 允许“不写 REWARDABLE 脏记录”，并把是否保留非 REWARDABLE participation 列为待确认。该点与现有奖励计算文档兼容，不阻塞；Dev 不得在未明确状态语义的情况下写 REWARDABLE，也不得生成 grant_order/outbox 或扣 quota。
~~~

## Open Questions

~~~text
1. 0 金额奖励是否保留 participation 运营记录仍未最终定稿；本次开发只能按 analysis.md 的保守边界处理，不进入发奖链路。
2. grant_order.FAILED / RETRY / CLOSED 与 outbox.DLQ / FAILED 的最终状态同步边界属于后续 Outbox/Callback 任务；本任务只要求字段和生命周期可承载。
~~~

## Decision

~~~text
PASS
~~~

## Score

~~~text
90 / 100
~~~

规则：

~~~text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
~~~

需求：2026-05-24-113400-5
