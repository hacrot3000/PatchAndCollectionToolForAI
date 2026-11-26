package server

import (
 "reflect"
 "strings"
 "testing"
 "time"
 "context"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20InterruptedCollectionCheckpointProof(t *testing.T){
 for _,tc:=range []struct{name string;kind string;tail []agentchat.HistoryItem;duplicate bool;want bool}{
  {"exact","message",nil,false,true},
  {"collection","collection",nil,false,true},
  {"duplicate_marker","collection",nil,true,false},
  {"already_observed","collection",[]agentchat.HistoryItem{{Type:"function_call_output",CallID:"read",Content:"verified receipt"}},false,false},
  {"later_patch","collection",[]agentchat.HistoryItem{{Type:"function_call",CallID:"patch",Name:"taskdeck_submit_patch",Arguments:`{}`}},false,false},
  {"duplicate_call","collection",[]agentchat.HistoryItem{{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}},false,false},
 }{
  t.Run(tc.name,func(t *testing.T){
   x:=agentchat.Session{Status:"failed",ProviderRun:&agentchat.ProviderRun{ID:"r1-e1",Kind:tc.kind,Phase:"acknowledged"},
    History:append([]agentchat.HistoryItem{{Type:"user",Content:"original objective"},{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}},tc.tail...)}
   x.AppendEvent("collect_response_checkpoint","requesting_ai","read","r1-e1")
   if tc.duplicate{x.AppendEvent("collect_response_checkpoint","requesting_ai","read","r1-e1")}
   before:=append([]agentchat.HistoryItem(nil),x.History...)
   if got:=agentCloseInterruptedCollection(&x);got!=tc.want{t.Fatalf("checkpoint closed=%t want=%t",got,tc.want)}
   if !tc.want{
    if !reflect.DeepEqual(before,x.History){t.Fatal("ambiguous checkpoint modified history")}
   }else{
    if len(x.History)!=len(before)+1||!strings.Contains(x.History[len(before)].Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("missing explicitly unverified diagnostic")}
    if agentCloseInterruptedCollection(&x){t.Fatal("checkpoint closed twice")}
   }
   if x.ProviderRun.Phase!="acknowledged"{t.Fatal("ACK downgraded")}
  })
 }
}

func TestAgentCLP20StartupAcknowledgedCollectionNoReplay(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.Status="requesting_ai"
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"original objective"},{Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{}`},{Type:"function_call_output",CallID:"old-patch",Content:"verified old receipt"},{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{}`}}
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:"verified old receipt",FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 now:=time.Now().UTC()
 x.ProviderRun=&agentchat.ProviderRun{ID:"ack-run",Kind:"collection",Phase:"acknowledged",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old wire")),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
 x.PreviousResponseID="old-response"
 x.AppendEvent("collect_response_checkpoint","requesting_ai","read","ack-run")
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 calls:=0
 s.AgentChatCall=func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error){calls++;return agentchat.ResponsesResult{},nil}
 s.agentReconcilePendingOnStartup(context.Background())
 got,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.Status!="failed"||got.ProviderRun.Phase!="acknowledged"||got.PreviousResponseID!=""||!reflect.DeepEqual(got.ToolCalls,x.ToolCalls)||len(got.History)!=len(x.History)+1||!reflect.DeepEqual(got.History[:len(x.History)],x.History){t.Fatal("startup did not preserve acknowledged receipt and close interrupted read")}
 if !strings.Contains(got.History[len(got.History)-1].Content,"COLLECTION_LOCAL_INTERRUPTED"){t.Fatal("local interruption not recorded")}
 s.agentReconcilePendingOnStartup(context.Background())
 s.processAgentAutoFeedbackSession(context.Background(),store,id)
 after,err:=store.Get(id,"local");if err!=nil||calls!=0||!reflect.DeepEqual(after.History,got.History)||after.ProviderRun.Phase!="acknowledged"{t.Fatal("restart replayed or changed acknowledged run")}
}
