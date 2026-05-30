# Test Plan

## Scope

验证安全发布门禁交付物满足 `task.md`、`analysis.md` 和 `review.md`：

- canonical 报告格式选择 JSON，能自动聚合 checklist、命令结果、配置证据、威胁模型状态、风险接受和 open risks。
- P0/P1 默认阻断，WARN 不伪装成 PASS。
- 风险接受必须有 owner、acceptedBy、reason、compensatingControls、未来 expiresAt；P0 还要 Release/Security owner 确认。
- 缺失依赖扫描工具时非生产 WARN、生产 BLOCK。
- 报告和命令输出不得泄漏明文 secret。

## Test Strategy

- 单元测试覆盖 release gate 判定器的正反例，不依赖真实生产环境。
- CLI 构建由 `go build ./...` 覆盖。
- 全仓回归由 `go test ./...` 覆盖，确保新增包不破坏既有业务逻辑。
- 文档检查覆盖 checklist、报告模板、风险接受格式和运行命令。

## Test Cases

### TP-01 Complete Evidence

输入完整 release metadata、配置基线证据、威胁模型 P0/P1 fixed 状态和依赖扫描成功命令。

期望：

- 无 BLOCK。
- required commands 未执行时整体为 WARN。
- `threatModelItems` 包含解析出的 P0/P1 项。

### TP-02 Open P1 Threat Model Item

删除一个 P1 threat model status。

期望：

- 整体 `BLOCK`。
- `openRisks` 包含该 threat model item。

### TP-03 Invalid Risk Acceptance

提交缺少 `acceptedBy`、`compensatingControls` 和 `expiresAt` 的 risk acceptance。

期望：

- acceptance `valid=false`。
- 整体 `BLOCK`。
- `openRisks` 说明 invalid acceptance 原因。

### TP-04 Dependency Scanner Placeholder

不提供 `--dependency-scan-command`。

期望：

- staging/local 为 `WARN`。
- production 为 `BLOCK`。
- 不能输出假 PASS。

### TP-05 Secret Scan Block

在临时 fixture 注入高置信长 token literal。

期望：

- `secret_scan` 为 `BLOCK`。
- `sensitiveFindings` 只记录路径、行号、pattern 和脱敏摘要，不记录 secret 值。

### TP-06 Required Commands

执行仓库级必需命令。

期望：

- `go test ./...` 退出码 0。
- `go build ./...` 退出码 0。

## Required Commands

```bash
go test ./...
go build ./...
```

可选门禁烟测命令：

```bash
go run ./cmd/security-release-gate \
  --version local-smoke \
  --environment staging \
  --release-owner release \
  --security-owner security \
  --backend-owner backend \
  --ops-owner ops \
  --output /tmp/exmarketing-security-release-gate.json
```

## Data / Smoke

- 单元测试使用临时目录生成 threat model、alert rules、runbook、config evidence 和 risk acceptance fixture。
- 不读取真实生产 secret，不连接外部服务。
- 对真实仓库运行 CLI 时，缺少 config evidence 或 dependency scan command 应输出 WARN/BLOCK，而不是 PASS。

## Decision

```text
PASS
```

需求：2026-05-29-security-release-gate-report
