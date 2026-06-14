package marketing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

type MemoryRepository struct {
	mu                      sync.RWMutex
	workspaces              []Workspace
	workspaceMembers        []WorkspaceMember
	workspaceFolders        []WorkspaceFolder
	workspaceShareLinks     []WorkspaceShareLink
	workspaceActivityGrants []WorkspaceActivityGrant
	workspaceFileObjects    []WorkspaceFileObject
	workspaceAuditLogs      []WorkspaceAuditLog
	activities              []Activity
	workflowDrafts          map[string]WorkflowDraft
	workflowTemplates       []WorkflowDraftTemplate
	workflowVersions        []WorkflowVersion
	workflowExecutions      []WorkflowExecution
	nodeExecutions          []WorkflowNodeExecution
	factProviders           map[string]FactProvider
	activityStates          []ActivityState
	storageStates           []StorageState
	inboxByIdempotency      map[string]EventInbox
	inboxByEventIdentity    map[string]EventInbox
	participations          []Participation
	grantOrders             []GrantOrder
	quotaReservations       []QuotaReservation
	outboxMessages          []OutboxMessage
	quotas                  []Quota
	manualExportFiles       []ManualGrantExportFile
	manualExportDetails     []ManualGrantExportDetail
	auditRecords            []AuditRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		inboxByIdempotency:   make(map[string]EventInbox),
		inboxByEventIdentity: make(map[string]EventInbox),
		factProviders:        make(map[string]FactProvider),
		workflowDrafts:       make(map[string]WorkflowDraft),
	}
}

func (r *MemoryRepository) CreateActivity(ctx context.Context, activity Activity) (Activity, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.activities {
		if sameActivityCode(existing, activity) {
			return Activity{}, ErrActivityConflict
		}
	}
	r.activities = append(r.activities, activity)
	return activity, nil
}

func (r *MemoryRepository) GetActivity(ctx context.Context, id string) (Activity, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, activity := range r.activities {
		if activity.ID == id {
			return activity, nil
		}
	}
	return Activity{}, ErrNotFound
}

func (r *MemoryRepository) ListActivities(ctx context.Context) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := slices.Clone(r.activities)
	return out, nil
}

