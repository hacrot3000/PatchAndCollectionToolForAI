package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
	stopRequested  bool
	protocol       ProtocolState
	protocolCommand *os.File
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

	if spec.ProtocolCommands && !spec.ProtocolEvents {
		return Metadata{}, fmt.Errorf("Patch protocol command channel requires protocol events")
	}
	if spec.ProtocolCommands && runtime.GOOS == "windows" {
		return Metadata{}, fmt.Errorf("Patch protocol command channel is not available on Windows")
	}

	var protocolRead, protocolWrite *os.File
	if spec.ProtocolEvents && runtime.GOOS != "windows" {
		protocolRead, protocolWrite, err = os.Pipe()
		if err != nil {
			return Metadata{}, fmt.Errorf("create Patch protocol pipe for %s: %w", spec.Label, err)
		}
		cmd.ExtraFiles = append(cmd.ExtraFiles, protocolWrite)
		cmd.Env = setEnvironmentValue(cmd.Env, "TASKDECK_PATCH_EVENT_FD", "3")
	}

	var commandRead, commandWrite *os.File
	if spec.ProtocolCommands {
		commandRead, commandWrite, err = os.Pipe()
		if err != nil {
			if protocolRead != nil {
				_ = protocolRead.Close()
			}
			if protocolWrite != nil {
				_ = protocolWrite.Close()
			}
			return Metadata{}, fmt.Errorf("create Patch protocol command pipe for %s: %w", spec.Label, err)
		}
		cmd.ExtraFiles = append(cmd.ExtraFiles, commandRead)
		cmd.Env = setEnvironmentValue(cmd.Env, "TASKDECK_PATCH_COMMAND_FD", "4")
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if protocolWrite != nil {
		_ = protocolWrite.Close()
	}
	if commandRead != nil {
		_ = commandRead.Close()
	}
	if err != nil {
		if protocolRead != nil {
			_ = protocolRead.Close()
		}
		if commandWrite != nil {
			_ = commandWrite.Close()
		}
		return Metadata{}, fmt.Errorf("start PTY for %s: %w", spec.Label, err)
	}
	header := taskHeader(spec)
	if len(header) > m.maxScrollback {
		header = header[len(header)-m.maxScrollback:]
	}
	s := &managedSession{
		meta: Metadata{ID: id, TaskID: spec.TaskID, Label: spec.Label, CommandPreview: spec.Preview, Cwd: spec.Cwd, Status: "running", StartedAt: time.Now().Format(time.RFC3339)},
		cmd: cmd, ptyFile: ptmx, scrollback: append([]byte(nil), header...), maxScrollback: m.maxScrollback, subscribers: map[chan []byte]struct{}{},
		protocol: ProtocolState{Available: true, Enabled: protocolRead != nil, CommandsEnabled: commandWrite != nil},
		protocolCommand: commandWrite,
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	if protocolRead != nil {
		go s.protocolReadLoop(protocolRead)
	}
	go s.readLoop()
	return s.metadata(), nil
}

func setEnvironmentValue(env []string, key, value string) []string {
	if env == nil {
		env = os.Environ()
	}
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		out = append(out, item)
	}
	return append(out, prefix+value)
}

