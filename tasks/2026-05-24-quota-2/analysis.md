# Analysis

需求：2026-05-24-quota-2

## Review 反馈处理

已读取 `tasks/2026-05-24-quota-2/review.md`，本轮逐条处理如下：

- P1：不限额活动与 `QUOTA_RESERVATION_MISSING` 冲突。已修正口径：缺少 quota 配置行表示不限额，不生成 reservation，也不要求 callback 时存在 reservation；callback 应正常终结 grant_order / participation，且不移动 quota。只有“发奖创建时按配置应冻结 quota，但对应 reservation 缺失或无法匹配”的情况，才返回 `409 QUOTA_RESERVATION_MISSING` 或进入对账异常。
- P1：超时补偿缺少本任务交付入口和 owner。已固定最小交付面：本任务必须提供可由运维/后台触发的超时释放入口，默认作为 Admin/Operations 平面能力交付；入口必须支持 tenant、补偿阈值、limit、dry-run、operator、reason，并按默认 2 小时阈值扫描未终结且仍有 `FROZEN` reservation 的发奖链路。定时 worker 可作为增强，但不能替代这个可触发入口。
- P2：reservation 粒度表述过绝对。已区分 `BUDGET` / `STOCK` 为 grant_order 级 reservation，`USER_LIMIT` / `DAILY_LIMIT` 为 participation 级 reservation。多 grant_order 归属同一 participation 时，次数类 reservation 只冻结一次，并按 participation 的聚合结果只确认或释放一次，避免每个 callback 重复移动次数 quota。
- Open Question：不限额活动是否允许没有任何 quota reservation。默认答案：允许。完全未配置 quota 的活动产生的 grant_order 可以没有 reservation，callback 只更新 grant_order / participation 和回调审计，不返回 `QUOTA_RESERVATION_MISSING`。
- Open Question：超时补偿最小入口。默认答案：后台/运维触发入口为本任务最小交付面；repository/service 层能力或测试入口只能作为支撑，不能作为唯一交付结果。

## Analysis 自检修正

已读取上一轮 `analysis.md`。上一轮 `Decision PASS`、Score `90/100`，但仍有扣分说明、非阻塞待确认问题和一处错误假设，本轮修正如下：

- 错误假设：上一轮写成 `review.md` 仍为模板态，实际 review 已给出 `BLOCK`、P1/P2 和 Open Questions。本轮已改为逐条回应真实 review。
- 扣分项：上一轮认为超时补偿调度方式和默认阈值未唯一。已用默认最小可验收口径关闭：Admin/Operations 触发入口必交付，默认阈值 2 小时，可由请求参数或配置覆盖。
- 待确认：生产 callback 鉴权仍不纳入本任务，保留为非目标；但本任务要求 callback 业务处理必须按 `tenantId + commandId` 隔离。
- 待确认：`DLQ_CLOSED`、人工关闭或运营撤销是否释放 quota。默认最小口径：本任务不把普通 DLQ close 自动等同发奖失败；只有明确 `FAILED` callback 或本任务的超时释放入口会释放冻结。
- 待确认：历史缺少 reservation 的数据迁移。默认最小口径：不迁移历史数据；能证明发奖创建时应有 reservation 却缺失的记录进入对账异常，无法证明配置过 quota 的旧记录按不限额兼容处理并允许终结。
- 待确认：长期 `FROZEN` 告警展示。默认最小口径：补偿入口和对账查询必须能筛选长期冻结；专门告警 UI 不在本任务范围内。

## 背景目标

ExMarketing 是独立活动营销中台，接收标准事件、执行活动规则、生成 participation、grant_order 和 outbox，不直接修改外部钱包、会员、库存或其他权益系统。预算、库存、用户参与次数和用户每日参与次数由 `marketing_quota` 表示。

本需求要求把 quota 从“发奖前扣减”细化为“冻结、确认、释放”生命周期：

```text
事件命中活动
-> 计算奖励
-> 按配置原子冻结 quota
-> 创建 participation / grant_order / outbox
-> 外部发奖 callback 或超时补偿
-> SUCCESS 确认消耗 / FAILED 或 TIMEOUT 释放冻结
```

目标：

- 发奖成功时，冻结额度从 `frozen_value` 转为 `used_value`，不长期占用。
- 发奖失败或超时时，冻结额度释放，不增加 `used_value`。
- 重复 callback、相反 callback、重复补偿和并发补偿不会重复确认、重复释放或导致 quota 聚合漂移。
- 不配置 quota 的活动保持向后兼容，按不限额处理。

## 参与方

