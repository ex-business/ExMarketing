# Implementation

## Summary

已按 `task.md` 和 `analysis.md` 的 Owner 决策完成最小实现：

- Fact Provider 改为 registry 契约，按 `tenantId + factCode` 唯一定位，支持 enabled、HTTP provider、mapping、cache TTL、timeout、updatedAt。
- `/openapi/v1/facts/query` 不再接受 inline provider，只接受 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId`，服务端按 registry 查询 enabled provider。
- 活动创建和预上线校验时校验 `facts.<factCode>.<path>` 引用：当前 tenant 必须存在 enabled provider，且 mapping 覆盖引用字段；纯 `attributes.*` 规则不依赖 provider。
- HTTP Fact Query 增加响应 mapping、TTL 缓存、租户隔离 key、SSRF/allowlist 校验和明确失败 `errorCode`。
- 事件处理和模拟运行在活动规则引用 facts 时按需查询 provider，失败时规则拒绝并保留可解释原因。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/repository.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/fact/client.go`
- `internal/fact/client_test.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/worker/outbox_worker_test.go`
- `db/migration/002_audit_fact_outbox_indexes.sql`
- `docs/api.md`
- `docs/data-model.md`

## Behavior Changes

- Fact Query 请求体包含 `provider`、`endpoint`、`mapping` 等 inline 配置时会被严格 JSON 解码拒绝。
- Provider 不存在或未启用时，Fact Query 返回 `success=false` 和 `FACT_PROVIDER_NOT_CONFIGURED`。
- 预上线校验发现缺失 facts provider 时返回 `conclusion=BLOCK`，并在 item reason 中指出缺失的 `factCode`。
- Provider mapping 缺失或类型不匹配时，Fact Query 返回 `FACT_MAPPING_FAILED`，错误文本包含具体 `FACT_FIELD_MISSING` 或 `FACT_TYPE_MISMATCH` 原因。
- `cacheTtlSeconds` 只允许 `0..3600`；大于 `3600` 或负数会被 provider 校验拒绝。
- 缓存 key 包含 tenant、factCode、subject、biz、attributes 和 provider `updatedAt`，避免跨租户或配置变更污染。

## Verification

```bash
go test ./...
go build ./...
```

结果：均通过。

## Notes

`test-plan.md` 当前仍是模板内容，未提供具体失败用例；本轮返工依据 `task.md`、`analysis.md`、`review.md` 的验收口径和用户返工要求完成。

需求：2026-05-24-fact-query
