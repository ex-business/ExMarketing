# Test Plan

## Agent 指令

你是 Test Plan Agent。Review 已 PASS。本文件只设计后续 Test Agent 应执行的测试用例和命令清单。

只允许 Test Agent 在后续阶段修改/补充测试；本阶段不改业务代码、不改测试代码、不执行测试/构建/格式化/git 命令。

## Scope

验证 `MATCH` 活动匹配阶段：

- 查询边界：先按 `tenantId`，再按 `triggerEvents` 包含当前 `eventType`。
- 输出兼容：活动级 `decision` 只能使用 `PASSED / REJECTED / SKIPPED / ERROR`，不得新增 `MATCHED / UNMATCHED`。
- 未命中解释：仅解释同租户且触发当前事件类型的活动。
- 正常路径：状态、时间、入口、人群、MATCH 规则通过后进入 `matchedActivities`。
- 失败路径：状态、时间、入口、人群、MATCH 规则、配置错误、互斥裁剪、额度不足、幂等重复、并发。
- 持久化：内存仓储与 PostgreSQL 仓储都要覆盖 smoke。

## Test Strategy

1. 单元测试优先覆盖纯 MATCH 判定，放在 `internal/marketing` 或实际新增 MATCH service 所在 package。
2. HTTP/simulate 测试覆盖 `/admin/v1/simulate` 响应结构、summary 计数、无副作用。
3. 事件入口测试覆盖 `/openapi/v1/events` 的幂等重复、额度不足、并发提交，并确认 MATCH 结果不会破坏后续链路一致性。
4. PostgreSQL smoke 使用真实 `EX_MARKETING_DATABASE_URL`，验证活动查询边界、幂等唯一约束、并发和 quota/participation/outbox 持久化结果。
5. 每个测试数据使用唯一 prefix，避免污染共享库；PostgreSQL 测试需在 `t.Cleanup` 中清理。

## Test Cases

### A. 正常路径

- A1: 同租户、`triggerEvents` 包含事件类型、`status=ACTIVE`、`startTime <= occurredAt < endTime`、入口允许、人群允许、无失败 MATCH 规则。
  - 期望：活动出现在 `matchedActivities`。
  - 期望：`decision=PASSED`，`decisionReasonCode` 为空或为通过语义，不出现 `MATCHED`。
  - 期望：`summary.candidateCount=1`、`matchedCount=1`、`unmatchedCount=0`。
- A2: `display.visible=false` 但其他 MATCH 条件通过。
  - 期望：不因展示字段被拒绝，仍可 `PASSED`。
- A3: 多个非互斥活动全部通过。
  - 期望：全部进入 `matchedActivities`，按 `priority DESC, createdAt ASC, activityId ASC` 稳定排序。

### B. 查询边界与未命中解释

- B1: 存在跨租户活动，配置完全匹配当前事件但 `tenantId` 不同。
  - 期望：跨租户活动不出现在 `matchedActivities`、`unmatchedActivities`、summary 计数中。
  - 期望：响应中不输出 `TENANT_NOT_MATCHED`。
- B2: 同租户活动的 `triggerEvents` 不包含当前 `eventType`。
  - 期望：该活动不出现在 `matchedActivities`、`unmatchedActivities`、summary 计数中。
  - 期望：响应中不输出 `EVENT_TYPE_NOT_MATCHED`。
- B3: 无可解释活动。
  - 期望：两个活动列表均为空，`candidateCount/matchedCount/unmatchedCount` 全为 0。

### C. 基础拒绝原因

- C1: `status` 为 `DRAFT / SCHEDULED / PAUSED / ENDED`。
  - 期望：进入 `unmatchedActivities`，`decision=REJECTED`，`decisionReasonCode=ACTIVITY_NOT_ACTIVE`。
- C2: `occurredAt < startTime`。
  - 期望：`REJECTED/ACTIVITY_NOT_STARTED`。
- C3: `occurredAt == startTime`。
  - 期望：时间检查通过。
- C4: `occurredAt == endTime`。
  - 期望：`REJECTED/ACTIVITY_ENDED`。
