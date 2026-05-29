# Review

## Agent 指令

你是 Review Agent。请审核 `tasks/2026-05-27-engine-metrics-alerts/analysis.md`。

不改代码，不提交。

## Findings

~~~text
P0:
- 无。

P1:
- 无。

P2:
- Quota fail rate 的口径仍有一个实现期需收紧的细节：analysis.md 将 `exmarketing_quota_attempt_total` 定义为“预留 / 确认 / 释放尝试结果”，但失败率公式直接使用 `rate(exmarketing_quota_attempt_total{result!="skipped"}[5m])` 作为分母。若实现时把 confirm / release 也纳入同一分母，`quota_exceeded` 这类 reserve 阶段失败会被稀释，告警可能低估活动额度耗尽或异常拒绝。该项不影响当前 Analysis PASS，因为文档已明确至少区分 quota type、result、reason，并在验收标准中要求 quota 成功和 exceeded 可验证；后续 Test Plan / Dev 阶段应补充 `action=reserve|confirm|release` 标签，或明确 fail rate 只统计 reserve attempts。
~~~

## Review 结论

`analysis.md` 已覆盖任务原文要求的核心指标和告警口径：

- Event intake 成功 / 失败、重复、鉴权、payload、幂等、规则拒绝和 quota exceeded 的观测口径。
- Quota fail rate、outbox backlog、DLQ count、callback fail rate、worker claim latency、reward command latency 的指标名、类型、标签、单位和语义。
- Prometheus `/metrics` 或等价机器可读接口的最低交付口径。
- PostgreSQL 模式下 backlog / DLQ / oldest age 必须来自数据库状态快照，避免进程重启后丢失状态。
- Counter / Gauge / Histogram 的边界、敏感信息脱敏、高基数标签限制和错误原因枚举化。
- 默认告警阈值、窗口、严重级别、降噪规则和常见排查方向。
- 非目标范围清楚，未把完整监控平台、Grafana / PagerDuty、长期指标存储、tracing 或公网 metrics 鉴权混入本任务。
- 验收标准可执行，包含 metrics endpoint、文档、无流量、重启恢复、worker 未开启、PostgreSQL smoke / 集成测试和 `go test ./...` / `go build ./...`。

Analysis 自评分为 92/100。Review 认为该评分合理，满足“无 P0/P1 且 Score >= 80”的 PASS 条件。

## Open Questions

以下问题不阻塞：

1. 正式监控平台最终使用 Prometheus、云监控还是内部 recorder。
2. Metrics endpoint 的生产鉴权和网络暴露策略由后续部署 / 安全任务确定。
3. `activity_id` 作为 quota 标签是否需要在高基数环境中配置降级为 `activity_type` / `activity_code`。
4. Quota fail rate 实现时是否增加 `action` 标签，或用独立 reserve attempt 指标固定分母。

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

需求：2026-05-27-engine-metrics-alerts
