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

完成本轮最小返工和实现：

- 补齐 `test-plan.md`，明确 Review PASS 后的页面 smoke、后端权威错误、复制、版本/审批、模拟入口和必跑命令，Decision 为 PASS。
- 扩展 `/admin/` 的“活动配置”页面，提供通用活动列表、筛选、分页、详情、创建、编辑、复制、上线前校验、审批记录、活动审计和模拟入口。
- 保持后端 API 不扩范围：继续复用已有 activity config、approval records、prelaunch validation、simulate、audit API。
- 创建时 `config`、`rules`、`rewards` 分开提交；编辑时只提交后端允许的更新字段，不提交 `id`、`tenantId`、`activityCode`、`activityType`、`rules`、`rewards`。

## Changed Files

- `web/admin/index.html`
  - 活动列表增加搜索、状态过滤、类型过滤、时间过滤和基于当前返回 items 的前端分页。
  - 活动表格补充 tenant、priority、exclusiveGroup、stackable、updatedAt 等运营需要字段。
  - 创建区由单一 JSON textarea 改为结构化基础字段 + `config` / `rules` / `rewards` JSON 编辑区。
  - 增加保存前字段级校验：必填、时间顺序、基础 timezone 格式、JSON 语法、config 顶层字段、`entry.tenantIds` 租户边界。
  - 增加编辑保存，只提交 `UpdateActivityCommand` 支持字段。
  - 增加复制当前活动到新草稿，要求重新填写 `activityCode`，默认 `status=DRAFT`。
  - 详情页增加配置快照、审批版本有效性、版本历史最小口径说明、页面会话内最近模拟摘要、模拟跳转。
- `tasks/2026-05-29-admin-activity-config-pages/test-plan.md`
  - 从模板补齐为可执行测试计划并标记 PASS。
- `tasks/2026-05-29-admin-activity-config-pages/implementation.md`
  - 记录本次实现和验证结果。

## Behavior Changes

- 运营可以在“活动配置”页完成普通配置化活动的创建、编辑、查看、复制和进入模拟运行。
- 页面保存前会提示明显的字段错误，保存后仍以后端响应作为权威结果；重复编码、审批未通过上线、非法配置等后端错误会显示在结果区。
- 列表分页当前基于 `/admin/v1/activities` 已返回的 `items`，因为后端列表 API 当前未返回 total。
- 版本历史当前展示当前版本、更新时间、审批状态和审批记录/审计入口；后端未提供完整历史配置快照 API，因此页面明确不展示旧版本配置内容。
- 最近模拟结果当前只保留页面会话内摘要；后端未提供持久化模拟历史 API。

## Notes

- 已执行 `node` 提取页面脚本做语法检查，结果通过。
- 已启动 `EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing` 做本地 smoke，并用 `curl` 验证 `/healthz` 和 `/admin/` 页面包含活动配置、筛选、config 编辑、复制和模拟入口控件。
- 内置 Browser smoke 尝试失败，原因是当前会话返回 `iab` 不可用；已用本地 HTTP smoke 作为替代验证。
- 已执行 `go test ./...`，结果通过。
- 已执行 `go build ./...`，结果通过。
- 工作树存在其他任务目录和 `requirements/backlog.json` 的既有改动，本次未触碰和回退。

需求：2026-05-29-admin-activity-config-pages
