# Analysis

## 需求 ID

2026-05-24-113929-2

## 标题

活动展示与入口配置

## Review 反馈处理

- `review.md` 当前只有模板占位：`P0:`、`P1:`、`P2:` 未填写具体问题，`Open Questions` 为“待填写”，`Decision` 为模板 `PASS / BLOCK`，未形成实质打回意见。
- 本轮仍按返工处理，因为 review 文件存在且包含模板字段；处理方式是补齐完整需求说明、边界条件、风险和验收标准，供 Review Agent 重新审核。

## Analysis 自检修正

- 上一轮 `analysis.md` 为模板占位，`现状/目标理解/方案/改动范围/风险点/验收方式` 均未填写，无法进入研发；本轮已补齐为结构化需求。
- 上一轮 `Score = 0`，低于 80，按规则必须 BLOCK；本轮基于 `task.md`、`requirements/backlog.json`、`README.md`、`CLAUDE.md`、`docs/architecture.md`、`docs/api.md`、`docs/data-model.md`、`docs/roadmap.md`、`docs/workflow.md` 和 `act-analysis/22-独立活动营销中台设计.md` 重新评分。
- 上一轮 `Decision` 仍为模板 `PASS / BLOCK`，未给结论；本轮给出明确 `PASS`，原因是需求可按现有 `marketing_activity.config_json` 扩展点和现有活动创建/更新/查询边界落地，不需要等待额外业务决策。
- 上一轮未说明“引擎只使用通用字段，不绑定具体业务页面”的口径；本轮明确限定为通用展示、入口、渠道、租户和人群入口配置，不引入业务页面 ID、外部业务模型或专用活动 controller。

## 背景目标

ExMarketing 是独立活动营销中台，只接收标准事件、判断活动配置、生成参与记录和发奖指令，不依赖具体业务系统。当前活动主表已有 `config_json` 扩展字段，后台活动创建/更新接口也已声明支持 `config/configJson/config_json`。

本需求来自“22-独立活动营销中台设计”：后台不应为每个活动单独堆页面，应拆成通用配置组件，覆盖活动页面展示配置、活动入口配置、参与人群配置等能力。目标是把活动展示与入口相关的通用配置纳入活动配置模型，使活动后台能够配置“是否展示、展示内容、入口渠道、入口范围、租户范围、人群入口”等字段；引擎和模拟运行只读取通用字段，不绑定任何具体业务页面或业务系统模型。

## 参与方

- 运营/活动配置人员：在后台创建或更新活动时维护展示和入口配置。
- ExMarketing Admin API：接收、校验、保存、查询活动配置。
- ExMarketing Engine / Simulation：在活动匹配阶段读取通用入口字段，决定活动是否可作为候选，并输出可解释原因。
- 外部业务系统：只通过标准事件传入 `tenantId`、`eventType`、`sourceSystem`、`subject`、`biz`、`attributes`；不要求提供页面模型或业务库数据。
- 前台/业务页面消费者：未来可读取活动查询结果中的展示与入口字段进行页面渲染，但本需求不实现具体前台页面。

## 输入输出

### 输入

活动创建和更新请求中的活动主配置扩展对象，统一使用 `config` / `configJson` / `config_json` 承载。最小业务口径如下：

```json
{
  "display": {
    "visible": true,
    "title": "首存赠送",
    "subtitle": "首笔充值享奖励",
    "description": "活动说明",
    "bannerUrl": "https://example.com/banner.png",
    "sort": 100,
    "tags": ["new_user", "deposit"]
  },
  "entry": {
    "enabled": true,
    "entryCodes": ["HOME_BANNER", "PROMOTION_CENTER"],
    "channels": ["APP", "WEB"],
    "sourceSystems": ["payment"],
    "tenantIds": ["site_001"],
    "audience": {
      "subjectTypes": ["USER"],
      "segments": ["new_user"],
      "riskLevels": ["NORMAL"]
    }
  }
}
```

字段说明：

