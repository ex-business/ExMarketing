# Analysis

## 需求名称

db-migration-release: 数据库 migration 发布流程

## Review 反馈处理

本轮读取了 `review.md`。该文件仍是模板内容，`P0:`、`P1:`、`P2:` 均未填写具体问题，`Open Questions` 为“待填写”，`Decision` 为 `PASS / BLOCK` 占位，不构成实质 Review 打回意见。

处理口径：

- P0：无具体条目，无需修正。
- P1：无具体条目，无需修正。
- P2：无具体条目，无需修正。
- Open Questions：无具体问题；本轮仍按高风险生产化任务补齐需求、边界、异常、验收和默认最小可验收口径。
- Decision 占位：旧 review 未给出有效 BLOCK 原因，本轮不因模板占位继续 BLOCK。

## Analysis 自检修正

上一轮 `analysis.md` 为模板状态，Score 为 `0`，Decision 为 `PASS / BLOCK` 占位，背景、目标、细化需求、业务规则、输入输出、异常、边界、非目标、风险、验收和待确认问题均为“待填写”。这属于 Analysis 自身未完成导致的阻塞。

修正方式：

- 关闭旧 Score 0：本轮补齐可审核、可开发、可测试的需求说明，并按评分规则重新打分。
- 关闭旧 Decision 占位：本轮给出明确 `PASS` / `BLOCK` 结论。
- 补齐缺失背景：结合 `task.md`、`docs/architecture.md`、`docs/operations.md`、`docs/data-model.md`、`docs/api.md`、`docs/roadmap.md`，确认本需求属于 Phase 5 生产化能力，目标是让 PostgreSQL schema 变更可正式发布。
- 补齐默认口径：在现有材料没有唯一答案处，采用“最小可验收、生产前可审计、已有环境不破坏”的默认约束，关闭旧 BLOCK。

## 背景和目标

ExMarketing 当前是单体 Go 服务，支持 in-memory repository 和 PostgreSQL repository。PostgreSQL 是生产化方向的持久化存储，当前业务链路依赖活动、规则、奖励、quota、inbox、participation、grant_order、outbox、audit、fact provider 等表。现有运维文档说明首次启动容器会自动执行 `db/migration` 下 SQL，但已有数据卷新增 migration 需要人工执行单个 SQL 文件；生产前检查明确列出“数据库 migration 有正式发布流程”为缺口。

本需求要建立正式数据库 migration 发布流程，确保：

- 新环境可以一键初始化到目标 schema。
- 已有环境可以幂等升级，不重复执行已完成 migration，不破坏已有业务数据。
- schema drift 可以检测并阻止不安全发布。
- 发布前检查、失败处理、回滚或前进修复方案、本地验证脚本有明确流程和验收标准。

默认数据库范围为 PostgreSQL。in-memory repository 不在 migration 执行范围内，但相关开发/测试流程不能被破坏。

## 参与方

- Dev Agent / 开发者：新增或维护 migration 文件、执行本地验证、更新文档。
- Test Agent：按验收标准启动 PostgreSQL，验证新环境初始化、已有环境升级、drift 检测和失败场景。
- Commit Agent：只在 Review/Test PASS 后提交相关产物。
- 运维/发布人员：在测试、预发、生产环境执行发布前检查、migration 应用、失败处理和发布后验证。
- ExMarketing 服务：启动时或运维命令中使用 PostgreSQL schema，不得在 schema 未达标时静默运行核心链路。
- PostgreSQL：唯一纳入本需求的目标数据库。

## 输入输出

### 输入

- PostgreSQL 连接串：沿用 `EX_MARKETING_DATABASE_URL`，例如 `postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable`。
- migration 文件目录：使用项目既有 `db/migration` 目录。
- migration 文件：按版本号顺序排列，当前已知存在 `001_marketing_core.sql`、`002_audit_fact_outbox_indexes.sql` 这类格式；后续文件必须保持稳定版本号和描述。
- 操作模式：
  - 新环境初始化。
  - 已有环境升级。
  - 发布前检查。
  - schema drift 检测。
  - 本地验证。
  - 失败后诊断与修复。

### 输出

- 数据库中的 migration version 记录，至少能表达版本号、文件名或标识、checksum、执行状态、开始/完成时间、错误摘要。
- 数据库 schema 被初始化或升级到预期版本。
- 发布前检查结果：通过或失败，并能说明失败原因。
- schema drift 检测结果：通过或失败，并指出缺失对象、额外对象、checksum 不一致、已执行版本缺失或顺序异常等可定位信息。
- 本地验证结果：新库初始化、已有库重复升级、drift 检测均可通过命令复现。

