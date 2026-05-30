# Analysis

## Review 反馈处理

`review.md` 当前仍为模板态，未给出具体 P0 / P1 / P2 内容；但文件中存在 `PASS / BLOCK` 占位、`Open Questions` 占位和 P 级占位。按返工规则逐项处理如下：

| Review 占位/潜在意见 | 处理结果 |
|---|---|
| P0 / P1 / P2 为空 | 未发现具体打回项；本轮补齐完整需求细化，便于后续 Review 按真实内容审核。 |
| Open Questions 为“待填写” | 本轮列出待确认问题，并为每个问题给出默认最小可验收口径，不因 Owner 未决而 BLOCK。 |
| Decision 为 `PASS / BLOCK` 模板 | 本轮基于现有任务、`docs/api.md`、历史模拟运行/审批/上线前校验分析和 backlog 材料形成可审核需求，结论改为 PASS。 |

## Analysis 自检修正

上一轮 `analysis.md` 是模板态，存在 Score `0`、Decision `PASS / BLOCK` 未决，以及背景、目标、细化需求、业务规则、输入输出、异常、边界、非目标、风险、验收和待确认问题全部“待填写”。本轮逐项修正：

| 旧问题 | 修正方式 |
|---|---|
| 背景和已知信息缺失 | 补充 ExMarketing 管理后台、模拟运行、上线前校验、审批流、版本锁定和审计上下文。 |
| 目标理解缺失 | 明确本任务交付模拟运行页面、上线前校验页面和审批页面闭环，不做真实发奖、不做 Admin 鉴权。 |
| 细化需求缺失 | 按页面、用户流程、数据展示、操作动作和页面状态补齐。 |
| 业务规则缺失 | 汇总模拟无副作用、MATCH/QUALIFY/REWARD/RISK/LIMIT 解释、校验 PASS/WARN/BLOCK/APPROVAL_REQUIRED、普通/高危审批、版本锁定、审计规则。 |
| 输入输出缺失 | 明确标准事件输入、样例选择输入、模拟输出、校验输出、审批动作输入和审批记录输出。 |
| 异常和边界缺失 | 补齐非法 JSON、字段缺失、Fact Query 失败、quota 超限、BLOCK 配置上线、高危未二审、同人审批、版本不一致、并发/重复动作等场景。 |
| 非目标范围缺失 | 明确不改真实发奖链路、不做 RBAC/OA、不让前端绕过后端、不做历史活动治理、不扩展业务专用模型。 |
| 验收方式缺失 | 给出可测试页面流程、后端状态约束、审计查询、无副作用和构建测试验收。 |
| Score 0 / Decision 未决 | 根据补齐后的需求重新评分为 91，Decision PASS。 |

## 背景和已知信息

ExMarketing 是独立活动营销中台。活动上线前，运营需要在管理后台确认一条标准事件如果进入正式链路，会命中哪些活动、为何通过或拒绝、预计奖励是什么、quota 会受什么影响、将生成什么 outbox 指令，以及当前活动配置是否满足上线校验和审批要求。

已有材料确认：

