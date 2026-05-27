# Test Plan

## Agent 指令

你是后续 Test Agent。`review.md` 的 Decision 已确认为 `PASS`，本文件只定义后续应执行的测试用例和命令清单。

本轮 Test Plan Agent 未执行测试、构建、格式化或 git 命令；未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Scope

验证 `FORMULA` 最小可运行高级规则执行器，以及未开放能力的安全拒绝：

- `FORMULA` evaluator 已注册后，合法 Decimal / Boolean 公式可保存、预发布校验、模拟运行和正式运行。
- `FORMULA` 表达式只允许确定性、无副作用、字段路径受控的最小语法。
- `SCRIPT` 本期默认拒绝 inline code 和未审核脚本。
- 未注册高级 `LIMIT` evaluator 时继续拒绝，不与既有 quota / budget / stock / user limit / daily limit 混淆。
- 规则拒绝、执行错误、额度不足、幂等重复、并发和 PostgreSQL 持久化一致性不得退化。

## Test Strategy

1. 单元测试优先覆盖 `internal/rule`：表达式解析、AST/语义白名单、字段提取、类型检查、Decimal 计算、compare 语义和结构化错误。
2. Service / Admin API 测试覆盖活动创建、更新或 prelaunch validation：合法 `FORMULA` 可保存；不安全、非法、未注册 evaluator 的规则返回 400 类结构化错误。
3. Engine / HTTP 测试覆盖模拟运行和正式事件处理：公式结果影响活动决策，失败或错误不得默认通过，不生成错误 grant/outbox，不扣 quota。
4. 回归测试覆盖既有 `CONDITION`、规则 phase/sort 顺序、enabled=false、Fact Provider / plugin 字段边界、quota 错误码和幂等语义。
5. PostgreSQL smoke 使用真实 PG 验证规则 JSON 落库读取、非法配置事务回滚、幂等唯一约束、并发 quota 原子性和历史非法规则防御。

## Test Cases

### A. 正常路径

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| A1 | Decimal 公式通过 | 创建活动，`ruleType=FORMULA`，`formula="attributes.amount * 0.2"`，`resultType=DECIMAL`，`compare <= "500.00"`；事件 `attributes.amount=1000.00` | 保存和 prelaunch 通过；模拟/正式运行规则 `PASSED`；`actual=200.0000` 或等价 Decimal 输出；活动继续后续发奖链路 |
| A2 | Decimal 公式拒绝 | 同 A1，但事件 `attributes.amount=3000.00` | 规则 `REJECTED`；`actual=600.0000`；活动不生成成功 grant_order/outbox，不扣 quota |
| A3 | Decimal 精度 | `formula="attributes.amount * 0.1 + 0.2"`，输入能暴露 float 漂移的金额 | 使用 Decimal 语义比较，不能出现二进制 float 误差导致误判 |
| A4 | 括号和一元负号 | `formula="(attributes.deposit - attributes.fee) * -1"` 搭配合法比较 | 按算术优先级计算；结构化输出 expected/actual |
| A5 | Boolean 公式缺省 compare 通过 | `formula="attributes.isFirstDeposit"`，`resultType=BOOLEAN`，不传 `compare`；事件字段为 true | 保存通过；运行时 `PASSED` |
| A6 | Boolean 公式缺省 compare 拒绝 | 同 A5，但事件字段为 false | 运行时 `REJECTED`，不得默认发奖 |
| A7 | 标准字段引用 | `formula="tenantId"` 或 `formula="eventType"`，`resultType=STRING` 若实现则等值比较；若未实现 STRING evaluator 能力 | 已实现时通过等值测试；未实现时必须结构化拒绝，不能默认通过 |
| A8 | enabled=false 合法公式 | 合法 `FORMULA` 但 `enabled=false` | 保存和 prelaunch 通过；运行结果为 `SKIPPED` 或不参与决策；不影响活动通过 |
| A9 | 多规则排序 | 同一 phase 配置 `CONDITION` 与 `FORMULA`，sort 为 10/20 | 按 sort 升序执行和输出解释；已有 `CONDITION` 行为不变 |
| A10 | 模拟运行无副作用 | 对含合法 `FORMULA` 的活动执行 simulate | 输出公式计算值、expected、actual、reasonCode；不写 inbox/participation/grant_order/outbox，不扣 quota |

