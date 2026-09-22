package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestResizableSidebarFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"sidebar-resizer",
		"pointerdown",
		"pointermove",
		"gridTemplateColumns",
		"sidebar-width:",
		"dblclick",
		"localStorage.setItem",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("sidebar.js missing behavior %q", want)
		}
	}
}

func TestSidebarDoesNotInitializeInMobileProfile(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/sidebar.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "if(app.layoutProfile==='mobile'||initialized") {
		t.Fatal("desktop sidebar resize state must not initialize in mobile profile")
	}
}
