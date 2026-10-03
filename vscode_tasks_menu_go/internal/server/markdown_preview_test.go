package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
