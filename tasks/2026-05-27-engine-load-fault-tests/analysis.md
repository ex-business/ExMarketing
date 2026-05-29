# Analysis: 2026-05-27-engine-load-fault-tests

## 资料范围

本轮只做需求分析和需求细化，未读取实现代码，未写代码，未提交，未设计具体代码改法。

已读取：

- `tasks/2026-05-27-engine-load-fault-tests/review.md`
- `tasks/2026-05-27-engine-load-fault-tests/analysis.md` 上一版
- `tasks/2026-05-27-engine-load-fault-tests/task.md`
- `docs/api.md`
- `docs/data-model.md`
- `requirements/backlog.json` 中本需求和相邻活动引擎任务条目
- 相邻任务 Analysis：PostgreSQL E2E、engine metrics / alerts，用于对齐既有链路和可观测输出口径

未发现独立 PRD 目录。本分析以任务说明、现有 API / 数据模型和已提交相邻任务的业务口径为依据。

## Review 反馈处理

`review.md` 当前为模板态，但按本任务要求仍逐条回应：

| Review 项 | 处理 |
| --- | --- |
| `P0:` / `P1:` / `P2:` 均为空 | 未发现真实打回缺陷；本轮按高风险压测和故障注入任务补齐场景、输入输出、业务规则、边界异常、验收标准和默认最小口径。 |
| `Open Questions` 为“待填写” | 视为 Review 尚未形成具体问题；本轮不等待 Owner 决策，默认采用本地 PostgreSQL + HTTP / WEBHOOK outbox + 可控 callback stub 的最小验收口径。 |
| `Decision` 为 `PASS / BLOCK` 模板 | 不视为真实 PASS；本轮重新评分并给出明确 `Decision PASS`。 |

## Analysis 自检修正

上一版 `analysis.md` 是模板态，存在 `Score 0`、`Decision PASS / BLOCK` 和大量“待填写”，等价于 Analysis 自检未通过。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| 背景、目标、参与方、输入输出、业务规则、边界异常、非目标、风险、验收方式均为空 | 本轮全部补齐，并限定为“可本地运行的活动引擎压测与故障注入验收脚本”。 |
| `Score = 0` | 已形成可审核、可开发、可测试需求，重新评分为 91。 |
| `Decision = PASS / BLOCK` 模板 | 已给出 `Decision PASS`。 |
| 缺少默认口径 | 已按最小可验收原则确定默认压测规模、场景矩阵、输出指标、数据一致性检查、失败日志和环境约束。 |
| 旧隐含 BLOCK：没有 Owner 决策无法确定吞吐阈值、并发规模、故障方式 | 已关闭。吞吐以“必须输出和可比较”为强制项，不设置跨机器绝对 SLA；最小 profile 用固定小规模保证本地可跑，stress profile 可配置放大。 |

## 需求说明

本需求要求为 ExMarketing 活动引擎补充可本地运行的压测和故障注入验收能力，覆盖并发事件处理、quota 并发扣减、outbox 重试、callback 重复 / 乱序、PostgreSQL 重启或网络超时。脚本必须输出吞吐、错误率、延迟、数据一致性检查结果，并在失败时提供可复现日志。

本需求不是重写业务逻辑，也不是只补单元测试。验收对象是可执行脚本或命令集合，能在真实 PostgreSQL 模式下压测活动引擎核心链路，并证明故障期间不会产生重复发奖、quota 漂移、outbox 丢失、callback 二次终结或不可复现失败。

## 背景目标

活动引擎核心链路为：

```text
Event API
-> event inbox 幂等
-> 活动匹配 / 规则执行
-> reward 计算
-> quota 冻结
-> participation / grant_order / outbox
-> Outbox worker 投递
-> reward callback
-> quota 确认或释放
-> reconciliation / audit
```

此前 PostgreSQL E2E 任务验证单条闭环是否正确；metrics / alerts 任务定义运行观测口径。本任务在此基础上验证并发、重试、重复、乱序和数据库故障下的稳定性。

目标：

