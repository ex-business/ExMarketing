# Analysis

## 需求 ID

2026-05-24-113929-11

## 标题

首存活动插件

## Review 反馈处理

- `review.md` 当前为模板内容，`P0/P1/P2` 未填写具体问题，`Open Questions` 为“待填写”，`Decision` 为占位 `PASS / BLOCK`，没有可执行的打回原因。
- 本轮按“无实质 Review 打回项”处理，但仍保留本小节，便于 Review Agent 复核。

## Analysis 自检修正

- 上一轮 `analysis.md` 仅为模板，现状、目标、方案、范围、风险、验收方式均为“待填写”；本轮已补齐为可审核、可验收的需求说明。
- 上一轮 `Score = 0`；扣分原因是需求没有细化、范围不可控、验收标准缺失、架构一致性未说明。本轮按评分规则重新评估。
- 上一轮 `Decision = PASS / BLOCK` 占位；本轮给出明确 `Decision PASS`。
- 旧 BLOCK 关闭依据：任务原文、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/roadmap.md` 以及 `22-独立活动营销中台设计.md` 已能支撑一个默认最小可验收口径：首存插件只做首存上下文标准化和预检查解释，不直接发奖、不扣预算、不绕过通用规则/奖励/quota/outbox。

## 背景目标

ExMarketing 是独立活动营销中台，外部业务系统只通过标准事件、Fact Query、Reward Command、Reward Callback 与系统交互。首存赠送属于低风险配置化活动，目标是在不破坏公共内核的前提下，为 `FIRST_DEPOSIT` 活动提供首存语义上下文，使运营可通过活动配置完成首存赠送。

本需求的核心目标：

- 支持 `FIRST_DEPOSIT` 活动类型绑定首存插件 `FIRST_DEPOSIT`。
- 插件只处理首存事件上下文和特殊字段归一化，供规则和奖励配置引用。
- 首存赠送仍通过活动规则、奖励配置、quota、participation、grant_order、outbox、callback 完成。
- 插件不得直接发钱、不得直接扣预算/库存/次数、不得绕过幂等、不得查询外部存款表。

## 默认最小可验收口径

现有材料存在两类表述：

- 任务要求“实现首存活动插件，只处理首存上下文和特殊字段”。
- 架构和路线图强调“普通满减、首存、续存不需要配置 pluginCode，也应通过数据配置完成”。

本轮采用兼容口径：

- 首存活动仍必须能完全通过普通活动配置完成。
- 本任务新增的 `FIRST_DEPOSIT` 插件是可选增强，用于把首存相关事件字段标准化暴露到 `state.plugin.FIRST_DEPOSIT.*`。
- 配置 `pluginCode=FIRST_DEPOSIT` 时，插件参与上下文构建和预检查解释；不配置插件时，现有 `attributes.*` 规则链路仍可表达首存活动。
- 插件不引入业务专用 controller、独立发奖逻辑、外部存款查询、插件状态表或新发奖通道。

## 参与方

- 运营后台/活动配置方：创建 `FIRST_DEPOSIT` 活动，配置规则、奖励、预算、库存、次数、入口展示和审批上线。
- 支付/充值系统：上报标准 `DEPOSIT_SUCCESS` 事件。
- ExMarketing 活动引擎：负责匹配活动、执行规则、计算奖励、quota 原子处理、生成 participation/grant_order/outbox。
- 首存插件：从标准事件 `attributes` 中构建首存上下文，提供规则可引用字段和预检查结果。
- 外部发奖系统：接收 Reward Command 并通过 callback 回传发奖结果。
- 审核/风控/财务：通过审批、审计、模拟运行、对账结果检查首存活动风险。

## 输入

### 标准事件

首存活动只接受标准事件链路输入，推荐触发事件：

```text
eventType = DEPOSIT_SUCCESS
sourceSystem = payment
biz.bizType = DEPOSIT
biz.bizId = 外部充值/支付单号
subject.type = USER
subject.id = 用户 ID
```

事件 `attributes` 的最小字段：

```text
amount            充值金额，Decimal 字符串或 JSON number，必须可解析为 Decimal 且 > 0
currency          币种，如 CNY、USD
isFirstDeposit    是否首存，boolean
```

事件 `attributes` 的可选字段：

```text
paymentMethod     支付方式，如 BANK_CARD、USDT、PIX
channel           入口渠道，用于 entry.channels
segments/segment  用户分群
riskLevel         风险等级
depositNo         外部充值单号，若存在必须与 biz.bizId 不冲突
```

### 活动配置

活动主配置默认：

```text
activityType = FIRST_DEPOSIT
triggerEvents 包含 DEPOSIT_SUCCESS
pluginCode 可选；配置时必须为 FIRST_DEPOSIT
timezone 必填
status 上线前必须完成审批
```

规则配置默认：

```text
QUALIFY:
  attributes.isFirstDeposit = true
  attributes.amount >= 最低首存金额
