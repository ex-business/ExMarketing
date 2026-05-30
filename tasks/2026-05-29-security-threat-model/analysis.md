# Analysis

## 需求 ID

2026-05-29-security-threat-model

## Review 反馈处理

`tasks/2026-05-29-security-threat-model/review.md` 当前仍为模板态：

- Findings 中仅有 `P0:`、`P1:`、`P2:` 占位，没有具体问题描述。处理：不视为 Review Agent 的实质打回项；本轮无需逐条修复具体 Review Finding。
- Open Questions 为“待填写”。处理：不视为明确业务问题；本轮基于任务文档、README、架构、API、数据模型和运维文档补齐默认最小可验收口径。
- Decision 为 `PASS / BLOCK` 模板占位。处理：不视为明确 BLOCK 原因；本轮仍按高风险安全需求重新产出完整分析，供 Review Agent 正式审核。

## Analysis 自检修正

上一轮 `analysis.md` 是空模板，存在以下阻塞项：

- `Score = 0`，低于 80。修正：本轮补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题，并重新评分。
- `Decision = PASS / BLOCK` 模板占位。修正：本轮在材料足够确定业务目标、验收对象和参与方的前提下给出 `Decision PASS`。
- 背景、目标、细化需求、业务规则、输入输出、异常场景、边界条件、非目标范围、风险点、验收方式、待确认问题均为“待填写”。修正：本轮按正式安全威胁建模需求完整补齐。
- 旧分析未说明默认口径。修正：本轮明确采用“系统级威胁模型文档 + 数据流/信任边界图 + 风险矩阵 + 后续安全任务清单”为最小可验收交付物；威胁分类默认采用 STRIDE，风险分级默认采用 P0/P1/P2/P3，覆盖范围以 task.md 明确入口和资产为准。

## 背景和目标

ExMarketing 是独立营销系统，连接外部业务事件、Webhook Receiver、Fact Provider、MQ、外部发奖系统、管理后台、DB、Outbox/DLQ、人工导出和备份恢复链路。上线前需要形成正式安全威胁模型，明确资产、入口、信任边界、攻击面、现有控制、控制缺口和后续补救任务，作为发布前安全 review 的基线。

本需求的目标不是直接修复所有安全问题，而是产出可审核、可执行、可追踪的威胁建模交付物：

- 系统级威胁模型文档。
- 数据流和信任边界图。
- 风险矩阵，包含严重度、可能性、影响、现有控制、缺口、责任方向和后续任务。
- P0/P1 风险对应的可执行补救任务清单。

## 参与方

- 外部业务系统：通过 OpenAPI Event API 或 Webhook 上报标准事件，受 token、签名、tenant/sourceSystem 范围、幂等和请求体大小约束。
- 外部 Fact Provider：被 Fact Query 调用，返回规则需要的事实数据，受 provider registry、authRef、allowlist、SSRF 防护、timeout、mapping 和缓存约束。
- 外部发奖系统：接收 HTTP/Webhook/MQ Reward Command，并通过 Reward Callback 回传发奖结果，受 targetRef/MQ allowlist、callback tenant 定位、签名、timestamp 和幂等约束。
- 管理后台用户：创建活动、quota、审批、模拟、人工发奖、导出、DLQ replay/close、审计查询和工作流操作，生产环境默认要求鉴权或仅内网暴露。
- 运维/值班人员：处理 DLQ、Outbox backlog、callback failure、quota consistency、DB failure、备份恢复和告警演练，受 runbook、业务确认和审计约束。
- ExMarketing 服务：事件接入、规则匹配、quota reservation、grant_order、outbox、callback、audit、metrics、backup/migration 的系统主体。
- DB/MQ/对象文件或本地文件存储：保存核心业务数据、审计、MQ 凭据引用、导出文件和备份文件，是高价值资产和信任边界。

## 输入

威胁模型分析必须读取并引用现有需求和产品材料中的业务事实，至少包括：

- `tasks/2026-05-29-security-threat-model/task.md`
- `README.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/runbooks/exmarketing-alerts.md`
- 如涉及插件/调度事件，参考 `docs/activityplugin-lifecycle.md`

默认输入范围以文档描述为准，不要求读取代码，不允许用代码实现细节替代需求澄清。

## 输出

