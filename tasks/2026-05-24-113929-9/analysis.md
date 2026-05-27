# Analysis

## 需求 ID

2026-05-24-113929-9

## 标题

操作审计记录

## Review 反馈处理

本次已读取 `tasks/2026-05-24-113929-9/review.md`。该文件仍为模板内容，未给出实际 Review 结论或打回原因。

- `P0:`、`P1:`、`P2:`：均为空模板，无具体问题需要修复。
- `Open Questions: 待填写`：模板占位，无明确问题；本次在“待确认问题”中只保留不阻塞开发的边界说明。
- `Decision: PASS / BLOCK`：模板占位，不是实际 Review 决策；本次重新输出可审核需求和评分。

## Analysis 自检修正

上一版 `analysis.md` 仍为模板，存在 `Score 0`、`Decision PASS / BLOCK`、现状/目标/方案/风险/验收均“待填写”的问题。本次修正如下：

- 关闭 `Score 0`：按评分规则重新评估为 90/100。
- 关闭 `Decision PASS / BLOCK` 模板：本次给出明确 `PASS`。
- 补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准。
- 明确本任务与后续 `2026-05-24-113929-10 对账查询能力` 的边界：本任务只要求审计记录写入和审计查询过滤，不实现完整对账链路。
- 对不唯一的信息给出默认最小可验收口径：操作者优先来自请求体 `operator`；系统动作使用稳定系统 actor；请求来源记录为 `source` 摘要，不记录敏感 header 原文。

## 背景目标

ExMarketing 是独立活动营销中台，关键运营动作和关键系统异常会影响活动配置、上线、发奖、预算、库存和 DLQ 人工处置。当前数据模型已有 `marketing_audit`，API 文档也已定义部分审计 action，但本需求要求把操作审计口径补齐到可追责、可查询、可验收。

目标是：运营修改配置、上线审批、模拟运行、发奖失败、预算扣减失败、DLQ 操作都必须留下审计记录。每条审计记录至少说明操作者、动作、对象、前后快照、时间和请求来源。

## 参与方

- 运营人员：修改活动配置、发起模拟运行、处理 DLQ。
- 审批人员：提交、通过或驳回上线审批。
- 技术运营：查询审计记录，排查配置、预算扣减、发奖失败和 DLQ 处置过程。
- ExMarketing 系统：在自动处理事件、预算扣减、Outbox 投递失败等场景写入系统审计。
- 外部业务系统：通过事件、发奖回调等接口触发系统动作；不是审计记录的人工操作者。

## 输入输出

### 审计输入

审计来源分为人工操作和系统动作：

- 人工操作：活动配置修改、上线审批提交/通过/驳回、模拟运行、DLQ replay、DLQ close。
- 系统动作：发奖失败、Outbox 重试耗尽进入 DLQ、预算/库存/次数扣减失败、事件处理中的关键失败。

人工操作的默认最小输入：

- `tenantId`：必填，用于租户隔离。
- `operator`：必填，作为审计 `actor`。
- `reason`：审批驳回、DLQ replay/close、人工关闭或风险处置类动作必填；普通配置修改可选。
- 请求来源：从服务端可获得的来源摘要生成，例如 API 名称、HTTP method/path、可脱敏 client IP、userAgent 摘要、requestId/traceId；不得记录完整 Authorization、token、secret 或原始敏感 header。

系统动作的默认最小输入：

- `tenantId`：必须从业务对象或请求上下文取得。
- `actor`：使用稳定系统身份，例如 `system`、`worker`、`openapi`。
- `source`：记录触发来源，例如 `event_api`、`outbox_worker`、`reward_callback`、`quota_reservation`。
- 关联对象：尽量记录 `activityId`、`eventId`、`eventInboxId`、`grantOrderId`、`commandId`、`outboxId`、`quotaType`、`scope`、`scopeKey`。

### 审计输出

写入 `marketing_audit` 的记录至少包含：

- `tenant_id`：租户 ID。
- `actor`：操作者；人工动作来自 `operator`，系统动作来自稳定系统身份。
- `action`：稳定枚举。
- `resource`：对象类型。
- `resource_id`：主对象 ID。
- `payload_json`：前后快照、差异摘要、原因、错误上下文、请求来源、关联 ID。
- `created_at`：服务端写入时间。

审计查询输出最小口径：

