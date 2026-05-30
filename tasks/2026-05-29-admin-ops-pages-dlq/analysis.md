# Analysis

## Review 反馈处理

- P1：审计页要求时间范围、`traceId/requestId` 过滤，但现有 `docs/api.md` 的 `GET /admin/v1/audit` 只声明 `tenantId`、`actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`limit`。处理：本期默认纳入审计查询能力扩展，新增 `startTime`、`endTime`、`traceId`、`requestId` 查询口径；其中时间过滤匹配 `marketing_audit.created_at`，`traceId/requestId` 匹配 `payload_json.source.traceId/requestId` 摘要字段。页面不得只做有限返回集前端过滤，也不得静默忽略过滤条件。若历史审计没有 source trace/request 字段，只返回可匹配记录并展示“历史记录可能缺少 source 摘要”的空态/提示。
- P2：manual decision 与 manual export 审计 action 未定义，导致测试难断言。处理：本分析补齐最小审计契约，新增本任务可验收 action：`MANUAL_GRANT_DECISION`、`MANUAL_GRANT_EXPORT_CREATE`、`MANUAL_GRANT_EXPORT_STATUS`。同时明确 resource、resourceId、payload 最小字段和敏感信息脱敏要求。
- P2：普通 Outbox 队列页、发奖单查询页、导出文件长期列表/下载接口被误读为全量分页列表能力。处理：本分析明确列表支持过滤分页只适用于已有或本任务明确扩展的查询 API；普通 Outbox、grant order、导出文件长期列表如未定义独立 API，本期不是全量列表能力，只通过对账查询、DLQ 查询和操作返回的真实数据完成定位、追踪和最小展示。
- Open Question：审计查询是否本任务内扩展后端 API。结论：是。默认最小可验收口径为扩展 `GET /admin/v1/audit` 支持 `startTime/endTime/traceId/requestId` 服务端过滤，关闭上一轮 BLOCK。
- Open Question：manual decision 与 manual export 审计 action 是否已有约定。结论：现有文档未约定，由本任务新增上述最小 action 契约，关闭待确认问题。

## Analysis 自检修正

- 上一轮 `analysis.md` 自评 `Score=88`、`Decision PASS`，但 Review 判定存在 P1 且 `Score=76`、`Decision BLOCK`。修正：本轮不沿用旧 PASS，按 Review 的 P1/P2 重写关键需求口径。
- 旧分析错误假设：把审计 time/trace 过滤写成验收项，但未说明 API 是否扩展。修正：明确本任务必须扩展审计服务端查询参数和字段来源。
- 旧分析扣分项：缺少 manual decision / manual export 最小审计 action 契约。修正：补充 action、resource、resourceId、payload 最小字段。
- 旧分析扣分项：列表分页边界不清。修正：明确“全量列表分页”不是所有页面共同承诺；缺少独立 API 的页面只做真实数据定位展示，不做伪全量列表。
- 旧待确认问题：普通 Outbox、grant order、导出文件长期列表/下载是否补齐。修正：默认本期不补齐这些独立长期列表/下载 API；但 audit time/trace 查询必须补齐，因为它是 task 明确验收项且现有 API 无法满足。

## 背景目标

活动引擎上线后，运营、技术支持、客服和财务需要在后台定位发奖失败、Outbox 堆积、DLQ、callback 异常、quota 失败、人工发奖和导出发奖问题。本需求的目标是把现有审计、对账、DLQ、人工发奖、导出发奖能力整理为可验收的 Admin 运维页面，并补齐少量对页面验收必需的 API 契约。

本需求不是重做活动引擎、发奖状态机或权限系统。页面必须基于服务 API 返回的真实数据解释链路，不能通过直连数据库、前端假数据或本地缓存伪造成功状态。

## 参与方

- 运营人员：查询审计、定位活动发奖状态、人工确认发奖、创建导出发奖。
- 技术运营 / 技术支持：排查 Outbox、DLQ、callback、quota、重试、错误上下文。
- 客服 / 财务对账人员：按用户、事件、发奖单、导出文件追踪奖励处理结果。
- ExMarketing 后端：提供 Admin API、审计记录、对账链路、DLQ 操作、人工发奖和导出发奖。
- 外部奖励系统 / 线下处理方：作为 callback 或导出文件处理结果来源；本任务只追踪状态，不接入新的外部系统。

## 需求说明

### 1. 操作审计页

- 查询条件：`tenantId` 必填；可选 `actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`startTime`、`endTime`、`traceId`、`requestId`、`limit`。
- 本期必须扩展 `GET /admin/v1/audit` 支持 `startTime/endTime/traceId/requestId` 服务端过滤。
- `startTime/endTime` 使用 RFC3339，按 `marketing_audit.created_at` 闭区间或半开区间过滤，具体边界需在 API 文档固定；默认最小口径为 `created_at >= startTime` 且 `created_at < endTime`。
- `traceId/requestId` 来源为 `payload_json.source.traceId` 与 `payload_json.source.requestId`。只匹配脱敏 source 摘要字段，不匹配完整 payload 文本。
- 列表展示 `createdAt`、`actor`、`action`、`resource`、`resourceId`、`tenantId`、source 摘要、业务摘要。
- 详情展示 before/after、错误上下文或操作摘要，但不得展示 Authorization、Cookie、token、secret、完整 broker/provider 凭据、完整敏感 payload。

