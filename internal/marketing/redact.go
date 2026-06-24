package marketing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const redactedValue = "[redacted]"

var sensitiveValuePattern = regexp.MustCompile(`(?i)(authorization:\s*bearer\s+|bearer\s+|password=|passwd=|pwd=|token=|secret=|signature=|credential=|client[_-]?key=|private[_-]?key=)[^&\s;,"'}]+`)
var urlUserInfoPattern = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^:/\s]+:)[^@\s/]+(@)`)

func RedactSensitive(value any) any {
	normalized, err := normalizeForRedaction(value)
	if err != nil {
		return RedactSensitiveString(err.Error())
	}
	return redactNormalized(normalized, "")
}

func RedactSensitiveString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if redactedURL, ok := redactURL(value); ok {
		value = redactedURL
	}
	value = urlUserInfoPattern.ReplaceAllString(value, "${1}"+redactedValue+"${2}")
	return sensitiveValuePattern.ReplaceAllStringFunc(value, func(match string) string {
		idx := strings.IndexAny(match, "=:")
		if idx < 0 {
			return redactedValue
		}
		prefix := match[:idx+1]
		if strings.Contains(strings.ToLower(match[:idx]), "bearer") {
			return "Bearer " + redactedValue
		}
		return prefix + redactedValue
	})
}

func SensitivePayloadHash(value any) string {
	redacted := RedactSensitive(value)
	payload, err := json.Marshal(redacted)
	if err != nil {
		payload = []byte(RedactSensitiveString(err.Error()))
	}
	mac := hmac.New(sha256.New, []byte(serviceCallAuditHashSecret()))
	_, _ = mac.Write(payload)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

func serviceCallAuditHashSecret() string {
	if value := strings.TrimSpace(os.Getenv("EX_MARKETING_AUDIT_HASH_SECRET")); value != "" {
		return value
	}
	return "exmarketing-service-call-audit-v1"
}

func normalizeForRedaction(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		var out any
		if err := json.Unmarshal(typed, &out); err != nil {
			return RedactSensitiveString(string(typed)), nil
		}
		return out, nil
	case []byte:
		var out any
		if err := json.Unmarshal(typed, &out); err == nil {
			return out, nil
		}
		return string(typed), nil
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var out any
		if err := json.Unmarshal(bytes, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func redactNormalized(value any, key string) any {
	if isSensitiveKey(key) {
		return redactedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			out[childKey] = redactNormalized(childValue, childKey)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = redactNormalized(item, key)
		}
		return out
	case string:
		return RedactSensitiveString(typed)
	default:
		return typed
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "_", ""), "-", ""))
	if key == "" {
		return false
	}
	for _, marker := range []string{
		"authorization", "cookie", "setcookie", "token", "secret", "password", "passwd", "pwd",
		"signature", "credential", "clientkey", "privatekey", "apikey", "accesskey",
	} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func redactURL(value string) (string, bool) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	changed := false
	if parsed.User != nil {
		username := parsed.User.Username()
		if username == "" {
			parsed.User = url.User(redactedValue)
		} else {
			parsed.User = url.UserPassword(username, redactedValue)
		}
		changed = true
	}
	query := parsed.Query()
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, redactedValue)
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
		return parsed.String(), true
	}
	return value, true
}
