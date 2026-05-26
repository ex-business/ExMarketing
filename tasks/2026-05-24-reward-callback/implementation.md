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

- 补齐 reward callback 的可配置 HMAC-SHA256 签名校验：配置了 tenant callback secret 时，请求必须携带 `X-ExMarketing-Timestamp` 和 `X-ExMarketing-Signature`，验签失败不会进入业务更新。
- 扩展 callback locator：支持 `billNo` / `bill_no` / `commandId` 定位 `marketing_grant_order.bill_no`，支持 `idempotencyKey` / `idempotency_key` 定位 `marketing_grant_order.idempotency_key`。
- 补齐 locator 冲突语义：多个 locator 指向不同 grant order，或请求 locator 与命中单据不一致时返回 `CALLBACK_LOCATOR_CONFLICT`。
- 保留既有状态机约束：callback 只终结 `SENDING` / `RETRY` grant order；重复同终态幂等，相反终态和不同 `externalGrantId` 冲突。
- 同步 API 文档和测试计划，明确签名、定位、错误码、重复回调与 quota reservation 行为。

## Changed Files

- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/engine/service.go`
- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `docs/api.md`
- `README.md`
- `tasks/2026-05-24-reward-callback/test-plan.md`
- `tasks/2026-05-24-reward-callback/implementation.md`

## Behavior Changes

- `POST /openapi/v1/reward/callback` 现在要求 JSON Content-Type。
- 配置 callback secret 的 tenant 必须先通过 timestamp + raw body HMAC 验签；缺签名返回 `CALLBACK_SIGNATURE_MISSING`，时间戳非法/过期返回 `CALLBACK_TIMESTAMP_INVALID`，签名不匹配返回 `CALLBACK_SIGNATURE_INVALID`。
- callback 请求可以只带 `idempotencyKey` / `idempotency_key` 定位 grant order，也可以带 `billNo` / `bill_no` / `commandId`。
- 成功响应增加 `billNo`、`idempotencyKey` 和 `status` 字段，保留原有 `commandId` / `grantStatus` 字段兼容现有调用方。
- 内存仓储和 PostgreSQL 仓储均按 `tenantId` 限定 locator 查询，避免跨 tenant 更新。

## Verification

~~~bash
go test ./internal/httpapi ./internal/marketing ./internal/postgres -run 'TestRewardCallback|TestMemoryCallback|TestPostgresCallback' -count=1
go test ./...
go build ./...
~~~

结果：以上命令均通过。

## Notes

- `test.md` 和 `commit.md` 当前仍是模板态，没有实际失败命令输出；本轮按返工反馈中的 Test/Commit 失败要求，优先确认无具体失败上下文，再完成最小修复和全量验证。
- 本轮未执行 git commit。

需求：2026-05-24-reward-callback