1. 提供一条或少量明确命令，能在本地真实 PostgreSQL 环境运行压测和故障注入。
2. 覆盖任务要求的五类场景：并发事件、quota 并发扣减、outbox 重试、callback 重复 / 乱序、PostgreSQL 重启或网络超时。
3. 每个场景输出吞吐、延迟、错误率和业务一致性检查。
4. 失败时输出 runId、随机 seed、环境摘要、请求 / 响应摘要、关键数据库状态和复现命令。
5. 不依赖管理后台页面，不通过直接写业务表替代正式 API 链路。
6. 默认 profile 能在开发机本地跑完；更大规模 profile 通过参数配置，不作为默认必过门槛。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 压测脚本调用者 | 执行本地命令，获取 PASS / FAIL、指标摘要和失败诊断。 |
| ExMarketing 服务 | 以 PostgreSQL repository 模式提供 Admin API、Event API、reward callback、reconciliation 和 outbox worker。 |
| PostgreSQL | 保存活动、quota、event inbox、participation、grant_order、outbox、reservation、audit 等真实状态，是一致性验证依据。 |
| Outbox worker | 并发 claim 和投递 outbox，接受 retry、DLQ、恢复后继续处理等场景验证。 |
| 外部 reward stub | 作为 HTTP / WEBHOOK 发奖目标，可按脚本控制返回成功、失败、超时、5xx，并发送 callback。 |
| 事件来源系统替身 | 由脚本并发调用 `POST /openapi/v1/events` 上报标准事件。 |
| Dev / Test Agent | 基于本 Analysis 实现脚本、执行场景、记录结果并判断是否满足验收。 |

## 输入输出

### 输入

| 输入 | 默认最小口径 |
| --- | --- |
| PostgreSQL 连接串 | 必须通过 `EX_MARKETING_DATABASE_URL` 指向真实 PostgreSQL；默认本地示例为 `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。 |
| 服务地址 | 默认 `http://localhost:8080`，允许环境变量覆盖。 |
| Outbox worker | 必须开启或由脚本启动；未开启不得判 PASS。 |
| reward stub 地址 | 默认由脚本启动本地 HTTP stub，或连接已启动 stub；端口可覆盖。 |
| runId | 默认自动生成；也允许传入固定 runId 用于复现。 |
| random seed | 默认自动生成并打印；失败复现必须支持传入同一 seed。 |
| profile | 默认 `local`；可扩展 `stress` / `soak`，但本需求最小验收以 `local` 为准。 |
| 并发规模 | `local` 默认：并发事件 500 条、并发度 25；quota contention 200 条、并发度 50、quota 容量 50；outbox retry 50 条；callback 重复 / 乱序每个 grant order 至少 3 次 callback 尝试；PostgreSQL 故障场景 100 条事件。 |
| 鉴权 / 签名 | 若当前环境启用 OpenAPI 鉴权或 callback secret，脚本必须使用合法 token / HMAC；本地最小口径允许关闭鉴权，但不能跳过业务校验。 |

### 输出

每次运行必须输出机器可读或稳定结构化摘要，至少包含：

1. `PASS` / `FAIL`、退出码、runId、seed、profile、开始 / 结束时间。
2. 每个场景的 total、success、duplicate、expected_reject、unexpected_error、timeout、retry、dlq 数量。
3. 吞吐：events/sec、outbox/sec、callbacks/sec，按场景输出。
4. 延迟：Event API、outbox command、callback 的 p50 / p95 / p99 或等价 percentile。
5. 错误率：`unexpected_error / total`，业务预期拒绝如 quota exhausted、duplicate 不计入 unexpected error。
6. 一致性检查摘要：event inbox、participation、grant_order、outbox、quota aggregate、quota reservation、audit / reconciliation 的计数和差异。
7. 失败日志位置或文件路径；日志必须包含复现命令、环境摘要、关键请求 / 响应、数据库快照摘要，且脱敏 token、secret、Authorization、签名和完整敏感 payload。

## 场景矩阵

### S1 并发事件压测

目的：验证 Event API 并发接入、幂等键、活动匹配、reward 计算、quota 冻结、grant_order 和 outbox 写入在真实 PostgreSQL 下不会产生丢失、重复或半成品。

默认输入：

- 一个已审批并激活的普通活动。
- 一个金额类 `BONUS` / `CASH` 奖励，使用 HTTP / WEBHOOK outbox。
- 500 个唯一标准事件，`eventType=DEPOSIT_SUCCESS`，`sourceSystem=payment`，`bizType=DEPOSIT`，每个事件拥有唯一 `bizId` 和正确 `Idempotency-Key`。
- 并发度 25。

