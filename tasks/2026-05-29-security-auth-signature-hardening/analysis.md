# Analysis

## 需求 ID

2026-05-29-security-auth-signature-hardening

## Review 反馈处理

`tasks/2026-05-29-security-auth-signature-hardening/review.md` 当前仍为模板态：

- Findings 中只有 `P0:`、`P1:`、`P2:` 占位，没有具体问题描述。处理：不视为实质 Review 打回项；本轮无需修复具体 Review Finding。
- Open Questions 为“待填写”。处理：不视为明确业务问题；本轮基于 `task.md`、`docs/api.md`、`docs/operations.md`、`docs/security-threat-model.md` 和相关业务规则补齐默认最小可验收口径。
- Decision 为 `PASS / BLOCK` 模板占位。处理：不视为明确 BLOCK 原因；本轮仍按高风险安全需求重新产出完整分析，供 Review Agent 正式审核。

## Analysis 自检修正

上一轮 `analysis.md` 是空模板，存在以下阻塞项：

- `Score = 0`，低于 80。修正：本轮补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题，并重新评分。
- `Decision = PASS / BLOCK` 模板占位。修正：本轮在业务目标、验收对象和参与方均可由现有材料确定的前提下给出 `Decision PASS`。
- 背景、目标、细化需求、业务规则、输入输出、异常场景、边界条件、非目标范围、风险点、验收方式、待确认问题均为“待填写”。修正：本轮按鉴权签名与租户隔离审计需求完整补齐。
- 旧分析未说明默认口径。修正：本轮采用“生产环境所有外部写入口必须鉴权或签名、所有业务写入必须 tenant guard、所有可重放入口必须 timestamp 加幂等或 nonce 防护、失败必须稳定错误码加脱敏审计”为默认最小可验收口径；Admin 因明确非目标不实现完整登录，默认以边界审计、内网/网关保护要求、操作人/原因/审计和后续鉴权接口预留作为本期可验收范围。

## 背景和目标

ExMarketing 的事件接入、Webhook Receiver、Reward Callback 和 Admin 操作都会影响活动匹配、quota 冻结/确认/释放、grant order、outbox 投递和真实发奖终态。伪造事件、签名绕过、token 跨租户使用、callback 重放、Admin 高危操作越权或跨 tenant 更新，都可能造成未授权发奖、重复发奖、预算不一致或审计不可追溯。

本需求目标是对鉴权、签名、租户隔离和重放防护进行审计与加固需求定义，产出后续 Dev/Test 可执行的安全 hardening 范围：

- OpenAPI Event API：生产环境 Bearer token 必须启用，并绑定允许的 `tenantId` 与 `sourceSystem`。
- Webhook Receiver：必须校验 receiver、enabled、timestamp、HMAC 签名、source allowlist、tenant/source 映射约束和重放窗口。
- Reward Callback：必须按 `tenantId` 参与定位，校验签名或等价 token，绑定 `billNo`/`commandId`/`idempotencyKey` 与 tenant，确保幂等终态和冲突拒绝。
- Admin 操作：本期不实现完整登录/RBAC，但必须完成边界审计，明确生产不得公网匿名暴露，高危写操作必须有 tenant guard、operator、reason、审计，以及后续鉴权接口预留。
- 所有安全失败必须有稳定错误码、不得写业务状态，并写入脱敏审计或可审计的安全事件。

## 参与方

- 外部业务系统：调用 `POST /openapi/v1/events` 上报标准事件，必须使用 Bearer token 和 `Idempotency-Key`。
- Webhook Provider：调用 `POST /openapi/v1/webhooks/{receiverCode}`，必须使用 timestamp 加 HMAC 签名，payload 经配置映射为标准事件。
- 外部发奖系统：调用 `POST /openapi/v1/reward/callback` 回传发奖结果，必须按 tenant 签名并提供发奖单定位字段。
- Admin/Ops 用户：使用 Admin API/UI 执行活动配置、审批、quota、人工发奖、导出、DLQ replay/close、quota timeout release、对账等操作。
- ExMarketing 服务：执行鉴权/签名校验、tenant guard、幂等、业务状态变更、审计和错误响应。
- Review/Test/Release Agent：审核需求、设计负向用例、验证安全失败不写业务状态，并检查错误码与审计 payload。

## 输入

本需求分析使用以下现有材料作为业务事实来源：

