# Analysis

## 需求 ID

2026-05-24-qualify

## Review 反馈处理

已读取 `tasks/2026-05-24-qualify/review.md`。当前 Review 文件仍是模板态，未填写具体 P0/P1/P2，但存在 `Open Questions` 占位和 `Decision PASS / BLOCK` 占位，因此本轮按返工处理。

| Review 项 | 处理 |
|---|---|
| `Open Questions` 为“待填写”。 | 未发现 Review Agent 提出的具体问题。为避免继续停留在模板态，本轮主动补齐 QUALIFY 的资料依据、输入输出、业务规则、边界条件、异常场景、非目标、风险、验收标准和待确认问题。 |
| `Decision` 为模板占位 `PASS / BLOCK`。 | 本轮不沿用占位结论，按评分规则重新评估。需求细节已收敛到可审核、可验收的最小口径，最终 Decision 为 `PASS`。 |
| 未填写 P0/P1/P2。 | 未发现可逐条回应的具体打回意见。若后续 Review Agent 补充 P0/P1，应回到 Analysis 继续逐条处理。 |

## Analysis 自检修正

已读取上一版 `analysis.md`。上一版为模板态，存在 Score `0`、`Decision PASS / BLOCK` 占位，以及“现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式”全部待填写，按 Analysis 自己上一轮 BLOCK 后返工处理。

| 上一版问题 | 修正 |
|---|---|
| Score 为 `0`，低于 80。 | 本轮基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/workflow.md`、`docs/roadmap.md` 和 `22-独立活动营销中台设计.md` 补齐完整需求，重新评分为 90。 |
| Decision 为模板占位，未给出 PASS/BLOCK。 | 本轮明确 Decision 为 `PASS`。 |
| 现状、目标、范围、风险、验收方式均为空。 | 本轮补充背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险、验收标准。 |
| 未定义“字段路径可读”和“字段不存在时可解释失败”的可验收口径。 | 本轮明确字段路径白名单、数组和 Decimal 比较规则、缺失字段/类型不匹配/事实源失败的稳定失败语义和验收样例。 |
| 未处理信息不唯一的问题。 | 本轮给出默认最小可验收口径：QUALIFY 只要求支持通用 `CONDITION` 规则，不新增业务专用 VIP/注册天数/渠道/标签 evaluator；VIP、注册天数、渠道、标签等均通过 `attributes.*`、`facts.*` 或受控 `state.*` 表达。该口径依据是设计文档“业务字段全部放到 attributes”“可选 facts”“新增规则只注册 evaluator，不修改主流程”，以及当前数据模型仅确认 `CONDITION` 已实现。 |

## 资料依据

- `tasks/2026-05-24-qualify/task.md`：要求实现 QUALIFY 阶段，基于 `attributes`、`state`、`facts` 判断 VIP、注册天数、渠道、用户标签等参与资格；验收重点为规则字段路径可读、字段不存在时返回可解释失败原因。
- `README.md`：ExMarketing 只接收标准事件、判断活动配置、生成 participation / grant_order / outbox；外部业务字段只能通过 `attributes`、`facts`、`mapping`、`adapter` 进入规则引擎。
- `CLAUDE.md`：禁止连接外部业务数据库、引用外部业务代码包、把业务强模型硬编码进活动系统；规则引擎变更需要覆盖字段缺失、数值比较等测试。
- `docs/architecture.md`：规则执行顺序固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`；每条规则输出 `status`、`passed`、`reasonCode`、`reason`、`field`、`expected`、`actual` 等解释信息；禁用规则为 `SKIPPED`。
- `docs/api.md`：规则字段路径支持标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、`state.plugin.<pluginCode>.*`；字段缺失、类型不匹配、非法操作符或未注册 evaluator 不会默认通过。
- `docs/data-model.md`：`marketing_rule.rule_phase` 合法值包含 `QUALIFY`；当前已实现规则类型为 `CONDITION`；规则拒绝时写 `REJECTED` participation，不生成 grant_order/outbox，不扣 quota。
- `22-独立活动营销中台设计.md`：QUALIFY 对应“用户资格规则”，判断 VIP、标签、注册时间、风险名单、代理、IP、设备；规则数据来源只能是事件 `attributes`、活动系统自身 `state`、可选 `facts`；后台上线前校验必须检查规则字段是否存在、JSON Schema 是否合法，并支持模拟运行解释每条规则通过/失败原因。

