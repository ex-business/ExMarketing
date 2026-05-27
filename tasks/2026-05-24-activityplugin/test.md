# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

- ActivityPlugin 注册表：
  - 覆盖重复 `pluginCode` 注册拒绝。
  - 覆盖重复注册失败后原插件仍可按编码查找。
- 插件生命周期与主流程：
  - 覆盖插件上下文进入规则、奖励和 snapshot。
  - 覆盖未知插件创建期拒绝和运行期拒绝。
  - 覆盖 simulate 输出插件 preview 且不持久化。
- 配置校验与字段路径：
  - 覆盖未知插件、插件与 activityType 不兼容。
  - 覆盖 `state.plugin.<pluginCode>.*` 合法引用。
  - 覆盖未绑定插件、引用其他插件、`plugin.*`、`state.<activityType>.*` 等非法路径拒绝。
- Quota / 幂等 / 并发：
  - 覆盖内存仓储 quota 不足无脏写。
  - 覆盖 participation / grant idempotency key 冲突。
  - 覆盖并发重复 grant 只处理一次。
  - 覆盖并发 quota 竞争不超扣。
- PostgreSQL smoke：
  - 启动 `docker compose` PostgreSQL。
  - 使用 `EX_MARKETING_DATABASE_URL` 运行 `go test ./... -count=1`。
  - 覆盖 PostgreSQL 仓储集成测试中的 idempotency、quota 不足无脏写、callback quota 确认/释放、并发 budget/stock 不超扣。

## Commands

~~~bash
go test ./...
~~~

结果：PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/plugin
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/marketing
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/workflow
all packages passed
~~~

~~~bash
go build ./...
~~~

结果：PASS

~~~bash
go test ./internal/... -run 'ActivityPlugin|Plugin|Quota|Idempotency|Concurrent' -count=1
~~~

结果：PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/marketing
ok github.com/ex-stack/ExMarketing/internal/plugin
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/rule
all matched internal packages passed
~~~

~~~bash
go test -race ./...
~~~

结果：PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/marketing
ok github.com/ex-stack/ExMarketing/internal/plugin
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/workflow
all packages passed with race detector
~~~

~~~bash
docker compose up -d postgres
~~~

结果：PASS

摘要：

~~~text
Container exmarketing-postgres Running
~~~

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -count=1
~~~

结果：PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/postgres 1.512s
ok github.com/ex-stack/ExMarketing/internal/plugin 1.085s
ok github.com/ex-stack/ExMarketing/internal/engine 0.432s
ok github.com/ex-stack/ExMarketing/internal/marketing 0.793s
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.816s
all packages passed with PostgreSQL URL enabled
~~~

## Result

~~~text
PASS
~~~

## Evidence

- 基础验证通过：`go test ./...`、`go build ./...`。
- 定向验证通过：插件、Quota、幂等、并发相关测试命令通过。
- Race 验证通过：`go test -race ./...` 未报告 data race。
- PostgreSQL smoke 通过：`docker compose up -d postgres` 后，使用真实 PostgreSQL URL 执行全量测试通过。
- 未执行手工 curl API smoke；原因是本轮已有自动化 HTTP/API 测试和 PostgreSQL 集成测试通过，且测试计划允许使用自动化 PostgreSQL smoke 覆盖。未发现需要打回 Dev Agent 的失败项。

需求：2026-05-24-activityplugin
