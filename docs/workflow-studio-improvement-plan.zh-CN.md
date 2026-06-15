# ExMarketing Workflow Studio 完善计划

这份文档用于说明 Workflow Studio 从“能画流程图”走向“可配置、可发布、可执行、可排查”的完整建设路线。

当前系统已经具备以下基础能力：

- 可视化画布，支持节点和连线。
- Workspace、Folder、Workflow 的入口和选择视图。
- 活动画布草稿保存。
- Workflow version 发布基础。
- WorkflowExecutor 执行基础。
- execution / node_execution 日志基础。
- 前端节点运行状态展示基础。

后续建议按阶段逐步完善。每个阶段都应该同时包含后端能力、前端接入和测试覆盖，不建议只做 UI 或只做后端。

## 当前整体缺口

当前 Studio / Workflow 已经具备可运行骨架：workspace、folder、activity、画布、draft、publish、execution、run history、compare、debug、template 都已经接入。但距离“运营可以长期稳定使用”的完整产品，还需要继续补齐以下能力。

### 1. 新建活动后的空白引导

当前新建活动已经进入空画布，不再自动套用完整链路。后续还需要：

- 空画布上的快捷入口：添加入口节点、从模板开始、导入 JSON。
- 新建后自动创建一个空 draft 记录，避免“暂无草稿”和“未保存”状态混淆。
- 新活动基础信息编辑面板：名称、code、tenant、workspace、folder。
- 空画布保存前的明确状态提示。

### 2. 节点配置产品化

节点配置 Schema 已经完成第一轮产品化，Event In、Inbox、Match、Reward、Quota 已经从通用字段升级为贴近业务的分组配置：

- 后端 schema 支持 `fieldGroups` 和 `preview`。
- 属性面板按节点类型展示分组配置表单。
- 配置预览展示节点读取内容、输出内容和执行说明。
- 重点节点已有更细的必填项、默认值、枚举选项、校验规则和说明文案。
- Validator 和 Executor 都支持 schema 默认值，旧 draft 不需要手动补齐默认配置也能继续发布和执行。

后续还需要继续把 Normalize、Condition、Qualify、Switch、Fact Query、Code、Limit、Callback、Audit 做到同等业务细节，并补配置错误红点和错误定位体验。

### 3. 变量选择器增强

变量引用不应该主要靠手写字符串。当前已经完成第一轮增强：

- 属性面板可以从上游路径节点的输出变量中选择输入来源。
- 变量选择项展示来源节点、节点 code、输出 key、变量类型和输出说明。
- 已选引用展示来源节点、Output Key、类型、JSONPath。
- JSONPath 使用结构化 `inputMapping` 保存，例如 `{ sourceNodeId, outputKey, path }`。
- 上游输出删除、改名或来源节点不在上游路径时，引用会进入失效状态。
- 发布前后端校验无效变量引用，并阻止发布。
- 点击“定位来源”可以选中并居中来源节点。
- 已补变量选择器搜索和类型过滤，支持按节点名、节点 code、output key、类型和说明筛选上游输出。
- 已补 JSONPath 快捷选择，常用 `$`、事件、主体、奖励和事实路径可点击填充，仍以结构化 `inputMapping.path` 保存。
- 已补批量修复和清空失效引用，能把失效引用重新匹配到同名 output，或移除已不可用的引用。

后续还可以继续补：基于真实样例 output 的树形 JSONPath 选择器、变量引用影响范围分析、跨节点批量替换。

### 4. 连线路由体验增强

Route model 已经完成第一轮前端体验增强：

- 边配置面板按默认路径、条件分支、业务结果、终止/阻断、输出变量分组展示 route type。
- 支持编辑 route key、label、条件表达式、condition ref 和默认分支。
- 条件分支可以一键填充默认表达式。
- 分支节点在检查清单执行后展示 Route Check，可直接看到缺默认分支、缺出口或条件路由问题。
- 边根据 route type 使用不同视觉样式：默认路径、条件分支、终止分支、输出路径有不同颜色/虚线。
- 检查清单返回的边问题会高亮对应连线，运行历史 route trace 继续高亮实际执行路径。
- 顶部浮动工具条层级已调整，属性面板打开时仍可点击检查、发布等操作。
- 已补条件表达式语法提示，常用 decision、status、risk、amount 表达式可一键填充。
- 已补表达式预览，会用当前 route key 构造样例上下文提示是否命中；复杂表达式明确以运行时为准。
- 已补当前边健康检查和“一键设为默认分支”，边级发布检查问题会在路由配置面板内直接展示。

