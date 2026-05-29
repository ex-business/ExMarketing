# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Preconditions

- `review.md` Decision 为 `PASS`。
- Dev Agent 已完成实现，并在 `implementation.md` 中列出实际变更文件、API 口径和本地启动方式。
- Test Agent 执行前必须确认 PostgreSQL、应用服务和 `/admin/` 页面依赖已按项目现有方式启动。
- Test Agent 只依据当前实现选择命令；若命令名或路径与实现不一致，应先记录 BLOCK 原因，不要猜测修改业务代码。

## Test Strategy

本轮测试目标是验证“通用活动配置完整页面”的端到端可用性，而不是只验证单个组件渲染。

覆盖层级：

- 后端单元/集成：活动创建、编辑、复制、版本、审批约束、模拟 API、quota/额度校验和 PostgreSQL 持久化。
- 前端页面 smoke：列表、创建、编辑、详情、复制、版本/审批展示、模拟入口、错误展示和刷新读回。
- API smoke：使用 HTTP 请求覆盖正常路径、重复 activityCode、非法字段、额度不足、幂等重复请求、并发保存。
- 数据库 smoke：确认 PostgreSQL 中活动主表、审批/审计记录、quota 相关状态符合预期，且模拟运行不写业务状态。

测试数据建议统一使用独立租户和唯一活动编码前缀：

~~~text
tenantId: tp_admin_20260529
activityCode prefix: TP_ADMIN_CFG_20260529_
activityType: FIRST_DEPOSIT
timezone: UTC
triggerEvents: ["DEPOSIT_SUCCESS"]
~~~

## Test Cases

### 1. Normal Path: 创建、查看、编辑、刷新读回

步骤：

1. 打开 `/admin/` 活动列表页。
2. 新建普通配置化活动，填写 `tenantId`、`activityCode`、`activityName`、`activityType`、`DRAFT`、起止时间、`timezone=UTC`、`priority`、`exclusiveGroup`、`stackable=false`。
3. 填写 `config.display`、`config.entry.channels`、`config.entry.sourceSystems`、`config.entry.audience`、`config.metadata`。
4. 填写最小合法 `rules`、`rewards`，保存。
5. 进入详情页，确认基础信息、配置快照、版本、审批状态、模拟入口可见。
6. 刷新页面，确认保存后的配置从后端读回，不丢失。
7. 编辑 `activityName`、`priority`、`stackable`、`config.display.title` 后保存。

期望：

- 创建成功，返回活动详情。
- 详情页展示当前版本、更新时间和审批状态。
- 编辑成功后 `version` 递增，刷新后仍显示新值。
- `tenantId`、`activityCode`、`activityType` 等身份字段在编辑页只读或不可提交修改。

### 2. Normal Path: 列表搜索、过滤、分页、操作入口

步骤：

1. 创建至少 3 个不同 `status/activityType/activityName` 的测试活动。
2. 在列表页按 `tenantId`、`activityCode`、`activityName` 搜索。
3. 分别按 `status`、`activityType`、时间范围过滤。
4. 调整页大小和页码。
5. 从列表进入查看、编辑、复制、模拟、审批/版本入口。

期望：

- 搜索和过滤结果只展示匹配活动；若后端不返回 total，当前结果分页仍可用。
- 翻页或切换操作入口后筛选条件不异常丢失。
- 空结果展示明确空态，不报错。

### 3. Normal Path: 复制活动

步骤：

1. 从已有活动详情或列表点击复制。
2. 输入新的 `activityCode`，保留或修改 `activityName`。
3. 保存复制后的活动。
4. 查看复制后活动详情。

期望：

- 新活动为 `DRAFT`，`version=1`，审批状态为 `NONE`。
- 复制基础配置、展示入口、规则、奖励、互斥叠加、时间和 timezone。
- 不复制 participation、grant_order、outbox、quota 已用/冻结量、模拟历史、审批状态、审计历史。

### 4. Normal Path: 模拟运行入口和无副作用

步骤：

1. 从已保存活动详情点击进入模拟运行。
2. 确认页面或请求上下文带入当前 `tenantId` 和 `activityCode/activityId`。
3. 使用标准 `DEPOSIT_SUCCESS` 事件 payload 执行模拟。
4. 模拟后返回活动详情，查看最近模拟结果摘要。
5. 查询数据库或 API，确认未写入正式 participation、grant_order、outbox、quota 使用量。

期望：

- `POST /admin/v1/simulate` 返回匹配/拒绝原因和奖励预览。
- 模拟结果展示清楚；若只保留页面会话内摘要，刷新后不得误称已持久化历史。
- 模拟不改变业务状态。

### 5. Error Path: 非法时间、非法 timezone、非法 JSON

步骤：

