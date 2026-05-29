# RED_ENVELOPE And LOTTERY Sample Plugins

`RED_ENVELOPE` and `LOTTERY` are sample ActivityPlugin implementations. They
validate gameplay configuration, build `state.plugin.<pluginCode>.*` context,
return candidate reward explanations, and emit state-change intents after the
main engine has qualified and calculated rewards.

They do not directly create participation records, grant orders, outbox
messages, quota reservations, wallet requests, or final inventory facts. Rewards,
stock, budget, user limits, participation snapshots, grant orders, outbox, and
callback reconciliation remain owned by the main activity engine.

## RED_ENVELOPE Template

```json
{
  "activityType": "RED_ENVELOPE",
  "pluginCode": "RED_ENVELOPE",
  "config": {
    "rounds": [
      {
        "roundCode": "morning",
        "startAt": "2026-05-29T10:00:00Z",
        "endAt": "2026-05-29T11:00:00Z",
        "poolCode": "pool_a",
        "poolTotalCount": 100,
        "emptyAllowed": true,
        "perUserLimit": 1,
        "display": {
          "title": "Morning red envelope",
          "button": "Claim"
        },
        "candidateRewards": [
          {
            "candidateCode": "packet_8_cny",
            "rewardCode": "REWARD_RED_PACKET_8",
            "displayName": "8 CNY"
          }
        ]
      }
    ]
  }
}
```

Prelaunch validation blocks empty rounds, duplicate or overlapping rounds,
invalid round windows, non-positive pool or per-user limits, inconsistent
`emptyAllowed=false` pools, unknown enabled `rewardCode` references, and unknown
config fields.

Runtime context includes `roundCode`, `roundStatus`, `poolCode`, `hit`,
`claimPosition`, `candidateRewardCode`, and `reasonCode`. The plugin can reject
with `RED_ENVELOPE_NO_ACTIVE_ROUND`, `RED_ENVELOPE_ALREADY_CLAIMED`,
`RED_ENVELOPE_USER_LIMIT_REACHED`, or `EMPTY_ENVELOPE`.

## LOTTERY Template

```json
{
  "activityType": "LOTTERY",
  "pluginCode": "LOTTERY",
  "config": {
    "lotteryCode": "main",
    "drawWindow": {
      "startAt": "2026-05-29T10:00:00Z",
      "endAt": "2026-05-29T11:00:00Z"
    },
    "perUserDrawLimit": 1,
    "seedSalt": "public-demo-salt",
    "items": [
      {
        "itemCode": "GOLD",
        "rewardCode": "REWARD_LOTTERY_GOLD",
        "weight": 10,
        "displayName": "Gold prize"
      },
      {
        "itemCode": "NO_PRIZE",
        "weight": 90,
        "displayName": "No prize",
        "noPrize": true
      }
    ]
  }
}
```

Prelaunch validation blocks duplicate item codes, negative weights, zero total
weight, invalid draw windows, non-positive draw limits, unknown enabled
`rewardCode` references, and unknown config fields.

Runtime context includes `lotteryCode`, `drawCount`, `drawIndex`, `itemCode`,
`hit`, `candidateRewardCode`, and `reasonCode`. The deterministic audit input
uses tenant, activity, activity version, plugin code, lottery code, subject,
standard event idempotency key, draw index, and public seed salt. The audit
explain payload stores the seed hash and weighted cursor, not a secret.

## Simulation

Simulation uses the same plugin `BuildContext` path and returns plugin context,
candidate results, precheck result, and audit explanation in the activity
preview. It does not persist plugin state, consume quota, create grant orders, or
write outbox messages.
