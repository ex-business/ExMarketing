# ExMarketing 服务、接口、提供者和资源整体改造方案

本文档说明如何把 ExMarketing 从“节点直接配置资源/Provider/endpoint”的模式，改造成“节点调用 Service Interface，Provider 治理实现，Resource 承载底层能力”的完整模型。

主设计见 [ExMarketing 服务、接口、提供者和资源设计](service-interface-provider-resource-design.zh-CN.md)。本文档只回答实施问题：改哪些模块、按什么顺序改、每阶段做到什么程度、如何验收。

## 改造目标

最终结构：

```text
Workflow Node
  -> Service Interface
  -> Provider Resolver
  -> Service Provider
  -> Resource
  -> Audit / Trace / Metrics
```

核心目标：

1. 新建 workflow 节点不再手填 HTTP URL、Redis 地址、DB 表、MQ broker、secret。
2. 业务能力统一建模为 `Service -> Interface -> Provider`。
3. 底层连接、Store、Topic、Counter、Lock 统一建模为 `Resource`。
4. Provider 层闭环 `PRIMARY`、`fallback`、`weight`、`environment`、`health`、`circuit`、`rateLimit`、`mock`。
5. Interface 契约通过 schema registry 强约束输入、输出和错误结构。
6. Service Interface 调用必须有权限校验、运行 trace、审计和脱敏。
7. 旧 `PROVIDER resourceRef`、`FACT_SERVICE`、`endpoint/providerCode` 进入兼容迁移期，不再作为新配置推荐入口。

## 当前现状

代码里已经有第一层基础，但还不是完整治理闭环。

| 模块 | 当前已有 | 主要缺口 |
| --- | --- | --- |
| DB migration | `db/migration/012_service_assets.sql` 已有 service/interface/provider 基础表 | 缺 schema、binding、audit、health/circuit/rate-limit、JSONB 索引和状态约束 |
| Model | `internal/marketing/service_asset_model.go` 已有 Service/Interface/Provider struct | 缺 schema registry、binding、audit、provider runtime state、兼容迁移模型 |
| Engine service | `internal/engine/service_asset_service.go` 已有保存、列表、PRIMARY resolve | Provider 选择策略只支持 PRIMARY，未闭环权限、fallback、health、circuit、rateLimit、schema |
| Admin API | `internal/httpapi/service_asset_admin.go` 已有基础 CRUD/test | 响应契约过简，缺统一 error/warning/meta、binding check、schema validate、migration API |
| Resource | `internal/marketing/resource_model.go`、`internal/engine/resource_service.go` 已有资源管理 | 需要和 Service Provider 依赖关系、Resource 权限、key policy、审计联动 |
| Executor | `internal/engine/workflow_executor.go` 已有 workflow 执行 | 需要抽出统一 ProviderChainExecutor / ServiceInterfaceInvoker / ResourceOperator |
| Studio | `frontend/admin-flow/src` 已有 workflow UI | 缺能力资产中心、Service/Interface/Provider 选择器、binding 编辑、migration diff |

## 后续代码改造总清单

后续写代码必须按本节推进。任何新增节点、资源、Provider 或 Studio 表单，都要先判断属于哪个改造项，避免继续扩散旧模型。

### A. 数据库和 Repository

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | 补 service schema registry | `db/migration/013_service_schema_registry.sql`、`internal/marketing/service_asset_model.go`、repository | 可保存 input/output/error schema，支持版本、状态、兼容检查 |
| P0 | 补 service interface binding | `db/migration/014_service_bindings.sql`、repository | 可按 workspace/folder/activity/node/nodeType 校验 Interface 调用权限 |
| P0 | 补 provider runtime state | `db/migration/015_service_provider_runtime_state.sql`、repository | 可保存 health/circuit/failure count/last check |
| P0 | 补 service call audit | `db/migration/016_service_call_audit.sql`、repository | service/resource 调用有脱敏审计，audit append-only |
| P1 | 补 rate limit 表 | `marketing_service_rate_limit` | ProviderResolver 可判断限流 |
| P1 | 补 migration audit | `marketing_service_migration_audit` | 旧字段迁移有 before/after diff 和操作者 |
| P1 | 补 JSONB 索引和 check 约束 | service/provider/schema/binding/audit 表 | 发布前 drift/status 能发现缺失索引和非法状态 |
| P1 | 内置资产 seed 化 | `db/migration/018_builtin_service_asset_seed.sql` 或 seed command | 内置 service/interface/provider 不再只靠代码虚拟定义 |

Repository 必须同时支持 PostgreSQL 和 MemoryRepository，测试不能只改一边。

### B. 后端领域模型

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | 增加 `ServiceSchema` | `internal/marketing/service_asset_model.go` | 有 schemaRef/version/type/status/schemaJson/examplesJson |
| P0 | 增加 `ServiceInterfaceBinding` | `internal/marketing/service_asset_model.go` | 有 scope/principal/nodeType/operation/providerPolicy/luaCallable |
| P0 | 增加 `ServiceProviderState` | `internal/marketing/service_asset_model.go` | 有 healthStatus/circuitStatus/failureCount/lastHealthCheckAt |
| P0 | 增加 `ProviderDecision` | `internal/marketing/service_asset_model.go` | 记录 selectedProvider/attempts/fallback reason |
| P0 | 增加 `ProviderChainStep` 共享模型 | `internal/marketing/workflow_model.go` 或独立文件 | 节点配置不再每个 handler 自己解析一套字段 |
| P1 | 增加 `ServiceCallAudit` | `internal/marketing/service_asset_model.go` | 审计字段固定且默认脱敏 |
| P1 | 增加 `MigrationPlan/MigrationItem` | `internal/marketing/workflow_validation_model.go` 或独立文件 | migration analyze/apply 使用结构化 diff |

模型命名必须和文档一致：`Service` 表达能力归属，`Interface` 表达契约，`Provider` 表达实现，`Resource` 表达底层能力。不要再把 Provider Resource 当成新业务入口。

