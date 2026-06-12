package marketing

import (
	"slices"
	"strings"
)

type WorkflowNodeConfigFieldType string
type WorkflowNodeConfigValidationRuleType string

const (
	WorkflowNodeConfigString  WorkflowNodeConfigFieldType = "STRING"
	WorkflowNodeConfigNumber  WorkflowNodeConfigFieldType = "NUMBER"
	WorkflowNodeConfigBoolean WorkflowNodeConfigFieldType = "BOOLEAN"
	WorkflowNodeConfigArray   WorkflowNodeConfigFieldType = "ARRAY"
	WorkflowNodeConfigObject  WorkflowNodeConfigFieldType = "OBJECT"
	WorkflowNodeConfigEnum    WorkflowNodeConfigFieldType = "ENUM"

	WorkflowNodeConfigRuleMinLength WorkflowNodeConfigValidationRuleType = "MIN_LENGTH"
	WorkflowNodeConfigRuleMaxLength WorkflowNodeConfigValidationRuleType = "MAX_LENGTH"
	WorkflowNodeConfigRulePattern   WorkflowNodeConfigValidationRuleType = "PATTERN"
	WorkflowNodeConfigRuleMinValue  WorkflowNodeConfigValidationRuleType = "MIN_VALUE"
	WorkflowNodeConfigRuleMaxValue  WorkflowNodeConfigValidationRuleType = "MAX_VALUE"
	WorkflowNodeConfigRuleMinItems  WorkflowNodeConfigValidationRuleType = "MIN_ITEMS"
	WorkflowNodeConfigRuleMaxItems  WorkflowNodeConfigValidationRuleType = "MAX_ITEMS"
)

type WorkflowNodeConfigSchema struct {
	NodeType    string                         `json:"nodeType"`
	Version     int                            `json:"version"`
	Category    string                         `json:"category,omitempty"`
	Title       string                         `json:"title"`
	Description string                         `json:"description,omitempty"`
	Fields      []WorkflowNodeConfigField      `json:"fields"`
	FieldGroups []WorkflowNodeConfigFieldGroup `json:"fieldGroups,omitempty"`
	Preview     WorkflowNodeConfigPreview      `json:"preview,omitempty"`
	Inputs      []WorkflowNodeInputVar         `json:"inputs"`
	Outputs     []WorkflowNodeOutputVar        `json:"outputs"`
}

type WorkflowNodeConfigFieldGroup struct {
	Key         string                    `json:"key"`
	Title       string                    `json:"title"`
	Description string                    `json:"description,omitempty"`
	Fields      []WorkflowNodeConfigField `json:"fields"`
}

type WorkflowNodeConfigPreview struct {
	Reads  []string `json:"reads,omitempty"`
	Writes []string `json:"writes,omitempty"`
	Notes  []string `json:"notes,omitempty"`
}

type WorkflowNodeInputVar struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

type WorkflowNodeOutputVar struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
}

type WorkflowNodeConfigField struct {
	Key             string                             `json:"key"`
	Label           string                             `json:"label"`
	Type            WorkflowNodeConfigFieldType        `json:"type"`
	Required        bool                               `json:"required,omitempty"`
	Default         any                                `json:"default,omitempty"`
	Options         []string                           `json:"options,omitempty"`
	Description     string                             `json:"description,omitempty"`
	ValidationRules []WorkflowNodeConfigValidationRule `json:"validationRules,omitempty"`
}

type WorkflowNodeConfigValidationRule struct {
	Type    WorkflowNodeConfigValidationRuleType `json:"type"`
	Value   any                                  `json:"value"`
	Message string                               `json:"message,omitempty"`
}

type WorkflowNodeConfigSchemaRegistry struct {
	schemas map[string]WorkflowNodeConfigSchema
}

