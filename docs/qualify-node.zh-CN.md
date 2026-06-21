# Qualify 资格校验节点使用说明

`qualify` 是活动流程里的准入判断节点，用来把“用户能不能参加活动、事件是否有效、风险是否需要阻断”转换成可解释、可审计、可路由的结果。

它不负责查询数据，不负责次数扣减，不负责发奖。它只负责基于已经准备好的事件、事实、活动配置和规则配置做判断。

## 定位

推荐链路：

```text
event-in -> inbox -> normalize -> fact-query -> user-level-check -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox -> audit
```

不需要用户等级判断时，可以省略 `user-level-check`，但奖励和频控仍使用新节点：

```text
event-in -> inbox -> normalize -> match -> fact-query -> activity-state -> quota-check -> qualify -> candidate-reward -> budget-reserve -> participation -> grant-order -> outbox
```

职责边界：

| 节点 | 职责 |
| --- | --- |
| `fact-query` | 查询用户、存款、交易、预测市场、风控、邀请等事实 |
| `user-level-check` | 专门判断用户是否存在、用户等级/VIP 是否满足门槛 |
| `quota-check` | 做次数、频率和计数器校验，必要时原子占用次数 |
| `qualify` | 执行业务资格、风控、名单、渠道、活动条件等规则判断 |
| `candidate-reward` | 生成或计算奖励候选 |
| `budget-reserve` | 锁定预算、库存、额度 |
| `participation` | 写参与记录 |
| `grant-order` / `outbox` | 生成发奖单和投递消息 |

设计原则：

- 能提前查询的事实放在 `fact-query`。
- 用户存在、等级/VIP 这种高频硬门槛可以放在 `user-level-check`。
- 次数、频率、周期计数放在 `quota-check`。
- 多条活动准入规则、风控规则、名单规则、业务条件组合放在 `qualify`。
- 不要在 `qualify` 里做外部查询；否则规则判断会变成不可控副作用。

## 输入

`qualify` 常用输入包括：

| 输入 | 来源 | 说明 |
| --- | --- | --- |
| `event_context` | `inbox` / `normalize` | 标准事件和事件 attributes |
| `matched_activity` | `match` | 匹配到的活动配置、活动状态、奖励配置 |
| `facts` | `fact-query` | 用户、存款、交易、预测市场、风险、邀请等外部事实 |
| `activity_state` | `activity-state` | 用户活动状态、任务进度、签到连续天数 |
| `quota_check` | `quota-check` | 次数频控结果 |
| `user_level_check` | `user-level-check` | 用户存在和等级判断结果 |
| `rule_config` | 节点配置 / 活动规则 | 当前节点要执行的规则列表和组合模式 |
| `context` | workflow 变量 | 上游节点写入的公共上下文 |

输入示例：

```json
{
  "event_context": {
    "eventType": "CHECKIN_SUCCESS",
    "tenantId": "site_001",
    "subject": {
      "type": "USER",
      "id": "u_demo_001"
    },
    "attributes": {
      "channel": "APP",
      "checkin": true,
      "timezone": "Asia/Shanghai"
    }
  },
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "status": "ACTIVE",
      "level": 1,
      "vipLevel": 1,
      "country": "CN",
      "channel": "APP",
      "tags": ["all_users", "checkin"]
    }
  },
  "quota_check": {
    "decision": "PASS",
    "remaining": 0
  }
}
```

## 输出

`qualify` 标准输出应包括：

| 输出 | 说明 |
| --- | --- |
| `decision` | `PASS` / `BLOCK` / `RISK` / `SKIP` |
| `passed` | 是否通过 |
| `stage_result` | 完整规则执行摘要 |
| `ruleResults` | 每条规则的执行明细 |
| `reasonCode` | 机器可识别失败原因 |
| `reason` | 人可读失败说明 |
| `failedRule` | 首个失败规则 |
| `PASS` | 路由输出，继续奖励链路 |
| `BLOCK` | 路由输出，阻断或进入审计 |
| `RISK` | 路由输出，进入风控、人工复核或审计 |

成功示例：

```json
{
  "decision": "PASS",
  "passed": true,
  "reasonCode": "QUALIFIED",
  "reason": "all rules passed",
  "ruleResults": [
    {
      "ruleCode": "USER_ACTIVE",
      "status": "PASSED",
      "passed": true
    },
    {
      "ruleCode": "CHECKIN_COMPLETED",
      "status": "PASSED",
      "passed": true
    }
  ]
}
```

失败示例：

```json
{
  "decision": "BLOCK",
  "passed": false,
  "reasonCode": "VIP_LEVEL_NOT_ENOUGH",
  "reason": "facts.USER_PROFILE.vipLevel 0 < 1",
  "failedRule": "VIP_LEVEL_MIN",
  "ruleResults": [
    {
      "ruleCode": "VIP_LEVEL_MIN",
      "status": "REJECTED",
      "passed": false,
      "actual": 0,
      "expected": ">= 1"
    }
  ]
}
```

风险示例：

