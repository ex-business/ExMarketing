# Test Plan

## Agent 指令

你是 Test Agent。请在 Dev Agent 完成实现后，按本计划执行验证。

约束：

- 本文件仅定义后续测试用例和命令清单。
- 当前 Test Plan Agent 不执行测试、不构建、不格式化、不修改业务代码。
- Test Agent 执行时应记录每个用例的请求、响应、关键数据库断言和最终结论。

## Test Strategy

本需求风险点是重复事件导致重复参与、重复发奖、重复 outbox 和重复 quota 扣减，因此测试必须同时覆盖 API 行为、业务副作用、并发竞态和 PostgreSQL 唯一性。

测试分层：

- 单元/服务层：验证 `Idempotency-Key` 必填、key 推导、payload 一致性比较、重复/冲突分支、quota 不足回滚。
- HTTP 集成层：通过 `POST /openapi/v1/events` 验证状态码、错误码、`duplicate=true` 和正常处理结果。
- PostgreSQL 集成层：使用真实 PostgreSQL 仓储验证 `marketing_event_inbox.idempotency_key` 唯一性、事务回滚和并发重复请求。
- 并发测试：同一 key 的相同 payload 并发提交，最终只能产生一套业务副作用，且不得出现 500。

## Test Data

基础标准事件：

~~~json
{
  "eventId": "evt-test-0001",
  "eventType": "ORDER_PAID",
  "sourceSystem": "mall",
  "tenantId": "tenant-a",
  "subject": {
    "type": "USER",
    "id": "user-1001"
  },
  "biz": {
    "bizType": "ORDER",
    "bizId": "order-1001"
  },
  "occurredAt": "2026-05-24T10:00:00Z",
  "attributes": {
    "amount": "99.00",
    "currency": "USD"
  }
}
~~~

对应 Header：

~~~text
Idempotency-Key: tenant-a:mall:ORDER_PAID:ORDER:order-1001
Content-Type: application/json
~~~

变体数据：

- `duplicatePayload`：与基础标准事件完全一致。
- `conflictPayload`：Header key 不变，但修改 `attributes.amount`、`subject.id` 或 `occurredAt` 任一字段。
- `mismatchedKeyPayload`：payload 不变，但 Header key 改为 `tenant-a:mall:ORDER_PAID:ORDER:wrong-order`。
- `missingKeyPayload`：payload 不变，但不发送 `Idempotency-Key`。
- `quotaExceededPayload`：命中一个 quota 已耗尽或库存/预算不足的活动。
- `zeroRewardPayload`：命中奖励金额为 0 的活动。
- `rejectedPayload`：命中活动但规则不通过。
- `noCampaignPayload`：不命中任何活动。

## Test Cases

### TP-01 正常路径：首次事件正常处理

前置条件：

- 配置一个可命中的活动，规则通过，奖励大于 0，BUDGET/STOCK/USER_LIMIT 均充足。
- PostgreSQL 或测试仓储为空，目标 `idempotency_key` 尚不存在。

步骤：

1. 发送基础标准事件和正确 `Idempotency-Key`。
2. 检查 HTTP 响应为成功，且不是重复结果。
3. 查询 `marketing_event_inbox`、participation、grant_order、outbox、quota 使用量。

期望：

- 返回成功状态码，`duplicate` 不为 `true`。
- `marketing_event_inbox` 对该 key 只有 1 条记录。
- 生成 1 条对应活动 participation。
- 生成 1 张对应 reward 的 grant_order。
- 生成 1 条对应 outbox 消息。
- BUDGET、STOCK、USER_LIMIT used 值各只增加一次。

### TP-02 正常路径：无命中活动

步骤：

1. 发送 `noCampaignPayload` 和匹配的 `Idempotency-Key`。
2. 再次发送完全相同请求。

期望：

- 首次请求成功返回，不产生 participation、grant_order、outbox。
- 重复请求返回 `200 OK` 且 `duplicate=true`。
- inbox 对该 key 只有 1 条记录。
- quota 不变化。

### TP-03 正常路径：规则拒绝

步骤：

1. 发送 `rejectedPayload` 和匹配的 `Idempotency-Key`。
2. 再次发送完全相同请求。

期望：

- 首次请求按现有规则拒绝语义返回。
- 可写 1 条 `REJECTED` participation。
- 不生成 grant_order 和 outbox。
- 重复请求返回 `200 OK` 且 `duplicate=true`。
- 不新增第二条 `REJECTED` participation。
- quota 不重复扣减。

### TP-04 正常路径：奖励为 0

步骤：

1. 发送 `zeroRewardPayload` 和匹配的 `Idempotency-Key`。
2. 再次发送完全相同请求。

