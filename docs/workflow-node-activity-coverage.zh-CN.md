# Workflow 节点对活动模板覆盖评估

本文档对比当前 Workflow 节点分类与活动模板目录中的业务活动，判断现有节点是否能表达这些活动，以及还缺哪些能力。

参考资料：

- `docs/business-activity-templates.md`
- `docs/activityplugin-red-envelope-lottery.md`
- `docs/activityplugin-lifecycle.md`
- `docs/event-input-node-design.zh-CN.md`
- `docs/workflow-node-catalog.zh-CN.md`

## 总体结论

当前节点分类可以覆盖普通配置型活动的主链路：

```text
EVENT_INPUT -> CORE -> ADVANCED(可选) -> OPS(审计/恢复)
```

其中 T1-T9 业务活动模板基本满足：

- 标准事件输入：`event-in` / `http-event` / `webhook-event`
- 幂等：`inbox`
- 字段标准化：`normalize`
- 活动匹配：`match`
- 外部事实：`fact-query`
- 条件和资格：`condition` / `qualify`
- 活动状态：`activity-state`
- 次数/频控：`quota-check` / `limit`
- 奖励候选和奖励计算：`candidate-reward` / `reward`
- 预算、库存锁定：`budget-reserve` / `quota`
- 参与记录和发奖：`participation` / `grant-order` / `outbox`
- 审计、对账、失败恢复：`audit` / `reconcile` / `retry` / `dlq`
- 数据组合和拆分：`combine` / `split`

红包雨、抽奖这类插件玩法现在可以用 workflow 表达主链路和部分玩法边界：`plugin-execute` 表达插件动作，`activity-state` 表达状态快照，`candidate-reward` 表达候选奖励，`random-pick` 表达可审计随机。但插件内部生命周期、真实状态提交、抢包/抽奖次数消费仍应由 ActivityPlugin 或后台服务作为权威。

## 覆盖等级

| 等级 | 含义 |
| --- | --- |
| 满足 | 当前节点和已有引擎语义可以配置和运行主流程 |
| 基本满足 | 主流程满足，但生产上线还依赖审批、回调、对账或外部事实配置 |
| 部分满足 | workflow 主链路能表达，但玩法状态或入口 runtime 仍缺专门能力 |
| 不满足 | 当前节点无法表达核心业务语义 |

## 普通业务活动模板覆盖矩阵

| ID | 活动模板 | 当前覆盖 | 推荐节点链路 | 主要依赖 | 缺口 |
| --- | --- | --- | --- | --- | --- |
| T1 | 签到有礼 | 满足 | `event-in -> inbox -> normalize -> match -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `attributes.continueDays`、`DAILY_LIMIT`、`BUDGET` | 如果连续签到天数不由事件携带，需要 `fact-query` 或业务侧计算 |
| T2 | 每月充值 | 基本满足 | `event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `attributes.amount`、月度事实、`USER_LIMIT`、`BUDGET` | 跨月周期更适合按月活动或明确 quota scopeKey；高风险上线需审批/对账 |
| T3 | 续存/复存 | 满足 | `event-in -> inbox -> normalize -> match -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `attributes.depositCount`、`attributes.amount` | 历史充值次数如果事件不带，需要 `fact-query` |
| T4 | 新手任务 | 满足 | `event-in -> inbox -> normalize -> match -> condition/qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `attributes.taskCode`、`attributes.completed`、非金额 fixed reward | 多任务包可用 `combine` 组合任务上下文，或用 `split` 拆分任务项 |
| T5 | 返水 | 基本满足 | `event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `validBetAmount`、结算事实、`BUDGET` | 高风险资金模板，生产必须配审批、callback、reconcile |
| T6 | 邀请好友 | 基本满足 | `event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `inviteeId`、邀请关系事实、邀请人/被邀请人限制 | 双主体关系建议用 `fact-query` + `combine` 生成资格上下文 |
| T7 | VIP 周/月红利 | 基本满足 | `event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `facts.VIP.level`、周期流水、`BUDGET` | 需要稳定 period scopeKey；生产必须有审批/对账 |
| T8 | 生日红利 | 基本满足 | `event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | 会员资料事实、生日周期、`USER_LIMIT`、`BUDGET` | 生日事实必须由 Fact Provider 或事件 attributes 提供 |
| T9 | 升级红利 | 满足 | `event-in -> inbox -> normalize -> match -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox -> audit` | `attributes.oldLevel`、`attributes.newLevel`、FORMULA/CONDITION、LADDER reward | 当前规则表达应使用安全 FORMULA + compare；workflow 节点无需新增 |

