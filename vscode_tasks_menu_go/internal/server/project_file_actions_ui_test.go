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
			want: []string{"TaskMenuProjectFileActions?.openMenu", "workspacePathForActiveRepository(change.path)", "globalThis.TaskMenuGitFiles={openWorkspaceFileView"},
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