```json
{
  "decision": "RISK",
  "passed": false,
  "reasonCode": "RISK_SCORE_TOO_HIGH",
  "reason": "risk score 92 >= 80",
  "failedRule": "RISK_SCORE_MAX"
}
```

## 节点配置

当前核心配置：

```json
{
  "ruleCodes": ["USER_EXISTS", "USER_ACTIVE", "CHECKIN_COMPLETED", "VIP_LEVEL_MIN"],
  "evaluationMode": "ALL_PASS",
  "stopOnFirstBlock": true,
  "riskPolicy": "BLOCK",
  "blockAction": "BLOCK",
  "reasonPath": "qualification.reason"
}
```

字段说明：

| 字段 | 说明 | 常用值 |
| --- | --- | --- |
| `ruleCodes` | 要执行的规则编码列表 | `USER_EXISTS`, `VIP_LEVEL_MIN` |
| `evaluationMode` | 多规则组合方式 | `ALL_PASS`, `ANY_PASS`, `FIRST_DECISION` |
| `stopOnFirstBlock` | 遇到阻断是否立即停止 | `true`, `false` |
| `riskPolicy` | 风控命中后的决策 | `PASS`, `BLOCK`, `RISK` |
| `blockAction` | 资格失败后的决策 | `BLOCK`, `RISK`, `SKIP` |
| `reasonPath` | 将失败原因写入上下文的位置 | `qualification.reason` |

### evaluationMode

`ALL_PASS`：

```text
所有规则都通过才 PASS。
任意一条失败，整体 BLOCK 或 RISK。
```

适合每日签到、首存、VIP 等级奖励等强约束活动。

```json
{
  "evaluationMode": "ALL_PASS",
  "ruleCodes": ["USER_EXISTS", "USER_ACTIVE", "VIP_LEVEL_MIN", "CHECKIN_COMPLETED"]
}
```

`ANY_PASS`：

```text
任意一条规则通过就 PASS。
所有规则都失败才 BLOCK。
```

适合多任务任选、活动任务包、满足任一条件领奖。

```json
{
  "evaluationMode": "ANY_PASS",
  "ruleCodes": ["FIRST_DEPOSIT", "TOTAL_DEPOSIT_7D", "TRADE_VOLUME_MIN"]
}
```

`FIRST_DECISION`：

```text
按规则顺序执行，遇到明确 PASS / BLOCK / RISK 就返回。
```

适合风控优先、黑名单优先、分层决策。

```json
{
  "evaluationMode": "FIRST_DECISION",
  "ruleCodes": ["BLACKLIST_CHECK", "RISK_SCORE_MAX", "VIP_LEVEL_MIN"]
}
```

## 常用规则库

下面是建议内置或标准化支持的常用资格规则。实际执行时，规则可以由活动规则表、节点配置、表达式或 Lua 脚本扩展。

### 用户基础类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `USER_EXISTS` | 用户是否存在 | `facts.USER_PROFILE.exists == true` |
| `USER_ACTIVE` | 用户状态是否正常 | `facts.USER_PROFILE.status == "ACTIVE"` |
| `USER_NOT_BANNED` | 用户未封禁 | `status != "BANNED"` |
| `USER_NOT_FROZEN` | 用户未冻结 | `status != "FROZEN"` |
| `USER_NOT_LOCKED` | 用户未锁定 | `locked == false` |
| `USER_CAN_RECEIVE_REWARD` | 用户可收奖 | `rewardDisabled != true` |
| `USER_CAN_WITHDRAW` | 用户提款状态正常 | `withdrawStatus == "NORMAL"` |
| `KYC_PASSED` | KYC / 实名通过 | `kycStatus == "PASSED"` |
| `EMAIL_VERIFIED` | 邮箱已验证 | `emailVerified == true` |
| `PHONE_VERIFIED` | 手机已验证 | `phoneVerified == true` |
| `ACCOUNT_AGE_MIN` | 账号注册至少 N 天 | `registeredDays >= N` |
| `ACCOUNT_AGE_MAX` | 新用户注册 N 天内 | `registeredDays <= N` |
| `USER_STATUS_ALLOWED` | 用户状态白名单 | `status in allowedStatuses` |
| `USER_STATUS_BLOCKED` | 用户状态黑名单 | `status not in blockedStatuses` |

### 等级 / VIP / 分层类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `VIP_LEVEL_MIN` | VIP 最低等级 | `vipLevel >= minVipLevel` |
| `VIP_LEVEL_MAX` | VIP 最高等级 | `vipLevel <= maxVipLevel` |
| `VIP_LEVEL_RANGE` | VIP 等级区间 | `min <= vipLevel <= max` |
| `VIP_LEVEL_ALLOWED` | VIP 等级白名单 | `vipLevel in allowedVipLevels` |
| `USER_LEVEL_MIN` | 用户最低等级 | `level >= minLevel` |
| `USER_LEVEL_RANGE` | 用户等级区间 | `level between min/max` |
| `USER_TIER_ALLOWED` | 用户层级允许 | `tier in ["SILVER", "GOLD"]` |
| `USER_SEGMENT_ALLOWED` | 用户分群允许 | `segment in allowedSegments` |
| `USER_TAG_REQUIRED` | 必须包含标签 | `tags contains requiredTag` |
| `USER_TAG_BLOCKED` | 禁止包含标签 | `tags not contains blockedTag` |
| `AUDIENCE_MATCHED` | 命中活动人群包 | `audienceMatched == true` |
| `VIP_UPGRADED` | VIP 升级事件有效 | `newVipLevel > oldVipLevel` |
| `LEVEL_UPGRADED` | 用户等级升级事件有效 | `newLevel > oldLevel` |

