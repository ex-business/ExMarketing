# Test Plan

## Agent 指令

你是后续 Test Agent。请在 Dev Agent 完成实现后执行本测试计划。

约束：

- 只验证 `2026-05-24-fact-query` 的实现，不扩大需求范围。
- 测试前先确认 `tasks/2026-05-24-fact-query/review.md` 的 Decision 为 `PASS`。
- 如实际项目命令与本文命令清单不一致，优先使用仓库已有 Makefile / README / CI 中的等价命令，并在测试报告中记录替换原因。
- 本计划中的命令清单供后续执行；Test Plan Agent 未执行任何测试、构建、格式化或 git 命令。

## Test Strategy

本需求风险集中在 provider registry 唯一来源、租户隔离、facts 规则配置校验、HTTP provider 查询、mapping、缓存、失败降级，以及不得破坏事件发奖主链路。

测试分四层：

1. 单元测试：覆盖 provider registry 校验、Fact Query 输入白名单、mapping、cache key、错误码、脱敏。
2. API / 集成测试：用 mock HTTP fact provider 验证 `/openapi/v1/facts/query`、`POST /admin/v1/activities`、模拟运行和正式事件处理。
3. 幂等 / 并发测试：验证重复事件、并发事件、同一上下文重复 facts 读取不会重复发奖、不会错误扣 quota、不会跨请求污染缓存。
4. PostgreSQL smoke：用真实 PostgreSQL 或项目集成测试环境验证 registry、activity、participation、grant_order、outbox、quota 的落库语义。

## Test Data

建议准备以下固定数据：

- Tenant A：`site_001`
- Tenant B：`site_002`
- FactCode：`USER_PROFILE`
- Subject：`{"type":"USER","id":"user_1001"}`
- Biz：`{"bizType":"DEPOSIT","bizId":"deposit_9001"}`
- 成功 provider 响应：

~~~json
{
  "data": {
    "vipLevel": "VIP3",
    "stats": {
      "depositCount30d": 5
    },
    "riskScore": 12
  }
}
~~~

- Mapping：

~~~json
{
  "vipLevel": {"source": "$.data.vipLevel", "type": "string"},
  "depositCount30d": {"source": "$.data.stats.depositCount30d", "type": "number"},
  "riskScore": {"source": "$.data.riskScore", "type": "number"}
}
~~~

- 依赖 attributes 的活动：规则只读取 `attributes.amount` 或现有 `state.*`。
- 依赖 facts 的活动：规则读取 `facts.USER_PROFILE.vipLevel == "VIP3"` 和 `facts.USER_PROFILE.depositCount30d >= 3`。
- 额度不足活动：设置 activity 或 subject quota，使 facts 规则通过后进入 quota 阶段时额度不足。

## Test Cases

### 1. 正常路径

1. Provider registry 可创建或预置 enabled HTTP provider。
   - 输入：`tenantId=site_001`、`factCode=USER_PROFILE`、`enabled=true`、`cacheTtlSeconds=0`、合法 endpoint、合法 mapping。
   - 期望：registry 按 `tenantId + factCode` 唯一定位，provider 可被查询和规则引用。

2. 后台保存 facts 规则成功。
   - 输入：`POST /admin/v1/activities` 创建 `site_001` 活动，规则引用 `facts.USER_PROFILE.vipLevel` 和 `facts.USER_PROFILE.depositCount30d`。
   - 期望：保存或上线通过；规则引用字段均被 mapping 覆盖；操作符类型兼容。

