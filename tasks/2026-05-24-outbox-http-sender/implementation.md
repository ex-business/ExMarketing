# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已按 `task.md`、`analysis.md`、`review.md` 和 `test-plan.md` 完成 Outbox HTTP Sender 的返工修复。

本轮重点修复：

- 禁用 outbox HTTP sender 的自动重定向，确保 `3xx` 不会被默认 `http.Client` 跟随到非配置目标并误判为 `SENT`。
- 补齐 HTTP / WEBHOOK sender 的关键回归测试，覆盖 `WEBHOOK` 成功、`3xx` 不跟随、超时、网络错误、非法 target、非法 payload 等场景。
- 保持现有 sender 最小口径：`2xx` 标记 `SENT`；`429` / `5xx` / 网络错误 / 超时进入可重试 `FAILED`；不可重试 `4xx`、非法 target、非法 payload 进入 `DLQ`；`MQ` 不由 HTTP sender 处理。

## Changed Files

- `internal/worker/outbox_worker.go`
  - 在默认 `http.Client` 上设置 `CheckRedirect`，返回 `http.ErrUseLastResponse`，让 sender 直接处理原始 `3xx` 响应。
- `internal/worker/outbox_worker_test.go`
  - 新增 `WEBHOOK` 2xx 成功测试。
  - 新增 `302` 重定向不跟随测试。
  - 新增超时、网络错误、非法 target、非法 payload 测试。
  - 保留并扩展现有 retry、DLQ、MQ 跳过和 grant order 状态流转测试。
- `tasks/2026-05-24-outbox-http-sender/test-plan.md`
  - 补齐测试策略、测试用例、数据 smoke、必跑命令和 `PASS` 结论。
- `tasks/2026-05-24-outbox-http-sender/implementation.md`
  - 记录本轮实现、行为变化和验证结果。

## Behavior Changes

- HTTP sender 不再跟随 `3xx` redirect；`302` 等非 2xx 响应按失败处理，避免把发奖指令发送到非显式配置的目标地址。
- `WEBHOOK` 与 `HTTP` 继续共享 POST JSON 投递逻辑。
- 请求超时和网络错误保持可重试失败，不丢失 outbox。
- 非法 target 或非法 JSON payload 进入不可重试失败 / DLQ，不发起无效投递。

## Notes

已执行验证：

~~~bash
go test ./internal/worker -run TestOutboxWorker -count=1 -v
go test ./...
go build ./...
~~~

结果均为 `PASS`。

`go test ./...` 在当前环境下未设置 `EX_MARKETING_DATABASE_URL` 时会跳过需要外部 PostgreSQL 的集成测试；仓库已有普通测试和不依赖外部数据库的 worker 回归测试通过。

需求：2026-05-24-outbox-http-sender
