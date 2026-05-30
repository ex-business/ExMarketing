# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

Review Decision 已确认为 PASS。本测试计划只定义后续 Test Agent 应执行的用例和命令清单，不在本阶段执行任何测试、构建、格式化或 git 命令。

测试目标：

1. 验证 Prometheus scrape 样例、Grafana dashboard / 面板定义、指标命名与标签规范、本地验证说明均已交付。
2. 验证 `/metrics` 或等价 endpoint 可被 Prometheus text format scrape，且不暴露 secret、token、用户隐私或禁止的高基数标签。
3. 验证 dashboard 覆盖 event、rule、reward、quota、outbox、callback、DLQ、worker、DB，并且 Rule/reward/DB 不是占位面板。
4. 验证 event、quota、callback 的失败率健康判断使用 5 分钟窗口下“失败率 + 最小失败数”双条件。
5. 验证正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 五类核心行为都会产出可观测指标，并能被 dashboard / PromQL 查询。

后续 Test Agent 应先做静态检查，再做本地运行 smoke，最后在 PostgreSQL 环境跑集成 smoke。若实现尚未完成或缺少启动说明，Test Agent 应将对应项记录为 FAIL，而不是跳过。

## Test Cases

### 1. 交付物静态检查

步骤：

1. 检查是否存在 Prometheus scrape 配置样例或接入说明。
2. 检查是否存在 Grafana dashboard JSON 或完整面板定义。
3. 检查是否存在指标命名、标签、单位、统计窗口、阈值和高基数字段限制说明。
4. 检查是否存在本地验证说明，且能覆盖启动服务、访问 `/metrics`、配置 Prometheus、导入 Grafana、确认核心查询可执行。

期望：

1. scrape 样例包含 job name、metrics path、scrape interval、static target。
2. dashboard 不硬编码生产 URL、secret、token 或本机绝对私有路径。
3. 文档明确 Counter、Gauge、5 分钟窗口、scrape interval、PostgreSQL 为强验收环境。

### 2. Metrics endpoint 和 scrape 格式

步骤：

1. 按实现文档启动 ExMarketing 服务。
2. 请求本地 `/metrics` 或文档指定的等价 endpoint。
3. 使用 Prometheus scrape 样例启动或配置 Prometheus，确认 target 为 healthy。
4. 检查 metrics 输出是否为 Prometheus text format。

期望：

1. `/metrics` 返回 2xx。
2. Prometheus target `up{job="exmarketing"}` 为 1。
3. 输出包含 event、rule、reward、quota、outbox、callback、worker、DB 所需指标或等价映射。
4. 无业务流量时 Counter rate 可为 0，但 `up` 和 Gauge 面板不应误判为服务故障。

### 3. 高基数和敏感信息治理

步骤：

1. 扫描 `/metrics` 输出、dashboard JSON / PromQL、scrape 配置和文档示例。
2. 检查是否出现禁止标签或敏感字段。
3. 特别检查 quota 指标和 dashboard 查询是否仍依赖 `activity_id` / `activityId`。

禁止出现：

```text
token
Authorization
Cookie
secret
password
payload
targetRef
eventId
bizId
subjectId
userId
activityId
activity_id
billNo
commandId
idempotencyKey
```

期望：

1. 可 scrape 指标、PromQL `by(...)` / selector、dashboard 变量均不使用禁止标签。
2. `reason` 使用枚举值，不直接暴露原始异常文本。
3. quota 面板使用低基数指标，按 `tenant_id`、`quota_type`、`scope`、`action`、`result`、`reason` 等维度聚合。

### 4. Grafana dashboard 覆盖和 PromQL 检查

步骤：

1. 导入 dashboard JSON，绑定 Prometheus datasource。
2. 打开 Overview、Event intake、Rule / reward、Quota、Outbox / DLQ、Callback、Worker、DB 分组。
3. 在 Grafana Explore 或 Prometheus UI 中执行核心 PromQL。
4. 检查失败率类查询是否同时体现失败率阈值与最小失败数门槛。

期望：

1. Overview 至少包含 `up`、event failed rate、quota reserve fail rate、outbox backlog / oldest age、DLQ count、callback failed rate、worker claim latency、DB error / slow operation。
2. Rule/reward/DB 面板有真实指标或等价指标映射，不允许只显示“待接入”。
3. event Red 条件为失败率 > 5% 且 5 分钟失败数 >= 20，或 internal_error 5 分钟内 >= 1。
4. quota reserve Red 条件为失败率 > 20% 且 5 分钟失败数 >= 10。
5. callback Red 条件为失败率 > 5% 且 5 分钟失败数 >= 5。

