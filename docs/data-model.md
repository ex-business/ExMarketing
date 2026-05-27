# 数据模型

## 总览

核心关系：

```text
marketing_activity
  ├─ marketing_rule
  ├─ marketing_reward
  └─ marketing_quota

marketing_event_inbox
  └─ marketing_participation
       └─ marketing_grant_order
            └─ marketing_outbox

marketing_audit
marketing_fact_provider
```

## marketing_activity

活动主表。

| 字段 | 说明 |
|---|---|
| `id` | 活动 ID，格式示例 `act_xxx` |
| `tenant_id` | 租户 ID |
| `activity_code` | 活动编码 |
| `activity_name` | 活动名称 |
| `activity_type` | 活动类型，如 `FIRST_DEPOSIT` |
| `status` | 活动状态 |
| `trigger_events` | 触发事件类型 JSON 数组 |
| `start_time` | 开始时间 |
| `end_time` | 结束时间 |
| `timezone` | 活动时区 |
| `priority` | 优先级，越大越先匹配 |
| `plugin_code` | 可选 ActivityPlugin 编码；未配置时走普通配置链路 |
| `exclusive_group` | 互斥组 |
| `stackable` | 是否允许叠加命中 |
| `version` | 配置版本 |
| `config_json` | 扩展配置 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

约束：`tenant_id + activity_code` 在 `marketing_activity_tenant_code_key` 下唯一，同一租户不能重复创建相同活动编码，不同租户可复用编码。

活动状态：

```text
DRAFT
SCHEDULED
ACTIVE
PAUSED
ENDED
```

审批状态保存在活动视图的 `approval` 快照中，并通过 `marketing_audit` 形成完整审批记录。状态包括：

```text
NONE
PENDING_L1
PENDING_L2
APPROVED
REJECTED
WITHDRAWN
```

`APPROVED` 只对锁定的 `activity.version` 有效。未审批、审批未完成、版本不一致或被驳回的活动不能进入 `SCHEDULED` / `ACTIVE`；正式事件处理仍只匹配 `ACTIVE` 活动，`SCHEDULED` 不会生成 participation、grant_order 或 outbox。

## marketing_rule

活动规则表。

| 字段 | 说明 |
|---|---|
| `id` | 规则 ID |
| `activity_id` | 活动 ID |
| `rule_code` | 规则编码 |
| `rule_phase` | 规则阶段 |
| `rule_type` | 规则类型 |
| `rule_config_json` | 规则配置 JSON |
| `sort` | 执行顺序 |
| `enabled` | 是否启用 |
| `version` | 版本 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

规则阶段：

```text
MATCH
QUALIFY
REWARD
RISK
LIMIT
```

