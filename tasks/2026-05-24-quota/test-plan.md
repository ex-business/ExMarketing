# Test Plan

## Agent 指令

你是后续 Test Agent。请只按本文件设计补充/执行测试，不改变需求口径。需要覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。

本计划基于：

- `tasks/2026-05-24-quota/task.md`
- `tasks/2026-05-24-quota/analysis.md`
- `tasks/2026-05-24-quota/review.md`

Review Decision 已为 `PASS`。

## Test Strategy

核心验收目标：

- 事件发奖阶段只做 quota freeze：`frozen_value` 增加，`used_value` 不增加。
- callback `SUCCESS` 将 reservation 从 `FROZEN` 变为 `CONFIRMED`，并把 `frozen_value` 转入 `used_value`。
- callback `FAILED` 将 reservation 从 `FROZEN` 变为 `RELEASED`，并释放 `frozen_value`，`used_value` 不变。
- 任何时刻 `used_value + frozen_value <= total_value`。
- quota 不足、并发抢占失败、grant_order/outbox 写入失败时，不留下 `REWARDABLE` participation、grant_order、outbox 或 quota reservation 脏写。
- 重复事件和重复 callback 必须幂等，不重复占用 quota，不重复移动聚合值。
- PostgreSQL 仓储必须覆盖并发和事务 smoke；内存仓储测试只能作为快速单元补充。

reservation 最小验收口径：

- 每条 reservation 至少可按 `tenantId + grant_order_id` 定位。
- reservation 行必须包含 `tenant_id`、`activity_id`、`grant_order_id`、`command_id`、`quota_type`、`scope`、`scope_key`、`reserved_amount`、`reservation_status`。
- callback 必须按 `tenantId + commandId` 定位 grant_order，再通过 grant_order 关联 reservation；不能仅按 `commandId` 或仅按 reservation id 更新。
- 同一 grant_order 对同一 `quota_type + scope + scope_key` 不得重复插入 reservation。

## Test Cases

### 1. Normal Path - Freeze Then Confirm

建议位置：`internal/postgres/repository_integration_test.go`，必要时补 `internal/engine/service_test.go` 的内存仓储快速测试。

步骤：

1. 创建 activity。
2. 创建 `BUDGET/ACTIVITY/TOTAL total=100`、`STOCK/ACTIVITY/REWARD_A total=10`、`USER_LIMIT/USER/user_1 total=1`。
3. 处理一个满足规则、奖励金额为 `20.0000` 的事件。
4. 断言事件处理成功后：
   - participation 为 `REWARDABLE`。
   - grant_order 为 `PENDING`。
   - outbox 为 `PENDING`。
   - budget quota `used_value=0`、`frozen_value=20`、`version` 递增。
   - stock quota `used_value=0`、`frozen_value=1`。
   - user limit quota `used_value=0`、`frozen_value=1`。
   - `marketing_quota_reservation` 为每个 grant_order/quota 写入 `FROZEN` reservation，且 amount、tenant、commandId、grantOrderId 正确。
5. 对同一 `tenantId + commandId` 发 `SUCCESS` callback。
6. 断言 callback 后：
   - grant_order 为 `SUCCESS`。
   - reservation 为 `CONFIRMED`，`confirmed_at` 非空，`released_at` 为空。
   - quota 聚合为 `used_value` 增加、`frozen_value` 归零。
   - `version` 再次递增。

### 2. Normal Path - Freeze Then Release

步骤：

1. 复用正常发奖准备数据，先让事件成功 freeze quota。
2. 对对应 `tenantId + commandId` 发 `FAILED` callback。
3. 断言：
   - grant_order 为 `FAILED`。
   - reservation 为 `RELEASED`，`released_at` 非空，`confirmed_at` 为空。
   - quota `frozen_value` 减少到 freeze 前水平。
   - quota `used_value` 不增加。
   - 重复执行同一个 `FAILED` callback 后，quota 和 reservation 不再变化。

### 3. Quota Insufficient - No Dirty Writes

覆盖 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。

用例：

1. `BUDGET total=10`，事件奖励金额 `20`，预期 `ErrQuotaExceeded` 或 HTTP `409 QUOTA_EXCEEDED`。
2. `STOCK total=0`，实际权益 reward 需要扣库存，预期 quota exceeded。
3. `USER_LIMIT total=1`，同一用户第二次 rewardable 事件预期 quota exceeded。
4. `DAILY_LIMIT total=1`，同一活动本地自然日内第二次 rewardable 事件预期 quota exceeded。

