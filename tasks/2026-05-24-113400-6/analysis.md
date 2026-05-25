# Analysis

## 需求 ID

```text
2026-05-24-113400-6
```

## 标题

```text
发奖回调与预算确认释放
```

## 分析依据

本次只做需求分析和需求细化，未读取业务代码，未设计具体代码改法，未修改代码。

已读取材料：

```text
tasks/2026-05-24-113400-6/task.md
tasks/2026-05-24-113400-6/review.md
README.md
CLAUDE.md
docs/architecture.md
docs/data-model.md
docs/api.md
docs/operations.md
docs/roadmap.md
../act-analysis/22-独立活动营销中台设计.md
```

未发现 `PRD/` 目录；以上文档作为本轮 PRD / 设计文档 / 业务规则 / API / 数据字典依据。

## 背景目标

ExMarketing 是独立活动营销中台，不直接给用户加钱，只接收标准事件，完成活动匹配、规则判断、奖励计算、预算库存占用、participation、grant_order 和 outbox 生成，再由外部钱包、积分、优惠券或礼品系统完成真实发放。

上游设计中的目标链路为：

```text
外部事件进入
-> 活动匹配 / 规则判断 / 奖励计算
-> 冻结预算 / 库存 / 次数
-> 生成 participation / grant_order / outbox
-> outbox worker 发出发奖指令
-> 外部发奖系统回调 SUCCESS / FAILED
-> SUCCESS 确认冻结消耗
-> FAILED 释放冻结占用
-> 更新 grant_order 与 participation
-> 记录审计并支持对账
```

本任务目标是补齐发奖回调后的预算安全闭环和最小对账能力：

```text
1. SUCCESS 回调确认单笔 grant_order 成功，并把该单对应冻结占用确认为已消耗。
2. FAILED 回调确认单笔 grant_order 失败，并释放该单对应冻结占用。
3. 重复回调必须幂等，不重复确认、不重复释放、不重复推进状态。
4. 冲突回调不得覆盖最终状态，不得自动反向改账。
5. 未知单号或跨 tenant 单号不得更新任何业务数据。
6. 运营 / 财务能查询 grant_order、participation、回调结果和预算确认 / 释放状态，定位长期冻结悬挂。
```

## 本轮返工收敛结论

上一轮 Review 的核心阻断点是：只存在聚合 `frozen_value` 时，无法证明每笔 FAILED 回调只释放对应 `grant_order` 的冻结份额，也无法证明重复回调不会重复释放。

本轮将需求收敛为强制主路径：

```text
本任务必须具备 grant_order 级 quota reservation / ledger / voucher 等价凭证。
每笔 grant_order 必须能关联到本次发奖占用的每一条 quota 份额。
回调只能确认或释放该 grant_order 关联凭证中的未终结份额。
不能仅依赖 marketing_quota 的 aggregate frozen_value 判断应确认或释放多少。
不能把“used_value 已预扣”作为本任务默认方案。
```

等价凭证是需求层概念，不限定具体代码实现方式，但必须满足以下业务属性：

```text
按 tenantId + grantOrderId / commandId 唯一归属。
记录 quotaType、scope、scopeKey、reservedAmount。
记录 reservationStatus，至少能区分 FROZEN / CONFIRMED / RELEASED。
记录 createdAt、updatedAt，并能表达 confirmedAt / releasedAt 或等价终结时间。
同一凭证只能从 FROZEN 终结一次，不能重复 confirmed 或 released。
对账查询能从 grant_order 反查该单影响了哪些 quota 份额。
历史已生成但缺少 reservation 凭证的 grant_order，不允许自动确认或释放预算，必须进入人工排查结果。
```

这消除了资金闭环的 P0 不确定性；如果 Dev 阶段发现无法提供等价凭证，本任务必须回退为 BLOCK，不能降级为聚合字段猜测。

## 参与方

外部发奖系统：

```text
接收 Reward Command。
按 commandId 幂等执行真实发放。
向 ExMarketing 回调 SUCCESS 或 FAILED。
```

