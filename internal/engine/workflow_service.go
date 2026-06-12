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
	resumed, err := s.executeWorkflowDefinition(ctx, workflowRuntime{
		Inbox:    marketing.EventInbox{ID: execution.EventInboxID, Event: event, IdempotencyKey: workflowExecutionInputString(execution.Input, "idempotencyKey")},
		Activity: activity,
	}, workflowExecutionOptions{
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
		WorkflowVersionID:  "debug-draft",
		WorkflowVersion:    0,
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
	return marketing.WorkflowExecutionCompare{
		Current:            workflowExecutionCompareSummary(current),
		Baseline:           workflowExecutionCompareSummary(baseline),
		CurrentPath:        currentPath,
		BaselinePath:       baselinePath,
		PathChanged:        !reflect.DeepEqual(currentPath, baselinePath),
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
	baselineByNode := map[string]marketing.WorkflowNodeExecution{}
	for _, node := range baseline {
		baselineByNode[node.NodeID] = node
	}
	out := []marketing.WorkflowExecutionPayloadNodeDiff{}
	truncated := false
	for _, node := range current {
		base := baselineByNode[node.NodeID]
		inputDiff := workflowJSONDiff(node.Input, base.Input, "$")
		outputDiff := workflowJSONDiff(node.Output, base.Output, "$")
		if len(inputDiff) == 0 && len(outputDiff) == 0 {
			continue
		}
		label := strings.TrimSpace(node.NodeTitle)
		if label == "" {
			label = node.NodeID
		}
		if len(out) >= options.MaxPayloadChangeNodes {
			truncated = true
			break
		}
		out = append(out, marketing.WorkflowExecutionPayloadNodeDiff{
			NodeID: node.NodeID,
			Label:  label,
			Input:  limitWorkflowJSONDiff(inputDiff, options.MaxJSONDiffRows),
			Output: limitWorkflowJSONDiff(outputDiff, options.MaxJSONDiffRows),
		})
	}
	return out, truncated
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
