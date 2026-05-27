# Analysis

## 需求 ID

2026-05-27-engine-e2e-postgres-postgresql

## Review 反馈处理

已读取 `tasks/2026-05-27-engine-e2e-postgres-postgresql/review.md`。该文件当前为模板态，没有真实 Review 打回内容：

| Review 项 | 处理 |
| --- | --- |
| `P0:` / `P1:` / `P2:` 均为空 | 无具体缺陷可逐条修复；本轮按高风险 PostgreSQL 端到端验收任务补齐完整需求口径。 |
| `Open Questions` 为“待填写” | 未形成 Review 阻塞问题；本轮主动关闭脚本命令、环境依赖、链路覆盖、失败输出、重复执行和非目标范围等关键歧义。 |
| `Decision` 为 `PASS / BLOCK` 模板 | 不视为真实 PASS；本轮重新输出可审核、可开发、可测试的 Analysis，并给出真实评分和结论。 |

## Analysis 自检修正

已读取上一轮 `analysis.md`。上一轮为模板态，存在 `Score 0`、`Decision PASS / BLOCK`、大量“待填写”，等价于 Analysis 自检未通过。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| `Score 0` | 已补充背景目标、参与方、输入输出、业务规则、边界条件、异常场景、非目标、风险、验收标准和待确认问题，并重新评分。 |
| `Decision PASS / BLOCK` 模板 | 已基于最小可验收口径给出明确 `Decision PASS`。 |
| 目标、范围、非目标未定义 | 已将任务限定为“真实 PostgreSQL 端到端验收脚本”，覆盖完整业务链路，但不修改业务语义、不新增后台页面、不依赖内存仓储。 |
| 缺少验收对象 | 已明确验收对象是一条可重复执行的命令或脚本，以及其在 Docker PostgreSQL、服务进程、Outbox worker、HTTP stub 和数据库状态上的可验证结果。 |
| 缺少默认口径 | 已按现有文档给出默认活动、事件、quota、reward、callback、对账查询和失败诊断口径，关闭旧 BLOCK。 |

本轮读取范围：`task.md`、`review.md`、上一轮 `analysis.md`、`docs/api.md`、`docs/data-model.md`、`docs/architecture.md`、`docs/workflow.md`、`docs/operations.md`、`docs/automation.md`，以及相关历史任务的 Analysis 文档用于对齐既有业务口径。本轮未读取实现代码，未写代码，未提交，未设计具体代码改法。

## 需求说明

本需求要求为 ExMarketing 活动引擎提供一个真实 PostgreSQL 端到端验收脚本。脚本必须以真实 PostgreSQL 仓储为准，串联活动配置、审批上线、quota、标准事件处理、发奖单、outbox 投递、reward callback、quota 终结和对账查询，证明核心活动引擎链路可以在接近本地生产形态的环境中闭环运行。

本需求不是单元测试补充，也不是管理后台页面测试。验收入口必须是一条命令可执行、可重复执行、可在失败时定位具体环节的自动化脚本或命令包装。

## 背景目标

ExMarketing 的核心链路为：

```text
标准事件接入
-> event inbox 幂等
-> 活动匹配与规则执行
-> reward 计算
-> quota 冻结
-> participation / grant_order / outbox
-> Outbox worker 发送
-> 外部发奖 callback
-> quota 确认或释放
-> 对账查询
```

现有文档明确服务支持两种 repository 模式：默认内存仓储，以及设置 `EX_MARKETING_DATABASE_URL` 后使用 PostgreSQL 仓储。本任务的目标是把核心链路的 PostgreSQL 模式做成可重复的端到端验收能力，避免只靠内存测试或人工点管理后台发现问题。

目标：

