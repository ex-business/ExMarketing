# Analysis

## 需求 ID

2026-05-27-rule-formula-script-limit

## Review 反馈处理

本轮已读取 `tasks/2026-05-27-rule-formula-script-limit/review.md`。该文件仍是模板形态，包含 `P0 / P1 / P2`、`Open Questions`、`Decision PASS / BLOCK` 占位，但未填写具体打回意见、问题描述或评分。

| Review 项 | 处理 |
|---|---|
| P0 占位，无具体内容。 | 无可逐条修复的 P0；本分析仍按高风险需求补齐安全边界、错误结构、非目标和验收。 |
| P1 占位，无具体内容。 | 无可逐条修复的 P1；本分析明确本期默认最小交付范围，避免 Dev 扩成任意脚本执行或外部业务库查询。 |
| P2 占位，无具体内容。 | 无可逐条修复的 P2；本分析补充边界、异常、风险和回归验收，供 Review Agent 审核。 |
| Open Questions 为“待填写”。 | 本轮不等待 Owner 决策。对不唯一的范围按“默认最小可验收口径”关闭：本期至少新增并注册一个可运行高级规则执行器，默认选择 `FORMULA`；`SCRIPT` 和未注册/不安全 `LIMIT` 继续拒绝。 |
| Decision 为模板 `PASS / BLOCK`。 | 视为上一轮 Review 未形成有效结论；本轮重新输出可审核需求并给出 Analysis Decision。 |

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版为模板，`Score=0`，`Decision=PASS / BLOCK` 未落定，且背景、目标、业务规则、输入输出、异常、边界、验收均为“待填写”。处理如下：

| 上一版问题 | 修正 |
|---|---|
| 背景和已知信息为空。 | 已补充活动引擎规则体系、当前已支持 `CONDITION`、`FORMULA / LIMIT / SCRIPT` 既有 Schema 边界、固定执行阶段和安全限制依据。 |
| 目标理解为空。 | 已明确本期目标：补齐一个最小可运行高级规则执行器，默认交付 `FORMULA`；未注册或不安全的 `FORMULA / LIMIT / SCRIPT` 必须拒绝。 |
| 业务规则、输入输出、异常和边界为空。 | 已按 `task.md`、`docs/api.md`、`docs/data-model.md`、`docs/architecture.md` 和既有规则 Schema 分析补齐。 |
| 待确认问题可能阻断研发。 | 已给出默认最小可验收口径，并把剩余问题列为不阻断后续需求，不再 Decision BLOCK。 |
| Score 0，不满足进入研发。 | 本轮按评分规则重新评分为 92，Decision PASS。 |

## 资料依据

