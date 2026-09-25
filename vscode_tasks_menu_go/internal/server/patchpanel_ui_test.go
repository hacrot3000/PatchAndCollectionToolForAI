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
		"app.materializeSession(meta,false)",
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
		"constraints?.patch_priority",
		"data-patch-priority-index",
		"selectedPromptPriorities()",
		"payload.priorities=priorities",
		"Select all PATCH",
		"Clear selection",
		"input.checked=!hidden&&kind==='PATCH'",
		"clearPromptPriority(Number(input.dataset.patchIndex))",
		"mode==='queue'",
		"const visible=items.slice(0,50)",
		"setQueueSummaryView('queue')",
		"setQueueSummaryView('failed')",
		"task-patch-summary-search",
		"Search name / id / summary / target",
		"String(item?.search?.text||'')",
		"groupItems.filter(queueItemMatchesSearch)",
		"applyQueuePromptSearch()",
		"row.hidden=Boolean(queueSearchQuery)",
		"summarySearchInput.oninput",
		"No item matches search",
		"item?.group==='failed'",
		"item?.group==='new'",
		"failure?.diagnosis_kind",
		"snapshot?.group_counts",
		"state?.available===false",
		"TaskMenuPatchPanel={open,close,toggle,start,enterRunningView,finishRunningView,leaveRunningView,openTerminalEvidence,renderQueueSnapshot,setQueueSummaryView,renderQueuePrompt,selectedPromptPriorities,selectAllPromptPatches,clearPromptSelection,renderResumeSnapshot,renderResumePrompt,submitResumeAction,renderHistorySnapshot,renderHistoryPrompt,renderHistoryReport,submitHistoryDetail,submitHistoryManagement,renderHistoryManagementResult,historyItemSupportAllowed,submitHistorySupport,renderHistorySupportResult,historyCleanupProjection,renderHistoryCleanupCapability,submitHistoryCleanup,renderHistoryCleanupResult,enterPlanView,leavePlanView,renderPlanSnapshot,enterHealthView,leaveHealthView,renderHealthSnapshot,submitItemAction,submitQueueDelete,renderActionResult,renderItemLifecycle,renderProgress,renderArtifacts",
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
		"app.materializeSession(meta,false)",
		"async function openTerminalEvidence()",
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
		"app.materializeSession(meta,false)",
		"if(haveResumeSnapshot)resumeNote.textContent='Native Resume command channel unavailable. Continue in terminal.'",
		"Native command channel unavailable. Open terminal evidence/fallback if needed.",
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
		"app.materializeSession(meta,false)",
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
		"historyTerminal.onclick=()=>openTerminalEvidence().catch(app.showError)",
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


func TestPatchPanelQueuePrioritiesAreCapabilityDrivenAndNeverOrderInWeb(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function patchPriorityCapability(prompt)",
		"String(raw.response_field||'')!=='priorities'",
		"min<0||max>9||min>max",
		"function selectedPromptPriorities()",
		"priority.onchange=()=>",
		"input.checked=true",
		"applyPromptConstraints(input,prompt)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Queue priority UI missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"priorities.sort(",
		"selectedPromptPriorities().sort(",
		"payload.indexes.sort(",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("web must not own Patch execution ordering: found %q", forbidden)
		}
	}
}


func TestPatchPanelQueueSearchUsesOnlyPythonProjection(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	start := strings.Index(js, "function queueItemMatchesSearch(item)")
	if start < 0 {
		t.Fatal("queue search matcher block not found")
	}
	rest := js[start:]
	relEnd := strings.Index(rest, "function queueSearchAvailable")
	if relEnd < 0 {
		t.Fatal("queue search matcher end not found")
	}
	block := rest[:relEnd]
	if !strings.Contains(block, "queueItemSearchText(item)") {
		t.Fatal("queue search matcher must use Python-projected search text")
	}
	for _, forbidden := range []string{"item?.name", "item?.detail", "item?.kind", "failure", "manifest", ".zip"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("queue search matcher must not infer/search browser fields directly: found %q", forbidden)
		}
	}
	for _, want := range []string{
		"function queueItemSearchText(item)",
		"String(item?.search?.text||'')",
		"function snapshotItemForPromptItem(promptItem)",
		"applyQueuePromptSearch()",
		"input.checked=false",
		"clearPromptPriority(index)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Queue search missing contract %q", want)
		}
	}
}


