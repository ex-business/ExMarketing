# Review

## Scope

审核对象：`tasks/2026-05-24-113929-6/analysis.md`

本次只审核 Analysis 产物是否足够支撑后续 Test Plan / Dev / Test，不修改业务代码。

## Findings

```text
P0:
- 无。

P1:
- 无。

P2:
- analysis.md 允许 item.result 出现 WARN，而 task.md 的验收只写逐项 PASS/BLOCK。Analysis 已说明 WARN 只能作为非阻断提示，最终上线判断以 BLOCK 为准；后续 Test Plan 需要覆盖“含 WARN 但无 BLOCK 时 allowPublish=true”的语义，避免实现把 WARN 当成第三种发布决策。
- analysis.md 给出了默认高危阈值：单次最大奖励 1000.0000、活动总预算 100000.0000。现有来源材料没有产品级阈值，这个默认值可作为本轮保守验收口径，但 Dev 应集中封装为可替换配置，避免硬编码散落在校验逻辑中。
- analysis.md 没有指定最终 API 路径或函数名，只定义了输入、输出、状态变更闸门和报告结构。该粒度足以进入研发，但 Test Plan 需要补齐通过“创建/更新活动上线入口”还是“独立预检接口”验证，确保上线前阻断能被真实调用链覆盖。
```

## Review Notes

- Analysis 已覆盖 task.md 要求的 8 类校验：概率总和、奖励金额超预算、库存、活动时间冲突、互斥组冲突、规则字段、JSON Schema、高危配置。
- Analysis 明确了整体决策规则：任一 `BLOCK` 则 `decision=BLOCK`、`allowPublish=false`，高危配置在审批流未落地前阻断上线，符合“不允许高危配置上线”。
- Analysis 对 Decimal、targetRef、rule field path、enabled=false 配置、quota 缺失、跨租户隔离、版本变化后旧报告失效等关键边界给出了可测试口径。
- Analysis 将审批流、模拟运行解释器、外部业务库和真实钱包能力列为非目标，范围控制合理。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          17/20
验收标准明确      18/20
架构一致性        17/20
总分              88/100
```

扣分原因：

- 高危阈值来自本轮默认口径，不是产品配置或既有文档中的确定值。
- API / 调用入口未在 Analysis 中落到具体路径，需要 Test Plan 和 Dev 阶段补齐。
- `WARN` 扩展了 task.md 的最小 PASS/BLOCK 结果集，需要测试约束避免影响发布决策。

## Open Questions

- 后续实现是否新增独立预检接口，还是只在活动状态变更到 `SCHEDULED` / `ACTIVE` 时触发校验？不阻塞本轮，最低要求是状态变更链路必须阻断 `BLOCK`。
- 默认高危阈值未来是否由 tenant / activityType 配置覆盖？不阻塞本轮，本轮按 Analysis 默认阈值验收。

## Decision

```text
PASS
```

规则：

```text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
```

需求：2026-05-24-113929-6
