package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const (
	ReasonPluginNotRegistered = "PLUGIN_NOT_REGISTERED"
	ReasonPluginIncompatible  = "PLUGIN_INCOMPATIBLE_ACTIVITY_TYPE"
	ReasonPluginPrecheck      = "PLUGIN_PRECHECK_REJECTED"
	ReasonPluginContextError  = "PLUGIN_CONTEXT_ERROR"
	ReasonPluginIntentError   = "PLUGIN_INTENT_ERROR"
)

type ActivityPlugin interface {
	Code() string
	CompatibleActivityTypes() []string
	ValidateConfig(json.RawMessage) error
	BuildContext(context.Context, BuildContextRequest) (ContextResult, error)
	AfterQualified(context.Context, AfterQualifiedRequest) ([]StateChangeIntent, error)
}

type BuildContextRequest struct {
	Activity marketing.Activity
	Event    marketing.ActivityEvent
}

type ContextResult struct {
	Context  map[string]any `json:"context,omitempty"`
	Precheck PrecheckResult `json:"precheckResult"`
	Explain  map[string]any `json:"explain,omitempty"`
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
	plugins map[string]ActivityPlugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]ActivityPlugin{}}
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
		r.plugins = map[string]ActivityPlugin{}
	}
	r.plugins[code] = activityPlugin
	return nil
}

func (r *Registry) Get(code string) (ActivityPlugin, bool) {
	if r == nil {
		return nil, false
	}
	activityPlugin, ok := r.plugins[strings.TrimSpace(code)]
	return activityPlugin, ok
}

func (r *Registry) IsCompatible(code string, activityType string) bool {
	activityPlugin, ok := r.Get(code)
	if !ok {
		return false
	}
	types := activityPlugin.CompatibleActivityTypes()
	return len(types) == 0 || slices.Contains(types, activityType)
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
	for code, activityPlugin := range r.plugins {
		out = append(out, marketing.PluginRegistration{
			PluginCode:              code,
			CompatibleActivityTypes: slices.Clone(activityPlugin.CompatibleActivityTypes()),
		})
	}
	return out
}
