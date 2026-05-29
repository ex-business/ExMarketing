# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

只写 tasks/2026-05-27-db-migration-release-migration/review.md，不改代码。

存在 P0/P1 或 Score < 80 必须 BLOCK。

## Findings

```text
P0:
无。

P1:
无。

P2:
1. migration version 表的最终表名、字段类型、索引/唯一约束、checksum 算法和状态枚举还未完全固化。analysis.md 已给出最小字段口径和行为约束，足以进入 Dev/Test；实现阶段必须把这些细节落到代码和文档中。
2. drift 检测的对象清单目前采用最小可验收口径：version/checksum + 核心表存在性 + 关键异常。该范围满足本任务验收，但列、索引、约束的完整比对需要后续按已发布 migration 清单逐步补齐。
3. 本地验证要求清晰，但具体命令名或脚本路径尚未指定。后续实现需要提供可复制命令，覆盖空库初始化、旧库升级、重复执行、失败 migration 和 drift 制造场景。
```

## Review Notes

- analysis.md 已从模板状态补齐背景、目标、参与方、输入输出、细化需求、业务规则、边界异常、非目标、风险点、验收标准和默认决策。
- 核心任务点覆盖充分：migration version 表、checksum 校验、已成功 migration 不可变、按版本升序执行、失败状态记录、低版本失败不得跳过高版本、已有业务数据不得被清空或重置。
- 新环境初始化和已有环境幂等升级均有明确验收口径，且要求重复执行无副作用。
- schema drift 已明确必须阻止发布，至少覆盖已执行文件 checksum 改变、关键表缺失、已应用版本文件缺失、重复版本和失败状态未处理。
- 回滚策略选择 forward-only，符合生产数据库 migration 常见约束；analysis.md 已补足失败停止、失败记录、人工恢复或新增修复 migration、已成功版本不得修改的处理规则。
- 历史已有业务表但无 version 表的高风险边界已被识别为 BLOCK 自动升级，避免直接重跑初始化误伤数据。
- 服务启动约束没有强行决定自动 apply 或独立命令，但要求 PostgreSQL repository 不得在 schema 未达标时静默处理业务事件，约束足够支撑实现。

## Open Questions

无阻塞问题。

非阻塞待实现阶段落地：

- migration version 表的最终命名、DDL 和 checksum 算法。
- migration 发布命令的入口形态，例如独立 Go command、workflowctl 子命令或脚本。
- 核心表之外的列、索引、约束 drift 清单如何分阶段扩展。

## Score

```text
需求清晰度        18/20
业务规则完整      19/20
边界异常清楚      19/20
验收标准明确      18/20
约束一致性        18/20
Score             92/100
```

扣分原因：

- 具体实现入口、version 表 DDL 和 checksum 算法仍留给 Dev 阶段定型。
- drift 深度采用本轮最小可验收口径，未一次性要求列、索引、约束全量比对。

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

需求：2026-05-27-db-migration-release-migration
