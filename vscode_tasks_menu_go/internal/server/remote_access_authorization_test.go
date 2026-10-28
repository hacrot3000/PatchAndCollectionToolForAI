package server

import (
 "context"
 "crypto/tls"
 "net/http"
 "net/http/httptest"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestFutureRemoteTransportCannotBypassTaskDeckAuthentication(t *testing.T) {
 s:=&Server{Config:config.Config{
  RemoteAccessEnabled:true,SharedServerEnabled:true,
  SharedProjectID:"workspace-A",
 }}
 r:=httptest.NewRequest(http.MethodGet,"https://taskdeck.local/api/tasks",nil)
 r.TLS=&tls.ConnectionState{}
 if err:=s.authorizeRemoteTransportAction(r,remotetransport.RoleAdmin,identity.PermissionTasksView);err==nil{
  t.Fatal("Portal administrator bypassed absent TaskDeck login")
 }
 principal:=identity.Principal{
  UserID:"local-user",ProjectKey:"workspace-A",
  Permissions:map[string]bool{
   identity.PermissionTasksView:true,
   identity.PermissionTasksRun:true,
   identity.PermissionTerminalCreate:true,
  },
 }
 authenticated:=r.WithContext(context.WithValue(r.Context(),sharedPrincipalContextKey{},principal))
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleViewer,identity.PermissionTasksView);err!=nil{
  t.Fatalf("authenticated viewer was denied permitted task listing: %v",err)
 }
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleViewer,identity.PermissionTasksRun);err==nil{
  t.Fatal("Portal viewer escalated to task execution")
 }
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleOperator,identity.PermissionTerminalCreate);err==nil{
  t.Fatal("Portal operator escalated to terminal")
 }
 s.Config.SharedProjectID="workspace-B"
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleAdmin,identity.PermissionTasksView);err==nil{
  t.Fatal("cross-project authentication accepted")
 }
 s.Config.SharedProjectID="workspace-A"
 s.Config.RemoteAccessEnabled=false
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleAdmin,identity.PermissionTasksView);err==nil{
  t.Fatal("disabled Remote accepted action")
 }
 s.Config.RemoteAccessEnabled=true
 s.Config.SharedServerEnabled=false
 if err:=s.authorizeRemoteTransportAction(authenticated,remotetransport.RoleAdmin,identity.PermissionTasksView);err==nil{
  t.Fatal("single-user auth was treated as shared TaskDeck login")
 }
 s.Config.SharedServerEnabled=true
 noTLS:=httptest.NewRequest(http.MethodGet,"http://taskdeck.local/api/tasks",nil)
 noTLS=noTLS.WithContext(authenticated.Context())
 if err:=s.authorizeRemoteTransportAction(noTLS,remotetransport.RoleAdmin,identity.PermissionTasksView);err==nil{
  t.Fatal("insecure local HTTP request accepted by remote gate")
 }
}
