# Task

## 标题

security-threat-model: 正式安全威胁建模

## 需求 ID

2026-05-29-security-threat-model

## 背景

ExMarketing 连接外部事件、Fact Provider、MQ、Webhook、发奖系统、导出文件和管理后台，存在多条信任边界。正式上线前需要先做威胁建模，明确资产、入口、攻击面、已有控制和缺口。

## 目标

- 建立系统级威胁模型。
- 覆盖 OpenAPI、Admin、Webhook Receiver、Reward Callback、Fact Provider、MQ、Outbox、DLQ、导出/备份文件。
- 给出风险优先级和后续补救任务。
- 形成发布前安全审计基线。

## 范围

- 资产：活动配置、quota、grant_order、outbox、audit、fact provider secret、MQ credential、导出文件、备份。
- 信任边界：公网入口、内网服务、DB、MQ、外部发奖系统、管理后台用户。
- 威胁：伪造事件、重放、越权、跨 tenant、SSRF、secret 泄漏、日志敏感信息、DLQ 重放滥用。
- 控制：鉴权、签名、nonce/timestamp、tenant guard、allowlist、脱敏、审计、权限。
- 输出：风险矩阵和补救任务列表。

## 交付物

- 威胁模型文档。
- 数据流/信任边界图。
- 风险矩阵：严重度、可能性、影响、现有控制、缺口。
- 后续安全任务清单。

## 非目标

- 不直接实现所有安全修复。
- 不替代外部渗透测试。
- 不做公司级 IAM 设计。

## 风险等级

高

## 验收标准

- 每个外部入口都有威胁分析。
- 每个高危资产都有保护措施或明确缺口。
- P0/P1 风险必须生成可执行后续任务。
- 文档可作为发布安全 review 输入。
