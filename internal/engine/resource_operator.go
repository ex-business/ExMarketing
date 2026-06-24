package engine

import (
	"context"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type ServiceResourceOperator interface {
	ExecuteFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult
	ExecuteResourceAPI(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, req workflowResourceAPIRequest) (map[string]any, error)
}

type workflowResourceOperator struct {
	service *Service
}

func newWorkflowResourceOperator(service *Service) workflowResourceOperator {
	return workflowResourceOperator{service: service}
}

func (op workflowResourceOperator) ExecuteFactQuery(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, config map[string]any) workflowNodeResult {
	return op.service.factQueryResourceRefWorkflowNodeHandler(ctx, node, runtime, config)
}

func (op workflowResourceOperator) ExecuteResourceAPI(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, req workflowResourceAPIRequest) (map[string]any, error) {
	return op.service.workflowExecuteResourceAPI(ctx, node, runtime, req)
}
