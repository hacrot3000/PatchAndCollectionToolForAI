package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibilityLauncherMigratesToGlobalTaskdeck(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil { t.Fatal(err) }
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "vscode_tasks_menu"))
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		"TASKDECK_BIN=\"$INSTALL_DIR/taskdeck\"",
		"if [[ ! -x \"$TASKDECK_BIN\" ]]",
		"install_taskdeck",
		"install.sh",
		"exec \"$TASKDECK_BIN\" --workspace \"$PWD\" \"$@\"",
	} {
		if !strings.Contains(src, want) { t.Fatalf("compatibility launcher missing %q", want) }
	}
	for _, forbidden := range []string{"BUILD_DIR=\"$SRC_DIR/.build\"", "exec \"$BIN\""} {
		if strings.Contains(src, forbidden) { t.Fatalf("compatibility launcher still contains local install behavior %q", forbidden) }
	}
}

func TestInstallScriptBuildsGlobalTaskdeck(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil { t.Fatal(err) }
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if os.IsNotExist(err) {
		t.Skip("install.sh is absent in legacy self-update source staging")
	}
	if err != nil { t.Fatal(err) }
	src := string(data)
	for _, want := range []string{
		"TARGET=\"$INSTALL_DIR/taskdeck\"",
		"go test ./...",
		"-buildvcs=false",
		"./cmd/vscode_tasks_menu",
		"mv -f \"$staged\" \"$TARGET\"",
		"$TARGET.revision",
	} {
		if !strings.Contains(src, want) { t.Fatalf("install.sh missing %q", want) }
	}
}
