# Test

## Agent 指令

你是 Test Agent。已根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行测试。

本轮只写入 `tasks/2026-05-27-business-activity-templates/test.md`，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

| Case | Result | Evidence |
|---|---|---|
| T1 签到有礼模板 schema + quota | PASS | `go test ./...` 覆盖 `internal/marketing/business_activity_templates_test.go`，`CreateActivityCommand.Validate` 与 `CreateQuotaCommand.Validate` 通过。 |
| T2 每月充值单月窗口 schema + quota | PASS | 模板测试覆盖 `2026-05` 单月活动窗口、用户月 quota 和预算 quota。 |
| T3 续存模板 schema + quota | PASS | 模板测试覆盖 `depositCount >= 2`、充值金额门槛、固定奖励和 quota。 |
| T4 新手任务模板 schema + quota | PASS | 模板测试覆盖任务事件、固定券类奖励、用户任务限制和库存 quota。 |
| T5 返水模板 schema + quota | PASS | 模板测试覆盖有效投注金额、比例奖励、周期用户限制和预算。 |
| T6 邀请好友模板 schema + quota | PASS | 模板测试覆盖被邀请人首存条件、邀请人固定奖励、邀请关系去重 quota 和预算。 |
| T7 VIP 周/月红利模板 schema + quota | PASS | 模板测试覆盖 `facts.VIP.level` 字段路径、周期流水、阶梯奖励和 quota。 |
| T8 生日红利模板 schema + quota | PASS | 模板测试覆盖 `facts.PROFILE.birthMonthDay` 字段路径、活跃条件、年度用户限制和预算。 |
| T9 升级红利安全 FORMULA | PASS | 模板测试显式断言 `formula = "attributes.newLevel - attributes.oldLevel"`，并断言比较条件在 `compare = {"op":">","value":"0"}`。 |
| 不新增 ActivityPlugin 依赖 | PASS | 模板测试断言每个模板 `PluginCode == ""`；文档说明普通活动配置不需要 `ActivityPlugin`、专用 controller、专用表或模板市场。 |
| 文档覆盖标准事件、attributes/facts、规则、奖励、quota、outbox/模拟和高风险边界 | PASS | 已人工核对 `docs/business-activity-templates.md` 与 `docs/api.md` 链接说明，覆盖模板矩阵、模拟边界、高风险资金边界和 T9 代表 payload。 |
| 全仓库测试和构建 | PASS | `go test ./...` 和 `go build ./...` 均通过。 |

## Commands

~~~bash
go test ./...
go build ./...
~~~

## Command Output Summary

`go test ./...`:

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.742s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.862s
... remaining packages PASS or [no test files]
~~~

`go build ./...`:

~~~text
PASS: command exited with code 0 and no stderr/stdout output.
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `internal/marketing/business_activity_templates_test.go` 覆盖 9 个模板活动创建 schema、quota schema、无 `PluginCode` 依赖，以及 T9 `FORMULA` 的算术公式和 `compare` 分离要求。
- `docs/business-activity-templates.md` 说明模板均为普通活动配置，使用标准事件、`attributes.*`、可选 `facts.*`、规则、奖励、quota 和自动发奖 outbox，并包含模拟运行和高风险资金模板边界。
- `docs/api.md` 已链接 `docs/business-activity-templates.md`，并说明这些模板不需要 `ActivityPlugin` 或业务专用接口。
- 未发现测试失败项，无需打回 Dev Agent。

需求：2026-05-27-business-activity-templates
