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
		"const view=await promise",
		"activateEditor(view.id,{force:true})",
		"announceOpenedFile(pathValue)",
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
		"const editorLanguageRegistry=[",
		"{id:'cpp',label:'C/C++'",
		"{id:'go',label:'Go',extensions:['go'],codeMirror:'go'}",
		"{id:'python',label:'Python'",
		"{id:'javascript',label:'JavaScript'",
		"{id:'typescript',label:'TypeScript'",
		"{id:'tsx',label:'TSX'",
		"{id:'json',label:'JSON'",
		"{id:'yaml',label:'YAML'",
		"{id:'markdown',label:'Markdown'",
		"{id:'html',label:'HTML'",
		"{id:'css',label:'CSS'",
		"{id:'xml',label:'XML'",
		"{id:'java',label:'Java'",
		"{id:'php',label:'PHP'",
		"{id:'sql',label:'SQL'",
		"{id:'rust',label:'Rust'",
		"{id:'vue',label:'Vue'",
		"{id:'cmake',label:'CMake'",
		"{id:'shell',label:'Shell'",
		"{id:'nim',label:'Nim',extensions:['nim','nims','nimble'],legacy:'nim'}",
		"{id:'lua',label:'Lua',extensions:['lua'],legacy:'lua'}",
		"function editorLanguageDefinition(pathValue)",
		"if(language?.codeMirror)options[language.codeMirror]=true",
		"if(language?.legacy)options.extraExtensions.push(legacySyntaxExtension(language.legacy))",
		"return editorLanguageDefinition(pathValue)?.label||'Plain text'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor language registry missing %q", want)
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
		"window.addEventListener('resize',()=>{if(renderedEditorSplitRoot)layoutEditorSplit();for(const view of editors.values())scheduleEditorMinimap(view);})",
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


func TestEditorSmartEnterPreservesIndentAndSplitsPairs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function editorLeadingIndent(text)",
		"function editorIndentUnit(state,line)",
		"function editorCompletionMenuOpen(view)",
		"const editorSmartEnterPairs=new Map([['{','}'],['(',')'],['[',']']])",
		"function smartEditorEnter(view)",
		"if(selection.from!==selection.to)return false",
		"insert='\\n'+childIndent+'\\n'+indent",
		"insert='\\n'+indent",
		"selection:{anchor}",
		"event.key==='Enter'",
		"if(smartEditorEnter(view))",
		"event.preventDefault();event.stopPropagation()",
		".cm-tooltip-autocomplete",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor smart Enter behavior missing %q", want)
		}
	}
}


func TestEditorOpeningBraceKeepsCurrentIndent(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function smartEditorOpenBrace(view)",
		"const insert=right==='}'?'{':'{}'",
		"selection:{anchor:pos+1}",
		"event.key==='{'",
		"if(smartEditorOpenBrace(view))",
		"event.preventDefault();event.stopPropagation()",
		"if(tabLines>0)return '\\t'",
		"return '\\t'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor opening-brace indentation behavior missing %q", want)
		}
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
		"if(!view.internalUpdate)setDirty(view,true)",
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
	if !strings.Contains(indexHTML, "/vendor/codemirror6-all.min.js?v=1c870b80") {
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
		"codeMirror:'go'",
		"codeMirror:'jsx'",
		"codeMirror:'typescript'",
		"codeMirror:'tsx'",
		"filenames:['cmakelists.txt']",
		"filenames:['.bashrc','.zshrc']",
		"extensions:['nim','nims','nimble']",
		"extensions:['lua']",
		"if(kind==='lua'&&ch==='[')",
		"match(/^\\[(=*)\\[/)",
		"function legacySyntaxExtension(kind)",
		"buildLegacyDecorations(update.view,kind)",
		"options.extraExtensions=[activeLineDecorationExtension(),whitespaceDecorationExtension(),bracketMatchingExtension()]",
		"options.extraExtensions.push(legacySyntaxExtension(language.legacy))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor language coverage missing %q", want)
		}
	}
	for _, forbidden := range []string{"CMake (plain)", "Shell (plain)", "Nim (plain)", "Lua (plain)"} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("editor still marks required language as plain text: %q", forbidden)
		}
	}
}
func TestEditorSupportsSCSSSassAndLess(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"{id:'scss',label:'SCSS',extensions:['scss'],codeMirror:'sass'}",
		"{id:'sass',label:'Sass',extensions:['sass'],legacy:'sass'}",
		"{id:'less',label:'Less',extensions:['less'],codeMirror:'less'}",
		"sass:new Set(",
		"sass:['//']",
		"sass:[['/*','*/']]",
		"legacyDollarVariableKinds=new Set(['shell','sass','powershell','makefile'])",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor Sass/Less coverage missing %q", want)
		}
	}
}


