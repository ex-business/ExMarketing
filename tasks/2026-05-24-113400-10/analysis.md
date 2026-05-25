# Analysis

## 需求 ID

2026-05-24-113400-10

## 标题

审计记录与对账

## Review 反馈处理

本次读取 `review.md` 后确认其仍为模板内容，未列出实际 P0、P1、P2 条目，也未填写明确打回原因。

- `P0:`、`P1:`、`P2:`：模板占位，无具体问题；本次无逐条修复项。
- `Open Questions: 待填写`：已在本分析文档补充“待确认问题”，用于后续 Review Agent 判断是否仍需追问。
- `Decision: PASS / BLOCK`：模板占位，无实际结论；本次按任务要求重新细化需求并给出新的 Score 与 Decision。

## 背景目标

ExMarketing 是独立活动营销中台，只负责活动配置、规则判断、奖励计算、预算库存、参与记录、幂等控制、发奖编排、审计对账，不直接修改外部业务系统余额，不连接外部业务库。

本需求来自《22-独立活动营销中台设计》中的“操作审计”“审计对账”“审批流配置”“模拟运行”“发奖失败/DLQ/人工处理”等能力要求。目标是让运营配置变更、上线审批、模拟运行、发奖失败、预算扣减失败、DLQ 人工操作等关键动作都有可追踪审计记录，并能围绕活动、用户、事件、发奖单进行排查和对账。

核心目标：

- 所有高风险运营动作和关键系统动作必须留下审计记录。
- 审计记录必须能说明谁在什么时间对什么对象做了什么动作，以及动作前后状态或快照。
- 对账查询必须串联活动、事件、参与记录、发奖单、Outbox、回调结果、Quota 影响和相关审计。
- 审计与对账只覆盖营销中台自身数据，不查询外部钱包、支付、会员或业务库。

## 参与方

- 运营人员：创建、修改、审批、上线、暂停活动，处理 DLQ，查看审计与对账。
- 审批人员：审批活动上线或高危配置变更。
- 技术运营 / 客服 / 财务对账人员：按活动、用户、事件、发奖单定位发奖、预算、Outbox、回调问题。
- 外部业务系统：上报事件、接收发奖指令、回调发奖结果。
- ExMarketing 系统：处理事件、生成参与记录、发奖单、Outbox，执行预算库存扣减，记录系统级审计。

## 输入输出

### 审计记录输入

审计来源包括两类：

- 运营动作：活动创建、活动配置修改、活动状态变更、上线审批、Quota 创建或调整、模拟运行、DLQ replay、DLQ close。
- 系统动作：事件接入、重复事件识别、规则命中或拒绝、Quota/Budget/Stock/UserLimit 扣减失败、发奖投递失败进入 DLQ、发奖回调成功或失败、对账异常识别。

每条审计记录至少需要包含：

- `tenantId`：租户 ID。
- `actor`：操作者；运营动作记录真实操作者，系统自动动作记录稳定系统身份，例如 `system`、`worker`、`openapi`。
- `action`：动作类型，必须为稳定枚举，不使用自由文本替代。
- `resource`：对象类型，例如 `ACTIVITY`、`QUOTA`、`EVENT`、`PARTICIPATION`、`GRANT_ORDER`、`OUTBOX`、`SIMULATION`、`APPROVAL`。
- `resourceId`：对象 ID；对象尚未创建或跨对象动作时可使用主对象 ID，并在 payload 中补充关联对象。
- `beforeSnapshot`：动作前关键状态；创建类动作可为空。
- `afterSnapshot`：动作后关键状态；失败类动作可为空，但必须记录失败上下文。
- `payload`：请求摘要、原因、错误码、关联 ID、差异摘要等扩展信息。
- `createdAt`：服务端记录时间。

不得记录 secret、token、完整 Authorization header、外部系统敏感凭证或不必要的个人敏感信息。

### 对账查询输入

对账至少需要支持以下查询维度：

- 按活动：`tenantId + activityId` 或 `tenantId + activityCode`。
- 按用户：`tenantId + subjectType + subjectId`。
- 按事件：`tenantId + eventId` 或标准幂等键 `tenantId:sourceSystem:eventType:bizType:bizId`。
- 按发奖单：`tenantId + commandId/billNo` 或 `grantOrderId`。