func DefaultWorkflowNodeConfigSchemas() WorkflowNodeConfigSchemaRegistry {
	schemas := []WorkflowNodeConfigSchema{
		groupedSchema("event-in", "事件入口", "接收活动事件并生成标准 raw_event。", groups(
			fieldGroup("event-source", "事件来源", "限定哪些业务事件可以触发这个 workflow。", fields(
				withDefault(arrayField("eventTypes", "允许事件类型", true, "例如 DEPOSIT_SUCCESS、PAY_SUCCESS、ORDER_PAID、USER_REGISTERED。为空会阻止发布。", rules(minItems(1), maxItems(30))), []string{"DEPOSIT_SUCCESS", "PAY_SUCCESS"}),
				withDefault(arrayField("sourceSystems", "来源系统", true, "允许接入的系统编码，例如 payment、trade、crm。", rules(minItems(1), maxItems(20))), []string{"payment"}),
				enumField("tenantScope", "租户范围", true, "CURRENT_TENANT", []string{"CURRENT_TENANT", "WORKSPACE_TENANT", "ANY_TENANT"}),
			)),
			fieldGroup("payload-contract", "载荷契约", "控制事件载荷版本、未知事件和 trace 字段。", fields(
				withDescription(stringField("payloadSchemaVersion", "Payload Schema", true, "standard-v1", rules(minLength(3), maxLength(64))), "后端用于解析 raw_event 的版本号。"),
				booleanField("rejectUnknownEventType", "拒绝未知事件类型", false, true),
				withDescription(stringField("traceKeyPath", "Trace Key Path", false, "$.traceId", rules(maxLength(128))), "从 raw_event 中读取 trace key 的 JSONPath。"),
			)),
		), preview(
			[]string{"外部事件 payload", "tenantId / eventType / eventId", "sourceSystem"},
			[]string{"raw_event", "eventId", "eventType", "tenantId"},
			[]string{"不做业务判断，只负责入口过滤和标准 raw_event 生成。"},
		)),
		groupedSchema("inbox", "幂等收件箱", "写入事件收件箱，处理重复事件和 payload 冲突。", groups(
			fieldGroup("idempotency", "幂等策略", "决定同一个业务事件重复进入时如何处理。", fields(
				withDescription(stringField("idempotencyKeyTemplate", "幂等 Key 模板", true, "${tenantId}:${sourceSystem}:${eventType}:${biz.bizType}:${biz.bizId}", rules(minLength(8), maxLength(256))), "支持从 raw_event 渲染变量，必须能稳定定位同一业务事件。"),
				enumField("duplicatePolicy", "重复事件策略", true, "RETURN_EXISTING", []string{"RETURN_EXISTING", "REJECT", "REPROCESS"}),
				numberField("ttlSeconds", "幂等保留秒数", true, 86400, rules(minValue(60), maxValue(2592000))),
			)),
			fieldGroup("conflict", "冲突处理", "payload 不一致或上游 trace 缺失时的处理方式。", fields(
				enumField("payloadConflictPolicy", "Payload 冲突策略", true, "BLOCK_ON_PAYLOAD_MISMATCH", []string{"BLOCK_ON_PAYLOAD_MISMATCH", "KEEP_FIRST", "OVERWRITE"}),
				booleanField("requireTraceKey", "要求 Trace Key", false, false),
			)),
		), preview(
			[]string{"raw_event", "可选 Idempotency-Key"},
			[]string{"event_context"},
			[]string{"重复事件默认直接返回首次处理结果，不继续重复执行下游。"},
		)),
		schema("normalize", "事件标准化", "标准化事件字段", fields(
			objectField("mapping", "字段映射", false, nil),
			objectField("defaults", "默认值", false, nil),
		)),
		groupedSchema("match", "活动匹配", "根据事件上下文匹配可参与的活动。", groups(
			fieldGroup("strategy", "匹配策略", "控制匹配数量、优先级和活动状态约束。", fields(
				enumField("mode", "匹配模式", true, "FIRST_MATCH", []string{"FIRST_MATCH", "ALL_MATCHES", "PRIORITY_MATCH"}),
				booleanField("requireActiveActivity", "仅匹配已上线活动", false, true),
				enumField("priorityOrder", "优先级排序", false, "PRIORITY_DESC", []string{"PRIORITY_DESC", "CREATED_DESC", "UPDATED_DESC"}),
			)),
			fieldGroup("scope", "范围和失败处理", "限定租户和匹配失败时的下游行为。", fields(
				enumField("tenantMatchPolicy", "租户匹配", true, "SAME_TENANT", []string{"SAME_TENANT", "WORKSPACE_TENANT", "ANY_TENANT"}),
				enumField("fallbackAction", "未匹配动作", true, "NO_MATCH_FAIL", []string{"NO_MATCH_FAIL", "NO_MATCH_SKIP", "USE_DEFAULT_ACTIVITY"}),
				withDescription(stringField("activityCodePattern", "活动 Code 规则", false, "", rules(maxLength(128))), "可选正则，用于只匹配特定 code 的活动。"),
			)),
		), preview(
			[]string{"event_context", "activity_config"},
			[]string{"matched_activity"},
			[]string{"输出 matched_activity 后，下游 Qualify / Reward 可以直接引用活动配置。"},
		)),
		groupedSchema("condition", "条件判断", "根据表达式输出 IF / ELSE 分支。", groups(
			fieldGroup("expression", "条件表达式", "定义判断逻辑和输入字段缺失时的处理方式。", fields(
				withDescription(stringField("expression", "表达式", true, "$.fact.amount >= 100", rules(minLength(1), maxLength(2000))), "支持读取 fact、event_context 或上游输出的条件表达式。"),
				enumField("expressionLanguage", "表达式语言", true, "RULE_EXPR", []string{"RULE_EXPR", "JSON_LOGIC"}),
				enumField("missingFieldPolicy", "缺失字段策略", true, "FALSE", []string{"FALSE", "ERROR", "SKIP"}),
			)),
			fieldGroup("routing", "分支输出", "控制条件成立、不成立和异常时的 route 语义。", fields(
				withDescription(stringField("trueLabel", "成立分支 Label", true, "IF", rules(minLength(1), maxLength(32))), "表达式为 true 时输出的分支标签。"),
				withDescription(stringField("falseLabel", "不成立分支 Label", true, "ELSE", rules(minLength(1), maxLength(32))), "表达式为 false 时输出的分支标签。"),
				enumField("errorAction", "表达式异常动作", true, "BLOCK", []string{"BLOCK", "ELSE", "SKIP"}),
			)),
		), preview(
			[]string{"fact", "condition_expr", "上游变量引用"},
			[]string{"IF", "ELSE"},
			[]string{"发布前会校验条件分支必须有默认路径或明确 ELSE。"},
		)),
		groupedSchema("qualify", "资格校验", "执行资格、风控和名单规则，输出 PASS / BLOCK / RISK。", groups(
			fieldGroup("rules", "规则集合", "定义需要执行的资格规则和命中策略。", fields(
				withDefault(arrayField("ruleCodes", "规则编码", true, "要执行的资格规则编码，例如 ELIGIBILITY、KYC、RISK_SCORE。", rules(minItems(1), maxItems(50))), []string{"ELIGIBILITY"}),
				enumField("evaluationMode", "执行模式", true, "ALL_PASS", []string{"ALL_PASS", "ANY_PASS", "FIRST_DECISION"}),
				booleanField("stopOnFirstBlock", "阻断即停止", false, true),
			)),
			fieldGroup("risk", "风险处理", "控制风险命中和资格失败时的分支。", fields(
				enumField("riskPolicy", "风控策略", true, "BLOCK", []string{"PASS", "BLOCK", "RISK"}),
				enumField("blockAction", "资格失败动作", true, "BLOCK", []string{"BLOCK", "RISK", "SKIP"}),
				withDescription(stringField("reasonPath", "原因输出 Path", false, "qualification.reason", rules(maxLength(160))), "写入资格失败或风险原因的上下文路径。"),
			)),
		), preview(
			[]string{"event_context", "matched_activity", "规则配置"},
			[]string{"PASS", "BLOCK", "RISK"},
			[]string{"PASS 分支进入奖励或限制，BLOCK/RISK 分支可接审计、告警或终止。"},
		)),
		groupedSchema("switch", "条件分支", "按上游阶段结果选择多个业务分支。", groups(
			fieldGroup("source", "分支来源", "定义从哪个字段读取分支值。", fields(
				withDescription(stringField("sourcePath", "分支字段路径", true, "stage_result.status", rules(minLength(1), maxLength(160))), "从 stage_result 或上游输出读取分支 key 的路径。"),
				enumField("matchMode", "匹配模式", true, "EXACT", []string{"EXACT", "PREFIX", "REGEX", "IN_SET"}),
				enumField("missingValuePolicy", "缺失值策略", true, "DEFAULT", []string{"DEFAULT", "BLOCK", "SKIP"}),
			)),
			fieldGroup("cases", "分支配置", "定义显式分支和默认分支。", fields(
				withDefault(arrayField("cases", "分支项", true, "分支匹配项，建议包含 key、label、target 语义。", rules(minItems(1), maxItems(30))), []string{"PASS", "BLOCK"}),
				withDescription(stringField("defaultCase", "默认分支", true, "DEFAULT", rules(minLength(1), maxLength(64))), "没有命中任何 case 时使用的默认分支。"),
				booleanField("requireDefaultRoute", "要求默认连线", false, true),
			)),
		), preview(
			[]string{"stage_result", "sourcePath 指向的业务状态"},
			[]string{"PASS", "BLOCK", "SKIP"},
			[]string{"画布 route label 应与 cases/defaultCase 保持一致，Executor 按 route rule 决定下一节点。"},
		)),
		groupedSchema("fact-query", "事实查询", "查询用户、订单、交易或外部事实数据。", groups(
			fieldGroup("query", "查询定义", "定义要查询的事实 key 和查询范围。", fields(
				withDefault(arrayField("factKeys", "事实 Key", true, "需要查询的事实字段，例如 USER_PROFILE、ORDER_SUMMARY。", rules(minItems(1), maxItems(50))), []string{"USER_PROFILE"}),
				enumField("source", "数据来源", true, "FACT_SERVICE", []string{"FACT_SERVICE", "WAREHOUSE", "CACHE", "HTTP_PROVIDER"}),
				withDescription(stringField("subjectPath", "主体 Path", true, "subject.id", rules(minLength(1), maxLength(160))), "从输入中读取 userId、orderId 或业务主体 ID 的路径。"),
			)),
			fieldGroup("runtime", "运行策略", "控制查询超时、缓存和失败时的行为。", fields(
				numberField("timeoutSeconds", "超时秒数", true, 3, rules(minValue(1), maxValue(30))),
				numberField("cacheTtlSeconds", "缓存秒数", false, 60, rules(minValue(0), maxValue(86400))),
				enumField("failureAction", "查询失败动作", true, "BLOCK", []string{"BLOCK", "USE_EMPTY", "SKIP"}),
			)),
		), preview(
			[]string{"subject", "fact_keys", "外部事实源"},
			[]string{"facts"},
			[]string{"facts 可供 condition、qualify、reward 等下游节点引用。"},
		)),
		groupedSchema("code", "代码执行", "用受控代码片段处理上下文并输出 code_result。", groups(
			fieldGroup("runtime", "运行时", "定义代码语言、执行超时和输出限制。", fields(
				enumField("language", "语言", true, "LUA", []string{"LUA", "EXPRESSION"}),
				numberField("timeoutSeconds", "超时秒数", true, 5, rules(minValue(1), maxValue(60))),
				numberField("maxOutputBytes", "最大输出字节数", true, 65536, rules(minValue(1024), maxValue(1048576))),
			)),
			fieldGroup("script", "脚本和输出", "定义实际执行代码和输出路径。", fields(
				withDescription(stringField("source", "代码", true, "", rules(minLength(1), maxLength(20000))), "节点执行的代码片段或表达式。"),
				withDescription(stringField("outputPath", "输出路径", true, "code_result", rules(minLength(1), maxLength(128))), "代码执行结果写入上下文的路径。"),
				booleanField("allowNetwork", "允许网络访问", false, false),
			)),
		), preview(
			[]string{"context", "input", "上游变量引用"},
			[]string{"code_result"},
			[]string{"仅暴露当前 Executor 真实支持的 LUA / EXPRESSION；外部网络访问默认关闭。"},
		)),
		groupedSchema("reward", "奖励计算", "根据资格结果生成 reward_items。", groups(
			fieldGroup("reward-definition", "奖励定义", "定义奖励编码、类型和金额计算方式。", fields(
				withDescription(stringField("rewardCode", "奖励 Code", true, "STUDIO_DEFAULT_REWARD", rules(minLength(2), maxLength(128), pattern(`^[A-Z0-9._-]+$`, "奖励 Code 只能包含大写字母、数字、点、下划线和短横线。"))), "用于后续发奖、审计和对账。"),
				enumField("rewardType", "奖励类型", true, "BONUS", []string{"BONUS", "CASH", "POINT", "COUPON"}),
				enumField("calculateType", "计算类型", true, "FIXED", []string{"FIXED", "RATIO", "LADDER", "RANDOM", "PROBABILITY", "POOL_SPLIT"}),
			)),
			fieldGroup("amount", "金额和币种", "固定奖励或比例奖励的基础参数。", fields(
				numberField("amount", "固定金额", true, 10, rules(minValue(0))),
				stringField("currency", "币种", true, "USD", rules(pattern(`^[A-Z]{3}$`, "币种必须是 3 位大写字母。"))),
				withDescription(stringField("amountSourcePath", "金额来源 Path", false, "$.qualified_context.order.amount", rules(maxLength(160))), "RATIO / LADDER 模式下从输入上下文读取金额的路径。"),
			)),
			fieldGroup("grant", "发放策略", "定义奖励如何被下游发放或导出。", fields(
				enumField("grantMode", "发放模式", true, "AUTO", []string{"AUTO", "MANUAL", "MANUAL_EXPORT"}),
				enumField("grantChannel", "发放渠道", true, "HTTP", []string{"HTTP", "MQ", "WEBHOOK", "INTERNAL"}),
			)),
		), preview(
			[]string{"qualified_context", "reward_config"},
			[]string{"reward_items"},
			[]string{"reward_items 会包含 rewardCode、rewardType、amount、currency 和 grant metadata。"},
		)),
		schema("decimal", "精度处理", "按当前 Decimal 契约把金额解析为固定 4 位小数字符串。", fields(
			withDescription(stringField("amountExpr", "金额表达式", false, "", rules(maxLength(160))), "当没有映射 amount_expr 或上游金额输出时使用的金额表达式。当前 handler 输出固定 4 位小数字符串。"),
			withDescription(stringField("amount", "固定金额", false, "", rules(maxLength(64), pattern(`^-?\d+(\.\d+)?$`, "固定金额必须是十进制数字。"))), "amountExpr 为空时的固定金额兜底。"),
		)),
		groupedSchema("limit", "次数限制", "限制用户、活动、租户或业务维度的参与次数。", groups(
			fieldGroup("counter", "计数维度", "定义计数 bucket 和周期。", fields(
				enumField("scope", "限制范围", true, "USER", []string{"USER", "ACTIVITY", "TENANT", "BIZ"}),
				withDescription(stringField("scopeKeyTemplate", "Scope Key 模板", true, "${subject.id}:${activityId}", rules(minLength(1), maxLength(256))), "用于组成计数 key，必须稳定定位同一限制主体。"),
				enumField("period", "周期", true, "ACTIVITY", []string{"ACTIVITY", "DAY", "WEEK", "MONTH", "ROLLING_WINDOW"}),
			)),
			fieldGroup("policy", "限制策略", "控制最大次数、占用模式和超限分支。", fields(
				numberField("maxCount", "最大次数", true, 1, rules(minValue(1), maxValue(1000000))),
				enumField("consumeMode", "占用模式", true, "ATOMIC", []string{"ATOMIC", "CHECK_ONLY", "RESERVE_ONLY"}),
				enumField("exceedAction", "超限动作", true, "BLOCK", []string{"BLOCK", "RISK", "SKIP"}),
			)),
		), preview(
			[]string{"subject", "activity_id", "历史参与计数"},
			[]string{"limit_hold"},
			[]string{"limit_hold 可被 quota 使用；超限时走 BLOCK/RISK/SKIP 分支。"},
		)),
		groupedSchema("quota", "预算库存扣减", "对奖励预算、库存或用户额度做原子占用。", groups(
			fieldGroup("quota-target", "Quota 对象", "定义要扣减的预算或库存维度。", fields(
				withDescription(stringField("quotaCode", "Quota Code", true, "DEFAULT_REWARD_BUDGET", rules(minLength(2), maxLength(128))), "后端 quota 资源编码。"),
				enumField("quotaType", "Quota 类型", true, "BUDGET", []string{"BUDGET", "STOCK", "USER_LIMIT", "DAILY_LIMIT"}),
				enumField("scope", "作用范围", true, "ACTIVITY", []string{"ACTIVITY", "USER", "TENANT", "WORKSPACE"}),
				withDescription(stringField("scopeKeyTemplate", "Scope Key 模板", true, "${activityId}", rules(minLength(1), maxLength(256))), "用于组成 quota bucket，例如 activityId、userId 或 tenantId。"),
			)),
			fieldGroup("consume", "扣减策略", "控制扣减方式、失败动作和扣减数量来源。", fields(
				enumField("consumeMode", "扣减模式", true, "ATOMIC", []string{"ATOMIC", "RESERVE_ONLY", "CHECK_ONLY"}),
				withDescription(stringField("amountField", "扣减数量字段", true, "reward_items[0].amount", rules(minLength(1), maxLength(160))), "从 reward_items 或上下文读取扣减金额/数量。"),
				enumField("failAction", "额度不足动作", true, "BLOCK", []string{"BLOCK", "RISK", "SKIP"}),
			)),
		), preview(
			[]string{"reward_items", "limit_hold"},
			[]string{"quota_hold"},
			[]string{"ATOMIC 模式会在 Executor 中记录 quota_hold，失败时走 BLOCK/RISK/SKIP 分支。"},
		)),
		schema("participation", "参与记录", "写入参与流水", fields(
			stringField("status", "初始状态", false, "CREATED"),
			booleanField("deduplicate", "参与去重", false, true),
		)),
		schema("grant-order", "发奖订单", "生成发奖订单", fields(
			enumField("provider", "发奖渠道", false, "INTERNAL", []string{"INTERNAL", "HTTP", "MQ", "WEBHOOK"}),
			numberField("expireSeconds", "过期秒数", false, 3600, rules(minValue(60), maxValue(604800))),
		)),
		schema("outbox", "消息发件箱", "写入 outbox", fields(
			enumField("channel", "发送渠道", false, "HTTP", []string{"HTTP", "MQ", "WEBHOOK"}),
			numberField("maxAttempts", "最大尝试次数", false, 3, rules(minValue(1), maxValue(10))),
		)),
		schema("http-call", "HTTP 发奖", "调用外部 HTTP 接口", fields(
			withDescription(stringField("url", "URL", true, "", rules(pattern(`^https?://`, "URL 必须以 http:// 或 https:// 开头。"), maxLength(2048))), "外部发奖或通知接口地址。"),
			enumField("method", "方法", false, "POST", []string{"GET", "POST", "PUT", "PATCH"}),
			numberField("timeoutSeconds", "超时秒数", false, 5, rules(minValue(1), maxValue(60))),
			objectField("headers", "请求头", false, nil),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("OutboxWorker"),
		)),
		schema("mq-publish", "MQ 发奖", "发布消息到队列", fields(
			withDescription(stringField("topic", "Topic", true, "", rules(minLength(1), maxLength(256))), "消息队列 topic 或 routing key。"),
			stringField("keyTemplate", "消息 Key 模板", false, "${eventId}", rules(maxLength(256))),
			objectField("headers", "消息头", false, nil),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("OutboxWorker"),
		)),
		schema("webhook", "Webhook 通知", "推送业务通知", fields(
			withDescription(stringField("receiverCode", "接收方编码", true, "", rules(pattern(`^[a-zA-Z0-9._-]+$`, "接收方编码只能包含字母、数字、点、下划线和短横线。"), maxLength(128))), "后端 Webhook receiver 配置中的业务编码。"),
			numberField("timeoutSeconds", "超时秒数", false, 5, rules(minValue(1), maxValue(60))),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("WebhookWorker"),
		)),
		groupedSchema("callback", "结果回调", "处理发奖渠道、外部系统或活动平台的回调结果。", groups(
			fieldGroup("provider", "回调来源", "定义允许哪个渠道回调以及如何识别回调事件。", fields(
				withDescription(stringField("provider", "渠道编码", true, "default", rules(minLength(1), maxLength(128), pattern(`^[a-zA-Z0-9._-]+$`, "渠道编码只能包含字母、数字、点、下划线和短横线。"))), "外部渠道或内部 provider 编码。"),
				withDescription(stringField("callbackIdPath", "回调 ID Path", true, "$.callbackId", rules(minLength(1), maxLength(160))), "从 provider_callback 中读取回调唯一 ID 的路径。"),
				withDescription(stringField("statusPath", "状态 Path", true, "$.status", rules(minLength(1), maxLength(160))), "从 provider_callback 中读取 SUCCESS / FAILED 等状态。"),
			)),
			fieldGroup("security", "安全和幂等", "控制验签、时间窗口和重复回调处理。", fields(
				booleanField("requireSignature", "验签", false, true),
				numberField("maxSkewSeconds", "最大时间偏移秒数", false, 300, rules(minValue(0), maxValue(86400))),
				enumField("duplicatePolicy", "重复回调策略", true, "RETURN_EXISTING", []string{"RETURN_EXISTING", "REJECT", "OVERWRITE"}),
			)),
			fieldGroup("execution", "执行语义", "回调节点解释回调结果，最终事实来自外部回调入口或对账任务。", fields(
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("RewardCallback/Reconciliation"),
			)),
		), preview(
			[]string{"provider_callback", "signature headers"},
			[]string{"callback_result"},
			[]string{"callback_result 会影响发奖订单最终状态，并可进入 audit / reconcile。"},
		)),
		schema("retry", "失败重试", "失败消息重试", fields(
			numberField("maxAttempts", "最大次数", false, 3, rules(minValue(1), maxValue(10))),
			numberField("backoffSeconds", "退避秒数", false, 30, rules(minValue(1), maxValue(3600))),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("RetryWorker/AdminAPI"),
		)),
		schema("dlq", "死信队列", "写入死信队列", fields(
			stringField("reasonTemplate", "原因模板", false, "${lastError}"),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("DLQWorker/AdminAPI"),
		)),
		groupedSchema("audit", "审计记录", "写入 workflow 关键节点的审计证据。", groups(
			fieldGroup("event", "审计事件", "定义审计动作、对象和严重级别。", fields(
				withDescription(stringField("action", "审计动作", true, "WORKFLOW_NODE", rules(minLength(2), maxLength(128))), "审计日志中的 action 字段。"),
				enumField("severity", "级别", true, "INFO", []string{"INFO", "WARN", "ERROR", "CRITICAL"}),
				withDescription(stringField("targetType", "对象类型", true, "WORKFLOW_NODE", rules(minLength(2), maxLength(128))), "审计日志中的 target type。"),
			)),
			fieldGroup("payload", "审计内容", "控制是否记录上下文和敏感字段处理。", fields(
				booleanField("includePayload", "包含上下文", false, true),
				withDefault(arrayField("redactPaths", "脱敏 Path", false, "需要从 payload 中脱敏的 JSONPath。", rules(maxItems(50))), []string{"$.token", "$.password"}),
				enumField("retentionPolicy", "保留策略", true, "STANDARD", []string{"STANDARD", "SHORT", "LONG", "LEGAL_HOLD"}),
			)),
		), preview(
			[]string{"stage_result", "node input/output", "execution metadata"},
			[]string{"audit_log"},
			[]string{"建议在 BLOCK、RISK、CALLBACK、RECONCILE 等关键路径后接 Audit。"},
		)),
		schema("reconcile", "对账校验", "一致性扫描和修复建议", fields(
			arrayField("targets", "对账对象", false, "grant、quota、callback、outbox。", rules(maxItems(10))),
			booleanField("autoRepair", "自动修复", false, false),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("ReconciliationJob"),
		)),
		schema("alert", "告警通知", "发送告警事件", fields(
			enumField("level", "级别", false, "WARN", []string{"INFO", "WARN", "ERROR", "CRITICAL"}),
			stringField("channel", "通知渠道", false, "default"),
			executionModeField("EXPLAIN_ONLY"),
			executionAuthorityField("Metrics/AlertingService"),
		)),
	}
	registry := WorkflowNodeConfigSchemaRegistry{schemas: map[string]WorkflowNodeConfigSchema{}}
	for _, item := range schemas {
		item.Category = defaultWorkflowNodeCategory(item.NodeType)
		registry.schemas[item.NodeType] = item
	}
	return registry
}

