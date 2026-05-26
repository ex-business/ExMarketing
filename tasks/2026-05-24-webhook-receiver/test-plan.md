# Test Plan

## Agent 指令

你是 Test Agent。请在 Dev Agent 完成实现后，按本计划补充或执行测试。

本文件只定义后续测试用例和命令清单。Test Plan Agent 本轮未执行测试、构建、格式化或 git 命令。

## 前置条件

- `tasks/2026-05-24-webhook-receiver/review.md` 的 `Decision` 为 `PASS`。
- Webhook Receiver 已实现 `POST /openapi/v1/webhooks/{receiverCode}`。
- 测试环境可配置 `EX_MARKETING_WEBHOOK_RECEIVERS_JSON`，至少包含一个 enabled receiver：
  - `receiverCode=payment-webhook`
  - `secret=receiver-secret`
  - `allowedTenantIds=["site_001"]`
  - `allowedSourceSystems=["payment"]`
  - `allowedIPs=[]`
  - mapping 能把 provider payload 映射为完整标准事件。
- 签名生成规则固定为：

```text
hex(HMAC-SHA256(receiver.secret, "<timestamp>.<rawBody>"))
```

- 默认合法 payload：

```json
{
  "id": "provider_evt_001",
  "user": {
    "id": "user_001"
  },
  "payment": {
    "id": "pay_001",
    "amount": 100,
    "currency": "USD"
  },
  "createdAt": "2026-05-24T12:00:00Z"
}
```

- 默认映射后的标准幂等键：

```text
site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001
```

## Test Strategy

- 单元测试覆盖 receiver 配置解析、receiverCode 定位、Content-Type、body size、timestamp、HMAC raw body 签名、受限 JSONPath 映射、tenant/source allowlist、`Idempotency-Key` 校验和错误码。
- 集成测试覆盖 Webhook HTTP 入口到既有标准事件链路，确认不会绕过 event inbox、quota、participation、grant_order、outbox 和 audit。
- 并发测试覆盖同一幂等键的重复 Webhook 在高并发下只产生一次业务效果。
- PostgreSQL smoke 覆盖核心表落库、重复请求不新增业务记录、quota 不足不产生发奖记录、审计脱敏字段可查。
- 所有多错误用例应尽量构造单一失败原因；多错误优先级不是本期验收目标。

## Test Cases

### 1. 正常路径

1. `POST /openapi/v1/webhooks/payment-webhook`，`Content-Type: application/json`，携带当前 Unix seconds timestamp 和正确小写 hex HMAC。
2. 断言 HTTP 成功，响应至少包含 `duplicate=false`。
3. 断言映射后的事件字段为：
   - `eventId=provider_evt_001`
   - `eventType=DEPOSIT_SUCCESS`
   - `sourceSystem=payment`
   - `tenantId=site_001`
   - `subject.type=USER`
   - `subject.id=user_001`
   - `biz.bizType=DEPOSIT`
   - `biz.bizId=pay_001`
   - `occurredAt=2026-05-24T12:00:00Z`
   - `attributes.amount=100`
   - `attributes.currency=USD`
   - `attributes.providerEventId=provider_evt_001`
4. 断言事件进入既有标准事件链路，并产生预期 participation、grant_order、outbox 或现有标准事件成功摘要。
5. 断言 audit 存在 `WEBHOOK_RECEIVED`，包含 `receiverCode`、`eventId`、`idempotencyKey`、`rawPayloadSha256`、`signatureSha256`，不包含完整 secret、完整 signature 或完整 raw payload。

### 2. 签名与接入安全

1. 缺少 `X-ExMarketing-Timestamp` 或 `X-ExMarketing-Signature`，断言 `400 WEBHOOK_SIGNATURE_MISSING`。
2. timestamp 非 Unix seconds，断言 `400 WEBHOOK_TIMESTAMP_INVALID`。
3. timestamp 超出当前时间前后 300 秒，断言 `400 WEBHOOK_TIMESTAMP_INVALID`。
4. signature 非 hex 或摘要不匹配，断言 `401 WEBHOOK_SIGNATURE_INVALID`。
5. 使用相同 JSON 语义但不同 raw body 空白/字段顺序生成错误签名，断言只有基于真实 raw body 的签名可通过。
6. receiver 不存在，断言 `400 WEBHOOK_RECEIVER_NOT_FOUND`。
7. receiver disabled，断言 `403 WEBHOOK_RECEIVER_DISABLED`。
8. 缺少或非 JSON `Content-Type`，断言 `415 UNSUPPORTED_MEDIA_TYPE`。
9. body 超过 `1 MiB`，断言 `413 REQUEST_TOO_LARGE`。
10. 非法 JSON，断言 `400 INVALID_JSON`。

