# Analysis

## 需求信息

- 需求 ID：`2026-05-24-113400-11`
- 标题：管理后台页面
- 风险等级：中
- 来源：`22-独立活动营销中台设计`
- 本轮角色：Analysis Agent，仅做需求分析和需求细化

## Review 反馈处理

本轮已先读取 `tasks/2026-05-24-113400-11/review.md`。该文件仍为 Review Agent 模板，没有填写实质打回项：

- `P0:`：为空。处理：无具体 P0 需要关闭。
- `P1:`：为空。处理：无具体 P1 需要关闭。
- `P2:`：为空。处理：无具体 P2 需要关闭。
- `Open Questions`：内容为“待填写”。处理：无 Review Agent 明确问题；本轮基于 `task.md`、上游设计和现有 API 文档补充待确认问题。
- `Decision`：内容为 `PASS / BLOCK` 占位。处理：不作为有效审核结论；本轮重新评分和决策。

上一轮 Analysis 失败的核心原因是：原需求包含审批流、上线发布和活动编辑发布闭环，但公开 API 未定义审批、上线、编辑、复制、暂停、恢复、结束、版本回滚等契约，导致 Score `79/100`、Decision `BLOCK`。

本轮 `task.md` 已新增 Owner 补充决策，明确回答了上一轮 BLOCK 的关键问题：

- 本期只交付“当前后端 API 已支持或已在本工程明确存在的本地管理页面”。
- 审批流、上线发布、活动编辑发布、复制、删除、暂停、恢复、结束、版本回滚均属于后续独立任务。
- 本期页面可以展示入口或占位状态，但必须禁用这些操作，不能伪造成功，不能用前端状态假装后端支持。
- 本期页面定位为本地开发 / 内网管理台，不作为公网生产 Admin。
- 所有页面操作必须展示后端真实错误原因，接口不存在或失败时不得显示成功。

处理结论：

- 上一轮审批 / 上线接口缺失不再阻塞本期，因为 Owner 已确认审批流和上线发布不在本期实现范围内。
- 上一轮活动编辑发布接口缺失不再阻塞本期，因为 Owner 已确认编辑、复制、删除、暂停、恢复、结束、版本回滚均延期。
- 本轮需求边界调整为“现有 API 范围内的通用本地管理页面 + 未接入能力禁用展示”，需求可审核、可验收。
- 未接入能力仍需作为页面约束写入验收：不能出现可点击成功路径，不能用 mock 或前端本地状态冒充真实后端结果。

## 参考资料

本轮只读取需求、任务产物和文档，未读取业务代码，未设计具体代码改法。

- `tasks/2026-05-24-113400-11/review.md`
- `tasks/2026-05-24-113400-11/task.md`
- `tasks/2026-05-24-113400-11/analysis.md` 历史版本
- `tasks/2026-05-24-113400-7/analysis.md`
- `tasks/2026-05-24-113400-10/analysis.md`
- `README.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/roadmap.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`
- `requirements/backlog.json`

## 背景目标

ExMarketing 是独立活动营销中台，只负责活动配置、规则判断、奖励计算、预算库存、参与记录、幂等控制、发奖编排、审计对账，不连接外部业务库，不直接修改外部余额，不内置具体业务系统模型。

上游设计要求后台具备活动配置、规则配置、奖励配置、预算库存、互斥叠加、风控限制、发奖方式、审批流、模拟运行、操作审计等能力。Owner 本轮已将本期范围收敛为现有后端能力可支撑的本地 / 内网页面。

本期目标：

- 建立通用管理后台页面，不再为每个活动类型单独堆一套页面。
- 支持当前 API 已明确的活动列表、新增活动、活动详情只读展示、Quota 创建和展示、模拟运行、审计查询、对账查询、DLQ 运维，以及 Agent 工作台 / git diff 相关页面。
- 对当前未接入的审批、上线、编辑发布、复制、删除、暂停、恢复、结束、版本回滚等能力做禁用和待接入展示。
- 所有成功状态必须来自真实后端结果；所有失败必须展示真实错误原因。

## 参与方

