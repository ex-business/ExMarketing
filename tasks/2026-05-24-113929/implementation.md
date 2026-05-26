# Implementation

## Summary

按返工反馈完成活动主配置最小修复：PostgreSQL `CreateActivity` 在同租户重复 `activityCode` 触发唯一约束时，返回项目领域错误 `marketing.ErrActivityConflict`，与 memory repository 和 HTTP 409 语义保持一致。

## Changed Files

- `internal/postgres/repository.go`
- `tasks/2026-05-24-113929/implementation.md`

## Behavior Changes

- PostgreSQL activity insert errors from `marketing_activity_tenant_code_key` are now mapped through `mapPostgresActivityError`.
- Duplicate `(tenant_id, activity_code)` on PostgreSQL-backed `CreateActivity` now satisfies `errors.Is(err, marketing.ErrActivityConflict)`.
- No schema, API, model, or unrelated behavior was changed in this rework patch.

## Verification

```bash
go test ./...
go build ./...
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -run '^TestPostgresActivityUniqueDetailAndUpdate$' -v
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1 -run 'Postgres|PostgreSQL|Smoke|Activity|Repository|Migration|Quota|Budget|Insufficient|Limit|Stock' -v
```

All commands passed.

## Notes

Existing unrelated worktree changes were left untouched. The fix is intentionally limited to the PostgreSQL duplicate activity error mapping reported by Test Agent.

需求：2026-05-24-113929
