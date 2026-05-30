# Analysis: 2026-05-29-alerts-rules-routing

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未写业务代码，未提交，未设计具体代码改法。

已读取：

- `tasks/2026-05-29-alerts-rules-routing/review.md`
- `tasks/2026-05-29-alerts-rules-routing/analysis.md` 上一版
- `tasks/2026-05-29-alerts-rules-routing/task.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- 相关历史/相邻任务材料：`tasks/2026-05-27-engine-metrics-alerts/analysis.md`、`tasks/2026-05-29-alerts-prometheus-grafana-prometheus-grafana/analysis.md`

未发现独立 PRD 目录。本分析以任务说明、已有 API / 数据模型 / 运维文档和监控类历史任务口径为依据。

## Review 反馈处理

| Review 项 | 处理 |
| --- | --- |
| P1：DB 写失败 P0 规则不可按当前指标验收。 | 已修正。当前 `docs/operations.md` 的 `exmarketing_db_operation_total` 仅可作为已埋点 DB 操作错误告警，不能宣称覆盖核心写失败。本需求明确本任务允许补齐告警所需的最小 DB write metrics 契约，并列出核心写路径、标签和验收口径；在这些写路径可观测前，`ExMarketingDBWriteFailure` 不得作为已启用 P0 通过验收。 |
| P1：Fact Query P2 告警缺少可实现指标契约。 | 已修正。补充 `exmarketing_fact_query_total{tenant_id,fact_code,result,reason}` 与 `exmarketing_fact_query_latency_seconds{tenant_id,fact_code,result}` 的最小指标契约。`ExMarketingFactQueryFailRateHigh` 必须依赖该契约或等价指标启用；若未补齐，只能作为未启用依赖项，不能计入 P2 覆盖验收。 |
| P2：DLQ/backlog 聚合缺少保留 `tenant_id,target_channel` 的要求。 | 已修正。规则表达式必须按 `tenant_id,target_channel` 聚合并映射为告警标签 `tenant,channel`；不得使用全局 sum 丢失路由维度。 |
| Open Question：本任务是否允许 Dev Agent 修改指标埋点？ | 默认最小口径：允许为了本任务告警可验收而补齐观测指标契约，包括 DB write metrics 和 Fact Query metrics；不允许改变 event、quota、grant_order、outbox、callback、Fact Query 的业务状态机或语义。 |
| Open Question：Fact Query 告警是否必须随本任务启用？ | 默认最小口径：必须随本任务以“可触发规则”启用，前提是同时补齐最小指标契约或提供等价可 scrape 指标映射；没有指标时不得以文档占位通过。 |
| Score 76 / Decision BLOCK。 | 已按上述阻塞点重写需求边界、输入输出、指标契约、业务规则、验收标准和非目标范围，重新评分并给出 `Decision PASS`。 |

## Analysis 自检修正

上一版 `analysis.md` 虽给出 `Score 91` 和 `Decision PASS`，但 Review 证明存在错误放宽和指标契约冲突。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| 错误假设：当前 DB operation 指标可覆盖核心 DB 写失败。 | 已关闭。当前已知口径只覆盖 metrics 刷新依赖的 outbox/DLQ 查询；核心 DB 写失败必须有 write metrics 或等价可观测信号支撑。 |
| 错误假设：Fact Query 告警可在指标缺失时仍作为 P2 必选项。 | 已关闭。Fact Query 告警必须绑定明确指标契约；缺失时验收失败而不是占位通过。 |
| 扣分项：非目标范围写了“不新增业务指标语义”，与告警可验收性冲突。 | 已修正为“不新增或改变业务状态机；允许补齐本任务告警所需观测指标契约”。 |
| 待确认问题：生产通知工具、最终 runbook URL。 | 不阻塞。继续采用抽象接收组和稳定占位 runbook 路径，部署阶段映射实际工具和最终 URL。 |
| 待确认问题：指标名与现有文档不一致。 | 不阻塞但必须显式映射。规则交付物必须列出现有指标或新增/等价指标映射，且语义可通过测试触发验证。 |

## 需求说明

本需求要求为 ExMarketing 活动引擎定义生产可用的告警规则与通知路由。告警必须围绕活动引擎数据一致性和发奖链路可靠性，覆盖 DLQ、callback、outbox、worker、DB、quota、event intake、claim latency、retry 和 Fact Query 等异常。

最小可验收交付为：

1. Alertmanager 或等价告警平台的规则文件/规则文档。
2. P0/P1/P2/P3 告警等级定义。
3. 通知路由、重复告警分组、升级策略、静默和维护窗口规则。
4. 告警抑制规则，尤其是 DB 故障时抑制下游泛洪告警。
5. 告警消息模板，包含 `service`、`tenant`、`channel`、`severity`、`summary`、`dashboard`、`runbook` 等上下文。
6. 告警所需指标契约和现有指标映射，包含 DB 核心写失败和 Fact Query 失败率的可验证观测口径。
7. 静态校验或单元验证说明，例如 promtool 检查或等价规则验证。

## 背景目标

ExMarketing 核心链路为：

```text
Event Intake
-> Idempotency
-> Campaign Match
-> Rule Evaluation
-> Fact Query
-> Quota / Budget / Inventory
-> Grant Order
-> Outbox
-> Worker Send
-> External Reward System
-> Callback In
-> Quota Confirm / Release
```

已有运维文档定义了 `/metrics`、核心 Prometheus 指标和建议阈值。上一阶段监控任务解决“看得见”，本任务解决“何时叫人、叫谁、如何降噪、如何升级、如何定位”。本轮返工额外明确：告警规则不能依赖不存在或语义不足的指标通过验收；若业务目标要求 DB 写失败和 Fact Query 告警，本任务必须同时声明并验收对应观测契约。

目标：

1. 对六类核心异常形成 P0/P1 告警：DLQ 持续增长、callback 失败率高、outbox backlog 超阈值、worker 全停、DB 核心写失败、quota 超扣风险。
2. 对次级异常形成 P2 告警：event intake 错误率升高、claim latency 升高、retry 激增、Fact Query 失败率高。
3. 定义 P3 观察类告警：低流量偶发失败、轻微延迟、单租户单通道短时 backlog 等。
4. 按业务运营、后端 oncall、DBA/基础设施三个接收组路由。
5. 支持维护窗口静默、重复告警抑制、父子故障抑制和升级。
6. 告警内容必须能直接跳转 dashboard 和 runbook，且不得泄露敏感信息或高基数字段。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| ExMarketing 服务 | 暴露指标并产生 event、rule、reward、quota、outbox、worker、callback、DB 和 Fact Query 相关观测数据。 |
| Fact Provider / 外部事实服务 | 被 Fact Query 调用；其失败、超时、mapping 异常会影响依赖 `facts.*` 的规则判断。 |
| PostgreSQL / Repository | 承载 event inbox、participation、grant_order、quota、outbox、audit、fact provider 等核心状态；核心写失败是 P0 数据一致性风险。 |
| Prometheus 或等价采集系统 | 采集指标并计算告警表达式。 |
| Alertmanager 或等价告警平台 | 分组、路由、静默、抑制、重复通知和升级。 |
| 业务运营 | 接收 P1/P2/P3 中与发奖结果、DLQ 待处理、客户影响相关的告警，执行业务侧沟通和人工处理。 |
| 后端 oncall | 接收活动引擎、outbox、callback、quota、worker、event intake、Fact Query 相关告警并牵头排障。 |
| DBA / 基础设施 | 接收 DB 错误、DB 慢、服务不可用、Prometheus scrape 异常或基础设施依赖故障告警。 |
| Dev Agent | 按需求交付告警规则；如规则依赖指标缺口，补齐最小观测契约或提供等价指标映射。 |
| Test Agent | 验证规则文件静态校验、告警表达式触发、路由标签、模板字段、静默和抑制规则。 |
| 运维 / SRE | 将抽象接收组映射到实际 IM、电话、工单或值班平台，并维护静默窗口。 |

## 输入输出

### 输入

| 输入 | 默认口径 |
| --- | --- |
| 指标来源 | 默认使用 `GET /metrics` 暴露的 Prometheus text format，现有指标清单以 `docs/operations.md` 为准；本任务允许补齐告警必需的 DB write 和 Fact Query 指标契约。 |
| Dashboard 链接 | 默认指向 ExMarketing Grafana dashboard；规则中使用可部署环境替换的 URL 模板，不写死生产私有地址。 |
| Runbook 链接 | 每条告警必须有稳定 runbook URL 或相对路径；默认按告警名映射到运维 runbook。 |
| 通知接收组 | 使用抽象组名：`backend-oncall`、`business-ops`、`dba-infra`、`platform-observability`。 |
| 环境标签 | 至少支持 `service="exmarketing"`、`env` 或等价环境维度；若当前指标未带 `env`，由 Prometheus external labels 或部署层注入。 |
| 业务标签 | 告警可使用 `tenant_id`、`target_channel`、`grant_channel`、`source_system`、`event_type`、`quota_type`、`scope`、`operation`、`table`、`fact_code`、`reason` 等低基数标签。 |

### 输出

必须产出：

1. 告警规则文件或规则文档，按 P0/P1/P2/P3 标注严重级别、表达式、持续时间、路由、summary、dashboard 和 runbook。
2. 通知路由和升级策略说明，明确初始接收组、重复提醒间隔、升级等待时间和升级对象。
3. 告警消息模板，至少包含：

```text
service
env
tenant
channel
severity
alertname
summary
description
dashboard
runbook
owner
first_seen
current_value
threshold
```

4. 抑制规则说明，至少覆盖 DB 故障抑制 outbox/callback/worker/event/Fact Query 泛洪、服务不可用抑制服务内业务指标、P0/P1 抑制同维度 P2/P3。
5. 静默和维护窗口说明，明确哪些标签可用于静默、静默审批字段和默认最大时长。
6. 指标契约与映射表，明确每条告警使用的指标名、标签、聚合维度、阈值、持续时间；现有指标不足时必须标注新增或等价指标。
7. 告警规则静态校验或单元验证说明，至少证明规则语法有效、标签存在、模板字段完整、关键规则能被触发。

## 告警等级定义

| 等级 | 含义 | 响应目标 | 默认通知 |
| --- | --- | --- | --- |
| P0 | 活动引擎核心链路不可用、数据一致性存在立即风险、发奖链路大面积中断或 quota 可能超扣 | 5 分钟内确认，15 分钟内给出止血动作 | 后端 oncall + DBA/基础设施；必要时通知业务运营 |
| P1 | 核心链路局部不可用或明显影响发奖可靠性，已有积压、DLQ 或失败率持续超过阈值 | 15 分钟内确认，30 分钟内给出处理计划 | 后端 oncall，按类型通知业务运营或 DBA/基础设施 |
| P2 | 异常趋势升高但尚未确认造成大面积影响，例如错误率、延迟、retry 明显上升 | 1 小时内确认 | 后端 oncall；业务相关项同步业务运营 |
| P3 | 观察类或低影响异常，用于工作时间排查和趋势治理 | 下一个工作时段处理 | 相关 owner 或观测渠道 |

默认升级：

1. P0 首次通知后 10 分钟未 ACK，升级到后端负责人和基础设施负责人；20 分钟未恢复，通知业务运营负责人。
2. P1 首次通知后 30 分钟未 ACK，升级到对应模块负责人；60 分钟未恢复，升级到后端负责人。
3. P2 连续 2 个统计窗口仍触发且无 ACK，升级为 P1 候选或提醒后端 oncall。
4. P3 不自动电话升级，只保留聚合提醒。

## 指标契约

### 通用标签限制

允许告警标签和 PromQL 聚合使用的低基数标签：

```text
tenant_id
source_system
event_type
result
reason
quota_type
scope
action
target_channel
send_status
grant_channel
callback_status
worker_id
operation
table
phase
fact_code
```

禁止作为可 scrape 指标标签、告警标签或消息上下文：

```text
token
Authorization
Cookie
secret
password
完整 payload
完整 targetRef
eventId
bizId
subjectId
userId
activityId
activity_id
billNo
commandId
idempotencyKey
原始错误文本
```

`reason` 必须是枚举化原因，不能直接使用异常 message。需要定位单个活动、事件、用户或订单时，通过审计、DB 查询、后台查询或脱敏日志处理，不通过告警标签承载。

### DB 核心写失败指标契约

当前 `exmarketing_db_operation_total` 若仅覆盖 metrics 刷新依赖的 outbox/DLQ 查询，只能支撑 `ExMarketingDBOperationError` 的已埋点操作错误告警，不能支撑 `ExMarketingDBWriteFailure`。

默认最小可验收口径：本任务允许并要求补齐或映射以下 DB write metrics。指标名可沿用 `exmarketing_db_operation_total` / `exmarketing_db_operation_latency_seconds`，但必须覆盖核心写路径；若使用其他指标名，必须在交付物中给出等价映射。

| 写路径 | 默认 operation | table | result | reason |
| --- | --- | --- | --- | --- |
| event inbox 写入或状态更新 | `event_inbox_write` | `marketing_event_inbox` | `success` / `failed` | 枚举化 DB 或一致性原因 |
| participation 创建或拒绝记录 | `participation_write` | `marketing_participation` | `success` / `failed` | 枚举化 DB 或一致性原因 |
| grant order 创建或终态更新 | `grant_order_write` | `marketing_grant_order` | `success` / `failed` | 枚举化 DB 或一致性原因 |
| quota reserve | `quota_reserve_write` | `marketing_quota` 或 `marketing_quota_reservation` | `success` / `failed` | `quota_exceeded`、`quota_aggregate_conflict`、`db_error` 等枚举 |
| quota confirm/release | `quota_finalize_write` | `marketing_quota` 或 `marketing_quota_reservation` | `success` / `failed` | `quota_reservation_missing`、`quota_aggregate_conflict`、`db_error` 等枚举 |
| outbox 创建、claim、状态更新 | `outbox_write` | `marketing_outbox` | `success` / `failed` | 枚举化 DB 或状态原因 |
| outbox 进入 DLQ、replay、close | `outbox_dlq_write` | `marketing_outbox` | `success` / `failed` | 枚举化 DB 或状态原因 |
| callback 接收后的 grant/quota 状态更新 | `callback_write` | `marketing_grant_order`、`marketing_quota` 或 `marketing_quota_reservation` | `success` / `failed` | `grant_order_not_found`、`status_conflict`、`quota_reservation_missing`、`quota_aggregate_conflict`、`db_error` 等枚举 |

`ExMarketingDBWriteFailure` 只允许基于上述核心写路径或等价写路径启用。查询类 DB 失败、dashboard 刷新失败、backlog/DLQ 统计查询失败不得伪装为核心写失败 P0，只能进入 `ExMarketingDBOperationError` 或 scrape/metrics 健康告警。

### Fact Query 指标契约

Fact Query 是可选能力，但一旦活动规则引用 `facts.*`，Fact Query 失败会影响规则判断。本任务默认要求 `ExMarketingFactQueryFailRateHigh` 随规则交付启用，因此必须具备以下指标或等价映射：

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_fact_query_total` | Counter | `tenant_id`, `fact_code`, `result`, `reason` | Fact Query 请求结果，包括成功、缓存命中、provider 失败、timeout、mapping 失败、字段缺失、配置缺失。 |
| `exmarketing_fact_query_latency_seconds` | Histogram | `tenant_id`, `fact_code`, `result` | Fact Query 从接收请求到成功返回、缓存命中或失败的耗时。 |