func TestEditorCommonProjectLanguageModes(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"{id:'dockerfile',label:'Dockerfile'",
		"filenamePrefixes:['dockerfile.']",
		"{id:'makefile',label:'Makefile'",
		"filenames:['makefile','gnumakefile','bsdmakefile']",
		"{id:'toml',label:'TOML',extensions:['toml']",
		"{id:'powershell',label:'PowerShell',extensions:['ps1','psm1','psd1']",
		"{id:'kotlin',label:'Kotlin',extensions:['kt','kts']",
		"{id:'csharp',label:'C#',extensions:['cs','csx']",
		"{id:'dart',label:'Dart',extensions:['dart']",
		"{id:'protobuf',label:'Protocol Buffers',extensions:['proto']",
		"{id:'graphql',label:'GraphQL',extensions:['graphql','gql']",
		"{id:'actionscript',label:'ActionScript',extensions:['as']",
		"{id:'nginx',label:'Nginx'",
		"{id:'apache',label:'Apache'",
		"{id:'config',label:'Config',extensions:['ini','properties'],filenames:['.env'],filenamePrefixes:['.env.']",
		"legacyDollarVariableKinds",
		"legacyHyphenIdentifierKinds",
		"legacyCommandFirstKinds",
		"powershell:[['<#','#>']]",
		"actionscript:[['/*','*/']]",
		"filenamePrefixes:['makefile.','gnumakefile.']",
		"extensions:['ts','mts','cts']",
		"const firstNonWhitespace=text.search(/\\S/)",
		"(kind==='toml'||kind==='config')&&i===firstNonWhitespace",
		"kind==='makefile'&&text[j]==='('",
		"kind==='powershell'?/[A-Za-z0-9_:?\\-]/",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor common language mode missing %q", want)
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
		"autocompletion: () => autocompletion",
		"completeFromList: () => completeFromList",
		"snippetCompletion: () => snippetCompletion",
		"sqlCompletion: () => sqlCompletion",
		"startCompletion: () => startCompletion",
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

func TestEditorRemapsOpenTabsAfterProjectRename(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function remapOpenedEditorPaths(oldPath,newPath)",
		"view.file.path===oldPath||view.file.path.startsWith(prefix)",
		"view.file.path=nextPath",
		"view.tab.dataset.id=nextID",
		"view.pane.dataset.id=nextID",
		"remapEditorSplitID(root,oldID,nextID)",
		"taskmenu:project-path-renamed",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor rename remap missing %q", want)
		}
	}
}

func TestEditorClosesCleanTabsForTrashedProjectPaths(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function closeEditorsForTrashedPath(pathValue)",
		"view.file.path===pathValue||view.file.path.startsWith(prefix)",
		"if(view.dirty)continue",
		"destroyEditor(view.id)",
		"taskmenu:project-path-trashed",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor trash integration missing %q", want)
		}
	}
}

func TestEditorTextFormatControls(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function editorEncodingChoice(file)",
		"function syncEditorFormatControls(view)",
		"editor-line-ending",
		"editor-encoding",
		"Line ending used when saving",
		"UTF-8 encoding used when saving",
		"['lf','LF']",
		"['crlf','CRLF']",
		"['utf-8','UTF-8']",
		"['utf-8-bom','UTF-8 BOM']",
		"line_ending:view.desiredLineEnding",
		"encoding:view.desiredEncoding",
		"view.desiredLineEnding=lineEndingSelect.value",
		"view.desiredEncoding=encodingSelect.value",
		"setDirty(view,true)",
		"lineEndingSelect.disabled=readonly",
		"encodingSelect.disabled=readonly",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor text format control missing %q", want)
		}
	}
}