最小可验收输出为一份正式威胁模型文档，可以写入后续实现/文档任务，但本 Analysis 仅定义需求，不创建该交付物。该文档必须包含：

- 范围声明：明确本次覆盖与不覆盖内容。
- 资产清单：活动配置、审批状态、quota、quota reservation、grant_order、outbox、DLQ、audit、fact provider registry/authRef、MQ runtime credential/authRef/TLS 文件引用、OpenAPI token、Webhook receiver secret、callback secret、导出文件、备份文件、metrics/logs/runbook 证据。
- 入口清单：OpenAPI Event API、Webhook Receiver、Reward Callback、Fact Query、Admin API/Admin UI、DLQ 管理 API、Manual Grant/Manual Export、Outbox sender 到 HTTP/Webhook/MQ、MQ provider、metrics、backup/restore/migration 命令或流程。
- 数据流/信任边界图：至少覆盖公网入口、管理入口、内网服务、DB、MQ、外部 Fact Provider、外部发奖系统、文件导出/备份、监控告警。
- 威胁枚举：默认按 STRIDE 分类，即 Spoofing、Tampering、Repudiation、Information Disclosure、Denial of Service、Elevation of Privilege。
- 风险矩阵：每条风险必须有入口/资产、威胁类别、攻击路径、影响、现有控制、缺口、严重度、可能性、风险等级、建议补救方向、验收证据。
- 后续任务清单：P0/P1 必须拆成可执行任务；P2/P3 可记录为 backlog，但必须有 owner 方向和验收口径。

## 业务规则

1. 覆盖规则：
   - 每个外部入口必须至少有一条威胁分析记录。
   - 每个高危资产必须有保护措施或明确缺口。
   - 每条跨信任边界的数据流必须标出 source、sink、协议、身份凭据、tenant 约束和失败后果。
   - OpenAPI、Admin、Webhook Receiver、Reward Callback、Fact Provider、MQ、Outbox、DLQ、导出/备份文件必须全部覆盖，不能只覆盖 HTTP API。

2. 风险分级默认口径：
   - P0：可能导致跨 tenant 发奖/扣减、重复发奖、预算或 quota 不一致、认证绕过、密钥泄漏可直接利用、大规模数据泄漏、生产核心写路径不可控篡改。
   - P1：可能导致单 tenant 资金/权益损失、发奖状态错误、DLQ replay 滥用、SSRF 访问内网资源、Admin 高危操作未鉴权或未审计、callback 签名绕过、备份/导出敏感数据泄漏。
   - P2：可能导致局部拒绝服务、敏感日志暴露、告警/审计不足、错误码泄漏、provider 配置错误放大、运维误操作风险。
   - P3：低影响的加固项、文档改进、监控细化或开发/测试环境隔离提醒。

3. 现有控制识别必须基于文档事实：
   - OpenAPI Event API 生产环境要求 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`，Bearer token 限定 tenantId 和 sourceSystem，幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`，默认请求体限制 1 MiB。
   - Webhook Receiver 要求 JSON、timestamp、HMAC-SHA256 signature，且可限制 allowedTenantIds、allowedSourceSystems、allowedIPs；可选 Idempotency-Key 若提供必须等于映射后的标准事件 key。
   - Reward Callback 以 tenantId 定位，支持 billNo/commandId/idempotencyKey，配置 secret 的 tenant 必须校验 timestamp 与 HMAC-SHA256 签名，冲突 locator 或相反终态必须拒绝。
   - Fact Provider 只按 tenant_id + fact_code 读取 enabled registry，不接受 inline provider 配置；auth_ref 不保存明文密钥；endpoint 需要 allowlist，并拦截 private/local IP 以防 SSRF。
   - MQ targetRef 只允许 destination URI，broker endpoint、凭据和 TLS 文件只存在 runtime sender config；targetRef 不允许账号、密码、token、query 或 fragment；destinationAllowlist 命中失败进入 retry/DLQ。
   - Outbox 使用 retry/DLQ 语义；SENT 只代表投递到 broker/目标系统成功，不代表发奖成功；真实终态以 callback 为准。
   - DLQ replay/close 只允许当前状态为 DLQ，DLQ_CLOSED 不允许 replay，operator 和 reason 必填，并写审计；业务确认是 replay/close 的默认前置约束。
   - Audit payload、metrics label、公开事故材料和日志不得包含 Authorization、Cookie、token、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始 provider error。
   - Backup/restore 输出必须脱敏 DSN；restore 默认拒绝源库等于目标库、空备份、不可读备份和非空目标库，`--force` 仅限明确本地演练复核。