### 事件基础类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `EVENT_TYPE_ALLOWED` | 事件类型匹配 | `eventType in allowedEventTypes` |
| `SOURCE_SYSTEM_ALLOWED` | 来源系统匹配 | `sourceSystem in allowedSourceSystems` |
| `TENANT_MATCHED` | 租户匹配 | `event.tenantId == activity.tenantId` |
| `BIZ_TYPE_ALLOWED` | 业务类型匹配 | `biz.bizType in allowedBizTypes` |
| `BIZ_ID_REQUIRED` | 业务 ID 必填 | `biz.bizId != ""` |
| `EVENT_TIME_VALID` | 事件时间有效 | `occurredAt in activity window` |
| `EVENT_NOT_TOO_OLD` | 事件不过期 | `now - occurredAt <= maxDelay` |
| `EVENT_ATTRIBUTE_REQUIRED` | 必填属性存在 | `attributes[key] exists` |
| `EVENT_ATTRIBUTE_MATCHED` | 属性值匹配 | `attributes[key] == expected` |
| `CHECKIN_COMPLETED` | 完成签到 | `attributes.checkin == true` |
| `TASK_COMPLETED` | 任务完成 | `attributes.taskCompleted == true` |
| `FIRST_LOGIN` | 首次登录 | `attributes.firstLogin == true` |
| `APP_VERSION_ALLOWED` | App 版本允许 | `appVersion >= minVersion` |

### 渠道 / 入口 / 投放来源类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `CHANNEL_ALLOWED` | 渠道白名单 | `channel in ["APP", "WEB"]` |
| `CHANNEL_BLOCKED` | 渠道黑名单 | `channel not in blockedChannels` |
| `ENTRANCE_ALLOWED` | 活动入口允许 | `entranceCode in allowedEntrances` |
| `CAMPAIGN_SOURCE_ALLOWED` | 投放来源允许 | `utmSource in allowedSources` |
| `CAMPAIGN_MEDIUM_ALLOWED` | 投放媒介允许 | `utmMedium in allowedMediums` |
| `AFFILIATE_ALLOWED` | 渠道代理允许 | `affiliateCode in allowedAffiliates` |
| `REFERRER_ALLOWED` | 来源页面允许 | `referrer in allowedReferrers` |
| `PLATFORM_ALLOWED` | 平台允许 | `platform in ["IOS", "ANDROID", "WEB"]` |
| `DEVICE_TYPE_ALLOWED` | 设备类型允许 | `deviceType in allowedDeviceTypes` |

### 地区 / 语言 / 币种类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `COUNTRY_ALLOWED` | 国家白名单 | `country in allowedCountries` |
| `COUNTRY_BLOCKED` | 国家黑名单 | `country not in blockedCountries` |
| `REGION_ALLOWED` | 地区白名单 | `region in allowedRegions` |
| `REGION_BLOCKED` | 地区黑名单 | `region not in blockedRegions` |
| `LANGUAGE_ALLOWED` | 语言限制 | `language in allowedLanguages` |
| `CURRENCY_ALLOWED` | 币种限制 | `currency in allowedCurrencies` |
| `TIMEZONE_ALLOWED` | 时区限制 | `timezone in allowedTimezones` |
| `IP_COUNTRY_ALLOWED` | IP 国家限制 | `ipCountry in allowedCountries` |
| `RESIDENCE_COUNTRY_MATCHED` | 居住国家匹配 | `profile.country == kyc.country` |

### 存款 / 支付类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `DEPOSIT_SUCCESS` | 存款成功事件 | `eventType == "DEPOSIT_SUCCESS"` |
| `FIRST_DEPOSIT` | 是否首存 | `isFirstDeposit == true` |
| `NOT_FIRST_DEPOSIT` | 非首存 | `isFirstDeposit == false` |
| `DEPOSIT_AMOUNT_MIN` | 单笔存款最低金额 | `amount >= minAmount` |
| `DEPOSIT_AMOUNT_MAX` | 单笔存款最高金额 | `amount <= maxAmount` |
| `DEPOSIT_AMOUNT_RANGE` | 单笔存款区间 | `min <= amount <= max` |
| `DEPOSIT_CURRENCY_ALLOWED` | 存款币种允许 | `currency in allowedCurrencies` |
| `PAYMENT_METHOD_ALLOWED` | 支付方式允许 | `paymentMethod in allowedMethods` |
| `PAYMENT_PROVIDER_ALLOWED` | 支付通道允许 | `provider in allowedProviders` |
| `TOTAL_DEPOSIT_TODAY` | 今日累计存款 | `todayAmount >= minAmount` |
| `TOTAL_DEPOSIT_7D` | 7 天累计存款 | `total7d >= minAmount` |
| `TOTAL_DEPOSIT_30D` | 30 天累计存款 | `total30d >= minAmount` |
| `DEPOSIT_COUNT_MIN` | 存款次数门槛 | `depositCount >= minCount` |
| `DEPOSIT_COUNT_7D_MIN` | 7 天存款次数 | `depositCount7d >= minCount` |
| `LAST_DEPOSIT_WITHIN` | 最近一次存款在 N 天内 | `daysSinceLastDeposit <= N` |
| `NO_CHARGEBACK` | 无拒付记录 | `chargebackCount == 0` |
| `DEPOSIT_STATUS_FINAL` | 存款最终成功 | `depositStatus == "SUCCESS"` |

