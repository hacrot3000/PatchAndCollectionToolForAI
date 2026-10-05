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
