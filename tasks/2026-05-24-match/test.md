# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已执行 test-plan.md 要求的核心测试与 smoke：

- MATCH 查询边界：`TestSimulateEventMatchFiltersAndReasons` 覆盖同租户 + eventType 可解释全集，验证跨租户活动和非触发 eventType 活动不进入 matched/unmatched/summary。
- MATCH 基础原因码：同一测试覆盖 `ACTIVITY_NOT_ACTIVE`、`ACTIVITY_NOT_STARTED`、`ACTIVITY_ENDED`。
- 入口、人群、配置错误：`TestSimulateEventMatchEntryAudienceAndConfigReasons` 覆盖 `ENTRY_DISABLED`、`ENTRY_CHANNEL_NOT_ALLOWED`、`ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`、`ENTRY_TENANT_NOT_ALLOWED`、`AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED`、`AUDIENCE_SEGMENT_NOT_ALLOWED`、`AUDIENCE_RISK_LEVEL_NOT_ALLOWED`、`ERROR/INVALID_MATCH_CONFIG`。
- MATCH 规则失败：`TestSimulateEventMatchRuleFailureUsesMatchReason` 覆盖启用 MATCH 规则失败时输出 `REJECTED/MATCH_RULE_REJECTED` 并保留规则解释。
- 互斥裁剪：`TestSimulateEventExclusiveGroupSelection` 覆盖空互斥组不互斥、同组全 stackable 保留、同组存在 non-stackable 时裁剪并输出 `EXCLUSIVE_GROUP_LOST`。
- 展示入口：`TestDisplayVisibleFalseDoesNotBlockEntryMatching` 覆盖 `display.visible=false` 不阻断内部 MATCH。
- 模拟无副作用与 quota 预览：`TestSimulateEventReturnsRuleExplainWithoutPersistence`、`TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence`、`TestSimulateEventQuotaExceededSuppressesGrantPreview` 覆盖模拟接口解释、quota 预览和无持久化副作用。
- 幂等、额度不足、并发/唯一约束相关 smoke：`internal/marketing`、`internal/httpapi`、`internal/postgres` 包测试覆盖 duplicate/conflict、budget/user limit/stock exceeded、atomic rollback、quota competition、outbox/grant rollback。
- PostgreSQL smoke：使用真实 `EX_MARKETING_DATABASE_URL` 执行 `internal/postgres` 以及 `internal/httpapi`、`internal/engine`。

## Commands

~~~bash
go test ./internal/marketing
# ok github.com/ex-stack/ExMarketing/internal/marketing (cached)

go test ./internal/rule
# ok github.com/ex-stack/ExMarketing/internal/rule (cached)

go test ./internal/httpapi
# ok github.com/ex-stack/ExMarketing/internal/httpapi 0.433s

go test ./internal/engine
# ok github.com/ex-stack/ExMarketing/internal/engine (cached)

go test ./...
# ok; all packages passed, including internal/engine, internal/httpapi, internal/marketing, internal/postgres, internal/rule

go test -race ./internal/marketing ./internal/httpapi ./internal/engine
# ok github.com/ex-stack/ExMarketing/internal/marketing 1.229s
# ok github.com/ex-stack/ExMarketing/internal/httpapi 1.869s
# ok github.com/ex-stack/ExMarketing/internal/engine 1.356s

go build ./...
# ok

docker compose ps
# exmarketing-postgres was already running and healthy before this Test Agent run

docker compose up -d postgres
# Container exmarketing-postgres Running

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
# ok github.com/ex-stack/ExMarketing/internal/postgres 0.336s

EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/httpapi ./internal/engine
# ok github.com/ex-stack/ExMarketing/internal/httpapi (cached)
# ok github.com/ex-stack/ExMarketing/internal/engine (cached)
~~~

未执行：

~~~bash
docker compose down
~~~

原因：`exmarketing-postgres` 在本轮测试开始前已运行 22 小时且处于 healthy 状态。为避免停掉用户已有本地服务，本次仅复用并确认容器 running/healthy，不执行 teardown。

## Result

~~~text
PASS
~~~

## Evidence

- 必需最低命令 `go test ./...`、`go build ./...` 均通过。
- test-plan.md 指定的定向命令 `go test ./internal/marketing`、`go test ./internal/rule`、`go test ./internal/httpapi`、`go test ./internal/engine` 均通过。
- race 命令 `go test -race ./internal/marketing ./internal/httpapi ./internal/engine` 通过，未发现 data race。
- PostgreSQL smoke 命令通过，数据库 URL 为 `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。
- 未发现 `MATCHED / UNMATCHED` 新 decision 枚举相关测试失败；现有测试断言仍使用 `PASSED / REJECTED / SKIPPED / ERROR` 语义。
- 未发现 `TENANT_NOT_MATCHED` 或 `EVENT_TYPE_NOT_MATCHED` 作为常规 MATCH 响应泄漏的测试失败。
- 未发现模拟接口写入持久化副作用、额度失败部分写入或重复扣减相关测试失败。

需求：2026-05-24-match
