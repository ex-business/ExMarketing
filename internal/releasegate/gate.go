package releasegate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Decision string

const (
	DecisionPass  Decision = "PASS"
	DecisionWarn  Decision = "WARN"
	DecisionBlock Decision = "BLOCK"

	StatusPass  = "PASS"
	StatusWarn  = "WARN"
	StatusBlock = "BLOCK"
	StatusSkip  = "SKIP"
)

type Metadata struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Environment   string `json:"environment"`
	ReleaseOwner  string `json:"releaseOwner"`
	SecurityOwner string `json:"securityOwner"`
	BackendOwner  string `json:"backendOwner"`
	OpsOwner      string `json:"opsOwner"`
}

type Options struct {
	RepoRoot              string
	Metadata              Metadata
	RunRequiredCommands   bool
	DependencyScanCommand string
	ConfigEvidencePath    string
	RiskAcceptancePath    string
	ThreatModelPath       string
	AlertRulesPath        string
	AlertRunbookPath      string
	Now                   time.Time
}

type Report struct {
	ReportID          string             `json:"reportId"`
	GeneratedAt       string             `json:"generatedAt"`
	Version           string             `json:"version"`
	Commit            string             `json:"commit"`
	Environment       string             `json:"environment"`
	ReleaseOwner      string             `json:"releaseOwner"`
	SecurityOwner     string             `json:"securityOwner"`
	BackendOwner      string             `json:"backendOwner"`
	OpsOwner          string             `json:"opsOwner"`
	OverallDecision   Decision           `json:"overallDecision"`
	Summary           Summary            `json:"summary"`
	Checks            []CheckResult      `json:"checks"`
	ThreatModelItems  []ThreatModelItem  `json:"threatModelItems"`
	RiskAcceptances   []RiskAcceptance   `json:"riskAcceptances"`
	OpenRisks         []OpenRisk         `json:"openRisks"`
	Artifacts         []Artifact         `json:"artifacts"`
	Checklist         []ChecklistItem    `json:"checklist"`
	SensitiveFindings []SensitiveFinding `json:"sensitiveFindings,omitempty"`
}

type Summary struct {
	ChecksTotal   int `json:"checksTotal"`
	Passed        int `json:"passed"`
	Warned        int `json:"warned"`
	Blocked       int `json:"blocked"`
	Skipped       int `json:"skipped"`
	AcceptedRisks int `json:"acceptedRisks"`
	OpenRisks     int `json:"openRisks"`
}

type CheckResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	Status     string `json:"status"`
	Blocking   bool   `json:"blocking"`
	Evidence   string `json:"evidence"`
	Command    string `json:"command,omitempty"`
	ExitCode   int    `json:"exitCode"`
	Summary    string `json:"summary"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	Owner      string `json:"owner"`
}

type ThreatModelItem struct {
	RiskID   string `json:"riskId"`
	Severity string `json:"severity"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
	Owner    string `json:"owner"`
}

