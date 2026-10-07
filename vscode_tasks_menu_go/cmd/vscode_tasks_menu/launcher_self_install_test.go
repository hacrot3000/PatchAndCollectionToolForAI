package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompatibilityLauncherMigratesToGlobalTaskdeck(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	launcherPath := filepath.Join(root, "vscode_tasks_menu")
	data, err := os.ReadFile(launcherPath)
	if os.IsNotExist(err) {
		t.Skip("root launcher is absent from legacy self-update source staging")
	}
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"TASKDECK_BIN=\"$INSTALL_DIR/taskdeck\"",
		"if [[ ! -x \"$TASKDECK_BIN\" ]]",
		"install_taskdeck",
		"install.sh",
		"exec \"$TASKDECK_BIN\" --workspace \"$PWD\" \"$@\"",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("compatibility launcher missing %q", want)
		}
	}
	for _, forbidden := range []string{"BUILD_DIR=\"$SRC_DIR/.build\"", "exec \"$BIN\""} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("compatibility launcher still contains local install behavior %q", forbidden)
		}
	}
}

func TestInstallScriptBuildsVersionedTaskdeckRelease(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if os.IsNotExist(err) {
		t.Skip("install.sh is absent in legacy self-update source staging")
	}
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		`APP_ROOT="${TASKDECK_APP_DIR:-${HOME}/.local/lib/taskdeck}"`,
		`RELEASES_DIR="$APP_ROOT/releases"`,
		`CURRENT_LINK="$APP_ROOT/current"`,
		`FINAL_RELEASE="$RELEASES_DIR/$RELEASE_ID"`,
		"PYTHON_310_AVAILABLE=0",
		"PYTHON_CMD=\"\"",
		"candidates=(python3.13 python3.12 python3.11 python3.10 python3 python)",
		"TASKDECK_PYTHON",
		"TaskDeck: dùng Python runtime:",
		"TaskDeck core vẫn sẽ được cài và có thể chạy bình thường",
		"if (( PYTHON_310_AVAILABLE )); then",
		"TASKDECK_INSTALL_FULL_VALIDATION",
		"installer dùng compile-only validation",
		"\"$PYTHON_CMD\" test_python_patch_entry.py",
		"\"$PYTHON_CMD\" -m py_compile python_patch_entry.py",
		"GOPROXY=off GOSUMDB=off go test ./...",
		"GOPROXY=off GOSUMDB=off go test -vet=off -run '^$' ./...",
		"Shared identity SQLite sẽ tự fallback sang sqlite3 CLI tương thích (SQLite 3.7+)",
		"Shared password hashing có thể dùng Python cũ nếu runtime đó có hashlib.scrypt",
		"self-update Python validation",
		"GOPROXY=off GOSUMDB=off go",
		"$SOURCE/internal/dbsqlite/sqlite_helper.py",
		"-buildvcs=false",
		"./cmd/vscode_tasks_menu",
		`$STAGED_RELEASE/patchtool/python_patch_entry.py`,
		`cp -a "$SOURCE_ROOT/_patch_lib" "$STAGED_RELEASE/patchtool/_patch_lib"`,
		`atomic_symlink "$FINAL_RELEASE" "$CURRENT_LINK"`,
		`atomic_symlink "$CURRENT_LINK/taskdeck" "$TARGET"`,
		"$TARGET.revision",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("install.sh missing %q", want)
		}
	}
	if strings.Contains(src, `die "Cần Python 3.10+`) {
		t.Fatal("install.sh must not abort installation when Python 3.10+ is unavailable")
	}
}
