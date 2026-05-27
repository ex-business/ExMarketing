# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Scope

本轮按 `test-plan.md` 验证管理后台通用配置页面改动，重点覆盖：

- 通用活动创建、详情、审批、上线前校验、模拟运行、quota、审计均复用真实 Admin API。
- 不通过前端本地状态或 mock 结果伪造审批、上线、模拟或审计成功。
- `FORMULA`、`LIMIT`、`SCRIPT`、`MANUAL`、`MANUAL_EXPORT` 等非目标能力不作为可运行能力交付。
- 未新增活动专属页面、专属 controller、专属 job 或专属 topic。

## Commands

~~~bash
go test ./...
go build ./...
go test -count=1 ./internal/postgres
go test -count=1 ./internal/httpapi
EX_MARKETING_ADDR=:18083 go run ./cmd/exmarketing
node <<'NODE'
// HTTP smoke against http://127.0.0.1:18083:
// - GET /healthz
// - GET /admin/
// - POST /admin/v1/activities
// - GET /admin/v1/activities
// - GET /admin/v1/activities/{id}
// - POST /admin/v1/quotas
// - POST /admin/v1/activities/prelaunch-validation
// - POST /admin/v1/activities/{id}/approval/submit
// - POST /admin/v1/activities/{id}/approval/approve
// - PUT /admin/v1/activities/{id}
// - POST /admin/v1/simulate
// - GET /admin/v1/activities/{id}/approval-records
// - GET /admin/v1/audit?tenantId=tenant_test_admin&activityId={id}&limit=20
NODE
~~~

未执行 npm / 前端脚本：仓库内未发现 `package.json`，当前后台页面为静态 `web/admin/index.html`。

## Results

### Go 回归

- `go test ./...`：PASS
- `go build ./...`：PASS
- `go test -count=1 ./internal/postgres`：PASS
- `go test -count=1 ./internal/httpapi`：PASS

覆盖证据：

- `internal/httpapi` 回归覆盖 `GET/POST/PUT /admin/v1/activities`、审批提交/审批通过/审批驳回、审批记录、上线前校验、模拟运行、审计查询、quota 创建等 Admin API。
- `internal/postgres` 非缓存回归通过，确认 PostgreSQL repository 基础访问链路未被本次改动破坏。

### HTTP Smoke

使用内存仓库启动服务：

~~~bash
EX_MARKETING_ADDR=:18083 go run ./cmd/exmarketing
~~~

执行结果：

- `GET /healthz` -> `200`
- `GET /admin/` -> `200`，页面 HTML 包含 `/admin/v1/activities/prelaunch-validation`
- `POST /admin/v1/activities` -> `201`，创建 `DRAFT` 通用首存/充值类活动成功
- `GET /admin/v1/activities` -> `200`
- `GET /admin/v1/activities/{id}` -> `200`
- `POST /admin/v1/quotas` -> `201`
- `POST /admin/v1/activities/prelaunch-validation` -> `200`，返回真实后端校验结果：`conclusion=BLOCK`，`allowGoLive=false`，包含 `HIGH_RISK_MISSING_BUDGET`、`HIGH_RISK_AUTO_FUND_REWARD`、`APPROVAL_REQUIRED` 等机器原因
- `POST /admin/v1/activities/{id}/approval/submit` -> `200`
- `POST /admin/v1/activities/{id}/approval/approve` 一级审批 -> `200`
- `POST /admin/v1/activities/{id}/approval/approve` 二级审批 -> `200`
- `GET /admin/v1/activities/{id}` -> `200`，`approval.status=APPROVED`，`approval.highRisk=true`
- `PUT /admin/v1/activities/{id}` 设置 `ACTIVE` -> `200`
- `POST /admin/v1/simulate` -> `200`，返回 `summary.matchedCount=1`、`grantOrders=1`、`outboxMessages=1`
- `GET /admin/v1/activities/{id}/approval-records` -> `200`
- `GET /admin/v1/audit?tenantId=tenant_test_admin&activityId={id}&limit=20` -> `200`

说明：第一次 smoke 中曾向 `prelaunch-validation` 的 `activity` 对象额外传入 `id` 字段，后端按 `DisallowUnknownFields` 返回 `400 invalid json body`。该失败由测试载荷不符合 API schema 导致；改为与页面 `validateSelectedActivity()` 相同的不含 `id` 的请求结构后，接口返回 `200` 并给出真实校验结果。

### Static Verification

- `web/admin/index.html` 的活动详情点击调用 `GET /admin/v1/activities/{id}`，不是只使用列表快照。
- 审批、驳回、审批记录、上线状态更新、上线前校验、活动审计均调用真实 Admin API。
- 模拟运行只调用 `POST /admin/v1/simulate`。
- Quota 创建只调用 `POST /admin/v1/quotas`；页面明确说明完整 quota 查询 API 未接入，缺失 quota 行按 `unlimited` 展示。
- 创建示例使用 `status=DRAFT`，包含 `config.entry/display/audience/metadata`、`CONDITION` 规则、`AUTO + HTTP` 奖励通道。
- 后端路由中存在页面引用的相关接口：`GET/POST/PUT /admin/v1/activities`、`approval`、`prelaunch-validation`、`quota`、`simulate`、`audit`。
- 未发现新增活动专属页面、专属 controller、专属 job 或专属 topic。

## Test Cases

- TC-01 正常路径：通过 Go 回归和 HTTP smoke 覆盖创建通用首存/充值类活动、quota、审批、二级审批、激活、模拟运行、审计查看。PASS
- TC-02 非首存事件不命中：由 `internal/httpapi` / `internal/engine` 规则回归覆盖不命中和无发奖预览路径。PASS
- TC-03 非法配置拦截：由 Go 回归覆盖未知字段、非法状态/时间、非法规则字段、不可用 evaluator、非法发奖目标等后端拒绝。PASS
- TC-04 Quota 超限 / unlimited 展示：由 Go 回归和页面静态核验覆盖 quota 风险、`unlimited` 说明、模拟预览不直接扣减。PASS
- TC-05 Fact Provider 缺失 / 失败：由 `internal/httpapi` 和 `internal/marketing` 回归覆盖 `FACT_PROVIDER_NOT_CONFIGURED` 等上线前校验/模拟原因。PASS
- TC-06 幂等重复：由 Go 回归覆盖重复事件、重复审批或版本冲突等路径。PASS
- TC-07 并发：由后端版本/审批约束回归覆盖核心冲突规则；本轮未做浏览器多会话手工并发。PASS with residual manual risk
- TC-08 多租户与唯一约束：由 Go 回归覆盖同租户唯一、跨租户隔离和审计过滤。PASS
- TC-09 PostgreSQL Smoke：`go test -count=1 ./internal/postgres` 通过。PASS
- TC-10 页面边界和空态：静态核验页面存在空态、错误输出和审计失败不阻断主体展示逻辑。PASS

## Result

~~~text
PASS
~~~

## Evidence

必要命令均通过；HTTP smoke 证明页面使用的核心 Admin API 可真实调用并返回后端结果。未发现需要打回 Dev Agent 的失败项。

需求：2026-05-24-113929-12
