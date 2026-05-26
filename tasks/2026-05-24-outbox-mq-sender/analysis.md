# Analysis

## 需求 ID

2026-05-24-outbox-mq-sender

## 需求名称

Outbox MQ Sender

## Review 反馈处理

上一轮 `review.md` 仍为模板态：

- `Findings` 中未填写具体 P0/P1/P2 条目：本轮无可逐条修复的 Review 缺陷，但按打回返工要求补齐完整需求说明、边界、异常场景、验收标准和非目标范围。
- `Open Questions` 为“待填写”：本轮不继续停留在占位问题，基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/api.md`、`docs/operations.md`、`docs/roadmap.md` 和 `22-独立活动营销中台设计.md` 给出默认最小可验收口径。
- `Decision` 为 `PASS / BLOCK` 占位：本轮按评分规则重新给出明确评分和决策。

## Analysis 自检修正

上一轮 `analysis.md` 为模板态，存在以下问题：

- `Score = 0`：已补充需求背景、目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准，重新评分。
- `Decision = PASS / BLOCK` 占位：已给出明确 `PASS`。
- `现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式` 均为“待填写”：已用本轮结构化需求细化替代。
- 潜在 BLOCK 原因“MQ 目标供应商和配置方式未决”：不等待 Owner 决策，默认最小可验收口径为同时支持 Kafka、RabbitMQ、Pulsar 三类 MQ 目标的出站 Command Out 语义；`targetRef` 作为目标引用，格式必须能区分供应商和目的地，连接凭据由运行环境配置，不写入活动配置或 outbox payload。
- 潜在 BLOCK 原因“MQ 失败如何不丢 outbox 未明确”：已明确 sender 只能在 broker 确认接受后将 outbox 置为 `SENT`；发送失败、超时、认证失败、目标不存在或 broker 不可达时必须保留原 outbox 记录，并进入既有 retry / backoff / DLQ 语义。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动规则、生成参与记录、发奖单和发奖指令，不直接修改外部钱包、积分、优惠券或会员权益。当前 Command Out 已通过 outbox 支持 HTTP / WEBHOOK 发奖指令发送，并具备 claim、retry、DLQ、replay、close 和 callback 闭环。

本需求来自“22-独立活动营销中台设计”中 Command Out 可通过 Reward Command / MQ / Webhook 对接外部发奖系统的目标。当前文档明确 `MQ` outbox delivery 尚不在现有 worker 范围内，活动创建阶段也拒绝 `grant_channel=MQ`。本需求要补齐 MQ 发奖指令发送适配边界，让奖励指令可以通过 Kafka、RabbitMQ、Pulsar 中任一 MQ 通道发送，同时继续保证业务事务与外部发送解耦，MQ 发送失败不会丢失 outbox 记录。

## 参与方

- ExMarketing Engine：在事件命中活动、奖励可发放时，按既有链路生成 participation、quota reservation、grant_order 和 outbox。
- Outbox Worker：异步 claim 可发送的 outbox 记录，并按目标通道发送 Command Out。
- MQ Broker：Kafka、RabbitMQ 或 Pulsar，作为外部发奖系统的异步消息入口。
- 外部发奖系统：消费 MQ 奖励指令，完成真实发奖，并通过 reward callback 回写 SUCCESS / FAILED。
- 运营 / 运维：通过 DLQ 查询、replay、close、审计和日志排查发送失败或人工关闭。

## 输入输出

### 输入

- 活动奖励配置：
  - `grantMode` 仍限定为 `AUTO`。
  - 新增可执行发奖通道 `grantChannel=MQ`。
  - `rewardConfig.targetRef` 对 MQ 必填，用于表达 MQ 供应商和目标目的地。
- Outbox 记录：
  - `target_channel=MQ`。
  - `target_ref` 来自奖励配置的 MQ 目标引用。
  - `payload_json` 为标准 Reward Command payload，必须包含外部发奖系统和 callback 对账所需字段。
- 运行环境配置：
  - MQ broker 连接信息、认证信息、TLS 等敏感配置必须来自运行环境或安全配置，不允许放入活动配置、outbox payload 或审计明文。

### 默认 MQ targetRef 最小口径

如现有材料未定义统一格式，本轮默认采用以下可验收口径关闭旧 BLOCK：

- `kafka://<topic>` 表示 Kafka topic。
- `rabbitmq://<exchange>/<routingKey>` 表示 RabbitMQ exchange 和 routing key；如目标使用默认 exchange，可用明确约定的空 exchange 表达，但不得省略 routing key。
- `pulsar://<tenant>/<namespace>/<topic>` 表示 Pulsar topic 的逻辑路径。
- `targetRef` 只表达消息目的地，不表达 broker 地址、账号、密码或 token。
- 未识别 scheme、缺少目的地、包含凭据或无法解析的 `targetRef` 必须在活动创建或上线前校验阶段拒绝。

