package server

import "bletonfc/vscode_tasks_menu/internal/agentchat"

// A verified provider-side conversation retains the FULL previous context
// even when TaskDeck sends only Next. Rebase to one canonical, compacted,
// stateless input after the first tool exchange (or a long plain transcript).
// This policy never mutates the selected session mode, and must preserve the
// pending function call when the next input is a function_call_output.
func agentContextNeedsRebase(x agentchat.Session) bool {
 if x.ArchivedHistoryItems>0 || x.ArchivedPatchCalls>0 {return true}
 if len(x.History)>40{return true}
 for _,item:=range x.History {
  if item.Type=="function_call" && (item.Name=="taskdeck_collect_workspace" || item.Name=="taskdeck_submit_patch"){
   return true
  }
 }
 return false
}
func agentContextRequestMode(x agentchat.Session,providerAPI string,conversationVerified bool) agentchat.ContextMode{
 if providerAPI=="chat_completions" || agentContextNeedsRebase(x) {return agentchat.Stateless}
 if x.Mode==agentchat.Conversation && !conversationVerified {return agentchat.Stateless}
 return x.Mode
}
func agentClearProviderRefsForStateless(x *agentchat.Session,mode agentchat.ContextMode){
 if x!=nil && mode==agentchat.Stateless{
  x.PreviousResponseID=""
  x.ConversationID=""
 }
}
