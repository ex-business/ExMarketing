# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

Review Decision 已确认 PASS。

本测试计划只定义后续 Test Agent 应执行的用例和命令清单，不在本阶段执行测试。

测试重点：

```text
1. 验证 participation 是所有发奖链路的强制前置凭证。
2. 验证 grant_order 必须绑定 participation，outbox 必须来源于 grant_order。
3. 验证幂等键唯一、重复事件不重复发奖、不重复扣 quota。
4. 验证 quota 不足、奖励为 0、规则拒绝等失败/跳过场景不会留下 REWARDABLE 脏记录。
5. 验证并发重复请求在 PostgreSQL 唯一约束和事务语义下不会产生重复 participation、grant_order、outbox 或重复扣减。
6. 验证失败原因至少能从 participation.rejectReason、grant_order.errorMessage、event_inbox.errorMessage、outbox.errorMessage 或 audit 中追踪。
```

测试层级：

```text
单元测试：覆盖状态枚举、状态流转、idempotency key 生成、reward payload / snapshot 基础字段。
集成测试：覆盖事件处理链路从 event_inbox 到 participation、grant_order、outbox 的事务行为。
PostgreSQL smoke：使用 Docker PostgreSQL 验证唯一约束、事务回滚、并发请求和 quota 扣减一致性。
回归命令：执行 go test ./... 与 go build ./...。
```

## Test Cases

### 1. Happy Path：有效事件生成可发奖链路

前置数据：

```text
活动启用，触发事件匹配。
规则全部通过。
至少一个 enabled reward 计算出 rewardAmount > 0。
BUDGET / STOCK / USER_LIMIT 均充足，或缺失 quota 行按无限制处理。
```

步骤：

```text
1. 发送一次有效标准事件。
2. 查询 event_inbox、marketing_participation、marketing_grant_order、outbox。
```

断言：

```text
event_inbox 非 duplicate，处理成功。
生成一条 participation，status=REWARDABLE。
participation 包含 tenantId、activityId、eventInboxId、subject、biz、snapshotJson、idempotencyKey。
每个实际 reward 生成一条 grant_order，grantStatus=PENDING。
grant_order.participationId 指向 participation。
grant_order 包含 rewardType、rewardAmount、rewardPayloadJson、billNo 或 commandId、grantChannel、idempotencyKey。
每条 outbox 指令能追溯到 grant_order，不允许直接从 reward 计算结果绕过 grant_order。
quota 扣减量与实际 reward 数量/配置一致。
```

### 2. Rule Rejected：规则拒绝只写 REJECTED participation

前置数据：

```text
活动匹配，但资格规则或风控规则不通过。
```

步骤：

```text
1. 发送触发活动但规则不通过的标准事件。
2. 查询 participation、grant_order、outbox、quota。
```

断言：

```text
生成 participation，status=REJECTED。
participation.rejectReason 包含 RULE_REJECTED 或可读拒绝原因。
不生成 grant_order。
不生成 outbox。
不扣 BUDGET / STOCK / USER_LIMIT。
```

### 3. Duplicate Event：完全重复请求幂等返回

步骤：

```text
1. 使用相同 Idempotency-Key 和完全相同 payload 发送有效事件。
2. 再发送一次完全相同请求。
3. 查询 event_inbox、participation、grant_order、outbox、quota。
```

断言：

```text
第二次响应 duplicate=true 或等价幂等响应。
event_inbox 不新增有效处理记录，或仅保留可识别 duplicate 的记录。
participation 数量不变。
grant_order 数量不变。
outbox 数量不变。
quota 不重复扣减。
```

### 4. Idempotency Conflict：同 key 不同 payload 拒绝处理

步骤：

```text
1. 使用某 Idempotency-Key 发送 payload A。
2. 使用同一 Idempotency-Key 发送 payload B，B 与 A 至少一个业务字段不同。
```

断言：

```text
第二次返回 INVALID_IDEMPOTENCY_KEY。
不新增 participation。
不新增 grant_order。
不新增 outbox。
不扣 quota。
失败原因可从 event_inbox.errorMessage 或响应中追踪。
```

### 5. Quota Exceeded：预算不足

前置数据：

```text
BUDGET 剩余额度低于本次 rewardAmount 或配置为 0。
规则通过，rewardAmount > 0。
```

步骤：

```text
1. 发送有效事件。
2. 查询 participation、grant_order、outbox、quota、audit 或 event_inbox。
```

断言：

```text
响应 QUOTA_EXCEEDED。
不保留 REWARDABLE participation。
不生成 grant_order。
不生成 outbox。
预算不变或事务回滚到请求前值。
失败原因可追踪，audit 中存在 QUOTA_EXCEEDED 或 event_inbox.errorMessage 包含明确原因。
```

