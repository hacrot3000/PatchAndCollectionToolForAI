package server

import (
 "context"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedAdminAssignRestrictedRoleWithoutEditPermissions(t *testing.T){
 s:=sharedLoginTestServer(t)
 ctx:=context.Background()
 admin,err:=s.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
 if err!=nil{t.Fatal(err)}
 id,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 if err=s.Identity.CreateProjectUser(ctx,
  identity.User{ID:id,Username:"restricted-member",PasswordHash:admin.PasswordHash,Enabled:true,CreatedAt:now,UpdatedAt:now},
  identity.ProjectMember{ProjectID:project.ID,UserID:id,RoleID:"system:developer",Enabled:true,CreatedAt:now,UpdatedAt:now},
 );err!=nil{t.Fatal(err)}
 if err=s.Identity.SetMemberPermission(ctx,identity.MemberPermission{
  ProjectID:project.ID,UserID:id,PermissionID:identity.ID(identity.PermissionGitPush),Effect:identity.PermissionAllow,
 });err!=nil{t.Fatal(err)}
 cookie:=sharedAPILogin(t,s,"alice")
 access:=httptest.NewRequest(http.MethodPatch,"https://taskdeck.test/api/admin/users/access",
  strings.NewReader(`{"user_id":"`+string(id)+`","role_id":"system:restricted-developer","enabled":true}`))
 access.Header.Set("Content-Type","application/json")
 access.AddCookie(cookie)
 result:=httptest.NewRecorder()
 s.Handler().ServeHTTP(result,access)
 if result.Code!=http.StatusNoContent{t.Fatalf("assign restricted role: %d %s",result.Code,result.Body)}
 member,err:=s.Identity.ProjectMember(ctx,project.ID,id)
 if err!=nil||member.RoleID!=identity.RestrictedDeveloperRoleID{t.Fatalf("role assignment: %+v %v",member,err)}
 user,err:=s.Identity.UserByID(ctx,id)
 if err!=nil{t.Fatal(err)}
 resolved,err:=identity.ResolveUserPrincipal(ctx,s.Identity,s.Config.SharedProjectID,user)
 if err!=nil{t.Fatal(err)}
 if !resolved.Restricted||resolved.Allowed(identity.PermissionGitPush){t.Fatal("stale overrides escaped role boundary")}

 // Even project admin cannot write granular overrides for this role.
 change:=httptest.NewRequest(http.MethodPut,"https://taskdeck.test/api/admin/users/permission",
  strings.NewReader(`{"user_id":"`+string(id)+`","permission_key":"git.push","effect":"ALLOW"}`))
 change.Header.Set("Content-Type","application/json")
 change.AddCookie(cookie)
 denied:=httptest.NewRecorder()
 s.Handler().ServeHTTP(denied,change)
 if denied.Code!=http.StatusConflict||!strings.Contains(denied.Body.String(),"does not support Edit permissions"){
  t.Fatalf("admin API accepted restricted override: %d %s",denied.Code,denied.Body)
 }
 memberCookie:=sharedAPILogin(t,s,"restricted-member")
 for _,path:=range []string{"/api/project/file","/api/git/status","/api/sessions","/api/admin/users","/admin/access"}{
  req:=httptest.NewRequest(http.MethodGet,"https://taskdeck.test"+path,nil)
  req.AddCookie(memberCookie)
  out:=httptest.NewRecorder()
  s.Handler().ServeHTTP(out,req)
  if out.Code!=http.StatusForbidden{t.Fatalf("restricted %s: %d %s",path,out.Code,out.Body)}
 }
 policyStatus:=httptest.NewRequest(http.MethodGet,"https://taskdeck.test/api/restricted/status",nil)
 policyStatus.AddCookie(memberCookie)
 policyResult:=httptest.NewRecorder()
 s.Handler().ServeHTTP(policyResult,policyStatus)
 if policyResult.Code!=http.StatusOK||
  !strings.Contains(policyResult.Body.String(),`"max_visible_lines":80`)||
  !strings.Contains(policyResult.Body.String(),`"ready":true`)||
  !strings.Contains(policyResult.Body.String(),`"release_ready":false`)||
  !strings.Contains(policyResult.Body.String(),`"policy_source":"restricted_role"`){
  t.Fatalf("restricted member status endpoint unavailable: %d %s",policyResult.Code,policyResult.Body)
 }
 page:=httptest.NewRequest(http.MethodGet,"https://taskdeck.test/",nil)
 page.AddCookie(memberCookie)
 ui:=httptest.NewRecorder()
 s.Handler().ServeHTTP(ui,page)
 if ui.Code!=http.StatusOK||!strings.Contains(ui.Body.String(),"Protected Code View"){
  t.Fatalf("restricted role did not receive protected UI: %d %s",ui.Code,ui.Body)
 }
 // Admin remains on the usual management UI; role policy is per-member.
 adminRequest:=httptest.NewRequest(http.MethodGet,"https://taskdeck.test/admin/access",nil)
 adminRequest.AddCookie(cookie)
 adminUI:=httptest.NewRecorder()
 s.Handler().ServeHTTP(adminUI,adminRequest)
 if adminUI.Code!=http.StatusOK{t.Fatalf("admin access interrupted: %d",adminUI.Code)}
}
