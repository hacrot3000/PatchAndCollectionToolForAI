package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalSelectedFileViewActionUsesServerValidation(t *testing.T) {
	for _, want := range []string{
		"preview.textContent='View'",
		"preview.hidden=true",
		"file.preview_kind==='text'||file.preview_kind==='image'",
		"/api/files/preview?path=",
		"taskmenu:project-file-open-request",
		"source:'terminal-preview'",
		"taskmenu:file-image-preview",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("appJS missing selected-file preview behavior %q", want)
		}
	}
}

func TestImagePreviewPopupIsLoadedAndUsesValidatedURL(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filepreview.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"taskmenu:file-image-preview",
		"info.kind!=='image'",
		"image.src=info.url",
		"file-preview-image",
		"image.naturalWidth",
		"Image preview failed or the file changed.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("filepreview.js missing %q", want)
		}
	}
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "filepreview.js") {
		t.Fatal("next.js must load filepreview.js")
	}
}
