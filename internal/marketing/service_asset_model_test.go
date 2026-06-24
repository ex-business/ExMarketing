package marketing

import (
	"encoding/json"
	"testing"
)

func TestServiceSchemaValidate(t *testing.T) {
	valid := ServiceSchema{
		TenantID:   "site_001",
		SchemaRef:  "user-profile-v1",
		SchemaType: ServiceSchemaOutput,
		Scope:      ServiceAssetScopeTenant,
		Version:    1,
		Status:     ServiceAssetActive,
		SchemaJSON: json.RawMessage(`{
			"type": "object",
			"required": ["userId"],
			"properties": {
				"userId": {"type": "string"}
			}
		}`),
		ExamplesJSON: json.RawMessage(`[{"userId":"u_001"}]`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid schema: %v", err)
	}

	invalid := valid
	invalid.SchemaJSON = json.RawMessage(`[]`)
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid schemaJson to fail")
	}

	invalid = valid
	invalid.Version = 0
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected non-positive version to fail")
	}
}

func TestServiceInterfaceBindingValidate(t *testing.T) {
	valid := ServiceInterfaceBinding{
		TenantID:                "site_001",
		ServiceRef:              "user-profile-service",
		InterfaceRef:            "query-user-profile",
		Scope:                   ServiceAssetScopeWorkspace,
		WorkspaceID:             "growth",
		PrincipalType:           ServicePrincipalRole,
		PrincipalID:             "EDITOR",
		AllowedNodeTypes:        []string{"fact-query"},
		AllowedOperations:       []ServiceInterfaceOperation{ServiceOperationCall},
		AllowedProviderPolicies: []string{"PRIMARY", "PRIMARY_WITH_FALLBACK"},
		Status:                  ServiceAssetActive,
		Metadata:                json.RawMessage(`{"reason":"test"}`),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid binding: %v", err)
	}

	invalid := valid
	invalid.AllowExplicitProviderRef = true
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected explicit provider binding without allowedProviderRefs to fail")
	}

	invalid = valid
	invalid.AllowedOperations = []ServiceInterfaceOperation{"DELETE"}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected invalid operation to fail")
	}
}
