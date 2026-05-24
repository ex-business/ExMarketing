# Implementation

## 需求

2026-05-24-094535 预算库存次数原子扣减

## 结果

已按 `task.md`、`analysis.md`、`review.md` 在 review PASS 后完成开发。

核心实现：

- 新增 activity 级原子发奖仓储接口 `CreateParticipationGrantOutboxesWithQuota`。
- `REWARDABLE` participation、BUDGET 扣减、STOCK 扣减、USER_LIMIT 扣减、grant_order 写入、outbox 写入现在在同一个事务内完成。
- quota 扣减失败时事务回滚，不生成 grant_order/outbox，不保留 `REWARDABLE` participation。
- STOCK quota 固定使用 `quotaType=STOCK`、`scope=ACTIVITY`、`scopeKey=<rewardCode>`，每个 enabled reward 默认扣减 `1`。
- 缺少 BUDGET、STOCK 或 USER_LIMIT quota 行仍表示不限制，保持兼容。
- quota 不足返回结构化 `QuotaExceededError`，HTTP 继续识别为 `409 Conflict`，message 包含 quota type/scope/scopeKey。
- quota 不足时写 `QUOTA_EXCEEDED` 审计记录，审计在失败事务外写入，不随主事务回滚；审计 payload 包含 `quotaType`、`scope`、`scopeKey`、`amount`、`reason` 和原始错误。
- HTTP `409 Conflict` 响应体增加稳定 `code=QUOTA_EXCEEDED` 以及 `quotaType/scope/scopeKey` 字段，调用方可区分 BUDGET、STOCK、USER_LIMIT。
- 内存仓储与 PostgreSQL 仓储实现同一 all-or-nothing 语义。

## 主要改动

- `internal/marketing/model.go`
  - 增加 `QuotaReservation`、`AtomicGrantCommand`、`AtomicGrantResult`、`QuotaExceededError`。
- `internal/marketing/repository.go`
  - 用 activity 级原子接口替换旧的 grant/outbox quota 接口。
- `internal/engine/service.go`
  - 移除事务外提前写 `REWARDABLE` participation。
  - 组装 budget/user/stock reservation 后一次性提交原子发奖事务。
  - quota 失败时写 `QUOTA_EXCEEDED` audit。
- `internal/postgres/repository.go`
  - 在 PostgreSQL transaction 内插入 participation、条件扣减 quota、写 grant_order/outbox。
  - 任一 quota 不足返回 `QuotaExceededError` 并回滚。
- `internal/marketing/memory_repository.go`
  - 使用 quota slice copy 先试扣，全部成功后才替换状态并写 participation/grant/outbox。
- `internal/httpapi/handler_test.go`
  - 增加 budget 不足返回 409 且写审计的 HTTP 行为测试。
  - 增加 USER_LIMIT 不足返回 409 且写结构化审计的 HTTP 行为测试。
  - 增加 STOCK 不足返回 409、不返回 rewardable/grant/outbox 结果且写结构化审计的 HTTP 行为测试。
- `internal/marketing/memory_repository_test.go`
  - 增加内存仓储多 reward 库存不足 rollback 测试。
  - 增加重复 inbox 不重复扣减测试。
- `internal/postgres/repository_integration_test.go`
  - 增加 PostgreSQL 集成测试，验证第二个 reward 库存不足时 participation/grant/outbox/quota 全部回滚。
- `docs/architecture.md`
  - 同步 activity 级原子事务、STOCK scopeKey 和失败审计。
- `docs/api.md`
  - 同步 STOCK quota 创建示例、处理流程、结构化 409 body 和 `QUOTA_EXCEEDED` audit payload。
- `docs/data-model.md`
  - 同步 quota 约定、participation 事务语义和 `QUOTA_EXCEEDED` audit action。

## 验证

已执行：

```bash
go test ./...
go build ./...
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
```

结果均通过。

本地 PostgreSQL 容器已在运行，`001_marketing_core.sql` 已由 compose 初始化过；本轮重新执行了幂等的 `002_audit_fact_outbox_indexes.sql`，随后带数据库环境变量的测试通过，其中 `internal/postgres` 集成测试实际连接本地 PostgreSQL 执行。

## 边界说明

- 本次保持 analysis 定义的单 activity 事务边界，没有扩大为跨 activity 全局事务。
- 规则拒绝仍只写 `REJECTED` participation，不扣 quota，不写 grant_order/outbox。
- 未新增 schema 字段，未引入外部业务系统依赖。
