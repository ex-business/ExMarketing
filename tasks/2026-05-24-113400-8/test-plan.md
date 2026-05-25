# Test Plan

## Agent 指令

你是 Test Agent。Dev 完成后按本计划执行测试。

本计划来自 `review.md` 的明确 `Decision: PASS`。测试范围以本期最小可验收能力为准：提交标准事件样例，模拟活动匹配、规则解释、奖励预览、quota 影响预览、participation / grant_order / outbox 预览，并证明模拟运行不产生真实业务副作用。

Review 中列为待确认的增强能力不作为本期阻塞项：DRAFT / SCHEDULED 活动模拟、按活动 ID / Code 过滤模拟范围、模拟审计记录写入、Fact Provider 真实查询策略。若 Dev 顺手实现，可补充验证；若未实现，不因此失败。

## Test Strategy

1. 优先跑单元测试覆盖模拟服务的纯业务分支，确保不依赖真实 PostgreSQL 也能验证活动筛选、规则解释、奖励计算、quota 预览和幂等稳定性。
2. 用 HTTP handler / integration 风格测试覆盖 `POST /admin/v1/simulate` 请求响应结构、错误码和无副作用边界。
3. 用内存仓库和 PostgreSQL 仓库各跑一遍 smoke，重点检查模拟运行不会写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`，也不会改 `marketing_quota.used_value`、`frozen_value`、`version`。
4. 并发测试只验证模拟请求之间无共享可变状态污染、无数据竞争、无 quota 变化；不要求性能压测。
5. 幂等重复测试验证同一事件样例重复模拟返回稳定预览，但不创建正式幂等记录，也不因缺少正式 `Idempotency-Key` 被拒绝。

## Test Cases

### 1. 正常路径：命中 ACTIVE 活动并生成完整预览

准备数据：

- tenantId: `site_001`
- ACTIVE 活动，事件类型 `DEPOSIT_SUCCESS`，时间窗口覆盖 `2026-05-24T12:00:00Z`
- QUALIFY 规则：`attributes.isFirstDeposit = true` 且 `attributes.amount >= 100.00`
- RATIO 奖励：`baseField=attributes.amount`、`rate=0.20`、`maxAmount=500.00`、currency=`CNY`
- BUDGET / STOCK / USER_LIMIT quota 均充足

执行：

- 调用 `POST /admin/v1/simulate`，提交标准事件样例，amount=`299.00`。

断言：

- HTTP 返回成功。
- `matchedActivities` 包含该活动，`decision=MATCHED`。
- `ruleResults` 包含每条规则，状态为 `PASSED`，字段、expected、actual、reason 信息完整。
- `rewardPreviews` 金额为 `59.8000`，Decimal 精度正确，不出现浮点误差。
- `quotaImpactPreviews` 包含 BUDGET、STOCK、USER_LIMIT，`wouldExceed=false`，delta 正确。
- `participationPreview`、`grantOrderPreviews`、`outboxCommandPreviews` 均存在，且标识为 preview。
- 仓库中没有新增正式 event inbox、participation、grant order、outbox，quota 未变化。

### 2. 正常路径：未命中活动必须返回原因

准备数据：

- 至少两个候选活动：一个 eventType 不匹配，一个规则条件不满足。

执行：

- 提交 `DEPOSIT_SUCCESS` 事件，但 amount 低于活动门槛，或 eventType 与其中一个活动不匹配。

断言：

- 返回中包含 `unmatchedActivities`。
- eventType 不匹配活动给出触发事件不匹配原因。
- 规则失败活动给出 `decision=REJECTED`，失败规则状态为 `FAILED`，包含字段、期望值、实际值和原因码。
- 未命中活动不得生成 grant_order / outbox 预览。
- 无真实业务写入。

### 3. 正常路径：奖励为 0 时不生成发奖链路

准备数据：

- 活动规则通过。
- 奖励配置可计算但结果为 `0.0000`，例如 RATIO 的基础金额为 `0.00`，或命中 0 奖励阶梯。

断言：

- 活动可显示为已通过或 qualified，但奖励预览说明 `zero reward` / 无可发奖励。
- 不生成 `grantOrderPreviews` 和 `outboxCommandPreviews`。
- 不生成实际 quota 扣减预览，或明确标识 delta 为 0 / skipped。
- 无真实 participation、grant_order、outbox 写入。

### 4. 额度不足：预算不足

准备数据：

- 命中活动，奖励金额大于 BUDGET quota 可用值。
- STOCK 和 USER_LIMIT 充足。

断言：

- 模拟结果显示预算 quota 项 `wouldExceed=true`。
- 原因码应为 `QUOTA_EXCEEDED` 或等价语义，说明 `availableBefore` 与本次 `delta`。
- 活动最终不得表现为可正式发奖成功；应明确正式运行会因 quota 不足失败或被拒绝。
- 不允许部分生成真实 grant_order / outbox。
- PostgreSQL / 内存仓库中的 quota `used_value`、`frozen_value`、`version` 均不变。

### 5. 额度不足：库存不足

准备数据：

- 多奖励活动，其中某个 rewardCode 的 STOCK quota 可用值为 0。

断言：

- 对应 STOCK quota 预览 `wouldExceed=true`。
- 多奖励活动中任一 quota 不足时，整体结果不得表现为部分奖励可发。
- 所有真实业务表和 quota 均无变化。

### 6. 额度不足：用户次数不足

准备数据：

- USER_LIMIT quota 对 `subject.id` 的可用次数为 0。

断言：

- USER_LIMIT quota 预览 `wouldExceed=true`。
- reason 指向用户次数 / limit 不足。
- 不生成可误解为已成功落库的 participation / grant_order / outbox 状态。
- quota 未变化。

### 7. 幂等重复：同一事件样例重复模拟稳定且无正式幂等写入

执行：

- 连续两次提交完全相同事件样例。
- 不传正式事件接口要求的 `Idempotency-Key` header。

断言：

- 两次请求均成功，不因缺少正式 `Idempotency-Key` 被拒绝。
- 两次响应的活动决策、规则结果、奖励金额、quota delta、预览 command 语义稳定一致。
- RANDOM / PROBABILITY 奖励若在测试数据中出现，同一事件样例和同一活动版本的结果必须稳定复现。
- 不写正式幂等记录；不新增 event inbox、participation、grant_order、outbox。

### 8. 幂等重复：正式事件接口重复逻辑不被模拟运行污染

执行：

- 先模拟一次事件样例。
- 再用同一业务事件调用正式 `POST /openapi/v1/events`，带正确 `Idempotency-Key`。
- 再重复调用正式事件接口一次。

断言：

- 模拟运行不占用正式幂等 key。
- 第一次正式事件仍按正常链路处理。
- 第二次正式事件返回 duplicate 语义，且不会因模拟运行产生误判。

### 9. 并发：多请求模拟同一事件不会互相影响

执行：

- 使用 `t.Parallel` 或 goroutine 并发提交同一事件样例，建议 20 到 100 个并发请求。
- 并发请求覆盖同一活动、同一 quota、同一 subject。

断言：

- 所有请求均返回一致的模拟预览，或在错误场景下返回一致的业务错误。
- 无数据竞争；使用 `go test -race` 执行相关包。
- quota 的 `used_value`、`frozen_value`、`version` 在并发前后完全一致。
- 正式业务表没有新增记录。

### 10. 并发：模拟运行不影响正式事件处理

执行：

- 并发发起多次模拟请求，同时发起一次正式事件请求。

断言：

- 正式事件处理结果只受正式事件自身影响，不受模拟请求提前扣 quota、冻结库存或写幂等影响。
- 正式事件完成后，真实业务表只包含正式事件产生的记录。
- 模拟请求返回的 quota 预览可以反映请求开始时或读取时的当前值，但不得执行写入。

### 11. PostgreSQL smoke：模拟成功路径无副作用

准备：

- 启动本地 PostgreSQL。
- 设置 `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'`。
- 使用 PostgreSQL 仓库创建活动和 quota。

