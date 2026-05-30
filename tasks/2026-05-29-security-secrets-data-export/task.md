# Task

## 标题

security-secrets-data-export: 密钥敏感数据与导出安全审计

## 需求 ID

2026-05-29-security-secrets-data-export

## 背景

系统会处理 broker 凭据、provider authRef、callback secret、导出文件、备份文件和错误日志。若 secret 或敏感数据进入 activity config、outbox payload、日志、审计或导出文件，会带来生产级安全风险。

## 目标

- 审计 secret 和敏感数据流向。
- 确保 secret 只通过运行时配置或安全引用使用，不落业务配置和日志。
- 导出文件、备份文件具备访问控制、过期、校验和审计。
- 错误响应和审计 payload 脱敏。

## 范围

- Secret：MQ credential、Fact Provider authRef、Webhook secret、Callback secret。
- 数据：subject、userId、IP、userAgent、provider response、导出文件、备份文件。
- 日志：errorMessage、request/response 摘要、outbox payload、audit payload。
- 导出：文件名、checksum、operator、expiresAt、download audit。
- 备份：路径、权限、保留周期、恢复审计。

## 交付物

- 敏感数据清单和流向文档。
- 脱敏规则和测试。
- 导出/备份安全要求。
- 发现问题的修复或后续任务。

## 非目标

- 不实现完整文件权限系统。
- 不接入对象存储。
- 不做数据分级平台。

## 风险等级

高

## 验收标准

- secret 不出现在 activity config、outbox payload、audit payload、日志。
- 错误响应不暴露 token/header/secret/provider 原始响应。
- 导出文件和备份文件有 checksum、operator、时间和审计。
- 敏感字段脱敏有测试覆盖。
- `go test ./...` 通过。
