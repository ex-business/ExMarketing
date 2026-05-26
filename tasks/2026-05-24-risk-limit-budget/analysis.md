# Analysis

## 需求 ID

2026-05-24-risk-limit-budget

## Review 反馈处理

本轮已读取 `tasks/2026-05-24-risk-limit-budget/review.md`，Review 结论为 `BLOCK`，Score `76/100`。处理如下：

| Review 项 | 处理 |
|---|---|
| P1：互斥范围在上一版中同时出现“同一事件候选组裁剪”和“跨历史 participation 活动期互斥”，口径冲突。 | 已修正。本轮互斥只继承 MATCH 阶段的“同一事件候选活动裁剪”语义：同一标准事件同时命中同租户、同 subject、同 `exclusiveGroup` 的多个活动时，按稳定排序只保留一个可发奖候选。不得查询历史 participation 判断“用户先参加 A 后不能参加 B”。跨事件、活动期级互斥列为非目标和后续 Phase 3 增强。 |
| P2：IP / Device 被写入 LIMIT 范围但验收缺失。 | 已修正。本轮最小交付不包含 IP / Device quota 执行。依据：`docs/roadmap.md` 将 IP / Device 限制列入 Phase 3；当前 `docs/api.md` 也说明 DAILY_LIMIT 模型已定义但业务流程待扩展，IP / DEVICE 仅是 scope 枚举。事件可继续携带 `attributes.ip`、`attributes.deviceId` 给 RISK/普通 CONDITION 规则读取，但不要求本轮实现 `IP` 或 `DEVICE` scope 的次数扣减、审计或错误码。 |
| P2：`LIMIT_EXCEEDED` 与 `QUOTA_EXCEEDED` 混用。 | 已修正。对外 HTTP 错误码固定为：所有 quota-backed 限制不足，包括 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`，均返回 `409 QUOTA_EXCEEDED`，body.code 也为 `QUOTA_EXCEEDED`。不再定义 `LIMIT_EXCEEDED` 作为本轮对外错误码。RISK/CONDITION 规则拒绝和互斥落选是活动级拒绝原因，不使用 `LIMIT_EXCEEDED`。 |
| Open Question 1：互斥是同一事件候选裁剪还是跨历史 participation。 | 已关闭为同一事件候选裁剪；跨历史 participation 互斥不在本轮。 |
| Open Question 2：IP / Device limit 是否本轮必须交付。 | 已关闭为不交付；只保留字段可被 RISK/CONDITION 规则读取。 |
| Open Question 3：次数类限制失败是否统一为 `409 QUOTA_EXCEEDED`。 | 已关闭为是。`USER_LIMIT` 和 `DAILY_LIMIT` 不足统一返回 `409 QUOTA_EXCEEDED`，并在 body/audit 中通过 `quotaType`、`scope`、`scopeKey` 区分。 |

## Analysis 自检修正

上一轮 `analysis.md` 自身为 `Decision PASS`、Score `88/100`，但被 Review 识别出以下错误假设和扣分项。本轮逐条修正：

| 上一轮问题 | 修正 |
|---|---|
| 把互斥写成“同一 tenant、subject、exclusiveGroup 下只允许一个活动产生发奖资格”，容易被理解为检查既有 participation。 | 改为“同一事件处理过程内的候选活动裁剪”，只影响当前事件同时命中的活动集合。 |
| `EXCLUSIVE_CONFLICT` 包含“既有有效参与记录”。 | 删除历史 participation 语义。本轮互斥拒绝原因固定为 `EXCLUSIVE_GROUP_LOST`，用于当前事件落选活动的 participation/audit 解释。 |
| 待确认问题又说跨事件生命周期级互斥为后续增强，和前文冲突。 | 已关闭冲突：跨事件生命周期级互斥明确列入非目标，不再作为待确认阻塞。 |
| DAILY_LIMIT、IP、DEVICE 范围过宽，验收不匹配。 | 本轮仅补齐 `DAILY_LIMIT + USER` 的自然日次数限制；IP/Device quota 执行移出本轮。 |
| `LIMIT_EXCEEDED` 与 `QUOTA_EXCEEDED` 双码并存。 | 对外失败统一为 `409 QUOTA_EXCEEDED`；活动级规则拒绝使用 participation 的 `reject_reason`，不作为 HTTP quota 错误。 |

## 资料依据

- `tasks/2026-05-24-risk-limit-budget/task.md`：要求实现 RISK、LIMIT、BUDGET 阶段，覆盖风控标签、次数、周期、互斥、预算、库存、幂等限制；额度不足或限制失败不能生成 grant_order，失败写审计并返回结构化错误。
- `README.md`、`CLAUDE.md`：系统只接收标准事件，不连接外部业务库，不直接改余额，必须通过 participation、grant_order、outbox 发奖；额度不足返回 `409 Conflict`。
- `docs/architecture.md`：核心链路为 `Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox`；规则阶段为 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`，其中 `BUDGET` 是 quota 类型，不是 rule phase。
- `docs/api.md`：Event API 错误码已有 `409 QUOTA_EXCEEDED`；模拟运行无副作用；quota 行缺失按无限制展示；OpenAPI event 幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- `docs/data-model.md`：`marketing_quota` 支持 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`；当前已实现口径包括 `BUDGET + ACTIVITY + TOTAL`、`STOCK + ACTIVITY + rewardCode`、`USER_LIMIT + USER + subject.id`；缺少 quota 行表示不限制。
- `docs/roadmap.md`：每日/周期 quota 属于 Phase 2 需要补齐；完整活动互斥组和 IP / Device 限制属于 Phase 3。
- `22-独立活动营销中台设计.md`：活动系统负责规则判断、预算库存、参与记录、幂等、发奖编排和审计；不负责真实风控判定和钱包余额修改。

## 背景目标

本需求在 ExMarketing 标准事件处理链路中补齐 RISK、LIMIT、BUDGET 的最小可上线口径，使发奖前能够统一拦截以下风险：

- 风控标签、风险等级或配置化风控条件不允许。
- 用户活动期次数超限。
- 用户按活动时区自然日次数超限。
- 同一事件同时命中互斥组内多个活动，需要稳定裁剪。
- 活动预算不足、奖品库存不足。
- 重复事件、幂等键冲突或重复发奖风险。

核心目标不是实现真实风控系统，也不是直接发奖或修改钱包，而是在标准事件进入后，只有当前 activity 的 RISK、LIMIT、BUDGET 全部通过时，才允许在同一 activity 级原子边界内生成 `REWARDABLE` participation、扣减 quota、生成 `marketing_grant_order` 和 `marketing_outbox`。任一 quota-backed 限制不足时，必须阻止 grant_order/outbox 生成，写失败审计，并向调用方返回稳定结构化错误。

## 参与方

| 参与方 | 关注点 |
|---|---|
| 外部业务系统 | 通过 `POST /openapi/v1/events` 或 Webhook 映射后的标准事件上报业务事实，提供稳定 `Idempotency-Key`。 |
| Event Intake | 校验事件模型、鉴权、租户授权、幂等键和 payload 一致性，写入 event inbox。 |
| 活动引擎 | 对 MATCH 后候选活动执行 QUALIFY、REWARD、RISK、LIMIT，并在发奖前检查 quota。 |
| 运营 / 管理后台 | 配置活动、规则、奖励、quota，查询模拟运行和审计解释。 |
| Quota / Ledger 边界 | 承载预算、库存、用户次数和用户日次数的原子额度语义。 |
| 外部发奖系统 | 只接收 outbox 发送的标准发奖指令并回调结果；本需求不改变其职责。 |

## 输入输出

### 输入

标准事件：

- `eventId`、`eventType`、`sourceSystem`、`tenantId`。
- `subject.type`、`subject.id`。
- `biz.bizType`、`biz.bizId`。
- `occurredAt`：带时区 RFC3339 时间，用于活动有效期和 DAILY_LIMIT 活动本地日计算。
- `attributes`：业务扩展字段，可包含 `riskTag`、`riskTags`、`riskLevel`、`ip`、`deviceId`、`amount` 等。IP/Device 本轮只作为普通字段供规则读取，不作为 quota scope 执行。

Header：

- `Idempotency-Key = tenantId:sourceSystem:eventType:bizType:bizId`。

活动和规则配置：

- 活动字段：`tenantId`、`activityCode`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`。
- 规则字段：`MATCH`、`QUALIFY`、`REWARD`、`RISK`、`LIMIT` 阶段的 `CONDITION` 规则。
- `RISK_LIMIT_BUDGET` 是运行解释阶段名；持久化 `rulePhase` 仍只允许 `RISK` 和 `LIMIT`，不得配置 `BUDGET` 或 `RISK_LIMIT_BUDGET` 作为 rule phase。

