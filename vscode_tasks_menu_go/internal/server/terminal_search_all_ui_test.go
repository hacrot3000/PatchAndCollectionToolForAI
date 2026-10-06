package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTerminalSearchAllScrollbackUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalsearchall.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const maxScannedLines=50000",
		"const maxResults=300",
		"terminal-search-all-backdrop",
		"function terminalView(view)",
		"function visibleTerminalViews()",
		"function scan(query,currentGeneration)",
		"line.translateToString(true)",
		"scannedLines>=maxScannedLines||found.length>=maxResults",
		"view.term.select(item.column,item.row,item.length)",
		"view.term.scrollToLine(item.row)",
		"app.activateView(item.sessionID,{force:true})",
		"caseInput.addEventListener('change',schedule)",
		"button.className='console-search-all-btn'",
		"button.textContent='Search all terminals…'",
		"globalThis.TaskMenuTerminalSearchAll={open,close,scan,decorate",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminal-wide scrollback search missing %q", want)
		}
	}
	if strings.Contains(js, "/api/") {
		t.Fatal("terminal-wide scrollback search must stay within already visible browser terminal buffers")
	}

	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(next), "import '/featuremods/terminalsearchall.js';") {
		t.Fatal("terminal-wide scrollback search module is not loaded")
	}

	menus, err := webassets.Files.ReadFile("featuremods/menus.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(menus), "'.console-search-all-btn'") {
		t.Fatal("Console menu must expose terminal-wide scrollback search")
	}
}
