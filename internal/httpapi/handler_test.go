package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/workflow"
)

func newTestHandler() *Handler {
	return NewHandler(engine.NewService(marketing.NewMemoryRepository()))
}

func TestHealth(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestActivityEventCreatesGrantCommand(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	event := `{
		"eventId": "evt_20260524_000001",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"grantOrders"`) {
		t.Fatalf("expected grant orders, got: %s", body)
	}
	if !strings.Contains(body, `"rewardAmount":"59.8000"`) {
		t.Fatalf("expected ratio reward amount, got: %s", body)
	}
	if !strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected outbox message, got: %s", body)
	}
}

func TestActivityEventBudgetExceededReturnsConflictAndAudit(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1")

	event := `{
		"eventId": "evt_20260524_000002",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_124"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_124")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "quota exceeded") || !strings.Contains(body, "BUDGET") {
		t.Fatalf("expected budget quota error, got: %s", body)
	}
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"BUDGET"`) {
		t.Fatalf("expected structured quota error, got: %s", body)
	}
	if strings.Contains(body, `"grantOrders"`) || strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected no grant/outbox response on quota failure, got: %s", body)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/admin/v1/audit", nil)
	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit status %d, got %d: %s", http.StatusOK, auditRec.Code, auditRec.Body.String())
	}
	if !strings.Contains(auditRec.Body.String(), `"action":"QUOTA_EXCEEDED"`) {
		t.Fatalf("expected quota audit record, got: %s", auditRec.Body.String())
	}
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"BUDGET"`) {
		t.Fatalf("expected structured quota audit payload, got: %s", auditRec.Body.String())
	}
}

func TestActivityEventUserLimitExceededReturnsConflictAndAudit(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "10001", "0.1")

	rec := postDepositEvent(t, handler, "evt_user_limit_exceeded", "pay_user_limit_exceeded")

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"USER_LIMIT"`) || !strings.Contains(body, `"scopeKey":"10001"`) {
		t.Fatalf("expected user limit quota error, got: %s", body)
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit", nil))
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"USER_LIMIT"`) {
		t.Fatalf("expected user limit audit payload, got: %s", auditRec.Body.String())
	}
}

func TestActivityEventStockExceededReturnsConflictAndNoRewardableResult(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "0.1")

	rec := postDepositEvent(t, handler, "evt_stock_exceeded", "pay_stock_exceeded")

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"STOCK"`) || !strings.Contains(body, `"scopeKey":"FIRST_DEPOSIT_RATIO"`) {
		t.Fatalf("expected stock quota error, got: %s", body)
	}
	if strings.Contains(body, `"status":"REWARDABLE"`) || strings.Contains(body, `"grantOrders"`) || strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected no rewardable/grant/outbox response on stock failure, got: %s", body)
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit", nil))
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"STOCK"`) {
		t.Fatalf("expected stock audit payload, got: %s", auditRec.Body.String())
	}
}

func TestListWorkflowRequirements(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify workflow endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/requirements", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"2026-05-24-test"`) {
		t.Fatalf("expected requirement id, got: %s", body)
	}
	if !strings.Contains(body, `"analysis":{"status":"WAITING"`) {
		t.Fatalf("expected analysis agent status, got: %s", body)
	}
}

func TestGetWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify workflow detail endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/requirements/2026-05-24-test", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"task.md"`) {
		t.Fatalf("expected task artifact, got: %s", body)
	}
	if !strings.Contains(body, `"agent-status.json"`) {
		t.Fatalf("expected status artifact, got: %s", body)
	}
}

func TestCreateAndIntakeWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := newTestHandler()

	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements", strings.NewReader(`{
		"title": "agent control",
		"description": "verify admin creates workflow requirements",
		"risk": "低"
	}`))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	reqHTTP = httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/intake", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ANALYSIS_READY"`) {
		t.Fatalf("expected intake status, got: %s", rec.Body.String())
	}
}

func TestDeleteWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	backlog := workflow.Backlog{Items: []workflow.Requirement{
		{ID: "req-a", Title: "A", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-b", Title: "B", Description: "keep me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
	}}
	if err := workflow.Save("requirements/backlog.json", backlog); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	reqHTTP := httptest.NewRequest(http.MethodDelete, "/admin/v1/workflow/requirements/req-a", nil)
	rec := httptest.NewRecorder()
	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	got, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "req-b" {
		t.Fatalf("expected only req-b, got: %+v", got.Items)
	}
}