- `tasks/2026-05-27-rule-formula-script-limit/task.md`：要求补齐 `FORMULA`、`SCRIPT` 或高级 `LIMIT` 的可运行执行器设计与最小实现边界；保持安全沙箱和字段路径限制；未注册或不安全配置必须拒绝；已支持规则有单元测试；错误原因结构化；不得查询外部业务库或执行任意不受控代码。
- `docs/api.md`：当前普通活动只允许已注册 evaluator 的 `CONDITION`；`FORMULA`、`LIMIT`、`SCRIPT` 未注册前即使结构合法也返回 `400 Bad Request`；规则执行顺序固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`；字段路径白名单和错误解释结构已存在。
- `docs/data-model.md`：`marketing_rule` 支持 `CONDITION / FORMULA / LIMIT / SCRIPT` 四类规则；当前已实现 `CONDITION`；规则阶段为 `MATCH / QUALIFY / REWARD / RISK / LIMIT`。
- `docs/architecture.md`：新规则类型通过 evaluator registry 扩展，规则结果需包含 `status`、`passed`、`reasonCode`、`reason`、`field`、`expected`、`actual`、`sort` 等解释字段；引擎只调用已注册 staged rule executor。
- `docs/roadmap.md`：Phase 1 公共内核已具备基础 `CONDITION`；后续活动需要更强配置化能力，但仍要求活动通过配置完成，不新增业务专用逻辑。
- `tasks/2026-05-24-rule-schema/analysis.md`：已定义四类 ruleType 的 Schema、安全边界、字段路径、未注册 evaluator 拒绝和 `enabled=false` 不放宽校验口径。

## 背景目标

ExMarketing 的活动规则目前以 `CONDITION` 为主，能覆盖简单字段判断，但对“金额派生值判断、多个字段组合计算、受控高级限制”的表达能力不足。`marketing_rule` 已预留 `FORMULA`、`LIMIT`、`SCRIPT` 类型，但既有文档明确：未注册 evaluator 或安全能力未开放时必须拒绝，不能默认通过。

本期需求风险等级为高，因为高级规则一旦边界不清，会引入任意代码执行、外部业务库查询、跨租户数据读取、不可解释拒绝、规则默认放行等问题。

默认最小可验收口径：

- 本期必须新增至少一个可运行高级规则执行器。
- 基于安全性和现有资料，默认选择 `FORMULA` 作为本期唯一新增可运行高级规则类型。
- `FORMULA` 只支持确定性、无副作用、字段路径受控的表达式判断；不得执行外部函数、SQL、脚本、HTTP、文件、环境变量、随机数或当前时间。
- `SCRIPT` 本期不开放可执行 inline code；无已注册、已审核、沙箱化脚本注册表时必须拒绝。
- `LIMIT` ruleType 本期不新增外部查询型限制；未注册高级 `LIMIT` evaluator 时继续拒绝。既有 quota-backed `BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT` 语义不在本期改变。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 运营 / 后台配置人员 | 能配置安全、可解释的高级公式规则；非法或未注册规则在创建、更新或上线前被明确拒绝。 |
| 活动引擎 | 只执行已注册 evaluator，按固定阶段产出结构化规则结果，不让错误规则默认通过。 |
| Rule Evaluator Registry | 管理 `ruleType` 到 evaluator 的注册关系；未注册类型必须拒绝。 |
| Fact Provider | 仅在规则字段引用 `facts.*` 且 provider 已启用时提供只读事实；本期规则自身不得直接查询外部业务库。 |
| ActivityPlugin | 只通过 `state.plugin.<pluginCode>.*` 暴露受控上下文，不替代规则执行器安全边界。 |
| Review / Dev / Test Agent | 基于本分析判断范围、实现最小能力、设计测试和验收。 |

## 需求范围

本期范围：

1. 定义并交付 `FORMULA` 的最小可运行规则语义。
2. 保持 evaluator 注册机制：只有已注册且通过安全校验的 `FORMULA` 可保存和运行。
3. 继续拒绝未注册 evaluator，包括未开放的 `SCRIPT` 和未注册高级 `LIMIT`。
4. 所有高级规则读取字段必须复用既有字段路径白名单。
5. 所有拒绝、错误和跳过结果必须结构化，能定位规则、阶段、字段、期望值和实际值。
6. 新增或已支持规则必须有单元测试覆盖合法、非法、安全和边界场景。

本期默认不要求后台 UI 新页面；现有 Admin create/update、上线前校验、模拟运行和正式事件处理链路能消费规则即可。

## 输入输出

### 输入

后台活动配置中的 `rules[]`，重点为 `ruleType=FORMULA`：

```json
{
  "ruleCode": "FORMULA_AMOUNT_BONUS_LIMIT",
  "rulePhase": "QUALIFY",
  "ruleType": "FORMULA",
  "ruleConfig": {
    "formula": "attributes.amount * 0.2",
    "resultType": "DECIMAL",
    "compare": {
      "op": "<=",
      "value": "500.00"
    },
    "scale": 4
  },
  "sort": 10,
  "enabled": true
}
```

通用输入约束：

- `ruleCode` 必填，活动内唯一。
- `rulePhase` 必填，只能是 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`。
- `ruleType` 必填；本期可运行新增范围仅为已注册 `FORMULA`。
- `ruleConfig` 必须是 JSON object，拒绝 `null`、数组、字符串、数字、布尔。
- `enabled=false` 只表示运行时跳过，不放宽保存和上线前 Schema / 安全校验。

