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

已按 `task.md`、`analysis.md`、`review.md`、`test-plan.md` 做最小实现与补测。

本轮没有发现 `test.md` 的实际 FAIL 原因，也没有发现 `commit.md` 的实际 BLOCK / FAILED 原因；两者仍是模板占位内容。因此直接进入 Review PASS 后的开发修复。

## Changed Files

- `web/admin/index.html`
  - 在活动配置页补充规则编辑器、奖励编辑器、quota 编辑器。
  - 保留并强化高级 JSON 模式，支持 rules/rewards JSON 格式化、最终 JSON 预览和人类可读摘要。
  - 前端保存前校验补充 `rewardType × calculateType` 矩阵：`POINT / COUPON / GIFT / DRAW_CHANCE / MEMBERSHIP` 只能选择 `FIXED`，非法组合定位到 `rewards[n].calculateType`。
  - Quota 编辑器支持 `BUDGET / STOCK / USER_LIMIT / DAILY_LIMIT` 默认 scope/scopeKey 生成，并在预览中把缺失 quota 表达为 `unlimited`。
- `internal/marketing/prelaunch_validation_test.go`
  - 增加参数化测试覆盖 `POINT + RATIO`、`COUPON + LADDER`、`GIFT + RANDOM`、`DRAW_CHANCE + PROBABILITY`、`MEMBERSHIP + POOL_SPLIT`。
  - 断言后端返回 `calculateType is unsupported for rewardType`，锁定后端上线前校验口径。
- `tasks/2026-05-29-admin-rule-reward-quota-editors/test-plan.md`
  - 补齐 Test Agent 可执行测试策略、用例、命令和 smoke 数据。

## Behavior Changes

- 运营可以通过普通表单生成 `CONDITION` 规则，再同步到 `activityRulesJson`。
- 运营可以通过奖励表单生成金额类和非金额类奖励；非金额类奖励不会开放非 `FIXED` 计算方式。
- 高级 JSON 模式仍是同一份数据源，创建活动时继续提交 `config`、`rules`、`rewards` 给后端权威校验。
- 预览区展示最终 `rules[]`、`rewards[]`、quota 摘要，以及规则、奖励、quota 的可读摘要。
- Quota 创建仍调用既有 `POST /admin/v1/quotas`，不声称支持修改运行态 `usedValue/frozenValue`。

## Notes

验证已完成：

~~~bash
go test ./internal/marketing -run 'TestCreateActivityCommandValidatesRewardSchemaBoundaries'
go test ./...
go build ./...
node -e "const fs=require('fs'); const html=fs.readFileSync('web/admin/index.html','utf8'); const scripts=[...html.matchAll(/<script>([\s\S]*?)<\/script>/g)].map(m=>m[1]).join('\n'); new Function(scripts); console.log('inline script syntax ok')"
EX_MARKETING_ADDR=:18080 go run ./cmd/exmarketing
curl -fsS -I http://127.0.0.1:18080/admin/
curl -fsS http://127.0.0.1:18080/admin/ | rg -n "规则编辑器|奖励编辑器|Quota 创建|最终 JSON 预览"
~~~

结果均为 PASS。

需求：2026-05-29-admin-rule-reward-quota-editors
