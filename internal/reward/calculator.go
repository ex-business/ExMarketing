package reward

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/decimal"
	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	ReasonOK                       = "OK"
	ReasonSkippedDisabledReward    = "SKIPPED_DISABLED_REWARD"
	ReasonInvalidRewardConfig      = "INVALID_REWARD_CONFIG"
	ReasonUnsupportedCalculateType = "UNSUPPORTED_CALCULATE_TYPE"
	ReasonFieldMissing             = "FIELD_MISSING"
	ReasonFieldTypeMismatch        = "FIELD_TYPE_MISMATCH"
	ReasonDecimalParseError        = "DECIMAL_PARSE_ERROR"
	ReasonNegativeAmount           = "NEGATIVE_AMOUNT"
	ReasonNegativeRate             = "NEGATIVE_RATE"
	ReasonRatioBaseMissing         = "RATIO_BASE_MISSING"
	ReasonRatioMaxAmountInvalid    = "RATIO_MAX_AMOUNT_INVALID"
	ReasonLadderNotMatched         = "LADDER_NOT_MATCHED"
	ReasonLadderOverlapped         = "LADDER_OVERLAPPED"
	ReasonRandomRangeInvalid       = "RANDOM_RANGE_INVALID"
	ReasonRandomStepInvalid        = "RANDOM_STEP_INVALID"
	ReasonProbabilityItemsEmpty    = "PROBABILITY_ITEMS_EMPTY"
	ReasonProbabilityTotalInvalid  = "PROBABILITY_TOTAL_INVALID"
	ReasonProbabilityItemInvalid   = "PROBABILITY_ITEM_INVALID"
	ReasonPoolAmountInvalid        = "POOL_AMOUNT_INVALID"
	ReasonPoolParticipantInvalid   = "POOL_PARTICIPANT_INVALID"
	ReasonPoolWeightInvalid        = "POOL_WEIGHT_INVALID"
	ReasonZeroRewardSkipped        = "ZERO_REWARD_SKIPPED"
)

type Context struct {
	Event          marketing.ActivityEvent
	TenantID       string
	ActivityID     string
	IdempotencyKey string
	State          map[string]any
	Facts          map[string]any
}

type Result struct {
	Type       marketing.RewardType `json:"type"`
	Amount     string               `json:"amount"`
	Currency   string               `json:"currency"`
	Payload    map[string]any       `json:"payload"`
	Skipped    bool                 `json:"skipped,omitempty"`
	ReasonCode string               `json:"reasonCode"`
	Reason     string               `json:"reason"`
}

type Error struct {
	ReasonCode string
	Reason     string
	Field      string
}

func (e Error) Error() string {
	if e.Field != "" {
		return e.ReasonCode + ": " + e.Field + ": " + e.Reason
	}
	return e.ReasonCode + ": " + e.Reason
}

type Calculator interface {
	Calculate(ctx Context, reward marketing.Reward) (Result, error)
}

type ConfigCalculator struct{}

type amountResult struct {
	amount     decimal.Decimal
	rewardType marketing.RewardType
	currency   string
	payload    map[string]any
	payable    bool
}

func (c ConfigCalculator) Calculate(ctx Context, configuredReward marketing.Reward) (Result, error) {
	if !configuredReward.Enabled {
		return skippedResult(configuredReward, ReasonSkippedDisabledReward, "reward is disabled", nil), nil
	}

	config, err := decodeConfig(configuredReward.Config)
	if err != nil {
		return Result{}, rewardError(ReasonInvalidRewardConfig, "rewardConfig must be non-empty object", "rewardConfig")
	}
	if configuredReward.CalculateType != marketing.CalculateProbability {
		if err := validateCurrency(configuredReward.Type, config.Currency); err != nil {
			return Result{}, err
		}
	}

	calculated, err := calculateAmount(ctx, configuredReward, config)
	if err != nil {
		return Result{}, err
	}
	rewardType := calculated.rewardType
	if rewardType == "" {
		rewardType = configuredReward.Type
	}
	currency := firstNonEmpty(calculated.currency, config.Currency)
	if calculated.payload == nil {
		calculated.payload = map[string]any{}
	}
	calculated.payload["source"] = string(configuredReward.CalculateType)
	calculated.payload["currency"] = currency
	calculated.payload["reasonCode"] = ReasonOK
	if config.ExpireAt != "" {
		calculated.payload["expireAt"] = config.ExpireAt
	}
	if config.RewardPayload != nil {
		calculated.payload["extra"] = config.RewardPayload
	}
	amount := calculated.amount.String()
	calculated.payload["type"] = rewardType
	calculated.payload["amount"] = amount
	calculated.payload["flowMultiple"] = firstNonEmpty(config.FlowMultiple, configuredReward.FlowMultiple)

	result := Result{
		Type:       rewardType,
		Amount:     amount,
		Currency:   currency,
		Payload:    calculated.payload,
		ReasonCode: ReasonOK,
		Reason:     "reward calculated",
	}
	if calculated.amount.IsZero() && !calculated.payable {
		result.Skipped = true
		result.ReasonCode = ReasonZeroRewardSkipped
		result.Reason = "reward amount is zero"
		result.Payload["reasonCode"] = ReasonZeroRewardSkipped
	}
	return result, nil
}

