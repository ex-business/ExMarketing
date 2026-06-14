# ExMarketing 事件输入节点设计草案

本文档用于重新定义 Workflow Studio 里的“事件输入”产品模型。目标是把“事件从哪里来”和“事件进入 workflow 前如何校验”拆清楚，避免把 HTTP、Webhook、MQ、定时、状态监听等能力都塞进一个 `event-in` 节点。

## 设计结论

`事件输入` 应该是一个节点分类，不应该只是一个节点。

当前 `event-in` 节点应从“所有事件入口”收口为“标准事件入口校验节点”。它只消费已经进入 ExMarketing 的标准事件，负责校验事件类型、来源系统、租户、载荷契约，然后输出 workflow 可继续使用的 `raw_event`。

推荐结构：

```text
事件输入节点 -> 标准事件入口校验 -> 幂等收件箱 -> 标准化 -> 活动匹配 -> 资格/奖励/额度/发奖
```

更完整的抽象：

```text
Trigger Source -> Event Adapter -> Standard Event -> Event In -> Inbox -> Workflow
```

## 核心概念

### Trigger Source

触发源，表示事件从哪里来。

示例：

- 外部业务系统调用 HTTP API。
- 第三方 Webhook 推送。
- Kafka / RabbitMQ / Pulsar topic 消息。
- Cron 定时任务。
- Redis / 配置中心 / 指标 / 全局状态变化。
- DB CDC / Change Stream。
- 文件导入。
- 管理台人工触发。
- ExMarketing 内部系统事件。

### Event Adapter

事件适配器，负责把不同来源的 payload 转成 ExMarketing 标准事件。

适配器应该处理：

- 来源鉴权。
- 原始 payload schema 校验。
- 字段映射。
- 默认字段补齐。
- 幂等键所需字段生成。
- 来源 trace / request / offset / cursor 记录。
- 安全脱敏。

### Standard Event

标准事件是所有 workflow 的唯一输入契约。不同入口最终都必须产出同一类标准事件：

```json
{
  "eventId": "evt_xxx",
  "eventType": "CHECKIN_SUCCESS",
  "sourceSystem": "app",
  "tenantId": "site_001",
  "subject": {
    "type": "USER",
    "id": "10001"
  },
  "biz": {
    "bizType": "CHECKIN",
    "bizId": "2026-06-14:user_10001"
  },
  "occurredAt": "2026-06-14T15:00:00+04:00",
  "attributes": {
    "channel": "APP",
    "streakDays": 7
  }
}
```

标准幂等键仍保持：

```text
tenantId:sourceSystem:eventType:biz.bizType:biz.bizId
```

### Event In

`event-in` 是标准事件进入 workflow 后的第一道校验节点。

它不应该负责监听 HTTP 端口、消费 MQ、跑 cron 或监听变量。它只负责：

- 校验 `eventType` 是否允许。
- 校验 `sourceSystem` 是否允许。
- 校验 `tenantScope`。
- 校验标准事件 schema 版本。
- 提取 trace key。
- 输出 `raw_event`、`eventId`、`eventType`、`tenantId`。

## 推荐节点分类

### 事件输入

| 节点类型 | 中文名 | 责任 | 当前可行性 |
| --- | --- | --- | --- |
| `standard-event` / `event-in` | 标准事件入口 | 接收已经标准化的事件，只做入口校验 | 已有基础，建议重命名和收口 |
| `http-event` | HTTP 接口事件 | 外部系统通过 OpenAPI 主动上报标准事件 | 已支持接口，UI 节点待建模 |
| `webhook-event` | Webhook 映射事件 | 接收第三方 payload，签名校验后映射为标准事件 | 已支持接口和 receiver config，UI 节点待建模 |
| `mq-event` | MQ Topic 事件 | 监听 Kafka/RabbitMQ/Pulsar topic，映射消息为标准事件 | 待实现入站 consumer |
| `schedule-event` | 定时事件 | 按 cron/延迟任务生成标准事件 | 部分能力可复用插件调度，节点模型待建 |
| `state-event` | 状态监听事件 | 监听 Redis/config/指标/全局状态条件，命中后生成事件 | 待实现 watcher |
| `cdc-event` | 数据变更事件 | 监听数据库变更并映射为标准事件 | 待实现 CDC adapter |
| `file-event` | 文件导入事件 | CSV/Excel/S3/OSS/SFTP 批量导入并生成事件 | 待实现导入 adapter |
| `manual-event` | 人工触发事件 | 管理台补偿、重放、手动执行 | 有 debug/replay 基础，生产补偿入口待收口 |
| `internal-event` | 内部系统事件 | ExMarketing 内部状态变化触发 workflow | 部分能力存在，节点模型待建 |

