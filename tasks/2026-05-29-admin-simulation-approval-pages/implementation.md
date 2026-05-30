# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

本轮按返工反馈做最小修复：

- `test.md` 和 `commit.md` 只有模板占位，未提供真实 FAIL / BLOCK 原因；本轮按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 落地缺口。
- 补齐 `test-plan.md`，Decision 为 PASS。
- 管理台模拟运行页改为结构化展示 summary、命中/未命中、阶段规则、奖励预览、quota 影响、Fact Query、插件解释和 participation / grant / outbox preview。
- 管理台活动详情页的上线前校验和审批记录改为结构化展示；goLive / rollback 明确调用后端活动状态更新接口。
- 后端上线前校验结论补齐 `WARN` 和 `APPROVAL_REQUIRED` 语义：高危未审批返回 `APPROVAL_REQUIRED`，仅警告项返回 `WARN` 且允许上线。
- 后端 `PUT /admin/v1/activities/{id}` 接收 `reason`，状态进入 `ACTIVE` 记录 `ACTIVITY_GO_LIVE`，回退到 `PAUSED` 记录 `ACTIVITY_ROLLBACK`；未审批上线阻断仍记录 `ACTIVITY_GO_LIVE_BLOCKED`。

## Changed Files

- `web/admin/index.html`
- `internal/marketing/prelaunch_validation.go`
- `internal/marketing/prelaunch_validation_test.go`
- `internal/marketing/model.go`
- `internal/engine/service.go`
- `internal/httpapi/handler_test.go`
- `tasks/2026-05-29-admin-simulation-approval-pages/test-plan.md`
- `tasks/2026-05-29-admin-simulation-approval-pages/implementation.md`

## Behavior Changes

- 模拟运行仍只调用 `POST /admin/v1/simulate`，页面明确标注 preview-only，不会把预览展示成已锁定 quota 或已投递 outbox。
- 上线前校验仍调用 `POST /admin/v1/activities/prelaunch-validation`，页面展示结论、审批状态、高危项和逐项校验。
- goLive 使用 `PUT /admin/v1/activities/{id}` 更新 `status=ACTIVE`；rollback 使用同一后端接口更新 `status=PAUSED`。两者都以后端审批/版本门禁为准，不在前端伪造状态。
- 审批动作仍使用现有 `submit`、`approve`、`reject`、`approval-records` API。
- 高危已审批但仍有非阻断警告时，校验结论为 `WARN` 且 `allowGoLive=true`。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

页面 smoke：

~~~bash
curl -fsS -o /tmp/exmarketing-admin.html -w '%{http_code}\n' http://127.0.0.1:8080/admin/
node - <<'NODE'
const fs = require('fs');
const html = fs.readFileSync('web/admin/index.html', 'utf8');
const match = html.match(/<script>([\s\S]*)<\/script>/);
if (!match) throw new Error('script tag not found');
new Function(match[1]);
console.log('admin inline script syntax OK');
NODE
~~~

结果：`/admin/` 返回 200，内联脚本语法检查通过。

## Notes

- Browser 插件本轮返回 `Browser is not available: iab`，因此未完成可视化点击验证；已用本地 HTTP 200 和脚本语法检查替代。
- 未新增独立 goLive / rollback API；本轮按 review 中允许的最小口径复用后端安全状态更新接口，并在审计 action 中区分 `ACTIVITY_GO_LIVE` / `ACTIVITY_ROLLBACK`。
- 高危金额阈值仍以后端现有 `ValidatePrelaunch` 判定为准，前端只展示后端结果，不另行覆盖审批等级。

需求：2026-05-29-admin-simulation-approval-pages
