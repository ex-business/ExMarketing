# Analysis: 2026-05-29-alerts-prometheus-grafana-prometheus-grafana

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未写业务代码，未提交，未设计具体代码改法。

已读取：

- `tasks/2026-05-29-alerts-prometheus-grafana-prometheus-grafana/review.md`
- `tasks/2026-05-29-alerts-prometheus-grafana-prometheus-grafana/analysis.md` 上一版
- `tasks/2026-05-29-alerts-prometheus-grafana-prometheus-grafana/task.md`
- `docs/api.md`
- `docs/operations.md`
- 相关历史任务材料：`tasks/2026-05-27-engine-metrics-alerts/analysis.md`

未发现独立 PRD 目录。本分析以任务说明、已有 API 文档、运维文档和历史 metrics 任务口径为依据。

## Review 反馈处理

| Review 项 | 处理 |
| --- | --- |
| P1-1：高基数标签治理没有形成可验收闭环，尤其 `activity_id` 仍可能被 Prometheus scrape。 | 已修正为强约束：本任务验收范围内所有新增或保留的可 scrape 指标都不得暴露 `activity_id`、`activityId`、`subjectId`、`eventId`、`bizId`、`userId`、`billNo`、`commandId`、完整 payload、原始错误文本等禁止标签。`docs/operations.md` 现有 quota 指标包含 `activity_id`，因此 dashboard 验收前必须迁移或补充低基数 quota 替代指标，并确保 Prometheus scrape 输出中不再暴露该标签。仅在 dashboard 不展开或文档提示风险不再可接受。 |
| P1-2：Rule/reward 与 DB 覆盖被降级为“待接入面板”。 | 已修正为强验收：Rule/reward 与 DB 属于本任务必需覆盖范围，不能只交付占位面板。dashboard 可以在无业务流量时显示 0 或 No data，但必须有真实可 scrape 指标或等价低基数指标支撑规则执行量、失败量、阶段耗时、奖励执行量、失败量、阶段耗时、DB 错误和慢操作/延迟。 |
| P2-1：健康阈值没有在面板和验收中统一使用“失败率 + 最小失败数”。 | 已修正为强约束：event、quota、callback 的失败率类面板和 Red/Yellow 判定必须同时使用失败率阈值和最小失败数门槛，避免低流量误报。验收标准中明确要求 PromQL 或面板定义体现该双条件。 |
| Open Question 1：是否必须补齐 rule、reward、DB 指标实现，还是只交付 dashboard 占位？ | 默认最小可验收口径：必须补齐可观测指标语义和 dashboard 真实数据支撑。原因是 `task.md` 背景和范围明确要求覆盖规则执行和数据库，Analysis Agent 不能自行降级范围。 |
| Open Question 2：现有 quota `activity_id` 是否允许继续被 scrape？ | 默认最小可验收口径：不允许。原因是 `task.md` 明确禁止 activityId/subject 等高基数字段直接打标签，且 Prometheus scrape 会采集完整时间序列，不能靠 dashboard 规避。 |
| Score 72 / Decision BLOCK | 已按上述三类阻塞点重写需求边界、输入输出、业务规则、验收标准和非目标范围，重新评分并给出 `Decision PASS`。 |

## Analysis 自检修正

上一版 `analysis.md` 虽给出 `Score 92` 和 `Decision PASS`，但 Review 证明存在错误放宽和错误假设。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| 错误假设：历史 `activity_id` 指标可作为兼容输入，只要 dashboard 不展开即可。 | 已关闭。Prometheus 采集层也必须治理高基数；验收要求可 scrape 指标不得暴露禁止标签。 |
| 错误假设：Rule/reward 与 DB 指标缺失时可交付“待接入面板定义”。 | 已关闭。任务范围明确包含 Rule/reward 和 DB，必须具备真实指标或等价已文档化指标支撑。 |
| 扣分项：健康阈值与 `docs/operations.md` 不一致。 | 已关闭。失败率类健康判断统一采用 5 分钟窗口下“失败率 + 最小失败数”双条件。 |
| 待确认问题：生产 Prometheus/Grafana 部署位置。 | 不阻塞。默认本任务只交付 scrape 样例、dashboard 和本地/测试验证说明，不部署真实生产 Prometheus/Grafana。 |
| 待确认问题：Grafana datasource 名称。 | 不阻塞。默认使用 datasource 变量或 `Prometheus` 占位，导入时绑定实际数据源。 |

