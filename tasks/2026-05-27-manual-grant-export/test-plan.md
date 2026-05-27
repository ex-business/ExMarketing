# Test Plan

## Agent 指令

你是 Test Agent。请在 Dev Agent 完成 implementation.md 且列出变更后执行本测试计划。

本计划只定义测试用例和命令清单。Test Agent 执行时应记录实际命令、结果、失败日志摘要和结论到 `test.md`。

## Test Strategy

- 覆盖两条主链路：`MANUAL` 人工确认 / 失败 / 关闭，以及 `MANUAL_EXPORT` 导出创建 / READY / 逐单确认 / 关闭。
- 覆盖 quota 分层终结：`BUDGET` / `STOCK` 按 grant order 终结，`USER_LIMIT` / `DAILY_LIMIT` 按 participation 下所有 grant order 聚合后一次性终结。
- 覆盖 review 指出的口径：`MANUAL_EXPORT` 成功确认默认应来自已 READY 导出文件内的 `SENDING` 单据；纯 `MANUAL` 动作的审计不应伪造 `exportId`。
- 覆盖错误路径：quota 不足、非法状态流转、locator 冲突、跨 tenant、空导出、文件生成失败、自动流程误处理人工单。
- 覆盖幂等和并发：重复事件、重复确认、并发确认、并发导出、导出与确认并发。
- PostgreSQL 或等价持久化仓储必须作为最终 smoke，不能只以内存仓储通过作为验收依据。

## Test Cases

### 1. 正常路径 - `MANUAL`

- 活动创建接受 `grantMode=MANUAL`，`targetRef` 可为空或普通说明字段；未知 `grantMode` 仍被拒绝。
- 正式事件命中 `MANUAL` reward 后生成 event inbox、participation、quota reservation、grant order，grant order 初始 `PENDING`，不生成 reward command outbox。
- 人工确认 `SUCCESS`：
  - grant order 进入 `SUCCESS`。
  - 对应 reward-level `BUDGET` / `STOCK` 从 `FROZEN` 到 `CONFIRMED`。
  - 单 reward participation 下 `USER_LIMIT` / `DAILY_LIMIT` 聚合到 `CONFIRMED`。
  - 写成功确认审计，审计包含 operator、reason、grantOrderId / billNo、前后状态和 quota 摘要；`exportId` 应为 null / absent。
- 人工确认 `FAILED`：
  - grant order 进入 `FAILED`。
  - reward-level reservation 释放为 `RELEASED`。
  - 单 reward participation 下 `USER_LIMIT` / `DAILY_LIMIT` 聚合释放为 `RELEASED`。
  - 审计 action 能区分失败确认，不与关闭混淆。
- 人工关闭 `CLOSED`：
  - grant order 进入 `CLOSED`。
  - reward-level reservation 释放。
  - 单 reward participation 下 participation-level reservation 释放。
  - 关闭原因进入审计和可对账字段。

### 2. 正常路径 - `MANUAL_EXPORT`

- 活动创建接受 `grantMode=MANUAL_EXPORT`，事件命中后生成 participation、quota reservation、grant order，grant order 初始 `PENDING`，不生成 reward command outbox。
- 创建导出时只纳入同租户、`grantMode=MANUAL_EXPORT`、状态为 `PENDING` 或允许重导 `RETRY` 的 grant order；`MANUAL`、终态单据、其他 tenant 单据不得纳入。
- 导出记录必须包含稳定 `exportId`、`fileName`、`fileFormat`、`status`、`operator`、`reason`、`filterSnapshot`、`rowCount`、`fileChecksum`、`storageRef` / 下载引用和明细。
- 文件未 READY 前，导出明细可存在，但 grant order 不得提前进入 `SENDING`。
- 文件生成成功后：
  - 导出状态进入 `READY`。
  - 文件内容字段完整，金额保留 4 位小数，非金额权益保留在 `rewardPayload`。
  - `rowCount` 与明细数量一致，重复下载 checksum 一致。
  - 明细内仍未终结 grant order 进入 `SENDING`。
  - 写导出 READY 审计。
