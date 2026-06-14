package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type schedulerTestPlugin struct {
	code      string
	metadata  Metadata
	failFirst bool
	failWith  string
	noEvents  bool
	calls     atomic.Int32
}

func (p *schedulerTestPlugin) Code() string {
	return p.code
}

func (p *schedulerTestPlugin) CompatibleActivityTypes() []string {
	return []string{"LOTTERY"}
}

func (p *schedulerTestPlugin) Metadata() Metadata {
	return p.metadata
}

func (p *schedulerTestPlugin) ValidateConfig(json.RawMessage) error {
	return nil
}

func (p *schedulerTestPlugin) BuildContext(context.Context, BuildContextRequest) (ContextResult, error) {
	return ContextResult{}, nil
}

func (p *schedulerTestPlugin) AfterQualified(context.Context, AfterQualifiedRequest) ([]StateChangeIntent, error) {
	return nil, nil
}

func (p *schedulerTestPlugin) ExecuteScheduledJob(context.Context, PluginScheduleExecutionRequest) (PluginScheduleExecutionResult, error) {
	call := p.calls.Add(1)
	if p.failFirst && call == 1 {
		message := "temporary failure"
		if p.failWith != "" {
			message = p.failWith
		}
		return PluginScheduleExecutionResult{}, errors.New(message)
	}
	if p.noEvents {
		return PluginScheduleExecutionResult{}, nil
	}
	return PluginScheduleExecutionResult{
		Events: []PluginGeneratedEvent{{Attributes: map[string]any{"roundId": "round_001"}}},
	}, nil
}

func TestPluginSchedulerRegistersExecutesAndPreventsRepeatSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	registry, scheduled := schedulerRegistry(t, true)
	schedules := NewMemoryPluginScheduleRepository()
	inbox := marketing.NewMemoryRepository()
	scheduler := NewPluginScheduler(registry, schedules, inbox)
	scheduler.SetNow(func() time.Time { return now })

	job, duplicate, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobDraw,
		ScheduledAt: now.Add(-time.Minute),
	})
	if err != nil || duplicate {
		t.Fatalf("register job duplicate=%v err=%v", duplicate, err)
	}
	again, duplicate, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobDraw,
		ScheduledAt: now.Add(-time.Minute),
	})
	if err != nil || !duplicate || again.JobID != job.JobID {
		t.Fatalf("expected duplicate registration, job=%+v duplicate=%v err=%v", again, duplicate, err)
	}

	executed, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("execute job: %v", err)
	}
	if executed.Status != PluginScheduleStatusSuccess {
		t.Fatalf("status=%s, want SUCCESS: %+v", executed.Status, executed)
	}
	if scheduled.calls.Load() != 1 {
		t.Fatalf("plugin calls=%d, want 1", scheduled.calls.Load())
	}

	repeated, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("repeat execute: %v", err)
	}
	if repeated.Status != PluginScheduleStatusSuccess || scheduled.calls.Load() != 1 {
		t.Fatalf("repeat should not rerun successful job: job=%+v calls=%d", repeated, scheduled.calls.Load())
	}
}