func decodeConfig(raw json.RawMessage) (marketing.RewardConfig, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || len(object) == 0 {
		return marketing.RewardConfig{}, errors.New("invalid config")
	}
	var config marketing.RewardConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return marketing.RewardConfig{}, err
	}
	return config, nil
}

func validateCurrency(rewardType marketing.RewardType, currency string) error {
	if (rewardType == marketing.RewardBonus || rewardType == marketing.RewardCash) && strings.TrimSpace(currency) == "" {
		return rewardError(ReasonInvalidRewardConfig, "currency is required for amount reward", "rewardConfig.currency")
	}
	return nil
}

func calculateAmount(ctx Context, configuredReward marketing.Reward, config marketing.RewardConfig) (amountResult, error) {
	switch configuredReward.CalculateType {
	case marketing.CalculateFixed:
		return calculateFixed(configuredReward.Type, config)
	case marketing.CalculateRatio:
		return calculateRatio(ctx, config)
	case marketing.CalculateLadder:
		return calculateLadder(ctx, config)
	case marketing.CalculateRandom:
		return calculateRandom(ctx, configuredReward, config)
	case marketing.CalculateProbability:
		return calculateProbability(ctx, configuredReward, config)
	case marketing.CalculatePoolSplit:
		return calculatePoolSplit(ctx, config)
	default:
		return amountResult{}, rewardError(ReasonUnsupportedCalculateType, "calculateType is unsupported", "calculateType")
	}
}

func calculateFixed(rewardType marketing.RewardType, config marketing.RewardConfig) (amountResult, error) {
	if result, ok, err := calculateFixedNonAmount(rewardType, config); ok || err != nil {
		return result, err
	}
	amount, err := parseConfigDecimal(config.Amount, "rewardConfig.amount", ReasonNegativeAmount)
	if err != nil {
		return amountResult{}, err
	}
	return amountResult{amount: amount, payload: map[string]any{}, payable: amount.IsPositive()}, nil
}

func calculateFixedNonAmount(rewardType marketing.RewardType, config marketing.RewardConfig) (amountResult, bool, error) {
	if rewardType == marketing.RewardPoint {
		points, err := parsePositiveInt(config.Points, "rewardConfig.points")
		if err != nil {
			return amountResult{}, true, err
		}
		return amountResult{amount: decimal.Zero(), payload: map[string]any{
			"points":    points,
			"unit":      firstNonEmpty(config.Unit, config.PointType),
			"pointType": firstNonEmpty(config.PointType, config.Unit),
		}, payable: true}, true, nil
	}
	if rewardType == marketing.RewardCoupon || rewardType == marketing.RewardGift {
		quantity, err := parsePositiveInt(config.Quantity, "rewardConfig.quantity")
		if err != nil {
			return amountResult{}, true, err
		}
		payload := map[string]any{"quantity": quantity}
		copyIfNotEmpty(payload, "couponTemplateId", config.CouponTemplateID)
		copyIfNotEmpty(payload, "couponType", config.CouponType)
		copyIfNotEmpty(payload, "faceValue", config.FaceValue)
		copyIfNotEmpty(payload, "discountRate", config.DiscountRate)
		copyIfNotEmpty(payload, "giftSkuId", config.GiftSkuID)
		copyIfNotEmpty(payload, "giftCode", config.GiftCode)
		copyIfNotEmpty(payload, "deliveryMode", config.DeliveryMode)
		copyIfNotEmpty(payload, "valuationAmount", config.ValuationAmount)
		return amountResult{amount: decimal.Zero(), payload: payload, payable: true}, true, nil
	}
	if rewardType == marketing.RewardDrawChance {
		chanceCount, err := parsePositiveInt(config.ChanceCount, "rewardConfig.chanceCount")
		if err != nil {
			return amountResult{}, true, err
		}
		payload := map[string]any{"chanceCount": chanceCount}
		copyIfNotEmpty(payload, "drawPoolCode", config.DrawPoolCode)
		copyIfNotEmpty(payload, "lotteryCode", config.LotteryCode)
		return amountResult{amount: decimal.Zero(), payload: payload, payable: true}, true, nil
	}
	if rewardType == marketing.RewardMembership {
		payload := map[string]any{}
		if strings.EqualFold(strings.TrimSpace(config.DurationType), "PERMANENT") {
			payload["durationType"] = "PERMANENT"
		} else {
			durationDays, err := parsePositiveInt(config.DurationDays, "rewardConfig.durationDays")
			if err != nil {
				return amountResult{}, true, err
			}
			payload["durationDays"] = durationDays
		}
		copyIfNotEmpty(payload, "membershipTier", config.MembershipTier)
		copyIfNotEmpty(payload, "membershipPlanId", config.MembershipPlanID)
		copyIfNotEmpty(payload, "benefitPackageId", config.BenefitPackageID)
		copyIfNotEmpty(payload, "valuationAmount", config.ValuationAmount)
		return amountResult{amount: decimal.Zero(), payload: payload, payable: true}, true, nil
	}
	return amountResult{}, false, nil
}

