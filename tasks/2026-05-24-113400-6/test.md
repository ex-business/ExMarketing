# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本轮严格遵守写入限制：仅修改本文件 `tasks/2026-05-24-113400-6/test.md`，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Environment

```text
工作目录: /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing
测试日期: 2026-05-25
PostgreSQL: docker compose postgres, localhost:54329
EX_MARKETING_DATABASE_URL: postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable
手工 HTTP 验证地址: http://127.0.0.1:18080
```

本地 PostgreSQL 原 volume 是旧 schema，缺少 `marketing_quota_reservation` 表。直接重放完整 migration 失败：

```text
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U exmarketing -d exmarketing -f /docker-entrypoint-initdb.d/001_marketing_core.sql
psql:/docker-entrypoint-initdb.d/001_marketing_core.sql:19: ERROR: relation "marketing_activity" already exists
```

按项目文档的本地测试库重置方式执行：

```bash
docker compose down -v && docker compose up -d postgres
docker compose exec -T postgres pg_isready -U exmarketing -d exmarketing
docker compose exec -T postgres psql -U exmarketing -d exmarketing -c '\dt'
```

重建后确认包含 `marketing_quota_reservation` 表。

## Commands

```bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go build ./...
```

结果：

```text
go test ./... PASS
go build ./... PASS
```

`go test ./...` 关键输出：

```text
ok github.com/ex-stack/ExMarketing/internal/httpapi   0.660s
ok github.com/ex-stack/ExMarketing/internal/marketing  (cached)
ok github.com/ex-stack/ExMarketing/internal/postgres   0.753s
ok github.com/ex-stack/ExMarketing/internal/reward     (cached)
ok github.com/ex-stack/ExMarketing/internal/rule       (cached)
ok github.com/ex-stack/ExMarketing/internal/worker     (cached)
ok github.com/ex-stack/ExMarketing/internal/workflow   1.042s
```

## Manual PostgreSQL HTTP Tests

启动服务：

```bash
EX_MARKETING_ADDR=':18080' \
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
go run ./cmd/exmarketing
```

使用 `ta6_*` / `ta6c_*` 前缀数据通过 `psql` 插入隔离测试记录，覆盖 grant_order、participation、quota、quota reservation。然后使用 `curl` 调用：

```text
POST /openapi/v1/reward/callback
GET  /admin/v1/reward/reconciliation
```

服务验证完成后已停止。

## Test Cases