Quota 配置：

- `BUDGET + ACTIVITY + TOTAL`：活动总预算。
- `STOCK + ACTIVITY + rewardCode`：单个奖励库存。
- `USER_LIMIT + USER + subject.id`：用户在当前 activity 下的总参与次数。
- `DAILY_LIMIT + USER + subject.id:activityLocalDate`：用户在当前 activity、活动 timezone 自然日下的参与次数；`activityLocalDate` 格式默认 `YYYY-MM-DD`。
- 缺少对应 quota 行表示不启用该限制，用于兼容旧活动。

### 成功输出

- 非重复事件且当前 activity 所有限制通过时，生成 `REWARDABLE` participation、`PENDING` grant_order、`PENDING` outbox。
- 同一 activity 内 participation、quota 扣减、grant_order、outbox 必须保持原子一致。
- 完全重复事件返回 `200 OK`、`duplicate=true`，不得新增 participation、grant_order、outbox，也不得重复扣 quota。

### 失败输出

- `RISK` 或 `LIMIT` 阶段 `CONDITION` 规则拒绝：写当前 activity 的 `REJECTED` participation，`reject_reason` 使用稳定原因码，不生成 grant_order/outbox，不扣 quota。
- 当前事件互斥落选活动：写 `REJECTED` participation 或等价可审计解释，`reject_reason=EXCLUSIVE_GROUP_LOST`，不生成 grant_order/outbox，不扣 quota。
- `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 不足：当前 activity 发奖链路整体失败，返回 `409 QUOTA_EXCEEDED`，不生成 grant_order/outbox，不保留 `REWARDABLE` 脏记录，写 `QUOTA_EXCEEDED` 审计。
- 幂等键复用但 payload 不一致：返回 `400 INVALID_IDEMPOTENCY_KEY`，不得进入活动发奖链路。

## 业务规则

### 执行顺序

固定业务顺序：

```text
MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET -> QUOTA_RESERVE -> GRANT_ORDER -> OUTBOX
```

说明：

- `RISK_LIMIT_BUDGET` 聚合执行已配置的 `RISK`、`LIMIT` 规则，并展示预算/库存/次数影响解释。
- `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 是 quota 类型，通过 quota 原子扣减表达，不作为 `rulePhase`。
- 奖励计算结果为无可发奖励时，不进入 quota 扣减和 grant_order/outbox。

