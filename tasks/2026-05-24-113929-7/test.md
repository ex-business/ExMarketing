# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- PASS: 合法模拟运行返回规则解释、奖励预览、Quota 预览、participation / grant order / outbox 预览，并保持 preview-only。
  - 覆盖测试：`TestSimulateEventReturnsRuleExplainWithoutPersistence`、`TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence`。
- PASS: 活动候选过滤、entry/audience/config 未命中原因、规则失败原因稳定输出。
  - 覆盖测试：`TestSimulateEventMatchFiltersAndReasons`、`TestSimulateEventMatchEntryAudienceAndConfigReasons`、`TestSimulateEventMatchRuleFailureUsesMatchReason`、`TestEntryConfigMismatchReasons`。
- PASS: 规则失败、Quota 超限时不展示可执行 grant/outbox 预览。
  - 覆盖测试：`TestSimulateEventRewardConditionFailureDoesNotPreviewGrantOrOutbox`、`TestSimulateEventQuotaExceededSuppressesGrantPreview`。
- PASS: Fact Query 在模拟运行中按只读链路查询、输出 `factQueryResults`、报告缓存命中，并在失败时隔离到依赖活动。
  - 覆盖测试：`TestSimulateEventQueriesFactsAndReportsCache`、`TestSimulateEventFactQueryFailureIsExplainedAndIsolated`。
- PASS: 互斥组落选沿用 `EXCLUSIVE_GROUP_LOST`，0 金额无可发权益沿用既有奖励跳过语义，非金额权益 0 金额仍可执行。
  - 覆盖测试：`TestSimulateEventExclusiveGroupSelection`、`TestProcessEventRewardConditionAllowsNonAmountRewardWithZeroAmount`。
- PASS: 插件模拟只输出 preview，不持久化插件执行副作用。
  - 覆盖测试：`TestSimulateEventIncludesPluginPreviewWithoutPersistence`。
- PASS: HTTP 请求校验覆盖非法标准事件 schema、未知顶层字段、缺少必填字段、非法 `occurredAt` 等错误输入。
  - 覆盖测试：`TestSimulateEventRejectsInvalidStandardEventSchema`、`TestActivityEventRejectsUnknownFields`、`TestActivityEventRejectsNestedUnknownFields`、`TestActivityEventRejectsMissingRequiredField`、`TestActivityEventRejectsInvalidSchemaFields`。
- PASS: 允许写脱敏 `SIMULATION_RUN` 审计，并可通过 audit 查询。
  - 覆盖测试：`TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence`。

## Commands

~~~bash
go test ./...
~~~

结果：PASS。

输出摘要：

~~~text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/decimal	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.680s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
?   	github.com/ex-stack/ExMarketing/internal/mqtarget	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.602s
~~~

~~~bash
go build ./...
~~~

结果：PASS。命令无 stderr/stdout 输出，退出码 0。

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` 退出码 0，覆盖全量 Go 包。
- `go build ./...` 退出码 0，覆盖全量 Go 包编译。
- 未发现测试失败；无需打回 Dev Agent。
- 本轮仅写入 `tasks/2026-05-24-113929-7/test.md`，未修改业务代码、测试代码、文档代码或配置文件。

需求：2026-05-24-113929-7
