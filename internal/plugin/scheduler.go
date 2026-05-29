package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	PluginScheduleJobStart      PluginScheduleJobType = "START"
	PluginScheduleJobEnd        PluginScheduleJobType = "END"
	PluginScheduleJobDraw       PluginScheduleJobType = "DRAW"
	PluginScheduleJobSettle     PluginScheduleJobType = "SETTLE"
	PluginScheduleJobCompensate PluginScheduleJobType = "COMPENSATE"

	PluginScheduleStatusPending PluginScheduleJobStatus = "PENDING"
	PluginScheduleStatusRunning PluginScheduleJobStatus = "RUNNING"
	PluginScheduleStatusSuccess PluginScheduleJobStatus = "SUCCESS"
	PluginScheduleStatusFailed  PluginScheduleJobStatus = "FAILED"
	PluginScheduleStatusRetry   PluginScheduleJobStatus = "RETRY"
	PluginScheduleStatusClosed  PluginScheduleJobStatus = "CLOSED"

	SchedulerErrorActivityNotFoundOrInactive = "ACTIVITY_NOT_FOUND_OR_INACTIVE"
	SchedulerErrorEventInboxFailed           = "EVENT_INBOX_FAILED"
	SchedulerErrorEventMappingInvalid        = "EVENT_MAPPING_INVALID"
	SchedulerErrorIdempotencyConflict        = "IDEMPOTENCY_CONFLICT"
	SchedulerErrorInternal                   = "INTERNAL_ERROR"
	SchedulerErrorPluginExecutionFailed      = ReasonPluginExecutionFailed
	SchedulerErrorPluginNotFound             = ReasonPluginNotRegistered
	SchedulerErrorPluginDisabled             = ReasonPluginDisabled
	SchedulerErrorPluginCapabilityMissing    = ReasonPluginCapabilityMissing
	SchedulerErrorPluginActivityUnsupported  = ReasonPluginIncompatible
)

type PluginScheduleJobType string
type PluginScheduleJobStatus string

type PluginScheduleJob struct {
	JobID              string                  `json:"jobId"`
	TenantID           string                  `json:"tenantId"`
	ActivityID         string                  `json:"activityId"`
	ActivityType       string                  `json:"activityType"`
	ActivityVersion    int                     `json:"activityVersion"`
	ActivityConfigHash string                  `json:"activityConfigHash,omitempty"`
	PluginCode         string                  `json:"pluginCode"`
	JobType            PluginScheduleJobType   `json:"jobType"`
	ScheduledAt        time.Time               `json:"scheduledAt"`
	Status             PluginScheduleJobStatus `json:"status"`
	RetryCount         int                     `json:"retryCount"`
	NextRetryAt        time.Time               `json:"nextRetryAt,omitempty"`
	ErrorClass         string                  `json:"errorClass,omitempty"`
	ErrorMessage       string                  `json:"errorMessage,omitempty"`
	LastRunAt          time.Time               `json:"lastRunAt,omitempty"`
	ClosedBy           string                  `json:"closedBy,omitempty"`
	ClosedReason       string                  `json:"closedReason,omitempty"`
	CreatedAt          time.Time               `json:"createdAt"`
	UpdatedAt          time.Time               `json:"updatedAt"`
}

type PluginScheduleRegisterCommand struct {
	JobID       string
	Activity    marketing.Activity
	PluginCode  string
	JobType     PluginScheduleJobType
	ScheduledAt time.Time
}

type PluginScheduleExecutionRequest struct {
	Job      PluginScheduleJob
	Now      time.Time
	TraceID  string
	Attempt  int
	Activity marketing.Activity
	Plugin   Metadata
}

type PluginScheduleExecutionResult struct {
	Events []PluginGeneratedEvent
}

type PluginGeneratedEvent struct {
	EventType  string
	Subject    marketing.EventSubject
	Biz        marketing.EventBiz
	OccurredAt time.Time
	Attributes map[string]any
}

