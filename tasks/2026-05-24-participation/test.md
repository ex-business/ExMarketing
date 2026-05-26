# Test

## Scope

根据以下文件执行测试：

- `tasks/2026-05-24-participation/task.md`
- `tasks/2026-05-24-participation/analysis.md`
- `tasks/2026-05-24-participation/review.md`
- `tasks/2026-05-24-participation/test-plan.md`
- `tasks/2026-05-24-participation/implementation.md`

## Test Cases

已覆盖的现有测试范围：

- participation 正常发奖链路：`REWARDABLE` participation、grant order、outbox、snapshot 最小字段和 participation idempotency key。
- 规则/入口拒绝链路：候选活动 entry/MATCH 拒绝时生成 `REJECTED` participation，且不生成 grant order/outbox。
- 互斥组失败链路：loser 活动生成 `REJECTED` participation。
- 零奖励/不可发奖励链路：拒绝或跳过奖励时不生成 grant/outbox。
- quota 链路：quota 不足回滚 participation、grant order、outbox；并发 quota 竞争不超扣。
- 幂等链路：重复 inbox、同 key 不同 payload、duplicate participation/grant idempotency key。
- 并发链路：同一发奖请求并发只处理一次。
- Webhook 链路：显式 header、派生 idempotency key、header 不匹配拒绝。
- simulate 链路：模拟运行返回 preview 且不持久化 inbox/participation/grant/outbox/quota。
- PostgreSQL repository integration：真实 PostgreSQL 下验证 atomic grant rollback、inbox duplicate/conflict、participation/grant 唯一约束、grant/outbox 创建失败事务回滚。

## Commands

~~~bash
go test ./...
~~~

结果：PASS。

~~~bash
go build ./...
~~~

结果：PASS。

~~~bash
go test ./... -run 'Participation.*Idempot|Idempot.*Participation|Participation.*Concurrent|Concurrent.*Participation|Quota.*Concurrent' -count=10
~~~

结果：PASS。

~~~bash
docker compose up -d postgres
docker compose ps postgres
~~~

结果：PASS，`exmarketing-postgres` 为 `healthy`，端口 `54329`。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -run 'Participation|Postgres|PostgreSQL' -count=1
~~~

结果：PASS。

~~~bash
go test ./... -race -run 'Participation|Idempot|Quota|Postgres|PostgreSQL' -count=1
~~~

结果：PASS。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./...
~~~

结果：PASS。

~~~bash
EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable' go test ./... -tags=integration -run 'Participation|Postgres|PostgreSQL' -count=1
~~~

结果：PASS。

## Result

~~~text
PASS
~~~

## Evidence

- 基础全量测试通过：`go test ./...`。
- 基础构建通过：`go build ./...`。
- participation/idempotency/concurrency/quota 专项重复测试通过：`-count=10`。
- PostgreSQL 容器健康，且带 `EX_MARKETING_DATABASE_URL` 的 PostgreSQL 专项与全量测试均通过，未依赖 skip 作为通过依据。
- race 专项通过，未发现 data race。
- 未修改业务代码、测试代码、文档代码、配置文件或其他文件；本轮仅写入 `tasks/2026-05-24-participation/test.md`。

## Failure Details

无失败命令。

## Reproduction

在仓库根目录执行 `Commands` 中列出的命令即可复现本轮验证结果。PostgreSQL 集成测试需要先启动 `docker compose up -d postgres`，并设置：

~~~bash
export EX_MARKETING_DATABASE_URL='postgres://exmarketing:exmarketing@localhost:54329/exmarketing?sslmode=disable'
~~~

需求：2026-05-24-participation
