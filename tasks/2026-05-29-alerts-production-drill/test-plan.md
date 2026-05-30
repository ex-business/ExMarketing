# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 的 Decision 为 `PASS` 且本文件 Decision 为 `PASS` 后才能执行测试。

本文件只定义后续测试用例和命令清单。Test Agent 执行时必须使用本地或测试环境，禁止在生产环境做破坏性故障注入。所有演练数据必须使用隔离 `drillId`、tenant、channel、activity/source/fact 标识，并在结束后清理或记录不可清理原因。

## Test Strategy

测试目标是验证告警演练资产能覆盖 `task.md` 和 `analysis.md` 中的 6 类核心故障，并额外覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke。

判定口径：

- 每个核心故障场景都必须有触发、观测、通知、恢复、清理和证据记录。
- 告警验证必须覆盖 Prometheus firing、Alertmanager route/receiver、通知 payload 字段和 resolved 恢复通知。
- DB 慢查询只验证 latency 指标和 runbook；只有核心写失败才能按 `ExMarketingDBWriteFailure` P0 验收。
- Quota 普通 reserve fail rate 只作为面板/辅助验证；P0 只按 `quota_aggregate_conflict|quota_reservation_missing` 验收。
- 如果目标告警被 DB P0 等高优先级告警 inhibit，需记录 inhibit 证据，不按未通知判失败。
- 任何残留 DLQ、异常冻结 quota、未恢复 worker/DB/provider mock、未关闭维护 silence 或测试数据污染，整体判 FAIL。

## Test Cases

### TC01 Static Alert And Routing Validation

目的：先验证告警规则、规则样例、Alertmanager 配置和通知模板本身可加载。

步骤：

1. 执行 Prometheus rule 静态校验。
2. 执行 Prometheus alert rule 单测。
3. 执行 Alertmanager 配置校验。
4. 检查 `monitoring/alertmanager-exmarketing.tmpl` 渲染 payload 字段包含 `service`、`env`、`tenant`、`channel`、`severity`、`alertname`、`owner`、`summary`、`description`、`dashboard`、`runbook`、`first_seen`、`current_value`、`threshold`。
5. 检查 alert labels 不包含 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始 provider 错误文本。

预期：静态校验和规则单测通过；模板字段完整；无高基数或敏感 label。

### TC02 Normal Path Baseline

目的：建立正常路径基线，证明没有演练故障时不会误触发 P0/P1。

步骤：

1. 启动本地或测试依赖和 ExMarketing 服务。
2. 创建隔离 `drillId`、tenant、channel、activity 和 mock reward provider。
3. 发送一批合法 source event，使其完成 activity 命中、quota reserve/confirm、outbox 写入、worker claim、reward send、callback accepted、audit/reconciliation 记录。
4. 查询 `/metrics` 和 Prometheus，记录 outbox backlog、oldest eligible age、worker claim success、callback accepted、quota fail、DB operation failed、DLQ count。
5. 查询 Alertmanager，确认目标 P0/P1 告警没有 firing。

预期：业务链路成功；`ExMarketingWorkerAllStopped`、`ExMarketingOutboxBacklogHigh`、`ExMarketingDLQGrowing`、`ExMarketingCallbackFailRateHigh`、`ExMarketingDBWriteFailure`、`ExMarketingQuotaOverDeductRisk` 均不 firing。

### TC03 Worker Stopped Or Not Consuming

目的：验证 worker 停止或不消费时触发 `ExMarketingWorkerAllStopped` P0，并能恢复。

步骤：

1. 准备 eligible outbox backlog，确保最老 eligible age 可超过 120s。
2. 停止 worker 或将 worker 配置为不 claim 当前隔离 tenant/channel。
3. 等待 Prometheus `for` 窗口满足，查询 worker claim success rate、eligible backlog、oldest eligible age。
4. 验证 Prometheus firing，Alertmanager 路由到 P0 receiver，并记录 firing 通知 payload。
5. 恢复 worker，等待 claim success rate 大于 0、oldest eligible age 下降、backlog 低于阈值。
6. 验证 Alertmanager resolved 通知。
7. 清理当前 `drillId` 产生的 outbox、activity 或 mock 数据。

