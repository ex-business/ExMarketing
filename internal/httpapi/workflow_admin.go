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
	"sync"
	"time"

	"github.com/ex-stack/ExMarketing/internal/workflow"
)

const workflowBacklogPath = "requirements/backlog.json"

type createWorkflowRequirementRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
}

type applyWorkflowTemplateRequest struct {
	Intake bool `json:"intake"`
}

type deleteWorkflowRequirementsRequest struct {
	IDs []string `json:"ids"`
}

type runWorkflowAgentRequest struct {
	Agent   string `json:"agent"`
	Command string `json:"command"`
}

type runWorkflowPipelineRequest struct {
	Command         string `json:"command"`
	MaxSteps        int    `json:"maxSteps"`
	ContinueQueue   bool   `json:"continueQueue"`
	MaxRequirements int    `json:"maxRequirements"`
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

func (h *Handler) listWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": workflow.BuiltinTemplates()})
}

func (h *Handler) applyWorkflowTemplate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var request applyWorkflowTemplateRequest
	if r.Body != nil && r.ContentLength != 0 {
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	var selected workflow.WorkflowTemplate
	found := false
	for _, template := range workflow.BuiltinTemplates() {
		if template.ID == id {
			selected = template
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "workflow template not found")
		return
	}

	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	existsByTitle := make(map[string]bool, len(backlog.Items))
	for _, item := range backlog.Items {
		existsByTitle[item.Title] = true
	}
	now := time.Now().UTC()
	created := make([]workflow.Requirement, 0, len(selected.Items))
	skipped := make([]workflow.RequirementTemplate, 0)
	for _, item := range selected.Items {
		if existsByTitle[item.Title] {
			skipped = append(skipped, item)
			continue
		}
		req := workflow.Requirement{
			ID:          uniqueRequirementID(backlog, now, item.Title),
			Title:       item.Title,
			Description: item.Description,
			Status:      workflow.StatusNew,
			Risk:        item.Risk,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if request.Intake {
			req.TaskDir = workflow.TaskDirFor(req.ID)
			req.Status = workflow.StatusAnalysisReady
			if err := workflow.CreateTaskFiles(req); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		backlog.Items = append(backlog.Items, req)
		existsByTitle[item.Title] = true
		created = append(created, req)
	}
	if err := workflow.Save(workflowBacklogPath, backlog); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"template": selected,
		"created":  created,
		"skipped":  skipped,
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
	id := requirementID(now, title)
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

func requirementID(now time.Time, title string) string {
	slug := workflow.Slug(title)
	if slug == "" {
		slug = now.Format("150405")
	}
	return now.Format("2006-01-02") + "-" + slug
}

func uniqueRequirementID(backlog workflow.Backlog, now time.Time, title string) string {
	base := requirementID(now, title)
	exists := make(map[string]bool, len(backlog.Items))
	for _, item := range backlog.Items {
		exists[item.ID] = true
	}
	if !exists[base] {
		return base
	}
	for seq := 2; ; seq++ {
		id := fmt.Sprintf("%s-%d", base, seq)
		if !exists[id] {
			return id
		}
	}
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
		reconcileCommitFailure(item)
		agent, err := workflow.ResolveAgent(item.Status, workflow.Agent(strings.ToLower(strings.TrimSpace(request.Agent))))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		run, err := workflow.RunAgent(*item, agent, request.Command)
		if err != nil {
			if agent == workflow.AgentCommit {
				item.Status = workflow.StatusCommitFail
				item.UpdatedAt = time.Now().UTC()
				if resetErr := workflow.ResetDevAndTestAfterCommitFailure(*item); resetErr != nil {
					writeError(w, http.StatusInternalServerError, resetErr.Error())
					return
				}
				_ = workflow.Save(workflowBacklogPath, backlog)
				_ = workflow.WriteAgentReport(*item, []workflow.AgentRun{run}, item.Status, fmt.Sprintf("Commit Agent 失败，已重置开发实现和实际测试，并把失败上下文打回 Dev Agent: %v", err))
			}
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
	if request.MaxSteps <= 0 || request.MaxSteps > 12 {
		request.MaxSteps = 12
	}
	if request.MaxRequirements <= 0 || request.MaxRequirements > 50 {
		if request.ContinueQueue {
			request.MaxRequirements = 50
		} else {
			request.MaxRequirements = 1
		}
	}

	backlog, err := workflow.Load(workflowBacklogPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	startIndex := -1
	for i := range backlog.Items {
		if backlog.Items[i].ID == id {
			startIndex = i
			break
		}
	}
	if startIndex < 0 {
		writeError(w, http.StatusNotFound, "requirement not found")
		return
	}

	currentIndex := startIndex
	allRuns := make([]workflow.AgentRun, 0, request.MaxSteps)
	completed := make([]workflow.Requirement, 0, request.MaxRequirements)
	message := "自动流水线运行完成。"
	for requirementStep := 0; requirementStep < request.MaxRequirements && currentIndex >= 0; requirementStep++ {
		item := &backlog.Items[currentIndex]
		if item.TaskDir == "" {
			writeError(w, http.StatusBadRequest, "taskDir is empty; run intake first")
			return
		}
		runs, itemMessage, pipelineErr := runRequirementPipeline(&backlog, item, request)
		allRuns = append(allRuns, runs...)
		message = itemMessage
		if err := workflow.WriteAgentReport(*item, runs, item.Status, itemMessage); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if pipelineErr != nil {
			writePipelineError(w, pipelineErr, runs)
			return
		}
		if item.Status != workflow.StatusCommitted || !request.ContinueQueue {
			writeJSON(w, http.StatusOK, map[string]any{
				"runs":          allRuns,
				"requirement":   item,
				"completed":     completed,
				"active":        item,
				"report":        filepath.Join(item.TaskDir, workflow.AgentReportFileName),
				"message":       message,
				"continueQueue": request.ContinueQueue,
			})
			return
		}
		completed = append(completed, *item)
		nextIndex := nextRunnableRequirementIndex(backlog, currentIndex)
		if nextIndex < 0 {
			message = "当前需求已完成。需求列表里没有下一个可运行任务。"
			writeJSON(w, http.StatusOK, map[string]any{
				"runs":          allRuns,
				"requirement":   item,
				"completed":     completed,
				"active":        item,
				"report":        filepath.Join(item.TaskDir, workflow.AgentReportFileName),
				"message":       message,
				"continueQueue": request.ContinueQueue,
			})
			return
		}
		currentIndex = nextIndex
	}
	active := &backlog.Items[currentIndex]
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":          allRuns,
		"requirement":   active,
		"completed":     completed,
		"active":        active,
		"report":        filepath.Join(active.TaskDir, workflow.AgentReportFileName),
		"message":       message,
		"continueQueue": request.ContinueQueue,
	})
}

type pipelineError struct {
	StatusCode  int
	Message     string
	Agent       workflow.Agent
	Log         string
	LogContent  string
	ParallelRun []workflow.AgentRun
}

func runRequirementPipeline(backlog *workflow.Backlog, item *workflow.Requirement, request runWorkflowPipelineRequest) ([]workflow.AgentRun, string, *pipelineError) {
	if err := workflow.EnsureTaskFiles(*item); err != nil {
		return nil, "", &pipelineError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
	}
	reconcileCommitFailure(item)
	runs := make([]workflow.AgentRun, 0, request.MaxSteps)
	message := "自动流水线运行完成。"
	for step := 0; step < request.MaxSteps; step++ {
		if item.Status == workflow.StatusReviewPass {
			parallelRuns, nextStatus, err := runReviewPassParallel(*item, request.Command)
			runs = append(runs, parallelRuns...)
			if err != nil {
				return runs, fmt.Sprintf("测试用例和开发并行阶段失败: %v", err), &pipelineError{
					StatusCode:  http.StatusInternalServerError,
					Message:     fmt.Sprintf("parallel test-plan/dev failed: %v", err),
					ParallelRun: parallelRuns,
				}
			}
			item.Status = nextStatus
			item.UpdatedAt = time.Now().UTC()
			if err := workflow.Save(workflowBacklogPath, *backlog); err != nil {
				return runs, message, &pipelineError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
			}
			if item.Status == workflow.StatusTestExecReady {
				continue
			}
		}
		agent, err := workflow.ResolveAgent(item.Status, workflow.AgentAuto)
		if err != nil {
			message = err.Error()
			break
		}
		run, err := workflow.RunAgent(*item, agent, request.Command)
		if err != nil {
			if agent == workflow.AgentCommit {
				run.NextStatus = workflow.StatusCommitFail
				runs = append(runs, run)
				item.Status = workflow.StatusCommitFail
				item.UpdatedAt = time.Now().UTC()
				if resetErr := workflow.ResetDevAndTestAfterCommitFailure(*item); resetErr != nil {
					return runs, message, &pipelineError{StatusCode: http.StatusInternalServerError, Message: resetErr.Error()}
				}
				if err := workflow.Save(workflowBacklogPath, *backlog); err != nil {
					return runs, message, &pipelineError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
				}
				return runs, fmt.Sprintf("Commit Agent 失败，已重置开发实现和实际测试，并把失败上下文打回 Dev Agent: %v", err), nil
			}
			logContent := ""
			if run.OutputPath != "" {
				if bytes, readErr := os.ReadFile(run.OutputPath); readErr == nil {
					logContent = filterAgentLogNoise(string(bytes))
				}
			}
			return runs, fmt.Sprintf("自动流水线在 %s 失败: %v", agent, err), &pipelineError{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("agent %s failed: %v", agent, err),
				Agent:      agent,
				Log:        run.OutputPath,
				LogContent: logContent,
			}
		}
		runs = append(runs, run)
		item.Status = run.NextStatus
		if run.Score >= 0 {
			item.Score = run.Score
		}
		item.UpdatedAt = time.Now().UTC()
		if err := workflow.Save(workflowBacklogPath, *backlog); err != nil {
			return runs, message, &pipelineError{StatusCode: http.StatusInternalServerError, Message: err.Error()}
		}
		if item.Status == workflow.StatusCommitted {
			break
		}
	}
	return runs, message, nil
}

func writePipelineError(w http.ResponseWriter, err *pipelineError, runs []workflow.AgentRun) {
	if err == nil {
		return
	}
	body := map[string]any{
		"error": err.Message,
		"runs":  runs,
	}
	if err.Agent != "" {
		body["agent"] = err.Agent
	}
	if err.Log != "" {
		body["log"] = err.Log
	}
	if err.LogContent != "" {
		body["logContent"] = err.LogContent
	}
	if len(err.ParallelRun) > 0 {
		body["runs"] = err.ParallelRun
	}
	writeJSON(w, err.StatusCode, body)
}

func nextRunnableRequirementIndex(backlog workflow.Backlog, currentIndex int) int {
	if len(backlog.Items) == 0 {
		return -1
	}
	for offset := 1; offset < len(backlog.Items); offset++ {
		index := (currentIndex + offset) % len(backlog.Items)
		item := backlog.Items[index]
		if isRunnableRequirement(item) {
			return index
		}
	}
	return -1
}

func isRunnableRequirement(item workflow.Requirement) bool {
	if item.TaskDir == "" {
		return false
	}
	if _, err := workflow.ResolveAgent(item.Status, workflow.AgentAuto); err != nil {
		return false
	}
	status, err := workflow.EnsureAgentStatus(item)
	if err != nil {
		return false
	}
	for _, agent := range status.Agents {
		if agent.Status == workflow.AgentStatusRunning {
			return false
		}
	}
	return true
}

func runReviewPassParallel(item workflow.Requirement, command string) ([]workflow.AgentRun, string, error) {
	status, err := workflow.EnsureAgentStatus(item)
	if err != nil {
		return nil, item.Status, err
	}
	agents := make([]workflow.Agent, 0, 2)
	if status.Agents[string(workflow.AgentTestPlan)].Status != workflow.AgentStatusDone {
		agents = append(agents, workflow.AgentTestPlan)
	}
	if status.Agents[string(workflow.AgentDev)].Status != workflow.AgentStatusDone {
		agents = append(agents, workflow.AgentDev)
	}
	if len(agents) == 0 {
		return nil, workflow.StatusTestExecReady, nil
	}
	runs := make([]workflow.AgentRun, len(agents))
	errs := make([]error, len(agents))
	var wg sync.WaitGroup
	for i, agent := range agents {
		wg.Add(1)
		go func(index int, current workflow.Agent) {
			defer wg.Done()
			runs[index], errs[index] = workflow.RunAgent(item, current, command)
		}(i, agent)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return runs, item.Status, fmt.Errorf("%s failed: %w", agents[i], err)
		}
	}
	status, err = workflow.EnsureAgentStatus(item)
	if err != nil {
		return runs, item.Status, err
	}
	if status.Agents[string(workflow.AgentTestPlan)].Status == workflow.AgentStatusDone &&
		status.Agents[string(workflow.AgentDev)].Status == workflow.AgentStatusDone {
		return runs, workflow.StatusTestExecReady, nil
	}
	return runs, workflow.StatusReviewPass, nil
}

func reconcileCommitFailure(item *workflow.Requirement) {
	if item == nil || item.Status != workflow.StatusCommitReady || item.TaskDir == "" {
		return
	}
	status, err := workflow.EnsureAgentStatus(*item)
	if err != nil {
		return
	}
	if status.Agents[string(workflow.AgentCommit)].Status == workflow.AgentStatusFailed {
		item.Status = workflow.StatusCommitFail
		item.UpdatedAt = time.Now().UTC()
		_ = workflow.ResetDevAndTestAfterCommitFailure(*item)
	}
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
	content = filterAgentLogNoise(content)
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
	content = filterAgentLogNoise(content)
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

func filterAgentLogNoise(content string) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if isNoisyCodexSkillIconWarning(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func isNoisyCodexSkillIconWarning(line string) bool {
	return strings.Contains(line, "codex_core_skills::loader") &&
		strings.Contains(line, "icon path must not contain '..'") &&
		(strings.Contains(line, "interface.icon_small") || strings.Contains(line, "interface.icon_large"))
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
