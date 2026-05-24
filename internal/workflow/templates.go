package workflow

type RequirementTemplate struct {
	ID          string `json:"id"`
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
}

type WorkflowTemplate struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Source      string                `json:"source"`
	Description string                `json:"description"`
	Items       []RequirementTemplate `json:"items"`
}

func BuiltinTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{MarketingPlatformTemplate()}
}

func MarketingPlatformTemplate() WorkflowTemplate {
	return WorkflowTemplate{
		ID:          "marketing-platform-22",
		Name:        "22-独立活动营销中台设计",
		Source:      "act-analysis/22-独立活动营销中台设计.md",
		Description: "按设计文档拆分独立活动营销中台核心能力，依次进入 Analysis / Review / Test Plan / Dev / Test / Commit。",
		Items: []RequirementTemplate{
			{
				ID:          "event-schema-validation",
				Order:       1,
				Title:       "标准事件模型 Schema 校验",
				Description: "来源：22-独立活动营销中台设计。定义并校验标准事件字段：eventId、eventType、sourceSystem、tenantId、subject、biz、occurredAt、attributes。业务字段只能进入 attributes，系统不能内置具体业务模型。验收：缺字段、类型错误、非法时间、非法 subject/biz 均返回明确错误，并补充 API 文档和单元测试。",
				Risk:        "高",
			},
			{
				ID:          "event-http-api-auth",
				Order:       2,
				Title:       "HTTP Event API 鉴权与接入",
				Description: "来源：22-独立活动营销中台设计。完善 POST /openapi/v1/events 的 Bearer Token 或接入方鉴权骨架、租户隔离、请求大小限制和错误响应格式。验收：未授权拒绝、合法事件接收、租户字段不可缺失，测试覆盖成功和失败路径。",
				Risk:        "高",
			},
			{
				ID:          "event-mq-consumer",
				Order:       3,
				Title:       "MQ Event Consumer 接入",
				Description: "来源：22-独立活动营销中台设计。设计并实现 marketing.events 的 MQ 消费入口，复用标准事件校验、幂等和引擎处理逻辑，支持 Kafka/RabbitMQ/Pulsar 的适配边界。验收：消费者不直接写业务逻辑；重复消息不重复发奖；失败消息进入可重试路径。",
				Risk:        "中",
			},
			{
				ID:          "webhook-receiver",
				Order:       4,
				Title:       "Webhook Receiver 接入",
				Description: "来源：22-独立活动营销中台设计。实现 SaaS 化 Webhook Receiver 的接入模型，支持外部系统将事件推给活动系统，统一转换为标准事件。验收：Webhook payload 映射、签名/来源校验、失败响应和审计记录完整。",
				Risk:        "中",
			},
			{
				ID:          "event-inbox-idempotency",
				Order:       5,
				Title:       "Event Inbox 幂等处理",
				Description: "来源：22-独立活动营销中台设计。实现 marketing_event_inbox 的 idempotency_key 唯一约束，推荐格式 tenantId + sourceSystem + eventType + bizType + bizId。验收：重复事件不会重复参与、不会重复发奖、不会重复扣减预算；重复请求返回稳定结果。",
				Risk:        "高",
			},
			{
				ID:          "activity-main-config",
				Order:       6,
				Title:       "活动主配置模型",
				Description: "来源：22-独立活动营销中台设计。完善 marketing_activity，覆盖 tenant_id、activity_code、activity_name、activity_type、status、start_time、end_time、timezone、priority、exclusive_group、stackable、version、config_json。验收：活动基础配置可创建、查询、更新，时间和 timezone 校验明确。",
				Risk:        "中",
			},
			{
				ID:          "activity-display-entry-config",
				Order:       7,
				Title:       "活动展示与入口配置",
				Description: "来源：22-独立活动营销中台设计。将活动页面展示、入口、渠道、租户、人群入口等通用配置纳入 config_json 或独立配置结构。验收：后台可配置入口和展示字段；引擎只使用通用字段，不绑定具体业务页面。",
				Risk:        "低",
			},
			{
				ID:          "rule-config-schema",
				Order:       8,
				Title:       "Rule 配置 Schema",
				Description: "来源：22-独立活动营销中台设计。完善 marketing_rule 的 rule_phase、rule_type、rule_config_json、sort、enabled、version，并定义 CONDITION/FORMULA/LIMIT/SCRIPT 的 JSON Schema。验收：非法字段、非法 op、非法 value 类型会被后台校验拒绝。",
				Risk:        "中",
			},
			{
				ID:          "reward-config-schema",
				Order:       9,
				Title:       "Reward 配置 Schema",
				Description: "来源：22-独立活动营销中台设计。完善 marketing_reward，覆盖 reward_type、calculate_type、reward_config_json、grant_mode、grant_channel、flow_multiple。验收：BONUS/CASH/COUPON/POINT/GIFT/DRAW_CHANCE/MEMBERSHIP 的配置边界清晰，非法配置拒绝上线。",
				Risk:        "中",
			},
			{
				ID:          "match-rule-phase",
				Order:       10,
				Title:       "MATCH 活动匹配规则",
				Description: "来源：22-独立活动营销中台设计。实现 MATCH 阶段，判断活动是否在线、时间有效、入口展示、租户匹配、event_type 匹配和互斥优先级候选。验收：未命中活动必须输出原因；多活动候选按 priority 和互斥组处理。",
				Risk:        "中",
			},
			{
				ID:          "qualify-rule-phase",
				Order:       11,
				Title:       "QUALIFY 用户资格规则",
				Description: "来源：22-独立活动营销中台设计。实现 QUALIFY 阶段，基于 attributes、state、facts 判断 VIP、注册天数、渠道、用户标签等参与资格。验收：规则字段路径可读；字段不存在时返回可解释失败原因。",
				Risk:        "中",
			},
			{
				ID:          "reward-rule-phase",
				Order:       12,
				Title:       "REWARD 奖励条件规则",
				Description: "来源：22-独立活动营销中台设计。实现 REWARD 阶段，判断订单金额、充值金额、投注金额、签到天数、邀请人数等是否满足奖励条件。验收：每条奖励条件有通过/失败原因；条件通过后只进入奖励计算，不直接发奖。",
				Risk:        "中",
			},
			{
				ID:          "risk-limit-budget-rule-phase",
				Order:       13,
				Title:       "RISK/LIMIT/BUDGET 限制规则",
				Description: "来源：22-独立活动营销中台设计。实现 RISK、LIMIT、BUDGET 阶段，覆盖风控标签、次数、周期、互斥、预算、库存、幂等限制。验收：额度不足或限制失败不能生成 grant_order；失败写审计并返回结构化错误。",
				Risk:        "高",
			},
			{
				ID:          "decimal-money-calculation",
				Order:       14,
				Title:       "Decimal 金额计算规范",
				Description: "来源：22-独立活动营销中台设计。所有奖励金额、预算、库存金额使用 Decimal，禁止 float64。验收：比例赠送、返水、阶梯奖励、上限截断均无精度漂移；代码中新增金额逻辑不得使用 float64。",
				Risk:        "高",
			},
			{
				ID:          "fixed-ratio-ladder-reward",
				Order:       15,
				Title:       "固定/比例/阶梯奖励计算",
				Description: "来源：22-独立活动营销中台设计。实现 FIXED、RATIO、LADDER 奖励计算器，支持 baseField、rate、maxAmount、ladders。验收：固定金额、充值比例赠送、连续签到阶梯奖励均有单元测试。",
				Risk:        "中",
			},
			{
				ID:          "random-probability-pool-reward",
				Order:       16,
				Title:       "随机/概率/奖池奖励计算",
				Description: "来源：22-独立活动营销中台设计。实现 RANDOM、PROBABILITY、POOL_SPLIT 的奖励计算边界，支持概率总和校验、奖池拆分和随机结果可审计。验收：概率非法不能上线；随机结果记录到 snapshot 或审计上下文。",
				Risk:        "高",
			},
			{
				ID:          "participation-snapshot",
				Order:       17,
				Title:       "Participation 快照记录",
				Description: "来源：22-独立活动营销中台设计。所有活动命中后必须先写 marketing_participation，保存当时活动配置、规则结果、事件 payload 和拒绝原因。验收：不能绕过 participation 直接发奖；REJECTED/REWARDABLE 状态可追溯。",
				Risk:        "高",
			},
			{
				ID:          "grant-order-state-machine",
				Order:       18,
				Title:       "Grant Order 状态机",
				Description: "来源：22-独立活动营销中台设计。实现 marketing_grant_order 的 PENDING/SENDING/SUCCESS/FAILED/RETRY/CLOSED 状态流转、bill_no、idempotency_key 和 retry_count。验收：状态流转合法；重复创建不会生成重复发奖单。",
				Risk:        "高",
			},
			{
				ID:          "quota-optimistic-lock",
				Order:       19,
				Title:       "Quota 乐观锁原子扣减",
				Description: "来源：22-独立活动营销中台设计。实现 marketing_quota 的 DB 乐观锁扣减，覆盖 total_value、used_value、frozen_value、version。验收：并发扣减不会超预算；失败时没有 grant_order/outbox 脏写。",
				Risk:        "高",
			},
			{
				ID:          "quota-freeze-confirm-release",
				Order:       20,
				Title:       "Quota 冻结确认释放",
				Description: "来源：22-独立活动营销中台设计。实现预算/库存先冻结、发奖成功后确认消耗、发奖失败后释放冻结额度。验收：成功、失败、超时补偿、重复回调均不会造成 frozen_value 长期悬挂或重复释放。",
				Risk:        "高",
			},
			{
				ID:          "outbox-http-sender",
				Order:       21,
				Title:       "Outbox HTTP Sender",
				Description: "来源：22-独立活动营销中台设计。实现 marketing_outbox 的 HTTP/Webhook 发奖指令发送，包含 target_ref、payload_json、send_status、错误记录。验收：HTTP 2xx 成功、4xx/5xx 失败、超时都能正确更新状态。",
				Risk:        "中",
			},
			{
				ID:          "outbox-mq-sender",
				Order:       22,
				Title:       "Outbox MQ Sender",
				Description: "来源：22-独立活动营销中台设计。实现 MQ 发奖指令发送适配边界，让 Command Out 可以走 Kafka/RabbitMQ/Pulsar。验收：发送逻辑与业务事务解耦；MQ 失败不会丢失 outbox 记录。",
				Risk:        "中",
			},
			{
				ID:          "retry-policy",
				Order:       23,
				Title:       "失败重试策略",
				Description: "来源：22-独立活动营销中台设计。实现 Outbox 和发奖失败的重试策略，支持最大重试次数、退避间隔、下一次执行时间和错误信息。验收：可重试错误进入 RETRY，不可重试错误进入人工处理或 CLOSED。",
				Risk:        "中",
			},
			{
				ID:          "dlq-replay",
				Order:       24,
				Title:       "DLQ 查看与重放",
				Description: "来源：22-独立活动营销中台设计。实现失败消息 DLQ，支持查看失败原因、手动重放、关闭、重新进入 outbox。验收：DLQ 操作有审计；重放不会重复扣减预算或重复生成 grant_order。",
				Risk:        "中",
			},
			{
				ID:          "reward-callback-api",
				Order:       25,
				Title:       "Reward Callback 回调接口",
				Description: "来源：22-独立活动营销中台设计。实现外部发奖系统回调成功/失败接口，按 bill_no/idempotency_key 定位 grant_order。验收：成功、失败、重复回调、未知单号、签名失败都有明确响应和测试。",
				Risk:        "高",
			},
			{
				ID:          "fact-provider-config",
				Order:       26,
				Title:       "Fact Provider 配置",
				Description: "来源：22-独立活动营销中台设计。实现 Fact Provider 配置模型，支持 factCode、provider、endpoint、method、authRef、mapping、cacheTtlSeconds。验收：未配置 provider 时后台不允许规则依赖 facts.* 字段。",
				Risk:        "中",
			},
			{
				ID:          "fact-query-cache",
				Order:       27,
				Title:       "Fact Query 查询与缓存",
				Description: "来源：22-独立活动营销中台设计。实现 POST /openapi/v1/facts/query 的查询适配、字段 mapping、缓存 TTL 和失败降级。验收：事件 attributes 足够时不强依赖 Fact Query；facts 查询失败有可解释原因。",
				Risk:        "中",
			},
			{
				ID:          "prelaunch-validation",
				Order:       28,
				Title:       "上线前配置校验",
				Description: "来源：22-独立活动营销中台设计。实现上线前校验：概率总和、奖励金额是否超预算、库存、活动时间冲突、互斥组冲突、规则字段、JSON Schema、高危配置。验收：返回逐项 PASS/BLOCK，不允许高危配置上线。",
				Risk:        "中",
			},
			{
				ID:          "simulation-explain-runner",
				Order:       29,
				Title:       "模拟运行解释器",
				Description: "来源：22-独立活动营销中台设计。模拟运行输入事件样例，输出命中的活动、未命中原因、每条规则通过/失败原因、预计奖励、预算/库存影响、最终发奖指令预览。验收：模拟运行不写真实 participation、grant_order、outbox。",
				Risk:        "中",
			},
			{
				ID:          "approval-workflow",
				Order:       30,
				Title:       "活动审批流",
				Description: "来源：22-独立活动营销中台设计。实现活动配置上线审批流，支持普通审批、高危二级审批、审批记录、驳回原因和版本锁定。验收：未审批活动不能上线；审批记录可审计。",
				Risk:        "中",
			},
			{
				ID:          "operation-audit",
				Order:       31,
				Title:       "操作审计记录",
				Description: "来源：22-独立活动营销中台设计。运营修改配置、上线审批、模拟运行、发奖失败、预算扣减失败、DLQ 操作都必须留下审计记录。验收：审计记录包含操作者、动作、对象、前后快照、时间和请求来源。",
				Risk:        "中",
			},
			{
				ID:          "reconciliation-query",
				Order:       32,
				Title:       "对账查询能力",
				Description: "来源：22-独立活动营销中台设计。支持按活动、用户、事件、参与记录、发奖单、outbox 查询对账。验收：可定位“业务日志写了但消息失败”“发奖成功但状态未更新”等异常。",
				Risk:        "中",
			},
			{
				ID:          "plugin-registry",
				Order:       33,
				Title:       "ActivityPlugin 注册表",
				Description: "来源：22-独立活动营销中台设计。建立 ActivityPlugin 注册表和生命周期接口，插件只负责特殊上下文，不负责通用风控、幂等、预算和发奖。验收：新增插件不修改主流程；普通活动优先走数据配置。",
				Risk:        "中",
			},
			{
				ID:          "first-deposit-plugin",
				Order:       34,
				Title:       "首存活动插件",
				Description: "来源：22-独立活动营销中台设计。实现首存活动插件，只处理首存上下文和特殊字段，通用规则、预算、幂等、发奖仍走核心流程。验收：首存赠送可通过配置完成，插件不直接发钱。",
				Risk:        "中",
			},
			{
				ID:          "admin-config-pages",
				Order:       35,
				Title:       "管理后台通用配置页面",
				Description: "来源：22-独立活动营销中台设计。补齐活动配置、规则配置、奖励配置、预算库存、互斥叠加、风控、发奖方式、审批流、模拟运行、操作审计页面。验收：不再为每个活动单独堆页面，通用组件可配置多种活动。",
				Risk:        "中",
			},
		},
	}
}