## 背景目标

ExMarketing 是独立活动营销中台，不能内置会员、注册、渠道、用户标签等外部业务强模型。QUALIFY 阶段的目标是让活动配置方用通用条件规则表达“谁有资格参与活动”，并在正式事件处理和模拟运行中得到稳定、可解释、可审计的判断结果。

本任务的核心目标：

- 明确 `rulePhase=QUALIFY` 的业务语义：参与资格判断。
- 支持基于标准事件字段、`attributes.*`、`facts.<factCode>.*` 和受控 `state.*` 的条件判断。
- 覆盖 VIP、注册天数、渠道、用户标签、风险标签、IP、设备、代理等常见资格条件的配置表达。
- 字段路径必须可读、可校验、可解释；非法路径不得静默保存或运行时默认通过。
- 字段不存在、事实源缺失、类型不匹配、数组不匹配等失败场景必须返回可解释原因。
- QUALIFY 拒绝只能影响 participation 结果，不得生成发奖单、outbox 或扣减 quota。

## 默认最小可验收口径

现有资料没有要求本任务新增 VIP、注册天数、渠道或标签的业务专用 evaluator，也没有要求 ExMarketing 直连会员系统。因此本轮固定以下最小可验收口径：

1. QUALIFY 使用通用 `CONDITION` 规则表达参与资格。
2. `rulePhase` 必须为 `QUALIFY`，`ruleType` 本期只要求可执行 `CONDITION`。
3. VIP、注册天数、渠道、标签等字段来自：
   - 事件 `attributes.*`，例如 `attributes.vipLevel`、`attributes.registerDays`、`attributes.channel`、`attributes.tags`。
   - 外部事实 `facts.<factCode>.*`，例如 `facts.USER_PROFILE.vipLevel`、`facts.USER_PROFILE.registerDays`。
   - 活动系统受控状态 `state.*`，例如 `state.subject.participationCount`。
4. 不新增外部业务数据库查询、不引用外部业务代码包、不新增业务专用顶层字段。
5. 字段缺失或不可比较时，该条件失败并返回解释；不能把缺失字段当作空字符串、0、false 或通过。

该口径能覆盖本任务验收目标，并与独立营销中台解耦原则一致。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 活动配置方 / 运营 | 能用可读字段路径配置 VIP、注册天数、渠道、用户标签等资格条件，并在配置错误时得到明确反馈。 |
| 外部业务系统 | 通过标准事件 `attributes` 或 Fact Provider 提供用户资格事实，不需要暴露数据库或业务代码。 |
| Fact Provider 维护方 | 为 `facts.<factCode>.*` 提供可选外部事实，并保证同租户、启用、mapping 覆盖规则字段。 |
| 活动引擎 | 在 MATCH 后、REWARD 前执行 QUALIFY，依据结果决定是否继续发奖链路。 |
| 规则引擎 | 按通用规则配置读取字段、执行比较、输出规则级解释。 |
| 审计 / 排障方 | 通过 participation 快照、模拟运行结果或规则结果定位拒绝原因。 |
| Review / Test Agent | 根据本分析检查范围、风险和验收样例是否完整。 |

## 输入输出

### 输入：QUALIFY 规则配置

活动规则数组中的一条或多条规则：

| 字段 | 要求 |
|---|---|
| `ruleCode` | 必填，活动内唯一，用于错误定位、模拟运行和 participation 快照。 |
| `rulePhase` | 必填，本任务目标阶段为 `QUALIFY`。 |
| `ruleType` | 本期可执行口径为 `CONDITION`。未注册的 `FORMULA`、`LIMIT`、`SCRIPT` 不属于本任务上线范围。 |
| `ruleConfig.logic` | 必填，`AND` 或 `OR`。 |
| `ruleConfig.conditions` | 必填，非空数组。 |
| `conditions[].field` | 必填，必须命中字段路径白名单。 |
| `conditions[].op` | 必填，支持 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。 |
| `conditions[].value` | 必填。等值和比较允许 string、number、boolean；`IN`、`NOT_IN` 必须为非空数组。 |
| `sort` | 同阶段执行顺序，升序执行；相同 sort 需保持稳定顺序。 |
| `enabled` | `false` 时运行结果为 `SKIPPED`，不影响通过/拒绝；但不放宽配置 Schema 校验。 |
| `version` | 用于快照和审计追溯。 |

