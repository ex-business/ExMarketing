# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 和本 `test-plan.md` 均为 PASS 后才能执行测试。

本计划只定义后续测试用例和命令清单。Test Agent 执行时必须先确认 Dev Agent 已完成实现，并按实际代码结构补齐具体测试文件或脚本名称。

## Test Strategy

- 覆盖 `/openapi/v1/facts/query` 的接口校验、provider registry 查询、HTTP provider 适配、mapping、缓存 TTL、安全拦截和错误解释。
- 覆盖活动事件处理链的 `attributes` 优先原则：规则不引用 `facts.*` 时不触发 Fact Query；规则引用 `facts.*` 且查询失败时只影响依赖该 fact 的活动。
- 覆盖缓存 key 隔离：`tenantId`、`factCode`、`subject`、`biz`、`attributes`、`eventType`、`sourceSystem`、`occurredAt`、provider 配置版本均会影响 key；`traceId` 不影响 key。
- 覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
- 单元测试优先使用 mock provider、fake clock、fake cache 和 in-memory HTTP server；集成 smoke 使用 PostgreSQL 测试库验证 `marketing_fact_provider` 配置读写和查询路径。

## Test Cases

### 1. Normal Path

1. `POST /openapi/v1/facts/query` 在 `tenantId + factCode` 存在 enabled HTTP provider 时成功查询 provider，按 `mapping_json` 输出固定结构：

   - 输入包含 `tenantId`、`factCode`、`subject`、`biz`、`attributes`、`traceId`。
   - provider 返回 `{"vip_level":"VIP3","score":88,"extra":"ignored"}`。
   - mapping 输出 `facts.USER_PROFILE.vipLevel = "VIP3"`、`facts.USER_PROFILE.score = 88`。
   - 未声明字段 `extra` 不得透传。
   - 首次响应 `success=true`、`cached=false`、`providerStatus` 可解释。

2. 完整标准事件上下文内部触发：

   - 输入包含 `eventId`、`eventType`、`sourceSystem`、`occurredAt`。
   - provider 实际收到的输入包含这些标准字段。
   - `traceId` 不传给 provider。

3. `attributes` 足够时不依赖 Fact Query：

   - 配置一个只读取 `attributes.amount` / `attributes.vipLevel` 的活动规则。
   - provider 未配置或不可用。
   - 事件仍可按 attributes 正常命中或拒绝，不能产生 Fact Query 调用。

4. 显式引用 `facts.USER_PROFILE.vipLevel` 的活动规则：

   - Fact Query 成功时规则可读取 `facts.USER_PROFILE.vipLevel` 并正常命中。
   - Fact Query 失败时该活动拒绝原因为 `FACT_QUERY_FAILED`、`FACT_MAPPING_FAILED` 或 `FACT_FIELD_MISSING` 之一。
   - 同一事件中不引用该 fact 的活动不受影响。

### 2. Request Validation And Registry Safety

1. 请求 JSON 非法、缺少 `tenantId` / `factCode` / `subject` / `biz` / `attributes`、字段类型错误时返回 `INVALID_FACT_QUERY`。
2. 请求出现未知顶层字段时返回 `INVALID_FACT_QUERY`，未知字段不得透传 provider。
3. 请求试图传 inline `endpoint`、`auth`、`mapping`、`timeout_ms`、`cache_ttl_seconds`、`provider_type` 时返回 `INVALID_FACT_QUERY` 或被明确拒绝，不能覆盖 registry。
4. provider 未配置返回 `FACT_PROVIDER_NOT_CONFIGURED`。
5. provider disabled 返回 `FACT_PROVIDER_DISABLED`。
6. 不支持的 provider type 或 method 返回稳定可解释错误，不访问外部 provider。
7. `cache_ttl_seconds` 小于 0 或大于 3600 时视为无效 provider 配置，不执行 provider 请求。

### 3. Mapping Failures

1. required mapping source 缺失：

   - 返回 `errorCode=FACT_MAPPING_FAILED`。
   - `reason=MAPPING_REQUIRED_MISSING`。
   - 不写入缓存。