查询条件应支持时间范围、状态过滤和分页，避免一次性返回无限结果。

### 对账查询输出

对账结果需要返回一条可读链路：

- 事件 Inbox：事件信息、幂等键、处理状态、错误信息。
- 活动命中：活动 ID、活动版本、匹配/拒绝原因。
- 参与记录：participation ID、状态、rejectReason、命中快照摘要。
- 奖励与发奖单：grantOrder ID、commandId/billNo、奖励金额、币种、发奖状态、错误信息。
- Outbox：outbox ID、投递目标、发送状态、重试次数、DLQ 状态、错误信息。
- 回调：外部发奖 ID、最终状态、回调时间、冲突或重复标记。
- Quota 影响：预算、库存、用户次数的扣减、冻结、确认、释放或失败信息。
- 审计时间线：相关审计记录，按时间正序或倒序展示，包含 actor、action、resource、snapshot/diff 摘要。

## 业务规则

### 必须记录审计的动作

- 活动配置创建、修改、状态变更、上线审批、暂停、结束。
- Quota/Budget/Stock/UserLimit 创建和调整。
- 模拟运行发起与结果摘要；模拟运行不能产生真实事件、参与、发奖、Outbox 或 Quota 扣减。
- 标准事件接入，至少记录事件入站或关键失败；重复事件必须可追踪但不得造成重复发奖或重复计数。
- 规则通过后进入发奖链路的关键状态变化。
- Quota/Budget/Stock/UserLimit 扣减不足或冲突失败。
- 发奖指令生成、投递失败、重试耗尽进入 DLQ。
- DLQ replay、DLQ close；必须记录操作者和原因。
- 发奖回调 SUCCESS/FAILED、重复回调、状态冲突、缺少 reservation、Quota 聚合冲突等对账相关异常。

### 审计快照规则

- 配置类动作必须记录前后快照或差异，至少能还原关键配置：活动基础信息、状态、时间、时区、规则、奖励、Quota、互斥叠加、发奖方式。
- 审批类动作必须记录审批人、审批动作、审批原因、审批前状态、审批后状态。
- 失败类动作必须记录错误码、错误消息、关联对象和可排查上下文。
- 系统自动动作若无人工操作者，actor 使用稳定系统身份，不能留空。
- 审计记录应追加写入，不允许通过后续业务操作覆盖历史记录。

### 对账规则

- 对账必须严格按 `tenantId` 隔离，任何查询不得跨租户返回数据。
- 发奖单对账以 `tenantId + commandId/billNo` 唯一定位，不允许只用 commandId 跨租户查询。
- 事件对账以标准幂等键串联事件、参与记录、发奖单和 Outbox。
- 活动对账需要能看到活动版本和参与快照，避免活动配置后续修改导致历史命中不可解释。
- Quota 不足场景如果事务回滚了 participation/grant_order/outbox，仍必须能通过审计看到失败原因和 quotaType/scope/scopeKey。
- DLQ close 后不得再 replay；对账输出必须区分 `DLQ` 与 `DLQ_CLOSED`。
- 重复事件、重复回调、相反终态回调必须在对账中可识别。

## 边界条件

- 内存仓储可用于本地快速开发，但涉及审计持久化、事务一致性和对账链路的验收必须以 PostgreSQL 语义为准。
- 模拟运行只记录模拟审计和结果摘要，不写真实业务流水。
- 创建类动作没有 beforeSnapshot；删除或关闭类动作可以没有 afterSnapshot，但必须记录最终状态。
- 对账查询目标不存在时返回明确的未找到结果，不应伪造空链路。
- 部分链路不存在是合法结果，例如规则拒绝无发奖单、零奖励无 Outbox、Quota 不足无 participation/grant_order/outbox。
- 审计 payload 允许扩展，但稳定检索字段不能只藏在不可查询 JSON 内。

## 异常场景