### C. Admin API

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | 统一响应结构 | `internal/httpapi/*` | 新 service/resource API 返回 `requestId/data/warnings/meta` |
| P0 | Schema API | `internal/httpapi/service_asset_admin.go` | CRUD、payload validate、compatibility-check 可用 |
| P0 | Binding API | `internal/httpapi/service_asset_admin.go` | binding CRUD 和 bindings/check 可用 |
| P0 | Provider test API | `internal/httpapi/service_asset_admin.go` | 测试时执行权限、schema、provider resolve、resource access |
| P1 | Health/circuit API | `internal/httpapi/service_asset_admin.go` | health-check 和 circuit reset 有权限和审计 |
| P1 | Migration API | `internal/httpapi/workflow_admin.go` 或独立 handler | analyze/apply 只改 draft，返回 diff/auditId |
| P1 | Resource API 对齐 | `internal/httpapi/resource_admin.go` | resource test/check 返回结构和 service API 一致 |

所有 API 错误必须返回稳定错误码，不允许只返回自然语言字符串。

### D. Provider 治理层

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | 拆出 ProviderResolver | `internal/engine/service_provider_resolver.go` | 不再把 provider 选择逻辑放在 CRUD service |
| P0 | 支持 PRIMARY | ProviderResolver | 按 priority 选择 active provider |
| P0 | 支持 fallback | ProviderResolver | primary 失败能选择 fallback provider |
| P1 | 支持 environment | ProviderResolver | draft/staging/prod 不串用 provider |
| P1 | 支持 weight | ProviderResolver | WEIGHTED 策略可预测、可测试 |
| P1 | 支持 health | `internal/engine/service_provider_state.go` | unhealthy provider 不被正式发布默认选中 |
| P1 | 支持 circuit | ProviderResolver + state | OPEN 跳过，HALF_OPEN 限量探测，reset 有审计 |
| P1 | 支持 rate limit | ProviderResolver | 超限返回 `SERVICE_PROVIDER_RATE_LIMITED` |
| P1 | 支持 mock | ProviderResolver | mock 只允许 preview/test，prod 发布阻断 |

ProviderResolver 输出必须进入运行 trace，不能只返回 provider 对象。

### E. Schema 和 Mapping

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | JSON Schema validator | `internal/engine/schema_validator.go` | 保存、测试、发布、运行时都能校验 |
| P0 | inputMapping 校验 | `internal/engine/service_interface_invoker.go`、validation | required 字段缺失发布失败 |
| P0 | outputMapping/valuePath 校验 | invoker/adapters | 输出不符合 schema 进入错误处理 |
| P1 | errorSchema 校验 | invoker/adapters | provider 错误结构化，不吞错 |
| P1 | schema compatibility-check | API + engine | breaking change 阻断直接覆盖 |
| P1 | mapping builder 元数据 | Admin API | Studio 能根据 schema required 字段生成表单 |

SchemaRef 不能只是字符串引用。找不到、DISABLED、不兼容时必须在保存或发布阶段失败。

### F. Executor 和 Adapter

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | ProviderChainExecutor | `internal/engine/provider_chain_executor.go` | 所有节点走统一 step 执行入口 |
| P0 | ServiceInterfaceInvoker | `internal/engine/service_interface_invoker.go` | 权限、schema、provider resolve、adapter、audit 顺序固定 |
| P0 | ResourceOperator | `internal/engine/resource_operator.go` | RESOURCE step 统一校验 capability/keyPolicy/audit |
| P0 | RESOURCE Provider adapter | `internal/engine/provider_adapter_resource.go` | Service Provider 可依赖 TABLE/REDIS/Store Resource |
| P1 | HTTP Provider adapter | `internal/engine/provider_adapter_http.go` | 只能通过 HTTP Resource/client 和 authRef 调用，不直连裸 URL |
| P1 | MQ Provider adapter | `internal/engine/provider_adapter_mq.go` | PUBLISH/REQUEST_REPLY 通过受控 MQ Resource |
| P1 | Plugin Provider adapter | `internal/engine/provider_adapter_plugin.go` | 只能调用已注册 plugin action |
| P1 | trace/audit writer | `internal/engine/service_call_audit.go` | 每次 attempt 有 trace，payload 默认脱敏 |

节点 handler 只能表达业务语义，不允许在 handler 里直接实现 HTTP、Redis、DB、MQ 连接逻辑。

### G. 节点规范和模板

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | 抽共享 ProviderChainStep schema | `internal/marketing/workflow_node_config_schema.go` | fact-query/activity-state/quota/candidate-reward 不复制字段 |
| P0 | fact-query 新默认 | schema、template、executor | 新模板使用 `SERVICE_INTERFACE user-profile-service.query-user-profile` |
| P0 | activity-state 新默认 | schema、template、executor | 内部 state 或 RESOURCE step，无 redisRef/endpoint |
| P0 | quota-check 新默认 | schema、template、executor | RESOURCE counter/lock step |
| P1 | candidate-reward 新默认 | schema、template、executor | 简单配置读 Resource，复杂奖励走 Service Interface |
| P1 | budget-reserve 新默认 | schema、template、executor | quota Service Interface，旧 reserveProviderRef 兼容 |
| P1 | outbox/http-call/mq-publish 收敛 | schema、executor、worker | 节点表达投递意图，真实协议在 Provider/Resource/worker |
| P1 | code/lua-query 收敛 | `internal/engine/workflow_lua.go` | 只能 service_call/resource_*，不能原始连接 |
| P1 | 旧 query 节点隐藏 | schema + Studio | `http-query/redis-query/grpc-query/plugin-query` 新建入口隐藏，只读兼容 |

当前代码里 `internal/marketing/workflow_validation_service.go` 仍有大量旧字段校验，例如 `endpoint/providerEndpoint/redisRef/grpcTarget/pluginCode`。这些逻辑要改成：

```text
新配置: 严格校验 targetType/serviceRef/interfaceRef/resourceRef
旧配置: 兼容读取 + deprecation warning + migration suggestion
正式发布: 高风险旧字段按策略阻断或要求 break-glass
```

