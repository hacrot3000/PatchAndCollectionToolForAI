package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticUIUsesTaskDeckBrand(t *testing.T) {
	if !strings.Contains(indexHTML, "<header><strong>TaskDeck</strong>") {
		t.Fatal("header must use TaskDeck brand")
	}
	if strings.Contains(indexHTML, "<strong>VS CODE TASKS</strong>") {
		t.Fatal("legacy VS CODE TASKS header brand must be removed")
	}
}

func TestStaticUIUsesOnlyLocalTerminalAssets(t *testing.T) {
	if strings.Contains(indexHTML, "cdn.jsdelivr.net") || strings.Contains(indexHTML, "https://") || strings.Contains(indexHTML, "http://") {
		t.Fatalf("index HTML must not depend on external browser assets")
	}
	for _, want := range []string{"/vendor/xterm.css", "/vendor/xterm.js", "/vendor/addon-fit.js"} {
		if !strings.Contains(indexHTML, want) {
			t.Fatalf("index HTML does not reference embedded asset %q", want)
		}
	}
}

func TestStaticUIEmbeddedAssetsAreServed(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
	}{
		{"/vendor/xterm.js", "application/javascript"},
		{"/vendor/addon-fit.js", "application/javascript"},
		{"/vendor/xterm.css", "text/css"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			staticUI(rr, req)
			res := rr.Result()
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
			}
			if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, tc.contentType) {
				t.Fatalf("content-type = %q, want prefix %q", got, tc.contentType)
			}
			if rr.Body.Len() < 100 {
				t.Fatalf("embedded asset unexpectedly small: %d bytes", rr.Body.Len())
			}
			if got := res.Header.Get("Cache-Control"); !strings.Contains(got, "immutable") {
				t.Fatalf("cache-control = %q, want immutable", got)
			}
		})
	}
}

func TestStaticUIMenuGroupsDefaultCollapsed(t *testing.T) {
	if !strings.Contains(appJS, "renderNode(buildTree(),menu,false)") {
		t.Fatalf("root menu groups must render collapsed by default")
	}
	if strings.Contains(appJS, "renderNode(buildTree(),menu,true)") {
		t.Fatalf("root menu groups unexpectedly render expanded by default")
	}
}

func TestStaticUIHasCopyConsoleAction(t *testing.T) {
	for _, want := range []string{
		"copy.className='copy-console'",
		"copy.textContent='📋 Copy console'",
		"copyConsole(view)",
		"view.term.buffer.active",
		"navigator.clipboard.writeText(text)",
		"view.stop.disabled=meta.status!=='running'",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("app JS missing copy-console behavior %q", want)
		}
	}
	if !strings.Contains(appCSS, ".copy-console{margin-left:6px") {
		t.Fatalf("copy console action must keep compact visual separation from Stop")
	}
	if strings.Contains(appJS, "view.stop.disabled=Boolean(view.tabReadOnly)") || strings.Contains(appJS, "view.stop.disabled=view.tabReadOnly") {
		t.Fatalf("terminal read-only must not disable Stop or utility actions")
	}
	if strings.Contains(appJS, "button:last-child") {
		t.Fatalf("Stop button must not be located by last-child after adding Copy console")
	}
}

func TestStaticUIHasWorkspaceTerminalAction(t *testing.T) {
	for _, want := range []string{
		`id="open-terminal"`,
		"async function startTerminal()",
		"JSON.stringify({kind:'terminal'})",
		"document.querySelector('#open-terminal').onclick",
		"meta.task_id===0?'Close terminal':'Stop'",
	} {
		if !strings.Contains(indexHTML+appJS, want) {
			t.Fatalf("terminal launch UI missing behavior %q", want)
		}
	}
}

func TestSharedUICoreActionsFollowCurrentUserCapabilities(t *testing.T) {
	for _, want := range []string{
		`id="identity" hidden`,
		`id="logout" hidden`,
		"fetch('/api/auth/me'",
		"function hasPermission(permission)",
		"!hasPermission('terminal.create')",
		"!hasPermission('tasks.run')",
		"meta.owner_user_id===currentUser?.user_id",
		"disableStdin:!canControl",
		"if(view.canControl&&!view.tabReadOnly&&!browserLeaseLost",
		"view.stop.hidden=!view.canControl",
		"fetch('/api/auth/logout'",
	} {
		if !strings.Contains(indexHTML+appJS, want) {
			t.Fatalf("shared capability UI missing %q", want)
		}
	}
}

