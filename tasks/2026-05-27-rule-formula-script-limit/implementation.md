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

本轮按默认最小可验收口径交付 `FORMULA` 可运行执行器，继续拒绝未注册的 `LIMIT` / `SCRIPT` evaluator。

已确认 `review.md` 为 PASS。`test.md` 和 `commit.md` 没有实际 FAIL / BLOCK 原因，只有模板占位，因此没有可优先修复的失败上下文。

## Changed Files

- `internal/ruleexpr/formula.go`
  - 新增受控公式 parser / AST / evaluator。
  - 支持 Decimal 字面量、boolean / string 字面量、字段引用、`+ - * /`、括号和一元负号。
  - 字段引用复用白名单：标准事件字段、`attributes.*`、`facts.*`、`state.*`、已绑定插件 `state.plugin.<pluginCode>.*`。
  - 不支持函数调用、SQL、HTTP、文件、环境变量、随机数、当前时间、赋值或任意脚本语义。
- `internal/marketing/model.go`
  - 创建 / 上线前校验允许安全 `FORMULA` 规则。
  - `FORMULA` 校验改为 parser / AST 白名单，不再依赖字符串 token contains。
  - 本期只开放 `DECIMAL` 和 `BOOLEAN` resultType；`INTEGER` / `STRING` 结构化拒绝。
  - `LIMIT` / `SCRIPT` 仍在 Schema 校验后按 evaluator 未注册拒绝。
- `internal/rule/evaluator.go`
  - 默认 registry 注册 `FORMULA` evaluator。
  - `FORMULA` 运行时结构化输出 `formula`、`fields`、`actual`、`expected` 和原因码。
  - 类型不匹配、非法公式、除以 0、历史非法配置不默认通过。
- `internal/rule/evaluator_test.go`
  - 覆盖 Decimal 公式通过 / 拒绝、boolean 缺省 compare、运行时不安全公式、类型错误和除以 0。
- `internal/marketing/prelaunch_validation_test.go`
  - 覆盖创建校验接受安全 `FORMULA`。
  - 覆盖非法字段、函数调用、缺失 compare、未支持 resultType、boolean 非法 op、scale 越界。
- `docs/api.md`
  - 更新普通活动规则能力说明：`CONDITION` + 安全 `FORMULA` 已注册，`LIMIT` / `SCRIPT` 仍未注册。
- `docs/data-model.md`
  - 更新 `FORMULA` 已实现能力和安全边界。

## Behavior Changes

- 安全 `FORMULA` 规则现在可以创建、上线前校验并参与规则执行。
- `FORMULA` Decimal 公式使用项目内 Decimal 类型计算，不使用 float。
- `FORMULA` boolean 公式缺省 `compare` 时以结果为 `true` 通过、`false` 拒绝。
- 非标准顶层字段、函数调用、未支持 resultType、非法 op、除以 0、类型不匹配都不会默认通过。
- `SCRIPT` 和高级 `LIMIT` 仍未开放执行能力。

## Notes

基础验证：

~~~bash
go test ./...
go build ./...
~~~

`go test ./...` 已通过；`go build ./...` 在最终交付前重新执行。
最终结果：两条命令均已通过。

需求：2026-05-27-rule-formula-script-limit
