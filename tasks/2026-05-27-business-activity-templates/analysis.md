# Analysis

## 需求 ID

2026-05-27-business-activity-templates

## Review 反馈处理

本轮已读取 `tasks/2026-05-27-business-activity-templates/review.md`，逐条处理如下：

| Review 项 | 处理 |
|---|---|
| P0: None | 无 P0。 |
| P1: T9 升级红利 `FORMULA` 样例仍不可按当前 schema 创建。`ruleConfig.formula` 已存在且补了 `resultType`，但公式字符串写成 `attributes.newLevel > attributes.oldLevel`；当前解析器只支持字段、字面量、`+ - * /` 和括号，比较符需要放在 `compare` 配置中。 | 已将 T9 `T9_NEW_GT_OLD` 规则改为 `formula = "attributes.newLevel - attributes.oldLevel"`、`resultType = "DECIMAL"`、`compare = {"op": ">", "value": "0"}`。需求口径同步收紧：`FORMULA` 的公式字符串只表达可解析算术结果，大小比较统一放入 `compare`，不得在 `formula` 内写 `>`、`>=`、`<`、`<=`、`==`、`!=`。 |
| P2: None | 无 P2。上一轮已关闭 T2 每月充值跨月误导问题，本轮保持单月活动窗口和按月拆活动的默认口径。 |
| Open Questions: None blocking beyond P1 | 已用最小可验收口径关闭 P1，不等待 Owner 决策。 |
| Score 78 / Decision BLOCK | 已关闭当前 P1；本轮重新评分并给出 Decision。 |

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版虽然自评 PASS，但仍存在 Review 指出的实质缺口：模板包范围说清楚了，具体可验收样例不足。逐条修正如下：

| 自检问题 | 修正 |
|---|---|
| 上一版自评 PASS，但 Review 证明 T9 `FORMULA` 样例仍不是当前 schema 可创建形态。 | 已把 `ruleConfig.formula` 内的比较表达式改为算术差值表达式，并通过 `compare` 表达“大于 0”。同时在通用输出、异常场景和验收标准中强调：`FORMULA` 必须使用 `formula`、`resultType` 和必要的 `compare`，比较符不得写入 `formula` 字符串。 |
| 上一版 T2 每月充值样例用单月编码和 `periodKey=2026-05`，但活动窗口跨 8 个月，容易让 Dev/Test 误认为 `USER_LIMIT` 会按 `periodKey` 自动分月。 | 已将 T2 推荐样例改为单月活动窗口，并说明跨月每月一次的默认最小可验收口径是“每月一个活动/活动副本”，不是单活动自动按 `periodKey` 重置 quota。 |
| 上一版保留的非阻塞待确认问题仍需要默认口径。 | 已继续保留默认最小可验收口径，不等待 Owner 决策；当前缺口均以配置边界和验收说明关闭。 |

## 资料依据

- `task.md`：不新增 ActivityPlugin 和管理后台增强，沉淀签到有礼、每月充值、续存、新手任务、返水、邀请好友、VIP 周/月红利、生日红利、升级红利模板；每个模板只使用标准事件 attributes、facts、规则、奖励、quota 和 outbox；提供模拟运行样例和边界说明。
- `docs/api.md`：标准事件幂等键、活动创建字段、规则字段白名单、奖励类型、quota、模拟运行、Fact Query、outbox、callback 和审批边界。
- `docs/data-model.md`：规则阶段、规则类型、字段路径、奖励计算、quota 类型、participation/grant_order/outbox 状态和 Fact Provider 映射。
- `docs/roadmap.md`：Phase 2 低风险活动和 Phase 3 高风险资金活动均要求通过配置完成，不新增业务专用 controller，模拟运行能解释命中和拒绝原因。
- `docs/architecture.md`：ExMarketing 只通过 Event In、Fact Query、Command Out、Callback In 与外部系统解耦；ActivityPlugin 不替代规则、奖励、quota、participation、grant_order、outbox 和幂等。

## 背景目标

ExMarketing 已具备活动、规则、奖励、quota、参与记录、发奖单、outbox、模拟运行和 Fact Query 等公共能力。当前任务不是新增玩法系统，而是把常见运营活动沉淀成普通配置化模板，让运营和接入方能够用统一活动模型创建活动，并用模拟运行提前验证命中、拒绝、发奖预览和 quota 风险。

目标：

1. 定义 9 类业务活动模板的默认字段、规则、奖励、quota、outbox 和模拟运行口径。
2. 每个模板均可通过标准事件、`attributes.*`、可选 `facts.*`、规则、奖励、quota 和 outbox 表达。
3. 不新增 ActivityPlugin、不新增业务专用 controller、不新增业务专用表、不新增管理后台模板市场或导入导出。
4. 高风险资金活动默认包含预算、用户次数、审批、审计、发奖回调和对账边界。
5. 对无法从现有材料唯一确定的业务参数，给出默认最小可验收口径，不等待 Owner 决策。

## 参与方

| 参与方 | 责任和关注点 |
|---|---|
| 运营 / 活动配置人员 | 按模板创建活动，理解必需字段、奖励和 quota，并通过模拟运行验收。 |
| 外部业务系统 | 上报标准事件；需要历史、会员、生日、邀请、VIP 状态时提供 Fact Provider。 |
| 活动引擎 | 按 MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET 统一链路处理。 |
| Reward / Wallet 系统 | 接收 outbox command，按 callback 回传发奖终态。 |
| 风控 / 审批 / 财务 | 审核高风险资金活动的预算、次数、审计和对账边界。 |
| Dev / Test Agent | 只按本文档实现或验收配置化模板能力，不用新增业务专用代码。 |

