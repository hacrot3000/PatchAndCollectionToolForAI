package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestMarkdownSelectedFileGetsRenderedPreviewKindAndContent(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "docs", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	content := "# UART tester\n\n- build\n- flash\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { t.Fatal(err) }

	files := (&Server{Workspace: workspace}).downloadableFilesFromText(path)
	if len(files) != 1 || files[0].PreviewKind != "markdown" {
		t.Fatalf("files=%+v want markdown preview kind", files)
	}

	info, _, err := (&Server{Workspace: workspace}).filePreviewInfo(path)
	if err != nil { t.Fatal(err) }
	if info.Kind != "markdown" || info.ProjectPath != "docs/README.md" {
		t.Fatalf("info=%+v", info)
	}
	if info.Content != content || !strings.HasPrefix(info.ContentType, "text/markdown") {
		t.Fatalf("content type/content mismatch: %+v", info)
	}
}

func TestMarkdownDetectionIsExtensionSpecific(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(path, []byte("# still plain text\n"), 0o644); err != nil { t.Fatal(err) }
	if got := previewFileHint(path); got != "text" {
		t.Fatalf("previewFileHint=%q want text", got)
	}
}

func utf16LEWithBOM(value string) []byte {
	runes := []rune(value)
	units := utf16.Encode(runes)
	out := []byte{0xFF, 0xFE}
	for _, unit := range units {
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}

func TestMarkdownPreviewSupportsUTF16LE(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "esp32_mainpcb_uart_tester", "BAO_CAO_MAINPCB_UART_NVS_20261003.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	content := "# Báo cáo UART NVS\r\n\r\n- Kết quả: PASS\r\n"
	raw := utf16LEWithBOM(content)
	if err := os.WriteFile(path, raw, 0o644); err != nil { t.Fatal(err) }

	if got := previewFileHint(path); got != "markdown" {
		t.Fatalf("previewFileHint=%q want markdown", got)
	}
	files := (&Server{Workspace: workspace}).downloadableFilesFromText(path)
	if len(files) != 1 || files[0].PreviewKind != "markdown" {
		t.Fatalf("files=%+v want markdown preview kind", files)
	}
	info, _, err := (&Server{Workspace: workspace}).filePreviewInfo(path)
	if err != nil { t.Fatal(err) }
	if info.Kind != "markdown" || info.Encoding != "utf-16le" || info.Content != content {
		t.Fatalf("info=%+v", info)
	}
}

func TestMarkdownPreviewSupportsWindows1252Fallback(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "report.md")
	raw := []byte{'#',' ','C','a','f',0xE9,'\n',0x93,'q','u','o','t','e',0x94,'\n'}
	if err := os.WriteFile(path, raw, 0o644); err != nil { t.Fatal(err) }

	if got := previewFileHint(path); got != "markdown" {
		t.Fatalf("previewFileHint=%q want markdown", got)
	}
	info, _, err := (&Server{Workspace: workspace}).filePreviewInfo(path)
	if err != nil { t.Fatal(err) }
	if info.Encoding != "windows-1252" || !strings.Contains(info.Content, "Café") || !strings.Contains(info.Content, "“quote”") {
		t.Fatalf("info=%+v", info)
	}
}

func TestMarkdownPreviewStillRejectsBinaryNUL(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "bad.md")
	if err := os.WriteFile(path, []byte{'#',' ',0,'x',1,2,3,0}, 0o644); err != nil { t.Fatal(err) }
	if got := previewFileHint(path); got != "" {
		t.Fatalf("previewFileHint=%q want empty for binary markdown", got)
	}
}
