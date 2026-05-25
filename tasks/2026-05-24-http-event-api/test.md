# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Scope

本轮只执行测试与检查，并只写入本文件。未修改业务代码、测试代码、文档代码、配置文件或其他任务文件。

覆盖目标：

- `POST /openapi/v1/events` Bearer Token 鉴权失败路径。
- tenantId / sourceSystem 授权失败路径。
- 合法 token、tenantId、sourceSystem 和标准事件成功接收路径。
- 鉴权失败不写入 event inbox 的回归。
- Content-Type、非法 JSON、请求 body size limit。
- 标准事件字段、未知字段、幂等键、重复事件和 payload conflict 回归。
- `EX_MARKETING_OPENAPI_TOKENS`、`EX_MARKETING_REQUIRE_OPENAPI_AUTH`、`EX_MARKETING_EVENT_MAX_BODY_BYTES` 配置解析。
- 文档中生产强制鉴权、token 格式、body size 配置和错误码说明。

## Commands

~~~bash
go test ./internal/httpapi -run 'TestActivityEvent(OpenAPIAuth|ContentType|CreatesGrantCommand|RequiresIdempotencyKey|RejectsMismatchedIdempotencyKey|RejectsUnknownFields|RejectsNestedUnknownFields|RejectsMissingRequiredField|DuplicateAndConflict|RejectsInvalidSchemaFields|AcceptsEmptyAndNestedAttributes)|TestOpenAPIEventConfigFromEnv' -count=1 -v
go test ./...
go build ./...
go test ./... -count=1
rg -n "EX_MARKETING_REQUIRE_OPENAPI_AUTH|EX_MARKETING_OPENAPI_TOKENS|EX_MARKETING_EVENT_MAX_BODY_BYTES|REQUEST_TOO_LARGE|UNAUTHORIZED|TENANT_FORBIDDEN|SOURCE_FORBIDDEN|UNSUPPORTED_MEDIA_TYPE" README.md docs/api.md docs/operations.md
~~~

## Result

~~~text
PASS
~~~

## Evidence

定向 HTTP/API 测试通过：

~~~text
PASS
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.257s
~~~

关键通过用例：

- `TestActivityEventOpenAPIAuthRejectsUnauthorizedBeforeInboxWrite`
  - `missing`
  - `basic`
  - `empty_bearer`
  - `unknown_bearer`
- `TestActivityEventOpenAPIAuthTenantAndSourceAuthorization`
- `TestActivityEventContentTypeInvalidJSONAndBodyLimit`
- `TestOpenAPIEventConfigFromEnv`
- `TestActivityEventCreatesGrantCommand`
- `TestActivityEventRequiresIdempotencyKey`
- `TestActivityEventRejectsMismatchedIdempotencyKey`
- `TestActivityEventRejectsUnknownFields`
- `TestActivityEventRejectsNestedUnknownFields`
- `TestActivityEventRejectsMissingRequiredField`
- `TestActivityEventDuplicateAndConflict`
- `TestActivityEventRejectsInvalidSchemaFields`
- `TestActivityEventAcceptsEmptyAndNestedAttributes`

全仓测试通过：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
?   	github.com/ex-stack/ExMarketing/internal/fact	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.462s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.384s
~~~

全仓非缓存测试通过：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/engine	0.200s
?   	github.com/ex-stack/ExMarketing/internal/fact	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.603s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.327s
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	0.646s
ok  	github.com/ex-stack/ExMarketing/internal/reward	1.022s
ok  	github.com/ex-stack/ExMarketing/internal/rule	0.836s
ok  	github.com/ex-stack/ExMarketing/internal/worker	1.170s
ok  	github.com/ex-stack/ExMarketing/internal/workflow	1.488s
~~~

构建通过：

~~~text
go build ./...
exit code 0
~~~

文档检查通过，命中内容包括：

- `README.md`：生产设置 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`、`EX_MARKETING_OPENAPI_TOKENS` 示例、token 格式、`EX_MARKETING_EVENT_MAX_BODY_BYTES`。
- `docs/api.md`：生产强制鉴权、token 格式、401/403/413/415 错误码、Content-Type 和 body limit。
- `docs/operations.md`：生产环境变量、强制鉴权无 token 启动失败、开发模式提示、body size 覆盖。

## Failure Details

无失败命令。

## Reproduction

在仓库根目录执行：

~~~bash
go test ./internal/httpapi -run 'TestActivityEvent(OpenAPIAuth|ContentType|CreatesGrantCommand|RequiresIdempotencyKey|RejectsMismatchedIdempotencyKey|RejectsUnknownFields|RejectsNestedUnknownFields|RejectsMissingRequiredField|DuplicateAndConflict|RejectsInvalidSchemaFields|AcceptsEmptyAndNestedAttributes)|TestOpenAPIEventConfigFromEnv' -count=1 -v
go test ./... -count=1
go build ./...
~~~

需求：2026-05-24-http-event-api
