# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Scope

- 读取并核对了 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md`。
- 复测 Dev Agent 声明已修复的 WARN-only 发布决策问题。
- 执行本任务相关 prelaunch 单元测试，覆盖概率、规则字段、奖励 schema、quota Decimal、高危阻断、预算可用额度、库存缺失、互斥冲突、插件注册、WARN-only 允许上线。
- 执行 prelaunch HTTP 接口测试，覆盖缺失 fact provider 阻断和逐项高危 `BLOCK` 返回。
- 执行全仓 Go 单元测试、race 测试和构建。
- 检查 PostgreSQL smoke 执行状态；当前环境未设置 `EX_MARKETING_DATABASE_URL`，相关集成测试被测试用例主动跳过，未连接真实 PostgreSQL。

## Commands

~~~bash
go test ./internal/marketing -run 'TestValidatePrelaunchWarnOnlyStillAllowsGoLive|TestValidatePrelaunch|TestCreateActivityCommandValidatesProbabilityTotalWeight|TestCreateActivityCommandValidatesRuleSchema|TestCreateActivityCommandValidatesRewardSchemaBoundaries|TestCreateQuotaCommandValidatesDecimalAndIntegerSemantics' -count=1

awk '
  /case hasWarn:/ {
    getline
    if ($0 ~ /conclusion = PrelaunchWarn/) {
      print "FAIL: warn-only prelaunch result sets conclusion to WARN; expected PASS with allowGoLive=true when no BLOCK items exist"
      print FILENAME ":" NR ": " $0
      exit 1
    }
  }
' internal/marketing/prelaunch_validation.go

go test ./internal/httpapi -run 'TestValidatePrelaunchEndpointReturnsItemizedHighRiskBlock|TestPrelaunchValidationBlocksMissingFactProvider' -count=1

go test ./... -count=1

go build ./...

go test ./internal/postgres -run 'TestPostgres' -count=1 -v

go test -race ./... -count=1
~~~

## Result

~~~text
PASS
~~~

## Evidence

通过项：

~~~text
go test ./internal/marketing ... PASS
awk WARN-only assertion PASS
go test ./internal/httpapi ... PASS
go test ./... -count=1 PASS
go build ./... PASS
go test -race ./... -count=1 PASS
~~~

关键输出摘要：

~~~text
ok   github.com/ex-stack/ExMarketing/internal/marketing  0.240s
ok   github.com/ex-stack/ExMarketing/internal/httpapi    0.188s
ok   github.com/ex-stack/ExMarketing/internal/postgres   0.920s
ok   github.com/ex-stack/ExMarketing/internal/workflow   2.057s
~~~

race 输出摘要：

~~~text
ok   github.com/ex-stack/ExMarketing/internal/decimal    1.174s
ok   github.com/ex-stack/ExMarketing/internal/engine     1.270s
ok   github.com/ex-stack/ExMarketing/internal/fact       1.513s
ok   github.com/ex-stack/ExMarketing/internal/httpapi    1.609s
ok   github.com/ex-stack/ExMarketing/internal/marketing  1.611s
ok   github.com/ex-stack/ExMarketing/internal/postgres   1.797s
ok   github.com/ex-stack/ExMarketing/internal/reward     1.888s
ok   github.com/ex-stack/ExMarketing/internal/rule       2.048s
ok   github.com/ex-stack/ExMarketing/internal/worker     1.721s
ok   github.com/ex-stack/ExMarketing/internal/workflow   2.141s
~~~

PostgreSQL smoke 状态：

~~~text
go test ./internal/postgres -run 'TestPostgres' -count=1 -v
...
EX_MARKETING_DATABASE_URL is not set
--- SKIP: TestPostgres...
PASS
ok   github.com/ex-stack/ExMarketing/internal/postgres  1.094s
~~~

## Notes

- 上一轮 `test.md` 记录的 WARN-only 失败已不再复现；只读 `awk` 断言通过，且 `TestValidatePrelaunchWarnOnlyStillAllowsGoLive` 通过。
- 本轮未发现需要打回 Dev Agent 的失败。
- PostgreSQL smoke 未在真实数据库上执行；如需覆盖 test-plan 中的真实 PostgreSQL 持久化 smoke，需要提供 `EX_MARKETING_DATABASE_URL` 后重新运行相关用例。

需求：2026-05-24-113929-6
