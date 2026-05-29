package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	RedEnvelopePluginCode = "RED_ENVELOPE"
	LotteryPluginCode     = "LOTTERY"

	ReasonRedEnvelopeNoRound          = "RED_ENVELOPE_NO_ACTIVE_ROUND"
	ReasonRedEnvelopeAlreadyClaimed   = "RED_ENVELOPE_ALREADY_CLAIMED"
	ReasonRedEnvelopeUserLimitReached = "RED_ENVELOPE_USER_LIMIT_REACHED"
	ReasonRedEnvelopeEmpty            = "EMPTY_ENVELOPE"
	ReasonRedEnvelopeHit              = "RED_ENVELOPE_HIT"
	ReasonLotteryNotStarted           = "LOTTERY_NOT_STARTED"
	ReasonLotteryEnded                = "LOTTERY_ENDED"
	ReasonLotteryDrawLimitReached     = "DRAW_LIMIT_REACHED"
	ReasonLotteryHit                  = "LOTTERY_HIT"
	ReasonLotteryNoPrize              = "NO_PRIZE"
)

type RedEnvelopePlugin struct{}

func (RedEnvelopePlugin) Code() string {
	return RedEnvelopePluginCode
}

func (RedEnvelopePlugin) CompatibleActivityTypes() []string {
	return []string{RedEnvelopePluginCode}
}

func (RedEnvelopePlugin) Metadata() Metadata {
	return Metadata{
		PluginCode:             RedEnvelopePluginCode,
		Version:                "1.0.0",
		Capabilities:           []string{CapabilityConfigValidate, CapabilityContextBuild, CapabilityBeforeMatch, CapabilityAfterRewardCalculated},
		SupportedActivityTypes: []string{RedEnvelopePluginCode},
		ConfigSchema:           json.RawMessage(redEnvelopeConfigSchema),
		StateSchema:            json.RawMessage(redEnvelopeStateSchema),
		DisableStrategies:      []string{DisabledStrategyBlock, DisabledStrategyReadOnly},
	}
}

func (p RedEnvelopePlugin) ValidateConfig(raw json.RawMessage) error {
	_, err := parseRedEnvelopeConfig(raw)
	return err
}

func (p RedEnvelopePlugin) ValidateActivityConfig(cmd marketing.CreateActivityCommand) error {
	cfg, err := parseRedEnvelopeConfig(cmd.ConfigPayload())
	if err != nil {
		return err
	}
	return validateRewardRefs(cmd.Rewards, redEnvelopeRewardRefs(cfg)...)
}

func (p RedEnvelopePlugin) BuildContext(_ context.Context, req BuildContextRequest) (ContextResult, error) {
	cfg, err := parseRedEnvelopeConfig(req.Activity.Config)
	if err != nil {
		return ContextResult{}, err
	}
	eventTime := req.Event.OccurredAt
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	round, ok := activeRedEnvelopeRound(cfg.Rounds, eventTime)
	context := map[string]any{
		"hit": false,
	}
	explain := map[string]any{
		"eventTime": eventTime.UTC().Format(time.RFC3339),
	}
	if !ok {
		context["roundStatus"] = "OUT_OF_WINDOW"
		return redEnvelopeRejected(context, explain, ReasonRedEnvelopeNoRound, "event is outside all red envelope rounds"), nil
	}

	context["roundCode"] = round.RoundCode
	context["roundStatus"] = "ACTIVE"
	context["poolCode"] = round.PoolCode
	context["poolTotalCount"] = round.PoolTotalCount
	context["emptyAllowed"] = round.EmptyAllowed
	context["display"] = round.Display
	explain["roundCode"] = round.RoundCode
	explain["poolCode"] = round.PoolCode

	if alreadyClaimed, _, valid := boolAttribute(req.Event.Attributes, "alreadyClaimed"); valid && alreadyClaimed {
		context["alreadyClaimed"] = true
		return redEnvelopeRejected(context, explain, ReasonRedEnvelopeAlreadyClaimed, "user already claimed this round"), nil
	}
	claimCount := intAttribute(req.Event.Attributes, "claimCount")
	if claimCount >= round.PerUserLimit {
		context["claimCount"] = claimCount
		return redEnvelopeRejected(context, explain, ReasonRedEnvelopeUserLimitReached, "red envelope per-user limit reached"), nil
	}

	position, seedHash := stablePosition(redEnvelopeSeed(req.Activity, req.Event, round), round.PoolTotalCount)
	context["alreadyClaimed"] = false
	context["claimPosition"] = position
	explain["seedHash"] = seedHash
	explain["position"] = position
	explain["idempotencyKey"] = req.Event.IdempotencyKey()

	candidate, hit := redEnvelopeCandidate(round, position)
	if !hit || strings.TrimSpace(candidate.RewardCode) == "" {
		context["hit"] = false
		context["reasonCode"] = ReasonRedEnvelopeEmpty
		return ContextResult{
			Context: context,
			Precheck: PrecheckResult{
				Allowed:    false,
				ReasonCode: ReasonRedEnvelopeEmpty,
				Reason:     "red envelope is empty",
			},
			Explain: explain,
		}, nil
	}
	context["hit"] = true
	context["candidateRewardCode"] = candidate.RewardCode
	context["candidateCode"] = firstNonEmptyString(candidate.CandidateCode, candidate.RewardCode)
	context["reasonCode"] = ReasonRedEnvelopeHit
	return ContextResult{
		Context: context,
		Precheck: PrecheckResult{
			Allowed:    true,
			ReasonCode: "OK",
			Reason:     "red envelope candidate selected",
		},
		Explain: explain,
		CandidateResults: []map[string]any{{
			"pluginCode":    RedEnvelopePluginCode,
			"roundCode":     round.RoundCode,
			"poolCode":      round.PoolCode,
			"candidateCode": firstNonEmptyString(candidate.CandidateCode, candidate.RewardCode),
			"rewardCode":    candidate.RewardCode,
			"hit":           true,
			"reasonCode":    ReasonRedEnvelopeHit,
		}},
	}, nil
}