3. `/openapi/v1/facts/query` 成功查询并 mapping。
   - 输入：只传 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId`。
   - Mock provider 返回成功 JSON。
   - 期望：`success=true`、`cached=false`、`facts.USER_PROFILE.vipLevel=VIP3`、`depositCount30d=5`，无原始 provider 响应透传。

4. 正式事件处理 facts 规则通过并发奖。
   - 输入：发送标准事件，attributes 与 facts 均满足活动规则。
   - 期望：生成 `marketing_participation=APPROVED` 或等价成功状态，生成 `marketing_grant_order` 和 `marketing_outbox`，按既有语义扣减 quota。

5. 不依赖 facts 的活动不触发 provider。
   - 输入：未配置任何 provider，只创建只依赖 `attributes.*` / `state.*` 的活动并发送匹配事件。
   - 期望：活动创建、模拟、正式事件处理均成功；mock provider 调用次数为 0。

### 2. Provider 配置和输入白名单

1. 未配置 provider 时拒绝保存 facts 规则。
   - 输入：`site_001` 无 enabled `USER_PROFILE` provider，活动规则引用 `facts.USER_PROFILE.vipLevel`。
   - 期望：后台拒绝保存或上线；错误包含 `USER_PROFILE`；错误码为 `FACT_PROVIDER_NOT_CONFIGURED` 或等价配置错误。

2. disabled provider 不可引用或查询。
   - 输入：`enabled=false` 的 `USER_PROFILE` provider。
   - 期望：后台保存 facts 规则失败；`/facts/query` 返回 `FACT_PROVIDER_DISABLED` 或等价错误。

3. 跨 tenant 同名 factCode 隔离。
   - 输入：只给 `site_002` 配置 `USER_PROFILE` provider，`site_001` 活动引用 `facts.USER_PROFILE.vipLevel`。
   - 期望：`site_001` 保存失败；`site_001` `/facts/query` 不会读取 `site_002` provider。

4. `/facts/query` 拒绝 inline provider。
   - 输入：请求体额外携带 `provider`、`endpoint`、`method`、`headers`、`auth`、`mapping` 任一字段。
   - 期望：请求失败，错误码 `FACT_INLINE_PROVIDER_NOT_ALLOWED` 或等价错误；不发起 HTTP provider 请求。

5. `/facts/query` 拒绝非白名单顶层字段。
   - 输入：请求体携带 `eventType`、`sourceSystem`、`occurredAt` 顶层字段。
   - 期望：请求失败或被明确忽略且不影响安全边界；若需求实现采用严格校验，应返回可识别参数错误。

6. `cacheTtlSeconds` 非法配置。
   - 输入：`cacheTtlSeconds=-1` 和 `cacheTtlSeconds=3601`。
   - 期望：provider 配置拒绝，错误码 `FACT_CACHE_TTL_INVALID` 或等价配置错误；不得静默截断。

### 3. Mapping、缓存和错误降级

1. Mapping 字段缺失。
   - 输入：provider 返回 `{ "data": {} }`，规则依赖 `facts.USER_PROFILE.vipLevel`。
   - 期望：Fact Query 或规则结果返回 `FACT_FIELD_MISSING`；依赖该 fact 的规则不通过。

2. Mapping 类型不匹配。
   - 输入：mapping 声明 `depositCount30d` 为 number，provider 返回字符串 `"five"`。
   - 期望：返回 `FACT_TYPE_MISMATCH`；活动拒绝；不生成 grant_order / outbox，不扣 quota。

3. Provider 非 2xx、超时、非 JSON。
   - 输入：mock provider 分别返回 500、超时、`text/plain`。
   - 期望：分别返回稳定错误码，如 `FACT_PROVIDER_ERROR`、`FACT_PROVIDER_TIMEOUT`、`FACT_RESPONSE_INVALID_JSON`；错误信息脱敏。

4. 安全拦截。
   - 输入：endpoint host 不在 allowlist、解析到 private/local IP、发生 redirect。
   - 期望：请求拒绝；错误码为 `FACT_ENDPOINT_NOT_ALLOWED`、`FACT_ENDPOINT_BLOCKED`、`FACT_REDIRECT_BLOCKED` 或等价错误；不泄漏内部地址细节。

5. `cacheTtlSeconds=0` 不跨事件缓存。
   - 输入：连续两个不同事件上下文查询相同 tenant/factCode/subject/biz。
   - 期望：mock provider 被调用 2 次；响应 `cached=false`。

6. `cacheTtlSeconds>0` 命中缓存。
   - 输入：`cacheTtlSeconds=60`，相同 tenant/factCode/subject/biz/必要 attributes 连续查询。
   - 期望：第一次 `cached=false`，第二次 `cached=true`；mock provider 只调用 1 次。

7. 缓存隔离。
   - 输入：改变 tenant、subject、biz、影响 provider 响应的 attributes 或 provider `updatedAt`。
   - 期望：不得命中旧缓存；不会跨 tenant 或跨用户复用 facts。

8. 同一事件上下文内重复 facts 规则复用结果。
   - 输入：同一活动两条规则都读取 `USER_PROFILE`。
   - 期望：同一事件处理内 mock provider 只调用 1 次；两条规则共享同一 mapped facts 或同一失败原因。

### 4. 额度不足

1. Facts 规则通过但 activity quota 不足。
   - 输入：enabled provider 返回 VIP3，活动 facts 规则通过；activity quota 已用尽。
   - 期望：活动因额度不足拒绝或不发奖；不生成 grant_order / outbox；participation 或规则解释能区分 facts 通过与 quota 不足。

2. Facts 规则通过但 subject quota 不足。
   - 输入：同一 subject 已达到参与次数或发奖次数上限，再次发送满足 facts 的事件。
   - 期望：不重复发奖；quota 不被扣成负数；拒绝原因是额度不足而不是 provider 错误。

3. Provider 失败时不得进入 quota 扣减。
   - 输入：provider 超时或 mapping 失败。
   - 期望：依赖 fact 的规则失败；不消耗任何 activity / subject quota。

### 5. 幂等重复

1. 相同事件幂等键重复上报。
   - 输入：同一 event id / request id / 业务幂等键发送两次，facts 查询第一次成功。
   - 期望：只产生一条有效 participation 或第二次返回已处理结果；grant_order、outbox、quota 扣减均只发生一次。

2. 重复上报时 provider 不导致重复发奖。
   - 输入：第一次 provider 成功并发奖，第二次 mock provider 返回不同 facts 或失败。
   - 期望：幂等结果以第一次处理为准；不得新增 grant_order / outbox；不得二次扣 quota。

3. `/facts/query` 调试接口重复调用不写业务表。
   - 输入：重复调用 `/openapi/v1/facts/query`。
   - 期望：不会写 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota`。

