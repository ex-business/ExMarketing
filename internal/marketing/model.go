package marketing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

type ActivityStatus string
type RulePhase string
type RuleStage string
type RuleType string
type RuleDecision string
type RewardType string
type CalculateType string
type GrantMode string
type GrantChannel string
type InboxStatus string
type ParticipationStatus string
type GrantStatus string
type OutboxStatus string
type QuotaType string
type QuotaScope string

const (
	ActivityDraft     ActivityStatus = "DRAFT"
	ActivityScheduled ActivityStatus = "SCHEDULED"
	ActivityActive    ActivityStatus = "ACTIVE"
	ActivityPaused    ActivityStatus = "PAUSED"
	ActivityEnded     ActivityStatus = "ENDED"

	RulePhaseMatch   RulePhase = "MATCH"
	RulePhaseQualify RulePhase = "QUALIFY"
	RulePhaseReward  RulePhase = "REWARD"
	RulePhaseRisk    RulePhase = "RISK"
	RulePhaseLimit   RulePhase = "LIMIT"

	RuleStageMatch           RuleStage = "MATCH"
	RuleStageQualify         RuleStage = "QUALIFY"
	RuleStageReward          RuleStage = "REWARD"
	RuleStageRiskLimitBudget RuleStage = "RISK_LIMIT_BUDGET"

	RuleTypeCondition RuleType = "CONDITION"
	RuleTypeFormula   RuleType = "FORMULA"
	RuleTypeLimit     RuleType = "LIMIT"

	RuleDecisionPassed   RuleDecision = "PASSED"
	RuleDecisionRejected RuleDecision = "REJECTED"
	RuleDecisionSkipped  RuleDecision = "SKIPPED"
	RuleDecisionError    RuleDecision = "ERROR"

	RewardBonus      RewardType = "BONUS"
	RewardCash       RewardType = "CASH"
	RewardCoupon     RewardType = "COUPON"
	RewardPoint      RewardType = "POINT"
	RewardGift       RewardType = "GIFT"
	RewardDrawChance RewardType = "DRAW_CHANCE"
	RewardMembership RewardType = "MEMBERSHIP"

	CalculateFixed       CalculateType = "FIXED"
	CalculateRatio       CalculateType = "RATIO"
	CalculateLadder      CalculateType = "LADDER"
	CalculateRandom      CalculateType = "RANDOM"
	CalculateProbability CalculateType = "PROBABILITY"
	CalculatePoolSplit   CalculateType = "POOL_SPLIT"

	GrantAuto   GrantMode = "AUTO"
	GrantManual GrantMode = "MANUAL"

	GrantHTTP         GrantChannel = "HTTP"
	GrantMQ           GrantChannel = "MQ"
	GrantWebhook      GrantChannel = "WEBHOOK"
	GrantManualExport GrantChannel = "MANUAL_EXPORT"

	InboxReceived  InboxStatus = "RECEIVED"
	InboxProcessed InboxStatus = "PROCESSED"
	InboxDuplicate InboxStatus = "DUPLICATE"
	InboxFailed    InboxStatus = "FAILED"

	ParticipationInit       ParticipationStatus = "INIT"
	ParticipationQualified  ParticipationStatus = "QUALIFIED"
	ParticipationRejected   ParticipationStatus = "REJECTED"
	ParticipationRewardable ParticipationStatus = "REWARDABLE"
	ParticipationGranted    ParticipationStatus = "GRANTED"
	ParticipationFailed     ParticipationStatus = "FAILED"

	GrantPending GrantStatus = "PENDING"
	GrantSending GrantStatus = "SENDING"
	GrantSuccess GrantStatus = "SUCCESS"
	GrantFailed  GrantStatus = "FAILED"
	GrantRetry   GrantStatus = "RETRY"
	GrantClosed  GrantStatus = "CLOSED"

	OutboxPending   OutboxStatus = "PENDING"
	OutboxSending   OutboxStatus = "SENDING"
	OutboxSent      OutboxStatus = "SENT"
	OutboxFailed    OutboxStatus = "FAILED"
	OutboxDLQ       OutboxStatus = "DLQ"
	OutboxDLQClosed OutboxStatus = "DLQ_CLOSED"

	QuotaBudget     QuotaType = "BUDGET"
	QuotaStock      QuotaType = "STOCK"
	QuotaUserLimit  QuotaType = "USER_LIMIT"
	QuotaDailyLimit QuotaType = "DAILY_LIMIT"

	QuotaScopeActivity QuotaScope = "ACTIVITY"
	QuotaScopeUser     QuotaScope = "USER"
	QuotaScopeIP       QuotaScope = "IP"
	QuotaScopeDevice   QuotaScope = "DEVICE"
)

