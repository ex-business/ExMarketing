# ExMarketing

ExMarketing 是 `ex-stack` 下的独立营销活动与流程编排系统，目标是把“活动配置、事件接入、资格判断、频控、发奖、通知、审计、对账”做成可复用、可调试、可运行的生产编排能力。

当前重点能力：

- 标准事件接入：OpenAPI、Webhook、MQ Topic、定时事件、人工触发、状态监听。
- Workflow Studio：工作区、文件夹、模板、画布编排、节点配置、调试运行、运行历史、版本管理。
- 节点体系：入口、标准化、幂等、事实查询、资格校验、次数频控、奖励、预算锁定、发奖订单、通知、审计、对账等。
- 字段绑定：节点配置值支持固定值、表达式和 Lua 脚本模式。
- Lua 配置：节点变量、Context / Resource / Node API、Lua 标准库补全、语法检查和运行 trace。
- 资源管理：公共内存、数据库 KV、公共表、Provider Adapter、Redis、MQ、Lua Resource API。
- 生产执行：PostgreSQL 真库、幂等收件箱、quota / budget / outbox / DLQ、回调签名、审计日志。

## Screenshots

### Workspace Studio

![Workspace Studio](docs/assets/readme-workspace-home.jpg)

### Workflow Studio And Lua Panel

![Workflow Studio And Lua Panel](docs/assets/readme-workflow-lua-panel.png)

## Quick Start

启动 PostgreSQL：

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
```

启动后端服务：

```bash
go run ./cmd/exmarketing
```

默认地址：

```text
http://localhost:8080
```

启动 Workflow Studio 前端：

```bash
npm install
npm run dev:admin-flow
```

默认前端地址：

```text
http://127.0.0.1:8081/admin-flow/
```

生产构建：

```bash
npm run typecheck:admin-flow
npm run build:admin-flow
```

停止本地 PostgreSQL：

```bash
docker compose down
```

如果没有配置 `EX_MARKETING_DATABASE_URL`，服务会使用内存仓储，适合快速本地验证，但不适合 E2E 和持久化调试。

## Studio Workflow

典型操作路径：

1. 进入 `http://127.0.0.1:8081/admin-flow/`，选择或创建工作区。
2. 在工作区中新建流程、从模板开始，或导入 JSON。
3. 在画布添加入口节点，例如标准事件入口、HTTP 接口事件、MQ Topic 事件、定时事件。
4. 串联幂等收件箱、事实查询、资格校验、次数频控、奖励、发奖订单、通知和审计节点。
5. 在右侧属性面板配置节点字段，必要时切换到 Lua 脚本。
6. 用“调试运行”填写 JSON payload，查看每个节点输入、输出、错误、重试和 trace。
7. 检查版本、发布流程，再通过 OpenAPI / MQ / Webhook 进入真实 execution。

每日签到奖励并通知、风控资格校验链路、次数限制和奖励对账等流程可以从模板或 `hello` 工作区继续演进。模板设计见 [业务活动模板](docs/business-activity-templates.md)。

## Node Model

节点按能力分类，而不是把所有入口或查询都压成一个节点：

| 分类 | 说明 | 代表节点 |
| --- | --- | --- |
| `EVENT_INPUT` | 事件接入与触发 | 标准事件入口、HTTP 接口事件、Webhook 映射事件、MQ Topic 事件、定时事件 |
| `CORE` | 活动核心判断链路 | 事件标准化、事实查询、资格校验、次数频控、奖励计算 |
| `STORAGE` | 状态、幂等、参与记录和资源状态 | 幂等收件箱、活动状态、通用存储、参与记录、资源锁 |
| `ADVANCED` | 高级编排与外部集成 | 条件分支、组合、拆分、代码执行、HTTP 发奖、MQ 发奖、Webhook 通知 |
| `OPS` | 运维、审计、补偿和对账 | 审计记录、失败重试、死信队列、奖励对账、告警通知 |

完整节点清单见 [Workflow 节点分类清单](docs/workflow-node-catalog.zh-CN.md)，活动覆盖评估见 [节点对活动模板覆盖评估](docs/workflow-node-activity-coverage.zh-CN.md)。

## Field Bindings And Lua

每个节点的配置字段都可以逐步支持三种模式：

