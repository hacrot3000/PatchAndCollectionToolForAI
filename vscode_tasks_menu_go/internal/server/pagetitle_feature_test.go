package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPageTitleFeatureUsesWorkspaceConfigAPI(t *testing.T) {
	js, err := webassets.Files.ReadFile("featuremods/pagetitle.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(js)
	for _, want := range []string{
		"/api/config/page-title",
		"method:'PUT'",
		"vscode_tasks_menu.ini",
		"localStorage.removeItem(key)",
		"document.title=String(data?.title||'').trim()||defaultTitle",
		"button.onclick=()=>editConfiguredTitle().catch(app.showError)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("pagetitle.js missing %q", want)
		}
	}
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/pagetitle.js';") {
		t.Fatal("next.js does not load pagetitle.js")
	}
}