### 字段路径输入范围

QUALIFY 可读字段路径：

```text
eventId
eventType
sourceSystem
tenantId
subject.type
subject.id
biz.bizType
biz.bizId
occurredAt
attributes.<path>
facts.<factCode>.<path>
state.activity.<field>
state.subject.<field>
state.quota.<quotaType>.<field>
state.exclusive.<field>
state.plugin.<pluginCode>.<path>
```

典型资格字段表达：

| 资格条件 | 推荐字段 |
|---|---|
| VIP 等级 | `attributes.vipLevel` 或 `facts.USER_PROFILE.vipLevel` |
| 注册天数 | `attributes.registerDays` 或 `facts.USER_PROFILE.registerDays` |
| 渠道 | `attributes.channel` |
| 用户标签 | `attributes.tags`、`attributes.segment` 或 `facts.USER_PROFILE.tags` |
| 风险标签 | `attributes.riskTag` 或 `facts.RISK_PROFILE.riskTags` |
| IP / 设备 | `attributes.ip`、`attributes.deviceId` |
| 主体类型 / 用户 ID | `subject.type`、`subject.id` |

非法字段示例：

```text
vipLevel
registerDays
channel
user.vipLevel
deposit.amount
member.tags
plugin.someField
state.vip.level
```

这些字段要么是非标准顶层业务字段，要么绕过了 `attributes` / `facts` / 受控 `state` 命名空间，必须在配置校验或运行解释中明确拒绝。

### 输出：配置校验结果

成功：

- 活动可保存或进入上线前校验下一步。
- 查询活动时保留 QUALIFY 规则配置。
- 模拟运行或正式事件处理可输出每条 QUALIFY 规则的执行结果。

失败：

- 非法字段路径、非法 op、非法 value 类型、空 conditions、未知 ruleType 或未注册 facts provider，返回 400 类配置错误。
- 错误必须至少定位到 `rules[index]`、`ruleCode`、`field` 或具体配置字段。
- 非法配置不得部分保存为可运行活动规则。

### 输出：运行结果

QUALIFY 通过：

- 继续进入后续 REWARD / RISK / LIMIT / quota / grant_order / outbox 链路。
- 规则结果中记录每条通过条件的 `field`、`expected`、`actual`。

QUALIFY 拒绝：

- 当前活动对该事件生成 `REJECTED` participation 或在模拟运行中标记 unmatched / rejected。
- 不生成 grant_order。
- 不写 outbox。
- 不扣减 BUDGET / STOCK / USER_LIMIT quota。
- 拒绝原因必须包含可机器识别 `reasonCode` 和可读 `reason`。

禁用规则：

- `enabled=false` 的 QUALIFY 规则输出 `SKIPPED`，不参与通过/拒绝决策。

## 业务规则

1. QUALIFY 在 MATCH 之后、REWARD 之前执行。
2. MATCH 已不通过的活动不应继续执行 QUALIFY。
3. 一个活动可以配置多条 QUALIFY 规则；同阶段按 `sort` 升序执行。
4. 多条启用的 QUALIFY 规则默认必须全部通过，活动才具备参与资格。
5. 单条 `CONDITION` 内部由 `logic` 决定：
   - `AND`：所有 condition 通过才通过。
   - `OR`：至少一个 condition 通过才通过。
6. 字段读取必须严格按白名单解析，不允许业务字段从非标准顶层读取。
7. 字段路径存在但值为 `null`，按不可比较或字段无有效值处理，不得默认通过。
8. 字段路径不存在时，当前 condition 失败，并返回 `FIELD_MISSING` 类解释。
9. `>`、`>=`、`<`、`<=` 只适用于数字或可解析 Decimal 字符串；不可解析时返回类型或比较错误。
10. `=` / `==` / `!=` / `<>` 支持 string、number、boolean 的同类型比较；number 和可解析 Decimal 字符串可按 Decimal 语义比较。
11. `IN` / `NOT_IN` 的期望值必须是非空数组。
12. 当实际字段是标量时：
    - `IN` 表示实际值在期望数组中。
    - `NOT_IN` 表示实际值不在期望数组中。
