# Analysis

## 需求

`2026-05-24-reward`：补齐 REWARD 奖励条件规则的需求定义。REWARD 阶段用于判断订单金额、充值金额、投注金额、签到天数、邀请人数等业务行为条件是否满足奖励资格。每条奖励条件必须输出可解释的通过/失败原因；条件通过后只进入奖励计算链路，不允许在 REWARD 规则判断阶段直接发奖。

## Review 反馈处理

- `review.md` 当前为模板，未填写具体 P0/P1/P2，也未给出真实打回原因；但模板中存在 `Open Questions` 和 `PASS / BLOCK` 占位。本轮按返工处理：补齐可审核的业务口径、边界条件、验收标准和待确认问题，避免下游 Agent 以模板空洞内容进入开发。
- 未发现 Review Agent 已提出的逐条问题，因此无逐项业务争议需要回应。若后续 Review 补充具体 P0/P1，应以本文件的“待确认问题”和“验收标准”为基础继续收敛。

## Analysis 自检修正

- 上一轮 `analysis.md` 为模板，`Score=0`，`Decision=PASS / BLOCK` 未决。本轮已补齐需求背景、参与方、输入输出、业务规则、边界条件、异常场景、非目标、风险和验收标准。
- 上一轮“现状/目标理解/方案/改动范围/风险点/验收方式”均为待填写。本轮不使用代码实现细节替代需求澄清，而是基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/roadmap.md` 和 `act-analysis/22-独立活动营销中台设计.md` 给出默认最小可验收口径。
- 上一轮没有明确 REWARD 阶段与 QUALIFY/RISK/LIMIT/BUDGET 的关系。本轮明确：运行顺序为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`；REWARD 只判断奖励条件，不扣 quota、不生成 grant_order、不写 outbox、不调用外部发奖。
- 上一轮没有明确数据来源。本轮明确：优先使用标准事件 `attributes.*`；活动自身状态使用 `state.*`；无法由事件携带的数据才通过已注册 Fact Provider 暴露为 `facts.<factCode>.*`。
- 上一轮没有明确验收口径。本轮补充通过/失败、字段缺失、数值比较、模拟运行、事件处理、幂等、无副作用、quota 不足和零奖励等验收场景。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动配置、生成参与记录、发奖单和发奖指令。系统不得连接外部业务数据库，不得引用外部业务代码包，不得直接修改用户余额。

当前活动规则已有业务阶段概念：`MATCH` 判断活动匹配，`QUALIFY` 判断参与资格，`REWARD` 判断奖励条件，`RISK/LIMIT` 判断风控和限制，`BUDGET` 作为 quota 类型参与额度控制。此需求聚焦 `REWARD` 阶段，让充值、订单、投注、签到、邀请等行为条件能通过通用 `CONDITION` 规则表达，并在运行结果中给出清晰解释。

默认最小可验收口径：本需求不新增业务专用活动类型，不新增专用 controller，不新增直接发奖逻辑；只要求普通配置化活动可以配置并执行 `rulePhase=REWARD`、`ruleType=CONDITION` 的奖励条件，并在事件处理和模拟运行中产出阶段级、规则级、条件级解释。

## 参与方

- 运营/后台配置方：配置活动、触发事件、REWARD 条件和奖励配置。
- 外部业务系统：上报标准事件，提供 `attributes` 或可选 Fact Provider 数据。
- ExMarketing 规则引擎：按阶段执行规则并输出解释结果。
- ExMarketing 奖励计算引擎：仅在 REWARD 条件通过后计算奖励。
- ExMarketing 编排链路：在规则与奖励计算通过后处理 participation、quota、grant_order、outbox。
- 外部奖励系统：只接收 outbox worker 发送的标准发奖指令，不由 REWARD 阶段直接调用。

## 输入

### 活动规则配置

REWARD 条件规则使用既有规则模型：

```text
rulePhase = REWARD
ruleType  = CONDITION
enabled   = true / false
sort      = 同阶段内排序
```

`ruleConfig` 最小结构：

```json
{
  "logic": "AND",
  "conditions": [
    {"field": "attributes.amount", "op": ">=", "value": "100.00"}
  ]
}
```

可表达的业务条件默认包括：

