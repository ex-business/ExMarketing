# Analysis

## Agent 指令

你是 Analysis Agent，只负责需求分析和需求细化。
请读取本目录 task.md、PRD/设计文档、业务规则文档、已有 API/数据字典说明。
不需要读取代码，不允许用代码实现细节替代需求澄清。

只写 analysis.md，不写代码，不改代码，不提交，不做实现，不设计具体代码改法。
你的目标是把需求细节一定一定搞清楚：背景、目标、参与方、输入输出、业务规则、边界条件、非目标范围、风险和验收方式。

## 评分规则

满分 100，低于 80 不允许进入研发：

~~~text
需求清晰度        20
业务规则完整      20
边界异常清楚      20
验收标准明确      20
约束一致性        20
~~~

## Review 反馈处理

本轮发现 `review.md` 存在模板占位：

- `P0:`、`P1:`、`P2:` 为空：未识别具体打回项；本次不按具体缺陷返工，但补齐完整需求、规则、异常和验收口径，供 Review Agent 重新审查。
- `Open Questions 待填写`：视为 Review 反馈未完成；本次在“待确认问题”中只保留非阻塞问题，并给出默认最小可验收口径。
- `Decision PASS / BLOCK` 占位：视为上一轮未形成可审核结论；本次明确 Score 和 Decision。

## Analysis 自检修正

上一轮 `analysis.md` 是模板，存在以下自检问题：

- Score 为 `0`：已重写为完整需求说明，并按评分规则重新评分。
- Decision 为 `PASS / BLOCK` 占位：已给出明确 `PASS`。
- 背景、目标、细化需求、业务规则、输入输出、异常场景、边界条件、非目标、风险、验收方式均为“待填写”：本次全部补齐。
- 缺少对 API/数据字典/业务规则文档的约束继承：本次基于 `task.md`、`README.md`、`docs/api.md`、`docs/data-model.md`、`docs/business-activity-templates.md`、`docs/activityplugin-lifecycle.md`、`docs/workflow.md`、`docs/architecture.md`、`docs/operations.md` 细化。
- 未关闭“字段、URL、错误码、配置来源或协议细节缺失”的潜在 BLOCK：本次采用最小可验收口径，优先复用既有 Admin Activity、审批和模拟 API；文档未明确的页面查询字段、复制行为和版本历史展示以只读、安全、可回退为默认约束。

## 背景和已知信息

ExMarketing 是独立活动营销中台，核心边界是接收标准事件、判断活动配置、生成参与记录、发奖单和发奖指令，不直接连接外部业务库、不直接改余额。

当前 `/admin/` 已有本地管理入口，主要覆盖 Agent 工作台、基础活动操作和模拟运行。运营缺少一套完整的通用活动配置页面，导致活动基础信息、展示入口、互斥叠加、版本查看、复制和上线前操作路径不完整。

已有文档约束：

- Admin 页面当前面向本地开发，不应直接暴露公网生产环境。
- 活动主表字段包括 `tenant_id`、`activity_code`、`activity_name`、`activity_type`、`status`、`trigger_events`、`start_time`、`end_time`、`timezone`、`priority`、`plugin_code`、`exclusive_group`、`stackable`、`version`、`config_json`。
- 同一租户下 `tenantId + activityCode` 唯一，不同租户可复用 `activityCode`。
- 活动状态为 `DRAFT / SCHEDULED / ACTIVE / PAUSED / ENDED`。
- 审批状态为 `NONE / PENDING_L1 / PENDING_L2 / APPROVED / REJECTED / WITHDRAWN`，审批只对锁定版本有效。
- 未审批、审批未完成、版本不一致或被驳回的活动不能进入 `SCHEDULED / ACTIVE`；正式事件处理只匹配 `ACTIVE` 活动。
- `config` / `configJson` / `config_json` 顶层只允许通用扩展字段 `entry`、`display`、`audience`、`metadata`。
- 模拟运行使用 `POST /admin/v1/simulate`，不写入 inbox、participation、grant_order、outbox、quota 等业务状态。

