package tasks

import (
	"strings"
	"testing"
)

func TestRemoteApprovalTokenPinsCompleteCanonicalTaskDefinition(t *testing.T) {
	raw := map[string]any{
		"label": "host health",
		"type": "process",
		"command": "/bin/echo",
		"args": []any{"ok"},
		"taskdeckRemoteAllowed": true,
	}
	id, _, err := stableTaskID(raw)
	if err != nil {
		t.Fatal(err)
	}
	task := Task{ID: id, Raw: raw}
	token, err := RemoteApprovalToken(task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, strings.Split(token, "@")[0]+"@") || len(strings.Split(token, "@")) != 2 || len(strings.Split(token, "@")[1]) != 64 {
		t.Fatalf("invalid approval token length: %q", token)
	}
	// JSON map order must not change an owner's approved definition.
	same := Task{ID: id, Raw: map[string]any{
		"taskdeckRemoteAllowed": true, "args": []any{"ok"},
		"command": "/bin/echo", "type": "process", "label": "host health",
	}}
	again, err := RemoteApprovalToken(same)
	if err != nil || again != token {
		t.Fatalf("canonical task approval unstable: %q %v", again, err)
	}
	changed := Task{ID: id, Raw: map[string]any{
		"label": "host health", "type": "process", "command": "/bin/sh",
		"args": []any{"-c", "unexpected"}, "taskdeckRemoteAllowed": true,
	}}
	if _, err := RemoteApprovalToken(changed); err == nil {
		t.Fatal("changed task definition retained an old approved menu ID")
	}
	task.SecretEnv = map[string]string{"TOKEN": "secret-ref"}
	if _, err := RemoteApprovalToken(task); err == nil {
		t.Fatal("remote approval allowed a task with host secret references")
	}
}
