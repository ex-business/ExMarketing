package marketing

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestWorkflowValidationServiceValidatesDefinition(t *testing.T) {
	service := NewWorkflowValidationService()
	result := service.ValidateDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected workflow validation service to pass, got %+v", result)
	}
}

func TestWorkflowNodeInputMappingRefJSONFields(t *testing.T) {
	bytes, err := json.Marshal(WorkflowNodeInputMappingRef{
		SourceNodeID: "node_inbox",
		OutputKey:    "event_context",
		Path:         "$.user.id",
	})
	if err != nil {
		t.Fatalf("marshal input mapping ref: %v", err)
	}
	output := string(bytes)
	for _, field := range []string{
		`"sourceNodeId":"node_inbox"`,
		`"outputKey":"event_context"`,
		`"path":"$.user.id"`,
	} {
		if !strings.Contains(output, field) {
			t.Fatalf("expected JSON field %s in %s", field, output)
		}
	}
}

func TestWorkflowEdgeRouteJSONFields(t *testing.T) {
	bytes, err := json.Marshal(WorkflowEdge{
		ID:     "qualify-risk",
		Source: "qualify",
		Target: "limit",
		Route: &WorkflowEdgeRoute{
			Type:         "RISK",
			Key:          "RISK",
			Label:        "RISK",
			Expression:   `decision == "RISK"`,
			ConditionRef: "qualify.case_3",
			Default:      true,
		},
	})
	if err != nil {
		t.Fatalf("marshal workflow edge route: %v", err)
	}
	output := string(bytes)
	for _, field := range []string{
		`"type":"RISK"`,
		`"key":"RISK"`,
		`"label":"RISK"`,
		`"expression":"decision == \"RISK\""`,
		`"conditionRef":"qualify.case_3"`,
		`"default":true`,
	} {
		if !strings.Contains(output, field) {
			t.Fatalf("expected JSON field %s in %s", field, output)
		}
	}
}

func TestWorkflowEdgeRouteModelBehavior(t *testing.T) {
	risk := WorkflowEdgeRoute{Type: WorkflowEdgeRouteType("risk"), Key: "risk", Expression: `decision == "RISK"`, Default: true}
	if !risk.IsSupported() {
		t.Fatalf("expected risk route to be supported")
	}
	if risk.NormalizedType() != WorkflowEdgeRouteRISK || risk.NormalizedKey() != "RISK" || risk.OutputKey() != "RISK" {
		t.Fatalf("unexpected normalized risk route: %+v type=%s key=%s output=%s", risk, risk.NormalizedType(), risk.NormalizedKey(), risk.OutputKey())
	}
	if !risk.IsDefault() {
		t.Fatalf("expected explicit default route")
	}
	if risk.RequiresCondition() {
		t.Fatalf("route with expression should not require condition")
	}
	passMissingCondition := WorkflowEdgeRoute{Type: WorkflowEdgeRoutePASS, Key: "PASS"}
	if !passMissingCondition.RequiresCondition() {
		t.Fatalf("PASS route without expression or conditionRef must require condition")
	}
	unsupported := WorkflowEdgeRoute{Type: WorkflowEdgeRouteType("UNKNOWN")}
	if unsupported.IsSupported() {
		t.Fatalf("unknown route type should be unsupported")
	}
}

