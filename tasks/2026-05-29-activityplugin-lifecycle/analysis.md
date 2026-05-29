# Analysis

## 需求 ID

2026-05-29-activityplugin-lifecycle

## 需求标题

activityplugin-lifecycle: 插件生命周期与版本契约

## 资料来源

- `tasks/2026-05-29-activityplugin-lifecycle/task.md`
- `tasks/2026-05-29-activityplugin-lifecycle/review.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/api.md`
- `docs/workflow.md`
- `docs/business-activity-templates.md`
- `tasks/2026-05-24-activityplugin/analysis.md`
- `tasks/2026-05-29-activityplugin-context-state/task.md`
- `tasks/2026-05-29-activityplugin-red-envelope-lottery/task.md`
- `tasks/2026-05-29-activityplugin-scheduler-events/task.md`

本轮只做需求分析和需求细化。不读取代码，不写代码，不改代码，不提交，不设计具体代码改法。

## Review 反馈处理

`review.md` 当前为模板内容，包含 `P0:`、`P1:`、`P2:`、`Open Questions 待填写` 和 `PASS / BLOCK` 规则说明，但没有实际填写的打回条目、严重级别、开放问题或拒绝原因。

处理结果：

- P0：无实际 finding；本轮不需要针对具体 P0 修正。
- P1：无实际 finding；本轮不需要针对具体 P1 修正。
- P2：无实际 finding；本轮不需要针对具体 P2 修正。
- Open Questions：为模板占位“待填写”，不是 Review Agent 提出的具体问题；本轮基于任务说明、架构、API、数据模型和相邻插件任务补齐最小可验收口径。
- Decision：模板中的 `PASS / BLOCK` 是判定规则，不是实际 BLOCK 结论；不作为 Review 打回阻塞。

## Analysis 自检修正

上一轮 `analysis.md` 是初始模板，存在以下 BLOCK 原因和扣分项：

- Score 为 `0`，低于 80。
- Decision 为 `PASS / BLOCK` 占位，没有有效结论。
- 背景、目标、细化需求、业务规则、输入输出、异常场景、边界条件、非目标、风险、验收方式和待确认问题均为“待填写”。
- 未回应插件版本契约、能力声明、生命周期顺序、禁用策略、审计 payload、错误分类和上线前校验的可验收口径。

修正结果：

- 已补齐背景目标、参与方、输入输出、业务规则、边界异常、非目标、风险和验收标准。
- 已给出默认最小可验收口径：本阶段必须让 ActivityPlugin 成为主活动引擎的受控扩展点；插件可提供玩法上下文、状态计算、候选结果或扩展校验，但不得直接发奖、扣 quota、写 participation、grant_order、outbox 或绕过 Event Inbox。
- 已明确当材料没有唯一答案时的默认约束：版本使用语义化版本兼容判断；活动上线时按 tenantId + activityId + activityVersion 固定解析为一个 pluginCode + pluginVersion；禁用策略默认对新上线 BLOCK，对已上线活动按活动配置策略处理，未配置时默认 BLOCK；插件执行失败默认回滚本活动级副作用并记录结构化错误与审计。
- 已关闭旧 BLOCK：本轮 Score 为 92，Decision 为 PASS。

## 背景和已知信息

ExMarketing 的核心链路是 Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox -> External Reward System。系统已有 ActivityPlugin 注册表基础能力，插件可作为红包雨、抽奖、翻卡、竞猜等玩法上下文扩展点，但必须服从主活动引擎的 MATCH / RULE / REWARD / QUOTA / PARTICIPATION / GRANT_ORDER / OUTBOX 边界。

已有文档明确：

- ActivityPlugin 是可选扩展点，普通首存、续存、签到、任务、VIP、邀请等能通过标准事件、规则、奖励、Quota 表达的活动应优先走普通配置。
- 插件上下文只能暴露为 `state.plugin.<pluginCode>.*`。
- 插件不得替代规则、奖励、Quota、参与记录、grant_order、outbox、callback 或幂等处理。
- 活动创建和上线前校验会严格校验活动配置、规则字段路径、奖励、审批状态和插件存在性。
- 模拟运行必须无副作用，正式处理必须保证 participation、quota、grant_order、outbox 的事务边界。

