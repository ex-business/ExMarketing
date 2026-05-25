# Analysis

## 需求 ID

2026-05-24-113400-5

## 标题

参与记录与发奖单生命周期

## 资料范围

已读取并用于本次分析：

```text
tasks/2026-05-24-113400-5/task.md
README.md
CLAUDE.md
docs/architecture.md
docs/data-model.md
docs/api.md
docs/workflow.md
docs/roadmap.md
docs/operations.md
requirements/backlog.json
tasks/2026-05-24-113400-4/analysis.md
tasks/2026-05-24-094535/analysis.md
tasks/2026-05-24-outbox-worker-dlq/analysis.md
tasks/2026-05-24-113400-6/analysis.md
```

说明：

```text
仓库内未发现 PRD/ 目录。
设计来源在 CLAUDE.md 中指向 22-独立活动营销中台设计；本次使用仓库内已落地的 docs 和 backlog 作为可访问的业务规则来源。
本次只做需求分析和需求细化；未读取业务代码，未修改代码，未提交。
```

## 背景目标

ExMarketing 是独立活动营销中台，不直接修改用户余额，只通过标准事件、活动匹配、规则判断、奖励计算、额度控制、参与记录、发奖单和 Outbox 与外部奖励系统集成。

本任务位于核心链路：

```text
Event Intake
  -> Idempotency
  -> Campaign Match
  -> Rule Evaluation
  -> Reward Calculation
  -> Quota / Budget / Inventory
  -> Participation
  -> Grant Order
  -> Outbox
  -> External Reward System
```

目标是把 `marketing_participation` 和 `marketing_grant_order` 定义为发奖链路的强制账务凭证：

```text
所有进入发奖链路的活动命中，必须先形成 participation 快照，再生成 grant_order。
系统禁止绕过 participation、grant_order 和 outbox 直接发奖。
参与状态必须能解释规则通过、规则拒绝、可发奖、已发奖、失败等生命周期节点。
发奖单状态必须能解释待发送、发送中、成功、失败、重试、关闭等生命周期节点。
幂等键必须唯一，重复事件不得重复参与、重复发奖或重复扣减额度。
失败原因必须可追踪到 participation、grant_order、event_inbox、audit 或 outbox 中的稳定原因。
```

## 参与方

```text
外部业务系统
  上报标准事件，提供 tenant、subject、biz、attributes 和 Idempotency-Key。

运营 / 活动配置方
  配置活动、规则、奖励、预算、库存、用户次数限制，并需要查询参与和发奖结果。

ExMarketing 事件处理链路
  接收事件、做幂等、匹配活动、执行规则、计算奖励、创建 participation 和 grant_order。

Quota / Budget / Inventory 能力
  在发奖单生成前完成活动预算、奖励库存、用户次数等资源校验和扣减。

Outbox 投递能力
  基于 grant_order 生成发奖指令，负责后续 HTTP/Webhook/MQ 等投递和重试。

外部发奖系统
  接收 Outbox 发奖命令，返回发奖结果；不允许 ExMarketing 直接加钱。

审计 / 对账人员
  通过 event_inbox、participation、grant_order、outbox、audit 追踪命中、拒绝、失败和重复事件。
```

## 输入

标准事件输入：

```text
eventId
eventType
sourceSystem
tenantId
subject.type
subject.id
biz.bizType
biz.bizId
occurredAt
attributes
Idempotency-Key header
```

活动与奖励输入：

```text
activity.id / tenantId / activityCode / status / triggerEvents / priority / exclusiveGroup / stackable / version
rules and ruleResults
rewards: rewardCode / rewardType / calculateType / rewardConfig / grantMode / grantChannel / enabled / version
quota: BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT
```

幂等键输入：

```text
event inbox idempotency key:
tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId

participation idempotency key:
必须在 tenant、activity、event 或业务主体维度唯一，确保同一事件同一活动只生成一条 participation。

grant_order idempotency key:
必须在 tenant、participation、reward 或 rewardCode 维度唯一，确保同一 participation 的同一奖励只生成一张发奖单。
```

