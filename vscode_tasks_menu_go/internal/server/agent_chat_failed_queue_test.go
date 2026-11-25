package server

import (
 "strings"
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
)
func TestAgentFailedQueueRetirementEligibility(t *testing.T){
 call:=agentchat.ToolCallRecord{Name:"taskdeck_submit_patch",Status:"error",RunID:"native-1",QueuePath:"patchs/PATCH_CLEAR_TEXT_test.zip",PackageSHA256:strings.Repeat("a",64),Result:`{"status":"FAIL"}`}
 if !agentFailedPatchRetirementEligible(call){t.Fatal("verified terminal failure rejected")}
 for _,status:=range []string{"done","unknown","running","pending","rejected"}{
  x:=call;x.Status=status
  if agentFailedPatchRetirementEligible(x){t.Fatalf("unsafe %s eligible",status)}
 }
 x:=call;x.Result=`{"status":"PASS"}`
 if agentFailedPatchRetirementEligible(x){t.Fatal("PASS evidence eligible")}
 x=call;x.AgentRunID="other-engine"
 if agentFailedPatchRetirementEligible(x){t.Fatal("foreign engine eligible")}
 x=call;x.PackageSHA256=""
 if agentFailedPatchRetirementEligible(x){t.Fatal("missing digest eligible")}
}
