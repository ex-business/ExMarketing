# Analysis

## 需求 ID

2026-05-24-decimal

## 标题

Decimal 金额计算规范

## Review 反馈处理

本轮读取了 `tasks/2026-05-24-decimal/review.md`。该文件目前是 Review Agent 模板，`Findings` 下的 `P0` / `P1` / `P2` 未填写具体问题，`Open Questions` 为占位文本，`Decision` 为 `PASS / BLOCK` 占位规则，不存在可逐条修复的实质打回意见。

处理结论：

- 对模板中的 `P0` / `P1` / `P2`：无具体条目，按“无实质 review finding”处理。
- 对模板中的 `Open Questions`：无具体问题，按“无实质待答问题”处理。
- 对模板中的 `PASS / BLOCK`：这是决策占位，不作为 Review 打回原因。

本次仍按返工要求补齐完整需求口径，使后续 Review 可直接按本文件审核。

## Analysis 自检修正

上一轮 `analysis.md` 是未完成模板，存在以下问题：

- `现状`、`目标理解`、`方案`、`改动范围`、`风险点`、`验收方式` 均为“待填写”。
- `Score` 为 `0`，低于 80。
- `Decision` 为 `PASS / BLOCK` 占位，未给出真实结论。

本轮修正方式：

- 补齐背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标、风险和验收标准。
- 对文档未唯一给出的舍入规则，给出默认最小可验收口径：金额计算全程使用 Decimal，最终对外金额统一量化为 4 位小数，默认按四舍五入到最近值处理；封顶比较在量化前完成，额度扣减使用最终量化后的金额。
- 明确金额、预算、库存、发奖相关新增逻辑禁止使用 `float64`，并要求比例赠送、返水、阶梯奖励、上限截断具备可重复验收样例。
- 重新评分并给出明确 `Decision PASS`。

## 背景与目标

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动规则、计算奖励、扣减预算库存、生成 participation / grant_order / outbox，不直接修改外部用户余额，也不依赖外部业务库。

本需求来源于《22-独立活动营销中台设计》中的统一金额要求：金额计算必须使用 Decimal，禁止使用 `float64`。当前任务要把“金额、预算、库存、奖励计算”相关的业务口径细化为可实现、可 review、可测试的规范，避免比例赠送、返水、阶梯奖励和封顶计算出现二进制浮点精度漂移。

目标：

- 所有奖励金额、预算金额、库存/次数额度值、流水倍数、比例费率等涉及资金或额度计算的值都按 Decimal 语义处理。
- 外部事件、活动配置、Quota 配置、模拟输出、发奖单、Outbox payload 和审计快照中的金额值保持字符串 Decimal 表达，不通过 JSON number 或浮点表达金额。
- 奖励计算结果对外统一输出 4 位小数，和现有数据模型 `DECIMAL(18,4)`、`reward_amount`、Quota `total_value / used_value / frozen_value` 保持一致。
- 新增金额逻辑不得使用 `float64` 承载、比较、计算或格式化金额。

## 参与方

- 运营/Admin：配置活动、奖励、预算、库存、次数限制，查看模拟结果和审计。
- 外部业务系统：通过标准事件上报充值、投注、订单等业务事实，金额字段放入 `attributes`。
- ExMarketing Engine：匹配活动、执行规则、计算奖励、预览/扣减 quota、生成 participation 和 grant_order。
- Reward/Outbox Worker：把 Decimal 金额以标准 payload 形式发给外部发奖系统。
- 外部发奖系统：接收标准发奖指令并按字符串金额处理，回调发奖结果。
- Review/Test Agent：审核本规范是否清楚，并按验收标准验证无精度漂移。

## 输入输出

### 输入

- 标准事件：`attributes.amount`、`attributes.validBetAmount`、`attributes.rebateBaseAmount`、`attributes.currency` 等业务字段。金额字段必须允许字符串 Decimal，推荐只接受字符串作为金额源。
- 奖励配置：`amount`、`rate`、`maxAmount`、`minAmount`、`step`、`ladders[].min`、`ladders[].max`、`ladders[].amount`、`items[].amount`、`poolAmount`、`weight`、`flowMultiple` 等。
- Quota 配置：`totalValue`、`usedValue`、`frozenValue`。其中 `BUDGET` 是金额语义，`STOCK` / `USER_LIMIT` / `DAILY_LIMIT` 是数量语义但仍以 Decimal 存储和原子扣减。
- 规则比较值：涉及 `>`、`>=`、`<`、`<=` 的金额或数值比较必须可解析为 Decimal。

### 输出

- `rewardPreviews.amount`、`grant_order.reward_amount`、`reward_payload_json.amount`、Outbox reward amount：统一输出 4 位小数字符串，例如 `60.0000`。
- `quotaImpactPreviews.amount`、`marketing_quota.used_value` 增量：按最终可发奖金额或数量扣减值输出 Decimal。
- `snapshot_json.rewards`：保存 rewardCode、calculateType、baseAmount、rate、maxAmount、capped、amount、currency 等解释字段，所有金额字段使用字符串。
- 零金额奖励：金额类奖励结果为 `0.0000` 且无其他实际权益时，输出 QUALIFIED/跳过语义，不生成 grant_order/outbox，不扣 quota。

