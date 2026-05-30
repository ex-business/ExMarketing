package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ex-stack/ExMarketing/internal/releasegate"
)

func main() {
	var opts releasegate.Options
	var output string
	flag.StringVar(&opts.RepoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&opts.Metadata.Version, "version", "", "release version or candidate")
	flag.StringVar(&opts.Metadata.Commit, "commit", "", "full commit SHA; defaults to git rev-parse HEAD")
	flag.StringVar(&opts.Metadata.Environment, "environment", "local", "target environment: local, test, staging, production")
	flag.StringVar(&opts.Metadata.ReleaseOwner, "release-owner", "", "release owner")
	flag.StringVar(&opts.Metadata.SecurityOwner, "security-owner", "", "security owner")
	flag.StringVar(&opts.Metadata.BackendOwner, "backend-owner", "", "backend owner")
	flag.StringVar(&opts.Metadata.OpsOwner, "ops-owner", "", "ops/platform owner")
	flag.StringVar(&opts.ConfigEvidencePath, "config-evidence", "", "JSON config baseline evidence path")
	flag.StringVar(&opts.RiskAcceptancePath, "risk-acceptance", "", "JSON risk acceptance path")
	flag.StringVar(&opts.DependencyScanCommand, "dependency-scan-command", "", "dependency vulnerability scan command; empty records WARN/BLOCK placeholder")
	flag.StringVar(&opts.ThreatModelPath, "threat-model", "", "threat model markdown path")
	flag.StringVar(&opts.AlertRulesPath, "alert-rules", "", "alert rules evidence path")
	flag.StringVar(&opts.AlertRunbookPath, "alert-runbook", "", "alert runbook evidence path")
	flag.BoolVar(&opts.RunRequiredCommands, "run-required-commands", false, "execute go test ./... and go build ./...")
	flag.StringVar(&output, "output", "-", "JSON report output path or - for stdout")
	flag.Parse()

	report, err := releasegate.Generate(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "security release gate failed: %v\n", err)
		os.Exit(2)
	}
	if err := releasegate.WriteJSON(output, report); err != nil {
		fmt.Fprintf(os.Stderr, "write report failed: %v\n", err)
		os.Exit(2)
	}
	if report.OverallDecision == releasegate.DecisionBlock {
		os.Exit(1)
	}
	if report.OverallDecision == releasegate.DecisionWarn {
		fmt.Fprintf(os.Stderr, "security release gate completed with WARN at %s\n", time.Now().UTC().Format(time.RFC3339))
	}
}
