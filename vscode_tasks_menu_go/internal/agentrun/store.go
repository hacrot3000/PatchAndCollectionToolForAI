package agentrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) file(runID string) string { return filepath.Join(m.dir, runID+".json") }

func (m *Manager) load() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 8<<20 {
			return errors.New("unsafe run journal record")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record Record
		if err := json.Unmarshal(body, &record); err != nil {
			return errors.New("invalid run journal record")
		}
		if err := record.Spec.Validate(); err != nil || filepath.Base(path) != record.Spec.RunID+".json" {
			return errors.New("invalid run journal binding")
		}
		switch record.Status {
		case Queued, Running, WaitingTool, Completed, Failed, Stopped, Interrupted:
		default:
			return errors.New("invalid run status")
		}
		if record.Status == Running || record.Status == WaitingTool {
			record.Status = Interrupted
			record.RetrySafe = false
			record.LastError = "TaskDeck restarted while run was active; inspect side effects before explicit resume"
			now := time.Now().UTC()
			record.UpdatedAt, record.FinishedAt = now, &now
			if err := m.saveLocked(record); err != nil {
				return err
			}
		}
		m.runs[record.Spec.RunID] = record
		if len(m.runs) > maxStoredRuns {
			return errors.New("run journal exceeds capacity")
		}
	}
	return nil
}

func (m *Manager) saveLocked(record Record) error {
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(body) > 8<<20 {
		return errors.New("run record exceeds storage limit")
	}
	tmp, err := os.CreateTemp(m.dir, ".run-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(body, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpPath, m.file(record.Spec.RunID)); err != nil {
		return err
	}
	return syncRunDir(m.dir)
}

func syncRunDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