### 6. Quota Exceeded：库存不足且多奖励不允许部分成功

前置数据：

```text
活动有多个 enabled reward。
至少一个 reward 的 STOCK 不足。
其他 reward 的预算和库存充足。
```

步骤：

```text
1. 发送有效事件。
2. 查询 grant_order、outbox、quota。
```

断言：

```text
响应 QUOTA_EXCEEDED。
同一 activity 下不生成任何 grant_order/outbox。
所有已尝试的预算、库存、用户次数扣减全部回滚。
不存在部分 reward 发奖成功、部分 reward 失败的中间状态。
```

### 7. Quota Exceeded：用户次数不足

前置数据：

```text
USER_LIMIT 已达到上限。
BUDGET / STOCK 充足。
规则通过，rewardAmount > 0。
```

步骤：

```text
1. 发送有效事件。
2. 查询 participation、grant_order、outbox、quota。
```

断言：

```text
响应 QUOTA_EXCEEDED。
不生成 grant_order/outbox。
不扣预算。
不扣库存。
用户次数不超过上限。
失败原因可追踪。
```

### 8. Zero Reward：奖励为 0 不进入发奖链路

前置数据：

```text
活动匹配，规则通过，但全部 enabled reward 计算结果为 0。
```

步骤：

```text
1. 发送有效事件。
2. 查询 participation、grant_order、outbox、quota。
```

断言：

```text
不生成 grant_order。
不生成 outbox。
不扣 quota。
不得写 status=REWARDABLE 的 participation。
如果实现保留 participation，状态必须是非 REWARDABLE 且原因可解释 ZERO_REWARD_SKIPPED。
```

### 9. Grant Order Traceability：发奖单可完整追溯

步骤：

```text
1. 基于 Happy Path 生成发奖链路。
2. 从任意 grant_order 出发反查 participation、event_inbox、activity、subject、biz 和 rewardPayloadJson。
```

断言：

```text
grant_order.participationId 可定位唯一 participation。
participation.eventInboxId 可定位原始事件。
tenantId、activityId、subjectType、subjectId、bizType、bizId 前后一致。
rewardPayloadJson 包含 rewardCode、rewardType、calculateType、currency 或必要发奖目标引用。
billNo / commandId 稳定且唯一，可用于回调定位唯一发奖单。
```

### 10. State Coverage：状态枚举与禁止流转

步骤：

```text
1. 检查 participation 状态枚举支持 INIT / QUALIFIED / REJECTED / REWARDABLE / GRANTED / FAILED。
2. 检查 grant_order 状态枚举支持 PENDING / SENDING / SUCCESS / FAILED / RETRY / CLOSED。
3. 若代码提供状态流转函数或 repository 更新方法，覆盖允许和禁止流转。
```

断言：

```text
允许 INIT -> QUALIFIED -> REWARDABLE -> GRANTED。
允许 INIT -> QUALIFIED -> REWARDABLE -> FAILED。
允许 INIT -> REJECTED、QUALIFIED -> REJECTED、REWARDABLE -> FAILED。
禁止 REJECTED -> REWARDABLE、REJECTED -> GRANTED。
允许 PENDING -> SENDING -> SUCCESS。
允许 PENDING -> SENDING -> FAILED -> RETRY -> SENDING。
允许 PENDING -> CLOSED、FAILED -> CLOSED、RETRY -> CLOSED。
禁止 SUCCESS -> SENDING、SUCCESS -> RETRY、CLOSED -> SENDING。
```

### 11. Concurrency：重复事件并发请求只处理一次

前置数据：

```text
使用 PostgreSQL 集成环境。
活动、规则、reward、quota 均满足 Happy Path。
```

步骤：

```text
1. 并发发送 N 个相同 Idempotency-Key 和相同 payload 的请求，建议 N >= 20。
2. 等待全部请求返回。
3. 查询 event_inbox、participation、grant_order、outbox、quota。
```

断言：

```text
最多一个请求完成真实发奖链路。
其余请求返回 duplicate=true 或等价幂等响应。
participation 仅一条。
每个 reward 的 grant_order 仅一条。
outbox 数量等于实际 reward 数，不随并发倍增。
quota 只扣一次。
无唯一键冲突泄漏为 500。
```

### 12. Concurrency：并发额度竞争不能超扣

前置数据：

```text
使用 PostgreSQL 集成环境。
活动规则通过。
quota 只够一个或固定少量请求成功。
多组请求使用不同 Idempotency-Key 和不同 bizId。
```

