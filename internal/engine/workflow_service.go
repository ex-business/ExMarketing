package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (s *Service) SaveWorkflowDraft(ctx context.Context, activityID string, cmd marketing.SaveWorkflowDraftCommand) (marketing.WorkflowDraft, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return marketing.WorkflowDraft{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, activityID)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	definition, err := marketing.NormalizeWorkflowDraftDefinition(cmd.Definition)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	now := s.now()
	draft := marketing.WorkflowDraft{
		ActivityID: activity.ID,
		TenantID:   activity.TenantID,
		Definition: definition,
		UpdatedBy:  strings.TrimSpace(cmd.Operator),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	saved, err := s.repository.SaveWorkflowDraft(ctx, draft)
	if err != nil {
		return marketing.WorkflowDraft{}, err
	}
	_ = s.audit(ctx, saved.TenantID, actorOrSystem(cmd.Operator), "SAVE_WORKFLOW_DRAFT", "WORKFLOW_DRAFT", saved.ActivityID, auditSnapshot(nil, nil, map[string]any{
		"activityId": saved.ActivityID,
		"updatedAt":  saved.UpdatedAt,
	}))
	return saved, nil
}

func (s *Service) GetWorkflowDraft(ctx context.Context, activityID string) (marketing.WorkflowDraft, error) {
	return s.repository.GetWorkflowDraft(ctx, strings.TrimSpace(activityID))
}

func (s *Service) ValidateWorkflowDefinition(ctx context.Context, activityID string, cmd marketing.ValidateWorkflowCommand) (marketing.WorkflowValidationResult, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return marketing.WorkflowValidationResult{}, marketing.ErrNotFound
	}
	if _, err := s.repository.GetActivity(ctx, activityID); err != nil {
		return marketing.WorkflowValidationResult{}, err
	}
	definition := cmd.Definition
	if len(definition) == 0 {
		draft, err := s.repository.GetWorkflowDraft(ctx, activityID)
		if err != nil {
			return marketing.WorkflowValidationResult{}, err
		}
		definition = draft.Definition
	}
	return s.workflowValidator.ValidateDefinition(definition), nil
}

func (s *Service) ValidateWorkflowDraft(ctx context.Context, activityID string) (marketing.WorkflowValidationResult, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return marketing.WorkflowValidationResult{}, marketing.ErrNotFound
	}
	if _, err := s.repository.GetActivity(ctx, activityID); err != nil {
		return marketing.WorkflowValidationResult{}, err
	}
	draft, err := s.repository.GetWorkflowDraft(ctx, activityID)
	if err != nil {
		return marketing.WorkflowValidationResult{}, err
	}
	return s.workflowValidator.ValidateDefinition(draft.Definition), nil
}

func (s *Service) PublishWorkflowVersion(ctx context.Context, activityID string, cmd marketing.PublishWorkflowCommand) (marketing.WorkflowVersion, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return marketing.WorkflowVersion{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, activityID)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	draft, err := s.repository.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return marketing.WorkflowVersion{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New("workflow draft is required before publish"))
		}
		return marketing.WorkflowVersion{}, err
	}
	if validation := s.validateWorkflowBeforePublish(draft.Definition); !validation.Valid {
		return marketing.WorkflowVersion{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New(workflowValidationMessage(validation)))
	}
	definition, err := marketing.NormalizeWorkflowDefinition(draft.Definition)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	now := s.now()
	version := marketing.WorkflowVersion{
		ID:          marketingID("wfver"),
		TenantID:    activity.TenantID,
		ActivityID:  activity.ID,
		Status:      marketing.WorkflowVersionActive,
		Definition:  definition,
		PublishedBy: strings.TrimSpace(cmd.Operator),
		PublishedAt: now,
		ReleaseNote: strings.TrimSpace(cmd.ReleaseNote),
		Action:      "PUBLISH",
		CreatedAt:   now,
	}
	created, err := s.repository.CreateWorkflowVersion(ctx, version)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	_ = s.audit(ctx, created.TenantID, actorOrSystem(cmd.Operator), "PUBLISH_WORKFLOW_VERSION", "WORKFLOW_VERSION", created.ID, auditSnapshot(nil, created, map[string]any{
		"activityId":  created.ActivityID,
		"version":     created.Version,
		"releaseNote": created.ReleaseNote,
	}))
	return created, nil
}

func (s *Service) validateWorkflowBeforePublish(definition json.RawMessage) marketing.WorkflowValidationResult {
	return s.workflowValidator.ValidateDefinition(definition)
}

func (s *Service) ListWorkflowVersions(ctx context.Context, activityID string) ([]marketing.WorkflowVersion, error) {
	return s.repository.ListWorkflowVersions(ctx, strings.TrimSpace(activityID))
}

func (s *Service) RollbackWorkflowVersion(ctx context.Context, activityID string, versionID string, cmd marketing.WorkflowVersionActionCommand) (marketing.WorkflowVersion, error) {
	activityID = strings.TrimSpace(activityID)
	versionID = strings.TrimSpace(versionID)
	if activityID == "" || versionID == "" {
		return marketing.WorkflowVersion{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, activityID)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	source, err := s.repository.GetWorkflowVersion(ctx, activity.ID, versionID)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	if validation := s.validateWorkflowBeforePublish(source.Definition); !validation.Valid {
		return marketing.WorkflowVersion{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New(workflowValidationMessage(validation)))
	}
	definition, err := marketing.NormalizeWorkflowDefinition(source.Definition)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	now := s.now()
	version := marketing.WorkflowVersion{
		ID:              marketingID("wfver"),
		TenantID:        activity.TenantID,
		ActivityID:      activity.ID,
		Status:          marketing.WorkflowVersionActive,
		Definition:      definition,
		PublishedBy:     strings.TrimSpace(cmd.Operator),
		PublishedAt:     now,
		ReleaseNote:     strings.TrimSpace(cmd.Note),
		SourceVersionID: source.ID,
		Action:          "ROLLBACK",
		CreatedAt:       now,
	}
	created, err := s.repository.CreateWorkflowVersion(ctx, version)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	_ = s.audit(ctx, created.TenantID, actorOrSystem(cmd.Operator), "ROLLBACK_WORKFLOW_VERSION", "WORKFLOW_VERSION", created.ID, auditSnapshot(nil, created, map[string]any{
		"activityId":      created.ActivityID,
		"version":         created.Version,
		"sourceVersionId": source.ID,
		"sourceVersion":   source.Version,
		"note":            created.ReleaseNote,
	}))
	return created, nil
}

func (s *Service) ArchiveWorkflowVersion(ctx context.Context, activityID string, versionID string, cmd marketing.WorkflowVersionActionCommand) (marketing.WorkflowVersion, error) {
	activityID = strings.TrimSpace(activityID)
	versionID = strings.TrimSpace(versionID)
	if activityID == "" || versionID == "" {
		return marketing.WorkflowVersion{}, marketing.ErrNotFound
	}
	version, err := s.repository.GetWorkflowVersion(ctx, activityID, versionID)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	if version.Status == marketing.WorkflowVersionActive {
		return marketing.WorkflowVersion{}, errors.Join(marketing.ErrInvalidWorkflow, errors.New("active workflow version cannot be archived; rollback another version first"))
	}
	if version.Status == marketing.WorkflowVersionArchived {
		return version, nil
	}
	archived, err := s.repository.UpdateWorkflowVersionStatus(ctx, activityID, versionID, marketing.WorkflowVersionArchived)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	_ = s.audit(ctx, archived.TenantID, actorOrSystem(cmd.Operator), "ARCHIVE_WORKFLOW_VERSION", "WORKFLOW_VERSION", archived.ID, auditSnapshot(nil, archived, map[string]any{
		"activityId": archived.ActivityID,
		"version":    archived.Version,
		"note":       strings.TrimSpace(cmd.Note),
	}))
	return archived, nil
}

func (s *Service) ListWorkflowExecutions(ctx context.Context, query marketing.WorkflowExecutionQuery) ([]marketing.WorkflowExecution, error) {
	query.ActivityID = strings.TrimSpace(query.ActivityID)
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.WorkflowVersionID = strings.TrimSpace(query.WorkflowVersionID)
	query.EventID = strings.TrimSpace(query.EventID)
	query.ExecutionID = strings.TrimSpace(query.ExecutionID)
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return s.repository.ListWorkflowExecutions(ctx, query)
}

func (s *Service) ListWorkflowNodeExecutions(ctx context.Context, executionID string) ([]marketing.WorkflowNodeExecution, error) {
	return s.ListWorkflowNodeExecutionsPage(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: executionID})
}

