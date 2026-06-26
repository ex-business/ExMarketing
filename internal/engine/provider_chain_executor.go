package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type ProviderChainExecutor struct {
	service          *Service
	resourceOperator ServiceResourceOperator
}

type providerChainServiceFactRequest struct {
	Node      marketing.WorkflowNode
	Runtime   workflowRuntime
	Config    map[string]any
	Step      workflowFactQueryProviderStep
	Remaining []string
	Request   any
	Facts     map[string]any
	Responses []any
	LuaCall   bool
}

type providerChainServiceFactResult struct {
	Facts     map[string]any
	Responses []any
	Traces    []any
	Result    workflowNodeResult
	Done      bool
}

func NewProviderChainExecutor(service *Service, resourceOperator ServiceResourceOperator) *ProviderChainExecutor {
	if resourceOperator == nil {
		resourceOperator = newWorkflowResourceOperator(service)
	}
	return &ProviderChainExecutor{service: service, resourceOperator: resourceOperator}
}

func (e *ProviderChainExecutor) ExecuteServiceInterfaceFactQuery(ctx context.Context, req providerChainServiceFactRequest) providerChainServiceFactResult {
	facts := req.Facts
	if facts == nil {
		facts = map[string]any{}
	}
	responses := append([]any(nil), req.Responses...)
	traces := []any{}
	step := req.Step
	stepConfig := workflowFactQueryProviderStepConfig(req.Config, step, req.Remaining)
	serviceRef := firstWorkflowString(step.ServiceRef, workflowConfigString(stepConfig, "serviceRef"))
	interfaceRef := firstWorkflowString(step.InterfaceRef, workflowConfigString(stepConfig, "interfaceRef"))
	providerRef := firstWorkflowString(step.ProviderRef, workflowConfigString(stepConfig, "providerRef"))
	providerPolicy := firstWorkflowString(firstWorkflowString(step.ProviderPolicy, workflowConfigString(stepConfig, "providerPolicy")), "PRIMARY")
	environment := firstWorkflowString(step.Environment, firstWorkflowString(workflowConfigString(stepConfig, "environment"), workflowConfigString(stepConfig, "runtimeEnvironment")))
	requestPayload := req.Request
	if requestPayload == nil {
		requestPayload = workflowServiceInterfaceInputPayload(req.Remaining)
	}
	startedAt := e.service.now()

	accessReq := serviceInterfaceAccessRequest{
		TenantID:        req.Runtime.Activity.TenantID,
		WorkspaceID:     req.Runtime.Activity.WorkspaceID,
		FolderID:        req.Runtime.Activity.FolderID,
		ActivityID:      req.Runtime.Activity.ID,
		NodeID:          req.Node.ID,
		NodeType:        req.Node.Type,
		ServiceRef:      serviceRef,
		InterfaceRef:    interfaceRef,
		Operation:       marketing.ServiceOperationCall,
		ProviderPolicy:  providerPolicy,
		ProviderRef:     providerRef,
		Environment:     environment,
		LuaCall:         req.LuaCall,
		RequireApproval: activityApprovalValid(req.Runtime.Activity),
	}
	accessDecision, err := e.service.checkServiceInterfaceAccess(ctx, accessReq)
	if err != nil {
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_INTERFACE_ACCESS_CHECK_FAILED", err.Error(), facts, responses)
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditDenied, "SERVICE_INTERFACE_ACCESS_CHECK_FAILED", err.Error(), startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}
	accessTrace := serviceInterfaceAccessTrace(accessDecision, accessReq)
	if !accessDecision.Allowed {
		responses = append(responses, map[string]any{"serviceAccess": accessTrace})
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", accessDecision.Code, accessDecision.Message, facts, responses)
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditDenied, accessDecision.Code, accessDecision.Message, startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}
	resolveReq := workflowServiceProviderResolveRequest(req.Runtime, req.Node, serviceRef, interfaceRef, providerRef, providerPolicy, environment)
	serviceInterface, providers, err := e.service.resolveServiceProviders(ctx, resolveReq)
	if err != nil {
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), facts, responses)
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditFailed, "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}
	validator := serviceSchemaValidator{service: e.service}
	if err := validator.validateInterfaceInput(ctx, req.Runtime.Activity.TenantID, serviceInterface, requestPayload); err != nil {
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_INPUT_INVALID", err.Error(), facts, responses)
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_INPUT_INVALID", err.Error(), startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}

	var lastFailed workflowNodeResult
	for providerIndex, provider := range providers {
		cacheAsideCacheProvider := strings.EqualFold(strings.TrimSpace(providerPolicy), "CACHE_ASIDE") && serviceProviderCacheEnabled(provider)
		providerStartedAt := time.Now().UTC()
		if _, err := e.service.consumeServiceProviderRuntime(ctx, provider); err != nil {
			traceStep := step
			traceStep.ProviderRef = provider.Code
			traceOutput := map[string]any{"serviceAccess": accessTrace, "errorCode": err.Error()}
			traces = append(traces, workflowFactQueryProviderStepTrace(traceStep, "SKIPPED_NEXT", req.Remaining, traceOutput))
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, "", marketing.ServiceCallAuditFailed, serviceCallErrorCode(err, "SERVICE_PROVIDER_RUNTIME_DENIED"), err.Error(), providerStartedAt, requestPayload, traceOutput)
			if providerIndex == len(providers)-1 {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), facts, responses)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			continue
		}
		resolvedConfig, resolvedStep, err := workflowServiceInterfaceProviderStepConfig(step, stepConfig, serviceRef, interfaceRef, providerRef, providerPolicy, provider)
		if err != nil {
			resolvedStep.ProviderRef = provider.Code
			traceOutput := map[string]any{"errorCode": err.Error()}
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, "", marketing.ServiceCallAuditFailed, serviceCallErrorCode(err, "SERVICE_PROVIDER_RESOURCE_REF_REQUIRED"), err.Error(), providerStartedAt, requestPayload, traceOutput)
			if providerIndex == len(providers)-1 {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), facts, responses)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "FAILED_NEXT", req.Remaining, traceOutput))
			continue
		}
		stepNode := workflowFactQueryProviderStepNode(req.Node, resolvedConfig)
		resourceResult := e.resourceOperator.ExecuteFactQuery(ctx, stepNode, req.Runtime, resolvedConfig)
		responses = append(responses, resourceResult.Output)
		resourceRef := workflowConfigString(resolvedConfig, "resourceRef")
		if resourceResult.Status == marketing.WorkflowNodeFailed {
			if err := e.service.recordServiceProviderFailure(ctx, provider, resourceResult.Error); err != nil {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), facts, responses)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			if schemaErr := validator.validateInterfaceError(ctx, req.Runtime.Activity.TenantID, serviceInterface, resourceResult.Output); schemaErr != nil {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_ERROR_INVALID", schemaErr.Error(), facts, responses)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_ERROR_INVALID", schemaErr.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, workflowConfigString(resourceResult.Output, "errorCode"), resourceResult.Error, providerStartedAt, requestPayload, resourceResult.Output)
			lastFailed = resourceResult
			if cacheAsideCacheProvider {
				if !serviceProviderCacheErrorFallback(provider) {
					return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: resourceResult, Done: true}
				}
				if providerIndex < len(providers)-1 {
					traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output, "cacheAside": map[string]any{"result": "ERROR_FALLBACK"}}
					traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "CACHE_ERROR_FALLBACK", req.Remaining, traceOutput))
					continue
				}
			}
			if providerIndex == len(providers)-1 {
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: resourceResult, Done: true}
			}
			traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output}
			traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "FAILED_NEXT", req.Remaining, traceOutput))
			continue
		}
		if cacheAsideCacheProvider && serviceProviderCacheMiss(provider, resourceResult.Output) {
			if err := e.service.recordServiceProviderSuccess(ctx, provider); err != nil {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), facts, responses)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output, "cacheAside": map[string]any{"result": "MISS"}}
			traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "CACHE_MISS", req.Remaining, traceOutput))
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditSuccess, "", "", providerStartedAt, requestPayload, resourceResult.Output)
			if providerIndex < len(providers)-1 {
				continue
			}
			if workflowFactQueryProviderShouldBlockOnMiss(step) {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "FACT_QUERY_PROVIDER_MISS", "cache-aside provider did not resolve requested facts", facts, responses)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			lastFailed = workflowNodeResult{}
			break
		}
		if err := validator.validateInterfaceOutput(ctx, req.Runtime.Activity.TenantID, serviceInterface, resourceResult.Output); err != nil {
			result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_OUTPUT_INVALID", err.Error(), facts, responses)
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_OUTPUT_INVALID", err.Error(), providerStartedAt, requestPayload, result.Output)
			return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
		}
		if err := e.service.recordServiceProviderSuccess(ctx, provider); err != nil {
			result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), facts, responses)
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), providerStartedAt, requestPayload, result.Output)
			return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
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
		traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output}
		if weightedTrace := serviceProviderWeightedSelectionTrace(resolveReq, providers, provider); len(weightedTrace) > 0 {
			traceOutput["weightedSelection"] = weightedTrace
		}
		traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, status, req.Remaining, traceOutput))
		e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditSuccess, "", "", providerStartedAt, requestPayload, resourceResult.Output)
		if status == "MISS" && workflowFactQueryProviderShouldBlockOnMiss(step) {
			if providerIndex == len(providers)-1 {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "FACT_QUERY_PROVIDER_MISS", "providerChain step did not resolve requested facts", facts, responses)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			continue
		}
		lastFailed = workflowNodeResult{}
		break
	}
	if lastFailed.Status == marketing.WorkflowNodeFailed {
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: lastFailed, Done: true}
	}
	return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces}
}