步骤：

```text
1. 并发发送 N 个不同业务事件，建议 N >= 20。
2. 查询成功数量、QUOTA_EXCEEDED 数量、quota 余额、grant_order、outbox。
```

断言：

```text
成功请求数不超过 quota 可承载数量。
失败请求返回 QUOTA_EXCEEDED。
quota 不为负数。
grant_order/outbox 数量与成功请求数和 reward 数一致。
失败请求不保留 REWARDABLE participation，不生成 grant_order/outbox。
```

### 13. PostgreSQL Smoke：唯一约束与事务回滚

前置数据：

```text
使用 Docker PostgreSQL，不使用内存仓储替代事务验收。
```

步骤：

```text
1. 执行数据库迁移或启动测试所需 schema。
2. 验证 participation.idempotency_key 唯一约束。
3. 验证 grant_order.idempotency_key 唯一约束。
4. 注入 grant_order 创建失败或 outbox 创建失败场景。
5. 查询 participation、grant_order、outbox、quota。
```

断言：

```text
重复 participation.idempotency_key 不能插入第二条有效记录。
重复 grant_order.idempotency_key 不能插入第二条有效记录。
grant_order 创建失败时，同一 activity 的 participation、quota、outbox 全部回滚。
outbox 创建失败时，同一 activity 的 participation、quota、grant_order 全部回滚。
数据库最终状态可对账，无半成品发奖链路。
```

### 14. Sensitive Data：快照和 payload 不保存敏感大对象

步骤：

```text
1. 构造包含 Authorization、token、secret 或较大无关 attributes 的事件。
2. 触发 Happy Path 或 Rule Rejected。
3. 查询 participation.snapshotJson 和 grant_order.rewardPayloadJson。
```

断言：

```text
snapshotJson 能解释活动版本、规则摘要、奖励摘要、事件摘要。
rewardPayloadJson 能解释发奖配置和计算结果。
不保存 Authorization header、token、secret 或完整无关外部业务大对象。
```

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

PostgreSQL smoke 建议命令清单由 Test Agent 根据仓库现有脚本选择。若仓库已有 Docker Compose 或集成测试脚本，优先使用现有脚本；否则按以下目标补齐等价命令：

~~~bash
# 启动 PostgreSQL 测试库，例如使用仓库已有 compose 文件或临时 postgres 容器
# 执行 schema migration
# 执行 PostgreSQL 集成测试包，至少覆盖事务、唯一约束、并发 quota
# 停止并清理 PostgreSQL 测试库
~~~

并发测试建议命令清单：

~~~bash
# 运行包含 -race 的相关 Go 测试包，若当前仓库支持
go test -race ./...

# 运行指定并发集成测试，具体包名由 Test Agent 按实现后的测试文件确定
# go test ./<integration-test-package> -run 'Test.*Concurrent|Test.*Idempotency|Test.*Quota' -count=1
~~~

## Data / Smoke

PostgreSQL smoke 数据集至少包含：

```text
tenant: t_test_lifecycle
subject: USER / user_lifecycle_001
biz: ORDER / order_lifecycle_001
sourceSystem: test-suite
eventType: ORDER_PAID 或仓库现有等价事件类型
activity: enabled，匹配上述 eventType
reward A: enabled，rewardAmount > 0，grantChannel 使用测试通道
reward B: enabled，用于 multi reward all-or-nothing
budget quota: 分别准备充足、不足两组
stock quota: 分别准备充足、不足两组，scopeKey 按 rewardCode
user limit quota: 分别准备未达上限、已达上限两组
```

smoke 查询点：

```text
marketing_event_inbox：幂等状态、duplicate、errorMessage。
marketing_participation：status、rejectReason、snapshotJson、idempotencyKey、eventInboxId。
marketing_grant_order：participationId、grantStatus、rewardAmount、rewardPayloadJson、commandId/billNo、errorMessage、idempotencyKey。
outbox 表：来源 grant_order、状态、payload、errorMessage。
quota / ledger 类表：预算、库存、用户次数扣减与回滚。
marketing_audit：QUOTA_EXCEEDED、IDEMPOTENCY_CONFLICT 或等价失败原因。
```

最低通过标准：

```text
1. go test ./... 通过。
2. go build ./... 通过。
3. PostgreSQL smoke 通过，且不以内存仓储替代事务验收。
4. Happy Path、额度不足、幂等重复、并发、PostgreSQL 唯一约束与事务回滚全部覆盖。
5. 无任何测试显示 grant_order 或 outbox 可绕过 participation 创建。
```

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113400-5