- 运营人员：通过通用页面创建活动、配置规则和奖励、创建或查看 quota、执行模拟运行、查看错误原因。
- 技术运营 / 运维：查看审计、对账、DLQ，执行当前 API 已支持的 DLQ replay / close。
- 开发 / Agent 工作流使用者：通过本地管理台查看需求、Agent 状态、任务产物、运行日志和当前 git diff。
- 外部业务系统：上报标准事件，接收 HTTP / Webhook 发奖指令，回调发奖结果；本页面不直接操作外部系统。
- ExMarketing 后端：提供本期页面依赖的真实 API 和业务校验结果。

## 本期范围

### 本期必须覆盖

- 活动列表：加载并展示活动列表。
- 新增活动：通过现有 `POST /admin/v1/activities` 创建活动，展示后端返回结果。
- 活动详情只读：展示活动基础信息、规则、奖励、quota、互斥叠加、插件、发奖方式和版本等后端可返回内容；如缺少详情 API，则只能基于列表返回内容展示只读摘要并标注能力边界。
- Quota 创建和展示：通过现有 quota 能力创建或展示 `BUDGET`、`STOCK`、`USER_LIMIT`；缺失 quota 明确展示为 `unlimited`。
- 模拟运行：通过 `POST /admin/v1/simulate` 发起模拟，展示命中 / 未命中原因、规则解释、奖励预览、预算库存影响、参与 / 发奖单 / outbox 预览，并明确模拟无真实业务副作用。
- 审计记录查询：通过 `GET /admin/v1/audit` 展示审计记录和过滤条件。
- 对账查询：通过 `GET /admin/v1/reward/reconciliation` 按活动、主体、事件、发奖单或幂等键展示链路；如果页面入口存在但接口返回无结果或错误，必须展示真实状态。
- DLQ 运维：通过 `GET /admin/v1/outbox/dlq`、`POST /admin/v1/outbox/{id}/replay`、`POST /admin/v1/outbox/{id}/close` 展示和操作当前租户 DLQ。
- Agent 工作台 / git diff：展示当前工程已有工作流页面能力和 git diff 能力，不扩展提交或发布语义。
- 后端错误展示：接口不存在、接口返回非 2xx、校验失败、状态冲突、跨租户或参数错误均展示失败，不显示成功。

### 本期必须禁用或标注待接入

- 提交审批、审批通过、审批拒绝、撤回。
- 上线、下线、发布、版本回滚。
- 活动编辑、复制、删除、暂停、恢复、结束。
- 生产公网 Admin 鉴权、完整 RBAC、SSO。
- MQ Command Out、MANUAL_EXPORT、MANUAL 发奖完整状态机。
- 当前后端未实现或会拒绝的 rule type、reward type、grant channel、quota 业务流程。

## 输入输出

### 页面输入