2. source 类型与 mapping 声明类型不匹配：

   - 返回 `errorCode=FACT_MAPPING_FAILED`。
   - `reason=MAPPING_TYPE_MISMATCH`。
   - 不写入缓存。

3. mapping key 非法：

   - key 包含 `facts.` 前缀、空 path、非法字符或路径穿越语义。
   - 返回 `reason=MAPPING_KEY_INVALID`。

4. mapping source 非法：

   - source 不是 `$` 起始简单对象路径，或包含通配、过滤、脚本、递归搜索。
   - 返回 `reason=MAPPING_SOURCE_INVALID`。

5. mapping 配置结构非法：

   - `mapping_json` 为空、非对象、缺少 source/type 或 type 不在允许集合。
   - 返回 `reason=MAPPING_CONFIG_INVALID`。

6. optional mapping source 缺失：

   - 查询成功。
   - 缺失字段从 facts 输出中省略。

### 4. Cache And Idempotency

1. TTL 内相同 provider 输入和相同 provider 配置版本：

   - 第一次访问 provider，`cached=false`。
   - 第二次不访问 provider，`cached=true`。
   - facts 内容与第一次一致。

2. `cache_ttl_seconds=0`：

   - 连续相同请求均访问 provider。
   - 响应均不应标记为缓存命中。

3. TTL 过期：

   - 使用 fake clock 推进到 TTL 后。
   - 下一次请求重新访问 provider。
   - 成功后刷新缓存。

4. 幂等重复：

   - 同一事件 `eventId`、同一 `subject`、同一 `biz`、同一 `attributes` 重复触发事实查询。
   - TTL 内只产生一次 provider 请求。
   - 活动事件处理重复消费时不能重复扣减额度或重复发放奖励；如果系统已有事件幂等表或 outbox 幂等约束，应断言幂等记录只生效一次。

5. `traceId` 不影响缓存：

   - 仅 `traceId` 不同，其余 provider 输入和配置版本相同。
   - 第二次必须命中缓存。

6. provider 输入上下文影响缓存：

   - 分别改变 `eventType`、`sourceSystem`、`occurredAt`、`subject.id`、`biz.bizId`、`attributes.amount`。
   - 每种变化都不得命中旧缓存，必须访问 provider。

7. provider 配置版本影响缓存：

   - 先以 `updated_at=t1` 查询并缓存。
   - 修改 `updated_at` 或等价配置指纹涉及的字段：`endpoint`、`method`、`auth_ref`、`mapping_json`、`cache_ttl_seconds`、`enabled`、`timeout_ms`。
   - 旧缓存 TTL 内再次查询也必须访问 provider，不得返回旧 facts。

8. 多租户隔离：

   - `tenantA` 与 `tenantB` 使用相同 `factCode`、subject、biz、attributes。
   - 两个租户不得共享 provider 配置或缓存结果。

### 5. Provider Failure And Security

1. endpoint host 不在 `EX_MARKETING_FACT_ALLOWLIST` 时返回 `FACT_PROVIDER_HOST_NOT_ALLOWLISTED`。
2. endpoint 解析到 private/local IP 时返回 `FACT_PROVIDER_PRIVATE_IP_BLOCKED`。
3. provider redirect 被拒绝，不能跟随跳转。
4. proxy 环境变量或系统代理不得影响 provider 请求。
5. provider 超时返回 `FACT_PROVIDER_TIMEOUT`，不缓存。
6. provider 返回非 2xx、网络错误、响应 JSON 不可解析时返回 `FACT_PROVIDER_HTTP_FAILED`，不缓存。
7. 失败响应、日志摘要和审计字段不得包含 secret、完整 token、完整 Authorization header 或 provider 明文凭据。

### 6. Insufficient Quota

