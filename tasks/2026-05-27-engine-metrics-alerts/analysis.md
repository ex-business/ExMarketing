# Analysis: 2026-05-27-engine-metrics-alerts

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未写代码，未提交，未设计具体代码改法。

已读取：

- `tasks/2026-05-27-engine-metrics-alerts/review.md`
- `tasks/2026-05-27-engine-metrics-alerts/analysis.md` 上一版
- `tasks/2026-05-27-engine-metrics-alerts/task.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/roadmap.md`
- `docs/workflow.md`
- 相关历史任务材料：PostgreSQL E2E、Worker 生产化、Quota、Callback、Outbox / DLQ 等任务文档

未发现独立 PRD 目录。本分析以任务说明、现有 API / 数据模型 / 运维文档和历史任务的业务口径为依据。

## Review 反馈处理

`review.md` 当前为模板态，但按本任务要求仍逐条回应：

| Review 项 | 处理 |
| --- | --- |
| `P0:` / `P1:` / `P2:` 均为空 | 未发现真实打回缺陷；本轮按生产化监控任务补齐指标语义、标签、采集入口、告警阈值、边界异常和验收标准。 |
| `Open Questions` 为“待填写” | 视为 Review 尚未形成具体问题；本轮不等待 Owner 决策，默认采用“Prometheus 文本 metrics endpoint 或等价可接入指标接口”的最小口径，并明确必须有稳定指标名、标签和阈值文档。 |
| `Decision` 为 `PASS / BLOCK` 模板 | 不视为真实 PASS；本轮重新评分并给出明确 `Decision PASS`。 |

## Analysis 自检修正

上一版 `analysis.md` 是模板态，存在 `Score 0`、`Decision PASS / BLOCK` 和大量“待填写”，等价于 Analysis 自检未通过。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| 背景、目标、参与方、输入输出、业务规则、边界异常、非目标、风险、验收方式均为空 | 本轮全部补齐，并限定为“活动引擎核心指标和告警口径”，不扩大到完整 APM / 日志平台建设。 |
| `Score = 0` | 已形成可审核、可开发、可测试需求，重新评分为 92。 |
| `Decision = PASS / BLOCK` 模板 | 已给出 `Decision PASS`。 |
| 缺少默认口径 | 已按最小可验收原则确定：必须暴露可被运维系统接入的指标；若项目没有既有指标系统，最低接受 `/metrics` Prometheus 文本 endpoint；告警先以文档建议阈值交付，不要求接入 PagerDuty / Grafana。 |
| 旧隐含 BLOCK：没有 Owner 决策无法确定平台 | 已关闭。平台不是本需求阻塞项，核心是指标语义稳定、可采集、可验证、阈值可落地。 |

## 需求说明

本需求要求为 ExMarketing 活动引擎补齐核心运行指标和告警建议，覆盖 event intake、quota、outbox、DLQ、reward callback、worker claim latency 和 reward command latency。目标是让运维和研发能在生产化前发现事件接入失败、额度拒绝异常升高、发奖指令堆积、DLQ 堆积、callback 失败和 worker 处理延迟等问题。

本需求的交付物不是单纯日志，也不是完整监控平台。最小可验收结果是：服务提供稳定的 metrics endpoint 或等价可接入指标接口，并在文档中列出每个指标的含义、标签、单位、采集方式和建议告警阈值。

## 背景目标

ExMarketing 核心链路为：

```text
Event Intake
-> Idempotency
-> Campaign Match
-> Rule Evaluation
-> Quota / Budget / Inventory
-> Grant Order
-> Outbox
-> External Reward System
-> Callback In
```

现有运维文档已要求生产前确认 Outbox worker、Quota 原子扣减、DLQ 告警和人工处理流程。本任务补齐“看得见”的生产化要求，避免只能通过人工 SQL 或出问题后查日志判断链路是否健康。

目标：

