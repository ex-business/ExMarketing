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
			{"id": "code", "type": "code", "config": {"language": "JAVASCRIPT"}}
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
		"WORKFLOW_EDGE_ENDPOINT_REQUIRED",
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
	if len(result.Items) != 1 || result.Items[0].Code != "WORKFLOW_UNSUPPORTED_CYCLE" {
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