func TestEditorCurrentLineAndWhitespaceDecorations(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"cm-taskdeck-active-line",
		"cm-taskdeck-space",
		"cm-taskdeck-tab",
		"function buildWhitespaceDecorations(view)",
		"function whitespaceDecorationExtension()",
		"function activeLineDecorationExtension()",
		"update.selectionSet",
		"options.extraExtensions=[activeLineDecorationExtension(),whitespaceDecorationExtension(),bracketMatchingExtension()]",
		"editor-whitespace-toggle",
		"Toggle visible spaces and tabs",
		"editor-show-whitespace",
		"whitespace.setAttribute('aria-pressed',enabled?'true':'false')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor current-line/whitespace support missing %q", want)
		}
	}
}

func TestEditorReopensRecentlyClosedTabs(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const closedEditorPaths=[]",
		"const maxClosedEditorPaths=30",
		"function rememberClosedEditor(view)",
		"async function reopenClosedEditor()",
		"rememberClosedEditor(view)",
		"key==='t'&&event.shiftKey",
		"reopenClosedEditor().catch(app.showError)",
		"reopenClosedEditor,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor reopen closed tab support missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem") && strings.Contains(js, "closedEditorPaths") {
		t.Fatal("closed editor stack must remain in-memory only")
	}
}

func TestEditorLargeFileMode(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function editorOptionsForFile(file)",
		"file?.large_file?{}:languageOptions(file.path)",
		"editor-large-file",
		"LARGE FILE",
		"Large-file mode: read-only plain text without syntax/whitespace decorations",
		"editor-large-file-mode",
		"view.whitespace.disabled=Boolean(view.file?.large_file)",
		"cmFactory.newEditor(host,file.content||'',editorOptionsForFile(file))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor large-file mode missing %q", want)
		}
	}
}

func TestEditorSavePreservesCursorSelectionAndViewport(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function applySavedEditorFile(view,file)",
		"const editorContent=view.cm.state.doc.toString()",
		"view.file={...file,content:editorContent}",
		"applySavedEditorFile(view,result.file)",
		"save.onmousedown=event=>event.preventDefault()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor save state preservation missing %q", want)
		}
	}
	if strings.Contains(js, "setEditorDocument(view,result.file)") {
		t.Fatal("successful save must not replace the CodeMirror document and reset cursor/selection/scroll")
	}
}

func TestEditorConfigurableAutoSave(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const editorAutoSavePreferenceKey='vscode-tasks-menu:editor-auto-save'",
		"const editorAutoSaveDelayMS=1000",
		"function scheduleEditorAutoSave(view)",
		"function setEditorAutoSave(enabled)",
		"Toggle editor auto-save (1 second debounce)",
		"autoSave.onclick=()=>setEditorAutoSave(!editorAutoSaveEnabled)",
		"saveEditor(view).catch(app.showError)",
		"clearTimeout(view.autoSaveTimer)",
		"localStorage[editorAutoSavePreferenceKey]=editorAutoSaveEnabled?'1':'0'",
		"get autoSaveEnabled(){return editorAutoSaveEnabled;}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor auto-save support missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem") && strings.Contains(js, "view.cm.state.doc.toString()") {
		t.Fatal("auto-save must never persist dirty editor contents into localStorage")
	}
}

func TestEditorBracketMatching(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"cm-taskdeck-bracket-match",
		"cm-taskdeck-bracket-mismatch",
		"const taskDeckBracketPairs={40:41,91:93,123:125}",
		"const taskDeckBracketReverse={41:40,93:91,125:123}",
		"const taskDeckBracketScanLimit=200000",
		"function bracketCandidateAtCursor(state)",
		"function findMatchingBracket(state,candidate)",
		"function buildBracketDecorations(view)",
		"function bracketMatchingExtension()",
		"update.docChanged||update.selectionSet",
		"bracketMatchingExtension()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor bracket matching missing %q", want)
		}
	}
}

