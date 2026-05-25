# 运维手册

## 本地启动

启动 PostgreSQL：

```bash
docker compose up -d postgres
```

确认数据库健康：

```bash
docker compose ps postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
```

运行服务：

```bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
```

开启 Outbox worker：

```bash
export EX_MARKETING_OUTBOX_WORKER=true
```

配置 Fact Query allowlist：

```bash
export EX_MARKETING_FACT_ALLOWLIST='member.example.com,profile.example.com'
```

## 停止服务

停止 Go 服务：

```bash
Ctrl-C
```

停止 PostgreSQL：

```bash
docker compose down
```

删除本地数据卷：

```bash
docker compose down -v
```

## 应用 migration

首次启动容器会自动执行 `db/migration` 下的 SQL。

已有数据卷时，新 migration 需要手动执行：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/002_audit_fact_outbox_indexes.sql
```

查看表：

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c '\dt'
```

## 清空本地测试数据

```bash
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c "
TRUNCATE
  marketing_outbox,
  marketing_grant_order,
  marketing_participation,
  marketing_event_inbox,
  marketing_reward,
  marketing_rule,
  marketing_quota,
  marketing_audit,
  marketing_activity;
"
```

## 常用查询

活动：

```sql
SELECT id, tenant_id, activity_code, status, start_time, end_time
FROM marketing_activity
ORDER BY created_at DESC;
```

事件 Inbox：

```sql
SELECT id, tenant_id, event_type, source_system, biz_type, biz_id, process_status, error_message
FROM marketing_event_inbox
ORDER BY created_at DESC
LIMIT 20;
```

参与记录：

```sql
SELECT id, activity_id, subject_type, subject_id, biz_type, biz_id, status, reject_reason
FROM marketing_participation
ORDER BY created_at DESC
LIMIT 20;
```

发奖单：

```sql
SELECT id, tenant_id, bill_no, reward_type, reward_amount, grant_status, grant_channel, error_message
FROM marketing_grant_order
ORDER BY created_at DESC
LIMIT 20;
```

Quota：

```sql
SELECT activity_id, quota_type, scope, scope_key, total_value, used_value, frozen_value, version
FROM marketing_quota
ORDER BY updated_at DESC;
```

Outbox：

```sql
SELECT id, tenant_id, message_type, target_channel, target_ref, send_status, retry_count, next_retry_at, error_message
FROM marketing_outbox
ORDER BY created_at DESC
LIMIT 20;
```

DLQ：

```sql
SELECT id, tenant_id, target_channel, target_ref, retry_count, error_message, updated_at, send_status
FROM marketing_outbox
WHERE send_status IN ('DLQ', 'DLQ_CLOSED')
ORDER BY updated_at DESC;
```

DLQ 管理 API：

```bash
curl 'http://localhost:8080/admin/v1/outbox/dlq?tenantId=site_001&includeClosed=true'

curl -X POST http://localhost:8080/admin/v1/outbox/{outboxId}/replay \
  -H 'content-type: application/json' \
  -d '{"tenantId":"site_001","operator":"ops","reason":"target recovered"}'

curl -X POST http://localhost:8080/admin/v1/outbox/{outboxId}/close \
  -H 'content-type: application/json' \
  -d '{"tenantId":"site_001","operator":"ops","reason":"business rejected"}'
```

`replay` 和 `close` 只允许当前状态为 `DLQ` 的记录；`DLQ_CLOSED` 不允许 replay。操作会写入 `marketing_audit`。

审计：

```sql
SELECT actor, action, resource, resource_id, created_at
FROM marketing_audit
ORDER BY created_at DESC
LIMIT 50;
```

## 常见问题

### quota exceeded

含义：

```text
活动预算不足，或用户次数限制已达上限。
```

排查：

```sql
SELECT quota_type, scope, scope_key, total_value, used_value
FROM marketing_quota
WHERE activity_id = '<activity_id>';
```

处理：

```text
确认配置是否正确。
如需提高预算或次数，调整 total_value。
不要直接降低 used_value，除非明确是在清理本地测试数据。
```

### Idempotency-Key does not match event payload

含义：

```text
请求头 Idempotency-Key 与事件 payload 计算出的幂等键不一致。
```

计算规则：

```text
tenantId:sourceSystem:eventType:bizType:bizId
```

### fact endpoint host is not allowlisted

含义：

```text
Fact Query endpoint host 不在 EX_MARKETING_FACT_ALLOWLIST 中。
```

处理：

```bash
export EX_MARKETING_FACT_ALLOWLIST='member.example.com'
```

### fact endpoint resolves to private or local address

含义：

```text
Fact endpoint 解析到了 private/local IP，被 SSRF 防护拦截。
```

处理：

```text
只能使用合法公网 provider，或后续设计内网 provider 的专用安全通道。
```

### outbox target refused

含义：

```text
Outbox worker 投递 HTTP/Webhook 失败。
```

排查：

```sql
SELECT id, target_ref, send_status, retry_count, error_message
FROM marketing_outbox
ORDER BY updated_at DESC;
```

处理：

```text
确认 targetRef 可达。
确认 rewardConfig.targetRef 配置正确。
失败超过重试次数后会进入 DLQ。
```

### duplicate event

含义：

```text
相同 idempotency_key 的事件已经处理过。
```

排查：

```sql
SELECT id, process_status, payload_json
FROM marketing_event_inbox
WHERE idempotency_key = '<key>';
```

## 生产前检查

生产部署前至少确认：

```text
Admin API 已加鉴权或只在内网暴露
OpenAPI Event In 已加鉴权
Reward callback 已加签名或 token 校验
Fact Query allowlist 已配置
Outbox worker 实例数和 claim 机制验证通过
Quota 原子扣减压测通过
DLQ 有告警和人工处理流程
审计表有保留策略
数据库 migration 有正式发布流程
```
