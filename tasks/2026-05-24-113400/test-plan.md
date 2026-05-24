# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

review.md Decision 已为 PASS，允许进入测试设计。

本轮测试目标是锁定标准事件入口的幂等、字段边界和发奖副作用边界。测试分四层：

- HTTP API：验证 `POST /openapi/v1/events` 的 Header 必填、标准事件模型、错误码、首次接收、重复接收和幂等冲突。
- Engine + Memory Repository：验证重复事件短路，不重复匹配活动、不重复生成 participation/grant_order/outbox、不重复扣 quota。
- Repository 语义一致性：Memory 与 PostgreSQL 对 `created / duplicate / idempotency conflict` 的行为一致，且 payload 比较基于标准化事件而不是原始 JSON 字符串。
- PostgreSQL smoke：用真实 PostgreSQL 验证 `marketing_event_inbox.idempotency_key` 唯一约束、重复事件去重、幂等冲突和并发写入行为。

测试必须覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。若实现无法稳定表达幂等冲突或 PostgreSQL 与 Memory 行为不一致，则测试计划结论应视为 BLOCK；当前基于 analysis/review 约束，测试设计可执行，Decision 为 PASS。

## Test Cases

### HTTP Event API

1. 首次合法事件正常路径

- 准备一个包含活动匹配条件和奖励配置的测试活动，事件 payload 只包含 `eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt/attributes`。
- 请求 Header 携带正确 `Idempotency-Key = tenantId:sourceSystem:eventType:bizType:bizId`。
- 期望：HTTP `202 Accepted`；响应 `duplicate=false`；写入 1 条 inbox；生成 1 条 participation、1 条 grant_order、1 条 outbox；quota 按一次扣减。

2. 缺少 `Idempotency-Key`

- 使用合法 payload，但不传 Header。
- 期望：HTTP `400`；错误码为稳定客户端错误，例如 `INVALID_IDEMPOTENCY_KEY`；不写 inbox；不生成 participation/grant_order/outbox。

3. `Idempotency-Key` 与 payload 不匹配

- Header 使用不同 tenant/source/eventType/biz 组合生成的 key。
- 期望：HTTP `400`；不写 inbox；不进入发奖链路。

4. 顶层未知字段被拒绝

- payload 增加标准模型之外的顶层字段，例如 `orderAmount`。
- 期望：HTTP `400`；不写 inbox。

5. 嵌套未知字段被拒绝

- `subject` 增加 `name` 或 `biz` 增加 `extra`。
- 期望：HTTP `400`；不写 inbox。

6. `attributes` 允许业务字段

- `attributes` 内放入数字、字符串和布尔业务字段。
- 期望：HTTP `202 Accepted`；规则和奖励只能通过 `attributes.*` 读取这些业务字段。

7. `attributes` 缺失或为 `null`

- 分别提交缺失 `attributes` 和 `attributes: null` 的合法事件。
- 期望：若实现采纳 analysis 建议，均归一化为 `{}` 后处理；读回 inbox payload 时为稳定空对象；重复提交不因 `nil`/`{}` 差异被误判为冲突。

8. 缺少必填字段

- 分别缺少 `eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt`。
- 期望：HTTP `400 INVALID_EVENT`；不写 inbox。

### Idempotency And Duplicate Handling

9. 完全重复事件

- 连续提交两次完全相同事件和相同 `Idempotency-Key`。
- 期望：第一次 `202`，第二次 `200 OK` 且 `duplicate=true`；inbox 仍为 1 条；participation/grant_order/outbox 数量不增加；quota 不重复扣减。

10. JSON 顺序不同但语义相同

- 第二次提交同一标准事件，但调整 JSON 字段顺序、`attributes` 内字段顺序。
- 期望：判定为 duplicate，而不是幂等冲突。

11. 数字 attributes 的重复事件

