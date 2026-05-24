package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/workflow"
)

const workflowBacklogPath = "requirements/backlog.json"

type createWorkflowRequirementRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
}

type deleteWorkflowRequirementsRequest struct {
	IDs []string `json:"ids"`
}

type runWorkflowAgentRequest struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
}

type runWorkflowPipelineRequest struct {
	Command  string `json:"command"`
	MaxSteps int    `json:"maxSteps"`
}

type createGitCommitRequest struct {
	RequirementID string `json:"requirementId"`
	Message       string `json:"message"`
}

func (h *Handler) getWorkflowConfig(w http.ResponseWriter, r *http.Request) {
	command := strings.TrimSpace(os.Getenv("EX_MARKETING_AGENT_CMD"))
	writeJSON(w, http.StatusOK, map[string]any{
		"agentCommandConfigured": command != "",
		"agentCommandPreview":    maskCommand(command),
		"analysisPassScore":      workflow.AnalysisPassScore(),
	})
}

func (h *Handler) createWorkflowRequirement(w http.ResponseWriter, r *http.Request) {
	var request createWorkflowRequirementRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	title := strings.TrimSpace(request.Title)
	description := strings.TrimSpace(request.Description)
	if title == "" || description == "" {
		writeError(w, http.StatusBadRequest, "title and description are required")
		return
	}
	risk := strings.TrimSpace(request.Risk)
	if risk == "" {
		risk = "中"
	}

	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	id := now.Format("2006-01-02") + "-" + workflow.Slug(title)
	if id == now.Format("2006-01-02")+"-" {
		id = now.Format("2006-01-02-150405")
	}
	for _, item := range backlog.Items {
		if item.ID == id {
			writeError(w, http.StatusConflict, "requirement already exists")
			return
		}
	}
	req := workflow.Requirement{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      workflow.StatusNew,
		Risk:        risk,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	backlog.Items = append(backlog.Items, req)
	if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

func (h *Handler) deleteWorkflowRequirement(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "requirement id is required")
		return
	}
	deleted, err := deleteRequirementsByID([]string{id})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(deleted) == 0 {
		writeError(w, http.StatusNotFound, "requirement not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (h *Handler) deleteWorkflowRequirements(w http.ResponseWriter, r *http.Request) {
	var request deleteWorkflowRequirementsRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	deleted, err := deleteRequirementsByID(request.IDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func deleteRequirementsByID(ids []string) ([]string, error) {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		return []string{}, nil
	}
	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		return nil, err
	}
	deleted := make([]string, 0, len(wanted))
	kept := make([]workflow.Requirement, 0, len(backlog.Items))
	for _, item := range backlog.Items {
		if wanted[item.ID] {
			deleted = append(deleted, item.ID)
			continue
		}
		kept = append(kept, item)
	}
	backlog.Items = kept
	if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
		return nil, err
	}
	return deleted, nil
}

func (h *Handler) intakeWorkflowRequirements(w http.ResponseWriter, r *http.Request) {
	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	created := make([]workflow.Requirement, 0)
	for i := range backlog.Items {
		item := &backlog.Items[i]
		if item.Status != workflow.StatusNew {
			continue
		}
		item.TaskDir = workflow.TaskDirFor(item.ID)
		item.Status = workflow.StatusAnalysisReady
		item.UpdatedAt = time.Now().UTC()
		if err := workflow.CreateTaskFiles(*item); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		created = append(created, *item)
	}
	if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": created})
}

func (h *Handler) runWorkflowAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var request runWorkflowAgentRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if r.Body != nil && r.ContentLength != 0 {
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	if request.Agent == "" {
		request.Agent = "auto"
	}
	if request.Command == "" {
		request.Command = os.Getenv("EX_MARKETING_AGENT_CMD")
	}

	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range backlog.Items {
		item := &backlog.Items[i]
		if item.ID != id {
			continue
		}
		if item.TaskDir == "" {
			writeError(w, http.StatusBadRequest, "taskDir is empty; run intake first")
			return
		}
		if err := workflow.EnsureTaskFiles(*item); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agent, err := workflow.ResolveAgent(item.Status, workflow.Agent(strings.ToLower(strings.TrimSpace(request.Agent))))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := workflow.RunAgent(*item, agent, request.Command)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("agent %s failed: %v; log=%s", agent, err, run.OutputPath))
			return
		}
		item.Status = run.NextStatus
		if run.Score >= 0 {
			item.Score = run.Score
		}
		item.UpdatedAt = time.Now().UTC()
		if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = workflow.WriteAgentReport(*item, []workflow.AgentRun{run}, item.Status, "单步 Agent 运行完成。")
		writeJSON(w, http.StatusOK, map[string]any{"run": run, "requirement": item})
		return
	}
	writeError(w, http.StatusNotFound, "requirement not found")
}

