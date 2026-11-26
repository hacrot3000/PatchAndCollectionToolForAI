package server

import (
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestAgentNativeStopRequiresExactSingleSelection(t *testing.T){
 item:=session.ProtocolItemState{Index:1,Name:"approved.zip",Kind:"PATCH",Status:"RUNNING",RunID:"run-approved"}
 f:=&fakeAgentPatchProtocol{expectedID:"native-1",state:session.ProtocolState{Items:[]session.ProtocolItemState{item}}}
 s:=&Server{Sessions:f}
 call:=agentchat.ToolCallRecord{PatchSessionID:"native-1",QueuePath:"patchs/approved.zip",SelectedIndex:1,PromptID:"prompt-1",RunID:"run-approved"}
 if !s.agentNativeStopMatches(call){t.Fatal("exact native selection denied")}
 for _,change:=range []func(*agentchat.ToolCallRecord){
  func(c *agentchat.ToolCallRecord){c.PatchSessionID="another"},
  func(c *agentchat.ToolCallRecord){c.QueuePath="patchs/another.zip"},
  func(c *agentchat.ToolCallRecord){c.SelectedIndex=2},
  func(c *agentchat.ToolCallRecord){c.RunID="another-run"},
  func(c *agentchat.ToolCallRecord){c.PromptID=""},
 }{
  bad:=call;change(&bad)
  if s.agentNativeStopMatches(bad){t.Fatalf("mismatched selection accepted: %+v",bad)}
 }
 other:=item;other.Name="unrelated.zip";other.Index=2
 f.state.Items=append(f.state.Items,other)
 if s.agentNativeStopMatches(call){t.Fatal("multi-item native Stop admitted")}
 f.state.Items=[]session.ProtocolItemState{{Index:1,Name:"approved.zip",Kind:"PATCH",Status:"PASS",RunID:"run-approved"}}
 if s.agentNativeStopMatches(call){t.Fatal("terminal item admitted for Stop")}
}