## 目标理解

本需求要交付“通用活动配置完整页面”，让运营可以通过同一套后台页面完成普通配置化活动的创建、编辑、查看、复制、列表筛选、版本查看，并从保存后的活动进入模拟运行和上线前校验/审批路径。

目标不是建设单个业务活动的专用配置器，也不是替代后端权威校验。前端负责降低误填概率、展示字段级和全局错误、串联运营路径；后端仍是保存、版本、审批、模拟和校验结果的最终权威。

## 参与方

- 运营人员：配置、复制、查看活动，发起模拟和上线前检查。
- 运营负责人/审批人：查看配置快照、版本和审批状态，执行审批链路中的判断。
- 开发/测试人员：通过本地 Admin 页面验证活动配置、模拟运行和错误展示。
- ExMarketing 后端：提供活动配置、审批、模拟、审计/版本信息和校验错误。
- 外部业务系统：只作为活动事件、Fact Query 和奖励发放系统的集成方，本需求不直接操作外部系统。

## 细化需求

### 页面范围

1. 活动列表页
   - 展示活动基础列：活动 ID、tenant、activityCode、activityName、activityType、status、start/end、timezone、priority、exclusiveGroup、stackable、version、updatedAt。
   - 支持搜索：按 `tenantId`、`activityCode`、`activityName` 做精确或包含式查询；最小口径为前端提交查询条件，后端不支持时允许本地内存列表过滤当前返回结果，但验收环境应能过滤已加载数据。
   - 支持过滤：`status`、`activityType`、时间范围。
   - 支持分页：最小口径为页码/页大小展示；若后端暂未返回总数，页面仍需能对当前结果进行分页展示并避免一次操作丢失筛选条件。
   - 支持操作入口：查看、编辑、复制、进入模拟运行、查看审批/版本。

2. 创建活动页
   - 支持填写基础信息：`tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`。
   - 支持配置展示入口：`config.display`、`config.entry`、`config.audience`、`config.metadata`。
   - 支持渠道、人群入口：最小口径落在 `entry.channels`、`entry.sourceSystems`、`entry.tenantIds`、`entry.audience.subjectTypes`、`entry.audience.segments`、`entry.audience.riskLevels`。
   - 支持普通规则和奖励配置的 JSON 输入或结构化区域。最小可验收口径允许使用 JSON 编辑区承载 `rules`、`rewards` 和可选 `quotas` 操作说明，但页面必须能明确展示后端字段级错误。
   - 新建默认 `status=DRAFT`；若用户选择 `SCHEDULED / ACTIVE`，页面必须提示需要当前版本审批通过，保存失败时展示后端权威错误。

3. 编辑活动页
   - 可编辑字段：`activityName`、`status`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`config` / `configJson` / `config_json`。
   - 身份字段不可编辑或必须只读：`id`、`tenantId`、`activityCode`、`activityType`。
   - 每次保存成功后展示新的 `version` 和 `updatedAt`；版本变化后旧审批不再视为当前版本有效。
   - 保存前进行前端提示：包括非法时间、空必填、timezone 基础格式、`entry.tenantIds` 不能扩大租户边界、明显 JSON 语法错误。
   - 保存后以后端校验为准；后端返回的字段错误必须挂到对应字段，全局 BLOCK 显示在页面顶部。

4. 活动详情页
   - 展示当前配置快照：基础信息、入口展示配置、规则、奖励、quota 摘要、版本、更新时间。
   - 展示审批状态快照：`NONE / PENDING_L1 / PENDING_L2 / APPROVED / REJECTED / WITHDRAWN`，并标明审批锁定版本是否等于当前活动版本。
   - 展示最近模拟结果入口和摘要。若后端没有持久化模拟历史，最小口径为展示本次页面会话内最近一次模拟结果，并提供重新模拟入口。
   - 展示版本历史。若后端已有审计/审批记录可提供历史，则按记录展示；若当前 API 只能提供当前版本，最小口径为展示当前版本、最近更新时间、审批记录，并明确“历史配置快照不可用”。该口径不阻塞创建/编辑/模拟闭环。

