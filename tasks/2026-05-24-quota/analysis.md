# Analysis

## 需求 ID

2026-05-24-quota

## 标题

Quota 乐观锁原子扣减

## Review 反馈处理

本轮读取了 `tasks/2026-05-24-quota/review.md`。该文件仍为模板态：

- `P0/P1/P2`：未填写具体问题，无需逐项修复。
- `Open Questions`：仅写“待填写”，未提出实际开放问题。
- `Decision`：仍为 `PASS / BLOCK` 占位，不构成明确打回原因。

处理结论：本次不是基于具体 Review finding 的返工，但仍按返工要求补充完整需求背景、业务规则、边界、异常、验收标准和评分，确保后续 Review Agent 可以基于明确口径审核。

## Analysis 自检修正

上一轮 `analysis.md` 是未完成模板，存在以下 BLOCK 原因：

- `现状 / 目标理解 / 方案 / 改动范围 / 风险点 / 验收方式` 均为“待填写”：本轮已改为完整需求分析，并补齐参与方、输入输出、业务规则、边界条件、异常场景、非目标范围、风险与验收标准。
- `Score = 0`：原因是需求未细化，低于 80 不允许进入研发。本轮按评分规则重新评估为 88。
- `Decision = PASS / BLOCK` 占位：本轮基于最小可验收口径给出 `PASS`。
- 隐含错误假设：上一轮 Analysis 指令要求读取相关代码，但本轮用户明确要求“不需要读取代码，不允许用代码实现细节替代需求澄清”。本轮只读取任务、README、CLAUDE、架构、API、数据模型、运维/路线图和设计文档，不读取业务实现代码。

旧 BLOCK 关闭依据：现有材料没有对“扣减”唯一限定为直接写 `used_value` 还是先写 `frozen_value`。结合 `docs/api.md` 的发奖回调规则、`docs/data-model.md` 的 `frozen_value` 字段和路线图中的“预算冻结/确认/释放模型”，本轮采用默认最小可验收口径：事件命中且需要发奖时先原子冻结 quota，回调成功再确认到 used，回调失败释放冻结。该口径能同时满足“不超预算”和“发奖失败不造成预算永久漂移”。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、执行活动规则、生成参与记录、发奖单和 Outbox 指令，不直接修改外部用户余额。预算、库存、用户次数等通用限制统一由 `marketing_quota` 承接。

本需求目标是把活动发奖链路中的 quota 占用变成数据库层面的原子乐观锁语义，覆盖：

- `total_value`：总额度上限。
- `used_value`：已确认消耗额度。
- `frozen_value`：已预留但等待发奖最终态确认的额度。
- `version`：quota 聚合版本，用于并发更新冲突识别和可审计递增。

核心业务目标：

- 并发事件同时命中同一 quota 时，任意 quota 的 `used_value + frozen_value` 不得超过 `total_value`。
- quota 不足或并发抢占失败时，本次活动命中不得留下 `REWARDABLE` participation、`grant_order` 或 `outbox` 脏数据。
- 发奖成功后冻结额度转为已使用；发奖失败后冻结额度释放；重复回调不得重复确认或重复释放。

## 参与方

- 外部事件系统：通过 OpenAPI 上报标准事件和 `Idempotency-Key`。
- ExMarketing Engine：匹配活动、执行规则、计算奖励、编排 quota、参与记录、发奖单和 Outbox。
- PostgreSQL 仓储：提供 quota 原子条件更新和 activity 级事务一致性。
- 外部发奖系统：接收 Outbox 发奖指令，并通过 callback 回传 `SUCCESS` 或 `FAILED`。
- 运营 / 管理后台：创建活动、创建 quota、查看审计和对账。
- Test / Review Agent：验证并发、事务回滚、幂等和回调确认/释放语义。

## 输入输出

### 输入

- 活动配置：活动、规则、奖励、发奖通道和目标地址。
- Quota 配置：`activityId`、`quotaType`、`scope`、`scopeKey`、`totalValue`。
- 标准事件：`tenantId`、`eventType`、`sourceSystem`、`subject`、`biz`、`occurredAt`、`attributes`。
- 幂等键：`tenantId:sourceSystem:eventType:bizType:bizId`。
- 发奖回调：`tenantId`、`commandId`、`status`、`externalGrantId`、`finishedAt`。

### 正常输出

- 事件处理成功时，按活动生成 participation、grant_order、outbox。
- quota 预留成功时，对应 quota 的 `frozen_value` 增加、`version` 递增，`used_value` 不在事件处理阶段增加。
- 发奖回调 `SUCCESS` 时，相关冻结额度转为已使用：`frozen_value` 减少、`used_value` 增加、`version` 递增。
- 发奖回调 `FAILED` 时，相关冻结额度释放：`frozen_value` 减少、`used_value` 不变、`version` 递增。
- 对账查询可看到 grant_order、outbox、quota reservation 与审计时间线。

