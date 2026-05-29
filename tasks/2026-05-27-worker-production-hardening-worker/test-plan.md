# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

本计划仅定义后续 Test Agent 应执行的测试范围、用例和命令清单。当前 `review.md` Decision 为 `PASS`，可以进入测试设计。

测试目标：

- 验证 worker 生产化语义：按开关启动、常驻轮询、空批次不退出、单条失败不拖垮 worker。
- 验证投递业务语义不被破坏：正常路径生成并投递 outbox；额度不足时不生成错误发奖链路；重复请求保持幂等。
- 验证 PostgreSQL 模式的强一致边界：多 worker 并发 claim 同一 outbox 时只允许一个成功，`SENT` / `DLQ` / `DLQ_CLOSED` 和未到期 retry 不被普通 claim。
- 验证停止和恢复：停止信号后停止新 claim，in-flight 在 drain 内完成或在 drain timeout 后可通过 stale 恢复。
- 验证观测：claim 延迟、DLQ 堆积、in-flight、claim / success / failure / DLQ / stale recovery 等关键计数可被测试读取；结构化日志字段完整且脱敏。

测试分层：

- 单元测试：worker loop、claim 条件、状态机、retry / DLQ 判断、metrics recorder、日志脱敏。
- 集成测试：使用 fake sender / controllable clock / controllable repository 验证常驻、drain、stale 恢复和幂等。
- PostgreSQL smoke：使用真实 PostgreSQL repository 验证并发 claim、锁语义、重启恢复、DLQ 堆积查询和 claim 延迟观测。

时间相关测试必须使用可注入 clock 或测试专用短阈值，避免真实等待 30 秒 drain 或 5 分钟 stale threshold。

## Test Cases

### TP-001 Worker 开关关闭不处理

前置条件：

- `EX_MARKETING_OUTBOX_WORKER` 未设置或为 `false`。
- 准备 1 条 eligible `PENDING` outbox。

步骤：

1. 启动服务或 worker bootstrap。
2. 等待超过一个测试 poll interval。
3. 查询 outbox 状态和 fake sender 调用次数。

期望：

- outbox 仍为 `PENDING`。
- fake sender 调用次数为 0。
- 没有 claim 成功计数。

### TP-002 正常路径：常驻处理第一批和后续新增 outbox

前置条件：

- `EX_MARKETING_OUTBOX_WORKER=true`。
- fake sender 返回成功。
- 准备第一条 eligible `PENDING` outbox。

步骤：

1. 启动 worker。
2. 等待第一条 outbox 被投递并写为 `SENT`。
3. 在 worker 不重启的情况下新增第二条 eligible `PENDING` outbox。
4. 等待第二条 outbox 被处理。

期望：

- 两条 outbox 均最终为 `SENT`。
- fake sender 收到两次不同 outbox 的调用。
- worker 未在空批次后退出。
- `claim_success`、`delivery_success`、`empty_batch` 或等价观测计数可被读取。

### TP-003 额度不足：不创建错误发奖链路

前置条件：

- 构造 campaign quota / budget / inventory 不足的事件输入。
- worker 开启或关闭均可，建议开启以验证无 outbox 时不会误投递。

步骤：

1. 调用标准事件处理入口。
2. 查询 participation、grant_order、marketing_outbox 和 quota / budget / inventory 变化。
3. 等待一个 poll interval。

期望：

- 请求返回既有的额度不足结果或业务拒绝结果。
- 不创建 grant_order。
- 不创建 marketing_outbox。
- quota / budget / inventory 不被错误扣减。
- fake sender 调用次数为 0。
- worker 空轮询不产生失败状态。

### TP-004 幂等重复：重复事件不重复扣减和不重复 outbox

前置条件：

- 构造同一 tenant 下相同 idempotency key / event id 的可中奖事件。
- fake sender 返回成功。

步骤：

1. 第一次提交事件，等待 outbox 生成并被 worker 投递。
2. 第二次提交相同幂等输入。
3. 查询 participation、quota、grant_order、marketing_outbox 和 sender 调用。

期望：

- 业务结果按既有幂等语义返回同一结果或重复结果。
- participation / grant_order / marketing_outbox 不重复创建。
- quota / budget / inventory 只扣减一次。
- 同一业务 commandId / bill_no 只由原 outbox 承载，不生成新的幂等标识。

### TP-005 单条投递失败不导致 worker 退出

前置条件：

- 准备两条 eligible outbox。
- fake sender 对第一条返回可重试失败，对第二条返回成功。

步骤：

1. 启动 worker。
2. 等待两条 outbox 都经过处理。
3. 查询状态和 worker 健康 / 统计快照。

期望：

- 第一条按既有规则进入 `FAILED` 并设置 `next_retry_at`，或达到上限进入 `DLQ`。
- 第二条为 `SENT`。
- worker 仍在运行。
- 失败计数和成功计数均可观测。

