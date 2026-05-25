# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本需求风险等级高，测试重点覆盖 HTTP / WEBHOOK outbox 自动投递、失败分类、指数退避、最大重试进入 DLQ、人工 replay / close、tenant 过滤、审计和 MQ 非本期范围。

测试分层：

1. Worker 单元测试：使用本地 `httptest.Server` 验证 HTTP 状态码、`Retry-After`、重试计数、`next_retry_at` 和 DLQ 流转。
2. Repository / API 测试：使用内存仓储验证 DLQ tenant 查询、replay、close、缺少 operator / reason、`DLQ_CLOSED` 禁止 replay 和审计记录。
3. 回归测试：保留现有活动创建、事件处理、quota、幂等、workflow、PostgreSQL 仓储测试，确认 retry / replay 不重新执行活动规则、quota 扣减、participation、grant_order 或 outbox 创建。
4. 全量命令：必须执行 `go test ./...` 和 `go build ./...`。

## Test Cases

1. Worker disabled smoke

- 不设置 `EX_MARKETING_OUTBOX_WORKER=true` 时只启动 HTTP API，不启动 worker。
- 上报匹配活动事件后仍生成 participation、grant_order、outbox，outbox 保持 `PENDING`。

2. HTTP / WEBHOOK 2xx 成功

- 准备 `target_channel=HTTP` 或 `WEBHOOK`、合法 http(s) `target_ref`、合法 JSON payload。
- 目标返回 2xx。
- 期望 outbox 进入 `SENT`，清空 `next_retry_at` 和错误信息。

3. 可重试失败

- 目标返回 HTTP 408、429、5xx，或模拟连接失败 / 超时。
- 期望 outbox 不丢失，`retry_count` 加 1，`send_status=FAILED`，写入脱敏截断后的 `error_message`，`next_retry_at` 使用本次失败时间作为基准。
- 第 1 次失败基础延迟 30s，第 2 次 60s，第 3 次 120s，最大 30m，jitter 只校验在正负 20% 区间内。

4. HTTP 429 Retry-After

- 目标返回 429 且 `Retry-After` 为秒数或 HTTP date。
- 期望 `next_retry_at` 优先按 `Retry-After` 计算。
- `Retry-After` 缺失或不可解析时退回指数退避。

5. 不可重试失败

- 目标返回 HTTP 400、401、403、404、409、422。
- payload 非合法 JSON。
- `target_ref` 非 http(s) URL。
- 期望直接进入 `DLQ`，自动投递停止，记录错误摘要。

6. 最大重试进入 DLQ

- 已有 `retry_count=4` 的 outbox 再次遇到可重试失败。
- 期望更新为 `retry_count=5`，状态进入 `DLQ`，`next_retry_at` 不再作为自动重试依据。

7. MQ 非本期范围

- 准备 `target_channel=MQ` outbox。
- 期望 worker 不 claim、不投递、不按本任务 retry / DLQ 处理。

8. DLQ 查询

- 多 tenant 准备 `DLQ` 和 `DLQ_CLOSED` outbox。
- `GET /admin/v1/outbox/dlq?tenantId=...` 只返回当前 tenant 的 `DLQ`。
- `includeClosed=true` 时可区分返回 `DLQ_CLOSED`。
- 返回字段至少包含 id、tenant、target channel、target ref、retry count、error message、updated time 和 status。

9. DLQ replay

- 对当前 `DLQ` 执行 replay，带 tenantId、operator、reason。
- 期望同一 outbox 变为 `PENDING`，不创建新的 participation、grant_order 或 outbox。
- payload 不变，保留原 `commandId` / `bill_no`。
- 写 audit，包含 operator、reason、replayed_at、前后状态。

10. DLQ close

- 对当前 `DLQ` 执行 close，带 tenantId、operator、reason。
- 期望同一 outbox 变为 `DLQ_CLOSED`。
- 写 audit，包含 operator、reason、closed_at、前后状态。

11. Replay / close 状态与参数校验

- 缺少 operator 或 reason 返回失败。
- replay `DLQ_CLOSED` 返回失败。
- close 非 `DLQ` 返回失败。
- 跨 tenant replay / close 返回不可见或失败。

12. 幂等链路回归

- 重复标准事件仍返回 duplicate，不新增 participation、grant_order、outbox，不重复 quota 扣减。
- replay 只重新发送原 outbox payload，不重新执行活动匹配、规则判断、奖励计算或 quota 扣减。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

本地 smoke 可用 `httptest.Server` 或临时 HTTP endpoint 模拟：

- 2xx：成功发送。
- 400 / 422：不可重试并进入 DLQ。
- 429 + `Retry-After`：按 header 计算下一次投递。
- 500 / 503：按指数退避重试，达到 5 次进入 DLQ。

管理端 smoke：

- `GET /admin/v1/outbox/dlq?tenantId=site_001`
- `POST /admin/v1/outbox/{id}/replay`
- `POST /admin/v1/outbox/{id}/close`
- `GET /admin/v1/audit`

## Decision

~~~text
PASS
~~~

需求：2026-05-24-outbox-worker-dlq
