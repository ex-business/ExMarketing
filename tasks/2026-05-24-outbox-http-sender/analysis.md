# Analysis

## 需求 ID

2026-05-24-outbox-http-sender

## 背景目标

ExMarketing 通过标准事件生成 participation、grant_order 和 marketing_outbox，外部发奖系统只应通过 outbox command 接收发奖指令。当前需求聚焦 marketing_outbox 的 HTTP / WEBHOOK 自动投递：将已生成的发奖指令按目标地址发送给外部系统，并根据发送结果更新 outbox 状态、错误信息和后续处理依据。

目标是补齐最小闭环：

- 事件处理成功后生成 outbox，但发奖投递与事件主链路解耦。
- Outbox sender 开启后，只投递 HTTP / WEBHOOK 类型发奖指令。
- HTTP 2xx 视为发送成功并标记 SENT。
- HTTP 4xx、5xx、网络错误、超时等发送失败必须被记录，不得丢失 outbox。
- 多 worker 或重复扫描场景下，同一 outbox 不得被重复并发投递。

## Review 反馈处理

- `review.md` 中 P0/P1/P2 均为空，未给出具体打回问题；本轮无需逐项修复具体缺陷。
- `review.md` 中 `Open Questions` 为“待填写”，本轮已补齐明确的默认最小口径：仅处理 HTTP / WEBHOOK；MQ 非本期；2xx 成功；非 2xx、网络错误、超时均为失败；失败至少记录状态、错误摘要、重试计数和下一次可处理时间。
- `review.md` 中 Decision 仍为模板 `PASS / BLOCK`，本轮以完整需求、边界、异常、验收标准和评分替换上一版空分析，关闭模板性 BLOCK 风险。

## Analysis 自检修正

- 上一版 `analysis.md` 的“现状、目标理解、方案、改动范围、风险点、验收方式”均为“待填写”；本轮已改为可审核的需求说明、输入输出、业务规则、异常场景、非目标范围、风险和验收标准。
- 上一版 Score 为 `0`，低于 80；本轮按评分规则重新评估为 88。
- 上一版 Decision 为 `PASS / BLOCK` 占位；本轮在给出最小可验收口径后判定 PASS。
- 上一版未说明失败分类、并发投递、状态流转、HTTP 目标边界、超时和非目标范围；本轮全部补齐。

## 参与方

- ExMarketing 事件处理链路：负责在活动命中、规则通过、quota 扣减和 grant_order 创建后，原子生成 marketing_outbox。
- Outbox sender：消费待发送 outbox，负责 HTTP / WEBHOOK 投递和状态更新。
- 外部发奖系统：接收发奖 command，返回 HTTP 响应。
- 运营 / 开发排查人员：通过数据库或已有运维接口查看 outbox 状态、错误信息和 DLQ。

## 输入

Outbox sender 的输入是已持久化的 marketing_outbox 记录，至少依赖以下业务字段：

- `id`：outbox 唯一标识。
- `tenant_id`：租户边界。
- `message_type`：消息类型，当前按发奖指令处理。
- `target_channel`：目标通道；本期只处理 `HTTP` 和 `WEBHOOK`。
- `target_ref`：外部发奖系统 HTTP(S) 目标地址。
- `payload_json`：发送给外部系统的 JSON body。
- `send_status`：发送状态。
- `retry_count`：已失败 / 已重试次数。
- `next_retry_at`：下一次允许 sender 处理的时间。
- `error_message`：最近一次错误摘要。

默认最小口径：

- 只扫描 `PENDING` 或已失败且达到 `next_retry_at` 的 HTTP / WEBHOOK outbox。
- `target_channel=MQ` 或其他未知通道不在本任务投递范围内，不应被 HTTP sender 发送。
- `target_ref` 为空、不是 HTTP(S) URL、payload 不是合法 JSON 或目标地址无法访问，均按发送失败处理，不丢弃 outbox。

## 输出

Outbox sender 对每条被处理的 outbox 输出以下状态结果之一：

- `SENT`：外部目标返回 HTTP 2xx，表示发奖指令已被目标系统接受。
- `FAILED`：本次发送未成功，但仍保留后续自动重试或人工处理空间。
- `DLQ`：本次失败已满足不可自动继续处理条件，需要人工排查或后续 replay/close 流程处理。

每次失败至少输出：

- 更新后的 `send_status`。
- 增加后的 `retry_count`。
- 可用于后续处理的 `next_retry_at`，若已进入 DLQ 则不再依赖自动重试时间。
- 脱敏且长度受控的 `error_message`，包含 HTTP 状态码、超时、网络错误或响应错误摘要。
- `updated_at` 更新时间。

## 业务规则

