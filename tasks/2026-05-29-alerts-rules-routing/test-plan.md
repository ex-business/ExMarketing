# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

本轮只设计后续 Test Agent 的验证范围，不执行任何测试命令。

测试目标：

1. 验证告警规则覆盖 `DLQ`、`callback`、`outbox`、`worker`、`DB write`、`quota` 六类 P0/P1 核心异常，以及 `event intake`、`claim latency`、`retry`、`Fact Query` P2 趋势异常。
2. 验证告警表达式可被静态校验，并能通过 promtool 单元输入触发、恢复和保持静默。
3. 验证路由、分组、升级、抑制和消息模板字段完整，尤其是 `service`、`env`、`tenant`、`channel`、`severity`、`summary`、`dashboard`、`runbook`。
4. 验证告警指标契约不使用高基数或敏感标签，不把 `eventId`、`bizId`、`subjectId`、`activityId`、`idempotencyKey`、原始错误文本等放入指标或告警标签。
5. 通过业务 smoke 覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL 写入/查询可观测性，确保告警规则依赖的指标在真实链路中可产生。

## Test Cases

### TC-01 静态校验：规则文件和 Alertmanager 路由语法

前置条件：Dev Agent 已交付告警规则、Alertmanager 或等价路由配置、promtool 单元测试文件。

步骤：

1. 使用 promtool 校验 Prometheus rule 文件语法。
2. 使用 promtool 校验 rule unit test 文件。
3. 使用 Alertmanager 校验工具或等价命令校验 route、receiver、inhibit、template 配置。
4. 检查所有规则都有 `severity`、`service`、`env`、`summary`、`dashboard`、`runbook` annotation 或 label。

预期结果：

- 静态校验全部通过。
- 不存在空 receiver、不可达 route、缺失模板字段或拼写错误的 label matcher。
- 不存在高基数或敏感标签。

### TC-02 正常路径：活动事件成功发奖不触发核心告警

步骤：

1. 准备一个有效 tenant、活动规则、quota、reward channel 和 PostgreSQL 数据。
2. 发送一条唯一 idempotency key 的正常 event。
3. 等待规则匹配、quota reserve、grant order 创建、outbox 发送、callback 成功、quota confirm 完成。
4. 采集 `/metrics`，记录 event、quota、grant order、outbox、callback、DB write、Fact Query 指标。
5. 将正常指标序列输入 promtool rule test。

预期结果：

- 业务结果为发奖成功。
- 不触发 DLQ、callback failure、outbox backlog、worker stopped、DB write failure、quota overspend、event intake error、Fact Query failure 告警。
- 成功路径指标包含低基数维度，且模板字段可以定位 dashboard/runbook。

### TC-03 额度不足：quota exhausted 不得误报超扣 P0

步骤：

1. 设置活动 quota 可用额度为 0 或低于本次发奖所需额度。
2. 发送符合活动规则的 event。
3. 验证业务返回或落库状态为额度不足、拒绝或未发奖。
4. 检查 quota 相关指标的 `result/reason` 是否使用枚举值，例如 `quota_exceeded`。
5. 用指标序列验证 quota 正常拒绝不会触发 `QuotaOverspendRisk` P0；若实现有 P2/P3 quota exhausted 趋势告警，则只允许按设计阈值触发低级别告警。

预期结果：

- 不创建成功 grant order，不产生超扣。
- 不触发 DB write failure 或 quota overspend P0。
- 告警维度保留 `tenant` 和必要 scope，但不包含 subject/user/event 级高基数字段。

### TC-04 幂等重复：重复 event 不产生重复发奖或告警噪声

步骤：

1. 使用相同 idempotency key 或业务幂等键连续发送同一 event 两次以上。
2. 验证只有首次请求推进完整发奖链路，重复请求命中幂等结果。
3. 检查 participation、grant order、outbox 记录没有重复创建。
4. 检查相关 metrics 中重复请求按 `idempotent_duplicate` 或等价枚举计数。
5. 用重复请求指标序列验证不会触发 event intake error、DB write failure、outbox backlog、callback failure 告警。

预期结果：

- 业务幂等成立。
- 重复请求不会造成 DLQ、retry 激增或重复通知。
- 告警模板和指标不暴露 idempotency key 原文。

### TC-05 并发：同一 quota 下并发请求不超扣

步骤：

1. 设置一个有限 quota，例如只允许成功 N 次。
2. 并发发送明显大于 N 的有效 event，使用不同幂等键。
3. 等待处理完成后统计成功发奖、额度不足、失败和 retry 结果。
4. 检查 PostgreSQL quota/reservation/grant order 状态一致。
5. 检查 quota 指标和 DB write 指标：正常冲突或额度拒绝不能被归类为 DB P0；真实写失败才可触发 DB 告警。
6. 用构造指标验证当实际成功数超过 quota 或出现 reservation 聚合冲突持续存在时，quota 超扣风险告警触发并路由到 backend-oncall。

预期结果：

- 成功发奖数不超过 quota。
- 并发冲突被稳定处理，无重复 grant order。
- 只有真实超扣风险触发 P0；正常额度竞争不触发 P0。

### TC-06 PostgreSQL smoke：核心写路径指标覆盖真实 DB 操作

步骤：

1. 在真实 PostgreSQL 测试环境执行一条正常 event。
2. 验证核心写路径至少覆盖：event inbox、participation、grant order、quota reserve/finalize、outbox write、callback write。
3. 临时使用测试隔离方式制造一个可恢复 DB write failure，例如只在测试 schema/table/transaction 中触发约束或连接错误。
4. 采集 `exmarketing_db_operation_total` 或等价 DB write metrics。
5. 用指标序列验证 `ExMarketingDBWriteFailure` 只基于核心写路径触发，不被 dashboard/backlog/DLQ 查询失败误触发。