### 输出

成功保存或运行时：

- 活动查询能读回原始规则配置。
- 模拟运行和正式运行的 `ruleResult` 中包含 `ruleCode`、`rulePhase`、`ruleType`、`status`、`passed`、`reasonCode`、`reason`、`field` 或 `formula`、`expected`、`actual`、`sort`。
- `FORMULA` 计算结果满足 `compare` 时该规则 `PASSED`；不满足时 `REJECTED`，且活动按既有规则阶段语义处理。
- `enabled=false` 的合法规则运行时为 `SKIPPED`，不影响活动决策。

失败时：

- 创建、更新或上线前校验返回 400 类结构化错误，至少定位到 `rules[index]`、`ruleCode`、字段名、错误码和原因。
- 正式运行遇到历史遗留非法规则或执行异常时，不得默认通过；应返回规则级 `ERROR` 或活动级拒绝，并给出结构化 `reasonCode`。
- 非法配置不得部分保存为可运行规则。

## FORMULA 业务规则

### 表达式能力

`FORMULA` 用于对标准事件、attributes、facts、state 或插件上下文中的字段做确定性计算，并将计算结果与期望值比较。

本期最小支持：

- 字段引用：必须来自字段路径白名单。
- 字面量：decimal / integer / boolean / string。
- 算术运算：`+`、`-`、`*`、`/`，仅对数值类型开放。
- 括号：允许用于改变算术优先级。
- 一元负号：允许用于数值。
- 比较：通过 `compare.op` 表达，支持 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。
- Decimal 语义：金额和数值计算必须使用 Decimal 口径，不能用 float 近似导致精度漂移。

本期不支持：

- 函数调用。
- SQL、正则、循环、条件分支、变量赋值、数组/对象构造。
- HTTP、数据库、文件系统、环境变量、secret、系统时间、随机数。
- 跨租户数据读取。
- 直接读取外部业务库或业务系统 SDK。

### ruleConfig Schema

`FORMULA` 顶层只允许：

| 字段 | 要求 |
|---|---|
| `formula` | 必填字符串，表达式中所有字段引用必须命中字段路径白名单。 |
| `resultType` | 必填，只能是 `DECIMAL`、`INTEGER`、`BOOLEAN`、`STRING`。本期可运行最小口径必须支持 `DECIMAL` 和 `BOOLEAN`；`INTEGER`、`STRING` 若未实现 evaluator 能力，必须结构化拒绝而非默认通过。 |
| `compare` | 可选 object；存在时只允许 `op`、`value`。若缺省，默认要求 `resultType=BOOLEAN` 且公式结果为 `true` 才通过。 |
| `scale` | 可选非负整数，仅 `DECIMAL` 时允许。默认输出 4 位小数，超出平台上限时拒绝。 |

`compare` 规则：

- `op` 必填，复用条件比较操作符。
- `value` 必填。
- `IN / NOT_IN` 的 `value` 必须是非空数组。
- `>`、`>=`、`<`、`<=` 只能用于数值结果。
- `BOOLEAN` 结果只允许等值判断，或在缺省 `compare` 时以 `true` 为通过。
- `STRING` 结果只允许等值、非等值、`IN`、`NOT_IN`。

### 字段路径规则

`FORMULA` 字段引用必须复用既有规则白名单：

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

约束：

- 非标准顶层业务字段必须拒绝，例如 `amount`、`orderAmount`、`deposit.amount`、`user.vipLevel`。
- `facts.<factCode>.*` 只能通过同租户已启用 Fact Provider 获取；规则不得在公式执行器里自定义 provider、endpoint、SQL 或请求参数。
- `state.plugin.<pluginCode>.*` 只允许活动绑定对应插件且插件已注册时读取。
- 字段缺失、值为 `null`、类型不匹配不得当作 `0`、空字符串或 `false` 默认通过。

