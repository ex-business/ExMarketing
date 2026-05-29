# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

本需求是配置化业务活动模板沉淀，核心验证目标是：9 个模板都能用当前活动 schema、规则 schema、奖励 schema 和 quota schema 表达；不新增 ActivityPlugin；T9 升级红利的 `FORMULA` 必须使用算术 `formula` + `compare`，不得把比较符写入公式字符串。

测试以自动化 schema 回归为主，文档审查为辅：

- Go 单测验证 9 个模板的 `CreateActivityCommand.Validate` 均通过。
- Go 单测验证各模板配套 quota 的 `CreateQuotaCommand.Validate` 均通过。
- Go 单测显式断言 T9 `FORMULA.formula = "attributes.newLevel - attributes.oldLevel"` 且 `compare = {"op": ">", "value": "0"}`。
- 文档审查确认 `docs/business-activity-templates.md` 覆盖标准事件、attributes/facts、规则、奖励、quota、outbox/模拟边界和高风险资金边界。

## Test Cases

| Case | Scope | Expected |
|---|---|---|
| T1 | 签到有礼模板：`SIGN_IN_SUCCESS`、`attributes.continueDays`、`LADDER` 奖励、用户日限和预算。 | 活动配置和 quota schema 通过；不需要插件。 |
| T2 | 每月充值模板：单月窗口、`DEPOSIT_SUCCESS`、单用户月 quota、预算。 | 活动配置和 quota schema 通过；不会暗示单活动自动跨月重置。 |
| T3 | 续存模板：`depositCount >= 2`、充值金额门槛、固定奖励。 | 活动配置和 quota schema 通过。 |
| T4 | 新手任务模板：任务完成事件、固定券类奖励、任务维度用户限制和库存。 | 活动配置和 quota schema 通过。 |
| T5 | 返水模板：有效投注金额、比例奖励、周期用户限制和预算。 | 活动配置和 quota schema 通过。 |
| T6 | 邀请好友模板：被邀请人首存事件、邀请人固定奖励、邀请关系去重 quota 和预算。 | 活动配置和 quota schema 通过。 |
| T7 | VIP 周/月红利模板：VIP facts、周期流水、阶梯奖励。 | 活动配置和 quota schema 通过；`facts.VIP.level` 字段路径合法。 |
| T8 | 生日红利模板：资料 facts、活跃属性、年度用户限制。 | 活动配置和 quota schema 通过；`facts.PROFILE.birthMonthDay` 字段路径合法。 |
| T9 | 升级红利模板：安全 `FORMULA` 判断新等级大于旧等级、阶梯奖励、等级维度用户限制。 | 活动配置和 quota schema 通过；公式字符串不含比较符，比较在 `compare`。 |
| Regression | 全仓库测试和构建。 | `go test ./...` 和 `go build ./...` 均 PASS。 |

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

建议 Test Agent 重点查看：

- `internal/marketing/business_activity_templates_test.go`
- `docs/business-activity-templates.md`
- `docs/api.md`

人工 smoke 可选：启动服务后选择 T9 payload 创建活动，再使用文档中的 `VIP_LEVEL_UPGRADED` pass/reject 事件调用 `POST /admin/v1/simulate`，确认 pass 事件出现 reward/quota/grant/outbox 预览，reject 事件不产生奖励和 quota 扣减预览。

## Decision

~~~text
PASS
~~~

需求：2026-05-27-business-activity-templates
