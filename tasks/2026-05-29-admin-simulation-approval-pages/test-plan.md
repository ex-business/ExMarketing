# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

- 后端回归：覆盖上线前校验结论 `PASS / WARN / BLOCK / APPROVAL_REQUIRED`、高危审批版本门禁、goLive/rollback 审计和审批记录。
- 管理台 smoke：通过 `/admin/` 页面验证模拟运行、上线前校验、审批动作、goLive、rollback 和审批历史均调用真实 Admin API。
- 无副作用验证：模拟运行前后不得新增 participation、grant_order、outbox 或 quota 扣减；页面必须把这些字段标记为 preview-only。
- 错误路径验证：非法 JSON、BLOCK 校验、高危未二审、自审批、版本不一致、驳回原因缺失和后端非 2xx 响应都必须显示真实失败。

## Test Cases

1. 模拟运行成功：选择 `DEPOSIT_SUCCESS` 样例，执行 `POST /admin/v1/simulate`，确认页面展示 summary、matched/unmatched、MATCH/QUALIFY/REWARD/RISK/LIMIT、reward preview、quota impact、participation/grant/outbox preview 和 raw JSON。
2. 模拟运行错误：输入非法 JSON 或缺少 `tenantId/eventType/subject/biz/occurredAt`，确认页面展示 JSON 解析或 `INVALID_EVENT`，不显示成功态。
3. Quota 超限模拟：构造会 `wouldExceed=true` 的 quota 场景，确认页面展示 `QUOTA_EXCEEDED` 类原因且不展示可执行 grant/outbox 预览。
4. 上线前校验 PASS/WARN：低风险配置返回 PASS；仅预算/库存可选警告返回 WARN 且 `allowGoLive=true`。
5. 上线前校验 BLOCK：非法 schema、预算不足、库存缺失、互斥冲突或插件不兼容返回 BLOCK，页面展示所有阻断项且 goLive 被后端拒绝。
6. 高危审批：自动资金奖励或随机/概率/奖池奖励在未审批时返回 `APPROVAL_REQUIRED`；普通审批一级通过，高危一级后进入 `PENDING_L2`，二级不同 operator 通过后才 `APPROVED`。
7. 审批限制：提交人不能审批自己；高危二级审批人不能等于一级审批人；reject 必须填写 reason；失败后页面保留后端状态。
8. 版本锁定：审批通过后修改配置导致版本变化，goLive 必须失败并提示重新提交审批。
9. goLive：当前版本审批通过后点击 goLive，页面调用 `PUT /admin/v1/activities/{id}` 更新为 `ACTIVE`，审计中可查 `ACTIVITY_GO_LIVE` 和 operator/reason。
10. rollback：已上线或排期活动点击 rollback，页面调用后端安全状态更新为 `PAUSED`，审计中可查 `ACTIVITY_ROLLBACK` 和 operator/reason；不得伪造审批记录。
11. 审批历史：点击审批记录，确认动作、操作者、原因、前后状态、锁定版本、高危标记和时间可查询。
12. 页面刷新：刷新后重新调用活动详情、审批记录和校验接口，不依赖本地成功状态。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用唯一 activityCode 前缀，例如 `SIM_APPROVAL_20260529_`，避免污染既有样例。
- 操作人建议使用 `ops_submitter`、`lead_l1`、`director_l2` 三个不同值。
- 样例事件必须保持标准事件字段：`tenantId`、`eventType`、`sourceSystem`、`subject`、`biz`、`attributes`、`occurredAt`。
- goLive / rollback 均以后端响应和审计记录为准；前端 raw JSON 只作证据展示。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-admin-simulation-approval-pages