13. 当实际字段是数组时：
    - `IN` 表示实际数组与期望数组至少有一个交集，适合用户标签、渠道集合等资格判断。
    - `NOT_IN` 表示实际数组与期望数组无交集，适合黑名单标签排除。
14. 数组元素比较必须使用可解释的基础类型；object/array 嵌套元素不作为本期可比较元素。
15. `facts.<factCode>.*` 字段只有在同租户存在启用 provider 且 mapping 覆盖字段时才允许配置和运行。
16. facts 查询失败、provider 未配置、mapping 字段缺失或类型不匹配时，依赖该 fact 的 QUALIFY 条件失败，不得默认通过。
17. `state.plugin.<pluginCode>.*` 只有活动绑定对应插件且插件上下文已受控暴露时才可读取。
18. QUALIFY 不能修改 state、quota、participation 以外的外部业务状态；它只是资格判断。
19. QUALIFY 不直接发奖、不计算奖励金额、不扣预算库存。
20. 模拟运行和正式事件处理的 QUALIFY 解释口径必须一致。

## 示例规则

VIP + 注册天数 + 渠道：

```json
{
  "ruleCode": "VIP_REGISTER_CHANNEL",
  "rulePhase": "QUALIFY",
  "ruleType": "CONDITION",
  "ruleConfig": {
    "logic": "AND",
    "conditions": [
      {"field": "attributes.vipLevel", "op": ">=", "value": 3},
      {"field": "attributes.registerDays", "op": ">=", "value": 7},
      {"field": "attributes.channel", "op": "IN", "value": ["APP", "WEB"]}
    ]
  },
  "sort": 10,
  "enabled": true
}
```

标签准入 + 风险标签排除：

```json
{
  "ruleCode": "SEGMENT_ALLOW_RISK_DENY",
  "rulePhase": "QUALIFY",
  "ruleType": "CONDITION",
  "ruleConfig": {
    "logic": "AND",
    "conditions": [
      {"field": "attributes.tags", "op": "IN", "value": ["new_user", "high_value"]},
      {"field": "attributes.riskTag", "op": "NOT_IN", "value": ["black", "bonus_abuse"]}
    ]
  },
  "sort": 20,
  "enabled": true
}
```

外部 facts 用户画像：

```json
{
  "ruleCode": "FACT_USER_PROFILE",
  "rulePhase": "QUALIFY",
  "ruleType": "CONDITION",
  "ruleConfig": {
    "logic": "AND",
    "conditions": [
      {"field": "facts.USER_PROFILE.vipLevel", "op": ">=", "value": 3},
      {"field": "facts.USER_PROFILE.registerDays", "op": ">=", "value": 30}
    ]
  },
  "sort": 30,
  "enabled": true
}
```

## 边界条件

- `attributes` 可以为空对象；引用不存在字段时必须失败并解释。
- 嵌套路径如 `attributes.profile.vipLevel` 可读，但中间节点缺失、不是 object 或最终值为 object 时，不得按成功比较。
- `attributes.tags` 可为字符串数组；若为逗号分隔字符串，本任务不要求自动拆分，除非外部事件明确按数组上报。
- 大小写敏感：默认字符串和标签比较区分大小写，不做隐式大小写归一化。
- 空字符串是存在值，可以参与等值比较；但不能参与数值比较。
- Decimal 字符串可用于数值比较，例如 `"3"`、`"100.00"`；不可解析字符串返回 `TYPE_MISMATCH` 或 `COMPARISON_NOT_SUPPORTED`。
- 时间字段 `occurredAt` 本期只要求支持等值/集合比较；不要求新增时间大小比较语义。
- `OR` 条件中，一个 condition 因字段缺失失败，不阻断其他 condition 继续判断；最终若至少一个通过则规则通过，但结果中应保留失败 condition 的解释。
- `AND` 条件中允许短路执行，但最终解释至少要包含导致失败的首个条件；模拟运行建议展示所有条件结果。
- facts provider 不可用时，依赖 facts 的 condition 失败；不依赖 facts 的活动不受影响。
- 同租户隔离：规则不得引用其他 tenant 的 facts provider 或 state。
- 插件上下文未绑定、插件未注册或 pluginCode 不匹配时，`state.plugin.*` 字段不可读。

