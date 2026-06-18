# 资格校验节点说明

`qualify`，中文名“资格校验”，是活动流程里的准入判断节点。

它回答一个核心问题：

> 当前事件里的这个用户，是否有资格继续进入奖励、预算、参与记录和发奖链路？

## 这个节点做什么

`qualify` 不负责查数据，也不负责扣次数、锁预算、发奖。它只基于上游已经准备好的数据做规则判断，并输出可路由的结果。

典型判断包括：

- 用户是否存在、状态是否正常。
- 用户等级、VIP、标签、国家、渠道是否满足活动门槛。
- 当前事件金额、任务状态、签到状态、邀请关系是否符合规则。
- 风控结果是否需要阻断或进入人工审核。
- 上游次数检查、活动状态检查、事实查询结果是否允许继续。

它通常输出三类结果：

| 输出 | 含义 | 后续流向 |
| --- | --- | --- |
| `PASS` | 资格通过 | 继续到奖励计算、预算锁定、参与记录、发奖 |
| `BLOCK` | 资格不通过 | 终止当前活动链路，可进入审计或拒绝记录 |
| `RISK` | 有风险或需要复核 | 进入风控、审批、人工处理或审计 |

## 数据从哪里来

`qualify` 的数据不是自己查询出来的，而是从上游节点和活动配置里读取。

常见来源如下：

| 数据来源 | 来自哪个节点/配置 | 举例 |
| --- | --- | --- |
| 标准事件 | `event-in` / `inbox` / `normalize` | `eventType`、`tenantId`、`subject.id`、`attributes.amount` |
| 活动匹配结果 | `match` | 当前命中的活动、活动规则、活动状态 |
| 外部事实 | `fact-query` | 用户资料、VIP、历史充值、邀请关系、风控标签 |
| 活动状态 | `activity-state` | 签到天数、任务进度、活动内用户状态 |
| 次数检查结果 | `quota-check` / `limit` | 今日是否已参加、周期剩余次数 |
| 节点配置 | `qualify` 自身配置 | 规则列表、判断模式、失败策略 |
| Workflow 上下文 | 上游节点输出 | `ctx.nodes.*`、`facts.*`、`state.*` |

关键原则：

- 如果要查用户资料、VIP、历史订单、风控结果，放在 `fact-query`。
- 如果要做次数、频率、周期计数，放在 `quota-check` 或 `limit`。
- 如果只是基于已有数据判断“能不能继续”，放在 `qualify`。

## 推荐链路位置

常见活动链路：

```text
event-in
-> inbox
-> normalize
-> match
-> fact-query
-> quota-check
-> qualify
-> reward / candidate-reward
-> budget-reserve / quota
-> participation
-> grant-order
-> outbox
```

`qualify` 一般放在事实查询和次数检查之后、奖励计算之前。

原因是：资格没通过时，不应该继续生成奖励、锁预算或发奖。

## 输入示例

```json
{
  "event_context": {
    "eventType": "DEPOSIT_SUCCESS",
    "tenantId": "site_001",
    "subject": {
      "type": "USER",
      "id": "u_10001"
    },
    "attributes": {
      "amount": "100.00",
      "currency": "USD",
      "channel": "APP"
    }
  },
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "status": "ACTIVE",
      "vipLevel": 2,
      "tags": ["normal_user"]
    }
  },
  "quota_check": {
    "decision": "PASS",
    "remaining": 1
  }
}
```

## 规则示例

首存赠送活动可能需要这些资格规则：

```json
{
  "logic": "AND",
  "rules": [
    {
      "code": "USER_EXISTS",
      "field": "facts.USER_PROFILE.exists",
      "op": "=",
      "value": true
    },
    {
      "code": "USER_ACTIVE",
      "field": "facts.USER_PROFILE.status",
      "op": "=",
      "value": "ACTIVE"
    },
    {
      "code": "MIN_AMOUNT",
      "field": "event_context.attributes.amount",
      "op": ">=",
      "value": "100.00"
    },
    {
      "code": "QUOTA_PASS",
      "field": "quota_check.decision",
      "op": "=",
      "value": "PASS"
    }
  ]
}
```

## 规则编码是什么

`qualifyRules[].ruleCode` 是资格校验节点要执行的规则编码。

