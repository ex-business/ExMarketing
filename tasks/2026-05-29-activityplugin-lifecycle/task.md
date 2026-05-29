# Task

## 标题

activityplugin-lifecycle: 插件生命周期与版本契约

## 需求 ID

2026-05-29-activityplugin-lifecycle

## 背景

当前系统已有 ActivityPlugin 注册表基础能力，但还不足以支撑红包雨、抽奖、翻卡、竞猜等玩法型活动长期扩展。插件必须成为主活动引擎的可控扩展点，而不是绕开 MATCH / RULE / REWARD / QUOTA / PARTICIPATION / GRANT_ORDER / OUTBOX 的旁路。

## 目标

- 定义 ActivityPlugin 的注册、版本、能力声明、生命周期和兼容性契约。
- 插件只能提供玩法上下文、状态计算、候选结果或扩展校验，不能直接发奖、扣 quota、写 outbox。
- 活动上线前能校验插件是否存在、版本是否兼容、能力是否满足配置要求。
- 插件执行失败必须有结构化错误、审计记录和可回滚状态。

## 范围

- 插件注册模型：pluginCode、version、capabilities、supportedActivityTypes、configSchema、stateSchema。
- 生命周期接口：Register、ValidateConfig、BeforeMatch、BeforeReward、AfterRewardCalculated、OnEvent、OnRollback、Disable。
- 兼容性规则：活动配置声明 pluginCode/versionRange，运行时按 tenant/activity/version 固定解析。
- 插件禁用策略：禁用后新活动不能上线，已上线活动按策略 BLOCK、降级或只读。
- 插件审计：注册、升级、禁用、执行失败、回滚都要记录。

## 交付物

- 插件生命周期契约文档或代码接口。
- 插件配置校验与活动上线前校验接入。
- 插件执行错误码和审计 payload 定义。
- 单元测试覆盖插件不存在、版本不兼容、能力缺失、执行失败、禁用插件。

## 非目标

- 不实现具体玩法插件业务逻辑。
- 不实现 Admin 页面。
- 不允许插件直接访问数据库业务表、直接发奖或直接扣减 quota。

## 风险等级

高

## 验收标准

- 新增插件不需要修改主引擎核心流程。
- 插件不存在、版本不兼容、能力缺失时活动上线被 BLOCK。
- 插件执行失败不产生 grant_order/outbox 脏写。
- 所有插件执行结果都能从 participation snapshot 或 audit 中追踪。
- `go test ./...` 通过。