## SCRIPT 业务规则

本期默认不开放 `SCRIPT` 可执行能力。原因：当前材料没有脚本注册表、审核流程、沙箱资源限制、语言运行时、权限模型和审计规范；直接开放会违反 task 中“不得执行任意不受控代码”。

最小口径：

- `ruleType=SCRIPT` 如果 evaluator、脚本注册表、脚本审核状态或沙箱能力任一缺失，创建、更新和上线前校验必须拒绝。
- 不接受 inline script 源码。
- 不接受 JavaScript、SQL、Shell、Lua、Python 等任意执行脚本作为本期默认能力。
- 即使未来开放，也只能引用已注册、已审核、受版本控制的 `scriptRef`，并显式声明输入字段白名单、超时和资源上限。

## LIMIT 业务规则

本期不改变既有 quota-backed 限制语义，也不把 `BUDGET` 误作为规则阶段。

最小口径：

- `rulePhase=LIMIT` 仍是合法阶段。
- `ruleType=LIMIT` 只有在对应 evaluator 已注册并通过安全校验时才允许保存和运行。
- 未注册高级 `LIMIT` evaluator 时，`ruleType=LIMIT` 必须拒绝，不能默认通过。
- 既有 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 的原子扣减、预览和 `QUOTA_EXCEEDED` 错误仍属于 quota / 发奖链路，不在本期重新定义。
- 高级 `LIMIT` 不得查询外部业务库；如需历史参与、周期统计或外部画像，必须来自已建模的 `state.*`、quota 快照或已注册 Fact Provider。

## 执行阶段和决策规则

固定业务执行顺序不变：

```text
MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET
```

规则：

1. `FORMULA` 可以配置在合法 `rulePhase` 中，按同阶段 `sort` 升序执行。
2. 同一阶段任一启用规则 `REJECTED` 时，该活动按既有阶段语义拒绝，不继续生成发奖结果。
3. 任一启用规则 `ERROR` 时不得默认通过；最小口径按活动拒绝处理，并输出结构化错误。
4. `enabled=false` 的规则为 `SKIPPED`，不参与通过/拒绝决策。
5. 未注册 evaluator 在保存、更新或上线前应被拒绝；如历史数据绕过校验进入运行时，也必须 `ERROR`，不得 `PASSED`。

## 异常场景

| 场景 | 期望 |
|---|---|
| `FORMULA` evaluator 未注册 | 创建、更新或上线前返回 400 类错误，`reasonCode=RULE_EVALUATOR_NOT_REGISTERED` 或等价稳定码。 |
| `SCRIPT` 使用 inline code | 返回脚本配置不安全，拒绝保存和上线。 |
| `LIMIT` evaluator 未注册 | 返回 evaluator 不可用，拒绝保存和上线。 |
| `formula` 为空字符串 | 返回 `FORMULA_INVALID` 或等价稳定码。 |
| `formula` 引用 `orderAmount` | 返回字段路径非法，提示业务字段必须走 `attributes.*`。 |
| `formula` 引用 `facts.USER.vip` 但 provider 未启用 | 返回 fact provider 不可用或上线前校验失败。 |
| `formula` 包含 `http()`、`sql()`、`now()`、`random()`、文件或环境变量语义 | 返回不安全表达式，拒绝。 |
| 数值公式中字段为字符串且不可解析 Decimal | 运行结果为 `ERROR` 或校验拒绝，不得按 0 通过。 |
| 除以 0 | 规则级 `ERROR`，活动不得默认通过。 |
| `compare.op=LIKE` | 返回操作符非法。 |
| `compare` 缺省且 `resultType=DECIMAL` | 返回配置非法；缺省 compare 只允许 boolean 公式。 |
| `enabled=false` 但 `ruleConfig` 不安全 | 仍在创建、更新或上线前拒绝；禁用不绕过安全校验。 |
| 历史非法规则运行时被加载 | 规则结果为 `ERROR`，结构化输出原因，不生成默认通过结果。 |