执行：

- 调用一次成功命中的模拟运行。
- 通过测试代码或 SQL 查询模拟前后的相关表计数和 quota 值。

断言：

- `marketing_event_inbox` 行数不变。
- `marketing_participation` 行数不变。
- `marketing_grant_order` 行数不变。
- `marketing_outbox` 行数不变。
- `marketing_quota.used_value`、`frozen_value`、`version` 不变。
- 响应中仍包含完整 preview。

### 12. PostgreSQL smoke：quota 不足无副作用

准备：

- PostgreSQL 中设置预算或库存不足。

执行：

- 调用一次会超限的模拟运行。

断言：

- 响应明确 `wouldExceed=true`。
- 所有业务表计数不变。
- quota 数值和版本不变。
- 没有 outbox 真实消息进入待发送、重试或 DLQ 状态。

### 13. 输入校验：非法标准事件

执行：

- 缺少 `tenantId`、`eventType`、`subject.id`、`biz.bizId` 或 `occurredAt`。
- `attributes.amount` 使用无法按 Decimal 解析的值。

断言：

- 返回 `INVALID_EVENT` 或等价错误。
- 不进入活动模拟，不返回误导性的 matched 活动。
- 不写任何正式业务表。

### 14. 规则异常：字段缺失、类型不匹配、未知 evaluator