`participation` 和 `grant_order` 的具体幂等键拼接格式由开发阶段在不破坏上述唯一性的前提下固定，并同步文档；不得依赖数据库自增 ID、当前时间或随机数表达幂等语义。

## 输出

事件处理输出应能稳定表达：

```text
是否重复事件 duplicate
命中的 participation 列表
生成的 grant_order 列表
生成的 outbox 发奖指令列表
规则拒绝原因
quota 不足原因
非预期失败原因
```

`marketing_participation` 必须记录：

```text
tenantId
activityId
eventInboxId
subjectType / subjectId
bizType / bizId
status
rejectReason
snapshotJson
idempotencyKey
createdAt / updatedAt
```

`snapshotJson` 业务要求：

```text
记录命中时活动配置版本、关键活动字段、规则结果摘要、奖励计算摘要、事件摘要和必要上下文。
快照用于事后解释，不应保存 secret、token、完整 Authorization header 或无关外部业务大对象。
快照必须能支持“为什么命中 / 为什么拒绝 / 按什么配置生成奖励”的基本排查。
```

`marketing_grant_order` 必须记录：

```text
tenantId
activityId
participationId
subjectType / subjectId
rewardType
rewardAmount
rewardPayloadJson
billNo / commandId
grantStatus
grantChannel
retryCount
errorMessage
idempotencyKey
createdAt / updatedAt
```

`rewardPayloadJson` 业务要求：

```text
包含 rewardCode、rewardType、calculateType、currency、flowMultiple、targetRef 或必要发奖目标引用。
包含奖励计算解释摘要，便于发奖系统、对账和人工排查。
不得包含无关敏感信息。
```

## 业务规则

### 总体顺序

标准事件进入正式处理后，业务顺序为：

```text
1. 校验事件模型和 Idempotency-Key。
2. 写入 marketing_event_inbox，重复事件直接返回 duplicate，不再进入活动处理。
3. 匹配可用活动。
4. 按 MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET 执行规则。
5. 规则拒绝时写 REJECTED participation，并保留 rejectReason。
6. 规则通过后计算全部 enabled reward。
7. 存在实际奖励时，进入 activity 级发奖事务。
8. 在同一 activity 级事务内写 REWARDABLE participation、扣 BUDGET/STOCK/USER_LIMIT、写 grant_order、写 outbox。
9. quota 不足时回滚 participation/grant_order/outbox，写可追踪失败原因和 QUOTA_EXCEEDED 审计。
```

### Participation 生命周期

允许状态：

```text
INIT
QUALIFIED
REJECTED
REWARDABLE
GRANTED
FAILED
```

状态语义：

```text
INIT
  已创建参与草稿或初始记录，但尚未完成规则资格确认。若当前链路没有持久化中间态，可不对外暴露 INIT 记录；但状态枚举必须保留。

QUALIFIED
  活动匹配和资格规则已通过，但尚未形成可发奖结果。若当前链路把规则通过后直接推进到 REWARDABLE，可不持久化 QUALIFIED 中间态；但模拟和排查需要能解释“规则已通过”。

REJECTED
  活动匹配后规则、互斥、风控、资格、奖励计算前置条件等未通过。必须记录 rejectReason，不生成 grant_order、不生成 outbox、不扣 quota。

REWARDABLE
  规则通过、奖励计算成功、存在至少一个实际奖励，并已完成发奖前必要 quota 校验和扣减。REWARDABLE participation 必须与 grant_order/outbox 在同一 activity 级事务内形成。

GRANTED
  外部发奖系统已确认成功后的最终参与状态。该状态由发奖回调任务负责推进，本任务只要求生命周期兼容，不实现回调确认逻辑。

FAILED
  发奖链路最终失败或人工关闭后参与失败。该状态由回调、DLQ、人工关闭或后续补偿任务推进，本任务只要求状态可承载失败原因。
```

状态流转约束：

