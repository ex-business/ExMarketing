# Analysis

## 需求 ID

2026-05-29-alerts-production-drill

## Review 反馈处理

上一轮 `review.md` 是模板态，但仍存在需要回应的占位项：

| Review 项 | 处理结论 |
| --- | --- |
| `P0:`、`P1:`、`P2:` 为空 | 未发现 Review Agent 给出的实质 P0/P1/P2 缺陷。本轮按高风险需求重新补齐背景、规则、边界、异常和验收标准，避免因模板空缺被打回。 |
| `Open Questions: 待填写` | 本轮不把“待填写”保留为阻塞项。对无唯一答案的细节采用默认最小可验收口径：本地或测试环境、仅使用隔离租户/通道/演练 runId、覆盖任务列出的 6 类核心故障，每类必须有触发、观测、通知、恢复、清理和记录。 |
| `Decision: PASS / BLOCK` | Review 未给出明确打回原因。本轮以可验收需求为目标补齐后给出 `Decision PASS`，供 Review Agent 重新审核。 |

## Analysis 自检修正

上一轮 `analysis.md` 为模板态，存在 `Score 0`、`Decision PASS / BLOCK` 未决以及所有关键章节 `待填写` 的问题。本轮逐项修正：

| 自检问题 | 修正方式 |
| --- | --- |
| 背景和已知信息缺失 | 补充任务目标、现有告警规则、Alertmanager 路由、runbook、运维手册、核心指标和数据模型依据。 |
| 目标理解缺失 | 明确本需求要交付“可重复告警演练能力”，验证触发、通知、恢复、清理和报告，不做生产破坏性演练。 |
| 细化需求缺失 | 将范围拆为 6 个必须覆盖的演练场景，并定义每个场景的目标告警、指标、触发条件、恢复验证和业务确认要求。 |
| 业务规则缺失 | 补充严重级别、接收方、升级、Alertmanager 抑制、敏感信息、业务确认、演练数据隔离、清理和报告规则。 |
| 输入输出缺失 | 定义演练输入配置、环境、数据、操作参数、指标查询和输出交付物。 |
| 异常场景和边界条件缺失 | 补充告警未触发、误触发、恢复失败、通知失败、抑制生效、低流量误报、数据污染、外部依赖不可用等情况。 |
| 验收方式缺失 | 补充静态校验、规则单测、至少一轮本地/测试演练记录、每场景证据、清理验证和报告验收。 |
| Score 低于 80 | 本轮补齐到可审核、可开发、可测试口径，自评分 92。 |
| Decision 未决 | 本轮关闭旧 BLOCK 条件，结论为 PASS。 |

## 背景和已知信息

ExMarketing 是营销活动引擎，覆盖活动命中、发奖单、quota、outbox、DLQ、reward callback、Fact Query、审计和对账链路。告警配置已经存在于 `monitoring/alerts-exmarketing.rules.yml`，本地 Prometheus scrape 配置为 `monitoring/prometheus-exmarketing.yml`，Alertmanager 路由和抑制配置为 `monitoring/alertmanager-exmarketing.yml`，运维处置和演练模板位于 `docs/runbooks/exmarketing-alerts.md`，运维入口说明位于 `docs/operations.md`。

本任务要求验证告警不是“只写规则”，而是能在本地或测试环境通过故障注入复现指标变化、触发告警、发送通知、完成恢复并形成演练报告。任务风险等级为高，因为演练覆盖 worker 停止、DB 写失败或慢查询、outbox 堆积、DLQ 增长、callback 失败和 quota 一致性风险等会影响发奖、预算、库存或用户权益的链路。

默认最小可验收口径：

- 环境限定为本地或测试环境，禁止在生产环境直接做破坏性演练。
- 演练必须使用隔离租户、隔离通道、隔离活动或唯一 drill runId，避免污染长期测试数据。
- 覆盖任务范围中的 6 类核心故障：Worker 停止或不消费、Outbox backlog 堆积、DLQ count 增长、Callback 失败率升高、DB 慢查询或连接失败、Quota 扣减冲突或失败率升高。
- 每类故障必须记录触发、观测、告警通知、恢复、清理和复盘证据。
- 若某类故障在当前环境不能真实破坏依赖，允许使用可审计的模拟注入方式，但必须让相同指标族和目标告警规则进入可验证状态，且报告中标记为 simulated。

