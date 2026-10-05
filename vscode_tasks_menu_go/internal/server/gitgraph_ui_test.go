package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitGraphUIProvidesDAGFiltersRefsDetailsAndContextActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['graph','Graph']",
		"function gitGraphLayout(commits)",
		"function gitGraphSVG(layout)",
		"git-graph-svg",
		"function gitGraphRefNodes(refs)",
		"gitGraphFilters={search:'',author:'',message:'',since:'',until:'',path:''}",
		"Search hash / author / message / ref",
		"Author filter",
		"Message contains",
		"Path filter",
		"view:'graph'",
		"gitView('graph',params)",
		"gitView('commit-files',params)",
		"Diff parent",
		"Visual diff",
		"openGitCommitFileDiffBetween",
		"openGitCommitFileDiff(activeRepoID,file,commit)",
		"Checkout commit (detached)",
		"Create branch here…",
		"Cherry-pick",
		"Revert",
		"Reset current branch here…",
		"Create tag here…",
		"Compare with HEAD",
		"graph-compare-head",
		"reset_commit",
		"create_branch_at",
		"checkout_commit",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Git graph UI missing %q", want)
		}
	}
	if strings.Contains(js, ".innerHTML") {
		t.Fatal("Git graph must not render commit or ref text with innerHTML")
	}
}

func TestFileCompareSupportsMissingCommitSidesAndCommitToHead(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"allowMissing=false",
		"params.set('allow_missing','1')",
		"function emptyCompareSource(",
		"async function openGitCommitFileDiff(repoID,file,commit)",
		"async function openGitCommitFileDiffBetween(repoID,file,leftRef,rightRef",
		"old_path",
		"allowMissing:true",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("commit file compare missing %q", want)
		}
	}
}
