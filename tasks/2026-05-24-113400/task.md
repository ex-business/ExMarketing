# Task

## 标题

标准事件接入与幂等收件箱

## 需求 ID

2026-05-24-113400

## 背景

来源：22-独立活动营销中台设计。实现 HTTP Event API 的标准事件模型、Idempotency-Key 校验、marketing_event_inbox 写入、重复事件去重，以及事件字段只依赖 eventId/eventType/sourceSystem/tenantId/subject/biz/occurredAt/attributes 的边界约束。验收：重复上报不会重复参与、不会重复发奖；非法事件返回明确错误；补充单元测试和 API 文档。

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