`result` 默认枚举：

```text
success
cache_hit
failed
rejected
timeout
mapping_failed
field_missing
provider_unavailable
```

`reason` 默认枚举至少覆盖：

```text
ok
cache_hit
provider_timeout
provider_http_error
provider_unavailable
provider_response_invalid
fact_mapping_failed
fact_field_missing
fact_provider_disabled
fact_provider_not_found
invalid_fact_query
internal_error
```

规则不得把 provider endpoint、authRef、完整 provider 响应、subjectId、eventId、bizId 或用户属性原文写入标签或告警消息。

## 业务规则

### 告警字段和标签

每条告警必须包含以下标签或注解：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `alertname` | 是 | 稳定告警名，使用 `ExMarketing...` 前缀。 |
| `service` | 是 | 固定为 `exmarketing` 或部署层等价服务名。 |
| `severity` | 是 | `P0` / `P1` / `P2` / `P3`。 |
| `tenant` | 条件必填 | 多租户维度；由 `tenant_id` 映射。跨租户聚合告警可用 `all`。 |
| `channel` | 条件必填 | outbox / callback / worker 场景使用 `target_channel` 或 `grant_channel`；不适用时用 `n/a`。 |
| `owner` | 是 | `backend-oncall`、`business-ops`、`dba-infra` 或 `platform-observability`。 |
| `summary` | 是 | 一句话说明故障影响。 |
| `description` | 是 | 包含当前值、阈值、持续时间和建议第一步排查方向。 |
| `dashboard` | 是 | 指向相关 dashboard 或面板。 |
| `runbook` | 是 | 指向对应处理手册。 |

