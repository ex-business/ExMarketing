# Implementation

## Summary

已按 `task.md` / `analysis.md` / `review.md` / `test-plan.md` 完成 engine metrics and alerts 的最小实现。

本轮开始前检查：

- `review.md` 为 `PASS`。
- `test-plan.md` 当前为 `PASS`，包含 Review P2 要求的 quota reserve-only fail rate 检查。
- `test.md` 和 `commit.md` 未包含真实 `FAIL` / `BLOCK` / `FAILED` 上下文，只有模板或待提交内容，因此无可优先修复的失败原因。

## Changed Files

- `internal/metrics/metrics.go`
  - 新增轻量 Prometheus text format recorder，支持 Counter、Gauge、Histogram、标签转义和按指标名清理状态类 Gauge。
- `internal/httpapi/handler.go`
  - 新增 `GET /metrics`。
  - Event API 记录 `exmarketing_event_intake_total` 和 `exmarketing_event_intake_latency_seconds`。
  - Reward callback 记录 `exmarketing_reward_callback_total` 和 `exmarketing_reward_callback_latency_seconds`。
- `internal/engine/service.go`
  - Service 接入 metrics registry。
  - Metrics 抓取前刷新 outbox backlog / DLQ Gauge。
  - Quota reserve 路径记录 `exmarketing_quota_attempt_total` 和 `exmarketing_quota_fail_total`，并加入 `action="reserve"`，避免 confirm / release 稀释 fail rate。
- `internal/marketing/model.go`
  - 新增 `OutboxBacklogMetric`。
- `internal/marketing/memory_repository.go`
  - 新增内存仓储 outbox backlog / oldest eligible age 快照。
- `internal/postgres/repository.go`
  - 新增 PostgreSQL outbox backlog / oldest eligible age 快照查询。
- `internal/worker/outbox_worker.go`
  - Worker 记录 claim、in-flight、claim latency、reward command delivery、reward command terminal latency、DLQ total 指标。
- `internal/httpapi/handler_test.go`
  - 新增 `/metrics` 测试，覆盖 event failure counter 和 outbox backlog gauge 输出。
- `docs/operations.md`
  - 新增 metrics endpoint、核心指标表、告警阈值和敏感标签约束。

## Behavior Changes

- 服务现在暴露 `GET /metrics`，返回 Prometheus text format。
- Outbox backlog / DLQ Gauge 在 metrics 抓取时从 repository 当前状态刷新；PostgreSQL 模式下可在进程重启后恢复 Gauge 状态。
- Event intake 指标覆盖 success、duplicate、failed、rejected、auth_failed 等结果。
- Callback 指标覆盖 accepted、duplicate、conflict、rejected、failed 等结果。
- Worker 指标覆盖 claim success/empty/failed/conflict、in-flight、claim latency、reward command sent/retry/dlq 和单次 delivery latency。
- 指标标签使用有限枚举和安全默认值，不使用 eventId、bizId、subjectId、billNo、commandId、targetRef、payload 或原始错误文本。

## Validation

已执行：

```bash
go test ./...
go build ./...
```

结果：

```text
PASS
```

## Notes

- `grant_channel` 在 callback HTTP handler 当前无法从 callback payload 直接确定，指标使用 `unknown` 作为安全低基数默认值。
- Counter 为进程生命周期累计值，重启清零；Gauge 类 outbox backlog / DLQ 由当前仓储状态刷新。
- 告警阈值已写入 `docs/operations.md`，实际生产阈值后续可按真实流量校准。

需求：2026-05-27-engine-metrics-alerts
