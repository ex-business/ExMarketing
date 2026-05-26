# Analysis

## 需求 ID

2026-05-24-rule-schema

## Review 反馈处理

本轮已重新读取 `tasks/2026-05-24-rule-schema/review.md`，Review 结论为 `BLOCK`，Score `72`。

| Review 项 | 处理 |
|---|---|
| P1：字段路径白名单遗漏标准事件字段。 | 已修正。字段路径规则、`CONDITION` Schema、业务规则、异常场景和验收标准均补充标准事件字段白名单：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`、`occurredAt`。同时保留 `attributes.*`、`facts.<factCode>.*`、`state.*`、`state.plugin.<pluginCode>.*` 等受控路径，并继续明确拒绝非标准顶层业务字段，如 `orderAmount`、`deposit.amount`、`user.vipLevel`。依据为 `22-独立活动营销中台设计.md` 的“活动系统只固定识别”字段清单，以及 `docs/api.md` 的标准事件模型。 |
| P2：`enabled=false` 非法 Schema 保存口径不一致。 | 已修正。当前资料只说明 Admin 创建/更新活动会做基础 Schema 校验，未发现独立规则草稿接口；因此本任务最小口径改为：只要通过当前 Admin create/update 或上线前校验提交 `rules[]`，无论 `enabled` 是 `true` 还是 `false`，`ruleConfig` 都必须满足对应 Schema，非法 Schema 一律拒绝保存为活动规则。`enabled=false` 仅表示运行时跳过一条已合法保存的规则，不代表允许保存非法 JSON 草稿。 |
| Open Question 1：是否保持标准事件字段兼容。 | 按 Review 建议处理为必须兼容，不再作为阻断问题。 |
| Open Question 2：是否存在单独草稿接口承载非法 ruleConfig。 | 从当前 `task.md`、`docs/api.md` 和设计文档无法推导出独立草稿规则接口；本轮明确当前 Admin create/update 直接拒绝非法 Schema。未来若新增专门草稿接口，应另立需求定义其保存、预览、上线阻断和错误展示语义。 |

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版自身 `Decision PASS`、Score `90`，但存在被 Review 打回后必须修正的错误口径。

| 上一版问题 | 修正 |
|---|---|
| 字段路径白名单只写 `attributes.* / facts.* / state.* / state.plugin.*`，遗漏标准事件字段。 | 增加“标准事件字段”和“扩展/状态字段”两组白名单，明确 `eventType`、`subject.id`、`biz.bizId` 等可作为 `CONDITION` 字段。 |
| `enabled=false` 非法 Schema 可作为草稿保存，与创建/更新/上线前校验全量拒绝冲突。 | 改为当前 Admin create/update 和上线前校验均全量校验 `rules[]`；`enabled=false` 只影响运行时决策，不放宽保存校验。 |
| 待确认问题把 `enabled=false` 草稿语义作为非阻断默认口径，容易让 Dev Agent 实现成允许非法规则落库。 | 删除该默认口径，改为风险和非目标说明：非法草稿接口不属于本任务；若未来需要，另行定义。 |

## 资料依据

- `tasks/2026-05-24-rule-schema/task.md`：要求完善 `marketing_rule` 的 `rule_phase`、`rule_type`、`rule_config_json`、`sort`、`enabled`、`version`，并定义 `CONDITION / FORMULA / LIMIT / SCRIPT` JSON Schema。
- `requirements/backlog.json`：本需求处于 `REVIEW_BLOCK`，验收重点是非法字段、非法 op、非法 value 类型被后台校验拒绝。
- `22-独立活动营销中台设计.md`：标准事件固定识别 `eventId / eventType / sourceSystem / tenantId / subject.type / subject.id / biz.bizType / biz.bizId / occurredAt / attributes`；业务字段进入 `attributes`；`SCRIPT` 为受控脚本，默认不建议开放。
- `docs/api.md`：事件模型只允许 8 个顶层字段；业务扩展字段必须放入 `attributes`；当前 Admin 创建活动会进行基础 Schema 校验；`BUDGET` 不是合法 `rulePhase`；字段缺失、类型不匹配、非法操作符或未注册 evaluator 不会默认通过。
- `docs/data-model.md`：`marketing_rule` 字段、合法 `rule_phase`、当前 `CONDITION` 操作符、当前已实现 `CONDITION`。
- `docs/architecture.md`：规则运行顺序为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`；禁用规则报告为 `SKIPPED` 且不影响决策；新规则类型通过 evaluator registry 扩展。

