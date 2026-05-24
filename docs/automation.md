# 自动化需求流水线

## 目标

用一个需求列表驱动 Agent 流程：

```text
新增需求
  -> intake 接收任务
  -> Analysis Agent 分析和打分
  -> Review Agent 审核
  -> Test Plan Agent 先写测试用例
  -> Dev Agent 开发并跑基础验证
  -> Test Agent 执行完整测试
  -> Commit Agent 提交
```

当前实现是文件驱动的最小版本：

```text
requirements/backlog.json     需求列表
cmd/workflowctl               需求接收、状态流转和 Agent 运行工具
tasks/<requirement-id>/       每个需求的 Agent 产物目录
```

## 需求状态

```text
NEW
ANALYSIS_READY
ANALYSIS_DONE
REVIEW_READY
REVIEW_PASS
REVIEW_BLOCK
DEV_READY
DEV_DONE
TEST_READY
TEST_PASS
TEST_FAIL
COMMIT_READY
COMMITTED
```

## 新增需求

```bash
go run ./cmd/workflowctl add \
  --title "add quota release" \
  --description "grant callback failed 时释放 quota 冻结额度" \
  --risk 高
```

命令会写入：

```text
requirements/backlog.json
```

## 接收任务

```bash
go run ./cmd/workflowctl intake
```

`intake` 会扫描 `NEW` 需求，生成：

```text
tasks/<requirement-id>/
  task.md
  analysis.md
  review.md
  implementation.md
  test.md
  commit.md
  agent-prompts.md
  agent-status.json
```

并把需求状态改为：

```text
ANALYSIS_READY
```

`agent-status.json` 会初始化五个 Agent 的可见状态，初始值都是 `WAITING`。

## Analysis Agent

读取：

```text
tasks/<id>/task.md
README.md
CLAUDE.md
docs/architecture.md
相关代码
```

写入：

```text
tasks/<id>/analysis.md
```

评分规则：

```text
需求清晰度        20
范围可控          20
风险可控          20
验收标准明确      20
架构一致性        20
```

默认低于 80 分不得进入研发。可通过 `EX_MARKETING_ANALYSIS_PASS_SCORE` 调整。

完成后更新状态：

```bash
go run ./cmd/workflowctl set-status --id <id> --status REVIEW_READY --score 85
```

也可以让 `workflowctl` 自动调用 Agent：

```bash
EX_MARKETING_AGENT_CMD='/Applications/Codex.app/Contents/Resources/codex exec --cd . --sandbox danger-full-access --ask-for-approval never -' \
  go run ./cmd/workflowctl run-agent --id <id>
```

`run-agent` 会根据当前状态自动选择 Agent：

```text
ANALYSIS_READY -> Analysis Agent
REVIEW_READY   -> Review Agent
TEST_PLAN_READY -> Test Plan Agent
TEST_PLAN_DONE -> Dev Agent
DEV_READY      -> Dev Agent
TEST_EXEC_READY -> Test Agent
TEST_READY     -> Test Agent
COMMIT_READY   -> Commit Agent
```

执行时会读取：

```text
tasks/<id>/agent-prompts.md
```

并把对应 prompt 通过 stdin 交给 `EX_MARKETING_AGENT_CMD`。Agent 输出会保存到：

```text
tasks/<id>/runs/<timestamp>-<agent>.log
```

Agent 执行状态会同步写入：

```text
tasks/<id>/agent-status.json
```

然后 `workflowctl` 会读取 Agent 写回的产物文件，自动推进状态：

```text
analysis.md: Score >= EX_MARKETING_ANALYSIS_PASS_SCORE 且 Decision PASS -> REVIEW_READY
analysis.md: 否则 -> REVIEW_BLOCK
review.md:   Decision PASS -> TEST_PLAN_READY
review.md:   否则 -> REVIEW_BLOCK
test-plan.md: Decision PASS -> DEV_READY
test-plan.md: 否则 -> TEST_FAIL
dev agent:   命令成功 -> TEST_EXEC_READY
test.md:     Result PASS -> COMMIT_READY
test.md:     否则 -> TEST_FAIL
commit agent: 命令成功 -> COMMITTED
```

## 查看 Agent 是否在干活

本地服务启动后，直接打开可视化看板：

```text
http://localhost:8080/admin/
```

看板会显示：

```text
需求列表
当前流程状态
Analysis / Review / Dev / Test / Commit 五段时间线
每个 Agent 的 WAITING / RUNNING / DONE / FAILED
产物文件内容
日志路径
新增需求
接收 NEW 需求
运行下一个 Agent 或指定 Agent
一键自动运行 Analysis -> Review -> Test Plan -> Dev -> Test -> Commit
查看 Git Diff
COMMIT_READY 后执行本地提交
生成 tasks/<id>/agent-report.md 汇总文档
```

查看结构化台账：

```bash
cat tasks/<id>/agent-status.json
```

通过命令查看：

```bash
go run ./cmd/workflowctl agent-status --id <id>
```

输出会包含：

```text
agent      status    started              finished             output
analysis   DONE      2026-05-24T...       2026-05-24T...       tasks/<id>/analysis.md
           log: tasks/<id>/runs/20260524T...-analysis.log
review     RUNNING   2026-05-24T...       -                    tasks/<id>/review.md
dev        WAITING   -                    -                    tasks/<id>/implementation.md
test       WAITING   -                    -                    tasks/<id>/test.md
commit     WAITING   -                    -                    tasks/<id>/commit.md
```

状态含义：

```text
WAITING  还没开始
RUNNING  已开始执行
DONE     执行成功，且产物解析通过
FAILED   命令失败，或产物无法解析
```

## Review Agent

读取：

```text
tasks/<id>/analysis.md
```

写入：

```text
tasks/<id>/review.md
```

规则：

```text
P0/P1 -> BLOCK
Score < EX_MARKETING_ANALYSIS_PASS_SCORE -> BLOCK
否则 PASS
```

通过后：

```bash
go run ./cmd/workflowctl set-status --id <id> --status DEV_READY
```

不通过：

```bash
go run ./cmd/workflowctl set-status --id <id> --status REVIEW_BLOCK
```

## Dev Agent

读取：

```text
task.md
analysis.md
review.md
```

开发完成后写：

```text
implementation.md
```

然后：

```bash
go run ./cmd/workflowctl set-status --id <id> --status TEST_READY
```

## Test Agent

读取：

```text
task.md
analysis.md
implementation.md
```

至少执行：

```bash
go test ./...
go build ./...
```

涉及 PostgreSQL 时执行 smoke。

通过后：

```bash
go run ./cmd/workflowctl set-status --id <id> --status COMMIT_READY
```

失败：

```bash
go run ./cmd/workflowctl set-status --id <id> --status TEST_FAIL
```

## Commit Agent

提交前检查：

```text
review.md = PASS
test.md = PASS
status = COMMIT_READY
```

提交后：

```bash
go run ./cmd/workflowctl set-status --id <id> --status COMMITTED
```

## 下一步自动化

当前版本已经支持本机自动跑 Agent。后续可以增强为：

后续可以接：

```text
GitHub Issues -> requirements/backlog.json
GitHub Actions 定时运行 workflowctl intake
Codex/Claude/OpenAI 子 Agent 根据 agent-prompts.md 自动执行
PR comment 回写状态
CI 成功后自动进入 COMMIT_READY
```
