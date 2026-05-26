# Test Plan

## Agent 指令

你是 Test Agent。只有 `review.md` Decision 为 `PASS` 后才能执行本计划。

本计划只覆盖需求 `2026-05-24-113929-3`：`FIXED` / `RATIO` / `LADDER` 奖励计算。不得扩大到新增或重构 `RANDOM` / `PROBABILITY` / `POOL_SPLIT`。

## Test Strategy

1. 优先补充 `internal/reward/calculator_test.go` 的纯计算单元测试，锁定 Decimal、字段路径、封顶、阶梯闭区间、非法配置和运行时失败原因。
2. 补充 `internal/engine/service_test.go` 或现有同层测试，验证计算结果进入 participation、grant_order、outbox 链路，且 0 金额、额度不足、计算失败不会产生实际发奖。
3. 对幂等重复和并发使用已有事件处理入口验证，不直接绕过 repository 或 quota 逻辑。
4. PostgreSQL smoke 必须使用真实 PostgreSQL DSN 验证持久化、唯一约束、事务和并发 quota 语义；不能只用内存仓储代表最终结论。
5. 所有金额断言统一使用 4 位小数字符串，例如 `18.0000`、`59.8000`、`500.0000`。

## Test Cases

### TC01 FIXED 正常路径

- 配置 `BONUS` 或 `CASH`，`calculateType=FIXED`，`rewardConfig.amount="18.00"`，`currency="CNY"`，合法 `targetRef`。
- 发送一条匹配活动和规则的标准事件。
- 期望计算输出 `amount=18.0000`。
- 期望创建 1 条 `REWARDABLE` participation、1 条 grant_order、1 条 outbox。
- 期望 grant payload 保留 `rewardCode`、`rewardType`、`calculateType=FIXED`、`currency`、`targetRef`。

### TC02 FIXED 0 金额

- 配置 `amount="0"`。
- 期望奖励计算成功输出 `0.0000`。
- 期望不创建实际 grant_order、不创建 outbox、不扣减 quota。
- 期望结果能区分这是成功计算出的 0 奖励，而不是配置错误或额度不足。

### TC03 FIXED 非法配置

- 分别覆盖缺少 `amount`、`amount` 不可解析、`amount < 0`、金额类缺少 `currency`、`AUTO + HTTP/WEBHOOK` 缺少或非法 `targetRef`。
- 期望配置校验或计算阶段返回明确错误。
- 期望不生成 grant_order/outbox，不扣 quota。

### TC04 RATIO 正常路径无封顶

- 配置 `baseField="attributes.amount"`、`rate="0.20"`，不配置 `maxAmount` 或配置足够高封顶。
- 事件 `attributes.amount="299.00"`。
- 期望输出 `59.8000`，不得出现浮点误差。
- 期望正常生成 participation、grant_order、outbox。

### TC05 RATIO 封顶

- 配置 `baseField="attributes.amount"`、`rate="0.20"`、`maxAmount="500.00"`。
- 事件 `attributes.amount="3000.00"`。
- 期望原始计算 `600.0000` 后封顶为 `500.0000`。
- 期望 grant payload 或计算摘要能解释封顶后的最终金额。

### TC06 RATIO 0 结果

- 覆盖 `rate="0"` 和 `maxAmount="0"` 两种路径。
- 期望计算成功输出 `0.0000`。
- 期望不生成实际 grant_order/outbox，不扣 quota。

### TC07 RATIO 运行时字段失败

- 分别覆盖 `baseField` 缺失、空字符串、布尔、对象、数组、不可解析 Decimal、负数金额。
- 期望返回明确计算失败原因，不得默认按 0 计算。
- 期望不生成 grant_order/outbox，不扣 quota。

### TC08 RATIO 配置失败

- 分别覆盖缺少 `baseField`、缺少 `rate`、`rate` 不可解析、`rate < 0`、`maxAmount` 不可解析、`maxAmount < 0`、`baseField` 使用非标准顶层业务字段如 `orderAmount`。
- 期望配置校验或计算阶段失败，错误可解释。
- 期望不会影响既有 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 测试。

### TC09 LADDER 正常路径和闭区间