## 目标理解

本需求的目标是交付一套可重复执行的生产告警演练资产，让后续 Dev/Test/运维可以在本地或测试环境验证：

- 核心故障场景能产生预期 ExMarketing 指标变化。
- Prometheus 告警规则能在规定窗口内触发目标 alert。
- Alertmanager 能按 severity、owner、tenant、channel 路由到正确接收方，并能发送 resolved 恢复通知。
- runbook 能指导值班人员完成首查、止血、恢复、验证和升级。
- 演练完成后可以清理演练数据，并留下时间线、指标、通知和修复动作记录。

成功不是要求真实造成生产事故，也不是要求建设混沌工程平台，而是要求最小可重复、可审计、可回滚的演练流程。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| Backend oncall | 负责 ExMarketing 业务链路告警演练、worker/outbox/DLQ/callback/quota 首查、恢复验证和报告填写。 |
| DBA/Infra | 负责 DB 连接失败、核心写失败、慢查询或连接池类场景的注入边界、恢复和 DB 证据。 |
| Platform observability | 负责 Prometheus、Alertmanager、Grafana、scrape、通知路由和 resolved 通知可用性。 |
| Business operations | 在涉及 DLQ replay/close、活动暂停/恢复、quota timeout-release、客户影响判断时提供业务确认。 |
| Reward/source/fact provider owner | 当故障模拟外部发奖、事件源或 Fact Provider 异常时确认影响和恢复条件。 |
| Test Agent | 根据本需求验证演练步骤、告警触发、清理和报告证据是否完整。 |

## 细化需求

### 交付物

必须交付以下内容：

- 故障注入脚本或操作手册：覆盖 6 类核心故障，每类包含前置条件、触发步骤、指标查询、预期告警、恢复步骤、清理步骤和失败排查。
- 演练数据准备和清理步骤：说明如何创建隔离 tenant/channel/activity/runId，如何确认 outbox、DLQ、callback、quota、audit、metrics 不污染长期测试数据。
- 演练报告模板：字段至少覆盖 runId、环境、参与人、场景、时间线、告警名、severity、通知接收方、指标证据、恢复动作、清理证据、业务确认、问题和改进项。
- 至少一轮本地或测试环境演练记录：至少覆盖 6 类核心故障中的每一类；若受环境限制使用模拟注入，报告必须标注模拟方式和未覆盖真实依赖的风险。

### 必须覆盖的演练场景

| 场景 | 最小触发目标 | 目标告警或验证对象 | 恢复验证 |
| --- | --- | --- | --- |
| Worker 停止或不消费 | 有 eligible outbox backlog，最老 eligible age 超过 120s，5 分钟无成功 claim | `ExMarketingWorkerAllStopped` P0；同时可观察 `exmarketing_outbox_backlog`、`exmarketing_outbox_oldest_eligible_age_seconds`、`exmarketing_worker_claim_total` | worker 恢复后成功 claim 速率大于 0，oldest eligible age 下降，backlog 低于阈值，Alertmanager 发送 resolved |
| Outbox backlog 堆积 | 单 tenant/channel 的 `PENDING|FAILED` eligible backlog 大于 100，oldest eligible age 大于 60s 并持续 5 分钟 | `ExMarketingOutboxBacklogHigh` P1 | backlog 和 oldest age 回落，reward command p95 小于等于 120s，DLQ 不继续增长 |
| DLQ count 增长 | 单 tenant/channel 10 分钟内新增 DLQ 数大于等于 5 | `ExMarketingDLQGrowing` P1；验证 `exmarketing_outbox_dlq_total` 和 `exmarketing_outbox_dlq_count` | 根因解除后 replay 或业务确认 close；DLQ count 下降或稳定，无新增 DLQ，audit 有 `OUTBOX_DLQ_REPLAY` 或 `OUTBOX_DLQ_CLOSE` |
| Callback 失败率升高 | 5 分钟内 callback rejected/failed/conflict 数大于等于 5，且失败率大于 5% | `ExMarketingCallbackFailRateHigh` P1 | 失败率低于 5%，accepted callback 恢复增长，对账显示终态符合预期，quota reservation 被正确 confirm 或 release |
| DB 慢查询或连接失败 | 连接失败或核心写路径失败；慢查询则验证 DB latency 仪表盘和 runbook，不冒充核心写 P0 | 核心写失败触发 `ExMarketingDBWriteFailure` P0；慢查询验证 `exmarketing_db_operation_latency_seconds` 和 DB Slow Or Error runbook | 核心写失败 5 分钟无新增，DB latency 回到基线，event/callback/outbox 依赖链路恢复；慢查询场景记录 p95 回落 |
| Quota 扣减冲突或失败率升高 | quota aggregate conflict 或 reservation missing 至少 1 次；或 reserve 失败率升高用于面板验证 | `ExMarketingQuotaOverDeductRisk` P0；同时观察 `exmarketing_quota_fail_total` | 无新增一致性失败，对账干净，aggregate used/frozen 与 reservation 匹配；任何 timeout-release 必须有业务确认和 audit |

