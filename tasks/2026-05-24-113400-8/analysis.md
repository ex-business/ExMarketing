# Analysis

## 任务信息

需求 ID：2026-05-24-113400-8

标题：模拟运行接口

风险等级：中

需求来源：

- `tasks/2026-05-24-113400-8/task.md`
- `requirements/backlog.json`
- `docs/api.md`
- `docs/architecture.md`
- `docs/data-model.md`
- `docs/roadmap.md`
- `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md`

## Review 反馈处理

本轮读取了 `tasks/2026-05-24-113400-8/review.md`。

Review 文件存在 `P0`、`P1`、`P2`、`Open Questions`、`Decision` 模板字段，但没有填写具体问题、打回原因或逐条评论；`Decision` 仍为模板 `PASS / BLOCK`，不能识别为明确打回意见。

处理方式：

1. 对模板中的 `P0/P1/P2`：未发现实际条目，本轮无可逐条修复项。
2. 对模板中的 `Open Questions`：内容为“待填写”，未形成问题；本分析仍基于业务文档列出本需求自身的待确认问题。
3. 对模板中的 `Decision`：未给出实际 PASS 或 BLOCK 结论；本轮按返工要求保留本小节，并重新输出完整需求分析。

## 评分规则

满分 100，低于 80 不允许进入研发：

```text
需求清晰度        20
需求边界清楚      20
风险可控          20
验收标准明确      20
约束一致性        20
```

## 背景目标

ExMarketing 是独立活动营销中台，通过标准事件接入外部业务系统，完成活动匹配、规则判断、奖励计算、预算库存控制、参与记录、发奖单和 Outbox 发奖指令编排。正式事件链路会写入 `marketing_event_inbox`、`marketing_participation`、`marketing_grant_order`、`marketing_outbox`，并影响 `marketing_quota`。

本需求要提供一个后台模拟运行能力，用运营或测试提供的事件样例，在不产生真实业务副作用的前提下，预演该事件会命中哪些活动、被哪些活动拒绝、每条规则为何通过或失败、预计奖励是多少、预算/库存/次数会受到什么影响，以及最终会生成怎样的发奖指令预览。

核心目标：

1. 支持输入标准事件样例进行活动模拟。
2. 输出命中活动、未命中活动及原因。
3. 输出每个活动内每条规则的阶段、通过/失败/跳过状态和原因。
4. 输出预计奖励和奖励计算解释。
5. 输出预算、库存、用户次数等 quota 影响预览。
6. 输出 participation、grant_order、outbox reward command 的预览。
7. 严格无副作用：不得写真实事件收件箱、参与记录、发奖单、outbox、quota 扣减或审计类业务结果。

## 参与方

运营 / 活动配置人员：

使用模拟运行验证活动配置、规则条件、奖励配置、预算库存和发奖指令是否符合预期。

测试 / QA：

使用事件样例覆盖命中、拒绝、奖励为 0、quota 不足、多活动、互斥等场景，确认上线前行为可解释。

营销中台后台：

提供模拟运行入口和结果展示，结果必须便于人读和问题定位。

活动匹配与规则引擎：

按正式链路的业务顺序评估活动和规则，并提供解释结果。

奖励计算与 quota 模块：

计算预期奖励，预估预算、库存、用户次数影响；模拟阶段只读当前额度状态，不实际扣减、冻结或确认。

Outbox / 外部发奖系统：

只作为指令预览对象；模拟运行不得实际投递 HTTP、Webhook、MQ 或人工发奖任务。

## 输入

模拟运行入口输入为标准事件样例，字段口径与正式 Event API 保持一致：

```text
eventId
eventType
sourceSystem
tenantId
subject.type
subject.id
biz.bizType
biz.bizId
occurredAt
attributes
```

输入要求：

1. 必须能表达租户、事件类型、来源系统、主体、业务对象、发生时间和业务属性。
2. `attributes` 承载外部业务字段，规则引擎通过 `attributes.*` 路径读取。
3. 若配置了依赖 `facts.*` 的规则，模拟运行需要沿用正式规则对 Fact Provider 的业务约束：未配置或不可用时不能静默通过，必须在规则结果中给出失败或不可评估原因。
4. 金额、预算、库存、奖励相关字段必须按 Decimal 语义处理；输入中的数值不应被解释为二进制浮点。
5. 不要求调用方提供正式 `Idempotency-Key` header；模拟运行可以基于事件样例展示推导出的幂等上下文，但不得写入正式幂等记录。