- 支持按 `tenantId` 查询，`tenantId` 必填。
- 支持按 `actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`limit` 过滤。
- 默认按 `createdAt` 倒序返回。
- `limit` 默认 100，必须有上限，默认最大不超过 500。

## 业务规则

### 必须记录的 action

本需求至少覆盖以下 action：

- `UPDATE_ACTIVITY`：活动主配置、展示入口配置、规则、奖励或状态修改。
- `APPROVAL_SUBMIT`：提交上线审批。
- `APPROVAL_APPROVE`：审批通过。
- `APPROVAL_REJECT`：审批驳回。
- `SIMULATION_RUN`：模拟运行。
- `GRANT_FAILED`：发奖回调失败或发奖单被失败终结。
- `OUTBOX_DELIVERY_FAILED`：Outbox 投递失败达到需要记录的失败节点。
- `OUTBOX_ENTER_DLQ`：Outbox 重试耗尽进入 DLQ。
- `OUTBOX_DLQ_REPLAY`：人工重放 DLQ。
- `OUTBOX_DLQ_CLOSE`：人工关闭 DLQ。
- `QUOTA_EXCEEDED`：预算、库存、用户次数或每日次数扣减失败。

已有 action `CREATE_ACTIVITY`、`CREATE_QUOTA`、`EVENT_IN` 可继续保留，不要求本任务删除或重命名。

### resource 口径

`resource` 使用稳定对象类型：

- `ACTIVITY`：活动配置和状态修改。
- `APPROVAL`：审批动作。
- `SIMULATION`：模拟运行。
- `GRANT_ORDER`：发奖失败或回调失败。
- `OUTBOX`：投递失败、DLQ、重放、关闭。
- `QUOTA`：预算、库存、次数扣减失败。
- `EVENT`：事件处理相关失败。

`resourceId` 默认使用主对象 ID；当对象尚未创建或失败发生在事务回滚后，使用可定位的业务键，并在 payload 中补充关联 ID。

### 快照与 payload 规则

- 配置修改必须记录 `beforeSnapshot` 和 `afterSnapshot`，至少包含活动状态、版本、时间、timezone、priority、互斥叠加、config 摘要、规则摘要、奖励摘要。
- 审批动作必须记录审批前状态、审批后状态、锁定版本、审批人、原因和是否高危。
- 模拟运行必须记录输入事件摘要、候选/命中/未命中数量、是否存在可发奖结果、是否有 quota 风险；不得记录完整敏感 attributes 或 provider 响应。
- 发奖失败必须记录 `grantOrderId`、`commandId`、失败状态、错误码/消息、外部回调状态或 worker 错误摘要。
- 预算扣减失败必须记录 `activityId`、`eventInboxId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason`、`error`。
- DLQ 操作必须记录 `outboxId`、原状态、新状态、operator、reason、targetChannel、targetRef 摘要、retryCount。
- 审计记录只能追加写入，不允许业务操作覆盖或删除历史审计。

### 请求来源规则

- 请求来源写入 payload 的 `source` 字段。
- 人工 HTTP 请求默认记录 `method`、`path`、`requestId/traceId`、脱敏 IP、userAgent 摘要。
- 系统动作默认记录内部来源，例如 `outbox_worker`、`event_api`、`reward_callback`。
- 不记录完整 Authorization header、Bearer token、Cookie、secret、Fact Provider 密钥、MQ 凭据、完整外部敏感响应。

### 事务一致性默认口径

对配置修改、审批、DLQ replay/close、quota 超时释放等人工关键动作，审计写入应与业务状态变更同成功同失败；若审计写入失败，该业务动作不能对外返回成功。

对 Outbox worker 投递失败、进入 DLQ、发奖失败等异步系统动作，审计写入失败不能吞掉原业务错误；但该失败必须可通过日志或重试机制暴露。本需求的最小验收以正常审计写入成功路径和业务失败已记录审计为准。

## 边界条件

- 模拟运行允许写审计，但不得写真实 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox` 或扣减 quota。
- Quota 不足导致业务事务回滚时，仍必须留下 `QUOTA_EXCEEDED` 审计。
- DLQ close 后不得再 replay；失败尝试也应返回明确错误，是否记录失败尝试审计不作为本期强制项。
- `operator` 缺失的人工动作应返回请求错误，不允许写入 actor 为空的审计。
- `tenantId` 缺失或不匹配时不得跨租户写入或查询审计。
- 创建类动作可无 `beforeSnapshot`；失败类动作可无 `afterSnapshot`，但必须有错误上下文。
- payload 允许扩展，但查询过滤需要依赖稳定字段或 payload 中稳定键，不能只依赖自由文本。

