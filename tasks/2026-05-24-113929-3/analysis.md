# Analysis

## 需求 ID

2026-05-24-113929-3

## 标题

固定 / 比例 / 阶梯奖励计算

## Review 反馈处理

本轮已读取 `tasks/2026-05-24-113929-3/review.md`，逐条处理如下。

### P0

无。

### P1：LADDER 未命中口径与现有实现 / 测试冲突

Review 指出上一版 `analysis.md` 将 `LADDER` 字段值未命中任何阶梯定义为输出 `0.0000`，但 `task.md` 只要求实现或固化 `FIXED` / `RATIO` / `LADDER` 并补充固定金额、充值比例赠送、连续签到阶梯奖励单测，没有提出要变更 LADDER 未命中的错误语义。Review 同时指出现有行为和测试已将该场景锁定为明确计算失败 `ReasonLadderNotMatched`。

处理方式：

- 本轮关闭上一版错误假设，不再要求 `LADDER` 未命中输出 `0.0000`。
- 本任务默认最小可验收口径改为：`LADDER` 运行时字段存在且可解析，但未命中任何阶梯时，应返回明确计算失败原因，建议沿用 `ReasonLadderNotMatched` 语义；不得把该场景按 `ZERO_REWARD_SKIPPED` 静默跳过。
- 验收标准中移除“未命中任何阶梯输出 0.0000”的要求，改为“未命中任何阶梯返回明确失败原因且不生成实际发奖”。
- 该处理依据是：`task.md` 没有提出改变既有错误语义；为避免扩大需求和破坏回归，本轮采用保持既有错误语义的默认口径。

### P2：非目标范围对 RANDOM / PROBABILITY / POOL_SPLIT 表述不准确

Review 指出上一版将 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 写成“实现或重构不包含”，容易被误解为要删除或禁用既有能力；而 `docs/data-model.md` 已声明这些 `calculateType` 当前已实现。

处理方式：

- 本轮非目标范围改为：本任务不新增、不重构、不改变 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 的既有能力、配置语义、稳定 seed、分摊语义或测试期望。
- 明确 Dev 不应因为本需求删除、禁用或弱化这些既有 `calculateType`。

### Open Questions：LADDER 未命中最终口径

Review 提出两个选项：

- A. 返回明确计算失败 `ReasonLadderNotMatched`，保持现有实现；
- B. 输出 `0.0000` 并按 `ZERO_REWARD_SKIPPED` 处理。

本轮不等待 Owner 决策，按最小变更原则关闭该问题：默认采用 A。理由是 `task.md` 未要求语义变更，且 Review 已说明 B 会扩大需求并诱导回归。

## Analysis 自检修正

本轮已读取上一版 `analysis.md`。上一版存在以下自检失败项和错误假设：

- 错误假设 1：认为 `review.md` 没有实质反馈。修正：本轮重新读取并逐条回应 P1、P2、Open Questions。
- 错误假设 2：将 `LADDER` 未命中解释为 `0.0000` 且“不阻断”。修正：改为明确计算失败，保持既有错误语义，不纳入 0 奖励验收。
- 扣分项 1：Score 虽写为 90，但 Review 因 P1 将其降为 75 并 BLOCK。修正：删除扩大需求的 LADDER 口径，补充不生成发奖的失败场景验收。
- 扣分项 2：非目标范围对 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 容易产生误导。修正：改为“不新增、不重构、不改变既有能力”，并明确不得删除或禁用。
- 待确认问题 1：`LADDER` 未命中是否失败还是 0 奖励。修正：本任务默认按明确失败处理，旧待确认问题关闭。

