# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 和本文件的 `Decision` 都为 `PASS` 后，才允许在 Dev Agent 完成实现后执行测试。

本计划只定义后续应执行的测试用例、数据构造和命令清单。Test Plan Agent 本轮未执行测试、构建、格式化或 git 命令。

## Test Strategy

测试目标是验证管理端只读对账查询 `GET /admin/v1/reward/reconciliation` 能按活动、用户/主体、事件、参与记录、发奖单、outbox 串起 ExMarketing 内部链路，并能定位“业务日志写了但消息失败”“发奖成功但状态未更新”等异常。

覆盖层级：

- Handler/API 参数校验：`tenantId`、定位条件、`limit`、`cursor`、`from/to`、`subjectType + subjectId`。
- Repository/Query 链路拼装：event inbox、participation、grant_order、outbox、quota reservation、audit timeline。
- Diagnostics：按链路状态生成机器可读诊断项，不把正常业务终态误报为异常。
- 租户隔离与脱敏：所有查询限定 tenant，响应不得包含 token、secret、Authorization、Cookie、callback 签名、broker 凭据或完整敏感 payload。
- 幂等与并发：重复事件不重复发奖；并发查询不修改业务数据且结果稳定。
- PostgreSQL smoke：使用真实 PostgreSQL schema 与迁移，验证 SQL、索引依赖、分页和数据类型映射。

## Test Cases

### 1. 参数校验

1. 缺少 `tenantId`
   - 请求：`GET /admin/v1/reward/reconciliation?activityId=A1`
   - 期望：`400`，错误码为实现统一后的对账查询参数错误码，例如 `INVALID_RECONCILIATION_QUERY` 或项目最终约定值。

2. 只有 `tenantId`，无任何定位条件
   - 请求：`GET /admin/v1/reward/reconciliation?tenantId=T1`
   - 期望：`400`，不得全表扫描。

3. `subjectType` 和 `subjectId` 只传其一
   - 请求分别只传 `subjectType=USER` 或只传 `subjectId=U1`
   - 期望：`400`。

4. 非法分页与时间参数
   - `limit=0`、`limit=-1`、`limit=501`、非数字 limit。
   - `from/to` 格式非法，或 `from > to`。
   - 期望：均返回 `400`，错误响应不泄露 SQL。

5. 合法但无数据
   - 请求：`tenantId=T1&eventId=missing-event`
   - 期望：`200`，`items=[]`，`diagnostics=[]` 或等价空诊断结构。

### 2. 正常路径

1. 按 `activityId` 查询完整成功链路
   - 构造：event inbox 已处理，participation 已生成，quota reservation 已确认或无额度活动返回空数组，grant_order 为 `SUCCESS`，outbox 为 `SENT`，audit 有事件处理、发奖单创建、outbox 发送、callback 成功记录。
   - 请求：`tenantId=T1&activityId=A1&limit=100`
   - 期望：返回该活动链路，包含 eventInbox、participation、grantOrder、outbox、quotaReservations、auditTimeline；时间线按时间升序；无异常 diagnostics。

2. 按 `subjectType + subjectId` 查询
   - 构造：同一用户在 T1 有多条链路，另一个用户和另一个租户也有相似数据。
   - 请求：`tenantId=T1&subjectType=USER&subjectId=U1`
   - 期望：只返回 T1/U1 数据，不返回 T1/U2 或 T2/U1。

3. 按 `eventId` 查询多活动命中
   - 构造：同一个 event inbox 命中两个 activity，生成两个 participation，每个 participation 有自己的发奖单和 outbox。
   - 请求：`tenantId=T1&eventId=E1`
   - 期望：返回两条或一个事件下两组 participation 链路，不丢任何 grant_order/outbox。

4. 按标准事件 `idempotencyKey` 查询
   - 构造：`T1:source:eventType:bizType:bizId` 对应 event inbox 和下游链路。
   - 请求：`tenantId=T1&idempotencyKey=<standard-event-key>`
   - 期望：能从 event inbox 下钻到 participation、grant_order、outbox。

5. 按 grant order 幂等键查询
   - 构造：传入的是 `participation.idempotencyKey:rewardCode`。
   - 请求：`tenantId=T1&idempotencyKey=<grant-order-key>`
   - 期望：能定位 grant_order 并回溯 participation/event inbox。

6. 按 `participationId` 查询多奖励
   - 构造：一个 participation 对应两个 rewardCode，产生两个 grant_order 和两个 outbox。
   - 请求：`tenantId=T1&participationId=P1`
   - 期望：两个 grant_order/outbox 全部返回，不只返回第一条。

7. 按 `commandId` / `grantOrderId` 查询
   - 构造：`marketing_grant_order.bill_no = CMD1`，有 quota reservation 和 audit。
   - 请求：`tenantId=T1&commandId=CMD1`；另测 `grantOrderId=G1`。
   - 期望：返回发奖单、上游 participation/event inbox、关联 outbox、quota reservation、audit timeline。

