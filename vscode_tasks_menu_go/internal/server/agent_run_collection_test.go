package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestPendingRunCollectionRequiresSingleUnresolvedExactCall(t *testing.T) {
	collect := agentchat.HistoryItem{Type: "function_call", CallID: "collect-1", Name: "taskdeck_collect_workspace"}
	patch := agentchat.HistoryItem{Type: "function_call", CallID: "patch-1", Name: "taskdeck_submit_patch"}
	output := agentchat.HistoryItem{Type: "function_call_output", CallID: collect.CallID}
	for _, tc := range []struct{
		name string
		history []agentchat.HistoryItem
		want bool
	}{
		{"single", []agentchat.HistoryItem{collect}, true},
		{"already reconciled", []agentchat.HistoryItem{collect,output}, false},
		{"mixed patch", []agentchat.HistoryItem{collect,patch}, false},
		{"patch only", []agentchat.HistoryItem{patch}, false},
	}{
		t.Run(tc.name,func(t *testing.T){
			got,ok:=pendingRunCollection(tc.history)
			if ok!=tc.want || ok && got.CallID!=collect.CallID {t.Fatalf("unexpected pending call: %+v %v",got,ok)}
		})
	}
}
