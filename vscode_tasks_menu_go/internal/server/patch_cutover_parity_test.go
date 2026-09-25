package server

import (
	"os"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPatchProtocolParityAndHeadlessBackingGate(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	for _, want := range []string{
		"['queue','Queue'",
		"['resume','Resume'",
		"['history','History'",
		"['plan','Plan'",
		"['health','Health'",
		"setQueueSummaryView('failed')",
		"Search name / id / summary / target",
		"Select all PATCH",
		"Clear selection",
		"selectedPromptPriorities()",
		"submitItemAction",
		"submitQueueDelete",
		"collect_failed",
		"delete_failed",
		"submitResumeAction",
		"renderHistoryReport",
		"submitHistoryManagement",
		"submitHistorySupport",
		"submitHistoryCleanup",
		"renderPlanSnapshot",
		"renderHealthSnapshot",
		"renderItemLifecycle",
		"renderProgress",
		"renderArtifacts",
		"Open terminal evidence",
		"app.materializeSession(meta,false)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Patch protocol/headless gate missing web contract %q", want)
		}
	}

	serverData, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	serverSrc := string(serverData)
	for _, want := range []string{
		`case "prompt-response":`,
		`case "resume-action":`,
		`case "queue-delete":`,
		`case "history-detail":`,
		`case "history-manage":`,
		`case "history-support":`,
		`case "history-cleanup":`,
	} {
		if !strings.Contains(serverSrc, want) {
			t.Fatalf("Patch protocol/headless gate missing server endpoint %q", want)
		}
	}

	if strings.Contains(js, "views.delete(activeSessionId)") {
		t.Fatal("headless native backing must retain PTY session as evidence/fallback")
	}

	uiData, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	uiSrc := string(uiData)
	for _, want := range []string{
		"return Number(meta?.task_id)!==-1;",
		"else if(autoAttachSession(meta))attach(meta,false);",
		"function materializeSession(meta,activate=true)",
	} {
		if !strings.Contains(uiSrc, want) {
			t.Fatalf("native Patch backing-session gate missing %q", want)
		}
	}

	start := strings.Index(js, "async function start(mode,sourceButton=null)")
	end := strings.Index(js[start:], "terminalEvidence.onclick")
	if start < 0 || end < 0 {
		t.Fatal("Patch start function bounds unavailable")
	}
	startBlock := js[start : start+end]
	if !strings.Contains(startBlock, "if(patchUIMode()==='terminal'){") ||
		!strings.Contains(startBlock, "app.materializeSession(meta,true)") {
		t.Fatal("legacy terminal Patch mode must explicitly materialize its terminal tab")
	}
	nativeStart := strings.Index(startBlock, "await assertHeadlessNativeSession(meta);")
	if nativeStart < 0 {
		t.Fatal("native Patch start branch unavailable")
	}
	if strings.Contains(startBlock[nativeStart:], "materializeSession(") {
		t.Fatal("starting a native Patch action must keep its backing PTY headless")
	}
	fallbackStart := strings.Index(js, "async function openTerminalEvidenceForSession(sessionId)")
	fallbackEnd := strings.Index(js[fallbackStart:], "async function openTerminalEvidence()")
	if fallbackStart < 0 || fallbackEnd < 0 {
		t.Fatal("explicit terminal evidence function bounds unavailable")
	}
	fallbackBlock := js[fallbackStart : fallbackStart+fallbackEnd]
	for _, want := range []string{"app.materializeSession(meta,false)", "app.activateView(sessionId)"} {
		if !strings.Contains(fallbackBlock, want) {
			t.Fatalf("explicit terminal evidence path missing %q", want)
		}
	}
}


