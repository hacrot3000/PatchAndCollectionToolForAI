package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRunningIndicatorSettingsFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/runningindicator.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/config/running-indicator",
		"method:'PUT'",
		"button.id='running-indicator-settings'",
		"Running indicator…",
		"Step boxes · 1 → 2 → 3 → 2 → 1",
		"Circular spinner",
		"BRAILLE PATTERN DOTS",
		"Always show elapsed time",
		"Animation speed",
		"cycles / minute · whole number",
		"rpm.min='1'",
		"rpm.step='1'",
		"Math.round(Number(rpm.value))",
		"mode.value==='spinner'||mode.value==='braille'",
		"rpm.disabled=!enabled",
		"globalThis.TaskMenuRunningIndicator?.apply?.(settings)",
		"if(app.taskData?.workspace)bootstrap()",
		"window.addEventListener('taskmenu:tasks',bootstrap,{once:true})",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("runningindicator.js missing behavior %q", want)
		}
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(next), "import '/featuremods/runningindicator.js';") {
		t.Fatal("next.js must load runningindicator.js")
	}
}
