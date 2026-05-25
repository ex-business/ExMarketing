# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本期测试聚焦 `POST /openapi/v1/events` 的 HTTP 接入边界，不扩大到凭证后台、OAuth、JWT、MQ Event In、Webhook Receiver 或 Fact Query。

测试分三层：

- HTTP handler 单元测试覆盖鉴权、tenant/sourceSystem 授权、Content-Type、body size、非法 JSON、标准事件校验、幂等键和成功接收。
- 配置解析测试覆盖环境变量 token 格式、强制鉴权启动前置条件和 body size 覆盖。
- 全仓验证执行 `go test ./...` 和 `go build ./...`，保证既有活动引擎、幂等、quota、reward callback、workflow 能力不回退。

组合错误优先级不作为本期阻塞验收；测试用例应固定单一失败原因。

## Test Cases

1. 缺少 Authorization，其他输入合法，强制鉴权模式下返回 `401 UNAUTHORIZED`，响应包含 `errorCode/message`，不泄漏 token 或 Authorization header。
2. `Authorization: Basic xxx` 返回 `401 UNAUTHORIZED`。
3. `Authorization: Bearer` 空 token 返回 `401 UNAUTHORIZED`。
4. 未配置 token 返回 `401 UNAUTHORIZED`。
5. 有效 token 但未授权 body `tenantId` 返回 `403 TENANT_FORBIDDEN`。
6. 有效 token 但未授权 body `sourceSystem` 返回 `403 SOURCE_FORBIDDEN`。
7. 合法 token、tenantId、sourceSystem、Content-Type 和标准事件成功接收，返回 `202 Accepted` 或既有重复语义。
8. 鉴权失败请求不得写入 inbox：同一 payload 后续携带合法 token 首次提交不得返回 duplicate。
9. 缺少 `Content-Type` 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
10. 非 `application/json` Content-Type 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
11. `application/json; charset=utf-8` 合法。
12. JSON 不可解析返回 `400 INVALID_JSON`。
13. 标准事件缺字段、类型错误或未知字段返回 `400 INVALID_EVENT`。
14. 缺少或不匹配 `Idempotency-Key` 返回 `400 INVALID_IDEMPOTENCY_KEY`。
15. 请求 body 超过默认 1 MiB 或配置值返回 `413 REQUEST_TOO_LARGE`。
16. `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且无有效 `EX_MARKETING_OPENAPI_TOKENS` 时配置加载失败，服务不得启动。
17. `EX_MARKETING_OPENAPI_TOKENS` 多 tenant/sourceSystem 解析正确，且不同 token 授权不合并。
18. 文档包含生产必须开启 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`、token 格式、body size 配置和错误响应说明。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

标准合法事件样例：

~~~json
{
  "eventId": "evt_20260524_000001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
  "occurredAt": "2026-05-24T12:00:00Z",
  "attributes": {"amount": "299.00", "currency": "CNY", "isFirstDeposit": true}
}
~~~

对应 Header：

~~~text
Content-Type: application/json
Authorization: Bearer token-a
Idempotency-Key: site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123
~~~

本地配置 smoke：

~~~bash
EX_MARKETING_REQUIRE_OPENAPI_AUTH=true
EX_MARKETING_OPENAPI_TOKENS='token-a:site_001:payment'
EX_MARKETING_EVENT_MAX_BODY_BYTES=1048576
~~~

## Decision

~~~text
PASS
~~~

需求：2026-05-24-http-event-api
