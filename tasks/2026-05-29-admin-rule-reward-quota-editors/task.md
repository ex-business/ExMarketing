# Task

## 标题

admin-rule-reward-quota-editors: 规则奖励额度编辑器

## 需求 ID

2026-05-29-admin-rule-reward-quota-editors

## 背景

活动配置的核心是规则、奖励和额度限制。当前能力主要在 API 和配置模板层，运营无法用通用 UI 安全配置 MATCH / QUALIFY / REWARD / RISK / LIMIT、奖励计算器和 quota。

## 目标

- 提供规则编辑器、奖励编辑器、quota 编辑器。
- 表单由 Schema 或元数据驱动，支持普通表单和高级 JSON 模式。
- 配置结果必须能被现有引擎直接执行。
- 非法字段、类型、概率、金额、quota 边界在保存或上线前明确提示。

## 范围

- 规则编辑器：phase、type、fieldPath、op、value、sort、enabled、reasonCode。
- 奖励编辑器：rewardType、calculateType、grantMode、grantChannel、flowMultiple、rewardConfig。
- Quota 编辑器：BUDGET、STOCK、USER_LIMIT、DAILY_LIMIT、scope、scopeKey、totalValue。
- JSON 高级模式：展示校验错误路径，支持格式化。
- 配置预览：展示最终 JSON 和人类可读摘要。

## 交付物

- 前端编辑器组件。
- Schema/metadata 定义和后端校验适配。
- 单元测试覆盖 schema 校验和关键 UI 数据转换。
- 使用示例：首存、签到、返水至少各一例。

## 非目标

- 不引入无限制脚本执行。
- 不做可视化复杂公式编排器。
- 不绕过后端上线前校验。

## 风险等级

高

## 验收标准

- MATCH/QUALIFY/REWARD/RISK/LIMIT 至少能配置常见 CONDITION。
- FIXED/RATIO/LADDER/RANDOM/PROBABILITY/POOL_SPLIT 至少能保存合法配置并拒绝非法配置。
- Quota 配置能表达预算、库存、次数限制。
- 保存后的配置可被模拟运行解释。
- `go test ./...` 通过。
