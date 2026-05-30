# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

完成告警规则、通知路由、抑制策略、消息模板、runbook、Fact Query 指标契约，以及本轮 PostgreSQL outbox smoke 失败的最小修复。

本轮返工优先处理 `test.md` 中的 `Result: FAIL`：

- `go run ./cmd/engine-e2e-postgres` 失败在 `OUTBOX_SEND_FAILED_CASE`，outbox 被 claim 后停留在 `SENDING`，reward stub 未收到第二条 command。
- `go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke` 失败为 6 条 outbox 全部停留在 `SENDING`。

`commit.md` 仍是模板内容，未发现实际 `BLOCK / FAILED` 提交上下文。

## Changed Files

- `monitoring/alerts-exmarketing.rules.yml`
  - 新增 ExMarketing P0/P1/P2/P3 Prometheus alert rules。
  - 覆盖 DLQ 持续增长、callback 失败率高、outbox backlog、worker 全停、DB 核心写失败、quota 一致性风险、event intake 错误率、claim latency、reward retry、Fact Query 失败率和 scrape target down。
  - DLQ/outbox/worker 保留 `tenant_id,target_channel` 聚合维度；callback 保留 `tenant_id,grant_channel`；Fact Query 保留 `tenant_id,fact_code`。
- `monitoring/alertmanager-exmarketing.yml`
  - 新增 P0/P1/P2/P3 路由、重复提醒间隔、接收组和 escalation-ready receiver。
  - 新增 DB P0 对下游 outbox/callback/worker/event/Fact Query 泛洪告警的抑制规则，优先按 `env,tenant` 精确抑制，`tenant="global"` 时才按 `env` 退化。
  - 新增 P0/P1 对同维度 P2/P3 的抑制。
- `monitoring/alertmanager-exmarketing.tmpl`
  - 新增统一消息模板字段：`service`、`env`、`tenant`、`channel`、`severity`、`alertname`、`owner`、`summary`、`description`、`dashboard`、`runbook`、`first_seen`、`current_value`、`threshold`。
- `monitoring/alerts-exmarketing.test.yml`
  - 新增 promtool alert rule 单测样例，用于覆盖 DLQ、outbox backlog、DB write failure、quota risk 等关键规则触发。
  - 返工补齐 `ExMarketingDLQGrowing`、`ExMarketingOutboxBacklogHigh`、`ExMarketingDBWriteFailure`、`ExMarketingQuotaOverDeductRisk` 的完整 `exp_labels` 和 `exp_annotations`，包括原始定位维度、`dashboard`、`runbook`、`summary`、`description`、`current_value`、`threshold`。
- `monitoring/prometheus-exmarketing.yml`
  - 引入 `alerts-exmarketing.rules.yml`，并添加本地 `env` external label。
- `docs/runbooks/exmarketing-alerts.md`
  - 新增所有告警的 runbook、维护窗口静默口径和升级策略。
- `docs/operations.md`
  - 补充告警规则、Alertmanager 路由、抑制、静默、promtool/alertmanager 校验命令和 Fact Query 指标说明。
- `internal/fact/client.go`
  - 新增 `exmarketing_fact_query_total` 和 `exmarketing_fact_query_latency_seconds` 埋点。
  - Result 枚举包括 `success`、`cache_hit`、`failed`、`rejected`、`timeout`、`mapping_failed`。
  - Reason 使用枚举化原因，不写入原始错误文本。
- `internal/metrics/metrics.go`
  - 新增 Fact Query 指标 HELP/TYPE 输出。
- `internal/fact/client_test.go`
  - 新增 Fact Query 成功和 cache hit 指标输出测试。
- `internal/worker/outbox_worker.go`
  - 修复 outbox worker 在 claim 成功后，如果 grant order 无法从 `PENDING/RETRY` 推进到 `SENDING`，直接返回导致 outbox 永久停在 `SENDING` 的问题。
  - 现在该路径会把 outbox 释放为可立即重试的 `FAILED`，写入 `OUTBOX_CLAIM_RELEASED` audit，记录 claim conflict 指标和 sanitized error，避免 PostgreSQL smoke 中 reward command 永久不发送、grant order 保持 `PENDING`、quota reservation 保持 `FROZEN`。
- `internal/worker/outbox_worker_test.go`
  - 新增 `TestOutboxWorkerReleasesClaimWhenGrantCannotAdvance`，覆盖 grant 状态推进冲突时 outbox claim 被释放、retry_count 增加和 audit 写入。

## Behavior Changes

- Prometheus 配置会加载新的 ExMarketing 告警规则文件。
- Fact Query 每次调用会记录低基数 Prometheus counter/histogram，用于 `ExMarketingFactQueryFailRateHigh` P2 告警。
- Outbox worker 在 grant 状态推进失败时不再把已 claim 的消息留在 `SENDING`；消息会进入 `FAILED` 并在下一轮 worker tick 立即重试。
- 新增的 Alertmanager 配置只使用抽象 webhook receiver，例如 `backend-oncall`、`business-ops`、`dba-infra`、`platform-observability`，未绑定具体公司 IM。
- DB 核心写失败 P0 规则只匹配 `event_inbox_write|participation_write|grant_order_write|quota_reserve_write|quota_finalize_write|outbox_write|outbox_dlq_write|callback_write`，不会把 outbox/DLQ 统计查询失败误报为核心写失败。

## Notes

- 已执行并通过：

~~~bash
docker run --rm --entrypoint promtool -v "$PWD:/work:ro" -w /work/monitoring prom/prometheus:v2.55.1 test rules alerts-exmarketing.test.yml
go test ./internal/worker -run 'TestOutboxWorker(AdvancesGrantOrderState|ReleasesClaimWhenGrantCannotAdvance)' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-e2e-postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke
go test ./...
go build ./...
~~~

- 本轮只针对 PostgreSQL outbox stuck in `SENDING` 的失败路径做最小修复，未修改告警规则表达式、Alertmanager 路由或业务状态机语义。
- 修复后可继续交给 Test Agent 重新执行完整测试计划。

需求：2026-05-29-alerts-rules-routing