- 配置 `field="attributes.continueDays"`，阶梯为 `[1,2]=>5.00`、`[3,6]=>18.00`、`[7,999]=>58.00`。
- 事件 `continueDays=1` 期望 `5.0000`。
- 事件 `continueDays=2` 期望仍命中第一档 `5.0000`。
- 事件 `continueDays=3` 和 `6` 期望 `18.0000`。
- 事件 `continueDays=7` 期望 `58.0000`。
- 期望阶梯数组顺序变化不影响唯一命中结果。

### TC10 LADDER 未命中和字段失败

- 使用不连续阶梯，例如 `[1,2]`、`[5,7]`，事件 `continueDays=3`。
- 期望返回明确计算失败原因，默认沿用 `ReasonLadderNotMatched` 语义。
- 期望不得输出 `0.0000`，不得按 `ZERO_REWARD_SKIPPED` 静默跳过。
- 分别覆盖 `field` 缺失、空字符串、布尔、对象、数组、不可解析 Decimal。
- 期望均不生成 grant_order/outbox，不扣 quota。

### TC11 LADDER 配置失败

- 覆盖缺少 `field`、缺少 `ladders`、空阶梯、缺少 `min/max/amount`、`min > max`、`amount < 0`、`amount` 不可解析、区间重叠、`field` 使用非标准顶层业务字段。
- 覆盖 `min = max` 单点闭区间为合法配置并可命中。
- 期望非法配置被拒绝，合法单点区间输出对应金额。

### TC12 额度不足

- 为 FIXED / RATIO / LADDER 至少各选 1 个正金额配置，设置活动级或 reward 级 quota 只剩 0 或不足以覆盖本次发奖。
- 期望活动和规则可命中，但 quota 阶段拒绝。
- 期望不创建 grant_order/outbox。
- 期望不出现部分 quota 扣减、负 quota、错误的 `REWARDABLE` 成功 participation。
- 期望错误原因能区分额度不足，而不是奖励计算失败。

### TC13 幂等重复

- 使用同一个 `Idempotency-Key` 重复提交同一事件，分别覆盖 FIXED、RATIO、LADDER 正常发奖路径。
- 第一次成功时最多创建 1 条 participation、1 条 grant_order、1 条 outbox，并扣 quota 一次。
- 第二次期望返回 duplicate 或既有幂等成功语义，不重复计算发奖、不重复扣 quota、不重复 outbox。
- 对 0 金额路径和 LADDER 未命中失败路径也重复提交一次，期望重复请求不产生新的 grant_order/outbox。
- 使用同一 key 提交不同 payload 时，期望返回既有幂等冲突错误。

### TC14 并发

- 使用真实 PostgreSQL 执行并发测试，不能只用内存仓储。
- 场景 A：共享 quota 只允许 1 次成功，10 个不同幂等 key 并发触发同一 FIXED 奖励。
- 场景 B：共享 quota 只允许 1 次成功，10 个不同幂等 key 并发触发同一 RATIO 奖励。
- 场景 C：共享 quota 只允许 1 次成功，10 个不同幂等 key 并发触发同一 LADDER 奖励。
- 期望每个场景成功数量不超过 quota，grant_order/outbox 数量与成功数量一致。
- 期望失败请求为额度不足或可解释事务失败；不得出现负 quota、重复 grant_order、重复 outbox。
- 场景 D：同一个幂等 key 并发提交 10 次，期望只处理一次。

### TC15 PostgreSQL Smoke

- 使用 `docker compose up -d postgres` 启动真实 PostgreSQL，或使用 Test Agent 环境提供的真实 PostgreSQL。
- 使用项目约定 DSN：`EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'`。
- 验证迁移后的表能持久化 FIXED / RATIO / LADDER 的 reward config、participation、grant_order、outbox。
- 验证 grant payload JSON 能保存计算摘要和 4 位金额字符串。
- 验证幂等唯一约束阻止重复 participation/grant/outbox。
- 验证 quota 条件更新或事务锁在并发下不超扣。
- 验证计算失败、0 金额、额度不足均不会留下可执行 outbox。

### TC16 回归范围