func TestValidateWorkflowDefinitionRequiresEntryNode(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "match", "type": "match", "kind": "logic"}
		],
		"edges": []
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 1 {
		t.Fatalf("expected one blocking issue, got %+v", result.Summary)
	}
	if len(result.Items) != 1 || result.Items[0].Code != "WORKFLOW_ENTRY_MISSING" {
		t.Fatalf("expected entry missing issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRejectsDuplicateNodeID(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "event-in", "type": "match", "kind": "logic"}
		],
		"edges": []
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 1 {
		t.Fatalf("expected one blocking issue, got %+v", result.Summary)
	}
	if len(result.Items) != 1 || result.Items[0].Code != "WORKFLOW_NODE_ID_DUPLICATED" || result.Items[0].NodeID != "event-in" {
		t.Fatalf("expected duplicate node id issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRequiresWorkflowMetadata(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 3 {
		t.Fatalf("expected three blocking issues, got %+v", result.Summary)
	}
	issues := map[string]WorkflowValidationIssue{}
	for _, item := range result.Items {
		issues[item.Code] = item
	}
	if issues["WORKFLOW_METADATA_ACTIVITY_ID_REQUIRED"].Path != "meta.activityId" {
		t.Fatalf("expected activity metadata issue, got %+v", result.Items)
	}
	if issues["WORKFLOW_METADATA_WORKSPACE_ID_REQUIRED"].Path != "meta.workspaceId" {
		t.Fatalf("expected workspace metadata issue, got %+v", result.Items)
	}
	if issues["WORKFLOW_METADATA_FOLDER_ID_REQUIRED"].Path != "meta.folderId" {
		t.Fatalf("expected folder metadata issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionValidatesNodeConfigSchema(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "http", "type": "http-call", "config": {"method": "POST"}}
		],
		"edges": [
			{"id": "event-in-http", "source": "event-in", "target": "http"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if len(result.Items) != 1 || result.Items[0].Code != "WORKFLOW_NODE_CONFIG_REQUIRED" || result.Items[0].NodeID != "http" || result.Items[0].Path != "nodes[1].config.url" {
		t.Fatalf("expected required node config issue, got %+v", result.Items)
	}

	ok := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "http", "type": "http-call", "config": {"url": "https://reward.example.com", "method": "POST", "timeoutSeconds": 5}}
		],
		"edges": [
			{"id": "event-in-http", "source": "event-in", "target": "http"}
		]
	}`))
	if !ok.Valid {
		t.Fatalf("expected valid workflow config, got %+v", ok)
	}

	ruleViolation := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "http", "type": "http-call", "config": {"url": "ftp://reward.example.com", "method": "POST"}}
		],
		"edges": [
			{"id": "event-in-http", "source": "event-in", "target": "http"}
		]
	}`))
	if ruleViolation.Valid {
		t.Fatalf("expected workflow validation rule violation")
	}
	if len(ruleViolation.Items) != 1 || ruleViolation.Items[0].Code != "WORKFLOW_NODE_CONFIG_RULE_VIOLATION" || ruleViolation.Items[0].NodeID != "http" || ruleViolation.Items[0].Path != "nodes[1].config.url" {
		t.Fatalf("expected config rule violation issue, got %+v", ruleViolation.Items)
	}

	missingCodeSource := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "code", "type": "code", "config": {"language": "LUA"}}
		],
		"edges": [
			{"id": "event-in-code", "source": "event-in", "target": "code"}
		]
	}`))
	if missingCodeSource.Valid {
		t.Fatalf("expected missing code source validation issue")
	}
	if len(missingCodeSource.Items) != 1 || missingCodeSource.Items[0].Code != "WORKFLOW_NODE_CONFIG_REQUIRED" || missingCodeSource.Items[0].NodeID != "code" || missingCodeSource.Items[0].Path != "nodes[1].config.source" {
		t.Fatalf("expected code source required issue, got %+v", missingCodeSource.Items)
	}
}

func TestValidateWorkflowDefinitionValidatesFactQueryProviderTargets(t *testing.T) {
	validHTTP := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "source": "HTTP_PROVIDER", "endpoint": "http://127.0.0.1:8080/facts"}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if !validHTTP.Valid {
		t.Fatalf("expected HTTP provider endpoint to pass validation, got %+v", validHTTP.Items)
	}

	legacyUseEmpty := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["CHECKIN_STREAK"], "source": "FACT_SERVICE", "subjectPath": "subject.id", "timeoutSeconds": 3, "failureAction": "USE_EMPTY"}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if !legacyUseEmpty.Valid {
		t.Fatalf("expected legacy FACT_SERVICE USE_EMPTY config without target to pass validation, got %+v", legacyUseEmpty.Items)
	}

	missingBlockingFactServiceTarget := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "source": "FACT_SERVICE", "failureAction": "BLOCK"}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if missingBlockingFactServiceTarget.Valid {
		t.Fatalf("expected BLOCK FACT_SERVICE config without target to fail")
	}
	if len(missingBlockingFactServiceTarget.Items) != 1 || missingBlockingFactServiceTarget.Items[0].Code != "WORKFLOW_FACT_PROVIDER_TARGET_REQUIRED" || missingBlockingFactServiceTarget.Items[0].Path != "nodes[1].config.endpoint" {
		t.Fatalf("expected missing FACT_SERVICE target issue, got %+v", missingBlockingFactServiceTarget.Items)
	}

	missingRedisResource := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "source": "REDIS", "redisRef": "marketing-fact-cache"}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if missingRedisResource.Valid {
		t.Fatalf("expected REDIS provider without resourceRef to fail")
	}
	if len(missingRedisResource.Items) != 1 || missingRedisResource.Items[0].Code != "WORKFLOW_FACT_PROVIDER_TARGET_REQUIRED" || missingRedisResource.Items[0].Path != "nodes[1].config.resourceRef" {
		t.Fatalf("expected resourceRef target issue, got %+v", missingRedisResource.Items)
	}

	validChain := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "providerChain": [
				{"source": "WORKFLOW_CONTEXT", "sourcePath": "$.attributes.USER_PROFILE", "onMiss": "NEXT"},
				{"source": "FACT_SERVICE", "resourceRef": "default_user_profile_store", "namespace": "facts", "onMiss": "BLOCK"}
			]}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if !validChain.Valid {
		t.Fatalf("expected resource-backed provider chain to pass validation, got %+v", validChain.Items)
	}

	missingChainResource := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "fact-query", "config": {"factKeys": ["USER_PROFILE"], "providerChain": [
				{"source": "REDIS", "redisRef": "marketing-fact-cache", "onMiss": "BLOCK"}
			]}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if missingChainResource.Valid {
		t.Fatalf("expected provider chain REDIS step without resourceRef to fail")
	}
	if len(missingChainResource.Items) != 1 || missingChainResource.Items[0].Code != "WORKFLOW_FACT_PROVIDER_TARGET_REQUIRED" || missingChainResource.Items[0].Path != "nodes[1].config.providerChain[0].resourceRef" {
		t.Fatalf("expected provider chain resourceRef target issue, got %+v", missingChainResource.Items)
	}

	validLuaQuery := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "facts", "type": "lua-query", "config": {"factKeys": ["USER_PROFILE"], "source": "facts = { USER_PROFILE = { exists = true } }", "failureAction": "BLOCK"}}
		],
		"edges": [
			{"id": "event-in-facts", "source": "event-in", "target": "facts"}
		]
	}`))
	if !validLuaQuery.Valid {
		t.Fatalf("expected lua-query source script to pass provider target validation, got %+v", validLuaQuery.Items)
	}
}