它不是页面展示名称，而是执行器用来识别“要跑哪类资格规则”的稳定 code。每个 code 代表一类检查逻辑，运行时会从事件、facts、quota_check 或 activity_state 里读取对应字段，然后输出通过、阻断或风险结果。属性面板里每一张规则卡片都有自己的 `ruleCode`，卡片上下顺序就是执行顺序。

当前 `qualify` 节点 schema 支持这些规则编码：

| 规则编码 | 作用 | 常见数据来源 |
| --- | --- | --- |
| `ELIGIBILITY` | 通用参与资格检查 | `matched_activity`、`event_context`、活动配置 |
| `KYC` | KYC / 实名 / 合规状态检查 | `facts.KYC_PROFILE.status`、`facts.KYC_PROFILE.verified` |
| `RISK_SCORE` | 风险评分或风险标签检查 | `facts.RISK_PROFILE.score`、`facts.RISK_PROFILE.blacklisted`、风控事实 |
| `VIP_LEVEL` | VIP / 用户等级门槛检查 | `facts.USER_PROFILE.vipLevel`、`facts.USER_PROFILE.level` |
| `DEPOSIT_REQUIRED` | 存款、首存、累计充值等门槛检查 | `event_context.attributes.amount`、`facts.DEPOSIT_PROFILE.*` |
| `COUNTRY_ALLOWED` | 国家、地区、 jurisdiction 白名单/黑名单检查 | `facts.USER_PROFILE.country`、`event_context.attributes.country` |
| `CHANNEL_ALLOWED` | 渠道、来源系统、入口限制检查 | `event_context.attributes.channel`、`event_context.sourceSystem` |
| `REGISTER_AGE` | 注册天数、账号年龄检查 | `facts.USER_PROFILE.registeredAt`、`facts.USER_PROFILE.registerAgeDays` |

KYC 规则推荐由独立事实查询节点先查询 `KYC_PROFILE`：

```json
{
  "factKeys": ["KYC_PROFILE"],
  "providerChain": [
    {
      "source": "FACT_SERVICE",
      "resourceRef": "default_kyc_profile_store",
      "table": "marketing_kyc_profile",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "onMiss": "BLOCK"
    }
  ]
}
```

风控规则推荐由独立事实查询节点先查询 `RISK_PROFILE`：

```json
{
  "factKeys": ["RISK_PROFILE"],
  "providerChain": [
    {
      "source": "FACT_SERVICE",
      "resourceRef": "default_risk_profile_store",
      "table": "marketing_risk_profile",
      "keyTemplate": "${tenantId}:${subject.type}:${subject.id}",
      "onMiss": "BLOCK"
    }
  ]
}
```

### ELIGIBILITY 是什么

`ELIGIBILITY` 是“通用参与资格”的基础规则编码。

它不是某一条特别具体的业务规则，比如 VIP、KYC、充值金额；它更像是活动参与前的基础门禁，用来判断当前用户和当前事件是否具备进入活动链路的基本资格。

通常检查这些内容：

- 当前事件是否已经匹配到有效活动。
- 当前活动是否允许这个租户、用户主体、渠道或来源系统参与。
- `subject.id`、`subject.type` 等基础主体字段是否有效。
- 活动状态、时间窗、活动范围是否允许继续。
- 用户是否被活动范围排除，例如黑名单、排除标签、非目标人群。
- 上游节点是否已经给出不可继续的状态。

可以简单理解为：

```text
ELIGIBILITY = 这个用户 / 事件有没有资格进入当前活动的基础判断
```

它和其它规则的区别：

| 规则编码 | 关注点 |
| --- | --- |
| `ELIGIBILITY` | 基础参与资格、活动范围、事件和主体是否合法 |
| `VIP_LEVEL` | VIP 或用户等级是否达到门槛 |
| `KYC` | 实名、合规、认证状态是否满足要求 |
| `DEPOSIT_REQUIRED` | 充值、首存、累计存款等金额条件 |
| `RISK_SCORE` | 风险评分、风险标签、黑名单策略 |

示例：

```json
{
  "qualifyRules": [
    {
      "ruleCode": "ELIGIBILITY",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "eventType", "op": "=", "value": "DEPOSIT_SUCCESS" },
        { "field": "subject.id", "op": "!=", "value": "" }
      ]
    },
    {
      "ruleCode": "VIP_LEVEL",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.vipLevel", "op": ">=", "value": 1 }
      ]
    }
  ],
  "evaluationMode": "ALL_PASS"
}
```

