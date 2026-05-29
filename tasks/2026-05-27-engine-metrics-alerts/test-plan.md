# Test Plan

## Agent 指令

你是 Test Plan Agent。已确认 `review.md` 的 `Decision` 为 `PASS`。

本文件只设计后续 Test Agent 应执行的测试用例和命令清单。本轮未执行测试、构建、格式化或 git 命令。

## Test Strategy

测试目标：验证 engine-metrics-alerts 交付是否满足 `analysis.md` 和 `review.md` 的指标、告警、PostgreSQL 快照和敏感信息约束。

测试分层：

1. 静态交付物检查：确认存在 metrics endpoint 或等价机器可读接口、指标文档、告警阈值文档，并检查指标名、类型、标签、枚举、单位和语义。
2. 单元测试：覆盖指标 recorder / collector 的 Counter、Gauge、Histogram 记录逻辑，尤其是 result / reason 枚举和敏感标签过滤。
3. API / 集成测试：通过 Event API、Quota、Outbox worker、Callback 等真实链路触发指标变化。
4. PostgreSQL smoke：使用 PostgreSQL 仓储验证 backlog、DLQ、oldest eligible age、重启恢复和 worker 未开启时的 Gauge 快照。
5. 并发与幂等：验证重复请求、并发 claim、并发 quota 预留不会产生错误业务状态，指标也不会误计或漏计核心结果。

必须重点核对 Review P2：Quota fail rate 的分母不得被 confirm / release 稀释。若实现使用 `exmarketing_quota_attempt_total`，测试必须确认存在 `action=reserve|confirm|release` 标签，或确认 fail rate 只统计 reserve attempt。

## Test Cases

### 1. Metrics Endpoint / 可解析性

步骤：

1. 启动服务后请求 `GET /metrics`，或实现文档声明的等价指标接口。
2. 使用 Prometheus text parser 或项目内解析工具验证输出格式可机器解析。
3. 在无业务流量时再次抓取指标。

期望：

- Endpoint 返回 2xx，不依赖业务流量。
- 输出包含 `exmarketing_` 前缀核心指标，或文档明确列出等价映射。
- 无流量时 Counter 为 0 或未出现该维度，Gauge 为 0 或当前真实状态，不产生虚假非零错误计数。
- 抓取 metrics 本身不改变 event / quota / callback / command 业务计数。

### 2. 指标文档和告警文档

步骤：

1. 检查交付文档是否列出指标名、类型、单位、标签、枚举、触发点和示例。
2. 检查告警文档是否覆盖 event intake failed rate、quota fail rate、outbox backlog、DLQ count、callback fail rate、worker claim latency、reward command latency。
3. 检查默认阈值是否包含统计窗口、严重级别、最小失败数或降噪规则。

期望：

- 文档覆盖 `analysis.md` 七类核心指标。
- 告警阈值至少包含 5 分钟失败率、backlog oldest age、DLQ > 0、worker claim P95、reward command P95。
- 文档说明 metrics endpoint 的访问路径和生产暴露边界，不声明可直接公网暴露。

### 3. 正常路径：Event -> Quota -> Outbox -> Worker -> Callback

步骤：

1. 准备 ACTIVE 活动、充足 quota、有效 reward target 和 PostgreSQL 仓储。
2. 提交合法 Event API 请求。
3. 启动或触发 Outbox worker 投递 reward command。
4. 发送合法成功 callback。
5. 抓取 metrics。

期望：

- `exmarketing_event_intake_total{result="success",reason="ok"}` 增加。
- `exmarketing_event_intake_latency_seconds` 产生成功样本。
- `exmarketing_quota_attempt_total` 记录 reserve 成功；如实现 action 标签，应包含 `action="reserve"`。
- `exmarketing_reward_command_total{result="sent"}` 增加。
- `exmarketing_reward_command_latency_seconds` 产生样本，且语义为 outbox 创建到投递终态，不代表最终发奖成功。
- `exmarketing_reward_callback_total{result="accepted",reason="ok"}` 增加。
- `exmarketing_reward_callback_latency_seconds` 产生成功样本。
- Outbox backlog / DLQ Gauge 最终回到符合数据库状态的值。

### 4. 额度不足路径

步骤：

1. 准备 ACTIVE 活动，并设置 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 中至少一种 quota 已耗尽或不足。
2. 提交合法 Event API 请求。
3. 抓取 metrics，并检查业务响应。