### 必需告警规则

默认统计窗口与阈值以 `docs/operations.md` 为依据。若实现阶段发现指标名不同，可以用等价指标映射，但业务语义、严重级别、低基数标签和降噪规则不得降低。

| 告警名 | 等级 | 默认触发条件 | 持续时间 | 路由 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `ExMarketingDLQAnyOpen` | P1 | 按 `tenant_id,target_channel` 聚合的 `exmarketing_outbox_dlq_count > 0` | 5m | backend-oncall + business-ops | 任一未关闭 DLQ 都代表发奖指令需要处理。告警标签必须映射 `tenant=tenant_id`、`channel=target_channel`。 |
| `ExMarketingDLQHighByTenantChannel` | P0 | 按 `tenant_id,target_channel` 聚合的未关闭 DLQ `>= 10` 或 10 分钟内持续增长 | 5m | backend-oncall + business-ops | 大量发奖进入 DLQ，可能影响用户权益。不得全局 sum 后丢失租户和通道。 |
| `ExMarketingCallbackFailRateHigh` | P1 | 按 `tenant_id,grant_channel` 聚合，5 分钟 callback `result=~"rejected|failed|conflict"` 失败率 `> 5%` 且失败数 `>= 5` | 5m | backend-oncall + business-ops | 重复 callback 不计失败；conflict 计失败。 |
| `ExMarketingOutboxBacklogHigh` | P1 | 按 `tenant_id,target_channel` 聚合的可投递 backlog `> 100` | 5m | backend-oncall | 发奖指令堆积。表达式必须保留 `tenant_id,target_channel`。 |
| `ExMarketingOutboxOldestEligibleTooOld` | P1 | 按 `tenant_id,target_channel` 的最老可投递 outbox 等待 `> 60s`；`> 120s` 可升级 P0 候选 | 5m | backend-oncall | 未到期 retry 不抬高该值。 |
| `ExMarketingWorkerAllStopped` | P0 | worker claim 成功速率为 0 且同 `tenant_id,target_channel` 存在 eligible backlog，或 worker scrape/up 不健康 | 5m | backend-oncall + dba-infra | worker 全停会导致发奖指令无法投递。 |
| `ExMarketingDBWriteFailure` | P0 | DB 核心写路径 `result="failed"` 在 5 分钟窗口持续增长，覆盖 DB 核心写失败指标契约中列出的写路径 | 1m-5m | dba-infra + backend-oncall | 数据一致性和发奖状态更新存在风险。仅查询指标不足以启用该告警。 |
| `ExMarketingDBOperationError` | P1 | 任意已埋点 DB 操作 `result="failed"` 持续增长但未确认是核心写失败 | 5m | dba-infra + backend-oncall | 可覆盖 metrics 刷新依赖的 outbox/DLQ 查询失败、慢查询或非核心操作错误。 |
| `ExMarketingQuotaOverDeductRisk` | P0 | quota 冻结/确认/释放出现一致性风险、聚合冲突、负数或超出 total 的可观测信号 | 1m | backend-oncall + business-ops | 默认用 `reason=~"quota_aggregate_conflict|quota_reservation_missing|internal_error"` 或等价一致性信号作为最小触发口径。 |
| `ExMarketingQuotaReserveFailRateHigh` | P2 | reserve 失败率 `> 20%` 且失败数 `>= 10` | 5m | backend-oncall + business-ops | 正常额度不足也需可见，但不默认 P1，除非伴随超扣风险或业务大面积投诉。 |
| `ExMarketingEventIntakeFailRateHigh` | P2 | event intake 失败率 `> 5%` 且失败数 `>= 20` | 5m | backend-oncall | payload、鉴权、幂等冲突等错误率升高。 |
| `ExMarketingEventIntakeInternalError` | P1 | `reason="internal_error"` 5 分钟内 `>= 1` | 0m-1m | backend-oncall | 内部错误不等待失败数门槛。 |
| `ExMarketingWorkerClaimLatencyHigh` | P2 | worker claim latency P95 `> 60s`；`> 120s` 可升级 P1 | 5m | backend-oncall | 表示 outbox 从 eligible 到 claim 变慢。 |
| `ExMarketingRetrySpike` | P2 | reward command retry / failed 速率相对过去基线明显升高，最小口径为 5 分钟 failed/retry 数 `>= 20` | 5m | backend-oncall | 用于提前发现外部发奖系统或网络异常。 |
| `ExMarketingFactQueryFailRateHigh` | P2 | 按 `tenant_id,fact_code` 聚合，Fact Query 失败率 `> 5%` 且失败数 `>= 10`，或 `reason="provider_timeout"` 持续增长 | 5m | backend-oncall | 影响依赖 `facts.*` 的规则判断。必须基于 Fact Query 指标契约或等价映射启用。 |
| `ExMarketingScrapeTargetDown` | P1 | `up{job="exmarketing"} == 0` | 2m | platform-observability + backend-oncall | 服务不可抓取时业务告警可能失真。 |