4. 风险矩阵字段规则：
   - 严重度至少使用 P0/P1/P2/P3。
   - 可能性至少使用 High/Medium/Low。
   - 影响必须说明业务后果，不得只写“安全风险”。
   - 现有控制必须能在文档中找到依据；找不到时写“未见明确控制”。
   - 缺口必须可转化为后续任务，不能只写“待确认”。

5. 后续任务规则：
   - P0/P1 必须生成补救任务，任务描述至少包含问题、入口/资产、建议控制、验收标准和优先级。
   - 如果缺口是“控制是否已启用”而不是“控制缺失”，默认任务为“发布前配置审计/启动阻断/运维检查”，不能阻塞威胁模型交付。
   - 如果现有材料没有唯一答案，默认采用更保守的生产口径：生产环境必须启用鉴权、签名、allowlist、tenant guard、脱敏、审计和告警。

## 边界条件

- 本需求只要求威胁建模交付物，不要求修改代码、配置或基础设施。
- 本次分析以系统级安全边界为主，不做公司级 IAM、组织权限、HR/审批制度、云账号治理或外部渗透测试替代。
- Admin 当前文档说明本地页面不应直接暴露公网生产环境；威胁模型必须将“生产 Admin 未鉴权或公网暴露”作为高风险缺口评估。
- 内存仓储模式可用于本地开发；威胁模型中的生产安全结论默认以 PostgreSQL 持久化模式为上线口径。
- Fact Query 仅覆盖 HTTP provider；其他 provider 类型不在当前最小范围内。
- MQ Event In 尚不作为当前已文档化入口处理；MQ Command Out、MQ provider runtime config 和 destination allowlist 必须覆盖。
- 导出文件和备份文件的下载、长期存储、加密、保留周期如果文档未定义，默认作为缺口进入风险矩阵，不因此 BLOCK。
- Threat model 图可以是 Mermaid、PlantUML 或 Markdown ASCII 图；验收看覆盖完整性，不限定具体画图工具。

## 异常场景

威胁模型必须至少覆盖以下异常或攻击场景：

- 伪造 OpenAPI 事件、未授权 sourceSystem 上报、token 跨 tenant 使用、Idempotency-Key 与 payload 不一致、重复事件重放。
- Webhook 签名缺失、timestamp 过期、签名不匹配、allowed tenant/source/IP 不匹配、provider payload 映射生成跨 tenant 事件。
- Reward Callback 签名缺失或错误、locator 冲突、跨 tenant commandId/idempotencyKey、重复 callback、相反终态冲突、externalGrantId 冲突。
- Fact Provider endpoint 指向内网/private/local 地址、host 不在 allowlist、provider 返回超大/恶意 JSON、mapping 泄漏敏感字段、authRef 被误写成明文。
- 活动配置中的 targetRef 尝试携带凭据、query、fragment、非允许协议或内网地址，导致 SSRF 或密钥泄漏。
- MQ runtime config 误配置 endpoint 凭据、destinationAllowlist 缺失或过宽、TLS 文件引用泄漏、sender disabled 造成 DLQ 堆积。
- Outbox worker 重试放大 provider 故障、SENT 被误认为发奖成功、DLQ replay 在 provider 未恢复时重复投递。
- Admin 高危操作缺少鉴权、操作者、reason 或审计，包括活动上线、quota 创建、manual grant decision、manual export、timeout release、DLQ replay/close。
- 导出文件包含 grant_order、subject、金额或权益数据但缺少访问控制、checksum、脱敏、过期清理或下载审计。
- 备份文件包含核心业务数据、fact provider metadata、audit、grant_order、outbox 等敏感信息但缺少加密、访问控制、保留策略或恢复目标隔离。
- metrics、日志、审计、告警、事故材料暴露 token、secret、完整 targetRef、完整 payload 或高基数字段。
- DB 核心写失败、quota reservation missing/aggregate conflict、callback finalize 失败导致预算冻结、确认或释放不一致。

## 非目标范围