- `tasks/2026-05-29-security-auth-signature-hardening/task.md`
- `docs/api.md`
- `docs/operations.md`
- `docs/security-threat-model.md`
- `docs/activityplugin-lifecycle.md` 中关于 plugin state tenant scope 和幂等边界的规则

本阶段不读取代码，不用代码实现现状替代需求澄清。

## 输出

后续实现与测试的最小可验收输出应包括：

- 安全审计报告：列出 OpenAPI、Webhook、Reward Callback、Admin 写操作的鉴权/签名/tenant guard/replay 防护状态、缺口和补救任务。
- 修复或补救任务：对发现的未授权写入口、签名缺失、tenant guard 缺失、重放防护缺失、错误码不稳定、审计缺失等问题给出可执行任务。
- 鉴权/签名测试用例：覆盖未授权、错误 token、跨 tenant token、错误 sourceSystem、签名缺失、签名错误、过期 timestamp、重复 nonce 或重复幂等、跨 tenant locator。
- 错误码和审计 payload 文档：明确每类安全失败的 HTTP 状态、机器错误码、审计 action、脱敏字段和禁止记录字段。

## 细化需求

### OpenAPI Event API

范围：`POST /openapi/v1/events`。

最小可验收口径：

- 生产环境必须设置 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`，且必须配置至少一个有效 `EX_MARKETING_OPENAPI_TOKENS` token scope；未配置有效 token 时生产启动或发布检查应失败。
- 请求必须携带 `Authorization: Bearer <token>`。缺失、非 Bearer、空 token 或未知 token 返回 `401 UNAUTHORIZED`，不得进入事件模型校验之后的业务写入。
- token 必须绑定允许的 `tenantId` 和 `sourceSystem`。token 无权访问事件 tenant 返回 `403 TENANT_FORBIDDEN`；无权访问事件 source 返回 `403 SOURCE_FORBIDDEN`。
- `Idempotency-Key` 必填，且必须等于 `tenantId:sourceSystem:eventType:bizType:bizId`。完全重复同一 payload 返回 duplicate，不重复写 participation、grant_order、outbox 或 quota；同 key 不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`。
- OpenAPI 失败响应必须包含稳定 `errorCode` 或等价机器码与 `message`，不得包含完整 token、完整 Authorization header、内部 token 配置或敏感 payload。
- 所有成功业务写入必须以 payload 中的 `tenantId` 为租户边界，并与 token scope、活动 tenant、quota、grant_order、outbox、audit 保持一致。

### Webhook Receiver

范围：`POST /openapi/v1/webhooks/{receiverCode}`。

最小可验收口径：

- receiver 必须来自 `EX_MARKETING_WEBHOOK_RECEIVERS_JSON` 或等价安全配置，且必须 enabled；未知 receiver 返回 `WEBHOOK_RECEIVER_NOT_FOUND`，禁用 receiver 返回 `WEBHOOK_RECEIVER_DISABLED`。
- 请求必须使用 `Content-Type: application/json`，并携带 `X-ExMarketing-Timestamp` 与 `X-ExMarketing-Signature`。
- 签名输入为 `<timestamp> + "." + <raw request body>`，算法为 lowercase hex HMAC-SHA256，secret 来自 receiver 配置。
- timestamp 必须在允许重放窗口内。现有材料未给唯一窗口值，默认最小可验收口径为 300 秒；实现若已有配置项，可配置但生产不得大于 300 秒，测试环境可放宽但必须显式标注。
- 签名缺失返回 `WEBHOOK_SIGNATURE_MISSING`；timestamp 缺失、格式错误或过期返回 `WEBHOOK_TIMESTAMP_INVALID`；签名不匹配返回 `WEBHOOK_SIGNATURE_INVALID`。上述失败不得解析后写入标准事件链。
- allowed IP、allowed tenant、allowed sourceSystem 约束命中失败返回 `WEBHOOK_SOURCE_FORBIDDEN` 或等价稳定错误码，不得写业务状态。
- mapping 只能生成标准事件字段；映射后的 `tenantId` 和 `sourceSystem` 必须在 receiver allowlist 内。provider payload 不允许绕过 mapping 直接透传为跨 tenant 事件。
- 若请求提供 `Idempotency-Key`，必须等于映射后标准事件的幂等键；未提供时由映射事件派生标准 key。重复同 payload 应幂等，改 payload 重放应拒绝。

