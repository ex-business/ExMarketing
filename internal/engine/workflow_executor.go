package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	exdecimal "github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	rewardcalc "github.com/ex-stack/ExMarketing/internal/reward"
	"github.com/ex-stack/ExMarketing/internal/rule"
)

type workflowRuntime struct {
	Inbox          marketing.EventInbox
	Activity       marketing.Activity
	Participation  marketing.Participation
	GrantOrders    []marketing.GrantOrder
	OutboxMessages []marketing.OutboxMessage
	Reservations   []marketing.QuotaReservation
	RuleResult     marketing.ActivityRuleResult
	Qualified      bool
	Reason         string
}

type workflowNodeResult struct {
	Status marketing.WorkflowNodeExecutionStatus
	Output map[string]any
	Error  string
}

type workflowNodeExecutionContext struct {
	ResolvedInput   map[string]any
	NodeOutputs     map[string]map[string]any
	NodeDefinitions map[string]marketing.WorkflowNode
	Variables       map[string]any
}

type workflowNodeExecutionContextKey struct{}

type workflowNodeHandler func(context.Context, marketing.WorkflowNode, workflowRuntime) workflowNodeResult

type workflowExecutionPlanData struct {
	NodeByID      map[string]marketing.WorkflowNode
	EdgesBySource map[string][]marketing.WorkflowEdge
	Incoming      map[string][]marketing.WorkflowEdge
	Roots         []marketing.WorkflowNode
	TopoIndex     map[string]int
	TopoLevels    map[string]int
}

type workflowNodeExecutionPolicy struct {
	Timeout          time.Duration
	MaxRetries       int
	RetryBackoff     time.Duration
	ConcurrencyGroup string
	ConcurrencyLimit int
}

type workflowPlanCompileError struct {
	Issues []marketing.WorkflowValidationIssue
}

func (err workflowPlanCompileError) Error() string {
	if len(err.Issues) == 0 {
		return "workflow cannot compile executor plan"
	}
	return err.Issues[0].Message
}

type workflowExecutionOptions struct {
	WorkflowVersionID  string
	WorkflowVersion    int
	Definition         json.RawMessage
	Mode               string
	IdempotencyKey     string
	SkipIdempotency    bool
	StartNodeID        string
	SourceExecutionID  string
	InitialNodeOutputs map[string]map[string]any
	InitialVariables   map[string]any
}

func (s *Service) executeActivityWorkflow(ctx context.Context, runtime workflowRuntime) error {
	version, err := s.repository.GetActiveWorkflowVersion(ctx, runtime.Activity.ID)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return nil
		}
		return err
	}
	idempotencyKey := workflowExecutionIdempotencyKey(runtime.Activity.ID, version.ID, runtime.Inbox.IdempotencyKey)
	_, err = s.executeWorkflowDefinition(ctx, runtime, workflowExecutionOptions{
		WorkflowVersionID: version.ID,
		WorkflowVersion:   version.Version,
		Definition:        version.Definition,
		Mode:              "PRODUCTION",
		IdempotencyKey:    idempotencyKey,
	})
	return err
}

func (s *Service) executeWorkflowDefinition(ctx context.Context, runtime workflowRuntime, opts workflowExecutionOptions) (marketing.WorkflowExecution, error) {
	definition, err := marketing.ParseWorkflowDefinition(opts.Definition)
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	if strings.TrimSpace(opts.Mode) == "" {
		opts.Mode = "PRODUCTION"
	}
	if !opts.SkipIdempotency {
		existing, err := s.findExistingWorkflowExecution(ctx, runtime.Activity.ID, runtime.Inbox.Event.EventID, opts.WorkflowVersionID, opts.IdempotencyKey)
		if err != nil {
			return marketing.WorkflowExecution{}, err
		}
		if existing.ID != "" && existing.Status != marketing.WorkflowExecutionFailed && existing.Status != marketing.WorkflowExecutionCanceled {
			return existing, nil
		}
	}
	now := s.now()
	execution := marketing.WorkflowExecution{
		ID:                marketingID("wfexec"),
		TenantID:          runtime.Activity.TenantID,
		ActivityID:        runtime.Activity.ID,
		WorkflowVersionID: opts.WorkflowVersionID,
		WorkflowVersion:   opts.WorkflowVersion,
		EventInboxID:      runtime.Inbox.ID,
		EventID:           runtime.Inbox.Event.EventID,
		Status:            marketing.WorkflowExecutionRunning,
		Input: marshalWorkflowPayload(map[string]any{
			"event":             runtime.Inbox.Event,
			"activityId":        runtime.Activity.ID,
			"idempotencyKey":    opts.IdempotencyKey,
			"mode":              strings.ToUpper(opts.Mode),
			"debug":             !strings.EqualFold(opts.Mode, "PRODUCTION"),
			"startNodeId":       opts.StartNodeID,
			"sourceExecutionId": opts.SourceExecutionID,
		}),
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	execution, err = s.repository.CreateWorkflowExecution(ctx, execution)
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}

	handlers := s.workflowNodeHandlers()
	status := marketing.WorkflowExecutionSuccess
	var errorMessage string
	executed := 0
	skipped := 0
	failed := 0
	nodeOutputs := cloneWorkflowNodeOutputs(opts.InitialNodeOutputs)
	workflowVars := cloneWorkflowVars(opts.InitialVariables)
	routeTrace := []map[string]any{}
	compileIssues := []marketing.WorkflowValidationIssue{}
	plan, err := workflowExecutionPlan(definition)
	if err != nil {
		status = marketing.WorkflowExecutionFailed
		errorMessage = err.Error()
		var compileErr workflowPlanCompileError
		if errors.As(err, &compileErr) {
			compileIssues = compileErr.Issues
		}
	}
	queue := append([]marketing.WorkflowNode(nil), plan.Roots...)
	if opts.StartNodeID != "" {
		if startNode, ok := plan.NodeByID[opts.StartNodeID]; ok {
			queue = []marketing.WorkflowNode{startNode}
		} else if err == nil {
			status = marketing.WorkflowExecutionFailed
			errorMessage = "debug start node not found: " + opts.StartNodeID
			queue = nil
		}
	}
	queued := map[string]bool{}
	for _, node := range queue {
		queued[node.ID] = true
	}
	seen := map[string]bool{}
	for err == nil && len(queue) > 0 {
		if ctxErr := ctx.Err(); ctxErr != nil {
			status = marketing.WorkflowExecutionCanceled
			errorMessage = ctxErr.Error()
			break
		}
		canceled, cancelReason, cancelErr := s.workflowExecutionCancelState(ctx, execution.ID)
		if cancelErr != nil {
			return marketing.WorkflowExecution{}, cancelErr
		}
		if canceled {
			status = marketing.WorkflowExecutionCanceled
			errorMessage = cancelReason
			break
		}
		node := queue[0]
		queue = queue[1:]
		queued[node.ID] = false
		if seen[node.ID] {
			continue
		}
		seen[node.ID] = true
		handler := handlers[nodeHandlerKey(node)]
		if handler == nil {
			handler = genericWorkflowNodeHandler
		}
		resolvedInput, inputErr := resolveWorkflowNodeInputs(node, runtime, nodeOutputs)
		workflowApplyNodeContextConfig(workflowVars, node)
		nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
			ResolvedInput:   resolvedInput,
			NodeOutputs:     cloneWorkflowNodeOutputs(nodeOutputs),
			NodeDefinitions: plan.NodeByID,
			Variables:       workflowVars,
		})
		resolvedNode := node
		resolvedConfig := workflowNodeConfigMap(node)
		var fieldBindingTrace []map[string]any
		var configErr error
		if inputErr == nil {
			resolvedNode, resolvedConfig, fieldBindingTrace, configErr = s.resolveWorkflowNodeConfig(nodeCtx, node, runtime)
			if len(fieldBindingTrace) > 0 {
				resolvedInput["fieldBindingTrace"] = fieldBindingTrace
				resolvedInput["resolvedConfig"] = workflowJSONValue(resolvedConfig)
			}
		}
		nodeStartedAt := s.now()
		nodeExecution := marketing.WorkflowNodeExecution{
			ID:          marketingID("wfnode"),
			TenantID:    runtime.Activity.TenantID,
			ExecutionID: execution.ID,
			NodeID:      resolvedNode.ID,
			NodeType:    nodeHandlerKey(resolvedNode),
			NodeTitle:   resolvedNode.Title,
			Status:      marketing.WorkflowNodeRunning,
			Input:       marshalWorkflowPayload(resolvedInput),
			StartedAt:   nodeStartedAt,
			CreatedAt:   nodeStartedAt,
			UpdatedAt:   nodeStartedAt,
		}
		nodeExecution, err = s.repository.CreateWorkflowNodeExecution(ctx, nodeExecution)
		if err != nil {
			return marketing.WorkflowExecution{}, err
		}
		var result workflowNodeResult
		if inputErr != nil {
			result = nodeFailed(inputErr.Error(), map[string]any{"inputMappingError": inputErr.Error()})
		} else if configErr != nil {
			result = nodeFailed(configErr.Error(), map[string]any{
				"fieldBindingError": configErr.Error(),
				"fieldBindingTrace": fieldBindingTrace,
			})
		} else {
			nodeCtx = context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
				ResolvedInput:   resolvedInput,
				NodeOutputs:     cloneWorkflowNodeOutputs(nodeOutputs),
				NodeDefinitions: plan.NodeByID,
				Variables:       workflowVars,
			})
			result = executeWorkflowNodeWithPolicy(nodeCtx, resolvedNode, runtime, handler)
		}
		finishedAt := s.now()
		nodeExecution.Status = result.Status
		nodeExecution.Output = marshalWorkflowPayload(result.Output)
		nodeExecution.ErrorMessage = result.Error
		nodeExecution.FinishedAt = &finishedAt
		nodeExecution.UpdatedAt = finishedAt
		if err := s.repository.UpdateWorkflowNodeExecution(ctx, nodeExecution); err != nil {
			return marketing.WorkflowExecution{}, err
		}
		nodeOutputs[node.ID] = result.Output
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
		if result.Status == marketing.WorkflowNodeFailed {
			continue
		}
		nextEdges := workflowNextEdges(node, result, plan.EdgesBySource[node.ID])
		for _, edge := range nextEdges {
			routeTrace = append(routeTrace, workflowExecutionRouteTraceItem(edge, result))
			target, ok := plan.NodeByID[edge.Target]
			if !ok || seen[target.ID] || queued[target.ID] {
				continue
			}
			queue = workflowEnqueueByTopology(queue, target, plan.TopoIndex)
			queued[target.ID] = true
		}
	}
	if err == nil {
		missing := 0
		for _, node := range definition.Nodes {
			if seen[node.ID] {
				continue
			}
			if createErr := s.createSkippedWorkflowNodeExecution(ctx, runtime, execution.ID, node, "not reached by selected route or upstream failure"); createErr != nil {
				return marketing.WorkflowExecution{}, createErr
			}
			missing++
		}
		skipped += missing
	}
	if err == nil {
		canceled, cancelReason, cancelErr := s.workflowExecutionCancelState(ctx, execution.ID)
		if cancelErr != nil {
			return marketing.WorkflowExecution{}, cancelErr
		}
		if canceled {
			status = marketing.WorkflowExecutionCanceled
			errorMessage = cancelReason
		}
	}
	if status == marketing.WorkflowExecutionSuccess && workflowRuntimeBusinessRejected(runtime) {
		status = marketing.WorkflowExecutionFailed
		if errorMessage == "" {
			errorMessage = workflowRuntimeBusinessRejectedMessage(runtime)
		}
	}
	finishedAt := s.now()
	execution.Status = status
	execution.ErrorMessage = errorMessage
	execution.Output = marshalWorkflowPayload(map[string]any{
		"executed":         executed,
		"skipped":          skipped,
		"failed":           failed,
		"qualified":        runtime.Qualified,
		"ruleDecision":     runtime.RuleResult.Decision,
		"participationId":  runtime.Participation.ID,
		"grantOrderCount":  len(runtime.GrantOrders),
		"outboxCount":      len(runtime.OutboxMessages),
		"reservationCount": len(runtime.Reservations),
		"routeTrace":       routeTrace,
		"compileIssues":    compileIssues,
		"mode":             strings.ToUpper(opts.Mode),
		"debug":            !strings.EqualFold(opts.Mode, "PRODUCTION"),
		"startNodeId":      opts.StartNodeID,
		"variables":        cloneWorkflowVars(workflowVars),
	})
	execution.FinishedAt = &finishedAt
	execution.UpdatedAt = finishedAt
	if err := s.repository.UpdateWorkflowExecution(ctx, execution); err != nil {
		return marketing.WorkflowExecution{}, err
	}
	return execution, nil
}

func workflowRuntimeBusinessRejected(runtime workflowRuntime) bool {
	return runtime.Participation.Status == marketing.ParticipationRejected ||
		runtime.RuleResult.Decision == marketing.RuleDecisionRejected
}

func workflowRuntimeBusinessRejectedMessage(runtime workflowRuntime) string {
	reason := strings.TrimSpace(runtime.RuleResult.Reason)
	if reason == "" {
		reason = strings.TrimSpace(runtime.Participation.RejectReason)
	}
	if reason == "" {
		reason = "business rejected"
	}
	code := strings.TrimSpace(runtime.RuleResult.ReasonCode)
	if code == "" {
		code = "BUSINESS_REJECTED"
	}
	return code + ": " + reason
}

func (s *Service) workflowNodeHandlers() map[string]workflowNodeHandler {
	return map[string]workflowNodeHandler{
		"event-in":          entryWorkflowNodeHandler,
		"http-event":        eventInputExplainWorkflowNodeHandler("HTTP", "OpenAPIEventHandler", true),
		"webhook-event":     eventInputExplainWorkflowNodeHandler("WEBHOOK", "WebhookReceiver", true),
		"mq-event":          mqEventWorkflowNodeHandler,
		"schedule-event":    scheduleEventWorkflowNodeHandler,
		"state-event":       s.stateEventWorkflowNodeHandler,
		"manual-event":      eventInputExplainWorkflowNodeHandler("MANUAL", "AdminManualTriggerAPI", false),
		"inbox":             inboxWorkflowNodeHandler,
		"normalize":         normalizeWorkflowNodeHandler,
		"match":             matchWorkflowNodeHandler,
		"fact-query":        s.factQueryWorkflowNodeHandler,
		"context-query":     contextFactQueryWorkflowNodeHandler,
		"local-cache-query": localCacheFactQueryWorkflowNodeHandler,
		"redis-query":       factAdapterRequiredWorkflowNodeHandler("REDIS", "RedisFactCache", false),
		"http-query":        s.httpFactQueryWorkflowNodeHandler,
		"grpc-query":        factAdapterRequiredWorkflowNodeHandler("GRPC", "GRPCFactProvider", false),
		"plugin-query":      factAdapterRequiredWorkflowNodeHandler("PLUGIN", "ActivityPlugin/FactProvider", false),
		"custom-query":      factAdapterRequiredWorkflowNodeHandler("CUSTOM", "CustomFactProvider", false),
		"lua-query":         s.luaFactQueryWorkflowNodeHandler,
		"activity-state":    s.activityStateWorkflowNodeHandler,
		"state-store":       s.stateStoreWorkflowNodeHandler,
		"quota-check":       s.quotaCheckWorkflowNodeHandler,
		"user-level-check":  userLevelCheckWorkflowNodeHandler,
		"condition":         decisionWorkflowNodeHandler,
		"qualify":           decisionWorkflowNodeHandler,
		"switch":            decisionWorkflowNodeHandler,
		"code":              s.codeWorkflowNodeHandler,
		"combine":           s.transformScriptWorkflowNodeHandler("combine", "combined_result"),
		"split":             s.transformScriptWorkflowNodeHandler("split", "split_items"),
		"candidate-reward":  candidateRewardWorkflowNodeHandler,
		"reward":            rewardWorkflowNodeHandler,
		"decimal":           decimalWorkflowNodeHandler,
		"limit":             limitWorkflowNodeHandler,
		"quota":             quotaWorkflowNodeHandler,
		"budget-reserve":    budgetReserveWorkflowNodeHandler,
		"lock":              s.lockWorkflowNodeHandler,
		"participation":     participationWorkflowNodeHandler,
		"audit":             auditWorkflowNodeHandler,
		"grant-order":       grantOrderWorkflowNodeHandler,
		"outbox":            s.outboxWorkflowNodeHandler,
		"retry":             retryWorkflowNodeHandler,
		"dlq":               dlqWorkflowNodeHandler,
		"http-call":         dispatchWorkflowNodeHandler("HTTP"),
		"mq-publish":        s.mqPublishWorkflowNodeHandler,
		"webhook":           dispatchWorkflowNodeHandler("WEBHOOK"),
		"callback":          s.callbackWorkflowNodeHandler,
		"approval-gate":     approvalGateWorkflowNodeHandler,
		"compensation":      compensationWorkflowNodeHandler,
		"random-pick":       randomPickWorkflowNodeHandler,
		"plugin-execute":    pluginExecuteWorkflowNodeHandler,
		"ab-bucket":         abBucketWorkflowNodeHandler,
		"risk-score":        riskScoreWorkflowNodeHandler,
		"settlement-window": settlementWindowWorkflowNodeHandler,
		"manual-adjust":     manualAdjustWorkflowNodeHandler,
		"reconcile":         reconcileWorkflowNodeHandler,
		"reward-reconcile":  rewardReconcileWorkflowNodeHandler,
		"alert":             alertWorkflowNodeHandler,
	}
}

func workflowExecutionPlan(definition marketing.WorkflowDefinition) (workflowExecutionPlanData, error) {
	nodeByID := map[string]marketing.WorkflowNode{}
	edgesBySource := map[string][]marketing.WorkflowEdge{}
	incoming := map[string][]marketing.WorkflowEdge{}
	issues := []marketing.WorkflowValidationIssue{}
	for index, node := range definition.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_NODE_ID_REQUIRED", "DAG compile failed: node id is required", fmt.Sprintf("nodes[%d].id", index), "", ""))
			continue
		}
		if _, ok := nodeByID[node.ID]; ok {
			issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_NODE_ID_DUPLICATED", fmt.Sprintf("DAG compile failed: node %q is duplicated", node.ID), fmt.Sprintf("nodes[%d].id", index), node.ID, ""))
		}
		nodeByID[node.ID] = node
	}
	edgeIDByValue := map[string]int{}
	for index, edge := range definition.Edges {
		edgeID := strings.TrimSpace(edge.ID)
		if edgeID != "" {
			if firstIndex, ok := edgeIDByValue[edgeID]; ok {
				issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_EDGE_ID_DUPLICATED", fmt.Sprintf("DAG compile failed: edge %q is duplicated", edgeID), fmt.Sprintf("edges[%d].id", index), "", edgeID))
				if firstIndex >= 0 {
					issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_EDGE_ID_DUPLICATED", fmt.Sprintf("DAG compile failed: edge %q is duplicated", edgeID), fmt.Sprintf("edges[%d].id", firstIndex), "", edgeID))
					edgeIDByValue[edgeID] = -1
				}
			} else {
				edgeIDByValue[edgeID] = index
			}
		}
		source := strings.TrimSpace(edge.Source)
		target := strings.TrimSpace(edge.Target)
		if source == "" || target == "" {
			issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_EDGE_ENDPOINT_REQUIRED", "DAG compile failed: edge source and target are required", fmt.Sprintf("edges[%d]", index), "", edge.ID))
			continue
		}
		if _, ok := nodeByID[source]; !ok {
			issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_EDGE_SOURCE_MISSING", fmt.Sprintf("DAG compile failed: source node %q does not exist", source), fmt.Sprintf("edges[%d].source", index), source, edge.ID))
			continue
		}
		if _, ok := nodeByID[target]; !ok {
			issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_EDGE_TARGET_MISSING", fmt.Sprintf("DAG compile failed: target node %q does not exist", target), fmt.Sprintf("edges[%d].target", index), target, edge.ID))
			continue
		}
		normalizedEdge := edge
		normalizedEdge.ID = edgeID
		normalizedEdge.Source = source
		normalizedEdge.Target = target
		edgesBySource[source] = append(edgesBySource[source], normalizedEdge)
		incoming[target] = append(incoming[target], normalizedEdge)
	}
	if len(issues) > 0 {
		return workflowExecutionPlanData{}, workflowPlanCompileError{Issues: issues}
	}
	var roots []marketing.WorkflowNode
	for _, node := range definition.Nodes {
		if node.Type == "event-in" || node.Kind == "entry" {
			roots = append(roots, node)
		}
	}
	if len(roots) == 0 && len(definition.Nodes) > 0 {
		return workflowExecutionPlanData{}, workflowPlanCompileError{Issues: []marketing.WorkflowValidationIssue{
			workflowCompileIssue("WORKFLOW_DAG_ENTRY_MISSING", "DAG compile failed: standard event entry node is required", "nodes", "", ""),
		}}
	}
	reachable := workflowReachableNodeIDs(roots, edgesBySource)
	for _, node := range definition.Nodes {
		if len(reachable) == 0 || reachable[node.ID] {
			continue
		}
		issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_NODE_UNREACHABLE", fmt.Sprintf("DAG compile failed: node %q is not reachable from an entry node", node.ID), "nodes", node.ID, ""))
	}
	if len(issues) > 0 {
		return workflowExecutionPlanData{}, workflowPlanCompileError{Issues: issues}
	}
	topo, levels, err := workflowTopologicalOrder(definition, incoming, edgesBySource)
	if err != nil {
		return workflowExecutionPlanData{}, err
	}
	topoIndex := map[string]int{}
	for index, node := range topo {
		topoIndex[node.ID] = index
	}
	return workflowExecutionPlanData{
		NodeByID:      nodeByID,
		EdgesBySource: edgesBySource,
		Incoming:      incoming,
		Roots:         roots,
		TopoIndex:     topoIndex,
		TopoLevels:    levels,
	}, nil
}

func workflowReachableNodeIDs(roots []marketing.WorkflowNode, edgesBySource map[string][]marketing.WorkflowEdge) map[string]bool {
	reachable := map[string]bool{}
	queue := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.TrimSpace(root.ID) == "" {
			continue
		}
		reachable[root.ID] = true
		queue = append(queue, root.ID)
	}
	for len(queue) > 0 {
		source := queue[0]
		queue = queue[1:]
		for _, edge := range edgesBySource[source] {
			target := strings.TrimSpace(edge.Target)
			if target == "" || reachable[target] {
				continue
			}
			reachable[target] = true
			queue = append(queue, target)
		}
	}
	return reachable
}

func workflowTopologicalOrder(definition marketing.WorkflowDefinition, incoming map[string][]marketing.WorkflowEdge, edgesBySource map[string][]marketing.WorkflowEdge) ([]marketing.WorkflowNode, map[string]int, error) {
	indegree := map[string]int{}
	nodeByID := map[string]marketing.WorkflowNode{}
	levels := map[string]int{}
	for _, node := range definition.Nodes {
		indegree[node.ID] = len(incoming[node.ID])
		nodeByID[node.ID] = node
	}
	var queue []marketing.WorkflowNode
	for _, node := range definition.Nodes {
		if indegree[node.ID] == 0 {
			queue = append(queue, node)
		}
	}
	var out []marketing.WorkflowNode
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		out = append(out, node)
		for _, edge := range edgesBySource[node.ID] {
			if levels[edge.Target] < levels[node.ID]+1 {
				levels[edge.Target] = levels[node.ID] + 1
			}
			indegree[edge.Target]--
			if indegree[edge.Target] == 0 {
				if target, ok := nodeByID[edge.Target]; ok {
					queue = append(queue, target)
				}
			}
		}
	}
	if len(out) != len(definition.Nodes) {
		issues := []marketing.WorkflowValidationIssue{}
		for nodeID, degree := range indegree {
			if degree > 0 {
				issues = append(issues, workflowCompileIssue("WORKFLOW_DAG_UNSUPPORTED_CYCLE", fmt.Sprintf("DAG compile failed: node %q is in an unsupported cycle", nodeID), "nodes", nodeID, ""))
			}
		}
		return nil, nil, workflowPlanCompileError{Issues: issues}
	}
	return out, levels, nil
}

func workflowCompileIssue(code string, message string, path string, nodeID string, edgeID string) marketing.WorkflowValidationIssue {
	return marketing.WorkflowValidationIssue{
		Severity: marketing.WorkflowValidationBlock,
		Code:     code,
		Message:  message,
		NodeID:   nodeID,
		EdgeID:   edgeID,
		Path:     path,
	}
}

func workflowNextEdges(node marketing.WorkflowNode, result workflowNodeResult, edges []marketing.WorkflowEdge) []marketing.WorkflowEdge {
	if len(edges) == 0 {
		return nil
	}
	if !workflowNodeIsBranch(node) {
		return edges
	}
	var defaultEdge *marketing.WorkflowEdge
	for index := range edges {
		edge := edges[index]
		if workflowRouteIsExplicitDefault(edge) {
			defaultEdge = &edge
		}
		if workflowRouteMatches(edge, result.Output) {
			return []marketing.WorkflowEdge{edge}
		}
	}
	if defaultEdge != nil {
		return []marketing.WorkflowEdge{*defaultEdge}
	}
	return nil
}

func workflowExecutionRouteTraceItem(edge marketing.WorkflowEdge, result workflowNodeResult) map[string]any {
	item := map[string]any{
		"edgeId":     edge.ID,
		"source":     edge.Source,
		"target":     edge.Target,
		"label":      edge.Label,
		"nodeStatus": result.Status,
	}
	if edge.Route != nil {
		item["routeType"] = edge.Route.NormalizedType()
		item["routeKey"] = edge.Route.NormalizedKey()
		item["routeLabel"] = edge.Route.Label
		item["expression"] = edge.Route.Expression
		item["conditionRef"] = edge.Route.ConditionRef
		item["default"] = edge.Route.IsDefault()
	}
	if decision := workflowOutputString(result.Output["decision"]); decision != "" {
		item["decision"] = decision
	}
	if route := workflowOutputString(result.Output["route"]); route != "" {
		item["route"] = route
	}
	return item
}

func workflowNodeIsBranch(node marketing.WorkflowNode) bool {
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "condition", "qualify", "user-level-check", "switch", "approval-gate", "ab-bucket", "risk-score", "settlement-window":
		return true
	default:
		return false
	}
}

func workflowEnqueueByTopology(queue []marketing.WorkflowNode, node marketing.WorkflowNode, topoIndex map[string]int) []marketing.WorkflowNode {
	queue = append(queue, node)
	for i := len(queue) - 1; i > 0; i-- {
		if topoIndex[queue[i].ID] >= topoIndex[queue[i-1].ID] {
			break
		}
		queue[i], queue[i-1] = queue[i-1], queue[i]
	}
	return queue
}

func executeWorkflowNodeWithPolicy(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, handler workflowNodeHandler) workflowNodeResult {
	policy := workflowNodePolicy(node)
	attempts := 0
	var last workflowNodeResult
	for {
		attempts++
		last = executeWorkflowNodeAttempt(ctx, node, runtime, handler, policy.Timeout)
		if last.Output == nil {
			last.Output = map[string]any{}
		}
		last.Output["attempt"] = attempts
		last.Output["maxRetries"] = policy.MaxRetries
		last.Output["retryBackoffMs"] = policy.RetryBackoff.Milliseconds()
		last.Output["concurrencyGroup"] = policy.ConcurrencyGroup
		last.Output["concurrencyLimit"] = policy.ConcurrencyLimit
		last.Output["concurrencyMode"] = "SERIAL_PER_EXECUTION"
		last.Output["concurrencyEnforced"] = false
		last.Output["concurrencyNote"] = "concurrencyGroup/concurrencyLimit are advisory metadata; cross-execution throttling is not enforced by the workflow executor"
		if last.Status != marketing.WorkflowNodeFailed || attempts > policy.MaxRetries {
			return last
		}
		if policy.RetryBackoff > 0 {
			timer := time.NewTimer(policy.RetryBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nodeFailed(ctx.Err().Error(), map[string]any{"attempt": attempts, "canceled": true})
			case <-timer.C:
			}
		}
	}
}

func executeWorkflowNodeAttempt(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, handler workflowNodeHandler, timeout time.Duration) workflowNodeResult {
	if timeout <= 0 {
		return handler(ctx, node, runtime)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resultCh := make(chan workflowNodeResult, 1)
	go func() {
		resultCh <- handler(attemptCtx, node, runtime)
	}()
	select {
	case result := <-resultCh:
		return result
	case <-attemptCtx.Done():
		return nodeFailed(fmt.Sprintf("node execution timeout after %s", timeout), map[string]any{"timeout": timeout.String()})
	}
}

func workflowNodePolicy(node marketing.WorkflowNode) workflowNodeExecutionPolicy {
	policy := workflowNodeExecutionPolicy{Timeout: 30 * time.Second, ConcurrencyGroup: "default", ConcurrencyLimit: 1}
	config := workflowNodeConfigMap(node)
	if seconds := workflowConfigNumber(config, "timeoutSeconds"); seconds > 0 {
		policy.Timeout = time.Duration(seconds * float64(time.Second))
	}
	if seconds := workflowConfigNumber(config, "retryBackoffSeconds"); seconds > 0 {
		policy.RetryBackoff = time.Duration(seconds * float64(time.Second))
	}
	if retries := workflowConfigNumber(config, "maxRetries"); retries > 0 {
		policy.MaxRetries = int(retries)
	}
	if retries := workflowConfigNumber(config, "maxAttempts"); retries > 1 {
		policy.MaxRetries = int(retries) - 1
	}
	if policy.MaxRetries > 10 {
		policy.MaxRetries = 10
	}
	if policy.RetryBackoff > 30*time.Second {
		policy.RetryBackoff = 30 * time.Second
	}
	if group := workflowConfigString(config, "concurrencyGroup"); group != "" {
		policy.ConcurrencyGroup = group
	}
	if limit := workflowConfigNumber(config, "concurrencyLimit"); limit > 0 {
		policy.ConcurrencyLimit = int(limit)
	}
	if policy.ConcurrencyLimit > 64 {
		policy.ConcurrencyLimit = 64
	}
	return policy
}

func workflowNodeConfigMap(node marketing.WorkflowNode) map[string]any {
	config := map[string]any{}
	if len(node.Config) == 0 {
		return workflowNodeConfigWithDefaults(node, config)
	}
	if err := json.Unmarshal(node.Config, &config); err != nil {
		return nil
	}
	return workflowNodeConfigWithDefaults(node, config)
}

func workflowNodeConfigWithDefaults(node marketing.WorkflowNode, config map[string]any) map[string]any {
	schema, ok := marketing.DefaultWorkflowNodeConfigSchemas().Get(nodeHandlerKey(node))
	if !ok {
		if len(config) == 0 {
			return nil
		}
		return config
	}
	for _, field := range schema.Fields {
		value, exists := config[field.Key]
		if exists && !workflowConfigValueEmpty(value) {
			continue
		}
		defaultValue, hasDefault := workflowNormalizeDefaultConfigValue(field.Default)
		if !hasDefault {
			continue
		}
		config[field.Key] = defaultValue
	}
	if len(config) == 0 {
		return nil
	}
	return config
}

func workflowNormalizeDefaultConfigValue(value any) (any, bool) {
	if workflowConfigValueEmpty(value) {
		return nil, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return value, true
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return value, true
	}
	if workflowConfigValueEmpty(decoded) {
		return nil, false
	}
	return decoded, true
}

func workflowConfigValueEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	default:
		return false
	}
}

func workflowConfigNumber(config map[string]any, key string) float64 {
	switch value := config[key].(type) {
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

func workflowExecutionIdempotencyKey(activityID string, versionID string, inboxKey string) string {
	return strings.Join([]string{strings.TrimSpace(activityID), strings.TrimSpace(versionID), strings.TrimSpace(inboxKey)}, ":")
}

func (s *Service) findExistingWorkflowExecution(ctx context.Context, activityID string, eventID string, versionID string, idempotencyKey string) (marketing.WorkflowExecution, error) {
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ActivityID: activityID, EventID: eventID, Limit: 20})
	if err != nil {
		return marketing.WorkflowExecution{}, err
	}
	for _, execution := range executions {
		if execution.WorkflowVersionID != versionID {
			continue
		}
		input := map[string]any{}
		_ = json.Unmarshal(execution.Input, &input)
		if strings.TrimSpace(fmt.Sprint(input["idempotencyKey"])) == idempotencyKey {
			return execution, nil
		}
	}
	return marketing.WorkflowExecution{}, nil
}

func (s *Service) workflowExecutionCancelState(ctx context.Context, executionID string) (bool, string, error) {
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: executionID, Limit: 1})
	if err != nil {
		return false, "", err
	}
	if len(executions) == 0 {
		return false, "", marketing.ErrNotFound
	}
	if executions[0].Status != marketing.WorkflowExecutionCanceled {
		return false, "", nil
	}
	reason := strings.TrimSpace(executions[0].ErrorMessage)
	if reason == "" {
		reason = "execution canceled"
	}
	return true, reason, nil
}

