package server

import (
	"bytes"
	"context"
	"errors"
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

func TestCompressedUploadOverwritePolicyAllowsExistingRoots(t *testing.T) {
	commands := fileTransferManualExtractCommandsWithPolicy("tar.gz", "/tmp/archive.tar.gz", "/srv/app", []string{"assets"}, "overwrite")
	if strings.Contains(commands["posix"], "test ! -e") {
		t.Fatalf("overwrite POSIX command still blocks existing roots: %s", commands["posix"])
	}
	if strings.Contains(commands["powershell"], "Test-Path -LiteralPath (Join-Path") {
		t.Fatalf("overwrite PowerShell command still blocks existing roots: %s", commands["powershell"])
	}
	command, err := remoteArchiveExtractCommandWithPolicy("tar.gz", "/tmp/archive.tar.gz", "/srv/app", []string{"assets"}, "overwrite")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "tar -xzf") || strings.Contains(command, "test ! -e") {
		t.Fatalf("overwrite extract command=%s", command)
	}
}

func TestCompressedUploadMergePolicyValidation(t *testing.T) {
	for _, value := range []string{"", "fail", "overwrite"} {
		if _, err := normalizeFileTransferArchiveMergePolicy(value); err != nil {
			t.Fatalf("merge policy %q rejected: %v", value, err)
		}
	}
	if _, err := normalizeFileTransferArchiveMergePolicy("unsafe"); err == nil {
		t.Fatal("unsupported merge policy accepted")
	}
}

func TestLegacyCompressedUploadRouteDoesNotUseProjectArchiveLimits(t *testing.T) {
	source, err := os.ReadFile("file_transfer_archive_upload.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "writeProjectTarGz(tmp, sources)") {
		t.Fatal("compressed upload must not reuse Project Archive entry/resource limits")
	}
	if !strings.Contains(text, "writeLargeFileTransferTarGz(r.Context(), tmp, sources, nil)") {
		t.Fatal("legacy compressed upload route must use the large streaming writer")
	}
}

func TestLargeCompressedUploadWriterCanBeCancelled(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 32; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f-%03d.txt", i)), []byte(strings.Repeat("x", 4096)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err = writeLargeFileTransferTarGz(ctx, &out, []projectArchiveSource{{Virtual: "many", Path: root, Info: info, Base: "many"}}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled huge-folder writer error=%v", err)
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
