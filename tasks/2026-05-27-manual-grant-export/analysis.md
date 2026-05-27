# Analysis

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未设计具体代码改法，未修改业务代码，未提交。

已读取：

- `tasks/2026-05-27-manual-grant-export/review.md`
- `tasks/2026-05-27-manual-grant-export/analysis.md`
- `tasks/2026-05-27-manual-grant-export/task.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/api.md`
- `docs/operations.md`
- `tasks/2026-05-24-grant-order/analysis.md`

未发现独立 `PRD/` 目录。本分析以任务说明、现有架构 / API / 数据模型 / 运维文档、上游 grant order 状态机分析和 Review 打回意见为依据。

## Review 反馈处理

- P1：上一版把 quota reservation 终结按单张 grant order 统一描述为 `FROZEN -> CONFIRMED/RELEASED`，没有区分 reward 维度 reservation 与 participation 维度 reservation。现有架构明确 `USER_LIMIT` / `DAILY_LIMIT` 是 participation-level reservation，必须基于同一 participation 下所有 grant order 的聚合结果一次性终结。
  - 本轮修正：明确 reservation 粒度。`BUDGET`、`STOCK` 是 reward 维度，单张 grant order 终态只终结该 reward 关联的 `BUDGET` / `STOCK` reservation；`USER_LIMIT`、`DAILY_LIMIT` 是 participation 维度，任何单张 grant order 的 `SUCCESS` / `FAILED` / `CLOSED` 都不得单独确认或释放用户次数 / 日次数。
  - 聚合默认口径：同一 participation 下所有 grant order 都进入终态后，才一次性处理 `USER_LIMIT` / `DAILY_LIMIT`。若至少一张 grant order 为 `SUCCESS`，则 participation-level reservation 确认为 `CONFIRMED`；若全部 grant order 都是 `FAILED` / `CLOSED` 且没有成功发奖，则释放为 `RELEASED`。仍存在 `PENDING` / `SENDING` / `RETRY` 时保持 `FROZEN`。
  - 验收补充：多 reward participation 中先成功一张、失败一张、关闭一张时，`BUDGET` / `STOCK` 随各自 reward 终结，`USER_LIMIT` / `DAILY_LIMIT` 只在最后一张进入终态后按聚合结果移动一次。
- P2：上一版说明人工单不自动超时释放，但没有明确既有 quota timeout-release / 其他超时释放入口是否必须排除 `MANUAL` / `MANUAL_EXPORT`。
  - 本轮修正：明确 `MANUAL` / `MANUAL_EXPORT` grant order 只能通过本需求定义的人工确认成功、人工确认失败、人工关闭、导出文件关闭未终结单据来终结。既有自动投递、callback、DLQ、quota timeout-release 等流程默认不得误处理人工发奖单。
  - 默认最小口径：除非请求显式提供“人工发奖处理范围”且操作语义是本需求定义的人工失败确认或关闭，否则通用 timeout-release 必须跳过 `MANUAL` / `MANUAL_EXPORT`。
- P2：上一版导出文件锁定与文件生成失败的原子性不够具体，没有规定候选锁定、导出明细创建、`exportId` 回写、状态推进、文件落盘 / checksum 的边界。
  - 本轮修正：补充导出事务与文件生成默认流程。导出创建先锁定候选并创建导出记录 / 明细 / grant order 关联，文件未 READY 前不得把 grant order 置为 `SENDING`；文件落盘和 checksum 成功后导出文件进入 `READY`，再将明细内 grant order 推进到 `SENDING`。文件生成失败时导出文件进入 `FAILED`，保留导出记录、筛选快照、失败原因和已锁定明细用于审计，但候选 grant order 不进入终态且可重新导出。
  - 重复导出口径：处于 `CREATED` / `GENERATING` / `READY` / `CLOSED` 且未作废的导出明细会阻止同一 grant order 被其他导出重复纳入；`FAILED` 导出明细只作为审计历史，不阻止该 grant order 后续重新导出。
- Open Question：`USER_LIMIT` / `DAILY_LIMIT` 在多 grant order participation 中的聚合终结条件是什么？
  - 已关闭为默认最小可验收口径：`USER_LIMIT` / `DAILY_LIMIT` 只按 participation 聚合终结。只要该 participation 至少存在一个成功发奖的 grant order，用户次数 / 日次数确认消耗；若该 participation 的所有 grant order 最终均失败或关闭，则释放用户次数 / 日次数。
- Review Score `78`、Decision `BLOCK`。
  - 本轮已补齐导致 BLOCK 的 P1 与两个 P2，重新评分为 `92`，Decision 改为 `PASS`。

## Analysis 自检修正