## 需求说明

本需求要求为 ExMarketing 活动引擎提供 Prometheus 与 Grafana 的可接入交付物，使本地、测试或生产运维环境能够通过 Prometheus scrape 服务指标，并通过 Grafana dashboard 快速判断活动引擎全链路健康状态。

最小可验收交付为：

1. Prometheus scrape 配置样例或等价接入说明。
2. Grafana dashboard JSON，或可直接复用的面板定义文档。
3. 指标命名、标签、单位、统计窗口、PromQL 查询、阈值和高基数限制说明。
4. 本地验证说明，能证明 `/metrics` 或等价 endpoint 可被 scrape，dashboard 所需核心查询能运行。
5. Rule/reward、quota、DB 等任务范围内指标必须有真实可 scrape 数据源，不能只交付占位面板。

## 背景目标

活动引擎链路包含：

```text
Event Intake
-> Rule / Reward
-> Quota
-> Grant Order / Outbox
-> Worker Send
-> External Reward System
-> Callback
-> DB State
```

已有运维文档定义了 `/metrics`、核心指标和建议阈值，但当前缺口是 Prometheus 如何采集、Grafana 如何组织面板，以及运维人员如何通过一个 dashboard 判断 event、rule/reward、quota、outbox、callback、worker 和 DB 是否健康。

目标：

1. 让 Prometheus 能按固定 scrape 配置抓取 ExMarketing 指标。
2. 让 Grafana dashboard 覆盖 event、rule/reward、quota、outbox、DLQ、callback、worker 和 DB 八类健康信号。
3. 指标标签必须控制基数，不把用户、事件、订单、活动实例、完整业务对象或敏感信息作为直接标签。
4. 面板必须支持快速判断：是否有事件接入失败、规则/奖励异常、quota 拒绝异常、outbox 堆积、DLQ 增长、callback 失败、worker 延迟或 DB 错误/慢操作。
5. 本地开发者能按文档启动服务、访问 metrics、配置 Prometheus scrape，并导入 dashboard 验证。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| ExMarketing 服务 | 暴露 `/metrics` 或等价 Prometheus text endpoint，提供活动引擎健康指标。 |
| Prometheus | 按 scrape 配置周期性采集 ExMarketing 指标。 |
| Grafana | 使用 Prometheus 数据源展示 dashboard 和健康面板。 |
| Event 来源系统 | 产生 event intake 成功、失败、重复、拒绝等指标输入。 |
| 活动规则与奖励引擎 | 产生规则执行、奖励计算、阶段耗时和失败指标输入。 |
| Quota 子系统 | 产生预留、确认、释放、失败率、冻结量、释放量、确认量等指标输入。 |
| Outbox 与 Worker | 产生 backlog、DLQ、retry、claim latency、发送耗时和 in-flight 指标输入。 |
| Callback 处理方 | 产生 callback 成功、失败、重复、冲突和处理延迟指标输入。 |
| DB / Repository 层 | 产生 DB 错误、慢操作、操作耗时，或由仓储状态刷新出的 backlog / DLQ Gauge。 |
| 运维 / SRE | 导入 dashboard、配置 scrape、基于面板判断链路健康和定位故障。 |
| Dev / Test Agent | 按需求补齐配置、dashboard、指标规范、文档和本地验证。 |

## 输入输出

### 输入

| 输入 | 默认口径 |
| --- | --- |
| Metrics endpoint | 默认 `GET /metrics`，返回 Prometheus text format；若服务地址不同，scrape 样例必须明确 `host`、`port`、`metrics_path`。 |
| Prometheus 数据源 | Grafana dashboard 默认使用 Prometheus datasource，占位名建议为 `${DS_PROMETHEUS}` 或 `Prometheus`，不得写死生产 URL。 |
| scrape target | 本地默认 `localhost:8080`，生产只提供示例，不要求部署。 |
| 指标清单 | 以 `docs/operations.md` 已列出的核心指标为基础，并补足 rule/reward、DB 和低基数 quota 替代指标的最小口径。 |
| 采集窗口 | 默认 scrape interval `15s`，面板默认使用 5 分钟 rate / increase / histogram 窗口；可在文档中说明可调整。 |
| 标签约束 | 只允许低基数业务维度和状态枚举；禁止敏感和高基数字段。 |

