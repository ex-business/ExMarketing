# ExMarketing 开发指南

## 项目定位

`ExMarketing` 是独立活动营销中台的 Go 实现，设计依据：

```text
/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md
```

系统必须保持与具体业务系统解耦：

- 禁止连接外部业务数据库。
- 禁止引用外部业务代码包。
- 禁止直接修改用户余额。
- 只接收标准事件，只判断活动配置，只生成参与记录、发奖单和发奖指令。
- 外部业务字段只能通过 `attributes`、`facts`、`mapping`、`adapter` 进入规则引擎。

## 标准开发流程

修改行为前必须先读：

1. `README.md`
2. `docs/architecture.md`
3. `docs/workflow.md`
4. 本次要改的 package
5. 涉及业务语义时，读取 `act-analysis/22-独立活动营销中台设计.md`

开发原则：

- 优先沿用现有分层，不要随意新增包边界。
- 行为变更必须同步更新测试和文档。
- 涉及金额、预算、库存、发奖的改动必须按账务安全规则处理。

完成任何代码改动前必须执行：

```bash
go test ./...
go build ./...
```

涉及数据库 schema 变更时：

```bash
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/<migration>.sql
```

然后必须用 PostgreSQL 仓储验证对应 API，不允许只用内存仓储验证。

## 本地运行

启动本地 PostgreSQL：

```bash
docker compose up -d postgres
```

使用 PostgreSQL 运行服务：

```bash
go run ./cmd/exmarketing
```

默认 DSN 是 `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。需要连接其他数据库时再设置 `EX_MARKETING_DATABASE_URL`。

开启 Outbox 投递：

```bash
export EX_MARKETING_OUTBOX_WORKER=true
```

Fact Query 必须显式配置 allowlist：

```bash
export EX_MARKETING_FACT_ALLOWLIST='example.com,api.partner.com'
```

服务默认使用 PostgreSQL 持久化。只有显式设置 `EX_MARKETING_REPOSITORY=memory` 时才会使用内存仓储；内存仓储只允许用于隔离测试，不允许承载业务数据。

## 包边界

```text
cmd/exmarketing       服务入口和运行时组装
internal/httpapi      HTTP API 边界
internal/engine       活动匹配、规则判断、奖励计算、额度/发奖/Outbox 编排
internal/marketing    领域模型和仓储接口
internal/postgres     PostgreSQL 仓储实现
internal/rule         配置化条件规则执行器
internal/reward       奖励计算器
internal/decimal      金额 Decimal 计算
internal/worker       Outbox 投递、重试、DLQ
internal/fact         HTTP Fact Provider 客户端
internal/plugin       活动插件接口
web/admin             最小本地后台页面
db/migration          PostgreSQL schema migration
```

边界规则：

- `internal/httpapi` 不写业务决策。
- `internal/engine` 负责流程顺序和账务安全。
- `internal/postgres` 负责事务边界和行级并发控制。
- `internal/decimal` 必须用于金额计算。
- 禁止用 `float64` 计算金额、预算、库存、奖励。

## 账务安全规则

以下规则是硬性要求：

- 必须先创建 participation，再创建 grant_order。
- 禁止绕过 `grant_order` 和 `outbox` 直接发奖。
- Quota 扣减、grant_order 创建、outbox 创建必须在活动级别保持原子性。
- PostgreSQL Outbox worker 必须先 claim 行，再投递，避免多 worker 重复发送。
- 发奖回调必须同时按 `tenantId` 和 `commandId` 限定更新范围。
- 外部事件幂等键必须使用：

```text
tenantId + sourceSystem + eventType + bizType + bizId
```

禁止引入读后写式额度扣减。必须使用 PostgreSQL 条件更新、行锁，或 Redis Lua 等原子方案。

## 安全规则

不得削弱 Admin 和 OpenAPI 的安全边界：

- Fact Query 必须要求 allowlist host。
- Fact Query 必须拒绝 private/local IP。
- Fact Query 禁止跟随 redirect。
- Fact Query HTTP client 禁止使用代理。
- 禁止记录 secret、token、完整 Authorization header。
- 未设计鉴权前，禁止新增面向生产的无鉴权管理操作。

如果某个接口只允许本地开发使用，必须在 `README.md` 中写清楚，并在生产环境禁用或加保护。

## 数据库迁移规则

所有 schema 变更都放在 `db/migration`。

迁移要求：

- 本地迭代优先使用 `CREATE TABLE IF NOT EXISTS` 和 `CREATE INDEX IF NOT EXISTS`。
- 核心表名必须与设计文档保持一致：
  - `marketing_activity`
  - `marketing_rule`
  - `marketing_reward`
  - `marketing_event_inbox`
  - `marketing_participation`
  - `marketing_grant_order`
  - `marketing_quota`
  - `marketing_outbox`
  - `marketing_audit`
  - `marketing_fact_provider`
- Go model 字段变化时，必须同步更新 migration、Postgres insert/scan、测试和文档。

## API 开发规则

当前 API：

```text
GET  /healthz
GET  /admin/
GET  /admin/v1/activities
POST /admin/v1/activities
POST /admin/v1/quotas
GET  /admin/v1/audit
POST /admin/v1/simulate
POST /openapi/v1/events
POST /openapi/v1/facts/query
POST /openapi/v1/reward/callback
```

新增 API 时：

- JSON decode 默认使用 `DisallowUnknownFields`。
- 事件和 Fact payload 需要使用 `UseNumber`。
- Quota 不足返回 `409 Conflict`，不要返回通用 `500`。
- 模拟运行必须无副作用。
- OpenAPI 事件接入必须保持幂等。

## 测试要求

每次改动最低要求：

```bash
go test ./...
go build ./...
```

以下模块变更必须加或跑针对性测试：

- 规则引擎：条件操作符、字段缺失、数值比较。
- 奖励计算：固定金额、比例金额、封顶金额。
- Quota 和发奖：额度不足、多奖励原子性、重复事件。
- Outbox worker：claim、retry、sent、DLQ。
- Fact Query：allowlist 拒绝、private IP 拒绝、redirect 拒绝。
- Callback：tenant 维度隔离更新。

涉及 PostgreSQL 行为时，必须用 Docker PostgreSQL 验证，因为内存仓储不能代表事务和锁语义。

## 任务流水线

复杂任务必须建立任务目录：

```text
tasks/YYYY-MM-DD-short-topic/
```

并按以下顺序产出：

```text
task.md
analysis.md
review.md
implementation.md
test.md
commit.md
```

模板位于：

```text
tasks/_template/
```

完整流程见：

```text
docs/workflow.md
```

需求列表自动化见：

```text
docs/automation.md
requirements/backlog.json
cmd/workflowctl
```

## 提交和推送流程

执行 `git commit`、`git push` 或 PR 前，必须走提交前 review 流程：

1. 执行 `go test ./...` 和 `go build ./...`。
2. 查看 `git status -sb`，确认本次提交范围。
3. 开独立 code review。
4. 如存在 P0/P1，必须先修复，禁止提交。
5. review 结论为 `PASS` 后才能提交。

提交信息保持简短明确，例如：

```text
Initial ExMarketing Go core
Add activity quota reservation
Harden fact provider requests
```

## 当前远端

```text
origin: https://github.com/ex-business/ExMarketing.git
default branch: main
```
