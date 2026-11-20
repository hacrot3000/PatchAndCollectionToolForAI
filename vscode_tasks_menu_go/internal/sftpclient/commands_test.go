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


func TestParseLongListNormalizesOpenSSHPathPrefixes(t *testing.T) {
	input := strings.Join([]string{
		`sftp> ls -lan "."`,
		"drwxr-xr-x    4 1000 1000       4096 Oct 01 10:00 ./.",
		"drwxr-xr-x    8 0    0          4096 Oct 01 10:00 ./..",
		"drwxr-xr-x    2 1000 1000       4096 Oct 01 10:00 ./folder",
		"-rw-r--r--    1 1000 1000         42 Oct 01 10:00 ./file.txt",
		"-rw-r--r--    1 1000 1000         43 Oct 01 10:00 /srv/root/absolute.txt",
	}, "\n")
	entries, err := ParseLongList(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries=%#v", entries)
	}
	got := []string{entries[0].Name, entries[1].Name, entries[2].Name}
	want := []string{"folder", "file.txt", "absolute.txt"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d name=%q want %q; entries=%#v", i, got[i], want[i], entries)
		}
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

func TestResumeCommandsQuotePaths(t *testing.T) {
	got, err := RegetCommand("/remote file.bin", "/tmp/local file.bin")
	if err != nil { t.Fatal(err) }
	if got != "reget \"/remote file.bin\" \"/tmp/local file.bin\"\n" {
		t.Fatalf("reget=%q", got)
	}
	got, err = ReputCommand("/tmp/local file.bin", "/remote file.bin")
	if err != nil { t.Fatal(err) }
	if got != "reput \"/tmp/local file.bin\" \"/remote file.bin\"\n" {
		t.Fatalf("reput=%q", got)
	}
}
