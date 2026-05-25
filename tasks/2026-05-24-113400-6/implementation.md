# Implementation

## Agent 指令

你是 Dev Agent。只有 review.md 为 PASS 且 test-plan.md 为 PASS 时才能开发。

必须按 task.md、analysis.md、review.md、test-plan.md 执行，不得扩大范围。

完成开发后必须先做基础验证：

~~~bash
go test ./...
go build ./...
~~~

## Summary

已完成发奖回调与预算确认释放的最小闭环实现。

本次实现将 quota 从发奖创建阶段的直接 `used_value` 消耗调整为 `frozen_value` 冻结，并新增 grant_order 级 `marketing_quota_reservation` 等价凭证。每笔 grant_order 可反查自身预算和库存 reservation；回调只能终结该单的 `FROZEN` reservation。

## Changed Files

- `internal/marketing/model.go`
- `internal/marketing/repository.go`
- `internal/marketing/memory_repository.go`
- `internal/postgres/repository.go`
- `internal/engine/service.go`
- `internal/httpapi/handler.go`
- `internal/httpapi/handler_test.go`
- `internal/marketing/memory_repository_test.go`
- `internal/worker/outbox_worker_test.go`
- `db/migration/001_marketing_core.sql`
- `docs/api.md`

## Behavior Changes

- `POST /openapi/v1/reward/callback` 按 `tenantId + commandId` 定位 grant_order。
- `SUCCESS` 首次回调将该单 reservation 从 `FROZEN` 确认为 `CONFIRMED`，并将 quota 聚合从 `frozen_value` 转入 `used_value`。
- `FAILED` 首次回调将该单 reservation 从 `FROZEN` 释放为 `RELEASED`，并减少 quota 聚合 `frozen_value`。
- 相同最终状态的重复回调返回 `duplicate=true`，不重复确认、释放或推进 participation。
- 相反最终状态、`externalGrantId` 冲突、缺少 reservation、reservation 已终结或 quota 聚合不一致均返回机器可识别错误。
- participation 在同一 participation 下所有 grant_order 都终结后推进：全部成功为 `GRANTED`，任一失败为 `FAILED`。
- 新增 `GET /admin/v1/reward/reconciliation`，可按 tenant 和 commandId 查询 grant_order、participation、回调结果和 quota reservation 明细。

## Verification

~~~bash
go test ./...
go build ./...
~~~

结果：PASS。

## Notes

本期未加入公网签名 / token 鉴权，保持 `analysis.md` 的范围口径：该回调接口不能作为公网生产暴露能力验收。

需求：2026-05-24-113400-6