## 背景目标

ExMarketing 是独立活动营销中台，活动规则必须通过通用配置表达，不能把会员、订单、充值等具体业务模型硬编码进活动系统。本任务补齐 `marketing_rule` 的配置契约和后台校验，使错误规则在创建、更新或上线前被拒绝，避免非法配置进入运行链路后被默认通过或产生不可解释结果。

目标：

- 明确 `rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version` 的合法范围。
- 定义 `CONDITION`、`FORMULA`、`LIMIT`、`SCRIPT` 四类 `ruleType` 的最小 JSON Schema。
- 明确字段路径白名单，既保持标准事件字段兼容，又禁止非标准顶层业务字段。
- 明确未知字段、非法枚举、非法字段路径、非法操作符、非法 value 类型、未注册 evaluator 的错误口径。
- 保持规则系统与外部业务系统解耦，只通过标准事件、`attributes`、facts、活动状态和插件上下文读取数据。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 运营 / 后台配置人员 | 配置规则时获得明确校验错误，不能把非法规则提交为活动配置。 |
| 活动引擎 | 只消费已校验规则，按固定阶段执行并输出可解释结果。 |
| Rule Evaluator | 按 `ruleType` 处理对应 Schema；未注册 evaluator 不得默认通过。 |
| Fact Provider | 为 `facts.<factCode>.*` 提供可选事实；未配置或未启用时不得让依赖该 fact 的规则上线。 |
| ActivityPlugin | 只暴露 `state.plugin.<pluginCode>.*` 上下文，不替代通用规则、发奖、quota 和幂等流程。 |
| 审批 / 上线前校验 | 复用本任务 Schema 结果，阻断非法或高危配置。 |
| 外部业务系统 | 只上报标准事件、可选 facts 和接收发奖指令，不被规则系统反向绑定数据库或代码包。 |

## 输入输出

### 输入

后台活动配置中的 `rules[]`。

| 字段 | 要求 |
|---|---|
| `ruleCode` | 必填，活动内唯一，用于审计、解释结果和错误定位。 |
| `rulePhase` | 必填，只能是 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`。 |
| `ruleType` | 必填，本任务定义 `CONDITION`、`FORMULA`、`LIMIT`、`SCRIPT`。 |
| `ruleConfig` / `ruleConfigJson` | 必填 JSON object，按 `ruleType` 校验；拒绝数组、字符串、数字、布尔、null。 |
| `sort` | 整数，缺省按数据模型默认 `0`；同阶段按升序执行。最小口径拒绝负数。 |
| `enabled` | 布尔，缺省按数据模型默认 `true`；`false` 只影响运行时跳过，不放宽保存校验。 |
| `version` | 正整数，缺省按数据模型默认 `1`；用于配置追溯和运行快照。 |

### 输出

成功时：

- 规则配置被保存，并在查询活动时保留 `ruleCode`、`rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version`。
- 运行解释中保留规则编码、阶段、类型、顺序、启用状态、判断结果和失败原因。
- `enabled=false` 的合法规则在运行解释中可展示为 `SKIPPED`，不得改变活动是否通过。

失败时：

- 当前 Admin 创建、更新或上线前校验返回 400 类校验错误。
- 错误至少定位到 `rules[index]`、字段名、错误码和原因。
- 非法配置不得部分保存为可运行活动规则；`enabled=false` 也不得绕过 Schema 校验。

## 规则阶段

合法存储阶段：

```text
MATCH
QUALIFY
REWARD
RISK
LIMIT
```

业务执行阶段：

```text
MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET
```

约束：

- `RISK_LIMIT_BUDGET` 是运行解释阶段名，不是可配置 `rulePhase`。
- `BUDGET` 是 quota 类型，不是合法 `rulePhase`。
- 同一 `rulePhase` 内按 `sort` 升序执行；`sort` 相同应保持稳定顺序或按规则编码稳定排序，避免随机执行。
- 阶段不合法时必须拒绝配置，不能降级到默认阶段。

## 字段路径规则

字段路径分两类：标准事件字段和扩展/状态字段。`CONDITION.field`、`FORMULA.formula` 内引用、`SCRIPT.inputs[]` 和 `LIMIT.field` 涉及字段读取时均必须使用同一白名单。

### 标准事件字段

以下字段必须保持兼容，可直接作为规则字段路径：

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
```

