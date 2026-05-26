# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后、Dev 开发前，根据 task.md、analysis.md、review.md 设计测试用例。

只写 test-plan.md，不改业务代码，不执行提交。

## Test Strategy

Review Decision 已确认为 `PASS`，本测试计划允许后续 Dev Agent 进入开发；Test Agent 在开发完成后执行本计划。

测试目标：

- 覆盖 HTTP / WEBHOOK outbox sender 的成功、失败、重试、DLQ、跳过和异常输入。
- 覆盖 quota 不足、规则拒绝、零金额奖励、重复事件等上游回归，确认 sender 不重新创建业务记录。
- 覆盖幂等重复和多 worker 并发 claim，避免同一 outbox 被重复并发投递。
- 覆盖 PostgreSQL 环境 smoke，特别是 claim / retry / status update 在真实 PostgreSQL 语义下可用。

建议分层：

- 单元测试：HTTP 响应分类、错误摘要截断/脱敏、非法 target / payload、retry / DLQ 状态计算。
- 集成测试：使用 `httptest.Server` 或等价 mock HTTP server 验证实际请求体、请求次数、超时和响应码处理。
- PostgreSQL smoke：使用 Docker PostgreSQL 或项目已有 PostgreSQL 测试环境，验证数据库 claim、状态更新和并发扫描。
- 回归测试：复用现有事件处理测试，确认无 outbox 场景和重复事件场景不被本需求破坏。

## Test Cases

### 正常路径

1. `HTTP` 目标返回 `200`
   - 准备一条 `target_channel=HTTP`、`send_status=PENDING`、合法 `target_ref`、合法 `payload_json` 的 outbox。
   - mock server 返回 `200`。
   - 期望：只发送一次；请求 body 等于原 `payload_json`；状态更新为 `SENT`；不再被下一轮普通扫描发送。

2. `WEBHOOK` 目标返回 `204`
   - 准备一条 `target_channel=WEBHOOK` 的 outbox。
   - mock server 返回 `204`。
   - 期望：状态为 `SENT`；`HTTP 204` 按 2xx 成功处理。

3. Worker 未开启
   - 通过上游流程生成 HTTP / WEBHOOK outbox，但不启动 sender worker。
   - 期望：outbox 保持待处理状态；mock server 请求数为 0。

### 失败、重试和 DLQ

4. `500` 可重试失败
   - mock server 返回 `500`。
   - 期望：outbox 不丢失；状态为可重试失败状态；`retry_count` 增加；`next_retry_at` 被设置；`error_message` 包含 500 摘要。

5. `429` 可重试失败并处理 `Retry-After`
   - mock server 返回 `429`，带 `Retry-After`。
   - 期望：状态为可重试失败；`retry_count` 增加；`next_retry_at` 体现 `Retry-After` 或项目等价策略；不进入立即成功。

6. 不可重试 `4xx`
   - 分别覆盖 `400`、`401`、`403`、`404`、`409`、`422` 中至少三个代表码，建议参数化覆盖全部。
   - 期望：进入 `DLQ` 或项目定义的等价人工处理状态；记录状态码摘要；普通自动扫描不再发送。

7. 最大重试次数进入 DLQ
   - 准备 `retry_count` 已达到最大自动尝试次数前一档的 outbox，mock server 返回 `500` 或制造超时。
   - 期望：本次失败后进入 `DLQ`；后续普通扫描不再请求目标。

8. 请求超时
   - mock server 延迟超过 sender timeout。
   - 期望：状态为可重试失败；`retry_count` 增加；`error_message` 包含 timeout 摘要；outbox 不丢失。

9. 网络错误 / 连接失败
   - `target_ref` 指向不可连接端口或 mock 连接错误。
   - 期望：状态为可重试失败；记录网络错误摘要；不崩溃。

10. HTTP 3xx
    - mock server 返回 `302` 并设置 `Location`。
    - 期望：不按成功处理；不得跟随到非配置目标；记录失败摘要，具体 FAILED / DLQ 按实现默认分类验证。

### 非法输入和非目标通道

11. `target_ref` 为空
    - 期望：不发起请求；状态进入失败或 DLQ；记录 invalid target 摘要；进程不崩溃。

12. `target_ref` 非 HTTP(S)
    - 例如 `ftp://example.com/reward` 或普通字符串。
    - 期望：不发起 HTTP 请求；状态进入失败或 DLQ；记录非法 URL 摘要。

13. `payload_json` 非法
    - 准备非法 JSON 字符串。
    - 期望：不构造替代 payload；状态进入失败或 DLQ；记录 invalid payload 摘要；不崩溃。

14. `target_channel=MQ`
    - 准备 MQ outbox。
    - 期望：HTTP sender 不发送；不把状态改为 `SENT`、`FAILED` 或 `DLQ`。

### 额度不足和上游回归

15. 额度不足不生成 outbox
    - 构造活动 quota 不足的标准事件。
    - 期望：不创建新的 participation、grant_order、marketing_outbox；sender 无可投递记录。

16. 规则拒绝不生成 outbox
    - 构造不满足活动规则的事件。
    - 期望：不创建 grant_order / marketing_outbox；sender 不发送。

