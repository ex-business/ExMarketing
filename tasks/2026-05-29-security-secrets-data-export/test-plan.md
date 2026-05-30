# Test Plan

## Agent 指令

你是 Test Plan Agent。请在 Review PASS 后根据 task.md、analysis.md、review.md 设计测试用例。

只允许写 test-plan.md。
禁止修改业务代码、测试代码、文档代码、配置文件或任何其他文件。
禁止执行测试命令、构建命令、格式化命令或 git 命令。
你的职责只是写清楚后续 Test Agent 应执行的测试用例和命令清单。

## Test Strategy

- 以单元测试覆盖统一脱敏规则，重点验证嵌套 JSON、header map、URL/DSN、provider raw error、token-like key/value。
- 以服务层测试覆盖审计 payload 入库前脱敏，确保 audit payload 不落 secret、token、Authorization、password。
- 以 HTTP API 测试覆盖错误响应脱敏，确保面向 caller 的 `error/message` 不回显敏感输入。
- 以 Fact client 测试覆盖 provider 请求失败时的错误摘要脱敏，避免 provider raw error 进入响应、规则解释或审计。
- 以人工导出 repository 测试覆盖 checksum、operator、createdAt、expiresAt、rowCount、detail trace 等安全元数据。
- 以现有 MQ target/runtime 测试覆盖 activity reward targetRef 与 runtime endpoint 的凭据隔离。
- 以现有 dbbackup 测试覆盖备份输出 SHA-256、路径、大小、UTC 时间，以及 DSN/pg_dump stderr 脱敏。
- 最后执行全量 `go test ./...` 与 `go build ./...`，PostgreSQL 集成测试需要本地 `docker compose up -d postgres` 或等价 `localhost:54329` 测试库。

## Test Cases

1. `marketing.RedactSensitive` 递归脱敏：
   - 输入包含 `Authorization: Bearer raw-token`、`Cookie`、`callbackSecret`、嵌套 `password=... token=...`、带 userinfo 和敏感 query 的 URL。
   - 期望输出不包含原始 token/password/secret/cookie，包含固定 redacted marker。

2. HTTP 错误响应脱敏：
   - 调用 `writeErrorCode` 或构造会返回错误的 handler path，message 中包含 Authorization/password/token。
   - 期望 response body 不包含原始敏感值。

3. Audit payload 脱敏：
   - 通过 `Service.RecordAudit` 写入含 Authorization、provider raw error、password/token 的 payload。
   - 期望 repository 中的 `AuditRecord.Payload` 不包含原始敏感值。

4. Fact provider 失败脱敏：
   - mock transport 返回包含 Authorization/password/token 的错误。
   - 期望 `FactQueryResponse.Error/Reason/ProviderStatus` 不包含原始 provider raw error 敏感片段。

5. MQ secret 隔离：
   - `mqtarget.Parse` 拒绝 userinfo、query、fragment、broker port、疑似 token/secret destination。
   - `LoadMQRuntimeConfigFromEnv` 拒绝 endpoint userinfo、password/token/secret query；允许 `authRef` 作为引用。

6. 人工导出安全元数据：
   - 创建 MANUAL_EXPORT 导出。
   - 期望结果包含 `exportId/fileFormat/rowCount/fileChecksum/operator/reason/createdAt/expiresAt/status`，且文件名不含完整用户标识或 secret。

7. 备份/恢复输出脱敏：
   - `Backup` 使用含密码的 PostgreSQL DSN 和失败 stderr。
   - 期望命令参数不含密码，错误输出不含 DSN password/token/secret，并生成文件 SHA-256、大小、UTC 创建时间。

8. 全量回归：
   - 执行所有 Go 测试，确认新增脱敏不会破坏活动、规则、outbox、callback、manual export、backup/restore、metrics 行为。

## Required Commands

~~~bash
docker compose up -d postgres
go test ./...
go build ./...
~~~

## Data / Smoke

- 敏感样例：`Authorization: Bearer raw-token`、`password=p@ssw0rd`、`token=abc123`、`callbackSecret=callback-secret`、`https://user:pass@example.com/path?token=query-token`。
- 导出样例：MANUAL_EXPORT grant order，operator=`ops`，reason=`daily export`，activity scope。
- 备份样例：`postgres://user:secret@localhost:54329/exmarketing?sslmode=disable&token=abc`。

## Decision

~~~text
PASS
~~~

需求：2026-05-29-security-secrets-data-export