1. Outbox 是发奖指令的唯一自动投递来源，禁止绕过 grant_order / outbox 直接调用外部发奖系统。
2. 事件主链路生成 participation、grant_order、outbox 后即完成业务记录落库；外部发奖系统不可用不得反向破坏已提交的业务记录。
3. Sender 只有在显式开启 outbox worker 时才自动投递；未开启时，outbox 保持待处理状态。
4. HTTP / WEBHOOK 投递使用 `target_ref` 作为目标地址，使用 `payload_json` 作为请求体。
5. HTTP 响应码 2xx 视为成功；成功后 outbox 标记为 `SENT`，不再被自动投递。
6. HTTP 4xx、5xx、网络连接错误、DNS 错误、TLS 错误、请求超时、响应读取失败均视为失败，必须更新状态和错误信息。
7. 失败不得删除 outbox，不得重新执行活动匹配、规则判断、奖励计算、quota 扣减、participation 创建或 grant_order 创建。
8. 同一 outbox 的 replay / retry 必须复用原 payload 和原 command 语义，不能生成新的业务记录或新的 outbox。
9. 多 worker 并发时，必须先独占 claim 再发送；同一 outbox 在同一处理窗口内最多被一个 worker 发送。
10. 进入 `SENT`、`DLQ` 或后续人工关闭状态的 outbox，不应再被普通自动扫描投递。
11. `tenant_id` 必须随 outbox 保留，用于状态查询、审计和后续 DLQ 操作隔离；本任务不允许跨 tenant 混淆状态。
12. 错误信息不得记录 secret、token、完整 Authorization header 或敏感 payload。

## 失败与重试默认口径

本任务标题是 HTTP Sender，原始验收只要求 2xx、4xx/5xx、超时能正确更新状态；项目文档同时要求 worker 支持 retry / DLQ。为避免阻塞 Owner 决策，本轮采用以下最小可验收默认：

- 2xx：立即 `SENT`。
- 429、5xx、网络错误、超时：可重试失败，标记 `FAILED`，增加 `retry_count`，设置 `next_retry_at`。
- 400、401、403、404、409、422 等明确业务或配置错误：不可重试失败，可直接进入 `DLQ`，并记录错误摘要。
- 若可重试失败达到最大自动尝试次数，进入 `DLQ`。
- 默认最大自动尝试次数沿用项目现有 outbox worker 文档语义：有限次重试后进入 DLQ；测试验收只要求能验证“未达上限仍 FAILED / 到达上限 DLQ”，不强制本需求定义具体秒级退避参数。
- 如目标返回 `Retry-After`，可作为下一次重试时间依据；若无该头，则使用系统默认退避策略。

## 边界条件

- `target_channel=MQ`：非本期，不发送，不因 HTTP sender 失败而进入 DLQ。
- `target_ref` 为空或格式非法：失败，记录错误，不发起无目标请求。
- `payload_json` 为空或非法 JSON：失败，记录错误；不构造替代 payload。
- HTTP 204：属于 2xx，视为成功。
- HTTP 3xx：默认不按成功处理；是否跟随重定向不作为本期能力，避免把发奖指令发送到非配置目标。
- HTTP 4xx：默认不可重试，除 429 外进入 DLQ。
- HTTP 5xx：默认可重试，超出最大次数进入 DLQ。
- 请求超时：默认可重试失败，记录 timeout。
- 外部目标返回超大错误体：错误摘要需要截断，不完整保存响应体。
- Sender 进程在 claim 后、发送前或发送中崩溃：不得造成永久不可处理；需要有恢复口径使该 outbox 能重新进入可处理状态或被人工识别。
- 重复事件：事件幂等层不应新增 participation、grant_order 或 outbox，因此 sender 不应收到重复业务命令。
- Quota 不足、规则拒绝、零金额奖励：上游不生成 outbox，sender 无需处理。

## 异常场景

- 外部发奖系统返回 200 但业务 body 表示失败：本期按 HTTP Sender 最小口径只以 HTTP 状态码判断发送成功；业务最终发奖结果由 reward callback / 对账链路处理。
- 外部目标接收请求后连接中断，sender 未拿到 2xx：按失败处理，后续可能重试；因此外部发奖系统必须按 commandId 或 payload 内幂等键保证幂等接收。
- Sender 对同一 outbox 重试，目标系统已处理过：目标系统应幂等返回成功或可识别重复；ExMarketing 不重新计算奖励。
- 目标地址长期不可用：outbox 最终进入 DLQ，等待人工 replay 或 close。
- 数据库短暂不可用：sender 不应在无法可靠更新状态时继续扩大投递；恢复后可重新扫描未完成 outbox。

## 非目标范围

- 不实现 MQ command out。
- 不新增外部发奖系统的业务协议、签名、鉴权或响应 body 语义。
- 不改变活动匹配、规则执行、奖励计算、quota 扣减、grant_order 创建逻辑。
- 不改变事件幂等键规则。
- 不做 Admin 鉴权、生产级权限模型或完整运维后台。
- 不实现 reward callback 的成功 / 失败确认逻辑；callback 和 quota confirm/release 属于后续或已有独立链路。
- 不保证外部发奖实际到账，只保证发奖指令 HTTP 投递状态可追踪。
- 不要求本期定义完整指标、告警、压测和生产发布流程。