```text
INIT -> QUALIFIED -> REWARDABLE -> GRANTED
INIT -> QUALIFIED -> REWARDABLE -> FAILED
INIT -> REJECTED
QUALIFIED -> REJECTED
REWARDABLE -> FAILED
```

禁止流转：

```text
REJECTED -> REWARDABLE
REJECTED -> GRANTED
FAILED -> GRANTED，除非后续人工重放需求定义明确补偿流程
GRANTED -> FAILED，除非后续对账冲正需求定义明确逆向流程
```

### Grant Order 生命周期

允许状态：

```text
PENDING
SENDING
SUCCESS
FAILED
RETRY
CLOSED
```

状态语义：

```text
PENDING
  发奖单已生成，等待 Outbox 投递或发送任务处理。

SENDING
  发奖指令正在投递或已被发送任务 claim。

SUCCESS
  外部发奖系统确认成功，发奖单完成。

FAILED
  单次或最终发送失败；若仍可重试，应进入 RETRY 或由 outbox 表表达重试调度。

RETRY
  等待下一次重试，不应生成新的 grant_order。

CLOSED
  人工关闭、超过重试策略关闭或业务确认不再发放。关闭必须保留原因。
```

状态流转约束：

```text
PENDING -> SENDING -> SUCCESS
PENDING -> SENDING -> FAILED -> RETRY -> SENDING
PENDING -> CLOSED
FAILED -> CLOSED
RETRY -> CLOSED
```

禁止流转：

```text
SUCCESS -> SENDING
SUCCESS -> RETRY
SUCCESS -> CLOSED，除非后续冲正/撤销需求明确允许
CLOSED -> SENDING，除非后续人工重放需求明确重新打开规则
```

### Participation 与 Grant Order 关系

```text
一条 grant_order 必须且只能归属于一条 participation。
一条 participation 可以没有 grant_order，前提是状态为 REJECTED，或全部奖励为 0 且按“无实际奖励”跳过。
一条 REWARDABLE participation 必须至少关联一条实际 grant_order。
grant_order 不得脱离 participation 单独创建。
outbox 发奖指令不得脱离 grant_order 单独创建。
同一 participation 下多奖励活动可以生成多张 grant_order；任一奖励创建失败时，同一 activity 本次发奖链路必须整体失败，不得部分发奖。
```

### 幂等规则

```text
重复标准事件不得重复写 participation、grant_order、outbox 或重复扣减 quota。
同一 Idempotency-Key 且 payload 完全一致，应返回 duplicate=true 的稳定响应。
同一 Idempotency-Key 但 payload 不一致，应返回 INVALID_IDEMPOTENCY_KEY，不得继续发奖。
participation.idempotency_key 必须唯一，用于兜底防止同一事件同一活动重复参与。
grant_order.idempotency_key 必须唯一，用于兜底防止同一参与记录同一奖励重复发奖。
billNo / commandId 必须稳定可追踪，外部回调应能通过 tenantId + commandId 定位唯一发奖单。
```

### 失败原因追踪

失败原因必须至少覆盖：

```text
RULE_REJECTED
REWARD_CALCULATION_FAILED
ZERO_REWARD_SKIPPED
QUOTA_EXCEEDED
GRANT_ORDER_CREATE_FAILED
OUTBOX_CREATE_FAILED
IDEMPOTENCY_CONFLICT
UNEXPECTED_ERROR
```

记录位置要求：

```text
规则拒绝：participation.rejectReason。
quota 不足：event_inbox.errorMessage 或响应错误 + marketing_audit 的 QUOTA_EXCEEDED payload。
发奖单发送失败：grant_order.errorMessage 和 outbox.errorMessage。
最终参与失败：participation.status=FAILED，并保留可读失败原因。
重复事件：event_inbox 或响应中可识别 duplicate，不应伪装成失败。
```

## 边界条件