func (p RedEnvelopePlugin) AfterQualified(_ context.Context, req AfterQualifiedRequest) ([]StateChangeIntent, error) {
	if !boolValue(req.PluginContext, "hit") {
		return nil, nil
	}
	roundCode := stringMapValue(req.PluginContext, "roundCode")
	return []StateChangeIntent{{
		PluginCode:                  RedEnvelopePluginCode,
		IntentType:                  "recordRedEnvelopeClaim",
		TenantID:                    req.Activity.TenantID,
		ActivityID:                  req.Activity.ID,
		ActivityCode:                req.Activity.Code,
		Subject:                     req.Event.Subject,
		Biz:                         req.Event.Biz,
		EventID:                     req.Event.EventID,
		ParticipationID:             req.ParticipationID,
		ParticipationIdempotencyKey: req.ParticipationIdempotencyKey,
		IdempotencyKey:              req.ParticipationIdempotencyKey + ":plugin:" + RedEnvelopePluginCode + ":round:" + roundCode,
		Payload: map[string]any{
			"roundCode":           roundCode,
			"poolCode":            stringMapValue(req.PluginContext, "poolCode"),
			"candidateRewardCode": stringMapValue(req.PluginContext, "candidateRewardCode"),
			"claimPosition":       req.PluginContext["claimPosition"],
		},
	}}, nil
}

type LotteryPlugin struct{}

func (LotteryPlugin) Code() string {
	return LotteryPluginCode
}

func (LotteryPlugin) CompatibleActivityTypes() []string {
	return []string{LotteryPluginCode}
}

func (LotteryPlugin) Metadata() Metadata {
	return Metadata{
		PluginCode:             LotteryPluginCode,
		Version:                "1.0.0",
		Capabilities:           []string{CapabilityConfigValidate, CapabilityContextBuild, CapabilityBeforeMatch, CapabilityAfterRewardCalculated},
		SupportedActivityTypes: []string{LotteryPluginCode},
		ConfigSchema:           json.RawMessage(lotteryConfigSchema),
		StateSchema:            json.RawMessage(lotteryStateSchema),
		DisableStrategies:      []string{DisabledStrategyBlock, DisabledStrategyReadOnly},
	}
}

func (p LotteryPlugin) ValidateConfig(raw json.RawMessage) error {
	_, err := parseLotteryConfig(raw)
	return err
}

func (p LotteryPlugin) ValidateActivityConfig(cmd marketing.CreateActivityCommand) error {
	cfg, err := parseLotteryConfig(cmd.ConfigPayload())
	if err != nil {
		return err
	}
	return validateRewardRefs(cmd.Rewards, lotteryRewardRefs(cfg)...)
}