期望：

- 首次请求成功。
- 可写 1 条 `QUALIFIED` participation。
- 不生成 grant_order 和 outbox。
- 重复请求返回 `200 OK` 且 `duplicate=true`。
- 不新增第二条 `QUALIFIED` participation。
- quota 不重复扣减。

### TP-05 额度不足：首次事件原子失败

前置条件：

- 配置一个可命中的活动，规则通过，奖励大于 0。
- 将 BUDGET、STOCK 或 USER_LIMIT 调整为不足以完成发奖。

步骤：

1. 发送 `quotaExceededPayload` 和匹配的 `Idempotency-Key`。
2. 查询 inbox、participation、grant_order、outbox、quota 使用量。

期望：

- 返回 `409 QUOTA_EXCEEDED`。
- 不生成 grant_order。
- 不生成 outbox。
- 不保留 `REWARDABLE` 脏 participation。
- quota used 不增加，或在同一事务内完整回滚到请求前状态。
- 不出现部分成功副作用。

### TP-06 额度不足：失败后重复上报不产生额外副作用

步骤：

1. 在 TP-05 前置条件下，连续两次发送相同 `quotaExceededPayload`。
2. 对比两次请求前后的业务表和 quota。

期望：

- 两次请求都不得产生 grant_order/outbox。
- 不得出现重复扣减或负库存/负预算。
- 如果实现选择记录失败 inbox，则同 key 最终仍只能有 1 条 inbox；重复请求行为必须稳定。
- 如果实现选择失败不落 inbox，也不得产生任何业务副作用。

### TP-07 幂等重复：相同 key + 相同 payload

步骤：

1. 发送基础标准事件。
2. 再发送完全相同的 Header 和 payload。
3. 第三次再发送完全相同请求。

期望：

- 第 1 次为首次成功处理。
- 第 2、3 次返回 `200 OK` 且 `duplicate=true`。
- inbox 只有 1 条。
- participation 只有 1 条。
- grant_order 只有 1 张。
- outbox 只有 1 条。
- BUDGET/STOCK/USER_LIMIT used 只增加一次。

### TP-08 幂等冲突：相同 key + 不同 payload

步骤：

1. 发送基础标准事件并成功处理。
2. 使用相同 Header key 发送 `conflictPayload`。

期望：

- 第 2 次返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 不新增 inbox。
- 不新增 participation/grant_order/outbox。
- quota 不变化。
- 错误响应不得泄露完整 token、Authorization header 或敏感 payload。

### TP-09 Header key 缺失

步骤：

1. 发送 `missingKeyPayload`，不带 `Idempotency-Key`。

期望：

- 返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 不写入 inbox。
- 不产生 participation/grant_order/outbox。
- quota 不变化。

### TP-10 Header key 与 payload 推导值不一致

步骤：

1. 发送 `mismatchedKeyPayload`。

期望：

- 返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 不写入 inbox。
- 不产生 participation/grant_order/outbox。
- quota 不变化。

### TP-11 JSON 规范化：字段顺序和空白不构成冲突

步骤：

1. 发送基础标准事件。
2. 使用相同 key 发送语义完全相同但 JSON 字段顺序、缩进、空白不同的 payload。

期望：

- 第 2 次返回 `200 OK` 且 `duplicate=true`。
- 不误判为 `INVALID_IDEMPOTENCY_KEY`。
- 不产生任何额外副作用。

### TP-12 JSON 内容变化：标准字段变化构成冲突

步骤：

1. 发送基础标准事件。
2. 使用相同 key 分别修改以下字段后发送：`eventId`、`subject.id`、`occurredAt`、`attributes.amount`。

期望：

- 每个修改后的请求都返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 不产生额外副作用。
- inbox、participation、grant_order、outbox、quota 均保持首次请求后的数量。

### TP-13 并发：相同 key + 相同 payload

步骤：

1. 准备一个新的合法事件 key。
2. 使用 20 到 100 个并发请求同时提交完全相同的 Header 和 payload。
3. 收集所有响应状态码和响应体。
4. 查询数据库最终状态。

期望：

- 不出现 500。
- 至多 1 个请求表现为首次处理成功，其余请求表现为稳定重复结果，或按实现定义返回等价的幂等成功结果。
- inbox 只有 1 条。
- participation 只有 1 条。
- grant_order 只有 1 张。
- outbox 只有 1 条。
- BUDGET/STOCK/USER_LIMIT used 只增加一次。

### TP-14 并发：相同 key + 不同 payload 混合

步骤：

