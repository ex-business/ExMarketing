# Agent Prompts

## Analysis Agent

~~~text
你是 Analysis Agent。请处理 2026-05-24-113929-2。
读取 tasks/2026-05-24-113929-2/task.md、README.md、CLAUDE.md、docs/architecture.md 和相关代码。
只写 tasks/2026-05-24-113929-2/analysis.md，不改代码。
按评分规则打分，Score >= 80 且方案可控才 Decision PASS。
~~~

## Review Agent

~~~text
你是 Review Agent。请审核 tasks/2026-05-24-113929-2/analysis.md。
只写 tasks/2026-05-24-113929-2/review.md，不改代码。
存在 P0/P1 或 Score < 80 必须 BLOCK。
~~~

tasks/2026-05-24-113929-2

## Dev Agent

~~~text
你是 Dev Agent。请在 ## Test Plan Agent

~~~text
你是 Test Plan Agent。请在 tasks/2026-05-24-113929-2/review.md 为 PASS 后，先设计测试用例。
只写 tasks/2026-05-24-113929-2/test-plan.md，不改业务代码。
测试计划必须覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke。
Decision 必须是 PASS / BLOCK。
~~~/review.md 和 tasks/2026-05-24-113929-2/test-plan.md 为 PASS 后开发。
严格按 tasks/2026-05-24-113929-2/task.md、analysis.md、review.md、test-plan.md 修改代码和文档。
完成后必须执行 go test ./... 和 go build ./...。
完成后写 tasks/2026-05-24-113929-2/implementation.md。
~~~

## Test Agent

~~~text
你是 Test Agent。请根据 tasks/2026-05-24-113929-2/task.md、analysis.md、review.md、test-plan.md、implementation.md 执行测试。
执行必要测试并写 tasks/2026-05-24-113929-2/test.md。
~~~

## Commit Agent

~~~text
你是 Commit Agent。只有 review.md 和 test.md 都 PASS 才能提交。
提交后写 tasks/2026-05-24-113929-2/commit.md。
~~~
