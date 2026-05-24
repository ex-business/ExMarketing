# Analysis

## 需求 ID

2026-05-24-113400-4

## 标题

奖励计算引擎

## 资料范围

已读取并用于本次分析：

```text
tasks/2026-05-24-113400-4/task.md
tasks/2026-05-24-113400-4/analysis.md
tasks/2026-05-24-113400-4/review.md
README.md
docs/architecture.md
docs/data-model.md
docs/api.md
docs/roadmap.md
requirements/backlog.json
```

本次只做需求分析和需求细化；未读取业务代码，未设计具体代码改法，未修改除本文件外的任何文件。

## 背景目标

ExMarketing 是独立活动营销中台，通过标准事件、可选 Fact Query、规则引擎、奖励计算、额度扣减、发奖单和 Outbox 与外部系统集成。奖励计算位于链路：

```text
Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Reward Calculation -> Quota / Budget / Inventory -> Grant Order -> Outbox
```

本任务目标是建立统一 RewardCalculator 业务口径，覆盖 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT` 六类 `calculateType` 的最小可执行与可验收规则。所有金额、比例、权重、奖池分摊和封顶计算必须使用 Decimal 语义，禁止二进制浮点口径参与业务金额计算。

Roadmap 中 `RANDOM / PROBABILITY / POOL_SPLIT` 更偏 Phase 4 玩法型活动，但 `task.md` 已明确把六类计算方式列入本需求。因此本次不再把后三类排除出交付范围，而是定义最小普通活动口径：

```text
本任务内实现：六类 calculateType 的统一计算、校验、解释结果、错误原因和基础测试。
本任务外保留：复杂玩法插件、场次调度、实时奖池生命周期、WebSocket 推送、排行榜、审批流和生产级概率库存审计。
```

## 参与方

```text
运营/后台配置方
  配置活动奖励、奖励类型、计算方式、金额、比例、封顶、阶梯、随机区间、概率奖项、奖池分摊、发奖目标和启用状态。

外部业务系统
  通过标准事件 attributes 提供奖励计算需要的业务字段；必要时通过可选 facts 提供外部事实。

ExMarketing 事件处理链路
  在活动匹配和规则判断通过后调用奖励计算；计算成功后进入 quota 扣减、grant_order 和 outbox 创建。

模拟运行接口
  使用同一奖励计算规则输出预计奖励或失败原因，不写真实业务表。

Quota / Budget / Inventory 能力
  使用奖励计算结果参与 BUDGET、STOCK、USER_LIMIT 等额度扣减。

外部发奖系统
  接收 outbox 投递的奖励命令；不参与奖励金额计算。

后续玩法/插件任务
  在本任务最小计算口径之上补充场次、奖池库存、玩法上下文、概率审批和前端互动。
