# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` Decision 为 PASS 且本文件 Decision 为 PASS 时，才允许按本计划补充测试并执行命令。

本计划只定义后续测试用例和命令清单。Test Agent 可以新增或修改测试代码，但不得扩大业务实现范围。若测试发现实现缺口，记录到 `test.md`，交回 Dev Agent 做最小修复。

## Test Strategy

1. 优先补单元测试覆盖调度模型、任务注册、状态机、插件能力校验、事件映射和 Event Inbox 幂等语义。建议按最终实现位置放在对应包内，例如 `internal/plugin`、`internal/marketing`、`internal/engine` 或新增 scheduler 包。
2. 用可控时钟和 fake repository / fake Event Inbox，避免依赖真实时间。所有重复执行、重试、重放断言都必须比较完整标准 payload，而不只比较幂等 key。
3. 并发测试要验证 claim 的原子性：同一 `jobId` 多 goroutine 同时执行，只有一个执行者能进入插件执行和 Event Inbox 写入路径。
4. 额度不足测试必须证明调度任务只产生标准事件并走既有活动处理链路；当后续 quota/budget 不足时，不得绕过 Event Inbox，也不得直接创建 `grant_order`、`outbox` 或 quota reservation。
5. PostgreSQL smoke 必须使用真实 PostgreSQL，通过 `EX_MARKETING_DATABASE_URL` 指向测试库。未设置该环境变量时只能跳过 repository integration test，不能把 PostgreSQL smoke 判为通过。

## Test Cases

### TC-01 Normal Path: START Job Generates Stable Standard Event

- 准备活动、绑定插件、插件注册元数据：未禁用、支持活动类型、具备 `EVENT_HANDLE`。
- 注册 `START` 任务，字段包含 `jobId`、`tenantId`、`activityId`、`pluginCode`、`jobType=START`、带时区 `scheduledAt`、`status=PENDING`、`retryCount=0`。
- 将执行时钟推进到 `scheduledAt` 之后，执行器 claim 成功，状态流转为 `PENDING -> RUNNING -> SUCCESS`。
- 断言插件内部事件 `plugin.event.generated` 被转换为标准事件并写入 Event Inbox。
- 断言标准事件字段：
  - `sourceSystem=activity_plugin`
  - `eventType=PLUGIN_ROUND_STARTED` 或实现中稳定映射的 START 事件类型
  - `biz.bizType=PLUGIN_SCHEDULE_JOB`
  - `biz.bizId=jobId`
  - `occurredAt=job.scheduledAt`，不得等于执行器当前时间
  - `attributes` 至少包含 `activityId`、`pluginCode`、`jobId`、`jobType`、`scheduledAt`、`sequence`
- 断言没有直接创建 `grant_order`、`outbox` 或 quota reservation；只有标准事件进入后续链路。
- 断言可按 `tenantId + activityId + pluginCode` 查询到该 job 的 `SUCCESS` 状态和审计记录。

### TC-02 Normal Path: END / DRAW / SETTLE / COMPENSATE Mapping

- 分别注册 `END`、`DRAW`、`SETTLE`、`COMPENSATE` 到期任务。
- 对每种 `jobType` 执行一次成功路径，断言事件类型映射稳定、`eventId` 默认形如 `plugin:<jobId>:<sequence>` 或实现定义的稳定格式。
- 对 `COMPENSATE` 覆盖无事件成功路径：允许成功但必须写审计，且不得创建发奖相关记录。
- 对插件声明必须产出事件的 jobType，模拟空事件输出，断言任务失败为 `EVENT_MAPPING_INVALID`，不得写入 Event Inbox。

### TC-03 Capability and Binding Validation

- 插件未注册：注册或执行失败，错误分类 `PLUGIN_NOT_FOUND`。
- 插件 disabled：失败分类 `PLUGIN_DISABLED`。
- 插件不支持活动类型：失败分类 `PLUGIN_ACTIVITY_TYPE_UNSUPPORTED`。
- 插件缺少 `EVENT_HANDLE`：失败分类 `PLUGIN_CAPABILITY_MISSING`。
- 插件不是活动绑定插件：注册或执行被拒绝，错误分类按实现定义但必须可审计，且不得执行插件。
- 断言这些不可重试错误不得自动进入 `RETRY`，不得写 Event Inbox。

