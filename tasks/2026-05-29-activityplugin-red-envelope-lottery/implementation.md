# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

- Implemented built-in `RED_ENVELOPE` and `LOTTERY` ActivityPlugin samples.
- Added plugin config validation, reward-reference validation against enabled activity rewards, deterministic gameplay context/audit output, candidate result snapshots, and post-qualification state-change intents.
- Added documentation for sample plugin templates and the boundary between plugin gameplay context and the main reward engine.
- `test.md` and `commit.md` only contained template placeholders and no concrete FAIL/BLOCK reason, so there was no prior failure context to minimally repair.

## Changed Files

- `internal/plugin/red_envelope_lottery.go`
  - Adds `RED_ENVELOPE` and `LOTTERY` plugins, metadata, config schemas, runtime context, deterministic audit, candidate results, and state-change intents.
- `internal/plugin/red_envelope_lottery_test.go`
  - Covers normal path, invalid config, unknown reward references, deterministic lottery output, and user-limit rejection.
- `internal/plugin/plugin.go`
  - Adds optional `ActivityConfigValidator` for plugin validation that needs full activity rewards.
- `internal/engine/service.go`
  - Registers both sample plugins by default and copies plugin candidate results into activity plugin snapshots.
- `internal/marketing/model.go`
  - Adds `candidateResults` to `ActivityPluginSnapshot`.
- `docs/activityplugin-red-envelope-lottery.md`
  - Documents RED_ENVELOPE and LOTTERY templates, runtime output, validation, and simulation boundary.
- `docs/activityplugin-lifecycle.md`
  - Links the sample plugins back to the lifecycle boundary documentation.

## Behavior Changes

- `RED_ENVELOPE` activities can now bind `pluginCode=RED_ENVELOPE` and validate rounds, windows, pool size, per-user limit, candidate rewards, overlap, and enabled reward references.
- `LOTTERY` activities can now bind `pluginCode=LOTTERY` and validate draw windows, draw limit, item uniqueness, non-negative weights, positive total weight, and enabled reward references.
- Both plugins build context only under `state.plugin.<pluginCode>.*` through the existing engine path and never directly write participation, grant orders, outbox messages, quota, or external reward calls.
- Successful plugin hits expose candidate reward details in `candidateResults`; rejected plugin prechecks expose reason codes such as `EMPTY_ENVELOPE`, `RED_ENVELOPE_USER_LIMIT_REACHED`, `NO_PRIZE`, and `DRAW_LIMIT_REACHED`.
- `AfterQualified` emits state-change intents (`recordRedEnvelopeClaim`, `recordLotteryDraw`) as auditable intent payloads only after the main engine qualifies the event.

## Notes

- First `go test ./...` run failed only because of an unused import in the new plugin file; removed it and reran successfully.
- Verification completed:

~~~bash
go test ./...
go build ./...
~~~

Both commands passed.

需求：2026-05-29-activityplugin-red-envelope-lottery