说明：

- 这些字段来自标准事件模型，是活动系统固定识别字段。
- `occurredAt` 是 RFC3339 时间字符串；可用于等值判断，是否支持大小比较取决于时间比较能力是否明确注册。最小口径不要求本任务新增时间比较语义。
- `attributes` 作为顶层 object 不应直接参与普通比较；业务字段应使用 `attributes.<path>`。

### 扩展和状态字段

允许字段路径：

```text
attributes.<path>
facts.<factCode>.<path>
state.activity.<field>
state.subject.<field>
state.quota.<quotaType>.<field>
state.exclusive.<field>
state.plugin.<pluginCode>.<path>
```

约束：

- `attributes.<path>` 是业务扩展字段唯一入口；可以表达 `attributes.amount`、`attributes.vipLevel`，但不能写成顶层 `amount`、`vipLevel`、`deposit.amount` 或 `user.vipLevel`。
- `facts.<factCode>.<path>` 必须匹配同租户已启用 Fact Provider；未配置或未启用 provider 时，依赖该 fact 的规则不得通过创建、更新或上线前校验。
- `state.plugin.<pluginCode>.<path>` 仅在活动绑定对应 `pluginCode`、插件已注册且与 `activityType` 兼容时允许。
- 禁止 `plugin.*`、`state.<activityType>.*`、SQL 表字段、外部业务模型强类型路径和非标准顶层业务字段。
- 字段缺失、值为 `null`、类型不匹配不得按默认值通过。

## ruleType Schema

### CONDITION

用途：声明式条件判断，是当前普通活动主要可执行规则类型。

Schema 口径：

```json
{
  "logic": "AND",
  "conditions": [
    {
      "field": "eventType",
      "op": "=",
      "value": "DEPOSIT_SUCCESS"
    },
    {
      "field": "attributes.amount",
      "op": ">=",
      "value": "100.00"
    }
  ]
}
```

规则：

- 顶层仅允许 `logic`、`conditions`。
- `logic` 必填，只能是 `AND` 或 `OR`。
- `conditions` 必填，必须是非空数组。
- 每个 condition 仅允许 `field`、`op`、`value`。
- `field` 必填，必须命中字段路径白名单；合法示例包括 `eventType`、`subject.id`、`biz.bizId`、`attributes.amount`、`facts.USER_PROFILE.vipLevel`、`state.subject.id`。
- `op` 必填，只能是 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。
- `value` 必填，允许 string、number、boolean；`IN / NOT_IN` 必须使用非空数组。
- 比较操作符 `>`、`>=`、`<`、`<=` 只允许数字或可解析 Decimal 字符串等明确可比较值；不能用于 boolean、object、array。
- `IN / NOT_IN` 的数组元素类型必须与字段预期类型兼容；不得混用 object、array、null。
- 未知顶层字段或 condition 内未知字段必须拒绝。

### FORMULA

用途：表达可控派生值计算或公式判断，面向后续 evaluator 扩展。本任务只定义配置 Schema 和校验边界，不要求把未注册公式 evaluator 变成可执行能力。

最小 Schema 口径：

```json
{
  "formula": "attributes.amount * 0.2",
  "resultType": "DECIMAL",
  "compare": {
    "op": ">=",
    "value": "100.00"
  },
  "scale": 2
}
```

规则：

- 顶层仅允许 `formula`、`resultType`、`compare`、`scale`。
- `formula` 必填，必须是字符串；字段引用必须命中字段路径白名单。
- 禁止 SQL、外部函数调用、HTTP、文件访问、环境变量、secret、随机数、当前时间等非确定性或越权能力。
- `resultType` 必填，只能是 `DECIMAL`、`INTEGER`、`BOOLEAN`、`STRING`。
- `scale` 可选，仅 `DECIMAL` 时允许，必须是非负整数。
- `compare` 可选；存在时仅允许 `op`、`value`，`op` 复用 `CONDITION` 的比较类或等值类操作符，类型必须与 `resultType` 兼容。
- 如果系统没有注册 `FORMULA` evaluator，当前 Admin create/update 和上线前校验必须拒绝 `FORMULA` 规则；不能保存成会默认通过的活动规则。

### LIMIT

用途：表达次数、周期、互斥、用户维度限制等规则配置。它是 `ruleType=LIMIT`，可与 `rulePhase=LIMIT` 组合，但两者不是同一字段。

