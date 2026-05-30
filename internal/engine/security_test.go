package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

func TestRecordAuditRedactsSensitivePayload(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	service := NewService(repo)

	err := service.RecordAudit(context.Background(), "site_001", "ops", "SECURITY_TEST", "TEST", "res_1", map[string]any{
		"Authorization": "Bearer audit-token",
		"nested": map[string]any{
			"providerError": "password=audit-secret token=audit-token-2",
		},
	})
	if err != nil {
		t.Fatalf("record audit: %v", err)
	}
	records, err := repo.ListAudit(context.Background(), marketing.AuditQuery{TenantID: "site_001", Action: "SECURITY_TEST"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one audit, got %d", len(records))
	}
	payload := string(records[0].Payload)
	for _, secret := range []string{"audit-token", "audit-secret", "audit-token-2"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("audit payload leaked %q: %s", secret, payload)
		}
	}
}
