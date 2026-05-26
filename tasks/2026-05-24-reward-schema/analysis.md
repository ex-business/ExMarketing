# Analysis

## 需求 ID

2026-05-24-reward-schema

## Review 反馈处理

本轮已读取 `tasks/2026-05-24-reward-schema/review.md`。该文件仍是 Review Agent 模板态，未给出实际 P0/P1/P2 明细；但存在 `Open Questions` 占位和 `PASS / BLOCK` 占位，因此本轮按“潜在打回需补齐可审核需求”处理。

| Review 项 | 处理 |
|---|---|
| `P0/P1/P2` 为空占位。 | 未发现具体问题；本轮补齐完整 reward schema 需求，供后续 Review Agent 按字段、规则、验收标准审核。 |
| `Open Questions` 为“待填写”。 | 不等待 Owner 决策。对当前材料未唯一明确的组合，给出默认最小可验收口径：Schema 覆盖 7 种 `rewardType`，但上线只允许满足可执行发奖边界的组合；`MQ`、`MANUAL_EXPORT`、`MANUAL` 在 sender、导出或人工状态机未定义前拒绝上线。 |
| `Decision` 为 `PASS / BLOCK` 占位。 | 本轮以需求可审核、可验收为目标重写 analysis，并按评分规则给出明确 `PASS`。 |

## Analysis 自检修正

上一版 `analysis.md` 为模板态，`Score 0` 且 `Decision PASS / BLOCK`，等同于 Analysis 自身 BLOCK。本轮逐项修正：

| 上一版问题 | 修正 |
|---|---|
| `现状 / 目标理解 / 方案 / 风险点 / 验收方式` 均为“待填写”。 | 已补充背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险和验收标准。 |
| `Score 0`，低于 80。 | 基于任务、README、CLAUDE、docs 和设计文档补齐需求，重新评分为 90。 |
| 未回应 7 种奖励类型边界。 | 已分别定义 `BONUS / CASH / COUPON / POINT / GIFT / DRAW_CHANCE / MEMBERSHIP` 的最小配置字段、计算输出和上线限制。 |
| 未定义非法配置拒绝口径。 | 已明确 Admin 创建/更新和上线前校验必须拒绝非法枚举、未知字段、非法 Decimal、非法 rewardType/calculateType 组合、缺少 `targetRef`、不支持 grant mode/channel 等。 |

## 资料依据

