# Analysis

## 需求 ID

`2026-05-27-mq-production-config-mq`

## 需求名称

MQ 生产连接配置与运行时隔离

## Review 反馈处理

本轮已读取 `tasks/2026-05-27-mq-production-config-mq/review.md`。该文件仍为模板态：

- `Findings` 中 `P0/P1/P2` 未填写具体问题：本轮无实质 Review 缺陷可逐条修复。
- `Open Questions` 为“待填写”：本轮不保留空问题，基于 `task.md`、`README.md`、`docs/api.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/operations.md` 和同类 MQ/outbox 任务，补齐默认最小可验收口径。
- `Decision` 为 `PASS / BLOCK` 占位：本轮重新按评分规则给出明确决策。

因此本节将 Review 模板态视为返工输入，处理方式是补齐需求目标、输入输出、业务规则、边界异常、验收标准和非目标范围，不继续 BLOCK。

## Analysis 自检修正

上一轮 `analysis.md` 为模板态，存在以下自检问题：

- `背景和已知信息 / 目标理解 / 细化需求 / 业务规则 / 输入输出 / 异常场景 / 边界条件 / 非目标范围 / 风险点 / 验收方式 / 待确认问题` 均为“待填写”：本轮已完整补齐。
- `Score = 0`：上一轮未完成需求分析，按规则应视为 BLOCK；本轮重新评分。
- `Decision = PASS / BLOCK` 占位：本轮给出明确 `PASS`。
- 旧缺口“生产 MQ 连接配置、认证引用、发送确认、错误分类和运行时隔离未定义”：本轮将其拆成配置来源、targetRef 约束、provider 确认语义、错误分类、审计脱敏和验收标准。
- 旧缺口“不能等待 Owner 决策”：本轮采用默认最小可验收口径关闭，即每类 MQ provider 至少一个运行时连接配置入口，活动配置和 outbox 只保存 destination，不保存 broker endpoint 或凭据。

当前不存在必须继续 BLOCK 的历史自检问题。

## 背景和已知信息

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动配置、生成 participation、grant_order 和 outbox 指令，不直接修改外部钱包、积分、优惠券或会员权益。外部发奖通过 Command Out 完成，真实发奖结果以 `/openapi/v1/reward/callback` 为准。

现有文档已明确：

- `EX_MARKETING_OUTBOX_WORKER=true` 启用 outbox worker。
- worker 负责发送 `HTTP`、`WEBHOOK` 和已配置的 `MQ` reward command。
- MQ Command Out 通过 Kafka、RabbitMQ、Pulsar adapter 边界发送。
- `targetRef` 对 MQ 仅表示目的地，支持 `kafka://<topic>`、`rabbitmq://<exchange>/<routingKey>`、`rabbitmq:///<routingKey>`、`pulsar://<tenant>/<namespace>/<topic>`。
- `targetRef` 不允许包含 broker 地址、账号、密码、token、query 或 fragment。
- MQ `SENT` 只代表 Kafka ack、RabbitMQ publisher confirm 或 Pulsar send ack，不代表真实权益已发放。
- MQ 失败、超时或 sender 未配置时必须保留 outbox payload，并按 retry / DLQ / replay / close 规则处理。

本任务的重点不是重新定义 MQ 发奖业务链路，而是把生产连接配置、安全隔离、认证引用、发送确认、错误分类和可审计验收补齐到可上线口径。

## 目标理解

目标是让 MQ Command Out 在生产环境可配置、可隔离、可诊断、可审计：

- 活动配置和 outbox payload 中不得保存 broker endpoint、用户名、密码、token、证书私钥或完整连接串。
- `targetRef` 只描述消息目的地，不描述连接凭据。
- Kafka、RabbitMQ、Pulsar 的连接参数和认证信息从运行时安全配置读取。
- 发送成功必须以 provider 的可靠确认语义为准。
- 发送失败必须分类为可重试、不可重试或配置错误，并进入一致的 retry / DLQ 路径。
- 日志、审计、错误信息和 API 响应不得泄露凭据。
- HTTP / WEBHOOK / MQ outbox 共存时互不影响。

## 参与方

- 活动运营：配置 `grantMode=AUTO`、`grantChannel=MQ` 和 MQ `targetRef`。
- ExMarketing Engine：按既有事务链路生成 participation、grant_order 和 outbox。
- Outbox Worker：读取 outbox，按 `target_channel=MQ` 使用运行时配置发送消息。
- MQ Broker：Kafka、RabbitMQ 或 Pulsar，接收 reward command。
- 外部发奖系统：消费 MQ 消息，幂等执行发奖，并通过 reward callback 回写结果。
- 运维 / SRE：配置运行时 MQ 连接、轮换凭据、查看日志、审计、DLQ 和告警。
- 安全 / 审计方：验证凭据不落库、不入业务配置、不进入 payload 或明文日志。