func (s *Service) ListWorkflowNodeExecutionsPage(ctx context.Context, query marketing.WorkflowNodeExecutionQuery) ([]marketing.WorkflowNodeExecution, error) {
	query.ExecutionID = strings.TrimSpace(query.ExecutionID)
	if query.ExecutionID == "" {
		return nil, marketing.ErrNotFound
	}
	if query.Limit < 0 {
		query.Limit = 0
	}
	if query.Limit > 500 {
		query.Limit = 500
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	return s.repository.ListWorkflowNodeExecutions(ctx, query)
}

func (s *Service) ListWorkflowNodeExecutionsSummary(ctx context.Context, executionID string) ([]marketing.WorkflowNodeExecution, error) {
	nodes, err := s.ListWorkflowNodeExecutions(ctx, executionID)
	if err != nil {
		return nil, err
	}
	for index := range nodes {
		nodes[index].Input = nil
		nodes[index].Output = nil
	}
	return nodes, nil
}

func (s *Service) GetWorkflowNodeExecution(ctx context.Context, executionID string, nodeExecutionID string) (marketing.WorkflowNodeExecution, error) {
	nodeExecutionID = strings.TrimSpace(nodeExecutionID)
	if nodeExecutionID == "" {
		return marketing.WorkflowNodeExecution{}, marketing.ErrNotFound
	}
	nodes, err := s.ListWorkflowNodeExecutions(ctx, executionID)
	if err != nil {
		return marketing.WorkflowNodeExecution{}, err
	}
	for _, node := range nodes {
		if node.ID == nodeExecutionID {
			return node, nil
		}
	}
	return marketing.WorkflowNodeExecution{}, marketing.ErrNotFound
}

func (s *Service) CancelWorkflowExecution(ctx context.Context, executionID string, cmd marketing.CancelWorkflowExecutionCommand) (marketing.WorkflowExecution, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return marketing.WorkflowExecution{}, marketing.ErrNotFound
	}
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: executionID, Limit: 1})
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	if len(executions) == 0 {
		return marketing.WorkflowExecution{}, marketing.ErrNotFound
	}
	execution := executions[0]
	if execution.Status != marketing.WorkflowExecutionRunning {
		return execution, nil
	}
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		reason = "execution canceled"
	}
	now := s.now()
	execution.Status = marketing.WorkflowExecutionCanceled
	execution.ErrorMessage = reason
	execution.Output = marshalWorkflowPayload(map[string]any{
		"canceled": true,
		"operator": strings.TrimSpace(cmd.Operator),
		"reason":   reason,
	})
	execution.FinishedAt = &now
	execution.UpdatedAt = now
	if err := s.repository.UpdateWorkflowExecution(ctx, execution); err != nil {
		return marketing.WorkflowExecution{}, err
	}
	_ = s.audit(ctx, execution.TenantID, actorOrSystem(cmd.Operator), "CANCEL_WORKFLOW_EXECUTION", "WORKFLOW_EXECUTION", execution.ID, auditSnapshot(nil, execution, map[string]any{
		"activityId":        execution.ActivityID,
		"workflowVersionId": execution.WorkflowVersionID,
		"reason":            reason,
	}))
	return execution, nil
}