func (e *ProviderChainExecutor) audit(ctx context.Context, req providerChainServiceFactRequest, serviceRef string, interfaceRef string, providerRef string, resourceRef string, status marketing.ServiceCallAuditStatus, errorCode string, errorMessage string, startedAt time.Time, request any, response any) {
	e.service.recordServiceCallAudit(ctx, serviceCallAuditInput{
		Runtime:      req.Runtime,
		Node:         req.Node,
		ServiceRef:   serviceRef,
		InterfaceRef: interfaceRef,
		ProviderRef:  providerRef,
		ResourceRef:  resourceRef,
		StepID:       req.Node.ID,
		Attempt:      1,
		Status:       status,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		StartedAt:    startedAt,
		Request:      request,
		Response:     response,
		ProviderTrace: map[string]any{
			"providerRef":          providerRef,
			"requestedProviderRef": req.Step.ProviderRef,
			"providerRefOverride":  strings.TrimSpace(req.Step.ProviderRef) != "",
			"providerPolicy":       req.Step.ProviderPolicy,
			"environment":          req.Step.Environment,
		},
		PermissionTrace: map[string]any{
			"luaCall": req.LuaCall,
		},
	})
}

func serviceProviderCacheErrorFallback(provider marketing.ServiceProvider) bool {
	config := serviceProviderConfigMap(provider)
	policy, _ := config["cacheErrorPolicy"].(string)
	switch strings.ToUpper(strings.TrimSpace(policy)) {
	case "", "FALLBACK":
		return true
	case "FAIL":
		return false
	default:
		return true
	}
}

