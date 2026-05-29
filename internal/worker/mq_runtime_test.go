package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLoadMQRuntimeConfigFromEnv(t *testing.T) {
	t.Setenv(mqSendersEnv, `{"providers":{"kafka":{"enabled":true,"endpoints":["kafka-1:9092"],"authRef":"secret/exmarketing/kafka","sendTimeoutMillis":5000,"destinationAllowlist":["reward.commands"]},"rabbitmq":{"enabled":false}}}`)

	config, ok, err := LoadMQRuntimeConfigFromEnv()
	if err != nil {
		t.Fatalf("expected config to load: %v", err)
	}
	if !ok {
		t.Fatal("expected config to be present")
	}
	if got := config.Providers["kafka"].AuthRef; got != "secret/exmarketing/kafka" {
		t.Fatalf("expected authRef to load, got %q", got)
	}
}

func TestMQRuntimeConfigRejectsCredentialEndpoint(t *testing.T) {
	t.Setenv(mqSendersEnv, `{"providers":{"kafka":{"enabled":true,"endpoints":["kafka://user:pass@broker:9092"]}}}`)

	_, _, err := LoadMQRuntimeConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("expected credential endpoint rejection, got %v", err)
	}
}

func TestRuntimeMQSenderRequiresConfiguredEnabledAllowedProvider(t *testing.T) {
	sender, err := NewRuntimeMQSender(MQRuntimeConfig{Providers: map[string]MQProviderConfig{
		"kafka": {Enabled: true, Endpoints: []string{"kafka-1:9092"}, DestinationAllowlist: []string{"reward.commands"}},
	}}, map[string]MQSender{"kafka": &recordingMQSender{}})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}
	if err := sender.Send(context.Background(), MQMessage{Provider: "rabbitmq", Destination: "reward.commands"}); err == nil {
		t.Fatal("expected missing provider error")
	}
	if err := sender.Send(context.Background(), MQMessage{Provider: "kafka", Destination: "other.commands"}); err == nil {
		t.Fatal("expected destination allowlist error")
	}
	if err := sender.Send(context.Background(), MQMessage{Provider: "kafka", Destination: "reward.commands"}); err != nil {
		t.Fatalf("expected allowed destination to send: %v", err)
	}
}

func TestRuntimeMQSenderWithoutTransportDoesNotAck(t *testing.T) {
	sender, err := NewRuntimeMQSender(MQRuntimeConfig{Providers: map[string]MQProviderConfig{
		"kafka": {Enabled: true, Endpoints: []string{"kafka-1:9092"}},
	}}, nil)
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}

	err = sender.Send(context.Background(), MQMessage{Provider: "kafka", Destination: "reward.commands"})
	if err == nil || !strings.Contains(err.Error(), "transport adapter is not registered") {
		t.Fatalf("expected missing transport error, got %v", err)
	}
}

func TestRuntimeMQSenderAppliesSendTimeout(t *testing.T) {
	sender, err := NewRuntimeMQSender(MQRuntimeConfig{Providers: map[string]MQProviderConfig{
		"kafka": {Enabled: true, Endpoints: []string{"kafka-1:9092"}, SendTimeoutMillis: 1},
	}}, map[string]MQSender{"kafka": delayedMQSender{delay: 20 * time.Millisecond}})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}

	err = sender.Send(context.Background(), MQMessage{Provider: "kafka", Destination: "reward.commands"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

type delayedMQSender struct {
	delay time.Duration
}

func (s delayedMQSender) Send(ctx context.Context, _ MQMessage) error {
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