### 失败输出

- quota 不足返回 `409 QUOTA_EXCEEDED`，包含 `quotaType`、`scope`、`scopeKey`。
- 重复事件返回 `200 OK` 且 `duplicate=true`，不得重复扣 quota、创建 grant_order 或创建 outbox。
- 同一幂等键但 payload 不一致返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 回调冲突、缺失 reservation 或 quota 聚合不一致返回机器可识别 409 错误。

## 业务规则

1. 额度可用性统一按 `used_value + frozen_value + 本次预留量 <= total_value` 判断。
2. 任意导致 `used_value + frozen_value > total_value` 的预留必须失败，不能依赖应用层读后写判断。
3. 每次成功预留、确认或释放 quota，都必须递增 `version`。
4. `total_value` 是运营配置的总上限。本需求不要求新增运营调整接口，但任何运行时占用都必须以当前 `total_value` 为上限。
5. Quota 类型默认口径：
   - `BUDGET + ACTIVITY + TOTAL`：本次同一 activity 所有实际发奖金额合计。
   - `STOCK + ACTIVITY + rewardCode`：每个实际生成发奖单的 enabled reward 扣 1。
   - `USER_LIMIT + USER + subject.id`：每个 rewardable participation 扣 1。
   - `DAILY_LIMIT + USER + subject.id:activityLocalDate`：每个 rewardable participation 在活动时区自然日扣 1。
6. 缺少对应 quota 行表示无限制，兼容未配置预算、库存或次数限制的活动。
7. 金额类奖励计算为 `0.0000` 且无实际权益时，不生成 grant_order/outbox，不扣 quota。
8. 非金额类实际权益即使 `reward_amount=0.0000`，仍按配置扣 STOCK、USER_LIMIT、DAILY_LIMIT；如配置了预算估值，是否扣 BUDGET 以现有奖励预算暴露规则为准，本需求不新增估值模型。
9. 同一 activity 下，参与记录、quota 预留、grant_order、outbox 必须保持 activity 级原子性。任一 quota 预留失败时，整组发奖副作用回滚。
10. 规则拒绝只写 `REJECTED` participation，不扣 quota，不生成 grant_order/outbox。
11. quota 不足时允许写 `QUOTA_EXCEEDED` 审计，用于排查；该审计不视为发奖链路脏写。
12. 发奖 callback 必须按 `tenantId + commandId` 定位，不能跨租户更新。
13. 重复最终态 callback 必须幂等，不得重复移动 `frozen_value`、`used_value` 或重复递增业务消耗。

## 边界条件

- 多个 quota 同时适用于一个 activity 时，必须全部预留成功才算活动发奖成功；任一失败则全部回滚。
- 多个 reward 同时生成 grant_order 时，BUDGET 按总金额一次性或等价聚合预留，STOCK 按 rewardCode 分别预留。
- 并发请求抢占同一 quota 的最后额度时，只允许满足额度条件的请求成功；失败请求不得留下 grant_order/outbox。
- `total_value = used_value + frozen_value` 时，新增正数预留必须失败。
- `total_value`、`used_value`、`frozen_value` 均按 Decimal 语义处理，禁止 float 误差导致越界。
- `frozen_value` 不得降为负数；确认或释放时必须能找到对应冻结记录并保证聚合值足够。
- 活动时区用于 DAILY_LIMIT 的自然日切分，不能使用服务器本地时区替代。
- 模拟运行只展示 quota 快照影响和 `wouldExceed`，不得真实预留、确认、释放或写 grant_order/outbox。

## 异常场景

- Quota 不足：返回 `409 QUOTA_EXCEEDED`，回滚本次 `REWARDABLE` participation、grant_order、outbox 和 quota 预留。
- 并发版本冲突：按 quota 抢占失败处理，外部表现为 quota 不足或可重试的业务冲突；不得返回通用 500。
- 多 quota 部分成功后后续失败：必须补偿为事务回滚后的“全部未占用”状态，不能留下部分冻结。
- Outbox 创建失败：本次 quota 预留和 grant_order 必须回滚。
- grant_order 创建失败：本次 quota 预留必须回滚，不能只冻结不发奖。
- callback `SUCCESS` 重复：返回重复语义，quota 不再变化。
- callback `FAILED` 重复：返回重复语义，quota 不再变化。
- callback 从 `SUCCESS` 改 `FAILED` 或从 `FAILED` 改 `SUCCESS`：返回冲突，quota 不得二次变化。
- reservation 缺失或聚合不一致：返回明确冲突错误，保留现场供对账排查。
- 重复事件：由事件幂等拦截，不得再次占用 quota。

