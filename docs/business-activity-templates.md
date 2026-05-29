# Business Activity Templates

This document records the default configuration templates for `2026-05-27-business-activity-templates`. The templates are ordinary activity configurations: they use standard events, `attributes.*`, optional `facts.*`, rules, rewards, quotas, and automatic reward outbox only. They do not require an `ActivityPlugin`, a dedicated controller, a dedicated table, or an admin template marketplace.

## Common Contract

- Submit each template as a normal `POST /admin/v1/activities` payload with `status=DRAFT`.
- Put integration hints such as `sourceSystem`, `bizType`, and period metadata under `config.metadata`; do not add unknown top-level fields.
- Use only `CONDITION` and safe `FORMULA` rules. `FORMULA.ruleConfig.formula` is an arithmetic expression made from supported fields, literals, `+ - * /`, and parentheses. Comparisons such as `> 0` must be placed in `ruleConfig.compare`.
- Amount rewards use Decimal semantics and produce 4-decimal runtime results. Zero-amount money rewards do not create grant orders, outbox messages, or quota deductions.
- Repeated events are controlled by the standard idempotency key: `tenantId + ":" + sourceSystem + ":" + eventType + ":" + bizType + ":" + bizId`.
- Simulation must be run through `POST /admin/v1/simulate`; it previews rule decisions, reward output, quota impact, grant orders, and outbox commands without writing business state.

## Template Matrix

| ID | Template | Trigger event | Data source | Reward | Required quotas |
|---|---|---|---|---|---|
| T1 | Sign-in gift | `SIGN_IN_SUCCESS` | `attributes.continueDays` | `BONUS` ladder | `DAILY_LIMIT` user/day, `BUDGET` |
| T2 | Monthly deposit | `DEPOSIT_SUCCESS` | `attributes.amount`, optional monthly facts | `BONUS` ratio | `USER_LIMIT` user/month, `BUDGET` |
| T3 | Redeposit | `DEPOSIT_SUCCESS` | `attributes.depositCount`, `attributes.amount` | `BONUS` fixed or ratio | `USER_LIMIT`, `BUDGET` |
| T4 | Newbie task | `TASK_COMPLETED` | `attributes.taskCode`, `attributes.completed` | fixed non-money or fixed bonus | `USER_LIMIT`, optional `STOCK` |
| T5 | Rebate | `REBATE_SETTLED` | `attributes.validBetAmount` or settlement facts | `BONUS` ratio | `USER_LIMIT` period, `BUDGET` |
| T6 | Invite friend | `INVITEE_FIRST_DEPOSIT` | `attributes.inviteeId`, `attributes.depositAmount` | `BONUS` fixed | inviter/invitee `USER_LIMIT`, `BUDGET` |
| T7 | VIP weekly/monthly bonus | `VIP_BONUS_ELIGIBLE` | `facts.VIP.level`, period turnover attributes | `BONUS` ladder | period `USER_LIMIT`, `BUDGET` |
| T8 | Birthday bonus | `BIRTHDAY_BONUS_ELIGIBLE` | profile facts and activity attributes | `BONUS` fixed | yearly `USER_LIMIT`, `BUDGET` |
| T9 | Upgrade bonus | `VIP_LEVEL_UPGRADED` | `attributes.oldLevel`, `attributes.newLevel` | `BONUS` ladder | level `USER_LIMIT`, `BUDGET` |

## High-Risk Money Boundary

T5 through T9 are high-risk money templates. Production rollout requires an activity-level `BUDGET`, user period quota, L2 approval, audit traceability, reward callback handling, and reconciliation by activity, subject, event, idempotency key, or command ID. Environments without those controls may still use the templates for simulation, but they should not be marked production-ready.

## Representative Payload

T9 uses the strict `FORMULA` shape required by the current schema:

```json
{
  "tenantId": "site_001",
  "activityCode": "TPL_UPGRADE_BONUS_202605",
  "activityName": "VIP upgrade bonus template",
  "activityType": "UPGRADE_BONUS",
  "status": "DRAFT",
  "triggerEvents": ["VIP_LEVEL_UPGRADED"],
  "startTime": "2026-05-01T00:00:00+08:00",
  "endTime": "2026-12-31T23:59:59+08:00",
  "timezone": "Asia/Shanghai",
  "priority": 100,
  "config": {"metadata": {"sourceSystem": "member", "bizType": "VIP_LEVEL"}},
  "exclusiveGroup": "upgrade_bonus",
  "stackable": false,
  "rules": [
    {
      "ruleCode": "T9_NEW_GT_OLD",
      "rulePhase": "QUALIFY",
      "ruleType": "FORMULA",
      "ruleConfig": {
        "formula": "attributes.newLevel - attributes.oldLevel",
        "resultType": "DECIMAL",
        "compare": {"op": ">", "value": "0"}
      },
      "sort": 1,
      "enabled": true,
      "version": 1
    }
  ],
  "rewards": [
    {
      "rewardCode": "T9_UPGRADE_LADDER",
      "rewardType": "BONUS",
      "calculateType": "LADDER",
      "rewardConfig": {
        "field": "attributes.newLevel",
        "ladders": [
          {"min": "2", "max": "3", "amount": "68.00"},
          {"min": "4", "max": "99", "amount": "188.00"}
        ],
        "currency": "CNY",
        "flowMultiple": "1",
        "targetRef": "https://wallet.example.com/reward"
      },
      "grantMode": "AUTO",
      "grantChannel": "HTTP",
      "enabled": true,
      "version": 1
    }
  ]
}
```

Quota examples after activity creation:

```json
[
  {"activityId": "act_t9", "quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "10001:4", "totalValue": "1"},
  {"activityId": "act_t9", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "40000.00"}
]
```

Simulation pass event:

```json
{
  "eventId": "evt_t9_pass",
  "eventType": "VIP_LEVEL_UPGRADED",
  "sourceSystem": "member",
  "tenantId": "site_001",
  "subject": {"type": "USER", "id": "10001"},
  "biz": {"bizType": "VIP_LEVEL", "bizId": "10001:4"},
  "occurredAt": "2026-05-29T10:00:00Z",
  "attributes": {"oldLevel": 3, "newLevel": 4, "currency": "CNY"}
}
```

Expected simulation result: `decision=PASSED`, reward preview amount `188.0000`, quota previews with `wouldExceed=false`, and grant/outbox previews. If `newLevel <= oldLevel`, the formula result is not greater than zero and the activity is rejected before reward and quota preview.