1. 一条命令启动或连接本地 PostgreSQL 验证环境，并完成端到端业务验收。
2. 验收必须使用 `EX_MARKETING_DATABASE_URL` 指向真实 PostgreSQL，不能退回内存仓储。
3. 脚本覆盖成功发奖和失败释放两条 callback 分支。
4. 脚本验证 participation、grant_order、outbox、quota reservation / aggregate、event inbox、audit / reconciliation 的关键状态。
5. 脚本可重复执行，不因上一次残留测试数据而误判。
6. 失败时输出具体失败环节、HTTP 请求、HTTP 响应、关键数据库状态和排查摘要。
7. 不依赖管理后台页面，只允许调用 HTTP API、命令行和数据库查询完成验收。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 验收脚本调用者 | 在本地或 CI 环境执行一条命令，读取最终 PASS / FAIL 和失败诊断。 |
| PostgreSQL | 保存活动、规则、奖励、quota、event inbox、participation、grant_order、outbox、audit 和 reservation 等真实状态。 |
| ExMarketing 服务 | 以 PostgreSQL repository 模式启动，提供 Admin API、OpenAPI Event API、Reward Callback API 和 reconciliation API。 |
| Outbox worker | 发送 HTTP / WEBHOOK reward command，将 outbox 从待发送推进到已发送或失败状态。 |
| Reward callback stub | 作为脚本控制的外部发奖系统替身，接收 outbox 请求，并按成功或失败用例回调 ExMarketing。 |
| 外部事件来源系统替身 | 由脚本直接调用 `POST /openapi/v1/events` 上报标准事件。 |
| Review / Dev / Test Agent | 基于本 Analysis 判断范围是否清晰、实现是否符合业务口径、测试是否覆盖 PostgreSQL 端到端场景。 |

## 输入输出

### 输入

脚本最小输入：

| 输入 | 默认口径 |
| --- | --- |
| PostgreSQL 连接串 | 通过 `EX_MARKETING_DATABASE_URL` 指定；默认可使用本地 Docker PostgreSQL：`postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。 |
| 服务地址 | 默认 `http://localhost:8080`；可通过环境变量覆盖。 |
| 是否启动 Outbox worker | 验收必须在 worker 开启条件下覆盖发送链路；默认要求 `EX_MARKETING_OUTBOX_WORKER=true`。 |
| OpenAPI 鉴权 | 如果环境启用 `EX_MARKETING_REQUIRE_OPENAPI_AUTH=true`，脚本必须使用有效 Bearer token；本地最小验收允许关闭鉴权，但不能因此跳过事件 API 幂等和业务校验。 |
| 测试租户 | 默认使用唯一测试租户前缀，例如 `e2e_pg_<runId>` 或固定 tenant 下唯一 activityCode / bizId，避免污染人工数据。 |
| 活动配置 | 使用普通活动，不使用插件；活动必须经过审批并激活后再处理正式事件。 |
| 标准事件 | 使用 `DEPOSIT_SUCCESS`、`payment`、`USER`、`DEPOSIT` 和唯一 `bizId`，attributes 至少包含 `amount`、`currency`、`isFirstDeposit`。 |
| quota 配置 | 至少覆盖 `BUDGET`、`STOCK`、`USER_LIMIT`、`DAILY_LIMIT`，并能验证冻结、确认和释放。 |
| reward callback secret | 若环境要求 callback 签名，脚本必须使用合法 HMAC；本任务不强制新增密钥管理能力。 |

### 输出

成功输出必须包含：

1. `PASS` 或等价明确成功标识。
2. 本次 runId / tenantId / activityId。
3. 创建的 quota 摘要。
4. 成功分支的 event inbox、participation、grant_order、outbox、quota 和 reconciliation 摘要。
5. 失败分支的 event inbox、participation、grant_order、outbox、quota 释放和 reconciliation 摘要。
6. 重复执行或重复事件幂等验证摘要。

失败输出必须包含：

1. 失败阶段名称，例如 `CREATE_ACTIVITY`、`SUBMIT_APPROVAL`、`APPROVE_ACTIVITY`、`ACTIVATE_ACTIVITY`、`CREATE_QUOTA`、`POST_EVENT_SUCCESS_CASE`、`OUTBOX_SEND`、`CALLBACK_SUCCESS`、`CALLBACK_FAILED`、`RECONCILIATION`。
2. 失败阶段的 HTTP method、path、请求体摘要、响应状态码和响应体。
3. 关键数据库状态快照：activity、quota、event inbox、participation、grant_order、outbox、reservation、audit 中与 runId 相关的记录。
4. 当前环境摘要：PostgreSQL DSN 是否设置、worker 是否开启、服务地址、测试 runId。敏感值必须脱敏。
5. 明确退出非 0 状态，便于 CI 识别。

## 业务规则

### 脚本执行规则

