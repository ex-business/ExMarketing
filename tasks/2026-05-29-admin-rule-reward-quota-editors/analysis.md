# Analysis

## 需求 ID

2026-05-29-admin-rule-reward-quota-editors

## Review 反馈处理

本轮已读取 `tasks/2026-05-29-admin-rule-reward-quota-editors/review.md`，并逐条处理 Review Agent 的打回意见：

| Review 项 | 处理 |
|---|---|
| P0 为空 | 无需处理；本轮无 P0 阻塞项。 |
| P1：缺少 `rewardType × calculateType` 可执行组合矩阵，可能误导实现允许所有奖励类型使用全部计算方式。 | 已在“奖励编辑器”和“奖励配置”中补充明确矩阵：金额类 `BONUS / CASH` 可使用 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`；非金额类 `POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 当前最小可上线口径只允许 `FIXED`。普通表单、JSON 模式、保存前校验、上线前校验和模拟前校验都必须拒绝非金额奖励搭配非 `FIXED` 的组合，并展示后端同口径错误。该修正关闭“配置结果必须能被现有引擎直接执行”的阻塞风险。 |
| P2：验收标准未显式要求“非金额奖励 + 非 FIXED calculateType 必须拒绝”。 | 已在验收标准、异常场景和奖励配置规则中补充该非法配置样例，要求覆盖 `POINT + RATIO`、`COUPON + LADDER`、`GIFT + RANDOM`、`DRAW_CHANCE + PROBABILITY`、`MEMBERSHIP + POOL_SPLIT` 至少一组或等价参数化用例。 |
| Open Questions：无阻塞性问题，需先修正 P1。 | 已按权威文档 `docs/data-model.md:209` 和 `docs/api.md:290` 的默认最小可验收口径修正，不等待 Owner 决策。 |
| Score 76 / Decision BLOCK | 阻塞原因已补齐，重新评分为 95，Decision 改为 PASS。 |

## Analysis 自检修正

上一轮 `analysis.md` 自身为 `Decision PASS`、`Score 91`，但 Review 指出该分析仍存在高风险缺口；本轮按 Analysis 自检返工处理：