### PromQL 聚合约束

1. DLQ、outbox backlog、oldest eligible age、worker claim latency 必须按 `tenant_id,target_channel` 保留维度，并映射为告警标签 `tenant`、`channel`。
2. Callback 失败率必须按 `tenant_id,grant_channel` 保留维度，并映射为 `tenant`、`channel`。
3. Fact Query 失败率必须按 `tenant_id,fact_code` 保留维度；告警标签 `channel` 使用 `n/a`，并增加 `fact_code` 标签或注解。
4. DB 核心写失败可按 `operation,table` 聚合；若缺少 tenant 维度，`tenant=all`、`channel=n/a`，消息中不得声称单租户影响。
5. 失败率类告警必须同时满足失败率阈值和最小失败数，避免低流量单次失败误报；`internal_error`、DB 核心写失败和 quota 超扣风险除外。

### 路由规则

默认路由按 `severity` 和 `owner` 双维度：

| 条件 | 初始接收组 | 额外接收组 |
| --- | --- | --- |
| P0 且 owner=`backend-oncall` | `backend-oncall` | `business-ops`；涉及 DB 时加 `dba-infra` |
| P0 且 owner=`dba-infra` | `dba-infra` | `backend-oncall` |
| P1 DLQ / callback / quota | `backend-oncall` | `business-ops` |
| P1 DB / target down | `dba-infra` 或 `platform-observability` | `backend-oncall` |
| P2 event / worker / retry / Fact Query | `backend-oncall` | 业务影响明确时加 `business-ops` |
| P3 | 对应 owner 的观察渠道 | 无自动升级 |

