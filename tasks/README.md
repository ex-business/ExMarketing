# Tasks

这里记录 ExMarketing 的任务流水线产物。

每个任务一个目录：

```text
tasks/YYYY-MM-DD-short-topic/
  task.md
  analysis.md
  review.md
  implementation.md
  test.md
  commit.md
  agent-prompts.md
  agent-status.json
  runs/
```

创建新任务：

```bash
go run ./cmd/workflowctl add --title "short topic" --description "需求描述"
go run ./cmd/workflowctl intake
go run ./cmd/workflowctl agent-status --id <requirement-id>
```

流程说明见：

```text
docs/workflow.md
```