func TestValidateWorkflowDefinitionValidatesInputMappingReferences(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		code string
		path string
	}{
		{
			name: "input mapping must be object",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "match", "type": "match", "config": {"inputMapping": []}}
				],
				"edges": [
					{"id": "event-in-match", "source": "event-in", "target": "match"}
				]
			}`),
			code: "WORKFLOW_NODE_INPUT_MAPPING_INVALID",
			path: "nodes[1].config.inputMapping",
		},
		{
			name: "input key must be declared",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "match", "type": "match", "config": {"inputMapping": {"unknown": {"sourceNodeId": "event-in", "outputKey": "raw_event"}}}}
				],
				"edges": [
					{"id": "event-in-match", "source": "event-in", "target": "match"}
				]
			}`),
			code: "WORKFLOW_NODE_INPUT_MAPPING_INPUT_UNSUPPORTED",
			path: "nodes[1].config.inputMapping.unknown",
		},
		{
			name: "source node must exist",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "match", "type": "match", "config": {"inputMapping": {"event_context": {"sourceNodeId": "missing", "outputKey": "event_context"}}}}
				],
				"edges": [
					{"id": "event-in-match", "source": "event-in", "target": "match"}
				]
			}`),
			code: "WORKFLOW_NODE_INPUT_MAPPING_SOURCE_MISSING",
			path: "nodes[1].config.inputMapping.event_context.sourceNodeId",
		},
		{
			name: "source node must be connected to target",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "inbox", "type": "inbox"},
					{"id": "match", "type": "match", "config": {"inputMapping": {"event_context": {"sourceNodeId": "inbox", "outputKey": "event_context"}}}}
				],
				"edges": [
					{"id": "event-in-inbox", "source": "event-in", "sourceHandle": "raw_event", "target": "inbox"},
					{"id": "event-in-match", "source": "event-in", "sourceHandle": "eventId", "target": "match"}
				]
			}`),
			code: "WORKFLOW_NODE_INPUT_MAPPING_SOURCE_NOT_CONNECTED",
			path: "nodes[2].config.inputMapping.event_context.sourceNodeId",
		},
		{
			name: "source output must be declared",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "inbox", "type": "inbox"},
					{"id": "match", "type": "match", "config": {"inputMapping": {"event_context": {"sourceNodeId": "inbox", "outputKey": "missing_output"}}}}
				],
				"edges": [
					{"id": "event-in-inbox", "source": "event-in", "target": "inbox"},
					{"id": "inbox-match", "source": "inbox", "target": "match"}
				]
			}`),
			code: "WORKFLOW_NODE_INPUT_MAPPING_OUTPUT_MISSING",
			path: "nodes[2].config.inputMapping.event_context.outputKey",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateWorkflowDefinition(tt.raw)
			if result.Valid {
				t.Fatalf("expected workflow validation to fail")
			}
			found := false
			for _, item := range result.Items {
				if item.Code == tt.code && item.Path == tt.path {
					found = true
					if item.NodeID == "" {
						t.Fatalf("expected nodeId on issue %+v", item)
					}
				}
			}
			if !found {
				t.Fatalf("expected issue %s at %s, got %+v", tt.code, tt.path, result.Items)
			}
		})
	}
}

func TestValidateWorkflowDefinitionAllowsTransitiveUpstreamInputMapping(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "inbox", "type": "inbox"},
			{"id": "match", "type": "match", "config": {"inputMapping": {"event_context": {"sourceNodeId": "event-in", "outputKey": "raw_event", "path": "$.subject"}}}}
		],
		"edges": [
			{"id": "event-in-inbox", "source": "event-in", "target": "inbox"},
			{"id": "inbox-match", "source": "inbox", "target": "match"}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected transitive upstream input mapping to be valid, got %+v", result)
	}
}

func TestValidateWorkflowDefinitionRequiresInputMappingForCoreContract(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1", "contractVersion": "core-node-contract.v1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "inbox", "type": "inbox", "config": {"inputMapping": {"raw_event": {"sourceNodeId": "event-in", "outputKey": "raw_event"}}}},
			{"id": "match", "type": "match"}
		],
		"edges": [
			{"id": "event-in-inbox", "source": "event-in", "target": "inbox"},
			{"id": "inbox-match", "source": "inbox", "target": "match"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected core contract workflow to require input mapping")
	}
	for _, item := range result.Items {
		if item.Code == "WORKFLOW_NODE_INPUT_MAPPING_REQUIRED" && item.NodeID == "match" && item.Path == "nodes[2].config.inputMapping.event_context" {
			return
		}
	}
	t.Fatalf("expected missing match.event_context input mapping issue, got %+v", result.Items)
}

func TestWorkflowValidationIssuesAreStableExplicitAndLocatable(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
	}{
		{
			name: "definition required",
			raw:  json.RawMessage(`null`),
		},
		{
			name: "definition invalid",
			raw:  json.RawMessage(`{"nodes":[{"id":"event-in","type":"event-in","unknown":true}],"edges":[]}`),
		},
		{
			name: "nodes required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[],"edges":[]}`),
		},
		{
			name: "node id required and entry missing",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"type":"match"}],"edges":[]}`),
		},
		{
			name: "node type required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","kind":"entry"},{"id":"match"}],"edges":[{"id":"event-in-match","source":"event-in","target":"match"}]}`),
		},
		{
			name: "node type unsupported",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"custom","type":"custom-node"}],"edges":[{"id":"event-in-custom","source":"event-in","target":"custom"}]}`),
		},
		{
			name: "node config invalid",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"http","type":"http-call","config":[]}],"edges":[{"id":"event-in-http","source":"event-in","target":"http"}]}`),
		},
		{
			name: "node config required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"http","type":"http-call","config":{"method":"POST"}}],"edges":[{"id":"event-in-http","source":"event-in","target":"http"}]}`),
		},
		{
			name: "node config type invalid",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"http","type":"http-call","config":{"url":123}}],"edges":[{"id":"event-in-http","source":"event-in","target":"http"}]}`),
		},
		{
			name: "node config enum invalid",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"http","type":"http-call","config":{"url":"https://example.com","method":"DELETE"}}],"edges":[{"id":"event-in-http","source":"event-in","target":"http"}]}`),
		},
		{
			name: "node config rule violation",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"http","type":"http-call","config":{"url":"ftp://example.com","method":"POST"}}],"edges":[{"id":"event-in-http","source":"event-in","target":"http"}]}`),
		},
		{
			name: "node input mapping issues",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"inbox","type":"inbox"},{"id":"match","type":"match","config":{"inputMapping":{"unknown":{"sourceNodeId":"event-in","outputKey":"raw_event"},"event_context":{"sourceNodeId":"inbox","outputKey":"missing_output","path":"user.id"},"activity_config":{"sourceNodeId":"missing","outputKey":"config"}}}}],"edges":[{"id":"event-in-inbox","source":"event-in","target":"inbox"},{"id":"inbox-match","source":"inbox","target":"match"}]}`),
		},
		{
			name: "node input mapping invalid",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match","config":{"inputMapping":[]}}],"edges":[{"id":"event-in-match","source":"event-in","target":"match"}]}`),
		},
		{
			name: "node input mapping source not connected",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"inbox","type":"inbox"},{"id":"match","type":"match","config":{"inputMapping":{"event_context":{"sourceNodeId":"inbox","outputKey":"event_context"}}}}],"edges":[{"id":"event-in-inbox","source":"event-in","sourceHandle":"raw_event","target":"inbox"},{"id":"event-in-match","source":"event-in","sourceHandle":"eventId","target":"match"}]}`),
		},
		{
			name: "metadata required",
			raw:  json.RawMessage(`{"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match"}],"edges":[{"id":"event-in-match","source":"event-in","target":"match"}]}`),
		},
		{
			name: "edge endpoint required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match"}],"edges":[{"id":"bad-edge","source":"event-in"}]}`),
		},
		{
			name: "edge id duplicated",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"normalize","type":"normalize"},{"id":"reward","type":"reward"}],"edges":[{"id":"same-edge","source":"event-in","target":"normalize"},{"id":" same-edge ","source":"normalize","target":"reward"}]}`),
		},
		{
			name: "edge route type required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match"}],"edges":[{"id":"event-in-match","source":"event-in","target":"match","route":{"key":"raw_event","label":"raw_event"}}]}`),
		},
		{
			name: "edge output route key required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match"}],"edges":[{"id":"event-in-match","source":"event-in","target":"match","route":{"type":"OUTPUT"}}]}`),
		},
		{
			name: "edge route unsupported",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"match","type":"match"}],"edges":[{"id":"event-in-match","source":"event-in","target":"match","route":{"type":"UNKNOWN","key":"UNKNOWN","label":"Unknown"}}]}`),
		},
		{
			name: "edge route condition required",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"qualify","type":"qualify","kind":"logic"},{"id":"limit","type":"limit","kind":"quota"}],"edges":[{"id":"event-in-qualify","source":"event-in","target":"qualify"},{"id":"qualify-risk","source":"qualify","target":"limit","route":{"type":"RISK","key":"RISK","label":"RISK"}}]}`),
		},
		{
			name: "branch route missing",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"qualify","type":"qualify","kind":"logic"},{"id":"reward","type":"reward","kind":"reward"},{"id":"limit","type":"limit","kind":"quota"}],"edges":[{"id":"event-in-qualify","source":"event-in","target":"qualify"},{"id":"qualify-pass","source":"qualify","target":"reward","route":{"type":"PASS","key":"PASS","label":"PASS","expression":"decision == \"PASS\""}},{"id":"qualify-risk","source":"qualify","target":"limit","route":{"type":"RISK","key":"RISK","label":"RISK","expression":"decision == \"RISK\"","default":true}}]}`),
		},
		{
			name: "branch default missing",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"condition","type":"condition","kind":"logic"},{"id":"reward","type":"reward","kind":"reward"},{"id":"audit","type":"audit","kind":"audit"}],"edges":[{"id":"event-in-condition","source":"event-in","target":"condition"},{"id":"condition-if","source":"condition","target":"reward","route":{"type":"IF","key":"IF","label":"IF","expression":"condition == true"}},{"id":"condition-else","source":"condition","target":"audit","route":{"type":"OUTPUT","key":"ELSE","label":"ELSE"}}]}`),
		},
		{
			name: "branch default duplicated",
			raw:  json.RawMessage(`{"meta":{"activityId":"activity-1","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"condition","type":"condition","kind":"logic"},{"id":"reward","type":"reward","kind":"reward"},{"id":"audit","type":"audit","kind":"audit"}],"edges":[{"id":"event-in-condition","source":"event-in","target":"condition"},{"id":"condition-if","source":"condition","target":"reward","route":{"type":"IF","key":"IF","label":"IF","expression":"condition == true","default":true}},{"id":"condition-else","source":"condition","target":"audit","route":{"type":"ELSE","key":"ELSE","label":"ELSE"}}]}`),
		},
		{
			name: "topology issues",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "event-in", "type": "match"},
					{"id": "qualify", "type": "qualify"},
					{"id": "reward", "type": "reward"}
				],
				"edges": [
					{"id": "missing-source", "source": "missing", "target": "qualify"},
					{"id": "missing-target", "source": "event-in", "target": "missing"}
				]
			}`),
		},
		{
			name: "execution graph issues",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "match", "type": "match"},
					{"id": "qualify", "type": "qualify"},
					{"id": "reward", "type": "reward"},
					{"id": "audit", "type": "audit"}
				],
				"edges": [
					{"id": "event-in-match", "source": "event-in", "sourceHandle": "raw_event", "target": "match"},
					{"id": "event-in-qualify", "source": "event-in", "sourceHandle": "raw_event", "target": "qualify"},
					{"id": "qualify-reward", "source": "qualify", "target": "reward"},
					{"id": "reward-qualify", "source": "reward", "target": "qualify"}
				]
			}`),
		},
		{
			name: "unreachable subgraph",
			raw: json.RawMessage(`{
				"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
				"nodes": [
					{"id": "event-in", "type": "event-in", "kind": "entry"},
					{"id": "match", "type": "match"},
					{"id": "qualify", "type": "qualify"},
					{"id": "reward", "type": "reward"}
				],
				"edges": [
					{"id": "event-in-match", "source": "event-in", "target": "match"},
					{"id": "qualify-reward", "source": "qualify", "target": "reward"}
				]
			}`),
		},
	}
	codePattern := regexp.MustCompile(`^WORKFLOW_[A-Z0-9_]+$`)
	seenCodes := map[string]bool{}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateWorkflowDefinition(tt.raw)
			if len(result.Items) == 0 {
				t.Fatalf("expected validation issues")
			}
			for _, item := range result.Items {
				if item.Severity != WorkflowValidationBlock && item.Severity != WorkflowValidationWarning {
					t.Fatalf("issue severity must be stable, got %+v", item)
				}
				if !codePattern.MatchString(item.Code) {
					t.Fatalf("issue code must be stable WORKFLOW_* format, got %+v", item)
				}
				if item.Message == "" {
					t.Fatalf("issue message must be explicit, got %+v", item)
				}
				if item.NodeID == "" && item.EdgeID == "" && item.Path == "" {
					t.Fatalf("issue must be locatable by nodeId, edgeId or path, got %+v", item)
				}
				seenCodes[item.Code] = true
			}
		})
	}
	for _, code := range []string{
		"WORKFLOW_DEFINITION_REQUIRED",
		"WORKFLOW_DEFINITION_INVALID",
		"WORKFLOW_NODES_REQUIRED",
		"WORKFLOW_METADATA_ACTIVITY_ID_REQUIRED",
		"WORKFLOW_METADATA_WORKSPACE_ID_REQUIRED",
		"WORKFLOW_METADATA_FOLDER_ID_REQUIRED",
		"WORKFLOW_NODE_ID_REQUIRED",
		"WORKFLOW_NODE_ID_DUPLICATED",
		"WORKFLOW_NODE_TYPE_REQUIRED",
		"WORKFLOW_NODE_TYPE_UNSUPPORTED",
		"WORKFLOW_NODE_CONFIG_INVALID",
		"WORKFLOW_NODE_CONFIG_REQUIRED",
		"WORKFLOW_NODE_CONFIG_TYPE_INVALID",
		"WORKFLOW_NODE_CONFIG_ENUM_INVALID",
		"WORKFLOW_NODE_CONFIG_RULE_VIOLATION",
		"WORKFLOW_NODE_INPUT_MAPPING_INVALID",
		"WORKFLOW_NODE_INPUT_MAPPING_INPUT_UNSUPPORTED",
		"WORKFLOW_NODE_INPUT_MAPPING_SOURCE_MISSING",
		"WORKFLOW_NODE_INPUT_MAPPING_SOURCE_NOT_CONNECTED",
		"WORKFLOW_NODE_INPUT_MAPPING_OUTPUT_MISSING",
		"WORKFLOW_NODE_INPUT_MAPPING_PATH_INVALID",
		"WORKFLOW_ENTRY_MISSING",
		"WORKFLOW_EDGE_ID_DUPLICATED",
		"WORKFLOW_EDGE_ENDPOINT_REQUIRED",
		"WORKFLOW_EDGE_ROUTE_TYPE_REQUIRED",
		"WORKFLOW_EDGE_ROUTE_KEY_REQUIRED",
		"WORKFLOW_EDGE_ROUTE_TYPE_UNSUPPORTED",
		"WORKFLOW_EDGE_ROUTE_CONDITION_REQUIRED",
		"WORKFLOW_BRANCH_ROUTE_MISSING",
		"WORKFLOW_BRANCH_DEFAULT_MISSING",
		"WORKFLOW_BRANCH_DEFAULT_DUPLICATED",
		"WORKFLOW_EDGE_SOURCE_MISSING",
		"WORKFLOW_EDGE_TARGET_MISSING",
		"WORKFLOW_OUTPUT_DUPLICATED_EDGE",
		"WORKFLOW_NODE_ISLAND",
		"WORKFLOW_NODE_UNREACHABLE",
		"WORKFLOW_UNSUPPORTED_CYCLE",
	} {
		if !seenCodes[code] {
			t.Fatalf("validation standard test did not cover code %s", code)
		}
	}
}

