# Analysis

## Agent 指令

你是 Analysis Agent，只负责需求分析和需求细化。
请读取本目录 task.md、PRD/设计文档、业务规则文档、已有 API/数据字典说明。
不需要读取代码，不允许用代码实现细节替代需求澄清。

只写 analysis.md，不写代码，不改代码，不提交，不做实现，不设计具体代码改法。
你的目标是把需求细节一定一定搞清楚：背景、目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、验收标准和待确认问题。
如果分析结果还不满足 Review / Dev / Test 可执行要求，必须由 Analysis Agent 基于现有材料、同类接口和最小可验收原则继续补齐，直到形成可审核、可开发、可测试的需求；不得因为缺少 Owner 决策、字段名、URL、错误码、配置来源或协议细节而直接 BLOCK。

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

`review.md` 当前为模板状态，包含 `P0:`、`P1:`、`P2:`、`Open Questions 待填写` 和 `PASS / BLOCK` 占位，但没有实际列出 Review Agent 的具体打回意见。

处理口径：

- `P0/P1/P2`：无具体条目可逐条整改；本轮按“未发生实质 Review 打回”处理。
- `Open Questions 待填写`：视为模板占位；本轮通过补齐背景目标、参与方、输入输出、业务规则、边界异常、非目标、风险和验收标准，消除可预见的 Review 空白。
- `PASS / BLOCK`：视为模板占位，不继承为有效 BLOCK；本轮重新打分并给出明确 Decision。

## Analysis 自检修正

上一轮 `analysis.md` 为模板，存在以下自检问题：

- `背景和已知信息`、`目标理解`、`细化需求`、`业务规则`、`输入输出`、`异常场景`、`边界条件`、`非目标范围`、`风险点`、`验收方式`、`待确认问题` 均为“待填写”。本轮已基于 `task.md`、`docs/security-threat-model.md`、`docs/api.md`、`docs/operations.md`、`docs/architecture.md`、`docs/data-model.md`、告警规则文档和相关安全任务补齐。
- `Score` 为 `0`，低于 80。原因是上一轮没有形成可审核、可开发、可测试的需求。本轮按评分规则重新评估为 92。
- `Decision` 为 `PASS / BLOCK` 占位。本轮给出 `PASS`。
- 旧文档没有说明默认口径。本轮采用默认最小可验收口径：发布门禁必须能在本地/CI 前置步骤中聚合检查结果并生成可追溯报告；P0/P1 安全问题、必须检查命令失败、生产关键安全配置缺失默认 BLOCK；P2 或有补偿措施且记录完整的风险可 WARN；风险接受必须有 owner、原因、补偿措施和到期时间，且不得无期限接受。

## 背景和已知信息

ExMarketing 是连接外部事件、Fact Query、Reward Command、Reward Callback、Admin、DLQ、导出、备份和监控的营销平台。安全威胁模型已经把 OpenAPI、Webhook Receiver、Reward Callback、Fact Provider、Admin、DLQ、Manual Export、Outbox、Metrics、Backup/Restore 等入口纳入发布安全基线，并列出 P0/P1/P2 风险与后续安全任务。

本需求要把安全审计从“文档建议”落到“发布前门禁”。发布前必须有 checklist、自动检查项、审计报告模板和风险接受记录，让高危问题不能静默上线，让可接受风险有明确责任人、原因、补偿措施和到期时间。

已知约束：

