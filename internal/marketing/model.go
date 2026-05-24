package marketing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/decimal"
)

type ActivityStatus string
type RulePhase string
type RuleType string
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

	RuleTypeCondition RuleType = "CONDITION"
	RuleTypeFormula   RuleType = "FORMULA"
	RuleTypeLimit     RuleType = "LIMIT"

	RewardBonus      RewardType = "BONUS"
	RewardCash       RewardType = "CASH"
	RewardCoupon     RewardType = "COUPON"
	RewardPoint      RewardType = "POINT"
	RewardGift       RewardType = "GIFT"
	RewardDrawChance RewardType = "DRAW_CHANCE"
	RewardMembership RewardType = "MEMBERSHIP"

	CalculateFixed CalculateType = "FIXED"
	CalculateRatio CalculateType = "RATIO"

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

	OutboxPending OutboxStatus = "PENDING"
	OutboxSending OutboxStatus = "SENDING"
	OutboxSent    OutboxStatus = "SENT"
	OutboxFailed  OutboxStatus = "FAILED"
	OutboxDLQ     OutboxStatus = "DLQ"

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
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

type RewardConfig struct {
	Amount        string `json:"amount,omitempty"`
	BaseField     string `json:"baseField,omitempty"`
	Rate          string `json:"rate,omitempty"`
	MaxAmount     string `json:"maxAmount,omitempty"`
	Currency      string `json:"currency,omitempty"`
	ExpireAt      string `json:"expireAt,omitempty"`
	FlowMultiple  string `json:"flowMultiple,omitempty"`
	TargetRef     string `json:"targetRef,omitempty"`
	RewardPayload any    `json:"rewardPayload,omitempty"`
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
	Event          ActivityEvent   `json:"event"`
	Participations []Participation `json:"participations"`
	GrantOrders    []GrantOrder    `json:"grantOrders"`
	OutboxMessages []OutboxMessage `json:"outboxMessages"`
}

type ProcessEventResult struct {
	Inbox          EventInbox      `json:"inbox"`
	Participations []Participation `json:"participations"`
	GrantOrders    []GrantOrder    `json:"grantOrders"`
	OutboxMessages []OutboxMessage `json:"outboxMessages"`
	Duplicate      bool            `json:"duplicate"`
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
	Config         json.RawMessage `json:"config,omitempty"`
	Rules          []Rule          `json:"rules"`
	Rewards        []Reward        `json:"rewards"`
}

var (
	ErrInvalidActivity = errors.New("invalid activity")
	ErrInvalidEvent    = errors.New("invalid event")
	ErrDuplicateEvent  = errors.New("duplicate event")
	ErrNotFound        = errors.New("not found")
	ErrQuotaExceeded   = errors.New("quota exceeded")
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
	if cmd.StartTime.IsZero() || cmd.EndTime.IsZero() || !cmd.EndTime.After(cmd.StartTime) {
		return errors.Join(ErrInvalidActivity, errors.New("valid startTime and endTime are required"))
	}
	if cmd.EndTime.Before(now) {
		return errors.Join(ErrInvalidActivity, errors.New("endTime must not be in the past"))
	}
	if strings.TrimSpace(cmd.Timezone) == "" {
		return errors.Join(ErrInvalidActivity, errors.New("timezone is required"))
	}
	if len(cmd.Rewards) == 0 {
		return errors.Join(ErrInvalidActivity, errors.New("at least one reward is required"))
	}
	return nil
}

func (event ActivityEvent) Validate() error {
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

func (event ActivityEvent) IdempotencyKey() string {
	return strings.Join([]string{
		event.TenantID,
		event.SourceSystem,
		event.EventType,
		event.Biz.BizType,
		event.Biz.BizID,
	}, ":")
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