该口径的依据是现有数据模型已将 `target_ref` 定义为“目标地址或引用”，设计文档要求支持 Kafka / RabbitMQ / Pulsar，但没有要求活动配置承载连接凭据。将 destination 与敏感连接配置分离，符合系统与外部业务解耦及安全规则。

### 输出

- 成功发送：
  - MQ broker 确认接受消息后，对应 outbox 进入 `SENT`。
  - 原 grant_order 保持等待外部 callback 的业务语义，不因“MQ 已发送”直接视为发奖成功。
- 可重试失败：
  - outbox 保持可追踪记录，更新错误信息、重试次数和下次重试时间。
  - 未超过最大重试次数时继续等待后续发送。
- 不可继续或重试耗尽：
  - outbox 进入 `DLQ`，可通过既有 DLQ 查询、replay、close 运维。

## 业务规则

1. MQ sender 不得改变事件处理主链路的原子性：participation、quota reservation、grant_order、outbox 仍必须在同一个 activity 级事务中完成；外部 MQ 发送只能在事务提交后异步进行。
2. 不允许绕过 grant_order 和 outbox 直接向 MQ 发奖；所有 MQ Command Out 必须由 outbox 驱动。
3. `grantChannel=MQ` 与 HTTP / WEBHOOK 同属自动发奖通道，但发送成功只代表发奖指令已投递到 broker，不代表真实权益已发放。
4. 真实发奖结果仍以 `/openapi/v1/reward/callback` 为准，callback 必须继续按 `tenantId + commandId` 定位 grant_order。
5. MQ 消息 payload 必须使用标准 Reward Command 语义，至少能支持外部发奖系统幂等处理和 callback：
   - `tenantId`
   - `commandId` / `billNo`
   - `activityId`
   - `participationId`
   - `subject.type`
   - `subject.id`
   - `rewardType`
   - `rewardAmount`
   - `rewardPayload`
   - `grantChannel=MQ`
   - `idempotencyKey`
6. MQ 消息必须以 `commandId` 或发奖单幂等键作为外部消费幂等依据；重复发送风险必须由外部消费者幂等承接，ExMarketing 不承诺 exactly-once。
7. Outbox worker 多实例并发时仍必须避免同一 outbox 被多个 worker 同时 claim；若发生网络超时导致 broker 可能已接收但本地未知，后续重试允许产生重复消息，但必须保留相同 commandId / idempotencyKey。
8. MQ 发送失败不得删除或覆盖 outbox payload；重放必须使用原 payload。
9. DLQ replay 对 MQ outbox 与 HTTP / WEBHOOK outbox 规则一致：仅允许 `DLQ` 回到 `PENDING`，`DLQ_CLOSED` 不可 replay。
10. `MQ` 通道启用后，活动创建和模拟运行的语义必须一致：模拟可预览 MQ outbox command，但不得实际发送 MQ 消息或写出 outbox。

## 边界条件

- 未启用 outbox worker 时：事件处理仍可生成 MQ outbox，但不会发送；记录必须保持 `PENDING` 或现有待发送语义。
- 未配置任何 MQ broker 连接信息时：服务可否启动由现有运行模式决定，但实际发送 MQ outbox 时必须失败为可诊断错误，并按 retry / DLQ 处理；不得静默丢弃。
- 同一服务同时存在 HTTP / WEBHOOK / MQ outbox 时：worker 必须按 `target_channel` 分流，任一通道失败不得影响其他通道已 claim 或可发送记录。
- 活动配置中 `grantChannel=MQ` 但 `targetRef` 为空、scheme 不合法、目的地缺失或含敏感凭据时：创建或上线前校验失败，不生成后续发奖链路。
- 外部发奖系统未 callback 时：grant_order 不因 MQ `SENT` 自动终结；长期未 callback 的处理仍走现有超时释放、对账或人工流程。
- MQ broker 返回已知不可重试错误时：允许直接进入 DLQ；错误信息需要能定位 provider、destination 和原因类型，但不得泄露凭据。
- MQ broker 超时或连接中断时：按“结果未知，可重试”处理，保留同一 outbox 和幂等标识。

## 异常场景

- broker 不可达、DNS 失败、连接超时、认证失败、授权不足。
- Kafka topic 不存在或 producer 被拒绝。
- RabbitMQ exchange / routing key 不存在或发布被 broker 拒绝。
- Pulsar topic 不存在、namespace 权限不足或 producer 创建失败。
- 发送过程中服务崩溃：已 claim 未完成的 outbox 不得永久卡死，必须能按既有超时或重试语义恢复为可处理状态或进入失败链路。
- payload 序列化失败：视为配置或数据错误，应记录 error 并进入 DLQ 或不可重试失败，不得发送空消息。
- 多 worker 并发：不得在正常 claim 语义下并发发送同一 outbox；异常重试导致的重复消息必须保持同一 commandId。
- DLQ replay 后仍失败：继续按 retry / DLQ 规则累计，不创建新的 grant_order 或新的 outbox。

