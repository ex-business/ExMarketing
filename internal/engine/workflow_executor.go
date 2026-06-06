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

type workflowNodeExecutionContext struct {
	ResolvedInput map[string]any
	NodeOutputs   map[string]map[string]any
	Variables     map[string]any
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
		canceled, cancelErr := s.workflowExecutionWasCanceled(ctx, execution.ID)
		if cancelErr != nil {
			return marketing.WorkflowExecution{}, cancelErr
		}
		if canceled {
			status = marketing.WorkflowExecutionCanceled
			errorMessage = "execution canceled"
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
		nodeStartedAt := s.now()
		nodeExecution := marketing.WorkflowNodeExecution{
			ID:          marketingID("wfnode"),
			TenantID:    runtime.Activity.TenantID,
			ExecutionID: execution.ID,
			NodeID:      node.ID,
			NodeType:    nodeHandlerKey(node),
			NodeTitle:   node.Title,
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
		} else {
			nodeCtx := context.WithValue(ctx, workflowNodeExecutionContextKey{}, workflowNodeExecutionContext{
				ResolvedInput: resolvedInput,
				NodeOutputs:   cloneWorkflowNodeOutputs(nodeOutputs),
				Variables:     workflowVars,
			})
			result = executeWorkflowNodeWithPolicy(nodeCtx, node, runtime, handler)
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
		"routeTrace":      routeTrace,
		"compileIssues":   compileIssues,
		"mode":            strings.ToUpper(opts.Mode),
		"debug":           !strings.EqualFold(opts.Mode, "PRODUCTION"),
		"startNodeId":     opts.StartNodeID,
		"variables":       cloneWorkflowVars(workflowVars),
	})
	execution.FinishedAt = &finishedAt
	execution.UpdatedAt = finishedAt
	if err := s.repository.UpdateWorkflowExecution(ctx, execution); err != nil {
		return marketing.WorkflowExecution{}, err
	}
	return execution, nil
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
		"code":          codeWorkflowNodeHandler,
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
	for index, edge := range definition.Edges {
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
		edgesBySource[edge.Source] = append(edgesBySource[edge.Source], edge)
		incoming[edge.Target] = append(incoming[edge.Target], edge)
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
			workflowCompileIssue("WORKFLOW_DAG_ENTRY_MISSING", "DAG compile failed: Event In entry node is required", "nodes", "", ""),
		}}
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
	case "condition", "qualify", "switch":
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

