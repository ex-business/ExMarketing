# Analysis

## 需求 ID

`2026-05-24-113929-6`

## 标题

上线前配置校验

## Review 反馈处理

已读取 `tasks/2026-05-24-113929-6/review.md`。该文件仍为 Review Agent 模板，未填写实际 P0 / P1 / P2、Open Questions 或明确打回原因。

| Review 项 | 处理 |
|---|---|
| `P0:` | 无实际内容，不构成阻塞。 |
| `P1:` | 无实际内容，不构成阻塞。 |
| `P2:` | 无实际内容，不构成阻塞。 |
| `Open Questions: 待填写。` | 模板占位，无具体问题。本轮主动补齐本需求的待确认问题和默认最小可验收口径。 |
| `Decision: PASS / BLOCK` | 模板占位，未形成 Review 结论。本轮按 Analysis 评分规则重新给出结论。 |

## Analysis 自检修正

已读取上一版 `tasks/2026-05-24-113929-6/analysis.md`。上一版为模板态，存在 Analysis 自身 BLOCK 条件。本轮逐条修正：

| 上一版问题 | 修正 |
|---|---|
| `现状`、`目标理解`、`方案`、`改动范围`、`风险点`、`验收方式` 均为“待填写”。 | 已补充需求说明、背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准和待确认问题。 |
| `Score = 0`，低于 80。 | 已基于 `task.md`、项目文档和设计文档重新细化，评分更新为 90。 |
| `Decision = PASS / BLOCK` 未决。 | 已给出明确 `Decision PASS`。 |
| 未定义 Review 可审核的校验项和输出结构。 | 已明确逐项校验结果必须返回 `PASS / WARN / BLOCK`，并给出概率、预算、库存、时间冲突、互斥组、规则字段、JSON Schema、高危配置的最小可验收口径。 |
| 未处理“不能等待 Owner 决策”的要求。 | 对高危阈值等不唯一材料给出默认保守口径：无法确定预算暴露或存在自动资金类发奖且缺少预算时阻断或标记高危；高危配置在本任务内以 `BLOCK` 阻止上线，不实现审批流。 |

## 资料依据

- `tasks/2026-05-24-113929-6/task.md`：要求实现上线前校验，覆盖概率总和、奖励金额是否超预算、库存、活动时间冲突、互斥组冲突、规则字段、JSON Schema、高危配置；验收为返回逐项 `PASS/BLOCK`，不允许高危配置上线。
- `README.md`、`CLAUDE.md`：ExMarketing 只维护活动配置、状态、幂等、预算和审计，不连接外部业务库，不直接改余额；金额、预算、库存禁止使用浮点语义；模拟运行必须无副作用。
- `docs/architecture.md`：活动链路为 `Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox`；上线前校验应服务于该配置链路，不改变发奖链路职责。
- `docs/api.md`、`docs/data-model.md`：已定义活动、规则、奖励、quota、互斥、入口、模拟运行、字段路径、Decimal、targetRef、grant mode/channel、quota 缺省无限制等语义。
- `docs/roadmap.md`：活动配置 JSON Schema 校验、上线前模拟报告、高危二级审批、互斥组完整实现、概率与库存上线前校验属于生产化前关键能力。
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`：明确“上线前必须做校验：概率总和、奖励金额是否超预算、库存、活动时间是否冲突、互斥组是否冲突、规则字段是否存在、JSON Schema 是否合法、是否存在高危配置、是否需要二级审批”。

## 背景目标

上线前配置校验是活动从草稿或待发布状态进入 `SCHEDULED` / `ACTIVE` 前的安全闸门。它的目标不是替代运行时 quota 原子扣减，也不是替代审批流，而是在配置上线前用可解释、可展示、可审计的结果提前阻断明显危险配置。

本需求的最小目标：

- 对单个活动完整配置快照执行上线前校验。
- 返回逐项校验结果，每项明确 `PASS` 或 `BLOCK`，允许补充 `WARN` 作为非阻断提示。
- 任一阻断项存在时，活动不得上线。
- 高危配置在审批流未落地前按阻断处理，不允许直接上线。
- 校验必须复用项目已定义的活动、规则、奖励、quota、互斥和标准字段语义，不引入业务专用模型。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 活动运营 / 后台配置人员 | 提交活动上线前能看到每一项是否通过、失败字段和修复方向。 |
| ExMarketing Admin 边界 | 接收活动 ID 或配置快照，返回结构化校验报告，并在状态变更前阻断失败配置。 |
| 活动引擎 / 配置模型 | 提供活动、规则、奖励、quota、互斥和插件注册信息的当前配置语义。 |
| Review / Dev / Test Agent | 按本文件验证需求是否可实现、可测试、可验收。 |
| 后续审批流任务 | 可复用高危识别结果，但审批状态机不属于本任务。 |

## 输入

上线前校验对象为一个活动的完整配置快照，至少包括：

- 活动主配置：`id`、`tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`pluginCode`、`version`、`configJson`。
- 规则配置：`rules[].ruleCode`、`rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version`。
- 奖励配置：`rewards[].rewardCode`、`rewardType`、`calculateType`、`rewardConfig`、`grantMode`、`grantChannel`、`enabled`、`version`。
- Quota 配置：当前活动已配置的 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 上下文配置：同一 `tenantId` 下已存在且状态为 `SCHEDULED` / `ACTIVE` 的活动列表，用于时间和互斥冲突判断。
- 目标动作：至少区分“保存草稿校验”和“上线前校验”。本需求验收重点为上线前校验。