- 对 READY 文件内 `SENDING` 单据逐单确认 `SUCCESS` / `FAILED` / `CLOSED`，按人工确认规则移动对应 quota，并能从 grant order 反查 export file，从 export file 反查 grant order。
- 关闭导出文件未终结单据时，只关闭该文件内仍为 `SENDING` / `RETRY` 的单据；已 `SUCCESS` / `FAILED` / `CLOSED` 的单据保持不变。

### 3. 额度不足

- 配置 `BUDGET` 不足时，事件处理返回 quota exceeded 或等价错误；不得留下 `REWARDABLE` participation、grant order、outbox、导出候选或 quota 脏数据。
- 配置 `STOCK` 不足时，同一 activity 级事务整体回滚；多 reward 场景不得产生部分 grant order。
- 配置 `USER_LIMIT` / `DAILY_LIMIT` 不足时，不生成 grant order，不冻结 reward-level quota。
- 不配置某类 quota 表示无限制；人工确认仍成功更新 grant order 和审计，不应因缺失 reservation 报错。

### 4. 多 Reward 与 Quota 聚合

- 同一 participation 命中多个 reward，每个 rewardCode 最多生成一张 grant order。
- 第一张 grant order `SUCCESS` 后，只确认该 reward 的 `BUDGET` / `STOCK`；`USER_LIMIT` / `DAILY_LIMIT` 仍保持 `FROZEN`。
- 一张 `SUCCESS`、其余 `FAILED` / `CLOSED` 后，最后一张进入终态时 `USER_LIMIT` / `DAILY_LIMIT` 一次性 `CONFIRMED`。
- 全部 grant order 都 `FAILED` / `CLOSED` 时，最后一张进入终态后 `USER_LIMIT` / `DAILY_LIMIT` 一次性 `RELEASED`。
- 重复确认任一终态单据不得二次移动 participation-level reservation。

### 5. 幂等重复

- 同一标准事件以相同 `Idempotency-Key` 和相同 payload 重复提交，返回 duplicate / already processed，不新增 participation、grant order、quota reservation、outbox 或导出候选。
- 同一 `participation.idempotency_key + ":" + rewardCode` 重复创建只能保留一张 grant order。
- 重复人工确认相同终态返回 duplicate / already_done，grant order、quota、审计不重复移动；若实现要求记录重复操作审计，必须不重复移动 quota。
- 已 `SUCCESS` 后确认 `FAILED` 或 `CLOSED` 返回状态冲突，不改写终态。
- 已 `FAILED` 后确认 `SUCCESS` 返回状态冲突。
- 已 `CLOSED` 后确认 `SUCCESS` 或 `FAILED` 返回状态冲突。
- READY / CLOSED 且未失败导出明细中的 grant order 不得被新导出重复纳入；`FAILED` 导出历史不阻止重新导出。
- 导出文件重复关闭返回 duplicate / already_closed，不重复关闭单据或移动 quota。

### 6. 并发

- 并发提交同一事件：只生成一条 inbox、一条 participation 和每个 rewardCode 一张 grant order；其他请求返回 duplicate 或唯一约束冲突映射后的幂等响应。
- 并发对同一 grant order 确认相同终态：只能一个请求执行状态推进和 quota 移动，其他请求返回 duplicate / already_done。
- 并发对同一 grant order 确认相反终态：只能一个终态胜出，其他请求返回状态冲突；quota 只能按胜出的终态移动一次。
- 两个导出请求并发选择同一批 `PENDING` grant order：每张 grant order 只能进入一个未失败导出文件；另一个请求应跳过或返回冲突，不能产生双明细。
- 导出创建与人工确认 / 关闭并发：导出只纳入成功锁定且仍未终结的单据；已终态单据跳过并在结果中可观察。
- 文件 READY 推进与逐单确认并发：不得出现导出未 READY 但单据已 `SENDING` 或已确认成功却缺失导出明细的状态。

### 7. PostgreSQL Smoke

