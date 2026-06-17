# ExMarketing Workflow 节点分类清单

本文档记录当前 Workflow Studio 支持的节点分类、节点职责和执行状态。

## 分类总览

当前节点分为 4 类：

| 分类 | 中文名 | 定位 |
| --- | --- | --- |
| `EVENT_INPUT` | 事件输入 | 描述事件从哪里来，以及标准事件如何进入 workflow |
| `CORE` | 核心链路 | 默认生产奖励链路节点 |
| `ADVANCED` | 高级集成 | 条件、脚本、转换、外部集成和回调扩展节点 |
| `OPS` | 运维节点 | 审计、重试、死信、对账和告警 |

## EVENT_INPUT 事件输入

事件输入是节点分类，不是单个节点。所有入口最终都应产出标准事件，然后进入标准事件入口校验和 Inbox 幂等链路。

| 节点类型 | 中文名 | 职责 | 当前状态 | 关键输出 |
| --- | --- | --- | --- | --- |
| `event-in` | 标准事件入口 | 校验已标准化事件的 `eventType`、`sourceSystem`、`tenant`，输出 workflow raw event | 已实现 | `raw_event`, `eventId`, `eventType`, `tenantId` |
| `http-event` | HTTP 接口事件 | 描述外部系统通过 `/openapi/v1/events` 主动上报标准事件 | 解释型，真实入口已存在 | `standard_event`, `source`, `adapter` |
| `webhook-event` | Webhook 映射事件 | 描述第三方 webhook payload 经签名校验和 mapping 转为标准事件 | 解释型，真实入口已存在 | `standard_event`, `source`, `adapter` |
| `mq-event` | MQ Topic 事件 | 描述监听 Kafka/RabbitMQ/Pulsar topic 并映射消息为标准事件 | 已完成 trigger adapter 第一版 | `standard_event`, `source`, `consumerPlan` |
| `schedule-event` | 定时事件 | 描述 cron/延迟任务生成标准事件 | 已完成 Scheduler adapter plan 第一版 | `standard_event`, `source`, `schedulePlan` |
| `state-event` | 状态监听事件 | 描述监听 Redis、配置、指标或状态条件并生成标准事件 | 已完成 ResourceAPI 快照判断第一版 | `standard_event`, `source`, `stateSnapshot`, `triggered` |
| `manual-event` | 人工触发事件 | 描述管理台人工补偿、重放或手动执行标准事件 | 待收口生产补偿入口 | `standard_event`, `source`, `adapter` |

## CORE 核心链路

核心链路是默认生产奖励流程。默认模板不依赖高级节点或运维节点即可完成事件接入、幂等、匹配、资格、奖励、额度、参与、发奖和发件箱。

推荐主链路有两种层级。

基础奖励链路：

```text
event-in -> inbox -> normalize -> match -> fact-query -> qualify -> reward -> limit -> quota -> participation -> grant-order -> outbox
```

生产活动链路：