不要求输入真实用户事件样例；事件样例驱动的模拟运行解释器属于后续 `2026-05-24-113929-7`。

## 输出

校验报告必须是结构化结果，满足后台展示和自动化测试：

- `activityId`、`tenantId`、`activityCode`、`version`。
- `checkedAt`：校验时间。
- `decision`：`PASS` / `BLOCK`。可额外包含 `WARN`，但最终上线判断只看是否存在 `BLOCK`。
- `allowPublish`：布尔值；仅当无 `BLOCK` 时为 `true`。
- `items[]`：逐项校验结果。
  - `code`：稳定机器码，例如 `PROBABILITY_TOTAL_INVALID`。
  - `name`：人类可读名称。
  - `category`：`SCHEMA` / `RULE` / `REWARD` / `BUDGET` / `STOCK` / `TIME` / `EXCLUSIVE` / `HIGH_RISK`。
  - `result`：`PASS` / `WARN` / `BLOCK`。
  - `field`：出错字段路径，例如 `rewards[0].rewardConfig.items`。
  - `reason`：可展示原因。
  - `expected`：期望值或规则。
  - `actual`：实际值摘要。
  - `relatedResourceIds`：涉及的其他活动、quota 或 rewardCode，可为空。
- `summary`：`passCount`、`warnCount`、`blockCount`、`highRiskCount`。

输出不得只返回单个错误字符串；即使发现多个失败项，也应尽量返回所有可独立判断的阻断项。

## 业务规则

### 1. 总体决策

- 任一校验项为 `BLOCK`，整体 `decision=BLOCK`，`allowPublish=false`。
- 所有必需校验项为 `PASS`，且仅存在非阻断 `WARN`，整体可为 `PASS`，`allowPublish=true`。
- 本任务不实现审批流；凡命中高危配置，最小可验收口径为生成 `HIGH_RISK_*` 阻断项，整体 `BLOCK`。
- 草稿保存可以返回 `BLOCK` 报告但不强制禁止保存；活动进入 `SCHEDULED` / `ACTIVE` 前必须无 `BLOCK`。

### 2. JSON Schema 校验

上线前必须重新执行活动、规则、奖励、quota 的 Schema 校验，不能只依赖创建时校验。

阻断条件：

