# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 后端回归：执行 `go test ./...` 和 `go build ./...`，确保管理页面改动不破坏现有 API、工作流、规则、奖励、quota、DLQ 和对账逻辑。
- 页面静态检查：对 `web/admin/index.html` 内嵌脚本执行 JS syntax check，避免前端语法错误导致本地后台打不开。
- 本地 smoke：用临时端口启动 `cmd/exmarketing`，访问 `/admin/`，并调用页面依赖的真实后端 API。
- 范围边界：审批、上线、编辑、复制、删除、暂停、恢复、结束、版本回滚必须保持禁用或待接入说明，不允许出现前端伪成功。
- API 边界：活动详情只能展示列表或创建响应中的真实字段；完整 quota 查询不存在时必须标注待接入，缺失 quota 展示为 unlimited 语义。

## Test Cases

1. 管理后台页面加载
   - 步骤：启动服务，访问 `GET /admin/`。
   - 期望：返回 HTML；导航包含 Agent 工作台、活动配置、模拟运行、审计运维。

2. 活动列表
   - 步骤：调用 `GET /admin/v1/activities`。
   - 期望：页面可展示空状态或真实活动列表；接口失败时展示 HTTP 状态和后端错误体。

3. 创建活动成功
   - 步骤：使用页面示例或等价 JSON 调用 `POST /admin/v1/activities`。
   - 期望：返回 201 和后端活动对象；页面刷新列表；规则和奖励只读摘要来自真实返回。

4. 创建活动失败
   - 步骤：提交非法 JSON、缺少必填字段、非法 rulePhase、非法 grantChannel 或非法 targetRef。
   - 期望：页面显示解析失败或后端非 2xx 错误，不显示成功。

5. Quota 创建
   - 步骤：调用 `POST /admin/v1/quotas` 创建 `BUDGET`、`STOCK`、`USER_LIMIT`。
   - 期望：成功时展示后端 quota 返回；失败时显示后端错误；页面明确完整 quota 查询待接入。

6. 模拟运行
   - 步骤：调用 `POST /admin/v1/simulate`。
   - 期望：展示命中/未命中、规则解释、奖励预览、quota 影响、participation/grant/outbox 预览；缺失 quota 显示 unlimited；模拟不写真实业务表。

7. 审计查询
   - 步骤：调用 `GET /admin/v1/audit?tenantId=site_001&limit=100`，可附加 actor/action。
   - 期望：展示真实审计记录或空状态；失败时展示错误。

8. 奖励对账
   - 步骤：调用 `GET /admin/v1/reward/reconciliation`，按 activityId、commandId、eventId、subject 或 idempotencyKey 查询。
   - 期望：展示真实链路或后端返回的无结果/错误；不得把空白链路显示为成功。

9. DLQ 运维
   - 步骤：调用 `GET /admin/v1/outbox/dlq`，并对 DLQ 消息执行 replay/close。
   - 期望：operator 和 reason 必填；非 DLQ 或已关闭消息返回真实错误；成功和失败均展示后端结果。

10. Fact Query 和 Callback
    - 步骤：调用 `POST /openapi/v1/facts/query`、`POST /openapi/v1/reward/callback`。
    - 期望：页面显示真实响应；allowlist、参数校验、状态冲突等后端错误不被吞掉。

11. 未接入操作
    - 步骤：查看活动配置页的审批、上线、编辑发布类入口。
    - 期望：按钮禁用并展示待后端接入说明；无可点击成功路径。

## Required Commands

~~~bash
go test ./...
go build ./...
node --check <(awk '/<script>/{flag=1;next}/<\/script>/{flag=0}flag' web/admin/index.html)
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -fsS http://127.0.0.1:18080/admin/
curl -fsS http://127.0.0.1:18080/admin/v1/activities
curl -fsS 'http://127.0.0.1:18080/admin/v1/audit?tenantId=site_001&limit=5'
curl -fsS 'http://127.0.0.1:18080/admin/v1/outbox/dlq?tenantId=site_001&limit=5'
~~~

## Data / Smoke

- 活动 smoke：通用 `DEPOSIT_BONUS` 活动，`CONDITION` 规则，`RATIO` 奖励，`AUTO + HTTP` 发奖通道，`targetRef=https://wallet.example.com/reward`。
- 模拟事件 smoke：`DEPOSIT_SUCCESS` 标准事件，`subject=USER/10001`，`attributes.amount=299.00`。
- Quota smoke：`BUDGET ACTIVITY TOTAL`、`STOCK ACTIVITY <rewardCode>`、`USER_LIMIT USER <subject.id>`。
- DLQ smoke：无数据时返回空列表；有 DLQ 数据时只通过真实 replay/close API 操作。

## Decision

~~~text
PASS
~~~

理由：计划覆盖本期页面可执行能力、后端真实错误展示、quota/API 缺口边界和未接入操作禁用要求；可进入 Dev 实现和 Test Agent 执行。

需求：2026-05-24-113400-11
