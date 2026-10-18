package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/dbadapter"
 "bletonfc/vscode_tasks_menu/internal/dbprofile"
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


func TestSharedDatabaseSessionHTTPRejectsOtherUserAndLeaksNoSessionIDs(t *testing.T) {
 t.Setenv("TASKDECK_SERVER_DB_HELPER","1")
 registry:=dbadapter.NewRegistry()
 manifest:=serverDBTestManifest()
 if err:=registry.Register(manifest);err!=nil{t.Fatal(err)}
 manager,err:=dbsession.NewManager(registry,4)
 if err!=nil{t.Fatal(err)}
 defer manager.Close()
 profiles,err:=dbprofile.NewStore(filepath.Join(t.TempDir(),"db_profiles.json"))
 if err!=nil{t.Fatal(err)}
 if _,err=profiles.Create(dbprofile.Profile{
  ID:"session-owner-profile",Name:"Protected DB",AdapterID:manifest.ID,
  Transport:dbprofile.TransportDirect,Host:"db.example.com",Port:3306,
 });err!=nil{t.Fatal(err)}
 cfg:=config.Default()
 cfg.SharedServerEnabled=true
 server:=&Server{
  Config:cfg,Workspace:t.TempDir(),DBProfiles:profiles,
  DBAdapters:registry,DBSessions:manager,
 }
 makeReq:=func(method,path,body,user,project string,admin bool)*http.Request{
  t.Helper()
  req:=httptest.NewRequest(method,path,strings.NewReader(body))
  req.Header.Set("Content-Type","application/json")
  perms:=map[string]bool{identity.PermissionDBRead:true}
  if admin{perms[identity.PermissionProjectAdmin]=true}
  return req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},
   identity.Principal{UserID:identity.ID(user),ProjectID:identity.ID(project),Permissions:perms}))
 }
 get:=func(method,path,body,user,project string,admin bool)*httptest.ResponseRecorder{
  req:=makeReq(method,path,body,user,project,admin)
  rec:=httptest.NewRecorder()
  if path=="/api/db/sessions"{server.dbSessions(rec,req)}else{server.dbSessionItem(rec,req)}
  return rec
 }
 create:=get(http.MethodPost,"/api/db/sessions",`{"profile_id":"session-owner-profile"}`,"alice","project-1",false)
 if create.Code!=http.StatusCreated{t.Fatalf("owner open: %d %s",create.Code,create.Body)}
 var meta dbsession.Metadata
 if err:=json.Unmarshal(create.Body.Bytes(),&meta);err!=nil{t.Fatal(err)}
 if meta.ID==""{t.Fatal("missing created session ID")}
 path:="/api/db/sessions/"+meta.ID
 listed:=get(http.MethodGet,"/api/db/sessions","","alice","project-1",false)
 if listed.Code!=http.StatusOK||!strings.Contains(listed.Body.String(),meta.ID){
  t.Fatalf("owner could not see own DB connection: %d %s",listed.Code,listed.Body)
 }
 for _,actor:=range []struct{user,project string}{
  {"bob","project-1"},{"alice","project-2"},
 }{
  listing:=get(http.MethodGet,"/api/db/sessions","",actor.user,actor.project,false)
  if listing.Code!=http.StatusOK||strings.Contains(listing.Body.String(),meta.ID){
   t.Fatalf("cross-user DB list disclosed session: %d %s",listing.Code,listing.Body)
  }
  for _,tc:=range []struct{method,path,body string}{
   {http.MethodGet,path,""},
   {http.MethodDelete,path,""},
   {http.MethodPost,path+"/request",`{"operation":"ping"}`},
  }{
   response:=get(tc.method,tc.path,tc.body,actor.user,actor.project,false)
   if response.Code!=http.StatusNotFound{
    t.Fatalf("cross-user session operation %s %s returned %d: %s",tc.method,tc.path,response.Code,response.Body)
   }
  }
 }
 adminView:=get(http.MethodGet,path,"","admin","project-1",true)
 if adminView.Code!=http.StatusOK{t.Fatalf("admin session oversight: %d %s",adminView.Code,adminView.Body)}
 ownerPing:=get(http.MethodPost,path+"/request",`{"operation":"ping"}`,"alice","project-1",false)
 if ownerPing.Code!=http.StatusOK{t.Fatalf("owner ping: %d %s",ownerPing.Code,ownerPing.Body)}
 ownerClose:=get(http.MethodDelete,path,"","alice","project-1",false)
 if ownerClose.Code!=http.StatusNoContent{t.Fatalf("owner close: %d %s",ownerClose.Code,ownerClose.Body)}
 _=os.Unsetenv("TASKDECK_SERVER_DB_HELPER")
}