## 非目标范围

- 不实现 MQ Event In，本需求只覆盖 Command Out。
- 不改变标准事件模型、活动匹配、规则引擎、奖励计算、quota 扣减、callback 协议。
- 不实现 exactly-once 投递；目标是 at-least-once + commandId 幂等。
- 不新增外部发奖系统消费端。
- 不新增业务专用 Kafka topic、业务专用 controller 或绕过通用 reward command 的发奖逻辑。
- 不把 broker 地址、账号、密码、token 写入活动配置、outbox payload、审计或普通日志。
- 不把 MQ `SENT` 等同于 grant_order `SUCCESS`。
- 不要求本轮完成生产级监控大盘、告警系统或跨地域 MQ 灾备。

## 风险

- 重复消息风险：网络超时后 broker 可能已接收，本地仍会重试；需依赖 commandId / idempotencyKey 保证外部幂等。
- 配置复杂度风险：三类 MQ provider 的目标引用语义不同，必须通过明确校验避免运行时才发现错误。
- 状态误读风险：运营可能将 `SENT` 理解为已发奖；文档和运维查询需要明确 `SENT` 仅表示指令已进入 broker。
- 安全风险：若 targetRef 或日志泄露 broker 凭据，会违反现有安全边界；必须禁止凭据进入业务配置和 payload。
- 回调缺失风险：MQ 发送成功但外部系统不 callback 会导致 quota 长期冻结；仍需依赖现有超时释放、对账和人工处理流程。
- 本地验证风险：没有真实 Kafka / RabbitMQ / Pulsar 环境时，仅用模拟发送无法覆盖 broker 差异；研发和测试阶段至少要覆盖可配置的失败、成功和 retry 状态语义，真实 broker 联调可作为增强验收。

## 验收标准

1. 活动配置可接受 `grantMode=AUTO` 且 `grantChannel=MQ` 的奖励配置；合法 `targetRef` 支持 Kafka、RabbitMQ、Pulsar 三类 scheme。
2. 非法 MQ `targetRef` 在活动创建或上线前校验阶段被拒绝，并返回可诊断错误；HTTP / WEBHOOK 原有校验不被削弱。
3. 上报命中奖励的标准事件后，系统仍按既有原子链路生成 participation、quota reservation、grant_order 和 outbox；MQ 发送不参与业务事务提交。
4. MQ outbox 的 payload 包含标准 Reward Command 关键字段，外部系统可用 `tenantId + commandId` 或 idempotencyKey 做幂等消费和 callback。
5. broker 确认接收后，outbox 进入 `SENT`；grant_order 不自动变为 `SUCCESS`，必须等待 reward callback。
6. broker 发送失败、超时、认证失败或目的地不可用时，outbox 记录不丢失，payload 不被改写，并按 retry / backoff / DLQ 规则流转。
7. 重试耗尽或不可重试失败后，MQ outbox 可在既有 DLQ API 中查询、replay、close；replay 不创建新的 outbox 或 grant_order。
8. 多 worker 并发处理 MQ outbox 时，不得正常并发 claim 同一条记录；异常重试重复发送时，消息中的 commandId / idempotencyKey 保持不变。
9. HTTP / WEBHOOK outbox 原有发送、retry、DLQ、replay、close 行为不回归。
10. 模拟运行只展示 MQ command 预览，不发送 MQ，不写 outbox，不扣 quota。
11. 错误日志、outbox error_message、audit payload 不包含 broker 密码、token、Authorization 或完整凭据。
12. 文档同步说明 MQ Command Out 的启用方式、targetRef 格式、`SENT` 与 callback 成功的区别、失败排查和 DLQ 操作方式。

## 待确认问题

以下问题不阻塞本轮进入研发，已给出默认最小口径；Owner 后续可覆盖：

- MQ `targetRef` 最终格式是否采用本文默认的 URI scheme。默认：采用 `kafka://`、`rabbitmq://`、`pulsar://` 三类 destination URI。
- 是否要求本轮接入真实 broker 做集成测试。默认：需求验收必须覆盖三类 provider 的配置校验和发送语义；如本地缺少真实 broker，可先用可控测试替身验证状态流转，真实 broker 联调作为后续生产化验收。
- 是否允许单服务同时配置多个同类 broker 集群。默认：本轮只要求每类 provider 一个运行环境配置入口；多集群路由不纳入本轮。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      19/20
架构一致性        19/20
总分              91/100
```

扣分说明：

- 三类 MQ 的真实 broker 运维差异较大，当前材料没有现成连接配置规范，仍保留少量实现与运维不确定性。
- 真实 broker 集成环境未在任务材料中明确，验收需区分状态语义验证和生产化联调。

## Decision

```text
PASS
```

需求细节已达到可审核、可研发、可验收标准。旧的模板态 BLOCK、Score 0、Open Questions 占位已用默认最小可验收口径关闭；剩余问题不阻塞本轮研发。