- 外部事件来源系统：上报标准事件。
- ExMarketing Engine：匹配活动、计算奖励、判断 quota、生成 participation、grant_order 和 outbox。
- Quota 聚合与 reservation 账本：记录冻结、确认、释放所需的业务凭证和聚合值。
- Outbox Worker / 外部发奖系统：投递发奖指令并产生最终发奖结果。
- Reward Callback 入口：接收外部发奖系统 `SUCCESS` / `FAILED` 最终态。
- Admin/Operations 超时补偿入口：由运维或后台任务触发超时释放，防止冻结长期悬挂。
- 运营 / 对账人员：查询长期冻结、缺失 reservation、冲突 callback 和 quota 聚合异常。
- Review / Test Agent：验证需求是否清晰、可控、可验收。

## 输入输出

### 输入

- 标准事件：`tenantId`、`eventType`、`sourceSystem`、`subject`、`biz`、`occurredAt`、`attributes`、`Idempotency-Key`。
- 活动配置：活动状态、时间、时区、规则、奖励、发奖通道和 `targetRef`。
- Quota 配置：`activityId`、`quotaType`、`scope`、`scopeKey`、`totalValue`。缺少对应 quota 行表示不限额。
- 奖励计算结果：实际可发奖 reward 的 `rewardCode`、`rewardType`、`rewardAmount`、权益 payload。
- 发奖回调：`tenantId`、`commandId`、`status=SUCCESS|FAILED`、`externalGrantId`、`message`、`finishedAt`。
- 超时补偿请求：`tenantId`、`olderThan` 或阈值、`limit`、`dryRun`、`operator`、`reason`；未传阈值时默认选择早于当前时间 2 小时的未终结冻结记录。

### 输出

- 事件处理成功且有 quota 配置时：创建 reservation，并增加对应 quota `frozen_value`。
- 事件处理成功但没有任何 quota 配置时：创建 participation、grant_order、outbox，不创建 reservation，不移动 quota。
- quota 不足时：返回 `409 QUOTA_EXCEEDED`，不得留下 `REWARDABLE` participation、grant_order、outbox 或部分 reservation。
- `SUCCESS` callback：grant_order 进入 `SUCCESS`；grant_order 级 reservation 确认；满足 participation 聚合确认条件时，participation 级 reservation 确认。
- `FAILED` callback：grant_order 进入 `FAILED`；grant_order 级 reservation 释放；满足 participation 聚合释放条件时，participation 级 reservation 释放。
- 超时补偿：符合阈值且未终结的 grant_order 进入失败或关闭类最终态，相关可释放 reservation 释放，并记录 `TIMEOUT_RELEASE` 类原因。
- 重复 callback / 重复补偿：返回或记录 duplicate / already-final 语义，quota 聚合不再变化。
- 对账查询：能按 tenant、commandId、activity、subject、event、idempotencyKey 查询 grant_order、participation、outbox、callback、reservation、quota 聚合快照和审计时间线。

## Quota 与 Reservation 粒度

本需求默认采用以下最小可验收粒度：

| quotaType | scopeKey 默认口径 | reservation 粒度 | 确认 / 释放触发 |
|---|---|---|---|
| `BUDGET` | `TOTAL` | grant_order 级 | 对应 grant_order 首次 `SUCCESS` 确认，首次 `FAILED` / TIMEOUT 释放 |
| `STOCK` | `rewardCode` | grant_order 级 | 对应 reward 的 grant_order 首次 `SUCCESS` 确认，首次 `FAILED` / TIMEOUT 释放 |
| `USER_LIMIT` | `subject.id` | participation 级 | 同一 participation 下任一 grant_order `SUCCESS` 后确认一次；全部 grant_order 最终失败/超时且无成功时释放一次 |
| `DAILY_LIMIT` | `subject.id:活动时区自然日` | participation 级 | 同 `USER_LIMIT`，只确认或释放一次 |

推导依据：

- `BUDGET` 与金额类 reward 或奖励价值相关，天然对应单个发奖单的实际发奖结果。
- `STOCK` 的 `scopeKey` 为 `rewardCode`，天然对应单个 reward / grant_order。
- `USER_LIMIT` 和 `DAILY_LIMIT` 代表参与资格次数，不应因同一 participation 下多个 reward 重复扣减。
- 同一 participation 多个 grant_order 时，次数类 quota 以“该次参与是否最终产生至少一个成功权益”为确认依据。

## 业务规则