### 输出

必须产出：

1. Prometheus scrape 配置样例，至少包含 job name、metrics path、scrape interval、static target 和本地验证 target。
2. Grafana dashboard JSON 或文档化面板定义。若交付 JSON，必须可导入 Grafana；若交付面板定义，必须包含每个面板标题、PromQL、单位、阈值和用途。
3. 指标命名与标签规范，明确允许标签、禁止标签、高基数字段处理方式、敏感信息处理方式。
4. 本地验证说明，至少覆盖启动服务、访问 `/metrics`、启动或配置 Prometheus、在 Grafana 导入 dashboard、确认核心面板查询可执行。
5. dashboard 健康判读说明，明确 green/yellow/red 规则和需要排查的异常类型。

## 业务规则

### Prometheus scrape

1. scrape 目标默认使用 ExMarketing 服务的 `/metrics`。
2. scrape 配置必须是样例性质，不包含生产账号、token、公网地址、公司内网机密域名或 secret。
3. 默认 job 名称使用 `exmarketing` 或等价稳定名称，便于 dashboard PromQL 过滤。
4. scrape interval 默认 `15s`；对于本地验证可接受 `15s` 到 `30s`。
5. metrics endpoint 不应要求带 Authorization、Cookie 或用户态 token；若生产未来需要网关保护，本任务只说明由内网或基础设施层限制访问，不在指标标签或配置中写入密钥。
6. Counter 为进程生命周期累计值，服务重启后可从 0 开始；dashboard 必须用 `rate()` 或 `increase()` 观察窗口趋势，不用 Counter 绝对值直接判断健康。
7. Gauge 类 backlog / DLQ 应展示当前值；若抓取失败或无数据，dashboard 应有单独面板暴露 scrape `up` 状态。

### 必需指标覆盖

本任务的 dashboard 不允许以“待接入”占位代替以下必需覆盖。若现有指标名不同，可在文档中建立等价映射，但语义、类型、标签基数和面板能力必须满足。

| 范围 | 必需观测能力 | 默认指标口径 |
| --- | --- | --- |
| Event intake | 成功/失败/重复/拒绝速率，失败率，P95 延迟，错误原因分布 | `exmarketing_event_intake_total`、`exmarketing_event_intake_latency_seconds` |
| Rule | 规则执行量、失败量、阶段耗时、失败原因 | `exmarketing_rule_execution_total`、`exmarketing_rule_stage_latency_seconds` |
| Reward | 奖励计算/生成量、失败量、阶段耗时、失败原因 | `exmarketing_reward_execution_total`、`exmarketing_reward_stage_latency_seconds` |
| Quota | reserve/confirm/release 尝试量、失败量、失败率、失败原因、冻结/释放/确认量 | 低基数 quota 指标，不得包含 `activity_id`；默认为 `exmarketing_quota_attempt_total`、`exmarketing_quota_fail_total` 或等价低基数替代指标 |
| Outbox | backlog by status、最老 eligible age、retry rate、send latency | `exmarketing_outbox_backlog`、`exmarketing_outbox_oldest_eligible_age_seconds`、`exmarketing_reward_command_total`、`exmarketing_reward_command_latency_seconds` |
| DLQ | 当前未关闭 DLQ、进入 DLQ 趋势、原因分布 | `exmarketing_outbox_dlq_count`、`exmarketing_outbox_dlq_total` |
| Callback | 成功率、失败率、重复回调、冲突数、P95 处理耗时 | `exmarketing_reward_callback_total`、`exmarketing_reward_callback_latency_seconds` |
| Worker | claim 成功/空批次/失败/冲突、claim latency P95、in-flight | `exmarketing_worker_claim_total`、`exmarketing_worker_claim_latency_seconds`、`exmarketing_worker_inflight` |
| DB | DB 操作量、错误数、慢操作数或慢操作延迟、P95/P99 操作耗时 | `exmarketing_db_operation_total`、`exmarketing_db_operation_latency_seconds` 或等价 DB 指标 |

### Grafana dashboard

dashboard 至少包含以下分组：

