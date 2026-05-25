# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

本测试计划用于后续 Test Agent 在 Dev Agent 完成实现后执行。测试重点是“上线前配置校验 + 高危二级审批 + 上线阻断”的业务闭环，必须同时覆盖接口返回的逐项校验结果、持久化状态、审计记录和数据库 smoke。

测试分层：

- 单元测试：覆盖校验器纯逻辑，包括概率、预算、库存、时间冲突、互斥组、规则字段、JSON Schema、高危识别、审批版本绑定。
- 集成测试：通过后台 API 或 service 层提交活动配置，验证返回结构、审批状态、上线阻断和审计落库。
- 幂等与并发测试：重复提交、重复审批、并发上线、并发审批必须得到稳定且可解释的结果。
- PostgreSQL smoke：在真实 PostgreSQL 测试库上验证活动、quota、审批、审计记录的写入和读取，不允许只用 mock 覆盖。

测试数据原则：

- 每个用例使用独立 `tenantId`、`activityCode`、`exclusiveGroup`，避免跨用例污染。
- 金额、概率、库存使用 Decimal 字符串验证，禁止用浮点断言。
- 所有负向用例都必须断言机器可识别错误码、字段路径、总体结论和 `allowPublish=false`。
- 所有审批相关用例都必须断言审批绑定 `activityVersion`，配置变更后旧审批失效。

## Test Cases

### 1. 正常路径：低风险活动校验并上线

步骤：

1. 创建 `DRAFT` 活动，包含合法 `startTime/endTime/timezone`、合法 `triggerEvents`、合法规则字段、固定奖励、充足 `BUDGET` 和 `STOCK` quota。
2. 执行上线前完整校验。
3. 提交上线到 `SCHEDULED` 或 `ACTIVE`。

期望：

- 校验总体为 `PASS` 或仅含非阻断 `WARN`。
- 返回逐项校验结果，包含概率、预算、库存、时间、互斥、规则字段、JSON Schema、高危识别、审批状态。
- `allowPublish=true`。
- 活动状态成功进入目标状态。
- 审计记录包含操作者、活动 ID、版本、校验摘要和上线动作。

### 2. 概率配置非法阻断

覆盖：

- 概率百分比总和不等于 100。
- 权重总和为 0。
- 权重为负数。
- 权重不是合法 Decimal。
- `items` 为空。

期望：

- 总体为 `BLOCK`。
- 概率校验项为 `ERROR`。
- 返回具体字段路径，例如 `rewards[].rewardConfig.items[].weight`。
- `allowPublish=false`。
- 活动不能进入 `SCHEDULED` 或 `ACTIVE`。

### 3. 额度不足：预算超限或缺失预算

步骤：