预期：P0 firing 和 resolved 均可观测；恢复后 backlog draining；无新增 DLQ 污染。

### TC04 Outbox Backlog High

目的：验证 eligible outbox backlog 堆积触发 `ExMarketingOutboxBacklogHigh` P1。

步骤：

1. 使用隔离 tenant/channel 产生超过 100 条 `PENDING|FAILED` 且 eligible 的 outbox。
2. 保持 oldest eligible age 大于 60s，并持续满足 5 分钟窗口。
3. 查询 `exmarketing_outbox_backlog`、`exmarketing_outbox_oldest_eligible_age_seconds` 和 reward command p95。
4. 验证 P1 firing、receiver、payload 字段。
5. 恢复 worker/provider path，使 backlog 和 oldest age 回落。
6. 验证 resolved，且 DLQ 没有持续增长。

预期：只针对隔离 tenant/channel firing；恢复后 P1 resolved；reward command p95 回到验收口径。

### TC05 DLQ Growing

目的：验证 DLQ 增长触发 `ExMarketingDLQGrowing` P1，并验证 replay/close 需要业务确认。

步骤：

1. 使用 mock provider 返回可控失败，使隔离 outbox exhausted retry 后进入 DLQ。
2. 在 10 分钟窗口内制造至少 5 条新增 DLQ。
3. 查询 `increase(exmarketing_outbox_dlq_total[10m])`、`exmarketing_outbox_dlq_count`、DLQ admin 列表、audit。
4. 验证 P1 firing 和通知路由。
5. 修复 mock provider。
6. 在记录 business operations 确认后，仅 replay 或 close 当前 `drillId` 的 DLQ。
7. 验证无新增 DLQ、DLQ count 下降或稳定、audit 有 `OUTBOX_DLQ_REPLAY` 或 `OUTBOX_DLQ_CLOSE`。
8. 验证 resolved。

预期：DLQ 场景具备触发、业务确认、恢复和清理证据；未批量处理非演练 DLQ。

### TC06 Callback Failure Rate High

目的：验证 callback 失败率升高触发 `ExMarketingCallbackFailRateHigh` P1。

步骤：

1. 通过 mock provider 或 admin/test helper 对隔离 grant command 发送 callback。
2. 在 5 分钟内制造至少 5 次 `rejected|failed|conflict` callback，且失败率大于 5%。
3. 同时发送部分 accepted callback，验证低流量单次失败不会误判为 PASS。
4. 查询 callback failure increase、accepted increase、reconciliation、audit。
5. 验证 P1 firing、route 和 payload。
6. 修复 callback payload/signature/provider 状态，继续发送 accepted callback。
7. 验证 failure rate 低于 5%，reconciliation 终态正确，quota reservation 已 confirm 或 release。
8. 验证 resolved。

预期：失败数和失败率双条件都满足才触发；恢复后 accepted callback 增长且一致性干净。

### TC07 DB Slow Query And DB Write Failure

目的：区分 DB 慢查询观测和核心写失败 P0。

步骤：

1. PostgreSQL 健康时执行 DB latency 查询，记录基线 p95。
2. 使用测试环境允许的方式制造慢查询或锁等待，只验证 `exmarketing_db_operation_latency_seconds`、Grafana 面板和 DB Slow Or Error runbook，不要求 P0。
3. 恢复慢查询/锁等待，记录 p95 回落。
4. 在隔离环境制造核心写路径失败，例如 `grant_order_write`、`quota_reserve_write`、`outbox_write` 或 `callback_write` 失败。
5. 查询 `increase(exmarketing_db_operation_total{result="failed"}[5m])`。
6. 验证 `ExMarketingDBWriteFailure` P0 firing、DBA/infra receiver、通知 payload。
7. 恢复 DB 写能力，验证 5 分钟无新增 failed core write、latency 回到基线、下游链路恢复。
8. 验证 resolved，并记录任何下游告警 inhibit 证据。

预期：慢查询不冒充 DB P0；核心写失败触发 P0；恢复后 DB 和业务链路均正常。

### TC08 Quota Consistency Risk And Insufficient Quota