- 租户：`tenantId`，所有管理查询和操作必须限定租户。
- 活动基础字段：`activityCode`、`activityName`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、可选 `pluginCode`、`config`。
- 规则配置：`ruleCode`、`rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version`。
- 奖励配置：`rewardCode`、`rewardType`、`calculateType`、`rewardConfig`、`grantMode`、`grantChannel`、`flowMultiple`、`enabled`、`version`。
- Quota 配置：`activityId`、`quotaType`、`scope`、`scopeKey`、`totalValue`。
- 模拟事件：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 审计过滤：`tenantId`、`actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`limit`。
- 对账过滤：`tenantId`，以及 `commandId/grantOrderId`、`activityId`、`subjectType+subjectId`、`eventId`、`idempotencyKey` 中至少一种。
- DLQ 操作：`tenantId`、outbox ID、`operator`、`reason`。

### 页面输出

- 活动列表：租户、活动编码、名称、类型、状态、触发事件、开始 / 结束时间、时区、优先级、互斥组、是否叠加、版本、更新时间。
- 活动详情只读摘要：基础配置、规则、奖励、quota、插件、互斥叠加、发奖方式、版本和后端可返回的审计摘要。
- 创建活动结果：成功时展示后端返回对象；失败时展示后端错误码、错误消息和字段级原因。
- Quota：quota 类型、scope、scopeKey、total / used / frozen / version；缺失 quota 展示 `unlimited`。
- 模拟结果：候选数量、命中 / 未命中活动、规则级解释、奖励预览、quota 影响预览、participation / grant_order / outbox 预览、插件预检查和状态变更意图。
- 审计列表：actor、action、resource、resourceId、payload 或 before / after snapshot、createdAt。
- 对账链路：event inbox -> participation -> grant_order -> outbox -> callback -> quota reservations -> audit timeline。
- DLQ：状态、错误原因、重试次数、targetRef、replay / close 结果和对应审计。
- 待接入能力：显示为禁用状态、不可执行说明或真实接口错误；不得展示“已成功”。

## 业务规则

### 通用页面规则

- 页面必须以通用活动模型组织，不能按首存、续存、签到、任务、返水、抽奖等活动类型硬编码独立页面。
- 普通活动优先通过活动、规则、奖励、quota 配置表达；只有普通配置无法表达的新玩法才选择 `pluginCode`。
- 外部业务字段只能通过标准事件 `attributes`、`facts.*`、活动 `config` 或 `state.*` 被规则引用，不得提升为 ExMarketing 顶层模型。
- 本期页面定位为本地开发 / 内网管理台，不作为公网生产 Admin；若将来生产暴露，必须另行定义鉴权、角色权限和 operator 来源。
- 页面不得使用静态 mock、前端本地状态或假成功替代真实后端结果。

### 活动配置规则

- 活动状态展示限定为 `DRAFT`、`SCHEDULED`、`ACTIVE`、`PAUSED`、`ENDED`。
- `timezone` 必填，开始 / 结束时间必须以明确时区展示。
- `triggerEvents` 必填，活动匹配依赖标准事件 `eventType`。
- `priority` 用于互斥组排序，值越大越先匹配。
- `exclusiveGroup` 表示互斥组；`stackable=false` 时同组默认只命中一个。
- 创建活动必须依赖后端 Schema 校验；非法配置不得显示为保存成功。
- 本期只允许新增活动和只读查看；编辑、复制、删除、暂停、恢复、结束、上线、版本回滚入口必须禁用或标注待接入。

### 规则配置规则

- 合法 `rulePhase` 为 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`；`BUDGET` 不是合法规则阶段。
- 页面展示执行顺序为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`，其中 `RISK_LIMIT_BUDGET` 汇总 `RISK`、`LIMIT` 和 quota 风险结果。
- 本期可执行规则类型以 `CONDITION` 为准；`FORMULA`、`LIMIT`、`SCRIPT` 等如后端未完整支持，应禁用或标注未支持。
- `CONDITION` 字段路径限定为 `attributes.*`、`facts.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`，以及绑定插件时的 `state.plugin.<pluginCode>.*`。
- 条件操作符限定为 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。
- 字段缺失、类型不匹配、非法操作符或未注册 evaluator 不能默认通过，必须展示后端失败原因。
- “风控 / 限制”只配置和展示 `RISK` / `LIMIT` 条件、IP / Device / UserLimit / Quota 风险预览，不宣称真实风控判定。

### 奖励配置规则

- 当前普通活动可创建奖励类型以 `BONUS`、`CASH`、`POINT` 为准。
- 当前已实现计算方式为 `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
- 奖励金额、预算、库存、流水倍数等数值必须使用 Decimal 字符串语义；页面不得引导使用浮点近似。
- `BONUS` / `CASH` 必须配置 `currency`。
- `FIXED` 必须配置 `amount`。
- `RATIO` 必须配置 `baseField` 和 `rate`，可配置 `maxAmount`。
- `LADDER` 使用闭区间阶梯，重叠阶梯必须校验失败。
- `RANDOM` / `PROBABILITY` 必须可审计；概率权重非法不得创建为可执行配置。
- `POOL_SPLIT` 只表示事件即时分摊，不表示活动结束后统一奖池结算。
- 计算结果为 `0.0000` 时展示为无实际奖励：不生成 grant_order / outbox，不扣 BUDGET / STOCK / USER_LIMIT。

