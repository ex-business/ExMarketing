# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以 `POST /openapi/v1/events` 接口级测试覆盖标准事件 JSON Schema、错误码和幂等键分流。
- 复用现有内存仓储和活动创建 helper，验证合法事件仍能进入既有 event inbox、规则、发奖和 outbox 流程。
- 使用表驱动测试覆盖缺字段、类型错误、未知字段、非法时间、非法 `subject`、非法 `biz`、非法 `attributes`。
- 对错误响应至少断言 HTTP 400、`code` 和 `error` 中的字段路径或原因。
- 保留重复事件和幂等冲突测试，确保 Schema 错误不混入 `INVALID_IDEMPOTENCY_KEY`。
- 覆盖模拟运行入口，确保标准事件 payload 与正式 Event API 口径一致。

## Test Cases

1. 合法事件通过：8 个标准顶层字段完整，`attributes` 含业务字段和嵌套 object/array，返回 `202 Accepted`。
2. 空 `attributes` 通过：`attributes: {}` 返回 `202 Accepted`。
3. 缺少任一必填字段：至少覆盖 `eventType`、`biz`、`occurredAt`、`attributes`，返回 `400 INVALID_EVENT`。
4. 顶层未知字段：例如 `orderAmount` / `amount` 放在顶层，返回 `400 INVALID_EVENT`，错误包含字段名。
5. `subject` 非法：`subject` 非 object、缺 `id`、包含 `subject.name` / `subject.vipLevel`，返回 `400 INVALID_EVENT`。
6. `biz` 非法：缺 `bizId`、包含 `biz.amount`，返回 `400 INVALID_EVENT`。
7. `occurredAt` 非法：日期不可解析或缺少时区，返回 `400 INVALID_EVENT`。
8. `attributes` 非法：缺失、`null`、array、string、number、boolean 均返回 `400 INVALID_EVENT`。
9. 幂等键缺失或与 payload 不匹配，返回 `400 INVALID_IDEMPOTENCY_KEY`。
10. 完全重复事件返回 `200 OK` 且 `duplicate=true`；同 key 不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`。
11. 模拟运行提交非法标准事件 payload 返回 `400 INVALID_EVENT`。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用现有 `DEPOSIT_SUCCESS` 活动样例和 `site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:<bizId>` 幂等键。
- 不需要 PostgreSQL Docker 验证；本次变更集中在 HTTP 解码、内存仓储接口级测试和文档，不改变数据库持久化语义。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-schema
