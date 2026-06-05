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

需要完善：

- 按 activity 和 workflow version 查看 execution 列表。
- execution 详情页或详情面板。
- 画布上覆盖节点运行状态。
- 查看节点 input payload。
- 查看节点 output payload。
- 展示错误原因和上下文。
- 展示节点耗时和 retry 次数。
- 展示分支 route trace。

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

需要完善：

- Workflow template catalog。
- Node preset catalog。
- 从模板创建 workflow。
- 保存当前 workflow 为模板。
- 模板版本管理。

推荐模板：

- 完整活动链路模板。
- 简化奖励链路模板。
- 风控和资格校验链路模板。
- Quota 保护奖励链路模板。
- Fact Query 数据增强链路模板。

验收标准：

- 新 workflow 不需要每次从空白画布开始。
- 模板生成的 draft 默认合法。

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

