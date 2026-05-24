# Task

## 标题

一句话描述任务。

## 背景

为什么要做这个任务。

## 目标

- 目标 1
- 目标 2

## 范围

本次允许修改：

- `path/to/file`

## 非目标

本次明确不做：

- 非目标 1

## 风险等级

```text
低 / 中 / 高
```

高风险示例：

```text
金额、Quota、发奖、Outbox、Fact Query、鉴权、数据库 schema
```

## 验收标准

- `go test ./...`
- `go build ./...`
- 如涉及 PostgreSQL，完成本地 smoke