1. 暴露活动引擎核心指标，覆盖任务要求列出的七类指标。
2. 指标必须可被机器采集，不能只靠自然语言日志。
3. 指标标签支持按租户、事件类型、活动、quota 类型、outbox channel、callback 结果等关键维度定位问题。
4. 指标不得泄露 token、secret、Authorization、完整 payload、用户敏感 attributes 或外部系统凭据。
5. 文档给出默认告警阈值建议、严重级别和排查方向。
6. 指标应兼容内存仓储本地开发和 PostgreSQL 生产验收；涉及 backlog / DLQ / claim latency 的强验收以 PostgreSQL 为准。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 外部事件来源系统 | 调用 Event API 上报业务事件，是 intake 成功 / 失败指标来源。 |
| ExMarketing 活动引擎 | 产生活动匹配、quota、grant_order、outbox、callback 等业务状态，是指标主体。 |
| Outbox worker | 产生命令投递、claim latency、backlog、DLQ 和 reward command latency 指标。 |
| 外部发奖系统 | 接收 reward command 并回调结果，是 callback 成功 / 失败指标关联方。 |
| PostgreSQL | 生产验收中提供 outbox backlog、DLQ count、quota 和 callback 状态的真实数据来源。 |
| 运维 / SRE | 采集指标、配置告警、处理 DLQ、排查堆积和失败率异常。 |
| Dev / Test Agent | 按本需求实现和验证指标语义、endpoint / 接口、文档阈值和异常场景。 |

## 输入输出

### 输入

| 输入 | 默认口径 |
| --- | --- |
| HTTP Event API 调用结果 | 统计 intake 成功、失败、重复事件、鉴权失败、payload 错误、幂等键错误、quota 拒绝等结果。 |
| Quota 处理结果 | 统计 quota 成功预留 / 确认 / 释放，以及 quota 不足失败；至少区分 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。 |
| Outbox 当前状态 | 统计待发送、发送中、失败待重试、DLQ、DLQ_CLOSED 等数量。 |
| Worker claim 过程 | 统计可投递消息从 eligible 到被 claim 的延迟、claim 成功 / 冲突 / 空批次。 |
| Reward command 投递过程 | 统计 outbox command 从创建到投递成功 / 失败 / DLQ 的端到端延迟。 |
| Reward callback 处理结果 | 统计 callback 成功、失败、重复、冲突、签名 / 鉴权失败、找不到单据、quota 终结失败等结果。 |
| 服务运行配置 | 指标暴露开关、监听地址、采集路径、标签维度限制和敏感信息脱敏规则。 |

### 输出

必须输出以下交付物：

1. 一个可接入指标采集系统的接口。默认最小口径为 `GET /metrics` 返回 Prometheus text format；如果采用项目内已有指标接口或 recorder，也必须提供等价机器可读输出和文档。
2. 指标文档，列出指标名、类型、单位、标签、触发点、状态枚举和示例。
3. 告警建议文档，列出默认阈值、统计窗口、严重级别、排查入口和降噪规则。
4. 测试或 smoke 结果能证明指标在成功、失败、堆积和 DLQ 场景下会变化。

## 指标口径

指标命名默认使用 `exmarketing_` 前缀。若项目已有命名规范，可保持本语义不变地映射到既有命名。

### Event intake

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_event_intake_total` | Counter | `tenant_id`、`source_system`、`event_type`、`result`、`reason` | Event API 接收结果总数。 |
| `exmarketing_event_intake_latency_seconds` | Histogram | `tenant_id`、`source_system`、`event_type`、`result` | Event API 从接收请求到业务处理完成的耗时。 |

`result` 默认枚举：

```text
success
duplicate
failed
rejected
auth_failed
```

`reason` 默认枚举包括：

```text
ok
duplicate
invalid_payload
idempotency_key_mismatch
auth_failed
activity_not_matched
rule_rejected
quota_exceeded
internal_error
```

### Quota fail rate

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_quota_attempt_total` | Counter | `tenant_id`、`activity_id`、`quota_type`、`scope`、`result`、`reason` | Quota 预留 / 确认 / 释放尝试结果。 |
| `exmarketing_quota_fail_total` | Counter | `tenant_id`、`activity_id`、`quota_type`、`scope`、`reason` | Quota 失败计数，用于 fail rate。 |

`quota_type` 至少覆盖：

```text
BUDGET
STOCK
USER_LIMIT
DAILY_LIMIT
```

失败率默认计算口径：