- 活动基础字段缺失或非法：`tenantId`、`activityCode`、`activityName`、`activityType`、`triggerEvents`、`startTime`、`endTime`、`timezone`。
- `startTime >= endTime`。
- `timezone` 不是可识别 IANA 时区或 `UTC`。
- `configJson` 顶层字段超出允许范围：`entry`、`display`、`audience`、`metadata`。
- 非法枚举：活动状态、规则阶段、规则类型、奖励类型、计算方式、发奖模式、发奖通道、quota 类型或 scope。
- Decimal、整数、布尔、数组、对象、URL 等字段类型不符合现有文档语义。
- `AUTO + HTTP/WEBHOOK` 缺少 `targetRef`，或 `targetRef` 不是 `http(s)` URL。
- `AUTO + MQ` 的 `targetRef` 不是允许的 destination URI，或包含 broker 地址、账号、密码、query、fragment。
- 当前文档明确拒绝的 `MANUAL`、`MANUAL_EXPORT` 或未注册可执行能力，作为上线阻断。

### 3. 规则字段校验

上线前必须检查所有规则，包括 `enabled=false` 的规则；禁用只影响运行时跳过，不放宽配置合法性。

阻断条件：

- `rulePhase` 不在 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT` 内；`BUDGET` 不是合法 `rulePhase`。
- `ruleType` 未注册可执行 evaluator。当前最小可上线口径为只允许可执行 `CONDITION`。
- `CONDITION.logic` 不是 `AND` / `OR`。
- `conditions` 为空或不是数组。
- `op` 不在 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN` 内。
- 比较操作符对应值不能按 Decimal 或对应类型解释。
- `IN` / `NOT_IN` 的值不是非空数组。
- 字段路径不在允许范围：标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。
- 规则引用 `facts.<factCode>.*`，但同租户没有 enabled fact provider。
- 规则引用 `state.plugin.<pluginCode>.*`，但活动未绑定该插件或插件未注册。
- 出现非标准顶层业务字段，例如 `orderAmount`、`deposit.amount`、`user.vipLevel`。

### 4. 概率总和校验

适用于 `calculateType=PROBABILITY` 的 enabled reward。

默认最小可验收口径：

- `items` 必须非空。
- 每个 item 必须有稳定唯一的 `itemCode`。
- 每个 item 的 `weight` 必须可解析为 Decimal，且 `weight >= 0`。
- 至少一个 item 的 `weight > 0`。
- 当前配置口径按百分比处理，所有生效 item 的 `weight` 总和必须等于 `100.0000`。
- 如果 item 表示空奖，必须显式配置为一个 item，不能靠“缺省剩余概率”隐式表达。
- item 的金额、权益数量或空奖标记必须可解释；既不是空奖也没有实际奖励字段时阻断。

失败示例机器码：

- `PROBABILITY_ITEMS_EMPTY`
- `PROBABILITY_ITEM_CODE_DUPLICATED`
- `PROBABILITY_WEIGHT_INVALID`
- `PROBABILITY_TOTAL_INVALID`
- `PROBABILITY_ITEM_REWARD_INVALID`

### 5. 奖励金额是否超预算

上线前校验要评估活动的最大可暴露预算风险，不替代运行时 quota 原子扣减。

默认最小可验收口径：

- 金额类 `BONUS` / `CASH` 必须按 Decimal 评估。
- `FIXED`：单次暴露为 `amount`。
- `RATIO`：如果有 `maxAmount`，单次暴露为 `maxAmount`；没有 `maxAmount` 且 `baseField` 无上限时，无法确定最大暴露，阻断。
- `LADDER`：单次暴露为所有阶梯输出中的最大金额；阶梯重叠或无可解释上限时阻断。
- `RANDOM`：单次暴露为 `maxAmount`。
- `PROBABILITY`：单次暴露为所有 item 中最大实际金额。
- `POOL_SPLIT`：单次暴露默认为 `poolAmount`；缺少 `poolAmount` 或无法确定奖池上限时阻断。
- 多个 enabled 金额 reward 可在同一次参与同时发放时，单次暴露取合计。
- 若已配置 `BUDGET + ACTIVITY + TOTAL`，且 `availableBudget = totalValue - usedValue - frozenValue` 小于单次最大暴露，阻断。
- 若未配置 `BUDGET`，但存在 `AUTO` 发放的金额类 reward，按高危配置阻断。
- 若预算 Decimal 非法、为负、或 quota scope 不是 `ACTIVITY + TOTAL`，阻断。