type ScheduledActivityPlugin interface {
	ActivityPlugin
	ExecuteScheduledJob(context.Context, PluginScheduleExecutionRequest) (PluginScheduleExecutionResult, error)
}

type ScheduleAuditRecord struct {
	Action     string
	JobID      string
	TenantID   string
	ActivityID string
	PluginCode string
	JobType    PluginScheduleJobType
	Status     PluginScheduleJobStatus
	ErrorClass string
	Message    string
	CreatedAt  time.Time
}

type PluginScheduleRepository interface {
	Save(context.Context, PluginScheduleJob) (PluginScheduleJob, bool, error)
	Get(context.Context, string) (PluginScheduleJob, error)
	ListByActivity(context.Context, string, string, string) ([]PluginScheduleJob, error)
	ListDue(context.Context, time.Time, int) ([]PluginScheduleJob, error)
	Claim(context.Context, string, time.Time) (PluginScheduleJob, bool, error)
	MarkSuccess(context.Context, string, time.Time) (PluginScheduleJob, error)
	MarkFailed(context.Context, string, string, string, bool, time.Time, time.Time) (PluginScheduleJob, error)
	Retry(context.Context, string, time.Time, time.Time) (PluginScheduleJob, error)
	RecoverRunning(context.Context, time.Time, time.Time, int) ([]PluginScheduleJob, error)
	Close(context.Context, string, string, string, time.Time) (PluginScheduleJob, error)
	AppendAudit(context.Context, ScheduleAuditRecord) error
	ListAudit(context.Context, string) ([]ScheduleAuditRecord, error)
}

type PluginScheduleInbox interface {
	CreateInbox(context.Context, marketing.EventInbox) (marketing.EventInbox, bool, error)
}

type PluginScheduler struct {
	registry   *Registry
	repository PluginScheduleRepository
	inbox      PluginScheduleInbox
	now        func() time.Time
	retryAfter []time.Duration
}

func NewPluginScheduler(registry *Registry, repository PluginScheduleRepository, inbox PluginScheduleInbox) *PluginScheduler {
	return &PluginScheduler{
		registry:   registry,
		repository: repository,
		inbox:      inbox,
		now:        func() time.Time { return time.Now().UTC() },
		retryAfter: []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute},
	}
}

