package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const AgentStatusFileName = "agent-status.json"

var agentStatusMu sync.Mutex

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
	PID        int        `json:"pid,omitempty"`
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
	status, changed := normalizeAgentStatus(req, status)
	for _, agent := range []Agent{AgentAnalysis, AgentReview, AgentTestPlan, AgentDev, AgentTest, AgentCommit} {
		key := string(agent)
		before := status.Agents[key]
		current := markStaleRunningIfNeeded(before)
		if agentStatusChanged(before, current) {
			changed = true
		}
		status.Agents[key] = current
	}
	if changed {
		if err := SaveAgentStatus(req.TaskDir, status); err != nil {
			return AgentStatusFile{}, err
		}
	}
	return status, nil
}

func loadAgentStatusForMutation(req Requirement) (AgentStatusFile, error) {
	status, err := LoadAgentStatus(req.TaskDir)
	if err != nil {
		return AgentStatusFile{}, err
	}
	status, changed := normalizeAgentStatus(req, status)
	if changed {
		if err := SaveAgentStatus(req.TaskDir, status); err != nil {
			return AgentStatusFile{}, err
		}
	}
	return status, nil
}

func normalizeAgentStatus(req Requirement, status AgentStatusFile) (AgentStatusFile, bool) {
	changed := false
	if status.RequirementID == "" {
		status = NewAgentStatusFile(req)
		changed = true
	}
	if status.Agents == nil {
		status.Agents = map[string]AgentStatus{}
		changed = true
	}
	for _, agent := range []Agent{AgentAnalysis, AgentReview, AgentTestPlan, AgentDev, AgentTest, AgentCommit} {
		key := string(agent)
		current, ok := status.Agents[key]
		if !ok {
			current = AgentStatus{Status: AgentStatusWaiting}
			changed = true
		}
		if current.Status == "" {
			current.Status = AgentStatusWaiting
			changed = true
		}
		if current.Output == "" {
			current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(agent))
			changed = true
		}
		status.Agents[key] = current
	}
	return status, changed
}

func agentStatusChanged(left AgentStatus, right AgentStatus) bool {
	return left.Status != right.Status ||
		left.Log != right.Log ||
		left.Output != right.Output ||
		left.PID != right.PID ||
		left.ExitCode != right.ExitCode ||
		left.NextStatus != right.NextStatus ||
		left.Error != right.Error ||
		!sameTimePtr(left.StartedAt, right.StartedAt) ||
		!sameTimePtr(left.FinishedAt, right.FinishedAt)
}

func sameTimePtr(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

func markStaleRunningIfNeeded(current AgentStatus) AgentStatus {
	if current.Status != AgentStatusRunning || current.Log == "" {
		return current
	}
	bytes, err := os.ReadFile(current.Log)
	if err == nil && hasAgentFinishedMarker(string(bytes)) {
		return current
	}
	if current.PID > 0 && !processExists(current.PID) {
		now := time.Now().UTC()
		current.Status = AgentStatusStale
		current.FinishedAt = &now
		current.Error = fmt.Sprintf("agent interrupted: process %d is not running", current.PID)
		return current
	}
	if current.PID > 0 {
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

func hasAgentFinishedMarker(content string) bool {
	lines := strings.Split(content, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "finishedAt=") &&
			strings.Contains(line, " exitCode=") &&
			strings.Contains(line, " error=")
	}
	return false
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func staleRunningTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("EX_MARKETING_STALE_AGENT_SECONDS"))
	if value == "" {
		return time.Minute
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func MarkAgentRunning(req Requirement, agent Agent, logPath string, pid int) error {
	agentStatusMu.Lock()
	defer agentStatusMu.Unlock()
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
		PID:       pid,
	}
	return SaveAgentStatus(req.TaskDir, status)
}

func MarkAgentDone(req Requirement, run AgentRun) error {
	agentStatusMu.Lock()
	defer agentStatusMu.Unlock()
	status, err := loadAgentStatusForMutation(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(run.Agent)]
	now := time.Now().UTC()
	current.Status = AgentStatusDone
	current.FinishedAt = &now
	current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(run.Agent))
	current.Log = run.OutputPath
	current.PID = 0
	current.ExitCode = run.ExitCode
	current.NextStatus = run.NextStatus
	current.Error = ""
	status.Agents[string(run.Agent)] = current
	return SaveAgentStatus(req.TaskDir, status)
}

func MarkAgentFailed(req Requirement, run AgentRun, runErr error) error {
	agentStatusMu.Lock()
	defer agentStatusMu.Unlock()
	status, err := loadAgentStatusForMutation(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(run.Agent)]
	if current.Status == AgentStatusStale && strings.Contains(current.Error, "stopped by user") {
		current.PID = 0
		current.ExitCode = run.ExitCode
		if current.Log == "" {
			current.Log = run.OutputPath
		}
		status.Agents[string(run.Agent)] = current
		return SaveAgentStatus(req.TaskDir, status)
	}
	now := time.Now().UTC()
	current.Status = AgentStatusFailed
	current.FinishedAt = &now
	current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(run.Agent))
	current.Log = run.OutputPath
	current.PID = 0
	current.ExitCode = run.ExitCode
	if runErr != nil {
		current.Error = runErr.Error()
	}
	status.Agents[string(run.Agent)] = current
	return SaveAgentStatus(req.TaskDir, status)
}

func MarkAgentInterrupted(req Requirement, agent Agent, reason string) error {
	agentStatusMu.Lock()
	defer agentStatusMu.Unlock()
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	current := status.Agents[string(agent)]
	now := time.Now().UTC()
	current.Status = AgentStatusStale
	current.FinishedAt = &now
	current.Output = filepath.Join(req.TaskDir, ArtifactForAgent(agent))
	current.PID = 0
	if reason == "" {
		reason = "agent interrupted"
	}
	current.Error = reason
	status.Agents[string(agent)] = current
	return SaveAgentStatus(req.TaskDir, status)
}

func ResetDevAndTestAfterCommitFailure(req Requirement) error {
	agentStatusMu.Lock()
	defer agentStatusMu.Unlock()
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return err
	}
	for _, agent := range []Agent{AgentDev, AgentTest} {
		status.Agents[string(agent)] = AgentStatus{
			Status: AgentStatusWaiting,
			Output: filepath.Join(req.TaskDir, ArtifactForAgent(agent)),
		}
	}
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