func (s *Service) createSkippedWorkflowNodeExecution(ctx context.Context, runtime workflowRuntime, executionID string, node marketing.WorkflowNode, reason string) error {
	now := s.now()
	finishedAt := now
	_, err := s.repository.CreateWorkflowNodeExecution(ctx, marketing.WorkflowNodeExecution{
		ID:           marketingID("wfnode"),
		TenantID:     runtime.Activity.TenantID,
		ExecutionID:  executionID,
		NodeID:       node.ID,
		NodeType:     nodeHandlerKey(node),
		NodeTitle:    node.Title,
		Status:       marketing.WorkflowNodeSkipped,
		Output:       marshalWorkflowPayload(map[string]any{"skipReason": reason}),
		ErrorMessage: reason,
		StartedAt:    now,
		FinishedAt:   &finishedAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	return err
}

func workflowRouteIsExplicitDefault(edge marketing.WorkflowEdge) bool {
	if edge.Route == nil {
		return false
	}
	return edge.Route.IsDefault()
}

func workflowRouteMatches(edge marketing.WorkflowEdge, output map[string]any) bool {
	if edge.Route == nil {
		return false
	}
	if edge.Route.IsDefault() {
		return false
	}
	if strings.TrimSpace(edge.Route.Expression) != "" {
		return workflowRouteExpressionMatches(edge.Route.Expression, output)
	}
	if strings.TrimSpace(edge.Route.ConditionRef) != "" {
		return workflowRouteConditionRefMatches(edge.Route.ConditionRef, output)
	}
	if outputKey := edge.Route.OutputKey(); outputKey != "" {
		return workflowRouteValueMatches(outputKey, output)
	}
	return workflowRouteValueMatches(string(edge.Route.NormalizedType()), output)
}

func workflowRouteExpressionMatches(expression string, output map[string]any) bool {
	expression = strings.TrimSpace(expression)
	for _, operator := range []string{">=", "<=", "==", "!=", ">", "<"} {
		parts := strings.SplitN(expression, operator, 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		actual, ok := workflowRouteOutputValue(output, left)
		if !ok {
			return false
		}
		expected := workflowRouteLiteralValue(parts[1])
		return workflowRouteCompare(actual, expected, operator)
	}
	return false
}

func workflowRouteConditionRefMatches(conditionRef string, output map[string]any) bool {
	value, ok := workflowRouteOutputValue(output, conditionRef)
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || strings.EqualFold(typed, "yes") || strings.EqualFold(typed, "matched")
	default:
		return false
	}
}

func workflowRouteOutputValue(output map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	if strings.HasPrefix(path, "$.") || path == "$" {
		value, err := extractWorkflowJSONPath(output, path)
		return value, err == nil
	}
	if value, ok := output[path]; ok {
		return value, true
	}
	if strings.Contains(path, ".") {
		value, err := extractWorkflowJSONPath(output, "$."+path)
		return value, err == nil
	}
	return nil, false
}

func workflowRouteLiteralValue(value string) any {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return strings.Trim(value, `"'`)
		}
	}
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

func workflowRouteCompare(actual any, expected any, operator string) bool {
	switch operator {
	case "==", "!=":
		matched := workflowRouteValuesEqual(actual, expected)
		if operator == "!=" {
			return !matched
		}
		return matched
	case ">", ">=", "<", "<=":
		left, leftOK := workflowRouteNumber(actual)
		right, rightOK := workflowRouteNumber(expected)
		if !leftOK || !rightOK {
			return false
		}
		switch operator {
		case ">":
			return left > right
		case ">=":
			return left >= right
		case "<":
			return left < right
		case "<=":
			return left <= right
		}
	}
	return false
}

func workflowRouteValuesEqual(actual any, expected any) bool {
	if left, leftOK := workflowRouteNumber(actual); leftOK {
		if right, rightOK := workflowRouteNumber(expected); rightOK {
			return left == right
		}
	}
	if left, ok := actual.(bool); ok {
		if right, ok := expected.(bool); ok {
			return left == right
		}
	}
	return strings.EqualFold(
		normalizeWorkflowDecisionValue(workflowOutputString(actual)),
		normalizeWorkflowDecisionValue(workflowOutputString(expected)),
	)
}

func workflowRouteNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func workflowRouteValueMatches(routeType string, output map[string]any) bool {
	if value, ok := workflowRouteOutputValue(output, routeType); ok {
		if typed, ok := value.(bool); ok {
			return typed
		}
	}
	for _, key := range []string{"decision", "status", "result", "route", "routeKey", "outputKey"} {
		if value := workflowOutputString(output[key]); value != "" {
			if strings.EqualFold(normalizeWorkflowDecisionValue(value), normalizeWorkflowDecisionValue(routeType)) {
				return true
			}
		}
	}
	return false
}

func workflowOutputString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func normalizeWorkflowDecisionValue(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PASSED":
		return "PASS"
	case "REJECTED":
		return "BLOCK"
	case "SKIPPED":
		return "SKIP"
	default:
		return strings.ToUpper(strings.TrimSpace(value))
	}
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

func resolveWorkflowNodeInputs(node marketing.WorkflowNode, runtime workflowRuntime, nodeOutputs map[string]map[string]any) (map[string]any, error) {
	input := map[string]any{
		"eventId":         runtime.Inbox.Event.EventID,
		"activityId":      runtime.Activity.ID,
		"participationId": runtime.Participation.ID,
		"qualified":       runtime.Qualified,
	}
	mapping := workflowInputMapping(node)
	if len(mapping) == 0 {
		return input, nil
	}
	resolved := map[string]any{}
	for inputKey, ref := range mapping {
		sourceOutput, ok := nodeOutputs[ref.SourceNodeID]
		if !ok {
			return input, fmt.Errorf("input %q references source node %q before it has output", inputKey, ref.SourceNodeID)
		}
		value, ok := sourceOutput[ref.OutputKey]
		if !ok {
			return input, fmt.Errorf("input %q references missing output %q from source node %q", inputKey, ref.OutputKey, ref.SourceNodeID)
		}
		extracted, err := extractWorkflowJSONPath(value, ref.Path)
		if err != nil {
			if workflowQualifyOptionalInputObject(node, inputKey) {
				extracted = map[string]any{}
				resolved[inputKey] = extracted
				input[inputKey] = extracted
				continue
			}
			return input, fmt.Errorf("input %q path %q cannot be resolved: %w", inputKey, ref.Path, err)
		}
		resolved[inputKey] = extracted
		input[inputKey] = extracted
	}
	input["inputMapping"] = mapping
	input["resolvedInputs"] = resolved
	return input, nil
}

func workflowQualifyOptionalInputObject(node marketing.WorkflowNode, inputKey string) bool {
	if !strings.EqualFold(nodeHandlerKey(node), "qualify") {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(inputKey)) {
	case "USER_PROFILE", "KYC_PROFILE", "RISK_PROFILE", "DEPOSIT_SUMMARY":
		return true
	default:
		return false
	}
}

func workflowInputMapping(node marketing.WorkflowNode) map[string]marketing.WorkflowNodeInputMappingRef {
	if len(node.Config) == 0 {
		return nil
	}
	config := map[string]any{}
	if err := json.Unmarshal(node.Config, &config); err != nil {
		return nil
	}
	rawMapping, ok := config["inputMapping"].(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]marketing.WorkflowNodeInputMappingRef{}
	for inputKey, rawRef := range rawMapping {
		ref := workflowInputMappingRef(rawRef)
		if ref.SourceNodeID == "" || ref.OutputKey == "" {
			continue
		}
		out[inputKey] = ref
	}
	return out
}

func workflowInputMappingRef(value any) marketing.WorkflowNodeInputMappingRef {
	item, ok := value.(map[string]any)
	if !ok {
		return marketing.WorkflowNodeInputMappingRef{}
	}
	sourceNodeID, _ := item["sourceNodeId"].(string)
	outputKey, _ := item["outputKey"].(string)
	path, _ := item["path"].(string)
	if sourceNodeID == "" {
		sourceNodeID, _ = item["nodeId"].(string)
	}
	if outputKey == "" {
		outputKey, _ = item["sourceOutputKey"].(string)
	}
	if outputKey == "" {
		outputKey, _ = item["output"].(string)
	}
	return marketing.WorkflowNodeInputMappingRef{
		SourceNodeID: strings.TrimSpace(sourceNodeID),
		OutputKey:    strings.TrimSpace(outputKey),
		Path:         strings.TrimSpace(path),
	}
}

func workflowNodeContext(ctx context.Context) workflowNodeExecutionContext {
	value, _ := ctx.Value(workflowNodeExecutionContextKey{}).(workflowNodeExecutionContext)
	return value
}

func workflowResolvedInput(ctx context.Context, key string) (any, bool) {
	value, ok := workflowNodeContext(ctx).ResolvedInput[key]
	return value, ok
}

func workflowResolvedInputValue(ctx context.Context) any {
	input := workflowNodeContext(ctx).ResolvedInput
	if input == nil {
		return nil
	}
	if value, ok := input["value"]; ok {
		return value
	}
	if value, ok := input["payload"]; ok {
		return value
	}
	if len(input) > 0 {
		return input
	}
	return nil
}

func workflowUpstreamOutput(ctx context.Context, outputKey string) (any, bool) {
	for _, output := range workflowNodeContext(ctx).NodeOutputs {
		if value, ok := output[outputKey]; ok {
			return value, true
		}
	}
	return nil, false
}

func workflowFactsFromContext(ctx context.Context) map[string]any {
	facts := map[string]any{}
	if value, ok := workflowResolvedInput(ctx, "facts"); ok {
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			for key, item := range mapped {
				facts[key] = item
			}
		}
	}
	if len(facts) == 0 {
		if value, ok := workflowUpstreamOutput(ctx, "facts"); ok {
			if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
				for key, item := range mapped {
					facts[key] = item
				}
			}
		}
	}
	for _, key := range []string{"USER_PROFILE", "KYC_PROFILE", "RISK_PROFILE", "DEPOSIT_SUMMARY"} {
		if value, ok := workflowResolvedInput(ctx, key); ok {
			facts[key] = workflowJSONValue(value)
		}
	}
	if len(facts) == 0 {
		return nil
	}
	return facts
}

func workflowConditionInput(ctx context.Context, runtime workflowRuntime) map[string]any {
	execCtx := workflowNodeContext(ctx)
	input := map[string]any{
		"event": workflowJSONValue(runtime.Inbox.Event),
		"input": workflowJSONValue(execCtx.ResolvedInput),
		"facts": workflowFactsFromContext(ctx),
	}
	if value, ok := workflowResolvedInput(ctx, "fact"); ok {
		input["fact"] = workflowJSONValue(value)
	} else if facts := workflowFactsFromContext(ctx); facts != nil {
		input["fact"] = facts
	}
	if value, ok := workflowResolvedInput(ctx, "condition_expr"); ok {
		input["condition_expr"] = value
	}
	return input
}

func workflowEvaluateConditionExpression(language string, expression string, input map[string]any) (bool, error) {
	switch strings.ToUpper(firstWorkflowString(language, "RULE_EXPR")) {
	case "RULE_EXPR":
		return workflowRouteExpressionMatches(expression, input), nil
	case "JSON_LOGIC":
		var logic any
		if err := json.Unmarshal([]byte(expression), &logic); err != nil {
			return false, err
		}
		value, err := workflowEvalJSONLogic(logic, input)
		if err != nil {
			return false, err
		}
		switch typed := value.(type) {
		case bool:
			return typed, nil
		case string:
			return strings.EqualFold(typed, "true"), nil
		default:
			return false, nil
		}
	default:
		return false, fmt.Errorf("expressionLanguage %s is unsupported", language)
	}
}

func workflowEvalJSONLogic(expr any, input map[string]any) (any, error) {
	object, ok := expr.(map[string]any)
	if !ok {
		return expr, nil
	}
	if len(object) != 1 {
		return nil, fmt.Errorf("json logic expression must contain one operator")
	}
	for op, rawArgs := range object {
		args, ok := rawArgs.([]any)
		if !ok {
			args = []any{rawArgs}
		}
		switch op {
		case "var":
			if len(args) == 0 {
				return nil, fmt.Errorf("var requires path")
			}
			path := workflowOutputString(args[0])
			if path == "" {
				return nil, fmt.Errorf("var path is required")
			}
			value, ok := workflowRouteOutputValue(input, path)
			if !ok && len(args) > 1 {
				return workflowEvalJSONLogic(args[1], input)
			}
			return value, nil
		case "==", "!=", ">", ">=", "<", "<=":
			if len(args) != 2 {
				return nil, fmt.Errorf("%s requires two args", op)
			}
			left, err := workflowEvalJSONLogic(args[0], input)
			if err != nil {
				return nil, err
			}
			right, err := workflowEvalJSONLogic(args[1], input)
			if err != nil {
				return nil, err
			}
			return workflowRouteCompare(left, right, op), nil
		case "and":
			for _, arg := range args {
				value, err := workflowEvalJSONLogic(arg, input)
				if err != nil {
					return nil, err
				}
				if !workflowTruthy(value) {
					return false, nil
				}
			}
			return true, nil
		case "or":
			for _, arg := range args {
				value, err := workflowEvalJSONLogic(arg, input)
				if err != nil {
					return nil, err
				}
				if workflowTruthy(value) {
					return true, nil
				}
			}
			return false, nil
		case "!":
			if len(args) != 1 {
				return nil, fmt.Errorf("! requires one arg")
			}
			value, err := workflowEvalJSONLogic(args[0], input)
			if err != nil {
				return nil, err
			}
			return !workflowTruthy(value), nil
		default:
			return nil, fmt.Errorf("json logic operator %s is unsupported", op)
		}
	}
	return nil, fmt.Errorf("json logic expression is empty")
}

func workflowTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.TrimSpace(typed) != "" && !strings.EqualFold(typed, "false")
	case nil:
		return false
	default:
		if number, ok := workflowRouteNumber(typed); ok {
			return number != 0
		}
		return true
	}
}

func workflowGrantOrderForReward(orders []marketing.GrantOrder, configuredReward marketing.Reward) marketing.GrantOrder {
	for _, order := range orders {
		if order.RewardType != "" && configuredReward.Type != "" && order.RewardType != configuredReward.Type {
			continue
		}
		if configuredReward.Code != "" {
			payload := map[string]any{}
			_ = json.Unmarshal(order.RewardPayload, &payload)
			if workflowOutputString(payload["rewardCode"]) != "" && workflowOutputString(payload["rewardCode"]) != configuredReward.Code {
				continue
			}
		}
		return order
	}
	if len(orders) > 0 {
		return orders[0]
	}
	return marketing.GrantOrder{}
}

func workflowGrantOrderCurrency(order marketing.GrantOrder) string {
	payload := map[string]any{}
	_ = json.Unmarshal(order.RewardPayload, &payload)
	return workflowOutputString(payload["currency"])
}

func workflowNormalizeFactCodes(refs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.Contains(ref, ".") {
			ref = strings.SplitN(ref, ".", 2)[0]
		}
		ref = strings.ToUpper(ref)
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func workflowFactQueryFailure(node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, factCode string, code string, message string, facts map[string]any, responses []any) workflowNodeResult {
	if code == "" {
		code = "FACT_QUERY_FAILED"
	}
	output := map[string]any{
		"errorCode":       code,
		"factCode":        factCode,
		"error":           message,
		"facts":           facts,
		"responses":       responses,
		"source":          strings.ToUpper(firstWorkflowString(workflowConfigString(config, "source"), "FACT_SERVICE")),
		"subject":         workflowJSONValue(workflowFactQuerySubject(context.Background(), config, runtime)),
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	}
	switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "failureAction"), "BLOCK")) {
	case "USE_EMPTY":
		output["failureAction"] = "USE_EMPTY"
		return nodeSuccess(output)
	case "SKIP":
		output["failureAction"] = "SKIP"
		return nodeSkipped(code+": "+message, output)
	default:
		output["failureAction"] = "BLOCK"
		return nodeFailed(code+": "+message, output)
	}
}

func workflowFactQueryKeys(config map[string]any, runtime workflowRuntime) []string {
	refs := workflowConfigStringList(config, "factKeys")
	if len(refs) == 0 {
		for factCode := range marketing.ExtractFactReferences(runtime.Activity.Rules) {
			refs = append(refs, factCode)
		}
	}
	return workflowNormalizeFactCodes(refs)
}

func workflowFactQuerySourcePaths(config map[string]any, factCode string) []string {
	paths := workflowConfigStringList(config, "sourcePaths")
	if len(paths) == 0 {
		if path := workflowConfigString(config, "sourcePath"); path != "" {
			paths = []string{path}
		}
	}
	if len(paths) == 0 && factCode != "" {
		paths = []string{"$.attributes." + strings.ToLower(factCode), "$.attributes." + factCode, "$.facts." + factCode}
	}
	return paths
}

func workflowFactQueryBaseContext(ctx context.Context, runtime workflowRuntime) map[string]any {
	execCtx := workflowNodeContext(ctx)
	return map[string]any{
		"event":      workflowJSONValue(runtime.Inbox.Event),
		"attributes": workflowJSONValue(runtime.Inbox.Event.Attributes),
		"subject":    workflowJSONValue(runtime.Inbox.Event.Subject),
		"biz":        workflowJSONValue(runtime.Inbox.Event.Biz),
		"input":      workflowJSONValue(execCtx.ResolvedInput),
		"outputs":    workflowJSONValue(execCtx.NodeOutputs),
		"vars":       workflowJSONValue(execCtx.Variables),
		"facts":      workflowFactsFromContext(ctx),
	}
}

func workflowFactValueFromPaths(source any, paths []string) (any, string, bool) {
	for _, path := range paths {
		value, err := extractWorkflowJSONPath(source, path)
		if err == nil {
			return value, path, true
		}
	}
	return nil, "", false
}

func workflowFactValueFromPathsForCode(source any, paths []string, factCode string, multiFact bool) (any, string, bool) {
	for _, path := range paths {
		value, err := extractWorkflowJSONPath(source, path)
		if err != nil {
			continue
		}
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			if factValue, ok := mapped[factCode]; ok {
				return factValue, strings.TrimRight(path, ".") + "." + factCode, true
			}
		}
		if !multiFact || strings.HasSuffix(strings.ToUpper(strings.TrimSpace(path)), "."+strings.ToUpper(factCode)) {
			return value, path, true
		}
	}
	return nil, "", false
}

func workflowFactOutput(node marketing.WorkflowNode, runtime workflowRuntime, source string, facts map[string]any, responses []any) map[string]any {
	return map[string]any{
		"factCount": len(facts),
		"facts":     facts,
		"responses": responses,
		"source":    source,
		"audit":     workflowHandlerAudit(node, runtime),
	}
}

func workflowFactQuerySubject(ctx context.Context, config map[string]any, runtime workflowRuntime) marketing.EventSubject {
	subject := runtime.Inbox.Event.Subject
	if value, ok := workflowResolvedInput(ctx, "subject"); ok {
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			if text := workflowOutputString(mapped["type"]); text != "" {
				subject.Type = text
			}
			if text := workflowOutputString(mapped["id"]); text != "" {
				subject.ID = text
			}
		} else if text := workflowOutputString(value); text != "" {
			subject.ID = text
		}
	}
	path := workflowConfigString(config, "subjectPath")
	if path == "" {
		return subject
	}
	base := map[string]any{
		"subject":    workflowJSONValue(subject),
		"event":      workflowJSONValue(runtime.Inbox.Event),
		"attributes": workflowJSONValue(runtime.Inbox.Event.Attributes),
		"biz":        workflowJSONValue(runtime.Inbox.Event.Biz),
	}
	if !strings.HasPrefix(path, "$.") && path != "$" {
		path = "$." + path
	}
	if value, err := extractWorkflowJSONPath(base, path); err == nil {
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			if text := workflowOutputString(mapped["type"]); text != "" {
				subject.Type = text
			}
			if text := workflowOutputString(mapped["id"]); text != "" {
				subject.ID = text
			}
		} else if text := workflowOutputString(value); text != "" {
			subject.ID = text
		}
	}
	return subject
}

func workflowLimitQuotaType(config map[string]any) marketing.QuotaType {
	period := strings.ToUpper(workflowConfigString(config, "period"))
	switch period {
	case "DAY":
		return marketing.QuotaDailyLimit
	default:
		return marketing.QuotaUserLimit
	}
}

func workflowLimitScopeKey(config map[string]any, runtime workflowRuntime) string {
	template := workflowConfigString(config, "scopeKeyTemplate")
	if template == "" {
		period := strings.ToUpper(workflowConfigString(config, "period"))
		if period == "DAY" {
			return dailyLimitScopeKey(runtime.Activity, runtime.Inbox.Event)
		}
		return runtime.Inbox.Event.Subject.ID
	}
	return workflowRenderTemplate(template, runtime)
}

func workflowQuotaType(value string, fallback marketing.QuotaType) marketing.QuotaType {
	switch marketing.QuotaType(strings.ToUpper(strings.TrimSpace(value))) {
	case marketing.QuotaBudget:
		return marketing.QuotaBudget
	case marketing.QuotaStock:
		return marketing.QuotaStock
	case marketing.QuotaUserLimit:
		return marketing.QuotaUserLimit
	case marketing.QuotaDailyLimit:
		return marketing.QuotaDailyLimit
	default:
		return fallback
	}
}

func workflowQuotaScope(value string, fallback marketing.QuotaScope) marketing.QuotaScope {
	switch marketing.QuotaScope(strings.ToUpper(strings.TrimSpace(value))) {
	case marketing.QuotaScopeActivity:
		return marketing.QuotaScopeActivity
	case marketing.QuotaScopeUser:
		return marketing.QuotaScopeUser
	case marketing.QuotaScopeIP:
		return marketing.QuotaScopeIP
	case marketing.QuotaScopeDevice:
		return marketing.QuotaScopeDevice
	default:
		return fallback
	}
}

func workflowQuotaScopeKey(config map[string]any, runtime workflowRuntime) string {
	template := workflowConfigString(config, "scopeKeyTemplate")
	if template != "" {
		return workflowRenderTemplate(template, runtime)
	}
	scope := workflowQuotaScope(workflowConfigString(config, "scope"), marketing.QuotaScopeActivity)
	if scope == marketing.QuotaScopeUser {
		return runtime.Inbox.Event.Subject.ID
	}
	return "TOTAL"
}

func workflowQuotaPlannedAmount(ctx context.Context, runtime workflowRuntime, config map[string]any) string {
	amountField := firstWorkflowString(workflowConfigString(config, "amountField"), "reward_items[0].amount")
	if value, ok := workflowRouteOutputValue(workflowNodeContext(ctx).ResolvedInput, amountField); ok {
		if text := workflowOutputString(value); text != "" {
			return text
		}
	}
	if value, ok := workflowUpstreamOutput(ctx, "reward_items"); ok {
		if amount, ok := workflowRewardItemsAmount(value); ok {
			return amount
		}
	}
	if len(runtime.GrantOrders) > 0 {
		return workflowGrantOrdersTotalAmount(runtime.GrantOrders)
	}
	return "1"
}

func workflowRewardItemsAmount(value any) (string, bool) {
	items, ok := workflowJSONValue(value).([]any)
	if !ok || len(items) == 0 {
		return "", false
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		return "", false
	}
	amount := workflowOutputString(item["amount"])
	return amount, amount != ""
}

func workflowGrantOrdersTotalAmount(orders []marketing.GrantOrder) string {
	total := 0.0
	hasAmount := false
	for _, order := range orders {
		amount, err := strconv.ParseFloat(strings.TrimSpace(order.RewardAmount), 64)
		if err != nil {
			continue
		}
		total += amount
		hasAmount = true
	}
	if !hasAmount {
		return "1"
	}
	return strconv.FormatFloat(total, 'f', -1, 64)
}

func workflowWorkflowRewardAmount(ctx context.Context, runtime workflowRuntime) float64 {
	for _, key := range []string{"candidate_rewards", "reward_items", "selected_rewards"} {
		if value, ok := workflowUpstreamOutput(ctx, key); ok {
			total := 0.0
			for _, item := range workflowAnyArray(value) {
				if object, ok := item.(map[string]any); ok {
					if number, ok := workflowRouteNumber(object["amount"]); ok {
						total += number
					}
				}
			}
			if total > 0 {
				return total
			}
		}
	}
	total, _ := strconv.ParseFloat(workflowGrantOrdersTotalAmount(runtime.GrantOrders), 64)
	return total
}

func workflowAnyArray(value any) []any {
	normalized := workflowJSONValue(value)
	switch typed := normalized.(type) {
	case []any:
		return typed
	case map[string]any:
		for _, key := range []string{"candidate_rewards", "reward_items", "selected_rewards", "items"} {
			if items, ok := typed[key]; ok {
				if array, ok := workflowJSONValue(items).([]any); ok {
					return array
				}
			}
		}
	}
	return nil
}

func workflowStableIndex(seed string, modulo int) int {
	if modulo <= 0 {
		return 0
	}
	var hash uint32 = 2166136261
	for _, b := range []byte(seed) {
		hash ^= uint32(b)
		hash *= 16777619
	}
	return int(hash % uint32(modulo))
}

func workflowPickWeightedLabel(seed string, buckets []string) string {
	type bucketSpec struct {
		label  string
		weight int
	}
	parsed := make([]bucketSpec, 0, len(buckets))
	total := 0
	for _, raw := range buckets {
		parts := strings.SplitN(raw, ":", 2)
		label := strings.TrimSpace(parts[0])
		if label == "" {
			continue
		}
		weight := 1
		if len(parts) == 2 {
			if parsedWeight, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && parsedWeight > 0 {
				weight = parsedWeight
			}
		}
		parsed = append(parsed, bucketSpec{label: label, weight: weight})
		total += weight
	}
	if len(parsed) == 0 || total <= 0 {
		return "A"
	}
	point := workflowStableIndex(seed, total)
	acc := 0
	for _, bucket := range parsed {
		acc += bucket.weight
		if point < acc {
			return bucket.label
		}
	}
	return parsed[len(parsed)-1].label
}

func workflowExtractRiskScore(ctx context.Context, config map[string]any) float64 {
	scorePath := firstWorkflowString(workflowConfigString(config, "scorePath"), "$.facts.riskScore")
	input := map[string]any{
		"facts": workflowFactsFromContext(ctx),
	}
	if value, ok := workflowResolvedInput(ctx, "facts"); ok {
		input["facts"] = workflowJSONValue(value)
	}
	if value, err := extractWorkflowJSONPath(input, scorePath); err == nil {
		if score, ok := workflowRouteNumber(value); ok {
			return score
		}
	}
	if value, ok := workflowUpstreamOutput(ctx, "riskScore"); ok {
		if score, ok := workflowRouteNumber(value); ok {
			return score
		}
	}
	return 0
}

func workflowRenderTemplate(template string, runtime workflowRuntime) string {
	replacements := map[string]string{
		"${subject.id}":   runtime.Inbox.Event.Subject.ID,
		"${subject.type}": runtime.Inbox.Event.Subject.Type,
		"${activityId}":   runtime.Activity.ID,
		"${tenantId}":     runtime.Activity.TenantID,
		"${eventId}":      runtime.Inbox.Event.EventID,
		"${eventType}":    runtime.Inbox.Event.EventType,
		"${biz.bizType}":  runtime.Inbox.Event.Biz.BizType,
		"${biz.bizId}":    runtime.Inbox.Event.Biz.BizID,
		"${date}":         runtime.Inbox.Event.OccurredAt.Format("2006-01-02"),
	}
	out := template
	for key, value := range replacements {
		out = strings.ReplaceAll(out, key, value)
	}
	return out
}

func workflowRenderNodeTemplate(template string, runtime workflowRuntime, node marketing.WorkflowNode) string {
	return strings.ReplaceAll(workflowRenderTemplate(template, runtime), "${nodeId}", node.ID)
}

func extractWorkflowJSONPath(value any, path string) (any, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return value, nil
	}
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("path must start with $")
	}
	current := value
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		if part == "" {
			return nil, fmt.Errorf("empty path segment")
		}
		next, err := extractWorkflowPathSegment(current, part)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func extractWorkflowPathSegment(value any, segment string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		next, ok := typed[segment]
		if !ok {
			return nil, fmt.Errorf("field %q not found", segment)
		}
		return next, nil
	case []any:
		index, err := strconv.Atoi(segment)
		if err != nil {
			return nil, fmt.Errorf("segment %q is not an array index", segment)
		}
		if index < 0 || index >= len(typed) {
			return nil, fmt.Errorf("array index %d out of range", index)
		}
		return typed[index], nil
	default:
		return nil, fmt.Errorf("cannot read %q from %T", segment, value)
	}
}

func workflowSetDottedPath(target map[string]any, path string, value any) error {
	path = strings.TrimSpace(strings.TrimPrefix(path, "$."))
	if path == "" {
		return fmt.Errorf("target path is required")
	}
	current := target
	parts := strings.Split(path, ".")
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty target path segment")
		}
		if strings.Contains(part, "[") || strings.Contains(part, "]") {
			return fmt.Errorf("array target path segment %q is not supported", part)
		}
		if index == len(parts)-1 {
			current[part] = workflowJSONValue(value)
			return nil
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	return nil
}

func workflowJSONValue(value any) any {
	bytes, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(bytes, &out); err != nil {
		return value
	}
	return out
}

func workflowHandlerAudit(node marketing.WorkflowNode, runtime workflowRuntime) map[string]any {
	return map[string]any{
		"nodeId":          node.ID,
		"nodeType":        nodeHandlerKey(node),
		"activityId":      runtime.Activity.ID,
		"workflowEventId": runtime.Inbox.Event.EventID,
		"tenantId":        runtime.Activity.TenantID,
	}
}