type RiskAcceptance struct {
	RiskID                 string `json:"riskId"`
	Severity               string `json:"severity"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	AcceptedBy             string `json:"acceptedBy"`
	Owner                  string `json:"owner"`
	Reason                 string `json:"reason"`
	CompensatingControls   string `json:"compensatingControls"`
	ExpiresAt              string `json:"expiresAt"`
	FollowUpAction         string `json:"followUpAction"`
	Evidence               string `json:"evidence"`
	ReleaseOwnerConfirmed  bool   `json:"releaseOwnerConfirmed,omitempty"`
	SecurityOwnerConfirmed bool   `json:"securityOwnerConfirmed,omitempty"`
	Valid                  bool   `json:"valid"`
	InvalidReason          string `json:"invalidReason,omitempty"`
}

type OpenRisk struct {
	RiskID         string `json:"riskId"`
	Severity       string `json:"severity"`
	Reason         string `json:"reason"`
	RequiredAction string `json:"requiredAction"`
	Owner          string `json:"owner"`
	DueDate        string `json:"dueDate"`
}

type Artifact struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ChecklistItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Severity string `json:"severity"`
}

type SensitiveFinding struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Pattern string `json:"pattern"`
	Summary string `json:"summary"`
}

type ConfigEvidence struct {
	OpenAPIAuthEnabled      bool              `json:"openApiAuthEnabled"`
	TokenInventoryEvidence  string            `json:"tokenInventoryEvidence"`
	AdminProtected          bool              `json:"adminProtected"`
	MetricsProtected        bool              `json:"metricsProtected"`
	FactAllowlistNonEmpty   bool              `json:"factAllowlistNonEmpty"`
	CallbackSecretsEvidence string            `json:"callbackSecretsEvidence"`
	WebhookSecretsEvidence  string            `json:"webhookSecretsEvidence"`
	MQDestinationAllowlist  bool              `json:"mqDestinationAllowlist"`
	PostgreSQLPersistence   bool              `json:"postgreSQLPersistence"`
	NonMemoryRepository     bool              `json:"nonMemoryRepository"`
	SecretsOutsideBizConfig bool              `json:"secretsOutsideBizConfig"`
	ThreatModelStatuses     map[string]string `json:"threatModelStatuses"`
	ThreatModelEvidence     map[string]string `json:"threatModelEvidence"`
	ThreatModelOwners       map[string]string `json:"threatModelOwners"`
	ManualConfirmation      bool              `json:"manualConfirmation"`
	ReleaseWindowEvidence   string            `json:"releaseWindowEvidence"`
}

func Generate(ctx context.Context, opts Options) (Report, error) {
	root := strings.TrimSpace(opts.RepoRoot)
	if root == "" {
		root = "."
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if opts.ThreatModelPath == "" {
		opts.ThreatModelPath = filepath.Join(root, "docs", "security-threat-model.md")
	}
	if opts.AlertRulesPath == "" {
		opts.AlertRulesPath = filepath.Join(root, "monitoring", "alerts-exmarketing.rules.yml")
	}
	if opts.AlertRunbookPath == "" {
		opts.AlertRunbookPath = filepath.Join(root, "docs", "runbooks", "exmarketing-alerts.md")
	}

	cfg, cfgLoaded, cfgErr := loadConfigEvidence(opts.ConfigEvidencePath)
	acceptances, acceptanceErr := loadRiskAcceptances(opts.RiskAcceptancePath)
	for i := range acceptances {
		validateAcceptance(&acceptances[i], now)
	}

	meta := opts.Metadata
	if strings.TrimSpace(meta.Environment) == "" {
		meta.Environment = "local"
	}
	if strings.TrimSpace(meta.Commit) == "" {
		meta.Commit = gitCommit(ctx, root)
	}

	report := Report{
		ReportID:        "security-release-gate-" + now.UTC().Format("20060102T150405Z"),
		GeneratedAt:     now.UTC().Format(time.RFC3339),
		Version:         meta.Version,
		Commit:          meta.Commit,
		Environment:     meta.Environment,
		ReleaseOwner:    meta.ReleaseOwner,
		SecurityOwner:   meta.SecurityOwner,
		BackendOwner:    meta.BackendOwner,
		OpsOwner:        meta.OpsOwner,
		RiskAcceptances: acceptances,
		Checklist:       defaultChecklist(),
		Artifacts: []Artifact{
			{Name: "security threat model", Path: rel(root, opts.ThreatModelPath)},
			{Name: "alert rules", Path: rel(root, opts.AlertRulesPath)},
			{Name: "alert runbook", Path: rel(root, opts.AlertRunbookPath)},
		},
	}

	report.Checks = append(report.Checks, metadataCheck(meta, now))
	if acceptanceErr != nil {
		report.Checks = append(report.Checks, simpleCheck("risk_acceptance_load", "Risk acceptance load", "risk", "P1", StatusBlock, true, acceptanceErr.Error(), now, "security"))
	}
	report.Checks = append(report.Checks, requiredCommandCheck(ctx, root, "go_test", "Go test", "go test ./...", opts.RunRequiredCommands, now)...)
	report.Checks = append(report.Checks, requiredCommandCheck(ctx, root, "go_build", "Go build", "go build ./...", opts.RunRequiredCommands, now)...)
	secretCheck, findings := secretScan(root, now)
	report.Checks = append(report.Checks, secretCheck)
	report.SensitiveFindings = findings
	report.Checks = append(report.Checks, dependencyScan(ctx, root, opts.DependencyScanCommand, isProduction(meta.Environment), now))
	report.Checks = append(report.Checks, configChecks(cfg, cfgLoaded, cfgErr, isProduction(meta.Environment), now)...)
	report.Checks = append(report.Checks, alertEvidenceChecks(opts.AlertRulesPath, opts.AlertRunbookPath, isProduction(meta.Environment), now)...)

	threatItems, threatChecks := threatModelChecks(opts.ThreatModelPath, cfg, cfgLoaded, acceptances, isProduction(meta.Environment), now)
	report.ThreatModelItems = threatItems
	report.Checks = append(report.Checks, threatChecks...)
	applyRiskAcceptances(report.Checks, acceptances)
	report.OpenRisks = openRisks(report.Checks, threatItems, acceptances)

	report.Summary = summarize(report.Checks, report.RiskAcceptances, report.OpenRisks)
	report.OverallDecision = decide(report.Summary)
	return report, nil
}

func WriteJSON(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func defaultChecklist() []ChecklistItem {
	return []ChecklistItem{
		{ID: "auth", Title: "OpenAPI auth enabled with tenant/source scoped tokens", Category: "authentication", Severity: "P0"},
		{ID: "signature", Title: "Webhook and reward callback timestamp + HMAC verification", Category: "signature", Severity: "P0"},
		{ID: "tenant_guard", Title: "Tenant guard on Event, Callback, Admin, Audit, Export and DLQ paths", Category: "tenant", Severity: "P0"},
		{ID: "secret", Title: "Secrets stay in runtime secret references and never in business payloads", Category: "secret", Severity: "P0"},
		{ID: "redaction", Title: "Logs, audit, metrics and incident output are redacted", Category: "privacy", Severity: "P1"},
		{ID: "dlq", Title: "DLQ replay/close requires DLQ state, operator, reason and business confirmation", Category: "operations", Severity: "P1"},
		{ID: "export_backup", Title: "Exports and backups record checksum, operator, retention and restore guard", Category: "data", Severity: "P1"},
		{ID: "metrics_privacy", Title: "Metrics labels avoid tokens, secrets, payloads and high-cardinality identifiers", Category: "metrics", Severity: "P1"},
		{ID: "threat_model", Title: "Threat model P0/P1 items have fixed, accepted or not-applicable status", Category: "threat-model", Severity: "P0"},
		{ID: "alerting", Title: "P0/P1 alert rules and runbooks exist for release evidence", Category: "alerting", Severity: "P1"},
	}
}

func metadataCheck(meta Metadata, now time.Time) CheckResult {
	var missing []string
	if strings.TrimSpace(meta.Version) == "" {
		missing = append(missing, "version")
	}
	if strings.TrimSpace(meta.Commit) == "" {
		missing = append(missing, "commit")
	}
	if strings.TrimSpace(meta.Environment) == "" {
		missing = append(missing, "environment")
	}
	for name, value := range map[string]string{
		"releaseOwner":  meta.ReleaseOwner,
		"securityOwner": meta.SecurityOwner,
		"backendOwner":  meta.BackendOwner,
		"opsOwner":      meta.OpsOwner,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return simpleCheck("release_metadata", "Release metadata", "metadata", "P1", StatusBlock, true, "missing: "+strings.Join(missing, ", "), now, "release")
	}
	return simpleCheck("release_metadata", "Release metadata", "metadata", "P1", StatusPass, false, "version, commit, environment and owners are present", now, "release")
}

func requiredCommandCheck(ctx context.Context, root, id, name, command string, run bool, now time.Time) []CheckResult {
	if !run {
		return []CheckResult{simpleCheck(id, name, "required-command", "P0", StatusWarn, false, "not executed in this run; execute with --run-required-commands for release evidence", now, "backend")}
	}
	return []CheckResult{runShellCheck(ctx, root, id, name, "required-command", "P0", command, true, now, "backend")}
}

func dependencyScan(ctx context.Context, root, command string, production bool, now time.Time) CheckResult {
	command = strings.TrimSpace(command)
	if command == "" {
		status := StatusWarn
		blocking := false
		summary := "dependency vulnerability scanner is not configured; report as WARN placeholder"
		if production {
			status = StatusBlock
			blocking = true
			summary = "production release requires dependency vulnerability evidence or a valid risk acceptance"
		}
		return simpleCheck("dependency_vulnerability_scan", "Dependency vulnerability scan", "dependency", "P1", status, blocking, summary, now, "security")
	}
	return runShellCheck(ctx, root, "dependency_vulnerability_scan", "Dependency vulnerability scan", "dependency", "P1", command, production, now, "security")
}

func configChecks(cfg ConfigEvidence, loaded bool, loadErr error, production bool, now time.Time) []CheckResult {
	if loadErr != nil {
		return []CheckResult{simpleCheck("config_evidence_load", "Config evidence load", "config", "P1", StatusBlock, true, loadErr.Error(), now, "ops")}
	}
	if !loaded {
		status := StatusWarn
		blocking := false
		summary := "config evidence file not supplied"
		if production {
			status = StatusBlock
			blocking = true
			summary = "production release requires config baseline evidence"
		}
		return []CheckResult{simpleCheck("config_baseline", "Production config baseline", "config", "P0", status, blocking, summary, now, "ops")}
	}
	items := []struct {
		id       string
		name     string
		severity string
		ok       bool
		evidence string
	}{
		{"openapi_auth", "OpenAPI auth enabled", "P0", cfg.OpenAPIAuthEnabled, cfg.TokenInventoryEvidence},
		{"admin_boundary", "Admin protected by internal network or authenticated gateway", "P0", cfg.AdminProtected, ""},
		{"metrics_boundary", "Metrics protected by private scrape path or gateway", "P1", cfg.MetricsProtected, ""},
		{"fact_allowlist", "Fact provider allowlist is non-empty", "P1", cfg.FactAllowlistNonEmpty, ""},
		{"callback_secret", "Reward callback secret evidence exists", "P0", strings.TrimSpace(cfg.CallbackSecretsEvidence) != "", cfg.CallbackSecretsEvidence},
		{"webhook_secret", "Webhook secret evidence exists", "P1", strings.TrimSpace(cfg.WebhookSecretsEvidence) != "", cfg.WebhookSecretsEvidence},
		{"mq_allowlist", "MQ destination allowlist exists", "P1", cfg.MQDestinationAllowlist, ""},
		{"postgres_persistence", "PostgreSQL persistence is configured", "P0", cfg.PostgreSQLPersistence, ""},
		{"non_memory_repository", "Non-memory repository is configured", "P0", cfg.NonMemoryRepository, ""},
		{"secret_config_boundary", "Secrets are outside business config", "P0", cfg.SecretsOutsideBizConfig, ""},
	}
	out := make([]CheckResult, 0, len(items)+1)
	for _, item := range items {
		if item.ok {
			summary := "evidence present"
			if item.evidence != "" {
				summary = "evidence: " + item.evidence
			}
			out = append(out, simpleCheck(item.id, item.name, "config", item.severity, StatusPass, false, summary, now, "ops"))
			continue
		}
		status := StatusWarn
		blocking := false
		if production || item.severity == "P0" {
			status = StatusBlock
			blocking = true
		}
		out = append(out, simpleCheck(item.id, item.name, "config", item.severity, status, blocking, "missing or false config evidence", now, "ops"))
	}
	if cfg.ManualConfirmation {
		out = append(out, simpleCheck("manual_confirmation", "Release and security manual confirmation", "process", "P1", StatusPass, false, "manual confirmation recorded", now, "release"))
	} else {
		status := StatusWarn
		blocking := false
		if production {
			status = StatusBlock
			blocking = true
		}
		out = append(out, simpleCheck("manual_confirmation", "Release and security manual confirmation", "process", "P1", status, blocking, "manual confirmation missing", now, "release"))
	}
	return out
}

func alertEvidenceChecks(rulesPath, runbookPath string, production bool, now time.Time) []CheckResult {
	out := []CheckResult{}
	for _, item := range []struct {
		id   string
		name string
		path string
	}{
		{"alert_rules", "P0/P1 alert rules file", rulesPath},
		{"alert_runbook", "P0/P1 alert runbook", runbookPath},
	} {
		if _, err := os.Stat(item.path); err == nil {
			out = append(out, simpleCheck(item.id, item.name, "alerting", "P1", StatusPass, false, rel(".", item.path), now, "ops"))
			continue
		}
		status := StatusWarn
		blocking := false
		if production {
			status = StatusBlock
			blocking = true
		}
		out = append(out, simpleCheck(item.id, item.name, "alerting", "P1", status, blocking, "missing: "+item.path, now, "ops"))
	}
	return out
}

func threatModelChecks(path string, cfg ConfigEvidence, cfgLoaded bool, acceptances []RiskAcceptance, production bool, now time.Time) ([]ThreatModelItem, []CheckResult) {
	ids, err := parseThreatModelRisks(path)
	if err != nil {
		return nil, []CheckResult{simpleCheck("threat_model_load", "Threat model load", "threat-model", "P0", StatusBlock, true, err.Error(), now, "security")}
	}
	accepted := map[string]RiskAcceptance{}
	for _, a := range acceptances {
		if a.Valid {
			accepted[a.RiskID] = a
		}
	}
	items := make([]ThreatModelItem, 0, len(ids))
	checks := []CheckResult{simpleCheck("threat_model_load", "Threat model load", "threat-model", "P0", StatusPass, false, fmt.Sprintf("%d P0/P1 items parsed", len(ids)), now, "security")}
	for _, item := range ids {
		status := strings.TrimSpace(cfg.ThreatModelStatuses[item.RiskID])
		evidence := strings.TrimSpace(cfg.ThreatModelEvidence[item.RiskID])
		owner := strings.TrimSpace(cfg.ThreatModelOwners[item.RiskID])
		if _, ok := accepted[item.RiskID]; ok {
			status = "accepted"
			if evidence == "" {
				evidence = "valid risk acceptance"
			}
		}
		if status == "" && !cfgLoaded {
			status = "missing"
		}
		reportItem := ThreatModelItem{RiskID: item.RiskID, Severity: item.Severity, Status: status, Evidence: evidence, Owner: owner}
		items = append(items, reportItem)
		if isClosedThreatStatus(status) && evidence != "" {
			continue
		}
		checkStatus := StatusWarn
		blocking := false
		if production || item.Severity == "P0" || item.Severity == "P1" {
			checkStatus = StatusBlock
			blocking = true
		}
		checks = append(checks, simpleCheck("threat_model_"+sanitizeID(item.RiskID), "Threat model item "+item.RiskID, "threat-model", item.Severity, checkStatus, blocking, "missing fixed/accepted/not-applicable status with evidence", now, "security"))
	}
	return items, checks
}

func isClosedThreatStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fixed", "accepted", "not-applicable", "not_applicable":
		return true
	default:
		return false
	}
}

func parseThreatModelRisks(path string) ([]ThreatModelItem, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var out []ThreatModelItem
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	re := regexp.MustCompile(`\|\s*((?:TM|SEC)-[A-Za-z0-9-]+)\s*\|.*\|\s*(P[01])\s*\|`)
	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if len(m) != 3 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, ThreatModelItem{RiskID: m[1], Severity: m[2]})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RiskID < out[j].RiskID })
	if len(out) == 0 {
		return nil, errors.New("no P0/P1 threat model items parsed")
	}
	return out, nil
}

func secretScan(root string, now time.Time) (CheckResult, []SensitiveFinding) {
	patterns := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"private_key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
		{"long_secret_assignment", regexp.MustCompile(`(?i)\b[a-z0-9_-]*(secret|password|token|authorization|api[_-]?key)[a-z0-9_-]*\b\s*[:=]\s*["']?[A-Za-z0-9_./+=:-]{24,}`)},
		{"dsn_password", regexp.MustCompile(`(?i)\b(postgres|mysql)://[^/\s:@]+:[^@\s]{8,}@`)},
	}
	var findings []SensitiveFinding
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", "node_modules", "vendor", "dist", "build", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		if !isTextCandidate(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		lines := bytes.Split(data, []byte{'\n'})
		for i, line := range lines {
			for _, p := range patterns {
				if p.re.Match(line) {
					findings = append(findings, SensitiveFinding{
						Path:    rel(root, path),
						Line:    i + 1,
						Pattern: p.name,
						Summary: "potential sensitive literal detected; value redacted",
					})
				}
			}
		}
		return nil
	})
	if len(findings) > 0 {
		return simpleCheck("secret_scan", "Repository secret scan", "secret", "P0", StatusBlock, true, fmt.Sprintf("%d potential sensitive literals found", len(findings)), now, "security"), findings
	}
	return simpleCheck("secret_scan", "Repository secret scan", "secret", "P0", StatusPass, false, "no high-confidence sensitive literals found", now, "security"), nil
}