### RISK 风控限制

- RISK 阶段使用 `CONDITION` 规则表达，不内置真实风控判定。
- 可判断字段仅来自标准事件、`attributes.*`、`facts.*`、`state.*`，不得连接外部风控库，不得引用外部业务代码。
- 默认最小口径：风控标签、风控等级由事件 `attributes.riskTag`、`attributes.riskTags`、`attributes.riskLevel` 或已配置 fact provider 提供。
- 规则依赖字段缺失、类型不匹配或 fact provider 不可用时，不得默认通过；该规则应按规则引擎既有语义拒绝或报错。
- RISK 规则失败是活动级拒绝，不扣 quota，不返回 `QUOTA_EXCEEDED`。拒绝原因使用 `RISK_RULE_REJECTED:<ruleCode>` 或等价可定位 reason code。

### LIMIT 次数和周期

- 本轮 LIMIT 的 quota-backed 执行范围只包括 `USER_LIMIT` 和 `DAILY_LIMIT`。
- `USER_LIMIT + USER + subject.id`：每个成功产生发奖资格的 participation 默认消耗 `1`。
- `DAILY_LIMIT + USER + subject.id:activityLocalDate`：每个成功产生发奖资格的 participation 默认消耗 `1`；`activityLocalDate` 按活动 `timezone` 从事件 `occurredAt` 计算。
- `USER_LIMIT` 和 `DAILY_LIMIT` 任一不足时，当前 activity 整体失败，返回 `409 QUOTA_EXCEEDED`，body 中用 `quotaType` 区分。
- 未配置对应 quota 行表示不限制；不得因为缺少 `USER_LIMIT` 或 `DAILY_LIMIT` quota 行而拒绝旧活动。
- `LIMIT` 阶段 `CONDITION` 规则仍可读取 `attributes.ip`、`attributes.deviceId` 等字段做普通条件判断，但这不是 IP/Device quota 扣减。

