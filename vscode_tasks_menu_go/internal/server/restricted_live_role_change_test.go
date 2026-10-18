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

// An existing browser session must not cache the old Developer grants.
// Authentication must re-resolve the current project member on each request.
func TestSharedLiveRoleDowngradeImmediatelyChangesExistingSession(t *testing.T) {
 s:=sharedLoginTestServer(t)
 ctx:=context.Background()
 administrator,err:=s.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 memberID,err:=identity.NewID()
 if err!=nil{t.Fatal(err)}
 member:=identity.ProjectMember{ProjectID:project.ID,UserID:memberID,
  RoleID:"system:developer",Enabled:true,CreatedAt:now,UpdatedAt:now}
 if err=s.Identity.CreateProjectUser(ctx,identity.User{
  ID:memberID,Username:"downgrade-member",PasswordHash:administrator.PasswordHash,
  Enabled:true,CreatedAt:now,UpdatedAt:now,
 },member);err!=nil{t.Fatal(err)}
 if err=s.Identity.SetMemberPermission(ctx,identity.MemberPermission{
  ProjectID:project.ID,UserID:memberID,
  PermissionID:identity.ID(identity.PermissionTasksRun),
  Effect:identity.PermissionAllow,
 });err!=nil{t.Fatal(err)}
 existingCookie:=sharedAPILogin(t,s,"downgrade-member")
 request:=func(path string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodGet,"https://taskdeck.test"+path,nil)
  req.AddCookie(existingCookie)
  out:=httptest.NewRecorder()
  s.Handler().ServeHTTP(out,req)
  return out
 }
 before:=request("/api/auth/me")
 if before.Code!=http.StatusOK||
  !strings.Contains(before.Body.String(),`"role_id":"system:developer"`){
  t.Fatalf("developer session not authenticated: %d %s",before.Code,before.Body)
 }
 member.RoleID=identity.RestrictedDeveloperRoleID
 member.UpdatedAt=time.Now().UTC()
 if err=s.Identity.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 after:=request("/api/auth/me")
 if after.Code!=http.StatusOK||
  !strings.Contains(after.Body.String(),`"role_id":"system:restricted-developer"`){
  t.Fatalf("old cookie retained developer privileges: %d %s",after.Code,after.Body)
 }
 for _,path:=range []string{"/api/schedules","/api/tasks","/api/git/status","/admin/access"}{
  out:=request(path)
  if out.Code!=http.StatusForbidden{
   t.Fatalf("downgraded session escaped restricted policy at %s: %d %s",path,out.Code,out.Body)
  }
 }
 member.RoleID="system:developer"
 member.UpdatedAt=time.Now().UTC()
 if err=s.Identity.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 members,err:=s.Identity.ListProjectMembers(ctx,project.ID)
 if err!=nil{t.Fatal(err)}
 for _,m:=range members{
  if m.UserID==memberID&&len(m.PermissionOverrides)!=0{
   t.Fatalf("old granular grants reappeared after returning to Developer: %+v",m.PermissionOverrides)
  }
 }
 // No re-login is required for the new effective role, but deleted overrides
 // must never be reattached to the still-active session.
 restored:=request("/api/auth/me")
 if restored.Code!=http.StatusOK||
  !strings.Contains(restored.Body.String(),`"role_id":"system:developer"`){
  t.Fatalf("restored developer session invalid: %d %s",restored.Code,restored.Body)
 }
}
