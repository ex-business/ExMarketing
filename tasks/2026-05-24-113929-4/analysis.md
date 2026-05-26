# Analysis

## 需求 ID

2026-05-24-113929-4

## 标题

随机 / 概率 / 奖池奖励计算

## Review 反馈处理

本轮已读取 `tasks/2026-05-24-113929-4/review.md`。该文件仍为 Review 模板，未填写实际 P0 / P1 / P2、Open Questions 或具体打回原因。

处理方式：

- P0：无实际反馈项。
- P1：无实际反馈项。
- P2：无实际反馈项。
- Open Questions：模板为“待填写”，无可逐条处理的问题。
- Decision：模板仍为 `PASS / BLOCK`，未形成实际打回结论。

因此本轮不按 Review 打回处理具体缺陷，但仍补充本需求的完整业务口径，供后续 Review Agent 审核。

## Analysis 自检修正

本轮已读取上一版 `tasks/2026-05-24-113929-4/analysis.md`。上一版为 Analysis 模板，存在以下自检失败项：

- `现状`、`目标理解`、`方案`、`改动范围`、`风险点`、`验收方式`均为“待填写”。本轮已改为完整需求说明、背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准。
- `Score = 0`，不满足进入研发条件。原因是上一版未完成需求分析。本轮按评分规则重新评分。
- `Decision = PASS / BLOCK` 未给出结论。原因是上一版未形成可审核需求。本轮给出明确 Decision。
- 待确认问题缺失。上一版没有列出关键不确定项。本轮对概率总和、奖池拆分、随机可审计等可能歧义给出默认最小可验收口径，不等待 Owner 决策。

本轮分析基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/api.md`、`docs/roadmap.md`、`docs/workflow.md` 和来源设计文档 `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`。本轮未读取代码、不写代码、不提交、不设计具体代码改法。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动匹配与规则判断、计算奖励、生成 participation、grant_order 和 outbox，不直接修改用户余额，不依赖外部业务数据库。

本需求来自“22-独立活动营销中台设计”，目标是补齐或固化三类高风险玩法型奖励计算边界：

- `RANDOM`：按配置区间生成稳定、可复现、可审计的随机金额。
- `PROBABILITY`：按配置概率选择奖励项，概率非法时不得成为可执行活动配置。
- `POOL_SPLIT`：按事件即时输入进行奖池拆分，支持按人数均分或按权重分摊。

本需求重点不是新增发奖通道，也不是做活动结束后统一奖池结算；重点是让随机、概率和奖池拆分在统一奖励计算链路中具备明确配置校验、Decimal 精度、稳定幂等、审计快照和验收标准。

## 参与方

- 活动配置方：通过 Admin 配置奖励类型、计算方式、概率、随机区间、奖池拆分方式和发奖目标。
- 事件接入方：通过标准事件提供幂等键、主体、业务对象和 `attributes.*` 业务字段。
- ExMarketing 引擎：在活动命中、规则通过后执行奖励计算，并把计算结果交给 participation、quota、grant_order、outbox 链路。
- 外部发奖系统：接收 ExMarketing 的标准发奖指令并执行真实发奖。
- 审计 / 对账人员：通过 participation snapshot、模拟运行结果、audit 或 reconciliation 查询随机与概率计算依据。
- Review / Dev / Test Agent：按本文件约束审核、实现和验证，不得用业务专用实现绕过统一链路。

## 输入

### 公共输入

- 活动奖励配置：
  - `rewardType`：本需求默认覆盖金额类 `BONUS` / `CASH`。
  - `calculateType`：`RANDOM` / `PROBABILITY` / `POOL_SPLIT`。
  - `rewardConfig`：对应计算方式的参数。
  - `grantMode`：当前可执行口径为 `AUTO`。
  - `grantChannel`：当前可执行口径为 `HTTP` / `WEBHOOK`。
  - `targetRef`：`AUTO + HTTP/WEBHOOK` 必填，必须是 `http` 或 `https` URL。
  - `currency`：金额类 `BONUS` / `CASH` 必填。
- 标准事件：
  - 顶层字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`。
  - 业务字段：只能通过 `attributes.*` 承载。
  - 幂等键：`tenantId:sourceSystem:eventType:bizType:bizId`，是随机和概率稳定结果的重要输入。
- 运行上下文：
  - 可读取标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。

### RANDOM 输入

最小配置示例：