### 交易所 / 预测市场类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `TRADE_PLACED` | 已交易 | `tradeCount > 0` |
| `TRADE_SETTLED` | 交易已结算 | `settled == true` |
| `TRADE_VOLUME_MIN` | 交易量门槛 | `tradeVolume >= minAmount` |
| `TRADE_VOLUME_7D` | 7 天交易量 | `tradeVolume7d >= minAmount` |
| `TRADE_COUNT_MIN` | 交易次数门槛 | `tradeCount >= minCount` |
| `TRADING_PNL_LOSS_MIN` | 交易 PnL 损失门槛 | `tradingPnl <= -minLoss` |
| `TRADING_PNL_PROFIT_MAX` | 交易 PnL 收益上限 | `tradingPnl <= maxProfit` |
| `TRADE_VOLUME_MULTIPLE_MIN` | 交易量倍数门槛 | `tradeVolume / deposit >= multiple` |
| `MARKET_CATEGORY_ALLOWED` | 市场品类允许 | `marketCategory in allowedCategories` |
| `MARKET_PROVIDER_ALLOWED` | 市场数据源允许 | `marketProvider in allowedProviders` |
| `MARKET_ALLOWED` | 市场允许 | `marketId in allowedMarkets` |
| `EVENT_MARKET_ALLOWED` | 预测事件/市场允许 | `eventId/marketId matched` |
| `PRICE_PROBABILITY_MIN` | 最低价格/概率 | `priceOrProbability >= minPriceOrProbability` |
| `PRICE_PROBABILITY_RANGE` | 价格/概率区间 | `minPriceOrProbability <= priceOrProbability <= maxPriceOrProbability` |
| `ORDER_TYPE_ALLOWED` | 订单类型允许 | `orderType in allowedTypes` |
| `NO_EARLY_EXIT` | 未提前退出 | `earlyExitUsed == false` |
| `CANCELED_ORDER_EXCLUDED` | 取消订单排除 | `canceled == false` |

### 活动时间 / 周期类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `ACTIVITY_ACTIVE` | 活动状态为 ACTIVE | `activity.status == "ACTIVE"` |
| `ACTIVITY_TIME_ACTIVE` | 当前时间在活动期 | `now between startTime/endTime` |
| `VISIBLE_TIME_ACTIVE` | 展示时间有效 | `now between visibleFrom/visibleTo` |
| `SETTLEMENT_TIME_ALLOWED` | 结算时间允许 | `settledAt in reward window` |
| `CLAIM_TIME_ACTIVE` | 领取时间有效 | `now in claim window` |
| `COOLDOWN_PASSED` | 冷却时间已过 | `now - lastRewardAt >= cooldown` |
| `PERIOD_MATCHED` | 周期匹配 | `periodKey == currentPeriod` |
| `CUSTOM_PERIOD_ACTIVE` | 自定义时间段有效 | `segmentKey in activeSegments` |
| `SCHEDULED_WINDOW_ACTIVE` | 定时窗口有效 | `currentTime in configured windows` |

### 名单 / 人群 / 标签类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `ALLOWLIST_CHECK` | 白名单检查 | `subject.id in allowlist` |
| `DENYLIST_CHECK` | 黑名单检查 | `subject.id not in denylist` |
| `SEGMENT_ALLOWED` | 分群允许 | `segment in allowedSegments` |
| `SEGMENT_BLOCKED` | 分群禁止 | `segment not in blockedSegments` |
| `TAG_REQUIRED` | 必须有标签 | `tags contains requiredTag` |
| `TAG_ANY_REQUIRED` | 任意标签命中 | `tags intersects requiredTags` |
| `TAG_ALL_REQUIRED` | 所有标签命中 | `tags contains all requiredTags` |
| `TAG_BLOCKED` | 禁止标签 | `tags not intersects blockedTags` |
| `COHORT_MATCHED` | cohort 命中 | `cohortId in allowedCohorts` |
| `EXPERIMENT_BUCKET_MATCHED` | AB 分桶命中 | `bucket in allowedBuckets` |