func TestValidateWorkflowDefinitionRejectsMissingEdgeEndpoints(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"}
		],
		"edges": [
			{"id": "missing-source", "source": "missing", "target": "match"},
			{"id": "missing-target", "source": "event-in", "target": "missing"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 2 {
		t.Fatalf("expected two blocking issues, got %+v", result.Summary)
	}
	issues := map[string]WorkflowValidationIssue{}
	for _, item := range result.Items {
		issues[item.Code] = item
	}
	if issues["WORKFLOW_EDGE_SOURCE_MISSING"].EdgeID != "missing-source" {
		t.Fatalf("expected missing source issue, got %+v", result.Items)
	}
	if issues["WORKFLOW_EDGE_TARGET_MISSING"].EdgeID != "missing-target" {
		t.Fatalf("expected missing target issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRejectsUnreachableNode(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"},
			{"id": "qualify", "type": "qualify", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"},
			{"id": "qualify-reward", "source": "qualify", "target": "reward"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 2 {
		t.Fatalf("expected two blocking issues, got %+v", result.Summary)
	}
	if len(result.Items) != 2 || result.Items[0].Code != "WORKFLOW_NODE_UNREACHABLE" || result.Items[0].NodeID != "qualify" {
		t.Fatalf("expected unreachable node issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRejectsIslandNode(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 1 {
		t.Fatalf("expected one blocking issue, got %+v", result.Summary)
	}
	if len(result.Items) != 1 || result.Items[0].Code != "WORKFLOW_NODE_ISLAND" || result.Items[0].NodeID != "reward" {
		t.Fatalf("expected island node issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRejectsDuplicateOutputEdge(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"},
			{"id": "qualify", "type": "qualify", "kind": "logic"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "sourceHandle": "raw_event", "target": "match"},
			{"id": "event-in-qualify", "source": "event-in", "sourceHandle": "raw_event", "target": "qualify"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 1 {
		t.Fatalf("expected one blocking issue, got %+v", result.Summary)
	}
	if len(result.Items) != 1 ||
		result.Items[0].Code != "WORKFLOW_OUTPUT_DUPLICATED_EDGE" ||
		result.Items[0].NodeID != "event-in" ||
		result.Items[0].EdgeID != "event-in-qualify" {
		t.Fatalf("expected duplicate output edge issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionRejectsDuplicateEdgeID(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "normalize", "type": "normalize", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"}
		],
		"edges": [
			{"id": "same-edge", "source": "event-in", "sourceHandle": "raw_event", "target": "normalize"},
			{"id": " same-edge ", "source": "normalize", "sourceHandle": "normalized_event", "target": "reward"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	var found bool
	for _, item := range result.Items {
		if item.Code == "WORKFLOW_EDGE_ID_DUPLICATED" && item.EdgeID == "same-edge" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate edge id issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionAllowsDifferentOutputHandles(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "pass", "type": "reward", "kind": "reward"},
			{"id": "block", "type": "limit", "kind": "limit"}
		],
		"edges": [
			{"id": "event-in-pass", "source": "event-in", "sourceHandle": "pass", "target": "pass"},
			{"id": "event-in-block", "source": "event-in", "sourceHandle": "block", "target": "block"}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected workflow validation to pass, got %+v", result)
	}
}

func TestValidateWorkflowDefinitionWarnsOnMultipleIncomingEdges(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "reward", "type": "reward", "kind": "reward"},
			{"id": "limit", "type": "limit", "kind": "quota"},
			{"id": "audit", "type": "audit", "kind": "audit"}
		],
		"edges": [
			{"id": "event-reward", "source": "event-in", "target": "reward"},
			{"id": "reward-audit", "source": "reward", "target": "audit"},
			{"id": "event-limit", "source": "event-in", "sourceHandle": "limit", "target": "limit"},
			{"id": "limit-audit", "source": "limit", "target": "audit"}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected workflow validation warning to remain valid, got %+v", result)
	}
	if result.Summary.WarningCount != 1 {
		t.Fatalf("expected one join warning, got %+v", result.Summary)
	}
	if len(result.Items) != 1 ||
		result.Items[0].Severity != WorkflowValidationWarning ||
		result.Items[0].Code != "WORKFLOW_JOIN_OR_SEMANTICS" ||
		result.Items[0].NodeID != "audit" {
		t.Fatalf("expected OR fan-in warning for audit, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionValidatesBranchRouteCoverage(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "qualify", "type": "qualify", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"},
			{"id": "limit", "type": "limit", "kind": "quota"}
		],
		"edges": [
			{"id": "event-in-qualify", "source": "event-in", "target": "qualify"},
			{"id": "qualify-pass", "source": "qualify", "target": "reward", "route": {"type": "PASS", "key": "PASS", "label": "PASS", "expression": "decision == \"PASS\""}},
			{"id": "qualify-risk", "source": "qualify", "target": "limit", "route": {"type": "RISK", "key": "RISK", "label": "RISK", "expression": "decision == \"RISK\"", "default": true}}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected branch coverage validation to fail")
	}
	if result.Summary.BlockCount != 1 ||
		len(result.Items) != 1 ||
		result.Items[0].Code != "WORKFLOW_BRANCH_ROUTE_MISSING" ||
		result.Items[0].NodeID != "qualify" ||
		result.Items[0].Path != "nodes[1].outputs.BLOCK" {
		t.Fatalf("expected missing BLOCK branch issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionAllowsCoveredBranchRoutes(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "switch", "type": "switch", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"},
			{"id": "limit", "type": "limit", "kind": "quota"},
			{"id": "audit", "type": "audit", "kind": "audit"}
		],
		"edges": [
			{"id": "event-in-switch", "source": "event-in", "target": "switch"},
			{"id": "switch-pass", "source": "switch", "target": "reward", "route": {"type": "PASS", "key": "PASS", "label": "PASS", "expression": "decision == \"PASS\""}},
			{"id": "switch-block", "source": "switch", "target": "limit", "route": {"type": "BLOCK", "key": "BLOCK", "label": "BLOCK", "expression": "decision == \"BLOCK\""}},
			{"id": "switch-skip", "source": "switch", "target": "audit", "route": {"type": "SKIP", "key": "SKIP", "label": "SKIP", "expression": "decision == \"SKIP\"", "default": true}}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected covered branch routes to pass, got %+v", result)
	}
}

func TestValidateWorkflowDefinitionRejectsUnsupportedCycle(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"},
			{"id": "reward", "type": "reward", "kind": "reward"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"},
			{"id": "match-reward", "source": "match", "target": "reward"},
			{"id": "reward-match", "source": "reward", "target": "match"}
		]
	}`))
	if result.Valid {
		t.Fatalf("expected workflow validation to fail")
	}
	if result.Summary.BlockCount != 1 {
		t.Fatalf("expected one blocking issue, got %+v", result.Summary)
	}
	var hasCycleBlock bool
	for _, item := range result.Items {
		if item.Severity == WorkflowValidationBlock && item.Code == "WORKFLOW_UNSUPPORTED_CYCLE" {
			hasCycleBlock = true
		}
	}
	if !hasCycleBlock {
		t.Fatalf("expected unsupported cycle issue, got %+v", result.Items)
	}
}

func TestValidateWorkflowDefinitionAcceptsEventInEntryNode(t *testing.T) {
	result := ValidateWorkflowDefinition(json.RawMessage(`{
		"meta": {"activityId": "activity-1", "workspaceId": "workspace-1", "folderId": "folder-1"},
		"nodes": [
			{"id": "event-in", "type": "event-in", "kind": "entry"},
			{"id": "match", "type": "match", "kind": "logic"}
		],
		"edges": [
			{"id": "event-in-match", "source": "event-in", "target": "match"}
		]
	}`))
	if !result.Valid {
		t.Fatalf("expected workflow validation to pass, got %+v", result)
	}
	if result.Summary.BlockCount != 0 || len(result.Items) != 0 {
		t.Fatalf("expected no issues, got %+v", result)
	}
}
