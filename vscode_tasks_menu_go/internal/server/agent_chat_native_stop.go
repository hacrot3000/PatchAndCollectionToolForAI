package server

import (
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/session"
)

// Only the exact approved selected item in its own native session may be
// stopped. A missing/mismatched or multi-item protocol is not permission.
func(s *Server)agentNativeStopMatches(call agentchat.ToolCallRecord)bool{
 if call.SelectedIndex<1||call.PromptID==""||call.QueuePath!="patchs/"+filepath.Base(call.QueuePath)||filepath.Base(call.QueuePath)=="."{return false}
 provider,ok:=s.Sessions.(session.ProtocolStateProvider);if !ok{return false}
 state,e:=provider.ProtocolState(call.PatchSessionID);if e!=nil{return false}
 matches:=0
 for _,item:=range state.Items{
  status:=strings.ToUpper(item.Status)
  if status!="RUNNING"&&status!="QUEUED"&&status!="PENDING"{continue}
  if item.Name!=filepath.Base(call.QueuePath)||item.Index!=call.SelectedIndex||!strings.EqualFold(item.Kind,"PATCH"){return false}
  if call.RunID!=""&&item.RunID!=call.RunID{return false}
  matches++
 }
 return matches==1
}