最小 Schema 口径：

```json
{
  "limitType": "USER_COUNT",
  "scope": "USER",
  "window": "ACTIVITY",
  "max": 1,
  "field": "subject.id"
}
```

规则：

- 顶层仅允许 `limitType`、`scope`、`window`、`max`、`field`、`resetTimezone`。
- `limitType` 必填，最小允许 `USER_COUNT`、`DAILY_COUNT`、`PERIOD_COUNT`、`EXCLUSIVE_GROUP`。
- `scope` 必填，最小允许 `USER`、`ACTIVITY`、`TENANT`、`BIZ`。
- `window` 必填，最小允许 `ACTIVITY`、`DAY`、`PERIOD`。
- `max` 必填，必须是大于等于 0 的整数。
- `field` 可选；存在时必须符合字段路径白名单，常见为 `subject.id`、`biz.bizId`、`state.subject.id` 或明确的状态字段。
- `resetTimezone` 可选；存在时必须是可识别 IANA 时区名或 `UTC`。
- 如果系统没有注册 `LIMIT` evaluator，当前 Admin create/update 和上线前校验必须拒绝 `LIMIT` ruleType；不得与 quota `BUDGET / STOCK / USER_LIMIT` 的原子扣减能力混淆。

### SCRIPT

用途：受控脚本扩展。设计文档明确 `SCRIPT` 默认不建议开放，因此本任务只定义安全配置边界和默认拒绝策略。

最小 Schema 口径：

```json
{
  "scriptRef": "risk_rule_v1",
  "language": "CEL",
  "inputs": ["eventType", "attributes.amount", "subject.id"]
}
```

规则：

- 顶层仅允许 `scriptRef`、`language`、`inputs`、`timeoutMs`。
- `scriptRef` 必填，必须引用已注册、已审核、同租户或全局允许的脚本标识；不得接收 inline script 源码。
- `language` 必填，最小允许 `CEL`；未明确沙箱和审核机制前不得允许 JavaScript、SQL、Shell、Lua 等任意执行脚本。
- `inputs` 必填，必须是非空字符串数组，每个元素必须符合字段路径白名单。
- `timeoutMs` 可选，必须是正整数，且不得超过平台默认上限。
- 默认策略：未注册 `SCRIPT` evaluator、未配置脚本注册表或脚本未审核时，当前 Admin create/update 和上线前校验必须拒绝 `SCRIPT` 规则。
- 禁止 inline code、SQL、外部 HTTP、文件系统、环境变量、secret 读取、随机数、当前时间和跨租户数据访问。

## 业务规则

1. 活动规则配置必须使用 `ruleType` 匹配对应 Schema；不能用 `CONDITION` 的结构冒充 `FORMULA`、`LIMIT` 或 `SCRIPT`。
2. 后台 JSON decode 和 Schema 校验必须拒绝未知字段，避免拼写错误被静默忽略。
3. 当前普通活动运行链路已实现 `CONDITION`；`FORMULA`、`LIMIT`、`SCRIPT` 只有在对应 evaluator 和安全边界注册后才能作为可执行规则保存和上线。
4. 当前 Admin create/update 不存在独立非法草稿规则语义；一旦请求包含 `rules[]`，无论每条规则 `enabled` 为何，都必须全量校验 Schema。
5. `enabled=false` 的合法规则运行时应跳过并可展示为 `SKIPPED`，不影响决策；它不代表允许保存非法 `ruleConfig`。
6. 未注册 evaluator、非法 `ruleType`、非法 `rulePhase`、非法字段路径、非法操作符、非法 value 类型均不得默认通过。
7. 规则校验不应连接外部业务数据库，不应引用外部业务代码，不应内置会员、订单、充值等强业务模型。
8. 规则版本 `version` 必须随可变更规则配置保持可追溯；命中活动时的 snapshot 应能反映当时规则版本和配置摘要。
9. 运行时若遇到历史遗留异常配置，必须失败为拒绝或错误结果，不得按通过处理。

## 边界条件

