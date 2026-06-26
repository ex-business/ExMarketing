package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type serviceInterfaceAccessRequest struct {
	TenantID        string
	WorkspaceID     string
	FolderID        string
	ActivityID      string
	NodeID          string
	NodeType        string
	ServiceRef      string
	InterfaceRef    string
	Operation       marketing.ServiceInterfaceOperation
	ProviderPolicy  string
	ProviderRef     string
	Environment     string
	ActorID         string
	ActorRoles      []string
	ActorGroups     []string
	ServiceAccount  string
	LuaCall         bool
	RequireApproval bool
}

type serviceInterfaceAccessDecision struct {
	Allowed           bool
	MatchedBindingIDs []string
	Binding           marketing.ServiceInterfaceBinding
	Code              string
	Message           string
}

type ServiceInterfaceAccessCheck struct {
	TenantID       string                              `json:"tenantId"`
	WorkspaceID    string                              `json:"workspaceId,omitempty"`
	FolderID       string                              `json:"folderId,omitempty"`
	ActivityID     string                              `json:"activityId,omitempty"`
	NodeID         string                              `json:"nodeId,omitempty"`
	NodeType       string                              `json:"nodeType,omitempty"`
	ServiceRef     string                              `json:"serviceRef"`
	InterfaceRef   string                              `json:"interfaceRef"`
	Operation      marketing.ServiceInterfaceOperation `json:"operation,omitempty"`
	ProviderPolicy string                              `json:"providerPolicy,omitempty"`
	ProviderRef    string                              `json:"providerRef,omitempty"`
	Environment    string                              `json:"environment,omitempty"`
	LuaCall        bool                                `json:"luaCall,omitempty"`
}

type ServiceInterfaceAccessCheckResult struct {
	Allowed           bool     `json:"allowed"`
	Code              string   `json:"code,omitempty"`
	Message           string   `json:"message,omitempty"`
	MatchedBindingIDs []string `json:"matchedBindingIds,omitempty"`
}

func (s *Service) CheckServiceInterfaceAccess(ctx context.Context, check ServiceInterfaceAccessCheck) (ServiceInterfaceAccessCheckResult, error) {
	decision, err := s.checkServiceInterfaceAccess(ctx, serviceInterfaceAccessRequest{
		TenantID:       check.TenantID,
		WorkspaceID:    check.WorkspaceID,
		FolderID:       check.FolderID,
		ActivityID:     check.ActivityID,
		NodeID:         check.NodeID,
		NodeType:       check.NodeType,
		ServiceRef:     check.ServiceRef,
		InterfaceRef:   check.InterfaceRef,
		Operation:      check.Operation,
		ProviderPolicy: check.ProviderPolicy,
		ProviderRef:    check.ProviderRef,
		Environment:    check.Environment,
		LuaCall:        check.LuaCall,
	})
	if err != nil {
		return ServiceInterfaceAccessCheckResult{}, err
	}
	return ServiceInterfaceAccessCheckResult{
		Allowed:           decision.Allowed,
		Code:              decision.Code,
		Message:           decision.Message,
		MatchedBindingIDs: decision.MatchedBindingIDs,
	}, nil
}