## 通用输入输出

### 标准事件输入

所有模板均使用统一事件模型：

```json
{
  "eventId": "evt_xxx",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_001"},
  "occurredAt": "2026-05-29T10:00:00Z",
  "attributes": {}
}
```

通用约束：

- `eventType` 必须命中活动 `triggerEvents`。
- `tenantId` 必须与活动租户一致，模板不得跨租户匹配。
- `subject.type` 默认 `USER`，`subject.id` 是发奖主体。
- `biz.bizType` 和 `biz.bizId` 必填，并参与幂等。
- 幂等键固定为 `tenantId + ":" + sourceSystem + ":" + eventType + ":" + biz.bizType + ":" + biz.bizId`。
- 业务字段只能出现在 `attributes.*` 或 `facts.<factCode>.*`，不得使用非标准顶层路径。

### 活动配置输出

每个模板至少输出：

- `tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`。
- `exclusiveGroup` 和 `stackable`，同类红利默认同组互斥。
- `rules[]`，只用 `CONDITION` 或安全 `FORMULA`；`FORMULA.ruleConfig` 必须使用 `formula` 和 `resultType`，不得使用 `expression`。当规则需要比较公式结果时，`formula` 字符串只写字段、字面量、`+ - * /` 和括号组成的算术表达式，比较条件写入 `compare`。
- `rewards[]`，用 `FIXED`、`RATIO`、`LADDER` 或当前已支持的非金额 `FIXED`。
- `quotas[]`，高风险资金活动必须包含 `BUDGET` 和用户次数限制。
- 模拟运行输入和预期输出摘要。

推荐活动 JSON 的默认最小可验收口径是完整 `POST /admin/v1/activities` 请求主体的活动主配置部分，而不是只含模板 metadata 的片段。为阅读清晰，本文把活动主配置、`rules`、`rewards` 和创建后单独配置的 `quotas` 分段展示；Dev/Test 验收时应将每个模板的推荐活动主配置与同模板 `rules` / `rewards` 合并为活动创建请求，quota 样例按 quota 接口配置。为避免重复，每个模板的推荐活动均使用以下公共 wrapper，并在模板内覆盖业务字段：

```json
{
  "tenantId": "site_001",
  "status": "DRAFT",
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100
}
```

公共字段约束：

- `tenantId` 示例值固定为 `site_001`，真实接入时替换为活动所属租户，不能通过 `entry.tenantIds` 扩大租户范围。
- `status` 默认 `DRAFT`。普通活动可在 L1 审批通过后上线；高风险资金模板必须完成 L2 审批后才能进入 `SCHEDULED` 或 `ACTIVE`。
- `startTime` / `endTime` 采用带时区偏移的 RFC3339 时间，且 `startTime < endTime`。
- `timezone` 默认 `Asia/Shanghai`，用于自然日、周、月和生日/升级周期解释；接入方可以按租户调整为 IANA 时区。
- `priority` 默认 100；同一 `exclusiveGroup` 下多个模板同时命中时按活动优先级和互斥叠加规则处理。
- 模板推荐的 `sourceSystem`、`bizType` 是配置说明 metadata，不作为活动顶层未知字段提交；完整请求中放入 `config.metadata.sourceSystem` 和 `config.metadata.bizType`。

## 通用业务规则

