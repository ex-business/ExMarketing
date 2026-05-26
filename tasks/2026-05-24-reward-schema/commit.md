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

- Reward 配置 Schema 校验覆盖 `BONUS` / `CASH` / `COUPON` / `POINT` / `GIFT` / `DRAW_CHANCE` / `MEMBERSHIP`。
- 金额类奖励支持 `FIXED` / `RATIO` / `LADDER` / `RANDOM` / `PROBABILITY` / `POOL_SPLIT`，非金额类当前最小可上线口径只支持 `FIXED`。
- 非金额类奖励以 `reward_amount=0.0000` 生成可发奖 payload，保留权益数量、外部引用、期限和估值字段。
- 文档同步更新 Reward Schema 和发奖 payload 语义。
- 未暂存 `tasks/*/runs/` 执行日志。

## Pre-Commit Checks

~~~bash
git status -sb
git diff --check
go test ./...
go build ./...
~~~

~~~text
PASS
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
hash: 3d66027df63045e5be86c6c236dbe0737cac2014
message: Validate reward configuration schema
remote: not pushed
~~~

需求：2026-05-24-reward-schema