8. 按 `outboxId` 查询
   - 构造一条可通过 `commandId` 或 `idempotencyKey` 回溯的 outbox。
   - 请求：`tenantId=T1&outboxId=O1`
   - 期望：返回 outbox 及可关联的 grant_order、participation、event inbox。

### 3. 异常定位与 Diagnostics

1. `EVENT_RECEIVED_NO_PARTICIPATION`
   - 构造：event inbox 存在且非重复，但无 participation。
   - 期望：返回 eventInbox，可空 participation/grantOrder/outbox，diagnostics 包含 `EVENT_RECEIVED_NO_PARTICIPATION`。

2. `PARTICIPATION_REJECTED`
   - 构造：participation 为 `REJECTED`，有 `rejectReason`，无 grant_order/outbox。
   - 期望：diagnostics 包含 `PARTICIPATION_REJECTED`，标识业务拒绝，不标记 outbox 失败。

3. `ZERO_REWARD_SKIPPED`
   - 构造：participation 为 `QUALIFIED`，拒绝/跳过原因表示 0 奖励，无 grant_order/outbox。
   - 期望：diagnostics 包含 `ZERO_REWARD_SKIPPED`，不误报为发奖失败。

4. `PARTICIPATION_REWARDABLE_NO_GRANT_ORDER`
   - 构造：participation 为可发奖状态，但无 grant_order。
   - 期望：diagnostics 包含该 code。

5. `GRANT_ORDER_NO_OUTBOX`
   - 构造：grant_order 存在，找不到关联 outbox。
   - 期望：diagnostics 包含 `GRANT_ORDER_NO_OUTBOX`。

6. 业务日志写了但消息失败
   - 构造：grant_order 存在，outbox 为 `FAILED`、`DLQ`、`DLQ_CLOSED` 各一例。
   - 期望：diagnostics 包含 `OUTBOX_FAILED_OR_DLQ`，响应展示 sendStatus、retryCount、errorMessage 脱敏摘要。

7. outbox 已发送但发奖未终结
   - 构造：outbox 为 `SENT`，grant_order 为 `PENDING`、`SENDING` 或 `RETRY`。
   - 期望：diagnostics 包含 `OUTBOX_SENT_GRANT_PENDING`。

8. 发奖成功但状态未更新
   - 构造：grant_order 为 `SUCCESS`，participation 不是 `GRANTED`。
   - 期望：diagnostics 包含 `GRANT_SUCCESS_PARTICIPATION_NOT_GRANTED`。

9. 发奖失败但额度仍冻结
   - 构造：grant_order 为 `FAILED` 或 `CLOSED`，quota reservation 仍为 `FROZEN`。
   - 期望：diagnostics 包含 `GRANT_FAILED_QUOTA_STILL_FROZEN`。

10. 发奖待定且额度冻结过久
    - 构造：grant_order 未终结，reservation 长时间 `FROZEN`，超过实现定义的阈值。
    - 期望：diagnostics 包含 `GRANT_PENDING_QUOTA_FROZEN_TOO_LONG`。

### 4. 额度不足

1. 活动额度不足导致 participation 被拒绝
   - 构造：活动配置有限额，已用尽；新事件命中活动后 participation 为 `REJECTED` 或项目既有额度不足状态，`rejectReason` 明确为 quota insufficient。
   - 查询：按 `eventId`、`activityId`、`subjectType + subjectId` 各查一次。
   - 期望：返回 participation 和拒绝原因；不生成 grant_order/outbox；diagnostics 应按业务拒绝处理，不误报 `GRANT_ORDER_NO_OUTBOX`。

2. 用户/主体额度不足
   - 构造：全局活动额度仍有余量，但 subject 维度额度不足。
   - 期望：只影响该 subject，其他 subject 正常；对账结果能展示 quota reservation 为空或失败摘要。

### 5. 幂等重复

1. 重复事件幂等
   - 构造：同一 `tenantId/sourceSystem/eventType/bizType/bizId` 上报两次或直接插入重复 inbox 标记。
   - 查询：按 `idempotencyKey` 和 `eventId`。
   - 期望：只关联原始 participation/grant_order/outbox；不会显示重复发奖；重复事件本身如可见，应标明 duplicate 或处理状态。

2. 重复 callback 幂等
   - 构造：同一 commandId 收到重复成功 callback。
   - 查询：按 `commandId`。
   - 期望：grant_order 仍为单一成功终态，audit 可展示重复 callback 摘要但不产生重复终态或重复 quota 确认。

3. 多条件冲突
   - 请求：`tenantId=T1&eventId=E1&commandId=CMD-other`
   - 期望：`200`，空结果；不得放宽为只按 event 或只按 command 查询。

### 6. 并发

1. 并发只读查询稳定性
   - 准备一批完整链路数据。
   - 并发执行 50 到 200 个相同 reconciliation 查询。
   - 期望：全部 `200`；响应结构一致；无 data race、panic、连接泄露或超时。