func (s *Service) workflowExecutionWasCanceled(ctx context.Context, executionID string) (bool, error) {
	executions, err := s.repository.ListWorkflowExecutions(ctx, marketing.WorkflowExecutionQuery{ExecutionID: executionID, Limit: 1})
	if err != nil {
		return false, err
	}
	if len(executions) == 0 {
		return false, marketing.ErrNotFound
	}
	return executions[0].Status == marketing.WorkflowExecutionCanceled, nil
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
			return input, fmt.Errorf("input %q path %q cannot be resolved: %w", inputKey, ref.Path, err)
		}
		resolved[inputKey] = extracted
	}
	input["inputMapping"] = mapping
	input["resolvedInputs"] = resolved
	return input, nil
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
	if !workflowStringInList(runtime.Inbox.Event.EventType, workflowConfigStringList(config, "eventTypes")) {
		return nodeFailed("EVENT_IN_TYPE_NOT_ALLOWED: event type is not accepted by node config", map[string]any{
			"errorCode": "EVENT_IN_TYPE_NOT_ALLOWED",
			"eventType": runtime.Inbox.Event.EventType,
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	if !workflowStringInList(runtime.Inbox.Event.SourceSystem, workflowConfigStringList(config, "sourceSystems")) {
		return nodeFailed("EVENT_IN_SOURCE_NOT_ALLOWED: source system is not accepted by node config", map[string]any{
			"errorCode":    "EVENT_IN_SOURCE_NOT_ALLOWED",
			"sourceSystem": runtime.Inbox.Event.SourceSystem,
			"audit":        workflowHandlerAudit(node, runtime),
		})
	}
	rawEvent := workflowJSONValue(runtime.Inbox.Event)
	return nodeSuccess(map[string]any{
		"raw_event": rawEvent,
		"eventId":   runtime.Inbox.Event.EventID,
		"eventType": runtime.Inbox.Event.EventType,
		"tenantId":  runtime.Inbox.Event.TenantID,
		"audit":     workflowHandlerAudit(node, runtime),
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
	return nodeSuccess(map[string]any{
		"event_context": map[string]any{
			"inboxId":        runtime.Inbox.ID,
			"status":         runtime.Inbox.ProcessStatus,
			"idempotencyKey": runtime.Inbox.IdempotencyKey,
			"event":          rawEvent,
		},
		"inboxId":        runtime.Inbox.ID,
		"status":         runtime.Inbox.ProcessStatus,
		"idempotencyKey": runtime.Inbox.IdempotencyKey,
		"audit":          workflowHandlerAudit(node, runtime),
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
	if workflowConfigBool(config, "requireActiveActivity", true) && runtime.Activity.Status != marketing.ActivityActive {
		return nodeFailed("MATCH_ACTIVITY_NOT_ACTIVE: activity is not active", map[string]any{
			"errorCode":  "MATCH_ACTIVITY_NOT_ACTIVE",
			"activityId": runtime.Activity.ID,
			"audit":      workflowHandlerAudit(node, runtime),
		})
	}
	matched := runtime.Activity.ID != "" && runtime.Activity.TenantID == runtime.Inbox.Event.TenantID
	if !matched {
		return nodeFailed("MATCH_ACTIVITY_MISMATCH: activity tenant does not match event tenant", map[string]any{
			"errorCode":  "MATCH_ACTIVITY_MISMATCH",
			"activityId": runtime.Activity.ID,
			"eventId":    runtime.Inbox.Event.EventID,
			"audit":      workflowHandlerAudit(node, runtime),
		})
	}
	matchedActivity := map[string]any{
		"activityId":   runtime.Activity.ID,
		"activityCode": runtime.Activity.Code,
		"name":         runtime.Activity.Name,
		"status":       runtime.Activity.Status,
		"tenantId":     runtime.Activity.TenantID,
		"matched":      true,
	}
	return nodeSuccess(map[string]any{
		"matched_activity": matchedActivity,
		"activityId":       runtime.Activity.ID,
		"activityCode":     runtime.Activity.Code,
		"matched":          true,
		"audit":            workflowHandlerAudit(node, runtime),
	})
}

func factQueryWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	config, failed, ok := workflowRequireNodeConfig(node)
	if !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	refs := workflowConfigStringList(config, "factKeys")
	if len(refs) == 0 {
		for factCode, fields := range marketing.ExtractFactReferences(runtime.Activity.Rules) {
			if len(fields) == 0 {
				refs = append(refs, factCode)
				continue
			}
			for field := range fields {
				refs = append(refs, factCode+"."+field)
			}
		}
	}
	if len(refs) == 0 {
		return nodeSkipped("FACT_QUERY_NO_KEYS: no fact references", map[string]any{
			"errorCode": "FACT_QUERY_NO_KEYS",
			"factCount": 0,
			"facts":     map[string]any{},
			"audit":     workflowHandlerAudit(node, runtime),
		})
	}
	facts := map[string]any{}
	for _, ref := range refs {
		facts[ref] = map[string]any{
			"key":       ref,
			"available": false,
			"source":    "runtime-placeholder",
		}
	}
	return nodeSuccess(map[string]any{
		"factCount": len(refs),
		"factKeys":  refs,
		"facts":     facts,
		"audit":     workflowHandlerAudit(node, runtime),
	})
}

func decisionWorkflowNodeHandler(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if canceled := workflowHandlerContextCanceled(ctx, node, runtime); canceled.Status != "" {
		return canceled
	}
	if _, failed, ok := workflowRequireNodeConfig(node); !ok {
		failed.Output["audit"] = workflowHandlerAudit(node, runtime)
		return failed
	}
	decision := normalizeWorkflowDecisionValue(string(runtime.RuleResult.Decision))
	if decision == "" {
		if runtime.Qualified {
			decision = "PASS"
		} else {
			decision = "BLOCK"
		}
	}
	output := map[string]any{
		"nodeType":   nodeHandlerKey(node),
		"decision":   decision,
		"qualified":  runtime.Qualified,
		"reasonCode": runtime.RuleResult.ReasonCode,
		"reason":     runtime.RuleResult.Reason,
		"audit":      workflowHandlerAudit(node, runtime),
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "condition":
		output["IF"] = runtime.Qualified
		output["ELSE"] = !runtime.Qualified
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
		"PASS":         output["PASS"],
		"BLOCK":        output["BLOCK"],
		"RISK":         output["RISK"],
		"IF":           output["IF"],
		"ELSE":         output["ELSE"],
		"route":        output["route"],
		"audit":        workflowHandlerAudit(node, runtime),
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
		result["dryRunResult"] = true
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
	for _, reward := range runtime.Activity.Rewards {
		items = append(items, map[string]any{
			"rewardId":      reward.ID,
			"rewardCode":    reward.Code,
			"rewardType":    reward.Type,
			"calculateType": reward.CalculateType,
			"grantMode":     reward.GrantMode,
			"grantChannel":  reward.GrantChannel,
		})
	}
	return nodeSuccess(map[string]any{
		"reward_items":          items,
		"configuredRewardCount": len(runtime.Activity.Rewards),
		"grantOrderCount":       len(runtime.GrantOrders),
		"audit":                 workflowHandlerAudit(node, runtime),
	})
}

func decimalWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if !runtime.Qualified {
		return nodeSkipped("activity not qualified", nil)
	}
	return nodeSuccess(map[string]any{"mode": "Decimal(18, 8)"})
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
		"scope":    firstWorkflowString(workflowConfigString(config, "scope"), "USER"),
		"period":   firstWorkflowString(workflowConfigString(config, "period"), "ACTIVITY"),
		"maxCount": maxCount,
		"subject":  runtime.Inbox.Event.Subject.ID,
		"allowed":  runtime.Qualified,
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
		"status":        "reserved-or-unlimited",
		"quotaCode":     workflowConfigString(config, "quotaCode"),
		"consumeMode":   firstWorkflowString(workflowConfigString(config, "consumeMode"), "ATOMIC"),
		"grantOrders":   len(runtime.GrantOrders),
		"activityId":    runtime.Activity.ID,
		"participation": runtime.Participation.ID,
	}
	return nodeSuccess(map[string]any{
		"quota_hold": hold,
		"status":     hold["status"],
		"audit":      workflowHandlerAudit(node, runtime),
	})
}

func participationWorkflowNodeHandler(_ context.Context, _ marketing.WorkflowNode, runtime workflowRuntime) workflowNodeResult {
	if runtime.Participation.ID == "" {
		return nodeFailed("participation was not created", nil)
	}
	return nodeSuccess(map[string]any{"participationId": runtime.Participation.ID, "status": runtime.Participation.Status})
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
		"awaitingCallback": true,
		"grantOrderCount":  len(runtime.GrantOrders),
		"provider":         workflowConfigString(config, "provider"),
		"requireSignature": config["requireSignature"],
	}
	return nodeSuccess(map[string]any{
		"callback_result":  result,
		"awaitingCallback": true,
		"grantOrderCount":  len(runtime.GrantOrders),
		"audit":            workflowHandlerAudit(node, runtime),
	})
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
