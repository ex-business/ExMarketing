package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const mqSendersEnv = "EX_MARKETING_MQ_SENDERS_JSON"

type MQRuntimeConfig struct {
	Providers map[string]MQProviderConfig `json:"providers"`
}

type MQProviderConfig struct {
	Enabled              bool     `json:"enabled"`
	Endpoints            []string `json:"endpoints"`
	AuthRef              string   `json:"authRef,omitempty"`
	TLS                  bool     `json:"tls,omitempty"`
	CAFile               string   `json:"caFile,omitempty"`
	ClientCertFile       string   `json:"clientCertFile,omitempty"`
	ClientKeyFile        string   `json:"clientKeyFile,omitempty"`
	SendTimeoutMillis    int      `json:"sendTimeoutMillis,omitempty"`
	DestinationAllowlist []string `json:"destinationAllowlist,omitempty"`
}

func LoadMQRuntimeConfigFromEnv() (MQRuntimeConfig, bool, error) {
	raw := strings.TrimSpace(os.Getenv(mqSendersEnv))
	if raw == "" {
		return MQRuntimeConfig{}, false, nil
	}
	var config MQRuntimeConfig
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return MQRuntimeConfig{}, true, fmt.Errorf("%s is invalid JSON: %w", mqSendersEnv, err)
	}
	if err := config.Validate(); err != nil {
		return MQRuntimeConfig{}, true, err
	}
	return config, true, nil
}

func (c MQRuntimeConfig) Validate() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("%s must define at least one provider", mqSendersEnv)
	}
	for provider, config := range c.Providers {
		if !isMQProvider(provider) {
			return fmt.Errorf("mq provider %s is unsupported", provider)
		}
		if !config.Enabled {
			continue
		}
		if len(config.Endpoints) == 0 {
			return fmt.Errorf("mq provider %s endpoints are required", provider)
		}
		for _, endpoint := range config.Endpoints {
			if invalidRuntimeEndpoint(endpoint) {
				return fmt.Errorf("mq provider %s endpoint must not contain credentials", provider)
			}
		}
		for _, destination := range config.DestinationAllowlist {
			if strings.TrimSpace(destination) == "" {
				return fmt.Errorf("mq provider %s destinationAllowlist contains empty destination", provider)
			}
		}
		if config.SendTimeoutMillis < 0 {
			return fmt.Errorf("mq provider %s sendTimeoutMillis must be non-negative", provider)
		}
	}
	return nil
}

type RuntimeMQSender struct {
	config     MQRuntimeConfig
	transports map[string]MQSender
}

func NewRuntimeMQSender(config MQRuntimeConfig, transports map[string]MQSender) (*RuntimeMQSender, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if transports == nil {
		transports = map[string]MQSender{}
	}
	return &RuntimeMQSender{config: config, transports: transports}, nil
}

func (s *RuntimeMQSender) Send(ctx context.Context, message MQMessage) error {
	providerConfig, ok := s.config.Providers[message.Provider]
	if !ok {
		return deliveryError{retryable: true, message: fmt.Sprintf("mq provider %s is not configured", message.Provider)}
	}
	if !providerConfig.Enabled {
		return deliveryError{retryable: true, message: fmt.Sprintf("mq provider %s is disabled", message.Provider)}
	}
	if !destinationAllowed(providerConfig.DestinationAllowlist, message.Destination) {
		return deliveryError{retryable: true, message: fmt.Sprintf("mq provider %s destination is not allowed", message.Provider)}
	}
	transport := s.transports[message.Provider]
	if transport == nil {
		return deliveryError{retryable: true, message: fmt.Sprintf("mq provider %s transport adapter is not registered", message.Provider)}
	}
	if providerConfig.SendTimeoutMillis > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(providerConfig.SendTimeoutMillis)*time.Millisecond)
		defer cancel()
	}
	return transport.Send(ctx, message)
}

func isMQProvider(provider string) bool {
	switch provider {
	case "kafka", "rabbitmq", "pulsar":
		return true
	default:
		return false
	}
}

func destinationAllowed(allowlist []string, destination string) bool {
	if len(allowlist) == 0 {
		return true
	}
	for _, allowed := range allowlist {
		if strings.TrimSpace(allowed) == destination {
			return true
		}
	}
	return false
}

func invalidRuntimeEndpoint(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	lower := strings.ToLower(raw)
	if strings.Contains(raw, "@") || strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") {
		return true
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.User != nil
}
