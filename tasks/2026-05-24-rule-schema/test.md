# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已按 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Test Agent 验证。

覆盖项：

- Rule Schema 正常路径：标准事件字段 `eventType`、`subject.id`、`biz.bizId` 和 `attributes.amount` 的合法 `CONDITION` 校验通过。
- Rule Schema 非法路径：非法 `rulePhase`、缺失 `logic`、空 `conditions`、未知配置字段、非法字段路径、非法 `op`、缺失 `value`、布尔大小比较、`IN` 标量、`IN` 空数组、`enabled=false` 非法 Schema、重复 `ruleCode`、负 `sort` 均被拒绝。
- `FORMULA / LIMIT / SCRIPT`：结构 Schema 合法后仍因 evaluator 未注册被拒绝，未保存为默认通过规则。
- Admin API / prelaunch：创建活动拒绝非法 rule schema；缺失 Fact Provider 的创建和上线前校验被阻断。
- 运行链路回归：标准事件字段 evaluator 可读取；非标准顶层字段被拒绝；规则阶段排序、禁用规则跳过、缺失 fact 拒绝路径已覆盖。
- 额度、幂等、并发回归：内存仓储和 HTTP API 的预算不足、用户次数不足、幂等重复/冲突、并发 quota 竞争、回调幂等等现有回归均通过。
- PostgreSQL smoke：仓库存在 PG 集成测试，但当前环境未设置 `EX_MARKETING_DATABASE_URL`，对应测试按代码约定 skip；未执行真实 PG 连接验证。

## Commands

~~~bash
go test ./...
go build ./...
go test ./... -run 'Rule|Schema|Condition|Formula|Limit|Script|Activity'
go test ./... -run 'Idempot|Duplicate|Quota|Budget|Limit|Concurrent|Postgres|PG'
go test ./internal/marketing -run 'TestCreateActivityCommandValidatesRuleSchema|TestCreateActivityCommandRejectsUnregisteredRuleEvaluatorsAfterSchemaValidation' -count=1 -v
go test ./internal/httpapi -run 'TestCreateActivityRejectsInvalidConfigAndRuleSchema|TestCreateActivityValidatesFactProviderReferences|TestPrelaunchValidationBlocksMissingFactProvider|TestActivityEventRuleRejectionReturnsExplainWithoutGrant|TestActivityEventQuotaExceededPreventsGrantOrder|TestActivityEventIdempotencyReturnsExistingResult|TestConcurrentQuotaReservationOnlyAllowsCapacity' -count=1 -v
go test ./internal/postgres -run 'TestPostgres' -count=1 -v
go test ./internal/rule -run 'TestExecuteRunsFourStagesWithStableRuleDetails|TestExecuteSkipsDisabledAndRejectsMissingFact|TestRegistryReturnsErrorForUnknownRuleType|TestConditionEvaluator' -count=1 -v
go test ./internal/marketing -run 'TestMemoryAtomicGrantDoesNotDoubleReserveOnDuplicateInbox|TestMemoryRepositoryRejectsDuplicateParticipationAndGrantIdempotencyKeys|TestMemoryConcurrentDuplicateGrantRequestOnlyProcessesOnce|TestMemoryConcurrentQuotaCompetitionDoesNotOverReserve|TestMemoryCreateInboxDetectsDuplicateAndConflict' -count=1 -v
go test ./internal/httpapi -run 'TestActivityEventDuplicateAndConflict|TestActivityEventBudgetExceededReturnsConflictAndAudit|TestActivityEventUserLimitExceededReturnsConflictAndAudit|TestActivityEventRequiresIdempotencyKey|TestActivityEventRejectsMismatchedIdempotencyKey|TestSimulateEventQuotaExceededSuppressesGrantPreview|TestActivityEventRejectsInvalidSchemaFields|TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent|TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown' -count=1 -v
~~~

## Result

~~~text
PASS
~~~

## Evidence

- `go test ./...`：PASS。所有 Go 包通过；`internal/postgres` 包在无 `EX_MARKETING_DATABASE_URL` 时按测试代码 skip PG 连接用例，包结果仍为 PASS。
- `go build ./...`：PASS，无编译错误。
- 规则 Schema 定向：
  - `TestCreateActivityCommandValidatesRuleSchema` PASS，覆盖非法 phase、非法/缺失 condition 字段、非法字段路径、非法 op/value、disabled invalid schema、重复 ruleCode、负 sort。
  - `TestCreateActivityCommandRejectsUnregisteredRuleEvaluatorsAfterSchemaValidation` PASS，覆盖 `FORMULA / LIMIT / SCRIPT` evaluator 未注册拒绝。
  - `TestCreateActivityRejectsInvalidConfigAndRuleSchema` PASS，覆盖 Admin 创建非法 rule schema 返回 Bad Request。
  - `TestCreateActivityValidatesFactProviderReferences` PASS，覆盖缺失 fact provider 拒绝、配置 provider 后允许。
  - `TestPrelaunchValidationBlocksMissingFactProvider` PASS，覆盖上线前校验 `FACT_PROVIDER_NOT_CONFIGURED` 阻断。
- 运行与回归：
  - `go test ./... -run 'Rule|Schema|Condition|Formula|Limit|Script|Activity'` PASS。
  - `go test ./... -run 'Idempot|Duplicate|Quota|Budget|Limit|Concurrent|Postgres|PG'` PASS。
  - `TestConditionEvaluatorStandardEventFields`、`TestConditionEvaluatorRejectsNonStandardTopLevelField`、`TestExecuteRunsFourStagesWithStableRuleDetails` PASS。
  - 内存仓储额度/幂等/并发定向用例全部 PASS。
  - HTTP API 预算不足、用户限制、幂等重复/冲突、事件 Schema、回调幂等定向用例全部 PASS。
- PostgreSQL smoke：
  - `go test ./internal/postgres -run 'TestPostgres' -count=1 -v` 输出 6 个 `TestPostgres...` 用例均 `SKIP: EX_MARKETING_DATABASE_URL is not set`，最终包 PASS。
  - 因当前环境没有真实 PG 连接串，本轮不能声明 G 类 PostgreSQL smoke 已真实落库验证。

需求：2026-05-24-rule-schema