func isTextCandidate(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".json", ".yml", ".yaml", ".toml", ".sql", ".txt", ".html", ".js", ".ts", ".css", ".sh":
		return true
	default:
		return false
	}
}

func loadConfigEvidence(path string) (ConfigEvidence, bool, error) {
	if strings.TrimSpace(path) == "" {
		return ConfigEvidence{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigEvidence{}, false, err
	}
	var cfg ConfigEvidence
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ConfigEvidence{}, false, err
	}
	if cfg.ThreatModelStatuses == nil {
		cfg.ThreatModelStatuses = map[string]string{}
	}
	if cfg.ThreatModelEvidence == nil {
		cfg.ThreatModelEvidence = map[string]string{}
	}
	if cfg.ThreatModelOwners == nil {
		cfg.ThreatModelOwners = map[string]string{}
	}
	return cfg, true, nil
}

func loadRiskAcceptances(path string) ([]RiskAcceptance, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []RiskAcceptance
	if err := json.Unmarshal(data, &out); err != nil {
		var wrapper struct {
			RiskAcceptances []RiskAcceptance `json:"riskAcceptances"`
		}
		if wrapperErr := json.Unmarshal(data, &wrapper); wrapperErr != nil {
			return nil, err
		}
		out = wrapper.RiskAcceptances
	}
	return out, nil
}