后续还可以继续补：分支节点旁一键补所有缺失默认/出口分支、route rule 批量修复、基于真实 trace 的表达式预览上下文。

### 5. 运行历史完整化

Run history 已经完成第一轮完整化：

- execution 列表支持分页，后端返回 `limit`、`offset`、`nextOffset`、`hasMore`。
- 节点 input/output 大 JSON 支持搜索、高亮、折叠、复制和下载。
- 运行历史面板展示单个 execution 的详情：execution id、event、version id、开始时间、耗时、状态和错误分类。
- 失败原因按错误码和关键词归类为超时、配置错误、路由错误、风控限制、外部依赖、事件接入或业务执行错误。
- execution trace 导出格式统一为 `workflow.execution.trace.v1`，包含 `exportedAt`、execution、node executions 和 route trace。
- 大 execution 日志改为按需加载：节点列表默认请求 summary，不带 input/output；选中节点时再请求单个 node execution 详情。
- 运行历史支持导入已下载的 Trace JSON，进入离线 Trace 查看模式，不依赖后端 execution 查询。
- 错误分类已收口为前端字典，详情面板展示错误分类、错误码和排查建议。

后续还可以继续补：execution 详情独立路由页、超大 JSON 的虚拟滚动、错误码字典从后端配置化返回。

### 6. Debug 工具增强

Debug run / rerun 已完成第一轮增强：

- 支持把当前 payload 保存为本地 Studio 模板，并从模板下拉中重新载入。
- 支持从选中的历史 execution 复制 event payload 来测试当前 draft。
- 从某个节点继续跑时，请求会带上 `sourceExecutionId`；后端会读取 source execution 的上游成功节点 output，作为本次 debug 的初始 node output，再从指定节点继续执行。
- Debug execution 在运行历史中显示 `DEBUG_DRAFT`，并用独立视觉样式区分 production execution。
- Debug run / rerun 成功后会自动执行当前 draft 的真实发布检查，并打开检查清单展示结果。
- 支持从当前运行节点的 input / output 一键填充 Debug payload，或保存成本地 payload 模板。
- 从节点继续跑前展示上游成功节点 output 预览、起点节点和输入缺口，便于判断 resume 上下文是否完整。

后续还可以继续补：团队共享 payload 模板、debug trace 离线 replay。

### 7. 版本管理完善

版本、rollback、archive、diff 已完成第一轮增强：

- Draft vs Active diff 已展示节点/连线新增、删除、修改的结构化明细，并支持点击定位。
- 两个历史 version 可以选择后对比，展示 summary、分组 diff 明细和可定位 diff 项。
- 发布弹窗支持填写发布说明，后端 `WorkflowVersion` 持久化 `releaseNote`。
- 版本生命周期扩展为 `DRAFT`、`ACTIVE`、`SUPERSEDED`、`ARCHIVED`；新 active 发布时旧 active 会变为 `SUPERSEDED`，不会误归档。
- Rollback 会生成新的 active version，并记录 `sourceVersionId`、`action=ROLLBACK` 和审计备注。
- Archive / rollback 确认弹窗支持填写审计备注，版本面板展示最近版本操作记录。
- 版本面板支持查看单个 version 的详情：节点数、连线数、发布人、发布时间、操作类型、来源版本、发布说明和与 active 的差异摘要。
- 版本面板展示 SUPERSEDED / ARCHIVED 治理提示，提醒归档旧 superseded version。

后续还可以继续补：后端版本 diff API、版本详情独立路由页、版本操作审计列表从后端 audit log 拉取、SUPERSEDED 自动清理策略。

### 8. Workspace Studio 首页设计

Workspace / Folder / Workflow 的 Studio 首页已经从简单列表升级为工作台底座：