func (p LotteryPlugin) BuildContext(_ context.Context, req BuildContextRequest) (ContextResult, error) {
	cfg, err := parseLotteryConfig(req.Activity.Config)
	if err != nil {
		return ContextResult{}, err
	}
	eventTime := req.Event.OccurredAt
	if eventTime.IsZero() {
		eventTime = time.Now().UTC()
	}
	context := map[string]any{
		"lotteryCode": cfg.LotteryCode,
		"hit":         false,
	}
	explain := map[string]any{
		"eventTime": eventTime.UTC().Format(time.RFC3339),
	}
	if !cfg.DrawWindow.StartAt.IsZero() && eventTime.Before(cfg.DrawWindow.StartAt) {
		return lotteryRejected(context, explain, ReasonLotteryNotStarted, "lottery window has not started"), nil
	}
	if !cfg.DrawWindow.EndAt.IsZero() && !eventTime.Before(cfg.DrawWindow.EndAt) {
		return lotteryRejected(context, explain, ReasonLotteryEnded, "lottery window has ended"), nil
	}
	drawCount := intAttribute(req.Event.Attributes, "drawCount")
	if drawCount >= cfg.PerUserDrawLimit {
		context["drawCount"] = drawCount
		return lotteryRejected(context, explain, ReasonLotteryDrawLimitReached, "lottery draw limit reached"), nil
	}
	drawIndex := drawCount + 1
	item, cursor, seedHash := chooseLotteryItem(cfg, req.Activity, req.Event, drawIndex)
	context["drawCount"] = drawCount
	context["drawIndex"] = drawIndex
	context["itemCode"] = item.ItemCode
	context["displayName"] = item.DisplayName
	context["noPrize"] = item.NoPrize
	context["reasonCode"] = ReasonLotteryHit
	explain["seedHash"] = seedHash
	explain["drawIndex"] = drawIndex
	explain["cursor"] = cursor
	explain["totalWeight"] = cfg.totalWeight.String()
	explain["idempotencyKey"] = req.Event.IdempotencyKey()

	if item.NoPrize || strings.TrimSpace(item.RewardCode) == "" || item.ItemCode == ReasonLotteryNoPrize {
		context["reasonCode"] = ReasonLotteryNoPrize
		return ContextResult{
			Context: context,
			Precheck: PrecheckResult{
				Allowed:    false,
				ReasonCode: ReasonLotteryNoPrize,
				Reason:     "lottery result is no prize",
			},
			Explain: explain,
		}, nil
	}
	context["hit"] = true
	context["candidateRewardCode"] = item.RewardCode
	return ContextResult{
		Context: context,
		Precheck: PrecheckResult{
			Allowed:    true,
			ReasonCode: "OK",
			Reason:     "lottery candidate selected",
		},
		Explain: explain,
		CandidateResults: []map[string]any{{
			"pluginCode":  LotteryPluginCode,
			"lotteryCode": cfg.LotteryCode,
			"itemCode":    item.ItemCode,
			"rewardCode":  item.RewardCode,
			"hit":         true,
			"reasonCode":  ReasonLotteryHit,
		}},
	}, nil
}

func (p LotteryPlugin) AfterQualified(_ context.Context, req AfterQualifiedRequest) ([]StateChangeIntent, error) {
	if !boolValue(req.PluginContext, "hit") {
		return nil, nil
	}
	return []StateChangeIntent{{
		PluginCode:                  LotteryPluginCode,
		IntentType:                  "recordLotteryDraw",
		TenantID:                    req.Activity.TenantID,
		ActivityID:                  req.Activity.ID,
		ActivityCode:                req.Activity.Code,
		Subject:                     req.Event.Subject,
		Biz:                         req.Event.Biz,
		EventID:                     req.Event.EventID,
		ParticipationID:             req.ParticipationID,
		ParticipationIdempotencyKey: req.ParticipationIdempotencyKey,
		IdempotencyKey:              req.ParticipationIdempotencyKey + ":plugin:" + LotteryPluginCode + ":draw:" + fmt.Sprint(req.PluginContext["drawIndex"]),
		Payload: map[string]any{
			"lotteryCode":         stringMapValue(req.PluginContext, "lotteryCode"),
			"drawIndex":           req.PluginContext["drawIndex"],
			"itemCode":            stringMapValue(req.PluginContext, "itemCode"),
			"candidateRewardCode": stringMapValue(req.PluginContext, "candidateRewardCode"),
		},
	}}, nil
}

