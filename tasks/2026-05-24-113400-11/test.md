# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行 Test Agent 验证。

重点覆盖：

- 后端全量测试和构建。
- 管理后台页面脚本语法。
- `/admin/` 页面加载。
- 活动列表、新增活动、非法创建错误展示边界。
- Quota 创建。
- 模拟运行预览。
- 审计、对账、DLQ、Fact Query、Callback 真实响应。
- 未接入审批、上线、编辑发布、复制、删除、暂停、恢复、结束、版本回滚入口禁用和待接入说明。

## Commands

~~~bash
go test ./...
go build ./...
node --check <(awk '/<script>/{flag=1;next}/<\/script>/{flag=0}flag' web/admin/index.html)
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -sS -D /tmp/exm_admin_headers.txt -o /tmp/exm_admin.html -w '%{http_code}\n' http://127.0.0.1:18080/admin/
curl -sS -D /tmp/exm_activities_headers.txt -o /tmp/exm_activities.json -w '%{http_code}\n' http://127.0.0.1:18080/admin/v1/activities
curl -sS -D /tmp/exm_audit_headers.txt -o /tmp/exm_audit.json -w '%{http_code}\n' 'http://127.0.0.1:18080/admin/v1/audit?tenantId=site_001&limit=5'
curl -sS -D /tmp/exm_dlq_headers.txt -o /tmp/exm_dlq.json -w '%{http_code}\n' 'http://127.0.0.1:18080/admin/v1/outbox/dlq?tenantId=site_001&limit=5'
curl -sS -D /tmp/exm_create_headers.txt -o /tmp/exm_create.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/activities ...
curl -sS -D /tmp/exm_invalid_headers.txt -o /tmp/exm_invalid.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/activities -d '{"tenantId":"site_001"}'
curl -sS -D /tmp/exm_quota_budget_headers.txt -o /tmp/exm_quota_budget.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/quotas ...
curl -sS -D /tmp/exm_quota_stock_headers.txt -o /tmp/exm_quota_stock.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/quotas ...
curl -sS -D /tmp/exm_quota_user_headers.txt -o /tmp/exm_quota_user.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/quotas ...
curl -sS -D /tmp/exm_sim_headers.txt -o /tmp/exm_sim.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/admin/v1/simulate ...
curl -sS -D /tmp/exm_recon_headers.txt -o /tmp/exm_recon.json -w '%{http_code}\n' 'http://127.0.0.1:18080/admin/v1/reward/reconciliation?tenantId=site_001&activityId=<createdActivityId>'
curl -sS -D /tmp/exm_audit_after_headers.txt -o /tmp/exm_audit_after.json -w '%{http_code}\n' 'http://127.0.0.1:18080/admin/v1/audit?tenantId=site_001&limit=20'
curl -sS -D /tmp/exm_dlq_after_headers.txt -o /tmp/exm_dlq_after.json -w '%{http_code}\n' 'http://127.0.0.1:18080/admin/v1/outbox/dlq?tenantId=site_001&limit=5'
curl -sS -D /tmp/exm_fact_headers.txt -o /tmp/exm_fact.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/openapi/v1/facts/query ...
curl -sS -D /tmp/exm_callback_headers.txt -o /tmp/exm_callback.json -w '%{http_code}\n' -X POST http://127.0.0.1:18080/openapi/v1/reward/callback ...
rg -n "Agent 工作台|活动配置|模拟运行|审计运维|待后端接入|Quota|unlimited" /tmp/exm_admin.html
rg -n "disabled|待后端接入|审批|上线|暂停|恢复|复制|删除|版本回滚" web/admin/index.html
~~~

## Evidence

- `go test ./...`：PASS。
  - 覆盖包包括 `internal/httpapi`、`internal/marketing`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow`。
- `go build ./...`：PASS。
- `node --check ... web/admin/index.html`：PASS。
- 服务启动：PASS。
  - 日志显示使用 in-memory repository，监听 `:18080`。
- `GET /admin/`：HTTP `200`。
  - HTML 包含 `Agent 工作台`、`活动配置`、`模拟运行`、`审计运维`。
  - HTML 明确说明活动详情接口和完整 quota 查询接口本期未明确存在，缺失 quota 视为 `unlimited`。
- `GET /admin/v1/activities`：HTTP `200`。
- `GET /admin/v1/audit?tenantId=site_001&limit=5`：HTTP `200`。
- `GET /admin/v1/outbox/dlq?tenantId=site_001&limit=5`：HTTP `200`。
- `POST /admin/v1/activities` 合法活动：HTTP `201`。
  - 创建活动 `TEST_AGENT_RATIO_1779716288`。
  - 返回 `id=act_8df1cb8441c0c67a46b36dd8`、`tenantId=site_001`、`status=ACTIVE`、`rules=1`、`rewards=1`。
- `POST /admin/v1/activities` 非法活动：HTTP `400`。
  - 响应：`invalid activity\nactivityCode is required`。
  - 符合失败操作展示真实后端错误的要求。
- `POST /admin/v1/quotas`：
  - `BUDGET / ACTIVITY / TOTAL`：HTTP `201`，`totalValue=10000.00`。
  - `STOCK / ACTIVITY / RATIO_BONUS`：HTTP `201`，`totalValue=100`。
  - `USER_LIMIT / USER / 10001`：HTTP `201`，`totalValue=1`。
- `POST /admin/v1/simulate`：HTTP `200`。
  - `matchedActivities` 数量为 `1`，`unmatchedActivities` 数量为 `0`。
  - 命中活动为 `TEST_AGENT_RATIO_1779716288`。
  - `ruleResult.decision=PASSED`，规则 `AMOUNT_MIN` 通过。
  - `rewardPreviews[0].rewardCode=RATIO_BONUS`，金额 `59.8000`，币种 `CNY`。
  - `quotaImpactPreviews` 覆盖 `BUDGET`、`USER_LIMIT`、`STOCK`，均 `wouldExceed=false`。
  - 返回 `participationPreview`、`grantOrderPreviews`、`outboxCommandPreviews`，均为 `sim_*` 预览对象。
- `GET /admin/v1/reward/reconciliation?tenantId=site_001&activityId=<createdActivityId>`：HTTP `200`。
  - 因模拟运行无真实业务写入，`eventInboxes=0`、`participations=0`、`grantOrders=0`、`outboxMessages=0`、`audits=0`。
  - 符合模拟无副作用边界。
- `GET /admin/v1/audit?tenantId=site_001&limit=20`：HTTP `200`。
  - 返回 `2` 条审计 action：`CREATE_ACTIVITY`、`SIMULATION_RUN`。
- `GET /admin/v1/outbox/dlq?tenantId=site_001&limit=5`：HTTP `200`。
  - `items=0`，空 DLQ 正常展示。
- `POST /openapi/v1/facts/query`：HTTP `400`。
  - 响应：`fact endpoint host is not allowlisted`。
  - 符合 Fact Query allowlist 错误真实透出要求。
- `POST /openapi/v1/reward/callback` 使用不存在 `commandId`：HTTP `404`。
  - 响应：`code=GRANT_ORDER_NOT_FOUND`、`error=not found`。
  - 符合 Callback 失败真实错误展示要求。
- 未接入操作静态验证：PASS。
  - `web/admin/index.html` 中审批、审批通过、上线、暂停、恢复、复制、删除、版本回滚按钮均为 `disabled`。
  - 页面显示“审批、上线、编辑发布、复制、删除、暂停、恢复、结束和版本回滚不在本期实现范围内”。

## Result

~~~text
PASS
~~~

未发现需要打回 Dev Agent 的测试失败。

需求：2026-05-24-113400-11
