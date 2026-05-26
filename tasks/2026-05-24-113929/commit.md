# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。

提交前必须检查：

~~~bash
git status -sb
go test ./...
go build ./...
~~~

## Scope

活动主配置模型提交：

- 完善 `marketing_activity` 数据模型、Repository、PostgreSQL 持久化和内存实现。
- 增加活动创建、查询、更新 HTTP 接口和校验。
- 增加 `(tenant_id, activity_code)` 唯一约束迁移与冲突处理。
- 补充活动配置相关单元测试、PostgreSQL 集成测试和接口文档。

## Review

~~~text
PASS
~~~

`tasks/2026-05-24-113929/review.md` Decision 为 PASS。

## Test

~~~text
PASS
~~~

`tasks/2026-05-24-113929/test.md` Result 为 PASS。

提交前检查：

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

结果：

~~~text
PASS
~~~

## Commit

~~~text
hash: 53aa892f5b83eebe050e95de203e569b9d1c94cf
message: feat: add marketing activity configuration model
remote: not pushed
~~~

需求：2026-05-24-113929
