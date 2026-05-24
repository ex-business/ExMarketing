# Analysis

## Agent 指令

你是 Analysis Agent。请读取本目录 task.md、项目 README.md、CLAUDE.md、docs/architecture.md 和相关代码。

只做分析，不改代码，不提交。

## 评分规则

满分 100，低于 70 不允许进入研发：

~~~text
需求清晰度        20
范围可控          20
风险可控          20
验收标准明确      20
架构一致性        20
~~~

## 现状

当前 ExMarketing 已有事件接入、活动匹配、规则计算、奖励计算、participation、grant_order、outbox、quota 和 audit 的第一版能力。

已实现的关键点：

- `internal/engine/service.go` 中 `ProcessEvent` 先写 `marketing_event_inbox` 做外部事件幂等，重复请求直接返回 duplicate，不再重复发奖。
- `evaluateActivity` 会创建 participation，计算多个 reward，然后调用 `CreateGrantOutboxesWithQuota` 写 grant_order/outbox 并扣 quota。
- `internal/postgres/repository.go` 的 `CreateGrantOutboxesWithQuota` 已在一个 PostgreSQL transaction 内扣 `BUDGET + ACTIVITY + TOTAL` 和 `USER_LIMIT + USER + subjectID`，再写 grant_order/outbox。
- PostgreSQL quota 扣减使用条件更新 `used_value + amount <= total_value`，避免读后写式并发超扣。
- HTTP 层对 `ErrQuotaExceeded` 返回 `409 Conflict`。

主要缺口：

- participation 当前在 quota 扣减事务外提前写入。若预算/次数不足，仍会留下 `REWARDABLE` participation，不满足“活动发奖前同一事务内原子校验和扣减”的严格要求。
- 库存 `STOCK` 只在模型和文档中定义，业务发奖路径没有扣减。多奖励活动无法按每个 reward 扣对应库存。
- `CreateGrantOutboxesWithQuota` 只接收 `totalRewardAmount` 和用户次数开关，不能表达每个 reward 的库存扣减需求。
- quota 不足或原子扣减失败时，当前只更新 inbox failed 并返回错误，没有明确写失败审计记录。
- 事务只覆盖单个 activity 的 quota/grant/outbox，不覆盖 participation。若一个事件命中多个活动，活动之间仍是逐个处理，不应扩大为跨活动全局事务，除非后续需求明确要求。
- 测试目前主要覆盖 HTTP happy path 和 workflow，对 quota 不足、多奖励原子性、重复事件不重复扣减、PostgreSQL 并发条件更新缺少针对性覆盖。

## 目标理解

本任务要把发奖前的资源占用改成强账务语义：

- 活动预算、奖励库存、用户参与次数必须在同一个事务内完成校验和扣减。
- 扣减失败时不能生成 grant_order，也不能生成 outbox。
- 同一幂等事件重复请求不能重复扣减预算、库存、次数。
- 多奖励活动中，同一 activity 命中的所有 reward 的 quota 扣减、grant_order、outbox 必须 all-or-nothing。
- 失败时要有可审计记录，并对调用方返回明确错误，quota 不足保持 `409 Conflict`。

本需求的合理边界是“单个 activity 发奖事务原子化”。事件 inbox 的幂等写入仍可保留在事务外作为事件级接入门禁；同一事件命中多个活动时，各 activity 独立处理，避免把多个活动和外部顺序策略绑成一个过大的事务。

## 方案

建议按“最小可控改动”实现：

1. 重构 repository 原子接口

将当前 `CreateGrantOutboxesWithQuota` 扩展或替换为一个更明确的 activity 级事务接口，例如：

```go
CreateParticipationGrantOutboxesWithQuota(ctx, cmd AtomicGrantCommand) (AtomicGrantResult, error)
```

该接口在一个事务内完成：

- 插入 participation。
- 原子扣 `BUDGET + ACTIVITY + TOTAL`，扣减金额为该 activity 下所有 reward amount 的合计。
- 原子扣 `USER_LIMIT + USER + subjectID`，扣减值为 `1`。
- 按 reward 配置扣 `STOCK`。建议默认使用 `STOCK + ACTIVITY + rewardCode` 或 `STOCK + ACTIVITY + rewardID`，需在实现阶段固定一种 scopeKey 并同步文档。
- 插入所有 grant_order。
- 插入所有 outbox。
- 任一步失败则 rollback，返回明确错误和失败 quota 类型。

2. 调整 engine 编排

`evaluateActivity` 仍负责规则判断、奖励计算、组装 participation/grant/outbox，但不再提前写 rewardable participation。规则拒绝的 participation 可以继续单独写入 rejected，因为它不进入发奖扣减链路。

发奖路径改为：规则通过 -> 计算全部 reward -> 组装 atomic command -> repository 事务内写 participation、扣 quota、写 grant/outbox。

3. 库存扣减配置

需要定义 reward 到库存 quota 的映射。可采用不改 schema 的方式：

- 每个 enabled reward 默认扣 1 个库存。
- 库存 quota 行使用 `quotaType=STOCK`、`scope=ACTIVITY`、`scopeKey=<rewardCode>`。
- 若不存在对应 STOCK quota，沿用现有 quota 语义：不存在即无限制。