### 幂等与标准化

| 节点类型 | 中文名 | 责任 |
| --- | --- | --- |
| `inbox` | 幂等收件箱 | 写入事件收件箱，处理重复事件和 payload 冲突 |
| `normalize` | 事件标准化 | 对标准事件或业务属性做二次字段标准化 |

### 活动决策

| 节点类型 | 中文名 | 责任 |
| --- | --- | --- |
| `match` | 活动匹配 | 根据租户、事件类型、活动状态和入口配置匹配活动 |
| `fact-query` | 事实查询 | 查询外部事实数据 |
| `qualify` | 资格校验 | 执行资格、风控、规则判断 |
| `condition` | 条件判断 | 通用条件判断 |
| `switch` | 条件分支 | 多分支路由 |

### 奖励与额度

| 节点类型 | 中文名 | 责任 |
| --- | --- | --- |
| `reward` | 奖励计算 | 计算奖励明细 |
| `limit` | 次数限制 | 用户/活动级次数限制 |
| `quota` | 预算库存扣减 | 预算、库存、用户额度预占和扣减 |

### 执行与投递

| 节点类型 | 中文名 | 责任 |
| --- | --- | --- |
| `participation` | 参与记录 | 写入参与记录 |
| `grant-order` | 发奖订单 | 创建发奖命令和状态机记录 |
| `outbox` | 消息发件箱 | 生成可重试投递消息 |
| `http-call` | HTTP 发奖 | 解释或承接 HTTP 投递能力 |
| `mq-publish` | MQ 发奖 | 解释或承接 MQ 投递能力 |
| `webhook` | Webhook 通知 | 解释或承接 Webhook 通知能力 |

### 运维治理

| 节点类型 | 中文名 | 责任 |
| --- | --- | --- |
| `audit` | 审计记录 | 写审计和运行解释 |
| `retry` | 失败重试 | 重试策略解释和重放入口 |
| `dlq` | 死信队列 | 死信解释、关闭、重放 |
| `reconcile` | 对账校验 | 对账查询和解释 |
| `alert` | 告警通知 | 告警解释或外部告警联动 |

## 事件输入节点契约

所有事件输入节点都应输出统一结构：

```json
{
  "standard_event": {},
  "source": {
    "type": "HTTP",
    "name": "openapi-events",
    "traceId": "trace_xxx",
    "requestId": "req_xxx",
    "receivedAt": "2026-06-14T15:00:00Z"
  },
  "adapter": {
    "version": "standard-v1",
    "mappingVersion": "v1"
  }
}
```

其中 `standard_event` 必须能通过 `ActivityEvent.Validate()`，并能生成稳定 `Idempotency-Key`。

不同输入节点可以有不同的配置，但输出必须一致。

## 各入口节点建议配置

### HTTP 接口事件

```json
{
  "path": "/openapi/v1/events",
  "method": "POST",
  "auth": {
    "type": "BEARER_TOKEN",
    "tenantScope": "TOKEN_BOUND"
  },
  "contentType": "application/json",
  "maxBodyBytes": 1048576,
  "payloadMode": "STANDARD_EVENT"
}
```

适合：签到、支付成功、注册成功、订单完成等业务系统主动上报。

### Webhook 映射事件