### TC-04 Duplicate Registration Idempotency

- 用同一 `tenantId + activityId + pluginCode + jobType + scheduledAt` 注册两次任务。
- 断言第二次返回已有任务或幂等成功，不创建第二条等价任务。
- 断言不同 `tenantId` 或不同 `scheduledAt` 可以创建独立任务。
- 断言无时区、空值、无法解析或不允许的历史 `scheduledAt` 被拒绝；历史补偿必须通过 `COMPENSATE` 表达。

### TC-05 Idempotent Duplicate Execution Same Payload

- 准备一个已成功写入 Event Inbox 的 job。
- 重复执行同一 `jobId`，或模拟 Event Inbox 对同 key 同 payload 返回 duplicate。
- 断言调度任务保持或返回 `SUCCESS`，插件不重复产生会导致发奖的业务结果。
- 断言重复生成的完整标准 payload 与第一次字节语义一致，包括 `eventId`、`subject`、`occurredAt`、`attributes`。
- 断言没有新增 participation、grant_order、outbox 或 quota reservation。

### TC-06 Idempotency Conflict Same Key Different Payload

- 第一次执行 job 成功写入 Event Inbox。
- 第二次模拟同一幂等 key 但 `eventId`、`occurredAt` 或 `attributes` 变化。
- 断言 Event Inbox 返回 `ErrIdempotencyConflict` 或等价错误时，任务失败分类为 `IDEMPOTENCY_CONFLICT`。
- 断言任务不得标记 `SUCCESS`，不得自动重试，必须写审计供人工介入。

### TC-07 Stable Payload After Activity Config Changes

- 注册调度任务并记录首次生成的标准 payload。
- 在任务重试或重放前修改活动配置中会影响插件输出的字段。
- 若实现保存 `activityVersion` 或 `configSnapshot`，断言重试仍使用快照生成与首次一致的 payload。
- 若实现不保存快照，断言标准 payload 中不包含可变活动配置字段，重复执行仍与首次一致。
- 若 payload 变化，测试必须失败并记录 review.md 中的 P2 风险。

### TC-08 Retryable Failures and Manual Close

- 模拟插件执行异常，断言状态 `RUNNING -> RETRY`，`retryCount` 递增，`nextRetryAt` 按默认退避 `1m, 5m, 15m` 或实现配置生成。
- 模拟 Event Inbox 暂时失败，断言 `EVENT_INBOX_FAILED` 可重试。
- 连续失败达到默认 3 次后，断言任务停在 `FAILED`，不再自动执行。
- 人工关闭失败任务时必须提供 `operator` 和 `reason`，断言状态进入 `CLOSED` 并写审计。
- 缺少 `operator` 或 `reason` 时关闭失败。
- 断言 `SUCCESS` 和 `CLOSED` 终态不能重新进入 `RUNNING`。

### TC-09 Concurrent Claim Single Job

- 创建一个到期 `PENDING` job。
- 用至少 20 个 goroutine 同时调用 claim/execute。
- 断言只有一个 goroutine claim 成功并调用插件执行；其他 goroutine 不得调用插件，不得写 Event Inbox。
- 断言最终只有一条标准事件进入 Event Inbox，job 只有一次成功审计。

### TC-10 Running Job Recovery

- 模拟任务已被 claim 到 `RUNNING`，但执行器崩溃未写终态。
- 补偿扫描在超时后只能把非终态 `RUNNING` 转为 `RETRY`。
- 若任务已被确认 `SUCCESS`，补偿扫描不得改回 `RETRY` 或再次执行。
- 断言恢复动作写审计。

### TC-11 Insufficient Quota / Budget Through Standard Chain

- 准备一个调度事件会命中活动规则和奖励计算的场景，但活动 budget、stock 或 user limit 不足。
- 执行调度 job，断言插件只生成标准事件，Event Inbox 接收成功或按既有幂等返回。
- 断言后续活动处理因额度不足返回既有 quota exceeded / not rewardable 语义，调度层不得直接改 quota 或直接创建发奖记录。
- 断言数据库或 fake repo 中没有对应 `grant_order`、`outbox`，quota used/frozen 未错误增加。
- 重复执行同一 job 时仍不得多扣额度或多建发奖记录。