### 互斥

本轮互斥只处理同一事件内的候选活动裁剪：

- 互斥输入集合为当前标准事件在 MATCH 阶段通过基础检查和规则后得到的候选活动。
- 只在同一 `tenantId`、同一 `subject.type/id`、同一非空 `exclusiveGroup` 内裁剪。
- `exclusiveGroup` 为空的活动不参与互斥。
- 同一非空 `exclusiveGroup` 内全部候选 `stackable=true` 时，全部保留。
- 同一非空 `exclusiveGroup` 内存在任一 `stackable=false` 候选时，该组进入单选模式，只保留排序第一的活动；排序默认 `priority DESC, createdAt ASC, activityId ASC`，不得随机。
- 落选活动写 `EXCLUSIVE_GROUP_LOST` 解释，不生成 grant_order/outbox，不扣 quota。
- 不查询历史 participation，不判断“用户曾经参加 A 后不能参加 B”，不要求跨事件并发互斥锁。

### BUDGET 预算和 STOCK 库存

- `BUDGET + ACTIVITY + TOTAL`：本次事件同一 activity 的所有金额类 `reward_amount` 合计作为预算消耗。
- 非金额 reward 的 `reward_amount=0.0000` 不代表不发奖；只要权益数量字段有效，仍需生成 grant_order/outbox，并按 rewardCode 消耗库存。
- `STOCK + ACTIVITY + rewardCode`：每个 enabled 且实际发放的 reward 默认消耗库存 `1`。
- 金额 reward 计算结果为 `0.0000` 时按无可发奖励处理，不生成 grant_order/outbox，不扣 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 同一 activity 下多个 reward 任一预算或库存不足时，本次 activity 整体失败，不允许部分发奖、部分扣减。

### 幂等

- 事件幂等键固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- participation 幂等键固定为 `tenantId:activityId:eventInbox.idempotency_key`。
- grant_order 幂等键固定为 `participation.idempotency_key:rewardCode`。
- 完全重复事件不重复创建 participation、grant_order、outbox，也不重复扣 quota。
- 同 key 不同 payload 必须拒绝，不能用后到 payload 覆盖已处理事件。

### 审计

- 事件进入写 `EVENT_IN` 审计。
- quota 创建写 `CREATE_QUOTA` 审计。
- quota-backed 限制不足写 `QUOTA_EXCEEDED` 审计。
- 最小审计字段必须包含：`activityId`、`eventInboxId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason`、`error`。
- RISK 规则拒绝和互斥落选应能通过 participation snapshot/reject_reason 或 audit 查询排查。
- 审计不得记录完整 Authorization header、token、secret 或敏感 payload。

## 边界条件

- 活动未配置 quota：该 quota 类型不限制。
- 活动配置多个 reward：预算按金额 reward 汇总，库存按 rewardCode 分别扣减，任一不足则全部回滚。
- 非金额 reward：`reward_amount=0.0000` 但权益数量有效时仍需要库存检查和发奖。
- 全部奖励均无实际发放：写 `QUALIFIED` 或等价非发奖状态，不生成 grant_order/outbox，不扣 quota。
- 多活动命中：互斥组内按同一事件裁剪；非互斥活动可分别进入 RISK/LIMIT/BUDGET。
- 跨租户：quota、participation、grant_order、audit 均按 tenant 隔离。
- 时间：DAILY_LIMIT 必须使用事件 `occurredAt` 和活动 `timezone` 计算活动本地日期，不能使用服务器本地日期。
- 并发：同一 quota 在并发事件下不得超发、超预算或超库存。
- 模拟运行：不得写 event_inbox、participation、grant_order、outbox 或扣 quota，只输出 RISK/LIMIT/BUDGET 预览和风险解释。

## 异常场景

