package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type registryTestPlugin struct {
	code string
}

func (p registryTestPlugin) Code() string {
	return p.code
}

func (p registryTestPlugin) CompatibleActivityTypes() []string {
	return []string{"LOTTERY"}
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
