# Task

## 标题

alerts-runbook-slo: 告警 Runbook 与 SLO

## 需求 ID

2026-05-29-alerts-runbook-slo-runbook-slo

## 背景

告警触发后必须能快速处理，否则只能制造噪音。每类关键告警需要有 runbook、影响判断、止血动作、恢复验证和复盘字段，同时要定义活动引擎的 SLO/SLA。

## 目标

- 为核心告警编写 runbook。
- 定义 SLI/SLO：事件处理成功率、发奖指令投递延迟、callback 处理成功率、DLQ 清理时效。
- 定义演练记录和复盘模板。
- 明确哪些告警需要业务确认，哪些由技术直接处理。

## 范围

- Runbook：DLQ 堆积、outbox backlog、callback 失败、quota 异常、worker 停止、DB 慢/错误、Fact Query 失败。
- 每个 runbook 包含：症状、影响、排查命令、dashboard、止血、恢复、验证、升级。
- SLO：可用性、延迟、错误率、数据一致性。
- 报告模板：时间线、影响面、根因、修复、预防项。

## 交付物

- `docs/operations` 下的 runbook 文档。
- SLO/SLA 指标定义。
- 演练记录模板。
- 告警到 runbook 的映射表。

## 非目标

- 不承诺具体业务赔付规则。
- 不替代公司统一 incident 流程。
- 不做真实 oncall 排班系统。

## 风险等级

中

## 验收标准

- 每个 P1/P2 告警都有 runbook。
- Runbook 包含可执行命令或 API 路径。
- SLO 指标能由现有 metrics 支撑。
- 演练后能填写复盘模板。
