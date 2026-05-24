# Analysis

## Agent 指令

你是 Analysis Agent。请读取本目录 task.md、项目 README.md、CLAUDE.md、docs/architecture.md 和相关代码。

只做分析，不改代码，不提交。

## 评分规则

满分 100，低于 70 不允许进入研发：

~~~text
需求清晰度        20
范围可控          20
风险可控          20
验收标准明确      20
架构一致性        20
~~~

## 现状

本需求来自 `requirements/backlog.json`，当前状态为 `ANALYSIS_READY`，`taskDir` 已指向 `tasks/2026-05-24-example-automated-requirement`。该目录已经包含 `task.md`、`analysis.md`、`review.md`、`implementation.md`、`test.md`、`commit.md`、`test-plan.md`、`agent-prompts.md`、`agent-status.json` 和 `runs/`。

现有自动化入口有两条：

- CLI：`cmd/workflowctl/main.go` 的 `add` 写入 `requirements/backlog.json`，`intake` 扫描 `NEW` 需求，设置 `TaskDir`，调用 `workflow.CreateTaskFiles`，并把状态推进到 `ANALYSIS_READY`。
- Admin API：`internal/httpapi/workflow_admin.go` 的 `POST /admin/v1/workflow/requirements` 创建需求，`POST /admin/v1/workflow/intake` 执行同样的 intake 文件生成逻辑。

任务文件生成逻辑在 `internal/workflow/taskfiles.go`：

- `CreateTaskFiles` 创建任务目录。
- 渲染 `task.md`、各 Agent 产物模板、`test-plan.md`、`agent-prompts.md`、`agent-report.md`。
- 写入 `agent-status.json`，初始包含 `analysis`、`review`、`test-plan`、`dev`、`test`、`commit` 六个 Agent 的 WAITING 状态。

Agent 运行逻辑在 `internal/workflow/agent.go`：

- `PromptForAgent` 从 `agent-prompts.md` 按 Agent 标题提取 prompt。
- `ResolveAgent` 根据需求状态选择下一个 Agent。
- `EvaluateAgentResult` 解析 `analysis.md` 的 Score 和 Decision，并按 `AnalysisPassScore()` 决定是否进入 `REVIEW_READY`。

已有测试覆盖了部分能力：

- `internal/workflow/agent_test.go` 覆盖 prompt 提取、analysis 结果解析、Agent 成功/失败状态。
- `internal/httpapi/handler_test.go` 覆盖 Admin 创建需求、intake、运行 Agent、pipeline 阻断。

当前主要缺口是行为一致性和验收覆盖：

- 当前任务目录中的 prompt 和评分规则使用 `70` 分门槛，但 `docs/automation.md`、`internal/workflow/taskfiles.go` 和 `AnalysisPassScore()` 默认值描述/实现为 `80`。这会导致不同入口、不同时间生成的任务对 PASS 门槛认知不一致。
- `TestPlanAgent` 已进入代码和部分文档，但本任务已有 `agent-prompts.md` 的 Dev/Test prompt 仍是旧格式，说明旧任务可通过 `EnsureTaskFiles` 补齐 test-plan prompt，但不会重写已有 Dev/Test prompt。
- 现有 HTTP intake 测试只断言状态，没有直接断言所有任务产物和 `agent-prompts.md` 内容完整。

## 目标理解

本任务要验证并固化“需求列表接收后自动生成任务目录和 Agent 提示词”的最小自动化闭环：

1. 新需求先进入 `requirements/backlog.json`，状态为 `NEW`。
2. intake 后自动创建 `tasks/<requirement-id>/`。
3. 目录内自动生成所有流水线产物模板。
4. 自动生成可被 `run-agent` 读取的 `agent-prompts.md`。
5. 自动生成 `agent-status.json`，让 CLI/API/Admin 页面能看到每个 Agent 的状态。
6. analysis 阶段的评分门槛与任务提示、review 提示、运行时判定保持一致。

该需求不涉及营销活动、规则、奖励、Quota、Outbox、Fact Query 或数据库 schema。

## 方案

建议进入研发，做小范围一致性修正和测试补强，不改业务链路。

1. 统一 Analysis PASS 门槛

   本任务明确使用 `Score >= 70` 才能 PASS，建议把自动化需求流水线的默认门槛统一为 70：

   - `internal/workflow/agent.go`：`AnalysisPassScore()` 默认值从 80 调整为 70，保留 `EX_MARKETING_ANALYSIS_PASS_SCORE` 覆盖能力。
   - `internal/workflow/taskfiles.go`：`renderAnalysis`、`renderReview`、`renderAgentPrompts` 中的 80 文案统一为 70，或改成基于 `AnalysisPassScore()` 渲染，避免再次漂移。
   - `docs/automation.md`：同步默认门槛说明。

