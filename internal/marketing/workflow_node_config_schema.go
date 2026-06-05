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
	NodeType    string                    `json:"nodeType"`
	Version     int                       `json:"version"`
	Title       string                    `json:"title"`
	Description string                    `json:"description,omitempty"`
	Fields      []WorkflowNodeConfigField `json:"fields"`
	Inputs      []WorkflowNodeInputVar    `json:"inputs"`
	Outputs     []WorkflowNodeOutputVar   `json:"outputs"`
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
		schema("event-in", "事件入口", "标准事件接入", fields(
			arrayField("eventTypes", "事件类型", false, "允许进入工作流的事件类型列表。"),
			arrayField("sourceSystems", "来源系统", false, "允许接入的来源系统列表。"),
		)),
		schema("inbox", "幂等收件箱", "事件幂等与去重", fields(
			stringField("idempotencyKeyTemplate", "幂等 Key 模板", false, "${tenantId}:${eventId}", rules(minLength(3), maxLength(256))),
			numberField("ttlSeconds", "幂等保留秒数", false, 86400, rules(minValue(60), maxValue(2592000))),
		)),
		schema("normalize", "事件标准化", "标准化事件字段", fields(
			objectField("mapping", "字段映射", false, nil),
			objectField("defaults", "默认值", false, nil),
		)),
		schema("match", "活动匹配", "匹配活动和事件上下文", fields(
			enumField("mode", "匹配模式", false, "FIRST_MATCH", []string{"STRICT", "FIRST_MATCH", "ALL_MATCH"}),
			booleanField("requireActiveActivity", "仅匹配已上线活动", false, true),
		)),
		schema("condition", "条件判断", "布尔条件分支", fields(
			stringField("expression", "表达式", false, ""),
			enumField("missingFieldPolicy", "缺失字段策略", false, "FALSE", []string{"FALSE", "ERROR", "SKIP"}),
		)),
		schema("qualify", "资格校验", "资格、风控和名单规则", fields(
			arrayField("ruleCodes", "规则编码", false, "要执行的资格规则编码。"),
			enumField("riskPolicy", "风控策略", false, "BLOCK", []string{"PASS", "BLOCK", "RISK"}),
		)),
		schema("switch", "条件分支", "按阶段结果选择路径", fields(
			stringField("sourcePath", "分支字段路径", false, "stage_result.status"),
			arrayField("cases", "分支项", false, "分支匹配项。"),
		)),
		schema("fact-query", "事实查询", "查询外部事实数据", fields(
			arrayField("factKeys", "事实 Key", false, "需要查询的事实字段。", rules(maxItems(50))),
			numberField("timeoutSeconds", "超时秒数", false, 3, rules(minValue(1), maxValue(30))),
		)),
		schema("code", "代码执行", "通过代码片段处理上下文并输出结果", fields(
			enumField("language", "语言", true, "JAVASCRIPT", []string{"JAVASCRIPT", "PYTHON", "EXPRESSION"}),
			withDescription(stringField("source", "代码", true, "", rules(minLength(1), maxLength(20000))), "节点执行的代码片段或表达式。"),
			numberField("timeoutSeconds", "超时秒数", false, 5, rules(minValue(1), maxValue(60))),
			stringField("outputPath", "输出路径", false, "code_result", rules(maxLength(128))),
		)),
		schema("reward", "奖励计算", "计算奖励结果", fields(
			enumField("calculateType", "计算类型", false, "FIXED", []string{"FIXED", "RATIO", "LADDER", "RANDOM", "PROBABILITY", "POOL_SPLIT"}),
			numberField("amount", "固定金额", false, 0, rules(minValue(0))),
			stringField("currency", "币种", false, "USD", rules(pattern(`^[A-Z]{3}$`, "币种必须是 3 位大写字母。"))),
		)),
		schema("decimal", "精度处理", "金额和比例精度处理", fields(
			numberField("scale", "小数位", false, 2, rules(minValue(0), maxValue(18))),
			enumField("roundingMode", "舍入模式", false, "HALF_UP", []string{"HALF_UP", "DOWN", "UP"}),
		)),
		schema("limit", "次数限制", "限制参与次数", fields(
			enumField("scope", "限制范围", false, "USER", []string{"USER", "ACTIVITY", "TENANT", "BIZ"}),
			numberField("maxCount", "最大次数", false, 1, rules(minValue(1))),
			stringField("period", "周期", false, "ACTIVITY", rules(minLength(3), maxLength(64))),
		)),
		schema("quota", "预算库存扣减", "预算和库存原子扣减", fields(
			stringField("quotaCode", "Quota Code", false, ""),
			enumField("consumeMode", "扣减模式", false, "ATOMIC", []string{"ATOMIC", "BEST_EFFORT"}),
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
		)),
		schema("mq-publish", "MQ 发奖", "发布消息到队列", fields(
			withDescription(stringField("topic", "Topic", true, "", rules(minLength(1), maxLength(256))), "消息队列 topic 或 routing key。"),
			stringField("keyTemplate", "消息 Key 模板", false, "${eventId}", rules(maxLength(256))),
			objectField("headers", "消息头", false, nil),
		)),
		schema("webhook", "Webhook 通知", "推送业务通知", fields(
			withDescription(stringField("receiverCode", "接收方编码", true, "", rules(pattern(`^[a-zA-Z0-9._-]+$`, "接收方编码只能包含字母、数字、点、下划线和短横线。"), maxLength(128))), "后端 Webhook receiver 配置中的业务编码。"),
			numberField("timeoutSeconds", "超时秒数", false, 5, rules(minValue(1), maxValue(60))),
		)),
		schema("callback", "结果回调", "处理发奖回调", fields(
			stringField("provider", "渠道编码", false, ""),
			booleanField("requireSignature", "验签", false, true),
		)),
		schema("retry", "失败重试", "失败消息重试", fields(
			numberField("maxAttempts", "最大次数", false, 3, rules(minValue(1), maxValue(10))),
			numberField("backoffSeconds", "退避秒数", false, 30, rules(minValue(1), maxValue(3600))),
		)),
		schema("dlq", "死信队列", "写入死信队列", fields(
			stringField("reasonTemplate", "原因模板", false, "${lastError}"),
		)),
		schema("audit", "审计记录", "写入审计证据", fields(
			stringField("action", "审计动作", false, "WORKFLOW_NODE"),
			booleanField("includePayload", "包含上下文", false, true),
		)),
		schema("reconcile", "对账校验", "一致性扫描和修复建议", fields(
			arrayField("targets", "对账对象", false, "grant、quota、callback、outbox。", rules(maxItems(10))),
			booleanField("autoRepair", "自动修复", false, false),
		)),
		schema("alert", "告警通知", "发送告警事件", fields(
			enumField("level", "级别", false, "WARN", []string{"INFO", "WARN", "ERROR", "CRITICAL"}),
			stringField("channel", "通知渠道", false, "default"),
		)),
	}
	registry := WorkflowNodeConfigSchemaRegistry{schemas: map[string]WorkflowNodeConfigSchema{}}
	for _, item := range schemas {
		registry.schemas[item.NodeType] = item
	}
	return registry
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

