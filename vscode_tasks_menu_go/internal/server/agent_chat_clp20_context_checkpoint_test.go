package server

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Inject a target-format build failure after a completed native read.
// Actual provider limits are tested separately by the final-wire regression.
func TestAgentCLP20LocalContextCheckpointPreservesReceiptAcrossRestart(t *testing.T){
 for _,fallback:=range []bool{false,true}{
  t.Run(map[bool]string{false:"typed",true:"fallback"}[fallback],func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
   identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
   x.History=[]agentchat.HistoryItem{{Type:"user",Content:"Original owner task: inspect missing scope"}}
   x.Status="requesting_ai"
   now:=time.Now().UTC()
   x.ProviderRun=&agentchat.ProviderRun{ID:"old-ack",Kind:"message",Phase:"acknowledged",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old request")),ProviderFingerprint:"fixture",CreatedAt:now,UpdatedAt:now}
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   reads,calls:=0,0
   receipt:=`{"status":"PASS","run_id":"synthetic-native-result","text":"exact observation"}`
   s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{reads++;return receipt}
   s.AgentChatCall=func(context.Context,agentchat.ProviderConfig,string,[]byte)(agentchat.ResponsesResult,error){calls++;t.Fatal("build failure crossed provider boundary");return agentchat.ResponsesResult{},nil}
   first:=agentchat.ResponsesResult{ResponseID:"old-response",Calls:[]agentchat.FunctionCall{{CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`}}}
   if fallback{
    raw,e:=json.Marshal(map[string]any{"project":map[string]string{"key":identity.Key,"id":identity.ID},"actions":[]map[string]string{{"type":"file","path":"a"}}});if e!=nil{t.Fatal(e)}
    first.Calls=nil;first.Text=string(raw)
   }
   _,clear,err:=s.agentChatRunCollection(context.Background(),store,&x,first,agentchat.ProviderConfig{APIType:"synthetic-invalid-format"},agentchat.ModelConfig{ID:x.ModelID,MaxInputTokens:65536},"key",identity,true)
   var local *agentCollectionLocalError
   if !errors.As(err,&local)||local.Stage!="collection_context_checkpoint"||!clear||reads!=1||calls!=0{t.Fatalf("bad checkpoint: %v reads=%d calls=%d",err,reads,calls)}
   x.Status="failed"
   agentCloseInterruptedCollection(&x)
   x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
   reopened,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
   saved,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
   want:=receipt;if fallback{want=agentchat.NativeCollectionTextObservationPrefix+receipt}
   if saved.History[len(saved.History)-1].Content!=want||saved.ProviderRun.ID!="old-ack"||saved.ProviderRun.Phase!="acknowledged"||len(saved.ToolCalls)!=0{t.Fatal("canonical observation or ACK lost")}
   observed,checkpoint:=0,0
   for _,e:=range saved.Events{if e.Kind=="collect_observed"{observed++};if e.Kind=="collect_context_checkpoint"&&e.Detail=="local_request_not_prepared"{checkpoint++}}
   if observed!=1||checkpoint!=1{t.Fatal("checkpoint event missing or duplicate")}
   s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
    calls++
    if !strings.Contains(string(payload),"Original owner task")||!strings.Contains(string(payload),"Continue with a narrower scope"){t.Fatal("new continuation lost owner objective")}
    return agentchat.ResponsesResult{ResponseID:"new-response",Text:"Ready for narrower scope."},nil
   }
   w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",map[string]any{"message":"Continue with a narrower scope","confirm_external":true,"expected_revision":saved.Revision})
   if w.Code!=200||calls!=1||reads!=1{t.Fatalf("new continuation failed or replayed read: %d %s",w.Code,w.Body)}
   final,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
   found:=0
   for _,it:=range final.History{if it.Content==want{found++}}
   if found!=1||final.ProviderRun.ID=="old-ack"{t.Fatal("old receipt lost or old provider run replayed")}
  })
 }
}
