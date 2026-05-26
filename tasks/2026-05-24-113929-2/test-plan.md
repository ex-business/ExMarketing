# Test Plan

## Test Strategy

围绕 `analysis.md` 的最小口径做覆盖：活动展示与入口配置仍通过 `config/configJson/config_json` 保存和读回；创建、更新阶段校验通用 JSON 字段；引擎和模拟运行只在 MATCH 阶段读取通用入口字段和标准事件字段，不绑定具体业务页面。

重点验证：

- `display.visible` 与 `entry.enabled` 语义分离。
- `entry.channels/sourceSystems/tenantIds/audience` 命中和拒绝原因可解释。
- 未配置新入口字段的旧活动保持兼容。
- 展示字段不影响发奖、quota、grant_order、outbox。
- 非法 config 类型、非法 URL、跨租户入口、数组元素类型或重复值返回 `400 Bad Request`。

## Test Cases

1. 创建包含 `display.visible/title/subtitle/description/bannerUrl/sort/tags` 和 `entry.enabled/entryCodes/channels/sourceSystems/tenantIds/audience` 的活动，期望 `201`，查询/响应可读回配置。
2. 更新活动 `configJson`，期望成功后 `version` 递增、`updatedAt` 刷新，配置可读回。
3. 非法配置：`display` 非 object、`entry.enabled` 非 boolean、`entry.channels` 非 string array、`entry.tenantIds` 跨租户、`display.bannerUrl` 非 http(s)，期望 `400`。
4. `entry.enabled=false`：`POST /admin/v1/simulate` 返回 unmatched activity，`decisionReasonCode=ENTRY_DISABLED`，无 grant/outbox preview。
5. `entry.enabled=false`：正式事件处理返回入口拒绝 `ruleResults`，不生成 participation、grant_order、outbox。
6. `display.visible=false` 且 `entry` 可用：正式事件仍可通过规则并生成奖励，证明展示隐藏不等于活动失效。
7. `entry.channels` 非空且事件缺失或不匹配 `attributes.channel`：入口 MATCH 拒绝，返回 `ENTRY_CHANNEL_NOT_ALLOWED`。
8. 旧活动缺少 `display` 或 `entry.enabled` 时保持原有状态、时间、事件类型和规则处理。
9. 文档检查 `docs/api.md` 包含展示/入口字段、默认语义和拒绝原因。

## Required Commands

```bash
go test ./...
go build ./...
```

## Data / Smoke

使用现有内存仓库 HTTP handler 测试创建活动、模拟运行和正式事件处理；事件样例通过标准字段 `tenantId/sourceSystem/subject` 与 `attributes.channel/segments/riskLevel` 驱动入口匹配，不使用外部业务页面或外部用户画像。

## Decision

```text
PASS
```

需求：2026-05-24-113929-2