func calculateRatio(ctx Context, config marketing.RewardConfig) (amountResult, error) {
	if strings.TrimSpace(config.BaseField) == "" {
		return amountResult{}, rewardError(ReasonRatioBaseMissing, "baseField is required", "rewardConfig.baseField")
	}
	base, err := decimalFromField(ctx, config.BaseField, ReasonRatioBaseMissing)
	if err != nil {
		return amountResult{}, err
	}
	if base.IsNegative() {
		return amountResult{}, rewardError(ReasonNegativeAmount, "base amount must not be negative", config.BaseField)
	}
	rate, err := parseConfigDecimal(config.Rate, "rewardConfig.rate", ReasonNegativeRate)
	if err != nil {
		return amountResult{}, err
	}
	amount := base.Mul(rate)
	capped := false
	maxAmount := ""
	if strings.TrimSpace(config.MaxAmount) != "" {
		max, err := parseConfigDecimal(config.MaxAmount, "rewardConfig.maxAmount", ReasonRatioMaxAmountInvalid)
		if err != nil {
			return amountResult{}, err
		}
		maxAmount = max.String()
		if amount.Cmp(max) > 0 {
			amount = max
			capped = true
		}
	}
	return amountResult{amount: amount, payload: map[string]any{
		"baseField":  config.BaseField,
		"baseAmount": base.String(),
		"rate":       rate.String(),
		"maxAmount":  maxAmount,
		"capped":     capped,
	}}, nil
}

type parsedLadder struct {
	index  int
	min    decimal.Decimal
	max    decimal.Decimal
	amount decimal.Decimal
}

func calculateLadder(ctx Context, config marketing.RewardConfig) (amountResult, error) {
	if strings.TrimSpace(config.Field) == "" {
		return amountResult{}, rewardError(ReasonInvalidRewardConfig, "field is required", "rewardConfig.field")
	}
	if len(config.Ladders) == 0 {
		return amountResult{}, rewardError(ReasonInvalidRewardConfig, "ladders is required", "rewardConfig.ladders")
	}
	fieldValue, err := decimalFromField(ctx, config.Field, ReasonFieldMissing)
	if err != nil {
		return amountResult{}, err
	}
	ladders, err := parseLadders(config.Ladders)
	if err != nil {
		return amountResult{}, err
	}
	var matched *parsedLadder
	for i := range ladders {
		ladder := ladders[i]
		if fieldValue.Cmp(ladder.min) >= 0 && fieldValue.Cmp(ladder.max) <= 0 {
			if matched != nil {
				return amountResult{}, rewardError(ReasonLadderOverlapped, "multiple ladders matched", "rewardConfig.ladders")
			}
			matched = &ladder
		}
	}
	if matched == nil {
		return amountResult{}, rewardError(ReasonLadderNotMatched, "field value did not match any ladder", config.Field)
	}
	return amountResult{amount: matched.amount, payload: map[string]any{
		"field":       config.Field,
		"fieldValue":  fieldValue.String(),
		"ladderIndex": matched.index,
		"ladderMin":   matched.min.String(),
		"ladderMax":   matched.max.String(),
	}}, nil
}

