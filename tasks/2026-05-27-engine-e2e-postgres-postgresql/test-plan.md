# Test Plan

## Scope

需求：`2026-05-27-engine-e2e-postgres-postgresql`

Review Decision：`PASS`

本计划只定义后续 Test Agent 应执行的测试用例和命令清单。Test Agent 必须验证真实 PostgreSQL 端到端验收脚本是否满足 task / analysis / review 的要求，不得用内存 repository 或管理后台页面替代。

## Test Strategy

1. 先做静态检查：确认 Dev 已记录实际 PostgreSQL E2E 命令、环境变量、端口覆盖方式、runId 隔离策略，以及成功/失败分支的 subjectId 或 quota totalValue 设计。
2. 再做通用质量门禁：运行全量 Go 测试和构建，确认新增脚本未破坏现有工程。
3. 最后做真实 PostgreSQL E2E：使用 Docker PostgreSQL 或等价真实 PostgreSQL，设置 `EX_MARKETING_DATABASE_URL`，启动服务和 outbox worker，执行 Dev 提供的一条 E2E 命令。
4. 所有验收都必须记录命令、关键环境变量摘要、退出码、PASS / FAIL 输出、失败阶段诊断和关键数据库状态摘要。
5. 若本地或 CI 无 Docker/PostgreSQL，Test Agent 必须判定为环境阻塞并记录 `BLOCK`，不得把未运行的 PostgreSQL E2E 当作 PASS。

## Preconditions

Test Agent 执行前必须确认：

- `review.md` 的 Decision 为 `PASS`。
- Dev 已在任务产物或实现说明中写明真实 E2E 命令；若未写明，直接 `BLOCK`。
- PostgreSQL 连接串指向真实 PostgreSQL，例如：

~~~bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
~~~

- 服务地址、stub 地址、worker 开关均可通过环境变量或命令参数覆盖。
- `EX_MARKETING_OUTBOX_WORKER=true` 或等价配置已启用。
- 测试数据使用唯一 `runId`，不得无条件清空非本次任务数据。

## Test Cases

### TC-01 正常路径：PostgreSQL 全链路成功发奖

步骤：

1. 启动或连接真实 PostgreSQL。
2. 设置 `EX_MARKETING_DATABASE_URL`，启动 ExMarketing 服务，确认不是内存 repository。
3. 执行 Dev 提供的 PostgreSQL E2E 命令。
4. 验证脚本通过 Admin API 创建活动、创建 quota、提交审批、审批通过、激活活动。
5. 上报成功分支标准事件，等待 outbox worker 发送。
6. 触发或接收 `SUCCESS` callback。
7. 调用 reconciliation API，并查询数据库状态。

预期：

- 命令退出码为 0，输出明确 `PASS`。
- event inbox、participation、grant_order、outbox 均存在并关联同一 runId / tenantId。
- outbox 进入 `SENT` 或等价已发送状态。
- grant_order 最终为 `SUCCESS`。
- BUDGET、STOCK、USER_LIMIT、DAILY_LIMIT reservation 从 `FROZEN` 进入确认口径。
- quota `frozen_value` 下降，`used_value` 按成功奖励金额或次数增加。
- reconciliation 返回本次链路的 event、participation、grant_order、outbox、reservation、callback/audit 时间线。

### TC-02 额度不足：quota 拒绝且无半成品发奖链路

步骤：

1. 使用 Dev 提供的额度不足场景，或在同一 E2E 命令支持的负向参数下运行 quota insufficient case。
2. 构造会超过 BUDGET、STOCK、USER_LIMIT 或 DAILY_LIMIT 任一额度的事件。
3. 通过 `POST /openapi/v1/events` 上报事件。
4. 查询 event inbox、participation、grant_order、outbox、reservation、quota 聚合。

预期：

- 事件处理返回或记录 `QUOTA_EXCEEDED` / 等价额度不足原因。
- 不得生成可发送的 grant_order 或 outbox。
- 若生成 participation，必须是拒绝态或可对账的失败态。
- quota 聚合不得出现错误的 `used_value` 增加。
- 不得残留无法终结的 `FROZEN` reservation。
- 失败输出必须包含 quota 当前值、请求摘要、响应摘要和相关数据库状态。

### TC-03 幂等重复：相同事件重复上报不重复发奖

步骤：