func TestDeleteWorkflowRequirementsBatch(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	backlog := workflow.Backlog{Items: []workflow.Requirement{
		{ID: "req-a", Title: "A", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-b", Title: "B", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-c", Title: "C", Description: "keep me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
	}}
	if err := workflow.Save("requirements/backlog.json", backlog); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	reqHTTP := httptest.NewRequest(http.MethodDelete, "/admin/v1/workflow/requirements", strings.NewReader(`{"ids":["req-a","req-b"]}`))
	rec := httptest.NewRecorder()
	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	got, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "req-c" {
		t.Fatalf("expected only req-c, got: %+v", got.Items)
	}
}

func TestRunWorkflowAgent(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify run agent endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	command := `cat >/dev/null && cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
88
~~~

## Decision

~~~text
PASS
~~~
EOF`
	body := `{"agent":"analysis","command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-agent", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"NextStatus":"REVIEW_READY"`) {
		t.Fatalf("expected review ready, got: %s", rec.Body.String())
	}
}

func TestRunWorkflowPipelineStopsOnAnalysisBlock(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify run pipeline endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	command := `cat >/dev/null && cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
60
~~~

## Decision

~~~text
BLOCK
~~~
EOF`
	body := `{"command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-pipeline", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"REVIEW_BLOCK"`) {
		t.Fatalf("expected review block, got: %s", rec.Body.String())
	}
	report, err := os.ReadFile("tasks/2026-05-24-test/agent-report.md")
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(report), "最终状态: REVIEW_BLOCK") {
		t.Fatalf("expected report final status, got: %s", string(report))
	}
}

func quoteJSON(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(s) + `"`
}

func createActivity(t *testing.T, handler *Handler) string {
	t.Helper()
	body := `{
		"tenantId": "site_001",
		"activityCode": "FIRST_DEPOSIT_BONUS",
		"activityName": "First Deposit Bonus",
		"activityType": "FIRST_DEPOSIT",
		"status": "ACTIVE",
		"triggerEvents": ["DEPOSIT_SUCCESS"],
		"startTime": "2026-05-01T00:00:00Z",
		"endTime": "2026-06-01T00:00:00Z",
		"timezone": "UTC",
		"priority": 100,
		"exclusiveGroup": "deposit_bonus",
		"stackable": false,
		"rules": [
			{
				"ruleCode": "FIRST_DEPOSIT_AND_AMOUNT",
				"rulePhase": "QUALIFY",
				"ruleType": "CONDITION",
				"ruleConfig": {
					"logic": "AND",
					"conditions": [
						{"field": "attributes.isFirstDeposit", "op": "=", "value": true},
						{"field": "attributes.amount", "op": ">=", "value": "100.00"},
						{"field": "attributes.vipLevel", "op": "IN", "value": [3, 4, 5]}
					]
				},
				"sort": 1,
				"enabled": true
			}
		],
		"rewards": [
			{
				"rewardCode": "FIRST_DEPOSIT_RATIO",
				"rewardType": "BONUS",
				"calculateType": "RATIO",
				"rewardConfig": {
					"baseField": "attributes.amount",
					"rate": "0.20",
					"maxAmount": "500.00",
					"currency": "CNY",
					"flowMultiple": "3"
				},
				"grantMode": "AUTO",
				"grantChannel": "HTTP",
				"flowMultiple": "3",
				"enabled": true
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	return activity.ID
}

func createQuota(t *testing.T, handler *Handler, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, totalValue string) {
	t.Helper()
	body := `{
		"activityId": "` + activityID + `",
		"quotaType": "` + string(quotaType) + `",
		"scope": "` + string(scope) + `",
		"scopeKey": "` + scopeKey + `",
		"totalValue": "` + totalValue + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/quotas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
}

func postDepositEvent(t *testing.T, handler *Handler, eventID string, bizID string) *httptest.ResponseRecorder {
	t.Helper()
	event := `{
		"eventId": "` + eventID + `",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "` + bizID + `"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:"+bizID)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}