含义：

- 先检查基础参与资格。
- 再检查 VIP 等级。
- 两个都通过才输出 `PASS`。

默认配置一般是：

```json
{
  "qualifyRules": [
    {
      "ruleCode": "ELIGIBILITY",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "eventType", "op": "=", "value": "DEPOSIT_SUCCESS" },
        { "field": "subject.id", "op": "!=", "value": "" },
        { "field": "facts.USER_PROFILE.exists", "op": "=", "value": true },
        { "field": "facts.USER_PROFILE.status", "op": "=", "value": "ACTIVE" }
      ]
    }
  ],
  "evaluationMode": "ALL_PASS",
  "stopOnFirstBlock": true,
  "riskPolicy": "BLOCK",
  "blockAction": "BLOCK"
}
```

### 属性面板怎么配置

在右侧属性面板选中“资格校验”节点后，进入“配置”页。

规则集合建议这样填：

| 字段 | 推荐值 | 说明 |
| --- | --- | --- |
| `规则编码` | `ELIGIBILITY` | 选择要执行的资格规则编码 |
| `执行模式` | `ALL_PASS` | 所有规则通过才通过 |
| `阻断即停止` | 开启 | 第一条阻断后不继续执行后续资格规则 |

风险处理建议这样填：

| 字段 | 推荐值 | 说明 |
| --- | --- | --- |
| `风控策略` | `BLOCK` | 风控命中时阻断 |
| `资格失败动作` | `BLOCK` | 资格失败走 `BLOCK` 分支 |
| `原因输出 Path` | `qualification.reason` | 将失败原因写入上下文 |

属性面板里的“规则详情预览”用于解释当前选中的规则编码：

- `ELIGIBILITY` 是什么。
- 建议读取哪些字段。
- 默认应该配置哪些条件。

属性面板里的“规则链”是资格校验的规则编辑器。它和事实查询节点的数据提供者链类似：可以添加多条规则、上下调整顺序、单独启停，并给每条规则配置自己的条件。这里保存到资格校验节点的 `qualifyRules` 配置中，执行器会按顺序转换成 `rulePhase=QUALIFY`、`ruleType=CONDITION` 规则并执行。

推荐默认条件：

```json
{
  "qualifyRules": [
    {
      "ruleCode": "ELIGIBILITY",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "eventType", "op": "=", "value": "DEPOSIT_SUCCESS" },
        { "field": "subject.id", "op": "!=", "value": "" },
        { "field": "facts.USER_PROFILE.exists", "op": "=", "value": true },
        { "field": "facts.USER_PROFILE.status", "op": "=", "value": "ACTIVE" }
      ]
    },
    {
      "ruleCode": "VIP_LEVEL",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.vipLevel", "op": ">=", "value": 1 }
      ]
    }
  ]
}
```

规则链执行顺序就是属性面板里的排列顺序。比如先放 `ELIGIBILITY`，再放 `VIP_LEVEL`，就是先校验通用参与资格，再校验 VIP 门槛。规则编码相同的节点内配置会覆盖活动 Rules 中的同名规则。

### 多个规则怎么组合

`evaluationMode` 决定多条 `qualifyRules` 的组合方式：

| 模式 | 含义 | 适用场景 |
| --- | --- | --- |
| `ALL_PASS` | 所有规则都通过才输出 `PASS` | 大多数生产活动 |
| `ANY_PASS` | 任一规则通过即可输出 `PASS` | 多条件任选其一，例如满足首存或满足累计充值 |
| `FIRST_DECISION` | 按顺序执行，第一条明确决策生效 | 黑名单、风控优先的链路 |

### 例子：VIP 活动

```json
{
  "qualifyRules": [
    {
      "ruleCode": "ELIGIBILITY",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "eventType", "op": "=", "value": "DEPOSIT_SUCCESS" },
        { "field": "subject.id", "op": "!=", "value": "" }
      ]
    },
    {
      "ruleCode": "VIP_LEVEL",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.vipLevel", "op": ">=", "value": 1 }
      ]
    },
    {
      "ruleCode": "COUNTRY_ALLOWED",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.country", "op": "IN", "value": ["BR", "MX"] }
      ]
    }
  ],
  "evaluationMode": "ALL_PASS",
  "stopOnFirstBlock": true,
  "blockAction": "BLOCK"
}
```