ExMarketing OpenAPI：

```text
接收 POST /openapi/v1/reward/callback。
校验请求模型。
以 tenantId + commandId 作为唯一业务定位边界。
返回成功、重复、冲突、未知单号或参数错误等机器可识别结果。
```

grant_order：

```text
单笔发奖指令与回调结果主凭证。
bill_no 对应外部 commandId。
状态范围沿用 PENDING / SENDING / SUCCESS / FAILED / RETRY / CLOSED。
```

quota reservation / ledger / voucher：

```text
单笔 grant_order 对 quota 占用、确认、释放的对账凭证。
用于证明 SUCCESS / FAILED 回调只影响对应发奖单份额。
```

participation：

```text
用户参与活动的业务凭证。
回调结果明确后按当前枚举推进到 GRANTED 或 FAILED。
```

运营 / 财务 / 对账人员：

```text
查询发奖单、参与记录、回调结果、quota reservation 状态和长期冻结悬挂。
排查重复回调、冲突回调、未知单号、预算漂移。
```

Outbox worker / DLQ：

```text
负责发奖指令投递、重试、DLQ 和人工关闭。
本任务不重做投递机制，只消费外部回调结果。
```

## 输入输出

### 回调输入

接口：

```http
POST /openapi/v1/reward/callback
Content-Type: application/json
```

请求字段：

```text
tenantId          必填；租户 ID。
commandId         必填；对应 grant_order.bill_no。
status            必填；只接受 SUCCESS / FAILED。
externalGrantId   SUCCESS 必填；FAILED 可选。
message           可选；外部系统说明，需限制长度或摘要保存。
finishedAt        必填；外部发奖完成时间，需可解析。
```

幂等定位键：

```text
tenantId + commandId
```

禁止只按 `commandId` 更新，避免跨 tenant 串单。

### SUCCESS 输出

首次成功回调应产生以下业务结果：

```text
grant_order.grant_status = SUCCESS。
grant_order 记录 externalGrantId、finishedAt、message 摘要和本系统处理时间。
该 grant_order 关联 quota reservation 从 FROZEN -> CONFIRMED。
marketing_quota 聚合值同步体现：frozen_value 减少，used_value 增加。
若同一 participation 下所有 grant_order 均 SUCCESS，则 participation.status = GRANTED。
审计记录 CALLBACK_SUCCESS 和 QUOTA_CONFIRMED。
```

### FAILED 输出

首次失败回调应产生以下业务结果：

```text
grant_order.grant_status = FAILED。
grant_order 记录 failed message 摘要、finishedAt 和本系统处理时间。
该 grant_order 关联 quota reservation 从 FROZEN -> RELEASED。
marketing_quota 聚合值同步体现：frozen_value 减少，used_value 不增加。
按本分析的多发奖单规则决定 participation 是否进入 FAILED。
审计记录 CALLBACK_FAILED 和 QUOTA_RELEASED。
```

### 重复回调输出

重复回调定义：

```text
同一 tenantId + commandId 已处于相同最终状态 SUCCESS 或 FAILED。
重复请求不要求 message 或 finishedAt 完全一致，但不得改变最终状态和 quota 终结结果。
```

重复回调结果：

```text
返回 200 或业务成功响应，包含 duplicate=true 或等价标识。
不再次更新 quota 聚合值。
不再次终结 reservation。
不再次推进 participation。
审计记录 CALLBACK_DUPLICATE 或保留可追踪日志。
```

### 异常输出

至少需要机器可识别错误：

```text
INVALID_CALLBACK_REQUEST   缺少必填字段、status 非法、finishedAt 不可解析、未知字段策略拒绝。
GRANT_ORDER_NOT_FOUND      tenantId + commandId 找不到发奖单。
CALLBACK_STATUS_CONFLICT   已有最终态与本次 status 冲突，或 externalGrantId 冲突。
GRANT_ORDER_CLOSED         发奖单已 CLOSED，普通回调不可重新打开。
QUOTA_RESERVATION_MISSING  找到 grant_order 但缺少该单 reservation 凭证。
QUOTA_RESERVATION_CONFLICT reservation 已终结为相反结果或金额不一致。
QUOTA_AGGREGATE_CONFLICT   聚合 quota 与 reservation 无法一致确认 / 释放。
```