func TestEditorFindReplacePanel(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"editor-find-panel",
		"editor-find-toggle",
		"function editorFindMatch(view",
		"function editorSelectionMatchesFind(view)",
		"function openEditorFind(view,{replace=false}={})",
		"function replaceEditorMatch(view)",
		"function replaceAllEditorMatches(view)",
		"findInput.placeholder='Find'",
		"findReplaceInput.placeholder='Replace'",
		"Match case",
		"findNext.onclick=()=>editorFindMatch(view,{direction:1})",
		"findPrevious.onclick=()=>editorFindMatch(view,{direction:-1})",
		"findReplace.onclick=()=>replaceEditorMatch(view)",
		"findReplaceAll.onclick=()=>replaceAllEditorMatches(view)",
		"view.findReplace.disabled=readonly",
		"view.findReplaceAll.disabled=readonly",
		"shortcutKey==='f'||shortcutKey==='h'",
		"openEditorFind(view,{replace:shortcutKey==='h'})",
		"event.stopPropagation()",
		"else if(key==='f')",
		"else if(key==='h')",
		"Replaced '+changes.length",
		"changes.length<10000",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor find/replace support missing %q", want)
		}
	}
}

func TestEditorOptionalMinimap(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const editorMinimapPreferenceKey='vscode-tasks-menu:editor-minimap'",
		"let editorMinimapEnabled=localStorage.getItem(editorMinimapPreferenceKey)==='1'",
		"editor-minimap",
		"editor-minimap-toggle",
		"function renderEditorMinimap(view)",
		"function scheduleEditorMinimap(view)",
		"function setEditorMinimap(enabled)",
		"function jumpFromEditorMinimap(view,event)",
		"requestAnimationFrame",
		"cancelAnimationFrame(view.minimapRAF)",
		"scroller.scrollTop",
		"scroller.clientHeight",
		"minimap.onclick=event=>jumpFromEditorMinimap(view,event)",
		"minimapToggle.onclick=()=>setEditorMinimap(!editorMinimapEnabled)",
		"localStorage[editorMinimapPreferenceKey]=editorMinimapEnabled?'1':'0'",
		"get minimapEnabled(){return editorMinimapEnabled;}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor minimap support missing %q", want)
		}
	}
}

func TestEditorLocalFileHistoryUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"editor-history-toggle",
		"editor-history-dialog",
		"function loadEditorLocalHistory(view)",
		"function loadEditorLocalHistoryEntry(view,id)",
		"'/api/project/file-history?path='",
		"function showEditorHistoryCompare(view,entry)",
		"function restoreEditorLocalHistory(view,entry)",
		"function showEditorLocalHistory(view)",
		"expected_sha256:latest.sha256",
		"line_ending:entry.line_ending||'preserve'",
		"encoding:entry.bom?'utf-8-bom':'utf-8'",
		"File changed again while restoring local history",
		"The current disk version will be saved into local history first.",
		"restore.disabled=editorReadOnly(view)",
		"history.onclick=()=>showEditorLocalHistory(view).catch(app.showError)",
		"if(result.file.history_warning)app.showError(new Error(result.file.history_warning))",
		"if(await restoreEditorLocalHistory(view,entry))finish()",
		"showEditorLocalHistory,",
		"restoreEditorLocalHistory,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor local file history UI missing %q", want)
		}
	}
	if strings.Contains(js, "localStorage.setItem") && strings.Contains(js, "history_warning") {
		t.Fatal("local file history contents/warnings must not be persisted through browser localStorage")
	}
}

func TestEditorLightweightSymbolOutlineAndBreadcrumb(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"editor-outline-toggle",
		"editor-outline-dialog",
		"editor-breadcrumb",
		"const editorSymbolControlWords=new Set(",
		"function editorSymbolLanguage(pathValue)",
		"function editorSymbolFromLine(line,language)",
		"function extractEditorSymbols(text,pathValue)",
		"function jumpEditorToLine(view,lineNumber)",
		"function currentEditorSymbol(view,symbols=null)",
		"function updateEditorBreadcrumb(view)",
		"function showEditorOutline(view)",
		"out.length<1000",
		"language==='go'",
		"language==='javascript'",
		"language==='python'",
		"language==='c-family'",
		"language==='php'",
		"language==='rust'",
		"language==='shell'",
		"breadcrumb.textContent=basename(view.file.path)+(symbol?' › '+symbol.kind+' '+symbol.name:'')",
		"outline.onclick=()=>showEditorOutline(view)",
		"row.onclick=()=>{finish();jumpEditorToLine(view,item.line);}",
		"cm.contentDOM.addEventListener('keyup',()=>updateEditorBreadcrumb(view))",
		"extractEditorSymbols,",
		"showEditorOutline,",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor symbol outline support missing %q", want)
		}
	}
}