func (s *Service) checkServiceInterfaceAccess(ctx context.Context, req serviceInterfaceAccessRequest) (serviceInterfaceAccessDecision, error) {
	req.TenantID = strings.TrimSpace(req.TenantID)
	req.ServiceRef = strings.TrimSpace(req.ServiceRef)
	req.InterfaceRef = strings.TrimSpace(req.InterfaceRef)
	req.NodeType = strings.TrimSpace(req.NodeType)
	req.NodeID = strings.TrimSpace(req.NodeID)
	req.ProviderRef = strings.TrimSpace(req.ProviderRef)
	req.ProviderPolicy = strings.ToUpper(strings.TrimSpace(req.ProviderPolicy))
	req.Environment = strings.TrimSpace(req.Environment)
	if req.ProviderPolicy == "" {
		req.ProviderPolicy = "PRIMARY"
	}
	req = serviceInterfaceAccessRequestWithActorContext(ctx, req)
	if req.Operation == "" {
		req.Operation = marketing.ServiceOperationCall
	}
	if req.TenantID == "" || req.ServiceRef == "" || req.InterfaceRef == "" {
		return denyServiceInterfaceAccess("SERVICE_INTERFACE_REF_REQUIRED", "tenantId, serviceRef and interfaceRef are required")
	}
	serviceInterface, err := s.GetServiceInterface(ctx, req.TenantID, req.ServiceRef, req.InterfaceRef)
	if err != nil {
		if errors.Is(err, marketing.ErrNotFound) {
			return denyServiceInterfaceAccess("SERVICE_INTERFACE_NOT_FOUND", fmt.Sprintf("service interface %s/%s is not found", req.ServiceRef, req.InterfaceRef))
		}
		return serviceInterfaceAccessDecision{}, err
	}
	if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
		return denyServiceInterfaceAccess("SERVICE_INTERFACE_DISABLED", fmt.Sprintf("service interface %s/%s is not ACTIVE", req.ServiceRef, req.InterfaceRef))
	}
	if serviceInterface.Operation != "" && serviceInterface.Operation != req.Operation {
		return denyServiceInterfaceAccess("SERVICE_INTERFACE_OPERATION_DENIED", fmt.Sprintf("operation %s is not allowed by interface %s/%s", req.Operation, req.ServiceRef, req.InterfaceRef))
	}
	if req.LuaCall && !serviceInterface.LuaCallable {
		return denyServiceInterfaceAccess("SERVICE_INTERFACE_LUA_FORBIDDEN", fmt.Sprintf("service interface %s/%s is not luaCallable", req.ServiceRef, req.InterfaceRef))
	}
	bindings, err := s.repository.ListServiceInterfaceBindings(ctx, marketing.ServiceInterfaceBindingQuery{
		TenantID:     req.TenantID,
		ServiceRef:   req.ServiceRef,
		InterfaceRef: req.InterfaceRef,
		Status:       marketing.ServiceAssetActive,
		Limit:        500,
	})
	if err != nil {
		return serviceInterfaceAccessDecision{}, err
	}
	if len(bindings) == 0 {
		return denyServiceInterfaceAccess("SERVICE_INTERFACE_BINDING_REQUIRED", fmt.Sprintf("service interface %s/%s requires an ACTIVE binding", req.ServiceRef, req.InterfaceRef))
	}
	denied := serviceInterfaceAccessDecision{}
	for _, binding := range bindings {
		if !serviceAssetStatusActive(binding.Status, binding.DeletedAt) {
			continue
		}
		if !serviceInterfaceBindingEnvironmentMatches(binding, req) {
			denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_ENVIRONMENT_DENIED", fmt.Sprintf("binding environment %s does not match runtime environment %s", binding.Environment, req.Environment))
			continue
		}
		if !serviceInterfaceBindingScopeMatches(binding, req) {
			if denied.Code == "" {
				denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_SCOPE_DENIED", fmt.Sprintf("binding scope does not cover node %s", req.NodeID))
			}
			continue
		}
		if !serviceInterfaceBindingPrincipalMatches(binding, req) {
			if denied.Code == "" {
				denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_ROLE_DENIED", "binding principal does not cover this workflow call")
			}
			continue
		}
		if binding.Deny {
			return denyServiceInterfaceAccess("SERVICE_INTERFACE_ACCESS_DENIED", fmt.Sprintf("service interface %s/%s is denied by binding %s", req.ServiceRef, req.InterfaceRef, binding.ID))
		}
		if len(binding.AllowedNodeTypes) > 0 && !workflowStringInList(req.NodeType, binding.AllowedNodeTypes) {
			denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_NODE_DENIED", fmt.Sprintf("node type %s is not allowed", req.NodeType))
			continue
		}
		if len(binding.AllowedOperations) > 0 && !serviceInterfaceOperationInList(req.Operation, binding.AllowedOperations) {
			denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_OPERATION_DENIED", fmt.Sprintf("operation %s is not allowed", req.Operation))
			continue
		}
		if len(binding.AllowedProviderPolicies) > 0 && !workflowStringInList(req.ProviderPolicy, binding.AllowedProviderPolicies) {
			denied = serviceInterfaceAccessDenied("SERVICE_PROVIDER_POLICY_DENIED", fmt.Sprintf("provider policy %s is not allowed", req.ProviderPolicy))
			continue
		}
		if req.ProviderRef != "" && (!binding.AllowExplicitProviderRef || !workflowStringInList(req.ProviderRef, binding.AllowedProviderRefs)) {
			denied = serviceInterfaceAccessDenied("SERVICE_PROVIDER_REF_DENIED", fmt.Sprintf("providerRef %s is not allowed by binding", req.ProviderRef))
			continue
		}
		if req.LuaCall && !binding.LuaCallable {
			denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_LUA_FORBIDDEN", "binding does not allow Lua service call")
			continue
		}
		if binding.RequireApproval && !req.RequireApproval {
			denied = serviceInterfaceAccessDenied("SERVICE_INTERFACE_APPROVAL_REQUIRED", "binding requires approval snapshot")
			continue
		}
		return serviceInterfaceAccessDecision{
			Allowed:           true,
			MatchedBindingIDs: []string{binding.ID},
			Binding:           binding,
		}, nil
	}
	if denied.Code != "" {
		return denied, nil
	}
	return denyServiceInterfaceAccess("SERVICE_INTERFACE_BINDING_REQUIRED", fmt.Sprintf("service interface %s/%s has no matching binding", req.ServiceRef, req.InterfaceRef))
}