- 固定值：直接填写业务配置。
- 表达式：从上下文、节点输出、资源引用中取值。
- Lua 脚本：用节点固定变量名计算字段值，不需要强制 `return`。

Lua 编辑器提供：

- 当前节点默认变量补全。
- `Context`、`Resource`、`ResourceRef`、`Node` API 补全。
- Lua 标准库、字符串、数组、table、math、os 等方法补全。
- 语法检查和错误行列提示。
- 调试运行 trace 中展示 resolved input / output。

设计细节见 [字段级表达式与输入解析设计](docs/field-bindings-design.zh-CN.md)。

## Resource Management

公共资源不直接散落在节点里，而是在资源管理中定义，再由节点引用。当前资源方向包括：

- 工作流上下文资源。
- 内存资源。
- 数据库 KV。
- 公共表。
- Redis。
- MQ Topic。
- HTTP / gRPC / Custom Provider Adapter。
- Lua Resource API。

资源可以按 workspace、folder、workflow 等范围授权使用。详细设计见 [公共资源管理设计](docs/resource-management-design.zh-CN.md)。

## Event Intake

标准事件字段：

```text
eventId, eventType, sourceSystem, tenantId, subject, biz, occurredAt, attributes
```

OpenAPI 事件入口：

```bash
curl -X POST http://localhost:8080/openapi/v1/events \
  -H 'content-type: application/json' \
  -H 'Authorization: Bearer dev-token' \
  -H 'Idempotency-Key: site_001:app:CHECKIN_SUCCESS:CHECKIN:daily_20260614_u_demo_001' \
  -d '{
    "eventId": "evt_checkin_20260614_0001",
    "eventType": "CHECKIN_SUCCESS",
    "sourceSystem": "app",
    "tenantId": "site_001",
    "subject": {"type": "USER", "id": "u_demo_001"},
    "biz": {"bizType": "CHECKIN", "bizId": "daily_20260614_u_demo_001"},
    "occurredAt": "2026-06-14T13:10:00Z",
    "attributes": {
      "channel": "APP",
      "checkin": true,
      "timezone": "Asia/Shanghai",
      "streakDays": 1
    }
  }'
```

生产环境建议开启 OpenAPI 事件鉴权：

```bash
export EX_MARKETING_REQUIRE_OPENAPI_AUTH=true
export EX_MARKETING_OPENAPI_TOKENS='dev-token:site_001:app'
```

事件入口设计见 [事件输入节点设计](docs/event-input-node-design.zh-CN.md)，分阶段计划见 [事件输入实施计划](docs/event-input-implementation-plan.zh-CN.md)。

## API Surface

常用接口：

```text
GET  /healthz
GET  /admin/
GET  /admin/v1/activities
POST /admin/v1/activities
POST /admin/v1/quotas
GET  /admin/v1/audit
GET  /admin/v1/reward/reconciliation
POST /admin/v1/simulate
GET  /admin/v1/workflow/requirements
GET  /admin/v1/workflow/requirements/{id}
POST /openapi/v1/events
POST /openapi/v1/facts/query
POST /openapi/v1/reward/callback
```

完整 API 示例见 [API 文档](docs/api.md)。

## Operations

迁移发布：

```bash
go run ./cmd/db-migration-release --mode precheck
go run ./cmd/db-migration-release --mode apply
go run ./cmd/db-migration-release --mode drift
```

PostgreSQL 备份与恢复演练：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-backup-restore --mode backup --output-dir backups/db-restore-drills