### 2. 对账查询页

- 查询条件：`tenantId` 必填；可按 `activityId`、`activityCode`、`subjectType + subjectId`、`eventId`、`idempotencyKey`、`grantOrderId`、`commandId/billNo`、`outboxId` 定位。
- 输出 event inbox、participation、grant order、outbox、callback、quota reservation、相关审计时间线。
- 对账页要解释“为什么没有 grant order/outbox”：包括规则拒绝、quota 不足、零奖励、重复事件、人工发奖不生成自动 outbox 等场景。
- 目标不存在时展示明确未找到，不显示成功态，不构造空链路。

### 3. 发奖单查询页

- 展示 `tenantId`、`grantOrderId`、`billNo/commandId`、`activityId`、`participationId`、`subjectType`、`subjectId`、`rewardType`、`rewardAmount`、`grantStatus`、`grantChannel`、`retryCount`、`errorMessage`、`idempotencyKey`、`createdAt`、`updatedAt`。
- 本期默认不新增 grant order 全量分页列表 API。最小可验收口径为：通过对账查询返回的真实 grant order 信息完成定位和详情展示。
- 页面标题或空态需明确“按条件定位发奖单”，不能承诺“全量发奖单列表”。

### 4. Outbox 队列页

- 展示 `id`、`tenantId`、`messageType`、`targetChannel`、脱敏 `targetRef`、`sendStatus`、`retryCount`、`nextRetryAt`、`errorMessage`、`createdAt`、`updatedAt`。
- DLQ 类数据使用 `GET /admin/v1/outbox/dlq?tenantId=...&includeClosed=...`。
- 普通 Outbox 非 DLQ 数据，本期默认不新增全量分页列表 API。最小可验收口径为：在对账链路中展示指定事件/发奖单关联 outbox；DLQ 页展示 `DLQ` 与可选 `DLQ_CLOSED`。
- 因没有普通 Outbox 全量列表 API，本期不验收普通 Outbox 的全局状态分页、全局 targetChannel 过滤、全局 retryCount 区间过滤；这些属于后续增强。

### 5. DLQ 运维页

- 默认展示当前 `tenantId` 的 `DLQ`；开启 `includeClosed=true` 后同时展示 `DLQ_CLOSED`。
- 展示错误上下文、`retryCount`、`targetChannel`、脱敏 `targetRef`、payload 摘要、最后更新时间。
- 支持 `replay` 和 `close`。操作前必须二次确认，`operator` 与 `reason` 必填。
- `replay` 只允许当前 `sendStatus=DLQ`，成功后同一 outbox 回到 `PENDING`，payload 不变。
- `close` 只允许当前 `sendStatus=DLQ`，成功后同一 outbox 进入 `DLQ_CLOSED`，不修改 participation、grant order 或 quota reservation。
- `DLQ_CLOSED` 不允许 replay。

### 6. 人工发奖页

- 支持按 `tenantId` 加 `grantOrderId` / `billNo` / `commandId` / `idempotencyKey` 定位人工发奖单。
- 支持确认 `SUCCESS`、确认 `FAILED`、关闭 `CLOSED`。
- `operator`、`reason` 必填；每次操作必须确认，并展示后端返回的真实结果。
- `MANUAL_EXPORT` 的 `SUCCESS` 只允许已进入 READY 导出文件明细且处于 `SENDING` 的单据；未导出单据可通过 `FAILED` / `CLOSED` 取消。
- 纯 `MANUAL` 审计不要求 `exportId`。

### 7. 导出发奖页