- 上一轮 `analysis.md` 给出 `Score=90` 和 `Decision=PASS`，但对 quota reservation 粒度存在错误假设：把所有 reservation 都按单张 grant order 终结。
  - 已修正为两层规则：reward-level `BUDGET` / `STOCK` 随单张 grant order 终结；participation-level `USER_LIMIT` / `DAILY_LIMIT` 按 participation 下所有 grant order 聚合后一次性终结。
- 上一轮扣分项没有覆盖导出原子性，导致文件生成失败时可能既无法审计，又可能重复导出或错误推进状态。
  - 已补充导出创建、文件生成、READY、FAILED、重导、审计保留和重复纳入阻断规则。
- 上一轮待确认“长期未处理人工发奖单是否自动超时释放”只给出“不自动释放”，没有约束既有 timeout-release 入口。
  - 已修正为：通用 timeout-release、自动投递、callback、DLQ 流程默认排除人工发奖单；人工单只能由本需求定义的人工动作终结。
- 上一轮错误风险：`MANUAL_EXPORT` 导出后逐单确认与一个 participation 多 reward 并存时，可能提前释放或确认 `USER_LIMIT` / `DAILY_LIMIT`。
  - 已补充多 reward、多导出、多终态混合场景下的聚合验收标准，关闭该风险。

## 背景目标

ExMarketing 当前链路已经具备标准事件接入、活动匹配、规则计算、quota 冻结、participation、grant order、outbox、callback、对账和审计能力。现有文档中 `MANUAL` / `MANUAL_EXPORT` 尚无完整状态机，活动创建阶段会被拒绝。

本需求目标是补齐人工发奖与导出发奖的基础闭环：

- `MANUAL`：系统生成 participation、quota reservation 和 grant order 后，不自动投递外部奖励命令，由运营 / 财务 / 客服等人工处理方线下完成发奖，并在系统中确认成功、确认失败或关闭。
- `MANUAL_EXPORT`：系统生成待导出的 grant order，由人工处理方创建可追踪导出文件，交给外部或线下流程处理，再按单据确认成功、确认失败或关闭，也可关闭导出文件内未终结单据。
- 两种模式都必须保留 participation、grant_order、quota、audit 和 reconciliation 链路，不允许绕过事件和规则直接补单。
- 导出文件必须可追踪，可从文件追溯到 grant order，也可从 grant order 追溯到导出文件。
- 人工终结必须幂等，不能重复确认 quota、重复释放 quota、提前终结 participation-level reservation 或反向覆盖终态。

## 参与方

- 外部事件上报方：提交标准事件，触发活动规则。
- ExMarketing 活动引擎：匹配活动、执行规则、计算奖励、冻结 quota、创建 participation 和 grant order。
- 运营 / 财务 / 客服等人工处理方：查看待处理人工发奖单、创建 / 下载导出文件、线下发奖、回填成功 / 失败 / 关闭结果。
- 外部线下发奖系统：可选接收导出文件或人工录入数据；本需求不要求 ExMarketing 与其自动联调。
- 对账 / 审计使用方：按活动、用户、事件、grant order、导出文件和审计时间线排查结果。
- Review / Dev / Test Agent：依据本分析进行审核、开发和测试。

## 发奖模式定义

### `MANUAL`

- 活动创建允许 `grantMode=MANUAL`。
- `grantChannel` 对 `MANUAL` 不是自动投递通道，默认可为空或固定为 `MANUAL` 语义值。
- `rewardConfig.targetRef` 对 `MANUAL` 不要求外部 URL / MQ destination；如填写，只作为人工处理说明或外部系统引用，不参与自动投递。
- 正式事件命中奖励后生成 participation、quota reservation、grant order，不生成 reward command outbox。
- grant order 初始为 `PENDING`，可人工确认 `SUCCESS` / `FAILED` 或关闭为 `CLOSED`。

### `MANUAL_EXPORT`

- 活动创建允许 `grantMode=MANUAL_EXPORT`。
- `grantChannel` 对 `MANUAL_EXPORT` 不是自动投递通道，默认可为空或固定为 `MANUAL_EXPORT` / `FILE` 语义值。
- `rewardConfig.targetRef` 对 `MANUAL_EXPORT` 不要求自动投递地址；如填写，只作为导出目的说明。
- 正式事件命中奖励后生成 participation、quota reservation、grant order，不生成自动投递 outbox。
- grant order 初始为 `PENDING`，导出文件 READY 后进入 `SENDING`，表示“已交付人工 / 文件流程处理中”，不表示外部已成功。

## 输入

- 活动奖励配置：
  - `grantMode`: `MANUAL` 或 `MANUAL_EXPORT`。
  - `rewardCode`、`rewardType`、`calculateType`、`rewardConfig`、`flowMultiple`、`enabled`、`version` 沿用现有奖励配置规则。