func TestPluginSchedulerGeneratedStandardEventPayloadIsStable(t *testing.T) {
	scheduledAt := time.Date(2026, 5, 29, 9, 30, 0, 0, time.UTC)
	job := PluginScheduleJob{
		JobID:              "job_round_draw",
		TenantID:           "site_001",
		ActivityID:         "act_lottery",
		ActivityType:       "LOTTERY",
		ActivityVersion:    3,
		ActivityConfigHash: "locked-config",
		PluginCode:         "LOTTERY_PLUGIN",
		JobType:            PluginScheduleJobDraw,
		ScheduledAt:        scheduledAt,
	}
	left, err := StandardEventFromPluginJob(job, PluginGeneratedEvent{}, 0)
	if err != nil {
		t.Fatalf("map left: %v", err)
	}
	right, err := StandardEventFromPluginJob(job, PluginGeneratedEvent{}, 0)
	if err != nil {
		t.Fatalf("map right: %v", err)
	}
	if !left.SameStandardPayload(right) {
		t.Fatalf("expected stable standard payload\nleft=%+v\nright=%+v", left, right)
	}
	if !left.OccurredAt.Equal(scheduledAt) {
		t.Fatalf("occurredAt=%s, want scheduledAt %s", left.OccurredAt, scheduledAt)
	}
	if left.EventID != "plugin:job_round_draw:0" || left.IdempotencyKey() != "site_001:activity_plugin:PLUGIN_DRAWN:PLUGIN_SCHEDULE_JOB:job_round_draw" {
		t.Fatalf("unexpected event identity: eventId=%s key=%s", left.EventID, left.IdempotencyKey())
	}

	mutatedActivity := schedulerActivity()
	mutatedActivity.Version = 99
	mutatedActivity.Config = json.RawMessage(`{"roundId":"changed","drawAt":"2026-05-29T10:30:00Z"}`)
	replayed, err := StandardEventFromPluginJob(job, PluginGeneratedEvent{}, 0)
	if err != nil {
		t.Fatalf("map replayed: %v", err)
	}
	if !left.SameStandardPayload(replayed) {
		t.Fatalf("payload changed after unrelated activity mutation\nleft=%+v\nreplayed=%+v", left, replayed)
	}
}

func TestPluginSchedulerMapsAllJobTypesAndAllowsCompensateNoEvent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		jobType   PluginScheduleJobType
		eventType string
	}{
		{PluginScheduleJobStart, "PLUGIN_ROUND_STARTED"},
		{PluginScheduleJobEnd, "PLUGIN_ROUND_ENDED"},
		{PluginScheduleJobDraw, "PLUGIN_DRAWN"},
		{PluginScheduleJobSettle, "PLUGIN_SETTLED"},
		{PluginScheduleJobCompensate, "PLUGIN_COMPENSATED"},
	}
	for _, tc := range cases {
		t.Run(string(tc.jobType), func(t *testing.T) {
			event, err := StandardEventFromPluginJob(PluginScheduleJob{
				JobID:              "job_" + strings.ToLower(string(tc.jobType)),
				TenantID:           "site_001",
				ActivityID:         "act_lottery",
				ActivityType:       "LOTTERY",
				ActivityVersion:    7,
				ActivityConfigHash: "config-hash",
				PluginCode:         "LOTTERY_PLUGIN",
				JobType:            tc.jobType,
				ScheduledAt:        now,
			}, PluginGeneratedEvent{}, 0)
			if err != nil {
				t.Fatalf("map: %v", err)
			}
			if event.EventType != tc.eventType || event.EventID != "plugin:job_"+strings.ToLower(string(tc.jobType))+":0" {
				t.Fatalf("unexpected event mapping: %+v", event)
			}
		})
	}

	registry, scheduled := schedulerRegistry(t, true)
	scheduled.noEvents = true
	repository := NewMemoryPluginScheduleRepository()
	scheduler := NewPluginScheduler(registry, repository, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })
	job, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobCompensate,
		ScheduledAt: now,
	})
	if err != nil {
		t.Fatalf("register compensate: %v", err)
	}
	executed, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("execute compensate: %v", err)
	}
	if executed.Status != PluginScheduleStatusSuccess || scheduled.calls.Load() != 1 {
		t.Fatalf("expected no-event compensate success, job=%+v calls=%d", executed, scheduled.calls.Load())
	}
	assertAuditAction(t, repository, job.JobID, "SCHEDULE_JOB_NO_EVENT_OUTPUT")
}

func TestPluginSchedulerRetriesRetryableFailureThenSucceeds(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	registry, _ := schedulerRegistry(t, true)
	registration, _ := registry.GetRegistration("LOTTERY_PLUGIN")
	scheduled := registration.Plugin.(*schedulerTestPlugin)
	scheduled.failFirst = true
	schedules := NewMemoryPluginScheduleRepository()
	scheduler := NewPluginScheduler(registry, schedules, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })

	job, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobStart,
		ScheduledAt: now,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	failed, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if failed.Status != PluginScheduleStatusRetry || failed.ErrorClass != SchedulerErrorPluginExecutionFailed || failed.RetryCount != 1 || failed.NextRetryAt.IsZero() {
		t.Fatalf("expected retryable failure, got %+v", failed)
	}

	scheduler.SetNow(func() time.Time { return failed.NextRetryAt })
	succeeded, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("retry execute: %v", err)
	}
	if succeeded.Status != PluginScheduleStatusSuccess || scheduled.calls.Load() != 2 {
		t.Fatalf("expected retry success, job=%+v calls=%d", succeeded, scheduled.calls.Load())
	}
}