可选:
  attributes.currency = 指定币种
  state.plugin.FIRST_DEPOSIT.paymentMethod IN 允许支付方式
  attributes.riskLevel / attributes.segments 做人群过滤
```

奖励配置默认：

```text
rewardType = BONUS 或 CASH
calculateType = FIXED / RATIO / LADDER
RATIO 默认 baseField = attributes.amount
金额类奖励必须配置 currency
grantMode = AUTO
grantChannel = HTTP / WEBHOOK / MQ
rewardConfig.targetRef 必填
```

Quota 配置默认：

```text
USER_LIMIT: 每用户活动周期内 1 次
BUDGET: 可选活动总预算
STOCK: 可选奖励库存
DAILY_LIMIT: 可选每日用户次数；首存默认非必需
```

## 输出

### 规则上下文输出

当活动绑定 `pluginCode=FIRST_DEPOSIT` 时，插件需在规则上下文中提供：

```text
state.plugin.FIRST_DEPOSIT.amount
state.plugin.FIRST_DEPOSIT.currency
state.plugin.FIRST_DEPOSIT.isFirstDeposit
state.plugin.FIRST_DEPOSIT.paymentMethod
state.plugin.FIRST_DEPOSIT.depositBizId
```

字段来源只能来自标准事件，不允许插件主动查询外部存款表。

### 决策与记录输出

命中且通过规则、quota 的事件由核心流程输出：

```text
marketing_event_inbox
marketing_participation
marketing_grant_order
marketing_quota reservation/aggregate
marketing_outbox
marketing_audit
```

模拟运行输出应包含：

```text
plugin.pluginCode = FIRST_DEPOSIT
plugin.pluginContext
plugin.precheckResult
ruleResult
rewardPreviews
quotaImpactPreviews
grantOrderPreviews
outboxCommandPreviews
```

正式事件处理仍以核心流程状态为准，插件本身不产生发奖单、不投递 outbox。

## 业务规则

1. 首存定义以标准事件 `attributes.isFirstDeposit=true` 为准；ExMarketing 不自行判断“是否用户历史第一笔充值”。
2. `attributes.amount` 必须可解析为 Decimal 且大于 0；金额计算不得使用浮点语义。
3. `attributes.currency` 必须存在；金额类奖励币种应与活动配置币种一致，不一致时规则应拒绝或配置校验应阻止上线。
4. 外部事件幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`；重复事件不得重复参与、重复计数或重复发奖。
5. 首存用户次数限制由通用 `USER_LIMIT` quota 表达，默认每个 `subject.id` 对同一活动只能奖励 1 次。
6. 活动互斥、优先级、入口、展示、人群、审批、审计、预算、库存、发奖回调均沿用通用能力。
7. 插件可以返回预检查拒绝原因，例如缺少金额、缺少币种、非首存、事件类型不支持；这些原因只用于解释和规则链路决策，不替代通用规则。
8. 奖励为 0 时按现有奖励规则处理：金额类结果为 `0.0000` 且无其他权益时不生成 grant_order/outbox，不扣 quota。
9. 缺少 quota 行时按通用规则视为无限制；但首存活动上线验收建议至少配置 `USER_LIMIT=1`，否则存在同一用户多笔不同充值单重复奖励风险。
10. 配置 `pluginCode` 时必须与 `activityType=FIRST_DEPOSIT` 兼容；其他活动类型绑定 `FIRST_DEPOSIT` 插件应被拒绝。

