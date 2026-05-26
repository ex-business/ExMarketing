# Review

## Findings

```text
P0:
- 无。

P1:
- 无。

P2:
- analysis.md:420 仍允许 Provider 配置入口采用 Admin API、受控 seed/fixture 或迁移初始化数据之一。该口径已要求写入正式 registry，并要求活动规则校验与 /openapi/v1/facts/query 读取同一份 registry，因此不阻塞；但 Test Plan 需要把“端到端先写入/预置 marketing_fact_provider，再创建 facts 规则并执行 Fact Query”列为强制验收，避免 Dev 只用内存硬编码 provider 通过单测。
```

## Review Notes

```text
- 前次 P1-1 已关闭：analysis.md:15-34、143-178 明确了 mapping_json 最小结构、outputPath 语义、source JSONPath 子集、type 枚举、required 默认值和非法 schema 规则。
- 前次 P1-2 已关闭：analysis.md:40-42、320、403-409 明确规则保存/上线必须校验 mapping 覆盖每个 facts.<factCode>.<path> 引用，缺失字段固定 FACT_FIELD_MISSING。
- 前次 P2 factCode 唯一性已关闭：analysis.md:48、313、394 统一为 tenant_id + fact_code 唯一，不允许同租户 disabled 重复记录。
- 前次 P2 配置入口已降为非阻塞：analysis.md:54-56、326、340、392-393 明确配置必须进入正式 registry，且规则校验和 Fact Query 使用同一份 registry。
```

## Open Questions

```text
- 无阻塞问题。
```

## Score

```text
需求清晰度        19/20
范围可控          18/20
风险可控          18/20
验收标准明确      19/20
架构一致性        18/20
总分              92/100
```

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

需求：2026-05-24-fact-provider