| 分组 | 必需面板 | 默认 PromQL 口径 |
| --- | --- | --- |
| Overview | `up`、总事件速率、event 失败率、quota reserve 失败率、DLQ 当前数、outbox 最老等待、worker in-flight、DB 错误趋势 | 使用 `up{job="exmarketing"}`、核心 Counter rate、Gauge 当前值。 |
| Event intake | 成功/失败/重复/拒绝速率，失败率，P95 延迟，错误原因分布 | 基于 `exmarketing_event_intake_total` 与 `exmarketing_event_intake_latency_seconds`。 |
| Rule / reward | 规则/奖励执行量、失败量、阶段耗时、失败原因 | 基于真实 rule/reward 指标；不得以空面板作为通过依据。 |
| Quota | reserve 失败率、冻结量、释放量、确认量、失败原因 | 基于低基数 quota 指标；不得包含或 group by `activity_id`。 |
| Outbox / DLQ | backlog by status、最老 eligible age、DLQ count、retry rate、send latency | 基于 outbox、DLQ、reward command 指标。 |
| Callback | 成功率、失败率、重复回调、冲突数、P95 处理耗时 | 基于 callback counter 和 latency histogram。 |
| Worker | claim 成功/空批次/失败/冲突、claim latency P95、in-flight | 基于 worker counter、latency histogram 和 in-flight gauge。 |
| DB | DB 错误数、慢操作数、操作耗时 P95/P99、按低基数 operation/table 聚合 | 基于真实 DB 指标；不得以空面板作为通过依据。 |

### 健康颜色与阈值

失败率类判断统一使用 5 分钟窗口，并同时满足失败率阈值和最小失败数才进入异常状态。失败数建议用 `increase(...[5m])`，失败率建议用 `rate(...[5m])` 或 `increase(...[5m])` 的分子/分母计算。

| 状态 | 口径 |
| --- | --- |
| Green | `up=1`；event 失败率 <= 5% 或 5 分钟失败数 < 20；quota reserve 失败率 <= 20% 或 5 分钟 reserve 失败数 < 10；callback 失败率 <= 5% 或 5 分钟失败数 < 5；DLQ count = 0；outbox oldest eligible age <= 60s；worker claim P95 <= 60s；DB 错误无持续增长且慢操作未超过文档阈值。 |
| Yellow | 任一指标超过 Green 阈值但未达到 Red；或核心面板短时 No data 但 scrape `up=1`；或 outbox oldest eligible age 在 60s 到 120s 之间；或 DB 慢操作开始增长但未持续。 |
| Red | `up=0`；DLQ count > 0；单 tenant/channel DLQ >= 10；outbox oldest eligible age > 120s；event `internal_error` 5 分钟内 >= 1；event 失败率 > 5% 且 5 分钟失败数 >= 20；quota reserve 失败率 > 20% 且 5 分钟失败数 >= 10；callback 失败率 > 5% 且 5 分钟失败数 >= 5；worker claim P95 > 120s；DB 错误持续增长或 DB 操作 P95/P99 明显超过文档阈值。 |

### 指标和标签规范

