package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// applyCommandJournalToState recovers in-flight command rows from bounded,
// run-owned files. This is a fallback: real-time machine events remain primary.
// The copy of protocol state is modified outside the session mutex, avoiding
// blocking the terminal reader on disk IO while a PATCH command runs.
func applyCommandJournalToState(projectRoot string, state *ProtocolState) {
	if state == nil || projectRoot == "" {
		return
	}
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return
	}
	for i := range state.Items {
		item := &state.Items[i]
		if item.RunID == "" || item.Index < 1 || (item.Kind != "PATCH" && item.Kind != "COLLECT") {
			continue
		}
		rows := readCommandJournal(root, *item)
		if len(rows) == 0 {
			continue
		}
		for _, row := range rows {
            // A stale journal can survive abrupt process termination. Do not
            // present QUEUE/RUNNING under a completed parent task.
            if item.Status != "RUNNING" && (row.Status == "QUEUE" || row.Status == "RUNNING") {
                if row.Status == "RUNNING" && item.Status != "PASS" {
                    row.Status = "INTERRUPTED"
                } else {
                    row.Status = "SKIPPED"
                }
            }
			updated := false
			for j := range item.Commands {
				if item.Commands[j].Index == row.Index {
					// Do not let a delayed journal rewind an already finished
					// command to QUEUE or RUNNING.
					if (row.Status == "QUEUE" || row.Status == "RUNNING") &&
						isTerminalCommandStatus(item.Commands[j].Status) {
						updated = true
						break
					}
					item.Commands[j] = row
					updated = true
					break
				}
			}
			if !updated && len(item.Commands) < 64 {
				item.Commands = append(item.Commands, row)
			}
		}
		sort.Slice(item.Commands, func(i, j int) bool { return item.Commands[i].Index < item.Commands[j].Index })
	}
}

func isTerminalCommandStatus(status string) bool {
	switch status {
	case "PASS", "FAIL", "INTERRUPTED", "SKIPPED":
		return true
	}
	return false
}

// Restrict all ancestors and the JSON leaf to real directories/files owned by
// this workspace. No absolute/../user-chosen paths are accepted.
func realRunDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func readCommandJournal(root string, item ProtocolItemState) []ProtocolCommandEvidenceState {
	parts := []string{
		root, "artifacts", "patch_tool", "runs", item.RunID, "command_items",
	}
	for i := 1; i < len(parts); i++ {
		path := filepath.Join(parts[:i+1]...)
		if !realRunDirectory(path) {
			return nil
		}
	}
	base := filepath.Join(parts...)
	dir, err := os.Open(base)
	if err != nil {
		return nil
	}
	entries, err := dir.ReadDir(4097)
	dir.Close()
	if err != nil && err != io.EOF {
		return nil
	}
	if len(entries) > 4096 {
		return nil
	}
	prefix := fmt.Sprintf("%03d_", item.Index)
	var itemFolder string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			if itemFolder != "" {
				return nil // Ambiguous run evidence; never guess.
			}
			itemFolder = filepath.Join(base, entry.Name())
		}
	}
	if itemFolder == "" || !realRunDirectory(itemFolder) {
		return nil
	}
	logFolder := filepath.Join(itemFolder, "command_logs")
	if !realRunDirectory(logFolder) {
		return nil
	}
	file := filepath.Join(logFolder, "command_states.json")
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 65536 {
		return nil
	}
	handle, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, 65537))
	if err != nil || len(data) > 65536 {
		return nil
	}
	var journal struct {
		Format string `json:"format"`
		Commands []ProtocolCommandEvidenceState `json:"commands"`
	}
	if err := json.Unmarshal(data, &journal); err != nil ||
		journal.Format != "taskdeck.patch.command-states.v1" ||
		len(journal.Commands) > 64 {
		return nil
	}
	out := make([]ProtocolCommandEvidenceState, 0, len(journal.Commands))
	seen := make(map[int]bool, len(journal.Commands))
	for _, command := range journal.Commands {
		if seen[command.Index] {
			return nil
		}
		seen[command.Index] = true
		// Reuse exactly the strict machine-channel validation for fallback
		// records; never promote arbitrary JSON into frontend state.
		event, err := json.Marshal(map[string]interface{}{
			"type": "command_state", "item_index": item.Index,
			"item_name": item.Name, "command": command,
		})
		if err != nil {
			return nil
		}
		_, _, row, err := protocolCommandStateEvent(event)
		if err != nil {
			return nil
		}
		out = append(out, row)
	}
	return out
}
