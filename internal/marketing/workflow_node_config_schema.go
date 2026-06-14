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
		groupedSchema("event-in", "标准事件入口", "接收已标准化事件，校验 eventType、sourceSystem、tenant 后进入流程。", groups(
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
			[]string{"已标准化事件", "tenantId / eventType / eventId", "sourceSystem"},
			[]string{"raw_event", "eventId", "eventType", "tenantId"},
			[]string{"不监听 HTTP/MQ/Webhook，只负责标准事件进入 workflow 前的入口校验。"},
		)),
		groupedSchema("http-event", "HTTP 接口事件", "外部系统通过 OpenAPI 主动上报标准事件。", groups(
			fieldGroup("endpoint", "接口定义", "描述当前活动可使用的标准事件上报接口。", fields(
				withDescription(stringField("path", "Path", true, "/openapi/v1/events", rules(minLength(1), maxLength(160))), "生产入口由 OpenAPI HTTP handler 承担，workflow 节点只做配置解释。"),
				enumField("method", "Method", true, "POST", []string{"POST"}),
				enumField("payloadMode", "Payload 模式", true, "STANDARD_EVENT", []string{"STANDARD_EVENT"}),
			)),
			fieldGroup("security", "安全策略", "描述 HTTP 入口鉴权和请求限制。", fields(
				enumField("auth", "鉴权", true, "BEARER_TOKEN", []string{"BEARER_TOKEN"}),
				numberField("maxBodyBytes", "最大请求字节数", true, 1048576, rules(minValue(1024), maxValue(10485760))),
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("OpenAPIEventHandler"),
			)),
		), preview(
			[]string{"HTTP JSON request", "Authorization", "Idempotency-Key"},
			[]string{"standard_event"},
			[]string{"解释型入口节点；真实接收由 /openapi/v1/events 完成。"},
		)),
		groupedSchema("webhook-event", "Webhook 映射事件", "接收第三方 payload，签名校验后映射为标准事件。", groups(
			fieldGroup("receiver", "Receiver", "描述 webhook receiver 和来源白名单。", fields(
				withDescription(stringField("receiverCode", "Receiver Code", true, "payment-webhook", rules(minLength(1), maxLength(128))), "对应 /openapi/v1/webhooks/{receiverCode}。"),
				enumField("signature", "签名算法", true, "HMAC_SHA256", []string{"HMAC_SHA256"}),
				withDefault(arrayField("allowedTenantIds", "允许租户", true, "允许接入的 tenantId。", rules(minItems(1), maxItems(20))), []string{"site_001"}),
				withDefault(arrayField("allowedSourceSystems", "允许来源系统", true, "允许接入的 sourceSystem。", rules(minItems(1), maxItems(20))), []string{"payment"}),
			)),
			fieldGroup("mapping", "映射说明", "描述第三方 payload 到标准事件的映射版本。", fields(
				withDescription(stringField("mappingVersion", "Mapping Version", true, "v1", rules(minLength(1), maxLength(64))), "具体 mapping 仍由 receiver 配置管理。"),
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("WebhookReceiver"),
			)),
		), preview(
			[]string{"Webhook raw payload", "signature headers", "receiver mapping"},
			[]string{"standard_event"},
			[]string{"解释型入口节点；真实接收和签名校验由 WebhookReceiver 完成。"},
		)),
		groupedSchema("mq-event", "MQ Topic 事件", "监听 Kafka/RabbitMQ/Pulsar topic 并映射消息为标准事件。", groups(
			fieldGroup("consumer", "Consumer", "描述消息来源和消费组。", fields(
				enumField("broker", "Broker", true, "KAFKA", []string{"KAFKA", "RABBITMQ", "PULSAR"}),
				withDescription(stringField("topic", "Topic / Queue", true, "", rules(minLength(1), maxLength(256))), "待监听的 topic 或 queue。"),
				withDescription(stringField("consumerGroup", "Consumer Group", true, "", rules(minLength(1), maxLength(256))), "消费组必须稳定。"),
			)),
			fieldGroup("runtime", "运行策略", "当前阶段仅做能力占位，不启动真实 consumer。", fields(
				enumField("ackPolicy", "Ack 策略", true, "ACK_AFTER_INBOX_COMMIT", []string{"ACK_AFTER_INBOX_COMMIT"}),
				enumField("retryPolicy", "重试策略", true, "RETRY_THEN_DLQ", []string{"RETRY_THEN_DLQ"}),
				executionModeField("EXPLAIN_ONLY"),
				executionAuthorityField("MQTriggerWorker"),
			)),
		), preview(
			[]string{"MQ message", "topic / partition / offset"},
			[]string{"standard_event"},
			[]string{"待实现入口节点；真实 consumer 需要单独实现 offset、ack、retry 和 DLQ。"},
		)),
		groupedSchema("schedule-event", "定时事件", "按 cron 或延迟任务生成标准事件。", groups(
			fieldGroup("schedule", "调度", "描述定时触发规则。", fields(
				enumField("scheduleType", "调度类型", true, "CRON", []string{"CRON", "DELAY"}),
				withDescription(stringField("cron", "Cron", false, "0 0 * * *", rules(maxLength(128))), "CRON 调度表达式。"),
				withDescription(stringField("timezone", "时区", true, "UTC", rules(minLength(2), maxLength(64))), "例如 UTC、Asia/Shanghai。"),
			)),
			fieldGroup("runtime", "运行策略", "当前阶段仅做能力占位。", fields(
				enumField("misfirePolicy", "错过执行策略", true, "FIRE_ONCE", []string{"FIRE_ONCE", "SKIP", "CATCH_UP"}),
				numberField("jitterSeconds", "抖动秒数", false, 0, rules(minValue(0), maxValue(3600))),
				executionModeField("EXPLAIN_ONLY"),
				executionAuthorityField("Scheduler"),
			)),
		), preview(
			[]string{"cron / delay schedule", "event template"},
			[]string{"standard_event"},
			[]string{"待实现入口节点；同一计划时间必须生成稳定 bizId。"},
		)),
		groupedSchema("state-event", "状态监听事件", "监听 Redis、配置、指标或全局状态条件并生成标准事件。", groups(
			fieldGroup("watch", "监听源", "描述可恢复状态来源和路径。", fields(
				enumField("source", "状态来源", true, "REDIS", []string{"REDIS", "DATABASE", "CONFIG", "METRICS"}),
				withDescription(stringField("key", "Key / Path", true, "", rules(minLength(1), maxLength(256))), "不建议使用进程内临时全局变量。"),
				withDescription(stringField("condition", "触发条件", true, "", rules(minLength(1), maxLength(2000))), "条件命中后生成标准事件。"),
			)),
			fieldGroup("runtime", "运行策略", "当前阶段仅做能力占位。", fields(
				numberField("intervalSeconds", "轮询秒数", true, 10, rules(minValue(1), maxValue(3600))),
				numberField("debounceSeconds", "去抖秒数", false, 60, rules(minValue(0), maxValue(86400))),
				executionModeField("EXPLAIN_ONLY"),
				executionAuthorityField("StateWatcher"),
			)),
		), preview(
			[]string{"state snapshot", "condition", "last-seen cursor"},
			[]string{"standard_event"},
			[]string{"待实现入口节点；生产环境必须监听可恢复状态并记录 cursor。"},
		)),
		groupedSchema("manual-event", "人工触发事件", "管理台人工补偿、重放或手动执行标准事件。", groups(
			fieldGroup("permission", "权限", "描述人工触发需要的权限和审计。", fields(
				withDefault(arrayField("allowedRoles", "允许角色", true, "可人工触发的角色。", rules(minItems(1), maxItems(20))), []string{"OPS", "ADMIN"}),
				booleanField("requireReason", "要求原因", false, true),
				booleanField("requireApproval", "要求审批", false, false),
			)),
			fieldGroup("runtime", "运行策略", "区分 debug-run 和有副作用的生产人工触发。", fields(
				enumField("payloadMode", "Payload 模式", true, "STANDARD_EVENT", []string{"STANDARD_EVENT"}),
				withDescription(stringField("auditAction", "审计 Action", true, "MANUAL_EVENT_TRIGGERED", rules(minLength(1), maxLength(128))), "生产人工触发必须写审计。"),
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("AdminManualTriggerAPI"),
			)),
		), preview(
			[]string{"operator", "reason", "standard event payload"},
			[]string{"standard_event"},
			[]string{"生产人工触发必须进入 Event Inbox；debug-run 仍保持无副作用。"},
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
		groupedSchema("activity-state", "活动状态", "读取或更新用户活动状态、任务进度、签到状态和插件状态。", groups(
			fieldGroup("state-key", "状态 Key", "定义活动状态的主体、命名空间和 key。", fields(
				enumField("operation", "操作", true, "READ", []string{"READ", "WRITE", "UPSERT", "CAS"}),
				withDescription(stringField("namespace", "命名空间", true, "activity", rules(minLength(1), maxLength(128))), "状态命名空间，例如 checkin、task、lottery、red-envelope。"),
				withDescription(stringField("stateKeyTemplate", "State Key 模板", true, "${activityId}:${subject.id}", rules(minLength(1), maxLength(256))), "必须稳定定位一个用户或业务维度的活动状态。"),
			)),
			fieldGroup("state-data", "状态数据", "控制写入路径、版本和并发策略。", fields(
				withDescription(stringField("valuePath", "Value Path", false, "$.state", rules(maxLength(160))), "WRITE/UPSERT/CAS 时从上下文读取要写入的状态。"),
				booleanField("requireVersion", "要求版本", false, true),
				enumField("missingPolicy", "缺失策略", true, "EMPTY_STATE", []string{"EMPTY_STATE", "BLOCK", "CREATE"}),
			)),
		), preview(
			[]string{"event_context", "facts", "plugin_state"},
			[]string{"activity_state", "state_version"},
			[]string{"当前 Executor 输出可运行状态快照；真实跨执行持久化权威应接 ActivityPlugin/StateStore。"},
		)),
		groupedSchema("quota-check", "次数频控校验", "对用户、设备、IP、活动或租户维度做次数和频率校验。", groups(
			fieldGroup("counter", "计数维度", "定义频控 bucket、窗口和上限。", fields(
				enumField("scope", "限制范围", true, "USER", []string{"USER", "ACTIVITY", "TENANT", "BIZ", "IP", "DEVICE"}),
				withDescription(stringField("scopeKeyTemplate", "Scope Key 模板", true, "${subject.id}:${activityId}", rules(minLength(1), maxLength(256))), "用于组成频控 key。"),
				enumField("window", "窗口", true, "DAY", []string{"ACTIVITY", "DAY", "WEEK", "MONTH", "ROLLING_WINDOW"}),
				numberField("maxCount", "最大次数", true, 1, rules(minValue(1), maxValue(1000000))),
			)),
			fieldGroup("policy", "处理策略", "控制校验、占用和超限分支。", fields(
				enumField("consumeMode", "占用模式", true, "CHECK_ONLY", []string{"CHECK_ONLY", "ATOMIC", "RESERVE_ONLY"}),
				enumField("exceedAction", "超限动作", true, "BLOCK", []string{"BLOCK", "RISK", "SKIP"}),
				executionModeField("REAL_HANDLER"),
				executionAuthorityField("WorkflowExecutor/QuotaStore"),
			)),
		), preview(
			[]string{"subject", "activity_id", "event_context"},
			[]string{"quota_check", "limit_hold"},
			[]string{"quota-check 是 limit 的生产语义别名，专门表达次数/频控，不表达预算库存。"},
		)),
		groupedSchema("candidate-reward", "候选奖励", "根据活动配置和上游状态生成待发奖励候选，不直接产生资金副作用。", groups(
			fieldGroup("source", "奖励来源", "定义候选奖励从哪里生成。", fields(
				enumField("source", "来源", true, "ACTIVITY_REWARD_CONFIG", []string{"ACTIVITY_REWARD_CONFIG", "INPUT", "PLUGIN_RESULT"}),
				withDescription(stringField("rewardItemsPath", "奖励 Path", false, "$.reward_items", rules(maxLength(160))), "source=INPUT/PLUGIN_RESULT 时读取上游奖励数组。"),
				booleanField("includeDisabledRewards", "包含禁用奖励", false, false),
			)),
			fieldGroup("decision", "候选策略", "控制未通过资格、空候选和审计字段。", fields(
				enumField("notQualifiedAction", "未通过动作", true, "SKIP", []string{"SKIP", "BLOCK"}),
				enumField("emptyAction", "空候选动作", true, "BLOCK", []string{"BLOCK", "SKIP"}),
				booleanField("includeTrace", "包含 Trace", false, true),
			)),
		), preview(
			[]string{"qualified_context", "activity reward config", "plugin_result"},
			[]string{"candidate_rewards", "reward_items"},
			[]string{"候选奖励只决定“应该给什么”，真正发奖仍由 budget-reserve/quota、grant-order、outbox 完成。"},
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
		groupedSchema("combine", "组合节点", "用 Lua 或表达式把多个上游输入组合成一个结构化结果。", groups(
			fieldGroup("runtime", "运行时", "定义组合脚本语言、超时和输出限制。", fields(
				enumField("language", "语言", true, "LUA", []string{"LUA", "EXPRESSION"}),
				numberField("timeoutSeconds", "超时秒数", true, 5, rules(minValue(1), maxValue(60))),
				numberField("maxOutputBytes", "最大输出字节数", true, 65536, rules(minValue(1024), maxValue(1048576))),
			)),
			fieldGroup("script", "组合脚本", "从 input、ctx、node_output 读取多个来源并 emit 组合结果。", fields(
				withDescription(stringField("source", "代码", true, "emit({combined = ctx.node.input})", rules(minLength(1), maxLength(20000))), "Lua 中使用 emit({...}) 输出组合结果。"),
				withDescription(stringField("outputPath", "输出路径", true, "combined_result", rules(minLength(1), maxLength(128))), "组合结果写入上下文的 key。"),
				booleanField("allowNetwork", "允许网络访问", false, false),
			)),
		), preview(
			[]string{"上游多个输出", "input", "ctx", "node_output"},
			[]string{"combined_result"},
			[]string{"适合把事件、事实、资格结果、奖励配置组合成一个下游上下文。Lua 运行在受控沙箱中。"},
		)),
		groupedSchema("split", "拆分节点", "用 Lua 或表达式把一个结构化输入拆分成数组或多个输出分片。", groups(
			fieldGroup("runtime", "运行时", "定义拆分脚本语言、超时和输出限制。", fields(
				enumField("language", "语言", true, "LUA", []string{"LUA", "EXPRESSION"}),
				numberField("timeoutSeconds", "超时秒数", true, 5, rules(minValue(1), maxValue(60))),
				numberField("maxOutputBytes", "最大输出字节数", true, 65536, rules(minValue(1024), maxValue(1048576))),
			)),
			fieldGroup("script", "拆分脚本", "从 input、ctx、node_output 读取对象并 emit 拆分结果。", fields(
				withDescription(stringField("source", "代码", true, "emit({items = ctx.node.input.items})", rules(minLength(1), maxLength(20000))), "Lua 中建议输出 items 数组，例如 emit({items = {...}})。"),
				withDescription(stringField("outputPath", "输出路径", true, "split_items", rules(minLength(1), maxLength(128))), "拆分结果写入上下文的 key。"),
				booleanField("allowNetwork", "允许网络访问", false, false),
			)),
		), preview(
			[]string{"结构化输入", "input", "ctx", "node_output"},
			[]string{"split_items", "split_result"},
			[]string{"适合把批量权益、名单、奖励明细拆成下游可循环/路由处理的分片。Lua 运行在受控沙箱中。"},
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
		groupedSchema("budget-reserve", "预算库存锁定", "在发奖前锁定活动预算、奖品库存或用户额度，避免并发超发。", groups(
			fieldGroup("reserve-target", "锁定对象", "定义要锁定的预算、库存或额度维度。", fields(
				withDescription(stringField("quotaCode", "Quota Code", true, "DEFAULT_REWARD_BUDGET", rules(minLength(2), maxLength(128))), "后端 quota 资源编码。"),
				enumField("quotaType", "Quota 类型", true, "BUDGET", []string{"BUDGET", "STOCK", "USER_LIMIT", "DAILY_LIMIT"}),
				enumField("scope", "作用范围", true, "ACTIVITY", []string{"ACTIVITY", "USER", "TENANT", "WORKSPACE"}),
				withDescription(stringField("scopeKeyTemplate", "Scope Key 模板", true, "${activityId}", rules(minLength(1), maxLength(256))), "用于组成锁定 bucket。"),
			)),
			fieldGroup("reserve-policy", "锁定策略", "控制锁定数量、释放策略和失败动作。", fields(
				withDescription(stringField("amountField", "锁定数量字段", true, "candidate_rewards[0].amount", rules(minLength(1), maxLength(160))), "从 candidate_rewards/reward_items 或上下文读取锁定金额/数量。"),
				enumField("reserveMode", "锁定模式", true, "ATOMIC", []string{"ATOMIC", "RESERVE_ONLY", "CHECK_ONLY"}),
				enumField("failAction", "锁定失败动作", true, "BLOCK", []string{"BLOCK", "RISK", "SKIP"}),
				booleanField("releaseOnFailure", "失败释放", false, true),
			)),
		), preview(
			[]string{"candidate_rewards", "reward_items", "quota_check"},
			[]string{"budget_reservation", "quota_hold"},
			[]string{"budget-reserve 是 quota 的生产语义别名，强调发奖前的预算/库存锁定。"},
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
		groupedSchema("approval-gate", "审批网关", "根据金额、风险或活动策略决定自动通过、阻断或进入人工审批。", groups(
			fieldGroup("policy", "审批策略", "定义哪些场景需要人工审批。", fields(
				enumField("mode", "模式", true, "AUTO_BELOW_THRESHOLD", []string{"AUTO", "AUTO_BELOW_THRESHOLD", "MANUAL_REQUIRED", "RISK_REQUIRED"}),
				numberField("amountThreshold", "金额阈值", false, 1000, rules(minValue(0))),
				withDefault(arrayField("riskLabels", "风险标签", false, "命中这些风险标签时进入审批。", rules(maxItems(50))), []string{"HIGH_RISK"}),
			)),
			fieldGroup("runtime", "运行权威", "审批状态由外部审批 API 和人工操作决定。", fields(
				enumField("pendingAction", "待审动作", true, "PAUSE", []string{"PAUSE", "BLOCK", "SKIP"}),
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("ActivityApproval/AdminAPI"),
			)),
		), preview(
			[]string{"candidate_rewards", "risk_result", "operator"},
			[]string{"approval_decision"},
			[]string{"Executor 只生成审批决策；真实审批流由 ActivityApproval/AdminAPI 负责。"},
		)),
		groupedSchema("compensation", "业务补偿", "描述发奖、预算、回调、通知失败后的补偿动作。", groups(
			fieldGroup("trigger", "触发条件", "定义何时创建补偿动作。", fields(
				enumField("triggerOn", "触发状态", true, "FAILED", []string{"FAILED", "TIMEOUT", "PARTIAL", "DRIFT"}),
				withDescription(stringField("sourcePath", "来源 Path", true, "$.stage_result", rules(minLength(1), maxLength(160))), "从上游输出读取失败或漂移来源。"),
				enumField("action", "补偿动作", true, "RETRY_CALLBACK", []string{"RETRY_CALLBACK", "RELEASE_BUDGET", "QUERY_PROVIDER", "REPLAY_EVENT", "MANUAL_REVIEW"}),
			)),
			fieldGroup("runtime", "执行权威", "补偿副作用应由后台任务或人工 API 执行。", fields(
				numberField("maxAttempts", "最大尝试次数", false, 3, rules(minValue(1), maxValue(10))),
				executionModeField("EXPLAIN_ONLY"),
				executionAuthorityField("CompensationWorker/AdminAPI"),
			)),
		), preview(
			[]string{"failed stage", "grant_order", "quota_hold"},
			[]string{"compensation_plan"},
			[]string{"当前节点生成补偿计划，不直接释放预算或重放消息。"},
		)),
		groupedSchema("random-pick", "随机抽取", "执行可审计的权重随机、奖池抽取或分桶随机。", groups(
			fieldGroup("random", "随机策略", "定义随机来源、候选集合和种子。", fields(
				enumField("mode", "模式", true, "WEIGHTED", []string{"WEIGHTED", "UNIFORM", "POOL", "BUCKET"}),
				withDescription(stringField("itemsPath", "候选 Path", false, "$.candidate_rewards", rules(maxLength(160))), "候选数组路径。"),
				withDescription(stringField("seedTemplate", "Seed 模板", true, "${tenantId}:${activityId}:${eventId}", rules(minLength(1), maxLength(256))), "生产随机必须可审计、可复现。"),
			)),
			fieldGroup("output", "输出", "控制抽取数量和空候选策略。", fields(
				numberField("pickCount", "抽取数量", true, 1, rules(minValue(1), maxValue(100))),
				enumField("emptyAction", "空候选动作", true, "SKIP", []string{"SKIP", "BLOCK"}),
				executionModeField("REAL_HANDLER"),
				executionAuthorityField("WorkflowExecutor"),
			)),
		), preview(
			[]string{"candidate_rewards", "seed"},
			[]string{"random_pick", "selected_rewards"},
			[]string{"避免在 Lua 内直接使用不可追溯随机。"},
		)),
		groupedSchema("plugin-execute", "插件执行", "调用活动插件动作，例如签到、抽奖、拆红包或任务结算。", groups(
			fieldGroup("plugin", "插件", "定义插件编码、动作和输入路径。", fields(
				withDescription(stringField("pluginCode", "插件 Code", true, "checkin", rules(minLength(1), maxLength(128))), "例如 checkin、lottery、red-envelope。"),
				withDescription(stringField("action", "动作", true, "EXECUTE", rules(minLength(1), maxLength(128))), "插件内动作，例如 SIGN_IN、DRAW、OPEN_PACKET。"),
				withDescription(stringField("inputPath", "输入 Path", false, "$", rules(maxLength(160))), "插件执行输入来源。"),
			)),
			fieldGroup("runtime", "执行权威", "复杂领域状态由 ActivityPlugin 负责。", fields(
				enumField("failureAction", "失败动作", true, "BLOCK", []string{"BLOCK", "SKIP", "RISK"}),
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("ActivityPlugin"),
			)),
		), preview(
			[]string{"event_context", "activity_state", "plugin_context"},
			[]string{"plugin_result", "candidate_rewards"},
			[]string{"Workflow 负责编排，插件负责领域状态、库存、随机和领取细节。"},
		)),
		groupedSchema("ab-bucket", "AB 分桶", "按稳定主体把用户分配到实验桶或运营策略桶。", groups(
			fieldGroup("bucket", "分桶", "定义实验编码、主体和桶比例。", fields(
				withDescription(stringField("experimentKey", "实验 Key", true, "default-experiment", rules(minLength(1), maxLength(128))), "实验或策略编码。"),
				withDescription(stringField("subjectKeyTemplate", "主体 Key 模板", true, "${subject.id}", rules(minLength(1), maxLength(256))), "稳定分桶主体。"),
				withDefault(arrayField("buckets", "桶列表", true, "桶配置，例如 A:50,B:50。", rules(minItems(1), maxItems(20))), []string{"A:50", "B:50"}),
			)),
			fieldGroup("runtime", "执行语义", "分桶在 Executor 内可确定性计算。", fields(
				executionModeField("REAL_HANDLER"),
				executionAuthorityField("WorkflowExecutor"),
			)),
		), preview(
			[]string{"subject", "experimentKey"},
			[]string{"ab_bucket"},
			[]string{"用于活动策略分流，不应替代风控或资格规则。"},
		)),
		groupedSchema("risk-score", "风险评分", "根据事实、规则或外部风控结果输出风险评分和标签。", groups(
			fieldGroup("risk", "风险策略", "定义风险来源和阈值。", fields(
				enumField("source", "来源", true, "FACTS", []string{"FACTS", "RULE_RESULT", "EXTERNAL_PROVIDER"}),
				withDescription(stringField("scorePath", "分数 Path", false, "$.facts.riskScore", rules(maxLength(160))), "从事实或 provider 结果读取风险分。"),
				numberField("blockThreshold", "阻断阈值", true, 90, rules(minValue(0), maxValue(100))),
				numberField("reviewThreshold", "复核阈值", true, 70, rules(minValue(0), maxValue(100))),
			)),
			fieldGroup("runtime", "运行策略", "外部 provider 暂由 fact-query 或风控服务承接。", fields(
				enumField("failureAction", "失败动作", true, "REVIEW", []string{"REVIEW", "BLOCK", "PASS"}),
				executionModeField("REAL_HANDLER"),
				executionAuthorityField("WorkflowExecutor/FactService"),
			)),
		), preview(
			[]string{"facts", "rule_result", "provider_result"},
			[]string{"risk_result"},
			[]string{"高风险可接 approval-gate 或 audit/alert。"},
		)),
		groupedSchema("settlement-window", "结算窗口", "限制活动发奖、对账或结算只能在指定窗口内执行。", groups(
			fieldGroup("window", "窗口", "定义允许执行的时间范围。", fields(
				enumField("windowType", "窗口类型", true, "DAILY", []string{"DAILY", "WEEKLY", "MONTHLY", "CUSTOM"}),
				withDescription(stringField("start", "开始", true, "00:00", rules(minLength(1), maxLength(64))), "窗口开始时间。"),
				withDescription(stringField("end", "结束", true, "23:59", rules(minLength(1), maxLength(64))), "窗口结束时间。"),
				withDescription(stringField("timezone", "时区", true, "UTC", rules(minLength(2), maxLength(64))), "例如 UTC、Asia/Shanghai。"),
			)),
			fieldGroup("policy", "策略", "控制窗口外如何处理。", fields(
				enumField("outsideAction", "窗口外动作", true, "SKIP", []string{"SKIP", "BLOCK", "DEFER"}),
				executionModeField("REAL_HANDLER"),
				executionAuthorityField("WorkflowExecutor/Scheduler"),
			)),
		), preview(
			[]string{"event time", "current time", "activity timezone"},
			[]string{"settlement_window"},
			[]string{"DEFER 的真实调度需要 Scheduler；Executor 当前只输出决策。"},
		)),
		groupedSchema("manual-adjust", "人工调整", "记录人工补偿、修正、加减奖励或状态修复意图。", groups(
			fieldGroup("adjust", "调整", "定义人工调整类型和对象。", fields(
				enumField("adjustType", "调整类型", true, "REWARD", []string{"REWARD", "BUDGET", "STATE", "GRANT_ORDER"}),
				withDescription(stringField("targetPath", "对象 Path", true, "$.grant_order", rules(minLength(1), maxLength(160))), "被调整对象路径。"),
				booleanField("requireReason", "要求原因", false, true),
				booleanField("requireApproval", "要求审批", false, true),
			)),
			fieldGroup("runtime", "执行权威", "真实调整必须由 AdminAPI 写审计并执行。", fields(
				executionModeField("EXTERNAL_ENDPOINT"),
				executionAuthorityField("AdminManualAdjustAPI"),
			)),
		), preview(
			[]string{"operator", "reason", "target"},
			[]string{"manual_adjustment"},
			[]string{"生产调整不应由普通 workflow 自动执行，必须保留人工审计链。"},
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
		schema("reward-reconcile", "奖励对账", "对发奖订单、预算锁定、回调和 outbox 做奖励链路一致性校验。", fields(
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
	case "event-in", "http-event", "webhook-event", "mq-event", "schedule-event", "state-event", "manual-event":
		return "EVENT_INPUT"
	case "condition", "switch", "code", "combine", "split", "decimal", "http-call", "mq-publish", "webhook", "callback", "approval-gate", "compensation", "random-pick", "plugin-execute", "ab-bucket", "risk-score", "settlement-window", "manual-adjust":
		return "ADVANCED"
	case "retry", "dlq", "audit", "reconcile", "reward-reconcile", "alert":
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
	case "event-in", "http-event", "webhook-event", "mq-event", "schedule-event", "state-event", "manual-event":
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
	case "activity-state":
		return inputVars(
			inputVar("event_context", "Event Context", "object", false, "事件上下文。"),
			inputVar("state_patch", "State Patch", "object", false, "状态写入或 CAS patch。"),
		)
	case "quota-check":
		return inputVars(
			inputVar("subject", "Subject", "object", true, "参与主体。"),
			inputVar("activity_id", "Activity ID", "string", true, "活动 ID。"),
		)
	case "candidate-reward":
		return inputVars(
			inputVar("qualified_context", "Qualified Context", "object", false, "资格通过后的上下文。"),
			inputVar("plugin_result", "Plugin Result", "object", false, "插件输出。"),
		)
	case "code":
		return inputVars(
			inputVar("context", "Context", "object", true, "工作流上下文。"),
			inputVar("input", "Input", "object", false, "上游输入对象。"),
		)
	case "combine":
		return inputVars(
			inputVar("items", "Items", "array", false, "待组合的上游对象数组。"),
			inputVar("context", "Context", "object", false, "可选组合上下文。"),
		)
	case "split":
		return inputVars(
			inputVar("payload", "Payload", "object", false, "待拆分的结构化对象。"),
			inputVar("items", "Items", "array", false, "待拆分的数组。"),
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
	case "budget-reserve":
		return inputVars(
			inputVar("candidate_rewards", "Candidate Rewards", "array", false, "候选奖励明细。"),
			inputVar("reward_items", "Reward Items", "array", false, "奖励明细。"),
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
	case "approval-gate":
		return inputVars(
			inputVar("candidate_rewards", "Candidate Rewards", "array", false, "候选奖励明细。"),
			inputVar("risk_result", "Risk Result", "object", false, "风险结果。"),
		)
	case "compensation":
		return inputVars(inputVar("stage_result", "Stage Result", "object", true, "失败、超时或漂移阶段结果。"))
	case "random-pick":
		return inputVars(inputVar("candidate_rewards", "Candidate Rewards", "array", false, "候选奖励数组。"))
	case "plugin-execute":
		return inputVars(
			inputVar("event_context", "Event Context", "object", false, "事件上下文。"),
			inputVar("activity_state", "Activity State", "object", false, "活动状态。"),
		)
	case "ab-bucket":
		return inputVars(inputVar("subject", "Subject", "object", false, "分桶主体。"))
	case "risk-score":
		return inputVars(inputVar("facts", "Facts", "object", false, "事实和风控特征。"))
	case "settlement-window":
		return inputVars(inputVar("event_context", "Event Context", "object", false, "事件上下文。"))
	case "manual-adjust":
		return inputVars(inputVar("operator", "Operator", "object", false, "人工操作人。"))
	case "retry":
		return inputVars(inputVar("failed_message", "Failed Message", "object", true, "失败消息。"))
	case "dlq":
		return inputVars(inputVar("failed_message", "Failed Message", "object", true, "失败消息。"))
	case "audit":
		return inputVars(inputVar("stage_result", "Stage Result", "object", true, "待审计阶段结果。"))
	case "reconcile", "reward-reconcile":
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
	case "http-event", "webhook-event", "mq-event", "schedule-event", "state-event", "manual-event":
		return outputVars(
			outputVar("standard_event", "Standard Event", "object", "由入口适配器产出的标准事件。"),
			outputVar("source", "Source Metadata", "object", "入口来源、trace、request、offset 或 operator 信息。"),
			outputVar("adapter", "Adapter Metadata", "object", "入口适配器和 mapping 版本信息。"),
		)
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
	case "combine":
		return outputVars(outputVar("combined_result", "Combined Result", "object", "组合后的结构化结果。"))
	case "split":
		return outputVars(
			outputVar("split_items", "Split Items", "array", "拆分后的项目数组。"),
			outputVar("split_result", "Split Result", "object", "完整拆分输出对象。"),
		)
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
	case "activity-state":
		return outputVars(
			outputVar("activity_state", "Activity State", "object", "活动状态快照。"),
			outputVar("state_version", "State Version", "number", "状态版本。"),
		)
	case "quota-check":
		return outputVars(
			outputVar("quota_check", "Quota Check", "object", "次数/频控校验结果。"),
			outputVar("limit_hold", "Limit Hold", "object", "兼容 limit 节点的限制占用结果。"),
		)
	case "candidate-reward":
		return outputVars(
			outputVar("candidate_rewards", "Candidate Rewards", "array", "待发奖励候选。"),
			outputVar("reward_items", "Reward Items", "array", "兼容下游发奖节点的奖励明细。"),
		)
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
	case "budget-reserve":
		return outputVars(
			outputVar("budget_reservation", "Budget Reservation", "object", "预算库存锁定结果。"),
			outputVar("quota_hold", "Quota Hold", "object", "兼容 quota 节点的锁定结果。"),
		)
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
	case "approval-gate":
		return outputVars(outputVar("approval_decision", "Approval Decision", "object", "审批决策。"))
	case "compensation":
		return outputVars(outputVar("compensation_plan", "Compensation Plan", "object", "补偿计划。"))
	case "random-pick":
		return outputVars(
			outputVar("random_pick", "Random Pick", "object", "随机抽取审计结果。"),
			outputVar("selected_rewards", "Selected Rewards", "array", "选中的奖励。"),
		)
	case "plugin-execute":
		return outputVars(
			outputVar("plugin_result", "Plugin Result", "object", "插件执行结果。"),
			outputVar("candidate_rewards", "Candidate Rewards", "array", "插件生成的候选奖励。"),
		)
	case "ab-bucket":
		return outputVars(outputVar("ab_bucket", "AB Bucket", "object", "分桶结果。"))
	case "risk-score":
		return outputVars(outputVar("risk_result", "Risk Result", "object", "风险评分结果。"))
	case "settlement-window":
		return outputVars(outputVar("settlement_window", "Settlement Window", "object", "结算窗口判定。"))
	case "manual-adjust":
		return outputVars(outputVar("manual_adjustment", "Manual Adjustment", "object", "人工调整意图。"))
	case "retry":
		return outputVars(outputVar("retry_message", "Retry Message", "object", "重试消息。"))
	case "dlq":
		return outputVars(outputVar("dlq_record", "DLQ Record", "object", "死信记录。"))
	case "audit":
		return outputVars(outputVar("audit_log", "Audit Log", "object", "审计记录。"))
	case "reconcile":
		return outputVars(outputVar("reconcile_report", "Reconcile Report", "object", "对账报告。"))
	case "reward-reconcile":
		return outputVars(outputVar("reward_reconcile_report", "Reward Reconcile Report", "object", "奖励链路对账报告。"))
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
