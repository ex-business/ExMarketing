package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
		WorkspaceID: strings.TrimSpace(firstNonEmptyString(cmd.WorkspaceID, activity.WorkspaceID)),
		FolderID:    strings.TrimSpace(firstNonEmptyString(cmd.FolderID, activity.FolderID)),
		Visibility:  marketing.WorkspaceVisibility(strings.TrimSpace(firstNonEmptyString(cmd.Visibility, string(activity.Visibility), string(marketing.WorkspaceTeam)))),
		Tags:        cleanWorkflowTemplateTags(cmd.Tags),
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

type workflowDraftTemplateUpdater interface {
	UpdateWorkflowDraftTemplate(ctx context.Context, template marketing.WorkflowDraftTemplate) (marketing.WorkflowDraftTemplate, error)
}

func (s *Service) UpdateWorkflowDraftTemplate(ctx context.Context, templateID string, cmd marketing.UpdateWorkflowDraftTemplateCommand) (marketing.WorkflowDraftTemplate, error) {
	updater, ok := s.repository.(workflowDraftTemplateUpdater)
	if !ok {
		return marketing.WorkflowDraftTemplate{}, errors.New("workflow draft template update is not supported by repository")
	}
	template, err := s.GetWorkflowDraftTemplate(ctx, templateID, cmd.VersionID, cmd.Version)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	if template.Source == marketing.WorkflowDraftTemplateBuiltin {
		return marketing.WorkflowDraftTemplate{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New("builtin template cannot be updated"))
	}
	before := template
	if strings.TrimSpace(cmd.Name) != "" {
		template.Name = strings.TrimSpace(cmd.Name)
	}
	if cmd.Description != nil {
		template.Description = strings.TrimSpace(*cmd.Description)
	}
	if cmd.Category != nil {
		template.Category = strings.TrimSpace(*cmd.Category)
	}
	if cmd.WorkspaceID != nil {
		template.WorkspaceID = strings.TrimSpace(*cmd.WorkspaceID)
	}
	if cmd.FolderID != nil {
		template.FolderID = strings.TrimSpace(*cmd.FolderID)
	}
	if cmd.Visibility != nil {
		template.Visibility = marketing.WorkspaceVisibility(strings.TrimSpace(*cmd.Visibility))
	}
	if cmd.Tags != nil {
		template.Tags = cleanWorkflowTemplateTags(cmd.Tags)
	}
	if strings.TrimSpace(cmd.Status) != "" {
		template.Status = marketing.WorkflowDraftTemplateStatus(strings.TrimSpace(cmd.Status))
	}
	template.UpdatedAt = s.now()
	updated, err := updater.UpdateWorkflowDraftTemplate(ctx, template)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	_ = s.audit(ctx, "", actorOrSystem(cmd.Operator), "UPDATE_WORKFLOW_TEMPLATE", "WORKFLOW_TEMPLATE", updated.VersionID, auditSnapshot(before, updated, map[string]any{
		"templateId": updated.ID,
		"version":    updated.Version,
	}))
	return updated, nil
}

func (s *Service) CopyWorkflowDraftTemplate(ctx context.Context, templateID string, cmd marketing.CopyWorkflowDraftTemplateCommand) (marketing.WorkflowDraftTemplate, error) {
	source, err := s.GetWorkflowDraftTemplate(ctx, templateID, cmd.VersionID, cmd.Version)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		name = source.Name + " Copy"
	}
	now := s.now()
	copied := marketing.WorkflowDraftTemplate{
		ID:          workflowTemplateSlug(name + "-" + marketing.NewID("copy")),
		VersionID:   marketing.NewID("wftplv"),
		Version:     1,
		Name:        name,
		Description: source.Description,
		Category:    strings.TrimSpace(firstNonEmptyString(cmd.Category, source.Category)),
		WorkspaceID: strings.TrimSpace(firstNonEmptyString(cmd.WorkspaceID, source.WorkspaceID)),
		FolderID:    strings.TrimSpace(firstNonEmptyString(cmd.FolderID, source.FolderID)),
		Visibility:  marketing.WorkspaceVisibility(strings.TrimSpace(firstNonEmptyString(cmd.Visibility, string(source.Visibility), string(marketing.WorkspaceTeam)))),
		Tags:        cleanWorkflowTemplateTags(append(source.Tags, cmd.Tags...)),
		Source:      marketing.WorkflowDraftTemplateSaved,
		Status:      marketing.WorkflowDraftTemplateActive,
		Definition:  workflowTemplateDefinitionForCopy(source.Definition, cmd),
		CreatedBy:   strings.TrimSpace(cmd.Operator),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created, err := s.repository.CreateWorkflowDraftTemplate(ctx, copied)
	if err != nil {
		return marketing.WorkflowDraftTemplate{}, err
	}
	_ = s.audit(ctx, "", actorOrSystem(cmd.Operator), "COPY_WORKFLOW_TEMPLATE", "WORKFLOW_TEMPLATE", created.VersionID, auditSnapshot(source, created, map[string]any{
		"sourceTemplateId": source.ID,
		"sourceVersion":    source.Version,
	}))
	return created, nil
}

func (s *Service) CompareWorkflowDraftTemplateVersions(ctx context.Context, templateID string, leftVersionID string, rightVersionID string) (marketing.WorkflowDraftTemplateCompareResult, error) {
	versions, err := s.ListWorkflowDraftTemplateVersions(ctx, templateID)
	if err != nil {
		return marketing.WorkflowDraftTemplateCompareResult{}, err
	}
	if len(versions) == 0 {
		return marketing.WorkflowDraftTemplateCompareResult{}, marketing.ErrNotFound
	}
	left := versions[len(versions)-1]
	right := versions[0]
	for _, item := range versions {
		if leftVersionID != "" && item.VersionID == leftVersionID {
			left = item
		}
		if rightVersionID != "" && item.VersionID == rightVersionID {
			right = item
		}
	}
	leftDef, err := marketing.ParseWorkflowDefinition(left.Definition)
	if err != nil {
		return marketing.WorkflowDraftTemplateCompareResult{}, err
	}
	rightDef, err := marketing.ParseWorkflowDefinition(right.Definition)
	if err != nil {
		return marketing.WorkflowDraftTemplateCompareResult{}, err
	}
	leftNodes := workflowTemplateNodeIDSet(leftDef.Nodes)
	rightNodes := workflowTemplateNodeIDSet(rightDef.Nodes)
	leftNodeByID := workflowTemplateNodeByID(leftDef.Nodes)
	rightNodeByID := workflowTemplateNodeByID(rightDef.Nodes)
	var added []string
	var removed []string
	common := 0
	for id := range rightNodes {
		if _, ok := leftNodes[id]; ok {
			common++
		} else {
			added = append(added, id)
		}
	}
	for id := range leftNodes {
		if _, ok := rightNodes[id]; !ok {
			removed = append(removed, id)
		}
	}
	nodeChanges := workflowTemplateNodeDiffs(leftNodeByID, rightNodeByID)
	edgeChanges := workflowTemplateEdgeDiffs(workflowTemplateEdgeByID(leftDef.Edges), workflowTemplateEdgeByID(rightDef.Edges))
	return marketing.WorkflowDraftTemplateCompareResult{
		LeftVersionID:  left.VersionID,
		RightVersionID: right.VersionID,
		LeftVersion:    left.Version,
		RightVersion:   right.Version,
		NodeDelta:      len(rightDef.Nodes) - len(leftDef.Nodes),
		EdgeDelta:      len(rightDef.Edges) - len(leftDef.Edges),
		AddedNodes:     added,
		RemovedNodes:   removed,
		CommonNodes:    common,
		NodeChanges:    nodeChanges,
		EdgeChanges:    edgeChanges,
		ChangedMeta: map[string]any{
			"name":       []string{left.Name, right.Name},
			"category":   []string{left.Category, right.Category},
			"visibility": []string{string(left.Visibility), string(right.Visibility)},
			"status":     []string{string(left.Status), string(right.Status)},
		},
	}, nil
}