1. quota 可用性统一按 `used_value + frozen_value + 本次冻结量 <= total_value` 判断。
2. 事件处理阶段只增加 `frozen_value`，不得直接增加 `used_value`。
3. 缺少对应 quota 行表示不限额，不生成 reservation，不移动 quota。
4. 不限额活动的 grant_order 允许没有任何 reservation；callback 不得因此返回 `QUOTA_RESERVATION_MISSING`。
5. 只有发奖创建时按配置应生成 reservation，但 callback / 补偿时缺失、状态不匹配或金额不匹配，才属于 reservation 异常。
6. 对已配置 quota 的 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 均适用冻结、确认和释放语义。
7. 多 reward、多 quota 的同一 activity 预留必须 all-or-nothing；任一已配置 quota 不足则全部回滚。
8. 每条 reservation 只能从 `FROZEN` 终结一次，终态只能是 `CONFIRMED` 或 `RELEASED`。
9. `SUCCESS` 只能确认 `FROZEN` reservation，并把相同数值从 `frozen_value` 转入 `used_value`。
10. `FAILED` 和 TIMEOUT 只能释放 `FROZEN` reservation，并减少 `frozen_value`，不得增加 `used_value`。
11. 相同最终态重复回调幂等，不重复移动 quota。
12. 相反最终态回调冲突，例如已 `SUCCESS` 后收到 `FAILED`，必须返回 `409 CALLBACK_STATUS_CONFLICT` 或等价机器可识别错误，quota 不得变化。
13. `SUCCESS` 必须提供 `externalGrantId`；同一 grant_order 已记录不同 `externalGrantId` 时必须冲突。
14. callback 必须按 `tenantId + commandId` 定位 grant_order，禁止跨租户更新。
15. grant_order 状态、reservation 状态、quota 聚合、participation 状态必须在同一业务原子结果内同成同败。
16. `BUDGET` 冻结金额为本 grant_order 对应金额类奖励消耗；金额必须使用 Decimal 语义。
17. `STOCK` 冻结数量默认每个 enabled 且实际发奖的 reward 扣 1。
18. `USER_LIMIT` 和 `DAILY_LIMIT` 冻结数量默认每个 rewardable participation 扣 1。
19. 金额类奖励结果为 `0.0000` 且无实际权益时，不生成 grant_order/outbox，不冻结 quota。
20. 规则拒绝时只写 `REJECTED` participation，不生成 grant_order/outbox，不冻结 quota。
21. 模拟运行只展示 quota 影响预览，不创建 reservation，不修改 `frozen_value` 或 `used_value`；缺少 quota 行展示为 `unlimited=true`。

## 超时补偿最小交付口径

本任务必须交付一个生产可触发的 Admin/Operations 超时补偿入口，防止 `FROZEN` reservation 长期悬挂。默认口径如下：

- 触发 owner：运维、后台管理员或受控后台任务通过 Admin/Operations 平面触发。
- 触发入口：必须是可在运行环境调用的后台补偿能力；只提供 repository/service 方法或测试 helper 不满足本任务验收。
- 默认阈值：2 小时。入口可通过请求参数或配置覆盖，但必须有明确默认值。
- 必填约束：`tenantId`、`operator`、`reason` 必填；支持 `dryRun=true` 预览候选记录；支持 `limit` 限制单次处理量。
- 补偿对象：发奖创建时间或最后更新时间早于阈值，grant_order 未进入 `SUCCESS` / `FAILED` / `CLOSED` 等最终态，且存在仍为 `FROZEN` 的 reservation。
- 补偿动作：将候选 grant_order 标记为失败或关闭类最终态；释放对应 grant_order 级 reservation；按 participation 聚合规则释放 participation 级 reservation。
- 并发规则：callback 与补偿谁先终结 reservation 谁生效；后到方只能返回 duplicate / conflict / already-final，不得覆盖或二次移动 quota。
- 审计规则：每次 dry-run、实际释放、跳过、冲突都必须可对账，实际释放原因应能识别为 `TIMEOUT_RELEASE`。
- 非目标：本任务不要求复杂调度平台、自动重试策略、审批流或告警 UI；定时 worker 可后续增强。

## 边界条件