type redEnvelopeConfig struct {
	Rounds []redEnvelopeRoundConfig `json:"rounds"`
}

type redEnvelopeRoundConfig struct {
	RoundCode        string                     `json:"roundCode"`
	StartAt          time.Time                  `json:"startAt"`
	EndAt            time.Time                  `json:"endAt"`
	PoolCode         string                     `json:"poolCode"`
	PoolTotalCount   int                        `json:"poolTotalCount"`
	EmptyAllowed     bool                       `json:"emptyAllowed"`
	PerUserLimit     int                        `json:"perUserLimit"`
	Display          map[string]string          `json:"display,omitempty"`
	CandidateRewards []redEnvelopeCandidateItem `json:"candidateRewards"`
}

type redEnvelopeCandidateItem struct {
	CandidateCode string `json:"candidateCode,omitempty"`
	RewardCode    string `json:"rewardCode,omitempty"`
	DisplayName   string `json:"displayName,omitempty"`
}

type lotteryConfig struct {
	LotteryCode      string              `json:"lotteryCode"`
	DrawWindow       lotteryDrawWindow   `json:"drawWindow,omitempty"`
	PerUserDrawLimit int                 `json:"perUserDrawLimit"`
	SeedSalt         string              `json:"seedSalt,omitempty"`
	Items            []lotteryItemConfig `json:"items"`
	totalWeight      *big.Rat
}

type lotteryDrawWindow struct {
	StartAt time.Time `json:"startAt,omitempty"`
	EndAt   time.Time `json:"endAt,omitempty"`
}

type lotteryItemConfig struct {
	ItemCode     string          `json:"itemCode"`
	RewardCode   string          `json:"rewardCode,omitempty"`
	Weight       json.RawMessage `json:"weight"`
	DisplayName  string          `json:"displayName,omitempty"`
	NoPrize      bool            `json:"noPrize,omitempty"`
	parsedWeight *big.Rat
}

func parseRedEnvelopeConfig(raw json.RawMessage) (redEnvelopeConfig, error) {
	var cfg redEnvelopeConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return cfg, err
	}
	if len(cfg.Rounds) == 0 {
		return cfg, errors.New("rounds is required")
	}
	seen := map[string]struct{}{}
	for i := range cfg.Rounds {
		round := &cfg.Rounds[i]
		round.RoundCode = strings.TrimSpace(round.RoundCode)
		round.PoolCode = strings.TrimSpace(round.PoolCode)
		if round.RoundCode == "" {
			return cfg, fmt.Errorf("rounds[%d].roundCode is required", i)
		}
		if _, ok := seen[round.RoundCode]; ok {
			return cfg, fmt.Errorf("rounds[%d].roundCode is duplicated", i)
		}
		seen[round.RoundCode] = struct{}{}
		if round.PoolCode == "" {
			return cfg, fmt.Errorf("rounds[%d].poolCode is required", i)
		}
		if round.StartAt.IsZero() || round.EndAt.IsZero() || !round.StartAt.Before(round.EndAt) {
			return cfg, fmt.Errorf("rounds[%d] requires startAt < endAt", i)
		}
		if round.PoolTotalCount <= 0 {
			return cfg, fmt.Errorf("rounds[%d].poolTotalCount must be positive", i)
		}
		if round.PerUserLimit == 0 {
			round.PerUserLimit = 1
		}
		if round.PerUserLimit <= 0 {
			return cfg, fmt.Errorf("rounds[%d].perUserLimit must be positive", i)
		}
		if len(round.CandidateRewards) == 0 {
			return cfg, fmt.Errorf("rounds[%d].candidateRewards is required", i)
		}
		for j := range round.CandidateRewards {
			candidate := &round.CandidateRewards[j]
			candidate.CandidateCode = strings.TrimSpace(candidate.CandidateCode)
			candidate.RewardCode = strings.TrimSpace(candidate.RewardCode)
			if candidate.CandidateCode == "" && candidate.RewardCode == "" {
				return cfg, fmt.Errorf("rounds[%d].candidateRewards[%d] requires candidateCode or rewardCode", i, j)
			}
		}
		if !round.EmptyAllowed && len(round.CandidateRewards) < round.PoolTotalCount {
			return cfg, fmt.Errorf("rounds[%d] emptyAllowed=false requires candidateRewards >= poolTotalCount", i)
		}
	}
	for i := 0; i < len(cfg.Rounds); i++ {
		for j := i + 1; j < len(cfg.Rounds); j++ {
			if cfg.Rounds[i].StartAt.Before(cfg.Rounds[j].EndAt) && cfg.Rounds[j].StartAt.Before(cfg.Rounds[i].EndAt) {
				return cfg, fmt.Errorf("rounds[%d] overlaps rounds[%d]", i, j)
			}
		}
	}
	return cfg, nil
}

