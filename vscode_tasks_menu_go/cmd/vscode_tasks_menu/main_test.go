package main

import (
	"net"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func TestCreateListenerFromInheritedFDKeepsAddress(t *testing.T) {
	original, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer original.Close()
	provider, ok := original.(interface{ File() (*os.File, error) })
	if !ok { t.Skip("listener has no File method") }
	file, err := provider.File()
	if err != nil { t.Fatal(err) }
	fd := int(file.Fd())
	inherited, err := createListener(config.Default(), fd, "")
	if err != nil { file.Close(); t.Fatal(err) }
	defer inherited.Close()
	// createListener takes ownership of the descriptor represented by file.
	if inherited.Addr().String() != original.Addr().String() {
		t.Fatalf("inherited addr=%s want %s", inherited.Addr(), original.Addr())
	}
}

func TestSelfUpdateRestoresTerminalsBeforeCompletedState(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	restore := strings.Index(src, "RestoreProjectTerminalsForStartup()")
	completed := strings.Index(src, `selfupdate.Update(ws, updateID, "completed"`)
	if restore < 0 {
		t.Fatal("serveForeground must restore project terminals during self-update startup")
	}
	if completed < 0 {
		t.Fatal("serveForeground must publish self-update completion")
	}
	if restore > completed {
		t.Fatal("self-update completion must not be published before terminal restore")
	}
}