含义：

- 先检查通用参与资格。
- 再检查 VIP 等级是否满足。
- 再检查国家/地区是否允许。
- 任一失败就输出 `BLOCK`。

### 例子：风控优先

```json
{
  "qualifyRules": [
    {
      "ruleCode": "RISK_SCORE",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.RISK_PROFILE.score", "op": "<", "value": 80 }
      ]
    },
    {
      "ruleCode": "KYC",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.kycStatus", "op": "=", "value": "VERIFIED" }
      ]
    },
    {
      "ruleCode": "VIP_LEVEL",
      "enabled": true,
      "logic": "AND",
      "conditions": [
        { "field": "facts.USER_PROFILE.vipLevel", "op": ">=", "value": 1 }
      ]
    }
  ],
  "evaluationMode": "FIRST_DECISION",
  "riskPolicy": "RISK",
  "blockAction": "BLOCK"
}
```

含义：

- 风险命中时输出 `RISK`。
- KYC 不通过时输出 `BLOCK`。
- VIP 等级满足后才允许继续。

## 输出示例

通过：

```json
{
  "decision": "PASS",
  "passed": true,
  "reasonCode": "QUALIFIED",
  "reason": "all qualification rules passed",
  "ruleResults": [
    { "code": "USER_EXISTS", "passed": true },
    { "code": "USER_ACTIVE", "passed": true },
    { "code": "MIN_AMOUNT", "passed": true },
    { "code": "QUOTA_PASS", "passed": true }
  ]
}
```

不通过：

```json
{
  "decision": "BLOCK",
  "passed": false,
  "reasonCode": "MIN_AMOUNT_NOT_MET",
  "reason": "event_context.attributes.amount is below 100.00",
  "failedRule": "MIN_AMOUNT"
}
```

风险：

```json
{
  "decision": "RISK",
  "passed": false,
  "reasonCode": "RISK_TAG_HIT",
  "reason": "user risk tag matched manual review policy",
  "failedRule": "RISK_TAG_CHECK"
}
```

## 和相邻节点的区别

| 节点 | 做什么 | 不做什么 |
| --- | --- | --- |
| `fact-query` | 查询事实数据 | 不决定用户是否通过资格 |
| `quota-check` | 检查次数、频率、周期限制 | 不判断复杂业务规则 |
| `qualify` | 汇总事件、事实、状态和规则，输出准入决策 | 不查外部系统、不扣库存、不发奖 |
| `reward` | 计算奖励 | 不判断用户是否有资格 |
| `quota` / `budget-reserve` | 锁预算、库存、额度 | 不判断业务资格 |

## 配置建议

推荐把 `qualify` 配成可解释、可审计的规则集合：

- 每条规则必须有稳定 `code`，例如 `USER_ACTIVE`、`VIP_LEVEL_MIN`、`MIN_AMOUNT`。
- 每条失败规则要能输出 `reasonCode` 和 `reason`。
- 规则读取字段应来自标准路径，例如 `event_context.*`、`facts.*`、`quota_check.*`。
- 不要在规则里写外部查询逻辑。
- 生产发奖链路建议显式配置 `PASS`、`BLOCK`、`RISK` 三条路由。

## 常见例子

### 每日签到

数据来源：

- `event_context.attributes.checkin`
- `activity_state.continueDays`
- `quota_check.decision`

判断：

- 用户已签到。
- 今日还没有领过。
- 活动状态允许参与。

### 首存赠送

数据来源：

- `event_context.attributes.amount`
- `facts.USER_PROFILE.exists`
- `facts.USER_PROFILE.status`
- `facts.DEPOSIT_PROFILE.isFirstDeposit`

判断：

- 用户存在且正常。
- 是首次充值。
- 金额达到活动门槛。

### VIP 红利

数据来源：

- `facts.USER_PROFILE.vipLevel`
- `facts.USER_PROFILE.status`
- `quota_check.remaining`

判断：

- VIP 等级满足要求。
- 用户状态正常。
- 当前周期还有领取次数。

## 一句话总结

`qualify` 是活动链路里的“准入裁判”。

它不生产数据，只消费上游准备好的事件、事实、状态和次数检查结果，然后输出 `PASS`、`BLOCK` 或 `RISK`，决定用户是否继续进入奖励和发奖链路。
