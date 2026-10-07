package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGenericFileCompareFeatureLoads(t *testing.T) {
	next, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(next)
	if !strings.Contains(js, "import '/featuremods/filecompare.js';") {
		t.Fatal("generic file compare feature is not loaded")
	}
}

func TestGenericFileCompareSupportsCoreSourcesAndViews(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function buildCompareModel(leftText,rightText)",
		"function diffOperations(leftText,rightText)",
		"function lcsBlock(a,b)",
		"Side by side",
		"Inline",
		"Left → Right",
		"Right → Left",
		"function replaceLineRange(text,start,count,replacementLines)",
		"async function copyHunk(hunk,direction)",
		"function projectSource(pathValue",
		"expected_sha256:sha",
		"function editorSource(view",
		"function savedEditorSource(view)",
		"function clipboardSource(",
		"async function openProjectFiles(leftPath,rightPath)",
		"async function openEditorSaved(view)",
		"async function openEditorClipboard(view)",
		"async function promptProjectCompare(pathValue)",
		"globalThis.TaskMenuFileCompare={",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("generic file compare missing %q", want)
		}
	}
	if strings.Contains(js, "innerHTML") {
		t.Fatal("file compare must not render source text through innerHTML")
	}
}

func TestFileCompareHighlightsSyntaxAndChangedSpans(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function compareLanguageID(pathValue)",
		"TaskMenuEditor?.languageForPath?.",
		"function compareSyntaxSource(pathValue,text)",
		"file-compare-syntax-keyword",
		"file-compare-syntax-comment",
		"function compareInlineRanges(left,right)",
		"function renderCompareCode(node,text,tokens=[],changed=[],changeKind='')",
		"file-compare-inline-change",
		"compareInlineRanges(row.left?.text||'',row.right?.text||'')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("file compare syntax/intra-line highlighting missing %q", want)
		}
	}
	if strings.Contains(js, ".innerHTML") {
		t.Fatal("file compare syntax highlighting must keep source rendering textContent-only")
	}
}

func TestFileCompareClassifiesAndFiltersChanges(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function classifyCompareRow(row)",
		"compareCommentOnly(text,tokens)",
		"compareIndentInsensitiveLanguages",
		"return 'important'",
		"return 'unimportant'",
		"View all",
		"View diff",
		"View diff context",
		"View unimportant",
		"function compareStructuralContextRange(model,index)",
		"const indentLanguages=new Set(['python','yaml','nim'])",
		"const braceLanguages=new Set(",
		"function compareVisibleIndexes(model)",
		"showUnimportant||row.importance!=='unimportant'",
		"No differences match the current filters",
		"including rows hidden by filters",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("file compare classification/filter/context missing %q", want)
		}
	}
	for _, want := range []string{
		".file-compare-cell.removed.important",
		".file-compare-cell.added.important",
		".file-compare-cell.removed.unimportant",
		".file-compare-inline-line.added.unimportant",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("file compare semantic color role missing %q", want)
		}
	}
}

func TestFileCompareSupportsArbitraryWritableSelections(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"let compareSelection=null",
		"function selectForCompare(source)",
		"async function compareWithSelected(source",
		"async function openSources(sources",
		"function createDetachedEditor",
		"file-compare-editor-deck",
		"Save Left",
		"Save Right",
		"async function saveCompareSide(side)",
		"source.baselineText=source.text",
		"setSourceBuffer(targetSide,next",
		"The destination will be marked unsaved until you press Save.",
		"const saver=typeof source.saveText==='function'?source.saveText:source.writeText",
		"activeSource.meta?.sha256",
		"taskmenu:file-transfer-remote-edited",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("arbitrary writable compare missing %q", want)
		}
	}

	editorData, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil {
		t.Fatal(err)
	}
	editorJS := string(editorData)
	for _, want := range []string{
		"function createDetachedEditor(host,text,pathValue",
		"cmFactory.newEditor(host,String(text??''),languageOptions(pathValue))",
		"createDetachedEditor,",
	} {
		if !strings.Contains(editorJS, want) {
			t.Fatalf("detached compare editor support missing %q", want)
		}
	}
}

