# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Review Gate

已读取 `tasks/2026-05-24-113929-12/review.md`，Decision 为 `PASS`，允许进入测试计划设计。

## Test Strategy

本轮测试目标是验证“管理后台通用配置页面”是否严格复用现有 Admin API 和通用活动模型，覆盖活动创建/编辑、规则、奖励、Quota、审批、上线前校验、模拟运行、审计展示，并防止活动专属页面、前端 mock 成功、不可用能力伪装可运行等回归。

Test Agent 后续执行时应优先使用自动化测试覆盖服务层/API/页面集成逻辑，再补充 PostgreSQL smoke 和必要的手工 UI 验收。所有测试都必须确认页面结果来自真实后端响应，不能只检查前端本地状态。

核心覆盖维度：

- 正常路径：创建通用活动、配置规则/奖励/quota、提交审批、审批通过、上线前校验、模拟运行、审计查看。
- 额度不足：Quota 超限时页面展示 `QUOTA_EXCEEDED`，不展示可执行发奖预览，不产生正式业务副作用。
- 幂等重复：重复保存、重复审批、重复模拟、重复标准事件样例必须保持幂等或返回明确重复/版本冲突原因。
- 并发：并发编辑同一活动版本、并发审批、并发 quota 预览/扣减路径必须验证乐观锁、版本校验和一致性提示。
- PostgreSQL smoke：确认活动、规则、奖励、quota、审批、审计等核心表写入和查询链路可用，租户隔离和唯一约束生效。

## Test Cases

### TC-01 正常路径：首存/充值类活动完整闭环

前置条件：

- 管理后台服务、API 服务、PostgreSQL 可连接。
- 使用测试租户 `tenant_test_admin`。
- 清理或使用唯一 `activityCode`，避免与历史数据冲突。

步骤：

1. 通过通用配置页面创建活动：`activityType` 使用普通配置化类型，`triggerEvents=["DEPOSIT_SUCCESS"]`，`status=DRAFT`，设置有效 `startTime/endTime/timezone`。
2. 配置 `config.entry`、`config.display`、`config.audience`、`config.metadata`，不得出现未知一层字段。
3. 配置 `CONDITION` 规则：`attributes.isFirstDeposit == true` 且 `attributes.amount >= 100`。
4. 配置奖励：`rewardType=BONUS`，金额/比例计算配置合法，`grantMode=AUTO`，`grantChannel=HTTP` 或 `WEBHOOK`，`targetRef` 合法。
5. 配置 quota：至少包含活动预算或用户次数限制。
6. 保存后提交审批，使用不同审批人审批通过。
7. 执行上线前校验。
8. 输入标准事件样例执行模拟运行。
9. 查看活动详情、审批记录、审计摘要。

期望结果：

- 活动创建/编辑成功，页面展示后端返回的活动 ID、版本、状态、更新时间。
- 审批通过后版本一致，允许进入可上线状态；未审批版本不能直接上线。
- 模拟结果展示命中活动、规则解释、奖励预览、quota 影响、grant order/outbox 预览。
- 审计中能看到创建、更新、审批、模拟、quota 配置等关键动作。
- 未新增任何活动专属页面或活动专属 controller/job/topic。

### TC-02 正常路径：非首存事件不命中

步骤：

1. 复用 TC-01 活动。
2. 输入 `eventType=DEPOSIT_SUCCESS`，但 `attributes.isFirstDeposit=false`。
3. 执行模拟运行。

期望结果：

- 页面展示未命中或规则拒绝原因。
- 不展示可执行 grant order/outbox 预览。
- 不消耗 quota，不写入正式 participation/grant_order/outbox。

### TC-03 非法配置拦截

步骤：

