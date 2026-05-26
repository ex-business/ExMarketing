# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

本文件仅给后续 Test Agent 的测试用例与命令清单。本轮未执行任何测试、构建、格式化或 git 命令。

## Test Strategy

Review Decision 为 `PASS`，可进入测试设计。

测试目标：验证上线前配置校验在活动进入 `SCHEDULED` / `ACTIVE` 前能返回逐项 `PASS` / `WARN` / `BLOCK`，并用整体 `decision` 与 `allowPublish` 正确阻断风险配置。

测试层级：

- 单元测试：覆盖校验器的纯规则判断，包括 Schema、规则字段、概率、预算、库存、时间冲突、互斥、高危、版本失效和多错误聚合。
- 集成测试：覆盖真实上线入口或独立预检接口。若 Dev 新增独立预检接口，也必须额外覆盖状态变更到 `SCHEDULED` / `ACTIVE` 的真实调用链，防止只测到旁路接口。
- PostgreSQL smoke：使用真实 PostgreSQL 测试库验证活动、规则、奖励、quota、互斥相关数据读写与校验报告落库或返回结构，不要求连接外部钱包、券、积分系统。
- 并发测试：覆盖同一活动重复预检、重复上线请求、并发上线互斥活动时的幂等与一致性。

重点断言：

- 任一 `items[].result=BLOCK` 时，`decision=BLOCK` 且 `allowPublish=false`。
- 仅存在 `WARN` 且无 `BLOCK` 时，`decision=PASS` 且 `allowPublish=true`。
- 高危配置必须返回 `HIGH_RISK_*` 阻断项，不允许上线。
- 同时存在多个独立错误时，`items[]` 必须尽量返回全部可判断问题，不能只返回第一个错误。
- 金额、预算、库存、概率权重必须使用 Decimal 语义，不能出现浮点误差导致 `100.0000` 等值误判。

## Test Cases

### A. 正常路径

1. 合法低风险活动上线前校验通过。
   - 数据：同租户无冲突活动；`startTime < endTime`；合法 timezone；合法 `configJson`；`CONDITION` 规则字段只引用标准事件字段；`FIXED` 非资金或低风险 reward；必要 quota 完整。
   - 断言：返回 Schema、规则字段、奖励、预算、库存、时间、互斥、高危等逐项结果；所有必需项 `PASS`；`decision=PASS`；`allowPublish=true`。

2. 状态变更真实入口通过。
   - 数据：沿用合法低风险活动。
   - 操作：通过创建为 `ACTIVE`、或从 `DRAFT` 更新为 `SCHEDULED` / `ACTIVE` 的真实入口触发校验。
   - 断言：状态变更成功；响应或审计中能关联本次校验结果；没有绕过上线前校验。

3. 普通无限量金额类 reward 缺少库存 quota。
   - 数据：金额类 reward 有有效预算 quota，但无库存 quota，且不属于库存型权益。
   - 断言：库存项最多返回 `WARN`；无 `BLOCK`；`decision=PASS`；`allowPublish=true`。

4. WARN 不影响上线判断。
   - 数据：构造一个只产生非阻断 `WARN` 的配置，例如可叠加活动存在时间重叠但业务允许，或普通金额类无库存 quota。
   - 断言：`summary.warnCount > 0`、`summary.blockCount=0`、`decision=PASS`、`allowPublish=true`。

5. 概率奖励合法。
   - 数据：`calculateType=PROBABILITY`，enabled items 非空，`itemCode` 唯一，权重为 Decimal 且合计 `100.0000`，空奖显式配置。
   - 断言：概率项 `PASS`，不因 Decimal 精度误判。

### B. Schema 与规则字段阻断

6. JSON Schema 基础字段非法。
   - 数据：分别构造 `startTime >= endTime`、非法 timezone、`configJson` 顶层未知字段。
   - 断言：返回对应 Schema `BLOCK`，定位 `field`，整体 `BLOCK`。