### Reward Callback

范围：`POST /openapi/v1/reward/callback`。

最小可验收口径：

- callback 请求必须携带 `tenantId`，所有定位、查询、状态更新、quota reservation 变更和审计均必须受该 `tenantId` 约束，不能跨 tenant 搜索或更新。
- `billNo` / `bill_no` / `commandId` 与 `idempotencyKey` 至少提供一个；同时提供时必须指向同一 tenant 下同一 grant order，否则返回 `409 CALLBACK_LOCATOR_CONFLICT`。
- 配置 callback secret 的 tenant 必须校验 `X-ExMarketing-Timestamp` 与 `X-ExMarketing-Signature`。签名输入为 `<timestamp> + "." + <raw_body>`，算法为 HMAC-SHA256。
- 生产 tenant/channel 默认必须配置 callback secret；现有材料允许“配置 secret 的 tenant 必须签名”，但安全 hardening 默认更保守：生产缺失 callback secret 视为发布阻断或 P0 补救任务。本期可记录为审计报告缺口，不因缺少真实 KMS 设计而 BLOCK。
- timestamp 默认重放窗口为 300 秒；实现若已有配置，可配置但生产不得大于 300 秒。
- 签名缺失返回 `401 CALLBACK_SIGNATURE_MISSING`；timestamp 无效返回 `401 CALLBACK_TIMESTAMP_INVALID`；签名错误返回 `401 CALLBACK_SIGNATURE_INVALID`。上述失败不得更新 grant_order、participation、quota 或 outbox。
- `status` 只接受 `SUCCESS` / `FAILED`；`SUCCESS` 必须有 `externalGrantId`。
- 首次 `SUCCESS` 确认 quota reservation；首次 `FAILED` 释放 quota reservation；不限额 grant_order 可无 reservation 并正常终结。
- 重复相同终态返回 duplicate，不重复移动 quota；相反终态、`externalGrantId` 冲突、已关闭单据、reservation 缺失或 quota 聚合冲突必须返回机器可识别错误并不进行部分成功更新。

### Admin 操作边界

范围：Admin API/UI、DLQ replay/close、manual grant decision、manual export、quota timeout release、activity/approval/quota 写操作和工作流/commit 等本地管理操作。

最小可验收口径：

- 本期非目标是不实现完整 Admin 登录和 RBAC；但审计报告必须明确 Admin 生产环境不得公网匿名暴露，必须通过内网、网关鉴权或后续 RBAC 保护。
- 所有 Admin 写操作必须有 tenant guard。请求体或查询参数中有 `tenantId` 的操作必须只影响该 tenant；通过资源 ID 定位的操作必须校验资源所属 tenant 后再变更。
- 高危操作必须要求 `operator` 和 `reason`，至少包括 manual grant decision、manual export、DLQ replay/close、quota timeout release、审批 approve/reject、活动上线或高风险配置变更。
- 高危操作必须写审计，审计 payload 至少包含 action、tenantId、resource/resourceId、operator、reason、结果和必要摘要；不得包含 token、secret、完整 Authorization、完整 targetRef、完整 payload 或明文敏感属性。
- 对缺少 tenant、operator、reason、资源不属于 tenant、资源状态不允许变更的请求，必须返回稳定错误码，并且不得产生业务状态变更。
- 后续鉴权接口预留的需求层口径：Admin handler 或入口层必须能接收已认证 operator/role/tenant scope 上下文；本期可以不实现真实登录，但不能把“任意字符串 operator”作为生产鉴权结论。

## 业务规则

1. 外部写入口无匿名成功路径：
   - OpenAPI Event API 在生产必须启用 Bearer token。
   - Webhook 必须启用 HMAC 签名。
   - Reward Callback 生产 tenant/channel 必须配置 callback secret 或列为 P0 发布阻断补救。
   - Admin 生产必须内网/网关/RBAC 保护；本期至少产出边界审计和后续鉴权预留。

2. tenant guard 规则：
   - 所有会写业务状态的接口必须有明确 tenant 来源。
   - token scope、receiver allowlist、callback locator、Admin tenant 参数和资源所属 tenant 必须一致。
   - 不允许通过 `activityId`、`grantOrderId`、`billNo`、`commandId`、`outboxId`、`idempotencyKey` 单独跨 tenant 定位并更新。
   - 不同 tenant 可以复用 activityCode、bizId、commandId 或类似业务标识，但定位必须包含 tenant。