- `display.visible`：是否允许对外展示。未配置时默认为不额外限制展示，保持历史活动兼容。
- `display.title/subtitle/description/bannerUrl/tags/sort`：展示字段，仅用于后台和未来前台消费，不参与奖励计算。
- `entry.enabled`：入口是否启用。未配置时默认为不额外限制入口，保持历史活动兼容。
- `entry.entryCodes`：通用入口编码，如首页 banner、活动中心、弹窗等；只能是字符串数组，不代表具体页面实现。
- `entry.channels`：通用渠道编码，如 `APP`、`WEB`、`H5`、`PC`；渠道值来自事件 `attributes.channel` 或模拟运行样例。
- `entry.sourceSystems`：允许触发入口的事件来源系统，对应标准事件 `sourceSystem`。
- `entry.tenantIds`：允许展示/入口的租户列表；必须与活动自身 `tenantId` 一致或为空。不同租户活动仍通过活动主字段 `tenantId` 隔离。
- `entry.audience.subjectTypes`：允许的标准主体类型，对应事件 `subject.type`。
- `entry.audience.segments/riskLevels`：通用人群入口标签，只能通过事件 `attributes.segments`、`attributes.segment`、`attributes.riskLevel` 或后续 Fact/规则能力提供；本需求不实时查询外部用户画像。

### 输出

- 创建、详情查询、列表查询、更新活动时，返回保存后的 `configJson` / `config_json`，其中包含展示与入口配置。
- 模拟运行或事件处理进入活动匹配时，如果因入口配置未命中，应能输出结构化拒绝原因，例如入口关闭、渠道不匹配、sourceSystem 不匹配、subjectType 不匹配、租户不匹配。
- 展示字段不应影响发奖金额、预算扣减、grant_order、outbox 或 callback。

## 业务规则

1. 活动身份和租户隔离仍以活动主字段 `tenantId`、`activityCode`、`activityType` 为准；`entry.tenantIds` 只作为入口范围配置，不能扩大活动租户边界。
2. `config_json` 是本需求首选承载方式；如后续拆独立配置结构，字段语义必须与本分析保持一致，并继续从活动查询结果中可见。
3. 创建和更新活动时必须校验展示与入口配置的 JSON 类型：
   - `display`、`entry`、`audience` 必须是 object。
   - `visible`、`enabled` 必须是 boolean。
   - `sort` 必须是整数。
   - `title/subtitle/description/bannerUrl` 必须是字符串。
   - `entryCodes/channels/sourceSystems/tenantIds/subjectTypes/segments/riskLevels/tags` 必须是字符串数组。
4. 未知顶层业务字段应按现有活动配置校验策略拒绝或明确保留为扩展字段；不能静默把业务页面私有字段变成引擎依赖。
5. `bannerUrl` 如配置，必须是 `http` 或 `https` URL；本需求不负责上传、裁剪、CDN 或图片安全扫描。
6. `entry.enabled = false` 时，活动不可通过入口展示和 MATCH 候选；应输出明确原因。
7. `display.visible = false` 时，活动不对展示入口返回；是否仍允许后台模拟或内部事件处理，应以 `entry.enabled` 和活动状态为准，不把“隐藏展示”误判为“活动失效”。
8. `entry.channels` 非空时，事件或模拟样例必须提供 `attributes.channel` 且命中列表；为空表示不限制渠道。
9. `entry.sourceSystems` 非空时，标准事件 `sourceSystem` 必须命中列表；为空表示不限制来源系统。
10. `entry.audience.subjectTypes` 非空时，标准事件 `subject.type` 必须命中列表；为空表示不限制主体类型。
11. `entry.audience.segments/riskLevels` 非空时，只能基于标准事件 attributes 或已有规则/facts 能力判断；当前任务不新增外部画像查询强依赖。
12. 引擎、模拟运行和后台查询不得引用具体业务页面、ACT 历史表、外部业务库或业务系统代码包。
13. 入口配置属于 MATCH 阶段的通用候选过滤；复杂用户资格仍属于 QUALIFY 规则，不在本需求中重新定义。
14. 更新活动展示/入口配置属于活动配置变更，必须递增活动 `version` 并刷新更新时间，保持命中快照可追溯。

## 边界条件

- 旧活动没有 `display` 或 `entry` 配置时，不应被破坏；默认按现有活动状态、时间、事件类型和规则继续运行。
- 配置空数组与字段缺失都表示“不限制”，但类型错误必须拒绝。
- `entry.tenantIds` 配置了与活动 `tenantId` 不一致的租户时，应拒绝配置，避免跨租户展示。
- 渠道大小写、枚举范围若项目暂无统一字典，本需求最小口径为非空字符串；后续可在后台字典能力中收敛。
- 展示字段过长、非法 URL、数组元素非字符串、重复值、嵌套对象类型错误均应返回明确 `400 Bad Request`。
- 活动处于 `DRAFT/PAUSED/ENDED` 时，入口配置不能覆盖活动状态判断。
- 活动时间未到或已过期时，入口配置不能覆盖时间判断。
- `display.visible=false` 与 `entry.enabled=true` 同时出现时，应解释为“不展示但入口逻辑仍启用”；如产品希望二者强绑定，需要另行确认。

