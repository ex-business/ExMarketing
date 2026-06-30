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

## ACT 21 类活动完整覆盖评估

本节按 `act-analysis/01-返水.md` 到 `act-analysis/21-每月充值.md` 逐项评估。结论分三层：

```text
基础主链路是否满足：事件、幂等、规则、事实、quota、奖励、发奖、审计
生产完整度是否满足：调度、状态、并发、审核、对账、回滚、后台页面
扩展方式：通过模板、Service Interface、ActivityPlugin、StateStore、Scheduler、Ranking 或 Gift Fulfillment 补齐
```

总体结论：

1. **基础主链路满足**：21 类活动都可以落到 ExMarketing 的标准事件、规则、事实查询、quota/budget、reward、grant-order、outbox 和 audit 主链路上。
2. **普通资金类活动基本满足**：首存、续存、返水、生日、升级、VIP 周/月、每月充值、签到、游戏活动等主要差在模板、事实服务、审批、周期 quota 和对账。
3. **复杂玩法类活动部分满足**：红包雨、主题抽奖、足球射门、幸运翻卡、嘉年华、赛事竞猜、任务体系、邀请/王者回归等需要正式 ActivityPlugin、状态存储、排行榜、调度和专用 Studio 页面。
4. **不是节点越多越好**：复杂玩法不应继续扩散成专用节点或专用 controller；应把通用发奖链路留在 Workflow Engine，把玩法状态放到插件运行时。

### 逐活动覆盖矩阵