```text
TC-01 SUCCESS 首次回调: PASS
- ta6_cmd_success 返回 HTTP 200
- grantStatus=SUCCESS
- participationStatus=GRANTED
- reservationStatus=CONFIRMED
- quota used_value=10.0000, frozen_value=0.0000

TC-02 FAILED 首次回调: PASS
- ta6_cmd_failed 返回 HTTP 200
- grantStatus=FAILED
- participationStatus=FAILED
- reservationStatus=RELEASED
- quota used_value=0.0000, frozen_value=0.0000

TC-04/TC-05 多 grant_order participation: PASS
- ta6_cmd_multi_a SUCCESS 后 participationStatus=REWARDABLE，未提前终结
- ta6_cmd_multi_b FAILED 后 participationStatus=FAILED
- 对账中 ta6_cmd_multi_a 保持 SUCCESS/CONFIRMED，能展示部分成功事实

TC-06 quota 聚合不足确认失败: PASS
- ta6_cmd_agg 返回 HTTP 409, code=QUOTA_AGGREGATE_CONFLICT
- grant_order 保持 PENDING
- reservation 保持 FROZEN
- quota used_value=0.0000, frozen_value=3.0000

TC-08 缺少 reservation: PASS
- ta6_cmd_missing 返回 HTTP 409, code=QUOTA_RESERVATION_MISSING
- grant_order 保持 PENDING
- participation 保持 REWARDABLE

TC-10 重复 SUCCESS: PASS
- 第二次 ta6_cmd_success SUCCESS 返回 HTTP 200, duplicate=true
- quota 未重复扣减，reservation 仍 CONFIRMED

TC-11 重复 FAILED: PASS
- 第二次 ta6_cmd_failed FAILED 返回 HTTP 200, duplicate=true
- quota 未重复释放，reservation 仍 RELEASED

TC-12 SUCCESS 后 FAILED 冲突: PASS
- ta6_cmd_success 后续 FAILED 返回 HTTP 409, code=CALLBACK_STATUS_CONFLICT
- grant_order 保持 SUCCESS，quota 不释放

TC-13 FAILED 后 SUCCESS 冲突: PASS
- ta6_cmd_failed 后续 SUCCESS 返回 HTTP 409, code=CALLBACK_STATUS_CONFLICT
- grant_order 保持 FAILED，quota.used_value 不增加

TC-14 externalGrantId 冲突: PASS
- ta6_cmd_success 第二个不同 externalGrantId 返回 HTTP 409, code=CALLBACK_STATUS_CONFLICT
- 原 externalGrantId=wallet_success_1 未被覆盖

TC-15 未知 commandId: PASS
- ta6_cmd_unknown 返回 HTTP 404, code=GRANT_ORDER_NOT_FOUND

TC-16 跨 tenant commandId: PASS
- site_001 + ta6_cmd_cross 返回 HTTP 404, code=GRANT_ORDER_NOT_FOUND
- site_002 的 ta6_go_tenant_b 保持 PENDING，reservation 保持 FROZEN，quota 不变

TC-17 非法请求: PASS
- SUCCESS 缺少 externalGrantId 返回 HTTP 400, code=INVALID_CALLBACK_REQUEST

TC-18 CLOSED grant_order: PASS
- ta6_cmd_closed 返回 HTTP 409, code=GRANT_ORDER_CLOSED
- grant_order 保持 CLOSED，reservation 保持 FROZEN，quota 不变

TC-19 并发重复 SUCCESS: PASS
- 20 个并发 ta6_cmd_concurrent SUCCESS
- summary: success_first=1 duplicate=19 conflict=0 total=20
- grant_order=SUCCESS, reservation=CONFIRMED
- quota used_value=20.0000, frozen_value=0.0000, version=2

TC-20 并发重复 FAILED: PASS
- 20 个并发 ta6c_cmd_failed FAILED
- summary: first=1 duplicate=19 conflict=0 total=20
- grant_order=FAILED, reservation=RELEASED
- quota used_value=0.0000, frozen_value=0.0000, version=2

TC-21 并发 SUCCESS 与 FAILED 竞争: PASS
- 10 个 SUCCESS + 10 个 FAILED 并发 ta6c_cmd_race
- summary: first=1 duplicate=9 conflict=10 total=20
- 本次最终落库为 FAILED/RELEASED
- 未出现 grant_order 与 reservation 相反终态
- quota used_value=0.0000, frozen_value=0.0000, version=2

TC-22 并发不同 grant_order 影响同一 quota: PASS
- ta6c_cmd_pair_success SUCCESS 与 ta6c_cmd_pair_failed FAILED 并发
- success 单 reservation=CONFIRMED
- failed 单 reservation=RELEASED
- 共享 quota used_value=5.0000, frozen_value=0.0000, version=3
```

## Reconciliation Evidence

```text
GET /admin/v1/reward/reconciliation?tenantId=site_001&commandId=ta6_cmd_success
HTTP 200
- commandId=ta6_cmd_success
- externalGrantId=wallet_success_1
- grantStatus=SUCCESS
- participationStatus=GRANTED
- reservationStatus=CONFIRMED
- callbackMessage=paid
- finishedAt=2026-05-24T12:05:00Z

GET /admin/v1/reward/reconciliation?tenantId=site_001&commandId=ta6_cmd_multi_a
HTTP 200
- commandId=ta6_cmd_multi_a
- grantStatus=SUCCESS
- participationStatus=FAILED
- reservationStatus=CONFIRMED
- 保留同 participation 下部分成功事实
```

## Database Evidence

最终关键数据库状态摘要：

```text
ta6_go_success     SUCCESS, external_grant_id=wallet_success_1
ta6_go_failed      FAILED
ta6_go_missing     PENDING
ta6_go_agg         PENDING
ta6_go_closed      CLOSED
ta6_go_tenant_b    PENDING
ta6_go_multi_a     SUCCESS
ta6_go_multi_b     FAILED
ta6_go_concurrent  SUCCESS

ta6_part_success     GRANTED
ta6_part_failed      FAILED
ta6_part_missing     REWARDABLE
ta6_part_agg         REWARDABLE
ta6_part_closed      REWARDABLE
ta6_part_tenant_b    REWARDABLE
ta6_part_multi       FAILED
ta6_part_concurrent  GRANTED

ta6_res_success     CONFIRMED
ta6_res_failed      RELEASED
ta6_res_missing     不存在，按预期触发 QUOTA_RESERVATION_MISSING
ta6_res_agg         FROZEN
ta6_res_closed      FROZEN
ta6_res_tenant_b    FROZEN
ta6_res_multi_a     CONFIRMED
ta6_res_multi_b     RELEASED
ta6_res_concurrent  CONFIRMED

ta6c_res_failed        RELEASED
ta6c_res_race          RELEASED
ta6c_res_pair_success  CONFIRMED
ta6c_res_pair_failed   RELEASED
```

## Result

```text
PASS
```

未发现需要打回 Dev Agent 的测试失败。

需求：2026-05-24-113400-6