func (h *Handler) runWorkflowPipeline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var request runWorkflowPipelineRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if r.Body != nil && r.ContentLength != 0 {
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	if request.Command == "" {
		request.Command = os.Getenv("EX_MARKETING_AGENT_CMD")
	}
	if request.MaxSteps <= 0 || request.MaxSteps > 6 {
		request.MaxSteps = 6
	}

	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range backlog.Items {
		item := &backlog.Items[i]
		if item.ID != id {
			continue
		}
		if item.TaskDir == "" {
			writeError(w, http.StatusBadRequest, "taskDir is empty; run intake first")
			return
		}
		if err := workflow.EnsureTaskFiles(*item); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		runs := make([]workflow.AgentRun, 0, request.MaxSteps)
		message := "自动流水线运行完成。"
		for step := 0; step < request.MaxSteps; step++ {
			agent, err := workflow.ResolveAgent(item.Status, workflow.AgentAuto)
			if err != nil {
				message = err.Error()
				break
			}
			run, err := workflow.RunAgent(*item, agent, request.Command)
			if err != nil {
				_ = workflow.WriteAgentReport(*item, runs, item.Status, fmt.Sprintf("自动流水线在 %s 失败: %v", agent, err))
				logContent := ""
				if run.OutputPath != "" {
					if bytes, readErr := os.ReadFile(run.OutputPath); readErr == nil {
						logContent = string(bytes)
					}
				}
				writeJSON(w, http.StatusInternalServerError, map[string]any{
					"error":      fmt.Sprintf("agent %s failed: %v", agent, err),
					"agent":      agent,
					"log":        run.OutputPath,
					"logContent": logContent,
				})
				return
			}
			runs = append(runs, run)
			item.Status = run.NextStatus
			if run.Score >= 0 {
				item.Score = run.Score
			}
			item.UpdatedAt = time.Now().UTC()
			if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if item.Status == workflow.StatusReviewBlock || item.Status == workflow.StatusTestFail || item.Status == workflow.StatusCommitted {
				break
			}
		}
		if err := workflow.WriteAgentReport(*item, runs, item.Status, message); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"runs":        runs,
			"requirement": item,
			"report":      filepath.Join(item.TaskDir, workflow.AgentReportFileName),
			"message":     message,
		})
		return
	}
	writeError(w, http.StatusNotFound, "requirement not found")
}

func (h *Handler) getWorkflowAgentLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent := strings.ToLower(strings.TrimSpace(r.PathValue("agent")))
	req, err := findRequirement(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	status, err := workflow.EnsureAgentStatus(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	current, ok := status.Agents[agent]
	if !ok || current.Log == "" {
		writeJSON(w, http.StatusOK, map[string]string{"content": "", "path": ""})
		return
	}
	content, err := readSafeTaskFile(req.TaskDir, current.Log)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content, "path": current.Log})
}