### TC-12 Audit and Sensitive Data

- 覆盖创建、重复注册、claim、成功、失败、重试、关闭、事件转换、Event Inbox duplicate、Event Inbox conflict 的审计记录。
- 断言审计字段包含 `tenantId`、`activityId`、`pluginCode`、`jobId`、`jobType`、状态变化和错误分类。
- 模拟错误消息中包含 `Authorization`、`Cookie`、`token`、`secret`，断言落库或日志摘要已脱敏。

### TC-13 PostgreSQL Repository Smoke

- 在真实 PostgreSQL 下创建调度 job，查询 `tenantId + activityId + pluginCode` 状态。
- 重复注册同一 job，断言 repository 层唯一约束或事务语义不产生重复任务。
- 并发 claim 同一 job，断言 PostgreSQL 下只有一个 claim 成功。
- 执行成功后重复执行，断言 Event Inbox 同 key 同 payload duplicate 语义正常。
- 模拟同 key 不同 payload，断言 PostgreSQL Event Inbox 返回幂等冲突。
- PostgreSQL smoke 必须使用唯一前缀或 run id 清理数据，避免污染共享测试库。

## Required Commands

Test Agent 完成测试代码后按顺序执行以下命令，并把关键输出写入 `test.md`：

~~~bash
go test ./...
go build ./...
~~~

若实现新增独立 scheduler 包，先单独运行聚焦测试，便于定位：

~~~bash
go test ./internal/... -run 'Test.*Plugin.*Schedule|Test.*Schedule.*Plugin|Test.*Scheduler|Test.*Idempot'
~~~

PostgreSQL smoke 使用真实测试库执行。示例 DSN 与现有项目文档一致，实际可按本地环境调整：

~~~bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go test ./internal/postgres -run 'TestPostgres.*Schedule|TestPostgresCreateInboxDetectsDuplicateAndConflict' -count=1
go test ./... -count=1
~~~

如果 Dev Agent 提供了专用 PostgreSQL E2E 命令或新增 scheduler smoke 命令，Test Agent 还必须追加执行该命令；若未提供，应在 `test.md` 标明未提供专用命令，仅执行 repository smoke 与全量测试。

## Data / Smoke

- 建议测试租户：`site_001`、`site_002`，用于验证多租户隔离。
- 建议活动：一个 ACTIVE 且审批通过的场次类活动，一个 inactive 活动用于失败分类。
- 建议插件：`round_lottery` 或实现内测试插件，能力包含 `EVENT_HANDLE`；另准备 disabled、missing capability、unsupported activity type 三个变体。
- 建议 job 类型全覆盖：`START`、`END`、`DRAW`、`SETTLE`、`COMPENSATE`。
- 建议固定时间：`2026-05-29T10:00:00Z` 作为 `scheduledAt`，执行器 `now` 使用 `2026-05-29T10:05:00Z`，用于断言 `occurredAt` 不取执行时刻。
- PostgreSQL smoke 数据必须带唯一前缀，例如 `scheduler_pg_<timestamp>`，测试结束清理该前缀下的 job、audit、inbox、participation、grant_order、outbox、quota 数据。

## Exit Criteria

- 正常路径、额度不足、幂等重复、幂等冲突、并发 claim、PostgreSQL smoke 均有自动化测试或明确的 smoke 验证记录。
- 所有调度插件事件必须走 Event Inbox；测试能证明没有调度层直接发奖路径。
- 同一 job 重复执行的完整标准 payload 稳定，特别是 `occurredAt=scheduledAt`。
- `go test ./...` 和 `go build ./...` 通过。
- PostgreSQL smoke 在设置 `EX_MARKETING_DATABASE_URL` 的真实 PostgreSQL 环境下通过；如果环境不可用，`test.md` 必须将该项记为 BLOCK 或环境阻塞，不能算 PASS。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-activityplugin-scheduler-events
