# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 的 Decision 为 `PASS`，且本文件 Decision 为 `PASS` 时，才执行本计划。

执行时不得修改业务代码、测试代码、文档代码或配置文件；只允许读取文件、启动必要的本地依赖、执行测试/构建/静态校验命令，并在 `test.md` 记录结果。

本计划面向需求 `2026-05-29-alerts-runbook-slo-runbook-slo`。本轮 Test Plan Agent 已确认 `review.md` Decision 为 `PASS`。

## Test Strategy

本需求主要交付运维文档、SLO/SLA 定义、演练/复盘模板和告警到 runbook 映射表。测试重点不是新增业务逻辑，而是验证文档是否能被真实值班人员按步骤执行，并且与现有告警、指标、管理 API、审计和 PostgreSQL 数据模型一致。

测试分四层：

1. 静态文档验收：确认 `docs/operations.md` 可发现 runbook、SLO/SLA、演练模板、复盘模板和告警映射表；确认所有 P1/P2 告警都有可执行 runbook，P0/P3 核心告警也被覆盖。
2. 告警规则一致性：确认 `monitoring/alerts-exmarketing.rules.yml` 中的 alertname、severity、owner、dashboard、runbook anchor 与文档映射一致，Prometheus/Alertmanager 配置可被工具校验。
3. 业务路径 smoke：按 runbook 执行正常路径、额度不足、幂等重复、并发、DLQ/replay/close、callback/reconciliation、Fact Query 失败等场景，验证文档步骤能指导排查、止血、恢复和验证。
4. PostgreSQL smoke：用只读 SQL 或测试环境 SQL 验证 runbook 引用的数据表、审计查询、DLQ/outbox/quota/reconciliation 检查路径可执行，且不泄漏敏感字段。

## Static Test Cases

### TC-DOC-001 Operations 入口可发现

- 前置条件：Dev 已完成文档交付。
- 步骤：
  - 打开 `docs/operations.md`。
  - 确认能找到 ExMarketing 告警 runbook 入口。
  - 确认能找到 SLI/SLO/SLA、演练记录模板、复盘模板、告警到 runbook 映射表。
- 通过标准：不需要知道具体文件路径也能从 `docs/operations.md` 进入全部交付内容。

### TC-DOC-002 P1/P2 告警 runbook 覆盖

- 覆盖告警：
  - `ExMarketingDLQGrowing`
  - `ExMarketingOutboxBacklogHigh`
  - `ExMarketingCallbackFailRateHigh`
  - `ExMarketingEventIntakeErrorRateHigh`
  - `ExMarketingClaimLatencyHigh`
  - `ExMarketingRewardRetrySpike`
  - `ExMarketingFactQueryFailRateHigh`
- 步骤：
  - 从 `monitoring/alerts-exmarketing.rules.yml` 提取 P1/P2 alertname。
  - 在 runbook 文档中逐个查找同名小节或明确映射 anchor。
- 通过标准：每个 P1/P2 告警都有症状、影响判断、排查入口、止血、恢复、验证、升级规则。

### TC-DOC-003 任务范围核心场景覆盖

- 覆盖场景：
  - DLQ 堆积
  - outbox backlog
  - callback 失败
  - quota 异常/额度不足/一致性风险
  - worker 停止
  - DB 慢/错误
  - Fact Query 失败
- 通过标准：每个场景都有明确 dashboard/API/查询入口、业务确认边界、恢复验证指标。

### TC-DOC-004 SLO 指标可计算

- 步骤：
  - 检查 SLI/SLO/SLA 章节。
  - 确认事件接入成功率、发奖指令投递延迟、callback 成功率、DLQ 清理时效、数据一致性、worker claim 延迟、Fact Query 成功率均有定义。
  - 确认每个 SLI 映射到现有 `/metrics` 指标或现有管理 API/审计数据。
- 通过标准：不得引用不存在的指标；counter 类指标必须使用 `rate` 或 `increase` 口径；低流量失败率必须说明最小失败数边界。

### TC-DOC-005 敏感信息与高基数字段约束

- 步骤：
  - 检查 runbook 示例、PromQL、curl、SQL、复盘模板。
  - 确认没有要求把 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始错误文本放入指标标签或公开复盘材料。
