# Task

## 标题

activityplugin-red-envelope-lottery: 红包雨与抽奖插件样板

## 需求 ID

2026-05-29-activityplugin-red-envelope-lottery

## 背景

需要用两个典型玩法验证 ActivityPlugin 契约是否可用：红包雨偏场次和库存抢占，抽奖偏概率和结果审计。样板插件用于证明插件只扩展玩法上下文，不绕过主活动引擎。

## 目标

- 实现 RED_ENVELOPE 与 LOTTERY 两个最小样板插件。
- 插件只输出玩法结果、候选奖励或上下文，不直接发奖。
- 奖励、库存、预算、参与记录、发奖单仍走主引擎。
- 样板可用于后续管理后台配置和模拟运行演示。

## 范围

- RED_ENVELOPE：场次、红包池、用户抢红包幂等、空包/命中原因。
- LOTTERY：抽奖次数、奖项候选、概率结果、随机种子或可审计随机来源。
- 插件配置 Schema：场次窗口、奖池、概率、每人次数、展示文案。
- 模拟运行输出插件上下文和命中/未命中原因。
- 插件失败不影响其他活动。

## 交付物

- 两个样板插件注册和测试。
- 对应活动配置模板样例。
- 模拟运行样例。
- 文档说明插件与主引擎职责边界。

## 非目标

- 不做真实前端互动页面。
- 不做 WebSocket 推送。
- 不做大规模排行榜。

## 风险等级

高

## 验收标准

- RED_ENVELOPE/LOTTERY 可注册、校验、模拟运行。
- 概率和库存非法配置会在上线前 BLOCK。
- 插件结果进入 participation snapshot 或 audit，可追溯。
- 插件不直接写 grant_order/outbox/quota。
- `go test ./...` 通过。
