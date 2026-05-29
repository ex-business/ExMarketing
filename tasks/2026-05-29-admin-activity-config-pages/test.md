# Test

## Scope

依据 `task.md`、`analysis.md`、`review.md`、`test-plan.md`、`implementation.md` 执行 Test Agent 验证。

本轮只修改本文件，未修改业务代码、测试代码、配置文件或其他文档。

## Commands

~~~bash
go test ./...
~~~

结果：PASS

摘要：

~~~text
ok github.com/ex-stack/ExMarketing/internal/httpapi 0.473s
ok github.com/ex-stack/ExMarketing/internal/workflow 0.493s
其他包通过或无测试文件
~~~

~~~bash
go build ./...
~~~

结果：PASS

~~~bash
node <<'NODE'
const fs = require('fs');
const html = fs.readFileSync('web/admin/index.html', 'utf8');
const scripts = [...html.matchAll(/<script[^>]*>([\s\S]*?)<\/script>/gi)].map(m => m[1]);
for (const [i, script] of scripts.entries()) {
  new Function(script);
  console.log(`script ${i + 1} syntax ok (${script.length} chars)`);
}
NODE
~~~

结果：PASS

摘要：

~~~text
script 1 syntax ok (84378 chars)
~~~

~~~bash
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
~~~

结果：PASS，服务使用 in-memory repository 启动成功。

随后执行 Node HTTP smoke，覆盖：

- `GET /healthz`
- `GET /admin/` 页面内容标记检查：活动配置、筛选、创建、编辑、复制、模拟入口、上线前校验、审批记录
- `POST /admin/v1/activities` 创建普通配置化活动
- `GET /admin/v1/activities` 列表读回
- `GET /admin/v1/activities/{id}` 详情读回
- `PUT /admin/v1/activities/{id}` 编辑并验证 `version` 递增
- 再次详情读回，验证刷新不丢已保存配置
- 身份字段更新拒绝
- 同租户重复 `activityCode` 拒绝
- 不同租户相同 `activityCode` 允许
- 非法时间拒绝
- 非法 timezone 拒绝
- `entry.tenantIds` 越租户拒绝
- 非法 `config` 顶层字段拒绝
- 未审批直接 ACTIVE 被 `APPROVAL_REQUIRED` 阻断
- `POST /admin/v1/activities/prelaunch-validation`
- `POST /admin/v1/simulate`

结果：PASS

摘要：

~~~json
{
  "ok": true,
  "activityId": "act_00a3e83a10757e068620c9d8",
  "activityCode": "TP_ADMIN_CFG_20260529_1780070382757",
  "checks": [
    "healthz",
    "admin markers",
    "create",
    "list",
    "detail",
    "update",
    "reload",
    "identity rejection",
    "duplicate rejection",
    "different tenant duplicate allowed",
    "bad time",
    "bad timezone",
    "bad tenant scope",
    "bad config",
    "approval block",
    "prelaunch validation",
    "simulate"
  ]
}
~~~

说明：第一次 smoke 直接使用 `test-plan.md` 的空 `rules/rewards` 最小 payload，后端按既有 API 约束返回 `400 invalid activity: at least one reward is required`。该 payload 与仓库现有活动创建 API 约束不一致，因此后续 smoke 改用项目现有合法 rule/reward 结构继续验证，未判定为实现失败。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/db-migration-release --mode apply
~~~

结果：PASS

摘要：

~~~text
APPLY PASS
applied=3 pending=0
applied 001 001_marketing_core.sql success
applied 002 002_audit_fact_outbox_indexes.sql success
applied 003 003_activity_unique_constraint.sql success
~~~

~~~bash
EX_MARKETING_ADDR=:18081 EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go run ./cmd/exmarketing
~~~

结果：PASS，服务使用 postgres repository 启动成功。

随后执行 PostgreSQL repository HTTP smoke：

- 创建活动
- 编辑活动
- 详情读回确认 `activityName`、`priority`、`stackable`、`version=2`
- 模拟运行

结果：PASS

摘要：

~~~json
{
  "ok": true,
  "activityId": "act_3c199d390b99aab759b326a4",
  "activityCode": "TP_ADMIN_PG_1780070440100",
  "tenantId": "tp_admin_pg_20260529"
}
~~~

~~~bash
docker exec exmarketing-postgres psql -U exmarketing -d exmarketing \
  -c "select tenant_id, activity_code, activity_name, activity_type, status, timezone, priority, exclusive_group, stackable, version, config_json from marketing_activity where tenant_id = 'tp_admin_pg_20260529' order by updated_at desc limit 5;" \
  -c "select count(*) as participation_count from marketing_participation where tenant_id = 'tp_admin_pg_20260529';" \
  -c "select count(*) as grant_order_count from marketing_grant_order where tenant_id = 'tp_admin_pg_20260529';" \
  -c "select count(*) as outbox_count from marketing_outbox where tenant_id = 'tp_admin_pg_20260529';"
~~~

结果：PASS

摘要：