func TestPluginSchedulerRejectsInvalidPluginRegistrationClasses(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		registry  *Registry
		activity  marketing.Activity
		wantClass string
	}{
		{
			name:      "plugin not registered",
			registry:  NewRegistry(),
			activity:  schedulerActivity(),
			wantClass: SchedulerErrorPluginNotFound,
		},
		{
			name: "plugin disabled",
			registry: schedulerRegistryWithMetadata(t, Metadata{
				PluginCode:             "LOTTERY_PLUGIN",
				Version:                "1.0.0",
				Capabilities:           []string{CapabilityContextBuild, CapabilityEventHandle},
				SupportedActivityTypes: []string{"LOTTERY"},
				Disabled:               true,
			}),
			activity:  schedulerActivity(),
			wantClass: SchedulerErrorPluginDisabled,
		},
		{
			name: "activity type unsupported",
			registry: schedulerRegistryWithMetadata(t, Metadata{
				PluginCode:             "LOTTERY_PLUGIN",
				Version:                "1.0.0",
				Capabilities:           []string{CapabilityContextBuild, CapabilityEventHandle},
				SupportedActivityTypes: []string{"FIRST_DEPOSIT"},
			}),
			activity:  schedulerActivity(),
			wantClass: SchedulerErrorPluginActivityUnsupported,
		},
		{
			name:     "unbound plugin",
			registry: schedulerRegistryWithMetadata(t, Metadata{PluginCode: "LOTTERY_PLUGIN", Version: "1.0.0", Capabilities: []string{CapabilityContextBuild, CapabilityEventHandle}, SupportedActivityTypes: []string{"LOTTERY"}}),
			activity: func() marketing.Activity {
				activity := schedulerActivity()
				activity.PluginCode = "OTHER_PLUGIN"
				return activity
			}(),
			wantClass: SchedulerErrorPluginNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheduler := NewPluginScheduler(tc.registry, NewMemoryPluginScheduleRepository(), marketing.NewMemoryRepository())
			scheduler.SetNow(func() time.Time { return now })
			_, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
				Activity:    tc.activity,
				PluginCode:  "LOTTERY_PLUGIN",
				JobType:     PluginScheduleJobStart,
				ScheduledAt: now,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantClass) {
				t.Fatalf("expected %s, got %v", tc.wantClass, err)
			}
		})
	}
}

func TestPluginSchedulerMarksInboxConflictNonRetryable(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	registry, _ := schedulerRegistry(t, true)
	schedules := NewMemoryPluginScheduleRepository()
	inbox := marketing.NewMemoryRepository()
	scheduler := NewPluginScheduler(registry, schedules, inbox)
	scheduler.SetNow(func() time.Time { return now })

	job, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobDraw,
		ScheduledAt: now,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	conflictingEvent, err := StandardEventFromPluginJob(job, PluginGeneratedEvent{Attributes: map[string]any{"roundId": "changed"}}, 0)
	if err != nil {
		t.Fatalf("map conflict event: %v", err)
	}
	conflictingEvent.EventID = conflictingEvent.EventID + ":conflict"
	_, _, err = inbox.CreateInbox(ctx, marketing.EventInbox{
		ID:             "inbox_existing",
		Event:          conflictingEvent,
		IdempotencyKey: conflictingEvent.IdempotencyKey(),
		ProcessStatus:  marketing.InboxReceived,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("seed inbox: %v", err)
	}

	failed, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("execute with conflict: %v", err)
	}
	if failed.Status != PluginScheduleStatusFailed || failed.ErrorClass != SchedulerErrorIdempotencyConflict || !failed.NextRetryAt.IsZero() {
		t.Fatalf("expected non-retryable idempotency conflict, got %+v", failed)
	}
}

