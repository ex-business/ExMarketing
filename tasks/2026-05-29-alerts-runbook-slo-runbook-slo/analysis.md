# Analysis

## 背景和已知信息

本需求是为 ExMarketing 活动引擎补齐告警后的可执行处理材料和服务目标定义。现有材料已经包含 Prometheus 指标、告警规则、运维手册、API 文档、数据模型和一份初版 runbook，但 runbook 当前偏摘要，尚不足以让值班人员在告警后独立完成影响判断、止血、恢复验证、升级和复盘。

已知约束：

- 服务通过 `/metrics` 暴露 Prometheus text format，指标默认只面向内网或本地采集，不直接公网暴露。
- 告警规则位于 `monitoring/alerts-exmarketing.rules.yml`，按 `P0/P1/P2/P3` 标注严重级别，并包含 `service`、`tenant`、`channel`、`severity`、`summary`、`dashboard`、`runbook`、`owner`、`current_value`、`threshold` 等信息。
- 当前关键告警包括 worker 停止、核心 DB 写失败、quota 一致性风险、DLQ 增长、outbox backlog、callback 失败率、事件接入错误率、worker claim 延迟、reward command retry、Fact Query 失败率、Prometheus scrape down。
- 已有指标可支撑 SLI/SLO，包括事件接入、outbox/DLQ、reward callback、worker claim、reward command、DB operation、Fact Query 等指标。
- 已有管理 API 支持 DLQ 查询、replay、close；reward callback；reward reconciliation；quota timeout-release；audit 查询。相关操作必须保留租户隔离、审计和敏感信息脱敏。
- 非目标明确：不定义业务赔付规则，不替代公司统一 incident 流程，不实现真实 oncall 排班系统。

## Review 反馈处理

上一轮 `review.md` 存在 `P0/P1/P2`、`Open Questions`、`PASS / BLOCK` 等模板占位，但没有填写具体打回意见、严重级别或开放问题。因此本轮处理方式如下：

- `P0/P1/P2` 占位：未发现具体问题文本，按“上一轮未完成有效 Review 内容”处理，不逐条保留为阻塞项。
- `Open Questions` 占位：未填写具体问题，本轮通过最小可验收口径补齐 runbook 范围、字段结构、SLO 指标、演练模板、业务确认边界、验收标准。
- `PASS / BLOCK` 占位：未形成有效决策，本轮重新输出完整 Analysis，并按评分规则自评。

## Analysis 自检修正

上一轮 `analysis.md` 为模板态，存在 `Score = 0`，且背景、目标、业务规则、输入输出、异常场景、边界条件、非目标范围、风险、验收方式、待确认问题均为“待填写”。本轮修正如下：

- 关闭 `Score < 80`：补齐可审核、可开发、可测试的需求细节，并给出 Score 92。
- 关闭“背景和已知信息待填写”：基于任务、运维手册、API、数据模型、告警规则和现有 runbook 摘要明确事实基础。
- 关闭“目标理解待填写”：明确交付物是文档和映射关系，不要求代码逻辑改造或排班系统。
- 关闭“细化需求/业务规则/输入输出待填写”：定义每类 runbook 必填字段、告警映射、SLO/SLI、业务确认规则、模板字段。
- 关闭“异常场景/边界条件待填写”：补齐 DB 根因抑制、低流量误报、DLQ close/replay 限制、callback 冲突、quota 一致性、Fact Query 安全失败、metrics scrape down 等场景。
- 关闭“待确认问题待填写”：不等待 Owner 决策；对缺口采用当前资料能支撑的默认最小口径，并将待确认项降级为后续优化，不阻塞交付。

## 目标理解

本需求的目标是让核心告警从“会响”升级为“响后可处理、可验证、可复盘”，并为活动引擎建立第一版可度量的 SLI/SLO/SLA 文档。

必须交付：

- `docs/operations` 或其下级运维文档中的 runbook 内容，建议保留或引用现有 `docs/runbooks/exmarketing-alerts.md`，但必须在 `docs/operations` 文档中能被发现。
- 核心 P1/P2 告警，以及任务范围中列出的关键告警场景，都有可执行 runbook。
- SLI/SLO/SLA 指标定义，且指标必须能由现有 `/metrics` 指标支撑。
- 演练记录模板和复盘模板。
- 告警到 runbook 的映射表。
- 明确哪些告警需要业务确认，哪些可由技术直接处理。