```

## 输入

奖励计算输入来自标准事件、活动上下文和 `marketing_reward` 配置。

### 奖励配置字段

```text
rewardCode         奖励编码；用于库存 scopeKey、结果解释和发奖追踪
rewardType         奖励类型；当前普通活动支持 BONUS / CASH / POINT
calculateType      计算方式；本任务覆盖 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT
rewardConfig       计算参数 JSON
grantMode          当前可执行边界为 AUTO
grantChannel       当前可执行边界为 HTTP / WEBHOOK
flowMultiple       流水倍数；如参与金额类 payload，必须按 Decimal 解析
enabled            是否启用
version            配置版本
```

### 可读取上下文

奖励计算只允许读取已明确开放的字段命名空间：

```text
attributes.<path>
state.activity.<field>
state.subject.<field>
state.quota.<quotaType>.<field>
state.exclusive.<field>
facts.<factCode>.<path>
```

约束：

```text
外部业务字段必须来自事件 attributes 或可选 facts。
不得要求 ExMarketing 直接连接外部业务数据库。
字段路径缺失或字段值类型不匹配时不得静默按 0 计算。
```

### 确定性随机输入

`RANDOM` 和 `PROBABILITY` 必须使用稳定种子保证正式执行、重复事件、重放和模拟运行在同一输入下可复现。

种子业务口径：

```text
seedSource 默认使用 idempotencyKey。
如果 idempotencyKey 不可用，按 eventId + ":" + activityId + ":" + rewardCode 拼接。
同一 tenantId、activityId、rewardCode、事件幂等键、rewardConfig version 下，随机结果必须稳定。
seedSalt 可选；存在时加入 seed 计算，用于运营在配置版本变化时明确改变随机序列。
不得使用当前时间、进程内随机数、数据库自增 ID 或 grantOrderId 作为随机结果来源。
```

## 输出

每个启用奖励应产出一个明确的计算结果或失败结果。

成功结果：

```text
rewardCode          奖励编码
rewardType          奖励类型
calculateType       计算方式
rewardAmount        Decimal 金额或数量
rewardPayload       扩展 JSON；记录计算解释信息
reasonCode          OK
reason              可读解释，用于模拟运行和测试断言
```

失败结果：

```text
rewardCode          奖励编码
calculateType       计算方式
reasonCode          稳定错误码
reason              可读失败原因
field               可选，指向相关配置字段或输入字段路径
```

链路消费规则：

```text
计算成功后，rewardAmount 写入 marketing_grant_order.reward_amount。
rewardPayload 写入 marketing_grant_order.reward_payload_json，并用于 outbox reward command。
BUDGET 扣减金额 = 同一 activity 本次所有成功奖励 rewardAmount 合计。
STOCK 每个生成发奖单的 enabled reward 默认扣减 1，scopeKey = rewardCode。
USER_LIMIT 每个 rewardable participation 默认扣减 1。
计算失败时不得生成 grant_order、outbox，也不得扣减 quota。
```

## 稳定 reasonCode

本任务要求最少固化以下 reasonCode，便于 Dev/Test/API 文档对齐：

```text
OK
SKIPPED_DISABLED_REWARD
INVALID_REWARD_CONFIG
UNSUPPORTED_CALCULATE_TYPE
FIELD_MISSING
FIELD_TYPE_MISMATCH
DECIMAL_PARSE_ERROR
INVALID_DECIMAL_SCALE
NEGATIVE_AMOUNT
NEGATIVE_RATE
INVALID_REWARD_TARGET
INVALID_REWARD_TYPE
RATIO_BASE_MISSING
RATIO_MAX_AMOUNT_INVALID
LADDER_NOT_MATCHED
LADDER_OVERLAPPED
RANDOM_RANGE_INVALID
RANDOM_STEP_INVALID
PROBABILITY_ITEMS_EMPTY
PROBABILITY_TOTAL_INVALID
PROBABILITY_ITEM_INVALID
POOL_AMOUNT_INVALID
POOL_PARTICIPANT_INVALID
POOL_WEIGHT_INVALID
ZERO_REWARD_SKIPPED
```

说明：

```text
创建活动阶段能发现的配置错误，应以 400 和对应 reasonCode 返回。
运行阶段因历史配置或输入数据导致的错误，应在处理结果、审计或模拟运行中保留同一 reasonCode。
```

## 业务规则

### 通用规则

1. 只有 `enabled=true` 的奖励配置参与计算；`enabled=false` 在模拟运行中可展示为 `SKIPPED_DISABLED_REWARD`，不生成发奖单、不扣 quota。
2. 本任务允许创建和执行的 `calculateType` 为 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT`。
3. 未知 `calculateType` 必须返回 `UNSUPPORTED_CALCULATE_TYPE`，不得默认按固定金额或 0 处理。
4. `rewardConfig` 必须是非空 JSON object；`null`、空对象、数组、字符串等均为非法配置。
5. 所有金额、比例、封顶、阶梯金额、随机区间、概率、权重、奖池金额、流水倍数必须使用 Decimal 语义解析和计算，禁止二进制浮点口径。
6. 金额类输出统一为最多 4 位小数；超过 4 位按四舍五入到 4 位输出。配置值本身可超过 4 位，但输出必须稳定。
7. 比例、基础金额、封顶、固定金额、阶梯金额、随机金额、概率、权重、奖池金额不得为负数。
8. 金额类奖励 `BONUS / CASH` 必须有 `currency`；`POINT` 可不要求 `currency`。
9. `POINT` 的 `rewardAmount` 本任务按 Decimal 数量处理，是否必须为整数留给后续 Reward Schema 任务。
10. `AUTO + HTTP/WEBHOOK` 发奖时 `targetRef` 必填，且必须为 `http` 或 `https` URL。
11. 字段路径缺失、字段值为 `null`、空字符串、布尔值、对象、数组或不可解析 Decimal 时，计算失败，不得按 0 处理。
12. 计算结果不得为负数；配置或输入导致负数结果时必须失败。
13. 同一活动存在多个 enabled reward 时，必须先完成全部奖励计算；任一奖励计算失败，整个 activity 不得进入 grant_order/outbox/quota 写入，避免部分发奖。
14. 模拟运行必须使用同一计算规则，但不得写入 event_inbox、participation、grant_order、outbox 或 quota。
15. 正式事件重复上报时，幂等收件箱应保证不重新生成 participation、grant_order、outbox 或重复扣减 quota。
16. `RANDOM / PROBABILITY` 同一输入重复计算必须得出同一结果，保证模拟运行和正式执行可解释。