每个失败场景断言：

- 不存在本次事件的 `REWARDABLE` participation。
- 不存在本次事件的 grant_order。
- 不存在本次事件的 outbox。
- 不存在本次事件的 quota reservation。
- 相关 quota 的 `used_value`、`frozen_value`、`version` 没有因失败请求漂移。
- 如系统写入 `QUOTA_EXCEEDED` audit，audit payload 包含 `activityId`、`eventInboxId`、`quotaType`、`scope`、`scopeKey`、`amount`。

### 4. Idempotent Duplicate Events

步骤：

1. 用相同 `tenantId:sourceSystem:eventType:bizType:bizId` 和完全相同 payload 连续提交同一事件两次。
2. 第一次成功 freeze quota，第二次返回 `duplicate=true`。
3. 断言第二次不会新增：
   - participation
   - grant_order
   - outbox
   - quota reservation
4. 断言 quota `used_value`、`frozen_value`、`version` 与第一次提交后完全一致。
5. 用相同 Idempotency-Key 但不同 payload 再提交一次，预期 `400 INVALID_IDEMPOTENCY_KEY` 或 repository `ErrIdempotencyConflict`，且不新增任何发奖副作用。

### 5. Idempotent Duplicate Callbacks

用例：

1. 首次 `SUCCESS` 后重复同一个 `SUCCESS` callback：
   - 返回 duplicate/idempotent 语义。
   - grant_order 保持 `SUCCESS`。
   - reservation 保持 `CONFIRMED`。
   - quota `used_value`、`frozen_value`、`version` 不再变化。
2. 首次 `FAILED` 后重复同一个 `FAILED` callback：
   - 返回 duplicate/idempotent 语义。
   - reservation 保持 `RELEASED`。
   - quota 不再变化。
3. `SUCCESS` 后再发 `FAILED`，或 `FAILED` 后再发 `SUCCESS`：
   - 返回 callback conflict。
   - reservation 和 quota 不再变化。
4. `SUCCESS` 重复 callback 携带不同 `externalGrantId`：
   - 返回 callback conflict。
   - quota 不再变化。

### 6. Callback Tenant Isolation

步骤：

1. tenant A 和 tenant B 各创建 grant_order，故意使用相同 `commandId`。
2. 对 tenant A 发送 callback。
3. 断言只更新 tenant A 的 grant_order、reservation 和 quota。
4. tenant B 的 grant_order、reservation 和 quota 完全不变。
5. 用不存在的 `tenantId + commandId` callback，预期 not found / invalid callback，不得跨租户命中任何 order。

### 7. Multi-Quota Atomicity

步骤：

1. 一个事件同时需要扣 `BUDGET=20`、`STOCK REWARD_A=1`、`STOCK REWARD_B=1`、`USER_LIMIT=1`。
2. 设置 `REWARD_B` stock total 为 0，让最后一个 quota 失败。
3. 断言整个 activity 级事务回滚：
   - budget、stock A、user limit 都没有 frozen/used 增量。
   - 没有 grant_order/outbox。
   - 没有 reservation。
4. 再制造 grant_order 唯一键冲突或 outbox 插入失败，断言 quota freeze 同样回滚。

### 8. Concurrency - Budget Does Not Overspend

PostgreSQL 必测。

步骤：

1. 创建同一 activity 的 `BUDGET total=100`。
2. 并发 50 个 goroutine，每个事件奖励金额 `10`，使用不同 Idempotency-Key 和不同用户。
3. 所有 goroutine 同时起跑。
4. 断言：
   - 成功事件数量最多 10。
   - quota `used_value + frozen_value <= 100`。
   - 成功事件对应 grant_order/outbox/reservation 数量一致。
   - 失败事件没有 grant_order/outbox/reservation。
   - 没有 500 类错误；失败应为 quota exceeded 或明确业务冲突。

### 9. Concurrency - Stock Last Item

PostgreSQL 必测。

步骤：

1. 创建 `STOCK/ACTIVITY/REWARD_A total=1`。
2. 并发 20 个满足条件的事件抢同一个 rewardCode。
3. 断言：
   - 只有 1 个事件成功生成 grant_order/outbox/reservation。
   - stock quota `used_value + frozen_value = 1`。
   - 其余事件 quota exceeded，且无脏写。

