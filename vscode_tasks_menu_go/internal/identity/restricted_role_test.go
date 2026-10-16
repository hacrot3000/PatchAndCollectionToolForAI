package identity

import (
 "context"
 "errors"
 "path/filepath"
 "testing"
 "time"
)

// A restricted role is a security boundary, not a permission preset.
func TestRestrictedDeveloperRoleRejectsOverridesAndPinsEffectivePermissions(t *testing.T) {
 ctx:=context.Background()
 db:=openRealSQLiteDatabase(t,filepath.Join(t.TempDir(),identityDBName))
 now:=time.Now().UTC()
 admin,err:=db.BootstrapFirstAdmin(ctx,"role-project","admin",testScryptHash,now)
 if err!=nil{t.Fatal(err)}
 if err=db.SeedSystemRoles(ctx);err!=nil{t.Fatal(err)}
 roles,err:=db.ListProjectRoles(ctx,admin.ProjectID)
 if err!=nil{t.Fatal(err)}
 found:=false
 for _,r:=range roles{
  if r.ID==RestrictedDeveloperRoleID {
   found=true
   if !r.SystemRole||len(r.Permissions)!=len(RestrictedDeveloperPermissions()){t.Fatalf("role configuration: %+v",r)}
  }
 }
 if !found{t.Fatal("restricted role not selectable in project Roles")}
 user:=User{ID:"test-user",Username:"restricted-member",PasswordHash:testScryptHash,
  Enabled:true,CreatedAt:now,UpdatedAt:now}
 if err=db.CreateUser(ctx,user);err!=nil{t.Fatal(err)}
 member:=ProjectMember{ProjectID:admin.ProjectID,UserID:user.ID,RoleID:"system:developer",
  Enabled:true,CreatedAt:now,UpdatedAt:now}
 if err=db.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 // An old override must not survive the transition into restricted mode.
 if err=db.SetMemberPermission(ctx,MemberPermission{ProjectID:admin.ProjectID,
  UserID:user.ID,PermissionID:ID(PermissionTasksRun),Effect:PermissionAllow});err!=nil{t.Fatal(err)}
 member.RoleID=RestrictedDeveloperRoleID
 if err=db.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 principal,err:=ResolveUserPrincipal(ctx,db,"role-project",user)
 if err!=nil{t.Fatal(err)}
 if !principal.Restricted||principal.RoleID!=RestrictedDeveloperRoleID{t.Fatalf("principal=%+v",principal)}
 if len(principal.Permissions)!=len(RestrictedDeveloperPermissions()){t.Fatalf("restricted permission set=%v",principal.Permissions)}
 if principal.Allowed(PermissionTasksRun)||principal.Allowed(PermissionGitWrite)||
  principal.Allowed(PermissionFilesDownload)||principal.Allowed(PermissionUsersManage){
  t.Fatal("restricted member gained unrestricted permissions")
 }
 if !principal.Allowed(PermissionFilesRead)||!principal.Allowed(PermissionFilesWrite)||
  !principal.Allowed(PermissionGitStatus){t.Fatal("approved protected gateway permissions absent")}
 if err=db.SetMemberPermission(ctx,MemberPermission{ProjectID:admin.ProjectID,
  UserID:user.ID,PermissionID:ID(PermissionGitPush),Effect:PermissionAllow});!errors.Is(err,ErrConflict){
  t.Fatalf("restricted user detailed permission assignment accepted: %v",err)
 }
 if err=db.SetProjectRolePermissions(ctx,admin.ProjectID,RestrictedDeveloperRoleID,[]string{PermissionUsersManage});!errors.Is(err,ErrConflict){
  t.Fatalf("reserved restricted role editable: %v",err)
 }
 // The previous permission override is permanently cleared on reassignment;
 // it cannot reappear when admin later restores the developer role.
 member.RoleID="system:developer"
 if err=db.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 restored,err:=db.EffectivePermissions(ctx,admin.ProjectID,user.ID)
 if err!=nil{t.Fatal(err)}
 if restored[PermissionGitPush]{t.Fatal("stale privilege leaked after role transition")}
 overrideRows,err:=db.ListProjectMembers(ctx,admin.ProjectID)
 if err!=nil{t.Fatal(err)}
 for _,item:=range overrideRows{
  if item.UserID==user.ID&&len(item.PermissionOverrides)!=0{
   t.Fatalf("stale member override survived restricted role assignment: %+v",item.PermissionOverrides)
  }
 }
}

func TestRestrictedDeveloperPrincipalCannotUseInjectedPermissions(t *testing.T){
 fake:=Principal{RoleID:RestrictedDeveloperRoleID,
  Permissions:map[string]bool{PermissionUsersManage:true,PermissionGitPush:true,PermissionFilesRead:true}}
 if fake.Allowed(PermissionUsersManage)||fake.Allowed(PermissionGitPush){
  t.Fatal("restricted principal accepted a permissive map")
 }
 if !fake.Allowed(PermissionFilesRead){t.Fatal("restricted reader permission should remain allowed")}
}
