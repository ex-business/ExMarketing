package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/dbmigration"
)

func main() {
	var (
		mode         = flag.String("mode", "precheck", "precheck, apply, drift, or status")
		migrationDir = flag.String("dir", "db/migration", "migration SQL directory")
		timeout      = flag.Duration("timeout", 30*time.Second, "database operation timeout")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	db, err := dbmigration.Open(ctx, os.Getenv("EX_MARKETING_DATABASE_URL"))
	if err != nil {
		fail(err)
	}
	defer db.Close()

	manager := dbmigration.Manager{DB: db, MigrationDir: *migrationDir}
	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "precheck":
		plan, err := manager.Precheck(ctx)
		if err != nil {
			fail(err)
		}
		printPlan("PRECHECK PASS", plan)
	case "apply":
		plan, err := manager.Apply(ctx)
		if err != nil {
			fail(err)
		}
		printPlan("APPLY PASS", plan)
	case "drift":
		if err := manager.Drift(ctx); err != nil {
			fail(err)
		}
		plan, err := manager.Precheck(ctx)
		if err != nil {
			fail(err)
		}
		printPlan("DRIFT PASS", plan)
	case "status":
		plan, err := manager.Precheck(ctx)
		if err != nil {
			fail(err)
		}
		printPlan("STATUS", plan)
	default:
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
}

func printPlan(title string, plan dbmigration.Plan) {
	fmt.Println(title)
	fmt.Printf("applied=%d pending=%d\n", len(plan.Applied), len(plan.Pending))
	for _, record := range plan.Applied {
		fmt.Printf("applied %03d %s %s\n", record.Version, record.FileName, record.Status)
	}
	for _, migration := range plan.Pending {
		fmt.Printf("pending %03d %s\n", migration.Version, migration.FileName)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
	os.Exit(1)
}