- workspace / folder / workflow 以文件管理视图组织，支持列表、网格和紧凑模式。
- 侧栏支持 workspace 切换、folder 树、最近编辑、收藏、归档、成员、资源、审计和存储入口。
- 主内容区支持 activity 搜索、状态/类型/可见性/负责人筛选、排序、分页、批量移动、批量归档和批量权限。
- 首页增加工作台概览，集中展示流程、文件夹、收藏、归档统计，以及最近编辑、收藏、归档快捷列表。
- 最近编辑、收藏、归档入口会同步侧栏 active 状态和内容区说明，不再只是隐式过滤。
- 空画布入口已支持添加入口节点、从模板开始、导入 JSON，并区分未保存草稿和空 draft 状态。

后续如果要继续增强，重点是 folder 拖拽移动的手感、跨 workspace 移动确认，以及真实多人协作下的最近访问记录。

### 9. Executor 生产能力

Executor 生产能力已经完成第一轮增强，重点从“能跑”推进到“可诊断、可恢复、可分页读取”：

- DAG 编译返回结构化错误，覆盖缺失端点、缺失入口、重复节点、重复连线 ID、不可达节点和不支持的环。
- 发布前 validator 与 executor 编译器都能拦截关键 DAG 结构问题，避免运行时才把错误节点标记为 skipped。
- 节点执行策略支持 timeout、maxRetries / maxAttempts、retryBackoffSeconds、concurrencyGroup、concurrencyLimit。
- 当前 executor 仍保持单 execution 串行执行，但会把并发策略写入节点 output，明确 `SERIAL_PER_EXECUTION` 和未跨 execution 强制并发。
- cancel 在节点间和最终落库前都会检查，避免最后一个节点执行期间的取消被成功状态覆盖。
- resume 支持从失败/取消 execution 生成新的 `RESUME` execution，并复用上游成功节点 output 和已提交业务副作用上下文。
- node execution 列表支持 `limit/offset` 分页和 `payload=summary`，大 trace 可以先拉 summary，再按需拉节点详情。
- 单元测试覆盖 DAG 编译问题、retry/concurrency policy、resume trace、node trace 分页和核心 handler 稳定输出。

后续如果继续增强，重点是真并行 ready-queue、group semaphore、分支 barrier、handler 幂等审计和更大规模 trace 的存储分片。

### 10. 模板体系实用化

模板体系已经完成第一轮实用化：

- 模板面板支持目录 + 预览两栏视图，可查看节点数、连线数、节点输出和更新时间。
- 模板预览会做基础合法性检查，缺入口、空模板或断连线会展示阻塞标记，合法模板才能应用。
- 从模板创建 draft 时支持选择业务参数：事件类型、奖励金额、币种、Quota Code、次数限制、Callback Provider。
- 后端应用模板时会把业务参数写入对应节点 config，并把 `activityId`、`workspaceId`、`folderId` 和 `templateParameters` 写入 workflow metadata。
- 保存当前 workflow 为团队模板时可以选择 workspace、folder 和可见性，保存后的模板 definition metadata 会保留这些信息。
- 模板版本管理 UI 已接入 `/draft-templates/{templateId}/versions`，可以选择历史版本创建 draft。

后续还可以继续补：模板详情独立页面、模板参数 schema 化、模板合法性改为后端真实 validation、模板版本 diff、模板收藏/归档和按 workspace 权限过滤的模板目录。

### 建议下一步

暂时不继续做权限相关内容。建议优先做更影响“运行起来”的三件事：

1. 空画布起步体验：空状态按钮、添加入口节点、从模板开始。
2. 新活动详情配置：新建后能编辑活动基础信息。
3. 节点配置完整化：继续把 Normalize、Condition、Qualify、Switch、Fact Query、Code、Limit、Callback、Audit 做到和重点节点同等细节。

## 阶段 1：Workflow 基础校验

目标：防止无效 workflow 被发布或执行。

需要完善：

- 校验入口节点是否存在。
- 校验节点 ID 是否唯一。
- 校验连线的 source / target 是否真实存在。
- 校验所有必要节点是否能从入口节点到达。
- 发布前检测不支持的环。
- 检测孤岛节点。
- 检测同一个输出点是否出现不允许的重复连线。
- 校验 workflow metadata 里是否包含 `activityId`、`workspaceId`、`folderId`。

后端工作：

- 增加 workflow validation service。
- 增加 validation result 模型，包含 severity、code、message、nodeId、edgeId、path。
- 增加校验当前 draft 的 API。
- 发布前强制执行校验。

前端工作：