- 使用 Docker PostgreSQL 和 `EX_MARKETING_DATABASE_URL=postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable` 执行持久化测试。
- PostgreSQL smoke 至少覆盖：
  - quota 不足事务回滚。
  - grant order / participation idempotency 唯一约束。
  - 并发重复确认幂等。
  - 并发导出唯一纳入。
  - 文件生成失败后保留导出记录和明细，grant order 保持 `PENDING` / `RETRY`，quota 保持 `FROZEN`，后续可重新导出。
  - `MANUAL_EXPORT` READY 文件内 `SENDING` 单据确认成功后，导出追踪、审计和 quota 状态均可从数据库查询验证。
- smoke 完成后通过 SQL 或 repository 查询确认无重复未失败导出明细、无负数 quota、无人工单被 outbox / callback / DLQ / timeout-release 自动终结。

### 8. 自动流程隔离

- 开启 outbox worker 后处理自动奖励任务，确认 `MANUAL` / `MANUAL_EXPORT` grant order 不被 worker claim，不生成自动投递 outbox。
- 调用 `/openapi/v1/reward/callback` 定位人工单时，默认不得终结 `MANUAL` / `MANUAL_EXPORT` 单据；应返回不支持、未找到或等价错误。
- DLQ replay / close 不应作用于人工发奖单。
- 通用 quota timeout-release 扫描人工 `PENDING` / `SENDING` / `RETRY` 单据时默认跳过，不更新 grant order 状态，不移动 quota；dry-run / 日志如有输出应标明 manual grant excluded。

### 9. 审计与对账

- 所有人工确认成功、确认失败、关闭、导出创建、导出 READY、导出失败、导出关闭和 participation-level quota 聚合终结均写审计。
- 审计不得记录 Authorization、Cookie、token、secret 或完整敏感 payload。
- `MANUAL` 纯人工动作不应要求或伪造 `exportId`；`MANUAL_EXPORT` 导出相关动作和 READY 文件内逐单确认应记录真实 `exportId`。
- 对账查询能回答：
  - billNo 当前状态。
  - 导出文件包含哪些 grant order 以及每张单当前终态。
  - reward-level reservation 状态。
  - participation-level reservation 状态及聚合依据。
  - 操作人、时间、原因和前后状态。

## Required Commands

Test Agent 应按以下顺序执行，并在 `test.md` 记录每条命令结果。若前置环境不可用，应记录 BLOCK 原因。

~~~bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -run 'TestPostgres.*Manual|TestPostgres.*Export|TestPostgres.*Grant|TestPostgres.*Quota|TestPostgres.*Concurrent' -count=1
~~~

如果 Dev Agent 新增了更精确的包级或用例名，Test Agent 应追加执行对应命令，例如：

~~~bash
go test ./internal/engine -run 'Test.*Manual|Test.*Export|Test.*Quota|Test.*Concurrent' -count=1
go test ./internal/httpapi -run 'Test.*Manual|Test.*Export|Test.*Audit|Test.*Reconciliation' -count=1
go test ./internal/postgres -run 'Test.*Manual|Test.*Export|Test.*Concurrent|Test.*Quota' -count=1
~~~

## Data / Smoke

- 使用唯一 tenant / activity / subject / biz 前缀，避免与现有测试数据冲突。
- 每个 smoke 用例结束后清理自身前缀数据，或使用隔离数据库。
- PostgreSQL smoke 查询重点：
  - `marketing_participation`：重复事件不新增记录；quota 不足不保留 `REWARDABLE` participation。
  - `marketing_grant_order`：状态机符合预期；人工单不被自动流程误改。
  - `marketing_quota` / reservation 表：`BUDGET` / `STOCK` 与 `USER_LIMIT` / `DAILY_LIMIT` 状态移动符合粒度规则。
  - 导出文件与明细表：未失败导出中同一 grant order 只有一条有效明细；FAILED 导出保留历史且允许重导。
  - audit 表：action、operator、reason、grantOrderId / billNo、exportId 可选性、前后状态、quota 摘要完整。
  - outbox / DLQ 表：`MANUAL` / `MANUAL_EXPORT` 不产生自动投递消息。

## Decision

~~~text
PASS
~~~

PASS 原因：`review.md` Decision 为 PASS；测试计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，并补充 review 中的 `MANUAL_EXPORT` 导出确认前置条件和审计 `exportId` 可选性验证。

需求：2026-05-27-manual-grant-export