本轮继续基于 `task.md`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/data-model.md`、`docs/api.md`、`docs/roadmap.md` 和来源设计文档 `/Users/tong/Documents/GitHub/OracleBetX-Projects/act-analysis/22-独立活动营销中台设计.md` 细化需求；不读取代码、不写代码、不设计具体代码改法。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、判断活动配置、生成参与记录、发奖单和发奖指令，不直接修改用户余额，也不依赖外部业务数据库。

本需求来自“22-独立活动营销中台设计”。目标是补齐或固化三类基础奖励计算能力，使普通配置化活动能通过统一奖励计算链路完成发奖金额计算：

- `FIXED`：固定金额奖励。
- `RATIO`：按事件或上下文字段金额乘比例计算奖励，并支持封顶。
- `LADDER`：按某个数值字段命中阶梯闭区间，返回对应奖励金额，典型场景为连续签到天数阶梯奖励。

该能力服务于 Phase 2 的低风险活动接入，覆盖首存、续存、每月充值、签到有礼等普通配置化活动。需求必须沿用现有标准事件、规则、奖励、participation、quota、grant_order、outbox 链路，不新增业务专用 controller，不绕过统一发奖链路。

## 参与方

- 活动配置方：通过 Admin 创建活动、规则和奖励配置。
- 事件接入方：通过 OpenAPI 或 Webhook 上报标准事件，事件业务字段进入 `attributes`。
- ExMarketing 引擎：在活动命中、规则通过后执行奖励计算，并把结果交给 participation、quota、grant_order、outbox 链路。
- 外部发奖系统：接收 ExMarketing 生成的标准发奖指令并实际发奖。
- 测试 / 审核 Agent：验证三类计算口径、异常输入、兼容性和回归风险。

## 输入

### 公共输入

- 活动奖励配置：
  - `rewardType`：本任务默认覆盖金额类 `BONUS` / `CASH`。
  - `calculateType`：本任务关注 `FIXED` / `RATIO` / `LADDER`。
  - `rewardConfig`：计算参数。
  - `grantMode`：当前可执行口径为 `AUTO`。
  - `grantChannel`：当前可执行口径为 `HTTP` / `WEBHOOK`。
- 标准事件：
  - 顶层字段：`eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject`、`biz`、`occurredAt`。
  - 业务字段：通过 `attributes.*` 承载。
- 运行上下文：
  - 可读取标准事件字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。

### FIXED 输入

`rewardConfig` 最小字段：

```json
{
  "amount": "18.00",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `amount` 必填，必须是可解析 Decimal 的非负金额字符串或数字。
- `currency` 对金额类 `BONUS` / `CASH` 必填。
- `targetRef` 对 `AUTO + HTTP/WEBHOOK` 必填，必须是 `http` 或 `https` URL。

### RATIO 输入

`rewardConfig` 最小字段：

```json
{
  "baseField": "attributes.amount",
  "rate": "0.20",
  "maxAmount": "500.00",
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `baseField` 必填，指向允许字段路径中的数值字段。
- `rate` 必填，必须是可解析 Decimal，且 `rate >= 0`。
- `maxAmount` 可选；如传入，必须是可解析 Decimal，且 `maxAmount >= 0`。
- `baseField` 对应运行时值必须可解析 Decimal；用于金额计算时默认要求 `base >= 0`。

### LADDER 输入

`rewardConfig` 最小字段：

```json
{
  "field": "attributes.continueDays",
  "ladders": [
    {"min": 1, "max": 2, "amount": "5.00"},
    {"min": 3, "max": 6, "amount": "18.00"},
    {"min": 7, "max": 999, "amount": "58.00"}
  ],
  "currency": "CNY",
  "targetRef": "https://wallet.example.com/reward"
}
```

默认口径：

- `field` 必填，指向允许字段路径中的数值字段。
- `ladders` 必填，必须是非空数组。
- 每个阶梯必须包含 `min`、`max`、`amount`。
- `min`、`max` 必须可解析为 Decimal 或整数数值，且 `min <= max`。
- `amount` 必须是可解析 Decimal 的非负金额。
- 阶梯区间为闭区间 `[min, max]`。
- 同一奖励配置内阶梯不得重叠。
- 阶梯可以不连续；但运行时字段落在未覆盖空档时，本任务默认返回明确计算失败，不按 0 奖励静默跳过。

## 输出

奖励计算成功后输出标准奖励结果，供 participation 快照、grant_order 和 outbox 使用：

- `rewardType`：沿用配置，如 `BONUS` / `CASH`。
- `calculateType`：`FIXED` / `RATIO` / `LADDER`。
- `rewardCode`：沿用奖励配置。
- `amount`：Decimal 金额，统一保留 4 位小数。
- `currency`：金额类奖励必填并透传。
- `flowMultiple`：按现有奖励配置口径透传；本任务不改变流水倍数规则。
- `rewardConfig` 摘要：用于审计、模拟和对账解释，不应包含未定义业务字段。

当金额结果为 `0.0000` 且无其他实际权益时，按现有文档口径不生成 grant_order、outbox，也不扣减 quota。该 `0.0000` 口径只适用于成功计算出的 0 金额，例如 `FIXED amount=0` 或 `RATIO rate=0`；不适用于 `LADDER` 未命中阶梯的计算失败场景。

## 业务规则

### 通用规则

- 金额计算必须使用 Decimal 语义，禁止使用 `float64` 做金额、比例、封顶、阶梯金额计算。
- 输出金额统一保留 4 位小数。
- 计算器只负责奖励金额计算，不负责绕过规则、quota、participation、grant_order、outbox 或 callback。
- 奖励只在活动命中且规则通过后计算。
- 配置校验应在创建活动或上线前发现明显错误，避免运行时才暴露非法配置。
- 字段路径必须遵守现有规则边界：标准事件顶层字段、`attributes.*`、`facts.<factCode>.*`、`state.activity.*`、`state.subject.*`、`state.quota.*`、`state.exclusive.*`、绑定插件时的 `state.plugin.<pluginCode>.*`。
- 非标准顶层业务字段，例如 `orderAmount`、`deposit.amount`、`user.vipLevel`，不得作为 `baseField` 或 `field` 直接读取。

### FIXED 规则

- 奖励金额直接等于 `rewardConfig.amount`。
- 示例：`amount = 18.00`，输出 `18.0000`。
- `amount = 0` 是合法配置，成功计算为 `0.0000`；运行时按无实际金额奖励处理，不生成实际发奖。

### RATIO 规则

- 计算公式：`rewardAmount = baseFieldValue * rate`。
- 如配置 `maxAmount`，最终金额为 `min(rewardAmount, maxAmount)`。
- 示例：充值 `attributes.amount = 299.00`，`rate = 0.20`，未封顶时输出 `59.8000`。
- 示例：充值 `attributes.amount = 3000.00`，`rate = 0.20`，`maxAmount = 500.00`，原始计算 `600.0000`，封顶后输出 `500.0000`。
- `rate = 0` 合法，成功计算为 `0.0000`，按无实际金额奖励处理。
- `baseField` 缺失、为空、布尔、对象、数组或不可解析 Decimal 时，必须返回明确计算失败，不得默认按 0 计算。

### LADDER 规则

- 从 `field` 读取运行时数值，按闭区间查找唯一命中阶梯。
- 命中后输出该阶梯的 `amount`。
- 示例：`attributes.continueDays = 1` 命中 `[1,2]`，输出 `5.0000`。
- 示例：`attributes.continueDays = 3` 命中 `[3,6]`，输出 `18.0000`。
- 示例：`attributes.continueDays = 7` 命中 `[7,999]`，输出 `58.0000`。
- `field` 缺失、为空、布尔、对象、数组或不可解析 Decimal 时，必须返回明确计算失败。
- `field` 可解析但未命中任何阶梯时，必须返回明确计算失败，默认失败原因沿用 `ReasonLadderNotMatched` 语义；不输出 `0.0000`，不按 `ZERO_REWARD_SKIPPED` 静默跳过。
- 阶梯顺序不应影响命中结果；但配置保存或上线前校验应拒绝重叠区间，避免同一输入命中多个金额。

## 边界条件

- Decimal 输入允许字符串或 JSON number；金额、比例、封顶推荐字符串，以避免上游 JSON number 精度歧义。
- 金额输出固定 4 位小数，不因输入小数位不同而改变输出格式。
- 负数金额、负数比例、负数封顶、负数阶梯奖励默认不允许。
- `maxAmount = 0` 合法，表示比例奖励封顶后可能成功计算为 `0.0000`。
- `LADDER min = max` 合法，表示单点闭区间。
- `LADDER` 阶梯可不连续；未覆盖空档不再解释为 0 奖励，而是计算失败。
- 本任务不改变活动状态、互斥组、quota、幂等、outbox retry、DLQ、callback 等既有业务语义。

## 异常场景

- 未知 `calculateType`：拒绝配置或返回明确错误，不得回退到固定金额。
- `FIXED` 缺少 `amount` 或金额非法：拒绝配置或返回明确配置错误。
- `RATIO` 缺少 `baseField` / `rate` 或 Decimal 非法：拒绝配置或返回明确配置错误。
- `RATIO` 运行时字段不存在或不可解析：本次事件该奖励计算失败，应能在模拟或处理结果中看到明确原因。
- `LADDER` 缺少 `field` / `ladders`，阶梯为空，`min > max`，区间重叠，`amount` 非法：拒绝配置或返回明确配置错误。
- `LADDER` 运行时字段不存在、不可解析或未命中任何阶梯：本次事件该奖励计算失败，不生成 grant_order/outbox，不扣 quota。
- `targetRef` 缺失或 URL 非法：对 `AUTO + HTTP/WEBHOOK` 拒绝配置。
- `currency` 缺失：对金额类 `BONUS` / `CASH` 拒绝配置。

## 非目标范围

本任务不包含：

- 新增外部业务数据库连接或查询。
- 新增业务专用 controller、业务专用发奖逻辑或绕过标准事件链路的能力。
- 修改钱包余额或直接完成真实发奖。
- 新增、重构、删除、禁用或改变 `RANDOM`、`PROBABILITY`、`POOL_SPLIT` 的既有能力；这些类型保持现有语义和兼容性。
- 扩展非金额类奖励的计算规则；非金额类当前最小可上线口径继续按既有文档处理。
- 新增签到插件真实状态表或签到状态写入逻辑；连续签到天数只作为标准事件 `attributes.continueDays` 或既有允许状态路径中的运行时字段被 `LADDER` 读取。
- 改造 quota、budget、inventory、outbox、callback 状态机。
- 改造 Admin UI 交互；如现有 API 和测试可覆盖，本任务无需前端页面变化。

## 风险

- 金额精度风险：比例和封顶如果使用浮点会产生资金误差，必须按 Decimal 口径验收。
- 配置风险：阶梯区间重叠会导致同一输入命中多个金额，必须在配置层拒绝。
- 未命中语义风险：将 `LADDER` 未命中改成 0 奖励会扩大需求并破坏既有错误语义；本轮已明确保持失败语义。
- 字段路径风险：`baseField` / `field` 如果允许非标准顶层业务字段，会破坏 ExMarketing 与业务系统解耦。
- 兼容风险：已有活动可能依赖现有奖励类型和错误语义，新增校验或文档口径不能误伤文档允许的合法配置。
- 解释性风险：模拟和快照如果缺少计算摘要，配置方难以判断比例封顶、阶梯命中或未命中失败原因。

## 验收标准

### 单元测试必须覆盖

- `FIXED` 固定金额：
  - `amount = 18.00` 输出 `18.0000`。
  - `amount = 0` 成功输出 `0.0000`，且不应产生实际发奖语义。
- `RATIO` 充值比例赠送：
  - `attributes.amount = 299.00`，`rate = 0.20`，无封顶或封顶足够高时输出 `59.8000`。
  - `attributes.amount = 3000.00`，`rate = 0.20`，`maxAmount = 500.00` 时输出 `500.0000`。
  - `baseField` 缺失或不可解析 Decimal 时返回明确失败。
- `LADDER` 连续签到阶梯奖励：
  - `continueDays = 1` 输出 `5.0000`。
  - `continueDays = 3` 输出 `18.0000`。
  - `continueDays = 7` 输出 `58.0000`。
  - `field` 缺失或不可解析 Decimal 时返回明确失败。
  - 未命中任何阶梯时返回明确失败原因，默认沿用 `ReasonLadderNotMatched` 语义；不得输出 `0.0000`。
  - 重叠阶梯配置被拒绝。

### 回归验收

- `go test ./...` 必须通过。
- `go build ./...` 必须通过。
- 创建活动或模拟运行不得要求新增业务专用字段；示例仍使用标准事件和 `attributes` / `state` 路径。
- 奖励计算结果进入现有 grant_order / outbox 链路，不新增直接发奖旁路。
- 金额输出保持 4 位小数。
- `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 既有能力不得因本任务被删除、禁用或改变语义。

## 待确认问题

以下问题不阻断本任务，已按最小可验收口径处理：

- `LADDER` 未命中是否应视为失败还是 0 奖励：本轮默认视为明确计算失败，沿用 `ReasonLadderNotMatched` 语义；若未来产品要改为 0 奖励，应另立需求并同步更新错误语义、测试和文档。
- `baseField` / `field` 是否允许 JSON number：本轮默认允许，但推荐字符串；金额计算必须进入 Decimal，不允许浮点语义。
- 本任务是否覆盖非金额奖励：本轮默认不覆盖，因为 `task.md` 验收只要求固定金额、充值比例赠送、连续签到阶梯奖励。

## Score

```text
92
```

评分说明：

- 需求清晰度：19 / 20，三类计算输入输出和失败语义明确，已关闭 LADDER 未命中歧义。
- 范围可控：19 / 20，限定在 `FIXED` / `RATIO` / `LADDER` 金额计算，不改变随机、概率、奖池瓜分既有能力。
- 风险可控：18 / 20，主要风险为 Decimal 精度、阶梯重叠、字段路径和未命中语义，已有明确约束。
- 验收标准明确：18 / 20，覆盖任务要求的三类单测、异常单测和回归命令。
- 架构一致性：18 / 20，遵守 ExMarketing 标准事件、奖励计算、participation、grant_order、outbox 边界。

## Decision

```text
PASS
```

需求细节已达到可审核、可验收标准，可以进入 Review / Dev 阶段。