## 细化需求

### R1. migration 版本记录

- 必须在数据库内维护 migration version 表，用于记录每个已应用 migration。
- version 表本身必须能在空库中被自动创建，并纳入初始化流程。
- 每个 migration 版本只能成功应用一次；重复执行同一版本不得再次执行 SQL 内容。
- 已成功应用的 migration 如果本地文件 checksum 发生变化，必须视为 drift 或非法修改，检查失败。
- 失败中的 migration 不能被误判为已完成；后续重试必须能识别失败状态并给出明确处理建议。

默认最小字段口径：

- version：版本号或可排序版本标识，唯一。
- name/file：migration 文件名或描述。
- checksum：文件内容校验值。
- status：执行状态，至少区分 success 与 failed。
- applied_at / started_at / finished_at：执行时间信息。
- error_message：失败摘要，需脱敏且长度受控。

### R2. migration 文件规则

- migration 文件必须按版本顺序执行。
- 已发布并成功执行过的 migration 文件不得修改内容；如需变更，必须新增更高版本 migration。
- 文件名必须能稳定解析顺序，继续沿用 `NNN_description.sql` 的最小口径。
- 同一版本号不得存在多个文件。
- 文件内容必须适合重复发布流程：由 version 表控制幂等，不要求每条 DDL 都手写 `IF NOT EXISTS`，但新建对象类 DDL 推荐具备数据库层幂等保护以降低手工恢复风险。
- migration 中不得写入环境专属 secret、token、密码、生产连接信息或不可脱敏业务数据。

### R3. 新环境一键初始化

- 空 PostgreSQL 数据库执行初始化后，应创建 version 表和全部业务 schema。
- 初始化后应包含当前数据模型文档描述的核心表能力：activity、rule、reward、quota、event inbox、participation、grant order、outbox、audit、fact provider，以及后续已纳入 migration 的对象。
- 初始化命令必须可以在本地 Docker PostgreSQL 上复现。
- 初始化完成后，再次执行同一流程应无副作用并报告所有版本已应用或跳过。

### R4. 已有环境幂等升级

- 对已经执行过部分 migration 的数据库，流程必须只执行未完成的新版本。
- 已成功版本的 checksum 必须与本地文件一致；不一致时停止升级。
- 已有业务数据不得被清空、覆盖或重置。
- 新增表、索引、列、约束时必须考虑已有数据；如果需要数据回填或默认值，需求上要求迁移过程给出可验证结果。
- 幂等升级的验收对象不是“无报错”而是“目标版本一致、历史数据仍存在、重复执行无额外变更”。

### R5. schema drift 检测

schema drift 检测必须覆盖至少三类风险：

- migration 记录 drift：version 表记录的版本与本地 migration 文件集合不一致，例如缺失本地文件、重复版本、checksum 改变、已执行版本顺序异常。
- schema 对象 drift：目标 schema 中关键表、索引、约束、列与 migration 期望不一致。
- 未受控变更 drift：数据库中出现影响核心 schema 的人工改动，导致发布脚本无法确认升级安全。

最小可验收口径：

- 必须先基于 version 表和 checksum 进行 drift 检测。
- 必须对核心业务表是否存在进行检测。
- 对列、索引、约束的深度检测可按当前已发布 migration 的对象清单逐步覆盖，但本需求至少要求能检测“关键表缺失”和“已执行文件被修改”。
- drift 检测失败时不得继续执行升级，除非运维明确先修复数据库或补充修复 migration。

### R6. 发布前检查

执行 migration 前必须有发布前检查步骤，输出明确 PASS/FAIL。

检查项至少包括：

- `EX_MARKETING_DATABASE_URL` 已设置且能连接 PostgreSQL。
- 当前数据库用户具备执行 migration 所需权限。
- migration 目录可读，文件名顺序合法，无重复版本。
- 已应用 migration 与本地文件 checksum 一致。
- 不存在失败未处理的 migration 记录。
- 目标数据库没有已知阻塞性 drift。
- 当前待执行 migration 列表可被打印或审计。

发布前检查失败必须停止，不允许继续应用 migration。

### R7. 失败处理、回滚与 forward-only 策略

默认策略采用 forward-only：