- 通过标准：示例使用脱敏值或占位符；复盘只要求脱敏统计。

## Business Smoke Test Cases

### TC-SMOKE-001 正常路径：事件接入到发奖 callback 成功

- 目标：验证 runbook/SLO 对正常链路的指标和审计口径可用。
- 步骤：
  - 在测试环境创建或选择一个有效活动、tenant、channel。
  - 发送一条合法事件，确保事件接入成功。
  - 等待 outbox 被 worker claim 并发送发奖指令。
  - 模拟或接收 reward provider 成功 callback。
  - 查询 `/metrics`、outbox、grant order、audit。
- 通过标准：
  - `exmarketing_event_intake_total{result="success"}` 增加。
  - reward command latency 可被 SLO PromQL 计算。
  - callback success 计数增加。
  - runbook 的恢复验证步骤能定位到同一 tenant/channel 的指标和审计证据。

### TC-SMOKE-002 额度不足：quota exceeded / reservation blocked

- 目标：覆盖用户要求的额度不足场景，并验证 quota runbook 不允许绕过一致性风险。
- 步骤：
  - 在测试环境设置活动 quota，使下一次参与超过可用额度。
  - 发送触发额度不足的事件。
  - 查询业务响应、quota 指标、审计和 runbook 指引。
- 通过标准：
  - 事件被拒绝或进入预期失败状态，不产生越额发奖。
  - 相关失败能在 `exmarketing_quota_fail_total` 或文档指定的管理 API/审计中定位。
  - runbook 明确 quota aggregate conflict / reservation missing 必须先冻结风险操作并完成 reconciliation，不能直接 replay 或手工改 used/frozen 绕过。

### TC-SMOKE-003 幂等重复：重复事件或重复 callback

- 目标：验证幂等重复不会造成重复发奖、重复 quota 扣减或错误告警。
- 步骤：
  - 使用相同业务幂等键或等价业务标识发送同一事件两次。
  - 对同一 grant order 发送重复 SUCCESS callback。
  - 查询 event intake、grant order、quota、callback 指标和 audit。
- 通过标准：
  - 第二次事件被识别为 duplicate 或等价幂等结果。
  - 不重复创建有效 grant order，不重复扣减 quota。
  - 重复 callback 不造成终态冲突；若发生 SUCCESS/FAILED 终态冲突，runbook 指向 reconciliation 和复盘。
  - SLO 事件成功率口径明确排除 duplicate。

### TC-SMOKE-004 并发：同一 quota scope 下并发参与

- 目标：验证 runbook 覆盖并发引发的 quota reservation、aggregate conflict、outbox backlog 和 callback 冲突排查。
- 步骤：
  - 对同一 tenant/activity/quota scope 并发发送多条合法事件，数量接近或超过 quota。
  - 观察 quota reservation、participation、outbox、grant order。
  - 检查是否触发 `ExMarketingQuotaOverDeductRisk` 或相关审计。
- 通过标准：
  - 成功数量不超过 quota。
  - 失败或拒绝路径可被 runbook 指引定位。
  - 若出现 aggregate conflict 或 reservation missing，告警映射到 `ExMarketingQuotaOverDeductRisk`，且业务确认要求清晰。

### TC-SMOKE-005 DLQ replay / close 业务确认

- 目标：验证 DLQ runbook 能指导处理可 replay 与需 close 的消息。
- 步骤：
  - 在测试环境制造或选取一条 `DLQ` outbox。
  - 按 runbook 查询 DLQ 原因、tenant/channel、业务影响。
  - 对可重放消息执行 replay；对需关闭消息模拟业务确认后执行 close。
- 通过标准：
  - replay 仅允许 `DLQ` 状态回到 `PENDING`，payload 不变。
  - `DLQ_CLOSED` 不允许 replay。
  - close 只关闭 outbox，不修改 participation、grant_order 或 quota reservation。
  - replay/close 均有 audit 记录和业务确认记录路径。

### TC-SMOKE-006 Fact Query 失败分类

