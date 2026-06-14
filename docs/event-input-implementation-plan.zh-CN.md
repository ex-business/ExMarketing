# ExMarketing 事件输入分阶段实施计划

本文档承接 `event-input-node-design.zh-CN.md`，用于把“事件输入是分类，不是单个节点”的设计拆成可执行阶段。原则是先收口产品概念，再补齐已有 handler 语义，最后逐步实现 MQ、定时、状态监听等真实入口能力。

## 总体目标

将当前单一 `event-in` 模型升级为：

```text
事件输入分类
  -> HTTP 接口事件 / Webhook 映射事件 / MQ Topic 事件 / 定时事件 / 状态监听事件 / 人工触发事件 / ...
  -> 标准事件入口校验
  -> Inbox 幂等收件箱
  -> 后续 workflow
```

所有入口最终必须产出同一个 `standard_event`，并继续遵守：

```text
tenantId:sourceSystem:eventType:biz.bizType:biz.bizId
```

作为标准幂等键。

## 阶段 0：确认边界

目标：冻结概念，避免后续把入口监听、事件映射、入口校验继续混在一个节点里。

改动范围：

- 文档确认 `事件输入` 是节点分类。
- 文档确认 `event-in` 是标准事件入口校验节点。
- 明确当前阶段不实现 MQ 入站、CDC、状态监听 runtime。

不做：

- 不改生产事件处理链路。
- 不新增 consumer。
- 不调整数据库模型。

验收标准：

- 产品和技术文档中不再把 `event-in` 描述成 HTTP/MQ/Webhook 监听器。
- 明确 `Trigger Source -> Event Adapter -> Standard Event -> Event In -> Inbox -> Workflow` 的分层。

## 阶段 1：UI 分类和文案收口

目标：先让管理台的节点库表达正确。

改动范围：

- 新增节点分类：`EVENT_INPUT` / `事件输入`。
- 将当前 `event-in` 从 `CORE 核心链路` 移入 `事件输入`。
- 将显示名从 `事件入口` 改为 `标准事件入口`。
- 将描述改为：

```text
接收已标准化事件，校验 eventType、sourceSystem、tenant 后进入流程。
```

- 配置预览中去掉 `HTTP / MQ / Webhook 标准事件接入` 这类误导文案。
- 左侧节点库增加预留节点，但标注能力状态：
  - `HTTP 接口事件`：已支持接口，节点解释型。
  - `Webhook 映射事件`：已支持接口，节点解释型。
  - `MQ Topic 事件`：待实现。
  - `定时事件`：待实现/可复用调度能力。
  - `状态监听事件`：待实现。
  - `人工触发事件`：待收口为生产补偿入口。

建议文件：

- `internal/marketing/workflow_node_config_schema.go`
- `internal/engine/workflow_template_service.go`
- `frontend/admin-flow/src/types.ts`
- `frontend/admin-flow/src/App.tsx`
- `frontend/admin-flow/src/components/PropertyPanel.tsx`

不做：

- 不让预留节点进入默认生产模板。
- 不让待实现节点发布为真实执行节点。

验收标准：

- 节点库出现 `事件输入` 分类。
- 当前已有入口节点显示为 `标准事件入口`。
- 当前 workflow 模板仍可保存、校验、发布、debug run。
- 待实现入口节点在 UI 中有明确状态，不误导为已可生产执行。

测试建议：

- `npm run typecheck:admin-flow`
- `npm run build:admin-flow`
- `go test ./internal/marketing ./internal/engine ./internal/httpapi`

## 阶段 2：`event-in` schema 与 handler 对齐

目标：当前 `event-in` schema 暴露的字段必须和 handler 真实行为一致。

当前需要处理的字段：

| 字段 | 当前问题 | 处理建议 |
| --- | --- | --- |
| `eventTypes` | handler 已校验 | 保留 |
| `sourceSystems` | handler 已校验 | 保留 |
| `tenantScope` | schema 有，handler 语义不完整 | 补 handler 或暂时隐藏 |
| `payloadSchemaVersion` | schema 有，未见版本解析 | 若只支持 `standard-v1`，改成固定枚举并校验 |
| `rejectUnknownEventType` | 和 `eventTypes` 语义重叠 | 明确为 true 时阻断，false 时跳过或降级 |
| `traceKeyPath` | schema 有，handler 未完整提取 | 补 trace 提取并写入 audit/source metadata |

建议语义：

