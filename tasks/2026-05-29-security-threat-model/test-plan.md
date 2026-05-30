# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

Review 已 PASS，本测试计划允许进入后续 Test Agent 阶段。

本需求交付物是安全威胁模型，不是代码修复。测试重点分两层：

1. 文档验收：检查威胁模型文档是否覆盖 task.md 和 analysis.md 要求的资产、入口、信任边界、STRIDE 风险矩阵、P0/P1 后续任务和 Review P2 补强点。
2. 回归验证：后续 Test Agent 在不扩大业务范围的前提下，运行现有 Go 测试、构建和 PostgreSQL smoke，确认威胁模型相关安全控制描述没有与当前系统基本行为冲突。

后续 Test Agent 必须记录每条测试的命令、结果、失败摘要和证据路径。若任一必测项失败，`test.md` 的 Result 应为 FAIL，并明确失败属于文档缺口、现有测试失败、环境缺失还是 PostgreSQL smoke 失败。

## Test Cases

### TP-01 Review Gate

- 前置：读取 `tasks/2026-05-29-security-threat-model/review.md`。
- 步骤：确认 Decision 为 `PASS`，且无 P0/P1 Finding。
- 期望：允许执行后续测试；若 Decision 不是 `PASS` 或存在 P0/P1，停止测试并在 `test.md` 写 BLOCK/FAIL。

### TP-02 威胁模型覆盖完整性

- 前置：Dev Agent 已产出正式威胁模型文档。
- 步骤：逐项检查文档是否覆盖 OpenAPI、Admin、Webhook Receiver、Reward Callback、Fact Provider、MQ、Outbox、DLQ、导出文件、备份文件。
- 期望：每个入口至少有独立威胁分析；每个入口都标出身份凭据、tenant/source 约束、主要攻击路径、现有控制和缺口。

### TP-03 高危资产覆盖完整性

- 步骤：检查资产清单和风险矩阵是否包含活动配置、quota、quota reservation、grant_order、outbox、DLQ、audit、fact provider secret/authRef、MQ credential/authRef/TLS 引用、OpenAPI token、Webhook secret、callback secret、导出文件、备份文件、metrics/logs/runbook 证据。
- 期望：每个高危资产都有保护措施或明确缺口；未见现有控制时不能写成“已控制”。

### TP-04 数据流和信任边界

- 步骤：检查数据流/信任边界图是否覆盖公网入口、管理入口、内网服务、DB、MQ、外部 Fact Provider、外部发奖系统、文件导出/备份、监控告警。
- 期望：每条跨边界数据流有 source、sink、协议、认证/签名、tenant 约束和失败后果。

### TP-05 STRIDE 风险矩阵字段

- 步骤：抽查并统计风险矩阵字段。
- 期望：每条风险包含入口/资产、STRIDE 类别、攻击路径、影响、现有控制、缺口、严重度、可能性、风险等级、建议补救方向、验收证据。

### TP-06 P0/P1 后续任务

- 步骤：检查所有 P0/P1 风险是否拆出后续安全任务。
- 期望：每个任务包含问题、入口/资产、建议控制、优先级、验收标准；发布前必须处理或明确豁免的项目不能只进入泛化 backlog。

### TP-07 Review P2 补强项

- 步骤：检查威胁模型是否明确 nonce/timestamp 口径。如果当前系统不采用 nonce，必须说明 replay window、幂等键、callback 终态冲突拒绝作为替代控制。
- 步骤：检查 SSRF 子场景是否包含 HTTP redirect、DNS rebinding、代理环境变量绕过，以及 private/local IP 拦截和 endpoint allowlist。
- 期望：两类 Review P2 均被补入威胁枚举或风险矩阵；若缺失则 FAIL。

### TP-08 正常路径

- 目标：验证当前系统的基本正常路径仍与威胁模型描述一致。
- 步骤：运行全量 Go 测试和 PostgreSQL E2E smoke。
- 期望：正常事件进入、规则匹配、quota reserve/confirm、grant_order、outbox、callback 终态、审计/指标的基础链路通过。

### TP-09 额度不足

- 目标：覆盖 quota 不足或 quota reserve 被拒绝的安全边界。
- 步骤：运行现有 quota/engine/httpapi 相关测试；如现有 PostgreSQL smoke 不覆盖额度不足，Test Agent 应补充手工 API 或最小测试数据验证，但新增测试代码需由后续 Dev/Test 流程另行批准。
- 期望：额度不足时不创建成功发奖终态，不错误确认 quota，不产生可投递成功的 reward command；指标/审计只记录低基数字段和脱敏信息。

### TP-10 幂等重复

