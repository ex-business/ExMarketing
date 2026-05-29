# Analysis

## 需求 ID

2026-05-27-db-backup-restore

## Review 反馈处理

已读取 `tasks/2026-05-27-db-backup-restore/review.md`。当前文件为模板态，但包含 `Open Questions` 与 `Decision PASS / BLOCK` 占位，本轮按返工要求逐条处理：

| Review 项 | 处理 |
| --- | --- |
| `P0:` / `P1:` / `P2:` 均为空 | 无具体缺陷条目；本轮不因空模板继续 BLOCK。 |
| `Open Questions` 为“待填写” | 未形成具体 Review 问题；本轮主动补齐备份范围、恢复目标、输入输出、校验脚本、smoke 链路、异常场景、非目标和验收标准。 |
| `Decision` 为 `PASS / BLOCK` 占位 | 旧 Review 未给出有效结论；本轮输出可审核 Analysis，并给出真实 Score 与 Decision。 |

## Analysis 自检修正

已读取上一轮 `analysis.md`。上一轮为模板态，存在 `Score 0`、`Decision PASS / BLOCK`、背景目标等章节“待填写”，等价于 Analysis 自检 BLOCK。本轮逐项修正：

| 自检项 | 修正 |
| --- | --- |
| `Score 0` | 已按评分规则补齐完整需求并重新评分。 |
| `Decision PASS / BLOCK` 占位 | 已给出明确 `Decision PASS`。 |
| 背景、目标、范围未定义 | 明确本需求是 PostgreSQL 灾备演练能力，目标是提供备份命令、恢复命令、校验脚本，并证明恢复后 Event In 到 Outbox 最小链路可跑通。 |
| 输入输出、业务规则、异常和边界缺失 | 已补充备份对象、恢复环境、校验结果、幂等隔离、敏感信息脱敏、失败诊断和重复执行规则。 |
| 验收对象缺失 | 已明确验收对象为文档化命令/脚本/检查清单，以及恢复后 PostgreSQL 状态和最小业务 smoke 证据。 |
| 待确认问题缺失 | 现有材料足以确定业务目标、参与方和验收对象；未决细节均按最小可验收口径默认，不再 BLOCK。 |

本轮读取范围：`task.md`、`review.md`、上一轮 `analysis.md`、`README.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/operations.md`、`docs/api.md`，以及相邻 PostgreSQL E2E / migration 任务的 Analysis 文档用于对齐既有业务口径。本轮未读取实现代码，未写代码，未提交，未设计具体代码改法。

## 需求说明

本需求要求为 ExMarketing 补齐 PostgreSQL 数据备份、恢复演练、恢复后数据校验，以及恢复后核心业务链路 smoke。最终产物必须让运维、测试和开发能用一组明确命令在本地 PostgreSQL 环境中完成：

1. 对当前 PostgreSQL 数据库生成可恢复备份。
2. 将备份恢复到一个隔离数据库或全新本地数据库。
3. 校验恢复后的 schema、migration 记录和核心业务表数据完整性。
4. 在恢复后的数据库上运行 Event In 到 Outbox 的最小链路，证明恢复库不仅“能连接”，还可以继续处理核心业务。

默认最小可验收口径：本任务面向本地/测试环境灾备演练，不直接操作生产库，不实现自动定时备份系统，不要求云存储生命周期管理。

## 背景目标

ExMarketing 的生产化方向依赖 PostgreSQL repository。现有文档已定义：

- 设置 `EX_MARKETING_DATABASE_URL` 后使用 PostgreSQL。
- migration 发布流程记录在 `schema_migrations`。
- 核心链路为 Event Intake -> Idempotency -> Campaign Match -> Rule Evaluation -> Quota / Budget / Inventory -> Grant Order -> Outbox。
- 本地 PostgreSQL 可通过 Docker 启动，恢复后可运行 PostgreSQL E2E smoke。

由于活动、事件 inbox、participation、grant_order、outbox、quota、audit 等表承载核心业务状态，必须有可执行的备份恢复演练。目标不是证明 `pg_dump` 命令存在，而是证明备份文件可以恢复，并且恢复后的数据库能继续支撑核心链路。