### 通知和恢复验证

每个场景必须验证 firing 和 resolved 两个阶段：

- firing：记录 alertname、severity、tenant、channel、owner、dashboard、runbook、current_value、threshold、StartsAt 和接收方。
- routed：确认 Alertmanager 按配置路由到对应 receiver。P0 backend 走 `backend-oncall-p0` 或 `dba-infra-p0`，业务相关 P0/P1 走 `business-ops`，P1 走 `backend-oncall`，P2 走 `backend-workqueue`，P3 走 `platform-observability`。
- resolved：恢复后记录 EndsAt 或 resolved 通知证据。若告警因窗口原因延迟恢复，报告必须记录等待窗口和最终状态。
- inhibited：DB P0 或高优先级告警抑制下游告警时，不视为失败；报告必须记录被抑制的下游 alert 和抑制规则依据。

## 业务规则

- 严重级别沿用现有告警规则：Worker stopped、DB write failure、Quota over-deduct risk 为 P0；DLQ growing、Outbox backlog high、Callback fail rate high 为 P1；Event intake、Claim latency、Reward retry、Fact Query 为 P2；Scrape target down 为 P3。
- P0 必须立即通知 backend oncall 或 DBA/infra；10 分钟未确认升级到 backend/infra leads；20 分钟未恢复或仍未确认则通知 business operations leadership。
- P1 通知 backend oncall 和相关业务/DB 接收方；30 分钟未确认升级到模块 owner；60 分钟未恢复升级到 backend lead。
- P2 进入 backend work queue/oncall channel，1 小时内处理；连续两个窗口重复出现应作为 P1 候选记录。
- 演练期间不得绕过 callback signature、SSRF 保护、provider allowlist、quota 一致性检查、tenant 隔离或 idempotency 检查。
- 技术 oncall 可以不经业务确认执行 worker 重启、DB 连接恢复、scrape 修复、指标检查、脱敏审计/对账查询、移除过期维护 silence。
- 以下动作必须有 business operations 确认：关闭 DLQ、客户可见 provider 事故后的 DLQ 或 callback replay、暂停或恢复活动、quota timeout-release、变更活动配置绕过 provider/source 问题、声明客户影响。
- 指标标签必须低基数，不得包含 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始 provider 错误文本。
- Counter 指标验证必须使用 `rate` 或 `increase`，不能依赖绝对值；服务重启导致 counter 归零时，报告中必须记录重启时间。
- DB 慢查询没有核心写失败时不得按 `ExMarketingDBWriteFailure` P0 验收，只能按 DB latency 和 runbook 验证；只有核心写路径失败才触发 DB P0。
- 演练数据必须可清理。无法清理的长期残留数据、未关闭 DLQ 或未复原配置均判定演练失败。

## 输入输出

### 输入

- 环境：本地或测试环境；默认服务地址 `:8080`，Prometheus 默认 `localhost:9090`，Alertmanager 地址按测试环境配置。
- 数据库：测试 PostgreSQL 或本地 PostgreSQL；使用隔离 DSN 或隔离库，禁止生产库。
- Prometheus/Alertmanager：加载 ExMarketing scrape、alert rules、routing、template 配置。
- 演练参数：`drillId`、tenant、channel、activity、source_system、event_type、fact_code、operator、reason、预期 alertname、开始/结束时间。
- 业务数据：可产生 outbox、callback、quota、audit 和 reconciliation 记录的最小活动/事件数据；必须能通过 runId 或隔离 tenant 识别。
- 通知接收端：本地或测试 webhook/mock receiver，必须能记录 firing/resolved payload。

