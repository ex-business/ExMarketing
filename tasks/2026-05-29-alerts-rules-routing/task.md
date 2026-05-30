# Task

## 标题

alerts-rules-routing: 告警规则与通知路由

## 需求 ID

2026-05-29-alerts-rules-routing

## 背景

只有指标不够，生产环境需要明确哪些异常触发告警、告警级别是什么、通知谁、多久升级、如何静默。告警规则必须围绕活动引擎数据一致性和发奖链路可靠性设计。

## 目标

- 定义 Alertmanager 或等价告警规则。
- 定义 P0/P1/P2/P3 告警等级和通知路由。
- 支持静默、维护窗口、重复告警抑制和升级策略。
- 告警内容包含可定位上下文和 runbook 链接。

## 范围

- P0/P1：DLQ 持续增长、callback 失败率高、outbox backlog 超阈值、worker 全停、DB 写失败、quota 超扣风险。
- P2：event intake 错误率升高、claim latency 升高、retry 激增、Fact Query 失败率高。
- 路由：业务运营、后端 oncall、DBA/基础设施。
- 告警字段：service、tenant、channel、severity、summary、dashboard、runbook。
- 告警抑制：依赖 DB 故障时抑制下游泛洪告警。

## 交付物

- 告警规则文件或文档。
- 通知路由和升级策略说明。
- 告警消息模板。
- 告警单元验证或 promtool 检查脚本。

## 非目标

- 不强绑定某个公司 IM。
- 不把所有 WARN 日志转成告警。
- 不使用高基数标签做告警分组。

## 风险等级

高

## 验收标准

- DLQ、callback、outbox、worker、DB、quota 六类核心异常都有告警规则。
- 告警消息能直接定位 dashboard 和 runbook。
- 维护窗口可静默预期告警。
- 告警规则文件可通过静态校验。
