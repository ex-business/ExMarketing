# Commit

## Scope

- Implemented built-in `RED_ENVELOPE` and `LOTTERY` ActivityPlugin samples.
- Added plugin config validation, deterministic audit candidate output, and state-change intents.
- Registered the plugins in the engine and persisted candidate plugin results into participation snapshots.
- Added sample documentation and task evidence files.
- Excluded `tasks/*/runs/` execution logs from the commit.

## Gates

```text
review.md: PASS
test.md: PASS
```

## Pre-Commit Checks

```text
git status -sb: PASS
git diff --check: PASS
go test ./...: PASS
go build ./...: PASS
```

## Commit

```text
hash: f415d3bd3374f21e671074963837a456896848ec
message: Add red envelope and lottery activity plugins
remote: not pushed
```

需求：2026-05-29-activityplugin-red-envelope-lottery
