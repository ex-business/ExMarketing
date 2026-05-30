# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发或修复。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。
如果 test.md 已存在且 Result 为 FAIL，必须优先读取失败原因，做最小修复后再更新 implementation.md。
如果 commit.md 或提交日志存在 BLOCK / FAILED / 提交失败，必须优先读取提交失败上下文，做最小修复后再更新 implementation.md。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

- 新增统一敏感数据脱敏工具，覆盖嵌套 JSON、header、URL/DSN userinfo、敏感 query、`password/token/secret/signature/credential/clientKey/privateKey/Authorization/Cookie` 等 key/value。
- 服务层审计写入前统一调用脱敏，避免 audit payload 持久化 secret、token、Authorization、provider raw error。
- HTTP 错误响应统一脱敏，避免 caller 侧错误 body 回显敏感片段。
- Fact provider 失败响应统一脱敏，避免 provider 原始错误摘要进入规则解释、响应或后续审计。
- 人工导出文件模型补充 `expiresAt`，memory/postgres 创建导出时设置默认 24h 过期时间；导出创建审计包含过期时间。
- 补齐本需求 test-plan，明确 Test Agent 后续测试样例和命令。

## Changed Files

- `internal/marketing/redact.go`
- `internal/marketing/redact_test.go`
- `internal/marketing/model.go`
- `internal/marketing/memory_repository.go`
- `internal/marketing/memory_repository_test.go`
- `internal/engine/service.go`
- `internal/engine/security_test.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/security_test.go`
- `internal/fact/client.go`
- `internal/fact/client_test.go`
- `internal/postgres/repository.go`
- `tasks/2026-05-29-security-secrets-data-export/test-plan.md`
- `tasks/2026-05-29-security-secrets-data-export/implementation.md`

## Behavior Changes

- `AuditRecord.Payload` 中的敏感 key/value 会在入库前替换为 `[redacted]`。
- HTTP 错误响应中的敏感片段会替换为 `[redacted]`。
- Fact Query provider 失败时，`Error`、`Reason`、`ProviderStatus` 不再回显 Authorization/token/password/secret 等原始内容。
- Manual grant export 返回和审计中的 export file 元数据包含 `expiresAt`，默认 `createdAt + 24h`。

## Verification

~~~bash
docker compose up -d postgres
go test ./...
go build ./...
~~~

结果：

~~~text
PASS
PASS
~~~

说明：

- 首次 `go test ./...` 失败于本地 PostgreSQL 未启动和 DSN userinfo 脱敏漏测；已启动 `docker compose` postgres，并补齐长错误文本中嵌入 DSN 的 userinfo 脱敏后重跑通过。
- `go build ./...` 通过。

## Notes

- 本期未实现完整文件权限系统、对象存储、下载接口或 KMS，保持 task.md 非目标范围。
- 当前导出过期时间为代码默认 24h；若后续需要租户级或环境级配置，可另立 hardening 任务。

需求：2026-05-29-security-secrets-data-export
