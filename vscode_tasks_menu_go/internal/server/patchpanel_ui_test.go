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
		"app.attachSession(meta,!['queue','resume','history'].includes(mode))",
		"/protocol",
		"const maxAttempts=followLifecycle?7200:40",
		"for(let attempt=0;attempt<maxAttempts;attempt+=1)",
		"renderQueueSnapshot(state.queue_snapshot)",
		"state?.prompt&&renderResumePrompt(sessionId,state.prompt)",
		"state?.prompt&&renderQueuePrompt(sessionId,state.prompt)",
		"renderResumeSnapshot(state.resume_snapshot)",
		"/resume-action",
		"submitResumeAction",
		"prompt?.prompt_kind!=='resume_action'",
		"failed_indexes",
		"can_retry",
		"can_collect",
		"can_delete",
		"mode==='queue'||mode==='resume'||mode==='history'",
		"renderItemLifecycle(state?.items)",
		"enterRunningView()",
		"finishRunningView()",
		"Open terminal evidence",
		"app.activateView(activeSessionId)",
		"panel.classList.add('running')",
		"runningBack.hidden=false",
		"renderProgress(state?.progress)",
		"if(state?.action_result)renderActionResult(state.action_result)",
		"/item-action",
		"response?.action_id",
		"waitForActionResult",
		"state?.action_result?.action_id===actionID",
		"activeQueuePrompt?.item_actions",
		"activeQueuePrompt?.queue_actions",
		"promptItemForQueueItem(item)",
		"/queue-delete",
		"response?.mutation_id",
		"waitForQueueMutation",
		"state?.queue_mutation_result",
		"window.confirm",
		"task-patch-queue-delete",
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
		"TaskMenuPatchPanel={open,close,toggle,start,enterRunningView,finishRunningView,leaveRunningView,openTerminalEvidence,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,renderResumeSnapshot,renderResumePrompt,submitResumeAction,renderHistorySnapshot,renderHistoryPrompt,renderHistoryReport,submitHistoryDetail,submitHistoryManagement,renderHistoryManagementResult,submitItemAction,submitQueueDelete,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts",
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
		"failure_policy===",
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


func TestPatchPanelRunningViewKeepsPTYAsSecondaryEvidence(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(action==='select')enterRunningView()",
		"app.attachSession(meta,!['queue','resume','history'].includes(mode))",
		"function openTerminalEvidence()",
		"app.views.has(activeSessionId)",
		"app.activateView(activeSessionId)",
		"finishRunningView();",
		"runningBack.onclick=()=>{if(runningFinished)leaveRunningView();}",
		"task-patch-panel.running .task-patch-summary",
		"task-patch-panel.running .task-patch-prompt",
		"task-patch-panel.running .task-patch-actions",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Running view missing contract %q", want)
		}
	}
	if strings.Contains(js, "views.delete(activeSessionId)") {
		t.Fatal("native Running view must retain the PTY session as evidence")
	}
}


func TestPatchPanelNativeResumeUsesPythonPromptContract(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function renderResumeSnapshot(snapshot)",
		"function renderResumePrompt(sessionId,prompt)",
		"prompt?.prompt_kind!=='resume_action'",
		"Array.isArray(prompt.options)?prompt.options:[]",
		"option?.available!==true",
		"String(option?.label||action)",
		"String(option?.description||'')",
		"resumeFailedRows(prompt)",
		"item?.can_retry",
		"item?.can_collect",
		"item?.can_delete",
		"resumeSelectionActions(prompt)",
		"JSON.stringify(payload)",
		"/resume-action",
		"payload.failed_indexes=indexes",
		"Recovery policy and availability come from the Python Patch Tool.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Resume UI missing Python-owned contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"previous_status==='FAIL'",
		"diagnosis_kind==='",
		"failure.status==='FAIL'",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("native Resume UI must not infer recovery policy: found %q", forbidden)
		}
	}
}

func TestPatchPanelNativeResumeKeepsPTYFallbackAndDestructiveConfirm(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"app.attachSession(meta,!['queue','resume','history'].includes(mode))",
		"if(haveResumeSnapshot)resumeNote.textContent='Native Resume command channel unavailable. Continue in terminal.'",
		"if(sessionId===activeSessionId)openTerminalEvidence()",
		"action==='delete_failed'&&!window.confirm",
		"if(action==='history')",
		"openTerminalEvidence();",
		"['all','failed','remaining','collect_failed'].includes(action)",
		"void pollProtocol(sessionId,true,true)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Resume fallback/safety contract missing %q", want)
		}
	}
}


