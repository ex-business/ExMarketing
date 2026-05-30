# Commit

## Scope

alerts-production-drill: 生产告警演练与故障注入。

本需求交付：

- 告警演练服务和本地 compose 编排。
- Prometheus / Alertmanager 本地配置。
- 告警演练 runbook 和脚本入口。
- 快速提交门禁与长耗时 live drill 分离。
- drill 服务默认端口改为 `18080`，避免和主 `/admin/` 的 `8080` 冲突。

## Review

```text
PASS
```

## Test

```text
PASS
```

说明：完整 live drill 因 Prometheus `for: 5m` 和 Alertmanager 等待窗口属于长耗时运维演练，不作为普通 Agent 15 分钟门禁；提交门禁以静态校验、Go 测试、构建和本地编排检查为准。

## Commit

```text
hash: pending
message: pending
remote: pending
```

需求：2026-05-29-alerts-production-drill
