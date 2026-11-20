package remotetransport

import (
 "errors"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestRemotePortalScopeIntersectsActualTaskDeckRBAC(t *testing.T){
 p:=identity.Principal{
  UserID:identity.ID("user-1"),ProjectKey:"workspace-A",
  Permissions:map[string]bool{
   identity.PermissionFilesRead:true,
   identity.PermissionTasksView:true,
   identity.PermissionTasksRun:true,
   identity.PermissionTerminalCreate:true,
   identity.PermissionPatchRun:true,
  },
 }
 cases:=[]struct{name string;role Role;permission string;allowed bool}{
  {"viewer read",RoleViewer,identity.PermissionFilesRead,true},
  {"viewer cannot execute",RoleViewer,identity.PermissionTasksRun,false},
  {"viewer cannot terminal",RoleViewer,identity.PermissionTerminalCreate,false},
  {"operator can run with TaskDeck grant",RoleOperator,identity.PermissionTasksRun,true},
  {"operator cannot Patch execute",RoleOperator,identity.PermissionPatchRun,false},
  {"operator cannot terminal",RoleOperator,identity.PermissionTerminalCreate,false},
  {"administrator still needs TaskDeck permission",RoleAdmin,identity.PermissionPatchRun,true},
  {"admin denied missing TaskDeck grant",RoleAdmin,identity.PermissionFilesWrite,false},
  {"unknown portal role",Role("superuser"),identity.PermissionFilesRead,false},
  {"unknown permission",RoleAdmin,"remote.execute_all",false},
 }
 for _,tc:=range cases {
  t.Run(tc.name,func(t *testing.T){
   err:=AuthorizeAction(true,tc.role,&p,"workspace-A",tc.permission)
   if (err==nil)!=tc.allowed {t.Fatalf("role=%s perm=%s allowed=%v err=%v",tc.role,tc.permission,tc.allowed,err)}
   if err!=nil&&!errors.Is(err,ErrRemoteActionDenied){t.Fatal(err)}
  })
 }
 if err:=AuthorizeAction(true,RoleAdmin,nil,"workspace-A",identity.PermissionTasksRun);err==nil{
  t.Fatal("Portal admin bypassed missing TaskDeck login")
 }
 if err:=AuthorizeAction(false,RoleAdmin,&p,"workspace-A",identity.PermissionTasksRun);err==nil{
  t.Fatal("disabled Remote allowed an application command")
 }
 if err:=AuthorizeAction(true,RoleAdmin,&p,"workspace-B",identity.PermissionTasksRun);err==nil{
  t.Fatal("cross-project access allowed")
 }
 noUser:=p;noUser.UserID=""
 if err:=AuthorizeAction(true,RoleAdmin,&noUser,"workspace-A",identity.PermissionTasksRun);err==nil{
  t.Fatal("anonymous TaskDeck principal accepted")
 }
}

func TestRestrictedDeveloperCannotEscapeByPortalAdminRole(t *testing.T) {
 p:=identity.Principal{
  UserID:"dev",ProjectKey:"workspace-A",
  RoleID:identity.RestrictedDeveloperRoleID,
  Restricted:true,
  Permissions:map[string]bool{
   identity.PermissionFilesRead:true,
   identity.PermissionFilesWrite:true,
   identity.PermissionTasksRun:true,
   identity.PermissionTerminalCreate:true,
  },
 }
 if err:=AuthorizeAction(true,RoleAdmin,&p,"workspace-A",identity.PermissionTasksRun);err==nil{
  t.Fatal("Portal admin escaped restricted developer policy")
 }
 if err:=AuthorizeAction(true,RoleViewer,&p,"workspace-A",identity.PermissionFilesRead);err!=nil{
  t.Fatalf("existing permitted read incorrectly blocked: %v",err)
 }
 if err:=AuthorizeAction(true,RoleViewer,&p,"workspace-A",identity.PermissionFilesWrite);err==nil{
  t.Fatal("Portal viewer gained write access")
 }
}
