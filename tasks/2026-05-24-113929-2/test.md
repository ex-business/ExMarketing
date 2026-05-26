# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

本轮只写入本文件，未修改业务代码、测试代码、文档代码、配置文件或其他文件。

## Test Cases

- 创建活动展示/入口配置并读回：覆盖 `display.visible/title/subtitle/description/bannerUrl/sort/tags`、`entry.enabled/entryCodes/channels/sourceSystems/tenantIds/audience`。
- 非法配置拒绝：覆盖 `display` 非 object、`entry.enabled` 非 boolean、`entry.channels` 非字符串数组、跨租户 `entry.tenantIds`、非法 `display.bannerUrl`。
- `entry.enabled=false`：模拟运行和正式事件处理均返回 `ENTRY_DISABLED`，且无 grant/outbox/participation 副作用。
- 入口不匹配原因：覆盖 channel、sourceSystem、subjectType 不匹配，对应 `ENTRY_CHANNEL_NOT_ALLOWED`、`ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`、`ENTRY_SUBJECT_TYPE_NOT_ALLOWED`。
- `display.visible=false` 且入口可用：正式事件仍可生成奖励，验证展示隐藏不等于活动失效。
- 文档语义检查：`docs/api.md` 包含 `display.visible=false`、`entry.enabled=false`、`ENTRY_DISABLED`、`entry.sourceSystems`、`entry.tenantIds` 的说明。
- 全量回归：执行 `go test ./...`。
- 构建验证：执行 `go build ./...`。

## Commands

```bash
go test ./...
```

结果：

```text
?   	github.com/ex-stack/ExMarketing/cmd/exmarketing	[no test files]
?   	github.com/ex-stack/ExMarketing/cmd/workflowctl	[no test files]
?   	github.com/ex-stack/ExMarketing/internal/decimal	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/engine	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/fact	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.603s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/mqconsumer	(cached)
?   	github.com/ex-stack/ExMarketing/internal/plugin	[no test files]
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.553s
```

```bash
go build ./...
```

结果：通过，无输出。

```bash
go test ./internal/httpapi -run 'Test(CreateActivityValidatesDisplayEntryAudienceConfig|EntryConfigFiltersSimulationAndProcessEvent|EntryConfigMismatchReasons|DisplayVisibleFalseDoesNotBlockEntryMatching)' -v
```

结果：

```text
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig/display_object
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig/enabled_bool
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig/entry_array
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig/tenant_scope
=== RUN   TestCreateActivityValidatesDisplayEntryAudienceConfig/banner_url
--- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig (0.00s)
    --- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig/display_object (0.00s)
    --- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig/enabled_bool (0.00s)
    --- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig/entry_array (0.00s)
    --- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig/tenant_scope (0.00s)
    --- PASS: TestCreateActivityValidatesDisplayEntryAudienceConfig/banner_url (0.00s)
=== RUN   TestEntryConfigFiltersSimulationAndProcessEvent
--- PASS: TestEntryConfigFiltersSimulationAndProcessEvent (0.00s)
=== RUN   TestEntryConfigMismatchReasons
=== RUN   TestEntryConfigMismatchReasons/channel
=== RUN   TestEntryConfigMismatchReasons/source_system
=== RUN   TestEntryConfigMismatchReasons/subject_type
--- PASS: TestEntryConfigMismatchReasons (0.00s)
    --- PASS: TestEntryConfigMismatchReasons/channel (0.00s)
    --- PASS: TestEntryConfigMismatchReasons/source_system (0.00s)
    --- PASS: TestEntryConfigMismatchReasons/subject_type (0.00s)
=== RUN   TestDisplayVisibleFalseDoesNotBlockEntryMatching
--- PASS: TestDisplayVisibleFalseDoesNotBlockEntryMatching (0.00s)
PASS
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.224s
```

```bash
rg -n "活动展示与入口配置|ENTRY_DISABLED|display.visible=false|entry.enabled=false|entry.sourceSystems|entry.tenantIds" docs/api.md
```

结果摘要：

```text
280:- `display.visible=false` 只表示不对展示入口可见，不会让内部事件处理或模拟运行失效。
281:- `entry.enabled=false` 会在 MATCH 阶段拒绝该活动候选，并返回 `ENTRY_DISABLED`。
283:- `entry.sourceSystems` 读取标准事件 `sourceSystem`；非空且不命中时返回 `ENTRY_SOURCE_SYSTEM_NOT_ALLOWED`。
284:- `entry.tenantIds` 只能为空或包含活动自身 `tenantId`，不能扩大租户边界；运行时非空且不命中返回 `ENTRY_TENANT_NOT_ALLOWED`。
```

## Result

```text
PASS
```

## Evidence

- `go test ./...` 通过，覆盖全仓库 Go 测试。
- `go build ./...` 通过，覆盖全仓库 Go 构建。
- 需求相关定向用例全部通过：
  - `TestCreateActivityValidatesDisplayEntryAudienceConfig`
  - `TestEntryConfigFiltersSimulationAndProcessEvent`
  - `TestEntryConfigMismatchReasons`
  - `TestDisplayVisibleFalseDoesNotBlockEntryMatching`
- 文档关键语义已检查，`docs/api.md` 包含展示可见性、入口启用、入口拒绝原因、来源系统和租户边界说明。
- 本轮未发现失败命令，无需打回 Dev Agent。

需求：2026-05-24-113929-2
