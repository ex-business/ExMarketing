package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func (s *Service) validateWorkflowServiceSchemaContracts(ctx context.Context, activity marketing.Activity, definition json.RawMessage, validation marketing.WorkflowValidationResult) marketing.WorkflowValidationResult {
	var workflow marketing.WorkflowDefinition
	if err := json.Unmarshal(definition, &workflow); err != nil {
		return validation
	}
	releaseEnvironment := workflowReleaseEnvironment(workflow)
	serviceOutputSchemas := map[string]json.RawMessage{}
	for nodeIndex, node := range workflow.Nodes {
		config := map[string]any{}
		if len(node.Config) > 0 {
			_ = json.Unmarshal(node.Config, &config)
		}
		for _, step := range workflowServiceInterfaceContractSteps(config) {
			serviceRef := strings.TrimSpace(step.ServiceRef)
			interfaceRef := strings.TrimSpace(step.InterfaceRef)
			path := fmt.Sprintf("nodes[%d].config", nodeIndex)
			if step.Path != "" {
				path = fmt.Sprintf("nodes[%d].config.%s", nodeIndex, step.Path)
			}
			if serviceRef == "" || interfaceRef == "" {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_INTERFACE_REF_REQUIRED",
					Message:  "serviceRef and interfaceRef are required for SERVICE_INTERFACE step",
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			accessReq := serviceInterfaceAccessRequest{
				TenantID:        activity.TenantID,
				WorkspaceID:     activity.WorkspaceID,
				FolderID:        activity.FolderID,
				ActivityID:      activity.ID,
				NodeID:          node.ID,
				NodeType:        node.Type,
				ServiceRef:      serviceRef,
				InterfaceRef:    interfaceRef,
				Operation:       step.Operation,
				ProviderPolicy:  step.ProviderPolicy,
				ProviderRef:     step.ProviderRef,
				Environment:     firstWorkflowString(step.Environment, releaseEnvironment),
				RequireApproval: activityApprovalValid(activity),
			}
			decision, err := s.checkServiceInterfaceAccess(ctx, accessReq)
			if err != nil {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_INTERFACE_ACCESS_CHECK_FAILED",
					Message:  err.Error(),
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			if !decision.Allowed {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     decision.Code,
					Message:  decision.Message,
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			serviceInterface, err := s.GetServiceInterface(ctx, activity.TenantID, serviceRef, interfaceRef)
			if err != nil {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_INTERFACE_NOT_FOUND",
					Message:  fmt.Sprintf("service interface %s/%s is not found", serviceRef, interfaceRef),
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			if !serviceAssetStatusActive(serviceInterface.Status, serviceInterface.DeletedAt) {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_INTERFACE_DISABLED",
					Message:  fmt.Sprintf("service interface %s/%s is not ACTIVE", serviceRef, interfaceRef),
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			if err := s.validateServiceInterfaceSchemas(ctx, serviceInterface); err != nil {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     serviceSchemaValidationCode(err),
					Message:  err.Error(),
					NodeID:   node.ID,
					Path:     path,
				})
				continue
			}
			outputSchema, _ := s.activeServiceInterfaceSchemaJSON(ctx, activity.TenantID, serviceInterface.OutputSchemaRef, marketing.ServiceSchemaOutput)
			if len(outputSchema) > 0 {
				serviceOutputSchemas[node.ID] = outputSchema
			}
			if _, providers, err := s.resolveServiceProviders(ctx, serviceProviderResolveRequest{
				TenantID:       activity.TenantID,
				ServiceRef:     serviceRef,
				InterfaceRef:   interfaceRef,
				ProviderRef:    step.ProviderRef,
				ProviderPolicy: step.ProviderPolicy,
				Environment:    firstWorkflowString(step.Environment, releaseEnvironment),
				SeedParts: []string{
					activity.ID,
					activity.Code,
					fmt.Sprint(activity.Version),
					node.ID,
				},
			}); err != nil {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     serviceProviderResolveValidationCode(err),
					Message:  err.Error(),
					NodeID:   node.ID,
					Path:     path,
				})
			} else if len(providers) == 0 {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_PROVIDER_NOT_FOUND",
					Message:  fmt.Sprintf("service provider candidate for %s/%s is not found", serviceRef, interfaceRef),
					NodeID:   node.ID,
					Path:     path,
				})
			} else {
				for _, provider := range providers {
					if serviceProviderProductionMockDenied(releaseEnvironment, step.ProviderPolicy, provider) {
						validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
							Severity: marketing.WorkflowValidationBlock,
							Code:     "SERVICE_PROVIDER_MOCK_PRODUCTION_DENIED",
							Message:  fmt.Sprintf("mock provider %s is not allowed in production release", provider.Code),
							NodeID:   node.ID,
							Path:     path,
						})
					}
					if err := s.validateProviderMappingCompatibility(ctx, activity.TenantID, serviceInterface, provider); err != nil {
						validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
							Severity: marketing.WorkflowValidationBlock,
							Code:     serviceSchemaValidationCode(err),
							Message:  err.Error(),
							NodeID:   node.ID,
							Path:     path,
						})
					}
					if err := s.validateMockProviderOutputCompatibility(ctx, activity.TenantID, serviceInterface, provider); err != nil {
						validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
							Severity: marketing.WorkflowValidationBlock,
							Code:     serviceSchemaValidationCode(err),
							Message:  err.Error(),
							NodeID:   node.ID,
							Path:     path,
						})
					}
				}
			}
		}
	}
	validation = s.validateWorkflowDownstreamSchemaRefs(workflow, serviceOutputSchemas, validation)
	return validation
}

