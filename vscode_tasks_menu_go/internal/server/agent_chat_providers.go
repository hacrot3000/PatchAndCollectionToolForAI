package server

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/secretstore"
)

const agentProviderConfigLimit = 128 << 10

func (s *Server) agentChatProviders()(*agentchat.ProviderStore,error){
 privateFile,err:=aiPrivatePath(s.Workspace)
 if err!=nil{return nil,err}
 return agentchat.NewProviderStore(filepath.Join(filepath.Dir(privateFile),"agent-providers-"+aiWorkspaceHash(s.Workspace)))
}
func agentProviderSecretID(workspace,owner,providerID string)string{
 h:=sha256.Sum256([]byte(owner))
 return "ai/agent/"+aiWorkspaceHash(workspace)+"/"+hex.EncodeToString(h[:12])+"/"+providerID
}
type agentProviderView struct{
 agentchat.ProviderConfig
 HasAPIKey bool `json:"has_api_key"`
}
func (s *Server) agentProviderView(owner string,p agentchat.ProviderConfig)agentProviderView{
 view:=agentProviderView{ProviderConfig:p}
 if s.ConnectionSecrets!=nil {
  key,err:=s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace,owner,p.ID))
  view.HasAPIKey=err==nil&&len(key)>0
 }
 return view
}

func (s *Server) agentChatProvidersAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet&&r.Method!=http.MethodPut{
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 store,err:=s.agentChatProviders()
 if err!=nil{http.Error(w,"agent provider store unavailable",http.StatusInternalServerError);return}
 owner:=aiPlanOwner(r)
 w.Header().Set("Cache-Control","no-store")
 if r.Method==http.MethodGet {
  providers,err:=store.List(owner);if err!=nil{http.Error(w,"cannot load agent providers",500);return}
  views:=make([]agentProviderView,0,len(providers))
  for _,p:=range providers{views=append(views,s.agentProviderView(owner,p))}
  writeJSON(w,200,map[string]any{"providers":views});return
 }
 r.Body=http.MaxBytesReader(w,r.Body,agentProviderConfigLimit+4096)
 if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))),"application/json"){
  http.Error(w,"JSON Content-Type required",http.StatusUnsupportedMediaType);return
 }
 var req struct {
  Provider agentchat.ProviderConfig `json:"provider"`
  APIKey string `json:"api_key,omitempty"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err=dec.Decode(&req);err!=nil{http.Error(w,"invalid provider configuration",400);return}
 var trailing any
 if err=dec.Decode(&trailing);!errors.Is(err,io.EOF){http.Error(w,"exactly one JSON object required",400);return}
 if len(req.APIKey)>8192||strings.TrimSpace(req.APIKey)!=req.APIKey {
  http.Error(w,"invalid API key length/whitespace",400);return
 }
 if err=agentchat.ValidateProvider(req.Provider);err!=nil{http.Error(w,err.Error(),400);return}
 if req.APIKey!="" {
  if s.ConnectionSecrets==nil{http.Error(w,"encrypted secret store unavailable",503);return}
  if err=s.ConnectionSecrets.Put(agentProviderSecretID(s.Workspace,owner,req.Provider.ID),[]byte(req.APIKey));err!=nil{
   http.Error(w,"unable to store encrypted provider key",500);return
  }
 }
 if err=store.Put(owner,req.Provider);err!=nil{http.Error(w,"unable to persist provider configuration",500);return}
 writeJSON(w,200,map[string]any{"provider":s.agentProviderView(owner,req.Provider)})
}

// Test Connection is user-triggered only. This intentionally tests text
// Responses compatibility, not future tool calling/streaming/stateful support.
func (s *Server) agentChatProviderTestAPI(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",405);return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 var req struct{
  ProviderID string `json:"provider_id"`
  ModelID string `json:"model_id"`
  ConfirmExternal bool `json:"confirm_external"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if dec.Decode(&req)!=nil{http.Error(w,"invalid provider test request",400);return}
 var trailing any
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){http.Error(w,"exactly one JSON object required",400);return}
 if !req.ConfirmExternal{http.Error(w,"external provider consent required",403);return}
 store,err:=s.agentChatProviders();if err!=nil{http.Error(w,"provider store unavailable",500);return}
 owner:=aiPlanOwner(r)
 p,err:=store.Get(owner,req.ProviderID)
 if err!=nil{http.Error(w,"provider not configured",404);return}
 if _,ok:=p.FindModel(req.ModelID);!ok{http.Error(w,"model not configured",400);return}
 if s.ConnectionSecrets==nil{http.Error(w,"secret store unavailable",503);return}
 secret,err:=s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace,owner,p.ID))
 if err!=nil||len(secret)==0{http.Error(w,"provider API key not configured",503);return}
 release,allowed:=s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner,20)
 if !allowed{http.Error(w,"provider request quota exceeded",429);return}
 defer release()
 body,err:=agentchat.BuildResponsesRequest(agentchat.RequestConfig{
  Model:req.ModelID,Instructions:"You are an API compatibility probe. Reply briefly. Do not use tools.",
  Mode:agentchat.Stateless,Next:[]agentchat.HistoryItem{{Type:"user",Content:"Reply with the word OK."}},
  MaxOutputTokens:64,
 })
 if err!=nil{http.Error(w,"provider probe construction failed",500);return}
 res,err:=agentchat.DoResponses(r.Context(),p,string(secret),body)
 if err!=nil{http.Error(w,"provider connection or Responses API test failed: "+err.Error(),502);return}
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,200,map[string]any{
  "ok":true,"provider_id":p.ID,"model_id":req.ModelID,"response_id":res.ResponseID,
  "input_tokens":res.InputTokens,"output_tokens":res.OutputTokens,
  "text_supported":strings.TrimSpace(res.Text)!="",
  "tool_calling_tested":false,"streaming_tested":false,"stateful_tested":false,
 })
}

var _ = secretstore.ErrNotFound
