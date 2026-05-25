# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本测试计划供后续 Test Agent 在 Dev Agent 完成开发后执行。当前阶段不得执行测试、构建、格式化或 git 命令。

测试重点：

- 审计记录完整性：所有关键运营动作和系统动作必须写入审计，包含 `tenantId`、`actor`、`action`、`resource`、`resourceId`、`beforeSnapshot` / `afterSnapshot` 或失败上下文、`payload`、`createdAt`。
- 对账链路完整性：按活动、用户、事件、发奖单查询时，能串联 event inbox、participation、grant order、outbox、callback、quota 影响和审计时间线。
- PostgreSQL 语义：事务一致性、唯一约束、幂等、并发和租户隔离必须以 PostgreSQL smoke 为准，不能只用内存仓储验收。
- 安全边界：审计 payload 不得包含 secret、token、完整 Authorization header 或不必要敏感信息。
- 范围边界：不要求完整 RBAC、完整审批流引擎、外部钱包/支付/会员/订单系统对账。

## Test Cases

### 1. 正常路径

1. 活动创建审计
   - 准备：使用租户 `tenant-a` 和 operator `ops-a` 创建活动。
   - 期望：返回创建成功；审计记录存在，`actor=ops-a`，`action` 为稳定枚举，`resource=ACTIVITY`，`resourceId` 为活动 ID，`beforeSnapshot` 为空或 null，`afterSnapshot` 包含关键配置，`createdAt` 为服务端时间。

2. 活动配置修改审计
   - 准备：修改活动时间、规则、奖励或 quota 相关配置。
   - 期望：业务修改成功；审计记录包含修改前后快照或差异摘要；历史参与快照不因活动新版本修改而丢失解释能力。

3. 活动状态变更 / 上线审批审计
   - 准备：执行上线审批通过、审批驳回、暂停或结束中的已实现最小动作。
   - 期望：审计记录包含审批人、动作、原因、前后状态；若 API 要求 reason，缺失 reason 时应拒绝且不得产生成功状态变更。

4. Quota 创建与调整审计
   - 准备：创建 budget / stock / user limit quota，并调整额度。
   - 期望：审计记录包含 quota 类型、scope、scopeKey、调整前后值和 operator。

5. 模拟运行
   - 准备：对活动执行 simulation / dry-run。
   - 期望：产生模拟审计和结果摘要；不创建真实 event_inbox、participation、grant_order、outbox；不扣减 quota。

6. 标准事件成功处理与对账
   - 准备：上报一个满足活动规则且额度充足的事件。
   - 期望：事件处理成功；生成 participation、grant_order、outbox；quota 扣减或预留记录正确；按事件幂等键查询对账时返回完整链路和审计时间线。

7. 发奖回调成功
   - 准备：对已生成发奖单提交 SUCCESS 回调。
   - 期望：发奖单状态更新为成功；callback 结果可在发奖单对账中看到；审计记录包含系统 actor、回调状态、外部发奖 ID 和关联发奖单。

8. 租户隔离正常查询
   - 准备：在 `tenant-a` 与 `tenant-b` 创建相同或相似的 commandId / eventId 数据。
   - 期望：任一对账接口必须按 `tenantId` 隔离；`tenant-a` 查询不得返回 `tenant-b` 数据。

### 2. 额度不足

1. Budget 不足
   - 准备：设置活动 budget 小于奖励金额，提交命中事件。
   - 期望：事件返回可识别的额度不足错误或拒绝结果；审计记录包含 quotaType、scope、scopeKey、错误码、错误消息；若事务回滚 participation / grant_order / outbox，对账仍能解释无发奖链路原因。

2. Stock 不足
   - 准备：设置库存为 0 或已耗尽，提交命中事件。
   - 期望：不重复扣减库存；不生成有效发奖单；审计和对账输出可定位库存不足。

3. UserLimit 不足
   - 准备：同一用户达到参与上限后再次提交事件。
   - 期望：返回用户限制不足或规则拒绝；不产生重复发奖；审计记录包含用户维度、限制类型和关联事件。

4. 额度不足后的恢复
   - 准备：额度不足失败后，运营调整 quota，再提交新的合法事件。
   - 期望：新事件可成功处理；历史失败审计保留；新成功链路与旧失败链路可分别对账。

### 3. 幂等重复

1. 重复事件同 payload
   - 准备：使用相同标准幂等键重复提交同一事件。
   - 期望：第二次返回 duplicate 或等价幂等结果；不新增 participation、grant_order、outbox；审计或对账能定位原始事件处理结果。

2. 重复事件不同 payload
   - 准备：使用相同幂等键但修改 payload 关键字段再次提交。
   - 期望：返回幂等冲突业务错误；不覆盖原始业务事实；审计记录包含失败请求摘要且不泄露敏感字段。

3. 重复发奖回调同终态
   - 准备：对同一发奖单重复提交相同 SUCCESS 或 FAILED 回调。
   - 期望：最终状态不被重复推进；对账可识别重复回调；审计记录保留重复回调上下文。

4. 发奖回调相反终态冲突
   - 准备：先提交 SUCCESS，再提交 FAILED，或反向操作。
   - 期望：系统拒绝或标记状态冲突；最终状态不被非法覆盖；对账显示冲突原因和审计时间线。

