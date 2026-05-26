# Analysis

需求：2026-05-24-reward-callback

## Review 反馈处理

已读取 `tasks/2026-05-24-reward-callback/review.md`。该文件目前仍是模板态：

- `P0:`、`P1:`、`P2:` 均未填写具体问题。处理：无可逐条修正的 Review 缺陷；本轮仍按高风险资金类回调需求补全可审核口径。
- `Open Questions` 为“待填写”。处理：视为未给出具体开放问题；本轮主动关闭任务材料中的关键歧义，包括回调定位键、签名失败、重复回调、未知单号、quota reservation 处理和错误响应。
- `Decision` 为 `PASS / BLOCK` 模板。处理：不作为真实 PASS；本轮重新输出完整需求、验收标准和评分，供 Review Agent 再审。

## Analysis 自检修正

已读取上一轮 `analysis.md`。上一轮为模板态，存在 `Score 0`、`Decision PASS / BLOCK`、各章节“待填写”，等价于 Analysis 自检未通过。本轮逐项修正：

- `Score 0`：原因是需求未分析。本轮补充背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标、风险、验收标准和待确认问题，按评分规则重新打分。
- `Decision PASS / BLOCK` 模板：原因是未给出真实结论。本轮基于默认最小可验收口径给出明确 `Decision PASS`。
- 任务要求“按 bill_no/idempotency_key 定位 grant_order”，但既有 `docs/api.md` 写作 `tenantId + commandId`。本轮默认口径：`billNo` / `bill_no` 是外部发奖指令号，语义等同现有 `commandId` / `marketing_grant_order.bill_no`；`idempotencyKey` / `idempotency_key` 可作为同一发奖单的辅助定位键。请求同时携带多个定位键时必须定位到同一租户下同一发奖单，否则返回冲突。
- 任务验收要求“签名失败”，而现有 `docs/api.md` 曾写“后续生产版本必须增加签名或 token 鉴权”。本轮默认口径：本需求必须纳入 Reward Callback 签名校验，最小协议沿用项目 Webhook HMAC 形态，作为本任务可验收范围。
- 旧分析没有边界和异常。本轮补充重复回调、相反终态、未知单号、签名缺失/错误、非法请求、外部单号冲突、reservation 缺失、quota 聚合冲突、跨租户隔离和并发竞态。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、计算活动奖励、生成 participation、grant_order 和 outbox，不直接修改外部钱包、积分、优惠券、礼品或会员系统。外部发奖系统完成真实权益发放后，需要把发奖最终结果回调给 ExMarketing。

本需求目标是定义并交付 Reward Callback 回调接口的业务口径：

- 外部发奖系统可以回传 `SUCCESS` 或 `FAILED` 最终态。
- ExMarketing 能在租户隔离下按发奖指令号或发奖幂等键定位唯一 `marketing_grant_order`。
- 成功、失败、重复回调、未知单号、签名失败都有稳定、机器可识别的响应。
- 回调终结 grant_order 后，相关 participation、quota reservation、quota 聚合和审计/对账信息保持一致。
- 回调不会绕过 grant_order，不会跨租户更新，不会重复确认或释放 quota。

## 参与方

- 外部发奖系统：钱包、积分、优惠券、礼品、抽奖、会员权益等真实发奖系统，负责回调最终结果。
- ExMarketing OpenAPI：接收 callback 请求，完成签名、租户、请求体和定位键校验。
- Grant Order：ExMarketing 内部发奖单，记录 `bill_no` / `commandId`、`idempotency_key`、发奖状态、外部发奖流水和错误信息。
- Quota Reservation：记录发奖前冻结的预算、库存、用户次数或每日次数。
- Quota 聚合：维护 `used_value` 和 `frozen_value`，由 callback 成功确认或失败释放。
- Participation：用户参与记录，随发奖最终态推进为 `GRANTED` 或 `FAILED`。
- 对账/运维人员：通过审计和对账查询排查未知单号、长期冻结、重复回调和状态冲突。

## 输入输出

### HTTP 入口

```text
POST /openapi/v1/reward/callback
Content-Type: application/json
X-ExMarketing-Timestamp: <unix_seconds>
X-ExMarketing-Signature: <lowercase_hex_hmac_sha256>
```

最小请求体：

