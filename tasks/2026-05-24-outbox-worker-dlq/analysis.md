# Analysis: 2026-05-24-outbox-worker-dlq

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未设计具体代码改法，未修改业务代码，未提交。

已读取：

- `tasks/2026-05-24-outbox-worker-dlq/task.md`
- `tasks/2026-05-24-outbox-worker-dlq/review.md`
- `README.md`
- `CLAUDE.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/roadmap.md`
- `docs/workflow.md`
- `docs/automation.md`

未发现独立 `PRD/` 目录。本分析以任务说明、Owner 补充决策、上游设计文档、已有 API 文档、数据模型、运维文档、Roadmap 和 Agent 流程文档为依据。

## 结论摘要

Owner 已在 `task.md` 中补齐上一轮 BLOCK 的关键决策：

- 本期范围明确为 `HTTP / WEBHOOK` Outbox Worker。
- `MQ Command Out` 不在本期实现范围内，拆分为后续独立任务。
- 指数退避参数、失败分类、`Retry-After` 规则和验收容差已明确。
- DLQ 人工关闭使用机器可判定状态 `DLQ_CLOSED`。
- replay / close 只允许对当前 `DLQ` 状态执行，缺少 operator 或 reason 必须失败。

因此，本需求已具备进入 Review Agent 的条件。后续 Dev Agent 应按本分析约束开发 HTTP/Webhook outbox 投递、指数退避 retry、DLQ 查询、replay、close 和审计能力，不实现 MQ sender / MQ retry / MQ DLQ / MQ replay / MQ close。

## 背景目标

ExMarketing 是独立活动营销中台，只通过标准事件、Fact Query 和 Reward Command 与外部系统交互。系统不直接给用户加钱，不连接外部业务数据库，不实现真实钱包、积分、优惠券、权益或实物发放。

标准链路：

```text
Event In -> Idempotency -> Campaign Match -> Rule Evaluation
-> Quota / Budget / Inventory -> Grant Order -> Outbox
-> External Reward System -> Callback In
```

本任务目标：

- 让业务记录与外部发奖投递解耦，事件处理响应不依赖外部发奖系统实时可用。
- 发奖指令写入 outbox 后，目标失败、超时、不可达或 worker 崩溃都不得丢失。
- 本期只处理 `target_channel = HTTP / WEBHOOK` 的 outbox 自动投递。
- 自动投递失败后按 Owner 确认的指数退避规则重试。
- 达到最大重试失败次数后进入 `DLQ`，自动投递停止。
- DLQ 支持 tenant 范围查看、人工 replay 和人工 close。
- close 后进入 `DLQ_CLOSED`，并可机器判断禁止 replay。
- replay / close 必须记录操作者、原因和审计。

## 参与方

- 外部业务系统：上报标准事件；不参与 outbox retry、DLQ、replay、close 决策。
- ExMarketing 活动引擎：完成活动匹配、规则判断、奖励计算、quota 扣减、participation、grant_order 和 outbox 生成；本任务不改变这条业务链路。
- Outbox worker：扫描可投递 HTTP/Webhook outbox，完成投递、失败记录、重试调度和 DLQ 流转。
- 外部发奖系统：接收 Reward Command，执行真实发奖，并通过 callback 回传结果；必须按 `commandId` / `bill_no` 做幂等。
- 运营 / 运维人员：在 tenant 范围内查看 DLQ，执行 replay 或 close，并填写操作者和原因。
- 后续 MQ 任务 owner：承接 MQ Command Out，不作为本任务验收对象。

## 输入输出

输入：

- rewardable 事件在 activity 级事务内生成的 `marketing_outbox`。
- outbox 字段：`tenant_id`、`message_type`、`target_channel`、`target_ref`、`payload_json`、`send_status`、`retry_count`、`next_retry_at`、`error_message`、`created_at`、`updated_at`。
- Reward Command payload，至少应能关联 tenant、`commandId` / `bill_no`、activity、participation、subject、reward、reason 和幂等信息。
- Worker 启停配置：`EX_MARKETING_OUTBOX_WORKER`。
- HTTP/Webhook 投递结果：HTTP 状态码、连接错误、DNS/TLS 错误、请求超时、响应读取超时。
- 人工 DLQ 操作输入：tenant 范围、outbox id、operator、reason、操作类型 replay / close。

输出：