分组默认：

```text
group_by: service, env, alertname, tenant, channel, severity
group_wait: 30s
group_interval: 5m
repeat_interval:
  P0: 10m
  P1: 30m
  P2: 2h
  P3: 24h
```

默认约束：

1. 不按 `eventId`、`activityId`、`subjectId`、`userId`、`bizId`、`billNo`、`commandId` 或 `idempotencyKey` 分组。
2. 对跨租户聚合告警，`tenant` 使用 `all`，消息中只展示聚合维度和 top 原因，不列出高基数明细。
3. 同一 `tenant + channel + alertname` 的重复告警必须合并，避免连续刷屏。

### 告警抑制

必须定义以下抑制关系：

| 父告警 | 被抑制告警 | 匹配标签 | 目的 |
| --- | --- | --- | --- |
| `ExMarketingDBWriteFailure` | outbox backlog、callback fail、worker latency、event intake internal error、Fact Query fail 中由 DB 引发的下游告警 | `service`, `env`，可选 `tenant` | DB 核心写失败时避免下游泛洪，但保留 DB 根因告警。 |
| `ExMarketingScrapeTargetDown` | 同服务同环境的业务指标告警 | `service`, `env` | 服务不可抓取时业务指标 No data 不应泛洪。 |
| P0 同名或同根因告警 | 同维度 P1/P2/P3 | `service`, `env`, `tenant`, `channel` | 高等级告警已覆盖时抑制低等级重复通知。 |
| `ExMarketingWorkerAllStopped` | worker claim latency、outbox oldest eligible age 的低等级告警 | `service`, `env`, `channel` | worker 全停是根因，延迟/积压是结果。 |