### H. 发布校验和 Release Gate

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | service/interface/provider/resource 校验 | `internal/marketing/prelaunch_validation.go` | 发布前能阻断缺失、禁用、无权限 |
| P0 | 节点 forbidden field 校验 | validation service | 新节点出现 endpoint/redisRef/rawSql 直接失败 |
| P0 | binding check 接入 | validation service | 没有 Interface Binding 不能发布 |
| P0 | schema check 接入 | validation service | input/output schema 缺失或不匹配不能发布 |
| P1 | provider health/circuit check | validation service | prod 发布不能选 unhealthy/open circuit 且无 fallback |
| P1 | break-glass | release gate | 高风险旧字段必须审批、过期、审计 |
| P1 | 错误定位 | validation model/API | 每个错误带 nodeId/path/field/action |

发布校验必须成为代码实现的硬门槛，不能只依赖 Studio 前端提示。

### I. Studio 前端

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P0 | Service/Interface selector | `frontend/admin-flow/src` | 节点可以选择 service -> interface，展示 schema/operation |
| P0 | Resource selector | `frontend/admin-flow/src` | 只显示有权限且 capability 匹配的 Resource |
| P0 | ProviderChain editor | `frontend/admin-flow/src/components` | 按 targetType 切换表单，不显示底层连接字段 |
| P0 | 发布错误定位 | Studio validation panel | 点击错误能定位 node/path/field |
| P1 | 能力资产中心 | Services/Interfaces/Providers/Schemas/Bindings 页面 | 可管理资产和运行 test/check |
| P1 | Binding 编辑器 | Studio/Admin | 可创建 scope/principal/nodeType 授权并运行 check |
| P1 | Migration Diff Panel | Studio | 旧字段能展示 before/after 并 apply 到 draft |
| P1 | Run history trace | Studio | 展示 selectedProvider、attempts、schema、auditId，默认脱敏 |

前端不能为了快速接入而重新加入 URL、broker、secret、raw SQL 输入框。

### J. 运维、监控和安全

| 优先级 | 改造项 | 文件/模块 | 完成标准 |
| --- | --- | --- | --- |
| P1 | Provider health monitor | worker/cron 或 admin action | 定期更新 healthStatus |
| P1 | Circuit metrics | monitoring | open/half-open/close 有指标和告警 |
| P1 | Rate limit metrics | monitoring | 超限、拒绝、fallback 有指标 |
| P1 | Audit retention job | ops job | 按保留周期清理或归档审计 |
| P1 | Secret/SSRF guard | HTTP/MQ Provider adapter | 禁止内网 SSRF、明文 secret、非法 host |
| P1 | Runbook 更新 | `docs/operations.md`、runbooks | unhealthy、circuit reset、seed drift、break-glass 有操作步骤 |

### K. 测试必须补齐

| 测试 | 必须覆盖 |
| --- | --- |
| `internal/marketing` | model validate、node schema、publish validation |
| `internal/engine` | ProviderResolver、ProviderChainExecutor、SchemaValidator、Lua service/resource API |
| `internal/httpapi` | service/schema/binding/provider/migration API |
| `internal/dbmigration` | 新 migration、drift、索引和外键 |
| `frontend/admin-flow` | selector、ProviderChain editor、validation panel、migration diff |
| e2e | 新 workflow 通过 Service Interface 查询事实，旧 workflow 可运行并提示迁移 |

每个阶段完成时，至少跑：

```text
go test ./internal/marketing ./internal/engine ./internal/httpapi ./internal/dbmigration
npm run build
```

如果改了 Studio 节点配置，还必须补 Playwright 截图或 e2e 验证。

## 后续写代码硬规则

后续实现必须遵守这些硬规则：

1. 新节点配置不得新增 `endpoint/redisRef/grpcTarget/broker/dbDsn/secret/rawSql`。
2. 业务能力调用一律先建 Service Interface，再由 Provider 实现。
3. 底层连接、Topic、Store、Counter、Lock 一律建 Resource。
4. Provider 可以依赖 Resource，普通节点不能绕过 Provider 直连外部业务系统。
5. `providerRef` 显式指定是高风险能力，必须有 Binding 授权和审计。
6. SchemaRef 必须能保存、校验、版本化，不能只是文本字段。
7. Binding 校验必须在保存、测试、发布和运行时至少发布/运行两层生效。
8. 旧字段只允许兼容读取、提示迁移和 break-glass，不允许成为新模板默认值。
9. 所有 service/resource 调用必须写 trace 和脱敏 audit。
10. MemoryRepository 和 PostgreSQL repository 必须同时支持新增模型。
11. 前端只是体验层，发布安全必须由后端 validation/release gate 保证。
12. 文档、API、schema、handler、模板、测试必须同一次改造保持一致。

## 目标分层

### 资产层

| 资产 | 作用 | 例子 |
| --- | --- | --- |
| `Service` | 能力归属 | `user-profile-service` |
| `Interface` | 能力契约 | `query-user-profile` |
| `Provider` | 契约实现 | `user-profile-resource-primary`、`wallet-http-primary` |
| `Resource` | 底层受控能力 | `default_user_profile_store`、`wallet-http-client`、`reward-command-topic` |
| `Schema` | 输入/输出/错误契约 | `user-profile-query-v1`、`user-profile-v1` |
| `Binding` | 谁能调用接口 | workspace/folder/activity/node 级授权 |
| `Audit` | 谁在何处调用了什么 | interface call audit、resource operation audit |

### 运行层

运行时必须固定顺序：

```text
1. Node 解析 providerChain step
2. 校验 Service Interface 存在且 ACTIVE
3. 校验 Interface Binding 和 actor/workspace/folder/activity/node 权限
4. 按 inputSchemaRef 校验 inputMapping 结果
5. ProviderResolver 选择 Provider
6. 校验 Provider environment/status/health/circuit/rateLimit/mock
7. 校验 Provider 依赖 Resource 权限和 capability
8. 调用 ProviderAdapter 或 ResourceOperator
9. 按 outputSchemaRef/errorSchemaRef 校验输出
10. 写 trace、audit、metrics
11. 失败时按 providerPolicy 决定 fallback、BLOCK、NEXT 或 retry
```

## 阶段改造总览

