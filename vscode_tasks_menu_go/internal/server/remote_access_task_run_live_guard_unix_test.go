//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

type remoteShortTaskAuditRevoke struct{
 identity.Store
 flag string
 role bool
 session *remoteTaskDeckSession
}
func(store remoteShortTaskAuditRevoke)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.tasks_run"||event.Result!="attempted"{
  return nil
 }
 if store.flag!=""{
  if err:=os.Setenv(store.flag,"0");err!=nil{return err}
 }
 if store.role{
  project,err:=store.Store.ProjectByKey(ctx,store.session.projectKey)
  if err!=nil{return err}
  member,err:=store.Store.ProjectMember(ctx,project.ID,store.session.userID)
  if err!=nil{return err}
  member.RoleID=identity.RestrictedDeveloperRoleID
  member.UpdatedAt=time.Now().UTC()
  return store.Store.UpsertProjectMember(ctx,member)
 }
 return nil
}

// These tests hit the exact audit->exec race with a genuine admin TaskDeck
// session and a process with an observable side effect. The attempted audit
// alone can NEVER launch a task after host opt-out or a local role change.
func TestRemoteShortTaskPostAuditRevocationPreventsExecution(t *testing.T){
 for _,tc:=range []struct{name,flag string;role bool}{
  {"preview_off","TASKDECK_REMOTE_TASKS_PREVIEW",false},
  {"run_off","TASKDECK_REMOTE_TASKS_RUN",false},
  {"role_downgrade","",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
    `{"tasks":[{"label":"Audit-bound","type":"process","command":"/bin/sh",
     "args":["-c","touch must-not-execute"]}]}`)
   original:=s.Identity
   s.Identity=remoteShortTaskAuditRevoke{
    Store:original,flag:tc.flag,role:tc.role,session:session}
   if result,err:=s.runRemoteTaskBounded(context.Background(),session,taskID,0);
    err==nil||result.Output!=""{
    t.Fatalf("revocation at attempted audit launched task: %+v %v",result,err)
   }
   if _,err:=os.Stat(filepath.Join(s.Workspace,"must-not-execute"));
    !os.IsNotExist(err){
    t.Fatalf("post-audit revoked Task ran a host command: %v",err)
   }
  })
 }
}