- 支持创建 `MANUAL_EXPORT` 导出文件。
- 导出只选择 `grantMode=MANUAL_EXPORT` 且 `PENDING` / `RETRY` 的 grant order。
- 创建后展示真实 `exportId`、文件名、格式、状态、`rowCount`、`checksum`、筛选快照、createdAt、updatedAt 和明细状态摘要。
- 未失败 / 未作废导出明细会阻止同一 grant order 被重复纳入。
- 本期默认不新增导出文件长期列表和下载接口。最小可验收口径为：创建导出后展示本次 API 返回的真实导出文件和明细摘要；长期历史列表、文件下载、导出回执导入为后续能力。

## 输入输出

### 通用输入

- `tenantId`：所有查询和操作必填。
- `operator`：所有危险操作必填，包括 DLQ replay/close、人工确认、导出创建。
- `reason`：所有危险操作必填，不能为空白字符串。
- `limit`：默认 100；审计最大 500。未定义 pagination token/offset 的接口只展示当前返回集。
- 时间输入：`startTime/endTime` 使用 RFC3339，以服务端时间字段过滤。

### 主要输出

- 审计输出：`actor`、`action`、`resource`、`resourceId`、`tenantId`、`createdAt`、source 摘要、before/after 或错误上下文摘要。
- 对账输出：event inbox、participation、grant order、outbox、callback、quota reservation、审计时间线。
- DLQ 输出：outbox ID、状态、targetChannel、脱敏 targetRef、retryCount、nextRetryAt、errorMessage、payload 摘要、审计记录。
- 人工发奖输出：grant order 当前状态、前后状态变化、quota reservation 影响摘要、审计记录。
- 导出输出：exportId、fileName、fileFormat、status、rowCount、checksum、filterSnapshot、明细摘要、审计记录。

## 最小审计契约

现有 action 继续沿用：

- `OUTBOX_DLQ_REPLAY`：resource=`OUTBOX`，resourceId=outboxId。
- `OUTBOX_DLQ_CLOSE`：resource=`OUTBOX`，resourceId=outboxId。

本任务新增或固化以下 action：

- `MANUAL_GRANT_DECISION`：人工发奖确认成功、确认失败、关闭统一使用该 action；resource=`GRANT_ORDER`；resourceId=grantOrderId 或 billNo/commandId 中后端稳定主键。
- `MANUAL_GRANT_EXPORT_CREATE`：创建导出文件；resource=`MANUAL_GRANT_EXPORT`；resourceId=exportId。
- `MANUAL_GRANT_EXPORT_STATUS`：导出文件进入 READY/FAILED/CLOSED 等状态变化时记录；resource=`MANUAL_GRANT_EXPORT`；resourceId=exportId。

审计 payload 最小字段：

- 通用：`tenantId`、`operator`、`reason`、`source`、`beforeStatus`、`afterStatus`、`createdAt` 或操作完成时间摘要。
- DLQ：`outboxId`、`targetChannel`、脱敏 `targetRef`、`previousSendStatus`、`newSendStatus`、`retryCount`、错误摘要。
- 人工发奖：`grantOrderId`、`billNo/commandId`、`idempotencyKey`、`grantMode`、`decisionStatus`、`previousGrantStatus`、`newGrantStatus`、可选 `exportId`。
- 导出创建：`exportId`、`fileFormat`、`rowCount`、`checksum`、筛选快照摘要、纳入/跳过数量摘要。
- 导出状态：`exportId`、`previousStatus`、`newStatus`、`rowCount`、失败原因摘要。

审计 payload 禁止包含 Authorization、Cookie、token、secret、完整 broker/provider 凭据、完整 targetRef 凭据、完整敏感 payload。页面展示时再次脱敏。

## 业务规则

1. Tenant 隔离
   - 所有查询和操作必须以 `tenantId` 为边界。
   - `commandId`、`eventId`、`grantOrderId`、`outboxId`、`exportId` 不能跨 tenant 返回或更新数据。
   - 危险操作 tenant 不匹配时必须失败，不能暴露其他 tenant 数据。

2. 审计规则
   - DLQ replay/close、人工发奖 decision、导出创建、导出状态变化必须写审计。
   - 审计记录必须能被审计页按 tenant、业务对象、时间和 trace/request 过滤查到。
   - 没有 trace/request 的历史记录不补造字段。

3. DLQ 规则
   - `replay` 和 `close` 只允许 `DLQ`。
   - `DLQ_CLOSED` 不允许 replay。
   - MQ outbox 的 `SENT` 只代表 broker ack / confirm，不代表真实权益发放；真实发奖结果以 callback 或人工确认终态为准。