- operator 缺失：运营动作必须拒绝或记录为不可接受的请求错误，不允许产生无 actor 审计。
- reason 缺失：DLQ replay/close、审批驳回、高危配置调整等人工动作必须要求原因。
- 重复事件：返回 duplicate，不新增 participation/grant_order/outbox，对账能定位原始事件处理结果。
- Idempotency-Key 与 payload 不一致：返回业务错误，并能审计失败请求摘要。
- Quota 不足：返回可识别 quota 类型、scope、scopeKey，并产生审计记录。
- Outbox 投递失败：记录重试、最终 DLQ 和错误信息；人工处理动作继续追加审计。
- 回调冲突：相反最终态、externalGrantId 冲突、reservation 缺失或 quota 聚合冲突必须能在对账中看到。
- 审计写入失败：关键业务动作不应被标记为成功且无审计；是否采用同事务强一致或失败中止需要在实现前明确。

## 非目标范围

- 不建设完整生产级鉴权、权限、RBAC 或审批流引擎。
- 不连接外部钱包、支付、会员、订单系统做跨系统对账。
- 不新增 MQ 发奖、人工导出发奖或外部账单拉取能力。
- 不修改奖励计算、规则引擎、Quota 扣减算法本身。
- 不引入新的活动玩法插件。
- 不做审计数据归档、冷热分层、报表 BI 或长期数据仓库。
- 不用审计记录替代 participation、grant_order、outbox、quota 等业务事实表。

## 风险

- 审计若与业务动作非原子写入，可能出现“业务成功但无审计”或“审计成功但业务失败”的解释风险。
- 快照过大可能导致存储膨胀，需要控制摘要字段和完整快照边界。
- actor 来源若没有统一约定，会导致审计无法追责。
- 对账跨多张表，缺少稳定关联键会导致链路断裂；必须优先依赖 tenantId、idempotencyKey、participationId、grantOrderId、billNo、outboxId。
- 审计 payload 若包含请求原文，可能泄露 token 或敏感业务字段。
- 当前文档中的审批流仍是目标能力，若实际活动上线审批 API 尚未存在，本需求只能定义审计要求，不能强行实现完整审批流。

## 验收标准

- 创建活动、创建 Quota、活动配置修改或状态变更后，能查询到包含 actor、action、resource、resourceId、before/after 或 payload、createdAt 的审计记录。
- 上线审批或审批驳回动作有独立审计记录，包含审批人、审批结果、原因、前后状态。
- 模拟运行产生审计记录和模拟结果摘要，但不会创建 event_inbox、participation、grant_order、outbox，也不会扣减 quota。
- 标准事件成功处理后，能按事件幂等键查询到事件、参与记录、发奖单、Outbox、Quota 影响和相关审计时间线。
- 规则拒绝、零奖励、Quota 不足、重复事件等无发奖链路场景，对账结果能解释为什么没有发奖单或 Outbox。
- 发奖失败进入 DLQ 后，能按活动、用户、事件或发奖单查到 outbox 错误、重试次数、DLQ 状态和审计记录。
- DLQ replay/close 必须要求 operator 和 reason，并写入审计；`DLQ_CLOSED` 不允许 replay。
- 发奖回调 SUCCESS/FAILED、重复回调、状态冲突均能通过发奖单对账查询看到最终状态或错误原因。
- 所有对账查询按 tenantId 隔离；跨租户 commandId 或 eventId 不得返回其他租户数据。
- 审计记录不得包含 secret、token、完整 Authorization header。
- PostgreSQL 验证通过；涉及事务和对账链路的验收不能只以内存仓储为准。

## 待确认问题

- actor 的来源是否统一采用请求体 `operator`，还是管理后台统一传递请求头，例如 `X-Operator`？
- 活动“修改配置、上线审批、暂停、结束”的现有 API 是否已定义？如果尚未定义，本需求是否只要求为这些动作预留审计语义，还是同时补齐最小管理 API？
- 审计记录是否要求业务动作与审计写入强一致，即审计失败时业务动作整体失败？
- 对账查询是否需要导出 CSV/Excel，还是仅提供分页 JSON 查询？
- 审计保留周期、分页上限和可查询时间范围是否有运营或合规要求？

## Score

```text
需求清晰度        17/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        19/20
总分              89/100
```

## Decision

```text
PASS
```

需求细节已明确到可审核、可验收。待确认问题不阻塞进入 Review/Development，但 Dev Agent 不应在本需求内扩展完整鉴权、RBAC、外部系统对账或审批流引擎。