func TestPatchPanelNativePlanIsReadOnlyPrimaryView(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function enterPlanView()",
		"panel.classList.add('plan')",
		"function leavePlanView()",
		"function renderPlanSnapshot(snapshot)",
		"snapshot.failure_policy",
		"snapshot.transaction_policy",
		"snapshot.previous_failure_action",
		"snapshot.static_conflicts",
		"snapshot.resources",
		"snapshot.previews",
		"snapshot.warnings",
		"snapshot.error",
		"state?.plan_snapshot&&!havePlanSnapshot",
		"renderPlanSnapshot(state.plan_snapshot)",
		"task-patch-panel.plan .task-patch-summary",
		"planTerminal.onclick=()=>openTerminalEvidence().catch(app.showError)",
		"planBack.onclick=leavePlanView",
		"app.materializeSession(meta,false)",
		"Native Plan snapshot unavailable or timed out. Use Terminal evidence/fallback.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Plan UI missing contract %q", want)
		}
	}
	start := strings.Index(js, "function renderPlanSnapshot(snapshot)")
	end := strings.Index(js[start:], "function enterRunningView()")
	if start < 0 || end < 0 {
		t.Fatal("native Plan renderer bounds unavailable")
	}
	renderer := js[start : start+end]
	for _, forbidden := range []string{
		"jsonFetch(",
		"/plan-action",
		"submitPlan",
		"window.confirm",
		"prompt_response",
	} {
		if strings.Contains(renderer, forbidden) {
			t.Fatalf("native Plan renderer must remain presentation-only: found %q", forbidden)
		}
	}
}

func TestPatchPanelPlanKeepsPTYAsExplicitFallbackNotPrimary(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(mode==='plan')enterPlanView();else leavePlanView();",
		"if(planMode){",
		"planStatus.textContent='PTY fallback';",
		"Use Terminal evidence/fallback.",
		"app.views.has(activeSessionId)",
		"app.activateView(activeSessionId)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Plan fallback contract missing %q", want)
		}
	}
}


func TestPatchPanelNativeHealthIsReadOnlyPrimaryView(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"['health','Health'",
		"function enterHealthView()",
		"panel.classList.add('health')",
		"function leaveHealthView()",
		"function renderHealthSnapshot(snapshot)",
		"snapshot.status",
		"snapshot.tool_version",
		"snapshot.summary",
		"snapshot.checks",
		"snapshot.warnings",
		"snapshot.errors",
		"state?.health_snapshot&&!haveHealthSnapshot",
		"renderHealthSnapshot(state.health_snapshot)",
		"task-patch-panel.health .task-patch-summary",
		"healthTerminal.onclick=()=>openTerminalEvidence().catch(app.showError)",
		"healthBack.onclick=leaveHealthView",
		"app.materializeSession(meta,false)",
		"Native Health snapshot unavailable or timed out. Use Terminal evidence/fallback.",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Health UI missing contract %q", want)
		}
	}
	start := strings.Index(js, "function renderHealthSnapshot(snapshot)")
	endOffset := strings.Index(js[start:], "function enterRunningView()")
	if start < 0 || endOffset < 0 {
		t.Fatal("native Health renderer bounds unavailable")
	}
	renderer := js[start : start+endOffset]
	for _, forbidden := range []string{
		"jsonFetch(",
		"/health-action",
		"window.confirm",
		"prompt_response",
		"SHA256SUMS",
		"PACKAGE_CONTENTS",
	} {
		if strings.Contains(renderer, forbidden) {
			t.Fatalf("native Health renderer must remain typed presentation-only: found %q", forbidden)
		}
	}
}

func TestPatchPanelHealthKeepsPTYAsExplicitFallbackNotPrimary(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(mode==='health')enterHealthView();else leaveHealthView();",
		"if(healthMode){",
		"healthStatus.textContent='PTY fallback';",
		"Native Health protocol state unavailable. Use Terminal evidence/fallback.",
		"app.views.has(activeSessionId)",
		"app.activateView(activeSessionId)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native Health fallback contract missing %q", want)
		}
	}
}


