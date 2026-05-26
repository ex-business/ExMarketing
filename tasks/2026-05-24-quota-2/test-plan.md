# Test Plan

## Agent 指令

你是后续 Test Agent。请只按本文件设计补充/执行测试，不改变 `task.md`、`analysis.md`、`review.md` 的需求口径。

本计划基于：

- `tasks/2026-05-24-quota-2/task.md`
- `tasks/2026-05-24-quota-2/analysis.md`
- `tasks/2026-05-24-quota-2/review.md`

Review Decision 已为 `PASS`。本阶段只设计测试用例和命令清单；Test Plan Agent 未执行任何测试、构建、格式化或 git 命令。

## Test Strategy

核心验收目标：

- 事件发奖阶段只做 quota freeze：配置 quota 时创建 `FROZEN` reservation，增加 `frozen_value`，不增加 `used_value`。
- `SUCCESS` callback 将应确认的 reservation 从 `FROZEN` 变为 `CONFIRMED`，并把对应值从 `frozen_value` 转入 `used_value`。
- `FAILED` callback 和 TIMEOUT 补偿将应释放的 reservation 从 `FROZEN` 变为 `RELEASED`，减少 `frozen_value`，不增加 `used_value`。
- `BUDGET` / `STOCK` 按 grant_order 粒度冻结、确认、释放；`USER_LIMIT` / `DAILY_LIMIT` 按 participation 粒度只冻结一次，并按聚合结果只确认或释放一次。
- 不配置 quota 的活动按不限额兼容：不创建 reservation，callback 正常终结，不返回 `QUOTA_RESERVATION_MISSING`。
- quota 不足、聚合冲突、reservation 缺失或并发抢占失败时，不留下 `REWARDABLE` participation、grant_order、outbox 或部分 reservation 脏写。
- 重复 callback、重复补偿、相反最终态和 callback/补偿并发不会重复确认、重复释放或覆盖已终结状态。
- PostgreSQL 仓储必须覆盖事务、并发、唯一约束和 smoke；内存仓储测试只能作为快速单元补充。

必须覆盖范围：

- 正常路径：freeze -> success confirm、freeze -> failed release、freeze -> timeout release。
- 额度不足：`BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`，以及多 quota all-or-nothing。
- 幂等重复：重复事件、重复 `SUCCESS`、重复 `FAILED`、重复 timeout compensation。
- 并发：并发事件抢最后额度、并发 callback、callback 与补偿竞态。
- PostgreSQL smoke：真实 PostgreSQL 上的 repository integration、全量 Go tests 和 SQL 一致性抽查。

## Test Cases

### 1. Normal Path - Freeze Then Confirm

建议位置：`internal/postgres/repository_integration_test.go`，必要时补 `internal/engine/service_test.go` 的内存仓储快速测试。

步骤：

1. 创建 activity。
2. 创建 `BUDGET/ACTIVITY/TOTAL total=100`、`STOCK/ACTIVITY/REWARD_A total=10`、`USER_LIMIT/USER/user_1 total=1`、`DAILY_LIMIT/USER/user_1:UTC_DATE total=1`。
3. 处理一个满足规则、奖励金额为 `20.0000` 的事件。
4. 断言事件处理成功后：
   - participation 为 `REWARDABLE`。
   - grant_order 为 `PENDING` 或当前实现的未终结发奖状态。
   - outbox 为 `PENDING`。
   - budget quota `used_value=0`、`frozen_value=20`。
   - stock quota `used_value=0`、`frozen_value=1`。
   - user limit quota `used_value=0`、`frozen_value=1`。
   - daily limit quota `used_value=0`、`frozen_value=1`。
   - reservation 均为 `FROZEN`，且 tenant、activity、commandId、grantOrderId、quotaType、scope、scopeKey、amount 正确。
5. 对同一 `tenantId + commandId` 发 `SUCCESS` callback，携带 `externalGrantId`。
6. 断言 callback 后：
   - grant_order 为 `SUCCESS`。
   - `BUDGET` / `STOCK` grant_order 级 reservation 为 `CONFIRMED`。
   - `USER_LIMIT` / `DAILY_LIMIT` participation 级 reservation 为 `CONFIRMED`。
   - 各 quota `frozen_value` 减少到 0，`used_value` 增加对应值。
   - `confirmed_at` 或等价审计字段非空，`released_at` 为空。

