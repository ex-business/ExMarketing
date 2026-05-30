package releasegate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratePassWithCompleteEvidence(t *testing.T) {
	root := gateFixture(t)
	cfgPath := writeConfig(t, root, completeConfig(t, root))

	report, err := Generate(context.Background(), Options{
		RepoRoot:              root,
		Metadata:              completeMetadata(),
		ConfigEvidencePath:    cfgPath,
		DependencyScanCommand: "true",
		RunRequiredCommands:   false,
		ThreatModelPath:       filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:        filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:      filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                   fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.OverallDecision != DecisionWarn {
		t.Fatalf("decision = %s, want WARN because go test/build were not executed", report.OverallDecision)
	}
	if report.Summary.Blocked != 0 {
		t.Fatalf("blocked checks = %d, want 0: %#v", report.Summary.Blocked, report.OpenRisks)
	}
	if len(report.ThreatModelItems) != 2 {
		t.Fatalf("threat model items = %d, want 2", len(report.ThreatModelItems))
	}
}

func TestOpenP1RiskBlocks(t *testing.T) {
	root := gateFixture(t)
	cfg := completeConfig(t, root)
	delete(cfg.ThreatModelStatuses, "TM-02")
	cfgPath := writeConfig(t, root, cfg)

	report, err := Generate(context.Background(), Options{
		RepoRoot:              root,
		Metadata:              completeMetadata(),
		ConfigEvidencePath:    cfgPath,
		DependencyScanCommand: "true",
		ThreatModelPath:       filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:        filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:      filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                   fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.OverallDecision != DecisionBlock {
		t.Fatalf("decision = %s, want BLOCK", report.OverallDecision)
	}
	if report.Summary.OpenRisks == 0 {
		t.Fatal("expected open risks")
	}
}

func TestInvalidRiskAcceptanceBlocks(t *testing.T) {
	root := gateFixture(t)
	cfgPath := writeConfig(t, root, completeConfig(t, root))
	acceptancePath := filepath.Join(root, "risk.json")
	writeJSONFile(t, acceptancePath, []RiskAcceptance{{
		RiskID:   "TM-02",
		Severity: "P1",
		Owner:    "security",
		Reason:   "temporary staging exposure",
	}})

	report, err := Generate(context.Background(), Options{
		RepoRoot:              root,
		Metadata:              completeMetadata(),
		ConfigEvidencePath:    cfgPath,
		RiskAcceptancePath:    acceptancePath,
		DependencyScanCommand: "true",
		ThreatModelPath:       filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:        filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:      filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                   fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.OverallDecision != DecisionBlock {
		t.Fatalf("decision = %s, want BLOCK", report.OverallDecision)
	}
	if report.RiskAcceptances[0].Valid {
		t.Fatal("invalid acceptance was marked valid")
	}
}

func TestDependencyPlaceholderWarnsLocallyAndBlocksProduction(t *testing.T) {
	root := gateFixture(t)
	cfgPath := writeConfig(t, root, completeConfig(t, root))

	local, err := Generate(context.Background(), Options{
		RepoRoot:           root,
		Metadata:           completeMetadata(),
		ConfigEvidencePath: cfgPath,
		ThreatModelPath:    filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:     filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:   filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate(local) error = %v", err)
	}
	if local.OverallDecision != DecisionWarn {
		t.Fatalf("local decision = %s, want WARN", local.OverallDecision)
	}

	meta := completeMetadata()
	meta.Environment = "production"
	prod, err := Generate(context.Background(), Options{
		RepoRoot:           root,
		Metadata:           meta,
		ConfigEvidencePath: cfgPath,
		ThreatModelPath:    filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:     filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:   filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate(prod) error = %v", err)
	}
	if prod.OverallDecision != DecisionBlock {
		t.Fatalf("production decision = %s, want BLOCK", prod.OverallDecision)
	}
}

func TestValidRiskAcceptanceDowngradesAcceptableBlockerToWarn(t *testing.T) {
	root := gateFixture(t)
	cfgPath := writeConfig(t, root, completeConfig(t, root))
	acceptancePath := filepath.Join(root, "risk.json")
	writeJSONFile(t, acceptancePath, []RiskAcceptance{{
		RiskID:               "dependency_vulnerability_scan",
		Severity:             "P1",
		Title:                "Dependency scan tool unavailable",
		Description:          "Scanner is unavailable in the release runner.",
		AcceptedBy:           "security-owner",
		Owner:                "backend-owner",
		Reason:               "No dependency changes in this release candidate.",
		CompensatingControls: "Manual go.sum review and follow-up scanner run.",
		ExpiresAt:            "2026-06-30T00:00:00Z",
		FollowUpAction:       "Install scanner in CI.",
		Evidence:             "change-ticket-123",
	}})
	meta := completeMetadata()
	meta.Environment = "production"

	report, err := Generate(context.Background(), Options{
		RepoRoot:           root,
		Metadata:           meta,
		ConfigEvidencePath: cfgPath,
		RiskAcceptancePath: acceptancePath,
		ThreatModelPath:    filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:     filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:   filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.OverallDecision != DecisionWarn {
		t.Fatalf("decision = %s, want WARN", report.OverallDecision)
	}
	if report.Summary.AcceptedRisks != 1 {
		t.Fatalf("accepted risks = %d, want 1", report.Summary.AcceptedRisks)
	}
}

func TestSecretScanBlocksHighConfidenceLiteral(t *testing.T) {
	root := gateFixture(t)
	if err := os.WriteFile(filepath.Join(root, "leak.go"), []byte(`package leak
const apiToken = "abcdefghijklmnopqrstuvwxyz1234567890"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, root, completeConfig(t, root))

	report, err := Generate(context.Background(), Options{
		RepoRoot:              root,
		Metadata:              completeMetadata(),
		ConfigEvidencePath:    cfgPath,
		DependencyScanCommand: "true",
		ThreatModelPath:       filepath.Join(root, "docs/security-threat-model.md"),
		AlertRulesPath:        filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"),
		AlertRunbookPath:      filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"),
		Now:                   fixedNow(),
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.OverallDecision != DecisionBlock {
		t.Fatalf("decision = %s, want BLOCK", report.OverallDecision)
	}
	if len(report.SensitiveFindings) == 0 {
		t.Fatal("expected sensitive finding")
	}
}

func gateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "docs/runbooks"))
	mustMkdir(t, filepath.Join(root, "monitoring"))
	writeText(t, filepath.Join(root, "docs/security-threat-model.md"), `# Threat Model

| ID | Area | STRIDE | Scenario | Impact | Controls | Gap | Severity | Likelihood | Impact | Action |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| TM-01 | OpenAPI | Spoofing | Forged event | Reward loss | Auth | Need release gate | P0 | Medium | High | Verify auth |
| TM-02 | Metrics | Information Disclosure | Secret label | Secret leak | Redaction | Need scan | P1 | Low | High | Verify labels |
`)
	writeText(t, filepath.Join(root, "monitoring/alerts-exmarketing.rules.yml"), "groups: []\n")
	writeText(t, filepath.Join(root, "docs/runbooks/exmarketing-alerts.md"), "# Runbook\n")
	writeText(t, filepath.Join(root, "README.md"), "# Fixture\n")
	return root
}

func completeMetadata() Metadata {
	return Metadata{
		Version:       "v1.2.3",
		Commit:        "0123456789abcdef0123456789abcdef01234567",
		Environment:   "staging",
		ReleaseOwner:  "release-owner",
		SecurityOwner: "security-owner",
		BackendOwner:  "backend-owner",
		OpsOwner:      "ops-owner",
	}
}

func completeConfig(t *testing.T, root string) ConfigEvidence {
	t.Helper()
	return ConfigEvidence{
		OpenAPIAuthEnabled:      true,
		TokenInventoryEvidence:  "vault/exmarketing/openapi",
		AdminProtected:          true,
		MetricsProtected:        true,
		FactAllowlistNonEmpty:   true,
		CallbackSecretsEvidence: "vault/exmarketing/callback",
		WebhookSecretsEvidence:  "vault/exmarketing/webhook",
		MQDestinationAllowlist:  true,
		PostgreSQLPersistence:   true,
		NonMemoryRepository:     true,
		SecretsOutsideBizConfig: true,
		ManualConfirmation:      true,
		ThreatModelStatuses: map[string]string{
			"TM-01": "fixed",
			"TM-02": "fixed",
		},
		ThreatModelEvidence: map[string]string{
			"TM-01": "security tests",
			"TM-02": "metrics label review",
		},
		ThreatModelOwners: map[string]string{
			"TM-01": "backend",
			"TM-02": "security",
		},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
}

func writeConfig(t *testing.T, root string, cfg ConfigEvidence) string {
	t.Helper()
	path := filepath.Join(root, "config-evidence.json")
	writeJSONFile(t, path, cfg)
	return path
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