- 标准事件：
  - `tenantId`、`eventId`、`eventType`、`sourceSystem`、`subject`、`biz`、`occurredAt`、`attributes`。
  - `Idempotency-Key` 固定为 `tenantId:sourceSystem:eventType:bizType:bizId`。
- 人工确认 / 关闭输入：
  - `tenantId` 必填。
  - `grantOrderId`、`billNo` / `commandId`、`idempotencyKey` 至少一种定位方式。
  - 多个定位字段同时提供时必须指向同一租户下同一 grant order。
  - `operator`、`reason` 必填。
  - 确认结果为 `SUCCESS` 或 `FAILED`；关闭操作使用 `CLOSED` 语义。
  - `externalGrantId` 成功确认时可选；若线下系统提供流水号则必须记录。
  - `finishedAt` 可选，不传时默认使用服务端处理时间。
- 导出创建输入：
  - `tenantId`、`operator`、`reason` 必填。
  - 筛选条件至少包含一个业务范围：`activityId`、`rewardCode`、`subjectType + subjectId`、`createdFrom/createdTo`、`limit` 或明确的 grant order 列表。
  - 默认只允许选择 `grantMode=MANUAL_EXPORT` 且状态为 `PENDING` / 可重导 `RETRY` 的 grant order。
  - 文件格式默认 `CSV`，字段顺序和内容必须稳定。

## 输出

- `marketing_grant_order`：
  - `grant_status` 覆盖人工 / 导出流程状态。
  - `bill_no` / `commandId` 作为发奖单唯一指令号，用于人工确认、导出文件和对账。
  - `idempotency_key` 仍为 `participation.idempotency_key + ":" + rewardCode`。
  - `reward_payload_json` 必须包含 `rewardCode`、`rewardType`、`calculateType`、`currency` / `unit`、`flowMultiple`、奖励解释字段，以及人工 / 导出展示字段。
  - `error_message` 记录最近失败、关闭或导出异常摘要。
- 导出文件 / 批次记录：
  - 默认业务模型名为 “manual grant export file”，具体存储载体由后续实现选择，但必须具备查询、审计和对账能力。
  - 必备字段：`tenantId`、`exportId`、`fileName`、`fileFormat`、`status`、`operator`、`reason`、`filterSnapshot`、`rowCount`、`fileChecksum`、`storageRef` 或下载引用、`createdAt`、`updatedAt`。
  - 必须存在 grant order 明细关联，明细至少包含 `exportId`、`grantOrderId`、`billNo`、导出行号、导出时 grant order 状态、明细状态。
  - 文件状态默认：`CREATED`、`GENERATING`、`READY`、`FAILED`、`CLOSED`。同步生成可从 `CREATED` 直接到 `READY`，但语义上仍必须满足原子边界。
- 导出文件内容：
  - `tenantId`、`exportId`、`billNo`、`grantOrderId`、`idempotencyKey`、`activityId`、`participationId`、`subjectType`、`subjectId`、`bizType`、`bizId`、`rewardCode`、`rewardType`、`rewardAmount`、`currency`、`rewardPayload`、`grantStatus`、`createdAt`。
- 审计记录：
  - 人工确认成功、人工确认失败、人工关闭、创建导出、导出 READY、导出生成失败、导出关闭都必须写审计。
  - 审计 payload 必须包含 `tenantId`、`operator`、`reason`、`grantOrderId` / `billNo`、`exportId`、前后状态、quota reservation 变更摘要和 source 摘要。
  - 审计不得记录 Authorization、Cookie、token、secret 或完整敏感 payload。
- 对账查询：
  - 现有 reward reconciliation 结果需能展示人工发奖状态、导出文件引用、quota reservation、participation 和审计时间线。

## 业务规则

### 创建活动配置

- `MANUAL` / `MANUAL_EXPORT` 应成为可创建的发奖模式，不再因“尚未实现”被拒绝。
- 奖励类型、计算方式、Decimal 语义、金额 4 位小数、非金额权益 payload、`flowMultiple` 限制、0 金额跳过等规则沿用现有文档。
- `MANUAL` / `MANUAL_EXPORT` 不需要自动投递 `targetRef`。如果现有 schema 必须保留该字段，默认按可选说明字段处理，不校验为 URL 或 MQ URI。
- 未审批、审批版本不一致或非 `ACTIVE` 活动仍不得在正式事件处理中生成 participation、grant order 或导出候选。

### 事件处理与链路完整性