- `attributes` 包含数字字段，例如 `amount: 12.34` 或整数 `count: 2`。
- 第二次从 PostgreSQL 读回已有 payload 后再比较。
- 期望：不会因为 `json.Number` 与 `float64` 表示差异误判为冲突；重复事件返回 `duplicate=true`。

12. 同一幂等键但 payload 不一致

- 保持 tenant/source/eventType/biz 不变，修改 `eventId`、`subject.id`、`occurredAt` 或 `attributes.amount` 任一字段。
- 期望：HTTP 返回明确客户端错误，按 analysis/review 推荐为 `400 INVALID_IDEMPOTENCY_KEY`；不新增 participation/grant_order/outbox；不扣 quota。

13. Memory repository 与 PostgreSQL repository 行为一致

- 对首次写入、完全重复、相同 key 不同 payload 分别运行同一组 repository 级测试。
- 期望：两种仓储都能表达 `created / duplicate / conflict`，且上层 HTTP/engine 映射一致。

### Quota / Insufficient Capacity

14. 额度不足不发奖

- 创建预算、库存或用户限制不足的活动，让事件能匹配活动但 quota 检查失败。
- 期望：HTTP 保持现有 `409 QUOTA_EXCEEDED` 语义；inbox 已记录首次事件；不生成 grant_order/outbox；participation 状态或记录方式符合现有设计；quota 不出现负数。

15. 额度不足后的重复事件

- 对同一额度不足事件再次提交完全相同 payload 和同一 key。
- 期望：第二次按 duplicate 短路，不重复执行 quota 检查，不新增 participation/grant_order/outbox。

### Concurrency

16. 并发提交同一合法事件

- 使用 `N >= 20` 个 goroutine 同时提交相同 payload 和相同 `Idempotency-Key`。
- 期望：最多 1 个请求进入首次处理路径；其余返回 duplicate；最终 inbox=1、participation=1、grant_order=1、outbox=1；quota 只扣一次；无 data race。

17. 并发提交同一 key 不同 payload

- 使用同一 `Idempotency-Key` 并发提交两组 payload，其中一组为原始事件，另一组修改 `attributes` 或 `eventId`。
- 期望：最终只有一个 payload 成为已保存 inbox；相同 payload 请求为 success/duplicate；不同 payload 请求为 idempotency conflict；不重复发奖。

18. 并发不同幂等键

- 并发提交多个不同 `bizId` 的合法事件。
- 期望：各事件互不影响；每个 key 最多处理一次；总 grant/outbox 数量等于不同 key 数量，受 quota 限制时按 quota 结果稳定。

### Field Boundary / Rule And Reward

19. 规则字段只读取标准字段和 `attributes.*`

- 覆盖 `eventId/eventType/sourceSystem/tenantId/subject.type/subject.id/biz.bizType/biz.bizId/occurredAt/attributes.*`。
- 期望：标准路径可读取；不存在的非标准顶层路径返回 not found 或规则不匹配，不触发外部业务字段读取。

20. 奖励 `baseField` 只允许 `attributes.*`

- `baseField=attributes.amount` 正常计算。
- `baseField=biz.amount`、`baseField=subject.id` 或其他非 `attributes.*` 路径返回明确错误，不生成发奖单。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

并发相关测试建议额外执行：

~~~bash
go test -race ./...
~~~

## Data / Smoke

### PostgreSQL Smoke

使用真实 PostgreSQL 跑一次集成级 smoke，验证 schema、唯一约束和 JSON payload 比较：

~~~bash
docker compose up -d postgres
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

PostgreSQL smoke 必须至少确认：

- `marketing_event_inbox.idempotency_key` 唯一约束存在且生效。
- 首次合法事件写入 inbox 后进入正常参与和发奖链路。
- 完全重复事件不会新增 participation/grant_order/outbox。
- 同一 key 不同 payload 返回幂等冲突，不静默 duplicate。
- 包含数字 `attributes` 的重复事件不会因 JSON 数字类型差异误判冲突。
- 并发同 key 写入最终只有一条 inbox，发奖副作用只发生一次。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113400
