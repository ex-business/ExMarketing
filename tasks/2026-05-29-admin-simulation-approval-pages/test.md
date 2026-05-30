# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已执行：

1. 全量 Go 回归：覆盖所有 package 的单元/HTTP handler/engine/marketing 测试。
2. 全量构建：确认所有 Go packages 可构建。
3. 定向后端回归：
   - 高危活动未审批 goLive 阻断、提交审批、禁止自审批、L1 后仍阻断、L2 后允许上线、审批历史可查。
   - 上线前校验 `BLOCK / APPROVAL_REQUIRED / WARN` 与高危审批版本门禁。
   - 模拟运行规则解释、无持久化、quota preview、quota 超限隐藏 grant/outbox preview、非法标准事件返回 `INVALID_EVENT`。
   - Fact Query 成功/失败解释、插件 preview 无持久化。
4. 管理台 smoke：
   - 本地 in-memory 服务启动后 `/admin/` 返回 200。
   - 管理台内联脚本语法检查通过。
   - 页面包含模拟运行 preview-only 文案、prelaunch-validation、approval submit/approve/reject、approval-records、goLive、rollback 关键调用路径。
5. 真实 Admin API smoke：
   - 使用唯一 activityCode `SIM_APPROVAL_20260529_1780127520643` 创建活动。
   - 未审批 `PUT /admin/v1/activities/{id}` `status=ACTIVE` 返回 `APPROVAL_REQUIRED`。
   - `POST /admin/v1/activities/prelaunch-validation` 返回 `APPROVAL_REQUIRED` 且 `allowGoLive=false`。
   - `submit -> approve L1 -> approve L2` 后审批状态到 `APPROVED`。
   - 审批通过后 goLive 到 `ACTIVE`。
   - `POST /admin/v1/simulate` 返回命中和 `grantOrderPreviews` / `outboxCommandPreviews`。
   - rollback 到 `PAUSED`。
   - `GET /admin/v1/activities/{id}/approval-records` 可查 `APPROVAL_SUBMIT` 和 `APPROVAL_APPROVE`。

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/httpapi -run 'Test(ActivityApprovalFlowBlocksGoLiveUntilRequiredApprovals|PrelaunchValidationBlocksMissingFactProvider|ValidatePrelaunchEndpointReturnsItemizedHighRiskApprovalRequired|SimulateEventReturnsRuleExplainWithoutPersistence|SimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence|SimulateEventQuotaExceededSuppressesGrantPreview|SimulateEventRejectsInvalidStandardEventSchema)$' -count=1 -v
go test ./internal/marketing -run 'TestValidatePrelaunch(RequiresApprovalForAutoFundRewardWithoutApprovalFlow|AllowsApprovedHighRiskVersion|WarnOnlyStillAllowsGoLive|BlocksInvalidProbabilityReward|BlocksExclusiveGroupSamePriorityConflict)$' -count=1 -v
go test ./internal/engine -run 'TestSimulateEvent(MatchFiltersAndReasons|RewardConditionFailureDoesNotPreviewGrantOrOutbox|QueriesFactsAndReportsCache|FactQueryFailureIsExplainedAndIsolated)$|TestSimulateEventIncludesPluginPreviewWithoutPersistence' -count=1 -v
EX_MARKETING_ADDR=127.0.0.1:18080 go run ./cmd/exmarketing
curl -fsS -o /tmp/exmarketing-admin.html -w '%{http_code}\n' http://127.0.0.1:18080/admin/
node - <<'NODE'
const fs = require('fs');
const html = fs.readFileSync('web/admin/index.html', 'utf8');
const match = html.match(/<script>([\s\S]*)<\/script>/);
if (!match) throw new Error('script tag not found');
new Function(match[1]);
const required = [
  'POST /admin/v1/simulate',
  'preview-only',
  '/admin/v1/activities/prelaunch-validation',
  '/approval/submit',
  "postSelectedActivityAction('approval/approve')",
  "postSelectedActivityAction('approval/reject')",
  '/approval-records',
  "status, reason: valueOf('activityReason')",
  'goLiveSelectedActivity',
  'rollbackSelectedActivity'
];
for (const text of required) {
  if (!html.includes(text)) throw new Error('missing admin marker: ' + text);
}
console.log('admin inline script syntax and markers OK');
NODE
curl -sS -o /tmp/exmarketing-sim-invalid.json -w '%{http_code}\n' -H 'Content-Type: application/json' --data '{"tenantId":"site_001"}' http://127.0.0.1:18080/admin/v1/simulate && cat /tmp/exmarketing-sim-invalid.json
curl -fsS http://127.0.0.1:18080/admin/ | rg -n "模拟运行|preview-only|approval|goLive|rollback|prelaunch-validation|POST /admin/v1/simulate"
node - <<'NODE'
// Real Admin API smoke: create activity, blocked goLive, prelaunch validation,
// submit/L1/L2 approval, goLive, simulate, rollback, approval records.
NODE
~~~