## 输入输出

### 输入

活动奖励配置：

- `grantMode=AUTO`。
- `grantChannel=MQ`。
- `rewardConfig.targetRef` 必填，且只能是 MQ destination URI。

合法 `targetRef`：

- `kafka://<topic>`
- `rabbitmq://<exchange>/<routingKey>`
- `rabbitmq:///<routingKey>`，表示 RabbitMQ 默认 exchange。
- `pulsar://<tenant>/<namespace>/<topic>`

运行时 MQ 配置：

- provider 类型：Kafka、RabbitMQ、Pulsar。
- broker endpoint / bootstrap servers / service URL。
- 认证引用或认证材料：用户名密码、token、SASL、TLS 证书引用等，具体形态由部署环境提供。
- TLS 开关、CA / client certificate 引用、连接超时、发送超时、重试相关参数。
- provider 启用状态和可选租户/目的地 allowlist。

默认最小可验收口径：

- 每类 provider 至少支持一个运行时连接配置入口。
- 不要求本轮支持同一 provider 多集群动态路由。
- 不要求活动配置选择具体 broker 集群；活动只能通过 `targetRef` 选择目的地。
- 如果运行环境未配置某 provider，指向该 provider 的 MQ outbox 发送时按“sender 未配置”失败处理，并进入 retry / DLQ，不静默丢弃。

### 输出

成功发送：

- provider 已确认接收消息后，对应 outbox 可进入 `SENT`。
- grant_order 仍等待 callback，不因 MQ `SENT` 自动变为 `SUCCESS`。
- 审计和日志可定位 tenant、outbox、provider、destination、确认类型和耗时，但不包含凭据。

失败发送：

- outbox payload 保持不变。
- outbox 记录保留错误分类、错误摘要、retry 次数和下次 retry 时间。
- 可重试失败按 backoff 重试。
- 不可重试失败或重试耗尽进入 `DLQ`。
- `DLQ` 可通过既有 API 查询、replay、close。

## 细化需求

### 配置来源与隔离

1. MQ broker 连接配置必须来自运行时配置或安全配置，不得来自活动配置、rewardConfig、outbox payload、event payload 或模拟运行输入。
2. 运行时配置允许表达 provider endpoint、认证方式、TLS、超时、启用状态和目的地约束。
3. 活动配置只允许保存 `targetRef` destination URI。
4. outbox 只允许保存 `target_channel=MQ`、`target_ref` 和标准 reward command payload。
5. 任何包含 `user:password@`、`token=`、`password=`、`secret=`、query、fragment 或完整 broker endpoint 的 MQ `targetRef` 都必须拒绝。
6. 配置缺失、配置格式错误、认证引用不可用时，必须产生可诊断错误，不允许以空配置或默认公共 broker 继续发送。

### Provider 确认语义

1. Kafka：只有 producer 收到 broker 对目标 topic 的确认后，才可视为发送成功。
2. RabbitMQ：只有 publisher confirm 或等价确认成功后，才可视为发送成功。
3. Pulsar：只有 send ack 成功后，才可视为发送成功。
4. 本地调用返回成功但未获得 provider 确认，不得把 outbox 标记为 `SENT`。
5. 超时、连接中断或确认结果未知时，按可重试失败处理；后续重复发送必须使用相同 commandId / idempotencyKey。

### 错误分类

可重试失败：

- broker 暂时不可达、DNS 临时失败、连接超时、发送超时、限流、临时网络中断、leader 切换、服务端临时不可用。

不可重试失败：

- `targetRef` 非法、provider scheme 不支持、destination 缺失、payload 无法序列化、消息超过 broker 或系统限制且无法自动恢复。

配置或认证失败：

- provider 未配置、配置解析失败、认证材料缺失、认证失败、授权不足、TLS 配置不可用。
- 默认按可诊断失败进入 retry；如果同一 outbox 重试耗尽，进入 DLQ。
- 对明显无法通过重试恢复的配置错误，允许直接进入 DLQ，但必须记录明确分类。

目标不存在或被拒绝：

- Kafka topic 不存在或无写权限。
- RabbitMQ exchange / routingKey 不存在、不可路由或被拒绝。
- Pulsar tenant / namespace / topic 不存在或无写权限。
- 默认按配置/授权类失败处理，保留 outbox 并进入 retry / DLQ。

