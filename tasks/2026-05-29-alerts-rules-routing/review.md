# Review: 2026-05-29-alerts-rules-routing

## Findings

未发现 P0/P1 阻塞问题。

上一轮两个 P1 已被有效收敛：

- DB 写失败 P0 不再假设现有 `exmarketing_db_operation_total` 已覆盖核心写路径；`analysis.md:212-229` 明确要求补齐或映射 DB write metrics，并在 `analysis.md:408`、`analysis.md:470` 将“仅查询指标覆盖”列为验收失败。
- Fact Query P2 不再以文档占位通过；`analysis.md:231-270` 给出 `exmarketing_fact_query_total` / `exmarketing_fact_query_latency_seconds` 契约，`analysis.md:409`、`analysis.md:472` 明确无指标不得通过。

## P2

### P2: Worker 全停规则的 `worker scrape/up` 口径仍偏模糊

- 位置：`analysis.md:302`, `analysis.md:426`
- 问题：`ExMarketingWorkerAllStopped` 的主口径“claim 成功速率为 0 且存在 eligible backlog”是可实现的，但 `或 worker scrape/up 不健康` 没有定义具体 worker 级健康指标。当前运维文档只定义服务级 scrape 样例和业务指标，未定义 per-worker `up` 或 heartbeat 指标。如果 Dev Agent 直接按 `up{job="exmarketing"} == 0` 解释，会与 `ExMarketingScrapeTargetDown` 重叠；如果按 worker_id 解释，又缺少指标契约。
- 建议：实现时将 P0 主规则限定为 `eligible backlog > 0` 且 `worker_claim_total{result="success"}` 同维度为 0；worker 级健康若要进入规则，应补一个低基数 heartbeat/inflight/claim-attempt 契约，或只作为 `ExMarketingScrapeTargetDown` 的下游说明。

### P2: DB 抑制下游告警需要避免过宽匹配

- 位置：`analysis.md:360`, `analysis.md:423`, `analysis.md:480`
- 问题：分析要求 DB 写失败抑制 outbox/callback/worker/event/Fact Query 泛洪，匹配标签为 `service, env`，`tenant` 可选。这个方向正确，但如果实现成全服务级一刀切 inhibition，可能在 DB 故障期间隐藏同环境内独立的 Fact Provider outage、外部 reward channel outage 或业务接入错误趋势。
- 建议：实现抑制时优先按 `service, env, tenant` 或根因标签精确匹配；只有明确无 tenant 维度的 DB 根因才退化为 `service, env`。同时保留分析已要求的 quota P0、DB P0 和必要 DLQ 总览不被抑制。

## Open Questions

- Worker 级健康是否需要新增专门指标，还是本任务只基于 claim/backlog 推导 worker 全停？
- 生产 Alertmanager 路由中不同 severity 的 `repeat_interval` 是否由 child routes 实现，还是由等价告警平台模板实现？这不影响需求进入研发。

## Score

```text
90
```

## Decision

```text
PASS
```

未发现 P0/P1，且 Score >= 80，不触发 BLOCK。
