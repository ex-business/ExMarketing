# Implementation

## Scope

本轮按 `task.md`、`analysis.md`、`review.md` 和 `test-plan.md` 完成本地管理后台页面的最小增强，保持审批、上线、活动编辑发布等未接入能力禁用。

## Changes

- 更新 `web/admin/index.html`：
  - 活动配置页从原始 JSON 面板扩展为通用活动列表、活动只读摘要、创建活动、Quota 创建和待接入操作区域。
  - 活动详情只展示 `GET /admin/v1/activities` 或创建响应中真实返回的字段；未假设存在 `GET /admin/v1/activities/{id}`。
  - Quota 区域只调用当前明确存在的 `POST /admin/v1/quotas`；页面明确说明完整 quota 查询待后端接入，缺失 quota 按 unlimited 语义表达。
  - 模拟运行页保留真实 `POST /admin/v1/simulate` 调用，并增加无副作用和预览字段说明。
  - 新增审计运维页，覆盖 `GET /admin/v1/audit`、`GET /admin/v1/reward/reconciliation`、`GET /admin/v1/outbox/dlq`、`POST /admin/v1/outbox/{id}/replay`、`POST /admin/v1/outbox/{id}/close`、`POST /openapi/v1/facts/query` 和 `POST /openapi/v1/reward/callback`。
  - 所有 POST/查询结果统一展示 HTTP 状态和后端响应体；JSON 解析失败在前端直接提示，不显示成功。
  - 审批、审批通过、上线、暂停、恢复、复制、删除、版本回滚等入口为 disabled，不存在前端伪成功路径。
- 更新 `tasks/2026-05-24-113400-11/test-plan.md`：
  - 将模板占位补齐为可执行测试计划，并将 Decision 写为 `PASS`。

## Boundary Notes

- 本期未新增后端接口。
- 本期未实现审批流、上线发布、活动编辑、复制、删除、暂停、恢复、结束、版本回滚。
- 本期未伪造活动详情或 quota 查询成功；页面只展示真实接口返回或待接入说明。

## Verification

已执行：

~~~bash
go test ./...
go build ./...
node --check <(awk '/<script>/{flag=1;next}/<\/script>/{flag=0}flag' web/admin/index.html)
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -fsS http://127.0.0.1:18080/admin/
curl -fsS http://127.0.0.1:18080/admin/v1/activities
curl -fsS 'http://127.0.0.1:18080/admin/v1/audit?tenantId=site_001&limit=5'
curl -fsS 'http://127.0.0.1:18080/admin/v1/outbox/dlq?tenantId=site_001&limit=5'
curl -sS -o /tmp/exmarketing_create.json -w '%{http_code}\n' http://127.0.0.1:18080/admin/v1/activities ...
curl -sS -o /tmp/exmarketing_sim.json -w '%{http_code}\n' http://127.0.0.1:18080/admin/v1/simulate ...
~~~

结果：

- `go test ./...`：PASS
- `go build ./...`：PASS
- JS syntax check：PASS
- `/admin/`：返回 HTML
- `GET /admin/v1/activities`：返回 JSON
- `GET /admin/v1/audit`：返回 JSON
- `GET /admin/v1/outbox/dlq`：返回 JSON
- `POST /admin/v1/activities` smoke：201
- `POST /admin/v1/simulate` smoke：200，返回 matched activity、rule result、reward preview、quota impact preview，其中缺失 quota 返回 `unlimited=true`

需求：2026-05-24-113400-11
