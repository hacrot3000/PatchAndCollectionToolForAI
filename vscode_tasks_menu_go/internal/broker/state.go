package broker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	runtimestate "bletonfc/vscode_tasks_menu/internal/state"
)

const (
	stateFileName = "session-broker.json"
	socketFileName = "session-broker.sock"
	lockFileName = "session-broker.lock"
)

func StatePath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), stateFileName)
}

func SocketPath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), socketFileName)
}

func LockPath(workspace string) string {
	return filepath.Join(runtimestate.Dir(workspace), lockFileName)
}

func NewInfo(workspace string) Info {
	return Info{
		ProtocolVersion: ProtocolVersion,
		PID:             os.Getpid(),
		Workspace:       workspace,
		SocketPath:      SocketPath(workspace),
		StartedAt:       time.Now().Format(time.RFC3339),
	}
}

func SaveInfo(info Info) error {
	if info.Workspace == "" {
		return fmt.Errorf("broker workspace is empty")
	}
	if info.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported broker protocol version %d", info.ProtocolVersion)
	}
	if info.PID <= 0 {
		return fmt.Errorf("invalid broker pid %d", info.PID)
	}
	if info.SocketPath == "" {
		return fmt.Errorf("broker socket path is empty")
	}
	if err := runtimestate.EnsureDir(info.Workspace); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode broker state: %w", err)
	}
	tmp, err := os.CreateTemp(runtimestate.Dir(info.Workspace), ".session-broker.*.tmp")
	if err != nil {
		return fmt.Errorf("create broker state temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write broker state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync broker state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close broker state: %w", err)
	}
	if err := os.Rename(name, StatePath(info.Workspace)); err != nil {
		return fmt.Errorf("replace broker state: %w", err)
	}
	return os.Chmod(StatePath(info.Workspace), 0o600)
}

func LoadInfo(workspace string) (Info, error) {
	data, err := os.ReadFile(StatePath(workspace))
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return Info{}, fmt.Errorf("parse broker state: %w", err)
	}
	if info.Workspace != workspace {
		return Info{}, fmt.Errorf("broker workspace mismatch")
	}
	if info.ProtocolVersion != ProtocolVersion {
		return Info{}, fmt.Errorf("unsupported broker protocol version %d", info.ProtocolVersion)
	}
	if info.PID <= 0 || info.SocketPath == "" {
		return Info{}, fmt.Errorf("invalid broker state")
	}
	return info, nil
}

func RemoveInfoIfPID(workspace string, pid int) {
	info, err := LoadInfo(workspace)
	if err != nil || info.PID != pid {
		return
	}
	_ = os.Remove(StatePath(workspace))
}