- 已成功发布到共享环境或生产环境的 migration 不执行自动 down 回滚。
- 失败发生在事务内且数据库自动回滚时，可修正同一未成功 migration 后重试，但必须保证 version 表中没有 success 记录。
- 失败发生后如果数据库处于部分变更状态，必须停止后续 migration，并要求通过人工诊断、备份恢复或新增修复 migration 进入可继续状态。
- 对生产或共享环境，修复已成功版本必须通过新增更高版本 migration，不允许修改已成功版本文件。

本需求要求提供失败回滚方案，但最小可验收口径为“明确 forward-only 主策略 + 失败时停止 + 记录失败 + 人工恢复或新增修复 migration + 重试验证”，而不是要求实现完整 down migration 框架。

### R8. 本地验证脚本或命令

必须提供本地可复现验证方式，覆盖：

- 启动 Docker PostgreSQL。
- 空库初始化。
- 重复执行初始化/升级验证幂等。
- 从仅有旧版本的库升级到最新版本。
- drift 检测可识别至少一种故障：例如修改已应用 migration 文件 checksum、删除关键表、或制造 version 记录不一致。

验证命令应与现有运维文档一致使用本地 PostgreSQL DSN，不依赖生产环境。

### R9. 服务启动与运行约束

- 使用 PostgreSQL repository 时，服务不得在 schema 缺失关键对象的情况下静默进入可处理业务事件状态。
- 如果自动应用 migration 被纳入实现，则必须保证启动失败时错误清楚、不会半启动；如果采用独立运维命令，则服务启动文档必须说明上线前必须先完成 migration 检查和应用。
- 本需求不强制选择“服务启动自动 apply”或“独立命令 apply”。默认最小可验收口径为：存在独立可执行的发布流程；服务侧至少不能掩盖 schema 未达标错误。

## 业务规则

- PostgreSQL schema 变更必须通过 migration 文件进入正式发布流程，不允许靠人工 psql 单条命令作为唯一发布方式。
- version 表是判断 migration 是否执行过的权威来源。
- checksum 是判断已发布文件是否被篡改或 drift 的权威依据。
- migration 执行顺序必须严格按版本号升序。
- 低版本失败时不得跳过执行高版本。
- 已成功 migration 的内容不可变；任何后续修正必须新增版本。
- 对已有环境升级时，业务数据保留优先级高于自动修复便利性。
- drift 检测失败默认阻止发布。
- 错误输出和日志不得泄露数据库密码、token、Authorization、完整 secret 或生产敏感配置。

## 边界条件

- 空库：必须能创建 version 表和完整 schema。
- 已有库无 version 表但已有业务表：视为历史环境接入。最小可验收口径是发布前检查必须识别并 BLOCK，要求人工基线确认或补建基线记录；不得直接重跑初始化破坏数据。
- version 表存在但无记录：按空 schema 或未初始化环境处理，但如果已存在业务表，必须按 drift/历史环境处理。
- migration 文件缺号：允许存在版本号不连续，只要排序唯一；不要求强制连续。
- migration 文件重名或重复版本：发布前检查失败。
- 已应用版本本地文件缺失：发布前检查失败。
- 已应用版本 checksum 改变：发布前检查失败。
- 数据库连接不可用：发布前检查失败。
- 数据库权限不足：发布前检查失败，不能部分执行。
- 多实例同时执行 migration：流程必须避免并发重复执行；最小口径要求通过数据库锁、唯一约束或运维串行约束保证只有一个执行者成功。
- 非 PostgreSQL：不纳入本需求。

## 异常场景

- SQL 执行失败：记录失败状态和脱敏错误摘要，停止后续 migration。
- migration 执行过程中进程崩溃：下次运行必须能通过 version 表和实际 schema 检测到未完成或 drift 状态，并停止或允许安全重试。
- 已成功版本被人工改库破坏对象：drift 检测失败，不能继续升级。
- 已有环境缺少历史 migration 记录：不能自动假定成功，必须进入基线/人工确认流程。
- 新 migration 引入不兼容变更：发布前检查无法完全替代 review/test；验收要求 Test Agent 使用 PostgreSQL smoke 覆盖核心链路。
- 部分 DDL 在 PostgreSQL 中无法事务化或影响较大：需求上要求文档明确风险和失败处理，不要求本轮实现在线无锁发布能力。

## 非目标范围

