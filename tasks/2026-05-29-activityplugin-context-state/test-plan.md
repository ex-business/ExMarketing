# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

在 `review.md` Decision 为 PASS 的前提下，后续 Test Agent 需要验证插件上下文与插件状态存储的契约，而不是验证具体玩法算法。

测试分为四层：

1. 单元测试：覆盖 `PluginContext` 只读快照、完整状态作用域校验、幂等键、状态版本、审计摘要字段。
2. 存储实现测试：Memory 与 PostgreSQL 或等价持久化实现必须跑同一组 contract cases，确保 `Get`、`PutIfAbsent`、`CompareAndSwap`、`AppendEvent`、`ListByRound` 语义一致。
3. 并发测试：验证同一用户/同一场次的并发创建、CAS 更新和事件追加不会覆盖或丢失。
4. 集成 smoke：验证插件状态不能替代或直接修改主引擎 `participation`、`grant_order`、`quota`、`outbox`，并覆盖 PostgreSQL 条件写原子性。

Review P2 审计口径按以下标准执行：重复幂等请求不得重复生成业务状态变更审计或重复 before/after 变更记录，但必须通过返回值、指标、日志或去重审计摘要之一可观测。

## Test Cases

### 1. 正常路径

- 构造标准事件、活动配置快照、规则结果、facts、固定 `now` 和 `traceId`，确认 `PluginContext` 包含需求要求的最小字段，并且插件无法修改原始 context 对象影响后续处理。
- 使用完整作用域 `tenantId + activityId + activityVersion + pluginCode + subject + biz + stateKey` 执行 `PutIfAbsent`，预期首次创建成功，版本为初始版本，审计包含 scope、eventId、idempotencyKey、traceId、operation、result、before/after 摘要。
- 对同一状态执行 `Get`，预期只返回同一完整作用域下的 payload 和版本。
- 对已有状态执行 `CompareAndSwap`，传入正确 `expectedVersion`，预期更新成功、版本递增、before/after 摘要反映变化。
- 使用不同幂等键执行 `AppendEvent` 多次，预期事件全部保留，顺序或序号可追踪，`ListByRound` 能按同一 tenant/activityVersion/pluginCode/biz 范围列出。
- 验证规则或奖励读取插件上下文时，只允许 `state.plugin.<pluginCode>.*` 路径，不接受短路径或跨插件路径。

### 2. 额度不足 / 主引擎边界

- 构造 quota 不足或预算不足事件，使插件可生成预检结果或状态变更意图，但主引擎拒绝最终发奖。
- 预期 quota 不足时不创建 `grant_order`，不写 `outbox` 发奖消息，不把插件状态当作最终发奖事实来源。
- 如果实现允许先写插件过程状态，必须验证该状态只表示玩法过程或失败原因，不增加用户可领奖额度，不绕过 quota。
- 验证插件代码不能直接调用或修改 quota、grant_order、outbox 的写接口；若存在 stateChangeIntents，也必须由主引擎授权处理。
- 验证 participation 快照记录插件摘要、预检失败原因或 stateChangeIntents，但不代表奖励已发放。

### 3. 幂等重复

- 使用同一 `idempotencyKey + pluginCode + stateKey` 重复执行 `PutIfAbsent`，预期返回首次结果或 `duplicate_noop`，payload、版本、计数不重复变化。
- 使用同一幂等键但不同 payload 重复写入，预期返回幂等冲突，不覆盖首次状态。
- 使用同一幂等键重复执行 `AppendEvent`，预期事件流只出现一条业务事件，不重复追加。
- 重复标准事件处理完整链路，预期插件抽奖次数、翻卡进度、红包池过程余额或等价计数不重复消费。
- 验证重复 no-op 的可观测性：返回值、指标、日志或去重审计摘要至少存在一种；同时确认不重复生成业务状态变更审计。

### 4. 并发