抑制不得隐藏以下告警：

1. DLQ 已存在或持续增长的事实告警，除非上游 P0 已明确同一根因且消息中包含 DLQ 当前数。
2. Quota 超扣风险 P0。
3. DB 核心写失败 P0。

### 静默和维护窗口

必须支持：

1. 按 `env`、`service`、`tenant`、`channel`、`alertname`、`severity` 建立静默。
2. 维护窗口必须填写 `owner`、`reason`、`start_time`、`end_time`、`scope` 和审批人或申请人。
3. 默认单次静默最长 4 小时；超过 4 小时必须拆分或标注长期维护原因。
4. P0 静默默认不允许只按 `severity=P0` 全局静默；必须限定 `alertname` 或明确维护对象。
5. 生产发布、DB migration、外部发奖系统维护可静默预期告警，但不得静默 quota 超扣风险和已存在 DLQ 的人工处理提醒，除非 runbook 明确已转入人工值守。
6. 静默不会改变告警状态，只影响通知；dashboard 仍需显示异常。

### 告警消息模板

默认模板内容：

```text
[{{ severity }}] {{ alertname }} - {{ summary }}
service={{ service }} env={{ env }} tenant={{ tenant }} channel={{ channel }}
current={{ current_value }} threshold={{ threshold }} duration={{ duration }}
owner={{ owner }}
dashboard={{ dashboard }}
runbook={{ runbook }}
first_seen={{ first_seen }}
```

模板要求：

1. P0/P1 必须包含“影响”和“第一步排查动作”。
2. DB、outbox、callback、quota、Fact Query 告警必须带对应 dashboard 面板链接。
3. 不展示原始 payload、完整 targetRef、凭据、用户 ID、订单号、活动 ID、事件 ID、Fact Provider 响应或原始错误文本。
4. reason 只显示枚举值；需要单点定位时提示查看审计、DB 查询、脱敏日志或 runbook。

## 边界条件

1. 本任务不要求真实部署 Alertmanager；可通过规则文件、规则文档和静态校验完成验收。
2. 若生产使用非 Alertmanager 平台，交付物必须等价表达告警表达式、等级、路由、升级、静默、抑制和模板字段。
3. 本任务允许补齐告警必需的观测指标契约，但不改变 event、quota、grant_order、outbox、callback、Fact Query 的业务状态机。
4. 当前只覆盖查询的 DB operation 指标不能满足 DB 核心写失败 P0 验收；必须补齐核心写路径或明确该 P0 未启用并导致验收失败。
5. Fact Query 告警不能以文档占位通过；必须有 `exmarketing_fact_query_*` 或等价指标支撑。
6. 低流量场景下失败率类告警必须同时满足失败率和最小失败数，避免单次失败造成误报；`internal_error`、DB 核心写失败和 quota 超扣风险除外。
7. 指标无数据时，不能直接等价为业务健康；必须由 `up`、scrape 状态和 No data 说明辅助判断。
8. Counter 服务重启后归零是正常行为；告警必须基于窗口函数或等价趋势计算。
9. 未到期 retry 不计入 oldest eligible age，但 retry 激增需要单独 P2 告警。
10. 多租户、多 channel 场景下必须按低基数维度聚合，不能按单个活动、事件、用户、订单展开告警。
11. 维护窗口静默只影响通知，不应删除、禁用或绕过规则验证。
12. Runbook 链接若暂未有最终文档，允许使用稳定占位路径，但告警名和路径必须一一对应，后续 runbook 任务可补正文。

