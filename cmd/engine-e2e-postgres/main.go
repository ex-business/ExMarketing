package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/httpapi"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/postgres"
	"github.com/ex-stack/ExMarketing/internal/worker"
)

const (
	defaultTenantID = "site_001"
	rewardCode      = "E2E_PG_BONUS"
)

type runner struct {
	ctx       context.Context
	db        *sql.DB
	baseURL   string
	runID     string
	tenantID  string
	eventTime time.Time
	activity  activityResponse
	lastHTTP  *httpTrace
	snapshots []string
}

type httpTrace struct {
	Stage      string `json:"stage"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Request    any    `json:"request,omitempty"`
	StatusCode int    `json:"statusCode"`
	Response   string `json:"response"`
}

type activityResponse struct {
	ID       string `json:"id"`
	Code     string `json:"activityCode"`
	Status   string `json:"status"`
	Version  int    `json:"version"`
	Approval struct {
		Status string `json:"status"`
	} `json:"approval"`
}

type processEventResponse struct {
	Inbox struct {
		ID             string `json:"id"`
		IdempotencyKey string `json:"idempotencyKey"`
		ProcessStatus  string `json:"processStatus"`
	} `json:"inbox"`
	Participations []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"participations"`
	GrantOrders []struct {
		ID              string `json:"id"`
		BillNo          string `json:"billNo"`
		GrantStatus     string `json:"grantStatus"`
		ParticipationID string `json:"participationId"`
	} `json:"grantOrders"`
	OutboxMessages []struct {
		ID         string `json:"id"`
		SendStatus string `json:"sendStatus"`
		TargetRef  string `json:"targetRef"`
	} `json:"outboxMessages"`
	Duplicate bool `json:"duplicate"`
}

type callbackResponse struct {
	CommandID           string                       `json:"commandId"`
	GrantStatus         string                       `json:"grantStatus"`
	ParticipationStatus string                       `json:"participationStatus"`
	Duplicate           bool                         `json:"duplicate"`
	Reservations        []marketing.QuotaReservation `json:"quotaReservations"`
}

type reconciliationResponse struct {
	Items []marketing.RewardReconciliationItem `json:"items"`
}

type rewardStub struct {
	server   *httptest.Server
	mu       sync.Mutex
	commands map[string]marketing.RewardCommand
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("FAIL: %v", err)
	}
}