## 插件玩法覆盖矩阵

| 活动类型 | 当前覆盖 | 已能表达的部分 | 缺口 |
| --- | --- | --- | --- |
| RED_ENVELOPE 红包雨 | 基本满足 | 标准事件进入、幂等、活动匹配、状态快照、资格、插件执行、候选奖励、预算锁定、参与记录、发奖、outbox、审计 | 场次真实状态、抢包并发消费、空包/命中原因仍依赖 ActivityPlugin 权威 |
| LOTTERY 抽奖 | 基本满足 | 标准事件进入、幂等、活动匹配、状态快照、次数频控、随机抽取、候选奖励、预算锁定、参与记录、发奖、outbox、审计 | 抽奖次数真实消费、奖池库存最终提交仍依赖 ActivityPlugin/Quota 权威 |

## 当前节点对活动能力的满足情况

### 已满足

普通活动模板的核心链路已经满足：

- 事件输入：`event-in`、`http-event`、`webhook-event`
- 幂等和 payload 冲突：`inbox`
- 活动匹配：`match`
- 事实增强：`fact-query`
- 条件和资格：`condition`、`qualify`
- 奖励：`reward`
- 次数、预算、库存：`limit`、`quota`
- 参与、发奖单、发件箱：`participation`、`grant-order`、`outbox`
- 审计和恢复：`audit`、`retry`、`dlq`、`reconcile`

数据加工也已经增强：

- `combine`：把多个上游输出组合成一个上下文。
- `split`：把结构化输入拆成数组或多个输出分片。
- `code`：通用 Lua / Expression。
- `decimal`：金额精度显式处理。

### 基本满足，但生产有条件

高风险资金活动 T5-T9 不是节点缺失，而是生产控制要求更高：

- 活动级 `BUDGET` quota。
- 用户周期 `USER_LIMIT`。
- 审批。
- 发奖 callback。
- 对账查询。
- DLQ/retry 监控。
- 审计链路。

这些能力当前由主引擎、OpenAPI、Admin API、OPS 节点和运维流程共同承担。节点分类可以表达，但上线前必须检查配置和环境能力。

### 已有第一版 adapter，仍需后台运行态完善

以下活动能力已经具备节点 adapter 第一版，但生产级后台 worker、cursor、replay 和 UI 运行态还要继续完善：

1. **MQ 入站触发**
   - `mq-event` 已输出 MQTriggerWorker adapter plan。
   - `internal/mqconsumer.Consumer` 已支持标准事件解析、幂等、ACK/DLQ/NACK_RETRY 语义。
   - 仍需生产 broker loop、offset 管理和 replay UI。

2. **定时触发**
   - `schedule-event` 已输出 Scheduler/EventInboxProducer plan。
   - 已要求稳定 eventId/bizId/idempotencyKey。
   - 仍需统一调度注册表和运行态可视化。

3. **状态监听**
   - `state-event` 已支持通过 `resourceRef` 读取快照并判断条件。
   - 仍需 Redis/config/metrics watcher、cursor、debounce 持久化和恢复语义。
   - `REDIS mode=KV/HASH/COUNTER/LOCK` 资源已能作为 workflow 状态读写能力使用，真实 Redis client adapter 仍是后续边界。

4. **红包雨/抽奖插件状态**
   - 已有 `activity-state`、`candidate-reward`、`random-pick`、`plugin-execute` 表达边界。
   - 真实插件状态提交、奖池消费、抢包/抽奖次数消费仍必须由 ActivityPlugin 或持久化服务负责。

## P0/P1/P2/P3 节点和资源落地状态

本轮已按 P0/P1/P2/P3 增加节点契约、handler、模板、前端目录和资源化执行底座。