本需求是在已有注册表基础上补齐长期可扩展所需的版本契约、能力声明、生命周期、兼容性、禁用策略、执行错误和审计。

## 目标理解

本需求目标不是实现某个具体玩法插件，而是定义并落地 ActivityPlugin 作为平台能力的稳定契约：

- 插件注册时必须声明身份、版本、支持的活动类型、能力、配置 Schema、状态 Schema 和生命周期能力。
- 活动配置必须声明所需插件、版本范围和能力要求；上线前必须校验插件存在、版本兼容、能力满足和配置合法。
- 运行时必须按活动配置版本固定解析插件版本，避免插件升级影响已审批活动。
- 插件生命周期必须嵌入主流程，但不得拥有发奖、扣 quota、写 outbox 等主引擎职责。
- 插件禁用、升级、执行失败和回滚都必须可审计、可追踪、可解释。

## 参与方

- 活动配置人员：在活动配置中可选声明插件编码、版本范围、所需能力和插件配置；负责上线前校验与审批。
- 活动主引擎：负责事件幂等、活动匹配、规则、奖励、Quota、participation、grant_order、outbox、事务和回滚。
- ActivityPlugin：提供玩法上下文、扩展校验、候选结果、状态计算或事件处理建议；不得绕过主链路。
- 插件注册者：注册、升级、禁用插件并提供能力声明、Schema 和兼容性说明。
- 审计与运维人员：通过 audit 追踪插件注册、升级、禁用、执行失败、回滚和活动固定解析结果。
- Test/Review/Dev Agent：基于本分析审核、开发和验证插件生命周期契约。

## 输入输出

### 输入

- 插件注册信息：
  - `pluginCode`：插件唯一编码，系统级唯一，建议使用稳定大写蛇形编码，如 `RED_ENVELOPE`、`LOTTERY`。
  - `version`：插件语义化版本，默认最小口径为 `MAJOR.MINOR.PATCH`。
  - `capabilities`：能力集合，例如 `CONTEXT_BUILD`、`CONFIG_VALIDATE`、`BEFORE_MATCH`、`BEFORE_REWARD`、`AFTER_REWARD_CALCULATED`、`EVENT_HANDLE`、`ROLLBACK`、`DISABLE_POLICY`。
  - `supportedActivityTypes`：允许绑定的活动类型集合；为空不允许上线绑定，不能表示“全部允许”。
  - `configSchema`：插件配置 Schema，用于创建和上线前校验。
  - `stateSchema`：插件状态或 snapshot 输出 Schema，用于审计、模拟和 participation snapshot 解释。
  - `disablePolicySupport`：插件支持的禁用后处理能力，至少声明是否支持 `BLOCK`、`DEGRADE`、`READ_ONLY`。
- 活动配置：
  - `pluginCode` 可选；未配置时走普通活动链路。
  - `pluginVersionRange`：活动可接受的插件版本范围；未配置但配置了 `pluginCode` 时，默认要求当前注册表中最新兼容版本，并在上线时固定到具体版本。
  - `requiredCapabilities`：活动依赖的插件能力；未配置时按活动引用的生命周期和规则字段推导最小能力。
  - `pluginConfig`：插件私有配置，只能放在活动扩展配置的受控插件配置区，不能新增非标准活动顶层字段。
  - `pluginDisabledStrategy`：已上线活动遇到插件禁用时的策略，可选 `BLOCK`、`DEGRADE`、`READ_ONLY`；未配置默认 `BLOCK`。
- 运行时上下文：
  - 标准事件、活动配置快照、活动版本、租户、traceId、已解析 facts、规则阶段结果、当前活动系统时间。
  - 插件不得要求外部业务系统直接调用插件，也不得要求标准事件新增非标准顶层字段。

### 输出

- 注册表查询结果：按 `pluginCode` 和版本范围解析为唯一插件版本，返回能力、支持活动类型和 Schema 摘要。
- 上线前校验结果：通过或结构化拒绝，包含错误码、字段、期望、实际、插件编码、版本范围和可读原因。
- 运行期插件执行结果：
  - `pluginCode`
  - `pluginVersion`
  - `lifecycleStep`
  - `status`：`PASS`、`REJECT`、`SKIP`、`ERROR`
  - `reasonCode`
  - `reason`
  - `contextPatch` 或 `state.plugin.<pluginCode>.*` 快照
  - `candidateResults`，例如候选奖励、抽奖候选、红包命中原因；只能供主流程继续计算或解释
  - `stateChangeIntents`，表示建议或待提交状态变更，不等同于 grant_order/outbox/quota 事实
  - `rollbackToken` 或等价回滚关联信息，用于失败时关联 OnRollback
