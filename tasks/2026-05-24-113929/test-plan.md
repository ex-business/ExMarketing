# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` 的 Decision 为 `PASS` 且本文件 Decision 为 `PASS` 后，才允许按本计划执行测试。

本计划只针对需求 `2026-05-24-113929`：活动主配置模型。测试范围以 `task.md`、`analysis.md`、`review.md` 为准，不得扩大到规则、奖励、预算库存、审批、自动上下线或外部业务系统模型。

## Test Strategy

- 优先覆盖 Admin/API 层可观察行为：活动主配置创建、查询、更新，以及错误输入返回明确 4xx/409，不出现 500。
- 覆盖模型字段持久化：`tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`version`、`configJson/config_json`。
- 覆盖时间和 timezone 校验：合法 RFC3339 带偏移、无偏移时间、非法 timezone、`startTime >= endTime`。
- 覆盖幂等/重复边界：同租户重复 `activityCode` 必须失败，不同租户相同 `activityCode` 必须成功。
- 覆盖并发：并发创建同一 `tenantId + activityCode` 只能成功一次；并发更新同一活动不能造成身份字段漂移、版本倒退或数据损坏。
- 覆盖额度不足回归：本任务不新增预算/库存/额度能力，但必须确认现有“额度不足”相关流程没有被活动主配置字段变更破坏；如果仓库已有 quota/budget/insufficient 相关测试，应执行对应测试并记录结果。
- 覆盖 PostgreSQL smoke：在真实 PostgreSQL 或项目集成测试容器上验证表结构、唯一约束、JSON、时间、version 持久化行为。

## Test Cases

### 正常路径

1. 创建最小合法活动主配置。
   - 输入：必填字段、合法 `status`、合法 RFC3339 `startTime/endTime`、合法 `timezone=UTC`，省略可默认字段。
   - 期望：创建成功；返回 `id`、`version=1`、`priority=0`、`stackable=false`、`createdAt`、`updatedAt`。

2. 创建完整活动主配置。
   - 输入：全部主配置字段，包括非空 `exclusiveGroup`、`stackable=true`、`priority`、object 类型 `configJson/config_json`。
   - 期望：创建成功；详情查询和列表查询均可完整读回字段。

3. 不同租户相同 `activityCode`。
   - 输入：`tenantA + SAME_CODE`、`tenantB + SAME_CODE`。
   - 期望：两个活动均创建成功，查询结果租户隔离。

4. 更新允许修改字段。
   - 更新：`activityName`、`status`、`startTime`、`endTime`、`timezone`、`priority`、`exclusiveGroup`、`stackable`、`configJson/config_json`。
   - 期望：更新成功；详情查询读回新值；`version` 递增；`updatedAt` 晚于或不同于原值。

5. 查询不存在活动。
   - 输入：不存在的 `id`。
   - 期望：返回 `404 Not Found` 或项目统一 Not Found 错误，不返回 500。

### 校验失败

6. 缺少必填字段。
   - 分别缺少 `tenantId`、`activityCode`、`activityName`、`activityType`、`status`、`startTime`、`endTime`、`timezone`。
   - 期望：均返回 `400 Bad Request`，错误信息能定位字段名。

7. 空白字符串字段。
   - 输入：只含空格的 `tenantId/activityCode/activityName/activityType`。
   - 期望：返回 `400 Bad Request`。

8. 非法 `status`。
   - 输入：创建或更新时传 `status=UNKNOWN`。
   - 期望：返回 `400 Bad Request`。

9. 非法时间格式。
   - 输入：非法日期、非 RFC3339 字符串。
   - 期望：返回 `400 Bad Request`，错误信息能定位时间字段。

10. 无时区偏移时间。
    - 输入：`2026-05-24T12:00:00`。
    - 期望：返回 `400 Bad Request`，明确不能接受无时区时间。

11. 时间顺序非法。
    - 输入：`startTime == endTime`、`startTime > endTime`。
    - 期望：返回 `400 Bad Request`。

12. 非法 `timezone`。
    - 输入：空字符串、`Mars/Base`、固定偏移 `+08:00`。
    - 期望：按 analysis 最小口径返回 `400 Bad Request`；如果实现明确支持固定偏移，Test Agent 必须在报告中标注与 analysis 建议的差异。

13. 非法字段类型。
    - 输入：`priority` 非整数、`stackable` 非布尔。
    - 期望：返回 `400 Bad Request`。

14. 非法 `configJson/config_json` 顶层类型。
    - 输入：数组、字符串、数字、布尔。
    - 期望：返回 `400 Bad Request`。

15. 更新身份字段。
    - 输入：更新请求中包含 `id`、`tenantId`、`activityCode`、`activityType` 的变更。
    - 期望：按 review 建议应返回 `400 Bad Request`；至少不能静默造成身份字段漂移。更新后详情查询身份字段必须保持不变。

### 幂等重复

16. 同租户重复 `activityCode`。
    - 输入：连续创建两次相同 `tenantId + activityCode`。
    - 期望：第一次成功，第二次返回 `409 Conflict` 或项目统一冲突错误；数据库中只有一条对应活动。

17. 重复更新同一 payload。
    - 输入：对同一活动连续提交两次完全相同的允许更新 payload。
    - 期望：两次请求行为必须明确且一致；若第二次成功，则 `version` 按成功更新次数递增；若实现基于乐观锁拒绝第二次，则返回 `409 Conflict`。不得出现 500、版本倒退或字段部分写入。

18. 重复请求失败后状态不污染。
    - 输入：先提交非法更新，再提交合法更新。
    - 期望：非法更新不改变原记录；合法更新成功并产生一次明确版本变化。

### 并发

19. 并发创建同一 `tenantId + activityCode`。
    - 并发量：至少 10 个并发请求。
    - 期望：最多 1 个成功，其余为 `409 Conflict` 或项目统一冲突错误；PostgreSQL 中不存在重复行。

20. 并发创建不同租户相同 `activityCode`。
    - 并发量：至少 10 个租户或 10 组请求。
    - 期望：不同租户之间不互相冲突。

21. 并发更新同一活动。
    - 并发量：至少 10 个请求，分别更新 `activityName/priority/configJson`。
    - 期望：所有成功更新均递增版本；最终记录是某个成功请求的完整结果；不得出现 JSON 破损、身份字段漂移、版本倒退、500。

22. 并发非法与合法更新混合。
    - 输入：合法更新与非法 timezone/非法 status 更新并发。
    - 期望：非法请求返回 400；合法请求可成功；最终记录只体现合法更新。

### 额度不足回归

23. 现有 quota/budget/insufficient 场景回归。
    - 前提：如果仓库已有预算、库存、额度、发奖或领取相关测试。
    - 操作：找到包含 `quota`、`budget`、`insufficient`、`limit`、`stock` 关键字的相关测试并执行。
    - 期望：已有“额度不足”行为保持原有错误码和错误信息，不因 `marketing_activity` 主配置字段新增/调整而变成成功、500 或错误类型漂移。

24. 无现有额度不足测试时的处理。
    - 操作：Test Agent 必须在测试报告中明确写出“未发现现有额度不足测试，且本需求非预算/额度范围，未新增业务测试代码”。
    - 期望：该说明不阻塞本任务，但不能漏报。

### PostgreSQL Smoke

25. PostgreSQL 表结构 smoke。
    - 检查：`marketing_activity` 存在所需列；列类型能承载字符串、时间、布尔、整数、JSON、version。
    - 期望：迁移后的 schema 与实现字段一致。

26. PostgreSQL 唯一约束 smoke。
    - 检查：同一 `tenant_id + activity_code` 唯一；不同 `tenant_id` 可重复 `activity_code`。
    - 期望：数据库约束和 API 行为一致。

27. PostgreSQL JSON smoke。
    - 输入：嵌套 object 类型 `configJson/config_json`。
    - 期望：写入成功；读回结构和类型保持一致。

28. PostgreSQL 时间 smoke。
    - 输入：不同时区偏移的 RFC3339 时间。
    - 期望：存储/读回语义稳定；比较按绝对时刻处理；`timezone` 字段保留原活动日历语义。

29. PostgreSQL version smoke。
    - 操作：创建后连续成功更新两次。
    - 期望：`version` 从 1 递增到 2、3；失败请求不递增。

## Required Commands

Test Agent 应根据项目实际模块和已有测试命名执行以下命令清单；如某命令因仓库结构不存在而无法执行，需在测试报告中写明原因和替代命令。

~~~bash
go test ./...
go build ./...
~~~

建议补充的定向命令：

~~~bash
go test ./... -run 'Activity|MarketingActivity'
go test ./... -run 'Postgres|PostgreSQL|Repository|Migration'
go test ./... -run 'Quota|Budget|Insufficient|Limit|Stock'
~~~

如项目提供 docker compose 或集成测试 profile，PostgreSQL smoke 应使用项目既有命令启动依赖，不得临时发明生产配置。示例占位如下，Test Agent 需替换为仓库实际命令：

~~~bash
# start project PostgreSQL test dependency according to repository docs
# run integration tests that exercise marketing_activity persistence
~~~

## Data / Smoke

- 测试数据使用唯一前缀：`tp-20260524-113929-<case>-<timestamp>`，避免与本地旧数据冲突。
- 至少准备两个租户：`tenant_tp_a`、`tenant_tp_b`。
- 至少准备三个合法 timezone：`UTC`、`Asia/Shanghai`、`America/New_York`。
- 每个创建成功的活动都必须通过详情查询或列表查询二次确认。
- 每个失败用例都必须确认 PostgreSQL 中没有半写入或污染原记录。
- 并发用例完成后必须查询 PostgreSQL 或 repository 层结果，确认唯一约束、版本和最终字段状态。

## Decision

~~~text
PASS
~~~

review.md 已 PASS，本测试计划覆盖正常路径、额度不足回归、幂等重复、并发和 PostgreSQL smoke，后续 Test Agent 可据此执行。

需求：2026-05-24-113929
