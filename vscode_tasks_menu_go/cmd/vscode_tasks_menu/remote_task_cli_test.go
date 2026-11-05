package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestHostRemoteTaskCLIListsOnlyReviewedProcessCandidates(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".vscode"), 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"version":"2.0.0","tasks":[
	{"label":"approved health","type":"process","command":"/bin/echo","args":["OK"],"taskdeckRemoteAllowed":true},
	{"label":"do not approve shell","type":"shell","command":"echo test","taskdeckRemoteAllowed":true},
	{"label":"unmarked process","type":"process","command":"/bin/echo","args":["NO"]}
	]}`
	if err := os.WriteFile(filepath.Join(workspace, ".vscode", "tasks.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := tasks.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := tasks.RemoteApprovalToken(loaded[0])
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = old }()
	runErr := runHostRemoteTaskCLI([]string{"--workspace", workspace, "list"})
	writer.Close()
	os.Stdout = old
	output, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || runErr != nil {
		t.Fatalf("host CLI: %v read: %v", runErr, err)
	}
	got := string(output)
	if !strings.Contains(got, approved) || !strings.Contains(got, "approved health") {
		t.Fatalf("approved full-digest task missing: %s", got)
	}
	if strings.Contains(got, "do not approve shell") || strings.Contains(got, "unmarked process") {
		t.Fatalf("host CLI proposed an unapproved task: %s", got)
	}
}