type Activity struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenantId"`
	Code           string          `json:"activityCode"`
	Name           string          `json:"activityName"`
	Type           string          `json:"activityType"`
	Status         ActivityStatus  `json:"status"`
	TriggerEvents  []string        `json:"triggerEvents"`
	StartTime      time.Time       `json:"startTime"`
	EndTime        time.Time       `json:"endTime"`
	Timezone       string          `json:"timezone"`
	Priority       int             `json:"priority"`
	ExclusiveGroup string          `json:"exclusiveGroup"`
	Stackable      bool            `json:"stackable"`
	Version        int             `json:"version"`
	Config         json.RawMessage `json:"config,omitempty"`
	Rules          []Rule          `json:"rules"`
	Rewards        []Reward        `json:"rewards"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type Rule struct {
	ID        string          `json:"id"`
	Code      string          `json:"ruleCode"`
	Phase     RulePhase       `json:"rulePhase"`
	Type      RuleType        `json:"ruleType"`
	Config    json.RawMessage `json:"ruleConfig"`
	Sort      int             `json:"sort"`
	Enabled   bool            `json:"enabled"`
	Version   int             `json:"version"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type RuleExecutionResult struct {
	RuleCode   string         `json:"ruleCode"`
	RulePhase  RulePhase      `json:"rulePhase"`
	RuleType   RuleType       `json:"ruleType"`
	Status     RuleDecision   `json:"status"`
	Passed     bool           `json:"passed"`
	ReasonCode string         `json:"reasonCode"`
	Reason     string         `json:"reason"`
	Field      string         `json:"field,omitempty"`
	Expected   any            `json:"expected,omitempty"`
	Actual     any            `json:"actual,omitempty"`
	Sort       int            `json:"sort"`
	Detail     map[string]any `json:"detail,omitempty"`
}

type RuleStageResult struct {
	Stage         RuleStage             `json:"stage"`
	Status        RuleDecision          `json:"status"`
	ExecutedCount int                   `json:"executedCount"`
	SkippedCount  int                   `json:"skippedCount"`
	FailedCount   int                   `json:"failedCount"`
	ReasonCode    string                `json:"reasonCode,omitempty"`
	Reason        string                `json:"reason,omitempty"`
	RuleResults   []RuleExecutionResult `json:"ruleResults"`
}

