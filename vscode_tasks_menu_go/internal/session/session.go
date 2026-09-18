package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"bletonfc/vscode_tasks_menu/internal/tasks"
	"github.com/creack/pty"
)

type Metadata struct {
	ID             string `json:"id"`
	TaskID         int    `json:"task_id"`
	Label          string `json:"label"`
	Title          string `json:"title,omitempty"`
	CommandPreview string `json:"command_preview"`
	Cwd            string `json:"cwd"`
	Status         string `json:"status"`
	ExitCode       *int   `json:"exit_code,omitempty"`
	StartedAt      string `json:"started_at"`
	EndedAt        string `json:"ended_at,omitempty"`
}

type managedSession struct {
	mu            sync.Mutex
	meta          Metadata
	cmd           *exec.Cmd
	ptyFile       *os.File
	scrollback    []byte
	maxScrollback int
	subscribers   map[chan []byte]struct{}
	stopRequested bool
}

type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*managedSession
	maxScrollback int
}

func NewManager(maxScrollback int) *Manager {
	if maxScrollback <= 0 {
		maxScrollback = 4 << 20
	}
	return &Manager{sessions: map[string]*managedSession{}, maxScrollback: maxScrollback}
}

func (m *Manager) Start(spec tasks.Execution) (Metadata, error) {
	id, err := randomID()
	if err != nil {
		return Metadata{}, err
	}
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Cwd
	cmd.Env = spec.Env
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		return Metadata{}, fmt.Errorf("start PTY for %s: %w", spec.Label, err)
	}
	header := taskHeader(spec)
	if len(header) > m.maxScrollback {
		header = header[len(header)-m.maxScrollback:]
	}
	s := &managedSession{
		meta: Metadata{ID: id, TaskID: spec.TaskID, Label: spec.Label, CommandPreview: spec.Preview, Cwd: spec.Cwd, Status: "running", StartedAt: time.Now().Format(time.RFC3339)},
		cmd: cmd, ptyFile: ptmx, scrollback: append([]byte(nil), header...), maxScrollback: m.maxScrollback, subscribers: map[chan []byte]struct{}{},
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	go s.readLoop()
	return s.metadata(), nil
}

func taskHeader(spec tasks.Execution) []byte {
	if spec.TaskID == 0 {
		return []byte(fmt.Sprintf(
			"==============================================================================\r\n"+
				"Terminal   : %s\r\n"+
				"Thư mục    : %s\r\n"+
				"Mô tả      : %s\r\n"+
				"==============================================================================\r\n\r\n",
			spec.Preview, spec.Cwd, spec.Detail,
		))
	}
	return []byte(fmt.Sprintf(
		"==============================================================================\r\n"+
			"Task       : %s\r\n"+
			"Thư mục    : %s\r\n"+
			"Lệnh       : %s\r\n"+
			"Mô tả      : %s\r\n"+
			"==============================================================================\r\n\r\n",
		spec.Label, spec.Cwd, spec.Preview, spec.Detail,
	))
}

func (m *Manager) List() []Metadata {
	m.mu.RLock()
	items := make([]*managedSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, s)
	}
	m.mu.RUnlock()
	out := make([]Metadata, 0, len(items))
	for _, s := range items {
		out = append(out, s.metadata())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out
}

func (m *Manager) Get(id string) (*managedSession, bool) {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	return s, ok
}

func (m *Manager) Metadata(id string) (Metadata, bool) {
	s, ok := m.Get(id)
	if !ok {
		return Metadata{}, false
	}
	return s.metadata(), true
}

func NormalizeTitle(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(value))
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func (m *Manager) SetTitle(id, title string) (Metadata, error) {
	s, ok := m.Get(id)
	if !ok {
		return Metadata{}, fmt.Errorf("session not found")
	}
	s.mu.Lock()
	s.meta.Title = NormalizeTitle(title)
	meta := s.meta
	s.mu.Unlock()
	return meta, nil
}


func (m *Manager) Input(id string, data []byte) error {
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session not found")
	}
	s.mu.Lock()
	file := s.ptyFile
	running := s.meta.Status == "running"
	s.mu.Unlock()
	if !running || file == nil {
		return fmt.Errorf("session is not running")
	}
	_, err := file.Write(data)
	return err
}