func (s *PluginScheduler) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *PluginScheduler) RegisterJob(ctx context.Context, cmd PluginScheduleRegisterCommand) (PluginScheduleJob, bool, error) {
	if s == nil || s.repository == nil {
		return PluginScheduleJob{}, false, errors.New("plugin scheduler repository is required")
	}
	activity := cmd.Activity
	pluginCode := strings.TrimSpace(cmd.PluginCode)
	if pluginCode == "" {
		pluginCode = strings.TrimSpace(activity.PluginCode)
	}
	if errClass, err := s.validatePluginForActivity(activity, pluginCode); err != nil {
		return PluginScheduleJob{}, false, errors.Join(marketing.ErrInvalidOperation, fmt.Errorf("%s: %w", errClass, err))
	}
	if !validScheduleJobType(cmd.JobType) {
		return PluginScheduleJob{}, false, errors.Join(marketing.ErrInvalidOperation, errors.New("jobType is invalid"))
	}
	if cmd.ScheduledAt.IsZero() {
		return PluginScheduleJob{}, false, errors.Join(marketing.ErrInvalidOperation, errors.New("scheduledAt is required"))
	}
	now := s.now().UTC()
	jobID := strings.TrimSpace(cmd.JobID)
	if jobID == "" {
		jobID = stableScheduleJobID(activity.TenantID, activity.ID, pluginCode, cmd.JobType, cmd.ScheduledAt.UTC())
	}
	job := PluginScheduleJob{
		JobID:              jobID,
		TenantID:           strings.TrimSpace(activity.TenantID),
		ActivityID:         strings.TrimSpace(activity.ID),
		ActivityType:       strings.TrimSpace(activity.Type),
		ActivityVersion:    activity.Version,
		ActivityConfigHash: rawJSONHash(activity.Config),
		PluginCode:         pluginCode,
		JobType:            cmd.JobType,
		ScheduledAt:        cmd.ScheduledAt.UTC(),
		Status:             PluginScheduleStatusPending,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	saved, duplicate, err := s.repository.Save(ctx, job)
	if err != nil {
		return PluginScheduleJob{}, false, err
	}
	_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_REGISTERED", saved, "", "", now))
	return saved, duplicate, nil
}

func (s *PluginScheduler) ExecuteDue(ctx context.Context, limit int) ([]PluginScheduleJob, error) {
	if limit <= 0 {
		limit = 100
	}
	due, err := s.repository.ListDue(ctx, s.now().UTC(), limit)
	if err != nil {
		return nil, err
	}
	out := make([]PluginScheduleJob, 0, len(due))
	for _, job := range due {
		executed, err := s.ExecuteJob(ctx, job.JobID)
		if err != nil {
			return out, err
		}
		out = append(out, executed)
	}
	return out, nil
}

func (s *PluginScheduler) ExecuteJob(ctx context.Context, jobID string) (PluginScheduleJob, error) {
	now := s.now().UTC()
	job, claimed, err := s.repository.Claim(ctx, strings.TrimSpace(jobID), now)
	if err != nil {
		return PluginScheduleJob{}, err
	}
	if !claimed {
		return job, nil
	}
	_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_CLAIMED", job, "", "", now))

	registration, errClass, err := s.resolvePluginForJob(job)
	if err != nil {
		return s.fail(ctx, job, errClass, err, false)
	}
	scheduledPlugin, ok := registration.Plugin.(ScheduledActivityPlugin)
	if !ok {
		return s.fail(ctx, job, SchedulerErrorPluginCapabilityMissing, errors.New("plugin does not implement scheduled execution"), false)
	}
	result, err := scheduledPlugin.ExecuteScheduledJob(ctx, PluginScheduleExecutionRequest{
		Job:     job,
		Now:     now,
		Attempt: job.RetryCount + 1,
		Plugin:  registration.Metadata,
	})
	if err != nil {
		return s.fail(ctx, job, SchedulerErrorPluginExecutionFailed, err, true)
	}
	for i, generated := range result.Events {
		event, err := StandardEventFromPluginJob(job, generated, i)
		if err != nil {
			return s.fail(ctx, job, SchedulerErrorEventMappingInvalid, err, false)
		}
		inbox := marketing.EventInbox{
			ID:             marketing.NewID("inbox"),
			Event:          event,
			IdempotencyKey: event.IdempotencyKey(),
			ProcessStatus:  marketing.InboxReceived,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if _, duplicate, err := s.inbox.CreateInbox(ctx, inbox); err != nil {
			if errors.Is(err, marketing.ErrIdempotencyConflict) {
				return s.fail(ctx, job, SchedulerErrorIdempotencyConflict, err, false)
			}
			return s.fail(ctx, job, SchedulerErrorEventInboxFailed, err, true)
		} else if duplicate {
			_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_EVENT_INBOX_DUPLICATE", job, "", event.IdempotencyKey(), now))
		} else {
			_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_EVENT_INBOX_CREATED", job, "", event.IdempotencyKey(), now))
		}
	}
	if len(result.Events) == 0 {
		_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_NO_EVENT_OUTPUT", job, "", "", now))
	}
	success, err := s.repository.MarkSuccess(ctx, job.JobID, now)
	if err != nil {
		return PluginScheduleJob{}, err
	}
	_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_SUCCESS", success, "", "", now))
	return success, nil
}

func (s *PluginScheduler) RetryJob(ctx context.Context, jobID string) (PluginScheduleJob, error) {
	now := s.now().UTC()
	job, err := s.repository.Retry(ctx, strings.TrimSpace(jobID), now, now)
	if err == nil {
		_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_RETRY_REQUESTED", job, "", "", now))
	}
	return job, err
}

func (s *PluginScheduler) RecoverRunningJobs(ctx context.Context, staleBefore time.Time, limit int) ([]PluginScheduleJob, error) {
	now := s.now().UTC()
	jobs, err := s.repository.RecoverRunning(ctx, staleBefore.UTC(), now, limit)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_RUNNING_RECOVERED", job, SchedulerErrorInternal, "running job recovered to retry", now))
	}
	return jobs, nil
}

func (s *PluginScheduler) CloseJob(ctx context.Context, jobID string, operator string, reason string) (PluginScheduleJob, error) {
	now := s.now().UTC()
	job, err := s.repository.Close(ctx, strings.TrimSpace(jobID), strings.TrimSpace(operator), strings.TrimSpace(reason), now)
	if err == nil {
		_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_CLOSED", job, "", reason, now))
	}
	return job, err
}

func (s *PluginScheduler) ListByActivity(ctx context.Context, tenantID string, activityID string, pluginCode string) ([]PluginScheduleJob, error) {
	return s.repository.ListByActivity(ctx, tenantID, activityID, pluginCode)
}

func (s *PluginScheduler) fail(ctx context.Context, job PluginScheduleJob, class string, cause error, retryable bool) (PluginScheduleJob, error) {
	now := s.now().UTC()
	nextRetryAt := time.Time{}
	if retryable && job.RetryCount < len(s.retryAfter) {
		nextRetryAt = now.Add(s.retryAfter[job.RetryCount])
	}
	failed, err := s.repository.MarkFailed(ctx, job.JobID, class, sanitizeScheduleError(cause), retryable && !nextRetryAt.IsZero(), nextRetryAt, now)
	if err != nil {
		return PluginScheduleJob{}, err
	}
	_ = s.repository.AppendAudit(ctx, scheduleAudit("SCHEDULE_JOB_FAILED", failed, class, failed.ErrorMessage, now))
	return failed, nil
}

func (s *PluginScheduler) validatePluginForActivity(activity marketing.Activity, pluginCode string) (string, error) {
	if strings.TrimSpace(activity.ID) == "" || strings.TrimSpace(activity.TenantID) == "" {
		return SchedulerErrorActivityNotFoundOrInactive, errors.New("activity is required")
	}
	if strings.TrimSpace(activity.PluginCode) != pluginCode {
		return SchedulerErrorPluginNotFound, errors.New("plugin is not bound to activity")
	}
	registration, errClass, err := classifyRegistration(s.registry, pluginCode, activity.Type)
	if err != nil {
		return errClass, err
	}
	if missing := MissingCapabilities(registration.Metadata.Capabilities, []string{CapabilityEventHandle}); len(missing) > 0 {
		return SchedulerErrorPluginCapabilityMissing, fmt.Errorf("missing capabilities: %s", strings.Join(missing, ","))
	}
	return "", nil
}

func (s *PluginScheduler) resolvePluginForJob(job PluginScheduleJob) (Registration, string, error) {
	registration, errClass, err := classifyRegistration(s.registry, job.PluginCode, job.ActivityType)
	if err != nil {
		return Registration{}, errClass, err
	}
	if missing := MissingCapabilities(registration.Metadata.Capabilities, []string{CapabilityEventHandle}); len(missing) > 0 {
		return Registration{}, SchedulerErrorPluginCapabilityMissing, fmt.Errorf("missing capabilities: %s", strings.Join(missing, ","))
	}
	return registration, "", nil
}

func StandardEventFromPluginJob(job PluginScheduleJob, generated PluginGeneratedEvent, sequence int) (marketing.ActivityEvent, error) {
	if strings.TrimSpace(job.JobID) == "" || strings.TrimSpace(job.TenantID) == "" || strings.TrimSpace(job.PluginCode) == "" {
		return marketing.ActivityEvent{}, errors.New("job identity is incomplete")
	}
	eventType := strings.TrimSpace(generated.EventType)
	if eventType == "" {
		eventType = defaultScheduleEventType(job.JobType)
	}
	if eventType == "" {
		return marketing.ActivityEvent{}, errors.New("eventType cannot be mapped")
	}
	subject := generated.Subject
	if strings.TrimSpace(subject.Type) == "" && strings.TrimSpace(subject.ID) == "" {
		subject = marketing.EventSubject{Type: "SYSTEM", ID: job.PluginCode}
	}
	biz := generated.Biz
	if strings.TrimSpace(biz.BizType) == "" {
		biz.BizType = "PLUGIN_SCHEDULE_JOB"
	}
	if strings.TrimSpace(biz.BizID) == "" {
		biz.BizID = job.JobID
		if sequence > 0 {
			biz.BizID = fmt.Sprintf("%s:%d", job.JobID, sequence)
		}
	}
	occurredAt := generated.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = job.ScheduledAt
	}
	attributes := map[string]any{
		"activityId":         job.ActivityID,
		"activityVersion":    job.ActivityVersion,
		"activityConfigHash": job.ActivityConfigHash,
		"pluginCode":         job.PluginCode,
		"jobId":              job.JobID,
		"jobType":            string(job.JobType),
		"scheduledAt":        job.ScheduledAt.UTC().Format(time.RFC3339Nano),
		"sequence":           sequence,
	}
	for key, value := range generated.Attributes {
		key = strings.TrimSpace(key)
		if key != "" {
			attributes[key] = value
		}
	}
	event := marketing.ActivityEvent{
		EventID:      fmt.Sprintf("plugin:%s:%d", job.JobID, sequence),
		EventType:    eventType,
		SourceSystem: "activity_plugin",
		TenantID:     job.TenantID,
		Subject:      subject,
		Biz:          biz,
		OccurredAt:   occurredAt.UTC(),
		Attributes:   attributes,
	}
	return event.Normalized(), event.Validate()
}