### 发奖方式规则

- 本期可执行边界为 `grantMode=AUTO` 且 `grantChannel=HTTP` 或 `WEBHOOK`。
- `rewardConfig.targetRef` 对可执行发奖必填，且必须是 `http` 或 `https` URL。
- `MQ`、`MANUAL_EXPORT`、`MANUAL` 当前未完整实现或创建会被拒绝；页面如展示这些选项，必须标注不可用并阻止提交为可执行活动。
- ExMarketing 不直接修改用户余额，只生成发奖单和 Outbox 指令。

### 预算库存与限制规则

- 页面需支持创建或查看 `BUDGET`、`STOCK`、`USER_LIMIT`；`DAILY_LIMIT` 模型已定义但业务流程待扩展，应标注待扩展。
- `BUDGET` 使用 `ACTIVITY + TOTAL`，扣减金额为同一 activity 本次所有 reward amount 合计。
- `STOCK` 使用 `ACTIVITY + rewardCode`，每个 enabled reward 默认扣 1。
- `USER_LIMIT` 使用 `USER + subject.id`，每个 rewardable participation 默认扣 1。
- 缺少 quota 行表示无限制，页面必须展示 `unlimited`，避免运营误以为已配置限制。
- Quota 不足时不能生成 grant_order / outbox，模拟运行应展示 `QUOTA_EXCEEDED` 和影响预览。

### 审批、上线和编辑发布规则

- 审批流和上线发布不在本期实现范围内。
- 本期不得提供可点击成功的提交审批、审批通过、审批拒绝、撤回、上线、下线、发布、版本回滚。
- 如果展示审批 / 上线区域，只能展示禁用状态和“待后端接入”说明。
- 不能在前端本地修改活动状态模拟审批成功、上线成功或发布成功。
- 活动编辑、复制、删除、暂停、恢复、结束、版本回滚不在本期实现范围内；相关入口必须禁用或标注待接入。

### 模拟运行规则

- 模拟运行必须无真实业务副作用，不写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。
- 模拟结果必须展示命中和未命中原因、规则级解释、奖励预览、预算库存影响、发奖指令预览。
- 若 quota 预览 `wouldExceed=true`，活动应展示 `QUOTA_EXCEEDED`，并且不展示可执行的 grant order / outbox command 预览。
- 模拟运行事件必须使用标准事件模型，业务字段只能放在 `attributes`。
- 模拟结果只代表当前配置和当前 quota 快照预览，不代表 quota 已锁定、奖励已发放、插件状态已消费或发奖指令已投递。

### 审计、对账和 DLQ 规则

- 审计页面至少展示当前 API 已记录 action：`CREATE_ACTIVITY`、`CREATE_QUOTA`、`EVENT_IN`、`SIMULATION_RUN`、`QUOTA_EXCEEDED`、`OUTBOX_DLQ_REPLAY`、`OUTBOX_DLQ_CLOSE`。
- 审计查询支持按 `tenantId`、`actor`、`action`、`resource`、`resourceId`、`activityId`、`eventId`、`commandId`、`limit` 过滤。
- 对账查询必须按租户隔离，支持 `commandId/grantOrderId`、`activityId`、`subjectType+subjectId`、`eventId`、`idempotencyKey`。
- 对账页面展示 event inbox、participation、grant_order、outbox、callback、quota reservations 和 audit timeline；链路缺失时说明原因，不能把空白链路显示为成功。
- DLQ `replay` 和 `close` 只允许当前状态为 `DLQ`，`DLQ_CLOSED` 不允许 replay，`operator` 和 `reason` 必填。

## 边界条件