```text
quota_fail_rate = rate(exmarketing_quota_fail_total[5m])
                / rate(exmarketing_quota_attempt_total{result!="skipped"}[5m])
```

缺少 quota 行表示不限额，不应计入 quota fail；可计入 `result=skipped` 或不计入 attempt，但文档必须固定一种口径。

### Outbox backlog

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_outbox_backlog` | Gauge | `tenant_id`、`target_channel`、`send_status` | 当前 outbox 待处理 / 发送中 / 重试 / DLQ 数量。 |
| `exmarketing_outbox_oldest_eligible_age_seconds` | Gauge | `tenant_id`、`target_channel` | 当前最老可投递 outbox 的等待时长。 |

Backlog 默认统计：

- `PENDING`
- 到期可重试的 `FAILED`
- `SENDING`
- `DLQ`
- `DLQ_CLOSED` 可单独统计，但不纳入自动处理 backlog。

未到期 retry 不计入可投递 backlog，但可以在 `send_status=FAILED` 下展示总量。

### DLQ count

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_outbox_dlq_count` | Gauge | `tenant_id`、`target_channel` | 当前未关闭 DLQ 数量。 |
| `exmarketing_outbox_dlq_total` | Counter | `tenant_id`、`target_channel`、`reason` | 新进入 DLQ 的累计数量。 |

`DLQ_CLOSED` 不计入当前待处理 DLQ count，但可在 backlog 中作为关闭历史展示。

### Callback fail rate

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_reward_callback_total` | Counter | `tenant_id`、`grant_channel`、`callback_status`、`result`、`reason` | Reward callback 处理结果总数。 |
| `exmarketing_reward_callback_latency_seconds` | Histogram | `tenant_id`、`grant_channel`、`result` | Callback 从接收请求到更新业务状态完成的耗时。 |

`callback_status` 默认枚举：

```text
SUCCESS
FAILED
UNKNOWN
```

`result` 默认枚举：

```text
accepted
duplicate
conflict
rejected
failed
```

`reason` 默认包含：

```text
ok
duplicate
signature_invalid
auth_failed
grant_order_not_found
tenant_mismatch
status_conflict
quota_reservation_missing
quota_aggregate_conflict
invalid_payload
internal_error
```

失败率默认计算：

```text
callback_fail_rate = rate(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m])
                   / rate(exmarketing_reward_callback_total[5m])
```

重复 callback 不算失败，但必须可观测为 `result=duplicate`。

### Worker claim latency

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_worker_claim_total` | Counter | `worker_id`、`tenant_id`、`target_channel`、`result` | Worker claim 尝试结果。 |
| `exmarketing_worker_claim_latency_seconds` | Histogram | `tenant_id`、`target_channel` | 从 outbox eligible 到被 claim 的时延。 |
| `exmarketing_worker_inflight` | Gauge | `worker_id`、`target_channel` | 当前 worker 正在处理的 outbox 数。 |

`eligible_at` 默认口径：

- 新 outbox：`created_at`
- retry outbox：`next_retry_at`
- stale `SENDING` 恢复：恢复为可处理状态的时间

### Reward command latency

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `exmarketing_reward_command_total` | Counter | `tenant_id`、`target_channel`、`result`、`reason` | Reward command 投递结果总数。 |
| `exmarketing_reward_command_latency_seconds` | Histogram | `tenant_id`、`target_channel`、`result` | 从 outbox 创建到投递成功 / 失败 / DLQ 的端到端耗时。 |
| `exmarketing_reward_command_delivery_latency_seconds` | Histogram | `tenant_id`、`target_channel`、`result` | 单次外部投递调用耗时。 |

`result` 默认枚举：

```text
sent
retry
dlq
failed
skipped
```

Outbox `SENT` 只表示 command 已到达外部系统或 broker，不代表奖励最终成功；最终发奖结果以 callback 指标为准。

## 业务规则

