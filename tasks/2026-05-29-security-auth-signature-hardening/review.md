# Review

## Scope

Reviewed `tasks/2026-05-29-security-auth-signature-hardening/analysis.md` against:

- `tasks/2026-05-29-security-auth-signature-hardening/task.md`
- `docs/api.md`
- `docs/operations.md`
- `docs/security-threat-model.md`
- `docs/activityplugin-lifecycle.md`

No code changes were made.

## Findings

```text
P0:
- None.

P1:
- None.

P2:
- analysis.md:32 uses "校验签名或等价 token" for Reward Callback. The detailed section later standardizes on tenant-scoped timestamp + HMAC signature, and docs/api.md also documents HMAC for configured callback secrets. Dev/Test should treat HMAC as the acceptance path unless a future protocol explicitly defines the token alternative.
- analysis.md:155-158 requires business writes to fail closed when security audit writes fail. This is defensible for high-risk entries, but only Webhook currently has an explicit `WEBHOOK_AUDIT_FAILED` contract. Test Plan should either define equivalent expected errors per entry or mark non-Webhook audit-write-failure checks as manual/follow-up, so implementation does not invent inconsistent behavior.
```

## Score

```text
92 / 100
```

Rationale: the analysis is complete enough for Test Plan and Dev to proceed. It covers the required entry points, tenant guard rules, replay posture, negative cases, stable error codes, audit redaction, and production release risks. The remaining issues are wording/precision problems, not blockers.

## Open Questions

- Should Reward Callback allow any non-HMAC "equivalent token" mode in this task, or should the task explicitly lock acceptance to timestamp + HMAC only?
- For non-Webhook entries, what exact error contract should be used when security audit persistence fails before business mutation?

## Decision

```text
PASS
```

No P0/P1 findings and Score is >= 80, so this review does not block the pipeline.

需求：2026-05-29-security-auth-signature-hardening