- 目标：验证 Fact Query runbook 能区分 provider timeout、provider unavailable、mapping failed、field missing、安全拦截。
- 步骤：
  - 分别模拟或查找上述失败类型。
  - 按 runbook 查询 `exmarketing_fact_query_total` 和相关 provider/fact_code 维度。
- 通过标准：
  - runbook 指向不同根因的排查入口。
  - provider-wide 故障要求协调 provider owner。
  - 安全失败不得通过放宽 SSRF、allowlist、redirect、proxy 等限制止血。

## PostgreSQL Smoke

### TC-PG-001 只读连接与基础表可见

- 目标：验证 Test Agent 能用测试环境 PostgreSQL 执行 runbook 引用的只读查询。
- 步骤：
  - 使用测试环境只读账号连接 PostgreSQL。
  - 查询 outbox、DLQ、participation、grant_order、quota reservation/aggregate、audit 相关表是否存在。
- 通过标准：表存在，查询能按 tenant/channel/activity 或脱敏业务键过滤。

### TC-PG-002 正常路径数据闭环

- 步骤：
  - 对 TC-SMOKE-001 的测试样本查询 participation、grant_order、outbox、callback/audit。
- 通过标准：状态流转与 runbook 中的正常恢复验证一致；查询结果不要求暴露完整敏感字段。

### TC-PG-003 额度不足与并发一致性

- 步骤：
  - 对 TC-SMOKE-002 和 TC-SMOKE-004 的测试样本查询 quota reservation、quota aggregate、audit。
- 通过标准：没有 over-deduct；失败样本有可追踪原因；如存在 conflict，能关联到 runbook 的 reconciliation 步骤。

### TC-PG-004 DLQ 状态约束

- 步骤：
  - 对 TC-SMOKE-005 的测试样本查询 outbox 状态、payload 摘要、audit。
- 通过标准：replay 后状态回到 `PENDING`；close 后为 `DLQ_CLOSED`；close 不改变 participation/grant_order/quota reservation。

## Required Commands

以下命令由后续 Test Agent 执行，本 Test Plan Agent 不执行。

### 文档和映射静态检查

~~~bash
rg -n "ExMarketing(DLQGrowing|OutboxBacklogHigh|CallbackFailRateHigh|EventIntakeErrorRateHigh|ClaimLatencyHigh|RewardRetrySpike|FactQueryFailRateHigh)" docs monitoring tasks/2026-05-29-alerts-runbook-slo-runbook-slo
rg -n "SLI|SLO|SLA|演练|复盘|业务确认|runbook|mapping|映射" docs/operations.md docs/runbooks
rg -n "token|Authorization|Cookie|secret|password|idempotencyKey|commandId|billNo|targetRef|payload" docs/operations.md docs/runbooks
~~~

### Prometheus / Alertmanager 配置检查

~~~bash
promtool check config monitoring/prometheus-exmarketing.yml
promtool test rules monitoring/alerts-exmarketing.test.yml
alertmanager --config.file=monitoring/alertmanager-exmarketing.yml --config.check
~~~

如果本机缺少 `promtool` 或 `alertmanager`，Test Agent 应先补齐工具链再执行上述命令。当前本地可用修复方式：

~~~bash
brew install prometheus
go install github.com/prometheus/alertmanager/cmd/alertmanager@v0.28.1
go install github.com/prometheus/alertmanager/cmd/amtool@v0.28.1
~~~

注意：部分官方 Alertmanager 版本没有 `--config.check` 参数，只有 `amtool check-config`。若本机 `alertmanager --config.check` 不存在，可用同等语义校验：

~~~bash
amtool check-config monitoring/alertmanager-exmarketing.yml
~~~

### Go 基础验证

~~~bash
go test ./...
go build ./...
~~~

### API smoke 命令模板

根据本地实际端口和鉴权方式替换 `BASE_URL`、`TOKEN`、`TENANT_ID`、`CHANNEL`、`ACTIVITY_ID`、`IDEMPOTENCY_KEY`。不要把真实 token 或完整业务标识写入 `test.md`。
执行 smoke 前必须确认 `BASE_URL` 指向当前仓库代码启动的 ExMarketing 服务；如果 `127.0.0.1:8080` 已被旧进程或其他服务占用，应改用空闲端口启动当前服务，并同步设置 `BASE_URL`。