## 风险

- 重复投递风险：worker 并发或崩溃恢复处理不严谨会导致外部重复发奖；验收必须覆盖同一 outbox 不被并发发送。
- 误判成功风险：只按 HTTP 2xx 判断发送成功，外部系统业务失败需依赖 callback / 对账补偿；本期需明确这是非目标。
- 错误分类风险：4xx/5xx、429、超时分类影响自动重试和 DLQ；本轮给出默认最小分类口径，避免实现阶段自由发挥。
- 状态卡死风险：claim 后崩溃可能留下长期 SENDING；需求要求具备恢复或人工识别口径。
- 安全风险：错误日志或 error_message 可能泄露 token、secret 或 payload；必须脱敏截断。
- PostgreSQL 与内存仓储差异风险：多 worker claim 和并发状态更新必须用 PostgreSQL 语义验证，不能只依赖内存测试。

## 验收标准

功能验收：

1. Worker 未开启时，上游生成的 HTTP / WEBHOOK outbox 保持待处理，不自动发送。
2. Worker 开启后，`target_channel=HTTP` 且目标返回 2xx，outbox 更新为 `SENT`，失败字段不再影响后续扫描。
3. Worker 开启后，`target_channel=WEBHOOK` 且目标返回 2xx，outbox 更新为 `SENT`。
4. 目标返回 500，outbox 不丢失，状态进入可重试失败，`retry_count` 增加，`error_message` 记录 500 摘要，`next_retry_at` 被设置。
5. 目标返回 429，按可重试失败处理；如有 `Retry-After`，下一次处理时间应体现该语义。
6. 目标返回 400 / 401 / 403 / 404 / 422，按不可重试失败处理，进入 DLQ 或等价人工处理状态，并记录错误摘要。
7. 请求超时，outbox 不丢失，状态进入可重试失败，错误信息包含 timeout 摘要。
8. 可重试失败达到最大自动尝试次数后，同一 outbox 进入 DLQ，后续普通自动扫描不再发送。
9. `target_channel=MQ` 的 outbox 不被 HTTP sender 发送，状态不因本 sender 改为成功或失败。
10. 非法 `target_ref` 或非法 `payload_json` 不会导致进程崩溃，outbox 进入失败 / DLQ 并记录原因。

一致性验收：

11. 同一 outbox 在多 worker 并发扫描下最多被一个 worker claim 并发送。
12. retry / replay 不新增 participation、grant_order、quota 扣减或新的 outbox。
13. 重复标准事件仍只生成一次业务 outbox；sender 不承担重复事件去重职责。
14. 成功或失败状态更新必须保留 tenant 维度，不允许跨 tenant 修改。
15. error_message 和日志不得包含完整 Authorization header、token、secret 或完整敏感 payload。

测试验收：

16. 需要覆盖 HTTP 2xx、4xx、5xx、429、超时、网络错误、非法 target、非法 payload、MQ 跳过、最大重试进入 DLQ。
17. 需要覆盖 PostgreSQL 下多 worker claim 不重复投递。
18. 需要保留或新增回归验证：quota 不足、规则拒绝、零金额奖励不生成 outbox；重复事件不新增 outbox。
19. 最低命令验收为 `go test ./...` 和 `go build ./...`。
20. 涉及 PostgreSQL claim / 并发语义的验收必须使用 Docker PostgreSQL 或等价 PostgreSQL 环境。

## 待确认问题

以下问题不阻塞本轮进入研发，已用默认最小口径关闭：

- 最大重试次数和退避精确参数：默认要求“有限次自动重试后 DLQ”，不在本需求强制秒级参数；实现可沿用项目现有 worker 配置或常量。
- HTTP 成功是否解析业务 body：默认不解析，只以 2xx 判断 sender 层成功；业务到账以 callback / 对账为准。
- 是否支持 HTTP 签名或 Authorization：默认不支持新增协议鉴权；`target_ref` 和 payload 已包含上游配置，不在 sender 中扩展密钥管理。
- 3xx 是否跟随：默认不跟随，不把发奖指令发送到非显式配置目标。

## Score

```text
88
```

评分：

- 需求清晰度：18/20，HTTP sender 的输入、输出、状态和异常分类已明确。
- 范围可控：18/20，限定 HTTP / WEBHOOK，排除 MQ、callback、Admin 鉴权和外部业务协议。
- 风险可控：17/20，重复投递、状态卡死、敏感信息和 PostgreSQL 并发风险已列入验收。
- 验收标准明确：18/20，覆盖成功、失败、超时、并发、非目标通道和回归。
- 架构一致性：17/20，遵守 outbox 解耦、grant_order/outbox 唯一发奖入口和 PostgreSQL claim 要求。

## Decision

```text
PASS
```