type ActivityRuleResult struct {
	ActivityID      string            `json:"activityId"`
	ActivityCode    string            `json:"activityCode"`
	ActivityVersion int               `json:"activityVersion"`
	Decision        RuleDecision      `json:"decision"`
	FailedStage     RuleStage         `json:"failedStage,omitempty"`
	ReasonCode      string            `json:"reasonCode,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	CanReward       bool              `json:"canReward"`
	CanReserveQuota bool              `json:"canReserveQuota"`
	StageResults    []RuleStageResult `json:"stageResults"`
}

func (r *Rule) UnmarshalJSON(data []byte) error {
	type alias Rule
	aux := struct {
		alias
		Enabled *bool `json:"enabled"`
	}{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&aux); err != nil {
		return err
	}
	*r = Rule(aux.alias)
	if aux.Enabled == nil {
		r.Enabled = true
	} else {
		r.Enabled = *aux.Enabled
	}
	return nil
}

type ConditionRuleConfig struct {
	Logic      string      `json:"logic"`
	Conditions []Condition `json:"conditions"`
}

type Condition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

type Reward struct {
	ID            string          `json:"id"`
	Code          string          `json:"rewardCode"`
	Type          RewardType      `json:"rewardType"`
	CalculateType CalculateType   `json:"calculateType"`
	Config        json.RawMessage `json:"rewardConfig"`
	GrantMode     GrantMode       `json:"grantMode"`
	GrantChannel  GrantChannel    `json:"grantChannel"`
	FlowMultiple  string          `json:"flowMultiple"`
	Enabled       bool            `json:"enabled"`
	Version       int             `json:"version"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

func (r *Reward) UnmarshalJSON(data []byte) error {
	type alias Reward
	aux := struct {
		alias
		Enabled *bool `json:"enabled"`
	}{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&aux); err != nil {
		return err
	}
	*r = Reward(aux.alias)
	if aux.Enabled == nil {
		r.Enabled = true
	} else {
		r.Enabled = *aux.Enabled
	}
	return nil
}

type RewardConfig struct {
	Amount                string            `json:"amount,omitempty"`
	BaseField             string            `json:"baseField,omitempty"`
	Rate                  string            `json:"rate,omitempty"`
	MaxAmount             string            `json:"maxAmount,omitempty"`
	Field                 string            `json:"field,omitempty"`
	Ladders               []RewardLadder    `json:"ladders,omitempty"`
	MinAmount             string            `json:"minAmount,omitempty"`
	Step                  string            `json:"step,omitempty"`
	SeedSalt              string            `json:"seedSalt,omitempty"`
	Items                 []ProbabilityItem `json:"items,omitempty"`
	PoolAmount            string            `json:"poolAmount,omitempty"`
	SplitBy               string            `json:"splitBy,omitempty"`
	ParticipantCountField string            `json:"participantCountField,omitempty"`
	WeightField           string            `json:"weightField,omitempty"`
	TotalWeightField      string            `json:"totalWeightField,omitempty"`
	Currency              string            `json:"currency,omitempty"`
	ExpireAt              string            `json:"expireAt,omitempty"`
	FlowMultiple          string            `json:"flowMultiple,omitempty"`
	TargetRef             string            `json:"targetRef,omitempty"`
	RewardPayload         any               `json:"rewardPayload,omitempty"`
}

type RewardLadder struct {
	Min    string `json:"min"`
	Max    string `json:"max"`
	Amount string `json:"amount"`
}

type ProbabilityItem struct {
	ItemCode   string     `json:"itemCode"`
	Weight     string     `json:"weight"`
	RewardType RewardType `json:"rewardType,omitempty"`
	Amount     string     `json:"amount"`
	Currency   string     `json:"currency,omitempty"`
	Label      string     `json:"label,omitempty"`
}

type ActivityEvent struct {
	EventID      string         `json:"eventId"`
	EventType    string         `json:"eventType"`
	SourceSystem string         `json:"sourceSystem"`
	TenantID     string         `json:"tenantId"`
	Subject      EventSubject   `json:"subject"`
	Biz          EventBiz       `json:"biz"`
	OccurredAt   time.Time      `json:"occurredAt"`
	Attributes   map[string]any `json:"attributes"`
}

type EventSubject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type EventBiz struct {
	BizID   string `json:"bizId"`
	BizType string `json:"bizType"`
}

type EventInbox struct {
	ID             string        `json:"id"`
	Event          ActivityEvent `json:"event"`
	IdempotencyKey string        `json:"idempotencyKey"`
	ProcessStatus  InboxStatus   `json:"processStatus"`
	ErrorMessage   string        `json:"errorMessage,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

type Participation struct {
	ID             string              `json:"id"`
	TenantID       string              `json:"tenantId"`
	ActivityID     string              `json:"activityId"`
	EventInboxID   string              `json:"eventInboxId"`
	Subject        EventSubject        `json:"subject"`
	Biz            EventBiz            `json:"biz"`
	Status         ParticipationStatus `json:"status"`
	RejectReason   string              `json:"rejectReason,omitempty"`
	Snapshot       json.RawMessage     `json:"snapshot"`
	IdempotencyKey string              `json:"idempotencyKey"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
}

type GrantOrder struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId"`
	ActivityID      string          `json:"activityId"`
	ParticipationID string          `json:"participationId"`
	Subject         EventSubject    `json:"subject"`
	RewardType      RewardType      `json:"rewardType"`
	RewardAmount    string          `json:"rewardAmount"`
	RewardPayload   json.RawMessage `json:"rewardPayload"`
	BillNo          string          `json:"billNo"`
	GrantStatus     GrantStatus     `json:"grantStatus"`
	GrantChannel    GrantChannel    `json:"grantChannel"`
	RetryCount      int             `json:"retryCount"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

type RewardCommand struct {
	CommandID       string         `json:"commandId"`
	TenantID        string         `json:"tenantId"`
	ActivityID      string         `json:"activityId"`
	ParticipationID string         `json:"participationId"`
	Subject         EventSubject   `json:"subject"`
	Reward          map[string]any `json:"reward"`
	Reason          map[string]any `json:"reason"`
	IdempotencyKey  string         `json:"idempotencyKey"`
}

type OutboxMessage struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenantId"`
	MessageType   string          `json:"messageType"`
	TargetChannel GrantChannel    `json:"targetChannel"`
	TargetRef     string          `json:"targetRef"`
	Payload       json.RawMessage `json:"payload"`
	SendStatus    OutboxStatus    `json:"sendStatus"`
	RetryCount    int             `json:"retryCount"`
	NextRetryAt   *time.Time      `json:"nextRetryAt,omitempty"`
	ErrorMessage  string          `json:"errorMessage,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type DLQOperationCommand struct {
	TenantID string `json:"tenantId"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type Quota struct {
	ID          string     `json:"id"`
	ActivityID  string     `json:"activityId"`
	Type        QuotaType  `json:"quotaType"`
	Scope       QuotaScope `json:"scope"`
	ScopeKey    string     `json:"scopeKey"`
	TotalValue  string     `json:"totalValue"`
	UsedValue   string     `json:"usedValue"`
	FrozenValue string     `json:"frozenValue"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type QuotaReservation struct {
	Type     QuotaType  `json:"quotaType"`
	Scope    QuotaScope `json:"scope"`
	ScopeKey string     `json:"scopeKey"`
	Amount   string     `json:"amount"`
}

type AtomicGrantCommand struct {
	Participation     Participation
	BudgetAmount      string
	ReserveUserLimit  bool
	UserScopeKey      string
	StockReservations []QuotaReservation
	GrantOrders       []GrantOrder
	OutboxMessages    []OutboxMessage
}

type AtomicGrantResult struct {
	Participation  Participation
	GrantOrders    []GrantOrder
	OutboxMessages []OutboxMessage
}

type QuotaExceededError struct {
	Type     QuotaType
	Scope    QuotaScope
	ScopeKey string
	Amount   string
}

func (e QuotaExceededError) Error() string {
	parts := []string{"quota exceeded"}
	if e.Type != "" {
		parts = append(parts, string(e.Type))
	}
	if e.Scope != "" {
		parts = append(parts, string(e.Scope))
	}
	if e.ScopeKey != "" {
		parts = append(parts, e.ScopeKey)
	}
	return strings.Join(parts, ": ")
}

func (e QuotaExceededError) Unwrap() error {
	return ErrQuotaExceeded
}

type CreateQuotaCommand struct {
	ActivityID string     `json:"activityId"`
	Type       QuotaType  `json:"quotaType"`
	Scope      QuotaScope `json:"scope"`
	ScopeKey   string     `json:"scopeKey"`
	TotalValue string     `json:"totalValue"`
}

type AuditRecord struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenantId"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	Resource   string          `json:"resource"`
	ResourceID string          `json:"resourceId"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"createdAt"`
}

type FactProvider struct {
	Code            string            `json:"factCode"`
	Provider        string            `json:"provider"`
	Endpoint        string            `json:"endpoint"`
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers,omitempty"`
	CacheTTLSeconds int               `json:"cacheTtlSeconds"`
}

type FactQueryRequest struct {
	Provider FactProvider  `json:"provider"`
	Event    ActivityEvent `json:"event"`
}

type FactQueryResponse struct {
	FactCode string         `json:"factCode"`
	Data     map[string]any `json:"data"`
}

type SimulationResult struct {
	Event          ActivityEvent        `json:"event"`
	Participations []Participation      `json:"participations"`
	GrantOrders    []GrantOrder         `json:"grantOrders"`
	OutboxMessages []OutboxMessage      `json:"outboxMessages"`
	RuleResults    []ActivityRuleResult `json:"ruleResults,omitempty"`
}

type ProcessEventResult struct {
	Inbox          EventInbox           `json:"inbox"`
	Participations []Participation      `json:"participations"`
	GrantOrders    []GrantOrder         `json:"grantOrders"`
	OutboxMessages []OutboxMessage      `json:"outboxMessages"`
	RuleResults    []ActivityRuleResult `json:"ruleResults,omitempty"`
	Duplicate      bool                 `json:"duplicate"`
}

type RewardCallback struct {
	CommandID       string      `json:"commandId"`
	TenantID        string      `json:"tenantId"`
	Status          GrantStatus `json:"status"`
	ExternalGrantID string      `json:"externalGrantId"`
	Message         string      `json:"message"`
	FinishedAt      time.Time   `json:"finishedAt"`
}

type CreateActivityCommand struct {
	TenantID       string          `json:"tenantId"`
	Code           string          `json:"activityCode"`
	Name           string          `json:"activityName"`
	Type           string          `json:"activityType"`
	Status         ActivityStatus  `json:"status"`
	TriggerEvents  []string        `json:"triggerEvents"`
	StartTime      time.Time       `json:"startTime"`
	EndTime        time.Time       `json:"endTime"`
	Timezone       string          `json:"timezone"`
	Priority       int             `json:"priority"`
	ExclusiveGroup string          `json:"exclusiveGroup"`
	Stackable      bool            `json:"stackable"`
	Version        int             `json:"version"`
	Config         json.RawMessage `json:"config,omitempty"`
	Rules          []Rule          `json:"rules"`
	Rewards        []Reward        `json:"rewards"`
}

var (
	ErrInvalidActivity     = errors.New("invalid activity")
	ErrInvalidEvent        = errors.New("invalid event")
	ErrDuplicateEvent      = errors.New("duplicate event")
	ErrIdempotencyConflict = errors.New("idempotency key payload conflict")
	ErrNotFound            = errors.New("not found")
	ErrQuotaExceeded       = errors.New("quota exceeded")
	ErrInvalidOperation    = errors.New("invalid operation")
)

func (cmd CreateActivityCommand) Validate(now time.Time) error {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(cmd.Code) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("activityCode is required"))
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("activityName is required"))
	}
	if strings.TrimSpace(cmd.Type) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("activityType is required"))
	}
	if len(cmd.TriggerEvents) == 0 {
		return errors.Join(ErrInvalidActivity, errors.New("triggerEvents is required"))
	}
	seenEvents := map[string]struct{}{}
	for i, eventType := range cmd.TriggerEvents {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			return invalidActivityf("triggerEvents[%d] is required", i)
		}
		if _, ok := seenEvents[eventType]; ok {
			return invalidActivityf("triggerEvents[%d] is duplicated", i)
		}
		seenEvents[eventType] = struct{}{}
	}
	if cmd.StartTime.IsZero() || cmd.EndTime.IsZero() || !cmd.EndTime.After(cmd.StartTime) {
		return errors.Join(ErrInvalidActivity, errors.New("valid startTime and endTime are required"))
	}
	if cmd.EndTime.Before(now) {
		return errors.Join(ErrInvalidActivity, errors.New("endTime must not be in the past"))
	}
	if strings.TrimSpace(cmd.Timezone) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("timezone is required"))
	}
	if _, err := time.LoadLocation(cmd.Timezone); err != nil {
		return invalidActivityf("timezone is invalid")
	}
	if cmd.Status != "" && !validActivityStatus(cmd.Status) {
		return invalidActivityf("status is invalid")
	}
	if cmd.Version > 1 {
		return invalidActivityf("version must be 0 or 1")
	}
	if err := validateActivityConfig(cmd.Config); err != nil {
		return err
	}
	for i, rule := range cmd.Rules {
		if err := validateRule(i, rule); err != nil {
			return err
		}
	}
	if len(cmd.Rewards) == 0 {
		return errors.Join(ErrInvalidActivity, errors.New("at least one reward is required"))
	}
	enabledRewards := 0
	for i, reward := range cmd.Rewards {
		if err := validateReward(i, reward); err != nil {
			return err
		}
		if reward.Enabled {
			enabledRewards++
		}
	}
	if enabledRewards == 0 {
		return errors.Join(ErrInvalidActivity, errors.New("at least one enabled reward is required"))
	}
	return nil
}

