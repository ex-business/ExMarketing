# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- Decimal 基础量化：覆盖 `333.33 -> 333.3300`、`0.00005 -> 0.0001`、`18.88888 -> 18.8889`、`59.800000 -> 59.8000`。
- FIXED 奖励：`amount="18.88888"` 输出 `18.8889`。
- RATIO 比例赠送：`attributes.amount="299.00"`、`rate="0.20"` 输出 `59.8000`，HTTP 事件处理也断言 `rewardAmount":"59.8000"`。
- RATIO 返水：`attributes.rebateBaseAmount="333.33"`、`rate="0.015"` 覆盖稳定 Decimal 计算。
- RATIO tie-breaking：`0.00005` 最终量化为 `0.0001`，锁定四舍五入口径。
- RATIO 封顶/非封顶：覆盖 `maxAmount`、`capped` 解释字段和最终金额 4 位输出。
- LADDER 阶梯奖励：覆盖闭区间命中、未命中和重叠区间拒绝。
- JSON number 兼容：`RewardConfig`、`RewardLadder`、`ProbabilityItem`、`CreateQuotaCommand`、规则条件和事件入口使用 `UseNumber` 或等价路径，避免先转 `float64`。
- Quota / Budget / Stock：覆盖预算金额 Decimal、库存和次数整数 Decimal 校验，零库存/次数立即超限。
- 原子性 / 幂等 / 并发：覆盖 memory repository 与 PostgreSQL 的 quota 不足回滚、重复 inbox、重复 participation/grant idempotency key、并发 quota 竞争。
- PostgreSQL smoke：在真实 PostgreSQL 容器上执行 `TestPostgres*`，覆盖 atomic grant、quota rollback、idempotency 和 JSON number payload。

## Commands

~~~bash
rg -n "float64|float32|strconv\\.ParseFloat|FormatFloat|json\\.NewDecoder|UseNumber" internal cmd
go test ./internal/decimal ./internal/reward ./internal/rule ./internal/marketing ./internal/engine
go test ./...
go build ./...
docker compose up -d postgres
docker compose ps postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/marketing ./internal/postgres -run 'Concurrent|Idempot|Duplicate|Atomic|Quota' -count=10
~~~

## Result

~~~text
PASS
~~~

## Evidence

- 静态扫描：
  - 未发现 `strconv.ParseFloat` 或 `FormatFloat`。
  - `float64` 命中仅见于 `internal/fact/client.go` 的普通 fact 数值类型兼容，以及 `internal/worker/outbox_worker.go` / `_test.go` 的重试 jitter；这些不是金额、预算、库存金额或奖励计算逻辑。
  - 金额相关 JSON number 路径可见 `UseNumber`：`internal/reward/calculator.go`、`internal/rule/evaluator.go`、`internal/marketing/model.go`、`internal/httpapi/handler.go`、`internal/httpapi/webhook.go`、`internal/mqconsumer/consumer.go`、`internal/postgres/repository.go`。
  - 仍存在若干 `json.NewDecoder` 未调用 `UseNumber` 的 HTTP/admin/fact decode 路径；本轮扫描后未发现其直接落入新增金额计算的 `float64` 路径，未作为阻塞项。
- 核心单元测试通过：
  - `ok github.com/ex-stack/ExMarketing/internal/decimal`
  - `ok github.com/ex-stack/ExMarketing/internal/reward`
  - `ok github.com/ex-stack/ExMarketing/internal/rule`
  - `ok github.com/ex-stack/ExMarketing/internal/marketing`
  - `ok github.com/ex-stack/ExMarketing/internal/engine`
- 全量测试通过：`go test ./...` 通过。
- 构建验证通过：`go build ./...` 通过。
- PostgreSQL smoke 环境：
  - `docker compose up -d postgres` 输出 `Container exmarketing-postgres Running`。
  - `docker compose ps postgres` 显示 `exmarketing-postgres` 为 `Up ... (healthy)`，端口 `54329->5432`。
- PostgreSQL 集成测试通过：
  - `EX_MARKETING_DATABASE_URL=... go test ./internal/postgres -run 'TestPostgres' -count=1` 通过。
- 并发/幂等/原子性 10 轮回归通过：
  - `ok github.com/ex-stack/ExMarketing/internal/marketing`
  - `ok github.com/ex-stack/ExMarketing/internal/postgres`
- 未发现测试失败；无需打回 Dev Agent。

需求：2026-05-24-decimal
