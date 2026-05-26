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

已按返工反馈做最小修复：

- 补齐 `test-plan.md`，明确 registry 端到端写入/预置后，活动规则校验和 Fact Query 必须读取同一份 Provider 配置。
- 收紧 Fact Provider mapping 契约：支持 `required`，默认 required；拒绝 `facts.` 前缀、首尾点、连续点、非法 source、非法 type。
- source JSONPath 子集支持对象点路径和数字数组下标，如 `$.data.tags[0]`，仍拒绝通配、过滤、脚本和递归搜索。
- 活动保存/预上线校验会拒绝非法 `facts.*` 路径，不再静默忽略 `facts`、`facts.USER_PROFILE`、`facts..vipLevel`、`facts.USER_PROFILE.`。
- disabled provider 与 missing provider 区分返回：`FACT_PROVIDER_DISABLED` / `FACT_PROVIDER_NOT_CONFIGURED`。
- mapping 覆盖按精确字段路径校验，`risk` 不覆盖 `risk.score`。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/fact/client.go`
- `internal/fact/client_test.go`
- `internal/marketing/memory_repository_test.go`
- `internal/httpapi/handler_test.go`
- `docs/api.md`
- `docs/data-model.md`
- `tasks/2026-05-24-fact-provider/test-plan.md`
- `tasks/2026-05-24-fact-provider/implementation.md`

## Behavior Changes

- Provider 配置 schema 非法时更早失败，特别是 mapping key/source/type 不符合 analysis.md 的最小结构时。
- `required=false` 的 mapping source 缺失时，Fact Query 省略该输出字段；required 默认值仍为 true。
- Fact Query 对 disabled provider 返回 `FACT_PROVIDER_DISABLED`。
- 保存活动或预上线校验时，非法 facts 字段路径返回 `FACT_FIELD_INVALID`，mapping 未覆盖字段返回 `FACT_FIELD_MISSING`。

## Validation

~~~bash
go test ./...
go build ./...
~~~

结果：PASS。

需求：2026-05-24-fact-provider
