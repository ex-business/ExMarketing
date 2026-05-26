# Test Plan

## Agent 指令

你是 Test Agent。只能在 Dev Agent 完成实现后执行本计划中的验证。测试前先确认 `review.md` 的 Decision 为 `PASS`，再按下列用例补充或执行测试。

本计划覆盖任务 `2026-05-24-113929-5`：Outbox 和发奖失败重试策略，包含最大重试次数、退避间隔、下一次执行时间、错误信息、可重试/不可重试流转，以及 PostgreSQL 仓储语义。

## Test Strategy

- 优先使用 PostgreSQL 仓储验证事务、锁、quota 和 outbox claim 语义；内存仓储测试只作为快速单元补充。
- 单元测试覆盖 worker 错误分类、retry backoff、状态同步和幂等保护。
- 集成测试覆盖 API/仓储/worker/callback 闭环，必须检查 `marketing_outbox`、`marketing_grant_order`、`marketing_quota_reservation`、`marketing_quota` 和 audit 记录。
- 并发测试必须验证同一 outbox 在多 worker 下只能被一个 worker claim，且并发终结只移动一次 quota。
- 所有失败错误信息和审计 payload 必须验证不包含 `Authorization`、`token`、broker password、完整 secret。

## Test Cases

### 1. 正常路径

1. 创建有 quota 的活动并提交一条命中奖励事件。
2. 断言同一事务生成 participation、quota reservation、grant_order、outbox。
3. worker claim `PENDING` outbox 后，断言 outbox 进入 `SENDING`，关联 grant_order 进入 `SENDING`。
4. HTTP/Webhook 2xx 或 MQ ack 成功后，断言 outbox 进入 `SENT`，grant_order 保持 `SENDING`，不得直接变为 `SUCCESS`。
5. 调用 reward callback `SUCCESS`，断言 grant_order 进入 `SUCCESS`，quota reservation 从 `FROZEN` 变为确认态，quota frozen 减少、used 增加。
6. 重复相同 `SUCCESS` callback，断言返回 duplicate 或等价幂等结果，不重复移动 quota。

### 2. 可重试失败与退避

1. 模拟网络超时、连接失败、HTTP `429`、HTTP `5xx`、MQ 临时不可用或 ack timeout。
2. 断言 outbox 从 `SENDING` 进入 `FAILED`，`retry_count` 从 0 增为 1，`next_retry_at` 晚于当前处理时间，`error_message` 有稳定分类且脱敏。
3. 断言关联 grant_order 从 `SENDING` 进入 `RETRY`，quota reservation 保持 `FROZEN`。
4. 在 `next_retry_at` 未到期时执行 worker claim，断言该 `FAILED` outbox 不会被提前领取。
5. 将时间推进到 `next_retry_at <= now` 后再次执行 worker claim，断言该 `FAILED` outbox 能被领取并进入 `SENDING`，grant_order 从 `RETRY` 回到 `SENDING`。
6. 再次模拟可重试失败，断言 retry 次数继续累加，下一次退避遵循默认指数退避：1m、2m、4m、8m、16m，且单次不超过 30m。

### 3. 最大重试次数与 DLQ

1. 对同一 outbox 连续触发可重试失败直到达到默认最大自动重试次数 `5`。
2. 再次失败时断言 outbox 进入 `DLQ`，不再设置新的自动执行时间，不再被自动 worker 领取。
3. 断言关联 grant_order 进入或保持 `RETRY` 人工处理态，不得自动 `SUCCESS/FAILED/CLOSED`。
4. 断言 quota reservation 仍为 `FROZEN`，participation 不进入最终失败态。
5. 通过 DLQ 查询断言能看到该记录、retry 次数、错误信息和时间线。

### 4. 不可重试失败

1. 分别模拟 HTTP `400/401/403/404/409/422`、payload schema 错误、非法 targetRef、租户或 commandId 不匹配、外部明确业务拒绝。
2. 断言 outbox 直接进入 `DLQ`，不继续自动重试。
3. 断言 grant_order 进入或保持 `RETRY`，quota reservation 保持 `FROZEN`。
4. 断言错误信息可定位原因但不泄露凭据。

### 5. DLQ replay 与 close