func TestStaticUIHasPersistentEditablePageTitle(t *testing.T) {
	for _, want := range []string{
		`<title>TaskDeck</title>`,
		`id="edit-title"`,
		"const defaultPageTitle='TaskDeck'",
		"function pageTitleSuffix(value)",
		"function formattedPageTitle(value)",
		"return suffix?defaultPageTitle+' - '+suffix:defaultPageTitle",
		"'vscode-tasks-menu:page-title:'+taskData.workspace",
		"localStorage.getItem(pageTitleStorageKey())",
		"localStorage.setItem(pageTitleStorageKey(),title)",
		"localStorage.removeItem(pageTitleStorageKey())",
		"document.title=formattedPageTitle(saved)",
		"restorePageTitle();",
		"document.querySelector('#edit-title').onclick",
	} {
		if !strings.Contains(indexHTML+appJS, want) {
			t.Fatalf("editable page title UI missing behavior %q", want)
		}
	}
	openTerminal := strings.Index(indexHTML, `id="open-terminal"`)
	editTitle := strings.Index(indexHTML, `id="edit-title"`)
	if openTerminal == -1 || editTitle == -1 || editTitle < openTerminal {
		t.Fatalf("Edit title button must appear immediately after Open Terminal")
	}
}

func TestStaticUINotFoundForUnknownAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/vendor/not-present.js", nil)
	rr := httptest.NewRecorder()
	staticUI(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}


func TestStaticUIUsesCompactDesktopHeaders(t *testing.T) {
	for _, want := range []string{
		":root{--taskmenu-header-height:30px",
		"header{height:var(--taskmenu-header-height);display:flex;align-items:center;gap:7px;padding:0 8px",
		"header #workspace{opacity:.65;flex:0 1 auto;min-width:0;max-width:min(48vw,720px)",
		"main{display:grid;grid-template-columns:310px 1fr;height:calc(100vh - var(--taskmenu-header-height))",
		".pane-head{display:flex;gap:4px;align-items:center;min-height:24px;padding:1px 5px",
		"body>header>button{background:transparent;border:0",
		".pane-head>button{background:transparent;border:0",
		".pane-head>select{height:20px;min-height:20px;padding:1px 5px;font-size:10px;line-height:16px}",
		".terminal{flex:1;min-height:0;padding:4px",
	} {
		if !strings.Contains(appCSS, want) {
			t.Fatalf("compact desktop header CSS missing %q", want)
		}
	}
}


func TestStaticUISendsTerminalLineContextForPartialFileSelection(t *testing.T) {
	for _, want := range []string{
		"function logicalBufferLine(buffer,row)",
		"function selectionLineContext(view)",
		"view.term.getSelectionPosition?.()",
		"line.translateToString(true)",
		"const context=selectionLineContext(view)",
		"scanSelection(view,text,context,seq)",
		"context:context.slice(0,65536)",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("terminal partial-file context missing %q", want)
		}
	}
}

func TestStaticUIPreservesSelectionDuringDetectedFileResize(t *testing.T) {
	for _, want := range []string{
		"if(term.hasSelection()){",
		"view.resizePending=true",
		"term.onSelectionChange(()=>{",
		"if(!selected&&view.resizePending){",
		"requestAnimationFrame(applyResize)",
		"function downloadFilesKey(files)",
		"if(key===view.downloadFilesKey)return",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("terminal selection resize protection missing %q", want)
		}
	}
	resizeGuard := strings.Index(appJS, "if(term.hasSelection()){")
	fitCall := strings.Index(appJS[resizeGuard:], "applyResize();")
	if resizeGuard < 0 || fitCall < 0 {
		t.Fatal("terminal resize guard must run before resize application")
	}
}

func TestStaticUITerminalActivationCanSkipFocus(t *testing.T) {
	for _, want := range []string{
		"function focusView(id)",
		"function activateView(id,{focus=true,force=false}={})",
		"if(focus)v.term.focus()",
		"activateView,focusView,activateExternalView",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("terminal activation focus control missing %q", want)
		}
	}
}