func validateAcceptance(a *RiskAcceptance, now time.Time) {
	missing := []string{}
	for name, value := range map[string]string{
		"riskId":               a.RiskID,
		"severity":             a.Severity,
		"acceptedBy":           a.AcceptedBy,
		"owner":                a.Owner,
		"reason":               a.Reason,
		"compensatingControls": a.CompensatingControls,
		"expiresAt":            a.ExpiresAt,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		a.Valid = false
		a.InvalidReason = "missing: " + strings.Join(missing, ", ")
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, a.ExpiresAt)
	if err != nil {
		expiresAt, err = time.Parse("2006-01-02", a.ExpiresAt)
	}
	if err != nil {
		a.Valid = false
		a.InvalidReason = "expiresAt must be RFC3339 or YYYY-MM-DD"
		return
	}
	if !expiresAt.After(now) {
		a.Valid = false
		a.InvalidReason = "expiresAt is not in the future"
		return
	}
	if strings.EqualFold(a.Severity, "P0") && (!a.ReleaseOwnerConfirmed || !a.SecurityOwnerConfirmed) {
		a.Valid = false
		a.InvalidReason = "P0 acceptance requires releaseOwnerConfirmed and securityOwnerConfirmed"
		return
	}
	a.Valid = true
}

func openRisks(checks []CheckResult, threatItems []ThreatModelItem, acceptances []RiskAcceptance) []OpenRisk {
	var out []OpenRisk
	for _, c := range checks {
		if c.Status == StatusBlock {
			out = append(out, OpenRisk{RiskID: c.ID, Severity: c.Severity, Reason: c.Summary, RequiredAction: "fix blocker or add valid risk acceptance where allowed", Owner: c.Owner})
		}
	}
	for _, item := range threatItems {
		if !isClosedThreatStatus(item.Status) {
			out = append(out, OpenRisk{RiskID: item.RiskID, Severity: item.Severity, Reason: "threat model item is not closed", RequiredAction: "set fixed, accepted or not-applicable with evidence", Owner: item.Owner})
		}
	}
	for _, a := range acceptances {
		if !a.Valid {
			out = append(out, OpenRisk{RiskID: a.RiskID, Severity: a.Severity, Reason: "invalid risk acceptance: " + a.InvalidReason, RequiredAction: "complete owner, acceptedBy, reason, controls and future expiresAt", Owner: a.Owner, DueDate: a.ExpiresAt})
		}
	}
	return out
}

