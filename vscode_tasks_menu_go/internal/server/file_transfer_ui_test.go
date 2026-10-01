package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestFileTransferWorkspaceUsesStructuredRemoteFileAPIs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/file-transfer/list",
		"/api/file-transfer/mutate",
		"/api/file-transfer/upload",
		"/api/file-transfer/download-ticket",
		"/api/file-transfer/download?ticket=",
		"app.fetchWithLease('/api/file-transfer/upload'",
		"New Folder",
		"Download",
		"Rename",
		"Delete",
		"directory removal is non-recursive",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filetransfer.js missing remote-file UI contract %q", want)
		}
	}
	if strings.Contains(js, "form.action='/api/file-transfer/download'") {
		t.Fatal("filetransfer.js regressed to lease-incompatible form POST downloads")
	}
}

func TestFileTransferModuleIsLoadedBeforeConnections(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	fileTransfer := strings.Index(js, "import '/featuremods/filetransfer.js';")
	connections := strings.Index(js, "import '/featuremods/connections.js';")
	if fileTransfer < 0 || connections < 0 || fileTransfer > connections {
		t.Fatalf("file-transfer module must load before Connections: filetransfer=%d connections=%d", fileTransfer, connections)
	}
}