func parseLadders(configured []marketing.RewardLadder) ([]parsedLadder, error) {
	ladders := make([]parsedLadder, 0, len(configured))
	for i, ladder := range configured {
		min, err := parseConfigDecimal(ladder.Min, fmt.Sprintf("rewardConfig.ladders[%d].min", i), ReasonInvalidRewardConfig)
		if err != nil {
			return nil, err
		}
		max, err := parseConfigDecimal(ladder.Max, fmt.Sprintf("rewardConfig.ladders[%d].max", i), ReasonInvalidRewardConfig)
		if err != nil {
			return nil, err
		}
		amount, err := parseConfigDecimal(ladder.Amount, fmt.Sprintf("rewardConfig.ladders[%d].amount", i), ReasonNegativeAmount)
		if err != nil {
			return nil, err
		}
		if min.Cmp(max) > 0 {
			return nil, rewardError(ReasonInvalidRewardConfig, "ladder min must be <= max", fmt.Sprintf("rewardConfig.ladders[%d]", i))
		}
		ladders = append(ladders, parsedLadder{index: i, min: min, max: max, amount: amount})
	}
	sort.SliceStable(ladders, func(i, j int) bool {
		return ladders[i].min.Cmp(ladders[j].min) < 0
	})
	for i := 1; i < len(ladders); i++ {
		if ladders[i].min.Cmp(ladders[i-1].max) <= 0 {
			return nil, rewardError(ReasonLadderOverlapped, "ladder ranges overlap", "rewardConfig.ladders")
		}
	}
	return ladders, nil
}

func calculateRandom(ctx Context, configuredReward marketing.Reward, config marketing.RewardConfig) (amountResult, error) {
	minAmount, err := parseConfigDecimal(config.MinAmount, "rewardConfig.minAmount", ReasonRandomRangeInvalid)
	if err != nil {
		return amountResult{}, err
	}
	maxAmount, err := parseConfigDecimal(config.MaxAmount, "rewardConfig.maxAmount", ReasonRandomRangeInvalid)
	if err != nil {
		return amountResult{}, err
	}
	if minAmount.Cmp(maxAmount) > 0 {
		return amountResult{}, rewardError(ReasonRandomRangeInvalid, "minAmount must be <= maxAmount", "rewardConfig.minAmount")
	}
	stepText := strings.TrimSpace(config.Step)
	if stepText == "" {
		return amountResult{}, rewardError(ReasonRandomStepInvalid, "step is required", "rewardConfig.step")
	}
	step, err := parsePositiveDecimal(stepText, "rewardConfig.step", ReasonRandomStepInvalid)
	if err != nil {
		return amountResult{}, err
	}
	candidateCount := int64(1)
	if minAmount.Cmp(maxAmount) < 0 {
		diff := maxAmount.Sub(minAmount)
		if step.Cmp(diff) <= 0 {
			steps, err := diff.FloorDivInt64(step)
			if err != nil {
				return amountResult{}, rewardError(ReasonRandomStepInvalid, "random range is too large", "rewardConfig.step")
			}
			candidateCount = steps + 1
		}
	}
	seedMaterial := seedMaterial(ctx, configuredReward, config.SeedSalt)
	index := deterministicIndex(seedMaterial, candidateCount)
	indexDecimal := decimal.MustParse(fmt.Sprintf("%d", index))
	amount := minAmount.Add(step.Mul(indexDecimal))
	return amountResult{amount: amount, payload: map[string]any{
		"minAmount":      minAmount.String(),
		"maxAmount":      maxAmount.String(),
		"step":           step.String(),
		"seedSource":     seedMaterial,
		"seedSalt":       config.SeedSalt,
		"selectedIndex":  index,
		"candidateCount": candidateCount,
	}}, nil
}

