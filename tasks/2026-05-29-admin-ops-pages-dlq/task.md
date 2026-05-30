# Task

## 标题

admin-ops-pages: 审计对账DLQ运维页面

## 需求 ID

2026-05-29-admin-ops-pages-dlq

## 背景

活动引擎上线后，运营和技术支持需要快速定位发奖失败、outbox 堆积、DLQ、callback 异常、quota 失败和人工发奖问题。需要把已有审计/对账/运维 API 做成可用页面。

## 目标

- 提供操作审计、对账查询、发奖单查询、Outbox 队列、DLQ 运维页面。
- 支持 DLQ 重放、关闭、查看错误上下文。
- 支持人工发奖确认和导出发奖追踪。
- 危险操作有确认弹窗、原因必填和审计记录。

## 范围

- 审计列表：action、actor、resource、tenant、时间、traceId 过滤。
- 对账查询：按 activity、subject、eventId、participation、grantOrder、outbox 查询。
- Outbox/DLQ：状态、targetChannel、retryCount、nextRetryAt、errorMessage。
- 操作：replay DLQ、close DLQ、manual decision、export file detail。
- 页面保护：跨 tenant 数据不得混查，敏感字段脱敏展示。

## 交付物

- 运维页面和 API 对接。
- 危险操作确认组件。
- 审计和对账字段说明。
- smoke 测试或手工验收脚本。

## 非目标

- 不做权限系统。
- 不做批量资金修复工具。
- 不直接修改数据库绕过服务 API。

## 风险等级

中

## 验收标准

- 可定位一次事件从 inbox 到 outbox/callback 的全链路。
- DLQ 重放/关闭必须写审计。
- 人工发奖和导出发奖状态可追踪。
- 敏感信息不在页面明文展示。
- `go test ./...` 通过。
