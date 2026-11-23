package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentSessionGetShowsOwnerScopedProviderEgressReviewWithoutReplay(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 store,err:=s.agentChatStore()
 if err!=nil{t.Fatal(err)}
 url:="/api/agent/sessions/"+id
 for _,tc:=range []struct{
  phase,status string
  expected bool
 }{
  {"sending","requesting_ai",true},
  {"unknown","failed",true},
  {"prepared","requesting_ai",false},
  {"acknowledged","completed",false},
  {"completed","completed",false},
  {"blocked","failed",false},
 }{
  x,err:=store.Get(id,"local")
  if err!=nil{t.Fatal(err)}
  now:=time.Now().UTC()
  x.ProviderRun=&agentchat.ProviderRun{
   ID:"r2-e7",Kind:"message",Phase:tc.phase,
   PayloadSHA256:strings.Repeat("a",64),
   ProviderFingerprint:"owner-config-fingerprint",
   CreatedAt:now,UpdatedAt:now,
  }
  x.Status=tc.status
  x,err=store.Save(x,x.Revision)
  if err!=nil{t.Fatalf("save phase %s: %v",tc.phase,err)}
  before:=x.Revision
  got:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,url,nil)
  if got.Code!=http.StatusOK{
   t.Fatalf("session GET %s returned %d %s",tc.phase,got.Code,got.Body.String())
  }
  var response struct{
   Session agentchat.Session `json:"session"`
   ProviderReview *agentchat.ProviderEgressReview `json:"provider_review"`
  }
  if err=json.Unmarshal(got.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
  if got.Header().Get("Cache-Control")!="no-store"{
   t.Fatal("private reconciliation result lacks no-store")
  }
  if response.Session.ID!=id||response.Session.Revision!=before{
   t.Fatal("read-only review changed the session or identity")
  }
  if tc.expected{
   if response.ProviderReview==nil||
    !response.ProviderReview.RequiresReconciliation||
    response.ProviderReview.AutomaticReplayAllowed||
    response.ProviderReview.RunID!="r2-e7"||
    response.ProviderReview.Phase!=tc.phase||
    !strings.Contains(response.ProviderReview.Guidance,"Never automatically replay"){
    t.Fatalf("phase %s did not expose a safe review: %+v",tc.phase,response.ProviderReview)
   }
  }else if response.ProviderReview!=nil{
   t.Fatalf("phase %s has spurious ambiguous review: %+v",tc.phase,response.ProviderReview)
  }
  persisted,err:=store.Get(id,"local")
  if err!=nil{t.Fatal(err)}
  if persisted.Revision!=before||persisted.ProviderRun.Phase!=tc.phase{
   t.Fatal("read-only GET mutated egress state or CAS revision")
  }
 }
}