1. 脚本必须是一条命令可执行；允许这条命令内部启动依赖、等待健康检查、执行 HTTP 调用和查询数据库。
2. 脚本必须显式验证当前服务使用 PostgreSQL repository。仅运行内存仓储时必须失败并说明原因。
3. 脚本必须可重复执行。默认最小口径是每次生成唯一 `runId`，并将 `runId` 写入 activityCode、bizId、eventId、subjectId 或 metadata，所有查询按 `runId` 定位。
4. 脚本可以在本次 run 开始前清理同一 `runId` 的残留数据；不得无条件清空非本任务数据，除非命令名或参数明确表示本地测试库重置。
5. 脚本不得要求人工点击管理后台页面，不得通过浏览器页面完成活动创建、审批、激活或对账。
6. 脚本可以使用 Admin API 创建活动、创建 quota、提交审批、审批、更新活动状态和查询对账；这是 API 验收，不是页面依赖。
7. 脚本必须等待异步 outbox worker 达到期望状态，等待应有超时上限；超时必须输出当前 outbox、grant_order 和 worker 相关状态。
8. 脚本必须在成功分支和失败分支使用不同标准事件和不同业务幂等键，避免两个分支相互干扰。

### 活动配置和审批规则

1. 活动必须通过 Admin API 创建，包含 `tenantId`、唯一 `activityCode`、`activityName`、`activityType`、`status`、`triggerEvents`、`startTime`、`endTime`、`timezone`、`priority`、规则和奖励。
2. 活动初始状态默认使用 `DRAFT` 或可创建的未上线状态；不得绕过审批直接创建 `ACTIVE`，除非现有 API 明确允许且审批快照已满足上线要求。
3. 脚本必须提交审批，并完成达到上线所需的审批动作；普通活动最小口径为一级审批通过。
4. 脚本必须将活动更新为 `ACTIVE` 后再上报正式事件。
5. 正式事件处理只匹配审批通过、版本一致且 `status=ACTIVE` 的活动；`SCHEDULED`、`DRAFT`、审批未完成或版本不一致不得作为成功验收。

### 规则与奖励默认口径

1. 最小验收活动使用普通 `CONDITION` 规则，不使用 ActivityPlugin。
2. 规则应基于标准事件字段和 `attributes.*`，例如 `attributes.isFirstDeposit = true` 且 `attributes.amount >= 100.00`。
3. 奖励使用 `BONUS` 或 `CASH` 金额类奖励，默认 `RATIO` 或 `FIXED` 均可；金额必须可预期并能用于 BUDGET quota 验证。
4. 发奖通道使用 `HTTP` 或 `WEBHOOK`，`rewardConfig.targetRef` 指向脚本控制的本地 callback stub 或 HTTP 接收端。
5. 成功和失败分支可以复用同一活动，也可以创建两套活动；默认最小口径是同一活动、两个不同事件，外部 stub 根据请求或脚本步骤分别回调 `SUCCESS` 与 `FAILED`。

### Quota 规则

1. 脚本必须创建并验证以下 quota：
   - `BUDGET + ACTIVITY + TOTAL`
   - `STOCK + ACTIVITY + <rewardCode>`
   - `USER_LIMIT + USER + <subjectId>`
   - `DAILY_LIMIT + USER + <subjectId>:<activityLocalDate>`
2. 事件命中并生成可发奖 reward 时，participation、quota 冻结、grant_order 和 outbox 必须在同一 activity 级业务事务中形成一致状态。
3. 成功分支在 reward callback `SUCCESS` 后：
   - grant_order 进入 `SUCCESS`。
   - grant_order 级 BUDGET / STOCK reservation 从 `FROZEN` 进入 `CONFIRMED`。
   - quota 聚合从 `frozen_value` 转入 `used_value`。
   - participation 最终应体现已发奖或成功态。
4. 失败分支在 reward callback `FAILED` 后：
   - grant_order 进入 `FAILED`。
   - 对应 reservation 从 `FROZEN` 进入 `RELEASED`。
   - quota 聚合减少 `frozen_value`，不得增加 `used_value`。
   - participation 最终应体现失败态或可对账的失败结果。
5. 对于 `USER_LIMIT` / `DAILY_LIMIT`，多 reward 场景只按 participation 聚合确认或释放一次；本 E2E 默认使用单 reward，避免多 reward 规则成为脚本的前置复杂度。
6. 缺少 quota 行表示不限额是既有兼容规则，但本脚本的核心验收必须显式配置 quota，不能只验证不限额路径。