| # | 活动 | 基础主链路 | 生产完整度 | 当前缺口 | 推荐扩展方式 |
| --- | --- | --- | --- | --- | --- |
| 01 | 返水 | 满足 | 基本满足 | 每日批结算、游戏黑名单、批量人工审核、返水明细报表、结算失败重跑 | `schedule-event` 生成结算事件；`fact-query` 查询有效投注和黑名单；返水模板；批量审核/对账 Studio |
| 02 | 邀请好友 | 满足 | 部分满足 | 邀请链路、短链、邀请关系归因、邀请人/被邀人双主体、三子活动统一管理、自邀请风控 | `INVITE_CAMPAIGN` 插件或 Service；双主体 fact；邀请关系 StateStore；互斥/风控模板 |
| 03 | 首存 | 满足 | 基本满足 | RMB/USDT 两套事实口径、首存互斥、钱包类型、手动取消/审核体验 | 首存模板；deposit fact Service；exclusiveGroup；grant target policy；审核面板 |
| 04 | 续存 | 满足 | 基本满足 | Cycle 周期、月边界、同场馆限制、与首存/月充值互斥、消费端复查 | 续存模板；period scopeKey；Service Interface 查询历史充值；互斥规则和 release gate |
| 05 | 新手任务 | 满足 | 基本满足 | 6 子任务任务包 UI、随机礼包概率校验、风控审核、任务状态聚合、任务重试 | `TASK_CENTER` 插件；任务模板；approval-gate；任务进度 StateStore |
| 06 | VIP 周红利 | 满足 | 基本满足 | 周期领取窗口、周/月互斥开关、中心/场馆钱包切换、XXL-Job 扫发和主动领取双通道 | 周红利模板；Scheduler；period USER_LIMIT；wallet target policy；互斥配置 |
| 07 | VIP 月红利 | 满足 | 技术可满足，但不建议复活旧逻辑 | 旧系统硬编码过期、半月边界、周/月互斥漂移、僵尸配置 | 作为新月红利模板重建；不要迁移旧硬编码；明确半月 period |
| 08 | 生日红利 | 满足 | 基本满足 | 生日 fact、年度防重、闰年策略、站内信模板、每日扫描调度 | Birthday 模板；Scheduler；profile fact Service；yearly USER_LIMIT；notification outbox |
| 09 | 升级红利 | 满足 | 基本满足 | 4 触发入口、3 钱包类型、自动晋级计算、跨级补差策略、Kafka topic 差异 | Upgrade 模板；VIP_LEVEL_UPGRADED 标准事件；wallet target policy；升级 fact Service |
| 10 | 红包雨 | 满足 | 部分满足 | 秒级场次、WebSocket 推送、抢包并发、Redis 原子消费、空包、场次清理、运营停止 | 正式 `RED_ENVELOPE` 插件；Plugin StateStore；Counter/Lock Resource；Realtime Push Provider；Scheduler |
| 11 | 主题抽奖 | 满足 | 部分满足 | 多玩法、多奖池、必中名单、抽奖次数消费、奖池库存提交、实物奖、前端玩法 UI | 扩展 `LOTTERY` 插件；DrawPool StateStore；PhysicalGift reward；必中名单审计 |
| 12 | 足球射门 | 满足 | 部分满足 | 立方分布算法、VIP/custom 优先级、奖池 INCR 回滚、射门动画和玩法记录 | `LOTTERY` 子策略或 `FOOTBALL_SHOT` 插件；可审计随机；Counter Resource |
| 13 | 幸运翻卡 | 满足 | 部分满足 | 卡池 SPOP、翻卡记录、两步手动开奖、号码唯一性、排行榜二重礼、实物/礼金混合 | `LUCKY_CARD` 插件；CardPool StateStore；Plugin Operation 设置号码/执行开奖；Ranking Service；PhysicalGift |
| 14 | 嘉年华 | 满足 | 部分满足，复杂度最高 | 投票对象管理、票数计算、实时 ZSET 榜、多轮晋级、粉丝榜、Excel 导入、晋级清榜 | `CARNIVAL_VOTE` 插件；Ranking Service；Vote Counter；Plugin Operation 晋级；Excel/import Studio |
| 15 | 签到有礼 | 满足 | 基本满足 | 补签策略、连续/累计/满签语义、周期日历、人工审核、签到状态 UI | Sign-in 模板；Sign StateStore；period quota；approval-gate；日历 Studio |
| 16 | 活动任务体系 | 满足 | 部分满足 | 分组/标签/任务配置、活跃度、排行榜、周期重置、任务快照、任务中心 UI | `TASK_CENTER` 插件；Task StateStore；Ranking Service；Scheduler 结算 |
| 17 | 王者回归 | 满足 | 部分满足 | 4 子活动互斥/叠加、老用户资格、未投注天数、邀请码安全、主题页统一 | Kings Return 模板包；exclusiveGroup；invite Service；profile/turnover facts；主题页 config |
| 18 | 赛事竞猜 | 满足 | 部分满足 | 竞猜选项、结果录入、中奖用户集合、固定奖池瓜分、手动派奖、合规审计 | `PREDICTION_POOL` 插件；PoolSplit reward；Plugin Operation 录入赛果/结算；Ranking/StateStore |
| 19 | 游戏活动 | 满足 | 基本满足 | 场馆钱包限制、游戏场馆事实、风险审核、连续签到/累计达标语义 | Game Activity 模板；venue fact Service；risk-score；approval-gate |
| 20 | 投注/首存豪礼 | 满足 | 部分满足 | 实物奖、收货地址加密、物流状态、发货审核、库存回滚、隐私审计 | `PHYSICAL_GIFT` reward 子系统；Gift Fulfillment；PII 加密；库存 quota；物流 Studio |
| 21 | 每月充值 | 满足 | 基本满足 | 首日参加、每日充值达标记录、满签奖、互斥并发保护、自动/手动审核 | Monthly Recharge 模板；daily deposit fact；period StateStore；exclusiveGroup；approval-gate |

### 不满足的不是基础链路，而是生产玩法闭环

下表把“不完整满足”的能力按平台扩展点归类。后续实现时优先补平台能力，再落单个活动模板。