- 空 `rules` 是否允许由活动类型决定；本任务不强制所有活动必须有规则，但一旦提供规则就必须全量校验。
- `conditions` 空数组非法；否则 `AND` 空集可能被误判为通过。
- `IN / NOT_IN` 空数组非法；否则业务含义不可审核。
- Decimal 字符串必须可解析；金额或数值比较不得使用 float 近似语义。
- `sort` 可为 0，但不得为非整数；最小口径拒绝负数。
- 重复 `ruleCode` 会导致解释结果和审计不可读，活动内应拒绝重复。
- 同一规则中字段路径大小写必须精确匹配，不做宽松纠错。
- `facts.*` 字段在 provider 禁用后，依赖该 fact 的规则不得继续通过创建、更新或上线前校验。
- 插件字段只允许对应插件命名空间，不能越权读取其他插件上下文。
- 标准事件字段兼容不等于开放任意顶层字段；`eventType` 合法，`orderAmount`、`deposit.amount`、`user.vipLevel` 非法。

## 异常场景

| 场景 | 期望 |
|---|---|
| `rulePhase=BUDGET` | 返回规则阶段非法，拒绝配置。 |
| `rulePhase=RISK_LIMIT_BUDGET` | 返回规则阶段非法，提示其为运行解释阶段而非存储阶段。 |
| `ruleType=UNKNOWN` | 返回规则类型非法，拒绝配置。 |
| `ruleConfig` 为 null、数组或字符串 | 返回规则配置类型非法，拒绝配置。 |
| `enabled=false` 但 `ruleConfig` Schema 非法 | 当前 Admin create/update 仍返回 Schema 校验错误，拒绝保存。 |
| `CONDITION.logic=XOR` | 返回 logic 非法。 |
| `CONDITION.op=LIKE` | 返回 op 非法。 |
| `CONDITION` 使用 `>` 比较 boolean | 返回 value 类型或字段类型不匹配。 |
| `IN` 的 value 不是数组或为空数组 | 返回 value 类型非法。 |
| 字段为 `eventType`、`subject.id`、`biz.bizId` | 按标准事件字段处理为合法字段路径。 |
| 字段为 `orderAmount`、`user.vipLevel`、`deposit.amount` | 返回字段路径非法，提示业务字段必须放入 `attributes.*`。 |
| 字段为 `facts.USER_PROFILE.vipLevel` 但 provider 未启用 | 返回 fact provider 不可用。 |
| `SCRIPT` 提供 inline 源码 | 返回脚本配置非法。 |
| `FORMULA` 包含 SQL 或外部函数 | 返回公式配置非法。 |
| evaluator 未注册 | 返回规则 evaluator 不可用，拒绝创建、更新或上线。 |

## 非目标范围

- 不实现新的规则运行算法，不扩大未注册 evaluator 的执行能力。
- 不实现完整公式表达式引擎。
- 不开放任意脚本执行，不设计脚本审核、沙箱、脚本市场或脚本运行时。
- 不实现新的规则草稿接口；非法 `ruleConfig` 草稿保存、草稿预览和草稿上线阻断需另立需求。
- 不实现新的 quota 扣减模型；`BUDGET`、`STOCK`、`USER_LIMIT` 的原子扣减仍属于 quota / 发奖链路。
- 不改变活动匹配、奖励计算、participation、grant_order、outbox、callback、DLQ 的既有业务流程。
- 不为具体业务系统新增硬编码字段、controller、model、job 或外部数据库查询。
- 不要求本任务新增后台页面交互，只要求后端配置契约能被后台页面消费。

## 风险

| 风险 | 等级 | 控制方式 |
|---|---|---|
| 字段路径收窄导致标准事件字段能力回退 | 高 | 明确标准事件字段白名单，验收覆盖 `eventType`、`subject.id`、`biz.bizId`。 |
| `enabled=false` 被误用为非法规则草稿 | 中 | 明确当前 create/update 全量校验，禁用只影响运行时跳过。 |
| `FORMULA / LIMIT / SCRIPT` 被定义后被误认为已可安全执行 | 高 | 明确只有注册 evaluator 且通过安全校验后才能保存和上线；未注册不得默认通过。 |
| `SCRIPT` 引入远程执行或越权读取 | 高 | 默认不开放 inline script，仅允许已注册 `scriptRef`；无注册表和沙箱时阻断。 |
| 字段路径过宽导致绑定业务模型 | 中 | 只允许标准事件字段和白名单前缀，禁止业务短路径和外部表字段。 |
| value 类型校验不足导致运行时比较异常 | 中 | 按 op 和字段类型做校验；比较类操作符只允许可比较值。 |
| `BUDGET` 被误配置为规则阶段 | 中 | 明确拒绝，`BUDGET` 仅为 quota 类型。 |