3. replay 防护规则：
   - OpenAPI 使用固定 `Idempotency-Key` 作为重放和冲突防护。
   - Webhook 使用 timestamp + HMAC + 标准事件幂等键；本期默认不强制 nonce 持久化，除非审计发现 provider 重放风险无法由时间窗和幂等覆盖。
   - Reward Callback 使用 timestamp + HMAC + tenant-scoped locator + 终态幂等；本期默认不强制 nonce 持久化，但相反终态和 locator 冲突必须拒绝。
   - 如果后续实现选择 nonce，nonce 只能作为增强控制；不得替代 idempotency、tenant guard 和终态冲突校验。

4. 错误码规则：
   - 安全失败必须返回稳定机器码，Review/Test 可以直接断言。
   - 已文档化错误码必须沿用，包括 `UNAUTHORIZED`、`TENANT_FORBIDDEN`、`SOURCE_FORBIDDEN`、`INVALID_IDEMPOTENCY_KEY`、`WEBHOOK_SIGNATURE_MISSING`、`WEBHOOK_TIMESTAMP_INVALID`、`WEBHOOK_SIGNATURE_INVALID`、`WEBHOOK_SOURCE_FORBIDDEN`、`CALLBACK_SIGNATURE_MISSING`、`CALLBACK_TIMESTAMP_INVALID`、`CALLBACK_SIGNATURE_INVALID`、`CALLBACK_LOCATOR_CONFLICT`、`CALLBACK_STATUS_CONFLICT`。
   - 未定义的新 Admin 安全错误码默认使用可读机器码，例如 `ADMIN_TENANT_FORBIDDEN`、`ADMIN_OPERATOR_REQUIRED`、`ADMIN_REASON_REQUIRED`、`ADMIN_RESOURCE_STATE_INVALID`，最终命名可由实现统一，但必须文档化且测试稳定。

5. 审计规则：
   - 未授权、签名失败、越权、跨 tenant、重放冲突、locator 冲突和高危 Admin 操作必须产生可追踪审计或安全日志。
   - 审计必须脱敏：不得记录完整 token、Authorization、Cookie、secret、password、完整 raw payload、完整 targetRef、完整 provider 响应或明文凭据。
   - 对高基数字段如 eventId、bizId、subjectId、activityId、billNo、commandId、idempotencyKey，可在审计中记录必要业务引用；但 metrics label 和公开事故材料不得包含这些字段。

6. 状态写入规则：
   - 安全校验失败必须发生在业务状态变更前。
   - 失败请求不得写 participation、grant_order、outbox、quota reservation、quota aggregate 或改变 grant_order 终态。
   - 如果安全审计本身写入失败，不能放行原始业务写入；对 Webhook 已有 `WEBHOOK_AUDIT_FAILED` 口径，其他入口需记录等价失败或进入内部错误。

## 输入输出

### OpenAPI Event API 输入输出

- 输入 header：`Authorization: Bearer <token>`、`Content-Type: application/json`、`Idempotency-Key`。
- 输入 body：标准事件 8 个顶层字段 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 成功输出：正常事件处理结果，或重复事件 `duplicate=true`。
- 失败输出：稳定错误码和 message；不得包含完整凭据或敏感 payload。

### Webhook 输入输出

- 输入 path：`receiverCode`。
- 输入 header：`Content-Type: application/json`、`X-ExMarketing-Timestamp`、`X-ExMarketing-Signature`、可选 `Idempotency-Key`。
- 输入 body：provider 原始 JSON，仅用于签名校验和 receiver mapping。
- 成功输出：映射为标准事件后的事件处理结果，或幂等重复结果。
- 失败输出：Webhook 专用稳定错误码或标准事件错误码。

### Reward Callback 输入输出

- 输入 header：`Content-Type: application/json`、`X-ExMarketing-Timestamp`、`X-ExMarketing-Signature`。
- 输入 body：`tenantId`、`billNo`/`commandId`、`idempotencyKey`、`status`、`externalGrantId`、`message`、`finishedAt`。
- 成功输出：callback 终结结果，重复同终态时返回 duplicate。
- 失败输出：callback 专用稳定错误码，不改变业务终态。

### Admin 输入输出