- 人工发奖和导出发奖必须走完整事件链路：event inbox、活动匹配、规则执行、奖励计算、participation、quota reservation、grant order。
- 不允许提供绕过 participation / grant_order / quota 的“直接导入发奖名单并确认成功”能力。
- `REWARDABLE` participation、quota reservation 和 grant order 必须在同一个 activity 级事务中完成。
- quota 不足时整笔 activity 级事务回滚，不保留 `REWARDABLE` participation、grant order、导出候选或 quota 脏数据。
- `MANUAL` / `MANUAL_EXPORT` 不生成自动 reward command outbox，不由 outbox worker 自动发送，不进入自动 `DLQ`。

### grant order 状态机

人工 / 导出发奖沿用现有 grant order 状态枚举：

```text
PENDING
SENDING
SUCCESS
FAILED
RETRY
CLOSED
```

默认状态语义：

| 状态 | `MANUAL` 含义 | `MANUAL_EXPORT` 含义 | 是否终态 |
|---|---|---|---|
| `PENDING` | 等待人工处理 | 等待纳入导出文件 | 否 |
| `SENDING` | 人工已领取或处理中 | 已导出，等待线下处理结果 | 否 |
| `SUCCESS` | 人工确认发奖成功 | 文件内该单已确认发奖成功 | 是 |
| `FAILED` | 人工确认发奖失败 | 文件内该单已确认发奖失败 | 是 |
| `RETRY` | 人工处理异常后待再次处理 | 导出失败或文件作废后待重新导出 / 处理 | 否 |
| `CLOSED` | 人工确认不再发奖并关闭 | 导出后确认不再处理并关闭 | 是 |

默认合法流转：

```text
CREATE -> PENDING
PENDING -> SENDING
PENDING -> SUCCESS
PENDING -> FAILED
PENDING -> CLOSED
PENDING -> RETRY
RETRY -> SENDING
RETRY -> SUCCESS
RETRY -> FAILED
RETRY -> CLOSED
SENDING -> SUCCESS
SENDING -> FAILED
SENDING -> RETRY
SENDING -> CLOSED
```

禁止流转：

```text
SUCCESS -> any other status
FAILED -> SUCCESS
FAILED -> SENDING
FAILED -> RETRY
CLOSED -> any other status
```

### quota reservation 粒度与终结规则

必须区分两类 reservation：

- Reward-level reservation：
  - `BUDGET`：金额预算，冻结金额等于对应 grant order 的 `reward_amount`。
  - `STOCK`：奖品库存，`scopeKey=rewardCode`，每个 enabled reward 默认冻结 1。
  - 终结粒度：单张 grant order。该 grant order 首次进入 `SUCCESS` 时确认对应 `BUDGET` / `STOCK`；首次进入 `FAILED` / `CLOSED` 时释放对应 `BUDGET` / `STOCK`。
- Participation-level reservation：
  - `USER_LIMIT`：用户参与次数，单个 rewardable participation 默认冻结 1。
  - `DAILY_LIMIT`：用户每日次数，单个 rewardable participation 默认冻结 1。
  - 终结粒度：同一 participation 下所有 grant order 的聚合结果。单张 grant order 的成功、失败或关闭不得单独移动 `USER_LIMIT` / `DAILY_LIMIT`。

Participation-level 聚合默认口径：

- 只要该 participation 下仍存在 `PENDING` / `SENDING` / `RETRY` grant order，`USER_LIMIT` / `DAILY_LIMIT` reservation 保持 `FROZEN`。
- 当该 participation 下所有 grant order 都进入 `SUCCESS` / `FAILED` / `CLOSED` 后，一次性终结 `USER_LIMIT` / `DAILY_LIMIT`。
- 若至少一张 grant order 为 `SUCCESS`，则 participation-level reservation 进入 `CONFIRMED`，表示本次参与已经成功消耗用户次数 / 日次数。
- 若所有 grant order 均为 `FAILED` / `CLOSED`，且没有任何成功发奖，则 participation-level reservation 进入 `RELEASED`。
- 聚合终结必须幂等，只能移动一次；后续重复确认同终态不得二次确认或释放。
- 完全未配置 quota 的不限额活动可以没有对应 reservation，确认 / 失败 / 关闭仍可终结 grant order 并写审计。

### 人工确认规则

- 人工确认必须按 `tenantId` 隔离定位 grant order，不允许跨 tenant 更新。
- 多个定位条件同时提供时必须指向同一 grant order，否则返回定位冲突。
- 首次确认 `SUCCESS`：
  - grant order 进入 `SUCCESS`。
  - 该 reward 维度的 `BUDGET` / `STOCK` reservation 从 `FROZEN` 进入 `CONFIRMED`。
  - 若同一 participation 下所有 grant order 均已终态，则按聚合规则处理 `USER_LIMIT` / `DAILY_LIMIT`。
