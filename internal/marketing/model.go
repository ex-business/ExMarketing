package marketing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
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
type QuotaReservationStatus string

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
	RuleTypeScript    RuleType = "SCRIPT"

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

	QuotaReservationFrozen    QuotaReservationStatus = "FROZEN"
	QuotaReservationConfirmed QuotaReservationStatus = "CONFIRMED"
	QuotaReservationReleased  QuotaReservationStatus = "RELEASED"
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
	PluginCode     string          `json:"pluginCode,omitempty"`
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

type PluginPrecheckResult struct {
	Allowed    bool   `json:"allowed"`
	ReasonCode string `json:"reasonCode,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type PluginStateChangeIntent struct {
	PluginCode                  string         `json:"pluginCode"`
	IntentType                  string         `json:"intentType"`
	TenantID                    string         `json:"tenantId"`
	ActivityID                  string         `json:"activityId"`
	ActivityCode                string         `json:"activityCode"`
	Subject                     EventSubject   `json:"subject"`
	Biz                         EventBiz       `json:"biz"`
	EventID                     string         `json:"eventId"`
	ParticipationID             string         `json:"participationId"`
	ParticipationIdempotencyKey string         `json:"participationIdempotencyKey"`
	IdempotencyKey              string         `json:"idempotencyKey"`
	Payload                     map[string]any `json:"payload,omitempty"`
}

type ActivityPluginSnapshot struct {
	PluginCode         string                    `json:"pluginCode,omitempty"`
	PluginContext      map[string]any            `json:"pluginContext,omitempty"`
	PrecheckResult     PluginPrecheckResult      `json:"precheckResult,omitempty"`
	Explain            map[string]any            `json:"explain,omitempty"`
	StateChangeIntents []PluginStateChangeIntent `json:"stateChangeIntents,omitempty"`
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

type rawConditionRuleConfig struct {
	Logic      string         `json:"logic"`
	Conditions []rawCondition `json:"conditions"`
}

type rawCondition struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

type formulaRuleConfig struct {
	Formula    string       `json:"formula"`
	ResultType string       `json:"resultType"`
	Compare    *ruleCompare `json:"compare,omitempty"`
	Scale      *int         `json:"scale,omitempty"`
}

type ruleCompare struct {
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

type limitRuleConfig struct {
	LimitType     string `json:"limitType"`
	Scope         string `json:"scope"`
	Window        string `json:"window"`
	Max           *int   `json:"max"`
	Field         string `json:"field,omitempty"`
	ResetTimezone string `json:"resetTimezone,omitempty"`
}

type scriptRuleConfig struct {
	ScriptRef string   `json:"scriptRef"`
	Language  string   `json:"language"`
	Inputs    []string `json:"inputs"`
	TimeoutMS *int     `json:"timeoutMs,omitempty"`
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

func (event *ActivityEvent) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("event must be object: %w", err)
	}
	if raw == nil {
		return errors.New("event must be object")
	}

	allowed := map[string]struct{}{
		"eventId":      {},
		"eventType":    {},
		"sourceSystem": {},
		"tenantId":     {},
		"subject":      {},
		"biz":          {},
		"occurredAt":   {},
		"attributes":   {},
	}
	for field := range raw {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%s is unknown; business fields must be placed in attributes", field)
		}
	}
	for _, field := range []string{"eventId", "eventType", "sourceSystem", "tenantId", "subject", "biz", "occurredAt", "attributes"} {
		if _, ok := raw[field]; !ok {
			return fmt.Errorf("%s is required", field)
		}
	}

	eventID, err := requiredString(raw, "eventId")
	if err != nil {
		return err
	}
	eventType, err := requiredString(raw, "eventType")
	if err != nil {
		return err
	}
	sourceSystem, err := requiredString(raw, "sourceSystem")
	if err != nil {
		return err
	}
	tenantID, err := requiredString(raw, "tenantId")
	if err != nil {
		return err
	}
	occurredAtValue, err := requiredString(raw, "occurredAt")
	if err != nil {
		return err
	}
	occurredAt, err := time.Parse(time.RFC3339, occurredAtValue)
	if err != nil {
		return fmt.Errorf("occurredAt must be RFC3339 time with timezone")
	}
	subject, err := decodeEventSubject(raw["subject"])
	if err != nil {
		return err
	}
	biz, err := decodeEventBiz(raw["biz"])
	if err != nil {
		return err
	}
	attributes, err := decodeObject(raw["attributes"], "attributes")
	if err != nil {
		return err
	}

	*event = ActivityEvent{
		EventID:      eventID,
		EventType:    eventType,
		SourceSystem: sourceSystem,
		TenantID:     tenantID,
		Subject:      subject,
		Biz:          biz,
		OccurredAt:   occurredAt,
		Attributes:   attributes,
	}
	return nil
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
	ExternalGrantID string          `json:"externalGrantId,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
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
	ID           string                 `json:"id,omitempty"`
	TenantID     string                 `json:"tenantId,omitempty"`
	ActivityID   string                 `json:"activityId,omitempty"`
	GrantOrderID string                 `json:"grantOrderId,omitempty"`
	CommandID    string                 `json:"commandId,omitempty"`
	Type         QuotaType              `json:"quotaType"`
	Scope        QuotaScope             `json:"scope"`
	ScopeKey     string                 `json:"scopeKey"`
	Amount       string                 `json:"amount"`
	Status       QuotaReservationStatus `json:"reservationStatus,omitempty"`
	CreatedAt    time.Time              `json:"createdAt,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt,omitempty"`
	ConfirmedAt  *time.Time             `json:"confirmedAt,omitempty"`
	ReleasedAt   *time.Time             `json:"releasedAt,omitempty"`
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
	Operator   string     `json:"operator,omitempty"`
	TenantID   string     `json:"tenantId,omitempty"`
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

type AuditQuery struct {
	TenantID   string
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	ActivityID string
	EventID    string
	CommandID  string
	Limit      int
}

type FactProvider struct {
	TenantID        string                 `json:"tenantId"`
	FactCode        string                 `json:"factCode"`
	ProviderType    string                 `json:"providerType"`
	Endpoint        string                 `json:"endpoint"`
	Method          string                 `json:"method"`
	AuthRef         string                 `json:"authRef,omitempty"`
	Mapping         map[string]FactMapping `json:"mapping"`
	CacheTTLSeconds int                    `json:"cacheTtlSeconds"`
	Enabled         bool                   `json:"enabled"`
	TimeoutMS       int                    `json:"timeoutMs"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

type FactMapping struct {
	Source string `json:"source"`
	Type   string `json:"type"`
}

type FactQueryRequest struct {
	TenantID   string         `json:"tenantId"`
	FactCode   string         `json:"factCode"`
	Subject    EventSubject   `json:"subject"`
	Biz        EventBiz       `json:"biz"`
	Attributes map[string]any `json:"attributes,omitempty"`
	TraceID    string         `json:"traceId,omitempty"`
}

type FactQueryResponse struct {
	TenantID       string         `json:"tenantId"`
	FactCode       string         `json:"factCode"`
	Success        bool           `json:"success"`
	Cached         bool           `json:"cached"`
	Facts          map[string]any `json:"facts,omitempty"`
	ErrorCode      string         `json:"errorCode,omitempty"`
	Error          string         `json:"error,omitempty"`
	ProviderStatus string         `json:"providerStatus,omitempty"`
}

type SimulationResult struct {
	Event               ActivityEvent              `json:"event"`
	SimulatedAt         time.Time                  `json:"simulatedAt"`
	MatchedActivities   []SimulationActivityResult `json:"matchedActivities"`
	UnmatchedActivities []SimulationActivityResult `json:"unmatchedActivities"`
	Summary             SimulationSummary          `json:"summary"`
	Participations      []Participation            `json:"participations"`
	GrantOrders         []GrantOrder               `json:"grantOrders"`
	OutboxMessages      []OutboxMessage            `json:"outboxMessages"`
	RuleResults         []ActivityRuleResult       `json:"ruleResults,omitempty"`
}

type SimulationSummary struct {
	CandidateCount        int  `json:"candidateCount"`
	MatchedCount          int  `json:"matchedCount"`
	UnmatchedCount        int  `json:"unmatchedCount"`
	HasRewardableActivity bool `json:"hasRewardableActivity"`
	HasQuotaRisk          bool `json:"hasQuotaRisk"`
}

type SimulationActivityResult struct {
	ActivityID             string                  `json:"activityId"`
	ActivityCode           string                  `json:"activityCode"`
	ActivityName           string                  `json:"activityName"`
	ActivityType           string                  `json:"activityType"`
	Status                 ActivityStatus          `json:"status"`
	Version                int                     `json:"version"`
	Priority               int                     `json:"priority"`
	ExclusiveGroup         string                  `json:"exclusiveGroup,omitempty"`
	Stackable              bool                    `json:"stackable"`
	Decision               RuleDecision            `json:"decision"`
	DecisionReasonCode     string                  `json:"decisionReasonCode,omitempty"`
	DecisionReason         string                  `json:"decisionReason,omitempty"`
	RuleResult             ActivityRuleResult      `json:"ruleResult"`
	Plugin                 *ActivityPluginSnapshot `json:"plugin,omitempty"`
	RewardPreviews         []RewardPreview         `json:"rewardPreviews,omitempty"`
	QuotaImpactPreviews    []QuotaImpactPreview    `json:"quotaImpactPreviews,omitempty"`
	ParticipationPreview   *Participation          `json:"participationPreview,omitempty"`
	GrantOrderPreviews     []GrantOrder            `json:"grantOrderPreviews,omitempty"`
	OutboxCommandPreviews  []OutboxMessage         `json:"outboxCommandPreviews,omitempty"`
	WouldCreateGrantOrders bool                    `json:"wouldCreateGrantOrders"`
}

type RewardPreview struct {
	RewardCode    string         `json:"rewardCode"`
	RewardType    RewardType     `json:"rewardType"`
	CalculateType CalculateType  `json:"calculateType"`
	GrantMode     GrantMode      `json:"grantMode"`
	GrantChannel  GrantChannel   `json:"grantChannel"`
	Amount        string         `json:"amount,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	Skipped       bool           `json:"skipped"`
	ReasonCode    string         `json:"reasonCode"`
	Reason        string         `json:"reason"`
	Detail        map[string]any `json:"detail,omitempty"`
}

type QuotaImpactPreview struct {
	QuotaType       QuotaType  `json:"quotaType"`
	Scope           QuotaScope `json:"scope"`
	ScopeKey        string     `json:"scopeKey"`
	CurrentTotal    string     `json:"currentTotal,omitempty"`
	CurrentUsed     string     `json:"currentUsed,omitempty"`
	CurrentFrozen   string     `json:"currentFrozen,omitempty"`
	AvailableBefore string     `json:"availableBefore,omitempty"`
	Delta           string     `json:"delta"`
	Unlimited       bool       `json:"unlimited"`
	WouldExceed     bool       `json:"wouldExceed"`
	ReasonCode      string     `json:"reasonCode"`
	Reason          string     `json:"reason"`
}

type ProcessEventResult struct {
	Inbox          EventInbox           `json:"inbox"`
	Participations []Participation      `json:"participations"`
	GrantOrders    []GrantOrder         `json:"grantOrders"`
	OutboxMessages []OutboxMessage      `json:"outboxMessages"`
	RuleResults    []ActivityRuleResult `json:"ruleResults,omitempty"`
	Duplicate      bool                 `json:"duplicate"`
}

type RewardCallbackResult struct {
	CommandID           string              `json:"commandId"`
	TenantID            string              `json:"tenantId"`
	GrantOrderID        string              `json:"grantOrderId"`
	ParticipationID     string              `json:"participationId"`
	GrantStatus         GrantStatus         `json:"grantStatus"`
	ParticipationStatus ParticipationStatus `json:"participationStatus,omitempty"`
	Duplicate           bool                `json:"duplicate"`
	Reservations        []QuotaReservation  `json:"quotaReservations,omitempty"`
}

type RewardCallback struct {
	CommandID       string      `json:"commandId"`
	TenantID        string      `json:"tenantId"`
	Status          GrantStatus `json:"status"`
	ExternalGrantID string      `json:"externalGrantId"`
	Message         string      `json:"message"`
	FinishedAt      time.Time   `json:"finishedAt"`
}

func (callback RewardCallback) Validate() error {
	if strings.TrimSpace(callback.TenantID) == "" || strings.TrimSpace(callback.CommandID) == "" {
		return errors.Join(ErrInvalidCallback, errors.New("tenantId and commandId are required"))
	}
	if callback.Status != GrantSuccess && callback.Status != GrantFailed {
		return errors.Join(ErrInvalidCallback, errors.New("status must be SUCCESS or FAILED"))
	}
	if callback.Status == GrantSuccess && strings.TrimSpace(callback.ExternalGrantID) == "" {
		return errors.Join(ErrInvalidCallback, errors.New("externalGrantId is required for SUCCESS"))
	}
	if callback.FinishedAt.IsZero() {
		return errors.Join(ErrInvalidCallback, errors.New("finishedAt is required"))
	}
	return nil
}

type RewardReconciliationItem struct {
	TenantID            string              `json:"tenantId"`
	CommandID           string              `json:"commandId"`
	GrantOrderID        string              `json:"grantOrderId"`
	ExternalGrantID     string              `json:"externalGrantId,omitempty"`
	ActivityID          string              `json:"activityId"`
	ParticipationID     string              `json:"participationId"`
	Subject             EventSubject        `json:"subject"`
	RewardType          RewardType          `json:"rewardType"`
	RewardAmount        string              `json:"rewardAmount"`
	GrantStatus         GrantStatus         `json:"grantStatus"`
	ParticipationStatus ParticipationStatus `json:"participationStatus"`
	CallbackMessage     string              `json:"callbackMessage,omitempty"`
	FinishedAt          *time.Time          `json:"finishedAt,omitempty"`
	ServerUpdatedAt     time.Time           `json:"serverUpdatedAt"`
	Reservations        []QuotaReservation  `json:"quotaReservations"`
	EventInbox          *EventInbox         `json:"eventInbox,omitempty"`
	OutboxMessages      []OutboxMessage     `json:"outboxMessages,omitempty"`
	AuditTimeline       []AuditRecord       `json:"auditTimeline,omitempty"`
}

type RewardReconciliationQuery struct {
	TenantID       string
	CommandID      string
	GrantOrderID   string
	ActivityID     string
	SubjectType    string
	SubjectID      string
	EventID        string
	IdempotencyKey string
	Limit          int
}

type CreateActivityCommand struct {
	Operator       string          `json:"operator,omitempty"`
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
	PluginCode     string          `json:"pluginCode,omitempty"`
	Version        int             `json:"version"`
	Config         json.RawMessage `json:"config,omitempty"`
	ConfigJSON     json.RawMessage `json:"configJson,omitempty"`
	ConfigSnake    json.RawMessage `json:"config_json,omitempty"`
	Rules          []Rule          `json:"rules"`
	Rewards        []Reward        `json:"rewards"`
}

type UpdateActivityCommand struct {
	Operator       string          `json:"operator,omitempty"`
	Name           *string         `json:"activityName,omitempty"`
	Status         *ActivityStatus `json:"status,omitempty"`
	StartTime      *time.Time      `json:"startTime,omitempty"`
	EndTime        *time.Time      `json:"endTime,omitempty"`
	Timezone       *string         `json:"timezone,omitempty"`
	Priority       *int            `json:"priority,omitempty"`
	ExclusiveGroup *string         `json:"exclusiveGroup,omitempty"`
	Stackable      *bool           `json:"stackable,omitempty"`
	Config         json.RawMessage `json:"config,omitempty"`
	ConfigJSON     json.RawMessage `json:"configJson,omitempty"`
	ConfigSnake    json.RawMessage `json:"config_json,omitempty"`
}

var (
	ErrInvalidActivity          = errors.New("invalid activity")
	ErrInvalidEvent             = errors.New("invalid event")
	ErrInvalidFactProvider      = errors.New("invalid fact provider")
	ErrDuplicateEvent           = errors.New("duplicate event")
	ErrIdempotencyConflict      = errors.New("idempotency key payload conflict")
	ErrNotFound                 = errors.New("not found")
	ErrQuotaExceeded            = errors.New("quota exceeded")
	ErrInvalidOperation         = errors.New("invalid operation")
	ErrInvalidCallback          = errors.New("invalid callback request")
	ErrCallbackConflict         = errors.New("callback status conflict")
	ErrGrantOrderClosed         = errors.New("grant order closed")
	ErrQuotaReservationMissing  = errors.New("quota reservation missing")
	ErrQuotaReservationConflict = errors.New("quota reservation conflict")
	ErrQuotaAggregateConflict   = errors.New("quota aggregate conflict")
	ErrActivityConflict         = errors.New("activity conflict")
)

const MaxFactCacheTTLSeconds = 3600

var factCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

func (provider FactProvider) Validate() error {
	if strings.TrimSpace(provider.TenantID) == "" {
		return errors.Join(ErrInvalidFactProvider, errors.New("tenantId is required"))
	}
	if !factCodePattern.MatchString(strings.TrimSpace(provider.FactCode)) {
		return errors.Join(ErrInvalidFactProvider, errors.New("factCode is invalid"))
	}
	if strings.ToUpper(strings.TrimSpace(provider.ProviderType)) != "HTTP" {
		return errors.Join(ErrInvalidFactProvider, errors.New("providerType must be HTTP"))
	}
	if err := validateHTTPURL("endpoint", provider.Endpoint, true); err != nil {
		return errors.Join(ErrInvalidFactProvider, err)
	}
	switch strings.ToUpper(strings.TrimSpace(provider.Method)) {
	case "GET", "POST":
	default:
		return errors.Join(ErrInvalidFactProvider, errors.New("method must be GET or POST"))
	}
	if provider.CacheTTLSeconds < 0 || provider.CacheTTLSeconds > MaxFactCacheTTLSeconds {
		return errors.Join(ErrInvalidFactProvider, errors.New("FACT_CACHE_TTL_INVALID: cacheTtlSeconds must be between 0 and 3600"))
	}
	if provider.TimeoutMS < 0 {
		return errors.Join(ErrInvalidFactProvider, errors.New("timeoutMs must be non-negative"))
	}
	if len(provider.Mapping) == 0 {
		return errors.Join(ErrInvalidFactProvider, errors.New("mapping is required"))
	}
	for outputPath, mapping := range provider.Mapping {
		if strings.TrimSpace(outputPath) == "" || strings.HasPrefix(outputPath, ".") || strings.Contains(outputPath, "..") {
			return errors.Join(ErrInvalidFactProvider, fmt.Errorf("mapping path %q is invalid", outputPath))
		}
		if !strings.HasPrefix(strings.TrimSpace(mapping.Source), "$.") {
			return errors.Join(ErrInvalidFactProvider, fmt.Errorf("mapping %s source is invalid", outputPath))
		}
		switch strings.ToLower(strings.TrimSpace(mapping.Type)) {
		case "string", "number", "boolean", "array", "object":
		default:
			return errors.Join(ErrInvalidFactProvider, fmt.Errorf("mapping %s type is invalid", outputPath))
		}
	}
	return nil
}

func ExtractFactReferences(rules []Rule) map[string]map[string]struct{} {
	refs := map[string]map[string]struct{}{}
	for _, rule := range rules {
		if rule.Type != RuleTypeCondition || emptyOrNullJSON(rule.Config) {
			continue
		}
		var cfg ConditionRuleConfig
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			continue
		}
		for _, condition := range cfg.Conditions {
			factCode, path, ok := splitFactField(condition.Field)
			if !ok {
				continue
			}
			if refs[factCode] == nil {
				refs[factCode] = map[string]struct{}{}
			}
			refs[factCode][path] = struct{}{}
		}
	}
	return refs
}