### 输出

- 每个场景的演练记录，包含时间线、触发方式、指标截图或查询结果、告警 payload、通知 receiver、恢复动作、清理动作和结论。
- 汇总演练报告，包含所有场景 PASS/FAIL、发现的问题、owner、due date 和复测要求。
- 演练数据清理证明，至少包含 outbox/DLQ/callback/quota/audit/reconciliation 相关残留检查或说明。
- 如发生失败，输出可复现日志和失败原因分类：规则未触发、指标未变化、通知未送达、恢复未成功、清理失败、文档不一致或环境不可用。

## 异常场景

- 告警未触发：必须检查指标是否产生、Prometheus 是否 scrape、规则 evaluation interval/for 窗口是否满足、label 是否匹配、是否被 Alertmanager inhibit 或 silence。
- 告警误触发：必须记录误触发指标、阈值、低流量分母、counter reset、测试数据污染或 label 聚合问题，并判定该场景 FAIL。
- 通知未送达：若 Prometheus alert firing 但 receiver 无记录，判为通知链路失败；需要记录 Alertmanager route、receiver、模板字段和 webhook 响应。
- 恢复后未 resolved：必须记录恢复指标、告警仍 firing 的表达式原因、for 窗口等待情况和最终状态；超出演练窗口仍未 resolved 判 FAIL。
- DB P0 抑制下游告警：若 DB write failure 激活，outbox/callback/worker/event/fact-query 下游告警被抑制是预期行为，不按下游未通知判失败，但必须记录抑制证据。
- 演练数据无法清理：未关闭测试 DLQ、未恢复 worker 配置、未恢复 DB 连接、未清除维护 silence、测试活动仍 ACTIVE 且会继续产生数据时，整体演练 FAIL。
- 外部 provider 不可用：不调用真实资金或真实发奖系统；使用测试 stub 或模拟 provider，并在报告中声明不覆盖真实 provider SLA。
- 低流量 callback 或 event 误报：失败率类告警必须同时满足最小失败数和失败率；单次失败不得视为成功触发。
- 服务重启导致 counter reset：演练报告必须基于 `increase/rate` 查询并注明重启，不能用绝对 counter 差值作为唯一证据。
- 维护 silence 冲突：若演练期间存在 silence，必须先记录或缩小 silence；目标告警被 silence 导致未通知时，不算演练通过。

## 边界条件

- 生产环境只允许桌面推演或非破坏性查询，不允许直接停止 worker、破坏 DB、制造真实 DLQ 或影响真实客户权益。
- 本地/测试环境可以使用模拟注入，但必须验证真实告警规则和通知路由，不得只写静态文档替代演练。
- 演练不要求覆盖所有 P2/P3 告警，但不得破坏已有 Event intake、Fact Query、scrape 等 runbook 与告警配置。
- 同一轮演练可串行执行场景，避免多个根因相互抑制导致证据不清。若并行执行，报告必须明确关联关系和抑制效果。
- 演练使用的 tenant/channel/fact_code 应保持低基数，不能为每条事件生成唯一 label。
- outbox backlog 场景必须区分 eligible backlog 和未到 retry 时间的 backlog；未到期 retry 不应抬高 oldest eligible age。
- DLQ replay 或 close 只能处理当前演练产生的 DLQ，不能批量操作非演练数据。
- quota consistency 场景必须以对账恢复为结束标准，不能只以告警 resolved 为结束。
- DB 慢查询场景默认以测试环境 p95 超过 1 秒作为排查阈值；若环境已有明确基线，以环境基线为准并在报告中记录。

## 非目标范围

- 不在生产环境直接做破坏性故障注入。
- 不建设通用混沌工程平台或长期压测平台。
- 不模拟外部发奖系统真实资金行为，不触发真实用户权益变更。
- 不修改告警阈值、业务规则、quota 算法、callback 签名策略、SSRF/allowlist 安全策略作为本需求目标。
- 不以代码实现细节替代演练需求；具体脚本语言、目录结构和实现方式由 Dev Agent 在后续阶段按本需求选择。
- 不要求覆盖所有可能的 provider/source/fact 组合；本轮只要求每类核心故障至少一个代表性 tenant/channel 或 fact_code。