- 首次确认 `FAILED`：
  - grant order 进入 `FAILED`。
  - 该 reward 维度的 `BUDGET` / `STOCK` reservation 从 `FROZEN` 进入 `RELEASED`。
  - 若同一 participation 下所有 grant order 均已终态，则按聚合规则处理 `USER_LIMIT` / `DAILY_LIMIT`。
- 关闭 `CLOSED`：
  - grant order 进入 `CLOSED`。
  - 该 reward 维度的 `BUDGET` / `STOCK` reservation 从 `FROZEN` 进入 `RELEASED`。
  - 若同一 participation 下所有 grant order 均已终态，则按聚合规则处理 `USER_LIMIT` / `DAILY_LIMIT`。
  - 关闭原因必须写入审计，用于区分“不再处理”和“发奖失败”。
- participation 状态聚合：
  - 仍有未终态 grant order 时，participation 不得被错误终结为最终成功或失败。
  - 所有关联 grant order 终态后，若至少一张 `SUCCESS`，默认 participation 可进入 `GRANTED`；若全部 `FAILED` / `CLOSED`，默认 participation 可进入 `FAILED` 或等价未发放终态。

### 幂等规则

- 同一标准事件重复上报且 payload 相同，不新增 participation、grant order 或导出候选。
- 同一 `participation.idempotency_key + ":" + rewardCode` 只能生成一张 grant order。
- 重复人工确认相同终态返回 duplicate / already_done，不重复移动 quota。
- 已 `SUCCESS` 后确认 `FAILED` 或 `CLOSED`，必须返回冲突。
- 已 `FAILED` 后确认 `SUCCESS`，必须返回冲突。
- 已 `CLOSED` 后确认 `SUCCESS` 或 `FAILED`，必须返回冲突。
- 同一 grant order 在未失败 / 未作废的导出文件明细中已存在时，不得被另一个导出文件重复纳入。
- 已 `SUCCESS`、`FAILED`、`CLOSED` 的 grant order 不得被新导出文件纳入。

### 导出文件规则

- 导出只允许包含 `grantMode=MANUAL_EXPORT` 且状态为 `PENDING` 或按重导规则允许的 `RETRY` grant order。
- `MANUAL` grant order 不得进入导出文件。
- 导出创建必须记录筛选条件快照，避免事后无法解释文件包含范围。
- 导出创建默认事务边界：
  - 在同一租户内按筛选条件锁定候选 grant order。
  - 跳过或报告已终态、非 `MANUAL_EXPORT`、已在未失败导出明细中的 grant order。
  - 创建导出文件记录，状态为 `CREATED` 或 `GENERATING`。
  - 创建导出明细，记录每个候选 grant order、行号和导出时快照。
  - 关联 grant order 与 `exportId` 用于对账，但在文件未 READY 前 grant order 保持 `PENDING` / `RETRY`，不得提前进入 `SENDING`。
- 文件生成成功默认顺序：
  - 根据导出明细生成稳定文件内容。
  - 成功落盘或形成稳定可读 `storageRef` 后计算 `rowCount` 和 `fileChecksum`。
  - 导出文件状态进入 `READY`。
  - 导出明细内仍未终结且仍属于该导出的 grant order 进入 `SENDING`。
  - 写 `MANUAL_EXPORT_READY` 审计。
- 文件生成失败默认顺序：
  - 导出文件状态进入 `FAILED`，记录失败原因。
  - 保留导出文件记录、筛选快照、失败原因和已创建明细作为审计历史。
  - 未交付文件的 grant order 不进入终态；若此前仍为 `PENDING` 则保持 `PENDING`，若来自可重导 `RETRY` 则保持 `RETRY`。
  - `BUDGET` / `STOCK` / `USER_LIMIT` / `DAILY_LIMIT` reservation 均保持 `FROZEN`，等待重新导出、人工失败确认或关闭。
  - `FAILED` 导出明细不阻止相关 grant order 后续重新导出。
- 文件内容必须稳定、可重复校验：
  - 同一 `READY` 导出记录下载多次，内容、`rowCount` 和 `fileChecksum` 一致。
  - 文件中每行对应一个 grant order。
  - 金额保留 4 位小数。
  - 非金额权益通过 `rewardPayload` 表达，不把 `reward_amount=0.0000` 误认为无奖励。
- 导出文件关闭：
  - 只关闭文件本身不自动代表所有 grant order 成功。
  - 默认最小可验收口径：提供按导出文件关闭未终结单据的能力，作用于该文件中仍为 `SENDING` / `RETRY` 的 grant order，将其置为 `CLOSED`，按 reservation 粒度释放或聚合终结 quota，并写逐单审计。
  - 已 `SUCCESS` / `FAILED` / `CLOSED` 的单据保持不变。

### 与自动流程的隔离