| 阶段 | 目标 | 是否阻断新功能 |
| --- | --- | --- |
| P0 文档和命名收敛 | 所有文档和 UI 文案统一 Service/Interface/Provider/Resource 边界 | 是 |
| P1 DDL 和模型补齐 | 补 schema/binding/audit/provider runtime state，约束和索引完整 | 是 |
| P2 Admin API 契约补齐 | Studio 能完整管理资产、校验、测试、迁移 | 是 |
| P3 Provider 治理闭环 | PRIMARY/fallback/weight/env/health/circuit/rateLimit/mock 可用 | 是 |
| P4 Executor 统一执行链 | 节点统一调用 Service Interface / Resource | 是 |
| P5 节点和 Lua 改造 | fact-query/activity-state/lua-query 使用新 providerChain | 否，分批 |
| P6 Studio 前端改造 | 能力资产中心和节点选择器落地 | 否，分批 |
| P7 迁移和兼容收敛 | 旧字段可分析、可迁移、可阻断发布 | 否，分批 |

## 当前实现进度

本节记录代码实现状态，后续开发以这里为准，不要再按旧模型新增能力。

| 日期 | 范围 | 已落地 | 仍缺 |
| --- | --- | --- | --- |
| 2026-06-24 | P0/P3/P5 第一批 | `ProviderChainStep` 共享模型；workflow validation 识别 `SERVICE_INTERFACE/RESOURCE` 新 step；旧 `PROVIDER/FACT_SERVICE/HTTP/REDIS/GRPC/PLUGIN/CUSTOM` 进入 legacy warning；新 `SERVICE_INTERFACE/RESOURCE` step 禁止 `endpoint/redisRef/grpcTarget/broker/dbDsn/secret/rawSql`；ProviderResolver 支持 `PRIMARY`、`PRIMARY_WITH_FALLBACK`、`WEIGHTED`、`ENVIRONMENT_MATCH`、`MOCK_ONLY` 策略入口；fact-query 执行链可用 `SERVICE_INTERFACE + PRIMARY_WITH_FALLBACK` 解析内置 Provider，且 primary provider 失败后会尝试 fallback provider；目标测试通过 | schema registry、binding、provider runtime state、health/circuit/rate-limit、统一 Admin API 响应、migration analyze/apply、Studio 资产中心仍未落地；`WEIGHTED` 当前只完成候选过滤和 priority/weight 排序，稳定 hash 流量分配待后续补齐 |

本轮已改代码：

| 文件 | 说明 |
| --- | --- |
| `internal/marketing/workflow_model.go` | 增加 `ProviderChainStep` / `ProviderChainTargetType` 共享模型 |
| `internal/marketing/workflow_validation_service.go` | 增加新旧 providerChain 边界校验、legacy migration warning、forbidden infra field 校验、Provider policy 校验 |
| `internal/marketing/workflow_model_test.go` | 更新 workflow validation 测试：旧配置 warning，新配置强校验 |
| `internal/engine/service_asset_service.go` | ProviderResolver 支持多策略名，并按 status/priority/weight/mock 过滤排序 |
| `internal/engine/service_asset_service_test.go` | 覆盖 unsupported policy、fallback、weighted、mock-only |
| `internal/engine/workflow_executor_test.go` | 覆盖 fact-query 通过 Service Interface + `PRIMARY_WITH_FALLBACK` 查询事实 |

当前后端行为：

```text
旧 source=HTTP/REDIS/GRPC/PLUGIN/PROVIDER/FACT_SERVICE
  -> 兼容通过
  -> 返回 LEGACY_NODE_CONFIG_MIGRATION_AVAILABLE warning

新 targetType=SERVICE_INTERFACE
  -> 必须 serviceRef + interfaceRef
  -> 禁止 endpoint/redisRef/grpcTarget/broker/dbDsn/secret/rawSql
  -> providerPolicy 支持 PRIMARY / PRIMARY_WITH_FALLBACK / WEIGHTED / ENVIRONMENT_MATCH / MOCK_ONLY
  -> PRIMARY_WITH_FALLBACK 在 fact-query 执行链中会按候选 provider 逐个尝试
  -> WEIGHTED 当前为第一阶段候选选择，完整稳定 hash 权重分流仍待实现

新 targetType=RESOURCE
  -> 必须 resourceRef
  -> 禁止 endpoint/redisRef/grpcTarget/broker/dbDsn/secret/rawSql
```

注意：`ENVIRONMENT_MATCH` 目前只完成策略名入口，尚未接入 request environment；health/circuit/rate-limit 仍待 provider runtime state 表落地后实现。

## P0 文档和命名收敛

要先把所有文档、接口、前端文案的语义固定下来，否则后续代码会继续混用。

改造项：

| 文件 | 要求 |
| --- | --- |
| `docs/service-interface-provider-resource-design.zh-CN.md` | 保持主设计为权威概念和契约文档 |
| `docs/resource-management-design.zh-CN.md` | 只描述 Resource 治理，HTTP/MQ Resource 只能是受控 client/topic/connection |
| `docs/fact-query-node.zh-CN.md` | 新配置首选 `SERVICE_INTERFACE`，`PROVIDER` 只作为兼容入口 |
| `docs/workflow-node-catalog.zh-CN.md` | 节点目录里标明哪些节点调用 Service Interface，哪些节点操作 Resource |
| `docs/api.md` / `docs/data-model.md` | API 和数据模型命名与主设计一致 |

验收：

```text
rg "节点首选入口.*PROVIDER|新配置优先.*PROVIDER|HTTP Provider Resource" docs
```

不能再出现把旧 Provider Resource 写成新配置默认入口的描述。

## P1 DDL 和模型补齐

当前 `012_service_assets.sql` 只够基础 CRUD。完整治理需要拆成最终表结构或追加 migration。

建议新增 migration：

```text
013_service_schema_registry.sql
014_service_bindings.sql
015_service_provider_runtime_state.sql
016_service_call_audit.sql
017_service_asset_constraints_indexes.sql
018_builtin_service_asset_seed.sql
```

### 表结构改造

