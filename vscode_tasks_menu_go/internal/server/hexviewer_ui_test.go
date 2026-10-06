package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestHexViewerUIContracts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/hexviewer.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"/api/project/bytes?path=",
		"const DEFAULT_LIMIT=4096",
		"Bytes per page",
		"Previous chunk",
		"Next chunk",
		"Jump to byte offset",
		"0x hexadecimal value",
		"hex-values",
		"hex-ascii",
		"value>=32&&value<=126?String.fromCharCode(value):'.'",
		"taskmenu:project-hex-open-request",
		"app.activateExternalView(id,{force})",
		"TaskMenuHexViewer",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("hexviewer.js missing %q", want)
		}
	}
	for _, forbidden := range []string{"innerHTML", "eval(", "new Function("} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("hex viewer uses forbidden %q", forbidden)
		}
	}
}

func TestProjectFileActionsExposeHexViewer(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function openProjectHex(pathValue)",
		"taskmenu:project-hex-open-request",
		"openWithButton(dialog,'Hex',()=>openProjectHex(pathValue))",
		"label:'Open With…'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project file actions missing hex viewer integration %q", want)
		}
	}
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/hexviewer.js';") {
		t.Fatal("hex viewer module is not loaded")
	}
}