### 风控 / 合规类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `BLACKLIST_CHECK` | 黑名单检查 | `blacklisted == false` |
| `RISK_SCORE_MAX` | 风险分上限 | `riskScore < maxRiskScore` |
| `DEVICE_RISK_CHECK` | 设备风险检查 | `deviceRisk != "HIGH"` |
| `IP_RISK_CHECK` | IP 风险检查 | `ipRisk != "HIGH"` |
| `MULTI_ACCOUNT_CHECK` | 多账号检查 | `multiAccount == false` |
| `ABUSE_HISTORY_CHECK` | 滥用历史检查 | `abuseCount == 0` |
| `BONUS_ABUSE_CHECK` | 奖励滥用检查 | `bonusAbuse == false` |
| `KYC_REQUIRED` | 必须实名 | `kycStatus == "PASSED"` |
| `AML_RISK_CHECK` | AML 风险检查 | `amlRisk != "HIGH"` |
| `SANCTION_CHECK` | 制裁名单检查 | `sanctionMatched == false` |
| `FRAUD_RULE_CHECK` | 反欺诈规则 | `fraudDecision != "BLOCK"` |
| `DEVICE_FINGERPRINT_UNIQUE` | 设备唯一性 | `deviceAccountCount <= limit` |
| `IP_ACCOUNT_COUNT_LIMIT` | 同 IP 账号数限制 | `ipAccountCount <= limit` |
| `PAYMENT_ACCOUNT_RISK_CHECK` | 支付账户风险 | `paymentAccountRisk != "HIGH"` |

### 邀请 / 裂变类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `INVITER_EXISTS` | 邀请人存在 | `inviter.exists == true` |
| `INVITER_ACTIVE` | 邀请人状态正常 | `inviter.status == "ACTIVE"` |
| `INVITEE_EXISTS` | 被邀请人存在 | `invitee.exists == true` |
| `INVITEE_NEW_USER` | 被邀请人是新用户 | `invitee.registeredDays <= N` |
| `INVITEE_FIRST_DEPOSIT` | 被邀请人已首存 | `invitee.firstDeposit == true` |
| `INVITEE_DEPOSIT_AMOUNT_MIN` | 被邀请人存款达标 | `invitee.depositAmount >= minAmount` |
| `INVITE_RELATION_VALID` | 邀请关系有效 | `relation.status == "VALID"` |
| `INVITE_RELATION_NOT_REWARDED` | 关系未发过奖 | `relation.rewarded == false` |
| `INVITE_COUNT_MIN` | 邀请人数门槛 | `inviteCount >= minCount` |
| `VALID_INVITEE_COUNT_MIN` | 有效邀请人数门槛 | `validInviteeCount >= minCount` |
| `INVITE_DEPTH_ALLOWED` | 邀请层级允许 | `depth <= maxDepth` |
| `SELF_INVITE_BLOCKED` | 禁止自邀请 | `inviter.id != invitee.id` |

### 任务 / 成就 / 签到类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `CHECKIN_COMPLETED` | 已签到 | `attributes.checkin == true` |
| `CHECKIN_STREAK_MIN` | 连续签到天数 | `streakDays >= minDays` |
| `CHECKIN_MISSED_ALLOWED` | 补签次数允许 | `missedDays <= maxMissed` |
| `TASK_COMPLETED` | 指定任务完成 | `task.status == "COMPLETED"` |
| `TASK_CODE_MATCHED` | 任务编码匹配 | `taskCode in allowedTaskCodes` |
| `TASK_PROGRESS_MIN` | 任务进度达标 | `progress >= target` |
| `ACHIEVEMENT_UNLOCKED` | 成就解锁 | `achievement.unlocked == true` |
| `MISSION_GROUP_COMPLETED` | 任务组完成 | `completedCount >= requiredCount` |
| `DAILY_TASK_COMPLETED` | 每日任务完成 | `dailyTask.completed == true` |
| `WEEKLY_TASK_COMPLETED` | 每周任务完成 | `weeklyTask.completed == true` |

### 余额 / 钱包 / 奖励账户类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `BALANCE_MIN` | 余额门槛 | `balance >= minBalance` |
| `BONUS_BALANCE_MAX` | 奖励余额上限 | `bonusBalance <= maxBonusBalance` |
| `WALLET_ACTIVE` | 钱包状态正常 | `wallet.status == "ACTIVE"` |
| `NO_PENDING_WITHDRAWAL` | 无待处理提款 | `pendingWithdrawal == false` |
| `NO_PENDING_REWARD` | 无待处理奖励 | `pendingReward == false` |
| `REWARD_ACCOUNT_AVAILABLE` | 奖励账户可用 | `rewardAccountAvailable == true` |
| `FLOW_REQUIREMENT_CLEARED` | 交易量要求已完成 | `tradeVolumeRemaining <= 0` |

### 频控 / 次数预判断类

