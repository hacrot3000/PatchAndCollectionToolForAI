package server

import (
 "context"
 "net/http"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func receiptResendFixture()agentchat.Session{
 now:=time.Now().UTC()
 return agentchat.Session{Owner:"local",Status:"failed",ProviderRun:&agentchat.ProviderRun{
  ID:"r9-e8",Kind:"collection",Phase:"unknown",CallID:"read",PayloadSHA256:strings.Repeat("a",64),CreatedAt:now,UpdatedAt:now,
 },History:[]agentchat.HistoryItem{
  {Type:"user",Content:"original task"},
  {Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"read",Arguments:`{}`},
  {Type:"function_call_output",CallID:"read",Content:`{"status":"INCOMPLETE","excerpt":"tiếng Việt"}`},
 }}
}

func TestReceiptResendCandidateCollectionAndLaterFailedMessage(t *testing.T){
 x:=receiptResendFixture()
 target:=agentReceiptResendCandidate(x)
 if target==nil||target.Kind!="collection"||target.Index!=2||target.CallID!="read"{t.Fatal("current COLLECT missing")}
 old:=*x.ProviderRun
 x.ProviderRun=&agentchat.ProviderRun{ID:"r10-e10",Kind:"message",Phase:"unknown"}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"continue original task"})
 x.CollectionResumeHistory=[]agentchat.CollectionResumeCheckpoint{{Previous:old,NewRunID:x.ProviderRun.ID,ReceiptSHA256:target.SHA256}}
 later:=agentReceiptResendCandidate(x)
 if later==nil||later.SHA256!=target.SHA256||later.Index!=2||later.RunID!=x.ProviderRun.ID{t.Fatal("exact previous receipt missing after failed continuation")}
 x.ProviderRun.ID="unrelated-run"
 if agentReceiptResendCandidate(x)!=nil{t.Fatal("unrelated later failure offered old receipt")}
 x.ProviderRun.ID="r11-e13"
 x.Events=[]agentchat.SessionEvent{
  {Seq:11,Kind:"tool_receipt_resend",CallID:"read",Detail:target.SHA256},
  {Seq:12,Kind:"turn_started"},{Seq:13,Kind:"provider_egress",Detail:"message"},
 }
 if again:=agentReceiptResendCandidate(x);again==nil||again.SHA256!=target.SHA256{t.Fatal("failed resend cannot retry")}
 x.Status="completed"
 if agentReceiptResendCandidate(x)!=nil{t.Fatal("completed turn offered resend")}
}

func TestReceiptResendCandidatePatchRequiresExactUnknownFeedback(t *testing.T){
 x:=receiptResendFixture()
 x.ProviderRun.Kind="feedback";x.ProviderRun.CallID="patch"
 x.History[1].Name="taskdeck_submit_patch";x.History[1].CallID="patch";x.History[2].CallID="patch"
 x.ToolCalls=[]agentchat.ToolCallRecord{{CallID:"patch",Name:"taskdeck_submit_patch",Status:"error",
  Result:x.History[2].Content,FeedbackSent:true,FeedbackStatus:"unknown"}}
 if target:=agentReceiptResendCandidate(x);target==nil||target.Kind!="patch"{t.Fatal("verified failed PATCH result not available")}
 x.ToolCalls[0].FeedbackStatus="acknowledged"
 if agentReceiptResendCandidate(x)!=nil{t.Fatal("acknowledged feedback offered resend")}
 x.ToolCalls[0].FeedbackStatus="unknown"
 x.ToolCalls[0].Result="different"
 if agentReceiptResendCandidate(x)!=nil{t.Fatal("mismatched receipt offered resend")}
 x.ToolCalls[0].Result=x.History[2].Content
 x.ToolCalls[0].Status="running"
 if agentReceiptResendCandidate(x)!=nil{t.Fatal("live native mutation offered resend")}
}

func TestReceiptResendAPIUsesExactSavedCollectionWithoutNativeReplay(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 fixture:=receiptResendFixture()
 x.Status=fixture.Status;x.ProviderRun=fixture.ProviderRun;x.History=fixture.History
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 target:=agentReceiptResendCandidate(x);if target==nil{t.Fatal("missing target")}
 calls:=0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{t.Fatal("old COLLECT rerun");return ""}
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if !strings.Contains(string(payload),"SAVED NATIVE RECEIPT")||!strings.Contains(string(payload),"tiếng Việt"){t.Fatal("exact receipt not included in mandatory new user message")}
  return agentchat.ResponsesResult{ResponseID:"resend-completed",Text:"Task complete: all work done."},nil
 }
 path:="/api/agent/sessions/"+id+"/messages"
 request:=map[string]any{"message":"Resend saved native result and continue the original task.","confirm_external":true,
  "expected_revision":x.Revision,"request_id":"receipt-resend-test","resend_receipt":target}
 wrong:=*target;wrong.SHA256=strings.Repeat("b",64);request["resend_receipt"]=&wrong
 w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,request)
 if w.Code!=409||calls!=0{t.Fatalf("mismatched receipt accepted: %d calls=%d",w.Code,calls)}
 request["resend_receipt"]=target
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,request)
 if w.Code!=200||calls!=1{t.Fatalf("resend failed: %d calls=%d %s",w.Code,calls,w.Body)}
 done,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if done.Status!="completed"||done.ProviderRun.ID==target.RunID||agentReceiptResendCandidate(done)!=nil{t.Fatal("resend not completed as a new turn")}
 count:=0
 for _,it:=range done.History{if it.Type=="function_call_output"&&it.CallID=="read"{count++}}
 if count!=1{t.Fatal("duplicate canonical tool result")}
 w=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,path,request)
 if w.Code!=200||calls!=1{t.Fatal("same explicit request replayed provider")}
}