func (s *managedSession) applyProtocolLine(line []byte) {
	if len(line) == 0 || len(line) > maxProtocolEventBytes {
		return
	}
	var envelope protocolEnvelope
	if err := json.Unmarshal(line, &envelope); err != nil {
		s.setProtocolError("invalid protocol JSON: " + err.Error())
		return
	}
	if envelope.Protocol != patchProtocolName || envelope.Version != patchProtocolVersion || strings.TrimSpace(envelope.Type) == "" {
		s.setProtocolError("unsupported Patch protocol envelope")
		return
	}
	raw := append(json.RawMessage(nil), line...)
	var itemState ProtocolItemState
	var artifactState ProtocolArtifactState
	var progressState ProtocolProgressState
	var actionResultState ProtocolActionResultState
	var queueMutationState ProtocolQueueMutationResultState
	var historyManagementState ProtocolHistoryManagementResultState
	var historySupportState ProtocolHistorySupportResultState
	var planSnapshotState ProtocolPlanSnapshotState
	var healthSnapshotState ProtocolHealthSnapshotState
	if envelope.Type == "prompt" {
		if _, err := protocolPromptID(raw); err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "item_started" || envelope.Type == "item_finished" {
		var err error
		itemState, _, err = protocolItemEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "artifact" {
		var err error
		artifactState, err = protocolArtifactEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "progress" {
		var err error
		progressState, err = protocolProgressEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "action_result" {
		var err error
		actionResultState, err = protocolActionResultEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "queue_mutation_result" {
		var err error
		queueMutationState, err = protocolQueueMutationResultEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "history_management_result" {
		var err error
		historyManagementState, err = protocolHistoryManagementResultEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "history_support_result" {
		var err error
		historySupportState, err = protocolHistorySupportResultEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "plan_snapshot" {
		var err error
		planSnapshotState, err = protocolPlanSnapshotEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	if envelope.Type == "health_snapshot" {
		var err error
		healthSnapshotState, err = protocolHealthSnapshotEvent(raw)
		if err != nil {
			s.setProtocolError(err.Error())
			return
		}
	}
	s.mu.Lock()
	s.protocol.EventCount++
	s.protocol.LastSeq = envelope.Seq
	s.protocol.LastEvent = raw
	s.protocol.Error = ""
	switch envelope.Type {
	case "run_started":
		s.protocol.Items = nil
		s.protocol.Artifacts = nil
		s.protocol.Progress = nil
		s.protocol.ActionResult = nil
		s.protocol.QueueMutation = nil
		s.protocol.HistoryManagement = nil
		s.protocol.HistorySupport = nil
		s.protocol.HistoryReport = nil
		s.protocol.PlanSnapshot = nil
		s.protocol.HealthSnapshot = nil
	case "queue_snapshot":
		s.protocol.QueueSnapshot = append(json.RawMessage(nil), raw...)
	case "resume_snapshot":
		s.protocol.ResumeSnapshot = append(json.RawMessage(nil), raw...)
	case "history_snapshot":
		s.protocol.HistorySnapshot = append(json.RawMessage(nil), raw...)
	case "history_report":
		s.protocol.HistoryReport = append(json.RawMessage(nil), raw...)
	case "plan_snapshot":
		planSnapshot := planSnapshotState
		s.protocol.PlanSnapshot = &planSnapshot
	case "health_snapshot":
		healthSnapshot := healthSnapshotState
		s.protocol.HealthSnapshot = &healthSnapshot
	case "prompt":
		s.protocol.Prompt = append(json.RawMessage(nil), raw...)
		s.protocol.ActionResult = nil
		s.protocol.HistorySupport = nil
		s.protocol.HistoryReport = nil
	case "item_started", "item_finished":
		s.protocol.Items = upsertProtocolItem(s.protocol.Items, itemState)
	case "artifact":
		s.protocol.Artifacts = upsertProtocolArtifact(s.protocol.Artifacts, artifactState)
	case "progress":
		progress := progressState
		s.protocol.Progress = &progress
	case "action_result":
		actionResult := actionResultState
		s.protocol.ActionResult = &actionResult
	case "queue_mutation_result":
		queueMutation := queueMutationState
		s.protocol.QueueMutation = &queueMutation
	case "history_management_result":
		historyManagement := historyManagementState
		s.protocol.HistoryManagement = &historyManagement
	case "history_support_result":
		historySupport := historySupportState
		s.protocol.HistorySupport = &historySupport
	case "run_finished":
		s.protocol.Prompt = nil
	}
	s.mu.Unlock()
}

func (s *managedSession) setProtocolError(message string) {
	s.mu.Lock()
	s.protocol.Error = message
	s.mu.Unlock()
}

func (s *managedSession) protocolReadLoop(file *os.File) {
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxProtocolEventBytes)
	for scanner.Scan() {
		s.applyProtocolLine(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		s.setProtocolError("Patch protocol read failed: " + err.Error())
	}
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

func (m *Manager) ProtocolState(id string) (ProtocolState, error) {
	s, ok := m.Get(id)
	if !ok {
		return ProtocolState{}, fmt.Errorf("session not found")
	}
	s.mu.Lock()
	state := cloneProtocolState(s.protocol)
	s.mu.Unlock()
	return state, nil
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


func (m *Manager) ProtocolCommand(id string, data []byte) error {
	line, err := validateProtocolCommand(data)
	if err != nil {
		return err
	}
	responsePromptID, isPromptResponse, err := protocolPromptResponseID(line)
	if err != nil {
		return err
	}
	actionPromptID, isItemAction, err := protocolItemActionPromptID(line)
	if err != nil {
		return err
	}
	resumePromptID, isResumeAction, err := protocolResumeActionPromptID(line)
	if err != nil {
		return err
	}
	historyPromptID, isHistoryDetail, err := protocolHistoryDetailPromptID(line)
	if err != nil {
		return err
	}
	queueDeletePromptID, isQueueDelete, err := protocolQueueDeletePromptID(line)
	if err != nil {
		return err
	}
	historyManagePromptID, isHistoryManage, err := protocolHistoryManagePromptID(line)
	if err != nil {
		return err
	}
	historySupportPromptID, isHistorySupport, err := protocolHistorySupportPromptID(line)
	if err != nil {
		return err
	}
	s, ok := m.Get(id)
	if !ok {
		return fmt.Errorf("session not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meta.Status != "running" || s.protocolCommand == nil {
		return fmt.Errorf("Patch protocol command channel is not enabled")
	}
	if isPromptResponse || isItemAction || isResumeAction || isHistoryDetail || isQueueDelete || isHistoryManage || isHistorySupport {
		if len(s.protocol.Prompt) == 0 {
			return fmt.Errorf("Patch session has no active prompt")
		}
		activePromptID, err := protocolPromptID(s.protocol.Prompt)
		if err != nil {
			return err
		}
		boundPromptID := responsePromptID
		commandName := "prompt_response"
		if isItemAction {
			boundPromptID = actionPromptID
			commandName = "item_action"
		} else if isResumeAction {
			boundPromptID = resumePromptID
			commandName = "resume_action"
		} else if isHistoryDetail {
			boundPromptID = historyPromptID
			commandName = "history_detail"
		} else if isQueueDelete {
			boundPromptID = queueDeletePromptID
			commandName = "queue_delete"
		} else if isHistoryManage {
			boundPromptID = historyManagePromptID
			commandName = "history_manage"
		} else if isHistorySupport {
			boundPromptID = historySupportPromptID
			commandName = "history_support"
		}
		if boundPromptID != activePromptID {
			return fmt.Errorf("Patch %s does not match the active prompt", commandName)
		}
	}
	if _, err := s.protocolCommand.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write Patch protocol command: %w", err)
	}
	if isPromptResponse || isResumeAction {
		s.protocol.Prompt = nil
	}
	return nil
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
	if s.protocolCommand != nil {
		_ = s.protocolCommand.Close()
		s.protocolCommand = nil
	}
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
