package server

import (
 "context"
 "errors"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
)

// Only invoked after successful provider completion for this exact immutable
// history. It cannot create a native receipt or accept a task milestone.
func (s *Server) acknowledgeRunPatchFeedback(ctx context.Context, owner string, spec agentchat.RunSpec, history []agentchat.HistoryItem) error {
 calls := map[string]string{}
 outputs := map[string]string{}
 for _, item := range history {
  if item.OriginRunID != spec.RunID || item.ProviderID != spec.ProviderID || item.ModelID != spec.ModelID { continue }
  if item.Type == "function_call" && item.Name == "taskdeck_submit_patch" { calls[item.CallID] = agentPatchPayloadSHA256(item.Arguments) }
  if item.Type == "function_call_output" { outputs[item.CallID] = item.Content }
 }
 if len(calls)==0 || len(outputs)==0 { return nil }
 inputs, err := s.ResolveAgentRunInputs(agentrun.WithPrincipalOwner(ctx,owner),spec)
 if err != nil { return err }
 release, ok := lockAgentTurn(inputs.OwnerWorkspacePath, owner, spec.ConversationID)
 if !ok { return errors.New("run feedback acknowledgement requires conversation review") }
 defer release()
 store,err := s.agentChatStore()
 if err != nil { return err }
 session,err := store.Get(spec.ConversationID,owner)
 if err != nil { return err }
 if session.ProjectID != inputs.ProjectID { return errors.New("run feedback acknowledgement scope mismatch") }
 changed:=false
 for i:=range session.ToolCalls {
  call:=&session.ToolCalls[i]
  if call.AgentRunID!=spec.RunID || calls[call.CallID]=="" || outputs[call.CallID]=="" { continue }
  if call.Name!="taskdeck_submit_patch" || call.ArgumentsSHA256!=calls[call.CallID] || call.Result!=outputs[call.CallID] || !call.FeedbackSent {
   return errors.New("run feedback receipt does not match the acknowledged provider input")
  }
  if call.FeedbackStatus=="acknowledged" { continue }
  if call.Status!="done" && call.Status!="error" && call.Status!="rejected" { return errors.New("run feedback receipt is not final") }
  if call.FeedbackStatus!="prepared" { return errors.New("run feedback delivery requires review") }
  call.FeedbackStatus="acknowledged"
  session.AppendEvent("feedback_acknowledged",session.Status,call.CallID,"agent_run")
  changed=true
 }
 if !changed { return nil }
 if session.Status=="requesting_ai" { session.Status="completed" }
 _,err=store.Save(session,session.Revision)
 return err
}