1. 模板不得新增 `ActivityPlugin`，也不得依赖 `state.plugin.*` 完成最小验收。
2. 模板不得新增业务专用 controller、数据库表、发奖逻辑、后台页面、模板选择器或导入导出。
3. 规则字段只允许标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`。
4. 能由当前事件携带的数据，默认使用 `attributes`；历史累计、会员资料、邀请关系、生日、VIP 状态才使用 Fact Query。
5. 金额计算使用 Decimal，奖励金额输出 4 位小数。
6. 金额类奖励计算为 `0.0000` 且无其他实际权益时，不生成 grant_order/outbox，不扣 quota。
7. 缺少 quota 行按当前系统兼容口径展示为 `unlimited=true`，但高风险资金模板不得以无限制作为生产推荐配置。
8. 重复事件不得重复 participation、grant_order 或 outbox。
9. 模拟运行不得写入 inbox、participation、grant_order、outbox 或 quota，只能展示预览。
10. `display.visible=false` 不影响事件处理；`entry.enabled=false` 在 MATCH 阶段拒绝候选活动。

## 高风险资金模板统一控制

T5 返水、T6 邀请好友、T7 VIP 周/月红利、T8 生日红利、T9 升级红利均按高风险资金模板处理。默认最小可验收口径如下：

| 控制项 | 默认口径 |
|---|---|
| 预算 | 必须配置活动级 `BUDGET` quota；缺失时模拟可展示 `unlimited=true`，但该模板不得作为生产推荐配置。 |
| 用户限制 | 必须配置 `USER_LIMIT` 或可等价表达单用户周期限制的 quota；周期由 `periodKey`、`scopeKey` 或活动时间窗口明确。 |
| 审批 | 生产上线必须走 L2 审批：提交人、L1 审批人、L2 审批人互不相同；审批只对锁定版本有效，配置变更后重新审批。 |
| 审计 | 必须保留 `CREATE_ACTIVITY`、`UPDATE_ACTIVITY`、`APPROVAL_SUBMIT`、`APPROVAL_APPROVE`、`CREATE_QUOTA`、`SIMULATION_RUN`、`EVENT_IN`、`QUOTA_EXCEEDED`、`GRANT_FAILED` 等可追踪审计；审计不得记录密钥、token、完整 Authorization、完整敏感 attributes 或完整 Fact Provider 响应。 |
| 发奖回调 | `AUTO` 发奖必须要求 Reward / Wallet 通过 `/openapi/v1/reward/callback` 回传终态；配置 callback secret 的租户必须校验签名，`SUCCESS` 确认 quota，`FAILED` 释放 quota。 |
| 对账 | 必须能用 `tenantId` + `activityId`、`subjectId`、`eventId`、`idempotencyKey` 或 `commandId` 查询 reconciliation，覆盖 participation、grant_order、outbox、callback 和 quota reservation。 |
| 财务复核 | 上线前财务复核预算上限、单用户次数、奖励金额或阶梯、流水倍数、目标发奖通道和对账查询口径；上线后按活动维度抽查 callback 成功率、长期冻结、DLQ 和 quota 使用。 |

上述控制项不要求本任务新增审批、审计、callback 或 reconciliation 功能；它们是高风险模板的生产配置边界和验收条件。若某环境尚未启用其中任一控制，高风险模板仍可作为模拟样例存在，但 Decision 口径不得标记为“可生产上线”。

## 模板配置样例

以下 JSON 是文档化模板包的最小可验收样例。`activityId` 在 quota 样例中用 `act_<template>` 占位，表示创建活动后得到的真实 ID；`targetRef` 使用示例安全地址或 MQ topic，不包含凭据。

### T1 签到有礼

默认口径：用户完成签到事件后，根据连续签到天数发放固定或阶梯奖励。营销系统不计算连续签到，只消费上游结果。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_SIGN_IN_DAILY_202605",
  "activityName": "签到有礼模板",
  "activityType": "SIGN_IN_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["SIGN_IN_SUCCESS"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "member", "bizType": "SIGN_IN"}},
  "exclusiveGroup": "sign_in_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t1_pass",
  "eventType": "SIGN_IN_SUCCESS",
  "sourceSystem": "member",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "SIGN_IN", "bizId": "10001:2026-05-29"},
  "occurredAt": "2026-05-29T02:00:00Z",
  "attributes": {"signDate": "2026-05-29", "continueDays": 3, "currency": "CNY"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T1_CONTINUE_DAYS_POSITIVE",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {"logic": "AND", "conditions": [{"field": "attributes.continueDays", "op": ">=", "value": "1"}]},
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T1_SIGN_IN_LADDER",
      "rewardType": "BONUS",
      "calculateType": "LADDER",
      "rewardConfig": {
        "field": "attributes.continueDays",
        "ladders": [
          {"min": "1", "max": "6", "amount": "2.00"},
          {"min": "7", "max": "999", "amount": "8.00"}
        ],
        "currency": "CNY",
        "flowMultiple": "1",
        "targetRef": "https://wallet.example.com/reward"
      },
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t1", "quotaType": "DAILY_LIMIT", "scope": "USER", "scopeKey": "10001:2026-05-29", "totalValue": "1"},
    {"activityId": "act_t1", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "5000.00"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | 使用上方事件，`continueDays=3`，每日 quota 未超。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `2.0000`；`quotaImpactPreviews` 出现且 `wouldExceed=false`；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.continueDays=0` 或缺失。 | `decision=REJECTED` 或 `ERROR`；`decisionReasonCode=RULE_NOT_PASSED` 或 `FIELD_MISSING`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：不维护真实签到状态，不计算连续天数；重复签到通过事件幂等和 `DAILY_LIMIT` 控制。

### T2 每月充值