- `total_value = used_value + frozen_value` 时，新增正数冻结必须失败。
- `frozen_value` 不得降为负数；确认或释放前必须确认聚合值足以覆盖 reservation。
- `used_value` 不得超过 `total_value`；确认成功后也必须满足聚合约束。
- Decimal 金额、库存数量和次数数量不得使用浮点近似语义。
- 并发事件抢占同一 quota 最后额度时，只允许额度条件满足的一方成功。
- 重复事件由事件幂等拦截，不得创建新的 reservation 或重复冻结 quota。
- 回调早于 outbox 标记 `SENT` 到达时，只要 grant_order 与应有 reservation 状态允许，仍应按最终态处理。
- Outbox `DLQ` 不等于发奖失败；默认不自动释放 quota。只有明确 `FAILED` callback 或本需求定义的超时补偿才释放。
- 当前 activity 后续新增 quota 配置，不应倒推要求历史不限额 grant_order 必须有 reservation。
- 当前 activity 后续删除 quota 配置，不应使已经冻结的 reservation 失去确认 / 释放义务。
- 无法证明历史 grant_order 创建时配置过 quota 的缺失 reservation 记录，默认按不限额兼容终结，并留下对账标记。
- 能证明历史 grant_order 创建时配置过 quota 但 reservation 缺失的记录，必须作为异常处理，不猜测金额。

## 异常场景

- quota 不足：返回 `409 QUOTA_EXCEEDED`，本次发奖副作用回滚，允许写失败审计。
- 并发预留冲突：按 quota 抢占失败处理，不返回通用 500，不留下部分冻结。
- grant_order 创建失败：本次 quota 冻结必须回滚。
- outbox 创建失败：本次 participation、grant_order 和 quota 冻结必须回滚。
- 未知 `commandId`：返回 `404 GRANT_ORDER_NOT_FOUND`，不得更新 quota。
- 非法 callback 请求：返回 `400 INVALID_CALLBACK_REQUEST`，不得更新 grant_order 或 quota。
- 不限额 grant_order 收到 callback：正常更新 grant_order / participation，不移动 quota。
- 已关闭 grant_order 收到新最终态：返回 `409 GRANT_ORDER_CLOSED` 或等价冲突。
- 已 `SUCCESS` 后收到重复 `SUCCESS`：返回 duplicate 语义，quota 不变。
- 已 `FAILED` 后收到重复 `FAILED`：返回 duplicate 语义，quota 不变。
- 已 `SUCCESS` 后收到 `FAILED`：返回冲突，已确认 quota 不释放。
- 已 `FAILED` 后收到 `SUCCESS`：返回冲突，已释放 quota 不重新消耗。
- 按配置应有 reservation 但缺失：返回 `409 QUOTA_RESERVATION_MISSING` 或进入对账异常，不猜测确认或释放。
- reservation 状态、粒度或金额冲突：返回 `409 QUOTA_RESERVATION_CONFLICT`，保留现场。
- quota 聚合不足或版本异常：返回 `409 QUOTA_AGGREGATE_CONFLICT`，不得产生负数或漂移。
- 超时补偿与 callback 并发：只能有一个终结结果；另一方不得二次移动 quota。
- 同一 participation 多个 grant_order 部分失败：失败的 grant_order 释放其 `BUDGET` / `STOCK`；若其他 grant_order 已成功，`USER_LIMIT` / `DAILY_LIMIT` 已确认且不得释放。
- 同一 participation 多个 grant_order 全部失败或超时：所有 grant_order 级 reservation 释放，participation 级次数 reservation 释放一次。

## 非目标范围

- 不直接修改外部钱包、会员、库存或业务系统余额。
- 不新增活动类型、奖励类型、quota 类型或规则 evaluator。
- 不设计具体代码改法、函数签名、SQL 语句或实现分层细节。
- 不实现 callback 签名、token 协议或公网生产鉴权；生产公网暴露需另起安全需求。
- 不实现完整 Outbox 重试策略、DLQ 自动关闭或 DLQ close 自动释放 quota。
- 不迁移历史缺少 reservation 的旧数据；本任务只要求兼容终结或识别异常。
- 不提供运营手工调整 `used_value` / `frozen_value` 的后台能力。
- 不要求模拟运行产生真实冻结、确认、释放副作用。
- 不要求专门的长期冻结告警 UI；对账查询和补偿入口可作为最小排查手段。

## 风险

- 账务一致性风险高：reservation、grant_order、participation 和 quota 聚合若不是同一业务原子结果，会出现悬挂冻结或状态漂移。
- 幂等风险高：重复 callback、重复补偿、并发相反最终态会导致重复确认或释放。
- 粒度混淆风险高：把 participation 级次数 quota 当作 grant_order 级处理，会在多 reward 场景重复扣次数。
- 不限额兼容风险高：把没有配置 quota 的正常 grant_order 误判为缺失 reservation，会阻断 callback 终结。
- 超时补偿风险中高：阈值过短会误释放仍可能成功的发奖；默认 2 小时降低误释放概率，但仍需业务后续评估。
- 历史口径差异风险中：旧文档曾出现事件阶段直接扣 `used_value` 的描述；本需求以冻结、确认、释放模型为准。
- 安全边界风险中：callback 当前文档主要定义业务语义，生产公网鉴权不在本任务范围内，不能把本需求完成等同于生产可公网暴露。