7. targetRef 非法。
   - 数据：`AUTO + HTTP/WEBHOOK` 缺少 `targetRef`、非 `http(s)` URL；`AUTO + MQ` targetRef 含 broker 地址、账号密码、query 或 fragment。
   - 断言：返回 Schema 或高危 `BLOCK`；不得上线。

8. 规则 phase/type/op/conditions 非法。
   - 数据：分别构造 `rulePhase=BUDGET`、未知 `ruleType`、非法 `op`、空 `conditions`、`IN` 值不是非空数组。
   - 断言：返回规则字段 `BLOCK`，字段路径准确。

9. 规则字段路径非法或依赖缺失。
   - 数据：引用 `orderAmount`、`deposit.amount`、`user.vipLevel` 等非标准顶层字段；引用未 enabled 的 `facts.<factCode>.*`；引用未绑定或未注册插件的 `state.plugin.<pluginCode>.*`。
   - 断言：返回规则字段 `BLOCK`；同租户 fact/plugin 注册正确时相同字段可通过。

10. disabled rule 仍做配置合法性校验。
    - 数据：`enabled=false` 的 rule 使用非法 op 或非法字段路径。
    - 断言：仍返回 `BLOCK`，不能因 disabled 被跳过配置校验。

### C. 概率、预算、库存、额度不足

11. 概率总和不等于 `100.0000`。
    - 数据：权重合计 `99.9999`、`100.0001`、或明显不足。
    - 断言：返回 `PROBABILITY_TOTAL_INVALID`，该项 `BLOCK`，整体 `BLOCK`。

12. 概率 item 明细非法。
    - 数据：items 为空、负权重、重复 `itemCode`、所有权重为 0、非空奖 item 缺少实际奖励字段。
    - 断言：返回 `PROBABILITY_ITEMS_EMPTY`、`PROBABILITY_WEIGHT_INVALID`、`PROBABILITY_ITEM_CODE_DUPLICATED`、`PROBABILITY_ITEM_REWARD_INVALID` 等稳定 code。

13. 预算额度不足。
    - 数据：金额类 reward 单次最大暴露为 `100.0000`；`BUDGET + ACTIVITY + TOTAL` 的 `totalValue - usedValue - frozenValue = 99.9999`。
    - 断言：返回预算不足 `BLOCK`；`expected` 包含单次最大暴露，`actual` 包含可用预算；整体 `BLOCK`。

14. 自动资金类缺少预算 quota。
    - 数据：`AUTO` 发放 `CASH` 或 `BONUS`，未配置 `BUDGET + ACTIVITY + TOTAL`。
    - 断言：返回 `HIGH_RISK_MISSING_BUDGET` 或等价稳定高危 code；整体 `BLOCK`。

15. 最大暴露无法确定。
    - 数据：`RATIO` 缺少 `maxAmount`；`POOL_SPLIT` 缺少 `poolAmount`；`LADDER` 阶梯重叠或无法解释上限。
    - 断言：返回预算或高危 `BLOCK`；不得上线。

16. 高危阈值阻断。
    - 数据：单次最大奖励超过默认 `1000.0000`，或活动总预算超过默认 `100000.0000`。
    - 断言：返回 `HIGH_RISK_*`；整体 `BLOCK`；不能被预算充足条件放行。

17. 库存额度不足或缺失。
    - 数据：库存型 reward 缺少 `STOCK + ACTIVITY + rewardCode`；可用库存为 0；`totalValue - usedValue - frozenValue < 0`。
    - 断言：返回库存 `BLOCK`；字段定位到对应 reward 或 quota。

18. disabled reward 不计入暴露但仍校验结构。
    - 数据：一个 enabled 低风险 reward；另一个 disabled reward 配置金额很高但结构合法。
    - 断言：预算/高危暴露只统计 enabled reward；若 disabled reward 结构非法则仍返回 Schema 或 reward 配置 `BLOCK`。

