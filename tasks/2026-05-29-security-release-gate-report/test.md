# Test

## Scope

根据 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 和 `implementation.md` 执行 Test Agent 验证。

本轮只写入本文件，未修改业务代码、测试代码、配置文件或其它文档。未启动本地服务，因此不涉及后台 PID、健康检查或清理。

## Commands

```bash
go test ./...
```

结果：PASS，退出码 0。

摘要：

```text
ok   github.com/ex-stack/ExMarketing/internal/releasegate 0.427s
ok   github.com/ex-stack/ExMarketing/internal/httpapi      0.559s
ok   github.com/ex-stack/ExMarketing/internal/workflow     0.763s
all other packages passed or had no test files
```

```bash
go build ./...
```

结果：PASS，退出码 0。

```bash
go run ./cmd/security-release-gate \
  --version local-smoke \
  --environment staging \
  --release-owner release \
  --security-owner security \
  --backend-owner backend \
  --ops-owner ops \
  --output -
```

结果：预期负向烟测 PASS，命令退出码 1，因为当前真实仓库没有提供配置证据、威胁模型处理状态或风险接受文件，门禁正确输出 `overallDecision: "BLOCK"`，没有把缺失证据伪装为 PASS。

关键输出摘要：

```text
overallDecision: BLOCK
summary.checksTotal: 25
summary.passed: 4
summary.warned: 4
summary.blocked: 17
summary.openRisks: 33
dependency_vulnerability_scan: WARN
config_baseline: WARN
threat_model_load: PASS, parsed 16 P0/P1 items
secret_scan: BLOCK, 272 potential sensitive literals found
threat_model_tm_01 ... threat_model_tm_17: BLOCK where status/evidence missing
```

## Test Cases

### TP-01 Complete Evidence

覆盖方式：`go test ./...` 执行 `internal/releasegate` 单元测试。

结果：PASS。

证据：

- `TestGeneratePassWithCompleteEvidence` 验证完整 metadata/config/threat model/alert evidence 和 dependency scan 成功时无 BLOCK。
- 由于未执行 required commands，整体按设计为 WARN，符合 test plan。
- `threatModelItems` 成功包含解析出的 P0/P1 项。

### TP-02 Open P1 Threat Model Item

覆盖方式：`go test ./...` 执行 `TestOpenP1RiskBlocks`。

结果：PASS。

证据：删除一个 P1 threat model 状态后，整体结论为 BLOCK，且产生 open risk。

### TP-03 Invalid Risk Acceptance

覆盖方式：`go test ./...` 执行 `TestInvalidRiskAcceptanceBlocks`。

结果：PASS。

证据：缺少 `acceptedBy`、`compensatingControls`、`expiresAt` 的 risk acceptance 被标记为 invalid，整体 BLOCK。

### TP-04 Dependency Scanner Placeholder

覆盖方式：`go test ./...` 执行 `TestDependencyPlaceholderWarnsLocallyAndBlocksProduction`，并通过 CLI 烟测补充验证真实仓库行为。

结果：PASS。

证据：

- staging/local 未配置 `--dependency-scan-command` 时为 WARN。
- production 未配置 dependency scan 证据时为 BLOCK。
- CLI 烟测显示 `dependency_vulnerability_scan: WARN`，未伪装为 PASS。

### TP-05 Secret Scan Block

覆盖方式：`go test ./...` 执行 `TestSecretScanBlocksHighConfidenceLiteral`，并通过 CLI 烟测验证真实仓库扫描摘要。

结果：PASS。

证据：

- fixture 注入高置信长 token literal 后 `secret_scan` 为 BLOCK。
- CLI 烟测在真实仓库输出 `secret_scan: BLOCK`，`sensitiveFindings` 只包含路径、行号、pattern 和 `value redacted` 摘要，没有输出 secret 值。

### TP-06 Required Commands

覆盖方式：执行仓库级必需命令。

结果：PASS。

证据：

- `go test ./...` 退出码 0。
- `go build ./...` 退出码 0。

## Findings

未发现需要打回 Dev Agent 的实现测试失败。

注意事项：

- CLI 真实仓库烟测返回 BLOCK 是预期结果，不是本轮失败。复现命令见 Commands 第三条。
- BLOCK 原因来自缺少发布配置证据、缺少 threat model P0/P1 处理状态、真实仓库 secret scan 命中和未提供风险接受。该行为符合需求中“缺失证据/P0/P1 默认阻断”和“扫描占位不得伪装 PASS”的规则。

## Result

```text
PASS
```

需求：2026-05-29-security-release-gate-report
