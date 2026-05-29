# Analysis: 2026-05-27-worker-production-hardening-worker

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未设计具体代码改法，未修改业务代码，未提交。

已读取：

- `tasks/2026-05-27-worker-production-hardening-worker/review.md`
- `tasks/2026-05-27-worker-production-hardening-worker/analysis.md` 上一版
- `tasks/2026-05-27-worker-production-hardening-worker/task.md`
- `tasks/2026-05-27-worker-production-hardening-worker/agent-prompts.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `README.md`
- `CLAUDE.md`
- 历史需求材料：`tasks/2026-05-24-outbox-worker-dlq/analysis.md`

未发现独立 PRD 目录。本分析以任务说明、现有 API / 数据模型 / 运维文档和历史 Outbox / DLQ 需求为依据。历史任务已定义 Outbox 投递、retry、DLQ、replay、close 的业务语义；本任务只补齐 Worker 生产化运行、恢复、观测和停止语义。

## Review 反馈处理

`review.md` 当前是模板态，但存在需要回应的条目：

- `P0/P1/P2`：未填写具体问题。本轮已按高风险生产化任务补齐常驻运行、优雅停止、shutdown drain、失败恢复、多 worker claim、监控、日志、异常和验收标准，避免因需求空缺被打回。
- `Open Questions 待填写`：上一轮没有可执行答案。本轮不等待 Owner 决策，按最小可验收原则给出默认口径：生产默认 shutdown drain 上限 30 秒、stale claim 恢复阈值 5 分钟、监控最低以结构化日志和可测试指标快照/指标采集器表达，测试环境允许缩短时间参数以完成自动化验证。
- `Decision PASS / BLOCK`：模板未做决策。本轮 Score 90，Decision PASS。

## Analysis 自检修正

上一版 `analysis.md` 是模板态，存在以下 BLOCK 原因：

- `背景和已知信息`、`目标理解`、`细化需求`、`业务规则`、`输入输出`、`异常场景`、`边界条件`、`非目标范围`、`风险点`、`验收方式`、`待确认问题` 均为待填写：本轮全部补齐。
- `Score = 0`：原因是没有形成可审核需求。本轮按评分规则重新打分为 90。
- `Decision = PASS / BLOCK`：未决策。本轮在旧缺口已关闭的基础上给出 `PASS`。
- 旧模板隐含错误假设是“缺 Owner 决策即可停在待确认”。本轮已按任务指令用默认最小可验收口径关闭缺口，只有后续扩展项保留为不阻塞待确认。

## 背景和目标

ExMarketing 活动引擎通过标准链路生成发奖指令：

```text
Event In -> Idempotency -> Campaign Match -> Rule Evaluation
-> Quota / Budget / Inventory -> Grant Order -> Outbox
-> External Reward System -> Callback In
```

现有文档已经定义 `marketing_outbox`、`send_status` 状态、DLQ 管理 API、retry / DLQ 语义，以及 PostgreSQL worker 必须先 claim 到 `SENDING` 再投递，避免多 worker 重复发奖。运维文档也把生产前检查明确到：Worker 实例数和 claim 机制验证通过、DLQ 有告警和人工处理流程。

本任务目标是将 Outbox worker 从“可发送”提升为“可长期生产运行”：

- Worker 开启后常驻运行，持续扫描并处理可投递 Outbox，而不是只执行一次。
- 收到停止信号时停止新 claim，并对已 claim / 已开始处理的消息执行 shutdown drain，避免停止时丢正在处理消息。
- Worker 崩溃或进程重启后，历史未完成的 outbox 可恢复到后续可处理状态。
- 多 worker 并发运行时，同一 outbox 在同一处理窗口内不得被重复 claim / 重复并发投递。
- claim 延迟、DLQ 堆积和关键处理结果可观测。
- 日志字段足够定位 tenant、outbox、worker 实例、状态流转、失败原因和耗时。

## 参与方

- ExMarketing 服务进程：承载 HTTP API、活动引擎和 Outbox worker。
- Outbox worker：按配置启动，常驻扫描、claim、投递、retry、DLQ 和恢复。
- PostgreSQL repository：生产验收的主要持久化对象，负责 outbox 状态、claim 条件和恢复语义。
- In-memory repository：本地开发和单元测试可用，要求不破坏现有行为，但生产化 claim / 恢复验收以 PostgreSQL 为准。
- 外部发奖系统：接收 HTTP / WEBHOOK / MQ Reward Command，并按 `commandId` / `bill_no` 做幂等。
- 运维 / SRE：观察 worker 健康、claim 延迟、DLQ 堆积、失败日志，并执行停机、重启和扩缩容。
- 运营人员：通过既有 DLQ 查询 / replay / close 流程处理 DLQ，不直接参与 worker claim。

## 输入输出

输入：

- 运行配置：`EX_MARKETING_OUTBOX_WORKER=true` 开启 worker；未开启时 worker 不应自动 claim 或投递。
- 可处理 outbox：`send_status` 为 `PENDING` 或按既有 retry 规则已到期的失败记录；`next_retry_at` 为空或已到期。
- 已 claim outbox：`send_status=SENDING`，表示某 worker 已占用并处于投递中。
- Stale claim：长期停留 `SENDING` 且超过恢复阈值的 outbox。
- 停止信号：进程上下文取消、SIGINT / SIGTERM 或等价服务停止事件。
- 外部投递结果：成功、可重试失败、不可重试失败、超时、网络错误、MQ sender 配置错误或 adapter 返回错误。

输出：

- 常驻 worker 生命周期状态：started、polling、claimed、draining、stopped。
- Outbox 状态流转仍遵循既有业务语义：`PENDING -> SENDING -> SENT / FAILED / DLQ`，`DLQ -> PENDING / DLQ_CLOSED`。
- 停止时不丢已 claim 消息：要么在 drain 窗口内写入最终状态，要么在恢复窗口后可重新处理。
- 重启后 stale `SENDING` 消息可恢复为可重试处理，不创建新的 participation、grant_order、quota 扣减或 outbox。
- 观测输出：worker 处理日志、claim 延迟、DLQ 堆积、in-flight 数、处理成功 / 失败 / 进入 DLQ 计数。

## 业务规则

### 启动与常驻运行

- 只有 `EX_MARKETING_OUTBOX_WORKER=true` 时自动启动 worker。
- Worker 启动后必须持续运行，直到服务停止或上下文取消。
- Worker 空闲时不得退出；没有可处理 outbox 时应等待下一轮扫描。
- Worker 不能因为单条消息投递失败、payload 异常、目标异常、MQ provider 缺失或状态冲突而整体退出。
- Worker 启动日志至少记录 `worker_id`、repository mode、enabled channels、poll interval、batch size、stale claim threshold、drain timeout。

默认最小可验收口径：

- 生产默认 poll 间隔不超过 5 秒。
- 测试环境允许通过配置或注入方式缩短 poll / stale / drain 时间，以便自动化测试。
- 常驻运行的验收以“处理完一批后还能继续处理后续新增 outbox”为准。

### Claim 与多 worker 并发

- PostgreSQL 模式下必须先 claim 到 `SENDING` 再投递。
- 同一 outbox 在同一处理窗口内最多只能被一个 worker claim 成功。
- 多 worker 同时扫描同一批 `PENDING` / 到期 retry outbox 时，不得出现重复并发投递。
- claim 必须受 tenant、channel、状态和 `next_retry_at` 约束，不得提前 claim 未到期 retry 消息。
- `SENT`、`DLQ`、`DLQ_CLOSED` 不得被普通扫描 claim。
- Claim 失败、状态已被其他 worker 改变或记录不存在时，应作为正常并发结果处理，不得污染状态。

### Shutdown Drain

- 收到停止信号后，worker 必须先停止新 claim。
- 已经 claim 或已经开始投递的消息进入 drain 阶段。
- Drain 阶段应尽力等待正在处理的消息完成状态写回。
- 默认生产 drain 上限为 30 秒；测试可使用更短上限。
- Drain 完成前成功投递的消息必须写入成功状态；失败投递必须按既有 retry / DLQ 规则写回。
- 如果 drain 超时仍有 in-flight 消息，worker 允许退出，但必须保证该消息不会永久丢失：后续进程重启或 stale claim 恢复会重新处理。
- 停止过程日志必须区分 `shutdown_requested`、`claim_stopped`、`drain_started`、`drain_completed`、`drain_timeout`、`worker_stopped`。

### 失败恢复

- 进程崩溃、被 kill、机器重启或投递后写回前异常，都不得导致 outbox 永久卡死。
- 默认 stale claim 恢复阈值为 5 分钟：`SENDING` 且 `updated_at` 早于当前时间 5 分钟以上的记录视为可恢复。
- 恢复 stale claim 时，不重新执行活动匹配、规则判断、奖励计算、quota 扣减、participation 创建、grant_order 创建或 outbox 创建。
- 恢复后的消息仍使用原 `payload_json`、原 `target_channel`、原 `target_ref`、原 `commandId` / `bill_no`。
- 恢复后再次投递可能造成外部重复收到同一 command；该风险由外部发奖系统幂等承接。
- 恢复动作必须可观测，至少记录 stale outbox id、tenant、原状态、恢复后状态、stale duration、worker id。

默认最小可验收口径：

- Stale `SENDING` 恢复为 `PENDING` 或项目等价的可再次 claim 状态。
- 恢复动作不增加 retry 次数；真正再次投递失败后才按既有 retry 规则增加 `retry_count`。
- 恢复动作保留历史 `error_message`，除非再次投递产生新的失败摘要。

### 观测与监控

关键指标必须可被测试或运维采集，最低覆盖：

- Worker 是否启动、是否正在 drain、当前 in-flight 数。
- claim 尝试数、claim 成功数、claim 空批次数、claim 冲突数。
- claimed outbox 的 claim 延迟：`now - eligible_at`，其中 `eligible_at` 为 `created_at` 或到期 retry 的 `next_retry_at`。
- 投递成功数、可重试失败数、不可重试失败数、进入 DLQ 数。
- stale claim 恢复数和恢复耗时。
- DLQ 堆积数量，至少按 tenant、target channel 维度可区分。
- 处理耗时和投递耗时。

默认最小可验收口径：

- 如果项目已有 metrics endpoint 或指标采集抽象，应以指标形式暴露上述语义。
- 如果项目尚无正式 metrics endpoint，本期最低接受结构化日志 + 可测试的 worker 统计快照 / recorder；不得只依赖人工读自然语言日志。
- claim 延迟监控的告警阈值默认建议：P95 超过 60 秒应产生可识别 warning 日志或指标标签。
- DLQ 堆积监控的告警阈值默认建议：同一 tenant 任意 channel `DLQ` 数量大于 0 产生 warning 观测事件；超过 10 条产生 high severity 观测事件。该阈值用于最低生产化提示，不替代后续正式告警平台。

### 日志字段

Worker 相关结构化日志至少包含：

- `worker_id`
- `event`
- `tenant_id`
- `outbox_id`
- `target_channel`
- `target_ref` 摘要或脱敏值
- `message_type`
- `send_status_before`
- `send_status_after`
- `retry_count`
- `next_retry_at`
- `claim_delay_ms`
- `delivery_duration_ms`
- `error_class`
- `error_message` 摘要
- `trace_id` 或 `request_id`，如当前上下文存在

日志不得包含 Authorization、Cookie、token、secret、签名密钥、MQ 明文凭据、完整敏感响应体或完整 provider credential。

## 异常场景

- Worker 开启但数据库短暂不可用：worker 不应永久退出；应记录错误并在下一轮继续尝试。
- Worker 启动后无可处理 outbox：进程保持常驻，输出空批次或心跳类观测，不退出。
- 单条 outbox payload 异常：只影响该 outbox，按既有不可重试失败 / DLQ 规则处理，不影响其他消息。
- 外部目标超时：按既有 retry / DLQ 规则处理；shutdown drain 中也不得跳过状态写回。
- 停止信号发生在 claim 前：不得 claim 新消息，worker 直接停止或进入空 drain。
- 停止信号发生在 claim 后投递前：该消息必须进入 drain，若未完成则后续 stale 恢复可继续处理。
- 停止信号发生在外部投递中：drain 等待到完成或超时；超时后靠 stale 恢复保证不永久卡死。
- 外部系统已收到 command，但 worker 写回前崩溃：重启后可能重投同一 command，必须保持同一 payload 和幂等标识。
- 多 worker 同时启动：同一 outbox 不得重复并发 claim；不同 outbox 可并行处理。
- Stale 恢复与正常 worker claim 并发：只有一个状态条件更新能成功；失败方记录冲突但不报致命错误。
- DLQ 堆积增长：不阻塞 worker 处理其他可投递消息，但必须产生可观测堆积信号。
- 日志中出现敏感信息风险：必须脱敏和截断，不能把完整外部错误响应直接输出。

## 边界条件

- 本任务不改变 Outbox 业务状态机，只补齐生产运行要求。
- 本任务不重新定义 HTTP / WEBHOOK / MQ 成功失败分类；沿用现有 Outbox sender / DLQ 规则。
- `next_retry_at` 未到期的消息不得因常驻扫描被提前投递。
- `DLQ` 和 `DLQ_CLOSED` 不参与普通自动 claim；DLQ replay 仍通过既有管理入口回到可投递状态。
- In-memory repository 不作为多 worker 生产并发 claim 的强验收对象；多 worker / stale 恢复强验收以 PostgreSQL 为准。
- Stale 恢复阈值不能小于单次正常投递超时加合理写回时间，避免抢占仍在正常处理的消息；默认 5 分钟满足最小口径。
- Worker 停止时不保证外部目标一定只收到一次；保证的是本系统不丢 outbox、状态可恢复、幂等标识不变。
- 系统时间影响 retry 到期、claim 延迟和 stale 判断；测试允许小范围时间容差。
- 监控阈值是最低可验收提示，不等同于正式 PagerDuty / Prometheus / Grafana 告警体系。

## 非目标范围

- 不实现新的发奖业务规则、活动规则、奖励计算或 quota 扣减逻辑。
- 不改变 DLQ replay / close API 语义。
- 不新增人工补发不存在 outbox 的能力。
- 不重设计 reward callback、对账、quota timeout release 或 grant_order 状态机。
- 不要求实现完整生产告警平台、Grafana dashboard、PagerDuty 集成、日志采集系统或长期指标存储。
- 不要求为 in-memory repository 提供跨进程并发 claim 保证。
- 不要求解决外部发奖系统不幂等导致的重复发奖问题。
- 不要求改造 MQ broker adapter 协议、topic 管理或 broker 连接配置。
- 不要求实现零停机滚动发布编排；本期只要求 worker 本身具备停止 drain 和重启恢复语义。

## 风险点

- 重复发奖风险高：投递成功但写回前崩溃会导致重启后重投；必须保持 command 幂等标识不变，并依赖外部系统幂等。
- 卡死风险高：`SENDING` 如果没有 stale 恢复会永久堆积；本任务必须补齐默认恢复口径。
- 并发风险高：多 worker claim、stale 恢复与正常 claim 并发都必须按状态条件判断。
- 停机风险中高：drain 超时太短会增加重投，太长会影响发布停止速度；默认 30 秒是最小生产折中。
- 观测风险中：没有正式指标系统时容易只留下不可测试日志；本期要求至少有结构化日志和可测试统计快照 / recorder。
- 安全风险中：外部错误、MQ 配置和 target 信息可能含敏感信息，日志必须脱敏。

## 验收标准

- `EX_MARKETING_OUTBOX_WORKER=false` 或未开启时，worker 不自动 claim 或投递 outbox。
- `EX_MARKETING_OUTBOX_WORKER=true` 时，worker 启动后常驻运行；处理完第一批 outbox 后，后续新增 outbox 仍会被处理。
- Worker 空批次扫描不会退出，也不会把任何消息错误标记为失败。
- 收到停止信号后，worker 停止新 claim。
- 停止时已有 in-flight outbox 在 drain 窗口内完成投递的，必须写入对应最终状态或 retry / DLQ 状态。
- Drain 超时退出的 in-flight outbox 不得丢失；重启并超过 stale 阈值后可重新进入可处理流程。
- 默认生产 drain timeout 为 30 秒；默认 stale claim threshold 为 5 分钟；测试允许缩短但语义一致。
- 进程重启后，stale `SENDING` outbox 可恢复为可再次 claim 状态，且不新增 participation、grant_order、quota 扣减或 outbox。
- Stale 恢复不改变原 payload、target channel、target ref、`commandId` / `bill_no`。
- PostgreSQL 模式下多个 worker 并发扫描同一批 outbox，同一 outbox 同一处理窗口最多一个 worker claim 成功。
- PostgreSQL 模式下多 worker 处理多条 outbox 时，不发生跨 tenant / 跨记录状态污染。
- `next_retry_at` 未到期消息不会被 claim；到期后可被 claim。
- `SENT`、`DLQ`、`DLQ_CLOSED` 不会被普通 worker claim。
- 单条消息投递失败不会导致 worker 整体退出，后续消息仍可处理。
- 数据库短暂错误、claim 冲突或目标失败会产生可观测错误，但 worker 后续可继续运行。
- Claim 延迟可观测，至少能识别 claimed 消息从可投递到被 claim 的延迟。
- DLQ 堆积可观测，至少能按 tenant 和 target channel 识别当前 DLQ 数量。
- Worker in-flight、claim 成功 / 冲突、投递成功 / 失败、进入 DLQ、stale 恢复等关键计数可被测试验证。
- Worker 日志包含规定结构化字段，且不泄露 token、secret、Authorization、Cookie、签名密钥或 MQ 明文凭据。
- `go test ./...` 和 `go build ./...` 应通过。
- PostgreSQL smoke 必须覆盖常驻处理、并发 claim、shutdown drain、重启后 stale 恢复、claim 延迟观测、DLQ 堆积观测。

## 待确认问题

以下问题不阻塞本期开发，默认口径已经在上文给出：

1. 正式生产告警平台使用 Prometheus、日志规则、云监控还是其他系统？本期默认只要求可测试指标语义和结构化日志。
2. Drain timeout 和 stale claim threshold 是否需要按租户、channel 或消息类型差异化？本期默认全局 30 秒和 5 分钟。
3. Claim 延迟和 DLQ 堆积的正式告警阈值是否由 SRE 统一管理？本期默认 P95 claim 延迟 60 秒 warning、tenant/channel DLQ 大于 0 warning、大于 10 high severity。
4. Worker 实例 ID 是否来自 hostname、进程 ID、配置项还是运行时生成？本期只要求同一运行周期内可区分 worker。
5. 是否需要将 worker 健康纳入 `/healthz` 或新增 readiness 语义？本期不强制改变健康检查 API，只要求 worker 观测可验证。

## 评分

```text
需求清晰度        18/20
业务规则完整      18/20
边界异常清楚      18/20
验收标准明确      18/20
约束一致性        18/20
总分              90/100
```

扣分原因：正式生产告警平台、指标暴露协议、worker_id 来源和健康检查集成仍需后续运维口径确认。但本轮已给出默认最小可验收口径，且不影响 Dev / Test 按常驻运行、drain、恢复、并发 claim、监控和日志要求推进。

## Decision

```text
PASS
```

PASS 原因：Score 为 90/100，业务目标、参与方、输入输出、业务规则、边界异常、非目标范围、风险和验收标准均已明确；剩余问题已提供默认口径，不构成 Decision BLOCK。

需求：2026-05-27-worker-production-hardening-worker