### B. FORMULA 配置与安全拒绝

| ID | 场景 | 输入 | 预期 |
|---|---|---|---|
| B1 | 空公式 | `formula=""` 或全空白 | 400；错误定位到 `rules[index].ruleConfig.formula` |
| B2 | ruleConfig 类型非法 | `ruleConfig=null`、数组、字符串、数字、布尔 | 400；不得保存 |
| B3 | 未知顶层字段 | `ruleConfig` 多传 `script`、`endpoint`、`sql`、`extra` | 400；未知字段不得静默忽略 |
| B4 | 非法字段路径 | `orderAmount`、`deposit.amount`、`user.vipLevel` | 400；提示必须走标准字段或 `attributes.*` 等白名单 |
| B5 | 非法 facts provider | 引用 `facts.USER_PROFILE.vipLevel` 但 provider 未启用或跨租户 | create/update/prelaunch 或运行时结构化失败，不得默认通过 |
| B6 | plugin 越权 | 引用未绑定或未注册插件的 `state.plugin.X.value` | 400 或 prelaunch 失败 |
| B7 | 函数调用 | `max(attributes.amount, 1)`、`http("...")`、`sql("...")`、`now()`、`random()` | 400；reasonCode 为不安全表达式或等价稳定码 |
| B8 | 赋值/分支/循环 | `a=1`、`if (...)`、`for (...)`、`while (...)`、三元条件 | 400；不得作为脚本执行 |
| B9 | 文件/环境/secret | `env.SECRET`、`readFile(...)`、`process.env.X` | 400 |
| B10 | 比较符非法 | `compare.op="LIKE"`、`BETWEEN` | 400；错误定位到 compare op |
| B11 | compare 缺失但非 Boolean | `resultType=DECIMAL` 且不传 `compare` | 400；缺省 compare 只允许 Boolean |
| B12 | IN/NOT_IN value 非法 | value 非数组、空数组、数组含 object/array/null、超过 100 项 | 400 |
| B13 | 数值比较用于非数值 | Boolean/String 结果使用 `>`、`>=`、`<`、`<=` | 400 |
| B14 | scale 边界 | `scale=-1`、小数、字符串、超过平台上限；以及合法 `0..8` | 非法值 400；合法值保存并按 scale 输出 |
| B15 | 长度/字段数/括号深度 | 表达式超过 1024 字符、字段引用超过 20、括号深度超过 8 | 400；避免复杂表达式拖垮运行时 |
| B16 | enabled=false 不绕过安全 | `enabled=false` 但公式含 `http()` 或非法字段 | 400；禁用规则也必须安全校验 |

### C. 执行期错误

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| C1 | 字段缺失 | 合法保存公式引用 `attributes.amount`，运行事件缺少该字段 | 规则 `ERROR` 或活动拒绝；不得按 0、空字符串或 false 通过 |
| C2 | null 值 | 公式字段值为 null | 结构化 `ERROR`；不得默认通过 |
| C3 | 类型不匹配 | 数值公式字段为不可解析字符串 | 结构化 `ERROR`；不生成发奖副作用 |
| C4 | 除以 0 | `formula="attributes.amount / attributes.count"`，count 为 0 | 规则 `ERROR`；活动不得默认通过 |
| C5 | 历史非法规则 | 通过测试夹具准备历史绕过校验的非法 `FORMULA` | 运行或 prelaunch 时失败为 `ERROR` / 拒绝上线；不得 `PASSED` |
| C6 | FORMULA evaluator 未注册环境 | 在未注册 evaluator 的 registry 中提交合法 FORMULA | 400，`RULE_EVALUATOR_NOT_REGISTERED` 或等价稳定码 |

### D. SCRIPT / LIMIT 拒绝