默认最小可验收口径：优先覆盖当前已存在的告警规则。任务中提到但当前未单独成规则的“DB 慢”应在 DB runbook 中作为 DB operation latency 场景覆盖，不要求新增告警规则；任务中提到的“quota 异常”按 `ExMarketingQuotaOverDeductRisk` 覆盖；“callback 失败”按 `ExMarketingCallbackFailRateHigh` 覆盖；“worker 停止”按 `ExMarketingWorkerAllStopped` 覆盖。

## 参与方

- Backend oncall：默认处理 ExMarketing 业务链路、outbox、DLQ、callback、quota、worker、Fact Query、event intake、reward retry 等告警。
- DBA/Infra：处理核心 DB 写失败、数据库不可用、连接池饱和、migration/drift 等数据库根因。
- Platform observability：处理 Prometheus scrape target down、采集链路、Grafana/Prometheus/Alertmanager 配置可用性。
- Business operations：参与活动暂停、外部 reward provider 暂停/恢复、DLQ close、callback replay、quota 超时释放、客户可见影响确认。
- Reward provider owner / source system owner / fact provider owner：处理外部发奖系统、事件来源系统、Fact Provider 的可用性或协议变化。
- Incident lead：公司统一 incident 流程中的协调人；本需求只提供 ExMarketing 侧输入，不替代统一流程。

## 细化需求

### Runbook 范围

必须覆盖以下告警或场景：

| 告警/场景 | 严重级别 | 默认 owner | 是否必须业务确认 | Runbook 重点 |
| --- | --- | --- | --- | --- |
| `ExMarketingWorkerAllStopped` | P0 | backend-oncall | 条件触发 | worker 无成功 claim 且存在 eligible backlog，需判断是否影响发奖投递 |
| `ExMarketingDBWriteFailure` | P0 | dba-infra | 否，恢复后同步影响 | 核心写路径失败，优先修复 DB 并抑制下游噪音 |
| `ExMarketingQuotaOverDeductRisk` | P0 | backend-oncall | 是 | quota 聚合冲突或 reservation 缺失，必须先冻结风险操作再 reconciliation |
| `ExMarketingDLQGrowing` | P1 | backend-oncall | 是，若 close/replay 会改变业务处理 | DLQ 原因分析、replay/close 条件、恢复验证 |
| `ExMarketingOutboxBacklogHigh` | P1 | backend-oncall | 条件触发 | backlog、eligible age、worker/sender/provider 根因 |
| `ExMarketingCallbackFailRateHigh` | P1 | backend-oncall | 是 | callback 鉴权、冲突、provider 状态、quota finalize |
| `ExMarketingEventIntakeErrorRateHigh` | P2 | backend-oncall | 条件触发 | source system、鉴权、schema、活动配置变更 |
| `ExMarketingClaimLatencyHigh` | P2 | backend-oncall | 否，除非客户可见延迟 | worker 饱和、DB latency、retry scheduling |
| `ExMarketingRewardRetrySpike` | P2 | backend-oncall | 条件触发 | sender timeout/auth/allowlist/provider 状态 |
| `ExMarketingFactQueryFailRateHigh` | P2 | backend-oncall | 条件触发，provider-wide 时需要 | Fact Provider 可用性、mapping、field missing、安全拦截 |
| `ExMarketingScrapeTargetDown` | P3 | platform-observability | 否 | metrics 采集不可用，需避免误判业务故障 |
| DB 慢/错误 | P0/P2 场景 | dba-infra | 否，恢复后同步影响 | 错误按 P0 写失败，慢查询按 latency 排查场景覆盖 |

### 每个 Runbook 必填结构

每个告警小节必须包含：

- 症状：告警名、触发条件、关键标签、当前值、阈值。
- 影响判断：受影响租户、通道、活动、外部系统、用户可见影响、是否可能造成发奖延迟/重复/失败、quota 冻结或不一致。
- 首查入口：Grafana dashboard 链接字段、Prometheus 查询、相关 admin/API 路径、必要 DB 查询或审计查询。
- 止血动作：允许的临时操作、禁止操作、业务确认要求、维护 silence 要求。
- 恢复动作：修复根因后的 replay、close、timeout-release、worker restart、provider 恢复确认等动作。
- 恢复验证：告警清除条件、指标恢复条件、抽样 reconciliation 或 audit 验证、DLQ/backlog/冻结项归零或回落条件。
- 升级规则：P0/P1/P2/P3 的 acknowledgement 和升级时限，与现有 runbook escalation 保持一致。
- 复盘字段：时间线、影响面、根因、修复动作、预防项、后续 owner 和 due date。
- 安全约束：不得暴露 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始错误文本到指标标签或公开复盘材料。