目的：覆盖 quota P0、一致性恢复，以及额度不足的非 P0 正常业务拒绝路径。

步骤：

1. 使用隔离活动配置有限 quota。
2. 正常消耗 quota，验证 reserve/confirm 成功、aggregate used/frozen 与 reservation 匹配。
3. 发送超过 quota 的合法请求，触发额度不足或 reserve fail 的业务拒绝。
4. 验证额度不足不会产生 `quota_aggregate_conflict|quota_reservation_missing`，不会触发 `ExMarketingQuotaOverDeductRisk` P0；普通 reserve fail rate 只记录为面板/辅助证据。
5. 使用安全注入方式制造 `quota_aggregate_conflict` 或 `quota_reservation_missing` 指标至少 1 次。
6. 验证 `ExMarketingQuotaOverDeductRisk` P0 firing、receiver、payload。
7. 按 runbook 做 reconciliation；如需 timeout-release，先记录 business operations 确认。
8. 验证无新增 consistency failure、aggregate used/frozen 与 reservations 匹配、audit 有修复动作、P0 resolved。

预期：额度不足是可控业务路径，不误报 P0；真正一致性风险触发并可恢复。

### TC09 Idempotent Duplicate Inputs

目的：验证重复 source event、重复 callback、重复 replay 或相同 idempotency key 不会重复发奖、重复扣 quota 或制造错误告警。

步骤：

1. 对相同隔离业务输入重复发送 source event 或相同 idempotency key 请求。
2. 对已终态 command 重复发送相同 callback。
3. 对同一 DLQ/replay 操作做一次成功后重复提交。
4. 查询 grant order、outbox、callback、quota reservation、audit 和 reconciliation。
5. 查询 P0/P1 告警状态。

预期：重复输入被幂等处理；不会重复扣减 quota，不会重复发奖；如产生 callback conflict，必须符合 TC06 的失败数和失败率才可触发 P1；不得触发 quota P0。

### TC10 Concurrent Drill Load

目的：验证并发下 worker、quota、callback 和告警指标稳定，且不会跨 tenant/channel 污染。

步骤：

1. 使用同一 `drillId` 的多个并发 worker/source/callback 客户端，对隔离 tenant/channel 执行事件进入、quota reserve、outbox claim、callback。
2. 并发制造部分可控失败，但不要超过非目标场景阈值，除非当前用例明确验证告警。
3. 查询 worker inflight、claim success/fail、outbox backlog、callback failure rate、quota fail、DB latency。
4. 查询业务数据，确认无重复 grant、无负数 quota、无跨 tenant/channel 数据。
5. 查询 Alertmanager，确认非目标 P0/P1 不误触发。

预期：并发场景保持数据一致；非目标告警不误报；如果达到阈值，必须能解释对应目标告警。

### TC11 PostgreSQL Smoke

目的：验证 PostgreSQL 基础可用性、迁移兼容和关键读写路径。

步骤：

1. 启动测试 PostgreSQL。
2. 执行迁移或确认当前 schema 已就绪。
3. 运行 PostgreSQL integration smoke，覆盖 event/grant/outbox/callback/quota/audit/reconciliation 的最小写入和查询。
4. 执行 `pg_isready`，并查询关键表中当前 `drillId` 相关记录。
5. 清理 `drillId` 数据或记录隔离数据保留策略。

预期：PostgreSQL ready；关键读写通过；没有连接失败、迁移 drift 或残留脏数据。

### TC12 End-To-End Drill Report Acceptance

目的：验证演练报告完整可审计。

步骤：

1. 对 TC02-TC11 汇总一份演练报告。
2. 每个场景填写 runId、环境、参与人、时间线、触发方式、alertname、severity、receiver、Prometheus 查询、Alertmanager payload、恢复动作、resolved 证据、清理证据和结论。
3. 失败场景必须填写失败分类、复现日志、owner、due date 和复测条件。
4. 检查报告没有敏感字段和高基数 label 泄漏。

预期：报告足以复盘和复测；所有 PASS 场景证据闭环；任何 FAIL 都有明确修复责任和复测条件。

## Required Commands

以下命令是后续 Test Agent 应执行的命令清单，本阶段不得执行。

