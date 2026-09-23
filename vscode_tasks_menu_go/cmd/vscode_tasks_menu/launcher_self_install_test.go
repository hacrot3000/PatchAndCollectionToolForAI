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

func TestInstallScriptBuildsVersionedTaskdeckRelease(t *testing.T) {
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
		`APP_ROOT="${TASKDECK_APP_DIR:-${HOME}/.local/lib/taskdeck}"`,
		`RELEASES_DIR="$APP_ROOT/releases"`,
		`CURRENT_LINK="$APP_ROOT/current"`,
		`FINAL_RELEASE="$RELEASES_DIR/$RELEASE_ID"`,
		"python3 test_python_patch_entry.py",
		"-buildvcs=false",
		"./cmd/vscode_tasks_menu",
		`$STAGED_RELEASE/patchtool/python_patch_entry.py`,
		`cp -a "$SOURCE_ROOT/_patch_lib" "$STAGED_RELEASE/patchtool/_patch_lib"`,
		`atomic_symlink "$FINAL_RELEASE" "$CURRENT_LINK"`,
		`atomic_symlink "$CURRENT_LINK/taskdeck" "$TARGET"`,
		"$TARGET.revision",
	} {
		if !strings.Contains(src, want) { t.Fatalf("install.sh missing %q", want) }
	}
}
