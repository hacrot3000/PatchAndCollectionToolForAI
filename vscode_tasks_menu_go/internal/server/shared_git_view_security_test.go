package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// A git.status grant is intentionally metadata-only: neither blame/content
// nor revision history endpoints may pass through shared authorization with
// that grant alone, regardless of the requested (untrusted) view parameter.
func TestSharedGitHistoricalViewsRequireLogOrDiffPermission(t *testing.T) {
 server:=&Server{}
 for _,tc:=range []struct{
  view,required string
 }{
  {"file-history",identity.PermissionGitLog},
  {"stashes",identity.PermissionGitLog},
  {"blame",identity.PermissionGitDiff},
  {"compare-tree",identity.PermissionGitDiff},
  {"compare-tree-diff",identity.PermissionGitDiff},
 }{
  t.Run(tc.view,func(t *testing.T){
   for _,grants:=range []struct{
    name string
    permissions map[string]bool
    expected int
   }{
    {"status-only",map[string]bool{identity.PermissionGitStatus:true},http.StatusForbidden},
    {"permitted",map[string]bool{tc.required:true},http.StatusNoContent},
   }{
    t.Run(grants.name,func(t *testing.T){
     called:=false
     handler:=server.sharedAuthorize(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
      called=true
      w.WriteHeader(http.StatusNoContent)
     }))
     req:=httptest.NewRequest(http.MethodGet,"/api/git/status?view="+tc.view,nil)
     principal:=identity.Principal{UserID:"viewer",ProjectID:"project-a",Permissions:grants.permissions}
     req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},principal))
     rec:=httptest.NewRecorder()
     handler.ServeHTTP(rec,req)
     if rec.Code!=grants.expected||called!=(grants.expected==http.StatusNoContent){
      t.Fatalf("view=%s grants=%s result=%d called=%t",tc.view,grants.name,rec.Code,called)
     }
    })
   }
  })
 }
}