- 生产 OpenAPI Event API 必须启用 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`，并配置 token 与 tenant/sourceSystem 作用域。
- Admin 与 `/admin/` 当前定位为本地或内网运营面，不应直接暴露公网生产。
- Fact Query 必须走 provider registry、allowlist、private/local IP 拦截、禁止 redirect、禁用 proxy。
- Reward Callback 配置 secret 的 tenant 必须通过 timestamp + raw body HMAC 校验；缺失、过期、签名错误不得写业务状态。
- MQ 运行时 credential、Fact Provider `authRef`、Webhook/Callback secret 必须作为运行时配置或 secret 引用，不能进入 activity config、outbox payload、audit payload、日志、metrics 或公开事故材料。
- DLQ replay/close、manual grant/export、quota timeout release 等高影响操作必须有 operator、reason、audit；客户可见或资金/权益结果可能变化时还需要业务确认。
- 告警规则已定义 P0/P1/P2/P3 级别和敏感标签限制，核心关注 DLQ、callback、outbox、worker、DB、quota、Fact Query。
- 发布门禁不替代公司 CI/CD、渗透测试、公司 IAM 或真实生产审批系统；它是 ExMarketing 仓库级的发布前安全证据和阻断规则。

## 目标理解

本需求的目标是定义并交付一个发布前安全门禁流程，使发布候选版本在进入正式发布前可以得到明确结论：

- `PASS`：所有必须检查通过；无未接受的 P0/P1；报告、命令、commit、环境和负责人可追溯。
- `BLOCK`：存在未通过的必须检查、未接受的 P0/P1、生产关键安全配置缺失、报告证据缺失或风险接受记录不合规。
- `WARN`：存在非阻断风险、P2/P3 事项、占位扫描能力、环境不可验证项或已接受的风险；WARN 不应掩盖 P0/P1 的默认 BLOCK。

交付物必须覆盖：

- 安全发布 checklist。
- 审计报告模板。
- 可运行的门禁脚本或命令集合。
- 风险接受记录格式。

## 参与方

- Release Owner：发起发布门禁，提供版本、commit、目标环境和发布窗口，确认最终报告。
- Security Owner：审核 P0/P1/P2 安全风险、风险接受和补偿措施。
- Backend Owner：对 Go 测试、构建、API/业务安全检查结果负责。
- Ops / Platform Owner：对生产配置基线、Admin/metrics 网络边界、secret 来源、备份/导出存储、告警路由负责。
- Business Owner：对 DLQ replay/close、manual grant/export、quota repair、风险接受的业务影响负责。
- Review / Test / Commit Agent：基于本需求审核、制定测试计划、执行验证和记录发布证据。

默认最小可验收口径：如果没有显式 owner 映射，报告中的 `releaseOwner`、`securityOwner`、`backendOwner`、`opsOwner` 至少必须填写为非空字符串；本需求不要求接入公司通讯录或权限系统。

## 细化需求

### 1. 发布门禁状态

门禁输出只允许以下顶层结论：

- `PASS`：允许继续发布。
- `BLOCK`：不允许发布，必须修复或补齐合规风险接受后重新生成报告。
- `WARN`：允许进入人工复核或非生产发布；正式生产发布若存在 WARN，Release Owner 和 Security Owner 必须在报告中确认。

正式生产发布的默认规则：

- P0/P1 安全问题未修复且无合规风险接受时必须 `BLOCK`。
- P0 风险原则上不得被普通风险接受绕过；只有威胁模型或 task 明确允许补偿控制时，才可作为限期风险接受进入 `WARN`，并必须记录补偿措施和到期时间。
- P2 可在记录 owner、原因、补偿措施和到期时间后进入 `WARN`。
- P3/信息项不阻断，但必须进入报告。

### 2. Checklist 范围

Checklist 必须至少覆盖 task.md 指定范围，并映射到威胁模型风险：

- 鉴权：OpenAPI auth flag、token tenant/source 作用域、未授权/越权负例。
- 签名：Webhook Receiver 和 Reward Callback timestamp + HMAC，缺失/过期/错误签名不写业务状态。
- Tenant guard：Event、Callback、Admin 查询/操作、Audit/Reconciliation、Manual Grant/Export、DLQ 操作不得跨 tenant。
- Secret：OpenAPI token、Webhook secret、Callback secret、Fact Provider `authRef`、MQ authRef/TLS refs 不得进入 activity config、outbox payload、audit payload、日志、metrics 和报告明文。
- 日志脱敏：错误响应、审计 source 摘要、provider 错误、outbox 失败原因、incident material 不得暴露 token/header/secret/完整 payload。
- DLQ 操作：replay/close 只能作用于 `DLQ`，`DLQ_CLOSED` 不能 replay；operator/reason 必填；客户可见影响需要业务确认证据。
- 导出/备份：导出文件和备份文件必须记录 checksum、operator、时间、访问/保留/加密或隔离存储口径；恢复不能覆盖源库或非演练目标。
- Metrics 隐私：Prometheus labels 不得包含 token、Authorization、Cookie、secret、password、完整 payload、完整 targetRef、eventId、bizId、subjectId、userId、activityId、billNo、commandId、idempotencyKey 或原始错误文本。
- Threat Model：`docs/security-threat-model.md` 中 P0/P1 项必须在报告中有状态：fixed、accepted、deferred-as-blocker 或 not-applicable。
- Alerting：P0/P1 告警规则和 runbook 链接必须作为发布证据，至少覆盖 worker all stopped、DB write failure、quota over-deduct risk、DLQ growing、outbox backlog high、callback fail rate high。

### 3. 自动检查项

门禁命令集合必须聚合以下检查结果，并把命令、退出码、开始/结束时间和摘要写入报告：

- `go test ./...`
- `go build ./...`
- secret scan：默认最小口径为对仓库文本内容执行敏感模式扫描，至少识别 token、Authorization、secret、password、private key、DSN 明文密码、callback/webhook secret、MQ credential 明文等高风险模式；允许使用占位命令，但占位必须在报告中标为 `WARN`，不得伪装为已扫描通过。
- 依赖漏洞扫描：默认最小口径为执行仓库可用的 Go 依赖漏洞扫描或等价占位命令；如果本地缺少扫描工具，报告必须标为 `WARN` 并列出“未完成依赖漏洞扫描”的风险项。若扫描结果含高危/严重漏洞，正式发布必须 `BLOCK`，除非有合规风险接受。
- 配置基线检查：至少校验生产环境期望项的证据存在，包括 OpenAPI auth 必开、Admin/metrics 内网或网关保护、Fact allowlist 非空、callback/webhook secret 配置证据、MQ destination allowlist 或风险接受、PostgreSQL persistence、非内存仓储、secret 不在业务配置中。
- 威胁模型检查：读取或引用威胁模型 P0/P1 风险清单，要求每项在报告中有处理状态。
- 测试结果检查：必须纳入 `go test ./...` 结果；可附加安全相关单测、告警规则静态校验、promtool/alertmanager 校验结果，但本需求不强制要求所有外部工具在每个环境可用。

默认最小可验收口径：当某类自动检查当前只能做占位时，门禁仍必须生成报告，但该项状态为 `WARN`；如果占位项属于 P0/P1 风险唯一证据来源，正式发布状态必须 `BLOCK`，直到补充证据或风险接受合规。

### 4. 审计报告模板

报告必须是可归档、可比对、可人工审阅的文本或结构化文件。默认最小字段：

- `reportId`：唯一报告 ID，建议包含 UTC 时间。
- `generatedAt`：报告生成时间，UTC/RFC3339。
- `version`：发布版本或候选版本。
- `commit`：完整 commit SHA；没有 commit 时必须记录当前工作区状态并 BLOCK 正式发布。
- `environment`：`local`、`test`、`staging`、`production` 等。
- `releaseOwner`、`securityOwner`、`backendOwner`、`opsOwner`。
- `overallDecision`：`PASS` / `BLOCK` / `WARN`。
- `summary`：检查总数、通过数、WARN 数、BLOCK 数、风险接受数。
- `checks[]`：每个检查的 id、名称、类别、严重级别、命令或证据来源、状态、退出码、摘要、开始/结束时间、日志/文件路径。
- `threatModelItems[]`：P0/P1/P2 风险映射、状态、证据、owner。
- `riskAcceptances[]`：风险接受记录。
- `openRisks[]`：未关闭风险，包含 severity、reason、requiredAction、owner、dueDate。
- `artifacts[]`：报告引用的 checklist、命令输出、测试结果、扫描结果、告警/配置证据路径。

报告不得包含 secret 明文、完整 token、完整 Authorization/Cookie、完整 DSN 密码、完整 provider response、完整 business payload 或未脱敏用户标识。

### 5. 风险接受记录格式

每条风险接受必须包含：

- `riskId`：可追溯到威胁模型、checklist 或扫描结果。
- `severity`：P0/P1/P2/P3。
- `title` 和 `description`。
- `acceptedBy`：业务或安全责任人，非空。
- `owner`：修复负责人，非空。
- `reason`：为什么当前发布可接受该风险。
- `compensatingControls`：补偿措施，非空；例如网络隔离、人工复核、只限非生产、监控告警、临时禁用功能。
- `expiresAt`：到期时间，必须为未来 RFC3339 日期时间或明确日期；不得为空，不得写“永久”。
- `followUpAction`：后续动作。
- `evidence`：支撑该接受的配置、截图、工单、文档或命令结果引用。

风险接受合规性：

- 缺少 `acceptedBy`、`owner`、`reason`、`compensatingControls` 或 `expiresAt` 任一字段，风险接受无效。
- 到期时间已过，无效。
- P0 风险接受默认无效，除非报告中明确记录这是临时补偿控制且 Security Owner 与 Release Owner 均确认；无补偿控制的 P0 必须 BLOCK。
- 已接受风险必须在报告 summary 中计数，不能隐藏为 PASS 项。

### 6. 流程

发布门禁流程必须支持：

1. 生成或更新 checklist。
2. 运行自动检查命令集合。
3. 汇总检查、威胁模型、配置基线、扫描、测试和风险接受。
4. 生成审计报告。
5. 人工确认：Release Owner 与 Security Owner 对正式发布报告确认。
6. 归档：报告与证据路径可追溯到版本和 commit。

默认最小可验收口径：流程可以先在仓库内以文档和本地命令形式运行，不要求接入公司 CI/CD；但输出必须足够让后续 CI/CD 复用。

## 业务规则

- `BLOCK` 优先级最高。只要存在任一 BLOCK 条件，整体结论必须为 `BLOCK`，不能被其他 PASS 覆盖。
- `WARN` 不等于 PASS。正式生产发布存在 WARN 时必须有人工确认；非生产可允许 WARN 继续，但报告必须保留。
- P0/P1 安全风险默认阻断正式发布；P2 默认 WARN；P3 默认记录。
- 自动检查命令失败默认 BLOCK，除非该命令被标记为非阻断占位，并且不属于 P0/P1 唯一证据来源。
- `go test ./...` 失败必须 BLOCK。
- `go build ./...` 失败必须 BLOCK。
- secret scan 发现高置信明文 secret、private key、生产 token、DSN 明文密码必须 BLOCK。
- 依赖扫描发现 critical/high 漏洞默认 BLOCK；medium 默认 WARN；low/info 记录。
- 生产配置基线缺失以下任一项必须 BLOCK：OpenAPI auth 必开、生产 token inventory 证据、Admin 非公网或受网关/RBAC 保护、callback secret 配置证据、Fact allowlist、PostgreSQL persistence、metrics 非公网或受控采集。
- P0/P1 告警或 runbook 完全缺失时必须 BLOCK；告警工具不可用但规则文件和 runbook 存在时可 WARN。
- 风险接受必须限期，且到期后自动视为未接受。
- 报告中任何 secret 泄漏都视为门禁失败。
- 门禁不改变业务 API 的请求/响应语义，不新增对外业务能力。

## 输入输出

### 输入

- 发布元数据：版本、commit、目标环境、发布窗口、Release Owner、Security Owner、Backend Owner、Ops Owner。
- 需求和安全文档：当前 task、威胁模型、API 文档、运维文档、数据模型、相关安全任务。
- 命令执行结果：测试、构建、secret scan、依赖扫描、配置基线、告警规则校验。
- 配置证据：生产或目标环境的脱敏配置摘要，例如 auth flag、allowlist、secret 引用存在性、Admin/metrics 暴露边界。
- 风险接受记录：人工提供或仓库内维护的风险接受条目。

### 输出

- 安全发布 checklist 文档或章节。
- 审计报告模板。
- 门禁命令集合的执行报告。
- 风险接受记录格式。
- 最终门禁结论：`PASS`、`BLOCK` 或 `WARN`。
- 可归档证据路径列表。

### 输出状态字段

每个检查项至少输出：

```text
id
name
category
severity
status: PASS / BLOCK / WARN / SKIP
blocking: true / false
evidence
command
exitCode
summary
startedAt
finishedAt
owner
```

`SKIP` 只能用于明确不适用项，必须有 reason；生产 P0/P1 检查不得无理由 SKIP。

## 异常场景

- 缺少发布元数据：报告可生成，但正式发布必须 BLOCK，直到补齐 version、commit、environment 和 owner。
- 工作区不是干净 commit 或无法解析 commit：正式发布 BLOCK；本地试跑可 WARN。
- 自动命令不存在：如果是占位允许项，WARN；如果是 `go test`、`go build` 或 P0/P1 唯一证据命令，BLOCK。
- secret scan 或依赖扫描工具不可用：标记 WARN；若目标为 production 且没有替代证据，高危安全发布必须 BLOCK。
- 配置证据无法读取：对应配置检查 BLOCK；不得假设生产安全。
- 风险接受字段缺失或过期：视为未接受，按原风险等级判定。
- 报告生成失败或报告缺少总体结论：BLOCK。
- 报告中包含敏感明文：BLOCK，并要求重新生成脱敏报告。
- 威胁模型文件缺失：BLOCK，因为本需求依赖其作为安全基线。
- 告警规则文件或 runbook 缺失：生产发布 BLOCK；非生产 WARN。
- P0/P1 风险标记为 not-applicable 但无依据：BLOCK。
- 同一风险既出现在 accepted 又出现在 open blocker：以 BLOCK 为准。

## 边界条件

- 环境边界：本需求覆盖 ExMarketing 仓库级发布前检查；不同环境可使用不同严格度，但 production 最严格。
- 数据边界：报告只保存脱敏摘要和证据路径，不保存原始 secret、完整业务 payload 或用户敏感标识。
- 工具边界：允许依赖扫描、secret scan、promtool 等外部工具不可用时降级为 WARN/占位，但不能把占位冒充 PASS。
- 风险边界：不允许无期限风险接受；不允许无补偿措施风险接受；不允许 P0/P1 静默通过。
- 文档边界：本需求不要求一次性修复威胁模型中所有后续任务，但要求每个 P0/P1 有发布门禁处理状态。
- API 边界：本需求不改变 OpenAPI、Admin、Webhook、Callback、Fact Query、DLQ、Manual Export、Audit、Metrics 的既有业务协议。
- 审计边界：门禁报告是发布审计证据，不等同于运行时 `marketing_audit` 业务审计表。

## 非目标范围

- 不替代公司统一 CI/CD、变更审批、IAM、KMS、漏洞管理平台或渗透测试。
- 不实现完整 Admin 登录/RBAC。
- 不接入真实 KMS、对象存储、企业工单或审批系统。
- 不承诺所有第三方依赖漏洞都能在离线环境扫描出来。
- 不修改业务 API、数据库模型、活动规则、奖励计算、Quota、Outbox、Callback 或 Fact Query 语义。
- 不保证生产网络 ACL 实际已经配置；本需求只要求门禁记录证据并在缺证时 BLOCK/WARN。
- 不允许用“后续确认”替代最小可验收口径；缺口必须进入报告并按规则判定。

## 风险点

- 占位扫描可能造成虚假安全感；必须在报告中显式 WARN，并对 production P0/P1 证据缺失 BLOCK。
- P0 风险接受若放宽过度，会破坏门禁价值；默认不允许无补偿控制的 P0 接受。
- 生产配置证据可能来自人工摘要，存在不真实风险；报告必须记录 owner 和 evidence path，后续可接入 CI/CD 或配置平台。
- 门禁报告若泄漏 secret，会把安全流程变成泄漏源；必须把报告脱敏作为 BLOCK 规则。
- 告警和 runbook 是运行期兜底，不应替代上线前修复；高危未修复仍按 BLOCK 处理。
- 多环境口径不一致可能导致 staging PASS、production BLOCK；报告必须明确 environment。
- 威胁模型更新后门禁清单可能滞后；报告必须引用威胁模型版本或文件路径。

## 验收方式

### 文档验收

- 存在安全发布 checklist，覆盖鉴权、签名、tenant guard、secret、日志脱敏、DLQ、导出/备份、metrics 隐私、威胁模型、告警。
- 存在审计报告模板，包含版本、commit、环境、检查结果、风险项、接受项、负责人、命令和证据路径。
- 存在风险接受记录格式，包含 owner、acceptedBy、reason、compensatingControls、expiresAt，且说明无期限接受无效。
- 文档明确 `PASS`、`BLOCK`、`WARN` 判定规则。

### 功能/流程验收

- 能通过门禁命令集合生成一份报告。
- 报告能追溯 commit、检查命令、退出码、结果摘要和证据路径。
- `go test ./...` 失败时整体 `BLOCK`。
- `go build ./...` 失败时整体 `BLOCK`。
- P0/P1 未修复且无合规风险接受时整体 `BLOCK`。
- P2 风险有合规风险接受时整体可为 `WARN`，并在 summary 计数。
- 缺少 owner 或 expiresAt 的风险接受无效，并按原风险等级判定。
- secret scan 发现明显明文 secret 时整体 `BLOCK`。
- 报告不得包含完整 token、Authorization、Cookie、secret、password、完整 DSN 密码、完整 payload 或高基数敏感标识。
- 依赖漏洞扫描或配置基线检查不可用时，不得输出假 PASS；必须 WARN 或 BLOCK。

### 回归命令验收

后续 Test Agent 至少应设计并执行：

```bash
go test ./...
go build ./...
```

并根据实现交付物补充门禁脚本/命令集合的正反例：

- 无 P0/P1、命令成功、风险接受为空时生成 PASS 报告。
- 注入 P1 open risk 时生成 BLOCK 报告。
- 注入缺少 expiresAt 的 risk acceptance 时生成 BLOCK 报告。
- 将依赖扫描配置为占位时生成 WARN，而不是 PASS。
- 注入报告敏感字段时生成 BLOCK 或脱敏失败。

## 待确认问题

以下问题不阻断本轮需求推进，已给出默认最小可验收口径：

- 依赖漏洞扫描具体工具未指定。默认使用仓库可用的 Go 漏洞扫描或等价占位；占位为 WARN，production P0/P1 证据缺失为 BLOCK。
- secret scan 具体工具未指定。默认使用仓库文本敏感模式扫描；发现高置信 secret BLOCK；工具不可用 WARN/production 按风险升级。
- 报告文件格式未指定。默认接受 Markdown 或 JSON/YAML，只要字段完整、可归档、可审核。
- 风险 owner 来源未指定。默认由报告输入填写非空 owner，不接入通讯录。
- P0 风险是否可接受未细化。默认 P0 不允许普通接受；只有具备明确补偿控制、到期时间、Release Owner 和 Security Owner 确认时可临时 WARN，否则 BLOCK。
- 生产配置证据来源未指定。默认接受脱敏配置摘要、命令输出或运维文档引用；证据缺失即 BLOCK/WARN，不能假设安全。

## Score

~~~text
92
~~~

评分说明：

- 需求清晰度 19/20：目标、交付物、参与方和流程明确；具体工具名称保留实现选择。
- 业务规则完整 19/20：PASS/BLOCK/WARN、P0/P1/P2、风险接受和检查结果聚合规则明确。
- 边界异常清楚 18/20：覆盖元数据缺失、工具不可用、证据缺失、敏感泄漏、威胁模型缺失等异常；外部企业审批系统不在范围内。
- 验收标准明确 18/20：文档、流程、命令、正反例均可测试；具体脚本名称由实现阶段确定。
- 约束一致性 18/20：与 task、API、威胁模型、运维告警和安全任务一致；未用代码实现细节替代需求澄清。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-security-release-gate-report
