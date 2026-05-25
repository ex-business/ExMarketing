# Task

## 标题

HTTP Event API 鉴权与接入

## 需求 ID

2026-05-24-http-event-api

## 背景

来源：22-独立活动营销中台设计。完善 POST /openapi/v1/events 的 Bearer Token 或接入方鉴权骨架、租户隔离、请求大小限制和错误响应格式。验收：未授权拒绝、合法事件接收、租户字段不可缺失，测试覆盖成功和失败路径。

## Owner 补充决策

### 凭证来源

本期使用环境变量配置 OpenAPI Event API 接入凭证，不新增完整凭证管理后台。

配置项：

- `EX_MARKETING_OPENAPI_TOKENS`

格式：

```text
token:tenantId[,tenantId...]:sourceSystem[,sourceSystem...];token2:tenantId2:sourceSystem2
```

规则：

- token 不能为空。
- tenantId 不能为空。
- sourceSystem 不能为空。
- 多个 token 可并存。
- token 不支持过期、轮换 API、禁用 API 或数据库持久化；这些是后续生产凭证管理任务。
- 日志、错误响应和审计不得输出完整 token 或完整 Authorization header。

### 鉴权策略

`POST /openapi/v1/events` 本期必须支持 Bearer Token。

请求必须包含：

```http
Authorization: Bearer <token>
```

鉴权规则：

- 缺少 Authorization 返回 `401 UNAUTHORIZED`。
- Authorization 不是 Bearer 格式返回 `401 UNAUTHORIZED`。
- Bearer token 为空返回 `401 UNAUTHORIZED`。
- token 不存在或不可用返回 `401 UNAUTHORIZED`。
- token 存在但不授权请求 body 中的 `tenantId` 返回 `403 TENANT_FORBIDDEN`。
- token 存在但不授权请求 body 中的 `sourceSystem` 返回 `403 SOURCE_FORBIDDEN`。
- 鉴权失败不得写入 event inbox，不得进入活动引擎。

### 生产强制策略

本期按环境变量 `EX_MARKETING_REQUIRE_OPENAPI_AUTH` 控制是否强制鉴权。

规则：

- `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 时，必须配置至少一个有效 token；否则服务启动失败。
- `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 时，所有 `/openapi/v1/events` 请求必须通过 Bearer 鉴权。
- 未设置或为 `false` 时，允许本地开发无鉴权，但必须在日志中提示当前为开发模式。
- 生产部署文档必须要求设置为 `true`。

### 请求大小与 Content-Type

本期请求 body 最大大小为 `1 MiB`，可通过 `EX_MARKETING_EVENT_MAX_BODY_BYTES` 覆盖。

规则：

- 超过限制返回 `413 REQUEST_TOO_LARGE`。
- 缺少 `Content-Type` 或不是 `application/json` 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
- JSON 不可解析返回 `400 INVALID_JSON`。
- 标准事件字段缺失或类型错误返回 `400 INVALID_EVENT`。

### 错误响应

所有失败响应必须是机器可识别 JSON：

```json
{
  "errorCode": "UNAUTHORIZED",
  "message": "unauthorized"
}
```

错误响应不得泄漏 token、Authorization header、内部配置或敏感 payload。

## 目标

- 为 `POST /openapi/v1/events` 增加 Bearer Token 鉴权骨架、租户隔离、sourceSystem 授权、请求大小限制和统一错误响应。
- 保证未授权请求不会进入 event inbox 或活动引擎。
- 保持本地开发可配置为无鉴权，生产可强制鉴权。

## 范围

- Bearer Token 解析和校验。
- token 到 tenantId / sourceSystem 的授权校验。
- 请求 body 最大大小限制。
- Content-Type 校验。
- 401 / 403 / 413 / 415 / 400 错误响应。
- 文档和测试。

## 非目标

- 完整凭证管理后台。
- token 数据库存储。
- token 轮换、禁用、过期。
- OAuth、JWT、签名验签、mTLS。
- OpenAPI 网关。
- 生产 RBAC。

## 风险等级

高

## 验收标准

- 缺少 Authorization 返回 `401 UNAUTHORIZED`，且不写 event inbox。
- 非 Bearer Authorization 返回 `401 UNAUTHORIZED`。
- 空 token 或未知 token 返回 `401 UNAUTHORIZED`。
- token 未授权 tenantId 返回 `403 TENANT_FORBIDDEN`。
- token 未授权 sourceSystem 返回 `403 SOURCE_FORBIDDEN`。
- 合法 token、tenantId、sourceSystem 和标准事件可正常接收。
- 请求超过 `1 MiB` 或配置值返回 `413 REQUEST_TOO_LARGE`。
- 非 `application/json` 返回 `415 UNSUPPORTED_MEDIA_TYPE`。
- 非法 JSON 返回 `400 INVALID_JSON`。
- 标准事件缺字段或类型错误返回 `400 INVALID_EVENT`。
- 错误响应不泄漏 token 或 Authorization header。
- `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true` 且未配置 token 时服务启动失败。
