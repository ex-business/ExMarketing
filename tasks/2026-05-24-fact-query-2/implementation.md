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

- 补齐 Fact Query 请求模型，允许标准事件上下文 `eventId`、`eventType`、`sourceSystem`、`occurredAt`，并要求直接查询必须包含 `tenantId`、`factCode`、`subject`、`biz`、`attributes`。
- 将 provider 输入改为规范化标准上下文，`traceId` 只用于追踪，不传 provider、不参与缓存 key。
- 重做 Fact Query 成功缓存 key：`provider 配置版本 + 规范化 provider 输入上下文`。配置版本优先使用 `updatedAt`，缺失时使用 provider 配置指纹。
- 补齐 mapping 失败 reason：`MAPPING_REQUIRED_MISSING`、`MAPPING_TYPE_MISMATCH`、`MAPPING_KEY_INVALID`、`MAPPING_SOURCE_INVALID`、`MAPPING_CONFIG_INVALID`。
- 补齐 provider 安全/失败错误码：`INVALID_FACT_QUERY`、`FACT_PROVIDER_HOST_NOT_ALLOWLISTED`、`FACT_PROVIDER_PRIVATE_IP_BLOCKED`、`FACT_CACHE_UNAVAILABLE`。
- 同步 Fact Query API、数据模型和架构文档，并补齐本需求测试计划。

## Changed Files

- `internal/fact/client.go`
- `internal/fact/client_test.go`
- `internal/marketing/model.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `docs/api.md`
- `docs/data-model.md`
- `docs/architecture.md`
- `tasks/2026-05-24-fact-query-2/test-plan.md`

## Behavior Changes

- `/openapi/v1/facts/query` 拒绝未知顶层字段和缺少必要标准上下文字段的请求，返回 `INVALID_FACT_QUERY`。
- Provider 请求体/GET 查询参数只包含规范化 provider 输入；不会包含 `traceId` 或 inline provider 配置。
- TTL 内相同 provider 输入和配置版本命中缓存；`traceId` 变化仍命中；`eventType`、`sourceSystem`、`occurredAt`、subject、biz、attributes 或 provider 版本变化会绕过旧缓存。
- `cache_ttl_seconds=0` 不缓存；只缓存成功 mapping 后的 facts。
- Mapping 失败仍使用对外稳定 `FACT_MAPPING_FAILED`，同时通过 `reason` 返回更细粒度原因。
- Provider host 不在 allowlist 和 private/local IP 拦截分别返回独立错误码。

## Notes

- 已执行 `go test ./...`：PASS。
- 已执行 `go build ./...`：PASS。
- 工作区已有较多其他任务文件和业务文件处于修改状态；本轮未回滚这些既有变更。

需求：2026-05-24-fact-query-2