func classifyRegistration(registry *Registry, pluginCode string, activityType string) (Registration, string, error) {
	if registry == nil {
		return Registration{}, SchedulerErrorPluginNotFound, errors.New("plugin registry is required")
	}
	registration, ok := registry.GetRegistration(pluginCode)
	if !ok {
		return Registration{}, SchedulerErrorPluginNotFound, errors.New("plugin is not registered")
	}
	if registration.Metadata.Disabled {
		return Registration{}, SchedulerErrorPluginDisabled, errors.New("plugin is disabled")
	}
	if activityType != "" && !slices.Contains(registration.Metadata.SupportedActivityTypes, activityType) {
		return Registration{}, SchedulerErrorPluginActivityUnsupported, errors.New("plugin does not support activity type")
	}
	return registration, "", nil
}

func defaultScheduleEventType(jobType PluginScheduleJobType) string {
	switch jobType {
	case PluginScheduleJobStart:
		return "PLUGIN_ROUND_STARTED"
	case PluginScheduleJobEnd:
		return "PLUGIN_ROUND_ENDED"
	case PluginScheduleJobDraw:
		return "PLUGIN_DRAWN"
	case PluginScheduleJobSettle:
		return "PLUGIN_SETTLED"
	case PluginScheduleJobCompensate:
		return "PLUGIN_COMPENSATED"
	default:
		return ""
	}
}