4. 人工发奖规则
   - `MANUAL` / `MANUAL_EXPORT` 不生成自动 outbox，不由 outbox worker 投递，不进入自动 DLQ。
   - `SUCCESS`、`FAILED`、`CLOSED` 为终态；重复同终态操作必须幂等，不得重复移动 quota。
   - 人工确认和 reward callback 是不同入口；`MANUAL` / `MANUAL_EXPORT` 默认不依赖 callback。

5. 导出发奖规则
   - `MANUAL` grant order 不得进入导出文件。
   - 空候选不得创建成功文件，需展示后端真实失败/跳过原因。
   - 导出创建必须记录筛选条件快照和 checksum，便于事后对账。

6. 列表能力边界
   - backlog 中“列表支持过滤分页”的本期可验收对象为审计列表和 DLQ 列表，以及已有 API 返回的对账结果集合。
   - 普通 Outbox 全量列表、grant order 全量列表、导出文件长期列表/下载不在本期最小范围，除非实现阶段已有明确 API 文档补充。

## 边界条件

- 审计查询无 `endTime` 时按 `startTime` 之后查；无 `startTime` 时按 `endTime` 之前查；均无时按现有默认倒序返回。
- `traceId/requestId` 同时存在时采用 AND 过滤，避免误扩大结果。
- `includeClosed=false` 时 DLQ 页默认只展示 `DLQ`；开启后同时展示 `DLQ_CLOSED`。
- 空列表必须有明确空状态，不能显示为加载失败。
- `targetRef`、payload、errorMessage 只展示脱敏摘要；完整敏感内容不展示。
- 页面面向本地/内网 Admin 运维；生产公网暴露前必须另行补齐 Admin 鉴权/RBAC。

## 异常场景

- 缺少 `tenantId`：查询或操作失败，提示 tenant 必填。
- 审计时间格式非法：提示 RFC3339 格式错误，不发起无效查询或展示后端校验错误。
- 后端尚未支持 `startTime/endTime/traceId/requestId` 时：本任务验收不通过，不能降级为前端过滤。
- 缺少 `operator` 或 `reason`：危险操作失败，不允许提交。
- 跨 tenant 查询或操作：返回未找到或越权错误，不返回其他 tenant 数据。
- DLQ replay/close 目标不是 `DLQ`：失败并展示后端原因。
- `DLQ_CLOSED` replay：失败，不允许重新打开。
- 对账目标不存在：展示未找到，不显示成功态或伪造空链路。
- 导出筛选结果为空或重复纳入：展示真实失败/跳过原因。
- 并发 DLQ 操作、并发人工确认、并发导出：以后端最终状态为准，页面刷新并展示冲突或已处理状态。
- 后端接口非 2xx、网络失败、参数校验失败：页面展示真实错误码/错误消息，不显示操作成功。

## 非目标范围

- 不做权限系统、RBAC、登录态或生产级 Admin 鉴权。
- 不做批量资金修复工具。
- 不直接修改数据库，不绕过服务 API。
- 不连接外部钱包、支付、会员、订单系统做跨系统对账。
- 不新增外部 reward callback 协议，不替代现有 callback。
- 不实现导出回执自动解析或批量导入成功/失败结果。
- 不新增普通 Outbox 全量分页列表、grant order 全量分页列表、导出文件长期列表或文件下载接口作为本期最小验收。
- 不做审计归档、BI 报表、长期数据仓库或复杂数据保留策略。

## 风险

- 审计 API 扩展风险：time/trace/request 服务端过滤涉及 `created_at` 和 JSON source 字段查询；需要明确字段来源和性能边界，否则页面验收不可达。
- 审计 action 兼容风险：新增 manual/export action 后，既有审计查询和对账时间线需能显示未知或新 action。
- API 覆盖风险：普通 Outbox、grant order、导出历史列表缺少独立 API，必须在页面文案和验收中限制为定位/追踪能力。
- 操作风险：DLQ close、人工 FAILED/CLOSED、导出状态变化会影响业务处理路径，必须二次确认、原因必填、审计可追踪。
- 租户隔离风险：按 commandId、eventId、outboxId 查询时若缺少 tenant 约束可能泄露数据；页面和 API 调用必须始终携带 tenantId。
- 隐私风险：payload、targetRef、错误信息和导出文件可能包含主体、业务 ID 或外部引用；页面必须默认脱敏。