期望：

- Event 被业务拒绝，不能生成不应存在的 reward command。
- `exmarketing_event_intake_total` 增加，`reason="quota_exceeded"`，result 按实现文档为 `rejected` 或等价值。
- `exmarketing_quota_fail_total{reason="quota_exceeded"}` 增加。
- `exmarketing_quota_attempt_total` 至少区分 `quota_type`，并按实现约定记录 reserve attempt。
- Quota fail rate 的测试口径只用 reserve attempts 做分母，或通过 `action="reserve"` 过滤；confirm / release 不得稀释 quota exceeded 失败率。

### 5. 幂等重复路径

步骤：

1. 使用同一幂等键提交一次合法 Event API 请求。
2. 使用完全相同请求再次提交。
3. 使用相同幂等键但不同 payload 再提交一次。
4. 抓取 metrics。

期望：

- 第二次相同请求返回幂等重复结果，不重复创建 grant order / outbox command。
- `exmarketing_event_intake_total{result="duplicate",reason="duplicate"}` 或等价重复指标增加。
- 第三次 payload mismatch 被拒绝，`reason="idempotency_key_mismatch"` 增加。
- 重复请求不重复增加 quota reserve 成功、reward command sent 或 callback accepted 计数。

### 6. Callback 重复、冲突和失败

步骤：

1. 对已投递的 grant order 发送成功 callback。
2. 重复发送相同 callback。
3. 对同一 grant order 发送冲突状态 callback。
4. 发送 grant order 不存在、签名无效或 tenant mismatch 的 callback。
5. 构造 callback 成功入参但 quota 终结失败场景。

期望：

- 首次成功：`exmarketing_reward_callback_total{result="accepted",reason="ok"}` 增加。
- 重复 callback：`result="duplicate"` 增加，不算入失败率分子。
- 状态冲突：`result="conflict",reason="status_conflict"` 增加。
- 找不到单据或鉴权失败：`result="rejected"`，reason 为 `grant_order_not_found`、`signature_invalid`、`auth_failed` 或等价枚举。
- Quota 终结失败：`reason="quota_aggregate_conflict"` 或实现文档中的等价有限枚举，不得只因 HTTP 2xx 记为成功。

### 7. Outbox Backlog / DLQ

步骤：

1. 在 PostgreSQL 中通过业务路径生成 `PENDING` outbox。
2. 停止或不启动 worker，等待 oldest eligible age 增长。
3. 构造外部 reward target 超时或持续失败，使 outbox 进入 retry，再达到 DLQ。
4. 将一条 DLQ 标记为 `DLQ_CLOSED`。
5. 抓取 metrics。

期望：

- `exmarketing_outbox_backlog{send_status="PENDING"}` 反映当前待处理数量。
- 到期 retry 的 `FAILED` 计入可处理 backlog；未到期 retry 不应计入 oldest eligible backlog。
- `exmarketing_outbox_oldest_eligible_age_seconds` 随最老可投递记录等待时间增长。
- `exmarketing_outbox_dlq_count` 只统计未关闭 DLQ。
- `exmarketing_outbox_dlq_total` 在新进入 DLQ 时递增。
- `DLQ_CLOSED` 不计入未处理 DLQ count。

### 8. Worker Claim Latency

步骤：

1. 准备一条已 eligible 一段时间的 outbox 记录。
2. 启动 worker claim。
3. 抓取 metrics。

期望：

- `exmarketing_worker_claim_total{result="success"}` 增加。
- `exmarketing_worker_claim_latency_seconds` 产生样本，值接近 eligible 到 claim 的等待时长。
- `exmarketing_worker_inflight` 在处理期间增加，处理完成后回落。
- 空批次或 claim conflict 按有限 `result` 枚举记录，不进入业务失败计数。

### 9. 并发：Quota Reserve 和 Worker Claim

步骤：

1. 配置有限 quota，例如库存只允许 N 个成功。
2. 并发提交大于 N 个合法 Event API 请求。
3. 并发启动两个或更多 worker claim 同一 tenant/channel 的 outbox。
4. 抓取 metrics 并检查数据库最终状态。

期望：

- 成功事件数量不超过 quota 限额。
- 超额请求进入 quota exceeded 路径，quota fail 指标准确增加。
- 不出现重复 outbox sent、重复 grant order 终态推进或负 quota。
- 多 worker 下同一 outbox 不被重复 claim / send。
- `exmarketing_worker_claim_total` 能区分 success、conflict、empty 或实现文档中的等价结果。

