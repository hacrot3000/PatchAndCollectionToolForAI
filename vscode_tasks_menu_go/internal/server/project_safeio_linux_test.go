//go:build linux

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPinnedProjectFileRejectsParentSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "file.txt")
	if err := os.WriteFile(path, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "sub-moved")
	if err := os.Rename(sub, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sub); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if pinned, err := openProjectPinnedFile(root, canonical); err == nil {
		pinned.close()
		t.Fatal("pinned open accepted swapped parent symlink")
	}
}

func TestPinnedProjectFileRejectsTargetSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	pinned, err := openProjectPinnedFile(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.close()

	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, _, err := pinned.readCurrent(projectEditableLimit); err == nil {
		t.Fatal("pinned read accepted swapped target symlink")
	}
}

func TestPinnedProjectFileAtomicWriteAndCommit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	pinned, err := openProjectPinnedFile(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.close()
	current, info, err := pinned.readCurrent(projectEditableLimit)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(current)
	if hex.EncodeToString(sum[:]) == "" {
		t.Fatal("unexpected empty hash")
	}
	if err := pinned.writeTemp([]byte("after\n"), info); err != nil {
		t.Fatal(err)
	}
	if err := pinned.commitTemp(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("saved=%q", got)
	}
	savedInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if savedInfo.Mode().Perm() != 0o640 {
		t.Fatalf("mode=%o want 640", savedInfo.Mode().Perm())
	}
}


func TestPinnedProjectFilePreservesUserXattr(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const name = "user.vscode_tasks_menu_test"
	if err := syscall.Setxattr(path, name, []byte("keep"), 0); err != nil {
		if err == syscall.ENOTSUP || err == syscall.EOPNOTSUPP || err == syscall.EPERM {
			t.Skipf("user xattr unavailable: %v", err)
		}
		t.Fatal(err)
	}
	pinned, err := openProjectPinnedFile(root, path)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.close()
	_, info, err := pinned.readCurrent(projectEditableLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err := pinned.writeTemp([]byte("after\n"), info); err != nil {
		t.Fatal(err)
	}
	if err := pinned.commitTemp(); err != nil {
		t.Fatal(err)
	}
	size, err := syscall.Getxattr(path, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := make([]byte, size)
	n, err := syscall.Getxattr(path, name, value)
	if err != nil {
		t.Fatal(err)
	}
	if string(value[:n]) != "keep" {
		t.Fatalf("xattr=%q want keep", value[:n])
	}
}