| 缺口类型 | 影响活动 | 需要扩展 |
| --- | --- | --- |
| 玩法状态 | 红包雨、抽奖、足球射门、幸运翻卡、嘉年华、赛事竞猜、任务体系、签到 | `marketing_plugin_state`、`marketing_plugin_state_event`、插件 StateStore API |
| 实时计数/并发 | 红包雨、嘉年华投票、足球射门、主题抽奖、签到、邀请申请 | Redis-backed Counter/Lock Resource，PG 权威事件补偿 |
| 排行榜 | 嘉年华、活动任务体系、幸运翻卡二重礼、粉丝榜、竞猜结果集合 | Ranking Service：ZSET + snapshot + settle + archive |
| 定时和结算 | 返水、生日、VIP 周/月、红包场次、排行榜结算、满签、竞猜结算 | Scheduler Worker：claim、retry、幂等 event inbox、运行记录 |
| 手动操作 | 幸运翻卡开奖、嘉年华晋级、赛事竞猜录入结果、批量审核、实物发货 | Plugin Operation Framework：analyze、preview、approve、execute、rollback、audit |
| C 端动作 API | 抢红包、抽奖、翻卡、投票、竞猜、签到、领取任务奖励 | Plugin Runtime OpenAPI：state、actions、records、rankings |
| 实物奖 | 投注/首存豪礼、主题抽奖、幸运翻卡 | Physical Gift Fulfillment：地址加密、库存、审核、发货、物流、隐私审计 |
| 复杂运营后台 | 任务中心、嘉年华、翻卡、竞猜、红包雨、邀请好友 | Studio 插件页面：配置、导入、预览、运行态、结算、审计 |

## 完全满足 21 类活动的扩展路线

### 阶段 1：补生产底座

先补平台能力，避免每个活动各自实现锁、状态、调度和审计。

| 能力 | 产物 |
| --- | --- |
| Plugin State Store | 状态表、状态事件表、CAS、幂等写、状态审计 |
| Scheduler Worker | job 注册、claim、retry、生成标准事件、运行记录、补偿重跑 |
| Ranking Service | rank incr/top/memberRank/snapshot/settle/archive |
| Plugin Operation Framework | 手动操作预览、审批、执行、回滚、审计 |
| Plugin Runtime API | C 端 state/action/records/rankings 统一入口 |

### 阶段 2：补普通资金活动模板

优先把基础可满足活动做成生产模板：

```text
首存、续存、返水、生日红利、升级红利、VIP 周红利、每月充值、签到有礼
```

每个模板必须带：

```text
标准事件契约
Fact Service 依赖
quota 默认值
reward 默认值
审批要求
对账要求
release checklist
```

### 阶段 3：补任务和邀请类活动

```text
邀请好友、新手任务、活动任务体系、王者回归、游戏活动
```

重点能力：

```text
双主体关系
邀请归因
任务进度
任务组/标签/任务
活跃度
任务排行榜
互斥规则
```

### 阶段 4：补互动玩法插件

```text
RED_ENVELOPE
LOTTERY
FOOTBALL_SHOT
LUCKY_CARD
CARNIVAL_VOTE
PREDICTION_POOL
```

每个插件必须提供：

```text
config schema
state schema
action schema
runtime context
state-change intents
settlement operation
audit explain
Studio 页面
```

### 阶段 5：补实物奖和模板市场

```text
PHYSICAL_GIFT reward
Gift Fulfillment
地址加密和脱敏
物流状态
库存回滚
活动模板市场
C 端页面协议
运营数据看板
```

## 设计原则

后续扩展必须遵守：

1. Workflow Engine 只负责通用主链路：事件、规则、奖励、quota、grant-order、outbox、审计。
2. ActivityPlugin 只负责玩法上下文、玩法状态、候选奖励和结算意图，不直接发钱、不直接写 outbox。
3. Plugin StateStore 保存过程状态，但不能替代 participation、grant_order、quota、outbox。
4. Redis 只能做实时加速和原子计数，PostgreSQL 必须有权威状态或可重放事件。
5. 手动开奖、晋级、结算、审核和发货必须有 preview、审批、执行、审计和可恢复路径。
6. 复杂活动优先沉淀成模板和插件页面，不要把旧 ACT 的专用 controller/table/service 逐个搬进 ExMarketing。
