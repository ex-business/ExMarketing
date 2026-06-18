# ExMarketing

ExMarketing 是 `ex-stack` 下的营销活动与流程编排系统，用来把事件接入、资格判断、次数频控、奖励、发奖、通知、审计和对账串成可调试、可发布、可运行的生产流程。

它包含两部分：

- **Workflow Studio**：面向运营和研发的活动编排工作台。
- **Execution Engine**：面向生产流量的事件执行、幂等、状态、发奖和审计引擎。

## 界面预览

### 工作区

![Workspace Studio](docs/assets/readme-workspace-home.jpg)

### 流程编排与 Lua 配置

![Workflow Studio And Lua Panel](docs/assets/readme-workflow-lua-panel.png)

## 快速启动

启动 PostgreSQL 并执行迁移：

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
```

启动后端：

```bash
go run ./cmd/exmarketing
```

启动前端 Studio：

```bash
npm install
npm run dev:admin-flow
```

访问地址：

```text
Backend: http://localhost:8080
Studio:  http://127.0.0.1:8081/admin-flow/
```

常用检查：

```bash
npm run typecheck:admin-flow
npm run build:admin-flow
```

停止本地数据库：

```bash
docker compose down
```

没有配置 `EX_MARKETING_DATABASE_URL` 时，后端会使用内存仓储，只适合快速本地验证。

## 核心能力

| 能力 | 说明 | 进一步阅读 |
| --- | --- | --- |
| 事件入口 | OpenAPI、Webhook、MQ Topic、定时事件、人工触发、状态监听 | [事件输入节点设计](docs/event-input-node-design.zh-CN.md) |
| 流程编排 | 工作区、文件夹、模板、画布、节点属性、调试运行、版本管理 | [Workflow Studio 完善计划](docs/workflow-studio-improvement-plan.zh-CN.md) |
| 节点体系 | 入口、标准化、幂等、事实查询、资格校验、频控、奖励、发奖、通知、审计 | [Workflow 节点分类清单](docs/workflow-node-catalog.zh-CN.md)、[Fact Query 事实查询节点](docs/fact-query-node.zh-CN.md)、[Qualify 资格校验节点](docs/qualify-node.zh-CN.md) |
| Lua 配置 | 固定配置与节点级 Lua 脚本同步，支持资源引用、上下文取值和动态默认值 | [字段级表达式与输入解析设计](docs/field-bindings-design.zh-CN.md) |
| 资源管理 | 公共内存、数据库 KV、公共表、Redis、MQ、Provider Adapter、权限范围 | [公共资源管理设计](docs/resource-management-design.zh-CN.md) |
| 活动模板 | 签到、存款、奖励、风控、资格链路等活动模板覆盖 | [业务活动模板](docs/business-activity-templates.md) |
| 生产执行 | PostgreSQL、幂等收件箱、quota、budget、outbox、DLQ、回调签名、审计 | [架构说明](docs/architecture.md) |

## Studio 使用路径

1. 进入 `http://127.0.0.1:8081/admin-flow/`，选择工作区和文件夹。
2. 新建流程、从模板开始，或导入 JSON。
3. 添加事件入口节点，再串联幂等、事实查询、资格、频控、奖励、发奖、通知和审计节点。
4. 在右侧属性面板配置节点字段；复杂字段可切换到 Lua 脚本。
5. 用“调试运行”填写 payload，查看每个节点的输入、输出、错误和 trace。
6. 检查版本和清单，通过后发布，生产事件再从 OpenAPI / MQ / Webhook 进入 execution。

## 标准事件

生产事件进入系统时统一成标准事件结构：

```text
eventId, eventType, sourceSystem, tenantId, subject, biz, occurredAt, attributes
```

最小签到事件示例：

```json
{
  "eventId": "evt_checkin_20260614_0001",
  "eventType": "CHECKIN_SUCCESS",
  "sourceSystem": "app",
  "tenantId": "site_001",
  "subject": { "type": "USER", "id": "u_demo_001" },
  "biz": { "bizType": "CHECKIN", "bizId": "daily_20260614_u_demo_001" },
  "occurredAt": "2026-06-14T13:10:00Z",
  "attributes": {
    "channel": "APP",
    "checkin": true,
    "timezone": "Asia/Shanghai",
    "streakDays": 1
  }
}
```

完整 API 示例见 [API 文档](docs/api.md)。

## 常用接口

```text
GET  /healthz
GET  /admin/
GET  /admin/v1/activities
POST /admin/v1/activities
POST /admin/v1/simulate
POST /openapi/v1/events
POST /openapi/v1/facts/query
POST /openapi/v1/reward/callback
```

## 文档入口

| 主题 | 文档 |
| --- | --- |
| 架构 | [Architecture](docs/architecture.md) |
| API | [API 文档](docs/api.md) |
| 数据模型 | [数据模型](docs/data-model.md) |
| 节点契约 | [Core Node Contract](docs/core-node-contract.md) |
| 节点分类 | [Workflow 节点分类清单](docs/workflow-node-catalog.zh-CN.md) |
| 资格校验 | [Qualify 资格校验节点](docs/qualify-node.zh-CN.md) |
| 活动覆盖 | [节点对活动模板覆盖评估](docs/workflow-node-activity-coverage.zh-CN.md) |
| Lua 配置 / 动态值 | [字段级表达式与输入解析设计](docs/field-bindings-design.zh-CN.md) |
| 资源管理 | [公共资源管理设计](docs/resource-management-design.zh-CN.md) |
| 工作台 UI | [Workspace Home UI 执行文档](docs/workspace-home-ui-execution-plan.zh-CN.md) |
| UI 标准 | [TailAdmin UI 标准](docs/tailadmin-ui-standard.zh-CN.md) |
| 活动模板 | [业务活动模板](docs/business-activity-templates.md) |
| ActivityPlugin | [Lifecycle](docs/activityplugin-lifecycle.md) / [Red Envelope & Lottery](docs/activityplugin-red-envelope-lottery.md) |
| 运维 | [Operations](docs/operations.md) |
| 安全 | [Threat Model](docs/security-threat-model.md) / [Release Gate](docs/security-release-gate.md) |
| 告警 | [Alert Runbook](docs/runbooks/exmarketing-alerts.md) / [Alert Drill](docs/runbooks/exmarketing-alert-drill.md) |
| Roadmap | [Roadmap](docs/roadmap.md) |

## 目录结构

```text
cmd/                    服务入口、迁移、备份恢复、E2E 工具
frontend/admin-flow/     Workflow Studio 前端源码
web/admin-flow/          Workflow Studio 构建产物
internal/engine/         事件执行与 workflow executor
internal/workflow/       workflow 定义和服务
internal/httpapi/        HTTP API
internal/marketing/      活动领域模型
internal/reward/         奖励计算和发奖相关逻辑
internal/worker/         outbox worker、重试和 DLQ
internal/postgres/       PostgreSQL 仓储
docs/                   设计文档和运维手册
```

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