| ID | 场景 | 输入 | 预期 |
|---|---|---|---|
| D1 | SCRIPT inline JavaScript | `ruleType=SCRIPT`，`ruleConfig.script="return true"` | 400；拒绝不受控代码 |
| D2 | SCRIPT SQL/Shell/Python/Lua | inline code 或 language 字段声明任意脚本 | 400 |
| D3 | SCRIPT 未审核 scriptRef | `scriptRef` 不存在、未审核或无沙箱能力 | 400 |
| D4 | LIMIT evaluator 未注册 | `ruleType=LIMIT`，结构看似合法 | 400；不得保存为默认通过规则 |
| D5 | LIMIT 与 quota 不混淆 | 配置既有 quota-backed budget/stock/user limit，同时提交未注册 `ruleType=LIMIT` | 未注册 LIMIT 规则 400；既有 quota 语义测试仍按原链路执行 |

### E. 额度不足

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| E1 | FORMULA 通过但活动预算不足 | 合法公式通过；活动 budget 已为 0 | 返回或记录 `QUOTA_EXCEEDED` / budget 不足；不得生成成功 grant_order/outbox；不得出现负预算 |
| E2 | FORMULA 通过但库存不足 | 合法公式通过；stock 不足 | 整体失败或拒绝；不得部分发奖；quota/participation/grant_order/outbox 一致 |
| E3 | FORMULA 通过但用户次数不足 | 同一 subject 达到 USER_LIMIT/DAILY_LIMIT 后再次触发 | 后续额度阶段拒绝；不得重复扣减或重复发奖 |
| E4 | FORMULA 拒绝时不触发额度扣减 | 公式结果不满足 compare，同时 quota 仍充足 | 活动在规则阶段拒绝；quota、grant_order、outbox 均无变化 |
| E5 | 模拟额度不足预览 | simulate 含公式和不足 quota 的活动 | 只输出风险/预览；不写业务表、不扣 quota |

### F. 幂等重复

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| F1 | 相同 eventId 重复提交 | 含合法 FORMULA 的事件首次成功后再次提交相同 payload | 只处理一次；不重复计算成多次发奖；不重复扣 quota |
| F2 | 相同 Idempotency-Key 相同 payload | 连续两次调用事件入口 | 第二次返回 duplicate 或等价幂等结果；业务记录数量不变 |
| F3 | 相同 Idempotency-Key 不同 payload | 第二次修改 `attributes.amount`，使公式结果不同 | 返回 `INVALID_IDEMPOTENCY_KEY` 或等价冲突；不得覆盖首次结果 |
| F4 | 重复非法配置提交 | 连续提交同一个非法 FORMULA / SCRIPT / LIMIT 配置 | 每次均 400；不得因缓存、草稿或禁用状态落库 |
| F5 | 历史 duplicate 查询解释一致 | 重复事件返回已有结果时 | 规则解释、actual/expected、decision 与首次处理保持一致 |

### G. 并发

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| G1 | 并发相同 eventId | N 个 goroutine 同时提交相同 eventId 和 payload | 最多一次创建业务记录；其余 duplicate；无唯一约束异常泄漏为 500 |
| G2 | 并发同一幂等键冲突 payload | N 个请求使用同 key，其中部分 payload 不同 | 首次请求确定结果；冲突请求稳定拒绝；无重复扣减 |
| G3 | 并发争用最后一份 budget | 多个不同事件都满足 FORMULA，活动剩余 budget=1 | 最多 1 个成功；其余额度不足；PG/内存仓储结果一致 |
| G4 | 并发争用 USER_LIMIT/DAILY_LIMIT | 同一 subject 多事件并发命中用户上限 | 成功数量不超过上限；失败有结构化额度不足原因 |
| G5 | 并发模拟 | 多个 simulate 请求同时执行同一公式活动 | 响应稳定；无持久化副作用；无 data race |
| G6 | 并发 prelaunch validation | 多个请求同时校验含 FORMULA 的活动配置 | 合法配置均通过；非法配置均拒绝；registry 状态无竞态 |

