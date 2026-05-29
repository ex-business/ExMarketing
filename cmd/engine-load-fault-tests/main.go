package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/httpapi"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/postgres"
	"github.com/ex-stack/ExMarketing/internal/worker"
)

const (
	defaultTenantPrefix = "site_001_load"
	rewardCode          = "LOAD_FAULT_BONUS"
)

type options struct {
	profile  string
	scenario string
	runID    string
	seed     int64
}

type runner struct {
	ctx        context.Context
	db         *sql.DB
	repo       marketing.Repository
	baseURL    string
	runID      string
	seed       int64
	profile    string
	scenario   string
	stub       *rewardStub
	lastHTTP   *httpTrace
	startedAt  time.Time
	logPath    string
	scenarios  []scenarioResult
	httpClient *http.Client
}

type scenarioResult struct {
	Name              string         `json:"name"`
	Result            string         `json:"result"`
	Total             int            `json:"total"`
	Success           int            `json:"success"`
	Duplicate         int            `json:"duplicate"`
	ExpectedReject    int            `json:"expected_reject"`
	UnexpectedError   int            `json:"unexpected_error"`
	Timeout           int            `json:"timeout"`
	Retry             int            `json:"retry"`
	DLQ               int            `json:"dlq"`
	EventsPerSecond   float64        `json:"events_per_second"`
	OutboxPerSecond   float64        `json:"outbox_per_second"`
	CallbackPerSecond float64        `json:"callbacks_per_second"`
	Latency           latencySummary `json:"latency"`
	Consistency       map[string]any `json:"consistency"`
	Details           map[string]any `json:"details,omitempty"`
}

type latencySummary struct {
	P50MS int64 `json:"p50_ms"`
	P95MS int64 `json:"p95_ms"`
	P99MS int64 `json:"p99_ms"`
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
	CommandID   string `json:"commandId"`
	GrantStatus string `json:"grantStatus"`
	Duplicate   bool   `json:"duplicate"`
}