### Event API 与幂等规则

1. 脚本必须通过 `POST /openapi/v1/events` 上报标准事件，而不是直接写数据库。
2. `Idempotency-Key` 必须等于 `tenantId:sourceSystem:eventType:bizType:bizId`。
3. 成功分支和失败分支事件必须使用不同 `bizId`，以产生两条独立发奖链路。
4. 脚本必须至少验证一次重复事件：同一 payload 和同一 `Idempotency-Key` 再次上报应返回 `duplicate=true` 或等价幂等结果，且不得新增 participation、grant_order 或 outbox。
5. 相同 `Idempotency-Key` 但 payload 不一致属于事件 API 负向用例；本任务可作为扩展验收，不作为最小必过条件。

### Outbox worker 规则

1. 脚本必须验证 outbox 从待发送状态推进到发送完成或可观测发送结果。
2. HTTP / WEBHOOK target 返回 2xx 时，outbox 应进入 `SENT`。
3. Outbox worker 发送成功只表示发奖指令已到达外部发奖系统，不代表真实发奖成功；grant_order 最终态必须由 callback 决定。
4. 脚本必须能观测 worker 未处理、处理超时、target 未收到请求或 outbox 进入失败 / DLQ 的情况，并输出诊断。
5. 本脚本不要求覆盖 MQ broker 投递，除非现有环境已提供稳定 MQ 依赖；最小验收使用 HTTP / WEBHOOK。

### Reward callback 与对账规则

1. 脚本必须对成功分支发起或触发 `SUCCESS` callback，并验证 quota 确认。
2. 脚本必须对失败分支发起或触发 `FAILED` callback，并验证 quota 释放。
3. callback 定位必须限定在 `tenantId` 下，并使用 `billNo` / `commandId` 或 `idempotencyKey` 定位 grant_order。
4. 如果环境配置 callback secret，脚本必须按 `timestamp + "." + raw_body` 的 HMAC-SHA256 协议签名；签名失败用例可作为扩展负向验收。
5. 脚本必须调用 reconciliation API，并按 `tenantId + commandId`、`activityId`、`subjectId` 或 `eventId/idempotencyKey` 至少一种方式确认返回中包含 event inbox、participation、grant_order、outbox、quota reservation 和审计时间线。

## 边界条件

| 边界 | 预期 |
| --- | --- |
| PostgreSQL 未启动或连接失败 | 脚本失败，输出数据库连接错误和建议启动方式。 |
| 未设置 `EX_MARKETING_DATABASE_URL` | 脚本失败，明确说明不能使用内存仓储完成本验收。 |
| 服务未启动 | 脚本可按约定启动服务或失败提示；无论哪种，失败信息必须指向服务健康检查。 |
| Outbox worker 未开启 | 脚本必须失败或显式启动 worker；不得把未发送 outbox 当作通过。 |
| 活动审批未完成 | 激活或事件处理不得算成功；脚本应输出 approval 状态。 |
| 活动时间窗口不包含事件 `occurredAt` | 事件不会命中活动；脚本必须使用覆盖当前 run 的活动时间。 |
| 时区影响 `DAILY_LIMIT` scopeKey | 脚本必须按活动 timezone 的自然日构造或验证 `subjectId:activityLocalDate`。 |
| reward amount 为 0 | 不会生成 grant_order/outbox/quota 扣减；脚本必须避免零金额奖励作为核心成功路径。 |
| quota 不足 | 应返回或记录 `QUOTA_EXCEEDED` 并回滚 rewardable 业务写入；可作为扩展负向验收。 |
| 重复事件 | 不得新增 participation、grant_order 或 outbox。 |
| worker 发送成功但 callback 未到 | outbox 可为 `SENT`，grant_order 仍未最终成功；脚本必须等待 callback 或输出卡点。 |
| callback 与超时释放并发 | 非最小验收；若出现应以先终结者为准，脚本输出最终对账状态。 |

## 异常场景