func defaultWorkflowNodeCategory(nodeType string) string {
	switch strings.TrimSpace(nodeType) {
	case "condition", "switch", "code", "decimal", "http-call", "mq-publish", "webhook", "callback":
		return "ADVANCED"
	case "retry", "dlq", "audit", "reconcile", "alert":
		return "OPS"
	default:
		return "CORE"
	}
}

func (r WorkflowNodeConfigSchemaRegistry) Get(nodeType string) (WorkflowNodeConfigSchema, bool) {
	schema, ok := r.schemas[strings.TrimSpace(nodeType)]
	return schema, ok
}

func (r WorkflowNodeConfigSchemaRegistry) List() []WorkflowNodeConfigSchema {
	out := make([]WorkflowNodeConfigSchema, 0, len(r.schemas))
	for _, schema := range r.schemas {
		out = append(out, schema)
	}
	slices.SortFunc(out, func(a, b WorkflowNodeConfigSchema) int {
		return strings.Compare(a.NodeType, b.NodeType)
	})
	return out
}

func schema(nodeType string, title string, description string, fields []WorkflowNodeConfigField) WorkflowNodeConfigSchema {
	return WorkflowNodeConfigSchema{
		NodeType:    nodeType,
		Version:     1,
		Title:       title,
		Description: description,
		Fields:      fields,
		Inputs:      defaultWorkflowNodeInputs(nodeType),
		Outputs:     defaultWorkflowNodeOutputs(nodeType),
	}
}

