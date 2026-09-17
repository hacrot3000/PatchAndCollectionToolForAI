package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// CurrentCwd returns the live working directory of a running terminal shell.
// On Linux, /proc/<pid>/cwd follows cd changes made by the shell, unlike the
// static cwd captured when the PTY was first created.
func (m *Manager) CurrentCwd(id string) (string, error) {
	s, ok := m.Get(id)
	if !ok {
		return "", fmt.Errorf("session not found")
	}
	s.mu.Lock()
	meta := s.meta
	pid := 0
	if s.cmd != nil && s.cmd.Process != nil {
		pid = s.cmd.Process.Pid
	}
	s.mu.Unlock()
	if meta.TaskID != 0 {
		return meta.Cwd, nil
	}
	if meta.Status == "running" && pid > 0 {
		cwd, err := os.Readlink(filepath.Join("/proc", fmt.Sprintf("%d", pid), "cwd"))
		if err == nil && cwd != "" {
			return filepath.Clean(cwd), nil
		}
	}
	if meta.Cwd == "" {
		return "", fmt.Errorf("terminal cwd unavailable")
	}
	return filepath.Clean(meta.Cwd), nil
}