func TestRemoteShortTaskLiveGuardStopsActiveProcessAfterHostOptOut(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Active host task","type":"process","command":"/bin/sh",
   "args":["-c","touch started-task; sleep 4; touch forbidden-after-revoke"]}]}`)
 result:=make(chan error,1)
 go func(){
  _,err:=s.runRemoteTaskBounded(context.Background(),session,taskID,0)
  result<-err
 }()
 started:=filepath.Join(s.Workspace,"started-task")
 deadline:=time.After(3*time.Second)
 for{
  if _,err:=os.Stat(started);err==nil{break}
  select{
  case err:=<-result:t.Fatalf("task quit before opt-out: %v",err)
  case <-deadline:t.Fatal("short task never launched")
  case <-time.After(10*time.Millisecond):
  }
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","0")
 select{
 case err:=<-result:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("active task survived host opt-out: %v",err)
  }
 case <-time.After(3*time.Second):
  t.Fatal("revoked task kept running instead of cancelling its process group")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"forbidden-after-revoke"));
  !os.IsNotExist(err){t.Fatalf("revoked task completed a forbidden side effect: %v",err)}
}

// A TaskDeck local role can be downgraded without invalidating the login
// or the Portal ceiling. The active process must still stop promptly.
func TestRemoteShortTaskLiveGuardStopsActiveProcessAfterRoleDowngrade(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,_,_:=remoteTaskOperatorFixture(t)
 ctx,cancel:=context.WithTimeout(context.Background(),4*time.Second)
 defer cancel()
 entered:=make(chan struct{})
 done:=make(chan error,1)
 go func(){
  _,err:=s.runRemoteShortTaskWithLiveRBAC(ctx,session,
   func(runCtx context.Context)error{
    close(entered)
    <-runCtx.Done()
    return runCtx.Err()
   })
  done<-err
 }()
 select{
 case <-entered:
 case <-time.After(3*time.Second):t.Fatal("authorized process was not admitted")
 }
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil{t.Fatal(err)}
 member,err:=s.Identity.ProjectMember(ctx,project.ID,session.userID)
 if err!=nil{t.Fatal(err)}
 member.RoleID=identity.RestrictedDeveloperRoleID
 member.UpdatedAt=time.Now().UTC()
 if err:=s.Identity.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 select{
 case err:=<-done:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("active TasksRun role downgrade ignored: %v",err)
  }
  if ctx.Err()!=nil{t.Fatal("task only stopped at request timeout")}
 case <-time.After(3*time.Second):
  t.Fatal("local TasksRun revocation left task process active")
 }
}

func TestRemoteShortTaskGuardPreservesNormalNonzeroExit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,_,_:=remoteTaskOperatorFixture(t)
 sentinel:=errors.New("process exit 7")
 execErr,authErr:=s.runRemoteShortTaskWithLiveRBAC(
  context.Background(),session,func(ctx context.Context)error{
   if ctx.Err()!=nil{t.Fatal("authorized task was cancelled")}
   return sentinel
  })
 if !errors.Is(execErr,sentinel)||authErr!=nil{
  t.Fatalf("nonzero process exit was wrongly treated as loss of authority: %v / %v",execErr,authErr)
 }
}


type remoteShortTaskAuditDrift struct{
 identity.Store
 filename string
 symlink string
 target string
}
func(store remoteShortTaskAuditDrift)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.tasks_run"||event.Result!="attempted"{
  return nil
 }
 if store.filename!=""{
  original,err:=os.ReadFile(store.filename)
  if err!=nil{return err}
  // Stable Task.ID remains unchanged. Only the authoritative source
  // hash can catch this update after the attempted-audit I/O.
  return os.WriteFile(store.filename,append(original,' '),0600)
 }
 if store.symlink!=""{
  if err:=os.Remove(store.symlink);err!=nil{return err}
  return os.Symlink(store.target,store.symlink)
 }
 return nil
}

func TestRemoteShortTaskPostAuditSnapshotDriftRejectsExecution(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 for _,which:=range []string{"source_change","cwd_symlink_change"}{
  t.Run(which,func(t *testing.T){
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   source:=`{"tasks":[{"label":"Post-audit-bound","type":"process",
    "command":"/bin/sh","args":["-c","touch stale-process-ran"]}]}`
   var injected remoteShortTaskAuditDrift
   if which=="cwd_symlink_change"{
    for _,directory:=range []string{"original-dir","new-dir"}{
     if err:=os.Mkdir(filepath.Join(s.Workspace,directory),0700);err!=nil{
      t.Fatal(err)
     }
    }
    link:=filepath.Join(s.Workspace,"working")
    if err:=os.Symlink("original-dir",link);err!=nil{t.Fatal(err)}
    source=`{"tasks":[{"label":"Post-audit-bound","type":"process",
     "command":"/bin/sh","args":["-c","touch stale-process-ran"],
     "options":{"cwd":"working"}}]}`
    injected.symlink=link
    injected.target="new-dir"
   }else{
    injected.filename=filepath.Join(s.Workspace,".vscode","tasks.json")
   }
   taskID:=previewOneRemoteRunTask(t,s,session,client,agent,source)
   injected.Store=s.Identity
   s.Identity=injected
   if result,err:=s.runRemoteTaskBounded(context.Background(),session,taskID,0);
    err==nil||result.Output!=""{
    t.Fatalf("stale Task launched after audit source/cwd drift: %+v %v",result,err)
   }
   for _,path:=range []string{
    filepath.Join(s.Workspace,"stale-process-ran"),
    filepath.Join(s.Workspace,"original-dir","stale-process-ran"),
    filepath.Join(s.Workspace,"new-dir","stale-process-ran"),
   }{
    if _,err:=os.Stat(path);!os.IsNotExist(err){
     t.Fatalf("post-audit changed Task had side effects at %q: %v",path,err)
    }
   }
  })
 }
}
