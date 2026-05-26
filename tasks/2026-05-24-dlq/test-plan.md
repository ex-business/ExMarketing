# Test Plan: 2026-05-24-dlq

## Agent 指令

你是后续 Test Agent。只有 `review.md` 为 PASS 且 Dev Agent 完成实现后，才执行本测试计划。

本文件只定义测试用例和命令清单。本轮 Test Plan Agent 未执行测试、构建、格式化或 git 命令。

测试目标：

- 覆盖 DLQ 查询、失败原因展示、手动 replay、手动 close、审计写入。
- 验证 replay 只让同一 outbox 重新进入可投递状态，不重复扣预算 / 配额，不重复生成 `grant_order`，不重复创建 outbox。
- 覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。

## Preconditions

后续 Test Agent 执行前需确认：

- Dev Agent 已完成 DLQ Admin API / service / repository 相关实现。
- 本地 PostgreSQL 测试环境可用；如项目已有 testcontainers 或集成测试配置，优先使用项目既有方式。
- 测试数据使用独立 tenant，例如 `tenant_dlq_test`，避免污染其他用例。
- 不依赖真实外部发奖系统；如项目已有 fake reward target / mock HTTP server，优先使用既有测试工具。

## Test Strategy

分层验证：

- Unit tests：验证状态流转、入参校验、tenant 隔离、审计 payload、幂等保护和并发条件更新。
- Repository / PostgreSQL integration tests：验证真实 PostgreSQL 下 outbox 状态更新、审计落库、唯一约束和事务一致性。
- HTTP handler tests：验证 Admin API 的请求 / 响应、错误码、includeClosed 查询语义和敏感错误信息脱敏。
- End-to-end smoke：构造一条真实进入 DLQ 的 outbox，执行查询、replay、worker 再投递或状态回到 PENDING、close 的完整 smoke。

关键断言：

- replay / close 只能操作当前 `send_status = DLQ` 的记录。
- replay 成功后仍是同一 outbox id，`payload_json`、`target_channel`、`target_ref`、`commandId` / `bill_no` 不变。
- replay 不创建新的 participation、grant_order、outbox，不再次扣减 quota / budget / inventory / user limit。
- close 成功后状态为 `DLQ_CLOSED`，后续 replay 必须失败。
- replay / close 成功必须写 `marketing_audit`；审计失败时操作不得静默成功。
- tenant 不匹配时不可读、不可操作。

## Test Cases

### 1. DLQ 查询正常路径

步骤：

1. 准备同一 tenant 下 2 条 `DLQ` outbox、1 条 `DLQ_CLOSED` outbox、1 条 `SENT` outbox。
2. 调用 `GET /admin/v1/outbox/dlq?tenantId=tenant_dlq_test`。
3. 调用 `GET /admin/v1/outbox/dlq?tenantId=tenant_dlq_test&includeClosed=true`。

期望：

- 默认查询只返回 `DLQ`，不返回 `DLQ_CLOSED` / `SENT`。
- `includeClosed=true` 返回 `DLQ` 和 `DLQ_CLOSED`，不返回 `SENT`。
- 响应包含 outbox id、tenant id、message type、target channel、target ref、send status、retry count、error message、created time、updated time。
- error message 展示失败摘要，但不得包含 token、Authorization、secret、签名密钥等敏感信息。

### 2. DLQ 查询租户隔离

步骤：

1. 准备 tenant A 和 tenant B 各 1 条 `DLQ` outbox。
2. 用 tenant A 查询列表。
3. 用 tenant A 对 tenant B 的 outbox id 执行 replay / close。

期望：

- tenant A 列表不包含 tenant B 记录。
- tenant A 操作 tenant B outbox 失败，返回 not found / forbidden / tenant mismatch 中项目约定的一种可识别错误。
- tenant B outbox 状态不变，且不产生 tenant A 的成功审计。

### 3. Replay 正常路径

步骤：

1. 通过活动链路或测试 fixture 准备一条 `DLQ` outbox，并关联已有 participation、grant_order、quota / budget 记录。
2. 记录 replay 前的 outbox id、payload、`commandId` / `bill_no`、participation 数量、grant_order 数量、outbox 数量、quota / budget 已用量。
3. 调用 `POST /admin/v1/outbox/{id}/replay`，body 包含正确 `tenantId`、`operator`、`reason`。
4. 查询 outbox 和 audit。