| 表 | 用途 |
| --- | --- |
| `marketing_service_schema` | 保存 JSON Schema、版本、状态、兼容信息 |
| `marketing_service_interface_binding` | 保存 workspace/folder/activity/node 对 interface 的调用权限 |
| `marketing_service_provider_state` | 保存 provider health、circuit、last check、失败计数 |
| `marketing_service_rate_limit` | 保存 provider/interface 限流配置 |
| `marketing_service_call_audit` | 保存 service interface 调用审计摘要 |
| `marketing_service_provider_audit` | 保存 provider 选择、fallback、失败原因 |
| `marketing_service_migration_audit` | 保存旧字段迁移 diff、执行人、结果 |

`marketing_service_provider` 需要补字段或通过 state 表承载：

```text
default_policy
fallback_policy_json
mock_enabled
health_check_config_json
circuit_policy_json
rate_limit_ref
allowed_environments_json
config_hash
```

约束要求：

| 约束 | 说明 |
| --- | --- |
| service code 唯一 | `(tenant_id, code)` |
| interface code 唯一 | `(tenant_id, service_ref, code)` |
| provider code 唯一 | `(tenant_id, service_ref, interface_ref, code)` |
| schema ref 唯一 | `(tenant_id, schema_ref, version)` |
| binding 唯一 | scope + principal + service/interface + operation |
| provider state 外键 | 指向 provider 唯一键 |
| audit 只追加 | 不允许更新业务关键列 |

JSONB 索引：

```text
provider.config_json
provider.resource_refs_json
schema.schema_json
binding.allowed_node_types_json
audit.redacted_input_json
audit.redacted_output_json
```

Model 改造文件：

| 文件 | 改造 |
| --- | --- |
| `internal/marketing/service_asset_model.go` | 增加 Schema、Binding、ProviderState、Audit、MigrationPlan 类型 |
| `internal/marketing/resource_model.go` | 增加 Resource dependency/query helper 字段 |
| `internal/marketing/workflow_validation_model.go` | 增加 service/interface/provider/resource 错误码 |

验收：

```text
go test ./internal/dbmigration ./internal/marketing
```

并确认 migration drift 能识别新增表、索引、外键、check 约束。

## P2 Admin API 契约补齐

当前 API 是基础 CRUD，需要改成 Studio 可直接使用的契约。

### 必须补齐的 API

| API | 作用 |
| --- | --- |
| `GET/POST/PATCH /admin/v1/services` | 管理 Service |
| `GET/POST/PATCH /admin/v1/services/{serviceCode}/interfaces` | 管理 Interface |
| `GET/POST/PATCH /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/providers` | 管理 Provider |
| `GET/POST/PATCH /admin/v1/service-schemas` | 管理 Schema |
| `POST /admin/v1/service-schemas/{schemaRef}/validate` | 校验 payload |
| `POST /admin/v1/service-schemas/compatibility-check` | 校验 schema 兼容性 |
| `GET/POST/PATCH /admin/v1/service-interface-bindings` | 管理 Binding |
| `POST /admin/v1/service-interface-bindings/check` | 检查调用权限 |
| `POST /admin/v1/services/{serviceCode}/interfaces/{interfaceCode}/test` | 测试 Interface 调用链 |
| `POST /admin/v1/service-providers/{providerCode}/health-check` | 触发 health check |
| `POST /admin/v1/service-providers/{providerCode}/circuit/reset` | 重置 circuit |
| `POST /admin/v1/workflows/{activityId}/migration/service-interface/analyze` | 分析旧字段迁移 |
| `POST /admin/v1/workflows/{activityId}/migration/service-interface/apply` | 应用迁移到 draft |

### 通用响应

所有新 API 返回结构统一：

```json
{
  "requestId": "req_001",
  "data": {},
  "warnings": [],
  "meta": {
    "tenantId": "site_001",
    "workspaceId": "growth",
    "environment": "draft"
  }
}
```

错误响应统一：

```json
{
  "requestId": "req_001",
  "error": {
    "code": "SERVICE_INTERFACE_BINDING_DENIED",
    "message": "interface is not allowed for this workflow node",
    "field": "providerChain[0].interfaceRef",
    "details": {
      "serviceRef": "user-profile-service",
      "interfaceRef": "query-user-profile",
      "nodeId": "facts"
    }
  }
}
```

HTTP handler 改造文件：

| 文件 | 改造 |
| --- | --- |
| `internal/httpapi/service_asset_admin.go` | 从基础 CRUD 扩展为完整 API |
| `internal/httpapi/resource_admin.go` | Resource test/permission 返回结构和 service API 对齐 |
| `internal/httpapi/handler.go` | 注册新增路由 |
| `internal/httpapi/handler_test.go` | 覆盖服务资产 API 契约 |

验收：

```text
go test ./internal/httpapi ./internal/engine
```

## P3 Provider 治理闭环

当前 `ResolveServiceProvider` 只支持 PRIMARY。需要把 Provider Resolver 做成治理层。

### ProviderResolver 行为

输入：

```text
tenantId
workspaceId
folderId
activityId
nodeId
nodeType
serviceRef
interfaceRef
providerPolicy
providerRef
environment
input
```

输出：

```text
selectedProvider
attemptPlan
fallbackPlan
governanceDecision
warnings
```

策略要求：

| 策略 | 行为 |
| --- | --- |
| `PRIMARY` | 选择 priority 最小且 healthy 的 provider |
| `PRIMARY_WITH_FALLBACK` | primary 失败后按 priority 选择 fallback |
| `WEIGHTED` | 在同一 priority 组内按 weight 选择 |
| `ENVIRONMENT_MATCH` | 只选择当前 environment 可用 provider |
| `MOCK_ONLY` | 只在 preview/test 环境允许 |

治理检查顺序：

```text
status ACTIVE
environment match
mock policy
healthStatus HEALTHY/UNKNOWN
circuitStatus CLOSED/HALF_OPEN
rate limit allowed
resource dependencies ACTIVE
```

新增或改造文件：