严格计数和原子占用建议放到 `quota-check`，但 `qualify` 可以读取 `quota-check` 或事实结果做预判断。

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `NOT_REWARDED_TODAY` | 今日未领奖 | `todayRewardCount == 0` |
| `DAILY_LIMIT_NOT_EXCEEDED` | 今日次数未超 | `todayCount < maxDaily` |
| `WEEKLY_LIMIT_NOT_EXCEEDED` | 每周次数未超 | `weekCount < maxWeekly` |
| `MONTHLY_LIMIT_NOT_EXCEEDED` | 每月次数未超 | `monthCount < maxMonthly` |
| `LIFETIME_LIMIT_NOT_EXCEEDED` | 生命周期次数未超 | `totalCount < maxTotal` |
| `BIZ_NOT_REWARDED` | 业务单未发过奖 | `bizRewarded == false` |
| `EVENT_NOT_REWARDED` | 事件未发过奖 | `eventRewarded == false` |
| `COOLDOWN_NOT_ACTIVE` | 不在冷却期 | `cooldownActive == false` |

### 奖励前置 / 库存预算类

预算、库存的真实锁定建议放到 `budget-reserve`，但 `qualify` 可以做轻量预判断。

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `REWARD_ENABLED` | 奖励启用 | `reward.enabled == true` |
| `REWARD_AVAILABLE` | 奖励可用 | `reward.status == "ACTIVE"` |
| `BUDGET_AVAILABLE` | 预算看起来足够 | `budget.remaining > 0` |
| `COUPON_AVAILABLE` | 券库存看起来足够 | `coupon.stock > 0` |
| `PRIZE_AVAILABLE` | 奖品库存看起来足够 | `prize.stock > 0` |
| `REWARD_AMOUNT_IN_RANGE` | 奖励金额合法 | `min <= rewardAmount <= max` |
| `REWARD_CURRENCY_ALLOWED` | 奖励币种允许 | `reward.currency in allowedCurrencies` |
| `GRANT_CHANNEL_ALLOWED` | 发奖渠道允许 | `grantChannel in allowedChannels` |

### 审批 / 人工 / 运营控制类

| 规则编码 | 作用 | 常用判断 |
| --- | --- | --- |
| `APPROVAL_REQUIRED_FOR_HIGH_VALUE` | 高价值奖励需要审批 | `rewardAmount >= threshold -> RISK` |
| `MANUAL_REVIEW_REQUIRED` | 需要人工复核 | `manualReview == true -> RISK` |
| `OPERATOR_ALLOWED` | 操作人有权限 | `operator.role in allowedRoles` |
| `CAMPAIGN_SWITCH_ON` | 运营开关开启 | `campaignSwitch == true` |
| `KILL_SWITCH_OFF` | 熔断开关未开启 | `killSwitch != true` |
| `MAINTENANCE_WINDOW_BLOCK` | 维护期阻断 | `inMaintenanceWindow == false` |

## 常用组合模板

### 每日签到奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "USER_ACTIVE",
    "CHECKIN_COMPLETED",
    "VIP_LEVEL_MIN",
    "CHANNEL_ALLOWED"
  ],
  "evaluationMode": "ALL_PASS",
  "stopOnFirstBlock": true,
  "blockAction": "BLOCK"
}
```

通过条件：

```text
用户存在且 ACTIVE
事件是签到成功
VIP 等级满足
渠道是 APP 或 WEB
```

### 连续签到奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "CHECKIN_COMPLETED",
    "CHECKIN_STREAK_MIN",
    "NOT_REWARDED_TODAY"
  ],
  "evaluationMode": "ALL_PASS"
}
```

需要 `fact-query` 或 `activity-state` 提供：

```json
{
  "CHECKIN_STATE": {
    "streakDays": 7,
    "todayRewardCount": 0
  }
}
```

### 首存奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "USER_ACTIVE",
    "FIRST_DEPOSIT",
    "DEPOSIT_AMOUNT_MIN",
    "PAYMENT_METHOD_ALLOWED",
    "RISK_SCORE_MAX"
  ],
  "evaluationMode": "ALL_PASS",
  "riskPolicy": "RISK"
}
```

### 复存 / 续存奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "NOT_FIRST_DEPOSIT",
    "DEPOSIT_AMOUNT_MIN",
    "DEPOSIT_COUNT_MIN",
    "LAST_DEPOSIT_WITHIN"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 存款累计任务

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "TOTAL_DEPOSIT_7D",
    "DEPOSIT_COUNT_MIN",
    "CHANNEL_ALLOWED"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 交易任务

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "TRADE_SETTLED",
    "TRADE_VOLUME_MIN",
    "TRADE_COUNT_MIN",
    "PRICE_PROBABILITY_MIN"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 手续费返还活动

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "TRADE_SETTLED",
    "TRADE_VOLUME_MIN",
    "MARKET_CATEGORY_ALLOWED",
    "RISK_SCORE_MAX"
  ],
  "evaluationMode": "ALL_PASS",
  "riskPolicy": "RISK"
}
```

