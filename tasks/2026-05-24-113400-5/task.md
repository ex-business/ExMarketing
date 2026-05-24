# Task

## 标题

参与记录与发奖单生命周期

## 需求 ID

2026-05-24-113400-5

## 背景

来源：22-独立活动营销中台设计。所有命中活动必须先写 marketing_participation 快照，再生成 marketing_grant_order，状态覆盖 INIT/QUALIFIED/REJECTED/REWARDABLE/GRANTED/FAILED 与 PENDING/SENDING/SUCCESS/FAILED/RETRY/CLOSED。验收：禁止绕过 participation 和 grant_order 直接发奖；幂等键唯一；失败原因可追踪。

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
