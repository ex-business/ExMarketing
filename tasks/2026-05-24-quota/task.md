# Task

## 标题

Quota 乐观锁原子扣减

## 需求 ID

2026-05-24-quota

## 背景

来源：22-独立活动营销中台设计。实现 marketing_quota 的 DB 乐观锁扣减，覆盖 total_value、used_value、frozen_value、version。验收：并发扣减不会超预算；失败时没有 grant_order/outbox 脏写。

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