func (r *MemoryRepository) UpdateActivity(ctx context.Context, activity Activity) (Activity, error) {
	if err := ctx.Err(); err != nil {
		return Activity{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.activities {
		if r.activities[i].ID == activity.ID {
			if r.activities[i].TenantID != activity.TenantID || r.activities[i].Code != activity.Code || r.activities[i].Type != activity.Type {
				return Activity{}, ErrInvalidActivity
			}
			r.activities[i] = activity
			return activity, nil
		}
	}
	return Activity{}, ErrNotFound
}

func (r *MemoryRepository) FindMatchingActivities(ctx context.Context, tenantID string, eventType string) ([]Activity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Activity
	for _, activity := range r.activities {
		if activity.TenantID != tenantID {
			continue
		}
		if !slices.Contains(activity.TriggerEvents, eventType) {
			continue
		}
		out = append(out, activity)
	}
	slices.SortFunc(out, func(a, b Activity) int {
		return b.Priority - a.Priority
	})
	return out, nil
}

func sameActivityCode(a Activity, b Activity) bool {
	return strings.TrimSpace(a.TenantID) == strings.TrimSpace(b.TenantID) &&
		strings.TrimSpace(a.Code) == strings.TrimSpace(b.Code)
}

func (r *MemoryRepository) SaveWorkflowDraft(ctx context.Context, draft WorkflowDraft) (WorkflowDraft, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowDraft{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.workflowDrafts[draft.ActivityID]; ok {
		draft.CreatedAt = existing.CreatedAt
	}
	r.workflowDrafts[draft.ActivityID] = draft
	return draft, nil
}

func (r *MemoryRepository) GetWorkflowDraft(ctx context.Context, activityID string) (WorkflowDraft, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowDraft{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	draft, ok := r.workflowDrafts[activityID]
	if !ok {
		return WorkflowDraft{}, ErrNotFound
	}
	return draft, nil
}

func (r *MemoryRepository) CreateWorkflowDraftTemplate(ctx context.Context, template WorkflowDraftTemplate) (WorkflowDraftTemplate, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowDraftTemplate{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if template.Version <= 0 {
		template.Version = 1
		for _, existing := range r.workflowTemplates {
			if existing.ID == template.ID && existing.Version >= template.Version {
				template.Version = existing.Version + 1
			}
		}
	}
	r.workflowTemplates = append(r.workflowTemplates, template)
	return template, nil
}

func (r *MemoryRepository) ListWorkflowDraftTemplates(ctx context.Context, query WorkflowDraftTemplateQuery) ([]WorkflowDraftTemplate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]WorkflowDraftTemplate, 0, len(r.workflowTemplates))
	for _, template := range r.workflowTemplates {
		if query.TemplateID != "" && template.ID != query.TemplateID {
			continue
		}
		if query.Source != "" && template.Source != query.Source {
			continue
		}
		if query.Status != "" && template.Status != query.Status {
			continue
		}
		if query.WorkspaceID != "" && template.WorkspaceID != query.WorkspaceID {
			continue
		}
		if query.FolderID != "" && template.FolderID != query.FolderID {
			continue
		}
		if query.Visibility != "" && template.Visibility != query.Visibility {
			continue
		}
		items = append(items, template)
	}
	slices.SortFunc(items, func(a, b WorkflowDraftTemplate) int {
		if a.ID != b.ID {
			return strings.Compare(a.ID, b.ID)
		}
		return b.Version - a.Version
	})
	if query.LatestOnly {
		latest := make([]WorkflowDraftTemplate, 0, len(items))
		seen := map[string]struct{}{}
		for _, item := range items {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			latest = append(latest, item)
		}
		items = latest
	}
	if query.Limit > 0 && len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

func (r *MemoryRepository) GetWorkflowDraftTemplate(ctx context.Context, templateID string, versionID string, version int) (WorkflowDraftTemplate, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowDraftTemplate{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest WorkflowDraftTemplate
	for _, template := range r.workflowTemplates {
		if template.ID != templateID {
			continue
		}
		if versionID != "" && template.VersionID != versionID {
			continue
		}
		if version > 0 && template.Version != version {
			continue
		}
		if versionID != "" || version > 0 {
			return template, nil
		}
		if latest.ID == "" || template.Version > latest.Version {
			latest = template
		}
	}
	if latest.ID == "" {
		return WorkflowDraftTemplate{}, ErrNotFound
	}
	return latest, nil
}

func (r *MemoryRepository) UpdateWorkflowDraftTemplate(ctx context.Context, template WorkflowDraftTemplate) (WorkflowDraftTemplate, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowDraftTemplate{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workflowTemplates {
		if r.workflowTemplates[i].VersionID == template.VersionID {
			r.workflowTemplates[i] = template
			return template, nil
		}
	}
	return WorkflowDraftTemplate{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkflowVersion(ctx context.Context, version WorkflowVersion) (WorkflowVersion, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if version.Version == 0 {
		version.Version = 1
		for _, existing := range r.workflowVersions {
			if existing.ActivityID == version.ActivityID && existing.Version >= version.Version {
				version.Version = existing.Version + 1
			}
		}
	}
	if version.Status == WorkflowVersionActive {
		for i := range r.workflowVersions {
			if r.workflowVersions[i].ActivityID == version.ActivityID && r.workflowVersions[i].Status == WorkflowVersionActive {
				r.workflowVersions[i].Status = WorkflowVersionSuperseded
			}
		}
	}
	r.workflowVersions = append(r.workflowVersions, version)
	return version, nil
}

func (r *MemoryRepository) GetActiveWorkflowVersion(ctx context.Context, activityID string) (WorkflowVersion, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowVersion{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := len(r.workflowVersions) - 1; i >= 0; i-- {
		version := r.workflowVersions[i]
		if version.ActivityID == activityID && version.Status == WorkflowVersionActive {
			return version, nil
		}
	}
	return WorkflowVersion{}, ErrNotFound
}

func (r *MemoryRepository) GetWorkflowVersion(ctx context.Context, activityID string, versionID string) (WorkflowVersion, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowVersion{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, version := range r.workflowVersions {
		if version.ActivityID == activityID && version.ID == versionID {
			return version, nil
		}
	}
	return WorkflowVersion{}, ErrNotFound
}

func (r *MemoryRepository) ListWorkflowVersions(ctx context.Context, activityID string) ([]WorkflowVersion, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []WorkflowVersion
	for _, version := range r.workflowVersions {
		if version.ActivityID == activityID {
			out = append(out, version)
		}
	}
	slices.SortFunc(out, func(a, b WorkflowVersion) int {
		return b.Version - a.Version
	})
	return out, nil
}

func (r *MemoryRepository) UpdateWorkflowVersionStatus(ctx context.Context, activityID string, versionID string, status WorkflowVersionStatus) (WorkflowVersion, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workflowVersions {
		if r.workflowVersions[i].ActivityID == activityID && r.workflowVersions[i].ID == versionID {
			r.workflowVersions[i].Status = status
			return r.workflowVersions[i], nil
		}
	}
	return WorkflowVersion{}, ErrNotFound
}

func (r *MemoryRepository) CreateWorkflowExecution(ctx context.Context, execution WorkflowExecution) (WorkflowExecution, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowExecution{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workflowExecutions = append(r.workflowExecutions, execution)
	return execution, nil
}

func (r *MemoryRepository) UpdateWorkflowExecution(ctx context.Context, execution WorkflowExecution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.workflowExecutions {
		if r.workflowExecutions[i].ID == execution.ID {
			r.workflowExecutions[i] = execution
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) CreateWorkflowNodeExecution(ctx context.Context, execution WorkflowNodeExecution) (WorkflowNodeExecution, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowNodeExecution{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodeExecutions = append(r.nodeExecutions, execution)
	return execution, nil
}

func (r *MemoryRepository) UpdateWorkflowNodeExecution(ctx context.Context, execution WorkflowNodeExecution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.nodeExecutions {
		if r.nodeExecutions[i].ID == execution.ID {
			r.nodeExecutions[i] = execution
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) ListWorkflowExecutions(ctx context.Context, query WorkflowExecutionQuery) ([]WorkflowExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []WorkflowExecution
	for _, execution := range r.workflowExecutions {
		if query.TenantID != "" && execution.TenantID != query.TenantID {
			continue
		}
		if query.ActivityID != "" && execution.ActivityID != query.ActivityID {
			continue
		}
		if query.WorkflowVersionID != "" && execution.WorkflowVersionID != query.WorkflowVersionID {
			continue
		}
		if query.EventID != "" && execution.EventID != query.EventID {
			continue
		}
		if query.ExecutionID != "" && execution.ID != query.ExecutionID {
			continue
		}
		out = append(out, execution)
	}
	slices.SortFunc(out, func(a, b WorkflowExecution) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	if len(out) > limit {
		offset := query.Offset
		if offset < 0 {
			offset = 0
		}
		if offset >= len(out) {
			return nil, nil
		}
		end := offset + limit
		if end > len(out) {
			end = len(out)
		}
		out = out[offset:end]
	} else if query.Offset > 0 {
		if query.Offset >= len(out) {
			return nil, nil
		}
		out = out[query.Offset:]
	}
	return out, nil
}

func (r *MemoryRepository) ListWorkflowNodeExecutions(ctx context.Context, query WorkflowNodeExecutionQuery) ([]WorkflowNodeExecution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []WorkflowNodeExecution
	for _, execution := range r.nodeExecutions {
		if execution.ExecutionID == query.ExecutionID {
			out = append(out, execution)
		}
	}
	slices.SortFunc(out, func(a, b WorkflowNodeExecution) int {
		return a.StartedAt.Compare(b.StartedAt)
	})
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Limit > 0 {
		if query.Offset >= len(out) {
			return nil, nil
		}
		end := query.Offset + query.Limit
		if end > len(out) {
			end = len(out)
		}
		out = out[query.Offset:end]
	} else if query.Offset > 0 {
		if query.Offset >= len(out) {
			return nil, nil
		}
		out = out[query.Offset:]
	}
	return out, nil
}

func (r *MemoryRepository) SaveFactProvider(ctx context.Context, provider FactProvider) (FactProvider, error) {
	if err := ctx.Err(); err != nil {
		return FactProvider{}, err
	}
	if err := provider.Validate(); err != nil {
		return FactProvider{}, err
	}
	now := time.Now().UTC()
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = now
	}
	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = now
	}
	provider.ProviderType = strings.ToUpper(strings.TrimSpace(provider.ProviderType))
	provider.Method = strings.ToUpper(strings.TrimSpace(provider.Method))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factProviders[factProviderKey(provider.TenantID, provider.FactCode)] = provider
	return provider, nil
}

func (r *MemoryRepository) FindEnabledFactProvider(ctx context.Context, tenantID string, factCode string) (FactProvider, error) {
	if err := ctx.Err(); err != nil {
		return FactProvider{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.factProviders[factProviderKey(tenantID, factCode)]
	if !ok {
		return FactProvider{}, ErrNotFound
	}
	if !provider.Enabled {
		return FactProvider{}, ErrFactProviderDisabled
	}
	if err := provider.Validate(); err != nil {
		return FactProvider{}, err
	}
	return provider, nil
}

func factProviderKey(tenantID string, factCode string) string {
	return strings.TrimSpace(tenantID) + "\x00" + strings.TrimSpace(factCode)
}

func sameActivityStateKey(state ActivityState, key ActivityStateKey) bool {
	return strings.TrimSpace(state.TenantID) == strings.TrimSpace(key.TenantID) &&
		strings.TrimSpace(state.ActivityID) == strings.TrimSpace(key.ActivityID) &&
		strings.TrimSpace(state.Namespace) == strings.TrimSpace(key.Namespace) &&
		strings.TrimSpace(state.StateKey) == strings.TrimSpace(key.StateKey)
}

func cloneActivityState(state ActivityState) ActivityState {
	if len(state.Data) > 0 {
		state.Data = append(json.RawMessage(nil), state.Data...)
	}
	return state
}

func mergeActivityStateData(existing json.RawMessage, patch json.RawMessage) json.RawMessage {
	var existingMap map[string]any
	var patchMap map[string]any
	if json.Unmarshal(existing, &existingMap) != nil || json.Unmarshal(patch, &patchMap) != nil {
		return append(json.RawMessage(nil), patch...)
	}
	if existingMap == nil {
		existingMap = map[string]any{}
	}
	for key, value := range patchMap {
		existingMap[key] = value
	}
	raw, err := json.Marshal(existingMap)
	if err != nil {
		return append(json.RawMessage(nil), patch...)
	}
	return raw
}

func sameStorageStateKey(state StorageState, key StorageStateKey) bool {
	return strings.TrimSpace(state.TenantID) == strings.TrimSpace(key.TenantID) &&
		strings.TrimSpace(state.Namespace) == strings.TrimSpace(key.Namespace) &&
		strings.TrimSpace(state.StorageKey) == strings.TrimSpace(key.StorageKey)
}

func cloneStorageState(state StorageState) StorageState {
	if len(state.Data) > 0 {
		state.Data = append(json.RawMessage(nil), state.Data...)
	}
	return state
}

func (r *MemoryRepository) GetActivityState(ctx context.Context, key ActivityStateKey) (ActivityState, error) {
	if err := ctx.Err(); err != nil {
		return ActivityState{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, state := range r.activityStates {
		if sameActivityStateKey(state, key) {
			return cloneActivityState(state), nil
		}
	}
	return ActivityState{}, ErrNotFound
}

func (r *MemoryRepository) UpsertActivityState(ctx context.Context, cmd UpsertActivityStateCommand) (ActivityState, error) {
	if err := ctx.Err(); err != nil {
		return ActivityState{}, err
	}
	state := cloneActivityState(cmd.State)
	if state.ID == "" {
		state.ID = NewID("actstate")
	}
	if len(state.Data) == 0 {
		state.Data = json.RawMessage(`{}`)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := ActivityStateKey{TenantID: state.TenantID, ActivityID: state.ActivityID, Namespace: state.Namespace, StateKey: state.StateKey}
	for i := range r.activityStates {
		if !sameActivityStateKey(r.activityStates[i], key) {
			continue
		}
		if cmd.RequireVersion && r.activityStates[i].Version != cmd.ExpectedVersion {
			return ActivityState{}, ErrActivityStateConflict
		}
		data := state.Data
		if cmd.Merge {
			data = mergeActivityStateData(r.activityStates[i].Data, state.Data)
		}
		r.activityStates[i].Data = data
		r.activityStates[i].Subject = state.Subject
		r.activityStates[i].LastEventID = state.LastEventID
		r.activityStates[i].ExpiresAt = state.ExpiresAt
		r.activityStates[i].Version++
		r.activityStates[i].UpdatedAt = state.UpdatedAt
		return cloneActivityState(r.activityStates[i]), nil
	}
	if cmd.RequireVersion && cmd.ExpectedVersion > 0 {
		return ActivityState{}, ErrNotFound
	}
	state.Version = 1
	r.activityStates = append(r.activityStates, state)
	return cloneActivityState(state), nil
}

func (r *MemoryRepository) GetStorageState(ctx context.Context, key StorageStateKey) (StorageState, error) {
	if err := ctx.Err(); err != nil {
		return StorageState{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, state := range r.storageStates {
		if sameStorageStateKey(state, key) {
			return cloneStorageState(state), nil
		}
	}
	return StorageState{}, ErrNotFound
}

func (r *MemoryRepository) UpsertStorageState(ctx context.Context, cmd UpsertStorageStateCommand) (StorageState, error) {
	if err := ctx.Err(); err != nil {
		return StorageState{}, err
	}
	state := cloneStorageState(cmd.State)
	if state.ID == "" {
		state.ID = NewID("store")
	}
	if len(state.Data) == 0 {
		state.Data = json.RawMessage(`null`)
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now().UTC()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := StorageStateKey{TenantID: state.TenantID, Namespace: state.Namespace, StorageKey: state.StorageKey}
	for i := range r.storageStates {
		if !sameStorageStateKey(r.storageStates[i], key) {
			continue
		}
		if cmd.RequireVersion && r.storageStates[i].Version != cmd.ExpectedVersion {
			return StorageState{}, ErrStorageStateConflict
		}
		if cmd.Append {
			state.Data = AppendStorageStateData(r.storageStates[i].Data, state.Data)
			state.ValueType = "ARRAY"
		}
		r.storageStates[i].Data = state.Data
		r.storageStates[i].ValueType = state.ValueType
		r.storageStates[i].LastEventID = state.LastEventID
		r.storageStates[i].ExpiresAt = state.ExpiresAt
		r.storageStates[i].Version++
		r.storageStates[i].UpdatedAt = state.UpdatedAt
		return cloneStorageState(r.storageStates[i]), nil
	}
	if cmd.RequireVersion && cmd.ExpectedVersion > 0 {
		return StorageState{}, ErrNotFound
	}
	if cmd.Append {
		state.Data = AppendStorageStateData(nil, state.Data)
		state.ValueType = "ARRAY"
	}
	state.Version = 1
	r.storageStates = append(r.storageStates, state)
	return cloneStorageState(state), nil
}

func (r *MemoryRepository) DeleteStorageState(ctx context.Context, key StorageStateKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.storageStates {
		if sameStorageStateKey(r.storageStates[i], key) {
			r.storageStates = append(r.storageStates[:i], r.storageStates[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) CreateInbox(ctx context.Context, inbox EventInbox) (EventInbox, bool, error) {
	if err := ctx.Err(); err != nil {
		return EventInbox{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	eventKey := inboxEventIdentityKey(inbox.Event)
	if existing, ok := r.inboxByEventIdentity[eventKey]; ok {
		existing.ProcessStatus = InboxDuplicate
		return existing, true, nil
	}
	existing, ok := r.inboxByIdempotency[inbox.IdempotencyKey]
	if ok {
		if !existing.Event.SameStandardPayload(inbox.Event) {
			return EventInbox{}, false, ErrIdempotencyConflict
		}
		existing.ProcessStatus = InboxDuplicate
		return existing, true, nil
	}
	r.inboxByIdempotency[inbox.IdempotencyKey] = inbox
	r.inboxByEventIdentity[eventKey] = inbox
	return inbox, false, nil
}

func (r *MemoryRepository) GetInboxByEvent(ctx context.Context, tenantID string, eventID string) (EventInbox, error) {
	if err := ctx.Err(); err != nil {
		return EventInbox{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	inbox, ok := r.inboxByEventIdentity[strings.Join([]string{strings.TrimSpace(tenantID), strings.TrimSpace(eventID)}, ":")]
	if !ok {
		return EventInbox{}, ErrNotFound
	}
	return inbox, nil
}

func inboxEventIdentityKey(event ActivityEvent) string {
	return strings.Join([]string{
		strings.TrimSpace(event.TenantID),
		strings.TrimSpace(event.EventID),
	}, ":")
}

func (r *MemoryRepository) UpdateInbox(ctx context.Context, inbox EventInbox) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inboxByIdempotency[inbox.IdempotencyKey] = inbox
	return nil
}

func (r *MemoryRepository) CreateParticipation(ctx context.Context, participation Participation) (Participation, error) {
	if err := ctx.Err(); err != nil {
		return Participation{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.participations {
		if existing.IdempotencyKey != "" && existing.IdempotencyKey == participation.IdempotencyKey {
			return Participation{}, ErrIdempotencyConflict
		}
	}
	r.participations = append(r.participations, participation)
	return participation, nil
}

func (r *MemoryRepository) CreateGrantOrder(ctx context.Context, order GrantOrder) (GrantOrder, error) {
	if err := ctx.Err(); err != nil {
		return GrantOrder{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.grantOrders {
		if existing.IdempotencyKey != "" && existing.IdempotencyKey == order.IdempotencyKey {
			return GrantOrder{}, ErrIdempotencyConflict
		}
	}
	r.grantOrders = append(r.grantOrders, order)
	return order, nil
}

func (r *MemoryRepository) UpdateGrantOrderByCommandID(ctx context.Context, tenantID string, commandID string, status GrantStatus, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.grantOrders {
		if r.grantOrders[i].TenantID == tenantID && r.grantOrders[i].BillNo == commandID {
			current := r.grantOrders[i].GrantStatus
			if current == GrantSuccess || current == GrantFailed || current == GrantClosed {
				return ErrCallbackConflict
			}
			if status == GrantSending && current != GrantPending && current != GrantRetry {
				return ErrCallbackConflict
			}
			if status == GrantRetry && current != GrantSending {
				return ErrCallbackConflict
			}
			r.grantOrders[i].GrantStatus = status
			r.grantOrders[i].ErrorMessage = message
			if status == GrantRetry {
				r.grantOrders[i].RetryCount++
			}
			r.grantOrders[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) ApplyRewardCallback(ctx context.Context, callback RewardCallback) (RewardCallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return RewardCallbackResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	orderIndex := r.rewardCallbackOrderIndexLocked(callback)
	if orderIndex == -2 {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if orderIndex < 0 {
		return RewardCallbackResult{}, ErrNotFound
	}

	order := &r.grantOrders[orderIndex]
	if mode := grantModeFromPayload(order.RewardPayload); mode == GrantManual || mode == GrantManualExport {
		return RewardCallbackResult{}, ErrCallbackConflict
	}
	if callback.CommandID != "" && order.BillNo != callback.CommandID {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if callback.IdempotencyKey != "" && order.IdempotencyKey != callback.IdempotencyKey {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if order.GrantStatus == GrantClosed {
		return RewardCallbackResult{}, ErrGrantOrderClosed
	}
	if order.GrantStatus == GrantSuccess || order.GrantStatus == GrantFailed {
		if order.GrantStatus != callback.Status || (order.ExternalGrantID != "" && callback.ExternalGrantID != "" && order.ExternalGrantID != callback.ExternalGrantID) {
			return RewardCallbackResult{}, ErrCallbackConflict
		}
		return RewardCallbackResult{
			CommandID:           order.BillNo,
			BillNo:              order.BillNo,
			IdempotencyKey:      order.IdempotencyKey,
			TenantID:            order.TenantID,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			GrantStatus:         order.GrantStatus,
			Status:              order.GrantStatus,
			ParticipationStatus: r.participationStatusLocked(order.ParticipationID),
			Duplicate:           true,
			Reservations:        r.reservationsForGrantOrderLocked(order.ID),
		}, nil
	}
	if order.GrantStatus != GrantSending && order.GrantStatus != GrantRetry {
		return RewardCallbackResult{}, ErrCallbackConflict
	}

	reservationIndexes := r.reservationIndexesForGrantOrderLocked(order.ID)
	if len(reservationIndexes) == 0 {
		if r.activityHadQuotaAtLocked(order.ActivityID, order.CreatedAt) {
			return RewardCallbackResult{}, ErrQuotaReservationMissing
		}
		order.GrantStatus = callback.Status
		order.ExternalGrantID = callback.ExternalGrantID
		order.ErrorMessage = callback.Message
		order.FinishedAt = &callback.FinishedAt
		order.UpdatedAt = now
		participationStatus := r.updateParticipationAfterCallbackLocked(order.ParticipationID, now)
		return RewardCallbackResult{
			CommandID:           order.BillNo,
			BillNo:              order.BillNo,
			IdempotencyKey:      order.IdempotencyKey,
			TenantID:            order.TenantID,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			GrantStatus:         order.GrantStatus,
			Status:              order.GrantStatus,
			ParticipationStatus: participationStatus,
		}, nil
	}
	for _, idx := range reservationIndexes {
		if r.quotaReservations[idx].Status != QuotaReservationFrozen {
			return RewardCallbackResult{}, ErrQuotaReservationConflict
		}
	}

	for _, idx := range reservationIndexes {
		reservation := &r.quotaReservations[idx]
		if isParticipationQuotaReservation(*reservation) {
			continue
		}
		if err := r.finishReservationLocked(reservation, callback.Status, now); err != nil {
			return RewardCallbackResult{}, err
		}
	}

	order.GrantStatus = callback.Status
	order.ExternalGrantID = callback.ExternalGrantID
	order.ErrorMessage = callback.Message
	order.FinishedAt = &callback.FinishedAt
	order.UpdatedAt = now

	participationStatus := r.updateParticipationAfterCallbackLocked(order.ParticipationID, now)
	if participationStatus == ParticipationGranted || participationStatus == ParticipationFailed {
		finalStatus := GrantSuccess
		if participationStatus == ParticipationFailed {
			finalStatus = GrantFailed
		}
		if err := r.finishParticipationQuotaReservationsLocked(order.ParticipationID, finalStatus, now); err != nil {
			return RewardCallbackResult{}, err
		}
	}
	return RewardCallbackResult{
		CommandID:           order.BillNo,
		BillNo:              order.BillNo,
		IdempotencyKey:      order.IdempotencyKey,
		TenantID:            order.TenantID,
		GrantOrderID:        order.ID,
		ParticipationID:     order.ParticipationID,
		GrantStatus:         order.GrantStatus,
		Status:              order.GrantStatus,
		ParticipationStatus: participationStatus,
		Reservations:        r.reservationsForGrantOrderLocked(order.ID),
	}, nil
}

func (r *MemoryRepository) ApplyManualGrantDecision(ctx context.Context, cmd ManualGrantDecisionCommand) (RewardCallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return RewardCallbackResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	orderIndex := r.manualGrantOrderIndexLocked(cmd)
	if orderIndex == -2 {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if orderIndex < 0 {
		return RewardCallbackResult{}, ErrNotFound
	}
	order := &r.grantOrders[orderIndex]
	mode := grantModeFromPayload(order.RewardPayload)
	if mode != GrantManual && mode != GrantManualExport {
		return RewardCallbackResult{}, ErrCallbackConflict
	}
	if cmd.CommandID != "" && order.BillNo != cmd.CommandID {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if cmd.IdempotencyKey != "" && order.IdempotencyKey != cmd.IdempotencyKey {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if cmd.GrantOrderID != "" && order.ID != cmd.GrantOrderID {
		return RewardCallbackResult{}, ErrCallbackLocatorConflict
	}
	if isFinalGrantStatus(order.GrantStatus) {
		if order.GrantStatus == cmd.Status && (order.ExternalGrantID == "" || cmd.ExternalGrantID == "" || order.ExternalGrantID == cmd.ExternalGrantID) {
			return RewardCallbackResult{
				CommandID:           order.BillNo,
				BillNo:              order.BillNo,
				IdempotencyKey:      order.IdempotencyKey,
				TenantID:            order.TenantID,
				GrantOrderID:        order.ID,
				ParticipationID:     order.ParticipationID,
				GrantStatus:         order.GrantStatus,
				Status:              order.GrantStatus,
				ParticipationStatus: r.participationStatusLocked(order.ParticipationID),
				Duplicate:           true,
				Reservations:        r.reservationsForGrantOrderLocked(order.ID),
			}, nil
		}
		if order.GrantStatus == GrantClosed {
			return RewardCallbackResult{}, ErrGrantOrderClosed
		}
		return RewardCallbackResult{}, ErrCallbackConflict
	}
	if mode == GrantManualExport && cmd.Status == GrantSuccess && (order.GrantStatus != GrantSending || !r.hasReadyManualExportDetailLocked(order.ID)) {
		return RewardCallbackResult{}, ErrManualGrantExportRequired
	}
	if order.GrantStatus != GrantPending && order.GrantStatus != GrantSending && order.GrantStatus != GrantRetry {
		return RewardCallbackResult{}, ErrCallbackConflict
	}
	reservationIndexes := r.reservationIndexesForGrantOrderLocked(order.ID)
	if len(reservationIndexes) == 0 && r.activityHadQuotaAtLocked(order.ActivityID, order.CreatedAt) {
		return RewardCallbackResult{}, ErrQuotaReservationMissing
	}
	for _, idx := range reservationIndexes {
		reservation := r.quotaReservations[idx]
		if reservation.Status != QuotaReservationFrozen && !isParticipationQuotaReservation(reservation) {
			return RewardCallbackResult{}, ErrQuotaReservationConflict
		}
	}
	for _, idx := range reservationIndexes {
		reservation := &r.quotaReservations[idx]
		if isParticipationQuotaReservation(*reservation) {
			continue
		}
		if err := r.finishReservationLocked(reservation, cmd.Status, now); err != nil {
			return RewardCallbackResult{}, err
		}
	}
	order.GrantStatus = cmd.Status
	order.ExternalGrantID = cmd.ExternalGrantID
	order.ErrorMessage = cmd.Reason
	order.FinishedAt = &cmd.FinishedAt
	order.UpdatedAt = now

	participationStatus := r.updateParticipationAfterCallbackLocked(order.ParticipationID, now)
	if participationStatus == ParticipationGranted || participationStatus == ParticipationFailed {
		finalStatus := GrantSuccess
		if participationStatus == ParticipationFailed {
			finalStatus = GrantFailed
		}
		if err := r.finishParticipationQuotaReservationsLocked(order.ParticipationID, finalStatus, now); err != nil {
			return RewardCallbackResult{}, err
		}
	}
	return RewardCallbackResult{
		CommandID:           order.BillNo,
		BillNo:              order.BillNo,
		IdempotencyKey:      order.IdempotencyKey,
		TenantID:            order.TenantID,
		GrantOrderID:        order.ID,
		ParticipationID:     order.ParticipationID,
		GrantStatus:         order.GrantStatus,
		Status:              order.GrantStatus,
		ParticipationStatus: participationStatus,
		Reservations:        r.reservationsForGrantOrderLocked(order.ID),
	}, nil
}

func (r *MemoryRepository) manualGrantOrderIndexLocked(cmd ManualGrantDecisionCommand) int {
	matches := map[int]struct{}{}
	add := func(idx int) {
		if idx >= 0 {
			matches[idx] = struct{}{}
		}
	}
	for i := range r.grantOrders {
		order := r.grantOrders[i]
		if order.TenantID != cmd.TenantID {
			continue
		}
		if cmd.GrantOrderID != "" && order.ID == cmd.GrantOrderID {
			add(i)
		}
		if cmd.CommandID != "" && order.BillNo == cmd.CommandID {
			add(i)
		}
		if cmd.IdempotencyKey != "" && order.IdempotencyKey == cmd.IdempotencyKey {
			add(i)
		}
	}
	if len(matches) > 1 {
		return -2
	}
	for idx := range matches {
		return idx
	}
	return -1
}

func (r *MemoryRepository) CreateManualGrantExport(ctx context.Context, cmd ManualGrantExportCreateCommand) (ManualGrantExportCreateResult, error) {
	if err := ctx.Err(); err != nil {
		return ManualGrantExportCreateResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	candidates := r.manualGrantExportCandidatesLocked(cmd)
	if len(candidates) == 0 {
		return ManualGrantExportCreateResult{}, ErrNotFound
	}
	exportID := NewID("manual_export")
	filterSnapshot, _ := json.Marshal(cmd)
	candidateOrders := make([]GrantOrder, 0, len(candidates))
	for _, orderIndex := range candidates {
		candidateOrders = append(candidateOrders, r.grantOrders[orderIndex])
	}
	rows, checksum, err := manualGrantExportCSV(exportID, candidateOrders)
	if err != nil {
		file := ManualGrantExportFile{
			ID:             exportID,
			TenantID:       cmd.TenantID,
			FileName:       exportID + ".csv",
			FileFormat:     "CSV",
			Status:         ManualGrantExportFailed,
			Operator:       cmd.Operator,
			Reason:         cmd.Reason,
			FilterSnapshot: filterSnapshot,
			RowCount:       len(candidates),
			ErrorMessage:   err.Error(),
			ExpiresAt:      now.Add(24 * time.Hour),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		r.manualExportFiles = append(r.manualExportFiles, file)
		return ManualGrantExportCreateResult{ExportFile: file}, nil
	}
	_ = rows
	file := ManualGrantExportFile{
		ID:             exportID,
		TenantID:       cmd.TenantID,
		FileName:       exportID + ".csv",
		FileFormat:     "CSV",
		Status:         ManualGrantExportReady,
		Operator:       cmd.Operator,
		Reason:         cmd.Reason,
		FilterSnapshot: filterSnapshot,
		RowCount:       len(candidates),
		FileChecksum:   checksum,
		StorageRef:     "memory://manual-grant-exports/" + exportID + ".csv",
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	r.manualExportFiles = append(r.manualExportFiles, file)
	details := make([]ManualGrantExportDetail, 0, len(candidates))
	for idx, orderIndex := range candidates {
		order := &r.grantOrders[orderIndex]
		detail := ManualGrantExportDetail{
			ID:                NewID("manual_export_detail"),
			ExportID:          exportID,
			TenantID:          cmd.TenantID,
			GrantOrderID:      order.ID,
			BillNo:            order.BillNo,
			RowNumber:         idx + 1,
			ExportGrantStatus: order.GrantStatus,
			DetailStatus:      ManualGrantExportDetailReady,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		r.manualExportDetails = append(r.manualExportDetails, detail)
		details = append(details, detail)
		order.GrantStatus = GrantSending
		order.UpdatedAt = now
	}
	return ManualGrantExportCreateResult{ExportFile: file, Details: details}, nil
}

func (r *MemoryRepository) rewardCallbackOrderIndexLocked(callback RewardCallback) int {
	byCommand := -1
	byIDem := -1
	for i := range r.grantOrders {
		order := r.grantOrders[i]
		if order.TenantID != callback.TenantID {
			continue
		}
		if callback.CommandID != "" && order.BillNo == callback.CommandID {
			byCommand = i
		}
		if callback.IdempotencyKey != "" && order.IdempotencyKey == callback.IdempotencyKey {
			byIDem = i
		}
	}
	if byCommand >= 0 && byIDem >= 0 && byCommand != byIDem {
		return -2
	}
	if byCommand >= 0 {
		return byCommand
	}
	return byIDem
}

func (r *MemoryRepository) ReleaseExpiredQuotaReservations(ctx context.Context, cmd QuotaTimeoutReleaseCommand) (QuotaTimeoutReleaseResult, error) {
	if err := ctx.Err(); err != nil {
		return QuotaTimeoutReleaseResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	limit := cmd.Limit
	if limit <= 0 {
		limit = 100
	}
	result := QuotaTimeoutReleaseResult{
		TenantID:  cmd.TenantID,
		OlderThan: cmd.OlderThan,
		Limit:     limit,
		DryRun:    cmd.DryRun,
		Operator:  cmd.Operator,
		Reason:    cmd.Reason,
	}
	now := time.Now().UTC()
	for i := range r.grantOrders {
		if len(result.Items) >= limit {
			break
		}
		order := &r.grantOrders[i]
		if order.TenantID != cmd.TenantID || isFinalGrantStatus(order.GrantStatus) {
			continue
		}
		mode := grantModeFromPayload(order.RewardPayload)
		if mode == GrantManual || mode == GrantManualExport {
			continue
		}
		candidateAt := order.UpdatedAt
		if candidateAt.IsZero() {
			candidateAt = order.CreatedAt
		}
		if !candidateAt.Before(cmd.OlderThan) {
			continue
		}
		if !r.hasFrozenReservationForGrantOrderLocked(order.ID) {
			continue
		}
		beforeStatus := order.GrantStatus
		if cmd.DryRun {
			result.Items = append(result.Items, QuotaTimeoutReleaseItem{
				CommandID:           order.BillNo,
				GrantOrderID:        order.ID,
				ParticipationID:     order.ParticipationID,
				ActivityID:          order.ActivityID,
				PreviousGrantStatus: beforeStatus,
				GrantStatus:         order.GrantStatus,
				ParticipationStatus: r.participationStatusLocked(order.ParticipationID),
				Result:              "DRY_RUN",
				Reservations:        r.reservationsForGrantOrderLocked(order.ID),
			})
			continue
		}
		for _, idx := range r.reservationIndexesForGrantOrderLocked(order.ID) {
			reservation := &r.quotaReservations[idx]
			if reservation.Status != QuotaReservationFrozen || isParticipationQuotaReservation(*reservation) {
				continue
			}
			if err := r.finishReservationLocked(reservation, GrantFailed, now); err != nil {
				return QuotaTimeoutReleaseResult{}, err
			}
		}
		order.GrantStatus = GrantFailed
		order.ErrorMessage = "TIMEOUT_RELEASE: " + cmd.Reason
		order.FinishedAt = &now
		order.UpdatedAt = now
		participationStatus := r.updateParticipationAfterCallbackLocked(order.ParticipationID, now)
		if participationStatus == ParticipationGranted || participationStatus == ParticipationFailed {
			finalStatus := GrantSuccess
			if participationStatus == ParticipationFailed {
				finalStatus = GrantFailed
			}
			if err := r.finishParticipationQuotaReservationsLocked(order.ParticipationID, finalStatus, now); err != nil {
				return QuotaTimeoutReleaseResult{}, err
			}
		}
		result.Items = append(result.Items, QuotaTimeoutReleaseItem{
			CommandID:           order.BillNo,
			GrantOrderID:        order.ID,
			ParticipationID:     order.ParticipationID,
			ActivityID:          order.ActivityID,
			PreviousGrantStatus: beforeStatus,
			GrantStatus:         order.GrantStatus,
			ParticipationStatus: participationStatus,
			Result:              "RELEASED",
			Reservations:        r.reservationsForGrantOrderLocked(order.ID),
		})
	}
	return result, nil
}

func (r *MemoryRepository) ListRewardReconciliation(ctx context.Context, query RewardReconciliationQuery) ([]RewardReconciliationItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	var out []RewardReconciliationItem
	addItem := func(item RewardReconciliationItem) {
		if len(out) >= limit {
			return
		}
		out = append(out, item)
	}

	if query.OutboxID != "" {
		for _, message := range r.outboxMessages {
			if len(out) >= limit {
				break
			}
			if message.TenantID != query.TenantID || message.ID != query.OutboxID {
				continue
			}
			command := outboxRewardCommand(message.Payload)
			var matched bool
			if command.CommandID != "" || command.ParticipationID != "" || command.IdempotencyKey != "" {
				for _, order := range r.grantOrders {
					if order.TenantID != query.TenantID {
						continue
					}
					if command.CommandID != "" && order.BillNo != command.CommandID {
						continue
					}
					if command.ParticipationID != "" && order.ParticipationID != command.ParticipationID {
						continue
					}
					if command.IdempotencyKey != "" && order.IdempotencyKey != command.IdempotencyKey {
						continue
					}
					item := r.rewardReconciliationItemForOrderLocked(order)
					item.OutboxMessages = []OutboxMessage{message}
					item.Diagnostics = reconciliationDiagnostics(item)
					if rewardReconciliationItemMatchesQuery(item, query) {
						addItem(item)
					}
					matched = true
				}
			}
			if !matched {
				item := RewardReconciliationItem{
					TenantID:        message.TenantID,
					CommandID:       command.CommandID,
					ParticipationID: command.ParticipationID,
					ActivityID:      command.ActivityID,
					Subject:         command.Subject,
					ServerUpdatedAt: message.UpdatedAt,
					OutboxMessages:  []OutboxMessage{message},
					Diagnostics:     []string{"OUTBOX_UNLINKED"},
				}
				if rewardReconciliationItemMatchesQuery(item, query) {
					addItem(item)
				}
			}
		}
		return out, nil
	}

	seenParticipations := map[string]struct{}{}
	for _, participation := range r.participations {
		if len(out) >= limit {
			break
		}
		if participation.TenantID != query.TenantID {
			continue
		}
		inbox := r.inboxByIDLocked(participation.EventInboxID)
		if !participationMatchesReconciliationQuery(participation, inbox, query) {
			continue
		}
		seenParticipations[participation.ID] = struct{}{}
		orders := r.grantOrdersForParticipationLocked(participation.ID)
		if len(orders) == 0 {
			item := r.rewardReconciliationItemForParticipationLocked(participation, inbox)
			item.Diagnostics = reconciliationDiagnostics(item)
			addItem(item)
			continue
		}
		for _, order := range orders {
			item := r.rewardReconciliationItemForOrderLocked(order)
			item.Diagnostics = reconciliationDiagnostics(item)
			if rewardReconciliationItemMatchesQuery(item, query) {
				addItem(item)
			}
		}
	}

	for _, order := range r.grantOrders {
		if len(out) >= limit {
			break
		}
		if order.TenantID != query.TenantID {
			continue
		}
		if _, ok := seenParticipations[order.ParticipationID]; ok {
			continue
		}
		item := r.rewardReconciliationItemForOrderLocked(order)
		item.Diagnostics = reconciliationDiagnostics(item)
		if rewardReconciliationItemMatchesQuery(item, query) {
			addItem(item)
		}
	}

	if query.EventID != "" || query.IdempotencyKey != "" {
		for _, inbox := range r.inboxByIdempotency {
			if len(out) >= limit {
				break
			}
			if inbox.Event.TenantID != query.TenantID {
				continue
			}
			if query.EventID != "" && inbox.Event.EventID != query.EventID {
				continue
			}
			if query.IdempotencyKey != "" && inbox.IdempotencyKey != query.IdempotencyKey {
				continue
			}
			if r.inboxHasParticipationLocked(inbox.ID) {
				continue
			}
			item := RewardReconciliationItem{
				TenantID:        inbox.Event.TenantID,
				Subject:         inbox.Event.Subject,
				ServerUpdatedAt: inbox.UpdatedAt,
				EventInbox:      &inbox,
				Reservations:    []QuotaReservation{},
				Diagnostics:     []string{"EVENT_RECEIVED_NO_PARTICIPATION"},
			}
			addItem(item)
		}
	}
	return out, nil
}

func participationMatchesReconciliationQuery(participation Participation, inbox EventInbox, query RewardReconciliationQuery) bool {
	if query.ParticipationID != "" && participation.ID != query.ParticipationID {
		return false
	}
	if query.ActivityID != "" && participation.ActivityID != query.ActivityID {
		return false
	}
	if query.SubjectType != "" && participation.Subject.Type != query.SubjectType {
		return false
	}
	if query.SubjectID != "" && participation.Subject.ID != query.SubjectID {
		return false
	}
	if query.EventID != "" && inbox.Event.EventID != query.EventID {
		return false
	}
	if query.IdempotencyKey != "" && inbox.IdempotencyKey != query.IdempotencyKey && participation.IdempotencyKey != query.IdempotencyKey {
		return false
	}
	return true
}

func rewardReconciliationItemMatchesQuery(item RewardReconciliationItem, query RewardReconciliationQuery) bool {
	if query.CommandID != "" && item.CommandID != query.CommandID {
		return false
	}
	if query.GrantOrderID != "" && item.GrantOrderID != query.GrantOrderID {
		return false
	}
	if query.ActivityID != "" && item.ActivityID != query.ActivityID {
		return false
	}
	if query.SubjectType != "" && item.Subject.Type != query.SubjectType {
		return false
	}
	if query.SubjectID != "" && item.Subject.ID != query.SubjectID {
		return false
	}
	if query.ParticipationID != "" && item.ParticipationID != query.ParticipationID {
		return false
	}
	if query.EventID != "" {
		if item.EventInbox == nil || item.EventInbox.Event.EventID != query.EventID {
			return false
		}
	}
	if query.IdempotencyKey != "" {
		if item.EventInbox == nil || item.EventInbox.IdempotencyKey != query.IdempotencyKey {
			if item.GrantOrder == nil || item.GrantOrder.IdempotencyKey != query.IdempotencyKey {
				return false
			}
		}
	}
	if query.OutboxID != "" {
		for _, message := range item.OutboxMessages {
			if message.ID == query.OutboxID {
				return true
			}
		}
		return false
	}
	return true
}

func (r *MemoryRepository) rewardReconciliationItemForOrderLocked(order GrantOrder) RewardReconciliationItem {
	participation := r.participationLocked(order.ParticipationID)
	inbox := r.inboxByIDLocked(participation.EventInboxID)
	item := RewardReconciliationItem{
		TenantID:        order.TenantID,
		CommandID:       order.BillNo,
		GrantOrderID:    order.ID,
		ExternalGrantID: order.ExternalGrantID,
		ActivityID:      order.ActivityID,
		ParticipationID: order.ParticipationID,
		Subject:         order.Subject,
		RewardType:      order.RewardType,
		RewardAmount:    order.RewardAmount,
		GrantStatus:     order.GrantStatus,
		CallbackMessage: order.ErrorMessage,
		FinishedAt:      order.FinishedAt,
		ServerUpdatedAt: order.UpdatedAt,
		Reservations:    r.reservationsForGrantOrderLocked(order.ID),
		GrantOrder:      &order,
	}
	item.ParticipationStatus = participation.Status
	if participation.ID != "" {
		item.Participation = &participation
	}
	if inbox.ID != "" {
		item.EventInbox = &inbox
	}
	item.OutboxMessages = r.outboxesForRewardLocked(order.BillNo, order.ParticipationID)
	item.AuditTimeline = r.auditTimelineLocked(order.TenantID, []string{order.ID, order.BillNo, order.ParticipationID, order.ActivityID, inbox.ID, inbox.Event.EventID, inbox.IdempotencyKey})
	return item
}

func (r *MemoryRepository) rewardReconciliationItemForParticipationLocked(participation Participation, inbox EventInbox) RewardReconciliationItem {
	item := RewardReconciliationItem{
		TenantID:            participation.TenantID,
		ActivityID:          participation.ActivityID,
		ParticipationID:     participation.ID,
		Subject:             participation.Subject,
		ParticipationStatus: participation.Status,
		ServerUpdatedAt:     participation.UpdatedAt,
		Reservations:        []QuotaReservation{},
		Participation:       &participation,
	}
	if inbox.ID != "" {
		item.EventInbox = &inbox
	}
	item.AuditTimeline = r.auditTimelineLocked(participation.TenantID, []string{participation.ID, participation.ActivityID, inbox.ID, inbox.Event.EventID, inbox.IdempotencyKey})
	return item
}

func reconciliationDiagnostics(item RewardReconciliationItem) []string {
	var diagnostics []string
	if item.EventInbox != nil && item.EventInbox.ProcessStatus != InboxDuplicate && item.ParticipationID == "" {
		diagnostics = append(diagnostics, "EVENT_RECEIVED_NO_PARTICIPATION")
	}
	if item.ParticipationStatus == ParticipationRejected {
		diagnostics = append(diagnostics, "PARTICIPATION_REJECTED")
	}
	if item.ParticipationStatus == ParticipationQualified && strings.Contains(strings.ToUpper(item.ParticipationRejectReason()), "ZERO_REWARD") {
		diagnostics = append(diagnostics, "ZERO_REWARD_SKIPPED")
	}
	if item.ParticipationStatus == ParticipationRewardable && item.GrantOrderID == "" {
		diagnostics = append(diagnostics, "PARTICIPATION_REWARDABLE_NO_GRANT_ORDER")
	}
	if item.GrantOrderID != "" && len(item.OutboxMessages) == 0 {
		diagnostics = append(diagnostics, "GRANT_ORDER_NO_OUTBOX")
	}
	for _, message := range item.OutboxMessages {
		if message.SendStatus == OutboxFailed || message.SendStatus == OutboxDLQ || message.SendStatus == OutboxDLQClosed {
			diagnostics = append(diagnostics, "OUTBOX_FAILED_OR_DLQ")
			continue
		}
		if message.SendStatus == OutboxSent && (item.GrantStatus == GrantPending || item.GrantStatus == GrantSending || item.GrantStatus == GrantRetry) {
			diagnostics = append(diagnostics, "OUTBOX_SENT_GRANT_PENDING")
		}
	}
	if item.GrantStatus == GrantSuccess && item.ParticipationStatus != "" && item.ParticipationStatus != ParticipationGranted {
		diagnostics = append(diagnostics, "GRANT_SUCCESS_PARTICIPATION_NOT_GRANTED")
	}
	if item.GrantStatus == GrantFailed || item.GrantStatus == GrantClosed {
		for _, reservation := range item.Reservations {
			if reservation.Status == QuotaReservationFrozen {
				diagnostics = append(diagnostics, "GRANT_FAILED_QUOTA_STILL_FROZEN")
				break
			}
		}
	}
	return diagnostics
}

func (item RewardReconciliationItem) ParticipationRejectReason() string {
	if item.Participation == nil {
		return ""
	}
	return item.Participation.RejectReason
}

func (r *MemoryRepository) grantOrdersForParticipationLocked(participationID string) []GrantOrder {
	var out []GrantOrder
	for _, order := range r.grantOrders {
		if order.ParticipationID == participationID {
			out = append(out, order)
		}
	}
	return out
}

func (r *MemoryRepository) inboxHasParticipationLocked(inboxID string) bool {
	for _, participation := range r.participations {
		if participation.EventInboxID == inboxID {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) CreateParticipationGrantOutboxesWithQuota(ctx context.Context, cmd AtomicGrantCommand) (AtomicGrantResult, error) {
	if err := ctx.Err(); err != nil {
		return AtomicGrantResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if participationIdempotencyExists(r.participations, cmd.Participation.IdempotencyKey) {
		return AtomicGrantResult{}, ErrIdempotencyConflict
	}
	for _, order := range cmd.GrantOrders {
		if grantOrderIdempotencyExists(r.grantOrders, order.IdempotencyKey) {
			return AtomicGrantResult{}, ErrIdempotencyConflict
		}
	}
	seenOrderKeys := map[string]struct{}{}
	for _, order := range cmd.GrantOrders {
		if order.IdempotencyKey == "" {
			continue
		}
		if _, ok := seenOrderKeys[order.IdempotencyKey]; ok {
			return AtomicGrantResult{}, ErrIdempotencyConflict
		}
		seenOrderKeys[order.IdempotencyKey] = struct{}{}
	}

	nextQuotas := slices.Clone(r.quotas)
	if !reserveQuotaIn(nextQuotas, cmd.Participation.ActivityID, QuotaBudget, QuotaScopeActivity, "TOTAL", cmd.BudgetAmount) {
		return AtomicGrantResult{}, QuotaExceededError{Type: QuotaBudget, Scope: QuotaScopeActivity, ScopeKey: "TOTAL", Amount: cmd.BudgetAmount}
	}
	if cmd.ReserveUserLimit && !reserveQuotaIn(nextQuotas, cmd.Participation.ActivityID, QuotaUserLimit, QuotaScopeUser, cmd.UserScopeKey, "1") {
		return AtomicGrantResult{}, QuotaExceededError{Type: QuotaUserLimit, Scope: QuotaScopeUser, ScopeKey: cmd.UserScopeKey, Amount: "1"}
	}
	if cmd.ReserveDailyLimit && !reserveQuotaIn(nextQuotas, cmd.Participation.ActivityID, QuotaDailyLimit, QuotaScopeUser, cmd.DailyLimitScopeKey, "1") {
		return AtomicGrantResult{}, QuotaExceededError{Type: QuotaDailyLimit, Scope: QuotaScopeUser, ScopeKey: cmd.DailyLimitScopeKey, Amount: "1"}
	}
	for _, reservation := range cmd.StockReservations {
		if !reserveQuotaIn(nextQuotas, cmd.Participation.ActivityID, reservation.Type, reservation.Scope, reservation.ScopeKey, reservation.Amount) {
			return AtomicGrantResult{}, QuotaExceededError{Type: reservation.Type, Scope: reservation.Scope, ScopeKey: reservation.ScopeKey, Amount: reservation.Amount}
		}
	}
	r.quotas = nextQuotas
	r.participations = append(r.participations, cmd.Participation)
	r.grantOrders = append(r.grantOrders, cmd.GrantOrders...)
	reservations := quotaReservationsForAtomicCommand(cmd, time.Now().UTC(), r.quotas)
	r.quotaReservations = append(r.quotaReservations, reservations...)
	r.outboxMessages = append(r.outboxMessages, cmd.OutboxMessages...)
	return AtomicGrantResult{
		Participation:  cmd.Participation,
		GrantOrders:    cmd.GrantOrders,
		OutboxMessages: cmd.OutboxMessages,
		Reservations:   reservations,
	}, nil
}

func (r *MemoryRepository) CreateOutbox(ctx context.Context, message OutboxMessage) (OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return OutboxMessage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outboxMessages = append(r.outboxMessages, message)
	return message, nil
}

func (r *MemoryRepository) CreateQuota(ctx context.Context, quota Quota) (Quota, error) {
	if err := ctx.Err(); err != nil {
		return Quota{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotas = append(r.quotas, quota)
	return quota, nil
}

func (r *MemoryRepository) ListActivityQuotas(ctx context.Context, activityID string) ([]Quota, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Quota
	for _, quota := range r.quotas {
		if quota.ActivityID == activityID {
			out = append(out, quota)
		}
	}
	return out, nil
}

func (r *MemoryRepository) ReserveQuota(ctx context.Context, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		total := decimal.MustParse(q.TotalValue)
		used := decimal.MustParse(q.UsedValue)
		frozen := decimal.MustParse(q.FrozenValue)
		increment := decimal.MustParse(amount)
		next := used.Add(frozen).Add(increment)
		if next.Cmp(total) > 0 {
			return false, nil
		}
		q.FrozenValue = frozen.Add(increment).String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return true, nil
	}
	return true, nil
}

func (r *MemoryRepository) ListPendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []OutboxMessage
	for _, message := range r.outboxMessages {
		if len(out) >= limit {
			break
		}
		if message.TargetChannel != GrantHTTP && message.TargetChannel != GrantWebhook && message.TargetChannel != GrantMQ {
			continue
		}
		if message.SendStatus == OutboxPending || message.SendStatus == OutboxFailed {
			if message.NextRetryAt == nil || !message.NextRetryAt.After(time.Now().UTC()) {
				out = append(out, message)
			}
		}
	}
	for _, claimed := range out {
		for i := range r.outboxMessages {
			if r.outboxMessages[i].ID == claimed.ID {
				r.outboxMessages[i].SendStatus = OutboxSending
				r.outboxMessages[i].UpdatedAt = time.Now().UTC()
			}
		}
	}
	return out, nil
}

func (r *MemoryRepository) ListOutboxDLQ(ctx context.Context, tenantID string, includeClosed bool, limit int) ([]OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if limit <= 0 {
		limit = 100
	}
	var out []OutboxMessage
	for _, message := range r.outboxMessages {
		if len(out) >= limit {
			break
		}
		if tenantID != "" && message.TenantID != tenantID {
			continue
		}
		if message.SendStatus == OutboxDLQ || includeClosed && message.SendStatus == OutboxDLQClosed {
			out = append(out, message)
		}
	}
	return out, nil
}

func (r *MemoryRepository) MarkOutboxSent(ctx context.Context, id string) error {
	return r.updateOutbox(ctx, id, func(message *OutboxMessage) {
		message.SendStatus = OutboxSent
		message.NextRetryAt = nil
		message.ErrorMessage = ""
		message.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) MarkOutboxFailed(ctx context.Context, id string, retryCount int, nextRetryAt time.Time, message string) error {
	return r.updateOutbox(ctx, id, func(item *OutboxMessage) {
		item.SendStatus = OutboxFailed
		item.RetryCount = retryCount
		item.NextRetryAt = &nextRetryAt
		item.ErrorMessage = message
		item.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) MarkOutboxDLQ(ctx context.Context, id string, retryCount int, message string) error {
	return r.updateOutbox(ctx, id, func(item *OutboxMessage) {
		item.SendStatus = OutboxDLQ
		item.RetryCount = retryCount
		item.NextRetryAt = nil
		item.ErrorMessage = message
		item.UpdatedAt = time.Now().UTC()
	})
}

func (r *MemoryRepository) RecoverStaleOutbox(ctx context.Context, staleBefore time.Time, limit int) ([]OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var recovered []OutboxMessage
	for i := range r.outboxMessages {
		if len(recovered) >= limit {
			break
		}
		item := &r.outboxMessages[i]
		if item.SendStatus != OutboxSending || !item.UpdatedAt.Before(staleBefore) {
			continue
		}
		before := *item
		item.SendStatus = OutboxPending
		item.NextRetryAt = nil
		item.UpdatedAt = time.Now().UTC()
		recovered = append(recovered, before)
	}
	return recovered, nil
}

func (r *MemoryRepository) CountOutboxDLQ(ctx context.Context) ([]OutboxDLQCount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := map[string]OutboxDLQCount{}
	for _, message := range r.outboxMessages {
		if message.SendStatus != OutboxDLQ {
			continue
		}
		key := message.TenantID + "\x00" + string(message.TargetChannel)
		count := counts[key]
		count.TenantID = message.TenantID
		count.TargetChannel = message.TargetChannel
		count.Count++
		counts[key] = count
	}
	out := make([]OutboxDLQCount, 0, len(counts))
	for _, count := range counts {
		out = append(out, count)
	}
	return out, nil
}

func (r *MemoryRepository) CountOutboxBacklog(ctx context.Context, now time.Time) ([]OutboxBacklogMetric, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := map[string]OutboxBacklogMetric{}
	for _, message := range r.outboxMessages {
		if message.SendStatus != OutboxPending && message.SendStatus != OutboxFailed && message.SendStatus != OutboxSending && message.SendStatus != OutboxDLQ && message.SendStatus != OutboxDLQClosed {
			continue
		}
		key := message.TenantID + "\x00" + string(message.TargetChannel) + "\x00" + string(message.SendStatus)
		metric := counts[key]
		metric.TenantID = message.TenantID
		metric.TargetChannel = message.TargetChannel
		metric.SendStatus = message.SendStatus
		metric.Count++
		if age := outboxEligibleAge(message, now); age > metric.OldestEligibleAgeSeconds {
			metric.OldestEligibleAgeSeconds = age
		}
		counts[key] = metric
	}
	out := make([]OutboxBacklogMetric, 0, len(counts))
	for _, metric := range counts {
		out = append(out, metric)
	}
	return out, nil
}

func outboxEligibleAge(message OutboxMessage, now time.Time) float64 {
	switch message.SendStatus {
	case OutboxPending, OutboxSending, OutboxDLQ:
	case OutboxFailed:
		if message.NextRetryAt != nil && message.NextRetryAt.After(now) {
			return 0
		}
	case OutboxDLQClosed:
		return 0
	default:
		return 0
	}
	eligibleAt := message.CreatedAt
	if message.SendStatus == OutboxFailed && message.NextRetryAt != nil {
		eligibleAt = *message.NextRetryAt
	}
	if eligibleAt.IsZero() || eligibleAt.After(now) {
		return 0
	}
	return now.Sub(eligibleAt).Seconds()
}

func (r *MemoryRepository) ReplayOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, replayedAt time.Time) (OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return OutboxMessage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outboxMessages {
		item := &r.outboxMessages[i]
		if item.ID != id || item.TenantID != tenantID {
			continue
		}
		if item.SendStatus != OutboxDLQ {
			return OutboxMessage{}, ErrInvalidOperation
		}
		if commandID := outboxRewardCommandID(item.Payload); commandID != "" {
			order := r.grantOrderByCommandIDLocked(tenantID, commandID)
			if order == nil {
				return OutboxMessage{}, ErrNotFound
			}
			if order.GrantStatus != GrantRetry {
				return OutboxMessage{}, ErrInvalidOperation
			}
		}
		before := *item
		item.SendStatus = OutboxPending
		item.NextRetryAt = nil
		item.UpdatedAt = replayedAt
		r.auditRecords = append(r.auditRecords, outboxOperationAudit(ctx, tenantID, operator, "OUTBOX_DLQ_REPLAY", before, item.SendStatus, reason, replayedAt))
		return *item, nil
	}
	return OutboxMessage{}, ErrNotFound
}

func (r *MemoryRepository) CloseOutboxDLQ(ctx context.Context, tenantID string, id string, operator string, reason string, closedAt time.Time) (OutboxMessage, error) {
	if err := ctx.Err(); err != nil {
		return OutboxMessage{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outboxMessages {
		item := &r.outboxMessages[i]
		if item.ID != id || item.TenantID != tenantID {
			continue
		}
		if item.SendStatus != OutboxDLQ {
			return OutboxMessage{}, ErrInvalidOperation
		}
		before := *item
		item.SendStatus = OutboxDLQClosed
		item.UpdatedAt = closedAt
		r.auditRecords = append(r.auditRecords, outboxOperationAudit(ctx, tenantID, operator, "OUTBOX_DLQ_CLOSE", before, item.SendStatus, reason, closedAt))
		return *item, nil
	}
	return OutboxMessage{}, ErrNotFound
}

func (r *MemoryRepository) CreateAudit(ctx context.Context, record AuditRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditRecords = append(r.auditRecords, record)
	return nil
}

func (r *MemoryRepository) ListAudit(ctx context.Context, query AuditQuery) ([]AuditRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var out []AuditRecord
	for _, record := range r.auditRecords {
		if len(out) >= limit {
			break
		}
		if !auditRecordMatches(record, query) {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (r *MemoryRepository) updateOutbox(ctx context.Context, id string, update func(*OutboxMessage)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.outboxMessages {
		if r.outboxMessages[i].ID == id {
			update(&r.outboxMessages[i])
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) reserveQuotaLocked(activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) bool {
	return reserveQuotaIn(r.quotas, activityID, quotaType, scope, scopeKey, amount)
}

func reserveQuotaIn(quotas []Quota, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) bool {
	for i := range quotas {
		q := &quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		total := decimal.MustParse(q.TotalValue)
		used := decimal.MustParse(q.UsedValue)
		frozen := decimal.MustParse(q.FrozenValue)
		increment := decimal.MustParse(amount)
		next := used.Add(frozen).Add(increment)
		if next.Cmp(total) > 0 {
			return false
		}
		q.FrozenValue = frozen.Add(increment).String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return true
	}
	return true
}

func quotaReservationsForAtomicCommand(cmd AtomicGrantCommand, now time.Time, quotas []Quota) []QuotaReservation {
	var reservations []QuotaReservation
	if len(cmd.GrantOrders) > 0 {
		firstOrder := cmd.GrantOrders[0]
		if cmd.ReserveUserLimit && quotaExistsIn(quotas, cmd.Participation.ActivityID, QuotaUserLimit, QuotaScopeUser, cmd.UserScopeKey) {
			reservations = append(reservations, QuotaReservation{
				ID:           NewID("reservation"),
				TenantID:     firstOrder.TenantID,
				ActivityID:   firstOrder.ActivityID,
				GrantOrderID: firstOrder.ID,
				CommandID:    firstOrder.BillNo,
				Type:         QuotaUserLimit,
				Scope:        QuotaScopeUser,
				ScopeKey:     cmd.UserScopeKey,
				Amount:       "1",
				Status:       QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
		if cmd.ReserveDailyLimit && quotaExistsIn(quotas, cmd.Participation.ActivityID, QuotaDailyLimit, QuotaScopeUser, cmd.DailyLimitScopeKey) {
			reservations = append(reservations, QuotaReservation{
				ID:           NewID("reservation"),
				TenantID:     firstOrder.TenantID,
				ActivityID:   firstOrder.ActivityID,
				GrantOrderID: firstOrder.ID,
				CommandID:    firstOrder.BillNo,
				Type:         QuotaDailyLimit,
				Scope:        QuotaScopeUser,
				ScopeKey:     cmd.DailyLimitScopeKey,
				Amount:       "1",
				Status:       QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	for _, order := range cmd.GrantOrders {
		if order.RewardAmount != "" && quotaExistsIn(quotas, cmd.Participation.ActivityID, QuotaBudget, QuotaScopeActivity, "TOTAL") {
			reservations = append(reservations, QuotaReservation{
				ID:           NewID("reservation"),
				TenantID:     order.TenantID,
				ActivityID:   order.ActivityID,
				GrantOrderID: order.ID,
				CommandID:    order.BillNo,
				Type:         QuotaBudget,
				Scope:        QuotaScopeActivity,
				ScopeKey:     "TOTAL",
				Amount:       order.RewardAmount,
				Status:       QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
		rewardCode := rewardCodeFromPayload(order.RewardPayload)
		if rewardCode != "" && quotaExistsIn(quotas, cmd.Participation.ActivityID, QuotaStock, QuotaScopeActivity, rewardCode) {
			reservations = append(reservations, QuotaReservation{
				ID:           NewID("reservation"),
				TenantID:     order.TenantID,
				ActivityID:   order.ActivityID,
				GrantOrderID: order.ID,
				CommandID:    order.BillNo,
				Type:         QuotaStock,
				Scope:        QuotaScopeActivity,
				ScopeKey:     rewardCode,
				Amount:       "1",
				Status:       QuotaReservationFrozen,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
		}
	}
	return reservations
}

func quotaExistsIn(quotas []Quota, activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string) bool {
	for _, quota := range quotas {
		if quota.ActivityID == activityID && quota.Type == quotaType && quota.Scope == scope && quota.ScopeKey == scopeKey {
			return true
		}
	}
	return false
}

func isParticipationQuotaReservation(reservation QuotaReservation) bool {
	return reservation.Type == QuotaUserLimit || reservation.Type == QuotaDailyLimit
}

func rewardCodeFromPayload(payload json.RawMessage) string {
	var body struct {
		RewardCode string `json:"rewardCode"`
	}
	_ = json.Unmarshal(payload, &body)
	return body.RewardCode
}

func grantModeFromPayload(payload json.RawMessage) GrantMode {
	var body struct {
		GrantMode GrantMode `json:"grantMode"`
	}
	_ = json.Unmarshal(payload, &body)
	if body.GrantMode == "" {
		return GrantAuto
	}
	return body.GrantMode
}

func rewardAmountFromPayload(payload json.RawMessage) string {
	var body struct {
		RewardAmount string `json:"rewardAmount"`
		Amount       string `json:"amount"`
	}
	_ = json.Unmarshal(payload, &body)
	return firstNonEmpty(body.RewardAmount, body.Amount)
}

func (r *MemoryRepository) hasReadyManualExportDetailLocked(grantOrderID string) bool {
	for _, detail := range r.manualExportDetails {
		if detail.GrantOrderID != grantOrderID || detail.DetailStatus != ManualGrantExportDetailReady {
			continue
		}
		for _, file := range r.manualExportFiles {
			if file.ID == detail.ExportID && file.Status == ManualGrantExportReady {
				return true
			}
		}
	}
	return false
}

func (r *MemoryRepository) hasBlockingManualExportDetailLocked(grantOrderID string) bool {
	for _, detail := range r.manualExportDetails {
		if detail.GrantOrderID != grantOrderID {
			continue
		}
		for _, file := range r.manualExportFiles {
			if file.ID == detail.ExportID && file.Status != ManualGrantExportFailed {
				return true
			}
		}
	}
	return false
}

func (r *MemoryRepository) manualGrantExportCandidatesLocked(cmd ManualGrantExportCreateCommand) []int {
	allowedIDs := map[string]struct{}{}
	for _, id := range cmd.GrantOrderIDs {
		if strings.TrimSpace(id) != "" {
			allowedIDs[id] = struct{}{}
		}
	}
	limit := cmd.Limit
	if limit <= 0 {
		limit = 1000
	}
	var candidates []int
	for i, order := range r.grantOrders {
		if len(candidates) >= limit {
			break
		}
		if order.TenantID != cmd.TenantID || grantModeFromPayload(order.RewardPayload) != GrantManualExport {
			continue
		}
		if order.GrantStatus != GrantPending && order.GrantStatus != GrantRetry {
			continue
		}
		if len(allowedIDs) > 0 {
			if _, ok := allowedIDs[order.ID]; !ok {
				continue
			}
		}
		if cmd.ActivityID != "" && order.ActivityID != cmd.ActivityID {
			continue
		}
		if cmd.SubjectType != "" && order.Subject.Type != cmd.SubjectType {
			continue
		}
		if cmd.SubjectID != "" && order.Subject.ID != cmd.SubjectID {
			continue
		}
		if !cmd.CreatedFrom.IsZero() && order.CreatedAt.Before(cmd.CreatedFrom) {
			continue
		}
		if !cmd.CreatedTo.IsZero() && !order.CreatedAt.Before(cmd.CreatedTo) {
			continue
		}
		if cmd.RewardCode != "" && rewardCodeFromPayload(order.RewardPayload) != cmd.RewardCode {
			continue
		}
		if r.hasBlockingManualExportDetailLocked(order.ID) {
			continue
		}
		candidates = append(candidates, i)
	}
	return candidates
}

func manualGrantExportCSV(exportID string, orders []GrantOrder) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	header := []string{"tenantId", "exportId", "billNo", "grantOrderId", "idempotencyKey", "activityId", "participationId", "subjectType", "subjectId", "rewardCode", "rewardType", "rewardAmount", "rewardPayload", "grantStatus", "createdAt"}
	if err := writer.Write(header); err != nil {
		return nil, "", err
	}
	for _, order := range orders {
		amount := firstNonEmpty(order.RewardAmount, rewardAmountFromPayload(order.RewardPayload))
		row := []string{
			order.TenantID,
			exportID,
			order.BillNo,
			order.ID,
			order.IdempotencyKey,
			order.ActivityID,
			order.ParticipationID,
			order.Subject.Type,
			order.Subject.ID,
			rewardCodeFromPayload(order.RewardPayload),
			string(order.RewardType),
			amount,
			string(order.RewardPayload),
			string(order.GrantStatus),
			order.CreatedAt.Format(time.RFC3339Nano),
		}
		if err := writer.Write(row); err != nil {
			return nil, "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

func (r *MemoryRepository) reservationIndexesForGrantOrderLocked(grantOrderID string) []int {
	var indexes []int
	for i := range r.quotaReservations {
		if r.quotaReservations[i].GrantOrderID == grantOrderID {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (r *MemoryRepository) reservationsForGrantOrderLocked(grantOrderID string) []QuotaReservation {
	indexes := r.reservationIndexesForGrantOrderLocked(grantOrderID)
	out := make([]QuotaReservation, 0, len(indexes))
	for _, idx := range indexes {
		out = append(out, r.quotaReservations[idx])
	}
	return out
}

func (r *MemoryRepository) finishReservationQuotaLocked(reservation QuotaReservation, status GrantStatus, now time.Time) error {
	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != reservation.ActivityID || q.Type != reservation.Type || q.Scope != reservation.Scope || q.ScopeKey != reservation.ScopeKey {
			continue
		}
		frozen := decimal.MustParse(q.FrozenValue)
		amount := decimal.MustParse(reservation.Amount)
		if frozen.Cmp(amount) < 0 {
			return ErrQuotaAggregateConflict
		}
		q.FrozenValue = frozen.Sub(amount).String()
		if status == GrantSuccess {
			used := decimal.MustParse(q.UsedValue)
			q.UsedValue = used.Add(amount).String()
		}
		q.Version++
		q.UpdatedAt = now
		return nil
	}
	return errors.Join(ErrQuotaAggregateConflict, errors.New("quota not found"))
}

func (r *MemoryRepository) finishReservationLocked(reservation *QuotaReservation, status GrantStatus, now time.Time) error {
	if err := r.finishReservationQuotaLocked(*reservation, status, now); err != nil {
		return err
	}
	reservation.UpdatedAt = now
	if status == GrantSuccess {
		reservation.Status = QuotaReservationConfirmed
		reservation.ConfirmedAt = &now
	} else {
		reservation.Status = QuotaReservationReleased
		reservation.ReleasedAt = &now
	}
	return nil
}

func (r *MemoryRepository) finishParticipationQuotaReservationsLocked(participationID string, status GrantStatus, now time.Time) error {
	for i := range r.quotaReservations {
		reservation := &r.quotaReservations[i]
		if !isParticipationQuotaReservation(*reservation) || reservation.Status != QuotaReservationFrozen {
			continue
		}
		if !r.grantOrderBelongsToParticipationLocked(reservation.GrantOrderID, participationID) {
			continue
		}
		if err := r.finishReservationLocked(reservation, status, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *MemoryRepository) grantOrderBelongsToParticipationLocked(grantOrderID string, participationID string) bool {
	for _, order := range r.grantOrders {
		if order.ID == grantOrderID && order.ParticipationID == participationID {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) updateParticipationAfterCallbackLocked(participationID string, now time.Time) ParticipationStatus {
	if participationID == "" {
		return ""
	}
	allFinal := true
	anySuccess := false
	anyFailed := false
	for _, order := range r.grantOrders {
		if order.ParticipationID != participationID {
			continue
		}
		switch order.GrantStatus {
		case GrantFailed, GrantClosed:
			anyFailed = true
		case GrantSuccess:
			anySuccess = true
		default:
			allFinal = false
		}
	}
	if !allFinal {
		return r.participationStatusLocked(participationID)
	}
	next := ParticipationGranted
	if anyFailed && !anySuccess {
		next = ParticipationFailed
	}
	for i := range r.participations {
		if r.participations[i].ID == participationID {
			r.participations[i].Status = next
			r.participations[i].UpdatedAt = now
			return next
		}
	}
	return ""
}

func (r *MemoryRepository) activityHadQuotaAtLocked(activityID string, at time.Time) bool {
	for _, quota := range r.quotas {
		if quota.ActivityID == activityID && (at.IsZero() || quota.CreatedAt.IsZero() || !quota.CreatedAt.After(at)) {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) hasFrozenReservationForGrantOrderLocked(grantOrderID string) bool {
	for _, reservation := range r.quotaReservations {
		if reservation.GrantOrderID == grantOrderID && reservation.Status == QuotaReservationFrozen {
			return true
		}
	}
	return false
}

func isFinalGrantStatus(status GrantStatus) bool {
	return status == GrantSuccess || status == GrantFailed || status == GrantClosed
}

func (r *MemoryRepository) grantOrderByCommandIDLocked(tenantID string, commandID string) *GrantOrder {
	for i := range r.grantOrders {
		if r.grantOrders[i].TenantID == tenantID && r.grantOrders[i].BillNo == commandID {
			return &r.grantOrders[i]
		}
	}
	return nil
}

func outboxRewardCommandID(payload json.RawMessage) string {
	return outboxRewardCommand(payload).CommandID
}

func outboxRewardCommand(payload json.RawMessage) RewardCommand {
	var command RewardCommand
	if err := json.Unmarshal(payload, &command); err != nil {
		return RewardCommand{}
	}
	command.CommandID = strings.TrimSpace(command.CommandID)
	command.ParticipationID = strings.TrimSpace(command.ParticipationID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.ActivityID = strings.TrimSpace(command.ActivityID)
	return command
}

func (r *MemoryRepository) participationStatusLocked(participationID string) ParticipationStatus {
	for _, participation := range r.participations {
		if participation.ID == participationID {
			return participation.Status
		}
	}
	return ""
}

func (r *MemoryRepository) participationLocked(participationID string) Participation {
	for _, participation := range r.participations {
		if participation.ID == participationID {
			return participation
		}
	}
	return Participation{}
}

func (r *MemoryRepository) inboxByIDLocked(inboxID string) EventInbox {
	for _, inbox := range r.inboxByIdempotency {
		if inbox.ID == inboxID {
			return inbox
		}
	}
	return EventInbox{}
}

func (r *MemoryRepository) outboxesForRewardLocked(commandID string, participationID string) []OutboxMessage {
	var out []OutboxMessage
	for _, message := range r.outboxMessages {
		if rawJSONContains(message.Payload, commandID) || rawJSONContains(message.Payload, participationID) {
			out = append(out, message)
		}
	}
	return out
}

func (r *MemoryRepository) auditTimelineLocked(tenantID string, relatedIDs []string) []AuditRecord {
	var out []AuditRecord
	for _, record := range r.auditRecords {
		if tenantID != "" && record.TenantID != "" && record.TenantID != tenantID {
			continue
		}
		for _, id := range relatedIDs {
			if id == "" {
				continue
			}
			if record.ResourceID == id || rawJSONContains(record.Payload, id) {
				out = append(out, record)
				break
			}
		}
	}
	return out
}

func auditRecordMatches(record AuditRecord, query AuditQuery) bool {
	if query.TenantID != "" && record.TenantID != query.TenantID {
		return false
	}
	if query.Actor != "" && record.Actor != query.Actor {
		return false
	}
	if query.Action != "" && record.Action != query.Action {
		return false
	}
	if query.Resource != "" && record.Resource != query.Resource {
		return false
	}
	if query.ResourceID != "" && record.ResourceID != query.ResourceID {
		return false
	}
	if query.ActivityID != "" && record.ResourceID != query.ActivityID && !rawJSONContains(record.Payload, query.ActivityID) {
		return false
	}
	if query.EventID != "" && record.ResourceID != query.EventID && !rawJSONContains(record.Payload, query.EventID) {
		return false
	}
	if query.CommandID != "" && record.ResourceID != query.CommandID && !rawJSONContains(record.Payload, query.CommandID) {
		return false
	}
	if !query.StartTime.IsZero() && record.CreatedAt.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && !record.CreatedAt.Before(query.EndTime) {
		return false
	}
	if query.TraceID != "" && auditPayloadSourceValue(record.Payload, "traceId") != query.TraceID {
		return false
	}
	if query.RequestID != "" && auditPayloadSourceValue(record.Payload, "requestId") != query.RequestID {
		return false
	}
	return true
}

func rawJSONContains(payload json.RawMessage, value string) bool {
	return value != "" && strings.Contains(string(payload), value)
}

func auditPayloadSourceValue(payload json.RawMessage, key string) string {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return ""
	}
	source, ok := body["source"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := source[key].(string)
	return value
}

func redactTargetRef(targetRef string) string {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return ""
	}
	if idx := strings.Index(targetRef, "?"); idx >= 0 {
		targetRef = targetRef[:idx]
	}
	if idx := strings.Index(targetRef, "#"); idx >= 0 {
		targetRef = targetRef[:idx]
	}
	for _, marker := range []string{"token=", "secret=", "password=", "authorization="} {
		if strings.Contains(strings.ToLower(targetRef), marker) {
			return "[REDACTED]"
		}
	}
	return targetRef
}

func outboxOperationAudit(ctx context.Context, tenantID string, operator string, action string, before OutboxMessage, after OutboxStatus, reason string, at time.Time) AuditRecord {
	source := AuditSourceFromContext(ctx)
	if len(source) == 0 {
		source = map[string]any{"component": "internal"}
	}
	payload, _ := json.Marshal(map[string]any{
		"outboxId":      before.ID,
		"operator":      operator,
		"reason":        reason,
		"beforeStatus":  before.SendStatus,
		"afterStatus":   after,
		"targetChannel": before.TargetChannel,
		"targetRef":     redactTargetRef(before.TargetRef),
		"retryCount":    before.RetryCount,
		"errorMessage":  before.ErrorMessage,
		"commandId":     outboxRewardCommandID(before.Payload),
		"operatedAt":    at,
		"source":        source,
	})
	return AuditRecord{
		ID:         NewID("audit"),
		TenantID:   tenantID,
		Actor:      operator,
		Action:     action,
		Resource:   "OUTBOX",
		ResourceID: before.ID,
		Payload:    payload,
		CreatedAt:  at,
	}
}

func participationIdempotencyExists(participations []Participation, key string) bool {
	if key == "" {
		return false
	}
	for _, existing := range participations {
		if existing.IdempotencyKey == key {
			return true
		}
	}
	return false
}

func grantOrderIdempotencyExists(orders []GrantOrder, key string) bool {
	if key == "" {
		return false
	}
	for _, existing := range orders {
		if existing.IdempotencyKey == key {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) releaseQuotaLocked(activityID string, quotaType QuotaType, scope QuotaScope, scopeKey string, amount string) {
	for i := range r.quotas {
		q := &r.quotas[i]
		if q.ActivityID != activityID || q.Type != quotaType || q.Scope != scope || q.ScopeKey != scopeKey {
			continue
		}
		frozen := decimal.MustParse(q.FrozenValue)
		decrement := decimal.MustParse(amount)
		q.FrozenValue = frozen.Sub(decrement).String()
		q.Version++
		q.UpdatedAt = time.Now().UTC()
		return
	}
}
