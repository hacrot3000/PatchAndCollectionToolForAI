package server

import (
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// SafeProviderText caps each event at 512 bytes. Keep independent diagnostic
// groups separate so a lengthy prefix digest never hides the success baseline
// or the measured token hint. These strings come only from local formatting.
func recordAgentCacheObservationDiagnostics(sink agentchat.DiagnosticSink,note,stats string){
 if sink==nil{return}
 emit:=func(event,message string){
  if message!=""{sink.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level:"info",Stage:"provider_cache",Event:event,SafeMessage:message})}
 }
 prefix,success,_:=strings.Cut(note,"; last_success_reason=")
 emit("cache.prefix",prefix)
 if success!=""{
  previous,learning,_:=strings.Cut(success,"; token_samples=")
  emit("cache.last_success_prefix","last_success_reason="+previous)
  if learning!=""{emit("cache.token_learning","token_samples="+learning)}
 }
 totals,nearZero,_:=strings.Cut(stats,"; near_zero_samples=")
 emit("cache.session_stats",totals)
 if nearZero!=""{emit("cache.near_zero","near_zero_samples="+nearZero)}
}