1. 尝试保存 `config_json` 未知一层字段，例如 `config.customBusinessModel`。
2. 尝试保存非法时间：`startTime >= endTime` 或缺少 timezone。
3. 尝试保存非法规则阶段：`rulePhase=BUDGET`。
4. 尝试保存非法规则字段路径，例如外部业务库字段或未允许的根路径。
5. 尝试保存 `grantMode=MANUAL_EXPORT` 或不可用 `SCRIPT/FORMULA/LIMIT` 配置。
6. 尝试保存非法 `targetRef`。

期望结果：

- 所有非法配置均被真实后端拒绝或在页面明确禁用。
- 页面展示字段级错误或明确机器原因，不以 mock 成功替代。
- `FORMULA`、`LIMIT`、`SCRIPT`、`MANUAL`、`MANUAL_EXPORT` 不得呈现为可运行能力。

### TC-04 额度不足 / Quota 超限

步骤：

1. 创建或复用活动，将活动预算、库存或用户次数 quota 设置为极小值。
2. 构造会命中规则且奖励金额会超过 quota 的标准事件样例。
3. 执行模拟运行。
4. 查询页面展示的 quota 状态。

期望结果：

- 模拟结果展示 `QUOTA_EXCEEDED`。
- 页面展示业务说明，例如预算不足、库存不足或用户次数已达上限。
- 不展示可执行 grant order/outbox 预览。
- 不写入正式业务副作用，不扣减正式 quota。
- 缺少 quota 行的活动应明确展示 `unlimited`，不得前端推断为超限或通过。

### TC-05 Fact Provider 缺失 / 失败

步骤：

1. 配置引用 `facts.<factCode>.*` 的规则。
2. 不配置或禁用对应 Fact Provider。
3. 执行上线前校验和模拟运行。

期望结果：

- 上线前校验阻止上线或返回明确失败。
- 模拟结果展示 `FACT_QUERY_FAILED`、`FACT_MAPPING_FAILED` 或 `FACT_FIELD_MISSING` 等真实机器原因。
- 页面不得自行推断为通过。

### TC-06 幂等重复：保存、审批、模拟

步骤：

1. 对同一活动配置发起两次相同保存请求。
2. 使用同一 `expectedVersion` 重复提交审批。
3. 对已审批版本重复审批通过。
4. 使用相同标准事件样例或相同幂等键重复模拟运行。

期望结果：

- 重复保存不产生重复活动；若版本变化，页面展示最新版本或版本冲突。
- 重复审批不得产生多条有效审批通过状态；版本不一致时提示刷新后重试。
- 重复模拟不产生正式 participation/grant_order/outbox/quota 消耗。
- 审计记录可追踪重复操作，且不破坏活动最终状态。

### TC-07 并发：编辑、审批、quota 预览

步骤：

1. 打开同一活动两个编辑会话，读取相同版本。
2. 会话 A 修改规则并保存成功。
3. 会话 B 基于旧版本修改奖励并保存。
4. 两个审批人同时对同一版本发起审批操作。
5. 并发发起多次命中同一低 quota 活动的模拟运行。

期望结果：

- 会话 B 保存被拒绝或提示版本冲突，不能覆盖会话 A 的配置。
- 并发审批遵循后端版本和审批流约束；提交人不能审批自己，高危活动二级审批要求被展示并执行。
- 并发模拟无正式副作用，quota 预览结果一致且不会变成真实扣减。
- 页面错误提示来自后端响应，包含可理解的业务原因。

### TC-08 多租户与唯一约束

步骤：

1. 在租户 A 创建 `activityCode=COMMON_DEPOSIT_TEST`。
2. 在同租户 A 再次创建同名活动。
3. 在租户 B 创建同名活动。
4. 配置 `entry.tenantIds` 包含非本活动租户。

期望结果：

- 同租户重复 `activityCode` 被拒绝。
- 不同租户允许复用 `activityCode`。
- `entry.tenantIds` 为空或只包含自身租户时合法；包含其他租户时被拒绝。
- 活动列表、审批、审计、quota 查询均保持租户隔离。