func validScheduleJobType(jobType PluginScheduleJobType) bool {
	switch jobType {
	case PluginScheduleJobStart, PluginScheduleJobEnd, PluginScheduleJobDraw, PluginScheduleJobSettle, PluginScheduleJobCompensate:
		return true
	default:
		return false
	}
}

func stableScheduleJobID(tenantID string, activityID string, pluginCode string, jobType PluginScheduleJobType, scheduledAt time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(tenantID),
		strings.TrimSpace(activityID),
		strings.TrimSpace(pluginCode),
		string(jobType),
		scheduledAt.UTC().Format(time.RFC3339Nano),
	}, ":")))
	return "plugin_job_" + hex.EncodeToString(sum[:])[:24]
}

func rawJSONHash(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		sum := sha256.Sum256([]byte(trimmed))
		return hex.EncodeToString(sum[:])
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		normalized = []byte(trimmed)
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:])
}

func sanitizeScheduleError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	replacers := []string{"authorization", "cookie", "token", "secret", "password"}
	lower := strings.ToLower(message)
	for _, marker := range replacers {
		if strings.Contains(lower, marker) {
			return "sensitive error redacted"
		}
	}
	if len(message) > 240 {
		return message[:240]
	}
	return message
}

func scheduleAudit(action string, job PluginScheduleJob, class string, message string, now time.Time) ScheduleAuditRecord {
	return ScheduleAuditRecord{
		Action:     action,
		JobID:      job.JobID,
		TenantID:   job.TenantID,
		ActivityID: job.ActivityID,
		PluginCode: job.PluginCode,
		JobType:    job.JobType,
		Status:     job.Status,
		ErrorClass: class,
		Message:    message,
		CreatedAt:  now.UTC(),
	}
}