- 发布检查清单展示真实校验结果。
- 高亮有问题的节点和连线。
- 点击检查项可以定位到对应节点或连线。

验收标准：

- 无效草稿可以保存。
- 无效草稿不能发布。
- 校验错误必须稳定、明确、可定位。

## 阶段 2：节点配置 Schema

目标：每类节点都要有结构化配置，而不是临时字段或自由文本。

当前进度：

- Event In、Inbox、Match、Reward、Quota 已完成产品化配置分组。
- WorkflowNodeConfigSchema 已支持 `fieldGroups` 和 `preview`。
- 前端属性面板已按分组渲染配置表单，并展示读取/输出预览。
- 发布校验会使用 schema 必填项、类型、枚举和校验规则。
- Executor 读取节点 config 时会合并 schema 默认值，保证 handler 能拿到稳定配置结构。

需要完善：

- 每类节点定义自己的配置 schema。
- schema 支持字段类型、名称、说明、默认值、是否必填、枚举选项、校验规则。
- 前端属性面板根据 schema 渲染配置表单。
- 节点配置保存到 workflow draft JSON。
- 发布前校验节点配置。

需要覆盖的节点：

- Event In
- Inbox
- Normalize
- Match
- Condition
- Qualify
- Switch
- Fact Query
- Code
- Reward
- Limit
- Quota
- Callback
- Audit

验收标准：

- 每个节点都能在属性面板里完整配置。
- 缺少必填配置时发布前可见。
- 配置结构稳定，可直接交给 executor handler 使用。

## 阶段 3：变量系统

目标：节点输入应该从上游节点输出中选择，而不是手写字符串。

当前进度：

- 每类节点已经通过 schema 声明输入和输出变量。
- 前端输入映射支持从上游路径节点输出中选择变量。
- 变量引用以结构化 `sourceNodeId / outputKey / path` 保存到 draft JSON。
- 属性面板展示变量类型、来源节点、输出 key 和 JSONPath。
- 无效引用会在属性面板显示失效状态。
- 发布前会校验 source node、output key、JSONPath 和上游路径合法性。
- Executor 可以从上游 `node_execution` output 中按 outputKey 和 JSONPath 解析变量。
- 点击变量引用可以定位到来源节点。

需要完善：

- 每类节点声明自己的输出变量。
- 每类节点声明自己接受的输入变量。
- 变量选择器展示可用的上游输出。
- 变量引用用结构化格式保存。
- 发布前校验变量引用是否有效。
- 上游输出删除或重命名后，下游引用要变成无效状态。

推荐变量引用格式：

```json
{
  "sourceNodeId": "node_inbox",
  "outputKey": "event_context",
  "path": "$.user.id"
}
```

验收标准：

- 用户不需要手动输入大部分变量名。
- 无效变量引用会阻止发布。
- Executor 可以从 node_execution output 中解析变量。

## 阶段 4：连线路由规则

目标：`IF`、`ELIF`、`ELSE`、`PASS`、`RISK` 这些连线标记要变成真正可执行的 routing rule。

当前进度：

- Edge 已保存结构化 route type、key、label、expression、conditionRef、default。
- 发布前会校验 route type、OUTPUT route key、条件 route 表达式、默认分支、分支出口覆盖和重复出口。
- Executor 已按 route rule 决定下一步节点，并在运行历史中记录 route trace。
- 前端边配置面板已升级为分组 route type 编辑器。
- 默认分支、条件分支、终止分支、输出路径在画布上有视觉区分。
- 检查清单返回的 route 问题会高亮边，并在分支节点旁显示 Route Check。

需要完善：

- Edge 保存结构化 route type。
- 条件连线保存表达式或条件引用。
- 分支节点校验出口覆盖情况。
- 默认分支必须显式存在。
- Executor 按照 route rule 决定下一步节点。

后端工作：

- 定义 route model。
- 发布前校验 route model。
- Executor 增加 route evaluation。

前端工作：

- 增加边配置面板。
- 支持编辑条件、route label、默认分支。
- 默认分支、条件分支、终止分支要有视觉区分。

验收标准：

- 分支不只是画布上的 label。
- Executor 执行路径必须和画布展示一致。

## 阶段 5：真实发布检查清单

目标：发布不是简单按钮，而是受控流程。

检查项包括：