- 不设计或实现具体代码改法。
- 不要求支持 MySQL、SQLite、MongoDB 或其他数据库。
- 不要求实现完整 down migration 框架。
- 不要求提供生产备份系统、备份调度或恢复自动化，只要求发布失败时有恢复/修复口径。
- 不要求重构现有业务表结构。
- 不要求补齐所有历史 schema 对象的逐列逐索引 drift 深度校验；本轮最小验收为 version/checksum + 核心表存在性 + 关键异常可检测。
- 不要求将 in-memory repository 迁入 migration 管理。
- 不要求实现在线零停机、蓝绿发布或跨版本应用兼容策略；如有破坏性 schema 变更，需在具体 migration 任务中单独分析。

## 风险点

- 高风险：migration 失败可能导致服务无法启动或业务链路不可用。
- 高风险：已有环境如果没有 version 基线，自动初始化可能误伤业务数据。
- 高风险：修改已发布 migration 文件会造成环境不可复现。
- 中风险：schema drift 检测粒度不足时，人工改库可能延迟到运行时才暴露。
- 中风险：DDL 锁表或长事务可能影响生产可用性，后续具体 migration 需要单独评估。
- 中风险：并发执行 migration 可能造成重复 DDL 或版本记录冲突，需要串行保护。

## 验收标准

### A1. 新环境初始化

- 在全新 Docker PostgreSQL 数据库上，执行 migration 发布流程一次成功。
- 数据库中存在 migration version 表，并记录全部已执行版本为 success。
- 核心业务表存在，至少覆盖当前数据模型中的 activity、rule、reward、quota、event inbox、participation、grant order、outbox、audit、fact provider。
- 再次执行同一流程成功，且不会重复应用已成功 migration。

### A2. 已有环境幂等升级

- 构造只执行旧版本 migration 的数据库，再运行发布流程后只应用缺失的新版本。
- 升级前插入的代表性业务数据在升级后仍存在。
- 升级完成后再次运行发布流程无新增变更且结果为已最新。

### A3. drift 检测

- 修改一个已应用 migration 文件内容后，发布前检查或 drift 检测必须失败并指出 checksum 不一致。
- 删除或破坏一个核心业务表后，drift 检测必须失败并指出 schema 不符合预期。
- 制造重复版本、已应用版本文件缺失或失败状态未处理时，发布前检查必须失败。

### A4. 失败处理

- 构造一个会失败的 migration 时，流程必须停止在失败版本，不继续执行后续版本。
- 失败信息必须被记录，且不得包含密码、token、Authorization 或完整连接串 secret。
- 修复失败原因后，按 forward-only/重试规则可以重新进入检查流程；已 success 的版本不得被重复执行。

### A5. 本地验证与文档

- 文档或脚本提供清晰命令，能在本地 PostgreSQL 复现初始化、升级、重复执行、drift 检测。
- Test Agent 可据此执行 `go test ./...`、`go build ./...` 和 PostgreSQL smoke 或等价命令。
- 运维文档不再只要求手工 `psql -f` 单个文件，而是描述正式发布检查、应用、验证、失败处理。

### A6. 生产前约束

- 发布前检查 FAIL 时不得进入 migration apply。
- schema 未达标时，使用 PostgreSQL 的服务不得静默处理业务事件。
- 产物中必须明确 forward-only 策略和历史环境基线处理约束。

## 待确认问题

以下问题不阻塞进入开发，均已给出默认最小口径：

- 是否采用 up/down 还是 forward-only？默认采用 forward-only，不做自动 down。
- migration 是否服务启动自动执行？默认要求独立发布流程可执行；服务启动不能掩盖 schema 未达标。
- drift 检测是否必须逐列逐索引完整比对？默认本轮最小覆盖 version/checksum、核心表存在性和关键异常；深度比对可后续增强。
- 历史已有业务表但无 version 表如何处理？默认 BLOCK 自动升级，要求人工基线确认或补建基线记录。
- 并发 migration 如何约束？默认必须有数据库级或运维级串行保护，验收关注不会重复执行成功版本。

## Score

```text
需求清晰度        18/20
业务规则完整      18/20
边界异常清楚      19/20
验收标准明确      19/20
约束一致性        18/20
总分              92/100
```

扣分说明：

- drift 检测深度在本轮采用最小可验收口径，未要求一次性完成逐列逐索引全量比对。
- 服务启动是否自动 apply 未强制指定，以避免越过现有架构决策；但已用独立发布流程和启动不得静默掩盖 schema 缺失关闭阻塞。

## Decision

```text
PASS
```

需求细节已达到可审核、可开发、可测试标准；无需要 Owner 立即决策才能推进的 BLOCK。