func splitFactField(field string) (string, string, bool) {
	if !strings.HasPrefix(field, "facts.") {
		return "", "", false
	}
	rest := strings.TrimPrefix(field, "facts.")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

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
	if err := validateActivityConfig(cmd.ConfigPayload(), cmd.TenantID); err != nil {
		return err
	}
	seenRules := map[string]struct{}{}
	for i, rule := range cmd.Rules {
		if err := validateRule(i, rule, cmd.PluginCode); err != nil {
			return err
		}
		ruleCode := strings.TrimSpace(rule.Code)
		if _, ok := seenRules[ruleCode]; ok {
			return invalidActivityf("rules[%d].ruleCode is duplicated", i)
		}
		seenRules[ruleCode] = struct{}{}
	}
	if len(cmd.Rewards) == 0 {
		return errors.Join(ErrInvalidActivity, errors.New("at least one reward is required"))
	}
	enabledRewards := 0
	for i, reward := range cmd.Rewards {
		if err := validateReward(i, reward, cmd.PluginCode); err != nil {
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

func (cmd CreateActivityCommand) ConfigPayload() json.RawMessage {
	return firstRawMessage(cmd.Config, cmd.ConfigJSON, cmd.ConfigSnake)
}

func (cmd UpdateActivityCommand) ApplyTo(activity Activity, now time.Time) (Activity, error) {
	if cmd.Name != nil {
		name := strings.TrimSpace(*cmd.Name)
		if name == "" {
			return Activity{}, invalidActivityf("activityName is required")
		}
		activity.Name = name
	}
	if cmd.Status != nil {
		if !validActivityStatus(*cmd.Status) {
			return Activity{}, invalidActivityf("status is invalid")
		}
		activity.Status = *cmd.Status
	}
	if cmd.StartTime != nil {
		activity.StartTime = *cmd.StartTime
	}
	if cmd.EndTime != nil {
		activity.EndTime = *cmd.EndTime
	}
	if activity.StartTime.IsZero() || activity.EndTime.IsZero() || !activity.EndTime.After(activity.StartTime) {
		return Activity{}, invalidActivityf("valid startTime and endTime are required")
	}
	if cmd.Timezone != nil {
		timezone := strings.TrimSpace(*cmd.Timezone)
		if timezone == "" {
			return Activity{}, invalidActivityf("timezone is required")
		}
		if _, err := time.LoadLocation(timezone); err != nil {
			return Activity{}, invalidActivityf("timezone is invalid")
		}
		activity.Timezone = timezone
	}
	if cmd.Priority != nil {
		activity.Priority = *cmd.Priority
	}
	if cmd.ExclusiveGroup != nil {
		activity.ExclusiveGroup = strings.TrimSpace(*cmd.ExclusiveGroup)
	}
	if cmd.Stackable != nil {
		activity.Stackable = *cmd.Stackable
	}
	if config, ok := cmd.ConfigPayloadIfSet(); ok {
		if err := validateActivityConfig(config, activity.TenantID); err != nil {
			return Activity{}, err
		}
		activity.Config = config
	}
	activity.Version++
	activity.UpdatedAt = now
	return activity, nil
}

func (cmd UpdateActivityCommand) ConfigPayloadIfSet() (json.RawMessage, bool) {
	for _, raw := range []json.RawMessage{cmd.Config, cmd.ConfigJSON, cmd.ConfigSnake} {
		if raw != nil {
			return raw, true
		}
	}
	return nil, false
}

func firstRawMessage(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
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
	Enabled       *bool                `json:"enabled,omitempty"`
	EntryCodes    []string             `json:"entryCodes,omitempty"`
	Channels      []string             `json:"channels,omitempty"`
	SourceSystems []string             `json:"sourceSystems,omitempty"`
	TenantIDs     []string             `json:"tenantIds,omitempty"`
	Audience      *entryAudienceConfig `json:"audience,omitempty"`
	EntranceCodes []string             `json:"entranceCodes,omitempty"`
	LandingURL    string               `json:"landingUrl,omitempty"`
}

type displayConfig struct {
	Visible     *bool    `json:"visible,omitempty"`
	Title       string   `json:"title,omitempty"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Description string   `json:"description,omitempty"`
	BannerURL   string   `json:"bannerUrl,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TermsURL    string   `json:"termsUrl,omitempty"`
	VisibleFrom string   `json:"visibleFrom,omitempty"`
	VisibleTo   string   `json:"visibleTo,omitempty"`
	Sort        *int     `json:"sort,omitempty"`
}

type audienceConfig struct {
	SegmentCodes []string `json:"segmentCodes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Countries    []string `json:"countries,omitempty"`
}

type entryAudienceConfig struct {
	SubjectTypes []string `json:"subjectTypes,omitempty"`
	Segments     []string `json:"segments,omitempty"`
	RiskLevels   []string `json:"riskLevels,omitempty"`
}

func validateActivityConfig(raw json.RawMessage, tenantID string) error {
	if emptyOrNullJSON(raw) {
		return nil
	}
	var cfg activityConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("config is invalid: %v", err)
	}
	if cfg.Entry != nil {
		if err := validateStringSlice("config.entry.entryCodes", cfg.Entry.EntryCodes); err != nil {
			return err
		}
		if err := validateStringSlice("config.entry.channels", cfg.Entry.Channels); err != nil {
			return err
		}
		if err := validateStringSlice("config.entry.sourceSystems", cfg.Entry.SourceSystems); err != nil {
			return err
		}
		if err := validateStringSlice("config.entry.tenantIds", cfg.Entry.TenantIDs); err != nil {
			return err
		}
		for i, entryTenantID := range cfg.Entry.TenantIDs {
			if strings.TrimSpace(entryTenantID) != strings.TrimSpace(tenantID) {
				return invalidActivityf("config.entry.tenantIds[%d] must match activity tenantId", i)
			}
		}
		if cfg.Entry.Audience != nil {
			if err := validateStringSlice("config.entry.audience.subjectTypes", cfg.Entry.Audience.SubjectTypes); err != nil {
				return err
			}
			if err := validateStringSlice("config.entry.audience.segments", cfg.Entry.Audience.Segments); err != nil {
				return err
			}
			if err := validateStringSlice("config.entry.audience.riskLevels", cfg.Entry.Audience.RiskLevels); err != nil {
				return err
			}
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
		if err := validateStringSlice("config.display.tags", cfg.Display.Tags); err != nil {
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

func validateRule(index int, rule Rule, pluginCode string) error {
	if strings.TrimSpace(rule.Code) == "" {
		return invalidActivityf("rules[%d].ruleCode is required", index)
	}
	if rule.Phase != RulePhaseMatch && rule.Phase != RulePhaseQualify && rule.Phase != RulePhaseReward && rule.Phase != RulePhaseRisk && rule.Phase != RulePhaseLimit {
		return invalidActivityf("rules[%d].rulePhase is invalid", index)
	}
	if rule.Sort < 0 {
		return invalidActivityf("rules[%d].sort must be non-negative", index)
	}
	if rule.Version < 0 || rule.Version > 1 {
		return invalidActivityf("rules[%d].version must be 0 or 1", index)
	}
	switch rule.Type {
	case RuleTypeCondition:
		return validateConditionRule(index, rule.Config, pluginCode)
	case RuleTypeFormula:
		if err := validateFormulaRule(index, rule.Config, pluginCode); err != nil {
			return err
		}
		return invalidActivityf("rules[%d].ruleType FORMULA evaluator is not registered", index)
	case RuleTypeLimit:
		if err := validateLimitRule(index, rule.Config, pluginCode); err != nil {
			return err
		}
		return invalidActivityf("rules[%d].ruleType LIMIT evaluator is not registered", index)
	case RuleTypeScript:
		if err := validateScriptRule(index, rule.Config, pluginCode); err != nil {
			return err
		}
		return invalidActivityf("rules[%d].ruleType SCRIPT evaluator is not registered", index)
	default:
		return invalidActivityf("rules[%d].ruleType is unsupported", index)
	}
}

func validateConditionRule(index int, raw json.RawMessage, pluginCode string) error {
	var cfg rawConditionRuleConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("rules[%d].ruleConfig is invalid: %v", index, err)
	}
	logic := strings.ToUpper(strings.TrimSpace(cfg.Logic))
	if logic != "AND" && logic != "OR" {
		return invalidActivityf("rules[%d].ruleConfig.logic is invalid", index)
	}
	if len(cfg.Conditions) == 0 {
		return invalidActivityf("rules[%d].ruleConfig.conditions is required", index)
	}
	for i, condition := range cfg.Conditions {
		if strings.TrimSpace(condition.Field) == "" {
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].field is required", index, i)
		}
		if !supportedConfiguredField(condition.Field, pluginCode) {
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].field is unsupported", index, i)
		}
		op := strings.ToUpper(strings.TrimSpace(condition.Op))
		switch op {
		case "=", "==", "!=", "<>":
			if !validRuleValue(condition.Value, false) {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value is invalid", index, i)
			}
		case ">", ">=", "<", "<=":
			if !validRuleValue(condition.Value, false) {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value is invalid", index, i)
			}
			value, _ := rawJSONValue(condition.Value)
			if _, err := decimal.Parse(jsonScalarString(value)); err != nil {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value must be decimal", index, i)
			}
		case "IN", "NOT_IN":
			values, ok := rawJSONArray(condition.Value)
			if !ok {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value must be array", index, i)
			}
			if len(values) == 0 {
				return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value must be non-empty array", index, i)
			}
			for valueIndex, value := range values {
				if !validRuleValue(value, false) {
					return invalidActivityf("rules[%d].ruleConfig.conditions[%d].value[%d] is invalid", index, i, valueIndex)
				}
			}
		default:
			return invalidActivityf("rules[%d].ruleConfig.conditions[%d].op is invalid", index, i)
		}
	}
	return nil
}

func validateFormulaRule(index int, raw json.RawMessage, pluginCode string) error {
	var cfg formulaRuleConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("rules[%d].ruleConfig is invalid: %v", index, err)
	}
	formula := strings.TrimSpace(cfg.Formula)
	if formula == "" {
		return invalidActivityf("rules[%d].ruleConfig.formula is required", index)
	}
	if containsUnsafeExpressionToken(formula) {
		return invalidActivityf("rules[%d].ruleConfig.formula is unsafe", index)
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.ResultType)) {
	case "DECIMAL", "INTEGER", "BOOLEAN", "STRING":
	default:
		return invalidActivityf("rules[%d].ruleConfig.resultType is invalid", index)
	}
	if cfg.Scale != nil {
		if strings.ToUpper(strings.TrimSpace(cfg.ResultType)) != "DECIMAL" {
			return invalidActivityf("rules[%d].ruleConfig.scale is only allowed for DECIMAL", index)
		}
		if *cfg.Scale < 0 {
			return invalidActivityf("rules[%d].ruleConfig.scale must be non-negative", index)
		}
	}
	if cfg.Compare != nil {
		if !validCompareOp(cfg.Compare.Op) {
			return invalidActivityf("rules[%d].ruleConfig.compare.op is invalid", index)
		}
		if !validRuleValue(cfg.Compare.Value, false) {
			return invalidActivityf("rules[%d].ruleConfig.compare.value is invalid", index)
		}
	}
	_ = pluginCode
	return nil
}

func validateLimitRule(index int, raw json.RawMessage, pluginCode string) error {
	var cfg limitRuleConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("rules[%d].ruleConfig is invalid: %v", index, err)
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.LimitType)) {
	case "USER_COUNT", "DAILY_COUNT", "PERIOD_COUNT", "EXCLUSIVE_GROUP":
	default:
		return invalidActivityf("rules[%d].ruleConfig.limitType is invalid", index)
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.Scope)) {
	case "USER", "ACTIVITY", "TENANT", "BIZ":
	default:
		return invalidActivityf("rules[%d].ruleConfig.scope is invalid", index)
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.Window)) {
	case "ACTIVITY", "DAY", "PERIOD":
	default:
		return invalidActivityf("rules[%d].ruleConfig.window is invalid", index)
	}
	if cfg.Max == nil {
		return invalidActivityf("rules[%d].ruleConfig.max is required", index)
	}
	if *cfg.Max < 0 {
		return invalidActivityf("rules[%d].ruleConfig.max must be non-negative", index)
	}
	if strings.TrimSpace(cfg.Field) != "" && !supportedConfiguredField(cfg.Field, pluginCode) {
		return invalidActivityf("rules[%d].ruleConfig.field is unsupported", index)
	}
	if strings.TrimSpace(cfg.ResetTimezone) != "" {
		if _, err := time.LoadLocation(cfg.ResetTimezone); err != nil {
			return invalidActivityf("rules[%d].ruleConfig.resetTimezone is invalid", index)
		}
	}
	return nil
}

func validateScriptRule(index int, raw json.RawMessage, pluginCode string) error {
	var cfg scriptRuleConfig
	if err := decodeStrict(raw, &cfg); err != nil {
		return invalidActivityf("rules[%d].ruleConfig is invalid: %v", index, err)
	}
	if strings.TrimSpace(cfg.ScriptRef) == "" {
		return invalidActivityf("rules[%d].ruleConfig.scriptRef is required", index)
	}
	if strings.ToUpper(strings.TrimSpace(cfg.Language)) != "CEL" {
		return invalidActivityf("rules[%d].ruleConfig.language is invalid", index)
	}
	if len(cfg.Inputs) == 0 {
		return invalidActivityf("rules[%d].ruleConfig.inputs is required", index)
	}
	for inputIndex, input := range cfg.Inputs {
		if !supportedConfiguredField(input, pluginCode) {
			return invalidActivityf("rules[%d].ruleConfig.inputs[%d] is unsupported", index, inputIndex)
		}
	}
	if cfg.TimeoutMS != nil && (*cfg.TimeoutMS <= 0 || *cfg.TimeoutMS > 5000) {
		return invalidActivityf("rules[%d].ruleConfig.timeoutMs is invalid", index)
	}
	return nil
}

func validateReward(index int, reward Reward, pluginCode string) error {
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
	return validateRewardConfigByCalculateType(index, reward, cfg, pluginCode)
}

func validRewardCalculateType(calculateType CalculateType) bool {
	switch calculateType {
	case CalculateFixed, CalculateRatio, CalculateLadder, CalculateRandom, CalculateProbability, CalculatePoolSplit:
		return true
	default:
		return false
	}
}

func validateRewardConfigByCalculateType(index int, reward Reward, cfg RewardConfig, pluginCode string) error {
	switch reward.CalculateType {
	case CalculateFixed:
		return validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.amount", index), cfg.Amount)
	case CalculateRatio:
		return validateRatioRewardConfig(index, cfg, pluginCode)
	case CalculateLadder:
		return validateLadderRewardConfig(index, cfg, pluginCode)
	case CalculateRandom:
		return validateRandomRewardConfig(index, cfg)
	case CalculateProbability:
		return validateProbabilityRewardConfig(index, reward, cfg)
	case CalculatePoolSplit:
		return validatePoolSplitRewardConfig(index, cfg, pluginCode)
	default:
		return invalidActivityf("rewards[%d].calculateType is unsupported", index)
	}
}

func validateRatioRewardConfig(index int, cfg RewardConfig, pluginCode string) error {
	if strings.TrimSpace(cfg.BaseField) == "" {
		return invalidActivityf("rewards[%d].rewardConfig.baseField is required", index)
	}
	if !supportedConfiguredField(cfg.BaseField, pluginCode) {
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

func validateLadderRewardConfig(index int, cfg RewardConfig, pluginCode string) error {
	if strings.TrimSpace(cfg.Field) == "" {
		return invalidActivityf("rewards[%d].rewardConfig.field is required", index)
	}
	if !supportedConfiguredField(cfg.Field, pluginCode) {
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

func validatePoolSplitRewardConfig(index int, cfg RewardConfig, pluginCode string) error {
	if err := validateNonNegativeDecimal(fmt.Sprintf("rewards[%d].rewardConfig.poolAmount", index), cfg.PoolAmount); err != nil {
		return err
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.SplitBy)) {
	case "COUNT":
		if strings.TrimSpace(cfg.ParticipantCountField) == "" {
			return invalidActivityf("rewards[%d].rewardConfig.participantCountField is required", index)
		}
		if !supportedConfiguredField(cfg.ParticipantCountField, pluginCode) {
			return invalidActivityf("rewards[%d].rewardConfig.participantCountField is unsupported", index)
		}
	case "WEIGHT":
		if strings.TrimSpace(cfg.WeightField) == "" || strings.TrimSpace(cfg.TotalWeightField) == "" {
			return invalidActivityf("rewards[%d].rewardConfig.weightField and totalWeightField are required", index)
		}
		if !supportedConfiguredField(cfg.WeightField, pluginCode) || !supportedConfiguredField(cfg.TotalWeightField, pluginCode) {
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
	seen := map[string]struct{}{}
	for i, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return invalidActivityf("%s[%d] is required", name, i)
		}
		if _, ok := seen[value]; ok {
			return invalidActivityf("%s[%d] is duplicated", name, i)
		}
		seen[value] = struct{}{}
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

func supportedConfiguredField(field string, pluginCode string) bool {
	field = strings.TrimSpace(field)
	if strings.HasPrefix(field, "attributes.") && len(field) > len("attributes.") {
		return true
	}
	if supportedStateField(field, pluginCode) {
		return true
	}
	if _, _, ok := splitFactField(field); ok {
		return true
	}
	switch field {
	case "eventId", "eventType", "sourceSystem", "tenantId", "occurredAt", "subject.type", "subject.id", "biz.bizType", "biz.bizId":
		return true
	default:
		return false
	}
}

func supportedStateField(field string, pluginCode string) bool {
	for _, prefix := range []string{"state.activity.", "state.subject.", "state.quota.", "state.exclusive."} {
		if strings.HasPrefix(field, prefix) && len(field) > len(prefix) {
			return true
		}
	}
	pluginCode = strings.TrimSpace(pluginCode)
	if pluginCode == "" {
		return false
	}
	prefix := "state.plugin." + pluginCode + "."
	return strings.HasPrefix(field, prefix) && len(field) > len(prefix)
}

func validCompareOp(op string) bool {
	switch strings.ToUpper(strings.TrimSpace(op)) {
	case "=", "==", "!=", "<>", ">", ">=", "<", "<=":
		return true
	default:
		return false
	}
}

func validRuleValue(raw json.RawMessage, allowArray bool) bool {
	value, ok := rawJSONValue(raw)
	if !ok {
		return false
	}
	switch value.(type) {
	case string, bool, json.Number:
		return true
	case []any:
		return allowArray
	default:
		return false
	}
}

func rawJSONValue(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func rawJSONArray(raw json.RawMessage) ([]json.RawMessage, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, false
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, false
	}
	return values, true
}

func containsUnsafeExpressionToken(expression string) bool {
	upper := strings.ToUpper(expression)
	for _, token := range []string{"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "HTTP", "FILE", "ENV", "SECRET", "RANDOM", "NOW(", "TIME("} {
		if strings.Contains(upper, token) {
			return true
		}
	}
	return false
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
	if strings.TrimSpace(event.EventID) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("eventId must be non-empty string"))
	}
	if strings.TrimSpace(event.EventType) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("eventType must be non-empty string"))
	}
	if strings.TrimSpace(event.SourceSystem) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("sourceSystem must be non-empty string"))
	}
	if strings.TrimSpace(event.TenantID) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("tenantId must be non-empty string"))
	}
	if strings.TrimSpace(event.Subject.Type) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("subject.type must be non-empty string"))
	}
	if strings.TrimSpace(event.Subject.ID) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("subject.id must be non-empty string"))
	}
	if strings.TrimSpace(event.Biz.BizType) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("biz.bizType must be non-empty string"))
	}
	if strings.TrimSpace(event.Biz.BizID) == "" {
		return errors.Join(ErrInvalidEvent, errors.New("biz.bizId must be non-empty string"))
	}
	if event.OccurredAt.IsZero() {
		return errors.Join(ErrInvalidEvent, errors.New("occurredAt is required"))
	}
	if event.Attributes == nil {
		return errors.Join(ErrInvalidEvent, errors.New("attributes must be object"))
	}
	return nil
}

func requiredString(raw map[string]json.RawMessage, field string) (string, error) {
	if isJSONNull(raw[field]) {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	var value string
	if err := json.Unmarshal(raw[field], &value); err != nil {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	return value, nil
}

func decodeEventSubject(data json.RawMessage) (EventSubject, error) {
	raw, err := decodeRawObject(data, "subject")
	if err != nil {
		return EventSubject{}, err
	}
	for field := range raw {
		if field != "type" && field != "id" {
			return EventSubject{}, fmt.Errorf("subject.%s is unknown; business fields must be placed in attributes", field)
		}
	}
	for _, field := range []string{"type", "id"} {
		if _, ok := raw[field]; !ok {
			return EventSubject{}, fmt.Errorf("subject.%s is required", field)
		}
	}
	subjectType, err := requiredRawString(raw["type"], "subject.type")
	if err != nil {
		return EventSubject{}, err
	}
	subjectID, err := requiredRawString(raw["id"], "subject.id")
	if err != nil {
		return EventSubject{}, err
	}
	return EventSubject{Type: subjectType, ID: subjectID}, nil
}

func decodeEventBiz(data json.RawMessage) (EventBiz, error) {
	raw, err := decodeRawObject(data, "biz")
	if err != nil {
		return EventBiz{}, err
	}
	for field := range raw {
		if field != "bizType" && field != "bizId" {
			return EventBiz{}, fmt.Errorf("biz.%s is unknown; business fields must be placed in attributes", field)
		}
	}
	for _, field := range []string{"bizType", "bizId"} {
		if _, ok := raw[field]; !ok {
			return EventBiz{}, fmt.Errorf("biz.%s is required", field)
		}
	}
	bizType, err := requiredRawString(raw["bizType"], "biz.bizType")
	if err != nil {
		return EventBiz{}, err
	}
	bizID, err := requiredRawString(raw["bizId"], "biz.bizId")
	if err != nil {
		return EventBiz{}, err
	}
	return EventBiz{BizType: bizType, BizID: bizID}, nil
}

func decodeRawObject(data json.RawMessage, field string) (map[string]json.RawMessage, error) {
	if isJSONNull(data) {
		return nil, fmt.Errorf("%s must be object", field)
	}
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil || raw == nil {
		return nil, fmt.Errorf("%s must be object", field)
	}
	return raw, nil
}

func requiredRawString(data json.RawMessage, field string) (string, error) {
	if isJSONNull(data) {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be non-empty string", field)
	}
	return value, nil
}

func decodeObject(data json.RawMessage, field string) (map[string]any, error) {
	if isJSONNull(data) {
		return nil, fmt.Errorf("%s must be object", field)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, fmt.Errorf("%s must be object", field)
	}
	return value, nil
}

func isJSONNull(data json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
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