func applyRiskAcceptances(checks []CheckResult, acceptances []RiskAcceptance) {
	valid := map[string]RiskAcceptance{}
	for _, a := range acceptances {
		if a.Valid {
			valid[a.RiskID] = a
		}
	}
	for i := range checks {
		if checks[i].Status != StatusBlock || !riskAcceptableCheck(checks[i]) {
			continue
		}
		if a, ok := valid[checks[i].ID]; ok {
			checks[i].Status = StatusWarn
			checks[i].Blocking = false
			checks[i].Summary = checks[i].Summary + "; accepted until " + a.ExpiresAt
			checks[i].Evidence = checks[i].Summary
		}
	}
}

func riskAcceptableCheck(check CheckResult) bool {
	switch check.ID {
	case "go_test", "go_build", "release_metadata", "secret_scan", "risk_acceptance_load", "threat_model_load":
		return false
	}
	return check.Category == "dependency" || check.Category == "config" || check.Category == "alerting" || check.Category == "threat-model"
}

func summarize(checks []CheckResult, acceptances []RiskAcceptance, risks []OpenRisk) Summary {
	s := Summary{ChecksTotal: len(checks), OpenRisks: len(risks)}
	for _, c := range checks {
		switch c.Status {
		case StatusPass:
			s.Passed++
		case StatusWarn:
			s.Warned++
		case StatusBlock:
			s.Blocked++
		case StatusSkip:
			s.Skipped++
		}
	}
	for _, a := range acceptances {
		if a.Valid {
			s.AcceptedRisks++
		}
	}
	return s
}