5. 复制活动
   - 从详情或列表复制一个现有活动作为新草稿。
   - 默认复制基础配置、展示入口、规则、奖励、互斥叠加、时间和 timezone。
   - 必须要求用户输入新的 `activityCode`；默认 `activityName` 可加“Copy”或由用户修改。
   - 复制后的活动默认 `status=DRAFT`，`version=1`，审批状态按新活动从 `NONE` 开始。
   - 不复制运行态数据：participation、grant_order、outbox、quota 使用量、模拟历史、审批状态、审计历史、插件运行状态。
   - Quota 配置是否复制：最小口径为不自动复制已用/冻结量；如页面提供 quota 重建入口，只允许创建新的额度总量，不能复制旧活动的 `used_value` / `frozen_value`。

6. 模拟和上线前路径
   - 活动保存成功后，页面提供“进入模拟运行”入口，带入当前活动的 tenant、activityCode/activityId 作为上下文提示。
   - 模拟仍由 `POST /admin/v1/simulate` 执行，输入标准事件 payload，不写业务状态。
   - 上线前校验/审批路径复用活动审批 API：提交审批、审批通过/驳回、查看审批记录。若当前页面只负责跳转到既有模拟/审批页，必须保留上下文并能返回活动详情。

## 业务规则

### 活动基础规则

