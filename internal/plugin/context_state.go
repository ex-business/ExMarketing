package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

var (
	ErrPluginStateInvalidScope        = errors.New("plugin state invalid scope")
	ErrPluginStateIdempotencyRequired = errors.New("plugin state idempotency key is required")
	ErrPluginStateIdempotencyConflict = errors.New("plugin state idempotency conflict")
	ErrPluginStateExists              = errors.New("plugin state already exists")
	ErrPluginStateVersionConflict     = errors.New("plugin state version conflict")
)

const (
	PluginStateResultCreated       = "created"
	PluginStateResultUpdated       = "updated"
	PluginStateResultAppended      = "appended"
	PluginStateResultDuplicateNoop = "duplicate_noop"
	PluginStateResultConflict      = "conflict"
	PluginStateResultRejected      = "rejected"
)

type PluginContext struct {
	Event       PluginContextEvent     `json:"event"`
	Activity    PluginActivitySnapshot `json:"activity"`
	RuleResults []marketing.ActivityRuleResult
	Facts       map[string]any `json:"facts,omitempty"`
	Now         time.Time      `json:"now"`
	TraceID     string         `json:"traceId"`
}

type PluginContextEvent struct {
	EventID        string                 `json:"eventId"`
	IdempotencyKey string                 `json:"idempotencyKey"`
	EventType      string                 `json:"eventType"`
	SourceSystem   string                 `json:"sourceSystem"`
	TenantID       string                 `json:"tenantId"`
	Subject        marketing.EventSubject `json:"subject"`
	Biz            marketing.EventBiz     `json:"biz"`
	OccurredAt     time.Time              `json:"occurredAt"`
	Attributes     map[string]any         `json:"attributes,omitempty"`
}

type PluginActivitySnapshot struct {
	ActivityID      string          `json:"activityId"`
	ActivityCode    string          `json:"activityCode"`
	ActivityType    string          `json:"activityType"`
	ActivityVersion int             `json:"activityVersion"`
	PluginCode      string          `json:"pluginCode"`
	Config          json.RawMessage `json:"config,omitempty"`
	Rules           []marketing.Rule
	Rewards         []marketing.Reward
}

func NewPluginContext(activity marketing.Activity, event marketing.ActivityEvent, ruleResults []marketing.ActivityRuleResult, facts map[string]any, now time.Time, traceID string) PluginContext {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(traceID) == "" {
		traceID = event.EventID
	}
	return PluginContext{
		Event: PluginContextEvent{
			EventID:        event.EventID,
			IdempotencyKey: event.IdempotencyKey(),
			EventType:      event.EventType,
			SourceSystem:   event.SourceSystem,
			TenantID:       event.TenantID,
			Subject:        event.Subject,
			Biz:            event.Biz,
			OccurredAt:     event.OccurredAt,
			Attributes:     cloneMap(event.Attributes),
		},
		Activity: PluginActivitySnapshot{
			ActivityID:      activity.ID,
			ActivityCode:    activity.Code,
			ActivityType:    activity.Type,
			ActivityVersion: activity.Version,
			PluginCode:      activity.PluginCode,
			Config:          cloneBytes(activity.Config),
			Rules:           cloneRules(activity.Rules),
			Rewards:         cloneRewards(activity.Rewards),
		},
		RuleResults: cloneRuleResults(ruleResults),
		Facts:       cloneMap(facts),
		Now:         now.UTC(),
		TraceID:     traceID,
	}
}

type PluginStateStore interface {
	Get(context.Context, PluginStateScope) (PluginStateRecord, bool, error)
	PutIfAbsent(context.Context, PluginStateScope, map[string]any, PluginStateWriteMeta) (PluginStateWriteResult, error)
	CompareAndSwap(context.Context, PluginStateScope, int64, map[string]any, PluginStateWriteMeta) (PluginStateWriteResult, error)
	AppendEvent(context.Context, PluginStateScope, string, map[string]any, PluginStateWriteMeta) (PluginStateWriteResult, error)
	ListByRound(context.Context, PluginStateRoundQuery) (PluginStateRoundSnapshot, error)
}