可选输入能力：

1. 可支持按活动 ID、活动编码或活动状态过滤模拟范围，用于后台单活动调试；若未提供过滤条件，则按当前租户和事件类型评估候选活动。
2. 可支持只评估 DRAFT / SCHEDULED / ACTIVE 等指定状态活动，便于上线前预演；具体允许状态由产品确认。

上述可选能力不是本需求进入研发的硬性前置；最小可验收范围为提交一个标准事件样例并得到完整模拟结果。

## 输出

模拟运行结果必须是结构化结果，至少包含以下信息。

整体结果：

```text
tenantId
event summary
simulation timestamp
matched activities
unmatched activities
summary counts
hasRewardableActivity
hasQuotaRisk
```

每个活动结果：

```text
activityId
activityCode
activityName
activityType
status
version
priority
exclusiveGroup
stackable
decision: MATCHED / REJECTED / SKIPPED / ERROR
decisionReasonCode
decisionReason
ruleResults
rewardPreviews
quotaImpactPreviews
participationPreview
grantOrderPreviews
outboxCommandPreviews
```

规则结果：

```text
stage: MATCH / QUALIFY / REWARD / RISK_LIMIT_BUDGET
storedPhase: MATCH / QUALIFY / REWARD / RISK / LIMIT, if applicable
ruleId
ruleCode
ruleType
sort
enabled
status: PASSED / FAILED / SKIPPED / ERROR
reasonCode
reason
field
expected
actual
```

奖励预览：

```text
rewardCode
rewardType
calculateType
grantMode
grantChannel
amount
currency
skipped
reasonCode
reason
calculation detail
```

quota 影响预览：

```text
quotaType: BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT
scope
scopeKey
currentTotal
currentUsed
currentFrozen, if tracked
availableBefore
delta
wouldExceed
reasonCode
reason
```

发奖链路预览：

```text
participationPreview
grantOrderPreview
outboxCommandPreview
```

预览对象必须能说明正式运行时将写入或生成什么，但必须标识为 preview，不得携带会被误认为已落库成功的真实业务状态。

## 业务规则

### 总体执行顺序

模拟运行应按正式事件处理链路的业务语义执行，但所有持久化和外部投递动作都改为预览：

1. 校验标准事件模型。
2. 根据 tenant、eventType、sourceSystem、活动状态、活动时间、触发事件等条件筛选候选活动。
3. 按活动优先级、互斥组、叠加规则评估活动命中关系。
4. 对每个候选活动按固定业务顺序执行规则：`MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET`。
5. `RISK_LIMIT_BUDGET` 是业务展示阶段，实际规则来源仍是 `RISK` 和 `LIMIT`；`BUDGET` 是 quota 类型，不是合法规则阶段。
6. 规则通过后计算全部 enabled reward。
7. 对有实际奖励的活动预估 BUDGET、STOCK、USER_LIMIT 等 quota 影响。
8. 输出 participation、grant_order、outbox command 预览。
9. 不写入真实数据，不发送发奖指令。

### 活动匹配规则

候选活动至少应受以下条件影响：

1. 租户匹配。
2. `eventType` 命中活动的 `triggerEvents`。
3. 活动状态和活动时间窗口满足配置。
4. 活动 timezone 必须按活动配置解释时间相关规则。
5. 同一互斥组默认只允许命中一个活动；`stackable` 控制是否允许叠加命中。
6. 未命中的活动必须返回未命中原因，而不是只返回命中活动列表。

### 规则解释规则

1. 每条规则必须输出通过、失败、跳过或错误状态。
2. 禁用规则应输出 `SKIPPED`，不得影响最终决策。
3. 字段缺失、类型不匹配、非法操作符、未注册 evaluator、Fact Provider 不可用，不能默认通过。
4. 规则解释必须包含阶段、规则编码、字段、期望值、实际值和原因信息，便于运营定位配置问题。
5. 一个活动在早期阶段失败时，后续阶段是否继续评估用于解释，可由实现选择；但输出必须能明确该活动最终为何未命中，且不能把未执行规则误报为通过。

### 奖励计算规则

