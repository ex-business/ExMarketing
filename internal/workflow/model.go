package workflow

import "time"

type Backlog struct {
	Items []Requirement `json:"items"`
}

type Requirement struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Risk        string    `json:"risk"`
	Score       int       `json:"score"`
	TaskDir     string    `json:"taskDir,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const (
	StatusNew           = "NEW"
	StatusAnalysisReady = "ANALYSIS_READY"
	StatusAnalysisDone  = "ANALYSIS_DONE"
	StatusReviewReady   = "REVIEW_READY"
	StatusReviewPass    = "REVIEW_PASS"
	StatusReviewBlock   = "REVIEW_BLOCK"
	StatusTestPlanReady = "TEST_PLAN_READY"
	StatusTestPlanDone  = "TEST_PLAN_DONE"
	StatusDevReady      = "DEV_READY"
	StatusDevDone       = "DEV_DONE"
	StatusTestExecReady = "TEST_EXEC_READY"
	StatusTestReady     = "TEST_READY"
	StatusTestPass      = "TEST_PASS"
	StatusTestFail      = "TEST_FAIL"
	StatusCommitReady   = "COMMIT_READY"
	StatusCommitted     = "COMMITTED"
)
