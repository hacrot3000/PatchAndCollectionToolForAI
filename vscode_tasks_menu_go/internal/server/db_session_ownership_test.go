package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/dbsession"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedDatabaseSessionsVisibleOnlyToOwnerOrProjectAdmin(t *testing.T) {
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 s:=&Server{Config:cfg}
 owned:=dbsession.Metadata{ID:"db-1",OwnerProjectID:"project-a",OwnerUserID:"alice"}
 unowned:=dbsession.Metadata{ID:"db-unowned"}

 request:=func(principal *identity.Principal)*http.Request{
  req:=httptest.NewRequest(http.MethodGet,"/api/db/sessions",nil)
  if principal!=nil {
   req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},*principal))
  }
  return req
 }
 for _,tc:=range []struct{
  name string
  principal *identity.Principal
  wantOwned,wantUnowned bool
 }{
  {"anonymous",nil,false,false},
  {"owner",&identity.Principal{UserID:"alice",ProjectID:"project-a"},true,false},
  {"another user",&identity.Principal{UserID:"bob",ProjectID:"project-a"},false,false},
  {"another project",&identity.Principal{UserID:"alice",ProjectID:"project-b"},false,false},
  {"project admin",&identity.Principal{UserID:"admin",ProjectID:"project-a",
   Permissions:map[string]bool{identity.PermissionProjectAdmin:true}},true,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   req:=request(tc.principal)
   if got:=s.dbSessionVisibleToRequest(req,owned);got!=tc.wantOwned{
    t.Fatalf("owned session visible=%v want=%v",got,tc.wantOwned)
   }
   if got:=s.dbSessionVisibleToRequest(req,unowned);got!=tc.wantUnowned{
    t.Fatalf("unowned session visible=%v want=%v",got,tc.wantUnowned)
   }
  })
 }
 cfg.SharedServerEnabled=false
 s.Config=cfg
 if !s.dbSessionVisibleToRequest(request(nil),owned)||
  !s.dbSessionVisibleToRequest(request(nil),unowned){
  t.Fatal("local single-user mode should preserve its existing sessions")
 }
}