- 审计记录：注册、升级、禁用、上线固定解析、执行失败、回滚都要记录结构化 payload。

## 细化需求

### 1. 插件注册模型

- 插件必须通过注册表注册后才能被活动引用。
- `pluginCode + version` 必须唯一；同一 `pluginCode` 可注册多个版本，但运行时解析必须得到唯一版本。
- 同一进程或同一注册表中重复注册完全相同的 `pluginCode + version` 必须被拒绝或幂等确认，不允许出现两个不同实现同时宣称相同版本。
- `capabilities` 必须显式声明，主引擎不得通过反射或运行时探测猜测能力。
- `supportedActivityTypes` 必须显式声明；活动 `activityType` 不在集合内时不能创建或上线绑定。
- `configSchema` 用于校验活动里的插件配置；无 Schema 的插件默认不能用于需要配置的活动。
- `stateSchema` 用于限制插件输出到 snapshot/audit 的结构，避免输出不可审计的任意 JSON。

### 2. 生命周期契约

生命周期名称采用任务指定口径：

- `Register`：插件启动或加载时注册元数据、版本、能力和 Schema。
- `ValidateConfig`：活动创建、更新、审批提交或上线前校验时校验插件配置、版本范围、能力和活动类型兼容性。
- `BeforeMatch`：活动匹配前或匹配初期提供玩法上下文或预校验；不得写发奖事实。
- `BeforeReward`：规则通过后、奖励计算前提供候选结果或扩展校验；可拒绝继续发奖。
- `AfterRewardCalculated`：奖励计算完成后、Quota/participation/grant_order/outbox 写入前补充解释、校验候选结果或生成状态变更意图。
- `OnEvent`：处理插件调度或插件内部事件转换后的标准事件上下文；仍必须经过 Event Inbox 幂等。
- `OnRollback`：当插件生命周期之后的主流程失败或需要回滚时执行补偿或记录回滚结果。
- `Disable`：插件被禁用时执行禁用前校验或生成禁用审计，不得主动修改已上线活动状态。

默认执行顺序：