func invalidActivityf(format string, args ...any) error {
	return errors.Join(ErrInvalidActivity, fmt.Errorf(format, args...))
}

func validActivityStatus(status ActivityStatus) bool {
	switch status {
	case ActivityDraft, ActivityScheduled, ActivityActive, ActivityPaused, ActivityEnded:
		return true
	default:
		return false
	}
}

type activityConfig struct {
	Entry    *entryConfig    `json:"entry,omitempty"`
	Display  *displayConfig  `json:"display,omitempty"`
	Audience *audienceConfig `json:"audience,omitempty"`
	Metadata map[string]any  `json:"metadata,omitempty"`
}

type entryConfig struct {
	Channels      []string `json:"channels,omitempty"`
	EntranceCodes []string `json:"entranceCodes,omitempty"`
	LandingURL    string   `json:"landingUrl,omitempty"`
}

type displayConfig struct {
	Title       string `json:"title,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	BannerURL   string `json:"bannerUrl,omitempty"`
	TermsURL    string `json:"termsUrl,omitempty"`
	VisibleFrom string `json:"visibleFrom,omitempty"`
	VisibleTo   string `json:"visibleTo,omitempty"`
	Sort        *int   `json:"sort,omitempty"`
}

type audienceConfig struct {
	SegmentCodes []string `json:"segmentCodes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Countries    []string `json:"countries,omitempty"`
}