1. 支持当前普通活动可配置的奖励类型：`BONUS`、`CASH`、`POINT`。
2. 支持当前文档已定义的计算方式：`FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT`。
3. 金额类奖励必须使用 Decimal 语义，输出金额保留 4 位小数。
4. `RATIO` 需要展示基础字段、基础金额、比例、封顶值和是否封顶。
5. `LADDER` 需要展示命中的阶梯。
6. `RANDOM` / `PROBABILITY` 需要使用稳定 seed 语义，模拟同一事件样例和同一活动版本时结果应稳定可复现。
7. `POOL_SPLIT` 只表达事件即时分摊预览，不表达活动结束后整池结算。
8. 奖励结果为 `0.0000` 时，按无实际奖励处理：不预览 grant_order/outbox/quota 扣减，活动结果应解释为已通过但无可发奖励。

### quota 影响规则

模拟运行必须展示正式运行时会影响的 quota 项，但不得实际扣减、冻结或确认。

当前支持的 quota 影响：

1. `BUDGET + ACTIVITY + TOTAL`：扣减金额为同一 activity 本次所有实际 reward amount 合计。
2. `STOCK + ACTIVITY + rewardCode`：每个 enabled 且实际发放的 reward 默认扣减 1。
3. `USER_LIMIT + USER + subject.id`：每个 rewardable participation 默认扣减 1。
4. `DAILY_LIMIT`：模型已定义；若当前业务流程未启用，应在输出中说明未参与本次影响或不适用。

quota 预览规则：

1. 缺少对应 quota 行表示不限制，输出应标识为 `unlimited` 或等价语义。
2. 当前剩余额度不足时，模拟结果应显示 `wouldExceed=true`，并给出 `QUOTA_EXCEEDED` 风格的原因。
3. 多奖励活动中任一 quota 预览不足，应显示正式运行会整体失败，不得表现为部分奖励可发。
4. 模拟运行不能改变 `used_value`、`frozen_value`、`version` 或任何 reservation 状态。

### 发奖链路预览规则

1. participation 预览应包含活动、事件、规则结果、奖励摘要和必要幂等上下文。
2. 规则拒绝时可预览 REJECTED participation 语义，但不得真实写入。
3. 规则通过且存在实际奖励时，可预览 REWARDABLE participation、grant_order 和 outbox command。
4. 规则通过但全部奖励为 0 时，只预览 QUALIFIED / zero reward skipped 语义，不预览 grant_order/outbox。
5. grant_order 预览必须能说明 rewardCode、rewardType、amount、currency、grantChannel、targetRef、payload 摘要和 command 标识语义。
6. outbox command 预览必须能说明目标通道、目标地址、消息类型和 payload 摘要。
7. 模拟运行不得绕过 grant_order 直接预览外部发钱动作；预览链路仍应体现 participation -> grant_order -> outbox 的正式顺序。

## 边界条件

1. 无活动匹配：返回空命中列表，并输出候选活动为空或每个未命中原因。
2. 多活动匹配：按活动优先级、互斥组和 stackable 规则输出最终命中与被互斥拒绝的活动。
3. 活动未到开始时间、已结束、暂停或租户不匹配：输出明确未命中原因。
4. 事件字段缺失或未知字段：返回事件校验错误，不进入活动模拟。
5. 规则字段缺失：规则失败并记录字段缺失原因。
6. Fact Query 失败：依赖该事实源的规则失败或错误，不默认通过。
7. 奖励配置非法：对应活动输出错误结果，不生成发奖预览。
8. 奖励为 0：不生成 grant_order/outbox/quota 影响预览。
9. quota 不足：输出会超限原因和影响项，不写审计、不扣额度。
10. 缺少 quota 配置：按无限制处理，并在 quota 预览中说明。
11. 预览中涉及随机或概率奖励：同样输入和同样配置版本必须稳定复现，避免运营每次点击看到不同结果。
12. 并发模拟：多个模拟请求之间不得互相影响，也不得影响正式事件链路。

## 异常场景

非法事件：

返回 `INVALID_EVENT` 或等价错误；不得输出误导性的命中结果，不写任何业务记录。

活动配置不可解析：

该活动结果为 `ERROR`，原因指向配置解析问题；其他活动可继续模拟，除非整体输入无法处理。

规则执行异常：

该规则输出 `ERROR`，该活动不得被判定为通过；错误原因需面向配置排查，而不是暴露内部栈信息。

