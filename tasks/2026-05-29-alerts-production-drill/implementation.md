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

完成返工最小修复：

- `test.md` 已给出真实 FAIL：`docker compose up -d postgres prometheus alertmanager` 缺少 `prometheus` 服务，且没有 live fault injection / notification receiver 编排，无法验证 firing、route、payload、resolved 和清理闭环。本轮优先按该失败原因修复。
- `commit.md` 仍为模板态，没有真实 BLOCK / FAILED / 提交失败上下文。
- 新增本地/测试环境告警演练手册，覆盖 Worker 停止、Outbox backlog、DLQ 增长、Callback 失败率、DB 写失败/慢查询、Quota 一致性风险 6 类核心故障。
- 按 `review.md` 的 P2 提醒，明确区分 quota 一致性 P0 与普通 quota reserve fail-rate：`ExMarketingQuotaOverDeductRisk` 只验收 `quota_aggregate_conflict|quota_reservation_missing`，普通 `quota_insufficient` / reserve fail-rate 只作为面板或辅助验证。
- 补充 Prometheus 规则样例测试，覆盖 Worker stopped、Callback fail-rate，并增加普通 quota insufficient reserve fail 不触发 P0 的负向用例。
- 补齐 `docker-compose.yml` 的 local live drill stack：`postgres`、`exmarketing-alert-drill`、`prometheus`、`alertmanager`。`exmarketing-alert-drill` 暴露 `/metrics`、scenario trigger/recover、cleanup 和 Alertmanager webhook receiver。
- 新增 `monitoring/prometheus-exmarketing.local.yml` 和 `monitoring/alertmanager-exmarketing.local.yml`，本地 Prometheus 加载真实 ExMarketing alert rules，Alertmanager 将通知发送到本地 drill receiver 并开启 `send_resolved`。
- 新增 `scripts/exmarketing-alert-drill.sh`，可串行触发 6 个核心场景，等待 expected receiver 的 firing/resolved webhook，执行恢复和 cleanup，并写 live report 到 `tasks/2026-05-29-alerts-production-drill/runs/`。
- 调整 Alertmanager 业务路由为 `continue: true`，确保 `ExMarketingDLQGrowing`、`ExMarketingCallbackFailRateHigh` 等业务类 P1 同时路由到 `business-ops` 和 backend oncall，匹配 test-plan 的通知验收口径。
- 在运维手册入口表中加入告警演练手册链接。

## Changed Files

- `docs/runbooks/exmarketing-alert-drill.md`
  - 新增生产告警演练手册，限定本地/测试环境执行。
  - 增加前置校验、6 场景矩阵、Prometheus/Alertmanager 证据查询、演练报告模板、可重复的 promtool 模拟记录和 local compose harness 记录。
- `monitoring/alerts-exmarketing.test.yml`
  - 将样例 oldest eligible age 调整为 180s，使 worker stopped P0 可被样例覆盖。
  - 新增 `ExMarketingWorkerAllStopped` 和 `ExMarketingCallbackFailRateHigh` 期望告警。
  - 新增 `quota_insufficient` reserve fail 不触发 `ExMarketingQuotaOverDeductRisk` 的负向测试。
- `docker-compose.yml`
  - 新增 `exmarketing-alert-drill`、`prometheus`、`alertmanager` 服务，使 `docker compose up -d postgres prometheus alertmanager` 可以带起本地 live drill stack。
- `cmd/exmarketing-alert-drill/main.go`
  - 新增本地安全故障注入 metrics/webhook receiver harness。
- `monitoring/prometheus-exmarketing.local.yml`
  - 本地 Prometheus scrape drill harness，并加载真实 alert rules。
- `monitoring/alertmanager-exmarketing.local.yml`
  - 本地 Alertmanager receiver 指向 drill harness，用于记录 firing/resolved payload。
- `monitoring/alertmanager-exmarketing.yml`
  - 业务类 P0/P1 route 增加 `continue: true`，让业务通知不吞掉 backend oncall 路由。
- `scripts/exmarketing-alert-drill.sh`
  - 新增可执行 live drill 脚本；默认使用 8080，若 8080 被非 drill 服务占用则自动回退 18080。
- `docs/operations.md`
  - 增加 `docs/runbooks/exmarketing-alert-drill.md` 作为生产告警演练入口。
- `tasks/2026-05-29-alerts-production-drill/implementation.md`
  - 记录本轮 Dev 实现、验证和交接说明。

## Behavior Changes

- 业务运行时代码无行为变化。
- 监控规则本身无阈值或表达式变化；只扩展规则测试覆盖面。
- Alertmanager 业务类告警路由现在会继续匹配后续 backend oncall route，避免业务接收方截断 P1 backend 通知。
- 运维文档现在提供可重复执行的本地/测试环境故障注入和演练报告流程。
- Test Agent 可通过 promtool 样例验证 6 个核心告警中的 P0/P1 表达式，并通过手册执行 live drill。

## Notes

已执行验证：

~~~bash
promtool check config monitoring/prometheus-exmarketing.yml
promtool check config monitoring/prometheus-exmarketing.local.yml
promtool check rules monitoring/alerts-exmarketing.rules.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
alertmanager --config.file=monitoring/alertmanager-exmarketing.local.yml --config.check
bash -n scripts/exmarketing-alert-drill.sh
docker compose config --services
EX_MARKETING_DRILL_HOST_PORT=18080 docker compose up -d exmarketing-alert-drill prometheus alertmanager
curl -fsS http://localhost:18080/drill/status
curl -fsS http://localhost:9090/-/ready
curl -fsS http://localhost:9093/-/ready
go test ./...
go build ./...
~~~

结果均为 PASS。本机已有一个非 drill ExMarketing 进程占用 `localhost:8080`，因此 compose smoke 使用 `EX_MARKETING_DRILL_HOST_PORT=18080` 验证；默认 compose 仍保留 `8080` 以满足 Test Agent 的 acceptance command。

交给 Test Agent 的重点：

- 必须重新执行 `go test ./...` 和 `go build ./...`。
- 建议同时执行 promtool 和 alertmanager 配置校验。
- 若执行 live drill，只能使用本地或测试环境，并使用隔离 `drill_id`、tenant、channel、activity/runId；禁止生产破坏性注入。
- Quota 验证必须按手册区分一致性 P0 与普通 reserve fail-rate 面板验证。

需求：2026-05-29-alerts-production-drill
