# Analysis

## 需求

`2026-05-24-113929`：活动主配置模型。

本轮只做需求分析和需求细化，不读取代码、不写代码、不改代码、不提交。分析依据来自：

- `tasks/2026-05-24-113929/task.md`
- `tasks/2026-05-24-113929/review.md`
- 上一轮 `tasks/2026-05-24-113929/analysis.md`
- `requirements/backlog.json`
- `README.md`
- `CLAUDE.md`
- `docs/architecture.md`
- `docs/api.md`
- `docs/data-model.md`
- `docs/roadmap.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

## Review 反馈处理

`review.md` 当前是模板态，包含 `P0:`、`P1:`、`P2:`、`Open Questions 待填写` 和 `PASS / BLOCK` 占位，但没有具体打回意见、风险描述、行号或可执行问题。

- P0：未填写具体问题。本轮不按实际 P0 处理，但补齐主配置字段、校验、边界和验收口径，避免空分析再次被打回。
- P1：未填写具体问题。本轮补齐范围、输入输出、业务规则和异常场景。
- P2：未填写具体问题。本轮补齐非目标、风险和评分。
- Open Questions：仅为“待填写”占位，没有实际问题。本轮主动列出仍需业务确认的问题，但这些问题不阻塞本任务的最小主配置口径。
- Decision：模板未给出有效 PASS/BLOCK。本轮以本分析的评分和 Decision 为准。

## Analysis 自检修正

上一轮 `analysis.md` 仍是模板态，存在 `待填写`、`Score 0`、`PASS / BLOCK` 占位，按流程视为未完成分析。

- BLOCK 原因：需求未细化。本轮已补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题。
- 扣分项：需求清晰度、范围可控、风险可控、验收标准、架构一致性均未填写。本轮按现有 task、backlog、API、数据模型和设计文档推导最小可实现范围。
- 待确认问题：上一轮未列。本轮补充不阻塞最小范围的待确认项。
- 错误假设：上一轮没有实际分析内容，不存在可追溯错误假设。本轮明确不把规则、奖励、预算、审批、页面展示等相邻需求纳入本任务。

## 背景目标

ExMarketing 是独立活动营销中台，要求与具体业务系统解耦：只维护营销系统自己的配置、状态、记录、幂等、预算和审计，不连接外部业务库，不内置业务强模型。

本任务来源于“22-独立活动营销中台设计”的活动主表部分，目标是完善 `marketing_activity` 的主配置能力，使活动基础配置可创建、查询、更新，并对时间和 `timezone` 做明确校验。

本任务是活动主配置模型，不是完整活动配置统一模型。规则、奖励、预算库存、审批、模拟运行、页面展示入口等能力已有独立 backlog 或已完成任务，本任务只定义主表字段和主配置管理语义。

## 参与方

- 运营 / 活动配置人员：创建、查询、更新活动主配置。
- 管理后台 / Admin API：承载活动主配置输入输出。
- 活动引擎：消费活动主配置中的租户、状态、时间、优先级、互斥叠加和扩展配置快照。
- Review / Dev / Test Agent：按本需求边界完成审核、实现和测试。
- 审计 / 运维人员：后续依赖主配置版本、时间、状态和更新时间排查活动配置问题。

## 输入输出

### 创建活动输入

创建活动主配置必须覆盖以下字段：

- `tenantId`：租户 ID，必填。
- `activityCode`：活动编码，必填。
- `activityName`：活动名称，必填。
- `activityType`：活动类型，必填。
- `status`：活动状态，必填或使用明确默认值。
- `startTime`：活动开始时间，必填。
- `endTime`：活动结束时间，必填。
- `timezone`：活动时区，必填。
- `priority`：活动优先级，整数，未传时默认为 `0`。
- `exclusiveGroup`：互斥组，可为空字符串。
- `stackable`：是否允许叠加命中，布尔值，未传时默认为 `false`。
- `version`：配置版本，创建初始语义为 `1`。
- `config` / `configJson`：活动扩展配置，可省略或为空对象。

### 查询活动输出

查询活动必须返回活动主配置的完整可审核视图，至少包括：

- 系统生成的 `id`。
- 上述全部主配置字段。
- `createdAt`、`updatedAt`。
- 当前配置版本 `version`。

列表查询至少应能返回活动主配置列表；如已有租户过滤约定，应保持按 `tenantId` 查询和租户隔离的方向，不在本任务扩展复杂检索。

### 更新活动输入输出

更新活动主配置时允许修改：

- `activityName`
- `status`
- `startTime`
- `endTime`
- `timezone`
- `priority`
- `exclusiveGroup`
- `stackable`
- `config`

更新后必须返回更新后的完整活动主配置视图，并推进 `version` 与 `updatedAt`。`tenantId`、`activityCode`、`activityType`、`id` 是活动身份字段，默认不允许通过普通更新修改；如未来要支持编码或类型迁移，应另立需求并带审批/审计约束。

## 业务规则

### 字段规则

- `tenantId`、`activityCode`、`activityName`、`activityType` 必须为非空字符串，去除首尾空白后不可为空。
- `activityCode` 在同一 `tenantId` 下应唯一；不同租户可使用相同活动编码。
- `activityName` 用于后台识别和运营展示，不参与规则判断。
- `activityType` 是活动类型标识，用于分类、插件兼容或后续配置校验；本任务不新增具体业务类型枚举。
- `status` 允许值为 `DRAFT`、`SCHEDULED`、`ACTIVE`、`PAUSED`、`ENDED`。
- `priority` 越大优先级越高，用于后续候选活动排序和互斥组处理。
- `exclusiveGroup` 为空表示不加入互斥组；非空表示同组活动后续按优先级和叠加策略处理。
- `stackable=false` 表示同互斥组默认只允许命中一个；`stackable=true` 表示允许与其他活动叠加。完整互斥执行不是本任务范围。
- `version` 创建时为 `1`；每次成功更新主配置应递增。客户端不得创建历史版本或跳号版本。
- `config_json` 只能保存活动主配置扩展信息，不得替代规则、奖励、预算、发奖、审批和事件幂等模型。

### 时间和时区规则

- `startTime`、`endTime` 必须是合法 RFC3339 时间，且必须包含时区偏移，例如 `Z` 或 `+08:00`。
- `startTime < endTime`，相等或开始晚于结束均拒绝。
- `timezone` 必须显式填写，且必须是合法 IANA 时区名或系统已明确支持的 `UTC`。推荐示例：`UTC`、`Asia/Shanghai`、`America/New_York`。
- 活动时间存储和比较应有统一语义：输入时间表示绝对时刻，`timezone` 表示活动日历语义和后台展示/校验语义。
- 不允许仅传本地无时区时间，例如 `2026-05-24T12:00:00`。
- 本任务只要求单活动时间有效性校验，不要求实现活动时间冲突校验；时间冲突属于上线前校验任务。

### 状态规则

- `DRAFT`：草稿，可保存未上线活动；是否允许缺规则/奖励不由本任务决定。
- `SCHEDULED`：已排期，需有合法时间窗口。
- `ACTIVE`：活动在线状态，需有合法时间窗口。
- `PAUSED`：暂停状态，保留配置但不应被活动匹配视为在线。
- `ENDED`：结束状态，保留历史配置。
- 本任务不定义自动从 `SCHEDULED` 切到 `ACTIVE`、从 `ACTIVE` 切到 `ENDED` 的调度行为；如需要自动状态流转，应另立任务。

### config_json 最小口径

`config_json` 最小要求：

- 可省略、为 `null` 或 JSON object。
- 如果提供，必须是合法 JSON object。
- 不允许字符串、数组、数字、布尔作为顶层 `config_json`。
- 本任务不强制定义展示入口、人群、渠道等细分 Schema；这些属于“活动展示与入口配置”等相邻需求。
- `config_json` 中如包含时间、URL、人群、入口等字段，本任务只保证作为扩展配置保存和读回，不赋予执行语义。

## 边界条件

- 创建相同 `tenantId + activityCode` 的活动应失败，避免同租户编码冲突。
- 不同 `tenantId` 下相同 `activityCode` 可以共存。
- 传入未知 `status` 应失败。
- 传入非法 RFC3339 时间、无时区时间、非法日期应失败。
- 传入非法 `timezone` 应失败。
- `startTime >= endTime` 应失败。
- `priority` 非整数应失败。
- `stackable` 非布尔值应失败。
- `config_json` 顶层不是 object 时应失败。
- 更新不存在的活动应返回明确未找到。
- 更新时若提供版本控制字段，版本冲突应返回明确冲突；如本阶段不做乐观锁，也必须在非目标中明确。
- 更新身份字段 `tenantId`、`activityCode`、`activityType`、`id` 应被拒绝或忽略并有明确 API 行为，不能静默造成身份漂移。

## 异常场景

- 请求体不是合法 JSON：返回 `400 Bad Request`。
- 缺少必填字段：返回 `400 Bad Request`，错误信息指出字段名。
- 字段类型错误：返回 `400 Bad Request`，错误信息指出字段路径。
- 活动编码重复：返回 `409 Conflict` 或项目已有的明确冲突错误，不应返回通用 `500`。
- 活动不存在：查询或更新返回 `404 Not Found`。
- 时间和时区校验失败：返回 `400 Bad Request`，错误信息需说明是时间格式、时间顺序还是 timezone 非法。
- 更新版本冲突：如实现乐观锁，返回 `409 Conflict`；如不实现乐观锁，则本任务验收不要求并发版本冲突处理。

## 非目标范围

本任务不包含：

- `marketing_rule` 配置 Schema、规则字段路径、规则执行器新增。
- `marketing_reward` 配置 Schema、奖励计算、发奖方式扩展。
- `triggerEvents`、入口展示、人群圈选的完整配置 Schema 扩展；如已有字段可保持读写，但不作为本任务核心新增。
- 预算、库存、用户次数 quota。
- 参与记录、发奖单、Outbox、DLQ、发奖回调。
- 自动状态调度、定时上下线。
- 活动时间冲突检测、互斥组冲突检测、高危配置审批。
- 管理后台复杂页面。
- 插件体系新增。
- 外部业务系统模型接入。

## 风险

- 时间语义风险：`startTime/endTime` 与 `timezone` 容易混淆。需求明确要求时间是绝对时刻，`timezone` 是活动日历和展示语义，避免用服务器本地时区隐式解释。
- 配置膨胀风险：`config_json` 容易被用于承载规则、奖励、预算等执行语义。本任务限定它只做主配置扩展保存和读回。
- 兼容风险：若现有创建活动接口已经同时接受规则、奖励等字段，本任务不能破坏现有普通活动创建流程。
- 租户隔离风险：活动编码唯一性必须按租户维度判断，不能做全局唯一导致多租户冲突。
- 更新并发风险：版本字段已进入模型，但本任务是否实现乐观锁需按现有 API 能力决定；最小口径要求更新成功递增版本。

## 验收标准

- 可以创建只包含活动主配置必填字段和合法默认字段的活动。
- 创建成功后查询列表或详情能读回 `tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`version`、`config_json`。
- 可以更新允许修改的主配置字段，更新后查询可读回新值，`version` 递增，`updatedAt` 更新。
- 同一租户重复 `activityCode` 创建失败；不同租户相同 `activityCode` 不冲突。
- 非法 `status` 创建或更新失败。
- 缺少 `timezone`、非法 `timezone`、无时区时间、非法时间、`startTime >= endTime` 均失败，错误信息可定位原因。
- `config_json` 为空或 object 时可接受；顶层为数组、字符串、数字或布尔时失败。
- 普通活动现有创建、查询能力不被破坏；如接口已支持规则和奖励，仍可兼容原有字段。
- 文档或 API 说明同步体现活动主配置字段、时间和 timezone 校验规则。
- 测试至少覆盖成功创建、成功更新、重复编码、非法状态、非法时间顺序、缺失/非法 timezone、非法 config 顶层类型。

## 待确认问题

- 更新接口的形态是否固定为 `PUT /admin/v1/activities/{id}`、`PATCH /admin/v1/activities/{id}`，还是沿用已有管理接口约定？现有材料只明确了创建和列表查询，本问题不阻塞主配置需求，但会影响实现入口。
- 更新是否必须要求客户端传入 `version` 做乐观锁？最小口径只要求服务端成功更新时递增版本；强乐观锁可另行确认。
- `timezone` 是否只接受 IANA 时区名和 `UTC`，还是允许固定偏移如 `+08:00`？本分析建议只接受 IANA/UTC，避免夏令时和日历语义歧义。
- `activityCode`、`activityName`、`activityType`、`exclusiveGroup` 的最大长度是否严格沿用设计稿 SQL：64/128/64/64？如无现有约束，建议按设计稿执行。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          18/20
验收标准明确      18/20
架构一致性        19/20
总分              92/100
```

## Decision

```text
PASS
```

需求细节已达到可审核、可验收状态。待确认问题不阻塞最小活动主配置模型开发，可在 Dev/Test 阶段按本分析的最小口径推进。