- 跑全量测试，确认既有 `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 行为没有被删除、禁用或改变。
- 确认非金额类奖励既有测试仍按原语义通过。
- 确认 outbox worker、HTTP API、workflow 现有测试不因新增计算类型校验回归。

## Required Commands

后续 Test Agent 必须按实际实现补充或调整精准 `-run` 过滤器，但至少执行以下命令清单。

~~~bash
go test ./internal/reward -run 'Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*|Test.*Calculator.*' -count=1
go test ./internal/engine -run 'Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*' -count=1
go test ./internal/marketing -run 'Test.*Prelaunch.*|Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*' -count=1
go test ./internal/httpapi -run 'Test.*Reward.*|Test.*Activity.*|Test.*Validation.*|Test.*Idempot.*' -count=1
go test ./...
go build ./...
~~~

PostgreSQL smoke 命令：

~~~bash
docker compose up -d postgres
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/postgres -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./internal/engine ./internal/httpapi -run 'Test.*Reward.*|Test.*Quota.*|Test.*Idempot.*|Test.*Concurrent.*|Test.*Fixed.*|Test.*Ratio.*|Test.*Ladder.*' -count=1
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

如果 PostgreSQL 集成测试使用 build tag，Test Agent 需追加执行：

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test -tags=integration ./...
~~~

## Data / Smoke

基础活动数据：

- `tenantId=site_001`
- `sourceSystem=wallet`
- `eventType=DEPOSIT_SUCCEEDED` 用于 FIXED / RATIO
- `eventType=CHECKIN_SUCCEEDED` 用于 LADDER
- `subject.type=USER`
- `subject.id=user_001`
- `biz.type=deposit` 或 `checkin`
- `biz.id` 每个用例唯一，幂等重复用例保持相同

奖励配置数据：

~~~json
{
  "fixed": {
    "rewardType": "BONUS",
    "calculateType": "FIXED",
    "rewardConfig": {
      "amount": "18.00",
      "currency": "CNY",
      "targetRef": "https://wallet.example.com/reward"
    }
  },
  "ratio": {
    "rewardType": "BONUS",
    "calculateType": "RATIO",
    "rewardConfig": {
      "baseField": "attributes.amount",
      "rate": "0.20",
      "maxAmount": "500.00",
      "currency": "CNY",
      "targetRef": "https://wallet.example.com/reward"
    }
  },
  "ladder": {
    "rewardType": "BONUS",
    "calculateType": "LADDER",
    "rewardConfig": {
      "field": "attributes.continueDays",
      "ladders": [
        {"min": 1, "max": 2, "amount": "5.00"},
        {"min": 3, "max": 6, "amount": "18.00"},
        {"min": 7, "max": 999, "amount": "58.00"}
      ],
      "currency": "CNY",
      "targetRef": "https://wallet.example.com/reward"
    }
  }
}
~~~

核心验收矩阵：

| 场景 | 必须断言 |
| --- | --- |
| 正常路径 | FIXED/RATIO/LADDER 均能输出 4 位金额并进入 participation -> grant_order -> outbox |
| 额度不足 | 不生成 grant_order/outbox，不产生负 quota 或部分扣减 |
| 幂等重复 | 同 key 同 payload 只发奖一次；同 key 不同 payload 返回冲突 |
| 并发 | PostgreSQL 下成功数不超过 quota，grant/outbox 与成功数一致 |
| PostgreSQL smoke | 真实库可迁移、可落库、可防重复、可保持事务一致 |

## Pass / Fail Criteria

PASS 条件：

- 所有新增或相关单元测试、集成测试通过。
- `go test ./...` 和 `go build ./...` 通过。
- PostgreSQL smoke 通过，或环境不可用时 Test Agent 在 `test.md` 明确记录未执行原因和替代验证；生产语义结论不得只依赖内存仓储。
- 正常路径、额度不足、幂等重复、并发、PostgreSQL smoke 均有明确测试记录。
- LADDER 未命中明确失败，不能被测成 `0.0000` 成功。

BLOCK 条件：

- 任一 FIXED / RATIO / LADDER 核心金额口径错误。
- 金额计算出现浮点误差或非 4 位金额输出。
- 额度不足、计算失败或 0 金额仍生成可执行 grant_order/outbox。
- 幂等重复导致重复发奖或重复扣 quota。
- PostgreSQL 并发出现超扣、负 quota、重复 grant_order、重复 outbox。
- `RANDOM` / `PROBABILITY` / `POOL_SPLIT` 既有行为被破坏。

## Decision

~~~text
PASS
~~~

理由：`review.md` Decision 为 `PASS`，需求边界清晰；本计划已覆盖正常路径、额度不足、幂等重复、并发、PostgreSQL smoke，以及 FIXED / RATIO / LADDER 的核心正负向和回归风险。

需求：2026-05-24-113929-3