~~~bash
# Static monitoring validation
promtool check rules monitoring/alerts-exmarketing.rules.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
amtool check-config monitoring/alertmanager-exmarketing.yml

# Unit and integration test baseline
go test ./...
go test ./internal/postgres -run Test -count=1
go test ./internal/worker -run Test -count=1
go test ./internal/httpapi -run Test -count=1
go test ./internal/marketing -run Test -count=1
go test ./internal/plugin -run Test -count=1

# Build baseline
go build ./...

# Local/test environment bring-up and health checks
docker compose up -d postgres prometheus alertmanager
docker compose ps
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
curl -fsS http://localhost:8080/metrics
curl -fsS http://localhost:9090/-/ready
curl -fsS http://localhost:9093/-/ready

# Prometheus alert and metric probes used during drill scenarios
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=ALERTS{service="exmarketing",alertstate="firing"}'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,send_status) (exmarketing_outbox_backlog{send_status=~"PENDING|FAILED|SENDING"})'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=max by (tenant_id,target_channel) (exmarketing_outbox_oldest_eligible_age_seconds)'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel) (rate(exmarketing_worker_claim_total{result="success"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,target_channel,reason) (increase(exmarketing_outbox_dlq_total[10m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,grant_channel,result,reason) (increase(exmarketing_reward_callback_total{result=~"rejected|failed|conflict"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (operation,table,reason) (increase(exmarketing_db_operation_total{result="failed"}[5m]))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=histogram_quantile(0.95, sum by (operation,table,le) (rate(exmarketing_db_operation_latency_seconds_bucket[5m])))'
curl -G http://localhost:9090/api/v1/query --data-urlencode 'query=sum by (tenant_id,quota_type,scope,reason) (increase(exmarketing_quota_fail_total{reason=~"quota_aggregate_conflict|quota_reservation_missing"}[5m]))'

# Alertmanager notification and route inspection
curl -fsS http://localhost:9093/api/v2/alerts
curl -fsS http://localhost:9093/api/v2/silences
curl -fsS http://localhost:9093/api/v2/receivers

# Admin/API evidence queries; replace placeholders with isolated drill values
curl "http://localhost:8080/admin/v1/outbox/dlq?tenantId=$TENANT&includeClosed=false"
curl "http://localhost:8080/admin/v1/reward/reconciliation?tenantId=$TENANT&activityId=$ACTIVITY_ID&limit=50"
curl "http://localhost:8080/admin/v1/audit?tenantId=$TENANT&limit=100"
curl -X POST "http://localhost:8080/admin/v1/outbox/$OUTBOX_ID/replay" -H 'Content-Type: application/json' -d '{"operator":"test-agent","reason":"drill root cause fixed"}'
curl -X POST "http://localhost:8080/admin/v1/outbox/$OUTBOX_ID/close" -H 'Content-Type: application/json' -d '{"operator":"test-agent","reason":"business confirmed no replay"}'
~~~

## Data / Smoke

Test Agent 必须准备并记录：

- `drillId`: 唯一值，例如 `alerts-drill-YYYYMMDD-HHMMSS`。
- 隔离 tenant/channel/activity/source/fact/provider mock，不复用长期共享测试活动。
- Mock reward provider 支持 success、retryable failure、permanent failure、callback accepted、callback rejected、callback conflict。
- Quota 数据包含正常额度、额度不足和一致性风险注入三种状态。
- PostgreSQL 使用本地或测试 DSN；禁止生产库。
- Notification receiver 使用测试 webhook/mock receiver，必须能保存 firing 和 resolved payload。
- 清理检查至少覆盖 outbox、DLQ、callback、quota reservation/aggregate、audit、reconciliation、worker 配置、DB 故障注入开关、Alertmanager silence。

PostgreSQL smoke 最低验收：

- `pg_isready` 成功。
- 迁移或 schema readiness 成功。
- 当前 `drillId` 的 event/grant/outbox/callback/quota/audit/reconciliation 关键读写可查询。
- DB 核心写失败恢复后，`increase(exmarketing_db_operation_total{result="failed"}[5m])` 在恢复窗口内不再增长。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-alerts-production-drill