准备数据：

- 活动规则引用不存在字段。
- 活动规则存在类型不匹配条件。
- 如系统支持注册 evaluator，构造未注册 evaluator 的规则配置。

断言：

- 对应规则状态为 `FAILED` 或 `ERROR`，不能默认通过。
- 活动最终不得被判定为可发奖。
- 响应面向配置排查，不暴露内部堆栈。
- 无真实业务写入。

### 15. 活动范围：默认最小范围只要求 ACTIVE

准备数据：

- 同租户、同事件类型下各创建 ACTIVE、DRAFT、SCHEDULED、ENDED / PAUSED 活动。

断言：

- 本期默认无过滤条件时必须正确覆盖 ACTIVE 活动。
- DRAFT / SCHEDULED 是否参与模拟不作为本期阻塞项；若实现支持，应断言响应中状态和原因清晰。
- ENDED / PAUSED 或时间窗不满足的活动如出现在候选结果中，应给出未命中原因。

### 16. 多活动与互斥组

准备数据：

- 两个 ACTIVE 活动都满足规则，属于同一 `exclusiveGroup`，priority 不同，`stackable=false`。
- 另一个 ACTIVE 活动满足规则且允许 stackable。

断言：

- 互斥组中只保留优先级最高的活动为最终可发奖命中。
- 被互斥拒绝的活动出现在 unmatched / skipped 结果中并说明原因。
- stackable 活动可同时命中。
- 各活动的 ruleResults 和 rewardPreviews 不互相污染。

## Required Commands

Test Agent 在 Dev 完成后执行以下命令。若项目包名或新增测试脚本有变化，可补充更精确的包路径，但不得删除全量检查。

~~~bash
go test ./...
go test -race ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -race ./...
docker compose down
~~~

可选手工 HTTP smoke 命令，适用于 Dev 暴露接口后排查响应结构：

~~~bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
curl -sS -X POST http://localhost:8080/admin/v1/simulate -H 'content-type: application/json' -d '<standard-event-json>'
docker compose down
~~~

## Data / Smoke

PostgreSQL smoke 至少记录以下前后快照：

~~~sql
select count(*) from marketing_event_inbox;
select count(*) from marketing_participation;
select count(*) from marketing_grant_order;
select count(*) from marketing_outbox;
select quota_code, used_value, frozen_value, version from marketing_quota order by quota_code;
~~~

模拟运行后断言：

- 成功路径、quota 不足路径、非法事件路径都不得改变上述 count 和 quota 数值。
- 如果本期实现新增模拟专用表或后台审计表，必须先确认需求范围；否则不应写入任何真实业务结果表。
- PostgreSQL smoke 必须至少覆盖一次命中并产生 preview 的请求，以及一次 quota 不足的请求。

## Exit Criteria

PASS 条件：

- Required Commands 全部通过。
- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 均有自动化覆盖。
- 所有无副作用断言通过。
- 响应结构包含运营可定位问题的规则、奖励、quota 和发奖链路解释。

BLOCK 条件：

- 模拟运行写入真实 participation、grant_order、outbox、event inbox，或修改 quota。
- quota 不足仍表现为可正式发奖成功。
- 重复模拟污染正式幂等。
- 并发模拟导致数据竞争或 quota / 预览污染。
- PostgreSQL smoke 无法证明无副作用。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113400-8