### 5. 正常路径 smoke

步骤：

1. 使用文档提供的本地或测试环境启动方式启动服务和 PostgreSQL 依赖。
2. 发送一条合法 event，触发 rule 执行、reward 生成、quota reserve/confirm、outbox worker 发送。
3. 模拟或执行成功 callback。
4. 观察 `/metrics`、Prometheus 和 Grafana dashboard。

期望：

1. event intake success 增长，失败率不升高。
2. rule / reward execution success 增长，阶段耗时 histogram 有样本。
3. quota reserve/confirm success 增长，冻结量、确认量有合理变化。
4. outbox backlog 最终回落，DLQ count 为 0。
5. callback success 增长，duplicate/conflict/failed 不异常增长。
6. worker claim 成功，claim latency 和 in-flight 可观察。
7. DB operation counter 和 latency histogram 有样本，DB error 不增长。

### 6. 额度不足场景

步骤：

1. 构造一个会触发 quota reserve 失败的 event，例如超出剩余额度或库存不足。
2. 连续发送足够数量的额度不足请求，使失败数达到或接近阈值，用于观察双条件逻辑。
3. 查询 quota 相关指标和 dashboard。

期望：

1. quota reserve fail counter 增长，`reason` 为枚举化的 insufficient / quota_exceeded 等低基数原因。
2. quota reserve 失败率面板可显示失败率。
3. 未达到 5 分钟失败数 >= 10 时，不应仅因低流量 100% 失败率进入 Red。
4. 达到失败率 > 20% 且 5 分钟失败数 >= 10 后，Overview / Quota 面板进入 Red。
5. 指标中不出现 `activity_id`、`subjectId`、`userId`、原始错误文本。

### 7. 幂等重复场景

步骤：

1. 使用相同幂等键或相同业务唯一键重复发送 event。
2. 重复发送同一 reward callback。
3. 查询 event intake、callback、quota、outbox 指标。

期望：

1. event duplicate / rejected / idempotent result 指标增长，具体口径按实现文档定义。
2. callback duplicate 指标增长。
3. 重复请求不导致 quota 重复 reserve / confirm，不导致 outbox 重复发送，不导致 DB 状态重复推进。
4. duplicate 被单独展示，不应错误计入 callback failed rate 的分子，除非实现明确把冲突类重复定义为失败。
5. dashboard 能区分 duplicate、conflict、failed、rejected。

### 8. 并发场景

步骤：

1. 并发发送多条合法 event，覆盖同一 tenant、相同 quota scope 和多个 event type。
2. 并发发送可能竞争同一 quota 的请求，混合成功与额度不足。
3. 观察 worker、quota、DB、outbox、callback 指标。

期望：

1. 服务无 panic、无明显数据竞争症状，业务响应符合实现预期。
2. quota 成功与失败计数总量与请求结果一致。
3. worker claim conflict / empty batch / success 指标可观察。
4. outbox backlog 不持续堆积，oldest eligible age 不超过 Red 阈值。
5. DB latency P95/P99 可观察；DB error 不因并发持续增长。
6. dashboard 不按高基数字段展开并发请求明细。

### 9. PostgreSQL smoke

步骤：

1. 使用 PostgreSQL 配置启动服务，确认不是内存仓储。
2. 执行正常路径、额度不足、幂等重复和并发 smoke 的最小组合。
3. 查询 PostgreSQL 相关状态，确认 outbox、DLQ、ledger/quota 或等价表状态合理。
4. 观察 DB metrics。

期望：

1. `exmarketing_db_operation_total` 或等价 DB 操作指标存在。
2. `exmarketing_db_operation_latency_seconds` 或等价 DB latency histogram 存在，并能查询 P95/P99。
3. DB error、slow operation 指标存在；正常 smoke 下不持续增长。
4. outbox backlog / DLQ Gauge 与 PostgreSQL 状态一致。
5. 若 PostgreSQL 环境无法启动或配置缺失，Test Agent 必须记录为阻塞或失败，并说明影响范围。

### 10. 异常路径和健康颜色

步骤：

1. 停止 ExMarketing 服务或指向不可用 target，观察 Prometheus target。
2. 构造 event internal_error。
3. 构造 outbox 堆积或 DLQ 记录。
4. 构造 callback failed / conflict。

期望：