func run() error {
	databaseURL := strings.TrimSpace(os.Getenv("EX_MARKETING_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("EX_MARKETING_DATABASE_URL is required; PostgreSQL E2E cannot run against the in-memory repository")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL: %w", err)
	}

	repo, err := postgres.NewRepository(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL repository: %w", err)
	}
	defer repo.Close()

	stub := newRewardStub()
	defer stub.server.Close()

	service := engine.NewService(repo)
	handler := httpapi.NewHandlerWithConfigs(service, httpapi.OpenAPIEventConfig{MaxBodyBytes: 1 << 20}, httpapi.WebhookReceiverConfig{})
	apiServer := httptest.NewServer(handler.Routes())
	defer apiServer.Close()

	workerCtx, stopWorker := context.WithCancel(ctx)
	defer stopWorker()
	go worker.NewOutboxWorker(repo, slog.New(slog.NewTextHandler(io.Discard, nil))).Run(workerCtx)

	r := &runner{
		ctx:       ctx,
		db:        db,
		baseURL:   apiServer.URL,
		runID:     "e2e_pg_" + time.Now().UTC().Format("20060102_150405_000000000"),
		eventTime: time.Now().UTC().Truncate(time.Second),
	}
	r.tenantID = tenantIDForRun(r.runID)

	if err := r.createActivity(stub.server.URL + "/reward"); err != nil {
		return r.fail("CREATE_ACTIVITY", err)
	}
	if err := r.approveAndActivateActivity(); err != nil {
		return r.fail("APPROVE_AND_ACTIVATE", err)
	}
	if err := r.createQuotas(); err != nil {
		return r.fail("CREATE_QUOTA", err)
	}

	success, err := r.postEvent("POST_EVENT_SUCCESS_CASE", "success_user", "success")
	if err != nil {
		return r.fail("POST_EVENT_SUCCESS_CASE", err)
	}
	if err := r.assertDuplicateEvent("POST_EVENT_DUPLICATE", "success_user", "success"); err != nil {
		return r.fail("POST_EVENT_DUPLICATE", err)
	}
	if err := r.waitForOutboxSent(stub, success.GrantOrders[0].BillNo); err != nil {
		return r.fail("OUTBOX_SEND_SUCCESS_CASE", err)
	}
	if err := r.callback("CALLBACK_SUCCESS", success.GrantOrders[0].BillNo, marketing.GrantSuccess); err != nil {
		return r.fail("CALLBACK_SUCCESS", err)
	}
	if err := r.assertReconciliation("RECONCILIATION_SUCCESS", success.GrantOrders[0].BillNo, marketing.GrantSuccess); err != nil {
		return r.fail("RECONCILIATION_SUCCESS", err)
	}

	failed, err := r.postEvent("POST_EVENT_FAILED_CASE", "failed_user", "failed")
	if err != nil {
		return r.fail("POST_EVENT_FAILED_CASE", err)
	}
	if err := r.waitForOutboxSent(stub, failed.GrantOrders[0].BillNo); err != nil {
		return r.fail("OUTBOX_SEND_FAILED_CASE", err)
	}
	if err := r.callback("CALLBACK_FAILED", failed.GrantOrders[0].BillNo, marketing.GrantFailed); err != nil {
		return r.fail("CALLBACK_FAILED", err)
	}
	if err := r.assertReconciliation("RECONCILIATION_FAILED", failed.GrantOrders[0].BillNo, marketing.GrantFailed); err != nil {
		return r.fail("RECONCILIATION_FAILED", err)
	}
	if err := r.assertFinalQuota(); err != nil {
		return r.fail("ASSERT_FINAL_QUOTA", err)
	}

	fmt.Printf("PASS runId=%s tenantId=%s activityId=%s activityCode=%s successCommand=%s failedCommand=%s\n",
		r.runID, r.tenantID, r.activity.ID, r.activity.Code, success.GrantOrders[0].BillNo, failed.GrantOrders[0].BillNo)
	return nil
}