func TestPluginSchedulerRedactsSensitiveRetryableFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	registry, _ := schedulerRegistry(t, true)
	registration, _ := registry.GetRegistration("LOTTERY_PLUGIN")
	scheduled := registration.Plugin.(*schedulerTestPlugin)
	scheduled.failFirst = true
	scheduled.failWith = "Authorization: Bearer token-value; Cookie=session; secret=raw"
	repository := NewMemoryPluginScheduleRepository()
	scheduler := NewPluginScheduler(registry, repository, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })

	job, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobDraw,
		ScheduledAt: now,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	failed, err := scheduler.ExecuteJob(ctx, job.JobID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if failed.ErrorMessage != "sensitive error redacted" {
		t.Fatalf("expected redacted error, got %q", failed.ErrorMessage)
	}
	audits, err := repository.ListAudit(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	for _, audit := range audits {
		lower := strings.ToLower(audit.Message)
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "token-value") || strings.Contains(lower, "secret=raw") {
			t.Fatalf("sensitive audit message was not redacted: %+v", audit)
		}
	}
}

func TestPluginSchedulerRequiresEventHandleCapabilityAndCloseAudit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	registry, _ := schedulerRegistry(t, false)
	schedules := NewMemoryPluginScheduleRepository()
	scheduler := NewPluginScheduler(registry, schedules, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })

	_, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobCompensate,
		ScheduledAt: now,
	})
	if err == nil || !strings.Contains(err.Error(), SchedulerErrorPluginCapabilityMissing) {
		t.Fatalf("expected missing EVENT_HANDLE rejection, got %v", err)
	}

	registry, _ = schedulerRegistry(t, true)
	scheduler = NewPluginScheduler(registry, schedules, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })
	job, _, err := scheduler.RegisterJob(ctx, PluginScheduleRegisterCommand{
		Activity:    schedulerActivity(),
		JobType:     PluginScheduleJobCompensate,
		ScheduledAt: now,
	})
	if err != nil {
		t.Fatalf("register closable job: %v", err)
	}
	if _, err := scheduler.CloseJob(ctx, job.JobID, "", "duplicate schedule"); err == nil {
		t.Fatalf("expected close to require operator")
	}
	closed, err := scheduler.CloseJob(ctx, job.JobID, "ops", "duplicate schedule")
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.Status != PluginScheduleStatusClosed || closed.ClosedBy != "ops" || closed.ClosedReason == "" {
		t.Fatalf("unexpected closed job: %+v", closed)
	}
	audits, err := schedules.ListAudit(ctx, job.JobID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) == 0 || audits[len(audits)-1].Action != "SCHEDULE_JOB_CLOSED" {
		t.Fatalf("expected close audit, got %+v", audits)
	}
}

func TestMemoryPluginScheduleRepositoryClaimOnlyOnceConcurrently(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	repository := NewMemoryPluginScheduleRepository()
	job := PluginScheduleJob{
		JobID:       "job_once",
		TenantID:    "site_001",
		ActivityID:  "act_lottery",
		PluginCode:  "LOTTERY_PLUGIN",
		JobType:     PluginScheduleJobEnd,
		ScheduledAt: now,
		Status:      PluginScheduleStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, _, err := repository.Save(ctx, job); err != nil {
		t.Fatalf("save: %v", err)
	}
	const workers = 20
	var wg sync.WaitGroup
	var claims atomic.Int32
	var errs atomic.Int32
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, claimed, err := repository.Claim(ctx, job.JobID, now); err != nil {
				errs.Add(1)
			} else if claimed {
				claims.Add(1)
			}
		}()
	}
	wg.Wait()
	if errs.Load() != 0 || claims.Load() != 1 {
		t.Fatalf("claims=%d errs=%d, want one successful claim and no errors", claims.Load(), errs.Load())
	}
	if _, claimed, err := repository.Claim(ctx, job.JobID, now); err != nil || claimed {
		t.Fatalf("post-concurrency claim claimed=%v err=%v", claimed, err)
	}
}