- `tasks/2026-05-24-reward-schema/task.md`：要求完善 `marketing_reward`，覆盖 `reward_type`、`calculate_type`、`reward_config_json`、`grant_mode`、`grant_channel`、`flow_multiple`；7 种奖励类型边界清晰，非法配置拒绝上线。
- `README.md`、`docs/api.md`：当前活动创建已包含 reward 配置；奖励计算使用 Decimal；`AUTO + HTTP/WEBHOOK` 为当前可执行边界；`targetRef` 必须是 http(s) URL；`MQ`、`MANUAL_EXPORT`、`MANUAL` 当前拒绝。
- `docs/data-model.md`：`marketing_reward`、`marketing_grant_order`、`marketing_quota`、参与快照和 reward payload 约束；计算结果为 `0.0000` 不生成发奖单、outbox 或 quota 扣减。
- `docs/architecture.md`、`CLAUDE.md`：ExMarketing 不直接修改用户余额，只生成 grant_order 和 outbox；禁止使用 `float64`；禁止绕过 participation/grant_order 发奖。
- `22-独立活动营销中台设计.md`：奖励类型包含 `BONUS / CASH / COUPON / POINT / GIFT / DRAW_CHANCE / MEMBERSHIP`；计算方式包含 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`；上线前必须校验概率、库存、预算、JSON Schema。

## 背景目标

ExMarketing 是独立活动营销中台，奖励配置必须以通用 Schema 描述，不绑定钱包、优惠券、积分、礼品、抽奖或会员系统的内部模型。系统只负责判断奖励、创建参与记录、发奖单和 outbox 指令，真正发放由外部奖励系统完成并回调。

本需求目标：

- 明确 `marketing_reward` 的字段级配置契约。
- 覆盖 7 种 `rewardType` 的合法配置边界和不支持组合。
- 统一 `calculateType` 与 `rewardConfig` 的 JSON Schema。
- 明确 `grantMode`、`grantChannel`、`targetRef`、`flowMultiple` 的上线规则。
- 让非法奖励配置在 Admin 创建/更新或上线前校验阶段被拒绝，避免运行期默认通过、错误发奖或无法对账。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 运营 / 后台配置人员 | 奖励类型、计算方式和发奖方式有明确可配置范围；错误配置能定位字段和原因。 |
| 活动引擎 | 只消费已校验的 reward 配置，生成可解释的 reward preview、grant_order 和 outbox。 |
| Reward Calculator | 按 `calculateType` 输出金额、积分、次数、权益或奖品 payload；金额和数量不得使用 float64。 |
| Quota / Budget | 根据奖励结果预览和扣减预算、库存、用户次数；非法或零奖励不得扣减。 |
| 外部奖励系统 | 通过 HTTP/WEBHOOK 接收标准发奖指令，按 `commandId` / idempotency key 幂等发放并回调。 |
| 审批 / 上线前校验 | 复用本 Schema 阻断非法枚举、非法组合、概率错误、预算风险和不支持发奖方式。 |

## 输入输出

### 输入

后台活动配置中的 `rewards[]`。

| 字段 | 要求 |
|---|---|
| `rewardCode` | 必填，活动内唯一，用于库存 `scopeKey`、发奖幂等、快照和对账。 |
| `rewardType` | 必填，只能是 `BONUS`、`CASH`、`COUPON`、`POINT`、`GIFT`、`DRAW_CHANCE`、`MEMBERSHIP`。 |
| `calculateType` | 必填，只能是 `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。 |
| `rewardConfig` / `rewardConfigJson` | 必填 JSON object，按 `rewardType + calculateType` 联合校验；拒绝数组、字符串、数字、布尔、null。 |
| `grantMode` | 必填；当前可上线最小口径只允许 `AUTO`。 |
| `grantChannel` | 必填；当前可上线最小口径只允许 `HTTP`、`WEBHOOK`。 |
| `flowMultiple` | 非负 Decimal，最多 4 位小数；缺省为 `0`。顶层 `flowMultiple` 是准入源。 |
| `enabled` | 布尔，缺省 `true`；`false` 只影响运行时跳过，不放宽 Schema 校验。 |
| `version` | 正整数，缺省 `1`；用于快照、稳定随机 seed 和配置追溯。 |

### 输出

成功时：

- 奖励配置可保存、查询、模拟预览和进入活动运行。
- reward preview 至少包含 `rewardCode`、`rewardType`、`calculateType`、`amount/value/count/quantity`、`currency/unit`、`grantChannel`、`skipped`、`reasonCode`、`reason` 和计算解释字段。
- 生成 grant_order 时，`reward_payload_json` 必须保留 `rewardCode`、`rewardType`、`calculateType`、`currency/unit`、`flowMultiple`、`targetRef`、计算来源和原因字段。

失败时：

- Admin 创建/更新和上线前校验返回 400 类校验错误或上线阻断结果。
- 错误至少定位到 `rewards[index]`、字段名、错误码和原因。
- 非法 reward 不得部分保存为可运行配置；`enabled=false` 也不得绕过 Schema。

## 通用业务规则

- 所有 Decimal 字段必须用字符串或精确 Decimal 语义解析，禁止使用二进制浮点语义。
- 金额类输出统一保留 4 位小数；积分、抽奖次数、礼品数量、会员天数等数量类输出必须是非负整数。
- 计算结果为 0 或空奖时，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT，并在 preview / participation snapshot 中记录跳过原因。
- `rewardConfig.targetRef` 对 `AUTO + HTTP/WEBHOOK` 必填，且必须是 `http` 或 `https` URL；不能是空串、相对路径、私有协议或内联 secret。
- `flowMultiple` 顶层字段为准。若兼容旧请求在 `rewardConfig.flowMultiple` 中也传入，必须与顶层值一致；不一致时拒绝配置。
- `flowMultiple > 0` 默认只允许 `rewardType=BONUS`。`CASH / POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 的默认最小口径必须为 `0`，除非后续另立需求定义其流水语义。
- `PROBABILITY` 的概率或权重必须可验证，总权重必须大于 0；概率和若采用显式概率，必须等于 1 或 100%，不能超出。
- `LADDER` 的区间必须闭区间、可排序且不重叠；边界值必须可解析 Decimal 或整数，取决于输出类型。
- `RANDOM` 必须配置稳定随机所需范围和 `seedSalt`；同一 tenant、activity、rewardCode、idempotency key、version 下结果必须稳定。
- `POOL_SPLIT` 只表示事件即时分摊，不表示活动结束后整池清算；必须明确 `splitMode=COUNT|WEIGHT`、奖池金额或字段来源。

## Reward Type 边界

### BONUS

用途：红利、礼金、不可直接提现或有流水要求的奖励。

要求：

- 必须有金额输出和 `currency`。
- 允许 `flowMultiple >= 0`。
- 兼容计算方式：`FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
- `rewardConfig` 可包含 `expireAt` 或 `expireDays`，二者同时存在时必须语义一致或拒绝。

