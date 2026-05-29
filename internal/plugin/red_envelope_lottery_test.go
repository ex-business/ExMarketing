package plugin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestRedEnvelopePluginValidatesAndBuildsAuditableHit(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	activity := samplePluginActivity(RedEnvelopePluginCode, redEnvelopeConfigJSON(now))
	event := samplePluginEvent("RED_ENVELOPE_CLAIM", "RED_ENVELOPE", now, nil)

	plugin := RedEnvelopePlugin{}
	if err := plugin.ValidateActivityConfig(sampleCreateCommand(RedEnvelopePluginCode, activity.Config)); err != nil {
		t.Fatalf("validate activity config: %v", err)
	}
	result, err := plugin.BuildContext(context.Background(), BuildContextRequest{Activity: activity, Event: event})
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !result.Precheck.Allowed || result.Context["hit"] != true || result.Context["candidateRewardCode"] != "REWARD_RED_PACKET" {
		t.Fatalf("unexpected red envelope result: precheck=%+v context=%+v", result.Precheck, result.Context)
	}
	if result.Explain["seedHash"] == "" || len(result.CandidateResults) != 1 {
		t.Fatalf("expected audit seed and candidate result: explain=%+v candidates=%+v", result.Explain, result.CandidateResults)
	}
	intents, err := plugin.AfterQualified(context.Background(), AfterQualifiedRequest{
		Activity:                    activity,
		Event:                       event,
		ParticipationID:             "part_1",
		ParticipationIdempotencyKey: "idem_part_1",
		PluginContext:               result.Context,
	})
	if err != nil {
		t.Fatalf("after qualified: %v", err)
	}
	if len(intents) != 1 || intents[0].IntentType != "recordRedEnvelopeClaim" || intents[0].Payload["candidateRewardCode"] != "REWARD_RED_PACKET" {
		t.Fatalf("unexpected intents: %+v", intents)
	}
}

func TestRedEnvelopePluginRejectsIllegalConfigAndLimits(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	plugin := RedEnvelopePlugin{}
	for name, raw := range map[string][]byte{
		"overlap":   []byte(`{"rounds":[{"roundCode":"r1","startAt":"2026-05-29T10:00:00Z","endAt":"2026-05-29T11:00:00Z","poolCode":"p","poolTotalCount":1,"perUserLimit":1,"emptyAllowed":true,"candidateRewards":[{"rewardCode":"REWARD_RED_PACKET"}]},{"roundCode":"r2","startAt":"2026-05-29T10:30:00Z","endAt":"2026-05-29T11:30:00Z","poolCode":"p","poolTotalCount":1,"perUserLimit":1,"emptyAllowed":true,"candidateRewards":[{"rewardCode":"REWARD_RED_PACKET"}]}]}`),
		"badReward": redEnvelopeConfigJSON(now),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "badReward" {
				cmd := sampleCreateCommand(RedEnvelopePluginCode, raw)
				cmd.Rewards[0].Code = "OTHER_REWARD"
				if err := plugin.ValidateActivityConfig(cmd); err == nil || !strings.Contains(err.Error(), "REWARD_RED_PACKET") {
					t.Fatalf("expected reward reference error, got %v", err)
				}
				return
			}
			if err := plugin.ValidateConfig(raw); err == nil {
				t.Fatalf("expected invalid config")
			}
		})
	}

	result, err := plugin.BuildContext(context.Background(), BuildContextRequest{
		Activity: samplePluginActivity(RedEnvelopePluginCode, redEnvelopeConfigJSON(now)),
		Event:    samplePluginEvent("RED_ENVELOPE_CLAIM", "RED_ENVELOPE", now, map[string]any{"claimCount": 1}),
	})
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if result.Precheck.Allowed || result.Precheck.ReasonCode != ReasonRedEnvelopeUserLimitReached {
		t.Fatalf("precheck=%+v, want limit reached", result.Precheck)
	}
}

func TestLotteryPluginValidatesAndBuildsDeterministicAudit(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	activity := samplePluginActivity(LotteryPluginCode, lotteryConfigJSON(now))
	event := samplePluginEvent("LOTTERY_DRAW", "LOTTERY", now, nil)

	plugin := LotteryPlugin{}
	if err := plugin.ValidateActivityConfig(sampleCreateCommand(LotteryPluginCode, activity.Config)); err != nil {
		t.Fatalf("validate activity config: %v", err)
	}
	left, err := plugin.BuildContext(context.Background(), BuildContextRequest{Activity: activity, Event: event})
	if err != nil {
		t.Fatalf("build left: %v", err)
	}
	right, err := plugin.BuildContext(context.Background(), BuildContextRequest{Activity: activity, Event: event})
	if err != nil {
		t.Fatalf("build right: %v", err)
	}
	if left.Context["itemCode"] != right.Context["itemCode"] || left.Explain["seedHash"] != right.Explain["seedHash"] {
		t.Fatalf("lottery result must be deterministic: left=%+v right=%+v", left, right)
	}
	if !left.Precheck.Allowed || left.Context["candidateRewardCode"] != "REWARD_LOTTERY" || len(left.CandidateResults) != 1 {
		t.Fatalf("unexpected lottery hit: precheck=%+v context=%+v candidates=%+v", left.Precheck, left.Context, left.CandidateResults)
	}
	intents, err := plugin.AfterQualified(context.Background(), AfterQualifiedRequest{
		Activity:                    activity,
		Event:                       event,
		ParticipationID:             "part_1",
		ParticipationIdempotencyKey: "idem_part_1",
		PluginContext:               left.Context,
	})
	if err != nil {
		t.Fatalf("after qualified: %v", err)
	}
	if len(intents) != 1 || intents[0].IntentType != "recordLotteryDraw" || intents[0].Payload["itemCode"] != "GOLD" {
		t.Fatalf("unexpected intents: %+v", intents)
	}
}