奖励计算异常：

该奖励输出 `ERROR`，对应活动不得预览 grant_order/outbox；多奖励活动不得显示部分正式可发。

quota 查询异常：

输出 quota 影响无法确认，活动不得被展示为确定可发；需要明确风险原因。

外部 Fact Provider 异常：

依赖该 facts 字段的规则失败或错误，结果中展示 provider 不可用、超时或字段缺失原因。

targetRef 非法：

如果活动配置允许进入模拟，结果必须显示发奖指令无法形成；若创建活动阶段已阻断该配置，则模拟中一般不会出现。

## 非目标范围

1. 不实现真实事件上报链路。
2. 不写 `marketing_event_inbox`。
3. 不写真实 `marketing_participation`。
4. 不写真实 `marketing_grant_order`。
5. 不写真实 `marketing_outbox`。
6. 不扣减、不冻结、不确认、不释放 `marketing_quota`。
7. 不发送 HTTP、Webhook、MQ 或人工发奖指令。
8. 不实现 Outbox 重试、DLQ、回放或关闭。
9. 不实现发奖回调后的预算确认 / 释放。
10. 不新增业务专用活动逻辑、专用 controller、专用 job 或专用发奖流程。
11. 不改变既有规则引擎、奖励计算器、quota、grant_order、outbox 的正式业务口径。
12. 不把模拟运行结果当作审批流或上线校验的唯一依据；上线前校验已有独立需求。

## 风险

无副作用风险：

模拟接口如果复用正式链路时隔离不彻底，可能误写 inbox、participation、grant_order、outbox 或扣减 quota，这是本需求最高风险。

解释一致性风险：

如果模拟逻辑与正式事件处理逻辑不一致，会导致运营上线前验证结果和线上实际行为不一致。

账务展示风险：

预算、库存、用户次数只能做预估，不得误导用户认为额度已锁定；并发正式请求可能在模拟后改变真实剩余额度。

随机奖励风险：

随机或概率奖励如果模拟不可复现，会降低排查价值；如果 seed 语义和正式链路不同，会造成预览与实际不一致。

多活动互斥风险：

只展示命中活动、不展示被互斥或未命中原因，会掩盖配置冲突。

Fact Provider 风险：

模拟运行可能触发外部 fact 查询，需避免不可控副作用；如果 Provider 不可用，结果必须明确不可评估，而不是默认通过。

范围膨胀风险：

本需求容易扩展到活动审批、上线前校验报告、配置编辑器、对账查询等后台能力；这些应保持在独立需求中。

## 验收标准

功能验收：

1. 提交合法标准事件样例后，接口返回结构化模拟结果。
2. 结果包含命中活动列表和未命中活动列表。
3. 每个未命中活动有明确原因。
4. 每个候选活动输出规则执行结果，覆盖 `MATCH -> QUALIFY -> REWARD -> RISK_LIMIT_BUDGET` 业务阶段。
5. 每条规则输出通过、失败、跳过或错误状态，以及 reasonCode、reason、field、expected、actual 等解释信息。
6. 规则字段缺失、类型不匹配、非法操作符或未注册 evaluator 不会默认通过。
7. 规则通过时输出预计奖励，金额类奖励按 Decimal 语义保留 4 位小数。
8. `FIXED`、`RATIO`、`LADDER`、`RANDOM`、`PROBABILITY`、`POOL_SPLIT` 奖励至少能输出对应的计算解释或明确不适用原因。
9. 奖励为 `0.0000` 时，不输出 grant_order/outbox/quota 扣减预览，并说明 zero reward skipped。
10. 输出 BUDGET、STOCK、USER_LIMIT 的影响预览，包括当前额度、预计影响、是否超限和原因。
11. quota 缺失时按无限制展示，不阻断模拟结果。
12. quota 不足时输出 would exceed 结果，不生成可发奖成功的误导性预览。
13. 规则通过且存在实际奖励时，输出 participation、grant_order、outbox command 预览。
14. 多奖励活动中任一奖励或 quota 不可发时，结果说明正式运行不会部分发奖。
15. 同一事件样例、同一活动版本、同一随机配置重复模拟时，随机 / 概率奖励预览稳定。

无副作用验收：