- `tenantScope=CURRENT_TENANT`：事件 `tenantId` 必须等于 activity/workspace 当前租户。
- `tenantScope=WORKSPACE_TENANT`：事件租户必须在 workspace 允许范围内。
- `tenantScope=ANY_TENANT`：仅允许系统级模板或受控场景，默认不开放。
- `payloadSchemaVersion=standard-v1`：当前只接受现有 `ActivityEvent` 模型。
- `rejectUnknownEventType=true`：不在 `eventTypes` 内直接失败。
- `rejectUnknownEventType=false`：不在 `eventTypes` 内返回 skipped，并可走默认跳过路径。
- `traceKeyPath`：从 `raw_event.attributes` 或原始 source metadata 中提取 trace key，写入 node output 的 `audit`。

改动范围：

- `entryWorkflowNodeHandler`
- workflow node execution output
- validation tests
- executor tests

不做：

- 不在此阶段实现新的入口 listener。

验收标准：

- `event-in` 每个配置项都有可证明的 handler 行为。
- run history 能解释 `EVENT_IN_TYPE_NOT_ALLOWED`、`EVENT_IN_SOURCE_NOT_ALLOWED`、tenant 不匹配、schema version 不匹配。
- 不再有“UI 可配置但运行时不生效”的字段。

测试建议：

- 增加 `event-in` handler 单测。
- 增加 workflow executor 测试：
  - eventType 不命中。
  - sourceSystem 不命中。
  - tenantScope 不命中。
  - payloadSchemaVersion 不支持。
  - traceKeyPath 提取成功/失败。

## 阶段 3：HTTP / Webhook 入口配置可视化

目标：把已经存在的 HTTP 和 Webhook 能力在 Studio 中表达出来，但不改变生产入口链路。

设计方式：

```text
HTTP 接口事件 / Webhook 映射事件 = 配置解释节点
标准事件入口 = workflow 内部执行节点
```

HTTP 节点建议配置：

```json
{
  "path": "/openapi/v1/events",
  "method": "POST",
  "auth": "BEARER_TOKEN",
  "payloadMode": "STANDARD_EVENT",
  "maxBodyBytes": 1048576
}
```

Webhook 节点建议配置：

```json
{
  "receiverCode": "payment-webhook",
  "signature": "HMAC_SHA256",
  "allowedTenantIds": ["site_001"],
  "allowedSourceSystems": ["payment"],
  "mappingVersion": "v1"
}
```

改动范围：

- 新增 node config schema。
- 新增 node preset。
- 属性面板展示能力状态。
- 模板中可选展示，但默认生产链路仍从 `标准事件入口` 开始。

不做：

- 不允许 UI 直接修改生产环境 token/secret。
- 不在 workflow 执行时重复执行 HTTP/Webhook 接收逻辑。

验收标准：

- 用户能在管理台看到一个活动支持哪些 HTTP/Webhook 入口。
- 节点清楚标识 `EXPLAIN_ONLY` 或 `EXTERNAL_ENDPOINT`。
- 发布校验不会误以为 HTTP/Webhook 节点本身会在 workflow 内监听请求。

## 阶段 4：Manual Trigger 生产补偿入口

目标：把 debug-run/replay 和真实生产人工触发区分开。

Manual Trigger 语义：

- debug-run：无副作用，只验证 draft。
- manual-event：有生产副作用，进入 Event Inbox 和真实发奖链路。

配置建议：

```json
{
  "allowedRoles": ["OPS", "ADMIN"],
  "requireReason": true,
  "requireApproval": false,
  "payloadMode": "STANDARD_EVENT",
  "auditAction": "MANUAL_EVENT_TRIGGERED"
}
```

改动范围：

- 新增 Admin API：人工触发标准事件。
- 强制记录 operator、reason、source IP/request id。
- 走 `ProcessEventFrom(ctx, event, "manual:<operator>")`。
- 管理台提供补偿触发 UI。

验收标准：

- 人工触发与 debug-run 在 run history/audit 中可区分。
- 人工触发必须写 Event Inbox。
- 重复人工触发遵守相同幂等规则。

## 阶段 5：MQ Topic 入站

目标：实现真正的 MQ 入站事件入口。

核心要求：

- 支持 Kafka 优先，RabbitMQ/Pulsar 后续扩展。
- 消费消息后先映射为标准事件。
- Event Inbox 提交成功后才 ack / commit offset。
- 幂等重复可 ack。
- 幂等冲突不可无限重试，应进入 DLQ 或阻断队列。
- 消费位置、topic、partition、offset、message key 必须写入 source metadata。

