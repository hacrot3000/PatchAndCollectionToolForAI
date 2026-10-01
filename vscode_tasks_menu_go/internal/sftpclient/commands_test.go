package sftpclient

import (
	"strings"
	"testing"
)

func TestQuotePathEscapesQuotesBackslashesAndGlobs(t *testing.T) {
	got, err := QuotePath(`/srv/my "file" [1]*?.txt`)
	if err != nil {
		t.Fatal(err)
	}
	want := `"/srv/my \"file\" \[1\]\*\?.txt"`
	if got != want {
		t.Fatalf("QuotePath=%q want %q", got, want)
	}
}

func TestQuotePathRejectsControlCharacters(t *testing.T) {
	if _, err := QuotePath("/srv/a\nb"); err == nil {
		t.Fatal("expected newline rejection")
	}
}

func TestCommandsQuoteAllPaths(t *testing.T) {
	got, err := RenameCommand("/old file", "/new [file]")
	if err != nil {
		t.Fatal(err)
	}
	if got != "rename \"/old file\" \"/new \\[file\\]\"\n" {
		t.Fatalf("rename command=%q", got)
	}
}

func TestParseLongList(t *testing.T) {
	input := strings.Join([]string{
		"sftp> ls -lan \"/srv/app\"",
		"drwxr-xr-x    4 1000 1000       4096 Sep 30 13:15 .",
		"drwxr-xr-x    8 0    0          4096 Sep 29 2026 ..",
		"-rw-r--r--    1 1000 1000       1234 Sep 30 12:01 app config.txt",
		"drwxr-xr-x    2 1000 1000       4096 Sep 29 2026 uploads",
		"lrwxrwxrwx    1 1000 1000         12 Sep 29 2026 current -> releases/v2",
	}, "\n")
	entries, err := ParseLongList(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries=%#v", entries)
	}
	if entries[0].Name != "app config.txt" || entries[0].Type != "file" || entries[0].Size != 1234 {
		t.Fatalf("file=%#v", entries[0])
	}
	if entries[1].Name != "uploads" || entries[1].Type != "directory" {
		t.Fatalf("dir=%#v", entries[1])
	}
	if entries[2].Name != "current" || entries[2].Type != "symlink" {
		t.Fatalf("symlink=%#v", entries[2])
	}
}