### 0 金额奖励规则

为避免“成功但无实际奖励”消耗预算、库存、用户次数，本任务统一口径如下：

```text
计算得出的 rewardAmount = 0 时，视为无发奖结果，不生成 grant_order/outbox。
0 金额不扣 BUDGET。
0 金额不扣 STOCK。
如果同一 activity 所有 enabled reward 都计算为 0，则不写 REWARDABLE 发奖链路；模拟运行展示 ZERO_REWARD_SKIPPED。
USER_LIMIT 只在存在至少一个实际发奖单时扣减。
固定金额配置 amount=0、比例结果=0、随机结果=0、概率命中空奖、奖池分摊结果=0 均适用该规则。
0 金额不是错误；它是可解释 skipped 结果。
```

若产品后续需要“0 金额也记录参与或占库存”，必须另开需求调整 quota 和 participation 语义。

### FIXED 固定奖励

语义：命中奖励后返回配置中的固定金额或数量。

配置字段：

```json
{
  "amount": "18.88",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
amount 必填，必须可解析为 Decimal。
amount 不得小于 0。
rewardAmount = amount 按统一精度口径输出。
amount = 0 时按 0 金额奖励规则跳过发奖和 quota 扣减。
rewardPayload 至少包含 currency、source="FIXED"。
空配置、缺 amount、非法 amount、负数 amount、金额类缺 currency 均返回明确错误。
```

### RATIO 比例奖励

语义：按事件或上下文字段乘以比例计算奖励，可选封顶。

配置字段：

```json
{
  "baseField": "attributes.amount",
  "rate": "0.20",
  "maxAmount": "500.00",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
baseField 必填，必须指向允许读取的数值字段。
rate 必填，必须可解析为 Decimal，且不得小于 0。
maxAmount 可选；存在时必须可解析为 Decimal，且不得小于 0。
currency 对金额类 rewardType 必填。
基础金额缺失、不可解析或为负数时返回明确错误。
未配置 maxAmount 时，rewardAmount = baseField * rate。
配置 maxAmount 时，rewardAmount = min(baseField * rate, maxAmount)。
结果为 0 时按 0 金额奖励规则跳过发奖和 quota 扣减。
最终 rewardAmount 按统一精度口径输出。
rewardPayload 至少包含 baseField、baseAmount、rate、maxAmount、capped、currency。
```

### LADDER 阶梯奖励

语义：读取一个可比较数值字段，根据阶梯区间命中对应奖励金额或数量。

配置字段：

```json
{
  "field": "attributes.continueDays",
  "ladders": [
    {"min": "1", "max": "2", "amount": "5.00"},
    {"min": "3", "max": "6", "amount": "18.00"},
    {"min": "7", "max": "999", "amount": "58.00"}
  ],
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
field 必填，必须指向允许读取的可比较数值字段。
ladders 必填且不可为空。
每个 ladder 必须包含 min、max、amount。
min、max、amount 均按 Decimal 解析。
min <= max。
amount 不得小于 0。
阶梯区间使用闭区间：min <= field <= max。
阶梯可以乱序配置，但执行前必须按 min 升序形成稳定解释。
阶梯不得重叠；重叠配置应在创建或执行时返回 LADDER_OVERLAPPED。
字段值命中多个阶梯在合法配置下不应发生；若历史配置导致多个命中，必须按配置错误处理，不得任意选第一档。
字段值未命中任何阶梯时返回 LADDER_NOT_MATCHED；不返回 0 奖励。
命中 amount=0 的阶梯时按 0 金额奖励规则跳过发奖和 quota 扣减。
rewardPayload 至少包含 field、fieldValue、ladderIndex、ladderMin、ladderMax、currency。
```