## 边界条件

- Decimal 输出默认 4 位小数；与奖励计算的 Decimal 语义保持一致。
- `scale` 必须是非负整数；默认最小口径允许 `0..8`，超过上限拒绝。
- 表达式长度默认最大 1024 字符；超过拒绝，避免复杂表达式拖垮运行时。
- 字段引用数量默认最大 20 个；超过拒绝，避免规则不可审核。
- 嵌套括号深度默认最大 8 层；超过拒绝。
- `IN / NOT_IN` 数组默认最大 100 项，且元素不能是 object、array 或 null。
- `occurredAt` 本期不要求公式做时间大小比较；如作为字符串等值判断可用，时间运算另立需求。
- 模拟运行不得写 event inbox、participation、grant_order、outbox 或扣 quota；只输出公式计算和比较预览。
- 正式运行不得因为公式错误生成 grant_order/outbox 或扣 quota。

## 非目标范围

- 不开放任意脚本执行。
- 不实现脚本注册表、脚本审核流、脚本市场、脚本版本发布或脚本沙箱资源调度。
- 不实现 SQL、HTTP、RPC、文件、环境变量或外部业务库查询。
- 不新增业务专用 controller、业务专用字段模型或接入方硬编码逻辑。
- 不改变标准事件模型、幂等键、活动匹配、奖励计算、quota reservation、grant_order、outbox、callback、DLQ 的既有业务语义。
- 不把 `BUDGET` 新增为合法 `rulePhase`。
- 不实现完整高级 `LIMIT` 历史统计语言；后续如需周期、IP、Device、跨活动历史互斥，应另立需求。
- 不要求新增后台 UI 页面；错误结构和 API 行为需能被现有后台消费。

## 风险

| 风险 | 等级 | 控制方式 |
|---|---|---|
| 任意代码执行 | P0 | 本期 `SCRIPT` 默认拒绝；`FORMULA` 禁止函数、SQL、HTTP、文件、环境变量和随机能力。 |
| 外部业务库查询 | P0 | 规则执行器只读标准上下文、`facts.*`、`state.*` 和插件上下文；Fact Query 只能走 registry provider。 |
| 错误规则默认通过 | P0 | 未注册 evaluator、字段缺失、类型错误、除零、非法 op 均不得 `PASSED`。 |
| Decimal 精度错误 | P1 | 数值公式使用 Decimal 语义，输出和比较不使用 float 近似。 |
| 范围膨胀成脚本平台 | P1 | 明确非目标，`SCRIPT` 只保留拒绝和安全边界。 |
| `LIMIT` 与 quota 混淆 | P1 | 保持 quota-backed 限制语义不变，未注册 `LIMIT` evaluator 拒绝。 |
| 字段路径过宽 | P1 | 复用白名单，拒绝非标准顶层业务字段和插件越权路径。 |
| 测试不足 | P2 | 验收要求覆盖合法公式、不安全公式、未注册 evaluator、结构化错误和运行解释。 |

## 验收标准

