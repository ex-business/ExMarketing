# Task

## 标题

admin-simulation-approval-pages: 模拟运行与审批页面

## 需求 ID

2026-05-29-admin-simulation-approval-pages

## 背景

活动上线前需要运营能看到“为什么命中、为什么拒绝、预计发什么、会影响多少 quota”，同时高危配置必须走审批。当前 API 已有基础能力，但需要管理后台闭环。

## 目标

- 提供模拟运行页面，支持输入标准事件 JSON 或选择样例。
- 输出活动命中、规则阶段、奖励预览、quota 影响、outbox 预览和拒绝原因。
- 提供上线前校验和审批页面。
- 支持普通审批、高危二级审批、驳回原因、版本锁定。

## 范围

- 模拟输入：tenantId、eventType、subject、biz、attributes、occurredAt。
- 模拟输出：MATCH/QUALIFY/REWARD/RISK/LIMIT 明细。
- 校验结果：PASS/WARN/BLOCK/APPROVAL_REQUIRED。
- 审批动作：submit、approve、reject、goLive、rollback。
- 审批审计：操作者、原因、前后快照、版本。

## 交付物

- 模拟运行页面。
- 上线前校验和审批页面。
- 审批 API 对接或缺口补齐。
- 页面操作 smoke 测试。

## 非目标

- 不做真实发奖。
- 不做 Admin 鉴权。
- 不让前端直接修改审批状态绕过后端。

## 风险等级

中

## 验收标准

- 模拟运行不写 participation/grant_order/outbox。
- BLOCK 配置不能上线。
- 高危配置必须二级审批后才能上线。
- 驳回原因和审批历史可查询。
- `go test ./...` 通过。