| 场景 | 最小结果 |
|---|---|
| 事件字段缺失、未知顶层字段、时间无时区、业务字段放错位置 | `400 INVALID_EVENT`，不得进入 quota 扣减。 |
| 缺少 Idempotency-Key、Header 与 payload 不匹配、同 key payload 不一致 | `400 INVALID_IDEMPOTENCY_KEY`，不得进入发奖链路。 |
| RISK 条件不通过 | 当前 activity `REJECTED`，`reject_reason=RISK_RULE_REJECTED:<ruleCode>`，不扣 quota，不生成 grant_order/outbox。 |
| LIMIT CONDITION 条件不通过 | 当前 activity `REJECTED`，`reject_reason=LIMIT_RULE_REJECTED:<ruleCode>`，不扣 quota，不生成 grant_order/outbox。 |
| 当前事件互斥落选 | `REJECTED/EXCLUSIVE_GROUP_LOST`，不扣 quota，不生成 grant_order/outbox。 |
| `USER_LIMIT` 不足 | `409 QUOTA_EXCEEDED`，`quotaType=USER_LIMIT`。 |
| `DAILY_LIMIT` 不足 | `409 QUOTA_EXCEEDED`，`quotaType=DAILY_LIMIT`。 |
| `BUDGET` 不足 | `409 QUOTA_EXCEEDED`，`quotaType=BUDGET`。 |
| `STOCK` 不足 | `409 QUOTA_EXCEEDED`，`quotaType=STOCK`。 |
| 非预期内部错误 | 不得生成 grant_order/outbox；如已进入 activity 事务，必须回滚。 |

结构化 quota 错误最小字段：

```json
{
  "code": "QUOTA_EXCEEDED",
  "error": "quota exceeded: STOCK: ACTIVITY: FIRST_DEPOSIT_RATIO",
  "quotaType": "STOCK",
  "scope": "ACTIVITY",
  "scopeKey": "FIRST_DEPOSIT_RATIO"
}
```

如系统同时保留 `errorCode/message` 别名，必须与 `code/error` 表达同一错误；本需求验收以 `code=QUOTA_EXCEEDED`、HTTP `409` 和 quota 定位字段为准。

## 非目标范围

- 不实现真实风控系统，不调用外部风控数据库。
- 不连接外部业务数据库，不引用外部业务代码包。
- 不直接修改用户余额，不绕过 grant_order/outbox 发奖。
- 不新增业务专用 controller、业务专用表或接入方硬编码字段。
- 不实现 IP / Device quota 次数限制；`IP`、`DEVICE` scope 的原子扣减、审计和验收归入后续 Phase 3。
- 不实现跨历史 participation 的活动期互斥，例如“用户已参加 A 后整个活动期不能参加 B”。
- 不实现周、月、滚动 N 小时或自定义周期；本轮周期最小口径为活动 timezone 自然日。
- 不实现完整预算冻结、确认、释放生命周期；本轮按发奖前原子扣减和失败回滚验收，完整冻结模型按后续 quota reservation / callback 任务处理。
- 不实现 MQ 发奖、人工导出、人工发奖状态机。
- 不要求本轮改动后台 UI，但 API、模拟运行和审计结果必须能支撑排查。

## 风险

| 风险 | 等级 | 说明与约束 |
|---|---|---|
| 额度超发 | P0 | 预算、库存和次数是账务安全边界，并发下必须使用原子额度语义，不能读后写。 |
| 部分发奖 | P0 | 多 reward 任一 quota 不足时必须整体失败，否则会出现账务和体验不一致。 |
| 错误码漂移 | P1 | 本轮固定 quota-backed 失败统一 `409 QUOTA_EXCEEDED`，避免调用方和测试断言分叉。 |
| 互斥范围扩大 | P1 | 若误做历史 participation 互斥，会引入查询范围、并发保护和运营语义争议；本轮明确不做。 |
| DAILY_LIMIT 日期误判 | P1 | 必须按活动 timezone 和事件 `occurredAt` 计算自然日，不使用服务器本地时间。 |
| IP/Device 范围扩张 | P2 | 设计文档支持 IP/设备作为未来限制，但 roadmap 列为 Phase 3；本轮不纳入 quota 扣减。 |
| RISK 字段缺失默认通过 | P2 | 风控规则依赖字段缺失不得静默放行，应输出可排查拒绝或错误。 |

## 验收标准

功能验收：