### 审计与脱敏

1. 普通日志、错误信息、outbox error_message、audit payload、DLQ 查询结果不得包含密码、token、secret、Authorization、私钥、完整连接串。
2. 允许记录 provider、destination、错误分类、错误码摘要、traceId、outboxId、tenantId、耗时和 retry 次数。
3. 发送失败审计必须足以支持运维定位“配置缺失、认证失败、目标不存在、broker 不可达、payload 错误、超时、重试耗尽”。
4. 凭据轮换不应要求修改活动配置或 outbox 记录。

## 业务规则

1. MQ 生产连接配置不得改变活动引擎事务边界；participation、quota reservation、grant_order、outbox 仍按既有原子链路写入。
2. MQ 发送必须由 outbox worker 异步执行，不允许在事件处理事务内直接连接 broker 发送。
3. `targetRef` 是业务目的地引用，不是连接串。
4. MQ `SENT` 只表示 command 已被 broker 接受；真实发奖成功仍以 reward callback 为准。
5. MQ 发送遵循 at-least-once 语义，不承诺 exactly-once。
6. 重试导致重复消息时，消息中的 commandId / billNo / idempotencyKey 必须稳定不变，由外部发奖系统幂等处理。
7. 运行时未启用 outbox worker 时，可生成 MQ outbox，但不得发送；记录保持待发送语义。
8. 同一服务中 HTTP / WEBHOOK / MQ outbox 必须按通道隔离处理，一个 provider 配置错误不得阻断其他通道发送。
9. 模拟运行只展示 MQ outbox command 预览，不发送 MQ，不写 outbox，不暴露运行时凭据。
10. DLQ replay 只能复用原 outbox 和原 payload，不能创建新的 grant_order 或新的 outbox。
11. `DLQ_CLOSED` 不允许 replay。
12. 生产配置变更或凭据轮换后的新发送应使用新运行时配置；历史 outbox 不需要改写。

## 边界条件

- provider 未配置：MQ outbox 发送失败并进入 retry / DLQ；服务不得静默标记 `SENT`。
- provider 被禁用：同未配置处理，并提示 provider disabled。
- `targetRef` scheme 不属于 Kafka/RabbitMQ/Pulsar：活动创建或上线前校验拒绝；历史脏数据发送时进入不可重试失败。
- `targetRef` 含 query、fragment、userinfo、疑似凭据字段：拒绝。
- Kafka topic 为空、RabbitMQ routingKey 为空、Pulsar tenant/namespace/topic 不完整：拒绝。
- RabbitMQ 默认 exchange 只允许 `rabbitmq:///<routingKey>` 这种明确格式，不允许省略 routingKey。
- broker ack 前进程崩溃：outbox 不得永久卡死，后续可按既有 claim 超时或 retry 语义恢复；重复发送使用相同幂等标识。
- broker 已接收但本地超时未知：按可重试处理，允许重复消息。
- 认证失败：不得输出明文账号或 secret，只记录认证失败分类。
- 凭据轮换期间短暂失败：按可重试处理；重试耗尽后进 DLQ。
- 多租户共用 provider：必须至少保证 tenantId 进入消息 payload 和审计，不允许跨 tenant 修改 grant_order 或 callback 结果。

## 异常场景

- 运行时 MQ 配置 JSON / 环境变量格式错误。
- 认证引用指向不存在的 secret。
- TLS CA 或客户端证书不可用。
- broker endpoint 配置错误、DNS 不通或端口不可达。
- Kafka producer 无 topic 写权限。
- RabbitMQ publisher confirm 超时或 unroutable。
- Pulsar producer 创建失败或 send ack 超时。
- reward command payload 序列化失败。
- 消息体超过默认限制或 provider 限制。
- outbox worker 多实例并发 claim 同一消息的保护失效风险。
- DLQ replay 后 provider 仍不可用。
- 日志或审计误记录完整连接串、token、password。

## 非目标范围

- 不新增 MQ Event In；本需求只覆盖 Command Out 的生产连接配置和隔离。
- 不重新设计 outbox 表、grant_order、callback、quota 或活动匹配流程。
- 不实现外部发奖系统消费端。
- 不提供 exactly-once 投递保证。
- 不要求本轮支持同一 provider 多集群、多 region、按租户动态集群路由。
- 不要求建设完整监控大盘或告警平台，但必须保留可审计和可排查信息。
- 不把 broker 凭据存入数据库、活动配置、outbox payload、审计或普通日志。
- 不把 MQ `SENT` 解释为发奖成功。
- 不改变 HTTP / WEBHOOK targetRef 的既有语义。