func (s *Service) ListWorkflowNodePresets() []marketing.WorkflowNodePreset {
	return []marketing.WorkflowNodePreset{
		nodePreset("event-in-standard", "event-in", "标准事件入口", map[string]any{"eventTypes": []string{"DEPOSIT_SUCCESS"}, "sourceSystems": []string{"payment"}}, "entry"),
		nodePreset("event-in-checkin-success", "event-in", "签到成功标准事件入口", map[string]any{
			"eventTypes":             []string{"CHECKIN_SUCCESS"},
			"eventTypeMatchMode":     "ANY",
			"sourceSystems":          []string{"app", "user-center"},
			"tenantScope":            "CURRENT_TENANT",
			"payloadSchemaVersion":   "standard-v1",
			"rejectUnknownEventType": true,
			"traceKeyPath":           "$.traceId",
		}, "entry", "checkin", "reward"),
		nodePreset("event-in-checkin-or-deposit", "event-in", "签到或存款标准事件入口", map[string]any{
			"eventTypes":             []string{"CHECKIN_SUCCESS", "DEPOSIT_SUCCESS"},
			"eventTypeMatchMode":     "ANY",
			"sourceSystems":          []string{"app", "payment"},
			"tenantScope":            "CURRENT_TENANT",
			"payloadSchemaVersion":   "standard-v1",
			"rejectUnknownEventType": true,
			"traceKeyPath":           "$.traceId",
		}, "entry", "checkin", "deposit", "any"),
		nodePreset("event-in-checkin-and-deposit", "event-in", "签到且存款标准事件入口", map[string]any{
			"eventTypes":               []string{"CHECKIN_SUCCESS", "DEPOSIT_SUCCESS"},
			"eventTypeMatchMode":       "ALL",
			"sourceSystems":            []string{"app", "payment"},
			"tenantScope":              "CURRENT_TENANT",
			"payloadSchemaVersion":     "standard-v1",
			"rejectUnknownEventType":   true,
			"traceKeyPath":             "$.traceId",
			"correlationKeyTemplate":   "${tenantId}:${subject.id}:${activityId}",
			"correlationWindowSeconds": 86400,
			"allEventsAction":          "WAIT",
		}, "entry", "checkin", "deposit", "all", "planned"),
		nodePreset("http-event-openapi", "http-event", "HTTP 接口事件", map[string]any{"path": "/openapi/v1/events", "method": "POST", "auth": "BEARER_TOKEN", "payloadMode": "STANDARD_EVENT", "maxBodyBytes": 1048576}, "event-input", "http", "explain"),
		nodePreset("webhook-event-hmac", "webhook-event", "Webhook 映射事件", map[string]any{"receiverCode": "payment-webhook", "signature": "HMAC_SHA256", "allowedTenantIds": []string{"site_001"}, "allowedSourceSystems": []string{"payment"}, "mappingVersion": "v1"}, "event-input", "webhook", "explain"),
		nodePreset("mq-event-kafka", "mq-event", "MQ Topic 事件", map[string]any{"broker": "KAFKA", "topic": "user-checkin-events", "consumerGroup": "exmarketing-checkin", "allowedSourceSystems": []string{"app"}, "ackPolicy": "ACK_AFTER_INBOX_COMMIT", "retryPolicy": "RETRY_THEN_DLQ", "maxBodyBytes": 1048576}, "event-input", "mq", "adapter"),
		nodePreset("schedule-event-cron", "schedule-event", "定时事件", map[string]any{"scheduleType": "CRON", "cron": "0 0 * * *", "timezone": "UTC", "eventType": "SCHEDULE_TICK", "sourceSystem": "exmarketing-scheduler", "bizType": "SCHEDULE", "bizIdTemplate": "${activityId}:${nodeId}:${eventId}", "eventIdTemplate": "${activityId}:${nodeId}:${biz.bizId}", "misfirePolicy": "FIRE_ONCE", "jitterSeconds": 0}, "event-input", "schedule", "adapter"),
		nodePreset("state-event-watch", "state-event", "状态监听事件", map[string]any{"source": "RESOURCE", "resourceRef": "workflow_memory", "namespace": "state-watch", "keyTemplate": "activity:state:${activityId}", "condition": "value.enabled == true", "intervalSeconds": 10, "debounceSeconds": 60}, "event-input", "state", "adapter"),
		nodePreset("manual-event-ops", "manual-event", "人工触发事件", map[string]any{"allowedRoles": []string{"OPS", "ADMIN"}, "requireReason": true, "requireApproval": false, "payloadMode": "STANDARD_EVENT", "auditAction": "MANUAL_EVENT_TRIGGERED"}, "event-input", "manual", "planned"),
		nodePreset("inbox-idempotent", "inbox", "幂等收件箱", map[string]any{"idempotencyKeyTemplate": "${tenantId}:${sourceSystem}:${eventType}:${biz.bizType}:${biz.bizId}", "ttlSeconds": 86400}, "store"),
		nodePreset("normalize-basic", "normalize", "基础标准化", map[string]any{"mapping": map[string]any{}, "defaults": map[string]any{}}, "transform"),
		nodePreset("match-active-first", "match", "在线活动优先匹配", map[string]any{"mode": "FIRST_MATCH", "requireActiveActivity": true}, "logic"),
		nodePreset("qualify-risk-block", "qualify", "资格和风控阻断", map[string]any{"ruleCodes": []string{"ELIGIBILITY", "RISK"}, "riskPolicy": "BLOCK"}, "risk"),
		nodePreset("fact-query-user-order", "fact-query", "组合事实查询", map[string]any{
			"factKeys":      []string{"USER_PROFILE", "ORDER_SUMMARY"},
			"providerChain": []string{"WORKFLOW_CONTEXT", "LOCAL_CACHE", "HTTP_PROVIDER"},
			"subjectPath":   "subject.id",
			"endpoint":      "https://user-service/internal/facts/query",
			"method":        "POST",
			"authRef":       "internal-service-token",
			"mapping": map[string]any{
				"value": map[string]any{"source": "$.data", "type": "object", "required": false},
			},
			"timeoutSeconds": 3,
		}, "query", "fact"),
		nodePreset("context-query-user-profile", "context-query", "上下文用户资料", map[string]any{"factKeys": []string{"USER_PROFILE"}, "sourcePath": "$.attributes.USER_PROFILE", "failureAction": "USE_EMPTY"}, "query", "fact", "context"),
		nodePreset("local-cache-query-user-profile", "local-cache-query", "本地缓存用户资料", map[string]any{"factKeys": []string{"USER_PROFILE"}, "cachePath": "$.vars.execution.facts.USER_PROFILE", "failureAction": "USE_EMPTY"}, "query", "fact", "cache"),
		nodePreset("http-query-user-profile", "http-query", "HTTP 用户资料查询", map[string]any{
			"factKeys":    []string{"USER_PROFILE"},
			"source":      "HTTP_PROVIDER",
			"subjectPath": "subject.id",
			"endpoint":    "https://user-service/internal/facts/user-profile",
			"method":      "POST",
			"authRef":     "internal-service-token",
			"mapping": map[string]any{
				"vipLevel":     map[string]any{"source": "$.data.profile.vipLevel", "type": "string", "required": false},
				"country":      map[string]any{"source": "$.data.profile.country", "type": "string", "required": false},
				"registeredAt": map[string]any{"source": "$.data.profile.registeredAt", "type": "string", "required": false},
			},
			"timeoutSeconds": 3,
			"failureAction":  "BLOCK",
		}, "query", "fact", "http"),
		nodePreset("redis-query-user-profile", "redis-query", "Redis 用户资料缓存", map[string]any{
			"factKeys":      []string{"USER_PROFILE"},
			"redisRef":      "marketing-fact-cache",
			"keyTemplate":   "marketing:fact:${tenantId}:USER_PROFILE:${subject.id}",
			"command":       "GET",
			"valuePath":     "$",
			"failureAction": "SKIP",
		}, "query", "fact", "redis", "planned"),
		nodePreset("grpc-query-user-profile", "grpc-query", "gRPC 用户资料查询", map[string]any{
			"factKeys":       []string{"USER_PROFILE"},
			"grpcTarget":     "dns:///user-service:9090",
			"service":        "user.ProfileFactService",
			"method":         "QueryFacts",
			"authority":      "user-service",
			"tls":            true,
			"timeoutSeconds": 3,
			"failureAction":  "SKIP",
		}, "query", "fact", "grpc", "planned"),
		nodePreset("plugin-query-user-profile", "plugin-query", "插件用户资料查询", map[string]any{
			"factKeys":       []string{"USER_PROFILE"},
			"pluginCode":     "user-profile",
			"action":         "QUERY_FACTS",
			"inputPath":      "$",
			"timeoutSeconds": 3,
			"failureAction":  "SKIP",
		}, "query", "fact", "plugin", "planned"),
		nodePreset("custom-query-user-profile", "custom-query", "自定义事实查询", map[string]any{
			"factKeys":     []string{"USER_PROFILE"},
			"providerCode": "custom-user-profile",
			"protocol":     "CUSTOM",
			"endpoint":     "custom://user-profile/facts",
			"requestTemplate": map[string]any{
				"subjectId": "${subject.id}",
				"tenantId":  "${tenantId}",
			},
			"responseMapping": map[string]any{
				"USER_PROFILE": map[string]any{"source": "$.profile", "type": "object", "required": false},
			},
			"timeoutSeconds": 3,
			"failureAction":  "SKIP",
		}, "query", "fact", "custom", "planned"),
		nodePreset("lua-query-user-profile", "lua-query", "Lua 上下文事实", map[string]any{"factCode": "USER_PROFILE", "source": "emit({facts = {USER_PROFILE = {vipLevel = input('vipLevel')}}})", "failureAction": "BLOCK"}, "query", "fact", "lua"),
		nodePreset("fact-query-checkin-streak", "fact-query", "签到连续天数事实", map[string]any{"factKeys": []string{"CHECKIN_STREAK"}, "source": "FACT_SERVICE", "subjectPath": "subject.id", "timeoutSeconds": 3, "failureAction": "USE_EMPTY"}, "query", "checkin"),
		nodePreset("activity-state-checkin", "activity-state", "签到活动状态", map[string]any{"storeType": "INTERNAL", "operation": "READ", "namespace": "checkin", "stateKeyTemplate": "${activityId}:${subject.id}", "missingPolicy": "EMPTY_STATE", "requireVersion": true}, "store", "checkin"),
		nodePreset("state-store-workflow-object", "state-store", "工作流对象存储", map[string]any{"resourceRef": "workflow_memory", "operation": "DEFINE", "namespace": "workflow", "keyTemplate": "${activityId}:${eventId}:${nodeId}", "valueType": "OBJECT", "defaultValue": map[string]any{}, "missingPolicy": "EMPTY"}, "store", "memory"),
		nodePreset("state-store-database-table", "state-store", "数据库表格存储", map[string]any{"resourceRef": "default_workflow_state", "operation": "UPSERT", "namespace": "workflow", "keyTemplate": "${activityId}:${subject.id}:table", "valueType": "TABLE", "defaultValue": []any{}, "missingPolicy": "EMPTY"}, "store", "database"),
		nodePreset("state-store-http-provider", "state-store", "外部 Provider 存储", map[string]any{"resourceRef": "external_storage_provider", "operation": "UPSERT", "namespace": "workflow", "keyTemplate": "${activityId}:${subject.id}:external", "valueType": "OBJECT", "valuePath": "$.input.value"}, "store", "external", "provider"),
		nodePreset("quota-check-daily-checkin", "quota-check", "每日签到频控", map[string]any{"scope": "USER", "scopeKeyTemplate": "${subject.id}:${activityId}", "window": "DAY", "maxCount": 1, "consumeMode": "CHECK_ONLY", "exceedAction": "BLOCK"}, "quota", "checkin"),
		nodePreset("combine-lua-context", "combine", "Lua 组合上下文", map[string]any{"language": "LUA", "outputPath": "combined_result", "source": "emit({combined = {eventId = ctx.event.eventId, subject = ctx.event.subject, input = ctx.node.input}})"}, "transform", "lua"),
		nodePreset("split-lua-items", "split", "Lua 拆分数组", map[string]any{"language": "LUA", "outputPath": "split_items", "source": "emit({items = ctx.node.input.items})"}, "transform", "lua"),
		nodePreset("candidate-reward-checkin", "candidate-reward", "签到候选奖励", map[string]any{"source": "ACTIVITY_REWARD_CONFIG", "notQualifiedAction": "SKIP", "emptyAction": "BLOCK", "includeTrace": true}, "reward", "checkin"),
		nodePreset("reward-fixed-usd", "reward", "固定美元奖励", map[string]any{"calculateType": "FIXED", "amount": 10, "currency": "USD"}, "reward"),
		nodePreset("reward-daily-checkin-fixed", "reward", "每日签到固定奖励", map[string]any{"rewardCode": "DAILY_CHECKIN_BONUS", "rewardType": "BONUS", "calculateType": "FIXED", "amount": 1, "currency": "USD", "grantMode": "AUTO", "grantChannel": "HTTP"}, "reward", "checkin"),
		nodePreset("limit-once", "limit", "用户一次限制", map[string]any{"scope": "USER", "maxCount": 1, "period": "ACTIVITY"}, "quota"),
		nodePreset("limit-daily-checkin-once", "limit", "每日签到一次限制", map[string]any{"scope": "USER", "scopeKeyTemplate": "${subject.id}:${activityId}", "period": "DAY", "maxCount": 1, "consumeMode": "ATOMIC", "exceedAction": "BLOCK"}, "quota", "checkin"),
		nodePreset("quota-atomic", "quota", "原子预算扣减", map[string]any{"quotaCode": "DEFAULT_REWARD_BUDGET", "consumeMode": "ATOMIC"}, "quota"),
		nodePreset("quota-daily-checkin-budget", "quota", "签到奖励预算扣减", map[string]any{"quotaCode": "DAILY_CHECKIN_REWARD", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKeyTemplate": "${activityId}", "consumeMode": "ATOMIC", "amountField": "reward_items[0].amount", "failAction": "BLOCK"}, "quota", "checkin"),
		nodePreset("budget-reserve-checkin", "budget-reserve", "签到预算库存锁定", map[string]any{"quotaCode": "DAILY_CHECKIN_REWARD", "quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKeyTemplate": "${activityId}", "reserveMode": "ATOMIC", "amountField": "candidate_rewards[0].amount", "failAction": "BLOCK", "releaseOnFailure": true}, "quota", "checkin"),
		nodePreset("approval-gate-threshold", "approval-gate", "金额阈值审批", map[string]any{"mode": "AUTO_BELOW_THRESHOLD", "amountThreshold": 1000, "pendingAction": "PAUSE"}, "approval"),
		nodePreset("random-pick-audit", "random-pick", "可审计随机抽取", map[string]any{"mode": "WEIGHTED", "itemsPath": "$.candidate_rewards", "seedTemplate": "${tenantId}:${activityId}:${eventId}", "pickCount": 1, "emptyAction": "SKIP"}, "random"),
		nodePreset("plugin-execute-checkin", "plugin-execute", "签到插件执行", map[string]any{"pluginCode": "checkin", "action": "SIGN_IN", "inputPath": "$", "failureAction": "BLOCK"}, "plugin", "checkin"),
		nodePreset("ab-bucket-50-50", "ab-bucket", "AB 50/50 分桶", map[string]any{"experimentKey": "default-experiment", "subjectKeyTemplate": "${subject.id}", "buckets": []string{"A:50", "B:50"}}, "experiment"),
		nodePreset("risk-score-facts", "risk-score", "事实风险评分", map[string]any{"source": "FACTS", "scorePath": "$.facts.riskScore", "blockThreshold": 90, "reviewThreshold": 70, "failureAction": "REVIEW"}, "risk"),
		nodePreset("settlement-window-daily", "settlement-window", "每日结算窗口", map[string]any{"windowType": "DAILY", "start": "00:00", "end": "23:59", "timezone": "UTC", "outsideAction": "SKIP"}, "settlement"),
		nodePreset("manual-adjust-audited", "manual-adjust", "人工审计调整", map[string]any{"adjustType": "REWARD", "targetPath": "$.grant_order", "requireReason": true, "requireApproval": true}, "ops"),
		nodePreset("compensation-callback-retry", "compensation", "回调失败补偿", map[string]any{"triggerOn": "FAILED", "sourcePath": "$.callback_result", "action": "RETRY_CALLBACK", "maxAttempts": 3}, "ops"),
		nodePreset("reward-reconcile-basic", "reward-reconcile", "奖励链路对账", map[string]any{"targets": []string{"grant", "quota", "callback", "outbox"}, "autoRepair": false}, "ops", "reconcile"),
		nodePreset("participation-snapshot", "participation", "参与快照", map[string]any{"status": "CREATED", "deduplicate": true}, "store"),
		nodePreset("grant-order-http", "grant-order", "HTTP 发奖单", map[string]any{"provider": "HTTP", "expireSeconds": 3600}, "grant"),
		nodePreset("outbox-http", "outbox", "HTTP 发件箱", map[string]any{"channel": "HTTP", "maxAttempts": 3}, "dispatch"),
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
	if len(cmd.Tags) > 0 {
		definition.Meta["tags"] = cleanWorkflowTemplateTags(cmd.Tags)
	}
	definition.Meta["templateParameters"] = normalizeWorkflowTemplateParameters(cmd.Parameters)
	out, err := json.Marshal(definition)
	if err != nil {
		return raw
	}
	return out
}

func workflowTemplateDefinitionForCopy(raw json.RawMessage, cmd marketing.CopyWorkflowDraftTemplateCommand) json.RawMessage {
	definition, err := marketing.ParseWorkflowDefinition(raw)
	if err != nil {
		return raw
	}
	if definition.Meta == nil {
		definition.Meta = map[string]any{}
	}
	definition.Meta["activityId"] = "template"
	definition.Meta["workspaceId"] = strings.TrimSpace(cmd.WorkspaceID)
	definition.Meta["folderId"] = strings.TrimSpace(cmd.FolderID)
	definition.Meta["visibility"] = firstNonEmptyString(cmd.Visibility, "TEAM")
	out, err := json.Marshal(definition)
	if err != nil {
		return raw
	}
	return out
}

func cleanWorkflowTemplateTags(tags []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func workflowTemplateNodeIDSet(nodes []marketing.WorkflowNode) map[string]struct{} {
	out := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func workflowTemplateNodeByID(nodes []marketing.WorkflowNode) map[string]marketing.WorkflowNode {
	out := make(map[string]marketing.WorkflowNode, len(nodes))
	for _, node := range nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			out[id] = node
		}
	}
	return out
}

func workflowTemplateEdgeByID(edges []marketing.WorkflowEdge) map[string]marketing.WorkflowEdge {
	out := make(map[string]marketing.WorkflowEdge, len(edges))
	for _, edge := range edges {
		id := strings.TrimSpace(edge.ID)
		if id != "" {
			out[id] = edge
		}
	}
	return out
}

func workflowTemplateNodeDiffs(left map[string]marketing.WorkflowNode, right map[string]marketing.WorkflowNode) []marketing.WorkflowTemplateNodeDiff {
	ids := make([]string, 0, len(left))
	for id := range left {
		if _, ok := right[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := []marketing.WorkflowTemplateNodeDiff{}
	for _, id := range ids {
		changes := workflowJSONValueDiff(workflowTemplateComparableNode(right[id]), workflowTemplateComparableNode(left[id]), true, true, "$")
		if len(changes) == 0 {
			continue
		}
		node := right[id]
		label := strings.TrimSpace(node.Title)
		if label == "" {
			label = id
		}
		out = append(out, marketing.WorkflowTemplateNodeDiff{
			NodeID:  id,
			Label:   label,
			Type:    node.Type,
			Kind:    node.Kind,
			Changes: changes,
		})
	}
	return out
}

func workflowTemplateComparableNode(node marketing.WorkflowNode) map[string]any {
	return map[string]any{
		"type":    node.Type,
		"kind":    node.Kind,
		"code":    node.Code,
		"title":   node.Title,
		"inputs":  node.Inputs,
		"outputs": node.Outputs,
		"config":  workflowTemplateConfigValue(node.Config),
	}
}

func workflowTemplateConfigValue(raw json.RawMessage) any {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func workflowTemplateEdgeDiffs(left map[string]marketing.WorkflowEdge, right map[string]marketing.WorkflowEdge) []marketing.WorkflowTemplateEdgeDiff {
	ids := make([]string, 0, len(left))
	for id := range left {
		if _, ok := right[id]; ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out := []marketing.WorkflowTemplateEdgeDiff{}
	for _, id := range ids {
		changes := workflowJSONValueDiff(workflowTemplateComparableEdge(right[id]), workflowTemplateComparableEdge(left[id]), true, true, "$")
		if len(changes) == 0 {
			continue
		}
		edge := right[id]
		out = append(out, marketing.WorkflowTemplateEdgeDiff{
			EdgeID:  id,
			Source:  edge.Source,
			Target:  edge.Target,
			Label:   edge.Label,
			Changes: changes,
		})
	}
	return out
}

func workflowTemplateComparableEdge(edge marketing.WorkflowEdge) map[string]any {
	return map[string]any{
		"source":       edge.Source,
		"target":       edge.Target,
		"sourceHandle": edge.SourceHandle,
		"targetHandle": edge.TargetHandle,
		"label":        edge.Label,
		"route":        workflowTemplateJSONValue(edge.Route),
	}
}

func workflowTemplateJSONValue(value any) any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return value
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
		builtinWorkflowTemplate("complete-activity-chain", "完整活动链路模板", "完整活动链路", "入口、幂等、标准化、匹配、事实查询、资格、奖励、限制、预算、参与、发奖单、发件箱、审计。", []string{"event-in", "inbox", "normalize", "match", "fact-query", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}, now),
		builtinWorkflowTemplate("simple-reward-chain", "简化奖励链路模板", "简化奖励链路", "用于快速搭建最短奖励链路。", []string{"event-in", "inbox", "normalize", "match", "fact-query", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}, now),
		builtinWorkflowTemplate("risk-qualify-chain", "风控和资格校验链路模板", "风控资格", "强调资格、事实查询、风控限制和审计。", []string{"event-in", "inbox", "fact-query", "qualify", "limit", "audit"}, now),
		builtinWorkflowTemplate("quota-protected-reward-chain", "Quota 保护奖励链路模板", "Quota 保护", "奖励前后加入次数限制和原子预算扣减。", []string{"event-in", "inbox", "normalize", "match", "fact-query", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}, now),
		builtinWorkflowTemplate("fact-query-enhanced-chain", "Fact Query 数据增强链路模板", "数据增强", "通过 Fact Query 增强上下文后再匹配和发奖。", []string{"event-in", "inbox", "normalize", "fact-query", "match", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"}, now),
		builtinWorkflowTemplate("production-checkin-reward-chain", "生产级签到奖励链路模板", "签到奖励", "签到入口、状态读取、每日频控、候选奖励、预算锁定、发奖、对账和审计。", []string{"event-in", "inbox", "normalize", "fact-query", "activity-state", "quota-check", "qualify", "candidate-reward", "budget-reserve", "participation", "grant-order", "outbox", "reward-reconcile", "audit"}, now),
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
	auditTarget := workflowTemplateFirstNodeID(nodeTypes, "audit")
	for i, nodeType := range nodeTypes {
		config := workflowTemplateNodeConfig(nodeType)
		if mapping := workflowTemplateInputMapping(nodeType, nodeTypes[:i]); len(mapping) > 0 {
			if config == nil {
				config = map[string]any{}
			}
			config["inputMapping"] = mapping
		}
		nodes = append(nodes, map[string]any{
			"id":       nodeType,
			"type":     nodeType,
			"kind":     workflowTemplateNodeKind(nodeType),
			"title":    workflowTemplateNodeTitle(nodeType),
			"inputs":   workflowTemplateNodeInputs(nodeType),
			"outputs":  workflowTemplateNodeOutputs(nodeType),
			"config":   config,
			"position": map[string]float64{"x": float64(80 + i*300), "y": 360},
		})
		if i > 0 {
			previous := nodeTypes[i-1]
			if previous == "qualify" {
				edges = append(edges, workflowTemplateRouteEdge(previous, nodeType, "PASS", false))
				if auditTarget != "" {
					edges = append(edges,
						workflowTemplateRouteEdge(previous, auditTarget, "BLOCK", false),
						workflowTemplateRouteEdge(previous, auditTarget, "RISK", true),
					)
				}
				continue
			}
			if nodeType == "audit" && previous != "qualify" {
				edges = append(edges, map[string]any{
					"id":     previous + "-" + nodeType,
					"source": previous,
					"target": nodeType,
				})
				continue
			}
			edges = append(edges, map[string]any{
				"id":     previous + "-" + nodeType,
				"source": previous,
				"target": nodeType,
			})
		}
	}
	meta := map[string]any{"activityId": "template", "workspaceId": "template", "folderId": "template"}
	if workflowTemplateCoreContractEnabled(nodeTypes) {
		meta["contractVersion"] = "core-node-contract.v1"
	}
	return map[string]any{
		"meta":     meta,
		"viewport": map[string]float64{"x": 24, "y": 36, "zoom": 1},
		"nodes":    nodes,
		"edges":    edges,
	}
}

func workflowTemplateCoreContractEnabled(nodeTypes []string) bool {
	has := map[string]bool{}
	for _, nodeType := range nodeTypes {
		has[nodeType] = true
	}
	for _, required := range []string{"event-in", "inbox", "normalize", "match", "fact-query", "qualify", "reward", "limit", "quota", "participation", "grant-order", "outbox", "audit"} {
		if !has[required] {
			return false
		}
	}
	return true
}

func workflowTemplateFirstNodeID(nodeTypes []string, nodeType string) string {
	for _, item := range nodeTypes {
		if item == nodeType {
			return item
		}
	}
	return ""
}

func workflowTemplateRouteEdge(source string, target string, routeKey string, defaultRoute bool) map[string]any {
	return map[string]any{
		"id":     source + "-" + strings.ToLower(routeKey) + "-" + target,
		"source": source,
		"target": target,
		"label":  routeKey,
		"route": map[string]any{
			"type":       routeKey,
			"key":        routeKey,
			"label":      routeKey,
			"expression": "decision == \"" + routeKey + "\"",
			"default":    defaultRoute,
		},
	}
}

func workflowTemplateNodeKind(nodeType string) string {
	switch nodeType {
	case "event-in":
		return "entry"
	case "inbox", "participation", "audit", "activity-state", "state-store":
		return "store"
	case "reward", "candidate-reward":
		return "reward"
	case "limit", "quota", "quota-check", "budget-reserve":
		return "quota"
	case "grant-order":
		return "grant"
	case "outbox", "plugin-execute":
		return "dispatch"
	case "fact-query", "context-query", "local-cache-query", "http-query", "redis-query", "grpc-query", "plugin-query", "custom-query", "lua-query":
		return "query"
	default:
		return "logic"
	}
}

func workflowTemplateNodeTitle(nodeType string) string {
	titles := map[string]string{"event-in": "标准事件入口", "inbox": "幂等收件箱", "normalize": "事件标准化", "match": "活动匹配", "qualify": "资格校验", "reward": "奖励计算", "limit": "次数限制", "quota": "预算库存扣减", "participation": "参与记录", "grant-order": "发奖订单", "outbox": "消息发件箱", "callback": "结果回调", "audit": "审计记录", "fact-query": "事实查询", "context-query": "上下文查询", "local-cache-query": "本地缓存查询", "http-query": "HTTP 查询", "redis-query": "Redis 查询", "grpc-query": "gRPC 查询", "plugin-query": "插件查询", "custom-query": "自定义查询", "lua-query": "Lua 查询", "combine": "组合节点", "split": "拆分节点", "activity-state": "活动状态", "state-store": "通用存储", "quota-check": "次数频控校验", "candidate-reward": "候选奖励", "budget-reserve": "预算库存锁定", "approval-gate": "审批网关", "compensation": "业务补偿", "random-pick": "随机抽取", "plugin-execute": "插件执行", "ab-bucket": "AB 分桶", "risk-score": "风险评分", "settlement-window": "结算窗口", "manual-adjust": "人工调整", "reconcile": "对账校验", "reward-reconcile": "奖励对账"}
	return titles[nodeType]
}

func workflowTemplateNodeInputs(nodeType string) []string {
	inputs := map[string][]string{"event-in": {"raw_event"}, "inbox": {"raw_event"}, "normalize": {"raw_event"}, "match": {"event_context"}, "qualify": {"matched_activity"}, "reward": {"qualified_context"}, "limit": {"subject"}, "quota": {"reward_items"}, "participation": {"qualified_context", "quota_hold"}, "grant-order": {"participation_id", "reward_items"}, "outbox": {"grant_order"}, "callback": {"quota_hold"}, "audit": {"stage_result"}, "fact-query": {"subject"}, "context-query": {"subject"}, "local-cache-query": {"subject"}, "http-query": {"subject"}, "redis-query": {"subject"}, "grpc-query": {"subject"}, "plugin-query": {"subject"}, "custom-query": {"subject"}, "lua-query": {"subject"}, "combine": {"items", "context"}, "split": {"payload", "items"}, "activity-state": {"event_context", "state_patch"}, "state-store": {"value", "state_patch"}, "quota-check": {"subject", "activity_id"}, "candidate-reward": {"qualified_context", "plugin_result"}, "budget-reserve": {"candidate_rewards", "reward_items"}, "approval-gate": {"candidate_rewards", "risk_result"}, "compensation": {"stage_result"}, "random-pick": {"candidate_rewards"}, "plugin-execute": {"event_context", "activity_state"}, "ab-bucket": {"subject"}, "risk-score": {"facts"}, "settlement-window": {"event_context"}, "manual-adjust": {"operator"}, "reconcile": {"date_range"}, "reward-reconcile": {"date_range"}}
	return inputs[nodeType]
}

func workflowTemplateNodeOutputs(nodeType string) []string {
	outputs := map[string][]string{"event-in": {"raw_event"}, "inbox": {"event_context"}, "normalize": {"normalized_event"}, "match": {"matched_activity"}, "qualify": {"stage_result", "decision", "PASS", "BLOCK", "RISK"}, "reward": {"reward_items"}, "limit": {"limit_hold"}, "quota": {"quota_hold"}, "participation": {"participation_id"}, "grant-order": {"grant_order"}, "outbox": {"HTTP", "MQ", "Webhook", "Failed"}, "callback": {"callback_result"}, "audit": {"audit_log"}, "fact-query": {"facts"}, "context-query": {"facts"}, "local-cache-query": {"facts"}, "http-query": {"facts"}, "redis-query": {"facts"}, "grpc-query": {"facts"}, "plugin-query": {"facts"}, "custom-query": {"facts"}, "lua-query": {"facts"}, "combine": {"combined_result"}, "split": {"split_items", "split_result"}, "activity-state": {"activity_state", "state_version"}, "state-store": {"stored_value", "storage_entry", "storage_version"}, "quota-check": {"quota_check", "limit_hold"}, "candidate-reward": {"candidate_rewards", "reward_items"}, "budget-reserve": {"budget_reservation", "quota_hold"}, "approval-gate": {"approval_decision"}, "compensation": {"compensation_plan"}, "random-pick": {"random_pick", "selected_rewards"}, "plugin-execute": {"plugin_result", "candidate_rewards"}, "ab-bucket": {"ab_bucket"}, "risk-score": {"risk_result"}, "settlement-window": {"settlement_window"}, "manual-adjust": {"manual_adjustment"}, "reconcile": {"reconcile_report"}, "reward-reconcile": {"reward_reconcile_report"}}
	return outputs[nodeType]
}

func workflowTemplateInputMapping(nodeType string, upstreamNodeTypes []string) map[string]any {
	ref := func(sourceNodeID string, outputKey string, path string) map[string]any {
		item := map[string]any{"sourceNodeId": sourceNodeID, "outputKey": outputKey}
		if strings.TrimSpace(path) != "" {
			item["path"] = path
		}
		return item
	}
	hasUpstream := func(candidate string) bool {
		for _, nodeType := range upstreamNodeTypes {
			if nodeType == candidate {
				return true
			}
		}
		return false
	}
	eventContextRef := func(path string) map[string]any {
		if hasUpstream("normalize") {
			return ref("normalize", "normalized_event", path)
		}
		if hasUpstream("inbox") {
			return ref("inbox", "event_context", path)
		}
		return ref("event-in", "raw_event", path)
	}
	switch nodeType {
	case "inbox":
		return map[string]any{"raw_event": ref("event-in", "raw_event", "")}
	case "normalize":
		return map[string]any{"raw_event": ref("event-in", "raw_event", "")}
	case "fact-query", "context-query", "local-cache-query", "http-query", "redis-query", "grpc-query", "plugin-query", "custom-query", "lua-query":
		return map[string]any{"subject": eventContextRef("$.subject")}
	case "match":
		return map[string]any{"event_context": eventContextRef("")}
	case "qualify":
		if !hasUpstream("match") {
			return map[string]any{"event_context": eventContextRef("")}
		}
		return map[string]any{
			"event_context":    eventContextRef(""),
			"matched_activity": ref("match", "matched_activity", ""),
		}
	case "activity-state":
		return map[string]any{"event_context": eventContextRef("")}
	case "quota-check":
		mapping := map[string]any{"subject": eventContextRef("$.subject")}
		if hasUpstream("match") {
			mapping["activity_id"] = ref("match", "matched_activity", "$.activityId")
		}
		return mapping
	case "reward":
		return map[string]any{"qualified_context": ref("qualify", "PASS", "")}
	case "candidate-reward":
		mapping := map[string]any{"qualified_context": ref("qualify", "PASS", "")}
		if hasUpstream("plugin-execute") {
			mapping["plugin_result"] = ref("plugin-execute", "plugin_result", "")
		}
		return mapping
	case "limit":
		mapping := map[string]any{"subject": eventContextRef("$.subject")}
		if hasUpstream("match") {
			mapping["activity_id"] = ref("match", "matched_activity", "$.activityId")
		}
		return mapping
	case "quota":
		return map[string]any{
			"reward_items": ref("reward", "reward_items", ""),
			"limit_hold":   ref("limit", "limit_hold", ""),
		}
	case "budget-reserve":
		mapping := map[string]any{}
		if hasUpstream("candidate-reward") {
			mapping["candidate_rewards"] = ref("candidate-reward", "candidate_rewards", "")
		}
		if hasUpstream("reward") {
			mapping["reward_items"] = ref("reward", "reward_items", "")
		}
		return mapping
	case "participation":
		quotaSource := "quota"
		if hasUpstream("budget-reserve") {
			quotaSource = "budget-reserve"
		}
		return map[string]any{
			"qualified_context": ref("qualify", "PASS", ""),
			"quota_hold":        ref(quotaSource, "quota_hold", ""),
		}
	case "grant-order":
		rewardSource := "reward"
		if hasUpstream("candidate-reward") {
			rewardSource = "candidate-reward"
		}
		return map[string]any{
			"participation_id": ref("participation", "participation_id", ""),
			"reward_items":     ref(rewardSource, "reward_items", ""),
		}
	case "outbox":
		return map[string]any{"grant_order": ref("grant-order", "grant_order", "")}
	case "reconcile", "reward-reconcile":
		return map[string]any{"date_range": eventContextRef("$.occurredAt")}
	case "plugin-execute":
		mapping := map[string]any{"event_context": eventContextRef("")}
		if hasUpstream("activity-state") {
			mapping["activity_state"] = ref("activity-state", "activity_state", "")
		}
		return mapping
	case "random-pick":
		return map[string]any{"candidate_rewards": ref("candidate-reward", "candidate_rewards", "")}
	case "approval-gate":
		mapping := map[string]any{"candidate_rewards": ref("candidate-reward", "candidate_rewards", "")}
		if hasUpstream("risk-score") {
			mapping["risk_result"] = ref("risk-score", "risk_result", "")
		}
		return mapping
	case "risk-score":
		return map[string]any{"facts": ref("fact-query", "facts", "")}
	case "settlement-window":
		return map[string]any{"event_context": eventContextRef("")}
	case "compensation":
		if hasUpstream("callback") {
			return map[string]any{"stage_result": ref("callback", "callback_result", "")}
		}
		return nil
	case "audit":
		if hasUpstream("qualify") {
			return map[string]any{"stage_result": ref("qualify", "stage_result", "")}
		}
		if len(upstreamNodeTypes) == 0 {
			return nil
		}
		source := upstreamNodeTypes[len(upstreamNodeTypes)-1]
		outputs := workflowTemplateNodeOutputs(source)
		if len(outputs) == 0 {
			return nil
		}
		return map[string]any{"stage_result": ref(source, outputs[0], "")}
	default:
		return nil
	}
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
			"mode":                    "FIRST_MATCH",
			"requireActiveActivity":   true,
			"requireApprovedActivity": false,
			"priorityOrder":           "PRIORITY_DESC",
			"tenantMatchPolicy":       "SAME_TENANT",
			"fallbackAction":          "NO_MATCH_FAIL",
			"activityCodePattern":     "",
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
		"participation": {
			"status":      "CREATED",
			"deduplicate": true,
		},
		"grant-order": {
			"provider":      "HTTP",
			"expireSeconds": 3600,
		},
		"outbox": {
			"channel":     "HTTP",
			"maxAttempts": 3,
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
			"providerChain":   []string{},
			"subjectPath":     "subject.id",
			"providerCode":    "",
			"endpoint":        "",
			"method":          "POST",
			"authRef":         "",
			"mapping":         map[string]any{},
			"redisRef":        "",
			"keyTemplate":     "",
			"grpcTarget":      "",
			"pluginCode":      "",
			"timeoutSeconds":  3,
			"cacheTtlSeconds": 60,
			"failureAction":   "USE_EMPTY",
		},
		"context-query": {
			"factKeys":      []string{"USER_PROFILE"},
			"sourcePath":    "$.attributes.USER_PROFILE",
			"failureAction": "USE_EMPTY",
		},
		"local-cache-query": {
			"factKeys":        []string{"USER_PROFILE"},
			"cachePath":       "$.vars.execution.facts.USER_PROFILE",
			"cacheTtlSeconds": 60,
			"failureAction":   "USE_EMPTY",
		},
		"http-query": {
			"factKeys":        []string{"USER_PROFILE"},
			"source":          "HTTP_PROVIDER",
			"subjectPath":     "subject.id",
			"providerCode":    "",
			"endpoint":        "https://user-service/internal/facts/user-profile",
			"method":          "POST",
			"authRef":         "internal-service-token",
			"mapping":         map[string]any{"vipLevel": map[string]any{"source": "$.data.profile.vipLevel", "type": "string", "required": false}},
			"timeoutSeconds":  3,
			"cacheTtlSeconds": 60,
			"failureAction":   "BLOCK",
		},
		"redis-query": {
			"factKeys":      []string{"USER_PROFILE"},
			"redisRef":      "marketing-fact-cache",
			"keyTemplate":   "marketing:fact:${tenantId}:USER_PROFILE:${subject.id}",
			"command":       "GET",
			"valuePath":     "$",
			"mapping":       map[string]any{},
			"failureAction": "SKIP",
		},
		"grpc-query": {
			"factKeys":       []string{"USER_PROFILE"},
			"grpcTarget":     "dns:///user-service:9090",
			"service":        "user.ProfileFactService",
			"method":         "QueryFacts",
			"authority":      "user-service",
			"tls":            true,
			"mapping":        map[string]any{},
			"timeoutSeconds": 3,
			"failureAction":  "SKIP",
		},
		"plugin-query": {
			"factKeys":       []string{"USER_PROFILE"},
			"pluginCode":     "user-profile",
			"action":         "QUERY_FACTS",
			"inputPath":      "$",
			"mapping":        map[string]any{},
			"timeoutSeconds": 3,
			"failureAction":  "SKIP",
		},
		"custom-query": {
			"factKeys":        []string{"USER_PROFILE"},
			"providerCode":    "custom-user-profile",
			"protocol":        "CUSTOM",
			"endpoint":        "custom://user-profile/facts",
			"requestTemplate": map[string]any{},
			"responseMapping": map[string]any{},
			"timeoutSeconds":  3,
			"failureAction":   "SKIP",
		},
		"lua-query": {
			"factCode":      "USER_PROFILE",
			"source":        "emit({facts = {USER_PROFILE = {vipLevel = input('vipLevel')}}})",
			"failureAction": "BLOCK",
		},
		"activity-state": {
			"storeType":        "INTERNAL",
			"operation":        "READ",
			"namespace":        "activity",
			"stateKeyTemplate": "${activityId}:${subject.id}",
			"valuePath":        "$.state",
			"requireVersion":   true,
			"missingPolicy":    "EMPTY_STATE",
			"providerCode":     "",
			"requestMapping":   map[string]any{},
			"responseMapping":  map[string]any{},
			"timeoutSeconds":   3,
		},
		"state-store": {
			"resourceRef":      "default_workflow_state",
			"storageType":      "MEMORY",
			"operation":        "DEFINE",
			"namespace":        "workflow",
			"keyTemplate":      "${activityId}:${eventId}:${nodeId}",
			"valueType":        "OBJECT",
			"valuePath":        "",
			"defaultValue":     map[string]any{},
			"tableSchema":      map[string]any{},
			"missingPolicy":    "EMPTY",
			"requireVersion":   false,
			"expectedVersion":  "",
			"providerCode":     "",
			"providerEndpoint": "",
			"httpMethod":       "POST",
			"requestHeaders":   map[string]any{},
			"timeoutSeconds":   3,
		},
		"quota-check": {
			"scope":              "USER",
			"scopeKeyTemplate":   "${subject.id}:${activityId}",
			"window":             "DAY",
			"maxCount":           1,
			"consumeMode":        "CHECK_ONLY",
			"exceedAction":       "BLOCK",
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor/QuotaStore",
		},
		"candidate-reward": {
			"source":                 "ACTIVITY_REWARD_CONFIG",
			"rewardItemsPath":        "$.reward_items",
			"includeDisabledRewards": false,
			"notQualifiedAction":     "SKIP",
			"emptyAction":            "BLOCK",
			"includeTrace":           true,
		},
		"budget-reserve": {
			"quotaCode":        "DEFAULT_REWARD_BUDGET",
			"quotaType":        "BUDGET",
			"scope":            "ACTIVITY",
			"scopeKeyTemplate": "${activityId}",
			"amountField":      "candidate_rewards[0].amount",
			"reserveMode":      "ATOMIC",
			"failAction":       "BLOCK",
			"releaseOnFailure": true,
		},
		"approval-gate": {
			"mode":               "AUTO_BELOW_THRESHOLD",
			"amountThreshold":    1000,
			"riskLabels":         []string{"HIGH_RISK"},
			"pendingAction":      "PAUSE",
			"executionMode":      "EXTERNAL_ENDPOINT",
			"executionAuthority": "ActivityApproval/AdminAPI",
		},
		"compensation": {
			"triggerOn":          "FAILED",
			"sourcePath":         "$.stage_result",
			"action":             "RETRY_CALLBACK",
			"maxAttempts":        3,
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "CompensationWorker/AdminAPI",
		},
		"random-pick": {
			"mode":               "WEIGHTED",
			"itemsPath":          "$.candidate_rewards",
			"seedTemplate":       "${tenantId}:${activityId}:${eventId}",
			"pickCount":          1,
			"emptyAction":        "SKIP",
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor",
		},
		"plugin-execute": {
			"pluginCode":         "checkin",
			"action":             "EXECUTE",
			"inputPath":          "$",
			"failureAction":      "BLOCK",
			"executionMode":      "EXTERNAL_ENDPOINT",
			"executionAuthority": "ActivityPlugin",
		},
		"ab-bucket": {
			"experimentKey":      "default-experiment",
			"subjectKeyTemplate": "${subject.id}",
			"buckets":            []string{"A:50", "B:50"},
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor",
		},
		"risk-score": {
			"source":             "FACTS",
			"scorePath":          "$.facts.riskScore",
			"blockThreshold":     90,
			"reviewThreshold":    70,
			"failureAction":      "REVIEW",
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor/FactService",
		},
		"settlement-window": {
			"windowType":         "DAILY",
			"start":              "00:00",
			"end":                "23:59",
			"timezone":           "UTC",
			"outsideAction":      "SKIP",
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor/Scheduler",
		},
		"manual-adjust": {
			"adjustType":         "REWARD",
			"targetPath":         "$.grant_order",
			"requireReason":      true,
			"requireApproval":    true,
			"executionMode":      "EXTERNAL_ENDPOINT",
			"executionAuthority": "AdminManualAdjustAPI",
		},
		"reward-reconcile": {
			"targets":            []string{"grant", "quota", "callback", "outbox"},
			"autoRepair":         false,
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "ReconciliationJob",
		},
		"combine": {
			"language":       "LUA",
			"timeoutSeconds": 5,
			"maxOutputBytes": 65536,
			"source":         "emit({combined = ctx.node.input})",
			"outputPath":     "combined_result",
			"allowNetwork":   false,
		},
		"split": {
			"language":       "LUA",
			"timeoutSeconds": 5,
			"maxOutputBytes": 65536,
			"source":         "emit({items = ctx.node.input.items})",
			"outputPath":     "split_items",
			"allowNetwork":   false,
		},
		"normalize": {"mapping": map[string]any{}, "defaults": map[string]any{}},
	}
	return configs[nodeType]
}
