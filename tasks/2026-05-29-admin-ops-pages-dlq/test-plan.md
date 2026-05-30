# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

Review Decision 为 PASS，本测试计划可进入后续 Test Agent 执行阶段。

测试目标是验证 Admin 运维页面和后端 Admin API 的真实联动，不允许使用前端假数据、直连数据库绕过服务 API 或只做前端过滤。测试分为四层：

1. 单元 / 集成回归：跑完整 Go 测试，覆盖新增 API 参数、状态转换、审计写入和幂等逻辑。
2. 本地构建验证：确认后端和前端相关代码可构建。
3. Admin API smoke：用真实服务 API 准备和验证审计、对账、DLQ、人工发奖、导出发奖、跨 tenant 和脱敏。
4. PostgreSQL smoke：只读检查核心表状态与审计记录落库，确认服务 API 结果与数据库事实一致。

测试数据应至少包含两个 tenant：`tenant_admin_ops_a` 与 `tenant_admin_ops_b`。所有正常路径、异常路径、幂等重复和并发用例都必须证明 tenant 隔离有效。

## Test Cases

### 1. 审计列表服务端过滤

- 准备同 tenant 下不同 `action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`traceId`、`requestId`、`createdAt` 的审计记录。
- 通过审计页或 `GET /admin/v1/audit` 分别按 `tenantId`、actor、action、resource、resourceId、activityId、eventId、commandId 查询。
- 使用 `startTime/endTime` 验证服务端按 `marketing_audit.created_at >= startTime` 且 `< endTime` 过滤，边界记录必须符合预期。
- 使用 `traceId/requestId` 验证服务端按 `payload_json.source.traceId/requestId` 过滤；两者同时传入时必须是 AND 语义。
- 验证历史无 source 摘要的记录不会被误匹配，页面显示明确空态或提示。
- 验证响应与页面详情不展示 Authorization、Cookie、token、secret、完整 broker/provider 凭据、完整 targetRef、完整敏感 payload。

### 2. 对账查询正常路径

- 触发一个正常活动事件，使 event inbox、participation、grant order、outbox、callback、quota reservation 和审计时间线均存在。
- 分别按 `activityId`、`activityCode`、`subjectType + subjectId`、`eventId`、`idempotencyKey`、`grantOrderId`、`commandId/billNo`、`outboxId` 查询。
- 验证对账页展示完整链路和真实状态，不构造不存在的节点。
- 验证普通 Outbox、发奖单、导出文件页面文案为“按条件定位/本次结果展示”口径，不宣称无 API 支撑的全量分页列表。

### 3. 额度不足路径

- 配置或选择一个 quota 已耗尽 / 不足的活动奖励场景。
- 触发参与事件后查询对账链路。
- 验证 participation 或规则结果能解释 quota 不足，grant order/outbox 不存在时页面明确说明原因。
- 验证没有创建自动 outbox，没有错误显示为成功发奖。
- 验证相关审计和对账信息可按 tenant 与业务 ID 查询。

### 4. 幂等重复路径

- 使用同一 `tenantId + idempotencyKey` 或同一业务事件重复提交两次。
- 验证第二次请求不重复创建 participation、grant order、outbox、quota reservation。
- 通过对账页按 `idempotencyKey` 查询，确认页面展示重复事件 / 已处理状态。
- 对同一个人工发奖单重复提交相同终态 decision，验证幂等成功或返回已处理，且不得重复移动 quota。
- 对同一个导出候选重复创建导出，验证未失败 / 未作废的导出明细会阻止重复纳入。

### 5. DLQ 运维正常路径

- 准备 `sendStatus=DLQ` 的 outbox。
- 访问 DLQ 页默认查询，验证只显示 `DLQ`；开启 `includeClosed=true` 后可同时显示 `DLQ_CLOSED`。
- 执行 replay，必须二次确认，`operator` 与 `reason` 必填。
- 验证 replay 后 outbox 回到 `PENDING`，payload 不变，并能查到 `OUTBOX_DLQ_REPLAY` 审计。
- 重新准备 `DLQ` outbox 执行 close，验证状态变为 `DLQ_CLOSED`，不修改 participation、grant order、quota reservation，并能查到 `OUTBOX_DLQ_CLOSE` 审计。
- 对 `DLQ_CLOSED` 执行 replay，验证后端拒绝，页面展示真实失败原因。

### 6. 人工发奖 decision 正常路径

- 准备 `MANUAL` grant order，分别按 `grantOrderId`、`billNo/commandId`、`idempotencyKey` 定位。
- 分别验证 `SUCCESS`、`FAILED`、`CLOSED` 三类 decision；每次必须填写 `operator` 与 `reason` 并通过确认弹窗。
- 验证操作后对账页可追踪最终状态、quota 影响摘要和审计时间线。
- 验证每次 decision 写入 `MANUAL_GRANT_DECISION` 审计，resource 为 `GRANT_ORDER`，payload 包含最小字段且脱敏。
- 对 `MANUAL_EXPORT` grant order 验证：只有已进入 READY 导出文件明细且处于 `SENDING` 的单据允许人工确认 `SUCCESS`。

### 7. 导出发奖正常路径

- 准备多个 `grantMode=MANUAL_EXPORT` 且 `PENDING` / `RETRY` 的 grant order。
- 创建导出，验证返回并展示真实 `exportId`、fileName、fileFormat、status、rowCount、checksum、filterSnapshot、createdAt、updatedAt 和明细状态摘要。
- 验证 `MANUAL` grant order 不会进入导出文件。
- 验证空候选不会创建成功文件，页面展示真实失败或跳过原因。
- 验证创建导出写入 `MANUAL_GRANT_EXPORT_CREATE` 审计；如果本实现暴露导出 READY/FAILED/CLOSED 状态变化，还要验证 `MANUAL_GRANT_EXPORT_STATUS` 审计。

### 8. 危险操作校验与错误态

- DLQ replay/close、人工 decision、导出创建缺少 `operator` 时不能提交。
- DLQ replay/close、人工 decision、导出创建缺少或只传空白 `reason` 时不能提交。
- 后端返回非 2xx、网络失败、参数校验失败时，页面必须展示真实错误码 / 错误消息，不得显示成功态。
- 查询无结果时显示明确空态；目标不存在时显示未找到，不伪造空链路。

### 9. 跨 tenant 隔离

- 在 `tenant_admin_ops_a` 创建 event、grant order、outbox、export 和 audit。
- 使用 `tenant_admin_ops_b` 携带相同 `commandId/eventId/grantOrderId/outboxId/exportId` 查询，必须返回未找到或越权错误，不得返回 A tenant 数据。
- 使用 B tenant 对 A tenant 的 DLQ、人工发奖、导出对象执行危险操作，必须失败且不得修改 A tenant 数据。
- 审计列表按 B tenant 查询时不得出现 A tenant 操作记录。

### 10. 并发冲突

- 并发对同一个 `DLQ` outbox 发起 replay 与 close，最终只能有一个操作成功；失败方必须返回状态冲突或已处理原因。
- 并发对同一个人工 grant order 发起 `SUCCESS` 与 `FAILED/CLOSED`，最终只能有一个终态生效；quota 不得重复移动。
- 并发创建同一筛选条件的 `MANUAL_EXPORT` 导出，未失败 / 未作废的导出明细不得重复纳入同一 grant order。
- 并发完成后通过对账页、审计页和 PostgreSQL smoke 核对最终状态与审计数量。

### 11. PostgreSQL smoke

- 只读检查 `marketing_audit` 中 DLQ replay/close、manual decision、manual export create/status 的 action、resource、resourceId、tenantId、operator、reason、source trace/request 摘要。
- 只读检查 outbox 状态：replay 后为 `PENDING`，close 后为 `DLQ_CLOSED`，payload 未变化。
- 只读检查 grant order 状态：人工 decision 后终态正确，重复 decision 没有重复扣减或释放 quota。
- 只读检查 quota reservation / ledger 相关表：正常路径、额度不足、人工终态、并发冲突后金额或额度没有漂移。
- 只读检查 export 相关表：rowCount、checksum、筛选快照、明细状态和重复纳入约束与 API 返回一致。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

后续 Test Agent 还应按本地项目实际启动方式执行 Admin API smoke。命令名称以仓库现有脚本为准；如果没有现成脚本，应使用 `curl` 或临时外部脚本手工调用 API，不提交该脚本。

建议命令清单：

~~~bash
# 1. 全量 Go 回归
go test ./...

# 2. 构建验证
go build ./...

# 3. 如仓库已有 smoke 脚本，则执行对应 Admin 运维 smoke
# 示例占位：以实际存在脚本替换，不得新增测试代码后提交
./scripts/admin-ops-smoke.sh

# 4. PostgreSQL 只读 smoke，使用本地实际连接串
psql "$DATABASE_URL" -c "select action, resource, resource_id, tenant_id, created_at from marketing_audit where tenant_id in ('tenant_admin_ops_a','tenant_admin_ops_b') order by created_at desc limit 50;"
~~~

## Data / Smoke

后续 Test Agent 执行 smoke 前应准备以下数据，并记录每个对象 ID 供复查：

- `tenant_admin_ops_a`：正常发奖事件 1 个，quota 不足事件 1 个，重复幂等事件 1 组，DLQ outbox 至少 2 个，MANUAL grant order 至少 3 个，MANUAL_EXPORT grant order 至少 3 个。
- `tenant_admin_ops_b`：与 A tenant 使用相同或相似业务 ID 的隔离数据至少 1 组。
- 审计 source：至少一组带 `traceId` 和 `requestId`，至少一组缺失 source 摘要用于历史兼容空态验证。
- 敏感字段样本：payload 或 targetRef 中放置 token/secret 样式值，验证 API 和页面均只显示脱敏摘要。
- 并发数据：为 DLQ、manual decision、manual export 分别准备独立对象，避免并发用例互相污染。

Smoke 判定标准：

- 正常路径可从事件定位到完整对账链路。
- 额度不足路径不创建错误的发奖或 outbox，并解释拒绝原因。
- 幂等重复不产生重复业务副作用。
- 并发冲突最终状态单一、可解释、审计可追踪。
- PostgreSQL 只读检查与 API / 页面展示一致。
- 所有危险操作都有 operator、reason、确认、审计。
- 跨 tenant 查询和操作均不泄露、不修改其他 tenant 数据。
- 敏感信息不明文展示。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-admin-ops-pages-dlq