func validateActivityConfig(raw json.RawMessage) error {
	if emptyOrNullJSON(raw) {
		return nil
	}
	var cfg activityConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("config is invalid: %v", err)
	}
	if cfg.Entry != nil {
		if err := validateStringSlice("config.entry.channels", cfg.Entry.Channels); err != nil {
			return err
		}
		if err := validateStringSlice("config.entry.entranceCodes", cfg.Entry.EntranceCodes); err != nil {
			return err
		}
		if err := validateHTTPURL("config.entry.landingUrl", cfg.Entry.LandingURL, false); err != nil {
			return err
		}
	}
	if cfg.Display != nil {
		if err := validateHTTPURL("config.display.bannerUrl", cfg.Display.BannerURL, false); err != nil {
			return err
		}
		if err := validateHTTPURL("config.display.termsUrl", cfg.Display.TermsURL, false); err != nil {
			return err
		}
		from, fromOK, err := parseOptionalTime("config.display.visibleFrom", cfg.Display.VisibleFrom)
		if err != nil {
			return err
		}
		to, toOK, err := parseOptionalTime("config.display.visibleTo", cfg.Display.VisibleTo)
		if err != nil {
			return err
		}
		if fromOK && toOK && from.After(to) {
			return invalidActivityf("config.display.visibleFrom must be before or equal to visibleTo")
		}
	}
	if cfg.Audience != nil {
		if err := validateStringSlice("config.audience.segmentCodes", cfg.Audience.SegmentCodes); err != nil {
			return err
		}
		if err := validateStringSlice("config.audience.tags", cfg.Audience.Tags); err != nil {
			return err
		}
		if err := validateStringSlice("config.audience.countries", cfg.Audience.Countries); err != nil {
			return err
		}
	}
	return nil
}

func validateRule(index int, rule Rule) error {
	if strings.TrimSpace(rule.Code) == "" {
		return invalidActivityf("rules[%d].ruleCode is required", index)
	}
	if rule.Phase != RulePhaseMatch && rule.Phase != RulePhaseQualify && rule.Phase != RulePhaseReward && rule.Phase != RulePhaseRisk && rule.Phase != RulePhaseLimit {
		return invalidActivityf("rules[%d].rulePhase is invalid", index)
	}
	if rule.Type != RuleTypeCondition {
		return invalidActivityf("rules[%d].ruleType is unsupported", index)
	}
	if rule.Version > 1 {
		return invalidActivityf("rules[%d].version must be 0 or 1", index)
	}
	var cfg ConditionRuleConfig
	if err := decodeStrict(rule.Config, &cfg); err != nil {
		return invalidActivityf("rules[%d].ruleConfig is invalid: %v", index, err)
	}
	logic := strings.ToUpper(strings.TrimSpace(cfg.Logic))
	if logic != "" && logic != "AND" && logic != "OR" {
		return invalidActivityf("rules[%d].ruleConfig.logic is invalid", index)
	}
	for i, condition := range cfg.Conditions {
		if strings.TrimSpace(condition.Field) == "" {
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].field is required", index, i)
		}
		if !supportedRuleField(condition.Field) {
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].field is unsupported", index, i)
		}
		op := strings.ToUpper(strings.TrimSpace(condition.Op))
		switch op {
		case "=", "==", "!=", "<>":
		case ">", ">=", "<", "<=":
			if _, err := decimal.Parse(jsonScalarString(condition.Value)); err != nil {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value must be decimal", index, i)
			}
		case "IN", "NOT_IN":
			if _, ok := condition.Value.([]any); !ok {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value must be array", index, i)
			}
		default:
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].op is invalid", index, i)
		}
	}
	return nil
}