- 订单金额：如 `attributes.orderAmount`、`attributes.amount`。
- 充值金额：如 `attributes.depositAmount`、`attributes.amount`。
- 投注金额：如 `attributes.betAmount`、`attributes.validBetAmount`。
- 签到天数：如 `attributes.signDays`，或已暴露的 `state.plugin.<pluginCode>.signDays` / `facts.<factCode>.signDays`。
- 邀请人数：如 `attributes.inviteCount`，或已暴露的 `facts.<factCode>.inviteCount`。

字段命名不由本需求强制统一为某个业务短名；只要路径符合平台允许的标准字段路径即可。非标准顶层字段如 `orderAmount`、`deposit.amount`、`user.vipLevel` 必须拒绝。

### 标准事件

事件输入仍只接受标准事件字段：

```text
eventId, eventType, sourceSystem, tenantId, subject, biz, occurredAt, attributes
```

外部事件幂等键固定为：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

能随事件携带的数据应优先放入 `attributes`，不要为了金额、天数、人数等事件已知信息强制实时 Fact Query。

### 可选事实数据

当签到天数、邀请人数、历史投注累计等无法由当前事件携带时，可使用已注册且 enabled 的 Fact Provider，将结果暴露为：

```text
facts.<factCode>.<path>
```

未注册或未启用的事实源不得被活动规则引用。

## 输出

### 规则解释输出

REWARD 阶段必须在 `ruleResult` 中输出：

- 阶段名称：`REWARD`。
- 每条 REWARD 规则的 `ruleCode`、执行状态、`passed`、`reasonCode`、`reason`、`sort`。
- 每个条件的 `field`、`op`、`expected`、`actual`、通过/失败状态和失败原因。
- disabled 规则应显示为 `SKIPPED`，不影响通过结论。

默认最小 reasonCode 口径：

```text
REWARD_CONDITION_PASSED
REWARD_CONDITION_FAILED
REWARD_FIELD_MISSING
REWARD_TYPE_MISMATCH
REWARD_INVALID_CONFIG
REWARD_RULE_SKIPPED
```

如现有规则引擎已有通用 reasonCode，可复用通用码，但必须能区分通过、失败、字段缺失、类型不匹配和跳过。

### 业务处理输出

- REWARD 阶段失败：该活动本次事件不进入奖励计算；应写入或预览 `REJECTED` participation，`reject_reason` 体现 REWARD 条件失败原因；不生成 grant_order/outbox，不扣 quota。
- REWARD 阶段通过：只表示允许进入奖励计算；后续是否生成发奖单还要取决于奖励计算结果、quota、库存、用户次数、幂等和 outbox 编排。
- 奖励计算结果全为金额 `0.0000`：应写 `QUALIFIED` participation，并记录 `ZERO_REWARD_SKIPPED: no payable reward`；不写 `REWARDABLE`，不生成 grant_order/outbox，不扣 quota。
- 通过且存在有效奖励：按既有链路进入 quota、grant_order、outbox。REWARD 阶段本身不得直接发奖。

### 模拟运行输出

`POST /admin/v1/simulate` 必须展示 REWARD 阶段的规则解释、奖励预览、quota 影响预览、participation 预览、grantOrder/outbox 预览。模拟运行不得写入 inbox、participation、grant_order、outbox 或 quota。

## 业务规则

1. 执行顺序固定为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
2. REWARD 规则只用于判断“是否满足奖励条件”，不承担奖励金额计算。金额计算仍由 reward calculator 根据 reward 配置处理。
3. 同一 REWARD 阶段内按 `sort` 顺序执行。默认最小口径为：所有 enabled REWARD 规则均通过，REWARD 阶段才通过；任一 enabled REWARD 规则失败，阶段失败。
4. 单条 `CONDITION` 规则内部按 `logic=AND/OR` 汇总条件结果。`AND` 要求全部条件通过；`OR` 要求至少一个条件通过。
5. 条件操作符范围沿用现有规则：`=`、`==`、`!=`、`<>`、`>`、`>=`、`<`、`<=`、`IN`、`NOT_IN`。
6. 数值比较必须使用 Decimal 语义。金额、投注流水、人数、天数等比较不得使用浮点语义。
7. `>`、`>=`、`<`、`<=` 两侧必须可解析为 Decimal；不可解析时条件失败并输出类型/格式原因，不得默认通过。
8. `IN`、`NOT_IN` 的 `value` 必须是非空数组；事件实际值与数组值按类型语义比较。
9. 字段缺失不默认通过。缺失字段应输出实际值为空/缺失，并给出字段缺失原因。
10. `enabled=false` 的 REWARD 规则运行时跳过，但保存配置时仍必须满足 Schema 校验，不得用 disabled 绕过非法配置。
11. 缺少 REWARD 规则的活动默认视为无额外奖励条件，不能阻断既有活动；但若配置了 REWARD 规则，必须严格按规则结果决定是否进入奖励计算。
12. `MATCH` 或 `QUALIFY` 失败时，不应继续产生可执行的 REWARD 奖励计算结果；可选输出后续阶段未执行/跳过说明。
13. `RISK/LIMIT/quota` 不属于本需求的 REWARD 条件。预算、库存、用户次数不足应在后续阶段返回 quota/limit 风险，不应伪装成 REWARD 条件失败。
14. 重复事件仍遵循事件幂等：完全重复返回 duplicate，不重复创建 participation、grant_order 或 outbox。
15. 多租户隔离不变：规则只能作用于当前活动所属 tenant 的事件和 Fact Provider。

