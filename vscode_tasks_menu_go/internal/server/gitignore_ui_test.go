package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitIgnoreWizardModule(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitignorewizard.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"Git ignore wizard",
		"TaskDeckGitIgnoreWizard",
		"Ignore file / similar files",
		"SELECTED FILE / FOLDER",
		"SIMILAR FILES",
		"COMMON IGNORE RULES",
		"RECOMMENDED",
		"BROAD",
		"match_count",
		"samples",
		"Copy pattern",
		"Add to .gitignore",
		"GIT-IGNORE PATTERN (EDITABLE)",
		"Checking customized pattern",
		"previewPatternNow",
		"current.apply(item,pattern)",
		"Analyzing untracked files and candidate patterns",
		"window.confirm",
		"function renderOption(item)",
		"function selectOption(id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitignorewizard.js missing %q", want)
		}
	}
	for _, forbidden := range []string{"innerHTML=", "eval(", "new Function("} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("gitignorewizard.js must not use unsafe dynamic rendering/execution %q", forbidden)
		}
	}
}

func TestGitChangesHasSmartIgnoreAction(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/gitstatus.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const gitIgnoreWizard=globalThis.TaskDeckGitIgnoreWizard",
		"async function openIgnoreWizard(change)",
		"view:'ignore-suggestions'",
		"view:'ignore-preview'",
		"ignore_pattern:String(pattern||'')",
		"action('ignore',{path,ignore_id:item.id,ignore_pattern:String(pattern||'')}",
		"recovery:false",
		"change.untracked&&change.path!=='.gitignore'",
		"actionButton('Ignore'",
		"Ignore this untracked path or choose a smart pattern for similar files",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("gitstatus.js missing smart ignore integration %q", want)
		}
	}
}

func TestGitIgnoreWizardLoadsBeforeGitStatus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	wizard := strings.Index(js, "featuremods/gitignorewizard.js")
	status := strings.Index(js, "featuremods/gitstatus.js")
	if wizard < 0 || status < 0 || wizard > status {
		t.Fatalf("gitignorewizard.js must load before gitstatus.js: wizard=%d status=%d", wizard, status)
	}
}
