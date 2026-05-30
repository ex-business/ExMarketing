package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteErrorCodeRedactsSensitiveMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErrorCode(rec, http.StatusBadRequest, "BAD_REQUEST", "Authorization: Bearer raw-token password=secret")

	body := rec.Body.String()
	for _, secret := range []string{"raw-token", "password=secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("error response leaked %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "[redacted]") {
		t.Fatalf("expected redacted marker in %s", body)
	}
}