| 文件 | 改造 |
| --- | --- |
| `internal/engine/service_asset_service.go` | 拆出 ProviderResolver，不把选择逻辑塞在 CRUD service 里 |
| `internal/engine/service_provider_resolver.go` | 新增 provider 选择、fallback、health/circuit/rateLimit 判断 |
| `internal/engine/service_provider_state.go` | 新增 health/circuit 状态读写 |
| `internal/marketing/service_asset_model.go` | 增加 ProviderPolicy、ProviderAttempt、ProviderDecision |

验收场景：

| 场景 | 预期 |
| --- | --- |
| primary healthy | 选择 primary |
| primary unhealthy + fallback healthy | 选择 fallback |
| circuit open | 跳过该 provider |
| environment 不匹配 | 不进入候选 |
| mock provider 在 prod | 发布失败 |
| weight 100/0 | 只选 weight > 0 |
| rate limit exceeded | 返回 `SERVICE_PROVIDER_RATE_LIMITED` |

## P4 Executor 统一执行链

不要让每个节点各自实现 HTTP/Redis/Provider 调用。新增统一执行器。

建议结构：

```text
ProviderChainExecutor
  ServiceInterfaceInvoker
    BindingChecker
    SchemaValidator
    ProviderResolver
    ProviderAdapterRegistry
  ResourceOperator
    ResourcePermissionChecker
    ResourceAdapterRegistry
  AuditWriter
```

新增文件建议：

| 文件 | 作用 |
| --- | --- |
| `internal/engine/provider_chain_executor.go` | 统一执行 `ProviderChainStep` |
| `internal/engine/service_interface_invoker.go` | 执行 Service Interface 调用 |
| `internal/engine/provider_adapter.go` | Provider adapter interface 和 registry |
| `internal/engine/provider_adapter_http.go` | HTTP Provider |
| `internal/engine/provider_adapter_mq.go` | MQ Provider |
| `internal/engine/provider_adapter_plugin.go` | Plugin Provider |
| `internal/engine/provider_adapter_resource.go` | RESOURCE Provider 转 ResourceOperator |
| `internal/engine/resource_operator.go` | Resource 操作统一入口 |
| `internal/engine/service_call_audit.go` | 调用审计写入 |

`ProviderChainStep` 标准结构：

```json
{
  "targetType": "SERVICE_INTERFACE",
  "serviceRef": "user-profile-service",
  "interfaceRef": "query-user-profile",
  "providerPolicy": "PRIMARY_WITH_FALLBACK",
  "providerRef": "",
  "operation": "CALL",
  "inputMapping": {
    "tenantId": "${tenantId}",
    "userId": "${subject.id}"
  },
  "outputAs": "facts.USER_PROFILE",
  "timeoutMs": 1000,
  "onMiss": "BLOCK"
}
```

Resource step：

```json
{
  "targetType": "RESOURCE",
  "resourceRef": "checkin-state-redis",
  "operation": "READ",
  "namespace": "checkin",
  "keyTemplate": "${tenantId}:${activityId}:${subject.id}",
  "valuePath": "$.state",
  "outputAs": "checkinState",
  "timeoutMs": 500,
  "onMiss": "NEXT"
}
```

验收：

```text
go test ./internal/engine ./internal/marketing
```

并覆盖 trace：

```text
serviceRef
interfaceRef
providerPolicy
selectedProvider
attempts
resourceRefs
inputSchemaValid
outputSchemaValid
durationMs
auditId
```

## P5 节点和 Lua 改造

优先改造会直接访问事实、状态、脚本能力的节点。

节点规范不是只改某几个 handler，而是要同时改：

```text
node catalog
node config schema
workflow template
prelaunch validation
executor handler
run history trace
Studio node config panel
legacy migration
```

所有节点配置统一遵守：

| 类型 | 规范 |
| --- | --- |
| 业务能力调用 | `targetType=SERVICE_INTERFACE` |
| 受控底层资源操作 | `targetType=RESOURCE` |
| 上游输出读取 | `targetType=WORKFLOW_CONTEXT` |
| 本次执行缓存 | `targetType=LOCAL_CACHE` |
| 旧公共 Provider | `source=PROVIDER` 只读兼容 |
| 旧 Fact Service | `source=FACT_SERVICE` 只读兼容 |

节点 schema 中不再新增这些字段：

```text
endpoint
providerEndpoint
redisRef
grpcTarget
broker
rawTopic
dbDsn
secret
rawSql
```

| 节点 | 改造方式 |
| --- | --- |
| `fact-query` | 新配置使用 `SERVICE_INTERFACE` 查询事实，Store/Cache fallback 用 `RESOURCE`，兼容旧 `PROVIDER/FACT_SERVICE` |
| `activity-state` | 内部状态或 `RESOURCE` step，禁止节点直填 Redis/HTTP |
| `state-store` | 只保留受控 Resource 操作，不暴露底层连接 |
| `quota-check` | 使用 `RESOURCE` counter/lock step；后续可迁移到 quota Service Interface |
| `candidate-reward` | 简单奖励配置读 `RESOURCE`，复杂奖励生成走 reward Service Interface |
| `budget-reserve` | 预算/库存锁定走 quota Service Interface，旧 `reserveProviderRef/resourceRef` 兼容读取 |
| `outbox` / `http-call` / `mq-publish` | 节点表达投递意图；真实协议实现落到 Service Provider 或 OutboxWorker 依赖 Resource |
| `lua-query` / `code` | 禁止原始 HTTP/Redis/DB/MQ，只允许 service/resource 受控函数 |

### 节点 Schema 改造

`internal/marketing/workflow_node_config_schema.go` 要把 `ProviderChainStep` 抽成共享 schema，不允许每个节点复制一套字段。

共享字段：

```json
{
  "targetType": "SERVICE_INTERFACE",
  "source": "SERVICE_INTERFACE",
  "serviceRef": "",
  "interfaceRef": "",
  "providerPolicy": "PRIMARY",
  "providerRef": "",
  "resourceRef": "",
  "operation": "",
  "namespace": "",
  "keyTemplate": "",
  "inputMapping": {},
  "valuePath": "",
  "outputAs": "",
  "timeoutMs": 1000,
  "onMiss": "BLOCK"
}
```