## 异常场景

- 活动配置更新失败：不应写成功审计；如记录失败审计，action 必须能区分失败或 payload 中明确 `success=false`。
- 审批版本不一致：拒绝审批，并记录或返回版本冲突；不得把旧版本审批写成通过。
- 模拟运行输入非法：返回校验错误；不强制写 `SIMULATION_RUN` 成功审计。
- 发奖回调失败状态：写 `GRANT_FAILED`，并保留 commandId、状态和消息摘要。
- Outbox 投递错误：记录投递失败摘要；重试耗尽进入 DLQ 时写 `OUTBOX_ENTER_DLQ`。
- Quota 扣减并发冲突或额度不足：写 `QUOTA_EXCEEDED`，不能生成 grant_order/outbox 或重复扣减。
- 审计查询超大 limit：按上限截断或返回参数错误，不能无限制扫描。

## 非目标范围

- 不实现 `2026-05-24-113929-10` 的完整对账查询能力。
- 不建设完整 RBAC、权限系统、登录态系统或审批流引擎。
- 不新增外部钱包、支付、会员、订单系统对账。
- 不修改奖励计算、规则引擎、Quota 扣减算法或 Outbox 重试策略本身。
- 不做审计归档、冷热分层、报表 BI、CSV/Excel 导出。
- 不要求记录完整请求/响应原文。
- 不用审计替代 participation、grant_order、outbox、quota 等业务事实表。

## 风险

- 如果审计与关键业务变更不一致，可能出现“业务成功但无审计”的合规风险。
- 快照过大可能造成审计表膨胀，需要记录关键快照和摘要，不记录完整敏感 payload。
- actor 和 source 口径不统一会降低可追责性，本需求默认统一使用 `operator` 和 payload.source。
- 异步 worker 的审计失败较难与原业务动作完全原子，需要通过日志、重试或告警补偿。
- 审计查询若没有 tenantId 强制过滤，会产生跨租户泄露风险。

## 验收标准

- 活动配置修改成功后，能查询到 `UPDATE_ACTIVITY` 审计，包含 tenantId、actor、resource、resourceId、beforeSnapshot、afterSnapshot、createdAt 和 source。
- 上线审批提交、通过、驳回动作分别产生审计，包含审批人、动作、审批前后状态、锁定版本、原因和请求来源。
- 模拟运行成功后产生 `SIMULATION_RUN` 审计，包含模拟输入摘要和结果摘要；同时确认未写真实 event_inbox、participation、grant_order、outbox，未扣减 quota。
- 发奖失败或失败回调会产生 `GRANT_FAILED` 审计，包含 grantOrderId 或 commandId、错误摘要和时间。
- Outbox 投递失败进入 DLQ 时产生 `OUTBOX_ENTER_DLQ` 审计；DLQ replay/close 必须要求 operator 和 reason，并分别写 `OUTBOX_DLQ_REPLAY` / `OUTBOX_DLQ_CLOSE`。
- 预算、库存、用户次数或每日次数不足时返回可识别错误，并写 `QUOTA_EXCEEDED` 审计，包含 quotaType、scope、scopeKey、amount、activityId 和事件/主体摘要。
- `GET /admin/v1/audit` 支持按 tenantId、actor、action、resource、resourceId、activityId、eventId、commandId、limit 过滤，默认按创建时间倒序返回。
- 人工动作缺少 operator 时返回 400 类错误，不产生 actor 为空的成功审计。
- 审计 payload 不包含 secret、token、完整 Authorization、Cookie、Fact Provider 明文密钥、MQ 凭据或完整敏感外部响应。
- PostgreSQL 模式下审计持久化验收通过；涉及事务一致性的验收不能只以内存仓储为准。

## 待确认问题

- 是否需要把失败的非法人工请求也写入审计？默认最小口径：只强制记录成功动作和已进入业务处理的关键失败，参数校验失败不强制写审计。
- 请求来源是否要包含完整 client IP？默认最小口径：只记录脱敏 IP 或可追踪 requestId，避免扩大隐私风险。
- 审计保留周期和归档策略未定义；默认本期不做归档，只保证查询有分页上限。

以上问题不阻塞本期开发；默认口径已足够覆盖任务验收。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          17/20
验收标准明确      18/20
架构一致性        18/20
总分              90/100
```

## Decision

```text
PASS
```

需求已明确到可审核、可验收。剩余待确认问题均已给出默认最小可验收口径，不构成 BLOCK。