type rewardStub struct {
	server       *httptest.Server
	failFirst    int32
	permanent400 int32
	mu           sync.Mutex
	attempts     map[string]int
	commands     map[string]marketing.RewardCommand
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	opts := parseOptions()
	if opts.profile != "local" {
		return fmt.Errorf("unsupported profile %q; only local is implemented", opts.profile)
	}
	databaseURL := strings.TrimSpace(os.Getenv("EX_MARKETING_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("EX_MARKETING_DATABASE_URL is required; load/fault tests must use a real PostgreSQL repository")
	}
	if opts.runID == "" {
		now := time.Now().UTC()
		opts.runID = fmt.Sprintf("load_fault_%s_%09d", now.Format("20060102_150405"), now.Nanosecond())
	}
	if opts.seed == 0 {
		opts.seed = time.Now().UTC().UnixNano()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect PostgreSQL %s: %w", redactDSN(databaseURL), err)
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
		ctx:        ctx,
		db:         db,
		repo:       repo,
		baseURL:    apiServer.URL,
		runID:      opts.runID,
		seed:       opts.seed,
		profile:    opts.profile,
		scenario:   opts.scenario,
		stub:       stub,
		startedAt:  time.Now().UTC(),
		logPath:    "tasks/2026-05-27-engine-load-fault-tests/runs/" + opts.runID + "-load-fault.log",
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
	_ = os.MkdirAll("tasks/2026-05-27-engine-load-fault-tests/runs", 0o755)
	fmt.Printf("START runId=%s seed=%d profile=%s scenario=%s repository=postgres dsn=%s baseUrl=%s log=%s\n",
		r.runID, r.seed, r.profile, opts.scenario, redactDSN(databaseURL), firstNonEmpty(os.Getenv("EX_MARKETING_BASE_URL"), r.baseURL), r.logPath)

	switch opts.scenario {
	case "pg-smoke":
		if err := r.scenarioPGSmoke(); err != nil {
			return r.fail("pg-smoke", err)
		}
	case "all":
		for _, scenario := range []struct {
			name string
			fn   func() error
		}{
			{"concurrent-events", r.scenarioConcurrentEvents},
			{"quota-contention", r.scenarioQuotaContention},
			{"duplicate-callback", r.scenarioDuplicateAndCallback},
			{"outbox-retry-dlq", r.scenarioOutboxRetryDLQ},
			{"postgres-timeout-fault", r.scenarioPostgresTimeoutFault},
		} {
			if err := scenario.fn(); err != nil {
				return r.fail(scenario.name, err)
			}
		}
	default:
		return fmt.Errorf("unsupported scenario %q; use pg-smoke or all", opts.scenario)
	}

	summary := map[string]any{
		"result":     "PASS",
		"runId":      r.runID,
		"seed":       r.seed,
		"profile":    r.profile,
		"startedAt":  r.startedAt.Format(time.RFC3339Nano),
		"finishedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"logPath":    r.logPath,
		"reproduce":  reproduceCommand(r.runID, r.seed, opts.scenario),
		"scenarios":  r.scenarios,
	}
	r.writeLog(summary)
	writeJSONLine("SUMMARY", summary)
	fmt.Printf("PASS runId=%s seed=%d profile=%s scenario=%s log=%s\n", r.runID, r.seed, r.profile, opts.scenario, r.logPath)
	return nil
}

func parseOptions() options {
	var opts options
	flag.StringVar(&opts.profile, "profile", "local", "profile")
	flag.StringVar(&opts.scenario, "scenario", "all", "scenario")
	flag.StringVar(&opts.runID, "run-id", "", "run id")
	flag.Int64Var(&opts.seed, "seed", 0, "random seed")
	flag.Parse()
	return opts
}

func newRewardStub() *rewardStub {
	stub := &rewardStub{attempts: map[string]int{}, commands: map[string]marketing.RewardCommand{}}
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
		stub.attempts[command.CommandID]++
		attempt := stub.attempts[command.CommandID]
		stub.commands[command.CommandID] = command
		stub.mu.Unlock()
		if atomic.LoadInt32(&stub.permanent400) == 1 {
			http.Error(w, "permanent failure", http.StatusBadRequest)
			return
		}
		if n := int(atomic.LoadInt32(&stub.failFirst)); n > 0 && attempt <= n {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	return stub
}

func (r *runner) scenarioPGSmoke() error {
	activity, err := r.createActivity("pg_smoke", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	subjects := makeSubjects("smoke_user_", 6)
	if err := r.createQuotasForSubjects(activity.ID, subjects, "1000", "100", "1"); err != nil {
		return err
	}
	res, latencies, err := r.postEvents(activity, "pg_smoke", 6, 3, func(i int) string { return subjects[i] })
	if err != nil {
		return err
	}
	if err := r.waitOutbox(activity.ID, 6, 25*time.Second); err != nil {
		return err
	}
	if err := r.callbackAll(res.commands, marketing.GrantSuccess); err != nil {
		return err
	}
	consistency, err := r.consistency(activity.ID)
	if err != nil {
		return err
	}
	r.record(scenarioResult{Name: "pg-smoke", Result: "PASS", Total: 6, Success: 6, EventsPerSecond: rate(6, res.duration), OutboxPerSecond: rate(6, res.duration), CallbackPerSecond: rate(6, res.duration), Latency: summarize(latencies), Consistency: consistency})
	return nil
}

func (r *runner) scenarioConcurrentEvents() error {
	activity, err := r.createActivity("s1", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	subjects := makeSubjects("s1_user_", 500)
	if err := r.createQuotasForSubjects(activity.ID, subjects, "10000", "500", "1"); err != nil {
		return err
	}
	concurrency := resolvedConcurrency(25)
	res, latencies, err := r.postEvents(activity, "s1", 500, concurrency, func(i int) string { return subjects[i] })
	if err != nil {
		return err
	}
	if err := r.waitOutbox(activity.ID, res.success, 45*time.Second); err != nil {
		return err
	}
	if err := r.callbackAll(res.commands, marketing.GrantSuccess); err != nil {
		return err
	}
	consistency, err := r.consistency(activity.ID)
	if err != nil {
		return err
	}
	r.record(scenarioResult{Name: "S1_concurrent_events", Result: "PASS", Total: 500, Success: res.success, UnexpectedError: res.unexpected, EventsPerSecond: rate(500, res.duration), OutboxPerSecond: rate(res.success, res.duration), CallbackPerSecond: rate(res.success, res.duration), Latency: summarize(latencies), Consistency: consistency, Details: map[string]any{"target_concurrency": 25, "executed_concurrency": concurrency}})
	return nil
}

func (r *runner) scenarioQuotaContention() error {
	var totalSuccess, totalReject int
	details := map[string]any{}
	for _, qt := range []marketing.QuotaType{marketing.QuotaBudget, marketing.QuotaStock, marketing.QuotaUserLimit, marketing.QuotaDailyLimit} {
		activity, err := r.createActivity("quota_"+strings.ToLower(string(qt)), r.stub.server.URL+"/reward")
		if err != nil {
			return err
		}
		subject := "quota_user_" + strings.ToLower(string(qt))
		switch qt {
		case marketing.QuotaBudget:
			if err := r.createQuota(activity.ID, qt, marketing.QuotaScopeActivity, "TOTAL", "500"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaStock, marketing.QuotaScopeActivity, rewardCode, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, subject, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, dailyScopeKey(subject), "200"); err != nil {
				return err
			}
		case marketing.QuotaStock:
			if err := r.createQuota(activity.ID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10000"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, qt, marketing.QuotaScopeActivity, rewardCode, "50"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, subject, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, dailyScopeKey(subject), "200"); err != nil {
				return err
			}
		case marketing.QuotaUserLimit:
			if err := r.createQuota(activity.ID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10000"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaStock, marketing.QuotaScopeActivity, rewardCode, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, qt, marketing.QuotaScopeUser, subject, "50"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, dailyScopeKey(subject), "200"); err != nil {
				return err
			}
		case marketing.QuotaDailyLimit:
			if err := r.createQuota(activity.ID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "10000"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaStock, marketing.QuotaScopeActivity, rewardCode, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, subject, "200"); err != nil {
				return err
			}
			if err := r.createQuota(activity.ID, qt, marketing.QuotaScopeUser, dailyScopeKey(subject), "50"); err != nil {
				return err
			}
		}
		concurrency := resolvedConcurrency(50)
		res, latencies, err := r.postEvents(activity, "quota_"+string(qt), 200, concurrency, func(int) string { return subject })
		if err != nil {
			return err
		}
		if res.success != 50 || res.expectedReject != 150 {
			return fmt.Errorf("%s success/reject=%d/%d, want 50/150", qt, res.success, res.expectedReject)
		}
		consistency, err := r.consistency(activity.ID)
		if err != nil {
			return err
		}
		details[string(qt)] = map[string]any{"total": 200, "success": res.success, "expected_reject": res.expectedReject, "target_concurrency": 50, "executed_concurrency": concurrency, "latency": summarize(latencies), "consistency": consistency}
		totalSuccess += res.success
		totalReject += res.expectedReject
	}
	r.record(scenarioResult{Name: "S2_quota_contention", Result: "PASS", Total: 800, Success: totalSuccess, ExpectedReject: totalReject, Details: details, Consistency: map[string]any{"quotaTypes": []string{"BUDGET", "STOCK", "USER_LIMIT", "DAILY_LIMIT"}}})
	return nil
}

func (r *runner) scenarioDuplicateAndCallback() error {
	activity, err := r.createActivity("s3_s4", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	subjects := makeSubjects("dupcallback_user_", 30)
	if err := r.createQuotasForSubjects(activity.ID, subjects, "1000", "30", "1"); err != nil {
		return err
	}
	res, _, err := r.postEvents(activity, "dupcallback", 30, resolvedConcurrency(10), func(i int) string { return subjects[i] })
	if err != nil {
		return err
	}
	if err := r.waitOutbox(activity.ID, res.success, 30*time.Second); err != nil {
		return err
	}
	dupe, err := r.postEvent(activity, "dupcallback", 0, subjects[0])
	if err != nil {
		return err
	}
	if !dupe.Duplicate {
		return errors.New("duplicate event was not reported as duplicate")
	}
	for i, cmd := range res.commands {
		status := marketing.GrantSuccess
		if i%2 == 1 {
			status = marketing.GrantFailed
		}
		if err := r.callback(cmd, status, http.StatusOK); err != nil {
			return err
		}
		if err := r.callback(cmd, status, http.StatusOK); err != nil {
			return err
		}
		opposite := marketing.GrantFailed
		if status == marketing.GrantFailed {
			opposite = marketing.GrantSuccess
		}
		if err := r.callback(cmd, opposite, http.StatusConflict); err != nil {
			return err
		}
	}
	if err := r.callback("unknown_command_"+r.runID, marketing.GrantSuccess, http.StatusNotFound); err != nil {
		return err
	}
	consistency, err := r.consistency(activity.ID)
	if err != nil {
		return err
	}
	r.record(scenarioResult{Name: "S3_S4_duplicate_callback_ordering", Result: "PASS", Total: 122, Success: 30, Duplicate: 31, ExpectedReject: 31, CallbackPerSecond: 30, Consistency: consistency, Details: map[string]any{"unknownCommand": "GRANT_ORDER_NOT_FOUND", "oppositeTerminalState": "CALLBACK_STATUS_CONFLICT"}})
	return nil
}

func (r *runner) scenarioOutboxRetryDLQ() error {
	atomic.StoreInt32(&r.stub.failFirst, 1)
	activity, err := r.createActivity("s5_retry", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	subjects := makeSubjects("retry_user_", 50)
	if err := r.createQuotasForSubjects(activity.ID, subjects, "1000", "50", "1"); err != nil {
		return err
	}
	res, _, err := r.postEvents(activity, "retry", 50, resolvedConcurrency(10), func(i int) string { return subjects[i] })
	if err != nil {
		return err
	}
	if err := r.waitOutboxRetry(activity.ID, res.success, 25*time.Second); err != nil {
		return err
	}
	atomic.StoreInt32(&r.stub.failFirst, 0)
	if err := r.forceOutboxRetryNow(activity.ID); err != nil {
		return err
	}
	if err := r.waitOutbox(activity.ID, res.success, 45*time.Second); err != nil {
		return err
	}
	retries, dlq, err := r.outboxRetryDLQCounts(activity.ID)
	if err != nil {
		return err
	}
	if retries < 50 {
		return fmt.Errorf("retry count=%d, want at least 50", retries)
	}
	atomic.StoreInt32(&r.stub.permanent400, 1)
	dlqActivity, err := r.createActivity("s5_dlq", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	dlqSubjects := makeSubjects("dlq_user_", 3)
	if err := r.createQuotasForSubjects(dlqActivity.ID, dlqSubjects, "1000", "3", "1"); err != nil {
		return err
	}
	if _, _, err := r.postEvents(dlqActivity, "dlq", 3, resolvedConcurrency(3), func(i int) string { return dlqSubjects[i] }); err != nil {
		return err
	}
	if err := r.waitDLQ(dlqActivity.ID, 3, 20*time.Second); err != nil {
		return err
	}
	atomic.StoreInt32(&r.stub.permanent400, 0)
	_, dlq2, err := r.outboxRetryDLQCounts(dlqActivity.ID)
	if err != nil {
		return err
	}
	consistency, err := r.consistency(activity.ID)
	if err != nil {
		return err
	}
	r.record(scenarioResult{Name: "S5_outbox_retry_dlq", Result: "PASS", Total: 53, Success: 50, Retry: retries, DLQ: dlq + dlq2, Consistency: consistency})
	return nil
}

func (r *runner) scenarioPostgresTimeoutFault() error {
	activity, err := r.createActivity("s7_pg_fault", r.stub.server.URL+"/reward")
	if err != nil {
		return err
	}
	subjects := makeSubjects("pgfault_user_", 100)
	if err := r.createQuotasForSubjects(activity.ID, subjects, "5000", "100", "1"); err != nil {
		return err
	}
	res, latencies, err := r.postEvents(activity, "pgfault", 100, resolvedConcurrency(25), func(i int) string { return subjects[i] })
	if err != nil {
		return err
	}
	faultCtx, cancel := context.WithTimeout(r.ctx, time.Nanosecond)
	defer cancel()
	var faultErr string
	if _, err := r.db.ExecContext(faultCtx, "SELECT pg_sleep(0.01)"); err != nil {
		faultErr = err.Error()
	}
	if faultErr == "" {
		return errors.New("failed to inject PostgreSQL timeout fault")
	}
	if err := r.waitOutbox(activity.ID, res.success, 35*time.Second); err != nil {
		return err
	}
	if err := r.callbackAll(res.commands, marketing.GrantSuccess); err != nil {
		return err
	}
	consistency, err := r.consistency(activity.ID)
	if err != nil {
		return err
	}
	consistency["fault"] = "context timeout via pg_sleep"
	consistency["faultError"] = faultErr
	r.record(scenarioResult{Name: "S7_postgres_timeout_fault", Result: "PASS", Total: 100, Success: res.success, EventsPerSecond: rate(100, res.duration), Latency: summarize(latencies), Consistency: consistency})
	return nil
}

type postResult struct {
	success        int
	expectedReject int
	unexpected     int
	commands       []string
	duration       time.Duration
}

func (r *runner) postEvents(activity activityResponse, prefix string, total int, concurrency int, subject func(int) string) (postResult, []time.Duration, error) {
	start := time.Now()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var latencies []time.Duration
	var out postResult
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				callStart := time.Now()
				result, err := r.postEvent(activity, prefix, i, subject(i))
				elapsed := time.Since(callStart)
				mu.Lock()
				latencies = append(latencies, elapsed)
				switch {
				case err == nil:
					out.success++
					if len(result.GrantOrders) > 0 {
						out.commands = append(out.commands, result.GrantOrders[0].BillNo)
					}
				case strings.Contains(err.Error(), "QUOTA_EXCEEDED"):
					out.expectedReject++
				default:
					out.unexpected++
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	out.duration = time.Since(start)
	if out.unexpected > 0 {
		return out, latencies, fmt.Errorf("%s unexpected errors=%d", prefix, out.unexpected)
	}
	return out, latencies, nil
}

func (r *runner) postEvent(activity activityResponse, prefix string, i int, subjectID string) (processEventResponse, error) {
	event := map[string]any{
		"eventId":      fmt.Sprintf("%s_%s_%03d", r.runID, prefix, i),
		"eventType":    "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId":     tenantID(r.runID),
		"subject":      map[string]any{"type": "USER", "id": subjectID},
		"biz":          map[string]any{"bizType": "DEPOSIT", "bizId": fmt.Sprintf("%s_%s_%03d", r.runID, prefix, i)},
		"occurredAt":   r.startedAt.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
		"attributes": map[string]any{
			"amount":         "100.00",
			"currency":       "USD",
			"isFirstDeposit": true,
			"activityCode":   activity.Code,
		},
	}
	var result processEventResponse
	status, body, err := r.doJSON("POST_EVENT", http.MethodPost, "/openapi/v1/events", event, &result)
	if err != nil {
		return result, err
	}
	if status == http.StatusAccepted || status == http.StatusOK {
		return result, nil
	}
	return result, fmt.Errorf("POST_EVENT status=%d body=%s", status, body)
}

func (r *runner) createActivity(suffix string, targetRef string) (activityResponse, error) {
	now := time.Now().UTC()
	code := r.runID + "_" + suffix + "_" + randomSuffix(r.seed)
	body := map[string]any{
		"operator":       "engine-load-fault-tests",
		"tenantId":       tenantID(r.runID),
		"activityCode":   code,
		"activityName":   "Load Fault " + code,
		"activityType":   "DEPOSIT_BONUS",
		"status":         "DRAFT",
		"triggerEvents":  []string{"DEPOSIT_SUCCESS"},
		"startTime":      now.Add(-time.Hour).Format(time.RFC3339),
		"endTime":        now.Add(24 * time.Hour).Format(time.RFC3339),
		"timezone":       "UTC",
		"priority":       100,
		"exclusiveGroup": "",
		"stackable":      true,
		"config":         map[string]any{"metadata": map[string]any{"runId": r.runID, "scenario": suffix}},
		"rules": []map[string]any{{
			"ruleCode": "LOAD_FAULT_DEPOSIT", "rulePhase": "QUALIFY", "ruleType": "CONDITION",
			"ruleConfig": map[string]any{"logic": "AND", "conditions": []map[string]any{{"field": "attributes.isFirstDeposit", "op": "=", "value": true}, {"field": "attributes.amount", "op": ">=", "value": "100.00"}, {"field": "attributes.activityCode", "op": "=", "value": code}}},
			"sort":       1, "enabled": true,
		}},
		"rewards": []map[string]any{{
			"rewardCode": rewardCode, "rewardType": "BONUS", "calculateType": "FIXED",
			"rewardConfig": map[string]any{"amount": "10.00", "currency": "USD", "flowMultiple": "1", "targetRef": targetRef},
			"grantMode":    "AUTO", "grantChannel": "HTTP", "enabled": true, "version": 1,
		}},
	}
	var activity activityResponse
	status, response, err := r.doJSON("CREATE_ACTIVITY", http.MethodPost, "/admin/v1/activities", body, &activity)
	if err != nil {
		return activity, err
	}
	if status != http.StatusCreated {
		return activity, fmt.Errorf("create activity status=%d body=%s", status, response)
	}
	if err := r.approveAndActivate(&activity); err != nil {
		return activity, err
	}
	return activity, nil
}

func (r *runner) approveAndActivate(activity *activityResponse) error {
	for _, step := range []struct {
		path string
		body map[string]any
	}{
		{"/admin/v1/activities/" + activity.ID + "/approval/submit", map[string]any{"operator": "load-submit", "expectedVersion": activity.Version, "reason": r.runID}},
		{"/admin/v1/activities/" + activity.ID + "/approval/approve", map[string]any{"operator": "load-l1", "reason": r.runID}},
		{"/admin/v1/activities/" + activity.ID + "/approval/approve", map[string]any{"operator": "load-l2", "reason": r.runID}},
	} {
		status, _, err := r.doJSON("APPROVAL", http.MethodPost, step.path, step.body, activity)
		if err != nil {
			return err
		}
		if status != http.StatusOK && activity.Approval.Status != "APPROVED" {
			return fmt.Errorf("approval %s status=%d", step.path, status)
		}
		if activity.Approval.Status == "APPROVED" {
			break
		}
	}
	status, response, err := r.doJSON("ACTIVATE_ACTIVITY", http.MethodPut, "/admin/v1/activities/"+activity.ID, map[string]any{"operator": "load-activate", "status": "ACTIVE"}, activity)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("activate activity status=%d body=%s", status, response)
	}
	return nil
}

func (r *runner) createQuota(activityID string, qt marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, total string) error {
	body := map[string]any{"operator": "engine-load-fault-tests", "tenantId": tenantID(r.runID), "activityId": activityID, "quotaType": qt, "scope": scope, "scopeKey": scopeKey, "totalValue": total}
	var out map[string]any
	status, response, err := r.doJSON("CREATE_QUOTA", http.MethodPost, "/admin/v1/quotas", body, &out)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("create quota status=%d body=%s", status, response)
	}
	return nil
}

func (r *runner) createQuotasForSubjects(activityID string, subjects []string, budgetTotal string, stockTotal string, perSubjectTotal string) error {
	if err := r.createQuotaFast(activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", budgetTotal); err != nil {
		return err
	}
	if err := r.createQuotaFast(activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, rewardCode, stockTotal); err != nil {
		return err
	}
	for _, subject := range subjects {
		if err := r.createQuotaFast(activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, subject, perSubjectTotal); err != nil {
			return err
		}
		if err := r.createQuotaFast(activityID, marketing.QuotaDailyLimit, marketing.QuotaScopeUser, dailyScopeKey(subject), perSubjectTotal); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) createQuotaFast(activityID string, qt marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, total string) error {
	now := time.Now().UTC()
	_, err := r.repo.CreateQuota(r.ctx, marketing.Quota{
		ID:          marketing.NewID("quota"),
		ActivityID:  activityID,
		Type:        qt,
		Scope:       scope,
		ScopeKey:    scopeKey,
		TotalValue:  total,
		UsedValue:   "0",
		FrozenValue: "0",
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	return err
}

func (r *runner) callbackAll(commands []string, status marketing.GrantStatus) error {
	for _, command := range commands {
		if err := r.callback(command, status, http.StatusOK); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) callback(commandID string, status marketing.GrantStatus, want int) error {
	body := map[string]any{
		"tenantId":   tenantID(r.runID),
		"commandId":  commandID,
		"status":     status,
		"message":    "load-fault " + string(status),
		"finishedAt": time.Now().UTC().Format(time.RFC3339),
	}
	if status == marketing.GrantSuccess {
		body["externalGrantId"] = "external_" + commandID
	}
	var out callbackResponse
	got, response, err := r.doJSON("CALLBACK", http.MethodPost, "/openapi/v1/reward/callback", body, &out)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("callback %s status=%d, want %d body=%s", commandID, got, want, response)
	}
	return nil
}

func (r *runner) waitOutbox(activityID string, wantSent int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sent, _, err := r.outboxStatusCounts(activityID)
		if err != nil {
			return err
		}
		if sent >= wantSent {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	sent, counts, _ := r.outboxStatusCounts(activityID)
	return fmt.Errorf("outbox sent=%d, want=%d counts=%v", sent, wantSent, counts)
}

func (r *runner) waitOutboxRetry(activityID string, wantRetry int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		retry, _, err := r.outboxRetryDLQCounts(activityID)
		if err != nil {
			return err
		}
		if retry >= wantRetry {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	retry, dlq, _ := r.outboxRetryDLQCounts(activityID)
	return fmt.Errorf("outbox retry=%d dlq=%d, want retry=%d", retry, dlq, wantRetry)
}

func (r *runner) waitDLQ(activityID string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, counts, err := r.outboxStatusCounts(activityID)
		if err != nil {
			return err
		}
		if counts[string(marketing.OutboxDLQ)] >= want {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	_, counts, _ := r.outboxStatusCounts(activityID)
	return fmt.Errorf("outbox dlq=%d, want=%d counts=%v", counts[string(marketing.OutboxDLQ)], want, counts)
}

func (r *runner) forceOutboxRetryNow(activityID string) error {
	_, err := r.db.ExecContext(r.ctx, `
		UPDATE marketing_outbox
		SET next_retry_at = NOW()
		WHERE payload_json::text LIKE '%' || $1 || '%'
		  AND send_status = 'FAILED'
	`, activityID)
	return err
}

func (r *runner) outboxStatusCounts(activityID string) (int, map[string]int, error) {
	rows, err := r.db.QueryContext(r.ctx, `
		SELECT send_status, count(*)
		FROM marketing_outbox
		WHERE payload_json::text LIKE '%' || $1 || '%'
		GROUP BY send_status
	`, activityID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, nil, err
		}
		counts[status] = count
	}
	return counts[string(marketing.OutboxSent)], counts, rows.Err()
}

func (r *runner) outboxRetryDLQCounts(activityID string) (int, int, error) {
	var retry, dlq int
	err := r.db.QueryRowContext(r.ctx, `
		SELECT COALESCE(sum(retry_count), 0)::int,
		       count(*) FILTER (WHERE send_status = 'DLQ')::int
		FROM marketing_outbox
		WHERE payload_json::text LIKE '%' || $1 || '%'
	`, activityID).Scan(&retry, &dlq)
	return retry, dlq, err
}

func (r *runner) consistency(activityID string) (map[string]any, error) {
	out := map[string]any{}
	queries := map[string]string{
		"event_inbox":   `SELECT count(*) FROM marketing_event_inbox WHERE tenant_id = $1 AND event_id LIKE $2 || '%'`,
		"participation": `SELECT count(*) FROM marketing_participation WHERE activity_id = $1`,
		"grant_order":   `SELECT count(*) FROM marketing_grant_order WHERE activity_id = $1`,
		"outbox":        `SELECT count(*) FROM marketing_outbox WHERE tenant_id = $2 AND payload_json::text LIKE '%' || $1 || '%'`,
		"reservation":   `SELECT count(*) FROM marketing_quota_reservation WHERE activity_id = $1`,
		"orphan_grant":  `SELECT count(*) FROM marketing_grant_order go LEFT JOIN marketing_participation p ON p.id = go.participation_id WHERE go.activity_id = $1 AND p.id IS NULL`,
		"orphan_outbox": `SELECT count(*) FROM marketing_outbox o WHERE o.tenant_id = $2 AND o.payload_json::text LIKE '%' || $1 || '%' AND NOT EXISTS (SELECT 1 FROM marketing_grant_order go WHERE o.payload_json::text LIKE '%' || go.bill_no || '%')`,
		"quota_drift":   `SELECT count(*) FROM marketing_quota WHERE activity_id = $1 AND (used_value + frozen_value > total_value OR frozen_value < 0 OR used_value < 0)`,
	}
	for name, query := range queries {
		var count int
		args := []any{activityID, tenantID(r.runID)}
		if name == "event_inbox" {
			args = []any{tenantID(r.runID), r.runID}
		} else if !strings.Contains(query, "$2") {
			args = []any{activityID}
		}
		if err := r.db.QueryRowContext(r.ctx, query, args...).Scan(&count); err != nil {
			return nil, err
		}
		out[name] = count
	}
	if out["orphan_grant"].(int) != 0 || out["orphan_outbox"].(int) != 0 || out["quota_drift"].(int) != 0 {
		return out, fmt.Errorf("consistency failed: %v", out)
	}
	out["result"] = "PASS"
	return out, nil
}

func (r *runner) doJSON(stage, method, path string, body any, out any) (int, string, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, "", err
		}
	}
	reqCtx, cancel := context.WithTimeout(r.ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, r.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("content-type", "application/json")
	if method == http.MethodPost && strings.HasPrefix(path, "/openapi/v1/events") {
		req.Header.Set("Idempotency-Key", idempotencyKey(body))
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.lastHTTP = &httpTrace{Stage: stage, Method: method, URL: path, Request: body, Response: err.Error()}
		return 0, "", err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	responseText := string(responseBody)
	r.lastHTTP = &httpTrace{Stage: stage, Method: method, URL: path, Request: body, StatusCode: resp.StatusCode, Response: responseText}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && out != nil && len(responseBody) > 0 {
		if err := json.NewDecoder(bytes.NewReader(responseBody)).Decode(out); err != nil {
			return resp.StatusCode, responseText, err
		}
	}
	return resp.StatusCode, responseText, nil
}

func (r *runner) record(result scenarioResult) {
	r.scenarios = append(r.scenarios, result)
	writeJSONLine("SCENARIO", result)
}

func (r *runner) fail(stage string, err error) error {
	summary := map[string]any{
		"result":     "FAIL",
		"stage":      stage,
		"error":      err.Error(),
		"runId":      r.runID,
		"seed":       r.seed,
		"profile":    r.profile,
		"startedAt":  r.startedAt.Format(time.RFC3339Nano),
		"finishedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"logPath":    r.logPath,
		"reproduce":  reproduceCommand(r.runID, r.seed, r.scenario),
		"lastHTTP":   r.lastHTTP,
		"database":   r.databaseSnapshot(),
	}
	r.writeLog(summary)
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	_, _ = fmt.Fprintln(os.Stderr, string(encoded))
	return fmt.Errorf("%s: %w", stage, err)
}

func (r *runner) writeLog(summary map[string]any) {
	encoded, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(r.logPath, encoded, 0o644)
}

func (r *runner) databaseSnapshot() map[string]any {
	out := map[string]any{}
	for name, query := range map[string]string{
		"activities":     `SELECT count(*) FROM marketing_activity WHERE tenant_id = $1 AND activity_code LIKE $2 || '%'`,
		"inbox":          `SELECT count(*) FROM marketing_event_inbox WHERE tenant_id = $1 AND event_id LIKE $2 || '%'`,
		"participations": `SELECT count(*) FROM marketing_participation WHERE tenant_id = $1 AND idempotency_key LIKE '%' || $2 || '%'`,
		"grants":         `SELECT count(*) FROM marketing_grant_order WHERE tenant_id = $1 AND idempotency_key LIKE '%' || $2 || '%'`,
		"outbox":         `SELECT count(*) FROM marketing_outbox WHERE tenant_id = $1 AND payload_json::text LIKE '%' || $2 || '%'`,
	} {
		var count int
		if err := r.db.QueryRowContext(context.Background(), query, tenantID(r.runID), r.runID).Scan(&count); err != nil {
			out[name] = err.Error()
		} else {
			out[name] = count
		}
	}
	return out
}

func idempotencyKey(body any) string {
	event, ok := body.(map[string]any)
	if !ok {
		return ""
	}
	biz, _ := event["biz"].(map[string]any)
	return strings.Join([]string{fmt.Sprint(event["tenantId"]), fmt.Sprint(event["sourceSystem"]), fmt.Sprint(event["eventType"]), fmt.Sprint(biz["bizType"]), fmt.Sprint(biz["bizId"])}, ":")
}

func tenantID(runID string) string {
	id := defaultTenantPrefix + "_" + runID
	if len(id) <= 64 {
		return id
	}
	return id[:64]
}

func makeSubjects(prefix string, total int) []string {
	subjects := make([]string, total)
	for i := 0; i < total; i++ {
		subjects[i] = fmt.Sprintf("%s%03d", prefix, i)
	}
	return subjects
}

func dailyScopeKey(subject string) string {
	return subject + ":" + time.Now().UTC().Format("2006-01-02")
}

func resolvedConcurrency(target int) int {
	if target < 1 {
		return 1
	}
	return target
}

func summarize(values []time.Duration) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	at := func(p float64) int64 {
		idx := int(float64(len(values)-1) * p)
		return values[idx].Milliseconds()
	}
	return latencySummary{P50MS: at(0.50), P95MS: at(0.95), P99MS: at(0.99)}
}

func rate(count int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(count) / d.Seconds()
}

func redactDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		prefix := dsn[:at]
		if scheme := strings.Index(prefix, "://"); scheme >= 0 {
			return prefix[:scheme+3] + "****:****" + dsn[at:]
		}
	}
	return dsn
}

func reproduceCommand(runID string, seed int64, scenario string) string {
	return fmt.Sprintf("EX_MARKETING_DATABASE_URL='<dsn>' EX_MARKETING_BASE_URL='%s' go run ./cmd/engine-load-fault-tests --profile local --scenario %s --run-id '%s' --seed '%d'", os.Getenv("EX_MARKETING_BASE_URL"), scenario, runID, seed)
}

func writeJSONLine(prefix string, value any) {
	encoded, _ := json.Marshal(value)
	fmt.Printf("%s %s\n", prefix, encoded)
}

func randomSuffix(seed int64) string {
	r := rand.New(rand.NewSource(seed + time.Now().UnixNano()))
	return fmt.Sprintf("%08x", r.Uint32())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