func (s *Service) ResumeWorkflowExecution(ctx context.Context, executionID string, cmd marketing.ResumeWorkflowExecutionCommand) (marketing.WorkflowExecutionTrace, error) {
	execution, err := s.findWorkflowExecutionByID(ctx, executionID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	if execution.Status != marketing.WorkflowExecutionFailed && execution.Status != marketing.WorkflowExecutionCanceled {
		return s.ExportWorkflowExecutionTrace(ctx, execution.ID)
	}
	activity, err := s.repository.GetActivity(ctx, execution.ActivityID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	version, err := s.repository.GetWorkflowVersion(ctx, execution.ActivityID, execution.WorkflowVersionID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	event, err := workflowExecutionInputEvent(execution.Input)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	startNodeID := strings.TrimSpace(cmd.StartNodeID)
	if startNodeID == "" {
		startNodeID = s.workflowResumeStartNodeID(ctx, execution.ID)
	}
	initialOutputs, err := s.workflowInitialNodeOutputsUntil(ctx, execution.ID, startNodeID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	resumeRuntime, err := s.workflowResumeRuntime(ctx, execution, activity, event)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	resumed, err := s.executeWorkflowDefinition(ctx, resumeRuntime, workflowExecutionOptions{
		WorkflowVersionID:  version.ID,
		WorkflowVersion:    version.Version,
		Definition:         version.Definition,
		Mode:               "RESUME",
		SkipIdempotency:    true,
		StartNodeID:        startNodeID,
		SourceExecutionID:  execution.ID,
		InitialNodeOutputs: initialOutputs,
	})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	_ = s.audit(ctx, execution.TenantID, actorOrSystem(cmd.Operator), "RESUME_WORKFLOW_EXECUTION", "WORKFLOW_EXECUTION", resumed.ID, auditSnapshot(execution, resumed, map[string]any{
		"sourceExecutionId": execution.ID,
		"startNodeId":       startNodeID,
		"reason":            strings.TrimSpace(cmd.Reason),
	}))
	return s.ExportWorkflowExecutionTrace(ctx, resumed.ID)
}

func (s *Service) workflowResumeRuntime(ctx context.Context, execution marketing.WorkflowExecution, activity marketing.Activity, event marketing.ActivityEvent) (workflowRuntime, error) {
	runtime := workflowRuntime{
		Inbox: marketing.EventInbox{
			ID:             execution.EventInboxID,
			Event:          event,
			IdempotencyKey: workflowExecutionInputString(execution.Input, "idempotencyKey"),
		},
		Activity: activity,
		RuleResult: marketing.ActivityRuleResult{
			ActivityID:      activity.ID,
			ActivityCode:    activity.Code,
			ActivityVersion: activity.Version,
			Decision:        marketing.RuleDecisionRejected,
			ReasonCode:      "RESUME_CONTEXT_REHYDRATED",
			Reason:          "resume context was reconstructed from persisted execution state",
		},
		Reason: "resume context was reconstructed from persisted execution state",
	}
	items, err := s.workflowResumeReconciliationItems(ctx, activity, event, runtime.Inbox.IdempotencyKey)
	if err != nil {
		return workflowRuntime{}, err
	}
	for _, item := range items {
		if item.EventInbox != nil && runtime.Inbox.ID == "" {
			runtime.Inbox = *item.EventInbox
		}
		if runtime.Participation.ID == "" {
			if item.Participation != nil {
				runtime.Participation = *item.Participation
			} else if item.ParticipationID != "" {
				runtime.Participation = marketing.Participation{
					ID:         item.ParticipationID,
					TenantID:   item.TenantID,
					ActivityID: item.ActivityID,
					Subject:    item.Subject,
					Status:     item.ParticipationStatus,
				}
			}
		}
		if item.GrantOrder != nil {
			runtime.GrantOrders = append(runtime.GrantOrders, *item.GrantOrder)
		} else if item.GrantOrderID != "" {
			runtime.GrantOrders = append(runtime.GrantOrders, marketing.GrantOrder{
				ID:              item.GrantOrderID,
				TenantID:        item.TenantID,
				ActivityID:      item.ActivityID,
				ParticipationID: item.ParticipationID,
				Subject:         item.Subject,
				RewardType:      item.RewardType,
				RewardAmount:    item.RewardAmount,
				BillNo:          item.CommandID,
				GrantStatus:     item.GrantStatus,
				ExternalGrantID: item.ExternalGrantID,
				FinishedAt:      item.FinishedAt,
				UpdatedAt:       item.ServerUpdatedAt,
			})
		}
		runtime.OutboxMessages = append(runtime.OutboxMessages, item.OutboxMessages...)
		runtime.Reservations = append(runtime.Reservations, item.Reservations...)
	}
	runtime.GrantOrders = dedupeWorkflowGrantOrders(runtime.GrantOrders)
	runtime.OutboxMessages = dedupeWorkflowOutboxMessages(runtime.OutboxMessages)
	runtime.Reservations = dedupeWorkflowQuotaReservations(runtime.Reservations)
	if runtime.Participation.ID != "" {
		switch runtime.Participation.Status {
		case marketing.ParticipationQualified, marketing.ParticipationRewardable, marketing.ParticipationGranted:
			runtime.Qualified = true
			runtime.RuleResult.Decision = marketing.RuleDecisionPassed
			runtime.RuleResult.CanReward = true
			runtime.RuleResult.CanReserveQuota = true
			runtime.RuleResult.ReasonCode = "RESUME_CONTEXT_REHYDRATED"
			runtime.RuleResult.Reason = "persisted participation is qualified"
			runtime.Reason = runtime.RuleResult.Reason
		default:
			runtime.RuleResult.ReasonCode = firstWorkflowString(runtime.Participation.RejectReason, "RESUME_CONTEXT_REHYDRATED")
			runtime.RuleResult.Reason = firstWorkflowString(runtime.Participation.RejectReason, "persisted participation is not qualified")
			runtime.Reason = runtime.RuleResult.Reason
		}
	}
	return runtime, nil
}

func (s *Service) workflowResumeReconciliationItems(ctx context.Context, activity marketing.Activity, event marketing.ActivityEvent, executionIdempotencyKey string) ([]marketing.RewardReconciliationItem, error) {
	items, err := s.repository.ListRewardReconciliation(ctx, marketing.RewardReconciliationQuery{
		TenantID:   activity.TenantID,
		ActivityID: activity.ID,
		EventID:    event.EventID,
		Limit:      50,
	})
	if err != nil {
		return nil, err
	}
	if len(items) > 0 || strings.TrimSpace(executionIdempotencyKey) == "" {
		return items, nil
	}
	return s.repository.ListRewardReconciliation(ctx, marketing.RewardReconciliationQuery{
		TenantID:       activity.TenantID,
		ActivityID:     activity.ID,
		IdempotencyKey: executionIdempotencyKey,
		Limit:          50,
	})
}

func dedupeWorkflowGrantOrders(items []marketing.GrantOrder) []marketing.GrantOrder {
	seen := map[string]struct{}{}
	out := make([]marketing.GrantOrder, 0, len(items))
	for _, item := range items {
		key := firstWorkflowString(item.ID, firstWorkflowString(item.BillNo, item.IdempotencyKey))
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeWorkflowOutboxMessages(items []marketing.OutboxMessage) []marketing.OutboxMessage {
	seen := map[string]struct{}{}
	out := make([]marketing.OutboxMessage, 0, len(items))
	for _, item := range items {
		key := item.ID
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeWorkflowQuotaReservations(items []marketing.QuotaReservation) []marketing.QuotaReservation {
	seen := map[string]struct{}{}
	out := make([]marketing.QuotaReservation, 0, len(items))
	for _, item := range items {
		key := firstWorkflowString(item.ID, strings.Join([]string{item.GrantOrderID, item.CommandID, string(item.Type), string(item.Scope), item.ScopeKey, item.Amount}, "\x00"))
		if key == "" {
			out = append(out, item)
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (s *Service) RunWorkflowDebug(ctx context.Context, activityID string, cmd marketing.WorkflowDebugRunCommand) (marketing.WorkflowExecutionTrace, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return marketing.WorkflowExecutionTrace{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, activityID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	draft, err := s.repository.GetWorkflowDraft(ctx, activity.ID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	event, err := workflowDebugEventPayload(cmd.Payload, cmd.Template, activity)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	return s.runWorkflowDebugWithDefinition(ctx, activity, draft.Definition, event, cmd)
}

func (s *Service) RerunWorkflowExecution(ctx context.Context, activityID string, executionID string, cmd marketing.WorkflowDebugRunCommand) (marketing.WorkflowExecutionTrace, error) {
	activityID = strings.TrimSpace(activityID)
	executionID = strings.TrimSpace(executionID)
	if activityID == "" || executionID == "" {
		return marketing.WorkflowExecutionTrace{}, marketing.ErrNotFound
	}
	activity, err := s.repository.GetActivity(ctx, activityID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activity.ID, ExecutionID: executionID, Limit: 1})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	if len(executions) == 0 {
		return marketing.WorkflowExecutionTrace{}, marketing.ErrNotFound
	}
	source := executions[0]
	event, err := workflowEventFromExecutionInput(source.Input)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	cmd.SourceExecutionID = source.ID
	if strings.HasPrefix(source.WorkflowVersionID, "debug-draft") {
		draft, err := s.repository.GetWorkflowDraft(ctx, activity.ID)
		if err != nil {
			return marketing.WorkflowExecutionTrace{}, err
		}
		return s.runWorkflowDebugWithDefinition(ctx, activity, draft.Definition, event, cmd)
	}
	version, err := s.repository.GetWorkflowVersion(ctx, activity.ID, source.WorkflowVersionID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	return s.runWorkflowDebugWithDefinition(ctx, activity, version.Definition, event, cmd)
}

func (s *Service) ExportWorkflowExecutionTrace(ctx context.Context, executionID string) (marketing.WorkflowExecutionTrace, error) {
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: strings.TrimSpace(executionID), Limit: 1})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	if len(executions) == 0 {
		return marketing.WorkflowExecutionTrace{}, marketing.ErrNotFound
	}
	nodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: executions[0].ID})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	return marketing.WorkflowExecutionTrace{
		SchemaVersion:  "workflow.execution.trace.v1",
		ExportedAt:     s.now(),
		Execution:      executions[0],
		NodeExecutions: nodes,
		NodeSummaries:  workflowNodeSummaries(nodes),
		RouteTrace:     workflowTraceRoute(executions[0].Output),
	}, nil
}

func workflowNodeSummaries(nodes []marketing.WorkflowNodeExecution) []marketing.WorkflowNodeSummary {
	out := make([]marketing.WorkflowNodeSummary, 0, len(nodes))
	for _, node := range nodes {
		input := workflowPayloadMap(node.Input)
		output := workflowPayloadMap(node.Output)
		summary := marketing.WorkflowNodeSummary{
			NodeID:      node.NodeID,
			NodeType:    node.NodeType,
			NodeTitle:   node.NodeTitle,
			Status:      node.Status,
			InputKeys:   sortedWorkflowPayloadKeys(input),
			OutputKeys:  sortedWorkflowPayloadKeys(output),
			Decision:    workflowNodeDecision(output),
			ErrorCode:   workflowNodeErrorCode(node, output),
			Explanation: workflowNodeExplanation(node, input, output),
		}
		if node.FinishedAt != nil {
			summary.DurationMs = node.FinishedAt.Sub(node.StartedAt).Milliseconds()
		}
		out = append(out, summary)
	}
	return out
}

func workflowPayloadMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func sortedWorkflowPayloadKeys(payload map[string]any) []string {
	if len(payload) == 0 {
		return nil
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func workflowNodeDecision(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	for _, key := range []string{"decision", "route", "status", "skipReason"} {
		if value := workflowPayloadString(output[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"PASS", "BLOCK", "RISK", "IF", "ELSE", "SKIP"} {
		if workflowTruthy(output[key]) {
			return key
		}
	}
	return ""
}

func workflowNodeErrorCode(node marketing.WorkflowNodeExecution, output map[string]any) string {
	if code := workflowPayloadString(output["errorCode"]); code != "" {
		return code
	}
	if code := workflowPayloadString(output["failureCode"]); code != "" {
		return code
	}
	return workflowErrorCode(node.ErrorMessage)
}

func workflowPayloadString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func workflowErrorCode(message string) string {
	message = strings.ToUpper(strings.TrimSpace(message))
	if message == "" {
		return ""
	}
	fields := strings.FieldsFunc(message, func(r rune) bool {
		return !(r == '_' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	for _, field := range fields {
		if len(field) >= 3 && strings.Contains(field, "_") {
			return field
		}
	}
	return "WORKFLOW_NODE_ERROR"
}

func workflowNodeExplanation(node marketing.WorkflowNodeExecution, input map[string]any, output map[string]any) string {
	switch node.Status {
	case marketing.WorkflowNodeFailed:
		if code := workflowNodeErrorCode(node, output); code != "" {
			return fmt.Sprintf("%s failed with %s.", node.NodeID, code)
		}
		return node.NodeID + " failed."
	case marketing.WorkflowNodeSkipped:
		if reason := workflowPayloadString(output["skipReason"]); reason != "" {
			return fmt.Sprintf("%s skipped: %s.", node.NodeID, reason)
		}
		return node.NodeID + " skipped."
	}
	decision := workflowNodeDecision(output)
	if decision != "" {
		return fmt.Sprintf("%s completed with decision %s.", node.NodeID, decision)
	}
	inputKeys := sortedWorkflowPayloadKeys(input)
	outputKeys := sortedWorkflowPayloadKeys(output)
	if len(outputKeys) > 0 {
		return fmt.Sprintf("%s read %d input keys and produced %s.", node.NodeID, len(inputKeys), strings.Join(outputKeys, ", "))
	}
	return node.NodeID + " completed."
}

func (s *Service) runWorkflowDebugWithDefinition(ctx context.Context, activity marketing.Activity, definition json.RawMessage, event marketing.ActivityEvent, cmd marketing.WorkflowDebugRunCommand) (marketing.WorkflowExecutionTrace, error) {
	if err := event.Validate(); err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	event = event.Normalized()
	now := s.now()
	debugVersionNumber, err := s.nextDebugWorkflowVersionNumber(ctx, activity.ID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	debugVersion, err := s.repository.CreateWorkflowVersion(ctx, marketing.WorkflowVersion{
		ID:          "debug-draft-" + marketingID("wfdbg"),
		TenantID:    activity.TenantID,
		ActivityID:  activity.ID,
		Version:     debugVersionNumber,
		Status:      marketing.WorkflowVersionDraft,
		Definition:  definition,
		PublishedBy: strings.TrimSpace(cmd.Operator),
		PublishedAt: now,
		ReleaseNote: "Studio debug draft snapshot",
		Action:      "DEBUG_DRAFT",
		CreatedAt:   now,
	})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	inbox := marketing.EventInbox{
		ID:             marketingID("debuginbox"),
		Event:          event,
		IdempotencyKey: "debug:" + marketingID("idem"),
		ProcessStatus:  marketing.InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	participation, grantOrders, outboxMessages, ruleResult, _, _, _, _, _ := s.simulateActivity(ctx, event, activity)
	qualified := ruleResult.Decision == marketing.RuleDecisionPassed
	initialNodeOutputs, err := s.debugInitialNodeOutputs(ctx, cmd.SourceExecutionID, cmd.StartNodeID)
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	execution, err := s.executeWorkflowDefinition(ctx, workflowRuntime{
		Inbox:          inbox,
		Activity:       activity,
		Participation:  participation,
		GrantOrders:    grantOrders,
		OutboxMessages: outboxMessages,
		RuleResult:     ruleResult,
		Qualified:      qualified,
		Reason:         ruleResult.Reason,
	}, workflowExecutionOptions{
		WorkflowVersionID:  debugVersion.ID,
		WorkflowVersion:    debugVersion.Version,
		Definition:         definition,
		Mode:               "DEBUG_DRAFT",
		IdempotencyKey:     inbox.IdempotencyKey,
		SkipIdempotency:    true,
		StartNodeID:        strings.TrimSpace(cmd.StartNodeID),
		SourceExecutionID:  strings.TrimSpace(cmd.SourceExecutionID),
		InitialNodeOutputs: initialNodeOutputs,
	})
	if err != nil {
		return marketing.WorkflowExecutionTrace{}, err
	}
	return s.ExportWorkflowExecutionTrace(ctx, execution.ID)
}

func (s *Service) nextDebugWorkflowVersionNumber(ctx context.Context, activityID string) (int, error) {
	versions, err := s.repository.ListWorkflowVersions(ctx, activityID)
	if err != nil {
		return 0, err
	}
	next := -1
	for _, version := range versions {
		if version.Version <= next {
			next = version.Version - 1
		}
	}
	return next, nil
}

func (s *Service) debugInitialNodeOutputs(ctx context.Context, sourceExecutionID string, startNodeID string) (map[string]map[string]any, error) {
	sourceExecutionID = strings.TrimSpace(sourceExecutionID)
	startNodeID = strings.TrimSpace(startNodeID)
	if sourceExecutionID == "" || startNodeID == "" {
		return nil, nil
	}
	nodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: sourceExecutionID})
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, node := range nodes {
		if node.NodeID == startNodeID {
			break
		}
		if node.Status != marketing.WorkflowNodeSuccess {
			continue
		}
		payload := map[string]any{}
		if len(node.Output) > 0 {
			if err := json.Unmarshal(node.Output, &payload); err != nil {
				return nil, err
			}
		}
		out[node.NodeID] = payload
	}
	return out, nil
}

func (s *Service) workflowResumeStartNodeID(ctx context.Context, executionID string) string {
	nodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: strings.TrimSpace(executionID)})
	if err != nil {
		return ""
	}
	for _, node := range nodes {
		if node.Status == marketing.WorkflowNodeFailed {
			return node.NodeID
		}
	}
	for _, node := range nodes {
		if node.Status == marketing.WorkflowNodeSkipped {
			return node.NodeID
		}
	}
	return ""
}

func (s *Service) workflowInitialNodeOutputsUntil(ctx context.Context, executionID string, startNodeID string) (map[string]map[string]any, error) {
	nodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: strings.TrimSpace(executionID)})
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, node := range nodes {
		if startNodeID != "" && node.NodeID == startNodeID {
			break
		}
		if node.Status != marketing.WorkflowNodeSuccess {
			continue
		}
		payload := map[string]any{}
		if len(node.Output) > 0 {
			if err := json.Unmarshal(node.Output, &payload); err != nil {
				return nil, err
			}
		}
		out[node.NodeID] = payload
	}
	return out, nil
}

func workflowExecutionInputEvent(raw json.RawMessage) (marketing.ActivityEvent, error) {
	input := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &input); err != nil {
		return marketing.ActivityEvent{}, err
	}
	var event marketing.ActivityEvent
	if err := json.Unmarshal(input["event"], &event); err != nil {
		return marketing.ActivityEvent{}, err
	}
	return event, event.Validate()
}

func workflowExecutionInputString(raw json.RawMessage, key string) string {
	input := map[string]any{}
	_ = json.Unmarshal(raw, &input)
	return strings.TrimSpace(fmt.Sprint(input[key]))
}

func workflowDebugEventPayload(raw json.RawMessage, template string, activity marketing.Activity) (marketing.ActivityEvent, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return workflowDebugTemplateEvent(template, activity), nil
	}
	var event marketing.ActivityEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return marketing.ActivityEvent{}, err
	}
	return event, nil
}

func workflowDebugTemplateEvent(template string, activity marketing.Activity) marketing.ActivityEvent {
	eventType := "DEPOSIT_SUCCESS"
	if len(activity.TriggerEvents) > 0 && strings.TrimSpace(activity.TriggerEvents[0]) != "" {
		eventType = activity.TriggerEvents[0]
	}
	now := time.Now().UTC()
	return marketing.ActivityEvent{
		EventID:      "debug_evt_" + strconv.FormatInt(now.UnixNano(), 36),
		EventType:    eventType,
		SourceSystem: "studio-debug",
		TenantID:     activity.TenantID,
		Subject:      marketing.EventSubject{Type: "USER", ID: "debug_user"},
		Biz:          marketing.EventBiz{BizType: "DEBUG", BizID: "debug_biz_" + strconv.FormatInt(now.UnixNano(), 36)},
		OccurredAt:   now,
		Attributes: map[string]any{
			"amount":   "200.00",
			"currency": "USD",
			"template": strings.TrimSpace(template),
		},
	}
}

func (s *Service) findWorkflowExecutionByID(ctx context.Context, executionID string) (marketing.WorkflowExecution, error) {
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: strings.TrimSpace(executionID), Limit: 1})
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	if len(executions) == 0 {
		return marketing.WorkflowExecution{}, marketing.ErrNotFound
	}
	return executions[0], nil
}

func (s *Service) CompareWorkflowExecutions(ctx context.Context, executionID string, baselineExecutionID string) (marketing.WorkflowExecutionCompare, error) {
	return s.CompareWorkflowExecutionsWithOptions(ctx, executionID, baselineExecutionID, marketing.WorkflowExecutionCompareOptions{})
}

func (s *Service) CompareWorkflowExecutionsWithOptions(ctx context.Context, executionID string, baselineExecutionID string, options marketing.WorkflowExecutionCompareOptions) (marketing.WorkflowExecutionCompare, error) {
	executionID = strings.TrimSpace(executionID)
	baselineExecutionID = strings.TrimSpace(baselineExecutionID)
	if executionID == "" || baselineExecutionID == "" {
		return marketing.WorkflowExecutionCompare{}, marketing.ErrNotFound
	}
	options = normalizeWorkflowExecutionCompareOptions(options)
	current, err := s.findWorkflowExecutionByID(ctx, executionID)
	if err != nil {
		return marketing.WorkflowExecutionCompare{}, err
	}
	baseline, err := s.findWorkflowExecutionByID(ctx, baselineExecutionID)
	if err != nil {
		return marketing.WorkflowExecutionCompare{}, err
	}
	currentNodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: current.ID})
	if err != nil {
		return marketing.WorkflowExecutionCompare{}, err
	}
	baselineNodes, err := s.repository.ListWorkflowNodeExecutions(ctx, marketing.WorkflowNodeExecutionQuery{ExecutionID: baseline.ID})
	if err != nil {
		return marketing.WorkflowExecutionCompare{}, err
	}
	currentPath := workflowNodeExecutionPath(currentNodes)
	baselinePath := workflowNodeExecutionPath(baselineNodes)
	currentDuration := workflowNodeExecutionsDurationMs(currentNodes)
	baselineDuration := workflowNodeExecutionsDurationMs(baselineNodes)
	payloadChanges, truncated := workflowNodeExecutionPayloadDiff(currentNodes, baselineNodes, options)
	nodeStatusChanges := workflowNodeExecutionStatusDiff(currentNodes, baselineNodes)
	sideEffectChanges, err := s.workflowExecutionSideEffectDiff(ctx, current, baseline)
	if err != nil {
		return marketing.WorkflowExecutionCompare{}, err
	}
	return marketing.WorkflowExecutionCompare{
		Current:            workflowExecutionCompareSummary(current),
		Baseline:           workflowExecutionCompareSummary(baseline),
		CurrentPath:        currentPath,
		BaselinePath:       baselinePath,
		PathChanged:        !reflect.DeepEqual(currentPath, baselinePath),
		NodeStatusChanges:  nodeStatusChanges,
		SideEffectChanges:  sideEffectChanges,
		CurrentFailures:    workflowNodeExecutionFailures(currentNodes),
		BaselineFailures:   workflowNodeExecutionFailures(baselineNodes),
		CurrentDurationMs:  currentDuration,
		BaselineDurationMs: baselineDuration,
		DurationDeltaMs:    currentDuration - baselineDuration,
		PayloadChanges:     payloadChanges,
		PayloadChangeLimit: options.MaxPayloadChangeNodes,
		JSONDiffLimit:      options.MaxJSONDiffRows,
		Truncated:          truncated,
	}, nil
}

func normalizeWorkflowExecutionCompareOptions(options marketing.WorkflowExecutionCompareOptions) marketing.WorkflowExecutionCompareOptions {
	if options.MaxPayloadChangeNodes <= 0 {
		options.MaxPayloadChangeNodes = 40
	}
	if options.MaxPayloadChangeNodes > 100 {
		options.MaxPayloadChangeNodes = 100
	}
	if options.MaxJSONDiffRows <= 0 {
		options.MaxJSONDiffRows = 24
	}
	if options.MaxJSONDiffRows > 100 {
		options.MaxJSONDiffRows = 100
	}
	return options
}

func workflowExecutionCompareSummary(execution marketing.WorkflowExecution) marketing.WorkflowExecutionCompareSummary {
	return marketing.WorkflowExecutionCompareSummary{
		ID:              execution.ID,
		Status:          execution.Status,
		EventID:         execution.EventID,
		WorkflowVersion: execution.WorkflowVersion,
		ErrorMessage:    execution.ErrorMessage,
		StartedAt:       execution.StartedAt,
		FinishedAt:      execution.FinishedAt,
	}
}

func workflowNodeExecutionPath(nodes []marketing.WorkflowNodeExecution) []string {
	ordered := append([]marketing.WorkflowNodeExecution(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].StartedAt.Before(ordered[j].StartedAt)
	})
	path := make([]string, 0, len(ordered))
	for _, node := range ordered {
		path = append(path, node.NodeID)
	}
	return path
}

func workflowNodeExecutionFailures(nodes []marketing.WorkflowNodeExecution) []string {
	failures := []string{}
	for _, node := range nodes {
		if node.Status != marketing.WorkflowNodeFailed {
			continue
		}
		label := strings.TrimSpace(node.NodeTitle)
		if label == "" {
			label = node.NodeID
		}
		failures = append(failures, label)
	}
	return failures
}

func workflowNodeExecutionsDurationMs(nodes []marketing.WorkflowNodeExecution) int64 {
	var total int64
	for _, node := range nodes {
		if node.FinishedAt == nil || node.FinishedAt.Before(node.StartedAt) {
			continue
		}
		total += node.FinishedAt.Sub(node.StartedAt).Milliseconds()
	}
	return total
}

func workflowNodeExecutionPayloadDiff(current []marketing.WorkflowNodeExecution, baseline []marketing.WorkflowNodeExecution, options marketing.WorkflowExecutionCompareOptions) ([]marketing.WorkflowExecutionPayloadNodeDiff, bool) {
	currentByNode := workflowNodeExecutionByNodeID(current)
	baselineByNode := workflowNodeExecutionByNodeID(baseline)
	nodeIDs := workflowNodeExecutionUnionIDs(current, baseline)
	out := []marketing.WorkflowExecutionPayloadNodeDiff{}
	truncated := false
	for _, nodeID := range nodeIDs {
		node := currentByNode[nodeID]
		base := baselineByNode[nodeID]
		inputDiff := workflowJSONDiff(node.Input, base.Input, "$")
		outputDiff := workflowJSONDiff(node.Output, base.Output, "$")
		if len(inputDiff) == 0 && len(outputDiff) == 0 {
			continue
		}
		if len(out) >= options.MaxPayloadChangeNodes {
			truncated = true
			break
		}
		out = append(out, marketing.WorkflowExecutionPayloadNodeDiff{
			NodeID: nodeID,
			Label:  workflowNodeExecutionLabel(node, base, nodeID),
			Input:  limitWorkflowJSONDiff(inputDiff, options.MaxJSONDiffRows),
			Output: limitWorkflowJSONDiff(outputDiff, options.MaxJSONDiffRows),
		})
	}
	return out, truncated
}

func workflowNodeExecutionStatusDiff(current []marketing.WorkflowNodeExecution, baseline []marketing.WorkflowNodeExecution) []marketing.WorkflowExecutionNodeStatusDiff {
	currentByNode := workflowNodeExecutionByNodeID(current)
	baselineByNode := workflowNodeExecutionByNodeID(baseline)
	nodeIDs := workflowNodeExecutionUnionIDs(current, baseline)
	out := []marketing.WorkflowExecutionNodeStatusDiff{}
	for _, nodeID := range nodeIDs {
		currentNode := currentByNode[nodeID]
		baselineNode := baselineByNode[nodeID]
		currentErrorCode := workflowNodeExecutionOutputString(currentNode.Output, "errorCode")
		baselineErrorCode := workflowNodeExecutionOutputString(baselineNode.Output, "errorCode")
		currentAttempt := workflowNodeExecutionOutputInt(currentNode.Output, "attempt")
		baselineAttempt := workflowNodeExecutionOutputInt(baselineNode.Output, "attempt")
		currentDuration := workflowNodeExecutionDurationMs(currentNode)
		baselineDuration := workflowNodeExecutionDurationMs(baselineNode)
		if currentNode.Status == baselineNode.Status &&
			strings.TrimSpace(currentNode.ErrorMessage) == strings.TrimSpace(baselineNode.ErrorMessage) &&
			currentErrorCode == baselineErrorCode &&
			currentAttempt == baselineAttempt &&
			currentDuration == baselineDuration {
			continue
		}
		out = append(out, marketing.WorkflowExecutionNodeStatusDiff{
			NodeID:             nodeID,
			Label:              workflowNodeExecutionLabel(currentNode, baselineNode, nodeID),
			CurrentStatus:      currentNode.Status,
			BaselineStatus:     baselineNode.Status,
			CurrentError:       strings.TrimSpace(currentNode.ErrorMessage),
			BaselineError:      strings.TrimSpace(baselineNode.ErrorMessage),
			CurrentErrorCode:   currentErrorCode,
			BaselineErrorCode:  baselineErrorCode,
			CurrentAttempt:     currentAttempt,
			BaselineAttempt:    baselineAttempt,
			CurrentDurationMs:  currentDuration,
			BaselineDurationMs: baselineDuration,
		})
	}
	return out
}

func workflowNodeExecutionByNodeID(nodes []marketing.WorkflowNodeExecution) map[string]marketing.WorkflowNodeExecution {
	out := map[string]marketing.WorkflowNodeExecution{}
	for _, node := range nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		if nodeID != "" {
			out[nodeID] = node
		}
	}
	return out
}

func workflowNodeExecutionUnionIDs(current []marketing.WorkflowNodeExecution, baseline []marketing.WorkflowNodeExecution) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, nodes := range [][]marketing.WorkflowNodeExecution{current, baseline} {
		for _, node := range nodes {
			nodeID := strings.TrimSpace(node.NodeID)
			if nodeID == "" {
				continue
			}
			if _, ok := seen[nodeID]; ok {
				continue
			}
			seen[nodeID] = struct{}{}
			out = append(out, nodeID)
		}
	}
	return out
}

func workflowNodeExecutionLabel(current marketing.WorkflowNodeExecution, baseline marketing.WorkflowNodeExecution, fallback string) string {
	for _, value := range []string{current.NodeTitle, baseline.NodeTitle, fallback} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return fallback
}

func workflowNodeExecutionDurationMs(node marketing.WorkflowNodeExecution) int64 {
	if node.FinishedAt == nil || node.FinishedAt.Before(node.StartedAt) || node.StartedAt.IsZero() {
		return 0
	}
	return node.FinishedAt.Sub(node.StartedAt).Milliseconds()
}

func workflowNodeExecutionOutputString(raw json.RawMessage, key string) string {
	output := map[string]any{}
	if len(raw) == 0 || json.Unmarshal(raw, &output) != nil {
		return ""
	}
	value, ok := output[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func workflowNodeExecutionOutputInt(raw json.RawMessage, key string) int {
	output := map[string]any{}
	if len(raw) == 0 || json.Unmarshal(raw, &output) != nil {
		return 0
	}
	switch value := output[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		return 0
	}
}

func (s *Service) workflowExecutionSideEffectDiff(ctx context.Context, current marketing.WorkflowExecution, baseline marketing.WorkflowExecution) ([]marketing.WorkflowExecutionSideEffectDiff, error) {
	currentSummary, err := s.workflowExecutionSideEffectSummary(ctx, current)
	if err != nil {
		return nil, err
	}
	baselineSummary, err := s.workflowExecutionSideEffectSummary(ctx, baseline)
	if err != nil {
		return nil, err
	}
	kinds := []string{"participation", "grantOrders", "outboxMessages", "quotaReservations", "rule"}
	out := []marketing.WorkflowExecutionSideEffectDiff{}
	if workflowExecutionSideEffectScopeShared(current, baseline) {
		scope := map[string]any{
			"scope":      "EVENT_LEVEL",
			"comparable": false,
			"reason":     "business side effects are persisted by tenant/activity/event or idempotency key, not by workflow execution id",
			"eventId":    current.EventID,
			"activityId": current.ActivityID,
			"currentId":  current.ID,
			"baselineId": baseline.ID,
		}
		out = append(out, marketing.WorkflowExecutionSideEffectDiff{
			Kind: "scope",
			Changes: []marketing.WorkflowExecutionJSONDiff{{
				Path:     "$.scope",
				Type:     "changed",
				Current:  scope,
				Baseline: scope,
			}},
			Current:  scope,
			Baseline: scope,
		})
	}
	for _, kind := range kinds {
		currentValue := currentSummary[kind]
		baselineValue := baselineSummary[kind]
		changes := workflowJSONValueDiff(currentValue, baselineValue, true, true, "$")
		if len(changes) == 0 {
			continue
		}
		out = append(out, marketing.WorkflowExecutionSideEffectDiff{
			Kind:     kind,
			Changes:  changes,
			Current:  currentValue,
			Baseline: baselineValue,
		})
	}
	return out, nil
}

func workflowExecutionSideEffectScopeShared(current marketing.WorkflowExecution, baseline marketing.WorkflowExecution) bool {
	if strings.TrimSpace(current.ID) == "" || current.ID == baseline.ID {
		return false
	}
	if current.TenantID != baseline.TenantID || current.ActivityID != baseline.ActivityID {
		return false
	}
	currentEventID := strings.TrimSpace(current.EventID)
	baselineEventID := strings.TrimSpace(baseline.EventID)
	if currentEventID != "" && currentEventID == baselineEventID {
		return true
	}
	currentIdempotencyKey := workflowExecutionInputString(current.Input, "idempotencyKey")
	baselineIdempotencyKey := workflowExecutionInputString(baseline.Input, "idempotencyKey")
	return currentIdempotencyKey != "" && currentIdempotencyKey == baselineIdempotencyKey
}

func (s *Service) workflowExecutionSideEffectSummary(ctx context.Context, execution marketing.WorkflowExecution) (map[string]any, error) {
	items, err := s.workflowExecutionReconciliationItems(ctx, execution)
	if err != nil {
		return nil, err
	}
	participation := map[string]any{"created": false}
	grantOrders := []map[string]any{}
	outboxes := []map[string]any{}
	reservations := []map[string]any{}
	rule := map[string]any{"decision": "", "reason": ""}
	seenGrant := map[string]struct{}{}
	seenOutbox := map[string]struct{}{}
	seenReservation := map[string]struct{}{}
	for _, item := range items {
		if item.Participation != nil {
			participation = workflowParticipationSideEffect(*item.Participation)
		} else if item.ParticipationID != "" && participation["created"] != true {
			participation = map[string]any{
				"created":     true,
				"status":      item.ParticipationStatus,
				"subjectType": item.Subject.Type,
				"subjectId":   item.Subject.ID,
			}
		}
		if item.Participation != nil && item.Participation.RejectReason != "" {
			rule["decision"] = "REJECTED"
			rule["reason"] = item.Participation.RejectReason
		} else if item.ParticipationStatus != "" {
			rule["decision"] = workflowSideEffectDecision(item.ParticipationStatus)
		}
		if item.GrantOrder != nil {
			if appendWorkflowSideEffectUnique(seenGrant, item.GrantOrder.ID) {
				grantOrders = append(grantOrders, workflowGrantOrderSideEffect(*item.GrantOrder))
			}
		} else if item.GrantOrderID != "" && appendWorkflowSideEffectUnique(seenGrant, item.GrantOrderID) {
			grantOrders = append(grantOrders, map[string]any{
				"rewardType":  item.RewardType,
				"amount":      item.RewardAmount,
				"status":      item.GrantStatus,
				"subjectType": item.Subject.Type,
				"subjectId":   item.Subject.ID,
			})
		}
		for _, message := range item.OutboxMessages {
			if appendWorkflowSideEffectUnique(seenOutbox, message.ID) {
				outboxes = append(outboxes, workflowOutboxSideEffect(message))
			}
		}
		for _, reservation := range item.Reservations {
			key := firstWorkflowString(reservation.ID, strings.Join([]string{reservation.GrantOrderID, reservation.CommandID, string(reservation.Type), string(reservation.Scope), reservation.ScopeKey, reservation.Amount}, "\x00"))
			if appendWorkflowSideEffectUnique(seenReservation, key) {
				reservations = append(reservations, workflowReservationSideEffect(reservation))
			}
		}
	}
	sort.SliceStable(grantOrders, func(i, j int) bool {
		return fmt.Sprint(grantOrders[i]) < fmt.Sprint(grantOrders[j])
	})
	sort.SliceStable(outboxes, func(i, j int) bool { return fmt.Sprint(outboxes[i]) < fmt.Sprint(outboxes[j]) })
	sort.SliceStable(reservations, func(i, j int) bool {
		return fmt.Sprint(reservations[i]) < fmt.Sprint(reservations[j])
	})
	return map[string]any{
		"participation": participation,
		"grantOrders": map[string]any{
			"count": len(grantOrders),
			"items": grantOrders,
		},
		"outboxMessages": map[string]any{
			"count": len(outboxes),
			"items": outboxes,
		},
		"quotaReservations": map[string]any{
			"count": len(reservations),
			"items": reservations,
		},
		"rule": rule,
	}, nil
}

func (s *Service) workflowExecutionReconciliationItems(ctx context.Context, execution marketing.WorkflowExecution) ([]marketing.RewardReconciliationItem, error) {
	query := marketing.RewardReconciliationQuery{
		TenantID:   execution.TenantID,
		ActivityID: execution.ActivityID,
		EventID:    execution.EventID,
		Limit:      50,
	}
	items, err := s.repository.ListRewardReconciliation(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 || strings.TrimSpace(execution.EventID) != "" {
		return items, nil
	}
	idempotencyKey := workflowExecutionInputString(execution.Input, "idempotencyKey")
	if strings.TrimSpace(idempotencyKey) == "" {
		return items, nil
	}
	return s.repository.ListRewardReconciliation(ctx, marketing.RewardReconciliationQuery{
		TenantID:       execution.TenantID,
		ActivityID:     execution.ActivityID,
		IdempotencyKey: idempotencyKey,
		Limit:          50,
	})
}

func workflowParticipationSideEffect(participation marketing.Participation) map[string]any {
	return map[string]any{
		"created":      participation.ID != "",
		"status":       participation.Status,
		"rejectReason": participation.RejectReason,
		"subjectType":  participation.Subject.Type,
		"subjectId":    participation.Subject.ID,
		"bizType":      participation.Biz.BizType,
		"bizId":        participation.Biz.BizID,
	}
}

func workflowGrantOrderSideEffect(order marketing.GrantOrder) map[string]any {
	return map[string]any{
		"rewardType":   order.RewardType,
		"amount":       order.RewardAmount,
		"status":       order.GrantStatus,
		"channel":      order.GrantChannel,
		"retryCount":   order.RetryCount,
		"errorMessage": order.ErrorMessage,
		"subjectType":  order.Subject.Type,
		"subjectId":    order.Subject.ID,
	}
}

func workflowOutboxSideEffect(message marketing.OutboxMessage) map[string]any {
	return map[string]any{
		"messageType":   message.MessageType,
		"targetChannel": message.TargetChannel,
		"status":        message.SendStatus,
		"retryCount":    message.RetryCount,
		"errorMessage":  message.ErrorMessage,
	}
}

func workflowReservationSideEffect(reservation marketing.QuotaReservation) map[string]any {
	return map[string]any{
		"quotaType": reservation.Type,
		"scope":     reservation.Scope,
		"scopeKey":  reservation.ScopeKey,
		"amount":    reservation.Amount,
		"status":    reservation.Status,
	}
}

func workflowSideEffectDecision(status marketing.ParticipationStatus) string {
	switch status {
	case marketing.ParticipationQualified, marketing.ParticipationRewardable, marketing.ParticipationGranted:
		return "PASSED"
	case marketing.ParticipationRejected, marketing.ParticipationFailed:
		return "REJECTED"
	default:
		return string(status)
	}
}

func appendWorkflowSideEffectUnique(seen map[string]struct{}, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	return true
}

func limitWorkflowJSONDiff(rows []marketing.WorkflowExecutionJSONDiff, limit int) []marketing.WorkflowExecutionJSONDiff {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func workflowJSONDiff(current json.RawMessage, baseline json.RawMessage, path string) []marketing.WorkflowExecutionJSONDiff {
	var currentValue any
	var baselineValue any
	currentPresent := len(current) > 0 && strings.TrimSpace(string(current)) != "null"
	baselinePresent := len(baseline) > 0 && strings.TrimSpace(string(baseline)) != "null"
	if currentPresent {
		_ = json.Unmarshal(current, &currentValue)
	}
	if baselinePresent {
		_ = json.Unmarshal(baseline, &baselineValue)
	}
	return workflowJSONValueDiff(currentValue, baselineValue, currentPresent, baselinePresent, path)
}

func workflowJSONValueDiff(current any, baseline any, currentPresent bool, baselinePresent bool, path string) []marketing.WorkflowExecutionJSONDiff {
	if currentPresent && baselinePresent && reflect.DeepEqual(current, baseline) {
		return nil
	}
	if !currentPresent && baselinePresent {
		return []marketing.WorkflowExecutionJSONDiff{{Path: path, Type: "removed", Baseline: baseline}}
	}
	if currentPresent && !baselinePresent {
		return []marketing.WorkflowExecutionJSONDiff{{Path: path, Type: "added", Current: current}}
	}
	currentMap, currentIsMap := current.(map[string]any)
	baselineMap, baselineIsMap := baseline.(map[string]any)
	if currentIsMap && baselineIsMap {
		keys := map[string]struct{}{}
		for key := range currentMap {
			keys[key] = struct{}{}
		}
		for key := range baselineMap {
			keys[key] = struct{}{}
		}
		orderedKeys := make([]string, 0, len(keys))
		for key := range keys {
			orderedKeys = append(orderedKeys, key)
		}
		sort.Strings(orderedKeys)
		out := []marketing.WorkflowExecutionJSONDiff{}
		for _, key := range orderedKeys {
			currentChild, currentHas := currentMap[key]
			baselineChild, baselineHas := baselineMap[key]
			out = append(out, workflowJSONValueDiff(currentChild, baselineChild, currentHas, baselineHas, workflowJSONChildPath(path, key))...)
		}
		return out
	}
	currentSlice, currentIsSlice := current.([]any)
	baselineSlice, baselineIsSlice := baseline.([]any)
	if currentIsSlice && baselineIsSlice {
		length := len(currentSlice)
		if len(baselineSlice) > length {
			length = len(baselineSlice)
		}
		out := []marketing.WorkflowExecutionJSONDiff{}
		for index := 0; index < length; index++ {
			currentHas := index < len(currentSlice)
			baselineHas := index < len(baselineSlice)
			var currentChild any
			var baselineChild any
			if currentHas {
				currentChild = currentSlice[index]
			}
			if baselineHas {
				baselineChild = baselineSlice[index]
			}
			out = append(out, workflowJSONValueDiff(currentChild, baselineChild, currentHas, baselineHas, fmt.Sprintf("%s[%d]", path, index))...)
		}
		return out
	}
	return []marketing.WorkflowExecutionJSONDiff{{Path: path, Type: "changed", Current: current, Baseline: baseline}}
}

func workflowJSONChildPath(path string, key string) string {
	if path == "$" {
		return "$." + key
	}
	return path + "." + key
}

func workflowEventFromExecutionInput(raw json.RawMessage) (marketing.ActivityEvent, error) {
	input := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &input); err != nil {
		return marketing.ActivityEvent{}, err
	}
	var event marketing.ActivityEvent
	if err := json.Unmarshal(input["event"], &event); err != nil {
		return marketing.ActivityEvent{}, err
	}
	return event, nil
}

func workflowTraceRoute(raw json.RawMessage) any {
	output := map[string]any{}
	_ = json.Unmarshal(raw, &output)
	return output["routeTrace"]
}

func (s *Service) ListWorkflowNodeConfigSchemas() []marketing.WorkflowNodeConfigSchema {
	return marketing.DefaultWorkflowNodeConfigSchemas().List()
}

func workflowValidationMessage(validation marketing.WorkflowValidationResult) string {
	for _, item := range validation.Items {
		if item.Severity == marketing.WorkflowValidationBlock {
			return "workflow validation failed: " + item.Code
		}
	}
	return "workflow validation failed"
}

func marshalWorkflowPayload(value any) json.RawMessage {
	bytes, _ := json.Marshal(marketing.RedactSensitive(value))
	return bytes
}
