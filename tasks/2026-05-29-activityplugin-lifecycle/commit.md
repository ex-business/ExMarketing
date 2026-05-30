# Commit

## Scope

已提交 ActivityPlugin 生命周期与版本契约相关改动：

- 插件注册、版本范围、能力声明、禁用策略和生命周期契约。
- 活动创建 / 上线前插件校验。
- 运行期插件失败审计与 grant_order / outbox 副作用阻断。
- 单元测试、契约文档和任务交付记录。

提交时未暂存 `tasks/*/runs/` 执行日志。

## Review

~~~text
PASS
~~~

来源：`tasks/2026-05-29-activityplugin-lifecycle/review.md`

## Test

~~~text
PASS
~~~

来源：`tasks/2026-05-29-activityplugin-lifecycle/test.md`

## Pre-Commit Checks

~~~text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
~~~

## Commit

~~~text
hash: f8ab4f325e011f8a78e9677668fc0531b9056490
short_hash: f8ab4f3
message: Add activity plugin lifecycle contract
remote: not pushed
~~~

## Post-Commit Status

~~~text
main...origin/main [ahead 1]
commit.md remains untracked by design because it was written after commit.
other unrelated tasks/* directories remain untracked.
~~~

需求：2026-05-29-activityplugin-lifecycle
