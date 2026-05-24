# Task

## 标题

Event Inbox 幂等处理

## 需求 ID

2026-05-24-event-inbox

## 背景

来源：22-独立活动营销中台设计。实现 marketing_event_inbox 的 idempotency_key 唯一约束，推荐格式 tenantId + sourceSystem + eventType + bizType + bizId。验收：重复事件不会重复参与、不会重复发奖、不会重复扣减预算；重复请求返回稳定结果。

## 目标

- 待 Analysis Agent 细化。

## 范围

- 待 Analysis Agent 评估。

## 非目标

- 待 Analysis Agent 评估。

## 风险等级

高

## 验收标准

- 待 Analysis Agent 定义。