func serviceProviderCacheMiss(provider marketing.ServiceProvider, output map[string]any) bool {
	config := serviceProviderConfigMap(provider)
	if path, _ := config["missPath"].(string); strings.TrimSpace(path) != "" {
		value, err := extractWorkflowJSONPath(output, path)
		if err != nil {
			return true
		}
		missValues := workflowConfigStringList(config, "missValues")
		if len(missValues) == 0 {
			return workflowValueEmpty(value)
		}
		valueString := strings.TrimSpace(fmt.Sprint(value))
		for _, missValue := range missValues {
			if strings.EqualFold(valueString, strings.TrimSpace(missValue)) {
				return true
			}
		}
		return false
	}
	return !workflowFactQueryProviderStepHasFacts(output) || workflowValueEmpty(output["facts"])
}

func workflowValueEmpty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func serviceCallErrorCode(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return fallback
	}
	if idx := strings.Index(message, ":"); idx > 0 {
		message = message[:idx]
	}
	message = strings.TrimSpace(message)
	if message == "" || errors.Is(err, marketing.ErrNotFound) {
		return fallback
	}
	return message
}

func serviceProviderWeightedSelectionTrace(req serviceProviderResolveRequest, providers []marketing.ServiceProvider, selected marketing.ServiceProvider) map[string]any {
	policy := strings.ToUpper(strings.TrimSpace(req.ProviderPolicy))
	if policy != "WEIGHTED" && policy != "WEIGHTED_WITH_FALLBACK" {
		return nil
	}
	total := 0
	candidates := make([]string, 0, len(providers))
	for _, provider := range providers {
		if provider.Weight > 0 {
			total += provider.Weight
		}
		candidates = append(candidates, provider.Code)
	}
	if total <= 0 {
		return nil
	}
	seed := serviceProviderSelectionSeed(req, providers)
	bucket := serviceProviderSelectionBucket(seed, total)
	return map[string]any{
		"policy":      policy,
		"seed":        seed,
		"bucket":      bucket,
		"totalWeight": total,
		"selected":    selected.Code,
		"candidates":  candidates,
	}
}
