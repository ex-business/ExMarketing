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

提交统一营销活动配置相关实现与配套任务记录。

已暂存并提交当前跟踪文件改动；未暂存 `tasks/*/runs/` 执行日志；未暂存其他任务下未跟踪的 `commit.md`。

提交前检查：

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Review

~~~text
PASS
~~~

## Test

~~~text
PASS
~~~

## Commit

~~~text
hash: 4a0c2331200f69b190348840d6d96004f8049d07
message: feat: add unified marketing activity config
remote: not pushed; local main is ahead of origin/main by 2 commits
~~~

需求：2026-05-24-113400-2
