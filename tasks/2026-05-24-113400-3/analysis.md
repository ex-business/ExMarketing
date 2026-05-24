# Analysis

## 需求说明

本需求要求将 ExMarketing 的规则判断从“活动命中后执行一组规则”细化为可解释的四阶段规则执行模型。阶段顺序以本任务 `task.md` 为准：

```text
MATCH -> QUALIFY -> REWARD -> RISK/LIMIT/BUDGET
```

四阶段含义如下：

- `MATCH`：活动匹配阶段，判断活动是否是本事件的候选活动。
- `QUALIFY`：参与资格阶段，判断主体、事件上下文和外部事实是否满足参与资格。
- `REWARD`：奖励条件阶段，判断是否满足奖励门槛和奖励配置所需输入条件。
- `RISK/LIMIT/BUDGET`：领取限制阶段，统一表达风控、互斥、次数、周期、预算和库存等发奖前限制。

与现有数据字典的关系：

- `marketing_rule.rule_phase` 当前合法值为 `MATCH / QUALIFY / REWARD / RISK / LIMIT`。
- `BUDGET` 在现有资料中是 `marketing_quota.quota_type`，不是 `rule_phase`。
- 因此本需求中的 `RISK/LIMIT/BUDGET` 是业务阶段名称，不要求新增独立 `BUDGET` 规则阶段；其可配置规则落在 `RISK` 或 `LIMIT`，预算/库存/次数等额度能力通过 quota 语义参与该阶段解释和裁决。

规则执行必须支持从三类数据来源读取字段：

- 事件 `attributes`。
- 活动系统自身状态 `state`，包括活动基础状态、时间窗口、参与次数、预算、库存、互斥组命中、幂等状态等。
- 可选 `facts`，字段路径形如 `facts.USER_PROFILE.vipLevel`。

每条参与执行或被跳过的规则都必须产生可解释结果，至少包括规则编码、阶段、规则类型、执行状态、是否通过、原因码和原因说明。新增规则 evaluator 时，主执行流程不得因为某个具体规则类型新增业务分支。

## 背景目标

来源材料：

- `tasks/2026-05-24-113400-3/task.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/api.md`
- `README.md`
- `CLAUDE.md`

ExMarketing 的定位是独立活动营销中台，通过 Event In、Fact Query、Reward Command、Callback In 与外部系统对接。现有核心链路为：

```text
Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox
```

本需求的目标不是新增具体营销玩法，而是给活动匹配、参与资格、奖励判断、额度限制和审计解释建立统一规则阶段模型，使模拟运行、线上事件处理、活动上线前校验、拒绝原因追踪和后续 evaluator 扩展具备一致口径。

## 参与方

- 外部业务系统：上报标准事件；在必要时提供 Fact Provider 或将业务字段放入 `attributes`。
- 活动配置方：配置活动基础信息、规则、奖励、互斥、预算、库存和次数限制。
- 规则引擎：按固定阶段顺序执行规则，读取标准字段路径，输出规则级解释结果。
- 活动处理流程：根据规则结果决定是否继续生成 participation、grant order、outbox，以及是否进入 quota 原子扣减。
- Quota / Budget / Inventory 能力：提供预算、库存、用户次数、每日次数等限制的检查和最终扣减依据。
- 审计和模拟运行使用方：查看活动命中、拒绝原因、规则明细、预计奖励和预计额度影响。
- 后续规则扩展方：通过注册 evaluator 扩展规则类型，不改变主流程的阶段编排。

## 输入

规则执行输入应包含以下业务信息：