- HTTP/Webhook 2xx 后，同一 outbox 进入发送成功语义。
- 可重试失败后，同一 outbox 保留原 payload，记录 `retry_count`、`next_retry_at`、`error_message`、`updated_at`。
- 不可重试失败后，同一 outbox 直接进入 `DLQ`。
- 达到最大重试失败次数后，同一 outbox 进入 `DLQ`，自动投递停止。
- DLQ 查询支持 tenant 范围过滤，并能区分 `DLQ` 与 `DLQ_CLOSED`。
- replay 后，同一 outbox 使用原 payload、原 `commandId` / `bill_no` 重新进入可投递流程。
- close 后，同一 outbox 进入 `DLQ_CLOSED`，并禁止 replay。
- replay / close 写审计，记录 tenant、outbox id、operator、reason、操作前状态、操作后状态和时间。

## 业务规则

### Outbox 生成与解耦

- Worker 未开启时，不自动投递 outbox；业务事件仍可生成 participation、grant_order 和 outbox。
- 外部目标失败不得回滚已完成的业务事务，不得导致 outbox 消失。
- retry / replay 不重新执行活动匹配、规则判断、奖励计算、quota 扣减、participation 创建、grant_order 创建或 outbox 创建。
- retry / replay 只重新投递同一 outbox 的原 `payload_json`。
- 同一 grant_order 的 outbox payload 必须保持同一 `commandId` / `bill_no`。
- 重复上报同一标准事件仍按幂等返回 duplicate，不因 retry / replay 生成额外业务记录。

### 本期通道范围

- 本期只投递 `target_channel = HTTP / WEBHOOK`。
- `target_channel = MQ` 不进行投递。
- MQ sender、MQ retry、MQ DLQ、MQ replay、MQ close、broker ack、message key、topic / queue 语义和验收环境全部拆到后续独立任务。
- 当前活动创建 API 仍只允许 `AUTO + HTTP / WEBHOOK`，并要求 `rewardConfig.targetRef` 为 `http(s)` URL；MQ 活动创建不属于本任务放开范围。

### HTTP / Webhook 成功规则

- HTTP/Webhook `target_ref` 必须是 `http` 或 `https` URL。
- HTTP 2xx 视为发送成功。
- 本期不校验响应体业务字段；2xx 但响应体为空、非 JSON 或业务字段异常，仍按发送成功处理。

### HTTP / Webhook 失败分类

可重试失败：

- 网络连接失败。
- DNS 错误。
- TLS 错误。
- 请求超时。
- 响应读取超时。
- HTTP 408。
- HTTP 429。
- HTTP 5xx。

不可重试失败，直接进入 `DLQ`：

- HTTP 400。
- HTTP 401。
- HTTP 403。
- HTTP 404。
- HTTP 409。
- HTTP 422。
- payload 格式错误。
- `target_ref` 非法。

错误摘要必须脱敏和截断，不得包含 secret、token、完整 Authorization header、签名密钥或完整敏感响应体。

### 指数退避

Owner 确认的重试策略：

- 最大重试失败次数：5 次。
- 初始延迟：30 秒。
- 退避倍率：2。
- 最大延迟：30 分钟。
- jitter：允许正负 20% 抖动，避免集中重试。
- 时间基准：以本次发送失败写入时间作为下一次 `next_retry_at` 的计算基准。
- 验收容差：`next_retry_at` 允许正负 2 秒测试容差，jitter 只校验落在允许区间内。

每次失败必须更新：

- `retry_count`
- `next_retry_at`
- `error_message`
- `updated_at`

退避计算口径：

- 第 1 次可重试失败的基础延迟为 30 秒。
- 第 2 次可重试失败的基础延迟为 60 秒。
- 第 3 次可重试失败的基础延迟为 120 秒。
- 后续按 `30s * 2^(retry_count-1)` 增长，并受 30 分钟最大延迟限制。
- 实际 `next_retry_at` 可在基础延迟的正负 20% jitter 区间内。
- HTTP 429 如果存在可解析 `Retry-After`，优先使用 `Retry-After`；不存在或不可解析时使用指数退避。
- 达到最大重试失败次数后进入 `DLQ`，不再设置为自动可投递。

### 状态流转

本期需求层状态流转：

```text
PENDING -> SENDING -> SENT
PENDING -> SENDING -> FAILED -> PENDING
FAILED -> DLQ
DLQ -> PENDING
DLQ -> DLQ_CLOSED
```

状态规则：

