# Test Plan

## Agent 指令

你是 Test Agent。请在 Dev Agent 完成实现后执行本测试计划。

限制：

```text
1. 按本文件列出的测试用例补齐或执行测试。
2. 可以修改测试代码以补足覆盖，但不得改变业务语义来迁就测试。
3. 测试必须验证 PostgreSQL 事务 / 并发语义；内存仓储结果不能替代预算安全验收。
4. 执行命令前先确认本地测试依赖和数据库配置，避免误连生产环境。
```

## Test Strategy

本任务风险点是发奖回调后的预算闭环。测试必须围绕 `tenantId + commandId` 幂等键，验证 `grant_order`、quota reservation、quota 聚合值、`participation` 和审计 / 对账结果同成同败。

覆盖层级：

```text
1. HTTP / handler 层：校验请求模型、错误码、重复回调响应、跨 tenant 隔离。
2. Service / repository 层：验证状态机、reservation 终结、quota 聚合更新、participation 推进。
3. PostgreSQL 集成层：验证事务原子性、行锁或唯一约束、并发重复与成功失败竞争。
4. 对账查询层：验证能查询正常、异常、悬挂冻结、reservation missing 和冲突记录。
```

## Test Cases

### 正常路径

```text
TC-01 SUCCESS 首次回调
Given tenant T1 下存在 grant_order GO1，commandId=C1，状态 PENDING/SENDING/RETRY
And GO1 关联 FROZEN reservation，marketing_quota.frozen_value 已包含 reservedAmount
When POST /openapi/v1/reward/callback status=SUCCESS，externalGrantId 非空
Then grant_order=SUCCESS
And reservation=CONFIRMED
And quota.frozen_value 减少 reservedAmount
And quota.used_value 增加 reservedAmount
And 单 grant_order participation=GRANTED
And 响应不是 duplicate
And 审计 / 对账可见 CALLBACK_SUCCESS 与 QUOTA_CONFIRMED
```

```text
TC-02 FAILED 首次回调
Given GO2 关联 FROZEN reservation
When callback status=FAILED
Then grant_order=FAILED
And reservation=RELEASED
And quota.frozen_value 减少 reservedAmount
And quota.used_value 不增加
And 单 grant_order participation=FAILED
And 审计 / 对账可见 CALLBACK_FAILED 与 QUOTA_RELEASED
```

```text
TC-03 多 grant_order 全部成功
Given 同一 participation 下 GO3A/GO3B 均有 FROZEN reservation
When GO3A SUCCESS
Then participation 保持 REWARDABLE 或等价未最终成功状态
When GO3B SUCCESS
Then participation=GRANTED
And 两笔 reservation 均 CONFIRMED
```

```text
TC-04 多 grant_order 部分成功且仍有待处理
Given 同一 participation 下 GO4A/GO4B/GO4C
When GO4A SUCCESS，GO4B 仍 PENDING/SENDING/RETRY
Then participation 不得提前 GRANTED 或 FAILED
And GO4A 明细保持 SUCCESS/CONFIRMED
```

```text
TC-05 多 grant_order 最终部分成功部分失败
Given 同一 participation 下 GO5A/GO5B
When GO5A SUCCESS and GO5B FAILED
Then participation=FAILED
And GO5A reservation=CONFIRMED
And GO5B reservation=RELEASED
And 对账明细能同时展示成功与失败事实
```

### 额度不足 / 预算异常

```text
TC-06 回调时 quota 聚合不足导致无法确认
Given GO6 reservation=FROZEN，reservedAmount=100
And marketing_quota.frozen_value 小于 100 或聚合快照与 reservation 不一致
When SUCCESS callback
Then 返回 QUOTA_AGGREGATE_CONFLICT 或等价错误
And grant_order 不得变为 SUCCESS
And reservation 仍为 FROZEN 或进入明确 CONFLICT/MISSING 异常态
And quota.frozen_value / used_value 不得变为负数或被部分更新
```

```text
TC-07 回调时 quota 聚合不足导致无法释放
Given GO7 reservation=FROZEN，reservedAmount=100
And marketing_quota.frozen_value 小于 100 或聚合快照与 reservation 不一致
When FAILED callback
Then 返回 QUOTA_AGGREGATE_CONFLICT 或等价错误
And grant_order 不得变为 FAILED
And reservation 不得 RELEASED
And quota 聚合值不被部分更新
```

