package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (s *Service) ListWorkflowDraftTemplates(ctx context.Context, query marketing.WorkflowDraftTemplateQuery) ([]marketing.WorkflowDraftTemplate, error) {
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 200
	}
	items := builtinWorkflowDraftTemplates(s.now())
	if query.TemplateID != "" || query.Source != "" || query.Status != "" {
		filtered := items[:0]
		for _, item := range items {
			if query.TemplateID != "" && item.ID != query.TemplateID {
				continue
			}
			if query.Source != "" && item.Source != query.Source {
				continue
			}
			if query.Status != "" && item.Status != query.Status {
				continue
			}
			filtered = append(filtered, item)
		}
		items = filtered
	}
	if query.Source != marketing.WorkflowDraftTemplateBuiltin {
		saved, err := s.repository.ListWorkflowDraftTemplates(ctx, query)
		if err != nil {
			return nil, err
		}
		items = append(items, saved...)
	}
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (s *Service) ListWorkflowDraftTemplateVersions(ctx context.Context, templateID string) ([]marketing.WorkflowDraftTemplate, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, marketing.ErrNotFound
	}
	var out []marketing.WorkflowDraftTemplate
	for _, item := range builtinWorkflowDraftTemplates(s.now()) {
		if item.ID == templateID {
			out = append(out, item)
		}
	}
	saved, err := s.repository.ListWorkflowDraftTemplates(ctx, marketing.WorkflowDraftTemplateQuery{TemplateID: templateID, Limit: 200})
	if err != nil {
		return nil, err
	}
	out = append(out, saved...)
	if len(out) == 0 {
		return nil, marketing.ErrNotFound
	}
	return out, nil
}

func (s *Service) GetWorkflowDraftTemplate(ctx context.Context, templateID string, versionID string, version int) (marketing.WorkflowDraftTemplate, error) {
	templateID = strings.TrimSpace(templateID)
	for _, item := range builtinWorkflowDraftTemplates(s.now()) {
		if item.ID != templateID {
			continue
		}
		if versionID != "" && item.VersionID != versionID {
			continue
		}
		if version > 0 && item.Version != version {
			continue
		}
		return item, nil
	}
	return s.repository.GetWorkflowDraftTemplate(ctx, templateID, strings.TrimSpace(versionID), version)
}

func (s *Service) ApplyWorkflowDraftTemplate(ctx context.Context, activityID string, cmd marketing.ApplyWorkflowDraftTemplateCommand) (marketing.WorkflowDraft, error) {
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(activityID))
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	template, err := s.GetWorkflowDraftTemplate(ctx, cmd.TemplateID, cmd.VersionID, cmd.Version)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	definition, err := workflowTemplateDefinitionForActivity(template.Definition, activity, cmd)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	if validation := s.workflowValidator.ValidateDefinition(definition); !validation.Valid {
		return marketing.WorkflowDraft{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New(workflowValidationMessage(validation)))
	}
	return s.SaveWorkflowDraft(ctx, activity.ID, marketing.SaveWorkflowDraftCommand{
		Operator:   strings.TrimSpace(cmd.Operator),
		Definition: definition,
	})
}

