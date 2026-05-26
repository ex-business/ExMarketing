# Implementation

## Summary

已按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 实现最小 Webhook Receiver 接入。

- 新增 `POST /openapi/v1/webhooks/{receiverCode}`。
- 新增 `EX_MARKETING_WEBHOOK_RECEIVERS_JSON` 启动配置解析。
- 支持 receiver enabled、secret、tenant/source allowlist、可选 IP allowlist、受限 mapping。
- 验签使用 `hex(HMAC-SHA256(receiver.secret, "<timestamp>.<rawBody>"))`。
- timestamp 重放窗口固定 300 秒。
- mapping 只支持 `const` 或 `$.a.b.c` 对象字段 path，不支持脚本、数组通配、过滤器、外部查询或 raw payload 全量透传。
- 映射后的事件复用既有标准 Event Engine / event inbox / 幂等 / quota / participation / grant_order / outbox 链路。
- Webhook 请求可省略 `Idempotency-Key`；如携带则必须等于映射后标准事件派生键。
- 成功、重复和主要失败路径写入 `WEBHOOK_*` 审计，payload 只记录 hash、receiverCode、eventId、idempotencyKey、errorCode/message 等脱敏字段。

## Changed Files

- `cmd/exmarketing/main.go`
  - 启动时加载 `LoadWebhookReceiverConfigFromEnv()`，并通过 `NewHandlerWithConfigs` 注入 HTTP handler。
- `internal/httpapi/handler.go`
  - Handler 增加 webhook 配置。
  - 注册 `/openapi/v1/webhooks/{receiverCode}` 和 `/openapi/v1/webhooks/`。
- `internal/httpapi/webhook.go`
  - 新增 receiver 配置模型、环境变量解析、签名校验、timestamp 校验、IP allowlist、受限 path/const mapping、Webhook HTTP handler、审计记录。
- `internal/engine/service.go`
  - 增加 `RecordAudit`，供 HTTP 边界写 Webhook 专属审计。
- `internal/httpapi/handler_test.go`
  - 增加 Webhook 成功、重复、签名失败、receiver disabled/not found、mapping failure、source forbidden、idempotency mismatch 测试。
- `docs/api.md`
  - 补充 Webhook Receiver API、签名规则、配置示例、mapping 边界和错误码。
- `README.md`
  - 补充 Webhook Receiver 快速配置和请求签名说明。
- `tasks/2026-05-24-webhook-receiver/test-plan.md`
  - 已补齐测试计划并标记 `PASS`。

## Behavior Changes

- 未配置 `EX_MARKETING_WEBHOOK_RECEIVERS_JSON` 时，Webhook receiver map 为空，标准事件 API 行为不变。
- 合法 Webhook 首次接收返回标准事件处理结果，HTTP 状态沿用现有首次事件语义 `202 Accepted`。
- 完全重复 Webhook 返回 `200 OK` 且 `duplicate=true`。
- receiver 不存在、disabled、签名缺失/非法、timestamp 超窗、来源不允许、mapping 失败、事件非法、幂等键不匹配会返回稳定 `errorCode/message`。
- 审计不会记录 receiver secret、完整 signature 或完整 raw payload。

## Verification

```bash
go test ./...
go build ./...
```

结果：

```text
PASS
PASS
```

需求：2026-05-24-webhook-receiver