- `PENDING` 表示可被 worker claim，且 `next_retry_at` 未到期的消息不得提前投递。
- `SENDING` 表示已被 worker 占用并处于投递中。
- `SENT` 表示外部 HTTP/Webhook 已返回 2xx。
- `FAILED` 表示一次投递失败的中间失败语义，可按 retry 规则重新回到可投递。
- `DLQ` 表示自动投递停止，等待人工处理。
- `DLQ_CLOSED` 表示人工关闭后的最终状态。
- `DLQ_CLOSED` 不允许 replay。
- 是否允许从 `DLQ_CLOSED` reopen 不在本期范围内。

### DLQ 查看 / Replay / Close

- DLQ 查看至少支持 tenant 范围过滤。
- DLQ 查询结果至少包含 id、tenant、target channel、target ref、retry count、error message、updated time 和当前状态。
- 查询必须能区分 `DLQ` 与 `DLQ_CLOSED`。
- replay 只允许对当前状态仍为 `DLQ` 的 outbox 执行。
- close 只允许对当前状态仍为 `DLQ` 的 outbox 执行。
- replay / close 缺少 operator 或 reason 时必须失败。
- 人工操作不得跨 tenant 生效。
- replay 后同一 outbox 重新进入可投递流程，且不得创建新的 participation、grant_order、quota 扣减、业务事件或 outbox。
- replay 后如果再次失败，继续按同一 retry 和 DLQ 规则处理。
- close 必须记录 operator、reason、closed_at。
- replay 必须记录 operator、reason、replayed_at。
- replay / close 必须写审计记录。

并发规则：

- close 与自动重试并发时，以状态条件更新为准；只有当前仍为 `DLQ` 才允许 close 成功。
- replay 与自动重试并发时，以状态条件更新为准；只有当前仍为 `DLQ` 才允许 replay 成功。
- replay 与 close 并发时，只有一个操作能基于当前 `DLQ` 状态成功；失败方必须得到可识别的状态冲突或不可执行结果。
- 多 worker 并发扫描时，同一 outbox 不得被多个 worker 同时投递。

## 边界条件

- 奖励计算结果为 `0.0000` 时不生成 grant_order / outbox，不进入本任务发送范围。
- 规则拒绝、quota 不足或重复事件不生成新的 rewardable outbox，不进入发送处理范围。
- `target_channel = MQ` 不投递，也不作为本任务 retry / DLQ / replay / close 验收项。
- HTTP/Webhook `target_ref` 为空或非法时，当前活动创建阶段应按 API 文档拒绝；历史或异常数据中已存在的非法 `target_ref` 发送时按不可重试失败直接进入 `DLQ`。
- `payload_json` 缺少外部发奖必要字段或格式不符合 Reward Command 时，按不可重试失败直接进入 `DLQ`。
- 目标不可达、连接被拒绝、超时或非 2xx 时，不得删除或跳过 outbox。
- HTTP 2xx 不校验响应体业务字段，可能存在外部实际失败但本系统视为发送成功的接入风险；该风险由外部 callback / 对账在后续能力中补齐。
- 外部系统成功处理但响应丢失时，worker 可能记录失败并后续重试；必须依赖外部发奖系统按同一 `commandId` 幂等。
- 外部系统完成发奖但 worker 写回成功状态前崩溃时，后续重试仍必须使用同一 payload 和同一幂等标识。
- Worker 在 `SENDING` 状态期间崩溃后的悬挂恢复未在 Owner 决策中给出明确阈值；本期验收不得用未定义的悬挂恢复策略作为硬性完成项，但实现不得破坏已定义状态语义。
- 系统时间会影响 `next_retry_at` 判断，验收按 Owner 确认的正负 2 秒容差处理。
- DLQ 查询最低要求是 tenant 过滤和 `DLQ` / `DLQ_CLOSED` 定位；完整分页、导出、告警、长期保留不是本期目标。

## 异常场景