### 6. 并发

1. 并发相同事件。
   - 输入：并发发送 N 个相同幂等键事件。
   - 期望：最终只有一次成功发奖；无重复 grant_order / outbox；quota 扣减一次；无唯一约束异常泄漏给业务调用方。

2. 并发不同事件争抢最后一个 quota。
   - 输入：剩余 quota 为 1，并发发送多个满足 facts 的不同事件。
   - 期望：最多 1 个事件成功发奖；其他事件因额度不足拒绝；quota 不为负。

3. 并发 facts 查询缓存。
   - 输入：`cacheTtlSeconds=60`，并发查询相同 cache key。
   - 期望：允许少量并发穿透但结果一致；不得写入错误缓存；不得跨 tenant 污染；如实现承诺 singleflight，则 provider 调用次数应为 1。

4. 并发跨 tenant 同名 factCode。
   - 输入：`site_001` 和 `site_002` 同时查询 `USER_PROFILE`，provider endpoint 或 mapping 不同。
   - 期望：结果完全隔离；缓存、规则解释、错误码均不串租户。

### 7. PostgreSQL Smoke

1. Provider registry 表结构 smoke。
   - 检查 `marketing_fact_provider` 或等价表/字段包含 `tenant_id`、`fact_code`、`provider_type`、`endpoint`、`method`、`auth_ref`、`mapping_json`、`cache_ttl_seconds`、`enabled`、`timeout_ms`、`created_at`、`updated_at`。
   - 检查 `tenant_id + fact_code` 唯一约束或等价唯一保证。

