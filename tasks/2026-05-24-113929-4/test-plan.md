# Test Plan

## Agent 指令

你是后续 Test Agent。请在 Dev Agent 完成实现后，按本文设计测试用例并执行命令。本文只定义测试计划，不代表当前已经执行测试。

本计划基于 `task.md`、`analysis.md`、`review.md`。`review.md` Decision 已为 `PASS`，可以进入测试设计阶段。

## Test Strategy

覆盖目标：

- 三类奖励计算：`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
- 配置校验：非法概率、非法随机区间、非法奖池拆分、金额类缺少 `currency`、可执行发奖缺少合法 `targetRef` 均必须阻断。
- 正常路径：活动命中后计算奖励，并继续进入 participation、quota、grant_order、outbox 标准链路。
- 额度不足：quota / budget / 奖励额度不足时不得生成 grant_order/outbox，不得扣减额度，返回可解释失败原因。
- 幂等重复：同一幂等事件重复处理时，随机 / 概率结果稳定，不重复 participation、不重复 grant_order/outbox、不重复扣减 quota。
- 并发：同一幂等键并发、同一活动不同幂等键并发、额度临界并发都不能产生重复发奖或额度超扣。
- PostgreSQL smoke：真实 PG repository + migration 场景下验证 activity、participation、quota/grant_order/outbox 持久化一致性。

建议测试层级：

- `internal/reward/calculator_test.go`：纯计算和 Decimal 精度。
- `internal/marketing/prelaunch_validation_test.go`：上线 / 可执行配置阻断。
- `internal/engine/service_test.go`：活动命中、规则通过、幂等、quota、participation、grant_order、outbox 链路。
- `internal/httpapi/handler_test.go`：Admin/API 层错误码与结构化错误。
- `internal/postgres/repository_integration_test.go`：PostgreSQL smoke 和唯一约束 / 事务一致性。

## Test Cases

### 1. 配置校验

- `PROBABILITY` 权重总和等于 `100.0000`：允许创建 / 更新为可执行状态。
- `PROBABILITY` 权重总和为 `99.9999`、`100.0001`、`0`：必须阻断，错误信息包含 probability / weight sum / 100 等可定位信息。
- `PROBABILITY.items` 为空：必须阻断。
- `PROBABILITY` item 缺少 `itemCode`、`weight`、`amount`：分别必须阻断。
- `PROBABILITY` itemCode 重复：必须阻断。
- `PROBABILITY` item `weight < 0`：必须阻断。
- `PROBABILITY` 全部 item `weight = 0`：必须阻断。
- `RANDOM` 缺少 `minAmount` / `maxAmount` / `step`：分别必须阻断。
- `RANDOM minAmount > maxAmount`：必须阻断。
- `RANDOM step <= 0` 或 Decimal 非法：必须阻断。
- `POOL_SPLIT` 缺少 `poolAmount` / `splitBy`：分别必须阻断。
- `POOL_SPLIT splitBy` 不是 `COUNT` / `WEIGHT`：必须阻断。
- `POOL_SPLIT COUNT` 缺少 `countField`：必须阻断。
- `POOL_SPLIT WEIGHT` 缺少 `weightField` / `totalWeightField`：分别必须阻断。
- 金额类 `BONUS` / `CASH` 缺少 `currency`：必须阻断。
- `AUTO + HTTP/WEBHOOK` 缺少 `targetRef` 或 `targetRef` 非 http/https URL：必须阻断。
- 未知 `calculateType`：必须阻断，不得回退到 `FIXED`。

### 2. RANDOM 正常路径

- `minAmount=1.00`、`maxAmount=88.00`、`step=0.01`：结果必须在闭区间内，且符合 `minAmount + n * step` 离散集合。
- 输出金额统一保留 4 位小数。
- 同一 `tenantId + activityId/activityCode + rewardCode + idempotencyKey + version + seedSalt` 重复计算：金额完全一致。
- 不同 `idempotencyKey`：允许得到不同金额，但仍满足范围和 step。
- `minAmount = maxAmount`：输出固定金额，且 snapshot / audit context 中 `calculateType=RANDOM`。
- `step` 大于区间长度：只允许返回 `minAmount`，不得超过 `maxAmount`。
- snapshot / audit context 至少包含 configVersion、seed 摘要、minAmount、maxAmount、step、最终金额。

### 3. PROBABILITY 正常路径

- 合法配置：`80.0000 / 19.0000 / 1.0000` 权重总和为 `100.0000`，按稳定 seed 命中一个 item。
- 重复幂等事件：命中同一个 `itemCode`，金额一致。
- 命中金额 item：输出该 item 金额，保留 4 位小数，并生成标准 grant_order/outbox。
- 命中 `amount=0.0000` 空奖 item：不生成 grant_order/outbox，不扣 quota，但 participation snapshot / 模拟结果记录命中 `itemCode`、权重总和、抽取摘要、空奖金额。
- `weight=0` 的 item：允许保存，但在多 seed 抽样中不得被命中。
- 调整 item 顺序：权重总和校验仍通过；snapshot 必须能解释抽取区间或命中 item。

### 4. POOL_SPLIT 正常路径

- `COUNT`：`poolAmount=1000.00`、`attributes.poolParticipantCount=4`，输出 `250.0000`。
- `COUNT`：`poolAmount=0`、`count=4`，输出 `0.0000`，不生成实际 grant_order/outbox。
- `WEIGHT`：`poolAmount=1000.00`、`attributes.userPoolWeight=2`、`attributes.poolTotalWeight=10`，输出 `200.0000`。
- `WEIGHT`：`weight=0`、`totalWeight=10`，输出 `0.0000`，不生成实际 grant_order/outbox。
- 输出金额统一保留 4 位小数。
- snapshot / audit context 至少包含 poolAmount、splitBy、运行时 count 或 weight/totalWeight、最终金额。

### 5. POOL_SPLIT 运行时失败

- `COUNT` 的 count 字段缺失：本次计算失败，不生成 grant_order/outbox，不扣 quota。
- `COUNT` 的 count 不可解析：本次计算失败。
- `COUNT` 的 count 为 `0` 或负数：本次计算失败。
- `WEIGHT` 的 weight 字段缺失或不可解析：本次计算失败。
- `WEIGHT` 的 totalWeight 字段缺失或不可解析：本次计算失败。
- `WEIGHT weight < 0`：本次计算失败。
- `WEIGHT totalWeight <= 0`：本次计算失败。
- `WEIGHT weight > totalWeight`：本次计算失败。
- 所有失败场景都必须保留可解释错误，不得返回通用 `500`。

### 6. 额度不足

- quota 剩余额度小于 RANDOM 计算金额：不得生成 grant_order/outbox，不得扣减 quota，participation 或处理结果记录额度不足。
- quota 剩余额度小于 PROBABILITY 命中金额 item：不得生成 grant_order/outbox，不得扣减 quota。
- quota 剩余额度小于 POOL_SPLIT 计算金额：不得生成 grant_order/outbox，不得扣减 quota。
- quota 剩余额度等于计算金额：允许通过，扣减后剩余额度为 `0.0000`。
- 空奖或 `0.0000` 结果：不得因为额度为 0 被误判为额度不足，应按空奖 / 无实际发奖语义处理。

### 7. 幂等重复

- 同一标准事件重复提交两次：只保留一条 participation 或等价幂等记录。
- RANDOM 重放：金额和 seed 摘要一致，不重复扣 quota，不重复生成 grant_order/outbox。
- PROBABILITY 重放：命中 item 和金额一致，不重复扣 quota，不重复生成 grant_order/outbox。
- POOL_SPLIT 重放：金额一致，不重复扣 quota，不重复生成 grant_order/outbox。
- 配置 version 变化后新事件允许产生新随机 / 概率结果；旧 participation snapshot 保留旧 version 上下文。

### 8. 并发

- 20 个 goroutine 同时处理相同幂等键的 RANDOM 事件：最终只有一次成功发奖链路，所有返回结果一致或明确幂等复用。
- 20 个 goroutine 同时处理相同幂等键的 PROBABILITY 事件：最终只有一次成功发奖链路，命中 item 一致。
- 20 个 goroutine 同时处理相同幂等键的 POOL_SPLIT 事件：最终只有一次成功发奖链路，金额一致。
- 多个不同幂等键并发，活动 quota 刚好只够 N 次发奖：成功数不得超过 N，quota 不得为负，grant_order/outbox 数量与成功扣减数一致。
- 并发额度不足场景中失败请求不得留下半成品 grant_order/outbox。

### 9. PostgreSQL Smoke

- 使用 migration 初始化 PostgreSQL schema。
- 创建一个可执行活动，分别覆盖 RANDOM、PROBABILITY、POOL_SPLIT 配置。
- 插入或通过 API 处理标准事件，验证数据库中 participation snapshot 写入计算摘要。
- 验证成功发奖路径下 grant_order 和 outbox 被持久化，金额为 4 位小数。
- 验证空奖 / `0.0000` 路径下有 participation snapshot，但没有 grant_order/outbox。
- 验证幂等重复事件在 PostgreSQL 唯一约束 / 事务下不会重复写 grant_order/outbox。
- 验证额度临界并发下 PostgreSQL 中 quota / grant_order / outbox 数量一致。

### 10. 回归

- `FIXED`、`RATIO`、`LADDER` 既有单元测试继续通过。
- 标准事件字段边界仍然生效，不允许直接读取非标准顶层业务字段。
- 奖励计算仍只通过 participation、quota、grant_order、outbox 链路，不新增直接发奖旁路。
- outbox worker 既有 retry / DLQ 行为不因新增计算类型回归。

## Required Commands

后续 Test Agent 应按需执行以下命令。当前 Test Plan Agent 不执行这些命令。

```bash
go test ./internal/reward -run 'Test.*(Random|Probability|Pool|Decimal)' -count=1
go test ./internal/marketing -run 'Test.*(Prelaunch|Validation|Reward)' -count=1
go test ./internal/engine -run 'Test.*(Reward|Idempot|Quota|Concurrent)' -count=1
go test ./internal/httpapi -run 'Test.*(Activity|Reward|Validation)' -count=1
go test ./internal/postgres -run 'Test.*(Reward|Participation|Grant|Outbox|Idempot|Quota)' -count=1
go test ./... -count=1
go build ./...
```

PostgreSQL smoke 如需本地容器：

```bash
docker compose up -d postgres
go test ./internal/postgres -run Test -count=1
docker compose down
```

如项目实际 service 名称不是 `postgres`，Test Agent 应先读取 `docker-compose.yml` 后替换为实际服务名。

## Data / Smoke

建议固定测试数据：

- tenantId：`tenant-test`
- sourceSystem：`test-suite`
- eventType：`reward.qualified`
- bizType：`order`
- rewardCode：`reward-random-001` / `reward-probability-001` / `reward-pool-001`
- activityCode：`activity-random-001` / `activity-probability-001` / `activity-pool-001`
- currency：`CNY`
- targetRef：`https://wallet.example.com/reward`
- RANDOM：`minAmount=1.00`、`maxAmount=88.00`、`step=0.01`、`seedSalt=test-salt`
- PROBABILITY：`SMALL_BONUS 80.0000 amount=2.00`、`BIG_BONUS 19.0000 amount=18.00`、`EMPTY 1.0000 amount=0.00`
- POOL_SPLIT COUNT：`poolAmount=1000.00`、`attributes.poolParticipantCount=4`
- POOL_SPLIT WEIGHT：`poolAmount=1000.00`、`attributes.userPoolWeight=2`、`attributes.poolTotalWeight=10`

PostgreSQL smoke 验证 SQL 方向：

- participation 表存在对应幂等事件和 snapshot / calculation context。
- grant_order 表只在实际金额大于 `0.0000` 且额度充足时新增。
- outbox 表只在实际发奖需要外发时新增。
- 幂等重复后 participation、grant_order、outbox 数量不增加。
- 额度不足后 grant_order、outbox 数量不增加，quota / budget 未被扣成负数。

## Decision

```text
PASS
```

测试设计已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，以及 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 的配置阻断、计算精度和审计快照要求。可以交给后续 Test Agent 在 Dev 完成后实现并执行。

需求：2026-05-24-113929-4
