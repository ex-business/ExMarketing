# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

Review 结论已确认为 PASS。本测试计划面向后续 Test Agent，用于验证 Dev Agent 实现的本地压测和故障注入能力，不要求本轮执行任何命令。

测试目标：

1. 验证压测脚本只能在真实 PostgreSQL repository 模式下判 PASS，不能以内存仓储结果替代。
2. 覆盖正常路径、额度不足、幂等重复、并发、outbox 重试、callback 重复 / 乱序、PostgreSQL smoke / 故障注入。
3. 每个场景都要检查指标输出和业务一致性输出：吞吐、延迟、错误率、runId、seed、profile、失败复现命令、数据库一致性摘要。
4. 预期业务拒绝和系统错误必须分开统计。`QUOTA_EXCEEDED`、duplicate、callback conflict 不计为 unexpected error；panic、不可解释 5xx、数据漂移必须判 FAIL。
5. 针对 review.md 的 P2，quota 并发扣减不能只验证一个模糊 quota。默认 local profile 必须分别覆盖 BUDGET、STOCK、USER_LIMIT、DAILY_LIMIT 四种 quota 瓶颈子用例；只覆盖 BUDGET 不得判定 S2 完成。

## Test Cases

### TC1 正常路径：并发事件压测

前置条件：

- `EX_MARKETING_DATABASE_URL` 指向本地真实 PostgreSQL。
- ExMarketing 服务和 outbox worker 已启动。
- reward callback stub 可用。

步骤：

1. 使用 local profile 准备一个已审批、已激活活动，奖励类型为 HTTP / WEBHOOK 发奖。
2. 并发提交 500 条唯一 `DEPOSIT_SUCCESS` 标准事件，并发度 25。
3. 等待 outbox worker 发送完成，并由 stub 回调成功。
4. 执行脚本内置一致性检查。

期望：

- Event API 无不可解释 5xx、panic、超时风暴。
- 每个唯一 idempotency key 最多一条 event inbox、一条 participation、一条 grant_order、一条 outbox。
- outbox 最终进入 SENT 或等价投递成功状态，grant_order 最终由 callback 进入成功终态。
- quota reservation 从 FROZEN 到 CONFIRMED 只发生一次。
- 输出 events/sec、outbox/sec、callbacks/sec、p50 / p95 / p99、unexpected_error rate、runId、seed。

### TC2 额度不足：quota 并发扣减

前置条件：

- 使用同一活动配置有限 quota。
- local profile 默认 200 条事件、并发度 50、quota 容量 50。

步骤：

1. 对 BUDGET quota 执行并发扣减场景，奖励金额固定且非零。
2. 对 STOCK quota 执行并发扣减场景，使用固定 rewardCode 作为库存 scopeKey。
3. 对 USER_LIMIT quota 执行并发扣减场景，使用固定 subjectId 作为用户 scopeKey。
4. 对 DAILY_LIMIT quota 执行并发扣减场景，使用固定 subjectId + 日期 scopeKey。
5. 每个子用例只让当前 quota 类型成为瓶颈，其余 quota 保持充足，避免检查顺序掩盖目标类型。
6. 对每种 quota 类型检查 aggregate、reservation、participation、grant_order、outbox。

期望：

- 成功发奖数量不超过 quota 容量。
- 超出容量的事件返回 `QUOTA_EXCEEDED` 或等价机器可识别业务拒绝。
- 额度不足事件不得留下 `REWARDABLE` participation、grant_order、outbox 或 FROZEN reservation。
- `used_value + frozen_value <= total_value`，`available_value >= 0`，reservation 汇总与 aggregate 差异为 0。
- 若任一 quota 类型未执行，S2 判 FAIL。

### TC3 幂等重复：重复事件和重复 callback

步骤：

1. 使用同一个 `Idempotency-Key` 重复提交同一标准事件至少 3 次。
2. 对一批已生成 grant_order 的样本重复发送相同 SUCCESS callback 至少 3 次。
3. 对另一批样本重复发送相同 FAILED callback 至少 3 次。
4. 查询 event inbox、participation、grant_order、outbox、reservation 和 audit / reconciliation 摘要。

期望：

- 重复事件返回幂等成功或 duplicate，不新增业务链路。
- SUCCESS 重复 callback 不重复确认 quota，不重复终结 grant_order。
- FAILED 重复 callback 不重复释放 quota。
- 重复请求计入 duplicate 或 expected_reject，不计入 unexpected_error。

### TC4 并发冲突：callback 乱序 / 相反终态

步骤：

1. 准备至少 30 个已生成 grant_order 的样本。
2. 对部分样本先发送 SUCCESS，再发送 FAILED。
3. 对部分样本先发送 FAILED，再发送 SUCCESS。
4. 对部分样本使用未知 commandId / billNo、tenant 不匹配、externalGrantId 冲突的 callback。

期望：