| 节点 | 作用 | 状态 |
| --- | --- | --- |
| `activity-state` | 活动/用户/插件状态快照 | 已实现 handler，跨执行持久化仍由 ActivityPlugin/StateStore 权威 |
| `quota-check` | 次数/频控校验 | 已实现，复用 `limit` 语义并输出 `quota_check` |
| `candidate-reward` | 生成待发奖励候选 | 已实现，从活动奖励配置或插件结果生成 |
| `budget-reserve` | 预算/库存锁定 | 已实现，复用 `quota` 语义并输出 `budget_reservation` |
| `reward-reconcile` / `reconcile` | 对账跟踪 | 已实现节点契约，真实扫描由 Reconciliation/Admin API 权威 |
| `approval-gate` | 审批网关 | 已实现决策输出，真实审批由 ActivityApproval/AdminAPI 权威 |
| `compensation` | 补偿计划 | 已实现计划输出，真实补偿由 CompensationWorker/AdminAPI 权威 |
| `random-pick` | 可审计随机抽取 | 已实现稳定 hash 抽取 |
| `plugin-execute` | 插件动作边界 | 已实现计划输出，真实插件动作为 ActivityPlugin 权威 |
| `ab-bucket` | AB 分桶 | 已实现稳定分桶 |
| `risk-score` | 风险评分 | 已实现 facts/rule 输出 |
| `settlement-window` | 结算窗口 | 已实现窗口判定，DEFER 仍需 Scheduler |
| `manual-adjust` | 人工调整 | 已实现计划输出，真实调整由 AdminManualAdjustAPI 权威 |
| `state-store` | 通用资源状态读写 | 已支持 `TABLE`、`HTTP`、`REDIS mode=KV/HASH/COUNTER/LOCK` 的 `resourceRef` 第一版 |
| `mq-publish` | MQ 出站发布意图 | 已支持 `MQ resourceRef` 第一版；真实 broker ack 仍由 OutboxWorker/MQ sender 权威 |
| `fact-query` | 事实查询 | 新模型优先走 Service Interface；兼容 `resourceRef` 第一版，可读状态/Redis 语义资源或受控 HTTP Resource |
| `outbox` / `callback` | 发件箱和回调边界 | 已支持 `resourceRef` 第一版，校验资源、生成计划并审计；真实投递/回调仍由 worker/API 权威 |
| `code` / `combine` / `split` / `lua-query` | Lua 上下文加工 | 已支持受控 Lua Resource API，所有资源调用走权限、审计和调用限额 |

注意：这些节点不能直接绕过 participation、grant_order、outbox 或 quota。真实发奖仍必须走 CORE 主链路。

资源化后的仍需补齐项：

- `REDIS mode=KV/HASH/COUNTER/LOCK` 当前是资源语义和状态承载版，不是真实 Redis client adapter。
- 生产 Kafka/RabbitMQ/Pulsar transport SDK 需要通过 OutboxWorker/MQSender 注入。
- 资源级 `requireApproval` 是第一版信号校验；`requireFormalApproval` 已可校验活动正式审批快照，独立资源审批单仍未建立。

## 对签到奖励的结论

签到奖励属于 T1，当前节点已经满足。

推荐最小链路：

```text
标准事件入口 -> 幂等收件箱 -> 事件标准化 -> 活动匹配 -> 事实查询 -> 活动状态 -> 次数频控 -> 资格校验 -> 候选奖励 -> 预算库存锁定 -> 参与记录 -> 发奖订单 -> 消息发件箱 -> 对账 -> 审计记录
```

如果签到天数由业务系统直接上报：

- 不需要 `fact-query`。

如果签到天数需要 ExMarketing 查询：

- 在 `match` 后增加 `fact-query`。

如果一次事件里包含多个任务或多个奖励候选：

- 使用 `combine` 组合上下文。
- 使用 `split` 拆分任务项或奖励项。

## 最终判断

| 范围 | 是否满足 |
| --- | --- |
| T1-T4 低风险普通模板 | 满足 |
| T5-T9 高风险资金模板 | 基本满足，但依赖审批、callback、reconcile、quota 配置 |
| RED_ENVELOPE 红包雨 | 基本满足，真实玩法状态仍需 ActivityPlugin 权威 |
| LOTTERY 抽奖 | 基本满足，真实奖池/次数消费仍需 ActivityPlugin/Quota 权威 |
| MQ 入站活动 | 基本满足，已完成 adapter 第一版，生产 broker loop/replay UI 待完善 |
| 定时活动 | 基本满足，已完成 adapter 第一版，统一调度注册表待完善 |
| 状态监听活动 | 基本满足，已完成 ResourceAPI 快照判断第一版，后台 watcher/cursor 待完善 |

补充说明：

- 出站 `MQ` 资源能力已经可用于通知、发奖指令或业务消息发布意图。
- 入站 `mq-event`、定时 `schedule-event`、状态监听 `state-event` 仍属于 trigger runtime 能力；当前节点 adapter 已可配置和验证，但生产 worker 运行态还需要继续补齐。