## 边界条件

- 事件类型不是 `DEPOSIT_SUCCESS`：不应命中首存活动，或由活动 `triggerEvents` 排除。
- `biz.bizType` 不是 `DEPOSIT`：默认不作为首存充值事件处理。
- `attributes.isFirstDeposit=false`：拒绝，原因可解释为非首存。
- `attributes.isFirstDeposit` 缺失或非 boolean：拒绝，原因可解释为首存标识缺失/非法。
- `attributes.amount` 缺失、为 0、负数、不可解析 Decimal：拒绝，原因可解释为金额非法。
- `attributes.currency` 缺失：拒绝，原因可解释为币种缺失。
- 支付方式缺失：若活动未配置支付方式限制，可继续；若配置了支付方式限制则拒绝。
- 同一用户不同充值单均标记 `isFirstDeposit=true`：通用 `USER_LIMIT=1` 应阻止重复奖励。
- 同一充值单重复推送：幂等键命中重复，返回重复结果，不创建新的 participation/grant_order/outbox。
- 活动未审批、非 `ACTIVE`、不在活动时间内、入口 disabled、租户不匹配：按通用活动匹配规则拒绝。
- Quota 不足：正式事件应按通用 quota 规则拒绝，不保留脏 `REWARDABLE` participation。
- 发奖失败：由 outbox retry/DLQ、callback、timeout release 和对账处理，插件不介入补偿。

## 异常场景

- 事件 payload 使用非标准顶层字段如 `deposit.amount`、`orderAmount`：不作为首存插件输入；规则保存或运行应按标准字段边界拒绝。
- 外部系统误传 `isFirstDeposit=true`：ExMarketing 不主动纠正，默认依赖事件源可信；运营必须用 `USER_LIMIT`、审计和对账降低重复奖励风险。
- 首存金额和奖励币种不一致：默认拒绝或配置为不命中，不做自动汇率换算。
- 多租户使用同一 `activityCode`：允许，但事件、活动、quota、callback 必须 tenant 隔离。
- 配置了未知插件、未知字段路径、非法 evaluator、非法 reward/channel/targetRef：活动创建或上线前校验应拒绝。
- 模拟运行出现插件预检查失败：应展示失败原因，不写入 inbox、participation、grant_order、quota、outbox。

## 非目标范围

- 不实现钱包加钱、余额变更或任何直接发奖。
- 不连接支付、会员、钱包或风控数据库。
- 不通过插件查询外部存款历史来判断真实首存。
- 不新增首存专用 HTTP API、controller、发奖 worker 或业务表。
- 不新增插件状态表，不持久化插件私有玩法状态。
- 不实现人工导出发奖、人工发奖状态机。
- 不实现跨币种汇率换算。
- 不改变现有幂等键、quota、grant_order、outbox、callback、DLQ、审批流程。
- 不要求本需求新增后台可视化配置页面；可继续通过现有活动创建 API 配置和模拟。

## 风险

- 事件源误标 `isFirstDeposit` 会导致资格误判；默认通过 `USER_LIMIT=1`、幂等和审计降低重复发奖，但不能替代支付系统事实判断。
- 若运营未配置 `USER_LIMIT`，同一用户多个不同 `bizId` 且都标记首存时可能多次奖励；验收应覆盖推荐 quota 配置。
- 插件字段与 `attributes.*` 字段并存可能造成配置歧义；默认要求插件字段只作为标准化镜像和解释增强，首存最低规则仍可使用 `attributes.*`。
- 如果将首存插件扩展为外部查询或发奖，会破坏 ExMarketing 解耦和账务安全边界；本需求明确禁止。
- 资金类首存奖励涉及预算冻结/确认/释放，对正式事件验收必须覆盖 callback 成功/失败或至少验证 grant_order/outbox 生成链路。

