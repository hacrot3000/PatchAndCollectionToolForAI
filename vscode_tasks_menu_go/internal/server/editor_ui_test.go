package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestEditorUsesVendoredCodeMirrorAndStaysOutsideTerminalSessions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"globalThis.cm6?.load",
		"cmFactory.newEditor",
		"dataset.viewKind='editor'",
		"app.activateExternalView(id,{force})",
		"/api/project/file?path=",
		"taskmenu:project-file-open-request",
		"contenteditable",
		"editor-tab",
		"editor-pane",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor.js missing %q", want)
		}
	}
	if strings.Contains(js, "app.views.set(") || strings.Contains(js, "/api/sessions") {
		t.Fatal("Editor must not register with terminal session broker")
	}
}


func TestEditorAutoOpenRespectsForegroundLockButTabClickForcesSwitch(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function activateEditor(id,{force=false}={})",
		"return app.activateExternalView(id,{force})",
		"tab.onclick=()=>activateEditor(id,{force:true})",
		"const view=await promise;activateEditor(view.id);return view",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor.js missing foreground-lock integration %q", want)
		}
	}
}

func TestEditorDeactivatesWhenAnotherExternalViewIsActivated(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(event.detail?.kind==='external'){",
		"if(editors.has(event.detail.id))activateEditorDOM(event.detail.id)",
		"else deactivateEditors()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor external-view deactivation missing %q", want)
		}
	}
	if strings.Contains(js, "if(event.detail?.kind==='external'&&editors.has(event.detail.id))activateEditorDOM(event.detail.id);") {
		t.Fatal("editor must also hide when SFTP/FTP, database, Patch, or another external view becomes active")
	}
}


func TestEditorInitialLanguageCoverageMapping(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"options.cpp=true",
		"options.go=true",
		"options.python=true",
		"options.javascript=true",
		"options.json=true",
		"options.yaml=true",
		"options.markdown=true",
		"options.html=true",
		"options.css=true",
		"options.xml=true",
		"options.java=true",
		"options.typescript=true",
		"options.jsx=true",
		"options.tsx=true",
		"return 'CMake'",
		"return 'Shell'",
		"return 'Lua'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor language mapping missing %q", want)
		}
	}
}


func TestEditorSupportsRecursiveSplitLayouts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"const editorSplitSupported=app.layoutProfile!=='mobile'",
		"#panes.editor-split-mode",
		"editor-split-resizer",
		"function splitEditorLeaf(firstID,secondID,orientation,ratio=.5)",
		"function layoutEditorSplitNode(node,rect,used)",
		"layoutEditorSplitNode(node.first",
		"layoutEditorSplitNode(node.second",
		"function renderEditorSplit(root)",
		"function syncEditorSplitForActive()",
		"function splitEditor(view,orientation)",
		"function swapEditorSplit(view)",
		"function unsplitEditor(view)",
		"view.cm.requestMeasure?.()",
		"splitVertical.onclick=()=>splitEditor(view,'vertical').catch(app.showError)",
		"splitHorizontal.onclick=()=>splitEditor(view,'horizontal').catch(app.showError)",
		"pane.addEventListener('pointerdown',()=>{if(activeEditorID!==id)activateEditor(id,{force:true});},true)",
		"window.addEventListener('resize',()=>{if(renderedEditorSplitRoot)layoutEditorSplit();})",
		"splitEditor,",
		"swapEditorSplit,",
		"unsplitEditor,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor split behavior missing %q", want)
		}
	}
	if strings.Contains(js, "app.views.set(") {
		t.Fatal("editor split must remain independent of terminal session registration")
	}
}


func TestEditorDirtySaveShortcutsAndCloseFlow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function setDirty(view,dirty)",
		"view.tab.classList.toggle('dirty',view.dirty)",
		"method:'PUT'",
		"expected_sha256:expectedSHA256",
		"putEditorFile(view,view.file.sha256)",
		"content:view.cm.state.doc.toString()",
		"Save changes?",
		"Discard",
		"Cancel",
		"function closeEditor(id)",
		"function goToLine(view)",
		"event.key==='Tab'",
		"replaceSelection('\\t')",
		"key==='s'",
		"key==='g'",
		"Discard unsaved changes and reload",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor dirty/save flow missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem") && strings.Contains(js, "view.cm.state.doc.toString()") {
		t.Fatal("dirty editor contents must not be persisted to localStorage")
	}
}