## 参与方

| 参与方 | 职责 |
| --- | --- |
| 运维/发布人员 | 执行备份、恢复演练、校验命令，保存恢复证据和失败诊断。 |
| Test Agent | 按测试计划验证备份命令、恢复命令、校验脚本和恢复后 smoke。 |
| Dev Agent | 按本需求补齐文档化命令、脚本或任务入口，但不得扩大为生产备份平台。 |
| PostgreSQL | 备份与恢复对象，保存 schema、migration 记录和核心业务数据。 |
| ExMarketing 服务 | 在恢复后的 PostgreSQL 上启动并处理标准事件。 |
| Outbox worker | 恢复后 smoke 中推进 outbox，证明异步发送链路仍可用。 |
| 外部发奖系统替身 | 可使用本地 HTTP stub 或测试目标地址接收 outbox 请求。 |

## 输入输出

### 输入

| 输入 | 默认口径 |
| --- | --- |
| 源数据库 DSN | `EX_MARKETING_DATABASE_URL`，必须是 PostgreSQL；未设置或指向内存仓储则失败。 |
| 备份输出目录 | 默认本地临时或 `backups/` 下的演练目录；目录不存在时可创建。 |
| 备份文件名 | 必须包含数据库名或项目标识、UTC 时间戳，避免覆盖旧备份。 |
| 恢复目标 DSN | 必须与源库隔离；默认使用本地 Docker PostgreSQL 中独立数据库或清空后的测试库。 |
| migration 目录 | 既有 `db/migration`，用于校验 `schema_migrations` 与核心表状态。 |
| smoke 配置 | 使用标准事件、普通活动、HTTP/WEBHOOK reward target 和唯一 `runId`。 |

### 输出

成功输出必须包含：

1. 备份文件路径、大小、生成时间和校验摘要。
2. 恢复目标数据库标识。
3. schema / migration 校验结果。
4. 核心表 row count 或按 runId 的数据校验摘要。
5. Event In 到 Outbox smoke 的 runId、tenantId、activityId、event inbox、participation、grant_order、outbox 状态。
6. 明确 `PASS`。

失败输出必须包含：

1. 失败阶段：`BACKUP`、`RESTORE`、`SCHEMA_CHECK`、`DATA_CHECK`、`SERVICE_START`、`EVENT_IN_SMOKE`、`OUTBOX_SMOKE` 等。
2. 脱敏后的源 DSN / 目标 DSN 摘要。
3. 关键命令、退出码、stderr 摘要。
4. 恢复目标库当前核心表状态。
5. 明确非 0 退出状态或 `FAIL` 标识。

## 业务规则

### 备份规则

1. 备份对象是 PostgreSQL 数据库，不包含内存仓储。
2. 备份必须包含 schema 与数据；只备份 schema 或只备份数据均不满足本需求。
3. 备份必须覆盖 `schema_migrations` 与核心业务表：`marketing_activity`、`marketing_rule`、`marketing_reward`、`marketing_quota`、`marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`、`marketing_audit`、`marketing_fact_provider`，以及已纳入 migration 的后续核心表。
4. 备份命令不得在输出中泄露数据库密码、token、Authorization、Cookie 或 secret。
5. 备份文件命名必须避免覆盖，且同一次演练产生的日志必须能关联到备份文件。
6. 最小口径允许使用 PostgreSQL 原生命令，如 custom format 或 plain SQL dump；具体格式由实现选择，但必须有对应恢复命令。

### 恢复规则

1. 恢复必须写入隔离目标库，默认不得直接覆盖源库。
2. 如果恢复目标库已有数据，必须显式要求 `--force`、`RESET_RESTORE_DB=true` 或等价确认；默认不能静默覆盖。
3. 恢复后必须能通过 `EX_MARKETING_DATABASE_URL=<恢复目标 DSN>` 启动或连接 ExMarketing。
4. 恢复流程必须先完成数据库对象恢复，再执行校验；不得用重新跑 migration 代替恢复备份。
5. 如果备份来自旧版本 schema，恢复后可要求按既有 migration 发布流程升级，但本任务的最小验收默认使用当前版本库生成当前版本备份。