func groupedSchema(nodeType string, title string, description string, groups []WorkflowNodeConfigFieldGroup, preview WorkflowNodeConfigPreview) WorkflowNodeConfigSchema {
	var fields []WorkflowNodeConfigField
	for _, group := range groups {
		fields = append(fields, group.Fields...)
	}
	item := schema(nodeType, title, description, fields)
	item.FieldGroups = groups
	item.Preview = preview
	return item
}

func fields(items ...WorkflowNodeConfigField) []WorkflowNodeConfigField {
	return items
}

func groups(items ...WorkflowNodeConfigFieldGroup) []WorkflowNodeConfigFieldGroup {
	return items
}

func fieldGroup(key string, title string, description string, fields []WorkflowNodeConfigField) WorkflowNodeConfigFieldGroup {
	return WorkflowNodeConfigFieldGroup{Key: key, Title: title, Description: description, Fields: fields}
}

func preview(reads []string, writes []string, notes []string) WorkflowNodeConfigPreview {
	return WorkflowNodeConfigPreview{Reads: reads, Writes: writes, Notes: notes}
}

func defaultWorkflowNodeInputs(nodeType string) []WorkflowNodeInputVar {
	switch nodeType {
	case "event-in":
		return nil
	case "inbox":
		return inputVars(
			inputVar("raw_event", "Raw Event", "object", true, "事件入口输出的原始事件。"),
			inputVar("Idempotency-Key", "Idempotency Key", "string", false, "幂等键。"),
		)
	case "normalize":
		return inputVars(inputVar("raw_event", "Raw Event", "object", true, "待标准化的原始事件。"))
	case "match":
		return inputVars(
			inputVar("event_context", "Event Context", "object", true, "事件上下文。"),
			inputVar("activity_config", "Activity Config", "object", false, "活动配置。"),
		)
	case "condition":
		return inputVars(
			inputVar("fact", "Fact", "object", false, "事实数据。"),
			inputVar("condition_expr", "Condition Expression", "string", false, "条件表达式。"),
		)
	case "qualify":
		return inputVars(
			inputVar("event_context", "Event Context", "object", true, "事件上下文。"),
			inputVar("matched_activity", "Matched Activity", "object", true, "活动匹配结果。"),
		)
	case "switch":
		return inputVars(inputVar("stage_result", "Stage Result", "object", true, "上游阶段结果。"))
	case "fact-query":
		return inputVars(
			inputVar("subject", "Subject", "object", true, "业务主体。"),
			inputVar("fact_keys", "Fact Keys", "array", false, "需要查询的事实 key。"),
		)
	case "code":
		return inputVars(
			inputVar("context", "Context", "object", true, "工作流上下文。"),
			inputVar("input", "Input", "object", false, "上游输入对象。"),
		)
	case "reward":
		return inputVars(
			inputVar("qualified_context", "Qualified Context", "object", true, "资格通过后的上下文。"),
			inputVar("reward_config", "Reward Config", "object", false, "奖励配置。"),
		)
	case "decimal":
		return inputVars(inputVar("amount_expr", "Amount Expression", "number", true, "待处理金额表达式。"))
	case "limit":
		return inputVars(
			inputVar("subject", "Subject", "object", true, "参与主体。"),
			inputVar("activity_id", "Activity ID", "string", true, "活动 ID。"),
		)
	case "quota":
		return inputVars(
			inputVar("reward_items", "Reward Items", "array", true, "奖励明细。"),
			inputVar("limit_hold", "Limit Hold", "object", false, "限制占用结果。"),
		)
	case "participation":
		return inputVars(
			inputVar("qualified_context", "Qualified Context", "object", true, "资格上下文。"),
			inputVar("quota_hold", "Quota Hold", "object", false, "Quota 占用结果。"),
		)
	case "grant-order":
		return inputVars(
			inputVar("participation_id", "Participation ID", "string", true, "参与记录 ID。"),
			inputVar("reward_items", "Reward Items", "array", true, "奖励明细。"),
		)
	case "outbox":
		return inputVars(inputVar("grant_order", "Grant Order", "object", true, "发奖订单。"))
	case "http-call", "mq-publish":
		return inputVars(inputVar("outbox_message", "Outbox Message", "object", true, "发件箱消息。"))
	case "webhook":
		return inputVars(inputVar("stage_result", "Stage Result", "object", true, "阶段结果。"))
	case "callback":
		return inputVars(inputVar("provider_callback", "Provider Callback", "object", true, "渠道回调数据。"))
	case "retry":
		return inputVars(inputVar("failed_message", "Failed Message", "object", true, "失败消息。"))
	case "dlq":
		return inputVars(inputVar("failed_message", "Failed Message", "object", true, "失败消息。"))
	case "audit":
		return inputVars(inputVar("stage_result", "Stage Result", "object", true, "待审计阶段结果。"))
	case "reconcile":
		return inputVars(inputVar("date_range", "Date Range", "object", true, "对账时间范围。"))
	case "alert":
		return inputVars(inputVar("metric_event", "Metric Event", "object", true, "指标事件。"))
	default:
		return nil
	}
}