1. 已注册 `FORMULA` evaluator 后，合法 `FORMULA` 规则可通过创建、更新或上线前校验，并在模拟运行和正式运行中参与决策。
2. `FORMULA` 支持至少一个 Decimal 计算场景，例如 `attributes.amount * 0.2 <= 500.00`，计算和比较使用 Decimal 语义。
3. `FORMULA` 支持 boolean 公式缺省 `compare` 的场景；结果为 `true` 通过，`false` 拒绝。
4. `FORMULA` 引用标准事件字段和 `attributes.*` 合法；引用 `orderAmount`、`deposit.amount`、`user.vipLevel` 等非标准顶层业务字段必须拒绝。
5. `FORMULA` 引用 `facts.<factCode>.*` 时，只允许同租户已启用 Fact Provider；未启用时创建、更新、上线前或运行时必须给出结构化失败原因。
6. `FORMULA` 包含函数调用、SQL、HTTP、文件、环境变量、随机数、当前时间、循环或赋值语义时必须拒绝。
7. `FORMULA` 类型不匹配、Decimal 解析失败、除以 0、非法 `compare.op` 不得默认通过。
8. 未注册 `FORMULA` evaluator 的环境中，`ruleType=FORMULA` 即使 Schema 合法也必须拒绝。
9. `ruleType=SCRIPT` 在本期默认拒绝；inline script 必须拒绝并返回不安全配置原因。
10. 未注册高级 `LIMIT` evaluator 时，`ruleType=LIMIT` 必须拒绝；不得与 quota `BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT` 混淆。
11. `enabled=false` 的合法高级规则运行时 `SKIPPED`；`enabled=false` 的非法或不安全配置仍必须拒绝保存和上线。
12. 模拟运行输出包含公式计算值、比较期望、实际值、规则状态和 reason code，且不写业务数据或扣 quota。
13. 正式运行中公式拒绝或错误时，不生成 grant_order/outbox，不扣 quota，不默认发奖。
14. 错误响应和规则结果必须结构化，能定位 `rules[index]`、`ruleCode`、`rulePhase`、`ruleType`、字段或公式、`reasonCode`、`reason`。
15. 单元测试必须覆盖：合法 Decimal 公式、合法 boolean 公式、字段白名单、非法字段、非法 op、除零、类型不匹配、未注册 evaluator、`SCRIPT` 拒绝、`LIMIT` 未注册拒绝、`enabled=false` 不绕过校验。
16. 回归测试必须确认既有 `CONDITION` 规则、规则执行顺序、`RISK_LIMIT_BUDGET` 阶段展示、quota 错误码和 Fact Query 安全边界未被破坏。

## 待确认问题

以下问题不阻断本轮进入 Review / Dev；如 Owner 后续选择不同方向，应另立需求或重新分析。

1. `FORMULA` 表达式最终语言是否采用 CEL、自研表达式或第三方安全表达式库。默认口径：需求只规定业务语义和安全边界，不指定实现库。
2. 是否要在本期同时开放高级 `LIMIT` evaluator。默认口径：不开放；未注册时拒绝，既有 quota 限制保持不变。
3. 是否未来开放 `SCRIPT`。默认口径：不开放 inline script；无注册表、审核和沙箱前全部拒绝。
4. `INTEGER`、`STRING` resultType 是否本期必须运行。默认口径：本期可运行最小范围为 `DECIMAL` 和 `BOOLEAN`；其他 resultType 若未实现必须结构化拒绝。

## Score

```text
92
```

评分：

| 维度 | 分数 | 理由 |
|---|---:|---|
| 需求清晰度 | 19/20 | 默认最小交付、参与方、输入输出、规则执行语义和非目标已明确。 |
| 业务规则完整 | 18/20 | `FORMULA` 可运行语义完整；`SCRIPT` 和 `LIMIT` 按安全默认拒绝收口。 |
| 边界异常清楚 | 19/20 | 覆盖未注册、不安全表达式、字段越权、类型错误、除零、禁用规则和历史非法规则。 |
| 验收标准明确 | 19/20 | 验收可直接转化为单元测试、模拟运行测试和回归测试。 |
| 约束一致性 | 17/20 | 保持 evaluator registry、字段白名单、Fact Query registry、quota 语义和固定执行阶段一致；表达式语言实现细节留给 Dev 选择。 |

## Decision

```text
PASS
```

结论：上一版 Analysis 的模板性 BLOCK 已关闭，Review 模板占位已逐项处理。本期按“新增可运行 `FORMULA` 执行器，`SCRIPT` 和未注册高级 `LIMIT` 继续拒绝”的默认最小可验收口径推进，需求细节清楚、可审核、可开发、可测试。