- HTTP/Webhook 返回 400、401、403、404、409、422，直接进入 `DLQ`。
- HTTP/Webhook 返回 408、429、500、502、503、504，按可重试失败处理。
- HTTP/Webhook 返回 429 且 `Retry-After` 可解析，优先按 `Retry-After` 计算下一次投递。
- HTTP/Webhook 返回 429 但 `Retry-After` 缺失或不可解析，按指数退避计算下一次投递。
- HTTP/Webhook 返回 2xx 但响应体为空、非 JSON 或业务字段异常，按发送成功处理。
- DNS 解析失败、TLS 握手失败、连接失败、连接被拒绝，按可重试失败处理。
- 请求超时或响应读取超时，按可重试失败处理。
- payload 格式错误或 `target_ref` 非法，直接进入 `DLQ`。
- Worker 在发送中状态期间崩溃或重启，不得创建新的业务记录或改变 retry / replay 的幂等要求。
- 多 worker 同时扫描同一批 `PENDING` 或到期失败 outbox，同一 outbox 不得重复 claim。
- DLQ replay 后再次连续失败，重新进入 `DLQ`。
- 运营重复 replay 同一 DLQ，只有当前状态仍为 `DLQ` 的操作可成功。
- 运营 close 非 `DLQ` 消息，必须失败。
- 运营 replay `DLQ_CLOSED` 消息，必须失败。
- 运营跨 tenant 查看、replay 或 close，必须失败或不可见。
- replay / close 缺少 operator 或 reason，必须失败。
- 错误摘要包含敏感信息时，必须脱敏和截断。
- Reward callback 在 outbox 发送成功写回前后到达；本任务不改变 callback 更新规则。

## 非目标范围

- 不实现 MQ Command Out。
- 不实现 MQ sender、MQ retry、MQ DLQ、MQ replay、MQ close。
- 不实现多 broker 适配、broker ack 语义、topic / queue 管理或 MQ 验收环境。
- 不改变活动匹配、规则判断、奖励计算、quota 扣减和 grant_order 生成逻辑。
- 不直接给用户加钱，不写外部钱包、积分、优惠券、实物或权益系统。
- 不重设计 reward callback；callback 仍按 `tenantId + commandId` 更新 grant_order。
- 不实现 Manual Export、Manual 发奖状态机。
- 不实现预算冻结、callback 后确认消耗、失败释放等 Phase 3 补偿模型。
- 不实现 OpenAPI 鉴权、完整 Admin 鉴权或 callback 签名。
- 不实现生产级告警平台、完整 Outbox 运维后台、指标大盘、运维导出或长期保留策略。
- 不新增业务专用发奖逻辑。
- 不用 retry / replay 修复历史错误 payload 或补发不存在的发奖单。
- 不定义 `DLQ_CLOSED` reopen 能力。

## 风险

- 重复发奖风险高：网络超时、响应丢失、worker 崩溃恢复、DLQ replay 都可能造成同一 `commandId` 多次到达外部系统；必须依赖外部发奖系统幂等。
- 并发风险高：多 worker claim、replay/close 并发、人工操作与 worker 并发都必须按状态条件判定。
- 状态迁移风险中：新增 `DLQ_CLOSED` 后，查询、replay、close 和审计必须使用机器状态，不能用错误文本替代。
- 安全风险中：DLQ 操作是管理能力，生产鉴权未完成前不能公网暴露。
- 审计风险中：当前审计 action 文档只列配置和事件相关动作，本任务需要补齐 DLQ replay / close 审计语义。
- 运维风险中：最低 DLQ 查询/处理能力不等于完整生产运维后台，后续仍需 Phase 5 补齐。
- 接入口径风险中：本期 HTTP 2xx 即成功，不校验响应体业务字段；外部系统必须通过 callback / 对账暴露真实发奖结果。
- 悬挂恢复风险中：Owner 未定义 `SENDING` 悬挂恢复阈值，本期不应把该能力扩展成隐含需求。

## 验收标准