1. 对 `DLQ` outbox 执行 replay，断言只允许 grant_order 为 `RETRY` 的记录 replay。
2. replay 成功后断言同一 outbox 回到 `PENDING`，payload 不变，不创建新的 grant_order 或 outbox，grant_order 保持 `RETRY` 直到 worker claim。
3. replay 后 worker claim 时，断言 outbox 进入 `SENDING`，grant_order 进入 `SENDING`。
4. 对 `DLQ` outbox 执行 close，必须带 `operator` 和 `reason`。
5. close 成功后断言 outbox 进入 `DLQ_CLOSED`，grant_order 进入 `CLOSED`，quota reservation 释放，participation 进入失败终态，并写审计。
6. 对 `DLQ_CLOSED` 再 replay，断言被拒绝。
7. 对已 `SUCCESS/FAILED/CLOSED` 的 grant_order 执行 replay/close/callback，断言返回 duplicate 或机器可识别冲突，不覆盖终态。

### 6. 额度不足

1. 创建 quota 小于奖励金额或库存不足的活动。
2. 提交命中奖励事件，断言请求返回明确额度不足/库存不足错误。
3. 断言不得生成 grant_order、outbox 或 quota reservation。
4. 断言 quota used/frozen 不变，并写入可追踪失败审计。
5. 重复提交同一幂等事件，断言仍不生成脏数据，不重复扣减。

### 7. 幂等重复

1. 用同一 `Idempotency-Key` 重复提交完全相同事件。
2. 断言第二次返回 duplicate 或等价幂等结果，且 participation、grant_order、outbox、quota reservation 均只有一份。
3. 用相同 `Idempotency-Key` 提交不同 payload，断言返回 `INVALID_IDEMPOTENCY_KEY` 或等价错误，不新增记录。
4. 对同一最终态 callback 重复提交，断言不重复确认或释放 quota。
5. 对相反终态 callback，例如先 `SUCCESS` 后 `FAILED`，断言返回冲突，不覆盖已成功状态。

### 8. 并发

1. 并发提交多条命中同一 quota 的事件，累计金额超过总额度。
2. 断言成功数量不超过 quota，失败请求不生成 grant_order/outbox/reservation 脏写。
3. 并发启动多个 worker claim 同一批到期 `PENDING/FAILED` outbox。
4. 断言同一 outbox 同一时刻只被一个 worker claim 和投递，不出现重复发送计数。
5. 并发执行 callback `SUCCESS` 与 callback `FAILED`，或 callback 与 DLQ close/timeout release。
6. 断言只有一个终结动作成功移动 quota，其他动作返回 duplicate 或冲突，grant_order 终态稳定。

### 9. PostgreSQL smoke

1. 使用 Docker PostgreSQL 启动真实数据库，并设置 `EX_MARKETING_DATABASE_URL`。
2. 跑全量 Go 测试，确保 PostgreSQL 相关测试不被跳过。
3. 至少执行一个端到端 smoke：创建活动和 quota -> 提交事件 -> 生成 outbox -> worker 成功发送 -> callback `SUCCESS` -> 查询对账。
4. 至少执行一个失败 smoke：提交事件 -> worker 可重试失败 -> 到期重试 -> 重试耗尽进入 DLQ -> replay -> close 或 callback 失败 -> quota 正确释放。
5. 直接查询 PostgreSQL 表确认关键字段：`send_status`、`retry_count`、`next_retry_at`、`error_message`、`grant_status`、reservation 状态、quota used/frozen。

## Required Commands

以下命令清单由 Test Agent 在 Dev 完成后执行；Test Plan Agent 不执行。

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go test ./...
```

如实现提供了更窄的包级测试，可先执行以下定向命令，再执行全量：

```bash
go test ./internal/worker ./internal/postgres ./internal/api ./internal/engine
go test -race ./internal/worker ./internal/postgres
```

PostgreSQL smoke 查询建议：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT send_status, retry_count, next_retry_at, error_message FROM marketing_outbox ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT grant_status, retry_count, error_message FROM marketing_grant_order ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT reservation_status, amount FROM marketing_quota_reservation ORDER BY created_at DESC LIMIT 10;"
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "SELECT quota_type, total_value, used_value, frozen_value, version FROM marketing_quota ORDER BY created_at DESC LIMIT 10;"
```

## Pass Criteria

- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 全部有通过记录。
- 可重试错误进入 `FAILED` + grant_order `RETRY`，到期后能重新 claim。
- 不可重试错误或重试耗尽进入 outbox `DLQ`，grant_order 保持 `RETRY`，quota 不提前释放。
- DLQ replay/close 行为与 quota、participation、audit 同步正确。
- callback `SUCCESS/FAILED`、重复 callback、冲突 callback 都不造成 quota 重复移动。
- PostgreSQL 测试通过，不能只以内存仓储结果作为验收依据。

## Decision

```text
PASS
```

需求：2026-05-24-113929-5