func (m *Manager) Resize(id string, rows, cols uint16) error {
	if rows == 0 || cols == 0 {
		return nil
	}
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session not found")
	}
	s.mu.Lock()
	file := s.ptyFile
	s.mu.Unlock()
	if file == nil {
		return nil
	}
	return pty.Setsize(file, &pty.Winsize{Rows: rows, Cols: cols})
}

func (m *Manager) Stop(id string) error {
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
	terminal := s.meta.TaskID == 0
	s.mu.Unlock()
	if terminal {
		if err := syscall.Kill(-pid, syscall.SIGHUP); err == nil {
			return nil
		}
		return process.Signal(syscall.SIGHUP)
	}
	if err := syscall.Kill(-pid, syscall.SIGINT); err == nil {
		return nil
	}
	return process.Signal(os.Interrupt)
}

func (m *Manager) Shutdown(grace time.Duration) {
	if grace <= 0 {
		grace = 2 * time.Second
	}
	ids := m.runningIDs()
	for _, id := range ids {
		_ = m.Stop(id)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if len(m.runningIDs()) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, id := range m.runningIDs() {
		m.forceKill(id)
	}
}

func (m *Manager) runningIDs() []string {
	m.mu.RLock()
	items := make(map[string]*managedSession, len(m.sessions))
	for id, s := range m.sessions {
		items[id] = s
	}
	m.mu.RUnlock()
	ids := make([]string, 0, len(items))
	for id, s := range items {
		s.mu.Lock()
		running := s.meta.Status == "running"
		s.mu.Unlock()
		if running {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *Manager) forceKill(id string) {
	s, ok := m.Get(id)
	if !ok {
		return
	}
	s.mu.Lock()
	if s.meta.Status != "running" || s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return
	}
	s.stopRequested = true
	pid := s.cmd.Process.Pid
	process := s.cmd.Process
	s.mu.Unlock()
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		_ = process.Kill()
	}
}

func (m *Manager) Remove(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return nil
	}
	s.mu.Lock()
	running := s.meta.Status == "running"
	s.mu.Unlock()
	if running {
		return fmt.Errorf("cannot remove running session")
	}
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Subscribe(id string) ([]byte, <-chan []byte, func(), error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, nil, nil, fmt.Errorf("session not found")
	}
	ch := make(chan []byte, 256)
	s.mu.Lock()
	backlog := append([]byte(nil), s.scrollback...)
	if s.meta.Status == "running" {
		s.subscribers[ch] = struct{}{}
	} else {
		close(ch)
	}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		if _, exists := s.subscribers[ch]; exists {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return backlog, ch, cancel, nil
}

func (s *managedSession) metadata() Metadata {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meta
}

func (s *managedSession) readLoop() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptyFile.Read(buf)
		if n > 0 {
			s.appendOutput(buf[:n])
		}
		if err != nil {
			break
		}
	}
	err := s.cmd.Wait()
	exitCode := 0
	if s.cmd.ProcessState != nil {
		exitCode = s.cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = 1
	}
	label := "Task"
	if s.meta.TaskID == 0 {
		label = "Terminal"
	}
	s.appendOutput([]byte(fmt.Sprintf("\r\n%s kết thúc với mã trả về: %d\r\n", label, exitCode)))
	s.mu.Lock()
	_ = s.ptyFile.Close()
	s.ptyFile = nil
	if s.stopRequested {
		s.meta.Status = "stopped"
	} else {
		s.meta.Status = "exited"
	}
	s.meta.ExitCode = &exitCode
	s.meta.EndedAt = time.Now().Format(time.RFC3339)
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
	s.mu.Unlock()
}

func (s *managedSession) appendOutput(data []byte) {
	chunk := append([]byte(nil), data...)
	s.mu.Lock()
	s.scrollback = append(s.scrollback, chunk...)
	if len(s.scrollback) > s.maxScrollback {
		keep := s.maxScrollback
		copy(s.scrollback, s.scrollback[len(s.scrollback)-keep:])
		s.scrollback = s.scrollback[:keep]
	}
	for ch := range s.subscribers {
		select {
		case ch <- chunk:
		default:
			close(ch)
			delete(s.subscribers, ch)
		}
	}
	s.mu.Unlock()
}

func randomID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