### 3. 映射与来源限制

1. 映射路径不存在，例如缺少 `payment.id`，断言 `400 WEBHOOK_MAPPING_FAILED`，且不写 event inbox。
2. 必填字段为空字符串或 null，例如 `user.id=""`，断言 `400 WEBHOOK_MAPPING_FAILED`。
3. 标准 string 字段给 number/object 且实现不得隐式转换，断言 `400 WEBHOOK_MAPPING_FAILED` 或 `400 INVALID_EVENT`，按实现边界固定。
4. `occurredAt` 缺失或无时区，断言 `400 WEBHOOK_MAPPING_FAILED` 或 `400 INVALID_EVENT`，且不得用服务接收时间兜底。
5. mapping 结果的 `tenantId` 不在 `allowedTenantIds`，断言 `403 WEBHOOK_SOURCE_FORBIDDEN`。
6. mapping 结果的 `sourceSystem` 不在 `allowedSourceSystems`，断言 `403 WEBHOOK_SOURCE_FORBIDDEN`。
7. `allowedIPs` 非空且请求来源 IP 不匹配，断言 `403 WEBHOOK_SOURCE_FORBIDDEN`。
8. attributes 只包含显式白名单字段，断言原始 payload 中未映射字段不会进入标准事件或审计 raw payload。

### 4. 幂等重复

1. 首次发送合法 payload，断言成功且 `duplicate=false`。
2. 第二次发送完全相同 payload、相同 receiver、相同派生幂等键，断言 `200 OK` 且 `duplicate=true`。
3. 断言第二次不会重复创建 participation、grant_order、outbox，也不会重复扣 quota。
4. 断言 audit 存在 `WEBHOOK_DUPLICATE` 或等价重复审计。
5. 请求携带正确 `Idempotency-Key=site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001`，断言正常成功。
6. 请求携带错误 `Idempotency-Key`，断言 `400 INVALID_IDEMPOTENCY_KEY`，且不进入发奖链路。
7. 相同派生幂等键但 payload 不同，例如同 `payment.id=pay_001`、不同 amount 或 eventId，断言 `400 INVALID_IDEMPOTENCY_KEY`，且不重复发奖。

### 5. 额度不足

1. 准备一个 quota 已耗尽或单次事件会超过活动 quota 的测试活动。
2. 发送合法 Webhook payload，签名、mapping、tenant/source 均通过。
3. 断言标准事件处理阶段返回 `409 QUOTA_EXCEEDED`。
4. 断言不创建成功 grant_order，不发送成功 outbox，不产生错误的 participation 成功态。
5. 断言 audit 存在 `WEBHOOK_QUOTA_EXCEEDED` 或等价 quota 审计信息，并能关联 receiverCode、eventId、idempotencyKey。

### 6. 并发

1. 并发发送 20 到 100 个完全相同的合法 Webhook 请求。
2. 断言最多一个请求返回首次成功语义，其余请求返回 `duplicate=true` 或等价幂等重复语义。
3. 断言数据库中同一幂等键只有一条有效 event inbox 记录。
4. 断言 participation、grant_order、outbox 均只有一次业务效果。
5. 断言 quota 只扣减一次。
6. 并发发送同一幂等键但 payload 不同的请求，断言只有一个 payload 可成为首次成功，其余冲突请求返回 `INVALID_IDEMPOTENCY_KEY` 或等价幂等冲突错误，且业务效果仍只有一次。

### 7. PostgreSQL Smoke

1. 在 PostgreSQL 测试库启动后执行一次正常 Webhook。
2. 查询 event inbox 表，断言存在派生幂等键 `site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001`，payload hash 或等价字段稳定。
3. 查询 participation / grant_order / outbox 相关表，断言正常路径产生预期记录。
4. 重复执行相同 Webhook 后再次查询，断言上述业务表记录数不增加，quota 不重复扣减。
5. 执行 quota 不足用例后查询，断言没有成功 grant_order / outbox，且存在 quota 失败审计。
6. 查询 audit 表，断言成功、重复、签名失败、映射失败、幂等冲突、quota 失败均有可追踪审计记录。
7. 检查 audit payload，不得出现 `receiver-secret`、完整 `X-ExMarketing-Signature` 或完整 raw payload。