func decide(s Summary) Decision {
	if s.Blocked > 0 || s.OpenRisks > 0 {
		return DecisionBlock
	}
	if s.Warned > 0 || s.AcceptedRisks > 0 {
		return DecisionWarn
	}
	return DecisionPass
}

func runShellCheck(ctx context.Context, root, id, name, category, severity, command string, blocking bool, now time.Time, owner string) CheckResult {
	start := time.Now().UTC()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	finish := time.Now().UTC()
	exitCode := 0
	status := StatusPass
	summary := "command passed"
	if err != nil {
		status = StatusBlock
		if !blocking {
			status = StatusWarn
		}
		summary = sanitizeCommandOutput(out)
		if summary == "" {
			summary = err.Error()
		}
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	check := simpleCheck(id, name, category, severity, status, blocking && status == StatusBlock, summary, now, owner)
	check.Command = command
	check.ExitCode = exitCode
	check.StartedAt = start.Format(time.RFC3339)
	check.FinishedAt = finish.Format(time.RFC3339)
	return check
}

func sanitizeCommandOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	text = regexp.MustCompile(`(?i)(authorization|token|secret|password|cookie)=?[^\s,;]+`).ReplaceAllString(text, "$1=<redacted>")
	if len(text) > 500 {
		text = text[:500] + "..."
	}
	return text
}

func simpleCheck(id, name, category, severity, status string, blocking bool, summary string, now time.Time, owner string) CheckResult {
	t := now.UTC().Format(time.RFC3339)
	return CheckResult{
		ID:         id,
		Name:       name,
		Category:   category,
		Severity:   severity,
		Status:     status,
		Blocking:   blocking,
		Evidence:   summary,
		ExitCode:   0,
		Summary:    summary,
		StartedAt:  t,
		FinishedAt: t,
		Owner:      owner,
	}
}

func gitCommit(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isProduction(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func rel(root, path string) string {
	absRoot, err1 := filepath.Abs(root)
	absPath, err2 := filepath.Abs(path)
	if err1 == nil && err2 == nil {
		if r, err := filepath.Rel(absRoot, absPath); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.ToSlash(r)
		}
	}
	return filepath.ToSlash(path)
}

func sanitizeID(value string) string {
	value = strings.ToLower(value)
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "_")
	return strings.Trim(value, "_")
}

func StableID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}
