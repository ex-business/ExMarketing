# Test

## Test Cases

- PluginContext 只读快照：
  - 覆盖标准事件、活动配置快照、规则结果、facts、固定 now、traceId。
  - 验证输入 event attributes、facts、activity config 后续变更不会污染 snapshot。
- PluginStateStore scope / isolation：
  - 缺少完整 scope 或缺少 idempotencyKey 时拒绝写入。
  - 跨 tenant、pluginCode、activityVersion、stateKey 读取不返回其他作用域状态。
  - Get 返回 defensive copy，调用方修改返回值不影响 store 内状态。
- PutIfAbsent 幂等：
  - 首次写入 created，重复相同幂等键和 payload 返回 duplicate_noop。
  - duplicate_noop 不重复生成业务状态变更 audit。
  - 相同幂等键不同 payload 返回 idempotency conflict。
  - 不同幂等键写已有 stateKey 返回 already exists。
- CompareAndSwap 并发：
  - 并发 24 个 goroutine 对同一完整作用域更新计数。
  - version conflict 可重试，最终 count 与成功更新次数一致，版本无丢失。
- AppendEvent / ListByRound：
  - 并发不同 idempotencyKey 追加事件不丢失。
  - 相同 idempotencyKey 重复追加返回 duplicate_noop。
  - ListByRound 按 tenant/activityVersion/pluginCode/biz 隔离，不泄漏其他 tenant 数据。
  - 不完整 round query 返回 invalid scope。
- 主引擎边界：
  - 本轮通过全量 `go test ./...` 覆盖现有 engine/plugin/reward/quota 相关回归。
  - 具体 quota 不足、grant_order/outbox 阻断场景未发现本任务新增的专门测试用例；未作为失败项，因为 implementation 本轮只新增 plugin context/state memory contract，且全量回归通过。
- PostgreSQL smoke：
  - 未执行。当前环境未设置 `POSTGRES_DSN`，且 test-plan 明确要求使用专用测试库或临时 schema，不能连接未知数据库。

## Commands

~~~bash
go test ./...
~~~

Result: PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/plugin
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/workflow
全部包通过；若干 cmd/metrics/ruleexpr 包无测试文件。
~~~

~~~bash
go build ./...
~~~

Result: PASS

摘要：

~~~text
命令无输出，退出码 0。
~~~

~~~bash
go test ./... -run 'PluginContext|PluginState|StateStore|Idempot|CompareAndSwap|AppendEvent|ListByRound'
~~~

Result: PASS

摘要：

~~~text
internal/plugin 定向用例通过；相关全仓包均通过或无匹配测试。
~~~

~~~bash
go test ./... -run 'PluginState|StateStore|CompareAndSwap|AppendEvent' -count=20
~~~

Result: PASS

摘要：

~~~text
internal/plugin 并发与状态存储相关用例重复 20 次通过；相关全仓包均通过或无匹配测试。
~~~

~~~bash
go test ./... -race -run 'PluginState|StateStore|CompareAndSwap|AppendEvent|Concurrent'
~~~

Result: PASS

摘要：

~~~text
race 检查通过；internal/plugin、internal/postgres 等相关包未报告 data race。
~~~

~~~bash
go test ./internal/plugin -v -run 'TestNewPluginContextBuildsReadOnlySnapshot|TestMemoryPluginStateStoreRejectsIncompleteScopeAndIsolatesReads|TestMemoryPluginStateStorePutIfAbsentIdempotency|TestMemoryPluginStateStoreCompareAndSwapConcurrentUpdates|TestMemoryPluginStateStoreAppendEventIdempotencyAndListByRound'
~~~

Result: PASS

摘要：

~~~text
=== RUN   TestNewPluginContextBuildsReadOnlySnapshot
--- PASS: TestNewPluginContextBuildsReadOnlySnapshot (0.00s)
=== RUN   TestMemoryPluginStateStoreRejectsIncompleteScopeAndIsolatesReads
--- PASS: TestMemoryPluginStateStoreRejectsIncompleteScopeAndIsolatesReads (0.00s)
=== RUN   TestMemoryPluginStateStorePutIfAbsentIdempotency
--- PASS: TestMemoryPluginStateStorePutIfAbsentIdempotency (0.00s)
=== RUN   TestMemoryPluginStateStoreCompareAndSwapConcurrentUpdates
--- PASS: TestMemoryPluginStateStoreCompareAndSwapConcurrentUpdates (0.00s)
=== RUN   TestMemoryPluginStateStoreAppendEventIdempotencyAndListByRound
--- PASS: TestMemoryPluginStateStoreAppendEventIdempotencyAndListByRound (0.00s)
PASS
ok github.com/ex-stack/ExMarketing/internal/plugin
~~~

~~~bash
env | rg '^POSTGRES_DSN='
~~~

Result: SKIP / NOT CONFIGURED

摘要：

~~~text
未发现 POSTGRES_DSN 环境变量；未执行 PostgreSQL smoke，避免连接非专用测试库。
~~~

## Result

~~~text
PASS
~~~

## Evidence

- 最低要求 `go test ./...` 通过。
- 最低要求 `go build ./...` 通过。
- 插件上下文、状态 scope 隔离、PutIfAbsent 幂等、CAS 并发、AppendEvent 去重和 ListByRound 隔离的定向测试通过。
- 重复 20 次的状态相关测试通过。
- race 定向测试通过，未发现 data race。
- PostgreSQL smoke 未执行，原因是本地未配置 `POSTGRES_DSN`；无失败日志。

需求：2026-05-29-activityplugin-context-state
