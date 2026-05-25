# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 合法标准事件：完整 8 个顶层字段、业务字段在 `attributes` 内，接口返回成功并进入既有发奖链路。
- 空和嵌套 `attributes`：`attributes: {}`、嵌套 object/array 均可通过。
- 缺必填字段：覆盖 `occurredAt`、`eventType`、`biz`、`attributes`，返回 `400 INVALID_EVENT`。
- 顶层未知字段：顶层业务字段返回 `400 INVALID_EVENT`，错误包含未知字段原因。
- `subject` / `biz` 非法：非 object、缺子字段、未知业务字段返回 `400 INVALID_EVENT`。
- `occurredAt` 非法：不可解析时间、缺少时区返回 `400 INVALID_EVENT`。
- `attributes` 非法：`null`、array 返回 `400 INVALID_EVENT`。
- 幂等键：缺失和 payload 不匹配返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 重复/冲突：完全重复事件返回 `200 OK` 且 `duplicate=true`；同 key 不同 payload 返回 `400 INVALID_IDEMPOTENCY_KEY`。
- 模拟运行：非法标准事件 payload 返回 `400 INVALID_EVENT`。

## Commands

~~~bash
go test ./...
go build ./...
go test ./internal/httpapi -run 'TestActivityEvent(CreatesGrantCommand|RequiresIdempotencyKey|RejectsMismatchedIdempotencyKey|RejectsUnknownFields|RejectsNestedUnknownFields|RejectsMissingRequiredField|DuplicateAndConflict|RejectsInvalidSchemaFields|AcceptsEmptyAndNestedAttributes)|TestSimulateEventRejectsInvalidStandardEventSchema' -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` 通过：
  - `internal/httpapi` 通过，耗时 `0.458s`。
  - `internal/engine`、`internal/marketing`、`internal/postgres`、`internal/reward`、`internal/rule`、`internal/worker`、`internal/workflow` 均通过。
  - 无测试文件包：`cmd/exmarketing`、`cmd/workflowctl`、`internal/decimal`、`internal/fact`、`internal/plugin`。
- `go build ./...` 通过，无错误输出。
- 定向接口级测试通过：
  - `TestActivityEventCreatesGrantCommand`
  - `TestSimulateEventRejectsInvalidStandardEventSchema`
  - `TestActivityEventRequiresIdempotencyKey`
  - `TestActivityEventRejectsMismatchedIdempotencyKey`
  - `TestActivityEventRejectsUnknownFields`
  - `TestActivityEventRejectsNestedUnknownFields`
  - `TestActivityEventRejectsMissingRequiredField`
    - `missing_occurredAt`
    - `missing_eventType`
    - `missing_biz`
    - `missing_attributes`
  - `TestActivityEventDuplicateAndConflict`
  - `TestActivityEventRejectsInvalidSchemaFields`
    - `null_attributes`
    - `array_attributes`
    - `subject_is_string`
    - `missing_subject_id`
    - `biz_unknown_field`
    - `invalid_time`
    - `time_missing_timezone`
  - `TestActivityEventAcceptsEmptyAndNestedAttributes`

## Failure Details

无失败命令、无失败日志、无需打回 Dev Agent。

需求：2026-05-24-schema
