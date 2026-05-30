# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

新增正式安全威胁模型文档 `docs/security-threat-model.md`，覆盖系统范围、资产清单、数据流/信任边界图、外部入口威胁、风险矩阵、P0/P1 后续补救任务和发布 review checklist。

根据 Review 反馈补强：

- 明确 Webhook / Reward Callback 当前未文档化 nonce store，现有 replay 控制依赖 timestamp、标准事件幂等键、callback 幂等终态和相反终态冲突拒绝，并将 nonce/replay posture 作为 P1 后续任务。
- 将 Fact Provider 和 Outbox egress 的 HTTP redirect、DNS rebinding、ambient proxy/proxy env 绕过列入 SSRF 风险和验收任务。

`test.md` 和 `commit.md` 均为模板态，没有具体 FAIL/BLOCK 原因；本次按 Review 中的 P2 建议和 task.md/analysis.md 验收项做最小文档修复。

## Changed Files

- `docs/security-threat-model.md`
- `tasks/2026-05-29-security-threat-model/implementation.md`

## Behavior Changes

无业务运行时行为变更。本次为文档和任务产物补齐。

## Notes

已执行基础验证：

~~~bash
go test ./...
go build ./...
~~~

需求：2026-05-29-security-threat-model
