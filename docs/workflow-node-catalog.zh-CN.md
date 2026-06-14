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
| `mq-event` | MQ Topic 事件 | 描述监听 Kafka/RabbitMQ/Pulsar topic 并映射消息为标准事件 | 待实现入站 consumer | `standard_event`, `source`, `adapter` |
| `schedule-event` | 定时事件 | 描述 cron/延迟任务生成标准事件 | 待实现/可复用调度能力 | `standard_event`, `source`, `adapter` |
| `state-event` | 状态监听事件 | 描述监听 Redis、配置、指标或状态条件并生成标准事件 | 待实现 watcher | `standard_event`, `source`, `adapter` |
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
| `fact-query` | 事实查询 | 查询用户、订单、交易、历史参与或外部事实数据 | `facts` |
| `activity-state` | 活动状态 | 读取或更新用户活动状态、任务进度、签到状态和插件状态 | `activity_state`, `state_version` |
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

## ADVANCED 高级集成

高级节点用于扩展主链路，不应该成为默认生产模板的必需节点。

| 节点类型 | 中文名 | 职责 | 当前状态 | 关键输出 |
| --- | --- | --- | --- | --- |
| `combine` | 组合节点 | 使用 Lua 或表达式把多个上游输入组合成一个结构化结果 | 已实现，Lua 沙箱 | `combined_result` |
| `split` | 拆分节点 | 使用 Lua 或表达式把结构化输入拆成数组或多个分片 | 已实现，Lua 沙箱 | `split_items`, `split_result` |
| `condition` | 条件判断 | 根据表达式输出 IF / ELSE 分支 | 已实现规则表达式/JSON Logic | `IF`, `ELSE`, `stage_result` |
| `switch` | 条件分支 | 根据阶段结果选择多个业务分支 | 已实现解释/路由语义 | `PASS`, `BLOCK`, `SKIP`, `route` |
| `code` | 代码执行 | 使用受控 Lua 或表达式处理上下文并输出中间变量 | 已实现 Lua/Expression | `code_result` |
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
| `mq-publish` | MQ 发奖 | 描述 MQ 投递意图，真实发送由 outbox worker 负责 | 解释型 | `channel`, `messageCount` |
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
