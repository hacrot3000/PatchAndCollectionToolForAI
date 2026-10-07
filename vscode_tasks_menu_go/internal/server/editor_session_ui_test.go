package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestEditorSessionPersistenceFrontendContract(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const editorSessionPersistDelayMS=250",
		"function localEditorViewsInTabOrder()",
		"function editorSessionPayload()",
		"content:view.dirty?view.cm.state.doc.toString():''",
		"source_sha256:String(view.file?.sha256||'')",
		"selection:{anchor:selection.anchor,head:selection.head}",
		"function persistEditorSessionNow()",
		"'/api/project/editor-session'",
		"function scheduleEditorSessionPersist()",
		"function restoreEditorSelection(view,selection)",
		"async function restorePersistedEditorSession()",
		"file.content=String(item.content??'')",
		"file.sha256=String(item.source_sha256)",
		"Recovered unsaved swap; source file changed on disk since the swap was written.",
		"restoreEditorSelection(view,item?.selection)",
		"TaskMenuTabOrder?.applyOrder?.(restoredOrder)",
		"persistSession:persistEditorSessionNow",
		"restorePersistedSession:restorePersistedEditorSession",
		"setTimeout(()=>restorePersistedEditorSession()",
		"function persistEditorSessionKeepalive()",
		"app.fetchWithLease('/api/project/editor-session'",
		"keepalive:true",
		"window.addEventListener('pagehide',()=>persistEditorSessionKeepalive())",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor session persistence contract missing %q", want)
		}
	}
}

func TestEditorSessionPersistenceExcludesRemoteWorkspaceEditors(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"!view.file?.remote_workspace_id",
		"view.file?.remote_workspace_id||seen.has(id)",
		"if(!view.file?.remote_workspace_id)scheduleEditorSessionPersist()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("local editor session scope missing %q", want)
		}
	}
}

func TestSelfUpdateFlushesEditorSessionBeforeStarting(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/selfupdate.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	flush := strings.Index(js, "await globalThis.TaskMenuEditor?.persistSession?.();")
	start := strings.Index(js, "endpoint+'&action=start'")
	if flush < 0 || start < 0 || flush >= start {
		t.Fatalf("editor session must flush before self-update start: flush=%d start=%d", flush, start)
	}
}

func TestTabReorderPersistsEditorSessionOrder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabdrag.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	save := strings.Index(js, "function saveOrder()")
	persist := strings.Index(js, "globalThis.TaskMenuEditor?.persistSession?.();")
	if save < 0 || persist < save {
		t.Fatalf("tab reorder editor persistence missing: save=%d persist=%d", save, persist)
	}
}