1. 创建活动失败：输出请求、响应、activityCode、tenantId 和当前同名活动查询结果。
2. 审批提交或审批失败：输出 approval records 和 activity version，判断是否提交人审批自己、版本不一致或高危需要二级审批。
3. 激活失败：输出活动状态、approval 快照、version、startTime、endTime。
4. 创建 quota 失败：输出 quotaType、scope、scopeKey、totalValue 和 API 响应。
5. 事件上报失败：输出 Event API 响应、Idempotency-Key、标准事件摘要和 event inbox 查询结果。
6. 规则未命中或规则拒绝：输出 participation 的 `REJECTED` 状态、reject_reason、规则快照或 reconciliation 摘要。
7. quota 冻结失败：输出 `QUOTA_EXCEEDED`、quota 聚合值、reservation 查询结果和是否残留 grant_order/outbox。
8. grant_order 未生成：输出 participation 状态、reward 计算摘要、零金额检查和 quota 状态。
9. outbox 未生成或未发送：输出 grant_order、outbox 状态、worker 开关、retry_count、next_retry_at、error_message。
10. callback 成功分支未确认 quota：输出 callback 响应、grant_order 状态、reservation 状态、quota used/frozen。
11. callback 失败分支未释放 quota：输出 callback 响应、grant_order 状态、reservation 状态、quota used/frozen。
12. reconciliation 缺少关键对象：输出查询参数、响应体和数据库链路快照。
13. 清理或重复执行异常：输出 runId、残留对象列表和脚本是否选择隔离 runId 继续执行。

## 非目标范围

1. 不新增或修改活动引擎业务语义。
2. 不新增活动类型、奖励类型、规则 evaluator、quota 类型、ActivityPlugin 或外部 Fact Provider。
3. 不实现管理后台页面自动化，不依赖 `/admin/` 页面点击。
4. 不把内存仓储测试作为本需求验收对象。
5. 不要求覆盖真实 MQ broker、Kafka、RabbitMQ 或 Pulsar 投递；HTTP / WEBHOOK outbox 为最小验收通道。
6. 不要求覆盖压力测试、并发压测、故障注入、worker 多实例 `SKIP LOCKED` 竞争；这些属于 load / fault 类任务。
7. 不要求测试所有 reward calculateType；最小验收只需一个金额类可预测奖励。
8. 不要求实现生产级密钥轮换、Admin 鉴权、外部钱包真实到账或人工补单。
9. 不允许通过直接插入业务表替代 API 链路；数据库查询只用于验证和诊断。
10. 不要求无条件清空整个本地数据库；清理范围必须限制在本次 run 或显式测试库重置参数内。

## 风险点

| 风险 | 影响 | 约束 |
| --- | --- | --- |
| 脚本误用内存仓储 | 无法验证 PostgreSQL transaction、reservation、worker claim 和查询语义 | 必须检测 `EX_MARKETING_DATABASE_URL` 并验证数据库中出现本次 run 数据。 |
| 重复执行污染数据 | 后续 run 误判或破坏人工数据 | 使用唯一 runId，清理只限本次 run 相关数据。 |
| 异步 worker 不稳定 | E2E 偶发失败或卡死 | 所有等待必须有超时、轮询间隔和状态输出。 |
| callback stub 与 outbox 协议不一致 | outbox SENT 但 callback 未执行，链路不闭环 | 明确 stub 接收 reward command 后由脚本控制 callback，或脚本读取 grant_order 后主动 callback。 |
| quota 验证不完整 | 只验证发奖单，不验证预算/库存/次数一致性 | 成功和失败分支必须分别断言 used/frozen/reservation 状态。 |
| 通过页面或直接 DB 写绕过业务链路 | 验收无法代表真实 API 使用 | 创建、审批、激活、事件、callback、对账必须走 HTTP API；DB 只用于验证。 |
| 日志泄露敏感数据 | token、签名、secret 或 payload 泄露 | 失败输出和审计摘要必须脱敏。 |
| 环境端口冲突 | 本地执行不稳定 | 脚本应支持服务地址、stub 地址和数据库连接串覆盖。 |

## 验收标准

功能验收：