## 验收标准

1. 审计页通过服务端 API 按 `tenantId`、actor、action、resource、resourceId、activityId、eventId、commandId、`startTime/endTime`、`traceId/requestId` 查询，不能只做前端过滤。
2. 审计查询时间过滤按 `marketing_audit.created_at` 生效，`traceId/requestId` 按 `payload_json.source` 摘要字段生效；历史缺字段记录有明确空态或提示。
3. DLQ replay 成功后 outbox 从 `DLQ` 回到 `PENDING`，并可查到 `OUTBOX_DLQ_REPLAY` 审计。
4. DLQ close 成功后 outbox 从 `DLQ` 进入 `DLQ_CLOSED`，并可查到 `OUTBOX_DLQ_CLOSE` 审计。
5. `DLQ_CLOSED` 不允许 replay，页面展示真实失败原因。
6. 人工发奖页可按 tenant + grant order 定位单据，并执行 `SUCCESS` / `FAILED` / `CLOSED`；操作原因必填，结果可在对账中追踪，并可查到 `MANUAL_GRANT_DECISION` 审计。
7. 导出发奖页可创建 `MANUAL_EXPORT` 导出，展示真实 exportId、rowCount、checksum、筛选快照和明细摘要，并可查到 `MANUAL_GRANT_EXPORT_CREATE` 审计。
8. 导出 READY/FAILED/CLOSED 等状态变化如由本任务触发或暴露，必须记录 `MANUAL_GRANT_EXPORT_STATUS` 审计；未暴露状态变更操作时，页面至少展示创建返回的真实状态。
9. 对账页能按活动、主体、事件、幂等键、grant order / commandId 查询，展示 inbox、participation、grant order、outbox、callback、quota reservation 和审计时间线。
10. 普通 Outbox、发奖单、导出文件长期列表若无独立 API，页面必须明确为“按条件定位/本次结果展示”，不得宣称全量分页列表，不允许前端假数据或直接查库。
11. 所有危险操作缺少 `operator` 或 `reason` 时不能提交；后端拒绝时页面不得显示成功。
12. 所有页面查询和操作必须按 `tenantId` 隔离；跨 tenant 的 commandId/eventId/outboxId/grantOrderId/exportId 不得返回或更新其他租户数据。
13. 敏感信息不在页面明文展示，包括 Authorization、Cookie、token、secret、broker/provider 凭据、完整 targetRef、完整敏感 payload。
14. 空列表、未找到、接口失败、状态冲突、网络失败均有明确状态提示。
15. smoke 测试或手工验收脚本覆盖：审计 time/trace 查询、对账查询、DLQ replay、DLQ close、人工发奖 decision、导出创建、敏感字段脱敏、跨 tenant 拒绝。
16. `go test ./...` 通过。

## 待确认问题

- 普通 Outbox 全量分页列表、grant order 全量分页列表、导出文件长期列表/下载是否后续补齐？默认本期不阻塞，不作为最小验收。
- Admin operator 最终来源是否来自登录用户、请求头还是页面输入？默认本期页面输入 `operator`，生产化前需统一鉴权身份。
- 审计和对账是否需要导出 CSV/Excel？默认本期不做通用审计/对账导出，只做 manual grant export 的业务导出。
- 审计保留周期、最大时间跨度和 JSON source 字段索引策略是否有生产约束？默认本期满足本地/内网运维查询，生产规模优化另立任务。

## Score

```text
91
```

评分：

- 需求清晰度：19/20。页面范围、参与方、输入输出、API 扩展点和最小列表口径明确。
- 业务规则完整：18/20。覆盖 tenant、审计、DLQ、人工发奖、导出发奖、对账规则，并补齐 manual/export 审计契约。
- 边界异常清楚：18/20。覆盖审计过滤缺口、跨租户、缺 operator/reason、状态冲突、空结果、脱敏、接口失败。
- 验收标准明确：18/20。验收项可直接转为 smoke 或手工脚本，且明确前端过滤不可替代服务端审计过滤。
- 约束一致性：18/20。与 `task.md`、`docs/api.md`、`docs/data-model.md`、`docs/operations.md`、`requirements/backlog.json` 对齐；对现有文档缺口给出默认最小可验收口径。

## Decision

```text
PASS
```

Review 的 P1/P2 和 Open Questions 已关闭。现有材料足以确定业务目标、验收对象和参与方；剩余问题均有默认最小可验收口径，不构成 Decision BLOCK。