规则运行时按 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET` 四个业务阶段输出解释结果。`RISK_LIMIT_BUDGET` 会合并执行 `RISK` 和 `LIMIT` 两类存储阶段；`BUDGET` 保持为 quota 类型，不作为合法 `rule_phase`。

规则类型：

```text
CONDITION
FORMULA
LIMIT
SCRIPT
```

当前已实现：

```text
CONDITION
FORMULA
```

当前 `CONDITION` / `FORMULA` 字段路径：

```text
eventId
eventType
sourceSystem
tenantId
subject.type
subject.id
biz.bizType
biz.bizId
occurredAt
attributes.<path>
facts.<factCode>.<path>
state.activity.<field>
state.subject.<field>
state.quota.<quotaType>.<field>
state.exclusive.<field>
state.plugin.<pluginCode>.<path>
```

后台创建活动和上线前校验会严格校验 `rule_config_json`：未知字段、空 `conditions`、非法 `rulePhase`、非法 `op`、缺失或非法 `value` 类型、非标准顶层业务字段，以及 `enabled=false` 的非法规则都会被拒绝。`FORMULA` 已注册为可运行 evaluator，最小能力支持 Decimal 算术 `+ - * /`、括号、一元负号、受控字段路径、Decimal 比较，以及 boolean 字段/字面量缺省 `compare`。`FORMULA` 不支持函数、脚本、SQL、HTTP、文件、环境变量、随机数或当前时间；不安全表达式会在创建、上线前校验或运行时结构化拒绝。`LIMIT`、`SCRIPT` 目前只保留 Schema 边界；在 evaluator / 安全能力未注册前，合法结构也不得保存为可运行规则。

当前条件操作符：

```text
=
==
!=
<>
>
>=
<
<=
IN
NOT_IN
```

## marketing_reward

奖励配置表。

| 字段 | 说明 |
|---|---|
| `id` | 奖励配置 ID |
| `activity_id` | 活动 ID |
| `reward_code` | 奖励编码 |
| `reward_type` | 奖励类型 |
| `calculate_type` | 计算方式 |
| `reward_config_json` | 奖励配置 JSON |
| `grant_mode` | 发奖模式 |
| `grant_channel` | 发奖通道 |
| `flow_multiple` | 流水倍数 |
| `enabled` | 是否启用 |
| `version` | 版本 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

当前普通活动可创建的奖励类型：

```text
BONUS
CASH
COUPON
POINT
GIFT
DRAW_CHANCE
MEMBERSHIP
```

当前已实现计算方式：

```text
FIXED
RATIO
LADDER
RANDOM
PROBABILITY
POOL_SPLIT
```

当前发奖边界：

```text
grant_mode    AUTO / MANUAL / MANUAL_EXPORT
grant_channel AUTO 下为 HTTP / WEBHOOK / MQ；人工发奖可为空或 MANUAL_EXPORT
targetRef     AUTO 下在 reward_config_json 内必填；人工发奖可为空或作为处理说明
```

MQ destination URI 支持 `kafka://<topic>`、`rabbitmq://<exchange>/<routingKey>`、`rabbitmq:///<routingKey>`（默认 exchange）和 `pulsar://<tenant>/<namespace>/<topic>`。`targetRef` 不允许包含 broker 地址、账号、密码、token、query 或 fragment。`MANUAL` / `MANUAL_EXPORT` 保留 participation、grant_order、quota reservation 与审计链路，但不生成自动 outbox；`MANUAL_EXPORT` 通过 `marketing_manual_grant_export_file` 与 `marketing_manual_grant_export_detail` 追踪导出文件、checksum 和 grant order 明细。

奖励计算统一使用 Decimal 语义，输出金额保留 4 位小数。金额类 `BONUS` / `CASH` 支持 `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`：`FIXED` 使用 `amount`；`RATIO` 使用 `baseField * rate` 并支持 `maxAmount` 封顶；`LADDER` 使用 `field` 和闭区间 `ladders`；`RANDOM` 使用 `minAmount/maxAmount/step` 和稳定 seed；`PROBABILITY` 使用 `items[].weight` 抽取奖项；`POOL_SPLIT` 支持 `splitBy` 或兼容别名 `splitMode` 的 `COUNT` / `WEIGHT` 即时分摊。非金额类当前最小可上线口径只支持 `FIXED`，并通过标准 payload 发给外部系统：

| rewardType | 必填配置 |
|---|---|
| `POINT` | `points` 正整数，`unit` 或 `pointType` |
| `COUPON` | `couponTemplateId`，`quantity` 正整数 |
| `GIFT` | `giftSkuId` 或 `giftCode`，`quantity` 正整数 |
| `DRAW_CHANCE` | `drawPoolCode` 或 `lotteryCode`，`chanceCount` 正整数 |
| `MEMBERSHIP` | `membershipTier`、`membershipPlanId` 或 `benefitPackageId`，并配置 `durationDays` 正整数或 `durationType=PERMANENT` |