17. 零金额奖励不生成 outbox
    - 构造奖励计算结果为 0 的事件。
    - 期望：不创建发奖 outbox；sender 不发送。

### 幂等重复

18. 重复标准事件只生成一次业务 outbox
    - 用相同事件幂等键重复提交同一标准事件。
    - 期望：participation、grant_order、marketing_outbox 均不重复新增；sender 最多发送一次原 outbox。

19. 重复扫描已 `SENT` outbox
    - 将同一 outbox 成功发送为 `SENT` 后再次触发普通扫描。
    - 期望：不会再次请求目标；状态保持 `SENT`。

20. retry 复用原 payload
    - 首次返回 `500`，下一次达到 `next_retry_at` 后返回 `200`。
    - 期望：第二次请求使用同一 outbox 的原 payload 和 command 语义；不新增 participation、grant_order 或新 outbox。

### 并发和 PostgreSQL

21. 多 worker 并发 claim 同一 outbox
    - 在 PostgreSQL 环境准备一条可处理 outbox，同时启动多个 sender worker 或并发调用 claim+send 入口。
    - mock server 记录请求次数。
    - 期望：同一处理窗口内最多一个 worker claim 成功；目标请求数为 1；最终状态一致。

22. 多条 outbox 并发处理
    - 在 PostgreSQL 环境准备多条不同 outbox，并发 worker 扫描。
    - 期望：每条 outbox 最多发送一次；状态更新无跨 tenant / 跨记录污染。

23. tenant 隔离
    - 准备不同 `tenant_id` 的 outbox。
    - 期望：状态更新保留各自 tenant；不会跨 tenant 修改或查询错记录。

24. claim 后恢复口径 smoke
    - 若实现存在 `SENDING` / lease / locked 状态，准备一条过期 claim 的 outbox。
    - 期望：达到恢复条件后可重新被处理，或至少可被人工识别；不会永久卡死在不可处理状态。

### 安全和错误信息

25. 错误摘要截断
    - mock server 返回超大错误 body。
    - 期望：`error_message` 被截断到实现定义长度；不完整保存响应体。

26. 敏感信息脱敏
    - payload 或错误响应中包含 `token`、`secret`、`Authorization` 等字段。
    - 期望：`error_message` 和日志不包含完整 Authorization header、token、secret 或完整敏感 payload。

## Required Commands

Test Agent 在 Dev Agent 完成实现后至少执行以下命令；本 Test Plan Agent 不执行这些命令。

~~~bash
go test ./...
go build ./...
~~~

如果项目已有 PostgreSQL 集成测试开关或 profile，Test Agent 必须额外执行对应命令。若当前仓库尚无固定命令，建议 Dev/Test Agent 在实现时补齐可重复执行的 PostgreSQL smoke 命令，例如：

~~~bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://postgres:postgres@localhost:5432/exmarketing_test?sslmode=disable' go test ./... -run 'Outbox|Postgres|Sender|Claim|Concurrent'
docker compose down
~~~

若仓库使用其他 PostgreSQL 启动方式，以项目现有脚本或 compose 文件为准，但必须在测试报告中记录实际执行命令、数据库连接方式和结果。

## Data / Smoke

PostgreSQL smoke 最小数据集：

- `tenant_a` 下 1 条 HTTP `PENDING` outbox，mock 返回 200。
- `tenant_a` 下 1 条 WEBHOOK `PENDING` outbox，mock 返回 204。
- `tenant_a` 下 1 条 HTTP `PENDING` outbox，mock 返回 500。
- `tenant_a` 下 1 条 HTTP `PENDING` outbox，mock 返回 429 + `Retry-After`。
- `tenant_a` 下 1 条 HTTP `PENDING` outbox，mock 返回 400 或 422。
- `tenant_a` 下 1 条 HTTP outbox，`target_ref` 非法。
- `tenant_a` 下 1 条 HTTP outbox，`payload_json` 非法。
- `tenant_a` 下 1 条 MQ `PENDING` outbox，用于验证跳过。
- `tenant_b` 下 1 条 HTTP `PENDING` outbox，用于验证 tenant 隔离。
- 1 条已 `SENT` outbox，用于验证普通扫描不重复发送。
- 1 条已达最大 retry 前置状态的 outbox，用于验证失败后进入 DLQ。

mock HTTP server 需要记录：

- 每个 outbox / command id 的请求次数。
- 收到的请求 body。
- 请求时间，用于验证 timeout / retry 延迟语义。
- 是否发生重定向跟随。

测试报告必须明确列出：

- HTTP 2xx、4xx、5xx、429、超时、网络错误、非法 target、非法 payload、MQ 跳过、最大重试 DLQ 的结果。
- 额度不足、规则拒绝、零金额奖励、重复事件的 outbox 数量验证结果。
- PostgreSQL 并发 claim 的 worker 数、请求次数和最终状态。
- `go test ./...`、`go build ./...`、PostgreSQL smoke 命令的执行结果。

## Decision

~~~text
PASS
~~~

需求：2026-05-24-outbox-http-sender