- 入口节点存在。
- 节点和连线合法。
- 所有必填节点配置已完成。
- 所有变量引用有效。
- 所有分支路由合法。
- Reward、Quota、Limit 等风险节点配置完整。
- workflow 没有不支持的环。
- workflow 可以编译成 executor plan。

前端工作：

- 替换目前静态检查清单。
- 展示 pass、warning、block 状态。
- 有 block 错误时禁用发布。

验收标准：

- 检查清单来自后端真实 validation。
- 用户能从检查清单定位并修复所有阻塞项。

## 阶段 6：版本管理

目标：workflow version 要可查看、可比较、可恢复。

需要完善：

- 展示当前 draft 状态。
- 展示当前 active published version。
- 展示历史 versions。
- 对比 draft 和 active version。
- 对比两个历史 version。
- 支持 rollback。
- 支持 archive 旧版本。

验收标准：

- 已发布 workflow 可以回滚。
- 发布前可以看出和线上版本的差异。
- draft 和 active version 的状态清晰区分。

## 阶段 7：执行引擎增强

目标：Executor 要支持生产级可靠执行。

需要完善：

- 将 workflow definition 编译为可执行 DAG。
- 按拓扑顺序执行节点。
- 支持分支路由。
- 支持 skipped 节点状态。
- 支持节点超时。
- 支持节点失败重试。
- 支持 execution 幂等 key。
- 支持取消 execution。
- 在可行情况下支持恢复执行。
- 持久化 execution 和 node_execution 日志。

验收标准：

- Executor 可以从一次事件触发执行已发布版本。
- 失败节点能记录明确失败原因。
- 分支和 skipped 节点能在日志里看到。

## 阶段 8：节点 Handler 完善

目标：每个可视化节点都有对应生产 handler。

每个 handler 需要具备：

- 输入校验。
- 配置校验。
- 业务执行。
- 输出 payload。
- 错误码。
- 超时行为。
- 重试行为。
- 审计字段。

建议优先级：

1. Event In
2. Inbox
3. Normalize
4. Match
5. Condition
6. Qualify
7. Reward
8. Limit
9. Quota
10. Fact Query
11. Switch
12. Code
13. Callback
14. Audit

验收标准：

- 画布上的每类节点都能执行。
- 每个 handler 有单元测试。
- handler 输出能被下游变量引用。

## 阶段 9：运行历史详情

目标：用户可以在 Studio 里看清楚一次 workflow 到底怎么跑的。

已完成：

- 按 activity 和 workflow version 查看 execution 列表。
- execution 详情面板。
- 画布上覆盖节点运行状态。
- 查看节点 input payload。
- 查看节点 output payload。
- 展示错误原因和上下文。
- 展示节点耗时和 retry 次数。
- 展示分支 route trace。
- execution 列表分页。
- 大 node payload 按需加载。
- payload 支持搜索、复制、下载和折叠。
- trace 导出格式标准化。

验收标准：

- 一次失败执行可以不查后端日志，直接在 Studio 里定位问题。
- 用户能看到实际走了哪条分支。

## 阶段 10：调试工具

目标：用户可以在发布前测试 workflow 逻辑。

需要完善：

- 手动输入测试 payload 触发 test run。
- 支持 payload 模板。
- 支持直接运行 draft workflow。
- 支持重跑某一次 execution。
- 条件允许时支持从某个节点继续跑。
- 支持导出 execution trace。

验收标准：

- 用户可以用样例事件验证 draft workflow。
- debug run 和 production execution 有明确区分。

## 阶段 11：Workspace 权限接入

目标：workspace、folder、member、share、audit 模型真正控制 Studio 行为。

需要完善：

- Workspace 成员管理 UI。
- Folder 可见性和权限控制。
- Share link 管理 UI。
- Audit log 展示。
- 按角色控制按钮是否显示。
- 后端 API 按角色鉴权。

验收标准：

- Viewer 不能编辑和发布。
- Editor 可以编辑 draft。
- Admin 可以管理 workspace 和 folder 设置。
- Owner 可以管理成员和分享。

## 阶段 12：模板和复用

目标：用户可以从成熟模式快速创建 workflow。

当前进度：