func workflowHandlerContextCanceled(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if err := ctx.Err(); err != nil {
		return nodeFailed("WORKFLOW_NODE_CONTEXT_CANCELED: "+err.Error(), map[string]any{
			"errorCode": "WORKFLOW_NODE_CONTEXT_CANCELED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	return workflowNodeResult{}
}

func workflowRequireNodeConfig(node marketing.WorkflowNode) (map[string]any, workflowNodeResult, bool) {
	config := workflowNodeConfigMap(node)
	if len(node.Config) > 0 && config == nil {
		return nil, nodeFailed("WORKFLOW_NODE_CONFIG_INVALID: config json is invalid", map[string]any{
			"errorCode": "WORKFLOW_NODE_CONFIG_INVALID",
		}), false
	}
	return config, workflowNodeResult{}, true
}

func workflowConfigString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	switch value := config[key].(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func workflowConfigBool(config map[string]any, key string, fallback bool) bool {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		if strings.EqualFold(strings.TrimSpace(typed), "true") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(typed), "false") {
			return false
		}
		return fallback
	default:
		return fallback
	}
}

func firstWorkflowString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func firstWorkflowAny(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func workflowConfigMap(config map[string]any, key string) map[string]any {
	if config == nil {
		return nil
	}
	value, ok := config[key]
	if !ok || value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func workflowApplyNodeContextConfig(vars map[string]any, node marketing.WorkflowNode) {
	if vars == nil {
		return
	}
	config := workflowNodeConfigMap(node)
	if len(config) == 0 {
		return
	}
	for key, value := range workflowConfigMap(config, "publicParams") {
		workflowSetVar(vars, "public", key, value)
	}
	for path, value := range workflowConfigMap(config, "contextPatch") {
		workflowApplyContextPatch(vars, path, value)
	}
}

func workflowApplyContextPatch(vars map[string]any, path string, value any) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "vars.")
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		workflowSetVar(vars, "execution", parts[0], value)
		return
	}
	scope := strings.TrimSpace(parts[0])
	key := strings.TrimSpace(strings.Join(parts[1:], "."))
	if scope == "" || key == "" {
		return
	}
	workflowSetVar(vars, scope, key, value)
}

func workflowConfigStringList(config map[string]any, key string) []string {
	value, ok := config[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if text := strings.TrimSpace(part); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func workflowStringInList(value string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func workflowMissingEventTypes(required []string, observed []string) []string {
	seen := map[string]bool{}
	for _, item := range observed {
		seen[strings.TrimSpace(strings.ToUpper(item))] = true
	}
	missing := []string{}
	for _, item := range required {
		item = strings.TrimSpace(item)
		if item == "" || seen[strings.ToUpper(item)] {
			continue
		}
		missing = append(missing, item)
	}
	return missing
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

func eventInputExplainWorkflowNodeHandler(sourceType string, authority string, implemented bool) workflowNodeHandler {
	return func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
			return canceled
		}
		mode := "EXPLAIN_ONLY"
		if implemented {
			mode = "EXTERNAL_ENDPOINT"
		}
		return nodeSuccess(map[string]any{
			"standard_event": workflowJSONValue(runtime.Inbox.Event),
			"source": map[string]any{
				"type":        sourceType,
				"implemented": implemented,
				"receivedAt":  runtime.Inbox.CreatedAt,
			},
			"adapter": map[string]any{
				"version":        "standard-v1",
				"mappingVersion": workflowConfigString(workflowNodeConfigMap(node), "mappingVersion"),
			},
			"executionMode":      mode,
			"executionAuthority": authority,
			"implemented":        implemented,
			"explanation":        "Event input nodes describe trigger/adaptor boundaries; workflow execution starts from the committed standard event.",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
}

func mqEventWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	broker := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "broker"), "KAFKA"))
	topic := strings.TrimSpace(workflowConfigString(config, "topic"))
	consumerGroup := strings.TrimSpace(workflowConfigString(config, "consumerGroup"))
	if topic == "" || consumerGroup == "" {
		return nodeFailed("MQ_EVENT_CONFIG_INVALID: topic and consumerGroup are required", map[string]any{
			"errorCode": "MQ_EVENT_CONFIG_INVALID",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	if broker != "KAFKA" && broker != "RABBITMQ" && broker != "PULSAR" {
		return nodeFailed("MQ_EVENT_BROKER_UNSUPPORTED: broker must be KAFKA, RABBITMQ or PULSAR", map[string]any{
			"errorCode": "MQ_EVENT_BROKER_UNSUPPORTED",
			"broker":    broker,
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	ackPolicy := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "ackPolicy"), "ACK_AFTER_INBOX_COMMIT"))
	retryPolicy := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "retryPolicy"), "RETRY_THEN_DLQ"))
	if ackPolicy != "ACK_AFTER_INBOX_COMMIT" {
		return nodeFailed("MQ_EVENT_ACK_POLICY_UNSUPPORTED: only ACK_AFTER_INBOX_COMMIT is supported", map[string]any{"errorCode": "MQ_EVENT_ACK_POLICY_UNSUPPORTED", "ackPolicy": ackPolicy, "audit": workflowHandlerAudit(node, runtime)})
	}
	if retryPolicy != "RETRY_THEN_DLQ" && retryPolicy != "DLQ_ONLY" {
		return nodeFailed("MQ_EVENT_RETRY_POLICY_UNSUPPORTED: retryPolicy must be RETRY_THEN_DLQ or DLQ_ONLY", map[string]any{"errorCode": "MQ_EVENT_RETRY_POLICY_UNSUPPORTED", "retryPolicy": retryPolicy, "audit": workflowHandlerAudit(node, runtime)})
	}
	allowlist := workflowConfigStringList(config, "allowedSourceSystems")
	if len(allowlist) == 0 {
		allowlist = workflowConfigStringList(config, "sourceSystems")
	}
	return nodeSuccess(map[string]any{
		"standard_event": workflowJSONValue(runtime.Inbox.Event),
		"source": map[string]any{
			"type":          "MQ",
			"broker":        broker,
			"topic":         topic,
			"consumerGroup": consumerGroup,
			"receivedAt":    runtime.Inbox.CreatedAt,
		},
		"consumerPlan": map[string]any{
			"adapter":              "internal/mqconsumer.Consumer",
			"payloadMode":          "STANDARD_EVENT",
			"ackPolicy":            ackPolicy,
			"retryPolicy":          retryPolicy,
			"dlqRequired":          retryPolicy != "DLQ_ONLY" || workflowConfigBool(config, "dlqRequired", true),
			"sourceAllowlist":      allowlist,
			"idempotencyHeader":    idempotencyKeyHeaderName(),
			"commitBoundary":       "ProcessEventFrom(mq) commits inbox before ACK",
			"maxBodyBytes":         int64(workflowConfigNumber(config, "maxBodyBytes")),
			"offsetCommitStrategy": "ACK_AFTER_INBOX_COMMIT",
		},
		"executionMode":      "TRIGGER_ADAPTER",
		"executionAuthority": "MQTriggerWorker/internal/mqconsumer.Consumer",
		"implemented":        true,
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func scheduleEventWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	scheduleType := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "scheduleType"), "CRON"))
	cron := strings.TrimSpace(workflowConfigString(config, "cron"))
	timezone := firstWorkflowString(workflowConfigString(config, "timezone"), "UTC")
	if scheduleType != "CRON" && scheduleType != "DELAY" {
		return nodeFailed("SCHEDULE_EVENT_TYPE_UNSUPPORTED: scheduleType must be CRON or DELAY", map[string]any{"errorCode": "SCHEDULE_EVENT_TYPE_UNSUPPORTED", "scheduleType": scheduleType, "audit": workflowHandlerAudit(node, runtime)})
	}
	if scheduleType == "CRON" && cron == "" {
		return nodeFailed("SCHEDULE_EVENT_CRON_REQUIRED: cron is required", map[string]any{"errorCode": "SCHEDULE_EVENT_CRON_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return nodeFailed("SCHEDULE_EVENT_TIMEZONE_INVALID: "+err.Error(), map[string]any{"errorCode": "SCHEDULE_EVENT_TIMEZONE_INVALID", "timezone": timezone, "audit": workflowHandlerAudit(node, runtime)})
	}
	eventType := firstWorkflowString(workflowConfigString(config, "eventType"), runtime.Inbox.Event.EventType)
	bizType := firstWorkflowString(workflowConfigString(config, "bizType"), "SCHEDULE")
	bizIDTemplate := firstWorkflowString(workflowConfigString(config, "bizIdTemplate"), "${activityId}:${nodeId}:${eventId}")
	bizID := workflowRenderNodeTemplate(bizIDTemplate, runtime, node)
	eventIDTemplate := firstWorkflowString(workflowConfigString(config, "eventIdTemplate"), "${activityId}:${nodeId}:${biz.bizId}")
	eventID := workflowRenderScheduleEventTemplate(eventIDTemplate, runtime, node, bizType, bizID)
	eventPlan := map[string]any{
		"tenantId":       runtime.Activity.TenantID,
		"eventType":      eventType,
		"sourceSystem":   firstWorkflowString(workflowConfigString(config, "sourceSystem"), "exmarketing-scheduler"),
		"eventId":        eventID,
		"occurredAt":     runtime.Inbox.Event.OccurredAt,
		"subject":        workflowJSONValue(runtime.Inbox.Event.Subject),
		"biz":            map[string]any{"bizType": bizType, "bizId": bizID},
		"attributes":     workflowJSONValue(runtime.Inbox.Event.Attributes),
		"idempotencyKey": runtime.Activity.TenantID + ":exmarketing-scheduler:" + eventType + ":" + bizType + ":" + bizID,
	}
	return nodeSuccess(map[string]any{
		"standard_event": workflowJSONValue(runtime.Inbox.Event),
		"source": map[string]any{
			"type":         "SCHEDULE",
			"scheduleType": scheduleType,
			"cron":         cron,
			"timezone":     timezone,
		},
		"schedulePlan": map[string]any{
			"adapter":        "Scheduler/EventInboxProducer",
			"misfirePolicy":  strings.ToUpper(firstWorkflowString(workflowConfigString(config, "misfirePolicy"), "FIRE_ONCE")),
			"jitterSeconds":  int64(workflowConfigNumber(config, "jitterSeconds")),
			"eventTemplate":  eventPlan,
			"commitBoundary": "scheduler creates marketing_event_inbox with stable idempotencyKey",
		},
		"executionMode":      "TRIGGER_ADAPTER",
		"executionAuthority": "Scheduler/EventInboxProducer",
		"implemented":        true,
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func workflowRenderScheduleEventTemplate(template string, runtime workflowRuntime, node marketing.WorkflowNode, bizType, bizID string) string {
	out := strings.TrimSpace(template)
	for placeholder, value := range map[string]string{
		"${biz.bizType}":     bizType,
		"${bizType}":         bizType,
		"${biz.bizId}":       bizID,
		"${bizId}":           bizID,
		"${scheduleBizId}":   bizID,
		"${scheduleBizType}": bizType,
	} {
		out = strings.ReplaceAll(out, placeholder, value)
	}
	return workflowRenderNodeTemplate(out, runtime, node)
}

func (s *Service) stateEventWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	source := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "source"), "RESOURCE"))
	keyTemplate := firstWorkflowString(workflowConfigString(config, "keyTemplate"), workflowConfigString(config, "key"))
	if keyTemplate == "" {
		return nodeFailed("STATE_EVENT_KEY_REQUIRED: keyTemplate is required", map[string]any{"errorCode": "STATE_EVENT_KEY_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
	}
	condition := strings.TrimSpace(workflowConfigString(config, "condition"))
	if condition == "" {
		return nodeFailed("STATE_EVENT_CONDITION_REQUIRED: condition is required", map[string]any{"errorCode": "STATE_EVENT_CONDITION_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
	}
	namespace := firstWorkflowString(workflowConfigString(config, "namespace"), "state-watch")
	key := workflowRenderNodeTemplate(keyTemplate, runtime, node)
	var snapshot any
	var resourceOutput map[string]any
	resourceRef := strings.TrimSpace(workflowConfigString(config, "resourceRef"))
	if resourceRef != "" {
		output, err := s.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
			Operation:   marketing.ResourceRead,
			ResourceRef: resourceRef,
			Namespace:   namespace,
			Key:         key,
			ValueType:   "OBJECT",
			Options:     map[string]any{"missingPolicy": firstWorkflowString(workflowConfigString(config, "missingPolicy"), "EMPTY")},
		})
		if err != nil {
			code := firstWorkflowString(workflowConfigString(output, "errorCode"), "STATE_EVENT_RESOURCE_READ_FAILED")
			return nodeFailed(code+": "+err.Error(), output)
		}
		resourceOutput = output
		snapshot = firstWorkflowAny(output["stored_value"], output["provider_response"])
		if entry, ok := workflowJSONValue(output["storage_entry"]).(map[string]any); ok {
			snapshot = firstWorkflowAny(entry["value"], snapshot)
		}
	} else {
		switch source {
		case "CONTEXT", "WORKFLOW_CONTEXT":
			snapshot = workflowJSONValue(runtime.Inbox.Event.Attributes)
		default:
			return nodeFailed("STATE_EVENT_RESOURCE_REF_REQUIRED: resourceRef is required for "+source+" state source", map[string]any{
				"errorCode":   "STATE_EVENT_RESOURCE_REF_REQUIRED",
				"source":      source,
				"namespace":   namespace,
				"key":         key,
				"resourceRef": resourceRef,
				"audit":       workflowHandlerAudit(node, runtime),
			})
		}
	}
	triggered, err := workflowStateEventConditionMatched(snapshot, condition)
	if err != nil {
		return nodeFailed("STATE_EVENT_CONDITION_INVALID: "+err.Error(), map[string]any{"errorCode": "STATE_EVENT_CONDITION_INVALID", "condition": condition, "audit": workflowHandlerAudit(node, runtime)})
	}
	output := map[string]any{
		"standard_event": workflowJSONValue(runtime.Inbox.Event),
		"source": map[string]any{
			"type":            "STATE",
			"source":          source,
			"namespace":       namespace,
			"key":             key,
			"intervalSeconds": int64(workflowConfigNumber(config, "intervalSeconds")),
			"debounceSeconds": int64(workflowConfigNumber(config, "debounceSeconds")),
		},
		"stateSnapshot":      workflowJSONValue(snapshot),
		"condition":          condition,
		"triggered":          triggered,
		"resource_result":    resourceOutput,
		"executionMode":      "TRIGGER_ADAPTER",
		"executionAuthority": "StateWatcher/ResourceAPI",
		"implemented":        true,
		"audit":              workflowHandlerAudit(node, runtime),
	}
	if !triggered {
		return nodeSkipped("STATE_EVENT_CONDITION_NOT_MATCHED: state condition did not match", output)
	}
	return nodeSuccess(output)
}

func idempotencyKeyHeaderName() string {
	return "Idempotency-Key"
}

func workflowStateEventConditionMatched(snapshot any, condition string) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return false, errors.New("condition is required")
	}
	if strings.EqualFold(condition, "true") {
		return true, nil
	}
	if strings.EqualFold(condition, "false") {
		return false, nil
	}
	ops := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range ops {
		if left, right, ok := strings.Cut(condition, op); ok {
			value, err := workflowStateEventConditionValue(snapshot, strings.TrimSpace(left))
			if err != nil {
				return false, err
			}
			expected := workflowStateEventLiteral(strings.TrimSpace(right))
			return workflowCompareStateEventValues(value, expected, op), nil
		}
	}
	value, err := workflowStateEventConditionValue(snapshot, condition)
	if err != nil {
		return false, err
	}
	return workflowStateEventTruthy(value), nil
}

func workflowStateEventConditionValue(snapshot any, expr string) (any, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "value" || expr == "$" {
		return workflowJSONValue(snapshot), nil
	}
	if strings.HasPrefix(expr, "$.") || strings.HasPrefix(expr, "$[") {
		return extractWorkflowJSONPath(snapshot, expr)
	}
	if strings.HasPrefix(expr, "value.") {
		return extractWorkflowJSONPath(snapshot, "$."+strings.TrimPrefix(expr, "value."))
	}
	return extractWorkflowJSONPath(snapshot, "$."+expr)
}

func workflowStateEventLiteral(value string) any {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	if strings.EqualFold(value, "null") {
		return nil
	}
	if len(value) >= 2 {
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) || (strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			return strings.Trim(value, `"'`)
		}
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

func workflowCompareStateEventValues(value any, expected any, op string) bool {
	if left, leftOK := workflowStateEventFloat(value); leftOK {
		if right, rightOK := workflowStateEventFloat(expected); rightOK {
			switch op {
			case "==":
				return left == right
			case "!=":
				return left != right
			case ">=":
				return left >= right
			case "<=":
				return left <= right
			case ">":
				return left > right
			case "<":
				return left < right
			}
		}
	}
	left := fmt.Sprint(value)
	right := fmt.Sprint(expected)
	switch op {
	case "==":
		return left == right
	case "!=":
		return left != right
	case ">=":
		return left >= right
	case "<=":
		return left <= right
	case ">":
		return left > right
	case "<":
		return left < right
	default:
		return false
	}
}

func workflowStateEventFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func workflowStateEventTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return strings.TrimSpace(typed) != "" && !strings.EqualFold(strings.TrimSpace(typed), "false")
	case float64:
		return typed != 0
	case json.Number:
		number, err := typed.Float64()
		return err == nil && number != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func entryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	if runtime.Inbox.Event.EventID == "" || runtime.Inbox.Event.EventType == "" || runtime.Inbox.Event.TenantID == "" {
		return nodeFailed("EVENT_IN_INVALID_INPUT: eventId, eventType and tenantId are required", map[string]any{
			"errorCode": "EVENT_IN_INVALID_INPUT",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	eventTypes := workflowConfigStringList(config, "eventTypes")
	if !workflowStringInList(runtime.Inbox.Event.EventType, eventTypes) {
		output := map[string]any{
			"errorCode":          "EVENT_IN_TYPE_NOT_ALLOWED",
			"eventType":          runtime.Inbox.Event.EventType,
			"requiredEventTypes": eventTypes,
			"audit":              workflowHandlerAudit(node, runtime),
		}
		if rejectUnknown, ok := config["rejectUnknownEventType"].(bool); ok && !rejectUnknown {
			return nodeSkipped("EVENT_IN_TYPE_NOT_ALLOWED: event type is not accepted by node config", output)
		}
		return nodeFailed("EVENT_IN_TYPE_NOT_ALLOWED: event type is not accepted by node config", output)
	}
	if !workflowStringInList(runtime.Inbox.Event.SourceSystem, workflowConfigStringList(config, "sourceSystems")) {
		return nodeFailed("EVENT_IN_SOURCE_NOT_ALLOWED: source system is not accepted by node config", map[string]any{
			"errorCode":    "EVENT_IN_SOURCE_NOT_ALLOWED",
			"sourceSystem": runtime.Inbox.Event.SourceSystem,
			"audit":        workflowHandlerAudit(node, runtime),
		})
	}
	rawEvent := workflowJSONValue(runtime.Inbox.Event)
	matchMode := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "eventTypeMatchMode"), "ANY"))
	if matchMode == "ALL" && len(eventTypes) > 1 {
		correlationKey := workflowRenderTemplate(firstWorkflowString(workflowConfigString(config, "correlationKeyTemplate"), "${tenantId}:${subject.id}:${activityId}"), runtime)
		output := map[string]any{
			"raw_event":                rawEvent,
			"eventId":                  runtime.Inbox.Event.EventID,
			"eventType":                runtime.Inbox.Event.EventType,
			"tenantId":                 runtime.Inbox.Event.TenantID,
			"eventTypeMatchMode":       "ALL",
			"multiEventSatisfied":      false,
			"currentEventType":         runtime.Inbox.Event.EventType,
			"requiredEventTypes":       eventTypes,
			"correlationKey":           correlationKey,
			"correlationWindowSeconds": workflowConfigNumber(config, "correlationWindowSeconds"),
			"waitingFor":               workflowMissingEventTypes(eventTypes, []string{runtime.Inbox.Event.EventType}),
			"audit":                    workflowHandlerAudit(node, runtime),
		}
		switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "allEventsAction"), "WAIT")) {
		case "BLOCK":
			return nodeFailed("EVENT_IN_ALL_NOT_SATISFIED: correlated event group is incomplete", output)
		default:
			return nodeSkipped("EVENT_IN_ALL_WAITING: correlated event group is incomplete", output)
		}
	}
	return nodeSuccess(map[string]any{
		"raw_event":          rawEvent,
		"eventId":            runtime.Inbox.Event.EventID,
		"eventType":          runtime.Inbox.Event.EventType,
		"tenantId":           runtime.Inbox.Event.TenantID,
		"eventTypeMatchMode": "ANY",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func inboxWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	if runtime.Inbox.ID == "" || runtime.Inbox.IdempotencyKey == "" {
		return nodeFailed("INBOX_INVALID_INPUT: inbox id and idempotency key are required", map[string]any{
			"errorCode": "INBOX_INVALID_INPUT",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	rawEvent := workflowJSONValue(runtime.Inbox.Event)
	persistenceMode := "PERSISTED"
	if strings.HasPrefix(runtime.Inbox.ID, "debuginbox_") {
		persistenceMode = "SIMULATED"
	}
	return nodeSuccess(map[string]any{
		"event_context": map[string]any{
			"inboxId":         runtime.Inbox.ID,
			"status":          runtime.Inbox.ProcessStatus,
			"idempotencyKey":  runtime.Inbox.IdempotencyKey,
			"event":           rawEvent,
			"persistenceMode": persistenceMode,
		},
		"inboxId":         runtime.Inbox.ID,
		"status":          runtime.Inbox.ProcessStatus,
		"idempotencyKey":  runtime.Inbox.IdempotencyKey,
		"persistenceMode": persistenceMode,
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func normalizeWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	rawEvent := workflowJSONValue(runtime.Inbox.Event)
	if value, ok := workflowResolvedInput(ctx, "raw_event"); ok {
		rawEvent = workflowJSONValue(value)
	} else if value, ok := workflowUpstreamOutput(ctx, "raw_event"); ok {
		rawEvent = workflowJSONValue(value)
	}
	normalized := map[string]any{
		"eventId":      runtime.Inbox.Event.EventID,
		"eventType":    runtime.Inbox.Event.EventType,
		"tenantId":     runtime.Inbox.Event.TenantID,
		"sourceSystem": runtime.Inbox.Event.SourceSystem,
		"subject":      workflowJSONValue(runtime.Inbox.Event.Subject),
		"biz":          workflowJSONValue(runtime.Inbox.Event.Biz),
		"attributes":   workflowJSONValue(runtime.Inbox.Event.Attributes),
		"occurredAt":   runtime.Inbox.Event.OccurredAt,
	}
	if mapping, ok := config["mapping"].(map[string]any); ok {
		for targetPath, rawSourcePath := range mapping {
			sourcePath := strings.TrimSpace(fmt.Sprint(rawSourcePath))
			if strings.TrimSpace(targetPath) == "" || sourcePath == "" {
				continue
			}
			value, err := extractWorkflowJSONPath(rawEvent, sourcePath)
			if err != nil {
				return nodeFailed("NORMALIZE_MAPPING_PATH_INVALID: "+err.Error(), map[string]any{
					"errorCode":  "NORMALIZE_MAPPING_PATH_INVALID",
					"targetPath": targetPath,
					"sourcePath": sourcePath,
					"audit":      workflowHandlerAudit(node, runtime),
				})
			}
			if err := workflowSetDottedPath(normalized, targetPath, value); err != nil {
				return nodeFailed("NORMALIZE_MAPPING_TARGET_INVALID: "+err.Error(), map[string]any{
					"errorCode":  "NORMALIZE_MAPPING_TARGET_INVALID",
					"targetPath": targetPath,
					"sourcePath": sourcePath,
					"audit":      workflowHandlerAudit(node, runtime),
				})
			}
		}
	}
	if defaults, ok := config["defaults"].(map[string]any); ok {
		for key, value := range defaults {
			if _, exists := normalized[key]; !exists {
				normalized[key] = value
			}
		}
	}
	return nodeSuccess(map[string]any{
		"normalized_event": normalized,
		"raw_event":        rawEvent,
		"normalized":       true,
		"eventId":          runtime.Inbox.Event.EventID,
		"audit":            workflowHandlerAudit(node, runtime),
	})
}

func matchWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	event := runtime.Inbox.Event
	audit := workflowHandlerAudit(node, runtime)
	baseOutput := map[string]any{
		"activityId":              runtime.Activity.ID,
		"activityCode":            runtime.Activity.Code,
		"eventId":                 event.EventID,
		"eventType":               event.EventType,
		"mode":                    strings.ToUpper(firstWorkflowString(workflowConfigString(config, "mode"), "FIRST_MATCH")),
		"tenantMatchPolicy":       strings.ToUpper(firstWorkflowString(workflowConfigString(config, "tenantMatchPolicy"), "SAME_TENANT")),
		"fallbackAction":          strings.ToUpper(firstWorkflowString(workflowConfigString(config, "fallbackAction"), "NO_MATCH_FAIL")),
		"priorityOrder":           strings.ToUpper(firstWorkflowString(workflowConfigString(config, "priorityOrder"), "PRIORITY_DESC")),
		"requireActiveActivity":   workflowConfigBool(config, "requireActiveActivity", true),
		"requireApprovedActivity": workflowConfigBool(config, "requireApprovedActivity", false),
		"audit":                   audit,
	}
	if runtime.Activity.ID == "" {
		return matchNodeNoMatch("MATCH_ACTIVITY_MISSING", "activity context is missing", baseOutput)
	}
	if strings.EqualFold(fmt.Sprint(baseOutput["tenantMatchPolicy"]), "SAME_TENANT") && runtime.Activity.TenantID != event.TenantID {
		return matchNodeNoMatch("MATCH_ACTIVITY_TENANT_MISMATCH", "activity tenant does not match event tenant", baseOutput)
	}
	if len(runtime.Activity.TriggerEvents) > 0 && !containsString(runtime.Activity.TriggerEvents, event.EventType) {
		return matchNodeNoMatch("MATCH_EVENT_TYPE_NOT_ALLOWED", "eventType is not configured on activity triggerEvents", baseOutput)
	}
	if pattern := strings.TrimSpace(workflowConfigString(config, "activityCodePattern")); pattern != "" {
		matched, err := regexp.MatchString(pattern, runtime.Activity.Code)
		if err != nil {
			baseOutput["errorCode"] = "MATCH_ACTIVITY_CODE_PATTERN_INVALID"
			baseOutput["error"] = err.Error()
			return nodeFailed("MATCH_ACTIVITY_CODE_PATTERN_INVALID: "+err.Error(), baseOutput)
		}
		if !matched {
			return matchNodeNoMatch("MATCH_ACTIVITY_CODE_PATTERN_NOT_MATCHED", "activityCode does not match configured pattern", baseOutput)
		}
	}
	if workflowConfigBool(config, "requireActiveActivity", true) {
		decision := matchWindowDecision(runtime.Activity, event.OccurredAt)
		if decision.ReasonCode == "APPROVAL_REQUIRED" && !workflowConfigBool(config, "requireApprovedActivity", false) {
			decision = matchDecision{Decision: marketing.RuleDecisionPassed}
		}
		if decision.Decision != marketing.RuleDecisionPassed {
			return matchNodeNoMatch(decision.ReasonCode, decision.Reason, baseOutput)
		}
	} else if workflowConfigBool(config, "requireApprovedActivity", false) && !activityApprovalValid(runtime.Activity) {
		return matchNodeNoMatch("APPROVAL_REQUIRED", "activity version is not approved", baseOutput)
	}
	if entryAllowed, entryRuleResult := activityEntryAllowed(runtime.Activity, event); !entryAllowed {
		return matchNodeNoMatch(entryRuleResult.ReasonCode, entryRuleResult.Reason, baseOutput)
	}
	matchedActivity := map[string]any{
		"activityId":     runtime.Activity.ID,
		"activityCode":   runtime.Activity.Code,
		"name":           runtime.Activity.Name,
		"status":         runtime.Activity.Status,
		"tenantId":       runtime.Activity.TenantID,
		"matched":        true,
		"priority":       runtime.Activity.Priority,
		"exclusiveGroup": runtime.Activity.ExclusiveGroup,
		"stackable":      runtime.Activity.Stackable,
		"triggerEvents":  runtime.Activity.TriggerEvents,
		"matchReason":    "tenant/eventType/status/timeWindow/entry matched",
	}
	baseOutput["matched_activity"] = matchedActivity
	baseOutput["matched"] = true
	baseOutput["matchReason"] = "tenant/eventType/status/timeWindow/entry matched"
	return nodeSuccess(baseOutput)
}

func matchNodeNoMatch(code string, reason string, output map[string]any) workflowNodeResult {
	output["matched"] = false
	output["errorCode"] = code
	output["reason"] = reason
	output["matched_activity"] = map[string]any{
		"matched":    false,
		"reasonCode": code,
		"reason":     reason,
	}
	switch strings.ToUpper(strings.TrimSpace(fmt.Sprint(output["fallbackAction"]))) {
	case "NO_MATCH_SKIP":
		return nodeSkipped(code+": "+reason, output)
	case "USE_DEFAULT_ACTIVITY":
		output["fallbackUsed"] = true
		return nodeSuccess(output)
	default:
		return nodeFailed(code+": "+reason, output)
	}
}

func factQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	return nodeFailed("FACT_QUERY_SERVICE_REQUIRED: fact-query requires Service.factQueryWorkflowNodeHandler", map[string]any{
		"errorCode":       "FACT_QUERY_SERVICE_REQUIRED",
		"executionMode":   "SERVICE_HANDLER_REQUIRED",
		"executionIntent": "real fact provider query",
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func (s *Service) factQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	if chain := workflowFactQueryProviderChain(config); len(chain) > 0 {
		return s.factQueryProviderChainWorkflowNodeHandler(ctx, node, runtime, config, chain)
	}
	if strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef")) != "" {
		return s.factQueryResourceRefWorkflowNodeHandler(ctx, node, runtime, config)
	}
	source := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "source"), "FACT_SERVICE"))
	if source != "FACT_SERVICE" && source != "HTTP_PROVIDER" {
		return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_SOURCE_UNSUPPORTED", "fact-query source "+source+" is not supported by workflow executor", map[string]any{}, nil)
	}
	refs := workflowFactQueryKeys(config, runtime)
	if len(refs) == 0 {
		return nodeSkipped("FACT_QUERY_NO_KEYS: no fact references", map[string]any{
			"errorCode": "FACT_QUERY_NO_KEYS",
			"factCount": 0,
			"facts":     map[string]any{},
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	responses := make([]any, 0, len(refs))
	for _, factCode := range refs {
		queryCtx := ctx
		var cancel context.CancelFunc
		if timeoutSeconds := workflowConfigNumber(config, "timeoutSeconds"); timeoutSeconds > 0 {
			queryCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
		}
		if cancel != nil {
			defer cancel()
		}
		request := marketing.FactQueryRequest{
			EventID:      runtime.Inbox.Event.EventID,
			EventType:    runtime.Inbox.Event.EventType,
			SourceSystem: runtime.Inbox.Event.SourceSystem,
			TenantID:     runtime.Activity.TenantID,
			FactCode:     factCode,
			Subject:      workflowFactQuerySubject(ctx, config, runtime),
			Biz:          runtime.Inbox.Event.Biz,
			OccurredAt:   runtime.Inbox.Event.OccurredAt,
			Attributes:   runtime.Inbox.Event.Attributes,
			TraceID:      runtime.Inbox.Event.EventID,
		}
		response, err := s.queryFactsWithNodeProvider(queryCtx, runtime, config, factCode, request)
		if err != nil {
			return workflowFactQueryFailure(node, runtime, config, factCode, "FACT_QUERY_FAILED", err.Error(), facts, responses)
		}
		responses = append(responses, workflowJSONValue(response))
		if !response.Success {
			return workflowFactQueryFailure(node, runtime, config, factCode, response.ErrorCode, response.Error, facts, responses)
		}
		if value, ok := response.Facts[factCode]; ok {
			facts[factCode] = value
		} else {
			for key, value := range response.Facts {
				facts[key] = value
			}
		}
	}
	return nodeSuccess(map[string]any{
		"factCount":       len(refs),
		"factKeys":        refs,
		"facts":           facts,
		"responses":       responses,
		"source":          source,
		"subject":         workflowJSONValue(workflowFactQuerySubject(ctx, config, runtime)),
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func (s *Service) factQueryResourceRefWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult {
	resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef"))
	resource, err := s.GetResource(ctx, runtime.Activity.TenantID, resourceRef)
	if err != nil {
		return workflowFactQueryFailure(node, runtime, config, "", "RESOURCE_NOT_FOUND", err.Error(), map[string]any{}, nil)
	}
	refs := workflowFactQueryKeys(config, runtime)
	if len(refs) == 0 {
		return nodeSkipped("FACT_QUERY_NO_KEYS: no fact references", map[string]any{
			"errorCode":   "FACT_QUERY_NO_KEYS",
			"factCount":   0,
			"facts":       map[string]any{},
			"resourceRef": resourceRef,
			"audit":       workflowHandlerAudit(node, runtime),
		})
	}
	if workflowResourceHasBuiltinProfileDataset(resource, config, refs) {
		return s.factQueryBuiltinProfileResourceWorkflowNodeHandler(ctx, node, runtime, config, resource, refs)
	}
	facts := map[string]any{}
	responses := make([]any, 0, len(refs))
	for _, factCode := range refs {
		keyTemplate := firstWorkflowString(workflowConfigString(config, "keyTemplate"), workflowConfigString(resource.ConfigMap(), "keyTemplate"))
		if keyTemplate == "" {
			keyTemplate = "${tenantId}:${subject.id}:${factCode}"
		}
		key := workflowRenderFactQueryTemplate(keyTemplate, runtime, node, factCode)
		operation := workflowFactQueryResourceOperation(resource, config)
		valueType := firstWorkflowString(workflowConfigString(config, "valueType"), "OBJECT")
		output, err := s.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
			Operation:   operation,
			ResourceRef: resourceRef,
			Namespace:   firstWorkflowString(workflowConfigString(config, "namespace"), "facts"),
			Key:         key,
			Value:       workflowFactQueryResourcePayload(ctx, config, runtime, factCode),
			ValueType:   valueType,
			Options: map[string]any{
				"factCode":       factCode,
				"valueType":      valueType,
				"timeoutSeconds": workflowConfigNumber(config, "timeoutSeconds"),
			},
		})
		if err != nil {
			responses = append(responses, output)
			return workflowFactQueryFailure(node, runtime, config, factCode, workflowConfigString(output, "errorCode"), err.Error(), facts, responses)
		}
		responses = append(responses, output)
		value, ok := workflowFactValueFromResourceOutput(output, factCode, workflowConfigString(config, "valuePath"))
		if !ok {
			return workflowFactQueryFailure(node, runtime, config, factCode, "FACT_QUERY_RESOURCE_VALUE_NOT_FOUND", "resource response did not contain requested fact", facts, responses)
		}
		facts[factCode] = value
	}
	return nodeSuccess(map[string]any{
		"factCount":       len(facts),
		"factKeys":        refs,
		"facts":           facts,
		"responses":       responses,
		"source":          "RESOURCE",
		"resourceRef":     resourceRef,
		"resource":        map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver},
		"subject":         workflowJSONValue(workflowFactQuerySubject(ctx, config, runtime)),
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func workflowResourceHasBuiltinProfileDataset(resource marketing.Resource, config map[string]any, refs []string) bool {
	if len(refs) != 1 {
		return false
	}
	factCode := strings.ToUpper(strings.TrimSpace(refs[0]))
	resourceConfig := resource.ConfigMap()
	dataset := workflowResourceDatasetConfig(resourceConfig, factCode)
	table := firstNonEmpty(
		workflowConfigString(config, "physicalTable"),
		workflowConfigString(config, "table"),
		workflowConfigString(config, "tableNamespace"),
		workflowConfigString(resourceConfig, "physicalTable"),
		workflowConfigString(resourceConfig, "table"),
		workflowConfigString(resourceConfig, "tableNamespace"),
		workflowConfigString(dataset, "physicalTable"),
		workflowConfigString(dataset, "table"),
	)
	adapter := firstNonEmpty(workflowConfigString(config, "adapter"), workflowConfigString(resourceConfig, "adapter"), workflowConfigString(dataset, "adapter"))
	switch factCode {
	case "USER_PROFILE":
		return resource.Code == defaultUserProfileStore ||
			(strings.EqualFold(adapter, "USER_PROFILE") && strings.EqualFold(table, "marketing_user_profile"))
	case "KYC_PROFILE":
		return resource.Code == defaultKYCProfileStore ||
			(strings.EqualFold(adapter, "KYC_PROFILE") && strings.EqualFold(table, "marketing_kyc_profile"))
	case "RISK_PROFILE":
		return resource.Code == defaultRiskProfileStore ||
			(strings.EqualFold(adapter, "RISK_PROFILE") && strings.EqualFold(table, "marketing_risk_profile"))
	default:
		return false
	}
}

func workflowResourceDatasetConfig(resourceConfig map[string]any, factCode string) map[string]any {
	datasets, ok := workflowJSONValue(resourceConfig["datasets"]).(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := datasets[factCode]
	if !ok {
		raw, ok = datasets[strings.ToUpper(factCode)]
	}
	if !ok {
		return nil
	}
	mapped, _ := workflowJSONValue(raw).(map[string]any)
	return mapped
}

func (s *Service) factQueryBuiltinProfileResourceWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, resource marketing.Resource, refs []string) workflowNodeResult {
	facts := map[string]any{}
	responses := make([]any, 0, len(refs))
	namespace := firstWorkflowString(workflowConfigString(config, "namespace"), "facts")
	subject := workflowFactQuerySubject(ctx, config, runtime)
	for _, factCode := range refs {
		factCode = strings.ToUpper(strings.TrimSpace(factCode))
		if factCode != "USER_PROFILE" && factCode != "KYC_PROFILE" && factCode != "RISK_PROFILE" {
			return workflowFactQueryFailure(node, runtime, config, factCode, "FACT_QUERY_RESOURCE_FACT_UNSUPPORTED", "default profile resource only supports USER_PROFILE, KYC_PROFILE or RISK_PROFILE", facts, responses)
		}
		resourceConfig := resource.ConfigMap()
		dataset := workflowResourceDatasetConfig(resourceConfig, factCode)
		keyTemplate := firstNonEmpty(
			workflowConfigString(config, "keyTemplate"),
			workflowConfigString(dataset, "keyTemplate"),
			workflowConfigString(resourceConfig, "keyTemplate"),
		)
		if keyTemplate == "" {
			keyTemplate = "${tenantId}:${subject.type}:${subject.id}"
		}
		key := workflowRenderFactQueryTemplate(keyTemplate, runtime, node, factCode)
		accessReq := marketing.ResourceAccessRequest{
			TenantID:    runtime.Activity.TenantID,
			WorkspaceID: runtime.Activity.WorkspaceID,
			FolderID:    runtime.Activity.FolderID,
			ActivityID:  runtime.Activity.ID,
			NodeID:      node.ID,
			NodeType:    nodeHandlerKey(node),
			Operation:   marketing.ResourceRead,
			Namespace:   namespace,
			ResourceKey: key,
		}
		if err := s.ValidateResourceAccess(ctx, resource, accessReq); err != nil {
			code := firstWorkflowString(strings.TrimSpace(err.Error()), "RESOURCE_PERMISSION_DENIED")
			return workflowFactQueryFailure(node, runtime, config, factCode, code, code+": resource access denied", facts, responses)
		}
		if code := resourceAdapterConfigError(resource); code != "" {
			return workflowFactQueryFailure(node, runtime, config, factCode, code, code+": resource adapter config is invalid", facts, responses)
		}
		value, err := s.workflowProfileFact(ctx, factCode, runtime.Activity.TenantID, subject)
		response := map[string]any{
			"success":     err == nil,
			"factCode":    factCode,
			"resourceRef": resource.Code,
			"resourceKey": key,
			"subject":     workflowJSONValue(subject),
		}
		if err != nil {
			response["error"] = err.Error()
			responses = append(responses, response)
			result := workflowFactQueryFailure(node, runtime, config, factCode, "FACT_QUERY_"+factCode+"_FAILED", err.Error(), facts, responses)
			_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, marketing.ResourceRead, result)
			return result
		}
		response["facts"] = map[string]any{factCode: value}
		responses = append(responses, response)
		facts[factCode] = value
	}
	result := nodeSuccess(map[string]any{
		"factCount":       len(facts),
		"factKeys":        refs,
		"facts":           facts,
		"responses":       responses,
		"source":          "RESOURCE",
		"resourceRef":     resource.Code,
		"resource":        map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver},
		"subject":         workflowJSONValue(subject),
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	})
	_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, marketing.ResourceRead, result)
	return result
}

func (s *Service) workflowProfileFact(ctx context.Context, factCode string, tenantID string, subject marketing.EventSubject) (map[string]any, error) {
	switch strings.ToUpper(strings.TrimSpace(factCode)) {
	case "KYC_PROFILE":
		return s.GetKYCProfileFact(ctx, tenantID, subject)
	case "RISK_PROFILE":
		return s.GetRiskProfileFact(ctx, tenantID, subject)
	default:
		return s.GetUserProfileFact(ctx, tenantID, subject)
	}
}

func workflowFactQueryResourceOperation(resource marketing.Resource, config map[string]any) marketing.ResourceOperation {
	if operation := strings.ToUpper(workflowConfigString(config, "operation")); operation != "" {
		return workflowNormalizeResourceAPIOperation(marketing.ResourceOperation(operation))
	}
	switch resource.Type {
	case marketing.ResourceHTTPProvider, marketing.ResourceGRPCProvider, marketing.ResourcePluginProvider, marketing.ResourceCustomProvider:
		return marketing.ResourceCall
	default:
		return marketing.ResourceRead
	}
}

func workflowRenderFactQueryTemplate(template string, runtime workflowRuntime, node marketing.WorkflowNode, factCode string) string {
	rendered := workflowRenderNodeTemplate(template, runtime, node)
	rendered = strings.ReplaceAll(rendered, "${factCode}", factCode)
	rendered = strings.ReplaceAll(rendered, "${factCode.lower}", strings.ToLower(factCode))
	return rendered
}

func workflowFactQueryResourcePayload(ctx context.Context, config map[string]any, runtime workflowRuntime, factCode string) map[string]any {
	return map[string]any{
		"tenantId":   runtime.Activity.TenantID,
		"activityId": runtime.Activity.ID,
		"eventId":    runtime.Inbox.Event.EventID,
		"eventType":  runtime.Inbox.Event.EventType,
		"factCode":   factCode,
		"subject":    workflowJSONValue(workflowFactQuerySubject(ctx, config, runtime)),
		"biz":        workflowJSONValue(runtime.Inbox.Event.Biz),
		"attributes": workflowJSONValue(runtime.Inbox.Event.Attributes),
	}
}

func workflowFactValueFromResourceOutput(output map[string]any, factCode string, valuePath string) (any, bool) {
	if strings.TrimSpace(valuePath) != "" {
		if value, err := extractWorkflowJSONPath(output, valuePath); err == nil {
			return value, true
		}
	}
	if facts, ok := workflowJSONValue(output["facts"]).(map[string]any); ok {
		if value, ok := facts[factCode]; ok {
			return value, true
		}
	}
	for _, key := range []string{"stored_value", "value", "data"} {
		if value, ok := output[key]; ok && value != nil {
			normalized := workflowJSONValue(value)
			if mapped, ok := normalized.(map[string]any); ok {
				if facts, ok := mapped["facts"].(map[string]any); ok {
					if factValue, ok := facts[factCode]; ok {
						return factValue, true
					}
				}
				if factValue, ok := mapped[factCode]; ok {
					return factValue, true
				}
				if data, ok := mapped["data"].(map[string]any); ok {
					if factValue, ok := data[factCode]; ok {
						return factValue, true
					}
				}
			}
			return normalized, true
		}
	}
	return nil, false
}

type workflowFactQueryProviderStep struct {
	Source         string
	ResourceRef    string
	Namespace      string
	Operation      string
	SourcePath     string
	CachePath      string
	KeyTemplate    string
	ValuePath      string
	Endpoint       string
	Method         string
	ProviderCode   string
	RedisRef       string
	GRPCTarget     string
	PluginCode     string
	TimeoutSeconds float64
	OnMiss         string
	Raw            map[string]any
}

func workflowFactQueryProviderChain(config map[string]any) []workflowFactQueryProviderStep {
	value, ok := config["providerChain"]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		steps := make([]workflowFactQueryProviderStep, 0, len(typed))
		for _, item := range typed {
			if source := strings.ToUpper(strings.TrimSpace(item)); source != "" {
				steps = append(steps, workflowFactQueryProviderStep{Source: source, OnMiss: "NEXT"})
			}
		}
		return steps
	case []any:
		steps := make([]workflowFactQueryProviderStep, 0, len(typed))
		for _, item := range typed {
			switch normalized := workflowJSONValue(item).(type) {
			case map[string]any:
				step := workflowFactQueryProviderStep{
					Source:         firstWorkflowString(strings.ToUpper(workflowConfigString(normalized, "source")), strings.ToUpper(workflowConfigString(normalized, "provider"))),
					ResourceRef:    workflowConfigString(normalized, "resourceRef"),
					Namespace:      workflowConfigString(normalized, "namespace"),
					Operation:      strings.ToUpper(workflowConfigString(normalized, "operation")),
					SourcePath:     workflowConfigString(normalized, "sourcePath"),
					CachePath:      workflowConfigString(normalized, "cachePath"),
					KeyTemplate:    workflowConfigString(normalized, "keyTemplate"),
					ValuePath:      workflowConfigString(normalized, "valuePath"),
					Endpoint:       workflowConfigString(normalized, "endpoint"),
					Method:         workflowConfigString(normalized, "method"),
					ProviderCode:   workflowConfigString(normalized, "providerCode"),
					RedisRef:       workflowConfigString(normalized, "redisRef"),
					GRPCTarget:     workflowConfigString(normalized, "grpcTarget"),
					PluginCode:     workflowConfigString(normalized, "pluginCode"),
					TimeoutSeconds: workflowConfigNumber(normalized, "timeoutSeconds"),
					OnMiss:         strings.ToUpper(firstWorkflowString(workflowConfigString(normalized, "onMiss"), "NEXT")),
					Raw:            normalized,
				}
				if step.Source == "" && step.ResourceRef != "" {
					step.Source = "RESOURCE"
				}
				if step.Source != "" {
					steps = append(steps, step)
				}
			default:
				if source := strings.ToUpper(strings.TrimSpace(fmt.Sprint(normalized))); source != "" {
					steps = append(steps, workflowFactQueryProviderStep{Source: source, OnMiss: "NEXT"})
				}
			}
		}
		return steps
	case string:
		legacy := workflowConfigStringList(config, "providerChain")
		steps := make([]workflowFactQueryProviderStep, 0, len(legacy))
		for _, source := range legacy {
			steps = append(steps, workflowFactQueryProviderStep{Source: strings.ToUpper(source), OnMiss: "NEXT"})
		}
		return steps
	default:
		return nil
	}
}

func workflowFactQueryProviderStepConfig(config map[string]any, step workflowFactQueryProviderStep, remaining []string) map[string]any {
	next := map[string]any{}
	for key, value := range config {
		next[key] = value
	}
	if step.Raw != nil {
		for key, value := range step.Raw {
			if key == "source" || key == "provider" || key == "onMiss" {
				continue
			}
			next[key] = value
		}
	}
	next["factKeys"] = remaining
	if step.ResourceRef != "" {
		next["resourceRef"] = step.ResourceRef
	}
	if step.Namespace != "" {
		next["namespace"] = step.Namespace
	}
	if step.Operation != "" {
		next["operation"] = step.Operation
	}
	if step.SourcePath != "" {
		next["sourcePath"] = step.SourcePath
	}
	if step.CachePath != "" {
		next["cachePath"] = step.CachePath
	}
	if step.KeyTemplate != "" {
		next["keyTemplate"] = step.KeyTemplate
	}
	if step.ValuePath != "" {
		next["valuePath"] = step.ValuePath
	}
	if step.Endpoint != "" {
		next["endpoint"] = step.Endpoint
		next["providerEndpoint"] = step.Endpoint
	}
	if step.Method != "" {
		next["method"] = step.Method
		next["httpMethod"] = step.Method
	}
	if step.ProviderCode != "" {
		next["providerCode"] = step.ProviderCode
	}
	if step.RedisRef != "" {
		next["redisRef"] = step.RedisRef
	}
	if step.GRPCTarget != "" {
		next["grpcTarget"] = step.GRPCTarget
	}
	if step.PluginCode != "" {
		next["pluginCode"] = step.PluginCode
	}
	if step.TimeoutSeconds > 0 {
		next["timeoutSeconds"] = step.TimeoutSeconds
	}
	return next
}

func workflowFactQueryProviderStepNode(node marketing.WorkflowNode, config map[string]any) marketing.WorkflowNode {
	node.Config = workflowRawJSON(config)
	return node
}

func workflowFactQueryProviderStepTrace(step workflowFactQueryProviderStep, status string, remaining []string, output any) map[string]any {
	trace := map[string]any{
		"source":    step.Source,
		"status":    status,
		"remaining": remaining,
		"onMiss":    firstWorkflowString(step.OnMiss, "NEXT"),
	}
	if step.ResourceRef != "" {
		trace["resourceRef"] = step.ResourceRef
	}
	if step.Namespace != "" {
		trace["namespace"] = step.Namespace
	}
	if step.KeyTemplate != "" {
		trace["keyTemplate"] = step.KeyTemplate
	}
	if output != nil {
		trace["output"] = output
	}
	return trace
}

func workflowFactQueryProviderStepHasFacts(output map[string]any) bool {
	facts, _ := workflowJSONValue(output["facts"]).(map[string]any)
	return len(facts) > 0
}

func workflowFactQueryProviderShouldBlockOnMiss(step workflowFactQueryProviderStep) bool {
	return strings.EqualFold(strings.TrimSpace(step.OnMiss), "BLOCK")
}

func workflowFactQueryProviderHasMiss(remaining []string, facts map[string]any) bool {
	return len(workflowRemainingFactKeys(remaining, facts)) > 0
}

func (s *Service) factQueryProviderChainWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, chain []workflowFactQueryProviderStep) workflowNodeResult {
	refs := workflowFactQueryKeys(config, runtime)
	if len(refs) == 0 {
		return nodeSkipped("FACT_QUERY_NO_KEYS: no fact references", map[string]any{
			"errorCode": "FACT_QUERY_NO_KEYS",
			"factCount": 0,
			"facts":     map[string]any{},
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	responses := make([]any, 0, len(chain))
	remaining := append([]string(nil), refs...)
	resolvedChain := make([]any, 0, len(chain))
	for _, step := range chain {
		provider := strings.ToUpper(strings.TrimSpace(step.Source))
		if provider == "" || len(remaining) == 0 {
			continue
		}
		step.Source = provider
		stepConfig := workflowFactQueryProviderStepConfig(config, step, remaining)
		stepNode := workflowFactQueryProviderStepNode(node, stepConfig)
		switch provider {
		case "WORKFLOW_CONTEXT", "CONTEXT":
			result := contextFactQuery(ctx, stepNode, runtime, stepConfig, remaining)
			responses = append(responses, result.response)
			stepFacts := map[string]any{}
			for key, value := range result.facts {
				stepFacts[key] = value
				facts[key] = value
			}
			status := "DONE"
			if workflowFactQueryProviderHasMiss(remaining, stepFacts) {
				status = "MISS"
			}
			resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, status, remaining, result.response))
			if status == "MISS" && workflowFactQueryProviderShouldBlockOnMiss(step) {
				return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_PROVIDER_MISS", "providerChain step did not resolve requested facts", facts, responses)
			}
		case "LOCAL_CACHE", "CACHE":
			result := localCacheFactQuery(ctx, stepNode, runtime, stepConfig, remaining)
			responses = append(responses, result.response)
			stepFacts := map[string]any{}
			for key, value := range result.facts {
				stepFacts[key] = value
				facts[key] = value
			}
			status := "DONE"
			if workflowFactQueryProviderHasMiss(remaining, stepFacts) {
				status = "MISS"
			}
			resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, status, remaining, result.response))
			if status == "MISS" && workflowFactQueryProviderShouldBlockOnMiss(step) {
				return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_PROVIDER_MISS", "providerChain step did not resolve requested facts", facts, responses)
			}
		case "RESOURCE":
			resourceResult := s.factQueryResourceRefWorkflowNodeHandler(ctx, stepNode, runtime, stepConfig)
			responses = append(responses, resourceResult.Output)
			if resourceResult.Status == marketing.WorkflowNodeFailed {
				return resourceResult
			}
			if mapped, ok := resourceResult.Output["facts"].(map[string]any); ok {
				for key, value := range mapped {
					facts[key] = value
				}
			}
			status := "DONE"
			if !workflowFactQueryProviderStepHasFacts(resourceResult.Output) {
				status = "MISS"
			}
			resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, status, remaining, resourceResult.Output))
			if status == "MISS" && workflowFactQueryProviderShouldBlockOnMiss(step) {
				return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_PROVIDER_MISS", "providerChain step did not resolve requested facts", facts, responses)
			}
		case "FACT_SERVICE", "HTTP_PROVIDER", "HTTP":
			if strings.TrimSpace(workflowConfigString(stepConfig, "resourceRef")) != "" {
				resourceResult := s.factQueryResourceRefWorkflowNodeHandler(ctx, stepNode, runtime, stepConfig)
				responses = append(responses, resourceResult.Output)
				if resourceResult.Status == marketing.WorkflowNodeFailed {
					return resourceResult
				}
				if mapped, ok := resourceResult.Output["facts"].(map[string]any); ok {
					for key, value := range mapped {
						facts[key] = value
					}
				}
				resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, "DONE", remaining, resourceResult.Output))
				break
			}
			httpResult := s.executeHTTPFactQuery(ctx, stepNode, runtime, stepConfig)
			if httpResult.Status == marketing.WorkflowNodeFailed {
				if !workflowFactQueryProviderShouldBlockOnMiss(step) {
					resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, "FAILED_NEXT", remaining, httpResult.Output))
					break
				}
				return httpResult
			}
			responses = append(responses, httpResult.Output["responses"])
			if mapped, ok := httpResult.Output["facts"].(map[string]any); ok {
				for key, value := range mapped {
					facts[key] = value
				}
			}
			resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, "DONE", remaining, httpResult.Output))
		default:
			if strings.TrimSpace(workflowConfigString(stepConfig, "resourceRef")) != "" {
				resourceResult := s.factQueryResourceRefWorkflowNodeHandler(ctx, stepNode, runtime, stepConfig)
				responses = append(responses, resourceResult.Output)
				if resourceResult.Status == marketing.WorkflowNodeFailed {
					return resourceResult
				}
				if mapped, ok := resourceResult.Output["facts"].(map[string]any); ok {
					for key, value := range mapped {
						facts[key] = value
					}
				}
				status := "DONE"
				if !workflowFactQueryProviderStepHasFacts(resourceResult.Output) {
					status = "MISS"
				}
				resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, status, remaining, resourceResult.Output))
				if status == "MISS" && workflowFactQueryProviderShouldBlockOnMiss(step) {
					return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_PROVIDER_MISS", "providerChain step did not resolve requested facts", facts, responses)
				}
				break
			}
			response := map[string]any{"provider": provider, "status": "ADAPTER_REQUIRED", "remaining": remaining}
			responses = append(responses, response)
			resolvedChain = append(resolvedChain, workflowFactQueryProviderStepTrace(step, "ADAPTER_REQUIRED", remaining, response))
		}
		remaining = workflowRemainingFactKeys(refs, facts)
	}
	if len(facts) == 0 || len(remaining) > 0 {
		return workflowFactQueryFailure(node, runtime, config, "", "FACT_QUERY_NO_PROVIDER_HIT", "providerChain did not resolve any requested facts", facts, responses)
	}
	return nodeSuccess(map[string]any{
		"factCount":       len(facts),
		"factKeys":        refs,
		"facts":           facts,
		"responses":       responses,
		"providerChain":   resolvedChain,
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func workflowRemainingFactKeys(refs []string, facts map[string]any) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := facts[ref]; !ok {
			out = append(out, ref)
		}
	}
	return out
}