- C5: `startTime >= endTime`、时间缺失或不可解析。
  - 期望：`ERROR/INVALID_MATCH_CONFIG`，不得默认通过。

### D. 入口与人群拒绝

- D1: `config.entry.enabled=false`。
  - 期望：`REJECTED/ENTRY_DISABLED`。
- D2: `entry.channels` 非空但事件 `attributes.channel` 缺失或不命中。
  - 期望：`REJECTED/ENTRY_CHANNEL_NOT_ALLOWED`。
- D3: `entry.sourceSystems` 非空但事件 `sourceSystem` 不命中。
  - 期望：`REJECTED/ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`。
- D4: `entry.tenantIds` 非空且不包含当前事件 `tenantId`。
  - 期望：`REJECTED/ENTRY_TENANT_NOT_ALLOWED`；不得因此查询其他租户活动。
- D5: `entry.audience.subjectTypes` 不包含 `subject.type`。
  - 期望：`REJECTED/AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED`。
- D6: `entry.audience.segments` 与 `attributes.segment` 单值、`attributes.segments` 数组均无交集。
  - 期望：`REJECTED/AUDIENCE_SEGMENT_NOT_ALLOWED`。
- D7: `entry.audience.riskLevels` 不包含 `attributes.riskLevel`。
  - 期望：`REJECTED/AUDIENCE_RISK_LEVEL_NOT_ALLOWED`。

### E. MATCH 规则与配置错误

- E1: 启用的 `rule_phase=MATCH` 规则条件不通过。
  - 期望：`REJECTED/MATCH_RULE_REJECTED`，规则解释中保留失败规则。
- E2: 禁用的 MATCH 规则不影响活动通过。
  - 期望：活动级仍可 `PASSED`；规则级可标记 `SKIPPED`。
- E3: `QUALIFY / REWARD / RISK_LIMIT_BUDGET` 等非 MATCH 阶段规则不参与 MATCH 拒绝。
  - 期望：MATCH 结果不被这些阶段规则改变。
- E4: MATCH 规则字段缺失、类型不匹配、非法操作符或 evaluator 未注册。
  - 期望：`ERROR/INVALID_MATCH_CONFIG` 或按实现约定的 `REJECTED/MATCH_RULE_REJECTED`，但不得默认通过。
- E5: `config.entry` 或 `audience` JSON 类型错误。
  - 期望：`ERROR/INVALID_MATCH_CONFIG`。

### F. 互斥、优先级与稳定性

- F1: `exclusiveGroup` 为空的多个候选。
  - 期望：不互斥，全部保留。
- F2: 同一非空 `exclusiveGroup` 内全部 `stackable=true`。
  - 期望：全部保留。
- F3: 同一非空 `exclusiveGroup` 内存在任一 `stackable=false`。
  - 期望：仅保留 `priority DESC, createdAt ASC, activityId ASC` 排序第一活动。
  - 期望：其他同组候选进入 `unmatchedActivities`，`REJECTED/EXCLUSIVE_GROUP_LOST`，解释中标明胜出活动。
- F4: 同优先级、同组、同 `createdAt` 的候选。
  - 期望：按 `activityId ASC` 稳定裁剪；连续运行结果一致。

### G. 额度不足

- G1: 事件 MATCH 成功，但后续 activity budget 不足。
  - 期望：MATCH 结果仍显示活动已通过；后续 quota/budget 阶段拒绝或返回额度不足错误。
  - 期望：不创建成功态 `grant_order`，不创建待发送成功 outbox，quota 使用量不被错误扣减。
- G2: 用户次数额度不足或库存额度不足。
  - 期望：行为同 G1；错误归属后续 quota/limit 阶段，不反向改写 MATCH 的 `decision`。
- G3: `/admin/v1/simulate` 覆盖额度不足预览。
  - 期望：模拟不写入 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。

### H. 幂等重复

- H1: `/openapi/v1/events` 首次提交匹配成功。
  - 期望：返回成功，创建一次 inbox/participation/grant/outbox 或按后续阶段配置创建对应记录。