- Workflow template catalog 已接入后端内置模板和保存模板。
- Node preset catalog 已接入并在模板面板展示。
- 支持从模板创建 workflow draft，并在创建时填写业务参数。
- 支持保存当前 workflow 为团队模板，并选择 workspace、folder、可见性。
- 模板版本管理 UI 已支持加载和选择历史版本。
- 模板卡片和预览区展示基础合法性标记。

后续需要完善：

- 模板参数 schema 化，按模板动态展示业务参数。
- 模板合法性改为后端真实 validation，而不是前端基础结构检查。
- 模板版本 diff 和 rollback。
- 保存模板时支持更完整的描述、标签、分类和归档。

推荐模板：

- 完整活动链路模板。
- 简化奖励链路模板。
- 风控和资格校验链路模板。
- Quota 保护奖励链路模板。
- Fact Query 数据增强链路模板。

验收标准：

- 新 workflow 不需要每次从空白画布开始。
- 模板生成的 draft 默认合法。
- 团队模板可以按 workspace/folder 归属管理。

## 阶段 13：前端体验细节

目标：Studio 在真实编辑过程中要可靠、明确、顺手。

需要完善：

- dirty 状态。
- 保存中状态。
- 已保存状态。
- 保存失败状态。
- 发布中状态。
- 发布失败详情。
- 自动保存。
- 快捷键。
- 节点配置错误红点。
- 连线错误红点。
- 点击错误定位节点或边。
- 更好的空状态。
- Workspace / Folder breadcrumb 导航。

验收标准：

- 用户始终知道当前改动是否已保存。
- 错误显示在需要修复的位置。

## 近期完成：Workspace Studio 首页设计

目标：让 Studio 首页成为真正的项目工作台，而不是简单活动列表。

已完成：

- 首页改成文件管理器式布局：左侧 workspace，中央 folder / workflow 工作区，右侧最近编辑和 workspace ops。
- 支持 workspace 根目录和 folder 级导航，breadcrumb 可以回到根目录。
- 支持 activity 搜索、状态过滤、排序和收藏过滤。
- 支持最近编辑列表。
- 支持本地收藏和本地归档视图，归档后可通过 Archived 筛选恢复。
- 支持 folder 拖拽移动，拖到另一个 folder 会调用后端 folder PUT 接口更新 `parentId` 和 `sortOrder`。
- 支持拖到 workspace 根目录区域，把 folder 移回根目录。
- E2E 覆盖首页文件管理视图、搜索、收藏、归档和 folder 拖拽移动。

后续仍可增强：

- Activity 收藏/归档从本地偏好升级为后端用户偏好。
- Folder 树增加展开/折叠和批量移动。
- Folder / workflow 增加右键菜单、重命名、删除确认。
- 最近编辑改为后端真实访问记录，而不只依赖 activity 更新时间。

## 近期完成：Executor 生产能力增强

目标：让已发布 workflow 的执行、诊断、恢复和大日志读取更接近生产可用。

已完成：

- DAG 编译增加结构化错误，返回稳定 `severity/code/message/nodeId/edgeId/path`，覆盖缺失端点、缺失入口和不支持的环。
- DAG 编译和发布前 validator 补齐重复连线 ID、不可达节点拦截，避免结构错误进入运行时。
- execution output 增加 `compileIssues`，编译失败时前端和 API 可以定位具体 DAG 问题。
- 节点执行策略扩展：支持 `retryBackoffSeconds`、`concurrencyGroup`、`concurrencyLimit`。
- 节点执行 output 暴露实际使用的 retry / concurrency 策略，方便运行历史排查。
- 重试策略支持 backoff，且受 context cancel 控制。
- execution cancel 在最终落库前会再次检查，避免最后节点执行期间的取消被成功状态覆盖。
- 新增 `POST /admin/v1/workflow/executions/{id}/resume`，对 FAILED / CANCELED execution 创建新的 `RESUME` execution。
- resume 会读取原 execution 的 event、workflow version 和上游成功节点 output，从失败节点或指定节点继续执行。
- node execution 列表支持 `limit/offset` 分页，并返回 `limit/offset/nextOffset/hasMore`。
- `payload=summary` 与分页兼容，大 trace 可以先拉 summary，再按需拉单个 node detail。
- 单元测试覆盖 DAG 编译 issue、retry/concurrency policy、resume trace、node trace 分页。

当前边界：