金额类 `BONUS` / `CASH` 必须配置 `currency`。`POINT`、`DRAW_CHANCE` 不允许 `currency`；`COUPON` 只有 `faceValue` 需要 `currency`；`GIFT` / `MEMBERSHIP` 只有 `valuationAmount` 需要 `currency`。`flow_multiple > 0` 只允许 `BONUS`，其他奖励类型必须为 `0` 或不传。

非金额 reward 的 `marketing_grant_order.reward_amount` 固定为 `0.0000`，不表示现金金额；实际权益数量、外部引用和可选估值写入 `reward_payload_json`。`valuationAmount` 只用于预算暴露预估，不得当作实际发奖金额。

金额计算结果为 `0.0000` 且无其他实际权益时按无实际奖励处理，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT/DAILY_LIMIT；非金额 reward 只要数量类字段为正整数，即使 `reward_amount=0.0000` 也会生成 grant_order/outbox，并按活动配置扣减 STOCK、USER_LIMIT 和 DAILY_LIMIT。`POOL_SPLIT` 是事件即时分摊模型，不表示活动结束后整池只扣一次。

## marketing_event_inbox

事件收件箱。负责事件幂等和处理状态。

| 字段 | 说明 |
|---|---|
| `id` | Inbox ID |
| `tenant_id` | 租户 ID |
| `event_id` | 外部事件 ID |
| `event_type` | 事件类型 |
| `source_system` | 来源系统 |
| `subject_type` | 主体类型 |
| `subject_id` | 主体 ID |
| `biz_type` | 业务类型 |
| `biz_id` | 业务 ID |
| `occurred_at` | 事件发生时间 |
| `payload_json` | 原始事件 JSON |
| `idempotency_key` | 幂等键，唯一 |
| `process_status` | 处理状态 |
| `error_message` | 错误信息 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

状态：

```text
RECEIVED
PROCESSED
DUPLICATE
FAILED
```

## marketing_participation

参与记录。规则拒绝时单独写 `REJECTED`。规则通过且存在发奖单时，`REWARDABLE` participation 必须与 quota 扣减、grant_order 和 outbox 在同一个 activity 级事务内写入；预算、库存或次数不足时事务回滚，不保留 `REWARDABLE` 脏记录。

| 字段 | 说明 |
|---|---|
| `id` | 参与记录 ID |
| `tenant_id` | 租户 ID |
| `activity_id` | 活动 ID |
| `event_inbox_id` | Inbox ID |
| `subject_type` | 主体类型 |
| `subject_id` | 主体 ID |
| `biz_type` | 业务类型 |
| `biz_id` | 业务 ID |
| `status` | 参与状态 |
| `reject_reason` | 拒绝原因 |
| `snapshot_json` | 命中时活动、事件、规则结果和奖励计算摘要快照 |
| `idempotency_key` | 参与幂等键，唯一 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

状态：

```text
INIT
QUALIFIED
REJECTED
REWARDABLE
GRANTED
FAILED
```

参与幂等键固定格式：

```text
tenantId + ":" + activityId + ":" + eventInbox.idempotency_key
```

其中 `eventInbox.idempotency_key` 固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。因此同一标准事件命中同一活动只能生成一条 participation，不依赖数据库自增 ID、当前时间或随机数。

`snapshot_json` 固定包含：

```text
activity     活动 ID、activityCode、activityType、pluginCode、version、互斥叠加字段、config、rules、rewards
event        inboxId、eventId、eventType、sourceSystem、tenantId、subject、biz、occurredAt、attributes、idempotencyKey
ruleResult   MATCH / QUALIFY / REWARD / RISK_LIMIT_BUDGET 执行摘要
rewards      rewardCode、rewardType、calculateType、grantChannel、amount、currency、skipped、reasonCode、reason
plugin       pluginCode、pluginContext、precheckResult、explain、stateChangeIntents
```