func inputVars(items ...WorkflowNodeInputVar) []WorkflowNodeInputVar {
	return items
}

func inputVar(key string, label string, valueType string, required bool, description string) WorkflowNodeInputVar {
	return WorkflowNodeInputVar{Key: key, Label: label, Type: valueType, Required: required, Description: description}
}

func defaultWorkflowNodeOutputs(nodeType string) []WorkflowNodeOutputVar {
	switch nodeType {
	case "event-in":
		return outputVars(
			outputVar("raw_event", "Raw Event", "object", "标准化前的事件载荷。"),
			outputVar("eventId", "Event ID", "string", "事件唯一 ID。"),
			outputVar("eventType", "Event Type", "string", "事件类型。"),
			outputVar("tenantId", "Tenant ID", "string", "租户 ID。"),
		)
	case "inbox":
		return outputVars(outputVar("event_context", "Event Context", "object", "幂等收件箱处理后的事件上下文。"))
	case "normalize":
		return outputVars(outputVar("normalized_event", "Normalized Event", "object", "字段标准化后的事件。"))
	case "match":
		return outputVars(outputVar("matched_activity", "Matched Activity", "object", "匹配到的活动上下文。"))
	case "condition":
		return outputVars(outputVar("IF", "IF", "boolean", "条件成立路径。"), outputVar("ELSE", "ELSE", "boolean", "条件不成立路径。"))
	case "qualify":
		return outputVars(
			outputVar("stage_result", "Stage Result", "object", "资格校验完整结果。"),
			outputVar("decision", "Decision", "string", "资格路由决策。"),
			outputVar("PASS", "PASS", "object", "资格通过结果。"),
			outputVar("BLOCK", "BLOCK", "object", "资格阻断结果。"),
			outputVar("RISK", "RISK", "object", "风险路径结果。"),
		)
	case "switch":
		return outputVars(outputVar("PASS", "PASS", "object", "通过分支。"), outputVar("BLOCK", "BLOCK", "object", "阻断分支。"), outputVar("SKIP", "SKIP", "object", "跳过分支。"))
	case "fact-query":
		return outputVars(outputVar("facts", "Facts", "object", "事实查询结果。"))
	case "code":
		return outputVars(outputVar("code_result", "Code Result", "object", "代码节点输出。"))
	case "reward":
		return outputVars(outputVar("reward_items", "Reward Items", "array", "奖励计算结果。"))
	case "decimal":
		return outputVars(outputVar("decimal_amount", "Decimal Amount", "number", "精度处理后的金额。"))
	case "limit":
		return outputVars(outputVar("limit_hold", "Limit Hold", "object", "次数限制占用结果。"))
	case "quota":
		return outputVars(outputVar("quota_hold", "Quota Hold", "object", "预算库存冻结结果。"))
	case "participation":
		return outputVars(outputVar("participation_id", "Participation ID", "string", "参与记录 ID。"))
	case "grant-order":
		return outputVars(outputVar("grant_order", "Grant Order", "object", "发奖订单。"))
	case "outbox":
		return outputVars(outputVar("HTTP", "HTTP", "object", "HTTP 发件箱消息。"), outputVar("MQ", "MQ", "object", "MQ 发件箱消息。"), outputVar("Webhook", "Webhook", "object", "Webhook 发件箱消息。"), outputVar("Failed", "Failed", "object", "失败消息。"))
	case "http-call":
		return outputVars(outputVar("http_result", "HTTP Result", "object", "HTTP 调用结果。"))
	case "mq-publish":
		return outputVars(outputVar("mq_result", "MQ Result", "object", "MQ 发布结果。"))
	case "webhook":
		return outputVars(outputVar("webhook_result", "Webhook Result", "object", "Webhook 推送结果。"))
	case "callback":
		return outputVars(outputVar("callback_result", "Callback Result", "object", "回调处理结果。"))
	case "retry":
		return outputVars(outputVar("retry_message", "Retry Message", "object", "重试消息。"))
	case "dlq":
		return outputVars(outputVar("dlq_record", "DLQ Record", "object", "死信记录。"))
	case "audit":
		return outputVars(outputVar("audit_log", "Audit Log", "object", "审计记录。"))
	case "reconcile":
		return outputVars(outputVar("reconcile_report", "Reconcile Report", "object", "对账报告。"))
	case "alert":
		return outputVars(outputVar("alert_event", "Alert Event", "object", "告警事件。"))
	default:
		return nil
	}
}

