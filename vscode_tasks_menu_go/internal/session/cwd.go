package session

import (
	"fmt"
	"os"
	"path/filepath"
    "runtime"
    "strconv"
    "strings"
    "os/exec"
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
        switch runtime.GOOS {
        case "linux":
            cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
            if err == nil && cwd != "" { return filepath.Clean(cwd), nil }
        case "darwin":
            // macOS has no /proc; lsof reports the active terminal's cwd.
            // If lsof is unavailable, retain the safe launch-directory fallback.
            output, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
            if err == nil && len(output) < 65536 {
                for _, line := range strings.Split(string(output), "\n") {
                    if !strings.HasPrefix(line, "n/") { continue }
                    cwd := filepath.Clean(strings.TrimPrefix(line, "n"))
                    if info, err := os.Stat(cwd); err == nil && info.IsDir() {
                        return cwd, nil
                    }
                }
            }
        }
    }
	if meta.Cwd == "" {
		return "", fmt.Errorf("terminal cwd unavailable")
	}
	return filepath.Clean(meta.Cwd), nil
}