规则拒绝时写 `REJECTED` participation 和 `reject_reason`，不生成 grant_order/outbox，不扣 quota。规则通过但全部奖励为 `0.0000` 时写 `QUALIFIED` participation，并在 `reject_reason` 中记录 `ZERO_REWARD_SKIPPED: no payable reward`；不写 `REWARDABLE`，不生成 grant_order/outbox，不扣 quota。

## marketing_grant_order

发奖单。系统不直接加钱，只生成发奖单和发奖指令。

| 字段 | 说明 |
|---|---|
| `id` | 发奖单 ID |
| `tenant_id` | 租户 ID |
| `activity_id` | 活动 ID |
| `participation_id` | 参与记录 ID |
| `subject_type` | 主体类型 |
| `subject_id` | 主体 ID |
| `reward_type` | 奖励类型 |
| `reward_amount` | 奖励金额 |
| `reward_payload_json` | 奖励扩展 payload |
| `bill_no` | 发奖指令号 / commandId |
| `grant_status` | 发奖状态 |
| `grant_channel` | 发奖通道 |
| `retry_count` | 重试次数 |
| `error_message` | 错误信息 |
| `idempotency_key` | 发奖幂等键，唯一 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

状态：

```text
PENDING
SENDING
SUCCESS
FAILED
RETRY
CLOSED
```

发奖单幂等键固定格式：

```text
participation.idempotency_key + ":" + rewardCode
```

因此同一 participation 的同一奖励配置只能生成一张 grant_order。

`reward_payload_json` 固定包含发奖和对账所需解释字段：

```text
rewardCode
rewardType
calculateType
currency
unit
flowMultiple
targetRef
source / amount / reasonCode
奖励计算方式对应的解释字段，例如 baseField、baseAmount、rate、maxAmount、capped
非金额权益字段，例如 points、quantity、chanceCount、durationDays、couponTemplateId、giftSkuId、drawPoolCode、membershipPlanId、valuationAmount
```

## marketing_quota

预算、库存和次数限制表。

| 字段 | 说明 |
|---|---|
| `id` | Quota ID |
| `activity_id` | 活动 ID |
| `quota_type` | Quota 类型 |
| `scope` | 作用域 |
| `scope_key` | 作用域 key |
| `total_value` | 总额度 |
| `used_value` | 已使用 |
| `frozen_value` | 已冻结，预留字段 |
| `version` | 乐观锁版本 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

Quota 类型：

```text
BUDGET
STOCK
USER_LIMIT
DAILY_LIMIT
```

作用域：

```text
ACTIVITY
USER
IP
DEVICE
```

当前实现：

```text
BUDGET + ACTIVITY + TOTAL
STOCK + ACTIVITY + rewardCode
USER_LIMIT + USER + subject.id
DAILY_LIMIT + USER + subject.id:activityLocalDate
```

约定：

```text
BUDGET 冻结金额 = grant_order 对应 reward_amount。
STOCK 每个 enabled reward 默认冻结 1，scope_key 固定使用 rewardCode。
USER_LIMIT 每个 rewardable participation 默认冻结 1。
DAILY_LIMIT 每个 rewardable participation 默认冻结 1，scope_key 使用 subject.id 加活动 timezone 自然日。
缺少对应 quota 行表示不限制，兼容未配置预算、库存或次数限制的活动。
```

冻结、确认和释放都必须使用条件更新。冻结时：

```sql
UPDATE marketing_quota
SET frozen_value = frozen_value + $amount,
    version = version + 1
WHERE used_value + frozen_value + $amount <= total_value;
```

`SUCCESS` callback 将 reservation 从 `FROZEN` 置为 `CONFIRMED`，并把同额 `frozen_value` 转入 `used_value`；`FAILED` callback 和 Admin 超时释放入口将 reservation 置为 `RELEASED`，只减少 `frozen_value`。不限额活动没有 quota row，也可以没有 reservation。

## marketing_outbox

发奖指令 Outbox。