- H2: 使用相同 `Idempotency-Key` 和等价 payload 重复提交。
  - 期望：返回 `200 OK` 且 `duplicate=true`。
  - 期望：不重复创建 participation、grant_order、outbox，不重复扣 quota。
- H3: 使用相同 `Idempotency-Key` 但不同 payload。
  - 期望：返回 `400 INVALID_IDEMPOTENCY_KEY` 或项目既有冲突错误。
  - 期望：不创建第二套业务记录。
- H4: 重复事件的 MATCH 展示或处理结果保持业务一致。
  - 期望：幂等由 Event Intake 处理，MATCH 不引入随机结果。

### I. 并发

- I1: 并发 N 次提交完全相同的事件与 `Idempotency-Key`。
  - 期望：最多一次成功创建业务记录，其余返回 duplicate 或等价幂等结果。
  - 期望：无 data race；`go test -race` 下通过。
- I2: 并发提交不同 `bizId`、同一活动、共享有限 budget/user limit/stock。
  - 期望：成功数量不超过额度；失败请求为额度不足；quota 使用量、participation、grant_order、outbox 数量一致。
- I3: 并发模拟同一事件。
  - 期望：响应一致，且无任何持久化副作用。

### J. PostgreSQL Smoke

- J1: PostgreSQL 下创建多租户活动，执行 MATCH/simulate。
  - 期望：只返回当前租户且当前事件类型的活动，summary 计数正确。
- J2: PostgreSQL 下验证正常事件链路。
  - 期望：inbox、participation、grant_order、outbox 与 quota 变化符合内存仓储相同行为。
- J3: PostgreSQL 下验证额度不足事务。
  - 期望：失败时 participation/grant_order/outbox 不残留，quota 不发生部分扣减。
- J4: PostgreSQL 下验证幂等重复与冲突。
  - 期望：重复 payload 命中 existing inbox；冲突 payload 返回错误。
- J5: PostgreSQL 下验证并发同 key 与共享 quota。
  - 期望：唯一约束和事务锁保证结果一致，无重复发奖。

## Required Commands

后续 Test Agent 在实现完成后按顺序执行：

~~~bash
go test ./internal/marketing
go test ./internal/rule
go test ./internal/httpapi
go test ./internal/engine
go test ./...
go test -race ./internal/marketing ./internal/httpapi ./internal/engine
go build ./...
~~~

PostgreSQL smoke 命令：

~~~bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/httpapi ./internal/engine
docker compose down
~~~

如 Test Agent 新增了专门的集成测试 build tag，应额外执行对应命令，例如：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -tags=integration ./...
~~~

## Data / Smoke

建议测试数据最小集合：

- `tenant_a` 当前事件：`eventType=DEPOSIT_SUCCESS`、`sourceSystem=payment`、`subject.type=USER`、`bizType=DEPOSIT`、`occurredAt=2026-05-24T12:00:00Z`。
- `tenant_b` 跨租户活动：与事件完全匹配但不得返回。
- 同租户非触发活动：`triggerEvents=["WITHDRAW_SUCCESS"]`，不得返回。
- 正常活动：`ACTIVE`、有效时间覆盖 occurredAt、入口和人群全部命中。
- 入口拒绝活动：分别覆盖 disabled、channel、sourceSystem、tenantIds。
- 人群拒绝活动：分别覆盖 subjectType、segments、riskLevel。
- MATCH 规则活动：一条通过、一条失败、一条禁用规则、一条非法规则。
- 互斥组活动：空组、全 stackable、含非 stackable、同优先级 tie-breaker。
- 额度活动：budget/user limit/stock 刚好够、刚好不够、并发争用。
- 幂等事件：同 key 同 payload、同 key 不同 payload、并发同 key。

Smoke 验证必须记录：

- 响应中的 `matchedActivities`、`unmatchedActivities` 和 summary。
- 所有活动级 `decision` 均属于 `PASSED / REJECTED / SKIPPED / ERROR`。
- 未出现常规响应原因码 `TENANT_NOT_MATCHED`、`EVENT_TYPE_NOT_MATCHED`。
- 模拟接口执行前后持久化表计数不变。
- 额度不足和并发失败时无部分写入、无重复扣减。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-match