允许的低基数标签：

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
```

禁止作为可 scrape 指标标签：

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

默认约束：

1. `tenant_id` 可保留，因为租户数量预期受控；如单环境租户数超过 100，需要在文档中建议按租户聚合或限制 scrape 范围。
2. `worker_id` 必须是低基数稳定实例标识，不能包含随机 UUID、pod UID、进程启动时间戳等无限增长内容。
3. `reason` 必须使用枚举化原因，不能直接放异常消息。
4. quota 指标必须去除 `activity_id` / `activityId` 维度，默认按 `tenant_id`、`quota_type`、`scope`、`action`、`result`、`reason` 聚合；如需要定位单活动问题，应使用日志、审计、DB 查询或后台查询页，不通过 Prometheus 标签承载。
5. PromQL 查询不得 `group by` 或筛选禁止标签；metrics endpoint 输出也不得包含禁止标签。

## 边界条件

1. 本地没有运行 Prometheus / Grafana 时，交付仍可通过配置文件和 dashboard JSON 静态检查，以及 `/metrics` curl 验证。
2. `/metrics` 无业务流量时，Counter rate 可能为 0；dashboard 应展示 `up` 和 Gauge 面板，不把 0 流量误判为服务故障。
3. 服务重启后 Counter 清零是正常行为；dashboard 使用窗口函数避免误报。
4. PostgreSQL 未启用或使用内存仓储时，outbox / DLQ / DB 面板可能没有完整业务数据；强验收以 PostgreSQL 环境为准。
5. 多租户、多 channel、多 worker 场景下 dashboard 必须提供变量或查询过滤方式，但不得默认展开高基数维度。
6. Grafana datasource 不得硬编码本机绝对路径或生产私有地址。
7. scrape 配置不负责 TLS、反向代理、服务发现、Kubernetes ServiceMonitor 等生产基础设施细节。
8. 若现有历史指标仍包含禁止标签，本任务验收失败；必须以低基数替代指标作为 dashboard 和 scrape 验收对象。

## 异常场景

| 场景 | 期望表现 |
| --- | --- |
| ExMarketing 服务未启动 | Prometheus `up=0`，Overview 显示 Red。 |
| `/metrics` 返回非 2xx 或格式错误 | Prometheus target unhealthy，本地验证失败。 |
| Event intake 大量失败 | Event 面板失败率升高；只有失败率超过阈值且失败数达到最小门槛才触发 Red。 |
| Event intake 低流量单次失败 | 面板可见失败，但不因失败率 100% 直接 Red，除非是 `internal_error` 或达到最小失败数。 |
| Rule 执行异常 | Rule 面板展示执行失败量、失败原因和阶段耗时升高，Overview 可进入 Yellow/Red。 |
| Reward 计算或生成异常 | Reward 面板展示失败量、失败原因和阶段耗时升高。 |
| Quota 不足或扣减异常 | Quota reserve fail rate 面板升高，按 `quota_type`、`scope`、`reason` 聚合，不按 activity 展开。 |
| Outbox 堆积 | backlog count 和 oldest eligible age 上升，Overview 进入 Yellow / Red。 |
| DLQ 增长 | DLQ count > 0 显示 Red，DLQ total 可观察新增趋势。 |
| Callback 重复或冲突 | Callback 面板能区分 duplicate、conflict、failed、rejected；失败率判断排除正常 duplicate，冲突按失败类处理。 |
| Worker 停止或 claim 变慢 | Worker claim rate 降低、claim latency P95 升高、in-flight 异常。 |
| DB 慢或错误 | DB 面板展示错误、慢操作或延迟 P95/P99；核心 DB 指标缺失时验收失败。 |
| 高基数标签误用 | 验收失败；必须从 metrics 输出、dashboard 查询和标签规范中移除。 |
| 指标暴露敏感信息 | 验收失败；Prometheus 配置、dashboard、文档和 metrics 输出不得包含 token、secret、用户隐私。 |

## 非目标范围

1. 不部署真实生产 Prometheus、Grafana、Alertmanager 或远程存储。
2. 不接入公司 IM、短信、电话、PagerDuty、Webhook 通知。
3. 不设计生产级服务发现、Kubernetes Operator、ServiceMonitor、TLS 证书或鉴权网关。
4. 不新增或改变业务规则、发奖逻辑、quota 扣减语义、outbox 投递语义或 callback 状态机。
5. 不把日志平台、链路追踪、APM、profiling、OpenTelemetry collector 作为本任务必交付。
6. 不提供按单个 `eventId`、`activityId`、`subjectId`、`userId`、`bizId` 查询的 Grafana 维度；这些定位需求由日志、审计、DB 或后台查询承担。
7. 不要求 dashboard 覆盖所有 SQL 表和所有 API，只覆盖任务范围内判断链路健康所需的核心面板。

## 风险点

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 现有 quota 指标包含 `activity_id` 标签 | Prometheus 会采集高基数时间序列，违背任务目标 | 本任务验收要求迁移或补充低基数 quota 替代指标；旧高基数指标不得作为可 scrape 验收对象。 |
| Rule/reward 或 DB 指标当前不足 | dashboard 无法真实覆盖任务范围 | 本任务不允许用占位面板通过验收；必须补齐真实指标或等价指标映射后再验收。 |
| PromQL 阈值不适合所有租户流量 | 低流量误报或高流量漏报 | 默认阈值使用“失败率 + 最小失败数”双条件，并允许后续按租户基线调参。 |
| Grafana datasource 名称不一致 | 导入后查询不可用 | dashboard 使用 datasource 变量或文档要求导入时绑定 Prometheus 数据源。 |
| 本地验证没有真实异常流量 | 部分失败面板无数据 | 验证说明应包含至少一次成功事件、一次失败/拒绝场景，以及 outbox / DLQ / DB 可观察数据的最小生成方式或说明。 |
| 指标 endpoint 暴露公网 | 信息泄露 | 文档明确 `/metrics` 默认仅内网/本地采集，不建议直接公网暴露。 |

## 验收标准

必须全部满足：

1. 存在 Prometheus scrape 配置样例或接入说明，能指向本地 ExMarketing `GET /metrics` 或等价 endpoint。
2. 本地执行 `curl http://localhost:8080/metrics` 或文档指定等价命令，能得到 Prometheus text format 指标输出。
3. `/metrics` 输出中，本任务验收范围内的新增或保留可 scrape 指标不得包含 `activity_id`、`activityId`、`eventId`、`subjectId`、`userId`、`bizId`、`billNo`、`commandId`、完整 payload、原始错误文本或敏感标签。
4. 现有 quota 观测必须提供低基数替代指标或迁移后指标，dashboard 不得依赖包含 `activity_id` 的 quota 指标。
5. 存在 Grafana dashboard JSON 或文档化面板定义，覆盖 event、rule、reward、quota、outbox、callback、DLQ、worker、DB 九类视图；Rule/reward 和 DB 不得只标注“待接入”或空面板。
6. Dashboard Overview 能一眼判断链路健康，至少包含 `up`、event failed rate、quota reserve fail rate、outbox backlog / oldest age、DLQ count、callback failed rate、worker claim latency、DB error/slow operation。
7. event、quota、callback 的失败率类 PromQL 或面板定义必须体现“失败率 + 5 分钟最小失败数”双条件：event 失败率 > 5% 且失败数 >= 20；quota reserve 失败率 > 20% 且失败数 >= 10；callback 失败率 > 5% 且失败数 >= 5。
8. PromQL 查询不按禁止的高基数标签聚合，不筛选禁止标签，不把禁止标签作为 dashboard 变量。
9. 配置、dashboard、文档和 metrics 示例不包含 secret、token、Authorization、Cookie、密码、真实生产凭据或用户隐私。
10. 本地验证说明能让 Dev/Test Agent 复现：启动服务、抓取 metrics、配置 Prometheus、导入 Grafana、确认核心面板查询可执行。
11. 文档明确 Counter 重启清零、Gauge 当前值、默认 5 分钟窗口、默认 scrape interval、强验收以 PostgreSQL 环境为准的限制。
12. `go test ./...` 通过；如无法执行，必须在测试产物中说明阻塞原因和影响范围。