func validateReward(index int, reward Reward) error {
	if strings.TrimSpace(reward.Code) == "" {
		return invalidActivityf("rewards[%d].rewardCode is required", index)
	}
	switch reward.Type {
	case RewardBonus, RewardCash, RewardPoint:
	default:
		return invalidActivityf("rewards[%d].rewardType is unsupported", index)
	}
	if !validRewardCalculateType(reward.CalculateType) {
		return invalidActivityf("rewards[%d].calculateType is unsupported", index)
	}
	if reward.GrantMode != GrantAuto {
		return invalidActivityf("rewards[%d].grantMode is unsupported", index)
	}
	if reward.GrantChannel != GrantHTTP && reward.GrantChannel != GrantWebhook {
		return invalidActivityf("rewards[%d].grantChannel is unsupported", index)
	}
	if reward.Version > 1 {
		return invalidActivityf("rewards[%d].version must be 0 or 1", index)
	}
	var cfg RewardConfig
	if err := decodeStrict(reward.Config, &cfg); err != nil {
		return invalidActivityf("rewards[%d].rewardConfig is invalid: %v", index, err)
	}
	if err := validateHTTPURL(fmt.Sprintf("rewards[%d].rewardConfig.targetRef", index), cfg.TargetRef, true); err != nil {
		return err
	}
	if (reward.Type == RewardBonus || reward.Type == RewardCash) && strings.TrimSpace(cfg.Currency) == "" {
		return invalidActivityf("rewards[%d].rewardConfig.currency is required", index)
	}
	if reward.FlowMultiple != "" && cfg.FlowMultiple != "" && reward.FlowMultiple != cfg.FlowMultiple {
		return invalidActivityf("rewards[%d].flowMultiple conflicts with rewardConfig.flowMultiple", index)
	}
	if reward.FlowMultiple != "" {
		if _, err := decimal.Parse(reward.FlowMultiple); err != nil {
			return invalidActivityf("rewards[%d].flowMultiple must be decimal", index)
		}
	}
	if cfg.FlowMultiple != "" {
		if _, err := decimal.Parse(cfg.FlowMultiple); err != nil {
			return invalidActivityf("rewards[%d].rewardConfig.flowMultiple must be decimal", index)
		}
	}
	return validateRewardConfigByCalculateType(index, reward, cfg)
}

func validRewardCalculateType(calculateType CalculateType) bool {
	switch calculateType {
	case CalculateFixed, CalculateRatio, CalculateLadder, CalculateRandom, CalculateProbability, CalculatePoolSplit:
		return true
	default:
		return false
	}
}

func validateRewardConfigByCalculateType(index int, reward Reward, cfg RewardConfig) error {
	switch reward.CalculateType {
	case CalculateFixed:
		return validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.amount", index), cfg.Amount)
	case CalculateRatio:
		return validateRatioRewardConfig(index, cfg)
	case CalculateLadder:
		return validateLadderRewardConfig(index, cfg)
	case CalculateRandom:
		return validateRandomRewardConfig(index, cfg)
	case CalculateProbability:
		return validateProbabilityRewardConfig(index, reward, cfg)
	case CalculatePoolSplit:
		return validatePoolSplitRewardConfig(index, cfg)
	default:
		return invalidActivityf("rewards[%d].calculateType is unsupported", index)
	}
}

func validateRatioRewardConfig(index int, cfg RewardConfig) error {
	if strings.TrimSpace(cfg.BaseField) == "" {
		return invalidActivityf("rewards[%d].rewardConfig.baseField is required", index)
	}
	if !supportedRuleField(cfg.BaseField) {
		return invalidActivityf("rewards[%d].rewardConfig.baseField is unsupported", index)
	}
	if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.rate", index), cfg.Rate); err != nil {
		return err
	}
	if cfg.MaxAmount != "" {
		if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.maxAmount", index), cfg.MaxAmount); err != nil {
			return err
		}
	}
	return nil
}

func validateLadderRewardConfig(index int, cfg RewardConfig) error {
	if strings.TrimSpace(cfg.Field) == "" {
		return invalidActivityf("rewards[%d].rewardConfig.field is required", index)
	}
	if !supportedRuleField(cfg.Field) {
		return invalidActivityf("rewards[%d].rewardConfig.field is unsupported", index)
	}
	if len(cfg.Ladders) == 0 {
		return invalidActivityf("rewards[%d].rewardConfig.ladders is required", index)
	}
	type interval struct {
		min decimal.Decimal
		max decimal.Decimal
	}
	var intervals []interval
	for i, ladder := range cfg.Ladders {
		min, err := parseNonNegativeConfigDecimal(fmt.Sprintf("rewards[%d].rewardConfig.ladders[%d].min", index, i), ladder.Min)
		if err != nil {
			return err
		}
		max, err := parseNonNegativeConfigDecimal(fmt.Sprintf("rewards[%d].rewardConfig.ladders[%d].max", index, i), ladder.Max)
		if err != nil {
			return err
		}
		if min.Cmp(max) > 0 {
			return invalidActivityf("rewards[%d].rewardConfig.ladders[%d].min must be <= max", index, i)
		}
		if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.ladders[%d].amount", index, i), ladder.Amount); err != nil {
			return err
		}
		intervals = append(intervals, interval{min: min, max: max})
	}
	for i := range intervals {
		for j := i + 1; j < len(intervals); j++ {
			if intervals[i].min.Cmp(intervals[j].max) <= 0 && intervals[j].min.Cmp(intervals[i].max) <= 0 {
				return invalidActivityf("rewards[%d].rewardConfig.ladders overlap", index)
			}
		}
	}
	return nil
}