- 并发 N 个 goroutine 对同一完整作用域、同一 stateKey 执行 `PutIfAbsent`，预期只有一个 created，其余为 existing、duplicate_noop 或 conflict，最终只有一份状态。
- 并发 N 个 goroutine 基于同一初始版本执行 `CompareAndSwap`，预期只有符合版本条件的更新成功，失败方返回可识别 version conflict，不发生静默覆盖。
- 并发 N 个不同幂等键执行 `AppendEvent`，预期最终事件数量等于不同幂等键数量，事件不丢失；同一幂等键并发追加只保留一条。
- 对 Memory 和 PostgreSQL 实现重复执行上述并发 contract cases，确认行为一致。
- 在并发失败场景下检查审计或观测记录，确认 conflict/rejected 可追踪到 traceId 和 idempotencyKey。

### 5. 隔离与拒绝

- 缺少 `tenantId`、`activityId`、`activityVersion`、`pluginCode`、`subject`、`biz` 或 `stateKey` 任一维度时写入，预期拒绝，不能降级为全局状态。
- tenant 与标准事件或活动配置 tenant 不一致时读取/写入，预期拒绝或无结果，并可观测。
- 跨 tenant、跨 activity、跨 activityVersion、跨 pluginCode、跨 subject、跨 biz、跨 stateKey 读取，预期不返回其他作用域状态。
- `ListByRound` 不得跨 tenant 或跨 pluginCode 汇总；相同 round/biz 下不同 tenant/pluginCode 的数据必须隔离。
- activityVersion 变化后，新旧版本状态默认隔离，不自动迁移。

### 6. PostgreSQL Smoke

- 使用 PostgreSQL store 初始化测试 schema 或测试容器数据库，执行完整 contract smoke：`PutIfAbsent`、重复 `PutIfAbsent`、正确版本 CAS、错误版本 CAS、`AppendEvent`、重复 `AppendEvent`、`ListByRound`。
- 验证数据库唯一约束或事务条件写能支撑 `idempotencyKey + pluginCode + stateKey` 去重，以及完整状态作用域隔离。
- 验证 PostgreSQL 并发 CAS 不丢更新：并发写完成后读取最终版本、payload 和审计记录，确认仅成功写入被持久化。
- 验证 PostgreSQL 回滚/冲突路径不会留下半写状态、半写事件或重复审计。
- Smoke 结束后清理测试数据或使用隔离测试库，避免污染开发/共享环境。

## Required Commands

后续 Test Agent 应按项目实际包路径调整精准测试命令；最少执行以下命令清单：

~~~bash
go test ./...
go build ./...
~~~

建议补充执行的定向命令：

~~~bash
go test ./... -run 'PluginContext|PluginState|StateStore|Idempot|CompareAndSwap|AppendEvent|ListByRound'
go test ./... -run 'PluginState|StateStore|CompareAndSwap|AppendEvent' -count=20
go test ./... -race -run 'PluginState|StateStore|CompareAndSwap|AppendEvent|Concurrent'
POSTGRES_DSN='postgres://<user>:<password>@<host>:<port>/<db>?sslmode=disable' go test ./... -run 'Postgres|PluginStateStore|StateStoreContract|Smoke'
~~~

## Data / Smoke

- 标准事件样例：同一 tenant 下使用固定 `eventId`、`idempotencyKey`、`eventType`、`sourceSystem`、`subject`、`biz`、`occurredAt`、attributes；重复测试必须复用同一幂等键。
- 活动配置样例：包含 `activityId`、`activityVersion`、`pluginCode`、插件配置、规则摘要和奖励摘要；跨版本测试使用相同 activityId 但不同 activityVersion。
- 隔离样例：准备 tenantA/tenantB、pluginA/pluginB、stateKeyA/stateKeyB、bizRoundA/bizRoundB，确保任一维度变化都不能读到其他维度数据。
- 额度不足样例：构造 quota/budget 剩余为 0 或低于奖励需求的活动，确认主引擎拒绝发奖且插件状态不成为发奖事实。
- 审计样例：每个写操作至少断言 operation、result、traceId、eventId 或 idempotencyKey、scope、before/after 摘要；重复 no-op 不应重复业务变更审计。
- PostgreSQL smoke 使用专用测试库或临时 schema，测试前后隔离数据，禁止连接生产库。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-activityplugin-context-state