## 待确认问题

以下问题不阻塞本轮进入研发，均已有默认最小可验收口径：

| 问题 | 默认口径 |
| --- | --- |
| 生产 Prometheus / Grafana 部署在哪里？ | 本任务不部署生产实例，只提供本地/测试可用配置和 dashboard。 |
| Grafana datasource 名称是什么？ | 默认使用 datasource 变量或 `Prometheus`，导入时绑定实际数据源。 |
| rule/reward 与 DB 指标是否已全部存在？ | 不等待确认。任务范围要求覆盖，因此必须补齐真实指标或文档化等价指标映射后验收。 |
| `activity_id` 历史 quota 指标如何处理？ | 不允许作为可 scrape 验收指标继续暴露；必须迁移或补充低基数替代指标，dashboard 使用替代指标。 |
| 告警是否需要接入 IM？ | 不需要。本任务只交付 dashboard、Prometheus 接入说明和可用于后续告警的阈值口径。 |

## Score

```text
94
```

评分：

| 维度 | 分数 | 说明 |
| --- | ---: | --- |
| 需求清晰度 | 19/20 | 交付物、目标、范围和本地/测试接入口径清楚；生产部署细节按非目标处理。 |
| 业务规则完整 | 19/20 | scrape、dashboard、必需指标、标签治理、健康阈值和双条件失败率规则明确。 |
| 边界异常清楚 | 19/20 | 覆盖无流量、重启、缺失指标、高基数、敏感信息、服务不可用、DB 慢/错等场景。 |
| 验收标准明确 | 19/20 | 可按配置、metrics 输出、dashboard、PromQL、标签和本地验证逐项审核。 |
| 约束一致性 | 18/20 | 已纠正上一版与 task / operations 的冲突；保留生产部署与通知集成作为非目标。 |

## Decision

```text
PASS
```

需求：2026-05-29-alerts-prometheus-grafana-prometheus-grafana