func TestEditorExternalConflictResolutionFlow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"response.status===409",
		"File changed outside the editor.",
		"compare.textContent='Compare'",
		"reload.textContent='Reload'",
		"overwrite.textContent='Overwrite'",
		"cancel.textContent='Cancel'",
		"function showConflictCompare(view,latest)",
		"pre.textContent=text",
		"putEditorFile(view,latest.sha256)",
		"if(result.conflict)continue",
		"setEditorDocument(view,latest)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor conflict flow missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("editor conflict/source UI must not render source through innerHTML")
	}
}


func TestEditorTracksTransactionsAndBlocksReadOnlyDocumentChanges(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function installEditorDispatchGuard(view)",
		"view.cm.state.update(...input)",
		"transaction.docChanged&&editorReadOnly(view)&&!view.internalUpdate",
		"if(transaction.docChanged&&!view.internalUpdate)setDirty(view,true)",
		"view.internalUpdate=true",
		"view.internalUpdate=false",
		"addEventListener('beforeinput'",
		"if(editorReadOnly(view))event.preventDefault()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor transaction guard missing %q", want)
		}
	}
	if strings.Contains(js, "addEventListener('input',()=>{if(!view.file.read_only)setDirty(view,true);}") {
		t.Fatal("dirty tracking must not depend on DOM input events")
	}
}


func TestEditorShowsBackendFileWarnings(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"editor-warning",
		"warningBadge.hidden=!file.warning",
		"warningBadge.title=file.warning||''",
		"view.warningBadge.hidden=!file.warning",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor warning UI missing %q", want)
		}
	}
}


func TestEditorWarnsBeforePageUnloadWhenDirty(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"window.addEventListener('beforeunload'",
		"[...editors.values()].some(view=>!view.closed&&view.dirty)",
		"event.preventDefault()",
		"event.returnValue=''",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor beforeunload guard missing %q", want)
		}
	}
}


func TestCodeMirrorImmutableAssetUsesVersionedURL(t *testing.T) {
	if !strings.Contains(indexHTML, "/vendor/codemirror6-all.min.js?v=52af430d") {
		t.Fatal("CodeMirror immutable asset must use a versioned URL")
	}
}


func TestEditorLanguageMappingCoversRequiredIDEFormats(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"else if(ext==='jsx')options.jsx=true",
		"else if(ext==='ts')options.typescript=true",
		"else if(ext==='tsx')options.tsx=true",
		"if(name==='cmakelists.txt'||ext==='cmake')return 'cmake'",
		"if(['sh','bash','zsh','fish','ksh'].includes(ext)",
		"if(ext==='lua')return 'lua'",
		"function legacySyntaxExtension(kind)",
		"buildLegacyDecorations(update.view,kind)",
		"options.extraExtensions=[legacySyntaxExtension(legacy)]",
		"return 'CMake'",
		"return 'Shell'",
		"return 'Lua'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor language coverage missing %q", want)
		}
	}
	for _, forbidden := range []string{"CMake (plain)", "Shell (plain)", "Lua (plain)"} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("editor still marks required language as plain text: %q", forbidden)
		}
	}
}

func TestVendoredCodeMirrorExposesIDEHooksAndTypeScriptModes(t *testing.T) {
	data, err := webassets.Files.ReadFile("vendor/codemirror6-all.min.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Decoration: () => Decoration",
		"EditorView: () => EditorView",
		"RangeSetBuilder: () => RangeSetBuilder",
		"ViewPlugin: () => ViewPlugin",
		"sqlCompletion: () => sqlCompletion",
		"function sqlCompletion(config2 = {})",
		"dialectName === \"mysql\" ? MySQL : dialectName === \"sqlite\" ? SQLite : StandardSQL",
		"schema: config2.schema",
		"upperCaseKeywords: config2.upperCaseKeywords !== false",
		"{ key: \"Ctrl-Space\", run: startCompletion }",
		"autocompletion()",
		"Array.isArray(options2.extraExtensions)",
		"typescript: javascript({ typescript: true })",
		"jsx: javascript({ jsx: true })",
		"tsx: javascript({ typescript: true, jsx: true })",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("vendored CodeMirror IDE hook missing %q", want)
		}
	}
}

func TestEditorAnnouncesOpenedProjectFiles(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function announceOpenedFile(pathValue)",
		"taskmenu:project-file-opened",
		"announceOpenedFile(pathValue)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor opened-file event missing %q", want)
		}
	}
}
