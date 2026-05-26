# Review

## Findings

~~~text
P0:

P1:

P2:
1. 当前 `PUT /admin/v1/activities/{id}` 文档只允许更新活动主配置，不包含 `rules[]` 更新语义；analysis 多处写到 Admin create/update 对 `rules[]` 全量校验。
   - 位置：tasks/2026-05-24-rule-schema/analysis.md:87, tasks/2026-05-24-rule-schema/analysis.md:287, tasks/2026-05-24-rule-schema/analysis.md:352
   - 说明：这不影响本需求通过，因为 analysis 已用“一旦请求包含 `rules[]`”收口，且创建活动路径确实承载规则配置。Dev 阶段应按现有 API 先覆盖 create / prelaunch validation；若本轮新增规则更新接口或扩展 update payload，再复用同一 Schema 校验。

2. `FORMULA` 的字段引用校验仍停留在安全边界描述，没有定义表达式 token 提取语法。
   - 位置：tasks/2026-05-24-rule-schema/analysis.md:223-229
   - 说明：analysis 已明确未注册 `FORMULA` evaluator 时必须拒绝保存和上线，因此不会造成运行风险。若后续开放 `FORMULA`，需要另补表达式语言、字段引用解析和类型推导规则。
~~~

## Open Questions

~~~text
1. 是否要在本轮扩展活动更新接口以支持 `rules[]` 更新？当前 Review 按“不新增接口，仅 create / prelaunch validation 复用 Schema”理解，不阻断。
2. `LIMIT.max=0` 是否作为“永不允许”配置保留，还是要求正整数？analysis 当前允许大于等于 0，未发现与现有文档冲突，不阻断。
~~~

## Score

~~~text
91
~~~

## Decision

~~~text
PASS
~~~

规则：无 P0/P1，且 Score >= 80，允许进入 Dev。

需求：2026-05-24-rule-schema