期望：

- replay 返回成功。
- 同一 outbox 状态从 `DLQ` 变为可投递状态，默认期望为 `PENDING`。
- outbox id 不变，payload 不变，`commandId` / `bill_no` 不变。
- participation 数量不变，grant_order 数量不变，outbox 数量不变。
- quota / budget / inventory / user limit 已用量不增加。
- `marketing_audit` 新增 `OUTBOX_DLQ_REPLAY`，包含 operator、reason、beforeStatus=`DLQ`、afterStatus=`PENDING`、resource_id、tenant、targetChannel、targetRef、retryCount 和失败摘要。

### 4. Close 正常路径

步骤：

1. 准备一条 `DLQ` outbox。
2. 调用 `POST /admin/v1/outbox/{id}/close`，body 包含正确 `tenantId`、`operator`、`reason`。
3. 再对同一 outbox 调用 replay。

期望：

- close 返回成功。
- outbox 状态变为 `DLQ_CLOSED`。
- `marketing_audit` 新增 `OUTBOX_DLQ_CLOSE`，包含 operator、reason、beforeStatus=`DLQ`、afterStatus=`DLQ_CLOSED`。
- close 不修改 participation、grant_order、quota reservation 的业务状态。
- close 后 replay 失败，状态仍为 `DLQ_CLOSED`，不新增 replay 成功审计。

### 5. 非 DLQ 状态不可 replay / close

步骤：

1. 分别准备 `PENDING`、`SENDING`、`SENT`、`FAILED`、`DLQ_CLOSED` 状态 outbox。
2. 对每条记录分别调用 replay 和 close。

期望：

- 所有非 `DLQ` 状态操作均失败。
- 返回状态冲突 / 不可执行类错误。
- 原状态不变。
- 不写成功审计。

### 6. 入参校验

步骤：

1. 对 `DLQ` outbox 调用 replay / close，分别缺少 `tenantId`、`operator`、`reason`。
2. 使用空字符串或仅空白字符作为 `operator` / `reason`。
3. 使用不存在的 outbox id。

期望：

- 缺少必填字段或空白字段返回 validation error。
- 不存在 id 返回 not found。
- outbox 状态不变。
- 不写成功审计。

### 7. 额度不足路径

步骤：

1. 构造一个活动奖励链路，第一次事件处理因 quota / budget / inventory / user limit 不足而不应生成可发奖 grant_order / outbox。
2. 确认没有 DLQ outbox 可供 replay。
3. 再构造一条已经生成 grant_order / outbox、但投递失败进入 `DLQ` 的记录，然后将 quota / budget 调整到不足状态。
4. 对该 DLQ 执行 replay。

期望：

- 额度不足导致的活动链路拒绝不会产生 DLQ replay 对象。
- 对既有 DLQ 执行 replay 时，不重新跑额度校验扣减，不因当前额度不足重复扣减或生成新 grant_order。
- replay 后同一 outbox 可重新投递；quota / budget / inventory / user limit 已用量不增加。

### 8. 幂等重复 replay

步骤：

1. 准备一条 `DLQ` outbox。
2. 连续两次对同一 outbox 调用 replay，第二次发生在第一次已成功提交后。
3. 记录 grant_order、outbox、quota / budget、audit。

期望：

- 第一次 replay 成功。
- 第二次 replay 因状态已不再是 `DLQ` 而失败或返回项目约定的幂等不可执行结果。
- participation、grant_order、outbox 数量不增加。
- quota / budget 已用量不增加。
- 只有一次成功 replay 审计；失败请求如项目记录失败审计，也必须明确标识失败，不能伪装为成功 replay。

### 9. 幂等重复事件与 replay 隔离

步骤：

1. 用相同 `tenantId:sourceSystem:eventType:bizType:bizId` 重复提交同一标准事件，触发项目既有事件幂等逻辑。
2. 确认只生成一组 participation、grant_order、outbox。
3. 将该 outbox 标记或驱动进入 `DLQ`。
4. 执行 replay。

期望：

