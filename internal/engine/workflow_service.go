package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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
	definition, err := marketing.NormalizeWorkflowDefinition(cmd.Definition)
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
		CreatedAt:   now,
	}
	created, err := s.repository.CreateWorkflowVersion(ctx, version)
	if err != nil {
		return marketing.WorkflowVersion{}, err
	}
	_ = s.audit(ctx, created.TenantID, actorOrSystem(cmd.Operator), "PUBLISH_WORKFLOW_VERSION", "WORKFLOW_VERSION", created.ID, auditSnapshot(nil, created, map[string]any{
		"activityId": created.ActivityID,
		"version":    created.Version,
	}))
	return created, nil
}

func (s *Service) ListWorkflowVersions(ctx context.Context, activityID string) ([]marketing.WorkflowVersion, error) {
	return s.repository.ListWorkflowVersions(ctx, strings.TrimSpace(activityID))
}

func (s *Service) ListWorkflowExecutions(ctx context.Context, query marketing.WorkflowExecutionQuery) ([]marketing.WorkflowExecution, error) {
	query.ActivityID = strings.TrimSpace(query.ActivityID)
	query.TenantID = strings.TrimSpace(query.TenantID)
	query.EventID = strings.TrimSpace(query.EventID)
	query.ExecutionID = strings.TrimSpace(query.ExecutionID)
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	return s.repository.ListWorkflowExecutions(ctx, query)
}

func (s *Service) ListWorkflowNodeExecutions(ctx context.Context, executionID string) ([]marketing.WorkflowNodeExecution, error) {
	executionID = strings.TrimSpace(executionID)
	if executionID == "" {
		return nil, marketing.ErrNotFound
	}
	return s.repository.ListWorkflowNodeExecutions(ctx, executionID)
}

func marshalWorkflowPayload(value any) json.RawMessage {
	bytes, _ := json.Marshal(marketing.RedactSensitive(value))
	return bytes
}
