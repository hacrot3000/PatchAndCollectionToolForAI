package server

import (
 "context"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Exercise the real per-session lease, not a provider heartbeat. A heartbeat
// can expire while a slow model still runs; a held turn lease must not.
func TestStartupReconcileLeavesLiveProviderTurnUnchanged(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 payload:=[]byte(`{"model":"m","input":"synthetic still-running turn"}`)
 x.Status="requesting_ai"
 x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{
  ID:"r-live-e1",Kind:"message",Phase:"sending",
  PayloadSHA256:agentchat.ProviderPayloadDigest(payload),
  CreatedAt:now,UpdatedAt:now,
 }
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 release,ok:=lockAgentTurn(s.Workspace,"local",id)
 if !ok{t.Fatal("failed to acquire test live worker lease")}
 released:=false
 defer func(){if !released{release()}}()

 // A concurrent second daemon sees SENDING but must not infer crash while
 // the real worker still owns this exact OS/process session lease.
 s.agentReconcilePendingOnStartup(context.Background())
 running,err:=store.Get(id,"local")
 if err!=nil||running.Status!="requesting_ai"||
  running.ProviderRun.Phase!="sending"||running.Revision!=x.Revision{
  t.Fatalf("startup overwrote a still-live provider turn: %+v %v",running,err)
 }
 // An explicit Stop is still allowed only through its existing owner
 // authorized flow; startup does not hijack a live lease.
 release();released=true
 s.agentReconcilePendingOnStartup(context.Background())
 unknown,err:=store.Get(id,"local")
 if err!=nil||unknown.Status!="failed"||unknown.ProviderRun.Phase!="unknown"||
  unknown.Revision!=x.Revision+1{
  t.Fatalf("released crashed worker was not fenced as UNKNOWN: %+v %v",unknown,err)
 }
 if review:=agentchat.ProviderEgressReviewForSession(unknown);review==nil||
  !review.RequiresReconciliation||review.AutomaticReplayAllowed{
  t.Fatal("released worker was incorrectly acknowledged or made replayable")
 }
 // Startup reconciliation must be idempotent after the first UNKNOWN.
 s.agentReconcilePendingOnStartup(context.Background())
 after,err:=store.Get(id,"local")
 if err!=nil||after.Revision!=unknown.Revision{
  t.Fatalf("repeated startup changed already-reconciled run: %+v %v",after,err)
 }
}

func TestStartupReconcileReloadsAuthoritativeJournalInsteadOfStaleSnapshot(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 original,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 // The startup scanner could have fetched CREATED before another process
 // committed a live provider turn. The reconciler must re-read under lease.
 now:=time.Now().UTC()
 original.Status="requesting_ai"
 original.ProviderRun=&agentchat.ProviderRun{
  ID:"r-fresh-e1",Kind:"message",Phase:"prepared",
  PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("exact staged bytes")),
  CreatedAt:now,UpdatedAt:now,
 }
 newer,err:=store.Save(original,original.Revision);if err!=nil{t.Fatal(err)}
 s.agentReconcileOneStartupSession(context.Background(),store,id)
 current,err:=store.Get(id,"local")
 if err!=nil||current.Revision!=newer.Revision||
  current.ProviderRun.Phase!="prepared"||current.Status!="requesting_ai"{
  t.Fatalf("fresh PREPARED run should remain eligible only for its exact-byte recovery: %+v %v",current,err)
 }
}
