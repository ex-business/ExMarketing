package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestNewPluginContextBuildsReadOnlySnapshot(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	activity := marketing.Activity{
		ID:         "act_1",
		TenantID:   "site_001",
		Code:       "lottery_1",
		Type:       "LOTTERY",
		Version:    3,
		PluginCode: "LOTTERY_TEST",
		Config:     []byte(`{"maxDraw":1}`),
	}
	event := pluginStateEvent("evt_1", "round_1")
	event.Attributes["amount"] = "20.00"
	facts := map[string]any{"vipLevel": "gold"}

	snapshot := NewPluginContext(activity, event, []marketing.ActivityRuleResult{{
		ActivityID:      activity.ID,
		ActivityVersion: activity.Version,
		Decision:        marketing.RuleDecisionPassed,
		CanReward:       true,
	}}, facts, now, "trace_1")

	event.Attributes["amount"] = "999.00"
	facts["vipLevel"] = "bronze"
	activity.Config[0] = '{'

	if snapshot.Event.IdempotencyKey != "site_001:game:ROUND_PLAY:ROUND:round_1" {
		t.Fatalf("unexpected idempotency key: %s", snapshot.Event.IdempotencyKey)
	}
	if snapshot.Event.Attributes["amount"] != "20.00" || snapshot.Facts["vipLevel"] != "gold" {
		t.Fatalf("snapshot should not share mutable maps: event=%+v facts=%+v", snapshot.Event.Attributes, snapshot.Facts)
	}
	if string(snapshot.Activity.Config) != `{"maxDraw":1}` {
		t.Fatalf("snapshot should not share config bytes: %s", string(snapshot.Activity.Config))
	}
	if !snapshot.Now.Equal(now) || snapshot.TraceID != "trace_1" {
		t.Fatalf("unexpected now/trace: %+v", snapshot)
	}
}

func TestMemoryPluginStateStoreRejectsIncompleteScopeAndIsolatesReads(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryPluginStateStore()
	scope := pluginStateScope("site_001", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw")
	meta := pluginStateMeta("evt_1", "idem_1")

	if _, err := store.PutIfAbsent(ctx, PluginStateScope{TenantID: "site_001"}, map[string]any{"count": 1}, meta); !errors.Is(err, ErrPluginStateInvalidScope) {
		t.Fatalf("expected invalid scope, got %v", err)
	}
	if _, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 1}, PluginStateWriteMeta{EventID: "evt_1"}); !errors.Is(err, ErrPluginStateIdempotencyRequired) {
		t.Fatalf("expected idempotency required, got %v", err)
	}
	if _, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 1}, meta); err != nil {
		t.Fatalf("put: %v", err)
	}

	for name, queryScope := range map[string]PluginStateScope{
		"tenant":  pluginStateScope("site_002", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw"),
		"plugin":  pluginStateScope("site_001", "act_1", 1, "OTHER_PLUGIN", "USER:user_1", "ROUND:round_1", "draw"),
		"version": pluginStateScope("site_001", "act_1", 2, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw"),
		"key":     pluginStateScope("site_001", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "other"),
	} {
		if _, ok, err := store.Get(ctx, queryScope); err != nil || ok {
			t.Fatalf("%s should be isolated, ok=%v err=%v", name, ok, err)
		}
	}

	got, ok, err := store.Get(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("get exact scope: ok=%v err=%v", ok, err)
	}
	got.Value["count"] = float64(99)
	again, _, _ := store.Get(ctx, scope)
	if again.Value["count"] != float64(1) {
		t.Fatalf("get should return a defensive copy, got %+v", again.Value)
	}
}

func TestMemoryPluginStateStorePutIfAbsentIdempotency(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryPluginStateStore()
	scope := pluginStateScope("site_001", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw")
	meta := pluginStateMeta("evt_1", "idem_1")

	created, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 1}, meta)
	if err != nil || created.Result != PluginStateResultCreated || created.Record.Version != 1 {
		t.Fatalf("unexpected create result=%+v err=%v", created, err)
	}
	duplicate, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 1}, meta)
	if err != nil || duplicate.Result != PluginStateResultDuplicateNoop || duplicate.Record.Version != 1 {
		t.Fatalf("unexpected duplicate result=%+v err=%v", duplicate, err)
	}
	if len(store.Audits()) != 1 {
		t.Fatalf("duplicate no-op should not create a second business-state audit, got %+v", store.Audits())
	}

	conflict, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 2}, meta)
	if !errors.Is(err, ErrPluginStateIdempotencyConflict) || conflict.Result != PluginStateResultConflict {
		t.Fatalf("expected idempotency conflict, result=%+v err=%v", conflict, err)
	}
	if _, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 1}, pluginStateMeta("evt_2", "idem_2")); !errors.Is(err, ErrPluginStateExists) {
		t.Fatalf("expected state exists for different idempotency key, got %v", err)
	}
}