## Result

~~~text
PASS
~~~

## Evidence

### 全量回归

`go test ./...` 通过：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.495s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.484s
... all listed packages PASS or [no test files]
~~~

`go build ./...` 通过，无错误输出。

### 定向测试证据

`internal/httpapi` 定向测试通过：

~~~text
PASS: TestActivityApprovalFlowBlocksGoLiveUntilRequiredApprovals
PASS: TestPrelaunchValidationBlocksMissingFactProvider
PASS: TestSimulateEventReturnsRuleExplainWithoutPersistence
PASS: TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence
PASS: TestSimulateEventQuotaExceededSuppressesGrantPreview
PASS: TestSimulateEventRejectsInvalidStandardEventSchema
PASS: TestValidatePrelaunchEndpointReturnsItemizedHighRiskApprovalRequired
ok github.com/ex-stack/ExMarketing/internal/httpapi
~~~

`internal/marketing` 定向测试通过：

~~~text
PASS: TestValidatePrelaunchBlocksInvalidProbabilityReward
PASS: TestValidatePrelaunchRequiresApprovalForAutoFundRewardWithoutApprovalFlow
PASS: TestValidatePrelaunchAllowsApprovedHighRiskVersion
PASS: TestValidatePrelaunchWarnOnlyStillAllowsGoLive
PASS: TestValidatePrelaunchBlocksExclusiveGroupSamePriorityConflict
ok github.com/ex-stack/ExMarketing/internal/marketing
~~~

`internal/engine` 定向测试通过：

~~~text
PASS: TestSimulateEventIncludesPluginPreviewWithoutPersistence
PASS: TestSimulateEventMatchFiltersAndReasons
PASS: TestSimulateEventRewardConditionFailureDoesNotPreviewGrantOrOutbox
PASS: TestSimulateEventQueriesFactsAndReportsCache
PASS: TestSimulateEventFactQueryFailureIsExplainedAndIsolated
ok github.com/ex-stack/ExMarketing/internal/engine
~~~

### 管理台 smoke 证据

本地服务启动：

~~~text
using in-memory repository
exmarketing listening addr=127.0.0.1:18080 configuredAddr=127.0.0.1:18080
~~~

`GET /admin/`：

~~~text
200
~~~

管理台脚本和关键 marker：

~~~text
admin inline script syntax and markers OK
~~~

非法模拟输入：

~~~text
400
{"code":"INVALID_EVENT","error":"eventId is required","errorCode":"INVALID_EVENT","message":"eventId is required"}
~~~

页面 HTML 中确认存在模拟运行、preview-only、approval、goLive、rollback、prelaunch-validation 和 `POST /admin/v1/simulate` 相关内容。

真实 Admin API smoke 输出：

~~~json
{
  "activityId": "act_f5729ed71a0e74664ffe0945",
  "activityCode": "SIM_APPROVAL_20260529_1780127520643",
  "checks": [
    "create",
    "blocked_goLive",
    "prelaunch_APPROVAL_REQUIRED",
    "submit",
    "l1",
    "l2",
    "goLive_ACTIVE",
    "simulate_preview",
    "rollback_PAUSED",
    "approval_records"
  ]
}
~~~

### Notes

一次临时静态 marker 检查使用了错误断言 `'/approval/approve'`，而实际页面实现为 `postSelectedActivityAction('approval/approve')` 拼接路径；该检查已按真实实现修正并通过，不属于产品失败。

需求：2026-05-29-admin-simulation-approval-pages
