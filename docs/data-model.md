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
| `exclusive_group` | 互斥组 |
| `stackable` | 是否允许叠加命中 |
| `version` | 配置版本 |
| `config_json` | 扩展配置 |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |

活动状态：

```text
DRAFT
SCHEDULED
ACTIVE
PAUSED
ENDED
```

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

规则类型：

```text
CONDITION
FORMULA
LIMIT
```

当前已实现：

```text
CONDITION
```

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
POINT
```

当前已实现计算方式：

```text
FIXED
RATIO
```

设计目标中的待实现计算方式：

```text
LADDER
RANDOM
PROBABILITY
POOL_SPLIT
DRAW_CHANCE
GIFT
```

当前可执行发奖边界：

```text
grant_mode    AUTO
grant_channel HTTP / WEBHOOK
targetRef     reward_config_json 内必填，且必须是 http(s) URL
```

`MQ`、`MANUAL_EXPORT` 和 `MANUAL` 发奖模式尚无完整 sender、导出或人工状态机，创建活动时会被拒绝。

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
| `snapshot_json` | 命中时活动配置快照 |
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
```

约定：

```text
BUDGET 扣减金额 = 同一 activity 本次所有 reward_amount 合计。
STOCK 每个 enabled reward 默认扣减 1，scope_key 固定使用 rewardCode。
USER_LIMIT 每个 rewardable participation 默认扣减 1。
缺少对应 quota 行表示不限制，兼容未配置预算、库存或次数限制的活动。
```

PostgreSQL 扣减必须使用条件更新：

```sql
UPDATE marketing_quota
SET used_value = used_value + $amount,
    version = version + 1
WHERE used_value + $amount <= total_value;
```

## marketing_outbox

发奖指令 Outbox。

| 字段 | 说明 |
|---|---|
| `id` | Outbox ID |
| `tenant_id` | 租户 ID |
| `message_type` | 消息类型 |
| `target_channel` | 目标通道 |
| `target_ref` | 目标地址或引用 |
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
CREATE_QUOTA
EVENT_IN
QUOTA_EXCEEDED
```

## marketing_fact_provider

Fact Provider 元数据表，当前只建表，运行时还没有从表加载 provider。

| 字段 | 说明 |
|---|---|
| `fact_code` | Fact 编码 |
| `provider` | Provider 类型 |
| `endpoint` | HTTP endpoint |
| `method` | HTTP method |
| `headers_json` | 请求头配置 |
| `cache_ttl_seconds` | 缓存 TTL |
| `created_at` | 创建时间 |
| `updated_at` | 更新时间 |
