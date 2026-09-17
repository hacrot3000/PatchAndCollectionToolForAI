package session

import (
	"fmt"
	"syscall"
)

// Kill forcefully terminates the whole process group owned by a running session.
// It is intentionally separate from Stop, which remains a graceful interrupt.
func (m *Manager) Kill(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session not found")
	}
	s.mu.Lock()
	if s.meta.Status != "running" || s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}
	s.stopRequested = true
	pid := s.cmd.Process.Pid
	process := s.cmd.Process
	s.mu.Unlock()
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	if err := process.Kill(); err != nil {
		return fmt.Errorf("kill session: %w", err)
	}
	return nil
}

// Clear drops the server-side scrollback without affecting the running process.
// New output continues to stream normally and a later reconnect starts from an
// empty backlog instead of replaying console text that the user cleared.
func (m *Manager) Clear(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session not found")
	}
	s.mu.Lock()
	s.scrollback = nil
	s.mu.Unlock()
	return nil
}