type workflowFactQueryPartialResult struct {
	facts    map[string]any
	response map[string]any
}

func contextFactQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	refs := workflowFactQueryKeys(config, runtime)
	result := contextFactQuery(ctx, node, runtime, config, refs)
	if len(result.facts) == 0 {
		return workflowFactQueryFailure(node, runtime, config, "", "CONTEXT_FACT_NOT_FOUND", "context-query did not resolve any requested facts", map[string]any{}, []any{result.response})
	}
	return nodeSuccess(workflowFactOutput(node, runtime, "WORKFLOW_CONTEXT", result.facts, []any{result.response}))
}

func contextFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, refs []string) workflowFactQueryPartialResult {
	base := workflowFactQueryBaseContext(ctx, runtime)
	facts := map[string]any{}
	resolved := []any{}
	for _, factCode := range refs {
		if value, path, ok := workflowFactValueFromPathsForCode(base, workflowFactQuerySourcePaths(config, factCode), factCode, len(refs) > 1); ok {
			facts[factCode] = value
			resolved = append(resolved, map[string]any{"factCode": factCode, "path": path, "hit": true})
		} else {
			resolved = append(resolved, map[string]any{"factCode": factCode, "hit": false})
		}
	}
	return workflowFactQueryPartialResult{facts: facts, response: map[string]any{
		"provider": "WORKFLOW_CONTEXT",
		"status":   "SUCCESS",
		"resolved": resolved,
		"audit":    workflowHandlerAudit(node, runtime),
	}}
}

func localCacheFactQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	refs := workflowFactQueryKeys(config, runtime)
	result := localCacheFactQuery(ctx, node, runtime, config, refs)
	if len(result.facts) == 0 {
		return workflowFactQueryFailure(node, runtime, config, "", "LOCAL_CACHE_FACT_NOT_FOUND", "local-cache-query did not resolve any requested facts", map[string]any{}, []any{result.response})
	}
	return nodeSuccess(workflowFactOutput(node, runtime, "LOCAL_CACHE", result.facts, []any{result.response}))
}

func localCacheFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, refs []string) workflowFactQueryPartialResult {
	execCtx := workflowNodeContext(ctx)
	cacheRoot := map[string]any{"vars": execCtx.Variables}
	if facts, ok := workflowJSONValue(workflowGetVar(execCtx.Variables, "execution", "facts")).(map[string]any); ok {
		cacheRoot["facts"] = facts
	}
	facts := map[string]any{}
	resolved := []any{}
	for _, factCode := range refs {
		paths := workflowConfigStringList(config, "cachePaths")
		if len(paths) == 0 {
			if path := workflowConfigString(config, "cachePath"); path != "" {
				paths = []string{path}
			}
		}
		if len(paths) == 0 {
			paths = []string{"$.facts." + factCode, "$.vars.facts." + factCode, "$.vars.execution.facts." + factCode}
		}
		if value, path, ok := workflowFactValueFromPathsForCode(cacheRoot, paths, factCode, len(refs) > 1); ok {
			facts[factCode] = value
			resolved = append(resolved, map[string]any{"factCode": factCode, "path": path, "hit": true})
		} else {
			resolved = append(resolved, map[string]any{"factCode": factCode, "hit": false})
		}
	}
	return workflowFactQueryPartialResult{facts: facts, response: map[string]any{
		"provider": "LOCAL_CACHE",
		"status":   "SUCCESS",
		"resolved": resolved,
		"audit":    workflowHandlerAudit(node, runtime),
	}}
}

func (s *Service) httpFactQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	return s.executeHTTPFactQuery(ctx, node, runtime, config)
}

