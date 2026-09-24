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
		"app.attachSession(meta,!['queue','resume','history','plan','health'].includes(mode))",
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
}
