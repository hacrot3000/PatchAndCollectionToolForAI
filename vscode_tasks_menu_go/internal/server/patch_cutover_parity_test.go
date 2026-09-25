package server

import (
	"os"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestPatchFinalNativeCutoverParityGate(t *testing.T) {
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
			t.Fatalf("final native parity gate missing web contract %q", want)
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
			t.Fatalf("final native parity gate missing server endpoint %q", want)
		}
	}

	if strings.Contains(js, "views.delete(activeSessionId)") {
		t.Fatal("native cutover must retain PTY session as evidence/fallback")
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
	if strings.Contains(startBlock, "attachSession(") || strings.Contains(startBlock, "materializeSession(") {
		t.Fatal("starting a native Patch action must not materialize a terminal tab")
	}
	fallbackStart := strings.Index(js, "async function openTerminalEvidence()")
	fallbackEnd := strings.Index(js[fallbackStart:], "function clearActionResult()")
	if fallbackStart < 0 || fallbackEnd < 0 {
		t.Fatal("explicit terminal evidence function bounds unavailable")
	}
	fallbackBlock := js[fallbackStart : fallbackStart+fallbackEnd]
	for _, want := range []string{"app.materializeSession(meta,false)", "app.activateView(activeSessionId)"} {
		if !strings.Contains(fallbackBlock, want) {
			t.Fatalf("explicit terminal evidence path missing %q", want)
		}
	}
}
