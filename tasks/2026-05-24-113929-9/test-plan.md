# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

- Review 结论已确认：`tasks/2026-05-24-113929-9/review.md` Decision 为 `PASS`。
- 本计划只覆盖“操作审计记录”本期范围：审计写入、必要 payload、查询过滤、事务一致性和 PostgreSQL 持久化 smoke；不得扩展成完整对账接口或跨表对账能力。
- 测试分层：
  - 单元测试：覆盖审计 payload 构造、action/resource 枚举、脱敏 source、operator/tenantId 校验、limit 上限。
  - HTTP handler / service 测试：覆盖人工动作成功路径、失败路径、查询过滤和返回顺序。
  - Repository 集成测试：覆盖 PostgreSQL 模式下 `marketing_audit` 持久化、JSON payload、过滤字段和倒序分页。
  - 并发测试：覆盖 quota 额度不足/并发扣减冲突只产生可追踪失败审计且不重复扣减。
- 后续 Test Agent 必须验证失败类审计仍可通过 `resourceId` 或稳定关联 ID 定位，例如 `activityId`、`eventId`、`eventInboxId`、`grantOrderId`、`commandId`、`outboxId`；不得只记录自由文本错误。
- 后续 Test Agent 必须检查审计 payload 不包含完整 `Authorization`、Bearer token、Cookie、secret、Fact Provider 明文密钥、MQ 凭据或完整敏感外部响应。

## Test Cases

### 1. 正常路径：活动配置修改审计

- 前置：创建一个 tenant 下的活动，准备 operator、requestId/traceId、HTTP method/path、可脱敏 IP/userAgent。
- 操作：调用活动配置修改接口，修改状态、版本、时间、timezone、priority、互斥叠加、config/rule/reward 摘要中的至少一项。
- 期望：
  - 业务修改成功。
  - 可查询到一条 `UPDATE_ACTIVITY` 审计。
  - 审计包含 `tenantId`、`actor=operator`、`resource=ACTIVITY`、`resourceId=activityId`、`createdAt`、payload.source。
  - payload 包含 `beforeSnapshot` 和 `afterSnapshot`，且能反映修改前后版本/状态/关键配置摘要。
  - payload.source 只保留来源摘要，不包含完整敏感 header。

### 2. 正常路径：上线审批提交/通过/驳回审计

- 前置：准备待审批活动和审批锁定版本。
- 操作：
  - 提交上线审批。
  - 审批通过。
  - 对另一条审批流程执行驳回并填写 reason。
- 期望：
  - 分别生成 `APPROVAL_SUBMIT`、`APPROVAL_APPROVE`、`APPROVAL_REJECT`。
  - `resource=APPROVAL`，`resourceId` 可稳定定位审批对象或活动对象。
  - payload 包含审批前状态、审批后状态、锁定版本、审批人、reason、source。
  - 驳回缺少 reason 时返回 400 类错误，不写入成功审计。

### 3. 正常路径：模拟运行审计且不产生真实业务副作用

