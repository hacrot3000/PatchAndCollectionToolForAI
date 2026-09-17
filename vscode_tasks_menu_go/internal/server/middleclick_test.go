package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestMiddleClickPrimarySelectionPasteFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/middleclick.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"onSelectionChange",
		"primarySelection",
		"event.button===1",
		"auxclick",
		"view.term.paste(primarySelection)",
		"taskmenu:session",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("middleclick.js missing behavior %q", want)
		}
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/middleclick.js';") {
		t.Fatal("next.js must load middleclick.js")
	}
}