func (s *Service) SaveCurrentWorkflowAsTemplate(ctx context.Context, activityID string, cmd marketing.SaveWorkflowDraftTemplateCommand) (marketing.WorkflowDraftTemplate, error) {
	activity, err := s.repository.GetActivity(ctx, strings.TrimSpace(activityID))
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	draft, err := s.repository.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	if validation := s.workflowValidator.ValidateDefinition(draft.Definition); !validation.Valid {
		return marketing.WorkflowDraftTemplate{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New(workflowValidationMessage(validation)))
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		name = activity.Name + " 模板"
	}
	templateID := strings.TrimSpace(cmd.TemplateID)
	if templateID == "" {
		templateID = workflowTemplateSlug(name)
	}
	now := s.now()
	template := marketing.WorkflowDraftTemplate{
		ID:          templateID,
		VersionID:   marketing.NewID("wftplv"),
		Name:        name,
		Description: strings.TrimSpace(cmd.Description),
		Category:    strings.TrimSpace(cmd.Category),
		Source:      marketing.WorkflowDraftTemplateSaved,
		Status:      marketing.WorkflowDraftTemplateActive,
		Definition:  workflowTemplateDefinitionForSave(draft.Definition, activity, cmd),
		CreatedBy:   strings.TrimSpace(cmd.Operator),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created, err := s.repository.CreateWorkflowDraftTemplate(ctx, template)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	_ = s.audit(ctx, activity.TenantID, actorOrSystem(cmd.Operator), "SAVE_WORKFLOW_TEMPLATE", "WORKFLOW_TEMPLATE", created.VersionID, auditSnapshot(nil, created, map[string]any{
		"activityId": activity.ID,
		"templateId": created.ID,
		"version":    created.Version,
	}))
	return created, nil
}

func (s *Service) ListWorkflowNodePresets() []marketing.WorkflowNodePreset {
	return []marketing.WorkflowNodePreset{
		nodePreset("event-in-standard", "event-in", "标准事件入口", map[string]any{"eventTypes": []string{"DEPOSIT_SUCCESS"}, "sourceSystems": []string{"payment"}}, "entry"),
		nodePreset("inbox-idempotent", "inbox", "幂等收件箱", map[string]any{"idempotencyKeyTemplate": "${tenantId}:${sourceSystem}:${eventType}:${biz.bizType}:${biz.bizId}", "ttlSeconds": 86400}, "store"),
		nodePreset("normalize-basic", "normalize", "基础标准化", map[string]any{"mapping": map[string]any{}, "defaults": map[string]any{}}, "transform"),
		nodePreset("match-active-first", "match", "在线活动优先匹配", map[string]any{"mode": "FIRST_MATCH", "requireActiveActivity": true}, "logic"),
		nodePreset("qualify-risk-block", "qualify", "资格和风控阻断", map[string]any{"ruleCodes": []string{"ELIGIBILITY", "RISK"}, "riskPolicy": "BLOCK"}, "risk"),
		nodePreset("fact-query-user-order", "fact-query", "用户订单事实增强", map[string]any{"factKeys": []string{"USER_PROFILE", "ORDER_SUMMARY"}, "timeoutSeconds": 3}, "query"),
		nodePreset("reward-fixed-usd", "reward", "固定美元奖励", map[string]any{"calculateType": "FIXED", "amount": 10, "currency": "USD"}, "reward"),
		nodePreset("limit-once", "limit", "用户一次限制", map[string]any{"scope": "USER", "maxCount": 1, "period": "ACTIVITY"}, "quota"),
		nodePreset("quota-atomic", "quota", "原子预算扣减", map[string]any{"quotaCode": "DEFAULT_REWARD_BUDGET", "consumeMode": "ATOMIC"}, "quota"),
		nodePreset("callback-signed", "callback", "签名回调", map[string]any{"provider": "default", "requireSignature": true}, "callback"),
		nodePreset("audit-full", "audit", "完整审计", map[string]any{"action": "WORKFLOW_NODE", "includePayload": true}, "audit"),
	}
}

func workflowTemplateDefinitionForActivity(raw json.RawMessage, activity marketing.Activity, cmd marketing.ApplyWorkflowDraftTemplateCommand) (json.RawMessage, error) {
	definition, err := marketing.ParseWorkflowDefinition(raw)
	if err != nil {
		return nil, err
	}
	if definition.Meta == nil {
		definition.Meta = map[string]any{}
	}
	definition.Meta["activityId"] = activity.ID
	definition.Meta["workspaceId"] = firstNonEmptyString(cmd.WorkspaceID, activity.WorkspaceID)
	definition.Meta["folderId"] = firstNonEmptyString(cmd.FolderID, activity.FolderID)
	definition.Meta["templateParameters"] = normalizeWorkflowTemplateParameters(cmd.Parameters)
	applyWorkflowTemplateParameters(&definition, cmd.Parameters)
	return json.Marshal(definition)
}

func workflowTemplateDefinitionForSave(raw json.RawMessage, activity marketing.Activity, cmd marketing.SaveWorkflowDraftTemplateCommand) json.RawMessage {
	definition, err := marketing.ParseWorkflowDefinition(raw)
	if err != nil {
		return raw
	}
	if definition.Meta == nil {
		definition.Meta = map[string]any{}
	}
	definition.Meta["activityId"] = activity.ID
	definition.Meta["workspaceId"] = firstNonEmptyString(cmd.WorkspaceID, activity.WorkspaceID)
	definition.Meta["folderId"] = firstNonEmptyString(cmd.FolderID, activity.FolderID)
	definition.Meta["visibility"] = firstNonEmptyString(cmd.Visibility, "TEAM")
	definition.Meta["templateParameters"] = normalizeWorkflowTemplateParameters(cmd.Parameters)
	out, err := json.Marshal(definition)
	if err != nil {
		return raw
	}
	return out
}

func applyWorkflowTemplateParameters(definition *marketing.WorkflowDefinition, parameters map[string]any) {
	params := normalizeWorkflowTemplateParameters(parameters)
	if len(params) == 0 {
		return
	}
	for index := range definition.Nodes {
		config := map[string]any{}
		if len(definition.Nodes[index].Config) > 0 {
			_ = json.Unmarshal(definition.Nodes[index].Config, &config)
		}
		switch strings.ToLower(strings.TrimSpace(definition.Nodes[index].Type)) {
		case "event-in":
			if value := workflowTemplateParamString(params, "eventType"); value != "" {
				config["eventTypes"] = []string{value}
			}
		case "reward":
			if value := workflowTemplateParamNumber(params, "rewardAmount"); value > 0 {
				config["amount"] = value
			}
			if value := workflowTemplateParamString(params, "currency"); value != "" {
				config["currency"] = value
			}
		case "quota":
			if value := workflowTemplateParamString(params, "quotaCode"); value != "" {
				config["quotaCode"] = value
			}
		case "limit":
			if value := workflowTemplateParamNumber(params, "limitMaxCount"); value > 0 {
				config["maxCount"] = value
			}
		case "callback":
			if value := workflowTemplateParamString(params, "callbackProvider"); value != "" {
				config["provider"] = value
			}
		}
		encoded, _ := json.Marshal(config)
		definition.Nodes[index].Config = encoded
	}
}

func normalizeWorkflowTemplateParameters(parameters map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range parameters {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}
		out[key] = value
	}
	return out
}