## 验收标准

1. 发奖前冻结：配置了 quota 的可发奖事件成功处理后，对应 reservation 为 `FROZEN`，quota `frozen_value` 增加，`used_value` 不增加。
2. 不限额兼容：完全未配置 quota 的活动可以生成 grant_order 且没有 reservation；`SUCCESS` / `FAILED` callback 可正常终结，不返回 `QUOTA_RESERVATION_MISSING`，quota 不变化。
3. quota 不足无脏写：额度不足时没有 `REWARDABLE` participation、grant_order、outbox 或部分 reservation 残留。
4. 成功确认：`SUCCESS` callback 后，grant_order 为 `SUCCESS`，grant_order 级 reservation 为 `CONFIRMED`，quota `frozen_value` 减少、`used_value` 增加。
5. 失败释放：`FAILED` callback 后，grant_order 为 `FAILED`，grant_order 级 reservation 为 `RELEASED`，quota `frozen_value` 减少、`used_value` 不变。
6. 次数 quota 不重复：同一 participation 多个 grant_order 时，`USER_LIMIT` / `DAILY_LIMIT` 只冻结一次；任一 grant_order 成功后只确认一次；全部失败或超时后只释放一次。
7. 重复成功幂等：重复 `SUCCESS` 返回 duplicate 或等价语义，reservation 和 quota 聚合不再变化。
8. 重复失败幂等：重复 `FAILED` 返回 duplicate 或等价语义，reservation 和 quota 聚合不再变化。
9. 相反最终态冲突：`SUCCESS` 后 `FAILED`、`FAILED` 后 `SUCCESS` 都返回冲突，quota 不二次移动。
10. 未知单号安全：未知 `commandId` 返回 `GRANT_ORDER_NOT_FOUND`，不更新任何 quota。
11. 缺失 reservation 安全：能证明发奖创建时应有 reservation 但缺失时，返回 `QUOTA_RESERVATION_MISSING` 或对账异常，不猜测确认或释放。
12. 聚合一致性：任意路径不得出现 `frozen_value < 0`、`used_value < 0`、`used_value > total_value`。
13. 多 quota 原子性：同一 activity 同时涉及 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 时，预留、确认、释放均保持一致，不能部分成功。
14. 并发安全：并发事件和并发 callback / 补偿不会造成超预算、重复释放或重复确认。
15. 超时补偿入口：Admin/Operations 入口可按 tenant 和默认 2 小时阈值 dry-run 与实际释放长期 `FROZEN` reservation，并记录 operator、reason 和释放结果。
16. 超时补偿幂等：释放后重复补偿不改变 quota；若 callback 已先终结，补偿不得覆盖。
17. 租户隔离：callback 和超时补偿必须按 `tenantId` 隔离，不得跨 tenant 影响同名 commandId 或 grant_order。
18. 模拟运行无副作用：simulate 可展示 quota 风险和 unlimited 状态，但不会修改 quota、reservation、participation、grant_order 或 outbox。
19. 对账可查：对账查询能定位 commandId、activity、subject、event、grantStatus、participationStatus、reservationStatus、callback 信息、quota 聚合快照和超时释放原因。
20. 文档同步：API、数据模型或运维文档需反映冻结、确认、释放、错误码、不限额兼容、长期冻结排查和超时补偿入口。
21. 测试覆盖：至少覆盖成功、失败、超时补偿、重复回调、相反回调、未知单号、不限额 callback、应有 reservation 缺失、聚合冲突、quota 不足、多 quota 原子性、多 grant_order 次数 quota 和并发场景。

## 待确认问题

当前需求可按上述默认最小可验收口径进入研发，无阻塞性待确认问题。

非阻塞建议后续单独确认：

- 生产环境 callback 鉴权采用签名、Bearer token 还是网关 mTLS。
- 超时补偿默认阈值是否按 reward channel 或活动等级调整；本任务默认 2 小时。
- DLQ_CLOSED、人工关闭或运营撤销是否应触发 quota 释放；本任务默认不自动释放。
- 是否需要为历史缺少 reservation 且可证明应冻结的数据提供一次性迁移或人工修复工具。
- 管理后台是否需要专门展示长期 `FROZEN` reservation 告警。

## 评分

```text
需求清晰度        19/20
范围可控          18/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              92/100
```

扣分说明：

- callback 生产鉴权不在本任务范围内，需后续安全需求承接。
- 超时补偿默认阈值 2 小时是最小可验收默认值，后续仍可能按业务渠道细化。

## Decision

```text
PASS
```