- 重复事件不重复创建 participation、grant_order、outbox。
- replay 不重新进入事件接入链路，不创建新的 inbox / participation / grant_order / outbox。
- replay 后 payload 中的 `commandId` / `bill_no` 与原 grant_order 一致。

### 10. 并发 replay vs replay

步骤：

1. 准备一条 `DLQ` outbox。
2. 并发发起 N 个 replay 请求，建议 N=10 或使用项目并发测试约定。
3. 查询最终 outbox、audit、grant_order、quota / budget。

期望：

- 最多 1 个 replay 成功。
- 其余请求返回状态冲突 / 不可执行 / 幂等失败类结果。
- 最终 outbox 状态为 `PENDING` 或项目定义的可投递状态。
- 只有一次成功 replay 审计。
- 不重复生成 grant_order / outbox，不重复扣 quota / budget。

### 11. 并发 replay vs close

步骤：

1. 准备一条 `DLQ` outbox。
2. 并发发起 1 个 replay 和 1 个 close，重复执行多轮以覆盖不同竞态结果。
3. 查询最终 outbox 和 audit。

期望：

- 每轮最多 1 个操作成功。
- replay 成功时最终状态为 `PENDING`，close 失败。
- close 成功时最终状态为 `DLQ_CLOSED`，replay 失败。
- 不允许出现两个操作都成功或审计显示两个成功操作。
- 不重复创建业务数据，不重复扣减。

### 12. 审计失败事务一致性

步骤：

1. 使用 repository mock、事务测试钩子或临时约束方式制造 audit insert 失败。
2. 对 `DLQ` outbox 执行 replay / close。
3. 查询 outbox 状态。

期望：

- API 返回可识别错误。
- outbox 状态保持 `DLQ`。
- 不出现“状态已变更但无审计”的结果。

### 13. PostgreSQL smoke

步骤：

1. 在真实 PostgreSQL 测试库中运行迁移或使用项目既有 schema 初始化。
2. 插入或通过 service 创建 tenant、activity、participation、grant_order、marketing_outbox、marketing_audit 所需最小数据。
3. 对一条 `DLQ` outbox 执行查询、replay、再次查询、close 另一条 DLQ、查询 audit。
4. 使用 SQL 或 repository 查询核对持久化结果。

期望：

- PostgreSQL schema 支持 `DLQ`、`DLQ_CLOSED` 状态。
- replay / close 在事务内正确更新 `marketing_outbox` 并写入 `marketing_audit`。
- 条件更新能防止并发重复成功。
- JSON payload 在 PostgreSQL 中保持结构和关键字段不变。
- 所有 smoke 数据可按 tenant 隔离查询。

## Required Commands

后续 Test Agent 在 Dev 实现后执行以下命令。若项目有更精确的 package 路径，可先跑窄范围测试，再跑全量命令。

~~~bash
go test ./...
go build ./...
~~~

建议的窄范围命令清单，具体 package 名称由 Test Agent 按实现后的目录确认：

~~~bash
go test ./... -run 'Test.*DLQ|Test.*Outbox.*Replay|Test.*Outbox.*Close|Test.*Audit'
go test ./... -run 'Test.*Postgres|Test.*Repository|Test.*Integration'
go test ./... -race -run 'Test.*DLQ.*Concurrent|Test.*Outbox.*Concurrent|Test.*Replay.*Concurrent|Test.*Close.*Concurrent'
~~~

如果项目使用环境变量控制 PostgreSQL 集成测试，Test Agent 应按项目现有说明设置，例如：

~~~bash
POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Test.*Postgres|Test.*Integration'
~~~

## Data / Smoke Checklist

Test Agent 最终报告至少记录：

- DLQ 查询默认不含 closed，`includeClosed=true` 含 closed。
- replay 成功前后的 outbox id、状态、payload hash、`commandId` / `bill_no`。
- replay 前后的 participation 数量、grant_order 数量、outbox 数量、quota / budget 已用量。
- close 后状态和 replay 拒绝结果。
- 并发测试成功请求数量和失败请求数量。
- PostgreSQL smoke 的 audit 记录 action、resource_id、operator、reason、beforeStatus、afterStatus。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-dlq
