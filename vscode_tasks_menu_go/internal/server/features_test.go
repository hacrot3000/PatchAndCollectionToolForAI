package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func embeddedFeaturesJS(t *testing.T) string {
	t.Helper()
	data, err := webassets.Files.ReadFile("features.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEmbeddedFeaturesAutoDetectFileAndURLActions(t *testing.T) {
	js := embeddedFeaturesJS(t)
	for _, want := range []string{
		"taskmenu:output",
		"/api/files/selection",
		"detected-actions",
		"download.textContent='Download'",
		"copy.textContent='Copy'",
		"navigator.clipboard.writeText(text)",
		"localhost|127\\.0\\.0\\.1",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing auto-detect behavior %q", want)
		}
	}
	if !strings.Contains(indexHTML, `/features.js`) {
		t.Fatalf("index HTML must load embedded features.js")
	}
}

func TestEmbeddedFeaturesDetectedFilesCanBeIgnoredAndPrioritized(t *testing.T) {
	js := embeddedFeaturesJS(t)
	for _, want := range []string{
		"ignore.textContent='Ignore'",
		"function ignoreFile(state,file)",
		"sessionStorage.setItem",
		"sessionStorage.getItem",
		"function userTypedFile(state,path)",
		"view.term.onData(data=>captureUserInput(state,data))",
		"/artifacts/ptv_to_ai/",
		"/^CR_[^/]+\\.zip$/i",
		"/^CR_[^/]+\\.txt$/i",
		"slice(-131072)",
		"remember(state.files,file.path,file,32)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing detected-file prioritization/ignore behavior %q", want)
		}
	}
}

func TestEmbeddedFeaturesSuppressGitCommandOutputFromFileDetection(t *testing.T) {
	js := embeddedFeaturesJS(t)
	for _, want := range []string{
		"function standaloneGitCommand(line)",
		"function taskUsesGit(view)",
		"beginGitOutputSuppression(state)",
		"endGitOutputSuppression(state)",
		"if(standaloneGitCommand(line))beginGitOutputSuppression(state)",
		"if(state.gitTaskOutput||state.suppressGitOutput)return",
		"state.seq++",
		"state.recent=''",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing Git output suppression behavior %q", want)
		}
	}
	if strings.Contains(js, "git status')") {
		t.Fatal("Git suppression must not be hard-coded only to git status")
	}
}

func TestEmbeddedFeaturesTaskSearchPalette(t *testing.T) {
	js := embeddedFeaturesJS(t)
	for _, want := range []string{
		"Search tasks…  Ctrl+K",
		"function searchTasks(query)",
		"task-search-results",
		"ArrowDown",
		"ArrowUp",
		"e.key==='Enter'",
		"e.key.toLowerCase()==='k'",
		"app.startTask(task)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("features.js missing task-search behavior %q", want)
		}
	}
}