~~~bash
export BASE_URL="http://127.0.0.1:8080"
export TOKEN="<redacted-test-token>"

curl -fsS "${BASE_URL}/healthz"
curl -fsS "${BASE_URL}/metrics" | head -n 20
curl -sS -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/metrics" | rg "exmarketing_(event_intake_total|reward_command_latency_seconds|reward_callback_total|outbox_dlq|quota_fail_total|worker_claim_latency_seconds|fact_query_total)"

curl -sS -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/admin/exmarketing/dlq?tenantId=<TENANT_ID>&channel=<CHANNEL>"
curl -sS -X POST -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" "${BASE_URL}/admin/exmarketing/dlq/<DLQ_ID>/replay" -d '{"reason":"test replay after root cause fixed"}'
curl -sS -X POST -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" "${BASE_URL}/admin/exmarketing/dlq/<DLQ_ID>/close" -d '{"reason":"business confirmed no replay"}'
curl -sS -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}/admin/exmarketing/audit?tenantId=<TENANT_ID>&resourceType=outbox&resourceId=<DLQ_ID>"
~~~

本机 8080 被占用时可用以下等价启动方式：

~~~bash
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
export BASE_URL="http://127.0.0.1:18080"
curl -fsS "${BASE_URL}/metrics" | rg "exmarketing_(event_intake_total|reward_command_latency_seconds|reward_callback_total|outbox_dlq|quota_fail_total|worker_claim_latency_seconds|fact_query_total)"
~~~

### PostgreSQL smoke 命令模板

使用测试环境只读 DSN。SQL 中只输出脱敏字段、状态、计数和时间，不输出完整 payload 或敏感业务标识。
如果 `EXMARKETING_PG_READONLY_DSN` 未设置，Test Agent 可启动仓库本地 PostgreSQL，并用本地测试库作为只读 smoke 数据源；该路径只用于验证 runbook 引用的表名和状态聚合查询可执行，不代表生产只读账号权限验证。

~~~bash
if test -z "${EXMARKETING_PG_READONLY_DSN:-}"; then
  docker compose up -d postgres
  export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
  go run ./cmd/db-migration-release --mode apply
  export EXMARKETING_PG_READONLY_DSN="$EX_MARKETING_DATABASE_URL"
fi

if command -v psql >/dev/null 2>&1; then
  psql "$EXMARKETING_PG_READONLY_DSN" -c "\dt"
  psql "$EXMARKETING_PG_READONLY_DSN" -c "select send_status, count(*) from marketing_outbox group by send_status order by send_status;"
  psql "$EXMARKETING_PG_READONLY_DSN" -c "select grant_status, count(*) from marketing_grant_order group by grant_status order by grant_status;"
  psql "$EXMARKETING_PG_READONLY_DSN" -c "select action, count(*) from marketing_audit where created_at > now() - interval '1 day' group by action order by action;"
  psql "$EXMARKETING_PG_READONLY_DSN" -c "select quota_type, scope, count(*) from marketing_quota_reservation group by quota_type, scope order by quota_type, scope limit 20;"
else
  docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "\dt"
  docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select send_status, count(*) from marketing_outbox group by send_status order by send_status;"
  docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select grant_status, count(*) from marketing_grant_order group by grant_status order by grant_status;"
  docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select action, count(*) from marketing_audit where created_at > now() - interval '1 day' group by action order by action;"
  docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "select quota_type, scope, count(*) from marketing_quota_reservation group by quota_type, scope order by quota_type, scope limit 20;"
fi
~~~

如果实际表名不同，Test Agent 应按 `docs/data-model.md` 和 runbook 文档中的最终表名调整查询，并在 `test.md` 记录差异。

## Decision

~~~text
PASS
~~~

理由：`review.md` 已 PASS；本计划覆盖静态文档验收、告警规则一致性、正常路径、额度不足、幂等重复、并发、DLQ 操作、Fact Query 分类和 PostgreSQL smoke，并列出后续 Test Agent 应执行的命令清单。

需求：2026-05-29-alerts-runbook-slo-runbook-slo
