package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	ReasonPluginNotRegistered       = "PLUGIN_NOT_FOUND"
	ReasonPluginVersionUnresolved   = "PLUGIN_VERSION_UNRESOLVED"
	ReasonPluginVersionIncompatible = "PLUGIN_VERSION_INCOMPATIBLE"
	ReasonPluginCapabilityMissing   = "PLUGIN_CAPABILITY_MISSING"
	ReasonPluginIncompatible        = "PLUGIN_ACTIVITY_TYPE_UNSUPPORTED"
	ReasonPluginConfigInvalid       = "PLUGIN_CONFIG_INVALID"
	ReasonPluginStateSchemaInvalid  = "PLUGIN_STATE_SCHEMA_INVALID"
	ReasonPluginDisabled            = "PLUGIN_DISABLED"
	ReasonPluginPrecheck            = "PLUGIN_PRECHECK_REJECTED"
	ReasonPluginExecutionFailed     = "PLUGIN_EXECUTION_FAILED"
	ReasonPluginRollbackFailed      = "PLUGIN_ROLLBACK_FAILED"
	ReasonPluginContextError        = ReasonPluginExecutionFailed
	ReasonPluginIntentError         = ReasonPluginExecutionFailed

	CapabilityConfigValidate        = "CONFIG_VALIDATE"
	CapabilityContextBuild          = "CONTEXT_BUILD"
	CapabilityBeforeMatch           = "BEFORE_MATCH"
	CapabilityBeforeReward          = "BEFORE_REWARD"
	CapabilityAfterRewardCalculated = "AFTER_REWARD_CALCULATED"
	CapabilityEventHandle           = "EVENT_HANDLE"
	CapabilityRollback              = "ROLLBACK"
	CapabilityDisablePolicy         = "DISABLE_POLICY"

	DisabledStrategyBlock    = "BLOCK"
	DisabledStrategyDegrade  = "DEGRADE"
	DisabledStrategyReadOnly = "READ_ONLY"
)

type ActivityPlugin interface {
	Code() string
	CompatibleActivityTypes() []string
	ValidateConfig(json.RawMessage) error
	BuildContext(context.Context, BuildContextRequest) (ContextResult, error)
	AfterQualified(context.Context, AfterQualifiedRequest) ([]StateChangeIntent, error)
}

type MetadataProvider interface {
	Metadata() Metadata
}

type Metadata struct {
	PluginCode             string          `json:"pluginCode"`
	Version                string          `json:"version"`
	Capabilities           []string        `json:"capabilities,omitempty"`
	SupportedActivityTypes []string        `json:"supportedActivityTypes,omitempty"`
	ConfigSchema           json.RawMessage `json:"configSchema,omitempty"`
	StateSchema            json.RawMessage `json:"stateSchema,omitempty"`
	Disabled               bool            `json:"disabled,omitempty"`
	DisableStrategies      []string        `json:"disableStrategies,omitempty"`
}

type Registration struct {
	Metadata Metadata
	Plugin   ActivityPlugin
}

type ResolveOptions struct {
	ActivityType         string
	RequiredCapabilities []string
	IncludeDisabled      bool
}

type BuildContextRequest struct {
	Activity marketing.Activity
	Event    marketing.ActivityEvent
}

type ContextResult struct {
	PluginCode       string           `json:"pluginCode,omitempty"`
	PluginVersion    string           `json:"pluginVersion,omitempty"`
	LifecycleStep    string           `json:"lifecycleStep,omitempty"`
	Status           string           `json:"status,omitempty"`
	ReasonCode       string           `json:"reasonCode,omitempty"`
	Reason           string           `json:"reason,omitempty"`
	Context          map[string]any   `json:"context,omitempty"`
	Precheck         PrecheckResult   `json:"precheckResult"`
	Explain          map[string]any   `json:"explain,omitempty"`
	CandidateResults []map[string]any `json:"candidateResults,omitempty"`
	RollbackToken    string           `json:"rollbackToken,omitempty"`
}

