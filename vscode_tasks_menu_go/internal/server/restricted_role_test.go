package server

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func restrictedRoleRequest(method,path,body string,principal identity.Principal)*http.Request{
 r:=httptest.NewRequest(method,path,strings.NewReader(body))
 return r.WithContext(context.WithValue(r.Context(),sharedPrincipalContextKey{},principal))
}
func restrictedRolePrincipal(id identity.ID)identity.Principal{
 allowed:=map[string]bool{}
 for _,permission:=range identity.RestrictedDeveloperPermissions(){allowed[permission]=true}
 return identity.Principal{ProjectID:"test-project",UserID:id,Username:string(id),
  RoleID:identity.RestrictedDeveloperRoleID,Restricted:true,Permissions:allowed}
}

func TestRestrictedRoleGatesLegacyPathsOnNormalSharedServer(t *testing.T){
 s:=protectedFixture(t)
 s.Config.Restricted=nil
 s.Config.SharedServerEnabled=true
 calls:=0
 handler:=s.restrictedGate(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++;w.WriteHeader(http.StatusNoContent)
 }))
 restricted:=restrictedRolePrincipal("member")
 for _,path:=range []string{
  "/api/project/file","/api/project/download","/api/git/status",
  "/api/tasks","/api/sessions","/api/ssh/profiles",
  "/api/admin/users","/admin/access","/featuremods/editor.js",
 }{
  rr:=httptest.NewRecorder()
  handler.ServeHTTP(rr,restrictedRoleRequest(http.MethodGet,path,"",restricted))
  if rr.Code!=http.StatusForbidden||calls!=0{
   t.Fatalf("restricted legacy route %s escaped: %d calls=%d",path,rr.Code,calls)
  }
 }
 rr:=httptest.NewRecorder()
 handler.ServeHTTP(rr,restrictedRoleRequest(http.MethodGet,"/","",restricted))
 if rr.Code!=http.StatusOK||!strings.Contains(rr.Body.String(),"Protected Code View"){
  t.Fatalf("restricted member did not receive isolated page: %d",rr.Code)
 }
 status:=httptest.NewRecorder()
 handler.ServeHTTP(status,restrictedRoleRequest(http.MethodGet,"/api/restricted/status","",restricted))
 if status.Code!=http.StatusOK||!strings.Contains(status.Body.String(),`"max_visible_lines":80`){
  t.Fatalf("role policy not applied: %d %s",status.Code,status.Body)
 }
 // Normal members retain historical shared-server routing.
 normal:=identity.Principal{ProjectID:"test-project",UserID:"admin",RoleID:"system:admin"}
 r:=httptest.NewRecorder()
 handler.ServeHTTP(r,restrictedRoleRequest(http.MethodGet,"/api/project/file","",normal))
 if r.Code!=http.StatusNoContent||calls!=1{t.Fatalf("admin incorrectly restricted: %d calls=%d",r.Code,calls)}
}

func TestRestrictedRoleServerQuotaPersistsByAccountWithoutCLIFlag(t *testing.T){
 s:=protectedFixture(t)
 s.Config.Restricted=nil
 s.Config.SharedServerEnabled=true
 gate:=s.restrictedGate(http.HandlerFunc(s.restrictedFileWindowAPI))
 user:=restrictedRolePrincipal("alice")
 run:=func(p identity.Principal,start,count int)*httptest.ResponseRecorder{
  payload:=`{"path":"main.go","start":`+fmt.Sprint(start)+`,"count":`+fmt.Sprint(count)+"}"
  recorder:=httptest.NewRecorder()
  gate.ServeHTTP(recorder,restrictedRoleRequest(http.MethodPost,"/api/restricted/file-window",payload,p))
  return recorder
 }
 first:=run(user,1,8)
 if first.Code!=http.StatusOK{t.Fatalf("expected first 8 of 10 visible lines: %d %s",first.Code,first.Body)}
 nine:=run(user,9,1)
 if nine.Code!=http.StatusTooManyRequests{t.Fatalf("quota not enforced for role account: %d %s",nine.Code,nine.Body)}
 again:=run(user,1,1)
 if again.Code!=http.StatusOK{t.Fatalf("previously viewed line no longer accessible: %d",again.Code)}
 another:=run(restrictedRolePrincipal("bob"),9,1)
 if another.Code!=http.StatusOK{t.Fatalf("distinct member should have independent quota: %d %s",another.Code,another.Body)}
}