```json
{
  "tenantId": "site_001",
  "billNo": "cmd_xxx",
  "idempotencyKey": "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123:rewardCode",
  "status": "SUCCESS",
  "externalGrantId": "wallet_bill_888",
  "message": "granted",
  "finishedAt": "2026-05-24T12:00:03Z"
}
```

字段口径：

- `tenantId` 必填，所有定位和更新必须限定在该租户内。
- `billNo` / `bill_no` / `commandId` 三者为同一语义：外部发奖指令号，对应 `marketing_grant_order.bill_no`。为兼容既有文档，`commandId` 可继续接受。
- `idempotencyKey` / `idempotency_key` 对应 `marketing_grant_order.idempotency_key`，可单独定位，也可与 `billNo` 一起用于一致性校验。
- `billNo` 与 `idempotencyKey` 至少提供一个；同时提供时必须定位到同一条 grant_order。
- `status` 必填，只接受 `SUCCESS` / `FAILED`。
- `SUCCESS` 必须提供非空 `externalGrantId`；`FAILED` 可不提供。
- `message` 可选，用于保存外部发奖说明或失败原因，不能作为状态判断依据。
- `finishedAt` 可选；提供时必须为带时区的 RFC3339 时间。未提供时以 ExMarketing 接收时间作为终结时间。

### 签名输入输出

默认最小签名协议沿用项目 Webhook HMAC 形态：

```text
signingInput = X-ExMarketing-Timestamp + "." + raw request body
signature = hex(HMAC-SHA256(callbackSecret, signingInput))
```

要求：

- 配置了 callback secret 的租户必须校验签名。
- 生产环境必须配置 callback secret；无签名、时间戳非法、签名不匹配或超出重放窗口均不得解析为有效回调。
- 本任务验收中必须覆盖签名失败，因此测试环境应配置 callback secret。
- 签名失败不得更新 grant_order、participation、quota、reservation 或审计成成功业务结果。

### 成功响应

首次成功或失败回调的响应必须包含可对账字段：

```json
{
  "tenantId": "site_001",
  "grantOrderId": "go_xxx",
  "billNo": "cmd_xxx",
  "idempotencyKey": "idem_xxx",
  "status": "SUCCESS",
  "duplicate": false
}
```

重复相同最终态：

```json
{
  "tenantId": "site_001",
  "grantOrderId": "go_xxx",
  "billNo": "cmd_xxx",
  "status": "SUCCESS",
  "duplicate": true
}
```

### 错误响应

错误响应必须稳定包含 `errorCode` 和 `message`，不得泄露 secret、完整签名、完整 Authorization、内部 SQL 或敏感 payload。

默认错误码：

```text
400 INVALID_CALLBACK_REQUEST
401 CALLBACK_SIGNATURE_MISSING
401 CALLBACK_TIMESTAMP_INVALID
401 CALLBACK_SIGNATURE_INVALID
404 GRANT_ORDER_NOT_FOUND
409 CALLBACK_LOCATOR_CONFLICT
409 CALLBACK_STATUS_CONFLICT
409 GRANT_ORDER_CLOSED
409 EXTERNAL_GRANT_ID_CONFLICT
409 QUOTA_RESERVATION_MISSING
409 QUOTA_RESERVATION_CONFLICT
409 QUOTA_AGGREGATE_CONFLICT
500 INTERNAL_ERROR
```

## 业务规则