### 10. PostgreSQL Smoke

PostgreSQL smoke 必须至少执行一次完整链路：

1. 使用 `docker compose up -d postgres` 启动本地 PostgreSQL。
2. 设置 `EX_MARKETING_DATABASE_URL=postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。
3. 跑 repository integration tests。
4. 跑全量 Go tests。
5. 对测试生成的 activity 前缀执行 SQL 抽查：
   - `marketing_quota` 不存在 `used_value + frozen_value > total_value`。
   - `marketing_quota_reservation` 不存在孤儿 grant_order。
   - `marketing_quota_reservation` 不存在同一 `tenant_id, grant_order_id, quota_type, scope, scope_key` 重复行。
   - `CONFIRMED` reservation 的 quota 聚合已转入 `used_value`。
   - `RELEASED` reservation 不增加 `used_value`。

### 11. Simulation Has No Side Effects

步骤：

1. 创建 quota 接近耗尽的 activity。
2. 调用 simulate，覆盖 wouldExceed=false 和 wouldExceed=true 两种情况。
3. 断言 simulate 返回 quotaImpactPreviews。
4. 断言 simulate 不写 participation、grant_order、outbox、quota reservation，也不改变 `marketing_quota`。

### 12. Decimal Boundary

步骤：

1. `BUDGET total=0.3000`，并发或连续处理 `0.1000`、`0.2000`。
2. 断言可刚好占满。
3. 再提交 `0.0001`，预期 quota exceeded。
4. 断言没有 float 误差导致超过或误拒。

## Required Commands

后续 Test Agent 执行命令清单：

```bash
go test ./...
```

```bash
docker compose up -d postgres
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Quota|TestPostgres.*Callback|TestPostgres.*Concurrent|TestPostgresAtomic' -count=1 -v
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
```

可选手工 smoke：

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/exmarketing
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT activity_id, quota_type, scope, scope_key, total_value, used_value, frozen_value, version FROM marketing_quota WHERE used_value + frozen_value > total_value;"
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*) FROM marketing_quota_reservation GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key HAVING COUNT(*) > 1;"
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT r.* FROM marketing_quota_reservation r LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id WHERE g.id IS NULL;"
```

清理命令由 Test Agent 根据本地环境决定：

```bash
docker compose down
```

## Data / Smoke

建议测试数据前缀：

- `quota_normal_confirm_`
- `quota_normal_release_`
- `quota_insufficient_`
- `quota_idem_event_`
- `quota_idem_callback_`
- `quota_tenant_iso_`
- `quota_multi_atomic_`
- `quota_concurrent_budget_`
- `quota_concurrent_stock_`
- `quota_simulate_`
- `quota_decimal_`

最小活动配置：

- `tenantId=site_001`
- `activityType=FIRST_DEPOSIT`
- `triggerEvents=["DEPOSIT_SUCCESS"]`
- `timezone=UTC`，daily limit 专项用 `Asia/Shanghai` 或其他非 UTC 时区补充。
- reward A：金额类 `BONUS/RATIO` 或 `BONUS/FIXED`，`rewardCode=REWARD_A`。
- reward B：多 quota 原子性测试专用，`rewardCode=REWARD_B`。

最小事件字段：

- `eventType=DEPOSIT_SUCCESS`
- `sourceSystem=payment`
- `tenantId=site_001`
- `subject.type=USER`
- `subject.id` 每个并发请求唯一，用户限制专项复用同一用户。
- `biz.bizType=DEPOSIT`
- `biz.bizId` 每个非重复请求唯一。
- `attributes.amount` 使用 Decimal 字符串。

必须保留的 smoke 断言 SQL：

```sql
SELECT COUNT(*)
FROM marketing_quota
WHERE used_value + frozen_value > total_value;
```

期望：`0`。

```sql
SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*)
FROM marketing_quota_reservation
GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key
HAVING COUNT(*) > 1;
```

期望：`0 rows`。

```sql
SELECT COUNT(*)
FROM marketing_quota_reservation r
LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id
WHERE g.id IS NULL;
```

期望：`0`。

## Decision

```text
PASS
```

理由：Review 已 PASS，测试计划覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，并补充了 reservation 关联和 callback 幂等的最小可验收断言。后续可交给 Test Agent 实现与执行。

需求：2026-05-24-quota