### RANDOM 随机金额

语义：在固定区间内按 Decimal 步长生成一个确定性随机奖励金额或数量。该能力用于普通活动里的轻量随机奖励，不包含抽奖奖项、库存池、场次或前端玩法。

配置字段：

```json
{
  "minAmount": "1.00",
  "maxAmount": "10.00",
  "step": "0.01",
  "currency": "CNY",
  "seedSalt": "campaign-202605",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
minAmount、maxAmount 必填，必须可解析为 Decimal，且不得小于 0。
minAmount <= maxAmount。
step 可选，默认 0.01；必须大于 0，且不得超过 maxAmount - minAmount，除非 minAmount = maxAmount。
候选值集合为 minAmount + n * step，且不超过 maxAmount。
如果 maxAmount - minAmount 不能被 step 整除，最后一个候选值不得超过 maxAmount，不自动补一个非 step 对齐值。
使用确定性随机输入在候选集合中选择一个值。
同一输入和同一配置版本必须返回同一 rewardAmount。
minAmount = maxAmount 时直接返回该值。
结果为 0 时按 0 金额奖励规则跳过发奖和 quota 扣减。
rewardPayload 至少包含 minAmount、maxAmount、step、seedSource、seedSalt、selectedIndex、candidateCount、currency。
```

预算口径：

```text
上线前预算估算按 maxAmount 作为单次最坏成本。
运行时 BUDGET 扣减按实际 rewardAmount。
```

### PROBABILITY 概率奖励

语义：在一组奖项中按权重确定性抽取一个奖项。该能力只覆盖单次事件即时抽取，不包含场次、排行榜、奖池库存或复杂抽奖插件。

配置字段：

```json
{
  "items": [
    {"itemCode": "NO_PRIZE", "weight": "70", "rewardType": "BONUS", "amount": "0", "label": "No prize"},
    {"itemCode": "SMALL", "weight": "25", "rewardType": "BONUS", "amount": "5.00"},
    {"itemCode": "BIG", "weight": "5", "rewardType": "BONUS", "amount": "50.00"}
  ],
  "currency": "CNY",
  "seedSalt": "lottery-lite-202605",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
items 必填且不可为空。
每个 item 必须有 itemCode、weight、amount。
itemCode 在同一 rewardConfig 内必须唯一。
weight 必须可解析为 Decimal，且必须大于 0。
amount 必须可解析为 Decimal，且不得小于 0。
所有 item 的 weight 合计必须大于 0。
概率单位为权重，不要求合计为 1、100 或 10000。
按 weight / totalWeight 的比例进行确定性选择。
同一输入和同一配置版本必须命中同一 item。
命中 amount=0 的 item 表示空奖，按 0 金额奖励规则跳过发奖和 quota 扣减。
命中 amount>0 的 item 正常生成奖励。
item.rewardType 可选；缺省使用外层 rewardType。若提供，必须是当前普通活动支持的 rewardType。
金额类奖项使用外层 currency；如 item.currency 存在，则必须与外层 currency 一致，避免同一 reward 配置混币种。
rewardPayload 至少包含 itemCode、selectedIndex、weight、totalWeight、seedSource、seedSalt、currency、label。
```

预算口径：

```text
上线前预算估算按 items 中最大 amount 作为单次最坏成本。
运行时 BUDGET 扣减按命中 item 的实际 amount。
STOCK 默认按 rewardCode 扣 1；本任务不支持按 itemCode 配置独立库存。
```

### POOL_SPLIT 奖池瓜分

语义：按当前事件提供的参与数或权重，从一个固定奖池金额中计算本次主体应得份额。该能力只覆盖事件即时计算，不包含活动结束后批量结算、动态奖池累计、排行榜或场次生命周期。

配置字段：