### TP-006 Retry 到期边界

前置条件：

- 准备一条 `FAILED` 且 `next_retry_at` 在未来的 outbox。
- 准备一条 `FAILED` 且 `next_retry_at` 已到期的 outbox。

步骤：

1. 启动 worker。
2. 等待一个 poll interval。
3. 查询两条 outbox 状态和 sender 调用。

期望：

- 未到期 outbox 不被 claim，状态保持 `FAILED`。
- 已到期 outbox 可被 claim 并投递。
- claim 查询受 `next_retry_at` 条件约束。

### TP-007 终态不参与普通 claim

前置条件：

- 准备 `SENT`、`DLQ`、`DLQ_CLOSED` 各一条 outbox。

步骤：

1. 启动 worker。
2. 等待一个 poll interval。
3. 查询状态和 sender 调用。

期望：

- 三条记录均不被 claim。
- sender 调用次数为 0。
- 终态记录无 `updated_at` 异常变更。

### TP-008 Shutdown drain：停止后不再 claim 新消息

前置条件：

- fake sender 可阻塞。
- 准备第一条 eligible outbox，使其被 claim 后阻塞在投递中。
- 在停止信号发出前后准备第二条 eligible outbox。

步骤：

1. 启动 worker 并等待第一条进入 in-flight。
2. 触发 worker stop / context cancel。
3. 在 stop 后插入第二条 eligible outbox。
4. 释放第一条 fake sender，使其完成。
5. 等待 worker stopped。

期望：

- 第一条在 drain 窗口内写入最终状态。
- 第二条没有被该 worker claim。
- 观测日志包含 shutdown requested、claim stopped、drain started、drain completed、worker stopped 或等价事件。

### TP-009 Shutdown drain timeout 后 stale 恢复

前置条件：

- fake sender 长时间阻塞，测试 drain timeout 设置为短值。
- stale threshold 使用测试短值或 controllable clock。

步骤：

1. 启动 worker 并让一条 outbox 进入 `SENDING` in-flight。
2. 触发停止，等待 drain timeout 后 worker 退出。
3. 验证该 outbox 未丢失且仍可识别为未完成。
4. 推进 clock 或等待超过测试 stale threshold。
5. 重启 worker，fake sender 改为成功。

期望：

- drain timeout 后未完成 outbox 不会被删除。
- 重启后 stale `SENDING` 恢复为可 claim 状态并重新投递。
- 恢复动作不新增 participation、grant_order、quota 扣减或 outbox。
- 原 payload、target channel、target ref、commandId / bill_no 保持不变。
- `stale_recovered` 或等价计数递增。

### TP-010 PostgreSQL 并发 claim：同一 outbox 只能一个 worker 成功

前置条件：

- 使用真实 PostgreSQL repository。
- 准备同一 tenant 下 1 条 eligible `PENDING` outbox。
- 启动至少 2 个 worker 实例，使用不同 `worker_id`。
- fake sender 阻塞，方便观察并发窗口。

步骤：

1. 同时释放多个 worker 开始扫描。
2. 等待 claim 尝试完成。
3. 查询该 outbox 状态、claim owner / worker 标识、sender 调用次数和 claim 冲突计数。

期望：

- 只有一个 worker claim 成功。
- 同一 outbox 同一处理窗口内 sender 调用次数为 1。
- 其他 worker 记录 claim conflict / no rows affected，不报致命错误。
- 数据库中状态为 `SENDING` 或后续最终状态，无重复状态污染。

### TP-011 PostgreSQL 多 worker 多记录并发吞吐

前置条件：

- 使用真实 PostgreSQL repository。
- 准备多个 tenant / channel 的多条 eligible outbox。
- 启动至少 2 个 worker。

步骤：

1. 并发启动 worker 处理整批 outbox。
2. 等待全部到达终态或 retry / DLQ。
3. 统计每条 outbox 的 sender 调用次数和最终状态。

期望：

- 每条 outbox 最多被一个 worker 同时 claim。
- 不发生跨 tenant 或跨 outbox 状态污染。
- 多条记录可以被并行处理。

### TP-012 Stale 恢复与正常 claim 并发

前置条件：

- 使用 PostgreSQL repository。
- 准备一条超过 stale threshold 的 `SENDING` outbox。
- 启动一个 stale recovery 流程和一个普通 worker claim 流程。

步骤：

1. 同时触发 stale recovery 和普通扫描。
2. 查询状态变更结果和冲突计数。

期望：

- 只有一个状态条件更新成功。
- 失败方按并发冲突处理，不产生重复投递。
- outbox 后续仍可被正常处理。

### TP-013 DLQ 堆积观测

前置条件：

- 准备不同 tenant / target channel 下的 `DLQ` outbox。
- 若阈值可配置，设置测试阈值为易触发值。

步骤：

1. 启动 worker 或 metrics collector。
2. 读取 metrics recorder / stats snapshot / structured log。

