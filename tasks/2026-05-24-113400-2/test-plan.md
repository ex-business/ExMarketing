# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本轮测试重点是“统一活动配置模型”在创建、读回和现有事件执行链路中的可验收性。测试分三层：

1. API / handler 层覆盖 `POST /admin/v1/activities` 的合法创建、严格 Schema、枚举、版本和未知字段拒绝。
2. 事件链路层覆盖合法活动命中后仍生成 `participation + grant_order + outbox`，并保持 quota 不足、幂等重复和并发下的既有原子语义。
3. PostgreSQL 层覆盖迁移、持久化读回、旧数据 reward version 默认值、quota 条件扣减和 outbox target 字段兼容真实数据库。

默认可验收 rewardType 固定为 `BONUS` / `CASH` / `POINT`。如 Dev 扩展 `COUPON` 等类型，必须同步补充 rewardConfig Schema、投递 payload 断言和端到端用例，否则测试按非法类型拒绝处理。

## Test Cases

### 正常路径

- `POST /admin/v1/activities` 创建 `CONDITION + FIXED + AUTO + HTTP` 活动，包含 activity `config.entry/display/audience/metadata`、`priority`、`exclusiveGroup`、`stackable`、activity/rule/reward `version`。期望 `201`，读回包含 config、rule version、reward version、`grantMode=AUTO`、`grantChannel=HTTP`、`targetRef=http(s)`。
- 创建 `CONDITION + RATIO + AUTO + WEBHOOK` 活动，`rewardConfig.baseField=attributes.amount`、`rate`、`maxAmount`、`flowMultiple`、`targetRef=https://...`。期望 `201`，上报匹配事件后生成 `REWARDABLE` participation、grant_order、outbox；outbox `target_channel=WEBHOOK` 且 `target_ref` 等于创建时的 `targetRef`。
- 创建无规则活动，保留合法奖励和触发事件。期望 `201`，上报匹配事件时仅依赖租户、状态、时间窗口和 trigger event 命中。
- 创建请求省略 activity/rule/reward `version` 或传 `0` / `1`。期望成功后读回均为 `1`。
- 创建 `flowMultiple` 只在顶层或只在 `rewardConfig` 中出现。期望成功，读回语义稳定；两处相同值也应成功。

### Schema 与非法配置

- 顶层未知字段返回 `400`，错误信息能定位到 activity 顶层。
- `config` 非 object、包含未知一层字段、`entry.channels` 非字符串数组、URL 非 `http(s)`、`display.visibleFrom > visibleTo` 均返回 `400`。
- `ruleConfig` 包含未知执行字段、`logic` 非 `AND/OR`、`conditions[n].field` 为空、`op` 非允许值、`IN/NOT_IN` 的 `value` 非数组、数值比较 value 非 Decimal 均返回 `400`。
- 未开放 `ruleType=FORMULA/LIMIT/SCRIPT` 返回 `400`。
- `rewardConfig` 包含未知执行字段、`FIXED.amount` 缺失/非 Decimal/小于等于 0、`RATIO.baseField` 缺失、`rate` 非 Decimal/小于等于 0、`maxAmount` 非 Decimal/小于等于 0 均返回 `400`。
- 未开放 `calculateType=LADDER/RANDOM/PROBABILITY/POOL_SPLIT` 返回 `400`。
- `rewardType` 非本任务开放范围返回 `400`；若实现扩展类型，必须有对应正反例和 outbox payload 断言。
- `grantMode=MANUAL`、`grantChannel=MQ`、`grantChannel=MANUAL_EXPORT` 均返回 `400`。
- `grantChannel=HTTP/WEBHOOK` 但 `rewardConfig.targetRef` 缺失、为空或非 `http(s)` URL 返回 `400`。
- 顶层 `flowMultiple` 与 `rewardConfig.flowMultiple` 冲突返回 `400`。
- `triggerEvents` 缺失、空数组、包含空字符串、重复事件返回 `400`。
- 非法 `timezone`、非法 `status`、`startTime >= endTime` 返回 `400`。
- activity/rule/reward `version > 1` 或非整数返回 `400`。
- `audience` 仅作为展示/圈选元数据验证：把资格条件只放在 `config.audience` 不应影响规则资格；真正资格仍必须由 `rules` 决定。

### 额度不足