- 空列表：无活动、无 quota、无审计、无对账记录、无 DLQ 时必须有明确空状态。
- 缺失 quota：展示为 `unlimited`，不是错误。
- 时间边界：结束时间早于开始时间应被后端拒绝；页面展示后端错误。
- 多奖励活动：每个 reward 分别展示 `rewardCode`、计算方式、库存 `scopeKey`、金额预览和发奖预览。
- 互斥叠加：同一 `exclusiveGroup` 且不可叠加时，应能展示优先级和最终命中结果。
- 插件活动：未注册插件或与活动类型不兼容时，创建失败必须展示后端错误。
- 后端拒绝字段：未知字段、非法枚举、不可解析 Decimal、非法 URL、非法规则路径必须展示后端错误。
- 租户隔离：任何查询和操作不得跨 tenant 返回或修改数据。
- 接口不存在：页面必须展示待接入或真实错误，不能吞掉错误或假装成功。

## 异常场景

- 活动创建失败：展示字段级或业务级错误，不保留“已成功”的假状态。
- 规则配置非法：非法 phase、非法 type、非法 field path、非法 op、value 类型不匹配。
- 奖励配置非法：金额格式错误、概率总和非法、阶梯重叠、缺少 `currency`、缺少 `targetRef`。
- 发奖方式非法：选择 `MQ`、`MANUAL_EXPORT`、`MANUAL` 等当前未实现方式时必须阻止提交或标注不可执行。
- 模拟运行失败：标准事件字段缺失、事件时间非法、subject / biz 非法、Fact Query 失败或后端返回风险原因。
- Quota 超限：页面展示 `409` 或 `QUOTA_EXCEEDED`，并说明不会生成发奖单和 Outbox。
- 审批 / 上线 / 编辑发布入口被点击：不得执行成功路径；应显示禁用、待接入或真实错误。
- DLQ 操作失败：非 DLQ 状态、缺 operator / reason、跨 tenant 或已关闭消息，必须展示失败原因。
- 对账无结果：明确提示未找到，不应显示空白链路为成功。

## 非目标范围

- 不实现审批流后端接口。
- 不实现上线发布后端接口。
- 不实现活动编辑、复制、删除、暂停、恢复、结束、版本回滚后端接口。
- 不建设公网生产 Admin 鉴权、完整 RBAC、SSO 或权限矩阵。
- 不实现业务系统能力：用户注册、支付、充值提现、钱包加钱、游戏结算、会员等级计算、真实风控判定、短信推送。
- 不新增业务专用活动页面或业务专用 controller。
- 不在页面内直接发奖、直接扣钱、直接修改用户余额。
- 不把外部业务字段提升为 ExMarketing 顶层模型。
- 不在本需求内定义 MQ Command Out、MANUAL_EXPORT、MANUAL 发奖完整状态机。
- 不用前端页面替代后端 Schema 校验、规则执行、奖励计算、quota 原子扣减和 outbox 投递逻辑。
- 不做移动端运营后台；默认面向桌面浏览器的本地 / 内网管理操作。

## 风险

- API 覆盖风险：部分页面能力依赖当前后端是否已有对应 API；若不存在，必须禁用并展示待接入，不能自行假设接口。
- 范围扩张风险：审批、上线、编辑发布是上游目标能力，但 Owner 已明确延期；Dev / Test 不能把这些作为本期可执行成功路径。
- 生产安全风险：当前 `/admin/` 页面文档定位为本地开发，不应公网暴露；生产使用前必须补齐 Admin 鉴权或内网保护。
- 能力错配风险：如果页面开放 `MQ`、`MANUAL_EXPORT`、`MANUAL`、`DAILY_LIMIT`、未实现 reward type 或 rule type，会被后端拒绝或误导运营。
- 风控误解风险：页面只能配置规则和展示风险预览，不能让运营误以为系统已完成真实风控判定。
- 文档不一致风险：`docs/api.md` 已列出更多审计 action 和过滤条件，`docs/data-model.md` 的 action 列表较旧；页面验收以最新 API 文档和后端真实返回为准。

## 验收标准

