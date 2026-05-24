package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/workflow"
)

const backlogPath = "requirements/backlog.json"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "add":
		err = add(os.Args[2:])
	case "list":
		err = list()
	case "intake":
		err = intake()
	case "run-agent":
		err = runAgent(os.Args[2:])
	case "agent-status":
		err = agentStatus(os.Args[2:])
	case "set-status":
		err = setStatus(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	title := fs.String("title", "", "requirement title")
	description := fs.String("description", "", "requirement description")
	risk := fs.String("risk", "中", "risk level: 低/中/高")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" || strings.TrimSpace(*description) == "" {
		return fmt.Errorf("title and description are required")
	}

	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	id := now.Format("2006-01-02") + "-" + workflow.Slug(*title)
	if id == now.Format("2006-01-02")+"-" {
		id = now.Format("2006-01-02-150405")
	}
	for _, item := range backlog.Items {
		if item.ID == id {
			return fmt.Errorf("requirement %s already exists", id)
		}
	}
	backlog.Items = append(backlog.Items, workflow.Requirement{
		ID:          id,
		Title:       strings.TrimSpace(*title),
		Description: strings.TrimSpace(*description),
		Status:      workflow.StatusNew,
		Risk:        strings.TrimSpace(*risk),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err := workflow.Save(backlogPath, backlog); err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}

func list() error {
	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	for _, item := range backlog.Items {
		fmt.Printf("%s\t%s\t%d\t%s\n", item.ID, item.Status, item.Score, item.Title)
	}
	return nil
}

func intake() error {
	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	changed := false
	for i := range backlog.Items {
		item := &backlog.Items[i]
		if item.Status != workflow.StatusNew {
			continue
		}
		item.TaskDir = workflow.TaskDirFor(item.ID)
		item.Status = workflow.StatusAnalysisReady
		item.UpdatedAt = time.Now().UTC()
		if err := workflow.CreateTaskFiles(*item); err != nil {
			return err
		}
		fmt.Printf("created %s\n", item.TaskDir)
		changed = true
	}
	if changed {
		return workflow.Save(backlogPath, backlog)
	}
	fmt.Println("no NEW requirements")
	return nil
}

func setStatus(args []string) error {
	fs := flag.NewFlagSet("set-status", flag.ExitOnError)
	id := fs.String("id", "", "requirement id")
	status := fs.String("status", "", "new status")
	score := fs.Int("score", -1, "score")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *status == "" {
		return fmt.Errorf("id and status are required")
	}

	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	for i := range backlog.Items {
		if backlog.Items[i].ID != *id {
			continue
		}
		backlog.Items[i].Status = *status
		if *score >= 0 {
			backlog.Items[i].Score = *score
		}
		backlog.Items[i].UpdatedAt = time.Now().UTC()
		return workflow.Save(backlogPath, backlog)
	}
	return fmt.Errorf("requirement %s not found", *id)
}

func runAgent(args []string) error {
	fs := flag.NewFlagSet("run-agent", flag.ExitOnError)
	id := fs.String("id", "", "requirement id")
	agentFlag := fs.String("agent", "auto", "agent: auto|analysis|review|dev|test|commit")
	command := fs.String("cmd", os.Getenv("EX_MARKETING_AGENT_CMD"), "agent command; prompt is passed on stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("id is required")
	}

	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	for i := range backlog.Items {
		item := &backlog.Items[i]
		if item.ID != *id {
			continue
		}
		agent, err := workflow.ResolveAgent(item.Status, workflow.Agent(strings.ToLower(strings.TrimSpace(*agentFlag))))
		if err != nil {
			return err
		}
		run, err := workflow.RunAgent(*item, agent, *command)
		if err != nil {
			return fmt.Errorf("agent %s failed: %w; output=%s", agent, err, run.OutputPath)
		}
		item.Status = run.NextStatus
		if run.Score >= 0 {
			item.Score = run.Score
		}
		item.UpdatedAt = time.Now().UTC()
		if err := workflow.Save(backlogPath, backlog); err != nil {
			return err
		}
		fmt.Printf("%s\t%s\t%s\tscore=%d\toutput=%s\n", item.ID, run.Agent, item.Status, item.Score, run.OutputPath)
		return nil
	}
	return fmt.Errorf("requirement %s not found", *id)
}

func agentStatus(args []string) error {
	fs := flag.NewFlagSet("agent-status", flag.ExitOnError)
	id := fs.String("id", "", "requirement id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("id is required")
	}

	backlog, err := workflow.Load(backlogPath)
	if err != nil {
		return err
	}
	for _, item := range backlog.Items {
		if item.ID != *id {
			continue
		}
		if item.TaskDir == "" {
			return fmt.Errorf("requirement %s has no taskDir; run intake first", *id)
		}
		status, err := workflow.EnsureAgentStatus(item)
		if err != nil {
			return err
		}
		if err := workflow.SaveAgentStatus(item.TaskDir, status); err != nil {
			return err
		}
		fmt.Printf("requirement: %s\n", status.RequirementID)
		fmt.Printf("taskDir: %s\n", item.TaskDir)
		fmt.Printf("workflowStatus: %s\n", item.Status)
		fmt.Printf("updatedAt: %s\n", status.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
		fmt.Printf("%-10s %-8s %-20s %-20s %s\n", "agent", "status", "started", "finished", "output")
		for _, agent := range []workflow.Agent{workflow.AgentAnalysis, workflow.AgentReview, workflow.AgentDev, workflow.AgentTest, workflow.AgentCommit} {
			current := status.Agents[string(agent)]
			started := "-"
			if current.StartedAt != nil {
				started = current.StartedAt.Format(time.RFC3339)
			}
			finished := "-"
			if current.FinishedAt != nil {
				finished = current.FinishedAt.Format(time.RFC3339)
			}
			fmt.Printf("%-10s %-8s %-20s %-20s %s\n", agent, current.Status, started, finished, current.Output)
			if current.Log != "" {
				fmt.Printf("%-10s %-8s %-20s %-20s log: %s\n", "", "", "", "", current.Log)
			}
			if current.Error != "" {
				fmt.Printf("%-10s %-8s %-20s %-20s error: %s\n", "", "", "", "", current.Error)
			}
		}
		return nil
	}
	return fmt.Errorf("requirement %s not found", *id)
}

func usage() {
	fmt.Println(`workflowctl commands:
  add --title <title> --description <description> [--risk 低|中|高]
  list
  intake
  run-agent --id <id> [--agent auto|analysis|review|dev|test|commit] [--cmd <command>]
  agent-status --id <id>
  set-status --id <id> --status <status> [--score n]`)
}
