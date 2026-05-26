# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- PASS: Provider schema / mapping 契约覆盖。现有测试覆盖 `facts.` 前缀、尾随点、通配 source、过滤 source、非法 type 等非法 mapping 拒绝。
- PASS: facts 路径校验覆盖。现有测试覆盖 `facts`、`facts.USER_PROFILE`、`facts..vipLevel`、`facts.USER_PROFILE.` 返回 `FACT_FIELD_INVALID`。
- PASS: 嵌套字段精确覆盖。现有测试覆盖 `facts.USER_PROFILE.risk.score` 提取为 `risk.score`，以及只有 mapping `risk` 时返回 `FACT_FIELD_MISSING`。
- PASS: 活动创建校验覆盖。现有 HTTP API 测试覆盖配置 provider 后可引用 facts 字段，missing provider 返回 `FACT_PROVIDER_NOT_CONFIGURED`，disabled provider 返回 `FACT_PROVIDER_DISABLED`。
- PASS: Fact Query registry 约束覆盖。现有 HTTP API 测试覆盖请求内 inline provider 字段被拒绝，未写入 registry 时查询返回 `FACT_PROVIDER_NOT_CONFIGURED`。
- PASS: Fact Query 执行覆盖。现有 fact client 测试覆盖 HTTP provider 响应 mapping、缓存、provider HTTP 失败、required=false 字段缺失可省略。
- PASS: PostgreSQL repository 回归随 `go test ./...` 执行通过，覆盖 fact provider 持久化相关包编译与测试。

## Commands

~~~bash
go test ./...
~~~

结果：PASS。

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/decimal
ok github.com/ex-stack/ExMarketing/internal/engine
ok github.com/ex-stack/ExMarketing/internal/fact
ok github.com/ex-stack/ExMarketing/internal/httpapi
ok github.com/ex-stack/ExMarketing/internal/marketing
ok github.com/ex-stack/ExMarketing/internal/postgres
ok github.com/ex-stack/ExMarketing/internal/reward
ok github.com/ex-stack/ExMarketing/internal/rule
ok github.com/ex-stack/ExMarketing/internal/worker
ok github.com/ex-stack/ExMarketing/internal/workflow
~~~

~~~bash
go build ./...
~~~

结果：PASS。

摘要：命令退出码为 0，无错误输出。

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` 完成，退出码 0。
- `go build ./...` 完成，退出码 0。
- 未发现失败命令，因此无失败日志、失败原因或复现步骤需要打回 Dev Agent。
- 本轮仅写入 `tasks/2026-05-24-fact-provider/test.md`，未修改业务代码、测试代码、配置文件或其他文件。

需求：2026-05-24-fact-provider