### 校验规则

1. 校验必须至少包含三类：
   - 连接校验：恢复目标 PostgreSQL 可连接。
   - schema 校验：`schema_migrations` 与核心表存在。
   - 数据校验：核心表 row count 或按 runId 的业务数据可查询。
2. `schema_migrations` 中已成功版本的 checksum 必须与本地 migration 文件一致；不一致则校验失败。
3. 核心表缺失、关键表为空但源库非空、恢复命令未导入业务数据，均视为失败。
4. 校验脚本必须输出结构化摘要，便于 Test Agent 判定 PASS / FAIL。
5. 校验不能依赖管理后台页面人工观察。

### 恢复后 smoke 规则

1. smoke 必须使用恢复目标数据库，不得回连源库。
2. smoke 必须走标准 HTTP 入口 `POST /openapi/v1/events`，不能直接写表。
3. smoke 必须产生或验证至少一条完整链路：
   - event inbox 写入并处理。
   - participation 生成。
   - grant_order 生成。
   - outbox 生成。
4. 默认最小口径不强制外部真实发奖成功；但必须证明 Event In 到 Outbox 已到达 `PENDING`、`SENT` 或可观测发送结果。
5. 如果开启 Outbox worker，则应验证 outbox 可推进；如果 worker 或 stub 不可用，最小 smoke 仍必须验证 outbox 已生成，并在输出中标明 worker 发送不属于本次最小通过证据。
6. smoke 数据必须使用唯一 `runId`，避免与恢复出来的历史数据冲突。
7. 重复执行 smoke 不得污染判断；同一 runId 可清理，或每次生成新 runId。

## 边界条件

| 边界 | 预期 |
| --- | --- |
| 未设置源 DSN | 备份阶段失败，提示必须设置 PostgreSQL DSN。 |
| 源 DSN 连接失败 | 备份失败，输出脱敏连接摘要和 PostgreSQL 错误。 |
| 备份目录不可写 | 备份失败，不创建空成功标识。 |
| 备份文件为空或过小 | 校验失败，不能进入恢复成功结论。 |
| 恢复目标与源库相同 | 默认失败，除非显式强制且仅限本地演练。 |
| 恢复目标已有数据 | 默认失败，避免误覆盖。 |
| `schema_migrations` 缺失 | 校验失败；若是历史库基线问题，需要先按 migration 发布流程处理，不在本任务自动修复。 |
| 核心表缺失 | 校验失败。 |
| 恢复后服务无法启动 | smoke 失败，输出健康检查和数据库错误摘要。 |
| 事件 API 鉴权开启 | smoke 必须使用有效 Bearer token；没有 token 则 smoke 失败。 |
| Outbox worker 未开启 | 最小 smoke 可验证 outbox 生成，但完整 worker 发送验收需标记为未覆盖。 |
| callback stub 不可用 | 不影响 Event In 到 Outbox 最小验收；完整发奖 callback 不作为本任务必过项。 |

## 异常场景

1. `pg_dump` 或等价备份命令失败：输出退出码、stderr 摘要、源库连接摘要和目标文件状态。
2. 备份中途被打断：不得留下误导性的 PASS；下次执行应生成新文件或清理未完成文件。
3. 恢复命令失败：输出恢复目标库状态，提示是否需要清理目标库后重试。
4. 恢复成功但 migration checksum 不一致：判定 `SCHEMA_CHECK FAIL`，不得继续 smoke 当作通过。
5. 恢复成功但关键表 row count 明显不一致：判定 `DATA_CHECK FAIL`。
6. 服务连接恢复库失败：输出 `EX_MARKETING_DATABASE_URL` 脱敏摘要和 `/healthz` 结果。
7. smoke 创建活动或 quota 失败：输出 Admin API 请求摘要、响应状态和响应体。
8. smoke 上报事件失败：输出 Idempotency-Key、事件摘要和 API 响应。
9. smoke 未生成 outbox：输出 participation、grant_order、reward 计算和 quota 状态，定位是规则未命中、零奖励、quota 不足还是写入失败。
10. 异步 worker 超时：输出 outbox 当前状态、retry_count、next_retry_at 和 error_message。