type MemoryPluginScheduleRepository struct {
	mu       sync.Mutex
	jobs     map[string]PluginScheduleJob
	unique   map[string]string
	audits   map[string][]ScheduleAuditRecord
	inserted []string
}

func NewMemoryPluginScheduleRepository() *MemoryPluginScheduleRepository {
	return &MemoryPluginScheduleRepository{
		jobs:   map[string]PluginScheduleJob{},
		unique: map[string]string{},
		audits: map[string][]ScheduleAuditRecord{},
	}
}

func (r *MemoryPluginScheduleRepository) Save(ctx context.Context, job PluginScheduleJob) (PluginScheduleJob, bool, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existingID := r.unique[scheduleUniqueKey(job)]; existingID != "" {
		return r.jobs[existingID], true, nil
	}
	if existing, ok := r.jobs[job.JobID]; ok {
		return existing, true, nil
	}
	r.jobs[job.JobID] = job
	r.unique[scheduleUniqueKey(job)] = job.JobID
	r.inserted = append(r.inserted, job.JobID)
	return job, false, nil
}

func (r *MemoryPluginScheduleRepository) Get(ctx context.Context, jobID string) (PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, marketing.ErrNotFound
	}
	return job, nil
}

func (r *MemoryPluginScheduleRepository) ListByActivity(ctx context.Context, tenantID string, activityID string, pluginCode string) ([]PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []PluginScheduleJob
	for _, job := range r.jobs {
		if job.TenantID == tenantID && job.ActivityID == activityID && (pluginCode == "" || job.PluginCode == pluginCode) {
			out = append(out, job)
		}
	}
	slices.SortFunc(out, func(a, b PluginScheduleJob) int {
		return strings.Compare(a.JobID, b.JobID)
	})
	return out, nil
}

func (r *MemoryPluginScheduleRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []PluginScheduleJob
	for _, job := range r.jobs {
		if len(out) >= limit {
			break
		}
		if job.Status == PluginScheduleStatusPending && !job.ScheduledAt.After(now) {
			out = append(out, job)
			continue
		}
		if job.Status == PluginScheduleStatusRetry && !job.NextRetryAt.IsZero() && !job.NextRetryAt.After(now) {
			out = append(out, job)
		}
	}
	return out, nil
}

func (r *MemoryPluginScheduleRepository) Claim(ctx context.Context, jobID string, now time.Time) (PluginScheduleJob, bool, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, false, marketing.ErrNotFound
	}
	if job.Status != PluginScheduleStatusPending && job.Status != PluginScheduleStatusRetry {
		return job, false, nil
	}
	if job.Status == PluginScheduleStatusPending && job.ScheduledAt.After(now) {
		return job, false, nil
	}
	if job.Status == PluginScheduleStatusRetry && (job.NextRetryAt.IsZero() || job.NextRetryAt.After(now)) {
		return job, false, nil
	}
	job.Status = PluginScheduleStatusRunning
	job.LastRunAt = now.UTC()
	job.UpdatedAt = now.UTC()
	r.jobs[jobID] = job
	return job, true, nil
}

