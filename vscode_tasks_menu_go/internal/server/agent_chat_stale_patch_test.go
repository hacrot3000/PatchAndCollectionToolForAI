package server

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentLegacyStalePatchRecoveryRequiresProofOfNoExecution(t *testing.T) {
	makeSession := func() agentchat.Session {
		args := `{"name":"test","patch_python":"print(1)"}`
		x := agentchat.Session{Owner:"local", Status:"tool_unknown",
			History: []agentchat.HistoryItem{{Type:"function_call", CallID:"patch-1", Name:"taskdeck_submit_patch", Arguments:args}},
			ToolCalls: []agentchat.ToolCallRecord{{CallID:"patch-1", Name:"taskdeck_submit_patch", Status:"unknown",
				ArgumentsSHA256:agentPatchPayloadSHA256(args), Result:agentLegacyStalePatchResult}},
		}
		x.AppendEvent("turn_finished","waiting_tool","","")
		return x
	}
	x := makeSession()
	if !agentLegacyStalePatchCanRecover(x,0) { t.Fatal("definite old pre-queue refusal was not recoverable") }
	agentRecordStalePatchDenial(&x,0)
	if x.Status!="tool_rejected" || !agentRepairableToolDenial(x.ToolCalls[0]) { t.Fatal("no safe rejection recorded") }
	for _, tc := range []struct{name string; mutate func(*agentchat.Session)}{
		{"queue marker",func(x *agentchat.Session){x.AppendEvent("tool_queuing","tool_queuing","patch-1","")}},
		{"truncated journal",func(x *agentchat.Session){x.EventSeq++}},
		{"native run",func(x *agentchat.Session){x.ToolCalls[0].RunID="native-run"}},
		{"queue ambiguity",func(x *agentchat.Session){x.ToolCalls[0].Result="Patch queue handoff failed/ambiguous; inspect Patch Tool queue before retry."}},
		{"unknown provider",func(x *agentchat.Session){x.ProviderRun=&agentchat.ProviderRun{Phase:"unknown"}}},
		{"run-owned",func(x *agentchat.Session){x.ToolCalls[0].AgentRunID="run-1"}},
		{"changed arguments",func(x *agentchat.Session){x.History[0].Arguments=`{}`}},
		{"already sent",func(x *agentchat.Session){x.ToolCalls[0].FeedbackSent=true}},
	}{
		t.Run(tc.name,func(t *testing.T){x:=makeSession();tc.mutate(&x);if agentLegacyStalePatchCanRecover(x,0){t.Fatal("ambiguous history was accepted")}})
	}
}
