package agentchat

import (
 "os"

 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

// Project the existing private pre-egress attempt ledger into the guard at
// the same canonical session CAS. The sidecar Total is monotonic even after
// bounded history eviction. This counts recorded attempts, NOT confirmed
// transmission or ACK. No new attempt is authorized or replayed here.
// The previous run is used because the incoming Save may already hold the
// next PREPARED run. Reconcile before it can replace the old sidecar.
func (s *Store) reconcileCollectionGuardAttempts(x *Session,previous Session){
 g,ok:=x.ReadCollectionGuard();if !ok{return}
 old,oldOK:=previous.ReadCollectionGuard()
 if !oldOK||old.Counters.Objective.Key!=g.Counters.Objective.Key||previous.ProviderRun==nil{return}
 run:=previous.ProviderRun
 // Only a persisted run that could have crossed egress has attempts.
 if run.Phase=="prepared"{return}
 audit,err:=s.readProviderAttemptsLocked(x.ID)
 if os.IsNotExist(err){return}
 if err!=nil{
  g.AttemptAuditIncomplete=true
  x.SetCollectionGuard(g)
  return
 }
 if audit.RunID!=run.ID{
  // The existing sidecar can legitimately refer to an earlier run
  // when no attempt of the current one was recorded.
  if g.AttemptRunID==run.ID{g.AttemptAuditIncomplete=true;x.SetCollectionGuard(g)}
  return
 }
 cursor:=uint64(0)
 if g.AttemptRunID==run.ID{cursor=g.AttemptCursor}
 if audit.Total<cursor||audit.Total>1000000000{
  g.AttemptAuditIncomplete=true
  x.SetCollectionGuard(g)
  return
 }
 if audit.Total==cursor{return}
 if x.EventSeq==^uint64(0){return}
 // One journal marker per reconciled delta, not one per potentially huge
 // unlimited retry backlog. Neither native read nor meaningful progress
 // counters are modified. The bounded audit cursor and count commit together.
 delta:=audit.Total-cursor
 seq:=x.EventSeq+1
 event:=cp.CounterEvent{Sequence:seq,Identity:guardDigest([]any{x.ID,run.ID,cursor,audit.Total}),Kind:cp.HTTPAttempt}
 next,_,applied,err:=cp.ApplyCounterEvent(g.Policy,g.Counters,event,true)
 if err!=nil||!applied{return}
 if delta-1>^uint64(0)-next.HTTPAttempts{next.HTTPAttempts=^uint64(0)}else{next.HTTPAttempts+=delta-1}
 g.Counters=next
 g.AttemptRunID=run.ID
 g.AttemptCursor=audit.Total
 x.AppendEvent("clp_http_attempt",x.Status,run.CallID,"recorded_pre_egress_attempts")
 x.SetCollectionGuard(g)
}