## 异常场景

- 创建/更新活动时 `configJson.display` 不是 object：拒绝，返回明确错误。
- `entry.channels` 传入字符串而非数组：拒绝，返回明确错误。
- `entry.tenantIds` 包含其他租户：拒绝，返回明确错误。
- `bannerUrl` 为非 http(s) URL：拒绝，返回明确错误。
- 事件 `attributes.channel` 缺失但活动限制了渠道：活动不命中，输出渠道缺失或不匹配原因。
- 事件 `sourceSystem` 不在配置范围：活动不命中，输出来源系统不匹配原因。
- 事件主体类型不在配置范围：活动不命中，输出主体类型不匹配原因。
- 查询活动详情时配置为空：返回空对象或缺省配置，不能返回不可解析 JSON。
- 模拟运行使用入口限制不满足的事件样例：不写 participation、grant_order、outbox，并输出未命中原因。

## 非目标范围

- 不实现具体前台活动页、页面路由、弹窗系统、CMS、素材上传或 CDN 管理。
- 不引入 ACT 或其他业务系统的页面 ID、表结构、controller、model、job、Kafka topic。
- 不新增外部用户画像查询强依赖；复杂 VIP、注册天数、标签、风险名单等资格判断由标准事件 attributes、Fact Provider 或 QUALIFY 规则负责。
- 不实现审批流、上线前高危校验、活动导入导出、页面权限系统。
- 不改变奖励计算、预算/库存扣减、participation、grant_order、outbox、callback 的账务语义。
- 不新增 ActivityPlugin；普通展示与入口配置必须走通用活动配置。
- 不定义生产级 Admin 鉴权，本地后台仍遵守现有安全边界。

## 风险

- 配置字段边界不清会导致业务页面私有字段渗透到引擎，破坏中台独立性。缓解：只允许通用 display/entry/audience 字段，引擎只读标准事件和通用配置。
- 默认值处理不当可能影响旧活动命中。缓解：缺失 display/entry 时保持兼容，不额外限制。
- `display.visible` 与 `entry.enabled` 语义容易混淆。缓解：在文档和验收中分别定义“展示可见”和“入口可用”。
- 人群入口容易被误解为实时用户画像。缓解：本需求只定义入口标签配置和标准字段读取，不新增外部查询。
- 租户配置若允许跨 tenant 会造成隔离风险。缓解：`entry.tenantIds` 不能扩大活动主 `tenantId`。

## 验收标准

1. 后台/API 可创建包含 `display`、`entry`、`audience` 的活动配置，查询活动列表和详情能返回同一配置。
2. 后台/API 可更新展示与入口配置，成功后活动 `version` 递增，`updatedAt` 刷新。
3. 非法配置会被拒绝并返回明确错误：类型错误、非法 URL、跨租户 `entry.tenantIds`、数组元素类型错误至少覆盖。
4. 未配置展示/入口字段的历史活动仍可按原有活动状态、时间、事件类型和规则运行。
5. 模拟运行或事件处理在入口关闭、渠道不匹配、来源系统不匹配、主体类型不匹配时，不把活动作为可发奖候选，并输出可解释原因。
6. `display` 展示字段不会影响奖励金额、quota 扣减、grant_order、outbox 或 callback。
7. 引擎和模拟运行只读取活动通用配置、标准事件字段和 attributes/facts/state，不引用具体业务页面、外部业务库或业务系统代码。
8. 文档需要补充活动展示与入口配置字段说明、示例请求和默认值语义。
9. 测试需覆盖创建、更新、查询、非法配置、默认兼容、入口匹配失败原因；最低仍需通过 `go test ./...` 和 `go build ./...`。

## 待确认问题

- 是否需要全局统一渠道枚举（例如只允许 `APP/WEB/H5/PC`），还是本阶段接受任意非空字符串。当前最小口径：接受任意非空字符串，后续字典化。
- `display.visible=false` 且 `entry.enabled=true` 是否允许内部事件继续命中。当前最小口径：允许，二者语义分离；如产品要求隐藏即不可参与，需要另行变更。
- 活动列表查询是否需要按入口、渠道、可见性筛选。当前最小口径：只保证配置可保存和返回，不新增筛选接口。

## Score

```text
需求清晰度        18/20
范围可控          19/20
风险可控          18/20
验收标准明确      18/20
架构一致性        19/20
总分              92/100
```

## Decision

```text
PASS
```

理由：需求已限定为通用活动展示与入口配置，能基于现有 `config_json`、活动创建/更新/查询、标准事件和 MATCH 阶段语义落地；剩余问题不阻塞最小可验收范围。