错误响应不得泄露其他 tenant 的发奖单是否存在。

### 对账查询输出

本任务需要最小机器可读对账查询能力。可以复用或扩展管理查询入口，但必须能表达：

```text
tenantId
commandId / billNo
grantOrderId
externalGrantId
activityId
participationId
subjectType / subjectId
rewardType / rewardAmount / currency
grantStatus
participationStatus
callbackStatus
callbackMessage 摘要
finishedAt
serverUpdatedAt
quota reservations:
  quotaType
  scope
  scopeKey
  reservedAmount
  reservationStatus: FROZEN / CONFIRMED / RELEASED / CONFLICT / MISSING
quota aggregate snapshot:
  totalValue
  usedValue
  frozenValue
isHangingFrozen
lastErrorCode / lastErrorMessage
```

最低查询维度：

```text
按 tenantId 查询发奖单。
按 tenantId + commandId 查询单笔发奖与回调结果。
按 activityId 查询活动维度发奖状态。
按 subjectId 查询用户维度发奖状态。
按 grantStatus 查询 PENDING / SENDING / RETRY / SUCCESS / FAILED / CLOSED。
按 reservationStatus 查询 FROZEN / CONFIRMED / RELEASED / CONFLICT / MISSING。
能识别长期悬挂冻结：发奖单未 SUCCESS / FAILED 且存在 FROZEN reservation，超过查询传入时间窗口。
```

长期复杂能力如批量导出、财务结算文件、跨系统自动勾稽、Admin UI 可后续拆分；本期只要求机器可读查询和运维说明。

## 业务规则

### 预算冻结、确认与释放

目标语义：

```text
发奖前：本次 grant_order 对应额度进入 FROZEN reservation，并体现在 marketing_quota.frozen_value。
SUCCESS：该 grant_order 的 FROZEN reservation 变为 CONFIRMED，frozen_value 减少，used_value 增加。
FAILED：该 grant_order 的 FROZEN reservation 变为 RELEASED，frozen_value 减少，used_value 不增加。
重复 SUCCESS / FAILED：reservation 已是相同终态时幂等返回，不改变聚合值。
冲突 SUCCESS / FAILED：不得反向终结，不得自动改账。
```

本任务不接受以下语义作为验收通过：

```text
只更新 grant_order，不处理 quota reservation。
只依赖 aggregate frozen_value 推断释放金额。
在发奖前直接 used_value 预扣，并把 FAILED 回退作为默认方案。
FAILED 后仅标记失败但让冻结长期悬挂。
SUCCESS / FAILED 冲突时以后到回调覆盖先到最终态。
```

若已有历史数据或早期链路只产生 `used_value` 预扣、没有 FROZEN reservation，本任务的需求处理为：

```text
新链路必须从发奖前 reservation 开始闭环。
旧 grant_order 缺少 reservation 时，callback 不自动确认或释放预算。
旧数据通过对账查询暴露为 QUOTA_RESERVATION_MISSING / MISSING，需要人工或迁移任务处理。
是否迁移历史 used_value 预扣数据不是本任务默认范围，除非 Owner 另行补充。
```

### Quota 类型影响范围

本任务涉及所有由发奖前占用产生的 quota：

```text
BUDGET：金额类预算，reservedAmount 为该 grant_order 奖励金额或其预算计量金额。
STOCK：奖品库存，reservedAmount 通常为 1 或奖励配置定义的库存计量。
USER_LIMIT：用户参与次数，reservedAmount 通常为 1。
```

对 `DAILY_LIMIT`、IP、DEVICE 等未来 quota 类型，本任务只要求 reservation 模型可扩展，不要求实现新增类型业务规则。

