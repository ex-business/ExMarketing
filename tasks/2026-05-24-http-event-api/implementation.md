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

已完成 `POST /openapi/v1/events` 的本期接入边界：

- 新增 OpenAPI Event 配置模型，支持 `EX_MARKETING_OPENAPI_TOKENS`、`EX_MARKETING_REQUIRE_OPENAPI_AUTH`、`EX_MARKETING_EVENT_MAX_BODY_BYTES`。
- `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且没有有效 token 时，服务启动前配置加载失败。
- 未开启强制鉴权时保留本地开发模式，并在服务启动配置加载时输出开发模式日志提示。
- Event API 强制校验 `Content-Type: application/json`，支持 `application/json; charset=utf-8`。
- Event API 使用 `http.MaxBytesReader` 限制请求 body，默认 1 MiB，可通过环境变量覆盖。
- 强制鉴权模式下校验 `Authorization: Bearer <token>`，缺失、非 Bearer、空 token、未知 token 返回 `401 UNAUTHORIZED`。
- 有效 token 按 body 中 `tenantId` 和 `sourceSystem` 做授权隔离，未授权分别返回 `403 TENANT_FORBIDDEN` / `403 SOURCE_FORBIDDEN`。
- Event API 失败响应新增稳定 `errorCode` / `message` 字段，并保留既有 `code` / `error` 字段兼容当前测试与调用方。
- 非法 JSON 返回 `400 INVALID_JSON`；标准事件模型错误继续返回 `400 INVALID_EVENT`；幂等键错误继续返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 鉴权失败、授权失败、Content-Type 失败、body 超限、非法 JSON 和非法事件均在 `ProcessEvent` 前返回，不写入 event inbox，不进入活动引擎。

## Changed Files

- `cmd/exmarketing/main.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `README.md`
- `docs/api.md`
- `docs/operations.md`
- `tasks/2026-05-24-http-event-api/test-plan.md`
- `tasks/2026-05-24-http-event-api/implementation.md`

## Behavior Changes

- 生产可通过环境变量强制 OpenAPI Event API Bearer 鉴权。
- `POST /openapi/v1/events` 现在要求 JSON Content-Type；缺失或非 JSON 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
- 请求 body 超过配置上限返回 `413 REQUEST_TOO_LARGE`。
- 鉴权与授权失败统一返回不泄漏凭证细节的 JSON 错误。
- 本地测试 helper 已统一为 Event API 请求设置 `Content-Type: application/json`，避免误测组合失败优先级。

## Verification

~~~bash
go test ./internal/httpapi -run 'TestActivityEvent(OpenAPIAuth|ContentType|CreatesGrantCommand|RequiresIdempotencyKey|RejectsMismatchedIdempotencyKey|RejectsUnknownFields|RejectsNestedUnknownFields|RejectsMissingRequiredField|DuplicateAndConflict|RejectsInvalidSchemaFields|AcceptsEmptyAndNestedAttributes)|TestOpenAPIEventConfigFromEnv' -count=1 -v
go test ./...
go build ./...
~~~

结果：

~~~text
PASS
~~~

## Notes

`test.md` 和 `commit.md` 当前仍为模板占位，没有实际 `FAIL`、`BLOCK`、`FAILED` 或提交失败上下文；本轮按 task.md、analysis.md、review.md、test-plan.md 做最小实现。

需求：2026-05-24-http-event-api