## 异常场景

| 场景 | 期望表现 |
| --- | --- |
| 服务不可抓取 | 触发 `ExMarketingScrapeTargetDown`，同服务业务指标告警被抑制。 |
| DB 核心写失败 | 触发 P0，路由 DBA/基础设施和后端 oncall；下游 backlog/callback/worker/Fact Query 泛洪告警被抑制。 |
| 只有 DB 查询指标失败 | 触发 `ExMarketingDBOperationError` 或 metrics 刷新相关告警，不得宣称核心 DB 写失败 P0 已覆盖。 |
| DB 慢但未失败 | 触发 P1/P2 DB latency 或 DB operation error 候选，不直接抑制下游。 |
| Worker 全停且 backlog 存在 | 触发 P0 `ExMarketingWorkerAllStopped`，抑制同 channel 的 worker latency 低等级告警。 |
| Outbox backlog 短时升高但很快恢复 | 规则需要持续 5 分钟才通知，避免瞬时峰值误报。 |
| DLQ 出现 1 条 | 按 `tenant_id,target_channel` 触发 P1，通知后端和业务运营，要求人工确认或按 runbook 处理。 |
| DLQ 单 tenant/channel >= 10 | 触发 P0，重复提醒间隔按 P0 执行。 |
| Callback 低流量单次失败 | 面板可见，但未达到失败数门槛时不触发失败率告警；若是内部错误可单独 P1。 |
| Callback conflict 激增 | 计入失败率并触发 P1，提示检查幂等、状态机和外部系统重放。 |
| Quota reserve 因真实额度不足升高 | 默认 P2，通知后端和业务运营；若出现超扣、一致性冲突或负数风险，立即 P0。 |
| Event intake payload 错误升高 | 触发 P2，通知后端 oncall 排查上游接入或字段变更。 |
| Fact Query timeout 升高 | 基于 Fact Query 指标触发 P2，提示检查 provider 可用性、超时、mapping 和缓存命中。 |
| Fact Query 指标缺失 | `ExMarketingFactQueryFailRateHigh` 不得作为已启用规则通过验收；必须补齐指标或等价映射。 |
| 生产维护期外部发奖系统停机 | 可按 `channel` 静默预期 outbox/retry/callback 告警，但不能静默已有 DLQ 人工处理提醒。 |
| 告警模板缺少 dashboard 或 runbook | 验收失败。 |
| 规则包含高基数或敏感标签 | 验收失败。 |

## 非目标范围

1. 不部署真实生产 Prometheus、Grafana、Alertmanager、PagerDuty、短信、电话或公司 IM。
2. 不强绑定某个公司通知工具；只定义抽象接收组和路由语义。
3. 不把所有 WARN 日志转成告警。
4. 不改变 event、quota、grant_order、outbox、callback、Fact Query、ActivityPlugin 或 DB migration 的业务状态机和处理语义。
5. 不通过告警规则修复 DB 写失败、Fact Provider 失败、quota 冲突或 outbox 堆积本身；告警只负责发现、路由和初步定位。
6. 不用高基数标签做告警分组或消息上下文。
7. 不在告警消息中承载单个用户、订单、活动、事件、完整 payload、完整 provider 响应或 provider 凭据。
8. 不替代 dashboard、runbook、日志、审计和 DB 查询；告警只负责发现、路由和初步定位。
9. 不定义完整公司级值班排班制度，只给出本服务默认升级策略。

## 风险点

| 风险 | 影响 | 默认缓解 |
| --- | --- | --- |
| 阈值过低导致告警噪音 | oncall 疲劳，忽略真实事故 | 失败率类必须同时满足失败率和最小失败数；设置 group/repeat interval。 |
| DB 查询指标被误认为核心写失败覆盖 | 生产最关键的数据一致性告警假覆盖 | DB P0 必须绑定核心写路径指标；查询失败只能作为 P1 DB operation error。 |
| Fact Query 无指标但规则占位通过 | P2 告警无法语义触发验证 | Fact Query 指标契约为必需项；缺失时验收失败而非占位通过。 |
| DB 故障引发下游告警风暴 | 通知泛洪，根因不清 | 定义 DB 根因抑制规则。 |
| 静默范围过大 | 真实事故被隐藏 | 限制 P0 全局静默，要求 owner、reason、时间和 scope。 |
| 高基数标签进入规则 | Prometheus/Alertmanager 压力升高，消息不可控 | 禁止高基数字段作为标签或 group_by。 |
| 告警缺少 runbook/dashboard | oncall 无法快速处理 | 模板和验收强制检查链接字段。 |
| 抽象接收组未映射实际工具 | 生产通知不可达 | 本任务输出接收组契约，部署前由运维映射，作为上线 checklist。 |

