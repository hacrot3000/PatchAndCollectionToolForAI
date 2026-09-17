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
		"detected-clear-all",
		"Clear files",
		"bar.querySelector('.detected-ignore')",
		"ignore.click()",
		"guard<1024",
		"sessionStorage",
		"sessionMenu.after(button)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("clearfiles.js missing behavior %q", want)
		}
	}
	if strings.Contains(js, "observe(document.body") {
		t.Fatal("clear detected files control must not observe the whole document body")
	}
	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/clearfiles.js';") {
		t.Fatal("next.js must load clearfiles.js")
	}
}
