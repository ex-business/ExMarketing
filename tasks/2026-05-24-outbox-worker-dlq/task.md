# Task

## 标题

Outbox Worker 重试与 DLQ

## 需求 ID

2026-05-24-outbox-worker-dlq

## 背景

来源：22-独立活动营销中台设计。实现 marketing_outbox 的 HTTP/MQ/Webhook 发送、指数退避重试、最大重试次数、DLQ、人工重放和关闭。验收：业务记录与消息发送解耦；发送失败不丢发奖指令；DLQ 可查看和重放。

## Owner 补充决策

### 本期范围

本期只实现 HTTP / Webhook Outbox Worker。

MQ Command Out 不在本期实现范围内，拆分为后续独立任务。本期如果遇到 `target_channel = MQ`，不进行投递，不把 MQ sender / MQ retry / MQ DLQ / MQ replay / MQ close 作为本任务验收项。

### 重试策略

Outbox Worker 使用指数退避重试。

默认参数：

- 最大重试失败次数：5 次。
- 初始延迟：30 秒。
- 退避倍率：2。
- 最大延迟：30 分钟。
- jitter：允许正负 20% 抖动，避免集中重试。
- 时间基准：以本次发送失败写入时间作为下一次 `next_retry_at` 的计算基准。
- 验收容差：`next_retry_at` 允许正负 2 秒的测试容差，jitter 验收只校验落在允许区间内。

每次失败必须更新：

- `retry_count`
- `next_retry_at`
- `error_message`
- `updated_at`

可重试失败：

- 网络连接失败。
- DNS / TLS 错误。
- 请求超时。
- HTTP 408。
- HTTP 429。
- HTTP 5xx。

不可重试失败：

- HTTP 400 / 401 / 403 / 404 / 409 / 422：视为业务或配置错误，直接进入 DLQ。
- payload 格式错误：直接进入 DLQ。
- `target_ref` 非法：直接进入 DLQ。

HTTP 2xx 视为发送成功。本期不校验响应体业务字段。

HTTP 429 如果存在 `Retry-After`，优先使用 `Retry-After`；如果不存在或不可解析，则使用指数退避。

达到最大重试失败次数后进入 DLQ，自动投递停止。

### DLQ 状态语义

Outbox 增加机器可判定的关闭语义，使用明确状态 `DLQ_CLOSED` 表示人工关闭后的最终状态。

状态流转：

```text
PENDING -> SENDING -> SENT
PENDING -> SENDING -> FAILED -> PENDING
FAILED -> DLQ
DLQ -> PENDING      # replay
DLQ -> DLQ_CLOSED   # close
```

规则：

- `DLQ` 可以 replay。
- `DLQ_CLOSED` 不允许 replay。
- close 只允许对 `DLQ` 执行。
- replay 只允许对 `DLQ` 执行。
- close 必须记录 operator、reason、closed_at。
- replay 必须记录 operator、reason、replayed_at。
- replay 不重新执行活动规则、奖励计算、quota 扣减、participation 创建、grant_order 创建。
- replay 只重新发送原 outbox payload。
- retry / replay 必须复用原 `commandId` / `bill_no`，依赖外部发奖系统幂等。
- replay 后如果再次失败，继续按同一重试和 DLQ 规则处理。
- close 与自动重试并发时，以状态条件更新为准；只有当前仍为 `DLQ` 才允许 close 成功。
- replay 与自动重试并发时，以状态条件更新为准；只有当前仍为 `DLQ` 才允许 replay 成功。

## 目标

- 实现 HTTP / Webhook Outbox Worker，使业务记录与外部发奖投递解耦。
- 外部目标失败、超时、不可达或 worker 崩溃时，不丢失发奖指令。
- 支持指数退避重试、最大重试失败次数、DLQ、人工 replay 和人工 close。
- 支持 tenant 范围内查看 DLQ，并能区分 `DLQ` 与 `DLQ_CLOSED`。

## 范围

- HTTP / Webhook outbox claim。
- HTTP / Webhook 发送。
- 指数退避 retry。
- 最大重试失败次数后进入 DLQ。
- DLQ 查询。
- DLQ replay。
- DLQ close。
- replay / close 审计记录。
- 相关单元测试和必要的本地 smoke 验证。

## 非目标

- MQ Command Out。
- MQ sender / retry / DLQ / replay / close。
- 生产级告警平台。
- 运维导出。
- 多 broker 适配。
- 外部发奖系统自身幂等实现。

## 风险等级

高

## 验收标准

- 创建符合当前 API 边界的 `AUTO + HTTP` 或 `AUTO + WEBHOOK` 活动并上报标准事件后，业务响应不依赖外部发奖系统实时可用，且生成 participation、grant_order、outbox。
- 未开启 `EX_MARKETING_OUTBOX_WORKER` 时，outbox 保持待发送且不会被自动投递。
- 开启 worker 后，目标返回 HTTP 2xx 时，同一 outbox 最终进入发送成功状态。
- 目标返回可重试失败、连接失败或超时时，同一 outbox 不丢失，失败次数、下一次重试时间和错误摘要可查询。
- `next_retry_at` 按 owner 确认的指数退避规则计算。
- 达到最大重试失败次数后，同一 outbox 进入 `DLQ`，自动投递停止。
- 不可重试失败直接进入 `DLQ`。
- DLQ 查询支持 tenant 范围过滤，并返回 outbox id、tenant、target channel、target ref、retry count、error message、updated time 和当前状态。
- 对 `DLQ` 执行 replay 后，同一 outbox 使用原 payload、原 `commandId` / `bill_no` 重新进入可投递流程。
- 对 `DLQ` 执行 close 后，同一 outbox 进入 `DLQ_CLOSED`。
- `DLQ_CLOSED` 不允许 replay。
- replay / close 缺少 operator 或 reason 时必须失败。
- replay / close 必须写审计记录。
- retry / replay 不重新执行活动匹配、规则判断、奖励计算、quota 扣减、participation 创建或 grant_order 创建。
- `target_channel = MQ` 不作为本期验收项。