1. 指标必须只反映业务和运行状态，不改变事件处理、quota、grant_order、outbox、callback 的原有业务语义。
2. 指标采集失败不得导致主业务请求失败；但应有结构化日志或健康诊断说明指标输出异常。
3. Counter 只能递增，不能因重启后尝试补历史而重复加计；历史状态类数据使用 Gauge 查询当前值。
4. Gauge 类 backlog / DLQ 指标必须来自当前状态快照，不得只依赖进程内计数，否则重启后会丢失。
5. PostgreSQL 模式下 backlog / DLQ / oldest age 必须以数据库状态为准；内存模式仅作为本地开发可观测，不作为生产强验收。
6. 高基数标签必须受控。不得把 `event_id`、`biz_id`、`subject_id`、`bill_no`、`command_id`、完整 `target_ref`、错误明细或用户 attributes 作为指标标签。
7. `activity_id` 可用于 quota 指标；如果担心活动数量导致高基数，文档必须说明可通过配置关闭或降级为 `activity_type` / `activity_code`。默认最小口径允许 `activity_id` 出现在 quota 指标中，因为排查 quota fail 需要定位活动。
8. `tenant_id` 是必要定位维度；单租户本地环境也必须输出该标签或等价默认值。
9. 所有错误原因必须归一化为有限枚举。原始错误文本只允许进日志，不允许进入指标标签。
10. 指标 endpoint 默认只面向内网 / 本地运维访问；不得在文档中声明可直接公网暴露。
11. 若 OpenAPI 鉴权开启，业务 API 的鉴权失败应计入 event intake 指标，但 metrics endpoint 自身鉴权策略不属于本任务强制范围。
12. Simulate API 不写业务表，默认不计入正式 event intake 成功率；如需观测模拟运行，应使用独立指标，不纳入本任务核心指标。

## 告警建议

默认阈值是最小可上线建议，后续可由 SRE 按真实流量调整。

| 告警 | 统计窗口 | 默认阈值 | 严重级别 | 排查方向 |
| --- | --- | --- | --- | --- |
| Event intake failed rate | 5 分钟 | 失败率 > 5% 且失败数 >= 20 | Warning | 检查鉴权、payload、幂等键、服务错误。 |
| Event intake internal error | 5 分钟 | `reason=internal_error` 数量 >= 1 | High | 检查服务日志、数据库连接、panic / error。 |
| Quota fail rate | 5 分钟 | 任一活动 / quota type 失败率 > 20% 且失败数 >= 10 | Warning | 判断是否活动配置额度耗尽或异常拒绝。 |
| BUDGET / STOCK exhausted | 5 分钟 | `quota_exceeded` 持续出现且活动仍 ACTIVE | Warning | 通知运营检查预算 / 库存。 |
| Outbox backlog age | 5 分钟 | 最老可投递 outbox age > 60 秒 | Warning | 检查 worker 是否开启、外部目标是否可达。 |
| Outbox backlog count | 5 分钟 | 单 tenant/channel 可投递 backlog > 100 | Warning | 检查 worker 吞吐、外部目标、retry。 |
| DLQ count | 即时或 5 分钟 | 未关闭 DLQ > 0 | Warning | 进入 DLQ 人工处理流程。 |
| DLQ count high | 5 分钟 | 单 tenant/channel 未关闭 DLQ >= 10 | High | 检查外部发奖系统、配置、凭据、targetRef。 |
| Callback fail rate | 5 分钟 | 失败率 > 5% 且失败数 >= 5 | High | 检查 callback payload、签名、跨 tenant、quota 终结。 |
| Worker claim latency P95 | 5 分钟 | P95 > 60 秒 | Warning | 检查 worker 运行、DB 锁、backlog、poll 间隔。 |
| Reward command latency P95 | 5 分钟 | P95 > 120 秒 | Warning | 检查 outbox backlog、外部目标耗时、retry。 |
| Worker in-flight stuck | 5 分钟 | in-flight > 0 且 command latency 无推进 | Warning | 检查外部调用卡住、shutdown drain、stale claim。 |

降噪规则：

- 请求总量过低时必须结合最小失败数，避免低流量租户单个失败触发失败率告警。
- `quota_exceeded` 对热门活动可能是业务预期，默认 Warning，不直接 High；只有突增、持续或影响核心活动时升级。
- `duplicate` 事件和重复 callback 默认不算失败，但短时间异常升高应作为独立观察项，不阻塞本任务。
- DLQ 大于 0 必须可见，因为它代表需要人工判断 replay / close；是否通知到人由后续告警平台决定。