- 输入：资源 ID、`tenantId`、操作 payload、`operator`、`reason`，以及后续可接入的认证上下文。
- 成功输出：变更后的资源或操作结果摘要。
- 失败输出：稳定错误码，明确是缺少字段、跨 tenant、状态不允许、未授权还是资源不存在。
- 审计输出：`marketing_audit` 或等价审计记录，包含操作摘要、操作者、原因、租户和结果。

## 异常场景

- OpenAPI 缺少 Authorization、非 Bearer、空 Bearer、未知 token。
- token 只允许 `site_001`，payload 使用 `site_002`。
- token 只允许 `payment`，payload 使用 `crm`。
- `Idempotency-Key` 缺失、格式不匹配、与 payload 不一致。
- 同一 `Idempotency-Key` 重复提交完全相同 payload。
- 同一 `Idempotency-Key` 重复提交不同 payload。
- Webhook receiver 不存在、receiver disabled。
- Webhook timestamp 缺失、非 unix seconds、超出默认 300 秒窗口。
- Webhook signature 缺失、大小写/hex 格式错误、HMAC 不匹配。
- Webhook source IP、mapped tenant、mapped sourceSystem 不在 allowlist。
- Webhook mapping 失败或 mapping 生成的标准事件缺少必填字段。
- Webhook 可选 `Idempotency-Key` 与映射后的标准事件 key 不一致。
- Callback tenant 缺失，或 locator 字段全部缺失。
- Callback 签名缺失、timestamp 过期、签名错误。
- Callback `commandId` 属于 tenant A，但请求 `tenantId` 为 tenant B。
- Callback 同时提供 `commandId` 和 `idempotencyKey`，二者指向不同 grant order。
- Callback 重复相同 `SUCCESS` 或相同 `FAILED`。
- Callback 先 `SUCCESS` 后 `FAILED`，或 `externalGrantId` 冲突。
- Callback 找不到 reservation、reservation 状态不一致、quota aggregate 冲突。
- Admin 写操作缺少 `tenantId`、`operator` 或 `reason`。
- Admin 使用 tenant A 请求修改 tenant B 的 activity/quota/grant/outbox。
- Admin replay 已关闭 DLQ、close 非 DLQ outbox、timeout release 已终结 grant order。
- 安全失败路径中的审计 payload 包含完整 Authorization、token、secret、raw body、完整 targetRef 或明文 provider error。

## 边界条件

- 本需求不要求选择具体加密库、存储表结构、中间件、handler 代码组织或配置加载实现。
- timestamp 默认按服务当前时间校验，生产最小可验收窗口为 300 秒；时钟偏差超过窗口的外部系统需先修正时钟或通过明确配置豁免。
- Webhook 和 Callback 本期默认用 timestamp + HMAC + 幂等作为重放防护；nonce 持久化是增强项，不作为本期唯一阻断条件。
- 本地开发可以关闭 OpenAPI auth，但生产发布检查必须把关闭状态视为失败或明确风险接受。
- Admin 完整登录/RBAC 不在本期实现范围，但 Admin 匿名公网暴露不能被视为生产可接受状态。
- Reward Callback secret 不接入真实 KMS；本期只要求 secret 不进入活动配置、outbox payload、审计 payload、metrics 或公开日志，并为后续 secret store/KMS 留出边界。
- 对历史数据或已有无签名回调的兼容策略不是本期目标；默认新增生产 tenant/channel 必须配置签名。
- 安全失败不写业务状态的要求优先于“返回更详细错误”；错误信息可以简化，但机器码必须稳定。

## 非目标范围

- 不实现完整 Admin 登录、SSO、RBAC、组织权限模型或公司级 IAM。
- 不接入真实 KMS、密钥轮换平台或云密钥管理服务。
- 不改变外部发奖系统协议之外的业务语义，例如 reward status 枚举、发奖金额计算、quota 算法或 outbox 投递语义。
- 不重新设计 ActivityPlugin、Fact Query、MQ sender、backup/restore 或导出文件保护；相关风险只在审计报告中引用或形成后续任务。
- 不要求本 Analysis 阶段读取代码、修改代码、提交代码或指定具体代码改法。
- 不承诺一次性修复全部 P1/P2 安全风险；本需求优先覆盖鉴权、签名、tenant guard、replay、错误码和审计。

## 风险点