```json
{
  "receiverCode": "payment-webhook",
  "signature": {
    "type": "HMAC_SHA256",
    "timestampHeader": "X-ExMarketing-Timestamp",
    "signatureHeader": "X-ExMarketing-Signature"
  },
  "allowedTenantIds": ["site_001"],
  "allowedSourceSystems": ["payment"],
  "mapping": {
    "eventId": {"path": "$.id"},
    "eventType": {"const": "DEPOSIT_SUCCESS"},
    "sourceSystem": {"const": "payment"},
    "tenantId": {"const": "site_001"}
  }
}
```

适合：第三方支付、CRM、外部运营平台回调。

### MQ Topic 事件

```json
{
  "broker": "KAFKA",
  "topic": "user-checkin-events",
  "consumerGroup": "exmarketing-checkin",
  "offsetPolicy": "COMMITTED",
  "ackPolicy": "ACK_AFTER_INBOX_COMMIT",
  "retryPolicy": "RETRY_THEN_DLQ",
  "mapping": {
    "eventId": {"path": "$.event_id"},
    "eventType": {"const": "CHECKIN_SUCCESS"}
  }
}
```

关键约束：

- 只有 Event Inbox 成功提交后才能 ack。
- 幂等冲突不能无限重试。
- consumer offset、message key、partition、offset 必须进入 source metadata。

### 定时事件

```json
{
  "scheduleType": "CRON",
  "cron": "0 0 * * *",
  "timezone": "Asia/Shanghai",
  "misfirePolicy": "FIRE_ONCE",
  "jitterSeconds": 60,
  "eventTemplate": {
    "eventType": "DAILY_CHECKIN_SETTLEMENT",
    "sourceSystem": "exmarketing-scheduler"
  }
}
```

关键约束：

- 同一个计划时间必须生成稳定 `bizId`。
- 重试不能改变 payload，否则会触发幂等冲突。
- 需要记录 schedule job id、scheduledAt、attempt。

### 状态监听事件

```json
{
  "source": "REDIS",
  "key": "activity:checkin:global",
  "watchMode": "POLLING",
  "intervalSeconds": 10,
  "condition": "value.enabled == true && value.stockLow == true",
  "debounceSeconds": 60,
  "eventTemplate": {
    "eventType": "STATE_THRESHOLD_REACHED",
    "sourceSystem": "state-watcher"
  }
}
```

关键约束：

- 不建议监听不可恢复的进程内全局变量。
- 推荐监听 Redis、数据库、配置中心、指标系统等可恢复状态。
- 必须有 last-seen cursor 或 snapshot version，防止重复触发或漏触发。

### 人工触发事件

```json
{
  "allowedRoles": ["OPS", "ADMIN"],
  "requireReason": true,
  "payloadMode": "STANDARD_EVENT",
  "auditAction": "MANUAL_EVENT_TRIGGERED"
}
```

关键约束：

- 必须有操作人、原因、审批或权限。
- 需要和 debug-run 区分：debug 无副作用，manual production trigger 有副作用。

## 当前系统适配建议

### 短期调整

短期不要大改执行引擎。先做产品模型和 UI 收口：

1. 左侧节点库新增分类 `事件输入`。
2. 把当前 `event-in` 从 `CORE 核心链路` 移到 `事件输入`。
3. 将当前 `事件入口` 改名为 `标准事件入口`。
4. 当前描述从 `HTTP / MQ / Webhook 标准事件接入` 改为：

```text
接收已标准化事件，校验 eventType、sourceSystem、tenant 后进入流程。
```

5. 预留入口节点：
   - `HTTP 接口事件`：已支持接口，节点先作为解释/配置节点。
   - `Webhook 映射事件`：已支持接口，节点先作为解释/配置节点。
   - `MQ Topic 事件`：待实现。
   - `定时事件`：待实现或接入已有调度能力。
   - `状态监听事件`：待实现。
   - `人工触发事件`：待从 debug/replay 收口为生产补偿入口。

### 中期调整

