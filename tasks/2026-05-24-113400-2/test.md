# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

- 活动统一配置创建读回：覆盖 activity `config.entry/display/audience/metadata`、rule version、reward version、`rewardConfig.targetRef` 持久化读回。
- 非法 reward delivery 拒绝：覆盖缺失 `targetRef`、`grantChannel=MQ`、`grantMode=MANUAL`、`rewardConfig` 未知字段。
- 非法 activity/rule schema 拒绝：覆盖重复 `triggerEvents`、activity `config` 未知字段、非法 rule op、非法 version。
- 事件执行链路：合法活动命中后生成 grant orders 和 outbox；RATIO 奖励按 `attributes.amount` 计算。
- 幂等与标准事件校验：覆盖缺失/不匹配 Idempotency-Key、重复事件、payload 冲突、标准事件未知字段和嵌套未知字段。
- 额度不足：覆盖 BUDGET、USER_LIMIT、STOCK 不足时返回冲突、写审计、不产生脏 grant/outbox。
- 原子回滚：覆盖多奖励第二个库存不足时 memory / PostgreSQL 下 participation、grant_order、outbox、quota 扣减整体回滚。
- PostgreSQL smoke：覆盖真实 PostgreSQL 连接下 migration、quota 原子扣减、inbox 幂等重复和冲突。
- 构建验证：覆盖全仓 `go build ./...`。

补充核对：

- `POST /admin/v1/activities` 使用 `json.Decoder.DisallowUnknownFields()`，顶层未知字段会被创建接口拒绝。
- `config`、`ruleConfig`、`rewardConfig` 使用 strict decode，嵌套未知字段会被拒绝。
- 当前测试未新增业务代码；仅执行现有自动化测试并核对实现路径。

## Commands

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run Test -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Activity|Quota|Idempot|Outbox|Config' -count=1
go test ./internal/httpapi -run 'TestCreateActivity|TestActivityEvent|TestQuota|TestIdempot|TestOutbox|TestConfig' -count=1 -v
go test ./internal/marketing ./internal/reward ./internal/rule -run 'Test.*' -count=1 -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...` passed.
  - `internal/httpapi` passed.
  - `internal/marketing` passed.
  - `internal/postgres` passed.
  - `internal/reward` passed.
  - `internal/rule` passed.
  - `internal/workflow` passed.
- `go build ./...` passed.
- PostgreSQL smoke passed against `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`.
- Verbose related test evidence:
  - `TestCreateActivityReturnsUnifiedConfigVersions` PASS.
  - `TestCreateActivityRejectsUnsupportedRewardDelivery/missing_targetRef` PASS.
  - `TestCreateActivityRejectsUnsupportedRewardDelivery/mq_channel` PASS.
  - `TestCreateActivityRejectsUnsupportedRewardDelivery/manual_mode` PASS.
  - `TestCreateActivityRejectsUnsupportedRewardDelivery/unknown_config` PASS.
  - `TestCreateActivityRejectsInvalidConfigAndRuleSchema/duplicate_trigger` PASS.
  - `TestCreateActivityRejectsInvalidConfigAndRuleSchema/unknown_config` PASS.
  - `TestCreateActivityRejectsInvalidConfigAndRuleSchema/bad_rule_op` PASS.
  - `TestCreateActivityRejectsInvalidConfigAndRuleSchema/bad_version` PASS.
  - `TestActivityEventCreatesGrantCommand` PASS.
  - `TestActivityEventDuplicateAndConflict` PASS.
  - `TestActivityEventBudgetExceededReturnsConflictAndAudit` PASS.
  - `TestActivityEventUserLimitExceededReturnsConflictAndAudit` PASS.
  - `TestActivityEventStockExceededReturnsConflictAndNoRewardableResult` PASS.
  - `TestMemoryAtomicGrantRollsBackWhenSecondRewardStockExceeded` PASS.
  - `TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox` PASS.
  - `TestMemoryCreateInboxDetectsDuplicateAndConflict` PASS.
  - `TestConfigCalculatorRatioUsesAttributesBaseField` PASS.
  - `TestConfigCalculatorRejectsNonAttributesBaseField` PASS.
  - `TestConditionEvaluatorStandardEventFields` PASS.
  - `TestConditionEvaluatorRejectsNonStandardTopLevelField` PASS.

需求：2026-05-24-113400-2
