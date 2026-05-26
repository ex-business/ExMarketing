# Review

## Agent 指令

你是 Review Agent。请审核 analysis.md。

不改代码，不提交。

## Findings

```text
P0:
- 无。

P1:
- 无。

P2:
- analysis.md:264、379 将 JSON 解析/INVALID_JSON 作为签名校验前也可能发生的边界，但 Webhook 是公网接入，建议实现阶段明确校验顺序为：size / Content-Type / receiver 定位 / timestamp+signature / JSON parse / mapping。这样非法或未签名 payload 不会先触发业务 JSON 解析。当前 body 上限为 1 MiB，且 analysis.md 已说明多错误优先级非目标，因此不构成阻塞。
- analysis.md:182、270-273、387-391 允许外部 `Idempotency-Key` 可选，同时要求复用既有标准事件链路。Dev 实现时需要显式把映射后的派生 key 作为内部 event inbox idempotency key 传入既有链路，避免误按 `/openapi/v1/events` 的“Header 必填”语义拒绝不带 header 的合法 Webhook。analysis 已定义派生 key 和重复/冲突语义，属于实现澄清，不阻塞。
- analysis.md:168、318 把 `allowedIPs` 为空定义为不做 IP 限制，并把代理可信链列为非目标。该口径满足最小 SaaS receiver，但生产公网部署前仍需要单独任务补齐可信反代 IP 提取、限流和指标告警，否则来源校验主要依赖 HMAC。
- analysis.md:376 要求 receiverCode 缺失返回 `WEBHOOK_RECEIVER_NOT_FOUND`，但路由 `POST /openapi/v1/webhooks/{receiverCode}` 下“缺失”通常会落到不同路径。Dev/Test 需要明确覆盖 `/openapi/v1/webhooks/` 或空 path segment 的响应是否由路由层统一映射；这不影响有 receiverCode 的核心接入协议。
```

## Open Questions

无阻塞性 Open Questions。以下作为后续生产化任务即可：

- receiver secret 是否需要强制全局唯一，或将 receiverCode 纳入签名串以降低误复用 secret 的跨 receiver 重放风险。
- 是否需要双 secret 轮换、密钥加密存储、receiver 管理 API / UI。
- 是否需要定义多错误同时存在时的全局响应优先级。

## Score

```text
需求清晰度        18/20
范围可控          18/20
风险可控          16/20
验收标准明确      18/20
架构一致性        19/20
合计              89/100
```

扣分说明：当前 analysis 已用默认最小协议关闭上一轮阻塞点，核心安全、映射、幂等、审计和验收口径可实现、可测试。扣分主要来自公网 Webhook 的生产化能力仍较薄：IP 可信链、secret 轮换、限流/指标和多错误优先级都被放到后续任务。

## Decision

```text
PASS
```

规则检查：

```text
无 P0/P1，Score 89 >= 80，因此 PASS。
```

需求：2026-05-24-webhook-receiver
