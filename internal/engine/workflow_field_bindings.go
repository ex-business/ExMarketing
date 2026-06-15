package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	workflowFieldBindingModeExpression = "EXPRESSION"
	workflowFieldBindingModeScript     = "SCRIPT"
)

type workflowFieldBinding struct {
	Mode      string `json:"mode"`
	Language  string `json:"language"`
	Source    string `json:"source"`
	OnError   string `json:"onError"`
	Fallback  any    `json:"fallback"`
	ValueType string `json:"valueType"`
}

func (s *Service) resolveWorkflowNodeConfig(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime) (marketing.WorkflowNode, map[string]any, []map[string]any, error) {
	config := workflowNodeConfigMap(node)
	if config == nil {
		return node, config, nil, nil
	}
	bindings := workflowFieldBindings(config)
	if len(bindings) == 0 {
		return node, config, nil, nil
	}
	resolved := cloneWorkflowMap(config)
	delete(resolved, "fieldBindings")
	trace := make([]map[string]any, 0, len(bindings))
	for field, binding := range bindings {
		started := time.Now()
		item := map[string]any{
			"field":    field,
			"mode":     firstWorkflowString(binding.Mode, workflowFieldBindingModeExpression),
			"language": firstWorkflowString(binding.Language, "LUA"),
			"source":   binding.Source,
		}
		value, err := s.evaluateWorkflowFieldBinding(ctx, node, runtime, binding)
		item["durationMs"] = time.Since(started).Milliseconds()
		if err != nil {
			item["status"] = "FAILED"
			item["error"] = err.Error()
			replacement, handled := workflowFieldBindingFallback(binding, config[field])
			if !handled {
				trace = append(trace, item)
				return node, config, trace, fmt.Errorf("field binding %q failed: %w", field, err)
			}
			item["fallbackApplied"] = true
			item["value"] = workflowJSONValue(replacement)
			resolved[field] = workflowJSONValue(replacement)
			trace = append(trace, item)
			continue
		}
		item["status"] = "SUCCESS"
		item["value"] = workflowJSONValue(value)
		resolved[field] = workflowJSONValue(value)
		trace = append(trace, item)
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return node, config, trace, err
	}
	node.Config = encoded
	return node, resolved, trace, nil
}

func workflowFieldBindings(config map[string]any) map[string]workflowFieldBinding {
	raw, ok := config["fieldBindings"].(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := map[string]workflowFieldBinding{}
	for field, value := range raw {
		mapped, ok := value.(map[string]any)
		if !ok {
			continue
		}
		binding := workflowFieldBinding{
			Mode:      strings.ToUpper(firstWorkflowString(workflowOutputString(mapped["mode"]), workflowFieldBindingModeExpression)),
			Language:  strings.ToUpper(firstWorkflowString(workflowOutputString(mapped["language"]), "LUA")),
			Source:    workflowOutputString(mapped["source"]),
			OnError:   strings.ToUpper(firstWorkflowString(workflowOutputString(mapped["onError"]), "FAIL")),
			Fallback:  mapped["fallback"],
			ValueType: strings.ToUpper(workflowOutputString(mapped["valueType"])),
		}
		if strings.TrimSpace(field) == "" || strings.TrimSpace(binding.Source) == "" {
			continue
		}
		out[field] = binding
	}
	return out
}

func (s *Service) evaluateWorkflowFieldBinding(ctx context.Context, node marketing.WorkflowNode, runtime workflowRuntime, binding workflowFieldBinding) (any, error) {
	if binding.Language != "" && binding.Language != "LUA" {
		return nil, fmt.Errorf("unsupported field binding language %s", binding.Language)
	}
	source := strings.TrimSpace(binding.Source)
	if source == "" {
		return nil, fmt.Errorf("field binding source is empty")
	}
	mode := firstWorkflowString(strings.ToUpper(binding.Mode), workflowFieldBindingModeExpression)
	var luaSource string
	switch mode {
	case workflowFieldBindingModeScript:
		luaSource = "emit({value = (function()\n" + source + "\nend)()})"
	default:
		luaSource = "emit({value = (" + source + ")})"
	}
	output, err := s.executeWorkflowLua(ctx, node, runtime, luaSource)
	if err != nil {
		return nil, err
	}
	value, ok := output["value"]
	if !ok {
		return nil, fmt.Errorf("field binding did not return value")
	}
	return workflowCoerceFieldBindingValue(value, binding.ValueType)
}

func workflowFieldBindingFallback(binding workflowFieldBinding, defaultValue any) (any, bool) {
	switch strings.ToUpper(binding.OnError) {
	case "USE_DEFAULT":
		return defaultValue, true
	case "USE_FALLBACK":
		return binding.Fallback, true
	case "NULL":
		return nil, true
	default:
		return nil, false
	}
}

func workflowCoerceFieldBindingValue(value any, valueType string) (any, error) {
	switch strings.ToUpper(strings.TrimSpace(valueType)) {
	case "", "ANY":
		return value, nil
	case "STRING":
		return workflowOutputString(value), nil
	case "NUMBER":
		switch typed := value.(type) {
		case float64:
			return typed, nil
		case int:
			return float64(typed), nil
		default:
			number, err := strconv.ParseFloat(strings.TrimSpace(workflowOutputString(value)), 64)
			if err != nil {
				return nil, err
			}
			return number, nil
		}
	case "BOOLEAN", "BOOL":
		switch typed := value.(type) {
		case bool:
			return typed, nil
		case string:
			return strings.EqualFold(strings.TrimSpace(typed), "true"), nil
		default:
			return value == true, nil
		}
	case "OBJECT":
		if mapped, ok := workflowJSONValue(value).(map[string]any); ok {
			return mapped, nil
		}
		return nil, fmt.Errorf("field binding value is not object")
	case "ARRAY":
		if items, ok := workflowJSONValue(value).([]any); ok {
			return items, nil
		}
		return nil, fmt.Errorf("field binding value is not array")
	default:
		return value, nil
	}
}

func cloneWorkflowMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = workflowJSONValue(value)
	}
	return out
}