1. 对成功分支使用相同 payload 和相同 `Idempotency-Key` 再次调用 `POST /openapi/v1/events`。
2. 记录重复调用响应。
3. 查询本次 runId 下 participation、grant_order、outbox、reservation 数量。

预期：

- 重复调用返回 `duplicate=true` 或等价幂等结果。
- participation、grant_order、outbox、quota reservation 数量不增加。
- quota `used_value` / `frozen_value` 不因重复事件变化。
- 脚本输出幂等验证摘要。

### TC-04 幂等重复：callback 重复不重复确认或释放

步骤：

1. 对成功分支的 grant_order 重复发送同一 `SUCCESS` callback。
2. 对失败分支的 grant_order 重复发送同一 `FAILED` callback。
3. 查询 grant_order、reservation、quota 聚合和 audit。

预期：

- 重复 callback 不改变已终结 grant_order 的最终状态。
- 成功分支 quota 不重复增加 `used_value`。
- 失败分支 quota 不重复释放或产生负数。
- callback API 返回幂等成功或等价已处理结果。

### TC-05 失败分支：callback FAILED 释放 quota

步骤：

1. 使用与成功分支不同的 bizId 和 subjectId，或确认 USER_LIMIT / DAILY_LIMIT `totalValue >= 2`。
2. 上报失败分支标准事件，等待 grant_order 和 outbox 生成。
3. 等待 outbox worker 发送。
4. 触发或接收 `FAILED` callback。
5. 查询 reservation、quota 聚合、grant_order、participation 和 reconciliation。

预期：

- 失败分支独立生成 grant_order 和 outbox。
- grant_order 最终为 `FAILED`。
- 对应 reservation 从 `FROZEN` 进入 `RELEASED`。
- quota `frozen_value` 下降，`used_value` 不因失败分支增加。
- participation 最终体现失败态或可对账失败结果。
- reconciliation 能区分成功分支和失败分支。

### TC-06 并发：重复事件并发提交只产生一条业务链路

步骤：

1. 使用同一 tenantId、sourceSystem、eventType、bizType、bizId 和 `Idempotency-Key` 并发提交同一标准事件。
2. 并发度建议从 5 到 20；以 Dev 脚本支持的参数为准。
3. 等待所有请求完成，再等待异步 worker 稳定。
4. 查询 event inbox、participation、grant_order、outbox、reservation 数量。

预期：

- 只有一个请求创建真实业务链路，其余请求返回 duplicate 或等价幂等结果。
- 同一幂等键下只有一条 event inbox 主记录。
- 不得重复生成 participation、grant_order、outbox 或重复冻结 quota。
- 无数据库唯一键异常泄漏为 500；若发生冲突，应被转换为幂等结果或明确业务错误。

### TC-07 并发：不同事件并发扣减 quota 保持一致

步骤：

1. 使用同一活动、不同 bizId、不同 subjectId 或足够 USER_LIMIT / DAILY_LIMIT。
2. 并发提交多条满足规则的标准事件。
3. quota 总量设置为可预测值，至少覆盖成功扣减和达到上限两种结果。
4. 等待 worker 和 callback 完成后查询 quota 聚合与 reservation。

预期：

- 成功数量不得超过 quota 总量。
- 超额事件必须被拒绝或不生成可发送发奖链路。
- quota `used_value + frozen_value + released/available` 口径一致，不出现负数或超过 totalValue。
- reservation 与 grant_order 一一对应，最终状态可对账。

### TC-08 PostgreSQL smoke：真实仓储和数据库状态验证

步骤：

1. 在未设置 `EX_MARKETING_DATABASE_URL` 的情况下运行 E2E 命令一次。
2. 在 PostgreSQL 未启动或连接串错误的情况下运行 E2E 命令一次。
3. 在正确 PostgreSQL 环境下运行 E2E 命令。
4. 用 SQL 或脚本内诊断确认本次 runId 数据写入 PostgreSQL 表。

预期：

- 未设置 DSN 时必须失败，并说明不能使用内存 repository 完成本验收。
- PostgreSQL 不可连接时必须失败，并输出连接错误和环境摘要。
- 正确环境下必须通过，且数据库中存在本次 runId 对应的 activity、quota、event inbox、participation、grant_order、outbox、reservation/audit 数据。
- 失败日志不得泄露完整 token、secret、Authorization header 或签名。

### TC-09 可重复执行：同一命令连续两次可通过

步骤：

1. 在同一 PostgreSQL 环境下连续执行两次 Dev 提供的 E2E 命令。
2. 两次执行都不手动清空全库。
3. 比较两次 runId、activityCode、bizId、subjectId 或 metadata。

