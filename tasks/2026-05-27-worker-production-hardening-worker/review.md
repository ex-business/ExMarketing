# Review

## Agent 指令

你是 Review Agent。请审核 `tasks/2026-05-27-worker-production-hardening-worker/analysis.md`。

不改代码，不提交。

## Findings

~~~text
P0:
- 无。

P1:
- 无。

P2:
- analysis.md 已将监控最低口径定义为“结构化日志 + 可测试的 worker 统计快照 / recorder”，但未进一步指定统计快照的读取边界由测试专用注入、内部接口还是现有 metrics 抽象承接。该项不影响当前需求 PASS，因为 analysis.md 已明确如果项目已有 metrics 抽象应复用，且验收要求关键计数可被测试验证；后续 Test Plan / Dev 阶段需要把这个观测边界落到具体测试入口。
~~~

## Review 结论

`analysis.md` 已覆盖任务原文要求的核心生产化能力：

- Worker 开启条件、常驻运行和空批次不退出。
- 停止信号后的停止新 claim、shutdown drain、drain timeout 和超时后的可恢复语义。
- 崩溃 / 重启后的 stale `SENDING` 恢复，且不重新执行业务计算、quota 扣减、grant_order / outbox 创建。
- PostgreSQL 多 worker 并发 claim 的唯一性约束，以及 retry 到期、`SENT` / `DLQ` / `DLQ_CLOSED` 不参与普通 claim 的边界。
- claim 延迟、DLQ 堆积、in-flight、成功 / 失败 / DLQ / stale 恢复等可观测要求。
- 结构化日志字段和敏感信息脱敏约束。
- 非目标范围清楚，避免把外部发奖幂等、完整告警平台、DLQ API 改造、业务规则重设计混入本任务。

评分为 90/100，符合 Review 规则的 PASS 条件。

## Open Questions

以下问题不阻塞：

1. 正式指标暴露形式由 Prometheus、日志规则、云监控还是项目内部 recorder 承接。
2. `worker_id` 的生产来源是否需要固定为 hostname / pid / 配置项。
3. Worker 健康是否后续接入 `/healthz` 或 readiness。

## Decision

~~~text
PASS
~~~

规则：

~~~text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
~~~

需求：2026-05-27-worker-production-hardening-worker