func denyServiceInterfaceAccess(code string, message string) (serviceInterfaceAccessDecision, error) {
	return serviceInterfaceAccessDenied(code, message), nil
}

func serviceInterfaceAccessDenied(code string, message string) serviceInterfaceAccessDecision {
	return serviceInterfaceAccessDecision{
		Allowed: false,
		Code:    code,
		Message: message,
	}
}

func serviceInterfaceOperationInList(value marketing.ServiceInterfaceOperation, allowed []marketing.ServiceInterfaceOperation) bool {
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}

func serviceInterfaceBindingScopeMatches(binding marketing.ServiceInterfaceBinding, req serviceInterfaceAccessRequest) bool {
	switch binding.Scope {
	case marketing.ServiceAssetScopeSystem, marketing.ServiceAssetScopeTenant:
		return true
	case marketing.ServiceAssetScopeWorkspace:
		return binding.WorkspaceID == "" || binding.WorkspaceID == req.WorkspaceID
	case marketing.ServiceAssetScopeFolder:
		if binding.WorkspaceID != "" && binding.WorkspaceID != req.WorkspaceID {
			return false
		}
		if binding.IncludeSubfolders {
			return binding.FolderID == "" || binding.FolderID == req.FolderID
		}
		return binding.FolderID == "" || binding.FolderID == req.FolderID
	case marketing.ServiceAssetScopeActivity:
		if binding.WorkspaceID != "" && binding.WorkspaceID != req.WorkspaceID {
			return false
		}
		if binding.FolderID != "" && binding.FolderID != req.FolderID {
			return false
		}
		return binding.ActivityID == "" || binding.ActivityID == req.ActivityID
	case marketing.ServiceAssetScopeNode:
		if binding.WorkspaceID != "" && binding.WorkspaceID != req.WorkspaceID {
			return false
		}
		if binding.FolderID != "" && binding.FolderID != req.FolderID {
			return false
		}
		if binding.ActivityID != "" && binding.ActivityID != req.ActivityID {
			return false
		}
		return binding.NodeID == "" || binding.NodeID == req.NodeID
	default:
		return false
	}
}

func serviceInterfaceBindingEnvironmentMatches(binding marketing.ServiceInterfaceBinding, req serviceInterfaceAccessRequest) bool {
	bindingEnv := normalizeServiceProviderEnvironment(binding.Environment)
	if bindingEnv == "" || serviceProviderDefaultEnvironment(bindingEnv) {
		return true
	}
	requestEnv := normalizeServiceProviderEnvironment(req.Environment)
	return requestEnv != "" && bindingEnv == requestEnv
}

func serviceInterfaceBindingPrincipalMatches(binding marketing.ServiceInterfaceBinding, req serviceInterfaceAccessRequest) bool {
	principalID := strings.TrimSpace(binding.PrincipalID)
	if principalID == "*" {
		if binding.PrincipalType == marketing.ServicePrincipalRole && binding.MinRole != "" {
			return serviceInterfaceActorHasMinRole(req.ActorRoles, binding.MinRole)
		}
		return true
	}
	switch binding.PrincipalType {
	case marketing.ServicePrincipalRole:
		requiredRole := marketing.WorkspaceRole(strings.ToUpper(principalID))
		if serviceInterfaceRoleRank(requiredRole) < 0 && binding.MinRole == "" {
			return workflowStringInList(principalID, req.ActorRoles)
		}
		if binding.MinRole != "" && serviceInterfaceRoleRank(binding.MinRole) > serviceInterfaceRoleRank(requiredRole) {
			requiredRole = binding.MinRole
		}
		if serviceInterfaceRoleRank(requiredRole) < 0 {
			return false
		}
		return serviceInterfaceActorHasMinRole(req.ActorRoles, requiredRole)
	case marketing.ServicePrincipalUser:
		return principalID != "" && principalID == req.ActorID
	case marketing.ServicePrincipalGroup:
		return workflowStringInList(principalID, req.ActorGroups)
	case marketing.ServicePrincipalAccount:
		return principalID != "" && principalID == req.ServiceAccount
	case marketing.ServicePrincipalWorkspace:
		return principalID == "" || principalID == req.WorkspaceID
	case marketing.ServicePrincipalFolder:
		return principalID == "" || principalID == req.FolderID
	case marketing.ServicePrincipalActivity:
		return principalID == "" || principalID == req.ActivityID
	case marketing.ServicePrincipalNode:
		return principalID == "" || principalID == req.NodeID
	default:
		return false
	}
}