func fields(items ...WorkflowNodeConfigField) []WorkflowNodeConfigField {
	return items
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
		return outputVars(outputVar("PASS", "PASS", "object", "资格通过结果。"), outputVar("BLOCK", "BLOCK", "object", "资格阻断结果。"), outputVar("RISK", "RISK", "object", "风险路径结果。"))
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
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigString, Required: required, Default: defaultValue, ValidationRules: flattenRules(validationRules)}
}

func numberField(key string, label string, required bool, defaultValue float64, validationRules ...[]WorkflowNodeConfigValidationRule) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigNumber, Required: required, Default: defaultValue, ValidationRules: flattenRules(validationRules)}
}

func booleanField(key string, label string, required bool, defaultValue bool) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigBoolean, Required: required, Default: defaultValue}
}

func arrayField(key string, label string, required bool, description string, validationRules ...[]WorkflowNodeConfigValidationRule) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigArray, Required: required, Description: description, ValidationRules: flattenRules(validationRules)}
}

func objectField(key string, label string, required bool, defaultValue any) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigObject, Required: required, Default: defaultValue}
}

func enumField(key string, label string, required bool, defaultValue string, options []string) WorkflowNodeConfigField {
	return WorkflowNodeConfigField{Key: key, Label: label, Type: WorkflowNodeConfigEnum, Required: required, Default: defaultValue, Options: options}
}

func withDescription(field WorkflowNodeConfigField, description string) WorkflowNodeConfigField {
	field.Description = description
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
