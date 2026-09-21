package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestMobileUIProfileIsIsolatedFromDesktopPresentation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/mobile.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(app.layoutProfile==='mobile')",
		"mobile-drawer-open",
		"mobile-actions-open",
		"mobile-terminal-keys",
		"window.visualViewport",
		"env(safe-area-inset-top)",
		"env(safe-area-inset-bottom)",
		"['Ctrl+C','\\x03']",
		"['↑','\\x1b[A']",
		"['Enter','\\r']",
		"app.browserLeaseLost",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("mobile.js missing %q", want)
		}
	}

	loader, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), "import '/featuremods/mobile.js';") {
		t.Fatal("next.js must load mobile.js")
	}
}

func TestCoreUIDetectsStableLayoutProfileOncePerPage(t *testing.T) {
	for _, want := range []string{
		"const layoutProfile=(matchMedia('(pointer: coarse)').matches||matchMedia('(max-width: 900px)').matches)?'mobile':'desktop';",
		"document.documentElement.dataset.taskmenuLayout=layoutProfile;",
		"get layoutProfile(){return layoutProfile;}",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("app JS missing layout-profile behavior %q", want)
		}
	}
}