func (s *Service) executeHTTPFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult {
	if config == nil {
		config = map[string]any{}
	}
	config["source"] = firstWorkflowString(workflowConfigString(config, "source"), "HTTP_PROVIDER")
	refs := workflowFactQueryKeys(config, runtime)
	if len(refs) == 0 {
		return nodeSkipped("FACT_QUERY_NO_KEYS: no fact references", map[string]any{
			"errorCode": "FACT_QUERY_NO_KEYS",
			"factCount": 0,
			"facts":     map[string]any{},
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	responses := make([]any, 0, len(refs))
	for _, factCode := range refs {
		queryCtx := ctx
		var cancel context.CancelFunc
		if timeoutSeconds := workflowConfigNumber(config, "timeoutSeconds"); timeoutSeconds > 0 {
			queryCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
		}
		if cancel != nil {
			defer cancel()
		}
		request := marketing.FactQueryRequest{
			EventID:      runtime.Inbox.Event.EventID,
			EventType:    runtime.Inbox.Event.EventType,
			SourceSystem: runtime.Inbox.Event.SourceSystem,
			TenantID:     runtime.Activity.TenantID,
			FactCode:     factCode,
			Subject:      workflowFactQuerySubject(ctx, config, runtime),
			Biz:          runtime.Inbox.Event.Biz,
			OccurredAt:   runtime.Inbox.Event.OccurredAt,
			Attributes:   runtime.Inbox.Event.Attributes,
			TraceID:      runtime.Inbox.Event.EventID,
		}
		response, err := s.queryFactsWithNodeProvider(queryCtx, runtime, config, factCode, request)
		if err != nil {
			return workflowFactQueryFailure(node, runtime, config, factCode, "FACT_QUERY_FAILED", err.Error(), facts, responses)
		}
		responses = append(responses, workflowJSONValue(response))
		if !response.Success {
			return workflowFactQueryFailure(node, runtime, config, factCode, response.ErrorCode, response.Error, facts, responses)
		}
		if value, ok := response.Facts[factCode]; ok {
			facts[factCode] = value
		} else {
			for key, value := range response.Facts {
				facts[key] = value
			}
		}
	}
	return nodeSuccess(map[string]any{
		"factCount":       len(refs),
		"factKeys":        refs,
		"facts":           facts,
		"responses":       responses,
		"source":          strings.ToUpper(workflowConfigString(config, "source")),
		"subject":         workflowJSONValue(workflowFactQuerySubject(ctx, config, runtime)),
		"cacheTtlSeconds": workflowConfigNumber(config, "cacheTtlSeconds"),
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func (s *Service) queryFactsWithNodeProvider(ctx context.Context, runtime workflowRuntime, config map[string]any, factCode string, request marketing.FactQueryRequest) (marketing.FactQueryResponse, error) {
	if provider, ok := workflowInlineHTTPFactProvider(runtime, config, factCode); ok {
		return s.factClient.Query(ctx, provider, request), nil
	}
	return s.QueryFacts(ctx, request)
}

func workflowInlineHTTPFactProvider(runtime workflowRuntime, config map[string]any, factCode string) (marketing.FactProvider, bool) {
	endpoint := workflowConfigString(config, "endpoint")
	if endpoint == "" {
		endpoint = workflowConfigString(config, "url")
	}
	if endpoint == "" {
		return marketing.FactProvider{}, false
	}
	mapping := workflowFactMappingConfig(config)
	timeoutMS := int(workflowConfigNumber(config, "timeoutMs"))
	if timeoutMS == 0 {
		timeoutMS = int(workflowConfigNumber(config, "timeoutSeconds") * 1000)
	}
	return marketing.FactProvider{
		TenantID:        runtime.Activity.TenantID,
		FactCode:        factCode,
		ProviderType:    "HTTP",
		Endpoint:        endpoint,
		Method:          firstWorkflowString(workflowConfigString(config, "method"), "POST"),
		AuthRef:         workflowConfigString(config, "authRef"),
		Mapping:         mapping,
		CacheTTLSeconds: int(workflowConfigNumber(config, "cacheTtlSeconds")),
		Enabled:         true,
		TimeoutMS:       timeoutMS,
	}, true
}

func workflowFactMappingConfig(config map[string]any) map[string]marketing.FactMapping {
	raw := config["mapping"]
	if raw == nil {
		raw = config["responseMapping"]
	}
	if raw == nil {
		raw = config["mappingJson"]
	}
	if raw == nil {
		return nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	mapping := map[string]marketing.FactMapping{}
	if err := json.Unmarshal(bytes, &mapping); err != nil {
		return nil
	}
	return mapping
}

func factAdapterRequiredWorkflowNodeHandler(provider string, adapter string, realExecution bool) workflowNodeHandler {
	return func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		config, failed, ok := workflowRequireNodeConfig(node)
		if !ok {
			failed.Output["audit"] = workflowHandlerAudit(node, runtime)
			return failed
		}
		output := map[string]any{
			"errorCode":          "FACT_PROVIDER_ADAPTER_REQUIRED",
			"provider":           provider,
			"adapter":            adapter,
			"executionMode":      map[bool]string{true: "REAL_HANDLER_REQUIRED", false: "EXPLAIN_NODE"}[realExecution],
			"executionAuthority": adapter,
			"factKeys":           workflowFactQueryKeys(config, runtime),
			"facts":              map[string]any{},
			"audit":              workflowHandlerAudit(node, runtime),
		}
		if strings.EqualFold(workflowConfigString(config, "failureAction"), "SKIP") || !realExecution {
			return nodeSkipped("FACT_PROVIDER_ADAPTER_REQUIRED: "+provider+" adapter is not registered", output)
		}
		return nodeFailed("FACT_PROVIDER_ADAPTER_REQUIRED: "+provider+" adapter is not registered", output)
	}
}

func luaFactQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	source := workflowConfigString(config, "source")
	if source == "" {
		return nodeFailed("LUA_QUERY_SOURCE_REQUIRED: source is required", map[string]any{
			"errorCode": "LUA_QUERY_SOURCE_REQUIRED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	luaOutput, err := executeWorkflowLua(ctx, node, runtime, source)
	if err != nil {
		return nodeFailed("LUA_QUERY_EXECUTION_FAILED: "+err.Error(), map[string]any{
			"errorCode": "LUA_QUERY_EXECUTION_FAILED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	if mapped, ok := workflowJSONValue(luaOutput["facts"]).(map[string]any); ok {
		for key, value := range mapped {
			facts[strings.ToUpper(strings.TrimSpace(key))] = value
		}
	}
	if len(facts) == 0 {
		if factCode := strings.ToUpper(workflowConfigString(config, "factCode")); factCode != "" {
			if value, ok := luaOutput["value"]; ok {
				facts[factCode] = value
			}
		}
	}
	if len(facts) == 0 {
		return workflowFactQueryFailure(node, runtime, config, "", "LUA_QUERY_EMPTY_FACTS", "lua-query must emit facts or value with factCode", map[string]any{}, []any{luaOutput})
	}
	return nodeSuccess(workflowFactOutput(node, runtime, "LUA_QUERY", facts, []any{luaOutput}))
}

func (s *Service) luaFactQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	source := workflowConfigString(config, "source")
	if source == "" {
		return nodeFailed("LUA_QUERY_SOURCE_REQUIRED: source is required", map[string]any{
			"errorCode": "LUA_QUERY_SOURCE_REQUIRED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	luaOutput, err := s.executeWorkflowLua(ctx, node, runtime, source)
	if err != nil {
		return nodeFailed("LUA_QUERY_EXECUTION_FAILED: "+err.Error(), map[string]any{
			"errorCode": "LUA_QUERY_EXECUTION_FAILED",
			"error":     err.Error(),
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	if mapped, ok := workflowJSONValue(luaOutput["facts"]).(map[string]any); ok {
		for key, value := range mapped {
			facts[key] = value
		}
	} else if factCode := strings.ToUpper(workflowConfigString(config, "factCode")); factCode != "" {
		if value, ok := luaOutput[factCode]; ok {
			facts[factCode] = value
		} else {
			if value, ok := luaOutput["value"]; ok {
				facts[factCode] = value
			}
		}
	}
	if len(facts) == 0 {
		return workflowFactQueryFailure(node, runtime, config, "", "LUA_QUERY_EMPTY_FACTS", "lua-query must emit facts or value with factCode", map[string]any{}, []any{luaOutput})
	}
	return nodeSuccess(workflowFactOutput(node, runtime, "LUA_QUERY", facts, []any{luaOutput}))
}

func (s *Service) activityStateWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	storeType := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "storeType"), "INTERNAL"))
	if storeType != "INTERNAL" {
		return nodeFailed("STATE_PROVIDER_ADAPTER_REQUIRED: custom activity state provider is not registered", map[string]any{
			"errorCode":          "STATE_PROVIDER_ADAPTER_REQUIRED",
			"storeType":          storeType,
			"providerCode":       workflowConfigString(config, "providerCode"),
			"executionMode":      "CUSTOM_PROVIDER_REQUIRED",
			"executionAuthority": "ActivityStateProvider",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
	operation := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "operation"), "READ"))
	stateKey := workflowRenderTemplate(firstWorkflowString(workflowConfigString(config, "stateKeyTemplate"), "${activityId}:${subject.id}"), runtime)
	namespace := firstWorkflowString(workflowConfigString(config, "namespace"), "activity")
	key := marketing.ActivityStateKey{
		TenantID:   runtime.Activity.TenantID,
		ActivityID: runtime.Activity.ID,
		Namespace:  namespace,
		StateKey:   stateKey,
	}
	switch operation {
	case "READ":
		state, err := s.repository.GetActivityState(ctx, key)
		if err != nil {
			if errors.Is(err, marketing.ErrNotFound) {
				return workflowActivityStateMissing(node, runtime, config, key)
			}
			return nodeFailed("ACTIVITY_STATE_READ_FAILED: "+err.Error(), map[string]any{
				"errorCode": "ACTIVITY_STATE_READ_FAILED",
				"error":     err.Error(),
				"audit":     workflowHandlerAudit(node, runtime),
			})
		}
		return nodeSuccess(workflowActivityStateOutput(node, runtime, operation, state, true))
	case "WRITE", "UPSERT", "CAS":
		if operation == "WRITE" {
			if _, err := s.repository.GetActivityState(ctx, key); err != nil {
				if errors.Is(err, marketing.ErrNotFound) {
					return nodeFailed("ACTIVITY_STATE_NOT_FOUND: state does not exist for WRITE", map[string]any{
						"errorCode": "ACTIVITY_STATE_NOT_FOUND",
						"audit":     workflowHandlerAudit(node, runtime),
					})
				}
				return nodeFailed("ACTIVITY_STATE_READ_FAILED: "+err.Error(), map[string]any{"errorCode": "ACTIVITY_STATE_READ_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
			}
		}
		data := workflowActivityStatePatch(ctx, config, runtime)
		now := time.Now().UTC()
		requireVersion := operation == "CAS" || workflowConfigBool(config, "requireVersion", false)
		expectedVersion := workflowActivityStateExpectedVersion(ctx)
		state, err := s.repository.UpsertActivityState(ctx, marketing.UpsertActivityStateCommand{
			State: marketing.ActivityState{
				ID:          marketing.NewID("actstate"),
				TenantID:    runtime.Activity.TenantID,
				ActivityID:  runtime.Activity.ID,
				Namespace:   namespace,
				StateKey:    stateKey,
				Subject:     runtime.Inbox.Event.Subject,
				Data:        data,
				LastEventID: runtime.Inbox.Event.EventID,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			ExpectedVersion: expectedVersion,
			RequireVersion:  requireVersion && expectedVersion > 0,
			Merge:           operation == "UPSERT" || operation == "CAS",
		})
		if err != nil {
			code := "ACTIVITY_STATE_WRITE_FAILED"
			if errors.Is(err, marketing.ErrActivityStateConflict) {
				code = "ACTIVITY_STATE_VERSION_CONFLICT"
			}
			return nodeFailed(code+": "+err.Error(), map[string]any{
				"errorCode":       code,
				"expectedVersion": expectedVersion,
				"error":           err.Error(),
				"audit":           workflowHandlerAudit(node, runtime),
			})
		}
		return nodeSuccess(workflowActivityStateOutput(node, runtime, operation, state, true))
	default:
		return nodeFailed("ACTIVITY_STATE_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{
			"errorCode": "ACTIVITY_STATE_OPERATION_UNSUPPORTED",
			"operation": operation,
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
}

func workflowActivityStateMissing(node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, key marketing.ActivityStateKey) workflowNodeResult {
	emptyState := marketing.ActivityState{
		TenantID:    key.TenantID,
		ActivityID:  key.ActivityID,
		Namespace:   key.Namespace,
		StateKey:    key.StateKey,
		Subject:     runtime.Inbox.Event.Subject,
		Data:        json.RawMessage(`{}`),
		Version:     0,
		LastEventID: runtime.Inbox.Event.EventID,
	}
	output := workflowActivityStateOutput(node, runtime, "READ", emptyState, false)
	output["errorCode"] = "ACTIVITY_STATE_NOT_FOUND"
	switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "missingPolicy"), "EMPTY_STATE")) {
	case "BLOCK":
		return nodeFailed("ACTIVITY_STATE_NOT_FOUND: state does not exist", output)
	case "CREATE":
		output["missingPolicy"] = "CREATE"
		output["createRequired"] = true
		return nodeSkipped("ACTIVITY_STATE_CREATE_REQUIRED: state does not exist", output)
	default:
		output["missingPolicy"] = "EMPTY_STATE"
		return nodeSuccess(output)
	}
}

func workflowActivityStateOutput(node marketing.WorkflowNode, runtime workflowRuntime, operation string, state marketing.ActivityState, exists bool) map[string]any {
	var data any = map[string]any{}
	if len(state.Data) > 0 {
		if err := json.Unmarshal(state.Data, &data); err != nil {
			data = string(state.Data)
		}
	}
	activityState := map[string]any{
		"id":          state.ID,
		"tenantId":    state.TenantID,
		"activityId":  state.ActivityID,
		"namespace":   state.Namespace,
		"stateKey":    state.StateKey,
		"subject":     workflowJSONValue(state.Subject),
		"exists":      exists,
		"data":        data,
		"version":     state.Version,
		"lastEventId": state.LastEventID,
	}
	if !state.CreatedAt.IsZero() {
		activityState["createdAt"] = state.CreatedAt
	}
	if !state.UpdatedAt.IsZero() {
		activityState["updatedAt"] = state.UpdatedAt
	}
	if state.ExpiresAt != nil {
		activityState["expiresAt"] = state.ExpiresAt
	}
	return map[string]any{
		"activity_state":     activityState,
		"state_version":      state.Version,
		"operation":          operation,
		"storeType":          "INTERNAL",
		"executionMode":      "REAL_HANDLER",
		"executionAuthority": "WorkflowExecutor/ActivityStateStore",
		"audit":              workflowHandlerAudit(node, runtime),
	}
}

func workflowActivityStatePatch(ctx context.Context, config map[string]any, runtime workflowRuntime) json.RawMessage {
	if value, ok := workflowResolvedInput(ctx, "state_patch"); ok {
		return workflowRawJSONObject(value)
	}
	valuePath := workflowConfigString(config, "valuePath")
	if valuePath != "" && valuePath != "$.state" {
		base := workflowFactQueryBaseContext(ctx, runtime)
		if value, err := extractWorkflowJSONPath(base, valuePath); err == nil {
			return workflowRawJSONObject(value)
		}
	}
	if value, ok := workflowUpstreamOutput(ctx, "plugin_result"); ok {
		return workflowRawJSONObject(value)
	}
	if value, ok := workflowResolvedInput(ctx, "state"); ok {
		return workflowRawJSONObject(value)
	}
	return json.RawMessage(`{}`)
}

func workflowRawJSONObject(value any) json.RawMessage {
	raw, err := json.Marshal(workflowJSONValue(value))
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`)
	}
	return raw
}

func workflowActivityStateExpectedVersion(ctx context.Context) int64 {
	if value, ok := workflowResolvedInput(ctx, "state_version"); ok {
		return workflowInt64(value)
	}
	if value, ok := workflowResolvedInput(ctx, "activity_state"); ok {
		if version, ok := workflowActivityStateVersion(value); ok {
			return version
		}
	}
	if value, ok := workflowUpstreamOutput(ctx, "state_version"); ok {
		return workflowInt64(value)
	}
	if value, ok := workflowUpstreamOutput(ctx, "activity_state"); ok {
		if version, ok := workflowActivityStateVersion(value); ok {
			return version
		}
	}
	return 0
}

func workflowStorageStateExpectedVersion(ctx context.Context) int64 {
	if value, ok := workflowResolvedInput(ctx, "storage_version"); ok {
		return workflowInt64(value)
	}
	if value, ok := workflowResolvedInput(ctx, "storage_entry"); ok {
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			return workflowInt64(mapped["version"])
		}
	}
	if value, ok := workflowUpstreamOutput(ctx, "storage_version"); ok {
		return workflowInt64(value)
	}
	if value, ok := workflowUpstreamOutput(ctx, "storage_entry"); ok {
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			return workflowInt64(mapped["version"])
		}
	}
	return 0
}

func workflowActivityStateVersion(value any) (int64, bool) {
	mapped, ok := workflowJSONValue(value).(map[string]any)
	if !ok {
		return 0, false
	}
	version, ok := mapped["version"]
	if !ok {
		return 0, false
	}
	return workflowInt64(version), true
}

func workflowInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		number, _ := typed.Int64()
		return number
	case string:
		number, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return number
	default:
		return 0
	}
}

func (s *Service) stateStoreWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	storageType := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "storageType"), "MEMORY"))
	operation := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "operation"), "DEFINE"))
	casOperation := operation == "CAS"
	if casOperation {
		config["requireVersion"] = true
	}
	namespace := firstWorkflowString(workflowConfigString(config, "namespace"), "workflow")
	storageKey := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "keyTemplate"), "${activityId}:${eventId}:${nodeId}"), runtime, node)
	valueType := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "valueType"), "OBJECT"))
	resource, hasResource, resourceResult := s.workflowStateStoreResourceConfig(ctx, node, runtime, config, marketing.ResourceOperation(operation), namespace, storageKey)
	if resourceResult.Status != "" {
		return resourceResult
	}
	storageOperation := operation
	if casOperation {
		expectedVersion := workflowInt64(workflowConfigString(config, "expectedVersion"))
		if expectedVersion == 0 {
			expectedVersion = workflowStorageStateExpectedVersion(ctx)
		}
		if expectedVersion == 0 {
			result := nodeFailed("STATE_STORE_VERSION_REQUIRED: CAS requires expectedVersion or upstream storage_version", map[string]any{
				"errorCode":   "STATE_STORE_VERSION_REQUIRED",
				"operation":   operation,
				"storageType": storageType,
				"audit":       workflowHandlerAudit(node, runtime),
			})
			if hasResource {
				if result.Output == nil {
					result.Output = map[string]any{}
				}
				result.Output["resourceRef"] = resource.Code
				result.Output["resource"] = map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver}
				_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, marketing.ResourceCAS, result)
			}
			return result
		}
		config["expectedVersion"] = fmt.Sprint(expectedVersion)
		storageOperation = "UPSERT"
	}
	storageType = strings.ToUpper(firstWorkflowString(workflowConfigString(config, "storageType"), storageType))
	var result workflowNodeResult
	switch storageType {
	case "MEMORY", "WORKFLOW_CONTEXT", "LOCAL_CACHE":
		result = workflowMemoryStateStore(ctx, node, runtime, config, storageOperation, namespace, storageKey, valueType)
	case "DATABASE":
		result = s.workflowDatabaseStateStore(ctx, node, runtime, config, storageOperation, namespace, storageKey, valueType)
	case "DATABASE_TABLE", "TABLE":
		result = s.workflowDatabaseTableStateStore(ctx, node, runtime, config, storageOperation, namespace, storageKey, valueType)
	case "HTTP", "REST", "EXTERNAL":
		result = s.workflowHTTPStateStore(ctx, node, runtime, config, storageOperation, namespace, storageKey, valueType)
	case "REDIS", "REDIS_KV", "REDIS_HASH", "REDIS_COUNTER", "REDIS_LOCK":
		if hasResource {
			result = s.workflowRedisStateStore(ctx, node, runtime, resource, config, storageOperation, namespace, storageKey, valueType)
		} else {
			result = nodeFailed("STATE_STORE_ADAPTER_REQUIRED: REDIS storage adapter requires resourceRef", map[string]any{
				"errorCode":          "STATE_STORE_ADAPTER_REQUIRED",
				"storageType":        storageType,
				"providerCode":       workflowConfigString(config, "providerCode"),
				"executionMode":      "RESOURCE_REF_REQUIRED",
				"executionAuthority": "RedisResourceStateStore",
				"audit":              workflowHandlerAudit(node, runtime),
			})
		}
	case "CUSTOM", "PLUGIN", "GRPC":
		result = nodeFailed("STATE_STORE_ADAPTER_REQUIRED: "+storageType+" storage adapter is not registered", map[string]any{
			"errorCode":          "STATE_STORE_ADAPTER_REQUIRED",
			"storageType":        storageType,
			"providerCode":       workflowConfigString(config, "providerCode"),
			"executionMode":      "ADAPTER_REQUIRED",
			"executionAuthority": storageType + "StateStore",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	default:
		result = nodeFailed("STATE_STORE_TYPE_UNSUPPORTED: storageType is not supported", map[string]any{
			"errorCode":   "STATE_STORE_TYPE_UNSUPPORTED",
			"storageType": storageType,
			"audit":       workflowHandlerAudit(node, runtime),
		})
	}
	if hasResource {
		if result.Output == nil {
			result.Output = map[string]any{}
		}
		result.Output["resourceRef"] = resource.Code
		result.Output["resource"] = map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver}
		_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, marketing.ResourceOperation(operation), result)
	}
	return result
}

func workflowStateStoreResourceOperation(operation marketing.ResourceOperation) marketing.ResourceOperation {
	switch strings.ToUpper(string(operation)) {
	case "DEFINE":
		return marketing.ResourceWrite
	case "INCREMENT", "CHECK_AND_INCREMENT":
		return marketing.ResourceCheckAndIncrement
	default:
		return operation
	}
}

func (s *Service) workflowStateStoreResourceConfig(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation marketing.ResourceOperation, namespace string, storageKey string) (marketing.Resource, bool, workflowNodeResult) {
	resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef"))
	if resourceRef == "" {
		return marketing.Resource{}, false, workflowNodeResult{}
	}
	operation = workflowStateStoreResourceOperation(operation)
	resource, err := s.GetResource(ctx, runtime.Activity.TenantID, resourceRef)
	if err != nil {
		code := "RESOURCE_NOT_FOUND"
		return marketing.Resource{}, true, nodeFailed(code+": "+err.Error(), map[string]any{"errorCode": code, "resourceRef": resourceRef, "audit": workflowHandlerAudit(node, runtime)})
	}
	req := marketing.ResourceAccessRequest{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		FolderID:    runtime.Activity.FolderID,
		ActivityID:  runtime.Activity.ID,
		NodeID:      node.ID,
		NodeType:    nodeHandlerKey(node),
		Operation:   operation,
		Namespace:   namespace,
		ResourceKey: storageKey,
	}
	if err := s.ValidateResourceAccess(ctx, resource, req); err != nil {
		code := strings.TrimSpace(err.Error())
		if code == "" {
			code = "RESOURCE_PERMISSION_DENIED"
		}
		return resource, true, nodeFailed(code+": resource access denied", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
	}
	if code := resourceAdapterConfigError(resource); code != "" {
		return resource, true, nodeFailed(code+": resource adapter config is invalid", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
	}
	if workflowResourceApprovalRequired(resource, operation, config, runtime) {
		return resource, true, nodeFailed("RESOURCE_APPROVAL_REQUIRED: high risk resource operation requires approval", map[string]any{
			"errorCode":    "RESOURCE_APPROVAL_REQUIRED",
			"resourceRef":  resourceRef,
			"resourceType": resource.Type,
			"operation":    operation,
			"audit":        workflowHandlerAudit(node, runtime),
		})
	}
	resourceConfig := resource.ConfigMap()
	for key, value := range resourceConfig {
		if _, exists := config[key]; !exists {
			config[key] = value
		}
	}
	workflowNormalizeProviderResourceConfig(node, resource, resourceConfig, config)
	config["storageType"] = workflowStorageTypeForResource(resource)
	return resource, true, workflowNodeResult{}
}

func workflowNormalizeProviderResourceConfig(node marketing.WorkflowNode, resource marketing.Resource, resourceConfig map[string]any, config map[string]any) {
	if resource.Type != marketing.ResourceHTTPProvider {
		return
	}
	if endpoint := firstWorkflowString(workflowConfigString(resourceConfig, "endpoint"), workflowConfigString(config, "endpoint")); endpoint != "" && !workflowExplicitConfigHasKey(node, "providerEndpoint") {
		config["providerEndpoint"] = endpoint
	}
	if method := firstWorkflowString(workflowConfigString(resourceConfig, "method"), firstWorkflowString(workflowConfigString(resourceConfig, "httpMethod"), workflowConfigString(config, "method"))); method != "" && !workflowExplicitConfigHasKey(node, "httpMethod") {
		config["httpMethod"] = method
	}
	if !workflowExplicitConfigHasKey(node, "requestHeaders") {
		if headers, ok := resourceConfig["headers"]; ok {
			config["requestHeaders"] = headers
		} else if headers, ok := config["headers"]; ok {
			config["requestHeaders"] = headers
		}
	}
	if timeoutSeconds := workflowConfigNumber(resourceConfig, "timeoutSeconds"); timeoutSeconds > 0 && !workflowExplicitConfigHasKey(node, "timeoutSeconds") {
		config["timeoutSeconds"] = timeoutSeconds
	}
}

func workflowExplicitConfigString(node marketing.WorkflowNode, key string) string {
	if len(node.Config) == 0 {
		return ""
	}
	var config map[string]any
	if err := json.Unmarshal(node.Config, &config); err != nil {
		return ""
	}
	return workflowConfigString(config, key)
}

func workflowExplicitConfigHasKey(node marketing.WorkflowNode, key string) bool {
	if len(node.Config) == 0 {
		return false
	}
	var config map[string]any
	if err := json.Unmarshal(node.Config, &config); err != nil || config == nil {
		return false
	}
	value, exists := config[key]
	return exists && !workflowConfigValueEmpty(value)
}

func workflowStorageTypeForResource(resource marketing.Resource) string {
	switch resource.Type {
	case marketing.ResourceMemory, marketing.ResourceWorkflowContext:
		return "MEMORY"
	case marketing.ResourceDatabaseKV:
		return "DATABASE"
	case marketing.ResourceDatabase:
		return "DATABASE"
	case marketing.ResourceDatabaseTable:
		return "DATABASE_TABLE"
	case marketing.ResourceHTTPProvider:
		return "HTTP"
	case marketing.ResourceRedisKV, marketing.ResourceRedisHash, marketing.ResourceRedisCounter, marketing.ResourceRedisLock:
		return "REDIS"
	case marketing.ResourceGRPCProvider:
		return "GRPC"
	case marketing.ResourcePluginProvider:
		return "PLUGIN"
	default:
		return "CUSTOM"
	}
}

func (s *Service) workflowRecordResourceAudit(ctx context.Context, resource marketing.Resource, runtime workflowRuntime, node marketing.WorkflowNode, operation marketing.ResourceOperation, result workflowNodeResult) error {
	status := marketing.ResourceAuditSuccess
	errorCode := ""
	errorMessage := ""
	if result.Status != marketing.WorkflowNodeSuccess {
		status = marketing.ResourceAuditFailed
		errorMessage = result.Error
		if result.Output != nil {
			errorCode = workflowConfigString(result.Output, "errorCode")
		}
	}
	return s.repository.CreateResourceAuditLog(ctx, marketing.ResourceAuditLog{
		ID:           marketing.NewID("resaudit"),
		TenantID:     runtime.Activity.TenantID,
		WorkspaceID:  runtime.Activity.WorkspaceID,
		FolderID:     runtime.Activity.FolderID,
		ActivityID:   runtime.Activity.ID,
		NodeID:       node.ID,
		NodeType:     nodeHandlerKey(node),
		ResourceID:   resource.ID,
		ResourceCode: resource.Code,
		Operation:    operation,
		Status:       status,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		CreatedAt:    time.Now().UTC(),
	})
}

type workflowResourceAPIRequest struct {
	Operation   marketing.ResourceOperation
	ResourceRef string
	Namespace   string
	Key         string
	Value       any
	ValueType   string
	Options     map[string]any
}

func (s *Service) workflowExecuteResourceAPI(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, req workflowResourceAPIRequest) (map[string]any, error) {
	resourceRef := strings.TrimSpace(req.ResourceRef)
	if resourceRef == "" {
		output := map[string]any{"errorCode": "RESOURCE_REF_REQUIRED", "audit": workflowHandlerAudit(node, runtime)}
		return output, errors.New("RESOURCE_REF_REQUIRED: resourceRef is required")
	}
	operation := workflowNormalizeResourceAPIOperation(req.Operation)
	resource, err := s.GetResource(ctx, runtime.Activity.TenantID, resourceRef)
	if err != nil {
		output := map[string]any{"errorCode": "RESOURCE_NOT_FOUND", "resourceRef": resourceRef, "audit": workflowHandlerAudit(node, runtime)}
		return output, fmt.Errorf("RESOURCE_NOT_FOUND: %w", err)
	}
	namespace := firstWorkflowString(req.Namespace, "workflow")
	key := workflowRenderNodeTemplate(firstWorkflowString(req.Key, "${activityId}:${eventId}:${nodeId}"), runtime, node)
	accessOperation := operation
	if accessOperation == marketing.ResourceCall && resource.Type == marketing.ResourceMQTopic {
		accessOperation = marketing.ResourcePublish
	}
	accessOperation = workflowStateStoreResourceOperation(accessOperation)
	accessReq := marketing.ResourceAccessRequest{
		TenantID:    runtime.Activity.TenantID,
		WorkspaceID: runtime.Activity.WorkspaceID,
		FolderID:    runtime.Activity.FolderID,
		ActivityID:  runtime.Activity.ID,
		NodeID:      node.ID,
		NodeType:    nodeHandlerKey(node),
		Operation:   accessOperation,
		Namespace:   namespace,
		ResourceKey: key,
	}
	if err := s.ValidateResourceAccess(ctx, resource, accessReq); err != nil {
		code := firstWorkflowString(strings.TrimSpace(err.Error()), "RESOURCE_PERMISSION_DENIED")
		output := map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)}
		return output, fmt.Errorf("%s: resource access denied", code)
	}
	if code := resourceAdapterConfigError(resource); code != "" {
		output := map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)}
		return output, fmt.Errorf("%s: resource adapter config is invalid", code)
	}
	if workflowResourceApprovalRequired(resource, accessOperation, req.Options, runtime) {
		output := map[string]any{"errorCode": "RESOURCE_APPROVAL_REQUIRED", "resourceRef": resourceRef, "resourceType": resource.Type, "operation": accessOperation, "audit": workflowHandlerAudit(node, runtime)}
		return output, errors.New("RESOURCE_APPROVAL_REQUIRED: high risk resource operation requires approval")
	}

	var result workflowNodeResult
	auditRecorded := false
	switch resource.Type {
	case marketing.ResourceMQTopic:
		if accessOperation != marketing.ResourcePublish && accessOperation != marketing.ResourceCall {
			output := map[string]any{"errorCode": "RESOURCE_OPERATION_UNSUPPORTED", "resourceRef": resourceRef, "operation": accessOperation, "audit": workflowHandlerAudit(node, runtime)}
			return output, errors.New("RESOURCE_OPERATION_UNSUPPORTED: MQ_TOPIC only supports PUBLISH/CALL")
		}
		result = nodeSuccess(workflowMQTopicPublishOutput(node, runtime, resource, namespace, key, req.Value))
	case marketing.ResourceMemory, marketing.ResourceWorkflowContext, marketing.ResourceDatabaseKV, marketing.ResourceDatabaseTable, marketing.ResourceHTTPProvider, marketing.ResourceRedisKV, marketing.ResourceRedisHash, marketing.ResourceRedisCounter, marketing.ResourceRedisLock:
		stateOperation := string(operation)
		if operation == marketing.ResourceCall {
			stateOperation = "CALL"
		}
		tempConfig := map[string]any{
			"resourceRef":  resource.Code,
			"operation":    stateOperation,
			"namespace":    namespace,
			"keyTemplate":  key,
			"valueType":    firstWorkflowString(req.ValueType, "OBJECT"),
			"defaultValue": req.Value,
		}
		for optionKey, optionValue := range req.Options {
			tempConfig[optionKey] = optionValue
		}
		configBytes, _ := json.Marshal(tempConfig)
		tempNode := node
		tempNode.Config = configBytes
		execCtx := workflowNodeContext(ctx)
		if execCtx.ResolvedInput == nil {
			execCtx.ResolvedInput = map[string]any{}
		}
		execCtx.ResolvedInput["value"] = req.Value
		result = s.stateStoreWorkflowNodeHandler(context.WithValue(ctx, workflowNodeExecutionContextKey{}, execCtx), tempNode, runtime)
		auditRecorded = true
	default:
		output := map[string]any{"errorCode": "RESOURCE_TYPE_UNSUPPORTED", "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)}
		return output, errors.New("RESOURCE_TYPE_UNSUPPORTED: resource type is not supported by workflow resource API")
	}
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	result.Output["resourceRef"] = resource.Code
	result.Output["resource"] = map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver}
	if !auditRecorded {
		_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, accessOperation, result)
	}
	if result.Status != marketing.WorkflowNodeSuccess {
		return result.Output, errors.New(result.Error)
	}
	return result.Output, nil
}

func workflowNormalizeResourceAPIOperation(operation marketing.ResourceOperation) marketing.ResourceOperation {
	switch strings.ToUpper(strings.TrimSpace(string(operation))) {
	case "", "READ":
		return marketing.ResourceRead
	case "WRITE":
		return marketing.ResourceWrite
	case "DEFINE":
		return marketing.ResourceWrite
	case "UPSERT":
		return marketing.ResourceUpsert
	case "APPEND":
		return marketing.ResourceAppend
	case "DELETE":
		return marketing.ResourceDelete
	case "CALL":
		return marketing.ResourceCall
	case "PUBLISH":
		return marketing.ResourcePublish
	case "INCREMENT", "CHECK_AND_INCREMENT":
		return marketing.ResourceCheckAndIncrement
	default:
		return operation
	}
}

func workflowResourceApprovalRequired(resource marketing.Resource, operation marketing.ResourceOperation, options map[string]any, runtime workflowRuntime) bool {
	if !workflowConfigBool(resource.ConfigMap(), "requireApproval", false) {
		return false
	}
	switch operation {
	case marketing.ResourceDelete, marketing.ResourceCall, marketing.ResourcePublish:
		if workflowConfigBool(resource.ConfigMap(), "requireFormalApproval", false) {
			return !activityApprovalValid(runtime.Activity)
		}
		if workflowConfigBool(options, "approved", false) || strings.TrimSpace(workflowConfigString(options, "approvalId")) != "" {
			return false
		}
		return true
	default:
		return false
	}
}

func workflowMQTopicPublishOutput(node marketing.WorkflowNode, runtime workflowRuntime, resource marketing.Resource, namespace string, key string, payload any) map[string]any {
	config := resource.ConfigMap()
	provider := strings.ToUpper(firstWorkflowString(resourceConfigString(config, "provider"), firstWorkflowString(resourceConfigString(config, "broker"), "KAFKA")))
	destination := firstWorkflowString(resourceConfigString(config, "destination"), firstWorkflowString(resourceConfigString(config, "targetRef"), resourceConfigString(config, "topic")))
	return map[string]any{
		"status":             "PUBLISH_ACCEPTED",
		"executionMode":      "RESOURCE_PUBLISH_INTENT",
		"executionAuthority": "MQTopicResourceAdapter/OutboxWorker",
		"provider":           provider,
		"destination":        destination,
		"namespace":          namespace,
		"key":                key,
		"payload":            workflowJSONValue(payload),
		"eventId":            runtime.Inbox.Event.EventID,
		"activityId":         runtime.Activity.ID,
		"audit":              workflowHandlerAudit(node, runtime),
	}
}

func (s *Service) workflowHTTPStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation string, namespace string, storageKey string, valueType string) workflowNodeResult {
	endpoint := strings.TrimSpace(workflowConfigString(config, "providerEndpoint"))
	if endpoint == "" {
		endpoint = strings.TrimSpace(workflowConfigString(config, "endpoint"))
	}
	if endpoint == "" {
		return nodeFailed("STATE_STORE_PROVIDER_REQUIRED: HTTP storage endpoint is required", map[string]any{"errorCode": "STATE_STORE_PROVIDER_REQUIRED", "storageType": "HTTP", "audit": workflowHandlerAudit(node, runtime)})
	}
	endpoint = workflowRenderNodeTemplate(endpoint, runtime, node)
	if err := workflowValidateHTTPProviderEndpoint(endpoint, config); err != nil {
		return nodeFailed("STATE_STORE_HTTP_ENDPOINT_DENIED: "+err.Error(), map[string]any{
			"errorCode": "STATE_STORE_HTTP_ENDPOINT_DENIED",
			"error":     err.Error(),
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	value := workflowStateStoreValue(ctx, config, runtime, valueType)
	expectedVersion := workflowInt64(workflowConfigString(config, "expectedVersion"))
	if expectedVersion == 0 {
		expectedVersion = workflowStorageStateExpectedVersion(ctx)
	}
	requestBody := map[string]any{
		"operation":       operation,
		"tenantId":        runtime.Activity.TenantID,
		"activityId":      runtime.Activity.ID,
		"eventId":         runtime.Inbox.Event.EventID,
		"namespace":       namespace,
		"key":             storageKey,
		"valueType":       valueType,
		"value":           workflowJSONValue(value),
		"expectedVersion": expectedVersion,
		"requireVersion":  workflowConfigBool(config, "requireVersion", false) && expectedVersion > 0,
		"providerCode":    workflowConfigString(config, "providerCode"),
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nodeFailed("STATE_STORE_HTTP_REQUEST_INVALID: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_HTTP_REQUEST_INVALID", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
	}
	timeoutSeconds := workflowConfigNumber(config, "timeoutSeconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
	defer cancel()
	method := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "httpMethod"), http.MethodPost))
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nodeFailed("STATE_STORE_HTTP_REQUEST_INVALID: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_HTTP_REQUEST_INVALID", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
	}
	req.Header.Set("Content-Type", "application/json")
	if err := workflowApplyHTTPProviderHeaders(req, config); err != nil {
		return nodeFailed("STATE_STORE_HTTP_SECRET_UNRESOLVED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_HTTP_SECRET_UNRESOLVED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
	}
	client := workflowHTTPProviderClient(config)
	resp, err := client.Do(req)
	if err != nil {
		return nodeFailed("STATE_STORE_HTTP_FAILED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_HTTP_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nodeFailed("STATE_STORE_HTTP_FAILED: provider returned non-2xx", map[string]any{"errorCode": "STATE_STORE_HTTP_FAILED", "statusCode": resp.StatusCode, "response": string(respBody), "audit": workflowHandlerAudit(node, runtime)})
	}
	output, schemaErr := workflowHTTPStateStoreOutput(node, runtime, config, operation, namespace, storageKey, valueType, respBody)
	if schemaErr != nil {
		return nodeFailed("RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID: "+schemaErr.Error(), map[string]any{"errorCode": "RESOURCE_HTTP_RESPONSE_SCHEMA_INVALID", "error": schemaErr.Error(), "audit": workflowHandlerAudit(node, runtime)})
	}
	return nodeSuccess(output)
}

func workflowHTTPStateStoreOutput(node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation string, namespace string, storageKey string, valueType string, body []byte) (map[string]any, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return workflowStateStoreOutput(node, runtime, "HTTP", operation, namespace, storageKey, valueType, nil, 0, operation != "READ" && operation != "DELETE"), nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return workflowStateStoreOutput(node, runtime, "HTTP", operation, namespace, storageKey, "STRING", string(body), 0, true), nil
	}
	if err := workflowValidateHTTPResponseSchema(config, decoded); err != nil {
		return nil, err
	}
	mapped, _ := workflowJSONValue(decoded).(map[string]any)
	if mapped == nil {
		return workflowStateStoreOutput(node, runtime, "HTTP", operation, namespace, storageKey, valueType, decoded, 0, true), nil
	}
	value := firstWorkflowAny(mapped["stored_value"], mapped["value"], mapped["data"])
	version := workflowInt64(firstWorkflowAny(mapped["storage_version"], mapped["version"]))
	exists := true
	if rawExists, ok := mapped["exists"]; ok {
		exists = workflowConfigBool(map[string]any{"exists": rawExists}, "exists", true)
	}
	if entry, ok := mapped["storage_entry"].(map[string]any); ok {
		if value == nil {
			value = firstWorkflowAny(entry["value"], entry["data"])
		}
		if version == 0 {
			version = workflowInt64(entry["version"])
		}
		if rawExists, ok := entry["exists"]; ok {
			exists = workflowConfigBool(map[string]any{"exists": rawExists}, "exists", exists)
		}
	}
	output := workflowStateStoreOutput(node, runtime, "HTTP", operation, namespace, storageKey, valueType, value, version, exists)
	output["provider_response"] = workflowJSONValue(decoded)
	return output, nil
}

func workflowValidateHTTPResponseSchema(config map[string]any, decoded any) error {
	raw, ok := config["responseSchema"]
	if !ok || raw == nil {
		return nil
	}
	schema, ok := workflowJSONValue(raw).(map[string]any)
	if !ok {
		return errors.New("responseSchema must be object")
	}
	for _, key := range []string{"required", "requiredPaths"} {
		for _, path := range resourceConfigStringList(schema, key) {
			value, err := extractWorkflowJSONPath(decoded, path)
			if err != nil || workflowDatabaseTableValueEmpty(value) {
				return fmt.Errorf("required response path %s is missing", path)
			}
		}
	}
	if fields, ok := workflowJSONValue(schema["fields"]).(map[string]any); ok {
		for path, rawType := range fields {
			value, err := extractWorkflowJSONPath(decoded, path)
			if err != nil {
				return fmt.Errorf("response path %s is missing", path)
			}
			if !workflowDatabaseTableColumnTypeValid(value, strings.ToUpper(fmt.Sprint(rawType))) {
				return fmt.Errorf("response path %s must be %s", path, rawType)
			}
		}
	}
	return nil
}

func workflowValidateHTTPProviderEndpoint(endpoint string, config map[string]any) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("endpoint scheme must be http or https")
	}
	if parsed.User != nil {
		return errors.New("endpoint must not contain userinfo")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return errors.New("endpoint host is required")
	}
	if blocked := workflowConfigStringList(config, "blockedHosts"); workflowHTTPHostBlocked(host, blocked) {
		return fmt.Errorf("endpoint host %s is blocked", host)
	}
	allowedHosts := workflowConfigStringList(config, "allowedHosts")
	allowedSuffixes := workflowConfigStringList(config, "allowedHostSuffixes")
	if len(allowedHosts) > 0 || len(allowedSuffixes) > 0 {
		if !workflowHTTPHostAllowed(host, allowedHosts, allowedSuffixes) {
			return fmt.Errorf("endpoint host %s is not in allowlist", host)
		}
	}
	if workflowConfigBool(config, "blockPrivateNetwork", false) {
		private, err := workflowHTTPHostIsPrivate(host)
		if err != nil {
			return err
		}
		if private {
			return fmt.Errorf("endpoint host %s is private or local", host)
		}
	}
	return nil
}

func workflowHTTPProviderClient(config map[string]any) *http.Client {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if !workflowConfigBool(config, "blockPrivateNetwork", false) {
		return client
	}
	dialer := &net.Dialer{
		ControlContext: func(ctx context.Context, network string, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			ip := net.ParseIP(host)
			if ip == nil {
				private, privateErr := workflowHTTPHostIsPrivate(host)
				if privateErr != nil {
					return privateErr
				}
				if private {
					return fmt.Errorf("endpoint dial address %s is private or local", host)
				}
				return nil
			}
			if workflowIPIsPrivateOrLocal(ip) {
				return fmt.Errorf("endpoint dial address %s is private or local", host)
			}
			return nil
		},
	}
	client.Transport = &http.Transport{DialContext: dialer.DialContext}
	return client
}

func workflowHTTPHostBlocked(host string, blockedHosts []string) bool {
	for _, blocked := range blockedHosts {
		if strings.EqualFold(strings.TrimSpace(blocked), host) {
			return true
		}
	}
	return false
}

func workflowHTTPHostAllowed(host string, allowedHosts []string, allowedSuffixes []string) bool {
	for _, allowed := range allowedHosts {
		normalized := strings.ToLower(strings.TrimSpace(allowed))
		if normalized != "" && host == normalized {
			return true
		}
	}
	for _, suffix := range allowedSuffixes {
		normalized := strings.ToLower(strings.TrimSpace(suffix))
		if normalized == "" {
			continue
		}
		if strings.HasPrefix(normalized, ".") {
			if strings.HasSuffix(host, normalized) || host == strings.TrimPrefix(normalized, ".") {
				return true
			}
			continue
		}
		if host == normalized || strings.HasSuffix(host, "."+normalized) {
			return true
		}
	}
	return false
}

func workflowHTTPHostIsPrivate(host string) (bool, error) {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true, nil
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return workflowIPIsPrivateOrLocal(ip), nil
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return false, fmt.Errorf("endpoint host %s could not be resolved for private network validation: %w", host, err)
	}
	if len(addresses) == 0 {
		return false, fmt.Errorf("endpoint host %s resolved no addresses", host)
	}
	for _, address := range addresses {
		if workflowIPIsPrivateOrLocal(address.IP) {
			return true, nil
		}
	}
	return false, nil
}

func workflowIPIsPrivateOrLocal(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func workflowMemoryStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation string, namespace string, storageKey string, valueType string) workflowNodeResult {
	execCtx := workflowNodeContext(ctx)
	if execCtx.Variables == nil {
		return nodeFailed("STATE_STORE_MEMORY_UNAVAILABLE: workflow variables are not available", map[string]any{"errorCode": "STATE_STORE_MEMORY_UNAVAILABLE", "audit": workflowHandlerAudit(node, runtime)})
	}
	compositeKey := namespace + ":" + storageKey
	current := workflowGetVar(execCtx.Variables, "storage", compositeKey)
	switch operation {
	case "READ":
		if current == nil {
			return workflowStateStoreMissing(node, runtime, config, "MEMORY", namespace, storageKey, valueType)
		}
		return nodeSuccess(workflowStateStoreOutput(node, runtime, "MEMORY", operation, namespace, storageKey, valueType, current, 1, true))
	case "DELETE":
		if storage, ok := execCtx.Variables["storage"].(map[string]any); ok {
			delete(storage, compositeKey)
		}
		return nodeSuccess(workflowStateStoreOutput(node, runtime, "MEMORY", operation, namespace, storageKey, valueType, nil, 0, false))
	case "DEFINE", "WRITE", "UPSERT", "APPEND":
		value := workflowStateStoreValue(ctx, config, runtime, valueType)
		if operation == "APPEND" {
			value = appendWorkflowStateValue(current, value)
			valueType = "ARRAY"
		}
		workflowSetVar(execCtx.Variables, "storage", compositeKey, value)
		return nodeSuccess(workflowStateStoreOutput(node, runtime, "MEMORY", operation, namespace, storageKey, valueType, value, 1, true))
	default:
		return nodeFailed("STATE_STORE_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{"errorCode": "STATE_STORE_OPERATION_UNSUPPORTED", "operation": operation, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func (s *Service) workflowDatabaseStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation string, namespace string, storageKey string, valueType string) workflowNodeResult {
	key := marketing.StorageStateKey{TenantID: runtime.Activity.TenantID, Namespace: namespace, StorageKey: storageKey}
	switch operation {
	case "READ":
		state, err := s.repository.GetStorageState(ctx, key)
		if err != nil {
			if errors.Is(err, marketing.ErrNotFound) {
				return workflowStateStoreMissing(node, runtime, config, "DATABASE", namespace, storageKey, valueType)
			}
			return nodeFailed("STATE_STORE_READ_FAILED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_READ_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		return nodeSuccess(workflowStorageStateOutput(node, runtime, "DATABASE", operation, state, true))
	case "DELETE":
		err := s.repository.DeleteStorageState(ctx, key)
		if err != nil && !errors.Is(err, marketing.ErrNotFound) {
			return nodeFailed("STATE_STORE_DELETE_FAILED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_DELETE_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		return nodeSuccess(workflowStateStoreOutput(node, runtime, "DATABASE", operation, namespace, storageKey, valueType, nil, 0, false))
	case "DEFINE", "WRITE", "UPSERT", "APPEND":
		value := workflowStateStoreValue(ctx, config, runtime, valueType)
		expectedVersion := workflowInt64(workflowConfigString(config, "expectedVersion"))
		if expectedVersion == 0 {
			expectedVersion = workflowStorageStateExpectedVersion(ctx)
		}
		appendValue := operation == "APPEND"
		if appendValue {
			valueType = "ARRAY"
		}
		now := time.Now().UTC()
		state, err := s.repository.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
			State: marketing.StorageState{
				ID:          marketing.NewID("store"),
				TenantID:    runtime.Activity.TenantID,
				Namespace:   namespace,
				StorageKey:  storageKey,
				ValueType:   valueType,
				Data:        workflowRawJSON(value),
				LastEventID: runtime.Inbox.Event.EventID,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			ExpectedVersion: expectedVersion,
			RequireVersion:  workflowConfigBool(config, "requireVersion", false) && expectedVersion > 0,
			Append:          appendValue,
		})
		if err != nil {
			code := "STATE_STORE_WRITE_FAILED"
			if errors.Is(err, marketing.ErrStorageStateConflict) {
				code = "STATE_STORE_VERSION_CONFLICT"
			}
			return nodeFailed(code+": "+err.Error(), map[string]any{"errorCode": code, "error": err.Error(), "expectedVersion": expectedVersion, "audit": workflowHandlerAudit(node, runtime)})
		}
		return nodeSuccess(workflowStorageStateOutput(node, runtime, "DATABASE", operation, state, true))
	default:
		return nodeFailed("STATE_STORE_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{"errorCode": "STATE_STORE_OPERATION_UNSUPPORTED", "operation": operation, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func (s *Service) workflowDatabaseTableStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, operation string, namespace string, storageKey string, valueType string) workflowNodeResult {
	tableNamespace := workflowDatabaseTableNamespace(config, namespace)
	primaryKey := workflowConfigStringList(config, "primaryKey")
	if len(primaryKey) == 0 {
		primaryKey = []string{"tenantId", "activityId", "subjectId"}
	}
	switch operation {
	case "READ":
		key := marketing.StorageStateKey{TenantID: runtime.Activity.TenantID, Namespace: tableNamespace, StorageKey: storageKey}
		state, err := s.repository.GetStorageState(ctx, key)
		if err != nil {
			if errors.Is(err, marketing.ErrNotFound) {
				return workflowStateStoreMissing(node, runtime, config, "DATABASE_TABLE", tableNamespace, storageKey, "TABLE")
			}
			return nodeFailed("STATE_STORE_TABLE_READ_FAILED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_TABLE_READ_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		output := workflowStorageStateOutput(node, runtime, "DATABASE_TABLE", operation, state, true)
		output["table"] = workflowConfigString(config, "physicalTable")
		output["primaryKey"] = primaryKey
		return nodeSuccess(output)
	case "DELETE":
		err := s.repository.DeleteStorageState(ctx, marketing.StorageStateKey{TenantID: runtime.Activity.TenantID, Namespace: tableNamespace, StorageKey: storageKey})
		if err != nil && !errors.Is(err, marketing.ErrNotFound) {
			return nodeFailed("STATE_STORE_TABLE_DELETE_FAILED: "+err.Error(), map[string]any{"errorCode": "STATE_STORE_TABLE_DELETE_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		output := workflowStateStoreOutput(node, runtime, "DATABASE_TABLE", operation, tableNamespace, storageKey, "TABLE", nil, 0, false)
		output["table"] = workflowConfigString(config, "physicalTable")
		output["primaryKey"] = primaryKey
		return nodeSuccess(output)
	case "DEFINE", "WRITE", "UPSERT", "APPEND":
		rows, tableErr := workflowDatabaseTableRows(ctx, node, config, workflowStateStoreValue(ctx, config, runtime, "TABLE"), runtime)
		if tableErr != nil {
			return nodeFailed(tableErr.Error(), map[string]any{"errorCode": workflowDatabaseTableErrorCode(tableErr), "error": tableErr.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		if len(rows) == 0 {
			return nodeFailed("STATE_STORE_TABLE_ROW_REQUIRED: table row is required", map[string]any{"errorCode": "STATE_STORE_TABLE_ROW_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
		}
		expectedVersion := workflowInt64(workflowConfigString(config, "expectedVersion"))
		if expectedVersion == 0 {
			expectedVersion = workflowStorageStateExpectedVersion(ctx)
		}
		requireVersion := workflowConfigBool(config, "requireVersion", false) && expectedVersion > 0
		appendValue := operation == "APPEND"
		now := time.Now().UTC()
		storedRows := make([]any, 0, len(rows))
		var lastState marketing.StorageState
		for _, row := range rows {
			rowKey := workflowDatabaseTableRowKey(row, primaryKey)
			if rowKey == "" {
				rowKey = storageKey
			}
			state, err := s.repository.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
				State: marketing.StorageState{
					ID:          marketing.NewID("store"),
					TenantID:    runtime.Activity.TenantID,
					Namespace:   tableNamespace,
					StorageKey:  rowKey,
					ValueType:   map[bool]string{true: "ARRAY", false: "OBJECT"}[appendValue],
					Data:        workflowRawJSON(row),
					LastEventID: runtime.Inbox.Event.EventID,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
				ExpectedVersion: expectedVersion,
				RequireVersion:  requireVersion && len(rows) == 1,
				Append:          appendValue,
			})
			if err != nil {
				code := "STATE_STORE_TABLE_WRITE_FAILED"
				if errors.Is(err, marketing.ErrStorageStateConflict) {
					code = "STATE_STORE_TABLE_VERSION_CONFLICT"
				}
				return nodeFailed(code+": "+err.Error(), map[string]any{"errorCode": code, "error": err.Error(), "expectedVersion": expectedVersion, "audit": workflowHandlerAudit(node, runtime)})
			}
			lastState = state
			storedRows = append(storedRows, workflowStorageStateData(state))
		}
		output := workflowStateStoreOutput(node, runtime, "DATABASE_TABLE", operation, tableNamespace, lastState.StorageKey, "TABLE", storedRows, lastState.Version, true)
		output["table"] = workflowConfigString(config, "physicalTable")
		output["primaryKey"] = primaryKey
		output["rowCount"] = len(storedRows)
		return nodeSuccess(output)
	default:
		return nodeFailed("STATE_STORE_TABLE_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{"errorCode": "STATE_STORE_TABLE_OPERATION_UNSUPPORTED", "operation": operation, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func workflowDatabaseTableNamespace(config map[string]any, fallback string) string {
	for _, key := range []string{"tableNamespace", "physicalTable", "table"} {
		if value := workflowConfigString(config, key); value != "" {
			return "table:" + value
		}
	}
	return "table:" + fallback
}

func workflowDatabaseTableRows(ctx context.Context, node marketing.WorkflowNode, config map[string]any, value any, runtime workflowRuntime) ([]map[string]any, error) {
	normalized := workflowJSONValue(value)
	var rows []map[string]any
	switch typed := normalized.(type) {
	case []any:
		for _, item := range typed {
			if row, ok := item.(map[string]any); ok {
				mapped, err := workflowDatabaseTablePrepareRow(ctx, node, config, row, runtime)
				if err != nil {
					return nil, err
				}
				rows = append(rows, mapped)
			}
		}
	case map[string]any:
		mapped, err := workflowDatabaseTablePrepareRow(ctx, node, config, typed, runtime)
		if err != nil {
			return nil, err
		}
		rows = append(rows, mapped)
	}
	return rows, nil
}

func workflowDatabaseTablePrepareRow(ctx context.Context, node marketing.WorkflowNode, config map[string]any, row map[string]any, runtime workflowRuntime) (map[string]any, error) {
	out := workflowDatabaseTableEnrichRow(row, runtime)
	out = workflowDatabaseTableApplyMapping(ctx, node, config, out, runtime, "keyMapping")
	out = workflowDatabaseTableApplyMapping(ctx, node, config, out, runtime, "valueMapping")
	if err := workflowDatabaseTableValidateRow(config, out); err != nil {
		return nil, err
	}
	return out, nil
}

func workflowDatabaseTableEnrichRow(row map[string]any, runtime workflowRuntime) map[string]any {
	out := make(map[string]any, len(row)+4)
	for key, value := range row {
		out[key] = value
	}
	if _, ok := out["tenantId"]; !ok {
		out["tenantId"] = runtime.Activity.TenantID
	}
	if _, ok := out["activityId"]; !ok {
		out["activityId"] = runtime.Activity.ID
	}
	if _, ok := out["subjectId"]; !ok {
		out["subjectId"] = runtime.Inbox.Event.Subject.ID
	}
	if _, ok := out["eventId"]; !ok {
		out["eventId"] = runtime.Inbox.Event.EventID
	}
	return out
}

func workflowDatabaseTableRowKey(row map[string]any, primaryKey []string) string {
	parts := make([]string, 0, len(primaryKey))
	for _, key := range primaryKey {
		value, ok := row[key]
		if !ok || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
			return ""
		}
		parts = append(parts, strings.TrimSpace(fmt.Sprint(value)))
	}
	return strings.Join(parts, ":")
}

func workflowDatabaseTableApplyMapping(ctx context.Context, node marketing.WorkflowNode, config map[string]any, row map[string]any, runtime workflowRuntime, mappingKey string) map[string]any {
	raw, ok := config[mappingKey]
	if !ok || raw == nil {
		return row
	}
	mapping, ok := workflowJSONValue(raw).(map[string]any)
	if !ok {
		return row
	}
	out := make(map[string]any, len(row)+len(mapping))
	for key, value := range row {
		out[key] = value
	}
	base := workflowFactQueryBaseContext(ctx, runtime)
	base["row"] = workflowJSONValue(row)
	for column, spec := range mapping {
		column = strings.TrimSpace(column)
		if column == "" {
			continue
		}
		if value, ok := workflowDatabaseTableMappingValue(base, spec, runtime, node); ok {
			out[column] = value
		}
	}
	return out
}

func workflowDatabaseTableMappingValue(base map[string]any, spec any, runtime workflowRuntime, node marketing.WorkflowNode) (any, bool) {
	switch typed := spec.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		if strings.HasPrefix(text, "$.") || text == "$" {
			value, err := extractWorkflowJSONPath(base, text)
			return value, err == nil
		}
		if strings.Contains(text, "${") {
			return workflowRenderNodeTemplate(text, runtime, node), true
		}
		return typed, true
	case map[string]any:
		if path := workflowConfigString(typed, "path"); path != "" {
			value, err := extractWorkflowJSONPath(base, path)
			if err == nil {
				return value, true
			}
			if fallback, ok := typed["default"]; ok {
				return workflowJSONValue(fallback), true
			}
			return nil, false
		}
		if template := workflowConfigString(typed, "template"); template != "" {
			return workflowRenderNodeTemplate(template, runtime, node), true
		}
		if value, ok := typed["value"]; ok {
			return workflowJSONValue(value), true
		}
	case nil:
		return nil, false
	default:
		return workflowJSONValue(spec), true
	}
	return nil, false
}

func workflowDatabaseTableValidateRow(config map[string]any, row map[string]any) error {
	columns := workflowDatabaseTableSchemaColumns(config)
	for _, column := range columns {
		value, exists := row[column.Name]
		if column.Required && (!exists || workflowDatabaseTableValueEmpty(value)) {
			return fmt.Errorf("STATE_STORE_TABLE_SCHEMA_INVALID: required column %s is missing", column.Name)
		}
		if exists && !workflowDatabaseTableColumnTypeValid(value, column.Type) {
			return fmt.Errorf("STATE_STORE_TABLE_SCHEMA_INVALID: column %s must be %s", column.Name, column.Type)
		}
	}
	return nil
}

type workflowDatabaseTableColumn struct {
	Name     string
	Type     string
	Required bool
}

func workflowDatabaseTableSchemaColumns(config map[string]any) []workflowDatabaseTableColumn {
	schema, _ := workflowJSONValue(config["schema"]).(map[string]any)
	if schema == nil {
		schema, _ = workflowJSONValue(config["tableSchema"]).(map[string]any)
	}
	if schema == nil {
		return nil
	}
	rawColumns := firstWorkflowAny(schema["columns"], schema["fields"])
	switch typed := workflowJSONValue(rawColumns).(type) {
	case []any:
		columns := make([]workflowDatabaseTableColumn, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				name := firstWorkflowString(workflowConfigString(mapped, "name"), workflowConfigString(mapped, "key"))
				if name == "" {
					continue
				}
				columns = append(columns, workflowDatabaseTableColumn{
					Name:     name,
					Type:     strings.ToUpper(firstWorkflowString(workflowConfigString(mapped, "type"), "ANY")),
					Required: workflowConfigBool(mapped, "required", false),
				})
			}
		}
		return columns
	case map[string]any:
		columns := make([]workflowDatabaseTableColumn, 0, len(typed))
		for name, item := range typed {
			column := workflowDatabaseTableColumn{Name: strings.TrimSpace(name), Type: "ANY"}
			if mapped, ok := workflowJSONValue(item).(map[string]any); ok {
				column.Type = strings.ToUpper(firstWorkflowString(workflowConfigString(mapped, "type"), "ANY"))
				column.Required = workflowConfigBool(mapped, "required", false)
			} else if typeName := strings.TrimSpace(fmt.Sprint(item)); typeName != "" {
				column.Type = strings.ToUpper(typeName)
			}
			if column.Name != "" {
				columns = append(columns, column)
			}
		}
		return columns
	default:
		return nil
	}
}

func workflowDatabaseTableValueEmpty(value any) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func workflowDatabaseTableColumnTypeValid(value any, columnType string) bool {
	if value == nil || columnType == "" || columnType == "ANY" {
		return true
	}
	switch strings.ToUpper(columnType) {
	case "STRING":
		_, ok := value.(string)
		return ok
	case "NUMBER", "DECIMAL":
		_, ok := workflowRouteNumber(value)
		return ok
	case "BOOLEAN", "BOOL":
		_, ok := value.(bool)
		return ok
	case "OBJECT", "JSON":
		_, ok := workflowJSONValue(value).(map[string]any)
		return ok
	case "ARRAY", "TABLE":
		_, ok := workflowJSONValue(value).([]any)
		return ok
	default:
		return true
	}
}

func workflowDatabaseTableErrorCode(err error) string {
	message := err.Error()
	if strings.HasPrefix(message, "STATE_STORE_TABLE_SCHEMA_INVALID") {
		return "STATE_STORE_TABLE_SCHEMA_INVALID"
	}
	return "STATE_STORE_TABLE_INVALID"
}

func (s *Service) workflowRedisStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, resource marketing.Resource, config map[string]any, operation string, namespace string, storageKey string, valueType string) workflowNodeResult {
	redisNamespace := workflowRedisNamespace(resource, namespace)
	redisKey := workflowRedisStorageKey(resource, config, storageKey)
	switch resource.Type {
	case marketing.ResourceRedisHash:
		field := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "fieldTemplate"), firstWorkflowString(workflowConfigString(config, "field"), "value")), runtime, node)
		redisKey = redisKey + "#" + field
	case marketing.ResourceRedisCounter:
		return s.workflowRedisCounterStateStore(ctx, node, runtime, resource, config, operation, redisNamespace, redisKey)
	case marketing.ResourceRedisLock:
		return s.workflowRedisLockStateStore(ctx, node, runtime, resource, config, operation, redisNamespace, redisKey)
	}
	result := s.workflowDatabaseStateStore(ctx, node, runtime, config, operation, redisNamespace, redisKey, valueType)
	workflowAnnotateRedisOutput(result.Output, resource, redisNamespace, redisKey)
	return result
}

func workflowRedisNamespace(resource marketing.Resource, namespace string) string {
	return "redis:" + resource.Code + ":" + firstWorkflowString(namespace, "workflow")
}

func workflowRedisStorageKey(resource marketing.Resource, config map[string]any, storageKey string) string {
	resourceConfig := resource.ConfigMap()
	prefix := firstWorkflowString(resourceConfigString(resourceConfig, "keyPrefix"), resourceConfigString(resourceConfig, "prefix"))
	if prefix == "" {
		prefix = workflowConfigString(config, "keyPrefix")
	}
	if prefix != "" && !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return prefix + storageKey
}

func workflowAnnotateRedisOutput(output map[string]any, resource marketing.Resource, namespace string, key string) {
	if output == nil {
		return
	}
	output["storageType"] = "REDIS"
	output["redisType"] = resource.Type
	output["redisNamespace"] = namespace
	output["redisKey"] = key
	output["adapterMode"] = "RESOURCE_STATE_BACKED"
}

func (s *Service) workflowRedisCounterStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, resource marketing.Resource, config map[string]any, operation string, namespace string, storageKey string) workflowNodeResult {
	switch operation {
	case "READ", "DELETE", "WRITE", "UPSERT", "DEFINE":
		result := s.workflowDatabaseStateStore(ctx, node, runtime, config, operation, namespace, storageKey, "NUMBER")
		workflowAnnotateRedisOutput(result.Output, resource, namespace, storageKey)
		return result
	case "INCREMENT", "CHECK_AND_INCREMENT":
		delta := workflowConfigNumber(config, "delta")
		if delta == 0 {
			delta = workflowConfigNumber(config, "step")
		}
		if delta == 0 {
			delta = 1
		}
		current := float64(0)
		currentState, err := s.repository.GetStorageState(ctx, marketing.StorageStateKey{TenantID: runtime.Activity.TenantID, Namespace: namespace, StorageKey: storageKey})
		if err != nil && !errors.Is(err, marketing.ErrNotFound) {
			return nodeFailed("REDIS_COUNTER_READ_FAILED: "+err.Error(), map[string]any{"errorCode": "REDIS_COUNTER_READ_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		if err == nil {
			if number, ok := workflowRouteNumber(workflowStorageStateData(currentState)); ok {
				current = number
			}
		}
		next := current + delta
		if max := workflowConfigNumber(config, "max"); max > 0 && next > max {
			output := workflowStateStoreOutput(node, runtime, "REDIS", operation, namespace, storageKey, "NUMBER", current, currentState.Version, true)
			output["errorCode"] = "REDIS_COUNTER_LIMIT_EXCEEDED"
			output["counter"] = current
			output["delta"] = delta
			output["max"] = max
			workflowAnnotateRedisOutput(output, resource, namespace, storageKey)
			return nodeFailed("REDIS_COUNTER_LIMIT_EXCEEDED: counter exceeds max", output)
		}
		now := time.Now().UTC()
		state, err := s.repository.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
			State: marketing.StorageState{
				ID:          marketing.NewID("store"),
				TenantID:    runtime.Activity.TenantID,
				Namespace:   namespace,
				StorageKey:  storageKey,
				ValueType:   "NUMBER",
				Data:        workflowRawJSON(next),
				LastEventID: runtime.Inbox.Event.EventID,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			ExpectedVersion: currentState.Version,
			RequireVersion:  currentState.Version > 0,
		})
		if err != nil {
			code := "REDIS_COUNTER_INCREMENT_FAILED"
			if errors.Is(err, marketing.ErrStorageStateConflict) {
				code = "REDIS_COUNTER_VERSION_CONFLICT"
			}
			return nodeFailed(code+": "+err.Error(), map[string]any{"errorCode": code, "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		output := workflowStorageStateOutput(node, runtime, "REDIS", operation, state, true)
		output["counter"] = next
		output["delta"] = delta
		workflowAnnotateRedisOutput(output, resource, namespace, storageKey)
		return nodeSuccess(output)
	default:
		return nodeFailed("REDIS_COUNTER_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{"errorCode": "REDIS_COUNTER_OPERATION_UNSUPPORTED", "operation": operation, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func (s *Service) workflowRedisLockStateStore(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, resource marketing.Resource, config map[string]any, operation string, namespace string, storageKey string) workflowNodeResult {
	switch operation {
	case "READ", "DELETE":
		result := s.workflowDatabaseStateStore(ctx, node, runtime, config, operation, namespace, storageKey, "OBJECT")
		workflowAnnotateRedisOutput(result.Output, resource, namespace, storageKey)
		return result
	case "WRITE", "UPSERT", "DEFINE":
		key := marketing.StorageStateKey{TenantID: runtime.Activity.TenantID, Namespace: namespace, StorageKey: storageKey}
		existing, err := s.repository.GetStorageState(ctx, key)
		if err == nil {
			expiresAt := existing.ExpiresAt
			if expiresAt == nil || expiresAt.After(time.Now().UTC()) {
				output := workflowStorageStateOutput(node, runtime, "REDIS", operation, existing, true)
				output["errorCode"] = "REDIS_LOCK_HELD"
				workflowAnnotateRedisOutput(output, resource, namespace, storageKey)
				return nodeFailed("REDIS_LOCK_HELD: lock is already held", output)
			}
		} else if !errors.Is(err, marketing.ErrNotFound) {
			return nodeFailed("REDIS_LOCK_READ_FAILED: "+err.Error(), map[string]any{"errorCode": "REDIS_LOCK_READ_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		ttlSeconds := workflowConfigNumber(config, "ttlSeconds")
		if ttlSeconds <= 0 {
			ttlSeconds = workflowConfigNumber(resource.ConfigMap(), "ttlSeconds")
		}
		if ttlSeconds <= 0 {
			ttlSeconds = 30
		}
		now := time.Now().UTC()
		expiresAt := now.Add(time.Duration(ttlSeconds * float64(time.Second)))
		owner := firstWorkflowString(workflowConfigString(config, "owner"), runtime.Inbox.Event.EventID)
		value := map[string]any{"owner": owner, "eventId": runtime.Inbox.Event.EventID, "acquiredAt": now.Format(time.RFC3339Nano), "expiresAt": expiresAt.Format(time.RFC3339Nano)}
		state, err := s.repository.UpsertStorageState(ctx, marketing.UpsertStorageStateCommand{
			State: marketing.StorageState{
				ID:          marketing.NewID("store"),
				TenantID:    runtime.Activity.TenantID,
				Namespace:   namespace,
				StorageKey:  storageKey,
				ValueType:   "OBJECT",
				Data:        workflowRawJSON(value),
				LastEventID: runtime.Inbox.Event.EventID,
				ExpiresAt:   &expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			ExpectedVersion: existing.Version,
			RequireVersion:  existing.Version > 0,
		})
		if err != nil {
			return nodeFailed("REDIS_LOCK_ACQUIRE_FAILED: "+err.Error(), map[string]any{"errorCode": "REDIS_LOCK_ACQUIRE_FAILED", "error": err.Error(), "audit": workflowHandlerAudit(node, runtime)})
		}
		output := workflowStorageStateOutput(node, runtime, "REDIS", operation, state, true)
		output["lock"] = value
		workflowAnnotateRedisOutput(output, resource, namespace, storageKey)
		return nodeSuccess(output)
	default:
		return nodeFailed("REDIS_LOCK_OPERATION_UNSUPPORTED: operation is not supported", map[string]any{"errorCode": "REDIS_LOCK_OPERATION_UNSUPPORTED", "operation": operation, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func workflowStateStoreMissing(node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any, storageType string, namespace string, storageKey string, valueType string) workflowNodeResult {
	output := workflowStateStoreOutput(node, runtime, storageType, "READ", namespace, storageKey, valueType, nil, 0, false)
	output["errorCode"] = "STATE_STORE_NOT_FOUND"
	switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "missingPolicy"), "EMPTY")) {
	case "BLOCK":
		return nodeFailed("STATE_STORE_NOT_FOUND: storage value does not exist", output)
	default:
		return nodeSuccess(output)
	}
}

func workflowStorageStateOutput(node marketing.WorkflowNode, runtime workflowRuntime, storageType string, operation string, state marketing.StorageState, exists bool) map[string]any {
	return workflowStateStoreOutput(node, runtime, storageType, operation, state.Namespace, state.StorageKey, state.ValueType, workflowStorageStateData(state), state.Version, exists)
}

func workflowStateStoreOutput(node marketing.WorkflowNode, runtime workflowRuntime, storageType string, operation string, namespace string, storageKey string, valueType string, value any, version int64, exists bool) map[string]any {
	return map[string]any{
		"stored_value":       workflowJSONValue(value),
		"storage_entry":      map[string]any{"namespace": namespace, "key": storageKey, "valueType": valueType, "value": workflowJSONValue(value), "exists": exists, "version": version},
		"storage_version":    version,
		"storageType":        storageType,
		"operation":          operation,
		"executionMode":      "REAL_HANDLER",
		"executionAuthority": "WorkflowExecutor/StateStore",
		"audit":              workflowHandlerAudit(node, runtime),
	}
}

func workflowStorageStateData(state marketing.StorageState) any {
	if len(state.Data) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(state.Data, &out); err != nil {
		return string(state.Data)
	}
	return out
}

func workflowStateStoreValue(ctx context.Context, config map[string]any, runtime workflowRuntime, valueType string) any {
	valuePath := workflowConfigString(config, "valuePath")
	if valuePath != "" {
		base := workflowFactQueryBaseContext(ctx, runtime)
		if value, err := extractWorkflowJSONPath(base, valuePath); err == nil {
			return coerceWorkflowStateValue(value, valueType)
		}
	}
	if value, ok := config["defaultValue"]; ok {
		return coerceWorkflowStateValue(value, valueType)
	}
	if value, ok := workflowResolvedInput(ctx, "value"); ok {
		return coerceWorkflowStateValue(value, valueType)
	}
	return coerceWorkflowStateValue(nil, valueType)
}

func coerceWorkflowStateValue(value any, valueType string) any {
	switch strings.ToUpper(valueType) {
	case "STRING":
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	case "NUMBER":
		return workflowConfigNumber(map[string]any{"value": value}, "value")
	case "BOOLEAN":
		return workflowConfigBool(map[string]any{"value": value}, "value", false)
	case "ARRAY", "TABLE":
		if items, ok := workflowJSONValue(value).([]any); ok {
			return items
		}
		if value == nil {
			return []any{}
		}
		return []any{workflowJSONValue(value)}
	case "OBJECT":
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			return mapped
		}
		return map[string]any{}
	default:
		return workflowJSONValue(value)
	}
}

func appendWorkflowStateValue(current any, value any) []any {
	items, _ := workflowJSONValue(current).([]any)
	if items == nil {
		items = []any{}
	}
	if incoming, ok := workflowJSONValue(value).([]any); ok {
		return append(items, incoming...)
	}
	return append(items, workflowJSONValue(value))
}

func workflowRawJSON(value any) json.RawMessage {
	raw, err := json.Marshal(workflowJSONValue(value))
	if err != nil || len(raw) == 0 {
		return json.RawMessage(`null`)
	}
	return raw
}

func quotaCheckWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	result := limitWorkflowNodeHandler(ctx, node, runtime)
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	if hold, ok := result.Output["limit_hold"]; ok {
		result.Output["quota_check"] = hold
	}
	result.Output["nodeType"] = nodeHandlerKey(node)
	result.Output["executionMode"] = "REAL_HANDLER"
	result.Output["executionAuthority"] = "WorkflowExecutor/QuotaStore"
	return result
}

func (s *Service) quotaCheckWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef")) == "" {
		return quotaCheckWorkflowNodeHandler(ctx, node, runtime)
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	scopeKey := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "scopeKeyTemplate"), "${subject.id}:${activityId}"), runtime, node)
	windowSegment, err := s.workflowQuotaWindowSegment(ctx, node, runtime, config)
	if err != nil {
		return nodeFailed(err.Error(), map[string]any{
			"errorCode":          "QUOTA_WINDOW_SEGMENT_FAILED",
			"scopeKey":           scopeKey,
			"executionMode":      "REAL_HANDLER",
			"executionAuthority": "WorkflowExecutor/RedisCounterResource",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
	logicalKey := workflowQuotaLogicalKey(scopeKey, windowSegment.SegmentKey)
	namespace := firstWorkflowString(workflowConfigString(config, "namespace"), "quota")
	operation := marketing.ResourceCheckAndIncrement
	if strings.EqualFold(workflowConfigString(config, "consumeMode"), "CHECK_ONLY") {
		operation = marketing.ResourceRead
	}
	output, err := s.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
		Operation:   operation,
		ResourceRef: workflowExplicitConfigString(node, "resourceRef"),
		Namespace:   namespace,
		Key:         logicalKey,
		ValueType:   "NUMBER",
		Options: map[string]any{
			"delta": workflowQuotaCheckDelta(config),
			"max":   workflowConfigNumber(config, "maxCount"),
			"window": map[string]any{
				"type":       windowSegment.Window,
				"segmentKey": windowSegment.SegmentKey,
				"start":      windowSegment.Start,
				"end":        windowSegment.End,
				"source":     windowSegment.Source,
			},
		},
	})
	if err != nil {
		if output == nil {
			output = map[string]any{}
		}
		output["quota_check"] = workflowQuotaCheckResult(config, runtime, scopeKey, logicalKey, windowSegment, output, false)
		output["executionMode"] = "REAL_HANDLER"
		output["executionAuthority"] = "WorkflowExecutor/RedisCounterResource"
		return nodeFailed(err.Error(), output)
	}
	quotaCheck := workflowQuotaCheckResult(config, runtime, scopeKey, logicalKey, windowSegment, output, true)
	return nodeSuccess(map[string]any{
		"quota_check":        quotaCheck,
		"limit_hold":         quotaCheck,
		"resource_result":    output,
		"nodeType":           nodeHandlerKey(node),
		"executionMode":      "REAL_HANDLER",
		"executionAuthority": "WorkflowExecutor/RedisCounterResource",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func workflowQuotaCheckDelta(config map[string]any) float64 {
	switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "consumeMode"), "CHECK_ONLY")) {
	case "ATOMIC":
		return 1
	default:
		return 0
	}
}

type workflowQuotaWindowSegment struct {
	Window     string
	SegmentKey string
	Start      string
	End        string
	Source     string
	LuaOutput  map[string]any
}

func (s *Service) workflowQuotaWindowSegment(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) (workflowQuotaWindowSegment, error) {
	window := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "window"), "DAY"))
	occurredAt := runtime.Inbox.Event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	segment := workflowQuotaWindowSegment{Window: window}
	if window == "CUSTOM_RANGE" {
		if source := workflowConfigString(config, "windowLuaSource"); source != "" {
			output, err := s.executeWorkflowLua(ctx, node, runtime, source)
			if err != nil {
				return segment, fmt.Errorf("QUOTA_WINDOW_LUA_FAILED: %w", err)
			}
			segment.LuaOutput = output
			segment.SegmentKey = firstWorkflowString(workflowOutputString(output["segmentKey"]), workflowOutputString(output["bucket"]))
			segment.Start = firstWorkflowString(workflowOutputString(output["windowStart"]), workflowOutputString(output["start"]))
			segment.End = firstWorkflowString(workflowOutputString(output["windowEnd"]), workflowOutputString(output["end"]))
			segment.Source = "LUA"
		}
		if segment.SegmentKey == "" {
			segment.SegmentKey = workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "windowSegmentTemplate"), "${date}"), runtime, node)
			segment.Start = workflowRenderNodeTemplate(workflowConfigString(config, "windowStartTemplate"), runtime, node)
			segment.End = workflowRenderNodeTemplate(workflowConfigString(config, "windowEndTemplate"), runtime, node)
			segment.Source = "TEMPLATE"
		}
		if strings.TrimSpace(segment.SegmentKey) == "" {
			return segment, fmt.Errorf("QUOTA_WINDOW_SEGMENT_REQUIRED: windowSegmentTemplate or windowLuaSource must produce segmentKey")
		}
		return segment, nil
	}
	segment.Source = "BUILTIN"
	switch window {
	case "ACTIVITY":
		segment.SegmentKey = "activity"
	case "WEEK":
		year, week := occurredAt.ISOWeek()
		segment.SegmentKey = fmt.Sprintf("%04dW%02d", year, week)
	case "MONTH":
		segment.SegmentKey = occurredAt.UTC().Format("200601")
	case "ROLLING_WINDOW":
		sizeSeconds := int(workflowConfigNumber(config, "windowSizeSeconds"))
		if sizeSeconds <= 0 {
			sizeSeconds = 3600
		}
		bucket := occurredAt.UTC().Unix() / int64(sizeSeconds)
		segment.SegmentKey = fmt.Sprintf("rolling:%ds:%d", sizeSeconds, bucket)
	default:
		segment.Window = "DAY"
		segment.SegmentKey = occurredAt.UTC().Format("20060102")
	}
	return segment, nil
}

func workflowQuotaLogicalKey(scopeKey string, segmentKey string) string {
	scopeKey = strings.TrimSpace(scopeKey)
	segmentKey = strings.TrimSpace(segmentKey)
	if segmentKey == "" {
		return scopeKey
	}
	if scopeKey == "" {
		return segmentKey
	}
	return scopeKey + ":" + segmentKey
}

func workflowQuotaCheckResult(config map[string]any, runtime workflowRuntime, scopeKey string, logicalKey string, windowSegment workflowQuotaWindowSegment, resourceOutput map[string]any, allowed bool) map[string]any {
	counter := firstWorkflowAny(resourceOutput["counter"], resourceOutput["stored_value"])
	return map[string]any{
		"scope":      firstWorkflowString(workflowConfigString(config, "scope"), "USER"),
		"scopeKey":   scopeKey,
		"logicalKey": logicalKey,
		"window":     windowSegment.Window,
		"windowSegment": map[string]any{
			"segmentKey": windowSegment.SegmentKey,
			"start":      windowSegment.Start,
			"end":        windowSegment.End,
			"source":     windowSegment.Source,
			"luaOutput":  windowSegment.LuaOutput,
		},
		"maxCount":     workflowConfigNumber(config, "maxCount"),
		"consumeMode":  firstWorkflowString(workflowConfigString(config, "consumeMode"), "CHECK_ONLY"),
		"exceedAction": firstWorkflowString(workflowConfigString(config, "exceedAction"), "BLOCK"),
		"allowed":      allowed,
		"counter":      counter,
		"subject":      runtime.Inbox.Event.Subject.ID,
		"activityId":   runtime.Activity.ID,
		"resourceRef":  workflowConfigString(resourceOutput, "resourceRef"),
	}
}

func (s *Service) lockWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef"))
	if resourceRef == "" {
		return nodeFailed("LOCK_RESOURCE_REF_REQUIRED: resourceRef is required", map[string]any{"errorCode": "LOCK_RESOURCE_REF_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
	}
	operation := workflowLockResourceOperation(config)
	key := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "keyTemplate"), "${activityId}:${subject.id}"), runtime, node)
	output, err := s.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
		Operation:   operation,
		ResourceRef: resourceRef,
		Namespace:   firstWorkflowString(workflowConfigString(config, "namespace"), "lock"),
		Key:         key,
		ValueType:   "OBJECT",
		Options: map[string]any{
			"ttlSeconds": workflowConfigNumber(config, "ttlSeconds"),
			"owner":      firstWorkflowString(workflowConfigString(config, "owner"), runtime.Inbox.Event.EventID),
		},
	})
	if err != nil {
		if output == nil {
			output = map[string]any{}
		}
		output["lock_result"] = workflowLockResult(config, key, output, false)
		return nodeFailed(err.Error(), output)
	}
	return nodeSuccess(map[string]any{
		"lock_result":        workflowLockResult(config, key, output, true),
		"resource_result":    output,
		"nodeType":           nodeHandlerKey(node),
		"executionMode":      "REAL_HANDLER",
		"executionAuthority": "WorkflowExecutor/RedisLockResource",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func workflowLockResourceOperation(config map[string]any) marketing.ResourceOperation {
	switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "operation"), "ACQUIRE")) {
	case "READ":
		return marketing.ResourceRead
	case "RELEASE", "DELETE":
		return marketing.ResourceDelete
	default:
		return marketing.ResourceUpsert
	}
}

func workflowLockResult(config map[string]any, key string, resourceOutput map[string]any, acquired bool) map[string]any {
	return map[string]any{
		"operation":   firstWorkflowString(workflowConfigString(config, "operation"), "ACQUIRE"),
		"key":         key,
		"acquired":    acquired,
		"lock":        firstWorkflowAny(resourceOutput["lock"], resourceOutput["stored_value"]),
		"resourceRef": workflowConfigString(resourceOutput, "resourceRef"),
		"errorCode":   workflowConfigString(resourceOutput, "errorCode"),
	}
}

func decisionWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	ruleResult := runtime.RuleResult
	if strings.EqualFold(node.Type, "qualify") {
		rules := runtime.Activity.Rules
		if nodeRules := workflowQualifyNodeRules(config); len(nodeRules) > 0 {
			nodeRuleCodes := map[string]struct{}{}
			for _, item := range nodeRules {
				nodeRuleCodes[strings.TrimSpace(item.Code)] = struct{}{}
			}
			rules = make([]marketing.Rule, 0, len(runtime.Activity.Rules)+len(nodeRules))
			for _, item := range runtime.Activity.Rules {
				if _, overridden := nodeRuleCodes[strings.TrimSpace(item.Code)]; overridden {
					continue
				}
				rules = append(rules, item)
			}
			rules = append(rules, nodeRules...)
		}
		if len(rules) > 0 {
			facts := workflowFactsWithQualifyInputObjects(ctx, config)
			ruleResult = rule.Execute(rule.Context{
				Event: runtime.Inbox.Event,
				Facts: facts,
			}, rules, nil)
			ruleResult.ActivityID = runtime.Activity.ID
			ruleResult.ActivityCode = runtime.Activity.Code
			ruleResult.ActivityVersion = runtime.Activity.Version
		}
	}
	qualified := runtime.Qualified
	if ruleResult.Decision != "" {
		qualified = ruleResult.Decision == marketing.RuleDecisionPassed
	}
	if strings.EqualFold(node.Type, "condition") && len(node.Config) > 0 {
		if expression := workflowConfigString(config, "expression"); expression != "" {
			language := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "expressionLanguage"), "RULE_EXPR"))
			if language != "RULE_EXPR" && language != "JSON_LOGIC" {
				return nodeFailed("CONDITION_LANGUAGE_UNSUPPORTED: expressionLanguage "+language+" is not supported", map[string]any{
					"errorCode":          "CONDITION_LANGUAGE_UNSUPPORTED",
					"expressionLanguage": language,
					"audit":              workflowHandlerAudit(node, runtime),
				})
			}
			conditionInput := workflowConditionInput(ctx, runtime)
			matched, evalErr := workflowEvaluateConditionExpression(language, expression, conditionInput)
			if evalErr != nil {
				ruleResult.ReasonCode = "CONDITION_EXPRESSION_ERROR"
				ruleResult.Reason = evalErr.Error()
				switch strings.ToUpper(firstWorkflowString(workflowConfigString(config, "errorAction"), "BLOCK")) {
				case "SKIP":
					return nodeSkipped("CONDITION_EXPRESSION_ERROR: "+evalErr.Error(), map[string]any{
						"errorCode": "CONDITION_EXPRESSION_ERROR",
						"reason":    evalErr.Error(),
						"audit":     workflowHandlerAudit(node, runtime),
					})
				case "ELSE":
					qualified = false
				default:
					qualified = false
					ruleResult.Decision = marketing.RuleDecisionRejected
				}
			} else if matched {
				qualified = true
				ruleResult.Decision = marketing.RuleDecisionPassed
			} else {
				qualified = false
				ruleResult.Decision = marketing.RuleDecisionRejected
				if ruleResult.ReasonCode == "" {
					ruleResult.ReasonCode = "CONDITION_NOT_MET"
				}
				if ruleResult.Reason == "" {
					ruleResult.Reason = "condition expression evaluated false"
				}
			}
		}
	}
	decision := normalizeWorkflowDecisionValue(string(ruleResult.Decision))
	if decision == "" {
		if qualified {
			decision = "PASS"
		} else {
			decision = "BLOCK"
		}
	}
	output := map[string]any{
		"nodeType":   nodeHandlerKey(node),
		"decision":   decision,
		"qualified":  qualified,
		"reasonCode": ruleResult.ReasonCode,
		"reason":     ruleResult.Reason,
		"ruleResult": workflowJSONValue(ruleResult),
		"audit":      workflowHandlerAudit(node, runtime),
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "condition":
		output["IF"] = qualified
		output["ELSE"] = !qualified
	case "qualify":
		output["PASS"] = decision == "PASS"
		output["BLOCK"] = decision == "BLOCK"
		output["RISK"] = decision == "RISK"
	case "switch":
		output["route"] = decision
		output[decision] = true
	}
	return nodeSuccess(map[string]any{
		"stage_result": output,
		"decision":     output["decision"],
		"qualified":    output["qualified"],
		"reasonCode":   output["reasonCode"],
		"reason":       output["reason"],
		"ruleResult":   output["ruleResult"],
		"PASS":         output["PASS"],
		"BLOCK":        output["BLOCK"],
		"RISK":         output["RISK"],
		"IF":           output["IF"],
		"ELSE":         output["ELSE"],
		"route":        output["route"],
		"audit":        workflowHandlerAudit(node, runtime),
	})
}

func workflowQualifyNodeRules(config map[string]any) []marketing.Rule {
	rawRules, _ := workflowJSONValue(config["qualifyRules"]).([]any)
	rules := make([]marketing.Rule, 0, len(rawRules))
	for index, raw := range rawRules {
		item, ok := workflowJSONValue(raw).(map[string]any)
		if !ok || !workflowConfigBool(item, "enabled", true) {
			continue
		}
		if rule := workflowQualifyConditionRuleFromConfig(item, index); rule.Code != "" {
			rules = append(rules, rule)
		}
	}
	if len(rules) > 0 {
		return rules
	}

	eligibilityRule := workflowConfigMap(config, "eligibilityRule")
	if len(eligibilityRule) == 0 || !workflowConfigBool(eligibilityRule, "enabled", true) {
		return nil
	}
	eligibilityRule["ruleCode"] = "ELIGIBILITY"
	rule := workflowQualifyConditionRuleFromConfig(eligibilityRule, 0)
	if rule.Code == "" {
		return nil
	}
	return []marketing.Rule{rule}
}

func workflowFactsWithQualifyInputObjects(ctx context.Context, config map[string]any) map[string]any {
	base := workflowFactsFromContext(ctx)
	facts := make(map[string]any)
	for key, value := range base {
		facts[key] = value
	}
	for _, inputObject := range workflowQualifyInputObjects(config) {
		if inputObject == "" || strings.EqualFold(inputObject, "EVENT_CONTEXT") {
			continue
		}
		if _, ok := facts[inputObject]; !ok {
			facts[inputObject] = map[string]any{}
		}
	}
	if len(facts) == 0 {
		return nil
	}
	return facts
}

func workflowQualifyInputObjects(config map[string]any) []string {
	rawRules, _ := workflowJSONValue(config["qualifyRules"]).([]any)
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range rawRules {
		item, ok := workflowJSONValue(raw).(map[string]any)
		if !ok || !workflowConfigBool(item, "enabled", true) {
			continue
		}
		inputObject := strings.ToUpper(strings.TrimSpace(workflowConfigString(item, "inputObject")))
		if inputObject == "" {
			inputObject = workflowDefaultQualifyInputObject(workflowConfigString(item, "ruleCode"))
		}
		if inputObject == "" {
			continue
		}
		if _, exists := seen[inputObject]; exists {
			continue
		}
		seen[inputObject] = struct{}{}
		out = append(out, inputObject)
	}
	return out
}

func workflowDefaultQualifyInputObject(ruleCode string) string {
	switch strings.ToUpper(strings.TrimSpace(ruleCode)) {
	case "KYC":
		return "KYC_PROFILE"
	case "RISK_SCORE":
		return "RISK_PROFILE"
	case "VIP_LEVEL", "COUNTRY_ALLOWED", "REGISTER_AGE":
		return "USER_PROFILE"
	case "DEPOSIT_REQUIRED":
		return "DEPOSIT_SUMMARY"
	case "ELIGIBILITY", "CHANNEL_ALLOWED":
		return "EVENT_CONTEXT"
	default:
		return ""
	}
}

func workflowQualifyConditionRuleFromConfig(config map[string]any, sort int) marketing.Rule {
	code := strings.ToUpper(strings.TrimSpace(firstWorkflowString(workflowConfigString(config, "ruleCode"), "ELIGIBILITY")))
	if code == "" {
		return marketing.Rule{}
	}
	logic := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "logic"), "AND"))
	if logic != "AND" && logic != "OR" {
		logic = "AND"
	}
	rawConditions, _ := workflowJSONValue(config["conditions"]).([]any)
	conditions := make([]marketing.Condition, 0, len(rawConditions))
	for _, raw := range rawConditions {
		item, ok := workflowJSONValue(raw).(map[string]any)
		if !ok {
			continue
		}
		field := strings.TrimSpace(workflowConfigString(item, "field"))
		op := strings.ToUpper(firstWorkflowString(workflowConfigString(item, "op"), "="))
		if field == "" || op == "" {
			continue
		}
		conditions = append(conditions, marketing.Condition{
			Field: field,
			Op:    op,
			Value: workflowJSONValue(item["value"]),
		})
	}
	if len(conditions) == 0 {
		return marketing.Rule{}
	}
	configPayload, err := json.Marshal(marketing.ConditionRuleConfig{
		Logic:      logic,
		Conditions: conditions,
	})
	if err != nil {
		return marketing.Rule{}
	}
	return marketing.Rule{
		Code:    code,
		Phase:   marketing.RulePhaseQualify,
		Type:    marketing.RuleTypeCondition,
		Config:  configPayload,
		Sort:    sort,
		Enabled: true,
		Version: 1,
	}
}

func userLevelCheckWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	base := workflowFactQueryBaseContext(ctx, runtime)
	user, userPath, userFound := workflowUserLevelCheckUser(base, config)
	exists := userFound
	if configuredExists, ok := workflowUserLevelCheckBool(user, workflowConfigString(config, "existsPath")); ok {
		exists = configuredExists
	}
	level, levelPath, levelFound := workflowUserLevelCheckNumber(base, user, config)
	minLevel := workflowConfigNumber(config, "minLevel")
	maxLevel := workflowConfigNumber(config, "maxLevel")
	requireExists := workflowConfigBool(config, "requireExistingUser", true)
	passed := true
	reasonCode := "USER_LEVEL_PASSED"
	reason := "user level passed"
	if requireExists && !exists {
		passed = false
		reasonCode = "USER_NOT_FOUND"
		reason = "user does not exist"
	} else if !levelFound {
		passed = false
		reasonCode = "USER_LEVEL_MISSING"
		reason = "user level is missing"
	} else if level < minLevel {
		passed = false
		reasonCode = "USER_LEVEL_TOO_LOW"
		reason = fmt.Sprintf("user level %.0f is lower than required %.0f", level, minLevel)
	} else if maxLevel > 0 && level > maxLevel {
		passed = false
		reasonCode = "USER_LEVEL_TOO_HIGH"
		reason = fmt.Sprintf("user level %.0f is higher than allowed %.0f", level, maxLevel)
	}
	if override := workflowConfigString(config, "blockReason"); override != "" && !passed {
		reason = override
	}
	decision := "PASS"
	if !passed {
		decision = strings.ToUpper(firstWorkflowString(workflowConfigString(config, "failDecision"), "BLOCK"))
		if decision != "RISK" && decision != "SKIP" {
			decision = "BLOCK"
		}
	}
	output := map[string]any{
		"user_level_check": map[string]any{
			"passed":        passed,
			"decision":      decision,
			"reasonCode":    reasonCode,
			"reason":        reason,
			"subject":       workflowJSONValue(runtime.Inbox.Event.Subject),
			"userFound":     userFound,
			"userExists":    exists,
			"userPath":      userPath,
			"level":         level,
			"levelPath":     levelPath,
			"minLevel":      minLevel,
			"maxLevel":      maxLevel,
			"levelSource":   firstWorkflowString(workflowConfigString(config, "levelSource"), "FACTS_OR_EVENT"),
			"requireExists": requireExists,
		},
		"decision":   decision,
		"qualified":  passed,
		"reasonCode": reasonCode,
		"reason":     reason,
		"PASS":       decision == "PASS",
		"BLOCK":      decision == "BLOCK",
		"RISK":       decision == "RISK",
		"audit":      workflowHandlerAudit(node, runtime),
	}
	if !passed && decision == "SKIP" {
		return nodeSkipped(reasonCode+": "+reason, output)
	}
	return nodeSuccess(output)
}

func workflowUserLevelCheckUser(base map[string]any, config map[string]any) (map[string]any, string, bool) {
	paths := workflowConfigStringList(config, "userPaths")
	if len(paths) == 0 {
		paths = []string{
			"$.facts.USER_PROFILE",
			"$.facts.user_profile",
			"$.input.facts.USER_PROFILE",
			"$.event.attributes.USER_PROFILE",
			"$.event.attributes.userProfile",
			"$.attributes.USER_PROFILE",
			"$.attributes.userProfile",
		}
	}
	for _, path := range paths {
		value, err := extractWorkflowJSONPath(base, path)
		if err != nil {
			continue
		}
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			return mapped, path, true
		}
	}
	return nil, "", false
}

func workflowUserLevelCheckNumber(base map[string]any, user map[string]any, config map[string]any) (float64, string, bool) {
	levelPaths := workflowConfigStringList(config, "levelPaths")
	if len(levelPaths) == 0 {
		if configured := workflowConfigString(config, "levelPath"); configured != "" {
			levelPaths = []string{configured}
		}
	}
	if len(levelPaths) == 0 {
		levelPaths = []string{
			"$.facts.USER_PROFILE.level",
			"$.facts.USER_PROFILE.vipLevel",
			"$.facts.USER_PROFILE.userLevel",
			"$.input.facts.USER_PROFILE.level",
			"$.event.attributes.userLevel",
			"$.event.attributes.vipLevel",
			"$.attributes.userLevel",
			"$.attributes.vipLevel",
		}
	}
	for _, path := range levelPaths {
		value, err := extractWorkflowJSONPath(base, path)
		if err != nil {
			continue
		}
		if number, ok := workflowRouteNumber(value); ok {
			return number, path, true
		}
	}
	for _, key := range []string{"level", "vipLevel", "userLevel"} {
		if user == nil {
			continue
		}
		if number, ok := workflowRouteNumber(user[key]); ok {
			return number, "user." + key, true
		}
	}
	return 0, "", false
}

func workflowUserLevelCheckBool(user map[string]any, path string) (bool, bool) {
	if user == nil {
		return false, false
	}
	key := strings.TrimSpace(path)
	key = strings.TrimPrefix(key, "$.")
	key = strings.TrimPrefix(key, "user.")
	if key == "" {
		key = "exists"
	}
	value, ok := user[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		if strings.EqualFold(strings.TrimSpace(typed), "true") {
			return true, true
		}
		if strings.EqualFold(strings.TrimSpace(typed), "false") {
			return false, true
		}
	}
	return workflowTruthy(value), true
}

func candidateRewardWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	if !runtime.Qualified {
		output := map[string]any{"errorCode": "CANDIDATE_REWARD_NOT_QUALIFIED", "candidate_rewards": []any{}, "reward_items": []any{}, "audit": workflowHandlerAudit(node, runtime)}
		if strings.EqualFold(workflowConfigString(config, "notQualifiedAction"), "BLOCK") {
			return nodeFailed("CANDIDATE_REWARD_NOT_QUALIFIED: activity not qualified", output)
		}
		return nodeSkipped("CANDIDATE_REWARD_NOT_QUALIFIED: activity not qualified", output)
	}
	items := workflowCandidateRewardItems(ctx, runtime, config)
	if len(items) == 0 {
		output := map[string]any{"errorCode": "CANDIDATE_REWARD_EMPTY", "candidate_rewards": []any{}, "reward_items": []any{}, "audit": workflowHandlerAudit(node, runtime)}
		if strings.EqualFold(workflowConfigString(config, "emptyAction"), "SKIP") {
			return nodeSkipped("CANDIDATE_REWARD_EMPTY: no reward candidates", output)
		}
		return nodeFailed("CANDIDATE_REWARD_EMPTY: no reward candidates", output)
	}
	return nodeSuccess(map[string]any{
		"candidate_rewards": items,
		"reward_items":      items,
		"candidateCount":    len(items),
		"source":            firstWorkflowString(workflowConfigString(config, "source"), "ACTIVITY_REWARD_CONFIG"),
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func codeWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	language := strings.ToUpper(workflowConfigString(config, "language"))
	source := workflowConfigString(config, "source")
	if language == "" {
		language = "EXPRESSION"
	}
	if source == "" {
		return nodeFailed("CODE_CONFIG_SOURCE_REQUIRED: source is required", map[string]any{
			"errorCode": "CODE_CONFIG_SOURCE_REQUIRED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	result := map[string]any{
		"language": language,
		"source":   source,
		"eventId":  runtime.Inbox.Event.EventID,
	}
	if language == "LUA" {
		luaOutput, err := executeWorkflowLua(ctx, node, runtime, source)
		if err != nil {
			return nodeFailed("CODE_LUA_EXECUTION_FAILED: "+err.Error(), map[string]any{
				"errorCode": "CODE_LUA_EXECUTION_FAILED",
				"language":  language,
				"eventId":   runtime.Inbox.Event.EventID,
				"audit":     workflowHandlerAudit(node, runtime),
			})
		}
		for key, value := range luaOutput {
			result[key] = value
		}
		result["value"] = luaOutput
	} else if language == "EXPRESSION" {
		if value, err := extractWorkflowJSONPath(workflowJSONValue(runtime.Inbox.Event), source); err == nil {
			result["value"] = value
		} else {
			result["error"] = err.Error()
		}
	} else {
		return nodeFailed("CODE_LANGUAGE_UNSUPPORTED: language "+language+" is not supported", map[string]any{
			"errorCode": "CODE_LANGUAGE_UNSUPPORTED",
			"language":  language,
			"eventId":   runtime.Inbox.Event.EventID,
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	outputPath := workflowConfigString(config, "outputPath")
	if outputPath == "" {
		outputPath = "code_result"
	}
	return nodeSuccess(map[string]any{
		"code_result": result,
		outputPath:    result,
		"nodeType":    nodeHandlerKey(node),
		"language":    language,
		"outputPath":  outputPath,
		"eventId":     runtime.Inbox.Event.EventID,
		"audit":       workflowHandlerAudit(node, runtime),
	})
}

func (s *Service) codeWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return s.codeWorkflowNodeHandlerWithLua(ctx, node, runtime, s.executeWorkflowLua)
}

func (s *Service) codeWorkflowNodeHandlerWithLua(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, luaExecutor func(context.Context, marketing.WorkflowNode, workflowRuntime, string) (map[string]any, error)) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	language := strings.ToUpper(workflowConfigString(config, "language"))
	source := workflowConfigString(config, "source")
	if language == "" {
		language = "EXPRESSION"
	}
	if source == "" {
		return nodeFailed("CODE_CONFIG_SOURCE_REQUIRED: source is required", map[string]any{"errorCode": "CODE_CONFIG_SOURCE_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
	}
	result := map[string]any{"language": language, "source": source, "eventId": runtime.Inbox.Event.EventID}
	if language == "LUA" {
		luaOutput, err := luaExecutor(ctx, node, runtime, source)
		if err != nil {
			return nodeFailed("CODE_LUA_EXECUTION_FAILED: "+err.Error(), map[string]any{"errorCode": "CODE_LUA_EXECUTION_FAILED", "language": language, "eventId": runtime.Inbox.Event.EventID, "audit": workflowHandlerAudit(node, runtime)})
		}
		for key, value := range luaOutput {
			result[key] = value
		}
		result["value"] = luaOutput
	} else if language == "EXPRESSION" {
		if value, err := extractWorkflowJSONPath(workflowJSONValue(runtime.Inbox.Event), source); err == nil {
			result["value"] = value
		} else {
			result["error"] = err.Error()
		}
	} else {
		return nodeFailed("CODE_LANGUAGE_UNSUPPORTED: language "+language+" is not supported", map[string]any{"errorCode": "CODE_LANGUAGE_UNSUPPORTED", "language": language, "eventId": runtime.Inbox.Event.EventID, "audit": workflowHandlerAudit(node, runtime)})
	}
	outputPath := workflowConfigString(config, "outputPath")
	if outputPath == "" {
		outputPath = "code_result"
	}
	return nodeSuccess(map[string]any{"code_result": result, outputPath: result, "nodeType": nodeHandlerKey(node), "language": language, "outputPath": outputPath, "eventId": runtime.Inbox.Event.EventID, "audit": workflowHandlerAudit(node, runtime)})
}

func transformScriptWorkflowNodeHandler(kind string, defaultOutputPath string) workflowNodeHandler {
	return func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
			return canceled
		}
		config, failed, ok := workflowRequireNodeConfig(node)
		if !ok {
			failed.Output["audit"] = workflowHandlerAudit(node, runtime)
			return failed
		}
		language := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "language"), "LUA"))
		source := workflowConfigString(config, "source")
		if source == "" {
			code := strings.ToUpper(kind)
			return nodeFailed(code+"_CONFIG_SOURCE_REQUIRED: source is required", map[string]any{
				"errorCode": code + "_CONFIG_SOURCE_REQUIRED",
				"audit":     workflowHandlerAudit(node, runtime),
			})
		}
		outputPath := firstWorkflowString(workflowConfigString(config, "outputPath"), defaultOutputPath)
		result := map[string]any{
			"nodeType":   nodeHandlerKey(node),
			"language":   language,
			"source":     source,
			"outputPath": outputPath,
			"eventId":    runtime.Inbox.Event.EventID,
		}
		var value any
		switch language {
		case "LUA":
			luaOutput, err := executeWorkflowLua(ctx, node, runtime, source)
			if err != nil {
				code := strings.ToUpper(kind)
				return nodeFailed(code+"_LUA_EXECUTION_FAILED: "+err.Error(), map[string]any{
					"errorCode": code + "_LUA_EXECUTION_FAILED",
					"language":  language,
					"eventId":   runtime.Inbox.Event.EventID,
					"audit":     workflowHandlerAudit(node, runtime),
				})
			}
			result["value"] = luaOutput
			for key, item := range luaOutput {
				result[key] = item
			}
			if item, ok := luaOutput[outputPath]; ok {
				value = item
			} else if kind == "split" {
				if item, ok := luaOutput["items"]; ok {
					value = item
				} else {
					value = luaOutput
				}
			} else if item, ok := luaOutput["combined"]; ok {
				value = item
			} else {
				value = luaOutput
			}
		case "EXPRESSION":
			expressionInput := workflowJSONValue(runtime.Inbox.Event)
			if resolved, ok := workflowResolvedInputFromContext(ctx); ok && len(resolved) > 0 {
				expressionInput = resolved
			}
			extracted, err := extractWorkflowJSONPath(expressionInput, source)
			if err != nil {
				result["error"] = err.Error()
				value = nil
			} else {
				value = extracted
			}
			result["value"] = value
		default:
			code := strings.ToUpper(kind)
			return nodeFailed(code+"_LANGUAGE_UNSUPPORTED: language "+language+" is not supported", map[string]any{
				"errorCode": code + "_LANGUAGE_UNSUPPORTED",
				"language":  language,
				"eventId":   runtime.Inbox.Event.EventID,
				"audit":     workflowHandlerAudit(node, runtime),
			})
		}
		if kind == "split" {
			result["split_items"] = value
			result["split_result"] = map[string]any{
				"items": value,
				"value": result["value"],
			}
			return nodeSuccess(map[string]any{
				"split_items":  value,
				"split_result": result["split_result"],
				outputPath:     value,
				"nodeType":     nodeHandlerKey(node),
				"language":     language,
				"outputPath":   outputPath,
				"eventId":      runtime.Inbox.Event.EventID,
				"audit":        workflowHandlerAudit(node, runtime),
			})
		}
		result["combined_result"] = value
		return nodeSuccess(map[string]any{
			"combined_result": value,
			outputPath:        value,
			"nodeType":        nodeHandlerKey(node),
			"language":        language,
			"outputPath":      outputPath,
			"eventId":         runtime.Inbox.Event.EventID,
			"audit":           workflowHandlerAudit(node, runtime),
		})
	}
}

func (s *Service) transformScriptWorkflowNodeHandler(kind string, defaultOutputPath string) workflowNodeHandler {
	return func(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
			return canceled
		}
		config, failed, ok := workflowRequireNodeConfig(node)
		if !ok {
			failed.Output["audit"] = workflowHandlerAudit(node, runtime)
			return failed
		}
		language := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "language"), "LUA"))
		source := workflowConfigString(config, "source")
		if source == "" {
			code := strings.ToUpper(kind)
			return nodeFailed(code+"_CONFIG_SOURCE_REQUIRED: source is required", map[string]any{"errorCode": code + "_CONFIG_SOURCE_REQUIRED", "audit": workflowHandlerAudit(node, runtime)})
		}
		outputPath := firstWorkflowString(workflowConfigString(config, "outputPath"), defaultOutputPath)
		result := map[string]any{"nodeType": nodeHandlerKey(node), "language": language, "source": source, "outputPath": outputPath, "eventId": runtime.Inbox.Event.EventID}
		var value any
		switch language {
		case "LUA":
			luaOutput, err := s.executeWorkflowLua(ctx, node, runtime, source)
			if err != nil {
				code := strings.ToUpper(kind)
				return nodeFailed(code+"_LUA_EXECUTION_FAILED: "+err.Error(), map[string]any{"errorCode": code + "_LUA_EXECUTION_FAILED", "language": language, "eventId": runtime.Inbox.Event.EventID, "audit": workflowHandlerAudit(node, runtime)})
			}
			result["value"] = luaOutput
			for key, item := range luaOutput {
				result[key] = item
			}
			if item, ok := luaOutput[outputPath]; ok {
				value = item
			} else if kind == "split" {
				if item, ok := luaOutput["items"]; ok {
					value = item
				} else {
					value = luaOutput
				}
			} else if item, ok := luaOutput["combined"]; ok {
				value = item
			} else {
				value = luaOutput
			}
		case "EXPRESSION":
			expressionInput := workflowJSONValue(runtime.Inbox.Event)
			if resolved, ok := workflowResolvedInputFromContext(ctx); ok && len(resolved) > 0 {
				expressionInput = resolved
			}
			extracted, err := extractWorkflowJSONPath(expressionInput, source)
			if err != nil {
				result["error"] = err.Error()
				value = nil
			} else {
				value = extracted
			}
		default:
			code := strings.ToUpper(kind)
			return nodeFailed(code+"_LANGUAGE_UNSUPPORTED: language "+language+" is not supported", map[string]any{"errorCode": code + "_LANGUAGE_UNSUPPORTED", "language": language, "audit": workflowHandlerAudit(node, runtime)})
		}
		result[outputPath] = value
		return nodeSuccess(map[string]any{outputPath: value, kind + "_result": result, "value": value, "nodeType": nodeHandlerKey(node), "language": language, "audit": workflowHandlerAudit(node, runtime)})
	}
}

func workflowResolvedInputFromContext(ctx context.Context) (map[string]any, bool) {
	executionContext, ok := ctx.Value(workflowNodeExecutionContextKey{}).(workflowNodeExecutionContext)
	if !ok || executionContext.ResolvedInput == nil {
		return nil, false
	}
	return executionContext.ResolvedInput, true
}

func rewardWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	if !runtime.Qualified {
		return nodeSkipped("REWARD_NOT_QUALIFIED: activity not qualified", map[string]any{
			"errorCode": "REWARD_NOT_QUALIFIED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	if len(runtime.Activity.Rewards) == 0 {
		return nodeFailed("REWARD_CONFIG_REQUIRED: no rewards configured", map[string]any{
			"errorCode": "REWARD_CONFIG_REQUIRED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	items := make([]any, 0, len(runtime.Activity.Rewards))
	calculator := rewardcalc.ConfigCalculator{}
	for _, reward := range runtime.Activity.Rewards {
		item := map[string]any{
			"rewardId":      reward.ID,
			"rewardCode":    reward.Code,
			"rewardType":    reward.Type,
			"calculateType": reward.CalculateType,
			"grantMode":     reward.GrantMode,
			"grantChannel":  reward.GrantChannel,
		}
		if reward.Enabled && len(reward.Config) > 0 {
			calculated, err := calculator.Calculate(rewardcalc.Context{
				Event:          runtime.Inbox.Event,
				TenantID:       runtime.Activity.TenantID,
				ActivityID:     runtime.Activity.ID,
				IdempotencyKey: runtime.Inbox.IdempotencyKey,
				Facts:          workflowFactsFromContext(ctx),
			}, reward)
			if err != nil {
				return nodeFailed("REWARD_CALCULATION_FAILED: "+err.Error(), map[string]any{
					"errorCode":  "REWARD_CALCULATION_FAILED",
					"rewardCode": reward.Code,
					"audit":      workflowHandlerAudit(node, runtime),
				})
			}
			item["amount"] = calculated.Amount
			item["currency"] = calculated.Currency
			item["payload"] = calculated.Payload
			item["reasonCode"] = calculated.ReasonCode
			item["reason"] = calculated.Reason
			item["skipped"] = calculated.Skipped
		} else if grant := workflowGrantOrderForReward(runtime.GrantOrders, reward); grant.ID != "" {
			item["amount"] = grant.RewardAmount
			item["currency"] = workflowGrantOrderCurrency(grant)
			item["grantOrderId"] = grant.ID
			item["billNo"] = grant.BillNo
		}
		items = append(items, item)
	}
	return nodeSuccess(map[string]any{
		"reward_items":          items,
		"configuredRewardCount": len(runtime.Activity.Rewards),
		"grantOrderCount":       len(runtime.GrantOrders),
		"audit":                 workflowHandlerAudit(node, runtime),
	})
}

func workflowCandidateRewardItems(ctx context.Context, runtime workflowRuntime, config map[string]any) []any {
	source := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "source"), "ACTIVITY_REWARD_CONFIG"))
	if source == "INPUT" || source == "PLUGIN_RESULT" {
		if value, ok := workflowResolvedInput(ctx, "plugin_result"); ok {
			if items := workflowAnyArray(value); len(items) > 0 {
				return items
			}
		}
		for _, key := range []string{"candidate_rewards", "reward_items", "selected_rewards"} {
			if value, ok := workflowUpstreamOutput(ctx, key); ok {
				if items := workflowAnyArray(value); len(items) > 0 {
					return items
				}
			}
		}
	}
	items := make([]any, 0, len(runtime.Activity.Rewards))
	calculator := rewardcalc.ConfigCalculator{}
	includeDisabled := workflowConfigBool(config, "includeDisabledRewards", false)
	for _, reward := range runtime.Activity.Rewards {
		if !reward.Enabled && !includeDisabled {
			continue
		}
		item := map[string]any{
			"rewardId":      reward.ID,
			"rewardCode":    reward.Code,
			"rewardType":    reward.Type,
			"calculateType": reward.CalculateType,
			"grantMode":     reward.GrantMode,
			"grantChannel":  reward.GrantChannel,
			"candidate":     true,
		}
		if reward.Enabled && len(reward.Config) > 0 {
			if calculated, err := calculator.Calculate(rewardcalc.Context{
				Event:          runtime.Inbox.Event,
				TenantID:       runtime.Activity.TenantID,
				ActivityID:     runtime.Activity.ID,
				IdempotencyKey: runtime.Inbox.IdempotencyKey,
				Facts:          workflowFactsFromContext(ctx),
			}, reward); err == nil {
				item["amount"] = calculated.Amount
				item["currency"] = calculated.Currency
				item["payload"] = calculated.Payload
				item["reasonCode"] = calculated.ReasonCode
				item["reason"] = calculated.Reason
				item["skipped"] = calculated.Skipped
			}
		}
		items = append(items, item)
	}
	return items
}

func decimalWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if !runtime.Qualified {
		return nodeSkipped("DECIMAL_NOT_QUALIFIED: activity not qualified", map[string]any{
			"errorCode": "DECIMAL_NOT_QUALIFIED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	raw, ok := workflowResolvedInput(ctx, "amount_expr")
	source := "input.amount_expr"
	if !ok {
		if value, ok := workflowUpstreamOutput(ctx, "decimal_amount"); ok {
			raw = value
			source = "upstream.decimal_amount"
			ok = true
		}
	}
	if !ok {
		if value, ok := workflowUpstreamOutput(ctx, "amount"); ok {
			raw = value
			source = "upstream.amount"
			ok = true
		}
	}
	if !ok {
		config, _, hasConfig := workflowRequireNodeConfig(node)
		if hasConfig {
			if value := firstWorkflowString(workflowConfigString(config, "amountExpr"), workflowConfigString(config, "amount")); value != "" {
				raw = value
				source = "config.amountExpr"
				ok = true
			}
		}
	}
	if !ok {
		return nodeFailed("DECIMAL_INPUT_REQUIRED: amount_expr is required", map[string]any{
			"errorCode": "DECIMAL_INPUT_REQUIRED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	value, err := exdecimal.Parse(workflowOutputString(raw))
	if err != nil {
		return nodeFailed("DECIMAL_PARSE_FAILED: amount_expr is invalid", map[string]any{
			"errorCode": "DECIMAL_PARSE_FAILED",
			"source":    source,
			"rawValue":  workflowOutputString(raw),
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	return nodeSuccess(map[string]any{
		"decimal_amount": value.String(),
		"rawValue":       workflowOutputString(raw),
		"source":         source,
		"precision":      "Decimal(18, 8)",
		"audit":          workflowHandlerAudit(node, runtime),
	})
}

func limitWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	maxCount := workflowConfigNumber(config, "maxCount")
	if maxCount < 0 {
		return nodeFailed("LIMIT_CONFIG_INVALID: maxCount must be non-negative", map[string]any{
			"errorCode": "LIMIT_CONFIG_INVALID",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	if maxCount == 0 {
		maxCount = 1
	}
	hold := map[string]any{
		"scope":        firstWorkflowString(workflowConfigString(config, "scope"), "USER"),
		"period":       firstWorkflowString(workflowConfigString(config, "period"), "ACTIVITY"),
		"maxCount":     maxCount,
		"subject":      runtime.Inbox.Event.Subject.ID,
		"scopeKey":     workflowLimitScopeKey(config, runtime),
		"consumeMode":  firstWorkflowString(workflowConfigString(config, "consumeMode"), "ATOMIC"),
		"exceedAction": firstWorkflowString(workflowConfigString(config, "exceedAction"), "BLOCK"),
		"allowed":      runtime.Qualified,
		"reservations": []any{
			workflowJSONValue(marketing.QuotaReservation{
				Type:     workflowLimitQuotaType(config),
				Scope:    workflowQuotaScope(workflowConfigString(config, "scope"), marketing.QuotaScopeUser),
				ScopeKey: workflowLimitScopeKey(config, runtime),
				Amount:   "1",
			}),
		},
	}
	return nodeSuccess(map[string]any{
		"limit_hold": hold,
		"decision":   normalizeWorkflowDecisionValue(string(runtime.RuleResult.Decision)),
		"reason":     runtime.RuleResult.Reason,
		"audit":      workflowHandlerAudit(node, runtime),
	})
}

func quotaWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	if !runtime.Qualified {
		return nodeSkipped("QUOTA_NOT_QUALIFIED: activity not qualified", map[string]any{
			"errorCode": "QUOTA_NOT_QUALIFIED",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	hold := map[string]any{
		"status":          "planned",
		"quotaCode":       workflowConfigString(config, "quotaCode"),
		"quotaType":       firstWorkflowString(workflowConfigString(config, "quotaType"), string(marketing.QuotaBudget)),
		"scope":           firstWorkflowString(workflowConfigString(config, "scope"), string(marketing.QuotaScopeActivity)),
		"scopeKey":        workflowQuotaScopeKey(config, runtime),
		"consumeMode":     firstWorkflowString(workflowConfigString(config, "consumeMode"), "ATOMIC"),
		"amountField":     firstWorkflowString(workflowConfigString(config, "amountField"), "reward_items[0].amount"),
		"plannedAmount":   workflowQuotaPlannedAmount(ctx, runtime, config),
		"grantOrders":     len(runtime.GrantOrders),
		"activityId":      runtime.Activity.ID,
		"participation":   runtime.Participation.ID,
		"commitAuthority": "CreateParticipationGrantOutboxesWithQuota",
	}
	hold["reservations"] = []any{workflowJSONValue(marketing.QuotaReservation{
		Type:     workflowQuotaType(workflowConfigString(config, "quotaType"), marketing.QuotaBudget),
		Scope:    workflowQuotaScope(workflowConfigString(config, "scope"), marketing.QuotaScopeActivity),
		ScopeKey: workflowQuotaScopeKey(config, runtime),
		Amount:   workflowOutputString(hold["plannedAmount"]),
		Status:   marketing.QuotaReservationFrozen,
	})}
	if len(runtime.Reservations) > 0 {
		committed := make([]any, 0, len(runtime.Reservations))
		for _, reservation := range runtime.Reservations {
			committed = append(committed, workflowJSONValue(reservation))
		}
		hold["committedReservations"] = committed
		hold["status"] = "committed"
	}
	return nodeSuccess(map[string]any{
		"quota_hold": hold,
		"status":     hold["status"],
		"audit":      workflowHandlerAudit(node, runtime),
	})
}

func budgetReserveWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	result := quotaWorkflowNodeHandler(ctx, node, runtime)
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	if hold, ok := result.Output["quota_hold"]; ok {
		result.Output["budget_reservation"] = hold
	}
	result.Output["nodeType"] = nodeHandlerKey(node)
	result.Output["reserveMode"] = workflowConfigString(workflowNodeConfigMap(node), "reserveMode")
	result.Output["releaseOnFailure"] = workflowConfigBool(workflowNodeConfigMap(node), "releaseOnFailure", true)
	return result
}

func participationWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Participation.ID == "" {
		return nodeFailed("participation was not created", nil)
	}
	return nodeSuccess(map[string]any{
		"participation_id": runtime.Participation.ID,
		"participationId":  runtime.Participation.ID,
		"status":           runtime.Participation.Status,
		"commitMode":       "runtime_committed",
		"commitAuthority":  "CreateParticipationGrantOutboxesWithQuota",
		"audit":            workflowHandlerAudit(node, runtime),
	})
}

func auditWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	action := workflowConfigString(config, "action")
	if action == "" {
		action = "WORKFLOW_NODE"
	}
	auditLog := map[string]any{
		"action":          action,
		"eventId":         runtime.Inbox.Event.EventID,
		"activityId":      runtime.Activity.ID,
		"participationId": runtime.Participation.ID,
		"qualified":       runtime.Qualified,
		"decision":        normalizeWorkflowDecisionValue(string(runtime.RuleResult.Decision)),
	}
	if workflowConfigBool(config, "includePayload", true) {
		auditLog["event"] = workflowJSONValue(runtime.Inbox.Event)
	}
	return nodeSuccess(map[string]any{
		"audit_log":  auditLog,
		"eventId":    runtime.Inbox.Event.EventID,
		"activityId": runtime.Activity.ID,
		"audit":      workflowHandlerAudit(node, runtime),
	})
}

func grantOrderWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("no grant orders created", nil)
	}
	orders := make([]any, 0, len(runtime.GrantOrders))
	for _, order := range runtime.GrantOrders {
		orders = append(orders, workflowJSONValue(order))
	}
	return nodeSuccess(map[string]any{
		"grant_order":     workflowJSONValue(runtime.GrantOrders[0]),
		"grant_orders":    orders,
		"grantOrderCount": len(runtime.GrantOrders),
		"commitMode":      "runtime_committed",
		"commitAuthority": "CreateParticipationGrantOutboxesWithQuota",
		"audit":           workflowHandlerAudit(node, runtime),
	})
}

func outboxWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.OutboxMessages) == 0 {
		return nodeSkipped("no outbox messages created", nil)
	}
	output := map[string]any{
		"HTTP":            []any{},
		"MQ":              []any{},
		"Webhook":         []any{},
		"Failed":          []any{},
		"outboxCount":     len(runtime.OutboxMessages),
		"commitMode":      "runtime_committed",
		"commitAuthority": "CreateParticipationGrantOutboxesWithQuota",
		"audit":           workflowHandlerAudit(node, runtime),
	}
	for _, message := range runtime.OutboxMessages {
		item := workflowJSONValue(message)
		switch {
		case message.SendStatus == marketing.OutboxFailed || message.SendStatus == marketing.OutboxDLQ:
			output["Failed"] = append(output["Failed"].([]any), item)
		case message.TargetChannel == marketing.GrantHTTP:
			output["HTTP"] = append(output["HTTP"].([]any), item)
		case message.TargetChannel == marketing.GrantMQ:
			output["MQ"] = append(output["MQ"].([]any), item)
		case message.TargetChannel == marketing.GrantWebhook:
			output["Webhook"] = append(output["Webhook"].([]any), item)
		default:
			output["Failed"] = append(output["Failed"].([]any), item)
		}
	}
	return nodeSuccess(output)
}

func (s *Service) outboxWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef")); resourceRef != "" {
		config, failed, ok := workflowRequireNodeConfig(node)
		if !ok {
			failed.Output["audit"] = workflowHandlerAudit(node, runtime)
			return failed
		}
		result := outboxWorkflowNodeHandler(ctx, node, runtime)
		if result.Status != marketing.WorkflowNodeSuccess {
			return result
		}
		resource, err := s.GetResource(ctx, runtime.Activity.TenantID, resourceRef)
		if err != nil {
			return nodeFailed("RESOURCE_NOT_FOUND: "+err.Error(), map[string]any{"errorCode": "RESOURCE_NOT_FOUND", "resourceRef": resourceRef, "audit": workflowHandlerAudit(node, runtime)})
		}
		operation := workflowOutboxResourceOperation(resource)
		channel := workflowOutboxResourceChannel(resource, config)
		key := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "keyTemplate"), "${activityId}:${eventId}:${nodeId}"), runtime, node)
		accessReq := marketing.ResourceAccessRequest{
			TenantID:    runtime.Activity.TenantID,
			WorkspaceID: runtime.Activity.WorkspaceID,
			FolderID:    runtime.Activity.FolderID,
			ActivityID:  runtime.Activity.ID,
			NodeID:      node.ID,
			NodeType:    nodeHandlerKey(node),
			Operation:   workflowStateStoreResourceOperation(operation),
			Namespace:   firstWorkflowString(workflowConfigString(config, "namespace"), "outbox"),
			ResourceKey: key,
		}
		if err := s.ValidateResourceAccess(ctx, resource, accessReq); err != nil {
			code := firstWorkflowString(strings.TrimSpace(err.Error()), "RESOURCE_PERMISSION_DENIED")
			return nodeFailed(code+": resource access denied", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
		}
		if code := resourceAdapterConfigError(resource); code != "" {
			return nodeFailed(code+": resource adapter config is invalid", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
		}
		if workflowResourceApprovalRequired(resource, operation, config, runtime) {
			return nodeFailed("RESOURCE_APPROVAL_REQUIRED: high risk resource operation requires approval", map[string]any{
				"errorCode":    "RESOURCE_APPROVAL_REQUIRED",
				"resourceRef":  resourceRef,
				"resourceType": resource.Type,
				"operation":    operation,
				"audit":        workflowHandlerAudit(node, runtime),
			})
		}
		matched := workflowOutboxMessagesForChannel(runtime.OutboxMessages, channel)
		if result.Output == nil {
			result.Output = map[string]any{}
		}
		result.Output["resourceRef"] = resource.Code
		result.Output["resource"] = map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver}
		result.Output["resourceized"] = true
		result.Output["resourceChannel"] = channel
		result.Output["resourceMessageCount"] = len(matched)
		result.Output["resourceDispatchPlan"] = map[string]any{
			"operation":          operation,
			"namespace":          accessReq.Namespace,
			"key":                key,
			"targetRef":          workflowResourceTargetRef(resource),
			"executionMode":      "OUTBOX_INTENT_RESOURCE_BOUND",
			"executionAuthority": "OutboxWorker",
		}
		_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, operation, result)
		return result
	}
	return outboxWorkflowNodeHandler(ctx, node, runtime)
}

func workflowOutboxResourceOperation(resource marketing.Resource) marketing.ResourceOperation {
	if resource.Type == marketing.ResourceMQTopic {
		return marketing.ResourcePublish
	}
	return marketing.ResourceCall
}

func workflowOutboxResourceChannel(resource marketing.Resource, config map[string]any) marketing.GrantChannel {
	if channel := strings.ToUpper(workflowConfigString(config, "channel")); channel != "" {
		return marketing.GrantChannel(channel)
	}
	switch resource.Type {
	case marketing.ResourceMQTopic:
		return marketing.GrantMQ
	case marketing.ResourceHTTPProvider:
		return marketing.GrantHTTP
	default:
		return marketing.GrantWebhook
	}
}

func workflowOutboxMessagesForChannel(messages []marketing.OutboxMessage, channel marketing.GrantChannel) []any {
	items := make([]any, 0)
	for _, message := range messages {
		if message.TargetChannel == channel {
			items = append(items, workflowJSONValue(message))
		}
	}
	return items
}

func workflowResourceTargetRef(resource marketing.Resource) string {
	config := resource.ConfigMap()
	switch resource.Type {
	case marketing.ResourceMQTopic:
		return firstWorkflowString(resourceConfigString(config, "destination"), firstWorkflowString(resourceConfigString(config, "targetRef"), resourceConfigString(config, "topic")))
	case marketing.ResourceHTTPProvider:
		return firstWorkflowString(resourceConfigString(config, "endpoint"), resourceConfigString(config, "providerEndpoint"))
	default:
		return firstWorkflowString(resourceConfigString(config, "targetRef"), resource.Code)
	}
}

func retryWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.OutboxMessages) == 0 {
		return nodeSkipped("no dispatch retry required", map[string]any{
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "OutboxWorker",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
	return nodeSuccess(map[string]any{
		"retryPolicy":        "outbox exponential backoff",
		"executionMode":      "EXPLAIN_ONLY",
		"executionAuthority": "OutboxWorker",
		"messageCount":       len(runtime.OutboxMessages),
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func dlqWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	return nodeSkipped("no exhausted retry in current execution", map[string]any{
		"executionMode":      "EXPLAIN_ONLY",
		"executionAuthority": "OutboxWorker/Admin DLQ API",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func dispatchWorkflowNodeHandler(channel string) workflowNodeHandler {
	return func(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
		count := 0
		for _, message := range runtime.OutboxMessages {
			if strings.EqualFold(string(message.TargetChannel), channel) {
				count++
			}
		}
		if count == 0 {
			return nodeSkipped("no "+channel+" dispatch", map[string]any{
				"channel":            channel,
				"executionMode":      "EXPLAIN_ONLY",
				"executionAuthority": "OutboxWorker",
				"audit":              workflowHandlerAudit(node, runtime),
			})
		}
		return nodeSuccess(map[string]any{
			"channel":            channel,
			"messageCount":       count,
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "OutboxWorker",
			"explanation":        "Workflow node explains dispatch intent; transport delivery is performed by OutboxWorker.",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
}

func (s *Service) mqPublishWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef")); resourceRef != "" {
		config, failed, ok := workflowRequireNodeConfig(node)
		if !ok {
			failed.Output["audit"] = workflowHandlerAudit(node, runtime)
			return failed
		}
		payload := workflowResolvedInputValue(ctx)
		if payload == nil {
			payload = firstWorkflowAny(config["payload"], map[string]any{
				"eventId":    runtime.Inbox.Event.EventID,
				"activityId": runtime.Activity.ID,
				"subject":    workflowJSONValue(runtime.Inbox.Event.Subject),
			})
		}
		output, err := s.workflowExecuteResourceAPI(ctx, node, runtime, workflowResourceAPIRequest{
			Operation:   marketing.ResourcePublish,
			ResourceRef: resourceRef,
			Namespace:   firstWorkflowString(workflowConfigString(config, "namespace"), "mq"),
			Key:         workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "keyTemplate"), "${activityId}:${eventId}:${nodeId}"), runtime, node),
			Value:       payload,
			ValueType:   "OBJECT",
		})
		if err != nil {
			return nodeFailed(err.Error(), output)
		}
		return nodeSuccess(map[string]any{
			"mq_result": output,
			"MQ":        []any{output},
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	return dispatchWorkflowNodeHandler("MQ")(ctx, node, runtime)
}

func callbackWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("CALLBACK_NO_GRANT_ORDER: no grant callback expected", map[string]any{
			"errorCode": "CALLBACK_NO_GRANT_ORDER",
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	result := map[string]any{
		"awaitingCallback":  true,
		"grantOrderCount":   len(runtime.GrantOrders),
		"provider":          workflowConfigString(config, "provider"),
		"requireSignature":  config["requireSignature"],
		"executionMode":     "EXTERNAL_ENDPOINT",
		"terminalAuthority": "RewardCallback/Reconciliation",
	}
	return nodeSuccess(map[string]any{
		"callback_result":   result,
		"awaitingCallback":  true,
		"grantOrderCount":   len(runtime.GrantOrders),
		"executionMode":     "EXTERNAL_ENDPOINT",
		"terminalAuthority": "RewardCallback/Reconciliation",
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func (s *Service) callbackWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if resourceRef := strings.TrimSpace(workflowExplicitConfigString(node, "resourceRef")); resourceRef != "" {
		result := callbackWorkflowNodeHandler(ctx, node, runtime)
		if result.Status != marketing.WorkflowNodeSuccess {
			return result
		}
		config := workflowNodeConfigMap(node)
		resource, err := s.GetResource(ctx, runtime.Activity.TenantID, resourceRef)
		if err != nil {
			return nodeFailed("RESOURCE_NOT_FOUND: "+err.Error(), map[string]any{"errorCode": "RESOURCE_NOT_FOUND", "resourceRef": resourceRef, "audit": workflowHandlerAudit(node, runtime)})
		}
		key := workflowRenderNodeTemplate(firstWorkflowString(workflowConfigString(config, "keyTemplate"), "${activityId}:${eventId}:${nodeId}"), runtime, node)
		accessReq := marketing.ResourceAccessRequest{
			TenantID:    runtime.Activity.TenantID,
			WorkspaceID: runtime.Activity.WorkspaceID,
			FolderID:    runtime.Activity.FolderID,
			ActivityID:  runtime.Activity.ID,
			NodeID:      node.ID,
			NodeType:    nodeHandlerKey(node),
			Operation:   marketing.ResourceCall,
			Namespace:   firstWorkflowString(workflowConfigString(config, "namespace"), "callback"),
			ResourceKey: key,
		}
		if err := s.ValidateResourceAccess(ctx, resource, accessReq); err != nil {
			code := firstWorkflowString(strings.TrimSpace(err.Error()), "RESOURCE_PERMISSION_DENIED")
			return nodeFailed(code+": resource access denied", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
		}
		if code := resourceAdapterConfigError(resource); code != "" {
			return nodeFailed(code+": resource adapter config is invalid", map[string]any{"errorCode": code, "resourceRef": resourceRef, "resourceType": resource.Type, "audit": workflowHandlerAudit(node, runtime)})
		}
		if workflowResourceApprovalRequired(resource, marketing.ResourceCall, config, runtime) {
			return nodeFailed("RESOURCE_APPROVAL_REQUIRED: high risk resource operation requires approval", map[string]any{
				"errorCode":    "RESOURCE_APPROVAL_REQUIRED",
				"resourceRef":  resourceRef,
				"resourceType": resource.Type,
				"operation":    marketing.ResourceCall,
				"audit":        workflowHandlerAudit(node, runtime),
			})
		}
		if result.Output == nil {
			result.Output = map[string]any{}
		}
		result.Output["resourceRef"] = resource.Code
		result.Output["resource"] = map[string]any{"id": resource.ID, "code": resource.Code, "type": resource.Type, "driver": resource.Driver}
		result.Output["resourceized"] = true
		result.Output["callbackResourcePlan"] = map[string]any{
			"operation":          marketing.ResourceCall,
			"namespace":          accessReq.Namespace,
			"key":                key,
			"targetRef":          workflowResourceTargetRef(resource),
			"executionMode":      "EXTERNAL_ENDPOINT_RESOURCE_BOUND",
			"executionAuthority": "RewardCallback/Reconciliation",
		}
		if callbackResult, ok := result.Output["callback_result"].(map[string]any); ok {
			callbackResult["resourceRef"] = resource.Code
			callbackResult["targetRef"] = workflowResourceTargetRef(resource)
		}
		_ = s.workflowRecordResourceAudit(ctx, resource, runtime, node, marketing.ResourceCall, result)
		return result
	}
	return callbackWorkflowNodeHandler(ctx, node, runtime)
}

func approvalGateWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	mode := strings.ToUpper(firstWorkflowString(workflowConfigString(config, "mode"), "AUTO_BELOW_THRESHOLD"))
	amount := workflowWorkflowRewardAmount(ctx, runtime)
	threshold := workflowConfigNumber(config, "amountThreshold")
	decision := "APPROVED"
	reason := "auto approved"
	if mode == "MANUAL_REQUIRED" {
		decision, reason = "PENDING", "manual approval required"
	} else if mode == "RISK_REQUIRED" {
		decision, reason = "PENDING", "risk review required"
	} else if mode == "AUTO_BELOW_THRESHOLD" && threshold > 0 && amount > threshold {
		decision, reason = "PENDING", "amount exceeds approval threshold"
	}
	result := map[string]any{
		"decision":           decision,
		"reason":             reason,
		"mode":               mode,
		"amount":             amount,
		"amountThreshold":    threshold,
		"pendingAction":      firstWorkflowString(workflowConfigString(config, "pendingAction"), "PAUSE"),
		"executionMode":      "EXTERNAL_ENDPOINT",
		"executionAuthority": "ActivityApproval/AdminAPI",
	}
	return nodeSuccess(map[string]any{
		"approval_decision": result,
		"decision":          decision,
		"route":             decision,
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func compensationWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	plan := map[string]any{
		"triggerOn":          firstWorkflowString(workflowConfigString(config, "triggerOn"), "FAILED"),
		"sourcePath":         firstWorkflowString(workflowConfigString(config, "sourcePath"), "$.stage_result"),
		"action":             firstWorkflowString(workflowConfigString(config, "action"), "RETRY_CALLBACK"),
		"maxAttempts":        workflowConfigNumber(config, "maxAttempts"),
		"grantOrderCount":    len(runtime.GrantOrders),
		"reservationCount":   len(runtime.Reservations),
		"executionMode":      "EXPLAIN_ONLY",
		"executionAuthority": "CompensationWorker/AdminAPI",
	}
	return nodeSuccess(map[string]any{
		"compensation_plan": plan,
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func randomPickWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	items := workflowCandidateRewardItems(ctx, runtime, map[string]any{"source": "INPUT"})
	if len(items) == 0 {
		output := map[string]any{"errorCode": "RANDOM_PICK_EMPTY", "selected_rewards": []any{}, "audit": workflowHandlerAudit(node, runtime)}
		if strings.EqualFold(workflowConfigString(config, "emptyAction"), "BLOCK") {
			return nodeFailed("RANDOM_PICK_EMPTY: no candidates", output)
		}
		return nodeSkipped("RANDOM_PICK_EMPTY: no candidates", output)
	}
	pickCount := int(workflowConfigNumber(config, "pickCount"))
	if pickCount <= 0 {
		pickCount = 1
	}
	if pickCount > len(items) {
		pickCount = len(items)
	}
	seed := workflowRenderTemplate(firstWorkflowString(workflowConfigString(config, "seedTemplate"), "${tenantId}:${activityId}:${eventId}"), runtime)
	start := workflowStableIndex(seed, len(items))
	selected := make([]any, 0, pickCount)
	for i := 0; i < pickCount; i++ {
		selected = append(selected, items[(start+i)%len(items)])
	}
	audit := map[string]any{
		"mode":       firstWorkflowString(workflowConfigString(config, "mode"), "WEIGHTED"),
		"seed":       seed,
		"startIndex": start,
		"pickCount":  pickCount,
		"total":      len(items),
	}
	return nodeSuccess(map[string]any{
		"random_pick":      audit,
		"selected_rewards": selected,
		"audit":            workflowHandlerAudit(node, runtime),
	})
}

func pluginExecuteWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	result := map[string]any{
		"pluginCode":         workflowConfigString(config, "pluginCode"),
		"action":             workflowConfigString(config, "action"),
		"activityId":         runtime.Activity.ID,
		"subject":            workflowJSONValue(runtime.Inbox.Event.Subject),
		"executionMode":      "EXTERNAL_ENDPOINT",
		"executionAuthority": "ActivityPlugin",
		"planned":            true,
	}
	return nodeSuccess(map[string]any{
		"plugin_result":      result,
		"candidate_rewards":  []any{},
		"executionMode":      "EXTERNAL_ENDPOINT",
		"executionAuthority": "ActivityPlugin",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func abBucketWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	experiment := firstWorkflowString(workflowConfigString(config, "experimentKey"), "default-experiment")
	subject := workflowRenderTemplate(firstWorkflowString(workflowConfigString(config, "subjectKeyTemplate"), "${subject.id}"), runtime)
	buckets := workflowConfigStringList(config, "buckets")
	if len(buckets) == 0 {
		buckets = []string{"A:50", "B:50"}
	}
	bucket := workflowPickWeightedLabel(experiment+":"+subject, buckets)
	return nodeSuccess(map[string]any{
		"ab_bucket": map[string]any{
			"experimentKey": experiment,
			"subjectKey":    subject,
			"bucket":        bucket,
			"buckets":       buckets,
		},
		"bucket": bucket,
		"route":  bucket,
		"audit":  workflowHandlerAudit(node, runtime),
	})
}

func riskScoreWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	score := workflowExtractRiskScore(ctx, config)
	blockThreshold := workflowConfigNumber(config, "blockThreshold")
	reviewThreshold := workflowConfigNumber(config, "reviewThreshold")
	decision := "PASS"
	if score >= blockThreshold && blockThreshold > 0 {
		decision = "BLOCK"
	} else if score >= reviewThreshold && reviewThreshold > 0 {
		decision = "RISK"
	}
	result := map[string]any{
		"source":          firstWorkflowString(workflowConfigString(config, "source"), "FACTS"),
		"score":           score,
		"decision":        decision,
		"blockThreshold":  blockThreshold,
		"reviewThreshold": reviewThreshold,
	}
	return nodeSuccess(map[string]any{
		"risk_result": result,
		"decision":    decision,
		"route":       decision,
		"PASS":        decision == "PASS",
		"BLOCK":       decision == "BLOCK",
		"RISK":        decision == "RISK",
		"audit":       workflowHandlerAudit(node, runtime),
	})
}

func settlementWindowWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	inWindow := true
	reason := "window check is advisory when time expression is not parsed"
	start := workflowConfigString(config, "start")
	end := workflowConfigString(config, "end")
	if start != "" && end != "" {
		nowText := runtime.Inbox.Event.OccurredAt.UTC().Format("15:04")
		inWindow = start <= nowText && nowText <= end
		reason = "event occurred at " + nowText + " UTC"
	}
	action := "PASS"
	if !inWindow {
		action = strings.ToUpper(firstWorkflowString(workflowConfigString(config, "outsideAction"), "SKIP"))
	}
	result := map[string]any{
		"inWindow":      inWindow,
		"action":        action,
		"windowType":    firstWorkflowString(workflowConfigString(config, "windowType"), "DAILY"),
		"start":         start,
		"end":           end,
		"timezone":      firstWorkflowString(workflowConfigString(config, "timezone"), "UTC"),
		"reason":        reason,
		"executionMode": "REAL_HANDLER",
	}
	return nodeSuccess(map[string]any{
		"settlement_window": result,
		"decision":          action,
		"route":             action,
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func manualAdjustWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	adjustment := map[string]any{
		"adjustType":         firstWorkflowString(workflowConfigString(config, "adjustType"), "REWARD"),
		"targetPath":         firstWorkflowString(workflowConfigString(config, "targetPath"), "$.grant_order"),
		"requireReason":      workflowConfigBool(config, "requireReason", true),
		"requireApproval":    workflowConfigBool(config, "requireApproval", true),
		"executionMode":      "EXTERNAL_ENDPOINT",
		"executionAuthority": "AdminManualAdjustAPI",
		"planned":            true,
	}
	return nodeSuccess(map[string]any{
		"manual_adjustment": adjustment,
		"audit":             workflowHandlerAudit(node, runtime),
	})
}

func reconcileWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if len(runtime.GrantOrders) == 0 {
		return nodeSkipped("no grant order to reconcile", map[string]any{
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "Reconciliation/Admin API",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
	return nodeSuccess(map[string]any{
		"reconciliationTracked": true,
		"executionMode":         "EXPLAIN_ONLY",
		"executionAuthority":    "Reconciliation/Admin API",
		"grantOrderCount":       len(runtime.GrantOrders),
		"audit":                 workflowHandlerAudit(node, runtime),
	})
}

func rewardReconcileWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	result := reconcileWorkflowNodeHandler(ctx, node, runtime)
	if result.Output == nil {
		result.Output = map[string]any{}
	}
	result.Output["reward_reconcile_report"] = map[string]any{
		"reconciliationTracked": result.Output["reconciliationTracked"],
		"grantOrderCount":       len(runtime.GrantOrders),
		"reservationCount":      len(runtime.Reservations),
		"executionMode":         "EXPLAIN_ONLY",
		"executionAuthority":    "Reconciliation/Admin API",
	}
	return result
}

func alertWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Qualified {
		return nodeSkipped("no alert condition", map[string]any{
			"executionMode":      "EXPLAIN_ONLY",
			"executionAuthority": "Metrics/Alertmanager",
			"audit":              workflowHandlerAudit(node, runtime),
		})
	}
	return nodeSuccess(map[string]any{
		"reason":             runtime.RuleResult.Reason,
		"executionMode":      "EXPLAIN_ONLY",
		"executionAuthority": "Metrics/Alertmanager",
		"audit":              workflowHandlerAudit(node, runtime),
	})
}

func genericWorkflowNodeHandler(_ context.Context, node marketing.WorkflowNode, _ workflowRuntime) workflowNodeResult {
	rawConfig := map[string]any{}
	if len(node.Config) > 0 {
		_ = json.Unmarshal(node.Config, &rawConfig)
	}
	return nodeSuccess(map[string]any{"nodeType": nodeHandlerKey(node), "config": rawConfig, "executedAt": time.Now().UTC()})
}