- Admin 完整鉴权/RBAC 明确不在本期实现范围，若生产环境没有网关或内网隔离，仍是 P0 发布风险。
- Webhook/Callback 没有 nonce store 时，重放防护依赖 timestamp 窗口和幂等/终态冲突；如果 provider payload 对同一业务可产生多 key 重放，需要后续增强 nonce 或 provider event ID 约束。
- callback secret “配置后必须签名”与“生产必须有 secret”的口径需要发布门禁落地，否则可能出现未配置 secret 的 tenant 被匿名 callback 终结。
- tenant guard 容易在通过资源 ID 定位的 Admin/Callback/DLQ 操作中遗漏，测试必须覆盖“ID 存在但属于其他 tenant”。
- 安全审计如果记录 raw body、完整 targetRef、Authorization 或 token，会把鉴权加固变成信息泄漏风险。
- 稳定错误码需要兼容已有 API 文档；新 Admin 错误码命名若不统一，会影响自动化测试和运维排障。

## 验收标准

Review/Test 可按以下标准验收：

- OpenAPI 未带 Authorization、错误 token、跨 tenant token、跨 source token 均被拒绝，且不会写 event inbox、participation、grant_order、outbox 或 quota。
- OpenAPI 重复同 payload 返回 duplicate，不重复发奖；同 key 不同 payload 返回 `INVALID_IDEMPOTENCY_KEY`。
- Webhook 缺签名、错签名、过期 timestamp、disabled receiver、allowlist 不匹配均被拒绝，且不会进入标准事件业务写入。
- Webhook 映射后的 tenant/source 必须受 receiver allowlist 约束，可选 `Idempotency-Key` 必须等于映射后标准事件 key。
- Callback 缺签名、错签名、过期 timestamp、跨 tenant locator、locator 冲突、相反终态冲突均被拒绝，且不会更新 grant_order、quota reservation 或 quota aggregate。
- Callback 重复相同终态幂等，不重复确认或释放 quota。
- Admin 高危写操作必须具备 tenant guard；跨 tenant 资源修改、缺少 operator/reason 或非法状态转换被拒绝。
- 所有安全失败有稳定机器错误码；已文档化错误码不得随意变更。
- 安全失败和高危操作有脱敏审计或安全日志，审计不得包含完整 token、Authorization、Cookie、secret、password、raw payload、完整 targetRef 或明文凭据。
- 安全审计报告列出每个入口的现有控制、缺口、风险等级、补救任务和对应测试证据。
- `go test ./...` 通过；如果测试阶段无法覆盖某个外部配置门禁，必须在 `test.md` 中记录未覆盖项和人工验证方式。

## 待确认问题

以下问题不阻塞本需求，因为已给出默认最小可验收口径：

- Webhook/Callback timestamp 窗口最终是多少？默认口径：生产 300 秒；如已有配置，生产不得大于 300 秒，测试需覆盖过期请求。
- 是否必须实现 nonce store？默认口径：本期不强制；timestamp + HMAC + 标准幂等/终态冲突为最小可验收，审计发现 provider 重放风险不足时再形成 nonce 补救任务。
- Callback secret 是否允许 tenant 未配置？默认口径：生产 tenant/channel 不允许未配置；缺失 secret 作为 P0 发布门禁或补救任务，开发/本地可显式豁免。
- Admin 生产鉴权采用网关、内网 ACL 还是后续 RBAC？默认口径：本期不实现完整登录，但生产不得公网匿名暴露；高危操作必须有 tenant guard、operator、reason 和审计。
- 新 Admin 安全错误码最终命名是什么？默认口径：可以由实现统一命名，但必须稳定、文档化、可测试，并区分未授权、跨 tenant、缺少 operator/reason 和状态不允许。

## Score

```text
91
```

评分说明：

- 需求清晰度：19/20，入口、目标、交付物和默认口径明确。
- 业务规则完整：19/20，覆盖 OpenAPI、Webhook、Callback、Admin、tenant guard、replay、错误码和审计。
- 边界异常清楚：18/20，列出主要安全异常和边界；Admin 真实 IAM 作为非目标和发布风险保留。
- 验收标准明确：18/20，可由 Review/Test 设计负向和幂等用例；部分生产配置门禁需人工证据。
- 约束一致性：17/20，严格不设计代码改法，兼容已有 API 文档；callback secret 与 Admin 生产保护仍需发布门禁落地。

## Decision

```text
PASS
```
