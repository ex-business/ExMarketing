# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

不改代码，不提交。

## Findings

~~~text
P0:
P1:
P2:
~~~

## Open Questions

待填写。

## Decision

~~~text
PASS / BLOCK
~~~

规则：

~~~text
存在 P0/P1 -> BLOCK
Score < 80 -> BLOCK
无 P0/P1 且 Score >= 80 -> PASS
~~~

需求：2026-05-24-113400-3