```text
无匹配活动：事件可处理完成，不生成 participation/grant_order/outbox。
活动匹配但规则拒绝：写 REJECTED participation，不生成 grant_order/outbox，不扣 quota。
规则通过但全部奖励计算为 0：不生成 grant_order/outbox，不扣 quota；是否写 participation 需按当前文档口径作为 skipped/rejected 可解释结果，不得写 REWARDABLE 脏记录。
多奖励活动：所有奖励的 grant_order/outbox/quota 必须 all-or-nothing。
缺少 BUDGET/STOCK/USER_LIMIT quota 行：按现有文档可视为无限制，除非配置明确要求必须存在。
STOCK quota 的 scopeKey：按 rewardCode 识别，每个实际发奖的 enabled reward 默认扣 1。
跨活动命中：事务边界为单个 activity；本任务不要求跨多个 activity 做全局事务。
模拟运行：只能输出 participation/grant_order/outbox 预览，不得写真实记录。
内存仓储仅可用于本地快速验证；涉及事务语义的验收必须以 PostgreSQL 行为为准。
```

## 异常场景

```text
事件模型非法
  返回 INVALID_EVENT，不写 participation/grant_order/outbox。

Idempotency-Key 缺失或与 payload 不一致
  返回 INVALID_IDEMPOTENCY_KEY，不写 participation/grant_order/outbox。

完全重复事件
  返回 duplicate=true，不新增 participation/grant_order/outbox，不重复扣 quota。

规则执行失败或字段缺失导致不通过
  写 REJECTED participation 和 rejectReason，不生成发奖单。

奖励配置非法或奖励计算失败
  不生成 grant_order/outbox，不扣 quota，失败原因必须可追踪。

预算不足
  返回 QUOTA_EXCEEDED，不保留 REWARDABLE participation，不生成 grant_order/outbox，写审计。

库存不足
  返回 QUOTA_EXCEEDED，不生成任何部分 grant_order/outbox，已尝试的同活动扣减必须回滚。

用户次数不足
  返回 QUOTA_EXCEEDED，不生成 grant_order/outbox，不扣预算或库存。

grant_order 创建失败
  同一 activity 的 participation、quota、outbox 均不得部分成功。

outbox 创建失败
  同一 activity 的 participation、quota、grant_order 均不得部分成功。

外部发奖系统发送失败
  属于 Outbox/回调任务后续处理；本任务只要求 grant_order 能承载 FAILED/RETRY/CLOSED 状态和 errorMessage。
```

## 非目标范围

本任务不包含：

```text
不实现外部钱包加钱、扣钱或用户余额修改。
不新增业务专用 controller、model、job 或外部业务库连接。
不设计具体代码改法，不限定 repository/API 的内部实现形式。
不实现 Outbox worker 的 claim、指数退避、DLQ 重放或人工关闭页面。
不实现发奖回调后的预算确认/释放完整流程。
不实现跨活动全局事务。
不实现复杂审批、对账后台、人工补发、冲正、撤销。
不改变奖励计算引擎的计算口径。
不改变标准事件 Schema 和幂等键口径。
```

## 风险

```text
账务风险：若允许绕过 participation 或 grant_order 直接发奖，将无法对账和追踪。
幂等风险：若 participation/grant_order 幂等键不唯一，重复事件或并发请求可能重复发奖。
事务风险：若 REWARDABLE participation、quota、grant_order、outbox 不一致，会出现“显示可发奖但无发奖单”或“有发奖单但无参与快照”。
状态风险：若状态流转未受控，SUCCESS/CLOSED/FAILED 之间可能被错误覆盖。
失败追踪风险：若 rejectReason/errorMessage/audit 缺失，运营无法解释用户为什么未获奖或为什么重复事件未处理。
范围膨胀风险：本任务容易扩到 Outbox 重试、回调确认、预算释放和对账后台；这些已有独立任务，应保持边界。
兼容风险：现有文档把缺失 quota 行视为无限制，本任务不应突然要求所有活动必须配置 quota。
```

## 验收标准

功能验收：