func workflowTemplateParamString(parameters map[string]any, key string) string {
	value, ok := parameters[key]
	if !ok || value == nil {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func workflowTemplateParamNumber(parameters map[string]any, key string) float64 {
	switch value := parameters[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		number, _ := value.Float64()
		return number
	case string:
		number, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return number
	default:
		return 0
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nodePreset(id string, nodeType string, name string, config map[string]any, tags ...string) marketing.WorkflowNodePreset {
	raw, _ := json.Marshal(config)
	return marketing.WorkflowNodePreset{ID: id, NodeType: nodeType, Name: name, Config: raw, Tags: tags}
}

func workflowTemplateSlug(value string) string {
	return "tpl-" + workspaceSlug(value)
}

func builtinWorkflowDraftTemplates(now time.Time) []marketing.WorkflowDraftTemplate {
	return []marketing.WorkflowDraftTemplate{
		builtinWorkflowTemplate("complete-activity-chain", "完整活动链路模板", "完整活动链路", "入口、幂等、标准化、匹配、资格、奖励、限制、预算、回调、审计。", []string{"event-in", "inbox", "normalize", "match", "qualify", "reward", "limit", "quota", "callback", "audit"}, now),
		builtinWorkflowTemplate("simple-reward-chain", "简化奖励链路模板", "简化奖励链路", "用于快速搭建最短奖励链路。", []string{"event-in", "inbox", "match", "reward", "quota", "callback", "audit"}, now),
		builtinWorkflowTemplate("risk-qualify-chain", "风控和资格校验链路模板", "风控资格", "强调资格、事实查询、风控限制和审计。", []string{"event-in", "inbox", "fact-query", "qualify", "limit", "audit"}, now),
		builtinWorkflowTemplate("quota-protected-reward-chain", "Quota 保护奖励链路模板", "Quota 保护", "奖励前后加入次数限制和原子预算扣减。", []string{"event-in", "inbox", "match", "qualify", "reward", "limit", "quota", "callback", "audit"}, now),
		builtinWorkflowTemplate("fact-query-enhanced-chain", "Fact Query 数据增强链路模板", "数据增强", "通过 Fact Query 增强上下文后再匹配和发奖。", []string{"event-in", "inbox", "normalize", "fact-query", "match", "qualify", "reward", "quota", "audit"}, now),
	}
}

func builtinWorkflowTemplate(id string, name string, category string, description string, nodeTypes []string, now time.Time) marketing.WorkflowDraftTemplate {
	raw, _ := json.Marshal(workflowTemplateDefinition(nodeTypes))
	return marketing.WorkflowDraftTemplate{
		ID:          id,
		VersionID:   id + "-v1",
		Version:     1,
		Name:        name,
		Description: description,
		Category:    category,
		Source:      marketing.WorkflowDraftTemplateBuiltin,
		Status:      marketing.WorkflowDraftTemplateActive,
		Definition:  raw,
		CreatedBy:   "system",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func workflowTemplateDefinition(nodeTypes []string) map[string]any {
	nodes := make([]map[string]any, 0, len(nodeTypes))
	edges := make([]map[string]any, 0, len(nodeTypes)-1)
	for i, nodeType := range nodeTypes {
		nodes = append(nodes, map[string]any{
			"id":       nodeType,
			"type":     nodeType,
			"kind":     workflowTemplateNodeKind(nodeType),
			"title":    workflowTemplateNodeTitle(nodeType),
			"inputs":   workflowTemplateNodeInputs(nodeType),
			"outputs":  workflowTemplateNodeOutputs(nodeType),
			"config":   workflowTemplateNodeConfig(nodeType),
			"position": map[string]float64{"x": float64(80 + i*300), "y": 360},
		})
		if i > 0 {
			previous := nodeTypes[i-1]
			edges = append(edges, map[string]any{
				"id":     previous + "-" + nodeType,
				"source": previous,
				"target": nodeType,
			})
		}
	}
	return map[string]any{
		"meta":     map[string]any{"activityId": "template", "workspaceId": "template", "folderId": "template"},
		"viewport": map[string]float64{"x": 24, "y": 36, "zoom": 1},
		"nodes":    nodes,
		"edges":    edges,
	}
}

func workflowTemplateNodeKind(nodeType string) string {
	switch nodeType {
	case "event-in":
		return "entry"
	case "inbox", "audit":
		return "store"
	case "reward":
		return "reward"
	case "limit", "quota":
		return "quota"
	case "fact-query":
		return "query"
	default:
		return "logic"
	}
}

func workflowTemplateNodeTitle(nodeType string) string {
	titles := map[string]string{"event-in": "事件入口", "inbox": "幂等收件箱", "normalize": "事件标准化", "match": "活动匹配", "qualify": "资格校验", "reward": "奖励计算", "limit": "次数限制", "quota": "预算库存扣减", "callback": "结果回调", "audit": "审计记录", "fact-query": "事实查询"}
	return titles[nodeType]
}

func workflowTemplateNodeInputs(nodeType string) []string {
	inputs := map[string][]string{"event-in": {"raw_event"}, "inbox": {"raw_event"}, "normalize": {"raw_event"}, "match": {"event_context"}, "qualify": {"matched_activity"}, "reward": {"qualified_context"}, "limit": {"subject"}, "quota": {"reward_items"}, "callback": {"quota_hold"}, "audit": {"stage_result"}, "fact-query": {"subject"}}
	return inputs[nodeType]
}

func workflowTemplateNodeOutputs(nodeType string) []string {
	outputs := map[string][]string{"event-in": {"raw_event"}, "inbox": {"event_context"}, "normalize": {"normalized_event"}, "match": {"matched_activity"}, "qualify": {"stage_result"}, "reward": {"reward_items"}, "limit": {"limit_hold"}, "quota": {"quota_hold"}, "callback": {"callback_result"}, "audit": {"audit_log"}, "fact-query": {"facts"}}
	return outputs[nodeType]
}

func workflowTemplateNodeConfig(nodeType string) map[string]any {
	configs := map[string]map[string]any{
		"event-in": {
			"eventTypes":             []string{"DEPOSIT_SUCCESS"},
			"sourceSystems":          []string{"payment"},
			"tenantScope":            "CURRENT_TENANT",
			"payloadSchemaVersion":   "standard-v1",
			"rejectUnknownEventType": true,
			"traceKeyPath":           "$.traceId",
		},
		"inbox": {
			"idempotencyKeyTemplate": "${tenantId}:${eventId}",
			"duplicatePolicy":        "RETURN_EXISTING",
			"ttlSeconds":             86400,
			"payloadConflictPolicy":  "BLOCK_ON_PAYLOAD_MISMATCH",
			"requireTraceKey":        false,
		},
		"match": {
			"mode":                  "FIRST_MATCH",
			"requireActiveActivity": true,
			"priorityOrder":         "PRIORITY_DESC",
			"tenantMatchPolicy":     "SAME_TENANT",
			"fallbackAction":        "NO_MATCH_FAIL",
			"activityCodePattern":   "",
		},
		"qualify": {
			"ruleCodes":        []string{"ELIGIBILITY"},
			"evaluationMode":   "ALL_PASS",
			"stopOnFirstBlock": true,
			"riskPolicy":       "BLOCK",
			"blockAction":      "BLOCK",
			"reasonPath":       "qualification.reason",
		},
		"reward": {
			"rewardCode":       "STUDIO_DEFAULT_REWARD",
			"rewardType":       "BONUS",
			"calculateType":    "FIXED",
			"amount":           10,
			"currency":         "USD",
			"amountSourcePath": "$.qualified_context.order.amount",
			"grantMode":        "AUTO",
			"grantChannel":     "HTTP",
		},
		"limit": {
			"scope":            "USER",
			"scopeKeyTemplate": "${subject.id}:${activityId}",
			"period":           "ACTIVITY",
			"maxCount":         1,
			"consumeMode":      "ATOMIC",
			"exceedAction":     "BLOCK",
		},
		"quota": {
			"quotaCode":        "DEFAULT_REWARD_BUDGET",
			"quotaType":        "BUDGET",
			"scope":            "ACTIVITY",
			"scopeKeyTemplate": "${activityId}",
			"consumeMode":      "ATOMIC",
			"amountField":      "reward_items[0].amount",
			"failAction":       "BLOCK",
		},
		"callback": {
			"provider":         "default",
			"callbackIdPath":   "$.callbackId",
			"statusPath":       "$.status",
			"requireSignature": true,
			"maxSkewSeconds":   300,
			"duplicatePolicy":  "RETURN_EXISTING",
		},
		"audit": {
			"action":          "WORKFLOW_TEMPLATE",
			"severity":        "INFO",
			"targetType":      "WORKFLOW_NODE",
			"includePayload":  true,
			"redactPaths":     []string{"$.token", "$.password"},
			"retentionPolicy": "STANDARD",
		},
		"fact-query": {
			"factKeys":        []string{"USER_PROFILE"},
			"source":          "FACT_SERVICE",
			"subjectPath":     "subject.id",
			"timeoutSeconds":  3,
			"cacheTtlSeconds": 60,
			"failureAction":   "BLOCK",
		},
		"normalize": {"mapping": map[string]any{}, "defaults": map[string]any{}},
	}
	return configs[nodeType]
}
