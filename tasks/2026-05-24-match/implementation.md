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

已按 `analysis.md` / `review.md` / `test-plan.md` 完成 MATCH 阶段最小修复：

- 模拟运行的 MATCH 判定改为使用事件 `occurredAt` 判断活动时间窗口，不再使用系统处理时间。
- `SimulateEvent` 对可解释全集输出稳定的 `matchedActivities` / `unmatchedActivities` / summary 计数；同租户和 eventType 查询边界继续由 repository 限定。
- 状态、时间、入口、人群、MATCH 规则失败、入口配置错误和互斥裁剪输出明确 `decisionReasonCode`。
- 互斥组裁剪改为组级算法：空组不互斥，全部 `stackable=true` 全部保留；同组存在任一 `stackable=false` 时只保留排序第一活动，其他同组候选输出 `EXCLUSIVE_GROUP_LOST`。
- 候选排序统一为 `priority DESC, createdAt ASC, activityId ASC`。
- 活动级 `decision` 沿用 `PASSED / REJECTED / SKIPPED / ERROR`，未新增 `MATCHED / UNMATCHED`。

## Changed Files

- `internal/engine/service.go`
  - 新增 MATCH 时间窗口判定、稳定排序、互斥组裁剪和 MATCH 规则失败原因归一化。
  - 入口配置 JSON 错误返回 `ERROR/INVALID_MATCH_CONFIG`。
  - 人群拒绝原因码改为 `AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED`、`AUDIENCE_SEGMENT_NOT_ALLOWED`、`AUDIENCE_RISK_LEVEL_NOT_ALLOWED`。
  - `activityState.active` 改为基于事件 `occurredAt`。
- `internal/engine/service_test.go`
  - 新增 MATCH 查询边界、状态/时间原因、入口/人群原因、MATCH 规则失败和互斥裁剪测试。
- `internal/httpapi/handler_test.go`
  - 更新人群拒绝原因码断言。
- `internal/mqconsumer/consumer_test.go`
  - 修正测试活动窗口，使测试事件 `occurredAt=2026-05-24T12:00:00Z` 落在活动有效期内，同时满足创建阶段 endTime 校验。
- `docs/api.md`
  - 更新入口人群原因码说明。

## Behavior Changes

- `occurredAt < startTime` 返回 `REJECTED/ACTIVITY_NOT_STARTED`。
- `occurredAt >= endTime` 返回 `REJECTED/ACTIVITY_ENDED`。
- 非 `ACTIVE` 活动返回 `REJECTED/ACTIVITY_NOT_ACTIVE`。
- 无效活动时间窗口或不可解释入口配置返回 `ERROR/INVALID_MATCH_CONFIG`。
- MATCH 阶段启用规则失败时，活动级原因返回 `MATCH_RULE_REJECTED`，规则解释仍保留原始失败详情。
- `display.visible=false` 不影响内部 MATCH。
- 跨租户活动和不触发当前 `eventType` 的活动不进入 matched/unmatched/summary。

## Notes

验证命令：

~~~bash
go test ./internal/engine ./internal/httpapi
go test ./...
go build ./...
~~~

结果：全部通过。

未处理事项：未执行 PostgreSQL docker smoke 和 race 测试；本轮按用户要求完成 `go test ./...` 与 `go build ./...`。

需求：2026-05-24-match