func (s *Service) activeServiceInterfaceSchemaJSON(ctx context.Context, tenantID string, schemaRef string, schemaType marketing.ServiceSchemaType) (json.RawMessage, error) {
	if strings.TrimSpace(schemaRef) == "" {
		return nil, nil
	}
	schema, err := s.getActiveServiceSchema(ctx, tenantID, schemaRef, schemaType)
	if err != nil {
		return nil, err
	}
	return schema.SchemaJSON, nil
}

func workflowReleaseEnvironment(workflow marketing.WorkflowDefinition) string {
	for _, key := range []string{"releaseEnvironment", "runtimeEnvironment", "environment"} {
		if value := workflowConfigString(workflow.Meta, key); value != "" {
			return value
		}
	}
	return ""
}

func serviceProviderProductionMockDenied(environment string, policy string, provider marketing.ServiceProvider) bool {
	switch normalizeServiceProviderEnvironment(environment) {
	case "prod", "production":
		return strings.EqualFold(strings.TrimSpace(policy), "MOCK_ONLY") || serviceProviderMockEnabled(provider)
	default:
		return false
	}
}

func (s *Service) validateProviderMappingCompatibility(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface, provider marketing.ServiceProvider) error {
	var config map[string]any
	if len(provider.Config) == 0 || json.Unmarshal(provider.Config, &config) != nil {
		return nil
	}
	for _, check := range []struct {
		configKey  string
		schemaRef  string
		schemaType marketing.ServiceSchemaType
		code       string
	}{
		{"requestMapping", serviceInterface.InputSchemaRef, marketing.ServiceSchemaInput, "SERVICE_PROVIDER_REQUEST_MAPPING_INVALID"},
		{"responseMapping", serviceInterface.OutputSchemaRef, marketing.ServiceSchemaOutput, "SERVICE_PROVIDER_RESPONSE_MAPPING_INVALID"},
		{"errorMapping", serviceInterface.ErrorSchemaRef, marketing.ServiceSchemaError, "SERVICE_PROVIDER_ERROR_MAPPING_INVALID"},
	} {
		mapping, ok := config[check.configKey].(map[string]any)
		if !ok || len(mapping) == 0 || strings.TrimSpace(check.schemaRef) == "" {
			continue
		}
		schema, err := s.getActiveServiceSchema(ctx, tenantID, check.schemaRef, check.schemaType)
		if err != nil {
			return fmt.Errorf("%s:%s:%w", check.code, check.schemaRef, err)
		}
		for targetPath := range mapping {
			if err := jsonSchemaCoversPath(schema.SchemaJSON, targetPath); err != nil {
				return fmt.Errorf("%s:%s:%s:%w", check.code, provider.Code, targetPath, err)
			}
		}
	}
	return nil
}

func (s *Service) validateMockProviderOutputCompatibility(ctx context.Context, tenantID string, serviceInterface marketing.ServiceInterface, provider marketing.ServiceProvider) error {
	if !serviceProviderMockEnabled(provider) || strings.TrimSpace(serviceInterface.OutputSchemaRef) == "" {
		return nil
	}
	var config map[string]any
	if len(provider.Config) == 0 || json.Unmarshal(provider.Config, &config) != nil {
		return nil
	}
	var payload any
	for _, key := range []string{"mockOutput", "mockResponse", "mockPayload"} {
		if value, ok := config[key]; ok {
			payload = value
			break
		}
	}
	if payload == nil {
		return nil
	}
	schema, err := s.getActiveServiceSchema(ctx, tenantID, serviceInterface.OutputSchemaRef, marketing.ServiceSchemaOutput)
	if err != nil {
		return fmt.Errorf("SERVICE_PROVIDER_MOCK_OUTPUT_INVALID:%s:%w", provider.Code, err)
	}
	if err := (serviceSchemaValidator{service: s}).validatePayload(schema.SchemaJSON, payload); err != nil {
		return fmt.Errorf("SERVICE_PROVIDER_MOCK_OUTPUT_INVALID:%s:%w", provider.Code, err)
	}
	return nil
}

