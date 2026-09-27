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
	if !strings.Contains(appCSS, ".copy-console{margin-left:24px") {
		t.Fatalf("copy console button must be visually separated from Stop")
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
		"if(view.canControl&&!browserLeaseLost",
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
		`id="edit-title"`,
		"const defaultPageTitle='VS Code Tasks Menu'",
		"'vscode-tasks-menu:page-title:'+taskData.workspace",
		"localStorage.getItem(pageTitleStorageKey())",
		"localStorage.setItem(pageTitleStorageKey(),title)",
		"localStorage.removeItem(pageTitleStorageKey())",
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