## 风险点

- 故障注入可能留下残留 DLQ、冻结 quota、测试活动或维护 silence，影响后续测试结论。
- Alertmanager 抑制规则可能让下游告警不通知，需要在报告中区分“未触发”和“被抑制”。
- DB 慢查询与 DB 写失败容易混淆，若验收口径不清会造成 P0 误报或漏报。
- 使用模拟注入可能覆盖不到真实 provider、网络、连接池或 worker 竞争问题，报告必须保留 simulated 标记。
- 失败率类告警在低流量下容易被单次失败干扰，必须使用最小失败数和失败率双条件。
- 指标或通知 payload 若泄露敏感字段，会违反现有运维安全约束。
- 多场景连续演练时，前一个场景的 backlog、DLQ 或 DB 状态可能污染下一个场景。

## 验收标准

### 文档和脚本/手册验收

- 存在可执行的故障注入脚本或操作手册，覆盖 6 类核心故障。
- 每个场景都有前置条件、触发步骤、观测指标、预期 alertname/severity/receiver、恢复步骤、清理步骤和失败排查。
- 演练报告模板字段覆盖 `docs/runbooks/exmarketing-alerts.md` 的 Drill Record Template，并额外包含清理证据和通知 payload 证据。
- 所有步骤明确本地/测试环境限定和生产禁止事项。

### 演练结果验收

- 至少完成一轮本地或测试环境演练记录。
- 6 类核心故障每类均有 PASS/FAIL 结论；PASS 必须有指标变化、告警 firing、通知路由、恢复 resolved、清理证据。
- 告警触发时间和恢复时间可记录，且报告中能看出 Prometheus `for` 窗口和 Alertmanager route 的影响。
- 演练不会污染长期测试数据：演练后无未处理测试 DLQ、无异常冻结 quota、无遗留破坏性配置、无未过期无说明 silence。
- 失败场景必须有复现日志、失败分类、owner 和复测条件。

### 配置一致性验收

- Prometheus 规则静态校验通过。
- 告警规则样例单测通过。
- Alertmanager 配置校验通过。
- 通知模板包含 service、env、tenant、channel、severity、alertname、owner、summary、description、dashboard、runbook、first_seen、current_value、threshold。
- Alert labels 不包含禁止的高基数或敏感字段。

## 待确认问题

以下问题不阻塞本轮推进，默认采用最小可验收口径：

| 问题 | 默认口径 |
| --- | --- |
| 演练脚本还是操作手册优先？ | 二者至少交付其一；若无法安全自动化，手册必须足够具体，可由 Test Agent 按步骤复现。 |
| 通知 receiver 是否必须是真实值班系统？ | 本地/测试环境可使用 mock webhook receiver，但必须验证 Alertmanager route、模板字段和 send_resolved 行为。 |
| DB 慢查询是否必须触发 P0？ | 不必须。慢查询默认只验证 latency 观测和 runbook；只有核心写失败才触发 `ExMarketingDBWriteFailure` P0。 |
| 是否必须真实调用外部发奖系统？ | 不必须，且不得触发真实资金行为。使用测试 stub 或模拟 provider，报告标注 simulated。 |
| quota 冲突如何安全模拟？ | 默认使用隔离演练数据产生 `quota_aggregate_conflict` 或 `quota_reservation_missing` 指标；不得手工修改生产或共享测试业务数据。 |

## Score

```text
92
```

评分说明：

| 维度 | 分数 | 说明 |
| --- | --- | --- |
| 需求清晰度 | 19/20 | 目标、交付物、参与方、场景和默认口径明确。 |
| 业务规则完整 | 18/20 | 覆盖严重级别、升级、业务确认、敏感信息、抑制和数据隔离。 |
| 边界异常清楚 | 18/20 | 覆盖未触发、误触发、通知失败、恢复失败、抑制、低流量、清理失败。 |
| 验收标准明确 | 19/20 | 文档、演练结果、配置一致性和报告证据均可审核。 |
| 约束一致性 | 18/20 | 与 task、operations、runbook、alert rules 和非目标保持一致；具体脚本实现留给 Dev 阶段。 |

## Decision

```text
PASS
```

需求：2026-05-29-alerts-production-drill