1. callback 只接收 `application/json`；请求体不可解析或存在非法字段时返回 `INVALID_CALLBACK_REQUEST`。
2. 回调必须先通过签名和时间戳校验，再执行业务状态更新。
3. 所有 grant_order 定位必须带 `tenantId`，禁止只按 `billNo`、`commandId` 或 `idempotencyKey` 跨租户查找。
4. `tenantId + billNo`、`tenantId + commandId`、`tenantId + idempotencyKey` 任一定位方式命中时，都只能定位一条 grant_order。
5. 同时提供多个定位键时，必须全部指向同一条 grant_order；不一致返回 `CALLBACK_LOCATOR_CONFLICT`。
6. 未知 `tenantId + locator` 返回 `GRANT_ORDER_NOT_FOUND`，不得创建新 grant_order。
7. `SUCCESS` 是最终成功态，首次成功应把 grant_order 标记为 `SUCCESS`，保存 `externalGrantId`、`message` 和终结时间。
8. `FAILED` 是最终失败态，首次失败应把 grant_order 标记为 `FAILED`，保存失败 `message` 和终结时间。
9. 相同最终态重复回调幂等返回 `duplicate=true`，不得重复确认、释放或移动 quota。
10. 已 `SUCCESS` 后收到 `FAILED`，或已 `FAILED` 后收到 `SUCCESS`，返回 `CALLBACK_STATUS_CONFLICT`，不得覆盖既有最终态。
11. 已记录 `externalGrantId` 的成功单，再收到不同 `externalGrantId` 的成功回调，返回 `EXTERNAL_GRANT_ID_CONFLICT`。
12. `CLOSED` 或人工关闭类终态的 grant_order 不允许 callback 改写，返回 `GRANT_ORDER_CLOSED`。
13. 首次 `SUCCESS` 对应的 `FROZEN` quota reservation 应转为 `CONFIRMED`，quota 聚合从 `frozen_value` 转入 `used_value`。
14. 首次 `FAILED` 对应的 `FROZEN` quota reservation 应转为 `RELEASED`，quota 聚合减少 `frozen_value`，不得增加 `used_value`。
15. 完全未配置 quota 的不限额 grant_order 可以没有 reservation；callback 正常终结，不返回 `QUOTA_RESERVATION_MISSING`。
16. 能证明发奖创建时应有 reservation 但 callback 时缺失的，返回 `QUOTA_RESERVATION_MISSING` 或进入对账异常，不得猜测金额确认或释放。
17. reservation 状态、金额、scope、scopeKey 或 grant_order 关联不一致时，返回 `QUOTA_RESERVATION_CONFLICT`。
18. quota 聚合无法满足确认或释放条件，例如 `frozen_value` 不足、版本冲突或会导致负数时，返回 `QUOTA_AGGREGATE_CONFLICT`。
19. 同一 participation 下多个 grant_order 时，`BUDGET` / `STOCK` 按 grant_order 粒度确认或释放；`USER_LIMIT` / `DAILY_LIMIT` 按 participation 聚合结果只确认或释放一次。
20. participation 状态应随聚合发奖结果推进：任一 grant_order 成功后可进入 `GRANTED`；全部 grant_order 最终失败且无成功时进入 `FAILED`。
21. Outbox `SENT` 只表示发奖指令已送达外部系统，不代表真实发奖成功；真实发奖成功必须以 callback `SUCCESS` 为准。
22. 模拟运行不触发 callback，不改变 grant_order、participation、quota 或 reservation。
23. callback 应写入可对账信息，包括租户、定位键、最终态、duplicate 标记、外部流水、失败原因、签名校验结果摘要和时间线；不得记录完整 secret 或完整签名。

## 边界条件

- `billNo`、`commandId`、`idempotencyKey` 均缺失：`400 INVALID_CALLBACK_REQUEST`。
- `status` 为空、大小写不符合约定或不是 `SUCCESS` / `FAILED`：`400 INVALID_CALLBACK_REQUEST`。
- `SUCCESS` 缺少 `externalGrantId`：`400 INVALID_CALLBACK_REQUEST`。
- `finishedAt` 不含时区或不可解析：`400 INVALID_CALLBACK_REQUEST`。
- 请求签名时间戳超出重放窗口：`401 CALLBACK_TIMESTAMP_INVALID` 或等价签名错误。
- 同一 `billNo` 在不同 tenant 下存在：只能更新请求 `tenantId` 下的记录，其他租户不受影响。
- `idempotencyKey` 指向 A 发奖单，`billNo` 指向 B 发奖单：`409 CALLBACK_LOCATOR_CONFLICT`。
- 回调早于 outbox 标记 `SENT` 到达：只要 grant_order 状态和 reservation 允许，仍可终结；不要求等待 outbox 状态。
- 不限额活动无 reservation：允许 callback 终结，不移动 quota。
- 已有 reservation 但 quota 聚合异常：不得强行终结为成功业务结果，应返回冲突并保留现场供对账。
- 并发重复 callback：只有一个请求执行状态迁移和 quota 移动，其他请求返回 duplicate 或 conflict。
- 回调与超时释放并发：谁先终结 grant_order / reservation 谁生效；后到方不得覆盖或二次移动 quota。

