package engine

import (
	"context"
	"encoding/json"
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

type cacheAsideWriteBackTarget struct {
	Provider    marketing.ServiceProvider
	Config      map[string]any
	Step        workflowFactQueryProviderStep
	ResourceRef string
}

type cacheAsideWriteBackResult struct {
	Summary map[string]any
	Warning string
	Err     error
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
	resolveReq.Binding = accessDecision.Binding
	serviceInterface, providers, err := e.service.resolveServiceProviders(ctx, resolveReq)
	if err != nil {
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), facts, responses)
		if providerRef != "" {
			code := serviceCallErrorCode(err, "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED")
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, providerRef, providerDecisionFromCode(code), code, startedAt, 0, environment, false, map[string]any{"error": err.Error()})
		} else {
			e.auditUnavailableProviderCandidates(ctx, req, resolveReq, accessDecision.Binding, startedAt)
		}
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditFailed, "SERVICE_INTERFACE_PROVIDER_NOT_RESOLVED", err.Error(), startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}
	validator := serviceSchemaValidator{service: e.service}
	if err := validator.validateInterfaceInput(ctx, req.Runtime.Activity.TenantID, serviceInterface, requestPayload); err != nil {
		result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_INPUT_INVALID", err.Error(), facts, responses)
		e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, providerRef, marketing.ProviderDecisionSchemaFailed, "SERVICE_SCHEMA_INPUT_INVALID", startedAt, 0, environment, false, map[string]any{"error": err.Error()})
		e.audit(ctx, req, serviceRef, interfaceRef, providerRef, "", marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_INPUT_INVALID", err.Error(), startedAt, requestPayload, result.Output)
		return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
	}

	var lastFailed workflowNodeResult
	var pendingCacheWriteBack *cacheAsideWriteBackTarget
	rateLimitSubject := workflowFactQuerySubject(ctx, req.Config, req.Runtime)
	for providerIndex, provider := range providers {
		cacheAsideCacheProvider := strings.EqualFold(strings.TrimSpace(providerPolicy), "CACHE_ASIDE") && serviceProviderCacheEnabled(provider)
		providerStartedAt := time.Now().UTC()
		if _, err := e.service.consumeServiceProviderRuntime(ctx, provider, serviceProviderRuntimeConsumeOption{Subject: rateLimitSubject, Binding: accessDecision.Binding}); err != nil {
			traceStep := step
			traceStep.ProviderRef = provider.Code
			traceOutput := map[string]any{"serviceAccess": accessTrace, "errorCode": err.Error()}
			traces = append(traces, workflowFactQueryProviderStepTrace(traceStep, "SKIPPED_NEXT", req.Remaining, traceOutput))
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, providerDecisionFromError(err), serviceCallErrorCode(err, "SERVICE_PROVIDER_RUNTIME_DENIED"), providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
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
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, serviceCallErrorCode(err, "SERVICE_PROVIDER_RESOURCE_REF_REQUIRED"), providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
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
				e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), result.Output)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			if schemaErr := validator.validateInterfaceError(ctx, req.Runtime.Activity.TenantID, serviceInterface, resourceResult.Output); schemaErr != nil {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_ERROR_INVALID", schemaErr.Error(), facts, responses)
				e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSchemaFailed, "SERVICE_SCHEMA_ERROR_INVALID", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), result.Output)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_ERROR_INVALID", schemaErr.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, workflowConfigString(resourceResult.Output, "errorCode"), resourceResult.Error, providerStartedAt, requestPayload, resourceResult.Output)
			lastFailed = resourceResult
			if cacheAsideCacheProvider {
				pendingCacheWriteBack = &cacheAsideWriteBackTarget{Provider: provider, Config: resolvedConfig, Step: resolvedStep, ResourceRef: resourceRef}
				if !serviceProviderCacheErrorFallback(provider) {
					return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: resourceResult, Done: true}
				}
				if providerIndex < len(providers)-1 {
					traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output, "cacheAside": map[string]any{"result": "ERROR_FALLBACK"}}
					traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "CACHE_ERROR_FALLBACK", req.Remaining, traceOutput))
					e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionFallback, firstWorkflowString(workflowConfigString(resourceResult.Output, "errorCode"), "CACHE_ERROR_FALLBACK"), providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
					continue
				}
			}
			if providerIndex == len(providers)-1 {
				e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, firstWorkflowString(workflowConfigString(resourceResult.Output, "errorCode"), resourceResult.Error), providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), resourceResult.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: resourceResult, Done: true}
			}
			traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output}
			traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "FAILED_NEXT", req.Remaining, traceOutput))
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionFallback, firstWorkflowString(workflowConfigString(resourceResult.Output, "errorCode"), resourceResult.Error), providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
			continue
		}
		if cacheAsideCacheProvider && serviceProviderCacheMiss(provider, resourceResult.Output) {
			pendingCacheWriteBack = &cacheAsideWriteBackTarget{Provider: provider, Config: resolvedConfig, Step: resolvedStep, ResourceRef: resourceRef}
			if err := e.service.recordServiceProviderSuccess(ctx, provider); err != nil {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), facts, responses)
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			traceOutput := map[string]any{"serviceAccess": accessTrace, "providerOutput": resourceResult.Output, "cacheAside": map[string]any{"result": "MISS"}}
			traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, "CACHE_MISS", req.Remaining, traceOutput))
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditSuccess, "", "", providerStartedAt, requestPayload, resourceResult.Output)
			if providerIndex < len(providers)-1 {
				e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, "CACHE_MISS", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
				continue
			}
			if workflowFactQueryProviderShouldBlockOnMiss(step) {
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "FACT_QUERY_PROVIDER_MISS", "cache-aside provider did not resolve requested facts", facts, responses)
				e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, "FACT_QUERY_PROVIDER_MISS", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			lastFailed = workflowNodeResult{}
			break
		}
		if err := validator.validateInterfaceOutput(ctx, req.Runtime.Activity.TenantID, serviceInterface, resourceResult.Output); err != nil {
			result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_SCHEMA_OUTPUT_INVALID", err.Error(), facts, responses)
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSchemaFailed, "SERVICE_SCHEMA_OUTPUT_INVALID", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), result.Output)
			e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_SCHEMA_OUTPUT_INVALID", err.Error(), providerStartedAt, requestPayload, result.Output)
			return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
		}
		var writeBackTrace map[string]any
		if pendingCacheWriteBack != nil && strings.EqualFold(strings.TrimSpace(providerPolicy), "CACHE_ASIDE") && !cacheAsideCacheProvider {
			writeBack := e.cacheAsideWriteBack(ctx, req, *pendingCacheWriteBack, resourceResult.Output)
			if writeBack.Summary != nil {
				writeBackTrace = writeBack.Summary
				responses = append(responses, map[string]any{"cacheAsideWriteBack": writeBack.Summary})
			}
			writeBackStatus := marketing.ServiceCallAuditSuccess
			writeBackCode := ""
			writeBackMessage := ""
			if writeBack.Err != nil {
				writeBackStatus = marketing.ServiceCallAuditFailed
				writeBackCode = "SERVICE_CACHE_WRITE_BACK_FAILED"
				writeBackMessage = writeBack.Err.Error()
			}
			writeBackResourceRef := workflowConfigString(writeBack.Summary, "resourceRef")
			e.audit(ctx, req, serviceRef, interfaceRef, pendingCacheWriteBack.Provider.Code, writeBackResourceRef, writeBackStatus, writeBackCode, writeBackMessage, time.Now().UTC(), resourceResult.Output, writeBack.Summary)
			if writeBack.Err != nil && serviceProviderCacheWriteBackRequired(pendingCacheWriteBack.Provider) {
				resultOutput := map[string]any{
					"errorCode":           "SERVICE_CACHE_WRITE_BACK_FAILED",
					"cacheAsideWriteBack": writeBack.Summary,
					"providerOutput":      resourceResult.Output,
				}
				result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_CACHE_WRITE_BACK_FAILED", writeBack.Err.Error(), facts, append(responses, resultOutput))
				e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditFailed, "SERVICE_CACHE_WRITE_BACK_FAILED", writeBack.Err.Error(), providerStartedAt, requestPayload, result.Output)
				return providerChainServiceFactResult{Facts: facts, Responses: responses, Traces: traces, Result: result, Done: true}
			}
			pendingCacheWriteBack = nil
		}
		if err := e.service.recordServiceProviderSuccess(ctx, provider); err != nil {
			result := workflowFactQueryFailure(req.Node, req.Runtime, req.Config, "", "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", err.Error(), facts, responses)
			e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSkipped, "SERVICE_PROVIDER_RUNTIME_STATE_FAILED", providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), result.Output)
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
		if writeBackTrace != nil {
			traceOutput["cacheAsideWriteBack"] = writeBackTrace
		}
		if weightedTrace := serviceProviderWeightedSelectionTrace(resolveReq, providers, provider); len(weightedTrace) > 0 {
			traceOutput["weightedSelection"] = weightedTrace
		}
		traces = append(traces, workflowFactQueryProviderStepTrace(resolvedStep, status, req.Remaining, traceOutput))
		e.audit(ctx, req, serviceRef, interfaceRef, provider.Code, resourceRef, marketing.ServiceCallAuditSuccess, "", "", providerStartedAt, requestPayload, resourceResult.Output)
		e.decision(ctx, req, serviceRef, interfaceRef, providerPolicy, provider.Code, marketing.ProviderDecisionSelected, status, providerStartedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), traceOutput)
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

