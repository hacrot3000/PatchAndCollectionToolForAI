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
		"renderProgress(state?.progress)",
		"if(state?.action_result)renderActionResult(state.action_result)",
		"/item-action",
		"response?.action_id",
		"waitForActionResult",
		"state?.action_result?.action_id===actionID",
		"activeQueuePrompt?.item_actions",
		"promptItemForQueueItem(item)",
		"['inspect','preview','validate']",
		"task-patch-action-result",
		"task-patch-item-action",
		"progress.elapsed_seconds",
		"progress.output_lines",
		"task-patch-progress",
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
		"const visible=items.slice(0,50)",
		"setQueueSummaryView('queue')",
		"setQueueSummaryView('failed')",
		"item?.group==='failed'",
		"item?.group==='new'",
		"failure?.diagnosis_kind",
		"snapshot?.group_counts",
		"state?.available===false",
		"TaskMenuPatchPanel={open,close,toggle,start,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,submitItemAction,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts",
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


func TestPatchPanelQueueFailedViewsUsePythonGroupingOnly(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"item?.group==='failed'",
		"item?.group==='new'",
		"failure?.status",
		"failure?.rc",
		"failure?.diagnosis_kind",
		"failure?.message",
		"snapshot?.group_counts",
		"No unresolved failed item",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Queue/Failed view missing protocol field %q", want)
		}
	}
	for _, forbidden := range []string{
		"item.name.includes('FAIL')",
		"item.name.includes('failed')",
		"failure_policy",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Queue/Failed UI must not infer Python failure policy: found %q", forbidden)
		}
	}
}


func TestPatchPanelNativeItemActionsArePromptAdvertisedAndCorrelated(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"activeQueuePrompt?.item_actions",
		"promptItemForQueueItem(item)",
		"String(item?.kind||'').toUpperCase()==='PATCH'",
		"JSON.stringify({prompt_id:String(activeQueuePrompt.prompt_id||''),action,index})",
		"response?.action_id",
		"state?.action_result?.action_id===actionID",
		"renderActionResult(state.action_result)",
		"actionResultOutput.textContent=String(result.output||'')",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native item action UI missing contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"AUTO STATUS:",
		"READY_TO_APPLY",
		"SOURCE_DRIFT",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("native item action UI must not infer action result from terminal text: found %q", forbidden)
		}
	}
}