func calculateProbability(ctx Context, configuredReward marketing.Reward, config marketing.RewardConfig) (amountResult, error) {
	if len(config.Items) == 0 {
		return amountResult{}, rewardError(ReasonProbabilityItemsEmpty, "items is required", "rewardConfig.items")
	}
	seen := map[string]struct{}{}
	totalWeight := decimal.Zero()
	weights := make([]decimal.Decimal, len(config.Items))
	amounts := make([]decimal.Decimal, len(config.Items))
	for i, item := range config.Items {
		if strings.TrimSpace(item.ItemCode) == "" {
			return amountResult{}, rewardError(ReasonProbabilityItemInvalid, "itemCode is required", fmt.Sprintf("rewardConfig.items[%d].itemCode", i))
		}
		if _, ok := seen[item.ItemCode]; ok {
			return amountResult{}, rewardError(ReasonProbabilityItemInvalid, "itemCode is duplicated", fmt.Sprintf("rewardConfig.items[%d].itemCode", i))
		}
		seen[item.ItemCode] = struct{}{}
		weight, err := parseConfigDecimal(item.Weight, fmt.Sprintf("rewardConfig.items[%d].weight", i), ReasonProbabilityItemInvalid)
		if err != nil {
			return amountResult{}, err
		}
		if item.Currency != "" && config.Currency != "" && item.Currency != config.Currency {
			return amountResult{}, rewardError(ReasonProbabilityItemInvalid, "item currency must match reward currency", fmt.Sprintf("rewardConfig.items[%d].currency", i))
		}
		totalWeight = totalWeight.Add(weight)
		weights[i] = weight
		if amount, err := probabilityItemAmount(configuredReward.Type, item, i); err != nil {
			return amountResult{}, err
		} else {
			amounts[i] = amount
		}
	}
	if totalWeight.Cmp(decimal.MustParse("100")) != 0 {
		return amountResult{}, rewardError(ReasonProbabilityTotalInvalid, "total weight must equal 100.0000", "rewardConfig.items")
	}
	seedMaterial := seedMaterial(ctx, configuredReward, config.SeedSalt)
	threshold := deterministicThreshold(seedMaterial, totalWeight)
	selected := len(config.Items) - 1
	cumulative := decimal.Zero()
	for i, weight := range weights {
		cumulative = cumulative.Add(weight)
		if threshold.Cmp(cumulative) < 0 {
			selected = i
			break
		}
	}
	item := config.Items[selected]
	rewardType := item.RewardType
	if rewardType == "" {
		rewardType = configuredReward.Type
	}
	payload, err := probabilityItemPayload(rewardType, item, amounts[selected], selected)
	if err != nil {
		return amountResult{}, err
	}
	payload["itemCode"] = item.ItemCode
	payload["selectedIndex"] = selected
	payload["weight"] = weights[selected].String()
	payload["totalWeight"] = totalWeight.String()
	payload["seedSource"] = seedMaterial
	payload["seedSalt"] = config.SeedSalt
	payload["label"] = item.Label
	return amountResult{amount: amounts[selected], rewardType: rewardType, currency: firstNonEmpty(item.Currency, config.Currency), payload: payload}, nil
}

func probabilityItemAmount(defaultType marketing.RewardType, item marketing.ProbabilityItem, index int) (decimal.Decimal, error) {
	rewardType := item.RewardType
	if rewardType == "" {
		rewardType = defaultType
	}
	switch rewardType {
	case marketing.RewardBonus, marketing.RewardCash:
		return parseConfigDecimal(item.Amount, fmt.Sprintf("rewardConfig.items[%d].amount", index), ReasonProbabilityItemInvalid)
	case marketing.RewardPoint:
		value := firstNonEmpty(item.Points, item.Amount)
		if _, err := parsePositiveInt(value, fmt.Sprintf("rewardConfig.items[%d].points", index)); err != nil {
			return decimal.Decimal{}, err
		}
		return decimal.Zero(), nil
	case marketing.RewardCoupon, marketing.RewardGift, marketing.RewardDrawChance, marketing.RewardMembership:
		return decimal.Zero(), nil
	default:
		return decimal.Decimal{}, rewardError(ReasonProbabilityItemInvalid, "item rewardType is unsupported", fmt.Sprintf("rewardConfig.items[%d].rewardType", index))
	}
}