1. 创建活动时设置 `startTime >= endTime`。
2. 创建活动时设置不可识别 timezone，例如 `Mars/Phobos`。
3. 在 JSON 编辑区输入语法错误 JSON。
4. 在 `config` 顶层加入非法字段，例如 `rules` 或 `secret`。

期望：

- 前端能在保存前提示明显非法输入。
- 后端返回错误时，字段级错误挂到对应字段。
- 非法 `config` 顶层字段返回保存失败，不得把失败展示为成功。

### 6. Error Path: 重复 activityCode

步骤：

1. 在同一 `tenantId` 下创建 `activityCode=TP_ADMIN_CFG_20260529_DUP`。
2. 再次使用相同 `tenantId + activityCode` 创建活动。
3. 使用不同 `tenantId` 但相同 `activityCode` 创建活动。

期望：

- 同租户重复创建返回 `409 Conflict` 或等价业务错误。
- 页面展示 `activityCode` 字段错误或全局“同租户活动编码重复”。
- 不同租户可使用相同 `activityCode`。

### 7. Error Path: 审批、版本和状态约束

步骤：

1. 新建 `DRAFT` 活动后直接尝试保存为 `ACTIVE`。
2. 发起审批但未完成时尝试保存为 `ACTIVE`。
3. 审批通过后再次编辑活动，使 `version` 递增。
4. 使用旧审批版本尝试上线。
5. 若实现支持 L2，高危活动测试提交人不能审批自己、L2 审批人不能与 L1 相同。

期望：

- 未审批、审批未完成、审批版本不一致、审批被驳回均不能进入 `SCHEDULED/ACTIVE`。
- 页面顶部显示全局 BLOCK，明确版本或审批原因。
- 审批锁定版本与当前活动版本的展示一致。

### 8. Quota Insufficient: 额度不足

步骤：

1. 创建带 quota 限制的活动，配置总额度小于测试事件预期奖励消耗，或使用已耗尽额度的测试活动。
2. 通过模拟或后端 validation 路径提交会触发奖励判断的标准事件。
3. 若项目已有正式事件处理 smoke，提交一条正式事件并观察匹配/发奖结果。

期望：

- 额度不足时返回明确拒绝原因或错误码，例如 quota insufficient / quota exceeded / no remaining quota。
- 页面显示全局错误或模拟拒绝原因，不展示为可发奖成功。
- PostgreSQL 中 quota `used_value/frozen_value` 不因失败请求异常增加。
- 若只执行模拟，必须确认模拟不写 quota 使用量。

### 9. Idempotency: 幂等重复请求

步骤：

1. 使用相同创建 payload 和相同幂等键重复提交创建请求两次；若项目没有幂等键，用相同业务唯一键重复提交。
2. 对同一编辑 payload 快速重复点击保存两次。
3. 对同一标准事件 `eventId` 重复执行模拟或正式事件入口，按实现支持范围验证。

期望：

- 创建重复请求不会产生两条相同 `tenantId + activityCode` 活动。
- 编辑重复提交最多产生一次有效版本递增；若实现每次保存均递增版本，必须有明确可解释行为且不丢配置。
- 重复事件不会重复写 participation、grant_order 或 quota 消耗；模拟重复仍无业务副作用。

### 10. Concurrency: 并发保存和复制

步骤：

1. 打开同一活动的两个编辑会话，记录相同初始 `version`。
2. 会话 A 修改 `priority` 保存成功。
3. 会话 B 基于旧页面修改 `activityName` 保存。
4. 并发发起两个复制请求，使用相同新 `activityCode`。
5. 并发发起两个不同新 `activityCode` 的复制请求。

期望：

- 若后端支持乐观锁，旧版本保存返回版本冲突；页面展示全局 BLOCK 并提示刷新。
- 若后端不支持乐观锁，最终详情必须可解释且不产生半写状态，Test Agent 需记录风险。
- 相同新 `activityCode` 的并发复制只成功一个，另一个返回重复错误。
- 不同新 `activityCode` 的并发复制可各自成功，且配置独立。

### 11. PostgreSQL Smoke

步骤：

1. 创建活动后查询 PostgreSQL 活动主表，确认 `tenant_id`、`activity_code`、`activity_name`、`activity_type`、`status`、`start_time`、`end_time`、`timezone`、`priority`、`exclusive_group`、`stackable`、`version`、`config_json` 落库正确。
2. 编辑活动后查询同一记录，确认 `version` 递增、`updated_at` 变化、不可编辑身份字段未变化。
3. 复制活动后查询新记录，确认 `status=DRAFT`、`version=1`、新 `activity_code`，且无运行态数据复制。
4. 审批动作后查询审批/审计相关表，确认锁定版本与页面展示一致。
5. 模拟运行后查询 participation、grant_order、outbox、quota 相关表，确认无正式业务写入。

期望：

- 页面展示与 PostgreSQL 权威数据一致。
- JSON 字段只包含允许的通用 `entry/display/audience/metadata` 顶层配置。
- 重复、失败、额度不足和模拟请求不会留下错误脏数据。