func TestPatchPanelHistorySupportIsCapabilityDrivenAndCorrelated(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function historyItemSupportAllowed(item)",
		"activeHistoryPrompt?.constraints",
		"constraints.item_actions",
		"item?.actions",
		"advertised.has('support')&&actions.has('support')",
		"function submitHistorySupport(sessionId,prompt,runID,item,sourceButton)",
		"/history-support",
		"body:JSON.stringify({prompt_id:promptID,run_id:runID,item_index:itemIndex})",
		"response?.support_id",
		"function waitForHistorySupport(sessionId,supportID,promptID,runID,itemIndex)",
		"result?.support_id===supportID",
		"String(result?.prompt_id||'')!==promptID",
		"String(result?.run_id||'')!==runID",
		"Number(result?.item_index)!==itemIndex",
		"renderHistorySupportResult(result)",
		"appendHistoryFile(historyManagementFiles,result.artifact)",
		"historySupportBusy",
		"historySupportPollGeneration",
		"support.textContent='Support'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History Support UI missing contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"item?.status==='FAIL'&&",
		"item?.kind==='PATCH'&&historyItemSupport",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("History Support must be capability-driven, found %q", forbidden)
		}
	}
}

func TestPatchPanelHistorySupportKeepsHistoryPromptReusable(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	start := strings.Index(js, "async function submitHistorySupport")
	endRel := strings.Index(js[start:], "function historyPromptRun")
	if start < 0 || endRel < 0 { t.Fatal("History support function bounds unavailable") }
	block := js[start:start+endRel]
	for _, forbidden := range []string{
		"activeHistoryPrompt=null",
		"clearPrompt()",
		"leaveHistoryView()",
	} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("History Support must not consume/clear reusable History prompt: %q", forbidden)
		}
	}
}


func TestPatchPanelHistoryCleanupIsCapabilityDrivenAndUsesPythonCounts(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function historyCleanupProjection(prompt)",
		"actions.has('cleanup')",
		"destructive.has('cleanup')",
		"constraints.cleanup",
		"remove_unpinned_idle_then_oldest_unpinned_over_limit",
		"cleanup.idle_eligible",
		"cleanup.overflow_eligible",
		"cleanup.pinned",
		"cleanup.limit",
		"historyCleanupButton.textContent=cleanup.eligible>0",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History Cleanup capability UI missing %q", want)
		}
	}
}

func TestPatchPanelHistoryCleanupIsPromptBoundCorrelatedAndListFree(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"async function submitHistoryCleanup(sessionId,prompt)",
		"/history-cleanup",
		"body:JSON.stringify({prompt_id:promptID,confirmed:true})",
		"response?.cleanup_id",
		"async function waitForHistoryCleanup(sessionId,cleanupID,promptID,snapshotBefore)",
		"result?.cleanup_id===cleanupID",
		"String(result?.prompt_id||'')!==promptID",
		"renderHistoryCleanupResult(result)",
		"snapshotToken!==snapshotBefore",
		"String(prompt?.prompt_id||'')!==promptID",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("native History Cleanup correlation UI missing %q", want)
		}
	}
	start := strings.Index(js, "async function submitHistoryCleanup")
	endRel := strings.Index(js[start:], "function historyActionsForRun")
	if start < 0 || endRel < 0 { t.Fatal("History Cleanup function bounds unavailable") }
	block := js[start:start+endRel]
	for _, forbidden := range []string{"run_id:", "run_ids", "candidates:", "paths:"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("History Cleanup browser request must not choose cleanup candidates: %q", forbidden)
		}
	}
}

func TestPatchPanelHistoryCleanupRequiresExplicitConfirmationAndPreservesPins(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"window.confirm(",
		"Python reports",
		"unpinned IDLE",
		"oldest unpinned over limit",
		"Pinned runs are preserved",
		"newest meaningful History is kept to limit",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("History Cleanup confirmation missing %q", want)
		}
	}
}


func TestPatchNativeStartDoesNotCreateTerminalTab(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	start := strings.Index(js, "async function start(mode,sourceButton=null)")
	endRel := strings.Index(js[start:], "terminalEvidence.onclick")
	if start < 0 || endRel < 0 { t.Fatal("Patch start function bounds unavailable") }
	block := js[start:start+endRel]
	if strings.Contains(block, "attachSession(") || strings.Contains(block, "materializeSession(") {
		t.Fatal("native Patch start must keep the backing PTY headless")
	}
	if !strings.Contains(js, "async function openTerminalEvidence()") ||
		!strings.Contains(js, "app.materializeSession(meta,false)") {
		t.Fatal("terminal evidence must be materialized only by the explicit fallback action")
	}
}