func TestLotteryPluginRejectsIllegalConfigAndDrawLimit(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	plugin := LotteryPlugin{}
	for name, raw := range map[string][]byte{
		"zeroWeight": []byte(`{"lotteryCode":"lotto","perUserDrawLimit":1,"items":[{"itemCode":"NO_PRIZE","weight":0,"noPrize":true}]}`),
		"duplicate":  []byte(`{"lotteryCode":"lotto","perUserDrawLimit":1,"items":[{"itemCode":"A","weight":1},{"itemCode":"A","weight":1}]}`),
		"badReward":  lotteryConfigJSON(now),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "badReward" {
				cmd := sampleCreateCommand(LotteryPluginCode, raw)
				cmd.Rewards[0].Code = "OTHER_REWARD"
				if err := plugin.ValidateActivityConfig(cmd); err == nil || !strings.Contains(err.Error(), "REWARD_LOTTERY") {
					t.Fatalf("expected reward reference error, got %v", err)
				}
				return
			}
			if err := plugin.ValidateConfig(raw); err == nil {
				t.Fatalf("expected invalid config")
			}
		})
	}

	result, err := plugin.BuildContext(context.Background(), BuildContextRequest{
		Activity: samplePluginActivity(LotteryPluginCode, lotteryConfigJSON(now)),
		Event:    samplePluginEvent("LOTTERY_DRAW", "LOTTERY", now, map[string]any{"drawCount": 1}),
	})
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if result.Precheck.Allowed || result.Precheck.ReasonCode != ReasonLotteryDrawLimitReached {
		t.Fatalf("precheck=%+v, want draw limit", result.Precheck)
	}
}

func samplePluginActivity(pluginCode string, config []byte) marketing.Activity {
	return marketing.Activity{
		ID:         "act_1",
		TenantID:   "site_001",
		Code:       pluginCode + "_ACTIVITY",
		Type:       pluginCode,
		Version:    1,
		PluginCode: pluginCode,
		Config:     config,
	}
}

func samplePluginEvent(eventType string, bizType string, occurredAt time.Time, attrs map[string]any) marketing.ActivityEvent {
	if attrs == nil {
		attrs = map[string]any{}
	}
	return marketing.ActivityEvent{
		EventID:      "evt_001",
		EventType:    eventType,
		SourceSystem: "app",
		TenantID:     "site_001",
		Subject:      marketing.EventSubject{Type: "USER", ID: "user_001"},
		Biz:          marketing.EventBiz{BizType: bizType, BizID: "biz_001"},
		OccurredAt:   occurredAt,
		Attributes:   attrs,
	}
}

func sampleCreateCommand(pluginCode string, config []byte) marketing.CreateActivityCommand {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	rewardCode := "REWARD_LOTTERY"
	if pluginCode == RedEnvelopePluginCode {
		rewardCode = "REWARD_RED_PACKET"
	}
	return marketing.CreateActivityCommand{
		TenantID:      "site_001",
		Code:          pluginCode + "_ACTIVITY",
		Name:          pluginCode + " Activity",
		Type:          pluginCode,
		TriggerEvents: []string{pluginCode + "_EVENT"},
		StartTime:     now.Add(-time.Hour),
		EndTime:       now.Add(time.Hour),
		Timezone:      "UTC",
		PluginCode:    pluginCode,
		Config:        config,
		Rewards: []marketing.Reward{{
			Code:          rewardCode,
			Type:          marketing.RewardCoupon,
			CalculateType: marketing.CalculateFixed,
			Config:        []byte(`{"amount":"8.00","currency":"CNY","targetRef":"https://wallet.example.com/reward"}`),
			GrantMode:     marketing.GrantAuto,
			GrantChannel:  marketing.GrantHTTP,
			Enabled:       true,
		}},
	}
}

func redEnvelopeConfigJSON(now time.Time) []byte {
	return []byte(`{"rounds":[{"roundCode":"morning","startAt":"` + now.Add(-time.Minute).Format(time.RFC3339) + `","endAt":"` + now.Add(time.Minute).Format(time.RFC3339) + `","poolCode":"pool_a","poolTotalCount":1,"perUserLimit":1,"emptyAllowed":false,"display":{"title":"Morning red envelope"},"candidateRewards":[{"candidateCode":"packet_1","rewardCode":"REWARD_RED_PACKET","displayName":"8 CNY"}]}]}`)
}

func lotteryConfigJSON(now time.Time) []byte {
	return []byte(`{"lotteryCode":"main","drawWindow":{"startAt":"` + now.Add(-time.Minute).Format(time.RFC3339) + `","endAt":"` + now.Add(time.Minute).Format(time.RFC3339) + `"},"perUserDrawLimit":1,"seedSalt":"public-demo","items":[{"itemCode":"GOLD","rewardCode":"REWARD_LOTTERY","weight":1,"displayName":"Gold prize"},{"itemCode":"NO_PRIZE","weight":0,"displayName":"No prize","noPrize":true}]}`)
}