type PrecheckResult struct {
	Allowed    bool   `json:"allowed"`
	ReasonCode string `json:"reasonCode,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type AfterQualifiedRequest struct {
	Activity                    marketing.Activity
	Event                       marketing.ActivityEvent
	ParticipationID             string
	ParticipationIdempotencyKey string
	PluginContext               map[string]any
}

type StateChangeIntent struct {
	PluginCode                  string                 `json:"pluginCode"`
	IntentType                  string                 `json:"intentType"`
	TenantID                    string                 `json:"tenantId"`
	ActivityID                  string                 `json:"activityId"`
	ActivityCode                string                 `json:"activityCode"`
	Subject                     marketing.EventSubject `json:"subject"`
	Biz                         marketing.EventBiz     `json:"biz"`
	EventID                     string                 `json:"eventId"`
	ParticipationID             string                 `json:"participationId"`
	ParticipationIdempotencyKey string                 `json:"participationIdempotencyKey"`
	IdempotencyKey              string                 `json:"idempotencyKey"`
	Payload                     map[string]any         `json:"payload,omitempty"`
}

type Registry struct {
	plugins map[string]map[string]Registration
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]map[string]Registration{}}
}

func (r *Registry) Register(activityPlugin ActivityPlugin) error {
	if activityPlugin == nil {
		return errors.New("plugin is nil")
	}
	code := strings.TrimSpace(activityPlugin.Code())
	if code == "" {
		return errors.New("plugin code is required")
	}
	if r.plugins == nil {
		r.plugins = map[string]map[string]Registration{}
	}
	metadata := normalizeMetadata(activityPlugin)
	if metadata.PluginCode != code {
		return errors.New("plugin metadata code must match plugin code")
	}
	if metadata.Version == "" {
		return errors.New("plugin version is required")
	}
	if _, err := parseSemver(metadata.Version); err != nil {
		return err
	}
	if len(metadata.SupportedActivityTypes) == 0 {
		return errors.New("plugin supported activity types are required")
	}
	if len(metadata.Capabilities) == 0 {
		return errors.New("plugin capabilities are required")
	}
	if r.plugins[code] == nil {
		r.plugins[code] = map[string]Registration{}
	}
	if _, exists := r.plugins[code][metadata.Version]; exists {
		return errors.New("plugin code and version is duplicated")
	}
	r.plugins[code][metadata.Version] = Registration{Metadata: metadata, Plugin: activityPlugin}
	return nil
}

func (r *Registry) Get(code string) (ActivityPlugin, bool) {
	registration, ok := r.GetRegistration(code)
	if !ok {
		return nil, false
	}
	return registration.Plugin, true
}

func (r *Registry) GetRegistration(code string) (Registration, bool) {
	registration, ok := r.Resolve(strings.TrimSpace(code), "", ResolveOptions{IncludeDisabled: true})
	return registration, ok == nil
}

func (r *Registry) Resolve(code string, versionRange string, options ResolveOptions) (Registration, error) {
	if r == nil {
		return Registration{}, errors.New(ReasonPluginNotRegistered + ": pluginCode is not registered")
	}
	versions := r.plugins[strings.TrimSpace(code)]
	if len(versions) == 0 {
		return Registration{}, errors.New(ReasonPluginNotRegistered + ": pluginCode is not registered")
	}
	candidates := make([]Registration, 0, len(versions))
	for _, registration := range versions {
		if !options.IncludeDisabled && registration.Metadata.Disabled {
			continue
		}
		if !versionMatches(registration.Metadata.Version, versionRange) {
			continue
		}
		if options.ActivityType != "" && !slices.Contains(registration.Metadata.SupportedActivityTypes, options.ActivityType) {
			continue
		}
		if missing := MissingCapabilities(registration.Metadata.Capabilities, options.RequiredCapabilities); len(missing) > 0 {
			continue
		}
		candidates = append(candidates, registration)
	}
	if len(candidates) == 0 {
		return Registration{}, errors.New(ReasonPluginVersionUnresolved + ": plugin version range cannot be resolved")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareSemver(candidates[i].Metadata.Version, candidates[j].Metadata.Version) > 0
	})
	return candidates[0], nil
}

func (r *Registry) IsCompatible(code string, activityType string) bool {
	_, err := r.Resolve(code, "", ResolveOptions{ActivityType: activityType, IncludeDisabled: true})
	return err == nil
}

func (r *Registry) Codes() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.plugins))
	for code := range r.plugins {
		out = append(out, code)
	}
	return out
}

func (r *Registry) Registrations() []marketing.PluginRegistration {
	if r == nil {
		return nil
	}
	out := make([]marketing.PluginRegistration, 0, len(r.plugins))
	for code, versions := range r.plugins {
		for _, registration := range versions {
			meta := registration.Metadata
			out = append(out, marketing.PluginRegistration{
				PluginCode:              code,
				Version:                 meta.Version,
				Capabilities:            slices.Clone(meta.Capabilities),
				CompatibleActivityTypes: slices.Clone(meta.SupportedActivityTypes),
				ConfigSchema:            slices.Clone(meta.ConfigSchema),
				StateSchema:             slices.Clone(meta.StateSchema),
				Disabled:                meta.Disabled,
				DisableStrategies:       slices.Clone(meta.DisableStrategies),
			})
		}
	}
	return out
}

func MissingCapabilities(declared []string, required []string) []string {
	set := map[string]struct{}{}
	for _, capability := range declared {
		set[strings.TrimSpace(capability)] = struct{}{}
	}
	var missing []string
	for _, capability := range required {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		if _, ok := set[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	return missing
}

func normalizeMetadata(activityPlugin ActivityPlugin) Metadata {
	code := strings.TrimSpace(activityPlugin.Code())
	metadata := Metadata{
		PluginCode:             code,
		Version:                "1.0.0",
		Capabilities:           []string{CapabilityConfigValidate, CapabilityContextBuild, CapabilityBeforeMatch, CapabilityAfterRewardCalculated},
		SupportedActivityTypes: slices.Clone(activityPlugin.CompatibleActivityTypes()),
		DisableStrategies:      []string{DisabledStrategyBlock},
	}
	if provider, ok := activityPlugin.(MetadataProvider); ok {
		custom := provider.Metadata()
		if strings.TrimSpace(custom.PluginCode) != "" {
			metadata.PluginCode = strings.TrimSpace(custom.PluginCode)
		}
		if strings.TrimSpace(custom.Version) != "" {
			metadata.Version = strings.TrimSpace(custom.Version)
		}
		if len(custom.Capabilities) > 0 {
			metadata.Capabilities = normalizeStringSlice(custom.Capabilities)
		}
		if len(custom.SupportedActivityTypes) > 0 {
			metadata.SupportedActivityTypes = normalizeStringSlice(custom.SupportedActivityTypes)
		}
		metadata.ConfigSchema = slices.Clone(custom.ConfigSchema)
		metadata.StateSchema = slices.Clone(custom.StateSchema)
		metadata.Disabled = custom.Disabled
		if len(custom.DisableStrategies) > 0 {
			metadata.DisableStrategies = normalizeStringSlice(custom.DisableStrategies)
		}
	}
	metadata.PluginCode = strings.TrimSpace(metadata.PluginCode)
	metadata.Version = strings.TrimSpace(metadata.Version)
	metadata.Capabilities = normalizeStringSlice(metadata.Capabilities)
	metadata.SupportedActivityTypes = normalizeStringSlice(metadata.SupportedActivityTypes)
	metadata.DisableStrategies = normalizeStringSlice(metadata.DisableStrategies)
	return metadata
}

func normalizeStringSlice(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type semver struct {
	major int
	minor int
	patch int
}

func parseSemver(value string) (semver, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("%s: plugin version must be MAJOR.MINOR.PATCH", ReasonPluginVersionIncompatible)
	}
	var out semver
	for i, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return semver{}, fmt.Errorf("%s: plugin version must be MAJOR.MINOR.PATCH", ReasonPluginVersionIncompatible)
		}
		switch i {
		case 0:
			out.major = parsed
		case 1:
			out.minor = parsed
		case 2:
			out.patch = parsed
		}
	}
	return out, nil
}

func compareSemver(left string, right string) int {
	a, errA := parseSemver(left)
	b, errB := parseSemver(right)
	if errA != nil || errB != nil {
		return strings.Compare(left, right)
	}
	if a.major != b.major {
		return a.major - b.major
	}
	if a.minor != b.minor {
		return a.minor - b.minor
	}
	return a.patch - b.patch
}

func versionMatches(version string, versionRange string) bool {
	versionRange = strings.TrimSpace(versionRange)
	if versionRange == "" || versionRange == "*" {
		return true
	}
	if strings.HasSuffix(versionRange, ".x") {
		return strings.HasPrefix(version, strings.TrimSuffix(versionRange, "x"))
	}
	if strings.HasPrefix(versionRange, "^") {
		base, err := parseSemver(strings.TrimPrefix(versionRange, "^"))
		current, errCurrent := parseSemver(version)
		return err == nil && errCurrent == nil && current.major == base.major && compareSemver(version, strings.TrimPrefix(versionRange, "^")) >= 0
	}
	if strings.Contains(versionRange, " ") {
		for _, constraint := range strings.Fields(versionRange) {
			if !matchConstraint(version, constraint) {
				return false
			}
		}
		return true
	}
	return version == versionRange
}

func matchConstraint(version string, constraint string) bool {
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(constraint, op) {
			cmp := compareSemver(version, strings.TrimSpace(strings.TrimPrefix(constraint, op)))
			switch op {
			case ">=":
				return cmp >= 0
			case "<=":
				return cmp <= 0
			case ">":
				return cmp > 0
			case "<":
				return cmp < 0
			case "=":
				return cmp == 0
			}
		}
	}
	return version == constraint
}