预期结果：

- PostgreSQL smoke 正常路径通过。
- DB write metrics 具备 `operation`、`table`、`result`、枚举 `reason`。
- DB P0 触发后可抑制同维度 outbox/callback/worker/event/Fact Query 泛洪告警，但不抑制 quota P0 和 DB P0 本身。

### TC-07 DLQ 持续增长和路由维度

步骤：

1. 构造 `tenant_id` + `target_channel` 维度下 DLQ 持续增长的指标序列。
2. 构造另一个 tenant/channel 正常或低增长序列。
3. 运行 promtool unit test。
4. 检查告警 label 映射为 `tenant`、`channel`，且 receiver 包含 backend-oncall 和必要 business-ops。

预期结果：

- 只有增长超过阈值且持续满足 for 条件的维度触发。
- 不允许全局 sum 丢失 tenant/channel。

### TC-08 Callback 失败率高

步骤：

1. 构造 callback success/failed counter，使单个 tenant/channel 的失败率超过 P1 阈值并满足持续时间。
2. 构造低流量偶发失败，验证不触发或只触发 P3 观察告警。
3. 检查告警 summary、description、dashboard、runbook。

预期结果：

- 高失败率触发 P1 并路由 backend-oncall，业务影响项同步 business-ops。
- 低流量偶发失败不产生 P1 噪声。

### TC-09 Outbox backlog 超阈值与 worker 全停

步骤：

1. 构造 eligible backlog 持续超过阈值且 claim success 为 0 的序列。
2. 构造 backlog 存在但 claim success 正常的序列。
3. 构造服务 scrape down 的序列，验证由 scrape target 告警负责，不与 worker 全停规则混淆。

预期结果：

- `WorkerAllStopped` 仅在 eligible backlog > 0 且 claim success 持续为 0 时触发。
- outbox backlog 告警和 worker 全停告警能被 DB P0 按精确 label 抑制。

### TC-10 Event Intake、Retry、Claim Latency、Fact Query P2

步骤：

1. 构造 event intake error rate 高于 P2 阈值的 counter。
2. 构造 retry 激增序列。
3. 构造 claim latency histogram p95 高于阈值的序列。
4. 构造 Fact Query `failed/timeout/provider_unavailable/mapping_failed` 失败率高的序列。
5. 分别运行 promtool unit test，检查 receiver、severity 和 runbook。

预期结果：

- 四类 P2 均可触发并恢复。
- Fact Query 规则必须依赖 `exmarketing_fact_query_total` / `exmarketing_fact_query_latency_seconds` 或交付物声明的等价指标；缺失指标时该测试必须 FAIL。

### TC-11 抑制、静默、维护窗口和重复通知

步骤：

1. 构造同一 `service/env/tenant/channel` 下 DB P0 与 outbox/callback/worker/event/Fact Query 告警同时触发。
2. 验证 Alertmanager inhibition 只抑制同根因和同维度的下游泛洪告警。
3. 构造不同 tenant/channel 的独立外部 provider outage 或 reward channel outage，验证不被过宽 DB 抑制隐藏。
4. 创建维护窗口 silence，使用 `service/env/tenant/channel/severity/alertname` matcher。
5. 检查 repeat_interval 和 escalation 设计。

预期结果：

- 抑制规则精确，不隐藏独立故障。
- 维护窗口只静默预期范围内告警。
- P0/P1/P2/P3 的重复通知和升级策略符合 analysis.md。

## Required Commands

~~~bash
# 静态规则校验，路径以 Dev Agent 实际交付为准
promtool check rules <prometheus-rule-file.yml>
promtool test rules <prometheus-rule-unit-test-file.yml>

# Alertmanager 或等价路由配置校验，路径以 Dev Agent 实际交付为准
amtool check-config <alertmanager-config.yml>

# Go 单元/集成测试，后续 Test Agent 执行
go test ./...
go build ./...

# 业务 smoke 命令，后续 Test Agent 按项目实际脚本替换或补齐参数
<start-postgresql-test-env-command>
<start-exmarketing-test-service-command>
<run-normal-event-smoke-command>
<run-quota-insufficient-smoke-command>
<run-idempotency-duplicate-smoke-command>
<run-concurrency-quota-smoke-command>
<run-postgresql-write-metrics-smoke-command>
~~~

## Data / Smoke

后续 Test Agent 需要准备以下测试数据：

1. 正常活动：tenant、campaign/rule、reward channel、quota、callback endpoint 全部有效。
2. 额度不足活动：相同规则但 quota 为 0 或低于发奖额度。
3. 幂等事件：同一业务事件可重复提交，幂等键固定。
4. 并发事件：同 tenant、同 quota scope、不同幂等键，数量大于 quota。
5. PostgreSQL smoke 数据：独立测试 schema 或可回滚 fixture，覆盖核心写路径和可恢复 DB write failure。
6. Promtool fixture：每条核心/P2 告警至少包含 firing、not firing、resolved 三类时间序列。
7. Alertmanager fixture：覆盖 P0/P1/P2/P3 route、receiver、inhibit、silence matcher、repeat/escalation 字段。

Smoke 判定：

- 正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke 全部通过，才允许测试结论为 PASS。
- 如果 DB write metrics 或 Fact Query metrics 未实现且无等价映射，测试必须 BLOCK。
- 如果告警规则静态校验失败、模板字段缺失、路由不可达或抑制规则过宽，测试必须 BLOCK。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-alerts-rules-routing