func (s *Service) validateWorkflowDownstreamSchemaRefs(workflow marketing.WorkflowDefinition, serviceOutputSchemas map[string]json.RawMessage, validation marketing.WorkflowValidationResult) marketing.WorkflowValidationResult {
	if len(serviceOutputSchemas) == 0 {
		return validation
	}
	for nodeIndex, node := range workflow.Nodes {
		for inputKey, ref := range workflowInputMapping(node) {
			outputSchema, ok := serviceOutputSchemas[ref.SourceNodeID]
			if !ok || strings.TrimSpace(ref.Path) == "" || ref.Path == "$" {
				continue
			}
			path := workflowInputMappingSchemaPath(ref)
			if err := jsonSchemaCoversPath(outputSchema, path); err != nil {
				validation = appendWorkflowValidationIssue(validation, marketing.WorkflowValidationIssue{
					Severity: marketing.WorkflowValidationBlock,
					Code:     "SERVICE_SCHEMA_DOWNSTREAM_FIELD_UNCOVERED",
					Message:  fmt.Sprintf("inputMapping %s references %s but upstream output schema does not cover it: %v", inputKey, path, err),
					NodeID:   node.ID,
					Path:     fmt.Sprintf("nodes[%d].config.inputMapping.%s.path", nodeIndex, inputKey),
				})
			}
		}
	}
	return validation
}

func workflowInputMappingSchemaPath(ref marketing.WorkflowNodeInputMappingRef) string {
	outputKey := strings.TrimSpace(ref.OutputKey)
	path := strings.TrimSpace(ref.Path)
	if path == "" || path == "$" {
		return outputKey
	}
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, ".")
	if outputKey == "" {
		return path
	}
	if path == "" {
		return outputKey
	}
	return outputKey + "." + path
}

type workflowServiceInterfaceContractStep struct {
	ServiceRef     string
	InterfaceRef   string
	ProviderPolicy string
	ProviderRef    string
	Environment    string
	Operation      marketing.ServiceInterfaceOperation
	Path           string
}

func workflowServiceInterfaceContractSteps(config map[string]any) []workflowServiceInterfaceContractStep {
	steps := []workflowServiceInterfaceContractStep{}
	if workflowConfigIsServiceInterface(config) {
		steps = append(steps, workflowServiceInterfaceContractStep{
			ServiceRef:     workflowConfigString(config, "serviceRef"),
			InterfaceRef:   workflowConfigString(config, "interfaceRef"),
			ProviderPolicy: workflowConfigString(config, "providerPolicy"),
			ProviderRef:    workflowConfigString(config, "providerRef"),
			Environment:    firstWorkflowString(workflowConfigString(config, "environment"), workflowConfigString(config, "runtimeEnvironment")),
			Operation:      workflowServiceInterfaceOperation(config),
		})
	}
	rawChain, ok := config["providerChain"].([]any)
	if !ok {
		return steps
	}
	for index, rawStep := range rawChain {
		step, ok := workflowJSONValue(rawStep).(map[string]any)
		if !ok {
			continue
		}
		if !workflowConfigIsServiceInterface(step) {
			continue
		}
		steps = append(steps, workflowServiceInterfaceContractStep{
			ServiceRef:     workflowConfigString(step, "serviceRef"),
			InterfaceRef:   workflowConfigString(step, "interfaceRef"),
			ProviderPolicy: workflowConfigString(step, "providerPolicy"),
			ProviderRef:    workflowConfigString(step, "providerRef"),
			Environment:    firstWorkflowString(workflowConfigString(step, "environment"), workflowConfigString(step, "runtimeEnvironment")),
			Operation:      workflowServiceInterfaceOperation(step),
			Path:           fmt.Sprintf("providerChain[%d]", index),
		})
	}
	return steps
}

func serviceProviderResolveValidationCode(err error) string {
	message := err.Error()
	if idx := strings.Index(message, ":"); idx > 0 {
		code := message[:idx]
		if strings.HasPrefix(code, "SERVICE_PROVIDER_") {
			return code
		}
	}
	if errors.Is(err, marketing.ErrNotFound) {
		return "SERVICE_PROVIDER_NOT_FOUND"
	}
	return "SERVICE_PROVIDER_NOT_RESOLVED"
}

func workflowServiceInterfaceOperation(config map[string]any) marketing.ServiceInterfaceOperation {
	operation := strings.ToUpper(strings.TrimSpace(workflowConfigString(config, "operation")))
	if operation == "" {
		return marketing.ServiceOperationCall
	}
	return marketing.ServiceInterfaceOperation(operation)
}

func workflowConfigIsServiceInterface(config map[string]any) bool {
	for _, key := range []string{"targetType", "source", "provider"} {
		if strings.EqualFold(strings.TrimSpace(workflowConfigString(config, key)), "SERVICE_INTERFACE") {
			return true
		}
	}
	return false
}

func appendWorkflowValidationIssue(validation marketing.WorkflowValidationResult, issue marketing.WorkflowValidationIssue) marketing.WorkflowValidationResult {
	validation.Items = append(validation.Items, issue)
	switch issue.Severity {
	case marketing.WorkflowValidationBlock:
		validation.Summary.BlockCount++
	case marketing.WorkflowValidationWarning:
		validation.Summary.WarningCount++
	}
	validation.Valid = validation.Summary.BlockCount == 0
	return validation
}

func serviceSchemaValidationCode(err error) string {
	message := err.Error()
	if index := strings.Index(message, ":"); index > 0 {
		return message[:index]
	}
	return "SERVICE_SCHEMA_INVALID"
}
