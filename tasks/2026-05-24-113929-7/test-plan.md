# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- 以 `internal/engine` 单元测试覆盖模拟运行解释器核心语义：候选活动、规则解释、Fact Query、互斥、奖励和 Quota 预览。
- 以 `internal/httpapi` 回归测试覆盖 `POST /admin/v1/simulate` 响应结构、无真实业务表副作用、审计查询和错误输入。
- 复用现有正式链路测试，确认模拟运行与 `ProcessEvent` 在活动匹配、规则、奖励、Quota 和 Fact Query 上保持同等解释语义，但不写 inbox / participation / grant_order / outbox / quota。
- Review 要求 reason code 优先沿用现有实现：互斥落选使用 `EXCLUSIVE_GROUP_LOST`，0 金额无可发权益使用 `ZERO_REWARD_SKIPPED`，不得新增同义码。

## Test Cases

1. 合法事件命中活动：返回 `simulatedAt`、`summary`、`matchedActivities`，命中活动含 `ruleResult`、`rewardPreviews`、`quotaImpactPreviews`、`participationPreview`、`grantOrderPreviews`、`outboxCommandPreviews`。
2. 无候选、非 ACTIVE、未开始、已结束、entry disabled、audience 不匹配：进入 unmatched，并输出稳定 `decisionReasonCode`。
3. 规则未通过：规则级 `status=REJECTED`，活动级 `decision=REJECTED`，不展示可执行 grant/outbox 预览。
4. 枚举契约：活动级 `decision` 和规则级 `status` 只允许 `PASSED / REJECTED / SKIPPED / ERROR`，不得输出 `MATCHED / UNMATCHED / FAILED`。
5. Fact Query 成功：规则引用 `facts.USER_PROFILE.vipLevel` 时触发只读 provider 查询，`factQueryResults` 输出 `queried=true`、`status=PASSED`、字段 `available=true`，活动可正常命中。
6. Fact Query 缓存：同一 provider 配置和标准事件上下文再次模拟命中成功缓存，`factQueryResults.cached=true`，且 `traceId` 不影响缓存 key。
7. Fact Query 失败隔离：依赖缺失 provider 的活动输出 `FACT_QUERY_FAILED`，不依赖该 fact 的活动继续解释和命中。
8. Fact mapping 失败：provider 返回字段缺失或类型不匹配时，依赖活动输出 `FACT_MAPPING_FAILED`，不得透传 provider 明文响应。
9. Fact 字段缺失：query 和 mapping 成功但规则读取字段缺失时，规则输出 `FACT_FIELD_MISSING`。
10. Quota 预览：缺失 quota 行显示 `unlimited=true`；quota 不足时 `hasQuotaRisk=true`、活动 `REJECTED/QUOTA_EXCEEDED`，不展示可执行 grant/outbox 预览。
11. 互斥组：同组 non-stackable 只保留正式优先级赢家，落选活动 `REJECTED/EXCLUSIVE_GROUP_LOST`。
12. 0 金额奖励：无可发权益时不生成 grant/outbox，原因沿用 `ZERO_REWARD_SKIPPED`；非金额权益金额为 0 但权益数量合法时仍可生成预览。
13. 无副作用：模拟前后正式事件仍可被 `ProcessEvent` 接受；模拟不消费幂等，不写 inbox / participation / grant_order / outbox / quota，不触发 worker 或外部发奖。
14. 审计：允许写脱敏 `SIMULATION_RUN`，可通过 audit 查询；审计不得记录 secret、token、完整 Authorization、完整敏感 attributes 或完整 provider 响应。
15. 请求校验：非法 JSON、缺少必填字段、非法 `occurredAt`、`attributes` 非 object、未知顶层业务字段返回 400。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

## Data / Smoke

- 使用内存仓储创建活动、quota 和 fact provider，避免真实数据库副作用。
- Fact Provider 使用可控 mock HTTP client，验证请求体只包含标准事件上下文且不包含 `traceId`。
- HTTP smoke 使用现有后台 handler 和 `/admin/v1/simulate`，再提交同一正式事件确认模拟未消费幂等。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113929-7