func (e *ProviderChainExecutor) decision(ctx context.Context, req providerChainServiceFactRequest, serviceRef string, interfaceRef string, providerPolicy string, providerRef string, decision marketing.ProviderDecision, reason string, startedAt time.Time, weight int, environment string, mock bool, details any) {
	e.service.recordProviderDecisionAudit(ctx, providerDecisionAuditInput{
		Runtime:              req.Runtime,
		Node:                 req.Node,
		ServiceRef:           serviceRef,
		InterfaceRef:         interfaceRef,
		ProviderPolicy:       providerPolicy,
		CandidateProviderRef: providerRef,
		Decision:             decision,
		Reason:               reason,
		StartedAt:            startedAt,
		Weight:               weight,
		Environment:          environment,
		Mock:                 mock,
		Details:              details,
	})
}

func (e *ProviderChainExecutor) auditUnavailableProviderCandidates(ctx context.Context, req providerChainServiceFactRequest, resolveReq serviceProviderResolveRequest, binding marketing.ServiceInterfaceBinding, startedAt time.Time) {
	policy := strings.ToUpper(strings.TrimSpace(resolveReq.ProviderPolicy))
	if policy == "" {
		policy = "PRIMARY"
	}
	if !serviceProviderPolicySupported(policy) {
		return
	}
	providers, err := e.service.ListServiceProviders(ctx, marketing.ServiceProviderQuery{
		TenantID:     resolveReq.TenantID,
		ServiceRef:   resolveReq.ServiceRef,
		InterfaceRef: resolveReq.InterfaceRef,
		Status:       marketing.ServiceAssetActive,
		Limit:        50,
	})
	if err != nil {
		return
	}
	providers = filterServiceProviderCandidates(providers, policy)
	if policy == "CACHE_ASIDE" {
		providers = filterServiceProviderCacheAsideEnvironmentCandidates(providers, resolveReq.Environment)
	} else {
		providers = filterServiceProviderEnvironmentCandidates(providers, resolveReq.Environment)
	}
	for _, provider := range providers {
		available, code, message, err := e.service.serviceProviderRuntimeAvailable(ctx, provider, serviceProviderGovernanceConfigOption{Binding: binding})
		if err != nil || available {
			continue
		}
		reason := firstWorkflowString(code, "SERVICE_PROVIDER_RUNTIME_DENIED")
		if message != "" {
			reason = reason + ":" + message
		}
		e.decision(ctx, req, resolveReq.ServiceRef, resolveReq.InterfaceRef, policy, provider.Code, providerDecisionFromCode(code), reason, startedAt, provider.Weight, provider.Environment, serviceProviderMockEnabled(provider), map[string]any{
			"runtimeGate": map[string]any{
				"code":    code,
				"message": message,
			},
		})
	}
}

