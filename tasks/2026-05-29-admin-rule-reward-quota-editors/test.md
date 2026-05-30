# Test

## Agent 指令

你是 Test Agent。请根据 task.md、analysis.md、review.md、test-plan.md 和 implementation.md 执行完整测试。

最低要求：

~~~bash
go test ./...
go build ./...
~~~

## Test Cases

已执行：

1. 后端奖励 Schema 边界定向测试
   - 覆盖 `POINT + RATIO`、`COUPON + LADDER`、`GIFT + RANDOM`、`DRAW_CHANCE + PROBABILITY`、`MEMBERSHIP + POOL_SPLIT` 非法矩阵拒绝。
   - 覆盖后端错误包含 `calculateType is unsupported for rewardType`。
2. 全量 Go 单元 / 集成测试
   - 覆盖活动创建、模拟、quota 预览、预算/库存/用户限制不足、幂等、并发 quota 竞争、PostgreSQL repository 集成测试等仓库既有测试。
3. 全量 Go 构建验证。
4. 前端 admin 单文件内联脚本语法验证。
5. 本地 admin 页面 smoke
   - 使用 in-memory repository 启动 `cmd/exmarketing`。
   - 验证 `/admin/` 返回 `200 OK`。
   - 验证页面包含 `规则编辑器`、`奖励编辑器`、`Quota 创建`、`最终 JSON 预览`。

未执行：

- 手工 PostgreSQL smoke SQL 未执行，因为当前测试上下文未提供 `TEST_DATABASE_URL` 或专用测试库连接串。仓库内 `go test ./...` 已执行包含 `internal/postgres` 的自动化集成测试。
- 浏览器交互级普通表单 / JSON 模式切换未使用 Playwright 执行；本轮以脚本语法检查、静态页面 smoke 和后端权威校验覆盖最低验收风险。

## Commands

~~~bash
go test ./internal/marketing -run 'TestCreateActivityCommandValidatesRewardSchemaBoundaries' -count=1
~~~

结果：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/marketing	0.312s
~~~

~~~bash
node -e "const fs=require('fs'); const html=fs.readFileSync('web/admin/index.html','utf8'); const scripts=[...html.matchAll(/<script>([\\s\\S]*?)<\\/script>/g)].map(m=>m[1]).join('\\n'); new Function(scripts); console.log('inline script syntax ok')"
~~~

结果：

~~~text
inline script syntax ok
~~~

~~~bash
go test ./...
~~~

结果摘要：

~~~text
ok  	github.com/ex-stack/ExMarketing/internal/httpapi	0.425s
ok  	github.com/ex-stack/ExMarketing/internal/marketing	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/postgres	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/reward	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/rule	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/worker	(cached)
ok  	github.com/ex-stack/ExMarketing/internal/workflow	0.412s
~~~

~~~bash
go build ./...
~~~

结果：退出码 0，无错误输出。

~~~bash
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -fsS -I http://127.0.0.1:18080/admin/
curl -fsS http://127.0.0.1:18080/admin/ | rg -n "规则编辑器|奖励编辑器|Quota 创建|最终 JSON 预览"
~~~

结果摘要：

~~~text
HTTP/1.1 200 OK
Content-Type: text/html; charset=utf-8

1373:          <h3 class="panel-title" style="margin-top:14px">规则编辑器</h3>
1438:          <h3 class="panel-title" style="margin-top:14px">奖励编辑器</h3>
1511:              <label>最终 JSON 预览</label>
1532:          <div class="panel-head"><h3 class="panel-title">Quota 创建</h3></div>
~~~

服务已用 `Ctrl-C` 停止，日志显示 `shutdown requested`。

## Result

~~~text
PASS
~~~

## Evidence

- 必跑命令 `go test ./...` 通过。
- 必跑命令 `go build ./...` 通过。
- 定向奖励矩阵测试通过，确认非金额奖励非 `FIXED` 组合被后端拒绝。
- 前端内联脚本可被 JavaScript parser 成功解析。
- admin 页面可由本地服务返回，并包含本任务要求的规则、奖励、quota、最终 JSON 预览入口。
- 未发现需要打回 Dev Agent 的失败命令或失败日志。

需求：2026-05-29-admin-rule-reward-quota-editors