func TestPatchPanelNativeHistoryUsesProjectedReadOnlyProtocol(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function renderHistorySnapshot(snapshot)",
		"function renderHistoryPrompt(sessionId,prompt)",
		"function renderHistoryReport(report)",
		"function submitHistoryDetail(sessionId,prompt,runID)",
		"function submitHistoryManagement(sessionId,prompt,action,runID)",
		"function waitForHistoryManagement(sessionId,managementID,promptID,runID,snapshotBefore)",
		"historyActionsForRun(runID)",
		"/history-manage",
		"history_management_result",
		"management_id",
		"result?.history_changed!==true",
		"snapshotToken!==snapshotBefore",
		"String(prompt?.prompt_id||'')!==promptID",
		"action==='delete'",
		"window.confirm",
		"confirmed",
		"renderHistoryManagementResult",
		"appendHistoryFile(historyManagementFiles,result.artifact)",
		"prompt?.prompt_kind!=='history_action'",
		"state?.history_snapshot",
		"state?.history_report",
		"/history-detail",
		"waitForHistoryReport",
		"prompt_id:String(prompt.prompt_id||'')",
		"advertised.has(runID)",
		"/api/files/download?path=",
		"taskmenu:project-file-open-request",
		"app.attachSession(meta,!['queue','resume','history'].includes(mode))",
		"historyBack.onclick=()=>stopHistoryAndBack()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History UI missing contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"LAST_RUN.json",
		"PINNED_RUNS.json",
		"unresolved_failures",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("native History UI must not read internal Patch Tool state %q", forbidden)
		}
	}
}

func TestPatchPanelHistoryKeepsTerminalAsFallbackNotPrimary(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"historyTerminal.onclick=openTerminalEvidence",
		"Native History command channel unavailable. Use Terminal fallback.",
		"Native History prompt timed out. Use Terminal fallback.",
		"mode==='history'",
		"renderHistoryPrompt(sessionId,state.prompt)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History fallback contract missing %q", want)
		}
	}
	if strings.Contains(js, "History uses PTY") {
		t.Fatal("History must no longer default to PTY rendering")
	}
}

func TestPatchPanelNativeQueueDeleteWaitsForPythonRefresh(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function queueActionAllowed(action)",
		"activeQueuePrompt?.queue_actions",
		"function submitQueueDelete(item,promptItem)",
		"window.confirm(`Delete ${name} from patchs/? This cannot be undone.`)",
		"JSON.stringify({prompt_id:promptID,index})",
		"/queue-delete",
		"response?.mutation_id",
		"state?.queue_mutation_result",
		"result?.mutation_id===mutationID",
		"String(prompt.prompt_id)!==oldPromptID",
		"renderQueueSnapshot(state.queue_snapshot)",
		"renderQueuePrompt(sessionId,prompt)",
		"Number(result.remaining)===0",
		"items.length===0",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Queue delete UI missing contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"latestQueueSnapshot.items.splice",
		"activeQueuePrompt.items.splice",
		"promptItem.index--",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("Queue delete UI must not locally mutate/reindex Python state: found %q", forbidden)
		}
	}
}


func TestPatchPanelHistoryManagementUsesOnlyAdvertisedPerRunCapabilities(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const allowed=historyActionsForRun(runID)",
		"for(const action of ['pin','unpin','export','delete'])",
		"if(!allowed.has(action))continue",
		"manage.dataset.historyAction=action",
		"historyManagementBusy||historyBusy",
		"History action ${action} is not advertised for this run",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History management capability gate missing %q", want)
		}
	}
}

func TestPatchPanelHistoryMutationWaitsForPythonRefresh(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"matched?.history_changed===true&&state?.history_snapshot",
		"const snapshotToken=JSON.stringify(state.history_snapshot)",
		"snapshotToken!==snapshotBefore&&(runs.length===0||freshPrompt)",
		"renderHistorySnapshot(state.history_snapshot)",
		"if(runs.length&&freshPrompt)renderHistoryPrompt(sessionId,prompt)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History mutation refresh contract missing %q", want)
		}
	}
}