export EX_MARKETING_RESTORE_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing_restore?sslmode=disable'
go run ./cmd/db-backup-restore --mode restore --backup-file backups/db-restore-drills/<backup>.dump
go run ./cmd/db-backup-restore --mode validate
go run ./cmd/db-backup-restore --mode smoke
```

Outbox worker：

```bash
export EX_MARKETING_OUTBOX_WORKER=true
```

MQ sender runtime config：

```bash
export EX_MARKETING_MQ_SENDERS_JSON='{"providers":{"kafka":{"enabled":true,"endpoints":["kafka-1:9092"],"authRef":"secret/exmarketing/kafka","sendTimeoutMillis":5000,"destinationAllowlist":["reward.commands"]}}}'
```

运维细节见 [运维手册](docs/operations.md)，告警处理见 [Alert Runbook](docs/runbooks/exmarketing-alerts.md)，发布安全闸门见 [Security Release Gate](docs/security-release-gate.md)。

## Repository Layout

```text
cmd/exmarketing                  service entrypoint
cmd/db-migration-release          PostgreSQL migration release command
cmd/db-backup-restore             backup / restore drill command
cmd/engine-e2e-postgres           PostgreSQL E2E runner
frontend/admin-flow               Workflow Studio frontend
web/admin-flow                    built Workflow Studio assets
internal/httpapi                  HTTP handlers and routing
internal/engine                   event processing pipeline
internal/workflow                 workflow definition and executor
internal/marketing                domain model and repository contracts
internal/rule                     condition rule evaluator
internal/reward                   reward calculator and reward schema validation
internal/decimal                  decimal money helper, no float64 for amount math
internal/worker                   outbox HTTP/Webhook/MQ sender with retry and DLQ
internal/fact                     fact provider client
internal/plugin                   activity plugin registry contract
internal/postgres                 PostgreSQL repository
internal/redislua                 Redis / Lua integration helpers
db/migration                      PostgreSQL schema migrations
docs                              design notes and runbooks
```

## Documentation Index

| 主题 | 文档 |
| --- | --- |
| 架构 | [Architecture](docs/architecture.md) |
| API | [API 文档](docs/api.md) |
| 数据模型 | [数据模型](docs/data-model.md) |
| Workflow Studio 完善计划 | [中文计划](docs/workflow-studio-improvement-plan.zh-CN.md) / [English plan](docs/workflow-studio-improvement-plan.md) |
| 工作台首页 UI | [Workspace Home UI 执行文档](docs/workspace-home-ui-execution-plan.zh-CN.md) |
| UI 标准 | [TailAdmin UI 标准](docs/tailadmin-ui-standard.zh-CN.md) |
| 节点契约 | [Core Node Contract](docs/core-node-contract.md) |
| 节点分类 | [Workflow 节点分类清单](docs/workflow-node-catalog.zh-CN.md) |
| 活动模板覆盖 | [Workflow 节点对活动模板覆盖评估](docs/workflow-node-activity-coverage.zh-CN.md) |
| 事件入口 | [事件输入节点设计](docs/event-input-node-design.zh-CN.md) |
| 事件入口实施 | [事件输入实施计划](docs/event-input-implementation-plan.zh-CN.md) |
| 字段绑定 / Lua | [字段级表达式与输入解析设计](docs/field-bindings-design.zh-CN.md) |
| 资源管理 | [公共资源管理设计](docs/resource-management-design.zh-CN.md) |
| 活动模板 | [业务活动模板](docs/business-activity-templates.md) |
| ActivityPlugin 生命周期 | [ActivityPlugin Lifecycle](docs/activityplugin-lifecycle.md) |
| 插件样例 | [Red Envelope / Lottery Plugins](docs/activityplugin-red-envelope-lottery.md) |
| 自动化需求流水线 | [Automation](docs/automation.md) |
| 任务流水线 | [Workflow](docs/workflow.md) |
| Roadmap | [Roadmap](docs/roadmap.md) |
| 安全威胁模型 | [Security Threat Model](docs/security-threat-model.md) |
| 发布安全闸门 | [Security Release Gate](docs/security-release-gate.md) |
| 运维 | [Operations](docs/operations.md) |
| 告警 | [Alert Runbook](docs/runbooks/exmarketing-alerts.md) |
| 告警演练 | [Alert Drill](docs/runbooks/exmarketing-alert-drill.md) |

## Agent Workflow

```bash
go run ./cmd/workflowctl add --title "example" --description "需求描述"
go run ./cmd/workflowctl intake
go run ./cmd/workflowctl list
EX_MARKETING_AGENT_CMD='/Applications/Codex.app/Contents/Resources/codex exec --cd . --sandbox danger-full-access --ask-for-approval never -' \
  go run ./cmd/workflowctl run-agent --id <requirement-id>
go run ./cmd/workflowctl agent-status --id <requirement-id>
```

任务流水线说明见 [Workflow](docs/workflow.md)。