- `tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`startTime`、`endTime`、`timezone` 为创建普通活动的核心必填项。
- `activityCode` 在同一 `tenantId` 下唯一；重复创建返回 `409 Conflict` 并展示为 `activityCode` 字段错误或全局“同租户活动编码重复”。
- `startTime` / `endTime` 必须是带时区偏移的 RFC3339 时间，且 `startTime < endTime`。
- `timezone` 必须是可识别 IANA 时区名或 `UTC`。
- `priority` 越大越优先匹配；页面不得把高优先级解释为排序越小越靠前。
- `exclusiveGroup` 表示互斥组；同组默认只命中一个。
- `stackable=false` 表示不允许叠加命中；`stackable=true` 表示允许与其他活动叠加，但仍受互斥组、规则、quota 和后端匹配规则约束。

### 展示与入口规则

- `display.visible=false` 只表示不在展示入口可见，不影响内部事件处理或模拟运行。
- `entry.enabled=false` 会在 MATCH 阶段拒绝活动候选，原因码为 `ENTRY_DISABLED`。
- `entry.channels` 读取标准事件 `attributes.channel`；非空且不命中时拒绝，原因码为 `ENTRY_CHANNEL_NOT_ALLOWED`。
- `entry.sourceSystems` 读取标准事件 `sourceSystem`；非空且不命中时拒绝，原因码为 `ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`。
- `entry.tenantIds` 只能为空或包含活动自身 `tenantId`，不能扩大租户边界；运行时非空且不命中返回 `ENTRY_TENANT_NOT_ALLOWED`。
- `entry.audience.subjectTypes` 读取 `subject.type`；非空且不命中返回 `AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED`。
- `entry.audience.segments` 读取 `attributes.segments` 或 `attributes.segment`；非空且无交集返回 `AUDIENCE_SEGMENT_NOT_ALLOWED`。
- `entry.audience.riskLevels` 读取 `attributes.riskLevel`；非空且不命中返回 `AUDIENCE_RISK_LEVEL_NOT_ALLOWED`。
- 兼容旧字段 `entry.entranceCodes`、`entry.landingUrl`、顶层 `audience.segmentCodes/tags/countries` 的读回展示；新入口匹配解释只以通用 `entry.*` 和标准事件字段为准。

### 配置 JSON 规则

- `config` 顶层只允许 `entry`、`display`、`audience`、`metadata`；未知顶层字段应在保存时返回 `400 Bad Request` 并展示明确错误。
- `metadata` 仅存放集成提示，如 `sourceSystem`、`bizType`、周期元数据，不承载 secret、token、账号密码或业务系统连接信息。
- `rules` 和 `rewards` 必须遵守后端已有规则、奖励和 Decimal 约束；前端可以做基础格式校验，但不得放宽后端校验。
- 普通活动不需要 `ActivityPlugin`；若配置 `pluginCode`，创建和上线前校验必须受插件注册、启用状态、activityType 兼容性和 capability 约束。

### 状态、审批和版本规则

- 新建和复制默认进入 `DRAFT`，避免误上线。
- 更新成功必须递增活动 `version`；页面要提示“审批只对当前版本有效”。
- 未审批、审批未完成、审批版本不一致或审批被驳回时，不能保存为 `SCHEDULED / ACTIVE`。
- 普通活动一级审批通过后可上线；高危活动需要 L2 审批，且提交人不能审批自己，二级审批人不能与一级审批人相同。
- `SCHEDULED` 不会处理正式事件并生成 participation、grant_order 或 outbox；正式事件只匹配审批通过、版本一致且 `status=ACTIVE` 的活动。

### 错误展示规则

- 字段级错误优先挂到字段旁，例如时间、timezone、activityCode 重复、JSON 语法、非法 config 顶层字段。
- 全局 BLOCK 展示在页面顶部，例如审批未通过、版本不一致、后端校验失败、模拟运行失败、网络错误。
- 后端校验结果和错误码是权威来源，前端提示不得把失败保存为成功。
- 页面刷新后必须从后端重新读取已保存配置，不得只依赖浏览器本地状态。

## 输入输出

### 创建活动输入

最小输入：

~~~json
{
  "tenantId": "site_001",
  "activityCode": "FIRST_DEPOSIT_BONUS",
  "activityName": "First Deposit Bonus",
  "activityType": "FIRST_DEPOSIT",
  "status": "DRAFT",
  "triggerEvents": ["DEPOSIT_SUCCESS"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-06-01T00:00:00Z",
  "timezone": "UTC",
  "priority": 100,
  "exclusiveGroup": "deposit_bonus",
  "stackable": false,
  "config": {
    "display": {"visible": true, "title": "首存赠送"},
    "entry": {"enabled": true, "channels": ["APP", "WEB"]},
    "metadata": {"sourceSystem": "payment", "bizType": "DEPOSIT"}
  },
  "rules": [],
  "rewards": []
}
~~~

输出：

- 成功：返回活动详情，至少包含 `id`、基础字段、`config`、`version`、`createdAt`、`updatedAt`。
- 失败：返回 HTTP 状态和错误信息；页面映射为字段错误或全局错误。

### 列表查询输入输出

输入筛选项：

- `tenantId`、`activityCode`、`activityName` 搜索。
- `status` 多选或单选。
- `activityType` 过滤。
- `startTime/endTime` 时间范围过滤。
- `page/pageSize` 分页。

输出最小展示：

- `items` 活动列表。
- 若 API 提供分页元数据则展示总数；若只返回 `items`，页面至少完成当前结果分页和空态展示。

### 更新活动输入输出

输入只允许更新：

- `activityName`
- `status`
- `startTime`
- `endTime`
- `timezone`
- `priority`
- `exclusiveGroup`
- `stackable`
- `config` / `configJson` / `config_json`

输出：

- 成功后返回更新后的活动详情，`version` 必须变化。
- 请求中包含 `id`、`tenantId`、`activityCode`、`activityType` 等身份字段时，必须失败并提示字段不可更新。

### 详情/版本输出

详情页输出应包含：

- 当前配置快照。
- 当前版本和更新时间。
- 审批状态和审批锁定版本。
- 审批记录。
- 最近模拟结果摘要或“暂无模拟结果”。
- 版本历史。最小可验收口径允许只展示当前版本和审批/审计记录，但页面必须不误称具备完整历史快照能力。

### 模拟输入输出

输入为标准事件 payload：

~~~json
{
  "eventId": "evt_sim_001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_sim_001"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {"amount": "299.00", "currency": "CNY"}
}
~~~

输出展示：

- `matchedActivities` / `unmatchedActivities`
- 每个活动的 `decision`、`decisionReasonCode`、`decisionReason`
- 规则结果、奖励预览、quota 影响预览、participation 预览、grant order 预览、outbox command 预览
- 若 `wouldExceed=true`，展示 quota 风险，不展示为可执行发奖结果

## 异常场景

- 创建时同租户 `activityCode` 重复：页面展示重复编码错误，不清空用户已填表单。
- `startTime >= endTime`：保存前提示，后端拒绝时展示字段错误。
- 时间不是带时区偏移的 RFC3339：字段错误。
- `timezone` 非 IANA/UTC：字段错误。
- `config` 顶层出现未知字段：字段或 JSON 区域错误。
- `entry.tenantIds` 不为空且不包含活动自身 `tenantId`，或包含其他租户导致扩大边界：保存前提示，后端拒绝时展示全局/字段错误。
- 保存为 `SCHEDULED / ACTIVE` 但审批状态不满足：展示审批 BLOCK，不把活动标记为已上线。
- 编辑请求误带身份字段：展示“身份字段不可修改”。
- 后端返回 `404 Not Found`：详情/编辑页展示资源不存在并提供返回列表。
- 后端返回 `409 Conflict`：按冲突类型展示重复编码、版本/审批冲突或 quota/状态冲突。
- 网络失败、服务未启动、JSON 响应不可解析：展示全局错误，保留表单草稿。
- 模拟运行失败：保留输入事件 payload，展示错误，不写入“最近模拟成功”。
- 模拟命中 quota 风险：展示为风险预览，不等同于保存失败。
- 页面刷新：已保存数据必须可从后端重新加载；未保存草稿是否恢复不作为验收要求。

## 边界条件

- 空列表、无筛选结果、接口返回空 `items` 都必须有清晰空态。
- `rules` 或 `rewards` 为空时，页面允许保存草稿，但上线前校验/模拟是否通过由后端权威判断。
- 不同租户允许相同 `activityCode`；列表和复制必须显式展示 tenant，避免误判重复。
- `display.visible=false` 不影响模拟入口可用。
- `entry.enabled=false` 的活动仍可保存和查看，但模拟应能解释 MATCH 拒绝原因。
- `SCHEDULED` 活动可展示和编辑，但不得被页面描述为已处理正式事件。
- 高危金额模板可配置和模拟，但生产就绪依赖预算、用户周期 quota、L2 审批、审计、回调和对账；本页面不绕过这些要求。
- 插件活动只在已配置 `pluginCode` 时展示插件相关摘要；普通活动不要求插件。
- Admin 无鉴权是既有非目标，页面文案和文档必须保持“本地/内网开发管理入口”边界，不得暗示公网生产可直接暴露。

## 非目标范围

- 不做 Admin 鉴权、权限模型、角色管理或审批人账号系统。
- 不做每个业务活动的专用配置页面，例如首存、VIP、红包雨、抽奖的专属表单。
- 不做复杂拖拽营销装修、可视化页面搭建或素材管理系统。
- 不直接连接外部业务数据库，不读取外部用户余额，不直接发奖。
- 不新增外部事件接入协议；模拟仍使用标准事件 payload。
- 不改变规则引擎、奖励计算、quota、outbox、callback 的核心业务语义。
- 不保证完整历史配置快照能力，除非后端已有或本任务明确补齐 API；最小验收为当前版本、审批记录和审计可见。
- 不要求复制活动时复制运行态数据、已用额度、冻结额度、审批状态或模拟历史。

## 风险点

- 前端表单与后端 schema 演进不一致，可能导致保存失败或字段丢失。缓解：未知字段交给后端权威拒绝，页面展示原始错误并保留输入。
- 版本和审批状态误解可能导致运营以为旧审批可上线新版本。缓解：详情和编辑页明确显示当前版本、审批锁定版本和版本不一致提示。
- 复制活动若误复制 quota 使用量或审批状态，会造成预算/审批风险。缓解：复制只创建新草稿，不复制运行态和审批态。
- 当前 API 文档对列表分页、版本历史和最近模拟结果持久化描述不足。默认最小口径是不阻塞配置闭环：当前结果分页、当前版本展示、会话内最近模拟摘要；完整持久历史作为后续增强。
- Admin 当前不面向公网生产，若被部署到公网会有安全风险。缓解：本需求不做鉴权，但文档和页面边界需保持本地/内网管理定位。

## 验收方式

### 功能验收

- 能从活动列表创建一个普通配置化活动，保存成功后详情页展示完整已保存配置和版本。
- 能编辑活动名称、状态、时间、timezone、priority、exclusiveGroup、stackable 和 `display/entry/audience/metadata`，保存成功后版本递增。
- 能查看活动详情，包括配置快照、审批状态、当前版本、最近模拟结果入口/摘要。
- 能复制已有活动为新的 `DRAFT` 活动，要求新 `activityCode`，不复制审批状态、运行态数据、已用/冻结 quota。
- 能按状态、类型、时间和关键词进行列表筛选，并能处理空结果。
- 保存后的活动可进入模拟运行页面或区域，并用标准事件 payload 发起模拟。

### 错误验收

- 非法时间、`startTime >= endTime`、非法 timezone 能展示明确字段错误。
- 同租户重复 `activityCode` 能展示重复错误；不同租户同编码不应被页面误拦截。
- `config` 顶层未知字段、`entry.tenantIds` 扩大租户边界、互斥/叠加配置明显错误能展示明确错误。
- 后端全局 BLOCK，例如审批未完成、版本不一致、后端校验失败，能在页面顶部展示。
- 页面刷新后能重新读取已保存活动，不丢失已保存配置。

### 流程验收

- 创建或编辑保存后，可直接跳转或进入模拟运行，并保留活动上下文。
- 审批状态和审批记录可从详情页查看；若当前版本未审批，页面不能暗示可上线。
- 模拟结果明确标注为预览，不代表 quota 已锁定、奖励已发放或 outbox 已投递。

### 测试验收

- `go test ./...` 通过。
- `go build ./...` 通过。
- 页面级 smoke 通过：列表加载、创建保存、编辑保存、详情查看、复制、非法输入错误展示、模拟入口可用。
- 如涉及 PostgreSQL 行为或 API 适配，需在 Docker PostgreSQL 环境验证保存、刷新读取、重复编码和版本递增。

## 待确认问题

以下问题不阻塞开发，均已有默认最小可验收口径：

- 列表分页是否必须由后端返回总数？默认：后端无总数时页面对当前返回结果分页；不阻塞。
- 版本历史是否必须展示每个历史版本的完整配置快照？默认：至少展示当前版本、审批记录和审计记录；完整历史快照作为增强，不阻塞。
- 最近模拟结果是否必须后端持久化？默认：可展示当前页面会话内最近一次模拟摘要；模拟本身仍由后端执行，不阻塞。
- 规则和奖励是否必须做全结构化表单？默认：最小验收允许 JSON 编辑区加基础校验和后端错误映射；未来可增强为结构化配置器。
- Quota 是否纳入本页面创建流程？默认：本页面展示/提示 quota 摘要和风险，创建 quota 可复用已有 `POST /admin/v1/quotas` 或保留为后续页面；不得复制旧活动使用量。

## Score

~~~text
90
~~~

扣分说明：

- 需求清晰度 18/20：页面范围和流程清楚，但版本历史和模拟历史受现有 API 明确度限制。
- 业务规则完整 18/20：继承活动、入口、审批、模拟规则，quota 配置页面深度采用最小口径。
- 边界异常清楚 18/20：覆盖主要字段、状态、网络、审批、复制异常，具体后端错误响应结构仍由实现阶段适配。
- 验收标准明确 18/20：功能、错误、流程、测试均可验收，页面视觉细节不在本分析展开。
- 约束一致性 18/20：符合现有 API/数据模型/工作流，不引入鉴权和专用活动页面。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-admin-activity-config-pages