说明：现有运行语义允许缺少 quota 表示无限制，但上线前对自动资金类活动必须采用更保守口径；无限预算不能默认直接上线。

### 6. 库存校验

适用于所有 enabled 且可能产生实际权益的 reward，包括非金额权益。

默认最小可验收口径：

- 每个 enabled reward 的库存 quota 如存在，必须是 `STOCK + ACTIVITY + rewardCode`。
- `totalValue`、`usedValue`、`frozenValue` 必须可解析且非负。
- `availableStock = totalValue - usedValue - frozenValue` 不得为负。
- 对 `GIFT`、`COUPON`、`DRAW_CHANCE`、`MEMBERSHIP`、`PROBABILITY`、`RANDOM`、明确库存型奖品，缺少 `STOCK` quota 或可用库存为 0 时阻断。
- 对普通无限量金额类 reward，缺少 `STOCK` 可以返回 `WARN`，但不能影响预算高危判断。
- `PROBABILITY` 中如果不同 item 对应不同实物、券模板或权益池，本任务最小口径要求每个可能实际发放的 item 都能映射到可解释库存；无法映射时阻断。

### 7. 活动时间冲突校验

冲突检查限定在同一 `tenantId`。

阻断条件：

- 当前活动目标状态为 `SCHEDULED` / `ACTIVE`，且与同租户已存在 `SCHEDULED` / `ACTIVE` 活动时间窗口重叠。
- 双方 `triggerEvents` 有交集。
- 双方面向同一入口或入口未限制，导致同一标准事件可能同时命中。
- 双方 `exclusiveGroup` 相同且非空，且存在不可叠加配置。
- 双方优先级相同或无法稳定决定唯一候选。

非阻断条件：

- 时间窗口仅首尾相接，不视为重叠。
- 不同 `tenantId` 不比较。
- `triggerEvents` 无交集时不阻断。
- 明确 `stackable=true` 且业务规则允许叠加时，不按互斥时间冲突阻断，但仍可返回提示。

时间比较必须基于实际时间点，不得仅比较本地日期字符串；`timezone` 用于解释活动本地日和展示语义。

### 8. 互斥组冲突校验

上线前校验的是配置层面的潜在冲突，不查询历史 participation。

阻断条件：

- 同租户、同非空 `exclusiveGroup` 的两个或多个可上线活动，时间窗口重叠，`triggerEvents` 有交集。
- 任一相关活动 `stackable=false` 或未显式开启叠加。
- `priority` 相同，或缺少稳定 tie-breaker 导致无法解释谁优先。
- 当前活动配置可能导致同一事件同一用户命中多个不可叠加活动。

通过条件：

- `exclusiveGroup` 为空，不参与互斥组冲突。
- 所有关联活动明确 `stackable=true`，且时间/事件重叠被业务允许。
- 有明确稳定优先级，且运行时同一事件候选裁剪能够只保留一个活动；此时可返回 `PASS` 或 `WARN`，但报告需列出相关活动。

### 9. 高危配置校验

由于审批流是后续独立任务，本任务对高危配置采用保守阻断口径：发现高危即 `BLOCK`，不允许上线。

默认高危项：