func providerDecisionFromError(err error) marketing.ProviderDecision {
	code := serviceCallErrorCode(err, "")
	return providerDecisionFromCode(code)
}

func providerDecisionFromCode(code string) marketing.ProviderDecision {
	switch code {
	case "SERVICE_PROVIDER_RATE_LIMITED":
		return marketing.ProviderDecisionRateLimited
	case "SERVICE_PROVIDER_UNHEALTHY":
		return marketing.ProviderDecisionUnhealthy
	case "SERVICE_PROVIDER_CIRCUIT_OPEN", "SERVICE_PROVIDER_HALF_OPEN_PROBE_LIMITED":
		return marketing.ProviderDecisionCircuitOpen
	default:
		return marketing.ProviderDecisionSkipped
	}
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

func (e *ProviderChainExecutor) cacheAsideWriteBack(ctx context.Context, req providerChainServiceFactRequest, target cacheAsideWriteBackTarget, providerOutput map[string]any) cacheAsideWriteBackResult {
	resourceRef := serviceProviderCacheWriteBackResourceRef(target.Provider, target.Config, target.ResourceRef)
	summary := map[string]any{
		"providerRef": target.Provider.Code,
		"resourceRef": resourceRef,
		"operation":   serviceProviderCacheWriteBackOperation(target.Provider),
	}
	if !serviceProviderCacheWriteBackEnabled(target.Provider) {
		summary["status"] = "SKIPPED"
		summary["reason"] = "DISABLED"
		return cacheAsideWriteBackResult{Summary: summary}
	}
	if strings.TrimSpace(resourceRef) == "" {
		err := errors.New("SERVICE_CACHE_WRITE_BACK_RESOURCE_REF_REQUIRED")
		summary["status"] = "FAILED"
		summary["warning"] = err.Error()
		return cacheAsideWriteBackResult{Summary: summary, Warning: err.Error(), Err: err}
	}
	facts := serviceProviderCacheWriteBackFacts(req.Remaining, providerOutput)
	if len(facts) == 0 {
		err := errors.New("SERVICE_CACHE_WRITE_BACK_PAYLOAD_EMPTY")
		summary["status"] = "FAILED"
		summary["warning"] = err.Error()
		return cacheAsideWriteBackResult{Summary: summary, Warning: err.Error(), Err: err}
	}
	summary["factKeys"] = facts
	written := 0
	var lastOutput map[string]any
	for _, factCode := range facts {
		payload, err := serviceProviderCacheWriteBackPayload(target.Provider, providerOutput, factCode)
		if err != nil {
			summary["status"] = "FAILED"
			summary["warning"] = err.Error()
			summary["factCode"] = factCode
			return cacheAsideWriteBackResult{Summary: summary, Warning: err.Error(), Err: err}
		}
		if err := serviceProviderValidateCacheWriteBackPayload(target.Provider, payload); err != nil {
			summary["status"] = "FAILED"
			summary["warning"] = err.Error()
			summary["factCode"] = factCode
			return cacheAsideWriteBackResult{Summary: summary, Warning: err.Error(), Err: err}
		}
		keyTemplate := firstWorkflowString(serviceProviderCacheWriteBackConfigString(target.Provider, "writeBackKeyTemplate", "cacheWriteBackKeyTemplate"), workflowConfigString(target.Config, "keyTemplate"))
		if keyTemplate == "" {
			keyTemplate = "${tenantId}:${subject.id}:${factCode}"
		}
		key := workflowRenderFactQueryTemplate(keyTemplate, req.Runtime, req.Node, factCode)
		output, err := e.resourceOperator.ExecuteResourceAPI(ctx, workflowFactQueryProviderStepNode(req.Node, target.Config), req.Runtime, workflowResourceAPIRequest{
			Operation:   serviceProviderCacheWriteBackOperation(target.Provider),
			ResourceRef: resourceRef,
			Namespace:   firstWorkflowString(serviceProviderCacheWriteBackConfigString(target.Provider, "writeBackNamespace", "cacheWriteBackNamespace"), firstWorkflowString(workflowConfigString(target.Config, "namespace"), "facts")),
			Key:         key,
			Value:       payload,
			ValueType:   firstWorkflowString(serviceProviderCacheWriteBackConfigString(target.Provider, "writeBackValueType", "cacheWriteBackValueType"), firstWorkflowString(workflowConfigString(target.Config, "valueType"), "OBJECT")),
			Options: map[string]any{
				"factCode":       factCode,
				"providerRef":    target.Provider.Code,
				"cacheAside":     true,
				"writeBack":      true,
				"timeoutSeconds": workflowConfigNumber(target.Config, "timeoutSeconds"),
			},
		})
		lastOutput = output
		if err != nil {
			warning := firstWorkflowString(workflowConfigString(output, "errorCode"), "SERVICE_CACHE_WRITE_BACK_FAILED")
			if strings.TrimSpace(err.Error()) != "" {
				warning = warning + ": " + err.Error()
			}
			summary["status"] = "FAILED"
			summary["warning"] = warning
			summary["factCode"] = factCode
			summary["resourceOutput"] = output
			return cacheAsideWriteBackResult{Summary: summary, Warning: warning, Err: errors.New(warning)}
		}
		written++
	}
	summary["status"] = "SUCCESS"
	summary["written"] = written
	if lastOutput != nil {
		summary["resourceOutput"] = lastOutput
	}
	return cacheAsideWriteBackResult{Summary: summary}
}

func serviceProviderCacheWriteBackResourceRef(provider marketing.ServiceProvider, config map[string]any, fallback string) string {
	providerRef := serviceProviderCacheWriteBackConfigString(provider, "writeBackResourceRef", "cacheWriteBackResourceRef", "writeBackStoreRef")
	if providerRef != "" {
		return providerRef
	}
	stepRef := serviceProviderCacheWriteBackConfigStringFromMap(config, "writeBackResourceRef", "cacheWriteBackResourceRef", "writeBackStoreRef")
	if stepRef != "" {
		return stepRef
	}
	return strings.TrimSpace(fallback)
}

func serviceProviderCacheWriteBackEnabled(provider marketing.ServiceProvider) bool {
	config := serviceProviderConfigMap(provider)
	return workflowConfigBool(config, "writeBack", workflowConfigBool(config, "cacheWriteBack", true))
}

func serviceProviderCacheWriteBackRequired(provider marketing.ServiceProvider) bool {
	config := serviceProviderConfigMap(provider)
	return workflowConfigBool(config, "writeBackRequired", workflowConfigBool(config, "strict", workflowConfigBool(config, "cacheWriteBackRequired", false)))
}

func serviceProviderCacheWriteBackOperation(provider marketing.ServiceProvider) marketing.ResourceOperation {
	config := serviceProviderConfigMap(provider)
	operation := firstWorkflowString(serviceProviderCacheWriteBackConfigStringFromMap(config, "writeBackOperation", "cacheWriteBackOperation"), "UPSERT")
	return workflowNormalizeResourceAPIOperation(marketing.ResourceOperation(operation))
}

func serviceProviderCacheWriteBackConfigString(provider marketing.ServiceProvider, keys ...string) string {
	return serviceProviderCacheWriteBackConfigStringFromMap(serviceProviderConfigMap(provider), keys...)
}

func serviceProviderCacheWriteBackConfigStringFromMap(config map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := workflowConfigString(config, key); value != "" {
			return value
		}
	}
	return ""
}