2. 活动配置落库 smoke。
   - 创建引用 facts 的活动后，确认活动规则持久化成功且没有 inline provider 配置落入 activity 规则 JSON。

3. 正式事件成功落库 smoke。
   - 成功事件后检查 `marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_quota` 符合既有主链路语义。

4. Facts 失败落库 smoke。
   - Provider 失败事件后检查 participation 为 rejected 或等价状态；拒绝原因或 snapshot 包含 facts 错误码；没有 grant_order / outbox；quota 未扣减。

5. 模拟运行 smoke。
   - 执行 simulation 后检查 inbox、participation、grant_order、outbox、quota 均无新增业务写入。

## Required Commands

后续 Test Agent 应根据项目实际脚本执行以下命令清单。若某命令不存在，记录原因并执行仓库等价命令。

~~~bash
go test ./...
go test -race ./...
go test ./... -run 'Fact|Provider|Mapping|Cache|Activity|Event|Quota|Idempot|Concurrent|Postgres|Integration'
go build ./...
~~~

如项目提供 Makefile 或集成测试脚本，补充执行：

~~~bash
make test
make integration-test
make postgres-smoke
~~~

建议的 API smoke 命令类型：

~~~bash
# 启动本地应用和 mock HTTP fact provider 后执行，具体端口以项目配置为准。
curl -sS -X POST http://127.0.0.1:<port>/openapi/v1/facts/query \
  -H 'Content-Type: application/json' \
  -d '{"tenantId":"site_001","factCode":"USER_PROFILE","subject":{"type":"USER","id":"user_1001"},"biz":{"bizType":"DEPOSIT","bizId":"deposit_9001"},"attributes":{"amount":100},"traceId":"test-fact-query-001"}'
~~~

建议的 PostgreSQL smoke 检查类型：

~~~bash
psql "$DATABASE_URL" -c '\d marketing_fact_provider'
psql "$DATABASE_URL" -c "select tenant_id, fact_code, enabled, cache_ttl_seconds, updated_at from marketing_fact_provider where fact_code = 'USER_PROFILE';"
psql "$DATABASE_URL" -c "select status, reject_reason, snapshot_json from marketing_participation order by created_at desc limit 5;"
psql "$DATABASE_URL" -c "select count(*) from marketing_grant_order;"
psql "$DATABASE_URL" -c "select count(*) from marketing_outbox;"
~~~

## Pass / Fail Criteria

PASS 条件：

- 正常路径、provider 配置校验、输入白名单、租户隔离、mapping、缓存、错误降级均有测试覆盖并通过。
- 额度不足、幂等重复、并发、PostgreSQL smoke 均执行并符合预期。
- 失败场景不会生成 grant_order / outbox，不扣 quota，并能输出可识别 facts 错误原因。
- 错误响应、规则解释、模拟结果和日志摘要不泄漏 token、Authorization、完整 headers、内部网络地址或 provider 原始敏感响应。

BLOCK 条件：

- Review Decision 不是 `PASS`。
- Dev 实现缺少 provider registry 唯一来源，仍允许 inline provider 绕过 registry。
- facts 查询失败会让依赖 facts 的规则默认通过。
- 发现跨 tenant provider、cache、规则解释或结果污染。
- 幂等、并发或额度不足场景存在重复发奖、重复扣 quota、quota 负数、重复 outbox。
- PostgreSQL smoke 无法验证关键数据落库语义，且无等价替代证据。

## Decision

~~~text
PASS
~~~

理由：`review.md` 已 PASS，当前测试计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 及本需求的 provider registry、租户隔离、Fact Query 输入白名单、mapping、缓存、失败降级核心风险。可交给后续 Test Agent 执行。

需求：2026-05-24-fact-query