1. attributes 路径命中但活动剩余额度不足：

   - 活动不得发奖。
   - 返回或记录稳定额度不足原因，例如现有系统中的 `QUOTA_INSUFFICIENT` / `REWARD_QUOTA_INSUFFICIENT` 等实际错误码。
   - 不应因为额度不足而触发不必要的 Fact Query。

2. facts 路径命中但活动剩余额度不足：

   - Fact Query 可成功返回 facts。
   - 奖励阶段因额度不足失败。
   - 不得把额度不足误报为 `FACT_QUERY_FAILED` 或 mapping 失败。
   - 不得重复扣减额度、冻结额度或写入重复奖励指令。

3. 幂等重复叠加额度不足：

   - 同一事件重复处理时，第一次已因额度不足拒绝。
   - 后续重复处理返回同一类拒绝结果或幂等结果。
   - 不得多次访问 provider 造成额外事实查询，也不得改变 quota/outbox 状态。

### 7. Concurrency

1. 并发相同 Fact Query：

   - N 个 goroutine 同时查询相同 `tenantId + factCode + provider 输入 + provider 配置版本`。
   - provider 请求次数应为 1 或符合实现明确的 singleflight / cache 策略上限。
   - 所有成功响应 facts 一致。
   - 无 data race。

2. 并发不同缓存 key：

   - 同时查询不同 subject、biz、attributes、eventType、tenant。
   - 不得串用 facts 或 provider 配置。

3. 并发配置变更：

   - 旧配置 TTL 缓存存在时，更新 provider 配置版本。
   - 新请求必须按新 key 查询 provider。
   - 不得出现新配置请求返回旧 mapping facts 的情况。

4. 并发额度扣减：

   - 多个事件同时命中同一活动，剩余额度小于请求数量。
   - 成功发奖数量不得超过额度。
   - 失败请求应为额度不足，不得出现负额度、重复 outbox 或事实查询异常掩盖额度错误。

### 8. PostgreSQL Smoke

1. 在 PostgreSQL 测试库准备 `marketing_fact_provider` 测试数据：

   - enabled provider。
   - disabled provider。
   - 不同 tenant 相同 factCode 的 provider。
   - 同一 provider 修改 `updated_at` 的版本变更场景。

2. 验证服务从 PostgreSQL 按 `tenant_id + fact_code` 读取 provider 配置：

   - enabled provider 可查询。
   - disabled provider 返回 `FACT_PROVIDER_DISABLED`。
   - 未配置返回 `FACT_PROVIDER_NOT_CONFIGURED`。
   - tenant 间不串配置。

3. 验证 PostgreSQL 事务或连接异常的最小 smoke：

   - provider 配置查询失败时返回稳定可解释错误或服务既有数据库错误包装。
   - 不应 panic。

4. 如实现涉及事件幂等、quota、outbox 或拒绝原因落库，应在 PostgreSQL smoke 中验证：

   - 重复事件只保留一份有效幂等处理结果。
   - 额度不足不会写入成功奖励 outbox。
   - facts 查询失败的拒绝原因可落库或可在处理结果中查询。

## Required Commands

后续 Test Agent 应按实际仓库结构执行以下命令；本 Test Plan Agent 不执行。

~~~bash
go test ./...
go test -race ./...
go build ./...
~~~

如仓库已有专用 integration 或 PostgreSQL smoke 标签，追加执行：

~~~bash
go test ./... -tags=integration
go test ./... -run 'Test.*Postgres.*|Test.*Fact.*Smoke.*'
~~~

## Data / Smoke

- 使用独立测试 tenant，例如 `tenant_fact_query_a`、`tenant_fact_query_b`。
- 使用独立 factCode，例如 `USER_PROFILE`、`RISK_PROFILE`。
- 使用 mock HTTP provider 统计请求次数、请求体、header、超时、redirect 和错误响应。
- 使用 fake clock 或可控 TTL，避免依赖真实等待。
- PostgreSQL smoke 必须使用测试库或事务回滚清理测试数据，不能污染共享环境。
- 所有涉及 secret/auth 的断言只验证脱敏行为，不打印明文密钥。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-fact-query-2