### CASH

用途：现金或可提现余额类奖励。

要求：

- 必须有金额输出和 `currency`。
- `flowMultiple` 必须为 `0`。
- 兼容计算方式：`FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
- 若需要不可提现、冻结、流水等语义，应使用 `BONUS`，不得用 `CASH` 隐式表达。

### POINT

用途：积分、经验值或平台点数。

要求：

- 输出字段为 `points` 或通用 `quantity/value`，必须是非负整数。
- `rewardConfig.unit` 或 `pointType` 必填，默认最小口径可使用 `POINT`。
- 不要求 `currency`；如传入 `currency` 必须拒绝，避免与金额类混淆。
- 兼容计算方式：`FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`；`POOL_SPLIT` 仅在分摊结果仍为整数且余数规则明确时允许，否则拒绝上线。

### COUPON

用途：优惠券、代金券、折扣券等外部券权益。

要求：

- `rewardConfig.couponTemplateId` 必填。
- `quantity` 必填或由计算结果得出，必须是正整数。
- `couponType` 可选，允许 `CASH_OFF`、`DISCOUNT`、`FREE_SPIN`、`CUSTOM`；不内置具体业务券模型。
- 面额券可配置 `faceValue` 和 `currency`；折扣券可配置 `discountRate`；二者不能混用为不可解释配置。
- 默认兼容 `FIXED`、`LADDER`、`PROBABILITY`；`RATIO / RANDOM / POOL_SPLIT` 若无法产生明确券模板和整数数量，必须拒绝上线。

### GIFT

用途：实物或数字礼品。

要求：

- `rewardConfig.giftSkuId` 或 `giftCode` 必填。
- `quantity` 必填或由计算结果得出，必须是正整数。
- `deliveryMode` 可选，允许 `EXTERNAL`、`MANUAL`、`DIGITAL`；当前可上线发奖仍必须通过 `AUTO + HTTP/WEBHOOK` 指令给外部系统处理。
- 预算估值如需参与 BUDGET，必须配置 `valuationAmount` 和 `currency`；未配置估值时只能参与 STOCK/USER_LIMIT，不能隐式扣金额预算。
- 默认兼容 `FIXED`、`PROBABILITY`；其他计算方式必须能输出明确 `giftSkuId + quantity`，否则拒绝。

### DRAW_CHANCE

用途：给用户增加抽奖次数、开箱次数或玩法机会。

要求：

- `rewardConfig.drawPoolCode`、`lotteryCode` 或等价外部玩法引用必填。
- `chanceCount` 必填或由计算结果得出，必须是正整数。
- 不允许 `currency` 和金额输出；如需抽中奖金，应由抽奖玩法后续产生新的奖励链路，不能在本 reward 中混合表达。
- 默认兼容 `FIXED`、`LADDER`、`PROBABILITY`；`RATIO` 仅在明确取整规则时允许，否则拒绝上线。

### MEMBERSHIP

用途：会员等级、VIP 天数、权益包等会员权益。

要求：

- `rewardConfig.membershipTier`、`membershipPlanId` 或 `benefitPackageId` 至少一个必填。
- `durationDays` 必填或由计算结果得出，必须是正整数；永久权益必须显式配置 `durationType=PERMANENT`。
- 不允许金额输出和 `currency`，除非仅作为预算估值字段 `valuationAmount + currency`。
- 默认兼容 `FIXED`、`LADDER`、`PROBABILITY`；其他计算方式若无法输出明确权益和期限，必须拒绝。

## Calculate Type Schema

### FIXED

最小配置：

```json
{
  "amount": "58.00",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

规则：

- 金额类使用 `amount`；积分/次数/数量类使用 `points`、`chanceCount`、`quantity` 或 `durationDays`。
- 输出值必须非负；上线口径下实际发奖值必须大于 0，0 值只允许作为跳过奖励的可解释配置。
- 金额类必须有 `currency`；非金额类不得误传 `currency`，除非是明确预算估值字段。

### RATIO

最小配置：

```json
{
  "baseField": "attributes.amount",
  "rate": "0.20",
  "maxAmount": "500.00",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

规则：

- `baseField` 必须是标准事件或受控扩展字段路径，例如 `attributes.amount`、`facts.<factCode>.<path>`、`state.*`。
- `rate` 必须为非负 Decimal。
- 金额类可配置 `maxAmount`；数量类若允许 `RATIO`，必须配置 `roundingMode`，允许 `FLOOR`、`CEIL`、`ROUND_HALF_UP`。
- `baseField` 缺失、不可解析 Decimal、`rate` 非法或封顶非法必须拒绝。

### LADDER

最小配置：

```json
{
  "field": "attributes.amount",
  "ladders": [
    {"min": "100.00", "max": "499.99", "amount": "18.00"},
    {"min": "500.00", "max": "999999.00", "amount": "58.00"}
  ],
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

规则：

- `field` 必填，字段路径必须符合规则引擎白名单。
- `ladders` 必须非空，区间闭合、边界可解析、`min <= max`、不重叠。
- 每个阶梯的输出字段必须与 `rewardType` 匹配，例如金额类 `amount`、积分 `points`、抽奖 `chanceCount`、会员 `durationDays`。

### RANDOM

规则：

- 必须配置 `minAmount/maxAmount/step` 或对应数量类的 `min/max/step`。
- `min <= max`，`step > 0`，步长必须能整除或明确覆盖范围。
- 必须配置 `seedSalt` 或使用平台默认稳定 seed；不得使用当前时间生成不可复现结果。
- 默认只适合金额、积分等数值型奖励；券、礼品、会员权益若需要随机选择，应使用 `PROBABILITY`。

### PROBABILITY

规则：

- `items` 必须非空。
- 每个 item 必须有正数 `weight` 或合法 `probability`，并包含与 `rewardType` 匹配的输出 payload。
- 允许空奖，但空奖 item 必须显式 `noReward=true` 并有原因；抽中空奖不生成 grant_order/outbox。
- 所有非空奖 item 必须通过对应 rewardType 的字段校验。

### POOL_SPLIT

规则：

- 必须配置 `poolAmount` 或 `poolField`，以及 `splitMode=COUNT|WEIGHT`。
- 金额类必须配置 `currency`。
- 分摊结果必须可解释，余数处理必须稳定。
- 只表示单次事件即时分摊；不提供活动结束批量清算、排行榜结算或跨事件累积分配。

## Grant Mode / Channel

当前可上线口径：

```text
grantMode    AUTO
grantChannel HTTP / WEBHOOK
```

规则：

- `AUTO + HTTP/WEBHOOK` 必须配置 `targetRef`，由 outbox 投递标准发奖指令。
- `MQ` 在当前 MQ outbox sender 未定义前拒绝上线，即使 Schema 字段形态合法也不能进入 ACTIVE 活动。
- `MANUAL_EXPORT` 在导出文件格式、状态机、审计和回传机制未定义前拒绝上线。
- `MANUAL` 在人工发奖流程、权限、审计和状态转换未定义前拒绝上线。
- 不支持的模式不得降级为 HTTP，也不得保存为会运行时静默跳过的 ACTIVE 配置。

## 边界条件和异常场景

- 未知 `rewardType`、`calculateType`、`grantMode`、`grantChannel`：拒绝。
- `rewardConfig` 不是 JSON object：拒绝。
- `rewardConfig` 出现当前 Schema 未允许字段：拒绝，避免隐藏业务模型。
- `rewardCode` 为空、重复或含不可追踪字符：拒绝。
- 金额、比例、数量、权重、概率不可解析或为负：拒绝。
- 金额类缺 `currency`：拒绝。
- 非金额类误传金额字段作为实际发奖金额：拒绝。
- `targetRef` 缺失、不是 http(s)、为空或带不可接受 secret：拒绝。
- `flowMultiple` 为负、不可解析或非 BONUS 大于 0：拒绝。
- `enabled=false` 但 Schema 非法：拒绝保存。
- `PROBABILITY` 权重总和为 0、概率超出范围或 item payload 非法：拒绝。
- `LADDER` 区间重叠、断言不可排序或输出字段与 rewardType 不匹配：拒绝。
- `RANDOM` 缺稳定 seed 口径或范围非法：拒绝。
- 模拟运行中 reward 计算失败：活动应给出不可发奖原因，不生成 preview 中的可执行 grant/outbox。

## 非目标范围

- 不实现外部钱包、积分、优惠券、礼品、抽奖或会员系统的真实发放逻辑。
- 不直接修改用户余额、会员等级、券库存或礼品库存。
- 不新增单独的 MQ sender、人工导出、人工发奖后台或人工审批状态机。
- 不定义具体业务的 coupon template、gift SKU、membership plan 数据表；这些只作为外部系统引用。
- 不新增活动结束后奖池清算、排行榜结算或跨事件批量分摊。
- 不改变标准事件 Schema、规则字段白名单、participation/grant_order/outbox 主流程。

## 风险

- 高风险：奖励 Schema 过宽会让非法业务字段或 secret 进入配置，导致错误发奖或泄露。
- 高风险：`rewardType + calculateType` 组合不清会让非金额权益被当成金额处理，影响预算和对账。
- 高风险：非 Decimal 计算会造成金额、比例、封顶和分摊误差。
- 中风险：设计文档列出 `MQ / MANUAL_EXPORT`，但当前 sender 和状态机未闭环；若允许上线会产生无法投递或无法回调的发奖单。
- 中风险：`flowMultiple` 同时可能出现在顶层和旧 `rewardConfig` 示例中；必须明确顶层为准并拒绝不一致。
- 中风险：COUPON/GIFT/DRAW_CHANCE/MEMBERSHIP 依赖外部系统引用，Schema 只能校验引用形态，不能证明外部模板真实存在。

## 验收标准

- `marketing_reward` 配置明确支持并校验 7 种 `rewardType`：`BONUS / CASH / COUPON / POINT / GIFT / DRAW_CHANCE / MEMBERSHIP`。
- `calculateType` 只允许 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`，非法枚举和非法组合被拒绝。
- 每种 `rewardType` 的必填字段、金额/非金额边界、数量整数边界、currency 规则和 `flowMultiple` 规则清晰可测。
- `AUTO + HTTP/WEBHOOK` 可上线，且必须有 http(s) `targetRef`；`MQ / MANUAL_EXPORT / MANUAL` 在当前阶段必须拒绝上线。
- `rewardConfig` 未知字段、缺必填字段、非法 Decimal、负数、非法概率、阶梯重叠、随机范围非法均返回可定位错误。
- `enabled=false` 不放宽 Schema 校验。
- 模拟运行能展示奖励预览、跳过原因、预算/库存影响和最终发奖指令预览；零奖励或空奖不生成可执行 grant/outbox preview。
- 实际运行仍遵守 participation -> quota -> grant_order -> outbox 链路，不直接发奖，不绕过幂等。
- 文档补充 reward schema 字段表、每类 reward 示例、非法示例和当前不支持的 grant mode/channel。
- 测试覆盖合法/非法 rewardType、calculateType、rewardConfig、flowMultiple、targetRef、概率、阶梯、随机和 grant mode/channel；最低通过 `go test ./...` 和 `go build ./...`。

## 待确认问题

- 是否需要在平台内校验 `couponTemplateId`、`giftSkuId`、`drawPoolCode`、`membershipPlanId` 的真实存在性？当前最小口径只校验非空引用和发奖指令，由外部奖励系统最终校验。
- 是否允许 `CASH` 配置流水倍数？当前最小口径拒绝，避免把可提现现金和红利混用。
- `POINT` 的小数积分是否存在业务需求？当前最小口径只允许整数积分。

以上问题不阻断本轮进入研发；默认口径已写入业务规则和验收标准。

## Score

```text
90
```

评分：

```text
需求清晰度        18/20
范围可控          18/20
风险可控          18/20
验收标准明确      18/20
架构一致性        18/20
```

扣分原因：外部券、礼品、抽奖、会员模板真实性无法仅靠当前系统文档闭环，只能按外部引用校验处理；`MQ / MANUAL_EXPORT / MANUAL` 需要后续独立需求。

## Decision

```text
PASS
```

需求：2026-05-24-reward-schema