func TestFileCompareSelectionActionsAcrossExplorerTransferAndTabs(t *testing.T) {
	files := map[string][]string{
		"featuremods/projectfileactions.js": {
			"Select for compare",
			"Compare with selected file",
			"selectProjectForCompare(pathValue)",
			"compareProjectWithSelected(pathValue)",
		},
		"featuremods/explorer.js": {
			"Compare selected files",
			"paths.length===2&&paths.every(value=>findLoadedItem(value)?.type==='file')",
			"compare.openProjectFiles(paths[0],paths[1])",
		},
		"featuremods/filetransfer.js": {
			"function compareRemoteFileSource(view,entry)",
			"function appendCompareSelectionActions(items,source)",
			"Compare selected files",
			"compareTransferSources(compareFiles.map(item=>compareLeftFileSource(view,item))",
			"compareTransferSources(compareFiles.map(item=>compareRemoteFileSource(view,item))",
			"writable:remoteEditorWritable()",
		},
		"featuremods/tabcontext.js": {
			"Select current editor for compare",
			"Compare selected file ↔ current editor",
			"compare?.selectForCompare?.(currentCompareSource)",
			"compare?.compareWithSelected?.(currentCompareSource",
		},
	}
	for file, wants := range files {
		data, err := webassets.Files.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		js := string(data)
		for _, want := range wants {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing arbitrary compare integration %q", file, want)
			}
		}
	}
}

func TestProjectAndEditorMenusExposeGenericCompare(t *testing.T) {
	shared, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Compare with another file…",
		"TaskMenuFileCompare",
		"promptProjectCompare(pathValue)",
	} {
		if !strings.Contains(string(shared), want) {
			t.Fatalf("shared project file actions missing compare contract %q", want)
		}
	}

	tabContext, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Compare current ↔ saved",
		"TaskMenuFileCompare?.openEditorSaved?.(view)",
		"Compare current ↔ clipboard",
		"TaskMenuFileCompare?.openEditorClipboard?.(view)",
	} {
		if !strings.Contains(string(tabContext), want) {
			t.Fatalf("editor context missing compare action %q", want)
		}
	}
}

func TestFileCompareSupportsGitAndRemoteSources(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function gitCommitSource(repoID,pathValue,ref",
		"view:'file-content'",
		"async function openGitCommitAgainstProject",
		"async function openGitCommits",
		"function remoteSource(profileID,pathValue",
		"/api/file-transfer/text?",
		"expected_sha256:sha",
		"function browserFileHandleSource(handle",
		"Local browser file changed after compare load",
		"async function openLeftRemote",
		"source_kind:to.kind",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("file compare external source support missing %q", want)
		}
	}
}

func TestGitFileHistoryExposesCommitCompareActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"gitFileCompareRef",
		"Compare current",
		"Use as Compare A",
		"Compare A ↔ this",
		"openGitCommitAgainstProject",
		"openGitCommits",
		"function setGitFilePath(path)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git file-history compare support missing %q", want)
		}
	}
}

func TestFileTransferMenusExposeSelectedLocalRemoteCompare(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filetransfer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function compareLeftFileSource(view,entry)",
		"async function compareLeftRemoteFiles(view,leftEntry,remoteEntry)",
		"Compare with selected remote file",
		"Compare with selected left file",
		"TaskMenuFileCompare",
		"browserFileHandleSource",
		"openLeftRemote",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("File Transfer compare integration missing %q", want)
		}
	}
}

func TestFileCompareSupportsGitThreeStateAndOpenEditorBuffer(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function gitStateSource(repoID,pathValue,state",
		"state!=='head'&&state!=='index'",
		"function editorForProjectPath(pathValue)",
		"for(const view of editor.editors.values())",
		"function workingProjectSource(pathValue",
		"if(view)return editorSource(view",
		"async function openGitStatePair(repoID,repoPath,workspacePath,mode)",
		"title:'HEAD ↔ Staged'",
		"title:'Staged ↔ Working'",
		"title:'HEAD ↔ Working'",
		"right:workingProjectSource(workspacePath",
		"title:'Git commit ↔ Current'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git three-state generic compare missing %q", want)
		}
	}

	gitData, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	gitJS := string(gitData)
	for _, want := range []string{
		"function openGenericGitStateCompare(path,mode)",
		"compare.openGitStatePair(activeRepoID,path,workspacePathForActiveRepository(path),mode)",
		"Open File Compare",
		"Compare current",
		"current editor buffer when open",
	} {
		if !strings.Contains(gitJS, want) {
			t.Fatalf("Git panel generic compare integration missing %q", want)
		}
	}
}