### H. PostgreSQL Smoke

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| H1 | migration/schema 可用 | 在干净 PG 测试库运行迁移和仓储集成测试 | `marketing_rule` 能保存 `FORMULA` JSON；相关 activity/event/grant/outbox/quota 表可用 |
| H2 | 合法 FORMULA 落库读取 | 通过 Admin/service 保存合法 Decimal 与 Boolean 公式，再查询活动 | JSON 不丢字段；`scale`、compare value、enabled、sort、version 类型正确 |
| H3 | 非法规则不半保存 | 提交非法公式、安全违规 SCRIPT、未注册 LIMIT | 返回 400；活动和规则无半保存记录；事务回滚正确 |
| H4 | 正式事件成功链路 | PG 下处理一个公式通过且 quota 足够的事件 | inbox、participation、grant_order、outbox、quota 记录符合预期 |
| H5 | 公式拒绝/错误无副作用 | PG 下处理公式拒绝、除零或类型错误事件 | 不生成成功 grant_order/outbox；quota 不扣减；participation/audit 有结构化原因 |
| H6 | 幂等唯一约束 | PG 下重复相同 eventId / idempotency key | 仅一套业务记录；重复返回受控结果 |
| H7 | 并发 quota 原子性 | PG 下并发争用最后预算/库存/用户次数 | 无超发；失败请求结构化返回额度不足 |
| H8 | 历史非法规则防御 | 通过测试夹具插入历史非法 rule_config_json | prelaunch 或运行拒绝；应用层不暴露原始 SQL 堆栈；规则不得默认通过 |

## Required Commands

后续 Test Agent 在实现完成后按顺序执行。以下仅为命令清单，本 Test Plan Agent 未运行。

~~~bash
go test ./internal/rule
go test ./internal/marketing
go test ./internal/engine
go test ./internal/httpapi
go test ./internal/decimal
go test ./...
go test -race ./internal/rule ./internal/engine ./internal/httpapi
go build ./...
~~~

针对本需求的定向用例可追加：

~~~bash
go test ./... -run 'Formula|FORMULA|Script|SCRIPT|Limit|LIMIT|Evaluator|Rule'
go test ./... -run 'Decimal|Boolean|Compare|Field|Provider|Plugin|Unsafe|Divide|Idempot|Duplicate|Quota|Budget|Stock|Concurrent'
~~~

PostgreSQL smoke 命令按仓库 compose 和环境变量约定执行：

~~~bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/httpapi ./internal/engine
docker compose down
~~~

如果 Test Agent 新增 integration build tag，应额外执行：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -tags=integration ./...
~~~

## Data / Smoke

建议准备以下最小数据集：

1. 租户：`tenant_formula_rule_test`，以及跨租户 `tenant_formula_rule_other`。
2. 标准事件：`eventType=DEPOSIT_SUCCESS`、`sourceSystem=payment`、`subject.type=USER`、`subject.id=user_formula_001`、`biz.bizType=DEPOSIT`、`biz.bizId=deposit_formula_001`、`occurredAt=2026-05-27T12:00:00Z`。
3. attributes：`amount=1000.00`、`deposit=1000.00`、`fee=10.00`、`count=2`、`isFirstDeposit=true`、`channel=APP`。
4. 合法规则：Decimal 公式、Boolean 缺省 compare、enabled=false 合法公式、同 phase 多 sort 规则。
5. 非法规则：非法字段、函数调用、SQL/HTTP/文件/env/random/now、非法 op、非法 compare、超长表达式、过多字段、过深括号、非法 scale、enabled=false 不安全配置。
6. `SCRIPT` 数据：inline JavaScript、SQL、Shell、未审核 `scriptRef`。
7. `LIMIT` 数据：未注册高级 LIMIT evaluator payload，和既有 quota-backed budget/stock/user limit 数据。
8. 额度数据：budget=0、budget=1、stock=0、stock=1、USER_LIMIT=1、DAILY_LIMIT=1。
9. 幂等数据：同一 eventId 重复、同一 Idempotency-Key 相同 payload、同一 key 不同 amount payload。
10. PostgreSQL smoke：干净库一套，另准备一条历史非法 `marketing_rule` 记录用于运行时防御测试。

Smoke 验证必须记录：

- 每个规则结果包含 `ruleCode`、`rulePhase`、`ruleType`、`status`、`passed`、`reasonCode`、`reason`、`expected`、`actual`、`sort`。
- 错误响应能定位 `rules[index]`、`ruleCode`、字段名或公式。
- 公式拒绝、公式错误、额度不足和幂等重复都没有重复发奖、重复 outbox 或重复扣 quota。
- PostgreSQL 下无半保存、无超发、无原始 SQL 错误堆栈暴露。

## Decision

~~~text
PASS
~~~

需求：2026-05-27-rule-formula-script-limit
