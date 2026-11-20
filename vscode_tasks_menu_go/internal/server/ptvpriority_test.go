package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPatchToolArtifactPriorityFeature(t *testing.T) {
	js, err := webassets.Files.ReadFile("featuremods/ptvpriority.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(js)
	for _, want := range []string{
		"/artifacts/ptv_to_ai/",
		"/^CR_[^/]+\\.(?:zip|txt)$/i",
		"/^FH_[^/]+\\.(?:zip|txt)$/i",
		"/^AS_[^/]+\\.(?:zip|txt)$/i",
		"/^UP_[^/]+\\.(?:zip|txt)$/i",
		"CODE_COLLECTION_RESULT_",
		"FAIL_HANDOFF_",
		"AI_TOOL_SYNC_RESULT_",
		"MANUAL_EXECUTION_RESULT_",
		"observer.observe(bar,{childList:true})",
		"requestAnimationFrame",
		"kind.textContent!==role.label",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("ptvpriority.js missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"observer.observe(document.body",
		"{childList:true,subtree:true}",
		"window.addEventListener('taskmenu:output'",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("ptvpriority.js contains browser-hot-path observer/listener %q", forbidden)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/ptvpriority.js';") {
		t.Fatal("next.js does not load ptvpriority.js")
	}
}
