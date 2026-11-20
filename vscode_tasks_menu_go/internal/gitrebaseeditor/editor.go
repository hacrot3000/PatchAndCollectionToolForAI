package gitrebaseeditor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EditorModeEnv = "TASKDECK_GIT_REBASE_EDITOR"
	TodoPathEnv   = "TASKDECK_GIT_REBASE_TODO"
	StatePathEnv  = "TASKDECK_GIT_REBASE_EDITOR_STATE"
)

type State struct {
	GitDir  string            `json:"git_dir"`
	Reword map[string]string `json:"reword,omitempty"`
}

func ActiveMode() string {
	if os.Getenv(EditorModeEnv) == "1" {
		return "auto"
	}
	return ""
}

func Run(mode string, args []string) error {
	if mode != "auto" {
		return fmt.Errorf("unsupported internal Git editor mode %q", mode)
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("internal Git editor requires exactly one target file")
	}
	target := args[0]
	if filepath.Base(target) == "git-rebase-todo" {
		return RunSequenceEditor(target, os.Getenv(TodoPathEnv))
	}
	return RunMessageEditor(target, os.Getenv(StatePathEnv))
}

func RunSequenceEditor(target, source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("interactive rebase todo source is missing")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read interactive rebase todo: %w", err)
	}
	if len(data) == 0 || len(data) > 1<<20 {
		return fmt.Errorf("interactive rebase todo has invalid size")
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write interactive rebase todo: %w", err)
	}
	return nil
}

func readState(path string) (State, error) {
	var state State
	path = strings.TrimSpace(path)
	if path == "" {
		return state, fmt.Errorf("interactive rebase editor state is missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, fmt.Errorf("read interactive rebase editor state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("decode interactive rebase editor state: %w", err)
	}
	if strings.TrimSpace(state.GitDir) == "" {
		return state, fmt.Errorf("interactive rebase editor Git directory is missing")
	}
	if state.Reword == nil {
		state.Reword = map[string]string{}
	}
	return state, nil
}

func currentTodoSHA(gitDir string) string {
	for _, name := range []string{
		filepath.Join(gitDir, "rebase-merge", "done"),
		filepath.Join(gitDir, "rebase-apply", "done"),
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

func RunMessageEditor(target, statePath string) error {
	state, err := readState(statePath)
	if err != nil {
		return err
	}
	sha := currentTodoSHA(state.GitDir)
	if sha == "" {
		// Git also invokes an editor for squash/fixup message composition. When
		// there is no TaskDeck reword override, preserve Git's prepared message.
		return nil
	}
	message, ok := state.Reword[sha]
	if !ok {
		for full, candidate := range state.Reword {
			if strings.HasPrefix(full, sha) || strings.HasPrefix(sha, full) {
				message, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return nil
	}
	message = strings.TrimSpace(message)
	if message == "" || len(message) > 4000 || strings.ContainsRune(message, '\x00') {
		return fmt.Errorf("invalid TaskDeck reword message")
	}
	return os.WriteFile(target, []byte(message+"\n"), 0o600)
}

func WriteState(path string, state State) error {
	if strings.TrimSpace(state.GitDir) == "" {
		return fmt.Errorf("Git directory is required")
	}
	if state.Reword == nil {
		state.Reword = map[string]string{}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