- 创建合法活动后配置 `BUDGET` quota 小于本次奖励总额，上报匹配事件。期望返回 `409 QUOTA_EXCEEDED`，不返回 grant/outbox 结果，不新增 `REWARDABLE` 脏记录，写 `QUOTA_EXCEEDED` audit，quota `used_value` 不增加。
- 配置 `STOCK` quota 为 0 或不足，scopeKey 使用 `rewardCode`。期望 `409 QUOTA_EXCEEDED`，不生成 grant_order/outbox，audit payload 包含 `quotaType=STOCK` 和对应 `scopeKey`。
- 配置 `USER_LIMIT` 对同一 subject 已达上限。期望 `409 QUOTA_EXCEEDED`，不扣 BUDGET/STOCK，不生成 grant_order/outbox。
- 多奖励活动中第二个奖励库存不足。期望整个 activity 级事务回滚，第一奖励也不能留下 quota 扣减、grant_order 或 outbox。

### 幂等重复

- 同一合法事件按相同 `Idempotency-Key` 完全重复上报两次。第一次成功生成 participation/grant_order/outbox 并扣 quota；第二次返回 `200` 且 `duplicate=true`，不新增 participation/grant_order/outbox，不重复扣 BUDGET/STOCK/USER_LIMIT。
- 相同 `Idempotency-Key` 但 payload 不一致。期望 `400 INVALID_IDEMPOTENCY_KEY`，不执行活动匹配，不写 grant/outbox，不扣 quota。
- 创建活动接口的重复 `activityCode + tenantId` 行为按现有唯一性/校验语义验证：不能产生两套同编码可执行活动导致重复发奖；如当前语义是冲突，则断言 4xx。

### 并发

- PostgreSQL 下同一 activity 的 `BUDGET` 只够 1 次奖励，并发提交两个不同幂等事件。期望最多 1 个成功，另一个 `409 QUOTA_EXCEEDED`，最终 `used_value <= total_value`，grant_order/outbox 数量等于成功次数。
- PostgreSQL 下同一 reward `STOCK` 只剩 1，并发提交两个不同 subject 的匹配事件。期望最多 1 个成功，库存不超扣。
- 并发重复同一个幂等事件。期望最多 1 条 inbox 进入正常处理，其余识别为 duplicate 或幂等冲突；不能重复扣 quota 或重复生成 outbox。
- 如有 outbox worker 测试，保留 `FOR UPDATE SKIP LOCKED` 语义回归：并发 worker 不应重复 claim 同一 outbox。

### PostgreSQL smoke

- 启动本地 PostgreSQL 后跑 repository / postgres 包测试，覆盖 migration 可用、activity config JSON、ruleConfig、rewardConfig、grantMode、grantChannel、flowMultiple、version 字段写入和读回。
- 对旧 `marketing_reward` 数据执行读回测试：缺失或历史默认 version 应返回 `1`。
- 用真实 PostgreSQL 跑一次完整 smoke：创建合法活动 -> 创建 BUDGET/STOCK/USER_LIMIT quota -> 上报匹配事件 -> 查询 participation、grant_order、outbox、quota、audit，确认字段和值与 API 响应一致。
- PostgreSQL 下验证非法配置不会写入半成品 activity/rule/reward 行。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

PostgreSQL smoke：

~~~bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run Test
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Activity|Quota|Idempot|Outbox|Config'
~~~

## Data / Smoke

基础合法活动数据：

- `tenantId=site_001`
- `activityCode=UNIFIED_CONFIG_SMOKE`
- `activityType=FIRST_DEPOSIT`
- `status=ACTIVE`
- `triggerEvents=["DEPOSIT_SUCCESS"]`
- `timezone=UTC`
- `ruleType=CONDITION`
- `ruleConfig.conditions=[{"field":"attributes.amount","op":">=","value":"100.00"}]`
- `rewardType=BONUS`
- `calculateType=FIXED` 或 `RATIO`
- `grantMode=AUTO`
- `grantChannel=HTTP` 或 `WEBHOOK`
- `rewardConfig.targetRef=https://wallet.example.com/reward`

事件 smoke：

- `Idempotency-Key=site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_unified_config_001`
- payload 顶层只包含标准事件字段。
- `attributes.amount="299.00"`，`attributes.isFirstDeposit=true`。

额度 smoke：

- BUDGET 不足：`totalValue` 小于奖励总额。
- STOCK 不足：`quotaType=STOCK`、`scope=ACTIVITY`、`scopeKey=<rewardCode>`、`totalValue=0`。
- USER_LIMIT 不足：`quotaType=USER_LIMIT`、`scope=USER`、`scopeKey=<subject.id>`、`totalValue=0`。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113400-2
