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

- Added `PluginContext` as a read-only plugin execution snapshot containing the standard event, activity configuration snapshot, rule results, resolved facts, engine-provided time, and trace id.
- Added `PluginStateStore` contract and `MemoryPluginStateStore` with `Get`, `PutIfAbsent`, `CompareAndSwap`, `AppendEvent`, and `ListByRound`.
- Enforced full plugin state scope isolation by `tenantId`, `activityId`, `activityVersion`, `pluginCode`, `subject`, `biz`, and `stateKey`.
- Enforced write idempotency keys, duplicate no-op behavior, idempotency conflict detection, CAS version conflicts, append-event deduplication, and before/after audit summaries.
- Updated ActivityPlugin lifecycle documentation to state the plugin context/state boundary and explicitly preserve `participation`, `grant_order`, `quota`, and `outbox` as main-engine facts.

## Changed Files

- `internal/plugin/context_state.go`
- `internal/plugin/context_state_test.go`
- `docs/activityplugin-lifecycle.md`
- `tasks/2026-05-29-activityplugin-context-state/implementation.md`

## Behavior Changes

- Plugins now have a typed context snapshot helper via `NewPluginContext`.
- Plugin process state can be stored through an isolated memory implementation that rejects incomplete scopes and does not allow cross-tenant, cross-plugin, cross-version, or cross-stateKey reads.
- Repeated writes with the same full scope and idempotency key return `duplicate_noop` without repeating state consumption or creating a second business-state audit.
- Reusing an idempotency key with a different operation or payload returns an idempotency conflict.
- CAS updates require the expected version and return version conflicts without consuming the idempotency key, allowing callers to reload and retry intentionally.
- `AppendEvent` stores immutable plugin state events and deduplicates repeated event writes by idempotency key.

## Notes

- `review.md` was PASS. `test-plan.md`, `test.md`, and `commit.md` were still template-like and did not contain an actionable failure reason; this implementation followed `task.md`, `analysis.md`, and the P2 review clarification.
- Review P2 audit ambiguity was resolved as: duplicate no-op is observable from the returned result, but it does not create another business-state-change audit record.
- Verification passed:
  - `go test ./...`
  - `go build ./...`

需求：2026-05-29-activityplugin-context-state