| 上一轮问题 | 本轮修正 |
|---|---|
| 错误假设：`FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT` 的表述过宽，容易被理解为所有 `rewardType` 均可使用全部计算方式。 | 已修正为分类型兼容矩阵。金额类 `BONUS / CASH` 支持全部 6 种计算方式；非金额类 `POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 仅支持 `FIXED`。 |
| 扣分项遗漏：未把现有后端会对非金额奖励非 `FIXED` 返回 `400 Bad Request` 写入需求约束。 | 已补充为保存、上线、模拟前的明确拒绝规则，并要求错误定位到 `rewards[n].calculateType` 或等价 JSON path。 |
| 验收缺口：非法配置样例未覆盖非金额奖励搭配非 `FIXED`。 | 已新增验收项，要求测试至少覆盖 `POINT + RATIO` 等非法组合。 |
| 上一轮 Score 91 过高，未反映核心兼容矩阵缺失。 | 本轮补齐阻塞缺口后重评为 `95`；保留少量扣分给外部权益模板真实性、FORMULA 专用编辑体验和 quota 运行态展示等非阻塞后续深化项。 |

## 资料依据

- `task.md`：要求提供规则编辑器、奖励编辑器、quota 编辑器，表单由 Schema 或元数据驱动，支持普通表单和高级 JSON 模式，配置结果能被现有引擎直接执行。
- `README.md` / `docs/api.md`：活动创建、quota 创建、模拟运行、审批和标准事件 API 约束。
- `docs/data-model.md`：`marketing_rule`、`marketing_reward`、`marketing_quota` 字段、枚举、运行阶段和 Decimal / quota 规则。
- `docs/business-activity-templates.md`：首存、签到、返水等普通配置化活动模板必须只使用标准事件、attributes、facts、规则、奖励、quota 和 outbox。
- 已有任务分析：`rule-schema`、`reward-schema`、`quota`、`admin-activity-config-pages` 的默认口径作为本需求约束来源，不引入相冲突的新业务语义。

## 背景和目标

活动配置的核心是规则、奖励和额度限制。当前能力主要在 API 和配置模板层，运营必须手写 JSON 或依赖开发协助，容易出现字段路径错误、奖励组合非法、概率和金额边界错误、quota 配错以及上线前才发现不可执行配置。

本需求目标是为管理后台补齐三个通用编辑器：

- 规则编辑器：编辑 `MATCH / QUALIFY / REWARD / RISK / LIMIT` 阶段的规则。
- 奖励编辑器：编辑 `BONUS / CASH / COUPON / POINT / GIFT / DRAW_CHANCE / MEMBERSHIP` 等奖励，并按可执行兼容矩阵限制计算方式；不能把全部计算方式开放给全部奖励类型。
- Quota 编辑器：编辑 `BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT` 四类额度、库存和次数限制。

编辑器必须同时支持普通表单和高级 JSON 模式，保存或上线前能明确提示非法字段、类型、概率、金额、quota 边界。最终生成的 JSON 必须与现有活动引擎、模拟运行和上线前校验兼容。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 运营配置人员 | 通过表单安全配置规则、奖励和 quota；错误能定位到具体字段或 JSON 路径。 |
| 运营负责人 / 审批人 | 查看最终 JSON 和人类可读摘要，判断配置是否可上线。 |
| ExMarketing 后端 | 继续作为保存、校验、模拟、审批和上线前阻断的权威。 |
| 活动引擎 | 只消费合法配置，运行结果可解释，不能因编辑器生成未知字段而默认通过。 |
| Reward / Quota / Fact Provider | 按既有 Schema、Decimal、字段路径和 quota 规则执行。 |
| Test / Review Agent | 能基于明确输入输出、异常和验收标准设计测试。 |

## 范围

### 规则编辑器

必须覆盖字段：

- `ruleCode`
- `rulePhase`
- `ruleType`
- `ruleConfig`
- `fieldPath` 或 `conditions[].field`
- `op`
- `value`
- `sort`
- `enabled`
- `reasonCode`

默认最小可验收口径：

- 普通表单优先支持 `CONDITION` 规则。
- `FORMULA` 可通过高级 JSON 或已有安全 Schema 编辑，不要求做可视化公式编排器。
- `LIMIT`、`SCRIPT` 若后端当前只保留 Schema 边界但未注册可运行 evaluator，编辑器必须按后端权威结果阻断保存或上线，不能让用户以为可执行。

### 奖励编辑器

必须覆盖字段：

- `rewardCode`
- `rewardType`
- `calculateType`
- `grantMode`
- `grantChannel`
- `flowMultiple`
- `rewardConfig`
- `enabled`
- `version`

必须支持合法配置的保存和非法配置拒绝。可执行组合矩阵如下：

| rewardType | 类型分类 | 允许 calculateType | 禁止 calculateType | 默认最小可验收口径 |
|---|---|---|---|---|
| `BONUS` | 金额类 | `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | 无，仍需满足各计算方式自身字段约束 | 可配置现金等价奖励，支持 `currency` 和 `flowMultiple > 0`。 |
| `CASH` | 金额类 | `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `flowMultiple > 0` | 可配置现金奖励，必须有 `currency`，不允许流水倍数。 |
| `POINT` | 非金额类 | `FIXED` | `RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `points` 正整数，`unit` 或 `pointType` 必填；不允许 `currency`。 |
| `COUPON` | 非金额类 | `FIXED` | `RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `couponTemplateId` 和 `quantity` 正整数必填；只有配置 `faceValue` 时才需要 `currency`。 |
| `GIFT` | 非金额类 | `FIXED` | `RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `giftSkuId` 或 `giftCode` 必填，`quantity` 正整数必填；只有配置 `valuationAmount` 时才需要 `currency`。 |
| `DRAW_CHANCE` | 非金额类 | `FIXED` | `RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `drawPoolCode` 或 `lotteryCode` 必填，`chanceCount` 正整数必填；不允许 `currency`。 |
| `MEMBERSHIP` | 非金额类 | `FIXED` | `RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` | `membershipTier`、`membershipPlanId` 或 `benefitPackageId` 至少一个必填，并配置 `durationDays` 正整数或 `durationType=PERMANENT`；只有配置 `valuationAmount` 时才需要 `currency`。 |

UI Schema、普通表单选项、高级 JSON 校验、保存前校验、上线前校验和模拟前校验都必须使用同一矩阵。若用户在 JSON 模式输入非金额奖励搭配非 `FIXED`，必须阻止保存或展示后端 `400 Bad Request`，错误路径应定位到 `rewards[n].calculateType`，并提示该 `rewardType` 当前只支持 `FIXED`。

### Quota 编辑器

必须覆盖字段：

- `quotaType`
- `scope`
- `scopeKey`
- `totalValue`

必须表达：

- `BUDGET + ACTIVITY + TOTAL`
- `STOCK + ACTIVITY + rewardCode`
- `USER_LIMIT + USER + subject.id`
- `DAILY_LIMIT + USER + subject.id:activityLocalDate`

默认口径：缺少 quota 行表示无限制；编辑器需要在预览中明确展示“不限制”，不能误展示为 0。

### JSON 高级模式

- 每个编辑器都必须提供高级 JSON 模式，展示当前编辑对象的真实 JSON。
- 支持格式化。
- JSON 语法错误不能保存。
- 后端或前端校验错误必须展示 JSON path，例如 `rules[0].ruleConfig.conditions[1].value`、`rewards[2].rewardConfig.items[0].weight`、`quotas[1].totalValue`。
- 普通表单和 JSON 模式切换时必须保持同一份配置数据；切换不能丢字段、改类型或悄悄清空未知但合法字段。

### 配置预览

必须展示：

- 最终活动片段 JSON：`rules[]`、`rewards[]`、quota 请求或 quota 摘要。
- 人类可读摘要：阶段、条件、奖励金额/权益、发奖方式、预算/库存/次数限制。
- 模拟运行入口：保存后能进入 `POST /admin/v1/simulate` 对同一活动配置做预览。

## 输入输出

### 规则输入输出

输入示例：

```json
{
  "ruleCode": "FIRST_DEPOSIT_AMOUNT",
  "rulePhase": "QUALIFY",
  "ruleType": "CONDITION",
  "ruleConfig": {
    "logic": "AND",
    "conditions": [
      {"field": "attributes.isFirstDeposit", "op": "=", "value": true},
      {"field": "attributes.amount", "op": ">=", "value": "100.00"}
    ]
  },
  "sort": 1,
  "enabled": true,
  "reasonCode": "FIRST_DEPOSIT_AMOUNT_NOT_MET"
}
```

输出要求：

- 保存时进入活动配置的 `rules[]`，字段名和类型与后端活动 API 兼容。
- 摘要展示为：`QUALIFY: attributes.isFirstDeposit = true AND attributes.amount >= 100.00`。
- 禁用规则仍必须合法保存；运行摘要中应显示 `SKIPPED` 或禁用状态，不影响判断。

### 奖励输入输出

输入示例：

```json
{
  "rewardCode": "FIRST_DEPOSIT_RATIO",
  "rewardType": "BONUS",
  "calculateType": "RATIO",
  "rewardConfig": {
    "baseField": "attributes.amount",
    "rate": "0.20",
    "maxAmount": "500.00",
    "currency": "CNY",
    "targetRef": "https://wallet.example.com/reward"
  },
  "flowMultiple": "3",
  "grantMode": "AUTO",
  "grantChannel": "HTTP",
  "enabled": true,
  "version": 1
}
```

输出要求：

- 保存时进入活动配置的 `rewards[]`。
- 摘要展示为：`BONUS RATIO: attributes.amount * 0.20, max 500.00 CNY, flow 3x, AUTO/HTTP`。
- preview 必须能展示金额/权益、currency/unit、跳过原因、targetRef 和 quota 影响。

### Quota 输入输出

输入示例：

```json
[
  {"quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "10000.00"},
  {"quotaType": "STOCK", "scope": "ACTIVITY", "scopeKey": "FIRST_DEPOSIT_RATIO", "totalValue": "100"},
  {"quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
]
```

输出要求：

- 对已存在活动，quota 编辑器以 `POST /admin/v1/quotas` 或等价后端能力创建/更新额度配置；若当前后端仅支持创建，页面必须按“创建 quota 行”的口径展示，不声称支持调整已用/冻结值。
- 摘要展示预算、库存、用户次数和每日次数限制。
- 不展示或编辑 `usedValue`、`frozenValue` 作为运营可直接修改字段；如只读展示，必须标明运行态。

## 业务规则

### 规则配置

1. `rulePhase` 只能是 `MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT`；`BUDGET` 不是合法规则阶段。
2. 运行解释阶段固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`，其中 `RISK_LIMIT_BUDGET` 只是解释阶段名。
3. `CONDITION.logic` 必填，只能是 `AND` 或 `OR`。
4. `conditions` 必须非空。
5. 操作符只能是 `=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。
6. 字段路径必须使用白名单：标准事件字段 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`、`occurredAt`，以及 `attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、`state.plugin.<pluginCode>.*`。
7. 非标准顶层业务字段如 `orderAmount`、`deposit.amount`、`user.vipLevel` 必须拒绝，用户应改用 `attributes.amount` 或 facts/state 路径。
8. `enabled=false` 只影响运行时跳过，不放宽保存或上线前 Schema 校验。

### 奖励配置

1. `rewardType` 只能是 `BONUS`、`CASH`、`COUPON`、`POINT`、`GIFT`、`DRAW_CHANCE`、`MEMBERSHIP`。
2. `calculateType` 只能是 `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
3. `rewardType × calculateType` 必须符合可执行组合矩阵：`BONUS / CASH` 支持全部 6 种计算方式；`POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 当前只支持 `FIXED`，搭配 `RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT` 必须拒绝。
4. 金额类 `BONUS / CASH` 必须配置 `currency`，金额计算使用 Decimal，输出保留 4 位小数。
5. `POINT`、`DRAW_CHANCE` 不允许 `currency`；`COUPON` 只有 `faceValue` 需要 `currency`；`GIFT` / `MEMBERSHIP` 只有 `valuationAmount` 需要 `currency`。
6. `flowMultiple > 0` 只允许 `BONUS`；其他奖励类型必须为 `0` 或不传。
7. 非金额类 `FIXED` 必须配置对应实际权益数量或引用：`POINT.points`、`COUPON.quantity`、`GIFT.quantity`、`DRAW_CHANCE.chanceCount`、`MEMBERSHIP.durationDays` 或永久会员口径；数量字段必须为正整数。
8. `AUTO` 发奖的 `grantChannel` 支持 `HTTP / WEBHOOK / MQ`；`HTTP / WEBHOOK` 的 `targetRef` 必须是 http(s) URL，`MQ` 必须是受支持的 destination URI，且不得包含凭据、query 或 fragment。
9. `MANUAL / MANUAL_EXPORT` 可配置为人工发奖口径，但不得生成自动 outbox；页面必须在摘要中明确其人工处理语义。
10. `LADDER` 区间必须闭区间、`min <= max`、不重叠。
11. `RANDOM` 必须有合法范围、步长和稳定 seed 口径；不得使用当前时间造成不可复现。
12. `PROBABILITY` 的 items 必须非空，权重总和必须大于 0；显式概率不能超出 1 或 100% 口径。
13. `POOL_SPLIT` 只表示事件即时分摊，不表示活动结束后整池清算。
14. 金额为 `0.0000` 且无其他实际权益时，不生成 grant_order/outbox，不扣 quota；非金额权益数量为正时可发奖，`reward_amount=0.0000` 不代表无奖励。

### Quota 配置

1. `quotaType` 只能是 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
2. `scope` 只能是 `ACTIVITY`、`USER`、`IP`、`DEVICE`；当前可执行默认组合以数据模型中的四类组合为准。
3. `BUDGET` 默认 `scope=ACTIVITY`、`scopeKey=TOTAL`，消耗金额等于实际 grant order 金额。
4. `STOCK` 默认 `scope=ACTIVITY`、`scopeKey=rewardCode`，每个实际发奖 reward 默认消耗 1。
5. `USER_LIMIT` 默认 `scope=USER`、`scopeKey=subject.id`，每个 rewardable participation 消耗 1。
6. `DAILY_LIMIT` 默认 `scope=USER`、`scopeKey=subject.id:activityLocalDate`，自然日按活动 timezone 计算。
7. `totalValue` 必须为正 Decimal 或正整数；预算可用 Decimal，库存/次数默认要求整数语义。
8. 缺少 quota 行表示无限制，不应自动创建 `totalValue=0`。
9. 编辑器不得允许运营直接改运行态的 `used_value`、`frozen_value` 或 `version`。

## 边界条件

- 普通表单无法覆盖的合法高级字段，必须可通过 JSON 模式保留和编辑。
- JSON 模式中删除必填字段、改变类型或添加未知字段时，保存前或保存响应必须明确定位错误。
- 多条规则 `sort` 相同时，摘要应提示执行顺序可能按后端稳定排序；不要求本需求定义新排序语义。
- 多奖励同时存在时，预算摘要必须展示总预算影响和每个 reward 的库存影响。
- 非金额奖励如 `POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 必须展示权益数量或外部引用，不得只展示 `0.0000` 金额。
- `facts.*` 字段路径只能作为只读事实引用；编辑器不负责创建 Fact Provider。
- 绑定 `pluginCode` 的活动才能使用对应 `state.plugin.<pluginCode>.*` 路径；普通活动不应引导用户使用插件路径。
- 模拟运行只预览规则、奖励、quota、grant/outbox 影响，不锁定 quota，不发奖，不消费插件状态。

## 异常场景

| 场景 | 期望 |
|---|---|
| JSON 语法错误 | 阻止保存，显示行列或 JSON path。 |
| 未知字段 | 阻止保存或上线，错误定位到具体路径。 |
| 非法 `rulePhase` / `ruleType` / `rewardType` / `calculateType` / `quotaType` | 阻止保存或上线，显示合法枚举。 |
| 非金额奖励搭配非 `FIXED` 计算方式，如 `POINT + RATIO`、`COUPON + LADDER`、`GIFT + RANDOM`、`DRAW_CHANCE + PROBABILITY`、`MEMBERSHIP + POOL_SPLIT` | 阻止保存或上线，错误定位到 `rewards[n].calculateType`，提示该 rewardType 当前只支持 `FIXED`。 |
| 字段路径不在白名单 | 阻止保存或上线，提示使用标准事件、attributes、facts 或 state 路径。 |
| `IN / NOT_IN` 的 value 不是非空数组 | 阻止保存。 |
| Decimal、金额、比例、权重、概率不可解析 | 阻止保存，不能自动转成 float 或 0。 |
| 概率总权重为 0 或概率超界 | 阻止保存或上线。 |
| LADDER 区间重叠或边界非法 | 阻止保存或上线。 |
| Quota `totalValue <= 0` | 阻止保存。 |
| STOCK scopeKey 不匹配任何 rewardCode | 至少给出风险提示；上线前应以后端校验为准阻断不可执行配置。 |
| targetRef 缺失或协议非法 | 阻止 AUTO 发奖配置保存或上线。 |
| 后端校验返回错误 | 前端必须展示后端权威错误，不得展示保存成功。 |

## 非目标范围

- 不引入无限制脚本执行。
- 不做可视化复杂公式编排器。
- 不新增规则 evaluator、奖励 calculator、quota 类型或新的活动引擎语义。
- 不绕过后端保存校验、模拟校验、审批和上线前校验。
- 不直接修改用户余额、积分、券库存、礼品库存或会员系统。
- 不校验外部 `couponTemplateId`、`giftSkuId`、`drawPoolCode`、`membershipPlanId` 的真实存在性；本轮只校验引用形态和发奖指令配置。
- 不提供运行态 quota 调账能力，不允许编辑 `used_value` / `frozen_value`。
- 不把模拟运行变成 quota 锁定、真实发奖或插件状态消费。

## 使用示例

### 首存

- 事件：`DEPOSIT_SUCCESS`
- 规则：`attributes.isFirstDeposit = true` 且 `attributes.amount >= 100.00`
- 奖励：`BONUS + RATIO`，`baseField=attributes.amount`，`rate=0.20`，`maxAmount=500.00`，`flowMultiple=3`
- Quota：`BUDGET TOTAL`、`USER_LIMIT subject.id`
- 验收：合法配置可保存，模拟金额按 Decimal 输出 4 位；非首存或金额不足显示规则拒绝。

### 签到

- 事件：`SIGN_IN_SUCCESS`
- 规则：`attributes.continueDays >= 1`
- 奖励：`BONUS + LADDER`，按 `attributes.continueDays` 阶梯发放
- Quota：`DAILY_LIMIT subject.id:activityLocalDate`、`BUDGET TOTAL`
- 验收：阶梯闭区间不重叠；同一活动时区自然日只允许一次。

### 返水

- 事件：`REBATE_SETTLED`
- 规则：`attributes.validBetAmount > 0`
- 奖励：`BONUS + RATIO`，`baseField=attributes.validBetAmount`，配置封顶
- Quota：`USER_LIMIT` 按结算周期或用户维度、`BUDGET TOTAL`
- 验收：有效投注额缺失或不可解析时拒绝；金额封顶和预算影响在预览中可见。

## 风险点

- 配置编辑器过宽会把未知业务字段写入活动配置，导致运行时不可解释或默认通过。
- 表单模式和 JSON 模式数据不同步会造成“预览正确但保存错误”。
- 概率、随机、奖池分摊若缺少稳定 seed 和 Decimal 口径，会导致模拟与正式运行不一致。
- Quota 编辑若误允许修改 `used/frozen`，会破坏预算、库存和对账。
- 外部奖励引用只能校验形态，不能证明外部系统真实存在；需在模拟、回调和对账中继续发现。
- 高风险活动配置页面必须依赖审批和上线前校验，前端不能作为唯一安全边界。

## 验收标准

1. 规则编辑器能以普通表单配置常见 `CONDITION`，覆盖 `MATCH / QUALIFY / REWARD / RISK / LIMIT` 阶段。
2. 规则编辑器能拒绝非法 phase、非法 op、空 conditions、非法字段路径、非法 value 类型，并展示错误路径。
3. 奖励编辑器能按兼容矩阵保存合法配置：`BONUS / CASH` 支持保存合法 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`；`POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 仅支持保存合法 `FIXED`。
4. 奖励编辑器能拒绝非法金额、比例、概率、权重、阶梯重叠、缺失或禁止的 currency、非法 targetRef、非法 `flowMultiple`，以及非金额奖励搭配非 `FIXED` 的组合。
5. 非金额奖励非法组合必须作为关键验收样例覆盖，至少包含以下一种或等价参数化覆盖：`POINT + RATIO`、`COUPON + LADDER`、`GIFT + RANDOM`、`DRAW_CHANCE + PROBABILITY`、`MEMBERSHIP + POOL_SPLIT`；预期均为拒绝保存或展示后端 `400 Bad Request`，不能生成可模拟执行的配置。
6. Quota 编辑器能表达预算、库存、用户次数、每日次数限制，并正确区分“不限制”和 `totalValue=0`。
7. JSON 高级模式支持格式化、语法错误提示、JSON path 错误定位，并与普通表单保持数据一致；在 JSON 模式输入非金额奖励非 `FIXED` 时也必须拒绝。
8. 配置预览展示最终 JSON 和人类可读摘要；摘要覆盖规则条件、奖励计算、发奖方式、quota 影响。
9. 保存后的活动配置可被 `POST /admin/v1/simulate` 解释，模拟结果展示 ruleResult、rewardPreviews、quotaImpactPreviews、grant/outbox preview 或拒绝原因。
10. 首存、签到、返水至少各有一例合法配置样例和一例非法配置校验样例。
11. 后端校验错误必须被 UI 展示，不得被前端吞掉或转换成保存成功。
12. 单元测试覆盖 schema 校验和关键 UI 数据转换；最低验收仍要求 `go test ./...` 通过。

## 待确认问题

当前无阻塞性待确认问题，可按上述默认最小可验收口径进入研发。

非阻塞后续问题：

- 是否需要在页面中只读展示 `used_value` / `frozen_value` 和长期冻结告警。
- 是否需要接入外部券、礼品、抽奖、会员系统的引用存在性校验。
- 是否需要为 `FORMULA` 做专用安全表达式编辑器；本轮默认只要求 JSON 或简单文本输入，不做复杂编排器。

## Score

```text
95
```

评分：

```text
需求清晰度        19/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      19/20
约束一致性        19/20
```

扣分原因：外部权益模板真实性、FORMULA 专用编辑体验和 quota 运行态展示仍需后续需求深化；但这些不影响本轮规则/奖励/quota 编辑器的最小可验收闭环。Review 指出的奖励类型与计算方式兼容矩阵已补齐，不再作为阻塞项。

## Decision

```text
PASS
```

需求：2026-05-29-admin-rule-reward-quota-editors