```text
规则拒绝的活动命中会生成 REJECTED participation，包含 rejectReason，不生成 grant_order/outbox。
规则通过且存在实际奖励时，先形成 participation，再形成 grant_order；任何 grant_order 都能追溯到 participation。
REWARDABLE participation 必须关联至少一张 grant_order。
grant_order 必须包含 rewardType、rewardAmount、rewardPayloadJson、billNo/commandId、grantStatus、grantChannel、idempotencyKey。
outbox 发奖指令必须来源于 grant_order，禁止直接从 reward 计算结果绕过 grant_order 发奖。
重复事件不会新增 participation、grant_order、outbox，也不会重复扣 quota。
同一 Idempotency-Key 但 payload 不一致返回 INVALID_IDEMPOTENCY_KEY。
多奖励活动任一 grant_order/outbox/quota 失败时，同一 activity 不得产生部分成功。
quota 不足时返回 QUOTA_EXCEEDED，不保留 REWARDABLE participation，不生成 grant_order/outbox，并能查询到失败原因。
发奖单状态至少覆盖 PENDING/SENDING/SUCCESS/FAILED/RETRY/CLOSED，参与状态至少覆盖 INIT/QUALIFIED/REJECTED/REWARDABLE/GRANTED/FAILED。
```

文档和可观测验收：

```text
docs/data-model.md 中 participation 和 grant_order 状态语义与本分析一致。
docs/api.md 中事件处理流程、重复事件响应、quota 不足响应和发奖链路说明与本分析一致。
README 或运维文档能说明系统不直接发奖，只生成 grant_order 和 outbox command。
失败原因能从 participation.rejectReason、grant_order.errorMessage、event_inbox.errorMessage、outbox.errorMessage 或 audit 中至少一处查到。
```

测试验收：

```text
go test ./...
go build ./...
涉及 PostgreSQL 事务语义时，必须使用 Docker PostgreSQL 验证。
```

必须覆盖的行为用例：

```text
1. happy path：有效事件 -> participation(REWARDABLE) -> grant_order(PENDING) -> outbox。
2. rule rejected：有效事件但规则不通过 -> participation(REJECTED)，无 grant_order/outbox。
3. duplicate event：重复请求 -> duplicate=true，无新增 participation/grant_order/outbox。
4. idempotency conflict：同 key 不同 payload -> INVALID_IDEMPOTENCY_KEY。
5. quota exceeded budget：预算不足 -> QUOTA_EXCEEDED，无 REWARDABLE participation/grant_order/outbox。
6. quota exceeded stock：某 reward 库存不足 -> 整个 activity 无部分 grant_order/outbox。
7. quota exceeded user limit：用户次数不足 -> 不扣预算/库存，不生成发奖单。
8. multi reward all-or-nothing：多个奖励中任一失败，不允许部分发奖。
9. zero reward：奖励结果为 0 时不生成 grant_order/outbox，不扣 quota，不写 REWARDABLE 脏记录。
10. grant order traceability：任一 grant_order 可通过 participationId 追溯 eventInboxId、subject、biz、activity 和 reward payload。
```

## 待确认问题

以下问题不阻塞进入开发，但需要在 Dev/Review 阶段保持文档一致：

```text
1. 规则通过但全部奖励为 0 时，是否需要保留一条非 REWARDABLE participation 作为“已参与但无奖励”的运营记录？当前 docs 口径倾向不生成发奖链路，不能写 REWARDABLE 脏记录。
2. participation.idempotency_key 和 grant_order.idempotency_key 的最终拼接格式需固定到文档，业务要求是唯一、稳定、可追踪。
3. grant_order.FAILED 与 outbox.DLQ/FAILED 的边界需由后续 Outbox/Callback 任务最终统一；本任务只要求状态可承载。
```

## 评分

```text
需求清晰度        18 / 20
范围可控          18 / 20
风险可控          17 / 20
验收标准明确      18 / 20
架构一致性        19 / 20
总分              90 / 100
```

## Decision

```text
PASS
```

理由：

```text
需求目标、数据对象、状态枚举、处理顺序、幂等约束、失败追踪和验收用例已足够明确。
剩余待确认问题不影响核心账务安全边界，也不需要用代码实现假设替代需求澄清。
```

需求：2026-05-24-113400-5