func serviceProviderCacheWriteBackFacts(remaining []string, providerOutput map[string]any) []string {
	facts, _ := workflowJSONValue(providerOutput["facts"]).(map[string]any)
	out := make([]string, 0, len(remaining))
	for _, factCode := range remaining {
		factCode = strings.ToUpper(strings.TrimSpace(factCode))
		if factCode == "" {
			continue
		}
		if _, ok := facts[factCode]; ok {
			out = append(out, factCode)
		}
	}
	if len(out) > 0 {
		return out
	}
	for factCode := range facts {
		if strings.TrimSpace(factCode) != "" {
			out = append(out, strings.ToUpper(strings.TrimSpace(factCode)))
		}
	}
	return out
}

func serviceProviderCacheWriteBackPayload(provider marketing.ServiceProvider, providerOutput map[string]any, factCode string) (any, error) {
	config := serviceProviderConfigMap(provider)
	if path := serviceProviderCacheWriteBackConfigStringFromMap(config, "writeBackValuePath", "cacheWriteBackValuePath"); path != "" {
		value, err := extractWorkflowJSONPath(providerOutput, path)
		if err != nil {
			return nil, fmt.Errorf("SERVICE_CACHE_WRITE_BACK_VALUE_PATH_INVALID:%s:%w", path, err)
		}
		return workflowJSONValue(value), nil
	}
	facts, _ := workflowJSONValue(providerOutput["facts"]).(map[string]any)
	if value, ok := facts[strings.ToUpper(strings.TrimSpace(factCode))]; ok {
		return workflowJSONValue(value), nil
	}
	return workflowJSONValue(providerOutput), nil
}

func serviceProviderValidateCacheWriteBackPayload(provider marketing.ServiceProvider, payload any) error {
	if workflowValueEmpty(payload) {
		return errors.New("SERVICE_CACHE_WRITE_BACK_PAYLOAD_EMPTY")
	}
	encoded, err := json.Marshal(workflowJSONValue(payload))
	if err != nil {
		return fmt.Errorf("SERVICE_CACHE_WRITE_BACK_PAYLOAD_INVALID:%w", err)
	}
	maxBytes := serviceProviderConfigInt(serviceProviderConfigMap(provider), 64*1024, "writeBackMaxBytes", "cacheWriteBackMaxBytes")
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	if len(encoded) > maxBytes {
		return fmt.Errorf("SERVICE_CACHE_WRITE_BACK_PAYLOAD_TOO_LARGE:%d>%d", len(encoded), maxBytes)
	}
	return nil
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