func validateRandomRewardConfig(index int, cfg RewardConfig) error {
	min, err := parseNonNegativeConfigDecimal(fmt.Sprintf("rewards[%d].rewardConfig.minAmount", index), cfg.MinAmount)
	if err != nil {
		return err
	}
	max, err := parseNonNegativeConfigDecimal(fmt.Sprintf("rewards[%d].rewardConfig.maxAmount", index), cfg.MaxAmount)
	if err != nil {
		return err
	}
	if min.Cmp(max) > 0 {
		return invalidActivityf("rewards[%d].rewardConfig.minAmount must be <= maxAmount", index)
	}
	stepText := strings.TrimSpace(cfg.Step)
	if stepText == "" {
		stepText = "0.01"
	}
	step, err := decimal.Parse(stepText)
	if err != nil || !step.IsPositive() {
		return invalidActivityf("rewards[%d].rewardConfig.step must be positive decimal", index)
	}
	if min.Cmp(max) < 0 && step.Cmp(max.Sub(min)) > 0 {
		return invalidActivityf("rewards[%d].rewardConfig.step must not exceed range", index)
	}
	return nil
}

func validateProbabilityRewardConfig(index int, reward Reward, cfg RewardConfig) error {
	if len(cfg.Items) == 0 {
		return invalidActivityf("rewards[%d].rewardConfig.items is required", index)
	}
	seen := map[string]struct{}{}
	total := decimal.Zero()
	for i, item := range cfg.Items {
		if strings.TrimSpace(item.ItemCode) == "" {
			return invalidActivityf("rewards[%d].rewardConfig.items[%d].itemCode is required", index, i)
		}
		if _, ok := seen[item.ItemCode]; ok {
			return invalidActivityf("rewards[%d].rewardConfig.items[%d].itemCode is duplicated", index, i)
		}
		seen[item.ItemCode] = struct{}{}
		weight, err := decimal.Parse(item.Weight)
		if err != nil || !weight.IsPositive() {
			return invalidActivityf("rewards[%d].rewardConfig.items[%d].weight must be positive decimal", index, i)
		}
		total = total.Add(weight)
		if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.items[%d].amount", index, i), item.Amount); err != nil {
			return err
		}
		if item.RewardType != "" {
			switch item.RewardType {
			case RewardBonus, RewardCash, RewardPoint:
			default:
				return invalidActivityf("rewards[%d].rewardConfig.items[%d].rewardType is unsupported", index, i)
			}
		}
		if item.Currency != "" && cfg.Currency != "" && item.Currency != cfg.Currency {
			return invalidActivityf("rewards[%d].rewardConfig.items[%d].currency conflicts with rewardConfig.currency", index, i)
		}
		if (item.RewardType == RewardBonus || item.RewardType == RewardCash || item.RewardType == "" && (reward.Type == RewardBonus || reward.Type == RewardCash)) && cfg.Currency == "" {
			return invalidActivityf("rewards[%d].rewardConfig.currency is required", index)
		}
	}
	if !total.IsPositive() {
		return invalidActivityf("rewards[%d].rewardConfig.items total weight must be positive", index)
	}
	return nil
}

func validatePoolSplitRewardConfig(index int, cfg RewardConfig) error {
	if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.poolAmount", index), cfg.PoolAmount); err != nil {
		return err
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.SplitBy)) {
	case "COUNT":
		if strings.TrimSpace(cfg.ParticipantCountField) == "" {
			return invalidActivityf("rewards[%d].rewardConfig.participantCountField is required", index)
		}
		if !supportedRuleField(cfg.ParticipantCountField) {
			return invalidActivityf("rewards[%d].rewardConfig.participantCountField is unsupported", index)
		}
	case "WEIGHT":
		if strings.TrimSpace(cfg.WeightField) == "" || strings.TrimSpace(cfg.TotalWeightField) == "" {
			return invalidActivityf("rewards[%d].rewardConfig.weightField and totalWeightField are required", index)
		}
		if !supportedRuleField(cfg.WeightField) || !supportedRuleField(cfg.TotalWeightField) {
			return invalidActivityf("rewards[%d].rewardConfig weight fields are unsupported", index)
		}
	default:
		return invalidActivityf("rewards[%d].rewardConfig.splitBy is unsupported", index)
	}
	return nil
}

func decodeStrict(raw json.RawMessage, dst any) error {
	if emptyOrNullJSON(raw) {
		return errors.New("must be object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func emptyOrNullJSON(raw json.RawMessage) bool {
	text := strings.TrimSpace(string(raw))
	return text == "" || text == "null"
}

func validateStringSlice(name string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return invalidActivityf("%s[%d] is required", name, i)
		}
	}
	return nil
}