## 异常场景

- 签名缺失：返回 `CALLBACK_SIGNATURE_MISSING`，无业务副作用。
- 签名错误：返回 `CALLBACK_SIGNATURE_INVALID`，无业务副作用。
- 未知单号或未知幂等键：返回 `GRANT_ORDER_NOT_FOUND`，无业务副作用。
- 重复成功：返回 `duplicate=true`，grant_order、quota 和 reservation 不变。
- 重复失败：返回 `duplicate=true`，grant_order、quota 和 reservation 不变。
- 成功后失败：返回 `CALLBACK_STATUS_CONFLICT`，保留成功态和已确认 quota。
- 失败后成功：返回 `CALLBACK_STATUS_CONFLICT`，保留失败态和已释放 quota。
- 成功外部流水冲突：返回 `EXTERNAL_GRANT_ID_CONFLICT`，保留首次成功流水。
- 已关闭发奖单：返回 `GRANT_ORDER_CLOSED`，无 quota 二次移动。
- 应有 reservation 缺失：返回 `QUOTA_RESERVATION_MISSING` 或对账异常，不猜测补账。
- reservation 已非 `FROZEN` 且不是同一最终态重复：返回 `QUOTA_RESERVATION_CONFLICT`。
- quota 聚合移动会导致负数或超过总额：返回 `QUOTA_AGGREGATE_CONFLICT`。
- 内部非预期错误：返回 `INTERNAL_ERROR`，不得暴露内部实现细节；已完成和未完成的状态必须可通过对账确认。

## 非目标范围

- 不直接调用外部钱包、积分、优惠券、礼品、抽奖或会员系统。
- 不新增活动类型、奖励类型、规则 evaluator、quota 类型或业务专用模型。
- 不实现外部发奖系统的发送、重试、DLQ 策略；这些属于 Outbox worker 范围。
- 不实现运营手工改账、手工补发或手工改 `used_value` / `frozen_value`。
- 不迁移历史脏数据；本任务只要求对历史不限额记录兼容终结，对应有 reservation 缺失记录给出可识别异常。
- 不用代码实现细节替代需求，不限定具体函数、SQL、锁实现或包结构。
- 不把 Admin 鉴权、完整生产安全体系或密钥管理后台作为本任务范围；但 Reward Callback 签名校验本身属于本任务范围。
- 不要求 callback 支持非 JSON、批量回调、异步回执或外部系统主动查询。

## 风险

- 账务一致性风险高：grant_order、reservation、quota 聚合和 participation 若不原子一致，会造成发奖成功但预算未确认，或发奖失败但冻结不释放。
- 幂等风险高：重复 callback、并发 callback、相反终态 callback 可能重复确认或释放 quota。
- 租户隔离风险高：只按 `billNo` 或 `idempotencyKey` 定位会导致跨租户误更新。
- 定位键歧义风险中高：任务使用 `bill_no/idempotency_key`，既有文档使用 `commandId`；本轮默认将 `billNo` 与 `commandId` 统一为 `marketing_grant_order.bill_no`。
- 安全风险高：callback 若无签名，外部可伪造成功或失败结果；因此本任务必须覆盖签名失败。
- 历史兼容风险中：完全未配置 quota 的 grant_order 无 reservation 是合法情况，不能误判为异常。
- 多奖励风险中：同一 participation 下多个 grant_order 可能部分成功，次数类 quota 不能按每张 grant_order 重复确认或释放。
- 可观测性风险中：未知单号、签名失败、状态冲突、reservation 缺失如果无审计，将难以排查。

## 验收标准