type PluginStateScope struct {
	TenantID        string `json:"tenantId"`
	ActivityID      string `json:"activityId"`
	ActivityVersion int    `json:"activityVersion"`
	PluginCode      string `json:"pluginCode"`
	Subject         string `json:"subject"`
	Biz             string `json:"biz"`
	StateKey        string `json:"stateKey"`
}

func NewPluginStateScope(activity marketing.Activity, event marketing.ActivityEvent, stateKey string) PluginStateScope {
	return PluginStateScope{
		TenantID:        event.TenantID,
		ActivityID:      activity.ID,
		ActivityVersion: activity.Version,
		PluginCode:      activity.PluginCode,
		Subject:         SubjectScopeKey(event.Subject),
		Biz:             BizScopeKey(event.Biz),
		StateKey:        strings.TrimSpace(stateKey),
	}
}

func SubjectScopeKey(subject marketing.EventSubject) string {
	return strings.TrimSpace(subject.Type) + ":" + strings.TrimSpace(subject.ID)
}

func BizScopeKey(biz marketing.EventBiz) string {
	return strings.TrimSpace(biz.BizType) + ":" + strings.TrimSpace(biz.BizID)
}

type PluginStateWriteMeta struct {
	EventID        string    `json:"eventId"`
	IdempotencyKey string    `json:"idempotencyKey"`
	TraceID        string    `json:"traceId,omitempty"`
	LifecycleStep  string    `json:"lifecycleStep,omitempty"`
	Now            time.Time `json:"now,omitempty"`
}

type PluginStateRecord struct {
	Scope          PluginStateScope `json:"scope"`
	Value          map[string]any   `json:"value,omitempty"`
	Version        int64            `json:"version"`
	LastEventID    string           `json:"lastEventId,omitempty"`
	IdempotencyKey string           `json:"idempotencyKey,omitempty"`
	TraceID        string           `json:"traceId,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

type PluginStateEvent struct {
	Scope          PluginStateScope `json:"scope"`
	Sequence       int64            `json:"sequence"`
	EventType      string           `json:"eventType"`
	Payload        map[string]any   `json:"payload,omitempty"`
	EventID        string           `json:"eventId"`
	IdempotencyKey string           `json:"idempotencyKey"`
	TraceID        string           `json:"traceId,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
}

type PluginStateAudit struct {
	Scope          PluginStateScope `json:"scope"`
	Operation      string           `json:"operation"`
	Result         string           `json:"result"`
	EventID        string           `json:"eventId,omitempty"`
	IdempotencyKey string           `json:"idempotencyKey,omitempty"`
	TraceID        string           `json:"traceId,omitempty"`
	LifecycleStep  string           `json:"lifecycleStep,omitempty"`
	Before         map[string]any   `json:"before,omitempty"`
	After          map[string]any   `json:"after,omitempty"`
	CreatedAt      time.Time        `json:"createdAt"`
}

type PluginStateWriteResult struct {
	Result string             `json:"result"`
	Record *PluginStateRecord `json:"record,omitempty"`
	Event  *PluginStateEvent  `json:"event,omitempty"`
	Audit  PluginStateAudit   `json:"audit"`
}

type PluginStateRoundQuery struct {
	TenantID        string `json:"tenantId"`
	ActivityID      string `json:"activityId"`
	ActivityVersion int    `json:"activityVersion"`
	PluginCode      string `json:"pluginCode"`
	Biz             string `json:"biz"`
}

type PluginStateRoundSnapshot struct {
	States []PluginStateRecord `json:"states"`
	Events []PluginStateEvent  `json:"events"`
}

type MemoryPluginStateStore struct {
	mu          sync.Mutex
	states      map[string]PluginStateRecord
	events      []PluginStateEvent
	idempotency map[string]memoryStateIdempotency
	audits      []PluginStateAudit
}

type memoryStateIdempotency struct {
	Operation string
	Payload   string
	Result    PluginStateWriteResult
}