### 2. Normal Path - Freeze Then Failed Release

步骤：

1. 复用正常发奖准备数据，先让事件成功 freeze quota。
2. 对对应 `tenantId + commandId` 发 `FAILED` callback。
3. 断言：
   - grant_order 为 `FAILED`。
   - 相关 reservation 为 `RELEASED`。
   - quota `frozen_value` 减少到 freeze 前水平。
   - quota `used_value` 不增加。
   - `released_at` 或等价审计字段非空，`confirmed_at` 为空。

### 3. Normal Path - Timeout Compensation Release

步骤：

1. 创建已冻结但未终结的 grant_order，`created_at` 或 `updated_at` 早于默认阈值 2 小时。
2. 调用 Admin/Operations 超时补偿入口，参数包含 `tenantId`、`limit`、`dryRun=true`、`operator`、`reason`。
3. 断言 dry-run 返回候选记录，不修改 grant_order、reservation 或 quota。
4. 再调用 `dryRun=false`。
5. 断言：
   - 候选 grant_order 进入失败或关闭类最终态。
   - grant_order 级 reservation 被释放。
   - 若该 participation 下所有 grant_order 均失败或超时，`USER_LIMIT` / `DAILY_LIMIT` reservation 释放一次。
   - quota `frozen_value` 下降，`used_value` 不增加。
   - 审计原因可识别为 `TIMEOUT_RELEASE`，并记录 `operator`、`reason`、处理数量、跳过数量。
6. 创建未超过阈值的 `FROZEN` reservation，断言默认 2 小时补偿不会选中。
7. 使用 `limit=1` 创建多个候选，断言单次最多处理 1 条，并返回剩余可继续处理的语义或数量。

### 4. Quota Insufficient - No Dirty Writes

覆盖 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。

用例：

1. `BUDGET total=10`，事件奖励金额 `20`，预期 `ErrQuotaExceeded` 或 HTTP `409 QUOTA_EXCEEDED`。
2. `STOCK total=0`，实际权益 reward 需要扣库存，预期 quota exceeded。
3. `USER_LIMIT total=1`，同一用户第二次 rewardable 事件预期 quota exceeded。
4. `DAILY_LIMIT total=1`，同一活动同一活动时区自然日内第二次 rewardable 事件预期 quota exceeded。

每个失败场景断言：

- 不存在本次事件的 `REWARDABLE` participation。
- 不存在本次事件的 grant_order。
- 不存在本次事件的 outbox。
- 不存在本次事件的 quota reservation。
- 相关 quota 的 `used_value`、`frozen_value`、`version` 没有因失败请求漂移。
- 如系统写入 `QUOTA_EXCEEDED` audit，audit payload 包含 `activityId`、`eventInboxId`、`quotaType`、`scope`、`scopeKey`、`amount`。

### 5. Multi-Quota Atomicity

步骤：

1. 一个事件同时需要扣 `BUDGET=20`、`STOCK REWARD_A=1`、`STOCK REWARD_B=1`、`USER_LIMIT=1`、`DAILY_LIMIT=1`。
2. 设置 `REWARD_B` stock total 为 0，让最后一个 quota 失败。
3. 断言整个事务回滚：
   - budget、stock A、user limit、daily limit 都没有 frozen/used 增量。
   - 没有 grant_order/outbox。
   - 没有 reservation。
4. 再制造 grant_order 唯一键冲突或 outbox 插入失败，断言 quota freeze 同样回滚。

### 6. Multi Grant Order - Participation Quota Once

步骤：

1. 同一 participation 下生成两个 grant_order：`REWARD_A` 和 `REWARD_B`。
2. 断言 `BUDGET` / `STOCK` 为每个 grant_order 分别创建 reservation。
3. 断言 `USER_LIMIT` / `DAILY_LIMIT` 对该 participation 只创建一组 reservation，冻结值只增加 1。
4. 场景 A：`REWARD_A` `SUCCESS`，`REWARD_B` `FAILED`。
   - `REWARD_A` 的 budget/stock 确认。
   - `REWARD_B` 的 budget/stock 释放。
   - participation 级 user/daily limit 只确认一次，不因失败 grant_order 再释放。
5. 场景 B：两个 grant_order 都 `FAILED` 或都被 timeout release。
   - 两个 grant_order 级 reservation 都释放。
   - participation 级 user/daily limit 只释放一次。