~~~text
marketing_activity:
tenant_id=tp_admin_pg_20260529
activity_code=TP_ADMIN_PG_1780070440100
activity_name=Admin Config PG Smoke Updated
activity_type=FIRST_DEPOSIT
status=DRAFT
timezone=UTC
priority=88
exclusive_group=tp_admin_pg_deposit
stackable=t
version=2
config_json={"display":{"visible":true,"title":"PG Smoke Updated"},"entry":{"enabled":true,"channels":["APP"],"sourceSystems":["payment"],"tenantIds":["tp_admin_pg_20260529"]},"metadata":{"sourceSystem":"payment"}}

participation_count=0
grant_order_count=0
outbox_count=0
~~~

宿主机 `psql` 不存在，已使用 Docker 容器内 `psql` 完成数据库核对。

## Test Cases

### 1. 创建、查看、编辑、刷新读回

结果：PASS

验证方式：

- API 创建活动成功，状态为 `DRAFT`，初始 `version=1`。
- 详情 API 能读回创建结果。
- 编辑 `activityName`、`priority`、`stackable`、`config.display.title` 成功，`version` 递增到 2。
- 再次 GET 详情可读回更新后的字段。
- PostgreSQL 查询确认落库字段与 API 展示一致。

### 2. 列表搜索、过滤、分页、操作入口

结果：PASS，覆盖静态页面标记和列表 API smoke。

验证方式：

- `/admin/` 包含活动配置页、搜索框、状态/类型/时间筛选、分页大小、创建、编辑、复制、模拟、上线前校验、审批记录等控件标记。
- `GET /admin/v1/activities` 返回创建后的活动。

限制：

- 本轮没有真实浏览器点击分页和筛选控件；以页面脚本语法检查、HTML 控件标记和列表 API smoke 覆盖最小页面 smoke。

### 3. 复制活动

结果：PASS，覆盖页面入口与后端等价创建约束。

验证方式：

- `/admin/` 包含 `prepareCopySelectedActivity` 复制入口。
- 后端验证不同租户可复用相同 `activityCode`，同租户重复 `activityCode` 被拒绝。

限制：

- 当前实现为前端复制到表单后重新创建新活动，没有独立后端 copy endpoint；本轮未执行真实浏览器点击复制按钮。

### 4. 模拟运行入口和无副作用

结果：PASS

验证方式：

- `/admin/` 包含 `openSimulationForSelectedActivity` 和 `POST /admin/v1/simulate` 模拟入口标记。
- in-memory 与 PostgreSQL repository 下 `POST /admin/v1/simulate` 均返回 200。
- PostgreSQL 查询确认模拟后 `marketing_participation`、`marketing_grant_order`、`marketing_outbox` 对测试租户计数均为 0。

### 5. 非法时间、非法 timezone、非法 JSON/config

结果：PASS

验证方式：

- `startTime >= endTime` 返回 400。
- `timezone=Mars/Phobos` 返回 400。
- `entry.tenantIds` 越租户返回 400。
- `config` 顶层加入非法 `rules` 返回 400。
- 页面脚本语法检查通过，包含保存前校验相关脚本。

### 6. 重复 activityCode

结果：PASS

验证方式：

- 同一 `tenantId + activityCode` 重复创建返回 409。
- 不同 `tenantId` 使用相同 `activityCode` 创建成功。

### 7. 审批、版本和状态约束

结果：PASS

验证方式：

- 未审批活动直接更新为 `ACTIVE` 被拒绝，返回 `APPROVAL_REQUIRED: activity version is not approved`。
- 页面包含上线前校验、提交审批、审批通过、审批驳回、审批记录入口标记。

### 8. Quota Insufficient

结果：部分覆盖。

验证方式：

- `go test ./...` 覆盖既有 quota insufficient 和模拟不持久化相关测试。
- PostgreSQL smoke 验证模拟不写 participation、grant_order、outbox。

限制：

- 本轮未额外创建耗尽 quota 的活动做页面/API 手工 smoke。

### 9. Idempotency

结果：部分覆盖。

验证方式：

- `go test ./...` 覆盖既有事件幂等测试。
- API smoke 覆盖同租户重复 `activityCode` 不会创建第二条活动。

限制：

- 本轮未额外执行并发或相同幂等键重复请求脚本。

### 10. Concurrency

结果：未执行专项并发 smoke。

说明：

- 本需求实现主要为 `/admin/` 页面改造；当前测试已覆盖创建、编辑、重复唯一约束、审批阻断和持久化读回。
- 未发现需要打回 Dev Agent 的并发相关实现失败证据。

### 11. PostgreSQL Smoke

结果：PASS

验证方式：

- Docker PostgreSQL 已运行。
- 迁移 apply 通过。
- PostgreSQL repository 服务启动成功。
- 创建、编辑、详情读回和模拟 API 通过。
- 容器内 `psql` 查询确认活动主表字段、`version=2`、`config_json` 落库正确。
- 模拟后 participation、grant_order、outbox 均未写入。

## Result

~~~text
PASS
~~~

需求：2026-05-29-admin-activity-config-pages
