package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPatchPanelUsesBuiltinSessionAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function installPatchPanel()",
		"task-patch-panel",
		"['queue','Queue'",
		"['resume','Resume'",
		"['history','History'",
		"['plan','Plan'",
		"JSON.stringify({kind:'patch',patch_mode:mode})",
		"app.attachSession(meta,true)",
		"/protocol",
		"const maxAttempts=followLifecycle?7200:40",
		"for(let attempt=0;attempt<maxAttempts;attempt+=1)",
		"renderQueueSnapshot(state.queue_snapshot)",
		"state?.prompt&&renderQueuePrompt(sessionId,state.prompt)",
		"renderItemLifecycle(state?.items)",
		"renderArtifacts(state?.artifacts)",
		"/api/files/download?path=",
		"taskmenu:project-file-open-request",
		"kind.endsWith('_text')",
		"task-patch-artifacts",
		"followLifecycle?7200:40",
		"followLifecycle?1000:250",
		"protocolPollGeneration",
		"/prompt-response",
		"submitPromptResponse(sessionId,prompt,'select')",
		"submitPromptResponse(sessionId,prompt,'cancel')",
		"prompt.initial_selected",
		"constraints.collect_exclusive",
		"mode==='queue'",
		"items.slice(0,50)",
		"state?.available===false",
		"TaskMenuPatchPanel={open,close,toggle,start,renderQueueSnapshot,renderQueuePrompt,renderItemLifecycle,renderArtifacts",
		"Patch panel enhancement disabled:",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("patchpanel.js missing %q", want)
		}
	}
	if strings.Contains(js, "run_python_patches.sh") {
		t.Fatal("Patch panel must use the built-in session API, not a project launcher path")
	}
}

func TestPatchPanelLoadsBeforeActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	panel := strings.Index(js, "patchpanel.js")
	activity := strings.Index(js, "activitybar.js")
	if panel < 0 || activity < 0 || panel > activity {
		t.Fatal("Patch panel must initialize before the Activity Bar binds its Patch view")
	}
}


func TestPatchPanelNativeSummaryDoesNotParseTerminalOutput(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, forbidden := range []string{
		"AUTO STATUS:",
		"CON TRỎ",
		"Missing patch signature",
		"terminal.write",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Patch native summary must not parse or infer terminal output: found %q", forbidden)
		}
	}
}


func TestPatchPanelPromptUsesProtocolDataNotTerminalHeuristics(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"prompt?.prompt_kind!=='queue_selection'",
		"item?.index",
		"item?.kind",
		"item?.group",
		"prompt.actions",
		"prompt.constraints",
		"Python validates the final selection.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native queue prompt missing protocol-driven contract %q", want)
		}
	}
}


func TestPatchPanelArtifactActionsUseStructuredProtocolOnly(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"artifact?.path",
		"artifact?.artifact_kind",
		"artifact?.primary",
		"renderArtifacts(state?.artifacts)",
		"download.href='/api/files/download?path='+encodeURIComponent(path)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("artifact UI missing structured protocol contract %q", want)
		}
	}
}