func probabilityItemPayload(rewardType marketing.RewardType, item marketing.ProbabilityItem, amount decimal.Decimal, index int) (map[string]any, error) {
	payload := map[string]any{}
	switch rewardType {
	case marketing.RewardBonus, marketing.RewardCash:
		payload["amount"] = amount.String()
		copyIfNotEmpty(payload, "currency", item.Currency)
	case marketing.RewardPoint:
		points, err := parsePositiveInt(firstNonEmpty(item.Points, item.Amount), fmt.Sprintf("rewardConfig.items[%d].points", index))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(firstNonEmpty(item.Unit, item.PointType)) == "" {
			return nil, rewardError(ReasonProbabilityItemInvalid, "unit is required", fmt.Sprintf("rewardConfig.items[%d].unit", index))
		}
		payload["points"] = points
		payload["unit"] = firstNonEmpty(item.Unit, item.PointType)
		payload["pointType"] = firstNonEmpty(item.PointType, item.Unit)
	case marketing.RewardCoupon:
		if strings.TrimSpace(item.CouponTemplateID) == "" {
			return nil, rewardError(ReasonProbabilityItemInvalid, "couponTemplateId is required", fmt.Sprintf("rewardConfig.items[%d].couponTemplateId", index))
		}
		quantity, err := parsePositiveInt(item.Quantity, fmt.Sprintf("rewardConfig.items[%d].quantity", index))
		if err != nil {
			return nil, err
		}
		payload["quantity"] = quantity
		copyIfNotEmpty(payload, "couponTemplateId", item.CouponTemplateID)
		copyIfNotEmpty(payload, "couponType", item.CouponType)
		copyIfNotEmpty(payload, "faceValue", item.FaceValue)
		copyIfNotEmpty(payload, "discountRate", item.DiscountRate)
		copyIfNotEmpty(payload, "currency", item.Currency)
	case marketing.RewardGift:
		if strings.TrimSpace(firstNonEmpty(item.GiftSkuID, item.GiftCode)) == "" {
			return nil, rewardError(ReasonProbabilityItemInvalid, "giftSkuId or giftCode is required", fmt.Sprintf("rewardConfig.items[%d].giftSkuId", index))
		}
		quantity, err := parsePositiveInt(item.Quantity, fmt.Sprintf("rewardConfig.items[%d].quantity", index))
		if err != nil {
			return nil, err
		}
		payload["quantity"] = quantity
		copyIfNotEmpty(payload, "giftSkuId", item.GiftSkuID)
		copyIfNotEmpty(payload, "giftCode", item.GiftCode)
		copyIfNotEmpty(payload, "deliveryMode", item.DeliveryMode)
		copyIfNotEmpty(payload, "valuationAmount", item.ValuationAmount)
		copyIfNotEmpty(payload, "currency", item.Currency)
	case marketing.RewardDrawChance:
		if strings.TrimSpace(firstNonEmpty(item.DrawPoolCode, item.LotteryCode)) == "" {
			return nil, rewardError(ReasonProbabilityItemInvalid, "drawPoolCode or lotteryCode is required", fmt.Sprintf("rewardConfig.items[%d].drawPoolCode", index))
		}
		chanceCount, err := parsePositiveInt(item.ChanceCount, fmt.Sprintf("rewardConfig.items[%d].chanceCount", index))
		if err != nil {
			return nil, err
		}
		payload["chanceCount"] = chanceCount
		copyIfNotEmpty(payload, "drawPoolCode", item.DrawPoolCode)
		copyIfNotEmpty(payload, "lotteryCode", item.LotteryCode)
	case marketing.RewardMembership:
		if strings.TrimSpace(firstNonEmpty(item.MembershipTier, item.MembershipPlanID, item.BenefitPackageID)) == "" {
			return nil, rewardError(ReasonProbabilityItemInvalid, "membershipTier, membershipPlanId, or benefitPackageId is required", fmt.Sprintf("rewardConfig.items[%d].membershipTier", index))
		}
		if strings.EqualFold(strings.TrimSpace(item.DurationType), "PERMANENT") {
			payload["durationType"] = "PERMANENT"
		} else {
			durationDays, err := parsePositiveInt(item.DurationDays, fmt.Sprintf("rewardConfig.items[%d].durationDays", index))
			if err != nil {
				return nil, err
			}
			payload["durationDays"] = durationDays
		}
		copyIfNotEmpty(payload, "membershipTier", item.MembershipTier)
		copyIfNotEmpty(payload, "membershipPlanId", item.MembershipPlanID)
		copyIfNotEmpty(payload, "benefitPackageId", item.BenefitPackageID)
		copyIfNotEmpty(payload, "valuationAmount", item.ValuationAmount)
		copyIfNotEmpty(payload, "currency", item.Currency)
	default:
		return nil, rewardError(ReasonProbabilityItemInvalid, "item rewardType is unsupported", fmt.Sprintf("rewardConfig.items[%d].rewardType", index))
	}
	return payload, nil
}

