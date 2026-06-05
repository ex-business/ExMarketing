package marketing

import "testing"

func TestDefaultWorkflowNodeConfigSchemasCoverSupportedNodeTypes(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{
		"event-in",
		"inbox",
		"normalize",
		"match",
		"fact-query",
		"code",
		"condition",
		"qualify",
		"switch",
		"reward",
		"decimal",
		"limit",
		"quota",
		"participation",
		"audit",
		"grant-order",
		"outbox",
		"retry",
		"dlq",
		"http-call",
		"mq-publish",
		"webhook",
		"callback",
		"reconcile",
		"alert",
	} {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected config schema for node type %s", nodeType)
		}
		if schema.NodeType != nodeType || schema.Version <= 0 || schema.Title == "" {
			t.Fatalf("expected complete schema for node type %s, got %+v", nodeType, schema)
		}
	}
}

func TestWorkflowNodeConfigSchemasCoverCoreStudioNodes(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{
		"event-in",
		"inbox",
		"normalize",
		"match",
		"condition",
		"qualify",
		"switch",
		"fact-query",
		"code",
		"reward",
		"limit",
		"quota",
		"callback",
		"audit",
	} {
		if _, ok := registry.Get(nodeType); !ok {
			t.Fatalf("expected core studio node config schema for %s", nodeType)
		}
	}
}

func TestCoreStudioNodeConfigSchemaFieldsAreRenderable(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{
		"event-in",
		"inbox",
		"normalize",
		"match",
		"condition",
		"qualify",
		"switch",
		"fact-query",
		"code",
		"reward",
		"limit",
		"quota",
		"callback",
		"audit",
	} {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected schema for %s", nodeType)
		}
		if len(schema.Fields) == 0 {
			t.Fatalf("expected renderable config fields for %s", nodeType)
		}
		if nodeType != "event-in" && len(schema.Inputs) == 0 {
			t.Fatalf("expected declared input variables for %s", nodeType)
		}
		if len(schema.Outputs) == 0 {
			t.Fatalf("expected declared output variables for %s", nodeType)
		}
		seen := map[string]bool{}
		for _, field := range schema.Fields {
			if field.Key == "" || field.Label == "" || field.Type == "" {
				t.Fatalf("expected stable renderable field metadata for %s, got %+v", nodeType, field)
			}
			if seen[field.Key] {
				t.Fatalf("duplicate field key %s in schema %s", field.Key, nodeType)
			}
			seen[field.Key] = true
			if field.Type == WorkflowNodeConfigEnum && len(field.Options) == 0 {
				t.Fatalf("expected enum options for %s.%s", nodeType, field.Key)
			}
		}
		inputKeys := map[string]bool{}
		for _, input := range schema.Inputs {
			if input.Key == "" || input.Label == "" {
				t.Fatalf("expected stable input variable metadata for %s, got %+v", nodeType, input)
			}
			if inputKeys[input.Key] {
				t.Fatalf("duplicate input key %s in schema %s", input.Key, nodeType)
			}
			inputKeys[input.Key] = true
		}
		outputKeys := map[string]bool{}
		for _, output := range schema.Outputs {
			if output.Key == "" || output.Label == "" {
				t.Fatalf("expected stable output variable metadata for %s, got %+v", nodeType, output)
			}
			if outputKeys[output.Key] {
				t.Fatalf("duplicate output key %s in schema %s", output.Key, nodeType)
			}
			outputKeys[output.Key] = true
		}
	}
}

func TestDefaultWorkflowNodeConfigSchemasAreSorted(t *testing.T) {
	items := DefaultWorkflowNodeConfigSchemas().List()
	if len(items) == 0 {
		t.Fatalf("expected node config schemas")
	}
	for index := 1; index < len(items); index++ {
		if items[index-1].NodeType > items[index].NodeType {
			t.Fatalf("expected schemas sorted by nodeType, got %s before %s", items[index-1].NodeType, items[index].NodeType)
		}
	}
}

func TestDefaultWorkflowNodeConfigSchemasIncludeValidationRules(t *testing.T) {
	schema, ok := DefaultWorkflowNodeConfigSchemas().Get("http-call")
	if !ok {
		t.Fatalf("expected http-call schema")
	}
	for _, field := range schema.Fields {
		if field.Key != "url" {
			continue
		}
		if field.Label == "" || field.Description != "" && field.Key == "" {
			t.Fatalf("expected field label and key, got %+v", field)
		}
		if field.Type != WorkflowNodeConfigString || !field.Required || len(field.ValidationRules) == 0 {
			t.Fatalf("expected url field type, required flag and validation rules, got %+v", field)
		}
		if field.ValidationRules[0].Type != WorkflowNodeConfigRulePattern {
			t.Fatalf("expected url pattern rule, got %+v", field.ValidationRules)
		}
		return
	}
	t.Fatalf("expected http-call url field")
}

func TestDefaultWorkflowNodeConfigSchemasIncludeCodeNodeRequiredSource(t *testing.T) {
	schema, ok := DefaultWorkflowNodeConfigSchemas().Get("code")
	if !ok {
		t.Fatalf("expected code schema")
	}
	fields := map[string]WorkflowNodeConfigField{}
	for _, field := range schema.Fields {
		fields[field.Key] = field
	}
	if fields["language"].Type != WorkflowNodeConfigEnum || !fields["language"].Required {
		t.Fatalf("expected required language enum field, got %+v", fields["language"])
	}
	if fields["source"].Type != WorkflowNodeConfigString || !fields["source"].Required {
		t.Fatalf("expected required source string field, got %+v", fields["source"])
	}
}
