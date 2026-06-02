package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type workflowRuntime struct {
	Inbox          marketing.EventInbox
	Activity       marketing.Activity
	Participation  marketing.Participation
	GrantOrders    []marketing.GrantOrder
	OutboxMessages []marketing.OutboxMessage
	RuleResult     marketing.ActivityRuleResult
	Qualified      bool
	Reason         string
}

type workflowNodeResult struct {
	Status marketing.WorkflowNodeExecutionStatus
	Output map[string]any
	Error  string
}

type workflowNodeHandler func(context.Context, marketing.WorkflowNode, workflowRuntime) workflowNodeResult

func (s *Service) executeActivityWorkflow(ctx context.Context, runtime workflowRuntime) error {
	version, err := s.repository.GetActiveWorkflowVersion(ctx, runtime.Activity.ID)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return nil
		}
		return err
	}
	definition, err := marketing.ParseWorkflowDefinition(version.Definition)
	if err != nil {
		return err
	}
	now := s.now()
	execution := marketing.WorkflowExecution{
		ID:                marketingID("wfexec"),
		TenantID:          runtime.Activity.TenantID,
		ActivityID:        runtime.Activity.ID,
		WorkflowVersionID: version.ID,
		WorkflowVersion:   version.Version,
		EventInboxID:      runtime.Inbox.ID,
		EventID:           runtime.Inbox.Event.EventID,
		Status:            marketing.WorkflowExecutionRunning,
		Input: marshalWorkflowPayload(map[string]any{
			"event":      runtime.Inbox.Event,
			"activityId": runtime.Activity.ID,
		}),
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	execution, err = s.repository.CreateWorkflowExecution(ctx, execution)
	if err != nil {
		return err
	}

	handlers := s.workflowNodeHandlers()
	status := marketing.WorkflowExecutionSuccess
	var errorMessage string
	executed := 0
	skipped := 0
	failed := 0
	for _, node := range workflowExecutionOrder(definition) {
		handler := handlers[nodeHandlerKey(node)]
		if handler == nil {
			handler = genericWorkflowNodeHandler
		}
		nodeStartedAt := s.now()
		nodeExecution := marketing.WorkflowNodeExecution{
			ID:          marketingID("wfnode"),
			TenantID:    runtime.Activity.TenantID,
			ExecutionID: execution.ID,
			NodeID:      node.ID,
			NodeType:    nodeHandlerKey(node),
			NodeTitle:   node.Title,
			Status:      marketing.WorkflowNodeRunning,
			Input: marshalWorkflowPayload(map[string]any{
				"eventId":         runtime.Inbox.Event.EventID,
				"activityId":      runtime.Activity.ID,
				"participationId": runtime.Participation.ID,
				"qualified":       runtime.Qualified,
			}),
			StartedAt: nodeStartedAt,
			CreatedAt: nodeStartedAt,
			UpdatedAt: nodeStartedAt,
		}
		nodeExecution, err = s.repository.CreateWorkflowNodeExecution(ctx, nodeExecution)
		if err != nil {
			return err
		}
		result := handler(ctx, node, runtime)
		finishedAt := s.now()
		nodeExecution.Status = result.Status
		nodeExecution.Output = marshalWorkflowPayload(result.Output)
		nodeExecution.ErrorMessage = result.Error
		nodeExecution.FinishedAt = &finishedAt
		nodeExecution.UpdatedAt = finishedAt
		if err := s.repository.UpdateWorkflowNodeExecution(ctx, nodeExecution); err != nil {
			return err
		}
		executed++
		switch result.Status {
		case marketing.WorkflowNodeSkipped:
			skipped++
		case marketing.WorkflowNodeFailed:
			failed++
			status = marketing.WorkflowExecutionFailed
			if errorMessage == "" {
				errorMessage = result.Error
			}
		}
	}
	finishedAt := s.now()
	execution.Status = status
	execution.ErrorMessage = errorMessage
	execution.Output = marshalWorkflowPayload(map[string]any{
		"executed":        executed,
		"skipped":         skipped,
		"failed":          failed,
		"qualified":       runtime.Qualified,
		"ruleDecision":    runtime.RuleResult.Decision,
		"participationId": runtime.Participation.ID,
		"grantOrderCount": len(runtime.GrantOrders),
		"outboxCount":     len(runtime.OutboxMessages),
	})
	execution.FinishedAt = &finishedAt
	execution.UpdatedAt = finishedAt
	if err := s.repository.UpdateWorkflowExecution(ctx, execution); err != nil {
		return err
	}
	return nil
}