## 风险点

- 凭据泄露风险：如果允许连接串进入 `targetRef` 或 error_message，会形成持久化泄露；必须通过校验和脱敏控制。
- 重复投递风险：超时或未知确认会导致重复 MQ 消息；必须依赖稳定 commandId / idempotencyKey。
- 运维误判风险：`SENT` 容易被误认为发奖成功；文档、审计和查询必须明确其含义。
- 多 provider 差异风险：Kafka、RabbitMQ、Pulsar 的确认与错误语义不同；需求只要求归一到成功、可重试失败、不可重试失败、配置/认证失败。
- 配置缺失风险：生产环境未配置 broker 时 MQ outbox 会堆积并进入 DLQ；需要可诊断错误和人工处理路径。
- 测试环境风险：没有真实 broker 时无法覆盖全部协议差异；最小验收必须覆盖配置隔离、targetRef 校验、确认语义替身、错误分类和 DLQ 流转。

## 验收方式

1. 合法 MQ `targetRef` 可通过活动配置校验，覆盖 Kafka、RabbitMQ、Pulsar 和 RabbitMQ 默认 exchange。
2. 非法 MQ `targetRef` 被拒绝，至少覆盖：未知 scheme、空 destination、缺 routingKey、含 userinfo、含 query、含 fragment、含疑似 password/token/secret。
3. 活动配置、outbox payload、audit payload 和普通日志中不得出现 broker endpoint、password、token、secret、Authorization、私钥或完整连接串。
4. 运行时未配置目标 provider 时，MQ outbox 发送失败为可诊断错误，payload 不变，并按 retry / DLQ 流转。
5. Kafka ack、RabbitMQ publisher confirm、Pulsar send ack 成功时，outbox 才能进入 `SENT`。
6. broker 发送超时或确认未知时，不得标记 `SENT`，必须保留原 outbox 并进入 retry。
7. 认证失败、授权不足、目标不存在、payload 序列化失败、provider 未配置均有明确错误分类和脱敏错误摘要。
8. 重试耗尽或不可重试失败进入 `DLQ` 后，可通过既有 DLQ API 查询、replay、close；replay 后复用原 outbox 和原 payload。
9. MQ `SENT` 后 grant_order 不自动变为 `SUCCESS`，仍等待 reward callback。
10. HTTP / WEBHOOK outbox 的发送、retry、DLQ 行为不因 MQ 配置缺失或 MQ provider 失败而回归。
11. 模拟运行包含 MQ command 预览但不发送 MQ、不写 outbox、不读取或输出运行时凭据。
12. 文档需说明生产 MQ 配置来源、`targetRef` 格式、凭据隔离规则、`SENT` 含义、错误分类和 DLQ 操作。
13. 基础验证命令后续至少包含 `go test ./...` 和 `go build ./...`；如有真实 broker 环境，可增加 Kafka/RabbitMQ/Pulsar 冒烟，但真实 broker 不作为本轮 PASS 的唯一前置。

## 待确认问题

以下问题不阻塞本轮研发，已给出默认最小可验收口径：

- 是否支持同一 provider 多集群。默认：本轮不支持多集群动态路由，每类 provider 一个运行时配置入口即可。
- 认证材料由环境变量、secret manager 还是挂载文件提供。默认：需求只要求来自运行时安全配置，并保证不进入活动配置/outbox/audit/log；具体部署载体由 Dev/Ops 选择。
- 是否必须接入真实 Kafka/RabbitMQ/Pulsar 做集成测试。默认：最小验收用可控替身覆盖确认和错误分类；真实 broker 冒烟作为生产化增强。
- 是否按租户限制 MQ destination allowlist。默认：本轮至少不允许 targetRef 携带凭据；租户级 allowlist 作为可选运行时约束，不阻塞基础验收。

## Score

```text
需求清晰度        18/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      19/20
约束一致性        19/20
总分              94/100
```

扣分原因：

- 生产环境 secret 管理载体和真实 broker 联调环境未在任务材料中指定。
- 多集群和租户级 destination allowlist 仍需后续运维策略确认，但不影响按默认单 provider 配置入口推进。

## Decision

```text
PASS
```

理由：现有材料足以确定业务目标、参与方、输入输出、配置隔离、安全约束、错误分类、边界异常和验收对象。缺失的生产部署细节已用默认最小可验收口径关闭，不构成 Decision BLOCK。