1. 服务不可用时 `up=0`，Overview 为 Red。
2. event `internal_error` 5 分钟内 >= 1 时显示 Red。
3. DLQ count > 0 显示 Red；单 tenant/channel DLQ >= 10 时显示 Red。
4. outbox oldest eligible age 60s 到 120s 为 Yellow，> 120s 为 Red。
5. callback failed rate 双条件生效。

## Required Commands

后续 Test Agent 应按实现文档和本地环境实际端口调整命令。以下是必须执行或等价执行的命令清单，本阶段不得执行。

~~~bash
go test ./...
go build ./...
~~~

静态检查命令：

~~~bash
python3 -m json.tool monitoring/grafana-exmarketing-dashboard.json >/dev/null
rg -n "activity_id|activityId|eventId|subjectId|userId|bizId|billNo|commandId|idempotencyKey|Authorization|Cookie|token|secret|password" monitoring/grafana-exmarketing-dashboard.json monitoring/prometheus-exmarketing.yml && exit 1 || true
rg -n "exmarketing_event_intake|exmarketing_rule|exmarketing_reward|exmarketing_quota|exmarketing_outbox|exmarketing_reward_callback|exmarketing_worker|exmarketing_db" monitoring/grafana-exmarketing-dashboard.json docs/operations.md
rg -n "rate\\(|increase\\(|histogram_quantile\\(|up\\{" monitoring/grafana-exmarketing-dashboard.json docs/operations.md
~~~

本地 metrics / Prometheus smoke 命令：

~~~bash
curl -fsS http://localhost:8080/metrics | head -n 80
curl -fsS http://localhost:8080/metrics | rg -n "activity_id|activityId|eventId|subjectId|userId|bizId|billNo|commandId|Authorization|Cookie|token|secret|password" && exit 1 || true
curl -fsS http://localhost:9090/api/v1/targets
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=up{job="exmarketing"}'
~~~

PromQL 验证命令示例：

~~~bash
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(rate(exmarketing_event_intake_total{result="failed"}[5m])) / clamp_min(sum(rate(exmarketing_event_intake_total[5m])), 1)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(increase(exmarketing_event_intake_total{result="failed"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(rate(exmarketing_quota_fail_total{action="reserve"}[5m])) / clamp_min(sum(rate(exmarketing_quota_attempt_total{action="reserve"}[5m])), 1)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(increase(exmarketing_quota_fail_total{action="reserve"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(rate(exmarketing_reward_callback_total{result="failed"}[5m])) / clamp_min(sum(rate(exmarketing_reward_callback_total[5m])), 1)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum(increase(exmarketing_reward_callback_total{result="failed"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum(rate(exmarketing_db_operation_latency_seconds_bucket[5m])) by (le, operation))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.99, sum(rate(exmarketing_db_operation_latency_seconds_bucket[5m])) by (le, operation))'
~~~

## Data / Smoke

后续 Test Agent 应准备以下数据或等价 fixture：

1. 正常 event：合法 tenant、source_system、event_type、subject / business key、可命中的 rule、可生成 reward、quota 足够。
2. 额度不足 event：同一 quota scope 下超出额度或库存，能稳定触发 reserve failed。
3. 幂等重复 event：与正常 event 使用相同业务唯一键或幂等键，重复发送后不重复发奖。
4. 重复 callback：同一 reward command / grant order 的成功 callback 重放。
5. callback conflict：同一 reward command 先成功后发送冲突状态，或按实现定义构造冲突。
6. 并发数据：至少 20 个并发请求，混合正常、额度不足和重复请求。
7. PostgreSQL smoke 数据：确认 outbox、DLQ、quota / ledger、callback 状态可在 PostgreSQL 中核对。

建议执行顺序：

1. 启动 PostgreSQL 和服务。
2. 跑正常 event 一次，确认全链路成功。
3. 跑额度不足，确认 quota fail 指标和低流量阈值行为。
4. 跑幂等重复，确认 duplicate 指标和无重复副作用。
5. 跑并发，确认 worker / outbox / DB 指标。
6. 构造 DLQ / callback conflict / service down 异常，确认健康颜色和 Red 条件。

Test Agent 在 test.md 中应记录：

1. 每个命令的执行结果。
2. 每个 smoke 场景的请求数量、成功数、失败数和关键指标查询结果。
3. PostgreSQL 状态核对结论。
4. 未执行项、阻塞原因和风险影响。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-alerts-prometheus-grafana-prometheus-grafana