## 验收标准

1. 创建或更新活动时，合法 `CONDITION` 规则可保存，查询后保留 `ruleCode`、`rulePhase`、`ruleType`、`ruleConfig`、`sort`、`enabled`、`version`。
2. `rulePhase` 只接受 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`；`BUDGET` 和 `RISK_LIMIT_BUDGET` 均被拒绝。
3. `CONDITION` 只接受 `AND / OR`、非空 `conditions` 和白名单操作符。
4. `CONDITION` 中未知字段、未知 op、缺少 `field/op/value`、`IN / NOT_IN` 非数组或空数组均返回明确 400 类校验错误。
5. 字段路径必须接受标准事件字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`、`occurredAt`。
6. 字段路径必须接受受控扩展路径：`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、合法 `state.plugin.<pluginCode>.*`。
7. 字段路径必须拒绝非标准顶层业务字段和业务短路径，例如 `orderAmount`、`deposit.amount`、`user.vipLevel`。
8. `facts.<factCode>.*` 在 provider 未配置或未启用时不得通过创建、更新或上线前校验。
9. `FORMULA`、`LIMIT`、`SCRIPT` 的配置结构按本分析 Schema 校验；未知顶层字段被拒绝。
10. `FORMULA / LIMIT / SCRIPT` 在 evaluator 未注册或安全能力未开放时，不得通过当前 Admin create/update 或上线前校验。
11. `SCRIPT` 不接受 inline code、SQL、Shell、HTTP 调用或未审核 `scriptRef`。
12. `enabled=false` 的合法规则运行时展示为跳过或不参与决策；`enabled=false` 的非法 Schema 仍必须在当前 Admin create/update 被拒绝。
13. 非法规则配置不得部分落库成可运行活动；错误响应能定位到规则数组下标和字段。
14. 文档需要说明四类 ruleType Schema、合法阶段、合法字段路径、合法操作符、未注册 evaluator 和 `enabled=false` 的处理口径。
15. 回归验证至少覆盖：合法标准事件字段 `eventType`、`subject.id`、`biz.bizId`；合法 `attributes.*`；非法业务短路径；非法 op；非法 value 类型；非法 `rulePhase`；未知字段；`enabled=false` 非法 Schema；未注册 evaluator；`SCRIPT` 默认拒绝。

## 待确认问题

以下问题不阻断本轮进入 Review / Dev，因为本分析已给出最小可验收默认口径；如 Owner 不接受默认口径，应另行改需求后重新分析。

1. `FORMULA` 的表达式语言最终采用 CEL、自研表达式还是只做声明式计算。默认口径：本任务只校验结构和禁止危险能力，不要求执行。
2. `LIMIT` ruleType 与 quota `USER_LIMIT / DAILY_LIMIT` 的长期边界是否要合并。默认口径：本任务只做规则配置 Schema，不改 quota 原子扣减。
3. `SCRIPT` 是否未来开放。默认口径：默认不开放 inline script；仅保留 `scriptRef` Schema，无注册表、审核和沙箱时阻断。
4. `occurredAt` 是否支持时间大小比较。默认口径：可作为标准字段做等值或集合判断；本任务不新增时间比较语义。

## Score

```text
94
```

评分：

| 维度 | 分数 | 理由 |
|---|---:|---|
| 需求清晰度 | 19/20 | 四类 ruleType、标准事件字段、扩展字段、op、value 类型和错误口径已明确。 |
| 范围可控 | 19/20 | 收口为 Schema 与后台校验，不扩大规则执行、脚本运行或草稿接口。 |
| 风险可控 | 19/20 | 已处理 Review 的字段兼容和 `enabled=false` 口径问题，并阻断未注册 evaluator。 |
| 验收标准明确 | 19/20 | 覆盖合法、非法、边界、文档和回归验证，含打回项专项验收。 |
| 架构一致性 | 18/20 | 保持标准事件、attributes、facts、state、plugin 输入边界，不连接外部业务库，不绕过 evaluator 注册模型。 |

## Decision

```text
PASS
```

结论：Review 打回项已逐条处理，需求细节达到可审核、可验收标准。Dev Agent 可基于本分析实现 Rule 配置 Schema 与后台校验，但不得借本任务开放未注册 `FORMULA / LIMIT / SCRIPT` 运行能力，也不得允许 `enabled=false` 非法规则通过当前 Admin create/update 保存。
