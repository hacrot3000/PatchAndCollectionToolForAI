package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTransferUploadQuickScanStopsAtManyFileThreshold(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < fileTransferUploadQuickScanFiles+15; i++ {
		path := filepath.Join(root, fmt.Sprintf("f-%04d.txt", i))
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := quickScanProjectArchiveSources([]projectArchiveSource{{Virtual: "many", Path: root, Info: info, Base: "many"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ManyFiles || result.Files != fileTransferUploadQuickScanFiles || result.Complete {
		t.Fatalf("quick scan result=%+v", result)
	}
}

func TestFileTransferArchiveRootsRejectUnsafeNames(t *testing.T) {
	for _, roots := range [][]string{
		{},
		{"../escape"},
		{"nested/name"},
		{"bad\\name"},
		{"line\nbreak"},
	} {
		if _, err := validateFileTransferArchiveRoots(roots); err == nil {
			t.Fatalf("unsafe roots accepted: %#v", roots)
		}
	}
	got, err := validateFileTransferArchiveRoots([]string{"alpha", "beta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("deduplicated roots=%#v", got)
	}
}

func TestFileTransferManualExtractCommandsPreflightAndCleanup(t *testing.T) {
	commands := fileTransferManualExtractCommands("tar.gz", "/srv/upload/.taskdeck-a.tar.gz", "/srv/upload", []string{"dir one", "dir'two"})
	posix := commands["posix"]
	for _, want := range []string{
		"test ! -e",
		"tar -xzf",
		"rm -f --",
		"/srv/upload/dir one",
	} {
		if !strings.Contains(posix, want) {
			t.Fatalf("POSIX command missing %q: %s", want, posix)
		}
	}
	powershell := commands["powershell"]
	for _, want := range []string{
		"Test-Path -LiteralPath",
		"tar -xzf $archive -C $dest",
		"Remove-Item -LiteralPath $archive",
	} {
		if !strings.Contains(powershell, want) {
			t.Fatalf("PowerShell command missing %q: %s", want, powershell)
		}
	}
}

func TestRemoteArchiveExtractIntoExistingCommandRejectsUnknownFormat(t *testing.T) {
	if _, err := remoteArchiveExtractIntoExistingCommand("rar", "/tmp/archive.rar", "/srv/app", []string{"assets"}); err == nil {
		t.Fatal("unsupported archive format was accepted")
	}
}

func TestRemoteArchiveExtractIntoExistingCommandPreventsOverwrite(t *testing.T) {
	command, err := remoteArchiveExtractIntoExistingCommand("tar.gz", "/tmp/archive.tar.gz", "/srv/app", []string{"assets", "index.html"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"test -d '/srv/app'",
		"test ! -e '/srv/app/assets'",
		"test ! -e '/srv/app/index.html'",
		"tar -xzf '/tmp/archive.tar.gz' -C '/srv/app'",
		"rm -f -- '/tmp/archive.tar.gz'",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("extract command missing %q: %s", want, command)
		}
	}
}