### 7. Idempotent Duplicate Events

步骤：

1. 用相同 `tenantId:sourceSystem:eventType:bizType:bizId` 和完全相同 payload 连续提交同一事件两次。
2. 第一次成功 freeze quota，第二次返回 `duplicate=true` 或等价语义。
3. 断言第二次不会新增 participation、grant_order、outbox、quota reservation。
4. 断言 quota `used_value`、`frozen_value`、`version` 与第一次提交后完全一致。
5. 用相同 Idempotency-Key 但不同 payload 再提交一次，预期 `ErrIdempotencyConflict` 或等价 409/400，且不新增任何发奖副作用。

### 8. Idempotent Duplicate Callbacks

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
   - 返回 `409 CALLBACK_STATUS_CONFLICT` 或等价机器可识别冲突。
   - reservation 和 quota 不再变化。
4. `SUCCESS` 重复 callback 携带不同 `externalGrantId`：
   - 返回 callback conflict。
   - quota 不再变化。
5. `SUCCESS` 缺少 `externalGrantId`：
   - 返回 `400 INVALID_CALLBACK_REQUEST` 或等价错误。
   - grant_order、reservation、quota 不变化。

### 9. Idempotent Duplicate Timeout Compensation

步骤：

1. 准备一条超过阈值的 `FROZEN` reservation。
2. 第一次实际补偿释放成功。
3. 第二次用相同条件再次触发补偿。
4. 断言第二次返回 0 processed、already-final、skipped 或等价语义。
5. 断言 grant_order、reservation 和 quota 聚合完全不变。

### 10. Callback Tenant Isolation

步骤：

1. tenant A 和 tenant B 各创建 grant_order，故意使用相同 `commandId`。
2. 对 tenant A 发送 callback。
3. 断言只更新 tenant A 的 grant_order、reservation 和 quota。
4. tenant B 的 grant_order、reservation 和 quota 完全不变。
5. 用不存在的 `tenantId + commandId` callback，预期 `GRANT_ORDER_NOT_FOUND` 或等价 not found，不得跨租户命中任何 order。

### 11. Timeout Tenant Isolation

步骤：

1. tenant A 和 tenant B 各创建超过阈值的 `FROZEN` reservation。
2. 调用补偿入口，仅传 tenant A。
3. 断言只释放 tenant A 的候选记录。
4. tenant B 的 grant_order、reservation、quota 完全不变。
5. 查询补偿审计，断言 tenant 字段可区分。

### 12. Unlimited Activity Compatibility

步骤：

1. 创建完全未配置 quota 的 activity。
2. 处理可发奖事件。
3. 断言 participation、grant_order、outbox 创建成功，但没有 reservation，quota 表不变化。
4. 对 grant_order 发送 `SUCCESS` callback，断言正常终结，不返回 `QUOTA_RESERVATION_MISSING`。
5. 重复创建另一个未配置 quota 的 grant_order 并发送 `FAILED` callback，断言正常终结，不移动 quota。
6. 后续为同一 activity 新增 quota 配置后，不倒推要求历史 grant_order 必须有 reservation。

### 13. Missing Reservation Safety

步骤：

1. 创建一个能证明发奖创建时应有 quota reservation 的 grant_order，例如 activity 当时存在 quota 配置或 grant_order 保存了 expected reservation 标记。
2. 人为构造缺失 reservation 的状态，或通过 repository 测试夹具插入 grant_order 而不插入 reservation。
3. 对该 grant_order 发送 `SUCCESS` 或 `FAILED` callback。
4. 断言返回 `409 QUOTA_RESERVATION_MISSING` 或进入对账异常。
5. 断言系统不猜测确认或释放金额，quota 聚合不变化。
6. 对无法证明历史创建时配置过 quota 的旧 grant_order，断言按不限额兼容终结，并留下对账标记。

### 14. Aggregate Conflict Safety

步骤：

1. 构造 `frozen_value` 小于 reservation amount 的 quota 聚合异常。
2. 对 reservation 执行 confirm 或 release。
3. 断言返回 `409 QUOTA_AGGREGATE_CONFLICT` 或等价错误。
4. 断言不得出现 `frozen_value < 0`、`used_value < 0`、`used_value > total_value`。
5. 断言 grant_order 和 reservation 不被标记为最终成功状态，现场保留可对账。