### 10. PostgreSQL Smoke / 重启恢复

步骤：

1. 使用 PostgreSQL 仓储运行一组最小链路：成功事件、quota exceeded、outbox backlog、DLQ、callback failed、claim latency、command latency。
2. 抓取 metrics 保存关键数值。
3. 重启服务进程，不清理 PostgreSQL。
4. 再次抓取 metrics。

期望：

- PostgreSQL smoke 覆盖每个核心指标至少一个可验证样本。
- 重启后 Counter 从进程生命周期重新开始是可接受的。
- 重启后 Gauge 类 backlog、DLQ count、oldest eligible age 基于 PostgreSQL 当前状态恢复。
- Worker 未开启时，backlog 仍可见，claim 指标无新增 claim。

### 11. 敏感信息和高基数标签

步骤：

1. 提交包含 Authorization、Cookie、secret、targetRef、payload attributes、eventId、bizId、subjectId、billNo、commandId、原始错误文本的请求或错误场景。
2. 抓取 metrics 文本。
3. 搜索上述敏感值和高基数字段值。

期望：

- 指标标签中不得出现 token、Authorization、Cookie、secret、完整 payload、完整 targetRef、eventId、bizId、subjectId、billNo、commandId 或原始错误文本。
- 错误原因必须归一化为有限枚举，例如 `invalid_payload`、`internal_error`、`unknown`。
- 空 tenant、source、channel 等标签按文档归一化为安全值，不破坏 Prometheus 格式。

## Required Commands

后续 Test Agent 应按实际项目脚本名称确认并执行。若命令不存在，应记录替代命令和原因。

~~~bash
go test ./...
go build ./...
~~~

建议的定向命令清单：

~~~bash
go test ./... -run 'Metrics|Metric|Prometheus|Recorder|Collector'
go test ./... -run 'Event|Intake|Idempot|Quota'
go test ./... -run 'Outbox|Worker|Claim|DLQ|Callback'
go test ./... -race -run 'Quota|Worker|Claim|Idempot'
~~~

PostgreSQL smoke 命令由实现落地后的脚本决定，Test Agent 应优先查找并执行项目已有脚本，例如：

~~~bash
go test ./... -run 'Postgres|PostgreSQL|Smoke|Integration'
~~~

如项目提供 docker compose 或 make 入口，Test Agent 应记录并执行对应 PostgreSQL smoke 命令，但不得跳过 `go test ./...` 和 `go build ./...`。

## Data / Smoke

PostgreSQL smoke 最小数据集：

1. 一个 tenant。
2. 一个 ACTIVE 活动。
3. 四类 quota 中至少覆盖一个成功 quota 和一个 quota exceeded；若实现已有 fixture，应扩展覆盖 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 的指标标签。
4. 一个合法事件 payload。
5. 一个重复幂等键 payload。
6. 一个幂等键相同但 payload 不同的请求。
7. 一个 reward target 成功响应。
8. 一个 reward target 持续失败或超时响应，用于进入 DLQ。
9. 一个成功 callback、一个重复 callback、一个失败或冲突 callback。
10. 至少一条已有 eligible outbox，用于 claim latency。

Smoke 必须记录：

- metrics endpoint URL 或等价接口路径。
- 每个核心指标抓取前后的样本片段。
- PostgreSQL 中 outbox / DLQ / callback / grant order 状态与 Gauge 的对应关系。
- 并发测试的请求数、成功数、quota exceeded 数、worker 数和最终数据库状态。

## Block Conditions

后续 Test Agent 遇到以下任一情况应判定测试失败或阻塞：

1. `review.md` 不再是 PASS。
2. 没有 metrics endpoint 或等价机器可读接口。
3. 文档缺失七类核心指标或告警阈值。
4. 正常路径、额度不足、幂等重复、并发或 PostgreSQL smoke 任一强制场景无法验证。
5. PostgreSQL 模式下 backlog / DLQ Gauge 只来自进程内计数，重启后不能恢复。
6. Quota fail rate 分母被 confirm / release 稀释，且没有 `action` 标签或 reserve-only 指标可修正。
7. 指标标签泄露敏感信息或高基数字段。
8. `go test ./...` 或 `go build ./...` 失败且无明确非本任务原因。

## Decision

~~~text
PASS
~~~

需求：2026-05-27-engine-metrics-alerts