func parseLotteryConfig(raw json.RawMessage) (lotteryConfig, error) {
	var cfg lotteryConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return cfg, err
	}
	cfg.LotteryCode = strings.TrimSpace(cfg.LotteryCode)
	if cfg.LotteryCode == "" {
		return cfg, errors.New("lotteryCode is required")
	}
	if !cfg.DrawWindow.StartAt.IsZero() && !cfg.DrawWindow.EndAt.IsZero() && !cfg.DrawWindow.StartAt.Before(cfg.DrawWindow.EndAt) {
		return cfg, errors.New("drawWindow requires startAt < endAt")
	}
	if cfg.PerUserDrawLimit == 0 {
		cfg.PerUserDrawLimit = 1
	}
	if cfg.PerUserDrawLimit <= 0 {
		return cfg, errors.New("perUserDrawLimit must be positive")
	}
	if len(cfg.Items) == 0 {
		return cfg, errors.New("items is required")
	}
	seen := map[string]struct{}{}
	total := new(big.Rat)
	for i := range cfg.Items {
		item := &cfg.Items[i]
		item.ItemCode = strings.TrimSpace(item.ItemCode)
		item.RewardCode = strings.TrimSpace(item.RewardCode)
		if item.ItemCode == "" {
			return cfg, fmt.Errorf("items[%d].itemCode is required", i)
		}
		if _, ok := seen[item.ItemCode]; ok {
			return cfg, fmt.Errorf("items[%d].itemCode is duplicated", i)
		}
		seen[item.ItemCode] = struct{}{}
		weight, err := parseJSONRat(item.Weight)
		if err != nil {
			return cfg, fmt.Errorf("items[%d].weight is invalid", i)
		}
		if weight.Sign() < 0 {
			return cfg, fmt.Errorf("items[%d].weight must not be negative", i)
		}
		item.parsedWeight = weight
		total.Add(total, weight)
	}
	if total.Sign() <= 0 {
		return cfg, errors.New("items total weight must be positive")
	}
	cfg.totalWeight = total
	return cfg, nil
}

