package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestQuickOpenUsesBackendBoundedSearchAndKeyboardNavigation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/files/search?q=",
		"&limit=50",
		"AbortController",
		"setTimeout(()=>search(query,currentSeq),80)",
		"ArrowDown",
		"ArrowUp",
		"event.key==='Enter'",
		"globalQuickOpenShortcut",
		"taskmenu:project-file-open-request",
		"data.results.slice(0,50)",
		"fuzzyNameIndexes(name,query)",
		"quick-open-match",
		"renderHighlightedName(name",
		"hasExactFilename(query)",
		"&refresh=1",
		"setTimeout(()=>{",
		"prefixSpec(input.value).query===spec.query",
		"function localQuickOpenItems()",
		"TaskMenuExplorer?.recent",
		"TaskMenuEditor?.editors",
		"TaskMenuDatabase?.views",
		"TaskMenuFileTransfer?.views",
		"TaskMenuConnections?.sshProfiles",
		"TaskMenuDatabase?.profiles",
		"TaskMenuFileTransfer?.profiles",
		"function prefixSpec(raw)",
		"if(value.startsWith('>'))",
		"if(value.startsWith('@'))",
		"if(value.startsWith('#'))",
		"if(lower.startsWith('git:'))",
		"if(lower.startsWith('ssh:'))",
		"TaskMenuCommandPalette?.commandList?.()",
		"TaskMenuProjectSymbols?.open?.({query:spec.query})",
		"TaskMenuProjectSearch?.open?.({query:spec.query})",
		"TaskMenuGitFiles?.openGraphSearch?.(spec.query)",
		"document.querySelectorAll('#tabs .tab,[role=\"tab\"]')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("quickopen.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/project/tree") {
		t.Fatal("Quick Open must not fetch the project tree")
	}
}

func TestQuickOpenIsLoadedBeforeGroupedMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	quick := strings.Index(js, "quickopen.js")
	menus := strings.Index(js, "menus.js")
	if quick < 0 || menus < 0 || quick > menus {
		t.Fatalf("next.js must load quickopen before menus: %q", js)
	}
}


func TestQuickOpenShortcutIsGlobalCaptureAndExact(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function globalQuickOpenShortcut(event)",
		"const primary=event.ctrlKey||event.metaKey",
		"!primary||event.shiftKey||event.altKey||event.key.toLowerCase()!=='p'",
		"event.preventDefault()",
		"event.stopPropagation()",
		"window.addEventListener('keydown',globalQuickOpenShortcut,true)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Quick Open global shortcut contract missing %q", want)
		}
	}
}

func TestGlobalQuickOpenKeepsOneCtrlPOverlayAndPrefixRouting(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Global Quick Open — files, recent, tabs, terminals, DB, connections",
		"kind:'recent'",
		"kind:'terminal'",
		"kind:'database'",
		"kind:'ssh'",
		"kind:'transfer'",
		"kind:'command'",
		"kind:'symbol'",
		"kind:'text'",
		"kind:'git'",
		"globalThis.TaskMenuQuickOpen={open,close,localQuickOpenItems,prefixSpec}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("global Quick Open contract missing %q", want)
		}
	}
	if strings.Count(js, "window.addEventListener('keydown',globalQuickOpenShortcut,true)") != 1 {
		t.Fatal("Global Quick Open must have exactly one Ctrl/Cmd+P shortcut owner")
	}
}