- `MANUAL` / `MANUAL_EXPORT` 不生成自动 outbox，因此自动 outbox worker 不应 claim 或投递这些 grant order。
- `/openapi/v1/reward/callback` 默认只处理自动投递奖励结果，不作为人工发奖单终结入口；若后续允许 callback 处理人工单，必须另立需求明确鉴权、来源和状态语义。
- 通用 quota timeout-release 默认不得把 `MANUAL` / `MANUAL_EXPORT` 的长期 `PENDING` / `SENDING` / `RETRY` 单自动标记为 `FAILED` 或释放 quota。
- 只有本需求定义的人工确认失败、人工关闭、导出文件关闭未终结单据可以终结人工单并释放相关 quota。
- 对账查询可以展示长期未处理人工单，但不自动改写其状态。

### 审计与对账

- 以下动作必须审计：
  - `MANUAL_GRANT_CONFIRM_SUCCESS`
  - `MANUAL_GRANT_CONFIRM_FAILED`
  - `MANUAL_GRANT_CLOSE`
  - `MANUAL_EXPORT_CREATE`
  - `MANUAL_EXPORT_READY`
  - `MANUAL_EXPORT_FAILED`
  - `MANUAL_EXPORT_CLOSE`
  - `MANUAL_QUOTA_PARTICIPATION_FINALIZE`
- action 名称可使用等价稳定枚举，但必须能区分成功确认、失败确认、关闭、导出创建、导出生成失败、导出关闭和 participation-level quota 聚合终结。
- 对账查询必须能回答：
  - 某个 `billNo` 当前是否待人工处理、已导出、已成功、已失败或已关闭。
  - 某个导出文件包含哪些 grant order，每个 grant order 当前终态是什么。
  - 某个 grant order 的 reward-level reservation 是否仍 `FROZEN`、已 `CONFIRMED` 或已 `RELEASED`。
  - 某个 participation 的 `USER_LIMIT` / `DAILY_LIMIT` 是否仍 `FROZEN`、已 `CONFIRMED` 或已 `RELEASED`，以及其聚合依据。
  - 某个人工确认或关闭由谁操作、何时操作、理由是什么。

## 边界条件

- 一个 participation 命中多个 reward 时，每个 rewardCode 最多一张 grant order；不同 rewardCode 可分别人工确认或导出确认。
- 任一 reward 的 quota 不足时，该 activity 本次 rewardable 链路整体失败，不生成部分人工 grant order。
- 多 reward participation 中，单张 grant order 终态不得提前确认或释放 `USER_LIMIT` / `DAILY_LIMIT`。
- 缺少 quota 行表示无限制；确认成功 / 失败 / 关闭时不移动缺失 quota，但仍更新 grant order 和审计。
- 金额类 `BONUS` / `CASH` 结果为 `0.0000` 且无其他实际权益时，不生成 grant order，不进入人工或导出流程。
- 非金额类奖励只要权益数量为正，即使 `reward_amount=0.0000` 也生成 grant order，可人工确认或导出确认。
- 模拟运行只展示人工 / 导出发奖预览，不写真实 grant order，不冻结 quota，不创建导出文件。
- 内存仓储可做快速验证；涉及唯一约束、并发、quota reservation 和导出文件一致性的验收必须以 PostgreSQL 或等价持久化仓储为准。
- 人工确认和外部 reward callback 是不同入口。默认最小口径：`MANUAL` / `MANUAL_EXPORT` 不依赖 `/openapi/v1/reward/callback`，但对账语义与 callback 终态一致。

## 异常场景

- 活动配置仍使用 `MANUAL` / `MANUAL_EXPORT` 但缺少有效 reward 配置：创建活动按现有 reward 校验拒绝。
- 人工确认定位不到 grant order：返回 `GRANT_ORDER_NOT_FOUND` 或等价机器可识别错误。
- 多定位字段指向不同 grant order：返回 locator conflict。
- tenant 不匹配：返回 not found 或 tenant forbidden，不跨租户暴露数据。
- grant order 当前为 `SUCCESS`、`FAILED`、`CLOSED`，再次确认相反终态：返回状态冲突，不改写状态。
- reward-level reservation 已被释放但人工确认成功：返回 quota reservation conflict，不得把已释放 quota 强行确认。
- participation-level reservation 已因聚合确认但又尝试关闭剩余非终态单据：必须返回状态冲突，因为该场景说明聚合规则或数据状态已不一致。
- 导出筛选结果为空：默认拒绝创建空文件并返回 `NO_EXPORTABLE_GRANT_ORDER`。
- 导出过程中有并发确认或关闭：导出只纳入创建时成功锁定 / 选中的未终结 grant order；已终态单据跳过并在结果中报告。
- 同一 grant order 被两个导出请求并发选择：只能进入一个未失败导出文件，另一个请求应跳过或返回冲突。
- 文件生成失败：记录 `MANUAL_EXPORT_FAILED` 审计，grant order 不进入终态，quota 保持冻结，导出记录与明细保留为失败审计历史。
- 导出文件已关闭后再次下载：允许下载历史文件用于对账；不得重新推进 grant order 状态。
- 导出文件已关闭后再次关闭：返回 duplicate / already_closed。
- 人工处理方遗漏长期 `PENDING` / `SENDING`：系统应可通过对账查询发现；本需求不要求自动超时释放人工发奖单。
- 通用 timeout-release 扫描到人工单：默认跳过，不更新状态，不移动 quota，并可在 dry-run 结果中标明“manual grant excluded”。