1. 后端 `WorkflowNodeConfigSchema` 增加 `EVENT_INPUT` 分类。
2. Node preset 增加上述事件输入节点。
3. Validator 支持“事件输入分类至少一个入口节点”。
4. `event-in` handler 补齐或移除当前 schema 中未生效字段：
   - `tenantScope`
   - `payloadSchemaVersion`
   - `rejectUnknownEventType`
   - `traceKeyPath`
5. Run history 中区分：
   - trigger adapter 运行记录。
   - workflow node 运行记录。

### 长期调整

1. 实现 MQ 入站 consumer。
2. 实现 schedule trigger runtime。
3. 实现 state watcher runtime。
4. 实现 CDC adapter。
5. 实现文件导入批处理入口。
6. 形成统一 Trigger Registry：

```text
trigger_id
trigger_type
activity_id / workflow_id
enabled
config
cursor / offset / last_seen
created_at
updated_at
```

## 可行性评估

### 已具备能力

- 标准事件模型已经存在。
- HTTP 标准事件入口已经存在。
- Webhook 映射入口已经存在。
- Event Inbox 幂等模型已经存在。
- Workflow draft / publish / validate / run history 基础已经存在。
- 插件调度相关能力可为 `schedule-event` 提供参考。

### 低风险改造

- UI 分类调整。
- `event-in` 重命名和文案修正。
- 新增解释型入口节点 schema。
- 运行历史展示来源 metadata。
- 文档和模板调整。

这些改造主要影响产品表达和 schema，不需要先改变生产事件处理链路。

### 中风险改造

- Validator 支持多入口和入口分类。
- `event-in` handler 补齐 `tenantScope`、`payloadSchemaVersion`、`traceKeyPath`。
- HTTP/Webhook 入口配置和 workflow 节点配置建立关联。
- Manual trigger 从 debug 语义升级为生产补偿语义。

这些改造会影响发布校验和运行解释，需要配套测试。

### 高风险改造

- MQ 入站 consumer。
- State watcher。
- CDC adapter。
- 文件批量导入。

这些能力涉及消费游标、ack、重试、DLQ、幂等冲突、并发、恢复和监控，不建议和 UI 分类调整放在同一阶段完成。

## 推荐实施顺序

第一阶段：产品模型收口。

- 新增 `事件输入` 分类。
- `事件入口` 改为 `标准事件入口`。
- 修正文案和配置预览。
- 预留 HTTP/Webhook/MQ/Schedule/State/Manual 输入节点。
- 暂不改变生产执行链路。

第二阶段：标准事件入口语义补齐。

- `event-in` handler 与 schema 完全对齐。
- 明确 `tenantScope`、`payloadSchemaVersion`、`traceKeyPath` 的运行时行为。
- 未实现字段从 UI 暂时隐藏。

第三阶段：HTTP/Webhook 节点配置化。

- 将现有 OpenAPI/Webhook 配置映射到 Studio 可视化配置。
- 节点仍输出标准事件，不绕过 Event Inbox。

第四阶段：MQ 入站。

- 实现 MQ trigger registry。
- consumer ack 必须在 inbox commit 后。
- 支持 retry / DLQ / replay。

第五阶段：Schedule / State / CDC / File。

- 按业务优先级逐个实现。
- 每个入口都必须遵守标准事件、幂等、审计、运行历史契约。

## 对签到奖励流程的落地方式

签到奖励第一期推荐使用 HTTP 标准事件入口：

```text
HTTP 接口事件 / 标准事件入口 -> Inbox -> Normalize -> Match -> Qualify -> Reward -> Limit -> Quota -> Participation -> Grant Order -> Outbox -> Audit
```

业务 App 或 User Center 在用户签到成功后上报：

```json
{
  "eventType": "CHECKIN_SUCCESS",
  "sourceSystem": "app",
  "biz": {
    "bizType": "CHECKIN",
    "bizId": "2026-06-14:user_10001"
  }
}
```

如果未来签到事件已经在 Kafka 中，则切换为：

```text
MQ Topic 事件 -> 标准事件入口 -> Inbox -> 后续奖励链路
```

后续奖励节点不需要改，只替换事件输入层。