```text
TC-08 缺少 reservation 的 grant_order
Given GO8 存在且状态允许回调
And GO8 没有关联 reservation / ledger / voucher 等价凭证
When SUCCESS 或 FAILED callback
Then 返回 QUOTA_RESERVATION_MISSING
And 不得通过 aggregate frozen_value 猜测确认或释放金额
And grant_order、participation、quota 聚合值不更新
And 对账查询 reservationStatus=MISSING
```

```text
TC-09 reservation 金额与 grant_order 奖励金额不一致
Given GO9.rewardAmount 与 reservation.reservedAmount 不一致，且业务要求金额一致
When SUCCESS 或 FAILED callback
Then 返回 QUOTA_RESERVATION_CONFLICT
And 不得确认 / 释放 reservation
And quota 聚合值不更新
```

### 幂等重复

```text
TC-10 重复 SUCCESS
Given GO10 已经由首次 SUCCESS 处理完成
When 再次发送相同 tenantId + commandId 的 SUCCESS callback
Then 响应 duplicate=true 或等价标识
And grant_order 仍 SUCCESS
And reservation 仍 CONFIRMED
And quota.frozen_value / used_value 不再次变化
And participation 不重复推进
```

```text
TC-11 重复 FAILED
Given GO11 已经由首次 FAILED 处理完成
When 再次发送相同 tenantId + commandId 的 FAILED callback
Then 响应 duplicate=true 或等价标识
And reservation 仍 RELEASED
And quota.frozen_value / used_value 不再次变化
```

```text
TC-12 SUCCESS 后 FAILED 冲突
Given GO12 已 SUCCESS 且 reservation=CONFIRMED
When 发送 FAILED callback
Then 返回 CALLBACK_STATUS_CONFLICT 或 QUOTA_RESERVATION_CONFLICT
And grant_order 保持 SUCCESS
And reservation 保持 CONFIRMED
And quota 不释放已确认金额
And 对账 / 审计可见冲突记录
```

```text
TC-13 FAILED 后 SUCCESS 冲突
Given GO13 已 FAILED 且 reservation=RELEASED
When 发送 SUCCESS callback
Then 返回 CALLBACK_STATUS_CONFLICT 或 QUOTA_RESERVATION_CONFLICT
And grant_order 保持 FAILED
And reservation 保持 RELEASED
And quota.used_value 不增加
```

```text
TC-14 externalGrantId 冲突
Given GO14 已 SUCCESS 且 externalGrantId=E1
When 再次发送 SUCCESS callback，externalGrantId=E2
Then 如果实现定义 E2 为冲突，应返回 CALLBACK_STATUS_CONFLICT
And 不覆盖原 externalGrantId
And quota 不重复更新
```

### 输入校验与隔离

```text
TC-15 未知 commandId
When tenantId=T1, commandId=UNKNOWN callback
Then 返回 GRANT_ORDER_NOT_FOUND
And 不更新任何 grant_order、participation、reservation、quota
And 审计 / 日志可追踪 CALLBACK_UNKNOWN_COMMAND
```

```text
TC-16 跨 tenant commandId
Given T2 下存在 commandId=C16
When 使用 tenantId=T1 + commandId=C16 callback
Then 返回 GRANT_ORDER_NOT_FOUND
And 不泄露 T2 数据
And T2 的 grant_order、reservation、quota 不更新
```

```text
TC-17 非法请求
覆盖缺少 tenantId、缺少 commandId、缺少 status、status 非 SUCCESS/FAILED、SUCCESS 缺少 externalGrantId、缺少 finishedAt、finishedAt 不可解析、未知字段策略拒绝。
Then 返回 INVALID_CALLBACK_REQUEST
And 不更新任何业务数据
```

```text
TC-18 CLOSED grant_order
Given grant_order= CLOSED
When SUCCESS 或 FAILED callback
Then 返回 GRANT_ORDER_CLOSED 或等价错误
And 不重新打开 grant_order
And 不确认 / 释放 reservation
```

### 并发

```text
TC-19 并发重复 SUCCESS
Given GO19 reservation=FROZEN
When 20 个并发请求同时发送 SUCCESS callback
Then 只有一个请求执行 FROZEN -> CONFIRMED
And 其余请求返回 duplicate=true 或等价幂等结果
And quota.frozen_value 只减少一次
And quota.used_value 只增加一次
And reservation 只有一个终结时间或等价终结记录
```