- 标准事件：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`、`attributes`。
- 活动配置：活动 ID、租户、活动编码、活动类型、状态、触发事件、开始结束时间、时区、优先级、互斥组、是否可叠加、版本和扩展配置。
- 规则配置：`ruleCode`、`rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version`。
- 奖励配置：奖励编码、奖励类型、计算方式、奖励配置、发奖模式、发奖通道、是否启用、版本。
- 活动系统状态 `state`：当前活动状态、活动时间判断结果、当前主体参与次数、当日或周期次数、互斥命中情况、预算剩余、库存剩余、幂等状态等。
- 可选外部事实 `facts`：按 `factCode` 聚合的外部事实集合。

字段路径命名空间约束：

- `attributes.<path>`：读取标准事件的业务扩展字段，例如 `attributes.amount`、`attributes.isFirstDeposit`。
- `state.activity.<field>`：读取活动自身状态，例如 `state.activity.status`、`state.activity.startTime`、`state.activity.endTime`、`state.activity.timezone`。
- `state.subject.<field>`：读取当前主体在活动内的状态，例如 `state.subject.participationCount`、`state.subject.dailyCount`。
- `state.quota.<quotaType>.<field>`：读取额度状态，例如 `state.quota.BUDGET.remaining`、`state.quota.STOCK.remaining`、`state.quota.USER_LIMIT.remaining`。
- `state.exclusive.<field>`：读取互斥裁决状态，例如 `state.exclusive.group`、`state.exclusive.alreadyMatched`。
- `facts.<factCode>.<path>`：读取可选外部事实，例如 `facts.USER_PROFILE.vipLevel`。

限制：

- 本需求不允许规则直接读取外部业务数据库。
- 能放入 `attributes` 的事件事实不应强依赖实时 Fact Query。
- `facts.*` 是可选能力；依赖 `facts.*` 的规则必须在配置或执行时具备明确 provider 来源和失败原因。

## 输出

规则执行输出应覆盖活动级、阶段级和规则级三个层次。

活动级输出：

- 活动 ID、活动编码、活动版本。
- 总体决策：`PASSED`、`REJECTED`、`SKIPPED` 或 `ERROR`。
- 最终失败阶段，若通过则为空。
- 最终原因码和原因说明。
- 是否允许进入奖励计算。
- 是否允许进入 quota 原子扣减和发奖单/outbox 生成。

阶段级输出：

- 阶段名称：`MATCH`、`QUALIFY`、`REWARD`、`RISK_LIMIT_BUDGET`。
- 阶段状态：`PASSED`、`REJECTED`、`SKIPPED` 或 `ERROR`。
- 本阶段执行的规则数、跳过规则数、失败规则数。
- 本阶段首个阻断原因和全部可展示原因。

规则级输出：

- `ruleCode`
- `rulePhase`
- `ruleType`
- `status`：`PASSED`、`FAILED`、`SKIPPED`、`ERROR`
- `passed`：布尔值；`SKIPPED` 时可为空或为 false，但必须由 `status` 区分。
- `reasonCode`
- `reason`
- `field`：相关字段路径，若适用。
- `expected` 和 `actual`：条件判断类规则可返回，敏感信息必须脱敏。
- `detail`：可选扩展信息，只允许放可审计、可对外解释的安全内容。

正式事件处理输出和落库追踪：

- 规则拒绝时应能在 participation 拒绝原因或审计记录中追踪到活动级最终原因和规则摘要。
- 规则通过且存在发奖单时，后续仍需遵守现有 activity 级事务：写 `REWARDABLE` participation、扣 quota、写 grant order、写 outbox。
- quota 最终扣减失败时应按现有语义回滚 participation/grant order/outbox，并记录 `QUOTA_EXCEEDED` 审计；规则阶段的预检查解释不得替代最终原子扣减。

模拟运行输出：

- 命中和未命中活动列表。
- 每个活动的阶段结果和规则结果。
- 预计 participation。
- 预计 grant order。
- 预计 outbox reward command。
- 预计预算、库存、次数影响。
- 拒绝原因。
- 不写入 inbox、participation、grant order、outbox 或 quota。

## 业务规则

1. 固定阶段顺序为 `MATCH -> QUALIFY -> REWARD -> RISK/LIMIT/BUDGET`。
2. `MATCH` 阶段负责候选活动判断，包括租户、触发事件、活动状态、活动时间窗口、优先级候选和基础互斥候选。
3. `QUALIFY` 阶段负责参与资格，包括主体身份、事件上下文、VIP、标签、注册时间、首存标记、风险名单、IP、设备和外部事实资格。
4. `REWARD` 阶段负责奖励门槛和奖励输入条件判断，例如金额门槛、流水门槛、邀请人数、签到天数、奖励计算所需字段存在且类型正确。奖励金额计算本身仍属于奖励计算能力，不由规则解释输出替代。
5. `RISK/LIMIT/BUDGET` 阶段负责发奖前限制裁决，包括风控结果、用户次数、每日/周期限制、互斥组、`stackable`、预算、库存、幂等和 quota 预检查解释。
6. 阶段内按 `sort` 升序执行；`sort` 相同时应有稳定顺序，避免同一配置多次执行输出顺序漂移。
7. `enabled=false` 的规则不参与通过/失败裁决，但在模拟运行和审计明细中应输出 `SKIPPED`，原因码为 `RULE_DISABLED`。
8. 未配置规则的阶段默认通过，并输出阶段级 `PASSED`，规则列表为空。
9. 同阶段多条规则默认聚合逻辑为全部通过才通过；单条 `CONDITION` 规则内部可继续使用其 `ruleConfig.logic` 表达 `AND` 或 `OR`。
10. 当前已知可执行规则类型为 `CONDITION`；设计文档提到但当前未开放或未注册 evaluator 的 `FORMULA`、`LIMIT`、`SCRIPT` 等规则类型不能默认通过，必须给出明确错误或配置校验失败。
11. 字段路径不存在、类型不匹配、Decimal 不可解析、操作符不支持、配置 JSON 非法时，规则结果必须为 `FAILED` 或 `ERROR`，不能静默通过。
12. 金额、数量、预算、库存、次数等数值比较必须按明确数值或 Decimal 语义解释，不能按字符串字典序解释。
13. 活动状态只有满足可执行状态且事件时间处于活动窗口内，`MATCH` 才能通过；`DRAFT`、`PAUSED`、`ENDED` 等不可执行状态必须有明确拒绝原因。
14. 多活动命中时应遵守 `priority`、`exclusiveGroup` 和 `stackable`：同互斥组或不可叠加时，只保留业务规则允许的活动，其余活动输出可解释拒绝或跳过原因。
15. `facts.*` 查询失败、超时、未授权或结构不符合预期时，依赖该字段的规则不得通过；应输出明确 reason code，例如 `FACT_UNAVAILABLE`、`FACT_TIMEOUT`、`FACT_FIELD_MISSING`。
16. 预算、库存、次数在规则阶段可以输出预检查和解释，但最终扣减仍必须由现有 quota 原子扣减流程完成。
17. 重复事件或幂等冲突不应重复执行发奖链路，也不应重复扣减 quota。
18. 规则解释输出不得泄漏 token、provider 内部地址、鉴权头、完整敏感风控详情或外部系统敏感原始响应。

## 边界条件

- 活动没有配置任何规则：所有规则阶段默认通过；仍必须执行活动基础匹配、奖励配置校验和 quota 事务约束。
- 活动只配置部分阶段规则：未配置阶段不阻断流程，输出阶段级通过。
- 未知 `rulePhase`：活动配置应被拒绝；若历史数据已存在，执行时该活动应进入 `ERROR` 或配置异常拒绝，不允许忽略。
- `BUDGET` 作为 `rule_phase` 出现：不符合当前数据字典，应视为非法规则阶段；预算限制应表达为 quota 或 `LIMIT` 阶段规则解释。
- 未知 `ruleType` 或未注册 evaluator：配置阶段或执行阶段必须产生明确错误，不能默认通过。
- `attributes` 为空：只要规则不依赖 attributes，可以继续；依赖字段缺失的规则失败。
- `facts` 未配置：不依赖 facts 的规则不受影响；依赖 facts 的规则应配置失败或执行失败，并输出明确原因。
- quota 行缺失：按现有资料，缺少对应 quota 行表示不限制；但规则解释应能区分“不限制”和“额度充足”。
- 奖励配置不存在或全部 disabled：规则阶段可以通过，但后续不应生成发奖单；模拟运行需说明无可发奖励。
- 多个奖励同时启用：预算扣减预估为同一 activity 本次所有 reward amount 合计；库存预估按每个 enabled reward 默认扣 1，`scopeKey` 为 rewardCode。
- 事件时间和活动时区：活动窗口判断必须使用活动配置的 timezone 语义，避免跨时区边界误判。
- 并发事件：规则预检查通过不代表最终 quota 必然扣减成功；最终以 activity 级事务和 quota 条件更新结果为准。

## 异常场景

- 事件模型非法、顶层未知字段、缺少必填字段或幂等 Header 不匹配。
- 活动状态不可执行、事件类型不在 `triggerEvents` 中、租户不匹配、事件时间不在活动窗口内。
- 规则配置 JSON 非法、缺少必要字段、操作符非法、字段路径非法。
- `attributes.amount` 等金额字段不可解析为 Decimal。
- 规则要求布尔值但实际字段为字符串，或规则要求数值但实际字段为对象/数组。
- `facts.USER_PROFILE` 未配置 provider、provider 不在 allowlist、请求超时、返回非预期结构或返回敏感错误。
- 用户已达到活动参与次数、每日限制或周期限制。
- 同互斥组已有更高优先级活动命中，当前活动被跳过或拒绝。
- `stackable=false` 时多个活动同时命中，只允许保留一个活动，其余活动输出解释。
- 预算不足、库存不足或用户次数不足，最终 quota 扣减失败。
- 重复事件返回 duplicate，不生成新的 participation、grant order、outbox 或 quota 扣减。
- 规则 evaluator 执行异常，活动不得继续进入发奖链路，必须形成可追踪错误原因。

## 非目标范围

- 不新增业务专用 controller、业务专用规则流程或外部业务数据库查询。
- 不在本需求中直接发奖、修改用户余额或绕过 grant order/outbox。
- 不把 `BUDGET` 新增为独立 `rule_phase`，除非后续数据字典另行变更。
- 不新增完整后台配置页面。
- 不实现 MQ Event In、MQ Command Out、人工发奖、导出发奖。
- 不实现新的奖励计算方式，例如 `LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`、`DRAW_CHANCE`、`GIFT`。
- 不改变 Fact Query 的安全边界，不放宽 allowlist、private/local IP、redirect、proxy 等限制。
- 不把真实风控判定内置到 ExMarketing；真实风控应由事件字段或 facts 提供，ExMarketing 只消费结果并解释规则。
- 不设计具体代码改法、包结构调整、函数签名或数据库迁移细节。

## 风险

- P1：阶段名称与数据字典存在表达差异。任务使用 `RISK/LIMIT/BUDGET` 作为业务阶段，数据字典只列 `RISK` 和 `LIMIT` 规则阶段，`BUDGET` 是 quota 类型。本文已将 `BUDGET` 定义为该业务阶段中的 quota 能力，避免要求新增非法 `rule_phase`。
- P1：规则预检查和最终 quota 扣减存在竞态。需求必须明确最终以 activity 级事务和 quota 原子扣减为准，预检查只用于解释和提前拒绝。
- P1：解释输出可能泄漏外部事实、风控细节或 provider 敏感信息。验收时必须检查 reason/detail 的脱敏和安全边界。
- P2：`facts.*` 能力是可选外部依赖，存在超时、不可用、结构漂移和权限问题。依赖 facts 的规则必须有明确失败原因，不能让活动误通过。
- P2：同阶段规则短路会减少完整原因收集；不短路会增加外部 facts 调用和延迟。需求口径为正式处理允许在活动已拒绝后停止后续阶段，模拟运行应尽量输出已执行阶段内的完整规则结果。
- P2：多活动互斥和 `stackable` 的解释既涉及匹配又涉及限制，容易出现重复原因。验收应要求最终活动列表中每个被排除活动有唯一、清晰的最终原因。

## 验收标准

1. 规则执行结果按固定阶段顺序展示：`MATCH -> QUALIFY -> REWARD -> RISK/LIMIT/BUDGET`。
2. 支持 `MATCH / QUALIFY / REWARD / RISK / LIMIT` 规则阶段；`BUDGET` 不作为合法 `rule_phase`，预算能力通过 quota 或限制阶段解释。
3. 阶段内规则按 `sort` 升序执行，`enabled=false` 规则在模拟和明细中输出 `SKIPPED` 且不影响裁决。
4. 无规则阶段默认通过；未知阶段、未知规则类型、未注册 evaluator 不得默认通过。
5. `CONDITION` 规则可以从 `attributes.*` 读取字段并输出通过/失败原因。
6. 规则可以从 `state.*` 命名空间读取活动状态、主体参与状态、互斥状态、quota 状态等活动系统自身状态。
7. 规则可以从可选 `facts.<factCode>.*` 读取字段；fact 缺失、provider 不可用、超时、结构错误均有明确失败原因。
8. 每条规则结果至少包含 `ruleCode`、`rulePhase`、`ruleType`、`status`、`passed`、`reasonCode`、`reason`。
9. 每个活动结果包含总体决策、失败阶段、最终原因和阶段明细。
10. 正式事件处理在规则拒绝时可追踪到 participation 拒绝原因或审计摘要；不得产生 grant order、outbox 或 quota 扣减。
11. 规则通过后仍必须遵守现有 activity 级事务：`REWARDABLE` participation、quota 扣减、grant order、outbox 同事务处理。
12. quota 不足时不保留 `REWARDABLE` 脏 participation，不生成 grant order/outbox，保留可审计 `QUOTA_EXCEEDED` 原因。
13. 模拟运行输出命中活动、未命中活动、规则明细、预计奖励、预计 outbox、预计 quota 影响，且不写业务表。
14. 多活动命中时按 priority、exclusiveGroup、stackable 输出最终保留活动和被排除活动的原因。
15. 字段缺失、类型不匹配、Decimal 解析失败、操作符非法、配置 JSON 非法都有明确错误或失败原因。
16. 新增 evaluator 只需作为规则类型能力注册并被规则引擎识别，不要求改动四阶段主流程的业务编排。
17. 解释输出不得包含 token、Authorization header、内部 provider URL、完整敏感风控响应等敏感信息。
18. 测试或验收用例至少覆盖：全阶段通过、MATCH 拒绝、QUALIFY 拒绝、REWARD 门槛不满足、RISK/LIMIT/BUDGET 拒绝、字段缺失、类型错误、fact 缺失/超时、预算不足、库存不足、用户次数不足、互斥拒绝、disabled 规则、未知 phase、未知 evaluator、模拟运行不落库、正式处理拒绝可追踪。

## 待确认问题

以下问题不阻断本需求进入 Review，因为本文已按现有任务和文档给出可验收口径；但建议在后续 PRD 或接口文档中固化：

1. 规则解释输出是否需要作为外部 OpenAPI 响应的稳定 schema，还是只要求后台模拟和内部审计稳定。
2. reason code 是否需要建立全局枚举表和中英文文案表。
3. 正式事件处理是否只返回最终拒绝原因，还是返回完整阶段/规则明细。
4. 模拟运行是否必须继续执行被拒绝活动的后续阶段以收集更多原因，还是停止于首个失败阶段。
5. `state.*` 命名空间是否需要在数据字典中新增正式字段清单。
6. facts 结果是否允许缓存命中信息进入规则 detail，或只展示业务字段判断结果。
7. 多活动互斥时，同优先级活动的裁决顺序是否需要业务配置明确。

## Score

```text
需求清晰度        18 / 20
需求边界清楚      18 / 20
风险可控          17 / 20
验收标准明确      19 / 20
约束一致性        18 / 20
总分              90 / 100
```

## Decision

```text
PASS
```

理由：Score >= 80。需求已明确四阶段顺序、`BUDGET` 与 `rule_phase` 的关系、字段读取命名空间、规则解释输出、异常场景、非目标范围和验收方式；剩余待确认项不影响 Review Agent 判断需求是否可审核、可验收。

需求：2026-05-24-113400-3