func (h *Handler) getWorkflowRunLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" || strings.Contains(runID, "/") || strings.Contains(runID, "\\") || strings.Contains(runID, "..") {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	req, err := findRequirement(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	path := filepath.Join(req.TaskDir, "runs", runID+".log")
	content, err := readSafeTaskFile(req.TaskDir, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": content, "path": path, "runId": runID})
}

func (h *Handler) getGitDiff(w http.ResponseWriter, r *http.Request) {
	output, code, err := runLocalCommand(r.Context(), 20*time.Second, "git", "diff", "--stat")
	full, fullCode, fullErr := runLocalCommand(r.Context(), 20*time.Second, "git", "diff")
	writeJSON(w, http.StatusOK, map[string]any{
		"stat":     output,
		"statCode": code,
		"statErr":  errorString(err),
		"diff":     full,
		"diffCode": fullCode,
		"diffErr":  errorString(fullErr),
	})
}

func (h *Handler) createGitCommit(w http.ResponseWriter, r *http.Request) {
	var request createGitCommitRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	request.RequirementID = strings.TrimSpace(request.RequirementID)
	request.Message = strings.TrimSpace(request.Message)
	if request.RequirementID == "" || request.Message == "" {
		writeError(w, http.StatusBadRequest, "requirementId and message are required")
		return
	}
	req, err := findRequirement(request.RequirementID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if req.Status != workflow.StatusCommitReady {
		writeError(w, http.StatusConflict, "requirement must be COMMIT_READY before commit")
		return
	}
	testOutput, testCode, testErr := runLocalCommand(r.Context(), 2*time.Minute, "go", "test", "./...")
	if testErr != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("go test failed: %s", testOutput))
		return
	}
	buildOutput, buildCode, buildErr := runLocalCommand(r.Context(), 2*time.Minute, "go", "build", "./...")
	if buildErr != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("go build failed: %s", buildOutput))
		return
	}
	statusOutput, _, _ := runLocalCommand(r.Context(), 20*time.Second, "git", "status", "-sb")
	if strings.TrimSpace(statusOutput) == "## main...origin/main" || strings.TrimSpace(statusOutput) == "" {
		writeError(w, http.StatusConflict, "no local changes to commit")
		return
	}
	if output, _, err := runLocalCommand(r.Context(), 30*time.Second, "git", "add", "-A"); err != nil {
		writeError(w, http.StatusInternalServerError, output)
		return
	}
	commitOutput, commitCode, commitErr := runLocalCommand(r.Context(), 30*time.Second, "git", "commit", "-m", request.Message)
	if commitErr != nil {
		writeError(w, http.StatusInternalServerError, commitOutput)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"testCode":     testCode,
		"testOutput":   testOutput,
		"buildCode":    buildCode,
		"buildOutput":  buildOutput,
		"commitCode":   commitCode,
		"commitOutput": commitOutput,
	})
}

func findRequirement(id string) (workflow.Requirement, error) {
	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		return workflow.Requirement{}, err
	}
	for _, item := range backlog.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return workflow.Requirement{}, errors.New("requirement not found")
}

func readSafeTaskFile(taskDir, path string) (string, error) {
	cleanTaskDir, err := filepath.Abs(filepath.Clean(taskDir))
	if err != nil {
		return "", err
	}
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		cleanPath, err = filepath.Abs(cleanPath)
		if err != nil {
			return "", err
		}
	}
	if cleanTaskDir == "." || strings.HasPrefix(cleanTaskDir, "..") {
		return "", errors.New("invalid taskDir")
	}
	if !strings.HasPrefix(cleanPath, cleanTaskDir+string(os.PathSeparator)) {
		return "", errors.New("log path is outside taskDir")
	}
	bytes, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func runLocalCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
	}
	if ctx.Err() != nil {
		return string(output), code, ctx.Err()
	}
	return string(output), code, err
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func maskCommand(command string) string {
	if command == "" {
		return ""
	}
	if len(command) <= 120 {
		return command
	}
	return command[:120] + "..."
}
