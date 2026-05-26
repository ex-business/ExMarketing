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

已按 Review PASS 后的 analysis/test-plan 做最小实现返工：

- 模拟运行活动结果补充 `factQueryResults`，展示活动规则引用的 factCode、引用规则、是否查询、是否缓存、查询状态、原因码和字段可用性。
- `SimulateEvent` 在规则引用 `facts.*` 时复用正式 `QueryFacts` 只读链路；成功结果进入规则上下文，缓存命中会在模拟解释中标记 `cached=true`。
- 模拟运行中 provider 未配置、禁用、HTTP 失败、超时、安全拦截等 provider 查询失败统一归类为 `FACT_QUERY_FAILED`；mapping 失败保留 `FACT_MAPPING_FAILED`。
- 未改正式 `ProcessEvent` 的 Fact Query 错误码契约，避免影响已有 `/openapi/v1/facts/query` 和正式事件链路。
- 补齐 `test-plan.md` 为 PASS，并同步 `docs/api.md` 的模拟运行字段、枚举、Fact Query 和审计口径。

## Changed Files

- `internal/marketing/model.go`
  - 新增 `FactQuerySimulation` 和 `FactQueryFieldResult`。
  - `SimulationActivityResult` 新增 `factQueryResults`。
- `internal/engine/service.go`
  - `simulateActivity` 返回并填充 Fact Query 解释。
  - `queryActivityFacts` 输出稳定排序的查询解释、字段可用性和缓存标记。
  - 模拟运行 provider 查询失败 reason code 归类为 `FACT_QUERY_FAILED`。
- `internal/engine/service_test.go`
  - 新增 Fact Query 成功、缓存命中、失败隔离测试。
- `docs/api.md`
  - 补充 `factQueryResults`、`PASSED / REJECTED / SKIPPED / ERROR` 枚举、只读 Fact Query 和脱敏 `SIMULATION_RUN` 审计说明。
- `tasks/2026-05-24-113929-7/test-plan.md`
  - 补齐测试策略、测试用例、数据 smoke 和 PASS 决议。

## Behavior Changes

- `POST /admin/v1/simulate` 对依赖 `facts.*` 的活动会展示结构化 `factQueryResults`。
- 同一 provider 配置和标准事件上下文下，模拟运行可命中成功 Fact Query 缓存并返回 `cached=true`。
- 依赖 Fact Query 的单个活动查询失败时，该活动进入 unmatched，`decisionReasonCode=FACT_QUERY_FAILED`；同一请求里不依赖该 fact 的活动继续解释。
- 模拟运行仍不写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox` 或 `marketing_quota`；允许保留脱敏 `SIMULATION_RUN` 审计。
- 互斥和 0 金额无可发权益继续沿用现有 `EXCLUSIVE_GROUP_LOST`、`ZERO_REWARD_SKIPPED`，未新增同义 reason code。

## Verification

~~~bash
go test ./...
~~~

结果：PASS。

~~~bash
go build ./...
~~~

结果：PASS。

## Notes

- 本轮没有发现 `test.md` 或 `commit.md` 中存在实际失败证据；两者仍为模板占位内容。
- 当前工作区已有 Analysis/Review/agent 状态文件改动，本轮未回退这些已有改动。

需求：2026-05-24-113929-7