- 并发策略已进入模型和日志，但 executor 仍按拓扑顺序串行执行，避免动态路由、幂等写和 handler 副作用被并行化破坏。
- 真正并行执行需要后续引入 ready-queue、group semaphore、分支 barrier 和 handler 幂等审计。

## 近期完成：资源管理底座和高级资源

目标：把节点里的 endpoint、表、Redis、MQ、Provider 配置抽离到公共资源管理，并通过权限、审计和 adapter 统一执行。

已完成：

- 资源模型、资源列表、资源详情、资源绑定、资源审计和资源管理入口已接入。
- `state-store + resourceRef` 支持内置 `default_workflow_state`、`workflow_memory`。
- 资源 scope 支持 workspace、folder、activity、node 等边界，binding 支持 operation、namespace、key policy 和角色限制。
- `DATABASE_TABLE` 和 `HTTP_PROVIDER` 已支持第一版执行。
- `REDIS_KV`、`REDIS_HASH`、`REDIS_COUNTER`、`REDIS_LOCK` 已支持第一版资源语义。
- `mq-publish + MQ_TOPIC resourceRef` 已支持出站发布意图和资源审计。
- `fact-query + resourceRef` 已支持资源读和 HTTP Provider CALL 查询，统一输出 `facts`。
- 核心 `outbox/callback + resourceRef` 已支持第一版资源绑定、权限校验、投递/回调计划和资源审计。
- HTTP Provider 安全治理已完成第一版：URL scheme、userinfo、host allowlist、blockedHosts、private network 阻断和 redirect 防护。
- HTTP Provider 已支持 `secretRef/headersRef`、response schema 强校验和 `TestResource dryRun` 探活。
- 公共表已支持 `schema/tableSchema`、`keyMapping`、`valueMapping` 强约束。
- `quota-check + REDIS_COUNTER resourceRef` 与专用 `lock + REDIS_LOCK resourceRef` 已完成第一版。
- `mq-event`、`schedule-event`、`state-event` 已从 planned/explain-only 收口到 trigger adapter 第一版。
- Lua Resource API 已支持 `resource_read/write/upsert/call/publish`，并增加 `maxResourceCalls` / `maxResourceResponseBytes` 限额。
- 高风险资源操作已有第一版 `requireApproval` 信号校验；`requireFormalApproval` 可强制校验当前活动版本正式审批快照。
- CAS 已要求明确版本，避免退化成普通写入。

当前边界：

- Redis 资源当前由 `marketing_storage_state` 承载语义，还不是真实 Redis client。
- `MQ_TOPIC` 当前生成 publish intent；OutboxWorker/MQSender 已有 adapter 边界，生产 Kafka/RabbitMQ/Pulsar SDK transport 需部署注入。
- 资源审批已有活动正式审批快照硬校验，但还没有独立 resource approval order。
- 生产 debug 写入审批仍需结合 Debug/Execution 权限和审批链路。

暂停项：

- 资源管理 P4 治理和运维暂不启动，包括资源版本、引用关系图、健康监控、迁移和停用保护。

## 阶段 14：测试覆盖

目标：随着 workflow 能力变复杂，测试要能保护核心行为。

需要补充：

- Workflow validation 单元测试。
- Workflow HTTP API 测试。
- Executor 单元测试。
- 分支执行测试。
- 变量解析测试。
- 发布和回滚测试。
- 节点 handler 测试。
- 前端 workflow picker 测试。
- 前端 publish checklist 测试。
- draft、publish、execute、inspect 端到端测试。

验收标准：

- `go test ./...` 通过。
- `npm run typecheck:admin-flow` 通过。
- `npm run build:admin-flow` 通过。
- 核心 workflow 流程有自动化测试保护。

## 推荐实施顺序

建议按下面顺序推进：

1. Workflow 基础校验。
2. 节点配置 schema。
3. 变量系统。
4. 连线路由规则。
5. 真实发布检查清单。
6. 版本 diff 和 rollback。
7. Executor 增强。
8. 节点 handler 完善。
9. 运行历史详情。
10. Debug test run。
11. Workspace 权限 UI。
12. 模板系统。
13. 前端体验打磨。
14. 测试覆盖扩展。

这个顺序的原则是：先把 workflow 的合法性和配置基础稳住，再做执行、调试、权限和体验增强。