预期：

- 所有合法事件返回成功或可解释的业务结果。
- 每个唯一事件最多产生一条 event inbox、一条命中 participation、一条 grant_order 和一条 outbox。
- 不出现 panic、死锁、数据竞争、不可解析响应或无诊断 5xx。
- 输出吞吐、延迟、错误率和总计数。

### S2 quota 并发扣减压测

目的：验证高并发争抢有限 quota 时，BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT 的冻结和聚合不会超扣、负数、漂移或留下半成品。

默认输入：

- 同一活动显式配置 quota。
- local profile 必须按独立子用例分别覆盖 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT` 四种 quota，每个子用例只让一种 quota 成为瓶颈，其余 quota 保持充足，避免因检查顺序掩盖未覆盖类型。
- 200 个并发事件争抢容量为 50 的业务 quota。
- 并发度 50。
- 默认单 reward，金额固定且非零；STOCK 使用固定 rewardCode，USER_LIMIT / DAILY_LIMIT 使用固定 subjectId 和日期 scopeKey。

预期：

- 成功发奖链路数量不超过 quota 容量。
- 超出容量的事件必须得到明确 `QUOTA_EXCEEDED` 或等价业务拒绝。
- quota 不足事件不得留下 `REWARDABLE` participation、grant_order、outbox 或 FROZEN reservation。
- 成功事件的 frozen / used / available 聚合与 reservation 明细一致。
- 总 quota 不得出现负 available、used 超 total、frozen 无对应 reservation。
- 测试报告必须按 quota 类型分别输出 total、success、expected_reject、unexpected_error 和一致性检查结果；只覆盖 BUDGET 不得判定 S2 完成。

### S3 outbox 重试压测

目的：验证 reward target 临时失败、超时、恢复后，outbox retry / next retry / DLQ 或最终 SENT 语义正确，且不会重复生成 grant_order 或重复扣 quota。

默认输入：

- 50 个成功生成 outbox 的事件。
- reward stub 前 N 次返回 5xx 或超时，之后恢复 2xx；N 默认 1 到 2 次，可配置。
- 如果实现存在最大重试和 DLQ 阈值，另设一小批始终失败样本验证进入 DLQ。

预期：

- 临时失败 outbox 保留原 payload、commandId / billNo，不创建新 outbox 替代旧 outbox。
- retry 期间 send_status、retry_count、next_retry_at、error_message 可观测。
- target 恢复后可发送成功并进入 `SENT` 或等价已发送状态。
- 永久失败样本按既有规则进入 `DLQ`，不会丢失诊断信息。
- outbox `SENT` 只代表指令已投递，不自动等同 grant_order 成功；最终态仍由 callback 决定。

### S4 callback 重复 / 乱序压测

目的：验证外部发奖回调在重复、乱序、冲突情况下幂等终结，不会二次确认或二次释放 quota。

默认输入：

- 至少 30 个已生成 grant_order 的样本。
- 对成功样本发送 `SUCCESS` callback，然后重复发送相同 `SUCCESS`。
- 对失败样本发送 `FAILED` callback，然后重复发送相同 `FAILED`。
- 对乱序 / 冲突样本先发送一种最终态，再发送相反最终态；默认以首次合法最终态为准，后续相反状态应被拒绝或标记 conflict。

预期：

- 完全重复 callback 返回幂等成功或 duplicate，不重复移动 quota。
- 相反最终态 callback 不得覆盖已完成 grant_order，不得二次移动 quota。
- `externalGrantId` 冲突、tenant 不匹配、未知 commandId / billNo 必须得到机器可识别错误。
- SUCCESS 样本 reservation 仅从 `FROZEN` 到 `CONFIRMED` 一次。
- FAILED 样本 reservation 仅从 `FROZEN` 到 `RELEASED` 一次。

### S5 PostgreSQL 重启或网络超时故障注入

目的：验证数据库短暂不可用、连接中断或超时期间，Event API、outbox worker、callback 处理不会产生不可恢复半状态；恢复后幂等重试能收敛到一致结果。

默认输入：

- 100 个事件，分批并发提交。
- 在提交和 worker 处理期间注入一次 PostgreSQL 重启、连接断开或网络超时。若本地环境不允许直接重启 PostgreSQL，最小可接受替代是通过可控 proxy / firewall / DSN 连接超时模拟短暂数据库不可用。
- 故障持续时间默认 3 到 10 秒，必须在输出中记录。

预期：

- 故障窗口内请求可以失败，但必须是可解释的数据库不可用 / timeout / 5xx，不得产生静默成功。
- 恢复后使用同一 event idempotency key 重试，不得重复生成 participation、grant_order、outbox 或重复扣 quota。
- worker 恢复后能继续处理 eligible outbox；stuck `SENDING` 如存在必须可恢复或被明确报告。
- 数据一致性检查必须证明没有 orphan grant_order、orphan outbox、无 reservation 的 frozen quota、无 outbox 的待发 grant_order 等异常。

## 业务规则

1. 所有核心场景必须使用真实 PostgreSQL；内存仓储结果不能作为本需求通过依据。
2. 活动创建、审批、激活、事件提交、callback 和对账必须走 HTTP API；数据库只用于一致性验证和失败诊断。
3. 测试数据必须用 runId 隔离，写入 tenantId、activityCode、eventId、bizId、subjectId 或 metadata 中，便于清理和复现。
4. 脚本不得无条件清空非本次 run 的业务数据；允许清理同 runId 的残留数据或要求使用专用本地测试库。
5. 业务预期结果与系统错误必须分开统计。`duplicate`、`QUOTA_EXCEEDED`、已知 callback conflict 属于预期业务结果；panic、无响应、不可解释 5xx、数据不一致属于失败。
6. 并发事件的 `Idempotency-Key` 必须符合文档：`tenantId:sourceSystem:eventType:bizType:bizId`。
7. 同一标准事件命中同一活动最多生成一条 participation；同一 participation / reward 最多生成一条 grant_order 和一条 outbox。
8. participation、quota 冻结、grant_order、outbox 必须保持同一 activity 级事务一致性；quota 不足不得留下半成品发奖链路。
9. callback 是 grant_order 最终成功 / 失败的唯一业务终结来源之一；outbox 发送成功不代表发奖成功。
10. 重复 callback、重复事件、重试 outbox 均不得导致 quota 二次移动。
11. PostgreSQL 故障后可以有请求失败，但恢复和重试后系统必须收敛；不要求所有故障窗口请求都成功。
12. 所有失败日志和输出必须脱敏，不得打印完整 Authorization、token、secret、callback signature、broker password 或外部凭据。

## 数据一致性检查

每个场景结束后必须执行一致性检查，至少覆盖：

1. event inbox：唯一 event idempotency key 只有一条记录；重复事件不新增业务链路。
2. participation：每个成功命中事件最多一条 participation；quota 不足或规则拒绝不留下 `REWARDABLE` 脏记录。
3. grant_order：每条 grant_order 必须关联有效 participation、activity、tenant、subject。
4. outbox：自动发奖 grant_order 必须有对应 outbox；重试不创建额外 outbox；DLQ / SENT 状态可解释。
5. quota reservation：`FROZEN`、`CONFIRMED`、`RELEASED` 数量和金额与 grant_order 最终态一致。
6. quota aggregate：`used_value + frozen_value <= total_value`；`available_value` 不为负；reservation 汇总与 aggregate 差异为 0。
7. callback：重复和冲突 callback 不改变已终结 grant_order 和 quota。
8. reconciliation：按 tenant + commandId / activityId / subjectId / idempotencyKey 至少一种查询方式能串起 event inbox、participation、grant_order、outbox、reservation 和 audit。
9. orphan 检查：不得存在本 runId 下 orphan outbox、orphan grant_order、无 reservation 的 frozen quota、无业务对象的 audit 关键记录。

## 边界条件

| 边界 | 预期 |
| --- | --- |
| 未设置 `EX_MARKETING_DATABASE_URL` | 脚本失败，明确说明本需求不能用内存仓储验收。 |
| PostgreSQL 不可连接 | 脚本失败并输出连接错误、DSN 脱敏摘要和建议启动方式。 |
| 服务未启动或 `/healthz` 不通 | 脚本失败或按约定启动服务；不得继续压测并误判。 |
| Outbox worker 未开启 | outbox 场景失败，输出 worker 开关、outbox backlog 和处理超时。 |
| reward stub 不可达 | outbox 应进入 retry / failed / DLQ；脚本输出 target 错误并按场景判断。 |
| 本地机器性能较低 | 默认不要求绝对吞吐 SLA，但必须输出吞吐和错误率；数据一致性仍为硬门槛。 |
| quota 容量设置过小 | 超出容量必须业务拒绝，不得系统错误或超扣。 |
| callback 到达早于 outbox SENT | 只要能合法定位 grant_order，应按 callback 业务规则终结；若系统不允许，必须给出机器可识别错误且不移动 quota。 |
| PostgreSQL 故障期间请求失败 | 允许，但必须可解释；恢复后重试必须幂等一致。 |
| 脚本中断后再次运行 | 使用新 runId 不受影响；使用同 runId 应能清理或报告残留并给出复现路径。 |

## 异常场景

1. 活动准备失败：输出活动创建、审批、激活请求和响应，以及 activity / approval 当前状态。
2. quota 准备失败：输出 quotaType、scope、scopeKey、totalValue、响应体。
3. 并发事件出现 unexpected error：输出样本请求、响应、idempotency key、event inbox 查询和服务错误摘要。
4. quota 超扣或漂移：输出 quota aggregate、reservation 明细、相关 participation / grant_order 列表和差异计算。
5. outbox retry 未发生：输出 outbox 状态、retry_count、next_retry_at、worker 状态、stub 收包记录。
6. outbox 重复发送无法解释：输出 commandId、target 请求次数、outbox 更新历史或 audit 摘要。
7. callback 重复导致二次移动：输出 callback 请求序列、grant_order 最终态、reservation 状态变化和 quota 差异。
8. callback 乱序覆盖最终态：输出首次最终态、后续冲突响应和最终 reconciliation。
9. PostgreSQL 故障后出现半状态：输出故障时间线、失败请求列表、恢复后重试结果和 orphan 检查。
10. 脚本自身超时：输出最后一个完成阶段、仍在等待的对象、轮询条件和复现命令。

## 非目标范围

1. 不修改活动引擎业务语义、状态机、quota 规则、callback 协议或 outbox retry 策略。
2. 不新增活动类型、奖励类型、规则 evaluator、quota 类型或 ActivityPlugin。
3. 不建设完整性能基准平台、Grafana 看板、长期压测服务或生产压测流程。
4. 不设置跨机器固定吞吐 SLA；本需求只要求输出可比较指标和通过一致性硬门槛。
5. 不要求覆盖真实 Kafka / RabbitMQ / Pulsar broker；默认用 HTTP / WEBHOOK outbox 完成最小验收。
6. 不要求覆盖所有 reward calculateType；默认使用一个金额类非零奖励。
7. 不要求用浏览器或管理后台页面执行。
8. 不允许通过直接插入业务表替代 API 链路；DB 查询仅用于验证和诊断。
9. 不要求在生产环境执行故障注入；本需求范围限定本地或 CI 可控环境。
10. 不要求解决压测发现的所有业务缺陷；脚本必须能暴露并复现缺陷，后续修复由 Dev 流程处理。

## 风险点

| 风险 | 影响 | 约束 |
| --- | --- | --- |
| 本地压测误用内存仓储 | 无法验证事务、锁、worker claim 和故障恢复 | 必须检测 PostgreSQL 连接并验证本 run 数据落库。 |
| 绝对吞吐阈值不适合所有开发机 | 好机器和慢机器结果不可比，容易误判 | 默认不设固定 req/s 门槛；强制输出指标和一致性结果。 |
| 故障注入破坏共享数据库 | 影响开发者数据 | 默认要求专用本地库或 runId 隔离；不得无条件清全库。 |
| 异步 worker 导致偶发 | 场景超时或状态未收敛 | 所有等待必须有超时、轮询条件和失败快照。 |
| callback / outbox stub 与真实协议偏差 | 验收无法覆盖真实链路 | stub 必须接收真实 outbox payload，并通过正式 callback API 终结 grant_order。 |
| 日志泄密 | token、secret、签名泄露 | 输出和日志必须脱敏。 |
| 压测脚本只看 HTTP 成功 | 数据已经漂移但脚本通过 | 数据一致性检查是硬门槛，HTTP 2xx 不足以 PASS。 |

## 验收标准

功能验收：

1. 提供可本地运行的压测 / 故障注入命令，至少支持一键运行默认 `local` profile。
2. 命令必须使用真实 PostgreSQL；未配置或不可连接时失败。
3. 默认 profile 覆盖 S1 并发事件、S2 quota 并发扣减、S3 outbox 重试、S4 callback 重复 / 乱序、S5 PostgreSQL 重启或网络超时。
4. 每个场景输出 total、success、expected_reject、unexpected_error、error_rate、吞吐和延迟。
5. 每个场景结束后执行数据一致性检查，任何差异都使脚本 FAIL。
6. 失败时输出可复现 runId、seed、profile、命令、环境摘要、关键请求 / 响应和数据库状态快照。
7. 脚本可连续运行两次，第二次不会因第一次残留数据误判。
8. 所有敏感信息在日志和摘要中脱敏。

场景验收：

9. S1 中 500 个唯一并发事件不得生成重复 participation / grant_order / outbox。
10. S2 中成功发奖链路数量不得超过 quota 容量，quota aggregate 和 reservation 汇总必须一致。
11. S3 中临时失败 outbox 必须按 retry 语义恢复发送或进入可解释失败 / DLQ，不得创建替代 outbox。
12. S4 中重复 callback 不得二次移动 quota；相反最终态 callback 不得覆盖已完成 grant_order。
13. S5 中 PostgreSQL 故障窗口允许请求失败，但恢复后同 idempotency key 重试必须收敛且无半状态。

测试验收：

14. Test Agent 必须运行 `go test ./...` 和 `go build ./...`。
15. Test Agent 必须运行默认压测 / 故障注入命令，并在 `test.md` 记录命令、环境摘要、结果和失败诊断。
16. 如果本地缺少 Docker / PostgreSQL / 故障注入能力，Test Agent 必须记录为环境阻塞或降级执行结果，不能把未运行当作 PASS。
17. 若实现提供 `stress` profile，Test Agent 可选运行；默认 `local` profile 是必验。

## 待确认问题

以下问题不阻塞进入研发，已按默认最小可验收口径关闭：

| 问题 | 默认最小口径 |
| --- | --- |
| 脚本文件名和命令名未指定 | Dev 可按项目既有脚本组织方式选择，但必须提供一条默认命令并在产物中记录。 |
| 吞吐是否需要固定 SLA | 不设置跨机器固定 SLA；必须输出吞吐、延迟和错误率，一致性为硬门槛。 |
| 故障注入方式未指定 | 优先 PostgreSQL restart；不可行时使用网络断开 / timeout proxy 模拟，并输出具体方式。 |
| 是否覆盖 MQ outbox | 默认不覆盖，HTTP / WEBHOOK 是最小验收通道。 |
| 是否需要长期 soak | 不作为最小验收；可扩展 profile。 |
| 是否清库 | 默认不清全库，只使用 runId 隔离或清理本 run 数据；建议专用本地测试库。 |
| callback 签名是否强制 | 若环境启用 secret 则必须签名；未启用时不阻塞默认本地验收。 |

## Score

```text
91
```

评分：

| 维度 | 分数 | 说明 |
| --- | --- | --- |
| 需求清晰度 | 18/20 | 明确交付物、场景、参与方、默认 profile 和非目标；脚本命名留给 Dev 按项目约定决定。 |
| 业务规则完整 | 19/20 | 覆盖事件幂等、quota、outbox、callback、PostgreSQL 故障和一致性规则。 |
| 边界异常清楚 | 18/20 | 覆盖环境缺失、worker、stub、故障窗口、低性能机器和重复执行。 |
| 验收标准明确 | 18/20 | 有功能、场景和测试验收；吞吐不设绝对 SLA 是为了避免本地硬件误判。 |
| 约束一致性 | 18/20 | 与现有 API / 数据模型一致，限定真实 PostgreSQL、HTTP API 链路和脱敏输出。 |

## Decision

```text
PASS
```

需求：2026-05-27-engine-load-fault-tests
