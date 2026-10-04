package session

import (
	"fmt"
)

type ProcessInfo struct {
	PID            int    `json:"pid"`
	PPID           int    `json:"ppid"`
	PGID           int    `json:"pgid,omitempty"`
	State          string `json:"state,omitempty"`
	ElapsedSeconds int64  `json:"elapsed_seconds,omitempty"`
	Command        string `json:"command,omitempty"`
	Args           string `json:"args,omitempty"`
	Depth          int    `json:"depth"`
}

type TerminateCapability interface {
	Terminate(string) error
}

type ProcessTreeCapability interface {
	ProcessTree(string) ([]ProcessInfo, error)
}

func (m *Manager) ProcessTree(id string) ([]ProcessInfo, error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	s.mu.Lock()
	if s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return []ProcessInfo{}, nil
	}
	pid := s.cmd.Process.Pid
	s.mu.Unlock()
	return collectProcessTree(pid)
}