如果后续需要一个 reward 扣多份库存或按 payload 扣库存，可以再在 `rewardConfig` 增加字段；本任务不建议扩 schema。

4. 失败审计

新增 audit action，例如：

```text
EVENT_REWARD_REJECTED
QUOTA_EXCEEDED
```

当预算、库存或用户次数不足时，写入 tenant、activityId、eventInboxId、subject、biz、quotaType、scope、scopeKey、amount、reason。审计记录可在事务外写入；它不应阻断主错误返回。

5. 错误表达

保留 `marketing.ErrQuotaExceeded` 供 HTTP 层识别 `409`。为便于审计和测试，可以增加结构化错误或包装错误，包含 quota 类型：

```text
quota exceeded: BUDGET ACTIVITY TOTAL
quota exceeded: STOCK ACTIVITY <rewardCode>
quota exceeded: USER_LIMIT USER <subjectID>
```

HTTP 对外仍可返回简洁错误，测试可断言状态码和不产生 grant/outbox。

6. 内存仓储对齐

`MemoryRepository` 只用于本地快速开发，但必须同步实现同一接口和同一 all-or-nothing 语义，使用现有 mutex 即可。PostgreSQL 语义仍以数据库测试为准。

## 改动范围

预计修改：

- `internal/marketing/repository.go`：调整原子发奖事务接口和必要 command/result 类型。
- `internal/marketing/model.go`：如需要，增加结构化 quota 错误或 audit action 常量；不建议新增表字段。
- `internal/engine/service.go`：把 rewardable participation 写入移动到原子事务内；接入库存 quota；失败审计。
- `internal/postgres/repository.go`：实现 participation + budget + stock + user limit + grant_order + outbox 的单事务写入。
- `internal/marketing/memory_repository.go`：对齐原子接口，保证本地模式行为一致。
- `internal/httpapi/handler.go`：确认 quota 错误继续返回 `409 Conflict`，必要时保留更明确错误消息。
- `internal/httpapi/handler_test.go` 或新增 engine/repository 测试：补充重复事件、quota 不足、多奖励 all-or-nothing。
- `docs/api.md`、`docs/data-model.md`、`docs/architecture.md`：同步 STOCK quota 的 scopeKey 约定、失败审计 action 和处理流程。

不建议修改：

- 不新增外部业务系统依赖。
- 不连接外部业务库。
- 不直接发奖或改用户余额。
- 不扩大为跨活动全局事务。

## 风险点

- 高风险：这是预算、库存、次数和发奖单的账务安全路径，必须以 PostgreSQL 事务和条件更新为准，不能只靠内存仓储测试。
- 事务边界风险：如果继续在事务外写 rewardable participation，会留下“失败但可发奖”的脏状态；这是必须修复的 P0 点。
- 库存映射风险：`STOCK` 的 `scopeKey` 若不固定，会导致运营创建 quota 和发奖扣减不一致。实现前必须在代码和文档中统一为 rewardCode 或 rewardID。
- 多奖励风险：任一 reward 的库存不足时，所有预算、次数、grant_order、outbox 都必须 rollback。
- 幂等风险：外部事件幂等已在 inbox 层处理，但 participation/grant_order 仍有唯一键。实现时要保留唯一键兜底，避免并发重复请求在异常路径重复扣减。
- 审计风险：失败审计不能放在同一个会 rollback 的事务里，否则失败原因会随事务回滚丢失。
- 兼容风险：当前不存在 quota 表示无限制。新增 STOCK 扣减时也应沿用“不存在对应 STOCK quota 即不限制”，避免破坏未配置库存的现有活动。

## 验收方式

最低验证：

```bash
go test ./...
go build ./...
```

必须增加并通过的行为测试：

- 预算不足：返回 `409`，不生成 grant_order，不生成 outbox，不扣 USER_LIMIT/STOCK，写失败审计。
- 用户次数不足：返回 `409`，不生成 grant_order/outbox，不扣 BUDGET/STOCK，写失败审计。
- 单个 reward 库存不足：返回 `409`，不生成 grant_order/outbox，不扣 BUDGET/USER_LIMIT，写失败审计。
- 多 reward 活动：第二个 reward 库存不足时，第一个 reward 的库存扣减也 rollback，grant_order/outbox 均为空。
- 重复幂等事件：第一次成功后第二次返回 duplicate，budget/stock/user limit 的 used_value 不增加，grant_order/outbox 数量不增加。
- 缺少 BUDGET、USER_LIMIT 或 STOCK quota 行时按无限制处理，兼容现有活动。
- 规则拒绝时只写 rejected participation，不扣 quota，不写 grant_order/outbox。

PostgreSQL smoke 必须执行：

```bash
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
```

建议额外做一个 PostgreSQL 并发测试：同一 activity 预算或库存只剩 1 份时，并发提交两个不同幂等事件，最终只能一个成功，quota used_value 不超过 total_value。

## Score

~~~text
82
~~~

## Decision

~~~text
PASS
~~~

需求：2026-05-24-094535
