# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以 `marketing_fact_provider` / repository registry 为唯一配置来源验证，不接受请求内 inline provider。
- 覆盖 Provider schema：`tenantId + factCode`、HTTP 类型、GET/POST、endpoint、`cacheTtlSeconds=0..3600`、mapping 非空、mapping key/source/type/required 契约。
- 覆盖活动保存/预上线校验：`facts.<factCode>.<path>` 必须存在同租户 enabled provider，且 mapping 精确覆盖被引用字段。
- 覆盖 Fact Query：按 registry 查询、mapping 成功、缓存、disabled/missing provider、mapping 缺字段/类型错误、SSRF allowlist。
- 最低执行 `go test ./...` 和 `go build ./...`。

## Test Cases

1. Provider schema 接受合法配置：`USER_PROFILE`、HTTP POST、合法 endpoint、`cacheTtlSeconds=0/300`、mapping `vipLevel`、`risk.score`、数组 source `$.data.tags[0]`。
2. Provider schema 拒绝非法 mapping：空 mapping、key 为空/首尾点/连续点/包含 `facts.` 前缀、source 通配或过滤、非法 type。
3. 活动保存引用未配置 `facts.USER_PROFILE.vipLevel` 时失败，错误包含 `FACT_PROVIDER_NOT_CONFIGURED`。
4. 活动保存引用 disabled provider 时失败，错误包含 `FACT_PROVIDER_DISABLED`。
5. 活动保存引用 provider 已启用但 mapping 未覆盖的字段时失败，错误包含 `FACT_FIELD_MISSING`。
6. 嵌套字段必须精确覆盖：`facts.USER_PROFILE.risk.score` 只有 mapping key `risk.score` 通过，只有 `risk` 不通过。
7. 非法 facts 路径 `facts`、`facts.USER_PROFILE`、`facts..vipLevel`、`facts.USER_PROFILE.` 保存失败。
8. 不依赖 `facts.*` 的活动不需要 Provider，可以正常保存和执行。
9. `/openapi/v1/facts/query` 携带 `provider`、`endpoint`、`auth`、`mapping` 等 inline 字段时被拒绝。
10. Fact Query 对 missing/disabled provider 返回稳定错误码。
11. Fact Query 成功时执行 mapping，并只输出声明字段；`required=false` 字段缺失时可省略，不导致整次查询失败。
12. Fact Query mapping 类型不匹配或 required 字段缺失时返回 mapping 失败，错误原因包含 `FACT_TYPE_MISMATCH` 或 `FACT_FIELD_MISSING`。
13. Registry 写入后，活动规则校验和 Fact Query 使用同一份配置，不能通过测试内硬编码 provider 绕过。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用 memory repository 单元测试覆盖受控 registry 写入、活动保存校验和 Fact Query registry 查找。
- 使用 HTTP round trip mock 覆盖 provider HTTP 成功响应、mapping、缓存和失败原因。
- PostgreSQL repository 集成测试需继续覆盖 `marketing_fact_provider` 保存、主键 upsert、enabled 查询和 JSON mapping round trip。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-fact-provider