```json
{
  "minAmount": "1.00",
  "maxAmount": "88.00",
  "step": "0.01",
  "seedSalt": "optional-activity-salt",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `minAmount`、`maxAmount`、`step` 必填，必须是可解析 Decimal。
- `0 <= minAmount <= maxAmount`。
- `step > 0`。
- 可选结果集合为 `minAmount + n * step`，且不得超过 `maxAmount`。
- `seedSalt` 可选，用于配置级扰动；不得作为安全密钥暴露给外部发奖系统。

### PROBABILITY 输入

最小配置示例：

```json
{
  "items": [
    {"itemCode": "SMALL_BONUS", "weight": "80.0000", "amount": "2.00"},
    {"itemCode": "BIG_BONUS", "weight": "19.0000", "amount": "18.00"},
    {"itemCode": "EMPTY", "weight": "1.0000", "amount": "0.00"}
  ],
  "seedSalt": "optional-activity-salt",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `items` 必填，必须是非空数组。
- 每个 item 必须有稳定唯一的 `itemCode`。
- `weight` 必填，必须是可解析 Decimal，且 `weight >= 0`。
- 本需求采用百分比概率口径：所有 enabled / 生效 item 的 `weight` 总和必须等于 `100.0000`。
- 至少一个 item 的 `weight > 0`。
- `amount` 必填，必须是可解析 Decimal，且 `amount >= 0`。
- `amount = 0.0000` 表示空奖或无实际金额奖励；可以被抽中，但不会生成实际 grant_order/outbox 或 quota 扣减。
- `seedSalt` 可选，用于配置级扰动；不得替代幂等键。

采用“权重总和必须等于 100”的依据是：`task.md` 明确要求“概率总和校验 / 概率非法不能上线”，既有测试计划材料也以“概率百分比总和不等于 100”为阻断口径。若未来要支持相对权重，应另立需求并同步更新配置语义、文档和测试。

### POOL_SPLIT 输入

`POOL_SPLIT` 是事件即时分摊模型，不表示活动结束后统一奖池结算。

按人数均分示例：

```json
{
  "poolAmount": "1000.00",
  "splitBy": "COUNT",
  "countField": "attributes.poolParticipantCount",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

按权重分摊示例：

```json
{
  "poolAmount": "1000.00",
  "splitBy": "WEIGHT",
  "weightField": "attributes.userPoolWeight",
  "totalWeightField": "attributes.poolTotalWeight",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `poolAmount` 必填，必须是可解析 Decimal，且 `poolAmount >= 0`。
- `splitBy` 必填；兼容别名 `splitMode`，但规范输出和文档推荐使用 `splitBy`。
- `splitBy` 只允许 `COUNT` / `WEIGHT`。
- `COUNT`：
  - `countField` 必填，指向允许字段路径中的参与人数。
  - 运行时 `countField` 必须可解析 Decimal 或整数，且 `count > 0`。
  - 计算公式：`rewardAmount = poolAmount / count`。
- `WEIGHT`：
  - `weightField` 必填，表示当前主体权重。
  - `totalWeightField` 必填，表示本次奖池总权重。
  - 两个字段运行时都必须可解析 Decimal。
  - `weight >= 0`，`totalWeight > 0`，且 `weight <= totalWeight`。
  - 计算公式：`rewardAmount = poolAmount * weight / totalWeight`。

## 输出

奖励计算成功后输出标准奖励结果，供 participation snapshot、grant_order、outbox、模拟运行和对账使用：

- `rewardCode`：沿用奖励配置。
- `rewardType`：沿用配置，如 `BONUS` / `CASH`。
- `calculateType`：`RANDOM` / `PROBABILITY` / `POOL_SPLIT`。
- `amount`：Decimal 金额，统一保留 4 位小数。
- `currency`：金额类奖励必填并透传。
- `flowMultiple`：按既有奖励配置口径透传；本需求不改变流水倍数规则。
- `calculationContext` 或等价快照摘要：
  - `configVersion`。
  - `idempotencyKey` 或其可审计引用。
  - `seedSource` 摘要或 hash，不要求暴露原始 `seedSalt`。
  - `RANDOM`：`minAmount`、`maxAmount`、`step`、命中序号或随机归一值、最终金额。
  - `PROBABILITY`：参与抽取的 item 列表摘要、权重总和、抽取值、命中 `itemCode`、最终金额。
  - `POOL_SPLIT`：`poolAmount`、`splitBy`、运行时 count / weight / totalWeight、最终金额。

金额结果为 `0.0000` 且无其他实际权益时，按既有文档口径不生成 grant_order、outbox，也不扣减 quota；但 participation snapshot 或模拟预览中仍应保留计算摘要，说明为什么没有实际发奖。

## 业务规则

### 通用规则

- 金额、概率、权重、奖池金额和拆分结果必须使用 Decimal 语义，禁止使用 `float64` 做资金或概率计算。
- 输出金额统一保留 4 位小数。
- 奖励只在活动命中且规则通过后计算。
- 计算器只负责奖励计算，不负责绕过 rule、quota、participation、grant_order、outbox 或 callback。
- 字段路径必须遵守 ExMarketing 标准边界，不允许直接读取 `orderAmount`、`deposit.amount`、`user.vipLevel` 等非标准顶层业务字段。
- 对可执行发奖配置，`rewardConfig.targetRef` 必须是 `http` 或 `https` URL。
- 金额类 `BONUS` / `CASH` 必须配置 `currency`。
- 非金额类奖励的随机、概率和奖池能力不纳入本期最小可验收范围；非金额类当前最小口径继续只支持既有 `FIXED`。

### RANDOM 规则

- 随机结果必须由稳定 seed 得出，默认 seed 输入至少包含：
  - `tenantId`
  - `activityId` 或 `activityCode`
  - `rewardCode`
  - `idempotencyKey`
  - 活动配置 `version`
  - 可选 `seedSalt`
- 同一租户、同一活动版本、同一奖励、同一幂等事件重复处理时，必须得到同一个随机金额。
- 活动版本变化后，允许因配置版本变化得到不同随机结果；旧 participation snapshot 必须保留旧版本计算上下文。
- `RANDOM` 只能在配置的离散候选集合内取值，不得低于 `minAmount` 或高于 `maxAmount`。
- 当 `minAmount = maxAmount` 时，结果固定为该金额，但仍应记录为 `RANDOM` 计算摘要。

### PROBABILITY 规则

- 概率 item 的 `weight` 总和必须等于 `100.0000`，否则配置非法。
- `weight = 0` 的 item 可保存为占位项，但不会被抽中。
- 概率抽取必须由稳定 seed 得出；重复幂等事件必须命中同一个 item。
- 抽中 `amount = 0.0000` 的空奖时，不生成实际 grant_order/outbox，不扣 quota，但 snapshot / 模拟结果必须记录命中 `itemCode` 和空奖原因。
- 多个 item 的 `itemCode` 不得重复，避免审计和对账无法定位。
- item 顺序不得影响概率总和校验；实际抽取区间可以按配置顺序形成，但必须在 snapshot 中可解释。

### POOL_SPLIT 规则

- `POOL_SPLIT` 只按本次事件携带或上下文可读取的即时数据计算当前主体奖励。
- `COUNT` 均分：`poolAmount / count`。
- `WEIGHT` 分摊：`poolAmount * weight / totalWeight`。
- `poolAmount = 0` 合法，结果为 `0.0000`，不生成实际发奖。
- `COUNT` 的 `count <= 0`、`WEIGHT` 的 `totalWeight <= 0`、`weight < 0` 或 `weight > totalWeight` 均为运行时计算失败。
- 本需求不保证同一奖池下所有事件累计金额恰好等于 `poolAmount`；该能力属于活动结束后统一结算或奖池状态管理，非本期范围。

## 上线和配置校验

`task.md` 明确要求“概率非法不能上线”。本轮默认最小可验收口径如下：

- 对 Admin 创建或更新活动时，如果系统当前没有独立“上线”动作，则创建 / 更新可执行活动配置时必须执行相同校验，非法配置返回 `400 Bad Request` 或等价结构化错误。
- 如果存在 DRAFT 保存能力，DRAFT 是否允许暂存非法概率不作为本期强制目标；但任何进入 `SCHEDULED` / `ACTIVE` / 可执行状态的配置必须阻断非法概率。
- `PROBABILITY.items[].weight` 总和不等于 `100.0000` 必须阻断。
- `RANDOM` 的区间、步长非法必须阻断。
- `POOL_SPLIT` 的 `splitBy`、必填字段、`poolAmount` 非法必须阻断。
- 配置错误必须可解释，不能只返回通用 `500`。

## 边界条件

- Decimal 输入允许字符串或 JSON number；金额、概率、权重推荐字符串，以避免 JSON number 精度歧义。
- 输出金额固定 4 位小数，不因输入小数位不同而改变输出格式。
- 负数金额、负数概率、负数 step、负数奖池金额默认不允许。
- `RANDOM.step` 大于区间长度时，只有 `minAmount` 一个候选值是可接受的；但不得生成超过 `maxAmount` 的结果。
- `PROBABILITY` 允许空奖 item，但空奖必须显式配置为 `amount = 0.0000` 或等价 0 值，不允许缺省金额被隐式当作空奖。
- `POOL_SPLIT.COUNT` 的参与人数来自事件或上下文字段，本需求不负责维护活动全局参与人数。
- `POOL_SPLIT.WEIGHT` 的总权重来自事件或上下文字段，本需求不负责聚合全活动总权重。
- 本需求不改变活动状态、互斥组、quota、幂等、outbox retry、DLQ、callback 等既有业务语义。

## 异常场景

- 未知 `calculateType`：拒绝配置或返回明确错误，不得回退到固定金额。
- `RANDOM` 缺少 `minAmount` / `maxAmount` / `step`：拒绝配置。
- `RANDOM` 的 `minAmount > maxAmount`、`step <= 0` 或 Decimal 非法：拒绝配置。
- `PROBABILITY.items` 为空：拒绝配置。
- `PROBABILITY` item 缺少 `itemCode`、`weight` 或 `amount`：拒绝配置。
- `PROBABILITY` item code 重复：拒绝配置。
- `PROBABILITY` 权重总和不等于 `100.0000`：拒绝配置，不得上线。
- `POOL_SPLIT` 缺少 `poolAmount` 或 `splitBy`：拒绝配置。
- `POOL_SPLIT.splitBy` 不是 `COUNT` / `WEIGHT`：拒绝配置。
- `POOL_SPLIT.COUNT` 缺少 `countField`，或运行时 count 缺失 / 不可解析 / 小于等于 0：配置拒绝或本次计算失败。
- `POOL_SPLIT.WEIGHT` 缺少 `weightField` / `totalWeightField`，或运行时字段缺失 / 不可解析 / 越界：配置拒绝或本次计算失败。
- `currency` 缺失：金额类 `BONUS` / `CASH` 拒绝配置。
- `targetRef` 缺失或 URL 非法：`AUTO + HTTP/WEBHOOK` 拒绝配置。
- 运行时计算失败时，不生成 grant_order/outbox，不扣 quota；模拟运行或处理结果必须能看到明确失败原因。

## 非目标范围

本任务不包含：

- 新增外部业务数据库连接或查询。
- 新增业务专用 controller、业务专用发奖逻辑或绕过标准事件链路的能力。
- 直接修改钱包余额或直接完成真实发奖。
- 新增 MQ 发奖 sender、人工导出、人工发奖状态机。
- 实现活动结束后统一奖池结算、排行榜结算、跨事件累计分摊或奖池余额状态表。
- 维护红包雨场次、抽奖次数、奖品池库存等玩法插件真实状态。
- 扩展非金额类奖励的 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 计算；非金额类继续按既有最小口径处理。
- 改造 quota、budget、inventory、participation、grant_order、outbox、callback 状态机。
- 改造 Admin UI 交互；如现有 API 和测试可覆盖，本任务无需前端页面变化。
- 改变 `FIXED` / `RATIO` / `LADDER` 既有语义。

## 风险

- 资金精度风险：随机金额、奖池分摊和概率权重如果使用浮点会产生误差，必须按 Decimal 验收。
- 概率配置风险：概率总和不等于 100 会导致抽奖概率不可解释，必须阻断上线。
- 审计风险：随机和概率如果不记录 seed 摘要、命中 item、抽取值或版本，事后无法解释用户为何中奖或未中奖。
- 幂等风险：随机 seed 如果不包含幂等键或配置版本，重复事件可能重复随机出不同结果，破坏发奖安全。
- 兼容风险：`POOL_SPLIT` 容易被误解为活动结束后统一结算；本轮明确只做事件即时分摊。
- 配置边界风险：允许非标准顶层业务字段会破坏 ExMarketing 与外部业务系统解耦。
- 空奖语义风险：`amount=0` 必须可解释为无实际发奖，不能生成金额为 0 的无意义 grant_order/outbox。

## 验收标准

### 配置和上线校验

- `PROBABILITY.items[].weight` 总和为 `100.0000` 的配置可以进入可执行状态。
- `PROBABILITY.items[].weight` 总和小于或大于 `100.0000` 时，创建 / 更新可执行配置或上线前校验必须失败，并返回明确概率非法原因。
- `PROBABILITY` item 缺少 `itemCode`、`weight`、`amount` 或 item code 重复时必须失败。
- `RANDOM minAmount > maxAmount`、`step <= 0`、Decimal 非法时必须失败。
- `POOL_SPLIT` 缺少 `poolAmount`、`splitBy` 或使用非法 `splitBy` 时必须失败。
- 金额类奖励缺少 `currency` 或可执行发奖缺少合法 `targetRef` 时必须失败。

### RANDOM 计算

- 相同 tenant、activity、rewardCode、idempotencyKey、version 和 seedSalt 下，重复计算得到相同金额。
- 不同 idempotencyKey 在同一配置下允许得到不同金额。
- 结果必须落在 `[minAmount, maxAmount]` 范围内，且符合 `step` 离散候选集合。
- `minAmount = maxAmount` 时输出该固定金额，并记录 RANDOM 计算摘要。
- participation snapshot、模拟结果或审计上下文中必须能看到随机计算摘要，至少包含配置版本、seed 摘要、范围、step 和最终金额。

### PROBABILITY 计算

- 权重总和合法时，按稳定 seed 命中一个 item；重复幂等事件必须命中同一个 item。
- 抽中金额 item 时，输出该 item 的金额，保留 4 位小数。
- 抽中 `amount = 0.0000` 的空奖时，不生成实际 grant_order/outbox，不扣 quota，但 snapshot / 模拟结果记录命中 `itemCode` 和空奖金额。
- snapshot、模拟结果或审计上下文中必须能看到概率计算摘要，至少包含配置版本、权重总和、抽取值或区间摘要、命中 `itemCode` 和最终金额。

### POOL_SPLIT 计算

- `COUNT`：`poolAmount=1000.00`、`count=4` 时输出 `250.0000`。
- `COUNT`：count 缺失、不可解析或 `<= 0` 时，本次计算失败，不生成 grant_order/outbox，不扣 quota。
- `WEIGHT`：`poolAmount=1000.00`、`weight=2`、`totalWeight=10` 时输出 `200.0000`。
- `WEIGHT`：`weight < 0`、`totalWeight <= 0`、`weight > totalWeight` 或字段不可解析时，本次计算失败。
- snapshot、模拟结果或审计上下文中必须能看到奖池拆分摘要，至少包含 `poolAmount`、`splitBy`、运行时 count / weight / totalWeight 和最终金额。

### 回归验收

- `go test ./...` 必须通过。
- `go build ./...` 必须通过。
- 标准事件幂等重放不会重复随机、不会重复参与、不会重复发奖、不会重复扣减 quota。
- 奖励计算结果继续进入现有 participation、quota、grant_order、outbox 链路，不新增直接发奖旁路。
- 金额输出保持 4 位小数。
- `FIXED` / `RATIO` / `LADDER` 既有行为不得因本任务被删除、禁用或改变语义。

## 待确认问题

以下问题不阻断本任务，已按最小可验收口径处理：

- 概率权重是否为百分比还是相对权重：本轮默认 `weight` 是百分比 Decimal，总和必须等于 `100.0000`。依据是 `task.md` 的“概率总和校验 / 概率非法不能上线”和既有测试计划中的“概率百分比总和不等于 100”阻断口径。
- `POOL_SPLIT` 是否需要跨事件累计保证整池金额完全分完：本轮默认不需要；只做事件即时分摊。跨事件结算、尾差归集、排行榜或活动结束统一结算另立需求。
- 随机审计是否必须写 `marketing_audit`：本轮默认满足“记录到 snapshot 或审计上下文”即可；最小验收要求 participation snapshot 或模拟 / 对账可见。若未来要求每次随机单独写 audit action，应另立审计增强需求。

## Score

```text
90
```

评分说明：

- 需求清晰度：18 / 20，三类计算的输入输出、默认口径和失败语义已明确；概率口径按百分比收敛。
- 范围可控：19 / 20，限定在金额类 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 奖励计算和校验，不扩展玩法状态或结算系统。
- 风险可控：18 / 20，主要风险为 Decimal、幂等随机、概率非法和审计可解释性，已有约束与验收。
- 验收标准明确：18 / 20，覆盖配置阻断、三类计算、快照审计和回归命令。
- 架构一致性：17 / 20，遵守标准事件、统一奖励计算、participation、quota、grant_order、outbox 边界；`POOL_SPLIT` 即时分摊与未来玩法结算仍需后续需求承接。

## Decision

```text
PASS
```

需求细节已达到可审核、可验收标准，可以进入 Review / Dev 阶段。
