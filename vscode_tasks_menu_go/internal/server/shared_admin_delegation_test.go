package server

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
)

// Only project administrators may delegate privileges that exceed their own.
// A users.manage/roles.manage member must not mint an administrator account
// or gain project.admin by creating an account and granting an override.
func TestDelegatedUserManagerCannotEscalateThroughAdminAPIs(t *testing.T) {
 s:=sharedLoginTestServer(t)
 ctx:=context.Background()
 admin,err:=s.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 memberID:=identity.ID("delegation-manager")
 if err=s.Identity.CreateProjectUser(ctx,identity.User{
  ID:memberID,Username:"delegation-manager",PasswordHash:admin.PasswordHash,
  Enabled:true,CreatedAt:now,UpdatedAt:now,
 },identity.ProjectMember{
  ProjectID:project.ID,UserID:memberID,RoleID:"system:developer",
  Enabled:true,CreatedAt:now,UpdatedAt:now,
 });err!=nil{t.Fatal(err)}
 for _,key:=range []string{
  identity.PermissionUsersManage,identity.PermissionUsersView,
  identity.PermissionRolesManage,identity.PermissionRolesView,
 } {
  if err=s.Identity.SetMemberPermission(ctx,identity.MemberPermission{
   ProjectID:project.ID,UserID:memberID,PermissionID:identity.ID(key),
   Effect:identity.PermissionAllow,
  });err!=nil{t.Fatal(err)}
 }
 effective,err:=s.Identity.EffectivePermissions(ctx,project.ID,memberID)
 if err!=nil{t.Fatal(err)}
 if effective[identity.PermissionProjectAdmin]||!effective[identity.PermissionUsersManage]||
  !effective[identity.PermissionRolesManage]||!effective[identity.PermissionFilesRead]{
  t.Fatalf("invalid delegated-role test setup: %v",effective)
 }
 cookie:=sharedAPILogin(t,s,"delegation-manager")
 request:=func(method,path string,payload any) *httptest.ResponseRecorder {
  t.Helper()
  encoded,err:=json.Marshal(payload);if err!=nil{t.Fatal(err)}
  req:=httptest.NewRequest(method,"https://taskdeck.test"+path,strings.NewReader(string(encoded)))
  req.Header.Set("Content-Type","application/json")
  req.AddCookie(cookie)
  rec:=httptest.NewRecorder()
  s.Handler().ServeHTTP(rec,req)
  return rec
 }
 assert:=func(label string,rec *httptest.ResponseRecorder,status int){
  t.Helper()
  if rec.Code!=status{t.Fatalf("%s: status=%d want=%d body=%s",label,rec.Code,status,rec.Body)}
 }
 createAdmin:=request(http.MethodPost,"/api/admin/users",map[string]any{
  "username":"delegation-forbidden-admin","password":"restricted-test-password-123!","role_id":"system:admin",
 })
 assert("create administrator",createAdmin,http.StatusForbidden)
 if _,err:=s.Identity.UserByUsername(ctx,"delegation-forbidden-admin");err==nil{
  t.Fatal("administrator created despite delegated privilege ceiling")
 }
 createChild:=request(http.MethodPost,"/api/admin/users",map[string]any{
  "username":"delegation-child","password":"restricted-test-password-123!","role_id":"system:developer",
 })
 assert("create ordinary user",createChild,http.StatusCreated)
 child,err:=s.Identity.UserByUsername(ctx,"delegation-child")
 if err!=nil{t.Fatal(err)}
 changeToAdmin:=request(http.MethodPatch,"/api/admin/users/access",map[string]any{
  "user_id":child.ID,"role_id":"system:admin","enabled":true,
 })
 assert("assign administrator role",changeToAdmin,http.StatusForbidden)
 target,err:=s.Identity.ProjectMember(ctx,project.ID,child.ID)
 if err!=nil{t.Fatal(err)}
 if target.RoleID!="system:developer"{t.Fatal("unauthorized role reassignment applied")}
 downgradeAdmin:=request(http.MethodPatch,"/api/admin/users/access",map[string]any{
  "user_id":admin.ID,"role_id":"system:developer","enabled":false,
 })
 assert("modify more-privileged member",downgradeAdmin,http.StatusForbidden)
 adminMember,err:=s.Identity.ProjectMember(ctx,project.ID,admin.ID)
 if err!=nil{t.Fatal(err)}
 if !adminMember.Enabled||adminMember.RoleID!="system:admin"{
  t.Fatal("administrator modified by delegated user manager")
 }
 // A disabled admin has no current effective grants. The immutable
 // underlying administrator role must still protect that membership.
 dormantID:=identity.ID("dormant-elevated-user")
 if err=s.Identity.CreateProjectUser(ctx,identity.User{
  ID:dormantID,Username:"dormant-elevated-user",PasswordHash:admin.PasswordHash,
  Enabled:true,CreatedAt:now,UpdatedAt:now,
 },identity.ProjectMember{
  ProjectID:project.ID,UserID:dormantID,RoleID:"system:admin",
  Enabled:false,CreatedAt:now,UpdatedAt:now,
 });err!=nil{t.Fatal(err)}
 enableDormantAdmin:=request(http.MethodPatch,"/api/admin/users/access",map[string]any{
  "user_id":dormantID,"role_id":"system:developer","enabled":true,
 })
 assert("change disabled administrator membership",enableDormantAdmin,http.StatusForbidden)
 dormant,err:=s.Identity.ProjectMember(ctx,project.ID,dormantID)
 if err!=nil{t.Fatal(err)}
 if dormant.Enabled||dormant.RoleID!="system:admin"{t.Fatal("disabled elevated member was modified")}

 grantAdmin:=request(http.MethodPut,"/api/admin/users/permission",map[string]any{
  "user_id":child.ID,"permission_key":identity.PermissionProjectAdmin,"effect":"ALLOW",
 })
 assert("grant project.admin override",grantAdmin,http.StatusForbidden)
 revokeAdmin:=request(http.MethodPut,"/api/admin/users/permission",map[string]any{
  "user_id":admin.ID,"permission_key":identity.PermissionFilesRead,"effect":"DENY",
 })
 assert("modify administrator overrides",revokeAdmin,http.StatusForbidden)
 ordinaryGrant:=request(http.MethodPut,"/api/admin/users/permission",map[string]any{
  "user_id":child.ID,"permission_key":identity.PermissionFilesRead,"effect":"ALLOW",
 })
 assert("delegate held permission",ordinaryGrant,http.StatusNoContent)

 createRole:=request(http.MethodPost,"/api/admin/roles",map[string]any{"name":"Delegated team role"})
 assert("create empty delegated role",createRole,http.StatusCreated)
 roles,err:=s.Identity.ListProjectRoles(ctx,project.ID)
 if err!=nil{t.Fatal(err)}
 var roleID identity.ID
 for _,role:=range roles{if role.Name=="Delegated team role"{roleID=role.ID;break}}
 if roleID==""{t.Fatal("created role not found")}
 grantRoleAdmin:=request(http.MethodPatch,"/api/admin/roles",map[string]any{
  "role_id":roleID,"permissions":[]string{identity.PermissionProjectAdmin},
 })
 assert("grant project.admin to custom role",grantRoleAdmin,http.StatusForbidden)
 grantOwnRole:=request(http.MethodPatch,"/api/admin/roles",map[string]any{
  "role_id":roleID,"permissions":[]string{identity.PermissionFilesRead},
 })
 assert("set custom role to held permission",grantOwnRole,http.StatusNoContent)
 alterAdminRole:=request(http.MethodPatch,"/api/admin/roles",map[string]any{
  "role_id":"system:admin","permissions":[]string{identity.PermissionFilesRead},
 })
 assert("alter more-privileged role",alterAdminRole,http.StatusForbidden)
}
