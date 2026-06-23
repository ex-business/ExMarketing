# ExMarketing Workflow 节点分类清单

本文档记录当前 Workflow Studio 支持的节点分类、节点职责和执行状态。

## 分类总览

Workflow Studio 的节点面板推荐按产品使用方式分为 4 类。这个分类面向活动编排用户，决定节点在“添加节点”和节点介绍里的展示方式。

| 分类 | 定位 | 展示建议 |
| --- | --- | --- |
| `基础节点` | 生产活动主链路和最常用业务节点 | 默认展开 |
| `高级节点` | 可选增强能力，例如用户分层、事实标准化、风控、插件、审批、对账 | 默认折叠 |
| `组合节点` | 数据组合、脚本处理、条件分支和表达式编排 | 默认折叠 |
| `其他节点` | 事件适配、底层查询、投递边界和运维辅助 | 默认折叠 |

说明：下文的 `EVENT_INPUT`、`CORE`、`ADVANCED`、`OPS` 是执行域和历史实现分类，用于解释节点边界；Studio 节点面板和节点介绍应优先使用上面的产品展示分类。

### Studio 展示分类

#### 基础节点

基础节点覆盖一条生产活动链路从事件进入到发奖出站的主流程。普通配置型活动应优先使用这些节点。

| 节点类型 | 中文名 | 放入基础节点的原因 |
| --- | --- | --- |
| `event-in` | 标准事件入口 | 所有标准事件进入 workflow 的入口 |
| `inbox` | 幂等收件箱 | 处理事件幂等和重复 payload |
| `normalize` | 事件标准化 | 把事件、主体、业务字段整理成统一上下文 |
| `match` | 活动匹配 | 找到事件命中的活动 |
| `fact-query` | 事实查询 | 获取资格、奖励、频控所需事实 |
| `activity-state` | 活动状态 | 读取或写入活动内业务状态，是状态获取的基础节点 |
| `quota-check` | 次数频控校验 | 通过 RESOURCE 读取或占用计数器 |
| `qualify` | 资格校验 | 输出 PASS/BLOCK/RISK 等业务决策 |
| `candidate-reward` | 候选奖励 | 从奖励来源生成待发奖励候选 |
| `budget-reserve` | 预算库存锁定 | 发奖前锁定预算或库存 |
| `participation` | 参与记录 | 写入参与事实 |
| `grant-order` | 发奖订单 | 创建发奖订单和状态机记录 |
| `outbox` | 消息发件箱 | 生成可重试投递消息 |
| `audit` | 审计记录 | 记录链路证据，基础链路也应该保留 |

#### 高级节点

高级节点用于增强基础链路，不应该成为普通模板的必选项。

| 节点类型 | 中文名 | 典型用途 |
| --- | --- | --- |
| `user-level-check` | 用户等级判断 | 按用户等级、VIP 等级或事件属性做分层 |
| `facts-compose` | 事实映射合并 | 多个事实查询结果合并为统一 `facts` |
| `facts-normalize` | 事实标准化 | 把不同来源事实标准化为 `standard_context` |
| `risk-score` | 风险评分 | 输出风险分、风险标签或黑名单结果 |
| `ab-bucket` | AB 分桶 | 运营策略或实验分桶 |
| `random-pick` | 随机抽取 | 奖池抽取、稳定随机或分桶抽奖 |
| `plugin-execute` | 插件执行 | 调用签到、抽奖、红包等玩法插件 |
| `approval-gate` | 审批网关 | 高风险或高金额动作进入审批 |
| `settlement-window` | 结算窗口 | 限制发奖、结算或对账窗口 |
| `manual-adjust` | 人工调整 | 人工补偿、修正或状态修复 |
| `compensation` | 业务补偿 | 生成失败后的补偿计划 |
| `reward-reconcile` | 奖励对账 | 奖励链路一致性检查 |
| `reconcile` | 对账校验 | quota、grant、callback、outbox 一致性扫描 |

#### 组合节点

组合节点用于把上游数据变成下游需要的结构，或者表达分支、条件和脚本逻辑。

| 节点类型 | 中文名 | 典型用途 |
| --- | --- | --- |
| `combine` | 组合节点 | 合并多个上游输入 |
| `split` | 拆分节点 | 把数组或结构拆成分片 |
| `code` | 代码执行 | 受控 Lua 或表达式处理 |
| `lua-query` | Lua 查询 | 用 Lua 从上下文或授权资源取数 |
| `condition` | 条件判断 | IF/ELSE 分支 |
| `switch` | 条件分支 | PASS/BLOCK/SKIP 等多分支路由 |
| `decimal` | 精度处理 | 金额和数值精度标准化 |

#### 其他节点

其他节点不是不能用，而是默认不应摆在主链路第一层。它们包括事件适配、底层查询、投递边界和运维辅助。

| 节点类型 | 中文名 | 说明 |
| --- | --- | --- |
| `state-store` | 通用存储 | 底层状态读写能力，普通状态获取优先用 `activity-state` |
| `context-query` | 上下文查询 | 低层事实查询能力 |
| `local-cache-query` | 本地缓存查询 | 低层事实查询能力 |
| `http-query` | HTTP 查询 | 低层事实查询能力 |
| `redis-query` | Redis 查询 | 低层事实查询能力 |
| `grpc-query` | gRPC 查询 | 低层事实查询能力 |
| `plugin-query` | 插件查询 | 低层事实查询能力 |
| `custom-query` | 自定义查询 | 低层事实查询能力 |
| `http-call` | HTTP 发奖 | 投递意图，真实发送由 outbox worker 完成 |
| `mq-publish` | MQ 发奖 | MQ 投递意图 |
| `webhook` | Webhook 通知 | Webhook 投递意图 |
| `callback` | 结果回调 | 外部回调边界 |
| `retry` | 失败重试 | 运维恢复 |
| `dlq` | 死信队列 | 运维恢复 |
| `alert` | 告警通知 | 监控告警边界 |
| `http-event` | HTTP 接口事件 | 事件来源适配 |
| `webhook-event` | Webhook 映射事件 | 事件来源适配 |
| `mq-event` | MQ Topic 事件 | 事件来源适配 |
| `schedule-event` | 定时事件 | 事件来源适配 |
| `state-event` | 状态监听事件 | 事件来源适配 |
| `manual-event` | 人工触发事件 | 事件来源适配 |

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
event-in -> inbox -> normalize -> match -> fact-query -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

