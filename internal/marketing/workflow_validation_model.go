package marketing

type WorkflowValidationSeverity string

const (
	WorkflowValidationBlock   WorkflowValidationSeverity = "BLOCK"
	WorkflowValidationWarning WorkflowValidationSeverity = "WARNING"
)

type WorkflowValidationResult struct {
	Valid   bool                      `json:"valid"`
	Items   []WorkflowValidationIssue `json:"items"`
	Summary WorkflowValidationSummary `json:"summary"`
}

type WorkflowValidationSummary struct {
	BlockCount   int `json:"blockCount"`
	WarningCount int `json:"warningCount"`
}

type WorkflowValidationIssue struct {
	Severity WorkflowValidationSeverity `json:"severity"`
	Code     string                     `json:"code"`
	Message  string                     `json:"message"`
	NodeID   string                     `json:"nodeId,omitempty"`
	EdgeID   string                     `json:"edgeId,omitempty"`
	Path     string                     `json:"path,omitempty"`
}