### Grant Order 状态流转

允许状态沿用当前文档：

```text
PENDING
SENDING
SUCCESS
FAILED
RETRY
CLOSED
```

允许流转：

```text
PENDING / SENDING / RETRY -> SUCCESS
PENDING / SENDING / RETRY -> FAILED
SUCCESS -> SUCCESS    幂等重复成功
FAILED -> FAILED      幂等重复失败
CLOSED -> CLOSED      普通回调不得重新打开
```

禁止流转：

```text
SUCCESS -> FAILED
FAILED -> SUCCESS
CLOSED -> SUCCESS
CLOSED -> FAILED
```

冲突定义：

```text
已 SUCCESS 后收到 FAILED。
已 FAILED 后收到 SUCCESS。
同一 commandId 已记录 externalGrantId 后，又收到不同 externalGrantId 且试图改变最终结果。
reservation 已 CONFIRMED 后请求 RELEASED。
reservation 已 RELEASED 后请求 CONFIRMED。
```

冲突处理：

```text
返回 CALLBACK_STATUS_CONFLICT 或 QUOTA_RESERVATION_CONFLICT。
保留原最终态和原 quota 终结结果。
记录 CALLBACK_CONFLICT 审计。
对账查询可检索冲突记录。
```

### Participation 状态流转

允许状态沿用当前文档：

```text
INIT
QUALIFIED
REJECTED
REWARDABLE
GRANTED
FAILED
```

本轮需求明确不新增 `PARTIAL` 或 `NEED_REVIEW` 状态，避免扩大枚举和外部语义。多发奖单部分成功 / 部分失败的业务归属收敛为：

```text
单 grant_order participation：
  grant_order SUCCESS -> participation.GRANTED
  grant_order FAILED  -> participation.FAILED

多 grant_order participation：
  全部 grant_order SUCCESS -> participation.GRANTED
  任一 grant_order FAILED，且不存在 PENDING / SENDING / RETRY 时 -> participation.FAILED
  存在 PENDING / SENDING / RETRY 时 -> participation 保持 REWARDABLE，不提前 GRANTED / FAILED
  部分 SUCCESS + 部分 FAILED 的最终态 -> participation.FAILED，对账明细展示已成功与已失败 grant_order
```

该规则表示 participation 的最终业务结果是“整组奖励未完整发放”。已成功单不会自动撤销；失败单释放自身 reservation；运营通过 grant_order 明细识别部分成功事实。

### 幂等与并发

```text
回调幂等键为 tenantId + commandId。
同一回调重复到达，不得重复终结 reservation。
并发重复 SUCCESS 只有一个请求能从 FROZEN -> CONFIRMED，其余识别为 duplicate。
并发重复 FAILED 只有一个请求能从 FROZEN -> RELEASED，其余识别为 duplicate。
并发 SUCCESS 与 FAILED 只能有一个结果终结 reservation；另一方必须返回冲突，不得覆盖。
grant_order 状态、reservation 状态、quota 聚合值、participation 状态必须作为同一业务原子结果提交或全部不提交。
```

### 回调安全边界

当前文档只要求后续生产版本增加签名或 token 鉴权，现有 API 示例未定义签名字段。

本任务安全边界收敛为：

```text
本期不设计签名算法、不新增 token 协议、不把鉴权作为本任务验收项。
本期 callback 只能作为本地开发、测试、受控内网或网关已鉴权后的内部入口验收。
本任务不得声称该 callback 可直接公网生产暴露。
docs/operations.md 的生产前检查必须继续要求 Reward callback 已加签名或 token 校验。
如果 Owner 要求本期直接生产公网可用，则必须另起或前置 callback 鉴权需求。
```

### 审计

以下动作必须可追踪：

```text
CALLBACK_SUCCESS
CALLBACK_FAILED
CALLBACK_DUPLICATE
CALLBACK_CONFLICT
CALLBACK_UNKNOWN_COMMAND
CALLBACK_INVALID
QUOTA_CONFIRMED
QUOTA_RELEASED
QUOTA_RESERVATION_MISSING
QUOTA_AGGREGATE_CONFLICT
```