func calculatePoolSplit(ctx Context, config marketing.RewardConfig) (amountResult, error) {
	poolAmount, err := parseConfigDecimal(config.PoolAmount, "rewardConfig.poolAmount", ReasonPoolAmountInvalid)
	if err != nil {
		return amountResult{}, err
	}
	splitBy := strings.ToUpper(strings.TrimSpace(firstNonEmpty(config.SplitBy, config.SplitMode)))
	switch splitBy {
	case "COUNT":
		countField := firstNonEmpty(config.CountField, config.ParticipantCountField)
		count, err := decimalFromField(ctx, countField, ReasonPoolParticipantInvalid)
		if err != nil {
			return amountResult{}, err
		}
		if !count.IsPositive() {
			return amountResult{}, rewardError(ReasonPoolParticipantInvalid, "participant count must be positive", countField)
		}
		amount, err := poolAmount.Div(count)
		if err != nil {
			return amountResult{}, rewardError(ReasonPoolParticipantInvalid, "participant count must be positive", countField)
		}
		return amountResult{amount: amount, payload: map[string]any{
			"poolAmount":       poolAmount.String(),
			"splitBy":          splitBy,
			"participantCount": count.String(),
		}}, nil
	case "WEIGHT":
		weight, err := decimalFromField(ctx, config.WeightField, ReasonPoolWeightInvalid)
		if err != nil {
			return amountResult{}, err
		}
		totalWeight, err := decimalFromField(ctx, config.TotalWeightField, ReasonPoolWeightInvalid)
		if err != nil {
			return amountResult{}, err
		}
		if weight.IsNegative() || !totalWeight.IsPositive() || weight.Cmp(totalWeight) > 0 {
			return amountResult{}, rewardError(ReasonPoolWeightInvalid, "weight must be >= 0 and <= totalWeight", "rewardConfig.weightField")
		}
		weighted := poolAmount.Mul(weight)
		amount, err := weighted.Div(totalWeight)
		if err != nil {
			return amountResult{}, rewardError(ReasonPoolWeightInvalid, "totalWeight must be positive", config.TotalWeightField)
		}
		return amountResult{amount: amount, payload: map[string]any{
			"poolAmount":  poolAmount.String(),
			"splitBy":     splitBy,
			"weight":      weight.String(),
			"totalWeight": totalWeight.String(),
		}}, nil
	default:
		return amountResult{}, rewardError(ReasonInvalidRewardConfig, "splitBy is unsupported", "rewardConfig.splitBy")
	}
}

func decimalFromField(ctx Context, path string, missingReason string) (decimal.Decimal, error) {
	value, ok := fieldValue(ctx, path)
	if !ok {
		return decimal.Decimal{}, rewardError(missingReason, "field is missing", path)
	}
	text, ok := decimalScalarString(value)
	if !ok {
		return decimal.Decimal{}, rewardError(ReasonFieldTypeMismatch, "field value is not a decimal scalar", path)
	}
	parsed, err := decimal.Parse(text)
	if err != nil {
		return decimal.Decimal{}, rewardError(ReasonDecimalParseError, "field value is not decimal", path)
	}
	return parsed, nil
}

func fieldValue(ctx Context, path string) (any, bool) {
	switch {
	case path == "eventId":
		return ctx.Event.EventID, ctx.Event.EventID != ""
	case path == "eventType":
		return ctx.Event.EventType, ctx.Event.EventType != ""
	case path == "sourceSystem":
		return ctx.Event.SourceSystem, ctx.Event.SourceSystem != ""
	case path == "tenantId":
		return ctx.Event.TenantID, ctx.Event.TenantID != ""
	case path == "subject.type":
		return ctx.Event.Subject.Type, ctx.Event.Subject.Type != ""
	case path == "subject.id":
		return ctx.Event.Subject.ID, ctx.Event.Subject.ID != ""
	case path == "biz.bizType":
		return ctx.Event.Biz.BizType, ctx.Event.Biz.BizType != ""
	case path == "biz.bizId":
		return ctx.Event.Biz.BizID, ctx.Event.Biz.BizID != ""
	case strings.HasPrefix(path, "attributes."):
		return nestedValue(ctx.Event.Attributes, strings.TrimPrefix(path, "attributes."))
	case strings.HasPrefix(path, "state."):
		return nestedValue(ctx.State, strings.TrimPrefix(path, "state."))
	case strings.HasPrefix(path, "facts."):
		return nestedValue(ctx.Facts, strings.TrimPrefix(path, "facts."))
	default:
		return nil, false
	}
}