1. 模拟运行后 `marketing_event_inbox` 无新增真实记录。
2. 模拟运行后 `marketing_participation` 无新增真实记录。
3. 模拟运行后 `marketing_grant_order` 无新增真实记录。
4. 模拟运行后 `marketing_outbox` 无新增真实记录。
5. 模拟运行后 `marketing_quota.used_value`、`frozen_value`、`version` 不变化。
6. 模拟运行不会触发 Outbox worker，也不会向外部 reward targetRef 发送发奖请求。
7. 模拟运行不会把结果写成正式审计、回调或 DLQ 状态。

异常验收：

1. 非法事件返回明确错误，且无任何业务副作用。
2. 活动配置错误、规则执行错误、奖励计算错误、quota 查询错误均能在结果中定位到活动或规则，不返回空白失败。
3. Fact Provider 不可用时，依赖 facts 的规则不通过或错误，并输出原因。
4. 活动互斥、stackable、priority 场景能展示最终命中和被排除原因。

文档验收：

1. API 文档说明模拟运行输入、输出和无副作用保证。
2. 文档明确模拟结果只是预览，不代表 quota 已锁定或奖励已发放。
3. 文档说明模拟运行与正式链路在规则、奖励、quota 业务语义上保持一致。

## 建议测试场景

1. Happy path：首存事件命中活动，规则全通过，产生奖励、quota 影响和发奖指令预览。
2. No activity：事件类型无候选活动，返回无命中和原因。
3. Rule rejected：活动候选命中但 QUALIFY 或 REWARD 规则失败，返回规则失败原因。
4. Disabled rule：禁用规则显示 skipped，不影响决策。
5. Missing field：规则依赖的 `attributes.amount` 缺失，规则失败。
6. Zero reward：奖励计算结果为 0，不预览 grant_order/outbox/quota 扣减。
7. Budget exceeded：预算不足，返回 wouldExceed，不写任何记录。
8. Stock exceeded：某 reward 库存不足，多奖励活动整体不可发。
9. User limit exceeded：用户次数不足，不展示为可发奖。
10. Unlimited quota：缺少 quota 行时展示无限制。
11. Mutual exclusive：同组多个活动满足条件，只命中优先级最高或符合叠加配置的活动，并解释其他活动被排除原因。
12. Stable random：随机/概率奖励重复模拟结果一致。
13. Fact provider failure：facts 依赖规则因 provider 不可用而失败。
14. Side-effect guard：模拟前后检查 inbox、participation、grant_order、outbox、quota 均无变化。

## 待确认问题

1. 模拟运行是否需要支持按活动 ID / 活动编码限定模拟范围？建议支持，但不作为最小验收前置。
2. 模拟运行是否允许评估 `DRAFT` 和 `SCHEDULED` 活动？上线前预演需要该能力，但需要产品确认默认活动状态范围。
3. 未命中活动列表是否返回全部候选活动，还是只返回因 eventType、tenant、时间、互斥被排除的活动？建议至少返回本租户同 eventType 候选活动。
4. Fact Provider 在模拟运行中是否允许真实发起外部查询？如果不允许，需要支持由输入直接提供 facts 样例。
5. 模拟运行是否需要写一条后台操作审计？当前需求强调不写真实参与、grant_order、outbox；若要求审计，需要确认审计是否属于允许的非业务副作用。
6. 随机/概率奖励的 seed 是否完全使用正式链路的幂等键推导，还是允许模拟请求显式传 seed？建议默认使用正式推导，可选 seed 需另行确认。

上述问题不阻塞最小需求进入研发；只要实现标准事件输入、完整解释输出和严格业务无副作用，即可满足本需求核心验收。

## Score

```text
92
```

评分明细：

```text
需求清晰度        18/20
需求边界清楚      19/20
风险可控          18/20
验收标准明确      19/20
约束一致性        18/20
```

扣分原因：

1. 是否默认评估 DRAFT / SCHEDULED 活动仍需产品确认。
2. Fact Provider 在模拟运行中是否允许真实调用外部服务仍需确认。
3. 是否写后台操作审计需进一步明确。

## Decision

```text
PASS
```

理由：本轮已把模拟运行接口的输入、输出、业务规则、无副作用边界、异常场景和验收标准细化到可审核、可开发、可测试的程度。剩余待确认问题不影响最小可验收范围，且均已标注为非阻塞或需产品进一步确认的增强口径。

需求：2026-05-24-113400-8