审计 payload 至少包含：

```text
tenantId
commandId
grantOrderId
participationId
activityId
subjectType / subjectId
rewardType / rewardAmount
externalGrantId
oldGrantStatus / newGrantStatus
oldParticipationStatus / newParticipationStatus
quota reservation ids or equivalent identifiers
quota deltas
status
message 摘要
finishedAt
serverProcessedAt
errorCode
```

不得记录：

```text
secret
token
完整 Authorization header
签名密钥
超长外部响应体
```

## 边界条件

```text
规则拒绝、零奖励、quota 不足、重复事件未生成 grant_order 时，不应存在可处理回调。
tenantId 缺失不得更新业务数据。
commandId 缺失不得更新业务数据。
status 缺失或不是 SUCCESS / FAILED 不得按成功或失败处理。
finishedAt 缺失或不可解析不得更新业务数据。
finishedAt 早于或晚于系统时间不直接拒绝，但必须原样记录，并用 serverProcessedAt 表达本系统处理时间。
SUCCESS 的 externalGrantId 必填；FAILED 的 externalGrantId 可选。
message 为空不影响状态推进。
message 过长必须摘要或截断，不影响合法状态推进。
未知 JSON 字段按 API 既有严格校验风格处理：拒绝并返回 INVALID_CALLBACK_REQUEST。
回调到达早于 outbox SENT 写回时，只要 grant_order 与 FROZEN reservation 存在且状态允许，仍可处理。
outbox DLQ 不等于发奖失败；只有外部明确 FAILED 回调或人工处理规则明确时才释放 reservation。
DLQ_CLOSED 是否释放 reservation 不属于本任务默认范围。
CLOSED grant_order 收到普通回调不得重新打开。
tenantId 正确但 commandId 属于其他 tenant，按 GRANT_ORDER_NOT_FOUND 处理。
commandId 正确但 tenantId 错误，按 GRANT_ORDER_NOT_FOUND 处理。
缺少 reservation 的 grant_order 不得自动确认 / 释放 quota，只能返回 QUOTA_RESERVATION_MISSING 并进入对账。
reservation 金额与 grant_order 奖励金额不一致时，不得继续确认 / 释放，进入 QUOTA_RESERVATION_CONFLICT。
quota 聚合值不足以执行 reservation 终结时，不得产生负数，进入 QUOTA_AGGREGATE_CONFLICT。
内存仓储只能用于基本行为验证；并发、事务、预算安全验收必须使用 PostgreSQL 语义。
```

## 异常场景

```text
SUCCESS 首次回调：
  grant_order SUCCESS，reservation CONFIRMED，frozen 减少，used 增加，必要时 participation GRANTED。

FAILED 首次回调：
  grant_order FAILED，reservation RELEASED，frozen 减少，used 不变，必要时 participation FAILED。

重复 SUCCESS：
  duplicate=true，reservation 保持 CONFIRMED，quota 不变。

重复 FAILED：
  duplicate=true，reservation 保持 RELEASED，quota 不变。

SUCCESS 后 FAILED：
  返回冲突，保持 SUCCESS / CONFIRMED，不释放已确认 quota。

FAILED 后 SUCCESS：
  返回冲突，保持 FAILED / RELEASED，不重新消耗 quota。

未知 commandId：
  返回 GRANT_ORDER_NOT_FOUND，不更新 grant_order、participation、reservation、quota。

跨 tenant commandId：
  返回 GRANT_ORDER_NOT_FOUND，不泄露其他 tenant 数据。

缺少 reservation：
  返回 QUOTA_RESERVATION_MISSING，不猜测金额，不更新 quota。

reservation 已终结为相反结果：
  返回 QUOTA_RESERVATION_CONFLICT，保留原结果。

数据库事务或原子提交失败：
  不得出现 grant_order 已 SUCCESS 但 reservation 未 CONFIRMED，或 reservation 已 RELEASED 但 grant_order 仍待处理。
```