## 业务规则

### Decimal 基础规则

- 金额、预算、库存金额、奖励金额、费率、流水倍数、比例计算中间值必须使用 Decimal 精确十进制语义。
- 禁止用 `float64` 作为金额值的解析目标、计算中间值、比较值、JSON decode 目标或格式化来源。
- JSON 中金额和费率的推荐表达是字符串 Decimal；若历史兼容允许 JSON number，系统也必须用保留原始数字文本的方式解析，不得先转为浮点。
- 金额输出统一保留 4 位小数，不足补 0。
- 默认最小舍入口径：计算过程不做中间舍入；最终对外金额量化为 4 位小数，采用四舍五入到最近值。若业务配置需要特殊舍入模式，本需求不扩展配置项，后续另立需求。
- 金额和 Quota 持久化范围沿用 `DECIMAL(18,4)`；超过该范围或无法量化到 4 位的输入必须在配置或事件处理阶段拒绝，不能静默截断。

### 固定金额 FIXED

- `amount` 必须是非负 Decimal。
- 输出金额 = `amount` 量化到 4 位。
- `amount=0` 按零金额奖励处理。

### 比例赠送 RATIO

- `baseField` 从标准事件、facts 或 state 中读取，值必须可解析为非负 Decimal。
- `rate` 必须是非负 Decimal，表示乘数，例如 `0.20` 表示 20%。
- 未配置 `maxAmount`：奖励金额 = `baseAmount * rate`，最终量化为 4 位。
- 配置 `maxAmount`：先用未量化计算值与 `maxAmount` 比较，取较小值，再量化为 4 位；`capped=true/false` 必须进入解释字段。
- 比例赠送必须覆盖类似 `0.1 + 0.2`、`299.00 * 0.20`、`333.33 * 0.15` 这类浮点易漂移场景，输出不得出现 `59.800000000000004` 等浮点痕迹。

### 返水 Rebate

- 返水在当前最小口径下不新增专用活动模型，可视为 `RATIO` 奖励：`baseField` 指向有效投注额、净输赢或业务事件携带的返水基数。
- 返水基数必须由外部系统通过 `attributes` 或已注册 fact 提供，ExMarketing 不连接外部游戏/投注库自行汇总。
- 返水计算遵循 `baseAmount * rate`、可选 `maxAmount` 封顶、最终 4 位输出。
- 若事件缺少返水基数、基数为负、费率非法或币种缺失，金额类 reward 必须拒绝或跳过，不得按 0 隐式发奖。

### 阶梯奖励 LADDER

- `field` 指向的阶梯判断值必须可解析为 Decimal。
- `ladders` 必须非空，区间按闭区间 `[min, max]` 判断。
- 阶梯区间不得重叠；边界命中必须唯一。
- 命中阶梯后使用该阶梯的 `amount` 作为奖励金额，并量化到 4 位。
- 未命中任何阶梯时按零金额奖励处理，不生成发奖单、不扣 quota。
- 阶梯边界测试必须覆盖 `min`、`max`、相邻边界和小数边界。

### 上限截断

- `maxAmount` 必须是非负 Decimal。
- 封顶只限制最终奖励金额，不改变原始 `baseAmount`、`rate` 或规则判断值。
- 当计算值大于 `maxAmount` 时，输出金额等于 `maxAmount` 的 4 位量化结果，并在解释字段标记 `capped=true`。
- 当计算值等于 `maxAmount` 时，输出金额等于 `maxAmount`，可标记 `capped=false` 或 `capped=true`，但验收要求解释字段能看出未超额发奖。

### Quota / Budget / Stock

- `BUDGET` 扣减金额 = 同一 activity 本次所有金额类 reward 的最终 4 位 `reward_amount` 合计。
- `STOCK` 按 rewardCode 维度扣减，默认每个实际发奖 reward 扣 `1.0000`；库存配置值必须是非负整数语义 Decimal。
- `USER_LIMIT` / `DAILY_LIMIT` 按 rewardable participation 扣 `1.0000`，配置值必须是非负整数语义 Decimal。
- 缺少对应 quota 行表示无限制，沿用现有兼容口径。
- 金额为 `0.0000` 的金额类 reward 不生成 grant_order/outbox，也不扣 BUDGET/STOCK/USER_LIMIT/DAILY_LIMIT。
- 多 reward 同一活动命中时，预算合计和库存/次数扣减必须保持原子性；任一 quota 不足时，不得留下 `REWARDABLE` 脏记录或部分 grant_order。

## 边界条件