func TestMemoryPluginStateStoreCompareAndSwapConcurrentUpdates(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryPluginStateStore()
	scope := pluginStateScope("site_001", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw")
	if _, err := store.PutIfAbsent(ctx, scope, map[string]any{"count": 0}, pluginStateMeta("evt_seed", "idem_seed")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const workers = 24
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				current, ok, err := store.Get(ctx, scope)
				if err != nil || !ok {
					errs <- fmt.Errorf("get: ok=%v err=%w", ok, err)
					return
				}
				nextCount := int(current.Value["count"].(float64)) + 1
				_, err = store.CompareAndSwap(ctx, scope, current.Version, map[string]any{"count": nextCount}, pluginStateMeta(fmt.Sprintf("evt_%d", i), fmt.Sprintf("idem_%d", i)))
				if err == nil {
					return
				}
				if !errors.Is(err, ErrPluginStateVersionConflict) {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	final, ok, err := store.Get(ctx, scope)
	if err != nil || !ok {
		t.Fatalf("get final: ok=%v err=%v", ok, err)
	}
	if final.Value["count"] != float64(workers) || final.Version != workers+1 {
		t.Fatalf("CAS lost updates: record=%+v", final)
	}
}

func TestMemoryPluginStateStoreAppendEventIdempotencyAndListByRound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryPluginStateStore()
	scope := pluginStateScope("site_001", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw_events")
	otherTenant := pluginStateScope("site_002", "act_1", 1, "LOTTERY_TEST", "USER:user_1", "ROUND:round_1", "draw_events")

	const events = 20
	var wg sync.WaitGroup
	for i := 0; i < events; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta := pluginStateMeta(fmt.Sprintf("evt_%d", i), fmt.Sprintf("idem_%d", i))
			if _, err := store.AppendEvent(ctx, scope, "DRAW", map[string]any{"index": i}, meta); err != nil {
				t.Errorf("append %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	duplicate, err := store.AppendEvent(ctx, scope, "DRAW", map[string]any{"index": 7}, pluginStateMeta("evt_7", "idem_7"))
	if err != nil || duplicate.Result != PluginStateResultDuplicateNoop {
		t.Fatalf("unexpected duplicate append result=%+v err=%v", duplicate, err)
	}
	if _, err := store.AppendEvent(ctx, otherTenant, "DRAW", map[string]any{"index": "other"}, pluginStateMeta("evt_other", "idem_other")); err != nil {
		t.Fatalf("append other tenant: %v", err)
	}

	round, err := store.ListByRound(ctx, PluginStateRoundQuery{
		TenantID:        "site_001",
		ActivityID:      "act_1",
		ActivityVersion: 1,
		PluginCode:      "LOTTERY_TEST",
		Biz:             "ROUND:round_1",
	})
	if err != nil {
		t.Fatalf("list round: %v", err)
	}
	if len(round.Events) != events {
		t.Fatalf("expected %d events for tenant scope, got %+v", events, round.Events)
	}
	for _, event := range round.Events {
		if event.Scope.TenantID != "site_001" || event.Scope.PluginCode != "LOTTERY_TEST" || event.Scope.Biz != "ROUND:round_1" {
			t.Fatalf("list leaked cross-scope event: %+v", event)
		}
	}
	if _, err := store.ListByRound(ctx, PluginStateRoundQuery{TenantID: "site_001"}); !errors.Is(err, ErrPluginStateInvalidScope) {
		t.Fatalf("expected list query to require full tenant/activity/version/plugin/biz scope, got %v", err)
	}
}

func pluginStateScope(tenantID string, activityID string, version int, pluginCode string, subject string, biz string, stateKey string) PluginStateScope {
	return PluginStateScope{
		TenantID:        tenantID,
		ActivityID:      activityID,
		ActivityVersion: version,
		PluginCode:      pluginCode,
		Subject:         subject,
		Biz:             biz,
		StateKey:        stateKey,
	}
}

func pluginStateMeta(eventID string, idempotencyKey string) PluginStateWriteMeta {
	return PluginStateWriteMeta{
		EventID:        eventID,
		IdempotencyKey: idempotencyKey,
		TraceID:        "trace_1",
		LifecycleStep:  "AFTER_REWARD_CALCULATED",
		Now:            time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC),
	}
}

func pluginStateEvent(eventID string, bizID string) marketing.ActivityEvent {
	return marketing.ActivityEvent{
		EventID:      eventID,
		EventType:    "ROUND_PLAY",
		SourceSystem: "game",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "user_1"},
		Biz:          marketing.EventBiz{BizType: "ROUND", BizID: bizID},
		OccurredAt:   time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC),
		Attributes:   map[string]any{},
	}
}
