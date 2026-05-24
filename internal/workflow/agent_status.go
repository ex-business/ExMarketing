package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const AgentStatusFileName = "agent-status.json"

const (
	AgentStatusWaiting = "WAITING"
	AgentStatusRunning = "RUNNING"
	AgentStatusDone    = "DONE"
	AgentStatusFailed  = "FAILED"
	AgentStatusStale   = "INTERRUPTED"
)

type AgentStatusFile struct {
	RequirementID string                 `json:"requirementId"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	Agents        map[string]AgentStatus `json:"agents"`
}

type AgentStatus struct {
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Output     string     `json:"output,omitempty"`
	Log        string     `json:"log,omitempty"`
	ExitCode   int        `json:"exitCode,omitempty"`
	NextStatus string     `json:"nextStatus,omitempty"`
	Error      string     `json:"error,omitempty"`
}

func NewAgentStatusFile(req Requirement) AgentStatusFile {
	now := time.Now().UTC()
	return AgentStatusFile{
		RequirementID: req.ID,
		UpdatedAt:     now,
		Agents: map[string]AgentStatus{
			string(AgentAnalysis): {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "analysis.md")},
			string(AgentReview):   {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "review.md")},
			string(AgentTestPlan): {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "test-plan.md")},
			string(AgentDev):      {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "implementation.md")},
			string(AgentTest):     {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "test.md")},
			string(AgentCommit):   {Status: AgentStatusWaiting, Output: filepath.Join(req.TaskDir, "commit.md")},
		},
	}
}

func LoadAgentStatus(taskDir string) (AgentStatusFile, error) {
	path := filepath.Join(taskDir, AgentStatusFileName)
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AgentStatusFile{}, nil
		}
		return AgentStatusFile{}, err
	}
	if len(bytes) == 0 {
		return AgentStatusFile{}, nil
	}
	var status AgentStatusFile
	if err := json.Unmarshal(bytes, &status); err != nil {
		return AgentStatusFile{}, err
	}
	return status, nil
}

func SaveAgentStatus(taskDir string, status AgentStatusFile) error {
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return err
	}
	status.UpdatedAt = time.Now().UTC()
	bytes, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	return os.WriteFile(filepath.Join(taskDir, AgentStatusFileName), bytes, 0o644)
}

func EnsureAgentStatus(req Requirement) (AgentStatusFile, error) {
	status, err := LoadAgentStatus(req.TaskDir)
	if err != nil {
		return AgentStatusFile{}, err
	}
	if status.RequirementID == "" {
		status = NewAgentStatusFile(req)
	}
	if status.Agents == nil {
		status.Agents = map[string]AgentStatus{}
	}
	for _, agent := range []Agent{AgentAnalysis, AgentReview, AgentTestPlan, AgentDev, AgentTest, AgentCommit} {
		key := string(agent)
		current, ok := status.Agents[key]
		if !ok {
			current = AgentStatus{Status: AgentStatusWaiting}
		}
		if current.Status == "" {
			current.Status = AgentStatusWaiting
		}
		if current.Output == "" {
			current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(agent))
		}
		current = markStaleRunningIfNeeded(current)
		status.Agents[key] = current
	}
	return status, nil
}

func markStaleRunningIfNeeded(current AgentStatus) AgentStatus {
	if current.Status != AgentStatusRunning || current.Log == "" {
		return current
	}
	bytes, err := os.ReadFile(current.Log)
	if err == nil && strings.Contains(string(bytes), "finishedAt=") {
		return current
	}
	info, statErr := os.Stat(current.Log)
	if statErr != nil {
		if current.StartedAt != nil && time.Since(*current.StartedAt) > staleRunningTimeout() {
			now := time.Now().UTC()
			current.Status = AgentStatusStale
			current.FinishedAt = &now
			current.Error = fmt.Sprintf("agent interrupted: log %s is not readable", current.Log)
		}
		return current
	}
	if time.Since(info.ModTime()) <= staleRunningTimeout() {
		return current
	}
	now := time.Now().UTC()
	current.Status = AgentStatusStale
	current.FinishedAt = &now
	current.Error = fmt.Sprintf("agent interrupted: no log update since %s", info.ModTime().UTC().Format(time.RFC3339))
	return current
}

func staleRunningTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("EX_MARKETING_STALE_AGENT_SECONDS"))
	if value == "" {
		return 5 * time.Minute
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func MarkAgentRunning(req Requirement, agent Agent, logPath string) error {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	status.Agents[string(agent)] = AgentStatus{
		Status:    AgentStatusRunning,
		StartedAt: &now,
		Output:    filepath.Join(req.TaskDir, ArtifactForAgent(agent)),
		Log:       logPath,
	}
	return SaveAgentStatus(req.TaskDir, status)
}

func MarkAgentDone(req Requirement, run AgentRun) error {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(run.Agent)]
	now := time.Now().UTC()
	current.Status = AgentStatusDone
	current.FinishedAt = &now
	current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(run.Agent))
	current.Log = run.OutputPath
	current.ExitCode = run.ExitCode
	current.NextStatus = run.NextStatus
	current.Error = ""
	status.Agents[string(run.Agent)] = current
	return SaveAgentStatus(req.TaskDir, status)
}

func MarkAgentFailed(req Requirement, run AgentRun, runErr error) error {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(run.Agent)]
	now := time.Now().UTC()
	current.Status = AgentStatusFailed
	current.FinishedAt = &now
	current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(run.Agent))
	current.Log = run.OutputPath
	current.ExitCode = run.ExitCode
	if runErr != nil {
		current.Error = runErr.Error()
	}
	status.Agents[string(run.Agent)] = current
	return SaveAgentStatus(req.TaskDir, status)
}

func ArtifactForAgent(agent Agent) string {
	switch agent {
	case AgentAnalysis:
		return "analysis.md"
	case AgentReview:
		return "review.md"
	case AgentTestPlan:
		return "test-plan.md"
	case AgentDev:
		return "implementation.md"
	case AgentTest:
		return "test.md"
	case AgentCommit:
		return "commit.md"
	default:
		return ""
	}
}
