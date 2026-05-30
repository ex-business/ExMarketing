# Analysis

## 背景和已知信息

本需求是高风险安全审计项，目标是防止 ExMarketing 在活动配置、Fact Query、Reward Callback、Outbox、人工导出、数据库备份、日志、指标和审计链路中泄露 secret 或敏感业务数据。

已读取材料：

- `tasks/2026-05-29-security-secrets-data-export/task.md`
- `tasks/2026-05-29-security-secrets-data-export/review.md`
- 本文件上一轮内容
- `docs/api.md`
- `docs/data-model.md`
- `docs/operations.md`
- `docs/security-threat-model.md`
- 当前任务目录内 `test-plan.md`、`implementation.md`、`test.md` 的占位信息

现有文档已明确以下基线：

- MQ broker endpoint、credential、TLS 文件和 `authRef` 属于运行时 sender 配置，不允许进入 activity config、outbox payload、audit payload 或日志。
- MQ `targetRef` 只表达业务目的地，如 topic/exchange/routingKey，不允许携带 broker 地址、账号、密码、token、query 或 fragment。
- Fact Provider `authRef` 是引用，不是明文密钥；Fact Query 受 allowlist、SSRF 防护、timeout 和 mapping 约束。
- Reward Callback 使用 tenant-scoped HMAC-SHA256 签名，错误响应不得泄露 token/header/secret/provider 原始响应。
- 人工导出会生成 CSV、checksum、导出明细和审计 action；当前没有长期导出列表或下载接口承诺。
- 备份恢复工具输出需要脱敏 DSN，备份成功证据包含文件路径、大小、SHA-256 和 UTC 创建时间。
- 指标标签禁止 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始错误文本。

## Review 反馈处理

`review.md` 当前是模板，`P0/P1/P2` 未填写具体问题，`Open Questions` 为“待填写”，`Decision` 为占位 `PASS / BLOCK`。本轮按“存在 Open Questions 或打回原因必须回应”的要求处理如下：

- `P0/P1/P2`：未发现具体 Review finding；本轮不按 Review finding 打回，但会按高风险安全需求补齐可审核细节。
- `Open Questions: 待填写`：无具体问题可逐条关闭；本轮将默认最小可验收口径写入“待确认问题”，并且不因缺少 Owner 决策而 BLOCK。
- `Decision: PASS / BLOCK`：为模板占位，不构成明确打回原因；本轮用完整需求、边界、验收和评分重新给出 Analysis Decision。

## Analysis 自检修正

上一轮 `analysis.md` 是空模板，存在以下自检问题：

- `Score = 0`：已补充背景目标、参与方、输入输出、业务规则、边界异常、非目标、风险和验收标准，重新评分。
- `Decision = PASS / BLOCK`：已改为明确 `PASS`。
- “背景和已知信息/目标理解/细化需求/业务规则/输入输出/异常场景/边界条件/非目标范围/风险点/验收方式/待确认问题”均为“待填写”：本轮全部补齐。
- 旧分析没有关闭“缺少 Owner 决策”的风险：本轮采用默认最小可验收口径，即先不引入对象存储、完整文件权限系统或数据分级平台，只要求现有导出/备份/日志/审计/错误响应链路做到不落 secret、必要元数据可审计、敏感字段脱敏可测试。

## 目标理解

本需求不是新增一套完整安全平台，而是在现有 ExMarketing 范围内完成 secret 与敏感数据流向审计，并形成可测试的保护要求：

- secret 只能通过运行时配置、安全引用或挂载文件被使用。
- 活动配置、outbox payload、audit payload、日志、错误响应、指标和导出/备份证据不得泄露 secret 明文或 provider 原始敏感响应。
- 人工导出和备份文件必须至少具备 checksum、operator 或操作者上下文、创建/过期或保留时间、访问/恢复审计口径。
- 对已知敏感字段提供统一脱敏规则和测试覆盖。
- 若发现范围内已有泄露路径，Dev Agent 需做最小修复或明确生成后续任务；本 Analysis 不指定代码改法。

## 参与方

