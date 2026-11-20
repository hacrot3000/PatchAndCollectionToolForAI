package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestUnifiedOperationCenterAggregatesSubsystemsAndControls(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/operationcenter.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"OPERATION CENTER",
		"['queued','running','completed','failed']",
		"async function collectGitOperations()",
		"async function collectTransferOperations()",
		"function collectPatchOperations()",
		"async function collectTaskOperations()",
		"function collectSelfUpdateOperations()",
		"function collectLocalOperations()",
		"/api/git/jobs",
		"/api/git/jobs/control",
		"/api/task-runs",
		"/api/sessions/'+encodeURIComponent(id)+'/stop",
		"TaskMenuFileTransfer?.operationSnapshot?.()",
		"TaskMenuPatchPanel?.operationSnapshot?.()",
		"TaskMenuSelfUpdate?.operationSnapshot?.()",
		"Copy error",
		"Clear completed",
		"action==='cancel'",
		"action==='retry'",
		"action==='open'",
		"globalThis.TaskMenuOperationCenter={open,close,refresh,begin",
		"setInterval(()=>{if(backdrop.classList.contains('visible'))refresh();},1500)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("operationcenter.js missing %q", want)
		}
	}
}

func TestOperationCenterAdaptersReuseExistingSubsystemControls(t *testing.T) {
	tests := []struct {
		file string
		want []string
	}{
		{
			file: "featuremods/filetransfer.js",
			want: []string{
				"async function fileTransferOperationSnapshot()",
				"async function fileTransferOperationControl(operation,action)",
				"body.action='remove_selected'",
				"body.action='retry_failed'",
				"body.action='clear_done'",
				"operationSnapshot:fileTransferOperationSnapshot",
				"operationControl:fileTransferOperationControl",
			},
		},
		{
			file: "featuremods/patchpanel.js",
			want: []string{
				"function patchOperationSnapshot()",
				"async function patchOperationControl(operation,action)",
				"'/api/sessions/'+encodeURIComponent(sessionID)+'/stop'",
				"operationSnapshot:patchOperationSnapshot",
				"operationControl:patchOperationControl",
			},
		},
		{
			file: "featuremods/selfupdate.js",
			want: []string{
				"function selfUpdateOperationSnapshot()",
				"async function selfUpdateOperationControl(_operation,action)",
				"globalThis.TaskMenuSelfUpdate={operationSnapshot:selfUpdateOperationSnapshot",
			},
		},
		{
			file: "featuremods/database.js",
			want: []string{
				"TaskMenuOperationCenter?.begin?.({",
				"title:'Database import · '",
				"title:'Database export · query result'",
				"operation?.complete?.(",
				"operation?.fail?.(",
			},
		},
		{
			file: "featuremods/database_workbench.js",
			want: []string{
				"title:'Database export · '+String(object.name||'table')",
				"operation?.complete?.({detail:base+ext})",
				"operation?.fail?.(error,{retry:()=>exportTableData(view)})",
			},
		},
	}
	for _, test := range tests {
		data, err := webassets.Files.ReadFile(test.file)
		if err != nil {
			t.Fatal(err)
		}
		js := string(data)
		for _, want := range test.want {
			if !strings.Contains(js, want) {
				t.Fatalf("%s missing Operation Center adapter contract %q", test.file, want)
			}
		}
	}
}

func TestOperationCenterLoadsAfterPatchAndActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	patch := strings.Index(js, "patchpanel.js")
	activity := strings.Index(js, "activitybar.js")
	operations := strings.Index(js, "operationcenter.js")
	if patch < 0 || activity < 0 || operations < 0 || operations < patch || operations < activity {
		t.Fatalf("Operation Center must load after Patch Panel and Activity Bar: %q", js)
	}
}