## 边界条件

| 边界 | 预期 |
| --- | --- |
| 未配置正式监控平台 | 不阻塞；最低提供 Prometheus 文本 endpoint 或等价机器可读接口。 |
| 内存仓储运行 | 可输出进程内指标；backlog / DLQ 快照只作为本地开发参考，生产验收以 PostgreSQL 为准。 |
| PostgreSQL 不可用 | 业务和 backlog 指标可能失败；metrics endpoint 应尽量返回已知进程指标，并对数据库快照类指标输出缺失或错误观测，不应卡死。 |
| Outbox worker 未开启 | Worker 指标显示无 claim / no worker；outbox backlog 和 oldest age 仍应反映堆积。 |
| 没有流量 | Counter 保持 0，Gauge 输出 0 或无该维度；不应产生虚假告警。 |
| 服务重启 | Counter 从进程生命周期重新开始是可接受的；Gauge 类状态必须能从数据库重新计算。 |
| 多 worker | 指标按 `worker_id` 区分 in-flight / claim，同时按 tenant/channel 聚合业务 backlog。 |
| 高基数 tenant / activity | 必须限制标签，错误详情不入标签；必要时文档说明活动维度可配置降级。 |
| Callback 成功但 quota 终结失败 | callback 指标应标记失败原因，如 `quota_aggregate_conflict`，不能只记 HTTP 2xx。 |
| Outbox command SENT 但 callback 未到 | reward command 指标显示 sent，callback 指标无最终成功；二者语义必须区分。 |

## 异常场景

1. Event API payload 非法：`event_intake_total{result="failed",reason="invalid_payload"}` 增加。
2. Idempotency-Key 不匹配：`reason="idempotency_key_mismatch"` 增加。
3. OpenAPI 鉴权失败：`result="auth_failed"` 增加，不记录完整 token。
4. 规则拒绝：可计为 intake `result="rejected"`、`reason="rule_rejected"`，不应误算为系统失败。
5. Quota 不足：quota fail 指标按 quota type 增加，event intake reason 可标记 `quota_exceeded`。
6. Outbox target 超时或拒绝：reward command 指标标记 retry / failed / dlq，outbox backlog 随状态变化。
7. Outbox worker 停止或未启动：claim 指标无增长，backlog age 增长并触发告警。
8. DLQ replay 后再次失败：DLQ total 继续累计，当前 DLQ count 按最终状态反映。
9. Callback 找不到 grant_order：callback 指标 `result="rejected",reason="grant_order_not_found"`。
10. Callback 状态冲突：callback 指标 `result="conflict",reason="status_conflict"`，不得重复移动 quota。
11. Metrics endpoint 被频繁抓取：不得影响主业务明显变慢；抓取本身不应改变业务计数。
12. 指标标签值包含非法字符或空值：必须归一化为安全值，例如 `unknown`，不能导致 endpoint 输出不可解析。

## 非目标范围

1. 不建设完整监控平台、Grafana dashboard、PagerDuty、短信、邮件或 IM 告警集成。
2. 不新增业务状态机，不改变 event、quota、grant_order、outbox、callback 语义。
3. 不新增活动类型、奖励类型、规则 evaluator、quota 类型或 ActivityPlugin。
4. 不要求实现长期指标存储、跨进程聚合服务或分布式 tracing。
5. 不要求把日志系统替换为指标系统；结构化日志仍可保留，但不能替代核心 metrics 输出。
6. 不要求对每个 eventId / commandId 提供指标维度；这些属于日志和对账查询范围。
7. 不要求 metrics endpoint 公网鉴权方案；生产暴露边界由后续安全 / 部署任务处理。
8. 不要求为 simulate API、admin 页面点击、workflow agent 状态建立完整业务指标。

## 风险点

