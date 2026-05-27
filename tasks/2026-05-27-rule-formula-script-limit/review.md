# Review

## Findings

~~~text
P0:

P1:

P2:
1. `FORMULA` 已从上一轮“仅 Schema 边界”推进为本轮可运行能力，但 analysis 未把表达式词法/AST 校验写成强制实现口径。
   - 位置：tasks/2026-05-27-rule-formula-script-limit/analysis.md:132-148, tasks/2026-05-27-rule-formula-script-limit/analysis.md:156, tasks/2026-05-27-rule-formula-script-limit/analysis.md:261-263
   - 说明：analysis 已明确允许的运算、禁止函数/SQL/HTTP/文件/时间/随机数、字段白名单、长度/字段数/括号深度限制，因此不构成 BLOCK。Dev 阶段必须用解析器/AST 白名单实现字段提取和安全校验，不能沿用简单字符串 contains 检查作为可执行公式安全边界。

2. `FORMULA` 的 `BOOLEAN` 最小能力没有列出逻辑运算符，验收只要求 boolean 公式缺省 `compare`。
   - 位置：tasks/2026-05-27-rule-formula-script-limit/analysis.md:135-140, tasks/2026-05-27-rule-formula-script-limit/analysis.md:167, tasks/2026-05-27-rule-formula-script-limit/analysis.md:297
   - 说明：按当前文本可理解为支持 boolean 字面量或 boolean 字段引用，例如 `attributes.isFirstDeposit`，不要求 `&& / || / !` 或内联比较表达式。Dev/Test 阶段应按这个最小口径实现和验收，避免自行扩展逻辑表达式语言。

3. 文档多处写“创建、更新或上线前校验”会校验 `rules[]`，但当前 API 文档中的活动更新接口主要是主配置更新语义。
   - 位置：tasks/2026-05-27-rule-formula-script-limit/analysis.md:122, tasks/2026-05-27-rule-formula-script-limit/analysis.md:237, tasks/2026-05-27-rule-formula-script-limit/analysis.md:295
   - 说明：这不影响通过。Dev 阶段应优先覆盖现有创建、审批/上线前校验、模拟运行和正式运行链路；如果本轮扩展 update payload 支持 `rules[]`，再复用同一校验口径。
~~~

## Open Questions

~~~text
1. `FORMULA` 表达式最终采用 CEL、安全第三方库还是自研 parser，由 Dev 决策；Review 仅要求 AST/语义白名单满足 analysis 的安全边界。
2. `BOOLEAN` 公式本期是否只支持 boolean 字段/字面量。当前 Review 按“只支持最小口径，不新增逻辑运算符”理解，不阻断。
~~~

## Score

~~~text
90
~~~

## Decision

~~~text
PASS
~~~

规则：无 P0/P1，且 Score >= 80，允许进入 Dev。

需求：2026-05-27-rule-formula-script-limit
