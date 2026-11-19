package server

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func agentProviderFixture()agentchat.ProviderConfig{
 return agentchat.ProviderConfig{
  ID:"custom-r2d",Name:"Custom Endpoint",APIType:"responses",BaseURL:"https://api.r2d.dev/v1",
  Models:[]agentchat.ModelConfig{{ID:"composer-2.5",Name:"composer-2.5",ToolCalling:true,MaxInputTokens:1280000,MaxOutputTokens:160000,ReasoningEfforts:[]string{"low","medium","high"},DefaultReasoningEffort:"medium"}},
 }
}
func agentProviderRequest(t *testing.T,handler http.HandlerFunc,method,path string,v any)*httptest.ResponseRecorder{
 t.Helper()
 body,_:=json.Marshal(v)
 r:=httptest.NewRequest(method,path,bytes.NewReader(body))
 r.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 handler(w,r)
 return w
}
func TestAgentChatProviderSettingsEncryptSecretsAndSuppressKey(t *testing.T){
 s:=newAIServerForTest(t)
 secret:="private-key-do-not-echo"
 p:=agentProviderFixture()
 w:=agentProviderRequest(t,s.agentChatProvidersAPI,http.MethodPut,"/api/agent/providers",map[string]any{"provider":p,"api_key":secret})
 if w.Code!=200{t.Fatalf("put %d: %s",w.Code,w.Body)}
 if strings.Contains(w.Body.String(),secret){t.Fatal("key leaked in response")}
 w=agentProviderRequest(t,s.agentChatProvidersAPI,http.MethodGet,"/api/agent/providers",nil)
 if w.Code!=200||!strings.Contains(w.Body.String(),"composer-2.5")||strings.Contains(w.Body.String(),secret)||!strings.Contains(w.Body.String(),"\"has_api_key\":true"){
  t.Fatalf("unsafe list: %d %s",w.Code,w.Body)
 }
 key,e:=s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace,"local",p.ID))
 if e!=nil||string(key)!=secret{t.Fatal("encrypted store not used for custom agent key")}
 store,e:=s.agentChatProviders();if e!=nil{t.Fatal(e)}
 ps,e:=store.List("another-owner");if e!=nil||len(ps)!=0{t.Fatal("provider leaked to other owner")}
}
func TestAgentChatProviderSettingsRejectBadConfig(t *testing.T){
 s:=newAIServerForTest(t)
 p:=agentProviderFixture();p.BaseURL="http://localhost:1234/"
 w:=agentProviderRequest(t,s.agentChatProvidersAPI,http.MethodPut,"/api/agent/providers",map[string]any{"provider":p})
 if w.Code!=400{t.Fatalf("unsafe endpoint allowed: %d %s",w.Code,w.Body)}
 p=agentProviderFixture()
 w=agentProviderRequest(t,s.agentChatProvidersAPI,http.MethodPut,"/api/agent/providers",map[string]any{"provider":p,"unexpected":"field"})
 if w.Code!=400{t.Fatalf("extra fields allowed: %d",w.Code)}
}
func TestAgentChatProviderConnectionProbeRequiresExplicitConsent(t *testing.T){
 s:=newAIServerForTest(t)
 w:=agentProviderRequest(t,s.agentChatProviderTestAPI,http.MethodPost,"/api/agent/providers/test",map[string]any{"provider_id":"test","model_id":"model"})
 if w.Code!=403{t.Fatalf("network attempt without consent allowed: %d",w.Code)}
 w=agentProviderRequest(t,s.agentChatProviderTestAPI,http.MethodPost,"/api/agent/providers/test",map[string]any{"confirm_external":true,"provider_id":"missing","model_id":"model"})
 if w.Code!=404{t.Fatalf("missing provider accepted: %d",w.Code)}
}
func TestAgentChatProviderSharedServerPermissions(t *testing.T){
 cases:=[]struct{method,path,permission string}{
  {"GET","/api/agent/providers",identity.PermissionSettingsRead},
  {"PUT","/api/agent/providers",identity.PermissionSettingsWrite},
  {"POST","/api/agent/providers/test",identity.PermissionSettingsWrite},
 }
 for _,tc:=range cases{
  req:=httptest.NewRequest(tc.method,tc.path,nil)
  perms:=sharedRoutePermissions(req)
  if len(perms)!=1||perms[0]!=tc.permission{t.Fatalf("%s %s incorrect: %+v",tc.method,tc.path,perms)}
 }
}