- Admin/Ops operator：创建人工导出、执行 DLQ 操作、执行备份恢复演练、查看审计和错误信息。
- External source system：调用 OpenAPI Event API 或 Webhook，上报 subject、IP、userAgent、attributes 等事件数据。
- Reward provider：接收 HTTP/Webhook/MQ outbox 命令，并通过 callback 回写最终状态。
- Fact Provider：被 Fact Query 调用，返回 provider response。
- Security/Release reviewer：审核 secret 流向、脱敏规则、导出/备份安全证据和测试结果。
- Backend service：处理活动、规则、Fact Query、Outbox、Callback、Audit、Export、Backup/Restore 和 Metrics。

## 敏感数据清单和流向

| 类别 | 示例 | 允许出现位置 | 禁止出现位置 | 默认脱敏/保留口径 |
| --- | --- | --- | --- | --- |
| OpenAPI token | `Authorization: Bearer <token>`、`EX_MARKETING_OPENAPI_TOKENS` | 运行时配置、请求鉴权内存态 | 日志、错误响应、审计 payload、指标标签、导出/备份证据 | 只显示前后少量字符或固定 `<redacted>` |
| Callback/Webhook secret | HMAC secret、signature key | 运行时 secret 来源、签名校验内存态 | activity config、outbox、audit payload、日志、错误响应、导出 | 固定 `<redacted>`，不回显长度和原文 |
| MQ credential | broker user/password、token、TLS private key | runtime sender config、挂载文件路径引用 | activity config、outbox payload、audit payload、日志、指标 | endpoint 去 userinfo/query，credential 固定脱敏 |
| Fact Provider authRef | `secret/...` 引用 | provider registry 中的安全引用 | 明文 provider 密钥、日志、audit payload、错误响应 | 允许记录引用类型/最后路径片段，禁止明文 secret |
| subject/userId | `subject.id`、`userId` | 业务表、必要查询条件、租户内审计引用 | 指标标签、公开错误、无必要的导出文件名 | 日志中 hash 或部分遮盖 |
| IP/userAgent | 请求 IP、UA | 安全审计摘要、风控必要字段 | 指标标签、公开错误、无关导出 | IP 掩码，UA 截断并去 token-like 片段 |
| provider response | Fact/Reward provider 原始响应、stderr | 必要时仅内部短期诊断且脱敏摘要 | 错误响应、audit payload、日志全文、导出 | 保存状态码、reason code、截断脱敏摘要 |
| outbox payload | reward command | outbox 表和发送链路中必要业务字段 | secret、token、完整 Authorization、broker credential | 目标只保留安全 targetRef/destination |
| audit payload | 操作审计 | operator、reason、resource、必要摘要 | secret、token、完整 header、provider 原始响应 | payload 必须是脱敏摘要 |
| export file | MANUAL_EXPORT CSV | 受控输出目录/存储，带 checksum/状态/过期 | 未审计下载、无过期长期暴露、文件名含敏感 ID | 文件名不含 secret/完整用户标识；内容按业务必要最小 |
| backup file | PostgreSQL dump | 受控备份目录，带 checksum/创建时间/保留口径 | 日志泄露 DSN 密码、无权限/无保留策略 | 输出脱敏 DSN；文件权限和路径纳入审计证据 |

## 细化需求

1. Secret 流向审计
   - 审计范围必须覆盖 OpenAPI token、Webhook secret、Callback secret、MQ credential/TLS refs、Fact Provider `authRef`、HTTP/Webhook/MQ reward target。
   - 需求交付物必须列出每类 secret 的来源、使用点、是否持久化、允许出现位置和禁止出现位置。
   - 明确禁止在 activity config、rule/reward config、outbox payload、audit payload、日志、错误响应、指标标签、导出文件和备份命令输出中保存或展示 secret 明文。

2. 敏感数据流向审计
   - 审计范围必须覆盖 subject、userId、IP、userAgent、attributes 中可能的身份字段、provider response、errorMessage、request/response 摘要、outbox payload、audit payload、export file、backup file。
   - 输出必须区分“业务必需持久化字段”和“只允许脱敏摘要字段”。
   - provider response 默认不作为完整业务证据保存；只能保存状态、reason code、截断且脱敏后的错误摘要。