func outputVars(items ...WorkflowNodeOutputVar) []WorkflowNodeOutputVar {
	return items
}

func outputVar(key string, label string, valueType string, description string) WorkflowNodeOutputVar {
	return WorkflowNodeOutputVar{Key: key, Label: label, Type: valueType, Description: description}
}

func stringField(key string, label string, required bool, defaultValue string, validationRules ...[]WorkflowNodeConfigValidationRule) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigString, Required: required, Default: defaultValue, Description: "填写" + label + "。", ValidationRules: flattenRules(validationRules)}
}

func numberField(key string, label string, required bool, defaultValue float64, validationRules ...[]WorkflowNodeConfigValidationRule) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigNumber, Required: required, Default: defaultValue, Description: "填写" + label + "，需符合数值范围。", ValidationRules: flattenRules(validationRules)}
}

func booleanField(key string, label string, required bool, defaultValue bool) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigBoolean, Required: required, Default: defaultValue, Description: "开启或关闭" + label + "。"}
}

func arrayField(key string, label string, required bool, description string, validationRules ...[]WorkflowNodeConfigValidationRule) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigArray, Required: required, Description: description, ValidationRules: flattenRules(validationRules)}
}

func objectField(key string, label string, required bool, defaultValue any) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigObject, Required: required, Default: defaultValue, Description: "填写" + label + "的 JSON 对象。"}
}

