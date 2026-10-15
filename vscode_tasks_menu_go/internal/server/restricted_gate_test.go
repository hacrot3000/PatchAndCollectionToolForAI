package server

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

func TestRestrictedGateBlocksLegacyAPIsBothAuthModes(t *testing.T) {
 for _,shared:=range []bool{false,true}{
  p:=restrictedpolicy.Default()
  s:=&Server{Config:config.Config{Restricted:&p,SharedServerEnabled:shared}}
  invoked:=0
  next:=http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   invoked++;w.WriteHeader(http.StatusNoContent)
  })
  gate:=s.restrictedGate(next)
  for _,path:=range []string{
   "/api/project/file", "/api/project/bytes",
   "/api/project/file-history", "/api/project/content/search",
   "/api/project/preview","/api/project/download",
   "/api/files/download", "/api/patch/ai-pack",
   "/api/git/status", "/api/git/jobs",
   "/api/sessions","/api/sessions/demo/ws",
   "/api/ssh/profiles","/api/file-transfer/download",
   "/api/db/sessions","/api/tasks","/api/ai/assist",
   "/api/config/auth-mode","/api/config-backup",
   "/api/restricted/unknown",
  }{
   rr:=httptest.NewRecorder()
   gate.ServeHTTP(rr,httptest.NewRequest(http.MethodGet,path,nil))
   if rr.Code!=http.StatusForbidden||invoked!=0 {
    t.Fatalf("shared=%v GET %s status=%d reached=%d",shared,path,rr.Code,invoked)
   }
  }
  rr:=httptest.NewRecorder()
  gate.ServeHTTP(rr,httptest.NewRequest(http.MethodPost,"/api/project/file",strings.NewReader("{}")))
  if rr.Code!=http.StatusForbidden||invoked!=0 {t.Fatal("restricted POST reached legacy backend")}
  for _,path:=range []string{"/api/health","/","/featuremods/editor.js"}{
   rr:=httptest.NewRecorder()
   gate.ServeHTTP(rr,httptest.NewRequest(http.MethodGet,path,nil))
   if rr.Code!=http.StatusNoContent {t.Fatalf("safe path %s denied: %d",path,rr.Code)}
  }
 }
}
func TestRestrictedStatusIsReadOnlyAndNoSecrets(t *testing.T){
 p:=restrictedpolicy.Default()
 p.MaxVisibleLines=60
 s:=&Server{Config:config.Config{Restricted:&p}}
 gate:=s.restrictedGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){t.Fatal("status must bypass existing routes")}))
 rr:=httptest.NewRecorder()
 gate.ServeHTTP(rr,httptest.NewRequest(http.MethodGet,"/api/restricted/status",nil))
 if rr.Code!=http.StatusOK||!strings.Contains(rr.Body.String(),"\"ready\":false")||
  !strings.Contains(rr.Body.String(),"\"max_visible_lines\":60"){
  t.Fatalf("status=%d body=%s",rr.Code,rr.Body.String())
 }
 if strings.Contains(rr.Body.String(),"Workspace")||strings.Contains(rr.Body.String(),"password"){t.Fatal("status exposed internals")}
 rr=httptest.NewRecorder()
 gate.ServeHTTP(rr,httptest.NewRequest(http.MethodPost,"/api/restricted/status",nil))
 if rr.Code!=http.StatusForbidden {t.Fatal("status mutated through POST")}
}
func TestUnrestrictedGatePreservesLegacyRouting(t *testing.T){
 s:=&Server{Config:config.Config{}}
 reached:=false
 gate:=s.restrictedGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){reached=true;w.WriteHeader(http.StatusAccepted)}))
 rr:=httptest.NewRecorder()
 gate.ServeHTTP(rr,httptest.NewRequest(http.MethodPost,"/api/sessions",nil))
 if !reached||rr.Code!=http.StatusAccepted {t.Fatal("unrestricted routing changed")}
}