多事实生产活动链路：

```text
event-in -> inbox -> normalize -> match -> fact-query -> facts-compose -> facts-normalize -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

| 节点类型 | 中文名 | 职责 | 关键输出 |
| --- | --- | --- | --- |
| `inbox` | 幂等收件箱 | 写入事件收件箱，处理重复事件和 payload 冲突 | `event_context` |
| `normalize` | 事件标准化 | 字段映射、默认值、时间格式、租户和业务主体规范化 | `normalized_event` |
| `match` | 活动匹配 | 按租户、事件类型、活动状态、时间窗和优先级匹配活动 | `matched_activity` |
| `fact-query` | 事实查询 | 查询用户、订单、交易、历史参与或外部事实数据；详见 [Fact Query 事实查询节点](fact-query-node.zh-CN.md) | `facts` |
| `activity-state` | 活动状态 | 默认读写 ExMarketing 内部活动状态；需要跨系统状态时才选择 RESOURCE 并填写资源 Ref | `activity_state`, `state_version` |
| `state-store` | 通用存储 | 通过资源管理读写工作流状态、公共表、Redis 语义资源或受控 HTTP Resource | `stored_value`, `storage_entry`, `storage_version` |
| `quota-check` | 次数频控校验 | 通过 RESOURCE 操作块读取或占用计数器，对用户、设备、IP、活动或租户维度做次数和频率校验 | `quota_check`, `limit_hold` |
| `qualify` | 资格校验 | 执行资格、风控、名单、规则判断；详见 [Qualify 资格校验节点](qualify-node.zh-CN.md) | `stage_result`, `PASS`, `BLOCK`, `RISK`, `decision` |
| `candidate-reward` | 候选奖励 | 通过 RESOURCE 操作块读取奖励来源并生成待发奖励候选，不直接产生资金副作用 | `candidate_rewards`, `reward_items` |
| `budget-reserve` | 预算库存锁定 | 在发奖前锁定活动预算或奖品库存，避免并发超发 | `budget_reservation`, `quota_hold` |
| `participation` | 参与记录 | 写入参与记录，关联事件、活动、奖励、quota 和阶段结果 | `participation_id` |
| `grant-order` | 发奖订单 | 生成发奖订单和状态机记录 | `grant_order`, `grant_orders` |
| `outbox` | 消息发件箱 | 生成可重试投递消息，由 outbox worker 发送 | `HTTP`, `MQ`, `Webhook`, `Failed` |

### `activity-state` 活动状态

`activity-state` 用来读取或更新某个活动内的用户状态。它表达的是“这个活动里，这个主体当前处于什么状态”，例如签到状态、任务进度、插件状态、领取状态、连续天数快照等。

它不负责做复杂资格判断，也不负责次数频控；这些应分别放到 `qualify` 和 `quota-check`。`activity-state` 默认使用 ExMarketing 内部活动状态，不需要资源 Ref；只有读取外部状态资源时，才在 `providerChain` 里选择 `RESOURCE` 并填写资源 Ref。HTTP、Redis、gRPC、Plugin 等底层实现应封装在资源定义里，节点不直接填写 endpoint、redisRef、grpcTarget 或 pluginCode。

常见用法：

- 每日签到：读取用户今天的签到状态、连续签到天数、上次签到日期。
- 任务活动：读取任务完成进度，例如 `depositDone`、`tradeCount`、`claimed`。
- 抽奖活动：读取用户抽奖状态、剩余机会、最近一次抽奖结果。
- 插件玩法：读取插件维护的活动状态快照，供后续节点判断或执行插件动作。

#### 状态操作 providerChain

`activity-state` 的状态获取统一使用 `providerChain`，每个元素是一条状态操作。当前默认模板使用 `INTERNAL`，不需要资源 Ref：

```json
{
  "providerChain": [
    {
      "source": "INTERNAL",
      "keyTemplate": "${activityId}:${subject.id}",
      "namespace": "checkin",
      "valuePath": "$.state",
      "outputAs": "currentState"
    }
  ]
}
```

字段说明：

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `source` | 数据来源类型。默认用 `INTERNAL` 读写 ExMarketing 内部活动状态；只有跨系统状态才用 `RESOURCE` | `INTERNAL` |
| `resourceRef` | 资源 Code。仅 `source=RESOURCE` 时需要；内部状态不填 | `default_workflow_state` |
| `keyTemplate` | 状态 key 模板，用来稳定定位“某活动下某用户”的状态 | `${activityId}:${subject.id}` |
| `namespace` | 状态命名空间，用于隔离签到、任务、抽奖、红包等不同状态域 | `checkin` |
| `valuePath` | 从资源响应中取状态对象的 JSONPath；默认内部状态使用约定字段 | `$.state` |
| `outputAs` | 本步骤输出别名，后续步骤或调试输出可引用 | `currentState` |

说明：`activity-state` 的读取操作由节点类型自动推断为 `READ`，UI 不再要求用户选择底层 `operation` 或 `miss` 策略。

#### Key 和命名空间

推荐把状态 key 设计成“活动 + 主体”：

```text
${activityId}:${subject.id}
```

这样同一个用户在不同活动下状态相互隔离。不要只用 `${subject.id}`，除非明确希望用户跨多个活动共享同一份状态。

`namespace` 用来区分业务域：

| 场景 | namespace | keyTemplate |
| --- | --- | --- |
| 每日签到 | `checkin` | `${activityId}:${subject.id}` |
| 任务活动 | `task` | `${activityId}:${subject.id}` |
| 抽奖活动 | `lottery` | `${activityId}:${subject.id}` |
| 红包活动 | `red-envelope` | `${activityId}:${subject.id}` |
| 插件状态 | `plugin` | `${activityId}:${pluginCode}:${subject.id}` |

#### READ 示例：每日签到状态

```json
{
  "type": "activity-state",
  "title": "签到活动状态",
  "config": {
    "providerChain": [
	      {
	        "source": "INTERNAL",
	        "namespace": "checkin",
	        "keyTemplate": "${activityId}:${subject.id}",
	        "valuePath": "$.state",
	        "outputAs": "currentState"
	      }
    ],
    "inputMapping": {
      "event_context": {
        "sourceNodeId": "normalize",
        "outputKey": "normalized_event"
      }
    }
  }
}
```

该配置表示：从 ExMarketing 内部活动状态里读取 `checkin` 命名空间下 `${activityId}:${subject.id}` 这条状态。节点输出中会包含：

```json
{
  "activity_state": {
    "namespace": "checkin",
    "key": "act_001:user_001",
    "data": {
      "lastCheckinDate": "2026-06-19",
      "streakDays": 8,
      "claimed": true
    }
  },
  "state_version": 3
}
```

#### 高级内部示例：写入签到状态

当某个后置节点确认签到成功后，执行器内部可以用 `UPSERT` 写回状态。该类底层操作不在普通属性面板默认暴露：

```json
{
  "type": "activity-state",
  "title": "写入签到状态",
  "config": {
    "providerChain": [
      {
        "source": "INTERNAL",
        "operation": "UPSERT",
        "namespace": "checkin",
        "keyTemplate": "${activityId}:${subject.id}",
        "valuePath": "$.input.state_patch",
        "outputAs": "updatedState",
        "onMiss": "BLOCK"
      }
    ]
  }
}
```

上游传入的 `state_patch` 可以是：

```json
{
  "lastCheckinDate": "2026-06-19",
  "streakDays": 9,
  "claimed": true
}
```

#### 多步骤状态链

执行器层的 `providerChain` 支持多步骤。比如先读内部活动状态，再把读到的状态写入另一个内部快照 key；普通 UI 不要求用户手动选择这些操作：

```json
{
  "providerChain": [
	    {
	      "source": "INTERNAL",
	      "namespace": "checkin",
	      "keyTemplate": "${activityId}:${subject.id}",
	      "outputAs": "currentState"
	    },
    {
      "source": "INTERNAL",
      "operation": "UPSERT",
      "namespace": "checkin_audit",
      "keyTemplate": "${activityId}:${subject.id}:${eventId}",
      "valuePath": "$.input.currentState_value",
      "outputAs": "stateSnapshot"
    }
  ]
}
```

如果这个状态来自外部系统，再把 `source` 改成 `RESOURCE` 并填写 `resourceRef`。

#### 和其他节点的边界

| 节点 | 负责什么 | 不负责什么 |
| --- | --- | --- |
| `activity-state` | 读取或写入活动内状态 | 不做复杂资格规则 |
| `quota-check` | 次数、频率、窗口计数 | 不保存业务状态对象 |
| `qualify` | 资格、风控、名单、规则判断 | 不直接读写状态资源 |
| `state-store` | 通用状态读写能力 | 不表达活动语义 |

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
| `providerChain` | 频控资源操作块。第一项 RESOURCE 操作用于指定计数器资源、计数 key、命名空间和操作 | 见下方示例 |
| `scope` | 限制范围，说明按用户、活动、租户、业务、IP 或设备限制 | `USER` |
| `scopeKeyTemplate` | 逻辑主体 key 模板，决定“谁”被限制 | `${subject.id}:${activityId}` |
| `window` | 时间窗口，支持 `ACTIVITY`、`DAY`、`WEEK`、`MONTH`、`ROLLING_WINDOW`、`CUSTOM_RANGE` | `DAY` |
| `maxCount` | 窗口内最大允许次数 | `1` |
| `consumeMode` | 占用模式，决定只检查还是检查并消耗次数 | `ATOMIC` |
| `exceedAction` | 超限动作，决定超限后阻断、打风险或跳过 | `BLOCK` |

#### 频控资源操作 providerChain

`quota-check` 也统一使用 `providerChain` RESOURCE 操作块。默认每日签到配置如下：

```json
{
  "providerChain": [
    {
      "source": "RESOURCE",
      "resourceRef": "default_checkin_quota_counter",
      "keyTemplate": "${subject.id}:${activityId}",
      "namespace": "checkin",
      "valuePath": "$.counter",
      "outputAs": "currentQuota"
    }
  ]
}
```

字段说明：

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `source` | 固定使用 `RESOURCE`，表示通过资源管理层访问受控计数器 | `RESOURCE` |
| `resourceRef` | 计数器资源 Code。每日签到默认使用内置 `REDIS mode=COUNTER` 资源 | `default_checkin_quota_counter` |
| `keyTemplate` | 计数主体 key。执行器会结合 `window` 生成最终 logicalKey | `${subject.id}:${activityId}` |
| `namespace` | 计数命名空间，用于区分签到、抽奖、任务、风控等计数域 | `checkin` |
| `valuePath` | 计数器值路径，通常使用约定值 | `$.counter` |
| `outputAs` | 资源操作输出别名 | `currentQuota` |

说明：频控的底层操作由 `consumeMode` 推断。`CHECK_ONLY` 对应只读检查；`ATOMIC` 对应检查并增加次数。UI 不再要求用户选择 `READ` 或 `CHECK_AND_INCREMENT`。

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

每日签到频控有两种常见放置方式。

资格校验前的预检查推荐使用 `consumeMode=CHECK_ONLY`。它只读取今天已有次数，不增加计数，避免用户资格失败时也被占用一次：

```json
{
  "providerChain": [
    {
      "source": "RESOURCE",
      "resourceRef": "default_checkin_quota_counter",
	      "keyTemplate": "${subject.id}:${activityId}",
	      "namespace": "checkin",
	      "valuePath": "$.counter",
	      "outputAs": "currentQuota"
    }
  ],
  "scope": "USER",
  "scopeKeyTemplate": "${subject.id}:${activityId}",
  "window": "DAY",
  "maxCount": 1,
  "consumeMode": "CHECK_ONLY",
  "exceedAction": "BLOCK"
}
```

资格通过、确认要发奖或确认签到成功之后，推荐使用 `consumeMode=ATOMIC` 做真实占用；底层会推断为检查并增加计数：

```json
{
  "providerChain": [
    {
      "source": "RESOURCE",
      "resourceRef": "default_checkin_quota_counter",
	      "keyTemplate": "${subject.id}:${activityId}",
	      "namespace": "checkin",
	      "valuePath": "$.counter",
	      "outputAs": "claimedQuota"
    }
  ],
  "scope": "USER",
  "scopeKeyTemplate": "${subject.id}:${activityId}",
  "window": "DAY",
  "maxCount": 1,
  "consumeMode": "ATOMIC",
  "exceedAction": "BLOCK"
}
```

第二种配置表示：同一个 `subject.id + activityId + DAY` 下，第一次签到会原子加 1 并通过；当天第二次再进入时会超限并阻断，不继续发奖励。

自定义时间段可以用 `CUSTOM_RANGE`：

```json
{
  "providerChain": [
    {
      "source": "RESOURCE",
      "resourceRef": "promo_counter",
	      "keyTemplate": "${subject.id}:${activityId}",
	      "namespace": "promo",
	      "valuePath": "$.counter",
	      "outputAs": "promoQuota"
    }
  ],
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

### `candidate-reward` 候选奖励

`candidate-reward` 用来生成“待发奖励候选”。它回答的是“这个用户通过资格后，理论上应该给什么奖励”，但它不直接扣预算、不写参与记录、不创建发奖订单，也不真正发送奖励。

它通常放在 `qualify` 的 `PASS` 分支之后、`budget-reserve` 之前：

```text
qualify(PASS) -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

常见用法：

- 每日签到：从活动奖励配置生成一条固定奖金候选。
- 任务活动：根据活动配置生成积分、奖金、券等奖励候选。
- 抽奖/红包：接收插件或随机节点输出的候选奖励，再交给预算库存节点锁定。
- A/B 活动：上游按桶决定奖励配置，`candidate-reward` 统一输出标准候选数组。

#### 节点边界

| 节点 | 负责什么 | 不负责什么 |
| --- | --- | --- |
| `candidate-reward` | 生成候选奖励数组，说明“可能要发什么” | 不扣预算、不发奖、不保证库存 |
| `budget-reserve` | 锁定预算或库存 | 不计算候选奖励，不做领取次数频控 |
| `grant-order` | 生成发奖订单 | 不决定用户是否有资格 |
| `outbox` | 生成可重试投递消息 | 不计算奖励金额 |

#### 输入

| 输入 | 来源 | 说明 |
| --- | --- | --- |
| `qualified_context` | `qualify.PASS` | 资格通过上下文。生产模板中从 `qualify` 的 `PASS` 输出映射过来 |
| `plugin_result` | `plugin-execute` 或上游节点 | 当 `source=PLUGIN_RESULT` 时，可从插件结果中读取候选奖励 |
| `candidate_rewards` / `reward_items` / `selected_rewards` | 上游节点输出 | 当 `source=INPUT` 或 `PLUGIN_RESULT` 时，可透传上游奖励数组 |
| 签到奖励配置 Redis Ref | `providerChain[0].resourceRef=checkin_reward_config_ref` | 签到默认来源。节点通过 RESOURCE 操作块读取 Redis 中的 `reward_items` 并生成候选奖励 |

#### 配置项

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `providerChain` | 奖励来源资源操作块。第一项 RESOURCE 操作用于指定从哪里读取奖励配置或候选奖励 | 见下方示例 |
| `includeDisabledRewards` | 是否包含活动配置中 disabled 的奖励。默认不包含 | `false` |
| `candidatePolicy` | 多个候选奖励命中时的选择策略 | `ALL_MATCHED` |
| `notQualifiedAction` | 用户未通过资格时的动作。`SKIP` 表示跳过候选生成；`BLOCK` 表示阻断 | `SKIP` |
| `emptyAction` | 候选为空时的动作。`BLOCK` 表示阻断；`SKIP` 表示跳过 | `BLOCK` |
| `includeTrace` | 是否保留 trace 配置。当前主要用于 UI 和审计语义 | `true` |

#### 奖励来源 providerChain

`candidate-reward` 的奖励来源也统一使用 `providerChain` RESOURCE 操作块：

```json
{
  "providerChain": [
    {
      "source": "RESOURCE",
      "resourceRef": "checkin_reward_config_ref",
      "keyTemplate": "${activityId}",
      "namespace": "reward",
      "valuePath": "$.reward_items",
      "outputAs": "rewardConfig"
    }
  ]
}
```

字段说明：

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `source` | 固定使用 `RESOURCE`，表示通过资源管理层访问受控奖励来源 | `RESOURCE` |
| `resourceRef` | 奖励来源资源 Code。签到默认使用 `checkin_reward_config_ref`，表示 Redis 中的签到奖励配置；通用活动可使用 `default_activity_reward_config` | `checkin_reward_config_ref` |
| `keyTemplate` | 奖励配置 key。默认按活动读取 | `${activityId}` |
| `namespace` | 奖励来源命名空间 | `reward` |
| `valuePath` | 外部资源返回候选数组时的路径。Redis 推荐存 `{ "reward_items": [...] }` | `$.reward_items` |
| `outputAs` | 资源操作输出别名 | `rewardConfig` |

说明：候选奖励来源只读取配置或候选数组，底层操作固定按 `READ` 推断，UI 不再展示 `operation` 或 `miss`。

#### 输出

成功时输出：

| 输出 | 类型 | 说明 |
| --- | --- | --- |
| `candidate_rewards` | array | 标准候选奖励数组，供 `budget-reserve` 使用 |
| `reward_items` | array | 与 `candidate_rewards` 同值，供后续 `grant-order` 使用 |
| `candidateCount` | number | 候选奖励数量 |
| `candidatePolicy` | string | 本次使用的候选选择策略 |
| `source` | string | 实际候选来源 |

候选奖励 item 的常见字段：

| 字段 | 说明 |
| --- | --- |
| `rewardId` | 活动奖励配置 ID |
| `rewardCode` | 奖励编码，例如 `DAILY_CHECKIN_BONUS` |
| `rewardType` | 奖品类型，例如 `CASH`、`POINT`、`COUPON`、`GIFT`、`DRAW_CHANCE`、`MEMBERSHIP` |
| `calculateType` | 产生方式，例如 `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` |
| `amount` | 预计算金额。来源于 reward calculator |
| `currency` | 币种 |
| `points` | 积分数量，适用于 `POINT` |
| `quantity` | 券、实物、权益份数，适用于 `COUPON` / `GIFT` |
| `chanceCount` | 活动参与次数或抽奖机会，适用于 `DRAW_CHANCE` |
| `grantMode` | 发放模式 |
| `grantChannel` | 发放渠道 |
| `candidate` | 固定为 `true`，表示这还是候选，不是已发奖励 |
| `payload` | 计算器输出的扩展数据 |
| `reasonCode` / `reason` | 计算原因或跳过原因 |
| `skipped` | 计算器是否标记跳过 |

#### 奖品模块

`candidate-reward` 可以视为“奖品模块”的入口。业界常见活动奖品建议按三层定义：

| 层 | 字段 | 说明 |
| --- | --- | --- |
| 奖品是什么 | `rewardType` | 现金、积分、优惠券、实物、抽奖机会、会员权益等 |
| 怎么产生 | `calculateType` | 固定、按比例、阶梯、随机金额、概率奖池、奖池瓜分 |
| 怎么发放 | `grantMode` / `grantChannel` | 自动发放、人工发放、导出；HTTP/MQ/Webhook/内部渠道 |

推荐支持的奖品类型：

| rewardType | 中文含义 | 常见字段 | 库存/预算 |
| --- | --- | --- | --- |
| `CASH` | 现金、USDT、红包金额 | `amount`, `currency` | 通常锁 `BUDGET` |
| `BONUS` | 体验金、彩金、平台奖励金 | `amount`, `currency`, `flowMultiple` | 通常锁 `BUDGET` |
| `POINT` | 积分、成长值、任务分 | `points`, `unit` / `pointType` | 可不锁预算，或锁积分预算 |
| `COUPON` | 优惠券、抵扣券、折扣券 | `quantity`, `couponTemplateId`, `faceValue`, `discountRate` | 通常锁 `STOCK` |
| `GIFT` | 实物、周边、礼包 | `quantity`, `giftSkuId`, `deliveryMode`, `valuationAmount` | 通常锁 `STOCK` |
| `DRAW_CHANCE` | 参加活动次数、抽奖机会、转盘次数 | `chanceCount`, `drawPoolCode` / `lotteryCode` | 可锁次数额度或活动库存 |
| `MEMBERSHIP` | VIP、会员、权益包 | `durationDays` / `durationType`, `membershipTier`, `benefitPackageId` | 可锁权益库存 |

推荐支持的产生方式：

| calculateType | 中文含义 | 例子 |
| --- | --- | --- |
| `FIXED` | 固定奖品 | 每天签到固定 `1 USDT`、`100` 积分、`1` 张券 |
| `RATIO` | 按比例 | 充值金额的 `1%` 返现，最多 `10 USDT` |
| `LADDER` | 阶梯 | 连续签到 3/7/30 天给不同金额 |
| `RANDOM` | 随机金额 | 红包金额在 `0.1` 到 `5 USDT` 中按步长随机 |
| `PROBABILITY` | 概率奖池 | 90% 积分、9% 券、1% 实物大奖 |
| `POOL_SPLIT` | 奖池瓜分 | 活动结束后按人数或权重瓜分总奖池 |

示例：一个签到奖品配置 Ref 可以这样存：

```json
{
  "reward_items": [
    {
      "rewardCode": "DAILY_CHECKIN_1_USDT",
      "rewardType": "CASH",
      "calculateType": "FIXED",
      "amount": "1",
      "currency": "USDT",
      "grantMode": "AUTO",
      "grantChannel": "WALLET"
    },
    {
      "rewardCode": "CHECKIN_100_POINTS",
      "rewardType": "POINT",
      "calculateType": "FIXED",
      "points": "100",
      "unit": "LOYALTY_POINT",
      "grantMode": "AUTO",
      "grantChannel": "INTERNAL"
    },
    {
      "rewardCode": "CHECKIN_RANDOM_USDT",
      "rewardType": "CASH",
      "calculateType": "RANDOM",
      "minAmount": "0.1",
      "maxAmount": "5",
      "step": "0.1",
      "currency": "USDT",
      "grantMode": "AUTO",
      "grantChannel": "WALLET"
    },
    {
      "rewardCode": "CHECKIN_DRAW_CHANCE",
      "rewardType": "DRAW_CHANCE",
      "calculateType": "FIXED",
      "chanceCount": "1",
      "drawPoolCode": "daily_checkin_lottery",
      "grantMode": "AUTO",
      "grantChannel": "INTERNAL"
    },
    {
      "rewardCode": "CHECKIN_MIXED_POOL",
      "rewardType": "DRAW_CHANCE",
      "calculateType": "PROBABILITY",
      "grantMode": "AUTO",
      "grantChannel": "INTERNAL",
      "items": [
        {
          "itemCode": "POINT_100",
          "weight": "70",
          "rewardType": "POINT",
          "points": "100",
          "unit": "LOYALTY_POINT"
        },
        {
          "itemCode": "COUPON_5",
          "weight": "25",
          "rewardType": "COUPON",
          "quantity": "1",
          "couponTemplateId": "tpl_checkin_5",
          "faceValue": "5",
          "currency": "USDT"
        },
        {
          "itemCode": "GIFT_TSHIRT",
          "weight": "5",
          "rewardType": "GIFT",
          "quantity": "1",
          "giftSkuId": "sku_tshirt",
          "deliveryMode": "ADDRESS_REQUIRED",
          "valuationAmount": "20",
          "currency": "USDT"
        }
      ]
    }
  ]
}
```

这里 `CHECKIN_MIXED_POOL` 是混合概率奖池：外层 `rewardType` 只是奖池默认类型，真正抽中的奖品类型由 item 的 `rewardType` 决定。`candidate-reward` 只生成候选，`budget-reserve` 再根据奖品类型决定锁预算还是锁库存。例如现金锁 `checkin_reward_budget_ref`，券/实物/权益锁 `checkin_prize_stock_ref`。

#### 候选选择策略

`candidatePolicy` 控制资源返回多条奖励时如何输出：

| 策略 | 说明 | 适用场景 |
| --- | --- | --- |
| `ALL_MATCHED` | 输出全部候选奖励 | 每日基础奖励 + 连续签到加奖 + VIP 加奖可以叠加 |
| `FIRST_MATCHED` | 只输出第一条候选奖励 | 普通/VIP 二选一，配置顺序即优先级 |
| `HIGHEST_VALUE` | 只输出金额最高的一条 | 多档奖励只给最高档 |
| `WEIGHTED_RANDOM` | 按 item 的 `weight` 稳定随机选一条 | 抽奖、随机券包 |
| `RANDOM_ONE` | 等概率稳定随机选一条 | 多个固定奖品随机给一个 |

每日签到默认推荐 `ALL_MATCHED`，这样基础签到奖励和额外奖励可以同时进入后续预算锁定。

#### 资格未通过和空候选

当 `runtime.Qualified=false`：

- `notQualifiedAction=SKIP`：节点跳过，输出空 `candidate_rewards` 和 `reward_items`。
- `notQualifiedAction=BLOCK`：节点失败，错误码为 `CANDIDATE_REWARD_NOT_QUALIFIED`。

当候选为空：

- `emptyAction=BLOCK`：节点失败，错误码为 `CANDIDATE_REWARD_EMPTY`。
- `emptyAction=SKIP`：节点跳过，输出空数组。

生产奖励链路通常推荐：

```json
{
  "notQualifiedAction": "SKIP",
  "emptyAction": "BLOCK"
}
```

含义是：资格没通过就不生成候选；资格通过但没有任何奖励配置是配置错误，应阻断并暴露问题。

#### 示例：每日签到固定奖励

生产签到模板里的候选奖励节点通常这样配置：

```json
{
  "type": "candidate-reward",
  "title": "签到候选奖励",
  "config": {
    "providerChain": [
      {
        "source": "RESOURCE",
        "resourceRef": "checkin_reward_config_ref",
	        "keyTemplate": "${activityId}",
	        "namespace": "reward",
	        "valuePath": "$.reward_items",
	        "outputAs": "rewardConfig"
      }
    ],
    "candidatePolicy": "ALL_MATCHED",
    "notQualifiedAction": "SKIP",
    "emptyAction": "BLOCK",
    "includeTrace": true,
    "inputMapping": {
      "qualified_context": {
        "sourceNodeId": "qualify",
        "outputKey": "PASS"
      }
    }
  }
}
```

`checkin_reward_config_ref` 是 Redis KV 语义资源。Redis 中建议按活动存对象，key 由资源前缀和节点 key 组合：

```text
reward:checkin:${activityId}
```

值示例：

```json
{
  "reward_items": [
    {
      "rewardCode": "DAILY_CHECKIN_1_USDT",
      "rewardType": "CASH",
      "calculateType": "FIXED",
      "amount": "1",
      "currency": "USDT",
      "grantMode": "AUTO",
      "grantChannel": "WALLET"
    }
  ]
}
```

假设活动奖励配置里有一条每日签到奖励：

```json
{
  "id": "reward_daily_checkin",
  "code": "DAILY_CHECKIN_BONUS",
  "type": "BONUS",
  "calculateType": "FIXED",
  "grantMode": "AUTO",
  "grantChannel": "HTTP",
  "enabled": true,
  "config": {
    "amount": 1,
    "currency": "USD"
  }
}
```

节点成功后会输出类似：

```json
{
  "candidate_rewards": [
    {
      "rewardId": "reward_daily_checkin",
      "rewardCode": "DAILY_CHECKIN_BONUS",
      "rewardType": "BONUS",
      "calculateType": "FIXED",
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "candidate": true,
      "amount": "1.0000",
      "currency": "USD"
    }
  ],
  "reward_items": [
    {
      "rewardCode": "DAILY_CHECKIN_BONUS",
      "amount": "1.0000",
      "currency": "USD",
      "candidate": true
    }
  ],
  "candidateCount": 1,
  "source": "RESOURCE"
}
```

下游连接方式：

```text
candidate-reward.candidate_rewards -> budget-reserve.candidate_rewards
candidate-reward.reward_items      -> grant-order.reward_items
```

#### 示例：插件输出候选奖励

当奖励由插件决定，例如抽奖插件、红包插件、任务插件，也建议通过 RESOURCE 操作块接入插件写出的候选奖励资源。RESOURCE 写法如下：

```json
{
  "type": "candidate-reward",
  "title": "插件候选奖励",
  "config": {
    "providerChain": [
      {
        "source": "RESOURCE",
        "resourceRef": "lottery_candidate_reward_store",
	        "keyTemplate": "${activityId}:${subject.id}:${eventId}",
	        "namespace": "lottery",
	        "valuePath": "$.reward_items",
	        "outputAs": "pluginRewardCandidates"
      }
    ],
    "candidatePolicy": "ALL_MATCHED",
    "notQualifiedAction": "SKIP",
    "emptyAction": "SKIP",
    "inputMapping": {
      "plugin_result": {
        "sourceNodeId": "plugin-execute",
        "outputKey": "plugin_result"
      }
    }
  }
}
```

上游插件输出可以是：

```json
{
  "reward_items": [
    {
      "rewardCode": "LOTTERY_SMALL_PRIZE",
      "rewardType": "BONUS",
      "amount": "0.5000",
      "currency": "USD",
      "reason": "lottery hit"
    }
  ]
}
```

`candidate-reward` 会把资源返回的数组标准化为 `candidate_rewards/reward_items` 输出，后续仍然由 `budget-reserve` 锁预算或库存。

#### 推荐实践

1. `candidate-reward` 应只放在 `qualify.PASS` 之后，不要让未资格通过用户进入预算锁定。
2. 签到固定奖励、阶梯奖励、比例奖励优先用 `checkin_reward_config_ref`，不要把金额硬编码在节点里。
3. 抽奖、红包、插件玩法可以用插件写出的候选奖励资源；预算库存仍应由 `budget-reserve` 统一锁定。
4. 资格通过但候选为空，生产环境建议 `emptyAction=BLOCK`，这样能尽早暴露活动奖励配置缺失。
5. 候选奖励只是计划，不是发奖事实。真正发奖事实以 `grant-order/outbox/callback` 为准。

### `budget-reserve` 预算库存锁定

`budget-reserve` 只做领奖前的预算/库存锁定。它读取 `candidate-reward` 输出的候选奖励数量，然后到预算或库存资源池里做原子锁定。

它不负责用户资格、每日签到频控、每用户领取次数。这些应分别放在 `qualify` 和 `quota-check`：

| 逻辑 | 推荐节点 |
| --- | --- |
| 用户等级、KYC、风控、活动状态 | `qualify` |
| 每日签到一次、每用户最多 N 次、周期窗口频控 | `quota-check` |
| 现金预算、券/实物/权益库存并发占用 | `budget-reserve` |

节点只需要选择 `reserveProviderRef`。`quotaCode`、`quotaType`、`scope`、`scopeKeyTemplate`、默认 `amountField` 都由 Ref 内部配置。这样用户不用关心内部 bucket 定位，只需要知道这次调用的是哪一个预算/库存锁定服务。

和 `candidate-reward` 的结合方式：

1. `candidate-reward` 先输出 `candidate_rewards`，里面包含 `rewardCode`、`rewardType`、`amount`、`quantity`、`currency` 等候选奖励参数。
2. `budget-reserve` 通过输入映射接收 `candidate_rewards`。
3. `reserveProviderRef` 指向锁定服务。锁定服务内部配置决定锁活动预算、奖品库存，或调用外部 HTTP/gRPC 服务。
4. 锁定服务按 `amountField` 和 `scopeKeyTemplate` 从 `candidate_rewards` 取值，例如用 `candidate_rewards[0].amount` 锁现金预算，用 `candidate_rewards[0].quantity` 锁奖品库存。

内置 Ref：

| reserveProviderRef | 用途 | 默认锁定类型 | 默认 scopeKey |
| --- | --- | --- | --- |
| `checkin_reward_budget_ref` | 每日签到现金/USDT 奖励预算 | `BUDGET` | `${activityId}` |
| `checkin_prize_stock_ref` | 每日签到券、实物、权益等奖品库存 | `STOCK` | `${activityId}:${candidate_rewards[0].rewardCode}` |

#### 锁定类型

| quotaType | 中文含义 | 常见 amountField |
| --- | --- | --- |
| `BUDGET` | 活动预算，适合现金、USDT、Bonus 金额 | `candidate_rewards[0].amount` |
| `STOCK` | 奖品库存，适合优惠券、实物、抽奖次数、权益名额 | `candidate_rewards[0].quantity` |

`USER_LIMIT` 和 `DAILY_LIMIT` 不属于 `budget-reserve` 的职责，应该在 `quota-check` 中配置。

#### 发钱：锁活动预算

```json
{
  "reserveProviderRef": "checkin_reward_budget_ref",
  "failAction": "BLOCK",
  "releaseOnFailure": true
}
```

Ref 内部配置：

```json
{
  "quotaCode": "DAILY_CHECKIN_REWARD",
  "quotaType": "BUDGET",
  "scope": "ACTIVITY",
  "scopeKeyTemplate": "${activityId}",
  "amountField": "candidate_rewards[0].amount",
  "reserveMode": "ATOMIC"
}
```

#### 发券或奖品：锁奖品库存

```json
{
  "reserveProviderRef": "checkin_prize_stock_ref",
  "failAction": "BLOCK",
  "releaseOnFailure": true
}
```

Ref 内部配置：

```json
{
  "quotaCode": "CHECKIN_COUPON_STOCK",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKeyTemplate": "${activityId}:${candidate_rewards[0].rewardCode}",
  "amountField": "candidate_rewards[0].quantity",
  "reserveMode": "ATOMIC"
}
```

这里的 `STOCK` 就是奖品库存。系统会按 `quotaType + scope + scopeKey` 找到对应库存池，检查 `total - used - frozen` 是否足够，足够就增加 frozen，不足就按 `failAction` 处理。`quotaCode` 用于业务识别和审计。

#### 外部预算/库存 Service Interface

预算/库存的外部实现应建模为 Service Interface，例如 `quota-service.reserve-quota`，再由 Service Provider 选择 HTTP/RPC/Plugin 实现。旧字段 `reserveProviderRef` / `resourceRef` 仍作为兼容别名支持，但新流程统一使用 `serviceRef/interfaceRef` 或受控 Service Provider：

```json
{
  "serviceRef": "quota-service",
  "interfaceRef": "reserve-quota",
  "providerPolicy": "PRIMARY",
  "contract": "QUOTA_RESERVE_V1",
  "quotaCode": "CHECKIN_COUPON_STOCK",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKeyTemplate": "${activityId}:${candidate_rewards[0].rewardCode}",
  "amountField": "candidate_rewards[0].quantity"
}
```

Service Provider 会收到统一请求体。底层 HTTP/RPC/MQ 连接和凭据由 Provider 依赖的 Resource 管理，节点不直接配置 endpoint、grpcTarget、broker 或 secret。核心字段如下：

```json
{
  "operation": "RESERVE",
  "tenantId": "site_001",
  "activityId": "act_001",
  "eventId": "evt_001",
  "idempotencyKey": "site_001:evt_001",
  "quotaCode": "CHECKIN_COUPON_STOCK",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKey": "act_001:COUPON_5",
  "amount": "1",
  "candidateRewards": [
    {
      "rewardCode": "COUPON_5",
      "rewardType": "COUPON",
      "quantity": 1
    }
  ],
  "rewardItems": []
}
```

外部服务建议返回：

```json
{
  "allowed": true,
  "reservationId": "resv_001",
  "status": "RESERVED",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKey": "act_001:COUPON_5",
  "amount": "1"
}
```

如果 `allowed=false`，`budget-reserve` 会按锁定失败处理。外部服务必须自己保证幂等和并发安全。

RPC Provider 也使用同一个 `QUOTA_RESERVE_V1` 契约。底层 gRPC client/channel 作为 Resource 管理，Provider 只引用受控 Resource：

```json
{
  "serviceRef": "quota-service",
  "interfaceRef": "reserve-quota",
  "provider": {
    "protocol": "RPC",
    "resourceRefs": ["budget-reserve-grpc-client"],
    "method": "Reserve"
  },
  "contract": "QUOTA_RESERVE_V1",
  "quotaCode": "CHECKIN_USDT_BUDGET",
  "quotaType": "BUDGET",
  "scope": "ACTIVITY",
  "scopeKeyTemplate": "${activityId}",
  "amountField": "candidate_rewards[0].amount"
}
```

兼容执行器可以继续识别旧 gRPC reserve Ref，并要求接入具体 gRPC adapter/client；没有 adapter 时不会伪造成功锁定。新模型应通过 Service Interface + Provider + Resource 组合表达。

#### 并发安全要求

`budget-reserve` 的核心不是普通读取，而是原子 reservation：

1. 不允许先读库存再写库存。
2. Postgres quota 使用单条条件更新：只有 `used + frozen + amount <= total` 时才增加 `frozen`。
3. Redis 类型 Ref 后续必须使用 Lua/CAS 脚本完成同样的 check-and-freeze。
4. 外部 HTTP/gRPC Service Provider 必须提供带幂等键的 reserve 接口，失败重试不能重复锁定。
5. 发奖成功后确认占用，发奖失败或超时后释放 frozen。

因此自定义扩展时，只要新的 Ref 遵守 `QUOTA_RESERVE_V1` 契约，就可以替换底层存储，但不能绕过原子锁定语义。

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
| `mq-publish` | MQ 发奖 | 描述 MQ 投递意图；配置 `MQ resourceRef` 时生成受控发布意图并记录资源审计 | 部分资源化；真实 broker ack 仍由 OutboxWorker/MQ sender 负责 | `mq_result`, `MQ` |
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

1. `基础节点` 是默认活动编排的第一层节点，普通奖励、签到、任务、通知链路应优先从这里选择。
2. `高级节点` 用于增强基础链路，例如事实标准化、用户分层、风控、插件、审批和对账，不应让普通模板默认强依赖。
3. `组合节点` 只负责数据整形、脚本处理和条件路由，不应伪装成业务状态、资格、频控或发奖节点。
4. `其他节点` 主要承载事件适配、底层查询、投递边界和运维辅助；只有需要明确底层能力时才直接暴露给活动配置用户。
5. `activity-state` 应保留并暴露为所有活动状态获取的基础节点；底层 `state-store` 作为通用存储能力放在其他节点。
6. `http-query`、`redis-query`、`grpc-query`、`plugin-query` 作为底层接入能力；新链路不直接填写 endpoint、redisRef、grpcTarget、pluginCode。业务能力通过 `SERVICE_INTERFACE` 接入，底层状态/连接通过 `RESOURCE` Ref 接入。
7. 生产奖励链路统一使用 `activity-state`、`quota-check`、`candidate-reward`、`budget-reserve`。
8. 解释型节点必须明确 `executionMode` 和 `executionAuthority`，不能让用户误以为 workflow 节点本身完成了外部投递、MQ 消费、回调处理或告警发送。

## 资源化能力状态

| 能力 | 当前状态 | 边界 |
| --- | --- | --- |
| `TABLE` | `state-store + resourceRef` 已支持 | 底层为受控虚拟公共表，不直接开放任意 SQL 表写入 |
| `HTTP` | 作为受控 HTTP Resource 和 Service Provider 协议已支持第一版 | 不直接作为 fact-query 节点来源暴露；业务能力由 Service Interface 引用，底层连接由 Resource Ref 承载 |
| `REDIS` | `state-store + resourceRef` 已支持第一版 | 单一 Redis 类型，通过 `mode=KV/HASH/COUNTER/LOCK` 表达不同语义；当前由 `marketing_storage_state` 承载，还不是真 Redis client |
| `MQ` | `mq-publish/outbox + resourceRef` 已支持第一版 | 节点生成 publish intent 或 dispatch plan，真实 broker ack 仍由 OutboxWorker/MQ sender |
| Lua Resource API | `code/combine/split/lua-query` 的 service 执行路径已支持 | 只能访问通过资源权限和 binding 授权的资源，并受调用次数/响应大小限制 |

仍未完成：

- 真实 Redis client adapter。
- 生产 Kafka/RabbitMQ/Pulsar transport SDK 注入。
- 独立资源审批单。
- `mq-event` broker loop/offset/replay UI、`schedule-event` 统一调度注册表、`state-event` watcher cursor/debounce。
