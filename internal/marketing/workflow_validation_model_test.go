package marketing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowValidationResultJSONFields(t *testing.T) {
	result := WorkflowValidationResult{
		Valid: false,
		Items: []WorkflowValidationIssue{{
			Severity: WorkflowValidationBlock,
			Code:     "WORKFLOW_NODE_UNREACHABLE",
			Message:  "node is unreachable",
			NodeID:   "reward",
			EdgeID:   "edge-1",
			Path:     "nodes[2]",
		}},
		Summary: WorkflowValidationSummary{BlockCount: 1},
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal validation result: %v", err)
	}
	output := string(bytes)
	for _, field := range []string{
		`"severity":"BLOCK"`,
		`"code":"WORKFLOW_NODE_UNREACHABLE"`,
		`"message":"node is unreachable"`,
		`"nodeId":"reward"`,
		`"edgeId":"edge-1"`,
		`"path":"nodes[2]"`,
	} {
		if !strings.Contains(output, field) {
			t.Fatalf("expected JSON field %s in %s", field, output)
		}
	}
}