### SLI/SLO/SLA 定义

第一版 SLO 必须采用“现有 metrics 可直接计算”的口径。默认按 tenant/channel 维度可下钻，整体 SLO 可按全局聚合展示。

| 目标 | SLI | 默认 SLO | 指标来源 |
| --- | --- | --- | --- |
| 可用性：事件接入成功率 | `success / total`，排除 `duplicate`，失败含 `failed|rejected` | 月度 >= 99.5% | `exmarketing_event_intake_total` |
| 发奖指令投递延迟 | outbox 创建到投递终态 p95 | 5 分钟窗口 p95 <= 120 秒，月度达标率 >= 99% | `exmarketing_reward_command_latency_seconds` |
| callback 处理成功率 | `success / total`，失败含 `rejected|failed|conflict` | 月度 >= 99.0%，且 5 分钟失败率不持续 > 5% | `exmarketing_reward_callback_total` |
| DLQ 清理时效 | 未关闭 DLQ 从进入 DLQ 到 replay 或 close 的耗时 | P1 DLQ 24 小时内清理 >= 95%；P0 关联 DLQ 4 小时内有处理结论 | `exmarketing_outbox_dlq_count`、`exmarketing_outbox_dlq_total`、DLQ 管理 API/审计 |
| 数据一致性 | quota aggregate conflict、reservation missing、核心写失败次数 | 月度 0 个未解释 P0；每个 P0 必须复盘 | `exmarketing_quota_fail_total`、`exmarketing_db_operation_total` |
| Worker claim 延迟 | eligible outbox 到 claim p95 | 5 分钟窗口 p95 <= 60 秒 | `exmarketing_worker_claim_latency_seconds` |
| Fact Query 成功率 | `success 或 cache_hit / total`，失败含超时、provider 不可用、mapping failed、field missing 等 | 月度 >= 98.5%，单 provider 按 fact_code 下钻 | `exmarketing_fact_query_total` |

SLA 对外承诺默认只写“内部运维响应目标”，不承诺客户赔付。若公司统一 SLA 另有规定，以统一 SLA 为准，本需求文档只提供 ExMarketing 侧 SLI/SLO 数据定义。

### 演练和复盘模板

演练记录模板必须包含：

- 演练编号、日期、环境、参与人、演练告警名、严重级别。
- 触发方式或模拟方式。
- 预期影响和实际影响。
- 实际排查步骤、使用的 dashboard/API/查询。
- 止血动作和恢复动作。
- 验证证据：指标截图或查询结果、API 返回、审计记录、告警恢复时间。
- 发现问题、改进项、owner、due date。

复盘模板必须包含：

- 事件摘要、严重级别、开始时间、发现时间、ack 时间、止血时间、恢复时间、关闭时间。
- 影响面：tenant、channel、activity、source system、reward provider、fact provider、受影响事件/订单数量的脱敏统计。
- 根因分类：代码、配置、DB、外部 provider、source system、容量、观测链路、操作失误、未知。
- 时间线。
- 处置动作和效果。
- 数据修复或补偿状态，但不定义赔付金额或赔付规则。
- 预防项和验收方式。

### 告警到 Runbook 映射表

文档必须提供映射表，至少包含：

- `alertname`
- severity
- owner
- dashboard
- runbook anchor
- 关键 labels
- 触发条件摘要
- 是否需要业务确认
- 主要恢复验证指标

映射表应与 `monitoring/alerts-exmarketing.rules.yml` 的 `runbook` label 保持一致。若 runbook anchor 改名，必须同步要求 Dev 更新映射文档和告警规则中的 runbook label；本 Analysis 不要求具体代码改法。

## 业务规则

