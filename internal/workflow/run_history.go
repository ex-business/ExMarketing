package workflow

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RunHistoryItem struct {
	ID         string    `json:"id"`
	Agent      string    `json:"agent"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modifiedAt"`
	Current    bool      `json:"current"`
	Status     string    `json:"status,omitempty"`
	NextStatus string    `json:"nextStatus,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func ListRunHistory(req Requirement) ([]RunHistoryItem, error) {
	status, err := EnsureAgentStatus(req)
	if err != nil {
		return nil, err
	}
	currentLogs := map[string]AgentStatus{}
	for _, agent := range status.Agents {
		if agent.Log != "" {
			currentLogs[filepath.Clean(agent.Log)] = agent
		}
	}

	runDir := filepath.Join(req.TaskDir, "runs")
	entries, err := os.ReadDir(runDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunHistoryItem{}, nil
		}
		return nil, err
	}
	items := make([]RunHistoryItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(entry.Name(), ".log")
		agent := ""
		if idx := strings.LastIndex(id, "-"); idx >= 0 && idx+1 < len(id) {
			agent = id[idx+1:]
		}
		path := filepath.Join(runDir, entry.Name())
		current, isCurrent := currentLogs[filepath.Clean(path)]
		items = append(items, RunHistoryItem{
			ID:         id,
			Agent:      agent,
			Path:       path,
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC(),
			Current:    isCurrent,
			Status:     current.Status,
			NextStatus: current.NextStatus,
			Error:      current.Error,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ModifiedAt.After(items[j].ModifiedAt)
	})
	return items, nil
}