```json
{
  "poolAmount": "1000.00",
  "splitBy": "COUNT",
  "participantCountField": "attributes.participantCount",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

权重分摊示例：

```json
{
  "poolAmount": "1000.00",
  "splitBy": "WEIGHT",
  "weightField": "attributes.userWeight",
  "totalWeightField": "attributes.totalWeight",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

业务规则：

```text
poolAmount 必填，必须可解析为 Decimal，且不得小于 0。
splitBy 必填，当前仅允许 COUNT 或 WEIGHT。
COUNT 模式下 participantCountField 必填，字段值必须可解析为 Decimal，且必须大于 0。
COUNT 模式 rewardAmount = poolAmount / participantCount。
WEIGHT 模式下 weightField 和 totalWeightField 必填，字段值必须可解析为 Decimal，且 weight >= 0、totalWeight > 0、weight <= totalWeight。
WEIGHT 模式 rewardAmount = poolAmount * weight / totalWeight。
结果按统一 4 位小数四舍五入。
余数不在本任务内做二次分配；本任务只保证单次计算不超过按公式应得的四舍五入结果。
poolAmount=0、COUNT 分摊结果=0、WEIGHT 中 weight=0 时，按 0 金额奖励规则跳过发奖和 quota 扣减。
rewardPayload 至少包含 poolAmount、splitBy、participantCount 或 weight/totalWeight、currency。
```

预算口径：

```text
上线前预算估算按 poolAmount 作为该 reward 配置最大风险敞口。
运行时 BUDGET 扣减按本次实际 rewardAmount。
本任务不提供“整池只扣一次”的结算模型；若需要活动结束后统一瓜分，另开玩法/结算任务。
```

## 边界条件

```text
rewardConfig 为 null、空对象、数组、字符串
未知 calculateType
enabled=false 奖励
缺少必填字段
字段路径命名空间非法
字段路径不存在
字段值为 null、空字符串、布尔值、对象或数组
Decimal 解析失败
金额、比例、权重或奖池为负数
金额计算结果超过 4 位小数
固定金额为 0
比例 rate 为 0
比例 baseField 为 0
比例结果超过 maxAmount
maxAmount 为 0 或负数
LADDER ladders 为空
LADDER min > max
LADDER amount 为 0 或负数
LADDER 阶梯乱序
LADDER 阶梯重叠
LADDER 字段值正好等于 min
LADDER 字段值正好等于 max
LADDER 未命中任何档位
RANDOM minAmount = maxAmount
RANDOM minAmount > maxAmount
RANDOM step 缺省
RANDOM step = 0 或负数
RANDOM 区间不能被 step 整除
PROBABILITY items 为空
PROBABILITY itemCode 重复
PROBABILITY weight = 0 或负数
PROBABILITY 命中空奖
PROBABILITY item rewardType 非法
POOL_SPLIT poolAmount = 0
POOL_SPLIT participantCount = 0
POOL_SPLIT weight = 0
POOL_SPLIT weight > totalWeight
金额类奖励缺 currency
AUTO + HTTP/WEBHOOK 缺 targetRef 或 targetRef 非 http(s)
多奖励活动中某一奖励失败
多奖励活动中部分奖励为 0、部分奖励大于 0
模拟运行中的计算失败
真实事件重复上报
quota 充足预期下的正常发奖
quota 不足导致的 409 回滚
```

## 异常场景

```text
400 INVALID_REWARD_CONFIG
  创建活动时 rewardConfig 不是合法 JSON object、缺必填字段、非法 Decimal、非法区间、阶梯重叠、概率奖项非法、奖池分摊字段非法。

400 UNSUPPORTED_CALCULATE_TYPE
  calculateType 不是 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT。

400 INVALID_REWARD_TARGET
  AUTO + HTTP/WEBHOOK 发奖缺 targetRef 或 targetRef 不是 http(s) URL。

事件处理失败
  历史活动配置或运行时上下文字段不满足计算要求时，不生成 grant_order/outbox，不扣 quota，并返回或记录明确失败原因。

409 QUOTA_EXCEEDED
  奖励计算成功且存在实际发奖金额，但预算、库存、用户次数不足时，按既有规则回滚 participation/grant_order/outbox，并写 QUOTA_EXCEEDED 审计。

Duplicate event
  完全重复事件返回 duplicate=true，不重新计算、不重复生成 participation/grant_order/outbox、不重复扣减 quota。

Simulation error
  模拟运行展示奖励计算失败原因和字段，不写 event_inbox、participation、grant_order、outbox 或 quota。

Zero reward
  奖励计算成功但 amount=0 时展示 ZERO_REWARD_SKIPPED，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT。
```

## 非目标范围

本任务不包含：

```text
新增业务专用 controller、job、model 或活动专属计算分支。
直接连接外部业务数据库或钱包系统。
直接给用户加钱。
绕过 participation、grant_order、outbox 的发奖逻辑。
发奖回调成功/失败后的预算确认释放。
Outbox retry、DLQ、人工重放能力。
完整后台审批流。
完整活动插件生命周期。
RANDOM 的动态库存池、场次、前端互动玩法。
PROBABILITY 的独立 item 库存、实物奖履约、概率审批、抽奖记录页。
POOL_SPLIT 的活动结束统一结算、排行榜结算、动态奖池累计、余数二次分配。
OpenAPI/Admin 鉴权。
Fact Query 缓存或 provider 配置中心。
新增 rewardType：COUPON、GIFT、DRAW_CHANCE、MEMBERSHIP 的完整发奖语义。
设计具体代码结构、函数签名、包拆分或数据库迁移细节。
```

## 风险

### P1 风险

1. `RANDOM / PROBABILITY / POOL_SPLIT` 原本在 Roadmap 中属于 Phase 4，当前为满足 task.md 范围只定义最小普通活动计算口径。复杂玩法不得在本任务中借机扩大范围。
2. 随机和概率若不确定性，会造成重复事件、模拟运行、正式运行结果不一致。本任务必须验收稳定 seed 规则。
3. 多奖励活动如果部分奖励成功、部分失败，可能产生 grant_order、quota 和 outbox 不一致。验收必须覆盖“先全部计算成功，再进入事务写入”的业务口径。
4. Decimal 精度若不固化会造成财务争议。本任务统一为输出最多 4 位小数并四舍五入，测试必须覆盖超精度输入和计算结果。
5. POOL_SPLIT 的即时分摊不等同于整池结算；若运营误用为活动结束瓜分，可能产生预算理解偏差。文档和创建校验必须说明本任务只支持事件即时计算。

### P2 风险

```text
POINT 是否必须为整数仍未由 Reward Schema 固化，本任务只按 Decimal 数量处理。
0 金额统一跳过发奖和 quota 扣减，可能与“记录一次参与但无奖励”的运营诉求不同。
RATIO、LADDER、POOL_SPLIT 的字段可能来自 facts，外部事实不可用时计算失败会影响活动命中体验。
PROBABILITY 只支持 rewardCode 级库存，不支持 itemCode 级库存，奖项库存控制需要后续任务补齐。
POOL_SPLIT 余数不二次分配，财务对账需要明确以单次 rewardAmount 为准。
创建阶段和运行阶段都可能遇到非法历史配置，需保证运行时也有明确失败原因。
```

## 验收标准

1. `FIXED` 正常固定金额可计算，输出 `rewardAmount`、`rewardPayload.currency` 和成功原因。
2. `FIXED` 覆盖空配置、缺 `amount`、非法 Decimal、负数 `amount`、金额类缺 `currency`，均返回明确错误。
3. `RATIO` 正常比例可计算，`baseField * rate` 输出符合 Decimal 精度口径。
4. `RATIO` 超过 `maxAmount` 时按 `maxAmount` 封顶，payload 或 reason 可见 `capped=true`。
5. `RATIO` 覆盖缺 `baseField`、字段不存在、字段非数值、负数基础金额、非法 `rate`、负数 `rate`、非法 `maxAmount`、负数 `maxAmount`。
6. `LADDER` 覆盖首档、中间档、末档命中。
7. `LADDER` 覆盖边界值等于 `min` 和等于 `max` 的闭区间命中。
8. `LADDER` 覆盖未命中任何阶梯，返回 `LADDER_NOT_MATCHED`，不生成奖励结果。
9. `LADDER` 覆盖空 ladders、缺 min/max/amount、min > max、amount 非法、amount 负数、阶梯重叠。
10. `RANDOM` 覆盖固定区间、正常区间、默认 step、自定义 step、min=max、min>max、step=0、负数区间。
11. `RANDOM` 同一输入重复计算必须得到同一 rewardAmount；不同 seedSalt 或配置版本允许得到不同结果。
12. `PROBABILITY` 覆盖多奖项按权重命中、空奖 amount=0、重复 itemCode、空 items、weight 非法、amount 非法。
13. `PROBABILITY` 同一输入重复计算必须命中同一 item；payload 可解释 selectedIndex、itemCode、weight、totalWeight。
14. `POOL_SPLIT` 覆盖 COUNT 均分、WEIGHT 权重分摊、participantCount=0、weight=0、weight>totalWeight、poolAmount 非法。
15. 所有金额、比例、封顶、阶梯金额、随机区间、权重、奖池金额不得使用二进制浮点作为业务计算口径；验收应覆盖典型精度用例，如 `0.1 + 0.2` 类误差不应出现在输出中。
16. 输出金额最多 4 位小数，超出 4 位按四舍五入稳定输出。
17. 未知 `calculateType` 返回 `UNSUPPORTED_CALCULATE_TYPE`。
18. `enabled=false` 奖励不参与计算，不生成 grant_order/outbox，不扣 STOCK。
19. 0 金额结果返回 `ZERO_REWARD_SKIPPED` 或等价可解释状态，不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT。
20. 多奖励活动中任一 enabled reward 计算失败时，该 activity 不生成任何 grant_order/outbox，不扣任何 quota。
21. 多奖励活动中部分奖励为 0、部分奖励大于 0 时，只对大于 0 的实际发奖结果生成 grant_order/outbox 和 STOCK 扣减；BUDGET 扣减实际奖励合计；USER_LIMIT 因存在实际发奖扣减 1。
22. 计算失败时不生成 grant_order/outbox，不扣 BUDGET/STOCK/USER_LIMIT。
23. 计算成功但 quota 不足时，按现有语义返回 `409 QUOTA_EXCEEDED`，回滚 participation/grant_order/outbox，并写审计。
24. 模拟运行使用同一计算规则，输出预计奖励、skipped 或失败原因，且不写 event_inbox、participation、grant_order、outbox 或 quota。
25. 重复事件不重新计算、不重复发奖、不重复扣减 quota；`RANDOM / PROBABILITY` 在重复计算场景下也必须保持稳定结果。
26. API/数据模型文档同步说明六类 `calculateType`、六类 rewardConfig 示例、错误边界、精度口径、0 金额跳过规则和后三类的最小普通活动边界。
27. 后续 Dev/Test 阶段最低验证应包括 `go test ./...` 和 `go build ./...`；如触及 PostgreSQL 语义，还需要 Docker PostgreSQL 集成验证。

## 待确认问题

以下问题不阻断本任务进入 Review/Dev，因为本分析已给出可验收默认口径；但必须在后续 Reward Schema、玩法插件、后台审批或结算任务中固化：

1. `POINT` 类型是否必须为整数，是否允许小数点数。
2. 是否存在运营诉求要求 0 金额也记录 participation 或消耗 USER_LIMIT；本任务默认不消耗。
3. `RANDOM` 是否需要按运营指定概率分布而不是均匀分布；本任务默认候选值均匀选择。
4. `PROBABILITY` 是否需要 itemCode 级库存、实物奖、券码、GIFT 履约；本任务不包含。
5. `POOL_SPLIT` 是否需要活动结束后统一结算、余数补偿和整池只扣一次预算；本任务不包含。
6. 后续是否将 `COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 纳入普通活动 `rewardType`。
7. 模拟运行是否需要展示 disabled reward 的 skipped 明细，还是只展示 enabled reward。
8. 是否需要建立全局 reasonCode 中英文文案表和对外错误码兼容策略。

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

理由：Score >= 80。需求已按 `task.md` 覆盖 `FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT` 六类计算方式，并为后三类补充了最小普通活动口径、确定性幂等、预算估算、0 金额处理、稳定 reasonCode、边界条件和验收标准。复杂玩法、场次、奖池生命周期和审批能力被明确列为非目标范围，不影响本任务可审核、可测试、可验收。