func validateHTTPURL(name, value string, required bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return invalidActivityf("%s is required", name)
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return invalidActivityf("%s must be http or https URL", name)
	}
	return nil
}

func parseOptionalTime(name, value string) (time.Time, bool, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false, invalidActivityf("%s must be RFC3339 time", name)
	}
	return parsed, true, nil
}

func supportedRuleField(field string) bool {
	if strings.HasPrefix(field, "attributes.") && len(field) > len("attributes.") {
		return true
	}
	if strings.HasPrefix(field, "state.") && len(field) > len("state.") {
		return true
	}
	if strings.HasPrefix(field, "facts.") && len(field) > len("facts.") {
		return true
	}
	switch field {
	case "eventId", "eventType", "sourceSystem", "tenantId", "occurredAt", "subject.type", "subject.id", "biz.bizType", "biz.bizId":
		return true
	default:
		return false
	}
}

func jsonScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		bytesValue, _ := json.Marshal(typed)
		return string(bytesValue)
	}
}

func validatePositiveDecimal(name, value string) error {
	parsed, err := decimal.Parse(value)
	if err != nil || parsed.Cmp(decimal.Zero()) <= 0 {
		return invalidActivityf("%s must be positive decimal", name)
	}
	return nil
}

func validateNonNegativeDecimal(name, value string) error {
	_, err := parseNonNegativeConfigDecimal(name, value)
	return err
}

func parseNonNegativeConfigDecimal(name, value string) (decimal.Decimal, error) {
	parsed, err := decimal.Parse(value)
	if err != nil {
		return decimal.Decimal{}, invalidActivityf("%s must be decimal", name)
	}
	if parsed.IsNegative() {
		return decimal.Decimal{}, invalidActivityf("%s must be non-negative decimal", name)
	}
	return parsed, nil
}

func (event *ActivityEvent) Validate() error {
	if event.EventID == "" || event.EventType == "" || event.SourceSystem == "" || event.TenantID == "" {
		return errors.Join(ErrInvalidEvent, errors.New("eventId, eventType, sourceSystem and tenantId are required"))
	}
	if event.Subject.Type == "" || event.Subject.ID == "" {
		return errors.Join(ErrInvalidEvent, errors.New("subject.type and subject.id are required"))
	}
	if event.Biz.BizType == "" || event.Biz.BizID == "" {
		return errors.Join(ErrInvalidEvent, errors.New("biz.bizType and biz.bizId are required"))
	}
	if event.OccurredAt.IsZero() {
		return errors.Join(ErrInvalidEvent, errors.New("occurredAt is required"))
	}
	if event.Attributes == nil {
		event.Attributes = map[string]any{}
	}
	return nil
}

func (event ActivityEvent) Normalized() ActivityEvent {
	event.OccurredAt = event.OccurredAt.UTC()
	if event.Attributes == nil {
		event.Attributes = map[string]any{}
	}
	return event
}

func (event ActivityEvent) SameStandardPayload(other ActivityEvent) bool {
	left := canonicalEventPayload(event.Normalized())
	right := canonicalEventPayload(other.Normalized())
	return reflect.DeepEqual(left, right)
}

func (event ActivityEvent) IdempotencyKey() string {
	return strings.Join([]string{
		event.TenantID,
		event.SourceSystem,
		event.EventType,
		event.Biz.BizType,
		event.Biz.BizID,
	}, ":")
}

func canonicalEventPayload(event ActivityEvent) map[string]any {
	return map[string]any{
		"eventId":      event.EventID,
		"eventType":    event.EventType,
		"sourceSystem": event.SourceSystem,
		"tenantId":     event.TenantID,
		"subject": map[string]any{
			"type": event.Subject.Type,
			"id":   event.Subject.ID,
		},
		"biz": map[string]any{
			"bizType": event.Biz.BizType,
			"bizId":   event.Biz.BizID,
		},
		"occurredAt": event.OccurredAt.Format(time.RFC3339Nano),
		"attributes": canonicalJSONValue(event.Attributes),
	}
}

func canonicalJSONValue(value any) any {
	bytesValue, err := json.Marshal(value)
	if err != nil {
		return value
	}
	decoder := json.NewDecoder(bytes.NewReader(bytesValue))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return value
	}
	return normalizeJSONValue(decoded)
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeJSONValue(item)
		}
		return out
	case json.Number:
		number, err := decimal.Parse(typed.String())
		if err != nil {
			return typed.String()
		}
		return map[string]string{"$number": number.String()}
	default:
		return typed
	}
}

func (cmd CreateQuotaCommand) Validate() error {
	if strings.TrimSpace(cmd.ActivityID) == "" {
		return errors.New("activityId is required")
	}
	if cmd.Type == "" || cmd.Scope == "" || strings.TrimSpace(cmd.ScopeKey) == "" {
		return errors.New("quotaType, scope and scopeKey are required")
	}
	value, err := decimal.Parse(cmd.TotalValue)
	if err != nil || value.Cmp(decimal.Zero()) <= 0 {
		return errors.New("totalValue must be a positive decimal")
	}
	return nil
}