3. 脱敏规则
   - token、Authorization、Cookie、secret、password、clientKey、privateKey、signature、auth、credential、DSN password 等关键字命中的值必须脱敏。
   - URL/DSN 必须去除 userinfo、password/token/secret 等 query 值；必要时保留 scheme、host、path 或 destination 摘要。
   - 错误响应面向 API caller 时只返回机器错误码、短 reason 和必要定位字段，不返回完整 header、token、secret、provider raw body、SQL/pg_dump 原始敏感 stderr。
   - 审计 payload 和日志可以保留 operator、reason、tenantId、resource/action、状态码、reason code、checksum、rowCount、createdAt/expiresAt 等非 secret 证据。

4. 导出安全
   - 人工导出创建必须记录 operator、reason、exportId、fileFormat、rowCount、checksum、createdAt、expiresAt 或默认过期策略、筛选摘要和状态。
   - 默认最小可验收口径：若现有接口未提供下载功能，本需求不新增长期下载接口，但必须定义导出文件元数据和创建审计；如已有本地文件产生，则文件名不得包含 secret 或完整用户标识。
   - 若存在下载入口，必须记录 download audit：operator/downloader、exportId、downloadedAt、client/IP 摘要、reason 或 request id；过期文件不得下载。
   - 导出内容只包含人工发奖所需最小字段；禁止包含 secret、token、完整 Authorization、provider raw response、callback secret 或 MQ credential。

5. 备份安全
   - 备份成功证据必须包含路径、大小、SHA-256、UTC 创建时间、脱敏源 DSN。
   - 恢复审计或演练证据必须包含 operator/执行者上下文、backup file、checksum 校验结果、restore target 脱敏 DSN、时间、结果和失败阶段。
   - 默认最小可验收口径：本期不实现完整备份权限系统或加密存储，但必须要求备份文件路径、权限/访问约束、保留周期和恢复审计在文档或运行输出中可验证。
   - 错误输出不得包含数据库密码、token、secret、Authorization 或 Cookie。

6. 指标、日志、审计和错误响应
   - 指标标签只能使用低基数字段；禁止任何 secret、高基数业务 ID、完整 payload、完整 targetRef 和原始错误文本。
   - 日志和审计必须使用脱敏摘要；错误信息需截断，且在截断前后均不得残留敏感片段。
   - SIMULATION_RUN、MANUAL_GRANT_EXPORT_CREATE、MANUAL_GRANT_EXPORT_STATUS、DLQ replay/close、callback failure、backup/restore 相关证据均需符合脱敏规则。

## 业务规则

- Activity config 只允许业务配置和安全目的地引用，不得承载 MQ broker 地址、credential、token、secret、callback secret 或 Fact Provider 明文密钥。
- MQ `targetRef` 只能是 `kafka://<topic>`、`rabbitmq://<exchange>/<routingKey>`、`rabbitmq:///<routingKey>` 或 `pulsar://<tenant>/<namespace>/<topic>`；不得包含 userinfo、query、fragment、password、token 或 secret。
- MQ endpoint、authRef、TLS 文件路径和 destination allowlist 属于运行时 sender 配置；运行时配置可记录引用名称，但不回显 credential 明文。
- Fact Provider `authRef` 是 secret 引用；若发现明文 secret 形式，至少要在审计结论中标为问题，并要求后续修复或本期最小修复。
- Callback/Webhook 签名失败、timestamp 失败、token 失败等错误不得进入业务更新；返回错误不得泄露签名材料或完整 header。
- Outbox payload 保存发送业务所需数据，但不得保存 secret 或完整 Authorization；`SENT` 只代表发送确认，不代表最终发奖成功。
- 人工导出只选择符合 MANUAL_EXPORT 状态规则的 grant order；导出 READY 后推进明细至 SENDING；未失败/未作废明细阻止重复纳入。
- 备份恢复默认拒绝同源同目标、空/不可读备份、目标库已有 public base table；`--force` 只作为本地/演练安全例外，生产使用需后续单独审批流程。

## 输入输出

输入：