预期：

- 两次命令均退出 0 并输出 `PASS`。
- 第二次不受第一次残留数据影响。
- 两次数据可按 runId 隔离查询。
- 脚本未无条件删除非本次 run 数据。

### TC-10 失败诊断：任一阶段失败输出可定位

步骤：

1. 人为制造一个可恢复失败，例如关闭 outbox worker、使用错误 callback secret、使用无效 PostgreSQL DSN，或占用 callback stub 端口。
2. 执行 E2E 命令并记录失败输出。

预期：

- 命令退出非 0。
- 输出明确失败阶段，例如 `OUTBOX_SEND`、`CALLBACK_SUCCESS`、`CALLBACK_FAILED`、`POST_EVENT_SUCCESS_CASE`。
- 输出 HTTP method/path、请求摘要、响应状态和响应体摘要。
- 输出 activity、quota、event inbox、participation、grant_order、outbox、reservation/audit 的 runId 相关快照。
- 输出环境摘要并脱敏敏感值。

## Required Commands

Test Agent 必须执行并记录以下命令。具体 PostgreSQL E2E 命令以 Dev 最终实现记录为准；若 Dev 未记录实际命令，判定 `BLOCK`。

~~~bash
go test ./...
go build ./...
~~~

PostgreSQL E2E 命令模板：

~~~bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
export EX_MARKETING_OUTBOX_WORKER=true
export EX_MARKETING_BASE_URL='http://localhost:8080'

# Replace with the exact command documented by Dev.
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND>
~~~

连续运行验证：

~~~bash
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND>
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND>
~~~

并发和负向场景若 Dev 提供独立参数或子命令，Test Agent 必须补充执行，例如：

~~~bash
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND> --case quota-insufficient
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND> --case duplicate-event-concurrency
<DEV_DOCUMENTED_POSTGRES_E2E_COMMAND> --case quota-concurrency
~~~

如果 Dev 将所有场景合并在一条命令中，Test Agent 只需运行该命令，但必须在输出中逐项核对 TC-01 到 TC-10。

## Data / Smoke

Test Agent 必须至少记录以下数据摘要：

- PostgreSQL DSN 是否设置，敏感信息脱敏。
- 服务地址、stub 地址、worker 开关。
- runId、tenantId、activityId、activityCode。
- 成功分支 bizId、subjectId、idempotencyKey、grantOrderId、outboxId。
- 失败分支 bizId、subjectId、idempotencyKey、grantOrderId、outboxId。
- BUDGET、STOCK、USER_LIMIT、DAILY_LIMIT 的 totalValue、usedValue、frozenValue。
- 成功分支 reservation 最终状态。
- 失败分支 reservation 最终状态。
- 幂等重复前后的 participation、grant_order、outbox、reservation 数量。
- 并发场景成功数、duplicate 数、拒绝数、异常数。
- reconciliation 响应是否包含 event、participation、grant_order、outbox、reservation、audit/callback 时间线。

## PASS Criteria

Test Agent 可判定 PASS 的条件：

- `go test ./...` 通过。
- `go build ./...` 通过。
- Dev 已记录实际 PostgreSQL E2E 命令。
- PostgreSQL E2E 在真实 PostgreSQL 上运行通过。
- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 均完成并符合预期。
- 连续运行两次不依赖清空全库且均通过。
- 失败诊断满足阶段、请求、响应、数据库状态和环境摘要要求。

## BLOCK Criteria

出现任一情况必须判定 BLOCK：

- `review.md` 不是 `PASS`。
- Dev 未记录实际 E2E 命令。
- 无法运行真实 PostgreSQL，且没有可替代真实 PostgreSQL 环境。
- E2E 命令退回内存 repository。
- 未覆盖成功 callback 确认和失败 callback 释放。
- 未覆盖 quota 不足、幂等重复、并发或 PostgreSQL smoke。
- 成功/失败分支复用同一 subjectId 且 USER_LIMIT / DAILY_LIMIT totalValue 不足，导致失败分支被次数限制提前挡住。
- 失败时缺少可定位诊断。
- 任一必跑命令失败且不是明确环境阻塞。

## Decision

~~~text
PASS
~~~

理由：`review.md` 已为 PASS，本计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，并给出后续 Test Agent 必跑命令和 PASS/BLOCK 判定规则。
