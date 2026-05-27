# Task

## 标题

worker-production-hardening: Worker 生产化

## 需求 ID

2026-05-27-worker-production-hardening-worker

## 背景

活动引擎基础完善任务。加强 Outbox worker 常驻运行、优雅停止、shutdown drain、失败恢复、claim 延迟监控、DLQ 堆积监控和日志字段。验收：停止时不丢正在处理消息；重启后可继续处理；多 worker 不重复 claim；关键指标可观测。

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
