package server

import (
 "bytes"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "sync"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/secretstore"
)

type mockAISecretStore struct{mu sync.Mutex; data map[string][]byte}
func (s *mockAISecretStore) Put(id string,value []byte)error{s.mu.Lock();defer s.mu.Unlock();if s.data==nil{s.data=map[string][]byte{}};s.data[id]=append([]byte(nil),value...);return nil}
func (s *mockAISecretStore) Get(id string)([]byte,error){s.mu.Lock();defer s.mu.Unlock();if v,ok:=s.data[id];ok{return append([]byte(nil),v...),nil};return nil,secretstore.ErrNotFound}
func (s *mockAISecretStore) Delete(id string)error{s.mu.Lock();defer s.mu.Unlock();if _,ok:=s.data[id];!ok{return secretstore.ErrNotFound};delete(s.data,id);return nil}

func newAIServerForTest(t *testing.T)*Server{
 t.Helper();t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 return &Server{Workspace:filepath.Join(t.TempDir(),"project"),ConnectionSecrets:&mockAISecretStore{}}
}
func invokeAI(t *testing.T,h http.HandlerFunc,method,url string,payload any)*httptest.ResponseRecorder{
 t.Helper()
 var body []byte
 if payload!=nil{body,_=json.Marshal(payload)}
 r:=httptest.NewRequest(method,url,bytes.NewReader(body))
 w:=httptest.NewRecorder();h(w,r);return w
}
func TestAISettingsOptInAndCredentialWriteOnly(t *testing.T){
 s:=newAIServerForTest(t)
 initial:=invokeAI(t,s.aiSettingsAPI,"GET","/api/ai/settings",nil)
 if initial.Code!=200 {t.Fatalf("GET: %d %s",initial.Code,initial.Body)}
 if strings.Contains(initial.Body.String(),"secret") {t.Fatal("default config must not expose credentials")}
 var view aiSettings
 if e:=json.Unmarshal(initial.Body.Bytes(),&view);e!=nil{t.Fatal(e)}
 if view.Enabled||view.Consent||view.HasAPIKey{t.Fatalf("must be disabled by default: %+v",view)}
 c:=aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini",TimeoutSeconds:25,MaxOutputBytes:1200,IncludeCWD:true,APIKey:"TEST_PRIVATE_KEY_SHOULD_NOT_ECHO"}
 put:=invokeAI(t,s.aiSettingsAPI,"PUT","/api/ai/settings",c)
 if put.Code!=200{t.Fatalf("PUT: %d %s",put.Code,put.Body.String())}
 if strings.Contains(put.Body.String(),c.APIKey){t.Fatal("API credential returned to browser")}
 var after aiSettings;json.Unmarshal(put.Body.Bytes(),&after)
 if !after.HasAPIKey||!after.Enabled||!after.IncludeCWD{t.Fatalf("incorrect persisted settings: %+v",after)}
 key,e:=s.ConnectionSecrets.Get(aiSecretID(s.Workspace));if e!=nil||string(key)!=c.APIKey{t.Fatal("encrypted credential store was not used")}
 again:=invokeAI(t,s.aiSettingsAPI,"GET","/api/ai/settings",nil)
 if again.Code!=200||strings.Contains(again.Body.String(),c.APIKey){t.Fatal("credential leaked on subsequent GET")}
}
func TestAISettingsConsentRequired(t *testing.T){
 s:=newAIServerForTest(t)
 w:=invokeAI(t,s.aiSettingsAPI,"PUT","/api/ai/settings",aiSettings{Enabled:true,Model:"gpt-4.1-mini"})
 if w.Code!=400{t.Fatalf("want rejection without consent: %d",w.Code)}
}
func TestAIAssistDisabledAndRequiresConsentWithoutCallingProvider(t *testing.T){
 s:=newAIServerForTest(t)
 req:=aiAssistRequest{Mode:"explain",Question:"why did it fail?",ConfirmExternal:true}
 w:=invokeAI(t,s.aiAssistAPI,"POST","/api/ai/assist",req)
 if w.Code!=403{t.Fatalf("disabled AI should refuse: %d",w.Code)}
 c:=aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini"}
 if e:=s.saveAISettings(c);e!=nil{t.Fatal(e)}
 req.ConfirmExternal=false
 w=invokeAI(t,s.aiAssistAPI,"POST","/api/ai/assist",req)
 if w.Code!=403{t.Fatalf("no per-request consent should refuse: %d",w.Code)}
}
func TestAIContextAlwaysRedactsSensitiveLines(t *testing.T){
 c,_:=normalizeAISettings(aiSettings{IncludeCWD:true,IncludeHistory:true,IncludeGit:true,MaxOutputBytes:3000})
 context:=buildAIContext(aiAssistRequest{
  Mode:"explain",Question:"debug",Command:"curl -H 'Authorization: Bearer abc123' http://host",
  Output:"password=oh-no token:abc123\n-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
  CWD:"/safe",Git:"feature/ai",History:"export API_KEY=verybad",
 },c)
 for _,private:=range []string{"abc123","oh-no","verybad","-----BEGIN PRIVATE KEY-----"} {
  if strings.Contains(context,private){t.Fatalf("context leaked %q",private)}
 }
 if !strings.Contains(context,"[REDACTED]"){t.Fatal("expected redaction marker")}
}
func TestAISettingsLimits(t *testing.T){
 for _,cfg:=range []aiSettings{
  {Model:"invalid model with spaces"},
  {Model:"ok",TimeoutSeconds:181},
  {Model:"ok",MaxOutputBytes:12001},
 }{if _,e:=normalizeAISettings(cfg);e==nil{t.Fatalf("unexpected validation success %+v",cfg)}}
}
func TestAISecretDeletionAndNoEcho(t *testing.T){
 s:=newAIServerForTest(t);s.ConnectionSecrets.Put(aiSecretID(s.Workspace),[]byte("remove-me"))
 w:=invokeAI(t,s.aiSettingsAPI,"PUT","/api/ai/settings",aiSettings{ClearAPIKey:true})
 if w.Code!=200{t.Fatalf("remove: %d %s",w.Code,w.Body)}
 _,e:=s.ConnectionSecrets.Get(aiSecretID(s.Workspace))
 if !errors.Is(e,secretstore.ErrNotFound){t.Fatal("secret was not deleted")}
}

func TestAIRequestBudgetLimitsConcurrencyAndHourlyCalls(t *testing.T){
 s:=newAIServerForTest(t)
 release,ok:=s.reserveAIRequest("alice",2)
 if !ok {t.Fatal("first request should be accepted")}
 if _,ok=s.reserveAIRequest("alice",2);ok{t.Fatal("concurrent request must be denied")}
 otherRelease,ok:=s.reserveAIRequest("bob",2)
 if !ok{t.Fatal("another user should have independent budget")}
 otherRelease()
 release()
 second,ok:=s.reserveAIRequest("alice",2)
 if !ok{t.Fatal("second request should fit quota")}
 second()
 if _,ok=s.reserveAIRequest("alice",2);ok{t.Fatal("third request should exceed quota")}
}