func TestPluginSchedulerRecoversOnlyStaleRunningJobs(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	repository := NewMemoryPluginScheduleRepository()
	staleRunning := PluginScheduleJob{
		JobID:       "job_stale_running",
		TenantID:    "site_001",
		ActivityID:  "act_lottery",
		PluginCode:  "LOTTERY_PLUGIN",
		JobType:     PluginScheduleJobEnd,
		ScheduledAt: now.Add(-time.Hour),
		Status:      PluginScheduleStatusRunning,
		LastRunAt:   now.Add(-30 * time.Minute),
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-30 * time.Minute),
	}
	success := staleRunning
	success.JobID = "job_success"
	success.JobType = PluginScheduleJobSettle
	success.Status = PluginScheduleStatusSuccess
	if _, _, err := repository.Save(ctx, staleRunning); err != nil {
		t.Fatalf("save running: %v", err)
	}
	if _, _, err := repository.Save(ctx, success); err != nil {
		t.Fatalf("save success: %v", err)
	}
	scheduler := NewPluginScheduler(NewRegistry(), repository, marketing.NewMemoryRepository())
	scheduler.SetNow(func() time.Time { return now })
	recovered, err := scheduler.RecoverRunningJobs(ctx, now.Add(-10*time.Minute), 10)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(recovered) != 1 || recovered[0].JobID != staleRunning.JobID || recovered[0].Status != PluginScheduleStatusRetry || recovered[0].RetryCount != 1 {
		t.Fatalf("unexpected recovered jobs: %+v", recovered)
	}
	stillSuccess, err := repository.Get(ctx, success.JobID)
	if err != nil {
		t.Fatalf("get success: %v", err)
	}
	if stillSuccess.Status != PluginScheduleStatusSuccess {
		t.Fatalf("success job must remain terminal, got %+v", stillSuccess)
	}
	assertAuditAction(t, repository, staleRunning.JobID, "SCHEDULE_JOB_RUNNING_RECOVERED")
}

func schedulerRegistry(t *testing.T, withEventHandle bool) (*Registry, *schedulerTestPlugin) {
	t.Helper()
	capabilities := []string{CapabilityContextBuild}
	if withEventHandle {
		capabilities = append(capabilities, CapabilityEventHandle)
	}
	scheduled := &schedulerTestPlugin{
		code: "LOTTERY_PLUGIN",
		metadata: Metadata{
			PluginCode:             "LOTTERY_PLUGIN",
			Version:                "1.0.0",
			Capabilities:           capabilities,
			SupportedActivityTypes: []string{"LOTTERY"},
		},
	}
	registry := NewRegistry()
	if err := registry.Register(scheduled); err != nil {
		t.Fatalf("register scheduler plugin: %v", err)
	}
	return registry, scheduled
}

func schedulerRegistryWithMetadata(t *testing.T, metadata Metadata) *Registry {
	t.Helper()
	scheduled := &schedulerTestPlugin{
		code:     metadata.PluginCode,
		metadata: metadata,
	}
	registry := NewRegistry()
	if err := registry.Register(scheduled); err != nil {
		t.Fatalf("register scheduler plugin: %v", err)
	}
	return registry
}

func assertAuditAction(t *testing.T, repository *MemoryPluginScheduleRepository, jobID string, action string) {
	t.Helper()
	audits, err := repository.ListAudit(context.Background(), jobID)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	for _, audit := range audits {
		if audit.Action == action {
			return
		}
	}
	t.Fatalf("expected audit action %s, got %+v", action, audits)
}

func schedulerActivity() marketing.Activity {
	return marketing.Activity{
		ID:            "act_lottery",
		TenantID:      "site_001",
		Code:          "lottery_001",
		Name:          "Lottery",
		Type:          "LOTTERY",
		Status:        marketing.ActivityActive,
		PluginCode:    "LOTTERY_PLUGIN",
		Version:       7,
		Config:        json.RawMessage(`{"roundId":"round_001","drawAt":"2026-05-29T10:00:00Z"}`),
		StartTime:     time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC),
		TriggerEvents: []string{"PLUGIN_ROUND_STARTED", "PLUGIN_ROUND_ENDED", "PLUGIN_DRAWN", "PLUGIN_SETTLED", "PLUGIN_COMPENSATED"},
	}
}
