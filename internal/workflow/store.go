package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func Load(path string) (Backlog, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Backlog{}, nil
		}
		return Backlog{}, err
	}
	if len(bytes) == 0 {
		return Backlog{}, nil
	}
	var backlog Backlog
	if err := json.Unmarshal(bytes, &backlog); err != nil {
		return Backlog{}, err
	}
	return backlog, nil
}

func Save(path string, backlog Backlog) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(backlog, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	return os.WriteFile(path, bytes, 0o644)
}
