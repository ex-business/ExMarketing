# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

- 已在 `POST /openapi/v1/events` 强制校验 `Idempotency-Key` 必填，且必须匹配标准事件 payload 计算出的幂等键。
- HTTP 层先校验事件必填字段，再比较 payload 幂等键；带 Header 但缺少标准必填字段时稳定返回 `INVALID_EVENT`。
- 标准事件在处理前归一化：`occurredAt` 统一 UTC，`attributes` 缺失或为 `null` 时归一化为 `{}`。
- inbox 写入保持首次处理、完全重复短路、相同 key 不同 payload 冲突三种语义；Memory 与 PostgreSQL 仓储行为一致。
- payload 比较使用标准化 canonical 事件结构，避免 JSON 字段顺序和 `json.Number` / `float64` 数字表示差异导致误判。
- 根据 Test Agent 反馈修正 PostgreSQL duplicate/conflict smoke 用例：重复路径现在使用真实 JSON 数字的 `json.Number` 与 `float64` 表示差异，不再把字符串 `"12.34"` 与 `"12.3400"` 误当作同一 payload。
- 规则字段边界补齐 `occurredAt`，奖励 `baseField` 继续限制为 `attributes.*`。
- README 和 API 文档已同步 Header 必填、标准字段边界、重复响应和幂等冲突语义。

## Changed Files

- `internal/httpapi/handler.go`
- `internal/engine/service.go`
- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/rule/evaluator.go`
- `internal/httpapi/handler_test.go`
- `internal/marketing/memory_repository_test.go`
- `internal/postgres/repository_integration_test.go`
- `internal/rule/evaluator_test.go`
- `internal/reward/calculator_test.go`
- `README.md`
- `docs/api.md`

## Behavior Changes

- 缺少 `Idempotency-Key`：返回 `400 INVALID_IDEMPOTENCY_KEY`，不写入 inbox。
- 带 `Idempotency-Key` 但缺少事件必填字段：返回 `400 INVALID_EVENT`，不写入 inbox。
- `Idempotency-Key` 与 payload 不匹配：返回 `400 INVALID_IDEMPOTENCY_KEY`，不写入 inbox。
- 首次合法事件：返回 `202 Accepted`，写入 inbox 并进入活动匹配、参与和发奖链路。
- 完全重复事件：返回 `200 OK` 且 `duplicate=true`，不会重复创建 participation、grant_order 或 outbox。
- 相同幂等键但 payload 不一致：返回 `400 INVALID_IDEMPOTENCY_KEY`，不会静默吞掉冲突事件。
- 顶层、`subject`、`biz` 中的未知字段由 HTTP 解码拒绝并返回 `400 INVALID_EVENT`；业务扩展字段仅允许放在 `attributes`。

## Notes

- 已执行 `go test ./...`。
- 已执行 `go build ./...`。
- 已执行 `docker compose up -d postgres`。
- 已执行 `EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...`，真实 PostgreSQL 集成路径通过。

需求：2026-05-24-113400