每个节点只声明允许的 `targetType` 和必填字段：

| 节点 | 允许 targetType | 必填 |
| --- | --- | --- |
| `fact-query` | `SERVICE_INTERFACE`、`RESOURCE`、`WORKFLOW_CONTEXT`、`LOCAL_CACHE`、兼容 `PROVIDER/FACT_SERVICE` | `factKeys`、`subjectPath`、`providerChain` |
| `activity-state` | `RESOURCE`、`WORKFLOW_CONTEXT`、内部 state adapter | `namespace`、`keyTemplate`、`outputAs` |
| `quota-check` | `RESOURCE` | `resourceRef`、`operation`、`scopeKeyTemplate`、`window`、`maxCount` |
| `candidate-reward` | `RESOURCE`、`SERVICE_INTERFACE` | `candidatePolicy`、`outputAs` |
| `budget-reserve` | `SERVICE_INTERFACE`、兼容 `RESOURCE` | `serviceRef/interfaceRef`、`idempotencyKeyTemplate`、`failAction` |
| `code/lua-query` | 通过 allowed services/resources 声明，不直接暴露 providerChain | `runtime`、`allowedServices`、`allowedResources` |

### 节点发布校验改造

`internal/marketing/prelaunch_validation.go` 和 `workflow_validation_service.go` 需要增加节点级强校验：

| 校验 | 错误码 |
| --- | --- |
| 节点使用禁止字段 | `NODE_FORBIDDEN_INFRA_FIELD` |
| targetType 不在节点允许列表 | `NODE_PROVIDER_CHAIN_TARGET_UNSUPPORTED` |
| `SERVICE_INTERFACE` 缺 service/interface | `SERVICE_INTERFACE_REF_REQUIRED` |
| `RESOURCE` 缺 resourceRef/operation | `RESOURCE_OPERATION_REQUIRED` |
| provider override 未授权 | `SERVICE_PROVIDER_OVERRIDE_DENIED` |
| Lua 调用未声明服务 | `LUA_SERVICE_NOT_ALLOWED` |
| Lua 调用未声明资源 | `LUA_RESOURCE_NOT_ALLOWED` |
| 旧字段进入正式发布 | `LEGACY_NODE_CONFIG_PUBLISH_DENIED` |

校验结果必须带：

```text
nodeId
nodeType
path
field
code
suggestion
```

### 节点模板改造

内置模板和新建节点默认值必须使用新规范：

| 模板位置 | 要求 |
| --- | --- |
| 每日签到 fact-query | `SERVICE_INTERFACE user-profile-service.query-user-profile` |
| 每日签到 activity-state | 内部 state adapter 或 `RESOURCE default_workflow_state` |
| 每日签到 quota-check | `RESOURCE default_checkin_quota_counter` |
| reward config | `RESOURCE checkin_reward_config_ref` 或 reward Service Interface |
| budget reserve | quota Service Interface |
| outbox | 保留 outbox intent，不在节点里直填 broker/url |

### 节点运行历史改造

run history 不应只显示节点原始 JSON。每个 ProviderChainStep 要输出：

```text
targetType
serviceRef/interfaceRef/resourceRef
providerPolicy
selectedProvider
operation
status
durationMs
onMiss action
schema validation summary
auditId
redacted input/output summary
```

旧字段运行时必须写 deprecation trace：

```text
legacyField
recommendedTargetType
recommendedServiceRef
recommendedInterfaceRef
migrationAvailable
```

Lua 允许：

```lua
service_call("user-profile-service", "query-user-profile", input)
resource_read("checkin-state-redis", "checkin", key)
resource_write("checkin-state-redis", "checkin", key, value)
mq_publish("reward-service", "publish-reward-command", payload)
```

Lua 禁止：

```lua
http.post(...)
redis.get(...)
db.query(...)
mq.raw_publish(...)
os.execute(...)
io.open(...)
```

改造文件：

| 文件 | 改造 |
| --- | --- |
| `internal/engine/workflow_executor.go` | 节点执行分流到 ProviderChainExecutor |
| `internal/engine/workflow_lua.go` | 注入 service/resource 受控函数，移除底层连接函数 |
| `internal/marketing/workflow_node_config_schema.go` | 节点 schema 增加 ProviderChainStep |
| `internal/marketing/prelaunch_validation.go` | 发布前校验 service/interface/provider/resource/schema/binding |
| `internal/marketing/workflow_validation_service.go` | 返回可定位到 node/path/field 的错误 |

## P6 Studio 前端改造

前端要从“资源选择器”升级成“能力资产中心 + 节点能力选择器”。

### 能力资产中心

页面：

```text
Services
  Service Detail
    Interfaces
    Providers
    Bindings
    Usage
    Audit
Schemas
Resources
Migration
```

### 节点配置面板

ProviderChain Step 编辑器按 `targetType` 切换：

| targetType | UI |
| --- | --- |
| `SERVICE_INTERFACE` | Service selector、Interface selector、ProviderPolicy、Input Mapping、Output As |
| `RESOURCE` | Resource selector、Operation、Namespace、Key Template、Value Path |
| `WORKFLOW_CONTEXT` | Source Path、Output As |
| `LOCAL_CACHE` | Cache Path、Output As |

禁止在节点表单里出现：

```text
HTTP URL
Redis address
DB DSN
MQ broker
secret
raw topic
```

改造文件方向：

| 文件/目录 | 改造 |
| --- | --- |
| `frontend/admin-flow/src/App.tsx` | 增加能力资产中心路由或 tab |
| `frontend/admin-flow/src/types.ts` | 增加 Service/Interface/Provider/Schema/Binding 类型 |
| `frontend/admin-flow/src/components` | 增加 Service selector、ProviderChain editor、Validation panel |
| `frontend/admin-flow/e2e` | 增加 service asset 和 migration diff 交互测试 |

验收：

```text
npm run build
npm run test:e2e
```

如果当前项目没有完整 e2e 脚本，至少保留 Playwright 截图验证主要页面状态。

## P7 迁移和兼容

迁移不能一次性删除旧字段。采用双读、提示、迁移、阻断、下线五步。

