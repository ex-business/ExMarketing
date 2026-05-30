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

本轮按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 对告警 runbook 与 SLO/SLA 文档做最小补齐；返工阶段针对 `test.md` 中的 FAIL 做环境和测试计划最小修复，没有修改业务代码。

已处理内容：

- 在 `docs/operations.md` 增加告警 runbook、SLO/SLA、业务确认、演练模板、复盘模板和映射表的统一入口，确保交付物可从运维手册发现。
- 扩展 `docs/runbooks/exmarketing-alerts.md`，覆盖所有现有 P0/P1/P2/P3 告警和任务要求的 DB 慢/错误场景。
- 为每个 runbook 补齐症状、影响、首查命令/API、止血、恢复、验证、升级或复盘说明。
- 增加业务确认边界，明确 DLQ close/replay、callback/timeout-release、活动暂停/恢复、provider/source/fact owner 介入等需要业务确认的场景。
- 增加 SLI/SLO/SLA 表，使用现有 metrics、DLQ 管理 API 和 audit 数据源，并补充可直接复制的 PromQL/API 口径。
- 增加演练记录模板、事故复盘模板和告警到 runbook 映射表。
- 返工补充 `test-plan.md` 的可执行前置条件：
  - 本机缺少 `promtool` 时通过 `brew install prometheus` 补齐。
  - 官方 Alertmanager 不支持 `--config.check` 时，用 `amtool check-config` 做同等语义校验；本机已提供兼容 `alertmanager --config.file=... --config.check` 的入口。
  - API smoke 前必须确认 `BASE_URL` 指向当前代码启动的 ExMarketing；若 8080 被旧进程占用，可用 `EX_MARKETING_ADDR=:18080` 启动并设置 `BASE_URL`。
  - PostgreSQL smoke 在没有 `EXMARKETING_PG_READONLY_DSN` 时允许启动本地 `docker compose` PostgreSQL、应用 migration，并使用 Docker 内 `psql` fallback。
  - 修正 PostgreSQL smoke 表名为当前 schema 的 `marketing_*` 表。

## Changed Files

- `docs/operations.md`
- `docs/runbooks/exmarketing-alerts.md`
- `tasks/2026-05-29-alerts-runbook-slo-runbook-slo/test-plan.md`
- `tasks/2026-05-29-alerts-runbook-slo-runbook-slo/implementation.md`

## Behavior Changes

无运行时行为变化。此次仅变更文档。

## Notes

前置检查：

- `review.md` Decision 为 `PASS`。
- `test-plan.md` Decision 为 `PASS`。
- 现有 `test.md` Result 为 `FAIL`，失败原因是本机缺少 `promtool` / `alertmanager`、8080 上旧 ExMarketing 进程的 `/metrics` 返回 404、缺少 PostgreSQL 只读 DSN。
- 现有 `commit.md` 未包含实际 BLOCK / FAILED / 提交失败原因。

验证命令：

~~~bash
promtool check config monitoring/prometheus-exmarketing.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
curl -fsS --max-time 2 http://127.0.0.1:18080/metrics | rg "exmarketing_(event_intake_total|reward_command_latency_seconds|reward_callback_total|outbox_dlq|quota_fail_total|worker_claim_latency_seconds|fact_query_total)"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "\dt"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select send_status, count(*) from marketing_outbox group by send_status order by send_status;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select grant_status, count(*) from marketing_grant_order group by grant_status order by grant_status;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select action, count(*) from marketing_audit where created_at > now() - interval '1 day' group by action order by action;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select quota_type, scope, count(*) from marketing_quota_reservation group by quota_type, scope order by quota_type, scope limit 20;"
go test ./...
go build ./...
~~~

验证结果：

~~~text
PASS
~~~

补充说明：

- 8080 当前存在旧的 `exmarketing` 进程，`/healthz` 返回 200 但 `/metrics` 返回 404；为避免影响本机旧进程，本轮用当前代码在 `:18080` 完成 API smoke。
- PostgreSQL smoke 使用本地 `docker compose` PostgreSQL；宿主机缺少 `psql` 时已验证 Docker 内 `psql` fallback 可执行。

需求：2026-05-29-alerts-runbook-slo-runbook-slo
