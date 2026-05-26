# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行 QUALIFY 用户资格规则测试。

本轮只验证 Dev Agent 已实现范围：

- QUALIFY 使用通用 `CONDITION` evaluator。
- `attributes.*` VIP、注册天数、渠道、标签数组准入、风险标签排除。
- `IN` / `NOT_IN` 在实际字段为标量和数组时的 membership / 交集语义。
- 缺失字段和缺失 fact 字段的可解释拒绝。
- QUALIFY 拒绝后 `CanReward=false`、`CanReserveQuota=false`。
- 未注册 evaluator 拒绝。
- 相关 engine / httpapi 回归，以及全量 Go 测试和构建。

## Test Cases

| ID | 结果 | 说明 |
|---|---|---|
| Q1 | PASS | `TestQualifyConditionSupportsVIPRegisterChannelAndTags` 覆盖 `attributes.vipLevel >= 3`、`attributes.registerDays >= 7`、`attributes.channel IN ["APP","WEB"]`。 |
| Q2 | PASS | 同一测试覆盖 `attributes.tags` 为数组时，`IN ["new_user","high_value"]` 有交集通过。 |
| Q3 | PASS | 同一测试覆盖 `attributes.riskTags` 为数组时，`NOT_IN ["black","bonus_abuse"]` 无交集通过。 |
| Q4 | PASS | `TestQualifyConditionExplainsMissingAttributeWithoutRewardOrQuota` 覆盖缺少 `attributes.registerDays` 时 `FIELD_MISSING`、`field=attributes.registerDays`。 |
| Q5 | PASS | `TestExecuteReportsDisabledAndMissingFact` 覆盖缺少 `facts.USER_PROFILE.vipLevel` 时 `FACT_FIELD_MISSING` 和具体字段。 |
| Q6 | PASS | `internal/engine` / `internal/httpapi` 定向回归覆盖 fact provider 引用校验相关用例。 |
| Q7 | PASS | `TestConditionEvaluatorRejectsNonStandardTopLevelField` 覆盖非法顶层业务字段不会默认通过。 |
| Q8 | PASS | `internal/rule` 定向回归覆盖 Condition 类型和 Decimal / missing 相关既有用例；本轮未发现类型不匹配回归。 |
| Q9 | PASS | `TestExecuteReportsDisabledAndMissingFact` 覆盖 disabled 规则 `SKIPPED` 且不影响继续执行后续 QUALIFY。 |
| Q10 | PASS | `TestQualifyConditionExplainsMissingAttributeWithoutRewardOrQuota` 覆盖 QUALIFY 拒绝后阻断 reward 和 quota。 |
| Q11 | PASS | `internal/engine` / `internal/httpapi` 定向回归覆盖 `Rule|Qualify|Simulate|Fact` 相关链路。 |
| Q12 | PASS | `TestRegistryRejectsUnregisteredEvaluator` 覆盖未注册 `FORMULA` evaluator 返回 `EVALUATOR_NOT_FOUND` error。 |

## Commands

~~~bash
go test ./internal/rule -run 'Qualify|Condition|Fact|Missing'
# ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)

go test ./internal/engine ./internal/httpapi -run 'Rule|Qualify|Simulate|Fact'
# ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/httpapi	(cached)

go test ./...
# ?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
# ?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
# ?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
# ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.544s
# ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
# ?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
# ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
# ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.473s

go build ./...
# exit code 0
~~~

## Result

~~~text
PASS
~~~

## Evidence

- 必跑命令 `go test ./...` 通过。
- 必跑命令 `go build ./...` 通过。
- 定向规则回归 `go test ./internal/rule -run 'Qualify|Condition|Fact|Missing'` 通过。
- 定向链路回归 `go test ./internal/engine ./internal/httpapi -run 'Rule|Qualify|Simulate|Fact'` 通过。
- 未发现失败命令、失败日志或需要打回 Dev Agent 的阻断问题。
- 本轮仅写入 `tasks/2026-05-24-qualify/test.md`，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

需求：2026-05-24-qualify