| 风险 | 影响 | 约束 |
| --- | --- | --- |
| 指标标签高基数 | 监控系统内存和存储压力暴涨 | 禁止 eventId、bizId、subjectId、commandId、原始错误等标签。 |
| 只做日志不做指标 | 无法机器告警和聚合 | 必须有 metrics endpoint 或等价机器可读接口。 |
| Counter 与 Gauge 混用 | 重启、重复采集后数据误判 | 累计事件用 Counter，当前状态用 Gauge。 |
| Outbox / DLQ 只看进程内状态 | 重启后 backlog 丢失 | PostgreSQL 模式必须从数据库快照计算。 |
| 指标影响主链路 | 业务请求延迟或失败 | 指标记录失败不得阻断主业务。 |
| 告警阈值过敏 | 低流量误报 | 默认阈值使用失败率 + 最小失败数。 |
| 敏感信息泄露 | token、payload、targetRef 泄露到监控 | 标签和文档必须明确脱敏与有限枚举。 |
| command sent 与 reward success 混淆 | 误以为发奖成功 | reward command latency 与 callback 结果必须分开定义。 |

## 验收标准

1. 提供 `GET /metrics` 或等价可接入指标接口；接口输出可被机器解析。
2. 指标文档列出所有核心指标的名称、类型、单位、标签、枚举和语义。
3. 告警文档列出 event intake 成功 / 失败、quota fail rate、outbox backlog、DLQ count、callback fail rate、worker claim latency、reward command latency 的建议阈值。
4. Event API 成功、重复、payload 错误、幂等键错误、鉴权失败或等价失败路径能反映到 intake 指标。
5. Quota 成功和 quota exceeded 能反映到 quota attempt / fail 指标，并至少区分 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
6. PostgreSQL 模式下 outbox backlog gauge 能反映 `PENDING`、到期 retry、`SENDING`、`DLQ` 的当前数量。
7. DLQ 当前数量和新进入 DLQ 累计数量可观测，`DLQ_CLOSED` 不计入未处理 DLQ count。
8. Callback 成功、失败、重复、冲突、找不到单据、quota 终结失败能反映到 callback 指标。
9. Worker claim latency 可观测，且能验证 oldest eligible outbox 被 claim 后产生延迟样本。
10. Reward command 从 outbox 创建到投递终态的 latency 可观测，且不与 callback 最终发奖结果混淆。
11. 指标不包含 token、Authorization、Cookie、secret、完整 targetRef、完整 payload、eventId、bizId、subjectId、billNo、commandId 或原始错误文本作为标签。
12. 指标采集接口在无业务流量时可正常返回，不产生虚假非零计数。
13. 服务重启后，Gauge 类 backlog / DLQ 指标能基于 PostgreSQL 当前状态恢复；Counter 重启清零可接受并在文档中说明。
14. Outbox worker 未开启时，backlog 指标仍显示堆积，worker claim 指标无新增 claim。
15. `go test ./...` 和 `go build ./...` 应通过。
16. PostgreSQL smoke 或集成测试必须覆盖：事件成功、quota exceeded、outbox backlog、进入 DLQ、callback failed、claim latency、command latency 至少各一个可验证样本。
17. 运维文档更新排查入口，至少包含指标路径、示例查询 / curl、阈值表和常见告警处理建议。

## 待确认问题

以下问题不阻塞本期开发，默认口径已在上文给出：

1. 正式监控平台是 Prometheus、云监控还是日志指标系统？本期默认 `GET /metrics` Prometheus 文本或等价机器可读接口。
2. 是否需要按业务线配置不同告警阈值？本期默认全局阈值，后续可按 tenant / activity 调整。
3. Metrics endpoint 是否需要鉴权？本期默认仅内网 / 本地运维暴露，公网鉴权不纳入本需求。
4. `activity_id` 是否允许作为 quota 指标标签？本期默认允许，用于排查额度失败；若监控高基数压力过大，后续可配置降级。

## Score

```text
92
```

评分：

```text
需求清晰度        19/20
业务规则完整      18/20
边界异常清楚      19/20
验收标准明确      18/20
约束一致性        18/20
```

扣分说明：

- 正式监控平台和 endpoint 鉴权策略仍需后续部署 / 安全任务确定，但已给出本期默认可验收口径，不阻塞开发。
- 告警阈值为初始建议，真实生产阈值需要上线后按流量校准。

## Decision

```text
PASS
```

需求：2026-05-27-engine-metrics-alerts
