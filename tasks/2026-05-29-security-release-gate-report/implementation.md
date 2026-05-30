# Implementation

## Summary

完成安全发布门禁最小实现：

- 新增 canonical JSON 报告生成器 `internal/releasegate`。
- 新增可运行 CLI `cmd/security-release-gate`，支持生成 PASS/WARN/BLOCK 门禁报告。
- 新增 `docs/security-release-gate.md`，包含 checklist、运行命令、报告字段、配置证据格式和风险接受格式。
- 将 `test-plan.md` 从模板补齐为 PASS 测试计划。

本轮未发现 `test.md` 中存在实质 FAIL 原因；`test.md` 仍为模板占位。`commit.md` 也未包含实质 BLOCK/FAILED 上下文，因此没有提交失败修复项需要优先处理。

## Changed Files

- `internal/releasegate/gate.go`
- `internal/releasegate/gate_test.go`
- `cmd/security-release-gate/main.go`
- `docs/security-release-gate.md`
- `tasks/2026-05-29-security-release-gate-report/test-plan.md`
- `tasks/2026-05-29-security-release-gate-report/implementation.md`

## Behavior Changes

- 门禁报告顶层输出 `overallDecision: PASS | WARN | BLOCK`。
- 聚合 release metadata、checklist、required command 状态、secret scan、dependency scan、config baseline、alert evidence、threat model P0/P1 状态、risk acceptances 和 open risks。
- `go test ./...` / `go build ./...` 可通过 `--run-required-commands` 纳入报告；未执行时记录 WARN，不伪装为 PASS。
- dependency scan 未配置时，非生产为 WARN，production 为 BLOCK；可用合规 risk acceptance 将可接受 blocker 降级为 WARN。
- P0/P1 threat model item 缺少 `fixed` / `accepted` / `not-applicable` 状态和 evidence 时 BLOCK。
- risk acceptance 缺少 owner、acceptedBy、reason、compensatingControls、未来 expiresAt 时无效并 BLOCK；P0 acceptance 还要求 release/security owner 确认。
- secret scan 命中 private key、长 secret/token/password/API key assignment 或 DSN 明文密码时 BLOCK，并只输出脱敏 finding。
- `go test`、`go build`、release metadata、secret scan、risk acceptance load 和 threat model load blocker 不允许通过 risk acceptance 绕过。

## Verification

```bash
go test ./...
go build ./...
```

结果：

```text
PASS
PASS
```

## Notes

- 评审中的 P2“选择 canonical 机器可读格式”已处理：选择 JSON。
- 评审中的 P2“扫描占位不得冒充 PASS”已处理：dependency scan 未配置会 WARN/BLOCK。
- CLI 默认不执行 `go test ./...` 和 `go build ./...`，避免在普通报告预览中递归触发；正式发布必须加 `--run-required-commands`。

需求：2026-05-29-security-release-gate-report
