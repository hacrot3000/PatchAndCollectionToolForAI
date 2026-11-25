package server

import (
 "context"
 "encoding/json"
 "reflect"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCLP20ExplicitUnknownCollectionResumePreservesAuditAndAck(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:"Fix the original issue"},
  {Type:"function_call",CallID:"old-patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"old","patch_python":"print(1)"}`},
  {Type:"function_call_output",CallID:"old-patch",Content:`{"status":"PASS","run_id":"patch-run"}`},
  {Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"ls","path":"."}]}`},
  {Type:"function_call_output",CallID:"read",Content:`{"status":"PASS","results":[]}`}}
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",Result:x.History[2].Content,FeedbackSent:true,FeedbackStatus:"acknowledged"}}
 x.Status="requesting_ai"
 x.LastRequestKey="old-accepted-message"
 x.LastRequestSHA=agentchat.ProviderPayloadDigest([]byte("original task"))
 x.LastRequestCode=200
 now:=time.Now().UTC()
 oldPayload:=[]byte(`{"model":"old","input":"old ambiguous bytes"}`)
 x.ProviderRun=&agentchat.ProviderRun{ID:"old-unknown",Kind:"collection",Phase:"sending",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest(oldPayload),ProviderFingerprint:"old-provider",CreatedAt:now,UpdatedAt:now}
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if _,err=store.RecordProviderAttempt(id,"local","old-unknown",1,oldPayload,false);err!=nil{t.Fatal(err)}
 x.Status="failed";x.ProviderRun.Phase="unknown"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 original:=append([]agentchat.HistoryItem(nil),x.History...)
 oldRun:=*x.ProviderRun
 oldPatch:=x.ToolCalls[0]
 if _,ok:=agentchat.CollectionResumeReceipt(x);!ok{t.Fatal("safe canonical checkpoint not recognized")}
 for _,status:=range []string{"cancelled","requesting_ai","waiting_tool"}{other:=x;other.Status=status;if _,ok:=agentchat.CollectionResumeReceipt(other);ok{t.Fatal("unsafe status eligible")}}
 other:=x;other.ToolCalls=append([]agentchat.ToolCallRecord(nil),x.ToolCalls...);other.ToolCalls[0].FeedbackStatus="unknown"
 if _,ok:=agentchat.CollectionResumeReceipt(other);ok{t.Fatal("unconfirmed PATCH eligible")}
 calls:=0
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 args,_:=json.Marshal(map[string]string{"name":"new patch","patch_python":"# TASKDECK_PROJECT_KEY: "+identity.Key+"\n# TASKDECK_PROJECT_ID: "+identity.ID+"\nprint('new')\n"})
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if strings.Contains(string(payload),"old ambiguous bytes")||strings.Contains(string(payload),`"previous_response_id"`){t.Fatal("old provider payload replayed")}
  if !strings.Contains(string(payload),"NEW continuation")||!strings.Contains(string(payload),"Fix the original issue"){t.Fatal("original objective lost")}
  return agentchat.ResponsesResult{ResponseID:"new-response",Calls:[]agentchat.FunctionCall{{CallID:"new-patch",Name:"taskdeck_submit_patch",Arguments:string(args)}}},nil
 }
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{t.Fatal("old native read replayed");return ""}
 path:="/api/agent/sessions/"+id+"/messages"
 request:=map[string]any{"message":"Resume original task","confirm_external":true,"expected_revision":x.Revision,"request_id":"new-resume-1"}
 // Ordinary Send now legitimately authorizes a distinct new user turn;
 // an explicit resume with the WRONG old identity must still be refused.
 request["resume_collection_run_id"]="wrong-run"
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,request)
 if w.Code!=409||calls!=0{t.Fatal("wrong checkpoint admitted")}
 request["resume_collection_run_id"]="old-unknown"
 request["confirm_external"]=false
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,request)
 if w.Code!=403||calls!=0{t.Fatal("missing consent accepted")}
 request["confirm_external"]=true
 w=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,request)
 if w.Code!=200||calls!=1{t.Fatalf("new continuation failed: HTTP %d calls %d %s",w.Code,calls,w.Body)}
 saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if saved.Status!="waiting_tool"||len(saved.ToolCalls)!=2||saved.ToolCalls[1].Status!="pending"||!reflect.DeepEqual(saved.ToolCalls[0],oldPatch){t.Fatal("old ACK changed or new Patch bypassed approval")}
 if !reflect.DeepEqual(saved.History[:len(original)],original)||len(saved.CollectionResumeHistory)!=1{t.Fatal("canonical history or recovery checkpoint lost")}
 cp:=saved.CollectionResumeHistory[0]
 if !reflect.DeepEqual(cp.Previous,oldRun)||cp.Previous.Phase!="unknown"||cp.NewRunID==oldRun.ID||cp.AttemptAudit==nil||cp.AttemptAudit.Total!=1||cp.ReceiptSHA256!=agentchat.ProviderPayloadDigest([]byte(original[4].Content)){t.Fatalf("old unknown audit lost: %+v",cp)}
 // A new server instance loads the same private journal after restart.
 restarted:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets,AgentChatCall:s.AgentChatCall,AgentCollectionCall:s.AgentCollectionCall}
 reopened,err:=restarted.agentChatStore();if err!=nil{t.Fatal(err)}
 fresh,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if !reflect.DeepEqual(fresh.CollectionResumeHistory,saved.CollectionResumeHistory)||!reflect.DeepEqual(fresh.ToolCalls,saved.ToolCalls){t.Fatal("restart lost canonical recovery or ACK")}
 w=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",path,request)
 if w.Code!=200||calls!=1{t.Fatal("repeated click after restart initiated new egress")}
}