1. 准备一个新的合法事件 key。
2. 并发提交一半基础 payload、一半 `conflictPayload`，Header key 相同。
3. 查询数据库最终状态。

期望：

- 不出现 500。
- 最终只能有一种 payload 成为该 key 的唯一事件。
- 与已落库 payload 不一致的请求返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 不产生双 participation、双 grant_order、双 outbox 或重复 quota 扣减。

### TP-15 PostgreSQL smoke：唯一约束存在并生效

前置条件：

- 使用真实 PostgreSQL 测试数据库。
- 已执行最新 migration。

步骤：

1. 检查 `marketing_event_inbox.idempotency_key` 存在唯一索引或唯一约束。
2. 直接或通过仓储尝试插入两条相同 `idempotency_key` 的 inbox 记录。
3. 通过 API 重复提交相同事件，确认应用层将数据库唯一冲突转换为稳定幂等响应。

期望：

- 数据库层拒绝重复 key。
- API 层不向用户暴露原始数据库错误。
- 相同 payload 的重复请求返回 `200 OK duplicate=true`。
- 不同 payload 的重复 key 返回 `400 INVALID_IDEMPOTENCY_KEY`。

### TP-16 PostgreSQL smoke：事务回滚

前置条件：

- 使用真实 PostgreSQL 测试数据库。
- 构造 quota 不足场景。

步骤：

1. 发送 `quotaExceededPayload`。
2. 查询 participation、grant_order、outbox、quota 使用量。
3. 重复发送同一请求。

期望：

- quota 不足返回 `409 QUOTA_EXCEEDED`。
- grant_order/outbox 不存在。
- 不保留 `REWARDABLE` 脏数据。
- quota 使用量保持请求前状态。
- 重复发送不改变最终状态。

## Required Commands

Test Agent 应执行以下基础命令：

~~~bash
go test ./...
go build ./...
~~~

如项目提供 PostgreSQL 集成测试标签或环境变量，Test Agent 还应执行对应 PostgreSQL 测试命令。建议命令形态如下，实际以项目已有 Makefile、脚本或测试约定为准：

~~~bash
TEST_DATABASE_URL=postgres://<user>:<password>@<host>:<port>/<db>?sslmode=disable go test ./... -tags=integration
~~~

并发专项测试如已有专用测试名，应单独执行：

~~~bash
go test ./... -run 'Test.*EventInbox.*Concurrent|Test.*Idempotency.*Concurrent' -count=1
~~~

PostgreSQL smoke 如已有专用测试名，应单独执行：

~~~bash
go test ./... -run 'Test.*EventInbox.*Postgres|Test.*Idempotency.*Postgres|Test.*Migration.*Inbox' -count=1
~~~

## Data / Smoke

Test Agent 需要在执行后记录以下数据库断言结果：

- `marketing_event_inbox`：按 `idempotency_key` 计数必须为 1。
- participation：按活动和 participation idempotency key 计数必须为 1 或按场景为 0。
- grant_order：按 participation 和 reward code 计数必须为 1 或按场景为 0。
- outbox：按事件或 grant_order 关联计数必须为 1 或按场景为 0。
- quota：BUDGET、STOCK、USER_LIMIT used 值在重复请求后不得再次增加。
- PostgreSQL schema：`marketing_event_inbox.idempotency_key` 必须有唯一性保障。

最低 smoke 矩阵：

| 场景 | 期望响应 | inbox | participation | grant_order | outbox | quota |
| --- | --- | --- | --- | --- | --- | --- |
| 首次正常发奖 | 成功 | 1 | 1 | 1 | 1 | 增加 1 次 |
| 相同请求重复 | `200 duplicate=true` | 1 | 1 | 1 | 1 | 不再增加 |
| 相同 key 不同 payload | `400 INVALID_IDEMPOTENCY_KEY` | 1 | 不新增 | 不新增 | 不新增 | 不变化 |
| 缺少 key | `400 INVALID_IDEMPOTENCY_KEY` | 0 | 0 | 0 | 0 | 不变化 |
| key 不匹配 | `400 INVALID_IDEMPOTENCY_KEY` | 0 | 0 | 0 | 0 | 不变化 |
| quota 不足 | `409 QUOTA_EXCEEDED` | 0 或 1，按实现定义但必须稳定 | 无 `REWARDABLE` 脏数据 | 0 | 0 | 回滚 |
| 并发重复 | 无 500，最终幂等成功 | 1 | 1 | 1 | 1 | 增加 1 次 |
| PostgreSQL 重复插入 | 数据库拒绝重复 key | 1 | 不适用 | 不适用 | 不适用 | 不适用 |

## Decision

~~~text
PASS
~~~

需求：2026-05-24-event-inbox