- 创建符合当前 API 边界的 `AUTO + HTTP` 或 `AUTO + WEBHOOK` 活动并上报标准事件后，业务响应不依赖外部发奖系统实时可用，且生成 participation、grant_order、outbox。
- 未开启 `EX_MARKETING_OUTBOX_WORKER` 时，outbox 保持待发送且不会被自动投递。
- 开启 worker 后，目标返回 HTTP 2xx 时，同一 outbox 最终进入发送成功状态。
- HTTP 2xx 响应体为空、非 JSON 或业务字段异常时，本期仍按发送成功验收。
- 目标返回网络连接失败、DNS/TLS 错误、请求超时、响应读取超时、HTTP 408、HTTP 429 或 HTTP 5xx 时，同一 outbox 不丢失，并更新 `retry_count`、`next_retry_at`、`error_message`、`updated_at`。
- `next_retry_at` 按 30 秒初始延迟、倍率 2、最大 30 分钟、正负 20% jitter、失败写入时间为基准计算；测试容差为正负 2 秒。
- HTTP 429 存在可解析 `Retry-After` 时，`next_retry_at` 优先使用 `Retry-After`。
- HTTP 400、401、403、404、409、422、payload 格式错误或 `target_ref` 非法时，同一 outbox 直接进入 `DLQ`。
- 达到 5 次最大重试失败次数后，同一 outbox 进入 `DLQ`，自动投递停止。
- 失败错误摘要不包含 secret、token、完整 Authorization header、签名密钥或完整敏感响应体。
- PostgreSQL 模式下多个 worker 并发运行，同一 outbox 不会被多个 worker 同时投递。
- Retry 和 replay 都只重新投递同一 outbox payload，不重新执行活动引擎。
- DLQ replay 后不新增 participation、grant_order、quota 扣减、业务事件或新的 outbox。
- 重复上报同一标准事件仍返回 duplicate，不因 retry / replay 生成额外业务记录。
- 对同一 grant_order 的 outbox payload 必须保持同一 `commandId` / `bill_no`。
- DLQ 查询支持 tenant 范围过滤，并返回 outbox id、tenant、target channel、target ref、retry count、error message、updated time 和当前状态。
- DLQ 查询能区分 `DLQ` 与 `DLQ_CLOSED`。
- 对 `DLQ` 执行 replay 后，同一 outbox 使用原 payload、原 `commandId` / `bill_no` 重新进入可投递流程。
- 对 `DLQ` 执行 close 后，同一 outbox 进入 `DLQ_CLOSED`。
- `DLQ_CLOSED` 不允许 replay，且该限制必须通过机器状态判断。
- replay / close 缺少 operator 或 reason 时必须失败。
- 非 `DLQ` 状态不得执行 replay 或 close。
- replay / close 不得跨 tenant 生效。
- replay / close 必须写审计，至少记录 outbox id、tenant_id、operator、reason、操作前状态、操作后状态、时间。
- replay 与 close 并发时，只有一个操作能基于当前 `DLQ` 状态成功。
- `target_channel = MQ` 不作为本期验收项；本期不得因未实现 MQ sender / retry / DLQ / replay / close 被判失败。
- 新增或复用的管理入口如未具备生产鉴权，必须明确限制为本地/内网运维用途，不能作为公网生产接口验收。
- 最低验证应包含常规测试、构建验证，以及涉及 PostgreSQL outbox claim / 状态条件更新的本地 PostgreSQL 验证。

## 待确认问题

以下问题不阻塞本期开发，但应在后续任务或发布前明确：

1. `SENDING` 悬挂恢复阈值是多少，恢复时是否增加 `retry_count`，是否改变 `next_retry_at`？
2. 人工 replay 后是否重置 `retry_count`，是否清空历史 `error_message`，还是保留历史失败摘要？本期最低要求是 replay 后立即重新进入可投递流程并保留同一 payload。
3. 重复 close 同一 `DLQ_CLOSED` 是返回幂等成功、冲突错误还是拒绝？
4. DLQ 查询 / replay / close 的最低入口是本地 admin API、页面操作、命令行工具还是内部运维接口？
5. outbox `DLQ_CLOSED` 是否需要联动 grant_order `CLOSED`？如果联动，callback 竞争、审计和状态优先级需单独定义。
6. 外部发奖系统对 `commandId` / `bill_no` 的幂等 SLA 是否需要形成正式接入协议？
7. 完整分页、导出、告警、保留策略和生产级 DLQ 后台何时纳入 Phase 5 任务？

## 评分

```text
需求清晰度        19/20
需求边界清楚      19/20
风险可控          18/20
验收标准明确      19/20
约束一致性        18/20
总分              93/100
```

扣分原因：本期范围、重试策略、失败分类和 DLQ close 机器语义已经由 Owner 明确，需求可审核、可验收、可进入后续 Review。剩余扣分来自 `SENDING` 悬挂恢复、replay 后计数字段细节、DLQ 操作入口形态、grant_order close 联动和生产化运维能力仍需后续任务确认，但这些不阻塞本期 HTTP/Webhook worker、retry、DLQ、replay、close 的实现。

## Decision

```text
PASS
```

PASS 原因：Score 为 93/100，且上一轮 BLOCK 的三个关键问题已由 Owner 决策补齐。本需求边界清楚、业务规则明确、异常场景和验收标准可执行，可交给 Review Agent 审核。