- 现有 API 请求：OpenAPI Event、Webhook、Reward Callback、Admin manual export、DLQ replay/close、simulation、Fact Query、backup/restore 命令输入。
- 运行时配置：OpenAPI token map、callback/webhook secret、MQ sender JSON、Fact allowlist、Fact Provider registry、PostgreSQL DSN。
- 业务数据：subject/userId、IP、userAgent、attributes、provider response、grant order、outbox、audit、export detail、backup file。

输出：

- 敏感数据清单和流向文档或等价说明。
- 统一脱敏规则，可覆盖日志、错误响应、audit payload、provider error summary、DSN/URL/header。
- 导出文件安全要求：exportId、fileFormat、rowCount、checksum、operator、reason、createdAt、expiresAt、状态、创建审计；若有下载入口则包含 download audit。
- 备份文件安全要求：路径、大小、SHA-256、UTC 创建时间、脱敏 DSN、保留/权限口径、恢复审计或演练证据。
- 测试覆盖：敏感字段脱敏、secret 不落 config/outbox/audit/log/error、导出/备份元数据和错误脱敏。

## 异常场景

- 请求或运行时配置中包含 `Authorization`、Cookie、token、password、secret、private key：错误响应、日志、审计、指标和导出证据必须脱敏。
- MQ `targetRef` 带 userinfo/query/fragment 或疑似 credential：必须拒绝保存或发送，错误摘要脱敏。
- MQ runtime endpoint 带 userinfo 或 `password/token/secret` query：必须作为配置错误处理，不能回显明文。
- Fact Provider 返回包含 token/secret/header/raw profile 的错误体：系统只保留状态和脱敏摘要。
- Callback 签名缺失/过期/错误：返回机器错误码，不写入业务终态，不记录完整签名或 raw body。
- 人工导出 rowCount 为 0：可返回空导出结果或不生成文件，但必须有状态、operator、reason 和审计说明。
- 导出文件生成失败：状态为 FAILED，记录失败阶段和脱敏错误摘要，不推进明细到 SENDING。
- 导出文件过期后下载：拒绝下载并写入失败下载审计；如本期无下载接口，则该规则作为后续下载入口的验收约束。
- 备份文件为空、不可读、checksum 不匹配或 restore target 不安全：拒绝恢复，输出脱敏错误摘要。
- 脱敏函数遇到未知嵌套 JSON、数组、URL、DSN、header map：默认递归扫描敏感 key 和 token-like 值，无法确认时倾向脱敏。

## 边界条件

- 本期最小可验收范围包括现有 ExMarketing 文档中出现的 API、导出、备份、日志、审计、指标和运行时配置；不要求扫描仓库外系统。
- “不出现 secret”按明文和可直接复原的编码值判断；Base64/URL-encoded/JWT-like/token-like 值也应作为敏感值处理。
- subject/userId 并非所有业务表中都禁止保存；禁止的是无必要进入日志、指标标签、公开错误、文件名或不受控导出。
- audit payload 允许保存 operator、reason、resourceId、checksum、rowCount、状态和筛选摘要，但不得保存完整 provider response、完整 Authorization、secret 或完整高敏 attributes。
- export file 允许包含人工发奖必要字段；若字段存在用户标识，应遵循业务必要最小原则，文件名不得包含完整用户标识。
- backup file 本身包含完整数据库数据，不能通过字段脱敏满足安全；其安全依赖路径、权限、保留周期、checksum、审计和后续加密/ACL任务。
- 本期不要求实现对象存储、KMS、企业级 RBAC 或数据分级平台；但不得因为这些非目标缺失而放松脱敏、checksum、operator、时间和审计要求。

## 非目标范围

- 不实现完整文件权限系统、企业 IAM/RBAC、对象存储接入、KMS 或数据分级平台。
- 不设计新的长期导出列表或下载接口；若已有下载入口则按本需求纳入审计和过期控制。
- 不改变奖励业务语义、Quota 语义、Callback 终态语义、DLQ 状态机或 Fact Query 业务映射。
- 不要求对历史已生成导出/备份文件做全量迁移清理；若发现历史风险，应记录为后续任务。
- 不以代码实现细节替代需求澄清；具体代码位置和实现方案由 Dev Agent 后续决定。

## 风险点