期望：

- DLQ count 可按 tenant 和 channel 区分。
- DLQ 大于 0 时产生 warning 观测事件或等价指标。
- DLQ 记录不被普通 worker 自动 claim。

### TP-014 Claim 延迟观测

前置条件：

- 准备一条 `created_at` 或 `next_retry_at` 明显早于当前测试时间的 eligible outbox。
- 启动 worker。

步骤：

1. 等待 outbox 被 claim。
2. 读取 metrics recorder / stats snapshot / structured log。

期望：

- claim delay 使用 `now - eligible_at` 计算。
- 指标或日志包含可验证的 `claim_delay_ms`。
- 延迟超过测试 warning 阈值时产生可识别 warning 标签或事件。

### TP-015 结构化日志字段和敏感信息脱敏

前置条件：

- 构造 target ref、provider error、HTTP header 或 MQ 配置中包含 `Authorization`、`Cookie`、`token`、`secret`、签名密钥等敏感字样。
- fake sender 返回失败并携带敏感错误内容。

步骤：

1. 触发 worker 处理失败 outbox。
2. 捕获结构化日志。

期望：

- 日志至少包含 `worker_id`、`event`、`tenant_id`、`outbox_id`、`target_channel`、`message_type`、状态前后、`retry_count`、`claim_delay_ms`、`delivery_duration_ms`、`error_class`、`error_message` 摘要。
- 日志不包含完整 Authorization、Cookie、token、secret、签名密钥、MQ 明文凭据或完整敏感响应体。

### TP-016 PostgreSQL smoke：端到端最小生产路径

前置条件：

- 使用真实 PostgreSQL 测试库。
- worker 开启。
- fake external reward system 返回成功。

步骤：

1. 通过标准事件入口提交一条可发奖事件。
2. 等待 participation、grant_order、marketing_outbox 创建。
3. 等待 worker claim 并投递成功。
4. 查询数据库状态和观测输出。

期望：

- 标准链路完成：Event In -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota -> Grant Order -> Outbox -> Worker -> Sender。
- outbox 最终为 `SENT`。
- quota 扣减、grant_order 和 outbox 数据一致。
- claim 延迟和成功计数可观测。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

后续 Test Agent 如项目已有 PostgreSQL 集成测试标签或环境变量，应补充执行对应命令。建议命令清单如下，具体名称以仓库现有测试约定为准：

~~~bash
EX_MARKETING_OUTBOX_WORKER=true go test ./... -run 'Outbox|Worker|DLQ|Quota|Idempot'
EX_MARKETING_OUTBOX_WORKER=true EX_MARKETING_REPOSITORY=postgres go test ./... -run 'Postgres|Outbox|Worker|Claim|Smoke'
go build ./...
~~~

执行约束：

- Test Agent 可以执行测试、构建和必要的测试环境启动命令。
- Test Plan Agent 本轮不得执行上述命令。
- 如果 PostgreSQL smoke 需要 Docker Compose 或本地 PostgreSQL，Test Agent 应按仓库现有说明启动测试依赖，并在报告中记录连接串、迁移方式和清理方式。

## Data / Smoke

PostgreSQL smoke 必须覆盖以下数据集：

- `PENDING` eligible outbox：用于正常 claim 和发送。
- `FAILED` retry 未到期 outbox：验证不会提前 claim。
- `FAILED` retry 已到期 outbox：验证到期后可 claim。
- `SENDING` stale outbox：验证恢复，不新增 participation / grant_order / quota 扣减 / outbox。
- `SENT` / `DLQ` / `DLQ_CLOSED` outbox：验证终态不参与普通 claim。
- 多 tenant、多 target channel outbox：验证并发处理无跨租户污染，DLQ count 可按 tenant/channel 区分。
- 额度不足事件：验证不创建 grant_order / outbox，不投递。
- 重复幂等事件：验证不重复扣减和不重复创建 outbox。

PostgreSQL smoke 最小断言：

- 同一 outbox 在多 worker 并发下 sender 调用次数最多为 1。
- drain timeout 后残留 `SENDING` 可在 stale threshold 后恢复并最终处理。
- claim delay、DLQ backlog、in-flight、claim success / conflict、delivery success / failure、DLQ、stale recovery 计数可被测试读取。
- 结构化日志字段完整并脱敏。

建议测试夹具：

- Fake sender：支持成功、可重试失败、不可重试失败、阻塞、超时、携带敏感错误内容。
- Controllable clock：支持推进 retry 到期、claim delay 和 stale threshold。
- Metrics recorder / stats snapshot：用于断言计数和延迟，不只依赖自然语言日志。
- Worker harness：支持启动多个 worker、指定 worker_id、触发 context cancel、等待 drain / stopped。

## Decision

~~~text
PASS
~~~

需求：2026-05-27-worker-production-hardening-worker