- 创建含 RISK 条件的活动，事件命中黑名单标签或风险等级时：当前 activity 写 `REJECTED` participation 或等价拒绝结果，`reject_reason` 可定位规则，不生成 grant_order/outbox，不扣任何 quota。
- 创建含 LIMIT CONDITION 的活动，条件不通过时：当前 activity 拒绝，不生成 grant_order/outbox，不扣 quota。
- 创建含 `USER_LIMIT + USER + subject.id` 的活动，同一用户超过次数后：后续事件返回 `409 QUOTA_EXCEEDED`，body.code 为 `QUOTA_EXCEEDED`，`quotaType=USER_LIMIT`，不生成 grant_order/outbox，写 `QUOTA_EXCEEDED` 审计。
- 创建含 `DAILY_LIMIT + USER + subject.id:YYYY-MM-DD` 的活动，同一用户在活动 timezone 同一自然日超过日限后：后续事件返回 `409 QUOTA_EXCEEDED`，`quotaType=DAILY_LIMIT`；跨到下一活动自然日且存在下一日 quota 行时可重新计数。
- 缺少 `USER_LIMIT` 或 `DAILY_LIMIT` quota 行时，活动不得因该限制失败，按无限制处理。
- 创建含 `exclusiveGroup` 且组内存在 `stackable=false` 的两个活动，同一事件同时命中时：仅排序第一活动进入发奖链路，其余活动输出 `EXCLUSIVE_GROUP_LOST`，不生成 grant_order/outbox，不扣 quota。
- 用户先通过事件 E1 参加活动 A，再通过事件 E2 命中同 exclusiveGroup 的活动 B：本轮不得因历史 participation 自动拒绝 B；除非 E2 同时命中 A/B 并在当前事件候选裁剪中落选。
- 创建含 BUDGET 的活动，剩余额度小于本次所有金额 reward 合计时：返回 `409 QUOTA_EXCEEDED`，`quotaType=BUDGET`，不生成任何 grant_order/outbox，不保留 `REWARDABLE` participation。
- 创建含 STOCK 的活动，某 reward 库存不足时：该 activity 本次所有 reward 均不发放，不生成部分 grant_order/outbox，其他 quota used_value 不变。
- 非金额 reward 数量有效且库存充足时，即使 `reward_amount=0.0000` 也能生成 grant_order/outbox，并扣减对应 `STOCK`。
- 金额 reward 结果为 `0.0000` 且无其他实际 reward 时，不生成 grant_order/outbox，不扣 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 完全重复事件返回 `200 OK`、`duplicate=true`，不重复扣减 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。
- 同幂等键不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`，不进入额度扣减。
- 模拟运行不产生任何 quota 扣减、participation、grant_order、outbox 副作用，但需要展示 RISK/LIMIT/BUDGET 阶段解释结果和 quota 风险预览。
- PostgreSQL 场景下并发触发同一预算、库存、用户次数或用户日次数，最终 `used_value` 不得超过 `total_value`。

审计与错误验收：

- 所有 quota-backed 失败均返回 HTTP `409`，body.code 固定为 `QUOTA_EXCEEDED`；不得返回 `LIMIT_EXCEEDED`。
- quota 失败响应必须包含 `quotaType`、`scope`、`scopeKey`；`error` 或 `message` 必须能定位失败额度。
- `QUOTA_EXCEEDED` 审计 payload 至少包含 `activityId`、`eventInboxId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason`、`error`。
- RISK 规则拒绝、LIMIT CONDITION 拒绝和互斥落选必须能通过 participation snapshot/reject_reason 或审计解释排查。
- 审计和错误响应不得包含完整 Authorization header、token、secret 或敏感 payload。

范围验收：

- 本轮测试不得要求 `IP` 或 `DEVICE` scope quota 扣减、超限、缺失字段错误或审计断言。
- 本轮测试不得要求跨历史 participation 的活动期互斥。
- 本轮不得新增 `LIMIT_EXCEEDED` 对外错误码。

## 待确认问题

无阻塞项。以下均已给出默认最小可验收口径，不影响进入 Review：

- IP / Device 次数限制：默认不在本轮交付，后续 Phase 3 单独定义 `IP`、`DEVICE` scope 的字段缺失、代理 IP 可信链、并发扣减、审计和错误码。
- 跨事件生命周期级互斥：默认不在本轮交付，后续如需要“用户参加 A 后整个活动期不能参加 B”，需单独定义历史 participation 查询范围、有效状态、并发保护和迁移策略。
- 更复杂周期：默认只做活动 timezone 自然日 `DAILY_LIMIT + USER`，周/月/滚动窗口后续单独定义。
- 预算冻结/确认/释放：默认本轮只验收发奖前原子扣减和失败回滚；完整 reservation 与 callback 释放按后续任务处理。

## Score

```text
需求清晰度        19/20
范围可控          19/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              93/100
```

## Decision

```text
PASS
```

需求：2026-05-24-risk-limit-budget