### D. 时间冲突、互斥、多租户

19. 同租户时间冲突阻断。
    - 数据：当前活动与已有 `SCHEDULED` / `ACTIVE` 活动时间窗口重叠，`triggerEvents` 有交集，同入口或入口未限制，互斥组相同且不可叠加，优先级相同或无稳定裁剪。
    - 断言：返回时间或互斥冲突 `BLOCK`，`relatedResourceIds` 包含已有活动 ID。

20. 首尾相接不算重叠。
    - 数据：已有活动 `endTime` 等于当前活动 `startTime`，其他条件相同。
    - 断言：不返回时间重叠 `BLOCK`。

21. 不同租户隔离。
    - 数据：不同 `tenantId` 的活动时间、事件、互斥组完全冲突。
    - 断言：不产生时间或互斥 `BLOCK`。

22. triggerEvents 无交集不阻断。
    - 数据：同租户同时间同互斥组，但事件集合无交集。
    - 断言：不返回时间或互斥 `BLOCK`。

23. stackable=true 且规则允许叠加。
    - 数据：同租户时间和事件重叠，双方明确可叠加。
    - 断言：不阻断；可返回 `WARN`；整体无其他错误时 `PASS`。

### E. 幂等重复与版本有效性

24. 重复预检幂等。
    - 数据：同一活动同一版本连续调用上线前校验。
    - 断言：除 `checkedAt`、trace id 等运行时字段外，`decision`、`allowPublish`、`items[].code/result/field`、`summary` 保持一致；不重复创建脏数据。

25. 重复上线请求幂等。
    - 数据：合法活动从 `DRAFT` 发布到 `SCHEDULED` / `ACTIVE` 后，使用相同 idempotency key 或等价业务幂等键重复请求。
    - 断言：第二次请求不重复副作用；返回同一最终状态或明确幂等响应；不会重复写入不一致校验报告。

26. 配置版本变化后旧报告失效。
    - 数据：版本 `v1` 校验通过；随后修改 reward、budget、stock、互斥组或时间窗口生成 `v2`。
    - 操作：尝试用 `v1` 报告或旧通过状态发布 `v2`。
    - 断言：必须重新校验；旧报告不能放行。

27. 多错误聚合。
    - 数据：同一活动同时存在概率总和错误、预算不足、非法规则字段和库存不足。
    - 断言：`items[]` 至少包含这些独立错误；`summary.blockCount` 与返回的 `BLOCK` 项一致。

### F. 并发

28. 同一活动并发预检。
    - 数据：同一活动同一版本，并发发起多次预检。
    - 断言：所有响应的核心结果一致；无死锁；无重复或损坏的报告记录。

29. 同一活动并发上线。
    - 数据：合法活动，并发请求从 `DRAFT` 发布到 `SCHEDULED` / `ACTIVE`。
    - 断言：最多一次状态迁移成功；其他请求返回幂等成功或明确冲突；最终活动状态一致。

30. 两个互斥活动并发上线。
    - 数据：同租户、同互斥组、时间和事件重叠、不可叠加的两个 `DRAFT` 活动。
    - 操作：并发发布两个活动。
    - 断言：不能两个都成功上线；至少一个返回互斥或并发冲突阻断；最终 PostgreSQL 中不存在两个同时可上线的冲突活动。

31. 并发下预算/库存快照一致。
    - 数据：校验过程中并发更新相关 quota 或活动版本。
    - 断言：要么基于一致快照返回结果，要么检测版本变化并要求重试；不能用混合旧新数据放行。

### G. PostgreSQL Smoke

32. PostgreSQL 基础持久化 smoke。
    - 准备：使用测试库迁移后的真实表结构插入活动、规则、奖励、quota、已有活动冲突数据。
    - 断言：校验服务能从 PostgreSQL 读取完整配置并返回结构化报告；字段 Decimal 读写无精度损失。