2. 固化任务目录和 prompt 生成行为

   保持 `CreateTaskFiles` 作为唯一生成入口，不新增包边界。补充单元测试断言：

   - `CreateTaskFiles` 会创建 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md`、`test.md`、`commit.md`、`agent-prompts.md`、`agent-report.md`、`agent-status.json`。
   - `agent-prompts.md` 包含 Analysis、Review、Test Plan、Dev、Test、Commit 六个 Agent prompt。
   - Analysis/Review prompt 的分数门槛与默认 `AnalysisPassScore()` 一致。
   - `agent-status.json` 初始化六个 Agent，状态均为 `WAITING`，output 指向对应产物文件。

3. 补强 intake 验收测试

   在 HTTP 或 workflow 层测试中补充对 intake 后文件存在和 prompt 内容的断言。优先放在 `internal/workflow`，因为生成逻辑本身不依赖 HTTP；HTTP 测试只需保持入口可用。

4. 可选清理

   当前已有旧任务的 `agent-prompts.md` 不需要批量重写，避免扩大范围。`EnsureTaskFiles` 保持只补缺失文件/缺失 Test Plan prompt 的兼容策略即可。

## 改动范围

预计修改：

- `internal/workflow/agent.go`
- `internal/workflow/taskfiles.go`
- `internal/workflow/agent_test.go`
- `docs/automation.md`

可选修改：

- `internal/httpapi/handler_test.go`，仅当 workflow 层测试不足以覆盖 intake 入口时补充。

不应修改：

- `internal/engine`
- `internal/marketing`
- `internal/postgres`
- `internal/reward`
- `internal/worker`
- `db/migration`
- OpenAPI/Admin 活动接口业务行为

## 数据模型影响

无数据库表、字段、索引、状态枚举变更。

任务状态枚举代码不需要新增；现有 `NEW -> ANALYSIS_READY -> REVIEW_READY -> TEST_PLAN_READY -> DEV_READY -> TEST_EXEC_READY -> COMMIT_READY -> COMMITTED` 已覆盖本需求。

## API 影响

不新增 API。

已有接口行为保持：

- `POST /admin/v1/workflow/requirements`
- `POST /admin/v1/workflow/intake`
- `GET /admin/v1/workflow/requirements`
- `GET /admin/v1/workflow/requirements/{id}`
- `POST /admin/v1/workflow/requirements/{id}/run-agent`

若默认 Analysis PASS 门槛统一为 70，`GET /admin/v1/workflow/config` 返回的 `analysisPassScore` 默认值会从 80 变为 70；这是本需求评分规则下的预期行为。

## 安全和账务影响

无账务影响：

- 不影响金额计算。
- 不影响 Quota / Budget / Stock。
- 不影响 grant_order。
- 不影响 Outbox。
- 不影响 Fact Query。
- 不影响 reward callback。

安全影响低：

- 不新增外部业务数据库连接。
- 不引用外部业务代码包。
- 不增加生产发奖或资金类操作。
- 不改变 Agent 命令执行模型；`EX_MARKETING_AGENT_CMD` 仍是本地开发工作流能力。

## 风险点

- P2：默认门槛从 80 统一到 70 会改变未设置 `EX_MARKETING_ANALYSIS_PASS_SCORE` 时的自动流转结果。该变化与本任务评分规则一致，但必须同步文档和测试，避免 Review Agent 仍按旧门槛阻断。
- P2：模板渲染和运行时门槛如果继续分别硬编码，后续容易再次漂移。建议测试中直接覆盖 prompt 文案和 `AnalysisPassScore()` 的一致性。
- P2：旧任务目录不会被 `CreateTaskFiles` 重写。对历史任务只应通过 `EnsureTaskFiles` 做兼容补齐，不应自动覆盖人工写过的产物。
- P3：Admin API 测试目前对生成文件内容断言不足，可能漏掉 prompt 缺失或 Agent 状态缺失问题。

无 P0/P1 风险。

## 验收方式

最低命令：

~~~bash
go test ./...
go build ./...
~~~

建议增加并通过的定向测试：

~~~bash
go test ./internal/workflow -run 'TestCreateTaskFiles|TestPromptForAgent|TestEvaluateAgentResult'
go test ./internal/httpapi -run 'TestCreateAndIntakeWorkflowRequirement|TestGetWorkflowRequirement'
~~~

手工 smoke：

~~~bash
tmpdir="$(mktemp -d)"
cd "$tmpdir"
cp -R /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing/. .
go run ./cmd/workflowctl add --title "auto generated prompts" --description "verify generated task dir and prompts" --risk 低
go run ./cmd/workflowctl intake
go run ./cmd/workflowctl list
find tasks -maxdepth 2 -type f | sort
~~~

验收断言：

- 新需求从 `NEW` 变为 `ANALYSIS_READY`。
- `tasks/<id>/` 自动存在。
- 目录包含 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md`、`test.md`、`commit.md`、`agent-prompts.md`、`agent-report.md`、`agent-status.json`。
- `agent-prompts.md` 包含六个 Agent prompt。
- `agent-status.json` 包含六个 Agent，初始状态为 `WAITING`。
- Analysis/Review prompt、analysis 模板、review 模板、运行时 `AnalysisPassScore()` 默认门槛一致，均为 70；设置 `EX_MARKETING_ANALYSIS_PASS_SCORE` 时运行时判定按环境变量覆盖。

不需要 PostgreSQL smoke。

## Score

~~~text
84
~~~

评分拆分：

~~~text
需求清晰度        15/20
范围可控          19/20
风险可控          18/20
验收标准明确      16/20
架构一致性        16/20
~~~

扣分原因：

- task.md 的目标、范围、验收标准仍是占位文本，需要 Analysis Agent 补齐。
- 现有代码/文档/当前任务文件之间存在 70 与 80 分门槛不一致。
- 已有测试覆盖入口，但对完整生成产物和 prompt 内容的断言不够直接。

## Decision

~~~text
PASS
~~~

理由：Score >= 70，需求低风险，改动集中在 workflow 自动化模板、阈值一致性和测试补强，不触碰营销核心业务、账务、数据库或外部集成，方案可控。

需求：2026-05-24-example-automated-requirement