## 验收标准

必须全部满足：

1. DLQ、callback、outbox、worker、DB、quota 六类核心异常都有 P0/P1 或明确等级的告警规则。
2. DB 核心写失败 P0 必须基于 DB write metrics 契约或等价指标，至少覆盖 event inbox、participation、grant_order、quota reserve/finalize、outbox、DLQ 操作和 callback 状态更新中的核心写路径；只覆盖 outbox/DLQ 查询的 DB operation 指标不得通过该项。
3. P2 覆盖 event intake 错误率升高、claim latency 升高、retry 激增、Fact Query 失败率高。
4. Fact Query P2 必须基于 `exmarketing_fact_query_total`、`exmarketing_fact_query_latency_seconds` 或等价指标启用，且能用失败率、失败数或 timeout 增长触发验证；无指标占位不得通过。
5. DLQ、outbox backlog、oldest eligible age、worker claim latency 的 PromQL 或等价表达式必须保留 `tenant_id,target_channel`，并映射为 `tenant,channel` 告警标签。
6. Callback 失败率必须保留 `tenant_id,grant_channel`；Fact Query 失败率必须保留 `tenant_id,fact_code`。
7. 每条告警都有 `service`、`tenant` 或聚合 `all`、`channel` 或 `n/a`、`severity`、`summary`、`dashboard`、`runbook`。
8. 告警路由至少覆盖 `business-ops`、`backend-oncall`、`dba-infra` 三类接收方。
9. 有 P0/P1/P2/P3 等级定义和升级策略。
10. 有维护窗口和静默规则，且明确 P0 全局静默限制。
11. 有重复告警分组和重复提醒间隔设置。
12. 有 DB 故障抑制下游泛洪告警的规则，且不得抑制 DB 核心写失败 P0、quota 超扣风险 P0 和必要 DLQ 总览。
13. 告警消息模板不包含敏感信息、高基数字段、完整 provider 响应或原始错误文本。
14. 规则文件可通过静态校验，例如 `promtool check rules` 或等价工具；若为文档化规则，必须提供可人工审核的表达式、阈值、持续时间和标签。
15. 单元验证或规则验证至少覆盖：DLQ > 0、DLQ 单 tenant/channel >= 10、callback 失败率高、outbox backlog 高、worker 全停、DB 核心写失败、DB 查询/operation 错误、quota 超扣风险、Fact Query 失败率或 timeout、DB 抑制下游告警。
16. 与 `docs/operations.md` 的指标名、标签约束和阈值建议保持一致；若使用新增或等价指标，必须在交付物中列出映射和差异原因。

## 待确认问题

以下问题不阻塞本需求进入研发，均已给出默认最小可验收口径：

| 问题 | 默认口径 |
| --- | --- |
| 生产实际使用 Alertmanager 还是其他告警平台？ | 默认交付 Alertmanager 兼容规则；其他平台必须等价表达规则、路由、静默、抑制和模板。 |
| 具体 IM / 电话 / 工单系统是什么？ | 不强绑定；使用抽象接收组，部署阶段映射实际工具。 |
| P0 是否必须电话通知？ | 需求层默认 P0 需要强提醒和升级，但不指定电话工具。 |
| Runbook 最终 URL 是什么？ | 默认使用稳定占位路径或相对路径，必须与告警名一一对应。 |
| 本任务是否允许新增指标？ | 允许新增或扩展告警必需的观测指标契约，特别是 DB 核心写失败和 Fact Query；不允许改变业务状态机。 |
| Fact Query 告警是否必须随本任务启用？ | 必须启用，前提是同时补齐最小指标契约或等价映射；无指标占位不能通过验收。 |

## Score

```text
93
```

评分拆分：

```text
需求清晰度        19/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      19/20
约束一致性        17/20
```

扣分原因：生产通知工具、最终 runbook URL 和具体告警平台仍依赖部署环境映射；DB write 与 Fact Query 需要 Dev/Test 在实现验收中补齐或映射观测指标，但本分析已给出默认最小可验收契约，不构成 BLOCK。

## Decision

```text
PASS
```

需求：2026-05-29-alerts-rules-routing