## 非目标范围

```text
不实现真实钱包、积分、优惠券、实物或权益发放。
不改变外部发奖系统的幂等责任；外部仍必须按 commandId 幂等。
不重做 Outbox worker retry / DLQ / replay / close。
不实现 MQ Command Out。
不实现完整 Admin UI。
不实现批量导出、财务结算报表或跨系统自动勾稽。
不实现 callback 签名算法、token 协议或 OpenAPI 鉴权。
不把本期无签名 callback 定义为生产公网可暴露能力。
不新增 PARTIAL / NEED_REVIEW participation 状态。
不实现复杂人工补偿状态机。
不定义 DLQ_CLOSED 自动释放预算规则。
不迁移缺少 reservation 的历史 used_value 预扣数据，除非 Owner 另行补充。
不新增业务专用活动逻辑，不为单个接入方写死字段。
不连接外部业务数据库，不引用外部业务代码包。
```

## 风险

已通过本轮需求收敛消除上一轮 P0/P1 阻断项：

```text
上一轮 P0：只有 aggregate frozen_value 无法证明单笔释放。
本轮处理：强制 grant_order 级 reservation / ledger / voucher 等价凭证，缺失则回调 BLOCK 为对账异常。

上一轮 P1：部分成功 / 部分失败状态不明确。
本轮处理：不新增 PARTIAL；多发奖单只要最终存在失败且无待处理单，participation.FAILED，对账展示明细。

上一轮 P1：无鉴权是否进入生产边界不明确。
本轮处理：本期不做鉴权，callback 只验收本地 / 内网 / 网关已鉴权后入口；生产公网暴露必须前置鉴权需求。
```

剩余风险：

```text
P2: 对账查询如果扩展为报表、导出、财务结算，会扩大范围；本期限定机器可读最小查询。
P2: 历史 grant_order 缺少 reservation 时无法自动修复，只能识别为 MISSING 并交由人工或迁移任务处理。
P2: 外部系统若不能提供 externalGrantId，SUCCESS 将被拒绝，可能需要接入方适配；这是本轮为提升成功凭证质量作出的业务约束。
P2: 多发奖单部分成功最终 participation.FAILED 可能与部分运营口径不同，但 grant_order 明细可保留事实；如需 PARTIAL 应另起需求。
```

## 验收标准

### 功能验收

```text
1. SUCCESS 回调能按 tenantId + commandId 找到 grant_order，更新为 SUCCESS，并把该单 reservation 从 FROZEN 确认为 CONFIRMED。
2. SUCCESS 回调后 quota 聚合值正确体现 frozen_value 减少、used_value 增加。
3. FAILED 回调能按 tenantId + commandId 找到 grant_order，更新为 FAILED，并把该单 reservation 从 FROZEN 释放为 RELEASED。
4. FAILED 回调后 quota 聚合值正确体现 frozen_value 减少、used_value 不增加。
5. 单 grant_order participation 成功后推进为 GRANTED，失败后推进为 FAILED。
6. 多 grant_order participation 全部 SUCCESS 后才推进为 GRANTED。
7. 多 grant_order participation 存在 PENDING / SENDING / RETRY 时，不因部分 SUCCESS 提前 GRANTED。
8. 多 grant_order participation 最终部分 SUCCESS + 部分 FAILED 时，participation.FAILED，grant_order 明细保留各自结果。
9. 重复 SUCCESS 返回 duplicate=true 或等价标识，reservation 和 quota 只确认一次。
10. 重复 FAILED 返回 duplicate=true 或等价标识，reservation 和 quota 只释放一次。
11. 未知 commandId、错误 tenantId、跨 tenant commandId 不更新任何业务数据，并返回明确错误。
12. SUCCESS 后 FAILED、FAILED 后 SUCCESS 的冲突回调不自动覆盖既有最终状态，并可被对账识别。
13. 缺少 reservation 的 grant_order 回调返回 QUOTA_RESERVATION_MISSING，不猜测 quota 变化。
14. CLOSED grant_order 收到普通回调不重新打开。
15. 对账查询能查到 commandId、activity、subject、grantStatus、participationStatus、reservationStatus、回调信息和 quota 聚合快照。
16. 长期悬挂冻结可通过查询识别：存在 FROZEN reservation 且 grant_order 未进入 SUCCESS / FAILED，超过查询传入时间窗口。
17. callback 处理、quota 确认 / 释放、未知单号、冲突回调、reservation 缺失均有审计或可追踪记录。
```

