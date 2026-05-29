package mqtarget

import "testing"

func TestParseAllowsMQDestinationRefs(t *testing.T) {
	for name, raw := range map[string]string{
		"kafka":           "kafka://reward.commands",
		"rabbit exchange": "rabbitmq://reward-exchange/reward.commands",
		"rabbit default":  "rabbitmq:///reward.commands",
		"pulsar":          "pulsar://tenant-a/rewards/commands",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw); err != nil {
				t.Fatalf("expected %s to parse: %v", raw, err)
			}
		})
	}
}

func TestParseRejectsCredentialsAndBrokerLikeRefs(t *testing.T) {
	for name, raw := range map[string]string{
		"userinfo":       "kafka://user:pass@reward.commands",
		"query":          "kafka://reward.commands?token=secret",
		"fragment":       "kafka://reward.commands#secret",
		"broker port":    "kafka://broker.example.com:9092",
		"suspicious key": "kafka://token=secret",
		"rabbit path":    "rabbitmq://reward-exchange/reward.commands/extra",
		"pulsar port":    "pulsar://broker.example.com:6650/rewards/commands",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw); err == nil {
				t.Fatalf("expected %s to be rejected", raw)
			}
		})
	}
}