- `AUTO` 发放 `CASH` 或 `BONUS`。
- 自动资金类 reward 未配置 `BUDGET + ACTIVITY + TOTAL`。
- 单次最大金额暴露无法确定。
- `RATIO` 缺少 `maxAmount`。
- `POOL_SPLIT` 缺少可确定 `poolAmount`。
- `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 等结果不完全固定的资金类奖励。
- 活动总预算超过默认保守阈值。若系统未配置阈值，本任务默认阈值为 `100000.0000`，仅用于上线前高危阻断的最小可验收口径。
- 单次最大奖励超过默认保守阈值。若系统未配置阈值，本任务默认阈值为 `1000.0000`。
- 变更已 `ACTIVE` / `SCHEDULED` 活动的奖励、预算、库存、互斥组、时间窗口、发奖目标。
- `targetRef` 指向未知或不允许的协议，或含凭据、query、fragment 等敏感/不稳定信息。
- 规则移除关键限制：用户次数、每日次数、风险等级、预算、库存相关配置由有到无。

默认阈值依据：现有材料未给出唯一阈值；为避免等待 Owner 决策，本轮给出保守可测阈值，并要求未来接入审批或运营阈值配置时替换默认值。

### 10. 状态变更约束

- 活动从 `DRAFT` / `PAUSED` / 可编辑状态进入 `SCHEDULED` / `ACTIVE` 前必须通过上线前校验。
- 直接创建 `ACTIVE` 活动也必须执行上线前校验。
- 已上线活动修改关键字段后再次上线或保持上线前，必须重新校验。
- 校验报告只对当前活动版本有效；配置版本变化后旧报告不得作为通过依据。

## 边界条件

- 多租户隔离：时间冲突、互斥冲突、fact provider、quota 和高危阈值均限定同一 `tenantId`。
- 缺少 quota：运行时可表示无限制；上线前按 reward 风险分类处理，资金类和库存型活动不能默认放行。
- 零金额奖励：仍需 Schema 合法；如无实际权益，不产生预算/库存阻断，但应返回说明。
- 非金额权益：`reward_amount=0.0000` 不代表无风险；必须校验权益数量、库存和外部引用。
- 插件活动：`pluginCode` 必须注册且与 `activityType` 兼容；插件只提供上下文，不得绕过规则、预算、库存和发奖链路。
- 已禁用 reward / rule：仍需配置合法，但预算、库存、高危暴露可只统计 enabled 项。
- 并发不是上线前校验要解决的问题；运行时超发仍由 quota 原子扣减保证。
- 模拟运行无副作用；本任务不要求输入事件样例或输出命中解释。

## 异常场景

| 场景 | 最小结果 |
|---|---|
| 活动不存在 | 返回不可校验错误，整体 `BLOCK`。 |
| 活动 tenant 与请求上下文不一致 | `BLOCK`，不得跨租户校验。 |
| 配置 JSON 解析失败 | `SCHEMA_JSON_INVALID`，定位字段，整体 `BLOCK`。 |
| Decimal 解析失败 | `DECIMAL_INVALID`，定位字段，整体 `BLOCK`。 |
| 读取同租户活动列表失败 | 无法判断冲突，整体 `BLOCK`。 |
| 读取 quota 失败 | 无法判断预算/库存，整体 `BLOCK`。 |
| 读取 fact provider / plugin registry 失败 | 依赖字段无法验证，整体 `BLOCK`。 |
| 校验过程中发现多个独立问题 | 尽量全部返回，不因第一个错误提前只返回单项。 |
| 已有活动数据不完整 | 对受影响校验项返回 `BLOCK` 或保守 `WARN`；涉及资金或互斥时必须 `BLOCK`。 |

## 非目标范围

- 不实现活动审批状态机、二级审批记录、审批角色和审批通过后解锁上线；这些属于 `2026-05-24-113929-8`。
- 不实现模拟运行解释器或事件样例驱动预览；这些属于 `2026-05-24-113929-7`。
- 不新增真实钱包、优惠券、积分、礼品、会员系统能力。
- 不改变运行时 quota 原子扣减、grant_order、outbox、callback 的业务语义。
- 不查询外部业务数据库或外部用户画像库；规则字段只按标准事件、facts、state 和插件上下文校验。
- 不设计具体代码包、函数、数据库迁移或前端页面交互。
- 不要求跨历史 participation 的生命周期级互斥；本任务只做配置层潜在冲突判断。

## 风险

- 高危阈值缺少产品配置，默认阈值可能偏保守，可能阻断部分运营活动；但符合“高危配置不得上线”的安全目标。
- 预算暴露对 `RATIO`、`POOL_SPLIT`、外部权益估值依赖上限配置；缺少上限只能阻断，可能需要运营补配置。
- 时间和互斥冲突依赖已存在活动数据完整性；历史脏配置可能导致保守阻断。
- 规则字段依赖 fact provider 和 plugin registry；相关配置不完整时必须阻断，可能暴露前置配置缺口。
- 如果未来审批流落地，本任务的 `HIGH_RISK` 阻断结果需要与审批通过状态组合，避免重复阻断已审批版本。

## 验收标准

1. 合法低风险活动执行上线前校验，返回 `decision=PASS`、`allowPublish=true`，并包含 Schema、规则字段、奖励、预算、库存、时间、互斥、高危等逐项结果。
2. `PROBABILITY` 奖励的概率 item 总和不等于 `100.0000` 时，返回 `PROBABILITY_TOTAL_INVALID`，该项 `BLOCK`，整体 `BLOCK`。
3. 概率 item 为空、权重为负、itemCode 重复、空奖未显式配置时，返回可定位字段的 `BLOCK`。
4. `RATIO` 金额奖励缺少 `maxAmount`，且无法确定单次最大暴露时，返回预算或高危 `BLOCK`。
5. 已配置 `BUDGET + ACTIVITY + TOTAL` 但可用预算小于单次最大暴露时，返回预算不足 `BLOCK`，说明 expected/actual。
6. 自动资金类 reward 缺少预算 quota 时，返回 `HIGH_RISK_MISSING_BUDGET`，整体 `BLOCK`。
7. 库存型 reward 缺少 `STOCK + ACTIVITY + rewardCode` 或可用库存为 0 时，返回库存 `BLOCK`。
8. 非库存普通金额类 reward 缺少库存 quota 不阻断，但报告中可返回 `WARN`。
9. 活动 `startTime >= endTime`、timezone 非法、config 顶层未知字段、非法 targetRef 均返回 Schema `BLOCK`。
10. `rulePhase=BUDGET`、未知 `ruleType`、非法 op、空 conditions、非标准顶层字段路径、缺失 fact provider 或未注册 plugin 字段引用均返回规则字段 `BLOCK`。
11. 同租户、同互斥组、时间重叠、事件重叠且不可叠加的活动上线校验返回互斥冲突 `BLOCK`，并列出相关活动 ID。
12. 不同租户活动不互相产生时间或互斥冲突。
13. 时间窗口首尾相接但不重叠时，不应误判为时间冲突。
14. 自动资金类、随机/概率/奖池资金类、单次或总预算超过默认阈值、已上线关键配置变更等高危项返回 `HIGH_RISK_*`，整体 `BLOCK`。
15. 活动配置版本变化后，旧校验通过结果不得继续作为上线依据。
16. 返回结果必须包含多个独立失败项；测试中同时存在概率错误和预算错误时，两项都应出现在 `items[]`。

## 待确认问题

以下问题不阻塞本轮进入研发；本轮已有默认最小可验收口径：

| 问题 | 本轮默认口径 |
|---|---|
| 高危阈值是否有运营配置来源？ | 没有则使用默认：单次最大奖励 `1000.0000`、活动总预算 `100000.0000`，触发即 `BLOCK`。 |
| 高危配置未来是否可通过二级审批上线？ | 本任务不实现审批流，全部高危先 `BLOCK`；后续审批流可复用 `HIGH_RISK_*` 结果。 |
| 概率是百分比还是相对权重？ | 本任务按百分比，生效 item 权重总和必须等于 `100.0000`。 |
| 缺少库存 quota 是否一定阻断？ | 库存型、非金额权益、概率/随机/奖池类阻断；普通无限量金额类可 `WARN`。 |
| `POOL_SPLIT` 是否要做活动结束后整池结算？ | 否。本任务只校验事件即时分摊配置的上限和风险。 |

## 评分

```text
需求清晰度        18/20
范围可控          19/20
风险可控          17/20
验收标准明确      19/20
架构一致性        17/20
总分              90/100
```

扣分原因：

- 高危阈值缺少产品级配置来源，只能给默认保守阈值。
- 后续审批流尚未落地，本任务只能将高危统一阻断。
- 时间/互斥冲突依赖现有活动数据完整性，历史脏配置会导致保守阻断。

## Decision

```text
PASS
```

需求：2026-05-24-113929-6