| 字段 | 说明 |
|---|---|
| `id` | Outbox ID |
| `tenant_id` | 租户 ID |
| `message_type` | 消息类型 |
| `target_channel` | 目标通道 |
| `target_ref` | 目标地址或引用；MQ 为 provider destination URI |
| `payload_json` | 消息 payload |
| `send_status` | 发送状态 |
| `retry_count` | 重试次数 |
| `next_retry_at` | 下次重试时间 |
| `error_message` | 错误信息 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

状态：

```text
PENDING
SENDING
SENT
FAILED
DLQ
DLQ_CLOSED
```

PostgreSQL worker 必须使用：

```text
FOR UPDATE SKIP LOCKED
```

先 claim 到 `SENDING`，再投递，避免多 worker 重复发奖。

## marketing_audit

审计表。

| 字段 | 说明 |
|---|---|
| `id` | 审计 ID |
| `tenant_id` | 租户 ID |
| `actor` | 操作人 |
| `action` | 操作类型 |
| `resource` | 资源类型 |
| `resource_id` | 资源 ID |
| `payload_json` | 操作 payload |
| `created_at` | 创建时间 |

当前 action：

```text
CREATE_ACTIVITY
UPDATE_ACTIVITY
APPROVAL_SUBMIT
APPROVAL_APPROVE
APPROVAL_REJECT
CREATE_QUOTA
EVENT_IN
SIMULATION_RUN
GRANT_FAILED
QUOTA_EXCEEDED
OUTBOX_DELIVERY_FAILED
OUTBOX_ENTER_DLQ
OUTBOX_DLQ_REPLAY
OUTBOX_DLQ_CLOSE
```

`resource` 使用稳定对象类型，例如 `ACTIVITY`、`APPROVAL`、`SIMULATION`、`GRANT_ORDER`、`OUTBOX`、`QUOTA`、`EVENT` 和 `WEBHOOK_RECEIVER`。`payload_json` 必须包含可追踪的 `source` 摘要；HTTP 来源只记录 method、path、requestId/traceId、脱敏 IP 和 userAgent 摘要，不记录 Authorization、Cookie、token 或 secret。

## marketing_fact_provider

Fact Provider registry。运行时 Fact Query 和活动规则校验只按 `tenant_id + fact_code` 读取 enabled provider，不接受 inline provider 配置。

| 字段 | 说明 |
|---|---|
| `tenant_id` | 租户 ID，和 `fact_code` 组成主键 |
| `fact_code` | Fact 编码 |
| `provider_type` | Provider 类型，当前只支持 `HTTP` |
| `endpoint` | HTTP endpoint |
| `method` | HTTP method，当前支持 `GET` / `POST` |
| `auth_ref` | 鉴权配置引用，不存放明文密钥 |
| `mapping_json` | 外部 JSON 响应到 `facts.<factCode>.*` 的显式字段映射 |
| `cache_ttl_seconds` | 缓存 TTL，`0` 不缓存，`1..3600` 启用缓存 |
| `enabled` | 是否允许规则引用和 Fact Query 执行 |
| `timeout_ms` | Provider 请求超时配置 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

`mapping_json` 最小结构为 `{ "<outputPath>": { "source": "$.<providerResponsePath>", "type": "string|number|boolean|array|object", "required": true|false } }`。`outputPath` 是相对路径，例如 `vipLevel` 或 `risk.score`，不得包含 `facts.` 前缀；`required` 可省略，默认 `true`。

Fact Query 缓存只缓存成功 mapping 后的 facts。缓存 key 必须包含 `tenant_id`、`fact_code`、规范化 provider 输入上下文和 provider 配置版本。provider 配置版本优先使用 `updated_at`；如果运行时没有可靠 `updated_at`，使用 `provider_type + endpoint + method + auth_ref + mapping_json + cache_ttl_seconds + enabled + timeout_ms` 的规范化配置指纹。配置变更后不要求主动清理旧缓存，后续请求会因版本 key 变化自然绕过旧缓存。
