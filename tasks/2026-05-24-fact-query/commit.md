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

- 已提交 fact query provider registry 相关实现、测试、迁移与 API/数据模型文档更新。
- 提交时未暂存 `tasks/*/runs/` 执行日志。
- `tasks/2026-05-24-webhook-receiver/*` 相关未暂存变更未纳入本次提交。

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: 5be14f0f5b8314ae407a6c0810b80e4881075a83
message: Implement fact query provider registry
remote: origin https://github.com/ex-business/ExMarketing.git
~~~

需求：2026-05-24-fact-query
