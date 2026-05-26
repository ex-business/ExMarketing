# Test Plan

## Agent 指令

你是 Test Agent。`review.md` 已为 PASS，本计划只定义后续测试用例和命令清单。

执行测试时必须按 `task.md`、`analysis.md`、`review.md` 和本文件验证 Rule 配置 Schema。不得扩大需求范围为实现新的 FORMULA / LIMIT / SCRIPT 执行能力；未注册 evaluator 的规则必须被拒绝。

## Test Strategy

1. 单元测试优先覆盖 rule schema validator：枚举、字段路径、未知字段、op、value 类型、`enabled=false`、重复 `ruleCode`、sort/version 边界。
2. Admin API / service 层测试覆盖 create/update 或 prelaunch validation：合法规则可保存，非法规则返回 400 类校验错误，且错误能定位到 `rules[index]` 和字段。
3. 运行链路回归只验证已合法保存的 `CONDITION` 行为：enabled false 跳过、失败条件不通过、历史异常配置不得默认通过。
4. 额度不足、幂等重复、并发测试作为交易/营销核心回归：确认本次规则 Schema 改动没有破坏 quota、participation、grant_order、outbox、PostgreSQL 持久化一致性。
5. PostgreSQL smoke 使用真实 PG 连接跑最小活动创建、事件处理、结果落库、唯一约束和事务回滚验证。

## Test Cases

### A. 正常路径

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| A1 | 合法 CONDITION 使用标准事件字段 | 创建活动，rules 包含 `eventType`、`subject.id`、`biz.bizId` 条件 | 保存成功；查询活动时保留 `ruleCode/rulePhase/ruleType/ruleConfig/sort/enabled/version` |
| A2 | 合法 CONDITION 使用 attributes 字段 | `attributes.amount >= "100.00"`、`attributes.vipLevel IN ["VIP1","VIP2"]` | 保存成功；运行事件命中时规则通过 |
| A3 | 合法 disabled 规则 | `enabled=false` 且 ruleConfig 完全合法 | 保存成功；运行解释中该规则为 SKIPPED 或不参与决策，不改变活动通过结果 |
| A4 | 多规则排序 | 同一 phase 下配置多个 sort：0、10、20 | 保存成功；运行解释按 sort 升序稳定输出 |
| A5 | version 默认/显式值 | 不传 version 与传 `version=2` 分别创建 | 不传时使用默认正整数；显式正整数保存并可查询 |

### B. Schema 非法路径

| ID | 场景 | 输入 | 预期 |
|---|---|---|---|
| B1 | 非法 rulePhase | `BUDGET`、`RISK_LIMIT_BUDGET` | 返回 400；不得保存规则 |
| B2 | 非法 ruleType | `UNKNOWN` | 返回 400；提示 ruleType 非法或 evaluator 不可用 |
| B3 | ruleConfig 类型非法 | null、数组、字符串、数字、布尔 | 返回 400；错误定位到对应 `rules[index].ruleConfig` |
| B4 | CONDITION logic 非法 | `logic=XOR` | 返回 400 |
| B5 | CONDITION conditions 空 | `conditions=[]` | 返回 400 |
| B6 | condition 缺字段 | 缺 `field` / `op` / `value` | 返回 400；错误定位到 condition 下标和字段 |
| B7 | 非法 op | `LIKE`、`BETWEEN` | 返回 400 |
| B8 | 非法 value 类型 | `>` 比较 boolean/object/array；`IN` value 不是数组；`IN` 空数组 | 返回 400 |
| B9 | 未知顶层字段 | ruleConfig 多传 `extra`；condition 多传 `caseInsensitive` | 返回 400；未知字段不得静默忽略 |
| B10 | 非法字段路径 | `orderAmount`、`deposit.amount`、`user.vipLevel`、`plugin.xxx` | 返回 400；不得被解释为 attributes |
| B11 | facts provider 不可用 | `facts.USER_PROFILE.vipLevel`，但 provider 未配置或禁用 | create/update 或 prelaunch validation 返回 400 |
| B12 | plugin 字段越权 | 使用未绑定插件的 `state.plugin.X.amount` | 返回 400 |
| B13 | enabled=false 非法 Schema | `enabled=false` 但 `op=LIKE` 或字段非法 | 仍返回 400；不得作为草稿保存 |
| B14 | 重复 ruleCode | 同一活动 rules 中两个相同 `ruleCode` | 返回 400；避免解释和审计歧义 |
| B15 | sort/version 边界 | sort 为负数/小数/字符串；version 为 0/负数/小数/字符串 | 返回 400 |

### C. FORMULA / LIMIT / SCRIPT 边界

| ID | 场景 | 输入 | 预期 |
|---|---|---|---|
| C1 | FORMULA schema 结构合法但 evaluator 未注册 | `ruleType=FORMULA`，合法 `formula/resultType/compare/scale` | 若未注册 FORMULA evaluator，返回 400，不得保存为默认通过规则 |
| C2 | FORMULA 危险表达式 | SQL、HTTP、文件、环境变量、随机数、当前时间、外部函数 | 返回 400 |
| C3 | FORMULA 字段引用非法 | `user.vipLevel + 1` | 返回 400 |
| C4 | LIMIT schema 结构合法但 evaluator 未注册 | `ruleType=LIMIT`，合法 `limitType/scope/window/max/field` | 若未注册 LIMIT evaluator，返回 400，不得混淆 quota 扣减 |
| C5 | LIMIT max 边界 | `max=0`、`max=1`、`max=-1`、`max=1.5` | 0 和正整数按 analysis 允许；负数/小数拒绝 |
| C6 | SCRIPT 默认拒绝 inline code | `script` / `inlineCode` / SQL / Shell / JavaScript | 返回 400 |
| C7 | SCRIPT 未审核 scriptRef | `scriptRef` 不存在或未审核 | 返回 400 |