```text
event-in -> inbox -> normalize -> match -> fact-query -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

| 节点类型 | 中文名 | 职责 | 关键输出 |
| --- | --- | --- | --- |
| `inbox` | 幂等收件箱 | 写入事件收件箱，处理重复事件和 payload 冲突 | `event_context` |
| `normalize` | 事件标准化 | 字段映射、默认值、时间格式、租户和业务主体规范化 | `normalized_event` |
| `match` | 活动匹配 | 按租户、事件类型、活动状态、时间窗和优先级匹配活动 | `matched_activity` |
| `fact-query` | 事实查询 | 查询用户、订单、交易、历史参与或外部事实数据；详见 [Fact Query 事实查询节点](fact-query-node.zh-CN.md) | `facts` |
| `activity-state` | 活动状态 | 读取或更新用户活动状态、任务进度、签到状态和插件状态 | `activity_state`, `state_version` |
| `state-store` | 通用存储 | 通过资源管理读写工作流状态、公共表、Redis 语义资源或 HTTP Provider | `stored_value`, `storage_entry`, `storage_version` |
| `quota-check` | 次数频控校验 | 对用户、设备、IP、活动或租户维度做次数和频率校验 | `quota_check`, `limit_hold` |
| `qualify` | 资格校验 | 执行资格、风控、名单、规则判断 | `stage_result`, `PASS`, `BLOCK`, `RISK`, `decision` |
| `candidate-reward` | 候选奖励 | 生成待发奖励候选，不直接产生资金副作用 | `candidate_rewards`, `reward_items` |
| `reward` | 奖励计算 | 计算奖励明细 | `reward_items` |
| `limit` | 次数限制 | 用户、活动、周期、事件维度次数限制 | `limit_hold` |
| `quota` | 预算库存扣减 | 预算、库存、用户额度预占和扣减 | `quota_hold` |
| `budget-reserve` | 预算库存锁定 | 在发奖前锁定活动预算、奖品库存或用户额度，避免并发超发 | `budget_reservation`, `quota_hold` |
| `participation` | 参与记录 | 写入参与流水，关联事件、活动、奖励、quota 和阶段结果 | `participation_id` |
| `grant-order` | 发奖订单 | 生成发奖订单和状态机记录 | `grant_order`, `grant_orders` |
| `outbox` | 消息发件箱 | 生成可重试投递消息，由 outbox worker 发送 | `HTTP`, `MQ`, `Webhook`, `Failed` |

### `quota-check` 次数频控校验

`quota-check` 用来判断某个主体在某个限制范围和时间窗口内是否还能继续参加活动。它不负责发奖，也不负责资格规则判断；它只负责频控计数、超限决策和输出可路由的校验结果。

常见用法：

- 每日签到：同一个用户、同一个活动、每天最多成功 1 次。
- 首存活动：同一个用户、同一个活动生命周期内最多参加 1 次。
- 风控活动：同一个设备、IP 或业务单号在指定窗口内最多触发 N 次。
- 自定义活动期：通过模板或 Lua 生成活动期时间段，例如 `promo-june`、`2026Q2`、`round-3`。

关键配置：

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `resourceRef` | 可选，引用资源管理里的 `REDIS_COUNTER` 计数器；配置后会走真实资源计数器 | `daily_checkin_counter` |
| `namespace` | 计数命名空间，用来隔离不同业务计数 | `checkin` |
| `scope` | 限制范围，说明按用户、活动、租户、业务、IP 或设备限制 | `USER` |
| `scopeKeyTemplate` | 逻辑主体 key 模板，决定“谁”被限制 | `${subject.id}:${activityId}` |
| `window` | 时间窗口，支持 `ACTIVITY`、`DAY`、`WEEK`、`MONTH`、`ROLLING_WINDOW`、`CUSTOM_RANGE` | `DAY` |
| `maxCount` | 窗口内最大允许次数 | `1` |
| `consumeMode` | 占用模式，决定只检查还是检查并消耗次数 | `ATOMIC` |
| `exceedAction` | 超限动作，决定超限后阻断、打风险或跳过 | `BLOCK` |

`计数维度` 决定“按谁、在哪个时间段、最多允许几次”。它由 `scope`、`scopeKeyTemplate`、`window` 和 `maxCount` 共同组成。

`限制范围 scope`：

- `USER`：按用户限制。最常见，例如“每个用户每天签到 1 次”。
- `ACTIVITY`：按活动整体限制。例如“整个活动每天最多发放 1000 次奖励”。
- `TENANT`：按租户限制。例如“每个站点每天最多触发 10000 次”。
- `BIZ`：按业务单号或业务对象限制。例如“同一笔存款只能参加一次首存活动”。
- `IP`：按 IP 限制。适合风控类频控，例如同一 IP 每小时最多 N 次。
- `DEVICE`：按设备限制。适合防刷或设备维度活动限制。

`Scope Key 模板 scopeKeyTemplate`：

- 这是频控的逻辑主体 key，决定具体限制对象。
- 模板可以使用事件、活动和上下文变量，例如 `${subject.id}`、`${activityId}`、`${tenantId}`、`${eventId}`。
- 推荐把活动维度放进去，避免不同活动之间互相影响。
- 推荐格式：`${subject.id}:${activityId}`、`${tenantId}:${activityId}:${subject.id}`、`${biz.bizId}:${activityId}`。
- 不建议只写 `${subject.id}`，除非明确希望同一个用户跨多个活动共享次数。

`窗口 window`：

- `DAY`：自然日窗口。适合每日签到、每日任务。
- `WEEK`：自然周窗口。适合每周任务。
- `MONTH`：自然月窗口。适合月度活动。
- `ACTIVITY`：活动生命周期窗口。适合首存、一次性报名、整个活动最多参加一次。
- `ROLLING_WINDOW`：滚动窗口。适合“最近 1 小时最多 N 次”这类风控频率限制。
- `CUSTOM_RANGE`：自定义时间段。适合活动期、赛季、轮次、运营配置周期；可以通过模板或 Lua 输出 `segmentKey`。

`最大次数 maxCount`：

- 表示同一个 `scopeKeyTemplate + window segment` 下允许通过的最大次数。
- `maxCount=1` 是“只允许一次”的语义。
- `maxCount=3` 是“该窗口内最多三次”的语义。
- 当 `consumeMode=ATOMIC` 时，每次通过都会立即占用一次；达到上限后按 `exceedAction` 处理。

逻辑 key 生成规则：

```text
logicalKey = scopeKeyTemplate 渲染结果 + ":" + windowSegment
```

示例：

| 场景 | scope | scopeKeyTemplate | window | maxCount | logicalKey 示例 |
| --- | --- | --- | --- | --- | --- |
| 每日签到一次 | `USER` | `${subject.id}:${activityId}` | `DAY` | `1` | `u_001:act_checkin:20260615` |
| 首存活动一次 | `BIZ` | `${biz.bizId}:${activityId}` | `ACTIVITY` | `1` | `dep_001:act_first_deposit:activity` |
| 用户每周 3 次 | `USER` | `${subject.id}:${activityId}` | `WEEK` | `3` | `u_001:act_weekly:2026W25` |
| 同 IP 每小时 10 次 | `IP` | `${attributes.ip}:${activityId}` | `ROLLING_WINDOW` | `10` | `1.2.3.4:act_risk:rolling:3600s:494444` |
| 活动轮次最多 2 次 | `USER` | `${subject.id}:${activityId}` | `CUSTOM_RANGE` | `2` | `u_001:act_promo:round-3` |

`处理策略` 是节点拿到频控结果之后的动作配置，主要由 `consumeMode` 和 `exceedAction` 组成。

`consumeMode`：

- `CHECK_ONLY`：只检查，不增加次数。适合预检查，例如“用户今天是否还可以签到”。
- `ATOMIC`：检查并立即加次数。适合真实执行，例如“签到成功后占用今天 1 次机会”。生产发奖链路通常优先使用该模式。
- `RESERVE_ONLY`：预占用，适合先锁住名额再由后续确认或释放。当前如果没有完整 confirm/release 链路，需要谨慎使用。

`exceedAction`：

- `BLOCK`：超过次数后直接阻断流程，节点失败。适合每日签到、首存奖励这类不能重复发奖的场景。
- `RISK`：不阻断流程，但输出风险标记，后续可以进入风控、人工审核或降级分支。
- `SKIP`：跳过后续奖励或业务动作，流程本身不视为异常。适合“已参加过则不重复发”的轻量场景。

每日签到奖励推荐配置：

```json
{
  "resourceRef": "daily_checkin_counter",
  "namespace": "checkin",
  "scope": "USER",
  "scopeKeyTemplate": "${subject.id}:${activityId}",
  "window": "DAY",
  "maxCount": 1,
  "consumeMode": "ATOMIC",
  "exceedAction": "BLOCK"
}
```

这表示：同一个 `subject.id + activityId + DAY` 下，第一次签到会原子加 1 并通过；当天第二次再进入时会超限并阻断，不继续发奖励。

自定义时间段可以用 `CUSTOM_RANGE`：

```json
{
  "resourceRef": "promo_counter",
  "namespace": "promo",
  "scope": "USER",
  "scopeKeyTemplate": "${subject.id}:${activityId}",
  "window": "CUSTOM_RANGE",
  "windowSegmentTemplate": "promo-2026Q2",
  "windowStartTemplate": "2026-04-01T00:00:00Z",
  "windowEndTemplate": "2026-06-30T23:59:59Z",
  "maxCount": 3,
  "consumeMode": "ATOMIC",
  "exceedAction": "BLOCK"
}
```

也可以用 Lua 动态生成时间段：

```json
{
  "window": "CUSTOM_RANGE",
  "windowLuaSource": "emit({segmentKey = 'promo-june', windowStart = '2026-06-01T00:00:00Z', windowEnd = '2026-06-30T23:59:59Z'})"
}
```

节点输出中的 `quota_check.logicalKey` 会把主体 key 和时间段拼起来，例如 `user_001:activity_001:20260615` 或 `user_001:activity_001:promo-june`。资源适配器再根据 `namespace`、资源配置和 `logicalKey` 生成物理存储 key。

## ADVANCED 高级集成

高级节点用于扩展主链路，不应该成为默认生产模板的必需节点。

| 节点类型 | 中文名 | 职责 | 当前状态 | 关键输出 |
| --- | --- | --- | --- | --- |
| `combine` | 组合节点 | 使用 Lua 或表达式把多个上游输入组合成一个结构化结果 | 已实现，Lua 沙箱，可调用授权 Resource API | `combined_result` |
| `split` | 拆分节点 | 使用 Lua 或表达式把结构化输入拆成数组或多个分片 | 已实现，Lua 沙箱，可调用授权 Resource API | `split_items`, `split_result` |
| `condition` | 条件判断 | 根据表达式输出 IF / ELSE 分支 | 已实现规则表达式/JSON Logic | `IF`, `ELSE`, `stage_result` |
| `switch` | 条件分支 | 根据阶段结果选择多个业务分支 | 已实现解释/路由语义 | `PASS`, `BLOCK`, `SKIP`, `route` |
| `code` | 代码执行 | 使用受控 Lua 或表达式处理上下文并输出中间变量 | 已实现 Lua/Expression；Lua 可调用授权 Resource API | `code_result` |
| `decimal` | 精度处理 | 将金额解析为固定 4 位小数字符串 | 已实现当前 Decimal 契约 | `decimal_amount` |
| `approval-gate` | 审批网关 | 按金额、风险或策略决定自动通过、阻断或进入人工审批 | 外部审批权威 | `approval_decision` |
| `compensation` | 业务补偿 | 生成发奖、预算、回调、通知失败后的补偿计划 | 解释型/后台任务边界 | `compensation_plan` |
| `random-pick` | 随机抽取 | 执行可审计的稳定随机、奖池抽取或分桶随机 | 已实现稳定 hash 抽取 | `random_pick`, `selected_rewards` |
| `plugin-execute` | 插件执行 | 调用签到、抽奖、红包等活动插件动作 | 外部插件权威 | `plugin_result`, `candidate_rewards` |
| `ab-bucket` | AB 分桶 | 按稳定主体分配实验桶或运营策略桶 | 已实现稳定分桶 | `ab_bucket` |
| `risk-score` | 风险评分 | 根据事实、规则或外部风控结果输出风险评分和标签 | 已实现 facts/rule 输出 | `risk_result` |
| `settlement-window` | 结算窗口 | 限制发奖、对账或结算只能在指定窗口内执行 | 已实现窗口判定/调度边界 | `settlement_window` |
| `manual-adjust` | 人工调整 | 记录人工补偿、修正、加减奖励或状态修复意图 | 外部 Admin API 权威 | `manual_adjustment` |
| `http-call` | HTTP 发奖 | 描述 HTTP 投递意图，真实发送由 outbox worker 负责 | 解释型 | `channel`, `messageCount` |
| `mq-publish` | MQ 发奖 | 描述 MQ 投递意图；配置 `MQ_TOPIC resourceRef` 时生成受控发布意图并记录资源审计 | 部分资源化；真实 broker ack 仍由 OutboxWorker/MQ sender 负责 | `mq_result`, `MQ` |
| `webhook` | Webhook 通知 | 描述 Webhook 通知投递意图，真实发送由 outbox worker 负责 | 解释型 | `channel`, `messageCount` |
| `callback` | 结果回调 | 描述等待或处理外部发奖回调的边界 | 外部 endpoint 语义 | `callback_result` |

## OPS 运维节点

运维节点用于可观测性、恢复、对账和告警，不属于默认业务主链路。

| 节点类型 | 中文名 | 职责 | 当前状态 | 关键输出 |
| --- | --- | --- | --- | --- |
| `audit` | 审计记录 | 记录事件、规则、奖励、quota、outbox、callback 全链路证据 | 已实现解释输出 | `audit_log` |
| `retry` | 失败重试 | 描述失败消息的重试策略和重放入口 | 解释型 | `retryPolicy`, `messageCount` |
| `dlq` | 死信队列 | 描述死信队列、人工修复和重新投递入口 | 解释型 | DLQ 解释输出 |
| `reward-reconcile` | 奖励对账 | 对发奖订单、预算锁定、回调和 outbox 做奖励链路一致性校验 | 解释型/Admin API 边界 | `reward_reconcile_report` |
| `reconcile` | 对账校验 | 对 grant、quota、callback、outbox 做一致性扫描和修复建议 | 解释型/Admin API 边界 | `reconciliationTracked` |
| `alert` | 告警通知 | 描述关键失败、DLQ 增长、回调失败和预算异常告警 | 解释型/指标系统边界 | `reason` |

## 使用原则

1. `EVENT_INPUT` 负责事件来源和入口适配；`event-in` 是标准事件进入 workflow 的校验节点。
2. `CORE` 负责生产奖励主链路，默认模板应优先使用 CORE 节点。
3. `ADVANCED` 用于扩展、转换、脚本和外部集成，不应默认强依赖。
4. `OPS` 用于审计、恢复、对账和告警，不能伪装成同步业务执行。
5. 解释型节点必须明确 `executionMode` 和 `executionAuthority`，不能让用户误以为 workflow 节点本身完成了外部投递、MQ 消费、回调处理或告警发送。

## 资源化能力状态

| 能力 | 当前状态 | 边界 |
| --- | --- | --- |
| `DATABASE_TABLE` | `state-store + resourceRef` 已支持 | 底层为受控虚拟公共表，不直接开放任意 SQL 表写入 |
| `HTTP_PROVIDER` | `state-store/fact-query/outbox/callback + resourceRef` 已支持第一版 | 已有 URL/allowlist/private network/redirect 防护第一版；还缺 secretRef 和 response schema 强校验 |
| `REDIS_KV/HASH/COUNTER/LOCK` | `state-store + resourceRef` 已支持第一版 | 当前由 `marketing_storage_state` 承载 Redis 语义，还不是真 Redis client |
| `MQ_TOPIC` | `mq-publish/outbox + resourceRef` 已支持第一版 | 节点生成 publish intent 或 dispatch plan，真实 broker ack 仍由 OutboxWorker/MQ sender |
| Lua Resource API | `code/combine/split/lua-query` 的 service 执行路径已支持 | 只能访问通过资源权限和 binding 授权的资源，并受调用次数/响应大小限制 |

仍未完成：

- 真实 Redis client adapter。
- 生产 Kafka/RabbitMQ/Pulsar transport SDK 注入。
- 独立资源审批单。
- `mq-event` broker loop/offset/replay UI、`schedule-event` 统一调度注册表、`state-event` watcher cursor/debounce。
