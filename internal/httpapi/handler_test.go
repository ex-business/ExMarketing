package httpapi

import (
	"context"
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

func TestCreateActivityReturnsUnifiedConfigVersions(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	if activity.Version != 1 || len(activity.Rules) != 1 || activity.Rules[0].Version != 1 || len(activity.Rewards) != 1 || activity.Rewards[0].Version != 1 {
		t.Fatalf("expected activity/rule/reward version 1, got activity=%d rules=%v rewards=%v", activity.Version, activity.Rules, activity.Rewards)
	}
	if !strings.Contains(string(activity.Config), `"entry"`) {
		t.Fatalf("expected activity config to be stored, got: %s", string(activity.Config))
	}
	if !strings.Contains(string(activity.Rewards[0].Config), `"targetRef":"https://wallet.example.com/reward"`) {
		t.Fatalf("expected reward targetRef to be stored, got: %s", string(activity.Rewards[0].Config))
	}
}

func TestCreateActivityRejectsUnsupportedRewardDelivery(t *testing.T) {
	handler := newTestHandler()
	for name, body := range map[string]string{
		"missing targetRef": strings.Replace(validActivityBody(), `,
					"targetRef": "https://wallet.example.com/reward"`, "", 1),
		"mq channel":     strings.Replace(validActivityBody(), `"grantChannel": "HTTP"`, `"grantChannel": "MQ"`, 1),
		"manual mode":    strings.Replace(validActivityBody(), `"grantMode": "AUTO"`, `"grantMode": "MANUAL"`, 1),
		"unknown config": strings.Replace(validActivityBody(), `"targetRef": "https://wallet.example.com/reward"`, `"targetRef": "https://wallet.example.com/reward", "unknown": true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			rec := postActivity(t, handler, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateActivityRejectsInvalidConfigAndRuleSchema(t *testing.T) {
	handler := newTestHandler()
	for name, body := range map[string]string{
		"duplicate trigger": strings.Replace(validActivityBody(), `"triggerEvents": ["DEPOSIT_SUCCESS"]`, `"triggerEvents": ["DEPOSIT_SUCCESS", "DEPOSIT_SUCCESS"]`, 1),
		"unknown config":    strings.Replace(validActivityBody(), `"metadata": {"owner": "growth"}`, `"metadata": {"owner": "growth"}, "quota": {}`, 1),
		"bad rule op":       strings.Replace(validActivityBody(), `"op": ">="`, `"op": "BETWEEN"`, 1),
		"bad version":       strings.Replace(validActivityBody(), `"version": 1,`, `"version": 2,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			rec := postActivity(t, handler, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestActivityEventCreatesGrantCommand(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)

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

	var result marketing.ProcessEventResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Participations) != 1 || len(result.GrantOrders) != 1 {
		t.Fatalf("expected one participation and grant, got participations=%d grants=%d", len(result.Participations), len(result.GrantOrders))
	}
	expectedParticipationKey := "site_001:" + activityID + ":site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123"
	if result.Participations[0].IdempotencyKey != expectedParticipationKey {
		t.Fatalf("unexpected participation idempotency key: %s", result.Participations[0].IdempotencyKey)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(result.Participations[0].Snapshot, &snapshot); err != nil {
		t.Fatalf("decode participation snapshot: %v", err)
	}
	for _, key := range []string{"activity", "event", "ruleResult", "rewards"} {
		if _, ok := snapshot[key]; !ok {
			t.Fatalf("expected snapshot key %s, got: %s", key, string(result.Participations[0].Snapshot))
		}
	}
	var rewardPayload map[string]any
	if err := json.Unmarshal(result.GrantOrders[0].RewardPayload, &rewardPayload); err != nil {
		t.Fatalf("decode reward payload: %v", err)
	}
	for _, key := range []string{"rewardCode", "rewardType", "calculateType", "currency", "flowMultiple", "targetRef"} {
		if _, ok := rewardPayload[key]; !ok {
			t.Fatalf("expected reward payload key %s, got: %s", key, string(result.GrantOrders[0].RewardPayload))
		}
	}
}

func TestActivityEventRuleRejectionReturnsExplainWithoutGrant(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	rec := postRawEvent(t, handler, depositEventJSON("evt_rule_reject", "pay_rule_reject", `"amount":"299.00","vipLevel":3,"isFirstDeposit":false`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_rule_reject")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"status":"REJECTED"`,
		`"rejectReason":"CONDITION_NOT_MET: attributes.isFirstDeposit != true"`,
		`"ruleResults"`,
		`"stage":"QUALIFY"`,
		`"ruleCode":"FIRST_DEPOSIT_AND_AMOUNT"`,
		`"reasonCode":"CONDITION_NOT_MET"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in response, got: %s", want, body)
		}
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) {
		t.Fatalf("expected no grant/outbox on rule rejection, got: %s", body)
	}
}

func TestSimulateEventReturnsRuleExplainWithoutPersistence(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	event := depositEventJSON("evt_sim_reject", "pay_sim_reject", `"amount":"50.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ruleResults"`) || !strings.Contains(body, `"failedStage":"QUALIFY"`) || !strings.Contains(body, `"field":"attributes.amount"`) {
		t.Fatalf("expected simulation rule explain, got: %s", body)
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) {
		t.Fatalf("expected no simulated grants after rejection, got: %s", body)
	}
}

func TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")
	createQuota(t, handler, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "10001", "1")

	event := depositEventJSON("evt_sim_success", "pay_sim_success", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"matchedActivities":[{`,
		`"rewardPreviews":[{`,
		`"quotaImpactPreviews":[{`,
		`"quotaType":"BUDGET"`,
		`"availableBefore":"100.0000"`,
		`"delta":"59.8000"`,
		`"unlimited":false`,
		`"wouldExceed":false`,
		`"participationPreview"`,
		`"grantOrderPreviews":[{`,
		`"outboxCommandPreviews":[{`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in simulation response, got: %s", want, body)
		}
	}

	accepted := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_sim_success")
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("expected formal event after simulation to be accepted, got %d: %s", accepted.Code, accepted.Body.String())
	}
}

func TestSimulateEventQuotaExceededSuppressesGrantPreview(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1")

	event := depositEventJSON("evt_sim_quota", "pay_sim_quota", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"hasQuotaRisk":true`,
		`"decisionReasonCode":"QUOTA_EXCEEDED"`,
		`"wouldExceed":true`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in quota risk simulation, got: %s", want, body)
		}
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) || strings.Contains(body, `"grantOrderPreviews":[{`) || strings.Contains(body, `"outboxCommandPreviews":[{`) {
		t.Fatalf("expected no grant/outbox previews when quota would exceed, got: %s", body)
	}
}

func TestActivityEventRequiresIdempotencyKey(t *testing.T) {
	handler := newTestHandler()

	rec := postRawEvent(t, handler, depositEventJSON("evt_missing_idem", "pay_missing_idem", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`), "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency error code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsMismatchedIdempotencyKey(t *testing.T) {
	handler := newTestHandler()

	rec := postRawEvent(t, handler, depositEventJSON("evt_bad_idem", "pay_bad_idem", `"amount":"299.00"`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:other_pay")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency error code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsUnknownFields(t *testing.T) {
	handler := newTestHandler()

	event := `{
		"eventId": "evt_unknown",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_unknown"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {},
		"orderAmount": "299.00"
	}`
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_unknown")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
		t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsNestedUnknownFields(t *testing.T) {
	handler := newTestHandler()

	event := `{
		"eventId": "evt_nested_unknown",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001", "name": "Alice"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_nested_unknown"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {}
	}`
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_nested_unknown")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
		t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsMissingRequiredField(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name  string
		event string
		key   string
	}{
		{
			name: "missing occurredAt",
			event: `{
				"eventId": "evt_missing_required",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_required"},
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_required",
		},
		{
			name: "missing eventType",
			event: `{
				"eventId": "evt_missing_event_type",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_event_type"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_event_type",
		},
		{
			name: "missing biz",
			event: `{
				"eventId": "evt_missing_biz",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_biz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postRawEvent(t, handler, tt.event, tt.key)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
				t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
			}
		})
	}
}

func TestActivityEventDuplicateAndConflict(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	event := depositEventJSON("evt_duplicate", "pay_duplicate", `"amount":299.00,"currency":"CNY","vipLevel":3,"isFirstDeposit":true`)
	first := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected first status %d, got %d: %s", http.StatusAccepted, first.Code, first.Body.String())
	}

	reordered := `{
		"tenantId": "site_001",
		"sourceSystem": "payment",
		"eventType": "DEPOSIT_SUCCESS",
		"eventId": "evt_duplicate",
		"subject": {"id": "10001", "type": "USER"},
		"biz": {"bizId": "pay_duplicate", "bizType": "DEPOSIT"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"isFirstDeposit": true,
			"vipLevel": 3.0,
			"currency": "CNY",
			"amount": 299.000
		}
	}`
	second := postRawEvent(t, handler, reordered, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate 200 response, got %d: %s", second.Code, second.Body.String())
	}
	if strings.Contains(second.Body.String(), `"grantOrders":[{`) || strings.Contains(second.Body.String(), `"outboxMessages":[{`) {
		t.Fatalf("expected duplicate response without new grant/outbox, got: %s", second.Body.String())
	}

	conflict := depositEventJSON("evt_conflict", "pay_duplicate", `"amount":299.00,"currency":"CNY","vipLevel":3,"isFirstDeposit":true`)
	conflictRec := postRawEvent(t, handler, conflict, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if conflictRec.Code != http.StatusBadRequest {
		t.Fatalf("expected conflict status %d, got %d: %s", http.StatusBadRequest, conflictRec.Code, conflictRec.Body.String())
	}
	if !strings.Contains(conflictRec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency conflict code, got: %s", conflictRec.Body.String())
	}
}

func TestActivityEventNormalizesNullAttributes(t *testing.T) {
	handler := newTestHandler()

	event := depositEventJSON("evt_null_attrs", "pay_null_attrs", `null`)
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_null_attrs")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"attributes":{}`) {
		t.Fatalf("expected attributes to normalize to empty object, got: %s", rec.Body.String())
	}
}

func TestActivityEventNormalizesMissingAttributes(t *testing.T) {
	handler := newTestHandler()

	event := `{
		"eventId": "evt_missing_attrs",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_attrs"},
		"occurredAt": "2026-05-24T12:00:00Z"
	}`
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_attrs")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"attributes":{}`) {
		t.Fatalf("expected attributes to normalize to empty object, got: %s", rec.Body.String())
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

func TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	eventRec := postDepositEvent(t, handler, "evt_callback_success", "pay_callback_success")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var eventResult marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &eventResult); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := eventResult.GrantOrders[0].BillNo

	rec := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_001",
		"message": "paid",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected callback OK, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"grantStatus":"SUCCESS"`, `"participationStatus":"GRANTED"`, `"reservationStatus":"CONFIRMED"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected %s, got: %s", want, rec.Body.String())
		}
	}

	rec = postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_001",
		"message": "paid again",
		"finishedAt": "2026-05-24T12:06:00Z"
	}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate success callback, got %d: %s", rec.Code, rec.Body.String())
	}

	reconcile := httptest.NewRecorder()
	handler.Routes().ServeHTTP(reconcile, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=site_001&commandId="+commandID, nil))
	if reconcile.Code != http.StatusOK {
		t.Fatalf("expected reconciliation OK, got %d: %s", reconcile.Code, reconcile.Body.String())
	}
	for _, want := range []string{`"commandId":"` + commandID + `"`, `"externalGrantId":"wallet_001"`, `"grantStatus":"SUCCESS"`, `"reservationStatus":"CONFIRMED"`} {
		if !strings.Contains(reconcile.Body.String(), want) {
			t.Fatalf("expected reconciliation %s, got: %s", want, reconcile.Body.String())
		}
	}
}

func TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	eventRec := postDepositEvent(t, handler, "evt_callback_failed", "pay_callback_failed")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var eventResult marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &eventResult); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := eventResult.GrantOrders[0].BillNo

	rec := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "FAILED",
		"message": "wallet rejected",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected failed callback OK, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"grantStatus":"FAILED"`, `"participationStatus":"FAILED"`, `"reservationStatus":"RELEASED"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected %s, got: %s", want, rec.Body.String())
		}
	}

	conflict := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_late",
		"message": "late success",
		"finishedAt": "2026-05-24T12:06:00Z"
	}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"CALLBACK_STATUS_CONFLICT"`) {
		t.Fatalf("expected conflict response, got %d: %s", conflict.Code, conflict.Body.String())
	}

	unknown := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "cmd_unknown",
		"status": "FAILED",
		"message": "not found",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if unknown.Code != http.StatusNotFound || !strings.Contains(unknown.Body.String(), `"code":"GRANT_ORDER_NOT_FOUND"`) {
		t.Fatalf("expected unknown command response, got %d: %s", unknown.Code, unknown.Body.String())
	}
}

func TestOutboxDLQQueryReplayAndClose(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := NewHandler(engine.NewService(repo))
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	_, err := repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_1",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_1"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		ErrorMessage:  "target returned status 500",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}
	_, err = repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_other_tenant",
		TenantID:      "site_002",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_2"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create other tenant outbox: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/outbox/dlq?tenantId=site_001", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected DLQ list status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"outbox_dlq_1"`) || strings.Contains(rec.Body.String(), "outbox_dlq_other_tenant") {
		t.Fatalf("expected tenant-scoped DLQ list, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_1/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "retry after wallet recovery"
	}`))
	handler.Routes().ServeHTTP(rec, replayReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected replay status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"sendStatus":"PENDING"`) || !strings.Contains(rec.Body.String(), `"commandId":"cmd_1"`) {
		t.Fatalf("expected replay to keep payload and return PENDING, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	replayReq = httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_1/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "retry after wallet recovery"
	}`))
	handler.Routes().ServeHTTP(rec, replayReq)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected second replay conflict, got %d: %s", rec.Code, rec.Body.String())
	}

	_, err = repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_close",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_close"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create close outbox: %v", err)
	}
	rec = httptest.NewRecorder()
	closeReq := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_close/close", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "business rejected"
	}`))
	handler.Routes().ServeHTTP(rec, closeReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sendStatus":"DLQ_CLOSED"`) {
		t.Fatalf("expected close to DLQ_CLOSED, got %d: %s", rec.Code, rec.Body.String())
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit", nil))
	for _, want := range []string{`"action":"OUTBOX_DLQ_REPLAY"`, `"action":"OUTBOX_DLQ_CLOSE"`, `"actor":"ops"`} {
		if !strings.Contains(auditRec.Body.String(), want) {
			t.Fatalf("expected audit %s, got: %s", want, auditRec.Body.String())
		}
	}
}

func TestOutboxDLQOperationRequiresOperatorAndReason(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := NewHandler(engine.NewService(repo))
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	_, err := repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_missing_reason",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_1"}`),
		SendStatus:    marketing.OutboxDLQ,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_missing_reason/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": ""
	}`))
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected missing reason conflict, got %d: %s", rec.Code, rec.Body.String())
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

func TestListWorkflowTemplates(t *testing.T) {
	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/templates", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"marketing-platform-22"`) ||
		!strings.Contains(body, `"order":35`) ||
		!strings.Contains(body, "标准事件模型 Schema 校验") ||
		!strings.Contains(body, "管理后台通用配置页面") {
		t.Fatalf("expected marketing platform template, got: %s", body)
	}
}

func TestApplyWorkflowTemplateCreatesIntakeTasks(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/templates/marketing-platform-22/apply", strings.NewReader(`{"intake":true}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body struct {
		Created []workflow.Requirement `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Created) != len(workflow.MarketingPlatformTemplate().Items) {
		t.Fatalf("expected %d created requirements, got %d", len(workflow.MarketingPlatformTemplate().Items), len(body.Created))
	}
	if body.Created[0].Status != workflow.StatusAnalysisReady || body.Created[0].TaskDir == "" {
		t.Fatalf("expected intake-created task, got: %+v", body.Created[0])
	}
	if _, err := os.Stat(body.Created[0].TaskDir + "/agent-prompts.md"); err != nil {
		t.Fatalf("expected task files: %v", err)
	}

	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/templates/marketing-platform-22/apply", strings.NewReader(`{"intake":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected idempotent status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var second struct {
		Created []workflow.Requirement         `json:"created"`
		Skipped []workflow.RequirementTemplate `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if len(second.Created) != 0 || len(second.Skipped) != len(workflow.MarketingPlatformTemplate().Items) {
		t.Fatalf("expected duplicate titles skipped, got created=%d skipped=%d", len(second.Created), len(second.Skipped))
	}
}

func TestFilterAgentLogNoise(t *testing.T) {
	content := strings.Join([]string{
		"before",
		"2026-05-24T12:04:05.228825Z  WARN codex_core_skills::loader: ignoring interface.icon_small: icon path must not contain '..'",
		"2026-05-24T12:04:05.228829Z  WARN codex_core_skills::loader: ignoring interface.icon_large: icon path must not contain '..'",
		"after",
	}, "\n")
	got := filterAgentLogNoise(content)
	if strings.Contains(got, "codex_core_skills::loader") || strings.Contains(got, "interface.icon_large") || strings.Contains(got, "interface.icon_small") {
		t.Fatalf("expected noisy Codex skill icon warnings filtered, got: %s", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected useful log lines preserved, got: %s", got)
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

func TestRunWorkflowPipelineReturnsAnalysisBlockToAnalysis(t *testing.T) {
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
	body := `{"maxSteps":1,"command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-pipeline", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ANALYSIS_FAIL"`) {
		t.Fatalf("expected analysis fail, got: %s", rec.Body.String())
	}
	report, err := os.ReadFile("tasks/2026-05-24-test/agent-report.md")
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(report), "最终状态: ANALYSIS_FAIL") {
		t.Fatalf("expected report final status, got: %s", string(report))
	}
}

func quoteJSON(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(s) + `"`
}

func createActivity(t *testing.T, handler *Handler) string {
	t.Helper()
	body := validActivityBody()
	rec := postActivity(t, handler, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	return activity.ID
}

func TestValidatePrelaunchEndpointReturnsItemizedApprovalRequired(t *testing.T) {
	handler := newTestHandler()
	body := `{
		"actor": "operator",
		"targetAction": "GO_LIVE",
		"activity": ` + validActivityBody() + `
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/prelaunch-validation", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"conclusion":"APPROVAL_REQUIRED"`) {
		t.Fatalf("expected approval required result, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"allowGoLive":false`) {
		t.Fatalf("expected go-live blocked, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"items"`) || !strings.Contains(rec.Body.String(), `"highRiskItems"`) {
		t.Fatalf("expected itemized validation result, got: %s", rec.Body.String())
	}
}

func validActivityBody() string {
	return `{
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
		"version": 1,
		"config": {
			"entry": {
				"channels": ["APP", "WEB"],
				"entranceCodes": ["deposit_page"],
				"landingUrl": "https://example.com/promo/first-deposit"
			},
			"display": {
				"title": "First Deposit Bonus",
				"bannerUrl": "https://example.com/banner.png",
				"termsUrl": "https://example.com/terms",
				"visibleFrom": "2026-05-01T00:00:00Z",
				"visibleTo": "2026-06-01T00:00:00Z",
				"sort": 100
			},
			"audience": {
				"segmentCodes": ["new_user"],
				"tags": ["first_deposit"],
				"countries": ["CN"]
			},
			"metadata": {"owner": "growth"}
		},
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
				"enabled": true,
				"version": 1
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
					"flowMultiple": "3",
					"targetRef": "https://wallet.example.com/reward"
				},
				"grantMode": "AUTO",
				"grantChannel": "HTTP",
				"flowMultiple": "3",
				"enabled": true,
				"version": 1
			}
		]
	}`
}

func postActivity(t *testing.T, handler *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
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
	event := depositEventJSON(eventID, bizID, `{
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}`)
	return postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:"+bizID)
}

func depositEventJSON(eventID string, bizID string, attributesJSON string) string {
	if attributesJSON != "null" && !strings.HasPrefix(strings.TrimSpace(attributesJSON), "{") {
		attributesJSON = "{" + attributesJSON + "}"
	}
	return `{
		"eventId": "` + eventID + `",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "` + bizID + `"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": ` + attributesJSON + `
	}`
}

func postRawEvent(t *testing.T, handler *Handler, event string, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func postRewardCallback(t *testing.T, handler *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/reward/callback", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}