默认口径：用户在自然月活动周期内充值成功，按单笔充值或上游月累计充值 facts 发奖。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_MONTHLY_DEPOSIT_202605",
  "activityName": "每月充值模板",
  "activityType": "MONTHLY_DEPOSIT_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["DEPOSIT_SUCCESS"],
  "startTime": "2026-05-01T00:00:00+08:00",
  "endTime": "2026-05-31T23:59:59+08:00",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "payment", "bizType": "DEPOSIT"}},
  "exclusiveGroup": "deposit_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t2_pass",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_t2_001"},
  "occurredAt": "2026-05-29T03:00:00Z",
  "attributes": {"amount": "500.00", "currency": "CNY", "periodKey": "2026-05"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T2_SINGLE_DEPOSIT_MIN",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.amount", "op": ">=", "value": "300.00"},
          {"field": "attributes.periodKey", "op": "=", "value": "2026-05"}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T2_DEPOSIT_RATIO",
      "rewardType": "BONUS",
      "calculateType": "RATIO",
      "rewardConfig": {"baseField": "attributes.amount", "rate": "0.05", "maxAmount": "200.00", "currency": "CNY", "flowMultiple": "3", "targetRef": "https://wallet.example.com/reward"},
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t2", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "20000.00"},
    {"activityId": "act_t2", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `attributes.amount=500.00`，预算和用户次数未超。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `25.0000`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT 且 `wouldExceed=false`；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.amount=100.00`。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：自然月由活动 `timezone` 与 `attributes.periodKey` 共同解释。当前 `USER_LIMIT + USER` 的 `scopeKey` 固定为 `subject.id`，不会按 `periodKey` 自动分月；因此推荐样例默认只表达 2026 年 5 月单月内用户一次。若运营要做 2026-05 到 2026-12 每月一次，最小可验收口径是每个自然月创建一个独立活动编码或活动副本，并分别设置当月 `startTime`、`endTime` 和 `periodKey`；单个跨月活动只能表达活动期内用户总次数。

### T3 续存

默认口径：非首存充值成功后发放固定、比例或阶梯奖励。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_RE_DEPOSIT_202605",
  "activityName": "续存模板",
  "activityType": "RE_DEPOSIT_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["DEPOSIT_SUCCESS"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "payment", "bizType": "DEPOSIT"}},
  "exclusiveGroup": "deposit_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t3_pass",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_t3_001"},
  "occurredAt": "2026-05-29T04:00:00Z",
  "attributes": {"amount": "300.00", "currency": "CNY", "isFirstDeposit": false}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T3_RE_DEPOSIT_QUALIFY",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.isFirstDeposit", "op": "=", "value": false},
          {"field": "attributes.amount", "op": ">=", "value": "200.00"}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T3_RE_DEPOSIT_FIXED",
      "rewardType": "BONUS",
      "calculateType": "FIXED",
      "rewardConfig": {"amount": "18.00", "currency": "CNY", "flowMultiple": "3", "targetRef": "https://wallet.example.com/reward"},
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t3", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "10000.00"},
    {"activityId": "act_t3", "quotaType": "DAILY_LIMIT", "scope": "USER", "scopeKey": "10001:2026-05-29", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `isFirstDeposit=false` 且 `amount=300.00`。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `18.0000`；`quotaImpactPreviews` 出现 BUDGET 和 DAILY_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.isFirstDeposit=true` 或字段缺失。 | `decision=REJECTED` 或 `ERROR`；`decisionReasonCode=RULE_NOT_PASSED` 或 `FIELD_MISSING`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：首存判断必须由上游事件或 Fact Provider 提供，营销系统不计算充值序号。

### T4 新手任务

默认口径：新用户完成指定任务事件后发放金额或非金额权益。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_NEW_USER_TASK_KYC_202605",
  "activityName": "新手任务模板",
  "activityType": "NEW_USER_TASK",
  "status": "DRAFT",
  "triggerEvents": ["TASK_COMPLETED"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "task", "bizType": "TASK"}},
  "exclusiveGroup": "new_user_task",
  "stackable": true
}
```

通过事件样例：

```json
{
  "eventId": "evt_t4_pass",
  "eventType": "TASK_COMPLETED",
  "sourceSystem": "task",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "TASK", "bizId": "10001:KYC_VERIFIED"},
  "occurredAt": "2026-05-29T05:00:00Z",
  "attributes": {"taskCode": "KYC_VERIFIED", "isNewUser": true, "segments": ["new_user"]}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T4_NEW_USER_TASK_MATCH",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.isNewUser", "op": "=", "value": true},
          {"field": "attributes.taskCode", "op": "IN", "value": ["KYC_VERIFIED", "FIRST_LOGIN"]}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T4_TASK_POINTS",
      "rewardType": "POINT",
      "calculateType": "FIXED",
      "rewardConfig": {"points": 100, "pointType": "LOYALTY", "targetRef": "kafka://reward.commands"},
      "grantMode": "AUTO",
      "grantChannel": "MQ",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t4", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"},
    {"activityId": "act_t4", "quotaType": "STOCK", "scope": "ACTIVITY", "scopeKey": "T4_TASK_POINTS", "totalValue": "100000"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `taskCode=KYC_VERIFIED`，`isNewUser=true`。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现，非金额 reward amount `0.0000` 且 payload 含 `points=100`；`quotaImpactPreviews` 出现 USER_LIMIT 和 STOCK；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.isNewUser=false` 或 `taskCode` 不在列表。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：不维护任务进度，只消费“任务已完成”事件；任务计算由上游任务系统负责。

### T5 返水

默认口径：按投注周期结算事件，根据有效投注额或亏损额发放比例奖励。返水是高风险资金活动。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_REBATE_WEEKLY_202605W22",
  "activityName": "返水模板",
  "activityType": "REBATE_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["REBATE_SETTLED", "BETTING_PERIOD_SETTLED"],
  "startTime": "2026-05-25T00:00:00+08:00",
  "endTime": "2026-06-01T00:00:00+08:00",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "game", "bizType": "REBATE_PERIOD"}},
  "exclusiveGroup": "rebate_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t5_pass",
  "eventType": "REBATE_SETTLED",
  "sourceSystem": "game",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "REBATE_PERIOD", "bizId": "10001:2026-W22:SLOTS"},
  "occurredAt": "2026-05-29T06:00:00Z",
  "attributes": {"validBetAmount": "10000.00", "netLossAmount": "800.00", "currency": "CNY", "periodStart": "2026-05-25T00:00:00+08:00", "periodEnd": "2026-06-01T00:00:00+08:00", "gameCategory": "SLOTS", "periodKey": "2026-W22"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T5_VALID_BET_MIN",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {"logic": "AND", "conditions": [{"field": "attributes.validBetAmount", "op": ">=", "value": "1000.00"}]},
      "sort": 1,
      "enabled": true
    },
    {
      "ruleCode": "T5_RISK_NORMAL",
      "rulePhase": "RISK",
      "ruleType": "CONDITION",
      "ruleConfig": {"logic": "AND", "conditions": [{"field": "facts.RISK_PROFILE.riskLevel", "op": "IN", "value": ["NORMAL", "LOW"]}]},
      "sort": 2,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T5_REBATE_RATIO",
      "rewardType": "BONUS",
      "calculateType": "RATIO",
      "rewardConfig": {"baseField": "attributes.validBetAmount", "rate": "0.01", "maxAmount": "300.00", "currency": "CNY", "flowMultiple": "1", "targetRef": "https://wallet.example.com/reward"},
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t5", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "100000.00"},
    {"activityId": "act_t5", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `validBetAmount=10000.00`，`facts.RISK_PROFILE.riskLevel=NORMAL`，预算未超。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `100.0000`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Quota risk | 当前 BUDGET 剩余小于 `100.0000`。 | `decision=REJECTED`；`decisionReasonCode=QUOTA_EXCEEDED`；`rewardPreviews` 可出现计算金额但标记不可执行；`quotaImpactPreviews` 出现且 `wouldExceed=true`；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：不计算有效投注额、不查询游戏流水；结算值必须由事件或 Fact Provider 提供。作为高风险资金模板，生产上线必须具备活动级 BUDGET、单用户单周期 USER_LIMIT、L2 审批、配置/审批/模拟/Fact Query/quota/发奖审计、signed callback、reconciliation 查询和财务复核；财务复核重点是结算周期、有效投注额来源、返水比例、封顶金额、流水倍数和预算余量。

### T6 邀请好友

默认口径：邀请关系达标事件触发后，给邀请人或被邀请人发奖励。默认发给事件主体，即邀请人。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_INVITE_QUALIFIED_202605",
  "activityName": "邀请好友模板",
  "activityType": "INVITE_FRIEND_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["INVITE_QUALIFIED"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "invite", "bizType": "INVITE"}},
  "exclusiveGroup": "invite_bonus",
  "stackable": true
}
```

通过事件样例：

```json
{
  "eventId": "evt_t6_pass",
  "eventType": "INVITE_QUALIFIED",
  "sourceSystem": "invite",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "inviter_10001"},
  "biz": {"bizType": "INVITE", "bizId": "inviter_10001:invitee_20001"},
  "occurredAt": "2026-05-29T07:00:00Z",
  "attributes": {"inviteeId": "invitee_20001", "qualified": true, "qualifyType": "DEPOSIT_AND_BET", "inviteeDepositAmount": "500.00", "inviteeValidBetAmount": "1200.00", "currency": "CNY"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T6_INVITE_QUALIFIED",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.qualified", "op": "=", "value": true},
          {"field": "attributes.qualifyType", "op": "IN", "value": ["DEPOSIT_AND_BET"]},
          {"field": "attributes.inviteeDepositAmount", "op": ">=", "value": "100.00"},
          {"field": "attributes.inviteeValidBetAmount", "op": ">=", "value": "500.00"}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T6_INVITER_FIXED",
      "rewardType": "BONUS",
      "calculateType": "FIXED",
      "rewardConfig": {"amount": "58.00", "currency": "CNY", "flowMultiple": "3", "targetRef": "https://wallet.example.com/reward"},
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t6", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "50000.00"},
    {"activityId": "act_t6", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "inviter_10001", "totalValue": "20"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `qualified=true` 且充值/投注达标。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `58.0000`、发奖主体为 `subject.id=inviter_10001`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.qualified=false` 或 `inviteeValidBetAmount=100.00`。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：不维护邀请关系、不判断作弊；达标结论由上游事件或 facts 提供。若同时奖励邀请人和被邀请人，默认建两个活动或两个 reward，并保证幂等键和发奖主体设计清楚。作为高风险资金模板，生产上线必须具备活动级 BUDGET、单邀请人 USER_LIMIT、L2 审批、配置/审批/模拟/quota/发奖审计、signed callback、reconciliation 查询和财务复核；缺任一项时只能作为模拟或草稿模板，不得标记为生产推荐配置。

### T7 VIP 周/月红利

默认口径：VIP 用户在周或月周期达标后领取或自动发放红利。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_VIP_PERIOD_WEEK_2026W22",
  "activityName": "VIP 周/月红利模板",
  "activityType": "VIP_PERIOD_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["VIP_BONUS_CLAIMED", "VIP_PERIOD_SETTLED"],
  "startTime": "2026-05-25T00:00:00+08:00",
  "endTime": "2026-06-01T00:00:00+08:00",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "member", "bizType": "VIP_PERIOD"}},
  "exclusiveGroup": "vip_period_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t7_pass",
  "eventType": "VIP_BONUS_CLAIMED",
  "sourceSystem": "member",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "VIP_PERIOD", "bizId": "10001:WEEK:2026-W22"},
  "occurredAt": "2026-05-29T08:00:00Z",
  "attributes": {"vipLevel": 3, "periodType": "WEEK", "periodKey": "2026-W22", "currency": "CNY", "validBetAmount": "5000.00"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T7_VIP_PERIOD_QUALIFY",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.periodType", "op": "IN", "value": ["WEEK", "MONTH"]},
          {"field": "attributes.vipLevel", "op": ">=", "value": "1"},
          {"field": "facts.VIP_STATE.claimed", "op": "=", "value": false}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T7_VIP_LEVEL_LADDER",
      "rewardType": "BONUS",
      "calculateType": "LADDER",
      "rewardConfig": {
        "field": "attributes.vipLevel",
        "ladders": [
          {"min": "1", "max": "2", "amount": "20.00"},
          {"min": "3", "max": "5", "amount": "80.00"}
        ],
        "currency": "CNY",
        "flowMultiple": "1",
        "targetRef": "https://wallet.example.com/reward"
      },
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t7", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "80000.00"},
    {"activityId": "act_t7", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `attributes.vipLevel=3`，`facts.VIP_STATE.claimed=false`。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `80.0000`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `facts.VIP_STATE.claimed=true`。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：周期 key 必须由上游明确提供，如 `2026-W22` 或 `2026-05`。模板不计算 VIP 等级、不维护领取状态；若领取状态不在事件中，必须使用 Fact Provider。作为高风险资金模板，生产上线必须具备活动级 BUDGET、单用户单周期 USER_LIMIT、L2 审批、配置/审批/模拟/Fact Query/quota/发奖审计、signed callback、reconciliation 查询和财务复核；财务复核重点是周期范围、VIP 阶梯金额、流水倍数、预算余量和领取状态事实来源。

### T8 生日红利

默认口径：用户生日当天或生日月触发领取事件后发放固定或 VIP 阶梯奖励。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_BIRTHDAY_BONUS_2026",
  "activityName": "生日红利模板",
  "activityType": "BIRTHDAY_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["BIRTHDAY_BONUS_CLAIMED"],
  "startTime": "2026-01-01T00:00:00+08:00",
  "endTime": "2026-12-31T23:59:59+08:00",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "member", "bizType": "BIRTHDAY"}},
  "exclusiveGroup": "birthday_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t8_pass",
  "eventType": "BIRTHDAY_BONUS_CLAIMED",
  "sourceSystem": "member",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "BIRTHDAY", "bizId": "10001:2026"},
  "occurredAt": "2026-05-29T09:00:00Z",
  "attributes": {"birthdayMatched": true, "birthdayPeriod": "2026", "vipLevel": 2, "currency": "CNY"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T8_BIRTHDAY_QUALIFY",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.birthdayMatched", "op": "=", "value": true},
          {"field": "facts.BIRTHDAY_STATE.claimed", "op": "=", "value": false},
          {"field": "attributes.vipLevel", "op": ">=", "value": "1"}
        ]
      },
      "sort": 1,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T8_BIRTHDAY_LADDER",
      "rewardType": "BONUS",
      "calculateType": "LADDER",
      "rewardConfig": {
        "field": "attributes.vipLevel",
        "ladders": [
          {"min": "1", "max": "2", "amount": "30.00"},
          {"min": "3", "max": "9", "amount": "100.00"}
        ],
        "currency": "CNY",
        "flowMultiple": "1",
        "targetRef": "https://wallet.example.com/reward"
      },
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t8", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "30000.00"},
    {"activityId": "act_t8", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `birthdayMatched=true`，`facts.BIRTHDAY_STATE.claimed=false`，`vipLevel=2`。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `30.0000`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.birthdayMatched=false` 或 `facts.BIRTHDAY_STATE.claimed=true`。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：默认由上游事件提供 `birthdayMatched` 和 `birthdayPeriod`，避免营销系统保存生日明文；Fact Provider 响应和审计不得暴露完整敏感资料。作为高风险资金模板，生产上线必须具备活动级 BUDGET、单用户生日周期 USER_LIMIT、L2 审批、配置/审批/模拟/Fact Query/quota/发奖审计、signed callback、reconciliation 查询和财务复核；财务复核重点是生日周期口径、敏感信息脱敏、阶梯金额和预算消耗。

### T9 升级红利

默认口径：用户 VIP 等级或会员等级升级事件触发后，按新等级发放奖励。

推荐活动：

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_LEVEL_UP_BONUS_202605",
  "activityName": "升级红利模板",
  "activityType": "LEVEL_UP_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["USER_LEVEL_UP", "VIP_LEVEL_UP"],
  "startTime": "2026-05-01T00:00:00Z",
  "endTime": "2026-12-31T23:59:59Z",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "member", "bizType": "LEVEL_UP"}},
  "exclusiveGroup": "level_up_bonus",
  "stackable": false
}
```

通过事件样例：

```json
{
  "eventId": "evt_t9_pass",
  "eventType": "VIP_LEVEL_UP",
  "sourceSystem": "member",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "LEVEL_UP", "bizId": "VIP:10001:2:3"},
  "occurredAt": "2026-05-29T10:00:00Z",
  "attributes": {"oldLevel": 2, "newLevel": 3, "levelType": "VIP", "upgradeReason": "MONTHLY_SETTLEMENT", "currency": "CNY"}
}
```

规则、奖励、quota 样例：

```json
{
  "rules": [
    {
      "ruleCode": "T9_LEVEL_UP_VALID",
      "rulePhase": "QUALIFY",
      "ruleType": "CONDITION",
      "ruleConfig": {
        "logic": "AND",
        "conditions": [
          {"field": "attributes.newLevel", "op": ">", "value": "0"},
          {"field": "attributes.oldLevel", "op": ">=", "value": "0"},
          {"field": "attributes.levelType", "op": "IN", "value": ["VIP", "MEMBER"]}
        ]
      },
      "sort": 1,
      "enabled": true
    },
    {
      "ruleCode": "T9_NEW_GT_OLD",
      "rulePhase": "QUALIFY",
      "ruleType": "FORMULA",
      "ruleConfig": {
        "formula": "attributes.newLevel - attributes.oldLevel",
        "resultType": "DECIMAL",
        "compare": {"op": ">", "value": "0"}
      },
      "sort": 2,
      "enabled": true
    },
    {
      "ruleCode": "T9_NOT_CLAIMED",
      "rulePhase": "RISK",
      "ruleType": "CONDITION",
      "ruleConfig": {"logic": "AND", "conditions": [{"field": "facts.LEVEL_STATE.upgradeRewardClaimed", "op": "=", "value": false}]},
      "sort": 3,
      "enabled": true
    }
  ],
  "rewards": [
    {
      "rewardCode": "T9_LEVEL_UP_LADDER",
      "rewardType": "BONUS",
      "calculateType": "LADDER",
      "rewardConfig": {
        "field": "attributes.newLevel",
        "ladders": [
          {"min": "1", "max": "2", "amount": "20.00"},
          {"min": "3", "max": "5", "amount": "88.00"}
        ],
        "currency": "CNY",
        "flowMultiple": "1",
        "targetRef": "https://wallet.example.com/reward"
      },
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true
    }
  ],
  "quotas": [
    {"activityId": "act_t9", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "60000.00"},
    {"activityId": "act_t9", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001", "totalValue": "1"}
  ]
}
```

模拟预期：

| 样例 | 输入差异 | 预期 |
|---|---|---|
| Pass | `oldLevel=2`、`newLevel=3`、`facts.LEVEL_STATE.upgradeRewardClaimed=false`。 | `decision=PASSED`；`decisionReasonCode` 为空或 `PASSED`；`rewardPreviews` 出现且金额 `88.0000`；`quotaImpactPreviews` 出现 BUDGET 和 USER_LIMIT；`grantOrderPreviews` 出现；`outboxCommandPreviews` 出现。 |
| Reject | `attributes.newLevel=2`、`attributes.oldLevel=3`，或 `facts.LEVEL_STATE.upgradeRewardClaimed=true`。 | `decision=REJECTED`；`decisionReasonCode=RULE_NOT_PASSED`；`rewardPreviews` 不出现或标记 skipped；`quotaImpactPreviews` 不出现扣减；`grantOrderPreviews` 不出现；`outboxCommandPreviews` 不出现。 |

边界：不计算升级条件、不维护等级历史。若要求“每个等级只领一次”，默认由上游将 `bizId` 设计为 `levelType:userId:oldLevel:newLevel`，并结合 Fact Provider 校验已领取状态。作为高风险资金模板，生产上线必须具备活动级 BUDGET、单用户升级链路 USER_LIMIT、L2 审批、配置/审批/模拟/Fact Query/quota/发奖审计、signed callback、reconciliation 查询和财务复核；财务复核重点是新旧等级事实来源、阶梯金额、幂等键和预算余量。

## 异常场景

| 场景 | 期望 |
|---|---|
| 模板配置使用未知顶层业务字段，如 `amount`、`vipLevel`、`newLevel`。 | 拒绝配置，提示必须使用 `attributes.*` 或 `facts.*`。 |
| 模板使用 `SCRIPT` 或未注册 `LIMIT` evaluator。 | 拒绝配置；本任务不得要求实现脚本或高级限制执行器。 |
| `FORMULA` 规则使用 `expression`、缺少 `formula` 或缺少 `resultType`。 | 创建或上线前校验拒绝；模板样例必须使用 `ruleConfig.formula` 和合法 `resultType`。 |
| `FORMULA.formula` 字符串直接包含 `>`、`>=`、`<`、`<=`、`==`、`!=` 等比较符。 | 创建或上线前校验拒绝；默认最小口径是 `formula` 只输出算术或字段结果，比较条件放入 `ruleConfig.compare`，例如 `formula = "attributes.newLevel - attributes.oldLevel"` 且 `compare.op = ">"`、`compare.value = "0"`。 |
| 事件缺少模板必需 attributes。 | 规则不通过或运行结果 `ERROR`，不得默认补 0、空字符串或 false 通过。 |
| Decimal 字段不可解析。 | 配置或运行拒绝，返回结构化原因。 |
| Fact Provider 未配置、禁用、超时或 mapping 失败。 | 使用 `FACT_QUERY_FAILED`、`FACT_MAPPING_FAILED`、`FACT_FIELD_MISSING` 等解释；依赖该 fact 的规则不得默认通过。 |
| quota 预览或正式扣减超限。 | 返回 `QUOTA_EXCEEDED` 风险或 409；不得生成可执行 grant order 和 outbox。 |
| 金额类奖励金额为 `0.0000`。 | 不生成 grant_order/outbox/quota 扣减；参与状态按现有零奖励口径处理。 |
| 非金额类 `FIXED` 权益数量为正。 | 即使 `reward_amount=0.0000`，仍生成 grant_order/outbox，并按配置扣减 STOCK、USER_LIMIT、DAILY_LIMIT。 |
| 重复事件上报。 | `duplicate=true`，不得重复 participation、grant_order 或 outbox。 |
| 高风险活动缺少 BUDGET 或 USER_LIMIT。 | 模拟可展示 `unlimited=true`，但模板验收标记为上线配置不完整，不作为生产推荐配置。 |
| 活动时间跨周/月/年。 | 周/月/生日/升级等周期口径必须由 `periodKey` 或活动 `timezone` 明确，不能依赖服务器本地时区。 |

## 边界条件

- 模板只定义配置和样例，不保证上游系统真实上报这些事件；上游事件契约需要另行对接。
- 模板不新增后台 UI、模板市场、配置向导、导入导出或 fixture 文件。
- 模板不实现签到连续天数、任务进度、返水统计、邀请关系、VIP 等级、生日匹配、升级判断等业务计算。
- 模板不直接发奖、不调用钱包 SDK，只生成 grant_order 和 outbox。
- 模板不新增 ActivityPlugin。后续若某玩法必须由营销系统维护真实状态，应另开 ActivityPlugin 或玩法状态需求。
- 周期限制在当前系统能力不足时，以活动时间窗口、事件 `periodKey`、用户次数 quota 和 Fact Provider 组合表达最小可验收口径。
- 所有模板都必须租户隔离，不能通过 `entry.tenantIds` 扩大活动租户边界。

## 非目标范围

1. 不新增或修改代码实现细节。
2. 不新增 ActivityPlugin、不实现签到/任务/VIP/邀请等真实玩法状态持久化。
3. 不新增管理后台页面、模板选择器、模板导入导出、可视化配置向导或 JSON fixture 文件。
4. 不新增业务专用 API、controller、数据库表或外部业务库查询。
5. 不新增任意脚本、SQL、HTTP 函数、动态代码执行能力。
6. 不定义外部钱包、会员、游戏、邀请系统的内部实现。
7. 不保证所有模板一次性可生产上线；本任务验收对象是文档化模板包、模拟样例和边界说明。

## 风险点

| 风险 | 控制方式 |
|---|---|
| 模板被误实现为业务专用代码。 | 明确所有模板只使用标准活动配置、规则、奖励、quota、outbox 和模拟运行。 |
| 上游字段命名不统一导致模板不可复用。 | 本轮给出默认 attributes 命名；接入方不同命名需在上游 mapping 或模板副本中显式调整。 |
| 高风险资金活动缺少生产控制。 | 返水、邀请、VIP、生日、升级默认要求 BUDGET、用户限制、L2 审批、审计、signed callback、reconciliation 和财务复核；模拟样例必须展示 quota 预览。 |
| 周/月/生日/等级等周期状态不准确。 | 默认要求上游提供 `periodKey`、`birthdayMatched`、`oldLevel/newLevel` 等确定结果，营销系统不自行推导。 |
| Fact Query 失败导致活动误发。 | 依赖 facts 的规则在 fact 失败、mapping 失败或字段缺失时不得默认通过。 |
| 重复发奖。 | 使用标准事件幂等、participation 幂等、grant_order 幂等和 quota 原子扣减；模板样例覆盖重复事件说明。 |

## 验收标准

1. `analysis.md` 列出的 9 个模板均包含活动主配置字段 `tenantId`、`activityCode`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`，以及标准事件 JSON、规则 JSON、奖励 JSON、quota 样例、通过模拟预期、拒绝或 quota 风险模拟预期和边界说明。
2. 所有规则字段均使用标准事件字段、`attributes.*`、`facts.*` 或当前允许的 `state.*`；不得出现非标准顶层业务字段；`FORMULA` 规则必须使用 `ruleConfig.formula` 和 `resultType`，需要大小比较时必须使用 `ruleConfig.compare`，不得把比较符写入 `formula` 字符串。
3. 每个模板的模拟预期必须说明 `decision`、关键 `decisionReasonCode`、是否出现 `rewardPreviews`、`quotaImpactPreviews`、`grantOrderPreviews`、`outboxCommandPreviews`。
4. 模板配置不得要求新增 ActivityPlugin、业务专用 controller、业务专用发奖逻辑、数据库表或管理后台增强。
5. 高风险资金模板，即返水、邀请好友、VIP 周/月红利、生日红利、升级红利，必须逐模板明确预算、用户次数、L2 审批、审计、signed callback、reconciliation 和财务复核边界。
6. 模拟运行预期必须能解释命中、规则拒绝、Fact Query 失败、quota 超限和零奖励跳过。
7. 重复事件、缺失字段、非法 Decimal、未知 fact、quota 超限、周期边界均有明确处理口径。
8. 产物只更新 `tasks/2026-05-27-business-activity-templates/analysis.md`，不写代码、不提交、不改其他 Agent 产物。

## 待确认问题

以下问题不阻塞本轮研发和 Review，均已有默认最小口径：

| 问题 | 默认口径 |
|---|---|
| 各业务系统实际事件名是否与模板默认一致。 | 模板给出推荐事件名；接入时可在不改变标准事件结构的前提下替换 `eventType` 和 `sourceSystem`。 |
| 交付物是否需要独立 JSON fixture 文件。 | 本任务只交付 `analysis.md` 文档化模板包，不新增 fixture 文件；自动化 fixture 另开需求。 |
| 推荐活动 JSON 是否必须是单个完整文件。 | 本文档不新增 fixture 文件；推荐活动主配置与同模板 `rules` / `rewards` 合并后即为活动创建请求，`quotas` 按 quota 接口单独创建。 |
| 周/月周期 quota 是否已有完整滚动窗口能力。 | 最小口径使用活动时间窗口、事件 `periodKey`、`USER_LIMIT` 和 Fact Provider 表达，不声称支持任意滚动周期；每月充值的推荐样例按单月活动验收，跨多月每月一次需拆成每月独立活动或活动副本。 |
| VIP、生日、邀请等状态由谁计算。 | 默认由上游事件 attributes 或 Fact Provider 提供确定结果，营销系统不新增业务状态计算。 |
| 非金额权益奖励是否全部可执行。 | 当前最小口径只要求非金额类 `FIXED` 权益 payload 可配置；若外部发奖系统不支持，使用 `MANUAL` 或 `MANUAL_EXPORT`。 |
| 是否需要后台一键创建模板。 | 非目标。本任务只沉淀模板配置说明和模拟样例。 |

## Score

```text
96
```

评分：

| 维度 | 分数 | 说明 |
|---|---:|---|
| 需求清晰度 | 19/20 | 已明确模板包交付形态、9 类模板、活动主配置字段、非目标和默认口径。 |
| 业务规则完整 | 20/20 | 每个模板均有事件、规则、奖励、quota 和模拟预期；T9 `FORMULA` 已使用算术公式加 `compare` 的当前可创建 schema；真实上游事件名仍允许接入时映射。 |
| 边界异常清楚 | 19/20 | 覆盖缺字段、Fact Query、quota、重复事件、零奖励、非金额权益、周期和高风险生产控制边界。 |
| 验收标准明确 | 20/20 | 核心 JSON 样例、模拟输出字段、`FORMULA` 的 `formula/resultType/compare` 约束和高风险控制验收项已落地；自动化 fixture 不在本轮。 |
| 约束一致性 | 18/20 | 与现有 API、数据模型、路线图和 ActivityPlugin 边界一致。 |

## Decision

```text
PASS
```

理由：Review P1 和 Open Question 已关闭；上一轮 Analysis 自检缺口已修正。T9 升级红利模板不再在 `FORMULA.formula` 字符串内写比较符，而是使用算术差值加 `compare` 表达新等级大于旧等级；T2 每月充值样例已按当前 quota 能力收敛为单月活动。本文档已给出默认最小可验收口径，需求细节清楚、可审核、可验收，可进入 Review Agent。