33. PostgreSQL 状态变更阻断 smoke。
    - 数据：预算不足或高危活动。
    - 操作：通过真实发布入口尝试变更为 `SCHEDULED` / `ACTIVE`。
    - 断言：事务回滚或保持原状态；数据库中活动状态未变为上线态；返回报告包含阻断项。

34. PostgreSQL 多租户 smoke。
    - 数据：两个租户存在相同 `activityCode`、`exclusiveGroup`、时间窗口和事件配置。
    - 断言：查询和冲突判断限定 tenant；不会跨租户误阻断。

35. PostgreSQL 幂等 smoke。
    - 数据：同一活动、同一版本、相同幂等键重复发布。
    - 断言：幂等记录、活动状态、校验报告之间一致；不会出现重复上线副作用。

## Required Commands

~~~bash
# 后续 Test Agent 按项目实际技术栈选择并执行；本 Test Plan Agent 不执行。

# 1. 先定位项目可用测试命令和模块结构
find . -maxdepth 3 -type f \( -name 'go.mod' -o -name 'pom.xml' -o -name 'build.gradle' -o -name 'package.json' \) -print

# 2. Go 项目候选命令
go test ./...
go test -race ./...
go build ./...

# 3. Java/Maven 项目候选命令
mvn test
mvn -DskipTests package

# 4. Node/TypeScript 项目候选命令
npm test
npm run test
npm run build

# 5. PostgreSQL smoke 候选命令
# 按项目已有脚本、环境变量和测试容器配置替换 DATABASE_URL / TEST_DATABASE_URL。
TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Postgres|PostgreSQL|Smoke|PublishValidation'
TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/exmarketing_test?sslmode=disable' mvn test -Dtest='*Postgres*,*PostgreSQL*,*Smoke*,*PublishValidation*'
TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/exmarketing_test?sslmode=disable' npm test -- --runInBand
~~~

## Data / Smoke

后续 Test Agent 应准备以下最小测试数据集：

- `tenant_a`、`tenant_b` 两个租户，用于多租户隔离。
- `campaign_low_risk_valid`：合法低风险活动，作为正常路径基线。
- `campaign_probability_invalid`：概率 item 总和错误、重复 itemCode、负权重、空奖未显式配置的变体。
- `campaign_budget_insufficient`：单次最大暴露大于可用预算。
- `campaign_high_risk_missing_budget`：自动资金类 reward 缺少预算 quota。
- `campaign_stock_insufficient`：库存型 reward 缺少库存 quota、库存为 0、库存为负的变体。
- `campaign_schema_invalid`：非法时间、timezone、targetRef、config 顶层字段。
- `campaign_rule_invalid`：非法 phase/type/op/conditions/field path、缺失 fact provider、缺失 plugin registry。
- `campaign_conflict_existing_active`：已有 `ACTIVE` 或 `SCHEDULED` 活动，用于时间与互斥冲突。
- `campaign_warn_only`：只产生 `WARN` 不产生 `BLOCK`，用于验证 `allowPublish=true`。

PostgreSQL smoke 数据要求：

- 所有金额、预算、库存和概率权重字段使用能暴露浮点误差的 Decimal 样例，例如 `0.1000`、`99.9999`、`100.0000`、`100.0001`。
- quota 至少覆盖 `BUDGET + ACTIVITY + TOTAL`、`STOCK + ACTIVITY + rewardCode`、缺失 quota、负数 quota、`usedValue + frozenValue > totalValue`。
- 已有活动至少覆盖同租户冲突、不同租户不冲突、首尾相接不冲突、事件集合无交集不冲突。
- 幂等测试必须包含相同请求重复提交和不同请求并发提交两类。

## Decision

~~~text
PASS
~~~

理由：Review 已 PASS；测试计划覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，并补齐 WARN 不阻断、真实上线入口、高危阈值、多错误聚合和多租户隔离等 Review 风险点。

需求：2026-05-24-113929-6