1. Event Inbox 幂等先于正式活动处理。
2. 活动候选匹配读取固定的活动配置版本。
3. 如果活动配置了插件，按 tenantId + activityId + activityVersion + pluginCode + pluginVersionRange 解析并固定具体 `pluginVersion`。
4. `ValidateConfig` 在创建、更新、审批提交和上线前执行；正式事件处理不应首次发现静态配置错误。
5. `BeforeMatch` 在规则字段需要读取 `state.plugin.<pluginCode>.*` 前完成。
6. 通用规则按 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET` 执行。
7. `BeforeReward` 在通用奖励计算前执行。
8. `AfterRewardCalculated` 在奖励结果出来后、Quota 和 grant_order/outbox 写入前执行。
9. Quota、participation、grant_order、outbox 仍由主引擎在自身事务边界内处理。
10. 任一插件步骤失败导致本活动不能继续时，必须不产生 grant_order/outbox 脏写，并触发审计；如果已有插件状态变更意图或临时状态，则通过 `OnRollback` 关联处理。

### 3. 版本兼容性

- 活动配置声明 `pluginCode` 和 `pluginVersionRange`，运行时不能仅按“当前最新版本”浮动执行。
- 上线前必须把版本范围解析为一个具体 `pluginVersion`，并写入活动版本快照或等价审计结果。
- 同一活动配置版本在审批通过后，其插件解析结果必须稳定；插件注册表升级不能改变已审批版本的运行语义。
- 默认兼容规则：同一 major 版本内可按范围选择满足条件的最高 patch/minor；跨 major 默认不兼容，除非活动显式声明范围且插件 Schema 校验通过。
- 如果 `pluginVersionRange` 无可用版本，活动创建或上线前校验必须 BLOCK。
- 如果插件版本被移除、禁用或不再满足能力要求，未上线活动不能上线；已上线活动按禁用策略处理。

### 4. 能力校验

- 活动配置所需能力必须是插件声明能力的子集。
- 规则引用 `state.plugin.<pluginCode>.*` 时，活动必须绑定同一 `pluginCode`，且插件必须具备上下文构建能力。
- 配置了 `BeforeReward` 或 `AfterRewardCalculated` 依赖的活动，插件必须声明对应能力。
- 插件没有声明某生命周期能力时，主流程只能跳过该步骤，不能调用隐式默认实现；如果活动要求该能力则校验失败。
- 能力缺失属于配置错误，应在活动创建、更新、审批提交或上线前 BLOCK。

### 5. 插件禁用策略

- 禁用插件后，新的活动创建、审批提交或上线默认必须 BLOCK。
- 已上线活动遇到禁用插件时按活动固定配置策略处理：
  - `BLOCK`：活动本次处理拒绝继续，记录插件禁用错误，不产生 grant_order/outbox/quota 副作用。
  - `DEGRADE`：仅当插件声明支持降级且活动配置了降级规则时允许继续；降级不得伪造插件命中结果，不得绕过规则、奖励、Quota。
  - `READ_ONLY`：仅允许读取已存在的插件状态或上下文用于解释，不允许执行会改变插件状态的生命周期。
- 未配置禁用策略时默认 `BLOCK`，这是最小安全口径。
- 插件禁用必须记录审计，包含操作人或系统来源、原因、插件编码、版本范围、影响的能力摘要和时间。

### 6. 错误码与审计 payload

默认错误码最小集合：

- `PLUGIN_NOT_FOUND`：插件编码不存在。
- `PLUGIN_VERSION_UNRESOLVED`：版本范围无法解析到具体版本。
- `PLUGIN_VERSION_INCOMPATIBLE`：解析版本与活动要求或 major 兼容规则不匹配。
- `PLUGIN_CAPABILITY_MISSING`：插件缺少活动要求能力。
- `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED`：活动类型不在插件支持范围内。
- `PLUGIN_CONFIG_INVALID`：插件配置不符合 configSchema。
- `PLUGIN_STATE_SCHEMA_INVALID`：插件输出不符合 stateSchema。
- `PLUGIN_DISABLED`：插件已禁用且策略不允许继续。
- `PLUGIN_EXECUTION_FAILED`：生命周期执行失败。
- `PLUGIN_ROLLBACK_FAILED`：回滚处理失败。

审计 payload 至少包含：

- `tenantId`
- `activityId`
- `activityCode`
- `activityVersion`
- `pluginCode`
- `pluginVersion`
- `pluginVersionRange`
- `lifecycleStep`
- `capabilitiesRequired`
- `capabilitiesDeclared`
- `status`
- `errorCode`
- `reason`
- `traceId`
- `eventId` 或 `idempotencyKey`，如发生在事件处理链路
- `operator`，如发生在注册、升级、禁用等管理动作
- `before` / `after` 摘要，如发生注册、升级、禁用或状态回滚

审计不得记录密钥、token、完整 Authorization、外部系统凭据或不可控大对象。

## 业务规则

1. 插件职责边界
   - 插件只能提供玩法上下文、状态计算、候选结果、扩展校验和审计解释。
   - 插件不得直接创建或修改 participation、grant_order、outbox、quota。
   - 插件不得直接发奖、调用外部发奖系统、扣预算、扣库存、确认 callback 或绕过 Event Inbox。

2. 活动上线前校验
   - 配置了 `pluginCode` 的活动必须校验插件存在。
   - 必须校验版本范围可解析为唯一具体版本。
   - 必须校验活动类型兼容。
   - 必须校验活动要求能力均被插件声明。
   - 必须校验 `pluginConfig` 满足 `configSchema`。
   - 必须校验规则字段路径只引用绑定插件的 `state.plugin.<pluginCode>.*`。
   - 任一失败都必须 BLOCK 上线。

3. 运行时固定解析
   - 正式事件处理必须使用活动版本快照中的固定插件版本。
   - 插件升级不能改变已上线活动版本的运行结果。
   - 同一事件、同一活动版本、同一插件版本、同一插件状态快照下，插件输出应可复现。

4. 失败处理
   - 插件在 `BeforeMatch` 或 `BeforeReward` 返回业务拒绝时，本活动不得进入发奖副作用链路。
   - 插件在 `AfterRewardCalculated` 返回错误时，主流程不得写入 grant_order/outbox；已经预留的临时状态必须可回滚或至少有回滚审计。
   - 插件执行异常必须结构化返回，不允许只返回不可解析文本。
   - 插件失败只影响绑定该插件的当前活动，不应影响同一事件下其他普通活动或其他插件活动。

5. 模拟运行
   - 模拟运行应执行与正式处理相同的插件校验和生命周期语义。
   - 模拟运行不得写 event_inbox、participation、grant_order、outbox、quota 或真实插件状态。
   - 模拟输出必须能展示插件上下文、候选结果、拒绝原因、错误码和状态变更意图。

6. 审计与追踪
   - 注册、升级、禁用、执行失败、回滚都必须审计。
   - 插件执行结果必须能从 participation snapshot 或 audit 中追踪。
   - 对于没有生成 participation 的插件拒绝或执行失败，必须至少有 audit 记录可追踪。

## 异常场景

- 活动配置未知 `pluginCode`：创建或上线前拒绝，错误码 `PLUGIN_NOT_FOUND`。
- `pluginVersionRange` 无匹配版本：拒绝，错误码 `PLUGIN_VERSION_UNRESOLVED`。
- 匹配版本跨 major 且未显式允许：拒绝，错误码 `PLUGIN_VERSION_INCOMPATIBLE`。
- 插件存在但不支持当前 `activityType`：拒绝，错误码 `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED`。
- 插件缺少活动要求能力：拒绝，错误码 `PLUGIN_CAPABILITY_MISSING`。
- 插件配置不符合 Schema：拒绝，错误码 `PLUGIN_CONFIG_INVALID`，返回字段级原因。
- 规则引用未绑定插件路径或其他插件路径：拒绝配置，不能等运行时报错。
- 插件被禁用且策略为默认或 `BLOCK`：正式处理拒绝，不产生发奖链路副作用。
- 插件被禁用且策略为 `DEGRADE`，但插件未声明支持降级：拒绝，按 `BLOCK` 处理。
- 插件生命周期 panic、超时或返回不可解析结果：按 `PLUGIN_EXECUTION_FAILED` 处理，记录审计，不生成 grant_order/outbox。
- `OnRollback` 失败：记录 `PLUGIN_ROLLBACK_FAILED`，不得因此补写 grant_order/outbox；需保留人工排障依据。
- 插件输出违反 `stateSchema`：按执行失败处理，不允许将非法状态写入 snapshot/audit。

## 边界条件

- 多租户：插件编码是系统能力编码，但活动、执行和审计必须携带 tenantId；插件不得跨 tenant 读取或修改状态。
- 活动版本：活动审批只对锁定的 `activity.version` 有效；插件版本固定解析也必须绑定该活动版本。
- 未配置插件：普通活动不受插件注册、禁用或升级影响。
- 多插件：本阶段默认一个活动最多绑定一个 ActivityPlugin；多个插件组合不在本需求范围内。
- Schema 兼容：插件升级时，新增可选字段可兼容；删除字段、改变字段含义、改变必填字段或改变输出路径默认视为不兼容。
- 时间与随机性：插件不得用不可审计随机数或本地当前时间破坏幂等；需要随机或当前时间时必须通过可审计上下文或稳定 seed 表达。
- 事务边界：插件不能扩大主引擎事务职责；主引擎在发奖副作用前必须有机会根据插件错误中止。
- 审计大小：插件 snapshot/audit 只保留摘要和必要解释，不应写入无限大 payload。

## 非目标范围

- 不实现具体红包雨、抽奖、翻卡、竞猜等玩法业务逻辑。
- 不实现 Admin 页面或插件管理 UI。
- 不实现分布式插件市场、热加载脚本插件或远程插件沙箱。
- 不允许插件直接访问数据库业务表、直接发奖、直接扣减 quota、直接写 outbox。
- 不改变标准 Event Inbox 幂等模型。
- 不改变通用规则、奖励、Quota、participation、grant_order、outbox 和 callback 的职责。
- 不实现多个插件在同一活动中的编排规则。
- 不为插件新增非标准事件顶层字段或业务专用 controller。

## 风险点

- 边界滑坡风险：插件如果拥有发奖或 quota 权限，会破坏统一账务和对账模型。本需求通过非目标、能力声明和验收强制限制。
- 版本漂移风险：如果活动运行时按最新插件执行，已审批活动可能被升级改变语义。本需求要求上线时固定具体版本。
- 禁用影响风险：禁用插件可能影响已上线活动。本需求默认新活动 BLOCK、已上线活动未配置策略则 BLOCK，并要求审计。
- 审计不足风险：插件错误如果只返回文本会难以排障。本需求要求结构化错误码和 payload。
- Schema 演进风险：插件配置和状态 Schema 不稳定会导致规则字段不可审核。本需求要求 configSchema/stateSchema 和兼容性判断。
- 测试遗漏风险：生命周期失败如果没有覆盖，可能产生 grant_order/outbox 脏写。验收必须覆盖失败无副作用。

## 验收标准

1. 注册和版本
   - 能注册带 `pluginCode`、`version`、`capabilities`、`supportedActivityTypes`、`configSchema`、`stateSchema` 的插件。
   - 相同 `pluginCode + version` 不允许出现两个不同注册结果。
   - 活动配置的版本范围能解析为唯一具体版本，并在上线或活动版本快照中固定。

2. 上线前校验
   - 插件不存在时活动创建、审批提交或上线前校验被 BLOCK。
   - 版本不兼容或无法解析时被 BLOCK。
   - 能力缺失时被 BLOCK。
   - 活动类型不兼容时被 BLOCK。
   - 插件配置不符合 Schema 时被 BLOCK，并返回结构化字段原因。

3. 生命周期
   - `BeforeMatch`、`BeforeReward`、`AfterRewardCalculated` 的执行顺序符合主流程边界。
   - 插件业务拒绝不会生成 grant_order/outbox，也不会扣 quota。
   - 插件执行失败不会产生 grant_order/outbox 脏写。
   - 需要回滚时能记录 `OnRollback` 结果；回滚失败有审计。

4. 禁用策略
   - 插件禁用后，新活动不能上线。
   - 已上线活动默认 `BLOCK`。
   - `DEGRADE` 或 `READ_ONLY` 只有在活动配置且插件声明支持时才允许。
   - 禁用动作和受影响执行都有 audit。

5. 审计和追踪
   - 注册、升级、禁用、执行失败、回滚均生成 audit。
   - 插件执行结果能从 participation snapshot 或 audit 追踪到 pluginCode、pluginVersion、lifecycleStep、status、reasonCode、traceId。
   - 审计不包含 secret/token/完整 Authorization。

6. 隔离性
   - 未配置插件的普通活动保持原行为。
   - 一个插件失败不影响同一事件下其他无关活动继续处理。
   - 插件不能直接修改 quota、grant_order、outbox。

7. 测试命令
   - 单元测试覆盖插件不存在、版本不兼容、能力缺失、执行失败、禁用插件。
   - `go test ./...` 通过。

## 待确认问题

无阻塞待确认问题。

默认最小可验收口径已明确：语义化版本、上线固定解析、一个活动一个插件、禁用默认 BLOCK、插件失败无发奖副作用、所有关键动作审计。后续如需支持多插件编排、远程插件、热加载插件或复杂降级策略，应另开需求。

## Score

```text
需求清晰度        18/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      18/20
约束一致性        18/20
总分              92/100
```

扣分说明：

- 需求清晰度扣 2 分：任务未指定最终配置字段在现有 JSON 中的精确落点，本轮按受控插件配置区给出最小口径，具体字段映射留给 Dev 在不改变语义的前提下沿用项目现有风格。
- 业务规则完整扣 1 分：降级策略的具体业务表现依赖未来具体插件，本轮限定为必须显式配置且插件声明支持。
- 边界异常扣 1 分：回滚失败后的人工处理流程未展开到运维 runbook，当前只要求结构化审计和无发奖脏写。
- 验收标准扣 2 分：不指定具体测试文件和实现结构，符合 Analysis Agent 不设计代码改法的约束。
- 约束一致性扣 2 分：相邻插件状态/调度需求尚未完成，本轮只按任务和现有文档建立兼容契约，避免提前定义具体状态存储实现。

## Decision

```text
PASS
```

需求：2026-05-29-activityplugin-lifecycle
