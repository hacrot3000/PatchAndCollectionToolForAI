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
		"state.recent=''",
		"taskmenu:file-detection-changed",
		"if(!state.fileDetectionEnabled)return;",
		"if(!state.fileDetectionEnabled||state.gitTaskOutput||state.suppressGitOutput)return;",
		"||!state.fileDetectionEnabled||",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing per-tab file detection behavior %q", want)
		}
	}

	urlDetect := strings.Index(js, "for(const url of detectURLs(text))")
	fileGate := strings.Index(js, "if(!state.fileDetectionEnabled)return;")
	backendCall := strings.Index(js, "'/api/files/selection'")
	if urlDetect < 0 || fileGate < 0 || backendCall < 0 {
		t.Fatal("cannot locate file detection flow")
	}
	if !(urlDetect < fileGate && fileGate < backendCall) {
		t.Fatalf("URL detection must remain before the file-detection OFF gate: url=%d gate=%d backend=%d", urlDetect, fileGate, backendCall)
	}
}
