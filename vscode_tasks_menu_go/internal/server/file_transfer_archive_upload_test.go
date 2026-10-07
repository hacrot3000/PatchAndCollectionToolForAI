package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTransferUploadQuickScanStopsAtManyFileThreshold(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < fileTransferUploadQuickScanFiles+15; i++ {
		name := filepath.Join(root, "file-"+strings.Repeat("0", 4-len(strings.TrimSpace(""))))
		_ = name
		path := filepath.Join(root, "f-"+formatTestIndex(i)+".txt")
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

func formatTestIndex(value int) string {
	const digits = "0123456789"
	if value == 0 {
		return "0000"
	}
	buf := []byte{'0', '0', '0', '0'}
	for i := len(buf)-1; i >= 0 && value > 0; i-- {
		buf[i] = digits[value%10]
		value /= 10
	}
	return string(buf)
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