1. 管理后台页面能打开，并能加载活动列表；失败时展示真实错误原因。
2. 活动后台不是按活动类型硬编码页面；同一套活动、规则、奖励、quota 表单至少能表达固定奖励、比例奖励、阶梯奖励三类普通活动。
3. 活动列表展示租户、活动编码、活动名称、类型、状态、触发事件、时间、时区、优先级、互斥组、叠加、版本和更新时间。
4. 新增活动页能通过现有 `POST /admin/v1/activities` 提交文档已支持的活动字段、`CONDITION` 规则、已支持奖励配置，以及 `AUTO + HTTP/WEBHOOK` 发奖方式。
5. 新增活动成功时展示后端返回结果；失败时展示后端错误码或原因，不显示假成功。
6. 页面阻止或明确标注当前不支持的 `MQ`、`MANUAL_EXPORT`、`MANUAL`、未实现 reward type、未实现 rule type、`DAILY_LIMIT` 执行语义。
7. 活动详情只读展示后端可获得的基础配置、规则、奖励、quota、互斥叠加、插件和发奖方式；缺少详情接口时展示待接入或列表摘要能力边界。
8. Quota 页面能创建或展示 `BUDGET`、`STOCK`、`USER_LIMIT`，并正确表达缺失 quota = `unlimited`。
9. 风控 / 限制区块能配置或展示 `RISK`、`LIMIT`、IP / Device / UserLimit / Quota 相关条件和模拟风险结果，不宣称真实风控判定。
10. 模拟运行页能输入标准事件，展示 matched / unmatched、规则原因、奖励预览、quota 影响、participation / grant_order / outbox 预览，并明确模拟无副作用。
11. 审计页面能按文档支持的过滤条件查看记录，并展示 actor、action、resource、resourceId、payload 或 snapshot、createdAt。
12. 对账页面能按活动、用户、事件、发奖单或幂等键展示 event inbox -> participation -> grant_order -> outbox -> callback -> quota / audit 时间线；无结果时明确提示未找到。
13. DLQ 运维页支持列表、重放、关闭，并校验 operator / reason；`DLQ_CLOSED` 不允许 replay。
14. Agent 工作台 / git diff 相关页面只展示本工程已有能力，不扩展为生产发布或强制提交流程。
15. 审批、上线、活动编辑发布、复制、删除、暂停、恢复、结束、版本回滚入口必须禁用或标注待接入，不能显示为已成功执行。
16. 页面不得产生真实发奖副作用；真实发奖只能由后端标准流程创建 grant_order / outbox 并由 worker 投递。

## 待确认问题

以下问题不阻塞本期进入 Review / Dev，但会影响后续任务拆分：

1. 活动详情是否已有稳定详情 API；若没有，本期是否接受基于活动列表返回内容做只读摘要并标注详情待接入？
2. Quota 是否已有独立查询 API；若没有，本期是否接受仅展示活动详情 / 创建响应 / 模拟结果中能获得的 quota 信息，并将完整 quota 列表标注待接入？
3. Admin operator 来源本期是否统一使用页面输入的 `operator`，还是沿用后端默认 actor；生产化前需要单独定义。
4. 页面是否需要分页、排序、导入 / 导出、CSV 下载和可访问性验收；当前任务未将其定义为本期硬性范围。
5. `docs/api.md` 与 `docs/data-model.md` 关于审计 action 的口径是否需要后续统一；本期页面字段验收以最新 API 文档和真实返回为准。

## 评分

```text
需求清晰度        18 / 20
范围可控          18 / 20
风险可控          17 / 20
验收标准明确      18 / 20
架构一致性        19 / 20
总分              90 / 100
```

扣分原因：本期范围已由 Owner 明确收敛，审批 / 上线 / 编辑发布缺口不再阻塞；仍保留少量风险在于活动详情、quota 展示等页面能力可能依赖后端实际接口覆盖程度，需通过禁用、待接入和真实错误展示控制。

## Decision

```text
PASS
```

Decision PASS 原因：Owner 已明确本期仅交付现有后端 API 支持或本工程明确存在的本地 / 内网管理页面，并将审批流、上线发布、活动编辑发布、复制、删除、暂停、恢复、结束、版本回滚排除出本期。需求边界、业务规则、异常场景和验收标准已明确到可审核、可验收；待确认问题不阻塞本期开发，但不得在本期用前端 mock 或实现假设补齐未接入能力。
