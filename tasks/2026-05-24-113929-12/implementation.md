# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

按 Review/Test Plan 的最小返工口径修复管理后台通用配置页面：

- 将活动详情改为调用真实 `GET /admin/v1/activities/{id}`，不再只依赖列表快照。
- 移除“审批、上线不在本期”的错误禁用区，改为调用真实审批、上线前校验、状态更新、审批记录和审计 API。
- 创建示例改为 `DRAFT`，避免绕过审批直接创建 `ACTIVE`；示例补齐 `config.entry/display/audience/metadata`、首存规则和充值金额规则。
- 补全 `test-plan.md`，明确 Review Gate、测试策略、正常路径、额度不足、幂等、并发、PostgreSQL smoke 和 PASS 决策。

## Changed Files

- `web/admin/index.html`
- `tasks/2026-05-24-113929-12/test-plan.md`
- `tasks/2026-05-24-113929-12/implementation.md`

## Behavior Changes

- 活动列表点击后会通过后端详情接口刷新活动详情。
- 活动详情展示状态、审批状态、版本、规则、奖励和原始后端响应。
- 活动详情支持真实调用：
  - `POST /admin/v1/activities/prelaunch-validation`
  - `POST /admin/v1/activities/{id}/approval/submit`
  - `POST /admin/v1/activities/{id}/approval/approve`
  - `POST /admin/v1/activities/{id}/approval/reject`
  - `PUT /admin/v1/activities/{id}` 更新 `SCHEDULED` / `ACTIVE` / `PAUSED`
  - `GET /admin/v1/activities/{id}/approval-records`
  - `GET /admin/v1/audit?tenantId=...&activityId=...`
- 所有审批、上线、高危二级审批、版本不一致等结果以后端响应为准，不在前端 mock 成功。

## Notes

验证已执行：

~~~bash
go test ./...
go build ./...
~~~

结果：均通过。

未新增活动专属页面、专属 controller、专属 job 或专属 topic；仍复用现有 Admin API 和通用活动模型。

需求：2026-05-24-113929-12