- 首次合法最终态生效，后续相反终态被拒绝或标记 conflict。
- 已终结 grant_order 不被覆盖。
- quota reservation 只发生一次 CONFIRMED 或 RELEASED 状态迁移。
- 未知、租户不匹配、externalGrantId 冲突返回机器可识别错误。

### TC5 outbox 重试和 DLQ

步骤：

1. 准备 50 个会生成 outbox 的成功事件。
2. 配置 reward stub 前 1 到 2 次返回 5xx 或超时，之后恢复 2xx。
3. 若实现有最大重试 / DLQ 阈值，另准备小批始终失败样本。
4. 等待 worker 重试并执行一致性检查。

期望：

- 临时失败时原 outbox payload、commandId / billNo 保持稳定，不创建重复 outbox。
- `retry_count`、`next_retry_at`、错误摘要可观测。
- stub 恢复后 outbox 收敛到 SENT 或等价状态。
- 永久失败样本按规则进入 DLQ，并保留错误诊断。
- outbox 发送成功不直接代表 grant_order 成功，最终态仍由 callback 决定。

### TC6 PostgreSQL smoke：真实数据库基础闭环

步骤：

1. 在真实 PostgreSQL 下启动服务。
2. 执行最小 smoke profile：创建 / 激活活动、提交少量事件、生成 outbox、接收 callback、执行一致性检查。
3. 验证脚本拒绝在未设置 `EX_MARKETING_DATABASE_URL` 或数据库不可连接时继续判 PASS。

期望：

- smoke 输出 PASS，且报告真实 PostgreSQL DSN 的脱敏摘要。
- 数据库不可用时命令失败并输出明确连接错误。
- 不允许以内存 repository 或 mock repository 结果作为本需求 PASS。

### TC7 PostgreSQL 故障注入：重启或网络超时

步骤：

1. 使用 local profile 准备 100 条事件。
2. 在事件提交和 outbox worker 处理期间注入一次 PostgreSQL 重启、连接断开或 3 到 10 秒网络超时。
3. 恢复后使用相同 idempotency key 重试故障窗口内失败请求。
4. 等待 outbox worker 继续处理 eligible outbox。
5. 执行 orphan 和 quota 一致性检查。

期望：

- 故障窗口内请求可以失败，但错误必须可解释。
- 恢复重试后不重复生成 participation、grant_order、outbox，不重复扣 quota。
- 不存在 orphan grant_order、orphan outbox、无 reservation 的 frozen quota、无 outbox 的待发 grant_order。
- stuck `SENDING` 如存在必须被恢复或明确报告为 FAIL。

### TC8 失败诊断和复现能力

步骤：

1. 故意配置 reward stub 持续失败或 PostgreSQL 不可连接。
2. 触发脚本失败。
3. 检查输出和日志。

期望：

- 失败输出包含 runId、seed、profile、开始 / 结束时间、复现命令、环境摘要、关键请求 / 响应摘要、数据库快照摘要。
- Authorization、token、secret、callback signature、数据库密码等敏感信息已脱敏。
- 退出码非 0，不能在数据不一致时输出 PASS。

## Required Commands

~~~bash
# 1. 基础单元 / 集成测试，确认新增压测工具没有破坏现有测试。
go test ./...

# 2. 基础构建，确认压测 / 故障注入入口可编译。
go build ./...

# 3. PostgreSQL smoke。命令名称以 Dev Agent 实际实现为准；Test Agent 必须使用真实 PostgreSQL DSN。
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario pg-smoke

# 4. 全量 local profile，覆盖并发事件、quota、outbox、callback、PostgreSQL 故障注入。
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario all

# 5. 固定 seed 复现失败。seed / runId 使用失败报告中给出的值。
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' \
EX_MARKETING_BASE_URL='http://localhost:8080' \
go run ./cmd/engine-load-fault-tests --profile local --scenario all --run-id '<failed-run-id>' --seed '<failed-seed>'
~~~

## Data / Smoke

PostgreSQL smoke 必须至少验证以下数据闭环：

1. activity 已创建、审批、激活，并能被事件匹配。
2. event inbox 中本 runId 的 idempotency key 唯一。
3. participation、grant_order、outbox 均能通过 tenant + activity + runId 串联。
4. quota aggregate 与 reservation 明细一致，无负 available、无超 used、无无主 frozen。
5. outbox retry 不产生重复 outbox。
6. callback 终结后 grant_order、reservation、audit / reconciliation 语义一致。
7. 清理或隔离策略只影响本 runId，不得清空非本次 run 数据。

Test Agent 记录结果时应保存：

- 命令、退出码、runId、seed、profile、场景名。
- 每个场景的 total、success、duplicate、expected_reject、unexpected_error、timeout、retry、dlq。
- throughput、latency percentile、error rate。
- 一致性检查 PASS / FAIL 和差异摘要。
- 失败日志路径及脱敏检查结论。

## Decision

~~~text
PASS
~~~

需求：2026-05-27-engine-load-fault-tests
