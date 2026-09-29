package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRunningAndCompletedTabsUseCompactStatusIcons(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Number.isFinite(elapsed)&&elapsed>600",
		"view.status.classList.add('session-status-running-long')",
		"view.status.title='Running · '+duration",
		"view.status.textContent='✓'",
		"view.status.classList.add('session-status-success')",
		"view.status.title='PASS · '+duration",
		"view.status.textContent='✕'",
		"view.status.classList.add('session-status-fail')",
		"view.status.title='FAIL · '+duration",
		"const LONG_RUNNING_BRAILLE_FRAMES=['⠋','⠙','⠹','⠸','⠼','⠴','⠦','⠧','⠇','⠏']",
		"const LONG_RUNNING_BRAILLE_FRAME_MS=3000",
		"function renderLongRunningIndicator(status)",
		"indicator.className='session-running-braille'",
		"indicator.textContent=LONG_RUNNING_BRAILLE_FRAMES[longRunningBrailleFrameIndex]",
		"setInterval(updateLongRunningBrailleFrame,LONG_RUNNING_BRAILLE_FRAME_MS)",
		"prefersReducedRunningMotion()",
		"renderLongRunningIndicator(view.status)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("all.js missing compact tab status behavior %q", want)
		}
	}
	if strings.Contains(js, "view.status.textContent=meta.status==='running'?duration:state.text+' '+duration;") {
		t.Fatal("tab status still uses the old running/completed text presentation")
	}
	if strings.Contains(js, "taskdeck-tab-running-spin") {
		t.Fatal("long-running tab status must not use a continuously rotating CSS spinner")
	}
	if strings.Contains(js, "session-running-cell") || strings.Contains(js, "taskdeck-tab-running-cell-") {
		t.Fatal("long-running tab status must not use the old three-cell box indicator")
	}
}

func TestDetectedFilePathCompactionKeepsFilename(t *testing.T) {
	data, err := webassets.Files.ReadFile("features.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function detectedPathMaxChars()",
		"function compactDetectedPath(value,maxChars=detectedPathMaxChars())",
		"const filename=parts.pop()||path",
		"const context=parts.slice(-2)",
		"return lead+firstShown+'/'+lastShown+'/'+filename",
		"link.textContent=compactDetectedPath(file.path)",
		"link.title=file.path",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing long detected-path behavior %q", want)
		}
	}
	if strings.Contains(js, "link.textContent=file.path;link.title=file.path") {
		t.Fatal("detected file display still renders the full path directly and relies on end ellipsis")
	}
}
