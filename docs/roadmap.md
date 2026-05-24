# Roadmap

## 当前阶段：Phase 1 公共内核

目标：

```text
抽出活动营销系统的最小公共内核，让普通配置化活动可以走统一链路。
```

已完成：

```text
marketing_activity
marketing_rule
marketing_reward
marketing_event_inbox
marketing_participation
marketing_grant_order
marketing_quota
marketing_outbox
marketing_audit
marketing_fact_provider

HTTP Event In
HTTP/Webhook Outbox worker
Reward callback
模拟运行
基础规则 CONDITION
FIXED / RATIO 奖励
PostgreSQL 本地部署
Quota 原子扣减
Outbox claim / retry / DLQ
最小 admin 页面
```

验收标准：

```text
go test ./...
go build ./...
Docker PostgreSQL 可启动
创建活动 -> 创建 quota -> 上报事件 -> participation -> grant_order -> outbox 可跑通
重复事件不会重复发奖
quota exceeded 返回 409
Outbox 多 worker 不重复 claim
```

## Phase 2 低风险活动接入

优先接入：

```text
签到有礼
每月充值
首存
续存
新手任务
活动任务体系
```

需要补齐：

```text
LADDER 奖励计算
每日/周期 quota
用户参与次数通用规则
活动配置 JSON Schema 校验
上线前模拟运行报告
基础后台配置页
活动导入/导出
```

验收标准：

```text
不新增业务专用 controller
不新增业务专用发奖逻辑
活动通过配置完成
模拟运行能解释命中和拒绝原因
```

## Phase 3 高风险资金活动接入

优先接入：

```text
返水
邀请好友
VIP 周红利
VIP 月红利
生日红利
升级红利
```

需要补齐：

```text
预算冻结/确认/释放模型
grant_order 回调后确认 quota 消耗
失败补偿和人工关闭
对账任务
高危配置二级审批
审计版本化
活动互斥组完整实现
风险标签规则
IP / Device 限制
```

验收标准：

```text
任意发奖失败不会造成预算永久漂移
任意重复消息不会重复发奖
任意 callback 不能跨 tenant 更新
资金活动必须有审计和对账结果
```

## Phase 4 玩法型活动接入

优先接入：

```text
红包雨
主题抽奖
幸运翻卡
足球射门
赛事竞猜
嘉年华
```

需要补齐：

```text
活动插件注册和生命周期
RED_ENVELOPE 插件
LOTTERY 插件
奖池库存
概率配置校验
RANDOM / PROBABILITY 计算器
POOL_SPLIT 计算器
WebSocket 推送事件
场次调度
排行榜状态
```

验收标准：

```text
插件只负责玩法上下文
插件不得直接发奖
插件不得绕过 quota / participation / grant_order / outbox
概率和库存必须上线前校验
```

## Phase 5 生产化

需要补齐：

```text
OpenAPI 鉴权
Admin 鉴权
Reward callback 签名
Fact Provider 配置中心
Fact Query 缓存
MQ Event In
MQ Command Out
Webhook Receiver
Outbox 运维后台
DLQ 重放后台
指标和告警
数据库 migration 发布流程
备份和恢复流程
```

生产前必须通过：

```text
安全 review
账务 review
压测
故障注入
重复事件测试
Outbox 重试测试
DLQ 重放测试
Quota 并发扣减测试
Callback 幂等测试
```

## 明确不做

ExMarketing 不负责：

```text
用户注册
订单支付
充值提现
钱包加钱
游戏结算
会员等级计算
真实风控判定
短信推送
```

这些能力由外部系统提供，ExMarketing 只通过标准事件、Fact Query 和 Reward Command 对接。