- 前置：准备活动、规则、奖励和一条模拟事件输入。
- 操作：调用模拟运行接口。
- 期望：
  - 返回模拟结果。
  - 生成 `SIMULATION_RUN`，`resource=SIMULATION`。
  - payload 包含输入事件摘要、候选/命中/未命中数量、是否存在可发奖结果、quota 风险摘要。
  - 不写真实 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`，不扣减 quota。
  - payload 不记录完整敏感 attributes 或 provider 响应。

### 4. 额度不足：quota 扣减失败审计

- 前置：配置预算、库存、用户次数或每日次数中至少一种额度不足场景。
- 操作：触发事件处理或奖励发放流程，使 quota 扣减失败。
- 期望：
  - 业务返回可识别的额度不足/扣减失败错误。
  - 生成 `QUOTA_EXCEEDED`，`resource=QUOTA`。
  - payload 包含 `activityId`、`eventInboxId` 或 `eventId`、`subject`、`biz`、`quotaType`、`scope`、`scopeKey`、`amount`、`reason`、`error`。
  - 不生成 grant_order/outbox，不重复扣减 quota。
  - 即使业务事务回滚，`QUOTA_EXCEEDED` 审计仍可查询。

### 5. 发奖失败 / 回调失败审计

- 前置：准备 grant order 或 command，并模拟 provider 回调失败、worker 失败或发奖单失败终结。
- 操作：触发失败处理路径。
- 期望：
  - 生成 `GRANT_FAILED`，`resource=GRANT_ORDER`。
  - payload 包含 `grantOrderId` 或 `commandId`、失败状态、错误码/消息摘要、外部回调状态或 worker 错误摘要。
  - 失败类审计允许无 `afterSnapshot`，但必须有错误上下文和稳定关联 ID。

### 6. Outbox 投递失败、进入 DLQ、人工 replay/close 审计

- 前置：准备 outbox 记录并模拟投递失败、重试耗尽、DLQ 人工操作。
- 操作：
  - 触发投递失败达到记录节点。
  - 触发进入 DLQ。
  - 调用 DLQ replay，传入 operator 和 reason。
  - 调用 DLQ close，传入 operator 和 reason。
- 期望：
  - 生成 `OUTBOX_DELIVERY_FAILED` 或对应失败摘要审计。
  - 重试耗尽生成 `OUTBOX_ENTER_DLQ`。
  - replay/close 分别生成 `OUTBOX_DLQ_REPLAY`、`OUTBOX_DLQ_CLOSE`。
  - payload 包含 `outboxId`、原状态、新状态、operator、reason、targetChannel、targetRef 摘要、retryCount。
  - DLQ replay/close 缺少 operator 或 reason 时返回 400 类错误，不产生 actor 为空的成功审计。
  - close 后再次 replay 返回明确错误，不得写成成功 replay。

### 7. 幂等重复：重复事件 / 重复命令 / 重复人工请求

- 前置：准备相同 `eventId`、`commandId` 或同一 DLQ 操作请求的重复调用数据。
- 操作：
  - 对同一事件重复投递。
  - 对同一发奖 command 重复处理失败回调。
  - 对同一 outboxId 重复 replay 或 close。
- 期望：
  - 业务幂等结果保持一致，不产生重复扣减、重复发奖或重复状态迁移。
  - 审计策略清晰且可验收：若实现选择记录重复尝试，payload 必须标记 duplicate/idempotent；若实现选择不重复写审计，必须能通过已有记录定位原始 action。
  - 不允许出现多条完全相同且无法区分的成功审计导致误判。
  - 查询按 `commandId`、`eventId`、`resourceId` 能定位重复处理相关记录。

### 8. 并发：quota 竞争和审计一致性

- 前置：设置剩余额度只允许少量请求成功，例如库存/预算仅够 1 次成功。
- 操作：并发发起多笔相同 tenant/activity/subject 的事件处理或扣减请求。
- 期望：
  - 成功数量不超过额度。
  - 失败请求返回可识别的额度不足或并发冲突错误。
  - 每个失败业务尝试至少有可定位的 `QUOTA_EXCEEDED` 审计，包含稳定关联 ID。
  - 不生成超额 grant_order/outbox，不出现负 quota。
  - 成功业务的审计和业务状态一致；若关键业务变更成功，审计不可缺失。

### 9. 审计查询过滤、排序和租户隔离

- 前置：写入多个 tenant、actor、action、resource、resourceId、activityId、eventId、commandId 的审计数据。
- 操作：调用 `GET /admin/v1/audit`：
  - 只传 `tenantId`。
  - 组合传 `actor`、`action`、`resource`、`resourceId`。
  - 分别按 `activityId`、`eventId`、`commandId` 过滤。
  - 传入默认 limit、合法 limit、超大 limit。
- 期望：
  - `tenantId` 必填；缺失时返回 400 类错误。
  - 不返回其它 tenant 的审计。
  - 默认按 `createdAt` 倒序返回。
  - `limit` 默认 100，最大不超过 500；超大 limit 被截断或返回参数错误，不能无限制扫描。

### 10. PostgreSQL smoke：审计持久化和 JSON 查询

- 前置：使用 PostgreSQL 集成测试环境，确保表结构包含 `marketing_audit` 所需字段。
- 操作：
  - 在 PostgreSQL repository 中插入人工动作审计、系统失败审计、quota 失败审计。
  - 重启 repository/重新建立连接后查询。
  - 使用 tenant/action/resource/resourceId/activityId/eventId/commandId 过滤。
- 期望：
  - 数据真实落入 PostgreSQL，不只存在于内存仓储。
  - `payload_json` 可持久化并正确反序列化。
  - `created_at` 使用服务端时间，倒序查询稳定。
  - 过滤结果与内存仓储行为一致。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

后续 Test Agent 可按需追加更聚焦的命令，但必须至少执行：

~~~bash
go test ./internal/marketing ./internal/httpapi ./internal/worker ./internal/postgres
go test -race ./internal/marketing ./internal/httpapi ./internal/worker
go test -run 'Test.*Audit|Test.*Quota|Test.*DLQ|Test.*Outbox|Test.*Postgres' ./...
go test ./...
go build ./...
~~~

如 PostgreSQL 集成测试依赖本地容器或环境变量，Test Agent 应先启动/配置项目既有 PostgreSQL 测试环境，再执行 repository integration 测试；不得用纯内存测试替代 PostgreSQL smoke。

## Data / Smoke

- 正常路径数据：
  - `tenantId`: `tenant-a`
  - `operator`: `ops-user-1`
  - `activityId`: 使用测试创建的活动 ID
  - `source`: HTTP method/path + requestId/traceId + 脱敏 IP/userAgent 摘要
- 额度不足数据：
  - 至少覆盖 `quotaType` 中预算、库存、用户次数或每日次数的一种。
  - `amount` 大于剩余额度，确认失败后仍存在 `QUOTA_EXCEEDED` 审计。
- 幂等重复数据：
  - 固定 `eventId` / `commandId` / `outboxId` 重复提交两次以上。
  - 断言业务状态只迁移一次，审计可解释重复行为。
- 并发数据：
  - 并发请求数建议不少于 10。
  - 剩余额度设置为 1 或小于并发数。
  - 断言成功数、失败数、审计数和 quota 余额一致。
- PostgreSQL smoke 数据：
  - 写入至少 3 类审计：人工成功动作、系统失败动作、quota 失败动作。
  - 重新查询确认 `payload_json`、`created_at`、tenant 隔离、过滤字段可用。
- 安全检查：
  - 构造带 `Authorization`、Cookie、secret 字段的请求上下文。
  - 断言审计 payload 中不存在完整 token、Cookie、secret 或明文凭据。

## Decision

~~~text
PASS
~~~

Review 已 PASS，测试范围可执行且覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke；不阻塞 Dev Agent 开发。

需求：2026-05-24-113929-9