## Required Commands

以下命令由后续 Test Agent 在实现完成后执行；本轮 Test Plan Agent 不执行。

```bash
# 1. 启动 PostgreSQL 等依赖，若项目 docker-compose.yml 已包含测试数据库
docker compose up -d postgres

# 2. 设置 Webhook receiver 测试配置
export EX_MARKETING_WEBHOOK_RECEIVERS_JSON='[{"receiverCode":"payment-webhook","enabled":true,"secret":"receiver-secret","allowedTenantIds":["site_001"],"allowedSourceSystems":["payment"],"allowedIPs":[],"mapping":{"eventId":{"path":"$.id"},"eventType":{"const":"DEPOSIT_SUCCESS"},"sourceSystem":{"const":"payment"},"tenantId":{"const":"site_001"},"subject.type":{"const":"USER"},"subject.id":{"path":"$.user.id"},"biz.bizType":{"const":"DEPOSIT"},"biz.bizId":{"path":"$.payment.id"},"occurredAt":{"path":"$.createdAt"},"attributes":{"amount":{"path":"$.payment.amount"},"currency":{"path":"$.payment.currency"},"providerEventId":{"path":"$.id"}}}}]'

# 3. 执行全部 Go 测试
go test ./...

# 4. 执行 Webhook / Event / Inbox / Quota 相关定向测试
go test ./... -run 'Webhook|Event|Inbox|Idempot|Quota|Audit'

# 5. 使用 race 检查并发幂等相关测试
go test -race ./... -run 'Webhook|Idempot|Concurrent|Quota'

# 6. 若项目提供 HTTP 集成测试标签或包，执行 Webhook HTTP 集成测试
go test ./... -tags=integration -run 'Webhook'

# 7. 手工 HTTP smoke：Test Agent 应用实际启动命令启动服务后，用脚本生成 timestamp/signature 并发送 curl 请求
timestamp="$(date +%s)"
body='{"id":"provider_evt_001","user":{"id":"user_001"},"payment":{"id":"pay_001","amount":100,"currency":"USD"},"createdAt":"2026-05-24T12:00:00Z"}'
signature="$(printf '%s.%s' "$timestamp" "$body" | openssl dgst -sha256 -hmac 'receiver-secret' -binary | xxd -p -c 256)"
curl -i \
  -X POST 'http://127.0.0.1:<PORT>/openapi/v1/webhooks/payment-webhook' \
  -H 'Content-Type: application/json' \
  -H "X-ExMarketing-Timestamp: $timestamp" \
  -H "X-ExMarketing-Signature: $signature" \
  --data "$body"

# 8. PostgreSQL smoke 查询，表名按实现后的实际 schema 调整
psql "$DATABASE_URL" -c "select * from event_inbox where idempotency_key = 'site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001';"
psql "$DATABASE_URL" -c "select action, resource, resource_id, payload_json from marketing_audit where resource = 'WEBHOOK_RECEIVER' order by created_at desc limit 20;"
```

## Data / Smoke

- 必须准备至少 3 组 provider payload：
  - 正常 payload：`provider_evt_001 / pay_001 / user_001 / amount=100`。
  - 重复 payload：与正常 payload 字节完全一致。
  - 冲突 payload：同 `payment.id=pay_001`，但 `id` 或 `payment.amount` 不同。
- 必须准备至少 2 组 receiver 配置：
  - enabled receiver：用于正常、重复、并发、quota 和 PostgreSQL smoke。
  - disabled 或 allowlist 不匹配 receiver：用于拒绝路径。
- 必须准备至少 1 个 quota 不足的活动或测试 fixture，使 Webhook 验签和映射成功后在标准事件处理阶段返回 `QUOTA_EXCEEDED`。
- PostgreSQL smoke 只要求验证核心落库结果和审计脱敏，不要求覆盖所有 schema 字段。

## Decision

```text
PASS
```

理由：Review 已 PASS，本计划覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke，并给出后续 Test Agent 可执行的测试命令清单。

需求：2026-05-24-webhook-receiver