- 金额输入为空字符串、非数字、NaN、Infinity、科学计数法是否支持不作为扩展目标；默认最小口径为拒绝不可规范解析的 Decimal 字符串。
- 负数金额、负数费率、负数预算、负数库存、负数流水倍数默认非法。
- 金额小于最小 4 位精度且量化后为 `0.0000` 时，按零金额奖励处理。
- 不同币种不能在同一金额 reward 中混算；金额类 `BONUS` / `CASH` 必须配置 `currency`。
- 非金额 reward 的 `reward_amount` 固定为 `0.0000`，不表示现金金额；其权益数量仍需按整数语义校验。
- 模拟运行只预览 Decimal 计算和 Quota 影响，不锁定 quota、不发奖、不投递 outbox。
- 幂等重放同一事件时，Decimal 计算结果必须稳定一致，不受当前时间、随机浮点误差或格式化差异影响。

## 异常场景

- 金额字段无法解析：创建活动、创建 quota 或事件处理应返回明确错误；模拟运行应给出不可计算原因。
- `baseField` 缺失或类型不匹配：对应 reward 不得按浮点默认值或 0 静默计算。
- `rate`、`amount`、`maxAmount`、`ladders` 非法：活动配置应被拒绝。
- 阶梯重叠或边界无法唯一命中：活动配置应被拒绝。
- Quota 不足：返回/记录 `QUOTA_EXCEEDED`，不生成部分发奖结果。
- 计算结果超过持久化范围：拒绝处理并保留可审计错误，不允许截断后继续发奖。

## 非目标范围

- 不设计或实现具体代码改法。
- 不新增专用返水活动插件，不连接外部投注、钱包、用户或风控数据库。
- 不改变现有 participation、grant_order、outbox 主流程。
- 不引入新的金额精度配置、币种小数位配置或运营可选舍入模式。
- 不实现预算冻结/确认/释放模型；该能力属于后续高风险资金活动阶段。
- 不改变非金额权益的发奖语义，只约束其估值字段和 quota 数量字段的 Decimal 表达。

## 风险

- 历史兼容风险：已有配置或事件如果使用 JSON number 表达金额，必须确保解析过程不经过浮点，否则仍可能漂移。
- 舍入争议风险：业务文档未指定银行家舍入、向下取整或按币种小数位舍入。本轮采用最终 4 位四舍五入的默认最小口径，后续如有币种级规则需另立需求。
- 预算风险：比例、阶梯、多奖励合计和封顶顺序不一致会造成预算与发奖单不一致，验收必须同时看 reward_amount 和 quota impact。
- 审计风险：若 snapshot/outbox 中金额格式不统一，会影响对账；必须统一字符串和 4 位输出。
- 范围风险：Decimal 规范容易扩散到所有规则数值比较。本轮只强约束金额、奖励、预算、库存/次数额度和相关数值比较，不扩大到普通非金额业务字段。

## 验收标准

- 创建活动时，金额类 reward 的 `amount`、`rate`、`maxAmount`、`ladders[].amount`、`flowMultiple` 等非法 Decimal 配置会被拒绝。
- 创建 quota 时，`totalValue` 必须按 Decimal 校验；`BUDGET` 支持金额 4 位语义，`STOCK` / `USER_LIMIT` / `DAILY_LIMIT` 拒绝非整数语义值。
- 比例赠送样例 `baseAmount=299.00, rate=0.20` 输出 `59.8000`，不得出现浮点尾差。
- 比例封顶样例 `baseAmount=9999.99, rate=0.20, maxAmount=500.00` 输出 `500.0000`，解释字段能体现封顶。
- 返水样例 `rebateBaseAmount=333.33, rate=0.015` 输出稳定 4 位 Decimal，重复模拟/重复事件结果一致。
- 阶梯奖励在区间边界 `min`、`max` 上命中唯一阶梯；重叠阶梯配置被拒绝。
- 金额类 reward 计算为 `0.0000` 时，只生成 QUALIFIED/跳过语义，不生成 grant_order/outbox，不扣 quota。
- 多 reward 命中时，`BUDGET` 扣减值等于所有最终 `reward_amount` 的 Decimal 合计，不受中间浮点误差影响。
- 模拟运行的 `rewardPreviews`、`quotaImpactPreviews` 与真实事件处理生成的 grant_order/outbox 金额口径一致。
- 新增金额、预算、库存、奖励计算逻辑不得使用 `float64`；Review/Test 可通过静态扫描和针对性测试确认。
- 基础回归仍需满足 `go test ./...` 和 `go build ./...`。

## 待确认问题

当前无阻塞问题。以下为后续可增强项，不阻塞本需求：

- 是否需要按币种配置输出小数位，例如 JPY 0 位、USDT 6 位。
- 是否需要运营可配置舍入模式，例如向下取整、银行家舍入。
- 是否需要支持科学计数法 Decimal 输入。
- 是否需要把历史 JSON number 金额输入逐步收敛为只允许字符串。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      19/20
架构一致性        20/20
总分              92/100
```

## Decision

```text
PASS
```