- 目标：覆盖重复 OpenAPI/Webhook 事件、重复 Reward Callback、重复 DLQ replay 请求。
- 步骤：运行现有 handler、repository、outbox worker、callback 相关测试；PostgreSQL smoke 中重复提交同一业务 key 或 idempotency key，观察结果。
- 期望：重复事件不会重复扣减 quota 或重复发奖；重复 callback 对同一终态应幂等接受或安全忽略；相反终态冲突必须拒绝；重复 DLQ replay 不得绕过状态机。

### TP-11 并发

- 目标：覆盖并发事件、并发 quota reserve、并发 worker claim、并发 callback finalize。
- 步骤：运行全量测试时开启 race 检测；对 PostgreSQL smoke 使用并发脚本或现有集成命令重复触发同一 tenant/activity/quota 范围。
- 期望：无 data race；PostgreSQL 下不出现 quota over-deduct、reservation missing、重复 grant_order、重复 outbox claim 或 callback 终态错乱。

### TP-12 PostgreSQL Smoke

- 前置：本地 Docker PostgreSQL 可用。
- 步骤：启动 postgres，应用 migration，运行 `cmd/engine-e2e-postgres`。
- 期望：命令成功；活动、quota、event inbox、participation、grant_order、outbox、callback、对账状态均写入 PostgreSQL；唯一 runId 可重复执行。

### TP-13 敏感信息和低基数指标

- 步骤：启动服务后抓取 `/metrics`，检查是否包含 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey。
- 期望：Prometheus labels 和公开输出不包含上述敏感或高基数字段；定位信息只能进入审计、对账查询、DB 查询或已脱敏日志。

### TP-14 Admin / DLQ 高危操作审计

- 步骤：检查现有测试或手工 API 验证 DLQ replay/close、manual grant、manual export、quota timeout release 等高危操作。
- 期望：operator 和 reason 必填；仅允许当前状态为 DLQ 的记录 replay；DLQ_CLOSED 不允许 replay；操作写入 audit；业务确认要求在威胁模型和 runbook 中明确。

### TP-15 MQ / targetRef / Fact Provider 边界

- 步骤：运行 MQ target、worker MQ runtime、Fact client 相关测试。
- 期望：targetRef 不允许账号、密码、token、query、fragment；MQ destinationAllowlist 失败进入 retry/DLQ；Fact Provider 仅使用 registry/authRef，不接受 inline provider 配置；endpoint allowlist 和 private/local IP 拦截生效。

## Required Commands

以下命令由后续 Test Agent 执行；Test Plan Agent 不执行。

~~~bash
go test ./...
go build ./...
~~~

建议补充命令：

~~~bash
go test -race ./...
docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/db-migration-release --mode apply
go run ./cmd/engine-e2e-postgres
go run ./cmd/exmarketing
curl -fsS http://localhost:8080/metrics | tee /tmp/exmarketing.metrics
rg 'exmarketing_(event_intake|rule_execution|reward_execution|quota_attempt|quota_amount|outbox_backlog|reward_callback|worker_claim|db_operation)' /tmp/exmarketing.metrics
rg 'Authorization|Cookie|token|secret|password|payload|targetRef|eventId|bizId|subjectId|userId|activityId|billNo|commandId|idempotencyKey' /tmp/exmarketing.metrics && exit 1 || true
promtool check config monitoring/prometheus-exmarketing.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
~~~

若本机缺少 `promtool` 或 `alertmanager`，Test Agent 应记录为环境缺失，不应把该项混同为业务测试失败。

## Data / Smoke

PostgreSQL smoke 数据要求：

- 使用 `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'`。
- 每次 smoke 使用唯一 runId 或唯一 tenant/activity/bizId，避免与历史数据互相污染。
- smoke 必须覆盖至少一个正常发奖链路：event intake -> rule -> quota reserve -> grant_order -> outbox -> sender/callback stub -> callback finalize -> reconciliation。
- smoke 必须记录 quota 初始值、reserve/confirm/release 结果、grant_order 状态、outbox 状态、callback 状态和 reconciliation 查询结果。
- 若现有 `cmd/engine-e2e-postgres` 未覆盖额度不足、重复幂等或并发，Test Agent 应在 `test.md` 标出覆盖缺口，并建议后续新增对应集成测试；不得在未经授权的 Test Plan 阶段修改测试代码。

手工/半自动场景数据要求：

- 额度不足：构造 quota 小于奖励金额或剩余额度为 0 的活动，提交有效事件，期望拒绝发奖且不确认 quota。
- 幂等重复：同 tenant/source/eventType/bizType/bizId 重复提交；同 commandId/idempotencyKey 重复 callback；同 outboxId 重复 replay。
- 并发：至少 20 个并发请求命中同一 tenant/activity/quota scope；期望最终 used/frozen 与成功订单数一致。
- PostgreSQL：所有上述场景优先在 PostgreSQL 仓储下验证，内存仓储只作为快速单测补充。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-security-threat-model
