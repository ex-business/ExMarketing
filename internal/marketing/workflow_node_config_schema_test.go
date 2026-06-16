package marketing

import (
	"slices"
	"testing"
)

func TestDefaultWorkflowNodeConfigSchemasCoverSupportedNodeTypes(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{
		"event-in",
		"http-event",
		"webhook-event",
		"mq-event",
		"schedule-event",
		"state-event",
		"manual-event",
		"inbox",
		"normalize",
		"match",
		"fact-query",
		"activity-state",
		"quota-check",
		"code",
		"combine",
		"split",
		"condition",
		"qualify",
		"user-level-check",
		"switch",
		"candidate-reward",
		"reward",
		"decimal",
		"limit",
		"quota",
		"budget-reserve",
		"participation",
		"audit",
		"grant-order",
		"outbox",
		"approval-gate",
		"compensation",
		"random-pick",
		"plugin-execute",
		"ab-bucket",
		"risk-score",
		"settlement-window",
		"manual-adjust",
		"retry",
		"dlq",
		"http-call",
		"mq-publish",
		"webhook",
		"callback",
		"reward-reconcile",
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
		"http-event",
		"webhook-event",
		"mq-event",
		"schedule-event",
		"state-event",
		"manual-event",
		"inbox",
		"normalize",
		"match",
		"condition",
		"qualify",
		"user-level-check",
		"switch",
		"fact-query",
		"activity-state",
		"quota-check",
		"code",
		"combine",
		"split",
		"candidate-reward",
		"reward",
		"limit",
		"quota",
		"budget-reserve",
		"callback",
		"approval-gate",
		"random-pick",
		"plugin-execute",
		"ab-bucket",
		"risk-score",
		"settlement-window",
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
		"user-level-check",
		"switch",
		"fact-query",
		"activity-state",
		"quota-check",
		"code",
		"candidate-reward",
		"reward",
		"limit",
		"quota",
		"budget-reserve",
		"callback",
		"approval-gate",
		"audit",
	} {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected schema for %s", nodeType)
		}
		if len(schema.Fields) == 0 {
			t.Fatalf("expected renderable config fields for %s", nodeType)
		}
		if !slices.Contains([]string{"event-in", "http-event", "webhook-event", "mq-event", "schedule-event", "state-event", "manual-event"}, nodeType) && len(schema.Inputs) == 0 {
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

func TestDefaultWorkflowNodeConfigSchemasClassifyEventInputCoreAdvancedAndOpsNodes(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	cases := map[string]string{
		"event-in":          "EVENT_INPUT",
		"http-event":        "EVENT_INPUT",
		"webhook-event":     "EVENT_INPUT",
		"mq-event":          "EVENT_INPUT",
		"schedule-event":    "EVENT_INPUT",
		"state-event":       "EVENT_INPUT",
		"manual-event":      "EVENT_INPUT",
		"condition":         "ADVANCED",
		"switch":            "ADVANCED",
		"code":              "ADVANCED",
		"combine":           "ADVANCED",
		"split":             "ADVANCED",
		"activity-state":    "STORAGE",
		"state-store":       "STORAGE",
		"lock":              "STORAGE",
		"quota-check":       "CORE",
		"user-level-check":  "CORE",
		"candidate-reward":  "CORE",
		"decimal":           "ADVANCED",
		"quota":             "CORE",
		"budget-reserve":    "CORE",
		"outbox":            "CORE",
		"approval-gate":     "ADVANCED",
		"compensation":      "ADVANCED",
		"random-pick":       "ADVANCED",
		"plugin-execute":    "ADVANCED",
		"ab-bucket":         "ADVANCED",
		"risk-score":        "ADVANCED",
		"settlement-window": "ADVANCED",
		"manual-adjust":     "ADVANCED",
		"http-call":         "ADVANCED",
		"callback":          "ADVANCED",
		"retry":             "OPS",
		"dlq":               "OPS",
		"audit":             "OPS",
		"reward-reconcile":  "OPS",
		"reconcile":         "OPS",
		"alert":             "OPS",
	}
	for nodeType, want := range cases {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected schema for %s", nodeType)
		}
		if schema.Category != want {
			t.Fatalf("expected %s category %s, got %s", nodeType, want, schema.Category)
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
	if slices.Contains(fields["language"].Options, "JAVASCRIPT") || slices.Contains(fields["language"].Options, "PYTHON") {
		t.Fatalf("code schema must only expose implemented languages, got %+v", fields["language"].Options)
	}
	if fields["source"].Type != WorkflowNodeConfigString || !fields["source"].Required {
		t.Fatalf("expected required source string field, got %+v", fields["source"])
	}
}

func TestDefaultWorkflowNodeConfigSchemasExposeOnlyImplementedHandlerOptions(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	conditionSchema, ok := registry.Get("condition")
	if !ok {
		t.Fatalf("expected condition schema")
	}
	conditionFields := workflowSchemaFieldsByKey(conditionSchema)
	if got := conditionFields["expressionLanguage"].Options; slices.Contains(got, "CEL") {
		t.Fatalf("condition schema must not expose unsupported CEL, got %+v", got)
	}
	if !slices.Contains(conditionFields["expressionLanguage"].Options, "RULE_EXPR") || !slices.Contains(conditionFields["expressionLanguage"].Options, "JSON_LOGIC") {
		t.Fatalf("condition schema must expose implemented expression languages, got %+v", conditionFields["expressionLanguage"].Options)
	}

	decimalSchema, ok := registry.Get("decimal")
	if !ok {
		t.Fatalf("expected decimal schema")
	}
	decimalFields := workflowSchemaFieldsByKey(decimalSchema)
	if _, ok := decimalFields["scale"]; ok {
		t.Fatalf("decimal schema must not expose unused scale config")
	}
	if _, ok := decimalFields["roundingMode"]; ok {
		t.Fatalf("decimal schema must not expose unused roundingMode config")
	}
	if decimalFields["amountExpr"].Type != WorkflowNodeConfigString {
		t.Fatalf("decimal schema must expose handler-supported amountExpr, got %+v", decimalFields["amountExpr"])
	}
}

func TestAdvancedAndOpsSchemasExposeExecutionSemantics(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{"http-event", "webhook-event", "mq-event", "schedule-event", "state-event", "manual-event", "http-call", "mq-publish", "webhook", "callback", "retry", "dlq", "reconcile", "alert"} {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected schema for %s", nodeType)
		}
		fields := workflowSchemaFieldsByKey(schema)
		if fields["executionMode"].Type != WorkflowNodeConfigString || fields["executionMode"].Default == "" {
			t.Fatalf("expected executionMode for %s, got %+v", nodeType, fields["executionMode"])
		}
		if fields["executionAuthority"].Type != WorkflowNodeConfigString || fields["executionAuthority"].Default == "" {
			t.Fatalf("expected executionAuthority for %s, got %+v", nodeType, fields["executionAuthority"])
		}
	}
}

func TestProductizedCoreNodeConfigSchemasIncludeGroupsAndPreview(t *testing.T) {
	registry := DefaultWorkflowNodeConfigSchemas()
	for _, nodeType := range []string{"event-in", "inbox", "normalize", "match", "condition", "qualify", "user-level-check", "switch", "fact-query", "code", "combine", "split", "reward", "limit", "quota", "callback", "audit"} {
		schema, ok := registry.Get(nodeType)
		if !ok {
			t.Fatalf("expected schema for %s", nodeType)
		}
		if len(schema.FieldGroups) < 2 {
			t.Fatalf("expected grouped config sections for %s, got %+v", nodeType, schema.FieldGroups)
		}
		if len(schema.Preview.Reads) == 0 || len(schema.Preview.Writes) == 0 {
			t.Fatalf("expected config preview for %s, got %+v", nodeType, schema.Preview)
		}
		groupedFieldCount := 0
		for _, group := range schema.FieldGroups {
			if group.Key == "" || group.Title == "" || len(group.Fields) == 0 {
				t.Fatalf("expected complete field group metadata for %s, got %+v", nodeType, group)
			}
			groupedFieldCount += len(group.Fields)
			for _, field := range group.Fields {
				if field.Description == "" {
					t.Fatalf("expected business description for %s.%s", nodeType, field.Key)
				}
				if field.Required && field.Default == nil {
					t.Fatalf("expected required field %s.%s to include a default value", nodeType, field.Key)
				}
				if field.Type == WorkflowNodeConfigEnum && len(field.Options) < 2 {
					t.Fatalf("expected enum options for %s.%s", nodeType, field.Key)
				}
			}
		}
		if groupedFieldCount != len(schema.Fields) {
			t.Fatalf("expected flat fields to mirror groups for %s, grouped=%d flat=%d", nodeType, groupedFieldCount, len(schema.Fields))
		}
	}
}

func workflowSchemaFieldsByKey(schema WorkflowNodeConfigSchema) map[string]WorkflowNodeConfigField {
	fields := map[string]WorkflowNodeConfigField{}
	for _, field := range schema.Fields {
		fields[field.Key] = field
	}
	return fields
}
