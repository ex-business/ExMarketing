package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type registryTestPlugin struct {
	code     string
	metadata Metadata
}

func (p registryTestPlugin) Code() string {
	return p.code
}

func (p registryTestPlugin) CompatibleActivityTypes() []string {
	return []string{"LOTTERY"}
}

func (p registryTestPlugin) Metadata() Metadata {
	return p.metadata
}

func (p registryTestPlugin) ValidateConfig(json.RawMessage) error {
	return nil
}

func (p registryTestPlugin) BuildContext(context.Context, BuildContextRequest) (ContextResult, error) {
	return ContextResult{
		Context: map[string]any{"drawChance": "1"},
		Precheck: PrecheckResult{
			Allowed:    true,
			ReasonCode: "OK",
			Reason:     "plugin precheck passed",
		},
	}, nil
}

func (p registryTestPlugin) AfterQualified(context.Context, AfterQualifiedRequest) ([]StateChangeIntent, error) {
	return []StateChangeIntent{{
		PluginCode: p.code,
		IntentType: "consumeDrawChance",
		Subject:    marketing.EventSubject{Type: "USER", ID: "user_001"},
	}}, nil
}

func TestRegistryRejectsDuplicatePluginCode(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(registryTestPlugin{code: "LOTTERY_TEST"}); err != nil {
		t.Fatalf("register first plugin: %v", err)
	}

	err := registry.Register(registryTestPlugin{code: "LOTTERY_TEST"})
	if err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("expected duplicate plugin code error, got %v", err)
	}

	plugin, ok := registry.Get("LOTTERY_TEST")
	if !ok || plugin.Code() != "LOTTERY_TEST" {
		t.Fatalf("registry lost original plugin: ok=%v plugin=%+v", ok, plugin)
	}
}

func TestRegistryResolvesHighestCompatibleVersionAndCapabilities(t *testing.T) {
	registry := NewRegistry()
	for _, version := range []string{"1.0.0", "1.2.0", "2.0.0"} {
		if err := registry.Register(registryTestPlugin{
			code: "LOTTERY_TEST",
			metadata: Metadata{
				PluginCode:             "LOTTERY_TEST",
				Version:                version,
				Capabilities:           []string{CapabilityConfigValidate, CapabilityContextBuild, CapabilityBeforeReward},
				SupportedActivityTypes: []string{"LOTTERY"},
				ConfigSchema:           json.RawMessage(`{"type":"object"}`),
				StateSchema:            json.RawMessage(`{"type":"object"}`),
				DisableStrategies:      []string{DisabledStrategyBlock, DisabledStrategyReadOnly},
			},
		}); err != nil {
			t.Fatalf("register %s: %v", version, err)
		}
	}

	registration, err := registry.Resolve("LOTTERY_TEST", "^1.0.0", ResolveOptions{
		ActivityType:         "LOTTERY",
		RequiredCapabilities: []string{CapabilityBeforeReward},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if registration.Metadata.Version != "1.2.0" {
		t.Fatalf("version=%s, want highest compatible 1.2.0", registration.Metadata.Version)
	}
	if string(registration.Metadata.ConfigSchema) == "" || string(registration.Metadata.StateSchema) == "" {
		t.Fatalf("expected schemas in registration: %+v", registration.Metadata)
	}
}

func TestRegistryRejectsMissingCapabilityAndDisabledPlugin(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(registryTestPlugin{
		code: "LOTTERY_TEST",
		metadata: Metadata{
			PluginCode:             "LOTTERY_TEST",
			Version:                "1.0.0",
			Capabilities:           []string{CapabilityConfigValidate},
			SupportedActivityTypes: []string{"LOTTERY"},
			Disabled:               true,
			DisableStrategies:      []string{DisabledStrategyBlock},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	if _, err := registry.Resolve("LOTTERY_TEST", "1.0.0", ResolveOptions{
		ActivityType:         "LOTTERY",
		RequiredCapabilities: []string{CapabilityContextBuild},
		IncludeDisabled:      true,
	}); err == nil || !strings.Contains(err.Error(), ReasonPluginVersionUnresolved) {
		t.Fatalf("expected unresolved when required capability is missing, got %v", err)
	}
	if _, err := registry.Resolve("LOTTERY_TEST", "1.0.0", ResolveOptions{ActivityType: "LOTTERY"}); err == nil || !strings.Contains(err.Error(), ReasonPluginVersionUnresolved) {
		t.Fatalf("expected disabled plugin to be excluded, got %v", err)
	}
}