## 非目标范围

- 不实现真实外部发奖系统、银行 / 钱包 / 优惠券平台集成。
- 不实现导出文件上传回执解析、自动批量导入成功 / 失败结果；本需求只要求人工确认 / 关闭和导出追踪。
- 不实现复杂审批流、双人复核、四眼原则或工单系统。
- 不改变现有 AUTO + HTTP / WEBHOOK / MQ outbox 投递和 callback 语义。
- 不允许人工创建绕过事件、规则、participation、grant order、quota 的任意发奖单。
- 不要求导出文件长期对象存储、加密存储、预签名 URL 或权限体系；只要求本地 / 后台可追踪、可下载或可读取的最小文件引用。
- 不定义最终 UI 交互样式；仅要求管理侧能力和可验证行为。
- 不处理已经 `FAILED` 或 `CLOSED` 的同一 grant order 补发；补发必须另立补偿需求。
- 不把通用 quota timeout-release 扩展为人工发奖自动失败处理器；人工单超时策略另立需求。

## 风险点

- quota 风险高：`BUDGET` / `STOCK` 与 `USER_LIMIT` / `DAILY_LIMIT` 粒度不同，若按单张 grant order 统一移动，会导致用户次数 / 日次数提前消耗或错误释放。
- 并发风险高：重复事件、重复确认、并发导出、导出与确认并发都可能造成同一 grant order 多次处理。
- 对账风险高：导出文件如果没有稳定 `exportId`、`fileChecksum`、筛选快照和明细关联，失败后无法证明线下处理范围。
- 操作风险中：人工确认失败和关闭都可能释放 quota，但业务含义不同，审计必须区分。
- 隐私风险中：导出文件可能包含用户主体、业务 ID 和 reward payload；审计与文件不应包含 token、secret、Authorization 或不必要敏感字段。
- 产品边界风险中：人工发奖容易被误解为“手工补单”。本需求明确不允许绕过事件和规则直接补发。

## 验收标准

