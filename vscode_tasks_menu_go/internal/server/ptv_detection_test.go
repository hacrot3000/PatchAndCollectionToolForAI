package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPTVDetectionStripsANSIAndKeepsPrimaryAliases(t *testing.T) {
	workspace := t.TempDir()
	outDir := filepath.Join(workspace, "artifacts", "ptv_to_ai")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(outDir, "CR_69d1f28c.zip")
	txtPath := filepath.Join(outDir, "CR_69d1f28c.txt")
	for _, path := range []string{zipPath, txtPath} {
		if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Patch Tool v6.18.6+ deliberately decorates upload-required paths on a TTY
	// with SGR background/underline controls. Xterm renders these invisibly, but
	// raw PTY detection must remove them before filepath parsing.
	console := strings.Join([]string{
		"\x1b[103m\x1b[30m!!! [PRIMARY - UPLOAD THIS FILE] !!!\x1b[0m",
		">>> ACTION REQUIRED: UPLOAD TO CHATGPT / AI SERVER <<<",
		"ZIP (preferred) — copy path below:",
		"\x1b[103m\x1b[4m" + zipPath + "\x1b[0m",
		"Clear-text TXT — copy path below:",
		"\x1b[103m\x1b[4m" + txtPath + "\x1b[0m",
	}, "\n")

	files := (&Server{Workspace: workspace}).downloadableFilesFromText(console)
	if len(files) != 2 {
		t.Fatalf("files = %#v, want ANSI-wrapped ZIP/TXT", files)
	}
	if files[0].Path != zipPath || files[1].Path != txtPath {
		t.Fatalf("files = %#v, want %q then %q", files, zipPath, txtPath)
	}
}

func TestPTVDetectionKeepsNewestWindowForLongConsole(t *testing.T) {
	workspace := t.TempDir()
	outDir := filepath.Join(workspace, "artifacts", "ptv_to_ai")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(outDir, "CR_tail1234.zip")
	if err := os.WriteFile(zipPath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	console := strings.Repeat("old noisy output that must not consume detection budget\n", 4000) +
		"!!! [PRIMARY - UPLOAD THIS FILE] !!!\n" +
		"\x1b[103m\x1b[4m" + zipPath + "\x1b[0m\n"
	files := (&Server{Workspace: workspace}).downloadableFilesFromText(console)
	if len(files) != 1 || files[0].Path != zipPath {
		t.Fatalf("files = %#v, want recent primary artifact %q", files, zipPath)
	}
}

func TestStripTerminalControlSequencesHandlesOSCAndCSI(t *testing.T) {
	got := stripTerminalControlSequences("before \x1b[31mred\x1b[0m \x1b]8;;https://example.invalid\x07link\x1b]8;;\x07 after")
	if got != "before red link after" {
		t.Fatalf("stripped = %q", got)
	}
}