- 管理后台入口为 `GET /admin/`，当前面向本地开发，不应直接暴露公网生产环境。
- 模拟运行已有管理端接口 `POST /admin/v1/simulate`，输入标准事件 JSON，输出命中/未命中、规则解释、奖励预览、quota 影响、participation / grant order / outbox 预览和插件解释。
- 模拟运行不得写入 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`；允许写一条脱敏 `SIMULATION_RUN` 审计。
- 审批接口已在文档中定义：`submit`、`approve`、`reject`、`approval-records`。
- 审批状态为 `NONE / PENDING_L1 / PENDING_L2 / APPROVED / REJECTED / WITHDRAWN`。
- 普通活动一级审批通过后可上线；高危活动一级通过后进入 `PENDING_L2`，必须由不同二级审批人通过后才可上线。
- 活动配置版本变化后旧审批失效；正式事件只匹配审批通过、版本一致且 `status=ACTIVE` 的活动。
- 上线前校验结论为 `PASS / WARN / BLOCK / APPROVAL_REQUIRED`；`BLOCK` 配置不能上线，高危配置必须走审批。

## 目标理解

本需求交付管理后台的“模拟运行与审批页面”闭环，让运营和审批人能在不真实发奖的前提下完成：

1. 输入标准事件 JSON 或选择样例，运行模拟并查看结构化解释。
2. 查看活动命中、规则阶段、奖励预览、quota 影响、outbox 预览和拒绝原因。
3. 对活动当前版本执行上线前校验，看到 `PASS / WARN / BLOCK / APPROVAL_REQUIRED` 的逐项结果。
4. 提交审批、普通审批、高危二级审批、驳回并填写原因。
5. 查看审批历史、操作者、原因、前后快照、版本锁定和高危标记。
6. 在页面上发起 `goLive` 和 `rollback` 操作时，必须受后端校验、审批和版本门禁约束。

本任务的核心是管理后台页面和既有/缺口 API 对接的业务闭环，不是重写规则引擎、审批状态机或发奖链路。

## 参与方

- 运营 / 活动配置人员：选择活动、准备标准事件样例、运行模拟、查看上线前校验结果、提交审批或发起上线/回滚。
- 一级审批人：审核普通活动，或对高危活动完成一级审批；不能审批自己提交的活动。
- 二级审批人：审核高危活动第二级；不能与提交人或一级审批人为同一 `operator`。
- 测试人员：通过页面 smoke 验证模拟、校验、审批、驳回、版本锁定和无副作用。
- 研发 / 排障人员：通过模拟解释、校验项和审计记录定位配置问题。
- ExMarketing 管理端：承载页面、表单、结果展示和操作反馈。
- ExMarketing 后端：提供模拟、校验、审批、审计、版本门禁和状态变更的权威结果。
- 外部发奖系统：不参与模拟运行；页面只展示 outbox 指令预览，不真实投递。

## 细化需求

### 1. 模拟运行页面

- 页面必须提供标准事件 JSON 输入区，字段覆盖 `tenantId`、`eventType`、`sourceSystem`、`subject`、`biz`、`attributes`、`occurredAt`；若输入完整标准事件，也应支持 `eventId`。
- 页面必须提供至少一组可选择样例。最小可验收样例为 `DEPOSIT_SUCCESS` 首存/充值事件，包含金额、币种、是否首存和渠道等常见 `attributes`。
- 样例载入后用户可编辑；编辑后的请求以 JSON 输入区为准。
- 运行模拟调用 `POST /admin/v1/simulate`，不得要求用户填写真实幂等请求头或外部发奖参数。
- 页面必须展示请求校验错误、非法 JSON、字段缺失、时间格式错误和后端业务错误。
- 页面必须明确标识模拟结果是 preview-only，不代表 quota 锁定、奖励发放、插件状态消费或 outbox 投递。

### 2. 模拟结果解释

模拟结果必须按结构化区域展示：

- 汇总：候选活动数、命中数、未命中数、是否有可发奖预览、是否存在 quota 风险。
- 命中活动：活动 ID、tenant、activityCode、activityName、activityType、version、decision、decisionReasonCode、decisionReason。
- 未命中活动：必须展示机器可读原因码和人可读原因，不能只显示“未命中”。
- 阶段解释：按 `MATCH / QUALIFY / REWARD / RISK / LIMIT` 分组展示规则、通过/拒绝/跳过/错误状态、字段、期望值、实际值、原因。
- 奖励预览：rewardCode、rewardType、calculateType、金额或权益 payload、币种、计算明细；金额按 4 位小数展示。
- Quota 影响：预算、库存、用户次数、每日次数等预览占用、`wouldExceed`、`unlimited` 和风险原因。
- Participation / grant order / outbox 预览：展示如果真实执行将生成的关键字段；quota 超限时不得展示可执行 grant order / outbox command 预览。
- Fact Query 解释：展示 provider 查询、缓存、成功、失败、mapping 失败和字段缺失原因；不得显示 secret、token、Authorization 或 provider 明文凭据。
- 插件解释：如存在插件，展示 pluginCode、pluginContext、precheckResult 和 stateChangeIntents；不展示为已消费状态。

活动级 `decision` 和规则级 `status` 只能使用：

```text
PASSED
REJECTED
SKIPPED
ERROR
```

不得在页面或 API 适配层引入 `MATCHED / UNMATCHED / FAILED` 作为机器状态。

### 3. 上线前校验页面

- 页面必须能针对当前活动和当前版本触发或读取上线前校验结果。
- 校验结果必须展示总体结论：`PASS / WARN / BLOCK / APPROVAL_REQUIRED`。
- 校验项必须逐项展示编码、名称、等级、结果、原因、涉及字段、当前值、期望值和建议处理方式。
- 至少覆盖：JSON Schema、规则字段、概率总和、奖励金额预算暴露、库存、活动时间冲突、互斥组冲突、高危配置、审批状态和版本一致性。
- `PASS`：允许继续上线，仍需满足活动状态和时间窗口规则。
- `WARN`：允许继续上线，但页面必须显示警告并保留审计摘要。
- `BLOCK`：不允许提交上线或 goLive，页面必须禁用或阻断上线动作，并展示所有阻断项。
- `APPROVAL_REQUIRED`：允许提交审批，但审批完成前不允许上线；高危原因必须可见。

### 4. 审批页面

- 页面必须展示当前审批状态：`NONE / PENDING_L1 / PENDING_L2 / APPROVED / REJECTED / WITHDRAWN`。
- 页面必须展示活动当前版本、审批锁定版本，以及二者是否一致。
- 提交审批调用 `POST /admin/v1/activities/{id}/approval/submit`，输入 `operator`、`expectedVersion`、`reason`。
- 审批通过调用 `POST /admin/v1/activities/{id}/approval/approve`，输入 `operator`、`reason`。
- 驳回调用 `POST /admin/v1/activities/{id}/approval/reject`，输入 `operator`、`reason`；`reason` 必填。
- 审批历史调用 `GET /admin/v1/activities/{id}/approval-records` 或等价审计查询，展示动作、操作人、原因、审批前后状态、锁定版本、高危标记和时间。
- 普通活动一级审批通过后状态变为 `APPROVED`；高危活动一级审批通过后进入 `PENDING_L2`，二级审批通过后才变为 `APPROVED`。
- 提交人不能审批自己提交的活动；高危二级审批人不能与一级审批人为同一 `operator`。
- 配置版本变化后，页面必须提示旧审批已失效，并要求重新提交审批。

### 5. goLive 与 rollback 页面动作

- `goLive` 是页面上的上线意图动作，必须由后端根据校验结论、审批状态、锁定版本、活动时间和目标状态决定是否允许。
- `BLOCK` 配置、未审批配置、高危未完成二级审批、审批版本不一致或活动已过期时，`goLive` 必须失败并展示明确原因。
- 最小可验收口径：若后端尚无独立 `goLive` API，页面可使用现有活动状态更新或审批门禁能力完成上线，但不得在前端直接改审批状态或绕过后端门禁。
- `rollback` 是页面上的回滚意图动作，默认最小口径为把已上线或排期的活动退回到安全的非发奖状态，例如 `PAUSED` 或 `DRAFT`，并记录操作者和原因；若后端未提供专用 rollback API，页面必须展示不可用原因或复用后端允许的安全状态变更，不能伪造审批记录。
- rollback 不回滚已发生的真实 participation、grant_order、outbox、callback 或 quota 结果；历史发奖对账不属于本任务。

## 输入输出

### 模拟输入

最小标准事件：

```json
{
  "eventId": "evt_sim_001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_sim_001"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {
    "amount": "299.00",
    "currency": "CNY",
    "isFirstDeposit": true,
    "channel": "APP"
  }
}
```

字段规则：

- `occurredAt` 必须是带时区的 RFC3339 时间。
- 业务字段必须放在 `attributes`，不得作为非标准顶层字段。
- `subject` 和 `biz` 必须是对象，且包含可解释的类型和 ID。
- 页面可根据当前活动上下文预填 `tenantId` 和 `eventType`，但用户可修改。

### 模拟输出

- `simulatedAt`
- `summary`
- `matchedActivities`
- `unmatchedActivities`
- 活动级 `decision / decisionReasonCode / decisionReason`
- `ruleResult`
- `factQueryResults`
- `rewardPreviews`
- `quotaImpactPreviews`
- `participationPreview`
- `grantOrderPreviews`
- `outboxCommandPreviews`
- `plugin`

### 校验输入输出

输入：

- 活动 ID
- 当前活动版本
- 目标动作：提交审批、审批通过、goLive、rollback 或只读检查
- 操作人 `operator`
- 操作原因 `reason`

输出：

- 总体结论：`PASS / WARN / BLOCK / APPROVAL_REQUIRED`
- `allowPublish`
- `requiresApproval`
- `requiresL2Approval`
- 校验项列表
- 高危配置列表
- 当前审批状态和锁定版本
- 可执行下一步动作列表

### 审批输入输出

提交审批：

```json
{
  "operator": "ops",
  "expectedVersion": 1,
  "reason": "ready"
}
```

审批或驳回：

```json
{
  "operator": "lead",
  "reason": "approved"
}
```

审批记录输出至少包含：

- 动作：submit / approve / reject / goLive / rollback / blocked
- 操作人
- 原因
- 审批前状态
- 审批后状态
- 锁定版本
- 当前活动版本
- 是否高危
- 前后快照或快照摘要
- 操作时间

## 业务规则

### 模拟运行规则

- 模拟运行必须复用正式链路的活动候选、规则解释、奖励计算、quota 快照、Fact Query 只读能力和插件解释口径。
- 模拟运行不得写入 inbox、participation、grant_order、outbox、quota；不得触发 HTTP/Webhook/MQ 发奖。
- 允许写脱敏 `SIMULATION_RUN` 审计，但不能记录 secret、token、完整 Authorization、Fact Provider 明文密钥、完整敏感 attributes 或完整 provider 响应。
- Quota 缺失行按 `unlimited=true` 展示；quota 预览超限时活动应为 `REJECTED` 并给出 `QUOTA_EXCEEDED` 类原因。
- 规则引用 `facts.*` 时，模拟必须使用正式链路同等 provider registry、mapping 和安全边界；provider 失败为 `FACT_QUERY_FAILED`，mapping 失败为 `FACT_MAPPING_FAILED`，字段缺失为 `FACT_FIELD_MISSING`。
- `display.visible=false` 不影响模拟；`entry.enabled=false` 应在 MATCH 阶段拒绝。

### 上线前校验规则

- 任一 `ERROR` 或阻断项存在，总体为 `BLOCK`，不允许上线。
- 无阻断项但存在高危项，总体为 `APPROVAL_REQUIRED`，审批完成前不允许上线。
- 仅存在 `WARN` 时，总体为 `WARN`，允许继续但必须展示警告。
- 全部通过时，总体为 `PASS`。
- 高危默认口径：资金类自动发奖、缺少预算的自动发奖、预算/库存配置、概率/随机/奖池奖励、外部 Fact 依赖、RISK/LIMIT 阶段、插件状态、已上线关键配置变更、发奖目标变更。
- 若需要金额阈值且系统无配置，本轮沿用保守默认：单次最大奖励 `1000.0000`、活动总预算 `100000.0000`；超过即高危，需要二级审批。

### 审批和版本规则

- 未审批活动不能进入 `SCHEDULED / ACTIVE`。
- `BLOCK` 活动不能提交上线；必须先修复配置。
- 高危活动必须完成二级审批后才能上线。
- 审批提交时锁定活动当前版本；审批通过只对锁定版本有效。
- 活动主配置、规则、奖励、quota 或关键上线字段变化后，旧审批失效。
- 驳回必须填写原因；驳回后不能上线，修改后需要重新提交。
- 审批通过不等于正式事件可命中；正式事件仍只匹配审批通过、版本一致且 `status=ACTIVE` 的活动。
- `SCHEDULED` 活动即使审批通过，也不得被正式事件命中或生成 participation、grant_order、outbox、quota 扣减。

### 审计规则

- 模拟运行、提交审批、审批通过、审批驳回、goLive、rollback、上线阻断都必须可审计。
- 审计必须包含操作者、动作、对象、原因、版本、前后状态或快照摘要、时间。
- 页面必须能查询并展示审批历史；模拟历史若后端未持久化，最小口径为展示本页面会话最近一次结果，不作为审计替代。

## 边界条件

- 标准事件 JSON 为空、非法 JSON、缺必填字段或含非标准顶层业务字段：页面阻止提交或展示后端 400 错误。
- 无候选活动：模拟展示空结果和原因，不算页面错误。
- 单个活动配置不可解释：该活动展示 `ERROR`，其他活动结果仍可展示。
- Fact Query 失败只影响依赖该 fact 的活动或规则，不应导致整页不可用。
- Quota 超限：展示风险和拒绝原因，不展示可执行 grant/outbox 预览。
- `BLOCK` 校验项多项同时存在：页面必须展示所有返回项，不能只显示第一项。
- 重复提交审批：不得创建多个并行审批流程；页面展示当前进行中状态。
- 审批人等于提交人：审批失败并展示原因。
- 高危二级审批人与一级审批人相同：审批失败并展示原因。
- `expectedVersion` 与当前版本不一致：提交、审批或 goLive 失败并提示重新加载/重新提交。
- 审批已通过但当前版本变化：goLive 失败，要求重新审批。
- 已驳回审批：不能 goLive；可修改配置后重新提交。
- rollback 后再次 goLive：必须重新满足当前版本校验和审批门禁。
- 网络错误或接口返回非 JSON：页面展示全局错误，不得把操作标记为成功。
- 页面刷新后必须从后端重新读取当前活动、审批记录和校验状态，不能只依赖本地缓存。

## 异常场景

- `POST /admin/v1/simulate` 返回 400：页面定位到 JSON 或字段错误。
- 模拟时后端返回 `FACT_QUERY_FAILED / FACT_MAPPING_FAILED / FACT_FIELD_MISSING`：页面在对应活动/规则区域展示，不隐藏为通用失败。
- 模拟返回 `ERROR` 活动：页面保留原始原因码和人可读原因，便于排障。
- 提交审批时活动不存在：页面展示 not found，不创建本地假状态。
- 提交审批时校验为 `BLOCK`：页面展示阻断项，不进入审批中。
- approve/reject 接口失败：页面保持原状态并提示刷新后重试。
- 审批记录查询失败：审批动作区可展示当前状态，但历史区必须明确加载失败。
- goLive 被后端拒绝：页面展示拒绝原因和需要修复的校验项或审批项。
- rollback 被后端拒绝或无可用 API：页面不得伪造成功，必须展示不可执行原因。

## 非目标范围

- 不做真实发奖，不触发 outbox worker，不调用外部 HTTP/Webhook/MQ 发奖。
- 不实现 Admin 鉴权、RBAC、审批人组织架构、审批通知、OA/Jira/Slack/企业微信集成。
- 不允许前端直接修改审批状态、伪造审批记录或绕过后端门禁。
- 不重构活动匹配、规则引擎、奖励计算、quota 扣减、outbox、callback、DLQ 或 Fact Provider。
- 不新增业务专用活动模型；所有事件业务字段仍只进入 `attributes`。
- 不治理历史已上线活动是否补审批；本轮默认只约束当前页面后续操作和后端当前门禁。
- 不保证模拟结果等同未来真实执行结果；真实执行仍受实时 quota、活动状态、审批版本、Fact Provider 和幂等状态影响。
- 不回滚已发生的真实发奖、callback、quota 使用或对账结果。

## 风险点

- 页面若把模拟预览误导为真实锁定，运营可能误判预算可用性；必须明确 preview-only。
- 高危判定阈值缺少产品配置，默认阈值可能偏保守；本轮用保守阈值确保可验收，不阻塞审批安全目标。
- goLive / rollback 若后端 API 不完整，页面只能复用现有安全状态变更或展示不可用；不得用前端状态伪造闭环。
- 审批身份当前只有 `operator` 字符串，未来接入真实登录态时需映射账号和权限。
- 模拟结果依赖当前 quota 快照和 Fact Query，只能解释“此刻按当前配置预览”，不能保证上线后一定同样命中。
- 审计脱敏不足会泄露敏感 attributes 或 provider 凭据；页面和接口展示都必须以脱敏结果为准。

## 验收标准

1. 模拟页面可选择样例并编辑标准事件 JSON，提交后展示 `summary`、命中活动、未命中活动、规则阶段、奖励预览、quota 影响、participation / grant order / outbox 预览。
2. 非法 JSON、缺少标准事件字段、非法 `occurredAt` 或非标准顶层业务字段能在页面展示明确错误。
3. 模拟结果中的活动级 `decision` 和规则级 `status` 只使用 `PASSED / REJECTED / SKIPPED / ERROR`。
4. 未命中活动必须展示 `decisionReasonCode` 和 `decisionReason`。
5. Quota 超限时展示 `QUOTA_EXCEEDED` 类原因，不展示可执行 grant order / outbox command 预览。
6. 模拟运行前后不新增或修改 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。
7. 模拟运行不会触发 HTTP/Webhook/MQ 发奖；允许写脱敏 `SIMULATION_RUN` 审计。
8. 上线前校验页面展示 `PASS / WARN / BLOCK / APPROVAL_REQUIRED` 总体结论和逐项校验结果。
9. `BLOCK` 配置不能 goLive，页面展示所有阻断原因。
10. 高危配置展示高危原因，未完成二级审批前不能 goLive。
11. 普通活动提交审批后，一级审批通过即可使锁定版本进入可上线状态。
12. 高危活动一级审批通过后进入 `PENDING_L2`，二级审批通过后才进入 `APPROVED`。
13. 提交人不能审批自己；高危二级审批人不能与一级审批人相同。
14. 驳回必须填写原因；驳回后活动不能上线；审批历史可看到驳回原因。
15. 活动配置版本变化后，旧审批不能继续 goLive，页面提示重新提交审批。
16. 审批记录页面可查询动作、操作者、原因、前后状态、锁定版本、高危标记和时间。
17. goLive 和 rollback 操作必须记录操作者和原因，并受后端状态/版本/审批门禁约束。
18. 页面刷新后重新读取后端状态，不能保留已失效本地审批或校验结果作为成功依据。
19. 页面操作 smoke 测试覆盖：模拟成功、模拟错误、BLOCK 校验、普通审批通过、高危二审、驳回、版本锁定失效、goLive 阻断。
20. 基础验证命令通过：`go test ./...`。

## 待确认问题

| 问题 | 默认最小可验收口径 |
|---|---|
| 是否已有独立 goLive / rollback API？ | 若已有则页面对接；若没有，goLive 复用后端允许的活动状态更新和审批门禁，rollback 只允许退到后端支持的安全非发奖状态或展示不可用，不伪造成功。 |
| 高危金额阈值是否产品已配置？ | 未配置时使用单次最大奖励 `1000.0000`、活动总预算 `100000.0000` 作为保守默认，触发二级审批。 |
| 审批人身份是否接入登录态？ | 本轮不做 Admin 鉴权，使用请求中的非空 `operator` 作为最小身份约束。 |
| 模拟历史是否需要持久化展示？ | 本轮不要求持久化模拟历史；只要求展示当前页面会话最近一次结果和脱敏模拟审计。 |
| rollback 是否恢复到 DRAFT 还是 PAUSED？ | 默认优先使用 `PAUSED` 作为已上线活动的安全回退状态；若后端仅允许 `DRAFT` 或其他状态，以后端状态机为准并展示结果。 |
| 历史 ACTIVE 活动是否补审批？ | 本轮不迁移历史数据；页面后续 goLive 和正式事件门禁按当前后端规则执行。 |

以上问题均已有默认口径，不构成 Decision BLOCK。

## Score

```text
需求清晰度        18/20
业务规则完整      19/20
边界异常清楚      18/20
验收标准明确      18/20
约束一致性        18/20
总分              91/100
```

扣分原因：goLive / rollback 是否已有专用 API、真实 Admin 登录态和高危阈值来源仍依赖后续产品/架构演进；本轮已给出保守默认口径，不阻塞 Review、Dev 和 Test。

## Decision

```text
PASS
```

需求：2026-05-29-admin-simulation-approval-pages