1. 创建自动发奖资金类活动，单次最大暴露或多奖励合计暴露超过活动总预算。
2. 创建未配置 `BUDGET + ACTIVITY + TOTAL` 的自动发奖资金类活动。
3. 创建 `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 中奖励上限不可确定的活动。

期望：

- 明确超过预算时返回 `BLOCK` 或需求定义的高危阻断结果。
- 未配置预算但可自动发奖时返回 `APPROVAL_REQUIRED`，审批前 `allowPublish=false`。
- 结果说明当前暴露、预算额度、差额或无法确定暴露原因。
- 审计记录中包含阻断或高危原因摘要。

### 4. 库存不足或缺少库存

覆盖：

- 库存型 enabled reward 缺少 `STOCK + ACTIVITY + rewardCode` quota。
- 库存 quota 为 0。
- 库存 quota 为负数或非法 Decimal。
- 普通无限量活动缺少库存时只产生 `WARN` 的场景。

期望：

- 库存型或概率/实物/奖池类缺少库存时返回 `BLOCK` 或 `APPROVAL_REQUIRED`，不得直接上线。
- 普通无限量活动缺少库存时允许上线但返回可展示 `WARN`。
- 返回字段路径、rewardCode、当前库存和建议处理。

### 5. 活动时间冲突和互斥组冲突

步骤：

1. 在同一 tenant 下准备一个已可上线或在线活动。
2. 新活动使用相同 `exclusiveGroup`、重叠时间窗口、重叠 `triggerEvents`、`stackable=false`。
3. 再准备相邻但不重叠的时间窗口。
4. 再准备不同 tenant 的同组活动。

期望：

- 同 tenant、同组、时间重叠、触发事件重叠且不可叠加时返回 `BLOCK`。
- 相邻但不重叠的时间窗口不误判冲突。
- 不同 tenant 不互相影响。
- 相同优先级且无法唯一命中时返回互斥冲突错误。

### 6. 规则字段与 JSON Schema 校验

覆盖：

- `rulePhase=BUDGET`。
- 未知 `ruleType`。
- 字段路径不以 `attributes.*`、`state.*`、`facts.*` 开头。
- 非法操作符。
- 操作符和值类型不匹配。
- 未知顶层字段。
- 非法 Decimal。
- `AUTO + HTTP/WEBHOOK` 缺少 http(s) `targetRef`。
- 未支持的 `MQ`、`MANUAL_EXPORT`、`MANUAL`、未知 `calculateType`、未知 `grantChannel`。

期望：

- 每种非法配置返回机器可识别错误码和字段路径。
- 总体为 `BLOCK`。
- 不允许提交审批通过或上线。

### 7. 高危配置与二级审批正常路径

步骤：

1. 创建自动发放现金或礼金活动，配置合法但触发高危规则。
2. 执行上线前校验。
3. 提交审批。
4. 使用不同审批人审批通过。
5. 对同一活动版本上线。

期望：

- 校验总体为 `APPROVAL_REQUIRED`。
- 审批通过前 `allowPublish=false`。
- 审批人不能与提交人相同。
- 审批通过后同一版本允许上线。
- 审批记录包含审批人、审批意见、审批时间、活动版本和校验摘要。

### 8. 审批拒绝和版本失效

步骤：

1. 提交高危活动审批后执行审批拒绝。
2. 尝试上线。
3. 再次提交高危活动审批并审批通过。
4. 修改奖励、预算、库存、互斥组或时间窗口等关键字段。
5. 尝试使用旧审批上线。

期望：

- 审批拒绝后不能上线。
- 配置变更后活动版本变化。
- 旧审批对新版本失效。
- 必须重新校验和重新审批。

### 9. 幂等重复

覆盖：

- 同一活动版本重复执行上线前校验。
- 同一幂等键或同一提交请求重复提交审批。
- 审批通过请求重复发送。
- 已上线活动重复上线。

期望：

- 重复校验返回稳定结果，不产生重复副作用。
- 重复提交审批不会创建多条有效待审批记录。
- 重复审批通过返回已处理或幂等成功，不重复推进状态、不重复写核心业务记录。
- 重复上线不重复改变版本、不重复创建上线审计；如写审计，必须标识为重复请求。

### 10. 并发

覆盖：

- 两个请求同时对同一活动版本提交审批。
- 两个不同审批人同时审批同一审批单，一个通过一个拒绝。
- 审批通过和配置修改同时发生。
- 审批通过和上线同时发生。
- 两个冲突活动同时申请上线。

期望：

- 同一活动版本最多存在一个有效审批流程。
- 审批单最终状态只能有一个确定结果，不出现同时通过和拒绝。
- 配置修改和审批通过并发时，版本检查阻止旧审批上线。
- 冲突活动并发上线时，最终不能出现两个同 tenant、同互斥组、重叠时间且不可叠加的在线活动。
- 数据库事务、唯一约束或乐观锁冲突返回可解释错误。

### 11. PostgreSQL smoke

步骤：

1. 使用 PostgreSQL 测试库启动完整集成测试环境。
2. 插入或通过 API 创建活动、规则、奖励、quota。
3. 执行一次低风险 PASS 流程。
4. 执行一次高危审批通过流程。
5. 执行一次 BLOCK 阻断流程。
6. 查询 PostgreSQL 中活动状态、quota、审批记录、审计记录。

期望：

- PostgreSQL 表结构、迁移和 DAO/repository 能正常工作。
- 活动状态与 API 返回一致。
- 审批记录绑定活动版本。
- 审计记录至少覆盖校验、提交审批、审批通过/拒绝、上线阻断。
- tenant 隔离字段正确写入，跨 tenant 查询不串数据。

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

后续 Test Agent 应根据项目实际测试标签和数据库配置追加执行以下命令清单；如命令名称与仓库约定不一致，需先在测试报告中说明替代命令：

~~~bash
# 单元与集成测试，必须包含校验器、审批、幂等、并发相关测试
go test ./... -count=1

# 开启 race 检测，重点覆盖审批、上线、并发冲突路径
go test ./... -race -count=1

# PostgreSQL smoke；如仓库使用 build tags、环境变量或 compose profile，请替换为实际约定
POSTGRES_DSN="$POSTGRES_DSN" go test ./... -count=1 -run 'Postgres|PostgreSQL|Smoke|Approval|Validation'

# 构建验证
go build ./...
~~~

禁止 Test Agent 跳过 PostgreSQL smoke；如果本地缺少 PostgreSQL 测试环境，测试结论必须标记为 BLOCK，并说明缺少的 DSN、容器或初始化脚本。

## Data / Smoke

建议准备以下最小数据集：

- `tenant-pass-001`：低风险固定奖励活动，预算和库存充足，预期 PASS。
- `tenant-risk-001`：自动发放现金/礼金活动，预算合法但触发高危，预期 APPROVAL_REQUIRED。
- `tenant-block-001`：概率非法、预算超限、库存不足、非法规则字段各一份，预期 BLOCK。
- `tenant-conflict-001`：已有在线活动 + 新建冲突活动，预期互斥/时间冲突 BLOCK。
- `tenant-idem-001`：同一活动版本重复提交校验、审批、上线，预期幂等。
- `tenant-concurrency-001`：同一活动版本并发审批/上线，预期单一最终状态。
- `tenant-pg-smoke-001`：PostgreSQL smoke 专用，测试后可按 tenant 清理。

PostgreSQL smoke 必查项：

- 活动配置版本是否随关键字段变更递增。
- 审批记录是否保存提交人、审批人、审批意见、审批时间、活动版本、校验摘要。
- 审计记录是否保存操作者、活动 ID、版本、动作、阻断/审批原因。
- quota 的 Decimal 值读取、比较和持久化不丢精度。
- 同名 `exclusiveGroup` 在不同 tenant 下不会互相冲突。

## Decision

~~~text
PASS
~~~

理由：`review.md` 已明确 PASS，需求和分析足以设计后续测试。测试计划已覆盖正常路径、额度不足、幂等重复、并发和 PostgreSQL smoke，并给出 Test Agent 应执行的命令清单。

需求：2026-05-24-113400-7
