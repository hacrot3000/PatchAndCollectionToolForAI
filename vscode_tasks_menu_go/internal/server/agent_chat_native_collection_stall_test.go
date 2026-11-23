package server

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Regression: a provider returns the exact large-source FBB Collection request
// as an assistant TEXT message instead of calling its advertised function.
// This must trigger Native COLLECT (the adapter here only replaces the native
// process for a protocol test), NOT mark the unexecuted request "completed".
func TestAgentNativeCollectionTextFBBRequestDoesNotSilentlyComplete(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil{t.Fatal(err)}
 request:=fmt.Sprintf(`{"id":"trace-fbb-mongodb-storage","project":{"key":%q,"id":%q},"limits":{"max_search_file_bytes":200000000,"max_decompile_file_bytes":200000000},"actions":[{"type":"search","paths":["docs/GM_52_76.c"],"query":"UserFollowBloodBound::HandleFBBMove","backend":"auto","filesystem":true,"fallback_search":true,"report_coverage":true,"max_matches":30},{"type":"search","paths":["docs/GM_52_76.c"],"query":"SqlFollowBloodBound","max_matches":100},{"type":"search","paths":["docs/GM_52_76.h"],"query":"UserFollowBloodBound","max_matches":60},{"type":"decompile","source":"docs/GM_52_76.c","name":"UserFollowBloodBound::HandleFBBMove","match":"exact","include_references":true,"max_reference_hits":15},{"type":"decompile","source":"docs/GM_52_76.c","name":"UserFollowBloodBound::HandleFBBSaleOut","match":"exact","include_references":true,"max_reference_hits":15},{"type":"decompile","source":"docs/GM_52_76.c","name":"UserFollowBloodBound::HandleFBBLoadDB","match":"exact","include_references":true,"max_reference_hits":15},{"type":"decompile","source":"docs/GM_52_76.c","name":"UserFollowBloodBound","match":"contains","max_matches":100},{"type":"decompile","source":"docs/GM_52_76.c","name":"SqlFollowBloodBound","match":"contains","max_matches":100}]}`,identity.Key,identity.ID)
 nativeCalls,providerCalls:=0,0
 s.AgentCollectionCall=func(root string,p patchtool.ProjectIdentity,raw []byte)string{
  nativeCalls++
  if root!=s.Workspace||p!=identity{t.Fatalf("native Collection wrong identity/root")}
  var envelope struct{
   Actions []struct{Type string `json:"type"`;Name string `json:"name"`} `json:"actions"`
   Limits map[string]uint64 `json:"limits"`
   Project patchtool.ProjectIdentity `json:"project"`
  }
  if err:=json.Unmarshal(raw,&envelope);err!=nil{t.Fatal(err)}
  if envelope.Project!=identity||len(envelope.Actions)!=8||
   envelope.Actions[3].Type!="decompile"||envelope.Actions[3].Name!="UserFollowBloodBound::HandleFBBMove"||
   envelope.Limits["max_decompile_file_bytes"]!=200000000 {
   t.Fatalf("native large-source Collection request changed: %+v",envelope)
  }
  return `{"source":"native_patch_tool_collect","status":"INCOMPLETE","partial_evidence_available":true,"text_excerpt":"FollowBloodBoundMgr::OnFBBMove observed","result_zip":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fbb.zip","result_text":"artifacts/patch_tool_code_collections/CODE_COLLECTION_RESULT_fbb.txt"}`
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,body []byte)(agentchat.ResponsesResult,error){
  providerCalls++
  if providerCalls==1{return agentchat.ResponsesResult{ResponseID:"fbb-proposal",Text:request},nil}
  var req struct{Input []map[string]any `json:"input"`}
  if err:=json.Unmarshal(body,&req);err!=nil{t.Fatal(err)}
  found:=false
  for _,item:=range req.Input{
   content,_:=item["content"].(string)
   if strings.Contains(content,"TaskDeck native Patch Tool COLLECT returned this observation")&&
    strings.Contains(content,"FollowBloodBoundMgr::OnFBBMove observed")&&strings.Contains(content,"INCOMPLETE"){
    found=true
   }
  }
  if !found{t.Fatal("model's text Collection was silently completed without native result and continuation")}
  return agentchat.ResponsesResult{ResponseID:"fbb-continued",Text:"FBB native partial evidence received; continue with targeted decompile."},nil
 }
 response:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"continue FBB source investigation","confirm_external":true,"expected_revision":1})
 if response.Code!=200||nativeCalls!=1||providerCalls!=2{
  t.Fatalf("Agent stalled after unexecuted FBB JSON: HTTP %d native=%d provider=%d body=%s",
   response.Code,nativeCalls,providerCalls,response.Body)
 }
}

func TestAgentMalformedTextCollectionIsReportedAndRepairable(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 bad:=fmt.Sprintf(`{"project":{"key":%q,"id":%q},"actions":{}}`,identity.Key,identity.ID)
 calls:=0
 s.AgentCollectionCall=func(_ string,_ patchtool.ProjectIdentity,_ []byte)string{
  t.Fatal("malformed text Collection was executed");return ""
 }
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,body []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls==1{return agentchat.ResponsesResult{ResponseID:"bad",Text:bad},nil}
  if !strings.Contains(string(body),"INVALID_COLLECTION_REQUEST"){
   t.Fatal("invalid text Collection was silently treated as completed")
  }
  return agentchat.ResponsesResult{ResponseID:"repaired",Text:"Native Collection request schema needs correction."},nil
 }
 res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"inspect the code","confirm_external":true,"expected_revision":1})
 if res.Code!=200||calls!=2{t.Fatalf("malformed request not sent back to AI for correction: %d %s",res.Code,res.Body)}
 store,_:=s.agentChatStore();session,_:=store.Get(id,"local")
 found:=false
 for _,e:=range session.Events{if e.Kind=="collect_request_rejected"{found=true}}
 if !found{t.Fatal("Collection request rejection was invisible to user")}
}

func TestAgentMismatchedTextCollectionIdentityFailsVisibly(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"stateless")
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
 foreign:=fmt.Sprintf(`{"project":{"key":"foreign","id":%q},"actions":[{"type":"search","query":"FBB"}]}`,identity.ID)
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
  calls++;return agentchat.ResponsesResult{ResponseID:"wrong-project",Text:foreign},nil
 }
 res:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",
  map[string]any{"message":"find FBB source","confirm_external":true,"expected_revision":1})
 if res.Code!=http.StatusConflict||calls!=1||
  !strings.Contains(res.Body.String(),"COLLECTION_IDENTITY_REJECTED"){
  t.Fatalf("foreign request must fail visibly, never silently complete: %d %s",res.Code,res.Body)
 }
 store,_:=s.agentChatStore();session,_:=store.Get(id,"local")
 if session.Status!="failed"{t.Fatalf("mismatched identity was not a failed turn: %q",session.Status)}
}
