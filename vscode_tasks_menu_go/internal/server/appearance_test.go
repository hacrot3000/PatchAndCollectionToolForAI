package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAppearanceControlsFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/appearance.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"appearance-controls",
		"Dark",
		"Light",
		"Default Mono",
		"DejaVu Mono",
		"Liberation Mono",
		"terminal-font-size",
		"view.term.options.fontSize",
		"view.term.options.fontFamily",
		"view.term.options.theme",
		"localStorage.setItem",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("appearance.js missing behavior %q", want)
		}
	}
}
