# Task

## 标题

建立 ExMarketing 任务流水线模板。

## 背景

希望在工程内尝试按任务进行分析、审核、开发、测试、提交的 Agent 协作流程。

## 目标

- 新增任务流水线说明文档。
- 新增任务模板。
- 新增一个示例任务目录。

## 范围

本次只修改文档和任务模板：

- `docs/workflow.md`
- `tasks/README.md`
- `tasks/_template/*`
- `tasks/2026-05-24-example-workflow/*`

## 非目标

- 不改 Go 代码。
- 不改数据库 schema。
- 不启动服务。

## 风险等级

```text
低
```

## 验收标准

- 文件结构清晰。
- 后续可以复制 `tasks/_template` 创建新任务。