func enumField(key string, label string, required bool, defaultValue string, options []string) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigEnum, Required: required, Default: defaultValue, Options: options, Description: "选择" + label + "。"}
}

func executionModeField(defaultValue string) WorkflowNodeConfigField {
	return withDescription(stringField("executionMode", "执行语义", false, defaultValue, rules(maxLength(64), pattern("^"+defaultValue+"$", "执行语义由节点契约固定，不能在画布配置中改写。"))), "说明该节点由 Executor 真实执行、解释执行，还是由外部入口/后台任务负责最终事实。")
}

func executionAuthorityField(defaultValue string) WorkflowNodeConfigField {
	return withDescription(stringField("executionAuthority", "执行权威", false, defaultValue, rules(maxLength(128))), "说明该节点最终副作用或事实状态归属的系统组件。")
}

func withDescription(field WorkflowNodeConfigField, description string) WorkflowNodeConfigField {
	field.Description = description
	return field
}

func withDefault(field WorkflowNodeConfigField, defaultValue any) WorkflowNodeConfigField {
	field.Default = defaultValue
	return field
}

func rules(items ...WorkflowNodeConfigValidationRule) []WorkflowNodeConfigValidationRule {
	return items
}

func minLength(value int) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMinLength, Value: value}
}

func maxLength(value int) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMaxLength, Value: value}
}

func pattern(value string, message string) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRulePattern, Value: value, Message: message}
}

func minValue(value float64) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMinValue, Value: value}
}

func maxValue(value float64) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMaxValue, Value: value}
}

func minItems(value int) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMinItems, Value: value}
}

func maxItems(value int) WorkflowNodeConfigValidationRule {
	return WorkflowNodeConfigValidationRule{Type: WorkflowNodeConfigRuleMaxItems, Value: value}
}

func flattenRules(groups [][]WorkflowNodeConfigValidationRule) []WorkflowNodeConfigValidationRule {
	var out []WorkflowNodeConfigValidationRule
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}
