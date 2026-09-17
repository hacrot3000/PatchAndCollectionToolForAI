package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTaskInputsFeatureModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/inputs.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"promptString",
		"pickString",
		"command type is not supported",
		"pendingByTask",
		"payload,inputs:values",
		"event.stopImmediatePropagation()",
		"app.startTask=runTaskWithInputs",
	} {
		if !strings.Contains(js, want) { t.Fatalf("inputs module missing %q", want) }
	}
}