### D. 额度不足

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| D1 | 活动预算不足 | 配置合法 CONDITION 活动和有限预算；先消耗预算到 0，再发送符合条件事件 | 事件规则可匹配，但发奖/participation 失败为额度不足；不得创建成功 grant_order；不得产生负数预算 |
| D2 | 用户次数不足 | 配置合法规则和用户维度限制；同一 subject 达到上限后再次触发 | 第二次返回或记录为用户额度不足/limit exceeded；不得重复发奖 |
| D3 | 库存不足事务回滚 | 在库存不足时触发活动 | participation、quota、grant_order、outbox 状态一致；失败不得部分落库成已发奖 |

### E. 幂等重复

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| E1 | 同一事件重复投递 | 对同一 `eventId` 连续调用两次事件处理 | 只产生一次 participation / grant_order / outbox；第二次返回幂等结果或重复忽略 |
| E2 | 幂等键重复但 payload 相同 | 相同业务幂等键重复请求 | 不重复扣减 quota；不重复发奖 |
| E3 | 幂等键重复但 payload 冲突 | 相同幂等键、不同关键 payload | 返回冲突或拒绝；不得覆盖首次结果 |
| E4 | 非法规则配置请求重复 | 连续提交同一个非法 rules payload | 每次均 400；不得因缓存或重复请求落库 |

### F. 并发

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| F1 | 并发创建相同 ruleCode 活动配置 | 并发提交包含重复 ruleCode 的活动配置或更新 | 全部非法请求均拒绝；合法并发请求不产生重复规则行 |
| F2 | 并发事件命中同一用户上限 | N 个并发事件使用同一 subject 命中同一活动 | 最多按 quota/user limit 成功；其余失败为 limit exceeded；无超发 |
| F3 | 并发消耗最后一份预算 | 多线程同时触发剩余 1 份预算的活动 | 仅 1 个成功；PG 中预算、participation、grant_order 一致 |
| F4 | 并发重复 eventId | 多线程同时提交相同 eventId | 只处理一次；无唯一约束异常泄漏为 500 |

### G. PostgreSQL Smoke

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| G1 | migration/schema 可用 | 在干净 PG 测试库启动应用或跑仓库既有 PG 集成测试 | `marketing_rule` 相关列存在：`rule_phase/rule_type/rule_config_json/sort/enabled/version` |
| G2 | 合法规则落库与读取 | 通过 Admin create 保存合法 CONDITION，再直接或通过 API 查询 | JSON 内容完整；布尔、整数、版本无类型漂移 |
| G3 | 非法规则不落库 | 提交非法字段/op/value 后查询 DB | 活动和规则不得出现半保存记录；事务回滚正确 |
| G4 | 历史异常配置防御 | 手工准备一条历史非法 rule_config_json，再触发运行或 prelaunch validation | 不得默认通过；应失败为明确错误或拒绝上线 |
| G5 | 约束和索引 smoke | 插入/创建同活动重复 ruleCode 或违反唯一约束数据 | 返回受控错误；应用层不暴露原始 SQL 堆栈 |

## Required Commands

后续 Test Agent 应根据仓库实际测试组织选择精确包路径；至少执行以下命令清单。

~~~bash
go test ./...
go build ./...
~~~

若仓库存在分层测试标签或脚本，追加执行：

~~~bash
go test ./... -run 'Rule|Schema|Condition|Formula|Limit|Script|Activity'
go test ./... -run 'Idempot|Duplicate|Quota|Budget|Limit|Concurrent|Postgres|PG'
~~~

若存在 PostgreSQL 集成测试环境变量或 compose 文件，按仓库约定启动测试 PG 后执行对应集成测试，例如：

~~~bash
TEST_DATABASE_URL='postgres://<user>:<password>@localhost:<port>/<db>?sslmode=disable' go test ./... -run 'Postgres|PG|Integration'
~~~

## Data / Smoke

建议准备以下最小测试数据：

1. 租户：`tenant_rule_schema_test`。
2. 活动：一个普通活动，包含合法 `CONDITION` 规则，phase 覆盖 `MATCH`、`QUALIFY`、`LIMIT` 中至少两个。
3. 标准事件 payload：包含 `eventId`、`eventType`、`sourceSystem`、`tenantId`、`subject.type`、`subject.id`、`biz.bizType`、`biz.bizId`、`occurredAt`、`attributes.amount`。
4. 非法规则 payload：非法 field、非法 op、非法 value 类型、未知字段、`enabled=false` 非法 Schema、未注册 FORMULA/LIMIT/SCRIPT。
5. quota 数据：预算 0、预算 1、用户上限 1 三类，用于额度不足和并发超发验证。
6. 幂等数据：同一 `eventId` 重复、同一幂等键相同 payload、同一幂等键冲突 payload。
7. PostgreSQL smoke 数据库：干净库一套，另准备一条历史非法 `marketing_rule` 记录用于防御性运行测试。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-rule-schema
