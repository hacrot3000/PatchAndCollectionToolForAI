package server

import (
 "context"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func receivedResponseRecoveryFixture(t *testing.T)(*Server,*agentchat.Store,agentchat.Session,agentchat.Session){
 t.Helper()
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Add(-time.Minute)
 x.Status="requesting_ai"
 x.RetryMode=agentchat.RetryUnlimited
 x.ProviderRun=&agentchat.ProviderRun{ID:"r-response-test",Kind:"message",Phase:"sending",
  PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("original exact request")),CreatedAt:now,UpdatedAt:now}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"original task"})
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 candidate:=x
 run:=*x.ProviderRun
 candidate.ProviderRun=&run
 candidate.History=append(append([]agentchat.HistoryItem(nil),x.History...),agentchat.HistoryItem{Type:"assistant",Content:"Exact received response: tiếng Việt 🔍"})
 candidate.Events=append([]agentchat.SessionEvent(nil),x.Events...)
 candidate.Status="completed"
 markAgentProviderOutcome(&candidate,"completed")
 candidate.AppendEvent("turn_finished","completed","","")
 return s,store,x,candidate
}

func TestReceivedProviderResponseCheckpointRecoversWithoutReplay(t *testing.T){
 s,store,base,candidate:=receivedResponseRecoveryFixture(t)
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;t.Fatal("response recovery attempted provider egress");return agentchat.ResponsesResult{},nil
 }
 if err:=store.StageProviderResponse(candidate);err!=nil{t.Fatal(err)}
 // Simulate a crash after checkpoint fsync and before primary journal Save.
 // An active lease must still prevent another worker from taking ownership.
 release,ok:=lockAgentTurn(s.Workspace,"local",base.ID);if !ok{t.Fatal("turn lease")}
 s.agentReconcileOneStartupSession(context.Background(),store,base.ID)
 got,err:=store.Get(base.ID,"local")
 release()
 if err!=nil||got.Revision!=base.Revision||got.Status!="requesting_ai"{t.Fatalf("live lease was overwritten: %v",err)}
 // Reopen the same private store to emulate restart.
 reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 s.agentReconcileOneStartupSession(context.Background(),reopened,base.ID)
 got,err=reopened.Get(base.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Revision!=base.Revision+1||got.Status!="completed"||got.ProviderRun.Phase!="completed"||
  len(got.History)!=len(candidate.History)||got.History[len(got.History)-1].Content!=candidate.History[len(candidate.History)-1].Content{
  t.Fatal("exact local response was not committed once")
 }
 s.agentReconcileOneStartupSession(context.Background(),reopened,base.ID)
 again,err:=reopened.Get(base.ID,"local");if err!=nil||again.Revision!=got.Revision||calls!=0{t.Fatal("recovery was not idempotent")}
}

func TestReceivedProviderResponseNormalSavePreservesAutomation(t *testing.T){
 s,store,base,candidate:=receivedResponseRecoveryFixture(t)
 if _,err:=store.UpdateAutomation(base.ID,"local",base.Workspace,base.Revision,"continue",true,"");err!=nil{t.Fatal(err)}
 saved,err:=s.saveAgentReceivedResponse(store,candidate)
 if err!=nil{t.Fatal(err)}
 if !saved.AutoContinueText||saved.Status!="completed"||saved.Revision!=base.Revision+1{t.Fatal("response save lost owner preference")}
 if _,found,err:=store.RecoverProviderResponse(base.ID,"local");err!=nil||found{t.Fatal("completed response eligible for duplicate recovery")}
}

func TestReceivedProviderResponseInvalidProjectionClosesWithoutThinkingForever(t *testing.T){
 s,store,base,candidate:=receivedResponseRecoveryFixture(t)
 // A response that cannot pass canonical validation must never become an
 // executable tool proposal, even when it has a valid checkpoint checksum.
 candidate.History=append(candidate.History,agentchat.HistoryItem{Type:"function_call_output",CallID:"orphan",Content:"unverified"})
 if _,err:=s.saveAgentReceivedResponse(store,candidate);err==nil{t.Fatal("invalid response was committed")}
 if !s.recoverAgentReceivedResponseOne(context.Background(),store,base.ID){t.Fatal("abandoned response was not handled")}
 got,err:=store.Get(base.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ProviderRun.Phase!="unknown"||len(got.History)!=len(base.History){t.Fatal("invalid response was exposed or left Thinking")}
 if got.Events[len(got.Events)-1].Detail!="provider_response_save_failed"{t.Fatal("missing precise local save failure marker")}
}

func TestReceivedProviderResponseLegacyOrphanWithNoBodyCloses(t *testing.T){
 s,store,base,_:=receivedResponseRecoveryFixture(t)
 // An old installation may have received HTTP 200 but not saved the body.
 // No checkpoint means no recovery of text and no permission to replay.
 if !agentAutoFeedbackNeedsWork(base){t.Fatal("stale sending marker not scheduled")}
 if !s.recoverAgentReceivedResponseOne(context.Background(),store,base.ID){t.Fatal("orphan not handled")}
 got,err:=store.Get(base.ID,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ProviderRun.Phase!="unknown"||len(got.History)!=len(base.History){t.Fatal("orphan was not safely closed")}
 if !strings.Contains(got.Events[len(got.Events)-1].Detail,"orphaned_provider_run"){t.Fatal("missing recovery event")}
}
