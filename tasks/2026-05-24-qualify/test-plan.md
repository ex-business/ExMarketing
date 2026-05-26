# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

1. 以通用 `CONDITION` evaluator 作为 QUALIFY 的最小可验收实现，不新增 VIP / 注册天数 / 渠道 / 标签专用 evaluator。
2. 单元测试优先覆盖 QUALIFY 阶段执行顺序、字段读取、Decimal 比较、`IN` / `NOT_IN` 标量与数组交集语义、缺失字段解释。
3. Schema / 服务层测试覆盖字段路径白名单、非法顶层业务字段、facts provider 引用校验、未注册 evaluator 拒绝保存。
4. 事件处理和模拟链路回归覆盖 QUALIFY 拒绝后不生成 grant_order / outbox、不预留 quota，并保留规则级解释。
5. Review P2 明确完整 condition trace 不是本任务最低验收；本计划最低验收为规则级 `field / expected / actual / reasonCode / reason` 稳定可解释。

## Test Cases

| ID | 场景 | 步骤 | 预期 |
|---|---|---|---|
| Q1 | VIP + 注册天数 + 渠道通过 | QUALIFY `CONDITION` 使用 `attributes.vipLevel >= 3`、`attributes.registerDays >= 7`、`attributes.channel IN ["APP","WEB"]` | 规则通过，进入后续阶段 |
| Q2 | 用户标签数组准入 | `attributes.tags` 为数组，规则 `IN ["new_user","high_value"]` | 实际数组与期望数组有交集时通过 |
| Q3 | 风险标签数组排除 | `attributes.riskTags` 为数组，规则 `NOT_IN ["black","bonus_abuse"]` | 无交集时通过，有交集时拒绝 |
| Q4 | 缺失字段解释 | 缺少 `attributes.registerDays` | QUALIFY 拒绝，规则结果包含 `FIELD_MISSING`、`field`、`expected` |
| Q5 | facts 字段缺失解释 | 规则引用 `facts.USER_PROFILE.vipLevel`，运行上下文没有该 fact | QUALIFY 拒绝，规则结果包含 `FACT_FIELD_MISSING` 和具体字段 |
| Q6 | facts provider 引用校验 | 活动保存时引用未配置或 mapping 缺失的 `facts.<factCode>.<path>` | 返回配置错误，错误能定位 factCode / field |
| Q7 | 非法字段路径拒绝 | 配置 `vipLevel`、`user.vipLevel`、`deposit.amount` | 保存或预检失败，不得运行时默认通过 |
| Q8 | 类型不匹配解释 | `attributes.vipLevel` 为不可解析 Decimal 字符串，规则使用 `>=` | QUALIFY 拒绝，规则结果为 `TYPE_MISMATCH` |
| Q9 | disabled 规则跳过 | `enabled=false` 且配置合法 | 规则结果为 `SKIPPED`，不影响 QUALIFY 通过 |
| Q10 | QUALIFY 拒绝无副作用 | 事件命中活动但 QUALIFY 失败 | participation 为 `REJECTED`，不生成 grant_order / outbox，不扣 quota |
| Q11 | 模拟与正式解释一致 | 对同一失败 payload 分别调用模拟和正式事件处理 | `failedStage=QUALIFY`、原因码、字段解释一致 |
| Q12 | 未注册 ruleType | `FORMULA` / `LIMIT` / `SCRIPT` 配置到 QUALIFY | 保存或运行拒绝，不得默认通过 |

## Required Commands

~~~bash
go test ./...
go build ./...
~~~

建议 Test Agent 追加定向回归：

~~~bash
go test ./internal/rule -run 'Qualify|Condition|Fact|Missing'
go test ./internal/engine ./internal/httpapi -run 'Rule|Qualify|Simulate|Fact'
~~~

## Data / Smoke

- 合法事件：`attributes.vipLevel`、`attributes.registerDays`、`attributes.channel`、`attributes.tags`、`attributes.riskTags`。
- 缺失字段事件：省略 `attributes.registerDays`。
- 类型错误事件：`attributes.vipLevel="VIP3"` 并使用 `>= 3`。
- facts smoke：`USER_PROFILE` provider 包含 `vipLevel`、`registerDays` mapping；另准备 mapping 缺失样例。
- 副作用检查：QUALIFY 拒绝后检查响应或快照中无 grant / outbox / quota reservation。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-qualify