### TC-09 PostgreSQL Smoke

步骤：

1. 通过 Admin API 或页面创建一条测试活动。
2. 配置至少一条规则、一条奖励、一条 quota。
3. 提交审批并审批通过。
4. 查询 PostgreSQL 中活动、规则、奖励、quota、审批、审计相关表。
5. 验证同租户唯一约束、版本字段、状态字段、创建/更新时间、审计字段。

期望结果：

- 核心记录均成功落库，字段值与 API/page 返回一致。
- 活动版本更新符合预期，审批与版本绑定。
- 审计记录包含操作人、动作、资源、版本、原因和时间。
- PostgreSQL 查询无错误，迁移后的 schema 与代码访问一致。

### TC-10 页面边界和空态

步骤：

1. 使用无活动的测试租户打开活动列表。
2. 打开无审计记录的活动详情。
3. 模拟审计查询失败或后端返回空。
4. 查看配置复杂活动时的摘要展示。

期望结果：

- 空活动列表展示创建入口和空态，不报错。
- 审计为空时展示空态；审计查询失败不影响活动详情主体展示。
- 配置摘要展示关键字段，不只暴露原始 JSON。

## Required Commands

Test Agent 后续应执行以下命令；本 Test Plan Agent 不执行这些命令。

~~~bash
go test ./...
go build ./...
~~~

如果仓库提供前端或 E2E 专用脚本，Test Agent 可在确认脚本名称后补充执行，例如：

~~~bash
npm test
npm run build
npm run e2e
~~~

如实际项目不是 Node 前端或未提供上述脚本，不应强行执行不存在的命令，应在 test.md 中记录跳过原因。

## Data / Smoke

建议测试数据：

- `tenantId`: `tenant_test_admin`
- `activityCode`: `COMMON_DEPOSIT_TEST_<timestamp>`
- `activityName`: `Common Deposit Test`
- `eventType`: `DEPOSIT_SUCCESS`
- 命中事件 attributes:

~~~json
{
  "isFirstDeposit": true,
  "amount": "150.00",
  "currency": "USD",
  "channel": "WEB",
  "ip": "127.0.0.1",
  "deviceId": "test-device-001"
}
~~~

- 不命中事件 attributes:

~~~json
{
  "isFirstDeposit": false,
  "amount": "150.00",
  "currency": "USD",
  "channel": "WEB",
  "ip": "127.0.0.1",
  "deviceId": "test-device-001"
}
~~~

- Quota 超限事件 attributes:

~~~json
{
  "isFirstDeposit": true,
  "amount": "999999.00",
  "currency": "USD",
  "channel": "WEB",
  "ip": "127.0.0.1",
  "deviceId": "test-device-001"
}
~~~

PostgreSQL smoke 检查清单：

- 活动主表存在测试活动，`tenant_id`、`activity_code`、`status`、`version`、`start_time`、`end_time` 正确。
- 规则表存在 `MATCH/QUALIFY` 或对应阶段规则，字段路径只来自允许根路径。
- 奖励表存在 `BONUS` 和合法 `grant_channel/target_ref`。
- quota 表存在预算/库存/用户限制配置；缺失 quota 行的活动在页面显示 `unlimited`。
- 审批表记录审批人、审批动作、版本、原因。
- 审计表记录创建、更新、审批、模拟、quota 创建等动作。
- 同租户重复 `activity_code` 插入或 API 创建被拒绝；不同租户同名可创建。

手工验收补充：

- 检查导航或路由中没有新增具体活动专属页面。
- 检查通用配置页面能复用配置至少两种普通活动类型或触发事件。
- 检查不可用能力被禁用、隐藏或展示真实后端拒绝原因。

## Decision

~~~text
PASS
~~~

理由：review.md 已 PASS，本测试计划覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke，并包含后续 Test Agent 可执行的命令清单。

需求：2026-05-24-113929-12