### VIP 周/月奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "USER_ACTIVE",
    "VIP_LEVEL_MIN",
    "TRADE_VOLUME_7D",
    "MONTHLY_LIMIT_NOT_EXCEEDED"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 生日奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "USER_ACTIVE",
    "KYC_PASSED",
    "VIP_LEVEL_MIN",
    "NOT_REWARDED_TODAY"
  ],
  "evaluationMode": "ALL_PASS"
}
```

需要 `fact-query` 提供生日、KYC、VIP 事实。

### 升级奖励

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "LEVEL_UPGRADED",
    "USER_LEVEL_MIN",
    "LIFETIME_LIMIT_NOT_EXCEEDED"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 邀请好友奖励

```json
{
  "ruleCodes": [
    "INVITER_EXISTS",
    "INVITER_ACTIVE",
    "INVITEE_NEW_USER",
    "INVITE_RELATION_VALID",
    "INVITEE_FIRST_DEPOSIT",
    "SELF_INVITE_BLOCKED"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 拉新任务包

```json
{
  "ruleCodes": [
    "INVITE_COUNT_MIN",
    "VALID_INVITEE_COUNT_MIN",
    "INVITEE_DEPOSIT_AMOUNT_MIN"
  ],
  "evaluationMode": "ALL_PASS"
}
```

### 高风险奖励活动

```json
{
  "ruleCodes": [
    "USER_EXISTS",
    "KYC_PASSED",
    "BLACKLIST_CHECK",
    "RISK_SCORE_MAX",
    "DEVICE_RISK_CHECK",
    "IP_RISK_CHECK",
    "MULTI_ACCOUNT_CHECK"
  ],
  "evaluationMode": "FIRST_DECISION",
  "riskPolicy": "RISK"
}
```

推荐路由：

```text
PASS -> candidate-reward
BLOCK -> audit
RISK -> approval-gate / audit
```

### 任选任务活动

```json
{
  "ruleCodes": [
    "FIRST_DEPOSIT",
    "TOTAL_DEPOSIT_7D",
    "TRADE_VOLUME_MIN",
    "TASK_COMPLETED"
  ],
  "evaluationMode": "ANY_PASS"
}
```

含义：完成任意一个交易、存款或任务条件即可继续发奖。

## 规则结果语义

建议每条规则统一输出：

```json
{
  "ruleCode": "VIP_LEVEL_MIN",
  "ruleType": "CONDITION",
  "status": "REJECTED",
  "passed": false,
  "reasonCode": "VIP_LEVEL_NOT_ENOUGH",
  "reason": "vipLevel 0 < 1",
  "field": "facts.USER_PROFILE.vipLevel",
  "actual": 0,
  "expected": ">= 1",
  "sort": 10,
  "detail": {
    "operator": ">=",
    "threshold": 1
  }
}
```

推荐状态：

| 状态 | 说明 |
| --- | --- |
| `PASSED` | 规则通过 |
| `REJECTED` | 规则明确失败，通常转 `BLOCK` |
| `RISK` | 命中风险，通常转 `RISK` |
| `SKIPPED` | 条件不适用或配置允许跳过 |
| `ERROR` | 规则执行异常 |

推荐 reasonCode：

| reasonCode | 说明 |
| --- | --- |
| `QUALIFIED` | 全部通过 |
| `CONDITION_NOT_MET` | 条件不满足 |
| `MISSING_FACT` | 必要事实缺失 |
| `USER_NOT_FOUND` | 用户不存在 |
| `USER_STATUS_INVALID` | 用户状态不允许 |
| `VIP_LEVEL_NOT_ENOUGH` | VIP 等级不足 |
| `DEPOSIT_AMOUNT_NOT_ENOUGH` | 存款金额不足 |
| `TRADE_VOLUME_NOT_ENOUGH` | 交易量不足 |
| `CHANNEL_NOT_ALLOWED` | 渠道不允许 |
| `COUNTRY_NOT_ALLOWED` | 国家不允许 |
| `BLACKLISTED` | 黑名单命中 |
| `RISK_SCORE_TOO_HIGH` | 风险分过高 |
| `LIMIT_EXCEEDED` | 次数限制超出 |
| `ACTIVITY_NOT_ACTIVE` | 活动未激活或不在时间窗 |
| `RULE_ERROR` | 规则执行异常 |

## 与表达式 / Lua 的关系

常用规则应尽量产品化成配置项和内置规则编码，不要所有逻辑都让运营写脚本。

推荐分层：

```text
内置规则编码
  -> 参数化配置
    -> 表达式
      -> Lua 脚本兜底
```

示例：参数化规则

```json
{
  "ruleCode": "VIP_LEVEL_MIN",
  "params": {
    "minVipLevel": 1,
    "path": "$.facts.USER_PROFILE.vipLevel"
  }
}
```

示例：表达式规则

```text
facts.USER_PROFILE.vipLevel >= 1 && event.attributes.checkin == true
```

示例：Lua 规则

```lua
local profile = facts.USER_PROFILE or {}
local channel = event.attributes.channel

qualified = profile.exists == true
  and profile.vipLevel >= 1
  and channel == "APP"
```

Lua 适合：

- 复杂数组判断。
- 多事实组合判断。
- 临时活动规则。
- 规则还没有产品化前的过渡。

Lua 不适合：

- 直接访问数据库或外部服务。
- 写入余额、发奖、扣库存。
- 隐藏关键风控逻辑。

## UI 建议

资格校验节点右侧属性面板建议分组：

### 基础信息

- 节点名称。
- 节点 ID。
- 节点类型。
- 输出分支：`PASS`、`BLOCK`、`RISK`。

### 规则集合

- 规则编码多选。
- 规则分组。
- 规则排序。
- 每条规则参数。
- 是否启用。
- 失败动作。

### 执行模式

- `ALL_PASS`。
- `ANY_PASS`。
- `FIRST_DECISION`。
- `stopOnFirstBlock`。

### 风险处理

- 风险命中输出 `RISK` 还是 `BLOCK`。
- 是否需要审批。
- 是否写审计。

### 运行日志

- 每条规则实际值。
- 每条规则期望值。
- 失败 reason。
- 最终 decision。
- 下游 route。

### 调试体验

调试时应展示：

```text
规则名
读取字段
实际值
期望值
是否通过
失败原因
最终路由
```

例如：

| 规则 | 字段 | 实际值 | 期望 | 结果 |
| --- | --- | --- | --- | --- |
| `USER_EXISTS` | `facts.USER_PROFILE.exists` | `true` | `true` | PASS |
| `VIP_LEVEL_MIN` | `facts.USER_PROFILE.vipLevel` | `0` | `>= 1` | BLOCK |

## 错误策略

建议区分“业务不满足”和“系统异常”。

业务不满足：

```text
用户不存在
等级不足
存款金额不足
渠道不允许
```

输出：

```text
decision = BLOCK
execution 可以是 BUSINESS_REJECTED
```

系统异常：

```text
规则配置非法
表达式编译失败
必要输入缺失且不可跳过
Lua 运行异常
```

输出：

```text
node status = FAILED
execution status = FAILED
```

缺失字段策略建议：

| 策略 | 说明 |
| --- | --- |
| `MISSING_AS_FALSE` | 缺失按条件不满足处理 |
| `MISSING_AS_RISK` | 缺失进入风险分支 |
| `MISSING_AS_ERROR` | 缺失视为系统配置错误 |
| `MISSING_SKIP_RULE` | 跳过当前规则 |

生产默认建议：

```text
资金/发奖活动：MISSING_AS_ERROR 或 MISSING_AS_FALSE
低风险营销活动：MISSING_AS_FALSE
灰度/实验活动：MISSING_SKIP_RULE 可选
```

## 审计和可观测性

每次执行应记录：

- 节点 ID。
- 节点版本。
- 规则列表。
- 每条规则输入字段。
- 每条规则实际值。
- 每条规则期望值。
- 每条规则结果。
- 最终 decision。
- reasonCode。
- reason。
- route。
- 是否停止于第一条失败规则。
- 执行耗时。

高风险字段建议脱敏：

- 手机号。
- 邮箱。
- 身份证件。
- IP。
- 设备指纹。
- 支付账号。

## 和其他节点的关系

### 和 `fact-query`

`fact-query` 查询：

```json
{
  "facts": {
    "USER_PROFILE": {
      "exists": true,
      "vipLevel": 1
    }
  }
}
```

`qualify` 判断：

```text
USER_PROFILE.exists == true
USER_PROFILE.vipLevel >= 1
```

### 和 `user-level-check`

`user-level-check` 适合做用户存在、等级、VIP 的通用硬门槛。

`qualify` 适合做更广义的活动规则组合。

如果流程里已经有 `user-level-check`，`qualify` 可以不再重复判断 VIP，或者只把其输出作为一条上游判断：

```text
user_level_check.decision == PASS
```

### 和 `quota-check`

`quota-check` 负责真实计数和频控。

`qualify` 可以读取 `quota_check.decision`，但不应该自己原子递增计数。

### 和 `condition`

`condition` 是单一表达式分支。

`qualify` 是规则集合和准入决策，适合需要审计、规则明细和多规则组合的场景。

### 和 `audit`

`BLOCK` 和 `RISK` 分支建议接 `audit`，记录为什么没有继续发奖。

## 推荐落地顺序

1. 先内置高频规则：用户、VIP、签到、存款、渠道、国家、风控分。
2. 再把规则参数化：金额、等级、渠道、国家、时间窗口都可配置。
3. 再支持表达式规则：用于中等复杂活动。
4. 最后支持 Lua 规则：用于复杂或临时活动。
5. 所有规则都必须有 trace、reasonCode 和审计输出。

## 当前实现注意

当前 `qualify` 在 workflow executor 中属于决策节点，输出 `PASS`、`BLOCK`、`RISK`，发布校验会要求对应分支路由完整。

如果活动域已经配置了 `QUALIFY` 阶段规则，执行器会把规则执行结果映射到 workflow 节点输出。否则会按节点配置和输入生成解释性决策。

后续建议继续增强：

- 规则参数表单产品化。
- 规则结果详情 UI。
- 规则库按分类选择。
- 配置错误红点和定位。
- 表达式/Lua 静态检查。
- 缺失字段策略。
- 风险分支审批联动。
- 规则执行耗时和命中率统计。