| 阶段 | 后端 | 前端 | 发布校验 |
| --- | --- | --- | --- |
| 兼容读取 | executor 继续识别旧字段 | 打开旧 workflow 显示兼容模式 | WARNING |
| 迁移分析 | migration analyze 生成建议 | 展示 migration diff | 可发布但提示 |
| 半自动迁移 | apply 写新 draft | 用户确认后应用 | 新 draft 重新 validate |
| 发布阻断 | 高风险旧字段禁止正式发布 | 定位到节点和字段 | ERROR |
| 兼容下线 | allowlist 才可执行旧字段 | 旧字段只读 | 需要 break-glass |

迁移规则：

| 旧配置 | 新目标 |
| --- | --- |
| `source=PROVIDER resourceRef=user-profile-provider` | `SERVICE_INTERFACE user-profile-service.query-user-profile` |
| `source=FACT_SERVICE providerCode=kyc` | `SERVICE_INTERFACE kyc-service.query-kyc-profile` |
| `redisRef/keyTemplate` | `RESOURCE REDIS` step |
| `endpoint/providerEndpoint` | 新建 HTTP Provider 草案 + HTTP Client Resource |
| `tableName/sql` | `RESOURCE TABLE` 或人工 review |

迁移 API 必须保证：

1. 只修改 draft，不改已发布 version。
2. apply 前重新跑权限、schema、resource 校验。
3. 一次 apply 全部成功或全部失败。
4. 保存 before/after diff 和 auditId。
5. 不自动迁移低置信度项。

## P8 发布校验和错误码

发布前必须检查：

| 检查 | 错误码 |
| --- | --- |
| Service 不存在或 disabled | `SERVICE_NOT_FOUND` / `SERVICE_DISABLED` |
| Interface 不存在或 disabled | `SERVICE_INTERFACE_NOT_FOUND` / `SERVICE_INTERFACE_DISABLED` |
| Binding 不允许 | `SERVICE_INTERFACE_BINDING_DENIED` |
| input mapping 缺 required 字段 | `SERVICE_MAPPING_INPUT_REQUIRED_MISSING` |
| schema 不存在或 inactive | `SERVICE_SCHEMA_NOT_FOUND` / `SERVICE_SCHEMA_INACTIVE` |
| Provider 不存在 | `SERVICE_PROVIDER_REQUIRED` |
| Provider unhealthy 且无 fallback | `SERVICE_PROVIDER_UNHEALTHY` |
| Circuit open 且无 fallback | `SERVICE_PROVIDER_CIRCUIT_OPEN` |
| Resource 不存在或 disabled | `RESOURCE_NOT_FOUND` / `RESOURCE_DISABLED` |
| Resource capability 不支持 | `RESOURCE_CAPABILITY_UNSUPPORTED` |
| 旧字段需要迁移 | `LEGACY_PROVIDER_CHAIN_PUBLISH_DENIED` |
| break-glass 缺审批 | `BREAK_GLASS_APPROVAL_REQUIRED` |

校验结果必须返回：

```text
severity
code
nodeId
path
field
message
suggestion
action
requestId
```

## P9 测试验收矩阵

| 测试层 | 覆盖 |
| --- | --- |
| Model unit | service/interface/provider/schema/binding validate |
| Repository | DDL、唯一索引、外键、JSONB 查询、audit append-only |
| Engine unit | ProviderResolver、BindingChecker、SchemaValidator、ProviderChainExecutor |
| API test | CRUD、test、health、circuit reset、migration analyze/apply |
| Executor integration | fact-query/activity-state/lua-query 调用 service/resource |
| Migration test | 旧 workflow 打开、分析、迁移、重新发布 |
| Frontend e2e | Service 列表、Interface 详情、Provider 配置、Binding 编辑、发布错误定位 |
| Security test | secret 脱敏、SSRF、providerRef 绕行、Lua 沙箱、审计隐私 |

最低通过标准：

```text
go test ./internal/marketing ./internal/engine ./internal/httpapi ./internal/dbmigration
npm run build
```

## 推荐实施顺序

不要先做大前端，也不要先把旧字段删掉。推荐顺序：

```text
1. P1 DDL/model 补齐
2. P2 Admin API 契约补齐
3. P3 ProviderResolver 治理闭环
4. P4 ProviderChainExecutor 统一执行链
5. P5 fact-query/activity-state/lua-query 接入
6. P8 发布校验错误码接入
7. P6 Studio 能力资产中心和节点编辑器
8. P7 migration analyze/apply 和旧字段发布阻断
9. 补齐 runbook、监控、audit retention
10. 灰度新模板，最后收敛旧字段
```

每一步完成后都应该保持旧 workflow 可运行。

## 不建议一次做的内容

以下内容可以延后，不要阻塞第一阶段治理闭环：

1. 全协议 Provider 平台化。
2. 复杂服务编排 DSL。
3. 自动生成所有 Provider。
4. 删除旧 workflow 字段。
5. 把所有节点一次性迁移。
6. 让运营直接编辑高级 JSON config。

第一阶段只需要让核心链路闭环：

```text
fact-query -> Service Interface -> RESOURCE Provider -> TABLE/REDIS Resource
activity-state -> RESOURCE step -> REDIS Resource
lua-query -> service_call/resource_read -> 审计和权限
```

## 完成定义

整体改造完成时，应满足：

1. 新建活动模板不包含底层连接字段。
2. Studio 节点配置默认选择 Service Interface。
3. Provider resolver 可以执行 PRIMARY、fallback、environment、health、circuit、rateLimit。
4. Interface input/output/error schema 在保存、测试、发布和运行时都生效。
5. Service Interface Binding 阻止越权调用。
6. Resource 权限阻止 providerRef 绕行和底层资源越权。
7. 所有 service/resource 调用都有脱敏审计。
8. 旧 workflow 能运行，但发布新版本时会提示或要求迁移。
9. Migration diff 能把旧 Provider Resource 转为 Service Interface step。
10. 运维可以处理 unhealthy provider、circuit reset、seed drift、break-glass 和误禁用恢复。
