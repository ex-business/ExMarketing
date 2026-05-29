# Task

## 标题

activityplugin-scheduler-events: 插件调度与事件回传

## 需求 ID

2026-05-29-activityplugin-scheduler-events

## 背景

红包雨、抽奖、竞猜等玩法需要场次开始、场次结束、开奖、补偿扫描等定时动作。这些动作必须可重试、可审计，并且回传事件仍要走标准 Event Inbox。

## 目标

- 为插件定义调度任务模型和事件回传模型。
- 场次类插件可注册 start/end/draw/settle/compensate 等任务。
- 调度执行失败可重试，不能导致重复发奖。
- 插件产生的内部事件必须转换成标准事件并进入 Event Inbox。

## 范围

- PluginScheduleJob：jobId、tenantId、activityId、pluginCode、jobType、scheduledAt、status、retryCount。
- Job 状态：PENDING、RUNNING、SUCCESS、FAILED、RETRY、CLOSED。
- 插件事件回传：plugin.event.generated -> 标准事件 payload。
- 调度幂等：同一 jobId 只能成功一次。
- 失败处理：错误分类、nextRetryAt、DLQ 或人工关闭。

## 交付物

- 插件调度模型和执行器最小实现。
- 插件事件转标准事件的入口。
- 调度失败重试和幂等测试。
- 场次状态审计和文档。

## 非目标

- 不接入分布式调度平台。
- 不实现复杂日历排班 UI。
- 不让调度任务直接创建 grant_order。

## 风险等级

中

## 验收标准

- 调度任务重复执行不会重复生成标准事件。
- 插件事件必须经过 Event Inbox 幂等处理。
- 失败任务可以重试或关闭，并有审计记录。
- 场次开始/结束/开奖状态可查询。
- `go test ./...` 通过。
