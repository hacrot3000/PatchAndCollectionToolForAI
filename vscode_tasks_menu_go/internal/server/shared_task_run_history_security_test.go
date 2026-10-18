package server

import (
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
    "bletonfc/vscode_tasks_menu/internal/identity"
    "bletonfc/vscode_tasks_menu/internal/session"
    "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestSharedTaskRunHistoryRequiresProjectPrincipal(t *testing.T) {
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg,Workspace:t.TempDir()}
    item:=taskRunRecord{ID:"run-a",ProjectID:"project-a",OwnerUserID:"alice"}
    anonymous:=httptest.NewRequest(http.MethodGet,"/api/task-runs",nil)
    if s.taskRunVisible(anonymous,item) {
        t.Fatal("anonymous shared request can view task-run history")
    }
    if _,err:=s.recordTaskRun(anonymous,taskRunRecordRequest{SessionID:"guessed-session"});err==nil||
        !strings.Contains(err.Error(),"not visible"){
        t.Fatalf("shared task-run recorder examined unauthenticated session: %v",err)
    }
    principal:=identity.Principal{UserID:"alice",ProjectID:"project-a",
        Permissions:map[string]bool{identity.PermissionTasksView:true}}
    owner:=anonymous.WithContext(context.WithValue(anonymous.Context(),sharedPrincipalContextKey{},principal))
    if !s.taskRunVisible(owner,item){
        t.Fatal("legitimate project task-view grant should see task history")
    }
    foreign:=identity.Principal{UserID:"alice",ProjectID:"project-b",
        Permissions:map[string]bool{identity.PermissionTasksView:true}}
    outsider:=anonymous.WithContext(context.WithValue(anonymous.Context(),sharedPrincipalContextKey{},foreign))
    if s.taskRunVisible(outsider,item) {
        t.Fatal("task-run history visible across project boundary")
    }
    cfg.SharedServerEnabled=false
    s.Config=cfg
    if !s.taskRunVisible(anonymous,item) {
        t.Fatal("local single-user task-run history was restricted")
    }
    if _,err:=s.recordTaskRun(anonymous,taskRunRecordRequest{SessionID:"missing"});err==nil||
        !strings.Contains(err.Error(),"session service unavailable"){
        t.Fatalf("local task recorder must preserve normal session lookup: %v",err)
    }
}


func TestSharedTaskRunLogWritesRequireOwnerOrProjectAdmin(t *testing.T) {
    meta:=session.Metadata{Kind:tasks.SessionKindTask,ProjectID:"project-a",OwnerUserID:"alice"}
    principal:=func(user,project string,perms ...string) identity.Principal {
        grant:=map[string]bool{}
        for _,permission:=range perms {grant[permission]=true}
        return identity.Principal{UserID:identity.ID(user),ProjectID:identity.ID(project),Permissions:grant}
    }
    viewer:=principal("bob","project-a",identity.PermissionTasksView)
    if sharedTaskRunRecordAllowed(viewer,meta) {
        t.Fatal("read-only task viewer overwrote a task log")
    }
    sameProjectRunner:=principal("bob","project-a",identity.PermissionTasksView,identity.PermissionTasksRun)
    if sharedTaskRunRecordAllowed(sameProjectRunner,meta){
        t.Fatal("different user's task run log was writable")
    }
    owner:=principal("alice","project-a",identity.PermissionTasksView,identity.PermissionTasksRun)
    if !sharedTaskRunRecordAllowed(owner,meta) {
        t.Fatal("task run originator cannot record own log")
    }
    admin:=principal("admin","project-a",identity.PermissionTasksView,identity.PermissionTasksRun,identity.PermissionProjectAdmin)
    if !sharedTaskRunRecordAllowed(admin,meta) {
        t.Fatal("project admin cannot maintain same-project task run logs")
    }
    foreignAdmin:=principal("admin","project-b",identity.PermissionTasksView,identity.PermissionTasksRun,identity.PermissionProjectAdmin)
    if sharedTaskRunRecordAllowed(foreignAdmin,meta) {
        t.Fatal("admin from another project can overwrite task logs")
    }
    meta.OwnerUserID=""
    if sharedTaskRunRecordAllowed(owner,meta) {
        t.Fatal("ownerless task run accepted a delegated log writer")
    }
}