## 边界条件

- 时间边界：活动时间、状态、入口、人群等仍由 MATCH/QUALIFY 处理，不纳入 REWARD 本需求。
- 货币边界：REWARD 条件可以比较金额字段，但不校验汇率或跨币种换算；币种一致性由活动配置或上游事件保证。
- 累计口径：若需求方未提供累计窗口，默认 REWARD 只比较本次事件或已显式传入/查询到的事实值，不在本需求内新增周期累计状态。
- 签到/邀请口径：默认使用事件 attributes 或 Fact Provider 已提供的确定值，不在本需求内新增签到流水表、邀请关系表或状态聚合任务。
- 多奖励配置：一组 REWARD 条件通过后，活动下 enabled rewards 均可进入奖励计算；某个 reward 是否产出发奖单由该 reward 计算结果和 quota 决定。
- 非金额权益：POINT、COUPON、GIFT、DRAW_CHANCE、MEMBERSHIP 的发奖资格同样受 REWARD 条件控制；其数量/权益字段由 reward 配置负责。
- 插件活动：插件只可提供 `state.plugin.<pluginCode>.*` 上下文，不得绕过 REWARD、reward calculation、quota、grant_order 或 outbox。
- 兼容性：既有没有 REWARD 规则的活动不能因本需求改变行为。

## 异常场景

- REWARD 配置字段路径非法：创建/上线前校验应拒绝，不能保存为可执行规则。
- REWARD 配置操作符非法、`conditions` 为空、`logic` 非法：应拒绝配置或在模拟/运行中返回配置错误，不得静默通过。
- 事件缺少被引用字段：该条件失败，并输出字段缺失原因。
- 数值字段不可解析：该条件失败，并输出类型/格式原因。
- Fact Provider 未注册、禁用或查询失败：依赖该事实的条件不得默认通过；应以事实不可用/字段缺失/查询失败原因解释。
- REWARD 条件失败：不进入奖励计算，不生成发奖单，不写 outbox，不扣 quota。
- REWARD 条件通过但 reward 配置无 enabled reward：应不生成发奖单；具体 participation 状态按既有“无实际奖励/零奖励”口径处理。
- REWARD 条件通过但 quota 不足：后续 quota 阶段返回 `QUOTA_EXCEEDED`，回滚 participation/grant_order/outbox，并记录审计。
- 重复事件：不重复执行产生副作用。
- 模拟运行：即使 REWARD 条件通过，也只能展示 preview，不产生任何写入或外部发奖。

## 非目标范围

- 不新增外部业务数据库连接。
- 不新增订单、充值、投注、签到、邀请的业务专用表或聚合任务。
- 不设计具体代码改法，不指定包、函数或实现细节。
- 不新增新的 rule type，如 FORMULA、LIMIT、SCRIPT 的执行能力。
- 不新增直接发奖、钱包加钱、会员升级、优惠券核销等外部业务动作。
- 不新增 MQ 发奖、人工发奖、导出发奖能力。
- 不改变 reward calculator 的奖励金额/权益计算规则。
- 不改变 quota、budget、stock、user limit 的扣减语义。
- 不改变 OpenAPI 事件幂等、安全和租户隔离边界。

## 风险

