# Test Plan

## Agent 指令

你是 Test Plan Agent。Review 已 PASS；本文件只定义后续 Test Agent 应执行的测试用例和命令清单。

只写 `tasks/2026-05-24-113929-8/test-plan.md`，不改业务代码，不执行测试、构建、格式化或 git 命令。

## Test Strategy

- 先覆盖领域层/Repository 层审批状态机：提交、一级审批、二级审批、驳回、撤回、版本锁定、重复请求、并发最终态。
- 再覆盖 HTTP Admin API：请求校验、错误码/错误原因稳定性、operator/reason/expectedVersion 必填、tenant 隔离、审批记录查询和审计查询。
- 再覆盖事件处理链路：未审批、审批版本不一致、已审批但 `SCHEDULED`、已审批且 `ACTIVE` 四类活动的匹配、quota、participation、grant_order、outbox 行为。
- 最后覆盖 PostgreSQL 集成 smoke：审批记录、审计记录、活动版本、事件幂等和并发审批在真实数据库约束下保持一致。

测试数据应使用唯一前缀，例如 `approval_20260524_113929_8_`，避免污染既有测试。所有断言必须同时检查返回结果和副作用，尤其是 participation、grant_order、outbox、quota、audit 的有无变化。

## Test Cases

### 1. 正常路径：普通活动一级审批后上线并处理事件

- 创建普通活动：非资金类奖励、无 budget/stock quota、无外部 AUTO 发奖、状态先为 `DRAFT`。
- 直接尝试更新为 `SCHEDULED` 或 `ACTIVE`：应失败，错误原因明确为未审批或审批门禁拒绝，并写入上线阻断审计。
- 使用正确 `operator=submitter_a` 和 `expectedVersion=当前版本` 提交审批：状态进入 `PENDING_L1`，锁定版本等于活动当前版本。
- 使用不同 `operator=approver_l1_a` 一级通过：审批状态进入 `APPROVED`。
- 将同版本活动更新为 `ACTIVE`：应成功，并写入上线成功审计。
- 提交匹配事件：应命中活动并保持既有规则、奖励、quota、participation、grant_order、outbox 行为不回退。
- 查询审批记录和审计：应能看到提交、一级通过、上线成功完整时间线。

### 2. 正常路径：高危活动二级审批后上线

- 创建高危活动，至少覆盖一种高危触发：`rewardType=BONUS`、`grantMode=AUTO`、`grantChannel=HTTP`，或配置 `BUDGET` / `STOCK` quota。
- 提交审批后应进入 `PENDING_L1`，风险等级识别为高危或需要二级审批。
- 一级审批通过后应进入 `PENDING_L2`，此时尝试更新为 `SCHEDULED` 或 `ACTIVE` 必须失败。
- 使用与提交人、一级审批人均不同的 `operator=approver_l2_a` 二级通过：审批状态进入 `APPROVED`。
- 将同版本活动更新为 `ACTIVE` 后提交匹配事件：应正常生成 participation、grant_order、outbox，并按既有 quota 原子扣减逻辑执行。
- 审批记录应包含 L1、L2 审批人、原因、锁定版本、风险等级。

### 3. 额度不足：审批通过不能绕过 quota

- 创建高危 `ACTIVE` 活动，完成二级审批，并配置 budget 或 stock quota 为不足值。
- 提交会触发奖励的事件：应返回或记录 quota exceeded 类拒绝，不得因为审批通过而绕过额度校验。
- 断言 participation、grant_order、outbox 不应部分写入；quota used/frozen 不应出现非原子残留。
- 查询审计：应包含事件进入和 quota 拒绝相关审计；审批审计仍保持可追溯。

### 4. 额度不足：未审批活动不得先扣 quota

- 创建配置 quota 的未审批活动，即使状态被请求更新为 `ACTIVE` 也应被审批门禁拒绝。
- 提交匹配事件或尝试通过 API 触发正式事件处理：不得创建 participation、grant_order、outbox，不得扣减 quota。
- 拒绝原因应优先体现审批门禁或活动不可上线，不应表现为已经进入奖励/额度处理后的失败。

### 5. 幂等重复：重复提交审批

- 同一活动、同一 `expectedVersion`、同一提交人在 `PENDING_L1` 状态重复提交审批。
- 期望不得创建多个并行审批单；应幂等返回当前审批单，或返回明确的已有进行中审批错误。
- 查询审批时间线：只能有一个进行中的审批流程，不能出现重复 submit 动作导致的状态混乱。

### 6. 幂等重复：重复审批通过

- 普通活动一级审批通过后，使用相同审批人和相同动作再次提交一级通过。
- 期望幂等返回 `APPROVED`，不得重复推进状态、不得重复写入会误导审计的关键状态变化。
- 高危活动二级审批通过后重复二级通过也应保持同样幂等语义。

### 7. 幂等重复：事件重复提交仍保持既有幂等

- 对已审批且 `ACTIVE` 的活动提交合法事件，记录 participation、grant_order、outbox、quota 变化。
- 使用同一 `Idempotency-Key` 和相同 payload 重复提交：应返回 duplicate 语义，不得新增 participation、grant_order、outbox，不得重复扣 quota。
- 使用同一 `Idempotency-Key` 但不同 payload：应返回 idempotency conflict，不得产生新的业务副作用。

### 8. 版本锁定：审批中修改活动

