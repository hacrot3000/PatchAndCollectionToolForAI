package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestClearDetectedFilesFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/clearfiles.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"detected-file-controls",
		"detected-files-toggle",
		"Files: ON",
		"Files: OFF",
		"TaskMenuFileDetection",
		"api.setEnabled(view,!api.isEnabled(view))",
		"detected-clear-all",
		"Clear files",
		"bar.querySelector('.detected-ignore')",
		"ignore.click()",
		"guard<1024",
		"sessionMenu.after(controls)",
		"taskmenu:file-detection-changed",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("clearfiles.js missing behavior %q", want)
		}
	}
	if strings.Contains(js, "observe(document.body") {
		t.Fatal("detected file controls must not observe the whole document body")
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/clearfiles.js';") {
		t.Fatal("next.js must load clearfiles.js")
	}
}
