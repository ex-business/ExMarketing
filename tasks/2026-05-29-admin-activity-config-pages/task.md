# Task

## 标题

admin-activity-config-pages: 活动配置完整页面

## 需求 ID

2026-05-29-admin-activity-config-pages

## 背景

当前 `/admin/` 主要是 Agent 工作台和基础活动操作，运营还不能完整配置活动基础信息。需要补齐通用活动配置页，让不同活动通过同一套页面配置，而不是为每个活动写专用页面。

## 目标

- 提供活动列表、创建、编辑、查看、复制、版本管理页面。
- 支持配置 tenant、activityCode、activityName、activityType、status、start/end、timezone、priority、exclusiveGroup、stackable、display/entry 配置。
- 页面保存前有前端提示，保存后仍由后端做权威校验。
- 活动保存后可直接进入模拟运行和上线前校验。

## 范围

- 活动列表：搜索、状态过滤、类型过滤、时间过滤、分页。
- 活动编辑：基础信息、展示入口、渠道、人群入口、互斥叠加、版本说明。
- 活动详情：配置快照、版本历史、审批状态、最近模拟结果。
- API 对接：复用已有 activity config / validation / simulation API。
- 错误展示：字段级错误、全局 BLOCK、后端校验结果。

## 交付物

- 管理后台页面和必要前端组件。
- 后端 API 缺口补齐或适配。
- 页面级 smoke 测试或最小交互验证说明。
- 文档说明运营使用路径。

## 非目标

- 不做 Admin 鉴权。
- 不做每个业务活动的专用页面。
- 不做复杂拖拽营销装修。

## 风险等级

中

## 验收标准

- 能创建、编辑、查看、复制一个普通配置化活动。
- 非法时间、非法 timezone、重复 activityCode、互斥配置错误能展示明确错误。
- 保存后的活动可进入模拟运行。
- 页面刷新不丢失已保存配置。
- `go test ./...` 和页面 smoke 通过。