配置建议：

```json
{
  "broker": "KAFKA",
  "topic": "user-checkin-events",
  "consumerGroup": "exmarketing-checkin",
  "ackPolicy": "ACK_AFTER_INBOX_COMMIT",
  "retryPolicy": "RETRY_THEN_DLQ",
  "mapping": {
    "eventId": {"path": "$.event_id"},
    "eventType": {"const": "CHECKIN_SUCCESS"},
    "sourceSystem": {"const": "app"}
  }
}
```

新增模型建议：

```text
trigger_id
trigger_type
activity_id
enabled
config_json
cursor_json
last_error
created_at
updated_at
```

验收标准：

- consumer 重启后不会丢消息。
- inbox 成功前不会提交 offset。
- 同 key 同 payload 重复消息安全 ack。
- 同 key 不同 payload 进入冲突处理，不反复打爆下游。
- MQ 入站 E2E 覆盖真实 broker 或 testcontainer。

## 阶段 6：Schedule Trigger

目标：实现定时生成标准事件。

适用场景：

- 每日签到结算。
- 排行榜开奖。
- 活动开始/结束。
- 过期补偿扫描。
- 周期性用户任务。

关键约束：

- 同一 schedule fire 必须生成稳定 `bizId`。
- 重试时 payload 不得变化。
- `occurredAt` 建议使用 `scheduledAt`，不是实际执行时间。
- misfire 策略必须明确。

验收标准：

- 同一个计划时间重复执行不会造成重复发奖。
- 活动配置变更后，已生成 job 的 payload 仍稳定。
- run history 能展示 scheduledAt、attempt、job id。

## 阶段 7：State Watch Trigger

目标：支持监听可恢复状态，而不是监听进程内临时变量。

支持来源优先级：

1. Redis key / stream。
2. 数据库状态表。
3. 配置中心。
4. 指标系统。

不建议：

- 直接监听 Go 进程内全局变量。
- 监听没有版本、cursor、更新时间的状态。

关键约束：

- 必须有 last-seen cursor、version 或 timestamp。
- 必须有 debounce / throttle。
- 必须记录触发前后的状态摘要。

验收标准：

- 服务重启后不会因为状态丢失重复触发或漏触发。
- 状态变化触发的事件能被审计和重放。

## 阶段 8：CDC / File Trigger

目标：扩展数据变更和批量导入入口。

CDC 要求：

- 明确 source table / collection。
- 明确 cursor / resume token。
- 明确 insert/update/delete 哪些动作触发。
- 映射为标准事件。

File Trigger 要求：

- 文件解析有 schema。
- 每行生成稳定 eventId / bizId。
- 支持部分成功、失败行导出、重试。
- 批次 id 必须进入 source metadata。

验收标准：

- 支持批次级和事件级审计。
- 单行失败不影响整批可恢复处理。
- 重复导入遵守 Event Inbox 幂等。

## 推荐执行顺序

优先级建议：

1. 阶段 1：UI 分类和文案收口。
2. 阶段 2：`event-in` schema 与 handler 对齐。
3. 阶段 3：HTTP / Webhook 入口配置可视化。
4. 阶段 4：Manual Trigger 生产补偿入口。
5. 阶段 5：MQ Topic 入站。
6. 阶段 6：Schedule Trigger。
7. 阶段 7：State Watch Trigger。
8. 阶段 8：CDC / File Trigger。

其中 1-3 是产品概念和已有能力收口，应该优先完成。4-8 是新增生产能力，需要单独评审、设计数据模型和补充 E2E。

## 第一批可执行任务清单

第一批建议只做低风险内容：

1. 新增 `EVENT_INPUT` schema 分类。
2. 把 `event-in` 显示名改为 `标准事件入口`。
3. 把 `event-in` 描述改为标准事件校验，不再写 HTTP/MQ/Webhook。
4. 左侧节点库新增 `事件输入` 分组。
5. 新增解释型节点 preset：
   - `HTTP 接口事件`
   - `Webhook 映射事件`
   - `MQ Topic 事件`
   - `定时事件`
   - `状态监听事件`
   - `人工触发事件`
6. 待实现节点必须在属性面板显示能力状态。
7. 默认模板继续使用 `标准事件入口 -> Inbox -> ...`，不加入未实现入口节点。
8. 更新文档和测试。

第一批验收：

- 管理台节点分类正确。
- 当前流程仍能 debug run。
- 发布校验不放宽。
- 未实现节点不会被误认为生产可执行。

