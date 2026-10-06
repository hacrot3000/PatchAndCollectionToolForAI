package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTaskRunHistoryUIProvidesDurableDetailsAndCompare(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function loadServerTaskRuns()",
		"async function persistServerTaskRun(view,meta)",
		"/api/task-runs",
		"/api/task-runs/log?id=",
		"async function openTaskRunDetail(item)",
		"async function compareSelectedTaskRuns()",
		"Select exactly two task runs to compare.",
		"Download log",
		"Run task again",
		"Git commit",
		"Project profile",
		"Target",
		"Log",
		"taskRunCompareSelection.size!==2",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("task run history UI missing %q", want)
		}
	}
}

func TestTaskRunHistoryUIRecordsOnlyFinishedTaskSessions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/all.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"if(!view||!meta||meta.task_id<=0||meta.status==='running')return null",
		"if(meta.task_id>0&&meta.status!=='running'&&!finalizedSessions.has(meta.id))",
		"project_profile_id:taskRunProjectProfileID()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("task run persistence guard missing %q", want)
		}
	}
}
