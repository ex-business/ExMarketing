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

已提交当前工作区中除本文件外的本次任务相关代码、文档、任务记录和工作流文件。

主要范围：

- activity 发奖路径的预算、库存、用户次数原子扣减。
- quota 不足时的结构化 409 响应和 `QUOTA_EXCEEDED` 审计。
- memory / PostgreSQL 仓储的 all-or-nothing 发奖事务实现。
- HTTP、memory、PostgreSQL 回归与集成测试。
- task workflow、agent runtime、CLI、管理接口和相关文档。

## Review

~~~text
PASS
~~~

门禁文件：`tasks/2026-05-24-094535/review.md`

## Test

~~~text
PASS
~~~

门禁文件：`tasks/2026-05-24-094535/test.md`

提交前复核命令：

~~~bash
git status -sb
go test ./...
go build ./...
~~~

结果：全部通过。

## Commit

~~~text
hash: fa932d897471527805658b2ba0123929151cd8bc
message: feat: add task workflow and atomic quota reservations
remote: origin https://github.com/ex-business/ExMarketing.git
~~~

状态：本地 `main` 已领先 `origin/main` 1 个提交，未执行 push。

需求：2026-05-24-094535