- 数据口径风险：订单金额、充值金额、投注金额等字段名由外部事件决定，配置方可能使用非标准路径。缓解：只允许标准路径，验收覆盖非法字段拒绝。
- 解释不足风险：只输出阶段失败会导致运营无法判断哪条条件失败。缓解：验收要求规则级和条件级 `actual/expected/reason`。
- 副作用风险：REWARD 阶段若直接发奖会破坏账务安全。缓解：明确 REWARD 只进入奖励计算，发奖必须经 grant_order/outbox。
- 精度风险：金额与流水比较若使用浮点会出现边界误判。缓解：需求要求 Decimal 语义。
- 兼容性风险：已有无 REWARD 规则活动可能被误拦截。缓解：无 REWARD 规则默认通过该阶段。
- Fact 可用性风险：依赖外部事实查询会带来超时或不可用。缓解：优先 attributes；事实不可用不得默认通过。

## 验收标准

### 配置验收

- 可以创建包含 `rulePhase=REWARD`、`ruleType=CONDITION` 的活动规则。
- REWARD 条件支持金额、天数、人数等 Decimal 可比较字段。
- 非标准顶层字段、非法操作符、空条件、非法 `logic`、不可解析 Decimal 配置会被拒绝或明确报错。
- `enabled=false` 的 REWARD 规则运行时显示跳过，不影响阶段通过，但不能绕过配置校验。

### 事件处理验收

- 当事件 `attributes.amount=299.00` 且 REWARD 条件为 `attributes.amount >= 100.00` 时，REWARD 阶段通过，并进入奖励计算。
- 当事件金额低于阈值时，REWARD 阶段失败，输出失败规则、失败字段、期望值和实际值；不生成 grant_order/outbox，不扣 quota。
- 当字段缺失或类型错误时，REWARD 阶段失败并输出字段缺失或类型不匹配原因。
- 当同一活动配置多条 enabled REWARD 规则时，任一规则失败则 REWARD 阶段失败；全部通过才进入奖励计算。
- 当活动没有 REWARD 规则时，既有活动仍可按原链路执行。
- 重复上报同一事件时，不重复创建 participation、grant_order 或 outbox。

### 链路副作用验收

- REWARD 条件通过后只进入奖励计算，不直接调用外部发奖系统。
- 只有后续奖励计算产生有效奖励且 quota/limit 允许时，才可写 `REWARDABLE` participation、grant_order 和 outbox。
- 奖励金额为 `0.0000` 时，不生成 grant_order/outbox，不扣 quota，并按零奖励口径记录。
- quota 不足时返回 quota 相关错误并回滚发奖副作用，不归因于 REWARD 条件失败。

### 模拟运行验收

- `POST /admin/v1/simulate` 输出 REWARD 阶段的规则级和条件级解释。
- 模拟运行中 REWARD 失败时，不展示可执行 grant order/outbox command 预览。
- 模拟运行无副作用，不写 inbox、participation、grant_order、outbox 或 quota。

### 文档和回归验收

- 需求实现后的文档应说明 REWARD 阶段用途、字段路径、失败解释和“不直接发奖”边界。
- 基础验证至少覆盖 `go test ./...` 和 `go build ./...`。
- 涉及 PostgreSQL 行为时，应使用 PostgreSQL 仓储验证事件处理、quota 回滚和幂等。

## 待确认问题

当前无阻塞型待确认问题。以下为非阻塞配置建议，不影响按默认最小口径进入研发：

- 字段命名是否需要运营后台提供模板别名，例如“订单金额”映射到 `attributes.orderAmount`。默认不新增别名，要求配置方使用标准字段路径。
- 累计投注金额、连续签到天数、累计邀请人数是否由上游事件携带还是 Fact Provider 提供。默认本需求不新增聚合状态，只消费已提供的 `attributes` / `facts` / `state.plugin` 值。
- REWARD reasonCode 是否完全采用新增 `REWARD_*` 码还是复用通用规则码。默认允许复用，但必须满足可区分和可审核要求。

## Score

```text
88
```

评分明细：

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        17/20
```

扣分原因：具体业务字段命名、累计窗口和 reasonCode 命名仍需在配置/实现阶段保持一致；但这些已给出默认最小可验收口径，不构成 BLOCK。

## Decision

```text
PASS
```

需求：2026-05-24-reward
