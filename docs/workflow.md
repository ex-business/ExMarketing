# 任务流水线 Workflow

## 目标

每个需求都按固定流水线推进：

```text
Task
  -> Analysis
  -> Review
  -> Development
  -> Test
  -> Commit
```

核心原则：

```text
先分析，再审核，再开发，再测试，最后提交。
```

任何涉及金额、Quota、发奖、Outbox、Fact Query、安全边界的任务，都必须严格走完整流程。

## 目录结构

每个任务一个目录：

```text
tasks/
  2026-05-24-add-quota-release/
    task.md
    analysis.md
    review.md
    implementation.md
    test.md
    commit.md
```

模板目录：

```text
tasks/_template/
  task.md
  analysis.md
  review.md
  implementation.md
  test.md
  commit.md
```

## Agent 角色

### Task Owner

职责：

```text
把需求写清楚。
定义目标、范围、非目标、验收标准。
```

产物：

```text
task.md
```

禁止：

```text
不要直接写实现方案。
不要夹带无关重构。
```

### Analysis Agent

职责：

```text
读取 task.md。
读取 README.md / docs/architecture.md / CLAUDE.md。
读取已有接口和数据模型文档。
输出细化需求、业务规则、边界条件、非目标范围、风险和验收方式。
```

产物：

```text
analysis.md
```

禁止：

```text
不改代码。
不写代码。
不提交。
不做实现。
不读取代码。
不扩大任务范围。
```

### Review Agent

职责：

```text
审核 analysis.md。
识别 P0/P1/P2 风险。
判断是否允许进入开发。
```

产物：

```text
review.md
```

规则：

```text
存在 P0/P1 -> BLOCK
只有 P2 或无问题 -> PASS
```

禁止：

```text
不改代码。
不替 Dev Agent 设计新方案，除非当前方案 BLOCK。
```

### Dev Agent

职责：

```text
只按 task.md + analysis.md + review.md 开发。
修改代码、测试和文档。
记录改动文件和行为变化。
```

产物：

```text
implementation.md
```

禁止：

```text
不得绕过 review 的约束。
不得修改任务范围外文件，除非先更新 analysis/review。
不得引入新的 P0/P1 风险。
```

### Test Agent

职责：

```text
运行测试。
运行必要的 PostgreSQL smoke。
记录命令、结果、失败原因。
```

产物：

```text
test.md
```

最低要求：

```bash
go test ./...
go build ./...
```

涉及 PostgreSQL 语义时，必须使用 Docker PostgreSQL 验证。

### Commit Agent

职责：

```text
检查 task/review/test 是否 PASS。
检查 git status。
提交并 push。
记录 commit hash 和 remote。
```

产物：

```text
commit.md
```

提交前置条件：

```text
review.md = PASS
test.md = PASS
无未解释的 git diff
```

## 标准命令

创建任务：

```bash
TASK=tasks/$(date +%F)-short-topic
mkdir -p "$TASK"
cp tasks/_template/*.md "$TASK"/
```

开发前检查：

```bash
git status -sb
go test ./...
go build ./...
```

PostgreSQL 验证：

```bash
docker compose up -d postgres
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
go run ./cmd/exmarketing
```

## 状态流转

```text
NEW
  -> ANALYZED
  -> REVIEW_PASS / REVIEW_BLOCK
  -> IMPLEMENTED
  -> TEST_PASS / TEST_FAIL
  -> COMMITTED
```

如果 review 或 test BLOCK/FAIL：

```text
回到 Analysis 或 Development
更新对应文档
重新 review / test
```

## 适用范围

必须走完整流程：

```text
金额计算
Quota / Budget / Stock
grant_order
outbox
reward callback
Fact Query
鉴权
数据库 schema
活动插件
生产部署
```

可走简化流程：

```text
纯文档 typo
注释修正
README 小补充
```

简化流程仍然要写明：

```text
改了什么
为什么不需要完整 review
是否需要测试
```