func NewMemoryPluginStateStore() *MemoryPluginStateStore {
	return &MemoryPluginStateStore{
		states:      map[string]PluginStateRecord{},
		idempotency: map[string]memoryStateIdempotency{},
	}
}

func (s *MemoryPluginStateStore) Get(_ context.Context, scope PluginStateScope) (PluginStateRecord, bool, error) {
	if err := validatePluginStateScope(scope); err != nil {
		return PluginStateRecord{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.states[scopeKey(scope)]
	if !ok {
		return PluginStateRecord{}, false, nil
	}
	return cloneStateRecord(record), true, nil
}

func (s *MemoryPluginStateStore) PutIfAbsent(_ context.Context, scope PluginStateScope, value map[string]any, meta PluginStateWriteMeta) (PluginStateWriteResult, error) {
	if err := validatePluginStateWrite(scope, meta); err != nil {
		return rejectedStateResult(scope, "PutIfAbsent", meta, err), err
	}
	value, payloadHash, err := cloneMapWithHash(value)
	if err != nil {
		return rejectedStateResult(scope, "PutIfAbsent", meta, err), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if result, handled, err := s.idempotencyResultLocked(scope, "PutIfAbsent", payloadHash, meta); handled {
		return result, err
	}
	key := scopeKey(scope)
	if existing, ok := s.states[key]; ok {
		result := s.conflictResultLocked(scope, "PutIfAbsent", meta, &existing, nil)
		return result, ErrPluginStateExists
	}
	now := writeNow(meta)
	record := PluginStateRecord{
		Scope:          scope,
		Value:          value,
		Version:        1,
		LastEventID:    strings.TrimSpace(meta.EventID),
		IdempotencyKey: strings.TrimSpace(meta.IdempotencyKey),
		TraceID:        strings.TrimSpace(meta.TraceID),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.states[key] = cloneStateRecord(record)
	result := s.recordResultLocked("PutIfAbsent", PluginStateResultCreated, meta, nil, &record)
	s.idempotency[idempotencyKey(scope, meta)] = memoryStateIdempotency{Operation: "PutIfAbsent", Payload: payloadHash, Result: cloneWriteResult(result)}
	return result, nil
}

func (s *MemoryPluginStateStore) CompareAndSwap(_ context.Context, scope PluginStateScope, expectedVersion int64, value map[string]any, meta PluginStateWriteMeta) (PluginStateWriteResult, error) {
	if err := validatePluginStateWrite(scope, meta); err != nil {
		return rejectedStateResult(scope, "CompareAndSwap", meta, err), err
	}
	if expectedVersion < 1 {
		err := fmt.Errorf("%w: expectedVersion must be positive", ErrPluginStateVersionConflict)
		return rejectedStateResult(scope, "CompareAndSwap", meta, err), err
	}
	value, payloadHash, err := cloneMapWithHash(value)
	if err != nil {
		return rejectedStateResult(scope, "CompareAndSwap", meta, err), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if result, handled, err := s.idempotencyResultLocked(scope, "CompareAndSwap", payloadHash, meta); handled {
		return result, err
	}
	key := scopeKey(scope)
	existing, ok := s.states[key]
	if !ok || existing.Version != expectedVersion {
		var before *PluginStateRecord
		if ok {
			before = &existing
		}
		result := s.conflictResultLocked(scope, "CompareAndSwap", meta, before, nil)
		return result, ErrPluginStateVersionConflict
	}
	now := writeNow(meta)
	next := existing
	next.Value = value
	next.Version++
	next.LastEventID = strings.TrimSpace(meta.EventID)
	next.IdempotencyKey = strings.TrimSpace(meta.IdempotencyKey)
	next.TraceID = strings.TrimSpace(meta.TraceID)
	next.UpdatedAt = now
	s.states[key] = cloneStateRecord(next)
	result := s.recordResultLocked("CompareAndSwap", PluginStateResultUpdated, meta, &existing, &next)
	s.idempotency[idempotencyKey(scope, meta)] = memoryStateIdempotency{Operation: "CompareAndSwap", Payload: payloadHash, Result: cloneWriteResult(result)}
	return result, nil
}

func (s *MemoryPluginStateStore) AppendEvent(_ context.Context, scope PluginStateScope, eventType string, payload map[string]any, meta PluginStateWriteMeta) (PluginStateWriteResult, error) {
	if err := validatePluginStateWrite(scope, meta); err != nil {
		return rejectedStateResult(scope, "AppendEvent", meta, err), err
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		err := fmt.Errorf("%w: eventType is required", ErrPluginStateInvalidScope)
		return rejectedStateResult(scope, "AppendEvent", meta, err), err
	}
	payload, payloadHash, err := cloneMapWithHash(payload)
	if err != nil {
		return rejectedStateResult(scope, "AppendEvent", meta, err), err
	}
	payloadHash = eventType + ":" + payloadHash
	s.mu.Lock()
	defer s.mu.Unlock()

	if result, handled, err := s.idempotencyResultLocked(scope, "AppendEvent", payloadHash, meta); handled {
		return result, err
	}
	event := PluginStateEvent{
		Scope:          scope,
		Sequence:       int64(len(s.events) + 1),
		EventType:      eventType,
		Payload:        payload,
		EventID:        strings.TrimSpace(meta.EventID),
		IdempotencyKey: strings.TrimSpace(meta.IdempotencyKey),
		TraceID:        strings.TrimSpace(meta.TraceID),
		CreatedAt:      writeNow(meta),
	}
	s.events = append(s.events, cloneStateEvent(event))
	result := s.eventResultLocked("AppendEvent", PluginStateResultAppended, meta, event)
	s.idempotency[idempotencyKey(scope, meta)] = memoryStateIdempotency{Operation: "AppendEvent", Payload: payloadHash, Result: cloneWriteResult(result)}
	return result, nil
}

func (s *MemoryPluginStateStore) ListByRound(_ context.Context, query PluginStateRoundQuery) (PluginStateRoundSnapshot, error) {
	if err := validatePluginStateRoundQuery(query); err != nil {
		return PluginStateRoundSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out PluginStateRoundSnapshot
	for _, state := range s.states {
		if roundMatches(query, state.Scope) {
			out.States = append(out.States, cloneStateRecord(state))
		}
	}
	for _, event := range s.events {
		if roundMatches(query, event.Scope) {
			out.Events = append(out.Events, cloneStateEvent(event))
		}
	}
	sort.Slice(out.States, func(i, j int) bool {
		if out.States[i].Scope.Subject == out.States[j].Scope.Subject {
			return out.States[i].Scope.StateKey < out.States[j].Scope.StateKey
		}
		return out.States[i].Scope.Subject < out.States[j].Scope.Subject
	})
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].Sequence < out.Events[j].Sequence })
	return out, nil
}

func (s *MemoryPluginStateStore) Audits() []PluginStateAudit {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PluginStateAudit, 0, len(s.audits))
	for _, audit := range s.audits {
		out = append(out, cloneStateAudit(audit))
	}
	return out
}

func (s *MemoryPluginStateStore) idempotencyResultLocked(scope PluginStateScope, operation string, payloadHash string, meta PluginStateWriteMeta) (PluginStateWriteResult, bool, error) {
	existing, ok := s.idempotency[idempotencyKey(scope, meta)]
	if !ok {
		return PluginStateWriteResult{}, false, nil
	}
	if existing.Operation != operation || existing.Payload != payloadHash {
		result := s.conflictResultLocked(scope, operation, meta, nil, nil)
		return result, true, ErrPluginStateIdempotencyConflict
	}
	result := cloneWriteResult(existing.Result)
	result.Result = PluginStateResultDuplicateNoop
	result.Audit.Result = PluginStateResultDuplicateNoop
	return result, true, nil
}

func (s *MemoryPluginStateStore) recordResultLocked(operation string, status string, meta PluginStateWriteMeta, before *PluginStateRecord, after *PluginStateRecord) PluginStateWriteResult {
	var scope PluginStateScope
	if after != nil {
		scope = after.Scope
	} else if before != nil {
		scope = before.Scope
	}
	result := PluginStateWriteResult{
		Result: status,
		Audit:  writeAudit(scope, operation, status, meta, beforeSummary(before), beforeSummary(after)),
	}
	if after != nil {
		record := cloneStateRecord(*after)
		result.Record = &record
	}
	s.audits = append(s.audits, cloneStateAudit(result.Audit))
	return result
}

func (s *MemoryPluginStateStore) eventResultLocked(operation string, status string, meta PluginStateWriteMeta, event PluginStateEvent) PluginStateWriteResult {
	result := PluginStateWriteResult{
		Result: status,
		Event:  &event,
		Audit: writeAudit(event.Scope, operation, status, meta, nil, map[string]any{
			"sequence":    event.Sequence,
			"eventType":   event.EventType,
			"payloadHash": payloadHash(event.Payload),
		}),
	}
	copied := cloneStateEvent(event)
	result.Event = &copied
	s.audits = append(s.audits, cloneStateAudit(result.Audit))
	return result
}

func (s *MemoryPluginStateStore) conflictResultLocked(scope PluginStateScope, operation string, meta PluginStateWriteMeta, before *PluginStateRecord, after *PluginStateRecord) PluginStateWriteResult {
	result := PluginStateWriteResult{
		Result: PluginStateResultConflict,
		Audit:  writeAudit(scope, operation, PluginStateResultConflict, meta, beforeSummary(before), beforeSummary(after)),
	}
	if before != nil {
		record := cloneStateRecord(*before)
		result.Record = &record
	}
	s.audits = append(s.audits, cloneStateAudit(result.Audit))
	return result
}

func validatePluginStateWrite(scope PluginStateScope, meta PluginStateWriteMeta) error {
	if err := validatePluginStateScope(scope); err != nil {
		return err
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		return ErrPluginStateIdempotencyRequired
	}
	return nil
}

func validatePluginStateScope(scope PluginStateScope) error {
	missing := []string{}
	if strings.TrimSpace(scope.TenantID) == "" {
		missing = append(missing, "tenantId")
	}
	if strings.TrimSpace(scope.ActivityID) == "" {
		missing = append(missing, "activityId")
	}
	if scope.ActivityVersion <= 0 {
		missing = append(missing, "activityVersion")
	}
	if strings.TrimSpace(scope.PluginCode) == "" {
		missing = append(missing, "pluginCode")
	}
	if strings.TrimSpace(scope.Subject) == "" || strings.Trim(scope.Subject, ":") == "" {
		missing = append(missing, "subject")
	}
	if strings.TrimSpace(scope.Biz) == "" || strings.Trim(scope.Biz, ":") == "" {
		missing = append(missing, "biz")
	}
	if strings.TrimSpace(scope.StateKey) == "" {
		missing = append(missing, "stateKey")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrPluginStateInvalidScope, strings.Join(missing, ","))
	}
	return nil
}

func validatePluginStateRoundQuery(query PluginStateRoundQuery) error {
	scope := PluginStateScope{
		TenantID:        query.TenantID,
		ActivityID:      query.ActivityID,
		ActivityVersion: query.ActivityVersion,
		PluginCode:      query.PluginCode,
		Subject:         "round:list",
		Biz:             query.Biz,
		StateKey:        "round",
	}
	return validatePluginStateScope(scope)
}

func rejectedStateResult(scope PluginStateScope, operation string, meta PluginStateWriteMeta, err error) PluginStateWriteResult {
	return PluginStateWriteResult{
		Result: PluginStateResultRejected,
		Audit:  writeAudit(scope, operation, PluginStateResultRejected, meta, nil, map[string]any{"error": err.Error()}),
	}
}

func writeAudit(scope PluginStateScope, operation string, result string, meta PluginStateWriteMeta, before map[string]any, after map[string]any) PluginStateAudit {
	return PluginStateAudit{
		Scope:          scope,
		Operation:      operation,
		Result:         result,
		EventID:        strings.TrimSpace(meta.EventID),
		IdempotencyKey: strings.TrimSpace(meta.IdempotencyKey),
		TraceID:        strings.TrimSpace(meta.TraceID),
		LifecycleStep:  strings.TrimSpace(meta.LifecycleStep),
		Before:         before,
		After:          after,
		CreatedAt:      writeNow(meta),
	}
}

func writeNow(meta PluginStateWriteMeta) time.Time {
	if meta.Now.IsZero() {
		return time.Now().UTC()
	}
	return meta.Now.UTC()
}

func scopeKey(scope PluginStateScope) string {
	return strings.Join([]string{
		scope.TenantID,
		scope.ActivityID,
		fmt.Sprint(scope.ActivityVersion),
		scope.PluginCode,
		scope.Subject,
		scope.Biz,
		scope.StateKey,
	}, "\x00")
}

func idempotencyKey(scope PluginStateScope, meta PluginStateWriteMeta) string {
	return strings.Join([]string{
		scopeKey(scope),
		strings.TrimSpace(meta.IdempotencyKey),
		scope.PluginCode,
		scope.StateKey,
	}, "\x00")
}

func roundMatches(query PluginStateRoundQuery, scope PluginStateScope) bool {
	return scope.TenantID == strings.TrimSpace(query.TenantID) &&
		scope.ActivityID == strings.TrimSpace(query.ActivityID) &&
		scope.ActivityVersion == query.ActivityVersion &&
		scope.PluginCode == strings.TrimSpace(query.PluginCode) &&
		scope.Biz == strings.TrimSpace(query.Biz)
}

func cloneStateRecord(record PluginStateRecord) PluginStateRecord {
	record.Value = cloneMap(record.Value)
	return record
}

func cloneStateEvent(event PluginStateEvent) PluginStateEvent {
	event.Payload = cloneMap(event.Payload)
	return event
}

func cloneStateAudit(audit PluginStateAudit) PluginStateAudit {
	audit.Before = cloneMap(audit.Before)
	audit.After = cloneMap(audit.After)
	return audit
}

func cloneWriteResult(result PluginStateWriteResult) PluginStateWriteResult {
	result.Audit = cloneStateAudit(result.Audit)
	if result.Record != nil {
		record := cloneStateRecord(*result.Record)
		result.Record = &record
	}
	if result.Event != nil {
		event := cloneStateEvent(*result.Event)
		result.Event = &event
	}
	return result
}

func beforeSummary(record *PluginStateRecord) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"version":   record.Version,
		"valueHash": payloadHash(record.Value),
	}
}

func payloadHash(value map[string]any) string {
	_, hash, err := cloneMapWithHash(value)
	if err != nil {
		return "unavailable"
	}
	return hash
}

func cloneMapWithHash(value map[string]any) (map[string]any, string, error) {
	if value == nil {
		value = map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(raw)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", err
	}
	return out, hex.EncodeToString(hash[:]), nil
}

func cloneMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	out, _, err := cloneMapWithHash(value)
	if err != nil {
		return nil
	}
	return out
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}

func cloneRules(value []marketing.Rule) []marketing.Rule {
	if len(value) == 0 {
		return nil
	}
	out := make([]marketing.Rule, len(value))
	copy(out, value)
	for i := range out {
		out[i].Config = cloneBytes(out[i].Config)
	}
	return out
}

func cloneRewards(value []marketing.Reward) []marketing.Reward {
	if len(value) == 0 {
		return nil
	}
	out := make([]marketing.Reward, len(value))
	copy(out, value)
	for i := range out {
		out[i].Config = cloneBytes(out[i].Config)
	}
	return out
}

func cloneRuleResults(value []marketing.ActivityRuleResult) []marketing.ActivityRuleResult {
	if len(value) == 0 {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out []marketing.ActivityRuleResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