## 异常场景

| 场景 | 期望结果 |
|---|---|
| `field=attributes.vipLevel`，事件缺少 `vipLevel`。 | QUALIFY 失败，返回 `FIELD_MISSING`，说明字段不存在。 |
| `field=attributes.registerDays`，值为 `"abc"`，op 为 `>=`。 | QUALIFY 失败，返回类型或比较错误，actual 展示脱敏后的实际值。 |
| `field=attributes.tags`，值为 `["black"]`，op 为 `NOT_IN`，期望 `["black", "bonus_abuse"]`。 | QUALIFY 失败，说明命中排除标签。 |
| `field=attributes.tags`，值为 `["new_user"]`，op 为 `IN`，期望 `["vip", "new_user"]`。 | 条件通过，actual 为实际标签集合。 |
| `field=facts.USER_PROFILE.vipLevel`，provider 未配置。 | 配置保存或上线前校验失败；若运行时配置漂移，则规则失败并返回 `FACT_PROVIDER_NOT_CONFIGURED`。 |
| facts provider 超时。 | 依赖该 facts 的 QUALIFY 失败，返回 `FACT_PROVIDER_TIMEOUT`，不得默认通过。 |
| 使用顶层 `vipLevel`。 | 配置校验失败，提示必须使用 `attributes.vipLevel` 或 `facts.<factCode>.vipLevel`。 |
| `enabled=false` 但 ruleConfig 非法。 | 配置校验仍失败；禁用不等于非法草稿。 |
| 无 QUALIFY 规则。 | 不因 QUALIFY 阶段拒绝；是否命中由 MATCH、REWARD、RISK/LIMIT/Quota 等后续规则决定。 |
| QUALIFY 全部通过但奖励计算为 0。 | 不属于 QUALIFY 决策；按奖励阶段既有语义处理。 |

## 非目标范围

- 不新增会员、VIP、注册天数、渠道、标签等业务专用数据模型。
- 不直连外部会员库、订单库、风控库或标签库。
- 不新增业务专用 controller、后台页面或活动类型。
- 不实现新的 `FORMULA`、`LIMIT`、`SCRIPT` evaluator。
- 不改变 MATCH、REWARD、RISK、LIMIT、Quota、grant_order、outbox、callback 的既有职责。
- 不新增发奖、预算、库存、用户次数扣减语义。
- 不定义 Fact Provider registry 的完整管理后台；仅要求 QUALIFY 对 `facts.*` 依赖遵守既有 provider 校验和失败语义。
- 不把字符串标签自动拆分为数组，不做大小写、语言、地区等业务归一化。
- 不要求实现复杂 JSONPath、数组过滤、脚本表达式或 SQL 查询。

## 风险

| 风险 | 影响 | 缓解口径 |
|---|---|---|
| 字段路径过宽导致业务耦合。 | 活动系统变成会员/订单强模型，难以复用。 | 严格白名单，只允许标准事件、`attributes`、`facts`、受控 `state`。 |
| 字段缺失被默认通过。 | 不符合资格的用户可能领奖。 | 缺失、null、类型不匹配一律失败并解释。 |
| facts provider 运行时不稳定。 | 外部依赖导致资格判断抖动。 | 不依赖 facts 的活动不调用 provider；依赖 facts 时失败即拒绝并给出稳定原因。 |
| 数组标签语义不清。 | 标签准入/排除可能反向。 | 固定 `IN=有交集`、`NOT_IN=无交集` 的数组语义。 |
| 数值比较使用浮点。 | VIP/金额/天数比较出现精度或类型问题。 | 需求层要求 Decimal/整数语义，禁止 float64 金额语义扩散。 |
| 与 MATCH audience 配置重叠。 | 渠道、segment 可能在 MATCH 和 QUALIFY 重复配置。 | MATCH 负责入口和候选过滤；QUALIFY 负责参与资格。重复配置应按两个阶段分别解释，不互相覆盖。 |
| 解释信息不稳定。 | Review/Test 无法验收，运营难以排障。 | 验收标准要求稳定 `reasonCode`、`field`、`expected`、`actual`。 |