2. 查询与写入并发
   - 在事件处理、outbox 状态更新或 callback 状态更新过程中并发执行 reconciliation 查询。
   - 期望：查询不修改业务状态；允许读到某个一致时间点的中间态，并给出对应 diagnostics；不得返回半结构化错误或 500。

3. 宽查询分页并发
   - 对同一 activity 准备超过 500 条链路。
   - 并发请求第一页和后续 cursor 页。
   - 期望：每页不超过 limit；cursor 可继续翻页；无重复或明显漏页，排序稳定。

### 7. PostgreSQL Smoke

1. 真实 PostgreSQL schema smoke
   - 使用项目标准 PostgreSQL 测试库和迁移创建表。
   - 插入最小完整链路：event inbox、participation、grant_order、outbox、quota reservation、audit。
   - 执行 reconciliation 查询。
   - 期望：SQL 能在真实 PostgreSQL 执行通过，字段类型、JSON/JSONB、时间、decimal、分页 cursor 均能正确映射。

2. PostgreSQL 缺失下游 smoke
   - 插入只有 event inbox、只有 grant_order、孤立 outbox 三类历史不完整数据。
   - 期望：查询返回可定位片段和 diagnostics，不因 left join/null 扫描失败。

3. PostgreSQL 租户隔离 smoke
   - 在 T1/T2 插入相同 eventId、commandId 或 subjectId。
   - 期望：按 T1 查询只返回 T1；按 T2 查询只返回 T2。

4. PostgreSQL 敏感字段脱敏 smoke
   - 在 outbox payload、audit detail 或 errorMessage 中放入 `Authorization`、`Cookie`、`token`、`secret`、callback signature 模拟值。
   - 期望：API 响应不包含原始敏感值。

## Required Commands

后续 Test Agent 在 Dev 实现完成后，应按项目实际测试环境执行以下命令。若项目提供 Makefile、docker compose 或专用 testcontainer 脚本，可使用等价项目标准命令，但必须覆盖同等范围。

~~~bash
go test ./...
go build ./...
~~~

建议补充执行的专项命令清单：

~~~bash
# 仅在项目已有 PostgreSQL 集成测试标签/环境变量时执行，命令名称按实际实现调整
POSTGRES_DSN="$POSTGRES_DSN" go test ./... -run 'Reconciliation|Postgres|PostgreSQL' -count=1

# 并发与 data race，若项目耗时可限定 reconciliation 相关 package
go test ./... -race -run 'Reconciliation|Concurrent|Idempotent' -count=1
~~~

手工 API smoke 命令模板，需先替换 host、token、tenantId 和测试数据 ID：

~~~bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$ADMIN_BASE_URL/admin/v1/reward/reconciliation?tenantId=$TENANT_ID&activityId=$ACTIVITY_ID&limit=100"

curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$ADMIN_BASE_URL/admin/v1/reward/reconciliation?tenantId=$TENANT_ID&eventId=$EVENT_ID"

curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$ADMIN_BASE_URL/admin/v1/reward/reconciliation?tenantId=$TENANT_ID&commandId=$COMMAND_ID"

curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$ADMIN_BASE_URL/admin/v1/reward/reconciliation?tenantId=$TENANT_ID&outboxId=$OUTBOX_ID"
~~~

## Data / Smoke

后续 Test Agent 至少准备以下命名数据集，优先使用测试 fixture、factory 或迁移后的真实 repository 写入，避免绕过业务唯一约束：

- `recon_success_full_chain`：完整成功链路。
- `recon_activity_multi_reward`：一个 participation 多个 grant_order/outbox。
- `recon_event_multi_activity`：一个 event 命中多个 activity。
- `recon_quota_insufficient_activity`：活动额度不足。
- `recon_quota_insufficient_subject`：用户/主体额度不足。
- `recon_duplicate_event`：重复事件幂等。
- `recon_duplicate_callback`：重复 callback 幂等。
- `recon_outbox_failed_dlq`：outbox FAILED/DLQ/DLQ_CLOSED。
- `recon_grant_success_participation_stale`：grant_order SUCCESS 但 participation 未 GRANTED。
- `recon_grant_failed_quota_frozen`：grant_order FAILED/CLOSED 但 quota 仍 FROZEN。
- `recon_orphan_outbox`：无法回溯上游的孤立 outbox。
- `recon_tenant_isolation`：T1/T2 相同业务 ID。
- `recon_sensitive_payload`：包含敏感字段的 outbox/audit/error 数据。

每个数据集测试后需确认：

- reconciliation 查询是只读操作，相关业务表记录数和状态未被查询修改。
- 所有响应对象都限定在请求 `tenantId`。
- 可空片段使用 `null`、空数组或项目约定结构表达，不因缺失下游导致 500。
- diagnostics code 使用稳定、可断言的机器可读字符串。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113929-10