### 数据一致性验收

```text
1. grant_order 状态、reservation 状态、quota 聚合值、participation 状态必须同成同败。
2. 任意重复回调、并发回调、失败重试不得导致 frozen_value < 0、used_value < 0 或 used_value > total_value。
3. 任意 grant_order 的 reservation 只能终结一次。
4. 任意 SUCCESS / FAILED 回调只能影响该 grant_order 关联的 reservation。
5. 发奖失败后冻结额度不会长期悬挂；要么 RELEASED，要么以异常状态可查询。
6. PostgreSQL 并发语义必须能证明同一 commandId 多个并发回调只生效一次。
```

### 测试验收

```text
1. 覆盖 SUCCESS、FAILED、重复 SUCCESS、重复 FAILED、未知单号。
2. 覆盖跨 tenant commandId 不可更新。
3. 覆盖 SUCCESS / FAILED 冲突回调。
4. 覆盖缺少 reservation 的 grant_order 不自动改 quota。
5. 覆盖多 grant_order participation 全成功、仍有 pending、部分成功失败三种状态推进。
6. PostgreSQL 测试或 smoke 覆盖 reservation 终结与 quota 聚合更新的原子一致性。
7. PostgreSQL 并发测试覆盖同一 commandId 多个重复回调只生效一次。
8. PostgreSQL 并发测试覆盖 SUCCESS 与 FAILED 竞争时只有一个最终态落库，另一方冲突。
9. go test ./... PASS。
10. go build ./... PASS。
```

### 文档验收

```text
docs/api.md 更新 callback 请求、响应、错误码、幂等语义和本期安全边界。
docs/data-model.md 更新 quota reservation / ledger / voucher 的业务口径、状态和与 grant_order 的关联关系。
docs/operations.md 增加排查长期冻结悬挂、未知回调、冲突回调、reservation missing 的查询说明。
docs/roadmap.md 如需保留生产前签名要求，不得因本任务删除该要求。
```

## 待确认问题

以下问题不阻断本轮 Review / Dev，因为本分析已给出本任务内的明确默认业务规则；如 Owner 不接受默认规则，应另行改需求后重新分析。

```text
1. 是否后续需要新增 PARTIAL / NEED_REVIEW participation 状态？
   本任务默认不新增，部分成功 + 部分失败最终 participation.FAILED，grant_order 明细表达部分成功事实。

2. 是否需要迁移历史 used_value 预扣或缺少 reservation 的旧 grant_order？
   本任务默认不迁移，回调时暴露为 QUOTA_RESERVATION_MISSING。

3. DLQ_CLOSED 或人工 close 是否释放预算？
   本任务默认不处理，只有明确 FAILED callback 才释放 reservation。

4. 最小对账查询使用现有 admin 查询扩展还是新增 reconciliation API？
   本任务不限定入口形态，但必须提供机器可读查询能力和文档说明。

5. 长期悬挂冻结的默认时间阈值是多少？
   本任务默认由查询参数传入；如未传，可由运维文档建议 30 分钟或 24 小时，但验收不绑定单一固定值。

6. callback 签名 / token 是否必须本期完成？
   本任务默认不完成；本期能力不得作为公网生产验收依据。
```

## Score

```text
需求清晰度        18/20
范围可控          17/20
风险可控          17/20
验收标准明确      19/20
架构一致性        18/20
总分              89/100
```

## Decision

```text
PASS
```