1. 成功回调：合法签名、合法租户、已存在未终结 grant_order、`status=SUCCESS` 且带 `externalGrantId` 时，返回成功，grant_order 进入 `SUCCESS`，保存外部流水和完成时间。
2. 失败回调：合法签名、合法租户、已存在未终结 grant_order、`status=FAILED` 时，返回成功，grant_order 进入 `FAILED`，保存失败说明和完成时间。
3. 重复成功：同一 grant_order 重复收到相同 `SUCCESS` 且外部流水一致，返回 `duplicate=true`，不重复移动 quota。
4. 重复失败：同一 grant_order 重复收到相同 `FAILED`，返回 `duplicate=true`，不重复移动 quota。
5. 相反终态：`SUCCESS` 后 `FAILED`、`FAILED` 后 `SUCCESS` 均返回 `CALLBACK_STATUS_CONFLICT`，既有状态和 quota 不变。
6. 未知单号：未知 `tenantId + billNo/commandId/idempotencyKey` 返回 `GRANT_ORDER_NOT_FOUND`，不创建、不更新任何业务记录。
7. 定位冲突：`billNo` 与 `idempotencyKey` 指向不同 grant_order 时返回 `CALLBACK_LOCATOR_CONFLICT`。
8. 签名失败：缺失、非法或不匹配签名返回 `CALLBACK_SIGNATURE_MISSING` / `CALLBACK_SIGNATURE_INVALID`，不更新 grant_order、participation、quota 或 reservation。
9. 非法请求：缺少 `tenantId`、缺少定位键、非法 `status`、`SUCCESS` 缺少 `externalGrantId`、非法 `finishedAt` 均返回 `INVALID_CALLBACK_REQUEST`。
10. 租户隔离：两个租户存在相同 `billNo` 或相同形态定位键时，只更新请求 `tenantId` 下的 grant_order。
11. 成功确认 quota：有 `FROZEN` reservation 的 `SUCCESS` 回调会把相关 reservation 置为 `CONFIRMED`，quota `frozen_value` 减少、`used_value` 增加。
12. 失败释放 quota：有 `FROZEN` reservation 的 `FAILED` 回调会把相关 reservation 置为 `RELEASED`，quota `frozen_value` 减少、`used_value` 不增加。
13. 不限额兼容：完全未配置 quota 的 grant_order 可被 `SUCCESS` 或 `FAILED` 正常终结，不返回 `QUOTA_RESERVATION_MISSING`。
14. reservation 缺失异常：能证明应有 reservation 但缺失时返回 `QUOTA_RESERVATION_MISSING` 或对账异常，不猜测移动 quota。
15. 聚合冲突安全：quota 聚合异常时返回 `QUOTA_AGGREGATE_CONFLICT`，不得产生负数、超总额或部分移动。
16. 多奖励次数 quota：同一 participation 多个 grant_order 时，`USER_LIMIT` / `DAILY_LIMIT` 只确认或释放一次。
17. participation 推进：任一 grant_order 成功后 participation 可对账为 `GRANTED`；全部最终失败且无成功时可对账为 `FAILED`。
18. 并发幂等：并发发送相同 callback，最终只有一次状态迁移和 quota 移动。
19. 审计可查：成功、失败、重复、未知单号、签名失败、状态冲突、reservation 异常都有审计或对账线索，且不记录完整 secret 或完整签名。
20. 文档同步：API 文档需说明 `billNo` / `bill_no` / `commandId` 兼容关系、`idempotencyKey` 定位规则、签名协议、错误码和重复回调语义。
21. 测试覆盖：至少覆盖成功、失败、重复成功、重复失败、相反终态、未知单号、签名失败、定位冲突、不限额 callback、reservation 缺失、quota 聚合冲突和租户隔离。

## 待确认问题

本轮不保留阻塞性待确认问题。以下为非阻塞后续增强点，不影响本需求进入研发：

- callback secret 的配置中心、轮换和多密钥灰度机制可后续单独设计；本需求只要求存在可配置 secret 并能校验 HMAC。
- 是否为不同外部发奖系统配置不同 secret 或 receiverCode，可作为多发奖渠道生产化增强；本需求默认按租户或运行时配置选择 secret。
- 是否增加外部发奖系统主动查询 callback 结果的接口，不在本任务范围。
- 是否对签名失败做限流或风控告警，不影响本任务最小可验收口径。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              91/100
```

扣分说明：

- 签名 secret 的配置来源和轮换策略仍属于后续生产化细节，本需求只定义最小校验协议。
- 历史缺失 reservation 的判定依赖已有记录可证明性，仍需实现阶段保持对账可观测。

## Decision

```text
PASS
```

理由：已关闭上一轮 Analysis 模板态 BLOCK，补齐 Review 模板中未填写项可能涉及的核心歧义；需求已覆盖成功、失败、重复回调、未知单号、签名失败、定位冲突、quota 一致性、租户隔离、非目标和可验收测试口径，Score >= 80，可进入 Review Agent 审核。
