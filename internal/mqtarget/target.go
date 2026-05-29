package mqtarget

import (
	"fmt"
	"net/url"
	"strings"
)

type Target struct {
	Provider    string
	Destination string
}

func Parse(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("targetRef is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Target{}, fmt.Errorf("targetRef is invalid")
	}
	if parsed.User != nil {
		return Target{}, fmt.Errorf("targetRef must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return Target{}, fmt.Errorf("targetRef must not contain query or fragment")
	}
	switch parsed.Scheme {
	case "kafka":
		if parsed.Host == "" || strings.Trim(parsed.Path, "/") != "" {
			return Target{}, fmt.Errorf("kafka targetRef must be kafka://<topic>")
		}
		if !validDestinationPart(parsed.Host) {
			return Target{}, fmt.Errorf("kafka topic is invalid")
		}
		return Target{Provider: "kafka", Destination: parsed.Host}, nil
	case "rabbitmq":
		routingKey := strings.TrimPrefix(parsed.Path, "/")
		if routingKey == "" || strings.Contains(routingKey, "/") {
			return Target{}, fmt.Errorf("rabbitmq targetRef must be rabbitmq://<exchange>/<routingKey> or rabbitmq:///<routingKey>")
		}
		if !validDestinationPart(routingKey) {
			return Target{}, fmt.Errorf("rabbitmq routingKey is invalid")
		}
		exchange := parsed.Host
		if exchange == "default" {
			exchange = ""
		}
		if exchange != "" && !validDestinationPart(exchange) {
			return Target{}, fmt.Errorf("rabbitmq exchange is invalid")
		}
		if exchange == "" {
			return Target{Provider: "rabbitmq", Destination: routingKey}, nil
		}
		return Target{Provider: "rabbitmq", Destination: exchange + "/" + routingKey}, nil
	case "pulsar":
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if parsed.Host == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return Target{}, fmt.Errorf("pulsar targetRef must be pulsar://<tenant>/<namespace>/<topic>")
		}
		if !validDestinationPart(parsed.Host) || !validDestinationPart(parts[0]) || !validDestinationPart(parts[1]) {
			return Target{}, fmt.Errorf("pulsar destination is invalid")
		}
		return Target{Provider: "pulsar", Destination: parsed.Host + "/" + parts[0] + "/" + parts[1]}, nil
	default:
		return Target{}, fmt.Errorf("targetRef scheme must be kafka, rabbitmq, or pulsar")
	}
}

func validDestinationPart(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '.', '_', '-':
			continue
		default:
			return false
		}
	}
	return true
}