func newRewardStub() *rewardStub {
	stub := &rewardStub{commands: map[string]marketing.RewardCommand{}}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/reward" {
			http.NotFound(w, req)
			return
		}
		defer req.Body.Close()
		var command marketing.RewardCommand
		if err := json.NewDecoder(req.Body).Decode(&command); err != nil {
			http.Error(w, "invalid reward command", http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		stub.commands[command.CommandID] = command
		stub.mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	return stub
}

func tenantIDForRun(runID string) string {
	tenantID := defaultTenantID + "_" + runID
	if len(tenantID) <= 64 {
		return tenantID
	}
	return tenantID[:64]
}

func (s *rewardStub) received(commandID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.commands[commandID]
	return ok
}

func (r *runner) createActivity(targetRef string) error {
	now := time.Now().UTC()
	body := map[string]any{
		"operator":       "e2e-dev-agent",
		"tenantId":       r.tenantID,
		"activityCode":   r.runID,
		"activityName":   "PostgreSQL E2E " + r.runID,
		"activityType":   "DEPOSIT_BONUS",
		"status":         "DRAFT",
		"triggerEvents":  []string{"DEPOSIT_SUCCESS"},
		"startTime":      now.Add(-time.Hour).Format(time.RFC3339),
		"endTime":        now.Add(24 * time.Hour).Format(time.RFC3339),
		"timezone":       "UTC",
		"priority":       100,
		"exclusiveGroup": "",
		"stackable":      true,
		"config": map[string]any{
			"metadata": map[string]any{"runId": r.runID},
		},
		"rules": []map[string]any{{
			"ruleCode":  "FIRST_DEPOSIT_AMOUNT",
			"rulePhase": "QUALIFY",
			"ruleType":  "CONDITION",
			"ruleConfig": map[string]any{
				"logic": "AND",
				"conditions": []map[string]any{
					{"field": "attributes.isFirstDeposit", "op": "=", "value": true},
					{"field": "attributes.amount", "op": ">=", "value": "100.00"},
				},
			},
			"sort":    1,
			"enabled": true,
		}},
		"rewards": []map[string]any{{
			"rewardCode":    rewardCode,
			"rewardType":    "BONUS",
			"calculateType": "FIXED",
			"rewardConfig": map[string]any{
				"amount":       "10.00",
				"currency":     "USD",
				"flowMultiple": "1",
				"targetRef":    targetRef,
			},
			"grantMode":    "AUTO",
			"grantChannel": "HTTP",
			"enabled":      true,
			"version":      1,
		}},
	}
	return r.doJSON("CREATE_ACTIVITY", http.MethodPost, "/admin/v1/activities", body, http.StatusCreated, &r.activity)
}

func (r *runner) approveAndActivateActivity() error {
	if err := r.doJSON("SUBMIT_APPROVAL", http.MethodPost, "/admin/v1/activities/"+r.activity.ID+"/approval/submit", map[string]any{
		"operator":        "e2e-submit",
		"expectedVersion": r.activity.Version,
		"reason":          r.runID,
	}, http.StatusOK, &r.activity); err != nil {
		return err
	}
	if err := r.doJSON("APPROVE_ACTIVITY_L1", http.MethodPost, "/admin/v1/activities/"+r.activity.ID+"/approval/approve", map[string]any{
		"operator": "e2e-l1",
		"reason":   r.runID,
	}, http.StatusOK, &r.activity); err != nil {
		return err
	}
	if r.activity.Approval.Status != "APPROVED" {
		if err := r.doJSON("APPROVE_ACTIVITY_L2", http.MethodPost, "/admin/v1/activities/"+r.activity.ID+"/approval/approve", map[string]any{
			"operator": "e2e-l2",
			"reason":   r.runID,
		}, http.StatusOK, &r.activity); err != nil {
			return err
		}
	}
	if r.activity.Approval.Status != "APPROVED" {
		return fmt.Errorf("activity approval status = %s, want APPROVED", r.activity.Approval.Status)
	}
	return r.doJSON("ACTIVATE_ACTIVITY", http.MethodPut, "/admin/v1/activities/"+r.activity.ID, map[string]any{
		"operator": "e2e-activate",
		"status":   "ACTIVE",
	}, http.StatusOK, &r.activity)
}

func (r *runner) createQuotas() error {
	for _, quota := range []map[string]any{
		{"quotaType": "BUDGET", "scope": "ACTIVITY", "scopeKey": "TOTAL", "totalValue": "20.00"},
		{"quotaType": "STOCK", "scope": "ACTIVITY", "scopeKey": rewardCode, "totalValue": "2"},
		{"quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "success_user", "totalValue": "1"},
		{"quotaType": "USER_LIMIT", "scope": "USER", "scopeKey": "failed_user", "totalValue": "1"},
		{"quotaType": "DAILY_LIMIT", "scope": "USER", "scopeKey": "success_user:" + time.Now().UTC().Format("2006-01-02"), "totalValue": "1"},
		{"quotaType": "DAILY_LIMIT", "scope": "USER", "scopeKey": "failed_user:" + time.Now().UTC().Format("2006-01-02"), "totalValue": "1"},
	} {
		quota["operator"] = "e2e-dev-agent"
		quota["tenantId"] = r.tenantID
		quota["activityId"] = r.activity.ID
		var out map[string]any
		if err := r.doJSON("CREATE_QUOTA", http.MethodPost, "/admin/v1/quotas", quota, http.StatusCreated, &out); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) postEvent(stage string, subjectID string, suffix string) (processEventResponse, error) {
	event := r.event(subjectID, suffix)
	var result processEventResponse
	if err := r.doJSON(stage, http.MethodPost, "/openapi/v1/events", event, http.StatusAccepted, &result); err != nil {
		return result, err
	}
	if result.Duplicate {
		return result, errors.New("first event unexpectedly returned duplicate=true")
	}
	if len(result.Participations) != 1 || len(result.GrantOrders) != 1 || len(result.OutboxMessages) != 1 {
		return result, fmt.Errorf("expected 1 participation, 1 grant_order and 1 outbox, got %d/%d/%d", len(result.Participations), len(result.GrantOrders), len(result.OutboxMessages))
	}
	if result.Inbox.ProcessStatus != "PROCESSED" || result.Participations[0].Status != "REWARDABLE" || result.GrantOrders[0].GrantStatus != "PENDING" {
		return result, fmt.Errorf("unexpected event result statuses: inbox=%s participation=%s grant=%s", result.Inbox.ProcessStatus, result.Participations[0].Status, result.GrantOrders[0].GrantStatus)
	}
	return result, nil
}

func (r *runner) assertDuplicateEvent(stage string, subjectID string, suffix string) error {
	var result processEventResponse
	if err := r.doJSON(stage, http.MethodPost, "/openapi/v1/events", r.event(subjectID, suffix), http.StatusOK, &result); err != nil {
		return err
	}
	if !result.Duplicate || len(result.GrantOrders) != 0 || len(result.OutboxMessages) != 0 {
		return fmt.Errorf("duplicate event should not create grants/outbox: duplicate=%v grants=%d outbox=%d", result.Duplicate, len(result.GrantOrders), len(result.OutboxMessages))
	}
	return nil
}

func (r *runner) event(subjectID string, suffix string) map[string]any {
	return map[string]any{
		"eventId":      r.runID + "_" + suffix,
		"eventType":    "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId":     r.tenantID,
		"subject":      map[string]any{"type": "USER", "id": subjectID},
		"biz":          map[string]any{"bizType": "DEPOSIT", "bizId": r.runID + "_" + suffix},
		"occurredAt":   r.eventTime.Format(time.RFC3339),
		"attributes": map[string]any{
			"amount":         "100.00",
			"currency":       "USD",
			"isFirstDeposit": true,
		},
	}
}

func (r *runner) waitForOutboxSent(stub *rewardStub, commandID string) error {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if stub.received(commandID) {
			items, err := r.reconcile(commandID)
			if err == nil && len(items) == 1 && len(items[0].OutboxMessages) == 1 && items[0].OutboxMessages[0].SendStatus == marketing.OutboxSent {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("outbox did not reach SENT or reward stub did not receive commandID=%s", commandID)
}

func (r *runner) callback(stage string, commandID string, status marketing.GrantStatus) error {
	body := map[string]any{
		"tenantId":        r.tenantID,
		"commandId":       commandID,
		"status":          status,
		"message":         "e2e " + string(status),
		"finishedAt":      time.Now().UTC().Format(time.RFC3339),
		"externalGrantId": r.runID + "_external_" + strings.ToLower(string(status)),
	}
	if status == marketing.GrantFailed {
		delete(body, "externalGrantId")
	}
	var result callbackResponse
	if err := r.doJSON(stage, http.MethodPost, "/openapi/v1/reward/callback", body, http.StatusOK, &result); err != nil {
		return err
	}
	if result.GrantStatus != string(status) {
		return fmt.Errorf("callback grantStatus=%s, want %s", result.GrantStatus, status)
	}
	return nil
}

func (r *runner) assertReconciliation(stage string, commandID string, status marketing.GrantStatus) error {
	items, err := r.reconcile(commandID)
	if err != nil {
		return err
	}
	if len(items) != 1 {
		return fmt.Errorf("reconciliation returned %d items for commandID=%s", len(items), commandID)
	}
	item := items[0]
	if item.GrantStatus != status {
		return fmt.Errorf("reconciliation grantStatus=%s, want %s", item.GrantStatus, status)
	}
	if item.EventInbox == nil || item.ParticipationID == "" || len(item.OutboxMessages) != 1 || len(item.Reservations) != 4 {
		return fmt.Errorf("reconciliation missing chain objects: eventInbox=%v participation=%s outbox=%d reservations=%d", item.EventInbox != nil, item.ParticipationID, len(item.OutboxMessages), len(item.Reservations))
	}
	wantReservation := marketing.QuotaReservationConfirmed
	if status == marketing.GrantFailed {
		wantReservation = marketing.QuotaReservationReleased
	}
	for _, reservation := range item.Reservations {
		if reservation.Status != wantReservation {
			return fmt.Errorf("reservation %s/%s/%s status=%s, want %s", reservation.Type, reservation.Scope, reservation.ScopeKey, reservation.Status, wantReservation)
		}
	}
	_ = stage
	return nil
}

func (r *runner) reconcile(commandID string) ([]marketing.RewardReconciliationItem, error) {
	var response reconciliationResponse
	path := "/admin/v1/reward/reconciliation?tenantId=" + r.tenantID + "&commandId=" + commandID + "&limit=10"
	if err := r.doJSON("RECONCILIATION_QUERY", http.MethodGet, path, nil, http.StatusOK, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (r *runner) assertFinalQuota() error {
	rows, err := r.db.QueryContext(r.ctx, `
		SELECT quota_type, scope, scope_key, used_value::text, frozen_value::text
		FROM marketing_quota
		WHERE activity_id = $1
	`, r.activity.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	got := map[string][2]string{}
	for rows.Next() {
		var typ, scope, key, used, frozen string
		if err := rows.Scan(&typ, &scope, &key, &used, &frozen); err != nil {
			return err
		}
		got[typ+":"+scope+":"+key] = [2]string{used, frozen}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	want := map[string][2]string{
		"BUDGET:ACTIVITY:TOTAL":        {"10.0000", "0.0000"},
		"STOCK:ACTIVITY:" + rewardCode: {"1.0000", "0.0000"},
		"USER_LIMIT:USER:success_user": {"1.0000", "0.0000"},
		"USER_LIMIT:USER:failed_user":  {"0.0000", "0.0000"},
		"DAILY_LIMIT:USER:success_user:" + time.Now().UTC().Format("2006-01-02"): {"1.0000", "0.0000"},
		"DAILY_LIMIT:USER:failed_user:" + time.Now().UTC().Format("2006-01-02"):  {"0.0000", "0.0000"},
	}
	for key, expected := range want {
		actual, ok := got[key]
		if !ok {
			return fmt.Errorf("quota %s missing; got keys=%v", key, got)
		}
		if actual != expected {
			return fmt.Errorf("quota %s used/frozen=%v, want %v", key, actual, expected)
		}
	}
	return nil
}

func (r *runner) doJSON(stage string, method string, path string, body any, wantStatus int, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(r.ctx, method, r.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	if method == http.MethodPost && strings.HasPrefix(path, "/openapi/v1/events") {
		req.Header.Set("Idempotency-Key", idempotencyKey(body))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.lastHTTP = &httpTrace{Stage: stage, Method: method, URL: path, Request: body, Response: err.Error()}
		return err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	r.lastHTTP = &httpTrace{Stage: stage, Method: method, URL: path, Request: body, StatusCode: resp.StatusCode, Response: string(responseBody)}
	if resp.StatusCode != wantStatus {
		return fmt.Errorf("%s %s returned status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, string(responseBody))
	}
	if out != nil {
		decoder := json.NewDecoder(bytes.NewReader(responseBody))
		if err := decoder.Decode(out); err != nil {
			return fmt.Errorf("decode %s response: %w body=%s", stage, err, string(responseBody))
		}
	}
	return nil
}

func idempotencyKey(body any) string {
	event, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	biz, _ := event["biz"].(map[string]any)
	return strings.Join([]string{
		fmt.Sprint(event["tenantId"]),
		fmt.Sprint(event["sourceSystem"]),
		fmt.Sprint(event["eventType"]),
		fmt.Sprint(biz["bizType"]),
		fmt.Sprint(biz["bizId"]),
	}, ":")
}

func (r *runner) fail(stage string, err error) error {
	diagnostics := map[string]any{
		"stage":    stage,
		"error":    err.Error(),
		"runId":    r.runID,
		"tenantId": r.tenantID,
		"env": map[string]any{
			"databaseURLSet": os.Getenv("EX_MARKETING_DATABASE_URL") != "",
			"serviceURL":     r.baseURL,
			"repository":     "postgres",
			"outboxWorker":   "enabled",
		},
		"lastHTTP": r.lastHTTP,
		"database": r.databaseSnapshot(),
	}
	encoded, _ := json.MarshalIndent(diagnostics, "", "  ")
	_, _ = fmt.Fprintln(os.Stderr, string(encoded))
	return fmt.Errorf("%s: %w", stage, err)
}

func (r *runner) databaseSnapshot() map[string]json.RawMessage {
	activityID := r.activity.ID
	out := map[string]json.RawMessage{}
	queries := map[string]struct {
		sql  string
		args []any
	}{
		"activity": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT id, tenant_id, activity_code, status, version, created_at, updated_at FROM marketing_activity WHERE id = $1 OR activity_code = $2 ORDER BY created_at) t`,
			args: []any{activityID, r.runID},
		},
		"quota": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT quota_type, scope, scope_key, total_value::text, used_value::text, frozen_value::text, version FROM marketing_quota WHERE activity_id = $1 ORDER BY quota_type, scope_key) t`,
			args: []any{activityID},
		},
		"inbox": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT id, event_id, idempotency_key, process_status, error_message FROM marketing_event_inbox WHERE tenant_id = $1 AND event_id LIKE $2 || '%' ORDER BY created_at) t`,
			args: []any{r.tenantID, r.runID},
		},
		"participation": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT id, event_inbox_id, subject_id, biz_id, status, reject_reason FROM marketing_participation WHERE activity_id = $1 ORDER BY created_at) t`,
			args: []any{activityID},
		},
		"grant_order": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT id, participation_id, subject_id, bill_no, grant_status, reward_amount::text, error_message FROM marketing_grant_order WHERE activity_id = $1 ORDER BY created_at) t`,
			args: []any{activityID},
		},
		"outbox": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT id, send_status, retry_count, error_message, payload_json FROM marketing_outbox WHERE tenant_id = $1 AND payload_json::text LIKE '%' || $2 || '%' ORDER BY created_at) t`,
			args: []any{r.tenantID, r.runID},
		},
		"reservation": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT grant_order_id, command_id, quota_type, scope, scope_key, reserved_amount::text, reservation_status FROM marketing_quota_reservation WHERE activity_id = $1 ORDER BY created_at) t`,
			args: []any{activityID},
		},
		"audit": {
			sql:  `SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json) FROM (SELECT action, resource, resource_id, payload_json FROM marketing_audit WHERE tenant_id = $1 AND (resource_id = $2 OR payload_json::text LIKE '%' || $3 || '%') ORDER BY created_at LIMIT 50) t`,
			args: []any{r.tenantID, activityID, r.runID},
		},
	}
	for name, query := range queries {
		var raw json.RawMessage
		if err := r.db.QueryRowContext(r.ctx, query.sql, query.args...).Scan(&raw); err != nil {
			out[name] = json.RawMessage(fmt.Sprintf("%q", err.Error()))
			continue
		}
		out[name] = raw
	}
	return out
}