func decodeStrict(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("config is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func validateRewardRefs(rewards []marketing.Reward, refs ...string) error {
	enabled := map[string]struct{}{}
	for _, reward := range rewards {
		if reward.Enabled {
			enabled[strings.TrimSpace(reward.Code)] = struct{}{}
		}
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := enabled[ref]; !ok {
			return fmt.Errorf("rewardCode %s is not an enabled activity reward", ref)
		}
	}
	return nil
}

func redEnvelopeRewardRefs(cfg redEnvelopeConfig) []string {
	var refs []string
	for _, round := range cfg.Rounds {
		for _, candidate := range round.CandidateRewards {
			if candidate.RewardCode != "" {
				refs = append(refs, candidate.RewardCode)
			}
		}
	}
	return refs
}

func lotteryRewardRefs(cfg lotteryConfig) []string {
	var refs []string
	for _, item := range cfg.Items {
		if !item.NoPrize && item.RewardCode != "" {
			refs = append(refs, item.RewardCode)
		}
	}
	return refs
}

func activeRedEnvelopeRound(rounds []redEnvelopeRoundConfig, eventTime time.Time) (redEnvelopeRoundConfig, bool) {
	for _, round := range rounds {
		if !eventTime.Before(round.StartAt) && eventTime.Before(round.EndAt) {
			return round, true
		}
	}
	return redEnvelopeRoundConfig{}, false
}

func redEnvelopeCandidate(round redEnvelopeRoundConfig, position int) (redEnvelopeCandidateItem, bool) {
	if len(round.CandidateRewards) == 0 {
		return redEnvelopeCandidateItem{}, false
	}
	if round.EmptyAllowed {
		if position > len(round.CandidateRewards) {
			return redEnvelopeCandidateItem{}, false
		}
		return round.CandidateRewards[position-1], true
	}
	index := (position - 1) % len(round.CandidateRewards)
	return round.CandidateRewards[index], true
}

func chooseLotteryItem(cfg lotteryConfig, activity marketing.Activity, event marketing.ActivityEvent, drawIndex int) (lotteryItemConfig, string, string) {
	seed := strings.Join([]string{
		event.TenantID,
		activity.ID,
		fmt.Sprint(activity.Version),
		LotteryPluginCode,
		cfg.LotteryCode,
		event.Subject.Type,
		event.Subject.ID,
		event.IdempotencyKey(),
		fmt.Sprint(drawIndex),
		cfg.SeedSalt,
	}, "|")
	sum := sha256.Sum256([]byte(seed))
	n := binary.BigEndian.Uint64(sum[:8]) % 1_000_000_000
	cursor := new(big.Rat).Mul(cfg.totalWeight, new(big.Rat).SetUint64(n))
	cursor.Quo(cursor, big.NewRat(1_000_000_000, 1))
	acc := new(big.Rat)
	for _, item := range cfg.Items {
		acc.Add(acc, item.parsedWeight)
		if cursor.Cmp(acc) < 0 {
			return item, cursor.FloatString(8), hex.EncodeToString(sum[:])
		}
	}
	return cfg.Items[len(cfg.Items)-1], cursor.FloatString(8), hex.EncodeToString(sum[:])
}

func stablePosition(seed string, total int) (int, string) {
	sum := sha256.Sum256([]byte(seed))
	value := binary.BigEndian.Uint64(sum[:8])
	return int(value%uint64(total)) + 1, hex.EncodeToString(sum[:])
}

func redEnvelopeSeed(activity marketing.Activity, event marketing.ActivityEvent, round redEnvelopeRoundConfig) string {
	return strings.Join([]string{
		event.TenantID,
		activity.ID,
		fmt.Sprint(activity.Version),
		RedEnvelopePluginCode,
		round.RoundCode,
		event.Subject.Type,
		event.Subject.ID,
		event.IdempotencyKey(),
	}, "|")
}

func redEnvelopeRejected(context map[string]any, explain map[string]any, reasonCode string, reason string) ContextResult {
	context["reasonCode"] = reasonCode
	return ContextResult{
		Context: context,
		Precheck: PrecheckResult{
			Allowed:    false,
			ReasonCode: reasonCode,
			Reason:     reason,
		},
		Explain: explain,
	}
}

func lotteryRejected(context map[string]any, explain map[string]any, reasonCode string, reason string) ContextResult {
	context["reasonCode"] = reasonCode
	return ContextResult{
		Context: context,
		Precheck: PrecheckResult{
			Allowed:    false,
			ReasonCode: reasonCode,
			Reason:     reason,
		},
		Explain: explain,
	}
}

func parseJSONRat(raw json.RawMessage) (*big.Rat, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil, errors.New("weight is required")
	}
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal(raw, &unquoted); err != nil {
			return nil, err
		}
		text = strings.TrimSpace(unquoted)
	}
	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return rat, nil
}

func intAttribute(attributes map[string]any, key string) int {
	value, ok := attributes[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func boolValue(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	typed, _ := value.(bool)
	return typed
}

func stringMapValue(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var _ ActivityPlugin = RedEnvelopePlugin{}
var _ MetadataProvider = RedEnvelopePlugin{}
var _ ActivityConfigValidator = RedEnvelopePlugin{}
var _ ActivityPlugin = LotteryPlugin{}
var _ MetadataProvider = LotteryPlugin{}
var _ ActivityConfigValidator = LotteryPlugin{}

const redEnvelopeConfigSchema = `{"type":"object","required":["rounds"],"additionalProperties":false}`
const redEnvelopeStateSchema = `{"type":"object","properties":{"roundCode":{"type":"string"},"claimPosition":{"type":"integer"},"candidateRewardCode":{"type":"string"}}}`
const lotteryConfigSchema = `{"type":"object","required":["lotteryCode","items"],"additionalProperties":false}`
const lotteryStateSchema = `{"type":"object","properties":{"lotteryCode":{"type":"string"},"drawIndex":{"type":"integer"},"itemCode":{"type":"string"},"candidateRewardCode":{"type":"string"}}}`