### 15. Concurrency - Budget Does Not Overspend

PostgreSQL 必测。

步骤：

1. 创建同一 activity 的 `BUDGET total=100`。
2. 并发 50 个 goroutine，每个事件奖励金额 `10`，使用不同 Idempotency-Key 和不同用户。
3. 所有 goroutine 同时起跑。
4. 断言：
   - 成功事件数量最多 10。
   - quota `used_value + frozen_value <= 100`。
   - 成功事件对应 participation/grant_order/outbox/reservation 数量一致。
   - 失败事件没有 grant_order/outbox/reservation。
   - 没有 500 类错误；失败应为 quota exceeded 或明确业务冲突。

### 16. Concurrency - Stock Last Item

PostgreSQL 必测。

步骤：

1. 创建 `STOCK/ACTIVITY/REWARD_A total=1`。
2. 并发 20 个满足条件的事件抢同一个 rewardCode。
3. 断言：
   - 只有 1 个事件成功生成 grant_order/outbox/reservation。
   - stock quota `used_value + frozen_value = 1`。
   - 其余事件 quota exceeded，且无脏写。

### 17. Concurrency - Callback Same Final State

PostgreSQL 必测。

步骤：

1. 创建一条已 freeze 的 grant_order。
2. 并发 20 个 goroutine 对同一 `tenantId + commandId` 发送相同 `SUCCESS` callback。
3. 断言：
   - 只有一个请求完成实际 confirm。
   - 其他请求返回 duplicate/already-final 或等价语义。
   - quota 只从 frozen 转 used 一次。
   - reservation 只有一个终结时间，不出现重复审计导致聚合漂移。
4. 用 `FAILED` callback 重复同样测试，断言只释放一次。

### 18. Concurrency - Callback Versus Timeout

PostgreSQL 必测。

步骤：

1. 创建一条超过阈值且仍为 `FROZEN` 的 grant_order。
2. 并发触发 `SUCCESS` callback 和 timeout compensation。
3. 断言只有一个结果生效：
   - 若 callback 先成功，reservation `CONFIRMED`，quota 转入 `used_value`，补偿返回 skipped/already-final/conflict，不释放。
   - 若补偿先成功，reservation `RELEASED`，quota 不增加 `used_value`，callback 返回 conflict/already-final，不重新确认。
4. 重复多轮，确认不会出现 frozen 负数、used 超总额或 grant_order 状态反复变化。

### 19. Simulation Has No Side Effects

步骤：

1. 创建 quota 接近耗尽的 activity。
2. 调用 simulate，覆盖 wouldExceed=false、wouldExceed=true 和 unlimited=true 三种情况。
3. 断言 simulate 返回 quota impact preview。
4. 断言 simulate 不写 participation、grant_order、outbox、quota reservation，也不改变 `marketing_quota`。

### 20. Decimal Boundary

步骤：

1. `BUDGET total=0.3000`，连续或并发处理 `0.1000`、`0.2000`。
2. 断言可刚好占满。
3. 再提交 `0.0001`，预期 quota exceeded。
4. 断言没有 float 误差导致超过或误拒。

### 21. PostgreSQL Smoke

PostgreSQL smoke 必须至少执行一次完整链路：

1. 使用 `docker compose up -d postgres` 启动本地 PostgreSQL。
2. 设置 `EX_MARKETING_DATABASE_URL=postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。
3. 跑 repository integration tests。
4. 跑全量 Go tests。
5. 对测试生成的 activity 前缀执行 SQL 抽查：
   - `marketing_quota` 不存在 `used_value + frozen_value > total_value`。
   - `marketing_quota` 不存在 `used_value < 0` 或 `frozen_value < 0`。
   - `marketing_quota_reservation` 不存在孤儿 grant_order。
   - `marketing_quota_reservation` 不存在同一 `tenant_id, grant_order_id, quota_type, scope, scope_key` 重复行。
   - `FROZEN` reservation 的总额与对应 quota `frozen_value` 可对账。
   - `CONFIRMED` reservation 的总额与对应 quota `used_value` 可对账。
   - `RELEASED` reservation 不增加 `used_value`。

## Required Commands

后续 Test Agent 执行命令清单如下。Test Plan Agent 本轮不得执行这些命令。

```bash
go test ./...
```

```bash
go build ./...
```

```bash
docker compose up -d postgres
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Quota|TestPostgres.*Reservation|TestPostgres.*Callback|TestPostgres.*Timeout|TestPostgres.*Concurrent|TestPostgresAtomic' -count=1 -v
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
```

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go build ./...
```