func serviceInterfaceActorHasMinRole(actorRoles []string, minRole marketing.WorkspaceRole) bool {
	requiredRank := serviceInterfaceRoleRank(minRole)
	if requiredRank < 0 {
		return false
	}
	for _, actorRole := range actorRoles {
		if serviceInterfaceRoleRank(marketing.WorkspaceRole(strings.ToUpper(strings.TrimSpace(actorRole)))) >= requiredRank {
			return true
		}
	}
	return false
}

func serviceInterfaceRoleRank(role marketing.WorkspaceRole) int {
	switch marketing.WorkspaceRole(strings.ToUpper(strings.TrimSpace(string(role)))) {
	case marketing.WorkspaceRoleViewer:
		return 1
	case marketing.WorkspaceRoleEditor:
		return 2
	case marketing.WorkspaceRoleAdmin:
		return 3
	case marketing.WorkspaceRoleOwner:
		return 4
	default:
		return -1
	}
}

func serviceInterfaceAccessRequestWithActorContext(ctx context.Context, req serviceInterfaceAccessRequest) serviceInterfaceAccessRequest {
	source := marketing.AuditSourceFromContext(ctx)
	if req.ActorID == "" {
		req.ActorID = firstWorkflowString(workflowConfigString(source, "actor"), firstWorkflowString(workflowConfigString(source, "actorId"), workflowConfigString(source, "operator")))
	}
	if req.ServiceAccount == "" {
		req.ServiceAccount = firstWorkflowString(workflowConfigString(source, "serviceAccount"), workflowConfigString(source, "serviceAccountId"))
	}
	if len(req.ActorRoles) == 0 {
		req.ActorRoles = append(req.ActorRoles, serviceInterfaceAccessStringList(source["roles"])...)
		req.ActorRoles = append(req.ActorRoles, serviceInterfaceAccessStringList(source["actorRoles"])...)
		if role := workflowConfigString(source, "role"); role != "" {
			req.ActorRoles = append(req.ActorRoles, role)
		}
	}
	if len(req.ActorGroups) == 0 {
		req.ActorGroups = append(req.ActorGroups, serviceInterfaceAccessStringList(source["groups"])...)
		req.ActorGroups = append(req.ActorGroups, serviceInterfaceAccessStringList(source["actorGroups"])...)
		if group := workflowConfigString(source, "group"); group != "" {
			req.ActorGroups = append(req.ActorGroups, group)
		}
	}
	return req
}

func serviceInterfaceAccessStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
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
		for _, item := range parts {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func serviceInterfaceAccessTrace(decision serviceInterfaceAccessDecision, req serviceInterfaceAccessRequest) map[string]any {
	trace := map[string]any{
		"serviceRef":         req.ServiceRef,
		"interfaceRef":       req.InterfaceRef,
		"providerPolicy":     req.ProviderPolicy,
		"providerRef":        req.ProviderRef,
		"environment":        req.Environment,
		"allowed":            decision.Allowed,
		"matchedBindingIds":  decision.MatchedBindingIDs,
		"requiresApproval":   false,
		"allowExplicitRef":   false,
		"bindingScope":       "",
		"bindingPrincipal":   "",
		"bindingPrincipalId": "",
		"actorId":            req.ActorID,
		"actorRoles":         req.ActorRoles,
		"actorGroups":        req.ActorGroups,
		"serviceAccount":     req.ServiceAccount,
	}
	if decision.Binding.ID != "" {
		trace["requiresApproval"] = decision.Binding.RequireApproval
		trace["allowExplicitRef"] = decision.Binding.AllowExplicitProviderRef
		trace["bindingScope"] = decision.Binding.Scope
		trace["bindingPrincipal"] = decision.Binding.PrincipalType
		trace["bindingPrincipalId"] = decision.Binding.PrincipalID
		trace["bindingEnvironment"] = decision.Binding.Environment
		trace["bindingMinRole"] = decision.Binding.MinRole
		trace["bindingRateLimitRef"] = decision.Binding.RateLimitRef
	}
	if decision.Code != "" {
		trace["errorCode"] = decision.Code
		trace["reason"] = decision.Message
	}
	return trace
}