func (s *Service) workflowNodeHandlers() map[string]workflowNodeHandler {
	return map[string]workflowNodeHandler{
		"event-in":      entryWorkflowNodeHandler,
		"inbox":         inboxWorkflowNodeHandler,
		"normalize":     normalizeWorkflowNodeHandler,
		"match":         matchWorkflowNodeHandler,
		"fact-query":    factQueryWorkflowNodeHandler,
		"condition":     decisionWorkflowNodeHandler,
		"qualify":       decisionWorkflowNodeHandler,
		"switch":        decisionWorkflowNodeHandler,
		"reward":        rewardWorkflowNodeHandler,
		"decimal":       decimalWorkflowNodeHandler,
		"limit":         limitWorkflowNodeHandler,
		"quota":         quotaWorkflowNodeHandler,
		"participation": participationWorkflowNodeHandler,
		"audit":         auditWorkflowNodeHandler,
		"grant-order":   grantOrderWorkflowNodeHandler,
		"outbox":        outboxWorkflowNodeHandler,
		"retry":         retryWorkflowNodeHandler,
		"dlq":           dlqWorkflowNodeHandler,
		"http-call":     dispatchWorkflowNodeHandler("HTTP"),
		"mq-publish":    dispatchWorkflowNodeHandler("MQ"),
		"webhook":       dispatchWorkflowNodeHandler("WEBHOOK"),
		"callback":      callbackWorkflowNodeHandler,
		"reconcile":     reconcileWorkflowNodeHandler,
		"alert":         alertWorkflowNodeHandler,
	}
}

func workflowExecutionOrder(definition marketing.WorkflowDefinition) []marketing.WorkflowNode {
	if len(definition.Nodes) == 0 {
		return nil
	}
	nodeByID := map[string]marketing.WorkflowNode{}
	edgesBySource := map[string][]marketing.WorkflowEdge{}
	for _, node := range definition.Nodes {
		nodeByID[node.ID] = node
	}
	for _, edge := range definition.Edges {
		edgesBySource[edge.Source] = append(edgesBySource[edge.Source], edge)
	}
	var roots []marketing.WorkflowNode
	for _, node := range definition.Nodes {
		if node.Type == "event-in" || node.Kind == "entry" {
			roots = append(roots, node)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, definition.Nodes[0])
	}
	seen := map[string]struct{}{}
	var out []marketing.WorkflowNode
	var visit func(marketing.WorkflowNode)
	visit = func(node marketing.WorkflowNode) {
		if _, ok := seen[node.ID]; ok {
			return
		}
		seen[node.ID] = struct{}{}
		out = append(out, node)
		for _, edge := range edgesBySource[node.ID] {
			if target, ok := nodeByID[edge.Target]; ok {
				visit(target)
			}
		}
	}
	for _, root := range roots {
		visit(root)
	}
	for _, node := range definition.Nodes {
		visit(node)
	}
	return out
}

func nodeHandlerKey(node marketing.WorkflowNode) string {
	for _, value := range []string{node.Type, node.Code, node.Kind} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func nodeSuccess(output map[string]any) workflowNodeResult {
	return workflowNodeResult{Status: marketing.WorkflowNodeSuccess, Output: output}
}

func nodeSkipped(reason string, output map[string]any) workflowNodeResult {
	if output == nil {
		output = map[string]any{}
	}
	output["skipReason"] = reason
	return workflowNodeResult{Status: marketing.WorkflowNodeSkipped, Output: output}
}

func nodeFailed(message string, output map[string]any) workflowNodeResult {
	return workflowNodeResult{Status: marketing.WorkflowNodeFailed, Error: message, Output: output}
}

func entryWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{"eventId": runtime.Inbox.Event.EventID, "eventType": runtime.Inbox.Event.EventType, "tenantId": runtime.Inbox.Event.TenantID})
}

func inboxWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{"inboxId": runtime.Inbox.ID, "status": runtime.Inbox.ProcessStatus, "idempotencyKey": runtime.Inbox.IdempotencyKey})
}

func normalizeWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{"normalized": true, "eventId": runtime.Inbox.Event.EventID})
}

func matchWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{"activityId": runtime.Activity.ID, "activityCode": runtime.Activity.Code, "matched": true})
}

func factQueryWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	refs := marketing.ExtractFactReferences(runtime.Activity.Rules)
	if len(refs) == 0 {
		return nodeSkipped("no fact references", map[string]any{"factCount": 0})
	}
	return nodeSuccess(map[string]any{"factCount": len(refs), "facts": refs})
}

func decisionWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{
		"nodeType":   nodeHandlerKey(node),
		"decision":   runtime.RuleResult.Decision,
		"qualified":  runtime.Qualified,
		"reasonCode": runtime.RuleResult.ReasonCode,
		"reason":     runtime.RuleResult.Reason,
	})
}

func rewardWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if !runtime.Qualified {
		return nodeSkipped("activity not qualified", nil)
	}
	if len(runtime.Activity.Rewards) == 0 {
		return nodeSkipped("no rewards configured", nil)
	}
	return nodeSuccess(map[string]any{"configuredRewardCount": len(runtime.Activity.Rewards), "grantOrderCount": len(runtime.GrantOrders)})
}

func decimalWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if !runtime.Qualified {
		return nodeSkipped("activity not qualified", nil)
	}
	return nodeSuccess(map[string]any{"mode": "Decimal(18, 8)"})
}

func limitWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Qualified {
		return nodeSkipped("risk branch not selected", nil)
	}
	return nodeSuccess(map[string]any{"decision": runtime.RuleResult.Decision, "reason": runtime.RuleResult.Reason})
}

func quotaWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if !runtime.Qualified {
		return nodeSkipped("activity not qualified", nil)
	}
	return nodeSuccess(map[string]any{"status": "reserved-or-unlimited"})
}

func participationWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Participation.ID == "" {
		return nodeFailed("participation was not created", nil)
	}
	return nodeSuccess(map[string]any{"participationId": runtime.Participation.ID, "status": runtime.Participation.Status})
}

func auditWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSuccess(map[string]any{"eventId": runtime.Inbox.Event.EventID, "activityId": runtime.Activity.ID})
}

func grantOrderWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("no grant orders created", nil)
	}
	return nodeSuccess(map[string]any{"grantOrderCount": len(runtime.GrantOrders)})
}

func outboxWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.OutboxMessages) == 0 {
		return nodeSkipped("no outbox messages created", nil)
	}
	return nodeSuccess(map[string]any{"outboxCount": len(runtime.OutboxMessages)})
}

func retryWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.OutboxMessages) == 0 {
		return nodeSkipped("no dispatch retry required", nil)
	}
	return nodeSuccess(map[string]any{"retryPolicy": "outbox exponential backoff"})
}

func dlqWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSkipped("no exhausted retry in current execution", nil)
}

func dispatchWorkflowNodeHandler(channel string) workflowNodeHandler {
	return func(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		count := 0
		for _, message := range runtime.OutboxMessages {
			if strings.EqualFold(string(message.TargetChannel), channel) {
				count++
			}
		}
		if count == 0 {
			return nodeSkipped("no "+channel+" dispatch", map[string]any{"channel": channel})
		}
		return nodeSuccess(map[string]any{"channel": channel, "messageCount": count})
	}
}

func callbackWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("no grant callback expected", nil)
	}
	return nodeSuccess(map[string]any{"awaitingCallback": true, "grantOrderCount": len(runtime.GrantOrders)})
}

func reconcileWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("no grant order to reconcile", nil)
	}
	return nodeSuccess(map[string]any{"reconciliationTracked": true})
}

func alertWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Qualified {
		return nodeSkipped("no alert condition", nil)
	}
	return nodeSuccess(map[string]any{"reason": runtime.RuleResult.Reason})
}

func genericWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, _ workflowRuntime) workflowNodeResult {
	rawConfig := map[string]any{}
	if len(node.Config) > 0 {
		_ = json.Unmarshal(node.Config, &rawConfig)
	}
	return nodeSuccess(map[string]any{"nodeType": nodeHandlerKey(node), "config": rawConfig, "executedAt": time.Now().UTC()})
}