- 活动创建可接受 `grantMode=MANUAL` 和 `grantMode=MANUAL_EXPORT`，并继续拒绝未知 grantMode。
- 符合规则的事件命中 `MANUAL` reward 后，生成 event inbox、participation、quota reservation、grant order；不生成自动 reward command outbox。
- 符合规则的事件命中 `MANUAL_EXPORT` reward 后，生成 event inbox、participation、quota reservation、grant order；不生成自动 reward command outbox。
- 人工发奖不绕过 participation、grant_order、quota；quota 不足时不留下 `REWARDABLE` participation 或 grant order。
- `MANUAL` grant order 初始为 `PENDING`，可人工确认 `SUCCESS`、确认 `FAILED` 或关闭 `CLOSED`。
- `MANUAL_EXPORT` grant order 初始为 `PENDING`，导出文件 READY 后进入 `SENDING`，并可逐单确认 `SUCCESS`、确认 `FAILED` 或关闭 `CLOSED`。
- 单 reward 场景：`SUCCESS` 确认会确认该 grant order 的 `BUDGET` / `STOCK` reservation，并在 participation 下所有 grant order 已终态时按聚合结果确认或释放 `USER_LIMIT` / `DAILY_LIMIT`。
- 单 reward 场景：`FAILED` / `CLOSED` 会释放该 grant order 的 `BUDGET` / `STOCK` reservation，并在 participation 下所有 grant order 已终态时按聚合结果释放 `USER_LIMIT` / `DAILY_LIMIT`。
- 多 reward 场景：第一张 grant order `SUCCESS` 后，只确认该 reward 的 `BUDGET` / `STOCK`；`USER_LIMIT` / `DAILY_LIMIT` 仍保持 `FROZEN`，直到同一 participation 其他 grant order 也终态。
- 多 reward 场景：若一张 `SUCCESS`、其余 `FAILED` / `CLOSED`，最后一张终态后 `USER_LIMIT` / `DAILY_LIMIT` 一次性 `CONFIRMED`。
- 多 reward 场景：若全部 grant order 都 `FAILED` / `CLOSED`，最后一张终态后 `USER_LIMIT` / `DAILY_LIMIT` 一次性 `RELEASED`。
- 多 reward 场景：`USER_LIMIT` / `DAILY_LIMIT` 聚合终结幂等，重复确认任一单据不得重复移动 participation-level reservation。
- 已 `SUCCESS`、`FAILED`、`CLOSED` 的 grant order 不允许被相反终态覆盖。
- 导出文件有稳定 `exportId`、`fileName`、`fileFormat`、`status`、`rowCount`、`fileChecksum`、筛选快照和 grant order 明细关联。
- 导出文件未 READY 前，相关 grant order 不得提前进入 `SENDING`。
- 文件生成成功后，导出文件进入 `READY`，文件 checksum / rowCount 固化，明细内未终结 grant order 进入 `SENDING`。
- 文件生成失败后，导出文件进入 `FAILED`，保留导出记录和明细用于审计，grant order 保持 `PENDING` / `RETRY`，quota 保持 `FROZEN`，且可重新导出。
- 同一 grant order 不会同时出现在两个未失败 / 未作废的导出文件中；`FAILED` 导出历史不阻止重新导出。
- 导出文件内容字段完整，金额 4 位小数，非金额权益在 `rewardPayload` 中可对账。
- 按导出文件关闭未终结单据时，只影响该文件内仍未终结的 grant order，已成功 / 已失败 / 已关闭单据保持不变，并按 quota 粒度移动 reservation。
- 通用 quota timeout-release、自动投递 worker、callback、DLQ replay / close 默认不处理 `MANUAL` / `MANUAL_EXPORT` grant order。
- 长期 `PENDING` / `SENDING` / `RETRY` 人工单只能通过人工确认失败、人工关闭或导出文件关闭未终结单据终结；不会被自动超时释放误改为 `FAILED`。
- 所有人工确认、关闭、导出创建、导出 READY、导出失败、导出关闭和 participation-level quota 聚合终结均写审计。
- 对账查询可展示 grant order、participation、reward-level reservation、participation-level reservation、导出文件引用和审计时间线。
- PostgreSQL 或等价持久化验收覆盖唯一约束、事务回滚、并发导出、重复确认幂等、quota reservation 粒度联动和导出文件失败重导；不能只以内存仓储作为资金链路验收依据。
- 研发交付最低验证命令：`go test ./...`、`go build ./...`。

## 待确认问题

- 是否需要双人复核或审批后才能人工确认成功？
  - 默认最小口径：不需要。本任务只要求 `operator`、`reason` 和审计；双人复核另立需求。
- 导出文件是否必须使用对象存储并提供过期下载链接？
  - 默认最小口径：不要求。只要具备稳定文件引用、checksum、明细关联和可下载 / 可读取能力即可。
- 成功确认时 `externalGrantId` 是否必填？
  - 默认最小口径：可选。若线下系统提供流水号必须记录；没有流水号时以 `billNo`、operator、reason 和审计作为对账依据。
- 是否支持导入回执批量确认？
  - 默认最小口径：不支持。本任务只要求人工确认 / 关闭；回执导入另立需求。
- 长期未处理的人工发奖单是否自动超时释放？
  - 默认最小口径：不自动释放。通用 timeout-release 默认跳过人工单；通过对账查询发现后由人工确认失败或关闭处理。
- 多 reward participation 下，部分 reward 成功、部分 reward 失败时用户次数是否应消耗？
  - 默认最小口径：应消耗。只要该 participation 至少一张 grant order 成功，`USER_LIMIT` / `DAILY_LIMIT` 在所有 grant order 终态后确认消耗。

以上问题均已有默认最小可验收口径，不阻塞研发和测试。

## Score

```text
需求清晰度        19/20
业务规则完整      19/20
边界异常清楚      18/20
验收标准明确      18/20
约束一致性        18/20
总分              92/100
```

扣分原因：导出文件最终存储形态、双人复核、回执导入和长期人工单运营策略仍可在后续产品任务深化；但本轮已给出默认最小可验收口径，且已关闭 Review 关于 quota reservation 粒度、人工单超时流程隔离、导出文件原子性的 BLOCK 风险。

## Decision

```text
PASS
```

PASS 原因：Score 为 92/100；现有材料足以确定业务目标、验收对象和参与方。`MANUAL` / `MANUAL_EXPORT` 的参与链路、状态机、输入输出、quota reservation 粒度、导出文件事务边界、人工确认 / 关闭、幂等、审计、异常和验收标准均已形成可审核、可开发、可测试口径。

需求：2026-05-27-manual-grant-export