## 验收标准

### 配置验收

- 可以创建 `activityType=FIRST_DEPOSIT`、`pluginCode=FIRST_DEPOSIT`、`triggerEvents=["DEPOSIT_SUCCESS"]` 的活动。
- 该活动可配置首存规则：`attributes.isFirstDeposit=true`、`attributes.amount >= 100.00`。
- 该活动可配置 `RATIO` 奖励：按 `attributes.amount * rate` 计算，并受 `maxAmount` 封顶。
- 该活动可配置 `USER_LIMIT=1`、可选 `BUDGET` 和 `STOCK`。
- 未知插件、非 `FIRST_DEPOSIT` 活动绑定 `FIRST_DEPOSIT` 插件、非法字段路径、非法金额或非法发奖目标应被拒绝。

### 模拟验收

- 输入首存事件 `amount=299.00`、`currency=CNY`、`isFirstDeposit=true` 时，模拟运行应命中活动，展示插件上下文、规则通过、奖励预览、quota 影响预览、grant/outbox 预览。
- 输入 `isFirstDeposit=false` 时，模拟运行应拒绝并展示可识别原因，不展示可执行 grant/outbox。
- 输入缺少 `amount`、缺少 `currency`、金额非法、首存标识缺失时，模拟运行应拒绝并展示可识别原因。
- 模拟运行不得写入 inbox、participation、grant_order、outbox 或 quota。

### 正式事件验收

- 首次合法 `DEPOSIT_SUCCESS` 事件应走通核心链路：inbox -> participation -> quota reservation -> grant_order -> outbox。
- 重复提交相同幂等键和相同 payload 应返回 duplicate，不重复创建 participation、grant_order、outbox，不重复扣 quota。
- 同一用户第二个不同 `bizId` 且仍为首存事件时，在配置 `USER_LIMIT=1` 后应被 quota/次数限制拒绝。
- 插件不得直接修改余额，不得直接创建外部发奖结果；实际发奖仍由 outbox command 和 callback 完成。
- quota 不足时应按通用规则拒绝，不保留脏奖励记录。

### 架构验收

- 首存插件只暴露 `state.plugin.FIRST_DEPOSIT.*` 上下文和预检查结果。
- 通用规则、奖励计算、预算库存、参与记录、grant_order、outbox、callback、审计、对账流程保持统一。
- 不新增业务专用 controller，不新增支付/钱包数据库依赖，不新增首存专用发奖路径。
- 普通不绑定插件的首存配置仍可通过 `attributes.*` 完成，不因插件新增而失效。

## 待确认问题

以下问题不阻塞本轮进入研发，已给出默认口径：

- `isFirstDeposit` 的权威来源是谁？默认由支付/充值系统在事件 `attributes.isFirstDeposit` 中提供，ExMarketing 不查询外部历史。
- 插件是否必配？默认可选；任务验收覆盖 `pluginCode=FIRST_DEPOSIT`，但不破坏无插件配置化首存。
- 首存支付方式是否必填？默认非必填，仅在活动配置了支付方式限制时才要求匹配。
- 首存默认奖励币种如何处理？默认金额类奖励配置币种必须与事件币种一致，不做汇率换算。
- 是否强制创建 `USER_LIMIT=1`？默认创建活动不强制，但验收和上线建议必须配置；若未来要强制，应单独提出上线校验需求。

## 评分

```text
需求清晰度        18/20
范围可控          19/20
风险可控          17/20
验收标准明确      19/20
架构一致性        19/20
Total             92/100
```

扣分说明：

- 事件源是否真实权威判断首存仍依赖外部系统，ExMarketing 只能通过标准事件与 quota 降低风险。
- 是否强制 `USER_LIMIT=1` 当前按上线建议处理，未作为创建活动硬性阻断。

## Decision

```text
PASS
```
