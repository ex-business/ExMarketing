# Task

## 标题

alerts-prometheus-grafana: Prometheus 与 Grafana 接入

## 需求 ID

2026-05-29-alerts-prometheus-grafana-prometheus-grafana

## 背景

系统已有核心指标口径，但生产环境需要能被 Prometheus 拉取，并通过 Grafana 观察活动引擎全链路健康状态。指标必须覆盖事件进入、规则执行、quota、outbox、callback、worker 和数据库。

## 目标

- 提供 Prometheus scrape 配置或接入说明。
- 提供 Grafana dashboard JSON 或可复用面板定义。
- 指标标签控制基数，避免 activityId/subject 等高基数字段直接打标签。
- 面板能快速判断当前链路是否健康。

## 范围

- Event intake：成功/失败数、错误码、延迟。
- Rule/reward：执行数、失败数、阶段耗时。
- Quota：扣减失败率、冻结量、释放量、确认量。
- Outbox：backlog、DLQ count、retry count、send latency。
- Callback：成功/失败率、重复回调、冲突数。
- Worker/DB：claim latency、处理耗时、DB 错误和慢操作。

## 交付物

- Prometheus scrape 配置样例。
- Grafana dashboard JSON 或文档化面板。
- 指标命名和标签规范。
- 本地验证说明。

## 非目标

- 不部署真实生产 Prometheus。
- 不接入公司 IM 通知。
- 不把用户 ID、eventId、activityId 作为高基数指标标签。

## 风险等级

中

## 验收标准

- 本地 `/metrics` 或等价 endpoint 可被 Prometheus scrape。
- Dashboard 能展示 event、quota、outbox、callback、DLQ、worker 状态。
- 指标不暴露 secret、token、用户隐私。
- `go test ./...` 通过。