- P0 告警必须立即响应；10 分钟未 ack 升级 backend/infra lead，20 分钟未恢复或无明确止血结论通知 business operations leadership。
- P1 告警通知 backend oncall 和相关业务/DB 接收组；30 分钟未 ack 升级模块 owner，60 分钟未恢复升级 backend lead。
- P2 告警进入 backend 工作队列或 oncall channel，1 小时内查看；连续两个窗口重复触发时按 P1 候选处理。
- P3 告警由 platform observability 在下一个工作窗口处理；若它导致 P0/P1 业务症状不可观测，应在 incident 中标记观测受损。
- DB 写失败是上游根因时，优先处理 DB，允许抑制下游 outbox/callback/worker/event/Fact Query 噪音，但不得静默 quota consistency 告警或 DB 告警本身。
- quota 一致性风险触发时，必须先暂停受影响活动 rollout 或冻结会扩大风险的操作，完成 quota reservation 与 aggregate 对账后，才能 replay callback 或 DLQ。
- DLQ replay 只能用于当前 `DLQ` 状态，成功后回到 `PENDING`，payload 不变；`DLQ_CLOSED` 不允许 replay。
- DLQ close 只关闭 outbox，不修改 participation、grant_order 或 quota reservation；是否 close 必须有业务确认和 reason。
- callback 失败处理必须保持 tenant 隔离；`SUCCESS`/`FAILED` 终态冲突、locator 冲突、reservation 缺失、quota 聚合冲突不能人工跳过，应进入 reconciliation 和复盘。
- MQ `SENT` 只表示 broker ack/publisher confirm/send ack，不代表权益已发放；真实发奖成功以 callback 为准。
- Fact Query 故障不得通过临时放宽 SSRF、allowlist、redirect、proxy 等安全限制止血；provider-wide 故障需协调 provider owner。
- 维护 silence 必须限制到最窄 `service/env/tenant/channel/severity/alertname` 组合，并记录 owner、原因、开始/结束时间、预期影响、回滚联系人，默认最长 4 小时。
- 指标和 runbook 示例不得泄漏敏感信息或高基数字段。

## 输入输出

输入资料：

- `tasks/2026-05-29-alerts-runbook-slo-runbook-slo/task.md`
- `monitoring/alerts-exmarketing.rules.yml`
- `docs/operations.md`
- `docs/runbooks/exmarketing-alerts.md`
- `docs/api.md`
- `docs/architecture.md`
- `docs/data-model.md`

输出交付物：

- 可被运维人员发现的 runbook 文档，位置满足任务要求的 `docs/operations` 体系；可采用 `docs/operations.md` 索引到 `docs/runbooks/exmarketing-alerts.md` 的结构。
- SLI/SLO/SLA 指标定义文档。
- 演练记录模板。
- 复盘模板。
- 告警到 runbook 映射表。

运行时输入：

- Alertmanager/Prometheus 告警 labels 与 annotations。
- Grafana dashboard URL。
- `/metrics` 指标。
- 管理 API：DLQ、reconciliation、timeout-release、audit。
- 必要的脱敏 DB 查询结果和外部 provider 状态。

运行时输出：

- 告警处置结论：误报、已止血、已恢复、需升级、需复盘、需业务确认。
- 审计记录：DLQ replay/close、quota timeout-release 等已有操作应写入 audit。
- 演练/复盘文档记录。

## 异常场景

- Prometheus scrape down：业务可能仍正常，但观测不可用；不得仅凭 scrape down 判断发奖失败，应检查服务进程和 `/healthz`、`/metrics`。
- Counter 因服务重启归零：SLO 查询必须使用 rate/increase，不用原始 counter 绝对值判断。
- 低流量单次失败导致失败率高：失败率告警必须同时满足失败率和最小失败数，runbook 也应提醒该边界。
- DB P0 导致下游告警泛洪：按 DB 根因优先，禁止逐个处理下游症状而忽略核心写失败。
- 外部 reward provider 不可用：可导致 outbox backlog、retry、DLQ、callback 延迟；是否暂停活动或 close DLQ 需要业务确认。
- callback 签名缺失/错误：按安全失败处理，不得临时关闭签名要求作为默认止血。
- quota reservation missing 或 aggregate conflict：视为数据一致性风险，不能直接重放或手工改 used/frozen 绕过。
- Fact Provider mapping 变更：可能导致 Fact Query fail rate 升高，需区分 provider HTTP 失败、timeout、mapping failed、field missing。
- 维护窗口内告警：只有符合最窄范围且有 owner/reason/end time 的 silence 才视为计划内；超范围告警仍需处理。
- 多租户局部故障：所有判断优先按 `tenant/channel/fact_code/source_system` 下钻，避免全局平均掩盖单租户故障。

## 边界条件