```text
TC-20 并发重复 FAILED
Given GO20 reservation=FROZEN
When 20 个并发请求同时发送 FAILED callback
Then 只有一个请求执行 FROZEN -> RELEASED
And 其余请求返回 duplicate=true 或等价幂等结果
And quota.frozen_value 只减少一次
And quota.used_value 不增加
```

```text
TC-21 并发 SUCCESS 与 FAILED 竞争
Given GO21 reservation=FROZEN
When 10 个 SUCCESS 与 10 个 FAILED 并发到达
Then 最终只能落库一种最终态 SUCCESS/CONFIRMED 或 FAILED/RELEASED
And 另一类请求返回冲突或最终态幂等识别
And 不得出现 grant_order=SUCCESS 但 reservation=RELEASED
And 不得出现 grant_order=FAILED 但 reservation=CONFIRMED
And quota 聚合值与最终 reservation 状态一致
```

```text
TC-22 并发不同 grant_order 影响同一 quota
Given 同一 quota 下 GO22A/GO22B 各自有独立 FROZEN reservation
When 两笔 callback 并发完成，一笔 SUCCESS，一笔 FAILED
Then 每笔只终结自己的 reservation
And quota 聚合值等于两笔 delta 之和
And 不得互相释放或确认对方份额
```

### PostgreSQL Smoke

```text
TC-23 PostgreSQL 事务原子性 smoke
在 PostgreSQL 集成测试中模拟 callback 中途 repository 更新失败或约束冲突。
Then 不得出现 grant_order 已终态但 reservation / quota / participation 未同步的半提交数据。
```

```text
TC-24 PostgreSQL 锁与约束 smoke
在 PostgreSQL 上执行 TC-19、TC-20、TC-21。
Then 通过数据库真实事务、行锁、条件更新或唯一约束证明 reservation 只能终结一次。
内存仓储并发测试不能替代此项。
```

```text
TC-25 PostgreSQL 对账查询 smoke
准备 SUCCESS、FAILED、FROZEN 悬挂、MISSING、CONFLICT 五类数据。
When 调用对账查询接口或 repository 查询
Then 能按 tenantId、commandId、activityId、subjectId、grantStatus、reservationStatus 过滤
And 返回 grant_order、participation、callback、reservation、quota aggregate snapshot
And 对超过阈值的 FROZEN reservation 标记 isHangingFrozen=true 或等价字段
```

## Required Commands

Test Agent 应根据项目实际测试环境执行以下命令。此 Test Plan Agent 未执行这些命令。

```bash
# 单元测试与普通集成测试
go test ./...

# PostgreSQL 集成 / smoke 测试，环境变量名称以项目实际实现为准
EXMARKETING_TEST_DB_DSN='postgres://USER:PASSWORD@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Postgres|Callback|Quota|Reservation|Concurrency|Reconciliation' -count=1

# 并发与竞态辅助检查；如项目不支持 race 或存在已知不兼容项，需要在测试报告中说明
go test ./... -race -run 'Callback|Quota|Reservation|Concurrency' -count=1

# 构建验证
go build ./...
```

## Data / Smoke

测试数据至少准备以下种子：

```text
1. Tenant T1 / T2，用于验证 tenant 隔离。
2. Activity A1，quota total_value 足够，used_value=0，frozen_value 覆盖待回调 reservation。
3. 单 grant_order：C_SUCCESS、C_FAILED、C_DUP_SUCCESS、C_DUP_FAILED、C_CLOSED。
4. 多 grant_order participation：全成功、部分成功仍 pending、最终部分成功部分失败。
5. 异常 grant_order：缺少 reservation、reservation 金额不一致、quota aggregate 不足。
6. 长期悬挂：状态 PENDING/SENDING/RETRY 且 reservation=FROZEN，createdAt/updatedAt 早于查询阈值。
7. PostgreSQL 并发数据：每个并发 case 使用独立 commandId，避免测试间污染。
```

测试报告必须记录：

```text
1. 每个 TC 的 PASS / FAIL。
2. 失败时的请求 payload、响应错误码、关键表快照或查询结果。
3. PostgreSQL smoke 使用的数据库 DSN 是否为测试库。
4. 并发测试最终 grant_order、reservation、quota、participation 的落库快照。
```

## Decision

```text
PASS
```
