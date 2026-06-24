package marketing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactSensitiveRedactsNestedKeysURLsAndHeaders(t *testing.T) {
	payload := map[string]any{
		"headers": map[string]any{
			"Authorization": "Bearer secret-token-value",
			"Cookie":        "sid=secret-cookie",
		},
		"provider": map[string]any{
			"rawError": "upstream failed password=p@ssw0rd token=abc123",
			"url":      "https://user:pass@example.com/path?token=query-token&safe=1",
		},
		"items": []any{
			map[string]any{"callbackSecret": "callback-secret"},
		},
	}

	redacted := RedactSensitive(payload)
	bytes, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("marshal redacted: %v", err)
	}
	out := string(bytes)
	for _, secret := range []string{"secret-token-value", "secret-cookie", "p@ssw0rd", "abc123", "query-token", "callback-secret", "user:pass"} {
		if strings.Contains(out, secret) {
			t.Fatalf("redacted payload leaked %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, redactedValue) {
		t.Fatalf("expected redacted marker in %s", out)
	}
}

func TestRedactSensitiveStringHandlesProviderStatusAndDSNLikeText(t *testing.T) {
	input := "provider 500 Authorization: Bearer raw-token postgres://user:secret@localhost/db?password=pw&sslmode=disable"
	out := RedactSensitiveString(input)
	for _, secret := range []string{"raw-token", "secret", "password=pw"} {
		if strings.Contains(out, secret) {
			t.Fatalf("redacted string leaked %q: %s", secret, out)
		}
	}
}

func TestSensitivePayloadHashUsesRedactedPayload(t *testing.T) {
	left := SensitivePayloadHash(map[string]any{"headers": map[string]any{"Authorization": "Bearer token-a"}, "query": "ok"})
	right := SensitivePayloadHash(map[string]any{"headers": map[string]any{"Authorization": "Bearer token-b"}, "query": "ok"})
	if left == "" || !strings.HasPrefix(left, "hmac-sha256:") {
		t.Fatalf("expected hmac-sha256 hash, got %q", left)
	}
	if left != right {
		t.Fatalf("expected hashes to match after secret redaction, got %s and %s", left, right)
	}
	if strings.Contains(left, "token-a") || strings.Contains(right, "token-b") {
		t.Fatalf("hash leaked secret values: %s %s", left, right)
	}
}