func (r *MemoryPluginScheduleRepository) MarkSuccess(ctx context.Context, jobID string, now time.Time) (PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, marketing.ErrNotFound
	}
	if job.Status == PluginScheduleStatusClosed {
		return PluginScheduleJob{}, marketing.ErrInvalidOperation
	}
	job.Status = PluginScheduleStatusSuccess
	job.ErrorClass = ""
	job.ErrorMessage = ""
	job.NextRetryAt = time.Time{}
	job.UpdatedAt = now.UTC()
	r.jobs[jobID] = job
	return job, nil
}

func (r *MemoryPluginScheduleRepository) MarkFailed(ctx context.Context, jobID string, class string, message string, retryable bool, nextRetryAt time.Time, now time.Time) (PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, marketing.ErrNotFound
	}
	if job.Status == PluginScheduleStatusSuccess || job.Status == PluginScheduleStatusClosed {
		return job, nil
	}
	job.RetryCount++
	if retryable {
		job.Status = PluginScheduleStatusRetry
		job.NextRetryAt = nextRetryAt.UTC()
	} else {
		job.Status = PluginScheduleStatusFailed
		job.NextRetryAt = time.Time{}
	}
	job.ErrorClass = class
	job.ErrorMessage = message
	job.UpdatedAt = now.UTC()
	r.jobs[jobID] = job
	return job, nil
}

func (r *MemoryPluginScheduleRepository) Retry(ctx context.Context, jobID string, nextRetryAt time.Time, now time.Time) (PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, marketing.ErrNotFound
	}
	if job.Status != PluginScheduleStatusFailed {
		return PluginScheduleJob{}, marketing.ErrInvalidOperation
	}
	job.Status = PluginScheduleStatusRetry
	job.NextRetryAt = nextRetryAt.UTC()
	job.UpdatedAt = now.UTC()
	r.jobs[jobID] = job
	return job, nil
}

func (r *MemoryPluginScheduleRepository) RecoverRunning(ctx context.Context, staleBefore time.Time, now time.Time, limit int) ([]PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	recovered := make([]PluginScheduleJob, 0)
	for _, jobID := range r.inserted {
		if len(recovered) >= limit {
			break
		}
		job := r.jobs[jobID]
		if job.Status != PluginScheduleStatusRunning || job.LastRunAt.IsZero() || !job.LastRunAt.Before(staleBefore) {
			continue
		}
		job.Status = PluginScheduleStatusRetry
		job.RetryCount++
		job.NextRetryAt = now.UTC()
		job.ErrorClass = SchedulerErrorInternal
		job.ErrorMessage = "running job recovered to retry"
		job.UpdatedAt = now.UTC()
		r.jobs[jobID] = job
		recovered = append(recovered, job)
	}
	return recovered, nil
}

func (r *MemoryPluginScheduleRepository) Close(ctx context.Context, jobID string, operator string, reason string, now time.Time) (PluginScheduleJob, error) {
	if err := ctx.Err(); err != nil {
		return PluginScheduleJob{}, err
	}
	if operator == "" || reason == "" {
		return PluginScheduleJob{}, errors.Join(marketing.ErrInvalidOperation, errors.New("operator and reason are required"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok {
		return PluginScheduleJob{}, marketing.ErrNotFound
	}
	if job.Status == PluginScheduleStatusSuccess || job.Status == PluginScheduleStatusClosed {
		return PluginScheduleJob{}, marketing.ErrInvalidOperation
	}
	job.Status = PluginScheduleStatusClosed
	job.ClosedBy = operator
	job.ClosedReason = reason
	job.UpdatedAt = now.UTC()
	r.jobs[jobID] = job
	return job, nil
}

func (r *MemoryPluginScheduleRepository) AppendAudit(ctx context.Context, record ScheduleAuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits[record.JobID] = append(r.audits[record.JobID], record)
	return nil
}

func (r *MemoryPluginScheduleRepository) ListAudit(ctx context.Context, jobID string) ([]ScheduleAuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.audits[jobID]), nil
}

func scheduleUniqueKey(job PluginScheduleJob) string {
	return strings.Join([]string{
		job.TenantID,
		job.ActivityID,
		job.PluginCode,
		string(job.JobType),
		job.ScheduledAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")
}