## 非目标范围

1. 不实现生产级自动定时备份、保留策略、加密归档、跨区域复制或云对象存储生命周期。
2. 不直接操作生产库，不要求提供生产恢复 Runbook 的审批流程。
3. 不支持 MySQL、SQLite、MongoDB 或其他数据库。
4. 不要求恢复到任意历史版本 schema；默认恢复当前版本备份。
5. 不要求完整 callback 成功/失败发奖闭环；本任务必过链路截止到 Outbox。
6. 不设计具体代码改法，不指定必须用哪个脚本语言或包。
7. 不要求通过管理后台页面人工点击完成验收。

## 风险点

| 风险 | 影响 | 需求约束 |
| --- | --- | --- |
| 误把恢复目标指向源库 | 可能覆盖源数据 | 默认禁止源库与目标库相同，覆盖需显式强制且限本地。 |
| 只备份 schema 未备份数据 | 恢复后业务状态丢失 | 要求 schema + data，并做 row count / runId 数据校验。 |
| 恢复后只检查连接不跑业务 | 无法证明系统可用 | 必须跑 Event In 到 Outbox smoke。 |
| 敏感信息进入日志 | 安全风险 | DSN、token、secret 必须脱敏。 |
| Outbox 依赖外部系统 | smoke 不稳定 | 最小验收只要求到 Outbox；worker 推进作为完整验证增强。 |
| 恢复目标残留旧数据 | 验收误判 | 使用唯一 runId，并默认要求隔离目标库或显式清理。 |

## 验收标准

必须全部满足：

1. 提供明确备份命令或脚本入口，能对本地 PostgreSQL 生成非空、可恢复备份文件。
2. 提供明确恢复命令或脚本入口，能将备份恢复到隔离目标 PostgreSQL。
3. 提供校验脚本或命令，至少验证连接、`schema_migrations`、核心表存在、checksum 和核心表数据摘要。
4. 在恢复目标库上设置 `EX_MARKETING_DATABASE_URL` 后，能运行恢复后 smoke。
5. smoke 通过标准 Event API 上报事件，并验证 event inbox、participation、grant_order、outbox 至少各存在一条与 runId 关联的记录。
6. 命令失败时有明确失败阶段、退出码和脱敏诊断，不允许静默通过。
7. 重复执行不会覆盖旧备份，不会误清源库，不会因历史 smoke 数据误判。
8. `go test ./...` 与 `go build ./...` 仍应作为后续 Dev/Test 基础验证，但不能替代备份恢复演练验收。

## 待确认问题

无阻塞性待确认问题。以下按默认最小可验收口径执行：

| 问题 | 默认口径 |
| --- | --- |
| 备份格式选型 | 由实现选择 PostgreSQL 原生可恢复格式；必须配套恢复命令。 |
| 恢复目标库命名 | 使用隔离测试库或临时库，禁止默认覆盖源库。 |
| 是否要求 Outbox worker 发送到 `SENT` | 必过口径到 outbox 生成；worker 发送为增强验收，若环境可控应覆盖。 |
| 是否覆盖生产备份策略 | 不覆盖，本任务只做本地/测试可执行演练。 |

## Score

```text
92
```

评分说明：

| 维度 | 分数 | 说明 |
| --- | --- | --- |
| 需求清晰度 | 18/20 | 目标、参与方、产物和默认口径已明确。 |
| 业务规则完整 | 18/20 | 覆盖备份、恢复、校验和 smoke；生产备份平台细节列为非目标。 |
| 边界异常清楚 | 19/20 | 覆盖 DSN、目标库、权限、checksum、worker、鉴权等主要异常。 |
| 验收标准明确 | 19/20 | 明确可执行命令、恢复验证和 Event In 到 Outbox 证据。 |
| 约束一致性 | 18/20 | 与现有 PostgreSQL、migration、E2E 和 API 文档一致，未引入代码实现细节。 |

## Decision

```text
PASS
```