## 非目标范围

- 不新增外部业务数据库访问，不直接修改用户余额。
- 不新增业务专用活动逻辑、业务专用字段或接入方定制规则。
- 不改变活动创建、奖励计算、Outbox worker 投递协议的主流程。
- 不新增 MQ 投递、人工发奖、人工导出或审批流。
- 不实现运营调整 `used_value` / `frozen_value` 的管理能力；如需调整，另立需求定义审计和对账规则。
- 不把模拟运行变成额度锁定或试占用。
- 不以 Redis Lua 作为本需求目标；本需求明确采用 DB 乐观锁口径。
- 不扩大 quota 类型；本次只覆盖现有 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`。

## 风险

- 并发一致性风险高：如果仍存在读后写路径，会在高并发下超预算。
- 事务边界风险高：quota、participation、grant_order、outbox 任一不在同一 activity 原子边界内，都会产生脏写或悬挂冻结。
- 回调补偿风险中高：确认/释放重复执行会造成 `used_value` 或 `frozen_value` 漂移。
- 多 quota 组合风险中高：预算、库存、用户次数同时扣减时，部分成功回滚必须可靠。
- 文档口径差异风险中：旧数据模型示例曾写事件阶段直接增加 `used_value`，但 API/路线图已经要求冻结、确认、释放。本需求以冻结模型为准。
- 测试风险中：内存仓储不能证明数据库乐观锁，需要 PostgreSQL 并发测试作为验收依据。

## 验收标准

1. 并发扣减不超预算：设置 `BUDGET total_value = N`，并发提交超过 N 总金额的 rewardable 事件，最终 `used_value + frozen_value <= total_value`，成功发奖单数量或金额不超过额度。
2. 并发抢库存不超卖：设置 `STOCK total_value = 1`，并发命中同一 rewardCode，只允许一个事件生成 grant_order/outbox。
3. 用户次数限制生效：设置 `USER_LIMIT total_value = 1`，同一用户重复命中同一活动，只允许一次 rewardable 成功。
4. Daily limit 按活动时区生效：同一用户跨活动本地自然日可重新占用，当日重复占用受限。
5. quota 不足无脏写：失败事件不得留下 `REWARDABLE` participation、grant_order、outbox；允许存在 `QUOTA_EXCEEDED` 审计。
6. 多 quota 原子性：同一活动同时扣 BUDGET、STOCK、USER_LIMIT 时，任一 quota 不足后其他 quota 不得残留冻结或已用增量。
7. 事件幂等：完全重复事件返回 `duplicate=true`，quota、grant_order、outbox 均不新增。
8. 回调成功确认：`SUCCESS` 后相关 reservation 从冻结转确认，聚合表现为 `frozen_value` 减少、`used_value` 增加，重复 `SUCCESS` 不再变化。
9. 回调失败释放：`FAILED` 后相关 reservation 释放，聚合表现为 `frozen_value` 减少、`used_value` 不变，重复 `FAILED` 不再变化。
10. callback 租户隔离：相同 `commandId` 不得跨 tenant 更新 grant_order 或 quota。
11. 模拟运行无副作用：simulate 可展示 quota 风险，但不会修改 `marketing_quota`、participation、grant_order 或 outbox。
12. PostgreSQL 验证必需：并发和事务验收必须在 PostgreSQL 仓储下执行，不能只以内存仓储通过作为验收。

## 待确认问题

当前需求可以按上述默认最小可验收口径进入研发，无阻塞性待确认问题。

非阻塞建议后续单独确认：

- 运营后台是否需要显式展示 `frozen_value` 和长期冻结告警。
- 非金额权益带 `valuationAmount` 时是否应纳入 BUDGET 扣减。
- 是否需要给并发版本冲突和额度不足拆分不同外部错误码。

## 评分

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        17/20
总分              88/100
```

扣分说明：

- 现有文档中“事件阶段直接 used 扣减”和“冻结后回调确认”存在历史口径差异，已采用冻结模型作为本需求默认口径，但仍有文档一致性风险。
- 是否对非金额权益估值扣 BUDGET 尚未完全定稿，本需求将其列为非阻塞后续问题，不影响现金预算、库存和次数的原子扣减验收。

## Decision

```text
PASS
```

理由：Score 88，核心需求、边界、异常、验收标准和非目标范围已明确；无 P0/P1 Review 反馈；旧 Analysis BLOCK 已关闭；可交给 Review Agent 审核。