5. DLQ close 后 replay
   - 准备：将 outbox 置入 DLQ，执行 close 后再 replay。
   - 期望：close 必须要求 operator 和 reason 并写审计；`DLQ_CLOSED` 不允许 replay；失败 replay 尝试也应有可排查记录或明确业务错误。

### 4. 并发

1. 并发重复事件
   - 准备：使用同一幂等键并发提交多次命中事件。
   - 期望：最多一条 participation、grant_order、outbox 生效；其余请求返回 duplicate / conflict；无重复 quota 扣减；审计记录数量和状态可解释。

2. 并发额度扣减
   - 准备：设置 quota 只够 N 次发奖，并发提交大于 N 个合法事件。
   - 期望：成功数量不超过额度；失败请求返回额度不足或冲突重试后的明确结果；PostgreSQL 中 quota、participation、grant_order、outbox 总量一致。

3. 并发 quota 调整与事件处理
   - 准备：事件处理并发进行时执行 quota 调整。
   - 期望：不存在负数额度、重复冻结、丢失释放；每个成功/失败动作均有审计；对账能解释处理时采用的活动版本和 quota 状态。

4. 并发 DLQ replay
   - 准备：对同一 DLQ outbox 并发 replay。
   - 期望：最多一次 replay 生效；其他请求返回重复处理或状态冲突；审计记录包含 operator、reason 和最终 outbox 状态。

### 5. PostgreSQL Smoke

1. PostgreSQL profile 启动后的基础迁移 / 表结构检查
   - 期望：审计表、对账涉及表、唯一键、租户索引、幂等键索引存在；JSON / snapshot 字段类型满足实现约定。

2. PostgreSQL 正常链路 smoke
   - 步骤：创建活动、创建 quota、上线或启用活动、提交成功事件、提交发奖成功回调、按事件和发奖单查询对账。
   - 期望：链路完整；审计时间线按时间排序；重启应用后数据仍可查询。

3. PostgreSQL 事务一致性 smoke
   - 步骤：制造审计写入失败或违反审计约束的场景。
   - 期望：关键业务动作不得对外成功但缺少审计；若实现选择强一致，应整体失败；若实现选择失败中止，应返回明确错误。

4. PostgreSQL 租户隔离 smoke
   - 步骤：两个租户写入相同 commandId / eventId / subjectId。
   - 期望：查询必须带 tenantId；跨租户不得串数据；唯一约束以 tenantId 维度生效。

5. PostgreSQL 并发 smoke
   - 步骤：并发提交重复事件和额度竞争事件。
   - 期望：数据库最终一致，无重复发奖、无负额度、无孤儿 outbox / grant_order；失败请求可通过审计和对账解释。

6. PostgreSQL 敏感字段 smoke
   - 步骤：请求中携带 Authorization、token、secret 或等价字段。
   - 期望：审计 payload、snapshot、对账响应中不包含完整敏感值。

## Required Commands

后续 Test Agent 应在 Dev Agent 完成实现后执行以下命令；本 Test Plan Agent 当前不得执行。

~~~bash
go test ./...
go build ./...
~~~

如项目提供 PostgreSQL 集成测试 profile、compose 文件或 Make 目标，Test Agent 还应执行对应 PostgreSQL smoke 命令。命令名称以仓库实际实现为准，例如：

~~~bash
# 示例：仅在仓库实际提供时执行
go test ./... -tags=integration
go test ./... -run 'Postgres|Audit|Reconcile|Idempot|Quota|DLQ' -count=1
~~~

## Data / Smoke

建议后续 Test Agent 准备以下最小数据集：

- 租户：`tenant-a`、`tenant-b`。
- 操作者：`ops-a`、`approver-a`、`support-a`、系统 actor `system`。
- 活动：一个普通奖励活动，一个 dry-run 活动，一个低额度活动。
- Quota：budget、stock、user limit 各至少一组，包含充足和不足两类。
- 用户：`user-a`、`user-b`，其中 `user-a` 用于达到上限和重复事件。
- 事件：成功事件、规则拒绝事件、重复同 payload 事件、重复不同 payload 事件、额度不足事件。
- 发奖单：成功发奖单、失败进入 DLQ 发奖单、`DLQ_CLOSED` 发奖单。
- 回调：SUCCESS、FAILED、重复同终态、相反终态冲突、缺少 reservation 或 externalGrantId 冲突。

验收 smoke 查询至少覆盖：

- 按 `tenantId + activityId/activityCode` 查询活动对账。
- 按 `tenantId + subjectType + subjectId` 查询用户对账。
- 按 `tenantId + eventId` 或标准幂等键查询事件对账。
- 按 `tenantId + commandId/billNo` 或 `grantOrderId` 查询发奖单对账。
- 按时间范围、状态过滤、分页查询审计和对账结果。

## Decision

~~~text
PASS
~~~

理由：`review.md` 已为 PASS，当前测试计划覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke、租户隔离、敏感字段过滤、DLQ 人工处理和审计事务一致性；可作为 Dev 后 Test Agent 的执行依据。

需求：2026-05-24-113400-10