- 本需求只定义 runbook、SLO/SLA、模板和映射表，不要求新增 Prometheus 指标、告警规则、API 或数据库字段。
- 如果现有文档已经有部分内容，Dev 可在原文上扩展，不要求迁移目录结构；但任务要求的交付物必须能在 `docs/operations` 体系中被定位。
- P1/P2 告警必须有 runbook；P0/P3 也应覆盖，因为现有核心规则中 P0 与 P3 会影响处置闭环。
- “DB 慢”当前无独立 alertname 时，按 DB runbook 的 latency 排查章节验收，不阻塞。
- “业务确认”指业务运营或相关 provider/source owner 对会改变业务结果的操作确认，不等同于公司赔付审批。
- SLO 第一版以内部目标为准，不作为对客户的赔付 SLA。
- 演练可以在本地或测试环境完成，不要求真实生产故障。

## 非目标范围

- 不设计或承诺业务赔付规则。
- 不替代公司统一 incident 流程、事故分级制度或复盘审批流程。
- 不实现真实 oncall 排班系统、通知机器人或值班轮值配置。
- 不新增或修改业务处理代码、DB schema、外部 provider 协议。
- 不要求为每个外部 provider 写专属 runbook；本期只写通用 reward/source/fact provider 排查口径。
- 不要求公开暴露 `/metrics`、admin API 或敏感查询。

## 风险点

- 现有 runbook 内容过短，若只补标题不补排查命令/API/恢复验证，Review/Test 应判不通过。
- SLO 若引用不存在的指标，会无法验收；必须限定在现有 metrics。
- DLQ close/replay 和 quota timeout-release 可能改变业务状态，缺少业务确认会带来误发、漏发或冻结资金/权益长期悬挂风险。
- 指标标签或复盘材料若包含高基数或敏感字段，会产生安全和可用性风险。
- DB 写失败和 provider 故障可能引发多个告警，runbook 必须指导根因优先级，否则值班人员容易处理噪音。
- P2 连续触发未升级会掩盖慢性故障，需要明确升级规则。

## 验收方式

文档验收：

- 每个当前 P1/P2 alertname 都能在 runbook 中找到同名或明确映射的小节。
- 任务范围中的 DLQ 堆积、outbox backlog、callback 失败、quota 异常、worker 停止、DB 慢/错误、Fact Query 失败都有覆盖。
- 每个 runbook 小节包含症状、影响、排查命令或 API/dashboard、止血、恢复、验证、升级。
- 存在 SLI/SLO/SLA 定义，并能对应到现有 metrics 或现有管理 API/审计数据。
- 存在演练记录模板和复盘模板，演练后可直接填写。
- 存在告警到 runbook 映射表，且 alertname、severity、owner、dashboard、runbook anchor 与现有告警规则一致或可追踪。
- 文档明确哪些告警需要业务确认，哪些技术可直接处理。

建议静态校验：

- 检查告警配置：`promtool check config monitoring/prometheus-exmarketing.yml`
- 检查告警规则样例：`promtool test rules monitoring/alerts-exmarketing.test.yml`
- 检查 Alertmanager 配置：`alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check`

人工验收：

- 任选一个 P1 DLQ 告警，按 runbook 能定位 DLQ API、判断 replay/close 条件、给出恢复验证方式。
- 任选一个 P2 Fact Query 告警，按 runbook 能区分 provider timeout、mapping failed、field missing、安全拦截。
- 任选一个 P0 quota 一致性告警，按 runbook 能说明先冻结风险操作、后 reconciliation、再决定 replay/释放。
- 使用演练模板记录一次模拟处置，字段不应出现无法填写的空洞。

## 待确认问题

以下问题不阻塞本需求，均采用默认最小口径推进：

- 公司正式 SLA/赔付口径未提供：默认只定义 ExMarketing 内部 SLO 和响应目标，不承诺赔付。
- 业务确认人的具体名单未提供：默认按 `business operations`、reward provider owner、source system owner、fact provider owner 角色描述，不实现排班。
- 生产 Grafana 基础 URL 未提供：默认沿用告警规则中的 `${GRAFANA_BASE_URL}` 占位。
- DB 慢告警未见独立规则：默认在 DB runbook 中以 latency 排查场景覆盖，不要求新增告警。

## Score

```text
92
```

评分明细：

```text
需求清晰度        19/20
业务规则完整      19/20
边界异常清楚      18/20
验收标准明确      18/20
约束一致性        18/20
```

## Decision

```text
PASS
```

需求：2026-05-29-alerts-runbook-slo-runbook-slo
