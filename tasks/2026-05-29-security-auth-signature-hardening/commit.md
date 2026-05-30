# Commit

## Agent 指令

你是 Commit Agent。只有 review.md 为 PASS 且 test.md 为 PASS 时才能提交。
只允许写 commit.md；禁止修改业务代码、测试代码、文档代码、配置文件或其他任何文件。
如果提交前检查发现代码需要修复，必须在 commit.md 写明 BLOCK 和原因，让流程打回 Dev Agent，不能自己修。

## Scope

- `docs/api.md`
- `internal/httpapi/handler_test.go`
- `internal/postgres/repository_integration_test.go`
- `tasks/2026-05-29-security-auth-signature-hardening/agent-prompts.md`
- `tasks/2026-05-29-security-auth-signature-hardening/agent-report.md`
- `tasks/2026-05-29-security-auth-signature-hardening/agent-status.json`
- `tasks/2026-05-29-security-auth-signature-hardening/analysis.md`
- `tasks/2026-05-29-security-auth-signature-hardening/implementation.md`
- `tasks/2026-05-29-security-auth-signature-hardening/review.md`
- `tasks/2026-05-29-security-auth-signature-hardening/task.md`
- `tasks/2026-05-29-security-auth-signature-hardening/test-plan.md`
- `tasks/2026-05-29-security-auth-signature-hardening/test.md`

未暂存或提交 `tasks/*/runs/` 执行日志。

## Review

```text
PASS
```

Evidence: `tasks/2026-05-29-security-auth-signature-hardening/review.md` Decision is `PASS`; no P0/P1 findings.

## Test

```text
PASS
```

Evidence: `tasks/2026-05-29-security-auth-signature-hardening/test.md` records `go test ./...` and `go build ./...` as PASS.

## Pre-Commit Checks

```text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
```

## Commit

```text
hash: 285f8edc012570e39d187ea71d40060987b3c9ac
message: security auth signature hardening
remote: not pushed
```

需求：2026-05-29-security-auth-signature-hardening