- 不直接实现安全修复、代码改造、配置变更或基础设施部署。
- 不替代外部渗透测试、漏洞扫描、SAST/DAST、依赖漏洞治理。
- 不设计公司级 IAM、SSO、RBAC 组织模型或云账号权限体系。
- 不定义真实钱包、支付、游戏结算、会员等级和风控系统的内部安全模型。
- 不重新设计业务规则、奖励计算、quota 算法或 Outbox 可靠性实现。
- 不要求读取源代码验证实现是否符合文档；实现一致性可作为后续安全审计任务。

## 风险点

- 当前文档已有不少控制，但威胁模型尚未将控制与入口/资产逐项映射，发布前 review 容易漏掉跨边界风险。
- Admin 生产鉴权、权限分级、内网暴露策略在现有材料中不是完整 IAM 设计，必须作为风险矩阵重点项。
- 导出文件和备份文件的加密、保留、下载权限和审计细节不足，默认存在 P1/P2 级缺口，需要后续任务落地。
- Fact Query、targetRef、MQ endpoint 都涉及 SSRF 或外联边界，allowlist 和 private IP 拦截必须作为上线前强制控制检查。
- DLQ replay/close 和 quota timeout-release 是高影响运维动作，必须要求业务确认、operator、reason、审计和最小范围操作。
- 敏感信息脱敏跨日志、metrics、audit、runbook、backup 命令输出等多面，若只检查 API 响应会遗漏风险。

## 验收标准

Review/Test 可按以下标准验收威胁模型交付物：

- 覆盖完整性：OpenAPI、Admin、Webhook Receiver、Reward Callback、Fact Provider、MQ、Outbox、DLQ、导出文件、备份文件均有独立威胁分析。
- 资产完整性：task.md 列出的活动配置、quota、grant_order、outbox、audit、fact provider secret/authRef、MQ credential/authRef、导出文件、备份均出现在资产清单和风险矩阵中。
- 边界完整性：公网入口、管理入口、内网服务、DB、MQ、外部发奖系统、Fact Provider、文件存储/备份、监控告警至少各有一条数据流或信任边界说明。
- 风险矩阵可审核：每条风险包含入口/资产、STRIDE 类别、攻击路径、影响、现有控制、缺口、严重度、可能性、风险等级和验收证据。
- 高风险可执行：所有 P0/P1 风险都有后续任务，任务包含补救方向、优先级和可验证验收标准。
- 控制不虚写：所有“现有控制”都能从现有文档找到依据；没有依据的只能写“缺口”或“待配置审计”。
- 敏感信息要求明确：威胁模型必须显式要求 token、secret、Authorization、Cookie、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 不进入 metrics labels、公开事故材料和未脱敏日志。
- 发布可用性：文档读者可以基于该威胁模型判断发布前必须完成哪些 P0/P1 安全任务，哪些 P2/P3 可进入 backlog。

## 待确认问题

以下问题不阻塞本需求，因为已给出默认最小可验收口径：

- Admin 生产鉴权和权限模型最终采用什么方案？默认口径：生产环境 Admin 必须鉴权或仅内网暴露；高危操作必须有 operator、reason、审计和最小权限要求，具体 IAM 方案作为后续任务。
- 导出文件和备份文件是否已有统一加密与保留策略？默认口径：未见明确控制则作为 P1/P2 风险缺口，要求补充访问控制、checksum、加密或隔离存储、保留/销毁和下载/恢复审计。
- Threat model 文档最终存放路径和图格式是什么？默认口径：任意 Markdown 可审核格式均可接受，图可使用 Mermaid/PlantUML/ASCII；关键是覆盖范围和风险矩阵完整。
- 风险分级是否沿用公司统一安全评级？默认口径：本任务采用 P0/P1/P2/P3，并将 P0/P1 作为发布前必须处理或明确豁免的风险。

## Score

```text
92
```

评分说明：

- 需求清晰度：19/20，交付物、范围和默认口径明确。
- 业务规则完整：18/20，覆盖入口、资产、风险矩阵和后续任务规则；公司级 IAM 细节留给后续任务。
- 边界异常清楚：19/20，列出主要攻击/异常场景和非目标范围。
- 验收标准明确：19/20，可由 Review/Test 按覆盖项和矩阵字段检查。
- 约束一致性：17/20，严格不要求代码实现；少量生产策略仍依赖后续配置审计。

## Decision

```text
PASS
```
