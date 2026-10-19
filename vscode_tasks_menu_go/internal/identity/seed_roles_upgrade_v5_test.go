package identity

import (
 "context"
 "path/filepath"
 "testing"
 "time"
)

// Exercise real SQLite, not only a mock of the seed queries. Both historical
// branches wrote identity_role_seeds.version=5 with different meanings.
func TestRoleSeedUpgradesBothHistoricalV5Databases(t *testing.T) {
 for _,from:=range []string{"main_schedule_v5","feature_restricted_v5"}{
  t.Run(from,func(t *testing.T){
   ctx:=context.Background()
   db:=openRealSQLiteDatabase(t,filepath.Join(t.TempDir(),identityDBName))
   now:=time.Now().UTC()
   admin,err:=db.BootstrapFirstAdmin(ctx,"upgrade-project","alice",testScryptHash,now)
   if err!=nil{t.Fatal(err)}
   if err=db.SeedSystemRoles(ctx);err!=nil{t.Fatal(err)}
   // Simulate a pre-merge V5 DB. The current seeder produced V6/V7
   // on initial bootstrap, but old deployed DBs did not contain them.
   if _,err=db.db.ExecContext(ctx,"DELETE FROM identity_role_seeds WHERE version IN (6,7)");err!=nil{t.Fatal(err)}
   if _,err=db.db.ExecContext(ctx,"INSERT OR IGNORE INTO identity_role_seeds(version) VALUES(5)");err!=nil{t.Fatal(err)}
   // Permission removed by the administrator must remain absent.
   if err=db.SetRolePermissions(ctx,"system:admin",[]string{PermissionProjectAdmin,PermissionFilesRead});err!=nil{t.Fatal(err)}
   if from=="main_schedule_v5"{
    // Main V5 already registered schedule keys. Their admin grants were
    // deliberately revoked; V6 must not restore them merely because V5
    // was used for a different feature on the other branch.
    if _,err=db.db.ExecContext(ctx,"DELETE FROM roles WHERE id=?",string(RestrictedDeveloperRoleID));err!=nil{t.Fatal(err)}
   }else{
    // Feature V5 had the immutable Restricted Role but not schedules.
    // Drop the schedule keys to model the historic database precisely.
    for _,key:=range PermissionUpgradeV5Keys(){
     if _,err=db.db.ExecContext(ctx,"DELETE FROM permissions WHERE permission_key=?",key);err!=nil{t.Fatal(err)}
    }
    user:=User{ID:"prior-restricted-user",Username:"bob",PasswordHash:testScryptHash,
     Enabled:true,CreatedAt:now,UpdatedAt:now}
    if err=db.CreateUser(ctx,user);err!=nil{t.Fatal(err)}
    member:=ProjectMember{ProjectID:admin.ProjectID,UserID:user.ID,
     RoleID:RestrictedDeveloperRoleID,Enabled:true,CreatedAt:now,UpdatedAt:now}
    if err=db.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
    // Simulate an obsolete legacy override by injecting a row that
    // current SetMemberPermission correctly refuses to create.
    if _,err=db.db.ExecContext(ctx,`INSERT INTO member_permissions
      (project_id,user_id,permission_id,effect) VALUES(?,?,?,'ALLOW')`,
      string(admin.ProjectID),string(user.ID),PermissionUsersManage);err!=nil{t.Fatal(err)}
   }
   if err=db.SeedSystemRoles(ctx);err!=nil{t.Fatalf("migrate %s: %v",from,err)}
   for _,v:=range []int{5,6,7}{
    var count int
    if err=db.db.QueryRowContext(ctx,"SELECT count(*) FROM identity_role_seeds WHERE version=?",v).Scan(&count);err!=nil||count!=1{
     t.Fatalf("seed version %d count=%d: %v",v,count,err)
    }
   }
   effective,err:=db.EffectivePermissions(ctx,admin.ProjectID,admin.UserID)
   if err!=nil{t.Fatal(err)}
   if effective[PermissionUsersManage]||effective[PermissionGitPush]||
    effective[PermissionSettingsWrite]||effective[PermissionSchedulesRun]&&from=="main_schedule_v5"{
    t.Fatalf("migration regranted revoked admin permissions: %v",effective)
   }
   for _,key:=range PermissionUpgradeV5Keys(){
    if !KnownPermission(key){t.Fatalf("unknown schedule permission %s",key)}
    var registered int
    if err=db.db.QueryRowContext(ctx,"SELECT COUNT(*) FROM permissions WHERE permission_key=?",key).Scan(&registered);err!=nil||registered!=1{
     t.Fatalf("schedule key %s missing: %v",key,err)
    }
    if from=="main_schedule_v5"&&effective[key]{
     t.Fatalf("migrated main V5 restored revoked %s",key)
    }
   }
   roles,err:=db.ListProjectRoles(ctx,admin.ProjectID)
   if err!=nil{t.Fatal(err)}
   found:=false
   for _,role:=range roles{if role.ID==RestrictedDeveloperRoleID{
    found=true
    if len(role.Permissions)!=len(RestrictedDeveloperPermissions())||
     !role.SystemRole{t.Fatalf("restricted role widened: %+v",role)}
   }}
   if !found{t.Fatal("restricted developer role lost")}
   if from=="feature_restricted_v5"{
    var overrides int
    if err=db.db.QueryRowContext(ctx,`SELECT count(*) FROM member_permissions
     WHERE project_id=? AND user_id=?`,string(admin.ProjectID),"prior-restricted-user").Scan(&overrides);err!=nil||overrides!=0{
     t.Fatalf("old restricted user override survived: count=%d err=%v",overrides,err)
    }
    member:=ProjectMember{ProjectID:admin.ProjectID,UserID:"prior-restricted-user",
     RoleID:"system:developer",Enabled:true,CreatedAt:now,UpdatedAt:now}
    if err=db.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
    userPerms,err:=db.EffectivePermissions(ctx,admin.ProjectID,member.UserID)
    if err!=nil{t.Fatal(err)}
    if userPerms[PermissionUsersManage]{t.Fatal("old ALLOW override reappeared after role transition")}
   }
   // Startup is repeatable. Once V6/V7 exist it must not widen permissions.
   if err=db.SeedSystemRoles(ctx);err!=nil{t.Fatal(err)}
   repeat,err:=db.EffectivePermissions(ctx,admin.ProjectID,admin.UserID)
   if err!=nil{t.Fatal(err)}
   if len(repeat)!=len(effective){t.Fatalf("repeated seed altered grants: before=%v after=%v",effective,repeat)}
  })
 }
}