PostgreSQL smoke SQL：

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT COUNT(*) FROM marketing_quota WHERE used_value + frozen_value > total_value OR used_value < 0 OR frozen_value < 0;"
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT tenant_id, grant_order_id, quota_type, scope, scope_key, COUNT(*) FROM marketing_quota_reservation GROUP BY tenant_id, grant_order_id, quota_type, scope, scope_key HAVING COUNT(*) > 1;"
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT COUNT(*) FROM marketing_quota_reservation r LEFT JOIN marketing_grant_order g ON g.id = r.grant_order_id WHERE g.id IS NULL;"
```

```bash
psql 'postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' -c "SELECT reservation_status, COUNT(*), COALESCE(SUM(reserved_amount), 0) FROM marketing_quota_reservation GROUP BY reservation_status ORDER BY reservation_status;"
```

清理命令由 Test Agent 根据本地环境决定：

```bash
docker compose down
```

## Data / Smoke

建议测试数据前缀：

- `quota2_normal_confirm_`
- `quota2_normal_release_`
- `quota2_timeout_release_`
- `quota2_insufficient_`
- `quota2_multi_atomic_`
- `quota2_multi_grant_`
- `quota2_idem_event_`
- `quota2_idem_callback_`
- `quota2_idem_timeout_`
- `quota2_tenant_iso_`
- `quota2_unlimited_`
- `quota2_missing_reservation_`
- `quota2_aggregate_conflict_`
- `quota2_concurrent_budget_`
- `quota2_concurrent_stock_`
- `quota2_concurrent_callback_`
- `quota2_concurrent_timeout_`
- `quota2_simulate_`
- `quota2_decimal_`

最小活动配置：

- `tenantId=site_001`
- `activityType=FIRST_DEPOSIT`
- `triggerEvents=["DEPOSIT_SUCCESS"]`
- `timezone=UTC`；daily limit 专项用 `Asia/Shanghai` 或其他非 UTC 时区补充。
- reward A：金额类 `BONUS/RATIO` 或 `BONUS/FIXED`，`rewardCode=REWARD_A`。
- reward B：多 grant_order 和多 quota 原子性测试专用，`rewardCode=REWARD_B`。

最小事件字段：

- `eventType=DEPOSIT_SUCCESS`
- `sourceSystem=payment`
- `tenantId=site_001`
- `subject.type=USER`
- `subject.id` 每个并发请求唯一，用户限制专项复用同一用户。
- `biz.bizType=DEPOSIT`
- `biz.bizId` 每个非重复请求唯一。
- `attributes.amount` 使用 Decimal 字符串。

超时补偿最小请求字段：

- `tenantId=site_001`
- `olderThan` 或默认阈值 2 小时。
- `limit` 使用 `1` 和大于候选数两种。
- `dryRun=true/false` 都必须覆盖。
- `operator=qa_operator`
- `reason=qa timeout release smoke`

必须保留的 smoke 断言 SQL：

```sql
SELECT COUNT(*)
FROM marketing_quota
WHERE used_value + frozen_value > total_value
   OR used_value < 0
   OR frozen_value < 0;
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

## PASS / BLOCK Criteria

PASS 条件：

- 后续 Dev Agent 可根据本计划补齐自动化测试。
- 后续 Test Agent 可按命令清单执行基础验证、PostgreSQL integration 和 smoke SQL。
- 测试计划覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke。
- 测试计划明确超时补偿、次数类 quota 粒度、不限额兼容、缺失 reservation、聚合冲突和租户隔离断言。

BLOCK 条件：

- `review.md` 不是 `PASS`。
- 测试计划缺少正常路径、额度不足、幂等重复、并发或 PostgreSQL smoke 任一必需覆盖项。
- 命令清单无法让 Test Agent 进行最低要求的 `go test ./...`、`go build ./...` 和 PostgreSQL smoke。

## Decision

```text
PASS
```

理由：`review.md` 已 PASS，且本计划覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，并补充 quota-2 关键风险：超时补偿、participation 级次数 quota、不限额兼容、缺失 reservation、聚合冲突和租户隔离。

需求：2026-05-24-quota-2
