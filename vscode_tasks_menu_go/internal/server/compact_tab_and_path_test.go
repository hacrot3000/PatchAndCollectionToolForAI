package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRunningAndCompletedTabsUseConfigurableCompactStatusIcons(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Number.isFinite(elapsed)&&elapsed>600",
		"let runningIndicatorSettings={mode:'boxes',rpm:2}",
		"function normalizeRunningIndicatorSettings(value)",
		"function applyRunningIndicatorSettings(value)",
		"taskmenu:running-indicator-settings",
		"const mode=runningIndicatorSettings.mode",
		"mode==='time'",
		"mode==='spinner'",
		"'session-running-spinner'",
		"(60/runningIndicatorSettings.rpm)+'s'",
		"mode==='braille'",
		"'session-running-braille'",
		"--taskdeck-running-braille-duration",
		"@keyframes taskdeck-tab-running-braille",
		"⠋",
		"⠏",
		"Math.round(Number(value?.rpm)||2)",
		"'session-running-boxes'",
		"cell.className='session-running-cell'",
		"status.dataset.runningIndicatorMode===mode",
		"indicator=status.firstElementChild",
		"if(!reusable)",
		"status.dataset.runningIndicatorMode=mode",
		"@keyframes taskdeck-tab-running-spin",
		"@keyframes taskdeck-tab-running-cell-2{0%,100%{opacity:.16}20%,80%{opacity:1}}",
		"@keyframes taskdeck-tab-running-cell-3{0%,39%,61%,100%{opacity:.16}40%,60%{opacity:1}}",
		"view.status.classList.add('session-status-running-long')",
		"view.status.title='Running · '+duration",
		"view.status.textContent='✓'",
		"view.status.classList.add('session-status-success')",
		"view.status.title='PASS · '+duration",
		"view.status.textContent='✕'",
		"view.status.classList.add('session-status-fail')",
		"view.status.title='FAIL · '+duration",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("all.js missing configurable compact tab status behavior %q", want)
		}
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
