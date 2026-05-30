# Task

## 标题

security-auth-signature-hardening: 鉴权签名与租户隔离审计

## 需求 ID

2026-05-29-security-auth-signature-hardening

## 背景

事件接入、Webhook、Reward Callback 和 Admin 操作都可能影响真实发奖。必须审计鉴权、签名、租户隔离和重放防护，避免伪造事件、越权发奖、跨 tenant 更新和重复请求攻击。

## 目标

- 审计并加固 OpenAPI token、Webhook 签名、Reward Callback 签名和 Admin 操作边界。
- 确保所有写接口都有 tenant guard。
- 对重放攻击有 timestamp、nonce 或 idempotency 防护。
- 未授权、签名失败、越权、跨 tenant 请求有稳定错误码和审计。

## 范围

- OpenAPI：Bearer token、tenantId 绑定、sourceSystem 绑定。
- Webhook：HMAC 签名、timestamp、source allowlist、重放窗口。
- Callback：billNo/commandId 与 tenant 绑定，签名和幂等。
- Admin：当前可先做边界审计和后续鉴权接口预留，不实现完整登录。
- 测试：未授权、签名错误、过期 timestamp、重复 nonce、跨 tenant。

## 交付物

- 安全审计报告。
- 发现的问题修复或补救任务。
- 鉴权/签名测试用例。
- 错误码和审计 payload 文档。

## 非目标

- 不实现完整 Admin 登录和 RBAC。
- 不接入真实 KMS。
- 不改变外部发奖系统协议之外的业务语义。

## 风险等级

高

## 验收标准

- 外部写入口无匿名成功路径。
- 跨 tenant 更新被拒绝。
- 签名失败和重放请求不会写业务状态。
- 安全失败有脱敏审计。
- `go test ./...` 通过。