- Admin API/UI 当前文档定位为本地或内部运维面，生产 auth/RBAC 未完整定义；本需求只能要求导出/审计数据不泄露，不解决完整访问控制。
- 手工导出和备份文件可能包含高敏业务数据；本期最小口径不实现加密或对象存储，仍需后续生产 release gate 单独验证 ACL/加密/保留策略。
- provider response 结构不可控，脱敏规则必须覆盖嵌套 key、URL/DSN、header 和 token-like value，否则容易在错误摘要中泄露。
- 仅靠文档约束无法证明所有日志不泄露；必须有测试覆盖典型错误路径，并建议 Test Agent 增加字符串扫描断言。
- 过度脱敏可能影响排障；默认保留状态码、reason code、checksum、rowCount、resource/action 和截断后的非敏感摘要作为可运维证据。

## 验收方式

功能和安全验收：

- 敏感数据清单覆盖 task.md 中列出的 Secret、数据、日志、导出、备份范围，并明确允许/禁止位置。
- activity config、rule/reward config、outbox payload、audit payload、日志、错误响应和指标中不出现 OpenAPI token、Webhook secret、Callback secret、MQ credential、Fact Provider 明文密钥、完整 Authorization/Cookie。
- MQ `targetRef` 不能保存 credential、query 或 fragment；MQ runtime endpoint/config 错误摘要不得回显 credential。
- Callback/Webhook/OpenAPI auth 错误响应只暴露机器错误码和脱敏 reason，不暴露 token/header/secret/provider raw response。
- Fact Provider 失败、MQ sender 失败、backup/restore 失败和 export 失败均只记录脱敏摘要。
- 人工导出创建结果或审计包含 exportId、operator、reason、fileFormat、rowCount、checksum、createdAt、expiresAt 或默认过期策略、状态。
- 若存在导出下载入口，下载必须检查未过期并写入 download audit；若不存在下载入口，analysis/test-plan 明确不要求新增。
- 备份成功输出包含路径、大小、SHA-256、UTC 创建时间和脱敏 DSN；恢复/validate/smoke 失败输出不包含密码、token、secret、Authorization 或 Cookie。
- 敏感字段脱敏有单元测试覆盖，至少覆盖 header、URL、DSN、JSON 嵌套 payload、provider error、audit payload、export/backup evidence。

命令验收：

```bash
go test ./...
go build ./...
```

建议 Test Agent 额外做负向断言：

- 构造包含 `Authorization: Bearer secret-token-value`、`password=p@ssw0rd`、`secret=abc`、`callbackSecret`、provider raw error 的请求或配置，断言响应、审计、日志摘要、导出/备份输出不包含原文。
- 对导出/备份输出断言存在 checksum、operator/执行者上下文、时间字段和脱敏 DSN/路径证据。

## 待确认问题

以下问题不阻塞本期进入研发，按默认最小可验收口径推进：

- 生产导出文件是否必须加密：本期默认不实现加密，只要求 checksum、operator、过期、审计和不泄露 secret；加密作为后续生产 hardening。
- 导出文件默认过期时间：若现有材料没有配置，默认使用“必须存在 expiresAt 或文档化默认过期策略”；具体时长由实现沿用现有导出模型或设为最小保守默认。
- 备份保留周期具体天数：本期默认要求输出或文档中可见保留口径，不强制具体天数；生产发布前再由运维策略确定。
- Admin 下载权限模型：本期不实现完整权限系统；默认 Admin 面仅内部可达，下载入口如存在必须记录 operator 和 download audit。
- `authRef` 允许格式：本期默认按“引用而非明文”验收；具体 secret store scheme 可后续标准化。

## Score

```text
90
```

评分说明：

- 需求清晰度：18/20，目标、范围、参与方和最小口径明确。
- 业务规则完整：18/20，覆盖 secret、outbox、callback、Fact、导出、备份、指标日志审计。
- 边界异常清楚：18/20，列出关键失败和过期/配置错误场景。
- 验收标准明确：18/20，可由 Review/Test 按字段和负向断言验收。
- 约束一致性：18/20，与 task.md、API、data-model、operations、threat model 保持一致；未强行引入非目标平台能力。

## Decision

```text
PASS
```

需求：2026-05-29-security-secrets-data-export