func TestPatchNativeProductAcceptanceGate(t *testing.T) {
	panelData, err := webassets.Files.ReadFile("featuremods/patchpanel.js")
	if err != nil { t.Fatal(err) }
	js := string(panelData)
	uiData, err := os.ReadFile("ui.go")
	if err != nil { t.Fatal(err) }
	ui := string(uiData)
	serverData, err := os.ReadFile("server.go")
	if err != nil { t.Fatal(err) }
	server := string(serverData)

	// Built-in Patch sessions remain process-backed for compatibility but are
	// reserved as task_id=-1 and excluded from normal terminal-tab sync.
	for _, want := range []string{
		"ID:        -1,",
		"return Number(meta?.task_id)!==-1;",
		"else if(autoAttachSession(meta))attach(meta,false);",
	} {
		source := server
		if strings.Contains(want,"task_id") || strings.Contains(want,"autoAttachSession") { source=ui }
		if !strings.Contains(source,want) { t.Fatalf("headless backing acceptance missing %q",want) }
	}

	// Native Queue/Resume/History/Plan/Health stay headless, while the
	// explicitly selected legacy terminal interface materializes its session.
	start := strings.Index(js,"async function start(mode,sourceButton=null)")
	endRel := -1
	if start >= 0 { endRel=strings.Index(js[start:],"terminalEvidence.onclick") }
	if start < 0 || endRel < 0 { t.Fatal("Patch start block unavailable") }
	startBlock := js[start:start+endRel]
	if !strings.Contains(startBlock,"if(patchUIMode()==='terminal'){") ||
		!strings.Contains(startBlock,"app.materializeSession(meta,true)") {
		t.Fatal("legacy terminal mode materialization contract missing")
	}
	nativeStart := strings.Index(startBlock,"await assertHeadlessNativeSession(meta);")
	if nativeStart < 0 { t.Fatal("native Patch start branch unavailable") }
	if strings.Contains(startBlock[nativeStart:],"materializeSession(") {
		t.Fatal("native Patch start still materializes a terminal tab")
	}
	for _, want := range []string{
		"await assertHeadlessNativeSession(meta);",
		"app.activateExternalView('patch')",
		".task-patch-panel{display:none;position:absolute;inset:0",
		"patchTab.className='tab task-patch-tab'",
		"patchTab.onclick=()=>open()",
		"const current=String(app.active||'')",
		"await start('history');",
	} {
		if !strings.Contains(js,want) { t.Fatalf("native tab/navigation acceptance missing %q",want) }
	}
	for _, forbidden := range []string{
		"body.task-patch-workspace-active main>section{visibility:hidden}",
		"position:fixed;top:52px",
	} {
		if strings.Contains(js,forbidden) { t.Fatalf("Patch tab acceptance still contains obsolete overlay contract %q",forbidden) }
	}

	// Native evidence still has exactly one materialize(false) implementation;
	// legacy mode uses the separate materialize(true) branch above.
	if got:=strings.Count(js,"app.materializeSession(meta,false)"); got!=1 {
		t.Fatalf("native evidence materialization paths=%d want 1",got)
	}
	if got:=strings.Count(js,"app.materializeSession(meta,true)"); got!=2 {
		t.Fatalf("explicit legacy terminal materialization paths=%d want 2 (global legacy UI + History Terminal)",got)
	}
	if !strings.Contains(js,"openTerminalEvidenceForSession(run.sessionId)") {
		t.Fatal("parallel COLLECT terminal evidence must use the same explicit materialization path")
	}
	if strings.Contains(js,"openTerminalEvidence();") {
		t.Fatal("implicit terminal evidence call remains in normal Patch navigation")
	}
	if got:=strings.Count(js,"openTerminalEvidence().catch(app.showError)"); got!=3 {
		t.Fatalf("explicit backing-evidence button bindings=%d want 3",got)
	}
	if !strings.Contains(js,"historyTerminal.onclick=()=>openLegacyHistoryTerminal().catch(app.showError)") {
		t.Fatal("History Terminal must launch a separate explicit legacy History session")
	}

	// Reload/session polling must keep headless Patch sessions out of normal tabs.
	if !strings.Contains(ui,"setInterval(()=>{if(!browserLeaseLost)syncSessions().catch(()=>{});},1000);") ||
		!strings.Contains(ui,"else if(autoAttachSession(meta))attach(meta,false);") {
		t.Fatal("reload/session synchronization does not preserve headless Patch backing sessions")
	}
}