- 活动 version=1 时提交审批并锁定 version=1。
- 审批中修改活动主配置、规则、奖励或 quota，使活动 version 变为 2。
- 对旧审批单执行通过：必须失败，错误原因明确为版本不一致或配置已变化。
- 使用旧审批尝试上线 version=2：必须失败。
- 重新提交 version=2 并完成审批后，同版本上线才可成功。

### 9. 驳回和原因

- 对普通活动提交审批后，一级审批驳回且 `reason` 为空：必须失败。
- 使用非空 `reason` 驳回：状态进入 `REJECTED`，活动不能上线。
- 查询审批记录：最近一次驳回原因和历史驳回动作都可见。
- 驳回后修改活动产生新版本，重新提交应创建新的审批轮次或清晰的新流程，不得复用旧 `REJECTED` 结果上线。

### 10. 人员约束

- 提交人使用同一 `operator` 执行一级通过：必须失败。
- 高危活动中，二级审批人与提交人相同：必须失败。
- 高危活动中，二级审批人与一级审批人相同：必须失败。
- `operator` 为空提交、审批、驳回：必须失败。

### 11. SCHEDULED 事件语义

- 创建活动并完成审批，将活动状态更新为 `SCHEDULED`。
- 提交本应匹配的正式事件：必须按非 `ACTIVE` 语义拒绝，例如 `ACTIVITY_NOT_ACTIVE`。
- 断言不会生成 participation、grant_order、outbox，不会扣减 quota。
- 模拟运行如已有显式预演能力，不应因本需求被额外封死；默认正式事件链路必须保持 `ACTIVE` only。

### 12. 未审批和版本不一致的 ACTIVE 防护

- 构造未审批但状态为 `ACTIVE` 的活动，或通过 repository/fixture 模拟状态被错误设置为 `ACTIVE`。
- 提交正式事件：必须被审批门禁阻断，不得命中活动。
- 构造已审批 version=1 但当前活动 version=2 且 `ACTIVE` 的活动。
- 提交正式事件：必须因审批版本不一致被阻断，不得进入奖励链路。

### 13. 并发审批最终态

- 同一普通审批单并发执行两个一级通过请求：最多一个请求实际完成状态变更，最终状态为 `APPROVED`，不得出现重复审批单或不一致审计。
- 同一审批单并发执行通过和驳回：只能有一个最终状态成功；另一个请求应返回冲突或最新状态。
- 高危审批单在 `PENDING_L2` 并发执行二级通过和驳回，同样只能产生一个最终状态。
- 并发测试需在内存 repository 和 PostgreSQL repository 各覆盖一次；PostgreSQL 版本必须验证事务/唯一约束没有产生重复记录。

### 14. 租户隔离和查询

- tenant A 与 tenant B 使用相同 activityCode 或相似 activityId 前缀创建活动和审批。
- tenant A 查询 tenant B 的审批记录或执行审批动作：必须失败或返回空结果，不得跨租户读写。
- 审计查询也必须按 tenant 隔离。

### 15. PostgreSQL smoke

- 使用真实 PostgreSQL 启动服务或 repository integration test。
- 执行普通活动完整审批、上线、事件处理一次，验证数据库中审批记录、审计记录、activity version/status、participation、grant_order、outbox 一致。
- 执行高危活动 L1 后未 L2 的上线阻断一次，验证阻断审计落库。
- 执行并发审批通过/驳回一次，验证最终只有一个终态，审批动作记录没有违反唯一约束或事务一致性。
- 执行事件幂等重复一次，验证 PostgreSQL 下不会重复写 participation、grant_order、outbox 或 quota 扣减。

## Required Commands

后续 Test Agent 在开发完成后执行以下命令。若 `EX_MARKETING_DATABASE_URL` 未设置，PostgreSQL 集成测试会跳过；必须额外执行 PostgreSQL smoke 命令以覆盖真实数据库。

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres|Approval|Activity' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Approval|Activity|Event|Idempot|Quota|Concurrent|Postgres' -count=1
~~~

如开发新增了更精确的审批测试文件或测试名，Test Agent 应优先执行对应包的精确命令，例如：

~~~bash
go test ./internal/marketing -run 'Approval|ActivityApproval|ConcurrentApproval|IdempotentApproval' -count=1
go test ./internal/httpapi -run 'Approval|ActivityApproval|Audit|Scheduled|Idempotency' -count=1
go test ./internal/engine -run 'Approval|Scheduled|Quota|Idempotency|Event' -count=1
~~~

## Data / Smoke

- 普通活动样例：非资金类、无外部自动发奖、无 quota，事件类型可沿用 `DEPOSIT_SUCCESS`，状态从 `DRAFT` 开始。
- 高危活动样例：`rewardType=BONUS`、`grantMode=AUTO`、`grantChannel=HTTP`、`targetRef=https://reward.example.com`，并配置 budget 或 stock quota。
- 操作人样例：`submitter_a`、`approver_l1_a`、`approver_l2_a`、`submitter_b`，用于验证同人审批拒绝和租户隔离。
- 版本样例：提交审批前记录 `expectedVersion`；修改任一关键配置后必须断言 version 增加，并验证旧审批失效。
- PostgreSQL smoke 清理：测试数据 ID、activityCode、idempotency key 使用统一前缀 `approval_20260524_113929_8_`，测试前后按此前缀清理。
- 关键副作用断言：审批失败、上线阻断、`SCHEDULED` 事件拒绝、quota 不足、idempotency conflict 场景下，participation、grant_order、outbox、quota used/frozen 均不得出现不一致残留。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-113929-8