1. 提供一条命令可运行 PostgreSQL 端到端验收，命令退出码能准确表示 PASS / FAIL。
2. 运行时必须使用真实 PostgreSQL；未设置或无法连接 PostgreSQL 时失败。
3. 脚本可重复执行两次，第二次不会因第一次数据残留失败，也不会重复消费同一业务幂等键。
4. 脚本不依赖管理后台页面，不需要人工点击。
5. 脚本通过 API 创建活动、创建 quota、提交审批、审批通过、激活活动。
6. 活动激活后，标准事件成功进入 event inbox，并生成预期 participation。
7. 成功分支生成 grant_order 和 outbox，outbox worker 将 HTTP / WEBHOOK outbox 推进到 `SENT` 或等价已发送状态。
8. 成功分支 callback `SUCCESS` 后，grant_order 最终成功，BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT reservation 进入确认口径，quota `frozen_value` 下降且 `used_value` 按预期增加。
9. 失败分支生成独立 grant_order 和 outbox，callback `FAILED` 后 grant_order 最终失败，相关 reservation 释放，quota `frozen_value` 下降且 `used_value` 不因失败分支增加。
10. 重复上报同一标准事件返回幂等结果，不新增 participation、grant_order 或 outbox。
11. 对账查询能查到本次 run 的 event inbox、participation、grant_order、outbox、callback 信息、quota reservation 和审计时间线。
12. 任一阶段失败时，脚本输出具体阶段、请求、响应、数据库状态快照和环境摘要。

一致性验收：

13. participation、quota 冻结、grant_order 和 outbox 在事件成功处理后状态一致；quota 不足或规则拒绝时不得留下半成品发奖链路。
14. `SENT` outbox 不等同 grant_order 成功，最终成功/失败必须由 callback 决定。
15. 所有查询和 callback 定位都必须包含 tenant 维度，不允许跨 tenant 误判。
16. 失败输出不得包含完整 token、secret、Authorization header、签名或敏感配置。

测试验收：

17. Test Agent 至少运行 `go test ./...` 和 `go build ./...`。
18. Test Agent 必须运行本 PostgreSQL E2E 命令，并记录命令、环境变量摘要、结果和失败诊断。
19. 涉及 PostgreSQL 的验收必须使用 Docker PostgreSQL 或等价真实 PostgreSQL 环境，不能只用内存 repository。
20. 如果 CI 或本地环境缺少 Docker/PostgreSQL，Test Agent 必须将其记录为环境阻塞，而不是把未运行当作 PASS。

## 待确认问题

以下问题不阻塞进入研发，已按默认最小可验收口径关闭：

| 问题 | 默认最小口径 |
| --- | --- |
| 脚本文件名和命令名未指定 | Dev 可按项目既有脚本/命令组织方式选择，但必须保证一条命令可执行，并在文档或任务产物中写明。 |
| 是否由脚本启动服务和 PostgreSQL | 默认可连接既有服务，也可由命令包装启动；验收重点是命令可重复跑通真实 PostgreSQL 链路。 |
| callback stub 如何实现 | 默认使用脚本控制的本地 HTTP stub 或读取 grant_order 后主动调用 callback；不得依赖真实外部钱包。 |
| 是否要求 callback 签名 | 若当前环境配置 secret 则必须签名；最小 PostgreSQL E2E 不因未配置 secret 而 BLOCK，但不能破坏 reward callback 既有签名规则。 |
| 是否覆盖 MQ outbox | 默认不覆盖，HTTP / WEBHOOK 为最小验收通道。 |
| 是否清空数据库 | 默认不清空全库，只隔离 runId 或清理本次 run 数据；显式本地重置命令可另行提供。 |
| 是否覆盖 quota 不足负向路径 | 可作为增强；最小必过聚焦成功确认、失败释放和重复事件幂等。 |

## Score

```text
92
```

评分：

| 维度 | 分数 | 说明 |
| --- | --- | --- |
| 需求清晰度 | 19/20 | 已明确验收对象、一条命令、PostgreSQL 模式、API 链路和失败输出。 |
| 业务规则完整 | 18/20 | 覆盖活动审批、事件、quota、outbox、callback、对账；未强制所有 reward / MQ 组合，符合最小可验收范围。 |
| 边界异常清楚 | 18/20 | 覆盖环境、幂等、异步 worker、审批、quota、callback、对账异常。 |
| 验收标准明确 | 19/20 | 给出 20 条功能、一致性和测试验收标准，可由 Test Agent 执行。 |
| 约束一致性 | 18/20 | 与现有 API、数据模型、运维文档一致，明确不读页面、不走内存、不改业务语义。 |

## Decision

```text
PASS
```
