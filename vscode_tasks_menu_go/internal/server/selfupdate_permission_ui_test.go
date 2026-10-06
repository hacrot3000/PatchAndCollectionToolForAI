package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSelfUpdateUIPreflightsGranularPermissions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdate.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function canCheckSelfUpdate()",
		"function canRunSelfUpdate()",
		"app.hasPermission?.('selfupdate.check')",
		"app.hasPermission?.('selfupdate.run')",
		"checkUpdate.hidden=!canCheck",
		"if(!canCheckSelfUpdate())throw new Error('Self-update check permission is required')",
		"if(!canRunSelfUpdate())",
		"selfupdate.run permission is required to install it",
		"if(!canCheckSelfUpdate())return",
		"if(!canRunSelfUpdate())throw new Error('Self-update run permission is required')",
		"canCheck:canCheckSelfUpdate",
		"canRun:canRunSelfUpdate",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("selfupdate.js missing granular permission preflight %q", want)
		}
	}
}