func nestedValue(root map[string]any, path string) (any, bool) {
	if root == nil || path == "" {
		return nil, false
	}
	var current any = root
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok || current == nil {
			return nil, false
		}
	}
	return current, true
}

func decimalScalarString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", false
		}
		return typed, true
	case json.Number:
		return typed.String(), true
	case int:
		return fmt.Sprintf("%d", typed), true
	case int64:
		return fmt.Sprintf("%d", typed), true
	case int32:
		return fmt.Sprintf("%d", typed), true
	case uint:
		return fmt.Sprintf("%d", typed), true
	case uint64:
		return fmt.Sprintf("%d", typed), true
	case uint32:
		return fmt.Sprintf("%d", typed), true
	default:
		return "", false
	}
}

func parseConfigDecimal(value, field, negativeReason string) (decimal.Decimal, error) {
	parsed, err := decimal.Parse(value)
	if err != nil {
		return decimal.Decimal{}, rewardError(ReasonDecimalParseError, "value must be decimal", field)
	}
	if parsed.IsNegative() {
		return decimal.Decimal{}, rewardError(negativeReason, "value must not be negative", field)
	}
	return parsed, nil
}

func parsePositiveDecimal(value, field, reason string) (decimal.Decimal, error) {
	parsed, err := decimal.Parse(value)
	if err != nil {
		return decimal.Decimal{}, rewardError(ReasonDecimalParseError, "value must be decimal", field)
	}
	if !parsed.IsPositive() {
		return decimal.Decimal{}, rewardError(reason, "value must be positive", field)
	}
	return parsed, nil
}

func parsePositiveInt(value, field string) (int, error) {
	parsed, err := parsePositiveDecimal(value, field, ReasonInvalidRewardConfig)
	if err != nil {
		return 0, err
	}
	if !parsed.IsInteger() {
		return 0, rewardError(ReasonInvalidRewardConfig, "value must be positive integer", field)
	}
	asText := parsed.String()
	var integer int
	if _, err := fmt.Sscanf(asText, "%d.0000", &integer); err != nil || integer <= 0 {
		return 0, rewardError(ReasonInvalidRewardConfig, "value must be positive integer", field)
	}
	return integer, nil
}

func copyIfNotEmpty(payload map[string]any, key string, value string) {
	if strings.TrimSpace(value) != "" {
		payload[key] = value
	}
}

func skippedResult(configuredReward marketing.Reward, reasonCode, reason string, payload map[string]any) Result {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["reasonCode"] = reasonCode
	return Result{
		Type:       configuredReward.Type,
		Amount:     decimal.Zero().String(),
		Payload:    payload,
		Skipped:    true,
		ReasonCode: reasonCode,
		Reason:     reason,
	}
}

func rewardError(reasonCode, reason, field string) error {
	return Error{ReasonCode: reasonCode, Reason: reason, Field: field}
}

func seedMaterial(ctx Context, configuredReward marketing.Reward, seedSalt string) string {
	tenantID := firstNonEmpty(ctx.TenantID, ctx.Event.TenantID)
	idempotencyKey := ctx.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = ctx.Event.EventID + ":" + ctx.ActivityID + ":" + configuredReward.Code
	}
	return strings.Join([]string{
		tenantID,
		ctx.ActivityID,
		configuredReward.Code,
		idempotencyKey,
		fmt.Sprintf("%d", configuredReward.Version),
		seedSalt,
	}, "|")
}

func deterministicIndex(seed string, count int64) int64 {
	if count <= 1 {
		return 0
	}
	hash := sha256.Sum256([]byte(seed))
	value := new(big.Int).SetBytes(hash[:])
	mod := new(big.Int).Mod(value, big.NewInt(count))
	return mod.Int64()
}

func deterministicThreshold(seed string, total decimal.Decimal) decimal.Decimal {
	const scale = int64(1_000_000_000_000)
	index := deterministicIndex(seed, scale)
	numerator := total.Mul(decimal.MustParse(fmt.Sprintf("%d", index)))
	threshold, _ := numerator.Div(decimal.MustParse(fmt.Sprintf("%d", scale)))
	return threshold
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "0"
}
