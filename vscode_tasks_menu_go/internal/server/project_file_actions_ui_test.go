package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestSharedProjectFileActionsLoadedBeforeConsumers(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	shared := strings.Index(js, "projectfileactions.js")
	fileTransfer := strings.Index(js, "filetransfer.js")
	git := strings.Index(js, "gitstatus.js")
	explorer := strings.Index(js, "explorer.js")
	if shared < 0 || fileTransfer < 0 || git < 0 || explorer < 0 {
		t.Fatalf("project file action module or consumer missing")
	}
	if shared > fileTransfer || shared > git || shared > explorer {
		t.Fatalf("shared project file actions must load before consumers")
	}
}

func TestSharedProjectFileActionRegistryProvidesCommonWorkflow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function standardActions(pathValue,type='file')",
		"Open",
		"Reveal in Explorer",
		"Open containing folder",
		"Search / Replace in folder…",
		"Search / Replace in containing folder…",
		"SHA-256 checksum",
		"MD5 checksum (compatibility)",
		"Compare checksum…",
		"Generate SHA-256 manifest…",
		"Verify SHA-256 manifest…",
		"Open With…",
		"Preview archive…",
		"Extract archive…",
		"Create archive…",
		"/api/project/archive/preview?path=",
		"/api/project/archive/create",
		"/api/project/archive/extract",
		"/api/project/integrity?path=",
		"action:'manifest'",
		"action:'verify_manifest'",
		"Git History",
		"Git Blame",
		"Copy path",
		"TaskMenuGitFiles",
		"TaskMenuProjectSearch",
		"TaskMenuExplorer",
		"globalThis.TaskMenuProjectFileActions={standardActions,openMenu",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("shared project file action registry missing %q", want)
		}
	}
}

func TestProjectFileActionsAreUsedAcrossExplorerGitEditorAndHostFiles(t *testing.T) {
	tests := []struct {
		file string
		want []string
	}{
		{
			file: "featuremods/explorer.js",
			want: []string{"TaskMenuProjectFileActions?.standardActions?.(pathValue,type)", "appendSharedProjectActions(pathValue,type)"},
		},
		{
			file: "featuremods/filetransfer.js",
			want: []string{"panel.source==='host'&&selected.length===1", "TaskMenuProjectFileActions?.standardActions?.(projectPath,projectType)"},
		},
		{
			file: "featuremods/gitstatus.js",
			want: []string{"TaskMenuProjectFileActions?.openMenu", "workspacePathForActiveRepository(change.path)", "globalThis.TaskMenuGitFiles={"},
		},
		{
			file: "featuremods/tabcontext.js",
			want: []string{"TaskMenuProjectFileActions?.standardActions?.(view.file?.path,'file')", "addHeading('Project')"},
		},
	}
	for _, test := range tests {
		data, err := webassets.Files.ReadFile(test.file)
		if err != nil {
			t.Fatal(err)
		}
		js := string(data)
		for _, want := range test.want {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing shared project action integration %q", test.file, want)
			}
		}
	}
}

func TestGitWorkspaceFileActionSelectsContainingRepository(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function repositoryForProjectPath(pathValue)",
		"async function openWorkspaceFileView(pathValue,mode='history')",
		"await refreshRepositories(true)",
		"await selectRepository(match.repo.id,{reload:false})",
		"const repoPath=match.root?pathValue.slice(match.root.length+1):pathValue",
		"return openGitFileView(repoPath,mode)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("multi-repo Git project action missing %q", want)
		}
	}
}

func TestSharedProjectFileIntegrityActionsRemainReadOnlyUIWorkflows(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function projectIntegrity(pathValue)",
		"async function copyProjectChecksum(pathValue,algorithm)",
		"async function compareProjectChecksum(pathValue)",
		"async function generateProjectManifest(pathValue)",
		"async function verifyProjectManifest(pathValue)",
		"method:'POST'",
		"link.download='SHA256SUMS'",
		"Expected checksum must contain 32 hex characters for MD5 or 64 for SHA-256.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("shared project integrity workflow missing %q", want)
		}
	}
	if strings.Contains(js, "action:'manifest_write'") {
		t.Fatal("integrity manifest UI must not silently write into the project")
	}
}

func TestProjectArchiveActionsExposeSafeProjectWorkflow(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function projectArchiveKind(pathValue)",
		"async function previewProjectArchive(pathValue)",
		"async function createProjectArchive(paths,defaultOutput='')",
		"async function extractProjectArchive(pathValue)",
		"async function downloadProjectPathsAsZip(paths,name='taskdeck-selection.zip')",
		"Existing destinations are never overwritten.",
		"link.download=name||'taskdeck-selection.zip'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project archive UI workflow missing %q", want)
		}
	}
}

func TestProjectOpenWithUnifiesViewerAndHostActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function openProjectWith(pathValue)",
		"/api/project/preview?path=",
		"openWithButton(dialog,'Text Editor'",
		"openWithButton(dialog,'Hex'",
		"openWithButton(dialog,'Image Preview'",
		"openWithButton(dialog,'Markdown Preview'",
		"openWithButton(dialog,'Diff…'",
		"openWithButton(dialog,'Download'",
		"openWithButton(dialog,'Open terminal directory'",
		"if(!app.sharedMode)openWithButton(dialog,'Open via host application'",
		"/api/project/download?path=",
		"/api/project/open-host?path=",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("project Open With workflow missing %q", want)
		}
	}
	if strings.Contains(js, "if(app.sharedMode)openWithButton(dialog,'Open via host application'") {
		t.Fatal("host application action must not be exposed in shared mode")
	}
}
