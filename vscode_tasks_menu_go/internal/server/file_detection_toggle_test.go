package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPerTabFileDetectionSuppression(t *testing.T) {
	data, err := webassets.Files.ReadFile("features.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"vscode-tasks-menu:file-detection:",
		"fileDetectionEnabled:loadFileDetectionEnabled(view)",
		"TaskMenuFileDetection={isEnabled:isFileDetectionEnabled,setEnabled:setFileDetectionEnabled}",
		"state.files.clear()",
		"state.urls.clear()",
		"state.recent=''",
		"taskmenu:file-detection-changed",
		"if(!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;",
		"||!state.fileDetectionEnabled||",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing per-tab file detection behavior %q", want)
		}
	}

	gate := strings.Index(js, "if(!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;")
	urlDetect := strings.Index(js, "for(const url of detectURLs(text))")
	backendCall := strings.Index(js, "'/api/files/selection'")
	if gate < 0 || urlDetect < 0 || backendCall < 0 {
		t.Fatal("cannot locate unified detection flow")
	}
	if !(gate < urlDetect && urlDetect < backendCall) {
		t.Fatalf("file/URL OFF gate must run before both URL detection and backend file scan: gate=%d url=%d backend=%d", gate, urlDetect, backendCall)
	}
}
