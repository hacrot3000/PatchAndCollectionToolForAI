package identity

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveDBPathAcceptsConfiguredAbsolutePath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "identity.db")
	got, err := ResolveDBPath(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(want) {
		t.Fatalf("path=%q want=%q", got, filepath.Clean(want))
	}
}

func TestResolveDBPathRejectsRelativeConfiguredPath(t *testing.T) {
	if _, err := ResolveDBPath("state/identity.db"); err == nil {
		t.Fatal("relative identity DB path accepted")
	}
}

func TestResolveDBPathDefaultIsAbsoluteAndOutsideWorkspaceConcept(t *testing.T) {
	got, err := ResolveDBPath("")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("default identity DB path is not absolute: %q", got)
	}
	if filepath.Base(got) != identityDBName {
		t.Fatalf("default DB basename=%q want=%q", filepath.Base(got), identityDBName)
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(strings.ToLower(got), "taskdeck") {
			t.Fatalf("Windows default path missing TaskDeck directory: %q", got)
		}
	case "darwin":
		if !strings.Contains(got, filepath.Join("Application Support", "TaskDeck")) {
			t.Fatalf("macOS default path unexpected: %q", got)
		}
	default:
		if !strings.Contains(got, filepath.Join("taskdeck", identityDBName)) {
			t.Fatalf("Unix default path unexpected: %q", got)
		}
	}
}

func TestEnsureDBParentCreatesDedicatedDirectory(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "nested", "taskdeck", identityDBName)
	if err := EnsureDBParent(dbPath); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Dir(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("identity DB parent was not created as a directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("created identity directory mode=%#o should not grant group/other access", info.Mode().Perm())
	}
}

func TestEnsureDBParentRejectsSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(root, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDBParent(filepath.Join(linkDir, identityDBName)); err == nil {
		t.Fatal("symlink identity DB parent accepted")
	}
}