## Required Commands

Test Agent 应在实现完成后执行以下命令；本 Test Plan Agent 未执行这些命令。

~~~bash
go test ./...
go build ./...
~~~

如项目实现提供前端或页面 smoke 命令，Test Agent 还应执行对应命令，例如：

~~~bash
npm test
npm run build
npm run test:e2e
~~~

如果仓库不是 npm 前端或没有这些脚本，记录“不适用/脚本不存在”，不要临时新增脚本。

建议 API smoke 命令清单，由 Test Agent 按实际端口、鉴权和 endpoint 调整后执行：

~~~bash
curl -sS -X POST "$BASE_URL/admin/v1/activities" -H 'Content-Type: application/json' -d @create-activity.json
curl -sS "$BASE_URL/admin/v1/activities?tenantId=tp_admin_20260529"
curl -sS -X PUT "$BASE_URL/admin/v1/activities/$ACTIVITY_ID" -H 'Content-Type: application/json' -d @update-activity.json
curl -sS -X POST "$BASE_URL/admin/v1/activities/$ACTIVITY_ID/copy" -H 'Content-Type: application/json' -d @copy-activity.json
curl -sS -X POST "$BASE_URL/admin/v1/simulate" -H 'Content-Type: application/json' -d @simulate-event.json
~~~

建议 PostgreSQL smoke 命令清单，由 Test Agent 按实际连接串和表名调整后执行：

~~~bash
psql "$DATABASE_URL" -c "select tenant_id, activity_code, activity_name, activity_type, status, timezone, priority, exclusive_group, stackable, version, config_json from activity_config where tenant_id = 'tp_admin_20260529' order by updated_at desc limit 20;"
psql "$DATABASE_URL" -c "select * from activity_approval where tenant_id = 'tp_admin_20260529' order by created_at desc limit 20;"
psql "$DATABASE_URL" -c "select * from activity_quota where tenant_id = 'tp_admin_20260529' order by updated_at desc limit 20;"
psql "$DATABASE_URL" -c "select count(*) from participation where tenant_id = 'tp_admin_20260529';"
psql "$DATABASE_URL" -c "select count(*) from grant_order where tenant_id = 'tp_admin_20260529';"
psql "$DATABASE_URL" -c "select count(*) from outbox where tenant_id = 'tp_admin_20260529';"
~~~

## Data / Smoke

最小创建 payload：

~~~json
{
  "tenantId": "tp_admin_20260529",
  "activityCode": "TP_ADMIN_CFG_20260529_NORMAL",
  "activityName": "Admin Config Test Normal",
  "activityType": "FIRST_DEPOSIT",
  "status": "DRAFT",
  "triggerEvents": ["DEPOSIT_SUCCESS"],
  "startTime": "2026-05-29T00:00:00Z",
  "endTime": "2026-06-29T00:00:00Z",
  "timezone": "UTC",
  "priority": 100,
  "exclusiveGroup": "tp_admin_deposit",
  "stackable": false,
  "config": {
    "display": {
      "visible": true,
      "title": "Admin Config Test"
    },
    "entry": {
      "enabled": true,
      "channels": ["APP", "WEB"],
      "sourceSystems": ["payment"],
      "tenantIds": ["tp_admin_20260529"],
      "audience": {
        "subjectTypes": ["USER"],
        "segments": ["new_user"],
        "riskLevels": ["LOW"]
      }
    },
    "metadata": {
      "sourceSystem": "payment",
      "bizType": "DEPOSIT"
    }
  },
  "rules": [],
  "rewards": []
}
~~~

最小模拟 payload：

~~~json
{
  "eventId": "evt_tp_admin_20260529_001",
  "eventType": "DEPOSIT_SUCCESS",
  "sourceSystem": "payment",
  "tenantId": "tp_admin_20260529",
  "subject": {
    "type": "USER",
    "id": "user_tp_admin_001"
  },
  "biz": {
    "bizType": "DEPOSIT",
    "bizId": "pay_tp_admin_001"
  },
  "occurredAt": "2026-05-29T12:00:00Z",
  "attributes": {
    "amount": "299.00",
    "currency": "CNY",
    "channel": "APP",
    "segments": ["new_user"],
    "riskLevel": "LOW"
  }
}
~~~

额度不足数据要求：

- 需要一个 quota 总量小于单次奖励消耗的活动，或一个已经耗尽 quota 的活动。
- 失败后必须同时核对页面/API 错误和 PostgreSQL quota 状态。

幂等与并发数据要求：

- 幂等测试使用固定 `eventId`、固定 `activityCode` 和固定请求 payload。
- 并发复制测试使用相同新 `activityCode` 验证唯一约束，再使用不同新 `activityCode` 验证并发成功路径。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-admin-activity-config-pages