## 验收标准

### 配置校验

- 创建或更新活动时，`rulePhase=QUALIFY`、`ruleType=CONDITION`、合法 `logic`、非空 `conditions` 可保存。
- 合法字段路径 `attributes.vipLevel`、`attributes.registerDays`、`attributes.channel`、`attributes.tags`、`facts.USER_PROFILE.vipLevel`、`state.subject.participationCount` 可被识别。
- 非法字段路径 `vipLevel`、`user.vipLevel`、`deposit.amount`、`plugin.foo` 被拒绝，并返回字段路径错误。
- `IN` / `NOT_IN` 的 `value` 不是非空数组时被拒绝。
- 比较操作符配置 object、array、null 等不可比较值时被拒绝或运行失败可解释。
- `enabled=false` 的非法 QUALIFY 规则仍被拒绝保存。
- 依赖 `facts.<factCode>.*` 且 provider 不存在、未启用或 mapping 不覆盖字段时，保存或上线前校验失败。

### 正式事件处理

- 事件包含 `attributes.vipLevel=3`、`attributes.registerDays=10`、`attributes.channel=APP`，规则要求 VIP >= 3、注册天数 >= 7、渠道 IN `[APP, WEB]` 时，QUALIFY 通过。
- 事件缺少 `attributes.registerDays` 时，QUALIFY 拒绝，规则结果包含 `FIELD_MISSING`、字段名 `attributes.registerDays`，不生成 grant_order/outbox，不扣 quota。
- 事件 `attributes.tags=["black"]`，规则要求 `attributes.tags NOT_IN ["black", "bonus_abuse"]` 时，QUALIFY 拒绝，并解释命中排除标签。
- facts provider 超时或返回 mapping 字段缺失时，依赖该 facts 的 QUALIFY 拒绝，reasonCode 能区分超时和字段缺失。
- 重复事件幂等语义不因 QUALIFY 改变。

### 模拟运行

- `POST /admin/v1/simulate` 对同一输入事件展示 QUALIFY 阶段结果。
- 模拟结果至少能看到活动是否因 QUALIFY 拒绝、拒绝的 `ruleCode`、`field`、`expected`、`actual`、`reasonCode`、`reason`。
- 模拟运行无副作用，不写 inbox、participation、grant_order、outbox、quota。

### 回归范围

- 不含 QUALIFY 规则的既有活动行为不变。
- 只含 MATCH / REWARD / RISK / LIMIT 规则的既有活动执行顺序不变。
- Quota 不足仍按既有 `QUOTA_EXCEEDED` 语义处理，不被 QUALIFY 覆盖。
- Outbox、DLQ、reward callback、reconciliation 不因 QUALIFY 改动改变接口契约。

## 待确认问题

无阻断问题。本轮已采用默认最小可验收口径关闭上一版模板 BLOCK：

- 不新增业务专用 evaluator。
- 不新增外部业务数据库读取。
- QUALIFY 本期以通用 `CONDITION` 配置为唯一可执行规则类型。
- 标签数组语义按 `IN=有交集`、`NOT_IN=无交集` 验收。
- 字段缺失、facts 不可用、类型不匹配均按失败并解释处理。

后续可另立需求扩展：

- 专门的用户画像 provider 管理后台。
- 更复杂的周期/累计状态字段。
- 时间比较、大小写归一化、标签字典映射。
- FORMULA / LIMIT / SCRIPT evaluator。

## 评分

| 维度 | 分数 | 说明 |
|---|---:|---|
| 需求清晰度 | 18 / 20 | QUALIFY 的业务语义、输入输出、字段路径、失败解释已明确；仅保留未来扩展项。 |
| 范围可控 | 19 / 20 | 固定最小口径为 `CONDITION` + 白名单字段，不扩展业务模型和外部数据库。 |
| 风险可控 | 18 / 20 | 已覆盖字段缺失、facts 失败、数组语义、类型比较、阶段重叠等主要风险。 |
| 验收标准明确 | 18 / 20 | 配置校验、正式事件、模拟运行、回归范围均有可测标准。 |
| 架构一致性 | 17 / 20 | 与标准事件、facts、state、四阶段规则引擎和独立中台边界一致。 |

## Score

```text
90
```

## Decision

```text
PASS
```
